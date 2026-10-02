package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Spikes (SPEC-021). A spike is its own row (SD-1), not a feature: it has no
// spec, plan, tasks or merge, so nothing that works on features can reach it.
// Each mutation takes the caller's transaction and writes its audit row
// inside it (O-3); the conditional ones say in their result whether they
// changed anything, so a second call that finds the work done is not an
// error (FR-6.2).

// Spike states (SD-4).
const (
	SpikeIdea    = "idea"
	SpikeRunning = "running"
	SpikeEnded   = "ended"
	SpikeClosed  = "closed"
)

// How a run ended (FR-1.1).
const (
	SpikeConcluded = "concluded"
	SpikeBudget    = "budget"
	SpikeTurnLimit = "turn_limit"
	SpikeFailed    = "failed"
)

// How a person closed a spike (FR-7.2).
const (
	SpikeAnswered   = "answered"
	SpikeUnanswered = "unanswered"
)

// Sentinels for the conditional updates. Each says the spike wasn't in a state
// the move needs, which the service turns into a sentence.
var (
	ErrSpikeNotIdea     = errors.New("spike is not an idea")
	ErrSpikeNotRunning  = errors.New("spike is not running")
	ErrSpikeCannotClose = errors.New("spike cannot be closed")
)

// Spike is a row of the spikes table.
type Spike struct {
	ID           uuid.UUID
	PublicID     string
	InitiativeID uuid.UUID
	// FeatureID is the feature the spike was created on, or nil for one on an
	// initiative itself (SD-3).
	FeatureID *uuid.UUID
	Question  string
	State     string
	// BudgetOverride is the person's figure for this spike; TokenBudget is
	// what the run was actually given, set at the start (SD-6).
	BudgetOverride *int64
	TokenBudget    *int64
	TokensUsed     int64
	Draft          string
	DraftSavedAt   *time.Time
	EndedHow       string
	EndNote        string
	ClosedAs       string
	// FollowsID is the spike this one asks again for.
	FollowsID *uuid.UUID
	// BaseCommit and WorktreePath are set at the start; WorktreeRemovedAt
	// when the worktree was discarded.
	BaseCommit        string
	WorktreePath      string
	WorktreeRemovedAt *time.Time
	CreatedBy         string
	CreatedVia        string
	StartedBy         string
	StartedAt         *time.Time
	EndedAt           *time.Time
	ClosedBy          string
	ClosedAt          *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Owner is what the spike hangs off (SD-3): its feature when it has one, else
// its initiative. refType is the ref type of that owner, "feature" or
// "initiative".
func (s *Spike) Owner() (refType string, id uuid.UUID) {
	if s.FeatureID != nil {
		return "feature", *s.FeatureID
	}
	return "initiative", s.InitiativeID
}

// NewSpike is what CreateSpike needs.
type NewSpike struct {
	InitiativeID   uuid.UUID
	FeatureID      *uuid.UUID
	Question       string
	BudgetOverride *int64
	FollowsID      *uuid.UUID
	CreatedBy      string
	// CreatedVia is ui or mcp.
	CreatedVia string
}

const spikeCols = `s.id, s.public_id, s.initiative_id, s.feature_id, s.question, s.state,
	s.budget_override, s.token_budget, s.tokens_used, coalesce(s.draft, ''), s.draft_saved_at,
	coalesce(s.ended_how, ''), coalesce(s.end_note, ''), coalesce(s.closed_as, ''), s.follows_id,
	coalesce(s.base_commit, ''), coalesce(s.worktree_path, ''), s.worktree_removed_at,
	s.created_by, s.created_via, coalesce(s.started_by, ''), s.started_at, s.ended_at,
	coalesce(s.closed_by, ''), s.closed_at, s.created_at, s.updated_at`

func scanSpike(row pgx.Row) (*Spike, error) {
	var s Spike
	err := row.Scan(&s.ID, &s.PublicID, &s.InitiativeID, &s.FeatureID, &s.Question, &s.State,
		&s.BudgetOverride, &s.TokenBudget, &s.TokensUsed, &s.Draft, &s.DraftSavedAt,
		&s.EndedHow, &s.EndNote, &s.ClosedAs, &s.FollowsID,
		&s.BaseCommit, &s.WorktreePath, &s.WorktreeRemovedAt,
		&s.CreatedBy, &s.CreatedVia, &s.StartedBy, &s.StartedAt, &s.EndedAt,
		&s.ClosedBy, &s.ClosedAt, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateSpike inserts an idea spike, with an ID from the SPK sequence, and its
// spike.created audit row (FR-1.3). The caller has checked the owner and the
// question; the table's checks look again.
func CreateSpike(ctx context.Context, tx pgx.Tx, ns NewSpike) (*Spike, error) {
	id := NewID()
	s, err := scanSpike(tx.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO spikes (id, initiative_id, feature_id, question, budget_override, follows_id,
			                    created_by, created_via)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING *)
		SELECT `+spikeCols+` FROM ins s`,
		id, ns.InitiativeID, ns.FeatureID, ns.Question, ns.BudgetOverride, ns.FollowsID,
		ns.CreatedBy, ns.CreatedVia))
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"public_id": s.PublicID, "initiative_id": ns.InitiativeID.String(),
		"question": s.Question, "via": ns.CreatedVia}
	if ns.FeatureID != nil {
		payload["feature_id"] = ns.FeatureID.String()
	}
	if ns.BudgetOverride != nil {
		payload["budget_override"] = *ns.BudgetOverride
	}
	if ns.FollowsID != nil {
		payload["follows_id"] = ns.FollowsID.String()
	}
	if err := Audit(ctx, tx, ns.CreatedBy, "spike.created", "spike", &s.ID, payload); err != nil {
		return nil, err
	}
	return s, nil
}

// GetSpike returns a spike by row id, or ErrNotFound.
func GetSpike(ctx context.Context, q Querier, id uuid.UUID) (*Spike, error) {
	return scanSpike(q.QueryRow(ctx, `SELECT `+spikeCols+` FROM spikes s WHERE s.id = $1`, id))
}

// SpikeByPublicID returns a spike by its ID, "SPK-003", or ErrNotFound.
func SpikeByPublicID(ctx context.Context, q Querier, publicID string) (*Spike, error) {
	return scanSpike(q.QueryRow(ctx, `SELECT `+spikeCols+` FROM spikes s WHERE s.public_id = $1`, publicID))
}

// SpikeFilter narrows ListSpikes.
type SpikeFilter struct {
	InitiativeID *uuid.UUID
	FeatureID    *uuid.UUID
	// DirectOnly keeps the spikes on the initiative itself (no feature), for
	// the initiative's own section (SD-3).
	DirectOnly bool
	// State is one of the four states, or "" for all.
	State string
}

func querySpikes(ctx context.Context, q Querier, where, order string, args ...any) ([]Spike, error) {
	rows, err := q.Query(ctx, `SELECT `+spikeCols+` FROM spikes s WHERE `+where+` ORDER BY `+order, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Spike
	for rows.Next() {
		s, err := scanSpike(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// ListSpikes lists spikes by filter: those still open (idea, running, ended)
// before those closed, each group oldest first.
func ListSpikes(ctx context.Context, q Querier, f SpikeFilter) ([]Spike, error) {
	where := []string{"true"}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if f.InitiativeID != nil {
		where = append(where, "s.initiative_id = "+arg(*f.InitiativeID))
	}
	if f.FeatureID != nil {
		where = append(where, "s.feature_id = "+arg(*f.FeatureID))
	}
	if f.DirectOnly {
		where = append(where, "s.feature_id IS NULL")
	}
	if f.State != "" {
		where = append(where, "s.state = "+arg(f.State))
	}
	return querySpikes(ctx, q, strings.Join(where, " AND "),
		`(s.state = 'closed'), s.created_at, s.id`, args...)
}

// StartSpike moves an idea to running, recording the budget it was given, the
// commit and path of its worktree, and a spike.started audit row (FR-3). The
// update is conditional on the state, so two starts can't both win; the loser
// gets ErrSpikeNotIdea. budgetSource says where the figure came from
// ("override", "default" or "entered") and is audited.
func StartSpike(ctx context.Context, tx pgx.Tx, id uuid.UUID, budget int64, budgetSource, baseCommit, worktreePath, actor string) (*Spike, error) {
	s, err := scanSpike(tx.QueryRow(ctx, `
		WITH upd AS (
			UPDATE spikes SET state = 'running', token_budget = $2, base_commit = $3, worktree_path = $4,
			       started_by = $5, started_at = now()
			WHERE id = $1 AND state = 'idea'
			RETURNING *)
		SELECT `+spikeCols+` FROM upd s`,
		id, budget, baseCommit, worktreePath, actor))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrSpikeNotIdea
	}
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "spike.started", "spike", &id,
		map[string]any{"budget": budget, "source": budgetSource}); err != nil {
		return nil, err
	}
	return s, nil
}

// AddSpikeTokens adds one model call's usage to the spike's total and returns
// the new total (FR-5.1). The addition happens in the database, so
// overlapping attempts see each other's spending (SD-7).
func AddSpikeTokens(ctx context.Context, q Querier, id uuid.UUID, n int64) (int64, error) {
	var total int64
	err := q.QueryRow(ctx, `UPDATE spikes SET tokens_used = tokens_used + $2 WHERE id = $1
		RETURNING tokens_used`, id, n).Scan(&total)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return total, err
}

// SpikeTokens reads the spike's running total.
func SpikeTokens(ctx context.Context, q Querier, id uuid.UUID) (int64, error) {
	var total int64
	err := q.QueryRow(ctx, `SELECT tokens_used FROM spikes WHERE id = $1`, id).Scan(&total)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return total, err
}

// SaveSpikeDraft replaces the findings draft, but only while the spike is
// running; a save that arrives after the run ended is refused with
// ErrSpikeNotRunning (FR-4.3).
func SaveSpikeDraft(ctx context.Context, q Querier, id uuid.UUID, draft string) error {
	var got uuid.UUID
	err := q.QueryRow(ctx, `UPDATE spikes SET draft = $2, draft_saved_at = now()
		WHERE id = $1 AND state = 'running' RETURNING id`, id, draft).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSpikeNotRunning
	}
	return err
}

// EndSpikeState moves a running spike to ended, with how the run ended and, for
// a failure, its last error as a note (FR-6.2 step 3). changed is false when
// the spike wasn't running, which leaves it as it is and writes nothing.
func EndSpikeState(ctx context.Context, tx pgx.Tx, id uuid.UUID, how, note string) (changed bool, err error) {
	var used int64
	var budget *int64
	err = tx.QueryRow(ctx, `
		UPDATE spikes SET state = 'ended', ended_how = $2, end_note = NULLIF($3, ''), ended_at = now()
		WHERE id = $1 AND state = 'running'
		RETURNING tokens_used, token_budget`, id, how, note).Scan(&used, &budget)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	payload := map[string]any{"how": how, "tokens_used": used, "note": note}
	if budget != nil {
		payload["token_budget"] = *budget
	}
	return true, Audit(ctx, tx, "subutai", "spike.ended", "spike", &id, payload)
}

// MarkSpikeWorktreeRemoved records that the spike's worktree has been
// discarded, once (FR-6.2 step 5). changed is false when it was already
// recorded, and nothing is audited then.
func MarkSpikeWorktreeRemoved(ctx context.Context, tx pgx.Tx, id uuid.UUID) (changed bool, err error) {
	tag, err := tx.Exec(ctx, `UPDATE spikes SET worktree_removed_at = now()
		WHERE id = $1 AND worktree_removed_at IS NULL`, id)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, Audit(ctx, tx, "subutai", "spike.worktree_discarded", "spike", &id, map[string]any{})
}

// CloseSpike closes a spike as a person has read it (FR-7.2). The update is
// conditional: answered only from ended, unanswered from ended or from idea.
// When no row changes it returns ErrSpikeCannotClose.
func CloseSpike(ctx context.Context, tx pgx.Tx, id uuid.UUID, as, actor string) (*Spike, error) {
	if as != SpikeAnswered && as != SpikeUnanswered {
		return nil, errors.New("a spike closes as answered or unanswered, not " + strconv.Quote(as))
	}
	s, err := scanSpike(tx.QueryRow(ctx, `
		WITH upd AS (
			UPDATE spikes SET state = 'closed', closed_as = $2, closed_by = $3, closed_at = now()
			WHERE id = $1 AND (state = 'ended' OR (state = 'idea' AND $2 = 'unanswered'))
			RETURNING *)
		SELECT `+spikeCols+` FROM upd s`, id, as, actor))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrSpikeCannotClose
	}
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "spike.closed", "spike", &id, map[string]any{"as": as}); err != nil {
		return nil, err
	}
	return s, nil
}

// RecordSpikeCodeKept audits the refs that kept commits made in a spike's
// worktree (FR-6.3). Nothing is deleted; a person decides.
func RecordSpikeCodeKept(ctx context.Context, tx pgx.Tx, id uuid.UUID, refs []string) error {
	if refs == nil {
		refs = []string{}
	}
	return Audit(ctx, tx, "subutai", "spike.code_kept", "spike", &id, map[string]any{"refs": refs})
}

// SpikeRun is a running spike with its newest run-spike dispatch, for the
// reconciliation sweep (FR-6.4). The server decides what the pair means; the
// dispatch fields are zero when the spike has no run at all.
type SpikeRun struct {
	Spike         Spike
	DispatchID    *uuid.UUID
	DispatchState string
	Outcome       []byte
	Error         string
	Attempt       int
}

// SpikesToReconcile lists the running spikes with the state of their newest
// run-spike dispatch, oldest first. It leaves out none: a spike whose run is
// still queued or running is returned too, and the caller skips it.
func SpikesToReconcile(ctx context.Context, q Querier) ([]SpikeRun, error) {
	rows, err := q.Query(ctx, `
		SELECT `+spikeCols+`, d.id, coalesce(d.state::text, ''), d.outcome, coalesce(d.error, ''),
		       coalesce(d.attempt, 0)
		FROM spikes s
		LEFT JOIN LATERAL (
			SELECT id, state, outcome, error, attempt FROM dispatches
			WHERE ref_type = 'spike' AND ref_id = s.id AND purpose = 'run-spike'
			ORDER BY queued_at DESC, id DESC LIMIT 1) d ON true
		WHERE s.state = 'running'
		ORDER BY s.created_at, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpikeRun
	for rows.Next() {
		var r SpikeRun
		s := &r.Spike
		if err := rows.Scan(&s.ID, &s.PublicID, &s.InitiativeID, &s.FeatureID, &s.Question, &s.State,
			&s.BudgetOverride, &s.TokenBudget, &s.TokensUsed, &s.Draft, &s.DraftSavedAt,
			&s.EndedHow, &s.EndNote, &s.ClosedAs, &s.FollowsID,
			&s.BaseCommit, &s.WorktreePath, &s.WorktreeRemovedAt,
			&s.CreatedBy, &s.CreatedVia, &s.StartedBy, &s.StartedAt, &s.EndedAt,
			&s.ClosedBy, &s.ClosedAt, &s.CreatedAt, &s.UpdatedAt,
			&r.DispatchID, &r.DispatchState, &r.Outcome, &r.Error, &r.Attempt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SpikesWithLiveWorktree lists the spikes that aren't running but still have a
// worktree that hasn't been discarded (FR-6.4), whatever their state.
func SpikesWithLiveWorktree(ctx context.Context, q Querier) ([]Spike, error) {
	return querySpikes(ctx, q, `s.state <> 'running' AND s.worktree_path IS NOT NULL
		AND s.worktree_removed_at IS NULL`, `s.created_at, s.id`)
}

// OpenSpikesUnderInitiative counts the spikes not yet closed across the
// initiative's whole subtree: the input G5 adds to its count of unfinished
// features.
func OpenSpikesUnderInitiative(ctx context.Context, q Querier, initiativeID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM initiatives WHERE id = $1
			UNION ALL
			SELECT i.id FROM initiatives i JOIN subtree s ON i.parent_id = s.id
		)
		SELECT count(*) FROM spikes sp
		JOIN subtree s ON sp.initiative_id = s.id
		WHERE sp.state <> 'closed'`, initiativeID).Scan(&n)
	return n, err
}

// EndedSpikesCount is how many spikes wait for a person to read them, for the
// Inbox (FR-8).
func EndedSpikesCount(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM spikes WHERE state = 'ended'`).Scan(&n)
	return n, err
}
