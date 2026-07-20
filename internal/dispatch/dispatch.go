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
	"time"

	"github.com/jackc/pgx/v5"

	"cromwell/internal/bus"
	"cromwell/internal/config"
	"cromwell/internal/provider"
	"cromwell/internal/rules"
	"cromwell/internal/store"
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
					"type": "string",
					"enum": []string{"approve", "request_changes", "escalate"},
					"description": "approve: the document meets the bar. request_changes: fixable problems, listed in comments. escalate: a human must decide; explain why in reasoning.",
				},
				"comments": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"section_ref": map[string]any{"type": "string", "description": "Heading of the section the comment is about, empty for whole-document comments"},
							"body":        map[string]any{"type": "string"},
						},
						"required": []string{"body"},
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

// PromptBuilder assembles the prompt for a claimed dispatch. Implemented by
// the server (it owns store queries and fresh config reads, O-6).
type PromptBuilder interface {
	BuildReview(ctx context.Context, d *store.Dispatch) (system, user string, turnCap int, err error)
}

// ProviderFactory returns the provider client for a model's provider name.
type ProviderFactory func(providerName string) (provider.Provider, error)

type Dispatcher struct {
	Store     *store.Store
	Bus       *bus.Bus
	Config    func() (*config.Config, error) // fresh read per use (O-6)
	Builder   PromptBuilder
	Providers ProviderFactory
	Log       *slog.Logger

	// BackoffBase paces in-attempt transient retries and the attempt-level
	// retry sweep; tests shrink it.
	BackoffBase time.Duration
	// MaxOutputTokens per model call.
	MaxOutputTokens int

	initOnce sync.Once
	kick     chan struct{}
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
// answer, or budget change). Coalesces.
func (dp *Dispatcher) Kick() {
	dp.ensureInit()
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
			dp.scan(ctx)
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

// admit runs governor checks in order (DESIGN-002 §6): budget, provider
// rate, worker cap (feature serialisation arrives with mutating dispatches
// in phase 2 — reviews are read-only and exempt). On approval it claims the
// dispatch and starts a worker.
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
		err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			_, err := store.CreateCheckpoint(ctx, tx, "budget", d.RefType, d.RefID,
				fmt.Sprintf("Budget cap $%.2f reached ($%.2f spent this %s period). Raise the cap or cancel queued work.",
					cfg.Budget.CapUSD, spent, cfg.Budget.Period),
				map[string]any{"spent_usd": spent, "cap_usd": cfg.Budget.CapUSD, "dispatch_id": d.ID.String()})
			return err
		})
		return false, "budget", err
	}
	if spent > cfg.Budget.WarnFraction*cfg.Budget.CapUSD {
		_ = dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.Audit(ctx, tx, "orchestrator", "budget.warning", d.RefType, &d.RefID,
				map[string]any{"spent_usd": spent, "cap_usd": cfg.Budget.CapUSD})
		})
	}

	// 2. Provider rate limit.
	if !dp.takeToken(model.Provider, cfg.Providers[model.Provider].Rate.RequestsPerMinute) {
		return false, "rate-limit", nil
	}

	// 3. Worker cap.
	select {
	case dp.workers <- struct{}{}:
	default:
		return false, "worker-cap", nil
	}

	// Claim, freezing the price snapshot (O-4).
	var claimed bool
	err = dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		claimed, err = store.MarkDispatchRunning(ctx, tx, d.ID, model.PricePerMTok)
		return err
	})
	if err != nil || !claimed {
		<-dp.workers
		return false, "claim-lost", err
	}

	dp.wg.Add(1)
	go func() {
		defer dp.wg.Done()
		defer func() { <-dp.workers }()
		dp.runOne(ctx, d, model.PricePerMTok)
		dp.Kick() // a finished dispatch may unblock the next
	}()
	return true, "", nil
}

// runOne executes one claimed dispatch attempt end to end.
func (dp *Dispatcher) runOne(ctx context.Context, d *store.Dispatch, price config.PricePerMTok) {
	outcome, usage, runErr := dp.runReviewLoop(ctx, d)
	cost := Cost(usage, price)

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

	raw, _ := json.Marshal(outcome)
	err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.MarkDispatchSucceeded(ctx, tx, d.ID, store.TokenUsage{
			Input: usage.Input, Output: usage.Output,
			CacheRead: usage.CacheRead, CacheWrite: usage.CacheWrite,
		}, cost, outcome)
	})
	if err != nil {
		dp.Log.Error("dispatcher: record success", "dispatch", d.ID, "err", err)
		return
	}
	dp.Bus.Publish(bus.DispatchSucceeded{
		DispatchID: d.ID, Purpose: d.Purpose, Role: d.Role,
		RefType: d.RefType, RefID: d.RefID, Outcome: raw,
	})
}

// runReviewLoop drives the agent until it calls submit_review, nudging when
// it stops without doing so, up to the turn cap.
func (dp *Dispatcher) runReviewLoop(ctx context.Context, d *store.Dispatch) (*rules.ReviewOutcome, provider.Usage, error) {
	var total provider.Usage

	system, user, turnCap, err := dp.Builder.BuildReview(ctx, d)
	if err != nil {
		return nil, total, err
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

	msgs := []provider.Message{provider.UserText(user)}
	for turn := 0; turn < turnCap; turn++ {
		_ = dp.Store.Heartbeat(ctx, d.ID)

		resp, err := dp.completeWithRetry(ctx, prov, provider.Request{
			Model: d.Model, System: system, MaxTokens: dp.MaxOutputTokens,
			Messages: msgs, Tools: []provider.ToolDef{ReviewOutcomeTool()},
		})
		if err != nil {
			return nil, total, err
		}
		total.Add(resp.Usage)

		if tu := resp.ToolUse(); tu != nil && tu.ToolName == "submit_review" {
			outcome, perr := rules.ParseReviewOutcome(tu.ToolInput)
			if perr == nil {
				return outcome, total, nil
			}
			// Malformed payload: hand the error back as a tool result so the
			// model can correct itself.
			msgs = append(msgs,
				provider.Message{Role: "assistant", Blocks: resp.Blocks},
				provider.Message{Role: "user", Blocks: []provider.Block{{
					Type: "tool_result", ToolUseID: tu.ToolUseID,
					Result: "Invalid input: " + perr.Error(), IsError: true,
				}}})
			continue
		}

		// No outcome call this turn: nudge (free text carries no workflow
		// effect, DESIGN-002 §4).
		msgs = append(msgs,
			provider.Message{Role: "assistant", Blocks: resp.Blocks},
			provider.UserText("Complete the review now by calling submit_review with your verdict."))
	}
	return nil, total, fmt.Errorf("turn cap %d reached without submit_review", turnCap)
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
	for i := range retry {
		d := retry[i]
		err := dp.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.RequeueDispatch(ctx, tx, d.ID)
		})
		if err != nil {
			dp.Log.Error("retry sweep: requeue", "dispatch", d.ID, "err", err)
		}
	}
	if len(retry) > 0 {
		dp.Kick()
	}
	for i := range exhausted {
		d := exhausted[i]
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
