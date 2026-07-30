package server

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cromwell/internal/lifecycle"
	"cromwell/internal/provider"
	"cromwell/internal/store"
)

// The web command centre handler suite (SPEC-004 NFR-4): the HTML handlers and
// the SSE stream driven directly against real Postgres with the mock provider,
// asserting rendered structure and live-region behaviour, as the phase 1–3
// suites do.

// getUI performs a plain browser-style GET and returns status + body.
func (h *harness) getUI(path string) (int, string) {
	h.t.Helper()
	req, _ := http.NewRequest("GET", h.api.URL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// postForm posts an application/x-www-form-urlencoded body (the browser's form
// encoding) and returns status + body.
func (h *harness) postForm(path string, form map[string]string) (int, string) {
	h.t.Helper()
	vals := make([]string, 0, len(form))
	for k, v := range form {
		vals = append(vals, k+"="+strings.ReplaceAll(v, " ", "+"))
	}
	req, _ := http.NewRequest("POST", h.api.URL+path, strings.NewReader(strings.Join(vals, "&")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func mustContain(t *testing.T, what, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("%s: expected %q in rendered output; got:\n%s", what, want, truncate(body, 800))
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// TestUIBrowseAndDrive is the load-bearing claim of SPEC-007 (§0, FR-1, FR-2,
// FR-3, FR-8): a person can understand and drive the project by browsing it,
// page to page, without typing an entity's path anywhere. It walks the surface
// the way an operator does — project → initiative → feature → document — and
// asserts each page renders from the store with its document as the body.
func TestUIBrowseAndDrive(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	// A unit estimate so the roll-ups show tokens + tier (FR-2.1).
	if code, _ := h.call("POST", "/api/estimate/set", map[string]any{
		"ref": "auth/login", "tokens": 12000}); code >= 300 {
		t.Fatalf("estimate set: %d", code)
	}
	// A milestone the feature belongs to, so the "part of" section renders (FR-8.2).
	h.call("POST", "/api/milestones", map[string]string{"name": "v1"})
	h.call("POST", "/api/milestones/members", map[string]string{"milestone": "v1", "ref": "auth/login", "action": "add"})

	// FR-1.1/FR-1.2: every top-level view and every entity URL renders on a
	// direct load, with the nav rail present.
	for _, v := range []struct{ path, marker string }{
		{"/ui", "Home"},
		{"/ui/project", "Project"},
		{"/ui/inbox", "Inbox"},
		{"/ui/documents", "Documents"},
		{"/ui/work", "Work"},
		{"/ui/i/auth", "Authentication"},
		{"/ui/f/auth/login", "Login form"},
	} {
		code, body := h.getUI(v.path)
		if code != 200 {
			t.Fatalf("GET %s: %d", v.path, code)
		}
		mustContain(t, "nav "+v.path, body, `href="/ui"`)
		mustContain(t, v.path, body, v.marker)
	}

	// FR-2.1: the initiative page lists its feature as a child, with the roll-up
	// from the sizing engine, and links to the feature's own page.
	_, initPage := h.getUI("/ui/i/auth")
	mustContain(t, "child feature", initPage, "Login form")
	mustContain(t, "child link", initPage, `href="/ui/f/auth/login"`)
	mustContain(t, "size tier", initPage, `data-tier=`)
	mustContain(t, "tokens", initPage, "12k")

	// FR-1.2: the feature page's breadcrumb carries its ancestry, each crumb a
	// link to that ancestor's page.
	_, featPage := h.getUI("/ui/f/auth/login")
	mustContain(t, "crumb project", featPage, `href="/ui/project"`)
	mustContain(t, "crumb initiative", featPage, `href="/ui/i/auth"`)
	// FR-8.2: the milestone it is a member of shows as a click-through.
	mustContain(t, "member of milestone", featPage, "v1")
	// FR-2.2: the start action is disabled with a plain-words reason (the spec
	// is not approved, so the feature is still an idea).
	mustContain(t, "gated start", featPage, "disabled")
	// The reason names both halves of the contract. Neither is approved in
	// this fixture, and a reason that named only the spec would leave a
	// reader with an approved spec stuck (see TestFeatureStartReason...).
	mustContain(t, "gate reason", featPage, "approved specification")
	mustContain(t, "gate reason names the dev-plan", featPage, "dev-plan")

	// FR-3.1: the feature has a spec but no design document, so the body is the
	// clear empty state that offers to attach one — not a blank panel.
	mustContain(t, "empty body state", featPage, "no design document here yet")
	// FR-3.2: all its documents are listed regardless.
	mustContain(t, "documents section", featPage, "login.md")

	// FR-1.1: an unknown path is a clear not-found page, not a 500.
	code, nf := h.getUI("/ui/f/auth/nope")
	if code != 404 {
		t.Errorf("unknown feature path: got %d, want 404", code)
	}
	mustContain(t, "not-found copy", nf, "Page not found")

	// FR-1.1: the document is itself a page at a readable URL.
	code, docPage := h.getUI("/ui/d/" + specPath)
	if code != 200 {
		t.Fatalf("GET document page: %d", code)
	}
	mustContain(t, "doc body heading", docPage, "<h1>Login form</h1>")
	mustContain(t, "doc owner crumb", docPage, `href="/ui/f/auth/login"`)

	// NFR-8 / SD-6: no rendered page offers a field into which an entity path is
	// typed. This is the hard rule the redesign exists to enforce.
	for _, p := range []string{"/ui", "/ui/project", "/ui/i/auth", "/ui/f/auth/login", "/ui/work"} {
		_, body := h.getUI(p)
		// No exemptions: the attach-a-document form names its field `file_path`
		// precisely so that `name="path"` can be banned outright.
		for _, banned := range []string{`name="ref"`, `name="path"`, `name="parent_path"`,
			`name="initiative_path"`, `name="milestone"`, `name="roadmap"`} {
			if strings.Contains(body, banned) {
				t.Errorf("%s renders a typed entity-path field %s (SD-6, NFR-8)", p, banned)
			}
		}
	}
}

// TestUIRootRedirectsToHome covers the plain courtesy of the bare address: a
// person who types the host and port lands on Home rather than a 404. The mux
// pattern matches the root exactly, so genuinely unknown paths must still 404
// rather than be swallowed into a redirect.
func TestUIRootRedirectsToHome(t *testing.T) {
	h := newHarness(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(h.api.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/ui" {
		t.Errorf("GET / = %d to %q; want a redirect to /ui", resp.StatusCode, resp.Header.Get("Location"))
	}

	for _, p := range []string{"/nope", "/api/nope"} {
		code, _ := h.getUI(p)
		if code != http.StatusNotFound {
			t.Errorf("GET %s = %d; an unknown path must still be a plain 404", p, code)
		}
	}
}

// TestUIEntityActions covers FR-4, FR-5 and FR-6: the actions relocated onto
// entity pages — editing a description, attaching a document, marking it the
// main one, setting an estimate, and starting a feature — each driving the same
// gated, audited service method the CLI uses, with the entity implied by the
// page rather than typed.
func TestUIEntityActions(t *testing.T) {
	h := newHarness(t)
	h.setupFeatureWithSpec() // auth/login

	ctx := context.Background()
	in, err := h.srv.Store.InitiativeBySlugPath(ctx, []string{"auth"})
	if err != nil {
		t.Fatal(err)
	}
	f, err := h.srv.featureByPath(ctx, "auth/login")
	if err != nil {
		t.Fatal(err)
	}

	// FR-4.1: editing the description persists it, writes an audit row, and the
	// new text appears in the parent's child list.
	desc := "How a person proves who they are when signing in."
	code, body := h.postForm("/ui/entity/describe", map[string]string{
		"ref_type": "feature", "id": f.ID.String(), "description": desc})
	if code != 200 {
		t.Fatalf("describe: %d\n%s", code, body)
	}
	mustContain(t, "description notice", body, "description was updated")
	mustContain(t, "description shown", body, "proves who they are")
	h.eventually("feature.updated audit row", func() bool {
		events, _ := h.srv.Store.AuditTail(ctx, "feature", &f.ID, 50)
		for _, e := range events {
			if e.Kind == "feature.updated" {
				return true
			}
		}
		return false
	})
	_, initPage := h.getUI("/ui/i/auth")
	mustContain(t, "description in child list", initPage, "proves who they are")

	// FR-6.1: attaching a design document makes it appear in the Documents
	// section and become the page's body — the capability that must exist
	// before `cromwell doc add` can be removed (DEC-003).
	designPath := "docs/design/login.md"
	if err := os.MkdirAll(filepath.Join(h.root, "docs/design"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, designPath),
		[]byte("# Login design\n\nTwo fields and a button.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, attached := h.postForm("/ui/entity/attach", map[string]string{
		"owner_type": "feature", "id": f.ID.String(), "file_path": designPath, "doc_type": "design"})
	if code != 200 {
		t.Fatalf("attach: %d\n%s", code, attached)
	}
	mustContain(t, "attach notice", attached, "Document attached")
	mustContain(t, "body is the design doc", attached, "Two fields and a button")
	h.eventually("document.registered audit row", func() bool {
		events, _ := h.srv.Store.AuditTail(ctx, "document", nil, 100)
		for _, e := range events {
			if e.Kind == "document.registered" {
				return true
			}
		}
		return false
	})

	// FR-3.1: marking a different document primary changes the page body.
	spec, err := store.LiveDocumentByPath(ctx, h.srv.Store.Pool, "docs/specs/login.md")
	if err != nil {
		t.Fatal(err)
	}
	_, marked := h.postForm("/ui/document/primary", map[string]string{"doc_id": spec.ID.String()})
	mustContain(t, "primary notice", marked, "main document")
	mustContain(t, "body is now the spec", marked, "Overview")

	// FR-5.1: setting an estimate through the page drives RecordEstimate, and a
	// bad input is surfaced inline rather than as a 500 (FR-2.2).
	_, est := h.postForm("/ui/entity/estimate", map[string]string{
		"ref_type": "feature", "id": f.ID.String(), "tokens": "24000"})
	mustContain(t, "estimate applied", est, "24k")
	mustContain(t, "estimate tier", est, `data-tier="rough"`)
	_, cited := h.postForm("/ui/entity/estimate", map[string]string{
		"ref_type": "feature", "id": f.ID.String(), "tokens": "24000", "cite_corpus": "1"})
	mustContain(t, "considered tier", cited, `data-tier="considered"`)
	code, bad := h.postForm("/ui/entity/estimate", map[string]string{
		"ref_type": "feature", "id": f.ID.String(), "tokens": "0"})
	if code != 200 {
		t.Fatalf("a bad estimate should re-render the page, got %d", code)
	}
	mustContain(t, "inline estimate error", bad, "positive number of tokens")

	// FR-5.1: creating a feature from the initiative page (no path typed).
	_, created := h.postForm("/ui/feature/new", map[string]string{
		"initiative_id": in.ID.String(), "slug": "reset", "name": "Password reset"})
	mustContain(t, "feature created", created, "Password reset")

	// FR-5.1: starting a feature that is not ready returns the gate reason
	// inline, never a 500.
	code, blocked := h.postForm("/ui/feature/start", map[string]string{"id": f.ID.String()})
	if code != 200 {
		t.Fatalf("a blocked start should re-render the page, got %d", code)
	}
	mustContain(t, "start blocked inline", blocked, "not ready")

	// FR-5.1: abandoning requires a reason (DESIGN-003 §6), then succeeds.
	_, noReason := h.postForm("/ui/feature/abandon", map[string]string{"id": f.ID.String()})
	mustContain(t, "abandon needs a reason", noReason, "requires a reason")
	_, abandoned := h.postForm("/ui/feature/abandon", map[string]string{
		"id": f.ID.String(), "reason": "descoped"})
	mustContain(t, "feature abandoned", abandoned, "was abandoned")
	if got := h.featureState("auth/login"); got != lifecycle.FeatAbandoned {
		t.Errorf("feature state = %s, want abandoned", got)
	}
}

// TestUIArchiveRaisesCheckpoint covers FR-5.1: a G5-blocked archive raises a
// gate-override checkpoint to the inbox rather than forcing the archive (L-6) —
// the same behaviour as the CLI, now driven from the initiative's own page.
func TestUIArchiveRaisesCheckpoint(t *testing.T) {
	h := newHarness(t)
	h.setupFeatureWithSpec() // auth/login — a non-terminal feature under auth

	ctx := context.Background()
	in, err := h.srv.Store.InitiativeBySlugPath(ctx, []string{"auth"})
	if err != nil {
		t.Fatal(err)
	}
	_, b := h.postForm("/ui/initiative/archive", map[string]string{"id": in.ID.String(), "reason": "x"})
	mustContain(t, "G5 block message", b, "blocked by gate G5")
	mustContain(t, "routed to inbox", b, "Inbox")

	// The checkpoint is now pending, and the UI badge counts it.
	_, badge := h.getUI("/ui/frag/inbox-badge")
	if strings.TrimSpace(badge) != "1" {
		t.Errorf("inbox badge = %q, want 1 after G5 block", strings.TrimSpace(badge))
	}
	// The initiative is not archived (no override answered yet).
	in, err = h.srv.Store.InitiativeBySlugPath(ctx, []string{"auth"})
	if err != nil {
		t.Fatal(err)
	}
	if in.Archived {
		t.Error("initiative archived without an answered override (L-6 violated)")
	}
}

// TestUIMilestoneAndRoadmapPages covers FR-8.3 and FR-9.1: a milestone renders
// as an unordered checklist with both the ticked count and a token bar, a
// roadmap as an ordered list, and both are reached by click-through from the
// entity that owns or belongs to them — never from a global index.
func TestUIMilestoneAndRoadmapPages(t *testing.T) {
	h := newHarness(t)
	h.setupFeatureWithSpec() // auth/login

	h.call("POST", "/api/estimate/set", map[string]any{"ref": "auth/login", "tokens": 12000})
	h.call("POST", "/api/milestones", map[string]string{"name": "First release"})
	h.call("POST", "/api/milestones/members", map[string]string{
		"milestone": "First release", "ref": "auth/login", "action": "add"})
	h.call("POST", "/api/roadmaps", map[string]string{"name": "The road to version one"})
	h.call("POST", "/api/roadmaps/entries", map[string]any{
		"roadmap": "The road to version one", "milestone": "First release", "position": 0})

	ctx := context.Background()
	m, err := store.MilestoneByName(ctx, h.srv.Store.Pool, "First release")
	if err != nil {
		t.Fatal(err)
	}
	rm, err := store.RoadmapByName(ctx, h.srv.Store.Pool, "The road to version one")
	if err != nil {
		t.Fatal(err)
	}

	// FR-8.3: both the ticked count and the token bar are shown (D-10).
	code, mp := h.getUI("/ui/m/" + m.ID.String())
	if code != 200 {
		t.Fatalf("GET milestone page: %d", code)
	}
	mustContain(t, "milestone name", mp, "First release")
	mustContain(t, "ticked count", mp, "0 of 1")
	mustContain(t, "token figure", mp, "12k")
	mustContain(t, "checklist member", mp, "Login form")
	mustContain(t, "member links through", mp, `href="/ui/f/auth/login"`)

	// FR-9.1: the roadmap is an ordered list of milestones.
	code, rp := h.getUI("/ui/r/" + rm.ID.String())
	if code != 200 {
		t.Fatalf("GET roadmap page: %d", code)
	}
	mustContain(t, "roadmap ordered list", rp, "<ol")
	mustContain(t, "roadmap milestone", rp, "First release")

	// FR-8.1: these are project-owned, so they appear on Home, and no nav
	// element offers an "all milestones" index (DESIGN-008 §5.1a).
	_, home := h.getUI("/ui")
	mustContain(t, "project milestone on Home", home, "First release")
	for _, p := range []string{"/ui", "/ui/project", "/ui/f/auth/login"} {
		_, body := h.getUI(p)
		if strings.Contains(body, `href="/ui/milestones"`) || strings.Contains(body, `href="/ui/roadmaps"`) {
			t.Errorf("%s offers a global milestone/roadmap index; there is none (§5.1a)", p)
		}
	}
}

// TestUIWorkViewIsTokensOnly covers FR-7.2: the former Cost view is now the
// Work view, counted in tokens, and no rendered page shows a currency figure.
func TestUIWorkViewIsTokensOnly(t *testing.T) {
	h := newHarness(t)
	h.setupFeatureWithSpec()
	h.call("POST", "/api/estimate/set", map[string]any{"ref": "auth/login", "tokens": 12000})

	code, work := h.getUI("/ui/work")
	if code != 200 {
		t.Fatalf("GET /ui/work: %d", code)
	}
	mustContain(t, "work in tokens", work, "tokens")
	mustContain(t, "by initiative", work, "By initiative")
	mustContain(t, "by feature", work, "By feature")
	mustContain(t, "the figure", work, "12k")

	// The old money-denominated URL redirects rather than 404s.
	req, _ := http.NewRequest("GET", h.api.URL+"/ui/cost", nil)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("GET /ui/cost = %d, want a redirect to /ui/work", resp.StatusCode)
	}

	// DoD 3: no currency anywhere on the rendered surface.
	for _, p := range []string{"/ui", "/ui/project", "/ui/work", "/ui/i/auth", "/ui/f/auth/login"} {
		_, body := h.getUI(p)
		if strings.Contains(body, "$") {
			t.Errorf("%s renders a currency figure; money is off the human surface (D-4)", p)
		}
	}
}

// TestUIInboxRespond covers FR-3: a raised checkpoint appears in the inbox, the
// badge counts it, and responding through the UI drives the same
// CheckpointResponded path the CLI uses, clearing it.
func TestUIInboxRespond(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	// Escalate a spec review to raise a real inbox checkpoint (as the CLI path).
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"escalate","reasoning":"a product decision"}`,
		provider.Usage{Input: 800, Output: 120})
	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})

	h.eventually("checkpoint in UI inbox", func() bool {
		_, body := h.getUI("/ui/inbox")
		return strings.Contains(body, "review-escalation")
	})
	// FR-3.1: the badge reflects the pending total.
	_, badge := h.getUI("/ui/frag/inbox-badge")
	if strings.TrimSpace(badge) != "1" {
		t.Errorf("inbox badge = %q, want 1", strings.TrimSpace(badge))
	}

	// Find the checkpoint id via the API inbox (the UI embeds it in a hidden field).
	pending := h.callList("GET", "/api/inbox")
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	id := pending[0]["ID"].(string)

	// FR-3.2: respond approve through the UI form; the same path advances the doc.
	code, after := h.postForm("/ui/respond", map[string]string{"id": id, "verb": "approve", "reason": "fine"})
	if code != 200 {
		t.Fatalf("ui respond: %d\n%s", code, after)
	}
	// The returned fragment is the refreshed (now clear) inbox. Assert on the
	// empty state's title rather than its prose, which is written for a human
	// and will be reworded again.
	mustContain(t, "inbox cleared fragment", after, "Nothing is waiting for you")

	h.eventually("doc approved after UI respond", func() bool {
		return h.docState(specPath) == lifecycle.DocApproved
	})
}

// TestUISSEDelivers covers FR-7 and FR-8.2: the SSE stream delivers a live
// signal when a checkpoint is raised, without a reload.
func TestUISSEDelivers(t *testing.T) {
	h := newHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", h.api.URL+"/ui/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("SSE content-type = %q", ct)
	}

	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()

	// Wait for the stream to open.
	waitFor(t, lines, func(l string) bool { return strings.Contains(l, "connected") })

	// Raise a checkpoint: create a feature under an initiative, then archive the
	// initiative — G5 blocks and raises a gate-override checkpoint synchronously,
	// firing the checkpoint-raised hub signal (FR-8.2).
	h.setupFeatureWithSpec()
	code, _ := h.call("POST", "/api/initiatives/archive", map[string]string{"path": "auth", "reason": "x"})
	if code != 409 {
		t.Fatalf("archive should be blocked by G5: %d", code)
	}

	// FR-7.1 / FR-8.2: a `changed` event carrying checkpoint.raised arrives.
	waitFor(t, lines, func(l string) bool { return strings.Contains(l, "checkpoint.raised") })
}

func waitFor(t *testing.T, lines <-chan string, cond func(string) bool) {
	t.Helper()
	deadline := time.After(12 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("SSE stream closed before condition met")
			}
			if cond(l) {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for SSE line")
		}
	}
}

// TestUIDocumentReview covers FR-11: approving an escalated review from the
// document's own page drives the same CheckpointResponded path as the inbox,
// and the document advances. The document is identified by its id, carried by
// the page — no path is typed (SD-6).
func TestUIDocumentReview(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	h.mock.RespondOutcome("submit_review",
		`{"verdict":"escalate","reasoning":"a product decision"}`,
		provider.Usage{Input: 800, Output: 120})
	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})

	// The document page offers the review controls once it is human-gated.
	h.eventually("review controls on the document page", func() bool {
		_, body := h.getUI("/ui/d/" + specPath)
		return strings.Contains(body, "waiting for your review")
	})

	doc, err := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, specPath)
	if err != nil {
		t.Fatal(err)
	}
	code, after := h.postForm("/ui/document/review", map[string]string{
		"doc_id": doc.ID.String(), "decision": "approve"})
	if code != 200 {
		t.Fatalf("document review: %d\n%s", code, after)
	}
	mustContain(t, "approve notice", after, "was approved")

	// The same audited outcome as the inbox respond: the document advances.
	h.eventually("doc approved after document-page review", func() bool {
		return h.docState(specPath) == lifecycle.DocApproved
	})
}

// TestUIServesStaticAssets confirms the vendored HTMX + SSE assets are embedded
// and served (DESIGN-007 CC-1).
func TestUIServesStaticAssets(t *testing.T) {
	h := newHarness(t)
	for _, asset := range []string{"/ui/static/htmx.min.js", "/ui/static/sse.js", "/ui/static/app.css"} {
		code, body := h.getUI(asset)
		if code != 200 {
			t.Errorf("GET %s: %d", asset, code)
		}
		if len(body) == 0 {
			t.Errorf("GET %s: empty", asset)
		}
	}
}
