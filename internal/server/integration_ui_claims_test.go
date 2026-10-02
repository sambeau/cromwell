package server

// The web UI for claims (SPEC-020 FR-1.5 to FR-1.7, FR-4, FR-10.2), against
// real Postgres with the phase-1 harness and the dispatcher held, as
// integration_claims_test.go does.

import (
	"context"
	"strings"
	"testing"

	"subutai/internal/lifecycle"
)

func (h *harness) taskPageHTML(taskID string) string {
	h.t.Helper()
	code, body := h.getUI("/ui/t/" + taskID)
	if code != 200 {
		h.t.Fatalf("GET task page: %d", code)
	}
	return body
}

func (h *harness) featurePageHTML() string {
	h.t.Helper()
	code, body := h.getUI("/ui/f/auth/login")
	if code != 200 {
		h.t.Fatalf("GET feature page: %d", code)
	}
	return body
}

// TestPersonClaimsATaskInTheUI is FR-10.2: a person claims a task in the web
// UI, submits it with a summary (the review is queued with the usual model),
// the chat agent's claim is released by the person, and the refusals appear as
// banners. The task page and the feature page show who executes, and the
// timeline tells the story.
func TestPersonClaimsATaskInTheUI(t *testing.T) {
	h := newHarness(t)
	h.setRole("code-reviewer", sonnet, haiku)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")

	// Before any claim: the panel invites one, and nobody has started.
	page := h.taskPageHTML(t1.ID.String())
	mustContain(t, "claimable", page, "You can implement this task yourself instead of an agent. Claiming it pauses this feature&#39;s agents until you submit it.")
	mustContain(t, "claim button", page, "Claim this task")
	mustContain(t, "executor line", page, "Nobody has started this task yet.")

	// The person claims T1.
	code, body := h.postForm("/ui/task/claim", map[string]string{"task": t1.PublicID})
	if code != 200 {
		t.Fatalf("claim: %d %s", code, truncate(body, 400))
	}
	mustContain(t, "claim notice", body, "You claimed "+t1.PublicID+".")
	mustContain(t, "person's panel", body, "Submit for code review")
	mustContain(t, "executor line", body, "Being implemented by "+h.srv.uiActor()+", who claimed it")
	mustContain(t, "unmeasured", body, "This work was done in chat or by a person, so its tokens aren&#39;t measured.")
	res, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.PersonClaimant())
	if err != nil || !res.Renewed {
		t.Fatalf("the claim is the person's and open: %+v, %v", res, err)
	}
	mustContain(t, "working copy path", body, res.WorkingCopy.Path)
	mustContain(t, "branch", body, res.WorkingCopy.Branch)

	// The feature page: the executor icon with the line as its name, the
	// label, and the waiting sentence under the timeline.
	feat := h.featurePageHTML()
	mustContain(t, "task list", feat, "Tasks in this feature")
	mustContain(t, "icon name", feat, `aria-label="Being implemented by `+h.srv.uiActor())
	mustContain(t, "icon", feat, "#i-owner")
	mustContain(t, "label", feat, "Claimed by "+h.srv.uiActor())
	mustContain(t, "waiting", feat, "T1 First is claimed by "+h.srv.uiActor()+", so this feature&#39;s agents are waiting.")

	// Refusals are banners, not errors.
	code, body = h.postForm("/ui/task/claim", map[string]string{"task": t2.PublicID})
	if code != 200 {
		t.Fatalf("refused claim: %d", code)
	}
	mustContain(t, "banner", body, `class="banner banner--error"`)
	mustContain(t, "one at a time", body, t1.PublicID+" is claimed by "+h.srv.uiActor()+", in this feature&#39;s working copy. Only one task in a feature can be worked at a time")
	code, body = h.postForm("/ui/task/submit", map[string]string{"task": t1.PublicID, "summary": "   "})
	if code != 200 {
		t.Fatalf("empty summary: %d", code)
	}
	mustContain(t, "summary banner", body, `class="banner banner--error"`)
	mustContain(t, "summary needed", body, "Say what you did, in a sentence or two")
	_, body = h.postForm("/ui/task/submit", map[string]string{"task": t1.PublicID, "summary": "I did it."})
	mustContain(t, "nothing changed", body, "Nothing in the working copy has changed since you claimed "+t1.PublicID)
	if code, _ = h.postForm("/ui/task/claim", map[string]string{"task": "nonsense"}); code != 400 {
		t.Errorf("a bad task id is a 400, got %d", code)
	}

	// Submit with a change and a summary.
	writeIn(t, res.WorkingCopy.Path, "a.go", "package main\n")
	code, body = h.postForm("/ui/task/submit", map[string]string{"task": t1.PublicID, "summary": "Added a.go."})
	if code != 200 {
		t.Fatalf("submit: %d", code)
	}
	mustContain(t, "submit notice", body, "You submitted "+t1.PublicID+" for code review.")
	mustContain(t, "submitted panel", body, "Submitted by "+h.srv.uiActor()+". A code reviewer is reviewing it.")
	reviews := h.dispatchesOf(t1.ID, "review-code")
	if len(reviews) != 1 || reviews[0].State != "queued" || reviews[0].Model != haiku {
		t.Fatalf("a person's work is reviewed with the usual model: %+v", reviews)
	}
	if strings.Contains(body, "the project&#39;s reviewer for the chat agent&#39;s work") {
		t.Error("a person's work isn't reviewed by the chat reviewer")
	}

	// The chat agent claims T2, and the person releases it.
	chat := h.srv.ChatClaimant()
	if _, err := h.srv.ClaimWork(ctx, t2.PublicID, chat); err != nil {
		t.Fatal(err)
	}
	page = h.taskPageHTML(t2.ID.String())
	mustContain(t, "chat panel", page, "The chat agent is working on this. Releasing it gives the task to an agent, and keeps what is in the working copy.")
	mustContain(t, "release", page, `action="/ui/task/release"`)
	feat = h.featurePageHTML()
	mustContain(t, "chat icon", feat, "#i-chat")
	mustContain(t, "chat label", feat, "Claimed by the chat agent")
	mustContain(t, "waiting on chat", feat, "T2 Second is claimed by the chat agent, so this feature&#39;s agents are waiting.")

	code, body = h.postForm("/ui/task/release", map[string]string{"task": t2.PublicID})
	if code != 200 {
		t.Fatalf("release: %d", code)
	}
	mustContain(t, "release notice", body, "You released "+t2.PublicID+".")
	mustContain(t, "claimable again", body, "Claim this task")
	if c := h.latestClaim(t2.ID); c.State != lifecycle.ClaimEnded || c.EndReason != "released" || c.EndedBy != h.srv.uiActor() {
		t.Fatalf("the person released it: %+v", c)
	}
	// Releasing again is a refusal, as a banner.
	_, body = h.postForm("/ui/task/release", map[string]string{"task": t2.PublicID})
	mustContain(t, "no claim to release", body, t2.PublicID+" has no claim to release.")
	mustContain(t, "banner", body, `class="banner banner--error"`)

	// The timeline tells it (FR-1.7), and the owner of the work is named on
	// the task list (FR-1.6).
	feat = h.featurePageHTML()
	for _, want := range []string{
		"Claimed by " + h.srv.uiActor() + ": First",
		"Submitted by " + h.srv.uiActor() + ": First",
		"Claimed by the chat agent: Second",
		"Released by " + h.srv.uiActor() + ": Second",
	} {
		mustContain(t, "timeline", feat, want)
	}
	if strings.Contains(feat, "is claimed by the chat agent, so this feature&#39;s agents are waiting") {
		t.Error("the released claim no longer holds the feature")
	}
}

// TestChatAgentsWorkNamesItsReviewer is FR-8.4: the task page says the review
// of the chat agent's work ran on the project's reviewer for it.
func TestChatAgentsWorkNamesItsReviewer(t *testing.T) {
	h := newHarness(t)
	h.setRole("code-reviewer", sonnet, haiku)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	chat := h.srv.ChatClaimant()
	res, err := h.srv.ClaimWork(ctx, t1.PublicID, chat)
	if err != nil {
		t.Fatal(err)
	}
	if page := h.taskPageHTML(t1.ID.String()); strings.Contains(page, "Reviewed with") {
		t.Error("nothing has been reviewed yet")
	}
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "Added greet.go."); err != nil {
		t.Fatal(err)
	}
	page := h.taskPageHTML(t1.ID.String())
	mustContain(t, "reviewer", page, "Reviewed with <code>"+sonnet+"</code>, the project&#39;s reviewer for the chat agent&#39;s work.")
	mustContain(t, "executor", page, "Implemented by the chat agent.")
	mustContain(t, "submitted", page, "Submitted by the chat agent. A code reviewer is reviewing it.")
	mustContain(t, "unmeasured", page, "so its tokens aren&#39;t measured.")
}
