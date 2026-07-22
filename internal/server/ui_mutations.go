package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

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
