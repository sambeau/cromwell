package store

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Transcripts (SPEC-012 FR-1): every agent run's conversation, one row per
// entry, appended as the run proceeds so a run that dies part-way keeps what
// it did (SD-1). Entries are keyed by run and attempt, because a retry reuses
// the dispatches row (SD-2).

// Transcript entry kinds (FR-1.1).
const (
	EntrySystem     = "system"
	EntryPrompt     = "prompt"
	EntryTurn       = "turn"
	EntryText       = "text"
	EntryToolCall   = "tool_call"
	EntryToolResult = "tool_result"
	EntryNudge      = "nudge"
	EntryOutcome    = "outcome"
	EntryError      = "error"
)

// TranscriptEntry is one row of a transcript.
type TranscriptEntry struct {
	ID           uuid.UUID
	DispatchID   uuid.UUID
	Attempt      int
	Seq          int
	Turn         int
	Kind         string
	ToolName     string
	ToolUseID    string
	Content      string
	ContentBytes int
	Truncated    bool
	IsError      bool
	LatencyMs    *int
	InputTokens  *int64
	OutputTokens *int64
	CacheRead    *int64
	CacheWrite   *int64
	CreatedAt    time.Time
}

const transcriptCols = `id, dispatch_id, attempt, seq, turn, kind, COALESCE(tool_name, ''),
	COALESCE(tool_use_id, ''), content, content_bytes, truncated, is_error, latency_ms,
	input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, created_at`

const insertTranscriptSQL = `
	INSERT INTO transcript_entries (id, dispatch_id, attempt, seq, turn, kind, tool_name,
		tool_use_id, content, content_bytes, truncated, is_error, latency_ms,
		input_tokens, output_tokens, cache_read_tokens, cache_write_tokens)
	VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, $12, $13,
		$14, $15, $16, $17)`

func (e *TranscriptEntry) args() []any {
	if e.ID == uuid.Nil {
		e.ID = NewID()
	}
	return []any{e.ID, e.DispatchID, e.Attempt, e.Seq, e.Turn, e.Kind, e.ToolName,
		e.ToolUseID, CleanText(e.Content), e.ContentBytes, e.Truncated, e.IsError, e.LatencyMs,
		e.InputTokens, e.OutputTokens, e.CacheRead, e.CacheWrite}
}

// AppendTranscript writes a group of entries in one round trip (FR-1.6). The
// entries are independent inserts, not a transaction: each one that lands is
// kept even if a later one fails.
func (s *Store) AppendTranscript(ctx context.Context, entries []TranscriptEntry) error {
	if len(entries) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for i := range entries {
		b.Queue(insertTranscriptSQL, entries[i].args()...)
	}
	br := s.Pool.SendBatch(ctx, b)
	var first error
	for range entries {
		if _, err := br.Exec(); err != nil && first == nil {
			first = err
		}
	}
	if err := br.Close(); err != nil && first == nil {
		first = err
	}
	return first
}

// appendErrorEntry records why an attempt failed, in the transaction that
// marks it failed (SD-3). It runs inside a savepoint, so whatever happens to
// this write, it can't undo the failure being recorded (FR-1.9, NFR-4); the
// reason is on the audit row as well. The sequence number follows whatever
// the attempt already wrote.
func appendErrorEntry(ctx context.Context, tx pgx.Tx, dispatchID uuid.UUID, reason string) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return
	}
	var attempt, seq, turn int
	if err := sp.QueryRow(ctx, `
		SELECT d.attempt,
		       COALESCE((SELECT MAX(seq) FROM transcript_entries t WHERE t.dispatch_id = d.id AND t.attempt = d.attempt), 0) + 1,
		       COALESCE((SELECT MAX(turn) FROM transcript_entries t WHERE t.dispatch_id = d.id AND t.attempt = d.attempt), 0)
		FROM dispatches d WHERE d.id = $1`, dispatchID).Scan(&attempt, &seq, &turn); err != nil {
		_ = sp.Rollback(ctx)
		return
	}
	e := TranscriptEntry{
		DispatchID: dispatchID, Attempt: attempt, Seq: seq, Turn: turn,
		Kind: EntryError, Content: reason, ContentBytes: len(reason), IsError: true,
	}
	if _, err := sp.Exec(ctx, insertTranscriptSQL, e.args()...); err != nil {
		_ = sp.Rollback(ctx)
		return
	}
	_ = sp.Commit(ctx)
}

// Transcript returns one attempt's entries in order.
func Transcript(ctx context.Context, q Querier, dispatchID uuid.UUID, attempt int) ([]TranscriptEntry, error) {
	rows, err := q.Query(ctx, `SELECT `+transcriptCols+` FROM transcript_entries
		WHERE dispatch_id = $1 AND attempt = $2 ORDER BY seq, id`, dispatchID, attempt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TranscriptEntry
	for rows.Next() {
		var e TranscriptEntry
		if err := rows.Scan(&e.ID, &e.DispatchID, &e.Attempt, &e.Seq, &e.Turn, &e.Kind, &e.ToolName,
			&e.ToolUseID, &e.Content, &e.ContentBytes, &e.Truncated, &e.IsError, &e.LatencyMs,
			&e.InputTokens, &e.OutputTokens, &e.CacheRead, &e.CacheWrite, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// TranscriptAttempts lists the attempts of a run that have a transcript,
// lowest first.
func TranscriptAttempts(ctx context.Context, q Querier, dispatchID uuid.UUID) ([]int, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT attempt FROM transcript_entries
		WHERE dispatch_id = $1 ORDER BY attempt`, dispatchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var a int
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TranscriptTurns counts the model turns an attempt has recorded so far.
func TranscriptTurns(ctx context.Context, q Querier, dispatchID uuid.UUID, attempt int) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM transcript_entries
		WHERE dispatch_id = $1 AND attempt = $2 AND kind = 'turn'`, dispatchID, attempt).Scan(&n)
	return n, err
}

// TranscriptPrunedAt reports when retention removed a run's transcript, or nil.
func TranscriptPrunedAt(ctx context.Context, q Querier, dispatchID uuid.UUID) (*time.Time, error) {
	var t *time.Time
	err := q.QueryRow(ctx, `SELECT transcript_pruned_at FROM dispatches WHERE id = $1`, dispatchID).Scan(&t)
	return t, err
}

// PruneTranscripts deletes the transcripts of runs that finished more than
// days ago and marks those runs as pruned (FR-2.4). The runs' outcomes, tokens
// and tool ledger stay. days <= 0 keeps everything.
func (s *Store) PruneTranscripts(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	var n int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		tag, err := tx.Exec(ctx, `
			DELETE FROM transcript_entries t USING dispatches d
			WHERE t.dispatch_id = d.id
			  AND d.state IN ('succeeded', 'failed', 'cancelled')
			  AND d.finished_at < $1 AND d.transcript_pruned_at IS NULL`, cutoff)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		_, err = tx.Exec(ctx, `
			UPDATE dispatches SET transcript_pruned_at = now()
			WHERE state IN ('succeeded', 'failed', 'cancelled')
			  AND finished_at < $1 AND transcript_pruned_at IS NULL
			  AND started_at IS NOT NULL`, cutoff)
		return err
	})
	return n, err
}

// ToolCallRow is one row of the tool ledger, for runs recorded before
// transcripts were kept (FR-3.6).
type ToolCallRow struct {
	Seq         int
	Tool        string
	ArgBytes    int
	ResultBytes int
	LatencyMs   int
	Status      string
}

// ToolCallsFor returns a run's tool ledger in order.
func ToolCallsFor(ctx context.Context, q Querier, dispatchID uuid.UUID) ([]ToolCallRow, error) {
	rows, err := q.Query(ctx, `SELECT seq, tool, arg_bytes, result_bytes, latency_ms, status
		FROM tool_calls WHERE dispatch_id = $1 ORDER BY seq`, dispatchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ToolCallRow
	for rows.Next() {
		var r ToolCallRow
		if err := rows.Scan(&r.Seq, &r.Tool, &r.ArgBytes, &r.ResultBytes, &r.LatencyMs, &r.Status); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CleanText makes a string storable as Postgres text (FR-1.7): invalid UTF-8
// becomes U+FFFD, and so does NUL, which Postgres text cannot hold.
func CleanText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "�")
	}
	return s
}

// CutMarker is the text left where the middle of an over-long entry was cut.
func CutMarker(cut int) string {
	return fmt.Sprintf("\n[… %s bytes cut from the middle …]\n", groupThousands(cut))
}

// Cut shortens s to about limit bytes, keeping its first three quarters and
// its last quarter with a marker between them (SD-4, FR-2.2). Cuts fall on
// character boundaries. It reports whether anything was cut. limit <= 0
// leaves s whole.
func Cut(s string, limit int) (string, bool) {
	if limit <= 0 || len(s) <= limit {
		return s, false
	}
	head := limit * 3 / 4
	tail := limit - head
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	start := len(s) - tail
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[:head] + CutMarker(start-head) + s[start:], true
}

func groupThousands(n int) string {
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
