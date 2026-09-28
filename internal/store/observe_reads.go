package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Reads behind the feature timeline and the lists of agent runs (SPEC-012
// FR-4, FR-5). None of them selects cost_usd: the human surface counts tokens
// only (D-4, SD-12).

// HistoryEvent is an audit row about a feature, one of its documents or one
// of its tasks, with the document's type or the task's title alongside.
type HistoryEvent struct {
	AuditEvent
	DocType string
	Label   string
	Path    string // the document's repository path, for its page link
}

// RunSummary is an agent run as the timeline and the run lists show it.
type RunSummary struct {
	ID           uuid.UUID
	State        string
	Purpose      string
	Role         string
	Model        string
	RefType      string
	RefID        uuid.UUID
	Label        string // the title of the document or task the run was about
	Path         string // the document's repository path, when it was about one
	Attempt      int
	QueuedAt     time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
	InputTokens  int64
	OutputTokens int64
	Verdict      string
	Error        string
}

// Tokens is everything the run consumed, cache reads and writes included in
// the input side.
func (r RunSummary) Tokens() int64 { return r.InputTokens + r.OutputTokens }

// featureRefs lists the feature itself, every document it owns (superseded
// revisions too) and every task it has, with their display names (FR-5.1).
const featureRefs = `
	WITH refs AS (
		SELECT 'feature'::ref_type AS rt, f.id AS rid, NULL::text AS doc_type, f.name AS label, NULL::text AS path
		FROM features f WHERE f.id = $1
		UNION ALL
		SELECT 'document'::ref_type, d.id, d.type::text, d.title, d.path
		FROM documents d WHERE d.owner_type = 'feature' AND d.owner_id = $1
		UNION ALL
		SELECT 'task'::ref_type, t.id, NULL::text, t.title, NULL::text
		FROM tasks t WHERE t.feature_id = $1
	)`

// FeatureHistory returns every audit event about a feature, its documents and
// its tasks, in the order they happened.
func FeatureHistory(ctx context.Context, q Querier, featureID uuid.UUID) ([]HistoryEvent, error) {
	rows, err := q.Query(ctx, featureRefs+`
		SELECT e.id, e.occurred_at, e.actor, e.kind, e.ref_type, e.ref_id, e.payload,
		       COALESCE(r.doc_type, ''), COALESCE(r.label, ''), COALESCE(r.path, '')
		FROM audit_events e JOIN refs r ON e.ref_type = r.rt AND e.ref_id = r.rid
		ORDER BY e.occurred_at, e.id`, featureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryEvent
	for rows.Next() {
		var h HistoryEvent
		if err := rows.Scan(&h.ID, &h.OccurredAt, &h.Actor, &h.Kind, &h.RefType, &h.RefID,
			&h.Payload, &h.DocType, &h.Label, &h.Path); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

const runSummaryCols = `d.id, d.state, d.purpose, d.role, d.model, d.ref_type, d.ref_id,
	d.attempt, d.queued_at, d.started_at, d.finished_at,
	COALESCE(d.input_tokens, 0) + COALESCE(d.cache_read_tokens, 0) + COALESCE(d.cache_write_tokens, 0),
	COALESCE(d.output_tokens, 0),
	COALESCE(d.outcome->>'verdict', ''), COALESCE(d.error, '')`

func scanRunSummaries(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}, withLabel bool) ([]RunSummary, error) {
	var out []RunSummary
	for rows.Next() {
		var r RunSummary
		dest := []any{&r.ID, &r.State, &r.Purpose, &r.Role, &r.Model, &r.RefType, &r.RefID,
			&r.Attempt, &r.QueuedAt, &r.StartedAt, &r.FinishedAt, &r.InputTokens, &r.OutputTokens,
			&r.Verdict, &r.Error}
		if withLabel {
			dest = append(dest, &r.Label, &r.Path)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FeatureRuns returns every run on a feature, its documents and its tasks,
// oldest first.
func FeatureRuns(ctx context.Context, q Querier, featureID uuid.UUID) ([]RunSummary, error) {
	rows, err := q.Query(ctx, featureRefs+`
		SELECT `+runSummaryCols+`, COALESCE(r.label, ''), COALESCE(r.path, '')
		FROM dispatches d JOIN refs r ON d.ref_type = r.rt AND d.ref_id = r.rid
		ORDER BY d.queued_at, d.id`, featureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunSummaries(rows, true)
}

// RunsForRef returns every run on one entity, oldest first (FR-4.1, FR-4.2).
func RunsForRef(ctx context.Context, q Querier, refType string, refID uuid.UUID) ([]RunSummary, error) {
	rows, err := q.Query(ctx, `SELECT `+runSummaryCols+` FROM dispatches d
		WHERE d.ref_type = $1 AND d.ref_id = $2 ORDER BY d.queued_at, d.id`, refType, refID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunSummaries(rows, false)
}

// ReviewRun is one agent verdict, as review health reads it (FR-7.1).
type ReviewRun struct {
	ID         uuid.UUID
	Purpose    string
	Role       string
	Model      string
	RefType    string
	RefID      uuid.UUID
	StartedAt  *time.Time
	FinishedAt *time.Time
	Tokens     int64
	Output     int64
	Outcome    json.RawMessage
}

// ReviewRuns returns the succeeded review and verification runs whose outcome
// carries a verdict, finished since the given time (zero means all time).
func ReviewRuns(ctx context.Context, q Querier, since time.Time) ([]ReviewRun, error) {
	rows, err := q.Query(ctx, `
		SELECT id, purpose, role, model, ref_type, ref_id, started_at, finished_at,
		       COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) +
		       COALESCE(cache_read_tokens, 0) + COALESCE(cache_write_tokens, 0),
		       COALESCE(output_tokens, 0), outcome
		FROM dispatches
		WHERE state = 'succeeded'
		  AND (purpose LIKE 'review-%' OR purpose = 'verify-feature')
		  AND outcome ? 'verdict'
		  AND finished_at >= $1
		ORDER BY finished_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewRun
	for rows.Next() {
		var r ReviewRun
		if err := rows.Scan(&r.ID, &r.Purpose, &r.Role, &r.Model, &r.RefType, &r.RefID,
			&r.StartedAt, &r.FinishedAt, &r.Tokens, &r.Output, &r.Outcome); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
