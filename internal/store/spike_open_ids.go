package store

import (
	"context"

	"github.com/google/uuid"
)

// OpenSpikeIDsUnderInitiative names the spikes not yet closed across the
// initiative's whole subtree, oldest first, by their public IDs: G5's refusal
// says which they are (SPEC-021 SD-14).
func OpenSpikeIDsUnderInitiative(ctx context.Context, q Querier, initiativeID uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM initiatives WHERE id = $1
			UNION ALL
			SELECT i.id FROM initiatives i JOIN subtree s ON i.parent_id = s.id
		)
		SELECT sp.public_id FROM spikes sp
		JOIN subtree s ON sp.initiative_id = s.id
		WHERE sp.state <> 'closed'
		ORDER BY sp.created_at, sp.id`, initiativeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
