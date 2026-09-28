package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/ident"
)

// migrateBelow applies every migration below version v, as a database made
// before that migration existed would have.
func migrateBelow(t *testing.T, conn *pgx.Conn, v int) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY, name text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var below []migration
	for _, m := range ms {
		if m.version < v {
			below = append(below, m)
		}
	}
	if err := applyMigrations(ctx, conn, map[int]bool{}, below); err != nil {
		t.Fatal(err)
	}
}

// TestBackfillNumbersExistingRowsInCreationOrder is SPEC-015 FR-2: rows made
// before 0010 are numbered by when they were created, not by insertion or id
// order, tasks per feature, and the next create continues each sequence.
func TestBackfillNumbersExistingRowsInCreationOrder(t *testing.T) {
	conn := freshConn(t)
	ctx := context.Background()
	migrateBelow(t, conn, 10)

	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	// Inserted out of creation order on purpose.
	late, early := uuid.New(), uuid.New()
	exec(`INSERT INTO initiatives (id, slug, name, created_at) VALUES ($1, 'late', 'Late', $2)`, late, at(10))
	exec(`INSERT INTO initiatives (id, slug, name, created_at) VALUES ($1, 'early', 'Early', $2)`, early, at(1))
	fB, fA := uuid.New(), uuid.New()
	exec(`INSERT INTO features (id, initiative_id, slug, name, created_at) VALUES ($1, $2, 'b', 'B', $3)`, fB, early, at(20))
	exec(`INSERT INTO features (id, initiative_id, slug, name, created_at) VALUES ($1, $2, 'a', 'A', $3)`, fA, early, at(5))
	for i, feat := range []uuid.UUID{fA, fA, fA, fB} {
		exec(`INSERT INTO tasks (id, feature_id, position, title, created_at) VALUES ($1, $2, $3, 't', $4)`,
			uuid.New(), feat, i, at(30-i))
	}
	exec(`INSERT INTO milestones (id, name, created_at) VALUES ($1, 'M', $2)`, uuid.New(), at(3))
	rmFirst, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	rmSecond, _ := uuid.NewV7()
	exec(`INSERT INTO roadmaps (id, name) VALUES ($1, 'second')`, rmSecond)
	exec(`INSERT INTO roadmaps (id, name) VALUES ($1, 'first')`, rmFirst)

	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pid := func(table string, id uuid.UUID) string {
		var p string
		if err := conn.QueryRow(ctx, `SELECT public_id FROM `+table+` WHERE id = $1`, id).Scan(&p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, c := range []struct {
		table string
		id    uuid.UUID
		want  string
	}{
		{"initiatives", early, "INIT-001"},
		{"initiatives", late, "INIT-002"},
		{"features", fA, "FEAT-001"},
		{"features", fB, "FEAT-002"},
		{"roadmaps", rmFirst, "RM-001"},
		{"roadmaps", rmSecond, "RM-002"},
	} {
		if got := pid(c.table, c.id); got != c.want {
			t.Errorf("%s %s: got %s, want %s", c.table, c.id, got, c.want)
		}
	}

	// Tasks: per feature, by created_at — the last inserted of A's is oldest.
	rows, err := conn.Query(ctx, `SELECT public_id FROM tasks ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	var tasks []string
	for rows.Next() {
		var p string
		_ = rows.Scan(&p)
		tasks = append(tasks, p)
	}
	rows.Close()
	want := []string{"FEAT-002-T01", "FEAT-001-T01", "FEAT-001-T02", "FEAT-001-T03"}
	if strings.Join(tasks, ",") != strings.Join(want, ",") {
		t.Errorf("tasks = %v, want %v", tasks, want)
	}
	var legacy bool
	_ = conn.QueryRow(ctx, `SELECT bool_and(legacy_doc_paths) FROM features`).Scan(&legacy)
	if !legacy {
		t.Error("features that existed before 0010 keep SPEC-009's paths (SD-16)")
	}

	// The next of each continues the sequence.
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	in, err := CreateInitiative(ctx, tx, nil, "third", "Third", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if in.PublicID != "INIT-003" {
		t.Errorf("next initiative = %s, want INIT-003", in.PublicID)
	}
	f, err := CreateFeature(ctx, tx, early, "c", "C", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if f.PublicID != "FEAT-003" {
		t.Errorf("next feature = %s", f.PublicID)
	}
	task, err := CreateTask(ctx, tx, fA, 9, "T9", "next", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if task.PublicID != "FEAT-001-T04" {
		t.Errorf("next task of FEAT-001 = %s, want FEAT-001-T04", task.PublicID)
	}
	m, err := CreateMilestone(ctx, tx, "project", nil, "Next", "", nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if m.PublicID != "MS-002" {
		t.Errorf("next milestone = %s", m.PublicID)
	}
	rm, err := CreateRoadmap(ctx, tx, "project", nil, "Next", "test")
	if err != nil {
		t.Fatal(err)
	}
	if rm.PublicID != "RM-003" {
		t.Errorf("next roadmap = %s", rm.PublicID)
	}
}

// TestChecklistBackfillRunsOnlyWithTheTable is FR-2.3: 0010's checklist block
// does nothing without M5's table, and numbers checklists when it's there.
func TestChecklistBackfillRunsOnlyWithTheTable(t *testing.T) {
	ctx := context.Background()

	// Without the table: covered by every other test's migration, but check
	// the column really isn't conjured from nowhere.
	conn := freshConn(t)
	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name = 'checklists'`).Scan(&n)
	if n != 0 {
		t.Skip("a checklists table exists on this branch; the with-table case below covers it")
	}

	// With a stand-in for M5's table, created before 0010 runs.
	conn = freshConn(t)
	migrateBelow(t, conn, 10)
	if _, err := conn.Exec(ctx, `CREATE TABLE checklists (id uuid PRIMARY KEY, name text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	first, _ := uuid.NewV7()
	time.Sleep(2 * time.Millisecond)
	second, _ := uuid.NewV7()
	for _, id := range []uuid.UUID{second, first} {
		if _, err := conn.Exec(ctx, `INSERT INTO checklists (id, name) VALUES ($1, 'c')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate with checklists: %v", err)
	}
	var p1, p2, p3 string
	_ = conn.QueryRow(ctx, `SELECT public_id FROM checklists WHERE id = $1`, first).Scan(&p1)
	_ = conn.QueryRow(ctx, `SELECT public_id FROM checklists WHERE id = $1`, second).Scan(&p2)
	if p1 != "CL-001" || p2 != "CL-002" {
		t.Errorf("checklists numbered %s, %s; want CL-001, CL-002", p1, p2)
	}
	if err := conn.QueryRow(ctx, `INSERT INTO checklists (id, name) VALUES ($1, 'new') RETURNING public_id`,
		uuid.New()).Scan(&p3); err != nil || p3 != "CL-003" {
		t.Errorf("a new checklist got %q (%v); want CL-003 from the column default", p3, err)
	}
}

// TestRegistryMatchesSequences is SD-2: every prefix in internal/ident has a
// sequence, and there is no sequence the registry doesn't know.
func TestRegistryMatchesSequences(t *testing.T) {
	s := testStore(t)
	got, err := IdentSequences(context.Background(), s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, k := range ident.Kinds {
		want = append(want, k.Prefix)
	}
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sequences %v, registry %v", got, want)
	}
}

// TestMintingFormatAndGaps is FR-1's acceptance: zero-padded to three, wider
// past 999, and a rolled-back create costs a number and nothing else.
func TestMintingFormatAndGaps(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	create := func(slug string) (*Initiative, error) {
		var in *Initiative
		err := s.WithTx(ctx, func(tx pgx.Tx) error {
			var e error
			in, e = CreateInitiative(ctx, tx, nil, slug, slug, "", "test")
			return e
		})
		return in, err
	}
	a, err := create("a")
	if err != nil || a.PublicID != "INIT-001" {
		t.Fatalf("first = %v, %v", a, err)
	}
	// Rolled back: the number is spent.
	_ = s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := CreateInitiative(ctx, tx, nil, "doomed", "Doomed", "", "test"); err != nil {
			return err
		}
		return fmt.Errorf("roll back")
	})
	b, err := create("b")
	if err != nil || b.PublicID != "INIT-003" {
		t.Fatalf("after a rollback = %v, %v; want INIT-003", b, err)
	}
	if _, err := s.Pool.Exec(ctx, `SELECT setval('ident_init_seq', 999)`); err != nil {
		t.Fatal(err)
	}
	c, err := create("c")
	if err != nil || c.PublicID != "INIT-1000" {
		t.Fatalf("thousandth = %v, %v; want INIT-1000", c, err)
	}
	got, err := InitiativeByPublicID(ctx, s.Pool, "init-1000")
	if err != nil || got.ID != c.ID {
		t.Errorf("lookup by ID = %v, %v", got, err)
	}
}

// TestAdvanceNeverGoesBackwards is FR-5.5's primitive.
func TestAdvanceNeverGoesBackwards(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mint := func() string {
		var id string
		_ = s.WithTx(ctx, func(tx pgx.Tx) error {
			var e error
			id, e = MintDecisionID(ctx, tx)
			return e
		})
		return id
	}
	advance := func(n int64) {
		if err := s.WithTx(ctx, func(tx pgx.Tx) error { return AdvanceIdent(ctx, tx, "DEC", n) }); err != nil {
			t.Fatal(err)
		}
	}
	advance(7)
	if got := mint(); got != "DEC-008" {
		t.Errorf("after advancing to 7: %s", got)
	}
	advance(3)
	if got := mint(); got != "DEC-009" {
		t.Errorf("advancing backwards moved the sequence: %s", got)
	}
}
