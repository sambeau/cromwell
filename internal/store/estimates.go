package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/sizing"
)

// Estimate is one stored estimate row (DESIGN-001 §7). Multiple rows per
// entity are allowed; the latest (created_at, then id) is current, prior rows
// are the re-estimation history (FR-1.1). Roll-ups are never stored — they are
// computed by the sizing engine over CurrentEstimate values (DESIGN-001 §7).
type Estimate struct {
	ID         uuid.UUID
	RefType    string
	RefID      uuid.UUID
	Tokens     int64
	Tier       sizing.Tier
	Rationale  string
	DispatchID *uuid.UUID
	CreatedAt  time.Time
}

// RecordEstimate inserts an estimate row and its audit event in the caller's
// transaction (O-3). The tier is the writer's honest assessment of the
// evidence (FR-1.2), never overridden here.
func RecordEstimate(ctx context.Context, tx pgx.Tx, refType string, refID uuid.UUID, tokens int64, tier sizing.Tier, rationale string, dispatchID *uuid.UUID, actor string) (*Estimate, error) {
	e := &Estimate{ID: NewID(), RefType: refType, RefID: refID, Tokens: tokens,
		Tier: tier, Rationale: rationale, DispatchID: dispatchID}
	_, err := tx.Exec(ctx, `
		INSERT INTO estimates (id, ref_type, ref_id, tokens, tier, rationale, dispatch_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.ID, refType, refID, tokens, string(tier), rationale, dispatchID)
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "estimate.recorded", refType, &refID,
		map[string]any{"tokens": tokens, "tier": string(tier), "rationale": rationale}); err != nil {
		return nil, err
	}
	return e, nil
}

const estimateCols = `id, ref_type, ref_id, tokens, tier, rationale, dispatch_id, created_at`

func scanEstimate(row pgx.Row) (*Estimate, error) {
	var e Estimate
	var tier string
	err := row.Scan(&e.ID, &e.RefType, &e.RefID, &e.Tokens, &tier, &e.Rationale, &e.DispatchID, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	e.Tier = sizing.Tier(tier)
	return &e, err
}

// CurrentEstimate returns the latest estimate for an entity, or ErrNotFound.
func CurrentEstimate(ctx context.Context, q Querier, refType string, refID uuid.UUID) (*Estimate, error) {
	return scanEstimate(q.QueryRow(ctx, `
		SELECT `+estimateCols+` FROM estimates
		WHERE ref_type = $1 AND ref_id = $2
		ORDER BY created_at DESC, id DESC LIMIT 1`, refType, refID))
}

// EstimateHistory returns all estimates for an entity, newest first (the
// current one leads, priors follow — FR-1.1).
func EstimateHistory(ctx context.Context, q Querier, refType string, refID uuid.UUID) ([]Estimate, error) {
	rows, err := q.Query(ctx, `
		SELECT `+estimateCols+` FROM estimates
		WHERE ref_type = $1 AND ref_id = $2
		ORDER BY created_at DESC, id DESC`, refType, refID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Estimate
	for rows.Next() {
		e, err := scanEstimate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// latestEstimates returns the current unit estimate for each of the given
// entities of one ref_type, keyed by id (entities with no estimate are
// absent). One windowed query, no N+1.
func latestEstimates(ctx context.Context, q Querier, refType string, ids []uuid.UUID) (map[uuid.UUID]sizing.UnitEstimate, error) {
	out := map[uuid.UUID]sizing.UnitEstimate{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `
		SELECT DISTINCT ON (ref_id) ref_id, tokens, tier
		FROM estimates
		WHERE ref_type = $1 AND ref_id = ANY($2)
		ORDER BY ref_id, created_at DESC, id DESC`, refType, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var tokens int64
		var tier string
		if err := rows.Scan(&id, &tokens, &tier); err != nil {
			return nil, err
		}
		out[id] = sizing.UnitEstimate{Tokens: tokens, Tier: sizing.Tier(tier)}
	}
	return out, rows.Err()
}

// FeatureSizingNode loads a feature and its tasks with their current estimates
// as a sizing tree (FR-2). The feature's own estimate is included; the sizing
// engine decides how it composes with the tasks (DESIGN-001 §7).
func FeatureSizingNode(ctx context.Context, q Querier, featureID uuid.UUID) (sizing.Node, error) {
	f, err := GetFeature(ctx, q, featureID)
	if err != nil {
		return sizing.Node{}, err
	}
	tasks, err := TasksForFeature(ctx, q, featureID)
	if err != nil {
		return sizing.Node{}, err
	}
	taskIDs := make([]uuid.UUID, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	taskEst, err := latestEstimates(ctx, q, "task", taskIDs)
	if err != nil {
		return sizing.Node{}, err
	}
	featEst, err := latestEstimates(ctx, q, "feature", []uuid.UUID{featureID})
	if err != nil {
		return sizing.Node{}, err
	}
	children := make([]sizing.Node, len(tasks))
	for i, t := range tasks {
		label := t.LocalID
		if label == "" {
			label = t.Title
		}
		n := sizing.Node{Ref: sizing.Ref{Type: "task", ID: t.ID, Name: label}}
		if e, ok := taskEst[t.ID]; ok {
			n.Estimate = &e
		}
		children[i] = n
	}
	node := sizing.Node{Ref: sizing.Ref{Type: "feature", ID: f.ID, Name: f.Slug}, Children: children}
	if e, ok := featEst[featureID]; ok {
		node.Estimate = &e
	}
	return node, nil
}

// InitiativeSizingNode loads an initiative's whole subtree (nested initiatives
// transitively, and every feature with its tasks) as a sizing tree.
// Initiatives carry no estimate of their own (schema CHECK); they roll up from
// their descendants (FR-2.1).
func InitiativeSizingNode(ctx context.Context, q Querier, initiativeID uuid.UUID) (sizing.Node, error) {
	in, err := GetInitiative(ctx, q, initiativeID)
	if err != nil {
		return sizing.Node{}, err
	}
	var children []sizing.Node

	// Nested initiatives first.
	childInits, err := childInitiativeIDs(ctx, q, initiativeID)
	if err != nil {
		return sizing.Node{}, err
	}
	for _, cid := range childInits {
		cn, err := InitiativeSizingNode(ctx, q, cid)
		if err != nil {
			return sizing.Node{}, err
		}
		children = append(children, cn)
	}

	// Then this initiative's own features.
	featIDs, err := featureIDsOfInitiative(ctx, q, initiativeID)
	if err != nil {
		return sizing.Node{}, err
	}
	for _, fid := range featIDs {
		fn, err := FeatureSizingNode(ctx, q, fid)
		if err != nil {
			return sizing.Node{}, err
		}
		children = append(children, fn)
	}
	return sizing.Node{Ref: sizing.Ref{Type: "initiative", ID: in.ID, Name: in.Slug}, Children: children}, nil
}

func childInitiativeIDs(ctx context.Context, q Querier, parentID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT id FROM initiatives WHERE parent_id = $1 ORDER BY slug`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

func featureIDsOfInitiative(ctx context.Context, q Querier, initiativeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT id FROM features WHERE initiative_id = $1 ORDER BY slug`, initiativeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

func scanIDs(rows pgx.Rows) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
