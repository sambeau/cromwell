package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/sizing"
)

// Unmeasured actuals (SPEC-020 SD-13, FR-7), against real Postgres.

// chatExecution records a chat claim's execution on a task (FR-1.2).
func chatExecution(t *testing.T, s *Store, taskID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		c, err := CreateClaim(ctx, tx, "task", taskID, nil, "chat", "claude", "mcp", "")
		if err != nil {
			return err
		}
		_, err = RecordClaimExecution(ctx, tx, "task", taskID, c, "abc123")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// writeSpec registers a live document of a type on a feature and records one
// writing act by the given kind.
func writeSpec(t *testing.T, s *Store, featureID uuid.UUID, docType, act, kind, path string) {
	t.Helper()
	ctx := context.Background()
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		d, err := RegisterDocument(ctx, tx, docType, "feature", &featureID, path, "Doc", "hash", nil, "sam")
		if err != nil {
			return err
		}
		return RecordWriter(ctx, tx, Writer{DocumentID: d.ID, Act: act, Kind: kind, Actor: "x"})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustUnmeasured(t *testing.T, s *Store, refType string, id uuid.UUID, want bool) {
	t.Helper()
	_, got, err := ActualTokens(context.Background(), s.Pool, refType, id)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("%s unmeasured = %v, want %v", refType, got, want)
	}
}

func TestChatExecutionMakesTaskFeatureInitiativeUnmeasured(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	init, fA, fB := seedTree(t, s)
	tasks, _ := TasksForFeature(ctx, s.Pool, fA)

	seedDispatch(t, s, "task", tasks[0].ID, 1500, 0.002, "u:t0")
	mustUnmeasured(t, s, "task", tasks[0].ID, false)
	mustUnmeasured(t, s, "feature", fA, false)
	mustUnmeasured(t, s, "initiative", init, false)

	chatExecution(t, s, tasks[1].ID)
	mustUnmeasured(t, s, "task", tasks[1].ID, true)
	mustUnmeasured(t, s, "task", tasks[0].ID, false)
	mustUnmeasured(t, s, "feature", fA, true)
	mustUnmeasured(t, s, "feature", fB, false)
	mustUnmeasured(t, s, "initiative", init, true)

	// The measured part is still the dispatch tokens.
	n, un, err := ActualTokens(ctx, s.Pool, "feature", fA)
	if err != nil || n != 1500 || !un {
		t.Fatalf("feature A = %d, %v, %v; want 1500, true", n, un, err)
	}
}

func TestSpecWriterMakesFeatureUnmeasured(t *testing.T) {
	s := testStore(t)
	_, fA, fB := seedTree(t, s)

	writeSpec(t, s, fA, "spec", ActWrote, WriterChat, "docs/a.md")
	writeSpec(t, s, fB, "spec", ActWrote, WriterAgent, "docs/b.md")
	mustUnmeasured(t, s, "feature", fA, true)
	mustUnmeasured(t, s, "feature", fB, false)

	// Only the latest act counts: an agent revising a chat-written spec makes
	// the feature measured again.
	ctx := context.Background()
	var docID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM documents WHERE owner_id=$1`, fA).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		return RecordWriter(ctx, tx, Writer{DocumentID: docID, Act: ActRevised, Kind: WriterAgent, Actor: "author"})
	}); err != nil {
		t.Fatal(err)
	}
	mustUnmeasured(t, s, "feature", fA, false)

	// A superseded document doesn't count, and a started act isn't a writing act.
	if _, err := s.Pool.Exec(ctx, `UPDATE documents SET state='superseded' WHERE owner_id=$1`, fB); err != nil {
		t.Fatal(err)
	}
	writeSpec(t, s, fB, "dev_plan", ActStarted, WriterPerson, "docs/b-plan.md")
	mustUnmeasured(t, s, "feature", fB, false)
	writeSpec(t, s, fB, "dev_plan", ActAdded, WriterPerson, "docs/b-plan2.md")
	mustUnmeasured(t, s, "feature", fB, true)
}

func TestBugReportIsTheBugsSpec(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, fA, _ := seedTree(t, s)
	if _, err := s.Pool.Exec(ctx, `UPDATE features SET kind='bug', public_id='BUG-9' WHERE id=$1`, fA); err != nil {
		t.Fatal(err)
	}
	// A "spec" on a bug isn't its spec; its bug report is.
	writeSpec(t, s, fA, "spec", ActWrote, WriterChat, "docs/x.md")
	mustUnmeasured(t, s, "feature", fA, false)
	writeSpec(t, s, fA, "bug_report", ActWrote, WriterPerson, "docs/bug.md")
	mustUnmeasured(t, s, "feature", fA, true)
}

// seedDoneTasks creates n done, estimated tasks whose descriptions share
// terms with a query, returning their ids.
func seedDoneTasks(t *testing.T, s *Store, featureID uuid.UUID, prefix string, n int, desc string) []uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var ids []uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		for i := 0; i < n; i++ {
			tk, err := CreateTask(ctx, tx, featureID, 100+i, fmt.Sprintf("%s%d", prefix, i), "Task "+prefix, desc, "sam")
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE tasks SET state='done' WHERE id=$1`, tk.ID); err != nil {
				return err
			}
			if _, err := RecordEstimate(ctx, tx, "task", tk.ID, 500, sizing.TierRough, "", nil, "sam"); err != nil {
				return err
			}
			ids = append(ids, tk.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestCorpusAndCalibrationFilterBeforeLimit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, fA, _ := seedTree(t, s)

	measured := seedDoneTasks(t, s, fA, "m", 5, "oauth token refresh")
	// Unmeasured neighbours match more strongly and are estimated later, so
	// they would fill the limit if the filter ran after it.
	unmeasured := seedDoneTasks(t, s, fA, "u", 4, "oauth token refresh login flow exchange")
	for _, id := range unmeasured {
		chatExecution(t, s, id)
	}

	rows, err := RetrieveCorpus(ctx, s.Pool, "oauth token refresh login flow exchange", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("corpus returned %d rows, want 5 measured", len(rows))
	}
	want := map[uuid.UUID]bool{}
	for _, id := range measured {
		want[id] = true
	}
	for _, r := range rows {
		if !want[r.RefID] {
			t.Errorf("corpus returned unmeasured %s", r.Name)
		}
	}

	cal, err := RecentCalibration(ctx, s.Pool, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(cal) != 5 {
		t.Fatalf("calibration returned %d rows, want 5 measured", len(cal))
	}
	for _, r := range cal {
		if !want[r.RefID] {
			t.Errorf("calibration returned unmeasured %s", r.Name)
		}
	}

	// An unmeasured feature (spec written in chat) is left out too.
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE features SET state='done' WHERE id=$1`, fA); err != nil {
			return err
		}
		_, err := RecordEstimate(ctx, tx, "feature", fA, 900, sizing.TierRough, "", nil, "sam")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	writeSpec(t, s, fA, "spec", ActWrote, WriterChat, "docs/fa.md")
	cal, _ = RecentCalibration(ctx, s.Pool, 50)
	for _, r := range cal {
		if r.RefType == "feature" {
			t.Errorf("calibration returned the unmeasured feature %s", r.Name)
		}
	}
}

func TestPurposeTokenSamplesSkipsUnmeasuredRound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, fA, _ := seedTree(t, s)
	tasks, _ := TasksForFeature(ctx, s.Pool, fA)

	// Task 0: a clean agent round. Task 1: an agent round that also holds a
	// chat execution. A third task's dispatch has no execution row.
	seedDispatch(t, s, "task", tasks[0].ID, 1000, 0.001, "p:t0")
	seedDispatch(t, s, "task", tasks[1].ID, 2000, 0.001, "p:t1")
	var loose uuid.UUID
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		tk, err := CreateTask(ctx, tx, fA, 9, "T9", "loose", "", "sam")
		if err != nil {
			return err
		}
		loose = tk.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	seedDispatch(t, s, "task", loose, 4000, 0.001, "p:loose")

	for key, taskID := range map[string]uuid.UUID{"p:t0": tasks[0].ID, "p:t1": tasks[1].ID} {
		var did uuid.UUID
		if err := s.Pool.QueryRow(ctx, `SELECT id FROM dispatches WHERE idempotency_key=$1`, key).Scan(&did); err != nil {
			t.Fatal(err)
		}
		if err := s.WithTx(ctx, func(tx pgx.Tx) error {
			return RecordAgentExecution(ctx, tx, taskID, did, "implementer", "m", "head")
		}); err != nil {
			t.Fatal(err)
		}
	}
	chatExecution(t, s, tasks[1].ID) // the same round as its agent run

	got, err := PurposeTokenSamples(ctx, s.Pool, "implement-task", 10)
	if err != nil {
		t.Fatal(err)
	}
	has := map[int64]bool{}
	for _, n := range got {
		has[n] = true
	}
	if len(got) != 2 || !has[1000] || !has[4000] || has[2000] {
		t.Fatalf("samples = %v, want 1000 and 4000 only (the unmeasured round's 2000 left out)", got)
	}
}
