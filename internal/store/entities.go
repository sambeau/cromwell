package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/lifecycle"
)

var ErrNotFound = errors.New("not found")

type Initiative struct {
	ID          uuid.UUID
	ParentID    *uuid.UUID
	Slug        string
	Name        string
	Description string
	Archived    bool
	CreatedAt   time.Time
}

func CreateInitiative(ctx context.Context, tx pgx.Tx, parentID *uuid.UUID, slug, name, description, actor string) (*Initiative, error) {
	in := &Initiative{ID: NewID(), ParentID: parentID, Slug: slug, Name: name, Description: description}
	_, err := tx.Exec(ctx, `
		INSERT INTO initiatives (id, parent_id, slug, name, description)
		VALUES ($1, $2, $3, $4, $5)`,
		in.ID, parentID, slug, name, description)
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "initiative.created", "initiative", &in.ID,
		map[string]any{"slug": slug, "name": name}); err != nil {
		return nil, err
	}
	return in, nil
}

func GetInitiative(ctx context.Context, q Querier, id uuid.UUID) (*Initiative, error) {
	var in Initiative
	err := q.QueryRow(ctx, `
		SELECT id, parent_id, slug, name, description, archived, created_at
		FROM initiatives WHERE id = $1`, id).
		Scan(&in.ID, &in.ParentID, &in.Slug, &in.Name, &in.Description, &in.Archived, &in.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &in, err
}

// InitiativeBySlugPath resolves "auth/basic" style paths from the root.
func (s *Store) InitiativeBySlugPath(ctx context.Context, path []string) (*Initiative, error) {
	var parent *uuid.UUID
	var in *Initiative
	for _, slug := range path {
		var row Initiative
		err := s.Pool.QueryRow(ctx, `
			SELECT id, parent_id, slug, name, description, archived, created_at
			FROM initiatives WHERE slug = $1 AND parent_id IS NOT DISTINCT FROM $2`,
			slug, parent).
			Scan(&row.ID, &row.ParentID, &row.Slug, &row.Name, &row.Description, &row.Archived, &row.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		in = &row
		parent = &row.ID
	}
	if in == nil {
		return nil, ErrNotFound
	}
	return in, nil
}

// NonTerminalFeatureCount counts features not in a terminal state across the
// initiative's whole subtree — the input to gate G5.
func NonTerminalFeatureCount(ctx context.Context, q Querier, initiativeID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM initiatives WHERE id = $1
			UNION ALL
			SELECT i.id FROM initiatives i JOIN subtree s ON i.parent_id = s.id
		)
		SELECT count(*) FROM features f
		JOIN subtree s ON f.initiative_id = s.id
		WHERE f.state NOT IN ('done', 'abandoned')`, initiativeID).Scan(&n)
	return n, err
}

func ArchiveInitiative(ctx context.Context, tx pgx.Tx, id uuid.UUID, actor, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE initiatives SET archived = true WHERE id = $1 AND NOT archived`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return Audit(ctx, tx, actor, "initiative.archived", "initiative", &id,
		map[string]any{"reason": reason})
}

type Feature struct {
	ID           uuid.UUID
	InitiativeID uuid.UUID
	Slug         string
	Name         string
	Description  string
	State        lifecycle.FeatureState
	CreatedAt    time.Time
}

func CreateFeature(ctx context.Context, tx pgx.Tx, initiativeID uuid.UUID, slug, name, description, actor string) (*Feature, error) {
	f := &Feature{ID: NewID(), InitiativeID: initiativeID, Slug: slug, Name: name, Description: description, State: lifecycle.FeatIdea}
	_, err := tx.Exec(ctx, `
		INSERT INTO features (id, initiative_id, slug, name, description)
		VALUES ($1, $2, $3, $4, $5)`,
		f.ID, initiativeID, slug, name, description)
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "feature.created", "feature", &f.ID,
		map[string]any{"slug": slug, "name": name, "initiative_id": initiativeID.String()}); err != nil {
		return nil, err
	}
	return f, nil
}

func GetFeature(ctx context.Context, q Querier, id uuid.UUID) (*Feature, error) {
	var f Feature
	err := q.QueryRow(ctx, `
		SELECT id, initiative_id, slug, name, description, state, created_at
		FROM features WHERE id = $1`, id).
		Scan(&f.ID, &f.InitiativeID, &f.Slug, &f.Name, &f.Description, &f.State, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}

// TransitionFeature applies a lifecycle event, writing the audit row in the
// same transaction. The lifecycle engine is the sole authority on legality.
func TransitionFeature(ctx context.Context, tx pgx.Tx, f *Feature, event lifecycle.FeatureEvent, actor string, payload map[string]any) error {
	next, err := lifecycle.FeatureTransition(f.State, event)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE features SET state = $2 WHERE id = $1`, f.ID, next); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["from"] = string(f.State)
	payload["to"] = string(next)
	payload["event"] = string(event)
	if err := Audit(ctx, tx, actor, "feature.transition", "feature", &f.ID, payload); err != nil {
		return err
	}
	f.State = next
	return nil
}

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// InitiativeAncestors returns the chain from the given initiative up to the
// root, nearest first — prompt assembly walks this for ancestor documents
// (vision §10).
func InitiativeAncestors(ctx context.Context, q Querier, id uuid.UUID) ([]Initiative, error) {
	rows, err := q.Query(ctx, `
		WITH RECURSIVE chain AS (
			SELECT *, 0 AS depth FROM initiatives WHERE id = $1
			UNION ALL
			SELECT i.*, c.depth + 1 FROM initiatives i JOIN chain c ON i.id = c.parent_id
		)
		SELECT id, parent_id, slug, name, description, archived, created_at
		FROM chain ORDER BY depth`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Initiative
	for rows.Next() {
		var in Initiative
		if err := rows.Scan(&in.ID, &in.ParentID, &in.Slug, &in.Name, &in.Description, &in.Archived, &in.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}
