package server

// Ending a spike's run (SPEC-021 FR-6): the leak check, the findings, the
// state, the commit and the discarded worktree, each step safe to run again so
// that a restart at any point leaves nothing stuck, and the reconciliation
// that finishes whatever an earlier call left (FR-6.4).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
	switch how {
	case store.SpikeConcluded, store.SpikeBudget, store.SpikeTurnLimit, store.SpikeFailed:
	default:
		return fmt.Errorf("a run ends as concluded, budget, turn_limit or failed, not %q", how)
	}
	// One ending at a time, so the heartbeat and the rules can't both write
	// the findings.
	s.spikeEndMu.Lock()
	defer s.spikeEndMu.Unlock()

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
	}
	return s.finishSpikeEnding(ctx, sp)
}

// recordSpikeEnd is steps 1 to 3 of FR-6.2.
func (s *Server) recordSpikeEnd(ctx context.Context, sp *store.Spike, how, note string) error {
	refs := s.spikeKeptRefs(ctx, sp)
	turnCap := s.spikeTurnCap()
	undo := func() {}
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
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
			How: how, Note: note, Used: cur.TokensUsed, Budget: budget, TurnCap: turnCap, Refs: refs,
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
// sentence about stopping at it.
func (s *Server) spikeTurnCap() int {
	cfg, err := s.freshConfig()
	if err != nil {
		return 0
	}
	if name, ok := cfg.Assignments["run-spike"]; ok {
		if role, err := s.roleOnly(name); err == nil && role > 0 {
			return role
		}
	}
	return cfg.Dispatch.TurnCap
}

// roleOnly is a role's own turn cap, or 0 when it sets none.
func (s *Server) roleOnly(name string) (int, error) {
	role, _, err := s.roleAndSkill(name)
	if err != nil {
		return 0, err
	}
	if role.Limits != nil {
		return role.Limits.TurnCap, nil
	}
	return 0, nil
}

// finishSpikeEnding is steps 4 and 5: commit the findings, discard the
// worktree. It runs for a spike that has ended, and again for any that an
// earlier call left half done.
func (s *Server) finishSpikeEnding(ctx context.Context, sp *store.Spike) error {
	if doc, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "findings", "spike", sp.ID); err == nil {
		s.commitSpikeFindings(sp, doc)
	}
	if sp.WorktreePath != "" && sp.WorktreeRemovedAt == nil {
		if err := s.discardSpikeWorktree(ctx, sp); err != nil {
			return err
		}
	}
	s.notifySpikeChanged(sp)
	return nil
}

// spikeEndReason says how a run ended, for the findings' commit message.
func spikeEndReason(how string) string {
	switch how {
	case store.SpikeConcluded:
		return "concluded"
	case store.SpikeBudget:
		return "stopped at the budget"
	case store.SpikeTurnLimit:
		return "stopped at the turn limit"
	}
	return "the run failed"
}

// commitSpikeFindings commits the findings file in the main checkout, once:
// when it has never been committed. What a person edits later is theirs to
// commit, and isn't swept up here.
func (s *Server) commitSpikeFindings(sp *store.Spike, doc *store.Document) {
	out, err := gitIn(s.RepoRoot, "ls-files", "--", doc.Path)
	if err != nil || strings.TrimSpace(out) != "" {
		return
	}
	s.commitDocument(doc.Path, fmt.Sprintf("%s: findings (%s)", sp.PublicID, spikeEndReason(sp.EndedHow)))
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

// ensureSpikeWorktree makes sure the spike's worktree exists before the run's
// first call (FR-4.2): if the path has no .git, anything there is removed and
// a detached worktree is made at the base commit. A detached worktree has no
// branch, so there is nothing to merge (SD-2).
func (s *Server) ensureSpikeWorktree(sp *store.Spike) (string, error) {
	abs := s.worktreeAbs(sp.WorktreePath)
	if !spikeWorktreeOK(abs) {
		return "", fmt.Errorf("the spike's working copy path %q isn't one Subutai makes", sp.WorktreePath)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
		return abs, nil
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

// ---- The leak check (FR-6.3) ----

// spikeKeptRefs finds the refs that hold a commit made in the spike's worktree.
// When the worktree's HEAD is where it started there are none. The refs found
// are audited once, with a checkpoint that asks a person; a later call, after
// the worktree is gone, reads them back from the audit row, so the findings
// still say so.
func (s *Server) spikeKeptRefs(ctx context.Context, sp *store.Spike) []string {
	if refs, ok := s.recordedKeptRefs(ctx, sp.ID); ok {
		return refs
	}
	refs := s.leakedRefs(sp)
	if len(refs) == 0 {
		return nil
	}
	var cp *store.Checkpoint
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.RecordSpikeCodeKept(ctx, tx, sp.ID, refs); err != nil {
			return err
		}
		quoted := make([]string, len(refs))
		for i, r := range refs {
			quoted[i] = "`" + r + "`"
		}
		var err error
		cp, err = store.CreateCheckpoint(ctx, tx, "spike-code-kept", "spike", sp.ID,
			fmt.Sprintf("Code from %s was kept on %s, outside its working copy. A spike's code is never merged. "+
				"Delete the branch, or keep it knowing it won't be built from.", sp.PublicID, joinWords(quoted)),
			map[string]any{"spike_id": sp.ID.String(), "refs": refs})
		return err
	})
	if err != nil {
		s.Log.Error("spike leak check: record", "spike", sp.PublicID, "err", err)
		return refs
	}
	s.notifyCheckpointRaised(cp)
	return refs
}

// recordedKeptRefs reads the refs an earlier call audited.
func (s *Server) recordedKeptRefs(ctx context.Context, id uuid.UUID) ([]string, bool) {
	var raw []byte
	err := s.Store.Pool.QueryRow(ctx, `SELECT payload FROM audit_events
		WHERE kind = 'spike.code_kept' AND ref_id = $1 ORDER BY occurred_at DESC LIMIT 1`, id).Scan(&raw)
	if err != nil {
		return nil, false
	}
	var p struct {
		Refs []string `json:"refs"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil, false
	}
	return p.Refs, true
}

// leakedRefs asks git: if the worktree's HEAD isn't the base commit, the
// newest commit made there, and every ref that contains it.
func (s *Server) leakedRefs(sp *store.Spike) []string {
	if sp.WorktreePath == "" || sp.BaseCommit == "" {
		return nil
	}
	abs := s.worktreeAbs(sp.WorktreePath)
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return nil // no worktree, nothing to check
	}
	head, err := gitIn(abs, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(head) == sp.BaseCommit {
		return nil
	}
	out, err := gitIn(abs, "rev-list", "HEAD", "--not", sp.BaseCommit)
	if err != nil {
		s.Log.Warn("spike leak check: rev-list", "spike", sp.PublicID, "err", err)
		return nil
	}
	commits := strings.Fields(out)
	if len(commits) == 0 {
		return nil
	}
	out, err = gitIn(s.RepoRoot, "for-each-ref", "--contains", commits[0], "--format=%(refname:short)")
	if err != nil {
		s.Log.Warn("spike leak check: for-each-ref", "spike", sp.PublicID, "err", err)
		return nil
	}
	return strings.Fields(out)
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
			s.spikeEndMu.Lock()
			err := s.finishSpikeEnding(ctx, &sp)
			s.spikeEndMu.Unlock()
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

	removed := false
	for _, name := range dirs {
		if running[name] {
			continue
		}
		if err := s.removeSpikeDir(filepath.Join(s.worktreesRoot(), name)); err != nil {
			s.Log.Error("spike reconciliation: leftover directory", "dir", name, "err", err)
			continue
		}
		removed = true
		s.Log.Info("a leftover spike working copy was removed", "dir", name)
	}
	if removed {
		_, _ = gitIn(s.RepoRoot, "worktree", "prune")
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
