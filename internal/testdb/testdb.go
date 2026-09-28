// Package testdb hands each test package its own database so `go test
// ./...`'s package-level parallelism cannot cross schemas. Used only from
// tests; skips when no test database is configured.
package testdb

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"

	"subutai/internal/compat"
)

// URL returns a connection string to a dedicated database named dbname,
// creating it on the server behind SUBUTAI_TEST_DATABASE_URL if needed.
// Cromwell's CROMWELL_TEST_DATABASE_URL is read when the new name is unset
// (SPEC-013 §3.3). compat(M7)
func URL(t *testing.T, dbname string) string {
	t.Helper()
	base := compat.Getenv("SUBUTAI_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("neither SUBUTAI_TEST_DATABASE_URL nor CROMWELL_TEST_DATABASE_URL is set; skipping integration test")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("bad SUBUTAI_TEST_DATABASE_URL: %v", err)
	}

	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()
	var exists bool
	if err := admin.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, dbname).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{dbname}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	u.Path = "/" + dbname
	return u.String()
}
