package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
)

// An implementer that has succeeded but whose completion hasn't committed
// still holds the feature's working copy: a sibling's implementer waits, and
// so does a claim (SPEC-020 SD-3, SD-4).
func TestImplementPendingCompletionHoldsTheFeature(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	t1 := seedActiveTask(t, s, fid, 0)
	t2 := seedActiveTask(t, s, fid, 1)

	var d1, d2 *Dispatch
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		if d1, err = EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m", "task", t1.ID, "impl:t1:0"); err != nil {
			return err
		}
		if d2, err = EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m", "task", t2.ID, "impl:t2:0"); err != nil {
			return err
		}
		_, err = MarkDispatchRunning(ctx, tx, d1.ID, nil)
		return err
	})
	if got, err := ImplementPendingCompletion(ctx, s.Pool, fid); err != nil || got != "" {
		t.Fatalf("a running implementer isn't pending completion: %q %v", got, err)
	}
	inTx(t, s, func(tx pgx.Tx) error {
		return MarkAttemptSucceeded(ctx, tx, d1.ID, 1, TokenUsage{}, 0, map[string]any{})
	})
	if got, err := ImplementPendingCompletion(ctx, s.Pool, fid); err != nil || got != t1.PublicID {
		t.Fatalf("pending = %q %v, want %s", got, err, t1.PublicID)
	}
	if hold, err := ImplementHold(ctx, s.Pool, t2.ID, d2.ID); err != nil || hold != QueueReasonCompletionPending {
		t.Errorf("the sibling's implementer should wait for the completion: %q %v", hold, err)
	}
	// StartImplementDispatch holds it too.
	var res StartResult
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		res, err = StartImplementDispatch(ctx, tx, d2.ID, nil, "")
		return err
	})
	if res.Outcome != StartQueued || res.Reason != QueueReasonCompletionPending {
		t.Errorf("start = %+v, want queued for the pending completion", res)
	}

	// A completion that never committed stops holding the copy after the window.
	if _, err := s.Pool.Exec(ctx, `UPDATE dispatches SET finished_at = now() - interval '1 hour' WHERE id = $1`, d1.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := ImplementPendingCompletion(ctx, s.Pool, fid); got != "" {
		t.Errorf("a stuck completion should stop holding the copy, got %q", got)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE dispatches SET finished_at = now() WHERE id = $1`, d1.ID); err != nil {
		t.Fatal(err)
	}

	// A task with a claim isn't pending: its work is the claimant's.
	inTx(t, s, func(tx pgx.Tx) error {
		_, err := CreateClaim(ctx, tx, "task", t1.ID, &fid, "chat", "chat-agent", "mcp", "")
		return err
	})
	if got, _ := ImplementPendingCompletion(ctx, s.Pool, fid); got != "" {
		t.Errorf("a claimed task isn't pending completion, got %q", got)
	}
}

// A completion that has moved the task on, or a rework that has been queued,
// doesn't hold the copy.
func TestImplementPendingCompletionEnds(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	t1 := seedActiveTask(t, s, fid, 0)
	var d1 *Dispatch
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		if d1, err = EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m", "task", t1.ID, "impl:t1:0"); err != nil {
			return err
		}
		if _, err = MarkDispatchRunning(ctx, tx, d1.ID, nil); err != nil {
			return err
		}
		return MarkAttemptSucceeded(ctx, tx, d1.ID, 1, TokenUsage{}, 0, map[string]any{})
	})
	inTx(t, s, func(tx pgx.Tx) error {
		return TransitionTask(ctx, tx, t1, lifecycle.TaskImplemented, "orchestrator", nil)
	})
	if got, _ := ImplementPendingCompletion(ctx, s.Pool, fid); got != "" {
		t.Errorf("a task in review isn't pending completion, got %q", got)
	}
	// Sent back and re-dispatched: the old success doesn't count.
	inTx(t, s, func(tx pgx.Tx) error {
		if err := TransitionTask(ctx, tx, t1, lifecycle.TaskRequestChanges, "orchestrator", nil); err != nil {
			return err
		}
		_, err := EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m", "task", t1.ID, "impl:t1:1")
		return err
	})
	if got, _ := ImplementPendingCompletion(ctx, s.Pool, fid); got != "" {
		t.Errorf("a queued rework isn't pending completion, got %q", got)
	}
}

// CancelIfTaskClaimed says "claimed" whether or not it was the call that
// cancelled the dispatch, so the retry sweep raises nothing (SPEC-020 FR-6.5).
func TestCancelIfTaskClaimedReportsAClaimThatGotThereFirst(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	task := seedActiveTask(t, s, fid, 0)
	var d *Dispatch
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		d, err = EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m", "task", task.ID, "impl:1")
		return err
	})
	var claimed bool
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		claimed, err = CancelIfTaskClaimed(ctx, tx, d.ID, "orchestrator")
		return err
	})
	if claimed {
		t.Error("an unclaimed task reports not claimed")
	}
	// The claim lands and cancels the dispatch before the sweep looks.
	inTx(t, s, func(tx pgx.Tx) error {
		if _, err := CreateClaim(ctx, tx, "task", task.ID, &fid, "chat", "chat-agent", "mcp", ""); err != nil {
			return err
		}
		_, err := CancelTaskImplementDispatches(ctx, tx, task.ID, "chat-agent", "claimed")
		return err
	})
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		claimed, err = CancelIfTaskClaimed(ctx, tx, d.ID, "orchestrator")
		return err
	})
	if !claimed {
		t.Error("a dispatch the claim already cancelled still means the task is claimed")
	}
}
