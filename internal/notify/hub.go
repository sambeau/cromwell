// Package notify is the command centre's realtime boundary (DESIGN-007 §6,
// CC-3). It fans in-process events out to connected browsers over Server-Sent
// Events without ever touching the orchestrator's single-consumer bus.
//
// The bus is many-producers/one-consumer — the orchestrator (DESIGN-002 §3). A
// second reader on that channel would *steal* events from the orchestrator, so
// the SSE source is not a second bus consumer. Instead the orchestrator — the
// bus's sole consumer — forwards each event it processes to this Hub, which
// holds one buffered channel per SSE connection and delivers non-blocking: a
// slow or dead client is dropped, never back-pressuring the orchestrator (a
// missed event costs a stale region until the next event, never a wrong action,
// SD-5).
//
// Hub is the single-server implementation of the Notifier interface DEC-002
// mandates; a hosted multi-process deployment can implement Notifier over
// Supabase Realtime with no change above this line.
package notify

import (
	"context"
	"sync"

	"cromwell/internal/bus"
)

// Notifier is the UI's realtime boundary. Subscribe returns a receive-only
// stream of events and a cancel func the caller must invoke to release the
// subscription. The single-server deployment wraps the in-process hub; a hosted
// deployment can implement this over Supabase Realtime (DEC-002).
type Notifier interface {
	Subscribe(ctx context.Context) (<-chan bus.Event, func())
}

// subBuffer is the per-connection buffer depth. A browser that falls this far
// behind is dropped rather than back-pressuring the orchestrator.
const subBuffer = 32

// Hub broadcasts events to all current subscribers. It is safe for concurrent
// use: the orchestrator calls Broadcast from its single goroutine while SSE
// handlers Subscribe and unsubscribe from their own.
type Hub struct {
	mu   sync.Mutex
	subs map[int]chan bus.Event
	next int
}

// NewHub returns an empty hub ready to Subscribe and Broadcast.
func NewHub() *Hub {
	return &Hub{subs: make(map[int]chan bus.Event)}
}

// Broadcast delivers ev to every current subscriber without blocking. A
// subscriber whose buffer is full is skipped for this event (it will catch up
// on the next event it can receive, or on a manual refresh); Broadcast never
// blocks the caller, so a slow browser cannot stall the orchestrator.
func (h *Hub) Broadcast(ev bus.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// Subscriber is behind; drop this event for it (SD-5).
		}
	}
}

// Subscribe registers a new subscriber and returns its event stream and an
// unsubscribe func. The stream is closed when the caller invokes unsubscribe or
// when ctx is cancelled, whichever comes first. The caller must always invoke
// unsubscribe (directly or via ctx cancellation) to avoid leaking the channel.
func (h *Hub) Subscribe(ctx context.Context) (<-chan bus.Event, func()) {
	h.mu.Lock()
	id := h.next
	h.next++
	ch := make(chan bus.Event, subBuffer)
	h.subs[id] = ch
	h.mu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			h.mu.Lock()
			if c, ok := h.subs[id]; ok {
				delete(h.subs, id)
				close(c)
			}
			h.mu.Unlock()
		})
	}

	// Release the subscription if the request context ends first.
	go func() {
		<-ctx.Done()
		unsub()
	}()

	return ch, unsub
}

// Subscribers reports the current subscriber count (for diagnostics/tests).
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
