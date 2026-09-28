package store

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Send to development (SPEC-011): the sent mark, held documents and human
// issues. Each mutation takes the caller's transaction and writes its audit
// row inside it (O-3).

// ErrAlreadySent is returned when a feature already carries the sent mark.
var ErrAlreadySent = errors.New("already sent")

// FeatureSend is a feature's sent mark (FR-1.1): who sent it, when, and the
// send's hold.
type FeatureSend struct {
	FeatureID uuid.UUID
	SentBy    string
	SentAt    time.Time
	Hold      bool
}

// GetFeatureSend returns a feature's mark, or ErrNotFound when it is unsent.
func GetFeatureSend(ctx context.Context, q Querier, featureID uuid.UUID) (*FeatureSend, error) {
	var fs FeatureSend
	err := q.QueryRow(ctx, `SELECT feature_id, sent_by, sent_at, hold FROM feature_sends WHERE feature_id = $1`, featureID).
		Scan(&fs.FeatureID, &fs.SentBy, &fs.SentAt, &fs.Hold)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &fs, err
}

// RecordFeatureSend writes the mark and its feature.sent audit row.
func RecordFeatureSend(ctx context.Context, tx pgx.Tx, featureID uuid.UUID, actor string, hold bool) (*FeatureSend, error) {
	fs := &FeatureSend{FeatureID: featureID, SentBy: actor, Hold: hold}
	err := tx.QueryRow(ctx, `
		INSERT INTO feature_sends (feature_id, sent_by, hold) VALUES ($1, $2, $3)
		ON CONFLICT (feature_id) DO NOTHING
		RETURNING sent_at`, featureID, actor, hold).Scan(&fs.SentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAlreadySent
	}
	if err != nil {
		return nil, err
	}
	return fs, Audit(ctx, tx, actor, "feature.sent", "feature", &featureID, map[string]any{"hold": hold})
}

// DeleteFeatureSend removes the mark (Withdraw, FR-4.6) and audits it.
func DeleteFeatureSend(ctx context.Context, tx pgx.Tx, featureID uuid.UUID, actor string, cancelled int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM feature_sends WHERE feature_id = $1`, featureID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return Audit(ctx, tx, actor, "feature.send_withdrawn", "feature", &featureID,
		map[string]any{"cancelled_dispatches": cancelled})
}

// SentFormingFeatureIDs lists the forming (idea or ready) features that carry the mark — the
// heartbeat sweep's scope (FR-2.4).
func SentFormingFeatureIDs(ctx context.Context, q Querier) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `
		SELECT f.id FROM features f JOIN feature_sends s ON s.feature_id = f.id
		WHERE f.state IN ('idea', 'ready') ORDER BY s.sent_at`)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

// DispatchesForRefSince returns a feature's dispatches of the given purposes
// queued at or after a moment, oldest first — what Withdraw inspects.
func DispatchesForRefSince(ctx context.Context, q Querier, refType string, refID uuid.UUID, purposes []string, since time.Time) ([]Dispatch, error) {
	rows, err := q.Query(ctx, `SELECT `+dispatchCols+` FROM dispatches
		WHERE ref_type = $1 AND ref_id = $2 AND purpose = ANY($3) AND queued_at >= $4
		ORDER BY queued_at`, refType, refID, purposes, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dispatch
	for rows.Next() {
		d, err := scanDispatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// CancelQueuedDispatch closes a dispatch that has not started. It reports
// false when the dispatch had already left the queue.
func CancelQueuedDispatch(ctx context.Context, tx pgx.Tx, id uuid.UUID, actor, why string) (bool, error) {
	var refType string
	var refID uuid.UUID
	err := tx.QueryRow(ctx, `
		UPDATE dispatches SET state = 'cancelled', finished_at = now()
		WHERE id = $1 AND state = 'queued' RETURNING ref_type, ref_id`, id).Scan(&refType, &refID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, Audit(ctx, tx, actor, "dispatch.cancelled", refType, &refID,
		map[string]any{"dispatch_id": id.String(), "why": why})
}

// LatestDispatchFailedForRef reports whether the most recent dispatch of a
// purpose for a ref has failed. A failed dispatch is still being dealt with:
// the retry sweep re-queues it while attempts remain, and a dispatch-failure
// checkpoint governs it after that. Either way it is the same row that runs
// again, so a new one must not be enqueued beside it.
func LatestDispatchFailedForRef(ctx context.Context, q Querier, refType string, refID uuid.UUID, purpose string) (bool, error) {
	var failed bool
	err := q.QueryRow(ctx, `SELECT COALESCE((SELECT state = 'failed' FROM dispatches
		WHERE ref_type = $1 AND ref_id = $2 AND purpose = $3
		ORDER BY queued_at DESC, id DESC LIMIT 1), false)`,
		refType, refID, purpose).Scan(&failed)
	return failed, err
}

// LiveDispatchForRef reports whether a dispatch of a purpose is queued or
// running for a ref.
func LiveDispatchForRef(ctx context.Context, q Querier, refType string, refID uuid.UUID, purpose string) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM dispatches
		WHERE ref_type = $1 AND ref_id = $2 AND purpose = $3 AND state IN ('queued','running'))`,
		refType, refID, purpose).Scan(&ok)
	return ok, err
}

// CountRunningDispatches is how many agent slots are in use now.
func CountRunningDispatches(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE state = 'running'`).Scan(&n)
	return n, err
}

// PurposeTokenSamples returns the total tokens (input + output) of the most
// recent successful dispatches of a purpose, newest first — the send screen's
// forecast input (SD-12).
func PurposeTokenSamples(ctx context.Context, q Querier, purpose string, limit int) ([]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) FROM dispatches
		WHERE purpose = $1 AND state = 'succeeded' AND input_tokens IS NOT NULL
		ORDER BY finished_at DESC LIMIT $2`, purpose, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var n int64
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Median of a sample; zero for an empty one.
func Median(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// ---- Held documents (FR-5) ----

// DocumentHold says a document in reviewing waits for a person. DispatchID
// is the agent review whose approval is held; nil when no agent has given a
// verdict (agent review switched off).
type DocumentHold struct {
	DocumentID uuid.UUID
	DispatchID *uuid.UUID
	CreatedAt  time.Time
}

// GetDocumentHold returns a document's hold, or ErrNotFound.
func GetDocumentHold(ctx context.Context, q Querier, docID uuid.UUID) (*DocumentHold, error) {
	var h DocumentHold
	err := q.QueryRow(ctx, `SELECT document_id, dispatch_id, created_at FROM document_holds WHERE document_id = $1`, docID).
		Scan(&h.DocumentID, &h.DispatchID, &h.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &h, err
}

// SetDocumentHold holds a document, replacing any earlier hold, and audits it.
func SetDocumentHold(ctx context.Context, tx pgx.Tx, docID uuid.UUID, dispatchID *uuid.UUID, actor, why string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO document_holds (document_id, dispatch_id) VALUES ($1, $2)
		ON CONFLICT (document_id) DO UPDATE SET dispatch_id = EXCLUDED.dispatch_id, created_at = now()`,
		docID, dispatchID); err != nil {
		return err
	}
	payload := map[string]any{"why": why}
	if dispatchID != nil {
		payload["dispatch_id"] = dispatchID.String()
	}
	return Audit(ctx, tx, actor, "document.held", "document", &docID, payload)
}

// ClearDocumentHold removes a hold, if there is one.
func ClearDocumentHold(ctx context.Context, tx pgx.Tx, docID uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM document_holds WHERE document_id = $1`, docID)
	return err
}

// ---- Human issues (FR-6) ----

// InsertIssue records a human issue on a document and returns its id.
func InsertIssue(ctx context.Context, tx pgx.Tx, docID uuid.UUID, author, sectionRef, body, via, quote string) (uuid.UUID, error) {
	id := NewID()
	_, err := tx.Exec(ctx, `
		INSERT INTO document_comments (id, document_id, author, section_ref, body, is_issue, via, quote)
		VALUES ($1, $2, $3, $4, $5, true, $6, $7)`,
		id, docID, author, nullable(sectionRef), body, via, nullable(quote))
	if err != nil {
		return uuid.Nil, err
	}
	payload := map[string]any{"issue_id": id.String(), "via": via}
	if quote != "" {
		payload["quote"] = quote
	}
	return id, Audit(ctx, tx, author, "document.issue_raised", "document", &docID, payload)
}

// OpenIssues returns a document's unresolved human issues, oldest first.
func OpenIssues(ctx context.Context, q Querier, docID uuid.UUID) ([]Comment, error) {
	rows, err := q.Query(ctx, `SELECT `+commentCols+` FROM document_comments
		WHERE document_id = $1 AND is_issue AND NOT resolved ORDER BY created_at`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		if err := scanComment(rows, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddressIssue resolves one issue with the reviewer's answer (SD-6).
func AddressIssue(ctx context.Context, tx pgx.Tx, issueID uuid.UUID, by string, dispatchID *uuid.UUID, status, note string) error {
	var docID uuid.UUID
	err := tx.QueryRow(ctx, `
		UPDATE document_comments
		SET resolved = true, addressed_by = $2, addressed_dispatch_id = $3,
		    addressed_note = $4, addressed_at = now()
		WHERE id = $1 AND is_issue AND NOT resolved RETURNING document_id`,
		issueID, by, dispatchID, note).Scan(&docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already answered: a replayed outcome is harmless
	}
	if err != nil {
		return err
	}
	return Audit(ctx, tx, by, "document.issue_addressed", "document", &docID,
		map[string]any{"issue_id": issueID.String(), "status": status, "note": note})
}

// SettleIssuesByApproval resolves every open issue on a document because a
// person approved it directly (SD-6): the approver's decision is the answer.
func SettleIssuesByApproval(ctx context.Context, tx pgx.Tx, docID uuid.UUID, actor string) (int, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE document_comments
		SET resolved = true, addressed_by = $2, addressed_at = now(),
		    addressed_note = 'Settled when ' || $2 || ' approved the document directly.'
		WHERE document_id = $1 AND is_issue AND NOT resolved`, docID, actor)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// DetachDocument removes a draft's registration — its row, sections,
// comments and hold — leaving the file on disk (FR-9.3). The state check is
// in the statement so a document that left draft in the meantime is refused.
func DetachDocument(ctx context.Context, tx pgx.Tx, d *Document, actor string) error {
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM documents WHERE id = $1 FOR UPDATE`, d.ID).Scan(&state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if state != "draft" {
		return errors.New("only a draft can be detached")
	}
	// Open issues go with the registration, so their words go on the audit
	// row: an issue is never dropped silently (SPEC-011 FR-6.8).
	open, err := OpenIssues(ctx, tx, d.ID)
	if err != nil {
		return err
	}
	var dropped []map[string]any
	for _, c := range open {
		dropped = append(dropped, map[string]any{"author": c.Author, "body": c.Body, "via": c.Via, "quote": c.Quote})
	}
	for _, q := range []string{
		`DELETE FROM document_holds WHERE document_id = $1`,
		`DELETE FROM document_comments WHERE document_id = $1`,
		`DELETE FROM document_sections WHERE document_id = $1`,
		`DELETE FROM documents WHERE id = $1`,
	} {
		if _, err := tx.Exec(ctx, q, d.ID); err != nil {
			return err
		}
	}
	payload := map[string]any{"path": d.Path, "type": d.Type, "owner_type": d.OwnerType}
	if len(dropped) > 0 {
		payload["open_issues"] = dropped
	}
	if d.OwnerID != nil {
		payload["owner_id"] = d.OwnerID.String()
	}
	return Audit(ctx, tx, actor, "document.detached", "document", &d.ID, payload)
}
