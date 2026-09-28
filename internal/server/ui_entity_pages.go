package server

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"subutai/internal/store"
)

// The plain read-only pages for milestones, roadmaps, tasks and documents, so
// every link resolves (SPEC-007 FR-9, FR-11). None is reached from a global
// index — there is no "all milestones" list (DESIGN-008 §5.1a); they are always
// arrived at by clicking through from a member or owner.

// --- Milestone (FR-9.1, FR-8.3) ---

// memberRow is one member of a milestone in its checklist display: a label
// linking to the member's page and whether it is finished (D-10).
type memberRow struct {
	ID    uuid.UUID // with Kind, what a removal form posts (SPEC-010 FR-3.2)
	Label string
	URL   string
	Done  bool
	Kind  string
	Sub   string // a short status beside it, such as a checklist's jobs ticked
}

type milestonePage struct {
	Milestone store.Milestone
	Card      milestoneCard
	Owner     crumb // link to the owning entity's page
	Members   []memberRow
	Locked    bool
	// A milestone can itself be a member of another (SPEC-010 FR-3.1): the
	// milestones it is directly in, and the ones it could join.
	MemberOf         []milestoneCard
	MilestoneChoices []milestoneChoiceGroup
	Notice           string
	Error            string
}

func (s *Server) handleUIMilestonePage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "milestone", r.PathValue("id"))
		return
	}
	s.renderMilestonePage(w, r, id, "", "")
}

// renderMilestonePage renders a milestone's page, with a notice or error when
// it is the response to an action taken on it.
func (s *Server) renderMilestonePage(w http.ResponseWriter, r *http.Request, id uuid.UUID, notice, errMsg string) {
	ctx := r.Context()
	m, err := store.GetMilestone(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "milestone", id.String(), err)
		return
	}
	card, err := s.milestoneCardFor(ctx, *m)
	if err != nil {
		s.uiError(w, err)
		return
	}
	rows, err := s.milestoneMemberRows(ctx, m.ID)
	if err != nil {
		s.uiError(w, err)
		return
	}
	memberOf, err := s.memberOfCards(ctx, "milestone", m.ID)
	if err != nil {
		s.uiError(w, err)
		return
	}
	choices, err := s.milestoneChoices(ctx, "milestone", m.ID, memberOf)
	if err != nil {
		s.uiError(w, err)
		return
	}
	page := milestonePage{
		Milestone: *m, Card: card, Owner: s.ownerCrumb(ctx, m.OwnerType, m.OwnerID),
		Members: rows, Locked: m.LockedAt != nil,
		MemberOf: memberOf, MilestoneChoices: choices, Notice: notice, Error: errMsg,
	}
	pushPageURL(w, r, milestoneURL(m.ID))
	s.render(w, "page-milestone", s.page(r.Context(), "browse", page))
}

// milestoneMemberRows resolves a milestone's live members to display rows. A
// feature is done when its state is done; an initiative or nested milestone is
// shown with a completion fraction resolved through its own leaves.
func (s *Server) milestoneMemberRows(ctx context.Context, milestoneID uuid.UUID) ([]memberRow, error) {
	members, err := store.Members(ctx, s.Store.Pool, milestoneID)
	if err != nil {
		return nil, err
	}
	var out []memberRow
	for _, mem := range members {
		switch mem.MemberType {
		case "feature":
			f, err := store.GetFeature(ctx, s.Store.Pool, mem.MemberID)
			if err != nil {
				return nil, err
			}
			path, err := s.featurePath(ctx, f)
			if err != nil {
				return nil, err
			}
			out = append(out, memberRow{ID: f.ID, Label: f.Name, URL: "/ui/f/" + path,
				Done: f.State == "done", Kind: "feature"})
		case "initiative":
			in, err := store.GetInitiative(ctx, s.Store.Pool, mem.MemberID)
			if err != nil {
				return nil, err
			}
			path, err := s.initiativePath(ctx, in.ID)
			if err != nil {
				return nil, err
			}
			n, err := store.NonTerminalFeatureCount(ctx, s.Store.Pool, in.ID)
			if err != nil {
				return nil, err
			}
			out = append(out, memberRow{ID: in.ID, Label: in.Name, URL: "/ui/i/" + path,
				Done: n == 0, Kind: "initiative"})
		case "milestone":
			sub, err := store.GetMilestone(ctx, s.Store.Pool, mem.MemberID)
			if err != nil {
				return nil, err
			}
			out = append(out, memberRow{ID: sub.ID, Label: sub.Name, URL: "/ui/m/" + sub.ID.String(),
				Done: sub.LockedAt != nil, Kind: "milestone"})
		case "checklist":
			// Done when it has jobs and every one is ticked (SPEC-014 FR-2.6).
			c, err := store.GetChecklist(ctx, s.Store.Pool, mem.MemberID)
			if err != nil {
				return nil, err
			}
			card, err := s.checklistCardFor(ctx, *c)
			if err != nil {
				return nil, err
			}
			out = append(out, memberRow{ID: c.ID, Label: c.Name, URL: card.URL,
				Done: card.Done, Kind: "checklist", Sub: card.Status()})
		}
	}
	return out, nil
}

// ownerCrumb builds a link to an owning entity's page. Milestones and roadmaps
// are owned by the project or an initiative (DESIGN-008 D-9); documents may also
// be owned by a feature, so all three kinds resolve here. Anything unresolvable
// falls back to the project, which always exists.
func (s *Server) ownerCrumb(ctx context.Context, ownerType string, ownerID *uuid.UUID) crumb {
	project := crumb{Label: "Project", URL: "/ui/project"}
	if ownerType == "project" || ownerID == nil {
		return project
	}
	switch ownerType {
	case "initiative":
		path, err := s.initiativePath(ctx, *ownerID)
		if err != nil {
			return project
		}
		label, id := path, ""
		if in, err := store.GetInitiative(ctx, s.Store.Pool, *ownerID); err == nil {
			label, id = in.Name, in.PublicID
		}
		return crumb{ID: id, Label: label, URL: "/ui/i/" + path}
	case "feature":
		f, err := store.GetFeature(ctx, s.Store.Pool, *ownerID)
		if err != nil {
			return project
		}
		path, err := s.featurePath(ctx, f)
		if err != nil {
			return project
		}
		return crumb{ID: f.PublicID, Label: f.Name, URL: "/ui/f/" + path}
	}
	return project
}

// --- Roadmap (FR-9.1) ---

type roadmapPage struct {
	Roadmap store.Roadmap
	Owner   crumb
	Card    roadmapCard
}

func (s *Server) handleUIRoadmapPage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "roadmap", r.PathValue("id"))
		return
	}
	ctx := r.Context()
	rm, err := store.GetRoadmap(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "roadmap", id.String(), err)
		return
	}
	cards, err := s.ownedRoadmapCards(ctx, rm.OwnerType, rm.OwnerID)
	if err != nil {
		s.uiError(w, err)
		return
	}
	// Find this roadmap's card among its owner's (cheap; roadmaps are few).
	var card roadmapCard
	for _, c := range cards {
		if c.ID == rm.ID {
			card = c
		}
	}
	if card.ID == uuid.Nil {
		card = roadmapCard{ID: rm.ID, Name: rm.Name, URL: "/ui/r/" + rm.ID.String()}
	}
	page := roadmapPage{Roadmap: *rm, Owner: s.ownerCrumb(ctx, rm.OwnerType, rm.OwnerID), Card: card}
	s.render(w, "page-roadmap", s.page(r.Context(), "browse", page))
}

// --- Task (FR-9.1) ---

type taskPage struct {
	Task        store.Task
	FeatureName string
	FeatureURL  string
	Runs        []runRow // the agent runs on this task (SPEC-012 FR-4.1)
}

func (s *Server) handleUITaskPage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "task", r.PathValue("id"))
		return
	}
	ctx := r.Context()
	t, err := store.GetTask(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "task", id.String(), err)
		return
	}
	page := taskPage{Task: *t}
	if page.Runs, err = s.runRowsFor(ctx, "task", t.ID); err != nil {
		s.uiError(w, err)
		return
	}
	if f, err := store.GetFeature(ctx, s.Store.Pool, t.FeatureID); err == nil {
		page.FeatureName = f.Name
		if path, err := s.featurePath(ctx, f); err == nil {
			page.FeatureURL = "/ui/f/" + path
		}
	}
	s.render(w, "page-task", s.page(r.Context(), "browse", page))
}

// --- Document (FR-1.1 /ui/d/<path>, FR-11 review controls kept) ---

// entityDocPage wraps the existing document view with breadcrumbs to its owner,
// so a document is a first-class page in the browsable surface.
type entityDocPage struct {
	*docPageData
	Breadcrumbs []crumb
	OwnerCrumb  crumb
	Runs        []runRow // the agent reviews of this document (SPEC-012 FR-4.2)
}

func (s *Server) handleUIDocumentPage(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	view, err := s.documentViewByPath(r.Context(), path, "", "")
	if err == store.ErrNotFound {
		// A document that moved leaves its old address pointing at its new
		// one (SPEC-015 FR-4.5).
		if to := s.movedFrom(r.Context(), path); to != "" {
			http.Redirect(w, r, "/ui/d/"+to, http.StatusFound)
			return
		}
	}
	if err != nil {
		s.notFoundOrErr(w, r, "document", path, err)
		return
	}
	page := entityDocPage{
		docPageData: view,
		OwnerCrumb:  s.ownerCrumb(r.Context(), view.Document.OwnerType, view.Document.OwnerID),
	}
	page.Breadcrumbs = []crumb{page.OwnerCrumb, docCrumb(view.Document)}
	if page.Runs, err = s.runRowsFor(r.Context(), "document", view.Document.ID); err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-entity-document", s.page(r.Context(), "documents", page))
}

// docCrumb is a document's own breadcrumb, led by its ID (SPEC-015 SD-17).
func docCrumb(d store.Document) crumb {
	return crumb{ID: idFor(d.PublicID, d.Title), Label: d.Title, Here: true}
}
