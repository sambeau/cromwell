package server

// The branch watch (SPEC-020 FR-5.6 to FR-5.9, SD-10, SD-11): commits made on
// a feature's branch by someone other than Subutai, while no task is claimed,
// raise an `unclaimed-commit` notice on the feature. Subutai's own commits are
// known by hash (worktree_commits), commits already on the main branch are
// left out, and a branch that was rewritten past a commit the watch had seen
// is worded as such. Uncommitted edits are not watched (§6).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// watchedCommit is a commit the watch is reporting.
type watchedCommit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Subject string `json:"subject"`
	At      string `json:"at"`
}

// watchedRewrite is a rewrite of the branch: the head the watch had seen, and
// the head it found.
type watchedRewrite struct {
	OldHead string `json:"old_head"`
	NewHead string `json:"new_head"`
}

// unclaimedContext is the context of an unclaimed-commit checkpoint.
type unclaimedContext struct {
	Commits  []watchedCommit  `json:"commits,omitempty"`
	Rewrites []watchedRewrite `json:"rewrites,omitempty"`
}

// mainRef is the branch the repository's root has checked out: the one
// mergeBranch merges into, and so the one that "commits already on main"
// means. A detached root falls back to HEAD.
func (s *Server) mainRef() string {
	out, err := gitIn(s.RepoRoot, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || strings.TrimSpace(out) == "" {
		return "HEAD"
	}
	return strings.TrimSpace(out)
}

// watchBranchLogged runs the watch before a commit the server is about to
// make in a working copy whose lock the caller holds (FR-5.7). A failure is
// logged and doesn't stop the commit: the watch looks from the head it last
// accounted for, so nothing is lost by trying again.
func (s *Server) watchBranchLogged(ctx context.Context, wt *store.Worktree) {
	if err := s.watchBranchLocked(ctx, wt); err != nil {
		s.Log.Warn("branch watch", "feature", wt.FeatureID, "err", err)
	}
}

// WatchBranch runs the watch for a worktree, taking the working copy's lock.
func (s *Server) WatchBranch(ctx context.Context, wt *store.Worktree) error {
	return s.withWorkingCopy(s.worktreeAbs(wt.Path), func() error { return s.watchBranchLocked(ctx, wt) })
}

// BranchWatchSweep runs the watch for every live worktree of a feature being
// built or verified (FR-5.6). It is a heartbeat duty, and the post-commit
// handler runs it too.
func (s *Server) BranchWatchSweep(ctx context.Context) {
	worktrees, err := s.Store.LiveWorktrees(ctx)
	if err != nil {
		s.Log.Error("branch watch", "err", err)
		return
	}
	for i := range worktrees {
		if ctx.Err() != nil {
			return
		}
		if err := s.WatchBranch(ctx, &worktrees[i]); err != nil {
			s.Log.Warn("branch watch", "feature", worktrees[i].FeatureID, "err", err)
		}
	}
}

// watchBranchLocked is the watch itself, for a caller that holds the working
// copy's lock (the lock isn't reentrant).
func (s *Server) watchBranchLocked(ctx context.Context, wt *store.Worktree) error {
	// Read the row again: the caller's copy may be older than the last watch.
	cur, err := store.LiveWorktreeForFeature(ctx, s.Store.Pool, wt.FeatureID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	f, err := store.GetFeature(ctx, s.Store.Pool, cur.FeatureID)
	if err != nil {
		return err
	}
	if f.State != lifecycle.FeatActive && f.State != lifecycle.FeatReview {
		return nil
	}
	abs := s.worktreeAbs(cur.Path)
	if _, err := os.Stat(abs); err != nil {
		return nil // the working copy is missing; the Inbox has its own question
	}
	head, err := gitIn(abs, "rev-parse", "--verify", "--quiet", "refs/heads/"+cur.Branch+"^{commit}")
	if err != nil {
		return nil // the branch isn't there yet
	}
	head = strings.TrimSpace(head)
	main := s.mainRef()

	// Step 1: start from the fork point.
	watched := cur.WatchedHead
	if watched == "" {
		out, err := gitIn(abs, "merge-base", main, head)
		if err != nil {
			return fmt.Errorf("finding where the branch left %s: %w", main, err)
		}
		watched = strings.TrimSpace(out)
	}
	if watched == head {
		if cur.WatchedHead == "" {
			return s.Store.WithTx(ctx, func(tx pgx.Tx) error { return store.SetWatchedHead(ctx, tx, cur.ID, head) })
		}
		return nil
	}

	var found unclaimedContext
	// Step 2: a branch that no longer holds what was seen was rewritten.
	if !commitExists(abs, watched) || !isAncestor(abs, watched, head) {
		found.Rewrites = append(found.Rewrites, watchedRewrite{OldHead: watched, NewHead: head})
	} else {
		// Steps 3 and 4: what is new, less Subutai's own and main's. A merge
		// commit made to bring main in is no work of anyone's, so it is left
		// out with main's own commits.
		out, err := gitIn(abs, "log", "--reverse", "--no-merges", "--format=%H%x1f%an%x1f%s%x1f%aI",
			watched+".."+head, "--not", main)
		if err != nil {
			return fmt.Errorf("listing the branch's new commits: %w", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			parts := strings.Split(line, "\x1f")
			if len(parts) != 4 {
				continue
			}
			ours, err := store.IsWorktreeCommit(ctx, s.Store.Pool, cur.ID, parts[0])
			if err != nil {
				return err
			}
			if !ours {
				found.Commits = append(found.Commits, watchedCommit{Hash: parts[0], Author: parts[1], Subject: parts[2], At: parts[3]})
			}
		}
	}

	// Step 5: a commit made while a claim is open is that claim's work (SD-10);
	// a rewrite is raised whatever the claim.
	if len(found.Commits) > 0 {
		if _, err := store.OpenClaimForFeature(ctx, s.Store.Pool, f.ID); err == nil {
			found.Commits = nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}

	// Step 6: raise or extend the notice, and move the watch to the head, in
	// one transaction.
	var raised *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		raised = nil
		if len(found.Commits) > 0 || len(found.Rewrites) > 0 {
			var err error
			if raised, err = s.raiseUnclaimed(ctx, tx, f, found); err != nil {
				return err
			}
		}
		return store.SetWatchedHead(ctx, tx, cur.ID, head)
	})
	if err != nil {
		return err
	}
	s.notifyCheckpointRaised(raised)
	return nil
}

// raiseUnclaimed raises the feature's unclaimed-commit checkpoint, or adds
// what was found to the one that is pending: checkpoints are one per kind and
// item while pending (FR-5.8). It returns the checkpoint only when it is new.
func (s *Server) raiseUnclaimed(ctx context.Context, tx pgx.Tx, f *store.Feature, found unclaimedContext) (*store.Checkpoint, error) {
	cp, err := store.CreateCheckpoint(ctx, tx, "unclaimed-commit", "feature", f.ID,
		unclaimedQuestion(f.PublicID, found), unclaimedMap(found))
	if err != nil || cp != nil {
		return cp, err
	}
	pending, err := store.PendingCheckpointFor(ctx, tx, "unclaimed-commit", "feature", f.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil // answered between the two statements; the next watch sees only what is new
	}
	if err != nil {
		return nil, err
	}
	var all unclaimedContext
	_ = json.Unmarshal(pending.Context, &all)
	all.Commits = append(all.Commits, found.Commits...)
	all.Rewrites = append(all.Rewrites, found.Rewrites...)
	_, err = store.UpdatePendingCheckpoint(ctx, tx, pending.ID, unclaimedQuestion(f.PublicID, all), unclaimedMap(all))
	return nil, err
}

func unclaimedMap(c unclaimedContext) map[string]any {
	out := map[string]any{}
	if len(c.Commits) > 0 {
		out["commits"] = c.Commits
	}
	if len(c.Rewrites) > 0 {
		out["rewrites"] = c.Rewrites
	}
	return out
}

// numberWords are the counts the question spells out; it never opens with a
// numeral (FR-5.8).
var numberWords = []string{"No", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten",
	"Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen", "Twenty"}

const unclaimedTail = "Work done outside a claim may have missed its code review, and the verifier checks the feature against its spec, not each line. Next time, claim the task first, or ask the chat agent to."

// maxListedCommits is how many commits the question names; the context holds
// them all.
const maxListedCommits = 8

// unclaimedQuestion is FR-5.8's question, in straight quotes.
func unclaimedQuestion(featureID string, c unclaimedContext) string {
	var parts []string
	if n := len(c.Commits); n > 0 {
		count := "Many"
		if n < len(numberWords) {
			count = numberWords[n]
		}
		verb, noun := "were", "commits"
		if n == 1 {
			verb, noun = "was", "commit"
		}
		var lines []string
		for i, cm := range c.Commits {
			if i == maxListedCommits {
				lines = append(lines, fmt.Sprintf("and %d more", n-maxListedCommits))
				break
			}
			lines = append(lines, fmt.Sprintf("%s by %s, \"%s\"", shortHash(cm.Hash), cm.Author, cm.Subject))
		}
		head := fmt.Sprintf("%s %s %s made on %s's branch while nobody had claimed a task", count, noun, verb, featureID)
		if n > len(numberWords)-1 {
			head = fmt.Sprintf("Many commits (%d) were made on %s's branch while nobody had claimed a task", n, featureID)
		}
		parts = append(parts, head+": "+strings.Join(lines, "; ")+".")
	}
	for _, r := range c.Rewrites {
		parts = append(parts, fmt.Sprintf("%s's branch was rewritten: commits Subutai had already seen are no longer on it. Its head was %s and is now %s.",
			featureID, shortHash(r.OldHead), shortHash(r.NewHead)))
	}
	return strings.Join(parts, " ") + " " + unclaimedTail
}

// shortHash is a commit's first seven characters, as git shows it.
func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// commitExists reports whether the commit is in the repository at all: a
// rewrite that has been garbage collected leaves nothing to compare.
func commitExists(dir, hash string) bool {
	cmd := exec.Command("git", "cat-file", "-e", hash+"^{commit}")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// isAncestor is `git merge-base --is-ancestor`: exit status 1 is the answer
// "no", not a failure.
func isAncestor(dir, ancestor, of string) bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, of)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// ---- Where the watch starts (FR-5.9) ----

// startWatching sets watched_head to the branch head, once addWorktree has
// created the branch, so history before the worktree is never reported.
func (s *Server) startWatching(ctx context.Context, wt *store.Worktree) {
	abs := s.worktreeAbs(wt.Path)
	head := headOf(abs)
	if head == "" {
		return
	}
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error { return store.SetWatchedHead(ctx, tx, wt.ID, head) })
	if err != nil {
		s.Log.Warn("setting where the branch watch starts", "feature", wt.FeatureID, "err", err)
	}
}

// BackfillWatchedHeads sets watched_head to the branch head for every live
// worktree that has none, at boot: a migration can't run git. History before
// M13 is never reported (FR-5.9).
func (s *Server) BackfillWatchedHeads(ctx context.Context) error {
	worktrees, err := s.Store.LiveWorktrees(ctx)
	if err != nil {
		return err
	}
	for i := range worktrees {
		wt := worktrees[i]
		if wt.WatchedHead != "" {
			continue
		}
		if _, err := os.Stat(s.worktreeAbs(wt.Path)); err != nil {
			continue
		}
		s.withWorkingCopy(s.worktreeAbs(wt.Path), func() error {
			s.startWatching(ctx, &wt)
			return nil
		})
	}
	return nil
}
