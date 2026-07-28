package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Reads and one mutation the workflow surface (SPEC-007) needs and phase 1–3
// did not: the documents attached to an entity, the milestones and roadmaps an
// entity owns or is a member of, and the resolution of an entity's main design
// document. All are additive over the 0005 owner columns; the CLI never needed
// them. Ownership queries use IS NOT DISTINCT FROM so the project (a null
// owner_id) matches cleanly.

// DocumentsForOwner lists an entity's live (non-superseded) documents, newest
// first — the entity page's Documents section (SPEC-007 FR-3.2). ownerID is nil
// for the project.
func DocumentsForOwner(ctx context.Context, q Querier, ownerType string, ownerID *uuid.UUID) ([]Document, error) {
	rows, err := q.Query(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE owner_type = $1 AND owner_id IS NOT DISTINCT FROM $2 AND state <> 'superseded'
		ORDER BY created_at DESC`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// PrimaryDocForOwner resolves an entity's main design document (SPEC-007
// FR-3.1, DESIGN-008 Q-B): the live document explicitly marked primary, or —
// when none is marked — the earliest-attached live document of type 'design'.
// It returns ErrNotFound when the entity has no design document, which the page
// renders as the "attach a document" empty state, not a blank body.
func PrimaryDocForOwner(ctx context.Context, q Querier, ownerType string, ownerID *uuid.UUID) (*Document, error) {
	d, err := scanDoc(q.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE owner_type = $1 AND owner_id IS NOT DISTINCT FROM $2
		  AND is_primary AND state <> 'superseded'
		LIMIT 1`, ownerType, ownerID))
	if err == nil {
		return d, nil
	}
	if err != ErrNotFound {
		return nil, err
	}
	// No explicit mark: fall back to the first attached design document.
	return scanDoc(q.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE owner_type = $1 AND owner_id IS NOT DISTINCT FROM $2
		  AND type = 'design' AND state <> 'superseded'
		ORDER BY created_at LIMIT 1`, ownerType, ownerID))
}

// SetPrimaryDocument marks a document as its owner's main design document,
// clearing any prior mark for the same owner first so the partial unique index
// holds, all in one transaction with an audit row (SPEC-007 FR-3.1, O-3). The
// "make this the main document" action calls this.
func SetPrimaryDocument(ctx context.Context, tx pgx.Tx, docID uuid.UUID, actor string) error {
	d, err := GetDocument(ctx, tx, docID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE documents SET is_primary = false
		WHERE owner_type = $1 AND owner_id IS NOT DISTINCT FROM $2
		  AND is_primary AND state <> 'superseded'`, d.OwnerType, d.OwnerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE documents SET is_primary = true WHERE id = $1`, docID); err != nil {
		return err
	}
	return Audit(ctx, tx, actor, "document.marked_primary", "document", &docID,
		map[string]any{"owner_type": d.OwnerType})
}

// MilestonesOwnedBy lists the milestones parented directly to an entity
// (SPEC-007 FR-8.1), newest first. ownerID is nil for project-level milestones
// (shown on Home).
func MilestonesOwnedBy(ctx context.Context, q Querier, ownerType string, ownerID *uuid.UUID) ([]Milestone, error) {
	rows, err := q.Query(ctx, `
		SELECT `+milestoneCols+` FROM milestones
		WHERE owner_type = $1 AND owner_id IS NOT DISTINCT FROM $2
		ORDER BY created_at DESC`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Milestone
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// RoadmapsOwnedBy lists the roadmaps parented directly to an entity (SPEC-007
// FR-8.1). ownerID is nil for project-level roadmaps.
func RoadmapsOwnedBy(ctx context.Context, q Querier, ownerType string, ownerID *uuid.UUID) ([]Roadmap, error) {
	rows, err := q.Query(ctx, `
		SELECT `+roadmapCols+` FROM roadmaps
		WHERE owner_type = $1 AND owner_id IS NOT DISTINCT FROM $2
		ORDER BY name`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Roadmap
	for rows.Next() {
		r, err := scanRoadmap(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// MilestonesForMember lists the milestones an entity is a direct member of
// (SPEC-007 FR-8.2) — distinct from the ones it owns. Direct memberships only:
// an initiative that resolves transitively into a milestone is not listed here,
// only one added as a member itself.
func MilestonesForMember(ctx context.Context, q Querier, memberType string, memberID uuid.UUID) ([]Milestone, error) {
	rows, err := q.Query(ctx, `
		SELECT `+prefixedMilestoneCols+` FROM milestone_members mm
		JOIN milestones m ON m.id = mm.milestone_id
		WHERE mm.member_type = $1 AND mm.member_id = $2
		ORDER BY m.created_at DESC`, memberType, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Milestone
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// prefixedMilestoneCols is milestoneCols qualified to the m alias, for joins.
const prefixedMilestoneCols = `m.id, m.name, m.description, m.target_date, m.state, m.locked_at, m.owner_type, m.owner_id, m.created_at`
