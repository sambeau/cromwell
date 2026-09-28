package server

// The decisions viewer and the decision acts in the web UI (SPEC-018 FR-2,
// FR-4.2, FR-5.4, FR-7).

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/config"
	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// surfacedPanel is what a decision or the conventions tells agents, shown on
// its page (FR-4.2).
type surfacedPanel struct {
	Kind         string // decision | conventions
	Line         string
	FromTitle    bool
	RulingWords  int
	RulingCap    int
	ReasonWords  int
	ReasonCap    int
	BodyWords    int
	BodyCap      int
	Accepted     bool
	Superseded   bool
	Supersedes   []string
	SupersededBy []string
	LeftOutOn    []string
}

func (s *Server) surfacedPanelFor(ctx context.Context, doc *store.Document) *surfacedPanel {
	if !surfacedType(doc.Type) {
		return nil
	}
	p := &surfacedPanel{Kind: doc.Type, Accepted: doc.State == lifecycle.DocApproved, Superseded: doc.State == lifecycle.DocSuperseded}
	m, _ := config.LoadManifest(s.CompartmentRoot, doc.Type)
	raw, err := s.readDocFile(doc.Path)
	var parsed *content.Doc
	if err == nil {
		parsed, _ = content.Parse(string(raw))
	}
	if doc.Type == "conventions" {
		if parsed != nil {
			p.BodyWords = lifecycle.CountWords(lifecycle.SurfacedBody(parsed.Body))
		}
		if m != nil {
			p.BodyCap = m.MaxBodyWords()
		}
		return p
	}
	var st store.SurfacedText
	if t, err := store.GetSurfacedText(ctx, s.Store.Pool, doc.ID); err == nil && doc.State != lifecycle.DocDraft && doc.State != lifecycle.DocReviewing {
		st = *t
	} else if err == nil || raw != nil {
		st = surfacedTextOf(doc, raw)
	}
	label := ""
	if doc.OwnerType == "initiative" {
		label = s.ownerName(ctx, doc.OwnerType, doc.OwnerID)
	}
	p.FromTitle = st.FromTitle
	p.Line = content.SurfacedLine(label, content.SurfaceDecision{ID: doc.PublicID, Title: doc.Title, Ruling: st.Ruling, Reason: st.Reason, FromTitle: st.FromTitle})
	if !st.FromTitle {
		p.RulingWords, p.ReasonWords = lifecycle.CountWords(st.Ruling), lifecycle.CountWords(st.Reason)
	}
	if m != nil {
		p.RulingCap, p.ReasonCap = m.MaxWords("ruling"), m.MaxWords("reason")
	}
	if rows, err := s.decisionRows(ctx); err == nil {
		for _, r := range rows {
			if r.Doc.PublicID == doc.PublicID {
				p.Supersedes, p.SupersededBy = r.Supersedes, r.SupersededBy
			}
		}
	}
	if parsed != nil && len(p.Supersedes) == 0 {
		p.Supersedes = upperAll(parsed.FrontMatterList("supersedes"))
	}
	if p.Accepted {
		if left, err := s.leftOutAnywhere(ctx); err == nil {
			p.LeftOutOn = left[doc.PublicID]
		}
	}
	return p
}

// decisionsPage is /ui/decisions (FR-7.1, FR-7.2).
type decisionsPage struct {
	Rows        []decisionRow
	Owner       string // "" all, "project", or an initiative's ID
	State       string // accepted | all | open | superseded
	OwnerName   string
	Initiatives []initiativeOption
	Conventions *store.Document
	Branch      *branchSummary
	LeftOut     map[string][]string
	Notice      string
	Error       string
}

func (decisionsPage) headTitle() string { return "Decisions" }

type initiativeOption struct {
	ID       uuid.UUID
	PublicID string
	Name     string
}

// branchSummary is what a dispatch on a chosen branch is told (FR-7.2).
type branchSummary struct {
	Name      string
	Tokens    int
	MaxTokens int
	Included  []string
	LeftOut   []string
	Block     string
}

func (s *Server) handleUIDecisions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := s.decisionsView(r.Context(), q.Get("owner"), q.Get("state"))
	if err != nil {
		s.uiError(w, err)
		return
	}
	page.Notice, page.Error = q.Get("notice"), q.Get("error")
	s.render(w, "page-decisions", s.page(r.Context(), "decisions", page))
}

func (s *Server) decisionsView(ctx context.Context, owner, state string) (*decisionsPage, error) {
	switch state {
	case "all", "open", "superseded":
	default:
		state = "accepted"
	}
	page := &decisionsPage{Owner: owner, State: state}
	opts, err := s.initiativeOptions(ctx)
	if err != nil {
		return nil, err
	}
	page.Initiatives = opts
	if conv, err := store.LiveConventions(ctx, s.Store.Pool); err == nil {
		page.Conventions = conv
	}
	rows, err := s.decisionRows(ctx)
	if err != nil {
		return nil, err
	}
	if page.LeftOut, err = s.leftOutAnywhere(ctx); err != nil {
		return nil, err
	}

	// The owner filter: the project alone, or an initiative's branch — what
	// its dispatches are told about.
	var branch map[string]bool
	switch {
	case owner == "project":
		page.OwnerName = "the project"
		branch = map[string]bool{"project": true}
		r, err := s.surfaced(ctx, surfaceScope{})
		if err != nil {
			return nil, err
		}
		page.Branch = s.summary("the project", r)
	case owner != "":
		in, err := s.initiativeFromRef(ctx, owner)
		if err != nil {
			return nil, err
		}
		page.OwnerName = withID(in.PublicID, in.Name)
		branch = map[string]bool{"project": true}
		anc, err := store.InitiativeAncestors(ctx, s.Store.Pool, in.ID)
		if err != nil {
			return nil, err
		}
		for _, a := range anc {
			branch[a.ID.String()] = true
		}
		id := in.ID
		r, err := s.surfaced(ctx, surfaceScope{InitiativeID: &id})
		if err != nil {
			return nil, err
		}
		page.Branch = s.summary(page.OwnerName, r)
	}
	for _, row := range rows {
		if branch != nil {
			key := "project"
			if row.Doc.OwnerType == "initiative" && row.Doc.OwnerID != nil {
				key = row.Doc.OwnerID.String()
			}
			if !branch[key] {
				continue
			}
		}
		if !stateMatches(state, row) {
			continue
		}
		row.LeftOut = len(page.LeftOut[row.Doc.PublicID]) > 0
		page.Rows = append(page.Rows, row)
	}
	return page, nil
}

func (s *Server) summary(name string, r content.SurfaceResult) *branchSummary {
	max := config.DefaultSurfacingMaxTokens
	if cfg, err := s.freshConfig(); err == nil {
		max = cfg.SurfacingMaxTokens()
	}
	return &branchSummary{Name: name, Tokens: r.Tokens, MaxTokens: max, Included: r.Included, LeftOut: r.LeftOut, Block: r.Block}
}

func stateMatches(filter string, row decisionRow) bool {
	st := row.Doc.State
	open := strings.Contains(row.State, "draft") || strings.Contains(row.State, "in review")
	switch filter {
	case "all":
		return true
	case "open":
		return open
	case "superseded":
		return st == lifecycle.DocSuperseded
	}
	return st == lifecycle.DocApproved
}

func (s *Server) initiativeOptions(ctx context.Context) ([]initiativeOption, error) {
	rows, err := s.Store.Pool.Query(ctx, `SELECT id, public_id, name FROM initiatives WHERE NOT archived ORDER BY public_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []initiativeOption
	for rows.Next() {
		var o initiativeOption
		if err := rows.Scan(&o.ID, &o.PublicID, &o.Name); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// initiativeFromRef finds an initiative by its ID (INIT-004), its row id, or
// its slug path, with a sentence when there is none.
func (s *Server) initiativeFromRef(ctx context.Context, ref string) (*store.Initiative, error) {
	ref = strings.TrimSpace(ref)
	if id, err := uuid.Parse(ref); err == nil {
		return store.GetInitiative(ctx, s.Store.Pool, id)
	}
	in, _, err := s.initiativeByRef(ctx, strings.Trim(ref, "/"))
	if err != nil {
		return nil, errors.New("There is no initiative " + ref + ".")
	}
	return in, nil
}

// handleDecisionCreate is New decision… and Supersede with a new decision
// (FR-2.1, FR-3.4). The owner comes from the form's owner field, or, for a
// supersession, from the decision being superseded.
func (s *Server) handleDecisionCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	req := NewDecision{Title: r.FormValue("title"), Actor: s.uiActor(), Via: "ui", OwnerType: "project"}
	for _, v := range r.Form["supersedes"] {
		for _, id := range strings.FieldsFunc(v, func(c rune) bool { return c == ',' || c == ' ' }) {
			req.Supersedes = append(req.Supersedes, id)
		}
	}
	back := "/ui/decisions"
	if od := strings.TrimSpace(r.FormValue("owner_doc")); od != "" {
		id, err := uuid.Parse(od)
		if err != nil {
			http.Error(w, "bad document id", http.StatusBadRequest)
			return
		}
		old, err := store.GetDocument(ctx, s.Store.Pool, id)
		if err != nil {
			s.notFoundOrErr(w, r, "document", od, err)
			return
		}
		req.OwnerType, req.OwnerID = old.OwnerType, old.OwnerID
		back = "/ui/d/" + old.Path
	} else if owner := strings.TrimSpace(r.FormValue("owner")); owner != "" && owner != "project" {
		in, err := s.initiativeFromRef(ctx, owner)
		if err != nil {
			http.Redirect(w, r, back+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		req.OwnerType, req.OwnerID = "initiative", &in.ID
	}
	doc, err := s.CreateDecision(ctx, req)
	if err != nil {
		if strings.HasPrefix(back, "/ui/d/") {
			s.renderDocumentPage(w, r, strings.TrimPrefix(back, "/ui/d/"), "", err.Error())
			return
		}
		http.Redirect(w, r, back+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/edit/"+doc.Path, http.StatusSeeOther)
}

// handleConventionsStart is Start the conventions document (FR-5.4).
func (s *Server) handleConventionsStart(w http.ResponseWriter, r *http.Request) {
	doc, err := s.StartConventions(r.Context(), s.uiActor(), "ui")
	if err != nil {
		http.Redirect(w, r, "/ui/decisions?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/edit/"+doc.Path, http.StatusSeeOther)
}
