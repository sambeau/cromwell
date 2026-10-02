// Package dispatch implements the dispatch pipeline and governance
// (DESIGN-002 §4-6): claiming queued dispatches under the governor, running
// the agent loop against the provider until the outcome tool fires,
// recording consumption and cost from the price snapshot frozen at claim.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/config"
	"subutai/internal/provider"
	"subutai/internal/store"
	"subutai/internal/toolhost"
)

// ReviewOutcomeTool is the reviewer's single outcome tool (O-2). Every
// dispatched agent gets exactly one outcome tool matching its purpose; the
// dispatch is complete only when it is called.
func ReviewOutcomeTool() provider.ToolDef {
	return provider.ToolDef{
		Name:        "submit_review",
		Description: "Submit your review verdict. This completes the review — call it exactly once, when you have reached a conclusion.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"verdict": map[string]any{
					"type":        "string",
					"enum":        []string{"approve", "request_changes", "escalate"},
					"description": "The verdict follows from your findings. request_changes: at least one major finding. approve: no major findings — say so even if minor ones remain, because minor findings do not send work back. escalate: a human must decide; explain why in reasoning.",
				},
				"comments": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"section_ref": map[string]any{"type": "string", "description": "Heading of the section the comment is about, empty for whole-document comments"},
							"body":        map[string]any{"type": "string"},
							"severity": map[string]any{
								"type":        "string",
								"enum":        []string{"major", "minor"},
								"description": "major: correctness, completeness, scope or soundness is at stake, and the work must change. minor: style, naming, tidiness — worth noting, not worth another round. Classify honestly in both directions: calling everything major stalls the work, and calling a real defect minor lets it through.",
							},
						},
						"required": []string{"body", "severity"},
					},
				},
				"reasoning": map[string]any{"type": "string", "description": "Why you reached this verdict"},
			},
			"required": []string{"verdict", "reasoning"},
		},
	}
}

// Cost computes USD from usage at per-million-token prices (F-3).
func Cost(u provider.Usage, p config.PricePerMTok) float64 {
	const mtok = 1e6
	return float64(u.Input)*p.Input/mtok +
		float64(u.Output)*p.Output/mtok +
		float64(u.CacheRead)*p.CacheRead/mtok +
		float64(u.CacheWrite)*p.CacheWrite/mtok
}

// Plan is everything a claimed dispatch needs to run: the assembled prompt,
// the tools offered (role profile + the one outcome tool), the name of that
// outcome tool, an optional per-purpose outcome validator (for in-dispatch
// self-correction, DESIGN-002 §4), and — for mutating purposes — the tool
// host context. Read-only document reviews carry a nil ToolCtx.
type Plan struct {
	System          string
	User            string
	TurnCap         int
	Tools           []provider.ToolDef
	OutcomeTool     string
	ValidateOutcome func(json.RawMessage) error
	ToolCtx         *toolhost.Context
}

// Planner assembles a dispatch's plan. Implemented by the server (it owns
// store queries and fresh config reads, O-6). One method for all purposes:
// the server switches on d.Purpose (DESIGN-006 §4.1 — one loop, many tools).
type Planner interface {
	Plan(ctx context.Context, d *store.Dispatch) (*Plan, error)
}

// ToolExecutor runs a non-outcome tool call in the worktree and returns the
// result to feed back to the agent. isErr marks a tool-level failure (a bad
// path, a drifted anchor, a failing command) — returned to the agent, not a
// dispatch failure (DESIGN-006 §4).
type ToolExecutor interface {
	Execute(ctx context.Context, tctx *toolhost.Context, name string, input json.RawMessage) (result string, isErr bool)
}

// ProviderFactory returns the provider client for a model's provider name.
type ProviderFactory func(providerName string) (provider.Provider, error)

type Dispatcher struct {
	Store     *store.Store
	Bus       *bus.Bus
	Config    func() (*config.Config, error) // fresh read per use (O-6)
	Planner   Planner
	Tools     ToolExecutor
	Providers ProviderFactory
	Log       *slog.Logger

	// OnCheckpoint, if set, is called after a checkpoint is created and its
	// transaction commits — the presentation-only signal that lights the live
	// inbox (DESIGN-007 §6, FR-8.2). Optional; nil disables it (e.g. in tests).
	OnCheckpoint func(cp *store.Checkpoint)

	// BackoffBase paces in-attempt transient retries and the attempt-level
	// retry sweep; tests shrink it.
	BackoffBase time.Duration
	// MaxOutputTokens per model call.
	MaxOutputTokens int
	// BranchHead, if set, reads the head of the branch of the feature that
	// owns an implement dispatch's task, to record as the agent's start head
	// (SPEC-020 FR-1.1). The dispatcher has no git of its own; an empty
	// answer, or no hook, records no head.
	BranchHead func(ctx context.Context, taskID uuid.UUID) string

	initOnce sync.Once
	kick     chan struct{}
	kickGen  atomic.Int64
	rate     map[string]*rateBucket
	rateMu   sync.Mutex
	workers  chan struct{}
	wg       sync.WaitGroup
	started  sync.Once
}

// ensureInit creates the fields touched from multiple goroutines (Kick is
// called by HTTP handlers and workers). Config-dependent sizing happens in
// defaults, on the Run goroutine only.
func (dp *Dispatcher) ensureInit() {
	dp.initOnce.Do(func() {
		dp.kick = make(chan struct{}, 1)
		dp.rate = map[string]*rateBucket{}
		if dp.BackoffBase == 0 {
			dp.BackoffBase = 2 * time.Second
		}
		if dp.MaxOutputTokens == 0 {
			dp.MaxOutputTokens = 4096
		}
		if dp.Log == nil {
			dp.Log = slog.Default()
		}
	})
}

func (dp *Dispatcher) defaults(cfg *config.Config) {
	if dp.workers == nil {
		dp.workers = make(chan struct{}, cfg.Dispatch.Workers)
	}
}

// Kick asks the dispatcher to re-scan the queue (after enqueue, checkpoint
// answer, or budget change). Kicks coalesce into a single pending signal, but
// the generation counter ensures a Kick issued after a commit always causes a
// scan that observes it: the Run loop re-scans while the generation keeps
// changing, so a Kick can never be lost to coalescing (which previously let a
// just-enqueued dispatch sit until the heartbeat).
func (dp *Dispatcher) Kick() {
	dp.ensureInit()
	dp.kickGen.Add(1)
	select {
	case dp.kick <- struct{}{}:
	default:
	}
}

// Run consumes kicks until ctx ends, scanning the queue and starting
// admitted dispatches on the worker pool.
func (dp *Dispatcher) Run(ctx context.Context) {
	dp.ensureInit()
	cfg, err := dp.Config()
	if err != nil {
		// Boot-time config is validated by serve before Run; a failure here
		// means the file broke after boot. Scans will retry on next kick.
		dp.Log.Error("dispatcher: config", "err", err)
	} else {
		dp.started.Do(func() { dp.defaults(cfg) })
	}
	for {
		select {
		case <-ctx.Done():
			dp.wg.Wait()
			return
		case <-dp.kick:
			// Re-scan while new Kicks arrive during a scan, so a dispatch
			// enqueued just before its Kick is never missed.
			for {
				start := dp.kickGen.Load()
				dp.scan(ctx)
				if dp.kickGen.Load() == start {
					break
				}
			}
		}
	}
}

func (dp *Dispatcher) scan(ctx context.Context) {
	dp.ensureInit()
	cfg, err := dp.Config()
	if err != nil {
		dp.Log.Error("dispatcher: config", "err", err)
		return
	}
	dp.started.Do(func() { dp.defaults(cfg) })
	queued, err := dp.Store.QueuedDispatches(ctx)
	if err != nil {
		dp.Log.Error("dispatcher: queue scan", "err", err)
		return
	}
	for i := range queued {
		d := queued[i]
		admitted, reason, err := dp.admit(ctx, cfg, &d)
		if err != nil {
			dp.Log.Error("dispatcher: governor", "dispatch", d.ID, "err", err)
			continue
		}
		if !admitted {
			_ = dp.Store.SetQueueReason(ctx, d.ID, reason)
			continue
		}
	}
}

// isMutating reports whether a dispatch purpose writes to a worktree — the
// governor serialises these per feature (check 3, DESIGN-006 §5). Reviews
// and verification are read-only and exempt (O-5).
func isMutating(purpose string) bool { return purpose == "implement-task" }

// admit runs governor checks in order (DESIGN-002 §6): budget, provider
// rate, feature serialisation (mutating dispatches only, check 3), worker
// cap. On approval it claims the dispatch and starts a worker.
func (dp *Dispatcher) admit(ctx context.Context, cfg *config.Config, d *store.Dispatch) (bool, string, error) {
	model, ok := cfg.Models[d.Model]
	if !ok {
		return false, "", dp.failPermanently(ctx, d, fmt.Sprintf("model %q not in config.yaml", d.Model))
	}

	// 1. Budget.
	since := PeriodStart(cfg.Budget.Period, time.Now().UTC())
	spent, err := store.PeriodCost(ctx, dp.Store.Pool, since)
	if err != nil {
		return false, "", err
	}
	projected := cfg.Budget.PerDispatchCapUSD
	if spent+projected > cfg.Budget.CapUSD {
		var cp *store.Checkpoint
		err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			var e error
			cp, e = store.CreateCheckpoint(ctx, tx, "budget", d.RefType, d.RefID,
				fmt.Sprintf("Budget cap $%.2f reached ($%.2f spent this %s period). Raise the cap or cancel queued work.",
					cfg.Budget.CapUSD, spent, cfg.Budget.Period),
				map[string]any{"spent_usd": spent, "cap_usd": cfg.Budget.CapUSD, "dispatch_id": d.ID.String()})
			return e
		})
		if err == nil && dp.OnCheckpoint != nil {
			dp.OnCheckpoint(cp)
		}
		return false, "budget", err
	}
	if spent > cfg.Budget.WarnFraction*cfg.Budget.CapUSD {
		_ = dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.Audit(ctx, tx, "orchestrator", "budget.warning", d.RefType, &d.RefID,
				map[string]any{"spent_usd": spent, "cap_usd": cfg.Budget.CapUSD})
		})
	}

	// 2. Feature serialisation (check 3): at most one mutating dispatch per
	// feature at a time, so two implementers never share a worktree
	// (DESIGN-006 §5). Read-only dispatches are exempt.
	// A running implementer, or a claim holding the working copy, holds it
	// (SPEC-020 SD-4). This is the cheap first filter; the start re-checks
	// under the feature's lock.
	if isMutating(d.Purpose) {
		reason, err := store.ImplementHold(ctx, dp.Store.Pool, d.RefID, d.ID)
		if err != nil {
			return false, "", err
		}
		if reason != "" {
			return false, reason, nil
		}
	}

	// 3. Provider rate limit.
	if !dp.takeToken(model.Provider, cfg.Providers[model.Provider].Rate.RequestsPerMinute) {
		return false, "rate-limit", nil
	}

	// 4. Worker cap.
	select {
	case dp.workers <- struct{}{}:
	default:
		return false, "worker-cap", nil
	}
	// The slot is the worker's once it starts; every other exit gives it back.
	started := false
	defer func() {
		if !started {
			<-dp.workers
		}
	}()

	// Claim, freezing the price snapshot (O-4). An implementer starts through
	// StartImplementDispatch, which re-checks every refusal under the
	// feature's lock and records the agent's execution (SPEC-020 FR-2.7).
	var claimed bool
	if isMutating(d.Purpose) {
		var head string
		if dp.BranchHead != nil {
			head = dp.BranchHead(ctx, d.RefID)
		}
		var res store.StartResult
		err = dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			var err error
			res, err = store.StartImplementDispatch(ctx, tx, d.ID, model.PricePerMTok, head)
			return err
		})
		if err != nil {
			return false, "claim-lost", err
		}
		switch res.Outcome {
		case store.StartQueued:
			return false, res.Reason, nil
		case store.StartCancelled:
			return false, "cancelled", nil
		case store.StartStarted:
			claimed = true
		}
	} else {
		err = dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			var err error
			claimed, err = store.MarkDispatchRunning(ctx, tx, d.ID, model.PricePerMTok)
			return err
		})
	}
	if err != nil || !claimed {
		return false, "claim-lost", err
	}

	started = true
	dp.wg.Add(1)
	go func() {
		defer dp.wg.Done()
		defer func() { <-dp.workers }()
		dp.runOne(ctx, d, model.PricePerMTok)
		dp.Kick() // a finished dispatch may unblock the next
	}()
	return true, "", nil
}

// runOne executes one claimed dispatch attempt end to end. The outcome is a
// raw JSON payload from the outcome tool; the rule engine parses it per
// purpose (DESIGN-006 §4.1).
func (dp *Dispatcher) runOne(ctx context.Context, d *store.Dispatch, price config.PricePerMTok) {
	outcome, usage, runErr := dp.runLoop(ctx, d)
	cost := Cost(usage, price)

	if errors.Is(runErr, store.ErrAttemptGivenUp) {
		// The stall sweep already failed this attempt and wrote down why; a
		// retry may be running on the same row. Nothing more to record.
		dp.Log.Warn("dispatcher: attempt given up on while still running", "dispatch", d.ID, "attempt", d.Attempt)
		return
	}
	if runErr != nil {
		err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.MarkDispatchFailed(ctx, tx, d.ID, runErr.Error())
		})
		if err != nil {
			dp.Log.Error("dispatcher: record failure", "dispatch", d.ID, "err", err)
		}
		// The heartbeat retry sweep decides on re-queue vs exhaustion.
		return
	}

	err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.MarkAttemptSucceeded(ctx, tx, d.ID, d.Attempt, store.TokenUsage{
			Input: usage.Input, Output: usage.Output,
			CacheRead: usage.CacheRead, CacheWrite: usage.CacheWrite,
		}, cost, json.RawMessage(outcome))
	})
	if err != nil {
		dp.Log.Error("dispatcher: record success", "dispatch", d.ID, "err", err)
		return
	}
	dp.Bus.Publish(bus.DispatchSucceeded{
		DispatchID: d.ID, Purpose: d.Purpose, Role: d.Role,
		RefType: d.RefType, RefID: d.RefID, Outcome: outcome,
	})
}

// runLoop is the generalised agent loop (DESIGN-006 §4.1): offer the plan's
// tools, execute non-outcome tool calls through the tool host and feed the
// results back, and complete when the outcome tool is called. Phase 1's
// single-tool review is the degenerate case (a plan with only the outcome
// tool and a nil ToolCtx).
func (dp *Dispatcher) runLoop(ctx context.Context, d *store.Dispatch) (json.RawMessage, provider.Usage, error) {
	var total provider.Usage

	plan, err := dp.Planner.Plan(ctx, d)
	if err != nil {
		return nil, total, err
	}
	if plan.ToolCtx != nil {
		// Which run is calling, for tools that record who did something
		// (report_bug, SPEC-019 FR-3.3).
		plan.ToolCtx.DispatchID, plan.ToolCtx.Role = d.ID.String(), d.Role
	}
	cfg, err := dp.Config()
	if err != nil {
		return nil, total, err
	}
	model, ok := cfg.Models[d.Model]
	if !ok {
		return nil, total, fmt.Errorf("model %q not in config.yaml", d.Model)
	}
	prov, err := dp.Providers(model.Provider)
	if err != nil {
		return nil, total, err
	}

	// The transcript starts with what the agent is told, before the first
	// model call, so even a run that fails on its first call shows its prompt
	// (SPEC-012 FR-1.2).
	rec := dp.newRecorder(d, cfg.Transcripts)
	rec.prompts(ctx, plan.System, plan.User)

	msgs := []provider.Message{provider.UserText(plan.User)}
	seq := 0
	for turn := 0; turn < plan.TurnCap; turn++ {
		if err := dp.stillCurrent(ctx, d); err != nil {
			return nil, total, err
		}
		n := turn + 1

		began := time.Now()
		resp, err := dp.completeWithRetry(ctx, prov, provider.Request{
			Model: d.Model, System: plan.System, MaxTokens: dp.MaxOutputTokens,
			Messages: msgs, Tools: plan.Tools,
		})
		if err != nil {
			return nil, total, err
		}
		total.Add(resp.Usage)
		// A model call can outlast the stall threshold. If the attempt was
		// given up on meanwhile, stop here: its transcript already ends with
		// why, and acting on this reply could race a retry (SPEC-012 FR-1.9).
		if err := dp.stillCurrent(ctx, d); err != nil {
			return nil, total, err
		}
		rec.turn(ctx, n, resp, time.Since(began))

		// Collect every tool_use block in the turn. Providers may return
		// parallel tool calls in one assistant message, and the API requires a
		// tool_result for each — answering only the first (as an earlier
		// version did) makes the next request malformed (400).
		var toolUses []*provider.Block
		for i := range resp.Blocks {
			if resp.Blocks[i].Type == "tool_use" {
				toolUses = append(toolUses, &resp.Blocks[i])
			}
		}
		if len(toolUses) == 0 {
			// No tool call this turn: nudge (free text carries no workflow
			// effect, DESIGN-002 §4).
			nudge := "Call a tool to proceed; finish by calling " + plan.OutcomeTool + "."
			msgs = append(msgs,
				provider.Message{Role: "assistant", Blocks: resp.Blocks},
				provider.UserText(nudge))
			rec.nudge(ctx, n, nudge)
			continue
		}

		// Produce exactly one tool_result per tool_use. A valid outcome-tool
		// call completes the dispatch; an invalid one, or any other tool, is
		// answered and the loop continues so the model can proceed.
		results := make([]provider.Block, 0, len(toolUses))
		recorded := make([]store.TranscriptEntry, 0, len(toolUses))
		for _, tu := range toolUses {
			if tu.ToolName == plan.OutcomeTool {
				if plan.ValidateOutcome != nil {
					if verr := plan.ValidateOutcome(tu.ToolInput); verr != nil {
						msg := "Invalid input: " + verr.Error()
						results = append(results, provider.Block{
							Type: "tool_result", ToolUseID: tu.ToolUseID,
							Result: msg, IsError: true,
						})
						recorded = append(recorded, rec.toolResult(n, tu, msg, true, 0))
						continue
					}
				}
				rec.outcome(ctx, n, tu, recorded)
				return tu.ToolInput, total, nil // dispatch complete
			}
			began := time.Now()
			result, isErr := dp.execTool(ctx, plan, tu)
			took := time.Since(began)
			seq++
			dp.recordToolCall(ctx, d.ID, seq, tu.ToolName, len(tu.ToolInput), len(result), int(took.Milliseconds()), isErr)
			results = append(results, provider.Block{
				Type: "tool_result", ToolUseID: tu.ToolUseID, Result: result, IsError: isErr,
			})
			recorded = append(recorded, rec.toolResult(n, tu, result, isErr, took))
		}
		rec.write(ctx, recorded...)
		msgs = append(msgs,
			provider.Message{Role: "assistant", Blocks: resp.Blocks},
			provider.Message{Role: "user", Blocks: results})
	}
	return nil, total, fmt.Errorf("turn cap %d reached without %s", plan.TurnCap, plan.OutcomeTool)
}

// stillCurrent refreshes the attempt's heartbeat and reports
// store.ErrAttemptGivenUp when the attempt is no longer the run's current,
// running one. A database error is not a reason to stop; the stall sweep is
// the backstop.
func (dp *Dispatcher) stillCurrent(ctx context.Context, d *store.Dispatch) error {
	ok, err := dp.Store.HeartbeatAttempt(ctx, d.ID, d.Attempt)
	if err == nil && !ok {
		return store.ErrAttemptGivenUp
	}
	return nil
}

// execTool runs one non-outcome tool call, enforcing the profile as
// defence in depth (the model was only offered profile tools, but the host
// rejects out-of-profile calls too — DESIGN-006 §4.1, FR-6.1).
func (dp *Dispatcher) execTool(ctx context.Context, plan *Plan, tu *provider.Block) (string, bool) {
	if plan.ToolCtx == nil || !plan.ToolCtx.InProfile(tu.ToolName) {
		return fmt.Sprintf("tool %q is not available to this role", tu.ToolName), true
	}
	return dp.Tools.Execute(ctx, plan.ToolCtx, tu.ToolName, tu.ToolInput)
}

// recordToolCall appends the tool ledger row, with the call's measured
// latency (SPEC-012 FR-1.8).
func (dp *Dispatcher) recordToolCall(ctx context.Context, dispatchID uuid.UUID, seq int, tool string, argBytes, resultBytes, latencyMs int, isErr bool) {
	status := "ok"
	if isErr {
		status = "error"
	}
	if err := dp.Store.RecordToolCall(ctx, dispatchID, seq, tool, argBytes, resultBytes, latencyMs, status); err != nil {
		dp.Log.Error("tool-call ledger", "dispatch", dispatchID, "err", err)
	}
}

// completeWithRetry retries transient provider errors with exponential
// backoff inside the attempt (FR-8.2).
func (dp *Dispatcher) completeWithRetry(ctx context.Context, prov provider.Provider, req provider.Request) (*provider.Response, error) {
	const tries = 4
	var last error
	for i := 0; i < tries; i++ {
		resp, err := prov.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		if !provider.IsTransient(err) || ctx.Err() != nil {
			return nil, err
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(dp.BackoffBase << i):
		}
	}
	return nil, fmt.Errorf("provider unavailable after %d tries: %w", tries, last)
}

// failPermanently marks a dispatch failed for a non-retryable reason and
// reports no admission error to the scan loop.
func (dp *Dispatcher) failPermanently(ctx context.Context, d *store.Dispatch, reason string) error {
	return dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.MarkDispatchFailed(ctx, tx, d.ID, reason)
	})
}

// RetrySweep is heartbeat duty 2 (DESIGN-002 §3): re-queue failed
// dispatches with attempts remaining once their backoff elapses; publish
// exhaustion for the rest so the rule engine raises checkpoints.
func (dp *Dispatcher) RetrySweep(ctx context.Context) {
	dp.ensureInit()
	cfg, err := dp.Config()
	if err != nil {
		return
	}
	dp.started.Do(func() { dp.defaults(cfg) })
	retry, exhausted, err := dp.Store.FailedRetryable(ctx, cfg.Dispatch.MaxAttempts, dp.BackoffBase)
	if err != nil {
		dp.Log.Error("retry sweep", "err", err)
		return
	}
	requeued := 0
	for i := range retry {
		d := retry[i]
		// An implement dispatch whose task has been claimed isn't revived: it
		// is cancelled (SPEC-020 FR-2.7).
		var ok bool
		err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			var err error
			ok, err = store.RequeueUnlessClaimed(ctx, tx, d.ID, "orchestrator")
			return err
		})
		if err != nil {
			dp.Log.Error("retry sweep: requeue", "dispatch", d.ID, "err", err)
		} else if ok {
			requeued++
		}
	}
	if requeued > 0 {
		dp.Kick()
	}
	for i := range exhausted {
		d := exhausted[i]
		// An exhausted implement dispatch of a claimed task raises nothing: the
		// claim took the task, and the dispatch is cancelled.
		if d.Purpose == "implement-task" {
			var ok bool
			err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
				var err error
				ok, err = store.CancelIfTaskClaimed(ctx, tx, d.ID, "orchestrator")
				return err
			})
			if err == nil && ok {
				continue
			}
		}
		msg := ""
		if d.Error != nil {
			msg = *d.Error
		}
		dp.Bus.Publish(bus.DispatchExhausted{
			DispatchID: d.ID, Purpose: d.Purpose,
			RefType: d.RefType, RefID: d.RefID, Error: msg,
		})
	}
}

// StallSweep is heartbeat duty 1: running dispatches without a recent
// heartbeat are failed (reason: stalled) and fed to the retry policy.
func (dp *Dispatcher) StallSweep(ctx context.Context) {
	cfg, err := dp.Config()
	if err != nil {
		return
	}
	stalled, err := dp.Store.StalledRunning(ctx, time.Duration(cfg.Dispatch.StallSeconds)*time.Second)
	if err != nil {
		dp.Log.Error("stall sweep", "err", err)
		return
	}
	for i := range stalled {
		d := stalled[i]
		err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.MarkDispatchFailed(ctx, tx, d.ID, "stalled: no heartbeat")
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			dp.Log.Error("stall sweep: fail", "dispatch", d.ID, "err", err)
		}
	}
}

// PeriodStart returns the UTC start of the budget period (DESIGN-004 §4).
func PeriodStart(period string, now time.Time) time.Time {
	switch period {
	case "monthly":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case "weekly":
		// ISO week: back up to Monday.
		d := int(now.Weekday()+6) % 7
		day := now.AddDate(0, 0, -d)
		return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	default: // total
		return time.Time{}
	}
}

type rateBucket struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

// takeToken implements a per-provider requests-per-minute token bucket
// (governor check 2).
func (dp *Dispatcher) takeToken(providerName string, perMinute int) bool {
	if perMinute <= 0 {
		return true
	}
	dp.rateMu.Lock()
	b, ok := dp.rate[providerName]
	if !ok {
		b = &rateBucket{tokens: float64(perMinute), last: time.Now()}
		dp.rate[providerName] = b
	}
	dp.rateMu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens += now.Sub(b.last).Minutes() * float64(perMinute)
	if max := float64(perMinute); b.tokens > max {
		b.tokens = max
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
