package lifecycle

import (
	"fmt"
	"strings"
)

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
	// The reasons are sentences a person reads as they stand: the Send to
	// development button shows them beside itself (SPEC-011 FR-2.6).
	pass := ownDesignApproved || parentDesignApproved
	reason := "Its initiative's design is approved, so this feature can be sent to development."
	switch {
	case ownDesignApproved:
		reason = "This feature's own design is approved, so it can be sent to development."
	case !pass:
		reason = "Neither this feature's design nor its initiative's design is approved yet. " +
			"A feature can be sent to development once one of them is."
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
// resolved item is done (DESIGN-003 §8). An item is a leaf a milestone
// resolves to: a feature, or a checklist (SPEC-014 SD-3), since DESIGN-010 §6
// asks for "at least one deliverable done" and a checklist is a deliverable. A
// milestone with nothing finished has shipped nothing, so there is nothing
// honest to snapshot (FR-5.2). The resolved count feeds the reason so an empty
// milestone reads differently from a live-but-unfinished one.
func G4(resolvedItems, doneItems int) GateResult {
	// A refusal is shown to a person as it stands — beside the disabled "Mark as
	// shipped" button in the web UI, and relayed by the chat agent — so it is a
	// full sentence that counts what the gate counts, in the word the progress
	// line uses: items (DESIGN-008 D-6, SPEC-010 FR-5.1, SPEC-014 FR-2.4).
	switch {
	case resolvedItems == 0:
		return GateResult{Gate: GateG4, Pass: false,
			Reason: "This milestone can't be marked as shipped yet, because nothing in it comes down to a feature or a checklist. " +
				"Add the work it is meant to deliver first."}
	case doneItems == 0 && resolvedItems == 1:
		return GateResult{Gate: GateG4, Pass: false,
			Reason: "This milestone can't be marked as shipped yet, because the one item in it isn't done. " +
				"Marking it as shipped records what actually went out, so at least one item has to be finished first."}
	case doneItems == 0:
		return GateResult{Gate: GateG4, Pass: false,
			Reason: fmt.Sprintf("This milestone can't be marked as shipped yet, because none of its %d items is done. "+
				"Marking it as shipped records what actually went out, so at least one has to be finished first.", resolvedItems)}
	}
	return GateResult{Gate: GateG4, Pass: true,
		Reason: fmt.Sprintf("%d of %d resolved item(s) done", doneItems, resolvedItems)}
}

// G5 — initiative-archivable: no non-terminal features in the subtree.
// Failure is overridable only via an answered gate-override checkpoint (L-6).
func G5(nonTerminalFeatures int) GateResult {
	return G5Open(nonTerminalFeatures, nil)
}

// G5Open is G5 that also names the spikes in the subtree that aren't closed
// (by their IDs): a running one is spending tokens and an ended one waits for
// a person to read it (SPEC-021 SD-14).
func G5Open(nonTerminalFeatures int, openSpikes []string) GateResult {
	if len(openSpikes) > 0 {
		var reason string
		if len(openSpikes) == 1 {
			reason = openSpikes[0] + " is still open"
		} else {
			reason = joinIDs(openSpikes) + " are still open"
		}
		if nonTerminalFeatures > 0 {
			reason = fmt.Sprintf("%d non-terminal feature(s) in subtree, and %s", nonTerminalFeatures, reason)
		}
		return GateResult{Gate: GateG5, Pass: false, Reason: reason}
	}
	if nonTerminalFeatures > 0 {
		return GateResult{
			Gate:   GateG5,
			Pass:   false,
			Reason: fmt.Sprintf("%d non-terminal feature(s) in subtree", nonTerminalFeatures),
		}
	}
	return GateResult{Gate: GateG5, Pass: true, Reason: "no non-terminal features in subtree"}
}

// joinIDs lists IDs as "A", "A and B" or "A, B and C".
func joinIDs(ids []string) string {
	if len(ids) == 1 {
		return ids[0]
	}
	return strings.Join(ids[:len(ids)-1], ", ") + " and " + ids[len(ids)-1]
}
