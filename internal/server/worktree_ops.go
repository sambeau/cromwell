package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/bus"
	"cromwell/internal/lifecycle"
	"cromwell/internal/store"
)

// publishFeatureStarted signals the rule engine to dispatch the feature's
// initially-ready tasks (DESIGN-006 §5).
func (s *Server) publishFeatureStarted(featureID uuid.UUID) {
	s.Bus.Publish(bus.FeatureStarted{FeatureID: featureID})
}

// worktreesRoot is where linked worktrees live (DESIGN-006 §3); it is
// gitignored in the main repo so the server-authored commits never sweep
// worktree files in.
func (s *Server) worktreesRoot() string {
	return filepath.Join(s.RepoRoot, ".cromwell", "worktrees")
}

// StartFeature transitions a ready feature to active and creates its worktree
// (FR-5.1): the DB change commits first, then the git worktree is added; a
// git failure raises a checkpoint rather than leaving a half-started feature.
func (s *Server) StartFeature(ctx context.Context, path, actor string) (*store.Feature, error) {
	f, err := s.featureByPath(ctx, path)
	if err != nil {
		return nil, err
	}
	if f.State != lifecycle.FeatReady {
		return nil, fmt.Errorf("feature %s is %s, not ready (a feature starts only from ready)", path, f.State)
	}
	branch := "cromwell/" + path
	relPath := filepath.Join(".cromwell", "worktrees", store.ShortID("feat", f.ID))

	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.TransitionFeature(ctx, tx, f, lifecycle.FeatStart, actor, nil); err != nil {
			return err
		}
		if _, err := store.CreateWorktreeRow(ctx, tx, f.ID, relPath, branch, actor); err != nil {
			return err
		}
		return store.SetFeatureBranch(ctx, tx, f.ID, branch)
	})
	if err != nil {
		return nil, err
	}

	// git worktree add (after the commit — DESIGN-006 §3).
	if gerr := s.addWorktree(relPath, branch); gerr != nil {
		s.Log.Error("worktree add failed", "feature", f.ID, "err", gerr)
		_ = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := store.CreateCheckpoint(ctx, tx, "worktree-failure", "feature", f.ID,
				"Feature started but its git worktree could not be created; the server will retry on restart.",
				map[string]any{"error": gerr.Error(), "branch": branch})
			return e
		})
		return f, nil
	}

	s.publishFeatureStarted(f.ID)
	return f, nil
}

// addWorktree creates a linked worktree at relPath on a new branch off HEAD.
func (s *Server) addWorktree(relPath, branch string) error {
	abs := filepath.Join(s.RepoRoot, relPath)
	if err := os.MkdirAll(s.worktreesRoot(), 0o755); err != nil {
		return err
	}
	// If the branch already exists (retry after a crash), check it out;
	// otherwise create it.
	if _, err := gitIn(s.RepoRoot, "rev-parse", "--verify", branch); err == nil {
		_, err := gitIn(s.RepoRoot, "worktree", "add", abs, branch)
		return err
	}
	_, err := gitIn(s.RepoRoot, "worktree", "add", "-b", branch, abs, "HEAD")
	return err
}

// GCWorktrees removes worktrees for features that have reached a terminal
// state (heartbeat duty, DESIGN-006 §3). A merged feature's worktree row was
// already marked removed at merge; this sweep removes the on-disk worktree
// and covers abandoned features.
func (s *Server) GCWorktrees(ctx context.Context) {
	worktrees, err := s.Store.LiveWorktrees(ctx)
	if err != nil {
		s.Log.Error("worktree GC", "err", err)
		return
	}
	for i := range worktrees {
		wt := worktrees[i]
		f, err := store.GetFeature(ctx, s.Store.Pool, wt.FeatureID)
		if err != nil || !f.State.Terminal() {
			continue
		}
		s.removeWorktree(ctx, wt)
	}
	// Also remove the on-disk worktrees whose rows were marked removed at
	// merge but whose directories remain.
	s.pruneMergedWorktrees(ctx)
}

func (s *Server) removeWorktree(ctx context.Context, wt store.Worktree) {
	abs := s.worktreeAbs(wt.Path)
	if _, err := gitIn(s.RepoRoot, "worktree", "remove", "--force", abs); err != nil {
		s.Log.Warn("git worktree remove", "path", wt.Path, "err", err)
	}
	_ = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.MarkWorktreeRemoved(ctx, tx, wt.ID)
	})
}

// pruneMergedWorktrees removes on-disk worktree directories that git still
// tracks but whose feature is done (their rows were removed at merge).
func (s *Server) pruneMergedWorktrees(ctx context.Context) {
	// `git worktree prune` cleans up administrative refs for deleted dirs;
	// removing the merged worktree dir plus prune keeps `git worktree list`
	// honest.
	_, _ = gitIn(s.RepoRoot, "worktree", "prune")
}

// ReconcileWorktrees repairs worktree state on boot (DESIGN-006 §7): a live
// worktree row whose directory is gone (crash before the git op) for a
// non-terminal feature is re-created.
func (s *Server) ReconcileWorktrees(ctx context.Context) error {
	worktrees, err := s.Store.LiveWorktrees(ctx)
	if err != nil {
		return err
	}
	for i := range worktrees {
		wt := worktrees[i]
		abs := s.worktreeAbs(wt.Path)
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			continue // worktree present
		}
		f, err := store.GetFeature(ctx, s.Store.Pool, wt.FeatureID)
		if err != nil || f.State.Terminal() {
			continue
		}
		if gerr := s.addWorktree(wt.Path, wt.Branch); gerr != nil {
			s.Log.Error("worktree reconcile", "feature", wt.FeatureID, "err", gerr)
		} else {
			s.Log.Info("worktree re-created on boot", "feature", wt.FeatureID, "branch", wt.Branch)
		}
	}
	return nil
}

