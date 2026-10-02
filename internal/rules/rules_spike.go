package rules

// Spikes (SPEC-021 FR-6.1). A run-spike dispatch ends its spike when it
// succeeds, with finish_spike's findings or a stop outcome, and when it is
// exhausted, instead of the dispatch-failure checkpoint: nobody is asked to
// retry a spike, because its budget is the stop. There is no action here that
// merges anything, and none that starts a spike (SD-2, NFR-4).

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/bus"
)

// EndSpike ends a spike's run: the findings are written up from the draft
// (or, for a run that concluded, from Findings), the spike moves to ended, and
// the worktree is discarded. Exhausted says the run failed all its attempts,
// so the server marks it cancelled and the retry sweep stops republishing it.
type EndSpike struct {
	SpikeID    uuid.UUID
	DispatchID uuid.UUID
	// How is concluded, budget, turn_limit or failed.
	How string
	// Findings is finish_spike's final text, when the run concluded.
	Findings string
	// Note is a failure's last error.
	Note      string
	Exhausted bool
}

func (EndSpike) ActionKind() string { return "end_spike" }

// SpikeEnding reads how a run-spike dispatch ended from its outcome: a
// finish_spike body is a conclusion, and a stop outcome names its reason.
func SpikeEnding(raw json.RawMessage) (how, findings string, err error) {
	var o struct {
		Findings string `json:"findings"`
		Ended    string `json:"ended"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return "", "", fmt.Errorf("spike outcome: %w", err)
	}
	switch {
	case strings.TrimSpace(o.Findings) != "":
		return "concluded", o.Findings, nil
	case o.Ended == "budget" || o.Ended == "turn_limit":
		return o.Ended, "", nil
	}
	return "", "", fmt.Errorf("spike outcome: neither findings nor a reason the run stopped")
}

// decideSpikeSucceeded routes a run-spike success. A spike's run never merges
// or approves anything; the only thing it can lead to is its own end.
func decideSpikeSucceeded(e bus.DispatchSucceeded) []Action {
	if e.RefType != "spike" {
		return nil
	}
	a := EndSpike{SpikeID: e.RefID, DispatchID: e.DispatchID}
	how, findings, err := SpikeEnding(e.Outcome)
	if err != nil {
		a.How, a.Note = "failed", "The run's outcome couldn't be read."
		return []Action{a}
	}
	a.How, a.Findings = how, findings
	return []Action{a}
}

// decideSpikeExhausted routes a run-spike that failed all its attempts.
func decideSpikeExhausted(e bus.DispatchExhausted) []Action {
	if e.RefType != "spike" {
		return nil
	}
	return []Action{EndSpike{
		SpikeID: e.RefID, DispatchID: e.DispatchID, How: "failed", Note: e.Error, Exhausted: true,
	}}
}
