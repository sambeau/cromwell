package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
)

// Claim is a person or the chat agent holding an item instead of an agent
// (SPEC-020 FR-2.1). The state machine is lifecycle's (FR-2.2).
type Claim struct {
	ID        uuid.UUID
	RefType   string
	RefID     uuid.UUID
	FeatureID *uuid.UUID // the feature whose working copy it holds; nil when it holds none
	Kind      string     // chat | person
	Actor     string
	Via       string // mcp | ui
	State     lifecycle.ClaimState
	EndReason string // done | released | abandoned, once ended
	EndedBy   string // who ended it, for released and abandoned
	ClaimedAt time.Time
	// LastActivityAt and LastActivity say when activity was last seen, and
	// what it was (SD-8).
	LastActivityAt time.Time
	LastActivity   string
	WorktreeSeen   string // the fingerprint the sweep last saw (FR-5.2)
	DeadlineAt     *time.Time
	SubmittedAt    *time.Time
	EndedAt        *time.Time
}

const claimCols = `id, ref_type, ref_id, feature_id, kind, actor, via, state,
	COALESCE(end_reason, ''), COALESCE(ended_by, ''), claimed_at, last_activity_at,
	last_activity, worktree_seen, deadline_at, submitted_at, ended_at`

func scanClaim(row pgx.Row) (*Claim, error) {
	var c Claim
	err := row.Scan(&c.ID, &c.RefType, &c.RefID, &c.FeatureID, &c.Kind, &c.Actor, &c.Via, &c.State,
		&c.EndReason, &c.EndedBy, &c.ClaimedAt, &c.LastActivityAt, &c.LastActivity, &c.WorktreeSeen,
		&c.DeadlineAt, &c.SubmittedAt, &c.EndedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanClaims(rows pgx.Rows) ([]Claim, error) {
	defer rows.Close()
	var out []Claim
	for rows.Next() {
		c, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// claimAuditKind names the audit row of each event (SPEC-020 FR-2.11). Done
// and abandon both end the claim; release has its own kind.
var claimAuditKind = map[lifecycle.ClaimEvent]string{
	lifecycle.ClaimEventClaim:    "claim.claimed",
	lifecycle.ClaimEventRenew:    "claim.renewed",
	lifecycle.ClaimEventResume:   "claim.resumed",
	lifecycle.ClaimEventSubmit:   "claim.submitted",
	lifecycle.ClaimEventSendBack: "claim.sent_back",
	lifecycle.ClaimEventRelease:  "claim.released",
	lifecycle.ClaimEventDone:     "claim.ended",
	lifecycle.ClaimEventAbandon:  "claim.ended",
}

// claimEndReason is the end_reason an ending event gives (SPEC-020 FR-2.1).
var claimEndReason = map[lifecycle.ClaimEvent]string{
	lifecycle.ClaimEventDone:    "done",
	lifecycle.ClaimEventRelease: "released",
	lifecycle.ClaimEventAbandon: "abandoned",
}

func auditClaim(ctx context.Context, tx pgx.Tx, actor string, e lifecycle.ClaimEvent, c *Claim, payload map[string]any) error {
	p := map[string]any{}
	for k, v := range payload {
		p[k] = v
	}
	p["claim_id"] = c.ID.String()
	p["kind"] = c.Kind
	p["via"] = c.Via
	if r, ok := claimEndReason[e]; ok {
		p["end_reason"] = r
	}
	return Audit(ctx, tx, actor, claimAuditKind[e], c.RefType, &c.RefID, p)
}

// CreateClaim opens a claim (the machine's claim event), audited on the
// claimed item. The partial unique indexes refuse a second claim that hasn't
// ended on the item, and a second open claim on the feature (SPEC-020
// FR-2.1); the error is the database's.
func CreateClaim(ctx context.Context, tx pgx.Tx, refType string, refID uuid.UUID, featureID *uuid.UUID, kind, actor, via, worktreeSeen string) (*Claim, error) {
	state, err := lifecycle.ClaimTransition("", lifecycle.ClaimEventClaim)
	if err != nil {
		return nil, err
	}
	c := &Claim{ID: NewID(), RefType: refType, RefID: refID, FeatureID: featureID,
		Kind: kind, Actor: actor, Via: via, State: state, LastActivity: "claimed", WorktreeSeen: worktreeSeen}
	err = tx.QueryRow(ctx, `
		INSERT INTO work_claims (id, ref_type, ref_id, feature_id, kind, actor, via, state, worktree_seen)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING claimed_at, last_activity_at`,
		c.ID, refType, refID, featureID, kind, actor, via, string(state), worktreeSeen).
		Scan(&c.ClaimedAt, &c.LastActivityAt)
	if err != nil {
		return nil, err
	}
	if err := auditClaim(ctx, tx, actor, lifecycle.ClaimEventClaim, c, nil); err != nil {
		return nil, err
	}
	return c, nil
}

func GetClaim(ctx context.Context, q Querier, id uuid.UUID) (*Claim, error) {
	return scanClaim(q.QueryRow(ctx, `SELECT `+claimCols+` FROM work_claims WHERE id = $1`, id))
}

// CurrentClaimFor returns the item's claim that hasn't ended, or ErrNotFound.
func CurrentClaimFor(ctx context.Context, q Querier, refType string, refID uuid.UUID) (*Claim, error) {
	return scanClaim(q.QueryRow(ctx, `SELECT `+claimCols+` FROM work_claims
		WHERE ref_type = $1 AND ref_id = $2 AND state <> 'ended'`, refType, refID))
}

// LatestClaimFor returns the item's most recent claim in any state, or
// ErrNotFound.
func LatestClaimFor(ctx context.Context, q Querier, refType string, refID uuid.UUID) (*Claim, error) {
	return scanClaim(q.QueryRow(ctx, `SELECT `+claimCols+` FROM work_claims
		WHERE ref_type = $1 AND ref_id = $2 ORDER BY claimed_at DESC, id DESC LIMIT 1`, refType, refID))
}

// LockCurrentClaimFor is CurrentClaimFor with FOR UPDATE, in the lock order
// of SPEC-020 FR-2.7.
func LockCurrentClaimFor(ctx context.Context, tx pgx.Tx, refType string, refID uuid.UUID) (*Claim, error) {
	return scanClaim(tx.QueryRow(ctx, `SELECT `+claimCols+` FROM work_claims
		WHERE ref_type = $1 AND ref_id = $2 AND state <> 'ended' FOR UPDATE`, refType, refID))
}

// OpenClaimForFeature returns the feature's open claim, the one holding its
// working copy (SD-4), or ErrNotFound.
func OpenClaimForFeature(ctx context.Context, q Querier, featureID uuid.UUID) (*Claim, error) {
	return scanClaim(q.QueryRow(ctx, `SELECT `+claimCols+` FROM work_claims
		WHERE feature_id = $1 AND state = 'open'`, featureID))
}

// ClaimsToSweep returns the claims the sweep looks at: open or returned
// (FR-5.1), oldest activity first.
func ClaimsToSweep(ctx context.Context, q Querier) ([]Claim, error) {
	rows, err := q.Query(ctx, `SELECT `+claimCols+` FROM work_claims
		WHERE state IN ('open', 'returned') ORDER BY last_activity_at, id`)
	if err != nil {
		return nil, err
	}
	return scanClaims(rows)
}

// TransitionClaim applies a claim event with a guarded update (WHERE state =
// the state the caller read), so a lost race returns ErrStaleState rather
// than overwriting. activity, when given, becomes the claim's last activity,
// stamped now. The event sets submitted_at, ended_at, end_reason and ended_by
// as it implies, and the audit row (FR-2.11) is written on the claimed item,
// with the caller's payload and the claim's id, kind and via.
func TransitionClaim(ctx context.Context, tx pgx.Tx, c *Claim, e lifecycle.ClaimEvent, actor, activity string, payload map[string]any) error {
	next, err := lifecycle.ClaimTransition(c.State, e)
	if err != nil {
		return err
	}
	reason := claimEndReason[e]
	var endedBy string
	if e == lifecycle.ClaimEventRelease || e == lifecycle.ClaimEventAbandon {
		endedBy = actor
	}
	var lastAt time.Time
	var submittedAt, endedAt *time.Time
	err = tx.QueryRow(ctx, `
		UPDATE work_claims SET
		  state = $3,
		  last_activity = CASE WHEN $4 <> '' THEN $4 ELSE last_activity END,
		  last_activity_at = CASE WHEN $4 <> '' THEN now() ELSE last_activity_at END,
		  submitted_at = CASE WHEN $5 THEN now() ELSE submitted_at END,
		  ended_at = CASE WHEN $3 = 'ended' THEN now() ELSE ended_at END,
		  end_reason = CASE WHEN $3 = 'ended' THEN $6 ELSE end_reason END,
		  ended_by = CASE WHEN $3 = 'ended' THEN NULLIF($7, '') ELSE ended_by END
		WHERE id = $1 AND state = $2
		RETURNING last_activity_at, submitted_at, ended_at`,
		c.ID, string(c.State), string(next), activity, e == lifecycle.ClaimEventSubmit, reason, endedBy).
		Scan(&lastAt, &submittedAt, &endedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrStaleState
	}
	if err != nil {
		return err
	}
	c.State = next
	c.LastActivityAt, c.SubmittedAt, c.EndedAt = lastAt, submittedAt, endedAt
	if activity != "" {
		c.LastActivity = activity
	}
	if next == lifecycle.ClaimEnded {
		c.EndReason, c.EndedBy = reason, endedBy
	}
	return auditClaim(ctx, tx, actor, e, c, payload)
}

// RecordClaimActivity notes activity on an open claim: what it was and,
// when given, the fingerprint seen (SD-8, FR-5.1). It touches only an open
// claim, so a sweep racing a submit or a release changes nothing, and says
// whether it changed anything. It writes no audit row: the caller audits
// when the sweep sees a change (FR-2.11).
func RecordClaimActivity(ctx context.Context, tx pgx.Tx, id uuid.UUID, activity, worktreeSeen string) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE work_claims SET last_activity = $2, last_activity_at = now(),
		  worktree_seen = CASE WHEN $3 <> '' THEN $3 ELSE worktree_seen END
		WHERE id = $1 AND state = 'open'`, id, activity, worktreeSeen)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// EndClaimsForFeature abandons every claim on the feature that hasn't ended,
// each audited, for a feature that is itself abandoned (SPEC-020 SD-16). It
// returns the claims it ended.
func EndClaimsForFeature(ctx context.Context, tx pgx.Tx, featureID uuid.UUID, actor, reason string) ([]Claim, error) {
	rows, err := tx.Query(ctx, `SELECT `+claimCols+` FROM work_claims
		WHERE feature_id = $1 AND state <> 'ended' ORDER BY claimed_at, id FOR UPDATE`, featureID)
	if err != nil {
		return nil, err
	}
	claims, err := scanClaims(rows)
	if err != nil {
		return nil, err
	}
	for i := range claims {
		if err := TransitionClaim(ctx, tx, &claims[i], lifecycle.ClaimEventAbandon, actor, "",
			map[string]any{"reason": reason}); err != nil {
			return nil, err
		}
	}
	return claims, nil
}
