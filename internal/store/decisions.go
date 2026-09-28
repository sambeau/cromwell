package store

// Decisions and conventions (SPEC-018): the text pushed into prompts, recorded
// at acceptance, and which decision superseded which (migration 0012).

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SurfacedText is what an accepted decision or conventions revision pushes
// into prompts (SD-14).
type SurfacedText struct {
	DocumentID uuid.UUID
	Ruling     string
	Reason     string
	FromTitle  bool
}

// RecordSurfacedText records, or replaces, a document's surfaced text.
func RecordSurfacedText(ctx context.Context, tx pgx.Tx, t SurfacedText) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO surfaced_texts (document_id, ruling, reason, from_title)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (document_id) DO UPDATE
		SET ruling = EXCLUDED.ruling, reason = EXCLUDED.reason,
		    from_title = EXCLUDED.from_title, recorded_at = now()`,
		t.DocumentID, t.Ruling, t.Reason, t.FromTitle)
	return err
}

// GetSurfacedText reads a document's surfaced text, or ErrNotFound.
func GetSurfacedText(ctx context.Context, q Querier, docID uuid.UUID) (*SurfacedText, error) {
	t := SurfacedText{DocumentID: docID}
	err := q.QueryRow(ctx, `SELECT ruling, reason, from_title FROM surfaced_texts WHERE document_id = $1`, docID).
		Scan(&t.Ruling, &t.Reason, &t.FromTitle)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return &t, err
}

// AcceptedDecision is an accepted decision with what it surfaces.
type AcceptedDecision struct {
	DocID      uuid.UUID
	PublicID   string
	Title      string
	OwnerType  string
	OwnerID    *uuid.UUID
	ApprovedAt *time.Time // the ID's first acceptance, so an amendment doesn't reorder it
	Ruling     string
	Reason     string
	FromTitle  bool
	Recorded   bool // false: no surfaced text is recorded for it
}

// AcceptedDecisions lists every accepted decision owned by the project or by
// one of the given initiatives, with its recorded surfaced text.
func AcceptedDecisions(ctx context.Context, q Querier, initiativeIDs []uuid.UUID) ([]AcceptedDecision, error) {
	if initiativeIDs == nil {
		initiativeIDs = []uuid.UUID{}
	}
	rows, err := q.Query(ctx, `
		SELECT d.id, COALESCE(d.public_id, ''), d.title, d.owner_type, d.owner_id,
		       COALESCE((SELECT min(x.approved_at) FROM documents x WHERE x.public_id = d.public_id), d.approved_at),
		       COALESCE(s.ruling, ''), COALESCE(s.reason, ''), COALESCE(s.from_title, false),
		       s.document_id IS NOT NULL
		FROM documents d LEFT JOIN surfaced_texts s ON s.document_id = d.id
		WHERE d.type = 'decision' AND d.state = 'approved'
		  AND (d.owner_type = 'project' OR (d.owner_type = 'initiative' AND d.owner_id = ANY($1)))
		ORDER BY d.public_id, d.created_at`, initiativeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AcceptedDecision
	for rows.Next() {
		var a AcceptedDecision
		if err := rows.Scan(&a.DocID, &a.PublicID, &a.Title, &a.OwnerType, &a.OwnerID, &a.ApprovedAt,
			&a.Ruling, &a.Reason, &a.FromTitle, &a.Recorded); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AcceptedConventions is the project's accepted conventions document with
// its recorded body, or ErrNotFound.
func AcceptedConventions(ctx context.Context, q Querier) (*Document, *SurfacedText, error) {
	d, err := scanDoc(q.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE type = 'conventions' AND owner_type = 'project' AND state = 'approved'
		ORDER BY approved_at DESC NULLS LAST, created_at DESC LIMIT 1`))
	if err != nil {
		return nil, nil, err
	}
	t, err := GetSurfacedText(ctx, q, d.ID)
	if err != nil {
		return d, nil, err
	}
	return d, t, nil
}

// LiveConventions is the project's conventions document that isn't
// superseded (the newest, while a revision is open), or ErrNotFound.
func LiveConventions(ctx context.Context, q Querier) (*Document, error) {
	return scanDoc(q.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE type = 'conventions' AND state <> 'superseded'
		ORDER BY created_at DESC LIMIT 1`))
}

// RecordSupersession records that one decision revision superseded another.
func RecordSupersession(ctx context.Context, tx pgx.Tx, superseded, superseding uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO decision_supersessions (superseded_id, superseding_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, superseded, superseding)
	return err
}

// Supersession is one recorded pair, by decision ID.
type Supersession struct {
	SupersededID  string // DEC-005
	SupersedingID string // DEC-008
}

// AllSupersessions lists every recorded supersession, by decision ID.
func AllSupersessions(ctx context.Context, q Querier) ([]Supersession, error) {
	rows, err := q.Query(ctx, `
		SELECT COALESCE(a.public_id, ''), COALESCE(b.public_id, '')
		FROM decision_supersessions s
		JOIN documents a ON a.id = s.superseded_id
		JOIN documents b ON b.id = s.superseding_id
		ORDER BY s.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Supersession
	for rows.Next() {
		var s Supersession
		if err := rows.Scan(&s.SupersededID, &s.SupersedingID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DecisionDocuments lists every decision row: the viewer groups them by ID
// and shows the newest revision of each.
func DecisionDocuments(ctx context.Context, q Querier) ([]Document, error) {
	rows, err := q.Query(ctx, `
		SELECT `+docCols+` FROM documents WHERE type = 'decision'
		ORDER BY public_id NULLS LAST, revision, created_at`)
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

// AcceptedWithoutSurfacedText lists accepted decisions and conventions that
// have no surfaced text recorded: those accepted before migration 0012.
func AcceptedWithoutSurfacedText(ctx context.Context, q Querier) ([]Document, error) {
	rows, err := q.Query(ctx, `
		SELECT `+docCols+` FROM documents d
		WHERE d.type IN ('decision', 'conventions') AND d.state = 'approved'
		  AND NOT EXISTS (SELECT 1 FROM surfaced_texts s WHERE s.document_id = d.id)`)
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

// LockDecisionID takes a transaction-scoped lock on a decision's ID, so an
// amendment's acceptance and a supersession of the same decision can't both
// commit (SPEC-018 R18-6).
func LockDecisionID(ctx context.Context, tx pgx.Tx, publicID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "subutai-decision:"+publicID)
	return err
}

// AcceptedDecisionForUpdate is a decision ID's accepted revision, locked for
// the rest of the transaction, or ErrNotFound when none is accepted.
func AcceptedDecisionForUpdate(ctx context.Context, tx pgx.Tx, publicID string) (*Document, error) {
	return scanDoc(tx.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE public_id = $1 AND type = 'decision' AND state = 'approved'
		ORDER BY revision DESC LIMIT 1 FOR UPDATE`, publicID))
}

// DocumentStateForUpdate re-reads a document's state under a row lock.
func DocumentStateForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (string, error) {
	var st string
	err := tx.QueryRow(ctx, `SELECT state FROM documents WHERE id = $1 FOR UPDATE`, id).Scan(&st)
	return st, err
}

// SupersededDecisionAtPath is a decision superseded by another one that still
// sits at its own path (SPEC-018 SD-6): a record, which keeps its page.
func SupersededDecisionAtPath(ctx context.Context, q Querier, path string) (*Document, error) {
	return scanDoc(q.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents d
		WHERE d.path = $1 AND d.type = 'decision' AND d.state = 'superseded'
		  AND EXISTS (SELECT 1 FROM decision_supersessions x WHERE x.superseded_id = d.id)
		ORDER BY d.revision DESC LIMIT 1`, path))
}
