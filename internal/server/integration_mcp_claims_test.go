package server

// The chat agent's task tools over the MCP JSON-RPC endpoint (SPEC-020 FR-3,
// FR-6, FR-9), with the mock provider against real Postgres. The dispatcher
// is held as integration_claims_test.go holds it.

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/store"
)

// asMap and asList read a decoded JSON value.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("want an object, got %T (%v)", v, v)
	}
	return m
}

func asList(t *testing.T, v any) []any {
	t.Helper()
	l, ok := v.([]any)
	if !ok {
		t.Fatalf("want a list, got %T (%v)", v, v)
	}
	return l
}

// toolOK calls a tool that must succeed.
func (h *harness) toolOK(name string, args map[string]any) map[string]any {
	h.t.Helper()
	out, isErr, text := h.callTool(name, args)
	if isErr {
		h.t.Fatalf("%s failed: %s", name, text)
	}
	return out
}

// refusalOf calls a tool that must be refused, and returns the sentence.
func (h *harness) refusalOf(name string, args map[string]any) string {
	h.t.Helper()
	_, isErr, text := h.callTool(name, args)
	if !isErr {
		h.t.Fatalf("%s should have been refused: %s", name, text)
	}
	return text
}

// featureTasks reads get_feature's tasks, by task ID, and in order.
func (h *harness) featureTasks(path string) (map[string]map[string]any, []any) {
	h.t.Helper()
	out := h.toolOK("get_feature", map[string]any{"path": path})
	list := asList(h.t, out["tasks"])
	byID := map[string]map[string]any{}
	for _, e := range list {
		m := asMap(h.t, e)
		byID[m["id"].(string)] = m
	}
	return byID, list
}

func TestClaimTaskToolResultAndRenewal(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanChain(), 2)
	h.startHeld(1)
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")

	// Before anything is claimed: get_feature lists the tasks in plan order.
	byID, list := h.featureTasks("auth/login")
	if len(list) != 2 || asMap(t, list[0])["id"] != t1.PublicID || asMap(t, list[1])["id"] != t2.PublicID {
		t.Fatalf("tasks in plan order: %v", list)
	}
	e1, e2 := byID[t1.PublicID], byID[t2.PublicID]
	if e1["claimable"] != true || e1["why_not"] != nil || e1["url"] != "/ui/t/"+t1.PublicID || e1["state"] != "active" {
		t.Fatalf("T1 entry = %v", e1)
	}
	if ex := asMap(t, e1["executor"]); ex["sentence"] != "Nobody has started this task yet." || ex["kind"] != nil {
		t.Fatalf("T1 executor = %v", ex)
	}
	if e1["claim"] != nil {
		t.Fatalf("no claim yet: %v", e1["claim"])
	}
	if e2["claimable"] != false || !strings.Contains(e2["why_not"].(string), "can't be claimed yet: it waits for T01") ||
		asList(t, e2["depends_on"])[0] != t1.PublicID {
		t.Fatalf("T2 entry = %v", e2)
	}
	// The pending task is refused with the same sentence.
	if got := h.refusalOf("claim_task", map[string]any{"task": t2.PublicID}); got != e2["why_not"] {
		t.Fatalf("claim_task refusal %q should be get_feature's why_not %q", got, e2["why_not"])
	}

	// Claiming.
	res := h.toolOK("claim_task", map[string]any{"task": t1.PublicID})
	task := asMap(t, res["task"])
	if task["id"] != t1.PublicID || task["state"] != "active" {
		t.Fatalf("task = %v", task)
	}
	claim := asMap(t, task["claim"])
	if claim["kind"] != "chat" || claim["who"] != "the chat agent" || claim["state"] != "open" || claim["since"] == nil || claim["last_activity"] == nil {
		t.Fatalf("claim = %v", claim)
	}
	if ex := asMap(t, task["executor"]); ex["kind"] != "chat" || !strings.HasPrefix(ex["sentence"].(string), "Being implemented by the chat agent") || ex["measured"] != false {
		t.Fatalf("executor = %v", ex)
	}
	feat := asMap(t, res["feature"])
	if feat["path"] != "auth/login" || feat["name"] == "" || feat["id"] == "" {
		t.Fatalf("feature = %v", feat)
	}
	wc := asMap(t, res["working_copy"])
	if !filepath.IsAbs(wc["path"].(string)) || wc["branch"] != "subutai/auth/login" ||
		wc["base_commit"] != gitOut(t, wc["path"].(string), "rev-parse", "HEAD") {
		t.Fatalf("working copy = %v", wc)
	}
	contract := asMap(t, res["contract"])
	for _, k := range []string{"spec", "dev_plan", "task"} {
		if contract[k] == nil {
			t.Errorf("contract has no %s", k)
		}
	}
	if !strings.Contains(asMap(t, contract["spec"])["body"].(string), "Login form") ||
		asMap(t, contract["task"])["description"] == "" {
		t.Errorf("contract = %v", contract)
	}
	if res["review_comments"] != nil {
		t.Errorf("a first round has no review comments")
	}
	var rules []string
	for _, r := range asList(t, res["rules"]) {
		rules = append(rules, r.(string))
	}
	if len(rules) < len(claimRuleSentences) {
		t.Fatalf("rules = %v", rules)
	}
	for i, want := range claimRuleSentences {
		if rules[i] != want {
			t.Errorf("rule %d = %q, want %q", i, rules[i], want)
		}
	}
	if !strings.Contains(strings.Join(rules, "\n"), "run_tests") {
		t.Errorf("the project's commands are in the rules: %v", rules)
	}
	if res["expires"] != "If nothing changes in the working copy for 24 hours, the person will be asked whether anyone is still working on this." {
		t.Errorf("expires = %v", res["expires"])
	}
	if res["next"] != "Implement the task in the working copy, run the project's build and tests, then call submit_task with a summary." {
		t.Errorf("next = %v", res["next"])
	}
	if res["renewed"] != nil {
		t.Errorf("a first claim isn't a renewal")
	}

	// Claiming again is the deliberate renewal: same result, activity renewed.
	again := h.toolOK("claim_task", map[string]any{"task": t1.PublicID})
	if again["renewed"] != true || wc["path"] != asMap(t, again["working_copy"])["path"] {
		t.Fatalf("renewal = %v", again)
	}
	if c := h.latestClaim(t1.ID); c.LastActivity != "renewed" {
		t.Fatalf("last activity = %s", c.LastActivity)
	}

	// get_feature now shows the claim.
	byID, _ = h.featureTasks("auth/login")
	if c := asMap(t, byID[t1.PublicID]["claim"]); c["state"] != "open" || c["who"] != "the chat agent" {
		t.Fatalf("claim in get_feature = %v", c)
	}
	if byID[t2.PublicID]["claimable"] != false {
		t.Fatalf("T2 = %v", byID[t2.PublicID])
	}
}

func TestClaimTaskToolRefusals(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	t1 := h.taskByLocal("T1")

	// Not being built yet: the refusal's sentence, as a tool error.
	got := h.refusalOf("claim_task", map[string]any{"task": t1.PublicID})
	if !strings.Contains(got, "isn't being built yet. A person starts building from the feature's page; you can't.") {
		t.Fatalf("refusal = %q", got)
	}
	byID, _ := h.featureTasks("auth/login")
	if e := byID[t1.PublicID]; e["claimable"] != false || e["why_not"] != got {
		t.Fatalf("entry = %v", e)
	}

	// FR-6.3: only tasks, and never verification.
	const only = "Only tasks can be claimed. Verification is always done by Subutai's verifier."
	feat := h.toolOK("get_feature", map[string]any{"path": "auth/login"})
	for _, ref := range []string{feat["id"].(string), "auth/login", "BUG-001"} {
		if got := h.refusalOf("claim_task", map[string]any{"task": ref}); got != only {
			t.Errorf("claim_task %q = %q", ref, got)
		}
	}
	if got := h.refusalOf("claim_task", map[string]any{"task": "FEAT-999-T01"}); !strings.Contains(got, "There is no task FEAT-999-T01.") {
		t.Errorf("unknown task = %q", got)
	}
	if got := h.refusalOf("claim_task", map[string]any{}); !strings.Contains(got, "say which task") {
		t.Errorf("no task = %q", got)
	}
}

func TestSubmitTaskToolResultAndRefusals(t *testing.T) {
	h := newHarness(t)
	h.setRole("code-reviewer", sonnet, haiku)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	t1 := h.taskByLocal("T1")

	// Nothing claimed yet.
	got := h.refusalOf("submit_task", map[string]any{"task": t1.PublicID, "summary": "Did it."})
	if got != "You haven't claimed "+t1.PublicID+", so there is nothing to submit. Claim it first." {
		t.Fatalf("refusal = %q", got)
	}
	res := h.toolOK("claim_task", map[string]any{"task": t1.PublicID})
	dir := asMap(t, res["working_copy"])["path"].(string)

	if got := h.refusalOf("submit_task", map[string]any{"task": t1.PublicID}); !strings.Contains(got, "needs a summary") {
		t.Errorf("no summary = %q", got)
	}
	if got := h.refusalOf("submit_task", map[string]any{"task": t1.PublicID, "summary": "Did it."}); !strings.Contains(got, "Nothing in the working copy has changed since you claimed "+t1.PublicID) {
		t.Errorf("no change = %q", got)
	}

	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"welcome\" }\n")
	sub := h.toolOK("submit_task", map[string]any{"task": t1.PublicID, "summary": "Added greet.go with Greet."})
	head := gitOut(t, dir, "rev-parse", "HEAD")
	if sub["commit"] != head {
		t.Fatalf("commit = %v, head %s", sub["commit"], head)
	}
	task := asMap(t, sub["task"])
	if task["state"] != "review" || asMap(t, task["claim"])["state"] != "submitted" {
		t.Fatalf("task = %v", task)
	}
	review := asMap(t, sub["review"])
	reviews := h.dispatchesOf(t1.ID, "review-code")
	if len(reviews) != 1 || review["run_id"] != reviews[0].ID.String() || review["model"] != sonnet || reviews[0].Model != sonnet {
		t.Fatalf("review = %v, dispatches %+v", review, reviews)
	}
	if sub["next"] != "A code reviewer will review this. get_feature shows its state and any comments. If it comes back, call claim_task to resume it. You can't review or approve it yourself." {
		t.Errorf("next = %v", sub["next"])
	}

	// Out of order.
	if got := h.refusalOf("submit_task", map[string]any{"task": t1.PublicID, "summary": "Again."}); !strings.Contains(got, "was already submitted and is in code review") {
		t.Errorf("second submit = %q", got)
	}
	if got := h.refusalOf("claim_task", map[string]any{"task": t1.PublicID}); !strings.Contains(got, "is in code review. If the reviewer asks for changes, it comes back to whoever implemented it; get_feature shows the comments.") {
		t.Errorf("claim in review = %q", got)
	}
	byID, _ := h.featureTasks("auth/login")
	e := byID[t1.PublicID]
	if e["claimable"] != false || e["state"] != "review" || !strings.Contains(e["why_not"].(string), "is in code review") ||
		asMap(t, e["claim"])["state"] != "submitted" {
		t.Fatalf("entry = %v", e)
	}
	if got := h.refusalOf("submit_task", map[string]any{"task": "auth/login", "summary": "x"}); got != "Only tasks can be claimed. Verification is always done by Subutai's verifier." {
		t.Errorf("submit a feature = %q", got)
	}
}

func TestPersonsClaimIsNotTheChatAgents(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	t1 := h.taskByLocal("T1")
	res, err := h.srv.ClaimWork(context.Background(), t1.PublicID, h.srv.PersonClaimant())
	if err != nil {
		t.Fatal(err)
	}
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	if got := h.refusalOf("submit_task", map[string]any{"task": t1.PublicID, "summary": "x"}); !strings.Contains(got, t1.PublicID+" is claimed by ") || !strings.Contains(got, ", not by you, so you can't submit it.") {
		t.Errorf("submit a person's claim = %q", got)
	}
	got := h.refusalOf("claim_task", map[string]any{"task": t1.PublicID})
	if !strings.Contains(got, "Only a person can release a claim, in the web UI.") {
		t.Errorf("claim a person's task = %q", got)
	}
	byID, _ := h.featureTasks("auth/login")
	e := byID[t1.PublicID]
	if e["claimable"] != false || e["why_not"] == nil || asMap(t, e["claim"])["kind"] != "person" {
		t.Fatalf("entry = %v", e)
	}
	// A release shows who released it.
	if err := h.srv.ReleaseClaim(context.Background(), t1.PublicID, "sam"); err != nil {
		t.Fatal(err)
	}
	byID, _ = h.featureTasks("auth/login")
	if c := asMap(t, byID[t1.PublicID]["claim"]); c["state"] != "ended" || c["released_by"] != "sam" {
		t.Fatalf("released claim = %v", c)
	}
}

// TestChatSendBackAndVerificationOverMCP is FR-3.6 and FR-6.3: after a
// send-back get_feature carries the comments, claim_task resumes with them,
// and the feature is still verified by a dispatched run.
func TestChatSendBackAndVerificationOverMCP(t *testing.T) {
	h := newHarness(t)
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	res := h.toolOK("claim_task", map[string]any{"task": t1.PublicID})
	dir := asMap(t, res["working_copy"])["path"].(string)

	h.mock.RespondOutcome("submit_review",
		`{"verdict":"request_changes","comments":[{"section_ref":"greet.go:3","body":"Greet should say welcome, not hello.","severity":"major"}],"reasoning":"wrong word"}`,
		provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"now right"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification",
		`{"criteria":[{"id":"AC1","met":true,"evidence":"greet.go present"}],"verdict":"approve","reasoning":"met"}`,
		provider.Usage{Input: 10, Output: 5})
	h.releaseQueue()

	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"hello\" }\n")
	h.toolOK("submit_task", map[string]any{"task": t1.PublicID, "summary": "Added greet.go."})
	h.eventually("the claim is sent back", func() bool { return h.latestClaim(t1.ID).State == lifecycle.ClaimReturned })

	byID, _ := h.featureTasks("auth/login")
	e := byID[t1.PublicID]
	if asMap(t, e["claim"])["state"] != "returned" || e["claimable"] != true {
		t.Fatalf("a returned claim is the chat agent's to resume: %v", e)
	}
	rc := asMap(t, e["review_comments"])
	cs := asList(t, rc["comments"])
	if rc["run_id"] == "" || len(cs) != 1 {
		t.Fatalf("review_comments = %v", rc)
	}
	f := asMap(t, cs[0])
	if f["file"] != "greet.go" || f["line"] != float64(3) || f["severity"] != "major" || !strings.Contains(f["text"].(string), "welcome") {
		t.Fatalf("finding = %v", f)
	}

	res2 := h.toolOK("claim_task", map[string]any{"task": t1.PublicID})
	if res2["resumed"] != true {
		t.Fatalf("resumption = %v", res2)
	}
	if rc2 := asMap(t, res2["review_comments"]); rc2["run_id"] != rc["run_id"] || len(asList(t, rc2["comments"])) != 1 {
		t.Fatalf("the comments come back with the claim: %v", rc2)
	}
	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"welcome\" }\n")
	h.toolOK("submit_task", map[string]any{"task": t1.PublicID, "summary": "Said welcome."})

	// FR-6.3 and FR-6.4: approved by the reviewer, verified by a dispatched run.
	h.eventually("feature done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })
	var verifies int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE purpose = 'verify-feature' AND state = 'succeeded'`).Scan(&verifies)
	if verifies != 1 {
		t.Fatalf("a dispatched verifier verified the feature: %d", verifies)
	}
	byID, _ = h.featureTasks("auth/login")
	e = byID[t1.PublicID]
	if e["state"] != "done" || asMap(t, e["claim"])["state"] != "ended" || e["claimable"] != false {
		t.Fatalf("done entry = %v", e)
	}
	if got := h.refusalOf("claim_task", map[string]any{"task": t1.PublicID}); !strings.Contains(got, "is done") {
		t.Errorf("claim a done task = %q", got)
	}
}

// TestChatClaimsABugsTask is SPEC-019 under SPEC-020: a bug's task is
// BUG-nnn-T01, and claiming and submitting it work as a feature's task does.
func TestChatClaimsABugsTask(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.approveDoc(h.setupFeatureWithSpec()) // creates the auth initiative
	rep := h.toolOK("report_bug", map[string]any{"on": "auth", "title": "Greet returns an empty string",
		"steps": "1. Call Greet()", "expected": "A welcome.", "actual": "Nothing."})
	bug := h.bugNamed("Greet returns an empty string")
	if bug == nil || rep["id"] == nil {
		t.Fatalf("the bug was not filed: %v", rep)
	}
	if code, page := h.triage(bug.Feature.ID, "accept", nil); code != 200 {
		t.Fatalf("accept: %d %s", code, truncate(page, 300))
	}
	bugPath := h.bugPath(bug)
	h.scriptApproval()
	h.scriptDocument(bugDevPlan("auth/" + bug.Feature.Slug))
	h.scriptApproval()
	h.scriptEstimate()
	if code, page := h.postValues("/ui/send", url.Values{"owner_kind": {"feature"},
		"owner_id": {bug.Feature.ID.String()}, "feature": {bug.Feature.ID.String()}}); code != 200 {
		t.Fatalf("send the bug: %d\n%s", code, truncate(page, 400))
	}
	h.eventually("the bug ready", func() bool {
		if h.featureState(bugPath) != lifecycle.FeatReady {
			return false
		}
		_, err := store.CurrentEstimate(ctx, h.srv.Store.Pool, "feature", bug.Feature.ID)
		return err == nil
	})
	h.holdQueue()
	if code, page := h.postValues("/ui/feature/start", url.Values{"id": {bug.Feature.ID.String()}}); code != 200 {
		t.Fatalf("start the bug: %d\n%s", code, truncate(page, 400))
	}
	h.eventually("the bug's implementer queued", func() bool {
		var n int
		_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE purpose = 'implement-task' AND state = 'queued'`).Scan(&n)
		return n == 1
	})
	tasks := h.tasks(bugPath)
	if len(tasks) != 1 || !strings.HasPrefix(tasks[0].PublicID, bug.Feature.PublicID+"-T") {
		t.Fatalf("the bug's task = %+v", tasks)
	}
	id := tasks[0].PublicID

	byID, _ := h.featureTasks(bugPath)
	if byID[id] == nil || byID[id]["claimable"] != true {
		t.Fatalf("the bug's tasks in get_feature: %v", byID)
	}
	res := h.toolOK("claim_task", map[string]any{"task": id})
	if asMap(t, res["task"])["id"] != id || asMap(t, res["contract"])["spec"] == nil {
		t.Fatalf("claim = %v", res)
	}
	dir := asMap(t, res["working_copy"])["path"].(string)
	writeIn(t, dir, "greet.go", "package main\n\nfunc Greet() string { return \"welcome\" }\n")
	sub := h.toolOK("submit_task", map[string]any{"task": id, "summary": "Made Greet say welcome."})
	if asMap(t, sub["task"])["state"] != "review" || asMap(t, sub["review"])["model"] == "" {
		t.Fatalf("submit = %v", sub)
	}
	// The bug itself can't be claimed (FR-6.3).
	if got := h.refusalOf("claim_task", map[string]any{"task": bug.Feature.PublicID}); !strings.Contains(got, "Only tasks can be claimed.") {
		t.Errorf("claim a bug = %q", got)
	}
}

// TestWorkATaskSkillSaysWhatTheRulesSay is FR-9.2: every fixed rule that
// claim_task returns appears in the skill subutai init installs, so the two
// can't drift. It also checks FR-9.1's content.
func TestWorkATaskSkillSaysWhatTheRulesSay(t *testing.T) {
	h := newHarness(t)
	data, err := os.ReadFile(filepath.Join(h.root, ".subutai/chat-skills/work-a-task/SKILL.md"))
	if err != nil {
		t.Fatalf("subutai init should install the work-a-task skill: %v", err)
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	skill := norm(string(data))
	if !strings.HasPrefix(string(data), "---\ndescription: ") {
		t.Errorf("the skill needs front-matter with a description")
	}
	for _, rule := range claimRuleSentences {
		if !strings.Contains(skill, norm(rule)) {
			t.Errorf("the skill lacks the rule %q", rule)
		}
	}
	for _, want := range []string{"get_feature", "claim_task", "submit_task", "Don't commit", "web UI", "independent reviewer", "Your tokens aren't measured"} {
		if !strings.Contains(skill, want) {
			t.Errorf("the skill should mention %q", want)
		}
	}
}
