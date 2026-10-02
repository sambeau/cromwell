package server

// Claiming a spike (SPEC-021 FR-13, FR-14): the claim's contents and
// refusals, renewal, release, saving and submitting findings, and the claim
// sweep's two questions for a spike. Tests that need T3's ending at the time
// box (EndSpike(time_box)) say so and skip until it exists.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/rules"
	"subutai/internal/store"
)

// startedSpike writes down a question and starts it for an executor with a
// four-hour time box, without kicking the dispatcher.
func (h *harness) startedSpike(executor, question string) *store.Spike {
	h.t.Helper()
	in := h.spikeInitiative("pf" + strings.ToLower(uuid.NewString()[:4]))
	sp := h.newSpike(in, question, nil)
	started, err := h.srv.startSpike(context.Background(), sp.ID, SpikeStartRequest{Executor: executor, TimeBoxHours: 4}, "sam", false)
	if err != nil {
		h.t.Fatal(err)
	}
	return started
}

func (h *harness) chatSpike() *store.Spike {
	return h.startedSpike(store.ExecutorChat, "Does the queue keep its order?")
}

// claimSpike claims as the chat agent, and fails the test if refused.
func (h *harness) claimSpike(sp *store.Spike) *SpikeClaimResult {
	h.t.Helper()
	res, err := h.srv.ClaimSpike(context.Background(), sp.PublicID, h.srv.ChatClaimant())
	if err != nil {
		h.t.Fatalf("claim %s: %v", sp.PublicID, err)
	}
	return res
}

func (h *harness) spikeExecs(sp *store.Spike) []store.Execution {
	h.t.Helper()
	es, err := store.ExecutionsFor(context.Background(), h.srv.Store.Pool, "spike", sp.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return es
}

func (h *harness) latestSpikeClaim(sp *store.Spike) *store.Claim {
	h.t.Helper()
	c, err := store.LatestClaimFor(context.Background(), h.srv.Store.Pool, "spike", sp.ID)
	if err != nil {
		h.t.Fatalf("latest claim of %s: %v", sp.PublicID, err)
	}
	return c
}

// pastDeadline backdates the spike's deadline, and its claim's, by an hour.
func (h *harness) pastDeadline(sp *store.Spike) {
	h.t.Helper()
	h.exec(`UPDATE spikes SET deadline_at = now() - interval '1 hour' WHERE id = $1`, sp.ID)
	h.exec(`UPDATE work_claims SET deadline_at = now() - interval '1 hour' WHERE ref_type = 'spike' AND ref_id = $1`, sp.ID)
}

// wantRefusal checks an error is a ClaimRefusal with exactly the sentence.
func wantExactRefusal(t *testing.T, err error, want string) {
	t.Helper()
	r, ok := AsClaimRefusal(err)
	if !ok {
		t.Fatalf("want the refusal %q, got %v", want, err)
	}
	if r.Sentence != want {
		t.Fatalf("refusal = %q\nwant      %q", r.Sentence, want)
	}
}

// FR-13.4 and the acceptance list: a claim returns the working copy, the
// contract, the deadline and the rules; the claim holds no feature and
// carries the spike's deadline; its execution is chat, spike, round 1,
// unmeasured.
func TestClaimSpikeReturnsTheWorkingCopyAndTheContract(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.chatSpike()
	res := h.claimSpike(sp)

	if res.Renewed || res.Spike.ID != sp.ID || res.Claim == nil {
		t.Fatalf("result = %+v", res)
	}
	if res.WorkingCopy.Path != h.srv.worktreeAbs(sp.WorktreePath) || res.WorkingCopy.BaseCommit != sp.BaseCommit || res.WorkingCopy.Branch != "" {
		t.Errorf("working copy = %+v", res.WorkingCopy)
	}
	if _, err := os.Stat(filepath.Join(res.WorkingCopy.Path, ".git")); err != nil {
		t.Errorf("the working copy isn't there: %v", err)
	}
	for _, k := range []string{"question", "where_it_came_from", "findings_template"} {
		if _, ok := res.Contract[k]; !ok {
			t.Errorf("the contract has no %s: %v", k, res.Contract)
		}
	}
	if res.Contract["question"] != sp.Question {
		t.Errorf("question = %v", res.Contract["question"])
	}
	if _, ok := res.Contract["draft"]; ok {
		t.Error("there is no draft to give yet")
	}
	secs, _ := res.Contract["findings_template"].([]map[string]any)
	var heads []string
	for _, s := range secs {
		heads = append(heads, s["heading"].(string))
		if s["heading"] == "Answer" && (s["required"] != true || s["for"] == "") {
			t.Errorf("Answer = %v", s)
		}
	}
	if got := strings.Join(heads, "|"); got != "Question|Answer|What we found|How we found out|What to do next|How this spike ended" {
		t.Errorf("template sections = %s", got)
	}
	if !strings.HasPrefix(res.TimeLeft, "The time box ends at ") || !strings.Contains(res.TimeLeft, " UTC, in 3 hours 59 minutes. Then the spike ends with whatever findings you have saved.") {
		t.Errorf("time left = %q", res.TimeLeft)
	}
	if len(res.Rules) < len(spikeClaimRuleSentences) {
		t.Fatalf("rules = %v", res.Rules)
	}
	for i, r := range spikeClaimRuleSentences {
		if res.Rules[i] != r {
			t.Errorf("rule %d = %q", i, res.Rules[i])
		}
	}
	for _, r := range res.Rules[len(spikeClaimRuleSentences):] {
		if !strings.HasPrefix(r, "The project allows the command ") {
			t.Errorf("a trailing rule that isn't a command: %q", r)
		}
	}

	c := h.latestSpikeClaim(sp)
	if c.FeatureID != nil || c.Kind != store.WriterChat || c.Via != "mcp" || c.State != lifecycle.ClaimOpen {
		t.Errorf("claim = %+v", c)
	}
	cur := h.getSpike(sp.ID)
	if c.DeadlineAt == nil || cur.DeadlineAt == nil || !c.DeadlineAt.Equal(*cur.DeadlineAt) {
		t.Errorf("claim deadline %v, spike deadline %v", c.DeadlineAt, cur.DeadlineAt)
	}
	execs := h.spikeExecs(sp)
	if len(execs) != 1 || execs[0].Kind != store.WriterChat || execs[0].RefType != "spike" || execs[0].Round != 1 || execs[0].Measured {
		t.Errorf("executions = %+v", execs)
	}
	if cur.TokensUsed != 0 {
		t.Errorf("tokens used = %d", cur.TokensUsed)
	}
	if h.auditRefCount("claim.claimed", sp.ID) != 1 {
		t.Error("the claim is audited on the spike")
	}
	_ = ctx
}

// FR-13.1's refusals, each its own sentence, and FR-13.5's pointer from
// claim_task.
func TestClaimSpikeRefusals(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	chat, person := h.srv.ChatClaimant(), h.srv.PersonClaimant()

	// An idea.
	in := h.spikeInitiative("pf")
	idea := h.newSpike(in, "An idea?", nil)
	_, err := h.srv.ClaimSpike(ctx, idea.PublicID, chat)
	wantExactRefusal(t, err, idea.PublicID+" isn't running, so it can't be claimed. A person starts a spike from its page in the web UI.")

	// An agent spike.
	agent := h.newSpike(in, "An agent's?", nil)
	agentStarted, _ := h.startSpikeQuiet(agent, 0)
	_, err = h.srv.ClaimSpike(ctx, agentStarted.PublicID, chat)
	wantExactRefusal(t, err, agent.PublicID+" is run by the spike runner, an agent, so it can't be claimed.")

	// The wrong surface for a chat spike, and for a person spike.
	cs := h.startedSpike(store.ExecutorChat, "Chat's?")
	_, err = h.srv.ClaimSpike(ctx, cs.PublicID, person)
	wantExactRefusal(t, err, cs.PublicID+" is to be run in chat, not in the web UI. Ask the chat agent to run it.")
	ps := h.startedSpike(store.ExecutorPerson, "A person's?")
	_, err = h.srv.ClaimSpike(ctx, ps.PublicID, chat)
	wantExactRefusal(t, err, ps.PublicID+" is to be run by a person in the web UI, not in chat.")

	// Not a spike's ID at all.
	_, err = h.srv.ClaimSpike(ctx, "FEAT-001", chat)
	if _, ok := AsClaimRefusal(err); !ok {
		t.Errorf("FEAT-001: %v", err)
	}
	_, err = h.srv.ClaimSpike(ctx, "SPK-999", chat)
	wantExactRefusal(t, err, "There is no spike SPK-999. list_spikes shows what exists.")

	// Held by someone else.
	h.claimSpike(cs)
	other := Claimant{Kind: store.WriterChat, Actor: "another-chat", Via: "mcp"}
	_, err = h.srv.ClaimSpike(ctx, cs.PublicID, other)
	wantExactRefusal(t, err, cs.PublicID+" is claimed by the chat agent. Only a person can release a claim, in the web UI.")

	// Past the deadline: a new claim is refused, with the time box's sentence.
	late := h.startedSpike(store.ExecutorChat, "Too late?")
	h.exec(`UPDATE spikes SET deadline_at = now() - interval '1 hour' WHERE id = $1`, late.ID)
	_, err = h.srv.ClaimSpike(ctx, late.PublicID, chat)
	wantExactRefusal(t, err, late.PublicID+"'s time box ended at "+time.Now().UTC().Add(-time.Hour).Format("15:04")+", so it is ending with the findings that were saved.")
	if _, err := store.LatestClaimFor(ctx, h.srv.Store.Pool, "spike", late.ID); err == nil {
		t.Error("a refused claim left a claim behind")
	}

	// A working copy that can't be made.
	broken := h.startedSpike(store.ExecutorChat, "Broken?")
	h.exec(`UPDATE spikes SET base_commit = 'deadbeefdeadbeefdeadbeefdeadbeefdeadbeef' WHERE id = $1`, broken.ID)
	if err := os.RemoveAll(h.srv.worktreeAbs(broken.WorktreePath)); err != nil {
		t.Fatal(err)
	}
	_, err = h.srv.ClaimSpike(ctx, broken.PublicID, chat)
	r, ok := AsClaimRefusal(err)
	if !ok || !strings.HasPrefix(r.Sentence, broken.PublicID+"'s working copy couldn't be made, so it can't be claimed yet: ") {
		t.Errorf("broken: %v", err)
	}

	// claim_task points to claim_spike; anything else keeps FR-6.3's sentence.
	_, err = h.srv.ClaimWork(ctx, cs.PublicID, chat)
	wantExactRefusal(t, err, cs.PublicID+" is a spike, not a task. Use claim_spike to run it.")
	_, err = h.srv.ClaimWork(ctx, "FEAT-001", chat)
	wantExactRefusal(t, err, onlyTasksSentence)
}

// A person claims a person spike through the web UI's claimant.
func TestAPersonClaimsAPersonSpike(t *testing.T) {
	h := newHarness(t)
	ps := h.startedSpike(store.ExecutorPerson, "By hand?")
	res, err := h.srv.ClaimSpike(context.Background(), ps.PublicID, h.srv.PersonClaimant())
	if err != nil {
		t.Fatal(err)
	}
	if res.Claim.Kind != store.WriterPerson || res.Claim.Via != "ui" || res.Claim.FeatureID != nil {
		t.Errorf("claim = %+v", res.Claim)
	}
	if execs := h.spikeExecs(ps); len(execs) != 1 || execs[0].Kind != store.WriterPerson || execs[0].Measured {
		t.Errorf("executions = %+v", execs)
	}
}

// FR-13.2: claiming again renews; after the deadline a renewal is refused.
func TestRenewingASpikeClaim(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.chatSpike()
	first := h.claimSpike(sp)
	again := h.claimSpike(sp)
	if !again.Renewed || again.Claim.ID != first.Claim.ID {
		t.Fatalf("renewal = %+v", again)
	}
	if h.latestSpikeClaim(sp).LastActivity != "renewed" || h.auditRefCount("claim.renewed", sp.ID) != 1 {
		t.Error("the renewal is activity and is audited")
	}
	if n := len(h.spikeExecs(sp)); n != 1 {
		t.Errorf("a renewal wrote an execution: %d rows", n)
	}

	h.pastDeadline(sp)
	_, err := h.srv.ClaimSpike(ctx, sp.PublicID, h.srv.ChatClaimant())
	wantExactRefusal(t, err, sp.PublicID+"'s time box ended at "+time.Now().UTC().Add(-time.Hour).Format("15:04")+", so it is ending with the findings that were saved.")
	if h.auditRefCount("claim.renewed", sp.ID) != 1 {
		t.Error("the refused renewal was recorded")
	}
}

// FR-14.1 and the first acceptance item: a release frees the claim and
// nothing else, and a claim after it is a new claim and a new execution.
func TestReleasingASpikeClaimKeepsTheSpikeRunning(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.chatSpike()
	wantExactRefusal(t, h.srv.ReleaseSpikeClaim(ctx, sp.PublicID, "sam"), sp.PublicID+" has no claim to release.")

	first := h.claimSpike(sp)
	writeIn(t, first.WorkingCopy.Path, "scratch.txt", "a note\n")
	if _, err := h.srv.SaveSpikeFindings(ctx, sp.PublicID, h.srv.ChatClaimant(), "## Answer\n\nMaybe.\n"); err != nil {
		t.Fatal(err)
	}
	head := h.headSHA()
	if err := h.srv.ReleaseSpikeClaim(ctx, sp.PublicID, "sam"); err != nil {
		t.Fatal(err)
	}
	c := h.latestSpikeClaim(sp)
	if c.State != lifecycle.ClaimEnded || c.EndReason != "released" || c.EndedBy != "sam" {
		t.Fatalf("claim = %+v", c)
	}
	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeRunning || cur.Draft == "" || cur.WorktreeRemovedAt != nil {
		t.Errorf("spike = %+v", cur)
	}
	if _, err := os.Stat(filepath.Join(first.WorkingCopy.Path, "scratch.txt")); err != nil {
		t.Errorf("the working copy's contents weren't kept: %v", err)
	}
	if h.headSHA() != head {
		t.Error("a release committed something")
	}
	if h.auditRefCount("claim.released", sp.ID) != 1 {
		t.Error("the release is audited")
	}

	second := h.claimSpike(sp)
	if second.Renewed || second.Claim.ID == first.Claim.ID {
		t.Fatalf("a claim after a release is a new claim: %+v", second.Claim)
	}
	if execs := h.spikeExecs(sp); len(execs) != 2 || execs[0].ClaimID == nil || execs[1].ClaimID == nil || *execs[0].ClaimID == *execs[1].ClaimID {
		t.Errorf("executions = %+v", execs)
	}
	if d, _ := second.Contract["draft"].(string); !strings.Contains(d, "Maybe.") {
		t.Errorf("the draft isn't in the new claim's contract: %v", second.Contract["draft"])
	}
}

// FR-13.3: a missing worktree is made by the claim.
func TestClaimingASpikeMakesAMissingWorktree(t *testing.T) {
	h := newHarness(t)
	sp := h.chatSpike()
	abs := h.srv.worktreeAbs(sp.WorktreePath)
	if err := os.RemoveAll(abs); err != nil {
		t.Fatal(err)
	}
	res := h.claimSpike(sp)
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		t.Fatalf("the claim didn't make the working copy: %v", err)
	}
	if head := strings.TrimSpace(gitOut(t, abs, "rev-parse", "HEAD")); head != sp.BaseCommit {
		t.Errorf("HEAD = %s, want the base commit %s", head, sp.BaseCommit)
	}
	if execs := h.spikeExecs(sp); len(execs) != 1 || execs[0].StartHead != sp.BaseCommit || res.WorkingCopy.Path != abs {
		t.Errorf("executions = %+v", execs)
	}
}

// FR-13.6: saving replaces the draft, is activity, and withdraws a pending
// claim-stale; it is refused for someone else, with no claim, and after the
// deadline.
func TestSavingSpikeFindings(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	sp := h.chatSpike()

	_, err := h.srv.SaveSpikeFindings(ctx, sp.PublicID, chat, "## Answer\n\nNot yet.\n")
	wantExactRefusal(t, err, "You haven't claimed "+sp.PublicID+", so there are no findings to save. Claim it first.")

	res := h.claimSpike(sp)
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "spike", sp.ID)
	if cp == nil {
		t.Fatal("a claim idle for 26 hours is asked about, on the spike")
	}

	if _, err := h.srv.SaveSpikeFindings(ctx, sp.PublicID, chat, "  \n"); err == nil {
		t.Error("blank findings were saved")
	}
	got, err := h.srv.SaveSpikeFindings(ctx, sp.PublicID, chat, "## Answer\n\nFirst.\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.Draft != "## Answer\n\nFirst.\n" || got.DraftSavedAt == nil {
		t.Fatalf("draft = %q", got.Draft)
	}
	got, err = h.srv.SaveSpikeFindings(ctx, sp.PublicID, chat, "## Answer\n\nSecond.\n")
	if err != nil || got.Draft != "## Answer\n\nSecond.\n" {
		t.Fatalf("a second save replaces the first: %q %v", got.Draft, err)
	}
	c := h.latestSpikeClaim(sp)
	if c.LastActivity != "findings" || time.Since(c.LastActivityAt) > time.Minute || claimActivityWords(c.LastActivity) != "findings saved" {
		t.Errorf("claim = %+v", c)
	}
	if st := h.checkpointState(cp.ID); st != "withdrawn" {
		t.Errorf("the question is %s, not withdrawn", st)
	}
	if h.auditRefCount("claim.activity", sp.ID) != 2 {
		t.Errorf("claim.activity rows = %d", h.auditRefCount("claim.activity", sp.ID))
	}

	// Someone else's claim, from the wrong surface, and after the deadline.
	other := Claimant{Kind: store.WriterChat, Actor: "another-chat", Via: "mcp"}
	_, err = h.srv.SaveSpikeFindings(ctx, sp.PublicID, other, "## Answer\n\nMine.\n")
	wantExactRefusal(t, err, sp.PublicID+" is claimed by the chat agent, not by you, so you can't save its findings.")
	if _, err := h.srv.SaveSpikeFindings(ctx, sp.PublicID, h.srv.PersonClaimant(), "## Answer\n\nMine.\n"); err == nil {
		t.Error("a person saved a chat agent's claim's findings")
	}
	h.pastDeadline(sp)
	_, err = h.srv.SaveSpikeFindings(ctx, sp.PublicID, chat, "## Answer\n\nToo late.\n")
	wantExactRefusal(t, err, sp.PublicID+"'s time box ended at "+time.Now().UTC().Add(-time.Hour).Format("15:04")+", so it is ending with the findings that were saved.")
	if d := h.getSpike(sp.ID).Draft; d != "## Answer\n\nSecond.\n" {
		t.Errorf("a refused save changed the draft: %q", d)
	}
}

// FR-13.7: a submit with incomplete findings is refused and changes nothing.
func TestSubmittingIncompleteSpikeFindingsChangesNothing(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	sp := h.chatSpike()

	_, err := h.srv.SubmitSpike(ctx, sp.PublicID, chat, goodFindings)
	wantExactRefusal(t, err, "You haven't claimed "+sp.PublicID+", so there is nothing to submit. Claim it first.")

	h.claimSpike(sp)
	if _, err := h.srv.SaveSpikeFindings(ctx, sp.PublicID, chat, "## Answer\n\nSaved.\n"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"## Answer\n\nYes.\n", "## What we found\n\nA lot.\n", "## Answer\n\nTODO\n\n## What we found\n\nA lot.\n"} {
		_, err := h.srv.SubmitSpike(ctx, sp.PublicID, chat, bad)
		r, ok := AsClaimRefusal(err)
		if !ok || !strings.HasPrefix(r.Sentence, "The findings can't be submitted yet: ") || !strings.HasSuffix(r.Sentence, "Nothing was changed.") {
			t.Errorf("submit %q: %v", bad, err)
		}
	}
	// The saved draft is itself incomplete, and so is refused when none is given.
	if _, err := h.srv.SubmitSpike(ctx, sp.PublicID, chat, ""); err == nil {
		t.Error("an incomplete draft was submitted")
	}
	cur := h.getSpike(sp.ID)
	c := h.latestSpikeClaim(sp)
	if cur.State != store.SpikeRunning || cur.Draft != "## Answer\n\nSaved.\n" || c.State != lifecycle.ClaimOpen || len(h.spikeExecs(sp)) != 1 || h.spikeExecs(sp)[0].SubmittedAt != nil {
		t.Errorf("a refused submit changed something: %+v %+v", cur, c)
	}
	// Someone else's, and none at all.
	other := Claimant{Kind: store.WriterChat, Actor: "another-chat", Via: "mcp"}
	_, err = h.srv.SubmitSpike(ctx, sp.PublicID, other, goodFindings)
	wantExactRefusal(t, err, sp.PublicID+" is claimed by the chat agent, not by you, so you can't submit it.")
	// Past the deadline a submit isn't taken (step 1).
	h.pastDeadline(sp)
	_, err = h.srv.SubmitSpike(ctx, sp.PublicID, chat, goodFindings)
	wantExactRefusal(t, err, sp.PublicID+"'s time box ended at "+time.Now().UTC().Add(-time.Hour).Format("15:04")+", so it is ending with the findings that were saved.")
	if h.getSpike(sp.ID).State != store.SpikeRunning {
		t.Error("a refused submit ended the spike")
	}
}

// FR-13.7 and SD-23: good findings end the spike concluded, with the findings
// committed, the worktree gone, the claim done and the execution submitted.
func TestSubmittingSpikeFindingsConcludesTheSpike(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	sp := h.chatSpike()
	res := h.claimSpike(sp)
	writeIn(t, res.WorkingCopy.Path, "scratch.txt", "throwaway\n")

	ended, err := h.srv.SubmitSpike(ctx, sp.PublicID, chat, goodFindings)
	if err != nil {
		t.Fatal(err)
	}
	if ended.State != store.SpikeEnded || ended.EndedHow != store.SpikeConcluded || ended.WorktreeRemovedAt == nil {
		t.Fatalf("spike = %+v", ended)
	}
	if _, err := os.Stat(res.WorkingCopy.Path); !os.IsNotExist(err) {
		t.Errorf("the worktree is still there: %v", err)
	}
	doc, text := h.findingsOf(sp)
	if !strings.Contains(text, "The header is read in one place") {
		t.Errorf("findings = %s", text)
	}
	if out := h.gitOut("ls-tree", "--name-only", "HEAD", "--", doc.Path); strings.TrimSpace(out) != doc.Path {
		t.Errorf("the findings aren't committed: %q", out)
	}
	c := h.latestSpikeClaim(sp)
	if c.State != lifecycle.ClaimEnded || c.EndReason != "done" || c.SubmittedAt == nil {
		t.Errorf("claim = %+v", c)
	}
	if execs := h.spikeExecs(sp); len(execs) != 1 || execs[0].SubmittedAt == nil {
		t.Errorf("executions = %+v", execs)
	}
	if h.auditRefCount("claim.submitted", sp.ID) != 1 {
		t.Error("the submit is audited")
	}
	var payload string
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT payload::text FROM audit_events WHERE kind = 'claim.submitted' AND ref_id = $1`, sp.ID).Scan(&payload); err != nil ||
		strings.Contains(payload, "commit") || strings.Contains(payload, "review") {
		t.Errorf("claim.submitted payload = %s (%v)", payload, err)
	}
	// An ended spike can't be claimed, renewed or saved to.
	_, err = h.srv.ClaimSpike(ctx, sp.PublicID, chat)
	wantExactRefusal(t, err, sp.PublicID+" has ended. A person reads its findings on its page.")
	_, err = h.srv.SaveSpikeFindings(ctx, sp.PublicID, chat, "## Answer\n\nAgain.\n")
	wantExactRefusal(t, err, sp.PublicID+" has ended. A person reads its findings on its page.")
}

// FR-13.7 step 2: with no findings given, the saved draft is submitted.
func TestSubmittingWithNoFindingsUsesTheDraft(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	sp := h.chatSpike()
	h.claimSpike(sp)

	_, err := h.srv.SubmitSpike(ctx, sp.PublicID, chat, "")
	wantExactRefusal(t, err, "There are no findings to submit, and none are saved. Write them as sections, starting with Answer and What we found.")

	if _, err := h.srv.SaveSpikeFindings(ctx, sp.PublicID, chat, goodFindings); err != nil {
		t.Fatal(err)
	}
	ended, err := h.srv.SubmitSpike(ctx, sp.PublicID, chat, "  ")
	if err != nil || ended.EndedHow != store.SpikeConcluded {
		t.Fatalf("submit: %+v %v", ended, err)
	}
	if _, text := h.findingsOf(sp); !strings.Contains(text, "By reading the handler and calling it.") {
		t.Errorf("findings = %s", text)
	}
}

// FR-14.2: the claim sweep asks about a spike's claim, with FR-13.1's words,
// and the checkpoint carries the release answer's label and consequence for
// the Inbox. Keep, a save and Release each do what they say.
func TestTheSweepAsksAboutASpikeClaim(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.chatSpike()
	res := h.claimSpike(sp)

	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-stale", "spike", sp.ID) != nil {
		t.Fatal("a fresh claim isn't stale")
	}
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "spike", sp.ID)
	if cp == nil {
		t.Fatal("no question")
	}
	deadline := h.getSpike(sp.ID).DeadlineAt.UTC().Format("15:04 on 2 January")
	want := sp.PublicID + ", Does the queue keep its order?, was claimed by the chat agent 1 day ago, and nothing has changed in its working copy or its findings for 26 hours. Its time box ends at " + deadline + ". Is someone still working on it?"
	if cp.Question != want {
		t.Fatalf("question = %q\nwant       %q", cp.Question, want)
	}
	for _, w := range []string{`"release_label": "Release the claim"`, `"release_consequence": "` + spikeReleaseConsequence + `"`, res.Claim.ID.String()} {
		if !strings.Contains(string(cp.Context), w) {
			t.Errorf("context lacks %s: %s", w, cp.Context)
		}
	}
	// No claim-deadline is ever raised for a spike, even a moment before it ends.
	if h.pendingOf("claim-deadline", "spike", sp.ID) != nil {
		t.Error("a spike's claim raised claim-deadline")
	}

	// Keep the claim restarts the clock.
	if err := h.srv.KeepClaimAnswered(ctx, rules.KeepClaim{RefType: "spike", RefID: sp.ID, ClaimID: res.Claim.ID.String(), Actor: "sam"}); err != nil {
		t.Fatal(err)
	}
	if c := h.latestSpikeClaim(sp); c.LastActivity != "kept" || c.State != lifecycle.ClaimOpen {
		t.Fatalf("claim = %+v", c)
	}
	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-stale", "spike", sp.ID) != nil {
		t.Fatal("the clock restarted")
	}

	// Stale again; Release the claim releases it, and the spike keeps running.
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp = h.pendingOf("claim-stale", "spike", sp.ID)
	if cp == nil {
		t.Fatal("no second question")
	}
	if err := h.srv.ReleaseClaimAnswered(ctx, rules.ReleaseClaim{RefType: "spike", RefID: sp.ID, ClaimID: res.Claim.ID.String(), Actor: "sam"}); err != nil {
		t.Fatal(err)
	}
	c := h.latestSpikeClaim(sp)
	if c.State != lifecycle.ClaimEnded || c.EndReason != "released" || c.EndedBy != "sam" {
		t.Fatalf("claim = %+v", c)
	}
	if h.getSpike(sp.ID).State != store.SpikeRunning {
		t.Error("the release ended the spike")
	}
	// An answer to a claim that has moved on is a no-op.
	if err := h.srv.ReleaseClaimAnswered(ctx, rules.ReleaseClaim{RefType: "spike", RefID: sp.ID, ClaimID: res.Claim.ID.String(), Actor: "sam"}); err != nil {
		t.Fatal(err)
	}
	// The chat agent can claim it again.
	if again := h.claimSpike(sp); again.Claim.ID == res.Claim.ID {
		t.Error("a claim after a release is new")
	}
}

// A task's checkpoint carries the task's own consequence (FR-14.2).
func TestATaskClaimQuestionCarriesTheTaskConsequence(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(context.Background())
	cp := h.pendingOf("claim-stale", "task", t1.ID)
	if cp == nil {
		t.Fatal("no question")
	}
	for _, w := range []string{`"release_label": "Release it to an agent"`, `"release_consequence": "The claim ends, the work in the working copy is kept, and an agent takes the task."`} {
		if !strings.Contains(string(cp.Context), w) {
			t.Errorf("context lacks %s: %s", w, cp.Context)
		}
	}
}

// FR-14.4: with the expiry and the deadline both passed, one sweep ends the
// spike and raises no claim-stale and no claim-deadline.
//
// DEPENDS ON T3: the ending is EndSpike(time_box), which T3 adds. Until it
// is merged the sweep's call is refused, and the last assertions are skipped.
func TestTheSweepEndsASpikeAtItsDeadline(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.chatSpike()
	res := h.claimSpike(sp)
	if _, err := h.srv.SaveSpikeFindings(ctx, sp.PublicID, h.srv.ChatClaimant(), goodFindings); err != nil {
		t.Fatal(err)
	}
	h.staleClaim(res.Claim)
	h.pastDeadline(sp)
	h.srv.ClaimSweep(ctx)

	if h.pendingOf("claim-stale", "spike", sp.ID) != nil || h.pendingOf("claim-deadline", "spike", sp.ID) != nil {
		t.Fatal("a spike past its deadline is ended, not asked about")
	}
	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeTimeBox {
		t.Fatalf("spike = %+v", cur)
	}
	if c := h.latestSpikeClaim(sp); c.State != lifecycle.ClaimEnded || c.EndReason != "expired" {
		t.Errorf("claim = %+v", c)
	}
}
