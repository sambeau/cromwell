package server

// The claim service and the dispatcher's races (SPEC-020 FR-2, FR-8), with
// the mock provider against real Postgres and the phase-1 harness. There are
// no MCP tools or UI pages yet, so the tests call ClaimWork, SubmitWork and
// ReleaseClaim directly. The dispatcher is "held" by shrinking the budget cap
// in config.yaml, which the governor reads afresh (O-6): queued dispatches
// then wait with the reason "budget" until the cap is put back.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/rules"
	"subutai/internal/store"
)

// ---- Fixtures ----

func devPlanTwoIndependent() string {
	return strings.Replace(devPlanOneTask,
		"| T1 | Greeting helper | | Add greet.go with a Greet function returning a welcome string |\n",
		"| T1 | First | | Add a.go with package main |\n| T2 | Second | | Add b.go with a Bee function |\n", 1)
}

func devPlanChain() string {
	return strings.Replace(devPlanOneTask,
		"| T1 | Greeting helper | | Add greet.go with a Greet function returning a welcome string |\n",
		"| T1 | First | | Add a.go with package main |\n| T2 | Second | T1 | Add b.go with a Bee function |\n", 1)
}

// claimsFeature takes auth/login to ready, with a plan of n tasks.
func (h *harness) claimsFeature(plan string, n int) {
	h.t.Helper()
	h.approveDoc(h.setupFeatureWithSpec())
	h.approveDoc(h.addDevPlan(plan))
	h.eventually("ready with its tasks", func() bool {
		return h.featureState("auth/login") == lifecycle.FeatReady && len(h.tasks("auth/login")) == n
	})
}

func (h *harness) holdQueue() { h.editConfig("cap_usd: 50.00", "cap_usd: 0.50") }

func (h *harness) releaseQueue() {
	h.editConfig("cap_usd: 0.50", "cap_usd: 50.00")
	h.srv.Dispatcher.Kick()
}

// startHeld presses Start building with the dispatcher held, and waits for
// the tasks to be active with an implement dispatch queued each.
func (h *harness) startHeld(tasks int) {
	h.t.Helper()
	h.holdQueue()
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		h.t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("implement dispatches queued", func() bool {
		var n int
		_ = h.srv.Store.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM dispatches WHERE purpose = 'implement-task' AND state = 'queued'`).Scan(&n)
		return n == tasks
	})
}

func (h *harness) taskByLocal(local string) store.Task {
	h.t.Helper()
	for _, tk := range h.tasks("auth/login") {
		if tk.LocalID == local {
			return tk
		}
	}
	h.t.Fatalf("no task %s", local)
	return store.Task{}
}

func (h *harness) exec(sql string, args ...any) {
	h.t.Helper()
	if _, err := h.srv.Store.Pool.Exec(context.Background(), sql, args...); err != nil {
		h.t.Fatalf("%s: %v", sql, err)
	}
}

// dispatchesOf lists a task's dispatches of a purpose, oldest first.
func (h *harness) dispatchesOf(taskID uuid.UUID, purpose string) []store.Dispatch {
	h.t.Helper()
	rows, err := h.srv.Store.Pool.Query(context.Background(),
		`SELECT id FROM dispatches WHERE ref_id = $1 AND purpose = $2 ORDER BY queued_at, id`, taskID, purpose)
	if err != nil {
		h.t.Fatal(err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			h.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []store.Dispatch
	for _, id := range ids {
		d, err := store.GetDispatch(context.Background(), h.srv.Store.Pool, id)
		if err != nil {
			h.t.Fatal(err)
		}
		out = append(out, *d)
	}
	return out
}

func (h *harness) execsOf(taskID uuid.UUID) []store.Execution {
	h.t.Helper()
	es, err := store.ExecutionsFor(context.Background(), h.srv.Store.Pool, "task", taskID)
	if err != nil {
		h.t.Fatal(err)
	}
	return es
}

func (h *harness) latestClaim(taskID uuid.UUID) *store.Claim {
	h.t.Helper()
	c, err := store.LatestClaimFor(context.Background(), h.srv.Store.Pool, "task", taskID)
	if err != nil {
		h.t.Fatalf("latest claim: %v", err)
	}
	return c
}

func (h *harness) taskState(id uuid.UUID) lifecycle.TaskState {
	h.t.Helper()
	tk, err := store.GetTask(context.Background(), h.srv.Store.Pool, id)
	if err != nil {
		h.t.Fatal(err)
	}
	return tk.State
}

// setRole puts a role on a different model, so that a test can tell the
// usual reviewer from the project's reviewer for the chat agent's work.
func (h *harness) setRole(role, from, to string) {
	h.t.Helper()
	p := filepath.Join(h.root, ".subutai/roles", role+".yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		h.t.Fatal(err)
	}
	out := strings.Replace(string(data), "model: "+from, "model: "+to, 1)
	if out == string(data) {
		h.t.Fatalf("role %s is not on %s", role, from)
	}
	if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func writeIn(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitIn(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

func wantRefusal(t *testing.T, err error, contains string) {
	t.Helper()
	r, ok := AsClaimRefusal(err)
	if !ok {
		t.Fatalf("want a refusal containing %q, got %v", contains, err)
	}
	if !strings.Contains(r.Sentence, contains) {
		t.Fatalf("refusal %q should contain %q", r.Sentence, contains)
	}
}

const (
	sonnet = "claude-sonnet-5" // the priciest configured model
	haiku  = "claude-haiku-4-5"
)

// ---- Claiming ----

// TestClaimTakesAnActiveTaskFromTheQueue is FR-2.3 and SD-3: a task Start
// building made active, with an implement dispatch queued, is claimable; the
// dispatch is cancelled, and the task's execution, base commit and result are
// the claim's.
func TestClaimTakesAnActiveTaskFromTheQueue(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")
	if t1.State != lifecycle.TaskActive {
		t.Fatalf("T1 should be active: %s", t1.State)
	}
	queued := h.dispatchesOf(t1.ID, "implement-task")

	res, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant())
	if err != nil {
		t.Fatal(err)
	}
	if res.Claim.State != lifecycle.ClaimOpen || res.Renewed || res.Resumed {
		t.Fatalf("a new open claim: %+v", res.Claim)
	}
	if len(res.Cancelled) != 1 || res.Cancelled[0] != queued[0].ID {
		t.Fatalf("the queued dispatch is cancelled: %v", res.Cancelled)
	}
	if d := h.dispatchesOf(t1.ID, "implement-task")[0]; d.State != "cancelled" {
		t.Fatalf("dispatch state = %s", d.State)
	}
	// T2's is untouched.
	if d := h.dispatchesOf(t2.ID, "implement-task")[0]; d.State != "queued" {
		t.Fatalf("T2's dispatch state = %s", d.State)
	}

	// The working copy, and the base commit reset to its head.
	if !filepath.IsAbs(res.WorkingCopy.Path) {
		t.Fatalf("the working copy is absolute: %s", res.WorkingCopy.Path)
	}
	head := gitOut(t, res.WorkingCopy.Path, "rev-parse", "HEAD")
	if res.WorkingCopy.BaseCommit != head || res.WorkingCopy.Branch != "subutai/auth/login" {
		t.Fatalf("working copy = %+v, head %s", res.WorkingCopy, head)
	}
	if res.Round != 1 {
		t.Fatalf("round = %d", res.Round)
	}

	// One execution, the chat agent's, unmeasured, from the head.
	es := h.execsOf(t1.ID)
	if len(es) != 1 || es[0].Kind != "chat" || es[0].Measured || es[0].Round != 1 ||
		es[0].StartHead != head || es[0].ClaimID == nil || *es[0].ClaimID != res.Claim.ID || es[0].Via != "mcp" {
		t.Fatalf("executions = %+v", es)
	}

	// The contract.
	for _, k := range []string{"spec", "dev_plan", "task"} {
		if _, ok := res.Contract[k]; !ok {
			t.Errorf("contract has no %s", k)
		}
	}
	spec := res.Contract["spec"].(map[string]any)
	if !strings.Contains(spec["body"].(string), "Login form") || spec["path"] != "docs/specs/login.md" {
		t.Errorf("spec = %v", spec)
	}
	if res.Contract["task"].(map[string]any)["id"] != t1.PublicID {
		t.Errorf("task = %v", res.Contract["task"])
	}
	rulesText := strings.Join(res.Rules, "\n")
	for _, want := range []string{"Don't commit", "build", "run_tests"} {
		if !strings.Contains(rulesText, want) {
			t.Errorf("rules should mention %q: %s", want, rulesText)
		}
	}
	if res.ReviewComments != nil {
		t.Errorf("a first round has no review comments")
	}

	// Claiming again renews, with the same result.
	again, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant())
	if err != nil || !again.Renewed || again.Claim.ID != res.Claim.ID {
		t.Fatalf("renewal: %+v, %v", again, err)
	}
	if es := h.execsOf(t1.ID); len(es) != 1 {
		t.Fatalf("a renewal records no execution: %+v", es)
	}
	if c := h.latestClaim(t1.ID); c.LastActivity != "renewed" {
		t.Fatalf("last activity = %s", c.LastActivity)
	}

	// The executor line.
	line, err := h.srv.executorLine(ctx, h.srv.Store.Pool, &t1)
	if err != nil || !strings.HasPrefix(line.Sentence, "Being implemented by the chat agent, who claimed it ") || line.Measured {
		t.Fatalf("executor line = %+v, %v", line, err)
	}
	if line.Kind != "chat" || line.Who != "the chat agent" {
		t.Fatalf("executor fields = %+v", line)
	}
}

// TestClaimRefusals is FR-2.4 and FR-6.5: each refusal, as a sentence.
func TestClaimRefusals(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanChain(), 2)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")

	// Nothing but a task's ID names something to claim (FR-6.3).
	f, _ := h.srv.featureByPath(ctx, "auth/login")
	for _, ref := range []string{f.PublicID, "BUG-007", f.PublicID + "-spec", "hello", ""} {
		_, err := h.srv.ClaimWork(ctx, ref, chat)
		wantRefusal(t, err, "Only tasks can be claimed. Verification is always done by Subutai's verifier.")
	}
	_, err := h.srv.ClaimWork(ctx, f.PublicID+"-T09", chat)
	wantRefusal(t, err, "There is no task "+f.PublicID+"-T09")

	// A feature nobody has started isn't being built.
	_, err = h.srv.ClaimWork(ctx, t1.PublicID, chat)
	wantRefusal(t, err, f.PublicID+" isn't being built yet. A person starts building from the feature's page; you can't.")

	h.startHeld(1)

	// A pending task waits for the one it depends on.
	_, err = h.srv.ClaimWork(ctx, t2.PublicID, chat)
	wantRefusal(t, err, t2.PublicID+" can't be claimed yet: it waits for T01 to be done. Claim one of those, or wait.")

	// An agent running on the feature (a dispatch made to look like it).
	d := h.dispatchesOf(t1.ID, "implement-task")[0]
	h.exec(`UPDATE dispatches SET state = 'running', started_at = now(), heartbeat_at = now() WHERE id = $1`, d.ID)
	_, err = h.srv.ClaimWork(ctx, t1.PublicID, chat)
	wantRefusal(t, err, "An agent is implementing "+t1.PublicID+" in this feature's working copy. Claim a task when it has finished; get_feature shows when.")
	h.exec(`UPDATE dispatches SET state = 'queued', started_at = NULL, heartbeat_at = NULL WHERE id = $1`, d.ID)

	// An attempt the stall sweep gave up on may still be running tools.
	h.exec(`UPDATE dispatches SET state = 'failed', heartbeat_at = now() WHERE id = $1`, d.ID)
	_, err = h.srv.ClaimWork(ctx, t1.PublicID, chat)
	wantRefusal(t, err, "An agent is implementing")
	h.exec(`UPDATE dispatches SET state = 'queued', heartbeat_at = NULL WHERE id = $1`, d.ID)

	// A spec under revision.
	h.exec(`UPDATE features SET spec_stale = true WHERE id = $1`, f.ID)
	_, err = h.srv.ClaimWork(ctx, t1.PublicID, chat)
	wantRefusal(t, err, f.PublicID+"'s spec is being revised. Claiming waits until a person decides whether building continues.")
	h.exec(`UPDATE features SET spec_stale = false WHERE id = $1`, f.ID)

	// A person's claim is theirs: the chat agent can't take it, and says who.
	person := h.srv.PersonClaimant()
	if _, err := h.srv.ClaimWork(ctx, t1.PublicID, person); err != nil {
		t.Fatal(err)
	}
	_, err = h.srv.ClaimWork(ctx, t1.PublicID, chat)
	wantRefusal(t, err, t1.PublicID+" is claimed by "+person.Actor+". Only a person can release a claim, in the web UI.")
	// ...and can't submit it.
	_, err = h.srv.SubmitWork(ctx, t1.PublicID, chat, "Done.")
	wantRefusal(t, err, "is claimed by "+person.Actor+", not by you")
	// Nor release it: the surface is the web UI's.
	_, err = h.srv.SubmitWork(ctx, t1.PublicID, Claimant{Kind: person.Kind, Actor: person.Actor, Via: "mcp"}, "Done.")
	wantRefusal(t, err, "not by you")

	// Done and abandoned tasks are over.
	h.exec(`UPDATE tasks SET state = 'done' WHERE id = $1`, t2.ID)
	_, err = h.srv.ClaimWork(ctx, t2.PublicID, chat)
	wantRefusal(t, err, t2.PublicID+" is already done. Choose another task from get_feature.")
}

// TestSecondClaimInAFeatureIsRefused is SD-4: one hand in the working copy.
func TestSecondClaimInAFeatureIsRefused(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")
	if _, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant()); err != nil {
		t.Fatal(err)
	}
	_, err := h.srv.ClaimWork(ctx, t2.PublicID, h.srv.PersonClaimant())
	wantRefusal(t, err, t1.PublicID+" is claimed by the chat agent, in this feature's working copy. Only one task in a feature can be worked at a time; wait for it to be submitted.")
	if c, err := store.CurrentClaimFor(ctx, h.srv.Store.Pool, "task", t2.ID); err == nil {
		t.Fatalf("T2 has no claim: %+v", c)
	}
}

// ---- Submitting ----

// TestSubmitCommitsAndQueuesTheReviewWithTheChatModel is FR-2.5 and FR-8.3:
// the commit and its trailers, its hash recorded as Subutai's, the task in
// review, and the code review queued with the project's reviewer for the chat
// agent's work, not the usual one.
func TestSubmitCommitsAndQueuesTheReviewWithTheChatModel(t *testing.T) {
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
	dir := res.WorkingCopy.Path

	// A summary is needed, and so is a change.
	_, err = h.srv.SubmitWork(ctx, t1.PublicID, chat, "  ")
	wantRefusal(t, err, "needs a summary")
	_, err = h.srv.SubmitWork(ctx, t1.PublicID, chat, "I did it.")
	wantRefusal(t, err, "Nothing in the working copy has changed since you claimed "+t1.PublicID+". Make the change, then submit; or ask a person to release the claim.")

	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"welcome\" }\n")
	sub, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "Added greet.go with Greet.")
	if err != nil {
		t.Fatal(err)
	}
	head := gitOut(t, dir, "rev-parse", "HEAD")
	if sub.Commit != head || sub.Task.State != lifecycle.TaskReview || sub.Notice != "" {
		t.Fatalf("submit result = %+v (head %s)", sub, head)
	}

	// The commit, its trailers, and its hash as Subutai's.
	msg := gitOut(t, dir, "log", "-1", "--format=%B")
	for _, want := range []string{"subutai: T1 — Greeting helper", "Added greet.go with Greet.", "Subutai-Executor: chat", "Subutai-Claim: " + res.Claim.ID.String()} {
		if !strings.Contains(msg, want) {
			t.Errorf("commit message lacks %q:\n%s", want, msg)
		}
	}
	wt, _ := store.LiveWorktreeForFeature(ctx, h.srv.Store.Pool, t1.FeatureID)
	if ok, _ := store.IsWorktreeCommit(ctx, h.srv.Store.Pool, wt.ID, head); !ok {
		t.Error("the submit's commit is recorded as Subutai's")
	}
	if got := gitOut(t, dir, "show", "--stat", "--format=", "HEAD"); !strings.Contains(got, "greet.go") {
		t.Errorf("the commit holds the work: %s", got)
	}

	// The claim and the execution.
	c := h.latestClaim(t1.ID)
	if c.State != lifecycle.ClaimSubmitted || c.LastActivity != "submitted" || c.SubmittedAt == nil {
		t.Fatalf("claim = %+v", c)
	}
	if es := h.execsOf(t1.ID); len(es) != 1 || es[0].SubmittedAt == nil {
		t.Fatalf("executions = %+v", es)
	}

	// The review: queued, with the chat reviewer model, whatever the usual is.
	reviews := h.dispatchesOf(t1.ID, "review-code")
	if len(reviews) != 1 || reviews[0].State != "queued" {
		t.Fatalf("reviews = %+v", reviews)
	}
	if reviews[0].Model != sonnet || sub.ReviewModel != sonnet || sub.ReviewRunID != reviews[0].ID.String() {
		t.Fatalf("the chat agent's work is reviewed with %s, the priciest model: dispatch %s, result %+v", sonnet, reviews[0].Model, sub)
	}

	// Out-of-order acts are refused.
	_, err = h.srv.SubmitWork(ctx, t1.PublicID, chat, "Again.")
	wantRefusal(t, err, "was already submitted and is in code review")
	_, err = h.srv.ClaimWork(ctx, t1.PublicID, chat)
	wantRefusal(t, err, "is in code review")
	err = h.srv.ReleaseClaim(ctx, t1.PublicID, "sam")
	wantRefusal(t, err, t1.PublicID+" is in code review. Wait for the verdict.")
	line, _ := h.srv.executorLine(ctx, h.srv.Store.Pool, mustTask(t, h, t1.ID))
	if line.Sentence != "Implemented by the chat agent." {
		t.Fatalf("executor line = %q", line.Sentence)
	}
}

func mustTask(t *testing.T, h *harness, id uuid.UUID) *store.Task {
	t.Helper()
	tk, err := store.GetTask(context.Background(), h.srv.Store.Pool, id)
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

// TestPersonsSubmitIsReviewedWithTheUsualModel is FR-8.3's other half: only
// the chat agent's work gets the stronger reviewer.
func TestPersonsSubmitIsReviewedWithTheUsualModel(t *testing.T) {
	h := newHarness(t)
	h.setRole("code-reviewer", sonnet, haiku)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	ctx := context.Background()
	person := h.srv.PersonClaimant()
	t1 := h.taskByLocal("T1")
	res, err := h.srv.ClaimWork(ctx, t1.PublicID, person)
	if err != nil {
		t.Fatal(err)
	}
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	sub, err := h.srv.SubmitWork(ctx, t1.PublicID, person, "Added greet.go.")
	if err != nil {
		t.Fatal(err)
	}
	if sub.ReviewModel != haiku {
		t.Fatalf("a person's work is reviewed with the usual model: %s", sub.ReviewModel)
	}
	if msg := gitOut(t, res.WorkingCopy.Path, "log", "-1", "--format=%B"); !strings.Contains(msg, "Subutai-Executor: person") {
		t.Errorf("trailer: %s", msg)
	}
	if es := h.execsOf(t1.ID); len(es) != 1 || es[0].Kind != "person" || es[0].Via != "ui" || es[0].Actor != person.Actor {
		t.Fatalf("executions = %+v", es)
	}

	// With the setting "same", the chat agent's work gets the usual model too.
	// (The cap's text sits above the claims block, so the edit is one line.)
	h.editConfig("  # chat_review_model: same", "  chat_review_model: same")
	if m, err := h.srv.codeReviewModelFor(ctx, t1.ID); err != nil || m != haiku {
		t.Fatalf("model = %s, %v", m, err)
	}
}

// codeReviewModelFor is the test's view of codeReviewModel.
func (s *Server) codeReviewModelFor(ctx context.Context, taskID uuid.UUID) (string, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return "", err
	}
	return s.codeReviewModel(ctx, s.Store.Pool, cfg, taskID, cfg.Assignments["review-code"])
}

// ---- The whole path ----

// TestSendBackReturnsToTheClaimAndApprovalEndsIt is SD-6, FR-2.6 and FR-6.3:
// a send-back returns the task to its claim and queues no implementer;
// resuming gives the comments back; the second review's approval ends the
// claim as done, and the dispatched verifier still verifies the feature,
// which merges.
func TestSendBackReturnsToTheClaimAndApprovalEndsIt(t *testing.T) {
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
	dir := res.WorkingCopy.Path

	// The whole chain, scripted up front: send back, approve, verify.
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"request_changes","comments":[{"section_ref":"greet.go:3","body":"Greet should say welcome, not hello.","severity":"major"}],"reasoning":"wrong word"}`,
		provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"now right"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification",
		`{"criteria":[{"id":"AC1","met":true,"evidence":"greet.go present"}],"verdict":"approve","reasoning":"met"}`,
		provider.Usage{Input: 10, Output: 5})
	h.releaseQueue()

	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"hello\" }\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "Added greet.go."); err != nil {
		t.Fatal(err)
	}
	h.eventually("the claim is sent back", func() bool { return h.latestClaim(t1.ID).State == lifecycle.ClaimReturned })

	// Back to the claim: the task is active, and no implementer was queued.
	if h.taskState(t1.ID) != lifecycle.TaskActive {
		t.Fatalf("task = %s", h.taskState(t1.ID))
	}
	if c := h.latestClaim(t1.ID); c.LastActivity != "sent_back" {
		t.Fatalf("activity = %s", c.LastActivity)
	}
	for _, d := range h.dispatchesOf(t1.ID, "implement-task") {
		if d.State != "cancelled" {
			t.Fatalf("no implementer for a claimed task: %+v", d)
		}
	}
	// The comments are on record, with the run that made them.
	rc, err := h.srv.reviewComments(ctx, h.srv.Store.Pool, t1.ID)
	if err != nil || rc == nil || len(rc.Comments) != 1 || rc.RunID == "" {
		t.Fatalf("review comments = %+v, %v", rc, err)
	}
	if f := rc.Comments[0]; f.File != "greet.go" || f.Line != 3 || f.Severity != "major" || !strings.Contains(f.Text, "welcome") {
		t.Fatalf("finding = %+v", f)
	}

	// A returned claim can't be submitted until resumed.
	_, err = h.srv.SubmitWork(ctx, t1.PublicID, chat, "Again.")
	wantRefusal(t, err, "was sent back by its code reviewer. Resume it by claiming it again")

	// Resuming: the same claim, round 2, with the comments.
	res2, err := h.srv.ClaimWork(ctx, t1.PublicID, chat)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Resumed || res2.Claim.ID != res.Claim.ID || res2.Round != 2 || res2.Claim.State != lifecycle.ClaimOpen {
		t.Fatalf("resumption = %+v", res2)
	}
	if res2.ReviewComments == nil || len(res2.ReviewComments.Comments) != 1 ||
		!strings.Contains(res2.ReviewComments.Comments[0].Text, "welcome") {
		t.Fatalf("the comments come back with the claim: %+v", res2.ReviewComments)
	}
	es := h.execsOf(t1.ID)
	if len(es) != 2 || es[1].Round != 2 || es[1].Kind != "chat" || es[1].SubmittedAt != nil {
		t.Fatalf("a resumed claim is a new execution row: %+v", es)
	}

	// Nothing changed since the resumption: no submit.
	_, err = h.srv.SubmitWork(ctx, t1.PublicID, chat, "Again.")
	wantRefusal(t, err, "Nothing in the working copy has changed")

	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"welcome\" }\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, chat, "Said welcome."); err != nil {
		t.Fatal(err)
	}

	// Approved: the claim is done, and the feature is verified and merged by
	// the dispatched verifier all the same.
	h.eventually("feature done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })
	c := h.latestClaim(t1.ID)
	if c.State != lifecycle.ClaimEnded || c.EndReason != "done" {
		t.Fatalf("the approved task's claim is done: %+v", c)
	}
	if h.taskState(t1.ID) != lifecycle.TaskDone {
		t.Fatalf("task = %s", h.taskState(t1.ID))
	}
	reviews := h.dispatchesOf(t1.ID, "review-code")
	if len(reviews) != 2 || reviews[0].Model != sonnet || reviews[1].Model != sonnet {
		t.Fatalf("both reviews use the chat reviewer model: %+v", reviews)
	}
	var verifies int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE purpose = 'verify-feature' AND state = 'succeeded'`).Scan(&verifies)
	if verifies != 1 {
		t.Fatalf("a dispatched verifier verified the feature: %d", verifies)
	}
	if _, err := os.Stat(filepath.Join(h.root, "greet.go")); err != nil {
		t.Fatalf("the claimed work is merged: %v", err)
	}
	// The task was implemented by the chat agent, and is unmeasured.
	line, _ := h.srv.executorLine(ctx, h.srv.Store.Pool, mustTask(t, h, t1.ID))
	if line.Sentence != "Implemented by the chat agent." || line.Measured {
		t.Fatalf("executor line = %+v", line)
	}
	if un, _ := store.TaskUnmeasured(ctx, h.srv.Store.Pool, t1.ID); !un {
		t.Fatal("a chat-implemented task is unmeasured")
	}
}

// TestReleaseKeepsTheWorkAndQueuesAnImplementer is SD-7 and FR-2.9: what was
// left in the working copy is committed, the claim ends, the task stays
// active and an implement dispatch is queued, which then finishes the task.
func TestReleaseKeepsTheWorkAndQueuesAnImplementer(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	ctx := context.Background()
	chat := h.srv.ChatClaimant()
	t1 := h.taskByLocal("T1")

	err := h.srv.ReleaseClaim(ctx, t1.PublicID, "sam")
	wantRefusal(t, err, t1.PublicID+" has no claim to release.")

	res, err := h.srv.ClaimWork(ctx, t1.PublicID, chat)
	if err != nil {
		t.Fatal(err)
	}
	dir := res.WorkingCopy.Path
	writeIn(t, dir, "greet.go", "package main\n\n// half done\n")

	// An implementer, a review and a verification, for after the release.
	h.mock.RespondOutcome("submit_implementation", `{"summary":"finished it"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"fine"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification",
		`{"criteria":[{"id":"AC1","met":true,"evidence":"present"}],"verdict":"approve","reasoning":"met"}`,
		provider.Usage{Input: 10, Output: 5})

	if err := h.srv.ReleaseClaim(ctx, t1.PublicID, "sam"); err != nil {
		t.Fatal(err)
	}
	c := h.latestClaim(t1.ID)
	if c.State != lifecycle.ClaimEnded || c.EndReason != "released" || c.EndedBy != "sam" {
		t.Fatalf("claim = %+v", c)
	}
	msg := gitOut(t, dir, "log", "-1", "--format=%B")
	for _, want := range []string{"subutai: T1 — work left by the chat agent, released", "Subutai-Executor: chat", "Subutai-Claim: " + res.Claim.ID.String()} {
		if !strings.Contains(msg, want) {
			t.Errorf("commit message lacks %q:\n%s", want, msg)
		}
	}
	head := gitOut(t, dir, "rev-parse", "HEAD")
	wt, _ := store.LiveWorktreeForFeature(ctx, h.srv.Store.Pool, t1.FeatureID)
	if ok, _ := store.IsWorktreeCommit(ctx, h.srv.Store.Pool, wt.ID, head); !ok {
		t.Error("the release's commit is recorded as Subutai's")
	}
	if h.taskState(t1.ID) != lifecycle.TaskActive {
		t.Fatalf("task = %s", h.taskState(t1.ID))
	}
	var queued int
	for _, d := range h.dispatchesOf(t1.ID, "implement-task") {
		if d.State == "queued" {
			queued++
		}
	}
	if queued != 1 {
		t.Fatalf("an implement dispatch is queued: %d", queued)
	}
	// A late submit says the claim was released, and by whom.
	_, err = h.srv.SubmitWork(ctx, t1.PublicID, chat, "Done.")
	wantRefusal(t, err, "was released by sam")

	// The agent takes the task over and finishes it.
	h.releaseQueue()
	h.eventually("feature done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })
	es := h.execsOf(t1.ID)
	if len(es) != 2 || es[0].Kind != "chat" || es[0].SubmittedAt != nil || es[1].Kind != "agent" || es[1].SubmittedAt == nil || !es[1].Measured {
		t.Fatalf("executions = %+v", es)
	}
	if es[1].StartHead == "" {
		t.Error("an agent's execution records the head it began at")
	}
	line, _ := h.srv.executorLine(ctx, h.srv.Store.Pool, mustTask(t, h, t1.ID))
	if line.Sentence != "Implemented by the implementer (claude-sonnet-5), from work the chat agent started." || line.Measured {
		t.Fatalf("executor line = %+v", line)
	}
	// An agent's code review of a released claim uses the usual model.
	if r := h.dispatchesOf(t1.ID, "review-code"); len(r) != 1 || r[0].Model != sonnet {
		t.Fatalf("reviews = %+v", r)
	}
}

// ---- The governor ----

// TestGovernorHoldsImplementersWhileAClaimIsOpen is SD-4 and FR-2.7: an open
// claim holds the feature's queued implement dispatches, with the reason, and
// submitting lets the next one start.
func TestGovernorHoldsImplementersWhileAClaimIsOpen(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")
	res, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant())
	if err != nil {
		t.Fatal(err)
	}

	h.releaseQueue()
	h.eventually("T2's dispatch is held for the claim", func() bool {
		d := h.dispatchesOf(t2.ID, "implement-task")[0]
		return d.State == "queued" && d.QueueReason != nil && *d.QueueReason == store.QueueReasonClaimOpen
	})
	if es := h.execsOf(t2.ID); len(es) != 0 {
		t.Fatalf("no agent began on T2: %+v", es)
	}

	// Submitting frees the working copy: T2's implementer starts. (Nothing is
	// scripted, so its run fails; that doesn't matter here.)
	writeIn(t, res.WorkingCopy.Path, "a.go", "package main\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, h.srv.ChatClaimant(), "Added a.go."); err != nil {
		t.Fatal(err)
	}
	h.eventually("T2's implementer starts", func() bool {
		es := h.execsOf(t2.ID)
		return len(es) == 1 && es[0].Kind == "agent" && es[0].Measured
	})
}

// TestClaimOnAnExhaustedDispatchWithdrawsItsCheckpoint is FR-6.5: a task
// whose agent failed all its attempts can be claimed; the dispatch-failure
// question is withdrawn, and neither a Retry answer nor the retry sweep starts
// anything.
func TestClaimOnAnExhaustedDispatchWithdrawsItsCheckpoint(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	d := h.dispatchesOf(t1.ID, "implement-task")[0]

	// The dispatch ran out of attempts, and the rules raised their question.
	h.exec(`UPDATE dispatches SET state = 'failed', attempt = 3, error = 'boom', finished_at = now() WHERE id = $1`, d.ID)
	var cp *store.Checkpoint
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		cp, err = store.CreateCheckpoint(ctx, tx, "dispatch-failure", "task", t1.ID,
			"Dispatch failed all attempts. Retry or cancel?", map[string]any{"dispatch_id": d.ID.String(), "error": "boom"})
		return err
	}); err != nil || cp == nil {
		t.Fatalf("checkpoint: %v", err)
	}

	res, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Cancelled) != 1 || res.Cancelled[0] != d.ID {
		t.Fatalf("cancelled = %v", res.Cancelled)
	}
	got, err := store.GetCheckpoint(ctx, h.srv.Store.Pool, cp.ID)
	if err != nil || got.State != "withdrawn" {
		t.Fatalf("checkpoint = %+v, %v", got, err)
	}
	if after := h.dispatchesOf(t1.ID, "implement-task")[0]; after.State != "cancelled" {
		t.Fatalf("dispatch = %s", after.State)
	}

	// A Retry answer that arrives afterwards changes nothing, and says so.
	if err := h.srv.execute(ctx, rules.RetryDispatch{DispatchID: d.ID}); err != nil {
		t.Fatalf("a Retry on a cancelled dispatch is not an error: %v", err)
	}
	if after := h.dispatchesOf(t1.ID, "implement-task")[0]; after.State != "cancelled" {
		t.Fatalf("a Retry must not revive it: %s", after.State)
	}
	var ignored int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'dispatch.retry_ignored' AND ref_id = $1`, t1.ID).Scan(&ignored)
	if ignored != 1 {
		t.Fatalf("the ignored retry is audited: %d", ignored)
	}

	// An implement dispatch that fails on a claimed task isn't retried by the
	// sweep: it is cancelled.
	var late *store.Dispatch
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		late, err = store.EnqueueDispatch(ctx, tx, "implement-task", "implementer", sonnet, "task", t1.ID, "implement:"+t1.ID.String()+":99")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	h.exec(`UPDATE dispatches SET state = 'failed', error = 'boom', finished_at = now() - interval '1 hour' WHERE id = $1`, late.ID)
	h.srv.Dispatcher.RetrySweep(ctx)
	if after, _ := store.GetDispatch(ctx, h.srv.Store.Pool, late.ID); after.State != "cancelled" {
		t.Fatalf("the sweep must not revive a claimed task's dispatch: %s", after.State)
	}
	if q, _ := h.srv.Store.QueuedDispatches(ctx); len(q) != 0 {
		t.Fatalf("nothing is queued: %+v", q)
	}
}

// TestFeatureAbandonEndsItsClaims is SD-16: abandoning the feature, at any of
// its sites, ends every claim on it and withdraws their questions.
func TestFeatureAbandonEndsItsClaims(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	if _, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant()); err != nil {
		t.Fatal(err)
	}
	var cp *store.Checkpoint
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		cp, err = store.CreateCheckpoint(ctx, tx, "claim-stale", "task", t1.ID, "Is someone still working on it?", map[string]any{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	f, _ := h.srv.featureByPath(ctx, "auth/login")
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.TransitionFeature(ctx, tx, f, lifecycle.FeatAbandon, "sam", map[string]any{"reason": "no longer wanted"})
	}); err != nil {
		t.Fatal(err)
	}
	c := h.latestClaim(t1.ID)
	if c.State != lifecycle.ClaimEnded || c.EndReason != "abandoned" || c.EndedBy != "sam" {
		t.Fatalf("claim = %+v", c)
	}
	if got, _ := store.GetCheckpoint(ctx, h.srv.Store.Pool, cp.ID); got.State != "withdrawn" {
		t.Fatalf("the stale question is withdrawn: %s", got.State)
	}
	if _, err := store.CurrentClaimFor(ctx, h.srv.Store.Pool, "task", t1.ID); err == nil {
		t.Fatal("no claim is left")
	}
}

// TestAbandonedTaskEndsItsClaim is SD-16's other route: the review deadlock's
// Abandon.
func TestAbandonedTaskEndsItsClaim(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.startHeld(2)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	res, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant())
	if err != nil {
		t.Fatal(err)
	}
	writeIn(t, res.WorkingCopy.Path, "a.go", "package main\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, h.srv.ChatClaimant(), "Added a.go."); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.execute(ctx, rules.AbandonTask{TaskID: t1.ID, Actor: "sam", Reason: "we did not converge"}); err != nil {
		t.Fatal(err)
	}
	if c := h.latestClaim(t1.ID); c.State != lifecycle.ClaimEnded || c.EndReason != "abandoned" {
		t.Fatalf("claim = %+v", c)
	}
}

// ---- What the implementer and the reviewer are told ----

// TestPlanImplementShowsTheReviewersCommentsInRoundTwo is FR-2.12: an agent
// reworking a task is told what the code reviewer asked for. Round 1 has no
// such section.
func TestPlanImplementShowsTheReviewersCommentsInRoundTwo(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"v1"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"request_changes","comments":[{"section_ref":"greet.go:1","body":"Add the Greet function.","severity":"major"}],"reasoning":"incomplete"}`,
		provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"v2"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification",
		`{"criteria":[{"id":"AC1","met":true,"evidence":"present"}],"verdict":"approve","reasoning":"met"}`,
		provider.Usage{Input: 10, Output: 5})
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("feature done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	var prompts []string
	for _, r := range h.mock.Requests {
		if len(r.Messages) == 0 || len(r.Messages[0].Blocks) == 0 {
			continue
		}
		if txt := r.Messages[0].Blocks[0].Text; strings.Contains(txt, "# Task to implement") {
			prompts = append(prompts, txt)
		}
	}
	// Each implementer turn repeats its prompt; look at the first and the last.
	if len(prompts) < 2 {
		t.Fatalf("two implementer rounds expected: %d prompts", len(prompts))
	}
	first, last := prompts[0], prompts[len(prompts)-1]
	if strings.Contains(first, "What the code reviewer asked for") {
		t.Error("round 1 has no reviewer comments")
	}
	for _, want := range []string{"# What the code reviewer asked for", "[major] greet.go:1: Add the Greet function."} {
		if !strings.Contains(last, want) {
			t.Errorf("round 2 lacks %q", want)
		}
	}
	if strings.Index(last, "# What the code reviewer asked for") < strings.Index(last, "# Task to implement") {
		t.Error("the comments come after the task")
	}
	// Two agent executions, one per round.
	t1 := h.taskByLocal("T1")
	es := h.execsOf(t1.ID)
	if len(es) != 2 || es[0].Round != 1 || es[1].Round != 2 || es[0].Kind != "agent" {
		t.Fatalf("executions = %+v", es)
	}
}

// TestQueueReviewUsesTheChatModelForAChatWrittenSpec is FR-8.3: a spec whose
// latest writing act is the chat agent's is reviewed with the project's
// reviewer for the chat agent's work; a person's is not.
func TestQueueReviewUsesTheChatModelForAChatWrittenSpec(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  string
		model string
	}{
		{"chat", store.WriterChat, sonnet},
		{"person", store.WriterPerson, haiku},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.setRole("spec-reviewer", sonnet, haiku)
			specPath := h.setupFeatureWithSpec()
			ctx := context.Background()
			doc, err := store.LiveDocumentByPath(ctx, h.srv.Store.Pool, specPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
				return store.RecordWriter(ctx, tx, store.Writer{DocumentID: doc.ID, Act: store.ActRevised,
					Kind: tc.kind, Actor: "someone", Via: "mcp"})
			}); err != nil {
				t.Fatal(err)
			}
			h.holdQueue()
			if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": specPath}); code != 200 {
				t.Fatalf("submit: %d %v", code, out)
			}
			h.eventually("the review is queued", func() bool {
				q, _ := h.srv.Store.QueuedDispatches(ctx)
				for _, d := range q {
					if d.Purpose == "review-spec" {
						if d.Model != tc.model {
							t.Fatalf("a %s-written spec is reviewed with %s, got %s", tc.name, tc.model, d.Model)
						}
						return true
					}
				}
				return false
			})
		})
	}
}

// ---- Units ----

func TestExecutorSentence(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	run, claimID := uuid.New(), uuid.New()
	agent := func(round int, done bool) store.Execution {
		e := store.Execution{Kind: "agent", Actor: "implementer", Model: "claude-sonnet-5", Round: round, Measured: true, DispatchID: &run}
		if done {
			t := now
			e.SubmittedAt = &t
		}
		return e
	}
	chat := func(round int, done bool) store.Execution {
		e := store.Execution{Kind: "chat", Actor: "chat-agent", Round: round, ClaimID: &claimID}
		if done {
			t := now
			e.SubmittedAt = &t
		}
		return e
	}
	person := store.Execution{Kind: "person", Actor: "sam", Round: 1, ClaimID: &claimID}
	task := func(s lifecycle.TaskState) *store.Task { return &store.Task{State: s} }
	cases := []struct {
		name  string
		task  *store.Task
		execs []store.Execution
		claim *store.Claim
		want  string
	}{
		{"nobody", task(lifecycle.TaskReady), nil, nil, "Nobody has started this task yet."},
		{"agent working", task(lifecycle.TaskActive), []store.Execution{agent(1, false)}, nil,
			"Being implemented by the implementer (claude-sonnet-5)."},
		{"chat done", task(lifecycle.TaskDone), []store.Execution{chat(1, true)}, nil, "Implemented by the chat agent."},
		{"person claimed", task(lifecycle.TaskActive), []store.Execution{person},
			&store.Claim{ID: claimID, State: lifecycle.ClaimOpen, ClaimedAt: now.Add(-3 * time.Hour)},
			"Being implemented by sam, who claimed it 3 hours ago."},
		{"released then finished by an agent", task(lifecycle.TaskReview), []store.Execution{chat(1, false), agent(1, true)}, nil,
			"Implemented by the implementer (claude-sonnet-5), from work the chat agent started."},
		{"reworked by the chat agent", task(lifecycle.TaskReview), []store.Execution{agent(1, true), chat(2, true)}, nil,
			"Implemented by the implementer (claude-sonnet-5), then reworked by the chat agent."},
		{"same executor again", task(lifecycle.TaskDone), []store.Execution{chat(1, true), chat(2, true)}, nil, "Implemented by the chat agent."},
	}
	for _, tc := range cases {
		got := executorSentence(tc.task, tc.execs, tc.claim, now)
		if got.Sentence != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got.Sentence, tc.want)
		}
	}
	if got := executorSentence(task(lifecycle.TaskDone), []store.Execution{agent(1, true)}, nil, now); got.RunID != run.String() || !got.Measured || got.Kind != "agent" {
		t.Errorf("fields = %+v", got)
	}
}

func TestWithWorkingCopyIsOneAtATimePerPath(t *testing.T) {
	s := &Server{}
	var inside, overlaps atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.withWorkingCopy("/work/a", func() error {
				if inside.Add(1) > 1 {
					overlaps.Add(1)
				}
				time.Sleep(2 * time.Millisecond)
				inside.Add(-1)
				return nil
			})
		}()
	}
	wg.Wait()
	if overlaps.Load() != 0 {
		t.Fatalf("two hands in one working copy: %d overlaps", overlaps.Load())
	}
	// Different working copies don't wait for one another.
	done := make(chan struct{})
	_ = s.withWorkingCopy("/work/a", func() error {
		go func() {
			_ = s.withWorkingCopy("/work/b", func() error { close(done); return nil })
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("another working copy was held up")
		}
		return nil
	})
}

func TestWorktreeFingerprintChangesWithTheWorkingCopy(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	writeIn(t, dir, "a.txt", "one\n")
	run("add", "-A")
	run("commit", "-qm", "one")

	fp := func() string {
		t.Helper()
		f, err := worktreeFingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	clean := fp()
	if fp() != clean {
		t.Fatal("a fingerprint is stable")
	}
	writeIn(t, dir, "a.txt", "two!\n")
	edited := fp()
	if edited == clean {
		t.Fatal("an edit changes it")
	}
	writeIn(t, dir, "b.txt", "new\n")
	added := fp()
	if added == edited {
		t.Fatal("a new file changes it")
	}
	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "two")
	if committed := fp(); committed == edited || committed == clean {
		t.Fatal("a commit changes it")
	}
	if err := os.Remove(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if fp() == "" {
		t.Fatal("a deletion still fingerprints")
	}
}
