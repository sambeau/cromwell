package lifecycle

import "fmt"

// Gates are named, pure functions over current state (DESIGN-003 §8). Every
// evaluation is audited by the caller as `gate.evaluated` with the returned
// reason. Phase 1 shipped G1 and G5 (SPEC-001 §2); phase 2 adds G2 (tasks
// complete) and G3 (verified); phase 3 adds G4 (milestone-lockable).

type Gate string

const (
	GateG0 Gate = "G0" // spec-ready: a feature's design is approved, so it may be specced
	GateG1 Gate = "G1" // contract-approved: guards feature idea → ready
	GateG2 Gate = "G2" // tasks-complete: guards feature active → review
	GateG3 Gate = "G3" // verified: guards feature review → done
	GateG4 Gate = "G4" // milestone-lockable: guards milestone open → locked
	GateG5 Gate = "G5" // initiative-archivable
)

type GateResult struct {
	Gate   Gate
	Pass   bool
	Reason string // human-readable; becomes the audit payload
}

// G0 — spec-ready. A feature may be specced when its own primary design
// document is approved, or its *immediate parent* initiative's is (SPEC-009
// FR-3). Approving a design is the single act that means "ready to spec";
// there is no separate readiness flag, because a gate is an expression over
// current state (DESIGN-003 §8).
//
// Inheritance stops at one level, and that is the whole point. Product
// planning is width-first — a broad tree of half-formed sub-initiatives with
// half-written designs — so a top-level design releasing everything beneath it
// would spec work nobody has thought about yet. Each level of the tree is an
// explicit decision.
func G0(ownDesignApproved, parentDesignApproved bool) GateResult {
	pass := ownDesignApproved || parentDesignApproved
	reason := "the initiative's design is approved, so its features are ready to spec"
	switch {
	case ownDesignApproved:
		reason = "this feature's own design is approved"
	case !pass:
		reason = "no approved design — a feature is ready to spec once its own design, or its initiative's, is approved"
	}
	return GateResult{Gate: GateG0, Pass: pass, Reason: reason}
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

// G4 — milestone-lockable. Guards milestone open → locked: at least one
// resolved member is done (DESIGN-003 §8). A milestone with nothing finished
// has shipped nothing, so there is nothing honest to snapshot (FR-5.2). The
// resolvedMembers count feeds the reason so an empty milestone reads
// differently from a live-but-unfinished one.
func G4(resolvedMembers, doneMembers int) GateResult {
	switch {
	case resolvedMembers == 0:
		return GateResult{Gate: GateG4, Pass: false, Reason: "milestone has no resolved members"}
	case doneMembers == 0:
		return GateResult{Gate: GateG4, Pass: false,
			Reason: fmt.Sprintf("no member done yet (%d resolved, none complete)", resolvedMembers)}
	}
	return GateResult{Gate: GateG4, Pass: true,
		Reason: fmt.Sprintf("%d of %d resolved member(s) done", doneMembers, resolvedMembers)}
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
