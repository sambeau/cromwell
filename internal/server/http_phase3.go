package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/ident"
	"subutai/internal/sizing"
	"subutai/internal/store"
)

// Phase-3 HTTP surface (SPEC-003): estimates and roll-ups, milestones, roadmaps,
// and extended cost. The CLI is the only caller; every mutation is transactional
// with its audit rows in the store layer.

func (s *Server) routesPhase3(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/estimate", s.handleEstimate)
	mux.HandleFunc("POST /api/estimate/set", s.handleEstimateSet)
	mux.HandleFunc("POST /api/estimate/ai", s.handleEstimateAI)
	mux.HandleFunc("GET /api/milestones", s.handleListMilestones)
	mux.HandleFunc("GET /api/milestone", s.handleGetMilestone)
	mux.HandleFunc("POST /api/milestones", s.handleCreateMilestone)
	mux.HandleFunc("POST /api/milestones/members", s.handleMilestoneMember)
	mux.HandleFunc("POST /api/milestones/lock", s.handleLockMilestone)
	mux.HandleFunc("POST /api/roadmaps", s.handleCreateRoadmap)
	mux.HandleFunc("POST /api/roadmaps/entries", s.handleRoadmapEntry)
	mux.HandleFunc("GET /api/roadmap", s.handleGetRoadmap)
	mux.HandleFunc("GET /api/cost/rollup", s.handleCostRollup)
	mux.HandleFunc("GET /api/cost/months", s.handleCostMonths)
}

// --- Estimates ---

// unestimatedRef is a leaf named in a `?` listing.
type unestimatedRef struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// estimateResponse is the roll-up for one entity plus, for a leaf, its estimate
// vs actual and re-estimation history (FR-2.2, FR-3.1).
type estimateResponse struct {
	Ref          string           `json:"ref"`
	RefType      string           `json:"ref_type"`
	Tokens       int64            `json:"tokens"`
	Tier         string           `json:"tier"`
	Estimated    bool             `json:"estimated"`
	Decomposed   bool             `json:"decomposed"`
	Complete     bool             `json:"complete"`
	Unestimated  []unestimatedRef `json:"unestimated"`
	ActualTokens int64            `json:"actual_tokens"`
	Delta        int64            `json:"delta"` // actual - tokens, when both known
	History      []estimateRow    `json:"history"`
}

type estimateRow struct {
	Tokens    int64  `json:"tokens"`
	Tier      string `json:"tier"`
	Rationale string `json:"rationale"`
	CreatedAt string `json:"created_at"`
}

func (s *Server) handleEstimate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		writeErr(w, 400, errors.New("missing ref"))
		return
	}
	refType, refID, err := s.resolveRef(ctx, ref)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	node, err := s.sizingNode(ctx, refType, refID)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	roll := sizing.RollUp(node)

	resp := estimateResponse{
		Ref: ref, RefType: refType, Tokens: roll.Tokens, Tier: string(roll.Tier),
		Estimated: roll.Estimated, Decomposed: roll.Decomposed, Complete: roll.Complete,
		Unestimated: []unestimatedRef{},
	}
	for _, u := range roll.Unestimated {
		resp.Unestimated = append(resp.Unestimated, unestimatedRef{Type: u.Type, Name: u.Name})
	}

	// Actuals and history are meaningful for the estimated entities.
	if refType == "feature" || refType == "task" || refType == "initiative" {
		if actual, aerr := store.ActualTokens(ctx, s.Store.Pool, refType, refID); aerr == nil {
			resp.ActualTokens = actual
			if roll.Estimated && actual > 0 {
				resp.Delta = actual - roll.Tokens
			}
		}
	}
	if refType == "feature" || refType == "task" {
		hist, _ := store.EstimateHistory(ctx, s.Store.Pool, refType, refID)
		for _, e := range hist {
			resp.History = append(resp.History, estimateRow{
				Tokens: e.Tokens, Tier: string(e.Tier), Rationale: e.Rationale,
				CreatedAt: e.CreatedAt.Format(time.RFC3339),
			})
		}
	}
	writeJSON(w, 200, resp)
}

// sizingNode loads the roll-up tree for any estimatable ref.
func (s *Server) sizingNode(ctx context.Context, refType string, refID uuid.UUID) (sizing.Node, error) {
	switch refType {
	case "initiative":
		return store.InitiativeSizingNode(ctx, s.Store.Pool, refID)
	case "feature":
		return store.FeatureSizingNode(ctx, s.Store.Pool, refID)
	case "task":
		t, err := store.GetTask(ctx, s.Store.Pool, refID)
		if err != nil {
			return sizing.Node{}, err
		}
		label := t.LocalID
		if label == "" {
			label = t.Title
		}
		n := sizing.Node{Ref: sizing.Ref{Type: "task", ID: t.ID, Name: label}}
		if e, err := store.CurrentEstimate(ctx, s.Store.Pool, "task", refID); err == nil {
			n.Estimate = &sizing.UnitEstimate{Tokens: e.Tokens, Tier: e.Tier}
		}
		return n, nil
	}
	return sizing.Node{}, errors.New("unsupported ref type for sizing")
}

func (s *Server) handleEstimateSet(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Ref        string `json:"ref"`
		Tokens     int64  `json:"tokens"`
		Rationale  string `json:"rationale"`
		CiteCorpus bool   `json:"cite_corpus"`
	}](w, r)
	if !ok {
		return
	}
	if req.Tokens <= 0 {
		writeErr(w, 400, errors.New("tokens must be positive"))
		return
	}
	ctx := r.Context()
	refType, refID, err := s.resolveRef(ctx, req.Ref)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	if refType != "feature" && refType != "task" {
		writeErr(w, 400, errors.New("estimates attach to features and tasks only"))
		return
	}
	// A human estimate is rough judgement, or considered when it cites the
	// corpus (FR-1.3). The tier follows the evidence, not a free choice.
	tier := sizing.TierRough
	if req.CiteCorpus {
		tier = sizing.TierConsidered
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.RecordEstimate(ctx, tx, refType, refID, req.Tokens, tier, req.Rationale, nil, actor(r))
		return err
	})
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"ref": req.Ref, "tokens": req.Tokens, "tier": string(tier)})
}

func (s *Server) handleEstimateAI(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Ref string `json:"ref"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	refType, refID, err := s.resolveRef(ctx, req.Ref)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	if err := s.enqueueEstimate(ctx, refType, refID); err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 202, map[string]any{"ref": req.Ref, "queued": true})
}

// --- Milestones ---

func (s *Server) handleCreateMilestone(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Name       string `json:"name"`
		TargetDate string `json:"target_date"`
	}](w, r)
	if !ok {
		return
	}
	var target *time.Time
	if req.TargetDate != "" {
		td, err := time.Parse("2006-01-02", req.TargetDate)
		if err != nil {
			writeErr(w, 400, errors.New("target-date must be YYYY-MM-DD"))
			return
		}
		target = &td
	}
	ctx := r.Context()
	var m *store.Milestone
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		m, err = store.CreateMilestone(ctx, tx, "project", nil, req.Name, "", target, actor(r))
		return err
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, m)
}

func (s *Server) handleMilestoneMember(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Milestone string `json:"milestone"`
		Ref       string `json:"ref"`
		Action    string `json:"action"` // add | remove
		Reason    string `json:"reason"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	m, err := s.milestoneByRef(ctx, req.Milestone)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	memberType, memberID, err := s.resolveMemberRef(ctx, req.Ref)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if req.Action == "remove" {
			return store.RemoveMember(ctx, tx, m.ID, memberType, memberID, req.Reason, actor(r))
		}
		return store.AddMember(ctx, tx, m.ID, memberType, memberID, actor(r))
	})
	if err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 200, map[string]any{"milestone": m.Name, "member": req.Ref, "action": req.Action})
}

// resolveMemberRef resolves a milestone member reference: an initiative, a
// feature, or a nested milestone (SD-2 — no checklists).
func (s *Server) resolveMemberRef(ctx context.Context, ref string) (string, uuid.UUID, error) {
	if f, err := s.featureByPath(ctx, ref); err == nil {
		return "feature", f.ID, nil
	}
	if in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(ref, "/")); err == nil {
		return "initiative", in.ID, nil
	}
	if m, err := s.milestoneByRef(ctx, ref); err == nil {
		return "milestone", m.ID, nil
	}
	return "", uuid.Nil, errors.New("member must be a feature path, initiative path, or milestone")
}

func (s *Server) handleLockMilestone(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Milestone string `json:"milestone"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	m, err := s.milestoneByRef(ctx, req.Milestone)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	var gateReason string
	var passed bool
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		g, lockErr := store.LockMilestone(ctx, tx, m.ID, actor(r))
		gateReason = g.Reason
		passed = g.Pass
		return lockErr
	})
	if err != nil {
		// G4 refusal is a 409 carrying the reason (no force path, FR-5.3).
		writeJSON(w, 409, map[string]any{"locked": false, "gate": "G4", "reason": gateReason})
		return
	}
	writeJSON(w, 200, map[string]any{"locked": true, "reason": gateReason, "passed": passed})
}

func (s *Server) handleListMilestones(w http.ResponseWriter, r *http.Request) {
	ms, err := store.ListMilestones(r.Context(), s.Store.Pool)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, ms)
}

func (s *Server) handleGetMilestone(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, err := s.milestoneByRef(ctx, r.URL.Query().Get("ref"))
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	var p store.Progress
	if m.State == "locked" {
		p, err = store.SnapshotProgress(ctx, s.Store.Pool, m.ID)
	} else {
		p, err = store.LiveProgress(ctx, s.Store.Pool, m.ID)
	}
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	cost, _ := store.MilestoneCost(ctx, s.Store.Pool, p.Leaves)
	writeJSON(w, 200, map[string]any{
		"milestone": m, "progress": map[string]any{"total": p.Total, "done": p.Done},
		"cost_usd": cost,
	})
}

func (s *Server) milestoneByRef(ctx context.Context, ref string) (*store.Milestone, error) {
	if ref == "" {
		return nil, errors.New("missing milestone reference")
	}
	if id, err := uuid.Parse(ref); err == nil {
		return store.GetMilestone(ctx, s.Store.Pool, id)
	}
	// Its ID, "MS-004" (SPEC-015 FR-7.5); a milestone named like one is still
	// found by its name.
	if r, ok := ident.Parse(ref); ok && r.Shape == ident.ShapeEntity && r.Kind.Name == "milestone" && ref == r.ID {
		if m, err := store.MilestoneByPublicID(ctx, s.Store.Pool, r.ID); err != store.ErrNotFound {
			return m, err
		}
	}
	return store.MilestoneByName(ctx, s.Store.Pool, ref)
}

// --- Roadmaps ---

func (s *Server) handleCreateRoadmap(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Name string `json:"name"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var rm *store.Roadmap
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		rm, err = store.CreateRoadmap(ctx, tx, "project", nil, req.Name, actor(r))
		return err
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, rm)
}

func (s *Server) handleRoadmapEntry(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Roadmap   string `json:"roadmap"`
		Milestone string `json:"milestone"`
		Position  int    `json:"position"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rm, err := s.roadmapByRef(ctx, req.Roadmap)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	m, err := s.milestoneByRef(ctx, req.Milestone)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.SetRoadmapEntry(ctx, tx, rm.ID, m.ID, req.Position, actor(r))
	})
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"roadmap": rm.Name, "milestone": m.Name, "position": req.Position})
}

func (s *Server) handleGetRoadmap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rm, err := s.roadmapByRef(ctx, r.URL.Query().Get("ref"))
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	entries, err := store.RoadmapEntries(ctx, s.Store.Pool, rm.ID)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	type entry struct {
		Position  int    `json:"position"`
		Milestone string `json:"milestone"`
		State     string `json:"state"`
	}
	var out []entry
	for _, e := range entries {
		m, err := store.GetMilestone(ctx, s.Store.Pool, e.MilestoneID)
		if err != nil {
			continue
		}
		out = append(out, entry{Position: e.Position, Milestone: m.Name, State: string(m.State)})
	}
	writeJSON(w, 200, map[string]any{"roadmap": rm.Name, "entries": out})
}

func (s *Server) roadmapByRef(ctx context.Context, ref string) (*store.Roadmap, error) {
	if ref == "" {
		return nil, errors.New("missing roadmap reference")
	}
	if id, err := uuid.Parse(ref); err == nil {
		return store.GetRoadmap(ctx, s.Store.Pool, id)
	}
	if r, ok := ident.Parse(ref); ok && r.Shape == ident.ShapeEntity && r.Kind.Name == "roadmap" && ref == r.ID {
		if rm, err := store.RoadmapByPublicID(ctx, s.Store.Pool, r.ID); err != store.ErrNotFound {
			return rm, err
		}
	}
	return store.RoadmapByName(ctx, s.Store.Pool, ref)
}

// --- Extended cost ---

func (s *Server) handleCostRollup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		writeErr(w, 400, errors.New("missing ref"))
		return
	}
	// A milestone or roadmap ref is looked up by name/id; otherwise it is an
	// entity path (initiative/feature).
	if m, err := s.milestoneByRef(ctx, ref); err == nil {
		var leaves []uuid.UUID
		if m.State == "locked" {
			p, _ := store.SnapshotProgress(ctx, s.Store.Pool, m.ID)
			leaves = p.Leaves
		} else {
			leaves, _ = store.ResolveMembers(ctx, s.Store.Pool, m.ID)
		}
		cost, err := store.MilestoneCost(ctx, s.Store.Pool, leaves)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ref": ref, "kind": "milestone", "cost_usd": cost})
		return
	}
	if rm, err := s.roadmapByRef(ctx, ref); err == nil {
		cost, err := store.RoadmapCost(ctx, s.Store.Pool, rm.ID)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ref": ref, "kind": "roadmap", "cost_usd": cost})
		return
	}
	refType, refID, err := s.resolveRef(ctx, ref)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	var cost float64
	switch refType {
	case "initiative":
		cost, err = store.InitiativeCost(ctx, s.Store.Pool, refID)
	case "feature":
		cost, err = store.FeatureCost(ctx, s.Store.Pool, refID)
	default:
		writeErr(w, 400, errors.New("cost rollup supports initiative, feature, milestone, or roadmap"))
		return
	}
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ref": ref, "kind": refType, "cost_usd": cost})
}

func (s *Server) handleCostMonths(w http.ResponseWriter, r *http.Request) {
	months, err := store.CostByMonth(r.Context(), s.Store.Pool)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, months)
}
