package server

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/store"
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

// uiFuncs are the template helpers. There is deliberately no currency
// formatter: money is off the human surface (DESIGN-008 D-4), and the absence
// of the helper is what keeps it off.
var uiFuncs = template.FuncMap{
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
	"deltaTokens": func(actual, estimate int64) int64 { return actual - estimate },
	// slice2 builds the two-crumb trail the plain read-only pages use: the
	// owning entity, then the page itself (unlinked).
	"slice2": func(owner crumb, here string) []crumb {
		return []crumb{owner, {Label: here, Here: true}}
	},

	// --- Design-system helpers -------------------------------------------
	// State and tier are rendered as hue AND icon AND word, never colour
	// alone, so these turn a stored value into its human label and its icon
	// name. Labels are full words in plain language (D-6).
	"stateLabel":  stateLabel,
	"stateIcon":   stateIcon,
	"tierLabel":   tierLabel,
	"tierExplain": tierExplain,
	// The confidence tier renders as a three-segment ring (design round 2 §2):
	// how many segments are filled IS the meaning, because the tiers are an
	// ordinal count of the evidence behind a number.
	"tierSegments": tierSegments,
	"tierTitle":    tierTitle,
	"docIcon":      docIcon,
	// Audit kinds are machine identifiers; an activity feed is for people.
	"eventLabel": eventLabel,
	// The inbox spells out what each answer will do, because these decisions are
	// expensive to reverse and a sentence of prose is cheap.
	"verbLabel":       verbLabel,
	"verbIcon":        verbIcon,
	"verbConsequence": verbConsequence,
	"add":             func(a, b int) int { return a + b },
	"inc":             func(i int) int { return i + 1 },
	"dec":             func(i int) int { return i - 1 },
	// nonPrimary drops the document that is already rendered as the page body,
	// so the Documents section lists the *other* documents rather than repeating
	// the one you are looking at (design round 4 §2).
	"nonPrimary": func(docs []docCard) []docCard {
		out := make([]docCard, 0, len(docs))
		for _, d := range docs {
			if !d.IsPrimary {
				out = append(out, d)
			}
		}
		return out
	},
	// article picks "a" or "an" so generated sentences read correctly
	// ("about an initiative", not "about a initiative").
	"article": func(v any) string {
		s := str(v)
		if s == "" {
			return "a"
		}
		switch s[0] {
		case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
			return "an"
		}
		return "a"
	},
	// spentPct is the share of an estimate already spent, clamped so an overrun
	// fills the bar rather than overflowing it.
	"spentPct": func(done, estimate int64) int {
		if estimate <= 0 {
			return 0
		}
		p := int(done * 100 / estimate)
		if p > 100 {
			return 100
		}
		if p < 0 {
			return 0
		}
		return p
	},
	// pips renders the countable done/not-done marks beside a milestone's
	// count, so "3 of 4" is legible without reading the number. It returns
	// one bool per item, capped so a huge milestone does not flood the row.
	"pips": func(done, total int) []bool {
		if total <= 0 {
			return nil
		}
		if total > 24 { // beyond this the number carries it; pips would be noise
			return nil
		}
		out := make([]bool, total)
		for i := 0; i < done && i < total; i++ {
			out[i] = true
		}
		return out
	},
}

// str normalises the several distinct string-kinded state types the store uses
// (lifecycle.FeatureState, lifecycle.DocumentState, sizing.Tier, …) into a plain
// string, so one template helper serves them all.
func str(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// stateLabel is the human-facing name of a lifecycle state (D-6): a phrase a
// reader understands, not the stored token.
func stateLabel(v any) string {
	state := str(v)
	switch state {
	case "idea":
		return "Idea"
	case "ready":
		return "Ready to start"
	case "active":
		return "In progress"
	case "review":
		return "In review"
	case "done":
		return "Done"
	case "abandoned":
		return "Abandoned"
	case "draft":
		return "Draft"
	case "reviewing":
		return "In review"
	case "approved":
		return "Approved"
	case "superseded":
		return "Superseded"
	case "open":
		return "Open"
	case "locked":
		// A locked milestone is one marked as shipped (SPEC-010 FR-5); the
		// stored state keeps its old name, people see what it means.
		return "Shipped"
	case "queued":
		return "Queued"
	case "running":
		return "Running"
	case "succeeded":
		return "Succeeded"
	case "failed":
		return "Failed"
	case "cancelled":
		return "Cancelled"
	case "":
		return ""
	default:
		return strings.ToUpper(state[:1]) + state[1:]
	}
}

// stateIcon maps a state to its lifecycle mark in the icon sprite. The marks are
// geometric siblings, so the progression reads as a family; states without a
// mark of their own borrow the nearest one.
func stateIcon(v any) string {
	state := str(v)
	switch state {
	case "idea", "ready", "active", "review", "done", "abandoned", "draft", "superseded":
		return state
	case "reviewing":
		return "review"
	case "approved":
		return "done"
	case "open":
		return "ready"
	case "locked":
		return "done"
	case "running", "queued":
		return "active"
	case "succeeded":
		return "done"
	case "failed", "cancelled":
		return "abandoned"
	default:
		return "ready"
	}
}

// tierLabel names a confidence tier in words, and says plainly when there is no
// estimate at all rather than showing a fourth colour.
func tierLabel(v any) string {
	switch str(v) {
	case "decomposed":
		return "Decomposed"
	case "considered":
		return "Considered"
	case "rough":
		return "Rough"
	default:
		return "Not estimated yet"
	}
}

// tierExplain says in one sentence what a confidence tier actually means, so a
// reader never has to know the vocabulary to trust the number (D-6).
func tierExplain(v any) string {
	switch str(v) {
	case "decomposed":
		return "Decomposed — every part of this has its own estimate, so the total is a sum of real sizings rather than one guess."
	case "considered":
		return "Considered — sized against work that has already finished, so there is evidence behind it."
	case "rough":
		return "Rough — sized from a name and a sentence, with nothing to check it against. Treat it as a placeholder."
	default:
		return "Nothing here has been sized yet."
	}
}

// tierSegments is how many of the ring's three segments a tier fills, which
// selects the sprite symbol (i-tier-0 … i-tier-3). The count is the meaning:
// each segment is one more piece of evidence behind the number.
func tierSegments(v any) int {
	switch str(v) {
	case "decomposed":
		return 3
	case "considered":
		return 2
	case "rough":
		return 1
	default:
		return 0
	}
}

// tierTitle is the ring's accessible name and tooltip: the tier, the count, and
// what that count actually rests on — so the mark never has to be decoded.
func tierTitle(v any) string {
	switch str(v) {
	case "decomposed":
		return "Decomposed estimate — 3 of 3: every child carries its own estimate"
	case "considered":
		return "Considered estimate — 2 of 3: sized against a written design"
	case "rough":
		return "Rough estimate — 1 of 3: sized from a name and a description"
	default:
		return "Not estimated — 0 of 3: nothing has been sized here yet"
	}
}

// eventLabel turns an audit event kind into a short phrase a person can read.
// The stored kinds are dotted identifiers meant for machines ("gate.evaluated");
// an activity feed is for humans, so it says what happened (D-6). Anything
// unmapped falls back to the identifier with its punctuation softened, so a new
// event kind degrades to something legible rather than to nothing.
func eventLabel(v any) string {
	switch str(v) {
	case "initiative.created":
		return "created this initiative"
	case "initiative.changed":
		return "edited this initiative"
	case "initiative.archived":
		return "archived this initiative"
	case "feature.created":
		return "created this feature"
	case "feature.updated":
		return "edited this feature"
	case "feature.transition":
		return "moved this feature on"
	case "feature.merged":
		return "merged this feature"
	case "feature.spec_stale":
		return "flagged the specification as out of date"
	case "task.created":
		return "added a task"
	case "task.deleted":
		return "removed a task"
	case "task.transition":
		return "moved a task on"
	case "task.review_comments":
		return "left review comments"
	case "devplan.decomposed":
		return "broke the development plan into tasks"
	case "document.registered":
		return "attached a document"
	case "document.indexed":
		return "indexed a document"
	case "document.validated":
		return "validated a document"
	case "document.transition":
		return "moved a document on"
	case "document.revision_created":
		return "started a new revision"
	case "document.marked_primary":
		return "made a document the page body"
	case "estimate.recorded":
		return "recorded an estimate"
	case "gate.evaluated":
		return "checked whether this could proceed"
	case "checkpoint.created", "checkpoint.raised":
		return "raised a question for a person"
	case "checkpoint.responded":
		return "answered a question"
	case "dispatch.queued":
		return "queued an agent run"
	case "dispatch.requeued":
		return "queued an agent run again"
	case "dispatch.running":
		return "started an agent run"
	case "dispatch.succeeded":
		return "finished an agent run"
	case "dispatch.failed":
		return "had an agent run fail"
	case "dispatch.cancelled":
		return "cancelled an agent run"
	case "budget.warning":
		return "warned that spending is near its limit"
	case "milestone.created":
		return "created a milestone"
	case "milestone.locked":
		return "marked a milestone as shipped"
	case "milestone.unlocked":
		return "reopened a shipped milestone"
	case "milestone.member_added":
		return "added something to a milestone"
	case "milestone.member_removed":
		return "took something out of a milestone"
	case "roadmap.created":
		return "created a roadmap"
	case "roadmap.entry_set":
		return "placed a milestone on a roadmap"
	case "roadmap.entry_removed":
		return "took a milestone off a roadmap"
	case "worktree.created":
		return "made a working copy of the repository"
	default:
		return strings.ReplaceAll(strings.ReplaceAll(str(v), ".", " "), "_", " ")
	}
}

// verbLabel, verbIcon and verbConsequence turn a checkpoint answer verb into
// something a person can act on confidently: a plain-language label, a mark, and
// a sentence saying what happens next if they choose it.
func verbLabel(v any) string {
	switch str(v) {
	case "approve":
		return "Approve"
	case "request_changes":
		return "Ask for changes"
	case "override":
		return "Override the gate"
	case "deny":
		return "Deny"
	case "retry":
		return "Try again"
	case "cancel":
		return "Cancel this work"
	case "proceed":
		return "Go ahead"
	case "continue":
		return "Carry on"
	case "pause":
		return "Pause"
	default:
		return stateLabel(v)
	}
}

func verbIcon(v any) string {
	switch str(v) {
	case "approve", "proceed", "continue":
		return "approve"
	case "request_changes":
		return "request-changes"
	case "override":
		return "unlock"
	case "deny", "cancel":
		return "close"
	case "retry":
		return "refresh"
	case "pause":
		return "clock"
	default:
		return "arrow-right"
	}
}

func verbConsequence(v any) string {
	switch str(v) {
	case "approve":
		return "The work carries on from where it stopped, and the decision is recorded against your name."
	case "request_changes":
		return "Your reason goes back to whoever wrote this, and they revise it before it comes round again."
	case "override":
		return "The rule that blocked this is set aside, with your reason kept on the record as the justification."
	case "deny":
		return "The request is refused and the work stays where it is."
	case "retry":
		return "The same step runs again from the beginning."
	case "cancel":
		return "This work stops for good. It stays visible as part of the history."
	case "proceed":
		return "The agent continues with the option it proposed."
	case "continue":
		return "The run picks up where it left off."
	case "pause":
		return "The work is held where it is until you come back to it."
	default:
		return "The agent resumes with your answer."
	}
}

// docIcon maps a document type to its icon name in the sprite.
func docIcon(v any) string {
	switch str(v) {
	case "design":
		return "doc-design"
	case "spec":
		return "doc-spec"
	case "dev_plan":
		return "doc-devplan"
	case "research":
		return "doc-research"
	default:
		return "doc-note"
	}
}

// assetVersion is a short content hash of the embedded static assets, appended to
// their URLs as a query string. Without it a browser has no reason to refetch a
// stylesheet whose URL never changes, so a UI change can appear half-applied
// until someone thinks to hard-refresh — which cost real confusion once already.
// Hashing the content means any edit busts the cache and no edit busts it twice.
var assetVersion = func() string {
	h := sha256.New()
	entries, err := fs.ReadDir(uiStaticFS, "ui/static")
	if err != nil {
		return "dev"
	}
	for _, e := range entries {
		b, err := uiStaticFS.ReadFile("ui/static/" + e.Name())
		if err != nil {
			continue
		}
		h.Write([]byte(e.Name()))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}()

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
// active, the operator identity, the shell's own chrome (title, breadcrumbs and
// the navigation tree), and the view's own data.
//
// Breadcrumbs live here rather than in each view's body because they belong to
// the persistent top bar: "where am I" is then answered in one place, at one
// size, on every page.
type pageData struct {
	Active string
	Actor  string
	Title  string
	Crumbs []crumb
	Nav    *navScope
	Data   any
	// AssetV busts the browser cache when the embedded CSS or JS changes.
	AssetV string
}

// headed is implemented by view structs that know their own page title and
// breadcrumb trail. The shell lifts both out of the view, so no handler has to
// pass them separately and no view can drift from its own heading.
type headed interface {
	headTitle() string
	headCrumbs() []crumb
}

func (s *Server) page(ctx context.Context, active string, data any) pageData {
	p := pageData{Active: active, Actor: s.uiActor(), Data: data, AssetV: assetVersion}
	if h, ok := data.(headed); ok {
		p.Title = h.headTitle()
		p.Crumbs = h.headCrumbs()
	}
	if nav, err := s.navTree(ctx, p.Crumbs); err == nil {
		p.Nav = nav
	} else {
		// The tree is navigation furniture; losing it must never cost the page.
		s.Log.Warn("ui nav tree", "err", err)
	}
	return p
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

	// The bare root sends a browser to Home. The pattern is "/{$}", which in Go's
	// mux matches the root path *exactly* — a plain "/" would swallow every
	// unmatched path and turn genuine 404s into redirects.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui", http.StatusFound)
	})
	mux.HandleFunc("GET /ui", s.handleUIDashboard)
	mux.HandleFunc("GET /ui/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui", http.StatusFound)
	})
	mux.HandleFunc("GET /ui/inbox", s.handleUIInbox)
	mux.HandleFunc("GET /ui/documents", s.handleUIDocuments)
	mux.HandleFunc("GET /ui/work", s.handleUIWork)
	// The verb-shaped Planning view and the money-denominated Cost view are
	// superseded by the browsable surface and the Work view (SPEC-007 §0,
	// FR-7.2); their old URLs redirect so existing links still land somewhere
	// sensible.
	mux.HandleFunc("GET /ui/planning", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/project", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /ui/cost", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/work", http.StatusMovedPermanently)
	})
	// A document is now a page at its own readable address rather than a query
	// parameter (FR-1.1); the old address redirects.
	mux.HandleFunc("GET /ui/document", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/d/"+r.URL.Query().Get("path"), http.StatusMovedPermanently)
	})

	// The workflow surface (SPEC-007): every entity is a page at a real,
	// readable URL, reached by breadcrumb, child list or nav rail — never by
	// typing a path into a form (FR-1.1, SD-6).
	mux.HandleFunc("GET /ui/project", s.handleUIProject)
	mux.HandleFunc("GET /ui/i/{path...}", s.handleUIInitiativePage)
	mux.HandleFunc("GET /ui/f/{path...}", s.handleUIFeaturePage)
	mux.HandleFunc("GET /ui/d/{path...}", s.handleUIDocumentPage)
	mux.HandleFunc("GET /ui/m/{id}", s.handleUIMilestonePage)
	mux.HandleFunc("GET /ui/r/{id}", s.handleUIRoadmapPage)
	// The milestone and roadmap editors, loaded into a <dialog> (SPEC-010 FR-6).
	mux.HandleFunc("GET /ui/m/{id}/edit", s.handleUIMilestoneEdit)
	mux.HandleFunc("GET /ui/m/{id}/candidates", s.handleUIMilestoneCandidates)
	mux.HandleFunc("GET /ui/r/{id}/edit", s.handleUIRoadmapEdit)
	mux.HandleFunc("GET /ui/t/{id}", s.handleUITaskPage)
	// Seeing the work (SPEC-012): a run's transcript, and review health.
	mux.HandleFunc("GET /ui/run/{id}", s.handleUIRun)
	mux.HandleFunc("GET /ui/review-health", s.handleUIReviewHealth)

	// Actions on their things (FR-5): each carries the entity's id from the
	// page it was rendered on, and calls the same gated, audited service
	// method the CLI uses.
	mux.HandleFunc("POST /ui/entity/describe", s.handleEntityDescribe)
	mux.HandleFunc("POST /ui/entity/attach", s.handleEntityAttach)
	mux.HandleFunc("POST /ui/entity/estimate", s.handleEntityEstimateSet)
	mux.HandleFunc("POST /ui/entity/estimate/ai", s.handleEntityEstimateAI)
	mux.HandleFunc("POST /ui/document/primary", s.handleDocumentPrimary)
	mux.HandleFunc("POST /ui/document/review", s.handleEntityDocumentReview)
	mux.HandleFunc("POST /ui/feature/start", s.handleEntityFeatureStart)
	// Send to development (SPEC-011 FR-4): the send screen, the send itself
	// — the only route that writes the sent mark — and Withdraw.
	mux.HandleFunc("GET /ui/send", s.handleUISend)
	mux.HandleFunc("POST /ui/send", s.handleUISendPost)
	mux.HandleFunc("POST /ui/send/withdraw", s.handleUISendWithdraw)
	// The document page's actions (SPEC-011 FR-5.5, FR-6, FR-9).
	mux.HandleFunc("POST /ui/document/submit", s.handleDocSubmit)
	mux.HandleFunc("POST /ui/document/revise", s.handleDocRevise)
	mux.HandleFunc("POST /ui/document/detach", s.handleDocDetach)
	mux.HandleFunc("POST /ui/document/issue", s.handleDocIssue)
	mux.HandleFunc("POST /ui/document/approve", s.handleDocApprove)
	mux.HandleFunc("POST /ui/document/send-back", s.handleDocSendBack)
	mux.HandleFunc("POST /ui/document/request-review", s.handleDocRequestReview)
	mux.HandleFunc("POST /ui/document/release", s.handleDocRelease)
	mux.HandleFunc("POST /ui/feature/abandon", s.handleEntityFeatureAbandon)
	mux.HandleFunc("POST /ui/feature/new", s.handleEntityFeatureCreate)
	mux.HandleFunc("POST /ui/initiative/new", s.handleEntityInitiativeCreate)
	mux.HandleFunc("POST /ui/initiative/archive", s.handleEntityInitiativeArchive)
	// Milestones and roadmaps (SPEC-010 FR-2 to FR-5).
	mux.HandleFunc("POST /ui/milestone/new", s.handleMilestoneCreate)
	mux.HandleFunc("POST /ui/roadmap/new", s.handleRoadmapCreate)
	mux.HandleFunc("POST /ui/milestone/member/add", s.handleMilestoneMemberAdd)
	mux.HandleFunc("POST /ui/milestone/member/remove", s.handleMilestoneMemberRemove)
	mux.HandleFunc("POST /ui/milestone/lock", s.handleMilestoneLock)
	mux.HandleFunc("POST /ui/milestone/unlock", s.handleMilestoneUnlock)
	mux.HandleFunc("POST /ui/roadmap/entry/place", s.handleRoadmapEntryPlace)
	mux.HandleFunc("POST /ui/roadmap/entry/remove", s.handleRoadmapEntryRemove)

	// Live-region fragments (re-read through the normal service path on an SSE
	// signal, SD-5).
	mux.HandleFunc("GET /ui/frag/queue", s.handleFragQueue)
	mux.HandleFunc("GET /ui/frag/events", s.handleFragEvents)
	mux.HandleFunc("GET /ui/frag/work", s.handleFragWork)
	mux.HandleFunc("GET /ui/frag/calibration", s.handleFragCalibration)
	mux.HandleFunc("GET /ui/frag/inbox", s.handleFragInbox)
	mux.HandleFunc("GET /ui/frag/inbox-badge", s.handleFragInboxBadge)
	mux.HandleFunc("GET /ui/frag/timeline", s.handleFragTimeline)
	mux.HandleFunc("GET /ui/frag/run/{id}", s.handleFragRunProgress)
	mux.HandleFunc("GET /ui/frag/review-health", s.handleFragReviewHealth)

	// Realtime stream and the inbox respond.
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
	s.render(w, "page-dashboard", s.page(r.Context(), "home", sum))
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

func (s *Server) handleFragWork(w http.ResponseWriter, r *http.Request) {
	sum, err := s.DashboardSummary(r.Context(), 20)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-work", sum)
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
	// RevisionSpecs is set only for a design-revision checkpoint (SPEC-009
	// FR-9.2a): the affected specs, each answered keep-or-invalidate on its
	// own line. Every other kind answers with one verb from Options.
	RevisionSpecs []revisionSpecView
}

// revisionSpecView is one line of the design-revision form.
type revisionSpecView struct {
	SpecDocID   string `json:"spec_doc_id"`
	SpecPath    string `json:"spec_path"`
	FeatureName string `json:"feature_name"`
}

func (s *Server) inboxItems(r *http.Request) ([]inboxItem, error) {
	pending, err := s.Store.PendingCheckpoints(r.Context())
	if err != nil {
		return nil, err
	}
	items := make([]inboxItem, 0, len(pending))
	for _, cp := range pending {
		item := inboxItem{Checkpoint: cp, Options: answerOptions(cp.Kind)}
		if cp.Kind == "design-revision" {
			var cctx struct {
				Affected []revisionSpecView `json:"affected"`
			}
			if err := json.Unmarshal(cp.Context, &cctx); err == nil {
				item.RevisionSpecs = cctx.Affected
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Server) handleUIInbox(w http.ResponseWriter, r *http.Request) {
	items, err := s.inboxItems(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-inbox", s.page(r.Context(), "inbox", items))
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
	// The kind determines the payload schema; look it up rather than trust the
	// form (the form's kind is presentational only).
	ctx := r.Context()
	cpBefore, err := store.GetCheckpoint(ctx, s.Store.Pool, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var response map[string]any
	if cpBefore.Kind == "design-revision" {
		// The one per-item answer (SPEC-009 FR-9.2a): a keep-or-invalidate
		// decision for every spec the checkpoint listed, no verb.
		response, err = designRevisionResponse(cpBefore, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		verb := strings.TrimSpace(r.FormValue("verb"))
		if verb == "" {
			http.Error(w, "an answer is required", http.StatusBadRequest)
			return
		}
		response = responseFor(cpBefore.Kind, verb, strings.TrimSpace(r.FormValue("reason")))
	}

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

// designRevisionResponse builds the per-spec answer payload from the posted
// form: one decision_<spec-id> field per affected spec, keep or invalidate.
// Every listed spec must be answered — a spec silently unanswered would be a
// spec silently kept, which is the exact silence the checkpoint exists to
// prevent.
func designRevisionResponse(cp *store.Checkpoint, r *http.Request) (map[string]any, error) {
	var cctx struct {
		Affected []struct {
			SpecDocID string `json:"spec_doc_id"`
		} `json:"affected"`
	}
	if err := json.Unmarshal(cp.Context, &cctx); err != nil {
		return nil, fmt.Errorf("this checkpoint's context could not be read: %v", err)
	}
	invalidate := []string{}
	keep := []string{}
	for _, a := range cctx.Affected {
		switch strings.TrimSpace(r.FormValue("decision_" + a.SpecDocID)) {
		case "keep":
			keep = append(keep, a.SpecDocID)
		case "invalidate":
			invalidate = append(invalidate, a.SpecDocID)
		default:
			return nil, fmt.Errorf("answer keep or invalidate for every specification listed")
		}
	}
	response := map[string]any{"invalidate": invalidate, "keep": keep}
	if reason := strings.TrimSpace(r.FormValue("reason")); reason != "" {
		response["reason"] = reason
	}
	return response, nil
}

// --- Documents ---

func (s *Server) handleUIDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := store.ListLiveDocuments(r.Context(), s.Store.Pool)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-documents", s.page(r.Context(), "documents", docs))
}

// --- Work (FR-7.2) ---

func (s *Server) handleUIWork(w http.ResponseWriter, r *http.Request) {
	view, err := s.workView(r)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-work", s.page(r.Context(), "work", view))
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
	// No context here by design: the error page is the one page that must render
	// even when a read is what failed, so it goes without the navigation tree.
	s.render(w, "page-error", pageData{Actor: s.uiActor(), Data: err.Error(), AssetV: assetVersion})
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
	case n <= 0:
		return "0"
	case n < 500:
		// Real work, but less than the unit. Rounding would print "0k", which
		// reads as "nothing happened" — say what is true instead.
		return "<1k"
	default:
		return groupThousands((n+500)/1_000) + "k"
	}
}

// groupThousands puts separators into a plain integer ("1240" → "1,240"), so a
// large figure stays readable without changing unit.
func groupThousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
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
