package server

// The Send to development suite (SPEC-011), with the mock provider against
// real Postgres, reusing the phase-1 harness and the authoring helpers.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/store"
)

// legacyDesignReviewer turns the test project into one made before SPEC-011:
// the design manifest names a design reviewer, and the role and its skill
// exist. Such a project keeps its reviewer (SD-8).
func (h *harness) legacyDesignReviewer() {
	h.t.Helper()
	comp := filepath.Join(h.root, ".subutai")
	write := func(rel, body string) {
		p := filepath.Join(comp, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			h.t.Fatal(err)
		}
	}
	write("roles/design-reviewer.yaml", "model: claude-sonnet-5\nskill: review-design\nidentity: \"Senior engineer reading a design\"\ntools: [read_file, list_files]\nlimits:\n  turn_cap: 12\n")
	write("skills/review-design/SKILL.md", "---\ndescription: Comment on a design\n---\n\n# Reviewing a design\n\nComment; there is no verdict.\n")
	manifest := filepath.Join(comp, "templates/design/manifest.yaml")
	data, err := os.ReadFile(manifest)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append([]byte("reviewer_role: design-reviewer\n"), data...), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func mustNotContain(t *testing.T, what, body, unwanted string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Errorf("%s: did not expect %q in rendered output", what, unwanted)
	}
}

// ---- Fixtures for the chain ----

var tiny = provider.Usage{Input: 100, Output: 50}

// scriptDocument scripts one author dispatch: submit_document with a body.
func (h *harness) scriptDocument(body string) {
	h.t.Helper()
	raw, err := json.Marshal(map[string]string{"body": body, "reasoning": "written from the design"})
	if err != nil {
		h.t.Fatal(err)
	}
	h.mock.RespondOutcome("submit_document", string(raw), tiny)
}

func (h *harness) scriptApproval() {
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"faithful to the design"}`, tiny)
}

func (h *harness) scriptEstimate() {
	h.mock.RespondOutcome("submit_estimate", `{"tokens":42000,"rationale":"one helper, one task"}`, tiny)
}

// designedInitiative makes initiative pf with an approved design and the
// named, described features.
func (h *harness) designedInitiative(features ...string) {
	h.t.Helper()
	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": "pf", "name": "Platform"}); code != 201 {
		h.t.Fatalf("create initiative: %d %v", code, out)
	}
	for _, f := range features {
		h.createFeature("pf", f, "The "+f+" behaviour", "Everything the "+f+" behaviour must do.")
	}
	h.registerDoc("docs/design/pf.md", "design", "initiative", "pf", cascadeDesignV1)
	h.approveDesignAsHuman("docs/design/pf.md")
}

func (h *harness) currentDoc(featurePath, docType string) *store.Document {
	h.t.Helper()
	f, err := h.srv.featureByPath(context.Background(), featurePath)
	if err != nil {
		h.t.Fatal(err)
	}
	d, err := store.CurrentDocForOwner(context.Background(), h.srv.Store.Pool, docType, "feature", f.ID)
	if err != nil {
		return nil
	}
	return d
}

func (h *harness) docStateIs(featurePath, docType string, want lifecycle.DocumentState) func() bool {
	return func() bool {
		d := h.currentDoc(featurePath, docType)
		return d != nil && d.State == want
	}
}

// ---- The load-bearing claim ----

// TestSendRunsTheChainAndStopsAtStartBuilding is SPEC-011's goal and FR-1,
// FR-2, FR-4.4 and FR-7: approving a design starts nothing; Send, from the
// initiative's send screen with one feature ticked, writes and reviews a
// spec, writes and reviews a plan, decomposes and estimates it, and stops at
// Start building. The unticked feature is untouched.
func TestSendRunsTheChainAndStopsAtStartBuilding(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("alpha", "beta")
	h.quiet()
	if n := len(h.mock.Requests); n != 0 {
		t.Fatalf("approving a design must start nothing; %d provider calls were made", n)
	}

	in, err := h.srv.Store.InitiativeBySlugPath(ctx, []string{"pf"})
	if err != nil {
		t.Fatal(err)
	}
	alpha, _ := h.srv.featureByPath(ctx, "pf/alpha")

	// The send screen renders both features, the steps and the slots.
	code, screen := h.getUI("/ui/send?initiative=" + in.ID.String())
	if code != 200 {
		t.Fatalf("send screen: %d\n%s", code, truncate(screen, 400))
	}
	for _, want := range []string{"The alpha behaviour", "The beta behaviour", "Write the specification",
		"spec-author", "Who reviews the specification", "agent slots are free", "no forecast yet"} {
		mustContain(t, "send screen", screen, want)
	}

	h.scriptDocument(cascadeSpec("alpha"))
	h.scriptApproval()
	h.scriptDocument(cascadeDevPlan("alpha"))
	h.scriptApproval()
	h.scriptEstimate()
	code, page := h.postForm("/ui/send", map[string]string{
		"owner_kind": "initiative", "owner_id": in.ID.String(), "feature": alpha.ID.String()})
	if code != 200 {
		t.Fatalf("send: %d\n%s", code, truncate(page, 400))
	}
	mustContain(t, "send notice", page, "Sent to development: The alpha behaviour")

	h.eventually("alpha ready with an estimate", func() bool {
		if h.featureState("pf/alpha") != lifecycle.FeatReady {
			return false
		}
		est, err := store.CurrentEstimate(ctx, h.srv.Store.Pool, "feature", alpha.ID)
		return err == nil && est.Tokens == 42000
	})
	if got := len(h.tasks("pf/alpha")); got != 1 {
		t.Errorf("the plan should decompose into one task; got %d", got)
	}
	h.quiet()
	var implement int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE purpose = 'implement-task'`).Scan(&implement)
	if implement != 0 {
		t.Errorf("the chain must stop at Start building; %d implement dispatches ran", implement)
	}
	if got := h.countPurpose("pf/beta", "write-spec"); got != 0 {
		t.Errorf("the unticked feature must not be specced; got %d", got)
	}
	_, featPage := h.getUI("/ui/f/pf/alpha")
	mustContain(t, "stops at Start building", featPage, "Start building</button>")
	mustContain(t, "sent mark", featPage, "Sent to development")
}

// ---- The hold (FR-5) ----

// TestHeldSpecWaitsThenTheReviewerDecides: with the hold on for one send,
// the reviewer's approval leaves the spec in review, held; nothing more is
// written; a relayed "let the reviewer decide", with the person's words,
// makes the approval stand and the plan follows.
func TestHeldSpecWaitsThenTheReviewerDecides(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	h.designedInitiative("alpha")

	h.scriptDocument(cascadeSpec("alpha"))
	h.scriptApproval()
	h.send("pf/alpha", true)
	var spec *store.Document
	h.eventually("spec held after an agent approval", func() bool {
		spec = h.currentDoc("pf/alpha", "spec")
		if spec == nil || spec.State != lifecycle.DocReviewing {
			return false
		}
		hold, err := store.GetDocumentHold(context.Background(), h.srv.Store.Pool, spec.ID)
		return err == nil && hold.DispatchID != nil
	})
	h.quiet()
	if got := h.docState(spec.Path); got != lifecycle.DocReviewing {
		t.Fatalf("a held spec must wait for a person; state = %s", got)
	}
	_, page := h.getUI("/ui/d/" + spec.Path)
	mustContain(t, "held panel", page, "This specification is waiting for you")
	mustContain(t, "release offered", page, "Let the reviewer decide")

	// A relay without the person's words is refused.
	if _, isErr, text := h.callTool("relay_release_hold", map[string]any{"path": spec.Path}); !isErr ||
		!strings.Contains(text, "quote") {
		t.Fatalf("a relay without a quote must be refused; got %v %q", isErr, text)
	}
	out, isErr, text := h.callTool("relay_release_hold", map[string]any{
		"path": spec.Path, "quote": "Fine, let the reviewer decide on alpha."})
	if isErr {
		t.Fatalf("release: %s", text)
	}
	if out["state"] != "approved" {
		t.Errorf("released spec should be approved; got %v", out["state"])
	}
	var quoted int
	_ = h.srv.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events
		WHERE kind = 'document.hold_released' AND payload->>'via' = 'mcp'
		AND payload->>'quote' = 'Fine, let the reviewer decide on alpha.'`).Scan(&quoted)
	if quoted != 1 {
		t.Errorf("the release should be audited via mcp with the quote; got %d rows", quoted)
	}
	// The approval is the reviewer's, not the chat agent's.
	var actor string
	_ = h.srv.Store.Pool.QueryRow(context.Background(), `SELECT actor FROM audit_events
		WHERE kind = 'document.transition' AND ref_id = $1 AND payload->>'to' = 'approved'`, spec.ID).Scan(&actor)
	if actor != "spec-reviewer" {
		t.Errorf("the approval should stand as the reviewer's; actor = %q", actor)
	}
}

// TestAgentReviewOffHoldsEverySpec is FR-5.1 and NFR-4: with agent review
// switched off, a submitted spec queues no review and waits; "let the
// reviewer decide" is refused; a requested one-off review runs and is held
// again; only a person's approval moves it.
func TestAgentReviewOffHoldsEverySpec(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	h.editConfig("  agent: true", "  agent: false")
	h.designedInitiative("alpha")

	h.scriptDocument(cascadeSpec("alpha"))
	h.send("pf/alpha", false) // the send screen can't turn the hold off; the server forces it
	h.eventually("spec held with no review", h.docStateIs("pf/alpha", "spec", lifecycle.DocReviewing))
	h.quiet()
	spec := h.currentDoc("pf/alpha", "spec")
	// The orchestrator writes the hold just after the spec enters review, and
	// no dispatch marks it, so wait for the hold itself.
	h.eventually("the spec's hold", func() bool {
		_, err := store.GetDocumentHold(context.Background(), h.srv.Store.Pool, spec.ID)
		return err == nil
	})
	n, _ := store.CountDispatchesForRef(context.Background(), h.srv.Store.Pool, "document", spec.ID, "review-spec")
	if n != 0 {
		t.Fatalf("agent review is off, so no review may be queued; got %d", n)
	}
	send, _ := store.GetFeatureSend(context.Background(), h.srv.Store.Pool, *spec.OwnerID)
	if send == nil || !send.Hold {
		t.Errorf("with agent review off the send's hold is forced on")
	}
	if err := h.srv.ReleaseHold(context.Background(), spec.ID, relayAct{Actor: "sam", Via: "ui"}); err == nil ||
		!strings.Contains(err.Error(), "switched off") {
		t.Fatalf("let the reviewer decide must be refused with agent review off; got %v", err)
	}

	// A one-off review, requested by a person, runs and is held again.
	h.scriptApproval()
	code, page := h.postForm("/ui/document/request-review", map[string]string{"doc_id": spec.ID.String()})
	if code != 200 {
		t.Fatalf("request review: %d", code)
	}
	mustContain(t, "review queued", page, "A fresh agent review is queued")
	h.eventually("one-off review held", func() bool {
		hold, err := store.GetDocumentHold(context.Background(), h.srv.Store.Pool, spec.ID)
		return err == nil && hold.DispatchID != nil
	})
	if got := h.docState(spec.Path); got != lifecycle.DocReviewing {
		t.Fatalf("the one-off approval must be held; state = %s", got)
	}
	if err := h.srv.ReleaseHold(context.Background(), spec.ID, relayAct{Actor: "sam", Via: "ui"}); err == nil {
		t.Fatal("still no releasing to a reviewer that is switched off")
	}

	// A person approves directly.
	if code, page := h.postForm("/ui/document/approve", map[string]string{"doc_id": spec.ID.String()}); code != 200 ||
		!strings.Contains(page, "You approved the document") {
		t.Fatalf("direct approval: %d\n%s", code, truncate(page, 300))
	}
	if got := h.docState(spec.Path); got != lifecycle.DocApproved {
		t.Fatalf("direct approval should approve; state = %s", got)
	}
}

// ---- Human issues and the revise loop (FR-3, FR-6) ----

// pauseAuthor unassigns write-spec so a test can learn an issue's id before
// scripting the author and reviewer who will answer it; resumeAuthor restores
// it and lets the heartbeat sweep pick the waiting draft up (FR-2.4).
func (h *harness) pauseAuthor() {
	h.editConfig("  write-spec: spec-author", "  # write-spec: spec-author")
}
func (h *harness) resumeAuthor() {
	h.editConfig("  # write-spec: spec-author", "  write-spec: spec-author")
}

func (h *harness) openIssues(path string) []store.Comment {
	h.t.Helper()
	d, err := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, path)
	if err != nil {
		h.t.Fatal(err)
	}
	issues, err := store.OpenIssues(context.Background(), h.srv.Store.Pool, d.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return issues
}

// TestIssueOnASentFeaturesSpecGoesBackToItsAuthor is FR-6.2 (reviewing,
// sent), FR-3.2 and FR-6.3: a relayed issue on a held spec sends it back;
// the author's revision prompt carries the issue; a review that approves
// without answering it is rejected inside its turn; one that answers it
// approves, and the issue is recorded as answered.
func TestIssueOnASentFeaturesSpecGoesBackToItsAuthor(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	h.designedInitiative("alpha")
	ctx := context.Background()

	h.scriptDocument(cascadeSpec("alpha"))
	h.scriptApproval()
	h.send("pf/alpha", true) // held, so the spec waits in review
	h.eventually("spec held", func() bool {
		d := h.currentDoc("pf/alpha", "spec")
		if d == nil {
			return false
		}
		_, err := store.GetDocumentHold(ctx, h.srv.Store.Pool, d.ID)
		return err == nil
	})
	h.quiet()
	spec := h.currentDoc("pf/alpha", "spec")

	h.pauseAuthor()
	_, isErr, text := h.callTool("relay_issue", map[string]any{
		"path": spec.Path, "issue": "The spec never says which timezone times are in.",
		"quote": "tell it the spec forgets timezones"})
	if isErr {
		t.Fatalf("relay_issue: %s", text)
	}
	if got := h.docState(spec.Path); got != lifecycle.DocDraft {
		t.Fatalf("an issue on a sent feature's spec in review sends it back; state = %s", got)
	}
	if _, err := store.GetDocumentHold(ctx, h.srv.Store.Pool, spec.ID); err == nil {
		t.Error("a return to draft clears the hold")
	}
	issues := h.openIssues(spec.Path)
	if len(issues) != 1 || issues[0].Via != "mcp" || issues[0].Quote != "tell it the spec forgets timezones" {
		t.Fatalf("one relayed issue with its quote expected; got %+v", issues)
	}
	issueID := issues[0].ID.String()

	// The revision, a review that forgets the issue (rejected in its turn),
	// and one that answers it.
	revised := strings.Replace(cascadeSpec("alpha"), "does what its design says, no more.",
		"does what its design says, with every time in UTC.", 1)
	h.scriptDocument(revised)
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"looks fine"}`, tiny)
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"the timezone is now stated",
		"issues":[{"issue_id":"`+issueID+`","status":"addressed","note":"Behaviour now says every time is in UTC."}]}`, tiny)
	reqsBefore := len(h.mock.Requests)
	h.resumeAuthor()
	h.srv.ReconcileAuthoringSweep(ctx)
	// The send holds its spec, so the answered approval waits for a person.
	h.eventually("revised spec approved by its reviewer, and held", func() bool {
		hold, err := store.GetDocumentHold(ctx, h.srv.Store.Pool, spec.ID)
		return err == nil && hold.DispatchID != nil && len(h.openIssues(spec.Path)) == 0
	})
	if code, page := h.postForm("/ui/document/release", map[string]string{"doc_id": spec.ID.String()}); code != 200 ||
		!strings.Contains(page, "approval stands") {
		t.Fatalf("release: %d\n%s", code, truncate(page, 300))
	}
	h.eventually("revised spec approved", func() bool { return h.docState(spec.Path) == lifecycle.DocApproved })

	// The author's prompt carried the draft and the issue.
	var authorPrompt, reviewRetry string
	for _, req := range h.mock.Requests[reqsBefore:] {
		for _, m := range req.Messages {
			for _, b := range m.Blocks {
				if strings.Contains(b.Text, "which you are revising") {
					authorPrompt = b.Text
				}
				if strings.Contains(b.Result, "cannot approve until human issue") {
					reviewRetry = b.Result
				}
			}
		}
	}
	mustContain(t, "revision prompt carries the issue", authorPrompt, "never says which timezone")
	mustContain(t, "revision prompt carries the quote", authorPrompt, "tell it the spec forgets timezones")
	if reviewRetry == "" {
		t.Error("an approval that skipped the issue should be rejected inside the reviewer's turn")
	}
	all, _ := store.CommentsForDocument(ctx, h.srv.Store.Pool, spec.ID, false)
	answered := false
	for _, c := range all {
		if c.IsIssue && c.Resolved && strings.Contains(c.AddressedNote, "UTC") && c.AddressedBy == "spec-reviewer" {
			answered = true
		}
	}
	if !answered {
		t.Error("the issue should be recorded as answered by the reviewer, with its note")
	}
}

// TestIssueOnAnApprovedSpecOpensASuccessor is SD-7 and FR-6.6: an issue on a
// ready, sent feature's approved spec opens a successor carrying it; Start
// building refuses meanwhile; the author fills it; its approval supersedes the
// old spec and its plan, returns the feature to idea, and the plan is
// rewritten.
func TestIssueOnAnApprovedSpecOpensASuccessor(t *testing.T) {
	h := newHarness(t)
	h.editConfig("  estimate: estimator", "  # estimate: estimator") // keep the FIFO to the documents
	h.designedInitiative("alpha")
	ctx := context.Background()

	h.scriptDocument(cascadeSpec("alpha"))
	h.scriptApproval()
	h.scriptDocument(cascadeDevPlan("alpha"))
	h.scriptApproval()
	h.send("pf/alpha", false)
	h.eventually("alpha ready", func() bool { return h.featureState("pf/alpha") == lifecycle.FeatReady })
	h.quiet()
	oldSpec := h.currentDoc("pf/alpha", "spec")
	oldPlan := h.currentDoc("pf/alpha", "dev_plan")

	h.pauseAuthor()
	code, page := h.postForm("/ui/document/issue", map[string]string{
		"doc_id": oldSpec.ID.String(), "issue": "It must also cover the empty case."})
	if code != 200 {
		t.Fatalf("raise issue: %d", code)
	}
	mustContain(t, "successor opened", page, "opened this revision")
	succ := h.currentDoc("pf/alpha", "spec")
	if succ == nil || succ.ID == oldSpec.ID || succ.State != lifecycle.DocDraft || succ.SupersedesID == nil {
		t.Fatalf("the issue should open a successor draft; current = %+v", succ)
	}
	if got := h.docState(oldSpec.Path); got != lifecycle.DocApproved {
		t.Fatalf("the approved spec stays approved until its successor is; state = %s", got)
	}
	if _, err := h.srv.StartFeature(ctx, "pf/alpha", "sam"); err == nil || !strings.Contains(err.Error(), "being revised") {
		t.Fatalf("Start building must refuse while the spec is revised; got %v", err)
	}
	issueID := h.openIssues(succ.Path)[0].ID.String()

	h.scriptDocument(strings.Replace(cascadeSpec("alpha"), "no more.", "including the empty case.", 1))
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"covers it",
		"issues":[{"issue_id":"`+issueID+`","status":"addressed","note":"Behaviour covers the empty case."}]}`, tiny)
	h.scriptDocument(cascadeDevPlan("alpha"))
	h.scriptApproval()
	h.resumeAuthor()
	h.srv.ReconcileAuthoringSweep(ctx)

	h.eventually("old spec and plan superseded, a new plan approved", func() bool {
		d, err := store.GetDocument(ctx, h.srv.Store.Pool, oldSpec.ID)
		p, perr := store.GetDocument(ctx, h.srv.Store.Pool, oldPlan.ID)
		cur := h.currentDoc("pf/alpha", "dev_plan")
		return err == nil && perr == nil && d.State == lifecycle.DocSuperseded &&
			p.State == lifecycle.DocSuperseded && cur != nil && cur.ID != oldPlan.ID &&
			cur.State == lifecycle.DocApproved
	})
	h.eventually("alpha ready again", func() bool { return h.featureState("pf/alpha") == lifecycle.FeatReady })
	var back int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'feature.transition'
		AND payload->>'event' = 'contract_invalidated' AND payload->>'cause' = 'spec-revision'`).Scan(&back)
	if back != 1 {
		t.Errorf("the revised spec should return alpha to idea once; got %d", back)
	}
}

// TestIssueOnAnUnsentFeatureIsRecordedOnly is FR-6.2's last row: nothing is
// dispatched for a feature nobody has sent. It is also SD-11: while a
// question about the spec waits in the Inbox, no act reaches it.
func TestIssueOnAnUnsentFeatureIsRecordedOnly(t *testing.T) {
	h := newHarness(t)
	h.designedInitiative("alpha")
	ctx := context.Background()
	path := "docs/specs/alpha.md"
	h.registerDoc(path, "spec", "feature", "pf/alpha", cascadeSpec("alpha"))
	if _, isErr, text := h.callTool("relay_issue", map[string]any{
		"path": path, "issue": "Name the error codes.", "quote": "it should name the error codes"}); isErr {
		t.Fatalf("relay_issue: %s", text)
	}
	h.quiet()
	if got := h.countPurpose("pf/alpha", "write-spec"); got != 0 {
		t.Fatalf("an unsent feature gets no author; got %d dispatches", got)
	}
	if got := h.docState(path); got != lifecycle.DocDraft {
		t.Fatalf("the draft is recorded on, not moved; state = %s", got)
	}

	// Escalate a review, then every act is refused until the Inbox answers.
	h.mock.RespondOutcome("submit_review", `{"verdict":"escalate","reasoning":"can't tell"}`, tiny)
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": path}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	h.eventually("escalated", func() bool { return len(h.pendingByKind("review-escalation")) == 1 })
	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"relay_verdict", map[string]any{"path": path, "verdict": "approve", "quote": "approve it"}},
		{"relay_issue", map[string]any{"path": path, "issue": "x", "quote": "x"}},
		{"relay_review_request", map[string]any{"path": path, "quote": "review it again"}},
	} {
		if _, isErr, text := h.callTool(tool.name, tool.args); !isErr || !strings.Contains(text, "Inbox") {
			t.Errorf("%s must be refused while the escalation waits; got %v %q", tool.name, isErr, text)
		}
	}
	_ = ctx
}

// TestReviseLoopIsBounded is FR-3.3: a spec sent back as often as the round
// cap allows raises an authoring-deadlock question instead of another round;
// answering retry in the Inbox allows exactly one more.
func TestReviseLoopIsBounded(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	h.designedInitiative("alpha")
	ctx := context.Background()
	sendBack := `{"verdict":"request_changes","reasoning":"incomplete",
		"comments":[{"section_ref":"Behaviour","body":"Say what happens on failure.","severity":"major"}]}`

	// max_review_rounds is 3 by default: write, review ×3 (each sent back),
	// with two revisions between.
	for i := 0; i < 3; i++ {
		h.scriptDocument(cascadeSpec("alpha"))
		h.mock.RespondOutcome("submit_review", sendBack, tiny)
	}
	h.send("pf/alpha", false)
	h.eventually("authoring-deadlock raised", func() bool { return len(h.pendingByKind("authoring-deadlock")) == 1 })
	h.quiet()
	if got := h.countPurpose("pf/alpha", "write-spec"); got != 3 {
		t.Fatalf("three rounds of writing expected before the cap; got %d", got)
	}
	// The revision prompts carried the finding.
	found := false
	for _, req := range h.mock.Requests {
		for _, m := range req.Messages {
			for _, b := range m.Blocks {
				if strings.Contains(b.Text, "which you are revising") && strings.Contains(b.Text, "Say what happens on failure.") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("a revising author should see the review's finding")
	}
	cp := h.pendingByKind("authoring-deadlock")[0]
	h.scriptDocument(cascadeSpec("alpha"))
	h.scriptApproval()
	if code, body := h.postForm("/ui/respond", map[string]string{"id": cp.ID.String(), "verb": "retry"}); code != 200 {
		t.Fatalf("respond: %d\n%s", code, truncate(body, 300))
	}
	h.eventually("one more round, approved", h.docStateIs("pf/alpha", "spec", lifecycle.DocApproved))
	_ = ctx
}

// ---- Withdraw (FR-4.6) ----

// TestWithdrawBeforeWorkStarts: a send whose spec writer waits in the queue
// (the budget holds it) can be withdrawn, which cancels the queued dispatch;
// once work has run, it can't.
func TestWithdrawBeforeWorkStarts(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	h.designedInitiative("alpha", "beta")
	ctx := context.Background()

	// Spend the whole budget so the governor holds new work in the queue.
	if _, err := h.srv.Store.Pool.Exec(ctx, `INSERT INTO dispatches
		(id, state, purpose, role, model, ref_type, ref_id, idempotency_key, cost_usd, finished_at)
		VALUES (gen_random_uuid(), 'succeeded', 'estimate', 'estimator', 'claude-sonnet-5', 'project', gen_random_uuid(), 'spent', 100, now())`); err != nil {
		t.Fatal(err)
	}
	h.send("pf/alpha", false)
	h.eventually("write-spec queued", func() bool { return h.countPurpose("pf/alpha", "write-spec") == 1 })
	alpha, _ := h.srv.featureByPath(ctx, "pf/alpha")
	code, page := h.postForm("/ui/send/withdraw", map[string]string{"id": alpha.ID.String()})
	if code != 200 {
		t.Fatalf("withdraw: %d", code)
	}
	mustContain(t, "withdrawn", page, "The send was withdrawn")
	if _, err := store.GetFeatureSend(ctx, h.srv.Store.Pool, alpha.ID); err != store.ErrNotFound {
		t.Fatalf("the mark should be gone; got %v", err)
	}
	var state string
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT state FROM dispatches WHERE ref_id = $1 AND purpose = 'write-spec'`, alpha.ID).Scan(&state)
	if state != "cancelled" {
		t.Fatalf("the queued writer should be cancelled; state = %s", state)
	}

	// Once a dispatch has left the queue, the send stands.
	if _, err := h.srv.Store.Pool.Exec(ctx, `DELETE FROM dispatches WHERE idempotency_key = 'spent'`); err != nil {
		t.Fatal(err)
	}
	h.send("pf/beta", false)
	h.eventually("beta's writer has run", func() bool {
		var n int
		_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches d JOIN features f ON f.id = d.ref_id
			WHERE f.slug = 'beta' AND d.purpose = 'write-spec' AND d.state <> 'queued'`).Scan(&n)
		return n == 1
	})
	beta, _ := h.srv.featureByPath(ctx, "pf/beta")
	if err := h.srv.WithdrawSend(ctx, beta.ID, "sam"); err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("withdraw after work started must be refused; got %v", err)
	}
}

// ---- The document page (FR-9) ----

// TestDocumentPageSubmitReviseDetach drives Submit, Revise and Detach from
// the document page, each with its refusal.
func TestDocumentPageSubmitReviseDetach(t *testing.T) {
	h := newHarness(t)
	h.designedInitiative("alpha")
	ctx := context.Background()

	// Submit: an invalid draft stays a draft and says why; a valid one goes
	// to review and is approved by its reviewer.
	bad := "docs/specs/bad.md"
	h.registerDoc(bad, "spec", "feature", "pf/alpha", "---\ntitle: Bad\ntype: spec\nowner: pf/alpha\n---\n\n# Bad\n\n## Overview\n\nOnly this.\n")
	badDoc, _ := store.LiveDocumentByPath(ctx, h.srv.Store.Pool, bad)
	_, page := h.postForm("/ui/document/submit", map[string]string{"doc_id": badDoc.ID.String()})
	mustContain(t, "validation refusal", page, "doesn&#39;t pass validation yet")
	if got := h.docState(bad); got != lifecycle.DocDraft {
		t.Fatalf("an invalid draft stays a draft; state = %s", got)
	}

	// Detach: the draft goes, the file stays, the issue's words are kept.
	if _, err := h.srv.RaiseIssue(ctx, badDoc.ID, "It has no behaviour at all.", "", relayAct{Actor: "sam", Via: "ui"}); err != nil {
		t.Fatal(err)
	}
	_, page = h.getUI("/ui/d/" + bad)
	mustContain(t, "detach offered", page, `action="/ui/document/detach"`)
	code, page := h.postForm("/ui/document/detach", map[string]string{"doc_id": badDoc.ID.String(), "confirm": "1"})
	if code != 200 {
		t.Fatalf("detach: %d", code)
	}
	mustContain(t, "detach notice", page, "was detached")
	if _, err := store.GetDocument(ctx, h.srv.Store.Pool, badDoc.ID); err != store.ErrNotFound {
		t.Fatalf("the registration should be gone; got %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.root, bad)); err != nil {
		t.Fatalf("detach must leave the file: %v", err)
	}
	var kept int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'document.detached'
		AND payload::text LIKE '%no behaviour at all%'`).Scan(&kept)
	if kept != 1 {
		t.Error("the dropped issue's words should be on the detach audit row")
	}

	good := "docs/specs/alpha.md"
	h.registerDoc(good, "spec", "feature", "pf/alpha", cascadeSpec("alpha"))
	goodDoc, _ := store.LiveDocumentByPath(ctx, h.srv.Store.Pool, good)
	h.scriptApproval()
	_, page = h.postForm("/ui/document/submit", map[string]string{"doc_id": goodDoc.ID.String()})
	mustContain(t, "submitted", page, "submitted for review")
	h.eventually("approved", func() bool { return h.docState(good) == lifecycle.DocApproved })
	if _, err := h.srv.DetachDocument(ctx, goodDoc.ID, "sam"); err == nil || !strings.Contains(err.Error(), "Only a draft") {
		t.Fatalf("detaching an approved document must be refused; got %v", err)
	}

	// Revise: opens a successor, and a second Revise is refused.
	_, page = h.postForm("/ui/document/revise", map[string]string{"doc_id": goodDoc.ID.String()})
	mustContain(t, "revision opened", page, "A revision was opened")
	succ := h.currentDoc("pf/alpha", "spec")
	if succ == nil || succ.SupersedesID == nil || *succ.SupersedesID != goodDoc.ID {
		t.Fatalf("Revise should open a successor; current = %+v", succ)
	}
	_, page = h.postForm("/ui/document/revise", map[string]string{"doc_id": goodDoc.ID.String()})
	mustContain(t, "second revise refused", page, "already open")
}

// ---- The design reviewer, retired (FR-10) and the starter pack (FR-11) ----

func TestDesignReviewerRetiredAndChainEnabled(t *testing.T) {
	h := newHarness(t)
	comp := filepath.Join(h.root, ".subutai")
	if _, err := os.Stat(filepath.Join(comp, "roles/design-reviewer.yaml")); !os.IsNotExist(err) {
		t.Error("the starter pack must not ship the design reviewer")
	}
	if _, err := os.Stat(filepath.Join(comp, "chat-skills/review-design/SKILL.md")); err != nil {
		t.Errorf("review-design should ship as a chat-side skill: %v", err)
	}
	cfg, _ := os.ReadFile(filepath.Join(comp, "config.yaml"))
	for _, want := range []string{"\n  write-spec: spec-author", "\n  write-dev-plan: dev-plan-author", "spec_review:"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("the generated config should carry %q", strings.TrimSpace(want))
		}
	}

	// A submitted design queues nothing and waits for a person.
	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": "pf", "name": "Platform"}); code != 201 {
		t.Fatalf("initiative: %d %v", code, out)
	}
	path := "docs/design/pf.md"
	h.registerDoc(path, "design", "initiative", "pf", cascadeDesignV1)
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": path}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	h.quiet()
	var n int
	_ = h.srv.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM dispatches`).Scan(&n)
	if n != 0 || h.docState(path) != lifecycle.DocReviewing {
		t.Fatalf("a design waits for a person with no agent review; %d dispatches, state %s", n, h.docState(path))
	}
	_, page := h.getUI("/ui/d/" + path)
	mustContain(t, "human decision", page, "waiting for your decision")

	// A relayed request for a design review is refused with advice.
	if _, isErr, text := h.callTool("relay_review_request", map[string]any{"path": path, "quote": "review my design"}); !isErr ||
		!strings.Contains(text, "review-design chat skill") {
		t.Fatalf("a design has no agent reviewer to request; got %v %q", isErr, text)
	}
	// And the person's verdict can be relayed.
	if _, isErr, text := h.callTool("relay_verdict", map[string]any{"path": path, "verdict": "approve",
		"quote": "Yes, approve the platform design."}); isErr {
		t.Fatalf("relayed design approval: %s", text)
	}
	if got := h.docState(path); got != lifecycle.DocApproved {
		t.Fatalf("relayed approval should approve the design; state = %s", got)
	}
	h.quiet()
	_ = h.srv.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM dispatches`).Scan(&n)
	if n != 0 {
		t.Errorf("approving a design starts nothing; %d dispatches", n)
	}
}

// ---- The relays (FR-8) ----

func TestRelayVerdictsAndRefusals(t *testing.T) {
	h := newHarness(t)
	h.designedInitiative("alpha")
	ctx := context.Background()
	path := "docs/specs/alpha.md"
	h.registerDoc(path, "spec", "feature", "pf/alpha", cascadeSpec("alpha"))

	for _, c := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"path": path, "verdict": "approve"}, "quote"},
		{map[string]any{"path": path, "verdict": "approve", "quote": "approve it"}, "Submit it for review first"},
		{map[string]any{"path": path, "verdict": "maybe", "quote": "hmm"}, "approve"},
		{map[string]any{"path": "docs/nope.md", "verdict": "approve", "quote": "x"}, "no current document"},
	} {
		if _, isErr, text := h.callTool("relay_verdict", c.args); !isErr || !strings.Contains(text, c.want) {
			t.Errorf("relay_verdict %v: want a refusal mentioning %q; got %v %q", c.args, c.want, isErr, text)
		}
	}
	// A relayed review request submits the draft; a relayed send-back is an
	// issue and a return to draft; a relayed approval approves, as a person.
	h.mock.RespondOutcome("submit_review", `{"verdict":"escalate","reasoning":"not sure"}`, tiny)
	if _, isErr, text := h.callTool("relay_review_request", map[string]any{"path": path, "quote": "get it reviewed"}); isErr {
		t.Fatalf("review request: %s", text)
	}
	h.eventually("escalated", func() bool { return len(h.pendingByKind("review-escalation")) == 1 })
	// The escalation is the Inbox's; answer it there.
	cp := h.pendingByKind("review-escalation")[0]
	if code, _ := h.postForm("/ui/respond", map[string]string{"id": cp.ID.String(), "verb": "request_changes", "reason": "Name the failure modes."}); code != 200 {
		t.Fatal("respond")
	}
	h.eventually("sent back", func() bool { return h.docState(path) == lifecycle.DocDraft })
	if got := len(h.openIssues(path)); got != 1 {
		t.Fatalf("a person's send-back on an escalation is an issue (SD-15); got %d open", got)
	}
	h.mock.RespondOutcome("submit_review", `{"verdict":"escalate","reasoning":"still not sure"}`, tiny)
	if _, isErr, text := h.callTool("relay_review_request", map[string]any{"path": path, "quote": "try again"}); isErr {
		t.Fatalf("second review request: %s", text)
	}
	h.eventually("escalated again", func() bool { return len(h.pendingByKind("review-escalation")) == 1 })
	cp = h.pendingByKind("review-escalation")[0]
	if code, _ := h.postForm("/ui/respond", map[string]string{"id": cp.ID.String(), "verb": "approve"}); code != 200 {
		t.Fatal("respond")
	}
	h.eventually("approved on the escalation", func() bool { return h.docState(path) == lifecycle.DocApproved })
	if got := len(h.openIssues(path)); got != 0 {
		t.Errorf("a person's approval settles open issues (SD-6); %d still open", got)
	}
	var quoted int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'document.review_requested'
		AND payload->>'via' = 'mcp' AND payload->>'quote' = 'get it reviewed'`).Scan(&quoted)
	if quoted != 1 {
		t.Errorf("a relayed review request is audited with its quote; got %d", quoted)
	}
}

// ---- The cascade, sent and unsent (FR-2.5) ----

// TestCascadeLeavesAnUnsentFeatureWithoutASpec: a revised design retires the
// single affected spec (SPEC-009 FR-9.1) whether or not its feature is sent,
// and rewrites it only for a sent one.
func TestCascadeLeavesAnUnsentFeatureWithoutASpec(t *testing.T) {
	h := newHarness(t)
	h.designedInitiative("alpha")
	path := "docs/specs/alpha.md"
	h.registerDoc(path, "spec", "feature", "pf/alpha", cascadeSpec("alpha"))
	h.approveDoc(path)
	spec, _ := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, path)

	h.reviseAndApproveDesign("docs/design/pf.md", "- A second seam.\n")
	h.eventually("spec retired", func() bool {
		d, err := store.GetDocument(context.Background(), h.srv.Store.Pool, spec.ID)
		return err == nil && d.State == lifecycle.DocSuperseded
	})
	h.quiet()
	if got := h.countPurpose("pf/alpha", "write-spec"); got != 0 {
		t.Fatalf("an unsent feature's retired spec is not rewritten; got %d", got)
	}
	if h.currentDoc("pf/alpha", "spec") != nil {
		t.Fatal("the unsent feature should be left without a spec")
	}
	// Sending it now writes the fresh spec against the revised design.
	h.scriptDocument(cascadeSpec("alpha"))
	h.scriptApproval()
	h.editConfig("  write-dev-plan: dev-plan-author", "  # write-dev-plan: dev-plan-author")
	h.send("pf/alpha", false)
	h.eventually("rewritten after the send", h.docStateIs("pf/alpha", "spec", lifecycle.DocApproved))
}

// ---- No configuration lets a spec through unreviewed (NFR-4) ----

func TestNoSettingApprovesASpecUnreviewed(t *testing.T) {
	for _, c := range []struct {
		agent, hold bool
		approved    bool
	}{
		{true, false, true}, // the reviewer approves
		{true, true, false}, // held for a person after the review
		{false, false, false},
		{false, true, false},
	} {
		t.Run(fmt.Sprintf("agent=%v,hold=%v", c.agent, c.hold), func(t *testing.T) {
			h := newHarness(t)
			if !c.agent {
				h.editConfig("  agent: true", "  agent: false")
			}
			if c.hold {
				h.editConfig("  hold: false", "  hold: true")
			}
			h.designedInitiative("alpha")
			path := "docs/specs/alpha.md"
			h.registerDoc(path, "spec", "feature", "pf/alpha", cascadeSpec("alpha"))
			if c.agent {
				h.scriptApproval()
			}
			if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": path}); code != 200 {
				t.Fatalf("submit: %d %v", code, out)
			}
			doc, _ := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, path)
			// Settle: approved, or held for a person.
			h.eventually("settled", func() bool {
				if h.docState(path) == lifecycle.DocApproved {
					return true
				}
				_, err := store.GetDocumentHold(context.Background(), h.srv.Store.Pool, doc.ID)
				return err == nil
			})
			if got := h.docState(path) == lifecycle.DocApproved; got != c.approved {
				t.Errorf("approved = %v, want %v", got, c.approved)
			}
		})
	}
}

// ---- The seam (NFR-3) ----

func TestOnlyTheWebUISends(t *testing.T) {
	h := newHarness(t)
	h.designedInitiative("alpha")
	alpha, _ := h.srv.featureByPath(context.Background(), "pf/alpha")
	for _, path := range []string{"/api/features/send", "/api/send", "/api/features/send-to-development"} {
		code, _ := h.call("POST", path, map[string]string{"id": alpha.ID.String(), "path": "pf/alpha"})
		if code != 404 && code != 405 {
			t.Errorf("POST %s answered %d; no API route may send (FR-4.5)", path, code)
		}
	}
	if _, err := store.GetFeatureSend(context.Background(), h.srv.Store.Pool, alpha.ID); err != store.ErrNotFound {
		t.Fatal("nothing but the web UI may write the sent mark")
	}
}

// TestAnOpenDesignRevisionKeepsG0 is a walkthrough finding: while a revision
// of an approved design is open, the approved design is still in force, so
// its features can still be sent.
func TestAnOpenDesignRevisionKeepsG0(t *testing.T) {
	h := newHarness(t)
	h.designedInitiative("alpha")
	if code, out := h.call("POST", "/api/docs/revise", map[string]string{"path": "docs/design/pf.md"}); code != 201 {
		t.Fatalf("revise: %d %v", code, out)
	}
	f, _ := h.srv.featureByPath(context.Background(), "pf/alpha")
	if ok, why := h.srv.sendReadiness(context.Background(), f); !ok {
		t.Fatalf("an open revision must not unapprove the design in force; refused: %s", why)
	}
}
