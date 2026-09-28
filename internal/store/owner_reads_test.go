package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TestOwnerReadsAndPrimaryDoc covers migration 0005 and the workflow-surface
// reads: existing rows become project-owned, an initiative can own a milestone,
// primary-document resolution falls back to the first design doc and honours an
// explicit mark, and an entity's memberships and documents read back correctly.
func TestOwnerReadsAndPrimaryDoc(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var (
		initID, featID            uuid.UUID
		designID, specID          uuid.UUID
		ownedMS, projectMS, memMS uuid.UUID
	)
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		in, err := CreateInitiative(ctx, tx, nil, "auth", "Authentication", "How people sign in", "sam")
		if err != nil {
			return err
		}
		initID = in.ID
		f, err := CreateFeature(ctx, tx, in.ID, "login", "Login form", "", "sam")
		if err != nil {
			return err
		}
		featID = f.ID

		// Two documents on the feature: a design doc and a spec, spec attached
		// first to prove the design-type fallback ignores non-design docs.
		fid := f.ID
		spec, err := RegisterDocument(ctx, tx, "spec", "feature", &fid,
			"docs/specs/login.md", "Login spec", "h-spec", nil, "sam")
		if err != nil {
			return err
		}
		specID = spec.ID
		design, err := RegisterDocument(ctx, tx, "design", "feature", &fid,
			"docs/design/login.md", "Login design", "h-design", nil, "sam")
		if err != nil {
			return err
		}
		designID = design.ID

		// A milestone owned by the initiative, and a project-owned one.
		om, err := CreateMilestone(ctx, tx, "project", nil, "Auth v1", "", nil, "sam")
		if err != nil {
			return err
		}
		ownedMS = om.ID
		if _, err := tx.Exec(ctx,
			`UPDATE milestones SET owner_type='initiative', owner_id=$2 WHERE id=$1`, om.ID, in.ID); err != nil {
			return err
		}
		pm, err := CreateMilestone(ctx, tx, "project", nil, "Project launch", "", nil, "sam")
		if err != nil {
			return err
		}
		projectMS = pm.ID

		// A third milestone the feature is a member of (owned by project).
		mm, err := CreateMilestone(ctx, tx, "project", nil, "Cross-cutting", "", nil, "sam")
		if err != nil {
			return err
		}
		memMS = mm.ID
		return AddMember(ctx, tx, mm.ID, "feature", f.ID, "sam")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Existing/default milestone is project-owned.
	pm, err := GetMilestone(ctx, s.Pool, projectMS)
	if err != nil || pm.OwnerType != "project" || pm.OwnerID != nil {
		t.Fatalf("project milestone owner = %s/%v, err %v; want project/nil", pm.OwnerType, pm.OwnerID, err)
	}

	// DocumentsForOwner lists both docs on the feature.
	docs, err := DocumentsForOwner(ctx, s.Pool, "feature", &featID)
	if err != nil || len(docs) != 2 {
		t.Fatalf("DocumentsForOwner = %d docs, err %v; want 2", len(docs), err)
	}

	// Primary resolves to the design doc by fallback (spec is ignored though
	// attached first).
	prim, err := PrimaryDocForOwner(ctx, s.Pool, "feature", &featID)
	if err != nil || prim.ID != designID {
		t.Fatalf("primary fallback = %v, err %v; want design %v", primID(prim), err, designID)
	}

	// Marking the spec primary overrides the fallback.
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		return SetPrimaryDocument(ctx, tx, specID, "sam")
	}); err != nil {
		t.Fatal(err)
	}
	prim, err = PrimaryDocForOwner(ctx, s.Pool, "feature", &featID)
	if err != nil || prim.ID != specID {
		t.Fatalf("primary after mark = %v; want spec %v", primID(prim), specID)
	}

	// MilestonesOwnedBy: the initiative owns exactly one; the project owns two
	// (Project launch + Cross-cutting).
	owned, err := MilestonesOwnedBy(ctx, s.Pool, "initiative", &initID)
	if err != nil || len(owned) != 1 || owned[0].ID != ownedMS {
		t.Fatalf("MilestonesOwnedBy(initiative) = %d, err %v; want 1 (%v)", len(owned), err, ownedMS)
	}
	projOwned, err := MilestonesOwnedBy(ctx, s.Pool, "project", nil)
	if err != nil || len(projOwned) != 2 {
		t.Fatalf("MilestonesOwnedBy(project) = %d, err %v; want 2", len(projOwned), err)
	}

	// MilestonesForMember: the feature is a direct member of exactly memMS.
	memberOf, err := MilestonesForMember(ctx, s.Pool, "feature", featID)
	if err != nil || len(memberOf) != 1 || memberOf[0].ID != memMS {
		t.Fatalf("MilestonesForMember = %d, err %v; want 1 (%v)", len(memberOf), err, memMS)
	}
}

func primID(d *Document) any {
	if d == nil {
		return nil
	}
	return d.ID
}

// TestUpdateEntityFields covers the shared update method: partial updates leave
// the untouched field alone and each writes an audit row.
func TestUpdateEntityFields(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var initID uuid.UUID
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		in, err := CreateInitiative(ctx, tx, nil, "auth", "Authentication", "old description", "sam")
		if err != nil {
			return err
		}
		initID = in.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Update only the description; name is left unchanged.
	newDesc := "How people prove who they are when they sign in"
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		return UpdateEntityFields(ctx, tx, "initiative", initID, nil, &newDesc, "operator")
	}); err != nil {
		t.Fatal(err)
	}
	in, err := GetInitiative(ctx, s.Pool, initID)
	if err != nil || in.Name != "Authentication" || in.Description != newDesc {
		t.Fatalf("after update: name=%q desc=%q err=%v", in.Name, in.Description, err)
	}

	var n int
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_events WHERE kind='initiative.updated' AND ref_id=$1`, initID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit rows = %d, err %v; want 1", n, err)
	}
}
