package server

import (
	"context"
	"time"
)

// Seeing the work (SPEC-012): transcript retention, and the shared helpers
// behind the transcript viewer, the feature timeline and review health.

// pruneEvery paces the retention sweep. Retention is counted in days, so
// running it on every heartbeat would only repeat a query that finds nothing.
const pruneEvery = time.Hour

// PruneTranscriptsSweep is a heartbeat duty (SPEC-012 FR-2.4): it deletes the
// transcripts of runs that finished longer ago than the configured retention,
// at most once an hour. Outcomes, tokens and the tool ledger are kept.
func (s *Server) PruneTranscriptsSweep(ctx context.Context, now time.Time) {
	if !s.lastPrune.IsZero() && now.Sub(s.lastPrune) < pruneEvery {
		return
	}
	s.lastPrune = now
	cfg, err := s.freshConfig()
	if err != nil {
		return
	}
	n, err := s.Store.PruneTranscripts(ctx, cfg.Transcripts.Retention())
	if err != nil {
		s.Log.Error("transcript retention", "err", err)
		return
	}
	if n > 0 {
		s.Log.Info("transcript retention", "entries_removed", n, "retention_days", cfg.Transcripts.Retention())
	}
}
