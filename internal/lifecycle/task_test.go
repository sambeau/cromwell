package lifecycle

import (
	"errors"
	"testing"
)

func TestTaskTransitionMatrix(t *testing.T) {
	states := []TaskState{TaskPending, TaskReady, TaskActive, TaskReview, TaskDone, TaskAbandoned}
	events := []TaskEvent{TaskReadyEvent, TaskClaim, TaskImplemented, TaskApprove, TaskRequestChanges, TaskAbandon}

	legal := map[TaskState]map[TaskEvent]TaskState{
		TaskPending: {
			TaskReadyEvent: TaskReady,
			TaskAbandon:    TaskAbandoned,
		},
		TaskReady: {
			TaskClaim:   TaskActive,
			TaskAbandon: TaskAbandoned,
		},
		TaskActive: {
			TaskImplemented: TaskReview,
			TaskAbandon:     TaskAbandoned,
		},
		TaskReview: {
			TaskApprove:        TaskDone,
			TaskRequestChanges: TaskActive,
			TaskAbandon:        TaskAbandoned,
		},
		TaskDone:      {},
		TaskAbandoned: {},
	}

	for _, s := range states {
		for _, e := range events {
			next, err := TaskTransition(s, e)
			want, ok := legal[s][e]
			if ok {
				if err != nil || next != want {
					t.Errorf("(%s, %s): got (%s, %v), want (%s, nil)", s, e, next, err, want)
				}
				continue
			}
			var ill *IllegalTransitionError
			if !errors.As(err, &ill) {
				t.Errorf("(%s, %s): want IllegalTransitionError, got %v", s, e, err)
			}
			if next != s {
				t.Errorf("(%s, %s): illegal transition must not move state; got %s", s, e, next)
			}
		}
	}
}

func TestTaskTerminal(t *testing.T) {
	for s, want := range map[TaskState]bool{
		TaskPending: false, TaskReady: false, TaskActive: false,
		TaskReview: false, TaskDone: true, TaskAbandoned: true,
	} {
		if s.Terminal() != want {
			t.Errorf("%s.Terminal() = %v, want %v", s, s.Terminal(), want)
		}
	}
}

func TestG2(t *testing.T) {
	cases := []struct {
		total, done, terminal int
		stale                 bool
		pass                  bool
	}{
		{3, 2, 3, false, true},  // all terminal, some done
		{3, 0, 3, false, false}, // all abandoned, none done
		{3, 1, 2, false, false}, // one still open
		{0, 0, 0, false, false}, // no tasks
		{3, 3, 3, true, false},  // stale contract blocks even when complete
	}
	for _, c := range cases {
		got := G2(c.total, c.done, c.terminal, c.stale)
		if got.Pass != c.pass {
			t.Errorf("G2(%d,%d,%d,stale=%v).Pass = %v, want %v (%q)",
				c.total, c.done, c.terminal, c.stale, got.Pass, c.pass, got.Reason)
		}
		if got.Gate != GateG2 || got.Reason == "" {
			t.Errorf("G2 malformed: %+v", got)
		}
	}
}

func TestG3(t *testing.T) {
	if r := G3(true, true); !r.Pass {
		t.Errorf("G3(approved, merged) should pass: %+v", r)
	}
	if r := G3(false, true); r.Pass {
		t.Errorf("G3(not approved) should fail: %+v", r)
	}
	if r := G3(true, false); r.Pass {
		t.Errorf("G3(not merged) should fail: %+v", r)
	}
}
