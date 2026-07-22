package notify

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"cromwell/internal/bus"
)

func recv(t *testing.T, ch <-chan bus.Event) bus.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return nil
	}
}

func TestHubBroadcastReachesSubscribers(t *testing.T) {
	h := NewHub()
	a, unsubA := h.Subscribe(context.Background())
	b, unsubB := h.Subscribe(context.Background())
	defer unsubA()
	defer unsubB()

	if got := h.Subscribers(); got != 2 {
		t.Fatalf("Subscribers() = %d, want 2", got)
	}

	ev := bus.CheckpointRaised{CheckpointID: uuid.New(), Kind: "gate-override"}
	h.Broadcast(ev)

	if got := recv(t, a).EventKind(); got != "checkpoint.raised" {
		t.Errorf("subscriber a got %q", got)
	}
	if got := recv(t, b).EventKind(); got != "checkpoint.raised" {
		t.Errorf("subscriber b got %q", got)
	}
}

func TestHubUnsubscribeStops(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(context.Background())
	unsub()

	if got := h.Subscribers(); got != 0 {
		t.Fatalf("Subscribers() = %d, want 0 after unsubscribe", got)
	}
	// The channel is closed; a receive should not block and should report closed.
	if _, ok := <-ch; ok {
		t.Error("expected closed channel after unsubscribe")
	}
	// Unsubscribing twice is safe.
	unsub()
	// Broadcasting to no subscribers is safe.
	h.Broadcast(bus.Tick{At: time.Now()})
}

func TestHubContextCancellationUnsubscribes(t *testing.T) {
	h := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = h.Subscribe(ctx)
	cancel()

	deadline := time.After(time.Second)
	for {
		if h.Subscribers() == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("context cancellation did not unsubscribe")
		case <-time.After(time.Millisecond):
		}
	}
}

// TestHubSlowSubscriberIsNotBlocking proves the orchestrator can never be
// stalled by a browser that stops reading: once a subscriber's buffer fills,
// Broadcast drops for it and returns immediately (SD-5).
func TestHubSlowSubscriberDoesNotBlock(t *testing.T) {
	h := NewHub()
	slow, unsub := h.Subscribe(context.Background())
	defer unsub()

	// Never read from slow; flood well past the buffer depth.
	done := make(chan struct{})
	go func() {
		for i := 0; i < subBuffer*10; i++ {
			h.Broadcast(bus.Tick{At: time.Now()})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Broadcast blocked on a slow subscriber")
	}
	// The slow subscriber buffered up to its depth and dropped the rest.
	if got := len(slow); got != subBuffer {
		t.Errorf("slow subscriber buffered %d events, want %d", got, subBuffer)
	}
}
