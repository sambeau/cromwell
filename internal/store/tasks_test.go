package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/lifecycle"
)

// seedFeature creates an initiative + feature and returns the feature id.
func seedFeature(t *testing.T, s *Store) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var fid uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		in, err := CreateInitiative(ctx, tx, nil, "auth", "Auth", "", "sam")
		if err != nil {
			return err
		}
		f, err := CreateFeature(ctx, tx, in.ID, "login", "Login", "", "sam")
		if err != nil {
			return err
		}
		fid = f.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fid
}

func TestTaskCreationAndReadiness(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)

	// Two tasks: T2 depends on T1.
	var t1, t2 *Task
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		t1, err = CreateTask(ctx, tx, fid, 0, "T1", "First", "", "orchestrator")
		if err != nil {
			return err
		}
		t2, err = CreateTask(ctx, tx, fid, 1, "T2", "Second", "", "orchestrator")
		if err != nil {
			return err
		}
		return SetTaskDependencies(ctx, tx, t2.ID, []uuid.UUID{t1.ID})
	})
	if err != nil {
		t.Fatal(err)
	}

	// T1 has no deps → make it ready and done; T2 should then be a dependent.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := TransitionTask(ctx, tx, t1, lifecycle.TaskReadyEvent, "orchestrator", nil); err != nil {
			return err
		}
		if err := TransitionTask(ctx, tx, t1, lifecycle.TaskClaim, "orchestrator", nil); err != nil {
			return err
		}
		if err := TransitionTask(ctx, tx, t1, lifecycle.TaskImplemented, "impl", nil); err != nil {
			return err
		}
		return TransitionTask(ctx, tx, t1, lifecycle.TaskApprove, "reviewer", nil)
	})
	if err != nil {
		t.Fatal(err)
	}

	dependents, err := ReadyDependents(ctx, s.Pool, fid, t1.ID)
	if err != nil || len(dependents) != 1 || dependents[0].ID != t2.ID {
		t.Fatalf("T2 should be a ready dependent once T1 is done: %+v %v", dependents, err)
	}

	// Counts feed G2: 2 total, 1 done, 1 terminal.
	total, done, terminal, err := TaskCounts(ctx, s.Pool, fid)
	if err != nil || total != 2 || done != 1 || terminal != 1 {
		t.Errorf("counts = (%d,%d,%d), want (2,1,1): %v", total, done, terminal, err)
	}
}

func TestReDecompositionReconcile(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)

	// First decomposition: T1 (will be done), T2 (will stay pending).
	var t1 *Task
	_ = s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		t1, err = CreateTask(ctx, tx, fid, 0, "T1", "First", "", "orchestrator")
		if err != nil {
			return err
		}
		_, err = CreateTask(ctx, tx, fid, 1, "T2", "Second", "", "orchestrator")
		return err
	})
	// Drive T1 to done.
	_ = s.WithTx(ctx, func(tx pgx.Tx) error {
		for _, ev := range []lifecycle.TaskEvent{lifecycle.TaskReadyEvent, lifecycle.TaskClaim, lifecycle.TaskImplemented, lifecycle.TaskApprove} {
			if err := TransitionTask(ctx, tx, t1, ev, "x", nil); err != nil {
				return err
			}
		}
		return nil
	})

	existing, err := ExistingTasksForPlan(ctx, s.Pool, fid)
	if err != nil {
		t.Fatal(err)
	}
	// New table drops T1 (done) and T2 (pending), adds T3.
	rows := []lifecycle.TaskRow{{Position: 0, LocalID: "T3", Title: "Third"}}
	plan := lifecycle.PlanDecomposition(rows, existing)

	if len(plan.Creates) != 1 || plan.Creates[0].LocalID != "T3" {
		t.Errorf("T3 create expected: %+v", plan.Creates)
	}
	if len(plan.DeleteLocalIDs) != 1 || plan.DeleteLocalIDs[0] != "T2" {
		t.Errorf("T2 (pending, dropped) should be deletable: %+v", plan.DeleteLocalIDs)
	}
	if len(plan.KeptMismatches) != 1 || plan.KeptMismatches[0] != "T1" {
		t.Errorf("T1 (done, dropped) must be kept as a mismatch: %+v", plan.KeptMismatches)
	}
}

func TestWorktreeAndSpecStale(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)

	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		w, err := CreateWorktreeRow(ctx, tx, fid, ".cromwell/worktrees/login", "cromwell/auth/login", "sam")
		if err != nil {
			return err
		}
		_ = w
		return SetFeatureBranch(ctx, tx, fid, "cromwell/auth/login")
	})
	if err != nil {
		t.Fatal(err)
	}

	wt, err := LiveWorktreeForFeature(ctx, s.Pool, fid)
	if err != nil || wt.Branch != "cromwell/auth/login" {
		t.Fatalf("worktree: %+v %v", wt, err)
	}

	// A second live worktree for the same feature is rejected.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := CreateWorktreeRow(ctx, tx, fid, "other", "other", "sam")
		return e
	})
	if err == nil {
		t.Error("second live worktree per feature should be rejected")
	}

	// spec_stale set/clear is idempotent and audited only on change.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := SetSpecStale(ctx, tx, fid, true, "orchestrator", "revision in flight"); err != nil {
			return err
		}
		return SetSpecStale(ctx, tx, fid, true, "orchestrator", "again") // no-op
	})
	if err != nil {
		t.Fatal(err)
	}
	stale, _ := FeatureSpecStale(ctx, s.Pool, fid)
	if !stale {
		t.Error("feature should be stale")
	}

	// GC the worktree.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		return MarkWorktreeRemoved(ctx, tx, wt.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LiveWorktreeForFeature(ctx, s.Pool, fid); err != ErrNotFound {
		t.Error("removed worktree should not be live")
	}
}

// TestGovernorSerialisationQuery is FR-7.2 at the store level: a running
// implement-task dispatch blocks another implement dispatch for a task of the
// same feature, but not for a different feature (governor check 3).
func TestGovernorSerialisationQuery(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)

	var t1, t2 *Task
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		t1, err = CreateTask(ctx, tx, fid, 0, "T1", "a", "", "orchestrator")
		if err != nil {
			return err
		}
		t2, err = CreateTask(ctx, tx, fid, 1, "T2", "b", "", "orchestrator")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Run an implement dispatch for T1.
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		d, err := EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m", "task", t1.ID, "impl:t1:0")
		if err != nil {
			return err
		}
		_, err = MarkDispatchRunning(ctx, tx, d.ID, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// T2 (same feature) is blocked.
	busy, err := MutatingDispatchActiveForTask(ctx, s.Pool, t2.ID, uuid.New())
	if err != nil || !busy {
		t.Errorf("T2 should be blocked by T1's running dispatch: busy=%v err=%v", busy, err)
	}

	// A task of a different feature is not blocked.
	var t3 *Task
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		in, err := CreateInitiative(ctx, tx, nil, "other", "Other", "", "sam")
		if err != nil {
			return err
		}
		f, err := CreateFeature(ctx, tx, in.ID, "thing", "Thing", "", "sam")
		if err != nil {
			return err
		}
		t3, err = CreateTask(ctx, tx, f.ID, 0, "T1", "c", "", "orchestrator")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if busy, _ := MutatingDispatchActiveForTask(ctx, s.Pool, t3.ID, uuid.New()); busy {
		t.Error("a different feature's task must not be blocked")
	}
}
