package server

// The side-effect-free claim check (SPEC-020 FR-2.4, FR-3.6, FR-4.1): the
// same refusal logic a claim runs, read-only, for the places that show whether
// a task can be claimed (get_feature, the task page) without claiming it.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// claimRefusalFor says why who can't claim the task now, in FR-2.4's
// sentence, or "" when they can. It runs the claimable's own refusal inside a
// read-only transaction that is always rolled back, so it changes nothing and
// takes no lock. A task who already holds is claimable: claiming again renews
// it, or resumes it after a send-back.
func (s *Server) claimRefusalFor(ctx context.Context, t *store.Task, who Claimant) (string, error) {
	c := s.claimables()["task"]
	target, err := c.load(ctx, s.Store.Pool, t.ID)
	if err != nil {
		return "", err
	}
	tx, err := s.Store.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	resume := false
	if cur, err := store.CurrentClaimFor(ctx, tx, "task", t.ID); err == nil {
		resume = who.holds(cur) && cur.State == lifecycle.ClaimReturned
	}
	return c.refusal(ctx, tx, target, who, resume), nil
}
