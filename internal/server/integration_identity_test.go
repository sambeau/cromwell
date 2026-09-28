package server

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/store"
)

// SPEC-015: documents with identity. Every test here drives the real server
// against real Postgres and a real git repository, as the other integration
// suites do.

// gitOut runs git in the harness repository and returns its output.
func (h *harness) gitOut(args ...string) string {
	h.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = h.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// writeCommitted writes a file and commits it, as a person would.
func (h *harness) writeCommitted(path, body string) {
	h.t.Helper()
	if err := os.MkdirAll(filepath.Join(h.root, filepath.Dir(path)), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, path), []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
	git(h.t, h.root, "add", "--", path)
	git(h.t, h.root, "commit", "-qm", "write "+path)
}

// readFile reads a repository file.
func (h *harness) readFile(path string) string {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.root, path))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

// uiCreateInitiative creates a top-level initiative from the project page,
// with the design box ticked or not.
func (h *harness) uiCreateInitiative(slug, name string, design bool) string {
	h.t.Helper()
	form := map[string]string{"slug": slug, "name": name, "start_design_offered": "1"}
	if design {
		form["start_design"] = "1"
	}
	code, body := h.postForm("/ui/initiative/new", form)
	if code != 200 {
		h.t.Fatalf("create initiative: %d\n%s", code, body)
	}
	return body
}

// uiCreateFeature creates a feature under an initiative from its page.
func (h *harness) uiCreateFeature(initiativeSlug, slug, name string, design bool) string {
	h.t.Helper()
	in, err := h.srv.Store.InitiativeBySlugPath(context.Background(), []string{initiativeSlug})
	if err != nil {
		h.t.Fatal(err)
	}
	form := map[string]string{"initiative_id": in.ID.String(), "slug": slug, "name": name, "start_design_offered": "1"}
	if design {
		form["start_design"] = "1"
	}
	code, body := h.postForm("/ui/feature/new", form)
	if code != 200 {
		h.t.Fatalf("create feature: %d\n%s", code, body)
	}
	return body
}

func (h *harness) docAt(path string) *store.Document {
	h.t.Helper()
	d, err := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, path)
	if err != nil {
		h.t.Fatalf("document at %s: %v", path, err)
	}
	return d
}

func (h *harness) auditKinds(kind string, ref uuid.UUID) int {
	h.t.Helper()
	var n int
	if err := h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_events WHERE kind = $1 AND ref_id = $2`, kind, ref).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

// noRedirect is a client that reports a redirect rather than following it.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func (h *harness) redirectOf(path string) (int, string) {
	h.t.Helper()
	resp, err := noRedirect.Get(h.api.URL + path)
	if err != nil {
		h.t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Location")
}

// identityDesign is a design that passes the starter manifest, for a test that
// needs to approve one.
func identityDesign(title, owner, id string, revision int) string {
	body := "---\ntitle: " + title + "\ntype: design\nowner: " + owner + "\n---\n\n# " + title +
		"\n\n## What this is for\n\nA reason.\n\n## The shape of it\n\nA shape.\n\n## Decisions\n\n- One, because.\n"
	if id == "" {
		return body
	}
	out, _ := content.SetIdentity(body, id, revision)
	return out
}

// --- Creating work creates its documents (FR-6) ---

func TestCreatingWorkStartsItsDesign(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	body := h.uiCreateInitiative("auth", "Authentication", true)
	mustContain(t, "notice names the ID and the design", body, "INIT-001 Authentication")
	mustContain(t, "notice names the design's path", body, "docs/work/INIT-001-auth/INIT-001-design.md")

	initDesign := "docs/work/INIT-001-auth/INIT-001-design.md"
	d := h.docAt(initDesign)
	if d.PublicID != "INIT-001-design" || d.Revision != 1 || d.State != lifecycle.DocDraft || d.Type != "design" {
		t.Errorf("initiative design = %s r%d %s %s", d.PublicID, d.Revision, d.State, d.Type)
	}
	raw := h.readFile(initDesign)
	if id, rev, ok := content.ReadIdentity(raw); !ok || id != "INIT-001-design" || rev != 1 {
		t.Errorf("front matter identity = %q %d %v", id, rev, ok)
	}
	mustContain(t, "title filled in", raw, `title: "Authentication"`)
	mustContain(t, "owner filled in", raw, `owner: "auth"`)
	mustContain(t, "heading filled in", raw, "# Authentication")
	if content.Hash([]byte(raw)) != d.ContentHash {
		t.Error("the stored hash should be the file's")
	}
	if log := h.gitOut("log", "-1", "--format=%an|%s", "--", initDesign); !strings.HasPrefix(log, "subutai|subutai: start the design of INIT-001") {
		t.Errorf("the design should be committed by subutai; got %q", log)
	}
	if st := h.gitOut("status", "--porcelain", "--", "docs"); strings.TrimSpace(st) != "" {
		t.Errorf("the checkout should be clean after a create; got %q", st)
	}

	h.uiCreateFeature("auth", "login", "Login form", true)
	fd := h.docAt("docs/work/INIT-001-auth/FEAT-001-design.md")
	if fd.PublicID != "FEAT-001-design" || fd.OwnerType != "feature" {
		t.Errorf("feature design = %+v", fd)
	}

	// Opted out: no file, no document.
	h.uiCreateFeature("auth", "later", "Later", false)
	f, _, err := h.srv.featureByRef(ctx, "FEAT-002")
	if err != nil || f.Slug != "later" {
		t.Fatalf("FEAT-002 = %v, %v", f, err)
	}
	if docs, _ := store.DocumentsForOwner(ctx, h.srv.Store.Pool, "feature", &f.ID); len(docs) != 0 {
		t.Errorf("an opted-out feature has documents: %v", docs)
	}
	if _, err := os.Stat(filepath.Join(h.root, "docs/work/INIT-001-auth/FEAT-002-design.md")); !os.IsNotExist(err) {
		t.Error("an opted-out feature should have no design file")
	}

	// Over MCP: on by default, and reported; off when asked.
	out, isErr, msg := h.callTool("create_feature", map[string]any{
		"initiative_path": "INIT-001", "slug": "signup", "name": "Sign up"})
	if isErr {
		t.Fatalf("create_feature: %s", msg)
	}
	if out["id"] != "FEAT-003" {
		t.Errorf("create_feature id = %v", out["id"])
	}
	dd, _ := out["design_document"].(map[string]any)
	if dd["id"] != "FEAT-003-design" || dd["path"] != "docs/work/INIT-001-auth/FEAT-003-design.md" {
		t.Errorf("create_feature design_document = %v", out["design_document"])
	}
	out, isErr, msg = h.callTool("create_initiative", map[string]any{
		"slug": "billing", "name": "Billing", "design_document": false})
	if isErr {
		t.Fatalf("create_initiative: %s", msg)
	}
	if out["id"] != "INIT-002" || out["design_document"] != nil {
		t.Errorf("create_initiative without a design = %v", out)
	}

	// A file already at the starter's path is never written over.
	h.writeCommitted("docs/work/INIT-003-ops/INIT-003-design.md", "# someone else's\n")
	body = h.uiCreateInitiative("ops", "Operations", true)
	mustContain(t, "the refusal says why", body, "write over it")
	if _, err := h.srv.Store.InitiativeBySlugPath(ctx, []string{"ops"}); err == nil {
		t.Error("a refused create should create nothing")
	}
	if got := h.readFile("docs/work/INIT-003-ops/INIT-003-design.md"); got != "# someone else's\n" {
		t.Errorf("the existing file was changed: %q", got)
	}
}

// --- Identity lives in front matter (FR-4) ---

func TestMovedDocumentStaysAttached(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", true)
	h.uiCreateFeature("auth", "login", "Login form", true)

	from := "docs/work/INIT-001-auth/FEAT-001-design.md"
	doc := h.docAt(from)

	// A move to another folder, committed: the hook's notification follows it.
	to := "docs/designs/login.md"
	if err := os.MkdirAll(filepath.Join(h.root, "docs/designs"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "mv", from, to)
	git(t, h.root, "commit", "-qm", "move the login design")
	if err := h.srv.OnPostCommit(ctx); err != nil {
		t.Fatal(err)
	}
	moved, err := store.GetDocument(ctx, h.srv.Store.Pool, doc.ID)
	if err != nil || moved.Path != to {
		t.Fatalf("after the move: %v, %v; want the same row at %s", moved, err, to)
	}
	if n := h.auditKinds("document.moved", doc.ID); n != 1 {
		t.Errorf("document.moved rows = %d", n)
	}
	var count int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE public_id = 'FEAT-001-design'`).Scan(&count)
	if count != 1 {
		t.Errorf("rows with the ID = %d; a move must not register a second document", count)
	}
	_, page := h.getUI("/ui/f/auth/login")
	mustContain(t, "the feature page still shows its design", page, "FEAT-001-design")
	if code, loc := h.redirectOf("/ui/d/" + from); code != http.StatusFound || loc != "/ui/d/"+to {
		t.Errorf("the old address = %d %q; want a redirect to the new one", code, loc)
	}

	// A rename in the same folder.
	git(t, h.root, "mv", to, "docs/designs/login-form.md")
	git(t, h.root, "commit", "-qm", "rename")
	_ = h.srv.OnPostCommit(ctx)
	if d, _ := store.GetDocument(ctx, h.srv.Store.Pool, doc.ID); d.Path != "docs/designs/login-form.md" {
		t.Errorf("after a rename: %s", d.Path)
	}

	// A move while the server wasn't listening (a pull, or a stopped server):
	// the boot and heartbeat check finds it.
	git(t, h.root, "mv", "docs/designs/login-form.md", "docs/login-form.md")
	git(t, h.root, "commit", "-qm", "moved with no hook")
	if err := h.srv.CatchUpScan(ctx); err != nil {
		t.Fatal(err)
	}
	if d, _ := store.GetDocument(ctx, h.srv.Store.Pool, doc.ID); d.Path != "docs/login-form.md" {
		t.Errorf("after an unannounced move: %s", d.Path)
	}

	// A copy isn't a move: the original stays, and the copy is noted once.
	h.writeCommitted("docs/copy.md", h.readFile("docs/login-form.md"))
	_ = h.srv.OnPostCommit(ctx)
	h.writeCommitted("docs/copy.md", h.readFile("docs/copy.md")+"\nmore\n")
	_ = h.srv.OnPostCommit(ctx)
	if d, _ := store.GetDocument(ctx, h.srv.Store.Pool, doc.ID); d.Path != "docs/login-form.md" {
		t.Errorf("a copy moved the document to %s", d.Path)
	}
	if n := h.auditKinds("document.copy_ignored", doc.ID); n != 1 {
		t.Errorf("copy_ignored rows = %d, want 1", n)
	}
}

func TestMovedApprovedDocumentRaisesNoIntegrityQuestion(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", false)
	h.writeCommitted("docs/auth.md", identityDesign("Auth", "auth", "", 0))
	in, _ := h.srv.Store.InitiativeBySlugPath(ctx, []string{"auth"})
	res, err := h.srv.AdoptDocument(ctx, AdoptRequest{Path: "docs/auth.md", DocType: "design",
		OwnerType: "initiative", OwnerID: &in.ID, State: lifecycle.DocApproved, Actor: "sam", Via: "ui"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.State != lifecycle.DocApproved || res.Doc.PublicID != "INIT-001-design" {
		t.Fatalf("adopted = %s %s", res.Doc.PublicID, res.Doc.State)
	}
	// Adoption itself is not drift.
	_ = h.srv.OnPostCommit(ctx)
	if len(h.pendingByKind("document-integrity")) != 0 {
		t.Fatal("adopting an approved document must not raise an integrity question")
	}
	git(t, h.root, "mv", "docs/auth.md", "docs/authentication.md")
	git(t, h.root, "commit", "-qm", "rename the approved design")
	_ = h.srv.OnPostCommit(ctx)
	h.quiet()
	if d, _ := store.GetDocument(ctx, h.srv.Store.Pool, res.Doc.ID); d.Path != "docs/authentication.md" {
		t.Errorf("approved document not followed: %s", d.Path)
	}
	if len(h.pendingByKind("document-integrity")) != 0 {
		t.Error("a move alone is not an edit, so it raises no integrity question")
	}
}

// TestDocumentWithoutIDKeepsPathIdentity is FR-4.4 and NFR-1: attach writes
// nothing, and a moved document with no ID is not followed.
func TestDocumentWithoutIDKeepsPathIdentity(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.setupFeatureWithSpec()
	before := h.readFile("docs/specs/login.md")
	d := h.docAt("docs/specs/login.md")
	if d.PublicID != "" || d.Revision != 0 {
		t.Errorf("an attached document has an ID: %s r%d", d.PublicID, d.Revision)
	}
	if h.readFile("docs/specs/login.md") != before {
		t.Error("attach must not write the file")
	}
	git(t, h.root, "mv", "docs/specs/login.md", "docs/login.md")
	git(t, h.root, "commit", "-qm", "move")
	_ = h.srv.OnPostCommit(ctx)
	_ = h.srv.CatchUpScan(ctx)
	if got, _ := store.GetDocument(ctx, h.srv.Store.Pool, d.ID); got.Path != "docs/specs/login.md" {
		t.Errorf("a document with no ID was followed to %s", got.Path)
	}
	if _, err := store.LiveDocumentByPath(ctx, h.srv.Store.Pool, "docs/login.md"); err != store.ErrNotFound {
		t.Error("the moved file must not be registered as a second document")
	}
}

// --- Adopt in place (FR-5) ---

// TestAdoptThisRepositorysDecisions adopts copies of DEC-001 to DEC-007
// exactly as they are in this repository: no renumbering, only the front
// matter touched, and the sequence moved past them.
func TestAdoptThisRepositorysDecisions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	src, err := filepath.Glob("../../docs/decisions/DEC-00*.md")
	if err != nil || len(src) != 7 {
		t.Fatalf("this repository's decisions: %v, %v", src, err)
	}
	originals := map[string]string{}
	for _, p := range src {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		dst := "docs/decisions/" + filepath.Base(p)
		originals[dst] = string(b)
		h.writeCommitted(dst, string(b))
	}
	for i, dst := range sortedKeys(originals) {
		var id string
		if i%2 == 0 {
			// Over MCP, as a draft.
			out, isErr, msg := h.callTool("adopt_document", map[string]any{
				"path": dst, "doc_type": "decision", "owner_type": "project"})
			if isErr {
				t.Fatalf("adopt_document %s: %s", dst, msg)
			}
			id, _ = out["id"].(string)
			if out["state"] != "draft" {
				t.Errorf("%s over MCP is %v; MCP adopts drafts only", dst, out["state"])
			}
		} else {
			// In the UI, as already approved.
			res, err := h.srv.AdoptDocument(ctx, AdoptRequest{Path: dst, DocType: "decision",
				OwnerType: "project", State: lifecycle.DocApproved, Actor: "sam", Via: "ui"})
			if err != nil {
				t.Fatalf("adopt %s: %v", dst, err)
			}
			id = res.Doc.PublicID
			if h.auditKinds("document.human_verdict", res.Doc.ID) != 1 {
				t.Errorf("%s: adopting as approved should record the person's verdict", dst)
			}
		}
		want := strings.ToUpper(filepath.Base(dst)[:7])
		if id != want {
			t.Errorf("%s adopted as %q, want %s", dst, id, want)
		}
		// Only the front matter changed: two lines in a new block, then the
		// file exactly as it was.
		got := h.readFile(dst)
		if got != "---\nid: "+want+"\nrevision: 1\n---\n\n"+originals[dst] {
			t.Errorf("%s: the file changed beyond its front matter:\n%.200s", dst, got)
		}
		if log := h.gitOut("log", "-1", "--format=%an|%s", "--", dst); !strings.HasPrefix(log, "subutai|subutai: adopt "+dst+" as "+want) {
			t.Errorf("%s committed as %q", dst, log)
		}
		d := h.docAt(dst)
		if !strings.HasPrefix(d.Title, want+":") {
			t.Errorf("%s titled %q; want its first heading", dst, d.Title)
		}
	}
	// The drafts adopted over MCP can be recorded as already approved by a
	// person (FR-5.8).
	d1 := h.docAt("docs/decisions/" + filepath.Base(src[0]))
	if _, err := h.srv.RecordAlreadyApproved(ctx, d1.ID, "sam"); err != nil {
		t.Fatalf("record as already approved: %v", err)
	}
	if d, _ := store.GetDocument(ctx, h.srv.Store.Pool, d1.ID); d.State != lifecycle.DocApproved {
		t.Errorf("recorded as approved, but it is %s", d.State)
	}

	// The next decision continues from them.
	h.writeCommitted("docs/decisions/a-new-idea.md", "# A new idea\n")
	res, err := h.srv.AdoptDocument(ctx, AdoptRequest{Path: "docs/decisions/a-new-idea.md",
		DocType: "decision", OwnerType: "project", State: lifecycle.DocDraft, Actor: "sam", Via: "ui"})
	if err != nil || res.Doc.PublicID != "DEC-008" {
		t.Fatalf("the next decision = %v, %v; want DEC-008", res, err)
	}
	// A second file named for a number already taken is refused, rather than
	// given a number its name contradicts.
	h.writeCommitted("docs/decisions/DEC-005-again.md", "# Again\n")
	if _, err := h.srv.AdoptDocument(ctx, AdoptRequest{Path: "docs/decisions/DEC-005-again.md",
		DocType: "decision", OwnerType: "project", State: lifecycle.DocDraft, Actor: "sam", Via: "ui"}); err == nil ||
		!strings.Contains(err.Error(), "Rename the file") {
		t.Errorf("a taken number in the name: %v", err)
	}
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestAdoptRefusals(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.setupFeatureWithSpec() // auth/login, spec at docs/specs/login.md (attached, draft)
	f, _, _ := h.srv.featureByRef(ctx, "auth/login")
	adopt := func(path, docType, state, via string) error {
		_, err := h.srv.AdoptDocument(ctx, AdoptRequest{Path: path, DocType: docType,
			OwnerType: "feature", OwnerID: &f.ID, State: lifecycle.DocumentState(state), Actor: "sam", Via: via})
		return err
	}
	refused := func(what string, err error, words string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), words) {
			t.Errorf("%s: got %v; want a refusal saying %q", what, err, words)
		}
	}

	// Not committed, or changed since.
	if err := os.WriteFile(filepath.Join(h.root, "docs/untracked.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refused("untracked", adopt("docs/untracked.md", "note", "draft", "ui"), "isn't committed yet")
	h.writeCommitted("docs/notes.md", "# Notes\n")
	if err := os.WriteFile(filepath.Join(h.root, "docs/notes.md"), []byte("# Notes, edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refused("uncommitted changes", adopt("docs/notes.md", "note", "draft", "ui"), "aren't committed")
	git(t, h.root, "checkout", "--", "docs/notes.md")

	// A spec or plan is never adopted as approved, and there is one per feature.
	h.writeCommitted("docs/other-spec.md", "# Other\n")
	refused("approved spec", adopt("docs/other-spec.md", "spec", "approved", "ui"), "adopted as a draft")
	refused("second spec", adopt("docs/other-spec.md", "spec", "draft", "ui"), "already has a spec")
	// Nor over MCP.
	refused("approved over MCP", adopt("docs/notes.md", "note", "approved", "mcp"), "Only a person")
	// A decision doesn't belong to a feature.
	refused("decision on a feature", adopt("docs/notes.md", "decision", "draft", "ui"), "not to a feature")
	// A foreign id: line.
	h.writeCommitted("docs/foreign.md", "---\nid: TICKLY-12\n---\n# Foreign\n")
	refused("foreign id", adopt("docs/foreign.md", "note", "draft", "ui"), "Correct or remove its id: line")

	// A registered document in review.
	h.mock.RespondOutcome("submit_review", `{"verdict":"escalate","reasoning":"unsure"}`, provider.Usage{Input: 10, Output: 10})
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": "docs/specs/login.md"}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	h.eventually("spec in review", func() bool { return h.docState("docs/specs/login.md") == lifecycle.DocReviewing })
	refused("in review", adopt("docs/specs/login.md", "", "", "ui"), "being reviewed")

	// Already has one.
	if err := adopt("docs/notes.md", "note", "draft", "ui"); err != nil {
		t.Fatalf("adopt notes: %v", err)
	}
	refused("already has an ID", adopt("docs/notes.md", "note", "draft", "ui"), "already has an ID")
	h.quiet()
}

// TestAdoptRegisteredDocumentKeepsEverythingElse gives an ID to a document
// attached, approved and revised before it had one (R15-6).
func TestAdoptRegisteredDocumentKeepsEverythingElse(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Auth"})
	h.registerDoc("docs/design/auth.md", "design", "initiative", "auth", identityDesign("Auth", "auth", "", 0))
	h.approveDesignAsHuman("docs/design/auth.md")
	succ, err := h.srv.ReviseDoc(ctx, "docs/design/auth.md", "sam")
	if err != nil {
		t.Fatal(err)
	}
	if succ.PublicID != "" {
		t.Errorf("a document with no ID revises without one; got %s", succ.PublicID)
	}
	// While the revision is open, neither can be adopted.
	if _, err := h.srv.AdoptDocument(ctx, AdoptRequest{Path: "docs/design/auth.md", Actor: "sam", Via: "ui"}); err == nil ||
		!strings.Contains(err.Error(), "revision open") {
		t.Errorf("adopting a document with an open revision: %v", err)
	}
	git(t, h.root, "add", "-A")
	git(t, h.root, "commit", "-qm", "the revision's working copy")
	h.approveDesignAsHuman(succ.Path)
	h.quiet()

	cur := h.docAt("docs/design/auth.md")
	if cur.SupersedesID == nil || cur.State != lifecycle.DocApproved {
		t.Fatalf("the revised design: %+v", cur)
	}
	// Over MCP, a registered approved document may be given an ID (choice 12).
	out, isErr, msg := h.callTool("adopt_document", map[string]any{
		"path": "docs/design/auth.md", "doc_type": "design", "owner_type": "initiative", "owner_path": "auth"})
	if isErr {
		t.Fatalf("adopt_document on the approved design: %s", msg)
	}
	if out["id"] != "INIT-001-design" || out["revision"] != float64(1) || out["state"] != "approved" {
		t.Errorf("adopted = %v", out)
	}
	_ = h.srv.OnPostCommit(ctx)
	h.quiet()
	if len(h.pendingByKind("document-integrity")) != 0 {
		t.Error("giving an approved document its ID must not read as an edit")
	}
}

// TestAdoptedDesignTakesOverFromAnUntouchedStarter is SD-15 and FR-6.6.
func TestAdoptedDesignTakesOverFromAnUntouchedStarter(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", true)
	h.writeCommitted("docs/auth-design.md", identityDesign("The real auth design", "auth", "", 0))
	code, body := h.postForm("/ui/entity/adopt", map[string]string{
		"owner_type": "initiative", "id": h.initiativeID("auth").String(),
		"file_path": "docs/auth-design.md", "doc_type": "design", "state": "approved"})
	if code != 200 {
		t.Fatalf("adopt: %d", code)
	}
	mustContain(t, "the notice says it is the main document now", body, "main document")
	d := h.docAt("docs/auth-design.md")
	if d.PublicID != "INIT-001-design-2" || !d.IsPrimary {
		t.Errorf("adopted design = %s primary=%v; want INIT-001-design-2, primary", d.PublicID, d.IsPrimary)
	}
	_, page := h.getUI("/ui/i/auth")
	mustContain(t, "the page shows the adopted design", page, "The real auth design")

	// A starter someone has written in doesn't give way.
	h.uiCreateInitiative("billing", "Billing", true)
	starter := "docs/work/INIT-002-billing/INIT-002-design.md"
	h.writeCommitted(starter, h.readFile(starter)+"\nSome thinking.\n")
	_ = h.srv.OnPostCommit(ctx)
	h.writeCommitted("docs/billing.md", "# Billing notes\n")
	h.postForm("/ui/entity/adopt", map[string]string{
		"owner_type": "initiative", "id": h.initiativeID("billing").String(),
		"file_path": "docs/billing.md", "doc_type": "design", "state": "draft"})
	if h.docAt("docs/billing.md").IsPrimary {
		t.Error("a starter with a person's writing in it must stay the main document")
	}
}

func (h *harness) initiativeID(slug string) uuid.UUID {
	h.t.Helper()
	in, err := h.srv.Store.InitiativeBySlugPath(context.Background(), []string{slug})
	if err != nil {
		h.t.Fatal(err)
	}
	return in.ID
}

// --- Revisions (FR-3) ---

func TestSuccessorKeepsTheIDAtTheNextRevision(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", true)
	path := "docs/work/INIT-001-auth/INIT-001-design.md"
	h.writeCommitted(path, identityDesign("Authentication", "auth", "INIT-001-design", 1))
	_ = h.srv.OnPostCommit(ctx)
	h.approveDesignAsHuman(path)

	succ, err := h.srv.ReviseDoc(ctx, path, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if succ.PublicID != "INIT-001-design" || succ.Revision != 2 {
		t.Errorf("successor = %s r%d; want INIT-001-design r2", succ.PublicID, succ.Revision)
	}
	if id, rev, _ := content.ReadIdentity(h.readFile(succ.Path)); id != "INIT-001-design" || rev != 2 {
		t.Errorf("working copy says %s r%d", id, rev)
	}
	git(t, h.root, "add", "-A")
	git(t, h.root, "commit", "-qm", "working copy")
	h.approveDesignAsHuman(succ.Path)
	h.quiet()

	archive := "docs/_superseded/INIT-001-design.r1.md"
	if id, rev, ok := content.ReadIdentity(h.readFile(archive)); !ok || id != "INIT-001-design" || rev != 1 {
		t.Errorf("the archive %s says %s r%d %v", archive, id, rev, ok)
	}
	cur := h.docAt(path)
	if cur.Revision != 2 || cur.State != lifecycle.DocApproved {
		t.Errorf("current = r%d %s", cur.Revision, cur.State)
	}
	if _, rev, _ := content.ReadIdentity(h.readFile(path)); rev != 2 {
		t.Errorf("the canonical file says revision %d", rev)
	}
	// An ID with no revision names the current one; with one, that revision
	// (a superseded one has no page, so the current is shown).
	if _, loc := h.redirectOf("/ui/id/INIT-001-design"); loc != "/ui/d/"+path {
		t.Errorf("/ui/id/INIT-001-design -> %q", loc)
	}
	if _, loc := h.redirectOf("/ui/id/init-001-design.r1"); loc != "/ui/d/"+path {
		t.Errorf("/ui/id/init-001-design.r1 -> %q", loc)
	}
}

// TestFreshDocumentAfterSupersessionReusesTheID is SD-6's cascade case: once
// every revision of an ID is superseded, the next document of that owner and
// type takes the same ID at the next revision; a second live one takes -2.
func TestFreshDocumentAfterSupersessionReusesTheID(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", false)
	h.uiCreateFeature("auth", "login", "Login", false)
	f, _, _ := h.srv.featureByRef(ctx, "FEAT-001")
	next := func() (string, int) {
		id, rev, err := store.NextDocumentIdentity(ctx, h.srv.Store.Pool, "feature", &f.ID, "spec")
		if err != nil {
			t.Fatal(err)
		}
		return id, rev
	}
	if id, rev := next(); id != "FEAT-001-spec" || rev != 1 {
		t.Fatalf("first = %s r%d", id, rev)
	}
	var docID uuid.UUID
	_ = h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		d, err := registerInTx(ctx, tx, "docs/s.md", []byte("x"), "spec", "feature", &f.ID, nil, "FEAT-001-spec", 1, "t")
		if err == nil {
			docID = d.ID
		}
		return err
	})
	if id, _ := next(); id != "FEAT-001-spec-2" {
		t.Errorf("with FEAT-001-spec live, the next is %s; want FEAT-001-spec-2", id)
	}
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE documents SET state = 'superseded' WHERE id = $1`, docID); err != nil {
		t.Fatal(err)
	}
	if id, rev := next(); id != "FEAT-001-spec" || rev != 2 {
		t.Errorf("after supersession: %s r%d; want FEAT-001-spec r2", id, rev)
	}
}

// --- Authored documents (SD-16) ---

func TestAuthoredDocumentsGoHomeByID(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", false)
	h.uiCreateFeature("auth", "login", "Login", false)
	f, _, _ := h.srv.featureByRef(ctx, "FEAT-001")
	body := "---\ntitle: Login spec\ntype: spec\nowner: auth/login\n---\n\n# Login spec\n"
	if err := h.srv.fileAuthoredDocument(ctx, f.ID, "spec", body, "spec-author"); err != nil {
		t.Fatal(err)
	}
	d := h.docAt("docs/work/INIT-001-auth/FEAT-001-spec.md")
	if d.PublicID != "FEAT-001-spec" || d.Revision != 1 {
		t.Errorf("authored spec = %s r%d", d.PublicID, d.Revision)
	}
	if id, _, _ := content.ReadIdentity(h.readFile(d.Path)); id != "FEAT-001-spec" {
		t.Errorf("the authored file carries %q", id)
	}

	// A feature that existed before 0010 keeps SPEC-009's path, with an ID.
	h.uiCreateFeature("auth", "old", "Old", false)
	old, _, _ := h.srv.featureByRef(ctx, "FEAT-002")
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE features SET legacy_doc_paths = true WHERE id = $1`, old.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.fileAuthoredDocument(ctx, old.ID, "spec", body, "spec-author"); err != nil {
		t.Fatal(err)
	}
	ld := h.docAt("docs/auth/old/spec.md")
	if ld.PublicID != "FEAT-002-spec" {
		t.Errorf("legacy authored spec = %s", ld.PublicID)
	}
	h.quiet()
}

// --- Detach (SD-13) ---

func TestDetachTakesTheIDOutOfTheFile(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", true)
	path := "docs/work/INIT-001-auth/INIT-001-design.md"
	d := h.docAt(path)
	_, outcome, err := h.srv.detachDocument(ctx, d.ID, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if outcome != "committed" {
		t.Errorf("a clean file's removal should be committed; got %q", outcome)
	}
	if id := content.ReadDeclaredID(h.readFile(path)); id != "" {
		t.Errorf("the detached file still says id: %s", id)
	}
	if st := h.gitOut("status", "--porcelain", "--", "docs"); strings.TrimSpace(st) != "" {
		t.Errorf("the checkout should be clean; got %q", st)
	}
}

// --- Showing IDs (FR-7) ---

func TestIDsOnPagesAndByID(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", true)
	h.uiCreateFeature("auth", "login", "Login form", true)
	f, _, _ := h.srv.featureByRef(ctx, "FEAT-001")
	var task *store.Task
	var ms *store.Milestone
	_ = h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if task, err = store.CreateTask(ctx, tx, f.ID, 0, "T1", "Build the form", "", "t"); err != nil {
			return err
		}
		ms, err = store.CreateMilestone(ctx, tx, "project", nil, "Beta", "", nil, "t")
		return err
	})
	if task.PublicID != "FEAT-001-T01" || ms.PublicID != "MS-001" {
		t.Fatalf("task %s, milestone %s", task.PublicID, ms.PublicID)
	}

	_, page := h.getUI("/ui/f/auth/login")
	mustContain(t, "title", page, "<title>FEAT-001 Login form — Subutai</title>")
	mustContain(t, "header", page, `<span class="ident">FEAT-001</span>Login form`)
	mustContain(t, "breadcrumb", page, `<span class="ident">INIT-001</span>Authentication`)
	_, page = h.getUI("/ui/i/auth")
	mustContain(t, "child list", page, `<span class="ident">FEAT-001</span>Login form`)
	_, page = h.getUI("/ui/documents")
	mustContain(t, "documents page", page, `<span class="ident">INIT-001-design</span>`)
	_, page = h.getUI("/ui/t/" + task.ID.String())
	mustContain(t, "task page", page, `<span class="ident">FEAT-001-T01</span>`)
	_, page = h.getUI("/ui/m/" + ms.ID.String())
	mustContain(t, "milestone page", page, `<span class="ident">MS-001</span>Beta`)

	for id, want := range map[string]string{
		"INIT-001":        "/ui/i/auth",
		"feat-001":        "/ui/f/auth/login",
		"FEAT-001-T01":    "/ui/t/" + task.ID.String(),
		"MS-001":          "/ui/m/" + ms.ID.String(),
		"FEAT-001-design": "/ui/d/docs/work/INIT-001-auth/FEAT-001-design.md",
	} {
		if code, loc := h.redirectOf("/ui/id/" + id); code != http.StatusFound || loc != want {
			t.Errorf("/ui/id/%s = %d %q; want %s", id, code, loc, want)
		}
	}
	if code, _ := h.getUI("/ui/id/FEAT-999"); code != http.StatusNotFound {
		t.Errorf("an unknown ID = %d", code)
	}

	// MCP: IDs accepted where a path is, in capitals, and reported.
	out, isErr, msg := h.callTool("get_feature", map[string]any{"path": "FEAT-001"})
	if isErr || out["path"] != "auth/login" || out["id"] != "FEAT-001" {
		t.Errorf("get_feature by ID = %v %v %s", out, isErr, msg)
	}
	docs := out["documents"].([]any)
	if dd := docs[0].(map[string]any); dd["id"] != "FEAT-001-design" || dd["revision"] != float64(1) || dd["row_id"] == nil {
		t.Errorf("document entry = %v", dd)
	}
	if _, isErr, _ := h.callTool("get_feature", map[string]any{"path": "feat-001"}); !isErr {
		t.Error("a lower-case ID is a path over MCP, and there is no such path")
	}
	out, _, _ = h.callTool("get_tree", nil)
	root := out["initiatives"].([]any)[0].(map[string]any)
	if root["id"] != "INIT-001" || root["features"].([]any)[0].(map[string]any)["id"] != "FEAT-001" {
		t.Errorf("get_tree ids: %v", root)
	}
	if _, err := h.srv.milestoneByRef(ctx, "MS-001"); err != nil {
		t.Errorf("milestone by ID: %v", err)
	}
}

// TestDesignPromptsTakeTheNewestApproved is FR-3.7: a newer draft design
// doesn't hide an approved one from the agents.
func TestDesignPromptsTakeTheNewestApproved(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.uiCreateInitiative("auth", "Authentication", false)
	h.uiCreateFeature("auth", "login", "Login", false)
	h.registerDoc("docs/auth.md", "design", "initiative", "auth", identityDesign("Approved auth", "auth", "", 0))
	h.approveDesignAsHuman("docs/auth.md")
	h.registerDoc("docs/auth-draft.md", "design", "initiative", "auth", "# A newer draft\n")
	f, _, _ := h.srv.featureByRef(ctx, "FEAT-001")
	got, err := h.srv.approvedDesigns(ctx, f)
	if err != nil || len(got) != 1 || got[0].Path != "docs/auth.md" {
		t.Errorf("approved designs = %v, %v; want the approved one", got, err)
	}
	h.quiet()
}
