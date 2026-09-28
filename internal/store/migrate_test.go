package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"subutai/internal/testdb"
)

// testDatabaseURL returns this package's dedicated integration-test
// database, or skips. CI and local dev set SUBUTAI_TEST_DATABASE_URL to a
// dedicated plain-Postgres instance (DEC-002); tests must never point this
// at a real project.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	return testdb.URL(t, "subutai_store_test")
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
		"checkpoints", "schema_migrations", "transcript_entries",
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
	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, conn, applied, broken); err == nil {
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

// TestMigrationGapIsFilledLater is SPEC-012 SD-10: a database that received a
// higher version first still gets the lower one when it arrives, and a gap in
// the numbering loads.
func TestMigrationGapIsFilledLater(t *testing.T) {
	conn := freshConn(t)
	ctx := context.Background()
	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	top, _ := SchemaVersion(ctx, conn)

	high := migration{version: top + 2, name: "high.sql", sql: `CREATE TABLE gap_high (id int)`}
	low := migration{version: top + 1, name: "low.sql", sql: `CREATE TABLE gap_low (id int)`}
	applied, _ := appliedVersions(ctx, conn)
	if err := applyMigrations(ctx, conn, applied, []migration{high}); err != nil {
		t.Fatalf("apply high: %v", err)
	}
	applied, _ = appliedVersions(ctx, conn)
	if err := applyMigrations(ctx, conn, applied, []migration{low, high}); err != nil {
		t.Fatalf("apply low after high: %v", err)
	}
	for _, table := range []string{"gap_low", "gap_high"} {
		var n int
		_ = conn.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.tables WHERE table_name=$1`, table).Scan(&n)
		if n != 1 {
			t.Errorf("table %s missing", table)
		}
	}
}
