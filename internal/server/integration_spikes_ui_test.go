package server

// The spikes suite's web half (SPEC-021 FR-1.3, FR-1.4, FR-3, FR-7.1, FR-8):
// the start screen and the one route that starts a spike, the pages that show
// the work, and creating, closing and asking again from the browser's forms.

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/store"
)

// uiText is a page as a reader sees it: the HTML entities the templates
// escape (an apostrophe is &#39;) are turned back into characters.
func (h *harness) uiText(path string) string {
	h.t.Helper()
	code, body := h.getUI(path)
	if code != 200 {
		h.t.Fatalf("GET %s = %d\n%s", path, code, truncate(body, 600))
	}
	return html.UnescapeString(body)
}

// postText posts a form and returns the status and the unescaped body. A
// redirect to a spike's page is followed, as a browser does, and its page
// returned.
func (h *harness) postText(path string, v url.Values) (int, string) {
	h.t.Helper()
	code, body := h.postValues(path, v)
	if code == http.StatusSeeOther && strings.HasPrefix(body, "/ui/s/") {
		return http.StatusOK, h.uiText(body)
	}
	return code, html.UnescapeString(body)
}

func wants(t *testing.T, what, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, truncate(body, 1500))
		}
	}
}

func lacks(t *testing.T, what, body string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(body, w) {
			t.Errorf("%s has %q, which it shouldn't:\n%s", what, w, truncate(body, 1500))
		}
	}
}

func startURL(sp *store.Spike) string { return "/ui/spikes/start?spike=" + sp.ID.String() }

// ---- FR-3: the start screen and the one route that starts a spike ----

func TestSpikeStartsFromTheWebUIOnly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")

	cfg, err := h.srv.freshConfig()
	if err != nil {
		t.Fatal(err)
	}
	_, model, refusal := h.srv.spikeRunnerFor(cfg)
	if refusal != "" {
		t.Fatalf("the test project has no spike runner: %s", refusal)
	}

	// Without decisions, and with the project's default budget.
	plain := h.newSpike(in, "Does the API honour the header?", nil)
	screen := h.uiText(startURL(plain))
	wants(t, "the start screen", screen,
		"Does the API honour the header?", "The pf initiative",
		"The spike runner runs it, on "+model+".",
		`value="1000000"`, "This is the project's default budget, from <code>spikes.default_token_budget</code>.",
		"The run stops when it reaches this many tokens. Whatever findings it has saved by then are written up, and you decide whether to ask again.",
		"No decisions apply here.",
		"It works in a throwaway copy of the code, made from the current main line. Nothing it writes there is kept: the copy is discarded when the run ends, and Subutai never merges it.",
		"agent slots are free now", "There aren't enough earlier spikes to forecast this one yet.",
		"<code>save_findings</code>", "<code>finish_spike</code>", "<code>write_file</code>")
	lacks(t, "the start screen", screen, "disabled")

	// With decisions: the block the agent will receive.
	h.acceptDecision("Use plain SQL", "project", "Every query is plain SQL.", "Simple.")
	h.acceptDecision("Cache nothing in pf", h.initiativePublicID("pf"), "The pf initiative caches nothing.", "Staleness.")
	screen = h.uiText(startURL(plain))
	wants(t, "the start screen with decisions", screen, "Every query is plain SQL.", "The pf initiative caches nothing.")
	lacks(t, "the start screen with decisions", screen, "No decisions apply here.")

	// With the spike's own budget: its figure, and no default sentence.
	own := h.newSpike(in, "Is the API fast enough?", i64(40_000))
	screen = h.uiText(startURL(own))
	wants(t, "the start screen with an override", screen, `value="40000"`)
	lacks(t, "the start screen with an override", screen, "This is the project's default budget")

	// With nobody assigned to run spikes: the sentence, and the button disabled.
	h.editConfig("  run-spike: spike-runner\n", "  # run-spike: spike-runner\n")
	screen = h.uiText(startURL(plain))
	wants(t, "the start screen with no runner", screen,
		"Nobody is assigned to run spikes, so this spike can't start. Assign <code>run-spike</code> to a role in <code>config.yaml</code>.",
		"disabled")
	if code, body := h.postText("/ui/spikes/start", url.Values{"spike": {plain.ID.String()}}); code != 200 {
		t.Errorf("starting with no runner = %d", code)
	} else {
		wants(t, "the refused start", body, "Nobody is assigned to run spikes, so this spike can't start.")
	}
	if got := h.getSpike(plain.ID); got.State != store.SpikeIdea {
		t.Errorf("a refused start left the spike %s", got.State)
	}
	h.editConfig("  # run-spike: spike-runner\n", "  run-spike: spike-runner\n")

	// The POST starts it once.
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	code, loc := h.postValues("/ui/spikes/start", url.Values{"spike": {own.ID.String()}, "budget": {"40,000"}})
	if code != http.StatusSeeOther || !strings.HasPrefix(loc, "/ui/s/"+own.PublicID) {
		t.Fatalf("POST /ui/spikes/start = %d %q, want a 303 to the spike's page", code, loc)
	}
	body := h.uiText(loc)
	wants(t, "the page after the start", body, own.PublicID+" has started, with a budget of 40,000 tokens.", "This spike is running.")
	started := h.getSpike(own.ID)
	if started.TokenBudget == nil || *started.TokenBudget != 40_000 || started.StartedBy == "" {
		t.Errorf("started spike = %+v", started)
	}
	h.spikeSettled(own)

	// A second POST is refused with the sentence, and starts nothing more.
	before := len(h.runsFor("spike", own.ID))
	_, body = h.postText("/ui/spikes/start", url.Values{"spike": {own.ID.String()}})
	wants(t, "the second start", body, "This spike has already been started.")
	if n := len(h.runsFor("spike", own.ID)); n != before {
		t.Errorf("the second POST queued a run (%d runs, was %d)", n, before)
	}
	// The screen for a spike that has started has no form.
	wants(t, "the screen after the start", h.uiText(startURL(own)), "This spike has already been started.")

	// A start with a bad budget is refused with a sentence.
	_, body = h.postText("/ui/spikes/start", url.Values{"spike": {plain.ID.String()}, "budget": {"-5"}})
	wants(t, "the bad budget", body, "A spike's budget is a positive whole number of tokens.")
	if got := h.getSpike(plain.ID); got.State != store.SpikeIdea {
		t.Errorf("a refused start left the spike %s", got.State)
	}

	// No other route starts one.
	for _, path := range []string{"/api/spikes/start", "/api/spikes/" + plain.ID.String() + "/start", "/api/spike/start"} {
		for _, method := range []string{"POST", "GET", "PUT"} {
			req, _ := http.NewRequest(method, h.api.URL+path, strings.NewReader("{}"))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 404 && resp.StatusCode != 405 {
				t.Errorf("%s %s = %d, want 404 or 405", method, path, resp.StatusCode)
			}
		}
	}
	if got := h.getSpike(plain.ID); got.State != store.SpikeIdea {
		t.Errorf("a spike with no start is %s", got.State)
	}
	if n := len(h.runsFor("spike", plain.ID)); n != 0 {
		t.Errorf("an idea has %d runs", n)
	}
	_ = ctx
}

// ---- FR-8: where spikes show ----

func TestSpikePagesShowTheWork(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	h.createFeature("pf", "login", "Login", "the login form")
	fid := h.featureID("pf/login")

	// The feature's token total before any spike, to compare after.
	doneBefore, err := store.ActualTokens(ctx, h.srv.Store.Pool, "feature", fid)
	if err != nil {
		t.Fatal(err)
	}

	// An idea: its sentences, the default budget and its two actions.
	idea, err := h.srv.CreateSpike(ctx, "initiative", in, "Which queue should we use?", nil, "sam", "ui")
	if err != nil {
		t.Fatal(err)
	}
	page := h.uiText("/ui/s/" + idea.PublicID)
	wants(t, "an idea's page", page,
		"Which queue should we use?", "The pf initiative",
		"This spike hasn't started yet.",
		"It will have the project's default budget of 1,000,000 tokens, unless you change it when you start it.",
		"Start this spike…", "/ui/spikes/start?spike="+idea.ID.String(), "Close without running",
		"Created by sam in the web UI.")
	lacks(t, "an idea's page", page, "The question is answered", "Ask again with a new budget")
	// /ui/id/ lands on the page.
	if code, loc := h.redirectOf("/ui/id/" + idea.PublicID); code != 302 || loc != "/ui/s/"+idea.PublicID {
		t.Errorf("/ui/id/%s = %d %q", idea.PublicID, code, loc)
	}

	// One that is running, with no draft and then a draft.
	live := h.newSpike(in, "Is the cache warm?", i64(40_000))
	live, _ = h.startSpikeQuiet(live, 40_000)
	page = h.uiText("/ui/s/" + live.PublicID)
	wants(t, "a running spike's page", page,
		"This spike is running.", "0 of 40,000 tokens", "The agent hasn't saved any findings yet.", "Its working copy is live.",
		"Started by sam, with a budget of 40,000 tokens.")
	lacks(t, "a running spike's page", page, "The question is answered", "Start this spike")
	if err := store.SaveSpikeDraft(ctx, h.srv.Store.Pool, live.ID, "## Answer\n\nProbably.\n\n## What we found\n\nA draft note.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSpikeTokens(ctx, h.srv.Store.Pool, live.ID, 31_004); err != nil {
		t.Fatal(err)
	}
	live = h.getSpike(live.ID)
	page = h.uiText("/ui/s/" + live.PublicID)
	wants(t, "a running spike's page with a draft", page,
		"31,004 of 40,000 tokens", "A draft note.", "The agent last saved this on "+spikeWhen(*live.DraftSavedAt),
		"If the run stops now, this is what is kept.")
	lacks(t, "a running spike's page with a draft", page, "The agent hasn't saved any findings yet.")

	// One that stopped at its budget, on the feature, as T4's hard-stop test.
	for i := 1; i <= 8; i++ {
		h.saveCall(tiny, "Not sure yet.", "Step "+string(rune('0'+i))+" of the investigation.")
	}
	stopped, err := h.srv.CreateSpike(ctx, "feature", fid, "Does the login form leak timing?", nil, "sam", "ui")
	if err != nil {
		t.Fatal(err)
	}
	h.startSpike(stopped, 1000)
	stopped = h.spikeSettled(stopped)
	doc, _ := h.findingsOf(stopped)
	page = h.uiText("/ui/s/" + stopped.PublicID)
	wants(t, "an ended spike's page", page,
		"This spike's run has ended: it stopped at its budget. It's waiting for you to read the findings.",
		"900 of 1,000 tokens", "including attempts that failed",
		stopped.PublicID+"-findings", "/ui/d/"+doc.Path,
		"Its working copy was discarded on "+spikeWhen(*stopped.WorktreeRemovedAt)+".",
		"The question is answered", "Close without an answer", "Ask again with a new budget",
		"Created by sam in the web UI.", "Started by sam, with a budget of 1,000 tokens.",
		"Ended: it stopped at its budget. It used 900 of its 1,000 tokens.", "Working copy discarded.",
		"Does the login form leak timing?", "Login")
	lacks(t, "an ended spike's page", page, "Start this spike", "The agent hasn't saved")
	runs := h.runsFor("spike", stopped.ID)
	if len(runs) != 1 {
		t.Fatalf("%d runs", len(runs))
	}
	wants(t, "the ended spike's page", page, "/ui/run/"+runs[0].ID.String())

	// One that concluded.
	concl := h.endedSpike(in, "Does the API honour the header?", nil)
	wants(t, "a concluded spike's page", h.uiText("/ui/s/"+concl.PublicID),
		"This spike's run has ended: it reached a conclusion. It's waiting for you to read the findings.",
		"Ended: it reached a conclusion.")

	// The owner sections: the initiative lists the spikes it owns directly,
	// the feature its own, each with its state and tokens used of budget.
	ipage := h.uiText("/ui/i/pf")
	wants(t, "the initiative's page", ipage, "Spikes", idea.PublicID, "Which queue should we use?", "Not started",
		live.PublicID, "31,004 of 40,000 tokens", concl.PublicID, "New spike…", `action="/ui/spikes"`)
	lacks(t, "the initiative's page", ipage, "Does the login form leak timing?")
	fpage := h.uiText("/ui/f/pf/login")
	wants(t, "the feature's page", fpage, "Spikes", stopped.PublicID, "Does the login form leak timing?", "900 of 1,000 tokens",
		"New spike…")
	lacks(t, "the feature's page", fpage, "Which queue should we use?")

	// The feature's timeline: the spike's two moments, linked to it, and no
	// change to the feature's own token total.
	wants(t, "the feature's timeline", fpage,
		"Spike "+stopped.PublicID+" started", "Spike "+stopped.PublicID+" ended: it stopped at its budget",
		"/ui/s/"+stopped.PublicID)
	if done, err := store.ActualTokens(ctx, h.srv.Store.Pool, "feature", fid); err != nil || done != doneBefore {
		t.Errorf("the feature's tokens = %d (%v), were %d: a spike's tokens are its own", done, err, doneBefore)
	}
	if rs, err := store.FeatureRuns(ctx, h.srv.Store.Pool, fid); err != nil || len(rs) != 0 {
		t.Errorf("the feature's runs = %d (%v): a spike's run isn't the feature's", len(rs), err)
	}
	hist, err := store.FeatureHistory(ctx, h.srv.Store.Pool, fid)
	if err != nil {
		t.Fatal(err)
	}
	spikeRows := 0
	for _, e := range hist {
		if e.RefType == "spike" {
			spikeRows++
			if e.Kind != "spike.started" && e.Kind != "spike.ended" {
				t.Errorf("the feature's history holds %s", e.Kind)
			}
		}
	}
	if spikeRows != 2 {
		t.Errorf("the feature's history holds %d spike rows, want 2", spikeRows)
	}

	// The run page: its purpose, crumbs and outcome sentence, and the stop entry.
	rpage := h.uiText("/ui/run/" + runs[0].ID.String())
	wants(t, "the budget-stopped run", rpage, "Running a spike", "/ui/s/"+stopped.PublicID, "/ui/f/pf/login",
		"It stopped at its budget.", "The run stopped here because it reached its budget of 1,000 tokens.")
	cruns := h.runsFor("spike", concl.ID)
	wants(t, "the concluded run", h.uiText("/ui/run/"+cruns[0].ID.String()), "It concluded.")

	// A run that hit its turn limit.
	h.editRole("spike-runner", "turn_cap: 40", "turn_cap: 3")
	for i := 1; i <= 5; i++ {
		h.saveCall(tiny, "Not sure.", "Turn "+string(rune('0'+i))+".")
	}
	limited := h.newSpike(in, "Will it stop at the turn limit?", nil)
	h.startSpike(limited, 0)
	limited = h.spikeSettled(limited)
	lruns := h.runsFor("spike", limited.ID)
	wants(t, "the turn-limited run", h.uiText("/ui/run/"+lruns[0].ID.String()),
		"It stopped at its turn limit.", "The run stopped here because it reached its turn limit of 3 turns.")
	wants(t, "the turn-limited spike", h.uiText("/ui/s/"+limited.PublicID),
		"This spike's run has ended: it stopped at its turn limit.")

	// /ui/spikes lists them all, open ones first.
	list := h.uiText("/ui/spikes")
	wants(t, "the spikes list", list, idea.PublicID, live.PublicID, stopped.PublicID, concl.PublicID,
		"Which queue should we use?", "On "+h.initiativePublicID("pf")+" The pf initiative", "900 of 1,000 tokens")

	// The Inbox: the count of ended spikes, linking to the list, while the
	// badge still counts checkpoints only.
	wants(t, "the inbox", h.uiText("/ui/inbox"), "/ui/frag/spikes-line")
	line := h.uiText("/ui/frag/spikes-line")
	wants(t, "the inbox line", line, "3 spikes have ended and are waiting for you to read their findings.", `href="/ui/spikes"`)

	// Closing as answered: SD-12's sentence, and the line counts one fewer.
	code, body := h.postText("/ui/spikes/close", url.Values{"spike": {concl.ID.String()}, "as": {"answered"}})
	if code != 200 {
		t.Fatalf("close = %d", code)
	}
	wants(t, "the closed spike", body, "This spike is closed: the question is answered.",
		"To build on this, cite "+concl.PublicID+"-findings in a design, then create a feature in the normal way.")
	lacks(t, "the closed spike", body, "The question is answered</button>", "Ask again with a new budget")
	wants(t, "the inbox line", h.uiText("/ui/frag/spikes-line"), "2 spikes have ended and are waiting for you to read their findings.")
	h.postText("/ui/spikes/close", url.Values{"spike": {limited.ID.String()}, "as": {"unanswered"}})
	wants(t, "the inbox line", h.uiText("/ui/frag/spikes-line"), "1 spike has ended and is waiting for you to read its findings.")
	h.postText("/ui/spikes/close", url.Values{"spike": {stopped.ID.String()}, "as": {"unanswered"}})
	if _, empty := h.getUI("/ui/frag/spikes-line"); strings.Contains(empty, "spike") {
		t.Errorf("with none waiting the line still says: %s", empty)
	}
	wants(t, "a spike closed without an answer", h.uiText("/ui/s/"+stopped.PublicID),
		"This spike is closed: the question wasn't answered.", "Closed without an answer, by ")

	// A spike on a page that doesn't exist is a 404.
	if code, _ := h.getUI("/ui/s/SPK-999"); code != 404 {
		t.Errorf("/ui/s/SPK-999 = %d, want 404", code)
	}

	// The code-kept checkpoint's answer reads "I've dealt with it" (FR-6.3).
	if opts := answerOptions("spike-code-kept"); len(opts) != 1 || verbLabel(opts[0]) != "I've dealt with it" {
		t.Errorf("spike-code-kept offers %v", opts)
	}

	// The milestone editor never offers a spike (FR-8.6): its candidates are
	// initiatives, features, milestones and checklists, and a spike is none.
	if code, out := h.call("POST", "/api/milestones", map[string]string{"name": "v1"}); code != 201 && code != 200 {
		t.Fatalf("milestone: %d %v", code, out)
	}
	m := h.milestoneNamed("v1")
	cands := h.uiText("/ui/m/" + m.ID.String() + "/candidates?q=log")
	lacks(t, "the milestone candidates", cands, "SPK-", "Which queue", "Does the login form leak timing?")
	wants(t, "the milestone candidates", cands, "Login")
}

// ---- FR-1.3, FR-1.4, FR-7: creating and closing from the browser ----

func TestSpikeUICreateCloseAndAgain(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	h.createFeature("pf", "login", "Login", "the login form")
	fid := h.featureID("pf/login")

	// New spike, on the initiative and on the feature.
	code, body := h.postText("/ui/spikes", url.Values{"owner_type": {"initiative"}, "owner_id": {in.String()},
		"question": {"Which queue should we use?"}, "budget": {"50,000"}})
	if code != 200 {
		t.Fatalf("create on the initiative = %d\n%s", code, truncate(body, 600))
	}
	wants(t, "the initiative's page", body, "SPK-001 was written down. Nothing runs until you start it from its page.",
		"Which queue should we use?")
	code, body = h.postText("/ui/spikes", url.Values{"owner_type": {"feature"}, "owner_id": {fid.String()},
		"question": {"Does the login form leak timing?"}})
	if code != 200 {
		t.Fatalf("create on the feature = %d", code)
	}
	wants(t, "the feature's page", body, "SPK-002 was written down.", "Does the login form leak timing?")
	a, err := store.SpikeByPublicID(ctx, h.srv.Store.Pool, "SPK-001")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.SpikeByPublicID(ctx, h.srv.Store.Pool, "SPK-002")
	if err != nil {
		t.Fatal(err)
	}
	if a.State != store.SpikeIdea || a.FeatureID != nil || a.InitiativeID != in || a.BudgetOverride == nil || *a.BudgetOverride != 50_000 || a.CreatedVia != "ui" {
		t.Errorf("SPK-001 = %+v", a)
	}
	if b.FeatureID == nil || *b.FeatureID != fid || b.BudgetOverride != nil {
		t.Errorf("SPK-002 = %+v", b)
	}
	if n := len(h.runsFor("spike", a.ID)) + len(h.runsFor("spike", b.ID)); n != 0 {
		t.Errorf("creating queued %d runs", n)
	}

	// Refusals show as sentences, and cost no number.
	for _, c := range []struct {
		v    url.Values
		want string
	}{
		{url.Values{"owner_type": {"initiative"}, "owner_id": {in.String()}, "question": {"  "}}, "A spike needs a question"},
		{url.Values{"owner_type": {"initiative"}, "owner_id": {in.String()}, "question": {strings.Repeat("x", 501)}}, "at most 500 characters"},
		{url.Values{"owner_type": {"initiative"}, "owner_id": {in.String()}, "question": {"Fine?"}, "budget": {"0"}}, "A spike's budget is a positive whole number of tokens."},
		{url.Values{"owner_type": {"initiative"}, "owner_id": {in.String()}, "question": {"Fine?"}, "budget": {"lots"}}, "A spike's budget is a positive whole number of tokens."},
	} {
		code, body := h.postText("/ui/spikes", c.v)
		if code != 200 {
			t.Fatalf("refused create = %d", code)
		}
		wants(t, "a refused create", body, "The spike wasn't created. ", c.want)
	}
	next, err := h.srv.CreateSpike(ctx, "initiative", in, "Fine?", nil, "sam", "ui")
	if err != nil || next.PublicID != "SPK-003" {
		t.Fatalf("after four refusals the next spike is %v (%v), want SPK-003", next, err)
	}

	// Close without running: closed, unanswered, no findings.
	code, body = h.postText("/ui/spikes/close", url.Values{"spike": {next.ID.String()}, "as": {"unanswered"}})
	if code != 200 {
		t.Fatalf("close = %d", code)
	}
	wants(t, "the closed idea", body, "SPK-003 is closed without an answer.", "This spike is closed: the question wasn't answered.")
	if got := h.getSpike(next.ID); got.State != store.SpikeClosed || got.ClosedAs != store.SpikeUnanswered || got.StartedAt != nil {
		t.Errorf("closed idea = %+v", got)
	}
	if _, err := store.CurrentDocForOwner(ctx, h.srv.Store.Pool, "findings", "spike", next.ID); err == nil {
		t.Error("closing without running made findings")
	}
	_, body = h.postText("/ui/spikes/close", url.Values{"spike": {next.ID.String()}, "as": {"unanswered"}})
	wants(t, "closing twice", body, "This spike can't be closed now: it is still running, or it is already closed.")

	// A running spike can't be closed, answered or asked again.
	run := h.newSpike(in, "Is it still running?", nil)
	run, _ = h.startSpikeQuiet(run, 10_000)
	for _, as := range []string{"answered", "unanswered", "again"} {
		_, body = h.postText("/ui/spikes/close", url.Values{"spike": {run.ID.String()}, "as": {as}})
		wants(t, "closing a running spike as "+as, body, "This spike can't be closed now: it is still running, or it is already closed.")
	}
	if got := h.getSpike(run.ID); got.State != store.SpikeRunning {
		t.Errorf("a refused close left the spike %s", got.State)
	}
	_, body = h.postText("/ui/spikes/close", url.Values{"spike": {run.ID.String()}, "as": {"sideways"}})
	wants(t, "a bad way to close", body, "A spike is closed as answered, closed without an answer, or asked again.")

	// An ended spike closes as answered; the findings are left as they are.
	ended := h.endedSpike(in, "Does the API honour the header?", nil)
	doc, _ := h.findingsOf(ended)
	code, loc := h.postValues("/ui/spikes/close", url.Values{"spike": {ended.ID.String()}, "as": {"answered"}})
	if code != http.StatusSeeOther || !strings.HasPrefix(loc, "/ui/s/"+ended.PublicID) {
		t.Fatalf("closing as answered = %d %q, want a 303 to the spike's page", code, loc)
	}
	body = h.uiText(loc)
	wants(t, "answered", body, ended.PublicID+" is closed: the question is answered.", "Closed as answered, by ")
	if after, _ := h.findingsOf(ended); after.State != doc.State {
		t.Errorf("the findings went from %s to %s", doc.State, after.State)
	}

	// Asking again redirects to the new spike's start screen.
	first := h.endedSpike(in, "Does the API retry?", nil)
	code, loc = h.postValues("/ui/spikes/close", url.Values{"spike": {first.ID.String()}, "as": {"again"}, "budget": {"30,000"}})
	if code != 303 || !strings.HasPrefix(loc, "/ui/spikes/start?spike=") {
		t.Fatalf("ask again = %d %q, want a 303 to the start screen", code, loc)
	}
	newID, err := uuid.Parse(strings.TrimPrefix(loc, "/ui/spikes/start?spike="))
	if err != nil {
		t.Fatal(err)
	}
	again := h.getSpike(newID)
	if again.State != store.SpikeIdea || again.FollowsID == nil || *again.FollowsID != first.ID ||
		again.Question != first.Question || again.BudgetOverride == nil || *again.BudgetOverride != 30_000 {
		t.Errorf("the new spike = %+v", again)
	}
	if got := h.getSpike(first.ID); got.State != store.SpikeClosed || got.ClosedAs != store.SpikeUnanswered {
		t.Errorf("the first spike = %s %s", got.State, got.ClosedAs)
	}
	screen := h.uiText(loc)
	wants(t, "the new spike's start screen", screen, "Does the API retry?", `value="30000"`,
		"This spike asks again after "+first.PublicID, "The header is read in one place, and a test shows it.")
	lacks(t, "the new spike's start screen", screen, "This is the project's default budget")
	wants(t, "the first spike's page", h.uiText("/ui/s/"+first.PublicID), "It was asked again as", `href="/ui/s/`+again.PublicID+`"`)
	wants(t, "the new spike's page", h.uiText("/ui/s/"+again.PublicID), "It asks again after", `href="/ui/s/`+first.PublicID+`"`)

	// A bad budget on ask again is refused, and closes nothing.
	other := h.endedSpike(in, "Does the API cache?", nil)
	_, body = h.postText("/ui/spikes/close", url.Values{"spike": {other.ID.String()}, "as": {"again"}, "budget": {"-1"}})
	wants(t, "a bad ask-again budget", body, "A spike's budget is a positive whole number of tokens.")
	if got := h.getSpike(other.ID); got.State != store.SpikeEnded {
		t.Errorf("a refused ask-again left the spike %s", got.State)
	}
}
