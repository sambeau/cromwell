package server

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/config"
	"subutai/internal/lifecycle"
	"subutai/internal/sizing"
	"subutai/internal/store"
)

// The workflow surface, Stage A (SPEC-007): the project as a browsable place.
// Every initiative, feature and document is a page at a real, readable URL,
// reached by breadcrumb, child list, or the nav rail — never by typing a path
// into a form (SD-6, D-1). Each page is its main design document, framed by the
// entity's status, size in tokens, relations and the actions valid for it in
// its state. Every action here calls the same gated, audited service method the
// CLI uses, with the entity implied by the page's URL (FR-5).

// --- View model (all fields are read-only display; mutations go through the
// existing service methods) ---

type crumb struct {
	// ID is the entity's minted ID, shown before its label (SPEC-015 SD-17).
	ID    string
	Label string
	URL   string
	Kind  string // entity kind, so the crumb carries its type icon
	Here  bool   // the current page's own crumb, rendered unlinked
}

// childCard is one entry in a parent's list of children — a sub-initiative or a
// feature — carrying its human summary and its size roll-up (FR-2.1).
type childCard struct {
	PublicID    string // "INIT-014", "FEAT-023" (SPEC-015 SD-17)
	Kind        string // "initiative" | "feature"
	Name        string
	Description string
	URL         string
	State       string // feature state; empty for initiatives
	Size        sizing.Rollup
}

// docCard is one attached document in the Documents section (FR-3.2).
type docCard struct {
	// PublicID and Revision are the document's ID; empty for one known only
	// by its path (SPEC-015 SD-12).
	PublicID  string
	Revision  int
	ID        uuid.UUID
	Title     string
	Type      string
	State     string
	URL       string
	IsPrimary bool
}

// milestoneCard is a milestone summarised for a relation list (FR-8.3): the
// ticked count and the token bar (finished tokens over estimated) both shown,
// per DESIGN-008 D-10.
type milestoneCard struct {
	ID          uuid.UUID
	PublicID    string // "MS-004" (SPEC-017 FR-3.3)
	Name        string
	URL         string
	Done        int
	Total       int
	DoneTokens  int64
	TotalTokens int64
	Locked      bool
}

// TokenPct is the token bar width for the card, clamped to [0,100].
func (m milestoneCard) TokenPct() int {
	if m.TotalTokens <= 0 {
		return 0
	}
	p := int(m.DoneTokens * 100 / m.TotalTokens)
	if p > 100 {
		return 100
	}
	return p
}

type roadmapCard struct {
	ID         uuid.UUID
	PublicID   string // "RM-001" (SPEC-017 FR-3.3)
	Name       string
	URL        string
	Milestones []milestoneCard
}

// entityPage is the one anatomy shared by Project, Initiative and Feature pages
// (DESIGN-008 §5.2): breadcrumbs, description, the document-as-body, a rail of
// status/size/actions, and the relation sections.
type entityPage struct {
	Kind        string // "project" | "initiative" | "feature"
	RefType     string // "project" | "initiative" | "feature"
	ID          uuid.UUID
	PublicID    string // the minted ID, "INIT-014" (SPEC-015 SD-17)
	Title       string
	Path        string // server-derived slug path (never user-typed) for action forms and links
	Slug        string
	Description string
	Editable    bool // description/title editable here (initiatives and features, FR-4.2)

	Breadcrumbs []crumb

	HasBody bool
	Body    template.HTML
	BodyDoc *docCard // the document rendered as the body, so the page can name it

	State string        // feature lifecycle state; empty otherwise
	Size  sizing.Rollup // the token roll-up (FR-2.1), tier and the ? for unestimated work
	Done  int64         // tokens actually spent on this entity so far (the rail's DONE)
	// DoneUnmeasured is true when the entity includes work done in chat or by
	// a person, so Done is "at least" (SPEC-020 FR-7.3).
	DoneUnmeasured bool

	// Feature actions and their plain-words gate reasons (FR-2.2, FR-5.1).
	CanStart    bool
	StartReason string
	CanAbandon  bool
	IsTerminal  bool
	// Send to development (SPEC-011 FR-4.1): the feature's send card, and
	// on an initiative whether any of its features could be sent.
	Send       sendCard
	CanSendAny bool
	// ShowStartCard is whether the blocked Start building card appears: not
	// for a feature nobody has sent, whose next step is sending it.
	ShowStartCard bool

	Children        []childCard
	Documents       []docCard
	OwnedMilestones []milestoneCard
	OwnedRoadmaps   []roadmapCard
	// The checklists planned here (SPEC-014 FR-3.1).
	OwnedChecklists []checklistCard
	MemberOf        []milestoneCard
	// The open milestones this entity could be added to, grouped by where each
	// is planned — the member-side end of D-12 (SPEC-010 FR-3.1).
	MilestoneChoices []milestoneChoiceGroup
	Activity         []store.AuditEvent
	// Timeline is a feature's journey (SPEC-012 FR-6). On a feature page it
	// replaces Activity, whose events it carries as its detail level (SD-7).
	Timeline *timelineView
	// Tasks are a feature's tasks with who executes each, and ClaimWaiting
	// says when a claim holds its working copy (SPEC-020 FR-1.6, FR-4.4).
	Tasks        []taskRow
	ClaimWaiting string

	// Bugs (SPEC-019 FR-2.5, FR-5.1, FR-5.2): the triage card on a bug's
	// page, the open bugs hanging off this initiative or feature, and
	// whether a bug can be reported here.
	Bug          *bugCard
	Bugs         []bugRow
	CanReportBug bool

	Notice string
	Error  string
}

// --- Path and breadcrumb helpers (all server-derived from the entity id) ---

// initiativePath returns an initiative's root-first slug path ("auth/basic").
func (s *Server) initiativePath(ctx context.Context, id uuid.UUID) (string, error) {
	anc, err := store.InitiativeAncestors(ctx, s.Store.Pool, id) // nearest first
	if err != nil {
		return "", err
	}
	parts := make([]string, len(anc))
	for i, a := range anc {
		parts[len(anc)-1-i] = a.Slug
	}
	return strings.Join(parts, "/"), nil
}

// featurePath returns a feature's full path ("auth/basic/login").
func (s *Server) featurePath(ctx context.Context, f *store.Feature) (string, error) {
	ip, err := s.initiativePath(ctx, f.InitiativeID)
	if err != nil {
		return "", err
	}
	return ip + "/" + f.Slug, nil
}

// initiativeCrumbs builds the breadcrumb chain Project ▸ … ▸ initiative, each
// crumb linking to that ancestor's page (FR-1.2).
func (s *Server) initiativeCrumbs(ctx context.Context, id uuid.UUID) ([]crumb, error) {
	anc, err := store.InitiativeAncestors(ctx, s.Store.Pool, id) // nearest first
	if err != nil {
		return nil, err
	}
	out := []crumb{{Label: "Project", URL: "/ui/project", Kind: "project"}}
	var cum []string
	for i := len(anc) - 1; i >= 0; i-- {
		cum = append(cum, anc[i].Slug)
		out = append(out, crumb{ID: anc[i].PublicID, Label: anc[i].Name, URL: "/ui/i/" + strings.Join(cum, "/"), Kind: "initiative"})
	}
	return out, nil
}

// --- Size roll-ups (through the sizing engine, never re-derived — FR-2.1) ---

func (s *Server) initiativeRollup(ctx context.Context, id uuid.UUID) (sizing.Rollup, error) {
	n, err := store.InitiativeSizingNode(ctx, s.Store.Pool, id)
	if err != nil {
		return sizing.Rollup{}, err
	}
	return sizing.RollUp(n), nil
}

func (s *Server) featureRollup(ctx context.Context, id uuid.UUID) (sizing.Rollup, error) {
	n, err := store.FeatureSizingNode(ctx, s.Store.Pool, id)
	if err != nil {
		return sizing.Rollup{}, err
	}
	return sizing.RollUp(n), nil
}

// projectRollup rolls the whole project up by treating the root initiatives as
// the children of a synthetic project node.
func (s *Server) projectRollup(ctx context.Context) (sizing.Rollup, error) {
	roots, err := store.RootInitiatives(ctx, s.Store.Pool)
	if err != nil {
		return sizing.Rollup{}, err
	}
	proj := sizing.Node{Ref: sizing.Ref{Type: "project", Name: "Project"}}
	for _, in := range roots {
		if in.Archived {
			continue
		}
		cn, err := store.InitiativeSizingNode(ctx, s.Store.Pool, in.ID)
		if err != nil {
			return sizing.Rollup{}, err
		}
		proj.Children = append(proj.Children, cn)
	}
	return sizing.RollUp(proj), nil
}

// --- Relation assembly shared by the entity pages ---

// documentCards turns an owner's documents into cards, marking the one that is
// (or resolves to) the main document so the Documents section shows it.
func (s *Server) documentCards(docs []store.Document, primaryID uuid.UUID) []docCard {
	out := make([]docCard, 0, len(docs))
	for _, d := range docs {
		out = append(out, docCard{
			PublicID: d.PublicID, Revision: d.Revision,
			ID: d.ID, Title: d.Title, Type: d.Type, State: string(d.State),
			URL: "/ui/d/" + d.Path, IsPrimary: d.ID == primaryID,
		})
	}
	return out
}

// bodyFor resolves and renders an owner's main design document as the page body
// (FR-3.1). It returns the rendered HTML and the card naming the document, or
// ok=false for the empty state.
func (s *Server) bodyFor(ctx context.Context, ownerType string, ownerID *uuid.UUID) (template.HTML, *docCard, bool) {
	doc, err := store.PrimaryDocForOwner(ctx, s.Store.Pool, ownerType, ownerID)
	if err != nil {
		return "", nil, false
	}
	var body string
	if raw, err := s.readDocFile(doc.Path); err == nil {
		if _, md, ferr := config.SplitFrontMatter(string(raw)); ferr == nil {
			body = md
		} else {
			body = string(raw)
		}
	} else {
		body = "_The document file could not be read from the working tree._"
	}
	return renderMarkdown(body), &docCard{
		PublicID: doc.PublicID, Revision: doc.Revision,
		ID: doc.ID, Title: doc.Title, Type: doc.Type, State: string(doc.State),
		URL: "/ui/d/" + doc.Path, IsPrimary: true,
	}, true
}

// milestoneCardFor summarises a milestone for a relation list, computing both
// the ticked count and the token progress (FR-8.3).
func (s *Server) milestoneCardFor(ctx context.Context, m store.Milestone) (milestoneCard, error) {
	locked := m.LockedAt != nil
	var prog store.Progress
	var err error
	if locked {
		prog, err = store.SnapshotProgress(ctx, s.Store.Pool, m.ID)
	} else {
		prog, err = store.LiveProgress(ctx, s.Store.Pool, m.ID)
	}
	if err != nil {
		return milestoneCard{}, err
	}
	done, total, err := s.milestoneTokens(ctx, prog.Leaves)
	if err != nil {
		return milestoneCard{}, err
	}
	return milestoneCard{
		ID: m.ID, PublicID: m.PublicID, Name: m.Name, URL: "/ui/m/" + m.ID.String(),
		Done: prog.Done, Total: prog.Total, DoneTokens: done, TotalTokens: total, Locked: locked,
	}, nil
}

// milestoneTokens sums the estimated tokens over a milestone's resolved leaf
// features, and the subset already finished — the numerator and denominator of
// the token bar (FR-8.3). Each leaf's size comes from the sizing engine, so it
// respects task decomposition, never a re-derivation.
func (s *Server) milestoneTokens(ctx context.Context, leaves []uuid.UUID) (done, total int64, err error) {
	for _, id := range leaves {
		roll, err := s.featureRollup(ctx, id)
		if err != nil {
			return 0, 0, err
		}
		if !roll.Estimated {
			continue
		}
		total += roll.Tokens
		f, err := store.GetFeature(ctx, s.Store.Pool, id)
		if err != nil {
			return 0, 0, err
		}
		if f.State == "done" {
			done += roll.Tokens
		}
	}
	return done, total, nil
}

func (s *Server) ownedMilestoneCards(ctx context.Context, ownerType string, ownerID *uuid.UUID) ([]milestoneCard, error) {
	ms, err := store.MilestonesOwnedBy(ctx, s.Store.Pool, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]milestoneCard, 0, len(ms))
	for _, m := range ms {
		card, err := s.milestoneCardFor(ctx, m)
		if err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, nil
}

func (s *Server) memberOfCards(ctx context.Context, memberType string, memberID uuid.UUID) ([]milestoneCard, error) {
	ms, err := store.MilestonesForMember(ctx, s.Store.Pool, memberType, memberID)
	if err != nil {
		return nil, err
	}
	out := make([]milestoneCard, 0, len(ms))
	for _, m := range ms {
		card, err := s.milestoneCardFor(ctx, m)
		if err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, nil
}

func (s *Server) ownedRoadmapCards(ctx context.Context, ownerType string, ownerID *uuid.UUID) ([]roadmapCard, error) {
	rms, err := store.RoadmapsOwnedBy(ctx, s.Store.Pool, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]roadmapCard, 0, len(rms))
	for _, rm := range rms {
		entries, err := store.RoadmapEntries(ctx, s.Store.Pool, rm.ID)
		if err != nil {
			return nil, err
		}
		card := roadmapCard{ID: rm.ID, PublicID: rm.PublicID, Name: rm.Name, URL: "/ui/r/" + rm.ID.String()}
		for _, e := range entries {
			m, err := store.GetMilestone(ctx, s.Store.Pool, e.MilestoneID)
			if err != nil {
				return nil, err
			}
			mc, err := s.milestoneCardFor(ctx, *m)
			if err != nil {
				return nil, err
			}
			card.Milestones = append(card.Milestones, mc)
		}
		out = append(out, card)
	}
	return out, nil
}

// --- Handlers ---

func (s *Server) handleUIProject(w http.ResponseWriter, r *http.Request) {
	page, err := s.projectPage(r.Context(), "", "")
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-entity", s.page(r.Context(), "browse", page))
}

func (s *Server) handleUIInitiativePage(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.PathValue("path"), "/")
	if path == "" {
		http.Redirect(w, r, "/ui/project", http.StatusFound)
		return
	}
	in, err := s.Store.InitiativeBySlugPath(r.Context(), strings.Split(path, "/"))
	if err != nil {
		s.uiNotFound(w, r, "initiative", path)
		return
	}
	page, err := s.initiativePage(r.Context(), in, "", "")
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-entity", s.page(r.Context(), "browse", page))
}

func (s *Server) handleUIFeaturePage(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.PathValue("path"), "/")
	f, err := s.featureByPath(r.Context(), path)
	if err != nil {
		s.uiNotFound(w, r, "feature", path)
		return
	}
	page, err := s.featurePage(r.Context(), f, "", "")
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-entity", s.page(r.Context(), "browse", page))
}

// uiNotFound renders a clear not-found page (FR-1.1), never a 500.
func (s *Server) uiNotFound(w http.ResponseWriter, r *http.Request, kind, path string) {
	w.WriteHeader(http.StatusNotFound)
	s.render(w, "page-notfound", s.page(r.Context(), "browse", map[string]string{"Kind": kind, "Path": path}))
}

// --- Page builders ---

func (s *Server) projectPage(ctx context.Context, notice, errMsg string) (*entityPage, error) {
	page := &entityPage{
		Kind: "project", RefType: "project", Title: "Project",
		Breadcrumbs: []crumb{{Label: "Project", URL: "/ui/project", Kind: "project", Here: true}},
		Notice:      notice, Error: errMsg,
	}
	body, bodyDoc, ok := s.bodyFor(ctx, "project", nil)
	page.HasBody, page.Body, page.BodyDoc = ok, body, bodyDoc

	roll, err := s.projectRollup(ctx)
	if err != nil {
		return nil, err
	}
	page.Size = roll

	roots, err := store.RootInitiatives(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	for _, in := range roots {
		r, err := s.initiativeRollup(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		page.Children = append(page.Children, childCard{
			PublicID: in.PublicID, Kind: "initiative", Name: in.Name, Description: in.Description,
			URL: "/ui/i/" + in.Slug, Size: r,
		})
	}

	docs, err := store.DocumentsForOwner(ctx, s.Store.Pool, "project", nil)
	if err != nil {
		return nil, err
	}
	page.Documents = s.documentCards(docs, primaryDocID(page.BodyDoc))

	if page.OwnedMilestones, err = s.ownedMilestoneCards(ctx, "project", nil); err != nil {
		return nil, err
	}
	if page.OwnedRoadmaps, err = s.ownedRoadmapCards(ctx, "project", nil); err != nil {
		return nil, err
	}
	if page.OwnedChecklists, err = s.ownedChecklistCards(ctx, "project", nil); err != nil {
		return nil, err
	}
	if page.Activity, err = s.Store.AuditTail(ctx, "project", nil, 10); err != nil {
		return nil, err
	}
	return page, nil
}

func (s *Server) initiativePage(ctx context.Context, in *store.Initiative, notice, errMsg string) (*entityPage, error) {
	path, err := s.initiativePath(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	crumbs, err := s.initiativeCrumbs(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if len(crumbs) > 0 {
		// Marked as the current page (the template renders it unlinked), but its
		// URL is kept so the navigation tree can tell where we are.
		crumbs[len(crumbs)-1].Here = true
	}
	page := &entityPage{
		Kind: "initiative", RefType: "initiative", ID: in.ID, PublicID: in.PublicID, Title: in.Name,
		Path: path, Slug: in.Slug, Description: in.Description, Editable: true,
		Breadcrumbs: crumbs, Notice: notice, Error: errMsg,
	}
	body, bodyDoc, ok := s.bodyFor(ctx, "initiative", &in.ID)
	page.HasBody, page.Body, page.BodyDoc = ok, body, bodyDoc

	if page.Size, err = s.initiativeRollup(ctx, in.ID); err != nil {
		return nil, err
	}
	page.Done, page.DoneUnmeasured, _ = store.ActualTokens(ctx, s.Store.Pool, "initiative", in.ID)

	// Children: sub-initiatives, then features.
	children, err := store.ChildInitiatives(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range children {
		r, err := s.initiativeRollup(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		page.Children = append(page.Children, childCard{
			PublicID: c.PublicID, Kind: "initiative", Name: c.Name, Description: c.Description,
			URL: "/ui/i/" + path + "/" + c.Slug, Size: r,
		})
	}
	feats, err := store.FeaturesForInitiative(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range feats {
		if f.State == lifecycle.FeatIdea || f.State == lifecycle.FeatReady {
			if _, err := store.GetFeatureSend(ctx, s.Store.Pool, f.ID); err == store.ErrNotFound {
				page.CanSendAny = true
			}
		}
		r, err := s.featureRollup(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		page.Children = append(page.Children, childCard{
			PublicID: f.PublicID, Kind: "feature", Name: f.Name, Description: f.Description,
			URL: "/ui/f/" + path + "/" + f.Slug, State: string(f.State), Size: r,
		})
	}

	docs, err := store.DocumentsForOwner(ctx, s.Store.Pool, "initiative", &in.ID)
	if err != nil {
		return nil, err
	}
	page.Documents = s.documentCards(docs, primaryDocID(page.BodyDoc))

	if page.OwnedMilestones, err = s.ownedMilestoneCards(ctx, "initiative", &in.ID); err != nil {
		return nil, err
	}
	if page.OwnedRoadmaps, err = s.ownedRoadmapCards(ctx, "initiative", &in.ID); err != nil {
		return nil, err
	}
	if page.OwnedChecklists, err = s.ownedChecklistCards(ctx, "initiative", &in.ID); err != nil {
		return nil, err
	}
	if page.MemberOf, err = s.memberOfCards(ctx, "initiative", in.ID); err != nil {
		return nil, err
	}
	if page.MilestoneChoices, err = s.milestoneChoices(ctx, "initiative", in.ID, page.MemberOf); err != nil {
		return nil, err
	}
	if page.Activity, err = s.Store.AuditTail(ctx, "initiative", &in.ID, 10); err != nil {
		return nil, err
	}
	if err := s.bugPageParts(ctx, page); err != nil {
		return nil, err
	}
	return page, nil
}

func (s *Server) featurePage(ctx context.Context, f *store.Feature, notice, errMsg string) (*entityPage, error) {
	path, err := s.featurePath(ctx, f)
	if err != nil {
		return nil, err
	}
	crumbs, err := s.initiativeCrumbs(ctx, f.InitiativeID)
	if err != nil {
		return nil, err
	}
	// The URL is kept on the current crumb so the navigation tree can locate the
	// page; the template renders a `Here` crumb unlinked regardless.
	crumbs = append(crumbs, crumb{ID: f.PublicID, Label: f.Name, URL: "/ui/f/" + path, Kind: "feature", Here: true})
	page := &entityPage{
		Kind: "feature", RefType: "feature", ID: f.ID, PublicID: f.PublicID, Title: f.Name,
		Path: path, Slug: f.Slug, Description: f.Description, Editable: true,
		Breadcrumbs: crumbs, State: string(f.State), Notice: notice, Error: errMsg,
	}
	page.IsTerminal = f.State == "done" || f.State == "abandoned"
	page.CanAbandon = !page.IsTerminal
	page.CanStart = f.State == "ready"
	if !page.CanStart && !page.IsTerminal {
		// Read the contract the same way evaluateContractGate does, so the
		// page never disagrees with the gate it is explaining.
		specApproved := s.currentDocApproved(ctx, "spec", f.ID)
		devPlanApproved := s.currentDocApproved(ctx, "dev_plan", f.ID)
		page.StartReason = featureStartReason(string(f.State), specApproved, devPlanApproved)
	}

	page.Send = s.sendCardFor(ctx, f)
	page.ShowStartCard = !page.CanStart && !page.IsTerminal && (f.State != lifecycle.FeatIdea || page.Send.Sent)

	body, bodyDoc, ok := s.bodyFor(ctx, "feature", &f.ID)
	page.HasBody, page.Body, page.BodyDoc = ok, body, bodyDoc

	if page.Size, err = s.featureRollup(ctx, f.ID); err != nil {
		return nil, err
	}
	page.Done, page.DoneUnmeasured, _ = store.ActualTokens(ctx, s.Store.Pool, "feature", f.ID)

	docs, err := store.DocumentsForOwner(ctx, s.Store.Pool, "feature", &f.ID)
	if err != nil {
		return nil, err
	}
	page.Documents = s.documentCards(docs, primaryDocID(page.BodyDoc))
	if page.MemberOf, err = s.memberOfCards(ctx, "feature", f.ID); err != nil {
		return nil, err
	}
	if page.MilestoneChoices, err = s.milestoneChoices(ctx, "feature", f.ID, page.MemberOf); err != nil {
		return nil, err
	}
	if page.Timeline, err = s.featureTimeline(ctx, f.ID); err != nil {
		return nil, err
	}
	if page.Tasks, page.ClaimWaiting, err = s.featureTaskRows(ctx, f.ID); err != nil {
		return nil, err
	}
	if err := s.bugPageParts(ctx, page); err != nil {
		return nil, err
	}
	return page, nil
}

// featureStartReason explains, in plain words, why a feature cannot be started
// yet given its state (FR-2.2).
// currentDocApproved reports whether a feature's current document of a type is
// approved, matching evaluateContractGate's reading: a missing document is not
// an error, it is simply not approved.
func (s *Server) currentDocApproved(ctx context.Context, docType string, featureID uuid.UUID) bool {
	docType = s.contractDocType(ctx, featureID, docType) // a bug's spec is its report
	d, err := store.CurrentDocForOwner(ctx, s.Store.Pool, docType, "feature", featureID)
	return err == nil && d.State == lifecycle.DocApproved
}

// featureStartReason explains, in the words a person needs, why "Start
// building" is unavailable. For a feature still forming that means naming the half of
// the contract that is actually missing: saying "once the specification is
// approved" to someone looking at an approved specification tells them to do
// something they have already done, and leaves them stuck.
func featureStartReason(state string, specApproved, devPlanApproved bool) string {
	switch state {
	case "idea":
		switch {
		case !specApproved && !devPlanApproved:
			return "Building can start once this feature has an approved specification and an approved dev-plan. Neither is approved yet, so it is still an idea."
		case !specApproved:
			return "Building can start once this feature's specification is approved. Its dev-plan is approved; the specification is not, so it is still an idea."
		case !devPlanApproved:
			return "Building can start once this feature's dev-plan is approved. Its specification is approved, but a dev-plan decomposes that specification into the tasks agents build, and this feature does not have an approved one yet."
		}
		// Both halves approved but still an idea: the gate has not been
		// re-evaluated yet, and the heartbeat will pick it up.
		return "This feature's specification and dev-plan are both approved. It is a moment away from being ready to start."
	case "active":
		return "This feature is already in active development."
	case "review":
		return "This feature is in review; its work is complete and awaiting verification."
	default:
		return "This feature is not in the ready state, so building can't start."
	}
}

func primaryDocID(d *docCard) uuid.UUID {
	if d == nil {
		return uuid.Nil
	}
	return d.ID
}

// notFoundOrErr renders a not-found page for store.ErrNotFound and a 500
// otherwise, for the plain read-only pages.
func (s *Server) notFoundOrErr(w http.ResponseWriter, r *http.Request, kind, ref string, err error) {
	if errors.Is(err, store.ErrNotFound) {
		s.uiNotFound(w, r, kind, ref)
		return
	}
	s.uiError(w, err)
}
