package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Worktree struct {
	ID           uuid.UUID
	FeatureID    uuid.UUID
	Path         string
	Branch       string
	GraphProject *string
	CreatedAt    time.Time
	RemovedAt    *time.Time
}

// CreateWorktreeRow inserts the DB record for a worktree; the git operation
// follows the commit (DESIGN-006 §3, commit-then-git). The partial unique
// index guarantees at most one live worktree per feature.
func CreateWorktreeRow(ctx context.Context, tx pgx.Tx, featureID uuid.UUID, path, branch, actor string) (*Worktree, error) {
	w := &Worktree{ID: NewID(), FeatureID: featureID, Path: path, Branch: branch}
	_, err := tx.Exec(ctx, `
		INSERT INTO worktrees (id, feature_id, path, branch) VALUES ($1, $2, $3, $4)`,
		w.ID, featureID, path, branch)
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "worktree.created", "feature", &featureID,
		map[string]any{"branch": branch, "path": path}); err != nil {
		return nil, err
	}
	return w, nil
}

// LiveWorktreeForFeature returns the feature's current worktree, or ErrNotFound.
func LiveWorktreeForFeature(ctx context.Context, q Querier, featureID uuid.UUID) (*Worktree, error) {
	var w Worktree
	err := q.QueryRow(ctx, `
		SELECT id, feature_id, path, branch, graph_project, created_at, removed_at
		FROM worktrees WHERE feature_id = $1 AND removed_at IS NULL`, featureID).
		Scan(&w.ID, &w.FeatureID, &w.Path, &w.Branch, &w.GraphProject, &w.CreatedAt, &w.RemovedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &w, err
}

// MarkWorktreeRemoved stamps removed_at after the git worktree is gone (GC).
func MarkWorktreeRemoved(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE worktrees SET removed_at = now() WHERE id = $1 AND removed_at IS NULL`, id)
	return err
}

// LiveWorktrees returns all worktrees not yet removed — the boot
// reconciliation input (DESIGN-006 §7).
func (s *Store) LiveWorktrees(ctx context.Context) ([]Worktree, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, feature_id, path, branch, graph_project, created_at, removed_at
		FROM worktrees WHERE removed_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Worktree
	for rows.Next() {
		var w Worktree
		if err := rows.Scan(&w.ID, &w.FeatureID, &w.Path, &w.Branch, &w.GraphProject, &w.CreatedAt, &w.RemovedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// SetSpecStale marks (or clears) a feature's contract as stale during a
// revision in flight (DESIGN-005 §6), audited.
func SetSpecStale(ctx context.Context, tx pgx.Tx, featureID uuid.UUID, stale bool, actor, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE features SET spec_stale = $2 WHERE id = $1 AND spec_stale <> $2`, featureID, stale)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // already in the desired state; no-op, no audit noise
	}
	return Audit(ctx, tx, actor, "feature.spec_stale", "feature", &featureID,
		map[string]any{"stale": stale, "reason": reason})
}

// FeatureSpecStale reports whether a feature's contract is stale.
func FeatureSpecStale(ctx context.Context, q Querier, featureID uuid.UUID) (bool, error) {
	var stale bool
	err := q.QueryRow(ctx, `SELECT spec_stale FROM features WHERE id = $1`, featureID).Scan(&stale)
	return stale, err
}

// SetFeatureBranch records the git branch on the feature once its worktree
// exists (DESIGN-006 §3).
func SetFeatureBranch(ctx context.Context, tx pgx.Tx, featureID uuid.UUID, branch string) error {
	_, err := tx.Exec(ctx, `UPDATE features SET branch = $2 WHERE id = $1`, featureID, branch)
	return err
}
