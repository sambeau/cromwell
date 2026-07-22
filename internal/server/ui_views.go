package server

import (
	"context"
	"html/template"
	"net/http"

	"cromwell/internal/bus"
	"cromwell/internal/config"
	"cromwell/internal/sizing"
	"cromwell/internal/store"
)

// View assembly for the read surfaces (SPEC-004 FR-4, FR-5, FR-6). Each view is
// built from existing store reads and the pure sizing engine — nothing here
// re-derives a roll-up or a cost the engine already computes (NFR-2).

// busCheckpointResponded builds the resume event the CLI's respond path
// publishes, so the UI feeds the rule engine identically (FR-3.2, CC-4).
func busCheckpointResponded(cp *store.Checkpoint, actor string) bus.CheckpointResponded {
	return bus.CheckpointResponded{
		CheckpointID: cp.ID, Kind: cp.Kind, RefType: cp.RefType, RefID: cp.RefID,
		Context: cp.Context, Response: cp.Response, RespondedBy: actor,
	}
}

// answerOptions lists the answer verbs the rule engine accepts for a checkpoint
// kind, so the inbox offers the right buttons (FR-3.2). The verbs map to the
// kind's structured response payload in responseFor; the rule engine
// (rules.decideCheckpointResponded) remains the arbiter of the resulting action.
func answerOptions(kind string) []string {
	switch kind {
	case "review-escalation", "verification-escalation":
		return []string{"approve", "request_changes"}
	case "gate-override":
		return []string{"override", "deny"}
	case "dispatch-failure":
		return []string{"retry", "cancel"}
	case "revision-in-flight":
		return []string{"continue", "pause"}
	case "budget":
		return []string{"proceed"}
	default:
		// worktree-failure, merge-conflict, config-error, document-integrity:
		// manual-fix checkpoints the engine takes no follow-up action on;
		// answering acknowledges and clears them.
		return []string{"acknowledge"}
	}
}

// responseFor maps a UI verb to the structured response payload the rule engine
// expects for a checkpoint kind (rules.EscalationResponse / OverrideResponse /
// FailureResponse / RevisionResponse). This is the single place the UI's verbs
// become the engine's schema, so the UI drives the identical
// CheckpointResponded path the CLI does (FR-3.2, CC-4) — no new authority. An
// optional free-text reason rides along where the schema carries one.
func responseFor(kind, verb, reason string) map[string]any {
	r := map[string]any{}
	if reason != "" {
		r["reason"] = reason
	}
	switch kind {
	case "review-escalation", "verification-escalation":
		r["decision"] = verb // approve | request_changes
	case "gate-override":
		r["override"] = verb == "override"
	case "dispatch-failure":
		r["retry"] = verb == "retry"
	case "revision-in-flight":
		r["continue"] = verb == "continue"
	case "budget":
		// Any answer re-kicks the queue; record the verb for the audit trail.
		r["answer"] = verb
	default:
		// Manual-fix checkpoints: record the acknowledgement; no engine action.
		r["answer"] = verb
	}
	return r
}

// --- Planning tree ---

// planningView is the initiative → feature → task tree with roll-ups, plus
// milestones with progress and roadmaps in order (FR-4).
type planningView struct {
	Roots      []treeNode
	Milestones []store.MilestoneProgress
	Roadmaps   []roadmapView
}

// treeNode is one node of the rendered planning tree: its type and label, its
// subtree roll-up computed through the sizing engine (FR-4.1), and its
// children.
type treeNode struct {
	Type     string
	Name     string
	Rollup   sizing.Rollup
	Children []treeNode
}

// toTreeNode derives a display node from a sizing node, computing each node's
// own subtree roll-up with sizing.RollUp — the same computation FR-4.1 asserts
// against, never a re-derivation.
func toTreeNode(n sizing.Node) treeNode {
	tn := treeNode{Type: n.Ref.Type, Name: n.Ref.Name, Rollup: sizing.RollUp(n)}
	for _, c := range n.Children {
		tn.Children = append(tn.Children, toTreeNode(c))
	}
	return tn
}

func (s *Server) planningView(r *http.Request) (planningView, error) {
	ctx := r.Context()
	roots, err := store.RootInitiatives(ctx, s.Store.Pool)
	if err != nil {
		return planningView{}, err
	}
	var nodes []treeNode
	for _, in := range roots {
		sn, err := store.InitiativeSizingNode(ctx, s.Store.Pool, in.ID)
		if err != nil {
			return planningView{}, err
		}
		nodes = append(nodes, toTreeNode(sn))
	}

	ms, err := store.MilestonesWithProgress(ctx, s.Store.Pool)
	if err != nil {
		return planningView{}, err
	}
	roadmaps, err := s.roadmapViews(ctx)
	if err != nil {
		return planningView{}, err
	}
	return planningView{Roots: nodes, Milestones: ms, Roadmaps: roadmaps}, nil
}

// roadmapView is a roadmap with its milestones resolved and in position order
// (FR-4.2).
type roadmapView struct {
	Roadmap store.Roadmap
	Entries []roadmapEntryView
}

type roadmapEntryView struct {
	Position  int
	Milestone store.Milestone
	Progress  store.Progress
	Locked    bool
}

func (s *Server) roadmapViews(ctx context.Context) ([]roadmapView, error) {
	rms, err := store.ListRoadmaps(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	out := make([]roadmapView, 0, len(rms))
	for _, rm := range rms {
		entries, err := store.RoadmapEntries(ctx, s.Store.Pool, rm.ID)
		if err != nil {
			return nil, err
		}
		rv := roadmapView{Roadmap: rm}
		for _, e := range entries {
			m, err := store.GetMilestone(ctx, s.Store.Pool, e.MilestoneID)
			if err != nil {
				return nil, err
			}
			locked := m.LockedAt != nil
			var prog store.Progress
			if locked {
				prog, err = store.SnapshotProgress(ctx, s.Store.Pool, m.ID)
			} else {
				prog, err = store.LiveProgress(ctx, s.Store.Pool, m.ID)
			}
			if err != nil {
				return nil, err
			}
			rv.Entries = append(rv.Entries, roadmapEntryView{
				Position: e.Position, Milestone: *m, Progress: prog, Locked: locked})
		}
		out = append(out, rv)
	}
	return out, nil
}

// --- Documents ---

// docPageData is a single document with its rendered body and comment thread
// (FR-5.1). The body is read from the working tree (git) and rendered read-only;
// there is no edit control (CC-5).
type docPageData struct {
	Document store.Document
	Body     template.HTML
	Comments []store.Comment
}

func (s *Server) documentView(r *http.Request) (*docPageData, error) {
	ctx := r.Context()
	path := r.URL.Query().Get("path")
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err != nil {
		return nil, err
	}
	comments, err := store.CommentsForDocument(ctx, s.Store.Pool, doc.ID, false)
	if err != nil {
		return nil, err
	}
	var body string
	if raw, err := s.readDocFile(doc.Path); err == nil {
		// Strip the YAML front matter so the read view shows only the prose;
		// a file without front matter renders whole.
		if _, md, ferr := config.SplitFrontMatter(string(raw)); ferr == nil {
			body = md
		} else {
			body = string(raw)
		}
	} else {
		body = "_The document file could not be read from the working tree._"
	}
	return &docPageData{
		Document: *doc,
		Body:     renderMarkdown(body),
		Comments: comments,
	}, nil
}

// --- Cost ---

// costView is the extended roll-up surface (FR-6.1): the grand total plus
// per-initiative (transitive), per-feature, per-milestone (resolved members),
// per-roadmap, and per-month costs, all from the frozen-price ledger.
type costView struct {
	Total       float64
	Initiatives []entityCost
	Features    []entityCost
	Milestones  []entityCost
	Roadmaps    []entityCost
	Months      []store.MonthCost
}

// entityCost is one labelled cost line.
type entityCost struct {
	Name string
	Cost float64
}

func (s *Server) costView(r *http.Request) (costView, error) {
	ctx := r.Context()
	var view costView

	rollup, err := s.Store.CostRollup(ctx)
	if err != nil {
		return view, err
	}
	for _, row := range rollup {
		view.Total += row.CostUSD
	}

	// Every initiative in the tree (roots + nested), each transitive; and every
	// feature under them.
	inits, err := s.allInitiatives(ctx)
	if err != nil {
		return view, err
	}
	for _, in := range inits {
		c, err := store.InitiativeCost(ctx, s.Store.Pool, in.ID)
		if err != nil {
			return view, err
		}
		view.Initiatives = append(view.Initiatives, entityCost{Name: in.Slug, Cost: c})

		feats, err := store.FeaturesForInitiative(ctx, s.Store.Pool, in.ID)
		if err != nil {
			return view, err
		}
		for _, f := range feats {
			fc, err := store.FeatureCost(ctx, s.Store.Pool, f.ID)
			if err != nil {
				return view, err
			}
			view.Features = append(view.Features, entityCost{Name: in.Slug + "/" + f.Slug, Cost: fc})
		}
	}

	// Milestones over their resolved members.
	ms, err := store.MilestonesWithProgress(ctx, s.Store.Pool)
	if err != nil {
		return view, err
	}
	for _, m := range ms {
		mc, err := store.MilestoneCost(ctx, s.Store.Pool, m.Progress.Leaves)
		if err != nil {
			return view, err
		}
		view.Milestones = append(view.Milestones, entityCost{Name: m.Milestone.Name, Cost: mc})
	}

	// Roadmaps.
	rms, err := store.ListRoadmaps(ctx, s.Store.Pool)
	if err != nil {
		return view, err
	}
	for _, rm := range rms {
		rc, err := store.RoadmapCost(ctx, s.Store.Pool, rm.ID)
		if err != nil {
			return view, err
		}
		view.Roadmaps = append(view.Roadmaps, entityCost{Name: rm.Name, Cost: rc})
	}

	// Months.
	months, err := store.CostByMonth(ctx, s.Store.Pool)
	if err != nil {
		return view, err
	}
	view.Months = months
	return view, nil
}

// allInitiatives flattens the initiative tree (roots first, then descendants in
// slug order) for the per-initiative cost and feature enumeration.
func (s *Server) allInitiatives(ctx context.Context) ([]store.Initiative, error) {
	roots, err := store.RootInitiatives(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	var out []store.Initiative
	var walk func(in store.Initiative) error
	walk = func(in store.Initiative) error {
		out = append(out, in)
		children, err := store.ChildInitiatives(ctx, s.Store.Pool, in.ID)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	for _, in := range roots {
		if err := walk(in); err != nil {
			return nil, err
		}
	}
	return out, nil
}
