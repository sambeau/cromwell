// Package rules is the orchestrator's decision core: pure functions from
// (event, snapshot of current state) to actions (DESIGN-002 §2). No I/O, no
// clock, no store — the server fetches the snapshot, calls Decide, and
// executes the returned actions, each one transactional with its audit rows
// (O-3). Idempotency keys make replay after crash or duplicate events safe
// (DESIGN-002 §8).
package rules

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"cromwell/internal/bus"
	"cromwell/internal/lifecycle"
)

// ---- Actions ----

type Action interface{ ActionKind() string }

// QueueReview queues a reviewer dispatch for a document in `reviewing`.
type QueueReview struct {
	DocID          uuid.UUID
	DocType        string
	IdempotencyKey string
}

func (QueueReview) ActionKind() string { return "queue_review" }

// ApproveDocument executes the approve transition; the same transaction
// supersedes the predecessor if this document revises one (DESIGN-003 §5)
// and performs the repo file operations.
type ApproveDocument struct {
	DocID uuid.UUID
	Actor string
}

func (ApproveDocument) ActionKind() string { return "approve_document" }

// ReturnForChanges executes request_changes: document back to draft,
// comments inserted for the author (DESIGN-003 §4).
type ReturnForChanges struct {
	DocID      uuid.UUID
	Actor      string
	DispatchID *uuid.UUID // set when the comments come from an agent-reviewer
	Comments   []ReviewComment
}

func (ReturnForChanges) ActionKind() string { return "return_for_changes" }

// RaiseCheckpoint creates a pending checkpoint (idempotent per kind+ref
// while pending, FR-6.2).
type RaiseCheckpoint struct {
	CPKind   string
	RefType  string
	RefID    uuid.UUID
	Question string
	Context  map[string]any
}

func (RaiseCheckpoint) ActionKind() string { return "raise_checkpoint" }

// EvaluateContractGate runs G1 for a feature and, on pass, fires
// contract_approved (idea → ready). The gate.evaluated audit row precedes
// the transition row in the same transaction (FR-3.2).
type EvaluateContractGate struct {
	FeatureID uuid.UUID
}

func (EvaluateContractGate) ActionKind() string { return "evaluate_contract_gate" }

// ReconcileAuthoring restores the authoring invariants over an owner's scope
// (SPEC-009 FR-4): every feature G0 admits and that has a description has a
// current spec, and every feature with an approved spec has a current
// dev-plan. The scope is a feature, or an initiative's direct child features —
// expanded by the server, because a rule may not reach the store.
//
// It is deliberately an invariant rather than "write a spec now": the same
// action is safe to run at any moment on any scope, which is what lets the
// heartbeat use it to recover from a lost event.
type ReconcileAuthoring struct {
	OwnerType string // "feature" or "initiative"
	OwnerID   uuid.UUID
}

func (ReconcileAuthoring) ActionKind() string { return "reconcile_authoring" }

// RecordReviewComments files a comments-only review outcome (SPEC-009
// FR-2.2): the reviewer's comments and reasoning go onto the document's
// thread, and the document stays exactly where it was. This is the whole of
// what a reviewer may do to a human-approved document — there is deliberately
// no variant of this action that transitions state.
type RecordReviewComments struct {
	DocID      uuid.UUID
	Actor      string
	DispatchID *uuid.UUID
	Reasoning  string
	Comments   []ReviewComment
}

func (RecordReviewComments) ActionKind() string { return "record_review_comments" }

// ReindexDocument re-parses and re-indexes a registered document's file.
type ReindexDocument struct {
	DocID uuid.UUID
}

func (ReindexDocument) ActionKind() string { return "reindex_document" }

// ArchiveInitiative archives after a G5 override was approved by a human.
type ArchiveInitiative struct {
	InitiativeID uuid.UUID
	Reason       string
	Actor        string
}

func (ArchiveInitiative) ActionKind() string { return "archive_initiative" }

// RetryDispatch re-queues a failed dispatch after a human chose retry on a
// dispatch-failure checkpoint.
type RetryDispatch struct {
	DispatchID uuid.UUID
}

func (RetryDispatch) ActionKind() string { return "retry_dispatch" }

// CancelDispatch closes a failed dispatch for good (human declined retry) so
// the retry sweep stops re-surfacing it.
type CancelDispatch struct {
	DispatchID uuid.UUID
	Actor      string
}

func (CancelDispatch) ActionKind() string { return "cancel_dispatch" }

// KickQueue re-runs the governor over queued dispatches (e.g. after a
// budget checkpoint was answered).
type KickQueue struct{}

func (KickQueue) ActionKind() string { return "kick_queue" }

// ---- Review outcome (the submit_review outcome tool's payload, O-2) ----

// Severity classifies a finding. Only major findings send work back; minor
// findings ride along on a round that is happening anyway and are dropped
// when nothing major remains (C-1, audit §3.3a).
const (
	SeverityMajor = "major"
	SeverityMinor = "minor"
)

type ReviewComment struct {
	SectionRef string `json:"section_ref,omitempty"`
	Body       string `json:"body"`
	// Severity is "major" or "minor". An empty value is read as major:
	// failing closed keeps a finding in play, where failing open would drop
	// it silently — the severity-deflation risk in audit §3.3a.
	Severity string `json:"severity,omitempty"`
}

// IsMajor reports whether the comment blocks approval.
func (c ReviewComment) IsMajor() bool { return c.Severity != SeverityMinor }

type ReviewOutcome struct {
	Verdict   string          `json:"verdict"` // approve | request_changes | escalate
	Comments  []ReviewComment `json:"comments,omitempty"`
	Reasoning string          `json:"reasoning"`
}

// HasMajor reports whether any finding blocks approval.
func (o *ReviewOutcome) HasMajor() bool {
	for _, c := range o.Comments {
		if c.IsMajor() {
			return true
		}
	}
	return false
}

// MinorComments returns the findings that do not block approval. On an
// approve verdict these are the ones being dropped, and they are recorded
// rather than discarded (audit §3.3a, C-9).
func (o *ReviewOutcome) MinorComments() []ReviewComment {
	var minor []ReviewComment
	for _, c := range o.Comments {
		if !c.IsMajor() {
			minor = append(minor, c)
		}
	}
	return minor
}

// ParseReviewOutcome validates the payload and, with it, the loop-termination
// rule: the verdict follows from the findings' severity rather than being a
// free choice (audit §3.3a). A reviewer may not approve while holding a major
// finding, and may not send work back over minors alone — the second half is
// what makes the loop terminate instead of grinding through ever-finer detail.
// escalate stays free: it is orthogonal to severity.
func ParseReviewOutcome(raw json.RawMessage) (*ReviewOutcome, error) {
	var o ReviewOutcome
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("review outcome: %w", err)
	}
	switch o.Verdict {
	case "approve", "request_changes", "escalate":
	default:
		return nil, fmt.Errorf("review outcome: unknown verdict %q", o.Verdict)
	}
	for i, c := range o.Comments {
		switch c.Severity {
		case "", SeverityMajor, SeverityMinor:
		default:
			return nil, fmt.Errorf("review outcome: comment %d has unknown severity %q (expected %q or %q)",
				i+1, c.Severity, SeverityMajor, SeverityMinor)
		}
	}
	switch o.Verdict {
	case "approve":
		if o.HasMajor() {
			return nil, fmt.Errorf("review outcome: cannot approve while a major finding stands — " +
				"either downgrade it to minor or request changes")
		}
	case "request_changes":
		if len(o.Comments) == 0 {
			return nil, fmt.Errorf("review outcome: request_changes needs at least one comment saying what to change")
		}
		if !o.HasMajor() {
			return nil, fmt.Errorf("review outcome: cannot request changes when every finding is minor — " +
				"approve and let the minor findings stand, or raise one to major")
		}
	}
	return &o, nil
}

// CommentsOutcome is the submit_comments outcome tool's payload (SPEC-009
// FR-2.2): comments and reasoning, and no verdict. The design reviewer joins
// the discussion; a human closes it.
type CommentsOutcome struct {
	Comments  []ReviewComment `json:"comments,omitempty"`
	Reasoning string          `json:"reasoning"`
	// Verdict has no meaning here and must be absent. It is parsed only so a
	// payload that carries one is rejected out loud rather than silently
	// stripped of it — the drift the confinement-by-omission principle exists
	// to prevent.
	Verdict string `json:"verdict,omitempty"`
}

// ParseCommentsOutcome validates a comments-only review payload. A verdict is
// an error, not a surplus field: a reviewer of a human-approved document has
// no approval authority, and an outcome that claims some must fail rather
// than be quietly ignored.
func ParseCommentsOutcome(raw json.RawMessage) (*CommentsOutcome, error) {
	var o CommentsOutcome
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("comments outcome: %w", err)
	}
	if o.Verdict != "" {
		return nil, fmt.Errorf("comments outcome: a verdict %q was given, but this review has no verdict — a human decides whether the document is approved", o.Verdict)
	}
	if o.Reasoning == "" {
		return nil, fmt.Errorf("comments outcome: reasoning is required — say what you checked and what stands")
	}
	return &o, nil
}

// ReviewIdempotencyKey is the deterministic key preventing double dispatch
// of the same review (DESIGN-002 §8).
func ReviewIdempotencyKey(docID uuid.UUID, contentHash string) string {
	return fmt.Sprintf("review:%s:%s", docID, contentHash)
}

// ---- Snapshot ----

// DocSnap is the rule engine's view of a document row.
type DocSnap struct {
	ID          uuid.UUID
	Type        string
	State       lifecycle.DocumentState
	ContentHash string
	OwnerType   string
	OwnerID     uuid.UUID
	Path        string
	IsSuccessor bool // supersedes_id is set — this doc is a revision (DESIGN-005 §6)
	// HumanApproval is the type's declared approval authority (SPEC-009
	// FR-2.1), read from the manifest by the server when it builds the
	// snapshot. When true, no dispatch outcome may move this document — the
	// review branch records comments and nothing else.
	HumanApproval bool
}

// FeatureSnap is the rule engine's view of a feature row.
type FeatureSnap struct {
	ID    uuid.UUID
	State lifecycle.FeatureState
}

// Snapshot carries exactly the state a rule may consult. The server
// populates the fields relevant to the event; absent entities are nil.
type Snapshot struct {
	Doc          *DocSnap
	OwnerFeature *FeatureSnap // the feature owning Doc (document events)
	Task         *TaskSnap    // the task the event refs (implement/review-code)
	RefFeature   *FeatureSnap // the feature the event refs (verify, feature-started)
}

// ---- Decide ----

// Decide maps an event to follow-up actions. Unknown or irrelevant events
// produce no actions — silence is a valid decision, never an error.
func Decide(ev bus.Event, snap Snapshot) []Action {
	switch e := ev.(type) {
	case bus.DocumentTransitioned:
		return decideDocumentTransition(e, snap)
	case bus.DispatchSucceeded:
		return decideDispatchSucceeded(e, snap)
	case bus.DispatchExhausted:
		return []Action{RaiseCheckpoint{
			CPKind:   "dispatch-failure",
			RefType:  e.RefType,
			RefID:    e.RefID,
			Question: fmt.Sprintf("Dispatch %s (%s) failed all attempts. Retry or cancel?", e.DispatchID, e.Purpose),
			Context:  map[string]any{"dispatch_id": e.DispatchID.String(), "error": e.Error},
		}}
	case bus.DocumentFileChanged:
		return decideFileChanged(e, snap)
	case bus.CheckpointResponded:
		return decideCheckpointResponded(e, snap)
	case bus.FeatureStarted:
		return []Action{DispatchReadyTasks{FeatureID: e.FeatureID}}
	}
	return nil
}

func decideDocumentTransition(e bus.DocumentTransitioned, snap Snapshot) []Action {
	switch e.To {
	case lifecycle.DocReviewing:
		if e.From != lifecycle.DocDraft || snap.Doc == nil {
			return nil // escalate leaves state at reviewing; no new dispatch
		}
		var actions []Action
		// A successor spec/dev-plan submitted for a feature already in flight
		// blocks new work and asks the human what a mid-flight revision means
		// (DESIGN-005 §6, FR-10.1). This is in addition to queuing its review.
		if snap.Doc.IsSuccessor && snap.Doc.OwnerType == "feature" && snap.OwnerFeature != nil &&
			(snap.OwnerFeature.State == lifecycle.FeatActive || snap.OwnerFeature.State == lifecycle.FeatReview) &&
			(snap.Doc.Type == "spec" || snap.Doc.Type == "dev_plan") {
			actions = append(actions, MarkRevisionInFlight{
				FeatureID: snap.OwnerFeature.ID, DocID: snap.Doc.ID, DocType: snap.Doc.Type,
			})
		}
		actions = append(actions, QueueReview{
			DocID:          snap.Doc.ID,
			DocType:        snap.Doc.Type,
			IdempotencyKey: ReviewIdempotencyKey(snap.Doc.ID, snap.Doc.ContentHash),
		})
		return actions

	case lifecycle.DocApproved:
		if snap.Doc == nil {
			return nil
		}
		// A design reaching approved is gate 1: the single act that means
		// "ready to spec". The scope is whatever owns the design — the
		// feature itself, or the initiative's direct child features — and
		// the server expands it, since a rule may not reach the store
		// (SPEC-009 FR-4.3).
		if snap.Doc.Type == "design" {
			return []Action{ReconcileAuthoring{
				OwnerType: snap.Doc.OwnerType, OwnerID: snap.Doc.OwnerID,
			}}
		}
		if snap.Doc.OwnerType != "feature" || snap.OwnerFeature == nil {
			return nil
		}
		f := snap.OwnerFeature
		// Contract documents approved while the feature is still forming:
		// a dev-plan decomposes into tasks (before G1), and both spec and
		// dev-plan approvals re-evaluate the now-two-part contract gate.
		if f.State == lifecycle.FeatIdea && (snap.Doc.Type == "spec" || snap.Doc.Type == "dev_plan") {
			var actions []Action
			if snap.Doc.Type == "dev_plan" {
				actions = append(actions, DecomposeDevPlan{FeatureID: f.ID, DevPlanDocID: snap.Doc.ID})
			}
			// An approved spec is what releases the dev-plan invariant. Without
			// this the chain stalls one step past gate 1: G1 needs both halves
			// of the contract and nothing would ever write the second.
			if snap.Doc.Type == "spec" {
				actions = append(actions, ReconcileAuthoring{OwnerType: "feature", OwnerID: f.ID})
			}
			actions = append(actions, EvaluateContractGate{FeatureID: f.ID})
			return actions
		}
		// A revised contract document approved for an in-flight feature:
		// re-decompose (dev-plan) so the task set matches the new plan
		// (DESIGN-005 §4, FR-10.2). spec_stale is cleared when the human
		// resolves the revision-in-flight checkpoint.
		if snap.Doc.IsSuccessor && snap.Doc.Type == "dev_plan" &&
			(f.State == lifecycle.FeatActive || f.State == lifecycle.FeatReview) {
			return []Action{ReDecomposeDevPlan{FeatureID: f.ID, DevPlanDocID: snap.Doc.ID}}
		}
	}
	return nil
}

func decideDispatchSucceeded(e bus.DispatchSucceeded, snap Snapshot) []Action {
	// Phase-2 execution purposes (implement-task, review-code, verify-feature)
	// route first; a document review falls through to the phase-1 path.
	if actions, handled := decideDispatchSucceededPhase2(e.Purpose, e.DispatchID, e.Role, e.Outcome, snap); handled {
		return actions
	}
	if actions, handled := decideDispatchSucceededPhase3(e); handled {
		return actions
	}
	if e.RefType != "document" || snap.Doc == nil {
		return nil
	}
	dispatchID := e.DispatchID
	// A human-approved type (FR-2.1) takes the comments-only path, and takes it
	// on the document's declared authority rather than on the shape of the
	// payload: this branch has no case that returns ApproveDocument, so no
	// outcome an agent can produce — verdict-shaped or otherwise — moves the
	// document. Only a human does that (FR-2.3).
	if snap.Doc.HumanApproval {
		outcome, err := ParseCommentsOutcome(e.Outcome)
		if err != nil {
			return []Action{RaiseCheckpoint{
				CPKind:   "dispatch-failure",
				RefType:  e.RefType,
				RefID:    e.RefID,
				Question: "Reviewer returned an unusable outcome. Retry or intervene?",
				Context:  map[string]any{"dispatch_id": e.DispatchID.String(), "error": err.Error()},
			}}
		}
		return []Action{RecordReviewComments{
			DocID:      snap.Doc.ID,
			Actor:      e.Role,
			DispatchID: &dispatchID,
			Reasoning:  outcome.Reasoning,
			Comments:   outcome.Comments,
		}}
	}
	outcome, err := ParseReviewOutcome(e.Outcome)
	if err != nil {
		// A malformed outcome should have failed the dispatch before it
		// succeeded; reaching here is a bug surfaced honestly.
		return []Action{RaiseCheckpoint{
			CPKind:   "dispatch-failure",
			RefType:  e.RefType,
			RefID:    e.RefID,
			Question: "Reviewer returned an unparseable outcome. Retry or intervene?",
			Context:  map[string]any{"dispatch_id": e.DispatchID.String(), "error": err.Error()},
		}}
	}
	switch outcome.Verdict {
	case "approve":
		return []Action{ApproveDocument{DocID: snap.Doc.ID, Actor: e.Role}}
	case "request_changes":
		return []Action{ReturnForChanges{
			DocID:      snap.Doc.ID,
			Actor:      e.Role,
			DispatchID: &dispatchID,
			Comments:   outcome.Comments,
		}}
	case "escalate":
		return []Action{RaiseCheckpoint{
			CPKind:   "review-escalation",
			RefType:  "document",
			RefID:    snap.Doc.ID,
			Question: fmt.Sprintf("Reviewer escalated %s. Approve or request changes?", snap.Doc.Path),
			Context:  map[string]any{"reasoning": outcome.Reasoning, "dispatch_id": dispatchID.String()},
		}}
	}
	return nil
}

func decideFileChanged(e bus.DocumentFileChanged, snap Snapshot) []Action {
	if snap.Doc == nil {
		return nil // unregistered path: not Cromwell's concern
	}
	if snap.Doc.State == lifecycle.DocApproved {
		// Approved documents are immutable; the honest path is revision
		// (DESIGN-003 §2). Do not reindex the drifted content.
		return []Action{RaiseCheckpoint{
			CPKind:   "document-integrity",
			RefType:  "document",
			RefID:    snap.Doc.ID,
			Question: fmt.Sprintf("Approved document %s was modified on disk. Revert the file or create a revision.", snap.Doc.Path),
			Context:  map[string]any{"commit": e.CommitHash},
		}}
	}
	return []Action{ReindexDocument{DocID: snap.Doc.ID}}
}

// EscalationResponse is the payload of a review-escalation checkpoint
// answer: decision approve | request_changes, with optional comments.
type EscalationResponse struct {
	Decision string          `json:"decision"`
	Comments []ReviewComment `json:"comments,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}

// OverrideResponse is the payload of a gate-override checkpoint answer.
type OverrideResponse struct {
	Override bool   `json:"override"`
	Reason   string `json:"reason,omitempty"`
}

// FailureResponse is the payload of a dispatch-failure checkpoint answer.
type FailureResponse struct {
	Retry bool `json:"retry"`
}

// RevisionResponse is the payload of a revision-in-flight checkpoint answer:
// continue applies the revision to later work; pause leaves the feature
// blocked (DESIGN-005 §6).
type RevisionResponse struct {
	Continue bool `json:"continue"`
}

// DeadlockResponse is the payload of a review-deadlock checkpoint answer:
// the reviewer and implementer could not converge within the round cap, so a
// human either takes the code as it stands or stops work on the task.
type DeadlockResponse struct {
	Decision string `json:"decision"` // approve | abandon
	Reason   string `json:"reason,omitempty"`
}

func decideCheckpointResponded(e bus.CheckpointResponded, snap Snapshot) []Action {
	switch e.Kind {
	case "review-escalation":
		var r EscalationResponse
		if err := json.Unmarshal(e.Response, &r); err != nil {
			return nil
		}
		// A review escalation refs either a document (spec/dev-plan review)
		// or a task (code review); the human's decision maps to the matching
		// approve/request_changes action.
		switch e.RefType {
		case "document":
			if snap.Doc == nil {
				return nil
			}
			switch r.Decision {
			case "approve":
				return []Action{ApproveDocument{DocID: snap.Doc.ID, Actor: e.RespondedBy}}
			case "request_changes":
				return []Action{ReturnForChanges{DocID: snap.Doc.ID, Actor: e.RespondedBy, Comments: r.Comments}}
			}
		case "task":
			if snap.Task == nil {
				return nil
			}
			switch r.Decision {
			case "approve":
				return []Action{ApproveTaskCode{TaskID: snap.Task.ID, Actor: e.RespondedBy}}
			case "request_changes":
				return []Action{ReturnTaskCode{TaskID: snap.Task.ID, Actor: e.RespondedBy, Comments: r.Comments}}
			}
		}
	case "review-deadlock":
		var r DeadlockResponse
		if err := json.Unmarshal(e.Response, &r); err != nil || snap.Task == nil {
			return nil
		}
		switch r.Decision {
		case "approve":
			return []Action{ApproveTaskCode{TaskID: snap.Task.ID, Actor: e.RespondedBy}}
		case "abandon":
			return []Action{AbandonTask{TaskID: snap.Task.ID, Actor: e.RespondedBy, Reason: r.Reason}}
		}
	case "verification-escalation":
		var r EscalationResponse
		if err := json.Unmarshal(e.Response, &r); err != nil || snap.RefFeature == nil {
			return nil
		}
		switch r.Decision {
		case "approve":
			return []Action{MergeFeature{FeatureID: snap.RefFeature.ID, Actor: e.RespondedBy}}
		case "request_changes":
			return []Action{ReturnFeatureForCriteria{FeatureID: snap.RefFeature.ID, Actor: e.RespondedBy}}
		}
	case "revision-in-flight":
		var r RevisionResponse
		if err := json.Unmarshal(e.Response, &r); err != nil {
			return nil
		}
		return []Action{ClearSpecStale{FeatureID: e.RefID, Continue: r.Continue}}
	case "gate-override":
		var r OverrideResponse
		if err := json.Unmarshal(e.Response, &r); err != nil || !r.Override {
			return nil
		}
		if e.RefType == "initiative" {
			return []Action{ArchiveInitiative{InitiativeID: e.RefID, Reason: r.Reason, Actor: e.RespondedBy}}
		}
	case "dispatch-failure":
		var r FailureResponse
		if err := json.Unmarshal(e.Response, &r); err != nil {
			return nil
		}
		dispatchID := e.RefID // fall back: ref is the dispatch itself
		if v, ok := contextDispatchID(e.Context); ok {
			dispatchID = v
		}
		if r.Retry {
			return []Action{RetryDispatch{DispatchID: dispatchID}}
		}
		return []Action{CancelDispatch{DispatchID: dispatchID, Actor: e.RespondedBy}}
	case "budget":
		return []Action{KickQueue{}}
	}
	return nil
}

func contextDispatchID(raw json.RawMessage) (uuid.UUID, bool) {
	var m struct {
		DispatchID string `json:"dispatch_id"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.DispatchID == "" {
		return uuid.UUID{}, false
	}
	id, err := uuid.Parse(m.DispatchID)
	if err != nil {
		return uuid.UUID{}, false
	}
	return id, true
}
