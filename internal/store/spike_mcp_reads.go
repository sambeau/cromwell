package store

import (
	"context"

	"github.com/google/uuid"
)

// SpikesFollowing returns the spikes that ask again for the given one
// (follows_id), oldest first. The MCP read tools show them as "followed by".
func SpikesFollowing(ctx context.Context, q Querier, id uuid.UUID) ([]Spike, error) {
	return querySpikes(ctx, q, "s.follows_id = $1", "s.created_at, s.id", id)
}
