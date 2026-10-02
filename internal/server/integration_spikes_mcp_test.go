package server

// SPEC-021 FR-9, FR-8.6 and NFR-3: the MCP spike tools.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"subutai/internal/store"
)

func TestSpikeMCPTools(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	h.createFeature("pf", "login", "Login", "Sign in.")
	h.createFeature("pf", "other", "Other", "Something else.")
	feat, err := h.srv.featureByPath(ctx, "pf/login")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := h.srv.featureByPath(ctx, "pf/other")
	initiative, err := store.GetInitiative(ctx, h.srv.Store.Pool, in)
	if err != nil {
		t.Fatal(err)
	}

	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		out, isErr, msg := h.callTool(name, args)
		if isErr {
			t.Fatalf("%s: %s", name, msg)
		}
		return out
	}
	refuse := func(name string, args map[string]any, want string) {
		t.Helper()
		_, isErr, msg := h.callTool(name, args)
		if !isErr || !strings.Contains(msg, want) {
			t.Errorf("%s %v: isError=%v msg=%q, want %q", name, args, isErr, msg, want)
		}
	}

	// Create, on an initiative by path and on a feature by ID and by path.
	a := call("create_spike", map[string]any{"on": "pf", "question": "Does the API honour the header?"})
	b := call("create_spike", map[string]any{"on": feat.PublicID, "question": "Is bcrypt fast enough?", "budget": 50000})
	c := call("create_spike", map[string]any{"on": "pf/login", "question": "Can we drop the cache?"})
	d := call("create_spike", map[string]any{"on": initiative.PublicID, "question": "By initiative ID?"})
	for i, r := range []map[string]any{a, b, c, d} {
		want := []string{"SPK-001", "SPK-002", "SPK-003", "SPK-004"}[i]
		if r["id"] != want || r["state"] != "idea" || r["created_via"] != "mcp" {
			t.Errorf("create %d = %v", i, r)
		}
		if r["next"] != "A person can start it from its page in the web UI." {
			t.Errorf("create %d next = %v", i, r["next"])
		}
	}
	if o := a["owner"].(map[string]any); o["type"] != "initiative" || o["id"] != initiative.PublicID || o["name"] != initiative.Name {
		t.Errorf("a owner = %v", o)
	}
	if o := b["owner"].(map[string]any); o["type"] != "feature" || o["id"] != feat.PublicID || o["name"] != "Login" {
		t.Errorf("b owner = %v", o)
	}
	if o := c["owner"].(map[string]any); o["id"] != feat.PublicID {
		t.Errorf("c owner = %v", o)
	}
	cfg, err := h.srv.freshConfig()
	if err != nil {
		t.Fatal(err)
	}
	if a["budget"] != float64(cfg.SpikeDefaultTokenBudget()) || a["budget_source"] != "project default" {
		t.Errorf("a budget = %v (%v)", a["budget"], a["budget_source"])
	}
	if b["budget"] != float64(50000) || b["budget_source"] != "set on the spike" {
		t.Errorf("b budget = %v (%v)", b["budget"], b["budget_source"])
	}

	// Recorded as MCP, still an idea, nothing dispatched.
	for _, r := range []map[string]any{a, b, c, d} {
		sp, err := store.SpikeByPublicID(ctx, h.srv.Store.Pool, r["id"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if sp.CreatedVia != "mcp" || sp.CreatedBy != h.srv.mcpActor() || sp.State != store.SpikeIdea {
			t.Errorf("%s: via %s by %s state %s", sp.PublicID, sp.CreatedVia, sp.CreatedBy, sp.State)
		}
		if n := len(h.runsFor("spike", sp.ID)); n != 0 {
			t.Errorf("%s: creating dispatched %d runs", sp.PublicID, n)
		}
	}

	// Refusals.
	refuse("create_spike", map[string]any{"on": "pf", "question": "   "}, "A spike needs a question")
	refuse("create_spike", map[string]any{"on": "pf", "question": strings.Repeat("q", 501)}, "at most 500 characters; this one is 501")
	refuse("create_spike", map[string]any{"on": "pf", "question": "Fine?", "budget": 0}, "positive whole number of tokens")
	refuse("create_spike", map[string]any{"on": "SPK-001", "question": "Fine?"}, "not on another spike")
	refuse("create_spike", map[string]any{"on": "nope/nothing", "question": "Fine?"}, "there is no initiative or feature at")
	refuse("create_spike", map[string]any{"question": "Fine?"}, "\"on\"")
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE features SET state = 'done' WHERE id = $1`, other.ID); err != nil {
		t.Fatal(err)
	}
	refuse("create_spike", map[string]any{"on": "pf/other", "question": "Fine?"}, "That feature is done")
	if sp, err := h.srv.CreateSpike(ctx, "initiative", in, "Next number?", nil, "sam", "ui"); err != nil || sp.PublicID != "SPK-005" {
		t.Fatalf("after refusals next = %v, %v", sp, err)
	}

	// List, with and without filters.
	ids := func(out map[string]any) []string {
		var got []string
		for _, s := range out["spikes"].([]any) {
			got = append(got, s.(map[string]any)["id"].(string))
		}
		return got
	}
	eq := func(got []string, want ...string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v", got, want)
		}
	}
	eq(ids(call("list_spikes", nil)), "SPK-001", "SPK-002", "SPK-003", "SPK-004", "SPK-005")
	eq(ids(call("list_spikes", map[string]any{"on": "pf/login"})), "SPK-002", "SPK-003")
	eq(ids(call("list_spikes", map[string]any{"on": feat.PublicID})), "SPK-002", "SPK-003")
	eq(ids(call("list_spikes", map[string]any{"on": "pf"})), "SPK-001", "SPK-004", "SPK-005")
	eq(ids(call("list_spikes", map[string]any{"state": "running"})))
	eq(ids(call("list_spikes", map[string]any{"state": "idea", "on": "pf/login"})), "SPK-002", "SPK-003")
	refuse("list_spikes", map[string]any{"state": "bogus"}, "idea, running, ended or closed")
	refuse("list_spikes", map[string]any{"on": "SPK-001"}, "not on another spike")
	first := call("list_spikes", nil)["spikes"].([]any)[0].(map[string]any)
	if first["question"] == nil || first["owner"] == nil || first["budget"] == nil || first["tokens_used"] == nil {
		t.Errorf("list entry is thin: %v", first)
	}

	// Get an idea: the optional fields are empty.
	g := call("get_spike", map[string]any{"spike": "SPK-001"})
	if g["findings"] != nil || g["run_id"] != nil || g["follows"] != nil || g["ended_how"] != nil || g["closed_as"] != nil || g["draft"] != nil {
		t.Errorf("get of an idea = %v", g)
	}
	if fb, ok := g["followed_by"].([]any); !ok || len(fb) != 0 {
		t.Errorf("followed_by = %v", g["followed_by"])
	}
	refuse("get_spike", map[string]any{"spike": "SPK-099"}, "there is no spike")
	refuse("get_spike", map[string]any{"spike": "FEAT-001"}, "there is no spike")

	// Drive one to its end through the service, then read every field.
	ended := h.endedSpike(in, "Is the answer written down?", nil)
	got := call("get_spike", map[string]any{"spike": ended.PublicID})
	if got["state"] != "ended" || got["ended_how"] != store.SpikeConcluded {
		t.Errorf("ended get = %v", got)
	}
	if got["tokens_used"] == float64(0) || got["budget"] != float64(cfg.SpikeDefaultTokenBudget()) || got["budget_source"] != "given at the start" {
		t.Errorf("ended budget/tokens = %v / %v / %v", got["budget"], got["budget_source"], got["tokens_used"])
	}
	doc, _ := h.findingsOf(ended)
	fnd, _ := got["findings"].(map[string]any)
	if fnd == nil || fnd["id"] != doc.PublicID || fnd["path"] != doc.Path || fnd["state"] != string(doc.State) {
		t.Errorf("findings = %v, want %s %s", got["findings"], doc.PublicID, doc.Path)
	}
	runs := h.runsFor("spike", ended.ID)
	if len(runs) == 0 || got["run_id"] != runs[len(runs)-1].ID.String() {
		t.Errorf("run_id = %v, runs %d", got["run_id"], len(runs))
	}

	// Close it unanswered, ask again, and read the chain both ways.
	if _, err := h.srv.CloseSpike(ctx, ended.ID, SpikeCloseAgain, nil, "sam"); err != nil {
		t.Fatal(err)
	}
	closed := call("get_spike", map[string]any{"spike": ended.PublicID})
	if closed["state"] != "closed" || closed["closed_as"] == nil {
		t.Errorf("closed get = %v", closed)
	}
	var next *store.Spike
	all, _ := store.ListSpikes(ctx, h.srv.Store.Pool, store.SpikeFilter{})
	for i := range all {
		if all[i].FollowsID != nil && *all[i].FollowsID == ended.ID {
			next = &all[i]
		}
	}
	if next == nil {
		t.Fatal("asking again made no following spike")
	}
	if fb := closed["followed_by"].([]any); len(fb) != 1 || fb[0] != next.PublicID {
		t.Errorf("followed_by = %v, want %s", fb, next.PublicID)
	}
	if n := call("get_spike", map[string]any{"spike": next.PublicID}); n["follows"] != ended.PublicID {
		t.Errorf("follows = %v, want %s", n["follows"], ended.PublicID)
	}

	// A running spike shows its draft.
	run := h.newSpike(in, "Is it still running?", nil)
	h.startSpikeQuiet(run, 1000)
	if err := store.SaveSpikeDraft(ctx, h.srv.Store.Pool, run.ID, "## Answer\n\nSo far, yes.\n"); err != nil {
		t.Fatal(err)
	}
	rg := call("get_spike", map[string]any{"spike": run.PublicID})
	if rg["state"] != "running" || rg["draft"] != "## Answer\n\nSo far, yes.\n" || rg["budget"] != float64(1000) {
		t.Errorf("running get = %v", rg)
	}

	// FR-8.6: a spike can't be a milestone deliverable, over MCP, REST or the UI form.
	call("create_milestone", map[string]any{"name": "Release one"})
	want := "A spike can't be a milestone deliverable, because it ships nothing. Add the feature its findings led to instead."
	for _, mt := range []string{"feature", "bug", "initiative", "checklist", "milestone"} {
		refuse("add_milestone_member", map[string]any{"milestone": "Release one", "member_type": mt, "member": "SPK-001"}, want)
	}
	code, out := h.call("POST", "/api/milestones/members", map[string]any{"milestone": "Release one", "ref": "SPK-001", "action": "add"})
	if code != 404 || !strings.Contains(strings.ToLower(jsonText(out)), "can't be a milestone deliverable") {
		t.Errorf("REST add of a spike: %d %v", code, out)
	}
	spk, _ := store.SpikeByPublicID(ctx, h.srv.Store.Pool, "SPK-001")
	form := url.Values{"member_type": {"spike"}, "member_id": {spk.ID.String()}}
	req := httptest.NewRequest("POST", "/ui/milestones/members", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = req.ParseForm()
	if _, _, _, err := h.srv.memberFromForm(ctx, req); err == nil || err.Error() != want {
		t.Errorf("UI add of a spike: %v", err)
	}
	form = url.Values{"member_type": {"feature"}, "member_id": {spk.ID.String()}}
	req = httptest.NewRequest("POST", "/ui/milestones/members", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = req.ParseForm()
	if _, _, _, err := h.srv.memberFromForm(ctx, req); err == nil || err.Error() != want {
		t.Errorf("UI add of a spike as a feature: %v", err)
	}
	if _, total := h.milestoneItems(h.milestoneNamed("Release one").ID); total != 0 {
		t.Errorf("a spike joined the milestone")
	}
}

func jsonText(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
