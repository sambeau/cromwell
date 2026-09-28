package server

// SPEC-017: chat as a proper seat. The mock provider stands in for every
// model, against real Postgres and a real git repository, as the other
// integration suites do. Every chat step goes over POST /mcp.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// ---- Helpers ----

// mustTool calls a tool and fails the test if it errs.
func (h *harness) mustTool(name string, args map[string]any) map[string]any {
	h.t.Helper()
	out, isErr, text := h.callTool(name, args)
	if isErr {
		h.t.Fatalf("%s: %s", name, text)
	}
	return out
}

// toolRefused calls a tool and fails the test unless it is refused with a
// message containing want.
func (h *harness) toolRefused(name string, args map[string]any, want string) {
	h.t.Helper()
	_, isErr, text := h.callTool(name, args)
	if !isErr || !strings.Contains(text, want) {
		h.t.Errorf("%s should be refused with %q; got isErr=%v %q", name, want, isErr, text)
	}
}

// docEntry finds a document entry of a type in a feature's get_feature
// result.
func (h *harness) docEntry(featurePath, docType string) map[string]any {
	h.t.Helper()
	out := h.mustTool("get_feature", map[string]any{"path": featurePath})
	for _, d := range out["documents"].([]any) {
		m := d.(map[string]any)
		if m["type"] == docType && m["state"] != "superseded" {
			return m
		}
	}
	h.t.Fatalf("%s has no %s in %v", featurePath, docType, out["documents"])
	return nil
}

func sentence(m map[string]any, key string) string {
	if v, ok := m[key].(map[string]any); ok {
		s, _ := v["sentence"].(string)
		return s
	}
	return ""
}

// verdictsOf reads a document's recorded verdicts.
func (h *harness) verdictsOf(docID uuid.UUID) []store.Verdict {
	h.t.Helper()
	vs, err := store.Verdicts(context.Background(), h.srv.Store.Pool, docID)
	if err != nil {
		h.t.Fatal(err)
	}
	return vs
}

// writersOf reads a document's recorded writers.
func (h *harness) writersOf(docID uuid.UUID) []store.Writer {
	h.t.Helper()
	ws, err := store.Writers(context.Background(), h.srv.Store.Pool, docID)
	if err != nil {
		h.t.Fatal(err)
	}
	return ws
}

func chatSpecPath(feature string) string { return "docs/chat/" + feature + "-spec.md" }

// ---- The end-to-end proof (FR-6) ----

// TestChatWrittenSpecFlowsToStartBuilding is SPEC-017's goal: the chat agent
// writes a spec, adopts it and submits it with no quote; the reviewer
// approves; a person presses Send and the send screen says spec writing and
// review are skipped; the plan is written, reviewed and estimated; it stops
// at Start building; and the spec says the chat agent wrote it. The read
// tools then tell the story (FR-4).
func TestChatWrittenSpecFlowsToStartBuilding(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("alpha")
	h.quiet()
	alpha, _ := h.srv.featureByPath(ctx, "pf/alpha")

	// 1–2. The chat agent writes the file, commits it, and adopts it.
	path := chatSpecPath("alpha")
	h.writeCommitted(path, cascadeSpec("alpha"))
	adopted := h.mustTool("adopt_document", map[string]any{
		"path": path, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/alpha"})
	specID, _ := adopted["id"].(string)
	if specID == "" {
		t.Fatalf("adopted spec has no ID: %v", adopted)
	}
	if got := sentence(adopted, "written_by"); got != "Written by the chat agent." {
		t.Errorf("adopt result written_by = %q", got)
	}

	// 3. Submitted by its ID, with no quote; the reviewer approves.
	h.scriptApproval()
	sub := h.mustTool("submit_for_review", map[string]any{"document": specID})
	if done, _ := sub["done"].(string); !strings.Contains(done, "Its review is queued") || !strings.Contains(done, "you don't") {
		t.Errorf("submit_for_review says %q", done)
	}
	h.eventually("spec approved by the reviewer", func() bool { return h.docState(path) == lifecycle.DocApproved })
	h.quiet()
	if n := h.countPurpose("pf/alpha", "write-dev-plan"); n != 0 {
		t.Fatalf("an unsent feature's approved spec starts nothing; %d plan runs", n)
	}
	var fromChat int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'document.transition'
		AND payload->>'event' = 'submit' AND payload->>'via' = 'mcp' AND (payload->>'submitted_from_chat')::boolean`).Scan(&fromChat)
	if fromChat != 1 {
		t.Errorf("the submission's own audit row says it came from chat; got %d", fromChat)
	}

	// 5. The send screen says both steps are skipped, and who did them.
	code, screen := h.getUI("/ui/send?feature=" + alpha.ID.String())
	if code != 200 {
		t.Fatalf("send screen: %d", code)
	}
	mustContain(t, "spec writing skipped", screen, "Already written by the chat agent. This step is skipped.")
	mustContain(t, "spec review skipped", screen, "Approved by the spec reviewer (")
	if strings.Count(screen, "This step is skipped.") < 2 {
		t.Errorf("both spec steps should be skipped on the send screen")
	}

	// 4, 6. A person presses Send; the plan is written, reviewed, estimated.
	h.scriptDocument(cascadeDevPlan("alpha"))
	h.scriptApproval()
	h.scriptEstimate()
	if code, page := h.postForm("/ui/send", map[string]string{
		"owner_kind": "feature", "owner_id": alpha.ID.String(), "feature": alpha.ID.String()}); code != 200 {
		t.Fatalf("send: %d\n%s", code, truncate(page, 300))
	}
	h.eventually("alpha ready with an estimate", func() bool {
		if h.featureState("pf/alpha") != lifecycle.FeatReady {
			return false
		}
		_, err := store.CurrentEstimate(ctx, h.srv.Store.Pool, "feature", alpha.ID)
		return err == nil
	})
	h.quiet()

	// 7. Spec writing never ran, and building hasn't started.
	if n := h.countPurpose("pf/alpha", "write-spec"); n != 0 {
		t.Errorf("spec writing must be skipped; %d write-spec runs", n)
	}
	var implement int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE purpose = 'implement-task'`).Scan(&implement)
	if implement != 0 {
		t.Errorf("the chain stops at Start building; %d implement runs", implement)
	}

	// 8. The spec says who wrote it and who approved it, on its page and
	// over MCP.
	_, page := h.getUI("/ui/d/" + path)
	mustContain(t, "spec page writer", page, "Written by the chat agent.")
	mustContain(t, "spec page verdict", page, "Approved by the spec reviewer (")
	mustContain(t, "link to the review", page, "Read the review")
	spec := h.docEntry("pf/alpha", "spec")
	if wb := spec["written_by"].(map[string]any); wb["kind"] != "chat" {
		t.Errorf("spec written_by = %v", wb)
	}
	if lv := spec["last_verdict"].(map[string]any); lv["kind"] != "agent" || lv["verdict"] != "approve" || lv["run_id"] == nil {
		t.Errorf("spec last_verdict = %v", lv)
	}
	// The plan says its author agent wrote it, and links the run.
	plan := h.currentDoc("pf/alpha", "dev_plan")
	_, planPage := h.getUI("/ui/d/" + plan.Path)
	mustContain(t, "plan writer", planPage, "Written by the dev plan author (")
	mustContain(t, "plan run link", planPage, "What the agent did")

	// FR-4.1: the timeline, from chat.
	tl := h.mustTool("get_timeline", map[string]any{"feature": alpha.PublicID})
	var sawWritten, sawApproved, sawSent bool
	var reviewRun string
	for _, raw := range tl["moments"].([]any) {
		m := raw.(map[string]any)
		switch m["label"] {
		case "Spec written":
			sawWritten = m["by"] == "chat" && m["who"] == "the chat agent"
		case "Spec approved":
			if c, ok := m["cause"].(map[string]any); ok && strings.HasPrefix(fmt.Sprint(c["what"]), "Review") {
				sawApproved = true
				reviewRun, _ = c["run_id"].(string)
			}
		case "Sent to development":
			sawSent = true
		}
	}
	if !sawWritten || !sawApproved || !sawSent {
		t.Fatalf("timeline moments: written by chat %v, approved by a review %v, sent %v\n%v",
			sawWritten, sawApproved, sawSent, tl["moments"])
	}

	// FR-4.2: the review run, from chat.
	run := h.mustTool("get_agent_run", map[string]any{"run_id": reviewRun})
	if c := run["conclusion"].(map[string]any); c["verdict"] != "approve" {
		t.Errorf("run conclusion = %v", c)
	}
	if applied, _ := run["verdict_applied"].(string); !strings.HasPrefix(applied, "Applied") {
		t.Errorf("verdict_applied = %q", applied)
	}
	if tr, ok := run["transcript"].(map[string]any); !ok || tr["tool_results"] == nil {
		t.Errorf("the transcript leaves tool results out unless asked: %v", run["transcript"])
	}
	h.toolRefused("get_agent_run", map[string]any{"run_id": reviewRun, "attempt": 5}, "attempt")
	h.toolRefused("get_agent_run", map[string]any{"run_id": uuid.NewString()}, "no agent run")
	h.toolRefused("get_timeline", map[string]any{"feature": "pf/nope"}, "no feature")
}

// TestChatAttachedSpecAndSubmitRefusals is FR-1.2, FR-1.3, FR-1.5 and SD-11:
// an attached spec is credited to the chat agent; each refusal is a
// sentence; and an ID names the revision in front of the person.
func TestChatAttachedSpecAndSubmitRefusals(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("beta")
	h.quiet()

	// A spec that fails its template: refused, listing what to fix.
	path := chatSpecPath("beta")
	h.writeCommitted(path, "---\ntitle: Spec for beta\ntype: spec\n---\n\n# Spec for beta\n\nNot finished.\n")
	att := h.mustTool("attach_document", map[string]any{
		"path": path, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/beta"})
	if got := sentence(att, "written_by"); got != "Written by the chat agent." {
		t.Errorf("attach result written_by = %q", got)
	}
	h.toolRefused("submit_for_review", map[string]any{"document": path}, "doesn't pass its template's checks")
	if got := h.docState(path); got != lifecycle.DocDraft {
		t.Fatalf("a refused submission leaves a draft; state = %s", got)
	}

	// Fixed, it is submitted; again, it is already in review.
	h.writeCommitted(path, cascadeSpec("beta"))
	_ = h.srv.OnPostCommit(ctx)
	h.scriptApproval()
	h.mustTool("submit_for_review", map[string]any{"document": path})
	h.eventually("approved", func() bool { return h.docState(path) == lifecycle.DocApproved })
	h.toolRefused("submit_for_review", map[string]any{"document": path}, "This document is approved")

	// A fresh review is a person's: the relay still needs their words.
	h.toolRefused("relay_review_request", map[string]any{"path": path}, "quote")

	// A document with no template can't be submitted.
	h.writeCommitted("docs/chat/beta-note.md", "# A note\n\nThoughts.\n")
	h.mustTool("attach_document", map[string]any{
		"path": "docs/chat/beta-note.md", "doc_type": "note", "owner_type": "feature", "owner_path": "pf/beta"})
	h.toolRefused("submit_for_review", map[string]any{"document": "docs/chat/beta-note.md"}, "no template")
	h.toolRefused("submit_for_review", map[string]any{"document": "docs/nothing.md"}, "no current document")

	// SD-11 and SD-1 case 4: adopt it for an ID, open a revision, and build
	// the feature. The ID names the open revision, and submitting a revision
	// of a feature being built is a person's call.
	spec := h.docAt(path)
	h.mustTool("adopt_document", map[string]any{"path": path, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/beta"})
	spec = h.docAt(path)
	if spec.PublicID == "" {
		t.Fatalf("adopting a registered spec gives it an ID")
	}
	succ, err := h.srv.ReviseDoc(ctx, path, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE features SET state = 'active' WHERE id = $1`, *spec.OwnerID); err != nil {
		t.Fatal(err)
	}
	h.toolRefused("submit_for_review", map[string]any{"document": spec.PublicID}, "being built")
	h.toolRefused("submit_for_review", map[string]any{"document": succ.Path}, "being built")
	// Named exactly, the superseded-to-be approved revision is still in force.
	h.toolRefused("submit_for_review", map[string]any{"document": fmt.Sprintf("%s.r%d", spec.PublicID, spec.Revision)}, "This document is approved")
}

// ---- Verdicts (FR-2.3, FR-2.4, FR-2.7) ----

// TestVerdictsRecordWhoAndHow: a relayed approval, a relayed send-back, a
// held agent approval and its relayed release, an escalation answer, and a
// person's approval in the web UI each record who gave the verdict and how;
// an issue on a document in review records none; and the timeline credits a
// relayed verdict to a person.
func TestVerdictsRecordWhoAndHow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("alpha", "beta", "gamma")
	h.quiet()

	// With agent review off, a submitted spec waits for a person, who
	// approves it through the chat agent.
	h.editConfig("  agent: true", "  agent: false")
	path := chatSpecPath("alpha")
	h.writeCommitted(path, cascadeSpec("alpha"))
	h.mustTool("adopt_document", map[string]any{"path": path, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/alpha"})
	sub := h.mustTool("submit_for_review", map[string]any{"document": path})
	if done, _ := sub["done"].(string); !strings.Contains(done, "Agent review is switched off") {
		t.Errorf("submit_for_review with agent review off says %q", done)
	}
	res := h.mustTool("relay_verdict", map[string]any{"path": path, "verdict": "approve", "quote": "Yes, alpha's spec is right."})
	if got := sentence(res, "last_verdict"); got != "Approved by a person, relayed by the chat agent: “Yes, alpha's spec is right.”" {
		t.Errorf("relayed approval reads %q", got)
	}
	vs := h.verdictsOf(h.docID(path))
	if len(vs) != 1 || vs[0].Kind != store.GiverPerson || vs[0].Via != "mcp" || vs[0].Quote == "" {
		t.Errorf("relayed approval recorded as %+v", vs)
	}
	var moment string
	tl := h.mustTool("get_timeline", map[string]any{"feature": "pf/alpha"})
	for _, raw := range tl["moments"].([]any) {
		if m := raw.(map[string]any); m["label"] == "Spec approved" {
			moment, _ = m["who"].(string)
			if m["relayed"] != true || m["by"] != "person" {
				t.Errorf("relayed approval moment = %v", m)
			}
		}
	}
	if moment != "a person, relayed by the chat agent" {
		t.Errorf("the relayed approval's moment is credited to %q", moment)
	}
	_, tlPage := h.getUI("/ui/f/pf/alpha")
	mustContain(t, "timeline page", tlPage, "by a person, relayed by the chat agent")

	// A relayed send-back is the person's verdict too.
	pathB := chatSpecPath("beta")
	h.writeCommitted(pathB, cascadeSpec("beta"))
	h.mustTool("attach_document", map[string]any{"path": pathB, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/beta"})
	h.mustTool("submit_for_review", map[string]any{"document": pathB})
	res = h.mustTool("relay_verdict", map[string]any{"path": pathB, "verdict": "send_back",
		"reason": "Name the error cases.", "quote": "Send beta back, it needs the error cases."})
	if got := sentence(res, "last_verdict"); !strings.HasPrefix(got, "Sent back by a person, relayed by the chat agent: “Send beta back") {
		t.Errorf("relayed send-back reads %q", got)
	}

	// An issue on a document in review, on a sent feature, sends it back but
	// records no verdict (FR-2.3).
	h.writeCommitted(pathB, cascadeSpec("beta")+"\n- Every error case is named\n")
	_ = h.srv.OnPostCommit(ctx)
	h.mustTool("submit_for_review", map[string]any{"document": pathB})
	h.pauseAuthor()
	h.send("pf/beta", true)
	before := len(h.verdictsOf(h.docID(pathB)))
	h.mustTool("relay_issue", map[string]any{"path": pathB, "issue": "The timeout is missing.", "quote": "Beta has no timeout."})
	if got := h.docState(pathB); got != lifecycle.DocDraft {
		t.Fatalf("an issue on a sent feature's spec in review sends it back; state = %s", got)
	}
	if after := len(h.verdictsOf(h.docID(pathB))); after != before {
		t.Errorf("an issue is not a verdict; verdicts %d -> %d", before, after)
	}
	h.resumeAuthor()

	// With agent review on and the hold on, the reviewer's approval is held,
	// and recorded so; a relayed release records who let it stand.
	h.editConfig("  agent: false", "  agent: true")
	h.editConfig("  hold: false", "  hold: true")
	pathG := chatSpecPath("gamma")
	h.writeCommitted(pathG, cascadeSpec("gamma"))
	h.mustTool("attach_document", map[string]any{"path": pathG, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/gamma"})
	h.scriptApproval()
	sub = h.mustTool("submit_for_review", map[string]any{"document": pathG})
	if done, _ := sub["done"].(string); !strings.Contains(done, "held for a person") {
		t.Errorf("submit_for_review with the hold on says %q", done)
	}
	gID := h.docID(pathG)
	h.eventually("held approval recorded", func() bool {
		vs := h.verdictsOf(gID)
		return len(vs) == 1 && vs[0].Held && vs[0].Kind == store.GiverAgent && vs[0].DispatchID != nil
	})
	_, page := h.getUI("/ui/d/" + pathG)
	mustContain(t, "held verdict", page, "and held for a person.")
	h.mustTool("relay_release_hold", map[string]any{"path": pathG, "quote": "Let the reviewer decide on gamma."})
	lv, err := store.LastVerdict(ctx, h.srv.Store.Pool, gID)
	if err != nil || lv.Kind != store.GiverAgent || lv.ReleasedVia != "mcp" || lv.ReleasedQuote == "" {
		t.Fatalf("release recorded as %+v (%v)", lv, err)
	}
	if got := verdictSentence(*lv); !strings.Contains(got, "a person let the reviewer decide, relayed by the chat agent: “Let the reviewer decide on gamma.”") {
		t.Errorf("release reads %q", got)
	}
	h.editConfig("  hold: true", "  hold: false")
}

// TestEscalationAndUIVerdicts: an escalation answered in the Inbox records a
// person's verdict with the review it ruled on; a design approved on its page
// in the web UI records the web UI actor.
func TestEscalationAndUIVerdicts(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("alpha")
	h.quiet()

	path := chatSpecPath("alpha")
	h.writeCommitted(path, cascadeSpec("alpha"))
	h.mustTool("attach_document", map[string]any{"path": path, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/alpha"})
	h.mock.RespondOutcome("submit_review", `{"verdict":"escalate","reasoning":"I can't tell whether this is right."}`, tiny)
	h.mustTool("submit_for_review", map[string]any{"document": path})
	h.eventually("escalated", func() bool { return len(h.pendingByKind("review-escalation")) == 1 })
	cp := h.pendingByKind("review-escalation")[0]
	if code, _ := h.postForm("/ui/respond", map[string]string{"id": cp.ID.String(), "verb": "approve"}); code != 200 {
		t.Fatal("respond")
	}
	h.eventually("approved on the escalation", func() bool { return h.docState(path) == lifecycle.DocApproved })
	lv, err := store.LastVerdict(ctx, h.srv.Store.Pool, h.docID(path))
	if err != nil || lv.Kind != store.GiverPerson || lv.Via != "escalation" || lv.DispatchID == nil {
		t.Fatalf("escalation answer recorded as %+v (%v)", lv, err)
	}
	if got := verdictSentence(*lv); !strings.HasSuffix(got, "answering the reviewer's escalation.") {
		t.Errorf("escalation answer reads %q", got)
	}
	d, err := store.GetDispatch(ctx, h.srv.Store.Pool, *lv.DispatchID)
	if err != nil || d.Purpose != "review-spec" {
		t.Errorf("the escalation answer names the review it ruled on; got %v (%v)", d, err)
	}

	// A design approved in the web UI, on its page.
	h.writeCommitted("docs/design/solo.md", cascadeDesignV1)
	if code, out := h.call("POST", "/api/docs", map[string]string{
		"path": "docs/design/solo.md", "type": "design", "owner_type": "initiative", "owner_ref": "pf"}); code != 201 {
		t.Fatalf("register: %d %v", code, out)
	}
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": "docs/design/solo.md"}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	doc := h.docAt("docs/design/solo.md")
	if code, _ := h.postForm("/ui/document/review", map[string]string{"doc_id": doc.ID.String(), "decision": "approve"}); code != 200 {
		t.Fatal("approve in the UI")
	}
	lv, err = store.LastVerdict(ctx, h.srv.Store.Pool, doc.ID)
	if err != nil || lv.Kind != store.GiverPerson || lv.Via != "ui" || lv.Actor != h.srv.uiActor() {
		t.Fatalf("UI approval recorded as %+v (%v)", lv, err)
	}
	_, page := h.getUI("/ui/d/docs/design/solo.md")
	mustContain(t, "UI approval", page, "Approved by "+h.srv.uiActor()+".")
	mustContain(t, "API-added design", page, "Added by sam.")
}

// ---- Writers (FR-2.2) ----

// TestWritersRecordWhoAndHow: an authored spec records its role, model and
// run; a starter design made from chat is started by the chat agent; a file
// attached in the web UI is added by the UI actor; and a revision opened by a
// relayed issue is the person's.
func TestWritersRecordWhoAndHow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.onlyWriteSpec()
	h.designedInitiative("alpha")
	h.quiet()

	// An authored spec.
	h.scriptDocument(cascadeSpec("alpha"))
	h.scriptApproval()
	h.send("pf/alpha", false)
	h.eventually("spec approved", func() bool {
		d := h.currentDoc("pf/alpha", "spec")
		return d != nil && d.State == lifecycle.DocApproved
	})
	spec := h.currentDoc("pf/alpha", "spec")
	ws := h.writersOf(spec.ID)
	if len(ws) != 1 || ws[0].Kind != store.WriterAgent || ws[0].Act != store.ActWrote ||
		ws[0].Actor != "spec-author" || ws[0].Model == "" || ws[0].DispatchID == nil {
		t.Fatalf("authored spec writers = %+v", ws)
	}
	_, page := h.getUI("/ui/d/" + spec.Path)
	mustContain(t, "authored spec", page, "Written by the spec author ("+ws[0].Model+").")
	mustContain(t, "run link", page, "/ui/run/"+ws[0].DispatchID.String())

	// A relayed issue on the approved spec opens a revision, credited to the
	// person who raised it, relayed.
	h.pauseAuthor()
	res := h.mustTool("relay_issue", map[string]any{"path": spec.PublicID, "issue": "Add the retry limit.", "quote": "Alpha needs a retry limit."})
	succPath, _ := res["path"].(string)
	succ := h.docAt(succPath)
	if succ == nil || succ.SupersedesID == nil {
		t.Fatalf("the issue should open a revision; got %v", res)
	}
	ws = h.writersOf(succ.ID)
	if len(ws) != 1 || ws[0].Act != store.ActOpenedRevision || ws[0].Kind != store.WriterPerson || ws[0].Via != "mcp" {
		t.Fatalf("revision writers = %+v", ws)
	}
	hist, err := h.srv.writerHistory(ctx, *succ)
	if err != nil {
		t.Fatal(err)
	}
	if got := writerSentence(hist); !strings.HasPrefix(got, "Written by the spec author (") ||
		!strings.Contains(got, "Revision opened by a person, relayed by the chat agent.") {
		t.Errorf("the revision's writer line reads %q", got)
	}
	h.resumeAuthor()

	// A starter design made from chat.
	out := h.mustTool("create_feature", map[string]any{"initiative_path": "pf", "slug": "delta", "name": "Delta"})
	dd := out["design_document"].(map[string]any)
	if got := sentence(dd, "written_by"); got != "Started from the template by the chat agent." {
		t.Errorf("starter design reads %q", got)
	}

	// A file attached in the web UI.
	h.writeCommitted("docs/notes/ui-note.md", "# Notes\n\nWritten by hand.\n")
	f, _ := h.srv.featureByPath(ctx, "pf/delta")
	if code, _ := h.postForm("/ui/entity/attach", map[string]string{
		"file_path": "docs/notes/ui-note.md", "doc_type": "note", "owner_type": "feature", "id": f.ID.String()}); code != 200 {
		t.Fatalf("attach in the UI: %d", code)
	}
	note := h.docAt("docs/notes/ui-note.md")
	if note == nil {
		t.Fatal("the UI attach registered nothing")
	}
	ws = h.writersOf(note.ID)
	if len(ws) != 1 || ws[0].Kind != store.WriterPerson || ws[0].Via != "ui" || ws[0].Actor != h.srv.uiActor() {
		t.Errorf("UI-attached note writers = %+v", ws)
	}
}

// ---- The send screen (FR-5.2) ----

// TestSendScreenWarnsOfAnUnsubmittedDraft: a chat draft nobody has submitted
// is "already written", and its review step says it won't be reviewed until
// someone submits it.
func TestSendScreenWarnsOfAnUnsubmittedDraft(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("alpha")
	h.quiet()
	path := chatSpecPath("alpha")
	h.writeCommitted(path, cascadeSpec("alpha"))
	h.mustTool("attach_document", map[string]any{"path": path, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/alpha"})
	alpha, _ := h.srv.featureByPath(ctx, "pf/alpha")
	_, screen := h.getUI("/ui/send?feature=" + alpha.ID.String())
	mustContain(t, "written by chat", screen, "Already written by the chat agent. This step is skipped.")
	mustContain(t, "draft warning", screen, "This specification is a draft nobody has submitted")
}

// ---- IDs (FR-3) ----

// TestIDsInAndOutOfTheChatTools: relays take document IDs; milestone,
// roadmap and checklist results give MS-, RM- and CL- IDs and still accept
// row ids; the plan section and the checklist page show IDs; /ui/id/CL-…
// redirects.
func TestIDsInAndOutOfTheChatTools(t *testing.T) {
	h := newHarness(t)
	h.designedInitiative("alpha")
	h.quiet()

	ms := h.mustTool("create_milestone", map[string]any{"name": "Beta release"})
	if id, _ := ms["id"].(string); !strings.HasPrefix(id, "MS-") || ms["row_id"] == nil {
		t.Fatalf("milestone result = %v", ms)
	}
	got := h.mustTool("get_milestone", map[string]any{"milestone": ms["row_id"]})
	if got["id"] != ms["id"] {
		t.Errorf("a row id still finds the milestone: %v", got["id"])
	}
	rm := h.mustTool("create_roadmap", map[string]any{"name": "The road"})
	if id, _ := rm["id"].(string); !strings.HasPrefix(id, "RM-") {
		t.Fatalf("roadmap result = %v", rm)
	}
	h.mustTool("place_roadmap_entry", map[string]any{"roadmap": rm["id"], "milestone": ms["id"]})
	road := h.mustTool("get_roadmap", map[string]any{"roadmap": rm["id"]})
	if first := road["milestones"].([]any)[0].(map[string]any); first["id"] != ms["id"] {
		t.Errorf("a roadmap's milestones give their IDs: %v", first)
	}
	cl := h.mustTool("create_checklist", map[string]any{"name": "Launch jobs", "jobs": []any{"Get the key"}})
	clID, _ := cl["id"].(string)
	if !strings.HasPrefix(clID, "CL-") {
		t.Fatalf("checklist result = %v", cl)
	}
	h.mustTool("add_milestone_member", map[string]any{"milestone": ms["id"], "member_type": "checklist", "member": clID})
	h.mustTool("add_milestone_member", map[string]any{"milestone": ms["id"], "member_type": "feature", "member": "pf/alpha"})
	got = h.mustTool("get_milestone", map[string]any{"milestone": ms["id"]})
	for _, raw := range got["members"].([]any) {
		m := raw.(map[string]any)
		switch m["type"] {
		case "checklist":
			if m["id"] != clID || m["row_id"] == nil {
				t.Errorf("checklist item = %v", m)
			}
		case "feature":
			if id, _ := m["id"].(string); !strings.HasPrefix(id, "FEAT-") || m["path"] != "pf/alpha" {
				t.Errorf("feature item = %v", m)
			}
		}
	}
	h.mustTool("relay_tick_job", map[string]any{"checklist": clID, "job": "Get the key", "ticked": true, "quote": "I got the key."})
	if c := h.mustTool("get_checklist", map[string]any{"checklist": clID}); c["done"] != true {
		t.Errorf("ticking by CL- ID: %v", c)
	}

	// Pages and addresses.
	_, plan := h.getUI("/ui/project")
	for _, id := range []string{ms["id"].(string), rm["id"].(string), clID} {
		mustContain(t, "plan section IDs", plan, `<span class="ident">`+id+`</span>`)
	}
	if code, loc := h.redirectOf("/ui/id/" + clID); code != 302 || loc != "/ui/c/"+cl["row_id"].(string) {
		t.Errorf("/ui/id/%s = %d %q", clID, code, loc)
	}
	if code, _ := h.getUI("/ui/id/CL-999"); code != 404 {
		t.Errorf("/ui/id/CL-999 = %d, want 404", code)
	}
	_, clPage := h.getUI("/ui/c/" + cl["row_id"].(string))
	mustContain(t, "checklist heading", clPage, `<span class="ident">`+clID+`</span>Launch jobs`)

	// A relay by a document's ID.
	h.editConfig("  agent: true", "  agent: false")
	path := chatSpecPath("alpha")
	h.writeCommitted(path, cascadeSpec("alpha"))
	ad := h.mustTool("adopt_document", map[string]any{"path": path, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/alpha"})
	h.mustTool("submit_for_review", map[string]any{"document": ad["id"]})
	h.mustTool("relay_verdict", map[string]any{"path": ad["id"], "verdict": "approve", "quote": "Approve it."})
	if got := h.docState(path); got != lifecycle.DocApproved {
		t.Errorf("a relay by ID approves the spec; state = %s", got)
	}
}

// ---- The backfill (FR-2.8, second pass) ----

// TestBackfillAtBootCreditsOlderDocuments: documents from before migration
// 0011 are credited when the server starts, using the configured actor
// names: the chat agent's by the MCP actor, a person's by any other name that
// isn't a role (the CLI's $USER included), marked inferred; a person's bare
// approval is recorded too; and running it again changes nothing.
func TestBackfillAtBootCreditsOlderDocuments(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("alpha") // the design is added and approved by sam, over the API
	h.quiet()
	chatPath := chatSpecPath("alpha")
	h.writeCommitted(chatPath, cascadeSpec("alpha"))
	h.mustTool("attach_document", map[string]any{"path": chatPath, "doc_type": "spec", "owner_type": "feature", "owner_path": "pf/alpha"})

	// Make everything so far older than 0011, with nothing recorded.
	for _, q := range []string{
		`UPDATE schema_migrations SET applied_at = now() + interval '1 hour' WHERE version = 11`,
		`DELETE FROM document_writers`, `DELETE FROM document_verdicts`,
	} {
		if _, err := h.srv.Store.Pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.srv.BackfillProvenance(ctx); err != nil {
		t.Fatal(err)
	}
	design := h.docAt("docs/design/pf.md")
	ws := h.writersOf(design.ID)
	if len(ws) != 1 || ws[0].Kind != store.WriterPerson || ws[0].Actor != "sam" || !ws[0].Inferred {
		t.Errorf("the API-added design's writers = %+v", ws)
	}
	vs := h.verdictsOf(design.ID)
	if len(vs) != 1 || vs[0].Kind != store.GiverPerson || vs[0].Actor != "sam" || !vs[0].Inferred {
		t.Errorf("the design's approval = %+v", vs)
	}
	ws = h.writersOf(h.docID(chatPath))
	if len(ws) != 1 || ws[0].Kind != store.WriterChat || !ws[0].Inferred {
		t.Errorf("the chat-added spec's writers = %+v", ws)
	}
	_, page := h.getUI("/ui/d/" + chatPath)
	mustContain(t, "inferred line", page, "Written by the chat agent (from the audit trail).")

	// Again: nothing more.
	if err := h.srv.BackfillProvenance(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM document_writers) + (SELECT count(*) FROM document_verdicts)`).Scan(&n)
	if n != 3 {
		t.Errorf("a second pass must add nothing; %d rows", n)
	}
}
