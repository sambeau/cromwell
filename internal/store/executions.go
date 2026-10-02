package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Execution is one executor's part in a round of an item's work (SPEC-020
// FR-1.1). Measured is true for an agent, whose tokens are metered, and false
// for the chat agent and a person.
type Execution struct {
	ID          uuid.UUID
	RefType     string
	RefID       uuid.UUID
	Round       int
	Kind        string // agent | chat | person
	Actor       string
	Model       string
	Via         string // agent | mcp | ui
	DispatchID  *uuid.UUID
	ClaimID     *uuid.UUID
	Measured    bool
	Inferred    bool // backfilled by the migration (FR-1.3)
	StartHead   string
	StartedAt   time.Time
	SubmittedAt *time.Time
}

const executionCols = `id, ref_type, ref_id, round, kind, actor, model, via, dispatch_id, claim_id,
	measured, inferred, start_head, started_at, submitted_at`

func scanExecution(row pgx.Row) (*Execution, error) {
	var e Execution
	err := row.Scan(&e.ID, &e.RefType, &e.RefID, &e.Round, &e.Kind, &e.Actor, &e.Model, &e.Via,
		&e.DispatchID, &e.ClaimID, &e.Measured, &e.Inferred, &e.StartHead, &e.StartedAt, &e.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CurrentTaskRound is one more than the number of request_changes
// transitions the task has had (SPEC-020 FR-1.2). It is not the review round
// cap's count, which counts review-code dispatches. Callers count it inside
// the transaction that records the row, after any transition it makes.
func CurrentTaskRound(ctx context.Context, q Querier, taskID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
		SELECT 1 + count(*) FROM audit_events
		WHERE kind = 'task.transition' AND ref_type = 'task' AND ref_id = $1
		  AND payload->>'event' = 'request_changes'`, taskID).Scan(&n)
	return n, err
}

// RecordAgentExecution records an implement dispatch's start as an agent
// execution of the task's current round. A dispatch that has a row already
// (a retried attempt) records nothing new (FR-1.2).
func RecordAgentExecution(ctx context.Context, tx pgx.Tx, taskID, dispatchID uuid.UUID, role, model, startHead string) error {
	round, err := CurrentTaskRound(ctx, tx, taskID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, model, dispatch_id, via, measured, start_head)
		VALUES ($1, 'task', $2, $3, 'agent', $4, $5, $6, 'agent', true, $7)
		ON CONFLICT (dispatch_id) WHERE kind = 'agent' DO NOTHING`,
		NewID(), taskID, round, role, model, dispatchID, startHead)
	return err
}

// RecordClaimExecution records a claim, or a returned claim resumed, as an
// execution of the item's current round (FR-1.2). A claim has at most one row
// per round, so a resume after a send-back is a new row and the same claim.
func RecordClaimExecution(ctx context.Context, tx pgx.Tx, refType string, refID uuid.UUID, c *Claim, startHead string) (*Execution, error) {
	round := 1
	if refType == "task" {
		var err error
		if round, err = CurrentTaskRound(ctx, tx, refID); err != nil {
			return nil, err
		}
	}
	return scanExecution(tx.QueryRow(ctx, `
		INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, dispatch_id, claim_id, via, measured, start_head)
		VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, $8, false, $9)
		RETURNING `+executionCols,
		NewID(), refType, refID, round, c.Kind, c.Actor, c.ID, c.Via, startHead))
}

// MarkDispatchExecutionSubmitted stamps submitted_at on the latest row of an
// implement dispatch: the agent has handed its work in (FR-1.2).
func MarkDispatchExecutionSubmitted(ctx context.Context, tx pgx.Tx, dispatchID uuid.UUID) error {
	return markExecutionSubmitted(ctx, tx, "dispatch_id", dispatchID)
}

// MarkClaimExecutionSubmitted stamps submitted_at on the latest row of a
// claim: the chat agent or the person has handed its work in (FR-1.2).
func MarkClaimExecutionSubmitted(ctx context.Context, tx pgx.Tx, claimID uuid.UUID) error {
	return markExecutionSubmitted(ctx, tx, "claim_id", claimID)
}

// markExecutionSubmitted stamps the latest execution whose column (one of
// this package's own two) holds id.
func markExecutionSubmitted(ctx context.Context, tx pgx.Tx, col string, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE executions SET submitted_at = now()
		WHERE id = (SELECT id FROM executions WHERE `+col+` = $1 ORDER BY started_at DESC, id DESC LIMIT 1)`, id)
	return err
}

// ExecutionsFor returns the item's executions in the order they began.
func ExecutionsFor(ctx context.Context, q Querier, refType string, refID uuid.UUID) ([]Execution, error) {
	rows, err := q.Query(ctx, `SELECT `+executionCols+` FROM executions
		WHERE ref_type = $1 AND ref_id = $2 ORDER BY started_at, id`, refType, refID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Execution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}
