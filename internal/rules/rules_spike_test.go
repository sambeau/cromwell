package rules

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/bus"
)

// A run-spike success ends its spike, how the outcome says, and nothing else.
func TestRunSpikeSuccessEndsTheSpike(t *testing.T) {
	spike, run := uuid.New(), uuid.New()
	cases := []struct {
		name, outcome, how, findings string
	}{
		{"budget", `{"ended":"budget"}`, "budget", ""},
		{"turn limit", `{"ended":"turn_limit"}`, "turn_limit", ""},
		{"concluded", `{"findings":"## Answer\n\nYes.\n"}`, "concluded", "## Answer\n\nYes.\n"},
		{"unreadable", `{"nonsense":1}`, "failed", ""},
	}
	for _, c := range cases {
		acts := Decide(bus.DispatchSucceeded{DispatchID: run, Purpose: "run-spike", Role: "spike-runner",
			RefType: "spike", RefID: spike, Outcome: json.RawMessage(c.outcome)}, Snapshot{})
		if len(acts) != 1 {
			t.Fatalf("%s: %d actions", c.name, len(acts))
		}
		end, ok := acts[0].(EndSpike)
		if !ok {
			t.Fatalf("%s: action is %T, want EndSpike", c.name, acts[0])
		}
		if end.SpikeID != spike || end.DispatchID != run || end.How != c.how || end.Findings != c.findings || end.Exhausted {
			t.Errorf("%s: %+v", c.name, end)
		}
	}
}

// An exhausted run-spike ends its spike as failed, with no retry question.
func TestExhaustedRunSpikeRaisesNoCheckpoint(t *testing.T) {
	spike, run := uuid.New(), uuid.New()
	acts := Decide(bus.DispatchExhausted{DispatchID: run, Purpose: "run-spike", RefType: "spike", RefID: spike, Error: "boom"}, Snapshot{})
	if len(acts) != 1 {
		t.Fatalf("%d actions", len(acts))
	}
	end, ok := acts[0].(EndSpike)
	if !ok || end.How != "failed" || end.Note != "boom" || !end.Exhausted || end.SpikeID != spike || end.DispatchID != run {
		t.Errorf("action = %#v", acts[0])
	}
	// Any other purpose still asks.
	other := Decide(bus.DispatchExhausted{DispatchID: run, Purpose: "implement-task", RefType: "task", RefID: spike}, Snapshot{})
	if len(other) != 1 {
		t.Fatalf("%d actions", len(other))
	}
	if _, ok := other[0].(RaiseCheckpoint); !ok {
		t.Errorf("an exhausted implementer led to %T", other[0])
	}
}

func TestSpikeEndingReadsOutcomes(t *testing.T) {
	if how, f, err := SpikeEnding(json.RawMessage(`{"findings":"  "}`)); err == nil {
		t.Errorf("blank findings read as %q %q", how, f)
	}
	if _, _, err := SpikeEnding(json.RawMessage(`{"ended":"elsewhere"}`)); err == nil {
		t.Error("an unknown reason was accepted")
	}
	if _, _, err := SpikeEnding(json.RawMessage(`not json`)); err == nil {
		t.Error("bad JSON was accepted")
	}
}
