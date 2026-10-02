package server

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

func renderNamed(t *testing.T, name string, data any) string {
	t.Helper()
	tmpl, err := loadUITemplates()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.t.ExecuteTemplate(&buf, name, data); err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return buf.String()
}

func htmlHas(t *testing.T, html string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(html, w) {
			t.Errorf("missing %q in:\n%s", w, html)
		}
	}
}

func htmlLacks(t *testing.T, html string, bad ...string) {
	t.Helper()
	for _, w := range bad {
		if strings.Contains(html, w) {
			t.Errorf("unexpected %q in:\n%s", w, html)
		}
	}
}

// TestTaskExecutorRenders is SPEC-020 FR-1.5 and FR-8.4: the executor line, the
// unmeasured sentence for work done in chat or by a person, and the reviewer's
// model for the chat agent's work.
func TestTaskExecutorRenders(t *testing.T) {
	nobody := renderNamed(t, "task-executor", executorView{
		Line: ExecutorLine{Sentence: "Nobody has started this task yet.", Measured: true}})
	htmlHas(t, nobody, "Nobody has started this task yet.")
	htmlLacks(t, nobody, "aren&#39;t measured", "Reviewed with")

	agent := renderNamed(t, "task-executor", executorView{
		Line:   ExecutorLine{Sentence: "Implemented by the implementer (claude-sonnet-5).", Kind: "agent", Measured: true},
		RunURL: "/ui/run/abc"})
	htmlHas(t, agent, "Implemented by the implementer (claude-sonnet-5).", `href="/ui/run/abc"`, "#i-agent")
	htmlLacks(t, agent, "aren&#39;t measured")

	chat := renderNamed(t, "task-executor", executorView{
		Line:         ExecutorLine{Sentence: "Implemented by the chat agent.", Kind: "chat"},
		Unmeasured:   true,
		ReviewedWith: "claude-opus-5"})
	htmlHas(t, chat, "Implemented by the chat agent.",
		"This work was done in chat or by a person, so its tokens aren&#39;t measured.",
		"Reviewed with <code>claude-opus-5</code>, the project&#39;s reviewer for the chat agent&#39;s work.")
}

// TestTaskClaimPanelRenders is SPEC-020 FR-4.1: each of the panel's six states
// has its sentence and its buttons, and no others.
func TestTaskClaimPanelRenders(t *testing.T) {
	now := time.Now()
	claim := func(kind string, state lifecycle.ClaimState) *store.Claim {
		return &store.Claim{ID: uuid.New(), Kind: kind, Actor: "sam", State: state,
			ClaimedAt: now.Add(-3 * time.Hour), LastActivityAt: now.Add(-time.Hour), LastActivity: "the working copy changed"}
	}
	base := claimPanel{TaskID: "FEAT-001-T01"}

	t.Run("claimable", func(t *testing.T) {
		p := base
		p.State = claimPanelClaimable
		html := renderNamed(t, "task-claim", p)
		htmlHas(t, html,
			"You can implement this task yourself instead of an agent. Claiming it pauses this feature&#39;s agents until you submit it.",
			"Claim this task", `action="/ui/task/claim"`, `value="FEAT-001-T01"`)
		htmlLacks(t, html, "Release", "Submit for code review")
	})

	t.Run("refused", func(t *testing.T) {
		p := base
		p.State, p.Refusal = claimPanelRefused, "FEAT-001-T01 can't be claimed yet: it waits for T00 to be done."
		html := renderNamed(t, "task-claim", p)
		htmlHas(t, html, "can&#39;t be claimed yet: it waits for T00 to be done.")
		htmlLacks(t, html, "<button", "<form")
	})

	t.Run("a person's open claim", func(t *testing.T) {
		p := base
		p.State, p.Claim, p.Holder, p.Own = claimPanelPerson, claim("person", lifecycle.ClaimOpen), "sam", true
		p.Path, p.Branch, p.SpecURL, p.PlanURL = "/work/feat-001", "feature/login", "/ui/d/specs/a.md", "/ui/d/plans/a.md"
		p.Comments = &ReviewComments{Comments: []ReviewFinding{{Severity: "major", File: "a.go", Line: 12, Text: "Check the error."}}}
		html := renderNamed(t, "task-claim", p)
		htmlHas(t, html, "Claimed by sam", "3h ago", "Last activity: the working copy changed",
			"<code>/work/feat-001</code>", "<code>feature/login</code>",
			`href="/ui/d/specs/a.md"`, `href="/ui/d/plans/a.md"`,
			"What the code reviewer asked for", "<code>a.go:12</code>", "Check the error.",
			`name="summary"`, "required", "Submit for code review", `action="/ui/task/submit"`,
			`action="/ui/task/release"`, "Release", "I&#39;m still working on this")
		htmlLacks(t, html, "Resume", "Claim this task")
	})

	t.Run("a returned claim of the person's own", func(t *testing.T) {
		p := base
		p.State, p.Claim, p.Holder, p.Own = claimPanelReturned, claim("person", lifecycle.ClaimReturned), "sam", true
		p.Comments = &ReviewComments{Comments: []ReviewFinding{{Severity: "minor", Text: "Rename it."}}}
		html := renderNamed(t, "task-claim", p)
		htmlHas(t, html, "Rename it.", "Resume", "Release", `action="/ui/task/claim"`)
		htmlLacks(t, html, "Submit for code review")
	})

	t.Run("a returned claim of another person's", func(t *testing.T) {
		p := base
		p.State, p.Claim, p.Holder = claimPanelReturned, claim("person", lifecycle.ClaimReturned), "pat"
		html := renderNamed(t, "task-claim", p)
		htmlHas(t, html, "Release")
		htmlLacks(t, html, "Resume")
	})

	t.Run("the chat agent's claim", func(t *testing.T) {
		p := base
		p.State, p.Claim, p.Holder = claimPanelChat, claim("chat", lifecycle.ClaimOpen), "the chat agent"
		p.Path = "/work/feat-001"
		html := renderNamed(t, "task-claim", p)
		htmlHas(t, html, "Claimed by the chat agent", "3h ago", "<code>/work/feat-001</code>",
			"The chat agent is working on this. Releasing it gives the task to an agent, and keeps what is in the working copy.",
			"Release", `action="/ui/task/release"`)
		htmlLacks(t, html, "Resume", "Submit for code review")
	})

	t.Run("submitted", func(t *testing.T) {
		p := base
		p.State, p.Claim, p.Holder = claimPanelSubmitted, claim("chat", lifecycle.ClaimSubmitted), "the chat agent"
		html := renderNamed(t, "task-claim", p)
		htmlHas(t, html, "Submitted by the chat agent. A code reviewer is reviewing it.")
		htmlLacks(t, html, "<button")
	})
}

// TestFeatureTasksRender is SPEC-020 FR-1.6 and FR-4.4: an executor icon named
// by the executor line, a label for a live claim, and the waiting sentence.
func TestFeatureTasksRender(t *testing.T) {
	page := &entityPage{Kind: "feature", ClaimWaiting: "T03 Add login is claimed by the chat agent, so this feature's agents are waiting.",
		Tasks: []taskRow{
			{PublicID: "FEAT-001-T01", Title: "Schema", State: "done", URL: "/ui/t/1", Icon: "agent", Line: "Implemented by the implementer (claude-sonnet-5)."},
			{PublicID: "FEAT-001-T02", Title: "Login", State: "active", URL: "/ui/t/2", Icon: "chat", Line: "Being implemented by the chat agent, who claimed it 3h ago.", ClaimLabel: "Claimed by the chat agent"},
			{PublicID: "FEAT-001-T03", Title: "Logout", State: "active", URL: "/ui/t/3", Icon: "owner", Line: "Being implemented by sam.", ClaimLabel: "Claimed by sam"},
			{PublicID: "FEAT-001-T04", Title: "Reset", State: "pending", URL: "/ui/t/4"},
		}}
	html := renderNamed(t, "feature-tasks", page)
	htmlHas(t, html, "agents are waiting.",
		`aria-label="Implemented by the implementer (claude-sonnet-5)."`, "#i-agent",
		`title="Being implemented by the chat agent, who claimed it 3h ago."`, "#i-chat",
		"#i-owner", "Claimed by the chat agent", "Claimed by sam")
	if n := strings.Count(html, "<svg class=\"icon icon-sm\" role=\"img\""); n != 3 {
		t.Errorf("three tasks have executors, got %d icons", n)
	}
	// Not a feature, or a feature with nothing to say: nothing is drawn.
	if got := strings.TrimSpace(renderNamed(t, "feature-tasks", &entityPage{Kind: "initiative"})); got != "" {
		t.Errorf("an initiative page draws nothing, got %q", got)
	}
	if got := strings.TrimSpace(renderNamed(t, "feature-tasks", &entityPage{Kind: "feature"})); got != "" {
		t.Errorf("a feature with no tasks draws nothing, got %q", got)
	}
}
