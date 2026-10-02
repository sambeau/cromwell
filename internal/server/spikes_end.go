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
	"subutai/internal/lifecycle"
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
// worktree. how is concluded, budget, turn_limit, failed or time_box; budget
// and turn_limit are an agent spike's, and time_box a chat or person spike's
// (FR-15.2). note is a failure's last error. The findings are built from the
// spike's draft. Every step is safe to run again: a spike that is no longer
// running skips to the last two, and one that already has findings isn't
// given a second set.
func (s *Server) EndSpike(ctx context.Context, spikeID uuid.UUID, how, note string) error {
	sp, err := store.GetSpike(ctx, s.Store.Pool, spikeID)
	if err != nil {
		return err
	}
	// A cheap refusal before the locks are taken; endSpikeLocked checks again
	// under them, against the spike as the locks left it.
	if err := checkSpikeEnding(sp, how); err != nil {
		return err
	}
	return s.withSpikeEndLocks(sp, func() error { return s.endSpikeLocked(ctx, spikeID, how, note) })
}

// checkSpikeEnding refuses a way of ending that isn't one, or that the spike's
// executor can't have (FR-15.2): the time box is a chat or person spike's, the
// budget and the turn limit an agent spike's. A spike with no executor hasn't
// started, and has no run to end.
func checkSpikeEnding(sp *store.Spike, how string) error {
	switch how {
	case store.SpikeConcluded, store.SpikeBudget, store.SpikeTurnLimit, store.SpikeFailed, store.SpikeTimeBox:
	default:
		return fmt.Errorf("a run ends as concluded, budget, turn_limit, failed or time_box, not %q", how)
	}
	switch {
	case sp.Executor == "":
		return nil
	case how == store.SpikeTimeBox && !unmeasuredExecutor(sp.Executor):
		return errors.New("A spike run by the spike runner has a token budget, not a time box, so it can't end at one.")
	case (how == store.SpikeBudget || how == store.SpikeTurnLimit) && unmeasuredExecutor(sp.Executor):
		return errors.New("A spike run in chat or by hand has a time box, not a token budget, so it can't end at one.")
	}
	return nil
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
	sp, err := store.GetSpike(ctx, s.Store.Pool, spikeID)
	if err != nil {
		return err
	}
	if err := checkSpikeEnding(sp, how); err != nil {
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
		// The spike's row first, then its claim's (SD-27): what the ending
		// reads of the claim can't change under it.
		cur, err := store.LockSpike(ctx, tx, sp.ID)
		if err != nil {
			return err
		}
		if cur.State != store.SpikeRunning {
			return nil // another call ended it
		}
		end := findingsEnd{How: how, Note: note, Executor: cur.Executor}
		if unmeasuredExecutor(cur.Executor) {
			if end.How, end.Actor, err = s.settleSpikeClaim(ctx, tx, cur, how); err != nil {
				return err
			}
			if cur.TimeBoxHours != nil {
				end.TimeBoxHours = *cur.TimeBoxHours
			}
		}
		changed, err := store.EndSpikeState(ctx, tx, sp.ID, end.How, note)
		if err != nil || !changed {
			return err
		}
		// Read the spike as it now stands, so the tokens in the findings are
		// the ones the audit row records.
		cur, err = store.GetSpike(ctx, tx, sp.ID)
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
		end.Used, end.Budget, end.TurnCap = cur.TokensUsed, budget, turnCap
		end.Refs, end.CheckFailed = kept.Refs, kept.CouldntCheck
		_, u, err := s.writeFindings(ctx, tx, cur, cur.Draft, end)
		undo = u
		return err
	})
	if err != nil {
		undo()
		return err
	}
	return nil
}

// settleSpikeClaim is what FR-15.2 does with a chat or person spike's claim
// as the spike ends, in the ending's transaction with the spike's row locked.
// If any claim of the spike ended done, a submit got there first, so the spike
// ends concluded whatever was asked (a newer claim can't undo it). A claim that
// hasn't ended is ended with the machine's expire (SD-21), which also
// withdraws its claim-stale. It returns how the spike ends, and who ran it,
// for the findings to name.
func (s *Server) settleSpikeClaim(ctx context.Context, tx pgx.Tx, sp *store.Spike, how string) (end, who string, err error) {
	end = how
	submitted, err := spikeSubmitted(ctx, tx, sp)
	if err != nil {
		return "", "", err
	}
	if submitted {
		end = store.SpikeConcluded
	}
	cur, err := store.LockCurrentClaimFor(ctx, tx, "spike", sp.ID)
	switch {
	case err == nil:
		if err := store.TransitionClaim(ctx, tx, cur, lifecycle.ClaimEventExpire, "subutai", "", map[string]any{"how": end}); err != nil {
			return "", "", err
		}
	case !errors.Is(err, store.ErrNotFound):
		return "", "", err
	}
	facts, err := readSpikeRunFacts(ctx, tx, sp)
	if err != nil {
		return "", "", err
	}
	_, who = spikeRanBy(sp, facts.Execs, facts.Claim)
	return end, who, nil
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
// used tokens (the planner makes its worktree before the first call), and a
// chat or person spike whose worktree was made, which is recorded when it is
// (a start whose make failed has none until its first claim makes it). It
// decides whether a remake must check what was kept first, and whether the
// leak check failing to find the directory is a failure to check.
func (s *Server) spikeHadWorkingCopy(ctx context.Context, sp *store.Spike) (bool, error) {
	if unmeasuredExecutor(sp.Executor) {
		return store.SpikeWorktreeMade(ctx, s.Store.Pool, sp.ID)
	}
	return sp.TokensUsed > 0, nil
}

// recordSpikeWorktreeMade audits that a chat or person spike's worktree
// exists, once; an agent spike's has no such record (spikeHadWorkingCopy reads
// its tokens).
func (s *Server) recordSpikeWorktreeMade(ctx context.Context, sp *store.Spike) error {
	if !unmeasuredExecutor(sp.Executor) {
		return nil
	}
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.RecordSpikeWorktreeMade(ctx, tx, sp.ID)
		return err
	})
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
		return abs, s.recordSpikeWorktreeMade(ctx, sp)
	}
	if check {
		had, err := s.spikeHadWorkingCopy(ctx, sp)
		if err != nil {
			return "", err
		}
		if had {
			s.spikeEndMu.Lock()
			_, err := s.spikeKept(ctx, sp)
			s.spikeEndMu.Unlock()
			if err != nil {
				return "", err
			}
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
	return abs, s.recordSpikeWorktreeMade(ctx, sp)
}

// ---- Reconciliation (FR-6.4) ----

// ReconcileSpikes finishes what an earlier call, or a restart, left. It runs
// at boot and on every heartbeat:
//   - a running spike whose run has succeeded, been cancelled, or failed with
//     no attempts left is ended, how read from the run's outcome or error;
//   - a running chat or person spike has no run, so it is never "lost" and its
//     directory is kept; it is ended concluded when its latest claim ended
//     done, else time_box when its deadline has passed (FR-12.4);
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
	past := map[uuid.UUID]bool{}
	if list, err := store.SpikesPastDeadline(ctx, s.Store.Pool); err != nil {
		s.Log.Error("spike reconciliation: deadlines", "err", err)
	} else {
		for _, sp := range list {
			past[sp.ID] = true
		}
	}
	running := map[string]bool{}
	for _, r := range runs {
		running[filepath.Base(r.Spike.WorktreePath)] = true
		if unmeasuredExecutor(r.Spike.Executor) {
			// No run to lose: its directory is kept, and it ends only at a
			// submit that stopped short or at its deadline (FR-12.4).
			if how, ok := s.timeBoxedEnding(ctx, &r.Spike, past[r.Spike.ID]); ok {
				if err := s.EndSpike(ctx, r.Spike.ID, how, ""); err != nil {
					s.Log.Error("spike reconciliation: end", "spike", r.Spike.PublicID, "err", err)
				}
			}
			continue
		}
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

// timeBoxedEnding says whether a running chat or person spike is over, and how,
// in FR-15.2's order: a claim of it ended done (a submit that stopped before
// its ending), which is concluded; then its deadline has passed, which is
// time_box.
func (s *Server) timeBoxedEnding(ctx context.Context, sp *store.Spike, pastDeadline bool) (string, bool) {
	submitted, err := spikeSubmitted(ctx, s.Store.Pool, sp)
	if err != nil {
		s.Log.Error("spike reconciliation: claim", "spike", sp.PublicID, "err", err)
		return "", false
	}
	if submitted {
		return store.SpikeConcluded, true
	}
	if pastDeadline {
		return store.SpikeTimeBox, true
	}
	return "", false
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
