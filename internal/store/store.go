package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the pgx pool. All mutation helpers take a pgx.Tx so callers
// compose state changes and their audit rows in one transaction (O-3); the
// only entry point that opens transactions is WithTx.
type Store struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

// WithTx runs fn in a transaction, committing on nil error.
func (s *Store) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// NewID returns a UUIDv7 (time-ordered, DESIGN-001 §2.5).
func NewID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New() // v4 fallback; NewV7 only fails if the clock does
	}
	return id
}

// ShortID is the human-facing display encoding of a UUID's tail
// (DESIGN-001 §2.5), e.g. FEAT-a3f9c2.
func ShortID(prefix string, id uuid.UUID) string {
	s := id.String()
	return fmt.Sprintf("%s-%s", prefix, s[len(s)-6:])
}

// Audit inserts an audit row inside the caller's transaction. Every
// lifecycle transition, gate evaluation, dispatch state change, and
// checkpoint event goes through here (FR-7.1).
func Audit(ctx context.Context, tx pgx.Tx, actor, kind, refType string, refID *uuid.UUID, payload map[string]any) error {
	p, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id, actor, kind, ref_type, ref_id, payload)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		NewID(), actor, kind, refType, refID, p)
	return err
}

// AuditEvent is a row from the audit stream (cromwell log).
type AuditEvent struct {
	ID         uuid.UUID
	OccurredAt time.Time
	Actor      string
	Kind       string
	RefType    string
	RefID      *uuid.UUID
	Payload    json.RawMessage
}

// AuditTail returns recent audit rows, optionally filtered to one ref,
// oldest first.
func (s *Store) AuditTail(ctx context.Context, refType string, refID *uuid.UUID, limit int) ([]AuditEvent, error) {
	q := `SELECT id, occurred_at, actor, kind, ref_type, ref_id, payload FROM audit_events`
	args := []any{}
	if refType != "" {
		q += ` WHERE ref_type = $1 AND ($2::uuid IS NULL OR ref_id = $2)`
		args = append(args, refType, refID)
	}
	// occurred_at is transaction time, identical for rows written in one
	// transaction; the UUIDv7 id is the within-transaction tiebreaker.
	q += fmt.Sprintf(` ORDER BY occurred_at DESC, id DESC LIMIT %d`, limit)
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Actor, &e.Kind, &e.RefType, &e.RefID, &e.Payload); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	// Reverse to oldest-first for tail-style reading.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}
