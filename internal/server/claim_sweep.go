package server

// The claim sweep (SPEC-020 FR-5.1 to FR-5.5): the heartbeat's look at every
// claim that is open or returned. It records activity it can see in an open
// claim's working copy, and asks a person when a claim has had none for too
// long or has passed its deadline. The answers (keep it, release it) are the
// claim service's: keep records activity `kept`, and release is ReleaseClaim.

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/rules"
	"subutai/internal/store"
)

// deadlineEnder is the optional hook of a claimable whose deadline ends the
// item instead of asking a question: the sweep calls endAtDeadline for a claim
// past its deadline (SPEC-021 FR-14.4).
type deadlineEnder interface {
	endAtDeadline(ctx context.Context, t *claimTarget) error
}

// ClaimSweep runs one pass of FR-5.1. It is the heartbeat's duty after
// StallSweep, and tests call it directly.
func (s *Server) ClaimSweep(ctx context.Context) {
	claims, err := store.ClaimsToSweep(ctx, s.Store.Pool)
	if err != nil {
		s.Log.Error("claim sweep", "err", err)
		return
	}
	if len(claims) == 0 {
		return
	}
	cfg, err := s.freshConfig()
	if err != nil {
		s.Log.Error("claim sweep: reading the configuration", "err", err)
		return
	}
	expiry := cfg.ClaimExpiry()
	for i := range claims {
		if ctx.Err() != nil {
			return
		}
		if err := s.sweepClaim(ctx, claims[i], expiry); err != nil {
			s.Log.Error("claim sweep", "claim", claims[i].ID, "err", err)
		}
	}
}

func (s *Server) sweepClaim(ctx context.Context, c store.Claim, expiry time.Duration) error {
	cl, ok := s.claimables()[c.RefType]
	if !ok {
		return nil
	}
	t, err := cl.load(ctx, s.Store.Pool, c.RefID)
	if err != nil {
		return err
	}

	// A claimable with a hard deadline of its own, a spike's time box, has the
	// deadline judged first: past it, the item ends and the claim with it, so
	// neither question is asked a moment before the ending withdraws it, and
	// no claim-deadline is ever raised (SPEC-021 FR-14.4, SD-20).
	de, hardDeadline := cl.(deadlineEnder)
	if hardDeadline && c.DeadlineAt != nil && !c.DeadlineAt.After(time.Now()) {
		return de.endAtDeadline(ctx, t)
	}

	// Step 1: the working copy, before expiry is judged, so that work done
	// while the server was down is seen first (SD-9).
	if c.State == lifecycle.ClaimOpen && t.Path != "" {
		if _, err := os.Stat(t.Path); err == nil {
			fresh, err := s.sweepFingerprint(ctx, t.Path, c.ID)
			if err != nil {
				return err
			}
			if fresh != nil {
				c = *fresh
			}
		}
	}

	now := time.Now()
	// Step 2: expiry. A claim that is open or returned holds the task, so
	// both are asked about (SD-9).
	if now.Sub(c.LastActivityAt) >= expiry {
		idle := now.Sub(c.LastActivityAt)
		q, release := cl.staleQuestion(t, &c, idle)
		if err := s.raiseClaimCheckpoint(ctx, cl, "claim-stale", t, &c, q, release, func(cur *store.Claim) bool {
			return now.Sub(cur.LastActivityAt) >= expiry
		}); err != nil {
			return err
		}
	}
	// Step 3: the deadline.
	if !hardDeadline && c.DeadlineAt != nil && !c.DeadlineAt.After(now) {
		q, release := cl.deadlineQuestion(t, &c)
		if err := s.raiseClaimCheckpoint(ctx, cl, "claim-deadline", t, &c, q, release, func(cur *store.Claim) bool {
			return cur.DeadlineAt != nil && !cur.DeadlineAt.After(now)
		}); err != nil {
			return err
		}
	}
	return nil
}

// sweepFingerprint compares the open claim's working copy with the fingerprint
// last seen, holding its lock (FR-5.1 step 1, FR-5.2). On a change it records
// activity `worktree`, conditionally on the claim still being open, audits
// `claim.activity` and withdraws a pending claim-stale (FR-5.4). It returns
// the claim as it then stands, or nil when nothing changed.
func (s *Server) sweepFingerprint(ctx context.Context, path string, claimID uuid.UUID) (*store.Claim, error) {
	var out *store.Claim
	err := s.withWorkingCopy(path, func() error {
		cur, err := store.GetClaim(ctx, s.Store.Pool, claimID)
		if err != nil {
			return err
		}
		if cur.State != lifecycle.ClaimOpen {
			return nil
		}
		fp, err := worktreeFingerprint(path)
		if err != nil {
			return err
		}
		if fp == cur.WorktreeSeen {
			return nil
		}
		return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			out = nil
			changed, err := store.RecordClaimActivity(ctx, tx, cur.ID, "worktree", fp)
			if err != nil || !changed {
				return err
			}
			if err := store.Audit(ctx, tx, "orchestrator", "claim.activity", cur.RefType, &cur.RefID,
				map[string]any{"claim_id": cur.ID.String(), "kind": cur.Kind, "via": cur.Via, "activity": "worktree"}); err != nil {
				return err
			}
			if _, err := store.WithdrawCheckpoints(ctx, tx, "claim-stale", cur.RefType, cur.RefID, "orchestrator",
				"the claim's working copy changed"); err != nil {
				return err
			}
			out, err = store.GetClaim(ctx, tx, cur.ID)
			return err
		})
	})
	return out, err
}

// raiseClaimCheckpoint raises a claim-stale or claim-deadline checkpoint on
// the claimed item, unless one is already pending. It looks at the claim again
// under its row lock, so an activity that has just withdrawn the question
// isn't followed by a stale one (FR-5.4); stillDue judges the fresh claim.
func (s *Server) raiseClaimCheckpoint(ctx context.Context, cl claimable, kind string, t *claimTarget, c *store.Claim, question, releaseLabel string, stillDue func(*store.Claim) bool) error {
	var cp *store.Checkpoint
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		cp = nil
		cur, err := store.LockClaim(ctx, tx, c.ID)
		if err != nil {
			return err
		}
		if (cur.State != lifecycle.ClaimOpen && cur.State != lifecycle.ClaimReturned) || !stillDue(cur) {
			return nil
		}
		cp, err = store.CreateCheckpoint(ctx, tx, kind, c.RefType, c.RefID, question, map[string]any{
			"claim_id":         c.ID.String(),
			"ref":              t.Label,
			"state":            string(cur.State),
			"executor":         map[string]any{"kind": c.Kind, "actor": c.Actor, "via": c.Via, "who": whoWords(c.Kind, c.Actor, "")},
			"claimed_at":       c.ClaimedAt.UTC().Format(time.RFC3339),
			"last_activity":    cur.LastActivity,
			"last_activity_at": cur.LastActivityAt.UTC().Format(time.RFC3339),
			"working_copy":     t.Path,
			"release_label":    releaseLabel,
			// What the release answer does, which the Inbox says beside it
			// (SPEC-021 FR-14.2). A question raised before it was stored has none.
			"release_consequence": cl.releaseConsequence(),
		})
		return err
	})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	s.notifyCheckpointRaised(cp)
	return nil
}

// ---- The answers (FR-5.3, FR-5.4, FR-5.10) ----

// KeepClaimAnswered is a person's "Keep the claim": activity `kept`, and the
// clock restarts. A claim that has moved on since the question was asked, or
// that isn't the claim it asked about, is left alone (FR-5.4).
func (s *Server) KeepClaimAnswered(ctx context.Context, a rules.KeepClaim) error {
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		cur, err := store.LockClaim(ctx, tx, parseClaimID(a.ClaimID))
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if cur.RefType != a.RefType || cur.RefID != a.RefID {
			return nil
		}
		_, err = store.KeepClaim(ctx, tx, cur.ID, a.Actor, a.ClearDeadline)
		return err
	})
}

// ReleaseClaimAnswered is a person's "Release it to an agent": ReleaseClaim,
// or, for a spike, "Release the claim": ReleaseSpikeClaim (SPEC-021 FR-14.2),
// with the person who answered as the one who released it. A claim that has
// moved on is left alone.
func (s *Server) ReleaseClaimAnswered(ctx context.Context, a rules.ReleaseClaim) error {
	cur, err := store.GetClaim(ctx, s.Store.Pool, parseClaimID(a.ClaimID))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if cur.RefType != a.RefType || cur.RefID != a.RefID ||
		(cur.State != lifecycle.ClaimOpen && cur.State != lifecycle.ClaimReturned) {
		return nil
	}
	var ref string
	var release func(ctx context.Context, ref, by string) error
	switch a.RefType {
	case "task":
		task, err := store.GetTask(ctx, s.Store.Pool, a.RefID)
		if err != nil {
			return err
		}
		ref, release = task.PublicID, s.ReleaseClaim
	case "spike":
		sp, err := store.GetSpike(ctx, s.Store.Pool, a.RefID)
		if err != nil {
			return err
		}
		ref, release = sp.PublicID, s.ReleaseSpikeClaim
	default:
		return nil
	}
	if err := release(ctx, ref, a.Actor); err != nil {
		if _, ok := AsClaimRefusal(err); ok {
			s.Log.Info("release after a claim question did nothing", "claim", cur.ID, "why", err)
			return nil
		}
		return err
	}
	return nil
}

// parseClaimID reads a claim's ID from a checkpoint's context; a malformed one
// is uuid.Nil, which no claim has, so the answer finds nothing to act on.
func parseClaimID(id string) uuid.UUID {
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil
	}
	return u
}

// claimMovedOnNotice is FR-5.4's notice for an answer that arrives after its
// question was withdrawn.
const claimMovedOnNotice = "That claim has moved on since the question was asked; nothing was changed."
