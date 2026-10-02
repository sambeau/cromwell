package server

// Tests for the first code review's findings on the executors milestone
// (REVIEW-020, round 1), against real Postgres with the phase-1 harness and the
// dispatcher held where a claim needs the queue quiet.

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/store"
)

// postNoFollow posts a form and returns the response without following a
// redirect, with the extra headers a browser would send.
func (h *harness) postNoFollow(path string, form map[string]string, headers map[string]string) *http.Response {
	h.t.Helper()
	vals := url.Values{}
	for k, v := range form {
		vals.Set(k, v)
	}
	req, err := http.NewRequest("POST", h.api.URL+path, strings.NewReader(vals.Encode()))
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// sendBackScript makes the reviewer send the work back n times, then approve,
// and the verifier approve.
func (h *harness) sendBackScript(n int) {
	h.t.Helper()
	for i := 0; i < n; i++ {
		h.mock.RespondOutcome("submit_review",
			`{"verdict":"request_changes","comments":[{"section_ref":"greet.go:3","body":"Greet should say welcome, not hello.","severity":"major"}],"reasoning":"wrong word"}`,
			provider.Usage{Input: 10, Output: 5})
	}
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"now right"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification",
		`{"criteria":[{"id":"AC1","met":true,"evidence":"greet.go present"}],"verdict":"approve","reasoning":"met"}`,
		provider.Usage{Input: 10, Output: 5})
}

// waitReturned waits for the nth code review to have sent the claim back.
func (h *harness) waitReturned(taskID store.Task, reviews int) {
	h.t.Helper()
	h.eventually("the claim is sent back", func() bool {
		return h.latestClaim(taskID.ID).State == lifecycle.ClaimReturned &&
			h.countWhere(`SELECT count(*) FROM dispatches WHERE ref_id = $1 AND purpose = 'review-code' AND state = 'succeeded'`, taskID.ID) >= reviews
	})
}

// TestReturnedClaimShowsTheReviewersCommentsOnTheTaskPage is B2 (FR-4.1,
// SD-6): a send-back records no execution, so the panel decides by the task's
// own round. It is also B11 (the chat reviewer is named from what the submit
// recorded, and keeps being named after config.yaml changes) and B13 (the
// panel and get_feature say what the last activity was in words).
func TestReturnedClaimShowsTheReviewersCommentsOnTheTaskPage(t *testing.T) {
	h := newHarness(t)
	h.setRole("code-reviewer", sonnet, haiku)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	t1 := h.taskByLocal("T1")
	res, err := h.srv.ClaimWork(ctx, t1.PublicID, chat)
	if err != nil {
		t.Fatal(err)
	}
	h.sendBackScript(1)
	h.releaseQueue()
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n\nfunc Greet() string { return \"hello\" }\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "Added greet.go."); err != nil {
		t.Fatal(err)
	}
	h.waitReturned(t1, 1)

	page := h.taskPageHTML(t1.ID.String())
	mustContain(t, "the comments", page, "What the code reviewer asked for")
	mustContain(t, "a finding", page, "Greet should say welcome, not hello.")
	mustContain(t, "its place", page, "<code>greet.go:3</code>")
	mustContain(t, "the send-back", page, "The code reviewer sent this back to the chat agent")
	mustContain(t, "activity in words", page, "Last activity: sent back by its code reviewer,")
	if strings.Contains(page, "sent_back") {
		t.Error("the panel prints a machine word")
	}
	mustContain(t, "the chat reviewer", page, "Reviewed with <code>"+sonnet+"</code>, the project&#39;s reviewer for the chat agent&#39;s work.")

	// A change to config.yaml later doesn't rewrite what the review used.
	h.editConfig("  # chat_review_model: same", "  chat_review_model: same")
	mustContain(t, "still the chat reviewer", h.taskPageHTML(t1.ID.String()), "Reviewed with <code>"+sonnet+"</code>")

	// get_feature carries the comments and the activity in words (MCP).
	tasks, _ := h.featureTasks("auth/login")
	task := tasks[t1.PublicID]
	if task["review_comments"] == nil {
		t.Errorf("get_feature lacks the review comments: %v", task)
	}
	if claim := asMap(t, task["claim"]); claim["last_activity_what"] != "sent back by its code reviewer" {
		t.Errorf("claim = %v", claim)
	}
}

// TestPersonsReturnedClaimShowsTheReviewersComments is B2 for a person's claim:
// Resume and Release, and what the reviewer asked for, before resuming.
func TestPersonsReturnedClaimShowsTheReviewersComments(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	t1 := h.taskByLocal("T1")
	res, err := h.srv.ClaimWork(context.Background(), t1.PublicID, h.srv.PersonClaimant())
	if err != nil {
		t.Fatal(err)
	}
	h.sendBackScript(1)
	h.releaseQueue()
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n\nfunc Greet() string { return \"hello\" }\n")
	if code, body := h.postForm("/ui/task/submit", map[string]string{"task": t1.PublicID, "summary": "Added greet.go."}); code != 200 {
		t.Fatalf("submit: %d %s", code, truncate(body, 300))
	}
	h.waitReturned(t1, 1)
	page := h.taskPageHTML(t1.ID.String())
	mustContain(t, "the comments", page, "What the code reviewer asked for")
	mustContain(t, "a finding", page, "Greet should say welcome, not hello.")
	mustContain(t, "resume", page, "Resume it to carry on in the working copy")
	mustContain(t, "activity in words", page, "Last activity: sent back by its code reviewer,")
	// Resuming says so, and the comments stay.
	_, body := h.postForm("/ui/task/claim", map[string]string{"task": t1.PublicID})
	mustContain(t, "resumed", body, "You are working on "+t1.PublicID+" again.")
	mustContain(t, "comments still there", body, "Greet should say welcome, not hello.")
}

// TestAPersonsOrUnchangedChatReviewerIsNotNamedAsTheChatReviewer is B11's
// other half: with chat_review_model set to "same" the chat agent's work gets
// the usual reviewer, and the task page doesn't say it had the project's
// reviewer for the chat agent's work.
func TestAPersonsOrUnchangedChatReviewerIsNotNamedAsTheChatReviewer(t *testing.T) {
	h := newHarness(t)
	h.setRole("code-reviewer", sonnet, haiku)
	h.editConfig("  # chat_review_model: same", "  chat_review_model: same")
	t1, res := h.claimedSetup()
	ctx := context.Background()
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	sub, err := h.srv.SubmitWork(ctx, t1.PublicID, h.srv.ChatClaimant(), "Added greet.")
	if err != nil {
		t.Fatal(err)
	}
	if sub.ReviewModel != haiku {
		t.Fatalf("the usual model is used: %s", sub.ReviewModel)
	}
	page := h.taskPageHTML(t1.ID.String())
	if strings.Contains(page, "the project&#39;s reviewer for the chat agent&#39;s work") {
		t.Error("named the chat reviewer although the usual one reviewed")
	}
}

// TestResubmittingAReviewedCommitStillQueuesAReview is B4: a submit whose head
// is a commit that was already reviewed in an earlier round still gets its
// own review, with a run to name.
func TestResubmittingAReviewedCommitStillQueuesAReview(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	t1 := h.taskByLocal("T1")
	res, err := h.srv.ClaimWork(ctx, t1.PublicID, chat)
	if err != nil {
		t.Fatal(err)
	}
	dir := res.WorkingCopy.Path
	h.sendBackScript(2)
	h.releaseQueue()

	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"hello\" }\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "First try."); err != nil {
		t.Fatal(err)
	}
	first := headOf(dir)
	h.waitReturned(t1, 1)

	if _, err := h.srv.ClaimWork(ctx, t1.PublicID, chat); err != nil {
		t.Fatal(err)
	}
	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"hi\" }\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "Second try."); err != nil {
		t.Fatal(err)
	}
	h.waitReturned(t1, 2)

	// Back to the first commit: its review key was used in round 1.
	if _, err := h.srv.ClaimWork(ctx, t1.PublicID, chat); err != nil {
		t.Fatal(err)
	}
	gitOut(t, dir, "reset", "--hard", first)
	sub, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "Back to the first, with a better summary.")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Commit != first || sub.ReviewRunID == "" {
		t.Fatalf("a review must be queued for the third submission: %+v", sub)
	}
	reviews := h.dispatchesOf(t1.ID, "review-code")
	if len(reviews) != 3 || reviews[2].ID.String() != sub.ReviewRunID {
		t.Fatalf("three reviews, the last the submit's: %d, %s", len(reviews), sub.ReviewRunID)
	}
	h.eventually("the feature is done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })
}

// TestAClaimJudgesACommitMadeJustBefore is B6 (FR-5.7): the claim runs the
// branch watch first, so a commit made before it is flagged as unclaimed work
// and not swallowed because a claim has opened.
func TestAClaimJudgesACommitMadeJustBefore(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()
	commitAs(t, dir, "quick.txt", "quick fix")
	if _, err := h.srv.ClaimWork(ctx, h.taskByLocal("T1").PublicID, h.srv.ChatClaimant()); err != nil {
		t.Fatal(err)
	}
	if h.pendingOf("unclaimed-commit", "feature", f.ID) == nil {
		t.Fatal("the commit made before the claim should have been flagged")
	}
}

// TestReconcileKeepsTheWatchedHead is B5: re-creating a missing worktree
// directory at boot doesn't move the branch watch past commits it never judged.
func TestReconcileKeepsTheWatchedHead(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()
	before, _ := store.LiveWorktreeForFeature(ctx, h.srv.Store.Pool, f.ID)
	if before.WatchedHead == "" {
		t.Fatal("the watch has a head")
	}
	// A commit made elsewhere while the server was down, and the directory gone.
	commitAs(t, dir, "elsewhere.txt", "made while the server was down")
	moved := headOf(dir)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	gitOut(t, h.root, "worktree", "prune")
	if err := h.srv.ReconcileWorktrees(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("the worktree was re-created: %v", err)
	}
	after, _ := store.LiveWorktreeForFeature(ctx, h.srv.Store.Pool, f.ID)
	if after.WatchedHead != before.WatchedHead || after.WatchedHead == moved {
		t.Fatalf("watched head moved from %s to %s (branch head %s)", before.WatchedHead, after.WatchedHead, moved)
	}
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) == nil {
		t.Fatal("the commit made while the server was down should be flagged")
	}
}

// TestAClaimWaitsForAnImplementersCompletion is B7: between an implementer's
// success and its completion's commit the working copy still holds the agent's
// work, so a claim on a sibling is refused with the agent-running sentence, and
// so is a sibling's implementer.
func TestAClaimWaitsForAnImplementersCompletion(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")
	d1 := h.dispatchesOf(t1.ID, "implement-task")[0]
	d2 := h.dispatchesOf(t2.ID, "implement-task")[0]
	// T1's implementer succeeded; its completion hasn't run.
	h.exec(`UPDATE dispatches SET state = 'succeeded', started_at = now(), finished_at = now() WHERE id = $1`, d1.ID)

	_, err := h.srv.ClaimWork(ctx, t2.PublicID, h.srv.ChatClaimant())
	wantRefusal(t, err, "An agent is implementing "+t1.PublicID+" in this feature's working copy.")
	if hold, err := store.ImplementHold(ctx, h.srv.Store.Pool, t2.ID, d2.ID); err != nil || hold != store.QueueReasonCompletionPending {
		t.Fatalf("the sibling's implementer is held: %q, %v", hold, err)
	}
	if res, err := h.startDispatch(d2.ID); err != nil || res.Outcome != store.StartQueued {
		t.Fatalf("StartImplementDispatch holds it too: %+v, %v", res, err)
	}

	// The completion commits; the window closes.
	writeIn(t, h.worktreeDir(t1.FeatureID), "a.go", "package main\n")
	if err := h.srv.completeImplementation(ctx, t1.ID, d1.ID, "Added a.go."); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.ClaimWork(ctx, t2.PublicID, h.srv.ChatClaimant()); err != nil {
		t.Fatalf("after the completion a claim is fine: %v", err)
	}
}

// TestACompletionWithAConfigErrorMakesNoCommit is B8: the review model is
// resolved before the commit, so a configuration error raises its checkpoint
// and leaves the working copy and the branch alone, with nothing for the watch
// to call unclaimed.
func TestACompletionWithAConfigErrorMakesNoCommit(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	d := h.dispatchesOf(t1.ID, "implement-task")[0]
	writeIn(t, dir, "greet.go", "package main\n")
	headBefore := headOf(dir)
	if err := os.Remove(filepath.Join(h.root, ".subutai/roles/code-reviewer.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.completeImplementation(ctx, t1.ID, d.ID, "Added greet."); err != nil {
		t.Fatal(err)
	}
	if headOf(dir) != headBefore {
		t.Error("a configuration error must not leave a commit behind")
	}
	if dirty, _ := workingCopyDirty(dir); !dirty {
		t.Error("the implementer's work stays in the working copy")
	}
	if h.taskState(t1.ID) != lifecycle.TaskActive {
		t.Errorf("the task is still active, got %s", h.taskState(t1.ID))
	}
	if h.pendingOf("config-error", "task", t1.ID) == nil {
		t.Error("the configuration error is raised")
	}
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Error("nothing was committed, so nothing is unclaimed")
	}
}

// TestUIClaimRoutesRedirectAndRefuseCrossSitePosts is B9 and B10 (FR-4.2): a
// post that works redirects with 303 to the task page, which says what
// happened, and a post from another site is refused before it does anything.
func TestUIClaimRoutesRedirectAndRefuseCrossSitePosts(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	form := map[string]string{"task": t1.PublicID}
	cross := map[string]string{"Sec-Fetch-Site": "cross-site"}

	// Cross-site, nothing happens.
	for _, route := range []string{"/ui/task/claim", "/ui/task/submit", "/ui/task/release"} {
		if resp := h.postNoFollow(route, map[string]string{"task": t1.PublicID, "summary": "x"}, cross); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s from another site = %d, want 403", route, resp.StatusCode)
		}
	}
	if _, err := store.CurrentClaimFor(ctx, h.srv.Store.Pool, "task", t1.ID); err == nil {
		t.Fatal("a cross-site claim must not claim")
	}
	if resp := h.postNoFollow("/ui/task/claim", form, map[string]string{"Origin": "http://evil.example"}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a foreign Origin = %d, want 403", resp.StatusCode)
	}

	// The person's own page works, and redirects.
	same := map[string]string{"Sec-Fetch-Site": "same-origin"}
	resp := h.postNoFollow("/ui/task/claim", form, same)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/ui/t/"+t1.ID.String()+"?done=claimed" {
		t.Fatalf("claim = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	_, body := h.getUI(resp.Header.Get("Location"))
	mustContain(t, "claim notice", body, "You claimed "+t1.PublicID+".")
	if c, err := store.CurrentClaimFor(ctx, h.srv.Store.Pool, "task", t1.ID); err != nil || c.Kind != "person" {
		t.Fatalf("the person's claim: %+v %v", c, err)
	}
	// Renewing says so.
	resp = h.postNoFollow("/ui/task/claim", form, same)
	if resp.Header.Get("Location") != "/ui/t/"+t1.ID.String()+"?done=renewed" {
		t.Errorf("renew location = %q", resp.Header.Get("Location"))
	}

	// Submit, then release: each redirects. A refresh is a GET, so no error.
	wt, _ := store.LiveWorktreeForFeature(ctx, h.srv.Store.Pool, t1.FeatureID)
	writeIn(t, h.srv.worktreeAbs(wt.Path), "a.go", "package main\n")
	resp = h.postNoFollow("/ui/task/submit", map[string]string{"task": t1.PublicID, "summary": "Added a.go."}, same)
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/ui/t/"+t1.ID.String()+"?done=submitted") {
		t.Fatalf("submit = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	for i := 0; i < 2; i++ { // a refresh shows the same notice, and no error
		_, body = h.getUI(resp.Header.Get("Location"))
		mustContain(t, "submit notice", body, "You submitted "+t1.PublicID+" for code review.")
		if strings.Contains(body, "banner--error") {
			t.Error("a refresh shows an error banner")
		}
	}
	// A refusal still renders the banner.
	refused := h.postNoFollow("/ui/task/release", form, same)
	if refused.StatusCode != http.StatusOK {
		t.Fatalf("refused release = %d", refused.StatusCode)
	}
	b, _ := io.ReadAll(refused.Body)
	if !strings.Contains(string(b), "is in code review. Wait for the verdict.") {
		t.Errorf("the refusal is a banner: %s", truncate(string(b), 400))
	}
}
