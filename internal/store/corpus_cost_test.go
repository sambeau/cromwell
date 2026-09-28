package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/sizing"
)

// seedDispatch drives a dispatch through the real queued→running→succeeded
// flow so the ledger looks exactly as production writes it.
func seedDispatch(t *testing.T, s *Store, refType string, refID uuid.UUID, tokens int64, cost float64, key string) {
	t.Helper()
	ctx := context.Background()
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		d, err := EnqueueDispatch(ctx, tx, "implement-task", "implementer", "deepseek-v4-flash", refType, refID, key)
		if err != nil || d == nil {
			t.Fatalf("enqueue: %v", err)
		}
		if _, err := MarkDispatchRunning(ctx, tx, d.ID, nil); err != nil {
			return err
		}
		return MarkDispatchSucceeded(ctx, tx, d.ID, TokenUsage{Input: tokens, Output: 0}, cost, map[string]string{})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestActualTokensByOwnership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	init, fA, _ := seedTree(t, s)

	tasks, err := TasksForFeature(ctx, s.Pool, fA)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("tasks: %v %v", tasks, err)
	}
	// A task dispatch and a feature-level dispatch both attribute to feature A.
	seedDispatch(t, s, "task", tasks[0].ID, 1500, 0.002, "d:t0")
	seedDispatch(t, s, "feature", fA, 500, 0.001, "d:fA")

	got, err := ActualTokens(ctx, s.Pool, "feature", fA)
	if err != nil || got != 2000 {
		t.Fatalf("feature actual = %d, %v; want 2000 (task 1500 + feature 500)", got, err)
	}
	// The initiative sees the same 2000 transitively.
	initActual, err := ActualTokens(ctx, s.Pool, "initiative", init)
	if err != nil || initActual != 2000 {
		t.Fatalf("initiative actual = %d, %v; want 2000", initActual, err)
	}
}

func TestCorpusRetrievalByFullText(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// A completed, estimated feature whose description shares terms with the
	// query, plus an unrelated one that should not match.
	var matchID uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		in, err := CreateInitiative(ctx, tx, nil, "auth", "Auth", "", "sam")
		if err != nil {
			return err
		}
		match, err := CreateFeature(ctx, tx, in.ID, "oauth", "OAuth login",
			"Implement OAuth token exchange and refresh for the login flow", "sam")
		if err != nil {
			return err
		}
		matchID = match.ID
		other, err := CreateFeature(ctx, tx, in.ID, "billing", "Billing export",
			"Export monthly invoices to CSV for finance", "sam")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE features SET state='done' WHERE id IN ($1,$2)`, match.ID, other.ID); err != nil {
			return err
		}
		if _, err := RecordEstimate(ctx, tx, "feature", match.ID, 4000, sizing.TierConsidered, "", nil, "sam"); err != nil {
			return err
		}
		_, err = RecordEstimate(ctx, tx, "feature", other.ID, 2000, sizing.TierRough, "", nil, "sam")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Give the match a real actual so the corpus row is a full reference point.
	seedDispatch(t, s, "feature", matchID, 4200, 0.003, "d:match")

	rows, err := RetrieveCorpus(ctx, s.Pool, "OAuth token refresh for login", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].RefID != matchID {
		t.Fatalf("corpus top hit = %+v, want the OAuth feature %s", rows, matchID)
	}
	if rows[0].EstimateTokens != 4000 || rows[0].EstimateTier != sizing.TierConsidered {
		t.Errorf("corpus estimate = %d/%s, want 4000/considered", rows[0].EstimateTokens, rows[0].EstimateTier)
	}
	if rows[0].ActualTokens != 4200 {
		t.Errorf("corpus actual = %d, want 4200", rows[0].ActualTokens)
	}
}

func TestCostRollupsExtended(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	init, fA, fB := seedTree(t, s)

	tasks, _ := TasksForFeature(ctx, s.Pool, fA)
	seedDispatch(t, s, "task", tasks[0].ID, 1000, 0.0040, "c:t0")
	seedDispatch(t, s, "feature", fB, 1000, 0.0010, "c:fB")

	// Initiative cost is transitive over both features.
	ic, err := InitiativeCost(ctx, s.Pool, init)
	if err != nil || !approx(ic, 0.0050) {
		t.Fatalf("initiative cost = %v, %v; want 0.0050", ic, err)
	}
	fc, err := FeatureCost(ctx, s.Pool, fA)
	if err != nil || !approx(fc, 0.0040) {
		t.Fatalf("feature A cost = %v, %v; want 0.0040", fc, err)
	}

	// A milestone over both features costs the union.
	var ms uuid.UUID
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		m, err := CreateMilestone(ctx, tx, "project", nil, "v1", "", nil, "sam")
		if err != nil {
			return err
		}
		ms = m.ID
		if err := AddMember(ctx, tx, m.ID, "feature", fA, "sam"); err != nil {
			return err
		}
		return AddMember(ctx, tx, m.ID, "feature", fB, "sam")
	})
	if err != nil {
		t.Fatal(err)
	}
	leaves, _ := ResolveMembers(ctx, s.Pool, ms)
	mc, err := MilestoneCost(ctx, s.Pool, leaves)
	if err != nil || !approx(mc, 0.0050) {
		t.Fatalf("milestone cost = %v, %v; want 0.0050", mc, err)
	}

	// Per-month rollup buckets the spend into one month.
	months, err := CostByMonth(ctx, s.Pool)
	if err != nil || len(months) != 1 || !approx(months[0].CostUSD, 0.0050) {
		t.Fatalf("months = %+v, %v; want one month at 0.0050", months, err)
	}
}

func approx(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
