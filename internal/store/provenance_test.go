package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestMigration0011ReadsBackTheTrail is SPEC-017 FR-2.8's first pass: on a
// database written as it was before 0011, the migration records what the
// audit trail pins down, marks it inferred, and leaves the rest for the
// server's pass, which knows the configured actor names.
func TestMigration0011ReadsBackTheTrail(t *testing.T) {
	conn := freshConn(t)
	ctx := context.Background()
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var before, eleven []migration
	for _, m := range ms {
		switch {
		case m.version < 11:
			before = append(before, m)
		case m.version == 11:
			eleven = append(eleven, m)
		}
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE schema_migrations (
		version integer PRIMARY KEY, name text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, conn, map[int]bool{}, before); err != nil {
		t.Fatalf("migrate to 0010: %v", err)
	}

	t0 := time.Now().Add(-time.Hour).UTC()
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	in, feat := uuid.New(), uuid.New()
	exec(`INSERT INTO initiatives (id, slug, name) VALUES ($1, 'pf', 'Platform')`, in)
	exec(`INSERT INTO features (id, initiative_id, slug, name) VALUES ($1, $2, 'alpha', 'Alpha')`, feat, in)
	doc := func(docType, path string) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO documents (id, type, state, owner_type, owner_id, path, title, content_hash)
			VALUES ($1, $2, 'draft', 'feature', $3, $4, $4, 'h')`, id, docType, feat, path)
		return id
	}
	audit := func(sec int, actor, kind string, ref uuid.UUID, payload string) {
		exec(`INSERT INTO audit_events (id, occurred_at, actor, kind, ref_type, ref_id, payload)
			VALUES ($1, $2, $3, $4, 'document', $5, $6::jsonb)`, uuid.New(), at(sec), actor, kind, ref, payload)
	}
	dispatch := func(purpose, role, model, refType string, ref uuid.UUID, finished int) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key, finished_at)
			VALUES ($1, 'succeeded', $2, $3, $4, $5, $6, $7, $8)`, id, purpose, role, model, refType, ref, id.String(), at(finished))
		return id
	}

	// An authored spec, approved by its reviewer.
	authored := doc("spec", "docs/a.md")
	writeRun := dispatch("write-spec", "spec-author", "model-w", "feature", feat, 0)
	audit(1, "spec-author", "document.registered", authored, `{}`)
	reviewRun := dispatch("review-spec", "spec-reviewer", "model-r", "document", authored, 2)
	audit(3, "spec-reviewer", "document.transition", authored, `{"event":"approve"}`)

	// A spec the chat agent added, approved by a person through it.
	chatDoc := doc("spec", "docs/b.md")
	audit(4, "chat-agent", "document.registered", chatDoc, `{}`)
	audit(5, "chat-agent", "document.human_verdict", chatDoc, `{"verdict":"approve","via":"mcp","quote":"Yes."}`)
	audit(6, "chat-agent", "document.transition", chatDoc, `{"event":"approve"}`)

	// A held approval, then released by a person in the web UI.
	held := doc("spec", "docs/c.md")
	heldRun := dispatch("review-spec", "spec-reviewer", "model-r", "document", held, 7)
	audit(8, "spec-reviewer", "document.held", held, `{"dispatch_id":"`+heldRun.String()+`"}`)
	audit(9, "sam", "document.hold_released", held, `{"via":"ui","dispatch_id":"`+heldRun.String()+`"}`)
	audit(10, "spec-reviewer", "document.transition", held, `{"event":"approve"}`)

	// A note adopted as already approved.
	note := doc("note", "docs/d.md")
	audit(11, "sam", "document.human_verdict", note, `{"verdict":"approve","via":"ui","already_approved":"adopt"}`)

	// A person's approval with nothing before it: left for the server.
	bare := doc("spec", "docs/e.md")
	audit(12, "sam", "document.transition", bare, `{"event":"approve"}`)

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, conn, applied, eleven); err != nil {
		t.Fatalf("migrate 0011: %v", err)
	}
	q := conn

	ws, _ := Writers(ctx, q, authored)
	if len(ws) != 1 || ws[0].Kind != WriterAgent || ws[0].Model != "model-w" || ws[0].DispatchID == nil ||
		*ws[0].DispatchID != writeRun || !ws[0].Inferred {
		t.Errorf("authored spec writers = %+v", ws)
	}
	if ws, _ := Writers(ctx, q, chatDoc); len(ws) != 0 {
		t.Errorf("the chat agent's writing is the server's to infer; the migration wrote %+v", ws)
	}
	vs, _ := Verdicts(ctx, q, authored)
	if len(vs) != 1 || vs[0].Kind != GiverAgent || vs[0].Model != "model-r" || *vs[0].DispatchID != reviewRun {
		t.Errorf("agent verdict = %+v", vs)
	}
	vs, _ = Verdicts(ctx, q, chatDoc)
	if len(vs) != 1 || vs[0].Kind != GiverPerson || vs[0].Via != "mcp" || vs[0].Quote != "Yes." {
		t.Errorf("relayed verdict = %+v", vs)
	}
	vs, _ = Verdicts(ctx, q, held)
	if len(vs) != 2 || !vs[0].Held || vs[1].Held || vs[1].ReleasedBy != "sam" || vs[1].ReleasedVia != "ui" {
		t.Errorf("held and released = %+v", vs)
	}
	vs, _ = Verdicts(ctx, q, note)
	if len(vs) != 1 || vs[0].Kind != GiverPerson || vs[0].Via != "ui" || vs[0].Actor != "sam" {
		t.Errorf("approved by adoption = %+v", vs)
	}
	if vs, _ := Verdicts(ctx, q, bare); len(vs) != 0 {
		t.Errorf("a bare person's approval is the server's to infer; the migration wrote %+v", vs)
	}

	// A relayed verdict must carry the person's words.
	if _, err := conn.Exec(ctx, `INSERT INTO document_verdicts (id, document_id, verdict, giver_kind, actor, via)
		VALUES ($1, $2, 'approve', 'person', 'chat-agent', 'mcp')`, uuid.New(), bare); err == nil {
		t.Error("the database must refuse a relayed verdict with no quote")
	}
}
