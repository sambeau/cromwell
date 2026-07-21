// Package bus defines the typed in-process events every source normalises
// into, and the bus the orchestrator consumes (DESIGN-002 §2-3). The
// pg NOTIFY channel mirrors these as a safety net; in the single-server
// deployment this bus is primary.
package bus

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"cromwell/internal/lifecycle"
)

type Event interface{ EventKind() string }

// DocumentTransitioned fires after a document state change is committed.
type DocumentTransitioned struct {
	DocID uuid.UUID
	From  lifecycle.DocumentState
	To    lifecycle.DocumentState
	Event lifecycle.DocumentEvent
	Actor string
}

func (DocumentTransitioned) EventKind() string { return "document.transition" }

// DocumentFileChanged fires when the git watcher (or boot catch-up scan)
// sees a registered document's file drift from its indexed content_hash.
type DocumentFileChanged struct {
	Path       string
	CommitHash string
}

func (DocumentFileChanged) EventKind() string { return "document.file_changed" }

// DispatchSucceeded fires when a dispatch completes via its outcome tool.
type DispatchSucceeded struct {
	DispatchID uuid.UUID
	Purpose    string
	Role       string
	RefType    string
	RefID      uuid.UUID
	Outcome    json.RawMessage
}

func (DispatchSucceeded) EventKind() string { return "dispatch.succeeded" }

// DispatchExhausted fires when a dispatch has failed its final attempt
// (DESIGN-002 §8) — the rule engine raises the dispatch-failure checkpoint.
type DispatchExhausted struct {
	DispatchID uuid.UUID
	Purpose    string
	RefType    string
	RefID      uuid.UUID
	Error      string
}

func (DispatchExhausted) EventKind() string { return "dispatch.exhausted" }

// CheckpointResponded fires when a human answers a checkpoint. Context is
// the checkpoint's stored context (reviewer reasoning, dispatch id, ...) so
// rules can resume without a store lookup.
type CheckpointResponded struct {
	CheckpointID uuid.UUID
	Kind         string
	RefType      string
	RefID        uuid.UUID
	Context      json.RawMessage
	Response     json.RawMessage
	RespondedBy  string
}

func (CheckpointResponded) EventKind() string { return "checkpoint.responded" }

// SubmitRequested is the HTTP API's submit action; the server runs
// validation synchronously before any transition (DESIGN-003 §3).
type SubmitRequested struct {
	DocID uuid.UUID
	Actor string
}

func (SubmitRequested) EventKind() string { return "document.submit_requested" }

// FeatureStarted fires when a feature transitions ready → active and its
// worktree exists; the rule engine dispatches its initially-ready tasks
// (DESIGN-006 §5).
type FeatureStarted struct {
	FeatureID uuid.UUID
}

func (FeatureStarted) EventKind() string { return "feature.started" }

// Tick is the heartbeat (DESIGN-002 §3): stall detection, retry sweep, GC,
// reconciliation. Handled by the server's heartbeat duties, not the rule
// engine.
type Tick struct{ At time.Time }

func (Tick) EventKind() string { return "tick" }

// Bus is the in-process event bus: many producers, one consumer (the
// orchestrator loop).
type Bus struct {
	ch chan Event
}

func New(buffer int) *Bus {
	return &Bus{ch: make(chan Event, buffer)}
}

// Publish enqueues an event. It blocks if the buffer is full — backpressure
// on producers is preferable to dropping workflow events.
func (b *Bus) Publish(e Event) { b.ch <- e }

func (b *Bus) Events() <-chan Event { return b.ch }
