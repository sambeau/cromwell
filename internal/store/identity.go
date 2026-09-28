package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/ident"
)

// IDs (SPEC-015). Entities get theirs from a column default at insert, so
// nothing here mints an entity ID; this file reads IDs back, works out the ID
// a new document should take, and records documents' identities and moves.

// OwnerPublicID is the owner part of a document's ID: an initiative's or
// feature's ID, or "PROJECT" (SD-6).
func OwnerPublicID(ctx context.Context, q Querier, ownerType string, ownerID *uuid.UUID) (string, error) {
	switch ownerType {
	case "project":
		return ident.ProjectOwner, nil
	case "initiative", "feature":
		if ownerID == nil {
			return "", fmt.Errorf("a %s owner needs an id", ownerType)
		}
		var pid string
		err := q.QueryRow(ctx, `SELECT public_id FROM `+ownerType+`s WHERE id = $1`, *ownerID).Scan(&pid)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return pid, err
	}
	return "", fmt.Errorf("unknown owner type %q", ownerType)
}

// IdentityUse says how an ID has been used so far: the highest revision any
// row has carried, and whether one of them is live (not superseded).
func IdentityUse(ctx context.Context, q Querier, publicID string) (maxRevision int, live bool, err error) {
	err = q.QueryRow(ctx, `
		SELECT COALESCE(max(revision), 0), COALESCE(bool_or(state <> 'superseded'), false)
		FROM documents WHERE public_id = $1`, publicID).Scan(&maxRevision, &live)
	return maxRevision, live, err
}

// NextDocumentIdentity is the ID and revision the next document of a type
// takes for an owner (SD-6, FR-3.2): the first of base, base-2, base-3 … with
// no live document, at one more than the highest revision it has had. So a
// spec superseded outright by the cascade is followed by the same ID at the
// next revision, and a second live design takes -2. Decisions are numbered
// from their own sequence instead; see MintDecisionID.
func NextDocumentIdentity(ctx context.Context, q Querier, ownerType string, ownerID *uuid.UUID, docType string) (string, int, error) {
	owner, err := OwnerPublicID(ctx, q, ownerType, ownerID)
	if err != nil {
		return "", 0, err
	}
	for n := 1; ; n++ {
		id := ident.DocumentID(owner, docType, n)
		maxRev, live, err := IdentityUse(ctx, q, id)
		if err != nil {
			return "", 0, err
		}
		if !live {
			return id, maxRev + 1, nil
		}
	}
}

// MintDecisionID draws the next decision number, "DEC-008".
func MintDecisionID(ctx context.Context, tx pgx.Tx) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT mint_ident('DEC')`).Scan(&id)
	return id, err
}

// AdvanceIdent moves a prefix's sequence so the next number minted is above n
// (FR-5.5). It never moves a sequence backwards.
func AdvanceIdent(ctx context.Context, tx pgx.Tx, prefix string, n int64) error {
	if _, ok := ident.KindByPrefix(prefix); !ok {
		return fmt.Errorf("unknown ID prefix %q", prefix)
	}
	_, err := tx.Exec(ctx, `SELECT ident_advance($1, $2)`, prefix, n)
	return err
}

// SetDocumentIdentity records a document's ID and revision.
func SetDocumentIdentity(ctx context.Context, tx pgx.Tx, docID uuid.UUID, publicID string, revision int) error {
	_, err := tx.Exec(ctx, `UPDATE documents SET public_id = $2, revision = $3 WHERE id = $1`,
		docID, publicID, revision)
	return err
}

// DocumentByIdentity finds the one row carrying an ID at a revision.
func DocumentByIdentity(ctx context.Context, q Querier, publicID string, revision int) (*Document, error) {
	return scanDoc(q.QueryRow(ctx,
		`SELECT `+docCols+` FROM documents WHERE public_id = $1 AND revision = $2`, publicID, revision))
}

// CurrentDocumentByPublicID is what an ID names without a revision: its
// newest live row, or failing that its newest row. While a revision is open
// that is the successor draft, the document being worked on.
func CurrentDocumentByPublicID(ctx context.Context, q Querier, publicID string) (*Document, error) {
	return scanDoc(q.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents WHERE public_id = $1
		ORDER BY (state <> 'superseded') DESC, revision DESC LIMIT 1`, publicID))
}

// LiveSuccessorOf finds a live document that revises the given one.
func LiveSuccessorOf(ctx context.Context, q Querier, docID uuid.UUID) (*Document, error) {
	return scanDoc(q.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents WHERE supersedes_id = $1 AND state <> 'superseded'
		ORDER BY created_at DESC LIMIT 1`, docID))
}

// IdentifiedDocuments lists every document row that has an ID, superseded
// ones included: the catch-up scan looks for any whose file has moved.
func IdentifiedDocuments(ctx context.Context, q Querier) ([]Document, error) {
	rows, err := q.Query(ctx, `SELECT `+docCols+` FROM documents WHERE public_id IS NOT NULL ORDER BY created_at`)
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

// MoveDocument re-points a document at the path its file now has, found by
// the ID in its front matter (SPEC-015 FR-4.2).
func MoveDocument(ctx context.Context, tx pgx.Tx, d *Document, to, actor string) error {
	if _, err := tx.Exec(ctx, `UPDATE documents SET path = $2 WHERE id = $1`, d.ID, to); err != nil {
		return err
	}
	return Audit(ctx, tx, actor, "document.moved", "document", &d.ID, map[string]any{
		"from": d.Path, "to": to, "public_id": d.PublicID, "revision": d.Revision,
	})
}

// MarkDocumentAdoptedApproved records a document adopted as already approved
// (SPEC-015 FR-5.3): the person adopting it is approving it, so approved_at is
// stamped now and the adoption's audit row names them.
func MarkDocumentAdoptedApproved(ctx context.Context, tx pgx.Tx, docID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE documents SET state = 'approved', approved_at = now() WHERE id = $1 AND state = 'draft'`, docID)
	return err
}

// --- Entities by their IDs ---

// InitiativeByPublicID finds an initiative by its ID, "INIT-014".
func InitiativeByPublicID(ctx context.Context, q Querier, publicID string) (*Initiative, error) {
	var id uuid.UUID
	if err := q.QueryRow(ctx, `SELECT id FROM initiatives WHERE public_id = $1`,
		strings.ToUpper(publicID)).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return GetInitiative(ctx, q, id)
}

// FeatureByPublicID finds a feature by its ID, "FEAT-023".
func FeatureByPublicID(ctx context.Context, q Querier, publicID string) (*Feature, error) {
	var id uuid.UUID
	if err := q.QueryRow(ctx, `SELECT id FROM features WHERE public_id = $1`,
		strings.ToUpper(publicID)).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return GetFeature(ctx, q, id)
}

// TaskByPublicID finds a task by its ID, "FEAT-023-T03".
func TaskByPublicID(ctx context.Context, q Querier, publicID string) (*Task, error) {
	return scanTask(q.QueryRow(ctx, `SELECT `+taskCols+` FROM tasks WHERE public_id = $1`, strings.ToUpper(publicID)))
}

// MilestoneByPublicID finds a milestone by its ID, "MS-004".
func MilestoneByPublicID(ctx context.Context, q Querier, publicID string) (*Milestone, error) {
	return scanMilestone(q.QueryRow(ctx, `SELECT `+milestoneCols+` FROM milestones WHERE public_id = $1`, strings.ToUpper(publicID)))
}

// RoadmapByPublicID finds a roadmap by its ID, "RM-001".
func RoadmapByPublicID(ctx context.Context, q Querier, publicID string) (*Roadmap, error) {
	return scanRoadmap(q.QueryRow(ctx, `SELECT `+roadmapCols+` FROM roadmaps WHERE public_id = $1`, strings.ToUpper(publicID)))
}

// IdentSequences lists the ident_<prefix>_seq sequences the database has, as
// prefixes: the registry test checks they match internal/ident (SD-2).
func IdentSequences(ctx context.Context, q Querier) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT upper(substring(sequencename from '^ident_(.*)_seq$'))
		FROM pg_sequences WHERE schemaname = current_schema() AND sequencename LIKE 'ident\_%\_seq'
		ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
