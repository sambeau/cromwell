package server

// SPEC-012 integration tests: transcripts written as runs proceed, including
// runs that fail or stall part-way; their size limits and retention; and the
// reads behind the timeline and review health. Real Postgres, mock provider.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"cromwell/internal/lifecycle"
	"cromwell/internal/provider"
	"cromwell/internal/store"
)

// runsFor returns every run on a ref, oldest first.
func (h *harness) runsFor(refType string, refID uuid.UUID) []store.Dispatch {
	h.t.Helper()
	rows, err := h.srv.Store.Pool.Query(context.Background(),
		`SELECT id FROM dispatches WHERE ref_type = $1 AND ref_id = $2 ORDER BY queued_at, id`, refType, refID)
	if err != nil {
		h.t.Fatal(err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			h.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []store.Dispatch
	for _, id := range ids {
		d, err := store.GetDispatch(context.Background(), h.srv.Store.Pool, id)
		if err != nil {
			h.t.Fatal(err)
		}
		out = append(out, *d)
	}
	return out
}

func (h *harness) transcript(id uuid.UUID, attempt int) []store.TranscriptEntry {
	h.t.Helper()
	es, err := store.Transcript(context.Background(), h.srv.Store.Pool, id, attempt)
	if err != nil {
		h.t.Fatal(err)
	}
	return es
}

func kinds(es []store.TranscriptEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Kind
	}
	return out
}

func (h *harness) docID(path string) uuid.UUID {
	h.t.Helper()
	d, err := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, path)
	if err != nil {
		h.t.Fatal(err)
	}
	return d.ID
}

// TestTranscriptOfASuccessfulRun is FR-1.2 to FR-1.6 and FR-1.8: a review run
// that calls a tool it isn't offered, then submits an invalid verdict, then a
// valid one, keeps every step in order, with tokens per turn and latency.
func TestTranscriptOfASuccessfulRun(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	h.mock.Respond(provider.Response{
		StopReason: "tool_use", Usage: provider.Usage{Input: 100, Output: 7},
		Blocks: []provider.Block{
			provider.TextBlock("Let me look at the file first."),
			{Type: "tool_use", ToolUseID: "toolu_a", ToolName: "read_file", ToolInput: []byte(`{"path":"docs/specs/login.md"}`)},
		},
	})
	// Approving while holding a major finding is refused by the validator.
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"approve","reasoning":"fine","comments":[{"body":"lockout is untested","severity":"major"}]}`,
		provider.Usage{Input: 120, Output: 20})
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"approve","reasoning":"clear and testable","comments":[{"body":"typo in overview","severity":"minor"}]}`,
		provider.Usage{Input: 140, Output: 25})

	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": specPath}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	h.eventually("approved", func() bool { return h.docState(specPath) == lifecycle.DocApproved })

	runs := h.runsFor("document", h.docID(specPath))
	if len(runs) != 1 {
		t.Fatalf("want one review run, got %d", len(runs))
	}
	es := h.transcript(runs[0].ID, 1)
	want := []string{"system", "prompt", "turn", "text", "tool_call", "tool_result",
		"turn", "tool_call", "tool_result", "turn", "tool_call", "outcome"}
	if got := kinds(es); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("entries = %v\nwant      %v", got, want)
	}
	for i, e := range es {
		if e.Seq != i+1 {
			t.Errorf("entry %d has seq %d", i, e.Seq)
		}
	}
	if es[0].Content == "" || es[1].Content == "" {
		t.Error("the prompts should be stored")
	}
	if es[2].Turn != 1 || es[2].InputTokens == nil || *es[2].InputTokens != 100 || *es[2].OutputTokens != 7 {
		t.Errorf("turn 1 tokens: %+v", es[2])
	}
	if es[3].Content != "Let me look at the file first." {
		t.Errorf("text entry: %q", es[3].Content)
	}
	if es[4].ToolName != "read_file" || es[4].ToolUseID != "toolu_a" || !strings.Contains(es[4].Content, "login.md") {
		t.Errorf("tool call: %+v", es[4])
	}
	if !es[5].IsError || es[5].LatencyMs == nil || !strings.Contains(es[5].Content, "not available") {
		t.Errorf("refused tool result: %+v", es[5])
	}
	if !es[8].IsError || !strings.Contains(es[8].Content, "Invalid input") {
		t.Errorf("the invalid outcome should come back as an error result: %+v", es[8])
	}
	last := es[len(es)-1]
	if last.ToolName != "submit_review" || !strings.Contains(last.Content, "clear and testable") || last.Turn != 3 {
		t.Errorf("outcome entry: %+v", last)
	}
}

// TestToolLatencyIsMeasured is FR-1.8: the ledger row and the transcript carry
// the same measured latency for a real tool call.
func TestToolLatencyIsMeasured(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	devPlan := h.addDevPlan(devPlanOneTask)
	h.approveDoc(devPlan)
	h.eventually("ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n"}`, provider.Usage{Input: 50, Output: 20})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"added greet.go","files_changed":["greet.go"]}`, provider.Usage{Input: 30, Output: 10})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 40, Output: 10})
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":true,"evidence":"greet.go present"}],"verdict":"approve","reasoning":"met"}`, provider.Usage{Input: 40, Output: 15})
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	task := h.tasks("auth/login")[0]
	impl := h.runsFor("task", task.ID)[0]
	var ledger int
	if err := h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT latency_ms FROM tool_calls WHERE dispatch_id = $1`, impl.ID).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	var result *store.TranscriptEntry
	for _, e := range h.transcript(impl.ID, 1) {
		if e.Kind == store.EntryToolResult {
			e := e
			result = &e
		}
	}
	if result == nil || result.LatencyMs == nil || *result.LatencyMs != ledger {
		t.Fatalf("transcript latency %v should equal the ledger's %d", result, ledger)
	}
}

// TestTranscriptOfAFailedRun is FR-1.5, FR-1.6 and SD-2/SD-3: a run whose
// second model call fails keeps its prompts and first turn and ends with the
// reason; its retry is a second attempt with its own entries.
func TestTranscriptOfAFailedRun(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	h.mock.Respond(provider.Response{
		StopReason: "end_turn", Usage: provider.Usage{Input: 90, Output: 5},
		Blocks:     []provider.Block{provider.TextBlock("Thinking about it.")},
	})
	h.mock.Fail(errors.New("provider said no"))

	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": specPath}); code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}
	docID := h.docID(specPath)
	h.eventually("run failed", func() bool {
		runs := h.runsFor("document", docID)
		return len(runs) == 1 && runs[0].State == "failed"
	})
	run := h.runsFor("document", docID)[0]
	first := h.transcript(run.ID, 1)
	want := []string{"system", "prompt", "turn", "text", "nudge", "error"}
	if got := kinds(first); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("attempt 1 entries = %v, want %v", got, want)
	}
	if e := first[len(first)-1]; !e.IsError || !strings.Contains(e.Content, "provider said no") {
		t.Errorf("error entry: %+v", e)
	}

	// The retry reuses the row as attempt 2, clearing dispatches.error; the
	// first attempt's reason survives in its transcript.
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"fine now"}`, provider.Usage{Input: 10, Output: 10})
	h.srv.Dispatcher.RetrySweep(context.Background())
	h.eventually("approved on retry", func() bool { return h.docState(specPath) == lifecycle.DocApproved })

	attempts, err := store.TranscriptAttempts(context.Background(), h.srv.Store.Pool, run.ID)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("attempts = %v, %v", attempts, err)
	}
	second := h.transcript(run.ID, 2)
	if got := kinds(second); strings.Join(got, ",") != "system,prompt,turn,tool_call,outcome" {
		t.Errorf("attempt 2 entries = %v", got)
	}
	if again := h.transcript(run.ID, 1); len(again) != len(first) {
		t.Errorf("attempt 1 changed after the retry: %v", kinds(again))
	}
}

// TestTranscriptOfAStalledRun is FR-1.6 for a process that died: the entries
// it wrote stay, and the stall sweep ends them with the reason.
func TestTranscriptOfAStalledRun(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	ctx := context.Background()
	docID := h.docID(specPath)

	// A run the dispatcher never picks up: inserted as running, as if claimed
	// by a process that then died.
	id := store.NewID()
	if _, err := h.srv.Store.Pool.Exec(ctx, `
		INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key, started_at, heartbeat_at)
		VALUES ($1, 'running', 'review-spec', 'spec-reviewer', 'm', 'document', $2, 'stall-test', now() - interval '1 hour', now() - interval '1 hour')`,
		id, docID); err != nil {
		t.Fatal(err)
	}
	u := int64(10)
	if err := h.srv.Store.AppendTranscript(ctx, []store.TranscriptEntry{
		{DispatchID: id, Attempt: 1, Seq: 1, Kind: store.EntrySystem, Content: "you review specs"},
		{DispatchID: id, Attempt: 1, Seq: 2, Kind: store.EntryPrompt, Content: "review this"},
		{DispatchID: id, Attempt: 1, Seq: 3, Turn: 1, Kind: store.EntryTurn, InputTokens: &u, OutputTokens: &u},
	}); err != nil {
		t.Fatal(err)
	}
	h.srv.Dispatcher.StallSweep(ctx)

	es := h.transcript(id, 1)
	if got := kinds(es); strings.Join(got, ",") != "system,prompt,turn,error" {
		t.Fatalf("entries = %v", got)
	}
	if e := es[3]; !strings.Contains(e.Content, "stalled") || e.Turn != 1 || e.Seq != 4 {
		t.Errorf("stall entry: %+v", e)
	}
}

// TestTranscriptLimits is FR-2.2 and FR-2.3: an over-long tool result is cut
// with its original size kept, and once the attempt's budget is spent later
// entries keep their shape but not their content.
func TestTranscriptLimits(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	devPlan := h.addDevPlan(devPlanOneTask)
	h.approveDoc(devPlan)
	h.eventually("ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	// 100 KB file, then read it back: the result is over the 32 KB default.
	big := strings.Repeat("abcdefghij", 10_000)
	h.mock.RespondToolUse("write_file", `{"path":"big.txt","content":"`+big+`"}`, provider.Usage{Input: 5, Output: 5})
	h.mock.RespondToolUse("read_file", `{"path":"big.txt"}`, provider.Usage{Input: 5, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"wrote a big file","files_changed":["big.txt"]}`, provider.Usage{Input: 5, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 5, Output: 5})
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":true,"evidence":"present"}],"verdict":"approve","reasoning":"met"}`, provider.Usage{Input: 5, Output: 5})
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	impl := h.runsFor("task", h.tasks("auth/login")[0].ID)[0]
	var read *store.TranscriptEntry
	for _, e := range h.transcript(impl.ID, 1) {
		if e.Kind == store.EntryToolResult && e.ToolName == "read_file" {
			e := e
			read = &e
		}
	}
	if read == nil {
		t.Fatal("no read_file result")
	}
	if !read.Truncated || read.ContentBytes < 100_000 || len(read.Content) > 33_000 ||
		!strings.Contains(read.Content, "bytes cut from the middle") {
		t.Errorf("read result should be cut: truncated=%v original=%d stored=%d", read.Truncated, read.ContentBytes, len(read.Content))
	}
}

// TestTranscriptBudget is FR-2.3 on the recorder's own terms: with a tiny
// attempt budget, entries past it exist and carry the marker.
func TestTranscriptBudget(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	cfgPath := h.root + "/.cromwell/config.yaml"
	appendFile(t, cfgPath, "transcripts:\n  max_attempt_bytes: 10\n")

	h.approveDoc(specPath)
	run := h.runsFor("document", h.docID(specPath))[0]
	es := h.transcript(run.ID, 1)
	if len(es) < 4 {
		t.Fatalf("entries = %v", kinds(es))
	}
	last := es[len(es)-1]
	if last.Kind != store.EntryOutcome || !last.Truncated || !strings.Contains(last.Content, "size limit") {
		t.Errorf("entry past the budget should keep its kind and carry the marker: %+v", last)
	}
}

// TestTranscriptPruning is FR-2.4: old finished runs lose their transcript and
// are marked; their outcome stays; recent runs keep theirs; retention 0 keeps
// everything.
func TestTranscriptPruning(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	ctx := context.Background()
	run := h.runsFor("document", h.docID(specPath))[0]
	if len(h.transcript(run.ID, 1)) == 0 {
		t.Fatal("expected a transcript")
	}

	// Recent: nothing goes.
	if n, err := h.srv.Store.PruneTranscripts(ctx, 180); err != nil || n != 0 {
		t.Fatalf("recent prune: %d %v", n, err)
	}
	// Retention 0: nothing goes, even when old.
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE dispatches SET finished_at = now() - interval '400 days' WHERE id = $1`, run.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := h.srv.Store.PruneTranscripts(ctx, 0); n != 0 {
		t.Fatalf("retention 0 removed %d entries", n)
	}
	n, err := h.srv.Store.PruneTranscripts(ctx, 180)
	if err != nil || n == 0 {
		t.Fatalf("old prune: %d %v", n, err)
	}
	if es := h.transcript(run.ID, 1); len(es) != 0 {
		t.Errorf("entries should be gone: %v", kinds(es))
	}
	d, _ := store.GetDispatch(ctx, h.srv.Store.Pool, run.ID)
	if len(d.Outcome) == 0 || d.InputTokens == nil {
		t.Error("the outcome and tokens must stay")
	}
	if at, _ := store.TranscriptPrunedAt(ctx, h.srv.Store.Pool, run.ID); at == nil {
		t.Error("the run should be marked as pruned")
	}
	// The heartbeat duty is paced: a second call within the hour does nothing.
	h.srv.PruneTranscriptsSweep(ctx, time.Now())
	h.srv.PruneTranscriptsSweep(ctx, time.Now())
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}
