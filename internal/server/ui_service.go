package server

import (
	"context"

	"github.com/google/uuid"

	"cromwell/internal/sizing"
	"cromwell/internal/store"
)

// The UI service layer: shared read assembly both renderings could use
// (DESIGN-007 §4). DashboardSummary gathers the situational picture in one
// call so a page (or a JSON client) renders it at the edge — no self-HTTP, no
// N round trips (CC-2, CC-7).

// DashboardSummary is Home's whole situational picture, led by what needs doing
// (SPEC-007 FR-7.1): the checkpoints awaiting the operator, the dispatches in
// flight, features grouped by lifecycle state, the work not yet estimated, and
// the project's own entry points — its top-level design writing, its top
// initiatives, and its project-level milestones and roadmaps. It is a read-only
// snapshot; realtime refreshes it by re-reading, never by mutation (SD-5).
//
// No figure here is money. The engine's cost ledger still exists and is
// untouched, but it is dormant behind the surface (DESIGN-008 D-4, Q-C): work is
// counted in tokens, the honest unit for agent labour.
type DashboardSummary struct {
	Running   []store.Dispatch   // in-flight dispatches
	Queued    []store.Dispatch   // queued, each with its QueueReason
	Escalated int                // pending checkpoints awaiting a human
	Waiting   []store.Checkpoint // the checkpoints themselves, to act on from Home
	Recent    []store.AuditEvent // recent audit rows, oldest first

	// The work picture, in tokens.
	Project     sizing.Rollup       // the whole project's roll-up
	ByState     []featureStateGroup // features grouped by lifecycle state
	Unestimated []sizing.Ref        // work carrying no estimate yet (the `?`)
	Calibration []store.CorpusRow   // recent estimate-vs-actual reference points

	// Entry points into the browsable surface.
	TopInitiatives []entityWork
	Milestones     []milestoneCard
	Roadmaps       []roadmapCard
	ProjectDocs    []docCard
}

// featureStateGroup is one lifecycle state and the features in it, so Home can
// show the shape of the work at a glance (FR-7.1).
type featureStateGroup struct {
	State    string
	Features []entityWork
}

// DashboardSummary assembles Home's read (FR-7.1). Every figure comes from an
// existing store read or the pure sizing engine — nothing is re-derived. Errors
// on the core reads are returned; calibration and the recent tail are
// best-effort context and never blank the page.
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

	sum := &DashboardSummary{
		Running:   running,
		Queued:    queued,
		Escalated: len(pending),
		Waiting:   pending,
		Recent:    recent,
	}

	// The project's own roll-up, and the unestimated work it names (the `?`).
	proj, err := s.projectRollup(ctx)
	if err != nil {
		return nil, err
	}
	sum.Project = proj
	sum.Unestimated = proj.Unestimated

	// Features grouped by lifecycle state, in the order work moves through them.
	groups := map[string][]entityWork{}
	inits, err := s.allInitiatives(ctx)
	if err != nil {
		return nil, err
	}
	for _, in := range inits {
		path, err := s.initiativePath(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		if in.ParentID == nil && !in.Archived {
			roll, err := s.initiativeRollup(ctx, in.ID)
			if err != nil {
				return nil, err
			}
			sum.TopInitiatives = append(sum.TopInitiatives, entityWork{
				Name: in.Name, URL: "/ui/i/" + path, Size: roll})
		}
		feats, err := store.FeaturesForInitiative(ctx, s.Store.Pool, in.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range feats {
			roll, err := s.featureRollup(ctx, f.ID)
			if err != nil {
				return nil, err
			}
			state := string(f.State)
			groups[state] = append(groups[state], entityWork{
				Name: f.Name, URL: "/ui/f/" + path + "/" + f.Slug, Size: roll})
		}
	}
	for _, state := range []string{"idea", "ready", "active", "review", "done", "abandoned"} {
		if fs, ok := groups[state]; ok {
			sum.ByState = append(sum.ByState, featureStateGroup{State: state, Features: fs})
		}
	}

	// Project-level entry points: the design writing and planning parented at
	// the project itself (FR-8.1).
	if docs, err := store.DocumentsForOwner(ctx, s.Store.Pool, "project", nil); err == nil {
		sum.ProjectDocs = s.documentCards(docs, uuid.Nil)
	}
	if ms, err := s.ownedMilestoneCards(ctx, "project", nil); err == nil {
		sum.Milestones = ms
	}
	if rms, err := s.ownedRoadmapCards(ctx, "project", nil); err == nil {
		sum.Roadmaps = rms
	}

	// Calibration is context; a failure here must not blank Home.
	if cal, err := store.RecentCalibration(ctx, s.Store.Pool, 5); err == nil {
		sum.Calibration = cal
	}
	return sum, nil
}
