package server

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// SPEC-016: editing in the browser. Each test drives the real server against
// real Postgres and a real git repository, through the editor's own routes.

// postValues posts a properly encoded form, as a browser does. A textarea's
// line breaks are sent as CRLF, so the text is sent that way too.
func (h *harness) postValues(path string, v url.Values) (int, string) {
	h.t.Helper()
	resp, err := noRedirect.PostForm(h.api.URL+path, v)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusFound {
		return resp.StatusCode, resp.Header.Get("Location")
	}
	return resp.StatusCode, string(body)
}

// shownAtOpen is the state and path each editor form carried when it was
// opened, by document row id, so a save sends them back as a browser does.
var shownAtOpen = map[string][2]string{}

// editorState opens the editor and returns what its form carries.
func (h *harness) editorState(path string) (docID, baseHash, text string) {
	h.t.Helper()
	code, body := h.getUI("/ui/edit/" + path)
	if code != 200 {
		h.t.Fatalf("editor for %s: %d\n%s", path, code, body)
	}
	docID = formValue(h.t, body, "doc_id")
	baseHash = formValue(h.t, body, "base_hash")
	shownAtOpen[docID] = [2]string{formValue(h.t, body, "shown_state"), formValue(h.t, body, "shown_path")}
	d := h.docAt(path)
	return docID, baseHash, strings.TrimPrefix(h.readFile(d.Path), bom)
}

var hiddenInput = regexp.MustCompile(`<input type="hidden" name="([a-z_]+)" value="([^"]*)">`)

// unescape undoes html/template's escaping of a hidden field's value.
func unescape(s string) string {
	return strings.NewReplacer("&#43;", "+", "&amp;", "&", "&#39;", "'", "&#34;", `"`, "&lt;", "<", "&gt;", ">").Replace(s)
}

func formValue(t *testing.T, body, name string) string {
	t.Helper()
	for _, m := range hiddenInput.FindAllStringSubmatch(body, -1) {
		if m[1] == name {
			return unescape(m[2])
		}
	}
	t.Fatalf("no %s in the form", name)
	return ""
}

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

// save posts the editor form.
func (h *harness) save(docID, baseHash, base, text, action, subject string) (int, string) {
	h.t.Helper()
	shown := shownAtOpen[docID]
	return h.postValues("/ui/edit/save", url.Values{
		"doc_id": {docID}, "base_hash": {baseHash}, "base": {crlf(base)},
		"text": {crlf(text)}, "action": {action}, "subject": {subject},
		"shown_state": {shown[0]}, "shown_path": {shown[1]},
	})
}

// designDraft makes an initiative with a starter design and returns the
// design's path.
func (h *harness) designDraft() string {
	h.t.Helper()
	h.uiCreateInitiative("auth", "Authentication", true)
	in, err := h.srv.Store.InitiativeBySlugPath(context.Background(), []string{"auth"})
	if err != nil {
		h.t.Fatal(err)
	}
	d, err := store.CurrentDocForOwner(context.Background(), h.srv.Store.Pool, "design", "initiative", in.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	if d.PublicID == "" {
		h.t.Fatal("the starter design has no ID")
	}
	return d.Path
}

// fillDesign replaces a starter design's placeholders, so it can be
// submitted, and commits it.
func (h *harness) fillDesign(path string) {
	h.t.Helper()
	filled := regexp.MustCompile(`\{\{[^}]*\}\}`).ReplaceAllString(h.readFile(path), "Filled in.")
	h.writeCommitted(path, filled)
	if err := h.srv.OnPostCommit(context.Background()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) headSHA() string { return strings.TrimSpace(h.gitOut("rev-parse", "HEAD")) }

func TestEditSaveWritesTheFileOnly(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()
	head := h.headSHA()

	code, page := h.getUI("/ui/edit/" + path)
	if code != 200 {
		t.Fatalf("editor: %d", code)
	}
	for _, want := range []string{"This document is a draft, so it is edited in place.", `name="text"`, "Save &amp; commit", "Preview", "id: INIT-001-design"} {
		if !strings.Contains(page, want) {
			t.Fatalf("editor lacks %q", want)
		}
	}
	// The document page offers Edit.
	if _, doc := h.getUI("/ui/d/" + path); !strings.Contains(doc, `href="/ui/edit/`+path+`"`) {
		t.Fatal("the document page has no Edit button")
	}

	id, base, text := h.editorState(path)
	edited := text + "\n## Polished in the browser\n\nA new paragraph.\n"
	code, body := h.save(id, base, text, edited, "save", "")
	if code != 200 || !strings.Contains(body, "Saved. The file is written; it isn&#39;t committed yet.") {
		t.Fatalf("save: %d\n%s", code, body)
	}
	if got := h.readFile(path); got != edited {
		t.Fatalf("file after save: CRLF from the browser must become the file's LF\n%q", got)
	}
	if h.headSHA() != head {
		t.Fatal("Save committed")
	}
	if st := h.gitOut("status", "--porcelain", "--", path); !strings.HasPrefix(st, " M") {
		t.Fatalf("the file should be modified and unstaged: %q", st)
	}
	d := h.docAt(path)
	if h.auditKinds("document.edited", d.ID) != 1 {
		t.Fatal("the save wasn't audited")
	}
	// Who changed the words (SPEC-017 FR-2): the person, in the web UI.
	var revised int
	_ = h.srv.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM document_writers
		WHERE document_id = $1 AND act = 'revised' AND writer_kind = 'person' AND via = 'ui'`, d.ID).Scan(&revised)
	if revised != 1 {
		t.Fatalf("a browser save must record the person as revising the document; got %d", revised)
	}

	// Reindexed at once, with no commit (SD-13, FR-6).
	h.eventually("reindexed after save", func() bool {
		d := h.docAt(path)
		if d.ContentHash != content.Hash([]byte(edited)) {
			return false
		}
		var n int
		_ = h.srv.Store.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM document_sections WHERE document_id = $1 AND heading = 'Polished in the browser'`, d.ID).Scan(&n)
		return n == 1
	})
	if _, doc := h.getUI("/ui/d/" + path); !strings.Contains(doc, "A new paragraph.") {
		t.Fatal("the document page doesn't show the new text")
	}

	// Saving the same text again writes nothing.
	_, base2, _ := h.editorState(path)
	if _, body := h.save(id, base2, edited, edited, "save", ""); !strings.Contains(body, "Nothing changed, so nothing was saved.") {
		t.Fatalf("unchanged save:\n%s", body)
	}
}

func TestEditSaveAndCommitCommitsOneFileAsTheOperator(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()

	// Other work in the tree: a staged file and an unstaged change.
	if err := os.WriteFile(filepath.Join(h.root, "staged.md"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "staged.md")
	if err := os.WriteFile(filepath.Join(h.root, "README.md"), []byte("# test, changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	id, base, text := h.editorState(path)
	edited := strings.Replace(text, "\n# ", "\n# Polished: ", 1)
	if edited == text {
		edited = text + "\nPolished.\n"
	}
	code, body := h.save(id, base, text, edited, "commit", "")
	if code != 200 || !strings.Contains(body, "Saved and committed as ") {
		t.Fatalf("save & commit: %d\n%s", code, body)
	}
	if files := strings.TrimSpace(h.gitOut("show", "--name-only", "--format=", "HEAD")); files != path {
		t.Fatalf("the commit should hold only %s, got %q", path, files)
	}
	if who := strings.TrimSpace(h.gitOut("log", "-1", "--format=%an <%ae> / %cn <%ce>")); who != "operator <operator@localhost> / subutai <subutai@localhost>" {
		t.Fatalf("author and committer: %q", who)
	}
	msg := h.gitOut("log", "-1", "--format=%B")
	if !strings.HasPrefix(msg, "Edit INIT-001-design in the browser\n\n"+path+", draft, revision 1.\nSaved from the Subutai web editor by operator.") {
		t.Fatalf("message:\n%s", msg)
	}
	if st := h.gitOut("status", "--porcelain"); !strings.Contains(st, "A  staged.md") || !strings.Contains(st, " M README.md") {
		t.Fatalf("other work in the tree was disturbed:\n%s", st)
	}
	if st := h.gitOut("status", "--porcelain", "--", path); st != "" {
		t.Fatalf("the edited file should be committed: %q", st)
	}

	// A subject of the person's own.
	_, base2, text2 := h.editorState(path)
	if _, body := h.save(id, base2, text2, text2+"\nMore.\n", "commit", "Tighten the goals"); !strings.Contains(body, "Saved and committed") {
		t.Fatalf("second commit:\n%s", body)
	}
	if subj := strings.TrimSpace(h.gitOut("log", "-1", "--format=%s")); subj != "Tighten the goals" {
		t.Fatalf("subject: %q", subj)
	}
}

func TestEditCommitRefusesSomeoneElsesUncommittedChanges(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()

	// Someone edits the file in a shell and doesn't commit.
	shell := h.readFile(path) + "\nWork in progress from vim.\n"
	if err := os.WriteFile(filepath.Join(h.root, path), []byte(shell), 0o644); err != nil {
		t.Fatal(err)
	}
	head := h.headSHA()
	id, base, text := h.editorState(path)
	code, body := h.save(id, base, text, text+"\nMine.\n", "commit", "")
	if code != 200 || !strings.Contains(body, "has changes that aren&#39;t committed, made outside this editor") {
		t.Fatalf("want the refusal: %d\n%s", code, body)
	}
	if h.readFile(path) != shell || h.headSHA() != head {
		t.Fatal("a refused Save & commit changed something")
	}
	// Save is allowed. But saving over someone else's uncommitted change
	// doesn't make it the editor's own, so Save & commit is still refused
	// (R16-12).
	if _, body := h.save(id, base, text, text+"\nMine.\n", "save", ""); !strings.Contains(body, "Saved. The file is written") {
		t.Fatalf("save:\n%s", body)
	}
	_, base2, text2 := h.editorState(path)
	if _, body := h.save(id, base2, text2, text2, "commit", ""); !strings.Contains(body, "made outside this editor") {
		t.Fatalf("Save then Save & commit got round the check:\n%s", body)
	}
	if h.headSHA() != head {
		t.Fatal("something was committed")
	}

	// On a clean file, the editor's own Save can be committed later.
	git(t, h.root, "checkout", "--", path)
	_, base3, text3 := h.editorState(path)
	if _, body := h.save(id, base3, text3, text3+"\nClean edit.\n", "save", ""); !strings.Contains(body, "Saved. The file is written") {
		t.Fatalf("clean save:\n%s", body)
	}
	_, base4, text4 := h.editorState(path)
	if _, body := h.save(id, base4, text4, text4, "commit", ""); !strings.Contains(body, "Committed as ") {
		t.Fatalf("committing the editor's own save:\n%s", body)
	}
	if h.headSHA() == head {
		t.Fatal("nothing was committed")
	}

	// Then someone edits it in vim: that isn't the editor's any more.
	vim := h.readFile(path) + "\nFrom vim again.\n"
	if err := os.WriteFile(filepath.Join(h.root, path), []byte(vim), 0o644); err != nil {
		t.Fatal(err)
	}
	_, base5, text5 := h.editorState(path)
	if _, body := h.save(id, base5, text5, text5, "commit", ""); !strings.Contains(body, "made outside this editor") {
		t.Fatalf("a vim edit after the editor's save was committed:\n%s", body)
	}

	// An untracked document is refused too.
	if err := os.WriteFile(filepath.Join(h.root, "docs/untracked.md"), []byte("---\ntitle: U\n---\n\n# U\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := h.call("POST", "/api/docs", map[string]string{"path": "docs/untracked.md", "type": "note", "owner_type": "project"}); code != 201 {
		t.Fatalf("register: %d %v", code, out)
	}
	uid, ubase, utext := h.editorState("docs/untracked.md")
	if _, body := h.save(uid, ubase, utext, utext+"\nx\n", "commit", ""); !strings.Contains(body, "made outside this editor") {
		t.Fatalf("an untracked file was committed:\n%s", body)
	}
}

func TestEditConflictWhenTheFileChangedOnDisk(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()
	id, base, text := h.editorState(path)

	// The same file is changed in a shell after the editor opened.
	shell := strings.Replace(text, "\n# ", "\n# Changed in vim: ", 1) + "\nA line from vim.\n"
	if err := os.WriteFile(filepath.Join(h.root, path), []byte(shell), 0o644); err != nil {
		t.Fatal(err)
	}

	// The freshness check says so before the save.
	if _, frag := h.getUI("/ui/frag/edit-fresh?doc_id=" + id + "&base=" + url.QueryEscape(base)); !strings.Contains(frag, "changed on disk since you opened it") {
		t.Fatalf("freshness fragment:\n%s", frag)
	}

	for _, action := range []string{"save", "commit"} {
		head := h.headSHA()
		code, body := h.save(id, base, text, text+"\nA line from the browser.\n", action, "")
		if code != 200 {
			t.Fatalf("%s: %d", action, code)
		}
		for _, want := range []string{
			"Your save was stopped", "What changed on disk", "What you changed",
			"A line from vim.", "A line from the browser.", "Copy your text", "Reload from disk",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: the conflict page lacks %q", action, want)
			}
		}
		if h.readFile(path) != shell {
			t.Fatalf("%s overwrote the change made on disk", action)
		}
		if h.headSHA() != head {
			t.Fatalf("%s committed after a conflict", action)
		}
	}
	d := h.docAt(path)
	if h.auditKinds("document.edited", d.ID) != 0 {
		t.Fatal("a stopped save was recorded as an edit")
	}
}

func TestEditApprovedDocumentStartsARevision(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()
	h.fillDesign(path)
	h.approveDesignAsHuman(path)
	orig := h.docAt(path)

	code, page := h.getUI("/ui/edit/" + path)
	if code != 200 {
		t.Fatalf("editor: %d", code)
	}
	for _, want := range []string{
		"This document is approved.", "Editing it starts a revision", "at revision 2",
		"Approving a revised design starts the cascade", "Start a revision and edit it",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the approved page lacks %q", want)
		}
	}
	if strings.Contains(page, `name="text"`) {
		t.Fatal("an approved document offered a text box")
	}
	if _, doc := h.getUI("/ui/d/" + path); !strings.Contains(doc, "Edit — starts a revision") {
		t.Fatal("the document page doesn't say Edit starts a revision")
	}

	code, loc := h.postValues("/ui/edit/revise", url.Values{"doc_id": {orig.ID.String()}})
	revPath := strings.TrimSuffix(path, ".md") + ".rev.md"
	if code != http.StatusSeeOther || loc != "/ui/edit/"+revPath+"?opened=1" {
		t.Fatalf("revise: %d %s", code, loc)
	}
	succ := h.docAt(revPath)
	if succ.State != lifecycle.DocDraft || succ.PublicID != orig.PublicID || succ.Revision != 2 || succ.SupersedesID == nil || *succ.SupersedesID != orig.ID {
		t.Fatalf("successor: %+v", succ)
	}
	if _, page := h.getUI(loc); !strings.Contains(page, "A revision was opened. The original stays approved until this is.") || !strings.Contains(page, "revision: 2") {
		t.Fatal("the successor's editor didn't open")
	}
	// Edit on the approved document now goes to the open revision.
	if code, to := h.redirectOf("/ui/edit/" + path); code != http.StatusFound || to != "/ui/edit/"+revPath {
		t.Fatalf("open revision redirect: %d %s", code, to)
	}
	if _, doc := h.getUI("/ui/d/" + path); !strings.Contains(doc, "Edit the open revision") {
		t.Fatal("the document page doesn't offer the open revision")
	}

	// The revision's working copy is Subutai's own, so Save & commit takes
	// it and the edit together.
	id, base, text := h.editorState(revPath)
	if _, body := h.save(id, base, text, text+"\nRevised.\n", "commit", ""); !strings.Contains(body, "Saved and committed") {
		t.Fatalf("commit the revision:\n%s", body)
	}
	if h.docAt(path).State != lifecycle.DocApproved {
		t.Fatal("the original should stay approved")
	}
	h.quiet()
	for _, cp := range h.pendingByKind("document-integrity") {
		if cp.RefID == orig.ID {
			t.Fatal("editing the revision raised an integrity question on the original")
		}
	}
}

func TestEditRefusesWhatShouldNotBeEdited(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	// An accepted decision is never edited; an approved note can't be
	// revised yet (SD-12).
	if why := h.srv.reviseRefusal(ctx, &store.Document{Type: "decision"}); !strings.Contains(why, "An accepted decision is never edited.") {
		t.Fatalf("decision: %q", why)
	}
	if why := h.srv.reviseRefusal(ctx, &store.Document{Type: "note"}); !strings.Contains(why, "no template") {
		t.Fatalf("note: %q", why)
	}
	if why := h.srv.editRefusal(ctx, &store.Document{State: lifecycle.DocSuperseded}); why != "A superseded document is a record, so it isn't edited." {
		t.Fatalf("superseded: %q", why)
	}

	// The author agent at work on a spec (SD-11).
	h.designedInitiative("login")
	h.pauseAuthor()
	h.registerDoc("docs/specs/login.md", "spec", "feature", "pf/login", cascadeSpec("login"))
	f, err := h.srv.featureByPath(ctx, "pf/login")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.Store.Pool.Exec(ctx, `INSERT INTO dispatches (id, purpose, role, model, ref_type, ref_id, idempotency_key, state)
		VALUES ($1, 'write-spec', 'spec-author', 'mock', 'feature', $2, 'test-author-at-work', 'running')`, store.NewID(), f.ID); err != nil {
		t.Fatal(err)
	}
	if _, page := h.getUI("/ui/edit/docs/specs/login.md"); !strings.Contains(page, "Its author agent is revising this document now") || strings.Contains(page, `name="text"`) {
		t.Fatal("the editor opened while the author was at work")
	}
}

func TestEditWarningsAndReview(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("login")
	h.pauseAuthor()
	h.registerDoc("docs/specs/login.md", "spec", "feature", "pf/login", cascadeSpec("login"))

	// Before sending: no warnings on the spec.
	if _, page := h.getUI("/ui/edit/docs/specs/login.md"); strings.Contains(page, "edit-warning") {
		t.Fatal("an unsent feature's spec warned")
	}
	h.send("pf/login", false)
	if _, page := h.getUI("/ui/edit/docs/specs/login.md"); !strings.Contains(page, "has been sent to development, and this specification is part of its contract") {
		t.Fatal("no sent warning on the spec")
	}
	// The design the sent feature builds from warns too.
	if _, page := h.getUI("/ui/edit/docs/design/pf.md"); !strings.Contains(page, "builds from this design and has been sent to development") {
		t.Fatal("no builders warning on the design")
	}
	// Being built.
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE features SET state = 'active' WHERE slug = 'login'`); err != nil {
		t.Fatal(err)
	}
	if _, page := h.getUI("/ui/edit/docs/specs/login.md"); !strings.Contains(page, "is being built. Its tasks are given the approved specification") {
		t.Fatal("no building warning on the spec")
	}
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE features SET state = 'idea' WHERE slug = 'login'`); err != nil {
		t.Fatal(err)
	}

	// A design under review: warned, and a save takes it back to draft.
	h.uiCreateInitiative("billing", "Billing", true)
	in, err := h.srv.Store.InitiativeBySlugPath(ctx, []string{"billing"})
	if err != nil {
		t.Fatal(err)
	}
	design, err := store.CurrentDocForOwner(ctx, h.srv.Store.Pool, "design", "initiative", in.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.fillDesign(design.Path)
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": design.Path}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	h.eventually("design in review", func() bool { return h.docState(design.Path) == lifecycle.DocReviewing })
	code, page := h.getUI("/ui/edit/" + design.Path)
	if code != 200 || !strings.Contains(page, "This document is under review. Saving a change takes it back to draft and cancels its review") {
		t.Fatal("no review warning")
	}
	id, base, text := h.editorState(design.Path)
	code, body := h.save(id, base, text, text+"\nA late fix.\n", "save", "")
	if code != 200 || !strings.Contains(body, "It went back to draft, so submit it again when you&#39;re done.") {
		t.Fatalf("save under review: %d\n%s", code, body)
	}
	if h.docState(design.Path) != lifecycle.DocDraft {
		t.Fatal("the design is still under review")
	}
	var withdrawn int
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events
		WHERE ref_id = $1 AND kind = 'document.transition' AND payload->>'event' = 'withdraw'`, design.ID).Scan(&withdrawn); err != nil {
		t.Fatal(err)
	}
	if withdrawn != 1 {
		t.Fatalf("withdrawals audited: %d", withdrawn)
	}
}

func TestEditRefusesToBreakAnID(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()
	id, base, text := h.editorState(path)
	before := h.readFile(path)

	for name, broken := range map[string]string{
		"deleted id":       strings.Replace(text, "id: INIT-001-design\n", "", 1),
		"changed id":       strings.Replace(text, "id: INIT-001-design", "id: INIT-009-design", 1),
		"changed revision": strings.Replace(text, "revision: 1", "revision: 2", 1),
		"deleted both":     strings.Replace(strings.Replace(text, "id: INIT-001-design\n", "", 1), "revision: 1\n", "", 1),
	} {
		if broken == text {
			t.Fatalf("%s: the fixture didn't change", name)
		}
		code, body := h.save(id, base, text, broken, "commit", "")
		if code != 200 || !strings.Contains(body, "This document&#39;s ID is INIT-001-design, at revision 1, and the text you saved doesn&#39;t say so.") {
			t.Fatalf("%s: want the refusal: %d\n%s", name, code, body)
		}
		if h.readFile(path) != before {
			t.Fatalf("%s: the file changed", name)
		}
	}
	if h.docAt(path).PublicID != "INIT-001-design" {
		t.Fatal("the row's ID changed")
	}

	// A document with no ID can't gain one in the editor.
	h.registerDoc("docs/notes/plain.md", "note", "project", "", "---\ntitle: Plain\n---\n\n# Plain\n")
	nid, nbase, ntext := h.editorState("docs/notes/plain.md")
	withID := strings.Replace(ntext, "title: Plain", "title: Plain\nid: PROJECT-note\nrevision: 1", 1)
	if _, body := h.save(nid, nbase, ntext, withID, "save", ""); !strings.Contains(body, "This document has no ID yet, and an ID can&#39;t be added by editing it.") {
		t.Fatalf("adding an ID:\n%s", body)
	}
	if _, body := h.save(nid, nbase, ntext, ntext+"\nFine.\n", "save", ""); !strings.Contains(body, "Saved.") {
		t.Fatalf("a plain edit:\n%s", body)
	}
}

func TestEditPreviewRendersProse(t *testing.T) {
	h := newHarness(t)
	code, body := h.postValues("/ui/edit/preview", url.Values{"text": {"---\r\ntitle: X\r\n---\r\n\r\n# Heading\r\n\r\nSome *prose*.\r\n<script>alert(1)</script>\r\n"}})
	if code != 200 || !strings.Contains(body, "<h1") || !strings.Contains(body, "<em>prose</em>") {
		t.Fatalf("preview: %d\n%s", code, body)
	}
	if strings.Contains(body, "title: X") || strings.Contains(body, "<script>") {
		t.Fatalf("the preview showed front matter or a script:\n%s", body)
	}
}

func TestEditRevisionThroughToApproval(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()
	h.fillDesign(path)
	h.approveDesignAsHuman(path)
	orig := h.docAt(path)
	if _, loc := h.postValues("/ui/edit/revise", url.Values{"doc_id": {orig.ID.String()}}); !strings.Contains(loc, ".rev.md") {
		t.Fatalf("revise: %s", loc)
	}
	revPath := strings.TrimSuffix(path, ".md") + ".rev.md"

	// Something unrelated is staged; the takeover mustn't sweep it in.
	if err := os.WriteFile(filepath.Join(h.root, "staged.md"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "staged.md")

	// Save only: the working copy stays uncommitted.
	id, base, text := h.editorState(revPath)
	revised := text + "\n## Revised in the browser\n\nThe approved text.\n"
	if _, body := h.save(id, base, text, revised, "save", ""); !strings.Contains(body, "Saved. The file is written") {
		t.Fatalf("save:\n%s", body)
	}
	h.approveDesignAsHuman(revPath)

	h.eventually("revision took over", func() bool {
		d, err := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, path)
		return err == nil && d.State == lifecycle.DocApproved && d.Revision == 2
	})
	if got := h.readFile(path); got != revised {
		t.Fatalf("the canonical file should hold the approved revision:\n%s", got)
	}
	files := strings.Fields(h.gitOut("show", "--name-only", "--format=", "HEAD"))
	if len(files) != 2 || !strings.Contains(strings.Join(files, " "), path) || !strings.Contains(strings.Join(files, " "), "docs/_superseded/INIT-001-design.r1.md") {
		t.Fatalf("the takeover commit holds %v", files)
	}
	if committed := h.gitOut("show", "HEAD:"+path); committed != revised {
		t.Fatalf("the takeover committed the wrong text:\n%s", committed)
	}
	if st := h.gitOut("status", "--porcelain", "--", "staged.md"); !strings.HasPrefix(st, "A ") {
		t.Fatalf("the unrelated staged file was swept in: %q", st)
	}
	for _, cp := range h.pendingByKind("document-integrity") {
		t.Fatalf("an integrity question was raised: %s", cp.Question)
	}
}

func TestEditStateChangedWhileOpen(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()
	h.fillDesign(path)

	// Submitted while the editor was open: the save would withdraw it
	// without the person having been told.
	id, base, text := h.editorState(path)
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": path}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	before := h.readFile(path)
	code, body := h.save(id, base, text, text+"\nLate.\n", "save", "")
	if code != 200 || !strings.Contains(body, "This document was submitted for review since you opened the editor") || !strings.Contains(body, "Copy your text") {
		t.Fatalf("submitted while open: %d\n%s", code, body)
	}
	if h.readFile(path) != before || h.docState(path) != lifecycle.DocReviewing {
		t.Fatal("a stale save changed something")
	}

	// Moved while open.
	h.uiCreateInitiative("billing", "Billing", true)
	in, err := h.srv.Store.InitiativeBySlugPath(context.Background(), []string{"billing"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := store.CurrentDocForOwner(context.Background(), h.srv.Store.Pool, "design", "initiative", in.ID)
	if err != nil {
		t.Fatal(err)
	}
	id2, base2, text2 := h.editorState(d.Path)
	git(t, h.root, "mv", d.Path, "docs/billing-design.md")
	git(t, h.root, "commit", "-qm", "move")
	if err := h.srv.OnPostCommit(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, body = h.save(id2, base2, text2, text2+"\nLate.\n", "save", "")
	if !strings.Contains(body, "This document moved to docs/billing-design.md since you opened the editor") {
		t.Fatalf("moved while open:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(h.root, d.Path)); !os.IsNotExist(err) {
		t.Fatal("the save recreated the old path")
	}
}

func TestEditStandsAsideForTheAuthorAndTheInbox(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.designedInitiative("login")
	h.pauseAuthor()
	h.registerDoc("docs/specs/login.md", "spec", "feature", "pf/login", cascadeSpec("login"))
	spec := h.docAt("docs/specs/login.md")
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.InsertIssue(ctx, tx, spec.ID, "sam", "", "Say what happens on a bad password.", "ui", "")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Unsent: nobody will revise it, so the person may, with a warning.
	if _, page := h.getUI("/ui/edit/docs/specs/login.md"); !strings.Contains(page, "no author agent will revise it") || !strings.Contains(page, `name="text"`) {
		t.Fatal("an unsent feature's draft with findings should be editable, with the warning")
	}
	// Sent: the author will revise it, so the editor stands aside.
	h.send("pf/login", false)
	if _, page := h.getUI("/ui/edit/docs/specs/login.md"); !strings.Contains(page, "the author agent will revise it") || strings.Contains(page, `name="text"`) {
		t.Fatal("a sent feature's draft that waits for its author was offered for editing")
	}
	// Unless the Inbox asks whether to allow another round.
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.CreateCheckpoint(ctx, tx, "authoring-deadlock", "document", spec.ID, "Another round?", nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, page := h.getUI("/ui/edit/docs/specs/login.md"); !strings.Contains(page, "the Inbox asks whether to give its author agent another round") || !strings.Contains(page, `name="text"`) {
		t.Fatal("under an authoring-deadlock question the person may edit the draft")
	}

	// A reviewing document with a question in the Inbox is refused.
	h.uiCreateInitiative("billing", "Billing", true)
	in, err := h.srv.Store.InitiativeBySlugPath(ctx, []string{"billing"})
	if err != nil {
		t.Fatal(err)
	}
	design, err := store.CurrentDocForOwner(ctx, h.srv.Store.Pool, "design", "initiative", in.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.fillDesign(design.Path)
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": design.Path}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	h.eventually("in review", func() bool { return h.docState(design.Path) == lifecycle.DocReviewing })
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.CreateCheckpoint(ctx, tx, "review-escalation", "document", design.ID, "Approve?", nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, page := h.getUI("/ui/edit/" + design.Path); !strings.Contains(page, "A question about this document is waiting in the Inbox") || strings.Contains(page, `name="text"`) {
		t.Fatal("a reviewing document with an Inbox question was offered for editing")
	}

	// A revised spec whose submission paused a feature being built.
	f, err := h.srv.featureByPath(ctx, "pf/login")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.CreateCheckpoint(ctx, tx, "revision-in-flight", "feature", f.ID, "Continue?", nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pred := spec.ID
	succ := &store.Document{Type: "spec", State: lifecycle.DocReviewing, OwnerType: "feature", OwnerID: &f.ID, SupersedesID: &pred}
	if why := h.srv.revisionInFlightRefusal(ctx, succ); !strings.Contains(why, "paused new tasks on its feature") {
		t.Fatalf("revision in flight: %q", why)
	}
}

func TestEditKeepsCRLFAndTheByteOrderMarkAndReindexes(t *testing.T) {
	h := newHarness(t)
	body := bom + "---\r\ntitle: Windows note\r\n---\r\n\r\n# Windows note\r\n\r\nFirst.\r\n"
	h.registerDoc("docs/notes/windows.md", "note", "project", "", body)
	id, base, text := h.editorState("docs/notes/windows.md")
	if strings.HasPrefix(text, bom) {
		t.Fatal("the editor should show the file without its byte-order mark")
	}
	edited := strings.ReplaceAll(text, "\r\n", "\n") + "\n## Added\n\nSecond.\n"
	if _, page := h.save(id, base, text, edited, "save", ""); !strings.Contains(page, "Saved.") {
		t.Fatalf("save:\n%s", page)
	}
	got := h.readFile("docs/notes/windows.md")
	want := bom + strings.ReplaceAll(edited, "\n", "\r\n")
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	h.eventually("CRLF file reindexed", func() bool {
		d := h.docAt("docs/notes/windows.md")
		var n int
		_ = h.srv.Store.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM document_sections WHERE document_id = $1 AND heading = 'Added'`, d.ID).Scan(&n)
		return d.ContentHash == content.Hash([]byte(want)) && n == 1
	})
	if _, page := h.getUI("/ui/d/docs/notes/windows.md"); strings.Contains(page, "title: Windows note") {
		t.Log("note: the document page still shows a CRLF file's front matter (M8 follow-up 6)")
	}
}

func TestEditCommitsWithNoGitIdentityAndSurvivesARefusedCommit(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("EMAIL", "")
	git(t, h.root, "config", "--unset", "user.name")
	git(t, h.root, "config", "--unset", "user.email")

	id, base, text := h.editorState(path)
	if _, body := h.save(id, base, text, text+"\nNo identity.\n", "commit", ""); !strings.Contains(body, "Saved and committed") {
		t.Fatalf("no git identity:\n%s", body)
	}
	if who := strings.TrimSpace(h.gitOut("log", "-1", "--format=%an / %cn")); who != "operator / subutai" {
		t.Fatalf("author and committer: %q", who)
	}

	// A pre-commit hook refuses the next commit: the file is saved, the
	// page says so, and nothing is left staged.
	hook := filepath.Join(h.root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	head := h.headSHA()
	_, base2, text2 := h.editorState(path)
	_, body := h.save(id, base2, text2, text2+"\nRefused.\n", "commit", "")
	if !strings.Contains(body, "Your text was saved to the file, but it couldn&#39;t be committed") {
		t.Fatalf("refused commit:\n%s", body)
	}
	if !strings.HasSuffix(h.readFile(path), "\nRefused.\n") || h.headSHA() != head {
		t.Fatal("the file should be saved and nothing committed")
	}
	if staged := strings.TrimSpace(h.gitOut("diff", "--cached", "--name-only")); staged != "" {
		t.Fatalf("left staged: %q", staged)
	}
}

func TestEditRefusesCrossSitePostsAndSymlinks(t *testing.T) {
	h := newHarness(t)
	path := h.designDraft()
	id, base, text := h.editorState(path)

	form := url.Values{"doc_id": {id}, "base_hash": {base}, "text": {text + "\nForged.\n"}, "action": {"commit"}}
	for _, hdr := range []map[string]string{
		{"Origin": "https://evil.example"},
		{"Sec-Fetch-Site": "cross-site"},
	} {
		req, _ := http.NewRequest("POST", h.api.URL+"/ui/edit/save", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%v: %d", hdr, resp.StatusCode)
		}
	}
	if strings.Contains(h.readFile(path), "Forged.") {
		t.Fatal("a cross-site post wrote the file")
	}
	// A same-origin post from the page itself is fine.
	req, _ := http.NewRequest("POST", h.api.URL+"/ui/edit/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", h.api.URL)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !strings.Contains(h.readFile(path), "Forged.") {
		t.Fatal("a same-origin post was refused")
	}

	// A document reached through a symbolic link out of the repository.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.md"), []byte("---\ntitle: X\n---\n\n# X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(h.root, "docs", "linked")); err != nil {
		t.Fatal(err)
	}
	if code, out := h.call("POST", "/api/docs", map[string]string{"path": "docs/linked/x.md", "type": "note", "owner_type": "project"}); code != 201 {
		t.Fatalf("register: %d %v", code, out)
	}
	lid, lbase, ltext := h.editorState("docs/linked/x.md")
	if _, body := h.save(lid, lbase, ltext, ltext+"\nEscaped.\n", "save", ""); !strings.Contains(body, "isn&#39;t a plain file inside the repository") {
		t.Fatalf("symlinked document:\n%s", body)
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "x.md")); strings.Contains(string(b), "Escaped.") {
		t.Fatal("the editor wrote outside the repository")
	}
}

func TestTaskPromptsReadTheApprovedSpecNotAnOpenRevision(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	f, err := h.srv.featureByPath(context.Background(), "auth/login")
	if err != nil {
		t.Fatal(err)
	}
	approved, _, _ := h.srv.contractBodies(context.Background(), f.ID)
	succ, err := h.srv.ReviseDoc(context.Background(), specPath, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, succ.Path), []byte(validSpec+"\nDRAFT TEXT NOT YET APPROVED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, _, _ := h.srv.contractBodies(context.Background(), f.ID)
	if strings.Contains(spec, "DRAFT TEXT") || spec != approved {
		t.Fatal("a task prompt would read the open revision's draft")
	}
}
