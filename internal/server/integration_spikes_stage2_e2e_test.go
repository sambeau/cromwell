package server

// SPEC-021 FR-18: the integration checks between M13 and M14 and the stage's
// end-to-end tests. A spike run in chat or by a person is held by M13's claim
// expiry and by its own deadline; its executor is recorded; its tokens are
// unmeasured; and the whole run, start to close, works through the routes and
// tools a person and the chat agent really have.

import (
	"context"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// startedViaScreen starts a new chat spike through the UI's POST, with a time
// box of 4 hours, as the start screen does.
func (h *harness) startedViaScreen(question string) *store.Spike {
	h.t.Helper()
	in := h.spikeInitiative("s2" + strings.ToLower(uuid.NewString()[:6]))
	return h.startViaScreen(h.newSpike(in, question, nil), "chat")
}

func (h *harness) startViaScreen(sp *store.Spike, executor string) *store.Spike {
	h.t.Helper()
	if code, body := h.postText("/ui/spikes/start", spikeForm(sp, "executor", executor, "time_box", "4")); code != 200 {
		h.t.Fatalf("start %s as %s = %d: %s", sp.PublicID, executor, code, truncate(body, 600))
	}
	got := h.getSpike(sp.ID)
	if got.State != store.SpikeRunning || got.Executor != executor {
		h.t.Fatalf("after the start: %s, executor %q", got.State, got.Executor)
	}
	return got
}

// heartbeatDuties runs the heartbeat's two duties that touch a spike, in its
// order (server.go: ClaimSweep, then ReconcileSpikes).
func (h *harness) heartbeatDuties() {
	h.t.Helper()
	ctx := context.Background()
	h.srv.ClaimSweep(ctx)
	h.srv.ReconcileSpikes(ctx)
}

// worktreePaths is the paths `git worktree list` shows, without the commit
// each is at (the main checkout's moves with the findings commit).
func (h *harness) worktreePaths() string {
	var paths []string
	for _, l := range strings.Split(strings.TrimSpace(h.worktreeList()), "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			paths = append(paths, f[0])
		}
	}
	return strings.Join(paths, "\n")
}

// closerRanItAudit is the spike.closed audit row's closer_ran_it, or "" if absent.
func (h *harness) closerRanItAudit(id uuid.UUID) string {
	h.t.Helper()
	var s *string
	if err := h.srv.Store.Pool.QueryRow(context.Background(), `SELECT payload->>'closer_ran_it' FROM audit_events
		WHERE kind = 'spike.closed' AND ref_id = $1`, id).Scan(&s); err != nil {
		h.t.Fatalf("audit row: %v", err)
	}
	if s == nil {
		return ""
	}
	return *s
}

// A chat spike's claim carries the deadline; inactivity raises claim-stale;
// saving withdraws it; the deadline ends the spike.
func TestChatSpikeIsHeldByItsClaimAndDeadline(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.startedViaScreen("Does the queue keep its order?")
	if sp.TimeBoxHours == nil || *sp.TimeBoxHours != 4 {
		t.Fatalf("time box = %v", sp.TimeBoxHours)
	}

	h.toolOK("claim_spike", map[string]any{"spike": sp.PublicID})
	c := h.latestSpikeClaim(sp)
	if c.FeatureID != nil || c.State != lifecycle.ClaimOpen || c.DeadlineAt == nil || sp.DeadlineAt == nil || !c.DeadlineAt.Equal(*sp.DeadlineAt) {
		t.Fatalf("claim = %+v, spike deadline %v", c, sp.DeadlineAt)
	}

	// Inactivity past claims.expiry_hours: M13's question, on the spike.
	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-stale", "spike", sp.ID) != nil {
		t.Fatal("a fresh claim is stale")
	}
	h.staleClaim(c)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "spike", sp.ID)
	if cp == nil {
		t.Fatal("inactivity raised no claim-stale on the spike")
	}
	if h.pendingOf("claim-deadline", "spike", sp.ID) != nil {
		t.Error("a spike raised claim-deadline")
	}

	// Saving findings is activity and withdraws the question.
	h.toolOK("save_spike_findings", map[string]any{"spike": sp.PublicID, "findings": goodFindings})
	if h.pendingOf("claim-stale", "spike", sp.ID) != nil {
		t.Error("saving findings left the claim-stale question pending")
	}
	if st := h.checkpointState(cp.ID); st == "pending" {
		t.Errorf("the question is still %s", st)
	}

	// The deadline passes: the heartbeat ends the spike as time_box.
	h.pastDeadline(sp)
	h.heartbeatDuties()
	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeTimeBox {
		t.Fatalf("spike = %s, %q", cur.State, cur.EndedHow)
	}
	if cur.WorktreeRemovedAt == nil {
		t.Error("the worktree wasn't discarded")
	}
	if _, err := os.Stat(h.spikeDir(sp)); !os.IsNotExist(err) {
		t.Errorf("the worktree directory remains: %v", err)
	}
	doc, text := h.findingsOf(sp)
	if !strings.Contains(text, "Yes: the API honours the header.") || !strings.Contains(text, "The spike reached the end of its time box of 4 hours.") {
		t.Errorf("the findings:\n%s", text)
	}
	if out := h.gitOut("ls-tree", "--name-only", "HEAD", "--", doc.Path); strings.TrimSpace(out) == "" {
		t.Errorf("%s isn't committed", doc.Path)
	}
	if c := h.latestSpikeClaim(sp); c.State != lifecycle.ClaimEnded || c.EndReason != "expired" {
		t.Errorf("claim = %+v", c)
	}
}

// An agent, a chat and a person spike each record their executor on the row
// and in executions.
func TestSpikeExecutorIsRecorded(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")

	// The agent spike runs to its conclusion on the mock provider.
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	agent := h.startSpike(h.newSpike(in, "Who ran the agent's?", nil), 0)
	agent = h.spikeSettled(agent)
	h.quiet()
	execs := h.spikeExecs(agent)
	runs := h.runsFor("spike", agent.ID)
	if len(execs) != 1 || len(runs) != 1 {
		t.Fatalf("agent executions %+v, runs %+v", execs, runs)
	}
	e := execs[0]
	if e.Kind != store.ExecutorAgent || e.DispatchID == nil || *e.DispatchID != runs[0].ID || e.Actor != runs[0].Role || e.Model != runs[0].Model ||
		!e.Measured || e.RefType != "spike" || e.Round != 1 {
		t.Errorf("agent execution = %+v", e)
	}

	// The chat spike, claimed over MCP.
	chat := h.startViaScreen(h.newSpike(in, "Who ran the chat's?", nil), "chat")
	h.toolOK("claim_spike", map[string]any{"spike": chat.PublicID})
	c := h.latestSpikeClaim(chat)
	execs = h.spikeExecs(chat)
	if len(execs) != 1 || execs[0].Kind != store.WriterChat || execs[0].ClaimID == nil || *execs[0].ClaimID != c.ID || execs[0].Measured ||
		execs[0].RefType != "spike" || execs[0].Round != 1 {
		t.Errorf("chat executions = %+v", execs)
	}

	// The person spike, claimed in the UI.
	person := h.startViaScreen(h.newSpike(in, "Who ran the person's?", nil), "person")
	if code, body := h.postText("/ui/spikes/claim", spikeForm(person)); code != 200 || !strings.Contains(body, "You are running "+person.PublicID+".") {
		t.Fatalf("claim in the UI = %d: %s", code, truncate(body, 600))
	}
	execs = h.spikeExecs(person)
	if len(execs) != 1 || execs[0].Kind != store.WriterPerson || execs[0].Via != "ui" || execs[0].Measured || execs[0].RefType != "spike" {
		t.Errorf("person executions = %+v", execs)
	}

	// The rows on spikes, and get_spike.
	for sp, want := range map[*store.Spike]string{agent: store.ExecutorAgent, chat: store.ExecutorChat, person: store.ExecutorPerson} {
		if got := h.getSpike(sp.ID).Executor; got != want {
			t.Errorf("%s: spikes.executor = %q, want %q", sp.PublicID, got, want)
		}
		out := h.toolOK("get_spike", map[string]any{"spike": sp.PublicID})
		if ex := asMap(t, out["executor"]); ex["kind"] != want {
			t.Errorf("%s: get_spike executor = %v, want %s", sp.PublicID, ex, want)
		}
		if out["measured"] != (want == store.ExecutorAgent) {
			t.Errorf("%s: get_spike measured = %v", sp.PublicID, out["measured"])
		}
	}

	// The executor's sentence, the same on the page and in get_spike
	// (FR-16.1, FR-16.2).
	for sp, prefix := range map[*store.Spike]string{
		agent:  "Run by the spike runner (" + runs[0].Model + ").",
		chat:   "Being run in chat by the chat agent, who claimed it",
		person: "Being run by hand by ",
	} {
		ex := asMap(t, h.toolOK("get_spike", map[string]any{"spike": sp.PublicID})["executor"])
		sentence, _ := ex["sentence"].(string)
		if !strings.HasPrefix(sentence, prefix) {
			t.Errorf("%s: get_spike executor sentence = %q, want it to start %q", sp.PublicID, sentence, prefix)
		}
		if code, body := h.getUI("/ui/s/" + sp.PublicID); code != 200 || !strings.Contains(body, html.EscapeString(sentence)) {
			t.Errorf("%s: the page doesn't say %q", sp.PublicID, sentence)
		}
	}

	// Once the chat spike is submitted, both say who ran it.
	h.toolOK("submit_spike", map[string]any{"spike": chat.PublicID, "findings": goodFindings})
	ex := asMap(t, h.toolOK("get_spike", map[string]any{"spike": chat.PublicID})["executor"])
	if ex["sentence"] != "Run in chat by the chat agent." {
		t.Errorf("ended chat spike: get_spike executor sentence = %v", ex["sentence"])
	}
	if _, body := h.getUI("/ui/s/" + chat.PublicID); !strings.Contains(body, "Run in chat by the chat agent.") {
		t.Error("the ended chat spike's page doesn't say who ran it")
	}
}

// A change in a claimed spike's detached working copy is the claim's
// activity, and withdraws a pending claim-stale (FR-14.3, SPEC-020 FR-5.2).
func TestSpikeWorkingCopyChangeIsClaimActivity(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.chatSpike()
	res := h.claimSpike(sp)
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-stale", "spike", sp.ID) == nil {
		t.Fatal("a stale spike claim raised no claim-stale")
	}
	if err := os.WriteFile(filepath.Join(res.WorkingCopy.Path, "scratch.txt"), []byte("trying something\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.srv.ClaimSweep(ctx)
	if c := h.latestSpikeClaim(sp); c.LastActivity != "worktree" {
		t.Errorf("last activity = %q, want worktree", c.LastActivity)
	}
	if h.pendingOf("claim-stale", "spike", sp.ID) != nil {
		t.Error("a change in the working copy didn't withdraw claim-stale")
	}
}

var tokenCountRe = regexp.MustCompile(`\d[\d,.]*\s*(k |M )?tokens`)

// A chat spike's tokens are unmeasured everywhere they would show.
func TestChatSpikeTokensAreUnmeasured(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	probe := h.newSpike(in, "What does the screen forecast?", nil)
	before := h.uiText(startURL(probe))
	samplesBefore, _ := store.PurposeTokenSamples(ctx, h.srv.Store.Pool, "run-spike", 20)
	forecastBefore, okBefore := h.srv.stepForecast(ctx, "run-spike")

	sp := h.startViaScreen(h.newSpike(in, "Is the chat spike measured?", nil), "chat")
	h.toolOK("claim_spike", map[string]any{"spike": sp.PublicID})
	h.toolOK("save_spike_findings", map[string]any{"spike": sp.PublicID, "findings": goodFindings})
	h.toolOK("submit_spike", map[string]any{"spike": sp.PublicID})

	execs := h.spikeExecs(sp)
	if len(execs) == 0 {
		t.Fatal("no executions")
	}
	for _, e := range execs {
		if e.Measured || e.DispatchID != nil {
			t.Errorf("execution = %+v, want unmeasured, with no run to count tokens from", e)
		}
	}
	if cur := h.getSpike(sp.ID); cur.State != store.SpikeEnded || cur.TokensUsed != 0 {
		t.Errorf("spike = %s, tokens %d", cur.State, cur.TokensUsed)
	}

	out := h.toolOK("get_spike", map[string]any{"spike": sp.PublicID})
	if v, ok := out["tokens_used"]; !ok || v != nil {
		t.Errorf("tokens_used = %v, want null", v)
	}
	if out["measured"] != false {
		t.Errorf("measured = %v", out["measured"])
	}

	page := h.uiText("/ui/s/" + sp.PublicID)
	wants(t, "the chat spike's page", page, "This spike ran in chat, so its tokens weren't measured.")
	lacks(t, "the chat spike's page", page, `id="spike-tokens"`, "spike-tokens-line", "bar-fill")

	_, text := h.findingsOf(sp)
	if !strings.Contains(text, "It ran in chat, so its tokens weren't measured.") {
		t.Errorf("the findings lack the unmeasured sentence:\n%s", text)
	}
	if m := tokenCountRe.FindString(text); m != "" {
		t.Errorf("the findings carry a token count %q:\n%s", m, text)
	}

	// The start screen's forecast ignores it.
	samplesAfter, _ := store.PurposeTokenSamples(ctx, h.srv.Store.Pool, "run-spike", 20)
	forecastAfter, okAfter := h.srv.stepForecast(ctx, "run-spike")
	if len(samplesAfter) != len(samplesBefore) || forecastAfter != forecastBefore || okAfter != okBefore {
		t.Errorf("the forecast moved: %v %d %v -> %v %d %v", samplesBefore, forecastBefore, okBefore, samplesAfter, forecastAfter, okAfter)
	}
	forecastLine := func(s string) string {
		for _, l := range strings.Split(s, "\n") {
			if strings.Contains(l, "orecast") || strings.Contains(l, "earlier spikes") {
				return strings.TrimSpace(l)
			}
		}
		return ""
	}
	after := h.uiText(startURL(probe))
	if forecastLine(before) == "" || forecastLine(before) != forecastLine(after) {
		t.Errorf("the start screen's forecast changed:\n%q\n%q", forecastLine(before), forecastLine(after))
	}
}

// The chat spike's whole run: written down over MCP, started in the UI,
// claimed, saved and submitted over MCP, closed in the UI.
func TestChatSpikeEndToEnd(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.spikeInitiative("pf")

	created := h.toolOK("create_spike", map[string]any{"on": "pf", "question": "Does the header win over the cookie?"})
	id, _ := created["id"].(string)
	sp, err := store.SpikeByPublicID(ctx, h.srv.Store.Pool, id)
	if err != nil {
		t.Fatal(err)
	}
	headBefore, branchesBefore, treesBefore := h.headSHA(), h.gitOut("branch", "--list"), h.worktreePaths()

	h.startViaScreen(sp, "chat")
	h.toolOK("claim_spike", map[string]any{"spike": id})
	if !strings.Contains(h.worktreeList(), "spk-") {
		t.Errorf("no spike worktree while it runs:\n%s", h.worktreeList())
	}
	h.toolOK("save_spike_findings", map[string]any{"spike": id, "findings": goodFindings})
	sub := h.toolOK("submit_spike", map[string]any{"spike": id})
	if entry := asMap(t, sub["spike"]); entry["state"] != store.SpikeEnded {
		t.Errorf("submitted spike = %v", entry)
	}
	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeConcluded {
		t.Fatalf("after submit: %s, %q", cur.State, cur.EndedHow)
	}

	// Only the findings commit was added; no branch; the worktree is gone.
	doc, _ := h.findingsOf(sp)
	if n := strings.TrimSpace(h.gitOut("rev-list", "--count", headBefore+"..HEAD")); n != "1" {
		t.Errorf("%s commits were added, want 1", n)
	}
	if files := strings.Fields(h.gitOut("diff", "--name-only", headBefore, "HEAD")); len(files) != 1 || files[0] != doc.Path {
		t.Errorf("the commit changed %v, want only %s", files, doc.Path)
	}
	if got := h.gitOut("branch", "--list"); got != branchesBefore {
		t.Errorf("branches changed:\n%q\n%q", branchesBefore, got)
	}
	if got := h.worktreePaths(); got != treesBefore {
		t.Errorf("worktrees changed:\n%q\n%q", treesBefore, got)
	}

	// MCP can't close it; a person does, in the UI.
	resp := h.rpc("tools/call", map[string]any{"name": "close_spike", "arguments": map[string]any{"spike": id, "as": "answered"}})
	if resp.Error == nil || resp.Error.Code != rpcMethodNotFound {
		t.Fatalf("close_spike over MCP = %+v, want method-not-found", resp.Error)
	}
	if h.getSpike(sp.ID).State != store.SpikeEnded {
		t.Error("the MCP call changed the spike")
	}
	_, body := h.postText("/ui/spikes/close", spikeForm(sp, "as", "answered"))
	wants(t, "the closed page", body, id+" is closed: the question is answered.")
	closed := h.getSpike(sp.ID)
	if closed.State != store.SpikeClosed || closed.ClosedAs != store.SpikeAnswered || closed.EndedHow != store.SpikeConcluded {
		t.Errorf("closed = %s as %q, ended %q", closed.State, closed.ClosedAs, closed.EndedHow)
	}
	if got := h.closerRanItAudit(sp.ID); got == "true" {
		t.Errorf("closer_ran_it = %s for a chat spike", got)
	}
}

// The person's whole run, through the UI routes alone.
func TestPersonSpikeEndToEnd(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Can the export be run by hand?", nil)
	headBefore, branchesBefore, treesBefore := h.headSHA(), h.gitOut("branch", "--list"), h.worktreePaths()

	h.startViaScreen(sp, "person")
	if code, _ := h.postText("/ui/spikes/claim", spikeForm(sp)); code != 200 {
		t.Fatalf("claim = %d", code)
	}
	if code, _ := h.postText("/ui/spikes/draft", spikeForm(sp, "findings", "## Answer\n\nA start.")); code != 200 {
		t.Fatalf("save = %d", code)
	}
	_, body := h.postText("/ui/spikes/submit", spikeForm(sp, "findings", goodFindings))
	wants(t, "the finished page", body, "You finished "+sp.PublicID+".", "This spike was run by hand, so its tokens weren't measured.")
	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeConcluded || cur.TokensUsed != 0 {
		t.Fatalf("after finishing: %s, %q, %d tokens", cur.State, cur.EndedHow, cur.TokensUsed)
	}
	doc, _ := h.findingsOf(sp)
	if files := strings.Fields(h.gitOut("diff", "--name-only", headBefore, "HEAD")); len(files) != 1 || files[0] != doc.Path {
		t.Errorf("the commits changed %v, want only %s", files, doc.Path)
	}
	if got := h.gitOut("branch", "--list"); got != branchesBefore {
		t.Errorf("branches changed:\n%q\n%q", branchesBefore, got)
	}
	if got := h.worktreePaths(); got != treesBefore {
		t.Errorf("worktrees changed:\n%q\n%q", treesBefore, got)
	}

	_, body = h.postText("/ui/spikes/close", spikeForm(sp, "as", "answered"))
	wants(t, "the closed page", body, sp.PublicID+" is closed: the question is answered.", "Closed by "+h.srv.uiActor()+", who also ran it.")
	if got := h.closerRanItAudit(sp.ID); got != "true" {
		t.Errorf("closer_ran_it = %q, want true", got)
	}
	execs := h.spikeExecs(sp)
	if len(execs) != 1 || execs[0].Kind != store.WriterPerson || execs[0].Measured {
		t.Errorf("executions = %+v", execs)
	}
}
