package server

import (
	"context"
	"html/template"
	"net/http"

	"github.com/google/uuid"

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

// --- Documents ---

// docPageData is a single document with its rendered body and comment thread
// (FR-5.1). The body is read from the working tree (git) and rendered read-only;
// there is no edit control (CC-5). ReviewCheckpointID is set when the document's
// review is human-gated (an open review-escalation checkpoint refs it), which is
// when — and only when — the review controls appear (SPEC-006 FR-5.1, SD-4).
type docPageData struct {
	Document           store.Document
	Body               template.HTML
	Comments           []store.Comment
	ReviewCheckpointID *uuid.UUID
	Notice             string
	Error              string
}

func (s *Server) documentView(r *http.Request) (*docPageData, error) {
	return s.documentViewByPath(r.Context(), r.URL.Query().Get("path"), "", "")
}

func (s *Server) documentViewByPath(ctx context.Context, path, notice, errMsg string) (*docPageData, error) {
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
	page := &docPageData{
		Document: *doc,
		Body:     renderMarkdown(body),
		Comments: comments,
		Notice:   notice,
		Error:    errMsg,
	}
	// Is this document's review human-gated? If an open review-escalation
	// checkpoint refs it, the review controls appear (SD-4).
	if cp, err := s.openReviewCheckpoint(ctx, doc.ID); err == nil && cp != nil {
		page.ReviewCheckpointID = &cp.ID
	}
	return page, nil
}

// openReviewCheckpoint returns the pending review-escalation checkpoint that
// refs a document, if any — a filter over PendingCheckpoints, not a new read.
func (s *Server) openReviewCheckpoint(ctx context.Context, docID uuid.UUID) (*store.Checkpoint, error) {
	pending, err := s.Store.PendingCheckpoints(ctx)
	if err != nil {
		return nil, err
	}
	for i := range pending {
		cp := pending[i]
		if cp.Kind == "review-escalation" && cp.RefType == "document" && cp.RefID == docID {
			return &cp, nil
		}
	}
	return nil, nil
}

// --- Work (FR-7.2) ---

// workView is the roll-up surface counted in tokens, the unit of work
// (DESIGN-008 D-4). It replaces the money-denominated Cost view: the project
// total plus per-initiative (transitive), per-feature and per-milestone sizes,
// each with its confidence tier and the `?` for work not yet estimated. No
// figure on this page is money; the engine's cost ledger is untouched but
// dormant behind the surface (SD-5, Q-C).
type workView struct {
	Project     sizing.Rollup
	Initiatives []entityWork
	Features    []entityWork
	Milestones  []milestoneCard
}

// entityWork is one labelled line of work, sized in tokens.
type entityWork struct {
	Name string
	URL  string
	Size sizing.Rollup
}

func (s *Server) workView(r *http.Request) (workView, error) {
	ctx := r.Context()
	var view workView

	proj, err := s.projectRollup(ctx)
	if err != nil {
		return view, err
	}
	view.Project = proj

	// Every initiative in the tree (roots + nested), each transitive; and every
	// feature under them, sized through the same sizing engine the pages use.
	inits, err := s.allInitiatives(ctx)
	if err != nil {
		return view, err
	}
	for _, in := range inits {
		path, err := s.initiativePath(ctx, in.ID)
		if err != nil {
			return view, err
		}
		roll, err := s.initiativeRollup(ctx, in.ID)
		if err != nil {
			return view, err
		}
		view.Initiatives = append(view.Initiatives, entityWork{
			Name: path, URL: "/ui/i/" + path, Size: roll})

		feats, err := store.FeaturesForInitiative(ctx, s.Store.Pool, in.ID)
		if err != nil {
			return view, err
		}
		for _, f := range feats {
			froll, err := s.featureRollup(ctx, f.ID)
			if err != nil {
				return view, err
			}
			view.Features = append(view.Features, entityWork{
				Name: path + "/" + f.Slug, URL: "/ui/f/" + path + "/" + f.Slug, Size: froll})
		}
	}

	// Milestones, each showing the ticked count and the token bar (FR-8.3).
	ms, err := store.ListMilestones(ctx, s.Store.Pool)
	if err != nil {
		return view, err
	}
	for _, m := range ms {
		card, err := s.milestoneCardFor(ctx, m)
		if err != nil {
			return view, err
		}
		view.Milestones = append(view.Milestones, card)
	}
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
