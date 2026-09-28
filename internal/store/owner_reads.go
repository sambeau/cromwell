package store

import (
	"context"
	"strings"

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

// MemberCandidate is one thing that could be added to a milestone, as the
// milestone-side picker shows it (SPEC-010 FR-3.3): its kind, id, name, and —
// for initiatives and features — its readable path.
type MemberCandidate struct {
	Kind string // "initiative" | "feature" | "milestone" (checklists join in M5)
	ID   uuid.UUID
	Name string
	Path string
}

// MemberCandidates lists what could be added to a milestone. With an empty
// query it is limited to scopeRoot's subtree — the initiative, everything under
// it, and the milestones planned there — or the whole project when scopeRoot is
// nil. With a query it searches the whole project by case-insensitive substring
// over name and path (DESIGN-008 §5.1a: subtree by default, search to reach
// anything).
//
// It is a plain name-and-path query, not the full-text index: that index is
// over document sections, so it would match entities by the prose of their
// documents rather than by what they are called (SPEC-010 SD-7).
//
// Each member type is one branch of the union; checklists (M5) add a fourth
// branch without changing the shape (FR-3.4). Current direct members, archived
// initiatives, abandoned features, the milestone itself and any milestone that
// already contains it (FR-1.5) are left out. It returns at most limit rows and
// reports whether there were more.
func MemberCandidates(ctx context.Context, q Querier, milestoneID uuid.UUID, scopeRoot *uuid.UUID, query string, limit int) ([]MemberCandidate, bool, error) {
	pattern := "%" + likeEscape(strings.TrimSpace(query)) + "%"
	rows, err := q.Query(ctx, `
		WITH RECURSIVE tree AS (
			SELECT id, name, archived, slug::text AS path FROM initiatives WHERE parent_id IS NULL
			UNION ALL
			SELECT i.id, i.name, i.archived, t.path || '/' || i.slug
			FROM initiatives i JOIN tree t ON i.parent_id = t.id
		),
		root AS (SELECT path FROM tree WHERE id = $2),
		scoped AS (
			SELECT t.* FROM tree t
			WHERE $3 <> '' OR $2::uuid IS NULL
			   OR t.path = (SELECT path FROM root)
			   OR starts_with(t.path, (SELECT path FROM root) || '/')
		),
		around AS (
			-- milestones that contain this one, which it may not contain back
			SELECT milestone_id AS id FROM milestone_members
			WHERE member_type = 'milestone' AND member_id = $1
			UNION
			SELECT mm.milestone_id FROM milestone_members mm
			JOIN around a ON mm.member_type = 'milestone' AND mm.member_id = a.id
		),
		candidates AS (
			SELECT 'initiative' AS kind, s.id, s.name, s.path FROM scoped s
			WHERE NOT s.archived
			UNION ALL
			SELECT 'feature', f.id, f.name, s.path || '/' || f.slug
			FROM features f JOIN scoped s ON f.initiative_id = s.id
			WHERE NOT s.archived AND f.state <> 'abandoned'
			UNION ALL
			SELECT 'milestone', m.id, m.name, ''
			FROM milestones m
			WHERE m.id <> $1 AND m.id NOT IN (SELECT id FROM around)
			  AND ($3 <> '' OR $2::uuid IS NULL OR m.owner_id IN (SELECT id FROM scoped))
		)
		SELECT c.kind, c.id, c.name, c.path FROM candidates c
		WHERE ($3 = '' OR c.name ILIKE $4 ESCAPE '\' OR c.path ILIKE $4 ESCAPE '\')
		  AND NOT EXISTS (
			SELECT 1 FROM milestone_members mm
			WHERE mm.milestone_id = $1 AND mm.member_type::text = c.kind AND mm.member_id = c.id)
		ORDER BY (c.kind = 'milestone'), c.path, c.name
		LIMIT $5`, milestoneID, scopeRoot, strings.TrimSpace(query), pattern, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []MemberCandidate
	for rows.Next() {
		var c MemberCandidate
		if err := rows.Scan(&c.Kind, &c.ID, &c.Name, &c.Path); err != nil {
			return nil, false, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// likeEscape makes a search term literal inside a LIKE pattern, so a % or _
// someone types matches itself.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

