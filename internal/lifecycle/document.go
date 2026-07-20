// Package lifecycle implements DESIGN-003: the document and feature state
// machines and the gate catalogue. Everything here is pure — no I/O, no
// clock, no store — so the full transition matrix is table-testable
// (SPEC-001 NFR-2).
package lifecycle

import "fmt"

type DocumentState string

const (
	DocDraft      DocumentState = "draft"
	DocReviewing  DocumentState = "reviewing"
	DocApproved   DocumentState = "approved"
	DocSuperseded DocumentState = "superseded"
)

type DocumentEvent string

const (
	DocSubmit         DocumentEvent = "submit"
	DocApprove        DocumentEvent = "approve"
	DocRequestChanges DocumentEvent = "request_changes"
	DocEscalate       DocumentEvent = "escalate"
	DocSupersede      DocumentEvent = "supersede"
	DocWithdraw       DocumentEvent = "withdraw"
)

// IllegalTransitionError is the typed rejection required by FR-5.1. There is
// no force flag; overrides happen only through answered checkpoints (L-6).
type IllegalTransitionError struct {
	Entity string // "document" | "feature"
	State  string
	Event  string
}

func (e *IllegalTransitionError) Error() string {
	return fmt.Sprintf("%s event %q is illegal in state %q", e.Entity, e.Event, e.State)
}

// docTransitions maps (state, event) to the resulting state. escalate is a
// legal event in reviewing that leaves the state unchanged — the checkpoint
// is its side effect, handled by the caller (DESIGN-003 §2).
var docTransitions = map[DocumentState]map[DocumentEvent]DocumentState{
	DocDraft: {
		DocSubmit: DocReviewing,
	},
	DocReviewing: {
		DocApprove:        DocApproved,
		DocRequestChanges: DocDraft,
		DocEscalate:       DocReviewing,
		DocWithdraw:       DocDraft,
	},
	DocApproved: {
		DocSupersede: DocSuperseded,
	},
}

// DocumentTransition returns the state event produces from state, or an
// *IllegalTransitionError. Callers persist the result and its audit row in
// the same transaction (DESIGN-002 O-3). Supersede must only be fired by the
// engine processing a successor's approval, never by a user action
// (DESIGN-003 §2) — that restriction is enforced at the API boundary, not
// here, because this function has no notion of actor.
func DocumentTransition(state DocumentState, event DocumentEvent) (DocumentState, error) {
	if next, ok := docTransitions[state][event]; ok {
		return next, nil
	}
	return state, &IllegalTransitionError{Entity: "document", State: string(state), Event: string(event)}
}

// ValidDocumentState reports whether s is one of the four canonical states.
func ValidDocumentState(s DocumentState) bool {
	switch s {
	case DocDraft, DocReviewing, DocApproved, DocSuperseded:
		return true
	}
	return false
}
