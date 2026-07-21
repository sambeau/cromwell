package lifecycle

import "fmt"

// Gates are named, pure functions over current state (DESIGN-003 §8). Every
// evaluation is audited by the caller as `gate.evaluated` with the returned
// reason. Phase 1 shipped G1 and G5 (SPEC-001 §2); phase 2 adds G2 (tasks
// complete) and G3 (verified). G4 (milestone-lockable) arrives with phase 3.

type Gate string

const (
	GateG1 Gate = "G1" // contract-approved: guards feature idea → ready
	GateG2 Gate = "G2" // tasks-complete: guards feature active → review
	GateG3 Gate = "G3" // verified: guards feature review → done
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

// G2 — tasks-complete. Guards feature active → review: every task done or
// abandoned, at least one done, and the contract not stale (DESIGN-003 §8).
// spec_stale being true is the one brake that holds a feature open while a
// human decides what a mid-flight revision means (DESIGN-005 §6).
func G2(totalTasks, doneTasks, terminalTasks int, specStale bool) GateResult {
	switch {
	case specStale:
		return GateResult{Gate: GateG2, Pass: false, Reason: "contract is stale (revision in flight)"}
	case totalTasks == 0:
		return GateResult{Gate: GateG2, Pass: false, Reason: "feature has no tasks"}
	case terminalTasks < totalTasks:
		return GateResult{Gate: GateG2, Pass: false,
			Reason: fmt.Sprintf("%d of %d task(s) not yet done or abandoned", totalTasks-terminalTasks, totalTasks)}
	case doneTasks == 0:
		return GateResult{Gate: GateG2, Pass: false, Reason: "no task completed (all abandoned)"}
	}
	return GateResult{Gate: GateG2, Pass: true, Reason: "all tasks complete"}
}

// G3 — verified. Guards feature review → done: the latest verification
// dispatch approved AND the branch is merged (DESIGN-003 §8). The merge is a
// side effect of processing the approve verdict (DESIGN-006 §6), so by the
// time this gate is evaluated for the transition, both terms hold.
func G3(verificationApproved, branchMerged bool) GateResult {
	switch {
	case !verificationApproved:
		return GateResult{Gate: GateG3, Pass: false, Reason: "verification not approved"}
	case !branchMerged:
		return GateResult{Gate: GateG3, Pass: false, Reason: "branch not merged"}
	}
	return GateResult{Gate: GateG3, Pass: true, Reason: "verified and merged"}
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
