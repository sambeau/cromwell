package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/lifecycle"
	"cromwell/internal/store"
)

// Milestones and roadmaps you can edit (SPEC-010). Creating happens on the
// owner's page; composing, ordering and locking happen in an editor loaded as
// a fragment into a native <dialog> (DESIGN-008 §5.1a, SPEC-010 FR-6).
//
// Every form here posts to a /ui/* route that calls the same audited store
// method the API and the MCP facet call, with every entity carried by id —
// never a typed path (NFR-4). An editor form posts with HTMX and gets the
// editor back, so the dialog stays open; the same route answers a plain post
// by re-rendering the page it came from (SD-9).

// memberPickerLimit caps the milestone-side picker; beyond it, search.
const memberPickerLimit = 50

// --- View models ---

// milestoneEditor is the milestone's modal: its checklist of members with
// removal, the picker to add more, and the lock (FR-3.2, FR-5).
type milestoneEditor struct {
	Milestone  store.Milestone
	Card       milestoneCard
	Owner      crumb
	Members    []memberRow
	Locked     bool
	CanLock    bool
	LockReason string // plain words when G4 would refuse
	LeafCount  int
	LeafDone   int
	Picker     memberPicker
	Notice     string
	Error      string
	Changed    bool // something was changed in this sitting; closing reloads the page
}

// memberPicker is the milestone-side add list (FR-3.2, FR-3.3).
type memberPicker struct {
	MilestoneID uuid.UUID
	Query       string
	ScopeLabel  string
	Candidates  []store.MemberCandidate
	More        bool
}

// milestoneChoice and milestoneChoiceGroup are the member-side "Add to a
// milestone" select, grouped by where each milestone is planned (FR-3.1).
type milestoneChoice struct {
	ID   uuid.UUID
	Name string
}

type milestoneChoiceGroup struct {
	Label   string
	Options []milestoneChoice
}

// roadmapEditor is the roadmap's modal: the ordered list with move and take
// off, and the place-a-milestone form (FR-4).
type roadmapEditor struct {
	Roadmap   store.Roadmap
	Owner     crumb
	Entries   []roadmapEntryRow
	Here      []milestoneChoice // not on the roadmap, planned by the same owner
	Elsewhere []milestoneChoice // not on the roadmap, planned anywhere else
	Notice    string
	Error     string
	Changed   bool
}

type roadmapEntryRow struct {
	Place int // 1-based
	Card  milestoneCard
	First bool
	Last  bool
}

// membershipView is what the member-side "Add to a milestone…" dialog needs:
// the member, the milestones it is directly in, and the ones it could join.
type membershipView struct {
	Kind     string
	ID       uuid.UUID
	Title    string
	MemberOf []milestoneCard
	Choices  []milestoneChoiceGroup
}

func (p *entityPage) Membership() membershipView {
	return membershipView{Kind: p.Kind, ID: p.ID, Title: p.Title, MemberOf: p.MemberOf, Choices: p.MilestoneChoices}
}

func (p milestonePage) Membership() membershipView {
	return membershipView{Kind: "milestone", ID: p.Milestone.ID, Title: p.Milestone.Name,
		MemberOf: p.MemberOf, Choices: p.MilestoneChoices}
}

// isEditorPost reports whether a request came from an editor form inside the
// modal: an explicit HTMX request that is not a boosted navigation. The body
// is hx-boost'ed, so a plain form post also carries HX-Request; HX-Boosted
// tells them apart.
func isEditorPost(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") == ""
}

// --- Building the editors ---

func (s *Server) milestoneEditorFor(ctx context.Context, id uuid.UUID, query, notice, errMsg string, changed bool) (*milestoneEditor, error) {
	m, err := store.GetMilestone(ctx, s.Store.Pool, id)
	if err != nil {
		return nil, err
	}
	card, err := s.milestoneCardFor(ctx, *m)
	if err != nil {
		return nil, err
	}
	rows, err := s.milestoneMemberRows(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	ed := &milestoneEditor{
		Milestone: *m, Card: card, Owner: s.ownerCrumb(ctx, m.OwnerType, m.OwnerID),
		Members: rows, Locked: m.State != lifecycle.MilestoneOpen,
		Notice: notice, Error: errMsg, Changed: changed,
	}
	if ed.Locked {
		return ed, nil
	}
	// Preview G4 exactly as LockMilestone will evaluate it, so a lock the gate
	// would refuse is shown disabled with its reason (DESIGN-008 §5.2).
	prog, err := store.LiveProgress(ctx, s.Store.Pool, m.ID)
	if err != nil {
		return nil, err
	}
	g := lifecycle.G4(prog.Total, prog.Done)
	ed.CanLock, ed.LeafCount, ed.LeafDone = g.Pass, prog.Total, prog.Done
	if !g.Pass {
		ed.LockReason = g4Plain(prog.Total, prog.Done)
	}
	if ed.Picker, err = s.memberPickerFor(ctx, m, query); err != nil {
		return nil, err
	}
	return ed, nil
}

func (s *Server) memberPickerFor(ctx context.Context, m *store.Milestone, query string) (memberPicker, error) {
	var scope *uuid.UUID
	label := "the whole project"
	if m.OwnerType == "initiative" && m.OwnerID != nil {
		scope = m.OwnerID
		if in, err := store.GetInitiative(ctx, s.Store.Pool, *m.OwnerID); err == nil {
			label = in.Name + " and everything inside it"
		}
	}
	cands, more, err := store.MemberCandidates(ctx, s.Store.Pool, m.ID, scope, query, memberPickerLimit)
	if err != nil {
		return memberPicker{}, err
	}
	return memberPicker{MilestoneID: m.ID, Query: strings.TrimSpace(query), ScopeLabel: label,
		Candidates: cands, More: more}, nil
}

func (s *Server) roadmapEditorFor(ctx context.Context, id uuid.UUID, notice, errMsg string, changed bool) (*roadmapEditor, error) {
	rm, err := store.GetRoadmap(ctx, s.Store.Pool, id)
	if err != nil {
		return nil, err
	}
	entries, err := store.RoadmapEntries(ctx, s.Store.Pool, rm.ID)
	if err != nil {
		return nil, err
	}
	ed := &roadmapEditor{Roadmap: *rm, Owner: s.ownerCrumb(ctx, rm.OwnerType, rm.OwnerID),
		Notice: notice, Error: errMsg, Changed: changed}
	placed := map[uuid.UUID]bool{}
	for i, e := range entries {
		m, err := store.GetMilestone(ctx, s.Store.Pool, e.MilestoneID)
		if err != nil {
			return nil, err
		}
		card, err := s.milestoneCardFor(ctx, *m)
		if err != nil {
			return nil, err
		}
		placed[m.ID] = true
		ed.Entries = append(ed.Entries, roadmapEntryRow{Place: i + 1, Card: card,
			First: i == 0, Last: i == len(entries)-1})
	}
	all, err := store.ListMilestones(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	for _, m := range all {
		if placed[m.ID] {
			continue
		}
		c := milestoneChoice{ID: m.ID, Name: m.Name}
		if m.OwnerType == rm.OwnerType && uuidPtrEqual(m.OwnerID, rm.OwnerID) {
			ed.Here = append(ed.Here, c)
		} else {
			ed.Elsewhere = append(ed.Elsewhere, c)
		}
	}
	sortChoices(ed.Here)
	sortChoices(ed.Elsewhere)
	return ed, nil
}

// milestoneChoices lists the open milestones a thing could join, grouped by
// where each is planned (FR-3.1). It leaves out locked milestones, the ones it
// is already directly in, and — for a milestone — itself and any milestone it
// contains, which would make a loop (FR-1.5).
func (s *Server) milestoneChoices(ctx context.Context, memberType string, memberID uuid.UUID, memberOf []milestoneCard) ([]milestoneChoiceGroup, error) {
	all, err := store.ListMilestones(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	already := map[uuid.UUID]bool{}
	for _, c := range memberOf {
		already[c.ID] = true
	}
	type key struct {
		t  string
		id uuid.UUID
	}
	groups := map[key]*milestoneChoiceGroup{}
	var order []key
	for _, m := range all {
		if m.State != lifecycle.MilestoneOpen || already[m.ID] {
			continue
		}
		if memberType == "milestone" {
			if m.ID == memberID {
				continue
			}
			inside, err := store.MilestoneContains(ctx, s.Store.Pool, memberID, m.ID)
			if err != nil {
				return nil, err
			}
			if inside {
				continue
			}
		}
		k := key{m.OwnerType, deref(m.OwnerID)}
		g, ok := groups[k]
		if !ok {
			g = &milestoneChoiceGroup{Label: "Planned in " + s.ownerCrumb(ctx, m.OwnerType, m.OwnerID).Label}
			groups[k] = g
			order = append(order, k)
		}
		g.Options = append(g.Options, milestoneChoice{ID: m.ID, Name: m.Name})
	}
	// The project's own plan first, then the initiatives' by name.
	sort.SliceStable(order, func(i, j int) bool {
		if (order[i].t == "project") != (order[j].t == "project") {
			return order[i].t == "project"
		}
		return groups[order[i]].Label < groups[order[j]].Label
	})
	out := make([]milestoneChoiceGroup, 0, len(order))
	for _, k := range order {
		sortChoices(groups[k].Options)
		out = append(out, *groups[k])
	}
	return out, nil
}

func sortChoices(cs []milestoneChoice) {
	sort.SliceStable(cs, func(i, j int) bool { return strings.ToLower(cs[i].Name) < strings.ToLower(cs[j].Name) })
}

func uuidPtrEqual(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// g4Plain says in plain words why G4 would refuse a lock (FR-5.1). The gate's
// own reason is written for the audit trail; this is written for the person
// looking at the disabled button.
func g4Plain(total, done int) string {
	switch {
	case total == 0:
		return "This milestone can't be locked yet, because nothing in it comes down to a feature. " +
			"Add the work it is meant to deliver first."
	case done == 0 && total == 1:
		return "This milestone can't be locked yet, because its one feature isn't done. " +
			"Locking records what actually shipped, so at least one feature has to be finished first."
	case done == 0:
		return fmt.Sprintf("This milestone can't be locked yet, because none of its %d features is done. "+
			"Locking records what actually shipped, so at least one has to be finished first.", total)
	}
	return ""
}

// planError turns a store error into the sentence a person reads (D-6).
func planError(err error) string {
	switch {
	case errors.Is(err, store.ErrMilestoneCycle):
		return "A milestone can't contain itself, directly or through another milestone inside it."
	case errors.Is(err, store.ErrNotFound):
		return "That is no longer there. The page may be out of date, so reload it and try again."
	case strings.Contains(err.Error(), "membership is frozen"):
		return "This milestone is locked, so what it contains is fixed."
	case strings.Contains(err.Error(), "already locked"):
		return "This milestone is already locked."
	}
	return err.Error()
}

// --- Fragment routes (FR-6.1) ---

func (s *Server) handleUIMilestoneEdit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "milestone", r.PathValue("id"))
		return
	}
	ed, err := s.milestoneEditorFor(r.Context(), id, r.URL.Query().Get("q"), "", "", false)
	if err != nil {
		s.notFoundOrErr(w, r, "milestone", id.String(), err)
		return
	}
	s.render(w, "milestone-editor-dialog", ed)
}

func (s *Server) handleUIMilestoneCandidates(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "milestone", r.PathValue("id"))
		return
	}
	ctx := r.Context()
	m, err := store.GetMilestone(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "milestone", id.String(), err)
		return
	}
	picker, err := s.memberPickerFor(ctx, m, r.URL.Query().Get("q"))
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "member-candidates", picker)
}

func (s *Server) handleUIRoadmapEdit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "roadmap", r.PathValue("id"))
		return
	}
	ed, err := s.roadmapEditorFor(r.Context(), id, "", "", false)
	if err != nil {
		s.notFoundOrErr(w, r, "roadmap", id.String(), err)
		return
	}
	s.render(w, "roadmap-editor-dialog", ed)
}

// --- Responses ---

// respondMilestone answers a milestone mutation: the editor fragment for a
// post from the modal, the member's page for a post from the member side, and
// otherwise the owner's page (SD-9).
func (s *Server) respondMilestone(w http.ResponseWriter, r *http.Request, milestoneID uuid.UUID, notice, errMsg string) {
	ctx := r.Context()
	if r.FormValue("from") == "member" {
		memberType := strings.TrimSpace(r.FormValue("member_type"))
		memberID, err := uuid.Parse(strings.TrimSpace(r.FormValue("member_id")))
		if err != nil {
			http.Error(w, "bad member id", http.StatusBadRequest)
			return
		}
		s.renderEntity(w, r, memberType, memberID, notice, errMsg)
		return
	}
	if isEditorPost(r) {
		ed, err := s.milestoneEditorFor(ctx, milestoneID, "", notice, errMsg, errMsg == "")
		if err != nil {
			s.notFoundOrErr(w, r, "milestone", milestoneID.String(), err)
			return
		}
		s.render(w, "milestone-editor", ed)
		return
	}
	m, err := store.GetMilestone(ctx, s.Store.Pool, milestoneID)
	if err != nil {
		s.notFoundOrErr(w, r, "milestone", milestoneID.String(), err)
		return
	}
	s.renderEntity(w, r, m.OwnerType, deref(m.OwnerID), notice, errMsg)
}

func (s *Server) respondRoadmap(w http.ResponseWriter, r *http.Request, roadmapID uuid.UUID, notice, errMsg string) {
	ctx := r.Context()
	if isEditorPost(r) {
		ed, err := s.roadmapEditorFor(ctx, roadmapID, notice, errMsg, errMsg == "")
		if err != nil {
			s.notFoundOrErr(w, r, "roadmap", roadmapID.String(), err)
			return
		}
		s.render(w, "roadmap-editor", ed)
		return
	}
	rm, err := store.GetRoadmap(ctx, s.Store.Pool, roadmapID)
	if err != nil {
		s.notFoundOrErr(w, r, "roadmap", roadmapID.String(), err)
		return
	}
	s.renderEntity(w, r, rm.OwnerType, deref(rm.OwnerID), notice, errMsg)
}

// planOwnerFromForm reads the owner a new milestone or roadmap belongs to from
// the page's hidden fields: the project, or an initiative by id.
func planOwnerFromForm(r *http.Request) (string, *uuid.UUID, error) {
	ownerType := strings.TrimSpace(r.FormValue("owner_type"))
	switch ownerType {
	case "project":
		return "project", nil, nil
	case "initiative":
		id, err := formID(r)
		if err != nil {
			return "", nil, err
		}
		return "initiative", &id, nil
	}
	return "", nil, fmt.Errorf("unknown owner kind %q", ownerType)
}

// --- Create (FR-2) ---

func (s *Server) handleMilestoneCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ownerType, ownerID, err := planOwnerFromForm(r)
	if err != nil {
		http.Error(w, "bad owner", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.renderEntity(w, r, ownerType, deref(ownerID), "", "A milestone needs a name, so give it one before creating it.")
		return
	}
	var target *time.Time
	if td := strings.TrimSpace(r.FormValue("target_date")); td != "" {
		t, err := time.Parse("2006-01-02", td)
		if err != nil {
			s.renderEntity(w, r, ownerType, deref(ownerID), "", "The target date must be a date such as 2026-12-31.")
			return
		}
		target = &t
	}
	description := strings.TrimSpace(r.FormValue("description"))
	err = s.Store.WithTx(r.Context(), func(tx pgx.Tx) error {
		_, e := store.CreateMilestone(r.Context(), tx, ownerType, ownerID, name, description, target, s.uiActor())
		return e
	})
	if err != nil {
		s.renderEntity(w, r, ownerType, deref(ownerID), "", planError(err))
		return
	}
	s.renderEntity(w, r, ownerType, deref(ownerID),
		"Milestone created: "+name+". Choose Edit beside it to add the work it will deliver.", "")
}

func (s *Server) handleRoadmapCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ownerType, ownerID, err := planOwnerFromForm(r)
	if err != nil {
		http.Error(w, "bad owner", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.renderEntity(w, r, ownerType, deref(ownerID), "", "A roadmap needs a name, so give it one before creating it.")
		return
	}
	err = s.Store.WithTx(r.Context(), func(tx pgx.Tx) error {
		_, e := store.CreateRoadmap(r.Context(), tx, ownerType, ownerID, name, s.uiActor())
		return e
	})
	if err != nil {
		s.renderEntity(w, r, ownerType, deref(ownerID), "", planError(err))
		return
	}
	s.renderEntity(w, r, ownerType, deref(ownerID),
		"Roadmap created: "+name+". Choose Edit beside it to place milestones on it in order.", "")
}

// --- Membership (FR-3) ---

// memberFromForm reads and checks the member a form names, returning its name
// for the notice.
func (s *Server) memberFromForm(ctx context.Context, r *http.Request) (string, uuid.UUID, string, error) {
	memberType := strings.TrimSpace(r.FormValue("member_type"))
	memberID, err := uuid.Parse(strings.TrimSpace(r.FormValue("member_id")))
	if err != nil {
		return "", uuid.Nil, "", errors.New("Choose something to add first.")
	}
	switch memberType {
	case "feature":
		f, err := store.GetFeature(ctx, s.Store.Pool, memberID)
		if err != nil {
			return "", uuid.Nil, "", err
		}
		return memberType, memberID, f.Name, nil
	case "initiative":
		in, err := store.GetInitiative(ctx, s.Store.Pool, memberID)
		if err != nil {
			return "", uuid.Nil, "", err
		}
		return memberType, memberID, in.Name, nil
	case "milestone":
		m, err := store.GetMilestone(ctx, s.Store.Pool, memberID)
		if err != nil {
			return "", uuid.Nil, "", err
		}
		return memberType, memberID, m.Name, nil
	}
	return "", uuid.Nil, "", fmt.Errorf("a milestone can hold features, initiatives and other milestones, not %q", memberType)
}

func (s *Server) handleMilestoneMemberAdd(w http.ResponseWriter, r *http.Request) {
	s.milestoneMemberChange(w, r, true)
}

func (s *Server) handleMilestoneMemberRemove(w http.ResponseWriter, r *http.Request) {
	s.milestoneMemberChange(w, r, false)
}

func (s *Server) milestoneMemberChange(w http.ResponseWriter, r *http.Request, add bool) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	milestoneID, err := uuid.Parse(strings.TrimSpace(r.FormValue("milestone_id")))
	if err != nil {
		// The member-side select starts on a placeholder with no value.
		if r.FormValue("from") == "member" {
			s.respondMilestone(w, r, uuid.Nil, "", "Choose a milestone to add this to.")
			return
		}
		http.Error(w, "bad milestone id", http.StatusBadRequest)
		return
	}
	m, err := store.GetMilestone(ctx, s.Store.Pool, milestoneID)
	if err != nil {
		s.notFoundOrErr(w, r, "milestone", milestoneID.String(), err)
		return
	}
	memberType, memberID, memberName, err := s.memberFromForm(ctx, r)
	if err != nil {
		s.respondMilestone(w, r, milestoneID, "", planError(err))
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if add {
			return store.AddMember(ctx, tx, milestoneID, memberType, memberID, s.uiActor())
		}
		return store.RemoveMember(ctx, tx, milestoneID, memberType, memberID, reason, s.uiActor())
	})
	if err != nil {
		s.respondMilestone(w, r, milestoneID, "", planError(err))
		return
	}
	notice := memberName + " was added to " + m.Name + "."
	if !add {
		notice = memberName + " was taken out of " + m.Name + "."
	}
	s.respondMilestone(w, r, milestoneID, notice, "")
}

// --- Lock (FR-5) ---

func (s *Server) handleMilestoneLock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	milestoneID, err := uuid.Parse(strings.TrimSpace(r.FormValue("milestone_id")))
	if err != nil {
		http.Error(w, "bad milestone id", http.StatusBadRequest)
		return
	}
	// The confirm step is part of the form (FR-5.2): a post without it — a
	// stray or scripted one — locks nothing.
	if r.FormValue("confirm") != "permanent" {
		s.respondMilestone(w, r, milestoneID, "",
			"Locking is permanent, so it needs the confirmation. Open Lock this milestone and confirm it there.")
		return
	}
	var g lifecycle.GateResult
	var prog store.Progress
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		p, e := store.LiveProgress(ctx, tx, milestoneID)
		if e != nil {
			return e
		}
		prog = p
		g, e = store.LockMilestone(ctx, tx, milestoneID, s.uiActor())
		return e
	})
	if err != nil {
		// G4 refused: its reason inline, nothing locked, no checkpoint and no
		// force path (L-6, SPEC-006 R6-1).
		if g.Gate == lifecycle.GateG4 && !g.Pass {
			s.respondMilestone(w, r, milestoneID, "", g4Plain(prog.Total, prog.Done))
			return
		}
		s.respondMilestone(w, r, milestoneID, "", planError(err))
		return
	}
	s.respondMilestone(w, r, milestoneID, fmt.Sprintf(
		"This milestone is now locked. Its snapshot holds %d features, %d of them done, and what it contains can no longer change.",
		prog.Total, prog.Done), "")
}

// --- Roadmap order (FR-4) ---

func (s *Server) handleRoadmapEntryPlace(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	roadmapID, err := uuid.Parse(strings.TrimSpace(r.FormValue("roadmap_id")))
	if err != nil {
		http.Error(w, "bad roadmap id", http.StatusBadRequest)
		return
	}
	milestoneID, err := uuid.Parse(strings.TrimSpace(r.FormValue("milestone_id")))
	if err != nil {
		s.respondRoadmap(w, r, roadmapID, "", "Choose a milestone to place on the roadmap.")
		return
	}
	place, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("place"))) // blank means the end
	m, err := store.GetMilestone(ctx, s.Store.Pool, milestoneID)
	if err != nil {
		s.respondRoadmap(w, r, roadmapID, "", planError(err))
		return
	}
	var final int
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		final, e = store.PlaceRoadmapEntry(ctx, tx, roadmapID, milestoneID, place, s.uiActor())
		return e
	})
	if err != nil {
		s.respondRoadmap(w, r, roadmapID, "", planError(err))
		return
	}
	s.respondRoadmap(w, r, roadmapID, fmt.Sprintf("%s is now number %d on the roadmap.", m.Name, final), "")
}

func (s *Server) handleRoadmapEntryRemove(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	roadmapID, err := uuid.Parse(strings.TrimSpace(r.FormValue("roadmap_id")))
	if err != nil {
		http.Error(w, "bad roadmap id", http.StatusBadRequest)
		return
	}
	milestoneID, err := uuid.Parse(strings.TrimSpace(r.FormValue("milestone_id")))
	if err != nil {
		http.Error(w, "bad milestone id", http.StatusBadRequest)
		return
	}
	name := "The milestone"
	if m, err := store.GetMilestone(ctx, s.Store.Pool, milestoneID); err == nil {
		name = m.Name
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.RemoveRoadmapEntry(ctx, tx, roadmapID, milestoneID, s.uiActor())
	})
	if err != nil {
		s.respondRoadmap(w, r, roadmapID, "", planError(err))
		return
	}
	s.respondRoadmap(w, r, roadmapID, name+" was taken off the roadmap. The milestone itself is unchanged.", "")
}
