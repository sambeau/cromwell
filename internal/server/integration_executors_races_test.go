package server

// The races and out-of-order acts of SPEC-020 FR-6.5 that no earlier test
// covers, against real Postgres with the dispatcher held (the budget cap is
// shrunk, so the real dispatcher starts nothing and the tests start dispatches
// themselves through store.StartImplementDispatch). Each iteration lets the
// goroutines loose together; which one wins varies, so each asserts the
// invariants that hold either way: exactly one winner, and never a claim and an
// implementer in one feature's working copy (FR-2.7, SD-4).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

const raceIterations = 20

// lagFor staggers the dispatcher's side of a race: a claim does its git work
// before it takes the feature's lock, so with no lag the dispatcher always
// wins. Cycling the lag lets each side win in some iterations; the assertions
// hold whichever does. Coverage depends on timing and the invariants do not;
// requireBothWon fails a test in which only one side ever won.
func lagFor(i int) time.Duration { return time.Duration(i%5) * 30 * time.Millisecond }

// requireBothWon fails the test if, over the iterations, one side never won:
// the race was then not exercised.
func requireBothWon(t *testing.T, aName string, a int, bName string, b int) {
	t.Helper()
	t.Logf("%s won %d, %s won %d of %d", aName, a, bName, b, raceIterations)
	if a == 0 || b == 0 {
		t.Fatalf("only one side ever won (%s %d, %s %d): the race was not exercised; widen lagFor", aName, a, bName, b)
	}
}

// startDispatch is the dispatcher's start of an implement dispatch.
func (h *harness) startDispatch(id uuid.UUID) (store.StartResult, error) {
	var res store.StartResult
	err := h.srv.Store.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		res, err = store.StartImplementDispatch(context.Background(), tx, id, nil, "")
		return err
	})
	return res, err
}

// resetTask puts a task back as Start building leaves it: active, no claim
// that hasn't ended, no execution, and its implement dispatch queued again.
func (h *harness) resetTask(taskID, dispatchID uuid.UUID) {
	h.t.Helper()
	h.exec(`UPDATE work_claims SET state = 'ended', end_reason = 'released', ended_by = 'test', ended_at = now()
		WHERE ref_type = 'task' AND ref_id = $1 AND state <> 'ended'`, taskID)
	h.exec(`DELETE FROM executions WHERE ref_type = 'task' AND ref_id = $1`, taskID)
	h.exec(`UPDATE dispatches SET state = 'queued', started_at = NULL, finished_at = NULL, heartbeat_at = NULL,
		queue_reason = NULL WHERE id = $1`, dispatchID)
	h.exec(`UPDATE tasks SET state = 'active' WHERE id = $1`, taskID)
}

// runTogether runs the functions at once, released by one barrier.
func runTogether(fns ...func()) {
	var ready, done sync.WaitGroup
	gate := make(chan struct{})
	for _, fn := range fns {
		ready.Add(1)
		done.Add(1)
		go func(fn func()) {
			defer done.Done()
			ready.Done()
			<-gate
			fn()
		}(fn)
	}
	ready.Wait()
	close(gate)
	done.Wait()
}

func (h *harness) countWhere(sql string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.srv.Store.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

// TestClaimRacingTheDispatcherHasOneWinner is FR-6.5: a claim racing
// StartImplementDispatch on the same task. Exactly one wins, and no implementer
// ever runs on a claimed task.
func TestClaimRacingTheDispatcherHasOneWinner(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	d := h.dispatchesOf(t1.ID, "implement-task")[0]
	var claimWins, startWins int

	for i := 0; i < raceIterations; i++ {
		h.resetTask(t1.ID, d.ID)
		var claimErr error
		var starts [2]store.StartResult
		var startErrs [2]error
		runTogether(
			func() { _, claimErr = h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant()) },
			func() { time.Sleep(lagFor(i)); starts[0], startErrs[0] = h.startDispatch(d.ID) },
			func() { time.Sleep(lagFor(i)); starts[1], startErrs[1] = h.startDispatch(d.ID) },
		)
		for _, err := range startErrs {
			if err != nil {
				t.Fatalf("iteration %d: a start errored: %v", i, err)
			}
		}
		started := 0
		for _, s := range starts {
			if s.Outcome == store.StartStarted {
				started++
			}
		}
		agentRows := h.countWhere(`SELECT count(*) FROM executions WHERE ref_type = 'task' AND ref_id = $1 AND kind = 'agent'`, t1.ID)
		openClaims := h.countWhere(`SELECT count(*) FROM work_claims WHERE ref_type = 'task' AND ref_id = $1 AND state <> 'ended'`, t1.ID)
		got, _ := store.GetDispatch(ctx, h.srv.Store.Pool, d.ID)

		if claimErr == nil {
			claimWins++
			if started != 0 || agentRows != 0 || openClaims != 1 || got.State != "cancelled" {
				t.Fatalf("iteration %d: the claim won, but starts=%d agent rows=%d claims=%d dispatch=%s (%+v)",
					i, started, agentRows, openClaims, got.State, starts)
			}
			continue
		}
		if _, ok := AsClaimRefusal(claimErr); !ok {
			t.Fatalf("iteration %d: the claim lost with %v, want a refusal", i, claimErr)
		}
		startWins++
		if started != 1 || agentRows != 1 || openClaims != 0 || got.State != "running" {
			t.Fatalf("iteration %d: the dispatcher won, but starts=%d agent rows=%d claims=%d dispatch=%s (%+v)",
				i, started, agentRows, openClaims, got.State, starts)
		}
	}
	requireBothWon(t, "claim", claimWins, "dispatcher", startWins)
}

// TestClaimRacingDispatchReadyTasks is FR-2.7 and FR-6.5: dispatchReadyTasks
// locks the feature and re-reads each task, so a task is either claimed or
// dispatched, never both.
func TestClaimRacingDispatchReadyTasks(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	f, err := h.srv.featureByPath(ctx, "auth/login")
	if err != nil {
		t.Fatal(err)
	}
	d := h.dispatchesOf(t1.ID, "implement-task")[0]

	for i := 0; i < raceIterations; i++ {
		// T1 is ready again, as before Start building, with no dispatch.
		h.resetTask(t1.ID, d.ID)
		h.exec(`DELETE FROM executions WHERE dispatch_id = $1`, d.ID)
		h.exec(`DELETE FROM dispatches WHERE id = $1`, d.ID)
		h.exec(`UPDATE tasks SET state = 'ready' WHERE id = $1`, t1.ID)

		var claimErr, readyErr error
		runTogether(
			func() { _, claimErr = h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant()) },
			func() { time.Sleep(lagFor(i)); readyErr = h.srv.dispatchReadyTasks(ctx, f.ID) },
		)
		if readyErr != nil {
			t.Fatalf("iteration %d: dispatchReadyTasks: %v", i, readyErr)
		}
		if claimErr != nil {
			if _, ok := AsClaimRefusal(claimErr); !ok {
				t.Fatalf("iteration %d: %v", i, claimErr)
			}
		}
		open := h.countWhere(`SELECT count(*) FROM work_claims WHERE ref_type = 'task' AND ref_id = $1 AND state <> 'ended'`, t1.ID)
		live := h.countWhere(`SELECT count(*) FROM dispatches WHERE ref_type = 'task' AND ref_id = $1
			AND purpose = 'implement-task' AND state IN ('queued', 'running')`, t1.ID)
		running := h.countWhere(`SELECT count(*) FROM dispatches WHERE state = 'running'`)
		if open+live != 1 || running != 0 {
			t.Fatalf("iteration %d: claim=%v: %d claims and %d live implement dispatches (want exactly one of them), %d running",
				i, claimErr == nil, open, live, running)
		}
		if (claimErr == nil) != (open == 1) {
			t.Fatalf("iteration %d: the claim returned %v but %d claims exist", i, claimErr, open)
		}
		if got := h.taskState(t1.ID); got != lifecycle.TaskActive {
			t.Fatalf("iteration %d: task = %s", i, got)
		}
		// Put the dispatch row back for the next reset (when there is one).
		if live == 1 {
			d = h.dispatchesOf(t1.ID, "implement-task")[0]
		} else {
			d = h.recreateImplementDispatch(t1.ID)
		}
	}
}

// recreateImplementDispatch gives a task a queued implement dispatch, as the
// start would have, so a loop can reset it.
func (h *harness) recreateImplementDispatch(taskID uuid.UUID) store.Dispatch {
	h.t.Helper()
	var d *store.Dispatch
	cfg, err := h.srv.freshConfig()
	if err != nil {
		h.t.Fatal(err)
	}
	role := cfg.Assignments["implement-task"]
	model, err := h.srv.modelForPurpose(cfg, "implement-task", role)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := h.srv.Store.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		d, err = store.EnqueueDispatch(context.Background(), tx, "implement-task", role, model, "task", taskID, "race:"+uuid.NewString())
		return err
	}); err != nil {
		h.t.Fatal(err)
	}
	return *d
}

// TestClaimOnOneTaskWhileASiblingStarts is FR-2.7 and FR-6.5: a claim on T1
// racing the start of T2's implementer in the same feature. Never both an open
// claim and a running implement dispatch.
func TestClaimOnOneTaskWhileASiblingStarts(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")
	d2 := h.dispatchesOf(t2.ID, "implement-task")[0]
	var claimWins, startWins int

	for i := 0; i < raceIterations; i++ {
		h.resetTask(t1.ID, h.dispatchesOf(t1.ID, "implement-task")[0].ID)
		h.resetTask(t2.ID, d2.ID)
		var claimErr, startErr error
		var start store.StartResult
		runTogether(
			func() { _, claimErr = h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant()) },
			func() { time.Sleep(lagFor(i)); start, startErr = h.startDispatch(d2.ID) },
		)
		if startErr != nil {
			t.Fatalf("iteration %d: %v", i, startErr)
		}
		openClaims := h.countWhere(`SELECT count(*) FROM work_claims WHERE state = 'open' AND feature_id = $1`, t1.FeatureID)
		running := h.countWhere(`SELECT count(*) FROM dispatches WHERE purpose = 'implement-task' AND state = 'running'`)
		if openClaims > 0 && running > 0 {
			t.Fatalf("iteration %d: an open claim and a running implementer in one feature", i)
		}
		if claimErr == nil {
			claimWins++
			if start.Outcome != store.StartQueued || start.Reason != store.QueueReasonClaimOpen || openClaims != 1 || running != 0 {
				t.Fatalf("iteration %d: claim won, but T2's start = %+v, claims=%d running=%d", i, start, openClaims, running)
			}
			continue
		}
		if _, ok := AsClaimRefusal(claimErr); !ok {
			t.Fatalf("iteration %d: %v", i, claimErr)
		}
		startWins++
		if start.Outcome != store.StartStarted || openClaims != 0 || running != 1 {
			t.Fatalf("iteration %d: the sibling started, but start = %+v, claims=%d running=%d", i, start, openClaims, running)
		}
	}
	requireBothWon(t, "claim", claimWins, "sibling", startWins)
}

// TestSendBackWhileAnAgentRunsOnAnotherTask is FR-6.5: the reviewer sends a
// claimed task back while an agent implements a sibling. The claim is
// returned, and resuming it is refused until the agent has finished.
func TestSendBackWhileAnAgentRunsOnAnotherTask(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")

	res, err := h.srv.ClaimWork(ctx, t1.PublicID, chat)
	if err != nil {
		t.Fatal(err)
	}
	writeIn(t, res.WorkingCopy.Path, "a.go", "package main\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "Added a.go."); err != nil {
		t.Fatal(err)
	}
	// The working copy is free, and T2's implementer starts (no provider is
	// involved: it is running, as far as the governor can tell).
	d2 := h.dispatchesOf(t2.ID, "implement-task")[0]
	if r, err := h.startDispatch(d2.ID); err != nil || r.Outcome != store.StartStarted {
		t.Fatalf("T2's implementer starts: %+v, %v", r, err)
	}
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"request_changes","comments":[{"section_ref":"a.go:1","body":"Add a function.","severity":"major"}],"reasoning":"thin"}`, tiny)
	h.releaseQueue()

	h.eventually("T1 is sent back", func() bool { return h.latestClaim(t1.ID).State == lifecycle.ClaimReturned })
	if got := h.taskState(t2.ID); got != lifecycle.TaskActive {
		t.Fatalf("T2 = %s", got)
	}
	_, err = h.srv.ClaimWork(ctx, t1.PublicID, chat)
	wantRefusal(t, err, "An agent is implementing "+t2.PublicID+" in this feature's working copy.")
	if c := h.latestClaim(t1.ID); c.State != lifecycle.ClaimReturned {
		t.Fatalf("a refused resume leaves the claim returned: %+v", c)
	}

	// The agent finishes: resuming now works, with the comments.
	h.exec(`UPDATE dispatches SET state = 'succeeded', finished_at = now() WHERE id = $1`, d2.ID)
	h.exec(`UPDATE tasks SET state = 'review' WHERE id = $1`, t2.ID)
	back, err := h.srv.ClaimWork(ctx, t1.PublicID, chat)
	if err != nil {
		t.Fatalf("resume after the agent finished: %v", err)
	}
	if !back.Resumed || back.Claim.State != lifecycle.ClaimOpen || back.ReviewComments == nil || len(back.ReviewComments.Comments) != 1 {
		t.Fatalf("resumption = %+v", back)
	}
}

// TestAbandonedFeatureWithAnOpenClaimRaisesNothing is FR-6.5 and SD-16: the
// claim ends with the feature, so the claim sweep has nothing to ask about, and
// the branch watch, which watches only active and review features, raises
// nothing for a commit nobody claimed.
func TestAbandonedFeatureWithAnOpenClaimRaisesNothing(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	f, err := h.srv.featureByPath(ctx, "auth/login")
	if err != nil {
		t.Fatal(err)
	}
	dir := h.worktreeDir(f.ID)

	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.TransitionFeature(ctx, tx, f, lifecycle.FeatAbandon, "sam", map[string]any{"reason": "no longer wanted"})
	}); err != nil {
		t.Fatal(err)
	}
	if c := h.latestClaim(t1.ID); c.State != lifecycle.ClaimEnded || c.EndReason != "abandoned" {
		t.Fatalf("the claim ends with the feature: %+v", c)
	}
	if _, err := store.CurrentClaimFor(ctx, h.srv.Store.Pool, "task", t1.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("no claim is left: %v", err)
	}

	// Backdated, the ended claim is still not swept.
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	if n := h.countWhere(`SELECT count(*) FROM checkpoints WHERE kind = 'claim-stale'`); n != 0 {
		t.Fatalf("the sweep raised %d claim-stale questions on an abandoned feature", n)
	}

	// An unclaimed commit on the abandoned feature's branch is not watched.
	commitAs(t, dir, "late.txt", "late fix")
	h.srv.BranchWatchSweep(ctx)
	if n := h.countWhere(`SELECT count(*) FROM checkpoints WHERE kind = 'unclaimed-commit'`); n != 0 {
		t.Fatalf("the watch raised %d unclaimed-commit notices on an abandoned feature", n)
	}
	if cp := h.pendingOf("unclaimed-commit", "feature", f.ID); cp != nil {
		t.Fatalf("pending notice: %+v", cp)
	}
}

// TestTransitionTaskStateGuard is FR-2.7: two transitions of one task from the
// same state race; exactly one succeeds, and the other gets ErrStaleState.
func TestTransitionTaskStateGuard(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")

	for i := 0; i < raceIterations; i++ {
		h.exec(`UPDATE tasks SET state = 'active' WHERE id = $1`, t1.ID)
		// Both read the task as active before either writes.
		var read sync.WaitGroup
		read.Add(2)
		var errs [2]error
		attempt := func(n int) func() {
			return func() {
				var once sync.Once
				signal := func() { once.Do(read.Done) }
				defer signal() // always signal, so a failed read cannot hang the other side
				errs[n] = h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
					cur, err := store.GetTask(ctx, tx, t1.ID)
					if err != nil {
						return err
					}
					signal()
					read.Wait()
					return store.TransitionTask(ctx, tx, cur, lifecycle.TaskImplemented, "test", nil)
				})
			}
		}
		runTogether(attempt(0), attempt(1))
		ok, stale := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, store.ErrStaleState):
				stale++
			default:
				t.Fatalf("iteration %d: %v", i, err)
			}
		}
		if ok != 1 || stale != 1 {
			t.Fatalf("iteration %d: %d succeeded and %d were stale, want one each (%v)", i, ok, stale, errs)
		}
		if got := h.taskState(t1.ID); got != lifecycle.TaskReview {
			t.Fatalf("iteration %d: task = %s", i, got)
		}
	}
}
