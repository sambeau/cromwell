package store

import (
	"context"

	"github.com/google/uuid"
)

// Reads behind a spike's page (SPEC-021 FR-8.1).

// SpikeHistory returns the audit rows that make up a spike's own timeline,
// oldest first: created, started, ended, the worktree discarded, code kept
// and closed. The run's own dispatch rows are left out; the page links the
// run instead.
func SpikeHistory(ctx context.Context, q Querier, id uuid.UUID) ([]AuditEvent, error) {
	rows, err := q.Query(ctx, `
		SELECT id, occurred_at, actor, kind, ref_type, ref_id, payload
		FROM audit_events
		WHERE ref_type = 'spike' AND ref_id = $1 AND kind LIKE 'spike.%'
		ORDER BY occurred_at, id`, id)
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
	return out, rows.Err()
}

// SpikeFollowedBy returns the spike that asks again after this one, or
// ErrNotFound when none does.
func SpikeFollowedBy(ctx context.Context, q Querier, id uuid.UUID) (*Spike, error) {
	return scanSpike(q.QueryRow(ctx, `SELECT `+spikeCols+` FROM spikes s
		WHERE s.follows_id = $1 ORDER BY s.created_at, s.id LIMIT 1`, id))
}
