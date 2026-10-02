package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// spikeFixture is an initiative with a child, and a feature on the parent.
type spikeFixture struct {
	s       *Store
	parent  *Initiative
	child   *Initiative
	feature *Feature
}

func newSpikeFixture(t *testing.T) *spikeFixture {
	t.Helper()
	s := testStore(t)
	ctx := context.Background()
	fx := &spikeFixture{s: s}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if fx.parent, err = CreateInitiative(ctx, tx, nil, "auth", "Authentication", "", "sam"); err != nil {
			return err
		}
		if fx.child, err = CreateInitiative(ctx, tx, &fx.parent.ID, "sso", "Single sign-on", "", "sam"); err != nil {
			return err
		}
		fx.feature, err = CreateFeature(ctx, tx, fx.parent.ID, "login", "Login form", "", "sam")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

func (fx *spikeFixture) create(t *testing.T, init uuid.UUID, feat *uuid.UUID, q string) *Spike {
	t.Helper()
	ctx := context.Background()
	var sp *Spike
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		sp, err = CreateSpike(ctx, tx, NewSpike{InitiativeID: init, FeatureID: feat, Question: q,
			CreatedBy: "sam", CreatedVia: "ui"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateSpike: %v", err)
	}
	return sp
}

// start moves a spike to running.
func (fx *spikeFixture) start(t *testing.T, id uuid.UUID, budget int64) *Spike {
	t.Helper()
	ctx := context.Background()
	var sp *Spike
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		sp, err = StartSpike(ctx, tx, id, SpikeStart{Budget: budget, BudgetSource: BudgetFromDefault,
			BaseCommit: "abc123", WorktreePath: "worktrees/spk-001",
			RefsAtStart: map[string]string{"refs/heads/main": "abc123"}, Actor: "sam"})
		return err
	})
	if err != nil {
		t.Fatalf("StartSpike: %v", err)
	}
	return sp
}

func (fx *spikeFixture) end(t *testing.T, id uuid.UUID, how, note string) bool {
	t.Helper()
	ctx := context.Background()
	var changed bool
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		changed, err = EndSpikeState(ctx, tx, id, how, note)
		return err
	})
	if err != nil {
		t.Fatalf("EndSpikeState: %v", err)
	}
	return changed
}

func (fx *spikeFixture) close(t *testing.T, id uuid.UUID, as string) error {
	t.Helper()
	ctx := context.Background()
	return fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := CloseSpike(ctx, tx, id, as, "sam")
		return err
	})
}

// FR-1.1, FR-1.3: a spike gets the next SPK number, its owner, who made it and
// how, and a spike.created audit row.
func TestCreateSpikeNumbersAndAudits(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()

	a := fx.create(t, fx.parent.ID, nil, "Can we stream the export?")
	b := fx.create(t, fx.parent.ID, &fx.feature.ID, "Does the login form need a captcha?")
	var c *Spike
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		c, err = CreateSpike(ctx, tx, NewSpike{InitiativeID: fx.parent.ID, Question: "Third?",
			CreatedBy: "chat", CreatedVia: "mcp", FollowsID: &a.ID, BudgetOverride: ptr(int64(5000))})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, sp := range []*Spike{a, b, c} {
		want := []string{"SPK-001", "SPK-002", "SPK-003"}[i]
		if sp.PublicID != want {
			t.Errorf("spike %d = %s, want %s", i, sp.PublicID, want)
		}
		if sp.State != SpikeIdea || sp.TokensUsed != 0 || sp.TokenBudget != nil {
			t.Errorf("%s: state %q, used %d, budget %v", sp.PublicID, sp.State, sp.TokensUsed, sp.TokenBudget)
		}
	}
	if a.FeatureID != nil || b.FeatureID == nil || *b.FeatureID != fx.feature.ID {
		t.Errorf("owners: %v, %v", a.FeatureID, b.FeatureID)
	}
	if a.CreatedVia != "ui" || c.CreatedVia != "mcp" || c.CreatedBy != "chat" {
		t.Errorf("created via: %q, %q by %q", a.CreatedVia, c.CreatedVia, c.CreatedBy)
	}
	if c.FollowsID == nil || *c.FollowsID != a.ID || c.BudgetOverride == nil || *c.BudgetOverride != 5000 {
		t.Errorf("follows %v, override %v", c.FollowsID, c.BudgetOverride)
	}

	got, err := SpikeByPublicID(ctx, fx.s.Pool, "SPK-002")
	if err != nil || got.ID != b.ID {
		t.Fatalf("SpikeByPublicID = %v, %v", got, err)
	}
	if _, err := GetSpike(ctx, fx.s.Pool, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSpike of nothing = %v, want ErrNotFound", err)
	}
	if _, err := SpikeByPublicID(ctx, fx.s.Pool, "SPK-099"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SpikeByPublicID of nothing = %v, want ErrNotFound", err)
	}
	if n := spikeAudits(t, fx.s, a.ID, "spike.created"); n != 1 {
		t.Errorf("spike.created rows = %d, want 1", n)
	}
}

func ptr[T any](v T) *T { return &v }

func spikeAudits(t *testing.T, s *Store, id uuid.UUID, kind string) int {
	t.Helper()
	var n int
	err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_events WHERE ref_type = 'spike' AND ref_id = $1 AND kind = $2`,
		id, kind).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// FR-1.1: each named check refuses a row that breaks it, written as raw SQL
// so the code's own guards can't stand in for the database's.
func TestSpikeChecksRefuseBadRows(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	fresh := fx.create(t, fx.parent.ID, nil, "A question")

	insert := func(cols, vals string) error {
		_, err := fx.s.Pool.Exec(ctx, `INSERT INTO spikes (id, initiative_id, created_by, created_via, `+cols+`)
			VALUES (gen_random_uuid(), $1, 'sam', 'ui', `+vals+`)`, fx.parent.ID)
		return err
	}
	update := func(set string) error {
		_, err := fx.s.Pool.Exec(ctx, `UPDATE spikes SET `+set+` WHERE id = $1`, fresh.ID)
		return err
	}
	started := `token_budget = 100, started_at = now()`
	ended := `state = 'ended', ` + started + `, ended_how = 'budget'`

	_, badVia := fx.s.Pool.Exec(ctx, `INSERT INTO spikes (id, initiative_id, question, created_by, created_via)
		VALUES (gen_random_uuid(), $1, 'q', 'sam', 'cli')`, fx.parent.ID)

	for _, tc := range []struct {
		name, constraint string
		err              error
	}{
		{"blank question", "spikes_question", insert("question", "'   '")},
		{"long question", "spikes_question", insert("question", "'"+strings.Repeat("q", 501)+"'")},
		{"bad state", "spikes_state", update(`state = 'paused'`)},
		{"bad ended_how", "spikes_ended_how_values", update(`state = 'ended', ` + started + `, ended_how = 'gave_up'`)},
		{"bad closed_as", "spikes_closed_as_values", update(`state = 'closed', closed_as = 'maybe'`)},
		{"bad created_via", "spikes_created_via", badVia},
		{"closed without closed_as", "spikes_closed_as", update(`state = 'closed'`)},
		{"closed_as while open", "spikes_closed_as", update(`closed_as = 'answered'`)},
		{"ended without ended_how", "spikes_ended_how", update(`state = 'ended', ` + started)},
		{"ended_how while idea", "spikes_ended_how", update(`ended_how = 'budget'`)},
		{"running without a budget", "spikes_started", update(`state = 'running', started_at = now()`)},
		{"running without a start", "spikes_started", update(`state = 'running', token_budget = 100`)},
		{"closed unrun as answered", "spikes_closed_unrun_unanswered", update(`state = 'closed', closed_as = 'answered'`)},
		{"zero override", "spikes_budget_positive", update(`budget_override = 0`)},
		{"negative budget", "spikes_budget_positive", update(`token_budget = -5`)},
		{"negative tokens", "spikes_tokens_nonneg", update(`tokens_used = -1`)},
		{"follows itself", "spikes_not_self_follow", update(`follows_id = id`)},
	} {
		if tc.err == nil || !strings.Contains(tc.err.Error(), tc.constraint) {
			t.Errorf("%s: err = %v, want a violation of %s", tc.name, tc.err, tc.constraint)
		}
	}

	// And the good shapes the checks must let through.
	if err := update(ended); err != nil {
		t.Errorf("an ended spike: %v", err)
	}
	if err := update(`state = 'closed', closed_as = 'answered'`); err != nil {
		t.Errorf("an ended spike closed after its run: %v", err)
	}
}

// SD-4, FR-1.1: StartSpike is conditional on the idea state, and the loser of
// a race gets the sentinel without a second audit row.
func TestStartSpikeIsConditional(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.create(t, fx.parent.ID, nil, "Q")

	started := fx.start(t, sp.ID, 1_000_000)
	if started.State != SpikeRunning || started.TokenBudget == nil || *started.TokenBudget != 1_000_000 ||
		started.StartedBy != "sam" || started.StartedAt == nil ||
		started.BaseCommit != "abc123" || started.WorktreePath != "worktrees/spk-001" ||
		started.RefsAtStart["refs/heads/main"] != "abc123" || len(started.RefsAtStart) != 1 {
		t.Errorf("started = %+v", started)
	}

	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := StartSpike(ctx, tx, sp.ID, SpikeStart{Budget: 5, BudgetSource: BudgetFromStartScreen,
			BaseCommit: "def", WorktreePath: "x", RefsAtStart: map[string]string{}, Actor: "sam"})
		return err
	})
	if !errors.Is(err, ErrSpikeNotIdea) {
		t.Errorf("second start = %v, want ErrSpikeNotIdea", err)
	}
	if n := spikeAudits(t, fx.s, sp.ID, "spike.started"); n != 1 {
		t.Errorf("spike.started rows = %d, want 1", n)
	}
	again, _ := GetSpike(ctx, fx.s.Pool, sp.ID)
	if *again.TokenBudget != 1_000_000 {
		t.Errorf("the loser changed the budget to %d", *again.TokenBudget)
	}
}

// FR-5.1: tokens add up in the database and each call returns the total.
func TestSpikeTokensAccumulate(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.start(t, fx.create(t, fx.parent.ID, nil, "Q").ID, 1000)

	for i, want := range []int64{150, 300, 450} {
		got, err := AddSpikeTokens(ctx, fx.s.Pool, sp.ID, 150)
		if err != nil || got != want {
			t.Fatalf("add %d = %d, %v; want %d", i, got, err, want)
		}
	}
	if got, err := SpikeTokens(ctx, fx.s.Pool, sp.ID); err != nil || got != 450 {
		t.Errorf("SpikeTokens = %d, %v", got, err)
	}
	if _, err := AddSpikeTokens(ctx, fx.s.Pool, uuid.New(), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("adding to nothing = %v", err)
	}
	if _, err := SpikeTokens(ctx, fx.s.Pool, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading nothing = %v", err)
	}
}

// FR-4.3: the draft is saved only while the run is going.
func TestSaveSpikeDraftOnlyWhileRunning(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	idea := fx.create(t, fx.parent.ID, nil, "Q")
	if err := SaveSpikeDraft(ctx, fx.s.Pool, idea.ID, "early"); !errors.Is(err, ErrSpikeNotRunning) {
		t.Errorf("save on an idea = %v, want ErrSpikeNotRunning", err)
	}

	fx.start(t, idea.ID, 100)
	if err := SaveSpikeDraft(ctx, fx.s.Pool, idea.ID, "## Answer\nYes."); err != nil {
		t.Fatal(err)
	}
	got, _ := GetSpike(ctx, fx.s.Pool, idea.ID)
	if got.Draft != "## Answer\nYes." || got.DraftSavedAt == nil {
		t.Errorf("draft = %q at %v", got.Draft, got.DraftSavedAt)
	}

	fx.end(t, idea.ID, SpikeConcluded, "")
	if err := SaveSpikeDraft(ctx, fx.s.Pool, idea.ID, "late"); !errors.Is(err, ErrSpikeNotRunning) {
		t.Errorf("save after the end = %v, want ErrSpikeNotRunning", err)
	}
	got, _ = GetSpike(ctx, fx.s.Pool, idea.ID)
	if got.Draft != "## Answer\nYes." {
		t.Errorf("a late save changed the draft to %q", got.Draft)
	}
}

// FR-6.2 step 3: ending is conditional on running, and idempotent.
func TestEndSpikeStateIsConditional(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.create(t, fx.parent.ID, nil, "Q")
	if fx.end(t, sp.ID, SpikeBudget, "") {
		t.Error("an idea was ended")
	}

	fx.start(t, sp.ID, 1000)
	if _, err := AddSpikeTokens(ctx, fx.s.Pool, sp.ID, 1200); err != nil {
		t.Fatal(err)
	}
	if !fx.end(t, sp.ID, SpikeFailed, "provider fell over") {
		t.Fatal("a running spike wasn't ended")
	}
	got, _ := GetSpike(ctx, fx.s.Pool, sp.ID)
	if got.State != SpikeEnded || got.EndedHow != SpikeFailed || got.EndNote != "provider fell over" || got.EndedAt == nil {
		t.Errorf("ended = %+v", got)
	}
	if fx.end(t, sp.ID, SpikeConcluded, "") {
		t.Error("an ended spike was ended again")
	}
	got, _ = GetSpike(ctx, fx.s.Pool, sp.ID)
	if got.EndedHow != SpikeFailed {
		t.Errorf("the second end changed how to %q", got.EndedHow)
	}
	if n := spikeAudits(t, fx.s, sp.ID, "spike.ended"); n != 1 {
		t.Errorf("spike.ended rows = %d, want 1", n)
	}
	var tokens, budget float64
	err := fx.s.Pool.QueryRow(ctx, `SELECT (payload->>'tokens_used')::float, (payload->>'token_budget')::float
		FROM audit_events WHERE ref_id = $1 AND kind = 'spike.ended'`, sp.ID).Scan(&tokens, &budget)
	if err != nil || tokens != 1200 || budget != 1000 {
		t.Errorf("audit payload tokens %v, budget %v, err %v", tokens, budget, err)
	}
}

// FR-6.2 step 5, FR-6.4: the worktree is marked discarded once, and a spike
// that still has one is found.
func TestSpikeWorktreeRemoval(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.create(t, fx.parent.ID, nil, "Q")
	fx.start(t, sp.ID, 100)

	live, err := SpikesWithLiveWorktree(ctx, fx.s.Pool)
	if err != nil || len(live) != 0 {
		t.Fatalf("while running: %d live, %v; want none", len(live), err)
	}
	fx.end(t, sp.ID, SpikeConcluded, "")
	if live, _ = SpikesWithLiveWorktree(ctx, fx.s.Pool); len(live) != 1 || live[0].ID != sp.ID {
		t.Fatalf("after the end: %v", live)
	}

	mark := func() bool {
		var changed bool
		err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
			var err error
			changed, err = MarkSpikeWorktreeRemoved(ctx, tx, sp.ID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return changed
	}
	if !mark() || mark() {
		t.Error("the mark should change once, then not")
	}
	if live, _ = SpikesWithLiveWorktree(ctx, fx.s.Pool); len(live) != 0 {
		t.Errorf("after the discard: %v", live)
	}
	if n := spikeAudits(t, fx.s, sp.ID, "spike.worktree_discarded"); n != 1 {
		t.Errorf("spike.worktree_discarded rows = %d, want 1", n)
	}
}

// FR-7.2: answered only from ended; unanswered from ended or idea; never from
// running or closed.
func TestCloseSpikeRules(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()

	idea := fx.create(t, fx.parent.ID, nil, "idea")
	if err := fx.close(t, idea.ID, SpikeAnswered); !errors.Is(err, ErrSpikeCannotClose) {
		t.Errorf("answered from idea = %v, want ErrSpikeCannotClose", err)
	}
	if err := fx.close(t, idea.ID, SpikeUnanswered); err != nil {
		t.Fatalf("unanswered from idea: %v", err)
	}
	got, _ := GetSpike(ctx, fx.s.Pool, idea.ID)
	if got.State != SpikeClosed || got.ClosedAs != SpikeUnanswered || got.ClosedBy != "sam" || got.ClosedAt == nil || got.StartedAt != nil {
		t.Errorf("closed idea = %+v", got)
	}
	if err := fx.close(t, idea.ID, SpikeUnanswered); !errors.Is(err, ErrSpikeCannotClose) {
		t.Errorf("closing twice = %v, want ErrSpikeCannotClose", err)
	}

	run := fx.create(t, fx.parent.ID, nil, "running")
	fx.start(t, run.ID, 100)
	for _, as := range []string{SpikeAnswered, SpikeUnanswered} {
		if err := fx.close(t, run.ID, as); !errors.Is(err, ErrSpikeCannotClose) {
			t.Errorf("%s from running = %v, want ErrSpikeCannotClose", as, err)
		}
	}
	fx.end(t, run.ID, SpikeBudget, "")
	if err := fx.close(t, run.ID, SpikeAnswered); err != nil {
		t.Fatalf("answered from ended: %v", err)
	}
	got, _ = GetSpike(ctx, fx.s.Pool, run.ID)
	if got.ClosedAs != SpikeAnswered || got.EndedHow != SpikeBudget {
		t.Errorf("closed after a run = %+v", got)
	}

	other := fx.create(t, fx.parent.ID, nil, "ended")
	fx.start(t, other.ID, 100)
	fx.end(t, other.ID, SpikeConcluded, "")
	if err := fx.close(t, other.ID, SpikeUnanswered); err != nil {
		t.Errorf("unanswered from ended: %v", err)
	}
	if err := fx.close(t, other.ID, "maybe"); err == nil {
		t.Error("closing as 'maybe' was allowed")
	}
	if n := spikeAudits(t, fx.s, run.ID, "spike.closed"); n != 1 {
		t.Errorf("spike.closed rows = %d, want 1", n)
	}
}

// ListSpikes puts open spikes before closed, then oldest first, and filters.
func TestListSpikesOrderAndFilters(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()

	first := fx.create(t, fx.parent.ID, nil, "first")    // will be closed
	fx.create(t, fx.parent.ID, &fx.feature.ID, "second") // idea
	third := fx.create(t, fx.parent.ID, nil, "third")    // running
	fourth := fx.create(t, fx.child.ID, nil, "fourth")   // ended
	fx.start(t, third.ID, 100)
	fx.start(t, fourth.ID, 100)
	fx.end(t, fourth.ID, SpikeConcluded, "")
	if err := fx.close(t, first.ID, SpikeUnanswered); err != nil {
		t.Fatal(err)
	}

	ids := func(f SpikeFilter) []string {
		t.Helper()
		got, err := ListSpikes(ctx, fx.s.Pool, f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, sp := range got {
			out = append(out, sp.PublicID)
		}
		return out
	}
	eq := func(name string, got []string, want ...string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	eq("all", ids(SpikeFilter{}), "SPK-002", "SPK-003", "SPK-004", "SPK-001")
	eq("parent", ids(SpikeFilter{InitiativeID: &fx.parent.ID}), "SPK-002", "SPK-003", "SPK-001")
	eq("parent direct", ids(SpikeFilter{InitiativeID: &fx.parent.ID, DirectOnly: true}), "SPK-003", "SPK-001")
	eq("feature", ids(SpikeFilter{FeatureID: &fx.feature.ID}), "SPK-002")
	eq("running", ids(SpikeFilter{State: SpikeRunning}), "SPK-003")
	eq("closed", ids(SpikeFilter{State: SpikeClosed}), "SPK-001")
}

// The Inbox counts the spikes waiting to be read.
func TestEndedSpikesCount(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	onChild := fx.create(t, fx.child.ID, nil, "on the child")
	fx.start(t, onChild.ID, 100)
	fx.end(t, onChild.ID, SpikeConcluded, "")
	if n, err := EndedSpikesCount(ctx, fx.s.Pool); err != nil || n != 1 {
		t.Errorf("EndedSpikesCount = %d, %v; want 1", n, err)
	}
}

// FR-6.4: a running spike comes back with its newest run-spike dispatch.
func TestSpikesToReconcile(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	withRun := fx.start(t, fx.create(t, fx.parent.ID, nil, "has a run").ID, 100)
	noRun := fx.start(t, fx.create(t, fx.parent.ID, nil, "has none").ID, 100)
	fx.create(t, fx.parent.ID, nil, "an idea")

	var d *Dispatch
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if d, err = EnqueueDispatch(ctx, tx, "run-spike", "spike-runner", "m", "spike", withRun.ID, "k1"); err != nil {
			return err
		}
		if err := MarkDispatchFailed(ctx, tx, d.ID, "boom"); err != nil {
			return err
		}
		// An older dispatch of another purpose must not be mistaken for the run.
		_, err = EnqueueDispatch(ctx, tx, "other", "spike-runner", "m", "spike", noRun.ID, "k2")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := SpikesToReconcile(ctx, fx.s.Pool)
	if err != nil || len(got) != 2 {
		t.Fatalf("SpikesToReconcile = %d, %v; want 2", len(got), err)
	}
	if got[0].Spike.ID != withRun.ID || got[0].DispatchID == nil || *got[0].DispatchID != d.ID ||
		got[0].DispatchState != "failed" || got[0].Error != "boom" {
		t.Errorf("with a run: %+v", got[0])
	}
	if got[1].Spike.ID != noRun.ID || got[1].DispatchID != nil || got[1].DispatchState != "" {
		t.Errorf("without a run: %+v", got[1])
	}
}

// FR-6.3: what the leak check found is audited once, and read back whole, a
// failed check with its reason; a spike nothing was recorded for has no record.
func TestRecordSpikeCodeKept(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.create(t, fx.parent.ID, nil, "Q")
	if _, err := SpikeKeptRecord(ctx, fx.s.Pool, sp.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("before anything is recorded: %v, want ErrNotFound", err)
	}
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		return RecordSpikeCodeKept(ctx, tx, sp.ID, SpikeKept{Refs: []string{"keep-this", "refs/stash"}})
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := SpikeKeptRecord(ctx, fx.s.Pool, sp.ID)
	if err != nil || len(got.Refs) != 2 || got.Refs[0] != "keep-this" || got.CouldntCheck != "" {
		t.Errorf("record = %+v, %v", got, err)
	}

	other := fx.create(t, fx.parent.ID, nil, "Q2")
	err = fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		return RecordSpikeCodeKept(ctx, tx, other.ID, SpikeKept{CouldntCheck: "git said no"})
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = SpikeKeptRecord(ctx, fx.s.Pool, other.ID)
	if err != nil || len(got.Refs) != 0 || got.CouldntCheck != "git said no" {
		t.Errorf("failed check = %+v, %v", got, err)
	}
}

// Appendix A, FR-1.1: a findings document can be owned by a spike, its ID's
// owner part is the spike's ID, and a transcript can end in a stop entry.
func TestSpikeOwnsFindingsAndStopEntryIsAccepted(t *testing.T) {
	fx := newSpikeFixture(t)
	ctx := context.Background()
	sp := fx.create(t, fx.parent.ID, nil, "Q")

	var doc *Document
	err := fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		doc, err = RegisterDocument(ctx, tx, "findings", "spike", &sp.ID,
			"docs/work/INIT-001-auth/SPK-001-findings.md", "SPK-001: Q", "hash", nil, "subutai")
		return err
	})
	if err != nil {
		t.Fatalf("RegisterDocument: %v", err)
	}
	if doc.OwnerType != "spike" || doc.Type != "findings" {
		t.Errorf("doc = %+v", doc)
	}
	owner, err := OwnerPublicID(ctx, fx.s.Pool, "spike", &sp.ID)
	if err != nil || owner != "SPK-001" {
		t.Errorf("OwnerPublicID = %q, %v", owner, err)
	}
	id, rev, err := NextDocumentIdentity(ctx, fx.s.Pool, "spike", &sp.ID, "findings")
	if err != nil || id != "SPK-001-findings" || rev != 1 {
		t.Errorf("NextDocumentIdentity = %q, %d, %v", id, rev, err)
	}
	if _, err := OwnerPublicID(ctx, fx.s.Pool, "spike", nil); err == nil {
		t.Error("a spike owner without an id was accepted")
	}

	// The owner check still refuses what it should.
	err = fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := RegisterDocument(ctx, tx, "spec", "task", &sp.ID, "docs/x.md", "x", "h", nil, "sam")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "documents_owner_kind") {
		t.Errorf("a task owner = %v, want a violation of documents_owner_kind", err)
	}

	// A run that stopped at its limit ends its transcript with a stop entry.
	run := fx.start(t, sp.ID, 100)
	var d *Dispatch
	err = fx.s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		d, err = EnqueueDispatch(ctx, tx, "run-spike", "spike-runner", "m", "spike", run.ID, "k")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fx.s.AppendTranscript(ctx, []TranscriptEntry{{DispatchID: d.ID, Attempt: 1, Seq: 1,
		Kind: EntryStop, Content: "The run stopped here because it reached its budget of 100 tokens."}})
	if err != nil {
		t.Errorf("a stop entry was refused: %v", err)
	}
	err = fx.s.AppendTranscript(ctx, []TranscriptEntry{{DispatchID: d.ID, Attempt: 1, Seq: 2, Kind: "halt"}})
	if err == nil || !strings.Contains(err.Error(), "transcript_entries_kind") {
		t.Errorf("an unknown kind = %v, want a violation of transcript_entries_kind", err)
	}
}
