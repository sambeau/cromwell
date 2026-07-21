package rules

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"cromwell/internal/bus"
)

func TestEstimateOutcomeRecordsEstimate(t *testing.T) {
	ref := uuid.New()
	did := uuid.New()
	ev := bus.DispatchSucceeded{
		DispatchID: did, Purpose: "estimate", Role: "estimator",
		RefType: "feature", RefID: ref,
		Outcome: json.RawMessage(`{"tokens": 4200, "rationale": "similar to FEAT-x"}`),
	}
	actions := Decide(ev, Snapshot{})
	if len(actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(actions))
	}
	rec, ok := actions[0].(RecordAIEstimate)
	if !ok {
		t.Fatalf("action = %T, want RecordAIEstimate", actions[0])
	}
	if rec.Tokens != 4200 || rec.RefType != "feature" || rec.RefID != ref || rec.DispatchID != did || rec.Actor != "estimator" {
		t.Errorf("record = %+v", rec)
	}
	if rec.Rationale != "similar to FEAT-x" {
		t.Errorf("rationale = %q", rec.Rationale)
	}
}

func TestEstimateOutcomeUnparseableEscalates(t *testing.T) {
	ev := bus.DispatchSucceeded{
		DispatchID: uuid.New(), Purpose: "estimate", Role: "estimator",
		RefType: "feature", RefID: uuid.New(),
		Outcome: json.RawMessage(`{"tokens": 0, "rationale": "nope"}`), // non-positive tokens
	}
	actions := Decide(ev, Snapshot{})
	if len(actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(actions))
	}
	if _, ok := actions[0].(RaiseCheckpoint); !ok {
		t.Fatalf("action = %T, want RaiseCheckpoint for a bad outcome", actions[0])
	}
}

func TestEstimateIdempotencyKeyDistinguishesReestimates(t *testing.T) {
	ref := uuid.New()
	k0 := EstimateIdempotencyKey("feature", ref, 0)
	k1 := EstimateIdempotencyKey("feature", ref, 1)
	if k0 == k1 {
		t.Errorf("re-estimate should get a fresh key: %s == %s", k0, k1)
	}
}
