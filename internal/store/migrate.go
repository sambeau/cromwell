// Package store owns Postgres access: migrations, repositories, and the
// transactional helpers that keep audit rows in the same transaction as the
// state changes they record (DESIGN-002 O-3). The CLI never imports this
// package except for `init`'s migration run (DESIGN-002 §3, FR-2.2).
package store

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entries {
		name := e.Name()
		num, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("migration %q: name must be NNNN_description.sql", name)
		}
		v, err := strconv.Atoi(num)
		if err != nil {
			return nil, fmt.Errorf("migration %q: bad version prefix: %w", name, err)
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: v, name: name, sql: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	// Versions must be unique and positive, but may have gaps. Two lines of
	// work can reserve numbers in parallel (SPEC-012 SD-10): a branch holding
	// 0008 must still load before 0007 has landed.
	for i, m := range ms {
		if m.version < 1 {
			return nil, fmt.Errorf("migration %q: version must be 1 or more", m.name)
		}
		if i > 0 && m.version == ms[i-1].version {
			return nil, fmt.Errorf("migrations %q and %q share version %d", ms[i-1].name, m.name, m.version)
		}
	}
	return ms, nil
}

// Migrate applies all unapplied migrations, each in its own transaction so a
// failure leaves the schema at the prior version (FR-1.2). Forward-only:
// there are no down migrations (DESIGN-001 §11).
func Migrate(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    integer PRIMARY KEY,
		name       text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	ms, err := loadMigrations()
	if err != nil {
		return err
	}
	return applyMigrations(ctx, conn, applied, ms)
}

// appliedVersions reads the set of migrations already applied.
func appliedVersions(ctx context.Context, conn *pgx.Conn) (map[int]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema versions: %w", err)
	}
	defer rows.Close()
	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// applyMigrations applies, in version order, every migration not already
// applied. It fills a gap as well as extending the top (SPEC-012 SD-10): a
// database that received 0008 before 0007 existed still gets 0007 later.
func applyMigrations(ctx context.Context, conn *pgx.Conn, applied map[int]bool, ms []migration) error {
	for _, m := range ms {
		if applied[m.version] {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s failed (nothing from it was applied): %w", m.name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// SchemaVersion returns the highest applied migration version, 0 if none.
func SchemaVersion(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (int, error) {
	var v int
	err := q.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	return v, err
}
