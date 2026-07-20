package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// testDatabaseURL returns the integration-test database, or skips. CI and
// local dev set CROMWELL_TEST_DATABASE_URL to a dedicated plain-Postgres
// instance (DEC-002); tests must never point this at a real project.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("CROMWELL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CROMWELL_TEST_DATABASE_URL not set; skipping integration test")
	}
	return url
}

// freshConn connects and resets the public schema so every test starts from
// an empty database.
func freshConn(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	return conn
}

func TestMigrateFromEmpty(t *testing.T) {
	conn := freshConn(t)
	ctx := context.Background()

	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	v, err := SchemaVersion(ctx, conn)
	if err != nil || v < 1 {
		t.Fatalf("schema version = %d, err %v; want >= 1", v, err)
	}

	// Idempotent: a second run applies nothing and succeeds.
	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}

	// The phase-1 tables exist (SPEC-001 NFR-3).
	for _, table := range []string{
		"initiatives", "features", "documents", "document_sections",
		"document_comments", "audit_events", "dispatches", "tool_calls",
		"checkpoints", "schema_migrations",
	} {
		var n int
		if err := conn.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`,
			table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s missing (n=%d, err=%v)", table, n, err)
		}
	}

	// documents_live_path enforces one live document per path.
	if _, err := conn.Exec(ctx, `
		INSERT INTO documents (id, type, state, owner_type, owner_id, path, title, content_hash)
		VALUES (gen_random_uuid(), 'spec', 'draft', 'project', NULL, 'docs/a.md', 'A', 'h1')`); err != nil {
		t.Fatalf("insert doc: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO documents (id, type, state, owner_type, owner_id, path, title, content_hash)
		VALUES (gen_random_uuid(), 'spec', 'draft', 'project', NULL, 'docs/a.md', 'A2', 'h2')`); err == nil {
		t.Error("second live document at same path should violate documents_live_path")
	}
}

// TestBrokenMigrationLeavesPriorVersion is FR-1.2's acceptance criterion: a
// failing migration leaves schema_migrations and the schema at the prior
// version.
func TestBrokenMigrationLeavesPriorVersion(t *testing.T) {
	conn := freshConn(t)
	ctx := context.Background()

	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	before, _ := SchemaVersion(ctx, conn)

	broken := []migration{{
		version: before + 1,
		name:    "9999_broken.sql",
		sql:     `CREATE TABLE will_rollback (id int); SELECT this_is_not_valid_sql;`,
	}}
	if err := applyMigrations(ctx, conn, before, broken); err == nil {
		t.Fatal("broken migration should fail")
	}

	after, _ := SchemaVersion(ctx, conn)
	if after != before {
		t.Errorf("schema version moved: %d -> %d", before, after)
	}
	var n int
	_ = conn.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_name='will_rollback'`).Scan(&n)
	if n != 0 {
		t.Error("partial migration state leaked: will_rollback exists")
	}
}
