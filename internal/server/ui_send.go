package server

// Send to development in the web UI (SPEC-011 FR-4), and the document page's
// new actions (FR-5.5, FR-6, FR-9). Every POST here calls a service method in
// review_send.go or documents.go, the same one the relay tools call; the
// handlers only carry the form and report back in sentences.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"subutai/internal/config"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// ---- The feature page's send card (FR-4.1, FR-1.4) ----

// sendCard is what a feature page says about sending: the button, why it is
// blocked, or the mark and Withdraw.
type sendCard struct {
	Sent        bool
	SentBy      string
	SentAt      time.Time
	Hold        bool
	CanSend     bool
	Reason      string // why it can't be sent, when it can't
	CanWithdraw bool
	Show        bool // the feature is in a state where sending means anything
}

func (s *Server) sendCardFor(ctx context.Context, f *store.Feature) sendCard {
	var c sendCard
	if f.State != lifecycle.FeatIdea && f.State != lifecycle.FeatReady {
		return c
	}
	c.Show = true
	if send, err := store.GetFeatureSend(ctx, s.Store.Pool, f.ID); err == nil {
		c.Sent, c.SentBy, c.SentAt, c.Hold = true, send.SentBy, send.SentAt, send.Hold
		c.CanWithdraw, _, _ = s.withdrawable(ctx, f)
		return c
	}
	c.CanSend, c.Reason = s.sendReadiness(ctx, f)
	return c
}

// ---- The send screen (FR-4.3) ----

type sendStep struct {
	Name      string
	Who       string // role and model, or "you"
	Done      bool
	DoneNote  string
	Note      string // a warning or explanation
	Forecast  int64
	Forecasts bool // whether Forecast is a real forecast
}

type sendFeature struct {
	ID          uuid.UUID
	Name        string
	URL         string
	Description string
	CanSend     bool
	Reason      string
	Steps       []sendStep
	Reviewer    string // who will review the spec, in a sentence
	Forecast    int64
	Forecasts   bool // every remaining step has a forecast
	Estimate    string
}

type sendScreen struct {
	Title       string
	Crumbs      []crumb
	BackURL     string
	OwnerKind   string // feature | initiative
	OwnerID     uuid.UUID
	Features    []sendFeature
	AnySendable bool
	AgentReview bool
	HoldDefault bool
	HoldForced  bool
	Workers     int
	Running     int
	Free        int
	Total       int64
	TotalKnown  bool
	Error       string
}

func (p sendScreen) headTitle() string   { return p.Title }
func (p sendScreen) headCrumbs() []crumb { return p.Crumbs }

// forecastMinSamples is how many past runs of a step make a forecast (SD-12).
const forecastMinSamples = 3

func (s *Server) stepForecast(ctx context.Context, purpose string) (int64, bool) {
	xs, err := store.PurposeTokenSamples(ctx, s.Store.Pool, purpose, 20)
	if err != nil || len(xs) < forecastMinSamples {
		return 0, false
	}
	return store.Median(xs), true
}

// roleWho names the role and model a purpose runs as, or "" when it is not
// assigned.
func (s *Server) roleWho(cfg *config.Config, purpose, role string) string {
	if role == "" {
		return ""
	}
	model, err := s.modelForPurpose(cfg, purpose, role)
	if err != nil {
		return role
	}
	return role + " on " + model
}

func (s *Server) reviewerRole(docType string) string {
	m, err := config.LoadManifest(s.CompartmentRoot, docType)
	if err != nil {
		return ""
	}
	return m.ReviewerRole
}

func (s *Server) sendFeatureView(ctx context.Context, cfg *config.Config, f *store.Feature, hold bool) (sendFeature, error) {
	path, err := s.featurePath(ctx, f)
	if err != nil {
		return sendFeature{}, err
	}
	v := sendFeature{ID: f.ID, Name: f.Name, URL: "/ui/f/" + path, Description: f.Description}
	v.CanSend, v.Reason = s.sendReadiness(ctx, f)

	spec, specErr := store.CurrentDocForOwner(ctx, s.Store.Pool, "spec", "feature", f.ID)
	plan, planErr := store.CurrentDocForOwner(ctx, s.Store.Pool, "dev_plan", "feature", f.ID)
	agent := cfg.AgentSpecReview()
	specHumanApproved := s.humanApprovalType("spec")

	written := func(d *store.Document, err error) (bool, string) {
		if err != nil {
			return false, ""
		}
		if d.State == lifecycle.DocDraft {
			if waits, _ := s.waitsForAuthor(ctx, d); waits {
				return false, "a draft that was sent back, which its author will revise"
			}
		}
		// Say who wrote it, so a person sees the step is skipped because
		// the chat agent or someone else did it (SPEC-017 FR-5.1).
		ws, _ := s.writerHistory(ctx, *d)
		return true, "Already written" + doneBy(ws) + ". This step is skipped."
	}
	approved := func(d *store.Document, err error) bool { return err == nil && d.State == lifecycle.DocApproved }
	approvedNote := func(d *store.Document, extra string) string {
		if v, err := store.LastVerdict(ctx, s.Store.Pool, d.ID); err == nil && v.Verdict == store.VerdictApprove {
			return strings.TrimSuffix(verdictSentence(*v), ".") + extra + ". This step is skipped."
		}
		return "Already approved" + extra + ". This step is skipped."
	}
	// A draft nobody has submitted satisfies the invariant (SPEC-011 FR-2.1),
	// so nothing will submit it for review: say so (SPEC-017 FR-5.2).
	unsubmitted := func(d *store.Document, err error, words string) string {
		if err != nil || d.State != lifecycle.DocDraft {
			return ""
		}
		if waits, _ := s.waitsForAuthor(ctx, d); waits {
			return ""
		}
		return "This " + words + " is a draft nobody has submitted, so it won't be reviewed until someone submits it, on its page or from chat."
	}

	v.Forecasts = true
	add := func(step sendStep, purpose string) {
		if !step.Done && purpose != "" {
			step.Forecast, step.Forecasts = s.stepForecast(ctx, purpose)
			if step.Forecasts {
				v.Forecast += step.Forecast
			} else {
				v.Forecasts = false
			}
		}
		v.Steps = append(v.Steps, step)
	}

	// 1. Write the spec.
	st := sendStep{Name: "Write the specification", Who: s.roleWho(cfg, "write-spec", cfg.Assignments["write-spec"])}
	st.Done, st.DoneNote = written(spec, specErr)
	if st.Who == "" && !st.Done {
		st.Note = "Nobody is assigned to write the specification, so it won't be written until you write it or assign write-spec in config.yaml."
	}
	add(st, "write-spec")

	// 2. Review the spec.
	st = sendStep{Name: "Review the specification", Done: approved(spec, specErr)}
	if st.Done {
		st.DoneNote = approvedNote(spec, "")
	}
	specNote := unsubmitted(spec, specErr, "specification")
	switch {
	case specHumanApproved:
		st.Who = "you"
		v.Reviewer = "You approve the specification: this project's spec template says a person approves specs."
	case !agent:
		st.Who = "you"
		v.Reviewer = "You review the specification. Agent review is switched off for this project, so every spec waits for a person."
	default:
		role := s.reviewerRole("spec")
		st.Who = s.roleWho(cfg, "review-spec", role)
		if hold {
			v.Reviewer = "The spec reviewer (" + st.Who + ") checks the specification, then it waits for you."
		} else {
			v.Reviewer = "The spec reviewer (" + st.Who + ") approves the specification."
		}
	}
	if specNote != "" {
		st.Note = specNote
	}
	if st.Who == "you" {
		v.Steps = append(v.Steps, st)
	} else {
		add(st, "review-spec")
	}

	// 3. The hold.
	if (hold || !agent) && !specHumanApproved && !st.Done {
		v.Steps = append(v.Steps, sendStep{Name: "Hold the specification for you", Who: "you",
			Note: "The specification waits after its review until you approve it, raise an issue, or let the reviewer decide."})
	}

	// 4. Write the plan.
	st = sendStep{Name: "Write the dev-plan", Who: s.roleWho(cfg, "write-dev-plan", cfg.Assignments["write-dev-plan"])}
	st.Done, st.DoneNote = written(plan, planErr)
	if st.Who == "" && !st.Done {
		st.Note = "Nobody is assigned to write the dev-plan, so it won't be written until you write it or assign write-dev-plan in config.yaml."
	}
	add(st, "write-dev-plan")

	// 5. Review the plan.
	st = sendStep{Name: "Review the dev-plan", Who: s.roleWho(cfg, "review-dev_plan", s.reviewerRole("dev_plan")), Done: approved(plan, planErr)}
	if st.Done {
		st.DoneNote = approvedNote(plan, ", and its tasks are decomposed")
	}
	if n := unsubmitted(plan, planErr, "dev-plan"); n != "" {
		st.Note = n
	}
	add(st, "review-dev_plan")

	// 6. Estimate.
	st = sendStep{Name: "Estimate the work", Who: s.roleWho(cfg, "estimate", cfg.Assignments["estimate"])}
	if est, err := store.CurrentEstimate(ctx, s.Store.Pool, "feature", f.ID); err == nil {
		v.Estimate = fmt.Sprintf("This feature is estimated at %s tokens to build (%s).", humanTokens(est.Tokens), est.Tier)
		if approved(plan, planErr) && plan.ApprovedAt != nil && est.CreatedAt.After(*plan.ApprovedAt) {
			st.Done, st.DoneNote = true, "Already estimated. This step is skipped."
		}
	}
	if st.Who == "" && !st.Done {
		st.Note = "Nobody is assigned to estimate, so the work won't be sized automatically."
	}
	add(st, "estimate")
	return v, nil
}

func (s *Server) sendScreenFor(ctx context.Context, r *http.Request) (*sendScreen, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	scr := &sendScreen{
		AgentReview: cfg.AgentSpecReview(),
		HoldDefault: cfg.HoldSpecs(),
		HoldForced:  !cfg.AgentSpecReview(),
		Workers:     cfg.Dispatch.Workers,
	}
	scr.Running, _ = store.CountRunningDispatches(ctx, s.Store.Pool)
	scr.Free = scr.Workers - scr.Running
	if scr.Free < 0 {
		scr.Free = 0
	}

	var features []store.Feature
	if raw := r.URL.Query().Get("feature"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, store.ErrNotFound
		}
		f, err := store.GetFeature(ctx, s.Store.Pool, id)
		if err != nil {
			return nil, err
		}
		path, err := s.featurePath(ctx, f)
		if err != nil {
			return nil, err
		}
		crumbs, err := s.initiativeCrumbs(ctx, f.InitiativeID)
		if err != nil {
			return nil, err
		}
		scr.OwnerKind, scr.OwnerID, scr.BackURL = "feature", f.ID, "/ui/f/"+path
		scr.Crumbs = append(crumbs, crumb{Label: f.Name, URL: scr.BackURL, Kind: "feature"}, crumb{Label: "Send to development", Here: true})
		scr.Title = "Send " + f.Name + " to development"
		features = []store.Feature{*f}
	} else {
		id, err := uuid.Parse(r.URL.Query().Get("initiative"))
		if err != nil {
			return nil, store.ErrNotFound
		}
		in, err := store.GetInitiative(ctx, s.Store.Pool, id)
		if err != nil {
			return nil, err
		}
		path, err := s.initiativePath(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		crumbs, err := s.initiativeCrumbs(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		scr.OwnerKind, scr.OwnerID, scr.BackURL = "initiative", in.ID, "/ui/i/"+path
		scr.Crumbs = append(crumbs, crumb{Label: "Send features to development", Here: true})
		scr.Title = "Send features of " + in.Name + " to development"
		if features, err = store.FeaturesForInitiative(ctx, s.Store.Pool, in.ID); err != nil {
			return nil, err
		}
	}
	scr.TotalKnown = true
	for i := range features {
		f := &features[i]
		if f.State != lifecycle.FeatIdea && f.State != lifecycle.FeatReady {
			continue // building or finished: nothing to send
		}
		v, err := s.sendFeatureView(ctx, cfg, f, scr.HoldDefault)
		if err != nil {
			return nil, err
		}
		if v.CanSend {
			scr.AnySendable = true
			scr.Total += v.Forecast
			if !v.Forecasts {
				scr.TotalKnown = false
			}
		}
		scr.Features = append(scr.Features, v)
	}
	return scr, nil
}

func (s *Server) handleUISend(w http.ResponseWriter, r *http.Request) {
	scr, err := s.sendScreenFor(r.Context(), r)
	if err != nil {
		s.notFoundOrErr(w, r, "feature", r.URL.RawQuery, err)
		return
	}
	s.render(w, "page-send", s.page(r.Context(), "browse", scr))
}

// handleUISendPost sends the ticked features (FR-4.4). It is the only route
// that writes the sent mark (FR-4.5).
func (s *Server) handleUISendPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	ownerKind := r.FormValue("owner_kind")
	ownerID, err := uuid.Parse(r.FormValue("owner_id"))
	if err != nil || (ownerKind != "feature" && ownerKind != "initiative") {
		http.Error(w, "bad owner", http.StatusBadRequest)
		return
	}
	hold := r.FormValue("hold") == "1"
	var sent, refused []string
	for _, raw := range r.Form["feature"] {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		f, err := store.GetFeature(ctx, s.Store.Pool, id)
		if err != nil {
			continue
		}
		if err := s.SendToDevelopment(ctx, id, hold, s.uiActor()); err != nil {
			refused = append(refused, f.Name+": "+err.Error())
			continue
		}
		sent = append(sent, f.Name)
	}
	notice, errMsg := "", ""
	switch {
	case len(sent) == 0 && len(refused) == 0:
		errMsg = "Nothing was ticked, so nothing was sent."
	case len(sent) > 0:
		notice = "Sent to development: " + strings.Join(sent, ", ") + ". The agents start as slots free up."
	}
	if len(refused) > 0 {
		errMsg = "Not sent — " + strings.Join(refused, " ")
	}
	s.renderEntity(w, r, ownerKind, ownerID, notice, errMsg)
}

func (s *Server) handleUISendWithdraw(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	id, err := formID(r)
	if err != nil {
		http.Error(w, "bad feature id", http.StatusBadRequest)
		return
	}
	if err := s.WithdrawSend(r.Context(), id, s.uiActor()); err != nil {
		s.renderEntity(w, r, "feature", id, "", err.Error())
		return
	}
	s.renderEntity(w, r, "feature", id, "The send was withdrawn. Nothing was started, and the feature can be sent again later.", "")
}

// ---- The document page's actions (FR-5.5, FR-6, FR-9) ----

// docActions is what the document page may offer, each with its reason when
// it is withheld.
type docActions struct {
	CanSubmit      bool
	CanRevise      bool
	SuccessorURL   string
	CanDetach      bool
	OpenIssueCount int
	// Review acts on an agent-reviewed document (spec, dev-plan).
	AgentReviewed    bool
	CanApprove       bool
	CanSendBack      bool
	CanRequestReview bool
	ReviewRunning    bool
	CanRaiseIssue    bool
	IssueWhy         string // why an issue can't be raised, when it can't
	Held             bool
	HeldByAgent      bool
	AgentOff         bool
	CanRelease       bool
	ReleaseWhy       string
	Pending          string // a question in the Inbox that blocks the acts
	AuthorAtWork     bool
	// A decision's own acts (SPEC-018 FR-3): Revise becomes Append an
	// amendment, or Record its ruling for a decision with none, beside
	// Supersede with a new decision.
	IsDecision   bool
	RecordRuling bool
	CanSupersede bool
}

func (s *Server) docActionsFor(ctx context.Context, doc *store.Document) docActions {
	var a docActions
	a.CanSubmit = doc.State == lifecycle.DocDraft
	a.CanDetach = doc.State == lifecycle.DocDraft
	if doc.State == lifecycle.DocApproved {
		if succ, err := s.liveSuccessor(ctx, doc.ID); err == nil && succ != nil {
			a.SuccessorURL = "/ui/d/" + succ.Path
		} else {
			a.CanRevise = true
		}
	}
	if doc.Type == "decision" {
		a.IsDecision = true
		if doc.State == lifecycle.DocApproved {
			a.CanSupersede = doc.PublicID != ""
			a.RecordRuling = a.CanRevise && !s.decisionHasRuling(ctx, doc)
			a.CanRevise = a.CanRevise && doc.PublicID != ""
		}
	}
	if issues, err := store.OpenIssues(ctx, s.Store.Pool, doc.ID); err == nil {
		a.OpenIssueCount = len(issues)
	}
	if err := s.refuseIfAuthorAtWork(ctx, doc); err != nil {
		a.AuthorAtWork = true
		a.CanSubmit = false
	}
	if err := s.refuseIfPending(ctx, doc); err != nil {
		a.Pending = err.Error()
	}

	if s.humanApprovalType(doc.Type) {
		// Designs: SPEC-009's approve and ask-for-changes panel covers the
		// verdict; an issue is a note for the approver (SD-14).
		a.CanRaiseIssue = doc.State == lifecycle.DocDraft || doc.State == lifecycle.DocReviewing
		return a
	}
	if doc.Type != "spec" && doc.Type != "dev_plan" {
		return a
	}
	a.AgentReviewed = true
	if a.Pending != "" {
		return a
	}
	a.CanRaiseIssue = doc.State != lifecycle.DocSuperseded
	if err := s.checkIssueAllowed(ctx, doc); err != nil {
		a.CanRaiseIssue, a.IssueWhy = false, err.Error()
	}
	if doc.State != lifecycle.DocReviewing {
		return a
	}
	a.CanApprove, a.CanSendBack = true, true
	live, _ := store.LiveDispatchForRef(ctx, s.Store.Pool, "document", doc.ID, "review-"+doc.Type)
	a.ReviewRunning = live
	a.CanRequestReview = !live && s.reviewerRole(doc.Type) != ""
	if hold, err := store.GetDocumentHold(ctx, s.Store.Pool, doc.ID); err == nil {
		a.Held = true
		a.HeldByAgent = hold.DispatchID != nil
	}
	if cfg, err := s.freshConfig(); err == nil {
		a.AgentOff = !cfg.AgentSpecReview()
	}
	if a.Held {
		if _, err := s.releaseState(ctx, doc); err == nil {
			a.CanRelease = true
		} else {
			a.ReleaseWhy = err.Error()
		}
	}
	return a
}

// docForm reads the document id every document-page form carries.
func (s *Server) docForm(w http.ResponseWriter, r *http.Request) (*store.Document, bool) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(r.FormValue("doc_id")))
	if err != nil {
		http.Error(w, "bad document id", http.StatusBadRequest)
		return nil, false
	}
	doc, err := store.GetDocument(r.Context(), s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "document", id.String(), err)
		return nil, false
	}
	return doc, true
}

// afterDocAct re-renders the document's page at its current path — an
// approval of a revision moves it — with the act's outcome.
func (s *Server) afterDocAct(w http.ResponseWriter, r *http.Request, doc *store.Document, notice string, err error) {
	path := doc.Path
	if fresh, ferr := store.GetDocument(r.Context(), s.Store.Pool, doc.ID); ferr == nil {
		path = fresh.Path
	}
	if err != nil {
		s.renderDocumentPage(w, r, path, "", err.Error())
		return
	}
	s.renderDocumentPage(w, r, path, notice, "")
}

func (s *Server) uiAct() relayAct { return relayAct{Actor: s.uiActor(), Via: "ui"} }

func (s *Server) handleDocSubmit(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if doc.State != lifecycle.DocDraft {
		s.afterDocAct(w, r, doc, "", fmt.Errorf("Only a draft can be submitted; this document is %s.", doc.State))
		return
	}
	if err := s.refuseIfAuthorAtWork(ctx, doc); err != nil {
		s.afterDocAct(w, r, doc, "", err)
		return
	}
	report, _, err := s.SubmitDoc(ctx, doc.Path, s.uiActor())
	if err != nil {
		s.afterDocAct(w, r, doc, "", err)
		return
	}
	if !report.Valid {
		var b strings.Builder
		b.WriteString("The document doesn't pass validation yet, so it stays a draft. ")
		for _, is := range report.Issues {
			b.WriteString(strings.TrimSuffix(is.Detail, "."))
			b.WriteString(". ")
		}
		s.afterDocAct(w, r, doc, "", errors.New(strings.TrimSpace(b.String())))
		return
	}
	s.afterDocAct(w, r, doc, "The document was submitted for review.", nil)
}

func (s *Server) handleDocRevise(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	if doc.State != lifecycle.DocApproved {
		s.afterDocAct(w, r, doc, "", fmt.Errorf("Only an approved document is revised; this one is %s, so edit the file directly.", doc.State))
		return
	}
	if succ, err := s.liveSuccessor(r.Context(), doc.ID); err == nil && succ != nil {
		s.afterDocAct(w, r, doc, "", errors.New("A revision of this document is already open. Work on that one."))
		return
	}
	// A decision's revision is an amendment, which the page offers and the
	// editor doesn't (SPEC-018 SD-7).
	if doc.Type == "decision" {
		if err := s.refuseIfPending(r.Context(), doc); err != nil {
			s.afterDocAct(w, r, doc, "", err)
			return
		}
		succ, err := s.reviseDocBy(r.Context(), doc.Path, s.uiActor(),
			writerAct{Act: store.ActOpenedRevision, Kind: store.WriterPerson, Actor: s.uiActor(), Via: "ui"})
		if err != nil {
			s.afterDocAct(w, r, doc, "", err)
			return
		}
		http.Redirect(w, r, "/ui/edit/"+succ.Path, http.StatusSeeOther)
		return
	}
	// The same refusals as the browser editor's revision (SPEC-016 SD-12).
	if why := s.reviseRefusal(r.Context(), doc); why != "" {
		s.afterDocAct(w, r, doc, "", errors.New(why))
		return
	}
	succ, err := s.reviseDocBy(r.Context(), doc.Path, s.uiActor(),
		writerAct{Act: store.ActOpenedRevision, Kind: store.WriterPerson, Actor: s.uiActor(), Via: "ui"})
	if err != nil {
		s.afterDocAct(w, r, doc, "", err)
		return
	}
	s.renderDocumentPage(w, r, succ.Path,
		"A revision was opened. Edit "+succ.Path+" in your editor and submit it here; the original stays approved until the revision is.", "")
}

func (s *Server) handleDocDetach(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "1" {
		s.afterDocAct(w, r, doc, "", errors.New("Tick the box to confirm the detach."))
		return
	}
	_, idOutcome, err := s.detachDocument(r.Context(), doc.ID, s.uiActor())
	if err != nil {
		s.afterDocAct(w, r, doc, "", err)
		return
	}
	target, notice := "project", "The document "+doc.Path+" was detached. Its file is still in the repository."
	switch idOutcome {
	case "committed":
		notice += " Its ID, " + doc.PublicID + ", was taken out of the file, and that change was committed."
	case "uncommitted":
		notice += " Its ID, " + doc.PublicID + ", was taken out of the file; that change isn't committed yet."
	}
	id := uuid.Nil
	if doc.OwnerID != nil {
		target, id = doc.OwnerType, *doc.OwnerID
	}
	s.renderEntity(w, r, target, id, notice, "")
}

func (s *Server) handleDocIssue(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	on, err := s.RaiseIssue(r.Context(), doc.ID, r.FormValue("issue"), r.FormValue("section"), s.uiAct())
	if err != nil {
		s.afterDocAct(w, r, doc, "", err)
		return
	}
	notice := "Your issue was recorded. The reviewer can't approve until it has been answered."
	if s.humanApprovalType(doc.Type) {
		notice = "Your issue was recorded as a note for the person who approves this document."
	}
	if on != nil && on.ID != doc.ID {
		// An approved document opened a successor to carry it (SD-7).
		s.renderDocumentPage(w, r, on.Path, "Your issue opened this revision of the approved document, and was recorded on it.", "")
		return
	}
	s.afterDocAct(w, r, doc, notice, nil)
}

func (s *Server) handleDocApprove(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	err := s.DirectApprove(r.Context(), doc.ID, s.uiAct())
	s.afterDocAct(w, r, doc, "You approved the document. Work that was waiting on it can now go ahead.", err)
}

func (s *Server) handleDocSendBack(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	err := s.SendBack(r.Context(), doc.ID, r.FormValue("reason"), s.uiAct())
	s.afterDocAct(w, r, doc, "The document goes back to its author, with your reason recorded as an issue.", err)
}

func (s *Server) handleDocRequestReview(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	msg, err := s.RequestReview(r.Context(), doc.ID, s.uiAct())
	s.afterDocAct(w, r, doc, msg, err)
}

func (s *Server) handleDocRelease(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	err := s.ReleaseHold(r.Context(), doc.ID, s.uiAct())
	s.afterDocAct(w, r, doc, "The reviewer's approval stands. The dev-plan is written next.", err)
}
