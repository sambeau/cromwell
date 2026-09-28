package server

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/store"
)

// SPEC-018: decisions and conventions. Each test drives the real server
// against real Postgres and a real git repository, with the mock provider
// wherever an agent runs, and proves what agents are told by reading the
// run's transcript (SPEC-012), not the planner's internals.

var placeholder = regexp.MustCompile(`(?s)\{\{.*?\}\}`)

// newDecision starts a decision through the web UI's form and returns its
// path. owner is "project" or an initiative's ID.
func (h *harness) newDecision(title, owner string, supersedes ...string) string {
	h.t.Helper()
	v := url.Values{"title": {title}, "owner": {owner}}
	if len(supersedes) > 0 {
		v.Set("supersedes", strings.Join(supersedes, ", "))
	}
	code, loc := h.postValues("/ui/decision/new", v)
	if code != 303 || !strings.HasPrefix(loc, "/ui/edit/") {
		h.t.Fatalf("new decision %q: %d %s", title, code, loc)
	}
	return strings.TrimPrefix(loc, "/ui/edit/")
}

// fillDecision writes the ruling and reason into a decision started from the
// template, and fills the rest of its placeholders.
func (h *harness) fillDecision(path, ruling, reason string) {
	h.t.Helper()
	text := h.readFile(path)
	text = strings.Replace(text, `"{{What was decided, in at most 75 words. This is what agents are told, word for word, so write it as the rule itself.}}"`, quoteYAML(ruling), 1)
	text = strings.Replace(text, `"{{Why, in one line of at most 25 words.}}"`, quoteYAML(reason), 1)
	text = placeholder.ReplaceAllString(text, "Filled in.")
	h.writeFile(path, text)
}

func bodyOf(t *testing.T, raw string) string {
	t.Helper()
	d, err := content.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return d.Body
}

func quoteYAML(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

func (h *harness) writeFile(path, text string) {
	h.t.Helper()
	abs := filepath.Join(h.root, path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(text), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// submit submits a document and fails the test unless it passes validation.
func (h *harness) submitOK(path string) {
	h.t.Helper()
	report, _, err := h.srv.SubmitDoc(context.Background(), path, "sam")
	if err != nil {
		h.t.Fatalf("submit %s: %v", path, err)
	}
	if !report.Valid {
		h.t.Fatalf("submit %s: invalid: %s", path, report.String())
	}
}

// accept submits a decision or the conventions and accepts it as a person.
func (h *harness) accept(path string) {
	h.t.Helper()
	h.submitOK(path)
	if err := h.srv.HumanApproveDocument(context.Background(), h.docAt(path).ID, "sam"); err != nil {
		h.t.Fatalf("accept %s: %v", path, err)
	}
}

// acceptDecision is newDecision, fillDecision and accept together.
func (h *harness) acceptDecision(title, owner, ruling, reason string, supersedes ...string) string {
	h.t.Helper()
	path := h.newDecision(title, owner, supersedes...)
	h.fillDecision(path, ruling, reason)
	h.accept(path)
	return path
}

// told returns the decision lines of a block, in order.
func told(block string) []string {
	var out []string
	for _, l := range strings.Split(block, "\n") {
		if strings.HasPrefix(l, "- DEC-") {
			out = append(out, l)
		}
	}
	return out
}

// reviewPrompt submits the login spec, lets the mock reviewer approve it, and
// returns the user prompt its transcript recorded: what it was told.
func (h *harness) reviewPrompt(specPath string) string {
	h.t.Helper()
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"clear"}`, provider.Usage{Input: 10, Output: 5})
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": specPath}); code != 200 {
		h.t.Fatalf("submit: %d %v", code, out)
	}
	h.eventually("spec approved", func() bool { return h.docState(specPath) == lifecycle.DocApproved })
	runs := h.runsFor("document", h.docID(specPath))
	es := h.transcript(runs[len(runs)-1].ID, 1)
	for _, e := range es {
		if e.Kind == store.EntryPrompt {
			return e.Content
		}
	}
	h.t.Fatal("the transcript has no prompt")
	return ""
}

func (h *harness) initiativePublicID(slug string) string {
	h.t.Helper()
	in, err := store.GetInitiative(context.Background(), h.srv.Store.Pool, h.initiativeID(slug))
	if err != nil {
		h.t.Fatal(err)
	}
	return in.PublicID
}

// TestDispatchIsToldItsBranchsDecisions is DoD 3.1: a dispatch's transcript
// holds exactly the conventions and the accepted decisions on its branch —
// the project's and its initiative's — and none from a sibling initiative,
// none in draft, and none superseded (DoD 3.2).
func TestDispatchIsToldItsBranchsDecisions(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec() // auth → login
	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": "billing", "name": "Billing"}); code != 201 {
		t.Fatalf("create billing: %d %v", code, out)
	}
	auth, billing := h.initiativePublicID("auth"), h.initiativePublicID("billing")

	conv, err := h.srv.StartConventions(context.Background(), "sam", "ui")
	if err != nil {
		t.Fatal(err)
	}
	h.writeFile(conv.Path, "---\ntitle: \"Project conventions\"\ntype: conventions\nowner: project\n"+
		"id: PROJECT-conventions\nrevision: 1\n---\n\n# Project conventions\n\n## Code\n\n- Errors are full sentences.\n")
	h.accept(conv.Path)

	old := h.acceptDecision("Store sessions in memory", "project", "Sessions live in memory.", "Simplest thing.")
	h.acceptDecision("Sessions in Postgres", "project", "Sessions live in Postgres.", "Restarts keep people signed in.", "DEC-001")
	h.acceptDecision("Passwords are hashed with argon2id", auth, "Hash passwords with argon2id.", "The current recommendation.")
	h.acceptDecision("Invoices are immutable", billing, "An issued invoice is never changed.", "Auditors require it.")
	h.newDecision("A draft nobody accepted", "project")

	if got, err := store.CurrentDocumentByPublicID(context.Background(), h.srv.Store.Pool, "DEC-001"); err != nil || got.State != lifecycle.DocSuperseded || got.Path != old {
		t.Errorf("DEC-001 should be superseded where it stands: %+v %v", got, err)
	}

	prompt := h.reviewPrompt(specPath)
	want := []string{
		"- DEC-002: Sessions live in Postgres. Why: Restarts keep people signed in.",
		"- DEC-003 (" + auth + " Authentication): Hash passwords with argon2id. Why: The current recommendation.",
	}
	if got := told(prompt); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("told:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	mustContain(t, "the conventions, headings moved down", prompt, "## Conventions\n\n### Code\n\n- Errors are full sentences.")
	for _, gone := range []string{"memory", "Invoices", "A draft nobody accepted", "DEC-004"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("the prompt should not mention %q", gone)
		}
	}
	// The block follows the project's name and comes before everything else
	// (SD-10).
	if !strings.HasPrefix(prompt, "# Project\n\n") || strings.Index(prompt, "# Project decisions and conventions") > strings.Index(prompt, "# Feature under review") {
		t.Errorf("the block should follow the project's name at the head of the prompt:\n%s", prompt)
	}
}

// TestSupersession is SD-6 and FR-3.5 to FR-3.8: accepting a decision that
// names another supersedes it without touching its file, a narrower owner
// can't supersede a wider one, a superseded ID can't be adopted again, and
// an amendment of a superseded decision can't be accepted.
func TestSupersession(t *testing.T) {
	h := newHarness(t)
	h.setupFeatureWithSpec()
	auth := h.initiativePublicID("auth")
	ctx := context.Background()

	a := h.acceptDecision("Use REST", "project", "The API is REST over JSON.", "Every client speaks it.")
	before := h.readFile(a)

	// An initiative's decision can't retire a project-wide one.
	code, loc := h.postValues("/ui/decision/new", url.Values{"title": {"GraphQL for auth"}, "owner": {auth}, "supersedes": {"DEC-001"}})
	if code != 303 || !strings.Contains(loc, "error=") {
		t.Fatalf("a narrower owner should be refused: %d %s", code, loc)
	}
	if msg, _ := url.QueryUnescape(loc); !strings.Contains(msg, "can't supersede it") {
		t.Errorf("the refusal should say why: %s", msg)
	}

	// An amendment of DEC-001, in review when DEC-001 is superseded.
	code, loc = h.postValues("/ui/document/revise", url.Values{"doc_id": {h.docAt(a).ID.String()}})
	if code != 303 || !strings.HasPrefix(loc, "/ui/edit/") {
		t.Fatalf("append an amendment: %d %s", code, loc)
	}
	amendment := strings.TrimPrefix(loc, "/ui/edit/")
	h.writeFile(amendment, placeholder.ReplaceAllString(h.readFile(amendment), "Clarified"))
	h.submitOK(amendment)

	r1 := h.docAt(a).ID
	b := h.acceptDecision("Use gRPC", "project", "The API is gRPC.", "Streaming.", "DEC-001")
	old, err := store.GetDocument(ctx, h.srv.Store.Pool, r1)
	if err != nil {
		t.Fatal(err)
	}
	if old.State != lifecycle.DocSuperseded || old.Path != a {
		t.Errorf("DEC-001 should be superseded where it stands: %s at %s", old.State, old.Path)
	}
	if h.readFile(a) != before {
		t.Error("a superseded decision's file must not change")
	}
	sups, _ := store.AllSupersessions(ctx, h.srv.Store.Pool)
	if len(sups) != 1 || sups[0].SupersededID != "DEC-001" || sups[0].SupersedingID != "DEC-002" {
		t.Errorf("supersessions: %+v", sups)
	}

	// The amendment in review went back to draft, and can't be submitted or
	// accepted now.
	if st := h.docAt(amendment).State; st != lifecycle.DocDraft {
		t.Errorf("the amendment in review should be back in draft, is %s", st)
	}
	report, _, err := h.srv.SubmitDoc(ctx, amendment, "sam")
	if err != nil || report.Valid || !strings.Contains(report.String(), "DEC-001 was superseded by DEC-002, so it can't be amended") {
		t.Errorf("submitting the amendment should be refused: %v %v", err, report)
	}

	// Detaching the left-over amendment, as the refusal says; its working
	// copy is then deleted.
	if _, err := h.srv.DetachDocument(ctx, h.docAt(amendment).ID, "sam"); err != nil {
		t.Fatalf("detach the amendment: %v", err)
	}
	if err := os.Remove(filepath.Join(h.root, amendment)); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "-A")
	git(t, h.root, "commit", "-qm", "decisions as written")

	// The superseded ID is closed to adoption, by its id: line.
	_, err = h.srv.AdoptDocument(ctx, AdoptRequest{Path: a, DocType: "decision", OwnerType: "project",
		State: lifecycle.DocApproved, Actor: "sam", Via: "ui"})
	if err == nil || !strings.Contains(err.Error(), "DEC-001 was superseded by DEC-002 and is kept as a record") {
		t.Errorf("adopting the superseded file again should be refused: %v", err)
	}
	// ...and by its file name.
	h.writeCommitted("docs/decisions/DEC-001-again.md", "# DEC-001: again\n")
	_, err = h.srv.AdoptDocument(ctx, AdoptRequest{Path: "docs/decisions/DEC-001-again.md", DocType: "decision",
		OwnerType: "project", State: lifecycle.DocApproved, Actor: "sam", Via: "ui"})
	if err == nil || !strings.Contains(err.Error(), "kept as a record") {
		t.Errorf("a file named for the superseded ID should be refused: %v", err)
	}

	// It keeps its page, as a record, and its ID leads there.
	if code, page := h.getUI("/ui/d/" + a); code != 200 || !strings.Contains(page, "so agents are no longer told it") {
		t.Errorf("the superseded decision's page: %d", code)
	}
	if code, loc := h.redirectOf("/ui/id/DEC-001"); code/100 != 3 || loc != "/ui/d/"+a {
		t.Errorf("/ui/id/DEC-001 should lead to its page: %d %s", code, loc)
	}

	// The viewer shows each pointing at the other.
	_, page := h.getUI("/ui/decisions?state=all")
	mustContain(t, "DEC-002 supersedes DEC-001", page, `<tr data-decision="DEC-002">`)
	row := page[strings.Index(page, `<tr data-decision="DEC-001">`):]
	row = row[:strings.Index(row, "</tr>")]
	mustContain(t, "DEC-001 superseded by DEC-002", row, `href="/ui/id/DEC-002"`)
	mustContain(t, "DEC-001's state", row, "superseded")

	// Superseding a decision that is no longer accepted is refused at
	// acceptance.
	c := h.newDecision("Use SOAP", "project")
	h.fillDecision(c, "The API is SOAP.", "Nostalgia.")
	text := strings.Replace(h.readFile(c), "supersedes: []", "supersedes: [DEC-001]", 1)
	h.writeFile(c, text)
	report, _, _ = h.srv.SubmitDoc(ctx, c, "sam")
	if report.Valid || !strings.Contains(report.String(), "DEC-001 isn't an accepted decision") {
		t.Errorf("superseding a superseded decision should fail validation: %v", report)
	}
	_ = b
}

// TestAmendment is SD-5 and FR-3.1 to FR-3.3: an amendment only appends, is
// accepted by a person, restates the ruling for the prompt, and archives the
// previous revision; one that edits the accepted text is refused.
func TestAmendment(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	a := h.acceptDecision("Deploy on Fridays", "project", "Deploys may go out any day.", "Automation makes them safe.")
	accepted := h.readFile(a)

	// The editor refuses an accepted decision, and says what to do.
	_, editor := h.getUI("/ui/edit/" + a)
	mustContain(t, "the editor's refusal", editor, "An accepted decision is never edited. Append a dated amendment, or supersede it with a new decision, from its page.")
	_, page := h.getUI("/ui/d/" + a)
	mustContain(t, "the page offers Append", page, "Append an amendment")
	mustContain(t, "the page offers Supersede", page, "Supersede with a new decision")
	mustContain(t, "the page shows what agents are told", page, "DEC-001: Deploys may go out any day. Why: Automation makes them safe.")

	code, loc := h.postValues("/ui/document/revise", url.Values{"doc_id": {h.docAt(a).ID.String()}})
	if code != 303 {
		t.Fatalf("revise: %d %s", code, loc)
	}
	rev := strings.TrimPrefix(loc, "/ui/edit/")
	skeleton := h.readFile(rev)
	mustContain(t, "the skeleton", skeleton, "## Amendment 1 — {{what changed}} (")

	// Unfilled, it fails on the placeholder.
	if report, _, _ := h.srv.SubmitDoc(ctx, rev, "sam"); report.Valid || !strings.Contains(report.String(), "placeholder") {
		t.Errorf("an unfilled amendment should fail: %v", report)
	}
	// Editing the accepted text is refused.
	edited := strings.Replace(placeholder.ReplaceAllString(skeleton, "Not on Fridays"), "Filled in.", "Rewritten.", 1)
	h.writeFile(rev, edited)
	if report, _, _ := h.srv.SubmitDoc(ctx, rev, "sam"); report.Valid || !strings.Contains(report.String(), "An accepted decision is never edited") {
		t.Errorf("editing the accepted text should be refused: %v", report)
	}
	// Appending, and restating the ruling, passes.
	good := strings.Replace(placeholder.ReplaceAllString(skeleton, "Not on Fridays"),
		`ruling: "Deploys may go out any day."`, `ruling: "Deploys go out Monday to Thursday."`, 1)
	h.writeFile(rev, good)
	h.accept(rev)

	cur, err := store.CurrentDocumentByPublicID(ctx, h.srv.Store.Pool, "DEC-001")
	if err != nil {
		t.Fatal(err)
	}
	if cur.Revision != 2 || cur.Path != a || cur.State != lifecycle.DocApproved {
		t.Errorf("revision 2 should be accepted at the canonical path: %+v", cur)
	}
	if !strings.HasPrefix(bodyOf(t, h.readFile(a)), strings.TrimRight(bodyOf(t, accepted), "\n")) {
		t.Error("the accepted text must be kept")
	}
	if _, err := os.Stat(filepath.Join(h.root, "docs/_superseded/DEC-001.r1.md")); err != nil {
		t.Errorf("revision 1 should be archived: %v", err)
	}
	block, _ := h.srv.surfacedBlock(ctx, surfaceScope{})
	if got := told(block); len(got) != 1 || got[0] != "- DEC-001: Deploys go out Monday to Thursday. Why: Automation makes them safe." {
		t.Errorf("agents should be told the restated ruling: %v", got)
	}
	_, view := h.getUI("/ui/decisions")
	mustContain(t, "the viewer lists the amendment", view, "Amendment 1 — Not on Fridays (")
}

// TestValidationKeepsTheRulingShort is FR-4: an over-long ruling or a
// multi-line reason fails at submit; conventions over their cap fail; the
// routes that skip Submit check the caps; and acceptance refuses a file
// changed after it was submitted.
func TestValidationKeepsTheRulingShort(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	p := h.newDecision("Too long", "project")
	h.fillDecision(p, strings.TrimSpace(strings.Repeat("words ", 200)), "Because.")
	report, _, err := h.srv.SubmitDoc(ctx, p, "sam")
	if err != nil || report.Valid || !strings.Contains(report.String(), "The ruling is 200 words; it can be at most 75") {
		t.Errorf("a 200-word ruling must be refused at submit: %v %v", err, report)
	}
	// Through the page, the same sentence.
	_, page := h.postValues("/ui/document/submit", url.Values{"doc_id": {h.docAt(p).ID.String()}})
	mustContain(t, "the page's refusal", page, "The ruling is 200 words")
	if strings.Contains(page, "..") {
		t.Error("the refusal ends its sentences once")
	}
	// A decision started from the template is submitted, never recorded as
	// already approved; an adopted one may be (SD-3).
	if strings.Contains(page, "This was already approved") {
		t.Error("a template decision shouldn't offer This was already approved")
	}
	h.writeCommitted("docs/decisions/DEC-040-old.md", "# DEC-040: An old ruling\n")
	if _, err := h.srv.AdoptDocument(ctx, AdoptRequest{Path: "docs/decisions/DEC-040-old.md", DocType: "decision",
		OwnerType: "project", State: lifecycle.DocDraft, Actor: "sam", Via: "ui"}); err != nil {
		t.Fatal(err)
	}
	_, page = h.getUI("/ui/d/docs/decisions/DEC-040-old.md")
	mustContain(t, "an adopted decision draft", page, "This was already approved")
	if _, err := h.srv.RecordAlreadyApproved(ctx, h.docAt("docs/decisions/DEC-040-old.md").ID, "sam"); err != nil {
		t.Errorf("record an adopted decision as accepted: %v", err)
	}

	q := h.newDecision("Two-line reason", "project")
	h.fillDecision(q, "Short.", "x")
	h.writeFile(q, strings.Replace(h.readFile(q), `reason: "x"`, "reason: |\n  one\n  two", 1))
	if report, _, _ := h.srv.SubmitDoc(ctx, q, "sam"); report.Valid || !strings.Contains(report.String(), "one line") {
		t.Errorf("a multi-line reason must be refused: %v", report)
	}

	// Accepted text changed on disk after submission is refused at acceptance.
	r := h.newDecision("Edited in review", "project")
	h.fillDecision(r, "Short.", "Why.")
	h.submitOK(r)
	h.writeFile(r, strings.Replace(h.readFile(r), `ruling: "Short."`, `ruling: "`+strings.Repeat("long ", 100)+`"`, 1))
	if err := h.srv.HumanApproveDocument(ctx, h.docAt(r).ID, "sam"); err == nil || !strings.Contains(err.Error(), "changed on disk since it was submitted") {
		t.Errorf("acceptance should refuse an edited file: %v", err)
	}
	if st := h.docAt(r).State; st != lifecycle.DocReviewing {
		t.Errorf("it should still be in review, is %s", st)
	}

	// Adopting as approved checks the caps too.
	h.writeCommitted("docs/decisions/DEC-050-long.md", "---\nruling: \""+strings.Repeat("word ", 90)+"\"\nreason: \"x\"\n---\n\n# DEC-050: Long\n")
	_, err = h.srv.AdoptDocument(ctx, AdoptRequest{Path: "docs/decisions/DEC-050-long.md", DocType: "decision", OwnerType: "project",
		State: lifecycle.DocApproved, Actor: "sam", Via: "ui"})
	if err == nil || !strings.Contains(err.Error(), "can't be recorded as accepted") {
		t.Errorf("adopting an over-long ruling as approved should be refused: %v", err)
	}

	// Conventions over 300 words fail.
	conv, err := h.srv.StartConventions(ctx, "sam", "ui")
	if err != nil {
		t.Fatal(err)
	}
	h.writeFile(conv.Path, "---\ntitle: C\ntype: conventions\nowner: project\nid: PROJECT-conventions\nrevision: 1\n---\n\n# C\n\n- "+strings.Repeat("rule ", 301)+"\n")
	if report, _, _ := h.srv.SubmitDoc(ctx, conv.Path, "sam"); report.Valid || !strings.Contains(report.String(), "at most 300") {
		t.Errorf("long conventions should be refused: %v", report)
	}
}

// TestSurfacingCapIsEnforced is SD-8, SD-9 and FR-6.4, through a real
// dispatch: the count cap admits the nearest and newest, and the prompt names
// what it left out.
func TestSurfacingCapIsEnforced(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	auth := h.initiativePublicID("auth")
	cfg := filepath.Join(h.root, ".subutai/config.yaml")
	data, _ := os.ReadFile(cfg)
	if err := os.WriteFile(cfg, append(data, []byte("\nsurfacing:\n  max_decisions: 2\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	h.acceptDecision("Oldest project rule", "project", "Rule one.", "Why one.")
	h.acceptDecision("Newer project rule", "project", "Rule two.", "Why two.")
	h.acceptDecision("Auth rule", auth, "Rule three.", "Why three.")

	prompt := h.reviewPrompt(specPath)
	want := []string{
		"- DEC-002: Rule two. Why: Why two.",
		"- DEC-003 (" + auth + " Authentication): Rule three. Why: Why three.",
	}
	if got := told(prompt); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("told:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	mustContain(t, "the left-out line", prompt, "Left out for space: DEC-001 (Oldest project rule).")

	_, page := h.getUI("/ui/decisions?owner=" + auth)
	mustContain(t, "the viewer's banner", page, "DEC-001 is left out in")
	mustContain(t, "the row's note", page, "This decision is left out of some dispatches, for lack of space.")
}

// TestThisRepositorysDecisionsAdopt is FR-8: DEC-001 to DEC-007 adopt as they
// are, surface by title in number order, record no supersession, and DEC-008
// is next; Record its ruling works on DEC-001; the boot backfill restores
// their surfaced text; and a hand edit never reaches a prompt.
func TestThisRepositorysDecisionsAdopt(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	files, err := filepath.Glob("../../docs/decisions/DEC-00*.md")
	if err != nil || len(files) != 7 {
		t.Fatalf("expected DEC-001 to DEC-007 in the repository: %v %v", files, err)
	}
	originals := map[string]string{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		path := "docs/decisions/" + filepath.Base(f)
		originals[path] = string(raw)
		h.writeCommitted(path, string(raw))
	}
	for _, f := range files {
		path := "docs/decisions/" + filepath.Base(f)
		// Through the web UI's route, as a person does.
		code, body := h.postValues("/ui/entity/adopt", url.Values{"file_path": {path}, "doc_type": {"decision"},
			"owner_type": {"project"}, "state": {"approved"}})
		if code >= 400 {
			t.Fatalf("adopt %s: %d %s", path, code, body)
		}
		d := h.docAt(path)
		if d == nil || d.State != lifecycle.DocApproved || !strings.HasPrefix(filepath.Base(path), d.PublicID) {
			t.Fatalf("%s should be accepted with its own number: %+v", path, d)
		}
		kept := h.readFile(path)
		if !strings.HasSuffix(kept, originals[path]) {
			t.Errorf("%s changed below its identity lines", path)
		}
	}
	block, err := h.srv.surfacedBlock(ctx, surfaceScope{})
	if err != nil {
		t.Fatal(err)
	}
	lines := told(block)
	if len(lines) != 7 {
		t.Fatalf("all seven should be told:\n%s", block)
	}
	for i, l := range lines {
		id := "DEC-00" + string(rune('1'+i))
		if !strings.HasPrefix(l, "- "+id+": ") || strings.Count(l, id) != 1 {
			t.Errorf("line %d = %q: want %s once, by its title", i, l, id)
		}
	}
	mustContain(t, "DEC-001 by its title", block, "- DEC-001: Server Language — Go")
	if sups, _ := store.AllSupersessions(ctx, h.srv.Store.Pool); len(sups) != 0 {
		t.Errorf("adoption records no supersession, even \"in part\": %+v", sups)
	}
	next := h.mustTool("create_decision", map[string]any{"title": "Next", "owner": "project"})
	if next["id"] != "DEC-008" {
		t.Errorf("the next decision should be DEC-008, got %v", next["id"])
	}

	// A hand edit to an accepted decision never reaches a prompt.
	dec2 := "docs/decisions/DEC-002-postgres-via-supabase.md"
	h.writeFile(dec2, strings.Replace(h.readFile(dec2), "# DEC-002:", "# DEC-002: EDITED", 1))
	if b, _ := h.srv.surfacedBlock(ctx, surfaceScope{}); strings.Contains(b, "EDITED") {
		t.Error("a hand edit reached the prompt")
	}
	git(t, h.root, "checkout", "--", dec2)

	// The boot backfill records what adoption before 0012 would have lacked.
	if _, err := h.srv.Store.Pool.Exec(ctx, `DELETE FROM surfaced_texts`); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.BackfillSurfaced(ctx); err != nil {
		t.Fatal(err)
	}
	if again, _ := h.srv.surfacedBlock(ctx, surfaceScope{}); again != block {
		t.Errorf("the backfill should restore the same block:\n%s", again)
	}

	// Record its ruling on DEC-001.
	dec1 := "docs/decisions/DEC-001-server-language-go.md"
	_, page := h.getUI("/ui/d/" + dec1)
	mustContain(t, "Record its ruling is offered", page, "Record its ruling")
	code, loc := h.postValues("/ui/document/revise", url.Values{"doc_id": {h.docAt(dec1).ID.String()}})
	if code != 303 {
		t.Fatalf("record its ruling: %d %s", code, loc)
	}
	rev := strings.TrimPrefix(loc, "/ui/edit/")
	text := h.readFile(rev)
	text = strings.Replace(text, `ruling: "{{What was decided, in at most 75 words, as agents should be told it.}}"`, `ruling: "The server, CLI and tooling are written in Go."`, 1)
	text = strings.Replace(text, `reason: "{{Why, in one line of at most 25 words.}}"`, `reason: "One static binary, and the team's experience."`, 1)
	h.writeFile(rev, text)
	h.accept(rev)
	block, _ = h.srv.surfacedBlock(ctx, surfaceScope{})
	mustContain(t, "the recorded ruling", block, "- DEC-001: The server, CLI and tooling are written in Go. Why: One static binary, and the team's experience.")
}

// TestOneConventionsDocument is FR-5.2 and FR-5.4.
func TestOneConventionsDocument(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.srv.StartConventions(ctx, "sam", "ui"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.StartConventions(ctx, "sam", "ui"); err == nil || !strings.Contains(err.Error(), "already has its conventions document") {
		t.Errorf("a second should be refused: %v", err)
	}
	h.writeCommitted("docs/more-conventions.md", "---\ntitle: More\ntype: conventions\nowner: project\n---\n\n# More\n")
	if code, out := h.call("POST", "/api/docs", map[string]string{"path": "docs/more-conventions.md", "type": "conventions", "owner_type": "project"}); code < 400 {
		t.Errorf("attaching a second should be refused: %d %v", code, out)
	}
	_, err := h.srv.AdoptDocument(ctx, AdoptRequest{Path: "docs/more-conventions.md", DocType: "conventions", OwnerType: "project",
		State: lifecycle.DocDraft, Actor: "sam", Via: "ui"})
	if err == nil || !strings.Contains(err.Error(), "already has its conventions document") {
		t.Errorf("adopting a second should be refused: %v", err)
	}
}

// TestDecisionToolsOverMCP is FR-2.3, FR-7.3 and FR-7.4.
func TestDecisionToolsOverMCP(t *testing.T) {
	h := newHarness(t)
	h.setupFeatureWithSpec()
	auth := h.initiativePublicID("auth")
	out := h.mustTool("create_decision", map[string]any{"title": "Tokens expire", "owner": auth})
	path, _ := out["path"].(string)
	if out["id"] != "DEC-001" || !strings.HasPrefix(path, "docs/work/"+auth+"-auth/DEC-001-tokens-expire") {
		t.Fatalf("create_decision: %v", out)
	}
	mustContain(t, "what to do next", out["next"].(string), "hand it in with submit_for_review")
	h.fillDecision(path, "Access tokens expire after an hour.", "Stolen tokens stop working.")
	h.mustTool("submit_for_review", map[string]any{"document": "DEC-001"})
	if err := h.srv.HumanApproveDocument(context.Background(), h.docAt(path).ID, "sam"); err != nil {
		t.Fatal(err)
	}
	h.toolRefused("create_decision", map[string]any{"title": "x", "owner": auth, "supersedes": []any{"DEC-404"}}, "DEC-404 isn't an accepted decision")

	list := h.mustTool("list_decisions", map[string]any{"owner": auth})
	ds := list["decisions"].([]any)
	if len(ds) != 1 {
		t.Fatalf("list_decisions: %v", list)
	}
	d := ds[0].(map[string]any)
	if d["id"] != "DEC-001" || d["state"] != "accepted" || d["ruling"] != "Access tokens expire after an hour." ||
		!strings.Contains(d["told_as"].(string), "Why: Stolen tokens stop working.") {
		t.Errorf("entry: %v", d)
	}
	if list["max_tokens"] == nil || list["left_out"] == nil {
		t.Errorf("an owner's list says what its dispatches are told: %v", list)
	}
	got := h.mustTool("get_decision", map[string]any{"id": "DEC-001"})
	if !strings.Contains(got["text"].(string), "## Context") || got["last_verdict"] == nil {
		t.Errorf("get_decision: %v", got)
	}
	h.toolRefused("get_decision", map[string]any{"id": "DEC-099"}, "there is no decision DEC-099")
}

// TestPromptOrderAndExecutionDispatches is SD-9a and SD-10, through a full
// implementation loop on the mock provider: the implementer, the code
// reviewer and the verifier are each told the block, after the project's name
// and before everything else, and the task comes after the shared contract.
func TestPromptOrderAndExecutionDispatches(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	specPath := h.setupFeatureWithSpec()
	h.acceptDecision("Greetings are warm", "project", "Every greeting says welcome.", "People like it.")
	const line = "- DEC-001: Every greeting says welcome. Why: People like it."

	h.approveDoc(specPath)
	h.approveDoc(h.addDevPlan(devPlanOneTask))
	h.eventually("feature ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })
	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n"}`, provider.Usage{Input: 50, Output: 20})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"added greet.go","files_changed":["greet.go"]}`, provider.Usage{Input: 30, Output: 10})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"fine"}`, provider.Usage{Input: 40, Output: 10})
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":true,"evidence":"ok"}],"verdict":"approve","reasoning":"met"}`, provider.Usage{Input: 40, Output: 15})
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("feature done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	prompts := map[string]string{}
	rows, err := h.srv.Store.Pool.Query(ctx, `SELECT id, purpose FROM dispatches`)
	if err != nil {
		t.Fatal(err)
	}
	type run struct {
		id      uuid.UUID
		purpose string
	}
	var runs []run
	for rows.Next() {
		var r run
		if err := rows.Scan(&r.id, &r.purpose); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, r)
	}
	rows.Close()
	for _, r := range runs {
		for _, e := range h.transcript(r.id, 1) {
			if e.Kind == store.EntryPrompt {
				prompts[r.purpose] = e.Content
			}
		}
	}
	inOrder := func(purpose string, parts ...string) {
		t.Helper()
		p, ok := prompts[purpose]
		if !ok {
			t.Fatalf("no %s run was recorded; runs: %v", purpose, runs)
		}
		if !strings.HasPrefix(p, "# Project\n\n") {
			t.Errorf("%s: the prompt should open with the project's name:\n%s", purpose, p)
		}
		last := -1
		for _, part := range parts {
			i := strings.Index(p, part)
			if i < 0 || i < last {
				t.Errorf("%s: %q is missing or out of order:\n%s", purpose, part, p)
				return
			}
			last = i
		}
	}
	inOrder("implement-task", "# Project decisions and conventions", line, "# Specification (the contract)", "# Dev-plan", "# How to work", "# Task to implement")
	inOrder("review-code", "# Project decisions and conventions", line, "# Specification", "# Dev-plan", "# Task under review", "# The diff to review")
	inOrder("verify-feature", "# Project decisions and conventions", line, "# Specification (with acceptance criteria)", "# Feature to verify")
	inOrder("review-spec", "# Project decisions and conventions", line, "# Feature under review", "## Document under review")
}
