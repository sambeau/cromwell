package server

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cromwell/internal/lifecycle"
	"cromwell/internal/provider"
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

// TestUIReadViewsRender covers FR-1.2, FR-2, FR-4, FR-5, FR-6, FR-8.1: every
// view renders on direct load with figures drawn from the store.
func TestUIReadViewsRender(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	// A unit estimate so the planning roll-up shows tokens + tier (FR-4.1).
	if code, _ := h.call("POST", "/api/estimate/set", map[string]any{
		"ref": "auth/login", "tokens": 1200}); code >= 300 {
		t.Fatalf("estimate set: %d", code)
	}
	// A milestone and a roadmap so those sections render (FR-4.2, FR-6.1).
	h.call("POST", "/api/milestones", map[string]string{"name": "v1"})
	h.call("POST", "/api/milestones/members", map[string]string{"milestone": "v1", "ref": "auth/login", "action": "add"})
	h.call("POST", "/api/roadmaps", map[string]string{"name": "2026"})
	h.call("POST", "/api/roadmaps/entries", map[string]any{"roadmap": "2026", "milestone": "v1", "position": 0})

	// FR-1.2: each view renders on direct load.
	for _, v := range []struct{ path, marker string }{
		{"/ui", "Dashboard"},
		{"/ui/inbox", "Inbox"},
		{"/ui/planning", "Initiative tree"},
		{"/ui/documents", "Documents"},
		{"/ui/cost", "Cost"},
	} {
		code, body := h.getUI(v.path)
		if code != 200 {
			t.Fatalf("GET %s: %d", v.path, code)
		}
		mustContain(t, "nav "+v.path, body, `href="/ui"`)
		mustContain(t, v.path, body, v.marker)
	}

	// FR-4.1: the planning tree shows the initiative, feature, and its roll-up.
	_, planning := h.getUI("/ui/planning")
	mustContain(t, "planning initiative", planning, "auth")
	mustContain(t, "planning feature", planning, "login")
	mustContain(t, "planning tier", planning, "tier-") // a tier badge rendered
	mustContain(t, "planning milestone", planning, "v1")
	mustContain(t, "planning roadmap", planning, "2026")

	// FR-5.1: a document renders with a state and (approved) body; here the doc
	// is a registered draft, so its markdown body renders from the working tree.
	_, docs := h.getUI("/ui/documents")
	mustContain(t, "documents list", docs, "login.md")
	code, docBody := h.getUI("/ui/document?path=" + specPath)
	if code != 200 {
		t.Fatalf("GET document: %d", code)
	}
	mustContain(t, "doc body heading", docBody, "<h1>Login form</h1>")
	mustContain(t, "doc body section", docBody, "Overview")
	// CC-5: no edit control on the read view.
	if strings.Contains(docBody, "<textarea") || strings.Contains(docBody, `type="submit">Save`) {
		t.Error("document view must not offer an edit control (CC-5)")
	}

	// FR-6.1: cost view lists the initiative and month sections.
	_, cost := h.getUI("/ui/cost")
	mustContain(t, "cost by initiative", cost, "By initiative")
	mustContain(t, "cost by month", cost, "By month")
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
	// The returned fragment is the refreshed (now clear) inbox.
	mustContain(t, "inbox cleared fragment", after, "Inbox clear")

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
