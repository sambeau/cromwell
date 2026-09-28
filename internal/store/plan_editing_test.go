package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The store half of SPEC-010: owner-scoped creation, dense roadmap ordering,
// taking a milestone off a roadmap, the nesting-cycle guard, and the
// milestone-side picker's candidate read.

func TestOwnerScopedCreation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	initID, _, _ := seedTree(t, s)

	var projM, initM *Milestone
	var initR *Roadmap
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if projM, err = CreateMilestone(ctx, tx, "project", nil, "Launch", "", nil, "sam"); err != nil {
			return err
		}
		if initM, err = CreateMilestone(ctx, tx, "initiative", &initID, "Auth beta", "Sign-in works", nil, "sam"); err != nil {
			return err
		}
		initR, err = CreateRoadmap(ctx, tx, "initiative", &initID, "Auth plan", "sam")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	owned, err := MilestonesOwnedBy(ctx, s.Pool, "initiative", &initID)
	if err != nil || len(owned) != 1 || owned[0].ID != initM.ID {
		t.Fatalf("initiative-owned milestones = %+v, %v; want just Auth beta", owned, err)
	}
	proj, _ := MilestonesOwnedBy(ctx, s.Pool, "project", nil)
	if len(proj) != 1 || proj[0].ID != projM.ID {
		t.Errorf("project-owned milestones = %+v; want just Launch", proj)
	}
	rms, _ := RoadmapsOwnedBy(ctx, s.Pool, "initiative", &initID)
	if len(rms) != 1 || rms[0].ID != initR.ID {
		t.Errorf("initiative-owned roadmaps = %+v; want Auth plan", rms)
	}

	// The audit row names the owner.
	var ownerType string
	if err := s.Pool.QueryRow(ctx, `
		SELECT payload->>'owner_type' FROM audit_events
		WHERE kind = 'roadmap.created' AND ref_type = 'initiative' AND ref_id = $1`, initID).Scan(&ownerType); err != nil {
		t.Fatalf("roadmap.created audit filed against its owner: %v", err)
	}
	if ownerType != "initiative" {
		t.Errorf("audit owner_type = %q", ownerType)
	}

	// A bad owner or a blank name writes nothing.
	for _, bad := range []func(tx pgx.Tx) error{
		func(tx pgx.Tx) error {
			_, e := CreateMilestone(ctx, tx, "initiative", nil, "x", "", nil, "sam")
			return e
		},
		func(tx pgx.Tx) error {
			_, e := CreateMilestone(ctx, tx, "feature", &initID, "x", "", nil, "sam")
			return e
		},
		func(tx pgx.Tx) error {
			_, e := CreateMilestone(ctx, tx, "project", nil, "   ", "", nil, "sam")
			return e
		},
		func(tx pgx.Tx) error { _, e := CreateRoadmap(ctx, tx, "project", &initID, "x", "sam"); return e },
		func(tx pgx.Tx) error { _, e := CreateRoadmap(ctx, tx, "project", nil, "", "sam"); return e },
	} {
		if err := s.WithTx(ctx, bad); err == nil {
			t.Error("a bad owner or blank name was accepted")
		}
	}
	all, _ := ListMilestones(ctx, s.Pool)
	if len(all) != 2 {
		t.Errorf("milestones after refused creates = %d, want 2", len(all))
	}
}

func TestPlaceAndRemoveRoadmapEntries(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var r uuid.UUID
	ms := make([]uuid.UUID, 3)
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		rm, err := CreateRoadmap(ctx, tx, "project", nil, "2026", "sam")
		if err != nil {
			return err
		}
		r = rm.ID
		for i, name := range []string{"a", "b", "c"} {
			m, err := CreateMilestone(ctx, tx, "project", nil, name, "", nil, "sam")
			if err != nil {
				return err
			}
			ms[i] = m.ID
			// Old-style positions that tie: every entry at 0.
			if err := SetRoadmapEntry(ctx, tx, r, m.ID, 0, "sam"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dense := false // the seeded positions tie until the first placement
	order := func() []uuid.UUID {
		t.Helper()
		es, err := RoadmapEntries(ctx, s.Pool, r)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]uuid.UUID, len(es))
		for i, e := range es {
			out[i] = e.MilestoneID
			if dense && e.Position != i+1 {
				t.Errorf("positions are not dense: %+v", es)
			}
		}
		return out
	}
	place := func(m uuid.UUID, at int) int {
		t.Helper()
		var got int
		if err := s.WithTx(ctx, func(tx pgx.Tx) error {
			var e error
			got, e = PlaceRoadmapEntry(ctx, tx, r, m, at, "sam")
			return e
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}
	same := func(got, want []uuid.UUID) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	// Even on tied positions, moving c to the front works and renumbers.
	start := order() // tie-broken by id; take it as the baseline
	dense = true
	c := start[2]
	if got := place(c, 1); got != 1 {
		t.Errorf("placed at %d, want 1", got)
	}
	if o := order(); !same(o, []uuid.UUID{c, start[0], start[1]}) {
		t.Errorf("after moving to first: %v", o)
	}
	// Move down one.
	place(c, 2)
	if o := order(); !same(o, []uuid.UUID{start[0], c, start[1]}) {
		t.Errorf("after moving down: %v", o)
	}
	// 0 and past-the-end both mean the end.
	if got := place(c, 0); got != 3 {
		t.Errorf("place 0 landed at %d, want 3", got)
	}
	place(start[0], 99)
	if o := order(); !same(o, []uuid.UUID{start[1], c, start[0]}) {
		t.Errorf("after placing past the end: %v", o)
	}

	// Taking one off renumbers the rest and is audited.
	if err := s.WithTx(ctx, func(tx pgx.Tx) error { return RemoveRoadmapEntry(ctx, tx, r, c, "sam") }); err != nil {
		t.Fatal(err)
	}
	if o := order(); !same(o, []uuid.UUID{start[1], start[0]}) {
		t.Errorf("after removal: %v", o)
	}
	var n int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'roadmap.entry_removed' AND ref_id = $1`, c).Scan(&n)
	if n != 1 {
		t.Errorf("entry_removed audit rows = %d", n)
	}
	// Removing what isn't there is not-found.
	err = s.WithTx(ctx, func(tx pgx.Tx) error { return RemoveRoadmapEntry(ctx, tx, r, c, "sam") })
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("second removal: %v, want ErrNotFound", err)
	}
}

func TestMilestoneCannotContainItself(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var a, b, c uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		for _, p := range []*uuid.UUID{&a, &b, &c} {
			m, err := CreateMilestone(ctx, tx, "project", nil, "m", "", nil, "sam")
			if err != nil {
				return err
			}
			*p = m.ID
		}
		return AddMember(ctx, tx, a, "milestone", b, "sam") // B inside A
	})
	if err != nil {
		t.Fatal(err)
	}
	add := func(outer, inner uuid.UUID) error {
		return s.WithTx(ctx, func(tx pgx.Tx) error { return AddMember(ctx, tx, outer, "milestone", inner, "sam") })
	}
	if err := add(a, a); !errors.Is(err, ErrMilestoneCycle) {
		t.Errorf("A inside A: %v", err)
	}
	if err := add(b, a); !errors.Is(err, ErrMilestoneCycle) {
		t.Errorf("A inside B, when B is inside A: %v", err)
	}
	// Deeper: C inside B, then A inside C closes a loop.
	if err := add(b, c); err != nil {
		t.Fatal(err)
	}
	if err := add(c, a); !errors.Is(err, ErrMilestoneCycle) {
		t.Errorf("A inside C, when C is inside B inside A: %v", err)
	}
	// Unrelated nesting still works.
	if err := add(a, c); err != nil {
		t.Errorf("C inside A (no cycle): %v", err)
	}
}

func TestMemberCandidates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var auth, billing, login, invoices, owned, other, self uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		a, err := CreateInitiative(ctx, tx, nil, "auth", "Authentication", "", "sam")
		if err != nil {
			return err
		}
		sub, err := CreateInitiative(ctx, tx, &a.ID, "basic", "Basic sign-in", "", "sam")
		if err != nil {
			return err
		}
		l, err := CreateFeature(ctx, tx, sub.ID, "login", "Login form", "", "sam")
		if err != nil {
			return err
		}
		b, err := CreateInitiative(ctx, tx, nil, "billing", "Billing", "", "sam")
		if err != nil {
			return err
		}
		inv, err := CreateFeature(ctx, tx, b.ID, "invoices", "Invoices 100%_done", "", "sam")
		if err != nil {
			return err
		}
		m, err := CreateMilestone(ctx, tx, "initiative", &a.ID, "Auth beta", "", nil, "sam")
		if err != nil {
			return err
		}
		o, err := CreateMilestone(ctx, tx, "initiative", &sub.ID, "Basic done", "", nil, "sam")
		if err != nil {
			return err
		}
		x, err := CreateMilestone(ctx, tx, "project", nil, "Launch", "", nil, "sam")
		if err != nil {
			return err
		}
		// Launch contains Auth beta, so Auth beta may not contain Launch.
		if err := AddMember(ctx, tx, x.ID, "milestone", m.ID, "sam"); err != nil {
			return err
		}
		// Login is already in Auth beta.
		if err := AddMember(ctx, tx, m.ID, "feature", l.ID, "sam"); err != nil {
			return err
		}
		auth, billing, login, invoices, self, owned, other = a.ID, b.ID, l.ID, inv.ID, m.ID, o.ID, x.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := func(cs []MemberCandidate) map[uuid.UUID]MemberCandidate {
		out := map[uuid.UUID]MemberCandidate{}
		for _, c := range cs {
			out[c.ID] = c
		}
		return out
	}

	// Default: the owner's subtree only.
	cs, more, err := MemberCandidates(ctx, s.Pool, self, &auth, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	got := ids(cs)
	if more {
		t.Error("more = true on a small set")
	}
	if c, ok := got[auth]; !ok || c.Kind != "initiative" || c.Path != "auth" {
		t.Errorf("subtree should offer the owner itself: %+v", cs)
	}
	if _, ok := got[owned]; !ok {
		t.Error("subtree should offer a milestone planned inside it")
	}
	for id, why := range map[uuid.UUID]string{
		billing: "an initiative outside the subtree", invoices: "a feature outside the subtree",
		login: "a current member", self: "the milestone itself", other: "a milestone that contains it",
	} {
		if _, ok := got[id]; ok {
			t.Errorf("default candidates include %s", why)
		}
	}

	// A search reaches the whole project, by name or by path, case-insensitive.
	cs, _, _ = MemberCandidates(ctx, s.Pool, self, &auth, "INVOICE", 50)
	if _, ok := ids(cs)[invoices]; !ok {
		t.Errorf("search by name should reach outside the subtree: %+v", cs)
	}
	cs, _, _ = MemberCandidates(ctx, s.Pool, self, &auth, "billing/inv", 50)
	if _, ok := ids(cs)[invoices]; !ok {
		t.Errorf("search by path: %+v", cs)
	}
	// % and _ are literal.
	cs, _, _ = MemberCandidates(ctx, s.Pool, self, &auth, "100%_", 50)
	if len(cs) != 1 || cs[0].ID != invoices {
		t.Errorf("literal %%_ search = %+v", cs)
	}
	cs, _, _ = MemberCandidates(ctx, s.Pool, self, &auth, "%", 50)
	if len(cs) != 1 {
		t.Errorf("a lone %% should match only names containing it: %+v", cs)
	}

	// A project-scoped default covers everything, and the limit reports more.
	cs, more, _ = MemberCandidates(ctx, s.Pool, self, nil, "", 2)
	if len(cs) != 2 || !more {
		t.Errorf("limit 2 over the project: %d rows, more=%v", len(cs), more)
	}
}
