package rules

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"subutai/internal/bus"
)

// Phase-3 rule additions: the AI estimator outcome (SPEC-003 FR-4). The
// `estimate` dispatch is read-only — its single side effect is recording an
// estimate row from the outcome tool. The confidence tier is assigned by the
// server from the corpus evidence, not by the agent (FR-1.2), so the outcome
// carries only tokens and rationale.

// RecordAIEstimate records an estimate produced by the estimator dispatch. The
// server decides the tier (considered when the corpus informed it, rough when
// empty) and writes the row with the estimating dispatch id (FR-4.1).
type RecordAIEstimate struct {
	RefType    string
	RefID      uuid.UUID
	DispatchID uuid.UUID
	Tokens     int64
	Rationale  string
	Actor      string
}

func (RecordAIEstimate) ActionKind() string { return "record_ai_estimate" }

// EstimateOutcome is the submit_estimate outcome-tool payload.
type EstimateOutcome struct {
	Tokens    int64  `json:"tokens"`
	Rationale string `json:"rationale"`
}

func ParseEstimateOutcome(raw json.RawMessage) (*EstimateOutcome, error) {
	var o EstimateOutcome
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("estimate outcome: %w", err)
	}
	if o.Tokens <= 0 {
		return nil, fmt.Errorf("estimate outcome: tokens must be positive, got %d", o.Tokens)
	}
	return &o, nil
}

// EstimateIdempotencyKey keys an estimate dispatch by how many the entity
// already has, so a crash/replay of one request is a no-op while a deliberate
// re-estimate (after the prior succeeded) gets a fresh key and a new row
// (mirrors the phase-2 implement key, DESIGN-006 §7).
func EstimateIdempotencyKey(refType string, refID uuid.UUID, priorEstimates int) string {
	return fmt.Sprintf("estimate:%s:%s:%d", refType, refID, priorEstimates)
}

// decideDispatchSucceededPhase3 routes the estimate dispatch outcome. Returns
// (nil, false) for any other purpose so the phase-1/2 paths run.
func decideDispatchSucceededPhase3(e bus.DispatchSucceeded) ([]Action, bool) {
	if e.Purpose != "estimate" {
		return nil, false
	}
	o, err := ParseEstimateOutcome(e.Outcome)
	if err != nil {
		return []Action{unparseableCheckpoint(e.RefType, e.RefID, e.DispatchID, err)}, true
	}
	return []Action{RecordAIEstimate{
		RefType: e.RefType, RefID: e.RefID, DispatchID: e.DispatchID,
		Tokens: o.Tokens, Rationale: o.Rationale, Actor: e.Role,
	}}, true
}
