package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
)

// Bugs (SPEC-019). A bug is a features row with kind 'bug' (SD-1); the facts
// only a bug has — who reported it, where it came from, and its triage — live
// in the bugs table beside it. Each mutation takes the caller's transaction
// and writes its audit row inside it (O-3).

// Triage states (SPEC-019 FR-2.1).
const (
	TriageReported  = "reported"
	TriageAccepted  = "accepted"
	TriageRejected  = "rejected"
	TriageDuplicate = "duplicate"
)

// Reporter kinds (FR-1.1).
const (
	ReporterPerson = "person"
	ReporterChat   = "chat"
	ReporterAgent  = "agent"
	ReporterReview = "review"
)

// Bug is a bug's row in the bugs table, with its features row.
type Bug struct {
	Feature            Feature
	OriginFeatureID    *uuid.UUID
	Triage             string
	ReporterKind       string
	ReportedBy         string
	ReportedDispatchID *uuid.UUID
	SourceTaskID       *uuid.UUID
	ReportedAt         time.Time
	DuplicateOf        *uuid.UUID
	DecidedBy          string
	DecidedVia         string
	DecidedQuote       string
	DecidedReason      string
	DecidedAt          *time.Time
}

// NewBug is what CreateBug needs.
type NewBug struct {
	InitiativeID       uuid.UUID
	OriginFeatureID    *uuid.UUID
	Title              string
	Description        string
	ReporterKind       string
	ReportedBy         string
	ReportedDispatchID *uuid.UUID
	SourceTaskID       *uuid.UUID
	// Via is the channel for the audit row: ui, mcp, agent or review.
	Via string
}

// CreateBug inserts a bug's features row, with an ID from the BUG sequence
// and a slug made from that ID, then its bugs row and a bug.reported audit
// row (FR-1.3). The feature.created row is written too, so everything that
// reads a feature's history reads a bug's.
func CreateBug(ctx context.Context, tx pgx.Tx, nb NewBug) (*Feature, error) {
	f := &Feature{ID: NewID(), InitiativeID: nb.InitiativeID, Name: nb.Title, Description: nb.Description,
		State: lifecycle.FeatIdea, Kind: KindBug}
	err := tx.QueryRow(ctx, `
		WITH m AS (SELECT mint_ident('BUG') AS pid)
		INSERT INTO features (id, initiative_id, slug, name, description, kind, public_id)
		SELECT $1, $2, lower(m.pid), $3, $4, 'bug', m.pid FROM m
		RETURNING public_id, slug, created_at`,
		f.ID, nb.InitiativeID, nb.Title, nb.Description).Scan(&f.PublicID, &f.Slug, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bugs (feature_id, origin_feature_id, reporter_kind, reported_by, reported_dispatch_id, source_task_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		f.ID, nb.OriginFeatureID, nb.ReporterKind, nb.ReportedBy, nb.ReportedDispatchID, nb.SourceTaskID); err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, nb.ReportedBy, "feature.created", "feature", &f.ID,
		map[string]any{"slug": f.Slug, "name": f.Name, "initiative_id": nb.InitiativeID.String(),
			"public_id": f.PublicID, "kind": KindBug}); err != nil {
		return nil, err
	}
	payload := map[string]any{"public_id": f.PublicID, "reporter_kind": nb.ReporterKind, "via": nb.Via}
	if nb.OriginFeatureID != nil {
		payload["origin_feature_id"] = nb.OriginFeatureID.String()
	}
	if nb.ReportedDispatchID != nil {
		payload["dispatch_id"] = nb.ReportedDispatchID.String()
	}
	if nb.SourceTaskID != nil {
		payload["task_id"] = nb.SourceTaskID.String()
	}
	if err := Audit(ctx, tx, nb.ReportedBy, "bug.reported", "feature", &f.ID, payload); err != nil {
		return nil, err
	}
	return f, nil
}

const bugCols = `b.origin_feature_id, b.triage, b.reporter_kind, b.reported_by, b.reported_dispatch_id,
	b.source_task_id, b.reported_at, b.duplicate_of, coalesce(b.decided_by, ''), coalesce(b.decided_via, ''),
	coalesce(b.decided_quote, ''), coalesce(b.decided_reason, ''), b.decided_at`

func featureColsAs(alias string) string {
	cols := strings.Split(FeatureCols, ", ")
	for i := range cols {
		cols[i] = alias + "." + cols[i]
	}
	return strings.Join(cols, ", ")
}

func scanBug(row pgx.Row) (*Bug, error) {
	var b Bug
	f := &b.Feature
	err := row.Scan(&f.ID, &f.InitiativeID, &f.Slug, &f.Name, &f.Description, &f.State, &f.CreatedAt,
		&f.PublicID, &f.LegacyDocPaths, &f.Kind,
		&b.OriginFeatureID, &b.Triage, &b.ReporterKind, &b.ReportedBy, &b.ReportedDispatchID,
		&b.SourceTaskID, &b.ReportedAt, &b.DuplicateOf, &b.DecidedBy, &b.DecidedVia,
		&b.DecidedQuote, &b.DecidedReason, &b.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

var bugSelect = `SELECT ` + featureColsAs("f") + `, ` + bugCols + `
	FROM features f JOIN bugs b ON b.feature_id = f.id`

// GetBug returns a bug by its features row id, or ErrNotFound when the row
// isn't a bug.
func GetBug(ctx context.Context, q Querier, id uuid.UUID) (*Bug, error) {
	return scanBug(q.QueryRow(ctx, bugSelect+` WHERE f.id = $1`, id))
}

// GetBugForUpdate locks the bug's row for a triage decision.
func GetBugForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Bug, error) {
	return scanBug(tx.QueryRow(ctx, bugSelect+` WHERE f.id = $1 FOR UPDATE OF b`, id))
}

func queryBugs(ctx context.Context, q Querier, where, order string, args ...any) ([]Bug, error) {
	rows, err := q.Query(ctx, bugSelect+` WHERE `+where+` ORDER BY `+order, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bug
	for rows.Next() {
		b, err := scanBug(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// TriageQueue lists the bugs waiting for triage, oldest first (FR-2.3).
func TriageQueue(ctx context.Context, q Querier) ([]Bug, error) {
	return queryBugs(ctx, q, `b.triage = 'reported' AND f.state = 'idea'`, `b.reported_at, f.id`)
}

// RecentlyTriaged lists the latest triage decisions, newest first.
func RecentlyTriaged(ctx context.Context, q Querier, limit int) ([]Bug, error) {
	return queryBugs(ctx, q, `b.triage <> 'reported'`, `b.decided_at DESC, f.id LIMIT $1`, limit)
}

// CountReportedBugs is the triage queue's length (FR-2.4).
func CountReportedBugs(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM bugs b JOIN features f ON f.id = b.feature_id
		WHERE b.triage = 'reported' AND f.state = 'idea'`).Scan(&n)
	return n, err
}

// OpenBugsOnInitiative lists the open bugs hanging off an initiative with no
// origin feature (FR-5.2): not rejected, not a duplicate, not finished.
func OpenBugsOnInitiative(ctx context.Context, q Querier, initiativeID uuid.UUID) ([]Bug, error) {
	return queryBugs(ctx, q, `f.initiative_id = $1 AND b.origin_feature_id IS NULL
		AND b.triage IN ('reported', 'accepted') AND f.state NOT IN ('done', 'abandoned')`,
		`f.public_id`, initiativeID)
}

// OpenBugsOnFeature lists the open bugs reported on a feature (FR-5.2).
func OpenBugsOnFeature(ctx context.Context, q Querier, featureID uuid.UUID) ([]Bug, error) {
	return queryBugs(ctx, q, `b.origin_feature_id = $1
		AND b.triage IN ('reported', 'accepted') AND f.state NOT IN ('done', 'abandoned')`,
		`f.public_id`, featureID)
}

// BugFilter narrows ListBugs (FR-6.2).
type BugFilter struct {
	// Triage is a triage state, "open" (reported or accepted and not
	// finished), or "" for all.
	Triage       string
	InitiativeID *uuid.UUID
	FeatureID    *uuid.UUID
}

// ListBugs lists bugs by filter, by ID.
func ListBugs(ctx context.Context, q Querier, bf BugFilter) ([]Bug, error) {
	where := []string{"true"}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	switch bf.Triage {
	case "":
	case "open":
		where = append(where, `b.triage IN ('reported', 'accepted') AND f.state NOT IN ('done', 'abandoned')`)
	default:
		where = append(where, "b.triage = "+arg(bf.Triage))
	}
	if bf.InitiativeID != nil {
		where = append(where, "f.initiative_id = "+arg(*bf.InitiativeID))
	}
	if bf.FeatureID != nil {
		where = append(where, "b.origin_feature_id = "+arg(*bf.FeatureID))
	}
	return queryBugs(ctx, q, strings.Join(where, " AND "), `f.public_id`, args...)
}

// OpenReportedBugByTitle finds a reported bug on the same origin with the same
// title, ignoring case and surrounding space: an agent's report of it is a
// duplicate (SPEC-019 SD-10).
func OpenReportedBugByTitle(ctx context.Context, q Querier, originFeatureID uuid.UUID, title string) (*Bug, error) {
	return scanBug(q.QueryRow(ctx, bugSelect+`
		WHERE b.origin_feature_id = $1 AND b.triage = 'reported' AND lower(btrim(f.name)) = lower(btrim($2))
		ORDER BY f.public_id LIMIT 1`, originFeatureID, title))
}

// CountBugsByDispatch is how many reports one run has filed (SD-10).
func CountBugsByDispatch(ctx context.Context, q Querier, dispatchID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM bugs WHERE reported_dispatch_id = $1 AND reporter_kind = 'agent'`,
		dispatchID).Scan(&n)
	return n, err
}

// TriageDecision is a person's decision on a reported bug (FR-2.2).
type TriageDecision struct {
	Decision    string // accepted | rejected | duplicate
	Reason      string
	DuplicateOf *uuid.UUID
	Actor       string
	Via         string // ui | mcp
	Quote       string
}

// RecordTriage writes a decision on a bug locked by GetBugForUpdate, and its
// bug.triaged audit row. The caller has checked the decision; the table's
// constraints check it again.
func RecordTriage(ctx context.Context, tx pgx.Tx, b *Bug, d TriageDecision) error {
	var quote, reason *string
	if d.Quote != "" {
		quote = &d.Quote
	}
	if strings.TrimSpace(d.Reason) != "" {
		r := strings.TrimSpace(d.Reason)
		reason = &r
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bugs SET triage = $2, duplicate_of = $3, decided_by = $4, decided_via = $5,
		       decided_quote = $6, decided_reason = $7, decided_at = now()
		WHERE feature_id = $1 AND triage = 'reported'`,
		b.Feature.ID, d.Decision, d.DuplicateOf, d.Actor, d.Via, quote, reason); err != nil {
		return err
	}
	payload := map[string]any{"decision": d.Decision, "via": d.Via}
	if quote != nil {
		// A relayed decision is the person's, carried by the chat agent
		// (DEC-006 Amendment 1), as a relayed verdict is.
		payload["quote"] = *quote
		payload["verdict_by"] = "person"
	}
	if reason != nil {
		payload["reason"] = *reason
	}
	if d.DuplicateOf != nil {
		payload["duplicate_of"] = d.DuplicateOf.String()
	}
	return Audit(ctx, tx, d.Actor, "bug.triaged", "feature", &b.Feature.ID, payload)
}

// ErrNotABug is returned when an id names a feature that isn't a bug.
var ErrNotABug = errors.New("not a bug")

// MinorFindingSet is one approving code review's minor findings on a task, as
// its task.review_minor_findings audit row recorded them.
type MinorFindingSet struct {
	TaskPublicID string
	TaskTitle    string
	Reviewer     string
	DispatchID   *uuid.UUID
	Comments     []struct {
		SectionRef string `json:"section_ref"`
		Body       string `json:"body"`
	}
}

// MinorFindingsForFeature reads every approving code review's minor findings
// across a feature's tasks, in the order they were raised (SPEC-019 SD-9).
func MinorFindingsForFeature(ctx context.Context, q Querier, featureID uuid.UUID) ([]MinorFindingSet, error) {
	rows, err := q.Query(ctx, `
		SELECT t.public_id, t.title, ae.actor, ae.payload
		FROM audit_events ae JOIN tasks t ON ae.ref_type = 'task' AND ae.ref_id = t.id
		WHERE ae.kind = 'task.review_minor_findings' AND t.feature_id = $1
		ORDER BY ae.occurred_at, ae.id`, featureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MinorFindingSet
	for rows.Next() {
		var m MinorFindingSet
		var raw []byte
		if err := rows.Scan(&m.TaskPublicID, &m.TaskTitle, &m.Reviewer, &raw); err != nil {
			return nil, err
		}
		var p struct {
			DispatchID string          `json:"dispatch_id"`
			Comments   json.RawMessage `json:"comments"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		_ = json.Unmarshal(p.Comments, &m.Comments)
		if id, err := uuid.Parse(p.DispatchID); err == nil {
			m.DispatchID = &id
		}
		if len(m.Comments) > 0 {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

// LatestReviewRunForFeature is the newest code review run on any of a
// feature's tasks: the run a bug of their findings names when an older audit
// row didn't record its own.
func LatestReviewRunForFeature(ctx context.Context, q Querier, featureID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `
		SELECT d.id FROM dispatches d JOIN tasks t ON d.ref_type = 'task' AND d.ref_id = t.id
		WHERE t.feature_id = $1 AND d.purpose = 'review-code' AND d.state = 'succeeded'
		ORDER BY d.finished_at DESC NULLS LAST, d.id DESC LIMIT 1`, featureID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}
