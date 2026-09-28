package server

import (
	"strings"
	"testing"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// TestBugWorkIsToldItsBranchsDecisions joins M11 and M12, built in parallel:
// a bug is a feature row under an initiative, so the dispatches that work on
// it are told the same decisions as a feature's would be — the project's and
// its initiative's, and none from a sibling initiative (SPEC-018 FR-5 over
// SPEC-019's bug path). Checked on the report's review, the first dispatch a
// sent bug gets.
func TestBugWorkIsToldItsBranchsDecisions(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	h.setupFeatureWithSpec() // auth → login
	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": "billing", "name": "Billing"}); code != 201 {
		t.Fatalf("create billing: %d %v", code, out)
	}
	auth, billing := h.initiativePublicID("auth"), h.initiativePublicID("billing")
	h.acceptDecision("Sessions in Postgres", "project", "Sessions live in Postgres.", "Restarts keep people signed in.")
	h.acceptDecision("Passwords are hashed with argon2id", auth, "Hash passwords with argon2id.", "The current recommendation.")
	h.acceptDecision("Invoices are immutable", billing, "An issued invoice is never changed.", "Auditors require it.")

	bug := h.reportOnInitiative("auth", "Sign-in fails after a restart")
	h.triage(bug.Feature.ID, "accept", nil)
	h.scriptApproval()
	h.send(h.bugPath(bug), false)
	h.eventually("the report approved", h.docStateIs(h.bugPath(bug), "bug_report", lifecycle.DocApproved))

	rep := h.currentDoc(h.bugPath(bug), "bug_report")
	runs := h.runsFor("document", rep.ID)
	if len(runs) == 0 {
		t.Fatal("the report was never reviewed")
	}
	var prompt string
	for _, e := range h.transcript(runs[len(runs)-1].ID, 1) {
		if e.Kind == store.EntryPrompt {
			prompt = e.Content
		}
	}
	want := []string{
		"- DEC-001: Sessions live in Postgres. Why: Restarts keep people signed in.",
		"- DEC-002 (" + auth + " Authentication): Hash passwords with argon2id. Why: The current recommendation.",
	}
	if got := told(prompt); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("a bug's review should be told its branch's decisions:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if strings.Contains(prompt, "Invoices") {
		t.Error("a bug under auth must not be told billing's decisions")
	}
}
