package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestMigration0014BackfillsAgentExecutions is SPEC-020 FR-1.3: on a database
// written as it was before 0014, one inferred agent execution appears for each
// implement-task dispatch with a dispatch.running row, started at the first
// such row, in the round the task's request_changes transitions imply.
func TestMigration0014BackfillsAgentExecutions(t *testing.T) {
	conn := freshConn(t)
	ctx := context.Background()
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var before, fourteen []migration
	for _, m := range ms {
		switch {
		case m.version < 14:
			before = append(before, m)
		case m.version == 14:
			fourteen = append(fourteen, m)
		}
	}
	if len(fourteen) != 1 {
		t.Fatalf("migration 14 not found")
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE schema_migrations (
		version integer PRIMARY KEY, name text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, conn, map[int]bool{}, before); err != nil {
		t.Fatalf("migrate to 0013: %v", err)
	}

	t0 := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	in, feat, task := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO initiatives (id, slug, name) VALUES ($1, 'pf', 'Platform')`, in)
	exec(`INSERT INTO features (id, initiative_id, slug, name) VALUES ($1, $2, 'alpha', 'Alpha')`, feat, in)
	exec(`INSERT INTO tasks (id, feature_id, position, title, public_id) VALUES ($1, $2, 0, 'A task', 'T-1')`, task, feat)
	audit := func(sec int, kind, refType string, ref uuid.UUID, payload string) {
		exec(`INSERT INTO audit_events (id, occurred_at, actor, kind, ref_type, ref_id, payload)
			VALUES ($1, $2, 'orchestrator', $3, $4, $5, $6::jsonb)`, uuid.New(), at(sec), kind, refType, ref, payload)
	}
	dispatch := func(purpose, role, model, state string) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key)
			VALUES ($1, $2, $3, $4, $5, 'task', $6, $7)`, id, state, purpose, role, model, task, id.String())
		return id
	}

	// Round 1: started, then re-marked running by a retry (the first counts).
	first := dispatch("implement-task", "implementer", "model-a", "succeeded")
	audit(10, "dispatch.running", "task", task, `{"dispatch_id":"`+first.String()+`"}`)
	audit(20, "dispatch.running", "task", task, `{"dispatch_id":"`+first.String()+`"}`)
	// Sent back once, then round 2 by another dispatch.
	audit(30, "task.transition", "task", task, `{"event":"implemented"}`)
	audit(31, "task.transition", "task", task, `{"event":"request_changes"}`)
	second := dispatch("implement-task", "implementer", "model-b", "running")
	audit(40, "dispatch.running", "task", task, `{"dispatch_id":"`+second.String()+`"}`)
	// A queued dispatch never ran; a review dispatch is not an implementation.
	dispatch("implement-task", "implementer", "model-a", "queued")
	review := dispatch("review-code", "code-reviewer", "model-c", "succeeded")
	audit(50, "dispatch.running", "task", task, `{"dispatch_id":"`+review.String()+`"}`)

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, conn, applied, fourteen); err != nil {
		t.Fatalf("migrate 0014: %v", err)
	}

	execs, err := ExecutionsFor(ctx, conn, "task", task)
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) != 2 {
		t.Fatalf("backfilled %d executions, want 2: %+v", len(execs), execs)
	}
	for i, want := range []struct {
		dispatch uuid.UUID
		model    string
		round    int
		sec      int
	}{{first, "model-a", 1, 10}, {second, "model-b", 2, 40}} {
		e := execs[i]
		if e.DispatchID == nil || *e.DispatchID != want.dispatch || e.Model != want.model || e.Round != want.round ||
			e.Kind != "agent" || e.Actor != "implementer" || e.Via != "agent" || !e.Measured || !e.Inferred ||
			e.StartHead != "" || e.ClaimID != nil || !e.StartedAt.Equal(at(want.sec)) {
			t.Errorf("execution %d = %+v, want dispatch %s, round %d", i, e, want.dispatch, want.round)
		}
	}
	// The claim machinery exists, empty, and the audit enum took 'claim'.
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM work_claims`).Scan(&n); err != nil || n != 0 {
		t.Errorf("work_claims: %d %v", n, err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO audit_events (id, actor, kind, ref_type, ref_id)
		VALUES (gen_random_uuid(), 'x', 'claim.test', 'claim', gen_random_uuid())`); err != nil {
		t.Errorf("claim ref_type: %v", err)
	}
}
