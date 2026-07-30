package rules

// Phase-2 rule actions and outcome parsing (DESIGN-005, DESIGN-006). The
// rule engine stays pure: it names coarse actions the server executes with
// store access (decompose, dispatch ready tasks, approve a task, merge a
// feature). The stateful multi-step work — readiness re-evaluation, G2/G3
// evaluation, worktree and git operations — lives in the server's action
// executor, exactly as phase 1's document-approval executor did.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"cromwell/internal/lifecycle"
)

// ---- Actions ----

// DecomposeDevPlan creates tasks from a newly-approved dev-plan's task table,
// before G1 advances the feature (DESIGN-005 §4, DP-2).
type DecomposeDevPlan struct {
	FeatureID    uuid.UUID
	DevPlanDocID uuid.UUID
}

func (DecomposeDevPlan) ActionKind() string { return "decompose_dev_plan" }

// ReDecomposeDevPlan reconciles tasks against a revised dev-plan
// non-destructively (DESIGN-005 §4, DP-4).
type ReDecomposeDevPlan struct {
	FeatureID    uuid.UUID
	DevPlanDocID uuid.UUID
}

func (ReDecomposeDevPlan) ActionKind() string { return "redecompose_dev_plan" }

// MarkRevisionInFlight sets spec_stale and raises the revision-in-flight
// checkpoint when a successor contract document is submitted for a feature
// already in flight (DESIGN-005 §6, FR-10.1).
type MarkRevisionInFlight struct {
	FeatureID uuid.UUID
	DocID     uuid.UUID
	DocType   string
}

func (MarkRevisionInFlight) ActionKind() string { return "mark_revision_in_flight" }

// ClearSpecStale resolves a revision-in-flight: clears the flag and, if the
// human chose continue, re-dispatches ready tasks (FR-10.2).
type ClearSpecStale struct {
	FeatureID uuid.UUID
	Continue  bool
}

func (ClearSpecStale) ActionKind() string { return "clear_spec_stale" }

// DispatchReadyTasks enqueues implement-task dispatches for every
// dispatchable task of the feature (the governor serialises them per
// feature, DESIGN-006 §5).
type DispatchReadyTasks struct {
	FeatureID uuid.UUID
}

func (DispatchReadyTasks) ActionKind() string { return "dispatch_ready_tasks" }

// CompleteImplementation records an implementer's finished work: the server
// commits the worktree, moves the task to review, and queues code review
// (DESIGN-006 §5).
type CompleteImplementation struct {
	TaskID     uuid.UUID
	DispatchID uuid.UUID
	Summary    string
}

func (CompleteImplementation) ActionKind() string { return "complete_implementation" }

// ApproveTaskCode records a code-review approval: task → done, readiness
// re-evaluation, then G2 (DESIGN-006 §5).
type ApproveTaskCode struct {
	TaskID uuid.UUID
	Actor  string
	// DroppedMinor are findings the reviewer raised that were not major
	// enough to send the work back. They are audited so a later
	// retrospective can pick them up rather than losing them (audit §3.3a).
	DroppedMinor []ReviewComment
}

func (ApproveTaskCode) ActionKind() string { return "approve_task_code" }

// AbandonTask stops work on a task the review loop could not converge on.
// TaskAbandon is always human and always carries a reason (lifecycle/task.go),
// so this action only ever originates from an answered checkpoint.
type AbandonTask struct {
	TaskID uuid.UUID
	Actor  string
	Reason string
}

func (AbandonTask) ActionKind() string { return "abandon_task" }

// ReturnTaskCode records a code-review request_changes: task → active,
// comments attached, implementer re-dispatched against the kept worktree.
type ReturnTaskCode struct {
	TaskID     uuid.UUID
	Actor      string
	DispatchID *uuid.UUID
	Comments   []ReviewComment
}

func (ReturnTaskCode) ActionKind() string { return "return_task_code" }

// MergeFeature records a verification approval (G3): merge the branch, feature
// → done, worktree GC (DESIGN-006 §6).
type MergeFeature struct {
	FeatureID uuid.UUID
	Actor     string
}

func (MergeFeature) ActionKind() string { return "merge_feature" }

// ReturnFeatureForCriteria records a verification request_changes: unmet
// criteria become tasks, feature → active (DESIGN-006 §6).
type ReturnFeatureForCriteria struct {
	FeatureID uuid.UUID
	Actor     string
	Unmet     []Criterion
}

func (ReturnFeatureForCriteria) ActionKind() string { return "return_feature_for_criteria" }

// ---- Verification outcome (the submit_verification outcome tool) ----

type Criterion struct {
	ID       string `json:"id"`
	Met      bool   `json:"met"`
	Evidence string `json:"evidence,omitempty"`
}

type VerificationOutcome struct {
	Criteria  []Criterion `json:"criteria"`
	Verdict   string      `json:"verdict"` // approve | request_changes | escalate
	Reasoning string      `json:"reasoning"`
}

// ParseVerificationOutcome validates the payload and enforces the evidence
// contract (C-2). The verify-feature skill asks for per-criterion evidence and
// the tool schema carries the field, but nothing checked it: an approve with an
// empty criteria array, or with every evidence blank, used to merge the feature.
// Rubber-stamp approval is the most common quality failure in multi-agent
// systems (MAST FM-3.1), and this is the gate it would walk through.
func ParseVerificationOutcome(raw json.RawMessage) (*VerificationOutcome, error) {
	var o VerificationOutcome
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("verification outcome: %w", err)
	}
	switch o.Verdict {
	case "approve", "request_changes", "escalate":
	default:
		return nil, fmt.Errorf("verification outcome: unknown verdict %q", o.Verdict)
	}
	// escalate is the honest way out when the criteria cannot be judged, so
	// it is not held to the evidence contract.
	if o.Verdict == "escalate" {
		return &o, nil
	}
	if len(o.Criteria) == 0 {
		return nil, fmt.Errorf("verification outcome: no criteria reported — " +
			"list the specification's acceptance criteria and what you checked for each")
	}
	for i, c := range o.Criteria {
		if strings.TrimSpace(c.ID) == "" {
			return nil, fmt.Errorf("verification outcome: criterion %d has no id", i+1)
		}
		if strings.TrimSpace(c.Evidence) == "" {
			return nil, fmt.Errorf(
				"verification outcome: criterion %q has no evidence — say what you read, ran or observed to decide it",
				c.ID)
		}
	}
	if o.Verdict == "approve" {
		if unmet := o.UnmetCriteria(); len(unmet) > 0 {
			return nil, fmt.Errorf("verification outcome: cannot approve with %d unmet criterion/criteria — "+
				"request changes instead", len(unmet))
		}
	}
	return &o, nil
}

// UnmetCriteria returns the criteria the verifier marked not met.
func (o *VerificationOutcome) UnmetCriteria() []Criterion {
	var unmet []Criterion
	for _, c := range o.Criteria {
		if !c.Met {
			unmet = append(unmet, c)
		}
	}
	return unmet
}

// ---- Implementation outcome (the submit_implementation outcome tool) ----

type ImplementationOutcome struct {
	Summary      string   `json:"summary"`
	FilesChanged []string `json:"files_changed,omitempty"`
}

func ParseImplementationOutcome(raw json.RawMessage) (*ImplementationOutcome, error) {
	var o ImplementationOutcome
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("implementation outcome: %w", err)
	}
	return &o, nil
}

// ---- Idempotency keys (DESIGN-006 §7) ----

func ImplementIdempotencyKey(taskID uuid.UUID, redispatch int) string {
	return fmt.Sprintf("implement:%s:%d", taskID, redispatch)
}

func CodeReviewIdempotencyKey(taskID uuid.UUID, commit string) string {
	return fmt.Sprintf("review-code:%s:%s", taskID, commit)
}

func VerifyIdempotencyKey(featureID uuid.UUID, branchHead string) string {
	return fmt.Sprintf("verify:%s:%s", featureID, branchHead)
}

// ---- Snapshot for phase-2 events ----

// TaskSnap is the rule engine's view of a task row.
type TaskSnap struct {
	ID        uuid.UUID
	FeatureID uuid.UUID
	State     lifecycle.TaskState
	// ReviewRounds is how many code reviews this task has already completed,
	// and ReviewRoundCap the configured limit. Severity gating (ParseReviewOutcome)
	// ends the ordinary loop; the cap is the backstop for genuine disagreement,
	// where majors keep recurring round after round (audit §3.3a).
	ReviewRounds   int
	ReviewRoundCap int
}

// decideDispatchSucceededPhase2 routes non-document dispatch outcomes
// (implement-task, review-code, verify-feature). Returns nil if the purpose
// is not a phase-2 execution purpose, so the phase-1 document path runs.
func decideDispatchSucceededPhase2(purpose string, dispatchID uuid.UUID, actor string, outcome json.RawMessage, snap Snapshot) ([]Action, bool) {
	switch purpose {
	case "implement-task":
		if snap.Task == nil {
			return nil, true
		}
		impl, err := ParseImplementationOutcome(outcome)
		summary := ""
		if err == nil {
			summary = impl.Summary
		}
		return []Action{CompleteImplementation{TaskID: snap.Task.ID, DispatchID: dispatchID, Summary: summary}}, true

	case "review-code":
		if snap.Task == nil {
			return nil, true
		}
		o, err := ParseReviewOutcome(outcome)
		if err != nil {
			return []Action{unparseableCheckpoint("task", snap.Task.ID, dispatchID, err)}, true
		}
		id := dispatchID
		switch o.Verdict {
		case "approve":
			// Minor findings are dropped from the loop but not from the
			// record: they are what a later retrospective picks up.
			return []Action{ApproveTaskCode{
				TaskID: snap.Task.ID, Actor: actor, DroppedMinor: o.MinorComments(),
			}}, true
		case "request_changes":
			// The cap catches the case severity gating cannot: reviewer and
			// implementer genuinely disagree, so majors recur every round.
			// A human decides rather than the two grinding on (audit §3.3a).
			if roundCap := snap.Task.ReviewRoundCap; roundCap > 0 && snap.Task.ReviewRounds >= roundCap {
				return []Action{RaiseCheckpoint{
					CPKind: "review-deadlock", RefType: "task", RefID: snap.Task.ID,
					Question: fmt.Sprintf(
						"The reviewer has asked for changes %d times on this task and major issues are still being found. Approve it as it stands, or stop work on it?",
						snap.Task.ReviewRounds),
					Context: map[string]any{
						"rounds": snap.Task.ReviewRounds, "dispatch_id": id.String(),
						"comments": o.Comments,
					},
				}}, true
			}
			return []Action{ReturnTaskCode{TaskID: snap.Task.ID, Actor: actor, DispatchID: &id, Comments: o.Comments}}, true
		case "escalate":
			return []Action{RaiseCheckpoint{
				CPKind: "review-escalation", RefType: "task", RefID: snap.Task.ID,
				Question: "Code reviewer escalated. Approve or request changes?",
				Context:  map[string]any{"reasoning": o.Reasoning, "dispatch_id": id.String()},
			}}, true
		}
		return nil, true

	case "verify-feature":
		if snap.RefFeature == nil {
			return nil, true
		}
		o, err := ParseVerificationOutcome(outcome)
		if err != nil {
			return []Action{unparseableCheckpoint("feature", snap.RefFeature.ID, dispatchID, err)}, true
		}
		switch o.Verdict {
		case "approve":
			return []Action{MergeFeature{FeatureID: snap.RefFeature.ID, Actor: actor}}, true
		case "request_changes":
			return []Action{ReturnFeatureForCriteria{FeatureID: snap.RefFeature.ID, Actor: actor, Unmet: o.UnmetCriteria()}}, true
		case "escalate":
			return []Action{RaiseCheckpoint{
				CPKind: "verification-escalation", RefType: "feature", RefID: snap.RefFeature.ID,
				Question: "Verifier escalated. Approve the feature or return it for changes?",
				Context:  map[string]any{"reasoning": o.Reasoning, "dispatch_id": dispatchID.String()},
			}}, true
		}
		return nil, true
	}
	return nil, false
}

func unparseableCheckpoint(refType string, refID, dispatchID uuid.UUID, err error) Action {
	return RaiseCheckpoint{
		CPKind: "dispatch-failure", RefType: refType, RefID: refID,
		Question: "An agent returned an unparseable outcome. Retry or intervene?",
		Context:  map[string]any{"dispatch_id": dispatchID.String(), "error": err.Error()},
	}
}
