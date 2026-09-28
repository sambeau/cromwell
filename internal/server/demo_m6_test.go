package server

// The SPEC-012 demo harness (DoD 2). Skipped unless SUBUTAI_M6_DEMO names a
// directory: it then drives one feature through the whole loop with the mock
// provider — a spec review whose first attempt fails part-way and is retried,
// a code review that sends the task back once, and a verification — seeds a
// handful of earlier verdicts from an obviously careless reviewer, writes the
// URLs and measurements to <dir>/demo.json, and serves the UI until
// <dir>/done appears, so a browser can walk it. No AI provider is involved.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"subutai/internal/bus"
	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/store"
)

func TestDemoM6(t *testing.T) {
	dir := os.Getenv("SUBUTAI_M6_DEMO")
	if dir == "" {
		t.Skip("set SUBUTAI_M6_DEMO to a directory to run the SPEC-012 demo")
	}
	h := newHarness(t)
	ctx := context.Background()
	u := func(in, out int64) provider.Usage { return provider.Usage{Input: in, Output: out} }
	turn := func(text, tool, input string, usage provider.Usage) provider.Response {
		return provider.Response{StopReason: "tool_use", Usage: usage, Blocks: []provider.Block{
			provider.TextBlock(text),
			{Type: "tool_use", ToolUseID: "toolu_" + uuid.NewString()[:8], ToolName: tool, ToolInput: []byte(input)},
		}}
	}

	// --- The spec, reviewed twice: the first attempt fails part-way. ---
	specPath := h.setupFeatureWithSpec()
	h.mock.Respond(turn("I'll check the specification against the design before deciding. First, the lockout rule.",
		"read_file", `{"path":"docs/specs/login.md"}`, u(4210, 64)))
	h.mock.Fail(fmt.Errorf("provider returned 400: request rejected"))
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": specPath}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	specID := h.docID(specPath)
	h.eventually("first spec review failed", func() bool {
		r := h.runsFor("document", specID)
		return len(r) == 1 && r[0].State == "failed"
	})
	h.mock.Respond(provider.Response{StopReason: "tool_use", Usage: u(4388, 412), Blocks: []provider.Block{
		provider.TextBlock("The behaviour section is testable as written: each rule has a matching acceptance criterion. " +
			"One small gap: the overview doesn't say which email formats are accepted. That is minor and shouldn't hold the spec."),
		{Type: "tool_use", ToolUseID: "toolu_spec_ok", ToolName: "submit_review", ToolInput: []byte(
			`{"verdict":"approve","reasoning":"Clear, testable, and a faithful translation of the design. Every behaviour has an acceptance criterion.","comments":[{"section_ref":"Overview","body":"Say which email address formats are accepted, or link to the rule.","severity":"minor"}]}`)},
	}})
	h.srv.Dispatcher.RetrySweep(ctx)
	h.eventually("spec approved", func() bool { return h.docState(specPath) == lifecycle.DocApproved })

	// --- The plan. ---
	devPlan := h.addDevPlan(devPlanOneTask)
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"approve","reasoning":"One task covers the whole specification, and its description is specific enough to build from."}`, u(3120, 188))
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": devPlan}); code != 200 {
		t.Fatalf("submit plan: %d %v", code, out)
	}
	h.eventually("ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	// --- Build, sent back once, rebuilt, approved, verified. ---
	h.mock.Respond(turn("Starting with the layout of the repository.", "list_files", `{"path":"."}`, u(6020, 58)))
	h.mock.Respond(turn("There is no greeting helper yet, so I'll add one.", "write_file",
		`{"path":"greet.go","content":"package main\n\n// greeting is the welcome text.\nconst greeting = \"welcome\"\n"}`, u(6240, 120)))
	h.mock.Respond(turn("Now the tests.", "run_command", `{"name":"run_tests"}`, u(6410, 40)))
	h.mock.RespondOutcome("submit_implementation", `{"summary":"Added greet.go with the welcome text.","files_changed":["greet.go"]}`, u(6480, 52))
	h.mock.Respond(turn("Reading the change.", "read_file", `{"path":"greet.go"}`, u(5100, 36)))
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"request_changes","reasoning":"The task asks for a Greet function; the change adds only a constant.","comments":[{"section_ref":"greet.go","body":"There is no Greet function, which the task description requires.","severity":"major"}]}`,
		u(5230, 260))
	h.mock.Respond(turn("The reviewer is right: the task names a Greet function.", "write_file",
		`{"path":"greet.go","content":"package main\n\n// Greet returns the welcome text.\nfunc Greet() string { return \"welcome\" }\n"}`, u(7010, 130)))
	h.mock.Respond(turn("Running the tests again.", "run_command", `{"name":"run_tests"}`, u(7180, 38)))
	h.mock.RespondOutcome("submit_implementation", `{"summary":"Replaced the constant with a Greet function.","files_changed":["greet.go"]}`, u(7240, 48))
	h.mock.Respond(turn("Reading the revised change.", "read_file", `{"path":"greet.go"}`, u(5320, 30)))
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"approve","reasoning":"Greet exists and returns the welcome text; the change is in scope.","comments":[{"section_ref":"greet.go","body":"The doc comment could say where the text is shown.","severity":"minor"}]}`,
		u(5450, 190))
	h.mock.Respond(turn("Checking each acceptance criterion against the merged code.", "run_command", `{"name":"run_tests"}`, u(8020, 44)))
	h.mock.RespondOutcome("submit_verification",
		`{"criteria":[{"id":"AC1","met":true,"evidence":"greet.go defines Greet, returning the welcome text; run_tests passes."}],"verdict":"approve","reasoning":"The one criterion is met, with evidence."}`,
		u(8140, 220))
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	// --- Earlier verdicts from a careless reviewer, so review health has
	// something to flag. These are seeded, and the walkthrough says so. ---
	for i := 0; i < 6; i++ {
		finished := time.Now().Add(-time.Duration(i+1) * time.Hour)
		if _, err := h.srv.Store.Pool.Exec(ctx, `
			INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key,
				started_at, finished_at, input_tokens, output_tokens, outcome)
			VALUES ($1, 'succeeded', 'review-code', 'code-reviewer', 'quick-model', 'task', $2, $3,
				$4, $5, 900, 18, '{"verdict":"approve","reasoning":"LGTM"}')`,
			store.NewID(), uuid.New(), uuid.NewString(), finished.Add(-3*time.Second), finished); err != nil {
			t.Fatal(err)
		}
	}

	// --- Measurements (NFR-2, NFR-3). ---
	type runSize struct {
		Purpose string `json:"purpose"`
		Attempt int    `json:"attempt"`
		Entries int    `json:"entries"`
		Bytes   int64  `json:"bytes"`
		OnDisk  int64  `json:"on_disk"`
		RunID   string `json:"run_id"`
	}
	var sizes []runSize
	rows, err := h.srv.Store.Pool.Query(ctx, `
		SELECT d.purpose, t.attempt, count(*), sum(octet_length(t.content)), sum(pg_column_size(t.content)), d.id::text
		FROM transcript_entries t JOIN dispatches d ON d.id = t.dispatch_id
		GROUP BY d.purpose, t.attempt, d.id, d.queued_at ORDER BY d.queued_at, t.attempt`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s runSize
		if err := rows.Scan(&s.Purpose, &s.Attempt, &s.Entries, &s.Bytes, &s.OnDisk, &s.RunID); err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, s)
	}
	rows.Close()

	// Write time: a typical turn's batch (turn, text, tool call) and a result
	// batch, each written 200 times against the same database.
	probe := h.runsFor("document", specID)[0].ID
	in, out := int64(5000), int64(120)
	lat := 3
	const n = 200
	began := time.Now()
	for i := 0; i < n; i++ {
		if err := h.srv.Store.AppendTranscript(ctx, []store.TranscriptEntry{
			{DispatchID: probe, Attempt: 99, Seq: 3*i + 1, Turn: i, Kind: store.EntryTurn, Content: "tool_use", InputTokens: &in, OutputTokens: &out},
			{DispatchID: probe, Attempt: 99, Seq: 3*i + 2, Turn: i, Kind: store.EntryText, Content: "Reading the change before deciding."},
			{DispatchID: probe, Attempt: 99, Seq: 3*i + 3, Turn: i, Kind: store.EntryToolCall, ToolName: "read_file", Content: `{"path":"greet.go"}`},
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.srv.Store.AppendTranscript(ctx, []store.TranscriptEntry{
			{DispatchID: probe, Attempt: 99, Seq: 100000 + i, Turn: i, Kind: store.EntryToolResult, ToolName: "read_file", Content: "package main\n\nfunc Greet() string { return \"welcome\" }\n", LatencyMs: &lat},
		}); err != nil {
			t.Fatal(err)
		}
	}
	perTurn := time.Since(began) / n
	if _, err := h.srv.Store.Pool.Exec(ctx, `DELETE FROM transcript_entries WHERE dispatch_id = $1 AND attempt = 99`, probe); err != nil {
		t.Fatal(err)
	}

	task := h.tasks("auth/login")[0]
	info := map[string]any{
		"base":         h.api.URL,
		"feature":      "/ui/f/auth/login",
		"task":         "/ui/t/" + task.ID.String(),
		"spec":         "/ui/d/" + specPath,
		"spec_run":     "/ui/run/" + h.runsFor("document", specID)[0].ID.String(),
		"task_runs":    runURLs(h.runsFor("task", task.ID)),
		"health":       "/ui/review-health",
		"sizes":        sizes,
		"turn_write":   perTurn.String(),
		"turn_write_n": n,
	}
	b, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "demo.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("demo serving at %s; waiting for %s", h.api.URL, filepath.Join(dir, "done"))

	// Serve until the browser is done, broadcasting the live signal every two
	// seconds so the page's live regions refresh while it looks.
	deadline := time.Now().Add(15 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "done")); err == nil {
			return
		}
		h.srv.Hub.Broadcast(bus.DocumentFileChanged{Path: "demo"})
		time.Sleep(2 * time.Second)
	}
	t.Fatal("the demo browser never finished")
}

func runURLs(runs []store.Dispatch) []string {
	var out []string
	for _, r := range runs {
		out = append(out, "/ui/run/"+r.ID.String())
	}
	return out
}
