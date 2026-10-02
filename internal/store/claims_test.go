package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
)

// seedActiveTask creates a task in the feature and moves it to active, the
// state a claim holds (SPEC-020 SD-3).
func seedActiveTask(t *testing.T, s *Store, fid uuid.UUID, pos int) *Task {
	t.Helper()
	ctx := context.Background()
	var task *Task
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		task, err = CreateTask(ctx, tx, fid, pos, "", "a task", "", "orchestrator")
		if err != nil {
			return err
		}
		if err := TransitionTask(ctx, tx, task, lifecycle.TaskReadyEvent, "orchestrator", nil); err != nil {
			return err
		}
		return TransitionTask(ctx, tx, task, lifecycle.TaskClaim, "orchestrator", nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func inTx(t *testing.T, s *Store, fn func(tx pgx.Tx) error) {
	t.Helper()
	if err := s.WithTx(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

func auditKinds(t *testing.T, s *Store, refType string, id uuid.UUID, prefix string) []string {
	t.Helper()
	rows, err := s.Pool.Query(context.Background(), `
		SELECT kind FROM audit_events WHERE ref_type = $1 AND ref_id = $2 AND kind LIKE $3
		ORDER BY occurred_at, id`, refType, id, prefix+"%")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		out = append(out, k)
	}
	return out
}

// SPEC-020 FR-2.1, FR-2.2, FR-2.11: a claim runs its machine, each step
// audited on the claimed task, and the guarded update refuses a stale copy.
func TestClaimLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	task := seedActiveTask(t, s, fid, 0)

	var c *Claim
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		c, err = CreateClaim(ctx, tx, "task", task.ID, &fid, "chat", "chat-agent", "mcp", "fp0")
		return err
	})
	if c.State != lifecycle.ClaimOpen || c.LastActivity != "claimed" || c.WorktreeSeen != "fp0" {
		t.Fatalf("created: %+v", c)
	}
	got, err := CurrentClaimFor(ctx, s.Pool, "task", task.ID)
	if err != nil || got.ID != c.ID || got.FeatureID == nil || *got.FeatureID != fid {
		t.Fatalf("current: %+v %v", got, err)
	}
	if open, err := OpenClaimForFeature(ctx, s.Pool, fid); err != nil || open.ID != c.ID {
		t.Fatalf("open for feature: %+v %v", open, err)
	}

	// A copy read now goes stale once the claim moves.
	stale := *c

	// Renew, with activity.
	time.Sleep(5 * time.Millisecond)
	before := c.LastActivityAt
	inTx(t, s, func(tx pgx.Tx) error {
		return TransitionClaim(ctx, tx, c, lifecycle.ClaimEventRenew, "chat-agent", "renewed", map[string]any{"note": "still here"})
	})
	if c.State != lifecycle.ClaimOpen || c.LastActivity != "renewed" || !c.LastActivityAt.After(before) {
		t.Errorf("renewed: %+v", c)
	}

	// Submit sets submitted_at.
	inTx(t, s, func(tx pgx.Tx) error {
		return TransitionClaim(ctx, tx, c, lifecycle.ClaimEventSubmit, "chat-agent", "submitted", nil)
	})
	if c.State != lifecycle.ClaimSubmitted || c.SubmittedAt == nil {
		t.Errorf("submitted: %+v", c)
	}

	// A stale copy (still "open") is refused by the guard, and nothing changes.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		return TransitionClaim(ctx, tx, &stale, lifecycle.ClaimEventRelease, "sam", "", nil)
	})
	if !errors.Is(err, ErrStaleState) {
		t.Errorf("stale transition = %v, want ErrStaleState", err)
	}
	if now, _ := GetClaim(ctx, s.Pool, c.ID); now.State != lifecycle.ClaimSubmitted {
		t.Errorf("a refused transition changed the claim: %+v", now)
	}

	// An illegal event is refused by the machine.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		return TransitionClaim(ctx, tx, c, lifecycle.ClaimEventRenew, "chat-agent", "", nil)
	})
	var ill *lifecycle.IllegalTransitionError
	if !errors.As(err, &ill) {
		t.Errorf("renew while submitted = %v, want an illegal transition", err)
	}

	// Send back, resume, submit, done.
	for _, step := range []struct {
		e        lifecycle.ClaimEvent
		activity string
		want     lifecycle.ClaimState
	}{
		{lifecycle.ClaimEventSendBack, "sent_back", lifecycle.ClaimReturned},
		{lifecycle.ClaimEventResume, "resumed", lifecycle.ClaimOpen},
		{lifecycle.ClaimEventSubmit, "submitted", lifecycle.ClaimSubmitted},
		{lifecycle.ClaimEventDone, "", lifecycle.ClaimEnded},
	} {
		inTx(t, s, func(tx pgx.Tx) error { return TransitionClaim(ctx, tx, c, step.e, "orchestrator", step.activity, nil) })
		if c.State != step.want {
			t.Fatalf("%s: state %s, want %s", step.e, c.State, step.want)
		}
	}
	if c.EndReason != "done" || c.EndedBy != "" || c.EndedAt == nil {
		t.Errorf("done: %+v", c)
	}
	if _, err := CurrentClaimFor(ctx, s.Pool, "task", task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an ended claim isn't current: %v", err)
	}
	if last, err := LatestClaimFor(ctx, s.Pool, "task", task.ID); err != nil || last.ID != c.ID || last.EndReason != "done" {
		t.Errorf("latest: %+v %v", last, err)
	}

	// The audit trail is on the task, in order, with the claim's identity.
	want := []string{"claim.claimed", "claim.renewed", "claim.submitted", "claim.sent_back",
		"claim.resumed", "claim.submitted", "claim.ended"}
	if kinds := auditKinds(t, s, "task", task.ID, "claim."); len(kinds) != len(want) {
		t.Errorf("audit kinds = %v, want %v", kinds, want)
	} else {
		for i := range want {
			if kinds[i] != want[i] {
				t.Errorf("audit kinds = %v, want %v", kinds, want)
				break
			}
		}
	}
	var kind, via, claimID, note string
	if err := s.Pool.QueryRow(ctx, `
		SELECT payload->>'kind', payload->>'via', payload->>'claim_id', COALESCE(payload->>'note', '')
		FROM audit_events WHERE kind = 'claim.renewed' AND ref_id = $1`, task.ID).
		Scan(&kind, &via, &claimID, &note); err != nil {
		t.Fatal(err)
	}
	if kind != "chat" || via != "mcp" || claimID != c.ID.String() || note != "still here" {
		t.Errorf("renewed payload: %s %s %s %s", kind, via, claimID, note)
	}
}

// Release and abandon end a claim with their reasons and say who ended it
// (SPEC-020 FR-2.1).
func TestClaimEndReasons(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)

	for _, tc := range []struct {
		e      lifecycle.ClaimEvent
		reason string
		kind   string
	}{
		{lifecycle.ClaimEventRelease, "released", "claim.released"},
		{lifecycle.ClaimEventAbandon, "abandoned", "claim.ended"},
	} {
		task := seedActiveTask(t, s, fid, 0)
		var c *Claim
		inTx(t, s, func(tx pgx.Tx) error {
			var err error
			c, err = CreateClaim(ctx, tx, "task", task.ID, &fid, "person", "sam", "ui", "")
			if err != nil {
				return err
			}
			return TransitionClaim(ctx, tx, c, tc.e, "alex", "", nil)
		})
		got, _ := GetClaim(ctx, s.Pool, c.ID)
		if got.EndReason != tc.reason || got.EndedBy != "alex" || got.EndedAt == nil || got.State != lifecycle.ClaimEnded {
			t.Errorf("%s: %+v", tc.e, got)
		}
		kinds := auditKinds(t, s, "task", task.ID, "claim.")
		if len(kinds) != 2 || kinds[1] != tc.kind {
			t.Errorf("%s: audit %v, want last %s", tc.e, kinds, tc.kind)
		}
	}
}

// SPEC-020 FR-2.1: one claim that hasn't ended per item, one open claim per
// feature, and the database's own check that end_reason goes with ended.
func TestClaimUniqueness(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	t1 := seedActiveTask(t, s, fid, 0)
	t2 := seedActiveTask(t, s, fid, 1)

	var c1 *Claim
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		c1, err = CreateClaim(ctx, tx, "task", t1.ID, &fid, "chat", "chat-agent", "mcp", "")
		return err
	})

	// A second claim that hasn't ended on the same task fails.
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := CreateClaim(ctx, tx, "task", t1.ID, &fid, "person", "sam", "ui", "")
		return err
	})
	if err == nil {
		t.Error("a second claim on the same task should violate work_claims_current")
	}

	// A second open claim on the same feature (another task) fails.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := CreateClaim(ctx, tx, "task", t2.ID, &fid, "person", "sam", "ui", "")
		return err
	})
	if err == nil {
		t.Error("a second open claim on the feature should violate work_claims_open_feature")
	}

	// Once the first is submitted it no longer holds the working copy, so
	// another task's claim may open; the first still blocks its own task.
	inTx(t, s, func(tx pgx.Tx) error {
		return TransitionClaim(ctx, tx, c1, lifecycle.ClaimEventSubmit, "chat-agent", "submitted", nil)
	})
	inTx(t, s, func(tx pgx.Tx) error {
		_, err := CreateClaim(ctx, tx, "task", t2.ID, &fid, "person", "sam", "ui", "")
		return err
	})

	// A claim that holds no working copy (no feature) doesn't collide on it.
	t3 := seedActiveTask(t, s, fid, 2)
	inTx(t, s, func(tx pgx.Tx) error {
		_, err := CreateClaim(ctx, tx, "task", t3.ID, nil, "person", "sam", "ui", "")
		return err
	})

	// end_reason is set exactly when the state is ended.
	if _, err := s.Pool.Exec(ctx, `UPDATE work_claims SET state = 'ended' WHERE id = $1`, c1.ID); err == nil {
		t.Error("ended without an end_reason should violate the check")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE work_claims SET end_reason = 'done' WHERE id = $1`, c1.ID); err == nil {
		t.Error("an end_reason on a claim that hasn't ended should violate the check")
	}
}

// RecordClaimActivity touches only an open claim; the sweep's list is open
// and returned claims; LockCurrentClaimFor finds the claim that hasn't ended.
func TestClaimActivityAndSweepList(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	t1 := seedActiveTask(t, s, fid, 0)
	t2 := seedActiveTask(t, s, fid, 1)

	var c1, c2 *Claim
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		if c1, err = CreateClaim(ctx, tx, "task", t1.ID, &fid, "chat", "chat-agent", "mcp", "fp0"); err != nil {
			return err
		}
		if err = TransitionClaim(ctx, tx, c1, lifecycle.ClaimEventSubmit, "chat-agent", "submitted", nil); err != nil {
			return err
		}
		if c2, err = CreateClaim(ctx, tx, "task", t2.ID, &fid, "person", "sam", "ui", "fp0"); err != nil {
			return err
		}
		return nil
	})

	var ok bool
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		ok, err = RecordClaimActivity(ctx, tx, c2.ID, "worktree", "fp1")
		return err
	})
	if got, _ := GetClaim(ctx, s.Pool, c2.ID); !ok || got.LastActivity != "worktree" || got.WorktreeSeen != "fp1" {
		t.Errorf("open claim activity: ok=%v %+v", ok, got)
	}
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		ok, err = RecordClaimActivity(ctx, tx, c1.ID, "worktree", "fp9")
		return err
	})
	if got, _ := GetClaim(ctx, s.Pool, c1.ID); ok || got.LastActivity != "submitted" || got.WorktreeSeen != "fp0" {
		t.Errorf("a submitted claim must not take activity: ok=%v %+v", ok, got)
	}

	// Only the open claim is swept; the submitted one is not.
	sweep, err := ClaimsToSweep(ctx, s.Pool)
	if err != nil || len(sweep) != 1 || sweep[0].ID != c2.ID {
		t.Errorf("sweep = %+v %v", sweep, err)
	}
	inTx(t, s, func(tx pgx.Tx) error {
		if err := TransitionClaim(ctx, tx, c1, lifecycle.ClaimEventSendBack, "orchestrator", "sent_back", nil); err != nil {
			return err
		}
		locked, err := LockCurrentClaimFor(ctx, tx, "task", t1.ID)
		if err != nil || locked.ID != c1.ID || locked.State != lifecycle.ClaimReturned {
			t.Errorf("lock: %+v %v", locked, err)
		}
		return nil
	})
	if sweep, _ := ClaimsToSweep(ctx, s.Pool); len(sweep) != 2 {
		t.Errorf("a returned claim is swept too: %+v", sweep)
	}
}

// SPEC-020 SD-16: abandoning a feature ends every claim on it, each audited.
func TestEndClaimsForFeature(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	other := seedFeature2(t, s)
	t1 := seedActiveTask(t, s, fid, 0)
	t2 := seedActiveTask(t, s, fid, 1)
	t3 := seedActiveTask(t, s, other, 0)

	var kept *Claim
	inTx(t, s, func(tx pgx.Tx) error {
		c1, err := CreateClaim(ctx, tx, "task", t1.ID, &fid, "chat", "chat-agent", "mcp", "")
		if err != nil {
			return err
		}
		if err := TransitionClaim(ctx, tx, c1, lifecycle.ClaimEventSubmit, "chat-agent", "submitted", nil); err != nil {
			return err
		}
		if _, err := CreateClaim(ctx, tx, "task", t2.ID, &fid, "person", "sam", "ui", ""); err != nil {
			return err
		}
		kept, err = CreateClaim(ctx, tx, "task", t3.ID, &other, "person", "sam", "ui", "")
		return err
	})

	var ended []Claim
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		ended, err = EndClaimsForFeature(ctx, tx, fid, "sam", "feature abandoned")
		return err
	})
	if len(ended) != 2 {
		t.Fatalf("ended %d claims, want 2", len(ended))
	}
	for _, c := range ended {
		got, _ := GetClaim(ctx, s.Pool, c.ID)
		if got.State != lifecycle.ClaimEnded || got.EndReason != "abandoned" || got.EndedBy != "sam" {
			t.Errorf("claim %s: %+v", c.ID, got)
		}
	}
	if got, _ := GetClaim(ctx, s.Pool, kept.ID); got.State != lifecycle.ClaimOpen {
		t.Errorf("another feature's claim was touched: %+v", got)
	}
	var reason string
	if err := s.Pool.QueryRow(ctx, `SELECT payload->>'reason' FROM audit_events
		WHERE kind = 'claim.ended' AND ref_id = $1`, t2.ID).Scan(&reason); err != nil || reason != "feature abandoned" {
		t.Errorf("audit reason = %q %v", reason, err)
	}
	inTx(t, s, func(tx pgx.Tx) error {
		again, err := EndClaimsForFeature(ctx, tx, fid, "sam", "again")
		if len(again) != 0 {
			t.Errorf("second call ended %d", len(again))
		}
		return err
	})
}

// seedFeature2 makes a second feature in another initiative.
func seedFeature2(t *testing.T, s *Store) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var fid uuid.UUID
	inTx(t, s, func(tx pgx.Tx) error {
		in, err := CreateInitiative(ctx, tx, nil, "other", "Other", "", "sam")
		if err != nil {
			return err
		}
		f, err := CreateFeature(ctx, tx, in.ID, "thing", "Thing", "", "sam")
		fid = f.ID
		return err
	})
	return fid
}

// SPEC-020 FR-2.7: TransitionTask refuses a task that has moved since it was
// read, rather than overwriting it.
func TestTransitionTaskStateGuard(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	task := seedActiveTask(t, s, fid, 0)
	stale := *task // read while active

	// The release event leaves the task active (FR-2.2).
	inTx(t, s, func(tx pgx.Tx) error {
		return TransitionTask(ctx, tx, task, lifecycle.TaskRelease, "sam", nil)
	})
	if task.State != lifecycle.TaskActive {
		t.Errorf("release: %s", task.State)
	}
	inTx(t, s, func(tx pgx.Tx) error {
		return TransitionTask(ctx, tx, task, lifecycle.TaskImplemented, "orchestrator", nil)
	})

	// The stale copy still says active; the row is in review.
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		return TransitionTask(ctx, tx, &stale, lifecycle.TaskImplemented, "orchestrator", nil)
	})
	if !errors.Is(err, ErrStaleState) {
		t.Errorf("stale transition = %v, want ErrStaleState", err)
	}
	if stale.State != lifecycle.TaskActive {
		t.Errorf("a refused transition changed the caller's copy: %s", stale.State)
	}
	if got, _ := GetTask(ctx, s.Pool, task.ID); got.State != lifecycle.TaskReview {
		t.Errorf("task = %s, want review", got.State)
	}
	// The refusal wrote no audit row for the lost attempt.
	var n int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'task.transition' AND ref_id = $1
		AND payload->>'event' = 'implemented'`, task.ID).Scan(&n)
	if n != 1 {
		t.Errorf("implemented audited %d times, want 1", n)
	}
}

// SPEC-020 FR-1.1, FR-1.2: executions record who did a round, with the
// database's checks, and the round counts request_changes transitions.
func TestExecutions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	task := seedActiveTask(t, s, fid, 0)

	if r, err := CurrentTaskRound(ctx, s.Pool, task.ID); err != nil || r != 1 {
		t.Fatalf("round = %d %v, want 1", r, err)
	}

	// An agent starts the task: round 1; a retried attempt adds nothing.
	var disp1 uuid.UUID
	inTx(t, s, func(tx pgx.Tx) error {
		d, err := EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m1", "task", task.ID, "impl:1")
		if err != nil {
			return err
		}
		disp1 = d.ID
		if err := RecordAgentExecution(ctx, tx, task.ID, d.ID, "implementer", "m1", "abc123"); err != nil {
			return err
		}
		return RecordAgentExecution(ctx, tx, task.ID, d.ID, "implementer", "m1", "abc123")
	})
	execs, err := ExecutionsFor(ctx, s.Pool, "task", task.ID)
	if err != nil || len(execs) != 1 {
		t.Fatalf("executions = %+v %v", execs, err)
	}
	if e := execs[0]; e.Kind != "agent" || !e.Measured || e.Round != 1 || e.Actor != "implementer" || e.Model != "m1" ||
		e.Via != "agent" || e.StartHead != "abc123" || e.DispatchID == nil || *e.DispatchID != disp1 ||
		e.ClaimID != nil || e.Inferred || e.SubmittedAt != nil {
		t.Errorf("agent row: %+v", e)
	}
	if un, err := TaskUnmeasured(ctx, s.Pool, task.ID); err != nil || un {
		t.Errorf("an agent-only task is measured: %v %v", un, err)
	}
	inTx(t, s, func(tx pgx.Tx) error {
		return MarkExecutionSubmitted(ctx, tx, struct{ DispatchID, ClaimID *uuid.UUID }{DispatchID: &disp1})
	})
	if execs, _ := ExecutionsFor(ctx, s.Pool, "task", task.ID); execs[0].SubmittedAt == nil {
		t.Error("the agent's row should be submitted")
	}

	// A review sends it back: round 2. The chat agent claims it.
	inTx(t, s, func(tx pgx.Tx) error {
		if err := TransitionTask(ctx, tx, task, lifecycle.TaskImplemented, "orchestrator", nil); err != nil {
			return err
		}
		return TransitionTask(ctx, tx, task, lifecycle.TaskRequestChanges, "orchestrator", nil)
	})
	if r, _ := CurrentTaskRound(ctx, s.Pool, task.ID); r != 2 {
		t.Errorf("round after one request_changes = %d, want 2", r)
	}
	var c *Claim
	var ex *Execution
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		if c, err = CreateClaim(ctx, tx, "task", task.ID, &fid, "chat", "chat-agent", "mcp", ""); err != nil {
			return err
		}
		ex, err = RecordClaimExecution(ctx, tx, "task", task.ID, c, "def456")
		return err
	})
	if ex.Kind != "chat" || ex.Measured || ex.Round != 2 || ex.Via != "mcp" || ex.Actor != "chat-agent" ||
		ex.ClaimID == nil || *ex.ClaimID != c.ID || ex.DispatchID != nil || ex.StartHead != "def456" {
		t.Errorf("claim row: %+v", ex)
	}
	if un, _ := TaskUnmeasured(ctx, s.Pool, task.ID); !un {
		t.Error("a task with a chat execution is unmeasured")
	}

	// The same claim recording a second row in the same round is refused;
	// after a send-back (a new round) its resume is a new row.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := RecordClaimExecution(ctx, tx, "task", task.ID, c, "x")
		return err
	})
	if err == nil {
		t.Error("a claim has one row per round")
	}
	inTx(t, s, func(tx pgx.Tx) error {
		if err := TransitionClaim(ctx, tx, c, lifecycle.ClaimEventSubmit, "chat-agent", "submitted", nil); err != nil {
			return err
		}
		if err := MarkExecutionSubmitted(ctx, tx, struct{ DispatchID, ClaimID *uuid.UUID }{ClaimID: &c.ID}); err != nil {
			return err
		}
		if err := TransitionTask(ctx, tx, task, lifecycle.TaskImplemented, "orchestrator", nil); err != nil {
			return err
		}
		if err := TransitionTask(ctx, tx, task, lifecycle.TaskRequestChanges, "orchestrator", nil); err != nil {
			return err
		}
		if err := TransitionClaim(ctx, tx, c, lifecycle.ClaimEventSendBack, "orchestrator", "sent_back", nil); err != nil {
			return err
		}
		if err := TransitionClaim(ctx, tx, c, lifecycle.ClaimEventResume, "chat-agent", "resumed", nil); err != nil {
			return err
		}
		_, err := RecordClaimExecution(ctx, tx, "task", task.ID, c, "ghi789")
		return err
	})
	execs, _ = ExecutionsFor(ctx, s.Pool, "task", task.ID)
	if len(execs) != 3 || execs[1].Round != 2 || execs[1].SubmittedAt == nil || execs[2].Round != 3 || execs[2].SubmittedAt != nil {
		t.Errorf("rows after resume: %+v", execs)
	}

	// The database's checks.
	bad := func(name, sql string, args ...any) {
		t.Helper()
		if _, err := s.Pool.Exec(ctx, sql, args...); err == nil {
			t.Errorf("%s should violate a check", name)
		}
	}
	bad("an agent row without a dispatch", `INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, via, measured)
		VALUES (gen_random_uuid(), 'task', $1, 1, 'agent', 'r', 'agent', true)`, task.ID)
	bad("an unmeasured agent row", `INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, dispatch_id, via, measured)
		VALUES (gen_random_uuid(), 'task', $1, 1, 'agent', 'r', $2, 'agent', false)`, task.ID, disp1)
	bad("a chat row without a claim", `INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, via, measured)
		VALUES (gen_random_uuid(), 'task', $1, 1, 'chat', 'r', 'mcp', false)`, task.ID)
	bad("a measured person row", `INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, claim_id, via, measured)
		VALUES (gen_random_uuid(), 'task', $1, 5, 'person', 'r', $2, 'ui', true)`, task.ID, c.ID)
	bad("a second row for one dispatch", `INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, dispatch_id, via, measured)
		VALUES (gen_random_uuid(), 'task', $1, 9, 'agent', 'r', $2, 'agent', true)`, task.ID, disp1)

	// Deleting an item's executions leaves other items' alone.
	other := seedActiveTask(t, s, fid, 1)
	inTx(t, s, func(tx pgx.Tx) error {
		d, err := EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m1", "task", other.ID, "impl:other")
		if err != nil {
			return err
		}
		return RecordAgentExecution(ctx, tx, other.ID, d.ID, "implementer", "m1", "")
	})
	inTx(t, s, func(tx pgx.Tx) error { return DeleteExecutionsFor(ctx, tx, "task", task.ID) })
	if execs, _ := ExecutionsFor(ctx, s.Pool, "task", task.ID); len(execs) != 0 {
		t.Errorf("not deleted: %+v", execs)
	}
	if execs, _ := ExecutionsFor(ctx, s.Pool, "task", other.ID); len(execs) != 1 {
		t.Errorf("another task's rows were touched: %+v", execs)
	}

	// Submitting needs a run or a claim.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		return MarkExecutionSubmitted(ctx, tx, struct{ DispatchID, ClaimID *uuid.UUID }{})
	})
	if err == nil {
		t.Error("MarkExecutionSubmitted with neither should be an error")
	}
}

// SPEC-020 FR-5.6: watched_head and the commits Subutai made.
func TestWorktreeWatch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	fid := seedFeature(t, s)
	var w *Worktree
	inTx(t, s, func(tx pgx.Tx) error {
		var err error
		w, err = CreateWorktreeRow(ctx, tx, fid, "wt", "br", "sam")
		return err
	})
	if got, err := LiveWorktreeForFeature(ctx, s.Pool, fid); err != nil || got.WatchedHead != "" {
		t.Fatalf("watched head starts empty: %+v %v", got, err)
	}
	if err := SetWatchedHead(ctx, s.Pool, w.ID, "abc"); err != nil {
		t.Fatal(err)
	}
	if got, _ := LiveWorktreeForFeature(ctx, s.Pool, fid); got.WatchedHead != "abc" {
		t.Errorf("watched head = %q", got.WatchedHead)
	}
	if live, _ := s.LiveWorktrees(ctx); len(live) != 1 || live[0].WatchedHead != "abc" {
		t.Errorf("live worktrees = %+v", live)
	}

	inTx(t, s, func(tx pgx.Tx) error {
		if err := RecordWorktreeCommit(ctx, tx, w.ID, "h1", "implementation"); err != nil {
			return err
		}
		return RecordWorktreeCommit(ctx, tx, w.ID, "h1", "implementation") // twice is fine
	})
	if ok, err := IsWorktreeCommit(ctx, s.Pool, w.ID, "h1"); err != nil || !ok {
		t.Errorf("h1: %v %v", ok, err)
	}
	if ok, _ := IsWorktreeCommit(ctx, s.Pool, w.ID, "h2"); ok {
		t.Error("h2 was never recorded")
	}
	err := s.WithTx(ctx, func(tx pgx.Tx) error { return RecordWorktreeCommit(ctx, tx, w.ID, "h3", "bogus") })
	if err == nil {
		t.Error("an unknown act should violate the check")
	}
}
