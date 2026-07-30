package rules

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"cromwell/internal/bus"
	"cromwell/internal/lifecycle"
)

func TestDevPlanApprovalDecomposesThenGates(t *testing.T) {
	docID, featID := uuid.New(), uuid.New()
	snap := Snapshot{
		Doc:          &DocSnap{ID: docID, Type: "dev_plan", State: lifecycle.DocApproved, OwnerType: "feature", OwnerID: featID},
		OwnerFeature: &FeatureSnap{ID: featID, State: lifecycle.FeatIdea},
	}
	actions := Decide(bus.DocumentTransitioned{DocID: docID, From: lifecycle.DocReviewing, To: lifecycle.DocApproved}, snap)
	if len(actions) != 2 {
		t.Fatalf("want [decompose, gate], got %d: %+v", len(actions), actions)
	}
	if d, ok := actions[0].(DecomposeDevPlan); !ok || d.FeatureID != featID {
		t.Errorf("action 0 should decompose: %+v", actions[0])
	}
	if _, ok := actions[1].(EvaluateContractGate); !ok {
		t.Errorf("action 1 should evaluate G1: %+v", actions[1])
	}
}

func TestSpecApprovalOnlyGates(t *testing.T) {
	docID, featID := uuid.New(), uuid.New()
	snap := Snapshot{
		Doc:          &DocSnap{ID: docID, Type: "spec", State: lifecycle.DocApproved, OwnerType: "feature", OwnerID: featID},
		OwnerFeature: &FeatureSnap{ID: featID, State: lifecycle.FeatIdea},
	}
	actions := Decide(bus.DocumentTransitioned{To: lifecycle.DocApproved, From: lifecycle.DocReviewing}, snap)
	if len(actions) != 1 {
		t.Fatalf("spec approval should only gate: %+v", actions)
	}
	if _, ok := actions[0].(EvaluateContractGate); !ok {
		t.Errorf("want EvaluateContractGate: %+v", actions[0])
	}
}

func TestFeatureStartedDispatchesTasks(t *testing.T) {
	featID := uuid.New()
	actions := Decide(bus.FeatureStarted{FeatureID: featID}, Snapshot{})
	if len(actions) != 1 {
		t.Fatalf("want DispatchReadyTasks: %+v", actions)
	}
	if d, ok := actions[0].(DispatchReadyTasks); !ok || d.FeatureID != featID {
		t.Errorf("wrong action: %+v", actions[0])
	}
}

func TestImplementationSucceeded(t *testing.T) {
	taskID, dispatchID := uuid.New(), uuid.New()
	snap := Snapshot{Task: &TaskSnap{ID: taskID, State: lifecycle.TaskActive}}
	actions := Decide(bus.DispatchSucceeded{
		DispatchID: dispatchID, Purpose: "implement-task", Role: "implementer",
		RefType: "task", RefID: taskID, Outcome: json.RawMessage(`{"summary":"added the form","files_changed":["form.go"]}`),
	}, snap)
	c, ok := actions[0].(CompleteImplementation)
	if !ok || c.TaskID != taskID || c.Summary != "added the form" {
		t.Errorf("want CompleteImplementation: %+v", actions)
	}
}

func TestCodeReviewVerdicts(t *testing.T) {
	taskID, dispatchID := uuid.New(), uuid.New()
	snap := Snapshot{Task: &TaskSnap{ID: taskID, State: lifecycle.TaskReview}}
	ev := func(o string) bus.DispatchSucceeded {
		return bus.DispatchSucceeded{DispatchID: dispatchID, Purpose: "review-code", Role: "code-reviewer",
			RefType: "task", RefID: taskID, Outcome: json.RawMessage(o)}
	}
	if a := Decide(ev(`{"verdict":"approve","reasoning":"clean"}`), snap); len(a) != 1 {
		t.Fatalf("approve: %+v", a)
	} else if _, ok := a[0].(ApproveTaskCode); !ok {
		t.Errorf("want ApproveTaskCode: %+v", a[0])
	}
	if a := Decide(ev(`{"verdict":"request_changes","comments":[{"body":"handle nil"}],"reasoning":"bug"}`), snap); len(a) != 1 {
		t.Fatalf("rc: %+v", a)
	} else if rc, ok := a[0].(ReturnTaskCode); !ok || len(rc.Comments) != 1 {
		t.Errorf("want ReturnTaskCode with comments: %+v", a[0])
	}
	if a := Decide(ev(`{"verdict":"escalate","reasoning":"design call"}`), snap); len(a) != 1 {
		t.Fatalf("esc: %+v", a)
	} else if cp, ok := a[0].(RaiseCheckpoint); !ok || cp.RefType != "task" || cp.CPKind != "review-escalation" {
		t.Errorf("want task review-escalation: %+v", a[0])
	}
}

// The loop terminates on severity, not on a human's patience: minor findings
// are dropped (and recorded) rather than sent back for another round.
func TestCodeReviewMinorFindingsDoNotSendWorkBack(t *testing.T) {
	taskID, dispatchID := uuid.New(), uuid.New()
	snap := Snapshot{Task: &TaskSnap{ID: taskID, State: lifecycle.TaskReview}}
	ev := func(o string) bus.DispatchSucceeded {
		return bus.DispatchSucceeded{DispatchID: dispatchID, Purpose: "review-code", Role: "code-reviewer",
			RefType: "task", RefID: taskID, Outcome: json.RawMessage(o)}
	}
	out := `{"verdict":"approve","comments":[
		{"body":"name could be clearer","severity":"minor"},
		{"body":"prefer a switch here","severity":"minor"}],"reasoning":"nothing major"}`
	a := Decide(ev(out), snap)
	if len(a) != 1 {
		t.Fatalf("approve with minors: %+v", a)
	}
	ap, ok := a[0].(ApproveTaskCode)
	if !ok {
		t.Fatalf("want ApproveTaskCode: %+v", a[0])
	}
	if len(ap.DroppedMinor) != 2 {
		t.Errorf("dropped minor findings must be carried for the record, got %d", len(ap.DroppedMinor))
	}
}

// The cap is the backstop for genuine disagreement: majors recurring every
// round stop being the agents' problem and become a human's.
func TestCodeReviewRoundCapRaisesDeadlockCheckpoint(t *testing.T) {
	taskID, dispatchID := uuid.New(), uuid.New()
	out := `{"verdict":"request_changes","comments":[{"body":"still wrong","severity":"major"}],"reasoning":"again"}`
	ev := bus.DispatchSucceeded{DispatchID: dispatchID, Purpose: "review-code", Role: "code-reviewer",
		RefType: "task", RefID: taskID, Outcome: json.RawMessage(out)}

	under := Snapshot{Task: &TaskSnap{ID: taskID, State: lifecycle.TaskReview, ReviewRounds: 2, ReviewRoundCap: 3}}
	if a := Decide(ev, under); len(a) != 1 {
		t.Fatalf("under cap: %+v", a)
	} else if _, ok := a[0].(ReturnTaskCode); !ok {
		t.Errorf("under the cap the work goes back to the implementer: %+v", a[0])
	}

	at := Snapshot{Task: &TaskSnap{ID: taskID, State: lifecycle.TaskReview, ReviewRounds: 3, ReviewRoundCap: 3}}
	if a := Decide(ev, at); len(a) != 1 {
		t.Fatalf("at cap: %+v", a)
	} else if cp, ok := a[0].(RaiseCheckpoint); !ok || cp.CPKind != "review-deadlock" {
		t.Errorf("at the cap a human decides: %+v", a[0])
	}

	// A cap of zero means unbounded, so an unconfigured project behaves as before.
	off := Snapshot{Task: &TaskSnap{ID: taskID, State: lifecycle.TaskReview, ReviewRounds: 99, ReviewRoundCap: 0}}
	if a := Decide(ev, off); len(a) != 1 {
		t.Fatalf("cap off: %+v", a)
	} else if _, ok := a[0].(ReturnTaskCode); !ok {
		t.Errorf("cap 0 disables the backstop: %+v", a[0])
	}
}

func TestVerificationVerdicts(t *testing.T) {
	featID, dispatchID := uuid.New(), uuid.New()
	snap := Snapshot{RefFeature: &FeatureSnap{ID: featID, State: lifecycle.FeatReview}}
	ev := func(o string) bus.DispatchSucceeded {
		return bus.DispatchSucceeded{DispatchID: dispatchID, Purpose: "verify-feature", Role: "verifier",
			RefType: "feature", RefID: featID, Outcome: json.RawMessage(o)}
	}
	if a := Decide(ev(`{"verdict":"approve","criteria":[{"id":"AC1","met":true,"evidence":"TestLogin covers valid and invalid credentials"}],"reasoning":"all met"}`), snap); len(a) != 1 {
		t.Fatalf("approve: %+v", a)
	} else if m, ok := a[0].(MergeFeature); !ok || m.FeatureID != featID {
		t.Errorf("want MergeFeature: %+v", a[0])
	}
	a := Decide(ev(`{"verdict":"request_changes","criteria":[{"id":"AC1","met":true,"evidence":"TestLogin passes"},{"id":"AC2","met":false,"evidence":"lockout not enforced"}],"reasoning":"one gap"}`), snap)
	rf, ok := a[0].(ReturnFeatureForCriteria)
	if !ok || len(rf.Unmet) != 1 || rf.Unmet[0].ID != "AC2" {
		t.Errorf("want ReturnFeatureForCriteria with the unmet criterion: %+v", a[0])
	}
}

func TestRevisionInFlightOnSubmit(t *testing.T) {
	docID, featID := uuid.New(), uuid.New()
	// A successor spec submitted for an active feature blocks + queues review.
	snap := Snapshot{
		Doc:          &DocSnap{ID: docID, Type: "spec", State: lifecycle.DocReviewing, OwnerType: "feature", OwnerID: featID, ContentHash: "h", IsSuccessor: true},
		OwnerFeature: &FeatureSnap{ID: featID, State: lifecycle.FeatActive},
	}
	actions := Decide(bus.DocumentTransitioned{DocID: docID, From: lifecycle.DocDraft, To: lifecycle.DocReviewing}, snap)
	if len(actions) != 2 {
		t.Fatalf("want [mark-revision, queue-review], got %+v", actions)
	}
	if m, ok := actions[0].(MarkRevisionInFlight); !ok || m.FeatureID != featID {
		t.Errorf("action 0 should mark revision in flight: %+v", actions[0])
	}
	if _, ok := actions[1].(QueueReview); !ok {
		t.Errorf("action 1 should still queue the review: %+v", actions[1])
	}

	// A first-draft (non-successor) submit does NOT mark revision.
	snap.Doc.IsSuccessor = false
	if a := Decide(bus.DocumentTransitioned{From: lifecycle.DocDraft, To: lifecycle.DocReviewing, DocID: docID}, snap); len(a) != 1 {
		t.Errorf("non-successor submit should only queue review: %+v", a)
	}
}

func TestRevisionInFlightResponse(t *testing.T) {
	featID := uuid.New()
	a := Decide(bus.CheckpointResponded{
		Kind: "revision-in-flight", RefType: "feature", RefID: featID,
		Response: json.RawMessage(`{"continue":true}`), RespondedBy: "sam",
	}, Snapshot{})
	c, ok := a[0].(ClearSpecStale)
	if !ok || c.FeatureID != featID || !c.Continue {
		t.Errorf("want ClearSpecStale continue=true: %+v", a)
	}
}

func TestReDecomposeOnRevisedDevPlanApproval(t *testing.T) {
	docID, featID := uuid.New(), uuid.New()
	snap := Snapshot{
		Doc:          &DocSnap{ID: docID, Type: "dev_plan", State: lifecycle.DocApproved, OwnerType: "feature", OwnerID: featID, IsSuccessor: true},
		OwnerFeature: &FeatureSnap{ID: featID, State: lifecycle.FeatActive},
	}
	a := Decide(bus.DocumentTransitioned{To: lifecycle.DocApproved, From: lifecycle.DocReviewing, DocID: docID}, snap)
	if len(a) != 1 {
		t.Fatalf("want ReDecomposeDevPlan: %+v", a)
	}
	if _, ok := a[0].(ReDecomposeDevPlan); !ok {
		t.Errorf("want ReDecomposeDevPlan: %+v", a[0])
	}
}

// C-2: the verify-feature skill asks for per-criterion evidence and the tool
// schema carries the field, but nothing used to check it — an approve with an
// empty criteria array merged the feature. Rubber-stamp approval is the most
// common quality failure in multi-agent systems, and this was the gate it
// would have walked through.
func TestVerificationEvidenceContract(t *testing.T) {
	cases := []struct {
		name, payload string
		wantErr       bool
	}{
		{"approve with evidence", `{"verdict":"approve","criteria":[{"id":"AC1","met":true,"evidence":"TestLogin passes"}],"reasoning":"met"}`, false},
		{"approve with no criteria at all", `{"verdict":"approve","criteria":[],"reasoning":"looks fine"}`, true},
		{"approve with blank evidence", `{"verdict":"approve","criteria":[{"id":"AC1","met":true,"evidence":"   "}],"reasoning":"met"}`, true},
		{"approve with evidence absent", `{"verdict":"approve","criteria":[{"id":"AC1","met":true}],"reasoning":"met"}`, true},
		{"approve while a criterion is unmet", `{"verdict":"approve","criteria":[{"id":"AC1","met":false,"evidence":"not implemented"}],"reasoning":"close enough"}`, true},
		{"criterion with no id", `{"verdict":"approve","criteria":[{"id":"","met":true,"evidence":"x"}],"reasoning":"met"}`, true},
		{"request_changes still needs evidence of the gap", `{"verdict":"request_changes","criteria":[{"id":"AC1","met":false}],"reasoning":"gap"}`, true},
		{"escalate is exempt — it is the honest way out", `{"verdict":"escalate","criteria":[],"reasoning":"criterion is ambiguous"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseVerificationOutcome(json.RawMessage(c.payload))
			if c.wantErr && err == nil {
				t.Fatalf("expected rejection, got none")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected acceptance, got %v", err)
			}
		})
	}
}
