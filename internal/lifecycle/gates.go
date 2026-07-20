package lifecycle

import "fmt"

// Gates are named, pure functions over current state (DESIGN-003 §8). Every
// evaluation is audited by the caller as `gate.evaluated` with the returned
// reason. Phase 1 ships G1 and G5 (SPEC-001 §2); G2/G3 arrive with tasks and
// verification in phase 2, G4 with milestones in phase 3.

type Gate string

const (
	GateG1 Gate = "G1" // contract-approved: guards feature idea → ready
	GateG5 Gate = "G5" // initiative-archivable
)

type GateResult struct {
	Gate   Gate
	Pass   bool
	Reason string // human-readable; becomes the audit payload
}

// G1 — contract-approved. Phase 1 passes devPlanRequired=false (SPEC-001
// D-1); phase 2 flips it to true. Same expression, extended, exactly as the
// scope decision promises.
func G1(specApproved, devPlanRequired, devPlanApproved bool) GateResult {
	pass := specApproved && (!devPlanRequired || devPlanApproved)
	reason := "current spec approved"
	switch {
	case !specApproved:
		reason = "current spec not approved"
	case devPlanRequired && !devPlanApproved:
		reason = "current dev-plan not approved"
	case devPlanRequired:
		reason = "current spec and dev-plan approved"
	}
	return GateResult{Gate: GateG1, Pass: pass, Reason: reason}
}

// G5 — initiative-archivable: no non-terminal features in the subtree.
// Failure is overridable only via an answered gate-override checkpoint (L-6).
func G5(nonTerminalFeatures int) GateResult {
	if nonTerminalFeatures > 0 {
		return GateResult{
			Gate:   GateG5,
			Pass:   false,
			Reason: fmt.Sprintf("%d non-terminal feature(s) in subtree", nonTerminalFeatures),
		}
	}
	return GateResult{Gate: GateG5, Pass: true, Reason: "no non-terminal features in subtree"}
}
