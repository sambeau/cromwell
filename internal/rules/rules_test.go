package rules

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"cromwell/internal/bus"
	"cromwell/internal/lifecycle"
)

func docSnap(state lifecycle.DocumentState) (*DocSnap, Snapshot) {
	d := &DocSnap{
		ID:          uuid.New(),
		Type:        "spec",
		State:       state,
		ContentHash: "hash1",
		OwnerType:   "feature",
		OwnerID:     uuid.New(),
		Path:        "docs/specs/login.md",
	}
	return d, Snapshot{Doc: d}
}

func TestSubmitQueuesReviewWithIdempotencyKey(t *testing.T) {
	d, snap := docSnap(lifecycle.DocReviewing)
	actions := Decide(bus.DocumentTransitioned{
		DocID: d.ID, From: lifecycle.DocDraft, To: lifecycle.DocReviewing,
		Event: lifecycle.DocSubmit,
	}, snap)
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d: %+v", len(actions), actions)
	}
	q, ok := actions[0].(QueueReview)
	if !ok {
		t.Fatalf("want QueueReview, got %T", actions[0])
	}
	if q.IdempotencyKey != ReviewIdempotencyKey(d.ID, "hash1") {
		t.Errorf("bad idempotency key: %s", q.IdempotencyKey)
	}
	if q.DocType != "spec" {
		t.Errorf("bad doc type: %s", q.DocType)
	}
}

// Escalation leaves the document in reviewing (From == To); re-entering the
// state must NOT queue another review.
func TestEscalateDoesNotRequeueReview(t *testing.T) {
	d, snap := docSnap(lifecycle.DocReviewing)
	actions := Decide(bus.DocumentTransitioned{
		DocID: d.ID, From: lifecycle.DocReviewing, To: lifecycle.DocReviewing,
		Event: lifecycle.DocEscalate,
	}, snap)
	if len(actions) != 0 {
		t.Fatalf("escalate must not requeue a review: %+v", actions)
	}
}

func TestVerdicts(t *testing.T) {
	d, snap := docSnap(lifecycle.DocReviewing)
	dispatch := uuid.New()
	ev := func(outcome string) bus.DispatchSucceeded {
		return bus.DispatchSucceeded{
			DispatchID: dispatch, Purpose: "review-spec", Role: "spec-reviewer",
			RefType: "document", RefID: d.ID, Outcome: json.RawMessage(outcome),
		}
	}

	// approve
	actions := Decide(ev(`{"verdict":"approve","reasoning":"clear and testable"}`), snap)
	if len(actions) != 1 {
		t.Fatalf("approve: %+v", actions)
	}
	if a := actions[0].(ApproveDocument); a.DocID != d.ID || a.Actor != "spec-reviewer" {
		t.Errorf("approve action wrong: %+v", a)
	}

	// request_changes carries comments and the dispatch id
	actions = Decide(ev(`{"verdict":"request_changes","comments":[{"section_ref":"Overview","body":"vague"}],"reasoning":"r"}`), snap)
	rc, ok := actions[0].(ReturnForChanges)
	if !ok || len(rc.Comments) != 1 || rc.Comments[0].Body != "vague" || rc.DispatchID == nil || *rc.DispatchID != dispatch {
		t.Errorf("request_changes action wrong: %+v", rc)
	}

	// escalate raises a review-escalation checkpoint with the reasoning
	actions = Decide(ev(`{"verdict":"escalate","reasoning":"the owner must weigh in"}`), snap)
	cp, ok := actions[0].(RaiseCheckpoint)
	if !ok || cp.CPKind != "review-escalation" || cp.Context["reasoning"] != "the owner must weigh in" {
		t.Errorf("escalate action wrong: %+v", cp)
	}

	// malformed outcome surfaces as a dispatch-failure checkpoint
	actions = Decide(ev(`{"verdict":"maybe"}`), snap)
	if cp, ok := actions[0].(RaiseCheckpoint); !ok || cp.CPKind != "dispatch-failure" {
		t.Errorf("malformed outcome should raise dispatch-failure: %+v", actions)
	}
}

func TestSpecApprovalEvaluatesG1(t *testing.T) {
	d, snap := docSnap(lifecycle.DocApproved)
	feat := &FeatureSnap{ID: d.OwnerID, State: lifecycle.FeatIdea}
	snap.OwnerFeature = feat
	actions := Decide(bus.DocumentTransitioned{
		DocID: d.ID, From: lifecycle.DocReviewing, To: lifecycle.DocApproved,
		Event: lifecycle.DocApprove,
	}, snap)
	if len(actions) != 2 {
		t.Fatalf("want [reconcile authoring, EvaluateContractGate]: %+v", actions)
	}
	if g := actions[1].(EvaluateContractGate); g.FeatureID != feat.ID {
		t.Errorf("wrong feature: %+v", g)
	}

	// A feature already past idea gets no gate evaluation.
	snap.OwnerFeature = &FeatureSnap{ID: feat.ID, State: lifecycle.FeatReady}
	if actions := Decide(bus.DocumentTransitioned{To: lifecycle.DocApproved, From: lifecycle.DocReviewing}, snap); len(actions) != 0 {
		t.Errorf("ready feature should not re-evaluate G1: %+v", actions)
	}
}

func TestFileChanged(t *testing.T) {
	// Draft doc: reindex.
	d, snap := docSnap(lifecycle.DocDraft)
	actions := Decide(bus.DocumentFileChanged{Path: d.Path, CommitHash: "abc123"}, snap)
	if r, ok := actions[0].(ReindexDocument); !ok || r.DocID != d.ID {
		t.Errorf("draft change should reindex: %+v", actions)
	}

	// Approved doc: integrity checkpoint, no reindex (FR-4.3).
	d2, snap2 := docSnap(lifecycle.DocApproved)
	actions = Decide(bus.DocumentFileChanged{Path: d2.Path, CommitHash: "abc123"}, snap2)
	cp, ok := actions[0].(RaiseCheckpoint)
	if !ok || cp.CPKind != "document-integrity" || cp.Context["commit"] != "abc123" {
		t.Errorf("approved change should raise integrity checkpoint: %+v", actions)
	}
	for _, a := range actions {
		if _, bad := a.(ReindexDocument); bad {
			t.Error("approved drifted content must not be reindexed")
		}
	}

	// Unregistered path: nothing.
	if actions := Decide(bus.DocumentFileChanged{Path: "README.md"}, Snapshot{}); len(actions) != 0 {
		t.Errorf("unregistered path should be ignored: %+v", actions)
	}
}

func TestCheckpointResponses(t *testing.T) {
	d, snap := docSnap(lifecycle.DocReviewing)

	// Escalation answered approve → ApproveDocument by the human.
	actions := Decide(bus.CheckpointResponded{
		Kind: "review-escalation", RefType: "document", RefID: d.ID,
		Response: json.RawMessage(`{"decision":"approve","reason":"looks right"}`), RespondedBy: "sam",
	}, snap)
	if a, ok := actions[0].(ApproveDocument); !ok || a.Actor != "sam" {
		t.Errorf("escalation approve: %+v", actions)
	}

	// Escalation answered request_changes with a comment.
	actions = Decide(bus.CheckpointResponded{
		Kind: "review-escalation", RefType: "document", RefID: d.ID,
		Response: json.RawMessage(`{"decision":"request_changes","comments":[{"body":"tighten scope"}]}`), RespondedBy: "sam",
	}, snap)
	if rc, ok := actions[0].(ReturnForChanges); !ok || len(rc.Comments) != 1 {
		t.Errorf("escalation request_changes: %+v", actions)
	}

	// Gate override on an initiative.
	initID := uuid.New()
	actions = Decide(bus.CheckpointResponded{
		Kind: "gate-override", RefType: "initiative", RefID: initID,
		Response: json.RawMessage(`{"override":true,"reason":"descoped"}`), RespondedBy: "sam",
	}, Snapshot{})
	if a, ok := actions[0].(ArchiveInitiative); !ok || a.InitiativeID != initID || a.Reason != "descoped" {
		t.Errorf("gate override: %+v", actions)
	}
	// Declined override does nothing.
	if actions := Decide(bus.CheckpointResponded{
		Kind: "gate-override", RefType: "initiative", RefID: initID,
		Response: json.RawMessage(`{"override":false}`),
	}, Snapshot{}); len(actions) != 0 {
		t.Errorf("declined override: %+v", actions)
	}

	// Budget answered → kick the queue.
	actions = Decide(bus.CheckpointResponded{
		Kind: "budget", Response: json.RawMessage(`{}`),
	}, Snapshot{})
	if _, ok := actions[0].(KickQueue); !ok {
		t.Errorf("budget response: %+v", actions)
	}
}

func TestDispatchExhaustedRaisesCheckpoint(t *testing.T) {
	id := uuid.New()
	ref := uuid.New()
	actions := Decide(bus.DispatchExhausted{
		DispatchID: id, Purpose: "review-spec", RefType: "document", RefID: ref,
		Error: "429 then 500 then timeout",
	}, Snapshot{})
	cp, ok := actions[0].(RaiseCheckpoint)
	if !ok || cp.CPKind != "dispatch-failure" || cp.Context["error"] != "429 then 500 then timeout" {
		t.Errorf("exhausted dispatch: %+v", actions)
	}
}

// The verdict follows from the findings' severity rather than being a free
// choice — the rule that both ends the pernickety loop and stops a reviewer
// approving over the top of a real defect (audit §3.3a).
func TestReviewVerdictMustMatchSeverity(t *testing.T) {
	cases := []struct {
		name, payload string
		wantErr       bool
	}{
		{"approve with no findings", `{"verdict":"approve","reasoning":"clean"}`, false},
		{"approve over minors", `{"verdict":"approve","comments":[{"body":"tidy","severity":"minor"}],"reasoning":"ok"}`, false},
		{"approve over a major", `{"verdict":"approve","comments":[{"body":"nil deref","severity":"major"}],"reasoning":"ok"}`, true},
		{"approve over an unclassified finding", `{"verdict":"approve","comments":[{"body":"nil deref"}],"reasoning":"ok"}`, true},
		{"changes on a major", `{"verdict":"request_changes","comments":[{"body":"nil deref","severity":"major"}],"reasoning":"bug"}`, false},
		{"changes on minors alone", `{"verdict":"request_changes","comments":[{"body":"tidy","severity":"minor"}],"reasoning":"nits"}`, true},
		{"changes with no comments", `{"verdict":"request_changes","reasoning":"bad"}`, true},
		{"escalate is orthogonal", `{"verdict":"escalate","comments":[{"body":"x","severity":"major"}],"reasoning":"human call"}`, false},
		{"unknown severity", `{"verdict":"approve","comments":[{"body":"x","severity":"critical"}],"reasoning":"ok"}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseReviewOutcome(json.RawMessage(c.payload))
			if c.wantErr && err == nil {
				t.Fatalf("expected rejection, got none")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected acceptance, got %v", err)
			}
		})
	}
}

// An unclassified finding is read as major: failing closed keeps it in play,
// where failing open would drop a real defect silently.
func TestUnclassifiedFindingIsMajor(t *testing.T) {
	if !(ReviewComment{Body: "x"}).IsMajor() {
		t.Error("a finding with no severity must count as major")
	}
	if (ReviewComment{Body: "x", Severity: SeverityMinor}).IsMajor() {
		t.Error("an explicitly minor finding must not count as major")
	}
}
