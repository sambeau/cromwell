package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Dispatch struct {
	ID               uuid.UUID
	State            string
	Purpose          string
	Role             string
	Model            string
	RefType          string
	RefID            uuid.UUID
	Attempt          int
	IdempotencyKey   string
	QueueReason      *string
	InputTokens      *int64
	OutputTokens     *int64
	CacheReadTokens  *int64
	CacheWriteTokens *int64
	CostUSD          *float64
	PriceSnapshot    json.RawMessage
	Outcome          json.RawMessage
	Error            *string
	QueuedAt         time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	HeartbeatAt      *time.Time
}

const dispatchCols = `id, state, purpose, role, model, ref_type, ref_id, attempt,
	idempotency_key, queue_reason, input_tokens, output_tokens, cache_read_tokens,
	cache_write_tokens, cost_usd, price_snapshot, outcome, error,
	queued_at, started_at, finished_at, heartbeat_at`

func scanDispatch(row pgx.Row) (*Dispatch, error) {
	var d Dispatch
	err := row.Scan(&d.ID, &d.State, &d.Purpose, &d.Role, &d.Model, &d.RefType, &d.RefID,
		&d.Attempt, &d.IdempotencyKey, &d.QueueReason, &d.InputTokens, &d.OutputTokens,
		&d.CacheReadTokens, &d.CacheWriteTokens, &d.CostUSD, &d.PriceSnapshot,
		&d.Outcome, &d.Error, &d.QueuedAt, &d.StartedAt, &d.FinishedAt, &d.HeartbeatAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &d, err
}

// EnqueueDispatch inserts a queued dispatch. The partial unique index on
// idempotency_key makes replay a no-op: inserting a key that already has a
// live (queued/running/succeeded) dispatch returns (nil, nil) and the caller
// moves on (DESIGN-002 §8, FR-8.1).
func EnqueueDispatch(ctx context.Context, tx pgx.Tx, purpose, role, model, refType string, refID uuid.UUID, idempotencyKey string) (*Dispatch, error) {
	id := NewID()
	tag, err := tx.Exec(ctx, `
		INSERT INTO dispatches (id, purpose, role, model, ref_type, ref_id, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (idempotency_key) WHERE state IN ('queued','running','succeeded') DO NOTHING`,
		id, purpose, role, model, refType, refID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil // already live: idempotent replay
	}
	if err := Audit(ctx, tx, "orchestrator", "dispatch.queued", refType, &refID,
		map[string]any{"dispatch_id": id.String(), "purpose": purpose, "role": role}); err != nil {
		return nil, err
	}
	d := &Dispatch{ID: id, State: "queued", Purpose: purpose, Role: role, Model: model,
		RefType: refType, RefID: refID, Attempt: 1, IdempotencyKey: idempotencyKey}
	return d, nil
}

func GetDispatch(ctx context.Context, q Querier, id uuid.UUID) (*Dispatch, error) {
	return scanDispatch(q.QueryRow(ctx, `SELECT `+dispatchCols+` FROM dispatches WHERE id = $1`, id))
}

// QueuedDispatches returns dispatches awaiting the governor, oldest first.
func (s *Store) QueuedDispatches(ctx context.Context) ([]Dispatch, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+dispatchCols+` FROM dispatches WHERE state = 'queued' ORDER BY queued_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dispatch
	for rows.Next() {
		d, err := scanDispatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// MarkDispatchRunning claims a queued dispatch under governor approval,
// freezing the price snapshot (O-4). Returns false if another worker won.
func MarkDispatchRunning(ctx context.Context, tx pgx.Tx, id uuid.UUID, priceSnapshot any) (bool, error) {
	snap, err := json.Marshal(priceSnapshot)
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE dispatches
		SET state = 'running', started_at = now(), heartbeat_at = now(),
		    price_snapshot = $2, queue_reason = NULL
		WHERE id = $1 AND state = 'queued'`, id, snap)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	var refType string
	var refID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT ref_type, ref_id FROM dispatches WHERE id = $1`, id).
		Scan(&refType, &refID); err != nil {
		return false, err
	}
	return true, Audit(ctx, tx, "orchestrator", "dispatch.running", refType, &refID,
		map[string]any{"dispatch_id": id.String()})
}

// SetQueueReason records why the governor is holding a dispatch (visible in
// subutai status; DESIGN-002 §6 "nothing is dropped").
func (s *Store) SetQueueReason(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE dispatches SET queue_reason = $2 WHERE id = $1 AND state = 'queued'`, id, reason)
	return err
}

// HeartbeatAttempt updates liveness for one attempt, and reports whether that
// attempt is still the run's current, running one. False means the run was
// given up on — failed by the stall sweep, perhaps already retried — and the
// loop holding it should stop (SPEC-012 FR-1.9).
func (s *Store) HeartbeatAttempt(ctx context.Context, id uuid.UUID, attempt int) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE dispatches SET heartbeat_at = now() WHERE id = $1 AND attempt = $2 AND state = 'running'`, id, attempt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// MarkAttemptSucceeded is MarkDispatchSucceeded for one attempt: it refuses
// when the attempt is no longer the run's current, running one, so a loop
// that was given up on can't record its outcome over a retry's (FR-1.9).
func MarkAttemptSucceeded(ctx context.Context, tx pgx.Tx, id uuid.UUID, attempt int, usage TokenUsage, costUSD float64, outcome any) error {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM dispatches
		WHERE id = $1 AND attempt = $2 AND state = 'running'`, id, attempt).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrAttemptGivenUp
	}
	return MarkDispatchSucceeded(ctx, tx, id, usage, costUSD, outcome)
}

// ErrAttemptGivenUp is returned when an attempt is no longer the run's
// current, running one.
var ErrAttemptGivenUp = errors.New("this attempt was given up on while it was still running")

// Heartbeat updates liveness for stall detection.
func (s *Store) Heartbeat(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE dispatches SET heartbeat_at = now() WHERE id = $1 AND state = 'running'`, id)
	return err
}

type TokenUsage struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
}

// MarkDispatchSucceeded records consumption, cost, and the structured
// outcome (FR-7.2).
func MarkDispatchSucceeded(ctx context.Context, tx pgx.Tx, id uuid.UUID, usage TokenUsage, costUSD float64, outcome any) error {
	out, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE dispatches
		SET state = 'succeeded', finished_at = now(),
		    input_tokens = $2, output_tokens = $3, cache_read_tokens = $4,
		    cache_write_tokens = $5, cost_usd = $6, outcome = $7
		WHERE id = $1 AND state = 'running'`,
		id, usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite, costUSD, out)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("dispatch not running")
	}
	var refType string
	var refID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT ref_type, ref_id FROM dispatches WHERE id = $1`, id).
		Scan(&refType, &refID); err != nil {
		return err
	}
	return Audit(ctx, tx, "orchestrator", "dispatch.succeeded", refType, &refID,
		map[string]any{"dispatch_id": id.String(), "cost_usd": costUSD,
			"input_tokens": usage.Input, "output_tokens": usage.Output,
			// The structured outcome rides along so `subutai log` answers
			// "what did the reviewer say" without a ledger query.
			"outcome": outcome})
}

// MarkDispatchFailed records a failure; the caller decides on retry.
func MarkDispatchFailed(ctx context.Context, tx pgx.Tx, id uuid.UUID, reason string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE dispatches SET state = 'failed', finished_at = now(), error = $2
		WHERE id = $1 AND state IN ('queued', 'running')`, id, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("dispatch not open")
	}
	var refType string
	var refID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT ref_type, ref_id FROM dispatches WHERE id = $1`, id).
		Scan(&refType, &refID); err != nil {
		return err
	}
	// The attempt's transcript ends with why it failed (SPEC-012 SD-3). This
	// is the one place every failure passes through — a loop error, a stalled
	// process, a refusal at admission — and RequeueDispatch later clears
	// dispatches.error, so the transcript is where the reason survives.
	appendErrorEntry(ctx, tx, id, reason)
	return Audit(ctx, tx, "orchestrator", "dispatch.failed", refType, &refID,
		map[string]any{"dispatch_id": id.String(), "error": reason})
}

// RequeueDispatch resets a failed dispatch for another attempt (fresh
// context each attempt, attempt incremented — DESIGN-002 §8).
func RequeueDispatch(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `
		UPDATE dispatches
		SET state = 'queued', attempt = attempt + 1, error = NULL,
		    started_at = NULL, finished_at = NULL, heartbeat_at = NULL, queued_at = now()
		WHERE id = $1 AND state = 'failed'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("dispatch not failed")
	}
	var refType string
	var refID uuid.UUID
	var attempt int
	if err := tx.QueryRow(ctx, `SELECT ref_type, ref_id, attempt FROM dispatches WHERE id = $1`, id).
		Scan(&refType, &refID, &attempt); err != nil {
		return err
	}
	return Audit(ctx, tx, "orchestrator", "dispatch.requeued", refType, &refID,
		map[string]any{"dispatch_id": id.String(), "attempt": attempt})
}

// StalledRunning returns running dispatches whose heartbeat is older than
// the threshold (heartbeat duty 1, DESIGN-002 §3).
func (s *Store) StalledRunning(ctx context.Context, olderThan time.Duration) ([]Dispatch, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+dispatchCols+` FROM dispatches
		WHERE state = 'running' AND heartbeat_at < now() - $1::interval`,
		olderThan.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dispatch
	for rows.Next() {
		d, err := scanDispatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// PeriodCost sums completed-dispatch cost since a period start (governor
// budget check, DESIGN-002 §6).
func PeriodCost(ctx context.Context, q Querier, since time.Time) (float64, error) {
	var total float64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM dispatches
		WHERE cost_usd IS NOT NULL AND queued_at >= $1`, since).Scan(&total)
	return total, err
}

// CostRow is one line of the cost rollup (subutai cost, FR-7.2).
type CostRow struct {
	RefType string
	RefID   uuid.UUID
	Count   int
	CostUSD float64
	Tokens  int64
}

func (s *Store) CostRollup(ctx context.Context) ([]CostRow, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT ref_type, ref_id, count(*),
		       COALESCE(SUM(cost_usd), 0),
		       COALESCE(SUM(input_tokens + output_tokens + COALESCE(cache_read_tokens,0) + COALESCE(cache_write_tokens,0)), 0)
		FROM dispatches
		WHERE state = 'succeeded'
		GROUP BY ref_type, ref_id
		ORDER BY 4 DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CostRow
	for rows.Next() {
		var r CostRow
		if err := rows.Scan(&r.RefType, &r.RefID, &r.Count, &r.CostUSD, &r.Tokens); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FailedRetryable returns failed dispatches eligible for the heartbeat retry
// sweep: attempts remaining and backoff elapsed (DESIGN-002 §3 duty 2).
func (s *Store) FailedRetryable(ctx context.Context, maxAttempts int, backoffBase time.Duration) ([]Dispatch, []Dispatch, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+dispatchCols+` FROM dispatches WHERE state = 'failed'`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var retry, exhausted []Dispatch
	now := time.Now()
	for rows.Next() {
		d, err := scanDispatch(rows)
		if err != nil {
			return nil, nil, err
		}
		if d.Attempt >= maxAttempts {
			exhausted = append(exhausted, *d)
			continue
		}
		// Exponential backoff: base * 2^(attempt-1) after the failure.
		wait := backoffBase << (d.Attempt - 1)
		if d.FinishedAt == nil || now.After(d.FinishedAt.Add(wait)) {
			retry = append(retry, *d)
		}
	}
	return retry, exhausted, rows.Err()
}

// MarkDispatchCancelled closes a failed dispatch for good (human declined
// retry on its dispatch-failure checkpoint).
func MarkDispatchCancelled(ctx context.Context, tx pgx.Tx, id uuid.UUID, actor string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE dispatches SET state = 'cancelled', finished_at = now()
		WHERE id = $1 AND state = 'failed'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("dispatch not failed")
	}
	var refType string
	var refID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT ref_type, ref_id FROM dispatches WHERE id = $1`, id).
		Scan(&refType, &refID); err != nil {
		return err
	}
	return Audit(ctx, tx, actor, "dispatch.cancelled", refType, &refID,
		map[string]any{"dispatch_id": id.String()})
}

// RecordToolCall appends a tool_calls ledger row (DESIGN-001 §8, FR-6.5).
func (s *Store) RecordToolCall(ctx context.Context, dispatchID uuid.UUID, seq int, tool string, argBytes, resultBytes, latencyMs int, status string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO tool_calls (id, dispatch_id, seq, tool, arg_bytes, result_bytes, latency_ms, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		NewID(), dispatchID, seq, tool, argBytes, resultBytes, latencyMs, status)
	return err
}

// MutatingDispatchActiveForTask reports whether another implement-task
// dispatch is queued/running for a task of the same feature as taskID —
// governor check 3 (DESIGN-006 §5). excludeID is the candidate dispatch.
func MutatingDispatchActiveForTask(ctx context.Context, q Querier, taskID, excludeID uuid.UUID) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM dispatches d
			JOIN tasks t ON t.id = d.ref_id AND d.ref_type = 'task'
			WHERE d.purpose = 'implement-task'
			  AND d.state = 'running'
			  AND d.id <> $2
			  AND t.feature_id = (SELECT feature_id FROM tasks WHERE id = $1)
		)`, taskID, excludeID).Scan(&exists)
	return exists, err
}

// CountDispatchesForRef counts all dispatches (any state) for an entity and
// purpose — the re-dispatch index for idempotency keys (DESIGN-006 §7).
func CountDispatchesForRef(ctx context.Context, q Querier, refType string, refID uuid.UUID, purpose string) (int, error) {
	var n int
	err := q.QueryRow(ctx,
		`SELECT count(*) FROM dispatches WHERE ref_type = $1 AND ref_id = $2 AND purpose = $3`,
		refType, refID, purpose).Scan(&n)
	return n, err
}
