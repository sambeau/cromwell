package store

import (
	"context"

	"github.com/google/uuid"
)

// Additive reads the browser command centre needs and the CLI never did
// (DESIGN-007 §8, CC-7; SPEC-004 FR-8.1). All are read-only over the existing
// tables — no schema change, no new authority.

const initiativeCols = `id, parent_id, slug, name, description, archived, created_at`

func scanInitiative(rows interface {
	Scan(dest ...any) error
}) (*Initiative, error) {
	var in Initiative
	if err := rows.Scan(&in.ID, &in.ParentID, &in.Slug, &in.Name, &in.Description,
		&in.Archived, &in.CreatedAt); err != nil {
		return nil, err
	}
	return &in, nil
}

// RootInitiatives lists the top-level (parent-less) initiatives, archived ones
// last, so the planning tree can be rendered from its roots. Each root is the
// entry point for InitiativeSizingNode + sizing.RollUp.
func RootInitiatives(ctx context.Context, q Querier) ([]Initiative, error) {
	rows, err := q.Query(ctx, `
		SELECT `+initiativeCols+` FROM initiatives
		WHERE parent_id IS NULL
		ORDER BY archived, slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Initiative
	for rows.Next() {
		in, err := scanInitiative(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *in)
	}
	return out, rows.Err()
}

// ChildInitiatives lists an initiative's direct sub-initiatives, ordered by
// slug — the tree walk the planning view needs for breadcrumbs and nesting.
func ChildInitiatives(ctx context.Context, q Querier, parentID uuid.UUID) ([]Initiative, error) {
	rows, err := q.Query(ctx, `
		SELECT `+initiativeCols+` FROM initiatives
		WHERE parent_id = $1
		ORDER BY archived, slug`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Initiative
	for rows.Next() {
		in, err := scanInitiative(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *in)
	}
	return out, rows.Err()
}

// FeaturesForInitiative lists an initiative's own features, ordered by slug.
func FeaturesForInitiative(ctx context.Context, q Querier, initiativeID uuid.UUID) ([]Feature, error) {
	rows, err := q.Query(ctx, `
		SELECT id, initiative_id, slug, name, description, state, created_at
		FROM features WHERE initiative_id = $1
		ORDER BY slug`, initiativeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Feature
	for rows.Next() {
		var f Feature
		if err := rows.Scan(&f.ID, &f.InitiativeID, &f.Slug, &f.Name, &f.Description,
			&f.State, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListLiveDocuments lists every non-superseded document across the tree, newest
// first — the Documents view's index in one read instead of N owner lookups
// (FR-8.1).
func ListLiveDocuments(ctx context.Context, q Querier) ([]Document, error) {
	rows, err := q.Query(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE state <> 'superseded'
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// MilestoneProgress pairs a milestone with its resolved progress: live for open
// milestones, the frozen snapshot for locked ones (FR-4.2). Assembled here so
// the Planning view makes one call instead of N.
type MilestoneProgress struct {
	Milestone Milestone
	Progress  Progress
	Locked    bool
}

// MilestonesWithProgress returns every milestone with its progress resolved the
// honest way — LiveProgress for open milestones, SnapshotProgress for locked
// ones — in one aggregate read (FR-8.1).
func MilestonesWithProgress(ctx context.Context, q Querier) ([]MilestoneProgress, error) {
	ms, err := ListMilestones(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]MilestoneProgress, 0, len(ms))
	for _, m := range ms {
		locked := m.LockedAt != nil
		var prog Progress
		if locked {
			prog, err = SnapshotProgress(ctx, q, m.ID)
		} else {
			prog, err = LiveProgress(ctx, q, m.ID)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, MilestoneProgress{Milestone: m, Progress: prog, Locked: locked})
	}
	return out, nil
}

// RecentCalibration lists the most recently estimated feature/task entities
// that are done and carry an actual, each with its current estimate and summed
// actual — the dashboard's "calibration health: recent estimate-vs-actual
// deltas" (FR-2.1). Ordered by estimate recency, which is the honest floor for
// "recent"; the delta itself is estimate vs actual, computed at render.
func RecentCalibration(ctx context.Context, q Querier, limit int) ([]CorpusRow, error) {
	if limit <= 0 {
		limit = 5
	}
	// The current estimate per entity is its newest row (created_at DESC, id
	// DESC, matching estimates_ref). Restrict to done entities so an actual has
	// been incurred, most-recently-estimated first.
	rows, err := q.Query(ctx, `
		WITH current AS (
			SELECT DISTINCT ON (e.ref_type, e.ref_id)
			       e.ref_type, e.ref_id, e.tokens, e.tier, e.created_at
			FROM estimates e
			ORDER BY e.ref_type, e.ref_id, e.created_at DESC, e.id DESC
		)
		SELECT c.ref_type, c.ref_id,
		       CASE c.ref_type WHEN 'feature' THEN f.name ELSE t.title END AS name,
		       c.tokens, c.tier
		FROM current c
		LEFT JOIN features f ON c.ref_type='feature' AND f.id=c.ref_id AND f.state='done'
		LEFT JOIN tasks    t ON c.ref_type='task'    AND t.id=c.ref_id AND t.state='done'
		WHERE (c.ref_type='feature' AND f.id IS NOT NULL)
		   OR (c.ref_type='task'    AND t.id IS NOT NULL)
		ORDER BY c.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CorpusRow
	for rows.Next() {
		var r CorpusRow
		if err := rows.Scan(&r.RefType, &r.RefID, &r.Name, &r.EstimateTokens, &r.EstimateTier); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		actual, err := ActualTokens(ctx, q, out[i].RefType, out[i].RefID)
		if err != nil {
			return nil, err
		}
		out[i].ActualTokens = actual
	}
	return out, nil
}

// ListRoadmaps lists every roadmap, name order — the Cost and Planning views'
// roadmap index (FR-4.2, FR-6.1).
func ListRoadmaps(ctx context.Context, q Querier) ([]Roadmap, error) {
	rows, err := q.Query(ctx, `SELECT `+roadmapCols+` FROM roadmaps ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Roadmap
	for rows.Next() {
		rm, err := scanRoadmap(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rm)
	}
	return out, rows.Err()
}

// RunningDispatches lists the dispatches currently executing — the in-flight
// half of the dashboard queue (the queued half is QueuedDispatches).
func (s *Store) RunningDispatches(ctx context.Context) ([]Dispatch, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+dispatchCols+` FROM dispatches WHERE state = 'running'
		ORDER BY started_at`)
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
