package server

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"subutai/internal/store"
)

// claimRefusalFor says why a person can't claim a task just now, in FR-2.4's
// words, or "" when they can. It runs the claim's own checks and writes
// nothing: the transaction it reads in is rolled back. It is the task page's
// "can this be claimed, and why not" (SPEC-020 FR-4.1).
func (s *Server) claimRefusalFor(ctx context.Context, t *store.Task) (string, error) {
	c := s.claimables()["task"]
	target, err := c.load(ctx, s.Store.Pool, t.ID)
	if err != nil {
		return "", err
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.Log.Warn("rolling back a read-only claim check", "err", err)
		}
	}()
	return c.refusal(ctx, tx, target, s.PersonClaimant(), false), nil
}
