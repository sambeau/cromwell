package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/lifecycle"
	"cromwell/internal/sizing"
)

// seedTree builds an initiative with two features (and one feature's tasks) so
// the sizing and milestone tests have a real tree. Returns the initiative and
// both feature ids.
func seedTree(t *testing.T, s *Store) (initiative, fA, fB uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		in, err := CreateInitiative(ctx, tx, nil, "auth", "Auth", "", "sam")
		if err != nil {
			return err
		}
		a, err := CreateFeature(ctx, tx, in.ID, "login", "Login", "", "sam")
		if err != nil {
			return err
		}
		b, err := CreateFeature(ctx, tx, in.ID, "logout", "Logout", "", "sam")
		if err != nil {
			return err
		}
		// feature A decomposes into two tasks.
		t1, err := CreateTask(ctx, tx, a.ID, 0, "T1", "form", "", "sam")
		if err != nil {
			return err
		}
		t2, err := CreateTask(ctx, tx, a.ID, 1, "T2", "session", "", "sam")
		if err != nil {
			return err
		}
		// A's tasks estimated (one considered, one rough → A rolls up rough).
		if _, err := RecordEstimate(ctx, tx, "task", t1.ID, 1200, sizing.TierConsidered, "", nil, "sam"); err != nil {
			return err
		}
		if _, err := RecordEstimate(ctx, tx, "task", t2.ID, 800, sizing.TierRough, "", nil, "sam"); err != nil {
			return err
		}
		// feature B estimated as a unit (considered).
		if _, err := RecordEstimate(ctx, tx, "feature", b.ID, 5000, sizing.TierConsidered, "corpus: FEAT-x", nil, "sam"); err != nil {
			return err
		}
		initiative, fA, fB = in.ID, a.ID, b.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestEstimatesLatestIsCurrent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, fA, _ := seedTree(t, s)

	// Re-estimate feature A as a unit twice; latest is current, both kept.
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := RecordEstimate(ctx, tx, "feature", fA, 9000, sizing.TierRough, "first", nil, "sam"); err != nil {
			return err
		}
		_, err := RecordEstimate(ctx, tx, "feature", fA, 7000, sizing.TierConsidered, "second", nil, "sam")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := CurrentEstimate(ctx, s.Pool, "feature", fA)
	if err != nil || cur.Tokens != 7000 || cur.Tier != sizing.TierConsidered {
		t.Fatalf("current = %+v, %v; want 7000 considered", cur, err)
	}
	hist, err := EstimateHistory(ctx, s.Pool, "feature", fA)
	if err != nil || len(hist) != 2 {
		t.Fatalf("history len = %d, %v; want 2", len(hist), err)
	}
}

func TestSizingTreeRollup(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	init, _, _ := seedTree(t, s)

	node, err := InitiativeSizingNode(ctx, s.Pool, init)
	if err != nil {
		t.Fatal(err)
	}
	r := sizing.RollUp(node)
	// A = 1200+800 = 2000 (rough), B = 5000 (considered) → 7000, worst = rough.
	if r.Tokens != 7000 {
		t.Errorf("tokens = %d, want 7000", r.Tokens)
	}
	if r.Tier != sizing.TierRough {
		t.Errorf("tier = %s, want rough (T2 rough taints)", r.Tier)
	}
	if !r.Complete {
		t.Error("all leaves estimated → complete")
	}
}

func TestSizingTreeListsUnestimated(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	init, _, fB := seedTree(t, s)

	// Add an unestimated feature under the initiative.
	var fC uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		in, err := GetFeature(ctx, tx, fB)
		if err != nil {
			return err
		}
		c, err := CreateFeature(ctx, tx, in.InitiativeID, "reset", "Reset", "", "sam")
		fC = c.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	node, err := InitiativeSizingNode(ctx, s.Pool, init)
	if err != nil {
		t.Fatal(err)
	}
	r := sizing.RollUp(node)
	if r.Complete {
		t.Error("unestimated feature → not complete")
	}
	found := false
	for _, u := range r.Unestimated {
		if u.ID == fC {
			found = true
		}
	}
	if !found {
		t.Errorf("unestimated listing %+v should name feature %s", r.Unestimated, fC)
	}
}

func TestMilestoneResolveProgressAndLock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	init, fA, fB := seedTree(t, s)

	var ms uuid.UUID
	// Milestone over the whole initiative (transitive to fA, fB).
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		m, err := CreateMilestone(ctx, tx, "v1", "first release", nil, "sam")
		if err != nil {
			return err
		}
		ms = m.ID
		return AddMember(ctx, tx, m.ID, "initiative", init, "sam")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Live resolution expands the initiative to its two features.
	leaves, err := ResolveMembers(ctx, s.Pool, ms)
	if err != nil || len(leaves) != 2 {
		t.Fatalf("resolved leaves = %v, %v; want 2 (fA,fB)", leaves, err)
	}

	// G4 refuses to lock with nothing done.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		_, lockErr := LockMilestone(ctx, tx, ms, "sam")
		if lockErr == nil {
			t.Error("locking with no done member should fail G4")
		}
		return nil // swallow so the tx commits the gate.evaluated audit
	})
	if err != nil {
		t.Fatal(err)
	}

	// Finish feature A, then lock succeeds and snapshots both leaves.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		f, err := GetFeature(ctx, tx, fA)
		if err != nil {
			return err
		}
		// Drive to done directly for the test (bypassing the full gate chain).
		if _, err := tx.Exec(ctx, `UPDATE features SET state = 'done' WHERE id = $1`, f.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		g, err := LockMilestone(ctx, tx, ms, "sam")
		if err != nil || !g.Pass {
			t.Fatalf("lock after one done should pass G4: %+v %v", g, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	m, _ := GetMilestone(ctx, s.Pool, ms)
	if m.State != lifecycle.MilestoneLocked || m.LockedAt == nil {
		t.Fatalf("milestone not locked: %+v", m)
	}
	// Snapshot progress reads the frozen leaves; 1 of 2 done.
	p, err := SnapshotProgress(ctx, s.Pool, ms)
	if err != nil || p.Total != 2 || p.Done != 1 {
		t.Fatalf("snapshot progress = %+v, %v; want 2/1", p, err)
	}

	// Membership is frozen after lock.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		return AddMember(ctx, tx, ms, "feature", fB, "sam")
	})
	if err == nil {
		t.Error("adding a member to a locked milestone should fail")
	}
}

func TestMilestoneDescopeBeforeLock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, fA, fB := seedTree(t, s)

	var ms uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		m, err := CreateMilestone(ctx, tx, "v1", "", nil, "sam")
		if err != nil {
			return err
		}
		ms = m.ID
		if err := AddMember(ctx, tx, m.ID, "feature", fA, "sam"); err != nil {
			return err
		}
		if err := AddMember(ctx, tx, m.ID, "feature", fB, "sam"); err != nil {
			return err
		}
		// fA ships, fB is descoped with a reason before lock.
		if _, err := tx.Exec(ctx, `UPDATE features SET state = 'done' WHERE id = $1`, fA); err != nil {
			return err
		}
		return RemoveMember(ctx, tx, m.ID, "feature", fB, "slipped to v2", "sam")
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := LockMilestone(ctx, tx, ms, "sam")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// The locked snapshot excludes the descoped feature.
	p, err := SnapshotProgress(ctx, s.Pool, ms)
	if err != nil || p.Total != 1 || p.Done != 1 {
		t.Fatalf("snapshot = %+v, %v; want 1/1 (fB descoped)", p, err)
	}
	// The descope with its reason is on the audit trail (FR-5.3).
	events, _ := s.AuditTail(ctx, "milestone", &ms, 100)
	var found bool
	for _, e := range events {
		if e.Kind == "milestone.member_removed" {
			found = true
		}
	}
	if !found {
		t.Error("descope should be audited as milestone.member_removed")
	}
}

func TestRoadmapOrdering(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var r, m1, m2 uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		rm, err := CreateRoadmap(ctx, tx, "2026", "sam")
		if err != nil {
			return err
		}
		a, err := CreateMilestone(ctx, tx, "v1", "", nil, "sam")
		if err != nil {
			return err
		}
		b, err := CreateMilestone(ctx, tx, "v2", "", nil, "sam")
		if err != nil {
			return err
		}
		r, m1, m2 = rm.ID, a.ID, b.ID
		if err := SetRoadmapEntry(ctx, tx, r, m2, 1, "sam"); err != nil {
			return err
		}
		return SetRoadmapEntry(ctx, tx, r, m1, 0, "sam")
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := RoadmapEntries(ctx, s.Pool, r)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	if entries[0].MilestoneID != m1 || entries[1].MilestoneID != m2 {
		t.Errorf("order = %+v, want [m1, m2]", entries)
	}
	// Reorder persists.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		return SetRoadmapEntry(ctx, tx, r, m1, 5, "sam")
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, _ = RoadmapEntries(ctx, s.Pool, r)
	if entries[0].MilestoneID != m2 {
		t.Errorf("after reorder, first should be m2: %+v", entries)
	}
}
