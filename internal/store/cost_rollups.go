package store

import (
	"context"

	"github.com/google/uuid"
)

// Extended cost roll-ups (SPEC-003 FR-7). Phase 1 already rolls cost per
// task/feature (CostRollup, grouped by owning entity). Phase 3 extends the
// same ledger — with its frozen per-dispatch price snapshots (phase 1 O-4) —
// to per-initiative (transitive), per-milestone (over resolved members),
// per-roadmap, and per calendar month. Every roll-up reads succeeded
// dispatches only, where cost_usd is set.

// InitiativeCost sums cost over an initiative's whole subtree: dispatches on
// its descendant features and their tasks, transitively (FR-7.1).
func InitiativeCost(ctx context.Context, q Querier, initiativeID uuid.UUID) (float64, error) {
	var cost float64
	err := q.QueryRow(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM initiatives WHERE id=$1
			UNION ALL
			SELECT i.id FROM initiatives i JOIN subtree s ON i.parent_id=s.id
		), feats AS (SELECT id FROM features WHERE initiative_id IN (SELECT id FROM subtree))
		SELECT COALESCE(SUM(cost_usd),0) FROM dispatches
		WHERE state='succeeded' AND cost_usd IS NOT NULL AND (
			(ref_type='feature' AND ref_id IN (SELECT id FROM feats)) OR
			(ref_type='task' AND ref_id IN (SELECT id FROM tasks WHERE feature_id IN (SELECT id FROM feats)))
		)`, initiativeID).Scan(&cost)
	return cost, err
}

// FeatureCost sums cost over a feature and its tasks' dispatches.
func FeatureCost(ctx context.Context, q Querier, featureID uuid.UUID) (float64, error) {
	var cost float64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_usd),0) FROM dispatches
		WHERE state='succeeded' AND cost_usd IS NOT NULL AND (
			(ref_type='feature' AND ref_id=$1) OR
			(ref_type='task' AND ref_id IN (SELECT id FROM tasks WHERE feature_id=$1))
		)`, featureID).Scan(&cost)
	return cost, err
}

// MilestoneCost sums cost over a milestone's resolved leaf features (each
// feature transitive to its tasks). Live for open milestones, snapshot leaves
// for locked ones — the caller passes the leaf set it wants (FR-7.1).
func MilestoneCost(ctx context.Context, q Querier, leafFeatureIDs []uuid.UUID) (float64, error) {
	if len(leafFeatureIDs) == 0 {
		return 0, nil
	}
	var cost float64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_usd),0) FROM dispatches
		WHERE state='succeeded' AND cost_usd IS NOT NULL AND (
			(ref_type='feature' AND ref_id = ANY($1)) OR
			(ref_type='task' AND ref_id IN (SELECT id FROM tasks WHERE feature_id = ANY($1)))
		)`, leafFeatureIDs).Scan(&cost)
	return cost, err
}

// RoadmapCost sums cost over every milestone in a roadmap, resolving each
// milestone's leaves live (open) or from its snapshot (locked).
func RoadmapCost(ctx context.Context, q Querier, roadmapID uuid.UUID) (float64, error) {
	entries, err := RoadmapEntries(ctx, q, roadmapID)
	if err != nil {
		return 0, err
	}
	seen := map[uuid.UUID]bool{}
	var leaves []uuid.UUID
	for _, e := range entries {
		ms, err := GetMilestone(ctx, q, e.MilestoneID)
		if err != nil {
			return 0, err
		}
		var ls []uuid.UUID
		if ms.State == "locked" {
			p, err := SnapshotProgress(ctx, q, e.MilestoneID)
			if err != nil {
				return 0, err
			}
			ls = p.Leaves
		} else {
			ls, err = ResolveMembers(ctx, q, e.MilestoneID)
			if err != nil {
				return 0, err
			}
		}
		for _, l := range ls {
			if !seen[l] {
				seen[l] = true
				leaves = append(leaves, l)
			}
		}
	}
	return MilestoneCost(ctx, q, leaves)
}

// MonthCost is one calendar month's total dispatch cost.
type MonthCost struct {
	Month   string // 'YYYY-MM'
	CostUSD float64
	Count   int
}

// CostByMonth rolls cost up per calendar month from the dispatch ledger,
// newest month first (FR-7.1). Months are bucketed by queued_at (when the
// spend was incurred), in UTC.
func CostByMonth(ctx context.Context, q Querier) ([]MonthCost, error) {
	rows, err := q.Query(ctx, `
		SELECT to_char(date_trunc('month', queued_at AT TIME ZONE 'UTC'), 'YYYY-MM') AS month,
		       COALESCE(SUM(cost_usd),0), count(*)
		FROM dispatches
		WHERE state='succeeded' AND cost_usd IS NOT NULL
		GROUP BY month
		ORDER BY month DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MonthCost
	for rows.Next() {
		var m MonthCost
		if err := rows.Scan(&m.Month, &m.CostUSD, &m.Count); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
