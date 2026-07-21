package store

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"cromwell/internal/sizing"
)

// Actuals and the calibration corpus (SPEC-003 FR-3, DESIGN-001 §7). Actual
// token consumption is summed from the dispatch ledger by owner and descendant
// ownership; the corpus (description, latest estimate, summed actuals) is a
// view over that ledger, not a stored table. The `considered` tier retrieves
// the corpus by Postgres full-text rank over entity descriptions (SD-1); no
// embeddings, and features/tasks are left unaltered (NFR-3) — the tsvector is
// computed at query time.

// tokenSum totals every token class of a succeeded dispatch. Kept as one
// fragment so actuals and cost roll-ups count identically.
const tokenSum = `COALESCE(input_tokens,0)+COALESCE(output_tokens,0)+COALESCE(cache_read_tokens,0)+COALESCE(cache_write_tokens,0)`

// ActualTokens sums the tokens an entity's completed dispatches consumed. A
// feature includes its tasks' dispatches; an initiative includes its whole
// subtree (FR-3.1). Only succeeded dispatches carry usage.
func ActualTokens(ctx context.Context, q Querier, refType string, refID uuid.UUID) (int64, error) {
	var total int64
	var err error
	switch refType {
	case "task":
		err = q.QueryRow(ctx, `
			SELECT COALESCE(SUM(`+tokenSum+`),0) FROM dispatches
			WHERE state='succeeded' AND ref_type='task' AND ref_id=$1`, refID).Scan(&total)
	case "feature":
		err = q.QueryRow(ctx, `
			SELECT COALESCE(SUM(`+tokenSum+`),0) FROM dispatches
			WHERE state='succeeded' AND (
				(ref_type='feature' AND ref_id=$1) OR
				(ref_type='task' AND ref_id IN (SELECT id FROM tasks WHERE feature_id=$1))
			)`, refID).Scan(&total)
	case "initiative":
		err = q.QueryRow(ctx, `
			WITH RECURSIVE subtree AS (
				SELECT id FROM initiatives WHERE id=$1
				UNION ALL
				SELECT i.id FROM initiatives i JOIN subtree s ON i.parent_id=s.id
			), feats AS (SELECT id FROM features WHERE initiative_id IN (SELECT id FROM subtree))
			SELECT COALESCE(SUM(`+tokenSum+`),0) FROM dispatches
			WHERE state='succeeded' AND (
				(ref_type='feature' AND ref_id IN (SELECT id FROM feats)) OR
				(ref_type='task' AND ref_id IN (SELECT id FROM tasks WHERE feature_id IN (SELECT id FROM feats)))
			)`, refID).Scan(&total)
	default:
		return 0, errors.New("actuals only defined for task, feature, initiative")
	}
	return total, err
}

// CorpusRow is one reference point: a completed entity's description, its
// current estimate, and what it actually consumed (DESIGN-001 §7).
type CorpusRow struct {
	RefType        string
	RefID          uuid.UUID
	Name           string
	Description    string
	EstimateTokens int64
	EstimateTier   sizing.Tier
	ActualTokens   int64
	Rank           float64
}

// RetrieveCorpus returns the nearest prior estimated-and-completed entities to
// the given description, by full-text rank over descriptions (FR-3.2, SD-1).
// Only done entities that carry an estimate qualify — a reference point needs
// both an estimate and an actual to anchor anything. Actuals are summed per
// row (the neighbour set is small, so a per-row sum is fine and exact).
func RetrieveCorpus(ctx context.Context, q Querier, description string, limit int) ([]CorpusRow, error) {
	if limit <= 0 {
		limit = 5
	}
	// Nearest-neighbour retrieval matches on term *overlap*, not exact
	// containment: plainto_tsquery ANDs the description's terms (a neighbour
	// would need every word), so we rewrite it to an OR query over the same
	// normalized lexemes and rank by ts_rank. An empty description yields an
	// empty query that matches nothing (→ rough, honestly).
	rows, err := q.Query(ctx, `
		WITH qq AS (
			SELECT replace(plainto_tsquery('english', $1)::text, '&', '|')::tsquery AS query
		)
		SELECT ref_type, ref_id, name, description, rank FROM (
			SELECT 'feature' AS ref_type, f.id AS ref_id, f.name, f.description,
			       ts_rank(to_tsvector('english', f.description), qq.query) AS rank
			FROM features f, qq
			WHERE f.state='done' AND f.description <> ''
			  AND to_tsvector('english', f.description) @@ qq.query
			  AND EXISTS (SELECT 1 FROM estimates e WHERE e.ref_type='feature' AND e.ref_id=f.id)
			UNION ALL
			SELECT 'task' AS ref_type, t.id AS ref_id, t.title AS name, t.description,
			       ts_rank(to_tsvector('english', t.description), qq.query) AS rank
			FROM tasks t, qq
			WHERE t.state='done' AND t.description <> ''
			  AND to_tsvector('english', t.description) @@ qq.query
			  AND EXISTS (SELECT 1 FROM estimates e WHERE e.ref_type='task' AND e.ref_id=t.id)
		) c
		ORDER BY rank DESC, ref_id DESC
		LIMIT $2`, description, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CorpusRow
	for rows.Next() {
		var r CorpusRow
		if err := rows.Scan(&r.RefType, &r.RefID, &r.Name, &r.Description, &r.Rank); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Fill in each neighbour's current estimate and summed actual.
	for i := range out {
		est, err := CurrentEstimate(ctx, q, out[i].RefType, out[i].RefID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if est != nil {
			out[i].EstimateTokens = est.Tokens
			out[i].EstimateTier = est.Tier
		}
		actual, err := ActualTokens(ctx, q, out[i].RefType, out[i].RefID)
		if err != nil {
			return nil, err
		}
		out[i].ActualTokens = actual
	}
	return out, nil
}
