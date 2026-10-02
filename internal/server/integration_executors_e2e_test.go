package server

// The end-to-end proof of SPEC-020 (FR-10.1 and FR-6.4): a chat agent
// implements a task over MCP, and Subutai reviews and verifies it exactly as it
// does an agent's. The mock provider is scripted per role, so the reviewer, the
// implementer and the verifier answer deterministically even when their
// dispatches run at once; the dispatcher is held, as integration_claims_test.go
// holds it, where a step needs it. FR-10.3 (a bug's task) is covered by
// TestChatClaimsABugsTask in integration_mcp_claims_test.go.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/store"
)

// roleRouter is a provider that hands each request to the mock scripted for
// its role: the implementer, the verifier, or the code reviewer on a given
// model. The scripts are therefore independent of the order in which
// concurrent dispatches reach the provider.
type roleRouter struct {
	mu     sync.Mutex
	mocks  map[string]*provider.Mock
	misses []string
}

func newRoleRouter() *roleRouter { return &roleRouter{mocks: map[string]*provider.Mock{}} }

// script returns the mock for a route, making it on first use.
func (r *roleRouter) script(route string) *provider.Mock {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.mocks[route] == nil {
		r.mocks[route] = &provider.Mock{}
	}
	return r.mocks[route]
}

func (r *roleRouter) missed() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.misses...)
}

func routeOf(req provider.Request) string {
	for _, tl := range req.Tools {
		switch tl.Name {
		case "submit_implementation":
			return "implement"
		case "submit_verification":
			return "verify"
		case "submit_review":
			return "review:" + req.Model
		}
	}
	return "other"
}

func (r *roleRouter) Complete(ctx context.Context, req provider.Request) (*provider.Response, error) {
	route := routeOf(req)
	r.mu.Lock()
	m := r.mocks[route]
	if m == nil {
		r.misses = append(r.misses, route)
	}
	r.mu.Unlock()
	if m == nil {
		return nil, fmt.Errorf("no script for %s", route)
	}
	return m.Complete(ctx, req)
}

// assertNoClaimedTaskApprovedWithoutReview is FR-6.4: every approval of a task
// that was ever claimed is backed by a succeeded code-review dispatch that
// started before it (the reviewer's approval), or by a person's answer to a
// checkpoint on the task (the round cap or an escalation). Nothing marks a
// claimed task done without one of them.
func (h *harness) assertNoClaimedTaskApprovedWithoutReview() {
	h.t.Helper()
	rows, err := h.srv.Store.Pool.Query(context.Background(), `
		SELECT a.ref_id,
		  (SELECT count(*) FROM dispatches d
		     WHERE d.purpose = 'review-code' AND d.ref_type = 'task' AND d.ref_id = a.ref_id
		       AND d.state = 'succeeded' AND d.started_at <= a.occurred_at) AS reviews,
		  (SELECT count(*) FROM checkpoints c
		     WHERE c.ref_type = 'task' AND c.ref_id = a.ref_id AND c.state = 'answered') AS answers
		FROM audit_events a
		WHERE a.kind = 'task.transition' AND a.payload->>'event' = 'approve'
		  AND EXISTS (SELECT 1 FROM work_claims w WHERE w.ref_type = 'task' AND w.ref_id = a.ref_id)`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id uuid.UUID
		var reviews, answers int
		if err := rows.Scan(&id, &reviews, &answers); err != nil {
			h.t.Fatal(err)
		}
		seen++
		if reviews == 0 && answers == 0 {
			h.t.Errorf("claimed task %s was approved with no succeeded review-code dispatch and no checkpoint answer", id)
		}
	}
	if seen == 0 {
		h.t.Error("expected at least one approval of a claimed task to check")
	}
}

// TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents is FR-10.1, the steps
// in order, with FR-6.4's audit check at the end.
func TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents(t *testing.T) {
	h := newHarness(t)
	h.setRole("code-reviewer", sonnet, haiku) // the usual reviewer is the cheaper model
	ctx := context.Background()

	// A feature with an estimate and a two-task plan, T01 and T02 independent;
	// and, beside it, a measured feature with an estimate that is done.
	h.claimsFeature(devPlanTwoIndependent(), 2)
	h.exec(`UPDATE features SET description = $1 WHERE slug = 'login'`,
		"Users sign in with an email address and password on the login form.")
	h.createFeature("auth", "signup", "Sign up form", "Users register with an email address and password on the sign up form.")
	for _, ref := range []string{"auth/login", "auth/signup"} {
		if code, out := h.call("POST", "/api/estimate/set", map[string]any{"ref": ref, "tokens": 5000}); code != 201 {
			t.Fatalf("estimate %s: %d %v", ref, code, out)
		}
	}
	h.exec(`UPDATE features SET state = 'done' WHERE slug = 'signup'`)

	// The roles answer from their own scripts, whatever order they are asked in.
	rr := newRoleRouter()
	h.srv.Dispatcher.Providers = func(string) (provider.Provider, error) { return rr, nil }
	usage := provider.Usage{Input: 10, Output: 5}
	rr.script("implement").
		RespondToolUse("write_file", `{"path":"b.go","content":"package main\n\nfunc Bee() string { return \"bzz\" }\n"}`, usage).
		RespondOutcome("submit_implementation", `{"summary":"Added b.go."}`, usage)
	rr.script("review:"+sonnet). // the chat agent's work: send back, then approve
					RespondOutcome("submit_review",
			`{"verdict":"request_changes","comments":[{"section_ref":"a.go:3","body":"Say hello from A.","severity":"major"}],"reasoning":"incomplete"}`, usage).
		RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"now right"}`, usage)
	rr.script("review:"+haiku).
		RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"fine"}`, usage)
	rr.script("verify").
		RespondOutcome("submit_verification",
			`{"criteria":[{"id":"AC1","met":true,"evidence":"a.go and b.go present"}],"verdict":"approve","reasoning":"met"}`, usage)

	// Step 1: a person presses Start building, with the dispatcher held.
	h.startHeld(2)
	t1, t2 := h.taskByLocal("T1"), h.taskByLocal("T2")
	d1 := h.dispatchesOf(t1.ID, "implement-task")[0]

	// Step 2: the chat agent claims T01. Its queued dispatch is cancelled; T02's
	// waits behind the claim, with the reason.
	res := h.toolOK("claim_task", map[string]any{"task": t1.PublicID})
	dir := asMap(t, res["working_copy"])["path"].(string)
	if got, _ := store.GetDispatch(ctx, h.srv.Store.Pool, d1.ID); got.State != "cancelled" {
		t.Fatalf("T01's implement dispatch = %s, want cancelled", got.State)
	}
	h.releaseQueue()
	h.eventually("T02's dispatch waits behind the claim", func() bool {
		d := h.dispatchesOf(t2.ID, "implement-task")[0]
		return d.State == "queued" && d.QueueReason != nil && *d.QueueReason == store.QueueReasonClaimOpen
	})
	if es := h.execsOf(t2.ID); len(es) != 0 {
		t.Fatalf("no agent runs while the claim is open: %+v", es)
	}
	if rr.script("implement").Remaining() != 2 {
		t.Fatal("the implementer was asked while the claim was open")
	}

	// Step 3: it writes a file and submits; T02's implementer runs while T01 is
	// reviewed.
	writeIn(t, dir, "a.go", "package main\n\nfunc A() string { return \"a\" }\n")
	sub := h.toolOK("submit_task", map[string]any{"task": t1.PublicID, "summary": "Added a.go."})
	if asMap(t, sub["task"])["state"] != "review" {
		t.Fatalf("submit = %v", sub)
	}

	// Step 4: the reviewer asks for changes. T01's claim is returned, and no
	// implementer is queued for it. Wait for T02 to finish too, so resuming
	// isn't refused for a running agent (FR-6.5).
	h.eventually("T01 sent back and T02 done", func() bool {
		return h.latestClaim(t1.ID).State == lifecycle.ClaimReturned && h.taskState(t2.ID) == lifecycle.TaskDone
	})
	if h.taskState(t1.ID) != lifecycle.TaskActive {
		t.Fatalf("T01 = %s", h.taskState(t1.ID))
	}
	for _, d := range h.dispatchesOf(t1.ID, "implement-task") {
		if d.State != "cancelled" {
			t.Fatalf("no implementer is queued for the chat agent's task: %+v", d)
		}
	}
	byID, _ := h.featureTasks("auth/login")
	e1 := byID[t1.PublicID]
	cs := asList(t, asMap(t, e1["review_comments"])["comments"])
	if asMap(t, e1["claim"])["state"] != "returned" || len(cs) != 1 ||
		!strings.Contains(asMap(t, cs[0])["text"].(string), "Say hello from A.") {
		t.Fatalf("get_feature shows the comments: %v", e1)
	}
	back := h.toolOK("claim_task", map[string]any{"task": t1.PublicID})
	rc := asList(t, asMap(t, back["review_comments"])["comments"])
	if back["resumed"] != true || len(rc) != 1 || asMap(t, rc[0])["file"] != "a.go" {
		t.Fatalf("the resumed claim returns the comments: %v", back)
	}

	// Step 5: it edits and submits again; the reviewer approves. The review of
	// the chat agent's work uses the chat reviewer model, the priciest
	// configured; T02's, the usual one.
	writeIn(t, dir, "a.go", "package main\n\nfunc A() string { return \"hello from A\" }\n")
	h.toolOK("submit_task", map[string]any{"task": t1.PublicID, "summary": "Said hello."})

	// Step 6: the verifier runs and approves, and the feature merges.
	h.eventually("feature done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })
	if reviews := h.dispatchesOf(t1.ID, "review-code"); len(reviews) != 2 || reviews[0].Model != sonnet || reviews[1].Model != sonnet {
		t.Fatalf("T01's reviews use the chat reviewer model: %+v", reviews)
	}
	if reviews := h.dispatchesOf(t2.ID, "review-code"); len(reviews) != 1 || reviews[0].Model != haiku {
		t.Fatalf("T02's review uses the usual model: %+v", reviews)
	}
	var verifies int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE purpose = 'verify-feature' AND state = 'succeeded'`).Scan(&verifies)
	if verifies != 1 {
		t.Fatalf("a dispatched verifier verified the feature: %d", verifies)
	}
	for _, f := range []string{"a.go", "b.go"} {
		if _, err := os.Stat(filepath.Join(h.root, f)); err != nil {
			t.Fatalf("%s is merged: %v", f, err)
		}
	}
	if c := h.latestClaim(t1.ID); c.State != lifecycle.ClaimEnded || c.EndReason != "done" {
		t.Fatalf("claim = %+v", c)
	}
	if m := rr.missed(); len(m) != 0 {
		t.Fatalf("a request had no script: %v", m)
	}

	// Step 7: the task page, get_feature and the timeline say who implemented
	// each task.
	mustContain(t, "T01 page", h.taskPageHTML(t1.ID.String()), "Implemented by the chat agent.")
	page2 := h.taskPageHTML(t2.ID.String())
	mustContain(t, "T02 page", page2, "Implemented by the implementer")
	mustNotContain(t, "T02 page", page2, "Implemented by the chat agent.")
	byID, _ = h.featureTasks("auth/login")
	if s := asMap(t, byID[t1.PublicID]["executor"])["sentence"]; s != "Implemented by the chat agent." {
		t.Fatalf("T01 executor = %v", s)
	}
	ex2 := asMap(t, byID[t2.PublicID]["executor"])
	if s := ex2["sentence"].(string); !strings.HasPrefix(s, "Implemented by the implementer") || ex2["kind"] != "agent" || ex2["measured"] != true {
		t.Fatalf("T02 executor = %v", ex2)
	}
	feat := h.featurePageHTML()
	for _, want := range []string{"Claimed by the chat agent: First", "Submitted by the chat agent: First", "Code review sent back"} {
		mustContain(t, "timeline", feat, want)
	}
	if n := strings.Count(feat, "Submitted by the chat agent: First"); n < 2 {
		t.Errorf("the timeline shows both submissions (at least two mentions): %d", n)
	}
	mustNotContain(t, "timeline", feat, "Claimed by the chat agent: Second")

	// The actuals: T01, the feature and the initiative are unmeasured; T02 and
	// the other feature are measured. The unmeasured feature is out of the
	// calibration corpus, and a measured feature with an estimate is in it.
	login, err := h.srv.featureByPath(ctx, "auth/login")
	if err != nil {
		t.Fatal(err)
	}
	signup, err := h.srv.featureByPath(ctx, "auth/signup")
	if err != nil {
		t.Fatal(err)
	}
	var initID uuid.UUID
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT initiative_id FROM features WHERE id = $1`, login.ID).Scan(&initID); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		kind string
		id   uuid.UUID
		want bool
	}{{"task", t1.ID, true}, {"task", t2.ID, false}, {"feature", login.ID, true},
		{"feature", signup.ID, false}, {"initiative", initID, true}} {
		if got, err := store.Unmeasured(ctx, h.srv.Store.Pool, c.kind, c.id); err != nil || got != c.want {
			t.Errorf("Unmeasured(%s) = %v, %v; want %v", c.kind, got, err, c.want)
		}
	}
	corpus, err := store.RetrieveCorpus(ctx, h.srv.Store.Pool, "sign in with an email address and password", 10)
	if err != nil {
		t.Fatal(err)
	}
	var hasLogin, hasSignup bool
	for _, r := range corpus {
		hasLogin = hasLogin || r.RefID == login.ID
		hasSignup = hasSignup || r.RefID == signup.ID
	}
	if hasLogin || !hasSignup {
		t.Fatalf("corpus has the unmeasured feature %v and the measured one %v: %+v", hasLogin, hasSignup, corpus)
	}

	// FR-6.4: nothing marked the claimed task done without a review.
	h.assertNoClaimedTaskApprovedWithoutReview()
}
