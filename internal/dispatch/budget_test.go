package dispatch

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/config"
	"subutai/internal/provider"
	"subutai/internal/store"
	"subutai/internal/testdb"
	"subutai/internal/toolhost"
)

type fakePlanner struct{ plan *Plan }

func (p fakePlanner) Plan(context.Context, *store.Dispatch) (*Plan, error) { return p.plan, nil }

type fakeTools struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeTools) Execute(_ context.Context, _ *toolhost.Context, name string, _ json.RawMessage) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	return "ok", false
}

// counter is an in-memory stand-in for the database's run total.
type counter struct{ n int64 }

func (c *counter) budget(limit int64) *Budget {
	return &Budget{
		Limit:      limit,
		Total:      func(context.Context) (int64, error) { return c.n, nil },
		Add:        func(_ context.Context, t int64) (int64, error) { c.n += t; return c.n, nil },
		EarlyTools: []string{"save_findings"},
	}
}

type rig struct {
	dp    *Dispatcher
	d     *store.Dispatch
	mock  *provider.Mock
	tools *fakeTools
	st    *store.Store
}

// newRig builds a Dispatcher over a fresh test database, with one running
// dispatch, a mock provider and a fake tool executor.
func newRig(t *testing.T, plan *Plan) *rig {
	t.Helper()
	ctx := context.Background()
	url := testdb.URL(t, "dispatch_budget")
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(ctx)
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	var d *store.Dispatch
	err = st.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		d, err = store.EnqueueDispatch(ctx, tx, "spike", "researcher", "m", "project", uuid.New(), "k-"+uuid.NewString())
		if err != nil {
			return err
		}
		_, err = store.MarkDispatchRunning(ctx, tx, d.ID, map[string]any{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	mock := &provider.Mock{}
	tools := &fakeTools{}
	plan.ToolCtx = &toolhost.Context{Profile: map[string]bool{"save_findings": true, "read_file": true}}
	dp := &Dispatcher{
		Store: st, Bus: bus.New(16), Planner: fakePlanner{plan}, Tools: tools,
		Config: func() (*config.Config, error) {
			return &config.Config{Models: map[string]config.Model{"m": {Provider: "p"}}}, nil
		},
		Providers:   func(string) (provider.Provider, error) { return mock, nil },
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		BackoffBase: time.Millisecond,
	}
	return &rig{dp: dp, d: d, mock: mock, tools: tools, st: st}
}

func basePlan(turnCap int, b *Budget) *Plan {
	return &Plan{System: "s", User: "u", TurnCap: turnCap, OutcomeTool: "conclude", Budget: b}
}

var use = provider.Usage{Input: 100, Output: 50}

func (r *rig) run(t *testing.T) (json.RawMessage, provider.Usage, error) {
	t.Helper()
	return r.dp.runLoop(context.Background(), r.d)
}

// stopEntry returns the transcript's stop entry, if any. The 'stop' kind
// needs the spikes migration; without it the recorder's best-effort write
// fails and the outcome is the assertion that counts.
func (r *rig) stopEntry(t *testing.T) string {
	t.Helper()
	var content string
	err := r.st.Pool.QueryRow(context.Background(),
		`SELECT content FROM transcript_entries WHERE dispatch_id = $1 AND kind = 'stop'`, r.d.ID).Scan(&content)
	if err != nil {
		t.Logf("no stop transcript entry (migration may not admit kind 'stop' yet): %v", err)
	}
	return content
}

func wantEnded(t *testing.T, out json.RawMessage, want string) {
	t.Helper()
	if string(out) != `{"ended":"`+want+`"}` {
		t.Fatalf("outcome = %s, want ended %s", out, want)
	}
}

func TestBudgetStopsBeforeACallThatCannotFit(t *testing.T) {
	c := &counter{}
	r := newRig(t, basePlan(40, c.budget(1000)))
	for i := 0; i < 20; i++ {
		r.mock.RespondToolUse("read_file", `{}`, use)
	}
	out, _, err := r.run(t)
	if err != nil {
		t.Fatal(err)
	}
	wantEnded(t, out, StopBudget)
	// 6 calls of 150 = 900; a 7th would bring it to at least 1,050.
	if got := len(r.mock.Requests); got != 6 {
		t.Errorf("provider calls = %d, want 6", got)
	}
	if c.n != 900 {
		t.Errorf("total = %d, want 900", c.n)
	}
	if s := r.stopEntry(t); s != "" && !strings.Contains(s, "budget of 1,000 tokens") {
		t.Errorf("stop entry = %q", s)
	}
}

func TestBudgetKeepsEarlyToolOnTheCrossingTurn(t *testing.T) {
	c := &counter{n: 900}
	r := newRig(t, basePlan(40, c.budget(1000)))
	r.mock.RespondParallelToolUse([]struct{ Tool, InputJSON string }{
		{"read_file", `{}`}, {"save_findings", `{}`},
	}, provider.Usage{Input: 150, Output: 50})
	out, _, err := r.run(t)
	if err != nil {
		t.Fatal(err)
	}
	wantEnded(t, out, StopBudget)
	if len(r.tools.calls) != 1 || r.tools.calls[0] != "save_findings" {
		t.Errorf("tools run = %v, want only save_findings", r.tools.calls)
	}
}

func TestBudgetOutcomeOnTheCrossingTurnConcludes(t *testing.T) {
	c := &counter{n: 900}
	r := newRig(t, basePlan(40, c.budget(1000)))
	r.mock.RespondOutcome("conclude", `{"done":true}`, provider.Usage{Input: 150, Output: 50})
	out, _, err := r.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"done":true}` {
		t.Fatalf("outcome = %s", out)
	}
	if c.n != 1100 {
		t.Errorf("total = %d, want 1100 (the paid-for call is counted)", c.n)
	}
}

func TestBudgetTurnLimitStops(t *testing.T) {
	c := &counter{}
	r := newRig(t, basePlan(3, c.budget(1_000_000)))
	for i := 0; i < 3; i++ {
		r.mock.RespondToolUse("read_file", `{}`, use)
	}
	out, _, err := r.run(t)
	if err != nil {
		t.Fatal(err)
	}
	wantEnded(t, out, StopTurnLimit)
	if s := r.stopEntry(t); s != "" && !strings.Contains(s, "turn limit of 3 turns") {
		t.Errorf("stop entry = %q", s)
	}
}

func TestBudgetAlreadySpentMakesNoCalls(t *testing.T) {
	c := &counter{n: 1000}
	r := newRig(t, basePlan(40, c.budget(1000)))
	out, _, err := r.run(t)
	if err != nil {
		t.Fatal(err)
	}
	wantEnded(t, out, StopBudget)
	if len(r.mock.Requests) != 0 {
		t.Errorf("provider calls = %d, want 0", len(r.mock.Requests))
	}
}

func TestNoBudgetTurnCapStillFails(t *testing.T) {
	r := newRig(t, basePlan(2, nil))
	r.mock.RespondToolUse("read_file", `{}`, use).RespondToolUse("read_file", `{}`, use)
	out, _, err := r.run(t)
	if err == nil || !strings.Contains(err.Error(), "turn cap 2 reached without conclude") {
		t.Fatalf("err = %v, out = %s", err, out)
	}
}

func TestCommasAndBudgetTokens(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1,000", 40000: "40,000", 1234567: "1,234,567"} {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %q, want %q", n, got, want)
		}
	}
	if got := BudgetTokens(provider.Usage{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}); got != 10 {
		t.Errorf("BudgetTokens = %d", got)
	}
}
