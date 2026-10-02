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
	// SpikeTimeBox is a chat or person spike that reached its deadline
	// (SPEC-021 SD-20, FR-15).
	SpikeTimeBox = "time_box"
)

// Who may run a spike (SPEC-021 SD-17), chosen at the start. The chat and
// person words are the ones a claim's kind and an execution's kind use.
const (
	ExecutorAgent  = "agent"
	ExecutorChat   = "chat"   // == WriterChat
	ExecutorPerson = "person" // == WriterPerson
)

// How a person closed a spike (FR-7.2).
const (
	SpikeAnswered   = "answered"
	SpikeUnanswered = "unanswered"
)

// Where a spike's token budget came from, as audited at the start (SD-6).
const (
	BudgetFromOverride    = "override"
	BudgetFromDefault     = "default"
	BudgetFromStartScreen = "start screen"
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
	// Executor is who may run the spike: ExecutorAgent, ExecutorChat or
	// ExecutorPerson. It is empty before the start (SPEC-021 SD-17).
	Executor string
	// TimeBoxHours and DeadlineAt are a chat or person spike's limit, set at
	// the start; an agent spike has a token budget instead (SD-18).
	TimeBoxHours *int
	DeadlineAt   *time.Time
	Draft        string
	DraftSavedAt *time.Time
	EndedHow     string
	EndNote      string
	ClosedAs     string
	// FollowsID is the spike this one asks again for.
	FollowsID *uuid.UUID
	// BaseCommit and WorktreePath are set at the start; WorktreeRemovedAt
	// when the worktree was discarded.
	BaseCommit string
	// RefsAtStart is every ref and its commit when the run started, for the
	// leak check (FR-6.3); nil when none was recorded.
	RefsAtStart       map[string]string
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
	coalesce(s.base_commit, ''), s.refs_at_start, coalesce(s.worktree_path, ''), s.worktree_removed_at,
	s.created_by, s.created_via, coalesce(s.started_by, ''), s.started_at, s.ended_at,
	coalesce(s.closed_by, ''), s.closed_at, s.created_at, s.updated_at,
	coalesce(s.executor, ''), s.time_box_hours, s.deadline_at`

// scanSpike scans a row of spikeCols, then any extra columns the query adds.
func scanSpike(row pgx.Row, extra ...any) (*Spike, error) {
	var s Spike
	var refs []byte
	dest := append([]any{&s.ID, &s.PublicID, &s.InitiativeID, &s.FeatureID, &s.Question, &s.State,
		&s.BudgetOverride, &s.TokenBudget, &s.TokensUsed, &s.Draft, &s.DraftSavedAt,
		&s.EndedHow, &s.EndNote, &s.ClosedAs, &s.FollowsID,
		&s.BaseCommit, &refs, &s.WorktreePath, &s.WorktreeRemovedAt,
		&s.CreatedBy, &s.CreatedVia, &s.StartedBy, &s.StartedAt, &s.EndedAt,
		&s.ClosedBy, &s.ClosedAt, &s.CreatedAt, &s.UpdatedAt,
		&s.Executor, &s.TimeBoxHours, &s.DeadlineAt}, extra...)
	err := row.Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(refs) > 0 {
		if err := json.Unmarshal(refs, &s.RefsAtStart); err != nil {
			return nil, err
		}
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

// SpikeStart is what StartSpike records about a run's beginning.
type SpikeStart struct {
	// Executor is who runs the spike: ExecutorAgent, ExecutorChat or
	// ExecutorPerson (SPEC-021 SD-17).
	Executor string
	// Budget and BudgetSource are an agent spike's token budget and where it
	// came from (BudgetFromOverride, BudgetFromDefault or
	// BudgetFromStartScreen, audited). A chat or person spike has neither.
	Budget       int64
	BudgetSource string
	// TimeBoxHours is a chat or person spike's time box, in whole hours; the
	// deadline is the start plus this (SD-18).
	TimeBoxHours int
	BaseCommit   string
	WorktreePath string
	// RefsAtStart is every ref and its commit, for the leak check (FR-6.3).
	RefsAtStart map[string]string
	Actor       string
}

// StartSpike moves an idea to running, recording the executor, the budget or
// the time box it was given, the commit and path of its worktree, the refs as
// they stood, and a spike.started audit row (FR-3, SPEC-021 FR-11.2). An
// agent spike gets a token budget and no deadline; a chat or person spike gets
// a time box and a deadline of the start plus the time box, and no budget
// (SD-18). The update is conditional on the state, so two starts can't both
// win; the loser gets ErrSpikeNotIdea.
func StartSpike(ctx context.Context, tx pgx.Tx, id uuid.UUID, st SpikeStart) (*Spike, error) {
	refs, err := json.Marshal(st.RefsAtStart)
	if err != nil {
		return nil, err
	}
	var budget *int64
	var hours *int
	switch st.Executor {
	case ExecutorAgent:
		budget = &st.Budget
	case ExecutorChat, ExecutorPerson:
		hours = &st.TimeBoxHours
	default:
		return nil, errors.New("a spike is run by the agent, the chat agent or a person, not " + strconv.Quote(st.Executor))
	}
	// The deadline is computed from the same now() as started_at, so it is
	// the start plus the time box to the microsecond.
	s, err := scanSpike(tx.QueryRow(ctx, `
		WITH upd AS (
			UPDATE spikes SET state = 'running', executor = $2, token_budget = $3, time_box_hours = $4,
			       deadline_at = CASE WHEN $4::int IS NULL THEN NULL ELSE now() + make_interval(hours => $4::int) END,
			       base_commit = $5, worktree_path = $6, refs_at_start = $7, started_by = $8, started_at = now()
			WHERE id = $1 AND state = 'idea'
			RETURNING *)
		SELECT `+spikeCols+` FROM upd s`,
		id, st.Executor, budget, hours, st.BaseCommit, st.WorktreePath, refs, st.Actor))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrSpikeNotIdea
	}
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"executor": st.Executor}
	if budget != nil {
		payload["budget"] = st.Budget
		payload["source"] = st.BudgetSource
	} else {
		payload["time_box_hours"] = st.TimeBoxHours
	}
	if err := Audit(ctx, tx, st.Actor, "spike.started", "spike", &id, payload); err != nil {
		return nil, err
	}
	return s, nil
}

// LockSpike reads a spike with FOR UPDATE, for a path that must see the row
// as it stands and keep it until the transaction ends. In the lock order of
// SPEC-021 SD-27 the spike's row comes before its claim's.
func LockSpike(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Spike, error) {
	return scanSpike(tx.QueryRow(ctx, `SELECT `+spikeCols+` FROM spikes s WHERE s.id = $1 FOR UPDATE OF s`, id))
}

// SpikesPastDeadline lists the running chat and person spikes whose deadline
// has passed, oldest deadline first, for the heartbeat to end (SPEC-021
// FR-15.1).
func SpikesPastDeadline(ctx context.Context, q Querier) ([]Spike, error) {
	return querySpikes(ctx, q, `s.state = 'running' AND s.executor IN ('chat', 'person')
		AND s.deadline_at IS NOT NULL AND s.deadline_at <= now()`, `s.deadline_at, s.id`)
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

// EndSpikeState moves a running spike to ended, with how the run ended (one of
// the Spike* constants, SpikeTimeBox included; the table's checks refuse a
// pairing the executor can't have) and, for a failure, its last error as a
// note (FR-6.2 step 3). changed is false when
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
	return CloseSpikeRanIt(ctx, tx, id, as, actor, false)
}

// CloseSpikeRanIt is CloseSpike for a closer who may also have run the spike:
// when ranIt is set the spike.closed audit row carries closer_ran_it: true
// (SPEC-021 SD-26, FR-16.4).
func CloseSpikeRanIt(ctx context.Context, tx pgx.Tx, id uuid.UUID, as, actor string, ranIt bool) (*Spike, error) {
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
	payload := map[string]any{"as": as}
	if ranIt {
		payload["closer_ran_it"] = true
	}
	if err := Audit(ctx, tx, actor, "spike.closed", "spike", &id, payload); err != nil {
		return nil, err
	}
	return s, nil
}

// SpikeKept is what the leak check found (FR-6.3): the refs that kept code,
// or, when the check couldn't run, why not (and no refs).
type SpikeKept struct {
	Refs []string `json:"refs"`
	// CouldntCheck is the reason the check failed, or "".
	CouldntCheck string `json:"couldnt_check,omitempty"`
}

// RecordSpikeCodeKept audits what the leak check found. Nothing is deleted; a
// person decides.
func RecordSpikeCodeKept(ctx context.Context, tx pgx.Tx, id uuid.UUID, kept SpikeKept) error {
	if kept.Refs == nil {
		kept.Refs = []string{}
	}
	payload := map[string]any{"refs": kept.Refs}
	if kept.CouldntCheck != "" {
		payload["couldnt_check"] = kept.CouldntCheck
	}
	return Audit(ctx, tx, "subutai", "spike.code_kept", "spike", &id, payload)
}

// SpikeKeptRecord reads what an earlier leak check audited, or ErrNotFound
// when none did.
func SpikeKeptRecord(ctx context.Context, q Querier, id uuid.UUID) (*SpikeKept, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT payload FROM audit_events
		WHERE kind = 'spike.code_kept' AND ref_type = 'spike' AND ref_id = $1
		ORDER BY occurred_at DESC, id DESC LIMIT 1`, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var k SpikeKept
	if err := json.Unmarshal(raw, &k); err != nil {
		return nil, err
	}
	return &k, nil
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
		sp, err := scanSpike(rows, &r.DispatchID, &r.DispatchState, &r.Outcome, &r.Error, &r.Attempt)
		if err != nil {
			return nil, err
		}
		r.Spike = *sp
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

// EndedSpikesCount is how many spikes wait for a person to read them, for the
// Inbox (FR-8).
func EndedSpikesCount(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM spikes WHERE state = 'ended'`).Scan(&n)
	return n, err
}
