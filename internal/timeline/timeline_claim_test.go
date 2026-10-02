package timeline

import (
	"testing"

	"github.com/google/uuid"
)

// TestSpikeClaimStaleMoment is SPEC-021 FR-14.2: the checkpoint moment for a
// spike's stale claim asks about a spike, and a task's keeps its words.
func TestSpikeClaimStaleMoment(t *testing.T) {
	spike := ev(1, "checkpoint.created", map[string]any{"kind": "claim-stale"})
	spike.RefType, spike.RefID = "spike", uuid.New()
	task := ev(2, "checkpoint.created", map[string]any{"kind": "claim-stale"})
	task.RefType, task.RefID = "task", uuid.New()
	ms := Build([]Event{ev(0, "feature.created", nil), spike, task}, nil, Options{})
	if got, want := labels(ms), "Created · Waiting for a person: is someone still working on a spike · Waiting for a person: is someone still working on a task"; got != want {
		t.Fatalf("moments = %q, want %q", got, want)
	}
}
