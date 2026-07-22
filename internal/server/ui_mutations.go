package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/lifecycle"
	"cromwell/internal/sizing"
	"cromwell/internal/store"
)

// The command-centre mutation slice (SPEC-006): planning and review actions the
// browser drives. Every handler here posts to a /ui/* route and calls the SAME
// gated, audited service method the /api/* handler and the CLI call — never
// self-HTTP (CC-2, NFR-2), never a new authority (CC-4, SD-2). The acting actor
// is the single configured operator (SD-3). On success a handler returns the
// refreshed fragment so the region updates without a reload; on a service-layer
// rejection it returns the same fragment with an inline error, never a raw 500
// (FR-6.2).

// renderPlanningBody re-reads the planning view and renders the planning-body
// fragment with an optional notice or error banner. It is the shared response
// for every planning mutation, so the tree, milestones, and roadmaps all
// reflect the change (FR-6.1).
func (s *Server) renderPlanningBody(w http.ResponseWriter, r *http.Request, notice, errMsg string) {
	view, err := s.planningView(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-planning", planningPage{View: view, Notice: notice, Error: errMsg})
}

// --- Estimates (FR-1) ---

// handleUIEstimateSet records a unit estimate on a feature or task (FR-1.1),
// through the same RecordEstimate path as POST /api/estimate/set. The tier
// follows the evidence — rough by default, considered when the operator cites
// the corpus — never a free choice (SPEC-003 FR-1.3).
func (s *Server) handleUIEstimateSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderPlanningBody(w, r, "", "could not read the form")
		return
	}
	ref := strings.TrimSpace(r.FormValue("ref"))
	tokens, terr := strconv.ParseInt(strings.TrimSpace(r.FormValue("tokens")), 10, 64)
	if ref == "" || terr != nil || tokens <= 0 {
		s.renderPlanningBody(w, r, "", "a ref (e.g. auth/login) and a positive token count are required")
		return
	}
	ctx := r.Context()
	refType, refID, err := s.resolveRef(ctx, ref)
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such ref: "+ref)
		return
	}
	if refType != "feature" && refType != "task" {
		s.renderPlanningBody(w, r, "", "estimates attach to features and tasks only, not "+refType)
		return
	}
	tier := sizing.TierRough
	if r.FormValue("cite_corpus") != "" {
		tier = sizing.TierConsidered
	}
	rationale := strings.TrimSpace(r.FormValue("rationale"))
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.RecordEstimate(ctx, tx, refType, refID, tokens, tier, rationale, nil, s.uiActor())
		return e
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, fmt.Sprintf("estimate set: %s = %d tokens (%s)", ref, tokens, tier), "")
}

// handleUIEstimateAI dispatches the AI estimator for a node (FR-1.2), through
// the same enqueueEstimate path as POST /api/estimate/ai. The dispatch is async;
// the estimate appears live via the SSE refresh when it completes — the UI never
// fabricates a tier.
func (s *Server) handleUIEstimateAI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderPlanningBody(w, r, "", "could not read the form")
		return
	}
	ref := strings.TrimSpace(r.FormValue("ref"))
	if ref == "" {
		s.renderPlanningBody(w, r, "", "a ref (e.g. auth/login) is required")
		return
	}
	ctx := r.Context()
	refType, refID, err := s.resolveRef(ctx, ref)
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such ref: "+ref)
		return
	}
	if err := s.enqueueEstimate(ctx, refType, refID); err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, "estimator dispatched for "+ref+" — the estimate will appear when it completes", "")
}

// --- Milestones (FR-2) ---

func (s *Server) handleUIMilestoneCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.renderPlanningBody(w, r, "", "a milestone name is required")
		return
	}
	var target *time.Time
	if td := strings.TrimSpace(r.FormValue("target_date")); td != "" {
		parsed, err := time.Parse("2006-01-02", td)
		if err != nil {
			s.renderPlanningBody(w, r, "", "target date must be YYYY-MM-DD")
			return
		}
		target = &parsed
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.CreateMilestone(ctx, tx, name, "", target, s.uiActor())
		return e
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, "milestone created: "+name, "")
}

// handleUIMilestoneMember adds or descopes a member (FR-2.1). Descope carries a
// reason onto the audit trail, exactly as the CLI's remove does.
func (s *Server) handleUIMilestoneMember(w http.ResponseWriter, r *http.Request) {
	milestone := strings.TrimSpace(r.FormValue("milestone"))
	ref := strings.TrimSpace(r.FormValue("ref"))
	action := strings.TrimSpace(r.FormValue("action"))
	if milestone == "" || ref == "" {
		s.renderPlanningBody(w, r, "", "a milestone and a member ref are required")
		return
	}
	ctx := r.Context()
	m, err := s.milestoneByRef(ctx, milestone)
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such milestone: "+milestone)
		return
	}
	memberType, memberID, err := s.resolveMemberRef(ctx, ref)
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if action == "remove" {
			return store.RemoveMember(ctx, tx, m.ID, memberType, memberID, strings.TrimSpace(r.FormValue("reason")), s.uiActor())
		}
		return store.AddMember(ctx, tx, m.ID, memberType, memberID, s.uiActor())
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	verb := "added to"
	if action == "remove" {
		verb = "descoped from"
	}
	s.renderPlanningBody(w, r, fmt.Sprintf("%s %s milestone %s", ref, verb, m.Name), "")
}

// handleUIMilestoneLock locks a milestone (FR-2.2, gate G4). A blocked lock
// surfaces the gate reason inline for the operator to descope — G4 raises no
// checkpoint (the CLI's 409 path); the UI never forces it (L-6).
func (s *Server) handleUIMilestoneLock(w http.ResponseWriter, r *http.Request) {
	milestone := strings.TrimSpace(r.FormValue("milestone"))
	if milestone == "" {
		s.renderPlanningBody(w, r, "", "a milestone is required")
		return
	}
	ctx := r.Context()
	m, err := s.milestoneByRef(ctx, milestone)
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such milestone: "+milestone)
		return
	}
	var gateReason string
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		g, lockErr := store.LockMilestone(ctx, tx, m.ID, s.uiActor())
		gateReason = g.Reason
		return lockErr
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", fmt.Sprintf("cannot lock %s — G4: %s. Descope the unfinished members, then lock.", m.Name, gateReason))
		return
	}
	s.renderPlanningBody(w, r, "milestone locked: "+m.Name+" ("+gateReason+")", "")
}

// --- Roadmaps (FR-3) ---

func (s *Server) handleUIRoadmapCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.renderPlanningBody(w, r, "", "a roadmap name is required")
		return
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.CreateRoadmap(ctx, tx, name, s.uiActor())
		return e
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, "roadmap created: "+name, "")
}

func (s *Server) handleUIRoadmapEntry(w http.ResponseWriter, r *http.Request) {
	roadmap := strings.TrimSpace(r.FormValue("roadmap"))
	milestone := strings.TrimSpace(r.FormValue("milestone"))
	if roadmap == "" || milestone == "" {
		s.renderPlanningBody(w, r, "", "a roadmap and a milestone are required")
		return
	}
	position, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("position")))
	ctx := r.Context()
	rm, err := s.roadmapByRef(ctx, roadmap)
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such roadmap: "+roadmap)
		return
	}
	m, err := s.milestoneByRef(ctx, milestone)
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such milestone: "+milestone)
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.SetRoadmapEntry(ctx, tx, rm.ID, m.ID, position, s.uiActor())
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, fmt.Sprintf("%s placed on roadmap %s at position %d", m.Name, rm.Name, position), "")
}

// --- Tree lifecycle (FR-4) ---

func (s *Server) handleUIInitiativeCreate(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.FormValue("slug"))
	name := strings.TrimSpace(r.FormValue("name"))
	if slug == "" || name == "" {
		s.renderPlanningBody(w, r, "", "a slug and a name are required")
		return
	}
	ctx := r.Context()
	var parentID *uuid.UUID
	if pp := strings.TrimSpace(r.FormValue("parent_path")); pp != "" {
		parent, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(pp, "/"))
		if err != nil {
			s.renderPlanningBody(w, r, "", "no such parent initiative: "+pp)
			return
		}
		parentID = &parent.ID
	}
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.CreateInitiative(ctx, tx, parentID, slug, name, strings.TrimSpace(r.FormValue("description")), s.uiActor())
		return e
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, "initiative created: "+slug, "")
}

func (s *Server) handleUIFeatureCreate(w http.ResponseWriter, r *http.Request) {
	initPath := strings.TrimSpace(r.FormValue("initiative_path"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	name := strings.TrimSpace(r.FormValue("name"))
	if initPath == "" || slug == "" || name == "" {
		s.renderPlanningBody(w, r, "", "an initiative path, a slug, and a name are required")
		return
	}
	ctx := r.Context()
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(initPath, "/"))
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such initiative: "+initPath)
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.CreateFeature(ctx, tx, in.ID, slug, name, strings.TrimSpace(r.FormValue("description")), s.uiActor())
		return e
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, "feature created: "+initPath+"/"+slug, "")
}

// handleUIFeatureStart transitions ready → active and creates the worktree,
// dispatching initially-ready tasks (FR-4.2). The dashboard queue reflects the
// new dispatches live via SSE.
func (s *Server) handleUIFeatureStart(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		s.renderPlanningBody(w, r, "", "a feature path is required")
		return
	}
	if _, err := s.StartFeature(r.Context(), path, s.uiActor()); err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, "feature started: "+path, "")
}

// handleUIFeatureAbandon requires a reason (DESIGN-003 §6), exactly as the CLI.
func (s *Server) handleUIFeatureAbandon(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.FormValue("path"))
	reason := strings.TrimSpace(r.FormValue("reason"))
	if path == "" {
		s.renderPlanningBody(w, r, "", "a feature path is required")
		return
	}
	if reason == "" {
		s.renderPlanningBody(w, r, "", "abandoning a feature requires a reason")
		return
	}
	ctx := r.Context()
	f, err := s.featureByPath(ctx, path)
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such feature: "+path)
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.TransitionFeature(ctx, tx, f, lifecycle.FeatAbandon, s.uiActor(), map[string]any{"reason": reason})
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	s.renderPlanningBody(w, r, "feature abandoned: "+path, "")
}

// handleUIInitiativeArchive archives an initiative (FR-4.3, gate G5). When G5
// blocks (non-terminal features), a gate-override checkpoint is raised to the
// inbox — the UI never archives without the answered checkpoint (L-6), exactly
// as the CLI and the SPEC-004 smoke path.
func (s *Server) handleUIInitiativeArchive(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		s.renderPlanningBody(w, r, "", "an initiative path is required")
		return
	}
	ctx := r.Context()
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(path, "/"))
	if err != nil {
		s.renderPlanningBody(w, r, "", "no such initiative: "+path)
		return
	}
	n, err := store.NonTerminalFeatureCount(ctx, s.Store.Pool, in.ID)
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	g := lifecycle.G5(n)
	reason := strings.TrimSpace(r.FormValue("reason"))
	actor := s.uiActor()
	var raised *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := store.Audit(ctx, tx, actor, "gate.evaluated", "initiative", &in.ID,
			map[string]any{"gate": string(g.Gate), "pass": g.Pass, "reason": g.Reason}); e != nil {
			return e
		}
		if g.Pass {
			return store.ArchiveInitiative(ctx, tx, in.ID, actor, reason)
		}
		var e error
		raised, e = store.CreateCheckpoint(ctx, tx, "gate-override", "initiative", in.ID,
			"Archive blocked by G5: "+g.Reason+". Override?",
			map[string]any{"gate": "G5", "reason": g.Reason, "requested_by": actor})
		return e
	})
	if err != nil {
		s.renderPlanningBody(w, r, "", err.Error())
		return
	}
	if g.Pass {
		s.renderPlanningBody(w, r, "initiative archived: "+path, "")
		return
	}
	s.notifyCheckpointRaised(raised)
	s.renderPlanningBody(w, r, "", "archive blocked by G5: "+g.Reason+" — a gate-override checkpoint is now in the inbox to answer.")
}

// --- Document review (FR-5) ---

// renderDocumentBody re-reads a document view and renders the swappable
// document-body fragment with an optional notice/error.
func (s *Server) renderDocumentBody(w http.ResponseWriter, r *http.Request, path, notice, errMsg string) {
	page, err := s.documentViewByPath(r.Context(), path, notice, errMsg)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-document", page)
}

// handleUIDocumentReview approves or requests changes on a human-gated review
// from the document view (FR-5.1). It is a second surface to the inbox respond
// path, not a new action (SD-4): it builds the same EscalationResponse and posts
// to the same RespondCheckpoint + CheckpointResponded path. request-changes
// carries a single reason (the §6 floor).
func (s *Server) handleUIDocumentReview(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.FormValue("path"))
	decision := strings.TrimSpace(r.FormValue("decision")) // approve | request_changes
	if path == "" || (decision != "approve" && decision != "request_changes") {
		s.renderDocumentBody(w, r, path, "", "a document and a decision (approve or request_changes) are required")
		return
	}
	ctx := r.Context()
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err != nil {
		s.renderDocumentBody(w, r, path, "", "no such document: "+path)
		return
	}
	cp, err := s.openReviewCheckpoint(ctx, doc.ID)
	if err != nil || cp == nil {
		s.renderDocumentBody(w, r, path, "", "this document has no open review to act on")
		return
	}
	response := map[string]any{"decision": decision}
	if reason := strings.TrimSpace(r.FormValue("reason")); reason != "" {
		response["reason"] = reason
	}
	actor := s.uiActor()
	var answered *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		answered, err = store.RespondCheckpoint(ctx, tx, cp.ID, response, actor)
		return err
	})
	if err != nil {
		s.renderDocumentBody(w, r, path, "", err.Error())
		return
	}
	// The same event the inbox respond and the CLI publish, so the orchestrator
	// resumes identically (SD-4).
	s.Bus.Publish(busCheckpointResponded(answered, actor))

	notice := "review approved — the document will advance"
	if decision == "request_changes" {
		notice = "changes requested — returned to the author"
	}
	s.renderDocumentBody(w, r, path, notice, "")
}
