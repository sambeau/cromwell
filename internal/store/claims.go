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
	EndReason string // done | released | abandoned | expired, once ended
	EndedBy   string // who ended it, for released and abandoned; empty for expired
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
	lifecycle.ClaimEventExpire:   "claim.expired",
}

// claimEndReason is the end_reason an ending event gives (SPEC-020 FR-2.1).
var claimEndReason = map[lifecycle.ClaimEvent]string{
	lifecycle.ClaimEventDone:    "done",
	lifecycle.ClaimEventRelease: "released",
	lifecycle.ClaimEventAbandon: "abandoned",
	lifecycle.ClaimEventExpire:  "expired",
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

// ClaimEndedDoneFor says whether any claim of the item ended done: for a
// spike, that a submit got there and the spike is ending as concluded
// (SPEC-021 FR-13.7). It is the one place that rule is written, and it looks
// at every claim, not only the latest, so a claim made after the submit can't
// hide it.
func ClaimEndedDoneFor(ctx context.Context, q Querier, refType string, refID uuid.UUID) (bool, error) {
	var done bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM work_claims
		WHERE ref_type = $1 AND ref_id = $2 AND state = 'ended' AND end_reason = 'done')`, refType, refID).Scan(&done)
	return done, err
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
	// Every event is activity or an ending, so the claimant has come back or
	// the claim is over: the question whether anyone is still working on it
	// is withdrawn with it (SPEC-020 FR-5.4, SD-16).
	if _, err := WithdrawCheckpoints(ctx, tx, "claim-stale", c.RefType, c.RefID, actor,
		"the claim moved on: "+string(e)); err != nil {
		return err
	}
	if next == lifecycle.ClaimEnded {
		if _, err := WithdrawCheckpoints(ctx, tx, "claim-deadline", c.RefType, c.RefID, actor,
			"the claim moved on: "+string(e)); err != nil {
			return err
		}
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

// LockClaim reads a claim with FOR UPDATE, by id. The sweep takes it alone,
// so that raising a stale question and an activity that withdraws it can't
// pass each other (SPEC-020 FR-5.4); it takes no other row lock, so it can't
// close a cycle with the lock order of FR-2.7.
func LockClaim(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Claim, error) {
	return scanClaim(tx.QueryRow(ctx, `SELECT `+claimCols+` FROM work_claims WHERE id = $1 FOR UPDATE`, id))
}

// KeepClaim records that a person kept a claim that was asked about: activity
// `kept`, so the clock restarts (SPEC-020 FR-5.3, SD-8). It works on an open
// or a returned claim, which the machine's renew event can't (a returned claim
// has no renewal), and says whether it changed anything. clearDeadline also
// removes a time box that has passed, so the question isn't asked again at
// once. The audit row says "kept", and any pending claim-stale is withdrawn
// (FR-5.4).
func KeepClaim(ctx context.Context, tx pgx.Tx, id uuid.UUID, actor string, clearDeadline bool) (bool, error) {
	var c Claim
	err := tx.QueryRow(ctx, `
		UPDATE work_claims SET last_activity = 'kept', last_activity_at = now(),
		  deadline_at = CASE WHEN $2 THEN NULL ELSE deadline_at END
		WHERE id = $1 AND state IN ('open', 'returned')
		RETURNING `+claimCols, id, clearDeadline).
		Scan(&c.ID, &c.RefType, &c.RefID, &c.FeatureID, &c.Kind, &c.Actor, &c.Via, &c.State,
			&c.EndReason, &c.EndedBy, &c.ClaimedAt, &c.LastActivityAt, &c.LastActivity, &c.WorktreeSeen,
			&c.DeadlineAt, &c.SubmittedAt, &c.EndedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := WithdrawCheckpoints(ctx, tx, "claim-stale", c.RefType, c.RefID, actor, "the claim was kept"); err != nil {
		return false, err
	}
	return true, Audit(ctx, tx, actor, "claim.activity", c.RefType, &c.RefID,
		map[string]any{"claim_id": c.ID.String(), "kind": c.Kind, "via": c.Via, "activity": "kept", "by": actor})
}

// SetClaimDeadline sets the claim's hard deadline (SPEC-021 SD-20), which for
// a spike is the spike's own, fixed at the start.
func SetClaimDeadline(ctx context.Context, tx pgx.Tx, claimID uuid.UUID, at time.Time) error {
	tag, err := tx.Exec(ctx, `UPDATE work_claims SET deadline_at = $2 WHERE id = $1`, claimID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordSpikeFindingsActivity notes that the claimant saved findings: activity
// `findings` on an open claim, a claim.activity audit row on the claimed item
// (as the claim sweep writes for a changed working copy), and a pending
// claim-stale withdrawn, since someone is plainly still working (SPEC-021
// FR-13.6). It reports whether the claim was open; a claim that wasn't is left
// as it is and nothing is audited.
func RecordSpikeFindingsActivity(ctx context.Context, tx pgx.Tx, claimID uuid.UUID, actor string) (bool, error) {
	changed, err := RecordClaimActivity(ctx, tx, claimID, "findings", "")
	if err != nil || !changed {
		return false, err
	}
	c, err := GetClaim(ctx, tx, claimID)
	if err != nil {
		return false, err
	}
	if err := Audit(ctx, tx, actor, "claim.activity", c.RefType, &c.RefID,
		map[string]any{"claim_id": c.ID.String(), "kind": c.Kind, "via": c.Via, "activity": "findings"}); err != nil {
		return false, err
	}
	if _, err := WithdrawCheckpoints(ctx, tx, "claim-stale", c.RefType, c.RefID, actor,
		"findings were saved"); err != nil {
		return false, err
	}
	return true, nil
}
