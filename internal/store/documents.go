package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/content"
	"cromwell/internal/lifecycle"
)

type Document struct {
	ID           uuid.UUID
	Type         string
	State        lifecycle.DocumentState
	OwnerType    string
	OwnerID      *uuid.UUID
	Path         string
	Title        string
	Tags         []string
	SupersedesID *uuid.UUID
	ContentHash  string
	IndexedAt    *time.Time
	SubmittedAt  *time.Time
	ApprovedAt   *time.Time
	CreatedAt    time.Time
}

const docCols = `id, type, state, owner_type, owner_id, path, title, tags,
	supersedes_id, content_hash, indexed_at, submitted_at, approved_at, created_at`

func scanDoc(row pgx.Row) (*Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.Type, &d.State, &d.OwnerType, &d.OwnerID, &d.Path,
		&d.Title, &d.Tags, &d.SupersedesID, &d.ContentHash, &d.IndexedAt,
		&d.SubmittedAt, &d.ApprovedAt, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &d, err
}

// RegisterDocument creates a document row in draft (FR-4.1). supersedes is
// non-nil for revisions (FR-4.4).
func RegisterDocument(ctx context.Context, tx pgx.Tx, docType, ownerType string, ownerID *uuid.UUID,
	path, title, contentHash string, supersedes *uuid.UUID, actor string) (*Document, error) {
	d := &Document{
		ID: NewID(), Type: docType, State: lifecycle.DocDraft,
		OwnerType: ownerType, OwnerID: ownerID, Path: path, Title: title,
		ContentHash: contentHash, SupersedesID: supersedes, Tags: []string{},
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO documents (id, type, state, owner_type, owner_id, path, title, tags, supersedes_id, content_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		d.ID, d.Type, d.State, d.OwnerType, d.OwnerID, d.Path, d.Title, d.Tags, d.SupersedesID, d.ContentHash)
	if err != nil {
		return nil, err
	}
	kind := "document.registered"
	payload := map[string]any{"path": path, "type": docType}
	if supersedes != nil {
		kind = "document.revision_created"
		payload["supersedes"] = supersedes.String()
	}
	if err := Audit(ctx, tx, actor, kind, "document", &d.ID, payload); err != nil {
		return nil, err
	}
	return d, nil
}

func GetDocument(ctx context.Context, q Querier, id uuid.UUID) (*Document, error) {
	return scanDoc(q.QueryRow(ctx, `SELECT `+docCols+` FROM documents WHERE id = $1`, id))
}

// LiveDocumentByPath finds the non-superseded document at a repo path.
func LiveDocumentByPath(ctx context.Context, q Querier, path string) (*Document, error) {
	return scanDoc(q.QueryRow(ctx,
		`SELECT `+docCols+` FROM documents WHERE path = $1 AND state <> 'superseded'`, path))
}

// CurrentDocForOwner returns the live (non-superseded) document of a type
// owned by an entity — "the current spec of feature X" (DESIGN-001 §5).
func CurrentDocForOwner(ctx context.Context, q Querier, docType, ownerType string, ownerID uuid.UUID) (*Document, error) {
	return scanDoc(q.QueryRow(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE type = $1 AND owner_type = $2 AND owner_id = $3 AND state <> 'superseded'
		ORDER BY created_at DESC LIMIT 1`, docType, ownerType, ownerID))
}

// TransitionDocument applies a lifecycle event and stamps the relevant
// timestamp columns, audit row in the same transaction (FR-5.1).
func TransitionDocument(ctx context.Context, tx pgx.Tx, d *Document, event lifecycle.DocumentEvent, actor string, payload map[string]any) error {
	next, err := lifecycle.DocumentTransition(d.State, event)
	if err != nil {
		return err
	}
	stamp := ""
	switch event {
	case lifecycle.DocSubmit:
		stamp = ", submitted_at = now()"
	case lifecycle.DocApprove:
		stamp = ", approved_at = now()"
	}
	if _, err := tx.Exec(ctx, `UPDATE documents SET state = $2`+stamp+` WHERE id = $1`, d.ID, next); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["from"] = string(d.State)
	payload["to"] = string(next)
	payload["event"] = string(event)
	if err := Audit(ctx, tx, actor, "document.transition", "document", &d.ID, payload); err != nil {
		return err
	}
	d.State = next
	return nil
}

// UpdateDocumentPath re-points a document's file (revision takeover,
// DESIGN-003 §5).
func UpdateDocumentPath(ctx context.Context, tx pgx.Tx, id uuid.UUID, path string) error {
	_, err := tx.Exec(ctx, `UPDATE documents SET path = $2 WHERE id = $1`, id, path)
	return err
}

// ReplaceSections rebuilds a document's section index (delete + insert,
// DESIGN-001 §5) and stamps content_hash / indexed_at.
func ReplaceSections(ctx context.Context, tx pgx.Tx, docID uuid.UUID, contentHash string, sections []content.Section) error {
	if _, err := tx.Exec(ctx, `DELETE FROM document_sections WHERE document_id = $1`, docID); err != nil {
		return err
	}
	for _, sec := range sections {
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_sections (id, document_id, position, heading, level, content)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			NewID(), docID, sec.Position, sec.Heading, sec.Level, sec.Content); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx,
		`UPDATE documents SET content_hash = $2, indexed_at = now() WHERE id = $1`,
		docID, contentHash)
	return err
}

// SearchHit is one FTS result (cromwell search, FR-4.1).
type SearchHit struct {
	DocID   uuid.UUID
	Path    string
	Title   string
	Heading string
	Snippet string
}

func (s *Store) SearchSections(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT d.id, d.path, d.title, sec.heading,
		       ts_headline('english', sec.content, websearch_to_tsquery('english', $1),
		                   'MaxWords=20, MinWords=5') AS snippet
		FROM document_sections sec
		JOIN documents d ON d.id = sec.document_id
		WHERE sec.fts @@ websearch_to_tsquery('english', $1)
		  AND d.state <> 'superseded'
		ORDER BY ts_rank(sec.fts, websearch_to_tsquery('english', $1)) DESC
		LIMIT $2`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.DocID, &h.Path, &h.Title, &h.Heading, &h.Snippet); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// Sections returns a document's indexed sections in order (prompt assembly).
func Sections(ctx context.Context, q Querier, docID uuid.UUID) ([]content.Section, error) {
	rows, err := q.Query(ctx, `
		SELECT position, heading, level, content FROM document_sections
		WHERE document_id = $1 ORDER BY position`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []content.Section
	for rows.Next() {
		var s content.Section
		if err := rows.Scan(&s.Position, &s.Heading, &s.Level, &s.Content); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type Comment struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	DispatchID *uuid.UUID
	Author     string
	SectionRef string
	Body       string
	Resolved   bool
	CreatedAt  time.Time
}

func InsertComment(ctx context.Context, tx pgx.Tx, docID uuid.UUID, dispatchID *uuid.UUID, author, sectionRef, body string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO document_comments (id, document_id, dispatch_id, author, section_ref, body)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		NewID(), docID, dispatchID, author, nullable(sectionRef), body)
	return err
}

// CommentsForDocument returns the comment thread, unresolved first-class:
// they persist across resubmission (DESIGN-001 §5).
func CommentsForDocument(ctx context.Context, q Querier, docID uuid.UUID, unresolvedOnly bool) ([]Comment, error) {
	sql := `SELECT id, document_id, dispatch_id, author, COALESCE(section_ref, ''), body, resolved, created_at
		FROM document_comments WHERE document_id = $1`
	if unresolvedOnly {
		sql += ` AND NOT resolved`
	}
	sql += ` ORDER BY created_at`
	rows, err := q.Query(ctx, sql, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.DispatchID, &c.Author, &c.SectionRef, &c.Body, &c.Resolved, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ApprovedDocsOwnedBy returns approved documents owned by any of the given
// entities — the "ancestry" auto-surfacing tier (DESIGN-002 §4 step 3).
func ApprovedDocsOwnedBy(ctx context.Context, q Querier, ownerType string, ownerIDs []uuid.UUID) ([]Document, error) {
	if len(ownerIDs) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		SELECT `+docCols+` FROM documents
		WHERE owner_type = $1 AND owner_id = ANY($2) AND state = 'approved'
		ORDER BY created_at`, ownerType, ownerIDs)
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

// RefreshDocumentTitle updates the display title from freshly parsed front
// matter (a document registered from a template keeps its placeholder title
// until the file gains a real one — found in the live smoke test).
func RefreshDocumentTitle(ctx context.Context, tx pgx.Tx, id uuid.UUID, title string) error {
	if title == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE documents SET title = $2 WHERE id = $1 AND title <> $2`, id, title)
	return err
}
