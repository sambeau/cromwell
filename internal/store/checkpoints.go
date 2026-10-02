package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Checkpoint struct {
	ID          uuid.UUID
	State       string
	Kind        string
	RefType     string
	RefID       uuid.UUID
	Question    string
	Context     json.RawMessage
	Options     json.RawMessage
	Response    json.RawMessage
	RespondedBy *string
	CreatedAt   time.Time
	AnsweredAt  *time.Time
}

const checkpointCols = `id, state, kind, ref_type, ref_id, question, context,
	options, response, responded_by, created_at, answered_at`

func scanCheckpoint(row pgx.Row) (*Checkpoint, error) {
	var c Checkpoint
	err := row.Scan(&c.ID, &c.State, &c.Kind, &c.RefType, &c.RefID, &c.Question,
		&c.Context, &c.Options, &c.Response, &c.RespondedBy, &c.CreatedAt, &c.AnsweredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

// CreateCheckpoint inserts a pending checkpoint. The partial unique index on
// (kind, ref) while pending makes creation idempotent (FR-6.2): replaying
// the triggering event returns (nil, nil).
func CreateCheckpoint(ctx context.Context, tx pgx.Tx, kind, refType string, refID uuid.UUID, question string, contextData map[string]any) (*Checkpoint, error) {
	ctxJSON, err := json.Marshal(contextData)
	if err != nil {
		return nil, err
	}
	id := NewID()
	tag, err := tx.Exec(ctx, `
		INSERT INTO checkpoints (id, kind, ref_type, ref_id, question, context)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (kind, ref_type, ref_id) WHERE state = 'pending' DO NOTHING`,
		id, kind, refType, refID, question, ctxJSON)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil // already pending: idempotent
	}
	if err := Audit(ctx, tx, "orchestrator", "checkpoint.created", refType, &refID,
		map[string]any{"checkpoint_id": id.String(), "kind": kind, "question": question}); err != nil {
		return nil, err
	}
	return &Checkpoint{ID: id, State: "pending", Kind: kind, RefType: refType, RefID: refID, Question: question, Context: ctxJSON}, nil
}

func GetCheckpoint(ctx context.Context, q Querier, id uuid.UUID) (*Checkpoint, error) {
	return scanCheckpoint(q.QueryRow(ctx, `SELECT `+checkpointCols+` FROM checkpoints WHERE id = $1`, id))
}

// PendingCheckpoints is the inbox (FR-6.1), oldest first.
func (s *Store) PendingCheckpoints(ctx context.Context) ([]Checkpoint, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+checkpointCols+` FROM checkpoints WHERE state = 'pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Checkpoint
	for rows.Next() {
		c, err := scanCheckpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// RespondCheckpoint records a human answer (FR-6.1): responder identity and
// reason live on the row and in the audit log.
func RespondCheckpoint(ctx context.Context, tx pgx.Tx, id uuid.UUID, response map[string]any, respondedBy string) (*Checkpoint, error) {
	respJSON, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE checkpoints
		SET state = 'answered', response = $2, responded_by = $3, answered_at = now()
		WHERE id = $1 AND state = 'pending'`, id, respJSON, respondedBy)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, errors.New("checkpoint is not pending")
	}
	cp, err := GetCheckpoint(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, respondedBy, "checkpoint.responded", cp.RefType, &cp.RefID,
		map[string]any{"checkpoint_id": id.String(), "kind": cp.Kind, "response": response}); err != nil {
		return nil, err
	}
	return cp, nil
}

// WithdrawCheckpoints withdraws the pending checkpoints of a kind on an item,
// because what they asked about has moved on, each with an audit row that says
// why (SPEC-020 FR-5.4). It returns how many it withdrew.
func WithdrawCheckpoints(ctx context.Context, tx pgx.Tx, kind, refType string, refID uuid.UUID, actor, reason string) (int, error) {
	rows, err := tx.Query(ctx, `
		UPDATE checkpoints SET state = 'withdrawn'
		WHERE kind = $1 AND ref_type = $2 AND ref_id = $3 AND state = 'pending'
		RETURNING id`, kind, refType, refID)
	if err != nil {
		return 0, err
	}
	return auditWithdrawn(ctx, tx, rows, kind, refType, refID, actor, reason)
}

// WithdrawImplementFailure withdraws the task's pending dispatch-failure
// checkpoint about an implement dispatch, for a claim that took the task
// (SPEC-020 SD-3). A dispatch-failure about the task's code review is left.
func WithdrawImplementFailure(ctx context.Context, tx pgx.Tx, taskID uuid.UUID, actor, reason string) (int, error) {
	rows, err := tx.Query(ctx, `
		UPDATE checkpoints c SET state = 'withdrawn'
		WHERE c.kind = 'dispatch-failure' AND c.ref_type = 'task' AND c.ref_id = $1 AND c.state = 'pending'
		  AND EXISTS (SELECT 1 FROM dispatches d
		              WHERE d.id::text = c.context->>'dispatch_id' AND d.purpose = 'implement-task')
		RETURNING c.id`, taskID)
	if err != nil {
		return 0, err
	}
	return auditWithdrawn(ctx, tx, rows, "dispatch-failure", "task", taskID, actor, reason)
}

// auditWithdrawn closes the rows of a withdrawing UPDATE and audits each.
func auditWithdrawn(ctx context.Context, tx pgx.Tx, rows pgx.Rows, kind, refType string, refID uuid.UUID, actor, reason string) (int, error) {
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := Audit(ctx, tx, actor, "checkpoint.withdrawn", refType, &refID,
			map[string]any{"checkpoint_id": id.String(), "kind": kind, "reason": reason}); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// PendingCheckpointFor returns the pending checkpoint of a kind on an item, or
// ErrNotFound. There is at most one: checkpoints are one per kind and item
// while pending.
func PendingCheckpointFor(ctx context.Context, q Querier, kind, refType string, refID uuid.UUID) (*Checkpoint, error) {
	return scanCheckpoint(q.QueryRow(ctx, `SELECT `+checkpointCols+` FROM checkpoints
		WHERE kind = $1 AND ref_type = $2 AND ref_id = $3 AND state = 'pending'`, kind, refType, refID))
}

// UpdatePendingCheckpoint replaces the question and context of a checkpoint
// that is still pending, for a notice that grows while it waits: more
// unclaimed commits are added to the one question rather than raising a second
// (SPEC-020 FR-5.8). It says whether the checkpoint was still pending.
func UpdatePendingCheckpoint(ctx context.Context, tx pgx.Tx, id uuid.UUID, question string, contextData map[string]any) (bool, error) {
	ctxJSON, err := json.Marshal(contextData)
	if err != nil {
		return false, err
	}
	var kind, refType string
	var refID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE checkpoints SET question = $2, context = $3
		WHERE id = $1 AND state = 'pending'
		RETURNING kind, ref_type, ref_id`, id, question, ctxJSON).Scan(&kind, &refType, &refID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, Audit(ctx, tx, "orchestrator", "checkpoint.updated", refType, &refID,
		map[string]any{"checkpoint_id": id.String(), "kind": kind, "question": question})
}
