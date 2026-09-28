package server

// SPEC-012 in the browser: the feature timeline, the transcript viewer, the
// run lists on task and document pages, and review health, rendered from real
// runs of the mock provider against real Postgres.

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/store"
)

// driveLoopWithSendback takes auth/login from a draft spec to done, with the
// code reviewer sending the one task back once (DoD 2's loop, without the
// failed run).
func (h *harness) driveLoopWithSendback() {
	h.t.Helper()
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	devPlan := h.addDevPlan(devPlanOneTask)
	h.approveDoc(devPlan)
	h.eventually("ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"v1","files_changed":["greet.go"]}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"request_changes","comments":[{"body":"missing Greet function","severity":"major"}],"reasoning":"incomplete"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n\nfunc Greet() string { return \"hi\" }\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"v2 with Greet","files_changed":["greet.go"]}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"now correct"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":true,"evidence":"greet.go defines Greet"}],"verdict":"approve","reasoning":"met"}`, provider.Usage{Input: 10, Output: 5})
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		h.t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })
}

// TestTimelineOfAFullLoop is FR-5.4 and FR-5.5 on a real run: the moments in
// order, each run in the phase it was queued in, and each verdict moment led
// to by the run that gave it.
func TestTimelineOfAFullLoop(t *testing.T) {
	h := newHarness(t)
	h.driveLoopWithSendback()
	f := mustFeatureID(t, h, "auth/login")
	view, err := h.srv.featureTimeline(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range view.Moments {
		got = append(got, m.Label)
	}
	want := []string{"Created", "Spec written", "Spec approved", "Plan written", "Plan approved",
		"Broken into 1 task", "Ready to build", "Building (1 of 1 task done)",
		"Code review sent back “Greeting helper” (round 1)", "Verifying", "Done"}
	// M3's "Sent to development" may come before the spec is written; nothing
	// else may be added or moved.
	if len(got) > 1 && got[1] == "Sent to development" {
		got = append(got[:1], got[2:]...)
	}
	if strings.Join(got, " · ") != strings.Join(want, " · ") {
		t.Fatalf("moments:\n got %s\nwant %s", strings.Join(got, " · "), strings.Join(want, " · "))
	}

	purposes := func(m momentView) string {
		var p []string
		for _, r := range m.Runs {
			p = append(p, strings.Fields(r.What)[0])
		}
		return strings.Join(p, ",")
	}
	byLabel := map[string]momentView{}
	for _, m := range view.Moments {
		byLabel[strings.SplitN(m.Label, " (", 2)[0]] = m
	}
	if got := purposes(byLabel["Building"]); got != "Building,Reviewing" {
		t.Errorf("Building holds %s", got)
	}
	if got := purposes(byLabel["Code review sent back “Greeting helper”"]); got != "Building,Reviewing" {
		t.Errorf("the sendback holds the rework: %s", got)
	}
	if got := purposes(byLabel["Verifying"]); got != "Checking" {
		t.Errorf("Verifying holds %s", got)
	}

	for label, want := range map[string]string{
		"Spec approved": "Reviewing the specification", "Plan approved": "Reviewing the development plan",
		"Code review sent back “Greeting helper”": "Reviewing the code", "Done": "Checking the feature",
	} {
		m := byLabel[label]
		if m.Cause == nil || !strings.HasPrefix(m.Cause.What, want) || m.By != "agent" {
			t.Errorf("%s should be led to by %q, got %+v (by %s)", label, want, m.Cause, m.By)
		}
	}
	if sb := byLabel["Code review sent back “Greeting helper”"]; sb.Cause.Verdict != "request_changes" {
		t.Errorf("the sendback's cause is the review that sent it back: %+v", sb.Cause)
	}
	if byLabel["Spec written"].By != "person" || byLabel["Spec written"].Actor != "sam" {
		t.Errorf("a person's act is attributed to them: %+v", byLabel["Spec written"].Moment)
	}
}

var currency = regexp.MustCompile(`\$\s*\d`)

// TestUIFeatureTimeline is FR-6: the line and the detail on the feature page,
// the links down to runs that resolve, no "Recent activity" on the feature
// while other pages keep theirs, the line-only live fragment, and no money
// even when a budget question is on the record (SD-12).
func TestUIFeatureTimeline(t *testing.T) {
	h := newHarness(t)
	h.driveLoopWithSendback()
	ctx := context.Background()
	f := mustFeatureID(t, h, "auth/login")
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.CreateCheckpoint(ctx, tx, "budget", "feature", f,
			"Budget cap $5.00 reached ($5.10 spent this monthly period).", map[string]any{"spent_usd": 5.1})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	code, body := h.getUI("/ui/f/auth/login")
	if code != 200 {
		t.Fatalf("feature page: %d", code)
	}
	for _, want := range []string{`class="journey-line"`, "Spec approved", "Building (1 of 1 task done)",
		"Code review sent back", "Verifying", "Done", "What happened at each step",
		"What led to this:", "Waiting for a person: the spending limit was reached", "by <b>sam</b>"} {
		mustContain(t, "feature page", body, want)
	}
	if strings.Contains(body, "Recent activity") {
		t.Error("the timeline replaces Recent activity on a feature page (SD-7)")
	}
	if m := currency.FindString(body); m != "" {
		t.Errorf("money on the feature page: %q", m)
	}
	links := regexp.MustCompile(`href="(/ui/run/[0-9a-f-]+)"`).FindAllStringSubmatch(body, -1)
	if len(links) < 7 {
		t.Fatalf("expected links to every run, found %d", len(links))
	}
	for _, l := range links {
		if c, _ := h.getUI(l[1]); c != 200 {
			t.Errorf("%s: %d", l[1], c)
		}
	}

	_, frag := h.getUI("/ui/frag/timeline?feature=" + f.String())
	if !strings.Contains(frag, "journey-line") || strings.Contains(frag, "What happened at each step") {
		t.Error("the live fragment is the line only, so an open detail isn't closed (R12-8)")
	}
	if c, _ := h.getUI("/ui/frag/timeline?feature=nope"); c != 400 {
		t.Errorf("a bad feature id is a bad request, got %d", c)
	}
	if _, init := h.getUI("/ui/i/auth"); !strings.Contains(init, "Recent activity") {
		t.Error("initiative pages keep Recent activity")
	}
}

// TestUIRunLinks is FR-4: the task page and the spec's page list their runs,
// and each link resolves.
func TestUIRunLinks(t *testing.T) {
	h := newHarness(t)
	h.driveLoopWithSendback()
	task := h.tasks("auth/login")[0]

	_, body := h.getUI("/ui/t/" + task.ID.String())
	mustContain(t, "task page", body, "Agent runs on this task")
	mustContain(t, "task page", body, "Asked for changes")
	if n := strings.Count(body, `class="run-line"`); n != 4 {
		t.Errorf("the task had 4 runs (two builds, two reviews); page lists %d", n)
	}
	_, doc := h.getUI("/ui/d/docs/specs/login.md")
	mustContain(t, "spec page", doc, "Agent reviews of this document")
	for _, l := range regexp.MustCompile(`href="(/ui/run/[0-9a-f-]+)"`).FindAllStringSubmatch(body+doc, -1) {
		if c, _ := h.getUI(l[1]); c != 200 {
			t.Errorf("%s: %d", l[1], c)
		}
	}
}

// TestUIAgentRunPage is FR-3: a finished review, a failed run with two
// attempts, a run from before transcripts, a pruned run, a running run and an
// unknown id.
func TestUIAgentRunPage(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	ctx := context.Background()

	// Attempt 1 fails part-way; attempt 2 approves with a minor finding.
	h.mock.Respond(provider.Response{StopReason: "tool_use", Usage: provider.Usage{Input: 1200, Output: 40},
		Blocks: []provider.Block{provider.TextBlock("I will read the spec."),
			{Type: "tool_use", ToolUseID: "toolu_r", ToolName: "read_file", ToolInput: []byte(`{"path":"docs/specs/login.md"}`)}}})
	h.mock.Fail(fmt.Errorf("model refused"))
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": specPath}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	docID := h.docID(specPath)
	h.eventually("failed", func() bool { r := h.runsFor("document", docID); return len(r) == 1 && r[0].State == "failed" })
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"clear and testable","comments":[{"section_ref":"Overview","body":"say which email formats count","severity":"minor"}]}`,
		provider.Usage{Input: 1300, Output: 60})
	h.srv.Dispatcher.RetrySweep(ctx)
	h.eventually("approved", func() bool { return h.docState(specPath) == lifecycle.DocApproved })
	run := h.runsFor("document", docID)[0]
	url := "/ui/run/" + run.ID.String()

	code, body := h.getUI(url)
	if code != 200 {
		t.Fatalf("run page: %d", code)
	}
	for _, want := range []string{"Reviewing the specification “Login form”", "What it concluded", "Approved",
		"clear and testable", "say which email formats count", "On “Overview”", "What it was told",
		"Its instructions", "Its task", "Turn 1", "tokens in", "Attempt 1", "Attempt 2", "spec-reviewer",
		`href="/ui/d/docs/specs/login.md"`} {
		mustContain(t, "run page", body, want)
	}
	if m := currency.FindString(body); m != "" {
		t.Errorf("money on the run page: %q", m)
	}

	if strings.Contains(body, "no result recorded") {
		t.Error("the outcome call is shown once, as the answer, not again as a tool")
	}
	_, first := h.getUI(url + "?attempt=1")
	if !strings.Contains(first, `data-state="failed"`) || strings.Contains(first, `badge" data-state="succeeded"`) {
		t.Error("an earlier attempt is shown as failed, not with the run's current state")
	}
	for _, want := range []string{"This attempt failed", "The run failed.", "read_file", "docs/specs/login.md",
		"I will read the spec.", "1,200 tokens in", "What the engine recorded", "model refused"} {
		mustContain(t, "attempt 1", first, want)
	}

	// A run from before transcripts: its ledger and outcome only.
	if _, err := h.srv.Store.Pool.Exec(ctx, `DELETE FROM transcript_entries WHERE dispatch_id = $1`, run.ID); err != nil {
		t.Fatal(err)
	}
	_, old := h.getUI(url)
	mustContain(t, "pre-M6 run", old, "There is no transcript for this run")
	mustContain(t, "pre-M6 run", old, "clear and testable")
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET transcript_pruned_at = now() WHERE id = $1`, run.ID); err != nil {
		t.Fatal(err)
	}
	_, pruned := h.getUI(url)
	mustContain(t, "pruned run", pruned, "transcript was removed")

	// A running run shows the status line that follows it, and nothing else polls.
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET state = 'running', finished_at = NULL WHERE id = $1`, run.ID); err != nil {
		t.Fatal(err)
	}
	_, running := h.getUI(url)
	mustContain(t, "running run", running, `hx-get="/ui/frag/run/`+run.ID.String())
	mustContain(t, "running run", running, "reached a conclusion yet")
	_, prog := h.getUI("/ui/frag/run/" + run.ID.String() + "?shown=0")
	mustContain(t, "progress", prog, "Running")

	if c, _ := h.getUI("/ui/run/" + uuid.NewString()); c != 404 {
		t.Errorf("unknown run: %d", c)
	}
}

// TestUIReviewHealth is FR-7 and FR-8 over seeded verdicts: empty, a flagged
// rubber stamp sorted first, a healthy reviewer, the window, and the card.
func TestUIReviewHealth(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	_, empty := h.getUI("/ui/review-health")
	mustContain(t, "empty page", empty, "No agent has reached a verdict yet")
	_, card := h.getUI("/ui/frag/review-health")
	mustContain(t, "empty card", card, "No agent has reached a verdict yet")
	_, home := h.getUI("/ui")
	mustContain(t, "home", home, `href="/ui/review-health"`)

	seed := func(role, model, purpose, outcome string, took time.Duration, output int, ago time.Duration) {
		t.Helper()
		finished := time.Now().Add(-ago)
		if _, err := h.srv.Store.Pool.Exec(ctx, `
			INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key,
				started_at, finished_at, input_tokens, output_tokens, outcome)
			VALUES ($1, 'succeeded', $2, $3, $4, 'task', $5, $6, $7, $8, 500, $9, $10)`,
			store.NewID(), purpose, role, model, uuid.New(), uuid.NewString(),
			finished.Add(-took), finished, output, outcome); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		seed("code-reviewer", "quick-model", "review-code", `{"verdict":"approve","reasoning":"ok"}`, 2*time.Second, 20, time.Hour)
		seed("spec-reviewer", "careful-model", "review-spec",
			`{"verdict":"request_changes","reasoning":"gaps","comments":[{"body":"x","severity":"major"},{"body":"y","severity":"minor"}]}`,
			80*time.Second, 900, time.Hour)
	}
	seed("spec-reviewer", "careful-model", "review-spec", `{"verdict":"approve","reasoning":"fine"}`, 60*time.Second, 800, 100*24*time.Hour)
	// A comments-only review has no verdict and isn't counted.
	seed("design-reviewer", "m", "review-design", `{"reasoning":"notes","comments":[{"body":"z"}]}`, time.Second, 10, time.Hour)

	_, page := h.getUI("/ui/review-health")
	for _, want := range []string{"code-reviewer", "quick-model", "Approves almost everything, quickly",
		"spec-reviewer", "careful-model", "How to read this", "at least 5 verdicts"} {
		mustContain(t, "review health", page, want)
	}
	if strings.Contains(page, "design-reviewer") {
		t.Error("a review with no verdict isn't counted")
	}
	if strings.Index(page, "quick-model") > strings.Index(page, "careful-model") {
		t.Error("the flagged rubber stamp sorts first")
	}
	mustContain(t, "all time", page, "1 of 6 verdicts")
	_, recent := h.getUI("/ui/review-health?window=30d")
	mustContain(t, "30 days", recent, "0 of 5 verdicts")
	// One old approval kept it above the line all time; without it, it is flagged.
	mustContain(t, "30 days", recent, "Hardly ever approves")
	if strings.Contains(page, "Hardly ever approves") {
		t.Error("all time, the careful reviewer approved 1 of 6, above the line")
	}
	if m := currency.FindString(page); m != "" {
		t.Errorf("money on review health: %q", m)
	}
	_, card = h.getUI("/ui/frag/review-health")
	mustContain(t, "card", card, "1 reviewer needs a look")
}
