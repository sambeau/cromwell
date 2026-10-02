package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
)

// startAs starts a spike for any executor.
func (fx *spikeFixture) startAs(t *testing.T, id uuid.UUID, st SpikeStart) *Spike {
	t.Helper()
	ctx := context.Background()
	st.BaseCommit, st.WorktreePath, st.Actor = "abc123", "worktrees/spk-001", "sam"
	st.RefsAtStart = map[string]string{"refs/heads/main": "abc123"}
	var sp *Spike
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		sp, err = StartSpike(ctx, tx, id, st)
		return err
	})
	if err != nil {
		t.Fatalf("StartSpike: %v", err)
	}
	return sp
}

// SPEC-021 FR-11.2: a chat spike's deadline is its start plus its time box,
// to the second, and it has no token budget.
func TestStartSpikeForChatSetsTheDeadline(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.create(t, fx.parent.ID, nil, "Q")
	started := fx.startAs(t, sp.ID, SpikeStart{Executor: ExecutorChat, TimeBoxHours: 4})
	if started.Executor != ExecutorChat || started.TimeBoxHours == nil || *started.TimeBoxHours != 4 ||
		started.TokenBudget != nil || started.DeadlineAt == nil || started.StartedAt == nil {
		t.Fatalf("started = %+v", started)
	}
	if got, want := started.DeadlineAt.Sub(*started.StartedAt), 4*time.Hour; got.Truncate(time.Second) != want {
		t.Errorf("deadline is %v after the start, want %v", got, want)
	}
	var payload string
	if err := fx.s.Pool.QueryRow(ctx, `SELECT payload::text FROM audit_events
		WHERE kind = 'spike.started' AND ref_id = $1`, sp.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"executor": "chat"`) || !strings.Contains(payload, `"time_box_hours": 4`) ||
		strings.Contains(payload, "budget") {
		t.Errorf("audit payload = %s", payload)
	}

	// An agent spike records its budget, and no deadline.
	ag := fx.create(t, fx.parent.ID, nil, "Q2")
	a := fx.startAs(t, ag.ID, SpikeStart{Executor: ExecutorAgent, Budget: 500, BudgetSource: BudgetFromDefault})
	if a.Executor != ExecutorAgent || a.TokenBudget == nil || *a.TokenBudget != 500 || a.TimeBoxHours != nil || a.DeadlineAt != nil {
		t.Errorf("agent spike = %+v", a)
	}

	// A start without an executor is refused before it touches the row.
	bad := fx.create(t, fx.parent.ID, nil, "Q3")
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := StartSpike(ctx, tx, bad.ID, SpikeStart{Executor: "robot"})
		return err
	})
	if err == nil {
		t.Error("an unknown executor should be refused")
	}
}

// SPEC-021 FR-11.1, FR-15.1: LockSpike reads the row, and SpikesPastDeadline
// lists only running chat and person spikes whose deadline has passed.
func TestSpikesPastDeadline(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	chat := fx.startAs(t, fx.create(t, fx.parent.ID, nil, "chat").ID, SpikeStart{Executor: ExecutorChat, TimeBoxHours: 1})
	person := fx.startAs(t, fx.create(t, fx.parent.ID, nil, "person").ID, SpikeStart{Executor: ExecutorPerson, TimeBoxHours: 2})
	agent := fx.startAs(t, fx.create(t, fx.parent.ID, nil, "agent").ID, SpikeStart{Executor: ExecutorAgent, Budget: 9})
	fx.create(t, fx.parent.ID, nil, "idea")

	if got, err := SpikesPastDeadline(ctx, fx.s.Pool); err != nil || len(got) != 0 {
		t.Fatalf("before any deadline: %v %v", got, err)
	}
	if _, err := fx.s.Pool.Exec(ctx, `UPDATE spikes SET deadline_at = now() - interval '1 minute'
		WHERE id = ANY($1)`, []uuid.UUID{chat.ID, person.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := SpikesPastDeadline(ctx, fx.s.Pool)
	if err != nil || len(got) != 2 {
		t.Fatalf("past deadline = %v, %v", got, err)
	}
	for _, sp := range got {
		if sp.ID == agent.ID {
			t.Error("an agent spike has no deadline")
		}
	}
	// An ended one is not listed.
	fx.end(t, chat.ID, SpikeTimeBox, "")
	if got, _ = SpikesPastDeadline(ctx, fx.s.Pool); len(got) != 1 || got[0].ID != person.ID {
		t.Errorf("after ending one: %v", got)
	}

	err = fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		locked, err := LockSpike(ctx, tx, person.ID)
		if err != nil || locked.ID != person.ID || locked.Executor != ExecutorPerson {
			t.Errorf("LockSpike = %+v, %v", locked, err)
		}
		if _, err := LockSpike(ctx, tx, uuid.New()); err != ErrNotFound {
			t.Errorf("LockSpike of nothing = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if e := SpikeEndingOf(SpikeTimeBox); e.Phrase != "Reached its time box" || e.Run != "The spike reached the end of its time box." ||
		e.Commit != "time box ended" || e.Lead == "" ||
		e.NotAnswered != "Not answered: the spike reached the end of its time box before it reached an answer." {
		t.Errorf("time_box ending = %+v", e)
	}
}

// SPEC-021 FR-11.1: each bad combination the new checks name is refused, and
// each good one accepted.
func TestSpikeExecutorChecks(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	fresh := fx.create(t, fx.parent.ID, nil, "A question")
	update := func(set string) error {
		_, err := fx.s.Pool.Exec(ctx, `UPDATE spikes SET `+set+` WHERE id = $1`, fresh.ID)
		return err
	}
	chatStarted := `executor = 'chat', time_box_hours = 4, deadline_at = now() + interval '4 hours', started_at = now()`
	agentStarted := `executor = 'agent', token_budget = 100, started_at = now()`

	for _, tc := range []struct {
		name, constraint, set string
	}{
		{"started without an executor", "spikes_started", `state = 'running', token_budget = 100, started_at = now()`},
		{"a running chat spike without a deadline", "spikes_started",
			`state = 'running', executor = 'chat', time_box_hours = 4, started_at = now()`},
		{"a running chat spike without a time box", "spikes_started",
			`state = 'running', executor = 'chat', deadline_at = now(), started_at = now()`},
		{"a running agent spike without a budget", "spikes_started", `state = 'running', executor = 'agent', started_at = now()`},
		{"an agent spike ended as time_box", "spikes_ended_how_executor", `state = 'ended', ` + agentStarted + `, ended_how = 'time_box'`},
		{"a chat spike ended at its budget", "spikes_ended_how_executor", `state = 'ended', ` + chatStarted + `, ended_how = 'budget'`},
		{"a person spike ended at its turn limit", "spikes_ended_how_executor",
			`state = 'ended', ` + strings.Replace(chatStarted, "'chat'", "'person'", 1) + `, ended_how = 'turn_limit'`},
		{"an unknown executor", "spikes_executor_values", `executor = 'robot'`},
		{"a time box of 0", "spikes_time_box_positive", `executor = 'chat', time_box_hours = 0`},
		{"a time box of 169", "spikes_time_box_positive", `executor = 'chat', time_box_hours = 169`},
		{"both limits on a chat spike", "spikes_one_limit", `state = 'running', ` + chatStarted + `, token_budget = 100`},
		{"an agent spike with a deadline", "spikes_one_limit", `state = 'running', ` + agentStarted + `, deadline_at = now()`},
		{"an agent spike with a time box", "spikes_one_limit", `state = 'running', ` + agentStarted + `, time_box_hours = 4`},
		{"closed after a run, without an executor", "spikes_started",
			`state = 'closed', closed_as = 'answered', ended_how = 'failed', token_budget = 100, started_at = now()`},
		{"closed after a run, ended_how a time box on an agent spike", "spikes_ended_how_executor",
			`state = 'closed', closed_as = 'answered', ended_how = 'time_box', ` + agentStarted},
	} {
		err := update(tc.set)
		if err == nil || !strings.Contains(err.Error(), tc.constraint) {
			t.Errorf("%s: err = %v, want a violation of %s", tc.name, err, tc.constraint)
		}
	}

	for _, tc := range []struct{ name, set string }{
		{"a chat spike ended at its time box", `state = 'ended', ` + chatStarted + `, ended_how = 'time_box'`},
		{"then closed", `state = 'closed', closed_as = 'answered'`},
	} {
		if err := update(tc.set); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}

	agent := fx.create(t, fx.parent.ID, nil, "agent")
	if _, err := fx.s.Pool.Exec(ctx, `UPDATE spikes SET state = 'ended', `+agentStarted+`, ended_how = 'budget' WHERE id = $1`, agent.ID); err != nil {
		t.Errorf("an agent spike ended at its budget: %v", err)
	}

	// A claim ended `expired` is accepted; one with a made-up reason is not.
	sp := fx.startAs(t, fx.create(t, fx.parent.ID, nil, "claimed").ID, SpikeStart{Executor: ExecutorChat, TimeBoxHours: 4})
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		c, err := CreateClaim(ctx, tx, "spike", sp.ID, nil, "chat", "chat-agent", "mcp", "")
		if err != nil {
			return err
		}
		if err := SetClaimDeadline(ctx, tx, c.ID, *sp.DeadlineAt); err != nil {
			return err
		}
		return TransitionClaim(ctx, tx, c, lifecycle.ClaimEventExpire, "orchestrator", "", nil)
	})
	if err != nil {
		t.Fatalf("expire a claim: %v", err)
	}
	c, err := LatestClaimFor(ctx, fx.s.Pool, "spike", sp.ID)
	if err != nil || c.State != lifecycle.ClaimEnded || c.EndReason != "expired" || c.EndedBy != "" ||
		c.DeadlineAt == nil || !c.DeadlineAt.Equal(*sp.DeadlineAt) {
		t.Errorf("expired claim = %+v, %v", c, err)
	}
	if n := fx.countAudit(t, sp.ID, "claim.expired"); n != 1 {
		t.Errorf("claim.expired rows = %d, want 1", n)
	}
	_, err = fx.s.Pool.Exec(ctx, `UPDATE work_claims SET end_reason = 'forgotten' WHERE id = $1`, c.ID)
	if err == nil || !strings.Contains(err.Error(), "work_claims_end_reason_values") {
		t.Errorf("a made-up end reason: %v", err)
	}
	_, err = fx.s.Pool.Exec(ctx, `UPDATE work_claims SET end_reason = NULL WHERE id = $1`, c.ID)
	if err == nil || !strings.Contains(err.Error(), "work_claims_end_reason") {
		t.Errorf("an ended claim with no reason: %v", err)
	}
}

func (fx *spikeFixture) countAudit(t *testing.T, id uuid.UUID, kind string) int {
	t.Helper()
	var n int
	if err := fx.s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_events WHERE kind = $1 AND ref_id = $2`, kind, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// SPEC-021 FR-13.6: a save notes `findings` on an open claim, audits it and
// withdraws a pending claim-stale; on a claim that isn't open it does nothing.
func TestRecordSpikeFindingsActivity(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.startAs(t, fx.create(t, fx.parent.ID, nil, "q").ID, SpikeStart{Executor: ExecutorPerson, TimeBoxHours: 4})
	var claim *Claim
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if claim, err = CreateClaim(ctx, tx, "spike", sp.ID, nil, "person", "sam", "ui", ""); err != nil {
			return err
		}
		_, err = CreateCheckpoint(ctx, tx, "claim-stale", "spike", sp.ID, "Is anyone still on this?", map[string]any{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var open bool
	err = fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		open, err = RecordSpikeFindingsActivity(ctx, tx, claim.ID, "sam")
		return err
	})
	if err != nil || !open {
		t.Fatalf("open claim: %v %v", open, err)
	}
	got, _ := GetClaim(ctx, fx.s.Pool, claim.ID)
	if got.LastActivity != "findings" || got.LastActivityAt.Before(claim.LastActivityAt) {
		t.Errorf("claim = %+v", got)
	}
	if n := fx.countAudit(t, sp.ID, "claim.activity"); n != 1 {
		t.Errorf("claim.activity rows = %d, want 1", n)
	}
	if _, err := PendingCheckpointFor(ctx, fx.s.Pool, "claim-stale", "spike", sp.ID); err != ErrNotFound {
		t.Errorf("claim-stale should have been withdrawn: %v", err)
	}

	// A submitted claim isn't open: nothing changes, nothing is audited.
	err = fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		return TransitionClaim(ctx, tx, claim, lifecycle.ClaimEventSubmit, "sam", "submitted", nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		open, err = RecordSpikeFindingsActivity(ctx, tx, claim.ID, "sam")
		return err
	})
	if err != nil || open {
		t.Errorf("submitted claim: %v %v", open, err)
	}
	if n := fx.countAudit(t, sp.ID, "claim.activity"); n != 1 {
		t.Errorf("claim.activity rows = %d, want still 1", n)
	}
}

// SPEC-021 FR-12.2: a spike's agent execution is written once per dispatch,
// in round 1, whatever the number of attempts.
func TestRecordAgentExecutionForASpike(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.startAs(t, fx.create(t, fx.parent.ID, nil, "q").ID, SpikeStart{Executor: ExecutorAgent, Budget: 100, BudgetSource: BudgetFromDefault})
	var d *Dispatch
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		d, err = EnqueueDispatch(ctx, tx, "run-spike", "spike-runner", "model-a", "spike", sp.ID, "run-spike:"+sp.ID.String())
		if err != nil {
			return err
		}
		if err := RecordAgentExecution(ctx, tx, "spike", sp.ID, d.ID, d.Role, d.Model, ""); err != nil {
			return err
		}
		return RecordAgentExecution(ctx, tx, "spike", sp.ID, d.ID, d.Role, d.Model, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	execs, err := ExecutionsFor(ctx, fx.s.Pool, "spike", sp.ID)
	if err != nil || len(execs) != 1 {
		t.Fatalf("executions = %+v, %v", execs, err)
	}
	e := execs[0]
	if e.Round != 1 || e.Kind != ExecutorAgent || e.Actor != "spike-runner" || e.Model != "model-a" || !e.Measured ||
		e.Inferred || e.DispatchID == nil || *e.DispatchID != d.ID || e.Via != "agent" {
		t.Errorf("execution = %+v", e)
	}
}

// TestMigration0016BackfillsSpikes is SPEC-021 FR-11.1: on a database holding
// stage-1 spikes (an idea, a running one, an ended one, a closed one that ran
// and one closed without a run), every started spike becomes an agent spike
// with one inferred execution for its run, and the idea and the unrun closed
// spike stay without an executor. The new checks then hold.
func TestMigration0016BackfillsSpikes(t *testing.T) {
	conn := freshConn(t)
	ctx := context.Background()
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var before, sixteen []migration
	for _, m := range ms {
		switch {
		case m.version < 16:
			before = append(before, m)
		case m.version == 16:
			sixteen = append(sixteen, m)
		}
	}
	if len(sixteen) != 1 {
		t.Fatalf("migration 16 not found")
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE schema_migrations (
		version integer PRIMARY KEY, name text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, conn, map[int]bool{}, before); err != nil {
		t.Fatalf("migrate to 0015: %v", err)
	}

	t0 := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	in := uuid.New()
	exec(`INSERT INTO initiatives (id, slug, name) VALUES ($1, 'pf', 'Platform')`, in)
	idea := uuid.New()
	exec(`INSERT INTO spikes (id, initiative_id, question, created_by, created_via)
		VALUES ($1, $2, 'q', 'sam', 'ui')`, idea, in)
	started := func(state, how, closedAs string) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO spikes (id, initiative_id, question, state, token_budget, started_at, ended_how, closed_as, created_by, created_via)
			VALUES ($1, $2, 'q', $3, 1000, $4, NULLIF($5, ''), NULLIF($6, ''), 'sam', 'ui')`, id, in, state, at(1), how, closedAs)
		return id
	}
	running := started("running", "", "")
	ended := started("ended", "budget", "")
	closed := started("closed", "concluded", "answered")
	closedUnrun := uuid.New()
	exec(`INSERT INTO spikes (id, initiative_id, question, state, closed_as, created_by, created_via)
		VALUES ($1, $2, 'q', 'closed', 'unanswered', 'sam', 'ui')`, closedUnrun, in)

	dispatch := func(spikeID uuid.UUID, purpose string) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key)
			VALUES ($1, 'running', $2, 'spike-runner', 'model-a', 'spike', $3, $4)`, id, purpose, spikeID, id.String())
		return id
	}
	audit := func(sec int, d, spikeID uuid.UUID) {
		exec(`INSERT INTO audit_events (id, occurred_at, actor, kind, ref_type, ref_id, payload)
			VALUES ($1, $2, 'orchestrator', 'dispatch.running', 'spike', $3, $4::jsonb)`,
			uuid.New(), at(sec), spikeID, `{"dispatch_id":"`+d.String()+`"}`)
	}
	runDispatch := map[uuid.UUID]uuid.UUID{}
	for i, id := range []uuid.UUID{running, ended, closed} {
		d := dispatch(id, "run-spike")
		runDispatch[id] = d
		audit(10+i, d, id)
		audit(20+i, d, id) // a retry marks it running again; the first counts
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, conn, applied, sixteen); err != nil {
		t.Fatalf("migrate 0016: %v", err)
	}

	for name, id := range map[string]uuid.UUID{"running": running, "ended": ended, "closed": closed} {
		var exe *string
		if err := conn.QueryRow(ctx, `SELECT executor FROM spikes WHERE id = $1`, id).Scan(&exe); err != nil || exe == nil || *exe != "agent" {
			t.Errorf("%s spike: executor = %v, %v", name, exe, err)
		}
		execs, err := ExecutionsFor(ctx, conn, "spike", id)
		if err != nil || len(execs) != 1 {
			t.Fatalf("%s spike: executions = %+v, %v", name, execs, err)
		}
		e := execs[0]
		if e.Round != 1 || e.Kind != "agent" || e.Actor != "spike-runner" || e.Model != "model-a" || e.Via != "agent" ||
			!e.Measured || !e.Inferred || e.StartHead != "" || e.ClaimID != nil ||
			e.DispatchID == nil || *e.DispatchID != runDispatch[id] || !e.StartedAt.Equal(at(map[string]int{"running": 10, "ended": 11, "closed": 12}[name])) {
			t.Errorf("%s spike: execution = %+v", name, e)
		}
	}
	for name, id := range map[string]uuid.UUID{"idea": idea, "closed without a run": closedUnrun} {
		var exe *string
		if err := conn.QueryRow(ctx, `SELECT executor FROM spikes WHERE id = $1`, id).Scan(&exe); err != nil || exe != nil {
			t.Errorf("%s: executor = %v, %v", name, exe, err)
		}
		if execs, _ := ExecutionsFor(ctx, conn, "spike", id); len(execs) != 0 {
			t.Errorf("%s: executions = %+v", name, execs)
		}
	}

	// The migration's checks hold on what it left, and refuse what stage 1
	// never wrote: a closed spike that ran without an executor, and an agent
	// spike with a deadline.
	if _, err := conn.Exec(ctx, `UPDATE spikes SET executor = NULL WHERE id = $1`, closed); err == nil ||
		!strings.Contains(err.Error(), "spikes_started") {
		t.Errorf("a closed spike that ran, without an executor: %v", err)
	}
	if _, err := conn.Exec(ctx, `UPDATE spikes SET deadline_at = now() WHERE id = $1`, running); err == nil ||
		!strings.Contains(err.Error(), "spikes_one_limit") {
		t.Errorf("an agent spike with a deadline: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO audit_events (id, actor, kind, ref_type, ref_id)
		VALUES (gen_random_uuid(), 'x', 'claim.expired', 'claim', gen_random_uuid())`); err != nil {
		t.Errorf("audit insert: %v", err)
	}
}
