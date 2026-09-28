package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/store"
)

// Checklists and jobs in the web UI (SPEC-014 FR-3 to FR-5). A job is ticked
// on the checklist page, as plain checkboxes (DESIGN-010 §6, D-10). Adding,
// changing, reordering and removing jobs happen in the checklist editor, a
// modal loaded by HTMX like the milestone editor, opened from the owner's page
// or the checklist's own page (SD-5).
//
// Every form posts to a /ui/* route that calls the same audited store method
// the chat tools call, with every entity carried by id (NFR-4). A post from
// inside the editor gets the editor back; any other post gets the checklist
// page (SPEC-010 SD-9).

// --- View models ---

// checklistCard is a checklist summarised for an owner's plan section: how
// many of its jobs are ticked, and whether it is done (SD-9).
type checklistCard struct {
	ID       uuid.UUID
	PublicID string // "CL-002" (SPEC-017 FR-3.3)
	Name     string
	URL      string
	Jobs     int
	Ticked   int
	Done     bool
}

// Status is the card's count in words, shared by every place a checklist is
// summarised.
func (c checklistCard) Status() string { return checklistStatusText(c.Jobs, c.Ticked) }

func checklistStatusText(jobs, ticked int) string {
	switch {
	case jobs == 0:
		return "No jobs yet"
	case ticked == jobs:
		return "Done: every job is ticked"
	case jobs == 1:
		return "Its one job isn't ticked yet"
	}
	return fmt.Sprintf("%d of %d jobs ticked", ticked, jobs)
}

// jobRow is one job as the page and the editor show it.
type jobRow struct {
	ID       uuid.UUID
	Place    int
	Title    string
	Note     string
	Ticked   bool
	TickedBy string
	TickedAt time.Time
	Relayed  bool   // ticked by the chat agent on a person's word
	Quote    string // the person's words, for a relayed tick
	First    bool
	Last     bool
}

func jobRows(jobs []store.Job) []jobRow {
	out := make([]jobRow, len(jobs))
	for i, j := range jobs {
		row := jobRow{ID: j.ID, Place: i + 1, Title: j.Title, Note: j.Note, Ticked: j.Ticked(),
			First: i == 0, Last: i == len(jobs)-1}
		if j.TickedBy != nil {
			row.TickedBy = *j.TickedBy
		}
		if j.TickedAt != nil {
			row.TickedAt = *j.TickedAt
		}
		row.Relayed = j.TickedVia != nil && *j.TickedVia == "mcp"
		if j.TickedQuote != nil {
			row.Quote = *j.TickedQuote
		}
		out[i] = row
	}
	return out
}

// checklistPage is /ui/c/{id}: the jobs as checkboxes (FR-4).
type checklistPage struct {
	Checklist store.Checklist
	Card      checklistCard
	Owner     crumb
	Jobs      []jobRow
	// The milestones it is directly in, and the open ones it could join, for
	// "Add to a milestone…" (FR-4.4).
	MemberOf         []milestoneCard
	MilestoneChoices []milestoneChoiceGroup
	Notice           string
	Error            string
}

func (p checklistPage) Membership() membershipView {
	return membershipView{Kind: "checklist", ID: p.Checklist.ID, Title: p.Checklist.Name,
		MemberOf: p.MemberOf, Choices: p.MilestoneChoices}
}

func (p checklistPage) headTitle() string { return p.Checklist.Name }
func (p checklistPage) headCrumbs() []crumb {
	return []crumb{p.Owner, {Label: p.Checklist.Name, Kind: "checklist", Here: true}}
}

// checklistEditor is the checklist's modal (FR-5).
type checklistEditor struct {
	Checklist store.Checklist
	Card      checklistCard
	Owner     crumb
	Jobs      []jobRow
	Notice    string
	Error     string
	Changed   bool // something changed in this sitting; closing reloads the page
}

func checklistURL(id uuid.UUID) string { return "/ui/c/" + id.String() }

// --- Building them ---

func (s *Server) checklistCardFor(ctx context.Context, c store.Checklist) (checklistCard, error) {
	st, err := store.GetChecklistStatus(ctx, s.Store.Pool, c.ID)
	if err != nil {
		return checklistCard{}, err
	}
	return checklistCard{ID: c.ID, PublicID: c.PublicID, Name: c.Name, URL: checklistURL(c.ID),
		Jobs: st.Jobs, Ticked: st.Ticked, Done: st.Done()}, nil
}

func (s *Server) ownedChecklistCards(ctx context.Context, ownerType string, ownerID *uuid.UUID) ([]checklistCard, error) {
	cls, err := store.ChecklistsOwnedBy(ctx, s.Store.Pool, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]checklistCard, 0, len(cls))
	for _, c := range cls {
		card, err := s.checklistCardFor(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, nil
}

func (s *Server) checklistPageFor(ctx context.Context, id uuid.UUID, notice, errMsg string) (*checklistPage, error) {
	c, err := store.GetChecklist(ctx, s.Store.Pool, id)
	if err != nil {
		return nil, err
	}
	card, err := s.checklistCardFor(ctx, *c)
	if err != nil {
		return nil, err
	}
	jobs, err := store.Jobs(ctx, s.Store.Pool, c.ID)
	if err != nil {
		return nil, err
	}
	memberOf, err := s.memberOfCards(ctx, "checklist", c.ID)
	if err != nil {
		return nil, err
	}
	choices, err := s.milestoneChoices(ctx, "checklist", c.ID, memberOf)
	if err != nil {
		return nil, err
	}
	return &checklistPage{
		Checklist: *c, Card: card, Owner: s.ownerCrumb(ctx, c.OwnerType, c.OwnerID), Jobs: jobRows(jobs),
		MemberOf: memberOf, MilestoneChoices: choices, Notice: notice, Error: errMsg,
	}, nil
}

func (s *Server) checklistEditorFor(ctx context.Context, id uuid.UUID, notice, errMsg string, changed bool) (*checklistEditor, error) {
	c, err := store.GetChecklist(ctx, s.Store.Pool, id)
	if err != nil {
		return nil, err
	}
	card, err := s.checklistCardFor(ctx, *c)
	if err != nil {
		return nil, err
	}
	jobs, err := store.Jobs(ctx, s.Store.Pool, c.ID)
	if err != nil {
		return nil, err
	}
	return &checklistEditor{Checklist: *c, Card: card, Owner: s.ownerCrumb(ctx, c.OwnerType, c.OwnerID),
		Jobs: jobRows(jobs), Notice: notice, Error: errMsg, Changed: changed}, nil
}

// checklistError turns a store refusal into the sentence a person reads (D-6).
func checklistError(err error) string {
	switch {
	case errors.Is(err, store.ErrJobAlreadyTicked):
		return "That job is already ticked."
	case errors.Is(err, store.ErrJobNotTicked):
		return "That job isn't ticked."
	case errors.Is(err, store.ErrJobTitleBlank):
		return "A job needs a title, such as “Get the API key”."
	case errors.Is(err, store.ErrJobTickedRename):
		return "A ticked job's title can't change, because the tick is for the job as it was. Untick it first, or add a new job."
	case errors.Is(err, errJobGone):
		return "That job is no longer on this checklist, so nothing was changed. The page has been brought up to date."
	}
	return planError(err)
}

// --- Pages and fragments ---

func (s *Server) handleUIChecklistPage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "checklist", r.PathValue("id"))
		return
	}
	s.renderChecklistPage(w, r, id, "", "")
}

func (s *Server) renderChecklistPage(w http.ResponseWriter, r *http.Request, id uuid.UUID, notice, errMsg string) {
	page, err := s.checklistPageFor(r.Context(), id, notice, errMsg)
	if err != nil {
		s.notFoundOrErr(w, r, "checklist", id.String(), err)
		return
	}
	pushPageURL(w, r, checklistURL(id))
	s.render(w, "page-checklist", s.page(r.Context(), "browse", *page))
}

func (s *Server) handleUIChecklistEdit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "checklist", r.PathValue("id"))
		return
	}
	ed, err := s.checklistEditorFor(r.Context(), id, "", "", false)
	if err != nil {
		s.notFoundOrErr(w, r, "checklist", id.String(), err)
		return
	}
	s.render(w, "checklist-editor-dialog", ed)
}

// respondChecklist answers a job change: the editor for a post from the modal,
// the checklist page for anything else (SPEC-010 SD-9).
func (s *Server) respondChecklist(w http.ResponseWriter, r *http.Request, checklistID uuid.UUID, notice, errMsg string) {
	if isEditorPost(r) {
		ed, err := s.checklistEditorFor(r.Context(), checklistID, notice, errMsg, errMsg == "")
		if err != nil {
			s.notFoundOrErr(w, r, "checklist", checklistID.String(), err)
			return
		}
		s.render(w, "checklist-editor", ed)
		return
	}
	s.renderChecklistPage(w, r, checklistID, notice, errMsg)
}

// --- Create (FR-3) ---

func (s *Server) handleChecklistCreate(w http.ResponseWriter, r *http.Request) {
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
		s.renderEntity(w, r, ownerType, deref(ownerID), "", "A checklist needs a name, so give it one before creating it.")
		return
	}
	description := strings.TrimSpace(r.FormValue("description"))
	err = s.Store.WithTx(r.Context(), func(tx pgx.Tx) error {
		_, e := store.CreateChecklist(r.Context(), tx, ownerType, ownerID, name, description, s.uiActor())
		return e
	})
	if err != nil {
		s.renderEntity(w, r, ownerType, deref(ownerID), "", planError(err))
		return
	}
	s.renderEntity(w, r, ownerType, deref(ownerID),
		"Checklist created: "+name+". Choose Edit beside it to add its jobs.", "")
}

// --- Jobs (FR-4, FR-5) ---

// errJobGone is a job form posted from a page that is out of date: the job
// was removed, perhaps by the chat agent, or isn't on the checklist the form
// came from (REVIEW-014 R14-8).
var errJobGone = errors.New("the job is no longer on this checklist")

// jobFromForm reads the job a form names, and checks it is on the checklist
// the form came from. Every job form carries both ids, so a job that has gone
// is answered on its checklist's page, brought up to date, with a sentence.
func (s *Server) jobFromForm(w http.ResponseWriter, r *http.Request) (*store.Job, bool) {
	checklistID, err := uuid.Parse(strings.TrimSpace(r.FormValue("checklist_id")))
	if err != nil {
		http.Error(w, "bad checklist id", http.StatusBadRequest)
		return nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(r.FormValue("job_id")))
	if err != nil {
		http.Error(w, "bad job id", http.StatusBadRequest)
		return nil, false
	}
	j, err := store.GetJob(r.Context(), s.Store.Pool, id)
	if err == nil && j.ChecklistID != checklistID {
		err = store.ErrNotFound
	}
	if errors.Is(err, store.ErrNotFound) {
		s.respondChecklist(w, r, checklistID, "", checklistError(errJobGone))
		return nil, false
	}
	if err != nil {
		s.uiError(w, err)
		return nil, false
	}
	return j, true
}

func (s *Server) handleJobTick(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	j, ok := s.jobFromForm(w, r)
	if !ok {
		return
	}
	tick, err := strconv.ParseBool(strings.TrimSpace(r.FormValue("tick")))
	if err != nil {
		http.Error(w, "bad tick", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	ctx := r.Context()
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.TickJob(ctx, tx, j.ID, tick, note, "ui", "", s.uiActor())
		return e
	})
	if err != nil {
		s.respondChecklist(w, r, j.ChecklistID, "", checklistError(err))
		return
	}
	notice := "Ticked: " + j.Title + "."
	if !tick {
		notice = "Unticked: " + j.Title + "."
	}
	s.respondChecklist(w, r, j.ChecklistID, notice, "")
}

func (s *Server) handleJobAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	checklistID, err := uuid.Parse(strings.TrimSpace(r.FormValue("checklist_id")))
	if err != nil {
		http.Error(w, "bad checklist id", http.StatusBadRequest)
		return
	}
	if _, err := store.GetChecklist(ctx, s.Store.Pool, checklistID); err != nil {
		s.notFoundOrErr(w, r, "checklist", checklistID.String(), err)
		return
	}
	var j *store.Job
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		j, e = store.AddJob(ctx, tx, checklistID, r.FormValue("title"), r.FormValue("note"), s.uiActor())
		return e
	})
	if err != nil {
		s.respondChecklist(w, r, checklistID, "", checklistError(err))
		return
	}
	s.respondChecklist(w, r, checklistID, "Added: "+j.Title+".", "")
}

func (s *Server) handleJobEdit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	j, ok := s.jobFromForm(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var edited *store.Job
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		edited, e = store.EditJob(ctx, tx, j.ID, r.FormValue("title"), r.FormValue("note"), s.uiActor())
		return e
	})
	if err != nil {
		s.respondChecklist(w, r, j.ChecklistID, "", checklistError(err))
		return
	}
	s.respondChecklist(w, r, j.ChecklistID, "Changed: "+edited.Title+".", "")
}

func (s *Server) handleJobMove(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	j, ok := s.jobFromForm(w, r)
	if !ok {
		return
	}
	place, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("place"))) // blank means the end
	ctx := r.Context()
	var final int
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		final, e = store.PlaceJob(ctx, tx, j.ID, place, s.uiActor())
		return e
	})
	if err != nil {
		s.respondChecklist(w, r, j.ChecklistID, "", checklistError(err))
		return
	}
	s.respondChecklist(w, r, j.ChecklistID, fmt.Sprintf("%s is now number %d on the checklist.", j.Title, final), "")
}

func (s *Server) handleJobRemove(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	j, ok := s.jobFromForm(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.RemoveJob(ctx, tx, j.ID, r.FormValue("reason"), s.uiActor())
		return e
	})
	if err != nil {
		s.respondChecklist(w, r, j.ChecklistID, "", checklistError(err))
		return
	}
	s.respondChecklist(w, r, j.ChecklistID, "Removed: "+j.Title+". The history still shows it.", "")
}
