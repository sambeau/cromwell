package store

// Who wrote each document, and who gave each verdict (SPEC-017 FR-2,
// migration 0011). Rows are written in the same transaction as the act they
// record (O-3), and read back to say it in a sentence.

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Writer kinds (SPEC-017 FR-2.1).
const (
	WriterAgent  = "agent"
	WriterChat   = "chat"
	WriterPerson = "person"
	WriterSystem = "system"
)

// Writing acts.
const (
	ActWrote          = "wrote"
	ActRevised        = "revised"
	ActAdded          = "added"
	ActStarted        = "started"
	ActOpenedRevision = "opened_revision"
)

// Writer is one writing act Subutai saw.
type Writer struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	Act        string
	Kind       string
	Actor      string
	Model      string
	DispatchID *uuid.UUID
	Via        string
	Inferred   bool
	At         time.Time
}

// RecordWriter writes one writing act.
func RecordWriter(ctx context.Context, tx pgx.Tx, w Writer) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO document_writers (id, document_id, act, writer_kind, actor, model, dispatch_id, via)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''))`,
		NewID(), w.DocumentID, w.Act, w.Kind, w.Actor, w.Model, w.DispatchID, w.Via)
	return err
}

// HasWriter reports whether a document has any writing act recorded.
func HasWriter(ctx context.Context, q Querier, docID uuid.UUID) (bool, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM document_writers WHERE document_id = $1`, docID).Scan(&n)
	return n > 0, err
}

const writerCols = `id, document_id, act, writer_kind, actor, COALESCE(model, ''), dispatch_id,
	COALESCE(via, ''), inferred, at`

func scanWriter(row pgx.Row) (Writer, error) {
	var w Writer
	err := row.Scan(&w.ID, &w.DocumentID, &w.Act, &w.Kind, &w.Actor, &w.Model, &w.DispatchID,
		&w.Via, &w.Inferred, &w.At)
	return w, err
}

// Writers returns a document's writing acts, oldest first.
func Writers(ctx context.Context, q Querier, docID uuid.UUID) ([]Writer, error) {
	rows, err := q.Query(ctx, `SELECT `+writerCols+` FROM document_writers
		WHERE document_id = $1 ORDER BY at, id`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Writer
	for rows.Next() {
		w, err := scanWriter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Verdict is one decision on a document (SPEC-017 SD-5, SD-6).
type Verdict struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	Verdict    string // approve | send_back
	Held       bool
	Kind       string // agent | person
	Actor      string
	Model      string
	DispatchID *uuid.UUID
	Via        string // ui | escalation | mcp, or "" for an agent
	Quote      string
	// ReleasedBy, ReleasedVia and ReleasedQuote say who let the reviewer
	// decide on a held spec.
	ReleasedBy    string
	ReleasedVia   string
	ReleasedQuote string
	Inferred      bool
	At            time.Time
}

// Verdict words.
const (
	VerdictApprove  = "approve"
	VerdictSendBack = "send_back"
)

// Verdict givers.
const (
	GiverAgent  = "agent"
	GiverPerson = "person"
)

// RecordVerdict writes one verdict.
func RecordVerdict(ctx context.Context, tx pgx.Tx, v Verdict) error {
	if v.Via == "mcp" && v.Quote == "" {
		return errors.New("a relayed verdict must carry the person's words")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO document_verdicts (id, document_id, verdict, held, giver_kind, actor, model, dispatch_id,
		                               via, quote, released_by, released_via, released_quote)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, NULLIF($9, ''), NULLIF($10, ''),
		        NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''))`,
		NewID(), v.DocumentID, v.Verdict, v.Held, v.Kind, v.Actor, v.Model, v.DispatchID,
		v.Via, v.Quote, v.ReleasedBy, v.ReleasedVia, v.ReleasedQuote)
	return err
}

const verdictCols = `id, document_id, verdict, held, giver_kind, actor, COALESCE(model, ''), dispatch_id,
	COALESCE(via, ''), COALESCE(quote, ''), COALESCE(released_by, ''), COALESCE(released_via, ''),
	COALESCE(released_quote, ''), inferred, at`

func scanVerdict(row pgx.Row) (Verdict, error) {
	var v Verdict
	err := row.Scan(&v.ID, &v.DocumentID, &v.Verdict, &v.Held, &v.Kind, &v.Actor, &v.Model, &v.DispatchID,
		&v.Via, &v.Quote, &v.ReleasedBy, &v.ReleasedVia, &v.ReleasedQuote, &v.Inferred, &v.At)
	return v, err
}

// Verdicts returns a document's verdicts, oldest first.
func Verdicts(ctx context.Context, q Querier, docID uuid.UUID) ([]Verdict, error) {
	rows, err := q.Query(ctx, `SELECT `+verdictCols+` FROM document_verdicts
		WHERE document_id = $1 ORDER BY at, id`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Verdict
	for rows.Next() {
		v, err := scanVerdict(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// LastVerdict returns a document's latest verdict, or ErrNotFound.
func LastVerdict(ctx context.Context, q Querier, docID uuid.UUID) (*Verdict, error) {
	v, err := scanVerdict(q.QueryRow(ctx, `SELECT `+verdictCols+` FROM document_verdicts
		WHERE document_id = $1 ORDER BY at DESC, id DESC LIMIT 1`, docID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// VerdictForRun is the latest verdict recorded against a review run: the
// agent's own, held or applied, or a person's ruling on its escalation.
func VerdictForRun(ctx context.Context, q Querier, dispatchID uuid.UUID) (*Verdict, error) {
	v, err := scanVerdict(q.QueryRow(ctx, `SELECT `+verdictCols+` FROM document_verdicts
		WHERE dispatch_id = $1 ORDER BY at DESC, id DESC LIMIT 1`, dispatchID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}
