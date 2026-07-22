package server

import (
	"context"

	"cromwell/internal/store"
)

// The UI service layer: shared read assembly both renderings could use
// (DESIGN-007 §4). DashboardSummary gathers the situational picture in one
// call so a page (or a JSON client) renders it at the edge — no self-HTTP, no
// N round trips (CC-2, CC-7).

// DashboardSummary is the dashboard's whole situational picture (vision §11,
// FR-2.1): the live queue, the recent event stream, cost burn against budget,
// and calibration health. It is a read-only snapshot; realtime refreshes it by
// re-reading, never by mutation (SD-5).
type DashboardSummary struct {
	Running      []store.Dispatch   // in-flight dispatches
	Queued       []store.Dispatch   // queued, each with its QueueReason
	Escalated    int                // pending checkpoints awaiting a human
	Recent       []store.AuditEvent // recent audit rows, oldest first
	CostTotal    float64            // total spend from the frozen-price ledger
	BudgetCap    float64            // budget.cap_usd
	BudgetWarn   float64            // absolute warn threshold (cap * warn_fraction)
	BudgetPeriod string             // monthly | weekly | total
	OverWarn     bool               // spend has crossed the warn threshold
	OverCap      bool               // spend has crossed the cap
	Calibration  []store.CorpusRow  // recent estimate-vs-actual reference points
}

// DashboardSummary assembles the dashboard read (FR-2.1). Every figure comes
// from an existing store read; the budget frame comes from the live config
// (O-6). Errors on the core reads are returned; calibration and the recent tail
// are best-effort context and never fail the page.
func (s *Server) DashboardSummary(ctx context.Context, recentLimit int) (*DashboardSummary, error) {
	if recentLimit <= 0 {
		recentLimit = 20
	}
	running, err := s.Store.RunningDispatches(ctx)
	if err != nil {
		return nil, err
	}
	queued, err := s.Store.QueuedDispatches(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := s.Store.PendingCheckpoints(ctx)
	if err != nil {
		return nil, err
	}
	recent, err := s.Store.AuditTail(ctx, "", nil, recentLimit)
	if err != nil {
		return nil, err
	}
	rollup, err := s.Store.CostRollup(ctx)
	if err != nil {
		return nil, err
	}
	var total float64
	for _, row := range rollup {
		total += row.CostUSD
	}

	sum := &DashboardSummary{
		Running:   running,
		Queued:    queued,
		Escalated: len(pending),
		Recent:    recent,
		CostTotal: total,
	}

	// Budget frame from the live config; if config is momentarily unreadable the
	// numbers still render, just without the cap overlay.
	if cfg, err := s.freshConfig(); err == nil {
		sum.BudgetCap = cfg.Budget.CapUSD
		sum.BudgetPeriod = cfg.Budget.Period
		sum.BudgetWarn = cfg.Budget.CapUSD * cfg.Budget.WarnFraction
		sum.OverWarn = sum.BudgetWarn > 0 && total >= sum.BudgetWarn
		sum.OverCap = sum.BudgetCap > 0 && total >= sum.BudgetCap
	}

	// Calibration is context; a failure here must not blank the dashboard.
	if cal, err := store.RecentCalibration(ctx, s.Store.Pool, 5); err == nil {
		sum.Calibration = cal
	}
	return sum, nil
}
