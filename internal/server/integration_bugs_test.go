package server

// The bugs suite (SPEC-019), with the mock provider against real Postgres and
// the phase-1 harness. The first test is the milestone's done-when: an
// agent-filed bug is triaged by a person, sent, its report reviewed as a
// spec, planned, built, reviewed, and verified as no longer reproducing.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// ---- Fixtures ----

// bugDevPlan is a one-task plan for a bug.
func bugDevPlan(owner string) string {
	return `---
title: Fix — dev plan
type: dev_plan
owner: ` + owner + `
---

# Fix — dev plan

## Approach

Make Greet return a non-empty welcome, and pin it with a test.

## Tasks

| id | title | depends_on | description |
|----|-------|------------|-------------|
| T1 | Fix the greeting | | Make Greet return "welcome" and add a regression test |
`
}

// bugNamed returns the bug with a title, or nil.
func (h *harness) bugNamed(title string) *store.Bug {
	h.t.Helper()
	var id uuid.UUID
	err := h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT id FROM features WHERE kind = 'bug' AND name = $1`, title).Scan(&id)
	if err != nil {
		return nil
	}
	b, err := store.GetBug(context.Background(), h.srv.Store.Pool, id)
	if err != nil {
		h.t.Fatal(err)
	}
	return b
}

// uiReportBug reports a bug through the web UI's dialog on an owner's page.
func (h *harness) uiReportBug(ownerType string, ownerID uuid.UUID, title string) (int, string) {
	h.t.Helper()
	return h.postValues("/ui/bugs", url.Values{
		"owner_type": {ownerType}, "owner_id": {ownerID.String()},
		"title": {title}, "steps": {"Open the page\nPress Greet"},
		"expected": {"It says welcome."}, "actual": {"It says nothing."},
	})
}

// triage posts a triage decision from the queue.
func (h *harness) triage(bugID uuid.UUID, decision string, extra map[string]string) (int, string) {
	h.t.Helper()
	v := url.Values{"bug_id": {bugID.String()}, "decision": {decision}}
	for k, x := range extra {
		v.Set(k, x)
	}
	return h.postValues("/ui/bugs/triage", v)
}

// reportOnInitiative reports a bug in the UI on initiative slug and returns it.
func (h *harness) reportOnInitiative(slug, title string) *store.Bug {
	h.t.Helper()
	if code, page := h.uiReportBug("initiative", h.initiativeID(slug), title); code != 200 ||
		!strings.Contains(page, "waits in the triage queue") {
		h.t.Fatalf("report %q: %d\n%s", title, code, truncate(page, 600))
	}
	b := h.bugNamed(title)
	if b == nil {
		h.t.Fatalf("bug %q not found", title)
	}
	return b
}

func (h *harness) bugPath(b *store.Bug) string {
	h.t.Helper()
	p, err := h.srv.featurePath(context.Background(), &b.Feature)
	if err != nil {
		h.t.Fatal(err)
	}
	return p
}

// scriptBuild scripts one task's implementation, its code review and the
// feature's verification, in the order they run.
func (h *harness) scriptBuild(file, content, review, verification string) {
	h.t.Helper()
	raw, _ := json.Marshal(map[string]string{"path": file, "content": content})
	h.mock.RespondToolUse("write_file", string(raw), tiny)
	h.mock.RespondOutcome("submit_implementation", `{"summary":"done","files_changed":["`+file+`"]}`, tiny)
	h.mock.RespondOutcome("submit_review", review, tiny)
	h.mock.RespondOutcome("submit_verification", verification, tiny)
}

// ---- The done-when (SPEC-019 §1, FR-4 acceptance) ----

// TestAgentFiledBugIsTriagedFixedAndVerified is the milestone's done-when. A
// mock implementer, building auth/login, files a bug with report_bug and
// carries on; the code reviewer approves with two minor findings. When login
// merges, the minors become one bug. A person accepts the agent's bug in the
// queue and sends it: its report is submitted and reviewed as its spec by the
// spec reviewer, with no spec written; the plan is written, reviewed and
// decomposed; it is estimated and ready. Start building runs the implementer,
// the code reviewer and the verifier — who is given the report and approves
// "The defect no longer reproduces" with evidence — and the bug merges.
func TestAgentFiledBugIsTriagedFixedAndVerified(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	h.approveDoc(h.addDevPlan(devPlanOneTask))
	h.eventually("login ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	// The implementer files a bug mid-task, then does its own task.
	h.mock.RespondToolUse("report_bug", `{"title":"Greet returns an empty string",
		"steps":"1. Call Greet()\n2. Look at what it returns",
		"expected":"It returns a welcome.","actual":"It returns an empty string.",
		"notes":"Seen in greet.go while adding the login helper."}`, tiny)
	h.scriptBuild("greet.go", "package main\n\nfunc Greet() string { return \"\" }\n",
		`{"verdict":"approve","reasoning":"in scope","comments":[
			{"body":"Name the return value","severity":"minor","section_ref":"greet.go"},
			{"body":"Add a doc comment","severity":"minor"}]}`,
		`{"criteria":[{"id":"AC1","met":true,"evidence":"greet.go present"}],"verdict":"approve","reasoning":"met"}`)
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start login: %d %v", code, out)
	}
	h.eventually("login done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	bug := h.bugNamed("Greet returns an empty string")
	if bug == nil {
		t.Fatal("the implementer's report_bug filed no bug")
	}
	if bug.Triage != store.TriageReported || bug.ReporterKind != store.ReporterAgent ||
		bug.ReportedBy != "implementer" || bug.ReportedDispatchID == nil {
		t.Errorf("the agent's bug should be reported by the implementer's run; got %+v", bug)
	}
	login := h.featureID("auth/login")
	if bug.OriginFeatureID == nil || *bug.OriginFeatureID != login || !strings.HasPrefix(bug.Feature.PublicID, "BUG-") {
		t.Errorf("the bug should hang off login with a BUG- ID; got %+v", bug)
	}
	h.eventually("the minor findings filed at the merge", func() bool {
		return h.bugNamed("Minor review findings on FEAT-001: Login form") != nil
	})
	minors := h.bugNamed("Minor review findings on FEAT-001: Login form")
	report := h.currentDoc(h.bugPath(minors), "bug_report")
	body := h.readFile(report.Path)
	for _, want := range []string{"Name the return value (at greet.go)", "Add a doc comment", "FEAT-001-T01"} {
		mustContain(t, "the minor findings' report", body, want)
	}

	// Nothing ran for either bug: they wait for triage.
	h.quiet()
	for _, b := range []*store.Bug{bug, minors} {
		if n := len(h.runsFor("feature", b.Feature.ID)); n != 0 {
			t.Errorf("%s: nothing runs before triage; %d runs", b.Feature.PublicID, n)
		}
	}
	_, queue := h.getUI("/ui/triage")
	mustContain(t, "triage queue", queue, "Greet returns an empty string")
	mustContain(t, "triage queue", queue, "Reported by the implementer (claude-sonnet-5) while working on FEAT-001-T01.")
	_, badge := h.getUI("/ui/frag/triage-badge")
	if strings.TrimSpace(badge) != "2" {
		t.Errorf("the triage badge should count two bugs; got %q", badge)
	}

	// A person accepts it in the queue, and sends it.
	if code, page := h.triage(bug.Feature.ID, "accept", nil); code != 200 || !strings.Contains(page, "was accepted") {
		t.Fatalf("accept: %d\n%s", code, truncate(page, 400))
	}
	bugPath := h.bugPath(bug)
	_, screen := h.getUI("/ui/send?feature=" + bug.Feature.ID.String())
	mustContain(t, "send screen", screen, "The bug report is the specification, so this step is skipped.")
	mustContain(t, "send screen", screen, "Review the bug report as a specification")

	h.scriptApproval()                                       // the report, reviewed as the spec
	h.scriptDocument(bugDevPlan("auth/" + bug.Feature.Slug)) // the plan
	h.scriptApproval()                                       // the plan's review
	h.scriptEstimate()
	if code, page := h.postValues("/ui/send", url.Values{"owner_kind": {"feature"},
		"owner_id": {bug.Feature.ID.String()}, "feature": {bug.Feature.ID.String()}}); code != 200 ||
		!strings.Contains(page, "Sent to development") {
		t.Fatalf("send the bug: %d\n%s", code, truncate(page, 600))
	}
	h.eventually("the bug ready with an estimate", func() bool {
		if h.featureState(bugPath) != lifecycle.FeatReady {
			return false
		}
		_, err := store.CurrentEstimate(ctx, h.srv.Store.Pool, "feature", bug.Feature.ID)
		return err == nil
	})
	if n := h.countPurpose(bugPath, "write-spec"); n != 0 {
		t.Errorf("the report is the spec, so nothing writes one; %d write-spec runs", n)
	}
	rep := h.currentDoc(bugPath, "bug_report")
	if rep.State != lifecycle.DocApproved {
		t.Fatalf("the report should be approved as the spec; it is %s", rep.State)
	}
	if runs := h.runsFor("document", rep.ID); len(runs) != 1 || runs[0].Purpose != "review-bug_report" || runs[0].Role != "spec-reviewer" {
		t.Errorf("the spec reviewer should review the report once; got %+v", runs)
	}
	if spec := h.currentDoc(bugPath, "spec"); spec != nil {
		t.Errorf("a bug has no separate spec; found %s", spec.Path)
	}

	// Start building: implement, review with no minors, verify against the report.
	h.scriptBuild("greet.go", "package main\n\nfunc Greet() string { return \"welcome\" }\n",
		`{"verdict":"approve","reasoning":"fixes the defect"}`,
		`{"criteria":[{"id":"The defect no longer reproduces","met":true,
			"evidence":"Calling Greet() now returns \"welcome\", following the steps to reproduce"}],
			"verdict":"approve","reasoning":"the defect no longer reproduces"}`)
	if code, page := h.postValues("/ui/feature/start", url.Values{"id": {bug.Feature.ID.String()}}); code != 200 {
		t.Fatalf("start building the bug: %d\n%s", code, truncate(page, 400))
	}
	h.eventually("the bug done", func() bool { return h.featureState(bugPath) == lifecycle.FeatDone })
	if got := h.readFile("greet.go"); !strings.Contains(got, `"welcome"`) {
		t.Errorf("the fix should be merged; greet.go is %q", got)
	}
	if ts := h.tasks(bugPath); len(ts) != 1 || !strings.HasPrefix(ts[0].PublicID, bug.Feature.PublicID+"-T") {
		t.Errorf("the bug's task should be numbered from its ID; got %+v", ts)
	}

	// The verifier was given the report, with its built-in criterion.
	verified := false
	for _, req := range h.mock.Requests {
		for _, tool := range req.Tools {
			if tool.Name == "submit_verification" && len(req.Messages) > 0 &&
				strings.Contains(fmt.Sprint(req.Messages[0]), "The defect no longer reproduces") &&
				strings.Contains(fmt.Sprint(req.Messages[0]), "Greet returns an empty string") {
				verified = true
			}
		}
	}
	if !verified {
		t.Error("the verifier's prompt should carry the bug's report and its criterion")
	}

	// The pipeline was the feature pipeline, less the spec writing (NFR-4).
	var purposes []string
	rows, _ := h.srv.Store.Pool.Query(ctx, `SELECT DISTINCT d.purpose FROM dispatches d
		LEFT JOIN tasks t ON d.ref_type = 'task' AND t.id = d.ref_id
		WHERE d.ref_id = $1 OR d.ref_id = $2 OR t.feature_id = $1 ORDER BY 1`, bug.Feature.ID, rep.ID)
	for rows.Next() {
		var p string
		_ = rows.Scan(&p)
		purposes = append(purposes, p)
	}
	rows.Close()
	plan := h.currentDoc(bugPath, "dev_plan")
	for _, r := range h.runsFor("document", plan.ID) {
		purposes = append(purposes, r.Purpose)
	}
	got := strings.Join(purposes, ",")
	want := "estimate,implement-task,review-bug_report,review-code,verify-feature,write-dev-plan,review-dev_plan"
	if got != want {
		t.Errorf("the bug's runs should be the feature pipeline's, minus write-spec:\n got %s\nwant %s", got, want)
	}
	// The second build had no minors, so no third bug.
	var bugs int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM features WHERE kind = 'bug'`).Scan(&bugs)
	if bugs != 2 {
		t.Errorf("an approval with no minor findings files nothing; %d bugs", bugs)
	}

	// The timeline tells the story.
	_, page := h.getUI("/ui/f/" + bugPath)
	for _, want := range []string{"Reported by an agent at work", "Accepted in triage", "Sent to development",
		"Report approved", "Plan approved", "Done"} {
		mustContain(t, "the bug's timeline", page, want)
	}
	_, health := h.getUI("/ui/review-health")
	mustContain(t, "review health", health, "reviewing the bug report")
}

// ---- Reporting (FR-1, FR-3) ----

// TestReportingABugFromEachSurface is FR-1.3, FR-1.4 and FR-3.1 to FR-3.2: a
// person reports on an initiative and on a feature in the web UI, and the chat
// agent reports over MCP. Each gets the next BUG- number, a bug-nnn slug, and
// a registered, committed draft report at the default home, as the bug's main
// document; nothing is dispatched; a feature made afterwards still gets the
// next FEAT- number.
func TestReportingABugFromEachSurface(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.setupFeatureWithSpec()

	a := h.reportOnInitiative("auth", "The sign-in page is blank")
	if code, page := h.uiReportBug("feature", h.featureID("auth/login"), "Login forgets the email"); code != 200 {
		t.Fatalf("report on a feature: %d\n%s", code, truncate(page, 400))
	}
	b := h.bugNamed("Login forgets the email")
	out := h.mustTool("report_bug", map[string]any{"on": "FEAT-001", "title": "Lockout never lifts",
		"steps": "Fail six times\nWait twenty minutes\nTry again", "expected": "You can sign in.",
		"actual": "Still locked out.", "notes": "log: {{.User}} TODO trace"})
	c := h.bugNamed("Lockout never lifts")

	for i, bug := range []*store.Bug{a, b, c} {
		id := fmt.Sprintf("BUG-%03d", i+1)
		if bug.Feature.PublicID != id || bug.Feature.Slug != strings.ToLower(id) || bug.Triage != store.TriageReported {
			t.Errorf("bug %d: got %s, slug %s, triage %s", i+1, bug.Feature.PublicID, bug.Feature.Slug, bug.Triage)
		}
		rep := h.currentDoc(h.bugPath(bug), "bug_report")
		if rep == nil || rep.State != lifecycle.DocDraft || rep.PublicID != id+"-bug-report" || !strings.HasPrefix(rep.Path, "docs/work/INIT-001-auth/"+id+"-bug-report") {
			t.Errorf("bug %d's report: %+v", i+1, rep)
			continue
		}
		if status := h.gitOut("status", "--porcelain", "--", rep.Path); strings.TrimSpace(status) != "" {
			t.Errorf("the report should be committed; git status says %q", status)
		}
		primary, err := store.PrimaryDocForOwner(ctx, h.srv.Store.Pool, "feature", &bug.Feature.ID)
		if err != nil || primary.ID != rep.ID {
			t.Errorf("the report should be the bug's main document")
		}
		if n := h.auditKinds("bug.reported", bug.Feature.ID); n != 1 {
			t.Errorf("bug %d: %d bug.reported rows", i+1, n)
		}
		body := h.readFile(rep.Path)
		mustContain(t, "report", body, "The defect no longer reproduces")
	}
	if a.OriginFeatureID != nil || b.OriginFeatureID == nil || *b.OriginFeatureID != h.featureID("auth/login") {
		t.Errorf("origins: on the initiative none, on the feature login; got %v %v", a.OriginFeatureID, b.OriginFeatureID)
	}
	if a.ReporterKind != store.ReporterPerson || c.ReporterKind != store.ReporterChat || out["id"] != "BUG-003" {
		t.Errorf("reporters: %s, %s; tool said %v", a.ReporterKind, c.ReporterKind, out["id"])
	}
	// The chat agent's literal text stays literal, and still passes validation.
	cBody := h.readFile(h.currentDoc(h.bugPath(c), "bug_report").Path)
	mustContain(t, "fenced notes", cBody, "```text\nlog: {{.User}} TODO trace\n```")
	mustContain(t, "numbered steps", cBody, "1. Fail six times\n2. Wait twenty minutes\n3. Try again")
	if r, err := h.srv.ValidateDoc(ctx, h.currentDoc(h.bugPath(c), "bug_report").Path); err != nil || !r.Valid {
		t.Errorf("the chat agent's report should validate; %v %+v", err, r)
	}

	h.quiet()
	if n := len(h.mock.Requests); n != 0 {
		t.Errorf("reporting dispatches nothing; %d provider calls", n)
	}
	// Numbering is per prefix.
	h.createFeature("auth", "signup", "Sign-up", "Create an account.")
	if f, _ := h.srv.featureByPath(ctx, "auth/signup"); f.PublicID != "FEAT-002" {
		t.Errorf("a feature after bugs is FEAT-002; got %s", f.PublicID)
	}
	// A feature can't take a bug's slug.
	if code, _ := h.call("POST", "/api/features", map[string]string{"initiative_path": "auth", "slug": "bug-009", "name": "Odd"}); code == 201 {
		t.Error("a feature slugged like a bug should be refused")
	}
	// A report missing its parts is refused before anything is minted.
	h.toolRefused("report_bug", map[string]any{"on": "auth", "title": "No steps"}, "steps to reproduce")
	var n int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM features WHERE kind = 'bug'`).Scan(&n)
	if n != 3 {
		t.Errorf("a refused report creates nothing; %d bugs", n)
	}

	// The database keeps kind and ID together, and a relay needs a quote.
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE features SET kind = 'bug' WHERE public_id = 'FEAT-002'`); err == nil {
		t.Error("a bug with a FEAT- ID should be refused by the database")
	}
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE bugs SET triage = 'accepted', decided_by = 'x',
		decided_via = 'mcp', decided_at = now() WHERE feature_id = $1`, a.Feature.ID); err == nil {
		t.Error("a relayed decision without a quote should be refused by the database")
	}

	// Pages: the initiative and the feature list their open bugs; the rail and
	// the initiative's feature list leave bugs out.
	_, ipage := h.getUI("/ui/i/auth")
	mustContain(t, "initiative bugs", ipage, "The sign-in page is blank")
	mustContain(t, "initiative menu", ipage, "Report a bug…")
	_, fpage := h.getUI("/ui/f/auth/login")
	mustContain(t, "feature bugs", fpage, "Login forgets the email")
	_, bpage := h.getUI("/ui/id/BUG-001")
	mustContain(t, "bug page", bpage, "Waiting for triage")
	mustContain(t, "bug page", bpage, `data-entity="bug"`)
	feats, _ := store.FeaturesForInitiative(ctx, h.srv.Store.Pool, h.initiativeID("auth"))
	for _, f := range feats {
		if f.IsBug() {
			t.Errorf("an initiative's features leave bugs out; found %s", f.PublicID)
		}
	}
}

// TestAgentReportsAreCappedAndDeduplicated is SD-10 and FR-3.3: a run may
// file three reports; the fourth, and a title matching a reported bug on the
// same origin, come back to the agent as tool errors; its run finishes
// normally.
func TestAgentReportsAreCappedAndDeduplicated(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	h.approveDoc(h.addDevPlan(devPlanOneTask))
	h.eventually("login ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	report := func(title string) {
		raw, _ := json.Marshal(map[string]string{"title": title, "steps": "Do it", "expected": "Fine", "actual": "Broken"})
		h.mock.RespondToolUse("report_bug", string(raw), tiny)
	}
	report("One")
	report("one ") // the same title, ignoring case and space
	report("Two")
	report("Three")
	report("Four")
	h.scriptBuild("greet.go", "package main\n", `{"verdict":"approve","reasoning":"ok"}`,
		`{"criteria":[{"id":"AC1","met":true,"evidence":"present"}],"verdict":"approve","reasoning":"met"}`)
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("login done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })
	for _, title := range []string{"One", "Two", "Three"} {
		if h.bugNamed(title) == nil {
			t.Errorf("%q should be filed", title)
		}
	}
	if h.bugNamed("Four") != nil {
		t.Error("a fourth report in one run should be refused")
	}
	var dup, capped int
	_ = h.srv.Store.Pool.QueryRow(context.Background(), `SELECT
		count(*) FILTER (WHERE status = 'error' AND tool = 'report_bug'),
		count(*) FILTER (WHERE status = 'ok' AND tool = 'report_bug') FROM tool_calls`).Scan(&dup, &capped)
	if dup != 2 || capped != 3 {
		t.Errorf("two refusals and three filings expected; got %d and %d", dup, capped)
	}
}

// ---- Triage (FR-2) ----

// TestTriageAcceptRejectAndDuplicate is FR-2: each decision from the queue or
// the bug's page, each refusal with its sentence, the abandonment that rejects
// and duplicates bring, the queue's order, and the counts.
func TestTriageAcceptRejectAndDuplicate(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.setupFeatureWithSpec()
	first := h.reportOnInitiative("auth", "First")
	second := h.reportOnInitiative("auth", "Second")
	third := h.reportOnInitiative("auth", "Third")
	fourth := h.reportOnInitiative("auth", "Fourth")

	_, queue := h.getUI("/ui/triage")
	if strings.Index(queue, "First") > strings.Index(queue, "Second") {
		t.Error("the queue lists the oldest first")
	}
	_, line := h.getUI("/ui/frag/triage-line")
	mustContain(t, "inbox line", line, "4 bug reports are waiting for triage.")
	_, inbox := h.getUI("/ui/inbox")
	mustContain(t, "inbox", inbox, "/ui/frag/triage-line")

	// Refusals.
	if _, page := h.triage(first.Feature.ID, "reject", nil); !strings.Contains(page, "Say why it") {
		t.Error("a rejection without a reason should be refused")
	}
	if _, page := h.triage(first.Feature.ID, "duplicate", map[string]string{"duplicate_of": first.Feature.PublicID}); !strings.Contains(page, "duplicate of itself") {
		t.Error("a bug can't duplicate itself")
	}
	if _, page := h.triage(first.Feature.ID, "duplicate", map[string]string{"duplicate_of": "FEAT-001"}); !strings.Contains(page, "isn") {
		t.Error("a feature isn't a bug to duplicate")
	}

	// Accept from the bug's page; reject and duplicate from the queue.
	code, page := h.triage(first.Feature.ID, "accept", map[string]string{"from": "bug"})
	if code != 200 || !strings.Contains(page, "BUG-001 was accepted") {
		t.Fatalf("accept from the page: %d\n%s", code, truncate(page, 400))
	}
	mustContain(t, "the page after accepting", page, "Send to development")
	h.triage(second.Feature.ID, "reject", map[string]string{"reason": "Works as intended."})
	h.triage(third.Feature.ID, "duplicate", map[string]string{"duplicate_of": "BUG-002"})
	if _, page := h.triage(fourth.Feature.ID, "duplicate", map[string]string{"duplicate_of": "BUG-003"}); !strings.Contains(page, "is itself a duplicate, of BUG-002") {
		t.Errorf("a duplicate of a duplicate should name the original; got\n%s", truncate(page, 600))
	}
	if _, page := h.triage(first.Feature.ID, "reject", map[string]string{"reason": "x"}); !strings.Contains(page, "was already accepted") {
		t.Error("a decided bug has nothing to triage")
	}

	for _, c := range []struct {
		b      *store.Bug
		triage string
		state  lifecycle.FeatureState
	}{{first, "accepted", lifecycle.FeatIdea}, {second, "rejected", lifecycle.FeatAbandoned},
		{third, "duplicate", lifecycle.FeatAbandoned}, {fourth, "reported", lifecycle.FeatIdea}} {
		got, _ := store.GetBug(ctx, h.srv.Store.Pool, c.b.Feature.ID)
		if got.Triage != c.triage || got.Feature.State != c.state {
			t.Errorf("%s: triage %s state %s, want %s %s", got.Feature.PublicID, got.Triage, got.Feature.State, c.triage, c.state)
		}
	}
	if n := h.auditKinds("bug.triaged", second.Feature.ID); n != 1 {
		t.Errorf("one bug.triaged row expected; got %d", n)
	}
	_, queue = h.getUI("/ui/triage")
	mustContain(t, "queue after", queue, "Fourth")
	mustContain(t, "recently triaged", queue, "Rejected by")
	mustContain(t, "recently triaged", queue, "It repeats BUG-002.")
	if strings.Contains(queue[:strings.Index(queue, "Recently triaged")], ">First<") {
		t.Error("an accepted bug leaves the queue")
	}
	_, badge := h.getUI("/ui/frag/triage-badge")
	if strings.TrimSpace(badge) != "1" {
		t.Errorf("one bug left in the queue; badge %q", badge)
	}
	// A rejected bug's page says who and why; its timeline has no second moment.
	_, bpage := h.getUI("/ui/f/" + h.bugPath(second))
	mustContain(t, "rejected page", bpage, "Works as intended.")
	tl, err := h.srv.featureTimeline(ctx, second.Feature.ID)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, m := range tl.Moments {
		labels = append(labels, m.Label)
	}
	if got := strings.Join(labels, " · "); got != "Reported · Rejected in triage" {
		t.Errorf("a rejection is one moment, after the report; got %q", got)
	}

	// A reported bug is closed by triage, not abandoned round it.
	if code, _ := h.call("POST", "/api/features/abandon", map[string]string{"path": h.bugPath(fourth), "reason": "meh"}); code == 200 {
		t.Error("abandoning a reported bug should be refused")
	}
}

// ---- The send and the report's review (FR-4) ----

// TestSendingABugNeedsAcceptanceAndSubmitsItsReport is FR-4.1, FR-4.3 and
// FR-4.4: Send refuses a reported bug, a rejected one, one whose origin is
// being built, and one whose report fails validation, each with its sentence;
// a report can't be submitted before acceptance; Send submits the draft
// report and queues its review, with no spec written.
func TestSendingABugNeedsAcceptanceAndSubmitsItsReport(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.onlyWriteSpec()
	h.setupFeatureWithSpec()
	bug := h.reportOnInitiative("auth", "Broken thing")
	rejected := h.reportOnInitiative("auth", "Not a bug")
	h.triage(rejected.Feature.ID, "reject", map[string]string{"reason": "Intended."})

	sendErr := func(b *store.Bug) string {
		err := h.srv.SendToDevelopment(ctx, b.Feature.ID, false, "sam")
		if err == nil {
			return ""
		}
		return err.Error()
	}
	if e := sendErr(bug); !strings.Contains(e, "hasn't been triaged yet") {
		t.Errorf("a reported bug can't be sent; got %q", e)
	}
	if e := sendErr(rejected); !strings.Contains(e, "rejected in triage") {
		t.Errorf("a rejected bug can't be sent; got %q", e)
	}
	rep := h.currentDoc(h.bugPath(bug), "bug_report")
	if _, isErr, text := h.callTool("submit_for_review", map[string]any{"document": rep.PublicID}); !isErr ||
		!strings.Contains(text, "accepted in triage") {
		t.Errorf("a reported bug's report can't be submitted; got %v %q", isErr, text)
	}
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": rep.Path}); code == 200 {
		t.Errorf("the API can't submit it either; got %d %v", code, out)
	}

	h.triage(bug.Feature.ID, "accept", nil)
	// A report broken by hand stops the send, and nothing is sent.
	good := h.readFile(rep.Path)
	h.writeCommitted(rep.Path, strings.Replace(good, "The defect no longer reproduces", "It works", 1))
	if e := sendErr(bug); !strings.Contains(e, "doesn't pass validation") || !strings.Contains(e, "The defect no longer reproduces") {
		t.Errorf("an invalid report stops the send; got %q", e)
	}
	if _, err := store.GetFeatureSend(ctx, h.srv.Store.Pool, bug.Feature.ID); err != store.ErrNotFound {
		t.Error("an invalid report leaves the bug unsent")
	}
	h.writeCommitted(rep.Path, good)

	h.scriptApproval()
	h.send(h.bugPath(bug), false)
	h.eventually("the report approved", h.docStateIs(h.bugPath(bug), "bug_report", lifecycle.DocApproved))
	h.quiet()
	if n := h.countPurpose(h.bugPath(bug), "write-spec"); n != 0 {
		t.Errorf("no spec is written for a bug; %d", n)
	}
	// With write-dev-plan unassigned (onlyWriteSpec), the plan waits.
	if h.featureState(h.bugPath(bug)) != lifecycle.FeatIdea {
		t.Error("the bug waits for its plan")
	}

	// A bug reported on a feature being built waits for it to merge.
	h.approveDoc("docs/specs/login.md")
	h.approveDoc(h.addDevPlan(devPlanOneTask))
	h.eventually("login ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })
	h.uiReportBug("feature", h.featureID("auth/login"), "Found while building")
	onLogin := h.bugNamed("Found while building")
	h.triage(onLogin.Feature.ID, "accept", nil)
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE features SET state = 'active' WHERE public_id = 'FEAT-001'`); err != nil {
		t.Fatal(err)
	}
	if e := sendErr(onLogin); !strings.Contains(e, "It can be sent once FEAT-001 has merged") {
		t.Errorf("a bug on a feature being built waits for the merge; got %q", e)
	}
}

// TestSentBackReportIsRevisedByTheSpecAuthor is SD-7 and FR-4.5: the spec
// reviewer sends back a sent bug's report; the spec author revises it in
// place, told it is a bug report and not to invent reproduction steps; the
// resubmission is reviewed and approved. An issue raised by a person on the
// report is must-address, as on a spec.
func TestSentBackReportIsRevisedByTheSpecAuthor(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	h.setupFeatureWithSpec()
	bug := h.reportOnInitiative("auth", "Vague thing")
	h.triage(bug.Feature.ID, "accept", nil)
	path := h.bugPath(bug)
	rep := h.currentDoc(path, "bug_report")

	h.mock.RespondOutcome("submit_review", `{"verdict":"request_changes","reasoning":"not reproducible",
		"comments":[{"body":"Say which page","severity":"major","section_ref":"Steps to reproduce"}]}`, tiny)
	revised := strings.Replace(h.readFile(rep.Path), "1. Open the page", "1. Open the sign-in page", 1)
	h.scriptDocument(revised)
	h.scriptApproval()
	h.send(path, false)
	h.eventually("the revised report approved", h.docStateIs(path, "bug_report", lifecycle.DocApproved))

	final := h.currentDoc(path, "bug_report")
	if final.ID != rep.ID {
		t.Error("the report is revised in place, not replaced")
	}
	mustContain(t, "revised report", h.readFile(final.Path), "Open the sign-in page")
	if n := h.countPurpose(path, "write-spec"); n != 1 {
		t.Errorf("one revision by the spec author; got %d", n)
	}
	var prompt string
	for _, req := range h.mock.Requests {
		for _, tool := range req.Tools {
			if tool.Name == "submit_document" && len(req.Messages) > 0 {
				prompt = fmt.Sprint(req.Messages[0])
			}
		}
	}
	for _, want := range []string{"This is a bug report", "Never invent reproduction steps", "Say which page",
		"must have a list item that starts \"The defect no longer reproduces\"", "The bug to write for"} {
		mustContain(t, "the reviser's prompt", prompt, want)
	}
	mustNotContain(t, "the reviser's prompt", prompt, "No approved design is attached")
	if spec := h.currentDoc(path, "spec"); spec != nil {
		t.Error("the revision must not create a spec beside the report")
	}
}

// TestReportIsHeldLikeASpec is SD-8 and FR-4.5: the hold, and agent review
// switched off, apply to a bug's report as to a spec.
func TestReportIsHeldLikeASpec(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.onlyWriteSpec()
	h.setupFeatureWithSpec()
	held := h.reportOnInitiative("auth", "Held one")
	h.triage(held.Feature.ID, "accept", nil)
	h.scriptApproval()
	h.send(h.bugPath(held), true)
	rep := h.currentDoc(h.bugPath(held), "bug_report")
	h.eventually("the report held after an agent approval", func() bool {
		hold, err := store.GetDocumentHold(ctx, h.srv.Store.Pool, rep.ID)
		return err == nil && hold.DispatchID != nil
	})
	_, page := h.getUI("/ui/d/" + rep.Path)
	mustContain(t, "held report", page, "This bug report is waiting for you")
	h.mustTool("relay_release_hold", map[string]any{"path": rep.Path, "quote": "Let the reviewer decide on the held bug."})
	h.eventually("released", h.docStateIs(h.bugPath(held), "bug_report", lifecycle.DocApproved))

	h.editConfig("  agent: true", "  agent: false")
	off := h.reportOnInitiative("auth", "Agent off")
	h.triage(off.Feature.ID, "accept", nil)
	h.send(h.bugPath(off), false)
	rep2 := h.currentDoc(h.bugPath(off), "bug_report")
	h.eventually("the report waits for a person", func() bool {
		_, err := store.GetDocumentHold(ctx, h.srv.Store.Pool, rep2.ID)
		return err == nil
	})
	h.quiet()
	if n, _ := store.CountDispatchesForRef(ctx, h.srv.Store.Pool, "document", rep2.ID, "review-bug_report"); n != 0 {
		t.Errorf("with agent review off, no agent reviews the report; %d", n)
	}
	// A person's issue on it, then their approval, settle it.
	if _, err := h.srv.RaiseIssue(ctx, rep2.ID, "Say which browser.", "", h.srv.uiAct()); err != nil {
		t.Fatalf("an issue on a report: %v", err)
	}
	if got := h.currentDoc(h.bugPath(off), "bug_report"); got.State != lifecycle.DocDraft {
		t.Errorf("an issue on a sent bug's report in review sends it back; state %s", got.State)
	}
}

// TestWithdrawingABugCancelsItsReportReview is R19-12: a bug's send is
// withdrawn while its report's review waits in the queue; the review is
// cancelled and the report is a draft again.
func TestWithdrawingABugCancelsItsReportReview(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.setupFeatureWithSpec()
	bug := h.reportOnInitiative("auth", "Withdraw me")
	h.triage(bug.Feature.ID, "accept", nil)
	if _, err := h.srv.Store.Pool.Exec(ctx, `INSERT INTO dispatches
		(id, state, purpose, role, model, ref_type, ref_id, idempotency_key, cost_usd, finished_at)
		VALUES (gen_random_uuid(), 'succeeded', 'estimate', 'estimator', 'claude-sonnet-5', 'project', gen_random_uuid(), 'spent', 100, now())`); err != nil {
		t.Fatal(err)
	}
	h.send(h.bugPath(bug), false)
	rep := h.currentDoc(h.bugPath(bug), "bug_report")
	h.eventually("the review queued", func() bool {
		n, _ := store.CountDispatchesForRef(ctx, h.srv.Store.Pool, "document", rep.ID, "review-bug_report")
		return n == 1
	})
	if err := h.srv.WithdrawSend(ctx, bug.Feature.ID, "sam"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if got := h.currentDoc(h.bugPath(bug), "bug_report"); got.State != lifecycle.DocDraft {
		t.Errorf("the report goes back to draft; state %s", got.State)
	}
	var state string
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT state FROM dispatches WHERE ref_id = $1`, rep.ID).Scan(&state)
	if state != "cancelled" {
		t.Errorf("the queued review is cancelled; state %s", state)
	}
}

// ---- Ownership and milestones (R19-4, FR-5.5) ----

// TestBugDocumentsAndMilestones: a bug has no spec and a feature no bug
// report; a milestone refuses a reported bug and takes an accepted one,
// counting it as one item; an initiative in a milestone doesn't bring in its
// bugs.
func TestBugDocumentsAndMilestones(t *testing.T) {
	h := newHarness(t)
	h.setupFeatureWithSpec()
	bug := h.reportOnInitiative("auth", "Milestone bug")

	h.writeCommitted("docs/notes/spec-for-bug.md", validSpec)
	h.toolRefused("attach_document", map[string]any{"path": "docs/notes/spec-for-bug.md", "doc_type": "spec",
		"owner_type": "feature", "owner_path": bug.Feature.PublicID}, "its report is its specification")
	rep := h.currentDoc(h.bugPath(bug), "bug_report")
	copyPath := "docs/notes/report-copy.md"
	h.writeCommitted(copyPath, h.readFile(rep.Path))
	h.toolRefused("attach_document", map[string]any{"path": copyPath, "doc_type": "bug_report",
		"owner_type": "feature", "owner_path": "auth/login"}, "A bug report belongs to a bug")

	h.mustTool("create_milestone", map[string]any{"name": "Fixes"})
	h.toolRefused("add_milestone_member", map[string]any{"milestone": "Fixes", "member_type": "bug", "member": bug.Feature.PublicID},
		"hasn't been accepted in triage")
	h.mustTool("relay_triage", map[string]any{"bug": bug.Feature.PublicID, "decision": "accept", "quote": "Yes, fix that one."})
	h.mustTool("add_milestone_member", map[string]any{"milestone": "Fixes", "member_type": "bug", "member": bug.Feature.PublicID})
	m := h.milestoneNamed("Fixes")
	if done, total := h.milestoneItems(m.ID); total != 1 || done != 0 {
		t.Errorf("an accepted bug is one item; got %d of %d", done, total)
	}
	h.mustTool("create_milestone", map[string]any{"name": "Auth"})
	h.mustTool("add_milestone_member", map[string]any{"milestone": "Auth", "member_type": "initiative", "member": "auth"})
	if _, total := h.milestoneItems(h.milestoneNamed("Auth").ID); total != 1 {
		t.Errorf("an initiative brings in its one feature and not its bug; got %d items", total)
	}
}

// ---- MCP (FR-6) ----

// TestBugMCPTools is FR-6: report_bug, list_bugs, get_bug and relay_triage;
// a relay leaves the audit row with via mcp and the quote; refusals.
func TestBugMCPTools(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.setupFeatureWithSpec()
	h.mustTool("report_bug", map[string]any{"on": "auth", "title": "A", "steps": "x", "expected": "y", "actual": "z"})
	h.mustTool("report_bug", map[string]any{"on": "auth/login", "title": "B", "steps": "x", "expected": "y", "actual": "z"})

	list := h.mustTool("list_bugs", map[string]any{"triage": "reported"})
	if bugs, _ := list["bugs"].([]any); len(bugs) != 2 {
		t.Errorf("two reported bugs; got %v", list)
	}
	onLogin := h.mustTool("list_bugs", map[string]any{"on": "FEAT-001"})
	if bugs, _ := onLogin["bugs"].([]any); len(bugs) != 1 {
		t.Errorf("one bug on login; got %v", onLogin)
	}

	h.toolRefused("relay_triage", map[string]any{"bug": "BUG-001", "decision": "accept"}, "quoted")
	h.toolRefused("relay_triage", map[string]any{"bug": "BUG-001", "decision": "reject", "quote": "No."}, "Say why")
	h.mustTool("relay_triage", map[string]any{"bug": "BUG-001", "decision": "reject",
		"reason": "Not worth it.", "quote": "Reject A, it isn't worth it."})
	got := h.mustTool("get_bug", map[string]any{"bug": "BUG-001"})
	dec, _ := got["decision"].(map[string]any)
	if got["triage"] != "rejected" || dec["quote"] != "Reject A, it isn't worth it." || dec["via"] != "mcp" {
		t.Errorf("get_bug should show the relayed decision; got %v", got)
	}
	var n int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'bug.triaged'
		AND payload->>'via' = 'mcp' AND payload->>'quote' = 'Reject A, it isn''t worth it.'`).Scan(&n)
	if n != 1 {
		t.Errorf("the relay is audited via mcp with the quote; %d rows", n)
	}
	h.mustTool("relay_triage", map[string]any{"bug": "BUG-002", "decision": "duplicate",
		"duplicate_of": "BUG-001", "quote": "B is the same as A."})
	if b := h.bugNamed("B"); b.Triage != store.TriageDuplicate || b.DecidedVia != "mcp" {
		t.Errorf("B should be a relayed duplicate; got %+v", b)
	}
	_, page := h.getUI("/ui/f/" + h.bugPath(h.bugNamed("A")))
	mustContain(t, "relayed decision on the page", page, "a person, relayed by the chat agent")
	mustContain(t, "relayed quote", page, "Reject A, it isn")
}

// ---- Units ----

func TestFillBugReport(t *testing.T) {
	tmpl, err := os.ReadFile(filepath.Join("..", "starter", "pack", "templates", "bug_report", "template.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := fillBugReport(string(tmpl), BugReport{Title: `A "quoted" title`, Summary: "Broken.",
		Steps: "one\ntwo", Expected: "e {{actual}}", Actual: "## not a heading"}, "auth/bug-001")
	for _, want := range []string{`title: "A \"quoted\" title"`, "owner: \"auth/bug-001\"", "1. one\n2. two",
		"e {{actual}}", "```text\n## not a heading\n```"} {
		if !strings.Contains(body, want) {
			t.Errorf("want %q in\n%s", want, body)
		}
	}
	for _, gone := range []string{"## Where it was found", "## Notes", "{{notes}}"} {
		if strings.Contains(body, gone) {
			t.Errorf("an empty optional section is left out; found %q", gone)
		}
	}
}
