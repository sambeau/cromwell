package dispatch

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"subutai/internal/config"
	"subutai/internal/provider"
	"subutai/internal/store"
)

// recorder writes one attempt's transcript as the agent loop runs (SPEC-012
// FR-1). It is owned by the loop's goroutine. Every write is best effort: a
// transcript is a record of the run and must never be the reason it fails
// (NFR-4), so errors are logged and the run carries on.
type recorder struct {
	store   *store.Store
	log     *slog.Logger
	id      uuid.UUID
	attempt int
	limits  config.TranscriptConfig

	seq  int
	used int // content bytes stored so far in this attempt (FR-2.3)
}

// budgetMarker replaces the content of entries written after the attempt's
// budget is spent (FR-2.3).
const budgetMarker = "[Not stored: this run reached its transcript size limit.]"

func (dp *Dispatcher) newRecorder(d *store.Dispatch, limits config.TranscriptConfig) *recorder {
	return &recorder{store: dp.Store, log: dp.Log, id: d.ID, attempt: d.Attempt, limits: limits}
}

// entry fills in the run, attempt and sequence, cuts the content to limit and
// applies the attempt budget.
func (r *recorder) entry(e store.TranscriptEntry, limit int) store.TranscriptEntry {
	r.seq++
	e.DispatchID, e.Attempt, e.Seq = r.id, r.attempt, r.seq
	e.Content = store.CleanText(e.Content)
	e.ContentBytes = len(e.Content)
	if content, cut := store.Cut(e.Content, limit); cut {
		e.Content, e.Truncated = content, true
	}
	switch e.Kind {
	case store.EntrySystem, store.EntryPrompt, store.EntryOutcome, store.EntryError, "stop":
		// What the agent was told and what it concluded are always kept
		// whole, within their own limit; the budget bounds what it did.
	default:
		if max := r.limits.MaxAttemptBytes; max > 0 && r.used+len(e.Content) > max {
			e.Content, e.Truncated = budgetMarker, true
		}
		r.used += len(e.Content)
	}
	return e
}

func (r *recorder) write(ctx context.Context, entries ...store.TranscriptEntry) {
	if err := r.store.AppendTranscript(ctx, entries); err != nil {
		r.log.Error("transcript write", "dispatch", r.id, "attempt", r.attempt, "err", err)
	}
}

// prompts records what the agent was told, once per attempt (FR-1.2).
func (r *recorder) prompts(ctx context.Context, system, user string) {
	r.write(ctx,
		r.entry(store.TranscriptEntry{Kind: store.EntrySystem, Content: system}, r.limits.MaxPromptBytes),
		r.entry(store.TranscriptEntry{Kind: store.EntryPrompt, Content: user}, r.limits.MaxPromptBytes))
}

// turn records one model reply: the turn's tokens and latency, then its text
// and tool calls in the order the model gave them (FR-1.3).
func (r *recorder) turn(ctx context.Context, n int, resp *provider.Response, latency time.Duration) {
	ms := int(latency.Milliseconds())
	u := resp.Usage
	entries := []store.TranscriptEntry{r.entry(store.TranscriptEntry{
		Kind: store.EntryTurn, Turn: n, Content: resp.StopReason, LatencyMs: &ms,
		InputTokens: &u.Input, OutputTokens: &u.Output, CacheRead: &u.CacheRead, CacheWrite: &u.CacheWrite,
	}, r.limits.MaxEntryBytes)}
	for _, b := range resp.Blocks {
		switch b.Type {
		case "text":
			entries = append(entries, r.entry(store.TranscriptEntry{
				Kind: store.EntryText, Turn: n, Content: b.Text}, r.limits.MaxEntryBytes))
		case "tool_use":
			entries = append(entries, r.entry(store.TranscriptEntry{
				Kind: store.EntryToolCall, Turn: n, ToolName: b.ToolName, ToolUseID: b.ToolUseID,
				Content: string(b.ToolInput)}, r.limits.MaxEntryBytes))
		}
	}
	r.write(ctx, entries...)
}

// toolResult builds the entry for one result sent back to the agent (FR-1.4).
// Results are written together once the turn's tools have run.
func (r *recorder) toolResult(n int, tu *provider.Block, result string, isErr bool, latency time.Duration) store.TranscriptEntry {
	ms := int(latency.Milliseconds())
	return r.entry(store.TranscriptEntry{
		Kind: store.EntryToolResult, Turn: n, ToolName: tu.ToolName, ToolUseID: tu.ToolUseID,
		Content: result, IsError: isErr, LatencyMs: &ms,
	}, r.limits.MaxToolResultBytes)
}

func (r *recorder) nudge(ctx context.Context, n int, text string) {
	r.write(ctx, r.entry(store.TranscriptEntry{Kind: store.EntryNudge, Turn: n, Content: text}, r.limits.MaxEntryBytes))
}

// outcome records what the run concluded, along with any results from its
// final turn that had not been written yet (FR-1.5).
func (r *recorder) outcome(ctx context.Context, n int, tu *provider.Block, pending []store.TranscriptEntry) {
	pending = append(pending, r.entry(store.TranscriptEntry{
		Kind: store.EntryOutcome, Turn: n, ToolName: tu.ToolName, ToolUseID: tu.ToolUseID,
		Content: string(tu.ToolInput)}, r.limits.MaxEntryBytes))
	r.write(ctx, pending...)
}

// stop records that a budgeted run ended at a limit rather than concluding.
func (r *recorder) stop(ctx context.Context, n int, text string) {
	r.write(ctx, r.entry(store.TranscriptEntry{Kind: "stop", Turn: n, Content: text}, r.limits.MaxEntryBytes))
}
