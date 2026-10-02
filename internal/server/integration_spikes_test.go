package server

// The spikes suite (SPEC-021), with the mock provider against real Postgres
// and a real git repository. Spikes are driven through the service methods
// the web UI and the MCP tools will call (NFR-1); the worktrees are real, so
// the tests check the directory and `git worktree list`, not only the row.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/provider"
	"subutai/internal/rules"
	"subutai/internal/store"
	"subutai/internal/toolhost"
)

// ---- Fixtures ----

// spikeInitiative creates an initiative and returns its row id.
func (h *harness) spikeInitiative(slug string) uuid.UUID {
	h.t.Helper()
	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": slug, "name": "The " + slug + " initiative"}); code != 201 {
		h.t.Fatalf("create initiative %s: %d %v", slug, code, out)
	}
	return h.initiativeID(slug)
}

// newSpike writes down a question on an initiative, as a person in the UI.
func (h *harness) newSpike(initiativeID uuid.UUID, question string, budget *int64) *store.Spike {
	h.t.Helper()
	sp, err := h.srv.CreateSpike(context.Background(), "initiative", initiativeID, question, budget, "sam", "ui")
	if err != nil {
		h.t.Fatalf("create spike: %v", err)
	}
	return sp
}

func (h *harness) getSpike(id uuid.UUID) *store.Spike {
	h.t.Helper()
	sp, err := store.GetSpike(context.Background(), h.srv.Store.Pool, id)
	if err != nil {
		h.t.Fatal(err)
	}
	return sp
}

// startSpike starts a spike as the operator does, with a figure of 0 for the
// spike's own or the default budget. The mock must already be scripted.
func (h *harness) startSpike(sp *store.Spike, budget int64) *store.Spike {
	h.t.Helper()
	started, err := h.srv.StartSpike(context.Background(), sp.ID, budget, "sam")
	if err != nil {
		h.t.Fatalf("start spike: %v", err)
	}
	return started
}

// startSpikeQuiet starts a spike in the database exactly as StartSpike does,
// but never kicks the dispatcher, so a test can look at the plan, or finish
// the run by hand, before anything runs.
func (h *harness) startSpikeQuiet(sp *store.Spike, budget int64) (*store.Spike, *store.Dispatch) {
	h.t.Helper()
	ctx := context.Background()
	head := strings.TrimSpace(h.gitOut("rev-parse", "HEAD"))
	var started *store.Spike
	var d *store.Dispatch
	err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		started, err = store.StartSpike(ctx, tx, sp.ID, budget, "start screen", head, h.srv.spikeWorktreeRel(sp.ID), "sam")
		if err != nil {
			return err
		}
		d, err = store.EnqueueDispatch(ctx, tx, "run-spike", "spike-runner", "claude-sonnet-5", "spike", sp.ID, "run-spike:"+sp.ID.String())
		return err
	})
	if err != nil {
		h.t.Fatal(err)
	}
	// Left queued, nobody kicks it; mark it running so no scan can claim it,
	// and the reconciliation leaves a run that is still going alone.
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET state = 'running' WHERE id = $1`, d.ID); err != nil {
		h.t.Fatal(err)
	}
	d.State = "running"
	return started, d
}

// spikeSettled waits until the spike's run has ended and its worktree is gone.
func (h *harness) spikeSettled(sp *store.Spike) *store.Spike {
	h.t.Helper()
	h.eventually("the spike to end and its worktree to be discarded", func() bool {
		cur, err := store.GetSpike(context.Background(), h.srv.Store.Pool, sp.ID)
		return err == nil && cur.State == store.SpikeEnded && cur.WorktreeRemovedAt != nil
	})
	return h.getSpike(sp.ID)
}

// sweepUntilSettled does what the heartbeat does until the spike has ended.
func (h *harness) sweepUntilSettled(sp *store.Spike) *store.Spike {
	h.t.Helper()
	h.eventually("the spike to end and its worktree to be discarded", func() bool {
		h.srv.Dispatcher.RetrySweep(context.Background())
		cur, err := store.GetSpike(context.Background(), h.srv.Store.Pool, sp.ID)
		return err == nil && cur.State == store.SpikeEnded && cur.WorktreeRemovedAt != nil
	})
	return h.getSpike(sp.ID)
}

// findingsOf is a spike's findings document and its text.
func (h *harness) findingsOf(sp *store.Spike) (*store.Document, string) {
	h.t.Helper()
	doc, err := store.CurrentDocForOwner(context.Background(), h.srv.Store.Pool, "findings", "spike", sp.ID)
	if err != nil {
		h.t.Fatalf("findings of %s: %v", sp.PublicID, err)
	}
	return doc, h.readFile(doc.Path)
}

// runPrompt is what the spike's first run was told, from its transcript.
func (h *harness) runPrompt(sp *store.Spike) string {
	h.t.Helper()
	runs := h.runsFor("spike", sp.ID)
	if len(runs) == 0 {
		h.t.Fatalf("%s has no run", sp.PublicID)
	}
	for _, e := range h.transcript(runs[0].ID, 1) {
		if e.Kind == store.EntryPrompt {
			return e.Content
		}
	}
	h.t.Fatal("the run's transcript has no prompt")
	return ""
}

// saveCall scripts a save_findings call with Answer and What we found.
func (h *harness) saveCall(usage provider.Usage, answer, found string) {
	h.t.Helper()
	h.mock.RespondToolUse("save_findings", findingsJSON("## Answer\n\n"+answer+"\n\n## What we found\n\n"+found+"\n"), usage)
}

// findingsJSON is the input of save_findings and finish_spike.
func findingsJSON(body string) string {
	raw, _ := json.Marshal(map[string]string{"findings": body})
	return string(raw)
}

const goodFindings = "## Answer\n\nYes: the API honours the header.\n\n## What we found\n\nThe header is read in one place, and a test shows it.\n\n## How we found out\n\nBy reading the handler and calling it.\n"

// worktreeList is `git worktree list` for the main checkout.
func (h *harness) worktreeList() string { return h.gitOut("worktree", "list") }

// spikeDir is a spike's worktree directory.
func (h *harness) spikeDir(sp *store.Spike) string {
	return filepath.Join(h.root, ".subutai", "worktrees", "spk-"+sp.ID.String()[len(sp.ID.String())-6:])
}

// editRole rewrites one line of a role file in the test project.
func (h *harness) editRole(role, old, new string) {
	h.t.Helper()
	p := filepath.Join(h.root, ".subutai", "roles", role+".yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		h.t.Fatal(err)
	}
	out := strings.Replace(string(data), old, new, 1)
	if out == string(data) {
		h.t.Fatalf("role %s no longer carries %q", role, old)
	}
	if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func i64(n int64) *int64 { return &n }

// ---- FR-1: creating ----

func TestCreatingASpike(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	h.createFeature("pf", "login", "Login", "Sign in.")
	feat, err := h.srv.featureByPath(ctx, "pf/login")
	if err != nil {
		t.Fatal(err)
	}

	// From the UI on an initiative and on a feature, and from MCP, in order.
	s1, err := h.srv.CreateSpike(ctx, "initiative", in, "Does the API honour the header?", nil, "sam", "ui")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := h.srv.CreateSpike(ctx, "feature", feat.ID, "Is bcrypt fast enough here?", i64(50_000), "sam", "ui")
	if err != nil {
		t.Fatal(err)
	}
	s3, err := h.srv.CreateSpike(ctx, "initiative", in, "  Can we drop the cache?  ", nil, h.srv.mcpActor(), "mcp")
	if err != nil {
		t.Fatal(err)
	}
	for i, sp := range []*store.Spike{s1, s2, s3} {
		want := []string{"SPK-001", "SPK-002", "SPK-003"}[i]
		if sp.PublicID != want || sp.State != store.SpikeIdea || sp.InitiativeID != in {
			t.Errorf("spike %d = %s %s on %s, want %s idea on pf", i+1, sp.PublicID, sp.State, sp.InitiativeID, want)
		}
		if got := h.auditKinds("spike.created", sp.ID); got != 1 {
			t.Errorf("%s has %d spike.created audit rows, want 1", sp.PublicID, got)
		}
	}
	if s2.FeatureID == nil || *s2.FeatureID != feat.ID || s1.FeatureID != nil {
		t.Errorf("owners: s1 feature %v, s2 feature %v", s1.FeatureID, s2.FeatureID)
	}
	if s2.BudgetOverride == nil || *s2.BudgetOverride != 50_000 {
		t.Errorf("s2 budget override = %v, want 50000", s2.BudgetOverride)
	}
	if s1.CreatedVia != "ui" || s3.CreatedVia != "mcp" || s3.CreatedBy != "chat-agent" {
		t.Errorf("created via/by: %s %s/%s", s1.CreatedVia, s3.CreatedVia, s3.CreatedBy)
	}
	if s3.Question != "Can we drop the cache?" {
		t.Errorf("the question wasn't trimmed: %q", s3.Question)
	}
	if n := len(h.runsFor("spike", s1.ID)); n != 0 {
		t.Errorf("creating a spike dispatched %d runs, want none", n)
	}

	// Refusals, each a sentence, and the next spike still gets the next number.
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE features SET state = 'done' WHERE id = $1`, feat.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	h.spikeInitiative("old")
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE initiatives SET archived = true WHERE slug = 'old'`); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		ownerType string
		owner     uuid.UUID
		question  string
		budget    *int64
		want      string
	}{
		{"blank", "initiative", in, "   ", nil, "A spike needs a question: one sentence saying what it should find out."},
		{"too long", "initiative", in, strings.Repeat("q", 501), nil, "A spike's question is at most 500 characters; this one is 501."},
		{"zero budget", "initiative", in, "Fine?", i64(0), "A spike's budget is a positive whole number of tokens."},
		{"negative budget", "initiative", in, "Fine?", i64(-5), "A spike's budget is a positive whole number of tokens."},
		{"done feature", "feature", feat.ID, "Fine?", nil, "That feature is done, so a spike can't be created on it."},
		{"archived initiative", "initiative", h.initiativeID("old"), "Fine?", nil, "That initiative is archived, so a spike can't be created on it."},
		{"a document", "document", in, "Fine?", nil, "A spike is created on an initiative or a feature."},
	}
	for _, c := range cases {
		_, err := h.srv.CreateSpike(ctx, c.ownerType, c.owner, c.question, c.budget, "sam", "ui")
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
	s4, err := h.srv.CreateSpike(ctx, "initiative", in, "Does the next number follow?", nil, "sam", "ui")
	if err != nil || s4.PublicID != "SPK-004" {
		t.Fatalf("after the refusals the next spike = %v, %v; want SPK-004", s4, err)
	}

	// The database refuses a row that breaks each named check.
	checks := map[string]string{
		"spikes_state":                   `INSERT INTO spikes (id, initiative_id, question, state, created_by, created_via) VALUES (gen_random_uuid(), $1, 'q', 'bogus', 'x', 'ui')`,
		"spikes_closed_as":               `INSERT INTO spikes (id, initiative_id, question, state, created_by, created_via) VALUES (gen_random_uuid(), $1, 'q', 'closed', 'x', 'ui')`,
		"spikes_started":                 `INSERT INTO spikes (id, initiative_id, question, state, created_by, created_via) VALUES (gen_random_uuid(), $1, 'q', 'running', 'x', 'ui')`,
		"spikes_budget_positive":         `INSERT INTO spikes (id, initiative_id, question, budget_override, created_by, created_via) VALUES (gen_random_uuid(), $1, 'q', 0, 'x', 'ui')`,
		"spikes_tokens_nonneg":           `INSERT INTO spikes (id, initiative_id, question, tokens_used, created_by, created_via) VALUES (gen_random_uuid(), $1, 'q', -1, 'x', 'ui')`,
		"spikes_question":                `INSERT INTO spikes (id, initiative_id, question, created_by, created_via) VALUES (gen_random_uuid(), $1, '   ', 'x', 'ui')`,
		"spikes_closed_unrun_unanswered": `INSERT INTO spikes (id, initiative_id, question, state, closed_as, created_by, created_via) VALUES (gen_random_uuid(), $1, 'q', 'closed', 'answered', 'x', 'ui')`,
		"spikes_not_self_follow":         `UPDATE spikes SET follows_id = id WHERE initiative_id = $1`,
		"spikes_ended_how":               `INSERT INTO spikes (id, initiative_id, question, state, closed_as, started_at, created_by, created_via) VALUES (gen_random_uuid(), $1, 'q', 'closed', 'answered', now(), 'x', 'ui')`,
		"spikes_created_via":             `INSERT INTO spikes (id, initiative_id, question, created_by, created_via) VALUES (gen_random_uuid(), $1, 'q', 'x', 'api')`,
	}
	for name, sql := range checks {
		_, err := h.srv.Store.Pool.Exec(ctx, sql, in)
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("check %s: err = %v, want it named", name, err)
		}
	}

	// Closing an idea leaves it closed, unanswered, with no findings.
	closed, err := h.srv.CloseSpike(ctx, s4.ID, store.SpikeUnanswered, nil, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if closed.State != store.SpikeClosed || closed.ClosedAs != store.SpikeUnanswered || closed.ClosedBy != "sam" {
		t.Errorf("closed idea = %s as %q by %q", closed.State, closed.ClosedAs, closed.ClosedBy)
	}
	if _, err := store.CurrentDocForOwner(ctx, h.srv.Store.Pool, "findings", "spike", s4.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a spike closed without running has findings: %v", err)
	}
	if closed.WorktreePath != "" || closed.StartedAt != nil {
		t.Errorf("a spike closed without running has a worktree %q or a start", closed.WorktreePath)
	}
}

// ---- FR-2: the findings ----

func TestWritingFindings(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?\nAnd the cookie?", nil)

	full := "## Answer\n\nYes.\n\n## What we found\n\nThe handler reads it.\n\n## How we found out\n\nBy reading.\n\n## What to do next\n\nShip it.\n"
	own := "## Question\n\nA different question.\n\n" + full + "\n## How this spike ended\n\nThe agent made this up.\n"
	disordered := "## What we found\n\nThe handler reads it.\n\n## Answer\n\nYes.\n\n## Notes\n\nAn extra section.\n"
	bare := "Just a paragraph with no headings at all.\n"
	endings := []struct {
		name string
		end  findingsEnd
		want string
	}{
		{"concluded", findingsEnd{How: store.SpikeConcluded, Used: 31004, Budget: 40000},
			"The agent reached a conclusion. It used 31,004 of its 40,000 tokens."},
		{"over the budget", findingsEnd{How: store.SpikeBudget, Used: 41210, Budget: 40000},
			"The spike stopped at its budget. It used 41,210 tokens, which is more than its budget of 40,000. The findings above are what it had saved by then."},
		{"under the budget", findingsEnd{How: store.SpikeBudget, Used: 900, Budget: 1000},
			"The spike stopped at its budget, because its next step would have gone over. It used 900 of its 1,000 tokens. The findings above are what it had saved by then."},
		{"turn limit", findingsEnd{How: store.SpikeTurnLimit, Used: 18220, Budget: 40000, TurnCap: 40},
			"The spike stopped at its turn limit of 40 turns, having used 18,220 of its 40,000 tokens. The findings above are what it had saved by then."},
		{"failed", findingsEnd{How: store.SpikeFailed, Note: "provider unavailable after 4 tries.\nsecond line", Used: 2100, Budget: 40000},
			"The spike's run failed: provider unavailable after 4 tries. second line. It used 2,100 of its 40,000 tokens. The findings above are what it had saved by then."},
	}
	drafts := map[string]string{"full": full, "empty": "", "own sections": own, "disordered": disordered, "bare": bare}
	for _, e := range endings {
		for dname, draft := range drafts {
			name := e.name + "/" + dname
			text := h.srv.buildFindings(sp.PublicID, sp.Question, "pf", draft, e.end, false)
			if err := h.srv.validateFindings(text); err != nil {
				t.Errorf("%s: doesn't validate: %v\n%s", name, err, text)
				continue
			}
			if got := strings.Count(text, "## How this spike ended"); got != 1 {
				t.Errorf("%s: How this spike ended appears %d times", name, got)
			}
			if i := strings.Index(text, "## How this spike ended"); !strings.HasSuffix(strings.TrimSpace(text[i:]), e.want) {
				t.Errorf("%s: the ending section isn't last, or says the wrong thing:\n%s", name, text[i:])
			}
			if got := strings.Count(text, "## Question"); got != 1 {
				t.Errorf("%s: Question appears %d times", name, got)
			}
			if !strings.Contains(text, "\n## Question\n\nDoes the API honour the header? And the cookie?\n") || strings.Contains(text, "A different question") {
				t.Errorf("%s: the question isn't the row's:\n%s", name, text)
			}
			if !strings.Contains(text, `title: "SPK-001: Does the API honour the header? And the cookie?"`) {
				t.Errorf("%s: the title is wrong:\n%s", name, text)
			}
			if strings.Contains(text, "The agent made this up") {
				t.Errorf("%s: the draft's own ending survived", name)
			}
			// Sections come in the template's order, whatever the draft's.
			if a, f := strings.Index(text, "## Answer"), strings.Index(text, "## What we found"); a < 0 || f < a {
				t.Errorf("%s: Answer and What we found are out of order", name)
			}
		}
	}

	// The fill-ins for an empty draft, by ending.
	for _, c := range []struct{ how, want string }{
		{store.SpikeBudget, "Not answered: the spike stopped at its budget before it reached an answer."},
		{store.SpikeTurnLimit, "Not answered: the spike stopped at its turn limit before it reached an answer."},
		{store.SpikeFailed, "Not answered: the spike's run failed before it reached an answer."},
	} {
		text := h.srv.buildFindings(sp.PublicID, sp.Question, "pf", "", findingsEnd{How: c.how, Budget: 10}, false)
		if !strings.Contains(text, "## Answer\n\n"+c.want+"\n") || !strings.Contains(text, "## What we found\n\nNothing was saved before the run stopped.\n") {
			t.Errorf("%s: the fill-ins are wrong:\n%s", c.how, text)
		}
	}
	// A placeholder Answer counts as empty; the draft's other text is kept.
	text := h.srv.buildFindings(sp.PublicID, sp.Question, "pf", "## Answer\n\nTODO\n\n## What we found\n\nA fact.\n", findingsEnd{How: store.SpikeBudget, Budget: 10}, false)
	if !strings.Contains(text, "Not answered: the spike stopped at its budget") || !strings.Contains(text, "A fact.") {
		t.Errorf("a placeholder answer wasn't filled in:\n%s", text)
	}
	// A bare draft is what was found. The heading it lacked is made for it.
	text = h.srv.buildFindings(sp.PublicID, sp.Question, "pf", bare, findingsEnd{How: store.SpikeFailed, Budget: 10}, false)
	if !strings.Contains(text, "## What we found\n\nJust a paragraph with no headings at all.\n") {
		t.Errorf("a draft without headings wasn't kept as what was found:\n%s", text)
	}

	// A run that stopped can't be sent back to fix its words: text that looks
	// like an unfinished template is rewritten, and the document validates.
	risky := "## Answer\n\nTODO later\n\n## What we found\n\nThe template says {{name}} and a TODO remains.\n"
	strict := h.srv.buildFindings(sp.PublicID, sp.Question, "pf", risky, findingsEnd{How: store.SpikeBudget, Budget: 10}, false)
	if h.srv.validateFindings(strict) == nil {
		t.Error("the unfinished-looking text should fail validation when it isn't rewritten")
	}
	soft := h.srv.buildFindings(sp.PublicID, sp.Question, "pf", risky, findingsEnd{How: store.SpikeBudget, Budget: 10}, true)
	if err := h.srv.validateFindings(soft); err != nil {
		t.Errorf("the rewritten findings don't validate: %v\n%s", err, soft)
	}

	// A code fence the draft left open doesn't swallow the ending.
	open := "## Answer\n\nYes.\n\n## What we found\n\n```go\nfunc main() {\n"
	text = h.srv.buildFindings(sp.PublicID, sp.Question, "pf", open, findingsEnd{How: store.SpikeBudget, Budget: 10}, false)
	if err := h.srv.validateFindings(text); err != nil {
		t.Errorf("an open fence broke the findings: %v\n%s", err, text)
	}

	// The leak paragraph follows the ending sentence.
	text = h.srv.buildFindings(sp.PublicID, sp.Question, "pf", full, findingsEnd{How: store.SpikeConcluded, Used: 5, Budget: 10, Refs: []string{"keep-this"}}, false)
	if !strings.HasSuffix(strings.TrimSpace(text), "The agent reached a conclusion. It used 5 of its 10 tokens.\n\n"+
		"Code from this spike was kept on `keep-this`, outside its working copy. Subutai hasn't deleted it; the Inbox asks what to do.") {
		t.Errorf("the leak paragraph is missing or misplaced:\n%s", text)
	}
}

// ---- FR-4: the prompt, the draft, the worktree ----

func TestSpikePromptCarriesDecisionsAndQuestion(t *testing.T) {
	h := newHarness(t)
	h.spikeInitiative("pf")
	h.spikeInitiative("other")
	h.acceptDecision("Use plain SQL", "project", "Every query is plain SQL.", "Simple.")
	h.acceptDecision("Cache nothing in pf", h.initiativePublicID("pf"), "The pf initiative caches nothing.", "Staleness.")
	h.acceptDecision("Other initiative rule", h.initiativePublicID("other"), "The other initiative uses queues.", "Scale.")

	sp := h.newSpike(h.initiativeID("pf"), "Does the API honour the header?", nil)
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(sp, 40_000)
	sp = h.spikeSettled(sp)
	if sp.EndedHow != store.SpikeConcluded {
		t.Fatalf("ended how = %q, want concluded", sp.EndedHow)
	}

	prompt := h.runPrompt(sp)
	for _, want := range []string{
		"Every query is plain SQL.", "The pf initiative caches nothing.",
		"# The question\n\nDoes the API honour the header?",
		"# Your budget\n\nYou have 40,000 tokens. The run stops without warning when it reaches this many tokens. " +
			"Save your findings with `save_findings` early and often: whatever you have saved when it stops is what is kept.",
		"# Your working copy", "never in a file",
		"# Where it came from\n\nThis spike was written down on the initiative",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "The other initiative uses queues.") {
		t.Error("a sibling initiative's decision reached the spike's prompt")
	}
	if strings.Contains(prompt, "# What the last spike found") || strings.Contains(prompt, "# What you have saved so far") {
		t.Error("a first run was told about earlier findings or a draft")
	}
	if i, j := strings.Index(prompt, "Every query is plain SQL."), strings.Index(prompt, "# The question"); i < 0 || j < i {
		t.Error("the surfaced decisions don't come first")
	}
	// The system prompt is the role's identity and the skill.
	runs := h.runsFor("spike", sp.ID)
	var system string
	for _, e := range h.transcript(runs[0].ID, 1) {
		if e.Kind == store.EntrySystem {
			system = e.Content
		}
	}
	if !strings.Contains(system, "You are a spike runner") || !strings.Contains(system, "# Procedure") {
		t.Errorf("the system prompt lacks the role or the skill:\n%s", system)
	}

	// A spike that asks again is given what the last one found.
	next, err := h.srv.CloseSpike(context.Background(), sp.ID, SpikeCloseAgain, i64(30_000), "sam")
	if err != nil {
		t.Fatal(err)
	}
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(next, 0)
	next = h.spikeSettled(next)
	again := h.runPrompt(next)
	if !strings.Contains(again, "# What the last spike found") || !strings.Contains(again, "The header is read in one place, and a test shows it.") ||
		!strings.Contains(again, "asks again after "+sp.PublicID) {
		t.Errorf("the second spike wasn't given the first's findings:\n%s", again)
	}
	if !strings.Contains(again, "You have 30,000 tokens.") {
		t.Errorf("the second spike's budget isn't its override:\n%s", again)
	}
}

func TestSaveFindingsKeepsADraft(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?", nil)
	started, d := h.startSpikeQuiet(sp, 100_000)
	plan, err := h.srv.Plan(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	wt := plan.ToolCtx.WorktreeRoot
	mainBefore := h.gitOut("status", "--porcelain")
	untrackedBefore := h.gitOut("ls-files", "--others", "--exclude-standard")
	headBefore := h.headSHA()
	treeBefore, _ := gitIn(wt, "status", "--porcelain")

	input := json.RawMessage(findingsJSON("## Answer\n\nMaybe.\n\n## What we found\n\nA first note.\n"))
	if got, isErr := h.srv.Execute(ctx, plan.ToolCtx, "save_findings", input); isErr || got != "Saved. If the run stops now, this is what is kept." {
		t.Fatalf("save_findings = %q (error %v)", got, isErr)
	}
	cur := h.getSpike(started.ID)
	if !strings.Contains(cur.Draft, "A first note.") || cur.DraftSavedAt == nil {
		t.Errorf("the draft wasn't kept: %q saved at %v", cur.Draft, cur.DraftSavedAt)
	}
	// A second save replaces the first.
	input = json.RawMessage(findingsJSON("## Answer\n\nYes.\n\n## What we found\n\nA second note.\n"))
	h.srv.Execute(ctx, plan.ToolCtx, "save_findings", input)
	if cur = h.getSpike(started.ID); strings.Contains(cur.Draft, "first note") || !strings.Contains(cur.Draft, "A second note.") {
		t.Errorf("the second save didn't replace the first: %q", cur.Draft)
	}
	// No file changes in the main checkout, and the worktree's tree is unchanged.
	if got := h.gitOut("status", "--porcelain"); got != mainBefore || h.headSHA() != headBefore {
		t.Errorf("the main checkout changed:\n%s", got)
	}
	if treeAfter, _ := gitIn(wt, "status", "--porcelain"); treeAfter != treeBefore {
		t.Errorf("the worktree changed:\n%s", treeAfter)
	}
	if got := h.gitOut("ls-files", "--others", "--exclude-standard"); got != untrackedBefore {
		t.Errorf("save_findings wrote files:\n%s", got)
	}

	// From anything but a spike's run, it is refused.
	other := &toolhost.Context{WorktreeRoot: wt, Profile: map[string]bool{"save_findings": true}}
	if got, isErr := h.srv.Execute(ctx, other, "save_findings", input); !isErr || got != "save_findings is only for a spike's run." {
		t.Errorf("from another purpose: %q (error %v)", got, isErr)
	}
	// A role that wasn't offered it can't call it through the loop either.
	if got, isErr := (&toolhost.Context{WorktreeRoot: wt, SpikeID: started.ID.String()}).InProfile("save_findings"), false; got || isErr {
		t.Error("a context without the tool in its profile admitted it")
	}
	// Blank findings aren't saved.
	if got, isErr := h.srv.Execute(ctx, plan.ToolCtx, "save_findings", json.RawMessage(findingsJSON("  "))); !isErr ||
		!strings.HasPrefix(got, "There is nothing to save") {
		t.Errorf("blank findings: %q (error %v)", got, isErr)
	}
	// Once the spike has ended, a late save is refused and changes nothing.
	if err := h.srv.EndSpike(ctx, started.ID, store.SpikeBudget, ""); err != nil {
		t.Fatal(err)
	}
	late := json.RawMessage(findingsJSON("## Answer\n\nToo late.\n"))
	if got, isErr := h.srv.Execute(ctx, plan.ToolCtx, "save_findings", late); !isErr || !strings.Contains(got, "already ended") {
		t.Errorf("a late save: %q (error %v)", got, isErr)
	}
	if got := h.getSpike(started.ID).Draft; strings.Contains(got, "Too late") {
		t.Errorf("a late save changed the draft: %q", got)
	}
}

func TestSpikeWorktreeIsMadeByThePlanner(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	branchesBefore := h.gitOut("branch", "--list")
	sp := h.newSpike(in, "Does the API honour the header?", nil)
	started, d := h.startSpikeQuiet(sp, 100_000)
	abs := h.srv.worktreeAbs(started.WorktreePath)
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("StartSpike made the worktree itself (stat: %v)", err)
	}
	if !strings.HasSuffix(started.WorktreePath, filepath.Join(".subutai", "worktrees", "spk-"+started.ID.String()[len(started.ID.String())-6:])) {
		t.Errorf("worktree path = %q", started.WorktreePath)
	}

	plan, err := h.srv.Plan(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if head, _ := gitIn(abs, "rev-parse", "HEAD"); strings.TrimSpace(head) != started.BaseCommit {
		t.Errorf("worktree HEAD = %s, want the base commit %s", strings.TrimSpace(head), started.BaseCommit)
	}
	if _, err := gitIn(abs, "symbolic-ref", "-q", "HEAD"); err == nil {
		t.Error("the worktree is on a branch, not detached")
	}
	if got := h.gitOut("branch", "--list"); got != branchesBefore {
		t.Errorf("a branch was made:\nbefore %q\nafter  %q", branchesBefore, got)
	}
	var rows int
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM worktrees`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("worktrees rows = %d (%v), want none", rows, err)
	}
	if !strings.Contains(h.worktreeList(), "spk-") {
		t.Errorf("git doesn't list the worktree:\n%s", h.worktreeList())
	}

	// The plan: the finish tool is the outcome, the budget is the row's, and
	// the tool context names the worktree and the spike, no feature or task.
	if plan.OutcomeTool != "finish_spike" || plan.Budget == nil || plan.Budget.Limit != 100_000 ||
		len(plan.Budget.EarlyTools) != 1 || plan.Budget.EarlyTools[0] != "save_findings" {
		t.Errorf("plan = outcome %q, budget %+v", plan.OutcomeTool, plan.Budget)
	}
	names := map[string]bool{}
	for _, tool := range plan.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"read_file", "list_files", "edit_file", "write_file", "run_command", "save_findings", "finish_spike"} {
		if !names[want] {
			t.Errorf("the plan doesn't offer %s", want)
		}
	}
	if names["report_bug"] {
		t.Error("the spike runner was offered report_bug")
	}
	if plan.ToolCtx.SpikeID != started.ID.String() || plan.ToolCtx.FeatureID != "" || plan.ToolCtx.TaskID != "" ||
		filepath.Clean(plan.ToolCtx.WorktreeRoot) != filepath.Clean(abs) {
		t.Errorf("tool context = %+v", plan.ToolCtx)
	}
	if plan.TurnCap != 40 {
		t.Errorf("turn cap = %d, want the role's 40", plan.TurnCap)
	}
	// The budget callbacks read and write the row.
	if total, err := plan.Budget.Add(ctx, 250); err != nil || total != 250 {
		t.Errorf("Add = %d, %v", total, err)
	}
	if total, err := plan.Budget.Total(ctx); err != nil || total != 250 {
		t.Errorf("Total = %d, %v", total, err)
	}
	// finish_spike is checked against the findings template.
	if err := plan.ValidateOutcome(json.RawMessage(findingsJSON(goodFindings))); err != nil {
		t.Errorf("good findings refused: %v", err)
	}
	for name, body := range map[string]string{
		"empty":       "",
		"no answer":   "## What we found\n\nA thing.\n",
		"blank found": "## Answer\n\nYes.\n\n## What we found\n\n",
		"placeholder": "## Answer\n\nTBD\n\n## What we found\n\nA thing.\n",
		"unfinished":  "## Answer\n\nYes.\n\n## What we found\n\nA {{thing}} to do.\n",
		"no headings": "Just text.",
	} {
		if err := plan.ValidateOutcome(json.RawMessage(findingsJSON(body))); err == nil {
			t.Errorf("%s findings were accepted", name)
		}
	}

	// A second plan reuses the worktree; one whose directory is gone makes it again.
	if err := os.WriteFile(filepath.Join(abs, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.Plan(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(abs, "scratch.txt")); err != nil {
		t.Errorf("a second plan remade a live worktree: %v", err)
	}
	if err := os.RemoveAll(abs); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.Plan(ctx, d); err != nil {
		t.Fatalf("the plan couldn't remake a worktree whose directory was lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		t.Errorf("the worktree wasn't remade: %v", err)
	}

	// The planner refuses a spike that isn't running.
	if err := h.srv.EndSpike(ctx, started.ID, store.SpikeBudget, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.Plan(ctx, d); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("a plan for an ended spike: %v", err)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Errorf("the ended spike's worktree is still there: %v", err)
	}
}

// ---- FR-5: the budget and the hard stop ----

func TestSpikeStopsHardAtItsBudget(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?", nil)
	// Eight scripted calls of 150 tokens, none of which finishes. With a budget
	// of 1,000 exactly six reach the provider: after the sixth the total is 900,
	// and 900 + 150 would pass the budget.
	for i := 1; i <= 8; i++ {
		h.saveCall(tiny, "Not sure yet.", "Step "+string(rune('0'+i))+" of the investigation.")
	}
	h.startSpike(sp, 1000)
	sp = h.spikeSettled(sp)

	if calls := len(h.mock.Requests); calls != 6 {
		t.Errorf("%d calls reached the provider, want exactly 6", calls)
	}
	if left := h.mock.Remaining(); left != 2 {
		t.Errorf("%d scripted steps left, want 2 unused", left)
	}
	if sp.EndedHow != store.SpikeBudget || sp.TokensUsed != 900 || sp.TokenBudget == nil || *sp.TokenBudget != 1000 {
		t.Errorf("spike = ended %q, used %d, budget %v; want budget, 900, 1000", sp.EndedHow, sp.TokensUsed, sp.TokenBudget)
	}
	doc, text := h.findingsOf(sp)
	if !strings.Contains(text, "Step 6 of the investigation.") || strings.Contains(text, "Step 7") {
		t.Errorf("the findings don't hold the last draft:\n%s", text)
	}
	if !strings.Contains(text, "The spike stopped at its budget, because its next step would have gone over. It used 900 of its 1,000 tokens.") {
		t.Errorf("the findings lack the ending section:\n%s", text)
	}
	if doc.State != "draft" || doc.OwnerType != "spike" || doc.PublicID != "SPK-001-findings" {
		t.Errorf("findings = %s %s owned by %s", doc.PublicID, doc.State, doc.OwnerType)
	}

	runs := h.runsFor("spike", sp.ID)
	if len(runs) != 1 || runs[0].State != "succeeded" {
		t.Fatalf("runs = %+v", runs)
	}
	if string(runs[0].Outcome) != `{"ended": "budget"}` && string(runs[0].Outcome) != `{"ended":"budget"}` {
		t.Errorf("the run's outcome = %s", runs[0].Outcome)
	}
	es := h.transcript(runs[0].ID, 1)
	last := es[len(es)-1]
	if last.Kind != "stop" || last.Content != "The run stopped here because it reached its budget of 1,000 tokens." {
		t.Errorf("the transcript ends with %s %q", last.Kind, last.Content)
	}
}

func TestSpikeSavesOnTheTurnThatCrossesTheBudget(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?", nil)
	h.saveCall(provider.Usage{Input: 250, Output: 50}, "Perhaps.", "The first note.")
	// The second call is allowed (300 + 300 fits in 1,000) but turns out larger,
	// and crosses the budget. Its save is kept; its other tool isn't run.
	h.mock.RespondParallelToolUse([]struct{ Tool, InputJSON string }{
		{"save_findings", findingsJSON("## Answer\n\nYes.\n\n## What we found\n\nThe crossing note.\n")},
		{"list_files", `{"dir":""}`},
	}, provider.Usage{Input: 700, Output: 100})
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny) // never reached
	h.startSpike(sp, 1000)
	sp = h.spikeSettled(sp)

	if sp.EndedHow != store.SpikeBudget || sp.TokensUsed != 1100 {
		t.Errorf("ended %q having used %d, want budget and 1100", sp.EndedHow, sp.TokensUsed)
	}
	if !strings.Contains(sp.Draft, "The crossing note.") {
		t.Errorf("the save on the crossing turn was lost: %q", sp.Draft)
	}
	_, text := h.findingsOf(sp)
	if !strings.Contains(text, "The crossing note.") ||
		!strings.Contains(text, "It used 1,100 tokens, which is more than its budget of 1,000.") {
		t.Errorf("the findings are wrong:\n%s", text)
	}
	runs := h.runsFor("spike", sp.ID)
	var listed int
	if err := h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM tool_calls WHERE dispatch_id = $1 AND tool = 'list_files'`, runs[0].ID).Scan(&listed); err != nil || listed != 0 {
		t.Errorf("the other tool on the crossing turn ran (%d, %v)", listed, err)
	}
	if h.mock.Remaining() != 1 || len(h.mock.Requests) != 2 {
		t.Errorf("calls = %d, left = %d; want 2 calls and 1 unused", len(h.mock.Requests), h.mock.Remaining())
	}
}

func TestSpikeFinishOnTheTurnThatCrossesTheBudget(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?", nil)
	h.saveCall(provider.Usage{Input: 250, Output: 50}, "Perhaps.", "The first note.")
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), provider.Usage{Input: 700, Output: 100})
	h.startSpike(sp, 1000)
	sp = h.spikeSettled(sp)

	if sp.EndedHow != store.SpikeConcluded || sp.TokensUsed != 1100 {
		t.Errorf("ended %q having used %d, want concluded and 1100", sp.EndedHow, sp.TokensUsed)
	}
	_, text := h.findingsOf(sp)
	if !strings.Contains(text, "The header is read in one place, and a test shows it.") ||
		!strings.Contains(text, "The agent reached a conclusion. It used 1,100 of its 1,000 tokens.") {
		t.Errorf("the findings are wrong:\n%s", text)
	}
	runs := h.runsFor("spike", sp.ID)
	es := h.transcript(runs[0].ID, 1)
	if last := es[len(es)-1]; last.Kind == "stop" {
		t.Errorf("a run that concluded ends with a stop entry: %q", last.Content)
	}
}

func TestSpikeTurnLimitEndsTheRun(t *testing.T) {
	h := newHarness(t)
	h.editRole("spike-runner", "turn_cap: 40", "turn_cap: 3")
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?", nil)
	for i := 1; i <= 5; i++ {
		h.saveCall(tiny, "Not sure.", "Turn "+string(rune('0'+i))+".")
	}
	h.startSpike(sp, 0)
	sp = h.spikeSettled(sp)

	if sp.EndedHow != store.SpikeTurnLimit {
		t.Fatalf("ended %q, want turn_limit", sp.EndedHow)
	}
	if sp.TokensUsed != 450 || len(h.mock.Requests) != 3 {
		t.Errorf("used %d tokens in %d calls, want 450 in 3", sp.TokensUsed, len(h.mock.Requests))
	}
	if sp.TokenBudget == nil || *sp.TokenBudget != 1_000_000 {
		t.Errorf("budget = %v, want the project default of 1,000,000", sp.TokenBudget)
	}
	_, text := h.findingsOf(sp)
	if !strings.Contains(text, "The spike stopped at its turn limit of 3 turns, having used 450 of its 1,000,000 tokens.") ||
		!strings.Contains(text, "Turn 3.") {
		t.Errorf("the findings are wrong:\n%s", text)
	}
	runs := h.runsFor("spike", sp.ID)
	if runs[0].State != "succeeded" {
		t.Errorf("the run is %s, want succeeded: the turn limit isn't a failure", runs[0].State)
	}
	es := h.transcript(runs[0].ID, 1)
	if last := es[len(es)-1]; last.Kind != "stop" || last.Content != "The run stopped here because it reached its turn limit of 3 turns." {
		t.Errorf("the transcript ends with %s %q", last.Kind, last.Content)
	}
}

func TestSpikeBudgetCarriesAcrossAttempts(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?", nil)
	// The first attempt saves, then the provider fails for good. Its retry
	// is told what was saved, and the budget counts both attempts.
	h.saveCall(tiny, "Perhaps.", "The note from the first attempt.")
	h.mock.Fail(errors.New("the provider is down"))
	for i := 1; i <= 7; i++ {
		h.saveCall(tiny, "Perhaps.", "A note from the retry, number "+string(rune('0'+i))+".")
	}
	h.startSpike(sp, 1000)
	sp = h.sweepUntilSettled(sp)

	runs := h.runsFor("spike", sp.ID)
	if len(runs) != 1 || runs[0].Attempt != 2 || runs[0].State != "succeeded" {
		t.Fatalf("runs = %+v, want one run on its second attempt", runs)
	}
	var retryPrompt string
	for _, e := range h.transcript(runs[0].ID, 2) {
		if e.Kind == store.EntryPrompt {
			retryPrompt = e.Content
		}
	}
	for _, want := range []string{"# What you have saved so far", "The note from the first attempt.",
		"An earlier attempt at this spike stopped. Build on these findings rather than starting again."} {
		if !strings.Contains(retryPrompt, want) {
			t.Errorf("the retry's prompt lacks %q:\n%s", want, retryPrompt)
		}
	}
	for _, e := range h.transcript(runs[0].ID, 1) {
		if e.Kind == store.EntryPrompt && strings.Contains(e.Content, "# What you have saved so far") {
			t.Error("the first attempt was told about a draft it hadn't saved yet")
		}
	}
	// 1 call + 1 failure in attempt 1 (150 tokens); the retry runs until the
	// next call wouldn't fit: 900 in all.
	if sp.EndedHow != store.SpikeBudget || sp.TokensUsed != 900 {
		t.Errorf("ended %q having used %d, want budget and 900", sp.EndedHow, sp.TokensUsed)
	}
	if got := len(h.mock.Requests); got != 7 {
		t.Errorf("%d calls reached the provider, want 7 (1, the failure, then 5)", got)
	}
	_, text := h.findingsOf(sp)
	if !strings.Contains(text, "number 5.") || strings.Contains(text, "number 6.") {
		t.Errorf("the findings don't hold the retry's last save:\n%s", text)
	}
}

// ---- FR-6: ending ----

func TestSpikeWorktreeIsDiscardedWhenItEnds(t *testing.T) {
	h := newHarness(t)
	h.editRole("spike-runner", "turn_cap: 40", "turn_cap: 2")
	in := h.spikeInitiative("pf")
	ctx := context.Background()

	endings := []struct {
		name   string
		how    string
		script func()
		start  int64
		sweep  bool
	}{
		{"concluded", store.SpikeConcluded, func() { h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny) }, 0, false},
		{"budget", store.SpikeBudget, func() {
			h.saveCall(provider.Usage{Input: 900, Output: 100}, "Maybe.", "Used it all.")
		}, 1000, false},
		{"turn limit", store.SpikeTurnLimit, func() {
			h.saveCall(tiny, "Maybe.", "One.")
			h.saveCall(tiny, "Maybe.", "Two.")
		}, 0, false},
		{"failed", store.SpikeFailed, func() {
			for i := 0; i < 3; i++ {
				h.mock.Fail(errors.New("the provider is down for good"))
			}
		}, 0, true},
	}
	for _, e := range endings {
		sp := h.newSpike(in, "Does it end well when it "+e.name+"?", nil)
		e.script()
		h.startSpike(sp, e.start)
		var done *store.Spike
		if e.sweep {
			done = h.sweepUntilSettled(sp)
		} else {
			done = h.spikeSettled(sp)
		}
		if done.EndedHow != e.how {
			t.Errorf("%s: ended %q", e.name, done.EndedHow)
		}
		if _, err := os.Stat(h.spikeDir(sp)); !os.IsNotExist(err) {
			t.Errorf("%s: the worktree directory is still there (%v)", e.name, err)
		}
		if got := h.worktreeList(); strings.Contains(got, "spk-") {
			t.Errorf("%s: git still lists the worktree:\n%s", e.name, got)
		}
		if done.WorktreeRemovedAt == nil {
			t.Errorf("%s: worktree_removed_at isn't set", e.name)
		}
		if got := h.auditKinds("spike.worktree_discarded", sp.ID); got != 1 {
			t.Errorf("%s: %d spike.worktree_discarded rows, want 1", e.name, got)
		}
		if got := h.auditKinds("spike.ended", sp.ID); got != 1 {
			t.Errorf("%s: %d spike.ended rows, want 1", e.name, got)
		}
		// The findings are committed, once, and the main checkout has nothing
		// else to show for the run.
		doc, _ := h.findingsOf(sp)
		if out := h.gitOut("log", "--format=%s", "-1", "--", doc.Path); !strings.HasPrefix(strings.TrimSpace(out), sp.PublicID+": findings (") {
			t.Errorf("%s: the findings' commit is %q", e.name, out)
		}
		if out := h.gitOut("status", "--porcelain", "--", doc.Path); strings.TrimSpace(out) != "" {
			t.Errorf("%s: the findings weren't committed: %s", e.name, out)
		}
		h.quiet()
		_ = ctx
	}
}

func TestExhaustedSpikeEndsWithoutRetryQuestion(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does it fail well?", nil)
	for i := 0; i < 3; i++ {
		h.mock.Fail(errors.New("provider says no"))
	}
	h.startSpike(sp, 0)
	sp = h.sweepUntilSettled(sp)
	// The run is cancelled just after the spike ends, so wait for that too.
	h.eventually("the exhausted run to be cancelled", func() bool {
		runs := h.runsFor("spike", sp.ID)
		return len(runs) == 1 && runs[0].State == "cancelled"
	})

	if sp.EndedHow != store.SpikeFailed || !strings.Contains(sp.EndNote, "provider says no") {
		t.Errorf("ended %q with note %q", sp.EndedHow, sp.EndNote)
	}
	runs := h.runsFor("spike", sp.ID)
	if len(runs) != 1 || runs[0].State != "cancelled" {
		t.Errorf("the run is %+v, want cancelled", runs)
	}
	var failures int
	if err := h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM checkpoints WHERE kind = 'dispatch-failure'`).Scan(&failures); err != nil || failures != 0 {
		t.Errorf("%d dispatch-failure checkpoints, want none (%v)", failures, err)
	}
	if got := len(h.pendingByKind("dispatch-failure")); got != 0 {
		t.Errorf("%d retry questions are pending", got)
	}
	// Nothing is left to republish: more sweeps change nothing.
	for i := 0; i < 3; i++ {
		h.srv.Dispatcher.RetrySweep(context.Background())
	}
	time.Sleep(100 * time.Millisecond)
	if got := h.auditKinds("spike.ended", sp.ID); got != 1 {
		t.Errorf("%d spike.ended rows after more sweeps, want 1", got)
	}
	_, text := h.findingsOf(sp)
	if !strings.Contains(text, "The spike's run failed: provider says no") ||
		!strings.Contains(text, "Not answered: the spike's run failed before it reached an answer.") {
		t.Errorf("the findings don't say it failed:\n%s", text)
	}
	if code, out := h.call("POST", "/api/respond", map[string]any{"id": runs[0].ID.String(), "response": map[string]any{"retry": true}}); code < 400 {
		t.Errorf("responding to a run with nothing to retry: %d %v", code, out)
	}
}

func TestSpikeEndIsReconciled(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")

	// A run that succeeded while the event was never handled: the spike is
	// still running, with its worktree, until the next heartbeat.
	sp := h.newSpike(in, "Was the event lost?", nil)
	started, d := h.startSpikeQuiet(sp, 1000)
	if _, err := h.srv.ensureSpikeWorktree(started); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSpikeDraft(ctx, h.srv.Store.Pool, sp.ID, "## Answer\n\nProbably.\n\n## What we found\n\nA lost event.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET state = 'succeeded', outcome = '{"ended":"budget"}' WHERE id = $1`, d.ID); err != nil {
		t.Fatal(err)
	}
	if cur := h.getSpike(sp.ID); cur.State != store.SpikeRunning {
		t.Fatalf("the spike is %s before the sweep", cur.State)
	}
	h.srv.ReconcileSpikes(ctx)
	done := h.getSpike(sp.ID)
	if done.State != store.SpikeEnded || done.EndedHow != store.SpikeBudget || done.WorktreeRemovedAt == nil {
		t.Errorf("after the sweep: %s %q removed %v", done.State, done.EndedHow, done.WorktreeRemovedAt)
	}
	if _, text := h.findingsOf(done); !strings.Contains(text, "A lost event.") {
		t.Errorf("the findings lack the draft:\n%s", text)
	}
	if _, err := os.Stat(h.spikeDir(sp)); !os.IsNotExist(err) {
		t.Errorf("the worktree is still there: %v", err)
	}

	// A run that concluded with finish_spike, never handled: the findings are its.
	sp2 := h.newSpike(in, "Was a conclusion lost?", nil)
	_, d2 := h.startSpikeQuiet(sp2, 1000)
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET state = 'succeeded', outcome = $2 WHERE id = $1`, d2.ID, findingsJSON(goodFindings)); err != nil {
		t.Fatal(err)
	}
	h.srv.ReconcileSpikes(ctx)
	done2 := h.getSpike(sp2.ID)
	if done2.EndedHow != store.SpikeConcluded {
		t.Errorf("ended %q, want concluded", done2.EndedHow)
	}
	if _, text := h.findingsOf(done2); !strings.Contains(text, "The header is read in one place") {
		t.Errorf("the findings lack finish_spike's body:\n%s", text)
	}

	// A run that failed with no attempts left, and one cancelled.
	sp3 := h.newSpike(in, "Was a failure lost?", nil)
	_, d3 := h.startSpikeQuiet(sp3, 1000)
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET state = 'failed', attempt = 3, error = 'out of attempts' WHERE id = $1`, d3.ID); err != nil {
		t.Fatal(err)
	}
	sp4 := h.newSpike(in, "Was a run cancelled?", nil)
	_, d4 := h.startSpikeQuiet(sp4, 1000)
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET state = 'cancelled' WHERE id = $1`, d4.ID); err != nil {
		t.Fatal(err)
	}
	h.srv.ReconcileSpikes(ctx)
	if cur := h.getSpike(sp3.ID); cur.EndedHow != store.SpikeFailed || cur.EndNote != "out of attempts" {
		t.Errorf("exhausted: %q %q", cur.EndedHow, cur.EndNote)
	}
	if d, _ := store.GetDispatch(ctx, h.srv.Store.Pool, d3.ID); d.State != "cancelled" {
		t.Errorf("the exhausted run is %s, want cancelled", d.State)
	}
	if cur := h.getSpike(sp4.ID); cur.EndedHow != store.SpikeFailed || cur.EndNote != "The run was cancelled." {
		t.Errorf("cancelled: %q %q", cur.EndedHow, cur.EndNote)
	}
	// A run that failed with attempts left is the retry sweep's, not ours.
	sp5 := h.newSpike(in, "Is a retry coming?", nil)
	_, d5 := h.startSpikeQuiet(sp5, 1000)
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET state = 'failed', attempt = 1, error = 'blip' WHERE id = $1`, d5.ID); err != nil {
		t.Fatal(err)
	}
	h.srv.ReconcileSpikes(ctx)
	if cur := h.getSpike(sp5.ID); cur.State != store.SpikeRunning {
		t.Errorf("a spike whose run will be retried was ended: %s", cur.State)
	}

	// An EndSpike that failed after writing its findings is completed
	// without writing them twice.
	sp6 := h.newSpike(in, "Did the ending stop halfway?", nil)
	started6, _ := h.startSpikeQuiet(sp6, 1000)
	if _, err := h.srv.ensureSpikeWorktree(started6); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSpikeDraft(ctx, h.srv.Store.Pool, sp6.ID, "## Answer\n\nYes.\n\n## What we found\n\nHalfway.\n"); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.recordSpikeEnd(ctx, started6, store.SpikeBudget, ""); err != nil {
		t.Fatal(err)
	}
	half := h.getSpike(sp6.ID)
	doc, _ := h.findingsOf(half)
	if half.State != store.SpikeEnded || half.WorktreeRemovedAt != nil || h.gitOut("ls-files", "--", doc.Path) != "" {
		t.Fatalf("not a half-done ending: %s, removed %v", half.State, half.WorktreeRemovedAt)
	}
	h.srv.ReconcileSpikes(ctx)
	h.srv.ReconcileSpikes(ctx) // and again: nothing more happens
	end := h.getSpike(sp6.ID)
	docs, err := store.DocumentsForOwner(ctx, h.srv.Store.Pool, "spike", &sp6.ID)
	if err != nil || len(docs) != 1 {
		t.Errorf("%d findings documents, want 1 (%v)", len(docs), err)
	}
	if end.WorktreeRemovedAt == nil || h.gitOut("ls-files", "--", doc.Path) == "" {
		t.Errorf("the ending wasn't completed: removed %v", end.WorktreeRemovedAt)
	}
	if got := h.auditKinds("spike.worktree_discarded", sp6.ID); got != 1 {
		t.Errorf("%d discard audit rows, want 1", got)
	}
	if out := h.gitOut("log", "--format=%s", "--", doc.Path); strings.Count(out, "findings") != 1 {
		t.Errorf("the findings were committed more than once:\n%s", out)
	}
}

func TestLeftoverSpikeWorktreeIsRemoved(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	head := strings.TrimSpace(h.gitOut("rev-parse", "HEAD"))

	// A directory no spike names, which git knows.
	leftover := filepath.Join(h.root, ".subutai", "worktrees", "spk-leftover")
	h.gitOut("worktree", "add", "--detach", leftover, head)
	// One git has forgotten.
	forgotten := filepath.Join(h.root, ".subutai", "worktrees", "spk-forgot")
	if err := os.MkdirAll(forgotten, 0o755); err != nil {
		t.Fatal(err)
	}
	// A running spike's worktree must survive the sweep, and a feature's must too.
	run := h.newSpike(in, "Is this one still running?", nil)
	started, _ := h.startSpikeQuiet(run, 1000)
	runDir, err := h.srv.ensureSpikeWorktree(started)
	if err != nil {
		t.Fatal(err)
	}
	h.createFeature("pf", "login", "Login", "Sign in.")
	// An ended spike whose worktree was never discarded.
	ended := h.newSpike(in, "Was this one left behind?", nil)
	endedStarted, _ := h.startSpikeQuiet(ended, 1000)
	endedDir, err := h.srv.ensureSpikeWorktree(endedStarted)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.EndSpikeState(ctx, tx, ended.ID, store.SpikeFailed, "left")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	h.srv.ReconcileSpikes(ctx)

	for _, dir := range []string{leftover, forgotten, endedDir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s is still there (%v)", dir, err)
		}
	}
	if got := h.worktreeList(); strings.Contains(got, "spk-leftover") || strings.Contains(got, filepath.Base(endedDir)) {
		t.Errorf("git still lists a removed worktree:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(runDir, ".git")); err != nil {
		t.Errorf("the running spike's worktree was removed: %v", err)
	}
	if cur := h.getSpike(ended.ID); cur.WorktreeRemovedAt == nil {
		t.Error("the ended spike's row doesn't say its worktree was discarded")
	}
	// The feature code's own sweeps leave a running spike alone too.
	h.srv.GCWorktrees(ctx)
	if err := h.srv.ReconcileWorktrees(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, ".git")); err != nil {
		t.Errorf("a feature sweep removed the running spike's worktree: %v", err)
	}
	if cur := h.getSpike(run.ID); cur.State != store.SpikeRunning {
		t.Errorf("the running spike is %s", cur.State)
	}
}

// ---- FR-7: closing, and asking again ----

// endedSpike runs a spike to its end with a concluding agent, and returns it.
func (h *harness) endedSpike(initiativeID uuid.UUID, question string, budget *int64) *store.Spike {
	h.t.Helper()
	sp := h.newSpike(initiativeID, question, budget)
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(sp, 0)
	return h.spikeSettled(sp)
}

func TestPersonClosesASpike(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")

	a := h.endedSpike(in, "Is the first question answered?", nil)
	b := h.endedSpike(in, "Is the second question answered?", nil)
	docA, _ := h.findingsOf(a)
	stateBefore := docA.State

	closedA, err := h.srv.CloseSpike(ctx, a.ID, store.SpikeAnswered, nil, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if closedA.State != store.SpikeClosed || closedA.ClosedAs != store.SpikeAnswered || closedA.ClosedBy != "sam" || closedA.ClosedAt == nil {
		t.Errorf("closed answered: %s as %q by %q", closedA.State, closedA.ClosedAs, closedA.ClosedBy)
	}
	if closedA.EndedHow != store.SpikeConcluded {
		t.Errorf("closing lost how the run ended: %q", closedA.EndedHow)
	}
	closedB, err := h.srv.CloseSpike(ctx, b.ID, store.SpikeUnanswered, nil, "sam")
	if err != nil || closedB.ClosedAs != store.SpikeUnanswered {
		t.Fatalf("closed unanswered: %v, %v", closedB, err)
	}
	// Closing doesn't approve the findings: they keep their own lifecycle.
	if cur, _ := h.findingsOf(a); cur.State != stateBefore {
		t.Errorf("closing moved the findings from %s to %s", stateBefore, cur.State)
	}
	if got := h.auditKinds("spike.closed", a.ID); got != 1 {
		t.Errorf("%d spike.closed rows, want 1", got)
	}

	// Closing twice, and closing a spike that is still running, are refused.
	want := "This spike can't be closed now: it is still running, or it is already closed."
	if _, err := h.srv.CloseSpike(ctx, a.ID, store.SpikeAnswered, nil, "sam"); err == nil || err.Error() != want {
		t.Errorf("closing twice: %v", err)
	}
	if _, err := h.srv.CloseSpike(ctx, a.ID, SpikeCloseAgain, nil, "sam"); err == nil || err.Error() != want {
		t.Errorf("asking again of a closed spike: %v", err)
	}
	running := h.newSpike(in, "Is this one still running?", nil)
	h.startSpikeQuiet(running, 1000)
	for _, as := range []string{store.SpikeAnswered, store.SpikeUnanswered, SpikeCloseAgain} {
		if _, err := h.srv.CloseSpike(ctx, running.ID, as, nil, "sam"); err == nil || err.Error() != want {
			t.Errorf("closing a running spike as %s: %v", as, err)
		}
	}
	if cur := h.getSpike(running.ID); cur.State != store.SpikeRunning {
		t.Errorf("a refused close moved the spike to %s", cur.State)
	}
	// An idea can be closed unanswered, but not answered, and isn't asked again.
	idea := h.newSpike(in, "Was this ever started?", nil)
	for _, as := range []string{store.SpikeAnswered, SpikeCloseAgain} {
		if _, err := h.srv.CloseSpike(ctx, idea.ID, as, nil, "sam"); err == nil || err.Error() != want {
			t.Errorf("closing an idea as %s: %v", as, err)
		}
	}
	if _, err := h.srv.CloseSpike(ctx, idea.ID, "bogus", nil, "sam"); err == nil {
		t.Error("an unknown way of closing was accepted")
	}
	if _, err := h.srv.CloseSpike(ctx, idea.ID, store.SpikeUnanswered, nil, "sam"); err != nil {
		t.Errorf("closing an idea unanswered: %v", err)
	}
	// Starting a closed spike is refused, in a sentence.
	if _, err := h.srv.StartSpike(ctx, a.ID, 0, "sam"); err == nil || err.Error() != "This spike has already been started." {
		t.Errorf("starting a closed spike: %v", err)
	}
}

func TestAskingAgainMakesASecondSpike(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	h.createFeature("pf", "login", "Login", "Sign in.")
	feat, _ := h.srv.featureByPath(ctx, "pf/login")

	first, err := h.srv.CreateSpike(ctx, "feature", feat.ID, "Does the header survive the proxy?", i64(20_000), "sam", "ui")
	if err != nil {
		t.Fatal(err)
	}
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(first, 0)
	first = h.spikeSettled(first)

	second, err := h.srv.CloseSpike(ctx, first.ID, SpikeCloseAgain, i64(90_000), "sam")
	if err != nil {
		t.Fatal(err)
	}
	if second.PublicID == first.PublicID || second.State != store.SpikeIdea || second.Question != first.Question {
		t.Errorf("second spike = %s %s %q", second.PublicID, second.State, second.Question)
	}
	if second.FollowsID == nil || *second.FollowsID != first.ID {
		t.Errorf("the second spike doesn't follow the first: %v", second.FollowsID)
	}
	if second.FeatureID == nil || *second.FeatureID != feat.ID || second.InitiativeID != in {
		t.Errorf("the second spike's owner is wrong: %v / %s", second.FeatureID, second.InitiativeID)
	}
	if second.BudgetOverride == nil || *second.BudgetOverride != 90_000 || second.CreatedVia != "ui" {
		t.Errorf("budget override %v via %s", second.BudgetOverride, second.CreatedVia)
	}
	closed := h.getSpike(first.ID)
	if closed.State != store.SpikeClosed || closed.ClosedAs != store.SpikeUnanswered {
		t.Errorf("the first spike is %s as %q, want closed unanswered", closed.State, closed.ClosedAs)
	}
	if n := len(h.runsFor("spike", second.ID)); n != 0 {
		t.Errorf("asking again started the second spike (%d runs)", n)
	}
	var follows int
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM spikes WHERE follows_id = $1`, first.ID).Scan(&follows); err != nil || follows != 1 {
		t.Errorf("%d spikes follow the first (%v)", follows, err)
	}

	// Started, its prompt holds the earlier findings, and the new budget.
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(second, 0)
	second = h.spikeSettled(second)
	prompt := h.runPrompt(second)
	if !strings.Contains(prompt, "The header is read in one place, and a test shows it.") || !strings.Contains(prompt, "You have 90,000 tokens.") {
		t.Errorf("the second run's prompt:\n%s", prompt)
	}

	// Asking again with no figure keeps the budget the spike had.
	third, err := h.srv.CloseSpike(ctx, second.ID, SpikeCloseAgain, nil, "sam")
	if err != nil || third.BudgetOverride == nil || *third.BudgetOverride != 90_000 {
		t.Errorf("third = %v, %v", third, err)
	}
	// A refusal in the new spike undoes the close: the owner is archived, so
	// the first spike isn't closed for nothing.
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(third, 0)
	third = h.spikeSettled(third)
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE initiatives SET archived = true WHERE id = $1`, in); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.CloseSpike(ctx, third.ID, SpikeCloseAgain, nil, "sam"); err == nil ||
		err.Error() != "That initiative is archived, so a spike can't be created on it." {
		t.Errorf("asking again under an archived initiative: %v", err)
	}
	if cur := h.getSpike(third.ID); cur.State != store.SpikeEnded {
		t.Errorf("a refused ask-again closed the spike: %s", cur.State)
	}
	if _, err := h.srv.CloseSpike(ctx, third.ID, SpikeCloseAgain, i64(-1), "sam"); err == nil {
		t.Error("a negative budget was accepted")
	}
}

// ---- NFR-4: there is no merge path ----

func TestSpikeHasNoMergePath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	branches := h.gitOut("branch", "--list")
	base := h.headSHA()

	// A whitelisted script that does what a run must not: commit in the
	// worktree and make a branch there. It only runs when the agent asks.
	h.editConfig(`    argv: ["true"]`, `    argv: ["sh", "-c", "echo leaked > leak.txt && git add -A && git -c user.name=x -c user.email=x@x commit -qm leaked && git branch keep-this"]`)

	// A run that leaves scaffolding but keeps no code.
	clean := h.newSpike(in, "Does scaffolding stay in the working copy?", nil)
	write, _ := json.Marshal(map[string]string{"path": "scratch.txt", "content": "throwaway\n"})
	h.mock.RespondToolUse("write_file", string(write), tiny)
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(clean, 0)
	clean = h.spikeSettled(clean)
	if _, err := os.Stat(filepath.Join(h.root, "scratch.txt")); !os.IsNotExist(err) {
		t.Errorf("the run's scaffolding reached the main checkout (%v)", err)
	}
	if got := h.gitOut("branch", "--list"); got != branches {
		t.Errorf("a branch exists after a clean run:\nbefore %q\nafter  %q", branches, got)
	}
	// After the run, the main checkout's history holds only the findings commit.
	log := strings.Fields(h.gitOut("log", "--format=%H", base+"..HEAD"))
	if len(log) != 1 {
		t.Fatalf("%d commits since the run began, want only the findings'", len(log))
	}
	doc, _ := h.findingsOf(clean)
	if files := strings.Fields(h.gitOut("show", "--name-only", "--format=", log[0])); len(files) != 1 || files[0] != doc.Path {
		t.Errorf("the findings commit touches %v, want only %s", files, doc.Path)
	}
	if msg := strings.TrimSpace(h.gitOut("log", "-1", "--format=%s")); !strings.HasPrefix(msg, "SPK-001: findings (") {
		t.Errorf("the commit message = %q", msg)
	}
	if got := h.auditKinds("spike.code_kept", clean.ID); got != 0 {
		t.Errorf("a clean run raised a leak: %d", got)
	}
	var rows int
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM worktrees`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("worktrees rows = %d (%v), want none", rows, err)
	}

	// The merge refuses a spike's ID: it finds no feature.
	if err := h.srv.mergeFeature(ctx, clean.ID, "sam"); err == nil {
		t.Error("mergeFeature accepted a spike's id")
	}
	// The rules emit no merge for a run-spike success, whatever its outcome.
	for _, outcome := range []string{`{"ended":"budget"}`, `{"ended":"turn_limit"}`, findingsJSON(goodFindings), `{"merge":true}`} {
		actions := rules.Decide(bus.DispatchSucceeded{DispatchID: uuid.New(), Purpose: "run-spike", Role: "spike-runner",
			RefType: "spike", RefID: clean.ID, Outcome: json.RawMessage(outcome)}, rules.Snapshot{})
		for _, a := range actions {
			if _, ok := a.(rules.EndSpike); !ok {
				t.Errorf("a run-spike success led to %T", a)
			}
		}
	}
	// No route, API or UI, merges, promotes or lands a spike; and a spike's
	// ID isn't a feature to start.
	for _, path := range []string{"/api/spikes/merge", "/api/spikes/promote", "/api/spikes/land", "/api/spike/merge",
		"/ui/spikes/merge", "/ui/spikes/promote", "/ui/spikes/land", "/api/spikes/" + clean.ID.String() + "/merge"} {
		if code, _ := h.call("POST", path, map[string]string{"spike": clean.ID.String()}); code != 404 && code != 405 {
			t.Errorf("POST %s = %d, want 404 or 405", path, code)
		}
	}
	for _, path := range []string{clean.PublicID, "pf/" + clean.PublicID, clean.ID.String()} {
		if code, _ := h.call("POST", "/api/features/start", map[string]string{"path": path}); code < 400 {
			t.Errorf("POST /api/features/start with %q = %d, want a refusal", path, code)
		}
	}
	if resp, err := http.Get(h.api.URL + "/ui/f/pf/" + clean.PublicID); err != nil || resp.StatusCode != 404 {
		t.Errorf("the feature page for a spike's ID: %v %v", resp, err)
	}

	// The leak check. The agent runs the script: its commit is on a branch
	// that outlives the worktree.
	leak := h.newSpike(in, "Is leaked code caught?", nil)
	h.mock.RespondToolUse("run_command", `{"name":"build"}`, tiny)
	h.saveCall(tiny, "Yes.", "The script committed and branched.")
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(leak, 0)
	leak = h.spikeSettled(leak)

	if out := h.gitOut("branch", "--list", "keep-this"); !strings.Contains(out, "keep-this") {
		t.Fatalf("the branch the script made is gone (Subutai must not delete it): %q", out)
	}
	commit := strings.TrimSpace(h.gitOut("rev-parse", "keep-this"))
	if refs := strings.Fields(h.gitOut("for-each-ref", "--contains", commit, "--format=%(refname:short)")); len(refs) != 1 || refs[0] != "keep-this" {
		t.Errorf("the worktree's commit is reachable from %v, want only keep-this", refs)
	}
	_, text := h.findingsOf(leak)
	if !strings.Contains(text, "Code from this spike was kept on `keep-this`, outside its working copy. Subutai hasn't deleted it; the Inbox asks what to do.") {
		t.Errorf("the findings don't name the branch:\n%s", text)
	}
	if got := h.auditKinds("spike.code_kept", leak.ID); got != 1 {
		t.Errorf("%d spike.code_kept audit rows, want 1", got)
	}
	cps := h.pendingByKind("spike-code-kept")
	if len(cps) != 1 || cps[0].RefType != "spike" || cps[0].RefID != leak.ID ||
		cps[0].Question != "Code from SPK-002 was kept on `keep-this`, outside its working copy. A spike's code is never merged. Delete the branch, or keep it knowing it won't be built from." {
		t.Errorf("the checkpoint = %+v", cps)
	}
	if _, err := os.Stat(h.spikeDir(leak)); !os.IsNotExist(err) {
		t.Errorf("the leaking spike's worktree is still there (%v)", err)
	}
	// The leaked commit isn't on the main line: only the findings were added.
	if got := strings.Fields(h.gitOut("log", "--format=%s", "keep-this..HEAD")); len(got) == 0 {
		t.Error("the main line doesn't have its own commits")
	}
	if h.gitOut("branch", "--contains", commit, "--list", "main", "master") != "" {
		t.Error("the leaked commit reached the main line")
	}
	// Answering the checkpoint changes nothing in Subutai.
	if code, out := h.call("POST", "/api/respond", map[string]any{"id": cps[0].ID.String(), "response": map[string]any{"answer": "acknowledge"}}); code != 200 {
		t.Errorf("answering the checkpoint: %d %v", code, out)
	}
	if out := h.gitOut("branch", "--list", "keep-this"); !strings.Contains(out, "keep-this") {
		t.Error("answering the checkpoint deleted the branch")
	}
}

// ---- SD-14: open spikes block archiving ----

func TestOpenSpikesBlockArchivingTheirInitiative(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Is it safe to archive?", nil)

	code, out := h.call("POST", "/api/initiatives/archive", map[string]string{"path": "pf"})
	if code != 409 || !strings.Contains(out["reason"].(string), sp.PublicID+" is still open") {
		t.Fatalf("archive with an open spike: %d %v", code, out)
	}
	// The UI route says so too, in the page a person reads.
	code, page := h.postForm("/ui/initiative/archive", map[string]string{"id": in.String()})
	if code != 200 || !strings.Contains(page, sp.PublicID+" is still open") {
		t.Errorf("the UI archive: %d\n%s", code, truncate(page, 600))
	}
	if st, _ := h.srv.Store.InitiativeBySlugPath(ctx, []string{"pf"}); st.Archived {
		t.Fatal("the initiative was archived with an open spike under it")
	}
	// One that has ended and waits for a person counts too.
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	h.startSpike(sp, 0)
	sp = h.spikeSettled(sp)
	if code, out := h.call("POST", "/api/initiatives/archive", map[string]string{"path": "pf"}); code != 409 {
		t.Errorf("archive with an ended spike: %d %v", code, out)
	}
	// Closed, it doesn't.
	if _, err := h.srv.CloseSpike(ctx, sp.ID, store.SpikeAnswered, nil, "sam"); err != nil {
		t.Fatal(err)
	}
	if code, out := h.call("POST", "/api/initiatives/archive", map[string]string{"path": "pf"}); code != 200 {
		t.Errorf("archive with only a closed spike: %d %v", code, out)
	}
}

// ---- Appendix A: the places that switch on an owner ----

func TestSpikeAppendixSites(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	sp := h.endedSpike(in, "Where does a spike show up?", nil)
	doc, _ := h.findingsOf(sp)

	// Attaching or adopting a document to a spike is refused, both ways in.
	const refusal = "A spike's only document is its findings, which its run writes."
	if _, err := h.srv.RegisterDoc(ctx, "docs/x.md", "note", "spike", sp.PublicID, "sam"); err == nil || err.Error() != refusal {
		t.Errorf("RegisterDoc on a spike: %v", err)
	}
	if _, err := h.srv.mcpResolveOwner(ctx, "spike", sp.PublicID); err == nil || err.Error() != refusal {
		t.Errorf("mcpResolveOwner on a spike: %v", err)
	}
	// The findings' owner is the spike, its crumb leads there, and its ID
	// is the spike's.
	if doc.OwnerType != "spike" || doc.OwnerID == nil || *doc.OwnerID != sp.ID || doc.PublicID != "SPK-001-findings" {
		t.Errorf("findings = %s owned by %s", doc.PublicID, doc.OwnerType)
	}
	if doc.Type != "findings" || !strings.HasPrefix(doc.Path, "docs/work/INIT-001-pf/SPK-001-findings") {
		t.Errorf("findings are %s at %s", doc.Type, doc.Path)
	}
	c := h.srv.ownerCrumb(ctx, "spike", doc.OwnerID)
	if c.ID != "SPK-001" || c.URL != "/ui/s/SPK-001" {
		t.Errorf("owner crumb = %+v", c)
	}
	// A spike-owned document's branch is the spike's initiative.
	if sc := h.srv.scopeForDocument(ctx, doc); sc.InitiativeID == nil || *sc.InitiativeID != in {
		t.Errorf("scope = %+v", sc)
	}
	// The rules' snapshot doesn't fail on a spike-owned document, and has no
	// owning feature for it.
	snap, err := h.srv.snapshot(ctx, bus.DocumentTransitioned{DocID: doc.ID, To: "reviewing"})
	if err != nil || snap.Doc == nil || snap.OwnerFeature != nil || snap.Doc.OwnerType != "spike" {
		t.Errorf("snapshot = %+v, %v", snap, err)
	}
	// The findings go through the ordinary lifecycle, and a person approves
	// them: submitting queues no agent review, because the manifest says so.
	if report, _, err := h.srv.SubmitDoc(ctx, doc.Path, "sam"); err != nil || !report.Valid {
		t.Fatalf("submit findings: %v %v", err, report)
	}
	h.quiet()
	if got := h.docState(doc.Path); got != "reviewing" {
		t.Errorf("submitted findings are %s, want reviewing (a person's to approve)", got)
	}
	if n := len(h.runsFor("document", doc.ID)); n != 0 {
		t.Errorf("%d agent reviews were dispatched for findings, want none", n)
	}
	if err := h.srv.HumanApproveDocument(ctx, doc.ID, "sam"); err != nil {
		t.Fatal(err)
	}
	if got := h.docState(doc.Path); got != "approved" {
		t.Errorf("approved findings are %s", got)
	}
	if st, _ := store.GetSpike(ctx, h.srv.Store.Pool, sp.ID); st.State != store.SpikeEnded {
		t.Errorf("approving the findings moved the spike to %s", st.State)
	}

	// An ID redirects to the spike's page.
	if code, loc := h.redirectOf("/ui/id/SPK-001"); code != 302 || loc != "/ui/s/SPK-001" {
		t.Errorf("/ui/id/SPK-001 = %d %q", code, loc)
	}
	if code, loc := h.redirectOf("/ui/id/spk-001"); code != 302 || loc != "/ui/s/SPK-001" {
		t.Errorf("/ui/id/spk-001 = %d %q", code, loc)
	}
	if code, _ := h.redirectOf("/ui/id/SPK-099"); code != 404 {
		t.Errorf("/ui/id/SPK-099 = %d, want 404", code)
	}
	if code, loc := h.redirectOf("/ui/id/SPK-001-findings"); code != 302 || !strings.HasPrefix(loc, "/ui/d/docs/work/") {
		t.Errorf("/ui/id/SPK-001-findings = %d %q", code, loc)
	}

	// The run's page says what it was and leads to the spike and its owner.
	runs := h.runsFor("spike", sp.ID)
	page, err := h.srv.buildRunPage(ctx, &runs[0], "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Heading != "Running a spike" {
		t.Errorf("run heading = %q", page.Heading)
	}
	var urls []string
	for _, cr := range page.Crumbs {
		urls = append(urls, cr.URL)
	}
	if !strings.Contains(strings.Join(urls, " "), "/ui/s/SPK-001") || !strings.Contains(strings.Join(urls, " "), "/ui/i/pf") {
		t.Errorf("run crumbs = %v", urls)
	}
}
