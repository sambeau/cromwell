package server

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/bus"
	"cromwell/internal/store"
)

// The web command centre (DESIGN-007, SPEC-004): a second rendering of the same
// service layer as /api/*, server-rendered HTML + HTMX + SSE embedded in the
// one binary (CC-1, CC-2). Every read calls the same store/service functions
// the JSON handlers do; the one mutation (respond) posts to the same
// CheckpointResponded path the CLI uses (CC-4). No handler here issues HTTP to
// /api/* (NFR-2).

//go:embed ui/templates/*.html
var uiTemplatesFS embed.FS

//go:embed ui/static/*
var uiStaticFS embed.FS

// uiTemplates holds the parsed template set. All templates share one set with
// unique {{define}} names, so pages and live-region fragments are rendered by
// name off the same tree.
type uiTemplates struct {
	t *template.Template
}

var uiFuncs = template.FuncMap{
	"usd":     func(v float64) string { return fmt.Sprintf("$%.4f", v) },
	"usd2":    func(v float64) string { return fmt.Sprintf("$%.2f", v) },
	"tokens":  humanTokens,
	"shortID": func(id uuid.UUID) string { return id.String()[:8] },
	"ago":     ago,
	"signed": func(v int64) string {
		if v >= 0 {
			return fmt.Sprintf("+%d", v)
		}
		return fmt.Sprintf("%d", v)
	},
	"title": strings.Title, //nolint:staticcheck // ASCII slugs only; adequate for labels
	"pct": func(done, total int) int {
		if total == 0 {
			return 0
		}
		return done * 100 / total
	},
	"clampPct":    clampPct,
	"deltaTokens": func(actual, estimate int64) int64 { return actual - estimate },
}

// clampPct returns cost as a whole-percent of cap, clamped to [0,100] for a
// progress bar width.
func clampPct(cost, cap float64) int {
	if cap <= 0 {
		return 0
	}
	p := int(cost / cap * 100)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func loadUITemplates() (*uiTemplates, error) {
	t, err := template.New("ui").Funcs(uiFuncs).ParseFS(uiTemplatesFS, "ui/templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing web UI templates: %w", err)
	}
	return &uiTemplates{t: t}, nil
}

// render executes a named template to the response, mapping a template error to
// a 500 (after a partial write there is little else to do, but the error is
// logged for the operator).
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.ui.t.ExecuteTemplate(w, name, data); err != nil {
		s.Log.Error("ui render", "template", name, "err", err)
	}
}

// pageData is the envelope every full page renders with: which nav item is
// active, the operator identity, and the view's own data.
type pageData struct {
	Active string
	Actor  string
	Data   any
}

func (s *Server) page(active string, data any) pageData {
	return pageData{Active: active, Actor: s.uiActor(), Data: data}
}

// uiActor is the single seam for the browser session's identity (SD-4, CC-6).
func (s *Server) uiActor() string {
	if cfg, err := s.freshConfig(); err == nil && cfg.Server.UIActor != "" {
		return cfg.Server.UIActor
	}
	return "operator"
}

// uiRoutes registers the web command centre surface. It is mounted on the same
// mux as /api/*; in practice it is reachable only over the TCP listener, since
// the CLI (the socket's only client) never requests /ui/* (FR-1.1).
func (s *Server) uiRoutes(mux *http.ServeMux) {
	static, _ := fs.Sub(uiStaticFS, "ui/static")
	mux.Handle("GET /ui/static/", http.StripPrefix("/ui/static/", http.FileServer(http.FS(static))))

	mux.HandleFunc("GET /ui", s.handleUIDashboard)
	mux.HandleFunc("GET /ui/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui", http.StatusFound)
	})
	mux.HandleFunc("GET /ui/inbox", s.handleUIInbox)
	mux.HandleFunc("GET /ui/planning", s.handleUIPlanning)
	mux.HandleFunc("GET /ui/documents", s.handleUIDocuments)
	mux.HandleFunc("GET /ui/document", s.handleUIDocument)
	mux.HandleFunc("GET /ui/cost", s.handleUICost)

	// Live-region fragments (re-read through the normal service path on an SSE
	// signal, SD-5).
	mux.HandleFunc("GET /ui/frag/queue", s.handleFragQueue)
	mux.HandleFunc("GET /ui/frag/events", s.handleFragEvents)
	mux.HandleFunc("GET /ui/frag/cost", s.handleFragCost)
	mux.HandleFunc("GET /ui/frag/calibration", s.handleFragCalibration)
	mux.HandleFunc("GET /ui/frag/inbox", s.handleFragInbox)
	mux.HandleFunc("GET /ui/frag/inbox-badge", s.handleFragInboxBadge)

	// Realtime stream and the one mutation.
	mux.HandleFunc("GET /ui/events", s.handleUIEvents)
	mux.HandleFunc("POST /ui/respond", s.handleUIRespond)
}

// --- Dashboard ---

func (s *Server) handleUIDashboard(w http.ResponseWriter, r *http.Request) {
	sum, err := s.DashboardSummary(r.Context(), 20)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-dashboard", s.page("dashboard", sum))
}

func (s *Server) handleFragQueue(w http.ResponseWriter, r *http.Request) {
	sum, err := s.DashboardSummary(r.Context(), 20)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-queue", sum)
}

func (s *Server) handleFragEvents(w http.ResponseWriter, r *http.Request) {
	sum, err := s.DashboardSummary(r.Context(), 20)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-events", sum)
}

func (s *Server) handleFragCost(w http.ResponseWriter, r *http.Request) {
	sum, err := s.DashboardSummary(r.Context(), 20)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-cost", sum)
}

func (s *Server) handleFragCalibration(w http.ResponseWriter, r *http.Request) {
	sum, err := s.DashboardSummary(r.Context(), 20)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-calibration", sum)
}

// --- Inbox ---

// inboxItem is a pending checkpoint prepared for rendering: the stored
// checkpoint plus its decoded context and the answer options the rule engine
// accepts for its kind.
type inboxItem struct {
	store.Checkpoint
	Options []string
}

func (s *Server) inboxItems(r *http.Request) ([]inboxItem, error) {
	pending, err := s.Store.PendingCheckpoints(r.Context())
	if err != nil {
		return nil, err
	}
	items := make([]inboxItem, 0, len(pending))
	for _, cp := range pending {
		items = append(items, inboxItem{Checkpoint: cp, Options: answerOptions(cp.Kind)})
	}
	return items, nil
}

func (s *Server) handleUIInbox(w http.ResponseWriter, r *http.Request) {
	items, err := s.inboxItems(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-inbox", s.page("inbox", items))
}

func (s *Server) handleFragInbox(w http.ResponseWriter, r *http.Request) {
	items, err := s.inboxItems(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-inbox", items)
}

func (s *Server) handleFragInboxBadge(w http.ResponseWriter, r *http.Request) {
	items, err := s.inboxItems(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-inbox-badge", len(items))
}

// handleUIRespond records a checkpoint answer through the same
// RespondCheckpoint + CheckpointResponded path the CLI uses (FR-3.2, CC-4). The
// answer and optional reason come from the posted form; the actor is the single
// configured operator (SD-4). On success it returns the refreshed inbox list so
// the answered item clears without a reload.
func (s *Server) handleUIRespond(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	id, err := uuid.Parse(r.FormValue("id"))
	if err != nil {
		http.Error(w, "bad checkpoint id", http.StatusBadRequest)
		return
	}
	verb := strings.TrimSpace(r.FormValue("verb"))
	if verb == "" {
		http.Error(w, "an answer is required", http.StatusBadRequest)
		return
	}
	// The kind determines the payload schema; look it up rather than trust the
	// form (the form's kind is presentational only).
	ctx := r.Context()
	cpBefore, err := store.GetCheckpoint(ctx, s.Store.Pool, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	response := responseFor(cpBefore.Kind, verb, strings.TrimSpace(r.FormValue("reason")))

	actor := s.uiActor()
	var cp *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		cp, err = store.RespondCheckpoint(ctx, tx, id, response, actor)
		return err
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	// Feed the same event the CLI path publishes so the orchestrator resumes.
	s.Bus.Publish(busCheckpointResponded(cp, actor))

	items, err := s.inboxItems(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-inbox", items)
}

// --- Planning ---

func (s *Server) handleUIPlanning(w http.ResponseWriter, r *http.Request) {
	view, err := s.planningView(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-planning", s.page("planning", view))
}

// --- Documents ---

func (s *Server) handleUIDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := store.ListLiveDocuments(r.Context(), s.Store.Pool)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-documents", s.page("documents", docs))
}

func (s *Server) handleUIDocument(w http.ResponseWriter, r *http.Request) {
	view, err := s.documentView(r)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.uiError(w, err)
		return
	}
	s.render(w, "page-document", s.page("documents", view))
}

// --- Cost ---

func (s *Server) handleUICost(w http.ResponseWriter, r *http.Request) {
	view, err := s.costView(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-cost", s.page("cost", view))
}

// --- Realtime ---

// handleUIEvents is the SSE stream (FR-7.1). It subscribes to the hub and
// forwards each event to the browser as a generic `changed` event carrying the
// event kind. Live regions bind to `sse:changed` and re-read their fragment
// through the normal service path — the message carries no authority (SD-5). A
// slow browser is dropped by the hub, never back-pressuring the orchestrator.
func (s *Server) handleUIEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsub := s.Hub.Subscribe(r.Context())
	defer unsub()

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			// The event name is generic so every live region can bind to one
			// trigger; the kind rides along as data for future granularity.
			fmt.Fprintf(w, "event: changed\ndata: %s\n\n", ev.EventKind())
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// uiError renders a minimal error page; the detail is logged, not leaked in a
// way that could carry secrets (NFR-5).
func (s *Server) uiError(w http.ResponseWriter, err error) {
	s.Log.Error("ui handler", "err", err)
	w.WriteHeader(http.StatusInternalServerError)
	s.render(w, "page-error", s.page("", err.Error()))
}

// notifyCheckpointRaised fans a presentation-only checkpoint-raised signal into
// the SSE hub so the live inbox lights the moment a checkpoint is created
// (FR-8.2, DESIGN-007 §6). Checkpoint creation emits no bus event — the rule
// engine acts on responses, not raises — so this is broadcast straight to the
// hub, bypassing the orchestrator bus. It is called after the creating
// transaction commits, and carries no authority (SD-5).
func (s *Server) notifyCheckpointRaised(cp *store.Checkpoint) {
	if cp == nil || s.Hub == nil {
		return
	}
	s.Hub.Broadcast(bus.CheckpointRaised{
		CheckpointID: cp.ID, Kind: cp.Kind, RefType: cp.RefType, RefID: cp.RefID,
	})
}

// humanTokens renders a token count compactly (1_234 → "1.2k").
func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ago renders a compact relative age.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
