package server

// The spikes suite's web half for who runs a spike (SPEC-021 stage 2, FR-12.1,
// FR-12.5, FR-14.2, FR-15.5, FR-16.1, FR-16.3, FR-16.4, FR-17.3): the start
// screen's Who runs it, the spike's page for a chat or person spike, the four
// routes that run a spike by hand, and the Inbox's release words.

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"subutai/internal/store"
)

func spikeForm(sp *store.Spike, kv ...string) url.Values {
	v := url.Values{"spike": {sp.ID.String()}}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

// radioTag is the start screen's executor radio for value, as its tag's text,
// so a test reads its attributes and not the page around them.
func radioTag(t *testing.T, page, value string) string {
	t.Helper()
	i := strings.Index(page, `name="executor" value="`+value+`"`)
	if i < 0 {
		t.Fatalf("no %q executor radio on the page", value)
	}
	start := strings.LastIndex(page[:i], "<input")
	end := i + strings.Index(page[i:], ">")
	return page[start : end+1]
}

func TestSpikeStartScreenOffersWhoRunsIt(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?", nil)

	screen := h.uiText(startURL(sp))
	wants(t, "the start screen", screen,
		`name="executor" value="agent"`, `name="executor" value="chat"`, `name="executor" value="person"`,
		"The spike runner", "The chat agent", ">You<",
		"An agent runs it on ", ", with a token budget.",
		"You or the chat agent ask it to run "+sp.PublicID+" in a chat session. It claims the spike over MCP.",
		"You run it by hand, from this spike's page.",
		"Time box, in hours", `name="time_box" inputmode="numeric" value="4"`,
		"Work done in chat or by hand can't be measured in tokens, so this spike is held to a time box instead. When the time box runs out, the spike ends with whatever findings have been saved.",
		"Budget in tokens", "describe the spike runner")
	if r := radioTag(t, screen, "agent"); !strings.Contains(r, " checked") || strings.Contains(r, "disabled") {
		t.Errorf("the spike runner isn't chosen by default: %s", r)
	}
	lacks(t, "the start screen", screen, "disabled")

	// With nobody assigned: the runner is disabled with the refusal beside it,
	// and the chat agent is chosen.
	h.editConfig("  run-spike: spike-runner\n", "  # run-spike: spike-runner\n")
	screen = h.uiText(startURL(sp))
	wants(t, "the start screen with no runner", screen, "Nobody is assigned to run spikes, so this spike can't start.")
	if r := radioTag(t, screen, "agent"); !strings.Contains(r, " disabled") || strings.Contains(r, " checked") {
		t.Errorf("the spike runner's radio with nobody assigned: %s", r)
	}
	if r := radioTag(t, screen, "chat"); !strings.Contains(r, " checked") {
		t.Errorf("the chat agent isn't chosen when nobody can run spikes: %s", r)
	}
	// Start stays available: the chat agent and a person need no runner.
	if b := startButton(t, screen); strings.Contains(b, "disabled") {
		t.Errorf("the Start button is disabled although the chat agent can run it: %s", b)
	}
}

// startButton is the start screen's submit button tag.
func startButton(t *testing.T, page string) string {
	t.Helper()
	i := strings.Index(page, "Start this spike</button>")
	if i < 0 {
		t.Fatalf("no Start button on the page")
	}
	start := strings.LastIndex(page[:i], "<button")
	return page[start:i]
}

// The Start button is disabled only when nothing can start: with no findings
// template, every start would be refused.
func TestSpikeStartIsDisabledWithoutAFindingsTemplate(t *testing.T) {
	h := newHarness(t)
	sp := h.newSpike(h.spikeInitiative("pf"), "Can it start without a template?", nil)
	tmpl := filepath.Join(h.root, ".subutai", "templates", "findings", "template.md")
	if err := os.Rename(tmpl, tmpl+".away"); err != nil {
		t.Fatal(err)
	}
	screen := h.uiText(startURL(sp))
	wants(t, "the start screen with no template", screen, "This project has no findings template yet")
	if b := startButton(t, screen); !strings.Contains(b, "disabled") || !strings.Contains(b, `aria-describedby="start-template"`) {
		t.Errorf("the Start button with no template: %s", b)
	}
	if err := os.Rename(tmpl+".away", tmpl); err != nil {
		t.Fatal(err)
	}
	if b := startButton(t, h.uiText(startURL(sp))); strings.Contains(b, "disabled") {
		t.Errorf("the Start button with the template back: %s", b)
	}
}

func TestSpikeStartsInChatAndByHandFromTheScreen(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	// A chat or person spike starts even when nobody can run spikes.
	h.editConfig("  run-spike: spike-runner\n", "  # run-spike: spike-runner\n")

	bad := h.newSpike(in, "Is the time box checked?", nil)
	for _, v := range []string{"0", "169", "four", "-1", "2.5"} {
		code, body := h.postText("/ui/spikes/start", spikeForm(bad, "executor", "chat", "time_box", v))
		if code != 200 {
			t.Fatalf("time box %q = %d", v, code)
		}
		wants(t, "the refused start", body, "A spike's time box is a whole number of hours, from 1 to 168.")
		if got := h.getSpike(bad.ID); got.State != store.SpikeIdea {
			t.Errorf("a time box of %q left the spike %s", v, got.State)
		}
	}

	chat := h.newSpike(in, "Does the queue keep its order?", nil)
	code, body := h.postText("/ui/spikes/start", spikeForm(chat, "executor", "chat", "time_box", "4"))
	if code != 200 {
		t.Fatalf("starting in chat = %d", code)
	}
	wants(t, "the chat spike's page", body,
		chat.PublicID+" has started, with a time box of 4 hours. Ask the chat agent to run it.",
		"Ask the chat agent to run "+chat.PublicID+". It claims the spike with <code>claim_spike</code>.",
		"To be run in chat. Waiting for the chat agent to claim it.", "Time box: 4 hours",
		"This spike ran in chat, so its tokens weren't measured.")
	lacks(t, "the chat spike's page", body, "spike-tokens", "Release the claim")
	if got := h.getSpike(chat.ID); got.State != store.SpikeRunning || got.Executor != store.ExecutorChat {
		t.Errorf("the chat spike: %s %q", got.State, got.Executor)
	}

	// An empty field is the project's default.
	hand := h.newSpike(in, "Can it be run by hand?", nil)
	_, body = h.postText("/ui/spikes/start", spikeForm(hand, "executor", "person", "time_box", ""))
	wants(t, "the person spike's page", body,
		hand.PublicID+" has started, with a time box of 4 hours. Claim it below when you are ready.",
		"I'll run this spike", "To be run by hand. Waiting for a person to claim it.")
	if got := h.getSpike(hand.ID); got.TimeBoxHours == nil || *got.TimeBoxHours != 4 {
		t.Errorf("time box: %v", got.TimeBoxHours)
	}

	// A notice not borne out by the spike's state isn't shown.
	other := h.newSpike(in, "Is a made-up notice shown?", nil)
	lacks(t, "an idea's page", h.uiText("/ui/s/"+other.PublicID+"?did=started"), "has started")

	// An unknown runner is refused, and the spike stays an idea.
	if _, body := h.postText("/ui/spikes/start", spikeForm(other, "executor", "robot")); !strings.Contains(body, "A spike is run by the spike runner, the chat agent or a person.") {
		t.Errorf("an unknown executor: %s", truncate(body, 600))
	}
	if h.getSpike(other.ID).State != store.SpikeIdea {
		t.Error("an unknown executor started the spike")
	}
	_ = ctx
}

func TestPersonRunsASpikeThroughTheRoutes(t *testing.T) {
	h := newHarness(t)
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Does the API honour the header?", nil)
	if _, body := h.postText("/ui/spikes/start", spikeForm(sp, "executor", "person", "time_box", "3")); !strings.Contains(body, "Claim it below when you are ready.") {
		t.Fatalf("start: %s", truncate(body, 800))
	}

	// Claim it.
	_, body := h.postText("/ui/spikes/claim", spikeForm(sp))
	wants(t, "the claimed page", body,
		"You are running "+sp.PublicID+".", "Save findings", "I've finished", "Release the claim",
		"The time box ends at ", "Your findings", "Being run by hand by ")
	got := h.getSpike(sp.ID)
	wants(t, "the claimed page", body, h.srv.worktreeAbs(got.WorktreePath))
	lacks(t, "the claimed page", body, "I'll run this spike")

	// An empty draft can't be saved; a draft can.
	if _, body := h.postText("/ui/spikes/draft", spikeForm(sp, "findings", "  ")); !strings.Contains(body, "There are no findings to save.") {
		t.Errorf("an empty save: %s", truncate(body, 800))
	}
	_, body = h.postText("/ui/spikes/draft", spikeForm(sp, "findings", "## Answer\n\nIt is a start."))
	wants(t, "the saved page", body, "Your findings are saved.", "It is a start.")

	// Finishing with what is missing names it, and changes nothing.
	_, body = h.postText("/ui/spikes/submit", spikeForm(sp, "findings", "## Answer\n\nIt is a start."))
	wants(t, "the refused finish", body, "The findings can't be submitted yet:", "Nothing was changed.")
	if h.getSpike(sp.ID).State != store.SpikeRunning {
		t.Fatal("a refused finish ended the spike")
	}

	// Finishing ends it as concluded, and the page shows its findings.
	_, body = h.postText("/ui/spikes/submit", spikeForm(sp, "findings", goodFindings))
	wants(t, "the finished page", body, "You finished "+sp.PublicID+".", "Findings", "Run by hand by ",
		"This spike was run by hand, so its tokens weren't measured.", "Ask again")
	lacks(t, "the finished page", body, "A new budget, in tokens", "Ask again with a new budget")
	// No run, so no "run" sentences; and Ask again, with no field, has its own layout.
	wants(t, "the finished page", body, "This spike has ended: it reached a conclusion.", "triage-no-field")
	lacks(t, "the finished page", body, "run has ended", "triage-with-field")
	ended := h.getSpike(sp.ID)
	if ended.State != store.SpikeEnded || ended.EndedHow != store.SpikeConcluded {
		t.Fatalf("after finishing: %s %s", ended.State, ended.EndedHow)
	}

	// Close as answered, by the person who ran it.
	_, body = h.postText("/ui/spikes/close", spikeForm(sp, "as", "answered"))
	wants(t, "the closed page", body, sp.PublicID+" is closed: the question is answered.",
		"Closed by "+h.srv.uiActor()+", who also ran it.")
}

func TestPersonSpikeAskAgainHasNoBudget(t *testing.T) {
	h := newHarness(t)
	sp := h.startedSpike(store.ExecutorPerson, "Is the question asked again?")
	if _, err := h.srv.ClaimSpike(context.Background(), sp.PublicID, h.srv.PersonClaimant()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.SubmitSpike(context.Background(), sp.PublicID, h.srv.PersonClaimant(), goodFindings); err != nil {
		t.Fatal(err)
	}
	code, loc := h.postValues("/ui/spikes/close", spikeForm(sp, "as", "again", "budget", "garbage"))
	if code != http.StatusSeeOther || !strings.HasPrefix(loc, "/ui/spikes/start?spike=") {
		t.Fatalf("asking again = %d %s", code, loc)
	}
	closed := h.getSpike(sp.ID)
	if closed.State != store.SpikeClosed || closed.ClosedAs != store.SpikeUnanswered {
		t.Errorf("the first spike: %s %s", closed.State, closed.ClosedAs)
	}
	screen := h.uiText(loc)
	wants(t, "the new spike's start screen", screen, "Time box, in hours")
}

func TestChatSpikePageUnclaimedClaimedAndReleased(t *testing.T) {
	h := newHarness(t)
	sp := h.chatSpike()

	page := h.uiText("/ui/s/" + sp.PublicID)
	wants(t, "the unclaimed chat spike", page, "Ask the chat agent to run "+sp.PublicID+". It claims the spike with <code>claim_spike</code>.",
		"To be run in chat. Waiting for the chat agent to claim it.", "Time box: 4 hours, ending at ", "left)")
	lacks(t, "the unclaimed chat spike", page, "Release the claim", "I'll run this spike")

	h.claimSpike(sp)
	page = h.uiText("/ui/s/" + sp.PublicID)
	wants(t, "the claimed chat spike", page, "It is claimed by the chat agent, who claimed it ",
		"Last activity: claimed, ", "Release the claim", "Being run in chat by the chat agent")
	lacks(t, "the claimed chat spike", page, "Save findings", "I'll run this spike")

	_, body := h.postText("/ui/spikes/release", spikeForm(sp))
	wants(t, "the released page", body, "You released the claim on "+sp.PublicID+".",
		"Ask the chat agent to run "+sp.PublicID+".")
	if c := h.latestSpikeClaim(sp); c.State != "ended" {
		t.Errorf("the claim is %s", c.State)
	}
	if h.getSpike(sp.ID).State != store.SpikeRunning {
		t.Error("releasing ended the spike")
	}

	// Releasing a claim that isn't there is a refusal, not a new state.
	_, body = h.postText("/ui/spikes/release", spikeForm(sp))
	wants(t, "the refused release", body, sp.PublicID+" has no claim to release.")
}

func TestSpikeListsShowTheExecutor(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	idea := h.newSpike(in, "An idea nobody runs?", nil)
	chat := h.newSpike(in, "A chat question?", nil)
	hand := h.newSpike(in, "A by-hand question?", nil)
	agent := h.newSpike(in, "An agent question?", nil)
	for sp, ex := range map[*store.Spike]string{chat: store.ExecutorChat, hand: store.ExecutorPerson} {
		if _, err := h.srv.startSpike(ctx, sp.ID, SpikeStartRequest{Executor: ex, TimeBoxHours: 2}, "sam", false); err != nil {
			t.Fatal(err)
		}
	}
	h.startSpikeQuiet(agent, 0)

	row := func(page string, sp *store.Spike) string {
		i := strings.Index(page, `href="/ui/s/`+sp.PublicID+`"`)
		if i < 0 {
			t.Fatalf("no row for %s", sp.PublicID)
		}
		rest := page[i:]
		if j := strings.Index(rest, "</a>"); j > 0 {
			return rest[:j]
		}
		return rest
	}
	for _, page := range []string{h.uiText("/ui/spikes"), h.uiText("/ui/i/pf")} {
		wants(t, "the chat row", row(page, chat), `spike-executor">chat<`, "Time box: 2 hours")
		wants(t, "the by-hand row", row(page, hand), `spike-executor">by hand<`)
		wants(t, "the agent row", row(page, agent), `spike-executor">agent<`)
		lacks(t, "the idea row", row(page, idea), "spike-executor")
	}
}

func TestInboxSpikeClaimStaleSaysReleaseTheClaim(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.chatSpike()
	h.claimSpike(sp)
	h.staleClaim(h.latestSpikeClaim(sp))
	h.srv.ClaimSweep(ctx)

	_, page := h.getUI("/ui/frag/inbox")
	page = strings.NewReplacer("&#39;", "'").Replace(page)
	wants(t, "the inbox", page, "Release the claim",
		"The claim ends. The spike keeps running, its draft and working copy are kept, and its executor can claim it again before the time box ends.",
		"Keep the claim")
	lacks(t, "the inbox", page, "Release it to an agent", "an agent takes the task")

	// A checkpoint raised before the consequence was stored falls back to the
	// task's words.
	h.exec(`UPDATE checkpoints SET context = context - 'release_consequence' - 'release_label' WHERE kind = 'claim-stale'`)
	_, page = h.getUI("/ui/frag/inbox")
	wants(t, "the inbox", page, "Release it to an agent", "an agent takes the task")
}

func TestInboxTaskClaimStaleKeepsItsWords(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	t1, res := h.claimedSetup()
	_ = t1
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)

	_, page := h.getUI("/ui/frag/inbox")
	wants(t, "the inbox", page, "Release it to an agent",
		"The claim ends, the work in the working copy is kept, and an agent takes the task.")
	lacks(t, "the inbox", page, "Release the claim")
}

// FR-17.3: no new route starts, closes or reopens a spike, and none can be
// reached from another site.
func TestSpikeClaimRoutesNeverStartCloseOrReopen(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")

	idea := h.newSpike(in, "An idea?", nil)
	agent := h.newSpike(in, "A running agent spike?", nil)
	h.startSpikeQuiet(agent, 0)
	chat := h.chatSpike()
	ended := h.endedSpike(in, "An ended spike?", nil)
	closed := h.endedSpike(in, "A closed spike?", nil)
	if _, err := h.srv.CloseSpike(ctx, closed.ID, store.SpikeAnswered, nil, "sam"); err != nil {
		t.Fatal(err)
	}
	spikes := map[string]*store.Spike{"idea": idea, "running agent": agent, "running chat": chat, "ended": ended, "closed": closed}
	before := map[string]string{}
	for name, sp := range spikes {
		cur := h.getSpike(sp.ID)
		before[name] = cur.State + "/" + cur.Executor + "/" + cur.ClosedAs
	}

	for _, route := range []string{"claim", "draft", "submit", "release"} {
		for name, sp := range spikes {
			form := spikeForm(sp, "findings", goodFindings)
			code, body := h.postValues("/ui/spikes/"+route, form)
			if code == http.StatusSeeOther {
				// The claim that was made on a running chat spike by hand
				// isn't possible: its executor is the chat agent, and the
				// page says so.
				t.Errorf("%s on a %s spike worked: %s", route, name, body)
			}
			cur := h.getSpike(sp.ID)
			if got := cur.State + "/" + cur.Executor + "/" + cur.ClosedAs; got != before[name] {
				t.Errorf("%s on a %s spike changed it from %s to %s", route, name, before[name], got)
			}
		}
	}

	// Nothing from another site reaches any of the four.
	for _, route := range []string{"claim", "draft", "submit", "release"} {
		for _, hd := range []map[string]string{{"Sec-Fetch-Site": "cross-site"}, {"Origin": "http://evil.example"}} {
			resp := h.postNoFollow("/ui/spikes/"+route, map[string]string{"spike": chat.ID.String(), "findings": goodFindings}, hd)
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s with %v = %d, want 403", route, hd, resp.StatusCode)
			}
		}
	}
	// A bad ID is refused before anything is read.
	if resp := h.postNoFollow("/ui/spikes/claim", map[string]string{"spike": "SPK-1"}, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a bad spike id = %d", resp.StatusCode)
	}
}

func TestSpikeClaimedByAnotherCannotBeRunByThePerson(t *testing.T) {
	h := newHarness(t)
	sp := h.startedSpike(store.ExecutorPerson, "Is it someone else's?")
	// Held by the chat agent: the person sees the holder and may release.
	if _, err := h.srv.ClaimSpike(context.Background(), sp.PublicID, h.srv.ChatClaimant()); err == nil {
		// A person spike refuses the chat agent's claim; if it ever allows it,
		// the page must not offer the editor.
		page := h.uiText("/ui/s/" + sp.PublicID)
		lacks(t, "a held person spike", page, "Save findings")
	}
	_, body := h.postText("/ui/spikes/submit", spikeForm(sp, "findings", goodFindings))
	wants(t, "a submit with no claim", body, "You haven't claimed "+sp.PublicID+", so there is nothing to submit.")
}

// A chat or person spike has a time box and no run, so its page doesn't say
// that a run has ended or stops.
func TestChatSpikePageSaysNothingOfARun(t *testing.T) {
	h := newHarness(t)
	sp := h.chatSpike()
	h.claimSpike(sp)
	if _, err := h.srv.SaveSpikeFindings(context.Background(), sp.PublicID, h.srv.ChatClaimant(), "## Answer\n\nA draft note.\n"); err != nil {
		t.Fatal(err)
	}
	h.postText("/ui/spikes/release", spikeForm(sp))
	page := h.uiText("/ui/s/" + sp.PublicID)
	wants(t, "the running chat spike's draft", page, "If the time box ends now, this is what is kept.")
	lacks(t, "the running chat spike's draft", page, "If the run stops now")

	if err := h.srv.EndSpike(context.Background(), sp.ID, store.SpikeTimeBox, ""); err != nil {
		t.Fatal(err)
	}
	page = h.uiText("/ui/s/" + sp.PublicID)
	wants(t, "the time-boxed chat spike", page, "This spike has ended: it reached its time box. It's waiting for you to read the findings.",
		"Ended: it reached its time box.")
	lacks(t, "the time-boxed chat spike", page, "run has ended", "Reached its")
}
