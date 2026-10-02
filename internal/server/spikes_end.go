package server

// Ending a spike's run (SPEC-021 FR-6): the leak check, the findings, the
// state, the commit and the discarded worktree, each step safe to run again so
// that a restart at any point leaves nothing stuck, and the reconciliation
// that finishes whatever an earlier call left (FR-6.4).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/config"
	"subutai/internal/rules"
	"subutai/internal/store"
)

// endSpikeAction carries out the rules' EndSpike (FR-6.1). A run that
// concluded has its final findings saved as the draft first, so the write-up
// is the same whichever way the run ended. An exhausted run is then marked
// cancelled, so the retry sweep stops republishing it.
func (s *Server) endSpikeAction(ctx context.Context, a rules.EndSpike) error {
	if a.How == store.SpikeConcluded && strings.TrimSpace(a.Findings) != "" {
		if err := store.SaveSpikeDraft(ctx, s.Store.Pool, a.SpikeID, a.Findings); err != nil && !errors.Is(err, store.ErrSpikeNotRunning) {
			return err
		}
	}
	if err := s.EndSpike(ctx, a.SpikeID, a.How, a.Note); err != nil {
		return err
	}
	if a.Exhausted {
		s.cancelSpikeRun(ctx, a.DispatchID)
	}
	return nil
}

// cancelSpikeRun closes a failed run for good. A run that is already closed is
// not an error.
func (s *Server) cancelSpikeRun(ctx context.Context, dispatchID uuid.UUID) {
	if dispatchID == uuid.Nil {
		return
	}
	_ = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.MarkDispatchCancelled(ctx, tx, dispatchID, "subutai")
	})
}

// EndSpike ends a spike's run (FR-6.2): the leak check, then the findings and
// the move to ended in one transaction, then the commit and the discarded
// worktree. how is concluded, budget, turn_limit or failed; note is a failure's
// last error. The findings are built from the spike's draft. Every step is
// safe to run again: a spike that is no longer running skips to the last two,
// and one that already has findings isn't given a second set.
func (s *Server) EndSpike(ctx context.Context, spikeID uuid.UUID, how, note string) error {
	if err := checkSpikeHow(how); err != nil {
		return err
	}
	sp, err := store.GetSpike(ctx, s.Store.Pool, spikeID)
	if err != nil {
		return err
	}
	return s.withSpikeEndLocks(sp, func() error { return s.endSpikeLocked(ctx, spikeID, how, note) })
}

func checkSpikeHow(how string) error {
	switch how {
	case store.SpikeConcluded, store.SpikeBudget, store.SpikeTurnLimit, store.SpikeFailed:
		return nil
	}
	return fmt.Errorf("a run ends as concluded, budget, turn_limit or failed, not %q", how)
}

// withSpikeEndLocks runs fn holding the spike's worktree's lock, then
// spikeEndMu: the first two of SPEC-021 SD-27's three locks, in its order
// (row locks come inside fn). One ending at a time, so the heartbeat and the
// rules can't both write the findings; and the worktree's lock, so a discard
// can't interleave with a claim's remake. A spike with no worktree path takes
// no working-copy lock. Neither lock is re-entrant: fn must not call
// withSpikeEndLocks, withWorkingCopy on the same path, or take spikeEndMu.
func (s *Server) withSpikeEndLocks(sp *store.Spike, fn func() error) error {
	path := ""
	if sp != nil && sp.WorktreePath != "" {
		path = s.worktreeAbs(sp.WorktreePath)
	}
	return s.withWorkingCopy(path, func() error {
		s.spikeEndMu.Lock()
		defer s.spikeEndMu.Unlock()
		return fn()
	})
}

// endSpikeLocked is EndSpike's body, for a caller that already holds both
// locks (withSpikeEndLocks): a submit that has ended the claim ends the spike
// through it.
func (s *Server) endSpikeLocked(ctx context.Context, spikeID uuid.UUID, how, note string) error {
	if err := checkSpikeHow(how); err != nil {
		return err
	}
	sp, err := store.GetSpike(ctx, s.Store.Pool, spikeID)
	if err != nil {
		return err
	}
	switch sp.State {
	case store.SpikeIdea:
		return errors.New("This spike hasn't started, so it has no run to end.")
	case store.SpikeRunning:
		if err := s.recordSpikeEnd(ctx, sp, how, note); err != nil {
			return err
		}
		if sp, err = store.GetSpike(ctx, s.Store.Pool, spikeID); err != nil {
			return err
		}
		s.notifySpikeChanged(sp)
	}
	return s.finishSpikeEnding(ctx, sp)
}

// recordSpikeEnd is steps 1 to 3 of FR-6.2.
func (s *Server) recordSpikeEnd(ctx context.Context, sp *store.Spike, how, note string) error {
	kept, err := s.spikeKept(ctx, sp)
	if err != nil {
		return err
	}
	turnCap := s.spikeTurnCap()
	undo := func() {}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		changed, err := store.EndSpikeState(ctx, tx, sp.ID, how, note)
		if err != nil || !changed {
			return err
		}
		// Read the spike as it now stands, so the tokens in the findings are
		// the ones the audit row records.
		cur, err := store.GetSpike(ctx, tx, sp.ID)
		if err != nil {
			return err
		}
		if _, err := store.CurrentDocForOwner(ctx, tx, "findings", "spike", sp.ID); err == nil {
			return nil // an earlier call wrote them
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		var budget int64
		if cur.TokenBudget != nil {
			budget = *cur.TokenBudget
		}
		_, u, err := s.writeFindings(ctx, tx, cur, cur.Draft, findingsEnd{
			How: how, Note: note, Used: cur.TokensUsed, Budget: budget, TurnCap: turnCap, Refs: kept.Refs, CheckFailed: kept.CouldntCheck,
		})
		undo = u
		return err
	})
	if err != nil {
		undo()
		return err
	}
	return nil
}

// spikeTurnCap is the turn limit the spike runner has, for the findings'
// sentence about stopping at it. A project whose config can't be read has none
// to name.
func (s *Server) spikeTurnCap() int {
	cfg, err := s.freshConfig()
	if err != nil {
		return 0
	}
	role, _, err := s.roleAndSkill(cfg.Assignments["run-spike"])
	if err != nil {
		return cfg.Dispatch.TurnCap
	}
	return turnCapFor(cfg, role)
}

// turnCapFor is a run's turn limit: the role's own, else the project's.
func turnCapFor(cfg *config.Config, role *config.Role) int {
	if role.Limits != nil && role.Limits.TurnCap > 0 {
		return role.Limits.TurnCap
	}
	return cfg.Dispatch.TurnCap
}

// finishSpikeEnding is steps 4 and 5: commit the findings, discard the
// worktree. It runs for a spike that has ended, and again for any that an
// earlier call left half done.
func (s *Server) finishSpikeEnding(ctx context.Context, sp *store.Spike) error {
	changed := false
	if doc, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "findings", "spike", sp.ID); err == nil {
		changed = s.commitSpikeFindings(sp, doc)
	}
	if sp.WorktreePath != "" && sp.WorktreeRemovedAt == nil {
		if err := s.discardSpikeWorktree(ctx, sp); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		// The page shows the commit and the discard; a sweep that found
		// nothing to do says nothing.
		s.notifySpikeChanged(sp)
	}
	return nil
}

// commitSpikeFindings commits the findings file in the main checkout, once:
// when it has never been committed. What a person edits later is theirs to
// commit, and isn't swept up here.
func (s *Server) commitSpikeFindings(sp *store.Spike, doc *store.Document) bool {
	// In HEAD, not in the index: a commit that failed after the add leaves the
	// file staged but still never committed.
	out, err := gitIn(s.RepoRoot, "ls-tree", "--name-only", "HEAD", "--", doc.Path)
	if err != nil || strings.TrimSpace(out) != "" {
		return false
	}
	msg := fmt.Sprintf("%s: findings (%s)", sp.PublicID, store.SpikeEndingOf(sp.EndedHow).Commit)
	if err := s.commitPath(doc.Path, msg); err != nil {
		s.Log.Warn("could not commit a spike's findings", "spike", sp.PublicID, "err", err)
		return false
	}
	return true
}

// ---- The worktree ----

// spikeWorktreeOK says a path is a spike's own directory, so nothing else is
// ever removed by the discard.
func spikeWorktreeOK(abs string) bool {
	return strings.HasPrefix(filepath.Base(abs), "spk-") && filepath.Base(filepath.Dir(abs)) == "worktrees"
}

// removeSpikeDir removes a worktree directory and what git remembers of it:
// git's own removal, or the directory removed and git told to forget it.
func (s *Server) removeSpikeDir(abs string) error {
	if !spikeWorktreeOK(abs) {
		return fmt.Errorf("%s is not a spike's working copy", abs)
	}
	if _, err := gitIn(s.RepoRoot, "worktree", "remove", "--force", abs); err != nil {
		s.Log.Info("git worktree remove didn't take; removing the directory", "path", abs, "err", err)
	}
	if _, err := os.Stat(abs); err == nil {
		if err := os.RemoveAll(abs); err != nil {
			return err
		}
	}
	_, _ = gitIn(s.RepoRoot, "worktree", "prune")
	return nil
}

// discardSpikeWorktree is step 5 of FR-6.2: the worktree goes, with whatever
// the agent left in it, and the row says when.
func (s *Server) discardSpikeWorktree(ctx context.Context, sp *store.Spike) error {
	if err := s.removeSpikeDir(s.worktreeAbs(sp.WorktreePath)); err != nil {
		return err
	}
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.MarkSpikeWorktreeRemoved(ctx, tx, sp.ID)
		return err
	})
}

// spikeHadWorkingCopy says whether a spike has had a working copy whose
// contents might have kept code (SPEC-021 FR-13.3): an agent spike that has
// used tokens (the planner makes its worktree before the first call), and every
// started chat or person spike, whose worktree is made at the start. It decides
// whether a remake must check what was kept first, and whether the leak check
// failing to find the directory is a failure to check.
func spikeHadWorkingCopy(sp *store.Spike) bool {
	switch sp.Executor {
	case store.ExecutorChat, store.ExecutorPerson:
		return sp.StartedAt != nil
	}
	return sp.TokensUsed > 0
}

// ensureSpikeWorktree makes sure the spike's worktree exists before the run's
// first call (FR-4.2): if the path has no .git, anything there is removed and
// a detached worktree is made at the base commit. A detached worktree has no
// branch, so there is nothing to merge (SD-2). A spike that had a working copy
// (spikeHadWorkingCopy) would have the remake prune the reflog the leak check
// reads, so what it kept is checked and recorded first; the end of the run then
// finds that record and doesn't raise it again. The caller holds the worktree's
// lock (SD-27) when more than one path can reach it; this takes spikeEndMu for
// the check, so the caller must not hold that.
func (s *Server) ensureSpikeWorktree(ctx context.Context, sp *store.Spike) (string, error) {
	return s.makeSpikeWorktree(ctx, sp, true)
}

// makeSpikeWorktree is ensureSpikeWorktree, with the check of what an earlier
// working copy kept made only when check is set. The start passes false: it
// makes the first working copy, so there is nothing to check.
func (s *Server) makeSpikeWorktree(ctx context.Context, sp *store.Spike, check bool) (string, error) {
	abs := s.worktreeAbs(sp.WorktreePath)
	if !spikeWorktreeOK(abs) {
		return "", fmt.Errorf("the spike's working copy path %q isn't one Subutai makes", sp.WorktreePath)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
		return abs, nil
	}
	if check && spikeHadWorkingCopy(sp) {
		s.spikeEndMu.Lock()
		_, err := s.spikeKept(ctx, sp)
		s.spikeEndMu.Unlock()
		if err != nil {
			return "", err
		}
	}
	if err := os.RemoveAll(abs); err != nil {
		return "", err
	}
	// Forget a worktree git still lists at the path, or `add` refuses.
	_, _ = gitIn(s.RepoRoot, "worktree", "prune")
	if err := os.MkdirAll(s.worktreesRoot(), 0o755); err != nil {
		return "", err
	}
	if _, err := gitIn(s.RepoRoot, "worktree", "add", "--detach", abs, sp.BaseCommit); err != nil {
		return "", err
	}
	return abs, nil
}

// ---- Reconciliation (FR-6.4) ----

// ReconcileSpikes finishes what an earlier call, or a restart, left. It runs
// at boot and on every heartbeat:
//   - a running spike whose run has succeeded, been cancelled, or failed with
//     no attempts left is ended, how read from the run's outcome or error;
//   - an ended spike whose findings were never committed, or any spike with a
//     worktree that was never discarded, has the last steps run again;
//   - a spike directory that no running spike names is removed.
func (s *Server) ReconcileSpikes(ctx context.Context) {
	// The directories are listed first: a spike started after this look has a
	// directory that wasn't in it, so the sweep can't remove a new one.
	dirs := s.spikeDirs()
	cfg, err := s.freshConfig()
	if err != nil {
		s.Log.Error("spike reconciliation: config", "err", err)
		return
	}
	runs, err := store.SpikesToReconcile(ctx, s.Store.Pool)
	if err != nil {
		s.Log.Error("spike reconciliation", "err", err)
		return
	}
	running := map[string]bool{}
	for _, r := range runs {
		running[filepath.Base(r.Spike.WorktreePath)] = true
		a, ok := spikeRunEnding(r, cfg.Dispatch.MaxAttempts)
		if !ok {
			continue
		}
		if err := s.endSpikeAction(ctx, a); err != nil {
			s.Log.Error("spike reconciliation: end", "spike", r.Spike.PublicID, "err", err)
		}
	}

	seen := map[uuid.UUID]bool{}
	finish := func(list []store.Spike) {
		for i := range list {
			sp := list[i]
			if seen[sp.ID] {
				continue
			}
			seen[sp.ID] = true
			// The worktree's lock, then spikeEndMu (SD-27).
			err := s.withSpikeEndLocks(&sp, func() error { return s.finishSpikeEnding(ctx, &sp) })
			if err != nil {
				s.Log.Error("spike reconciliation: finish", "spike", sp.PublicID, "err", err)
			}
		}
	}
	if ended, err := store.ListSpikes(ctx, s.Store.Pool, store.SpikeFilter{State: store.SpikeEnded}); err == nil {
		finish(ended)
	}
	if live, err := store.SpikesWithLiveWorktree(ctx, s.Store.Pool); err == nil {
		finish(live)
	}

	for _, name := range dirs {
		if running[name] {
			continue
		}
		if err := s.removeSpikeDir(filepath.Join(s.worktreesRoot(), name)); err != nil {
			s.Log.Error("spike reconciliation: leftover directory", "dir", name, "err", err)
			continue
		}
		s.Log.Info("a leftover spike working copy was removed", "dir", name)
	}
}

// spikeDirs names the spike directories under the worktrees folder.
func (s *Server) spikeDirs() []string {
	entries, err := os.ReadDir(s.worktreesRoot())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "spk-") {
			out = append(out, e.Name())
		}
	}
	return out
}

// spikeRunEnding says whether a running spike's run is over, and how, as the
// rules' action. A run still queued or running, or failed with attempts left,
// is not over.
func spikeRunEnding(r store.SpikeRun, maxAttempts int) (rules.EndSpike, bool) {
	a := rules.EndSpike{SpikeID: r.Spike.ID}
	if r.DispatchID == nil {
		a.How, a.Note = store.SpikeFailed, "The run was lost."
		return a, true
	}
	a.DispatchID = *r.DispatchID
	switch r.DispatchState {
	case "succeeded":
		how, findings, err := rules.SpikeEnding(r.Outcome)
		if err != nil {
			a.How, a.Note = store.SpikeFailed, "The run's outcome couldn't be read."
			return a, true
		}
		a.How, a.Findings = how, findings
		return a, true
	case "cancelled":
		a.How, a.Note = store.SpikeFailed, "The run was cancelled."
		return a, true
	case "failed":
		if r.Attempt >= maxAttempts {
			a.How, a.Note, a.Exhausted = store.SpikeFailed, r.Error, true
			return a, true
		}
	}
	return a, false
}
