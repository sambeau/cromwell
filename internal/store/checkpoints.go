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
