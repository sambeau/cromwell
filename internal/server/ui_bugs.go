package server

// Bugs in the web UI (SPEC-019 FR-2.3 to FR-2.5, FR-3.1, FR-5.1, FR-5.2):
// the triage queue, the triage card on a bug's page, the bugs lists on
// initiative and feature pages, and the report dialog. Every POST calls a
// service method in bugs.go, the same one the MCP tools call.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"subutai/internal/store"
)

// bugCard is what a bug's page, and the queue, say about its triage.
type bugCard struct {
	ID          uuid.UUID
	PublicID    string
	Title       string
	URL         string
	Triage      string
	ReportedBy  string // "Reported by Sam in the web UI", in a sentence
	ReportedAt  time.Time
	HangsOn     string // the initiative, and the feature when there is one
	OriginName  string
	OriginURL   string
	Summary     string
	Expected    string
	Actual      string
	ReportURL   string
	DecidedWord string // "Accepted", "Rejected", "Marked as a duplicate"
	DecidedBy   string
	DecidedAt   *time.Time
	Reason      string
	Quote       string
	DupOf       string
	DupOfURL    string
	State       string
}

// bugRow is one line in a bugs list.
type bugRow struct {
	PublicID string
	Title    string
	URL      string
	Triage   string
	State    string
}

// reporterWords says who reported a bug and how, in a sentence (FR-2.3).
func (s *Server) reporterWords(ctx context.Context, b *store.Bug) string {
	switch b.ReporterKind {
	case store.ReporterChat:
		return "Reported by the chat agent."
	case store.ReporterAgent, store.ReporterReview:
		who := "the " + roleWords(b.ReportedBy)
		if b.ReportedDispatchID != nil {
			if d, err := store.GetDispatch(ctx, s.Store.Pool, *b.ReportedDispatchID); err == nil && d.Model != "" {
				who += " (" + d.Model + ")"
			}
		}
		on := ""
		if b.SourceTaskID != nil {
			if t, err := store.GetTask(ctx, s.Store.Pool, *b.SourceTaskID); err == nil {
				on = t.PublicID
			}
		}
		if on == "" && b.OriginFeatureID != nil {
			if f, err := store.GetFeature(ctx, s.Store.Pool, *b.OriginFeatureID); err == nil {
				on = f.PublicID
			}
		}
		if b.ReporterKind == store.ReporterReview {
			return "Minor findings from the code review of " + on + ", by " + who + "."
		}
		if on != "" {
			return "Reported by " + who + " while working on " + on + "."
		}
		return "Reported by " + who + "."
	}
	return "Reported by " + b.ReportedBy + " in the web UI."
}

func triageWord(t string) string {
	switch t {
	case store.TriageAccepted:
		return "Accepted"
	case store.TriageRejected:
		return "Rejected"
	case store.TriageDuplicate:
		return "Marked as a duplicate"
	}
	return "Waiting for triage"
}

// bugCardFor builds the card for a bug, reading its report's sections.
func (s *Server) bugCardFor(ctx context.Context, b *store.Bug) bugCard {
	c := bugCard{ID: b.Feature.ID, PublicID: b.Feature.PublicID, Title: b.Feature.Name, Triage: b.Triage,
		ReportedBy: s.reporterWords(ctx, b), ReportedAt: b.ReportedAt, State: string(b.Feature.State),
		DecidedBy: b.DecidedBy, DecidedAt: b.DecidedAt, Reason: b.DecidedReason, Quote: b.DecidedQuote,
		DecidedWord: triageWord(b.Triage)}
	if b.DecidedVia == "mcp" {
		c.DecidedBy = "a person, relayed by the chat agent"
	}
	if path, err := s.featurePath(ctx, &b.Feature); err == nil {
		c.URL = "/ui/f/" + path
	}
	if in, err := store.GetInitiative(ctx, s.Store.Pool, b.Feature.InitiativeID); err == nil {
		c.HangsOn = in.PublicID + " " + in.Name
	}
	if b.OriginFeatureID != nil {
		if o, err := store.GetFeature(ctx, s.Store.Pool, *b.OriginFeatureID); err == nil {
			c.OriginName = o.PublicID + " " + o.Name
			if p, err := s.featurePath(ctx, o); err == nil {
				c.OriginURL = "/ui/f/" + p
			}
		}
	}
	if b.DuplicateOf != nil {
		if o, err := store.GetFeature(ctx, s.Store.Pool, *b.DuplicateOf); err == nil {
			c.DupOf = o.PublicID
			if p, err := s.featurePath(ctx, o); err == nil {
				c.DupOfURL = "/ui/f/" + p
			}
		}
	}
	if d, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "bug_report", "feature", b.Feature.ID); err == nil {
		c.ReportURL = "/ui/d/" + d.Path
		if secs, err := store.Sections(ctx, s.Store.Pool, d.ID); err == nil {
			for _, sec := range secs {
				switch sec.Heading {
				case "Summary":
					c.Summary = strings.TrimSpace(sec.Content)
				case "Expected":
					c.Expected = strings.TrimSpace(sec.Content)
				case "Actual":
					c.Actual = strings.TrimSpace(sec.Content)
				}
			}
		}
	}
	return c
}

func (s *Server) bugRows(ctx context.Context, bugs []store.Bug) []bugRow {
	out := make([]bugRow, 0, len(bugs))
	for i := range bugs {
		b := &bugs[i]
		r := bugRow{PublicID: b.Feature.PublicID, Title: b.Feature.Name, Triage: b.Triage, State: string(b.Feature.State)}
		if path, err := s.featurePath(ctx, &b.Feature); err == nil {
			r.URL = "/ui/f/" + path
		}
		out = append(out, r)
	}
	return out
}

// ---- The triage queue (FR-2.3) ----

type triagePage struct {
	Queue   []bugCard
	Recent  []bugCard
	Notice  string
	Error   string
	Crumbs  []crumb
	Heading string
}

func (p triagePage) headTitle() string   { return "Triage" }
func (p triagePage) headCrumbs() []crumb { return p.Crumbs }

func (s *Server) triagePageData(ctx context.Context, notice, errMsg string) (*triagePage, error) {
	p := &triagePage{Notice: notice, Error: errMsg,
		Crumbs: []crumb{{Label: "Triage", URL: "/ui/triage", Here: true}}}
	queue, err := store.TriageQueue(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	for i := range queue {
		p.Queue = append(p.Queue, s.bugCardFor(ctx, &queue[i]))
	}
	recent, err := store.RecentlyTriaged(ctx, s.Store.Pool, 20)
	if err != nil {
		return nil, err
	}
	for i := range recent {
		p.Recent = append(p.Recent, s.bugCardFor(ctx, &recent[i]))
	}
	return p, nil
}

func (s *Server) handleUITriage(w http.ResponseWriter, r *http.Request) {
	p, err := s.triagePageData(r.Context(), "", "")
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-triage", s.page(r.Context(), "triage", p))
}

func (s *Server) handleFragTriageBadge(w http.ResponseWriter, r *http.Request) {
	n, err := store.CountReportedBugs(r.Context(), s.Store.Pool)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-triage-badge", n)
}

// handleUITriageDecide records a triage decision from the queue or a bug's
// page (FR-2.2), and returns to wherever it was made.
func (s *Server) handleUITriageDecide(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	id, err := uuid.Parse(strings.TrimSpace(r.FormValue("bug_id")))
	if err != nil {
		http.Error(w, "bad bug id", http.StatusBadRequest)
		return
	}
	b, derr := s.DecideTriage(ctx, id, r.FormValue("decision"), r.FormValue("reason"), r.FormValue("duplicate_of"), s.uiAct())
	notice := ""
	if derr == nil {
		switch b.Triage {
		case store.TriageAccepted:
			notice = b.Feature.PublicID + " was accepted. It can now be sent to development from its page."
		case store.TriageRejected:
			notice = b.Feature.PublicID + " was rejected, with your reason recorded."
		case store.TriageDuplicate:
			notice = b.Feature.PublicID + " was marked as a duplicate."
		}
	}
	if r.FormValue("from") == "bug" {
		errMsg := ""
		if derr != nil {
			errMsg = derr.Error()
		}
		s.renderEntity(w, r, "feature", id, notice, errMsg)
		return
	}
	errMsg := ""
	if derr != nil {
		errMsg = derr.Error()
	}
	p, err := s.triagePageData(ctx, notice, errMsg)
	if err != nil {
		s.uiError(w, err)
		return
	}
	pushPageURL(w, r, "/ui/triage") // the queue's address, not the form's
	s.render(w, "page-triage", s.page(ctx, "triage", p))
}

// ---- Reporting (FR-3.1) ----

// handleUIReportBug creates a bug from the report dialog on an initiative's,
// a feature's or a bug's page, and opens the new bug's page.
func (s *Server) handleUIReportBug(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	ownerType := r.FormValue("owner_type")
	ownerID, err := uuid.Parse(strings.TrimSpace(r.FormValue("owner_id")))
	if err != nil || (ownerType != "initiative" && ownerType != "feature") {
		http.Error(w, "bad owner", http.StatusBadRequest)
		return
	}
	initiativeID, origin, err := s.reportOn(ctx, ownerType, ownerID)
	if err != nil {
		s.renderEntity(w, r, ownerType, ownerID, "", err.Error())
		return
	}
	f, _, err := s.ReportBug(ctx, BugReport{
		InitiativeID: initiativeID, OriginFeatureID: origin,
		Title: r.FormValue("title"), Summary: r.FormValue("summary"), Steps: r.FormValue("steps"),
		Expected: r.FormValue("expected"), Actual: r.FormValue("actual"), Notes: r.FormValue("notes"),
		ReporterKind: store.ReporterPerson, Actor: s.uiActor(), Via: "ui",
	})
	if err != nil {
		s.renderEntity(w, r, ownerType, ownerID, "", "The bug wasn't reported. "+err.Error())
		return
	}
	s.renderEntity(w, r, "feature", f.ID,
		f.PublicID+" was reported. It waits in the triage queue until someone accepts or rejects it.", "")
}

// ---- On entity pages (FR-2.5, FR-5.1, FR-5.2) ----

// bugPageParts fills an entity page's bug parts: the triage card for a bug,
// and the open bugs hanging off an initiative or a feature.
func (s *Server) bugPageParts(ctx context.Context, page *entityPage) error {
	switch page.Kind {
	case "initiative":
		bugs, err := store.OpenBugsOnInitiative(ctx, s.Store.Pool, page.ID)
		if err != nil {
			return err
		}
		page.Bugs, page.CanReportBug = s.bugRows(ctx, bugs), true
	case "feature":
		page.CanReportBug = true
		if b, err := store.GetBug(ctx, s.Store.Pool, page.ID); err == nil {
			c := s.bugCardFor(ctx, b)
			page.Bug = &c
			// A reported bug is closed by triage, not abandoned (R19-4).
			if b.Triage == store.TriageReported {
				page.CanAbandon = false
			}
			for i := range page.Breadcrumbs {
				if page.Breadcrumbs[i].Here {
					page.Breadcrumbs[i].Kind = "bug"
				}
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		bugs, err := store.OpenBugsOnFeature(ctx, s.Store.Pool, page.ID)
		if err != nil {
			return err
		}
		page.Bugs = s.bugRows(ctx, bugs)
	}
	return nil
}

func (s *Server) handleFragTriageLine(w http.ResponseWriter, r *http.Request) {
	n, err := store.CountReportedBugs(r.Context(), s.Store.Pool)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-triage-line", triageCountLine(n))
}

// triageCountLine is the Inbox's line about the queue (FR-2.4).
func triageCountLine(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "1 bug report is waiting for triage."
	}
	return fmt.Sprintf("%d bug reports are waiting for triage.", n)
}
