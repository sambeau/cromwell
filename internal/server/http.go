package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/bus"
	"cromwell/internal/config"
	"cromwell/internal/lifecycle"
	"cromwell/internal/store"
)

// The HTTP API is the single mutation surface for humans and the CLI (O-1).
// Every handler resolves the actor from the X-Cromwell-Actor header.

func actor(r *http.Request) string {
	if a := r.Header.Get("X-Cromwell-Actor"); a != "" {
		return a
	}
	return "unknown"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return v, false
	}
	return v, true
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/initiatives", s.handleCreateInitiative)
	mux.HandleFunc("POST /api/initiatives/archive", s.handleArchiveInitiative)
	mux.HandleFunc("POST /api/features", s.handleCreateFeature)
	mux.HandleFunc("POST /api/features/abandon", s.handleAbandonFeature)
	mux.HandleFunc("GET /api/features", s.handleGetFeature)
	mux.HandleFunc("POST /api/docs", s.handleRegisterDoc)
	mux.HandleFunc("POST /api/docs/validate", s.handleValidate)
	mux.HandleFunc("POST /api/docs/submit", s.handleSubmit)
	mux.HandleFunc("POST /api/docs/revise", s.handleRevise)
	mux.HandleFunc("GET /api/docs/comments", s.handleComments)
	mux.HandleFunc("GET /api/inbox", s.handleInbox)
	mux.HandleFunc("POST /api/respond", s.handleRespond)
	mux.HandleFunc("GET /api/log", s.handleLog)
	mux.HandleFunc("GET /api/cost", s.handleCost)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("POST /api/hook/post-commit", s.handlePostCommit)
	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queued, _ := s.Store.QueuedDispatches(ctx)
	pending, _ := s.Store.PendingCheckpoints(ctx)
	var running int
	_ = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE state = 'running'`).Scan(&running)
	version, _ := store.SchemaVersion(ctx, s.Store.Pool)

	// Config health: re-run the full compartment load and report
	// (DESIGN-004 §9's check loop).
	configHealth := "ok"
	if _, err := config.Load(s.CompartmentRoot, lifecycle.RuleKinds()); err != nil {
		configHealth = err.Error()
	}
	type queuedInfo struct {
		ID      string `json:"id"`
		Purpose string `json:"purpose"`
		Reason  string `json:"reason,omitempty"`
	}
	var queueInfo []queuedInfo
	for _, d := range queued {
		qi := queuedInfo{ID: d.ID.String(), Purpose: d.Purpose}
		if d.QueueReason != nil {
			qi.Reason = *d.QueueReason
		}
		queueInfo = append(queueInfo, qi)
	}
	writeJSON(w, 200, map[string]any{
		"schema_version":      version,
		"config":              configHealth,
		"queued":              queueInfo,
		"running":             running,
		"pending_checkpoints": len(pending),
	})
}

func (s *Server) handleCreateInitiative(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		ParentPath string `json:"parent_path"`
		Slug       string `json:"slug"`
		Name       string `json:"name"`
		Description string `json:"description"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var parentID *uuid.UUID
	if req.ParentPath != "" {
		parent, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(req.ParentPath, "/"))
		if err != nil {
			writeErr(w, 404, err)
			return
		}
		parentID = &parent.ID
	}
	var in *store.Initiative
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		in, err = store.CreateInitiative(ctx, tx, parentID, req.Slug, req.Name, req.Description, actor(r))
		return err
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, in)
}

// handleArchiveInitiative honours gate G5 with the checkpoint-override path
// (FR-3.1): refusal raises a gate-override checkpoint instead of archiving.
func (s *Server) handleArchiveInitiative(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(req.Path, "/"))
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	n, err := store.NonTerminalFeatureCount(ctx, s.Store.Pool, in.ID)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	g := lifecycle.G5(n)
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.Audit(ctx, tx, actor(r), "gate.evaluated", "initiative", &in.ID,
			map[string]any{"gate": string(g.Gate), "pass": g.Pass, "reason": g.Reason}); err != nil {
			return err
		}
		if g.Pass {
			return store.ArchiveInitiative(ctx, tx, in.ID, actor(r), req.Reason)
		}
		_, err := store.CreateCheckpoint(ctx, tx, "gate-override", "initiative", in.ID,
			"Archive blocked by G5: "+g.Reason+". Override?",
			map[string]any{"gate": "G5", "reason": g.Reason, "requested_by": actor(r)})
		return err
	})
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if g.Pass {
		writeJSON(w, 200, map[string]any{"archived": true})
		return
	}
	writeJSON(w, 409, map[string]any{"archived": false, "blocked_by": "G5", "reason": g.Reason,
		"note": "a gate-override checkpoint is in the inbox"})
}

func (s *Server) handleCreateFeature(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		InitiativePath string `json:"initiative_path"`
		Slug           string `json:"slug"`
		Name           string `json:"name"`
		Description    string `json:"description"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(req.InitiativePath, "/"))
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	var f *store.Feature
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		f, err = store.CreateFeature(ctx, tx, in.ID, req.Slug, req.Name, req.Description, actor(r))
		return err
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, f)
}

func (s *Server) handleGetFeature(w http.ResponseWriter, r *http.Request) {
	f, err := s.featureByPath(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, f)
}

func (s *Server) handleAbandonFeature(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		writeErr(w, 400, errors.New("abandoning requires a reason (DESIGN-003 §6)"))
		return
	}
	ctx := r.Context()
	f, err := s.featureByPath(ctx, req.Path)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.TransitionFeature(ctx, tx, f, lifecycle.FeatAbandon, actor(r),
			map[string]any{"reason": req.Reason})
	})
	if err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 200, f)
}

func (s *Server) handleRegisterDoc(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Path      string `json:"path"`
		Type      string `json:"type"`
		OwnerType string `json:"owner_type"`
		OwnerRef  string `json:"owner_ref"`
	}](w, r)
	if !ok {
		return
	}
	doc, err := s.RegisterDoc(r.Context(), req.Path, req.Type, req.OwnerType, req.OwnerRef, actor(r))
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, doc)
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Path string `json:"path"`
	}](w, r)
	if !ok {
		return
	}
	report, err := s.ValidateDoc(r.Context(), req.Path)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, report)
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Path string `json:"path"`
	}](w, r)
	if !ok {
		return
	}
	report, doc, err := s.SubmitDoc(r.Context(), req.Path, actor(r))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, err)
		} else {
			writeErr(w, 409, err)
		}
		return
	}
	status := 200
	if !report.Valid {
		status = 422
	}
	writeJSON(w, status, map[string]any{"report": report, "state": doc.State})
}

func (s *Server) handleRevise(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		Path string `json:"path"`
	}](w, r)
	if !ok {
		return
	}
	successor, err := s.ReviseDoc(r.Context(), req.Path, actor(r))
	if err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 201, successor)
}

func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	comments, err := store.CommentsForDocument(ctx, s.Store.Pool, doc.ID, false)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, comments)
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	pending, err := s.Store.PendingCheckpoints(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, pending)
}

// handleRespond records the answer and feeds CheckpointResponded back into
// the rule engine (FR-6.1).
func (s *Server) handleRespond(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[struct {
		ID       string         `json:"id"`
		Response map[string]any `json:"response"`
	}](w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	ctx := r.Context()
	var cp *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		cp, err = store.RespondCheckpoint(ctx, tx, id, req.Response, actor(r))
		return err
	})
	if err != nil {
		writeErr(w, 409, err)
		return
	}
	s.Bus.Publish(bus.CheckpointResponded{
		CheckpointID: cp.ID, Kind: cp.Kind, RefType: cp.RefType, RefID: cp.RefID,
		Context: cp.Context, Response: cp.Response, RespondedBy: actor(r),
	})
	writeJSON(w, 200, cp)
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	var refID *uuid.UUID
	if rid := r.URL.Query().Get("ref_id"); rid != "" {
		id, err := uuid.Parse(rid)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		refID = &id
	}
	events, err := s.Store.AuditTail(r.Context(), r.URL.Query().Get("ref_type"), refID, limit)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, events)
}

func (s *Server) handleCost(w http.ResponseWriter, r *http.Request) {
	rollup, err := s.Store.CostRollup(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	var total float64
	for _, row := range rollup {
		total += row.CostUSD
	}
	writeJSON(w, 200, map[string]any{"total_usd": total, "by_entity": rollup})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeErr(w, 400, errors.New("missing q"))
		return
	}
	hits, err := s.Store.SearchSections(r.Context(), q, 20)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, hits)
}

func (s *Server) handlePostCommit(w http.ResponseWriter, r *http.Request) {
	if err := s.OnPostCommit(r.Context()); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
