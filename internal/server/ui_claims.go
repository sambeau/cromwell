package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/ident"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// The web UI's side of claims (SPEC-020 FR-1.5, FR-1.6, FR-4): what the task
// page and the feature page show about who executes a task, and the three
// routes a person claims, submits and releases a task by. The routes call the
// same service methods the MCP tools do (NFR-1).

// executorView is the task page's executor include (FR-1.5).
type executorView struct {
	Line ExecutorLine
	// RunURL links the implementer's run when an agent executed the task.
	RunURL string
	// Unmeasured adds the sentence that its tokens aren't measured, for a
	// task done in chat or by a person (FR-1.5).
	Unmeasured bool
	// ReviewedWith is the model of a code review of the chat agent's work,
	// when it was the project's reviewer for that work (FR-8.4).
	ReviewedWith string
}

// Claim panel states (FR-4.1).
const (
	claimPanelClaimable = "claimable" // no claim, and it can be claimed
	claimPanelRefused   = "refused"   // no claim, and it can't be
	claimPanelPerson    = "person"    // a person's open claim
	claimPanelReturned  = "returned"  // a person's claim, sent back by the reviewer
	claimPanelChat      = "chat"      // the chat agent's open or returned claim
	claimPanelSubmitted = "submitted" // a claim in code review
)

// claimPanel is the task page's claim panel (FR-4.1).
type claimPanel struct {
	State    string
	TaskID   string // the public ID the forms post
	Refusal  string // FR-2.4's sentence, when the state is refused
	Holder   string // "the chat agent", or the person's name
	Claim    *store.Claim
	Own      bool // the claim is this person's, so Renew, Resume and Submit are theirs
	Path     string
	Branch   string
	SpecURL  string
	PlanURL  string
	Comments *ReviewComments
	// Activity says in words what the claim's last activity was.
	Activity string
}

// taskRow is one task in the feature page's list (FR-1.6).
type taskRow struct {
	PublicID string
	Title    string
	State    string
	URL      string
	// Icon is the sprite symbol of who executed it, "" when nobody has; Line
	// is the executor line, the icon's accessible name and title.
	Icon string
	Line string
	// ClaimLabel is "Claimed by the chat agent" for a claim that hasn't ended.
	ClaimLabel string
}

// executorIcon is the sprite symbol for an executor kind.
func executorIcon(kind string) string {
	switch kind {
	case store.WriterAgent:
		return "agent"
	case store.WriterChat:
		return "chat"
	case store.WriterPerson:
		return "owner"
	}
	return ""
}

// taskExecutorView reads who executed a task, for the task page.
func (s *Server) taskExecutorView(ctx context.Context, t *store.Task) (executorView, error) {
	line, err := s.executorLine(ctx, s.Store.Pool, t)
	if err != nil {
		return executorView{}, err
	}
	v := executorView{Line: line, Unmeasured: line.Kind != "" && !line.Measured}
	if line.Kind == store.WriterAgent && line.RunID != "" {
		v.RunURL = "/ui/run/" + line.RunID
	}
	v.ReviewedWith = s.chatReviewedWith(ctx, t)
	return v, nil
}

// chatReviewedWith is the model of the task's latest code review when it was
// chosen because the chat agent did the work: the project's reviewer for that
// work (FR-8.4). It reads what the submit recorded in its audit row, so it
// names a model only when the rule was applied then and chose a different one,
// and a later change to config.yaml doesn't rewrite it. It is "" otherwise.
func (s *Server) chatReviewedWith(ctx context.Context, t *store.Task) string {
	execs, err := store.ExecutionsFor(ctx, s.Store.Pool, "task", t.ID)
	if err != nil || len(execs) == 0 {
		return ""
	}
	last := execs[len(execs)-1]
	if last.Kind != store.WriterChat || last.ClaimID == nil {
		return ""
	}
	var raw []byte
	err = s.Store.Pool.QueryRow(ctx, `
		SELECT payload FROM audit_events
		WHERE kind = 'claim.submitted' AND ref_type = 'task' AND ref_id = $1 AND payload->>'claim_id' = $2
		ORDER BY occurred_at DESC, id DESC LIMIT 1`, t.ID, last.ClaimID.String()).Scan(&raw)
	if err != nil {
		return ""
	}
	var p struct {
		ChatReviewer bool   `json:"chat_reviewer"`
		Model        string `json:"model"`
	}
	if json.Unmarshal(raw, &p) != nil || !p.ChatReviewer {
		return ""
	}
	return p.Model
}

// taskClaimPanel decides which of FR-4.1's six states the task page is in.
func (s *Server) taskClaimPanel(ctx context.Context, t *store.Task) (claimPanel, error) {
	p := claimPanel{TaskID: t.PublicID}
	cur, err := store.CurrentClaimFor(ctx, s.Store.Pool, "task", t.ID)
	if errors.Is(err, store.ErrNotFound) {
		refusal, err := s.claimRefusalFor(ctx, t, s.PersonClaimant())
		if err != nil {
			return p, err
		}
		if refusal != "" {
			p.State, p.Refusal = claimPanelRefused, refusal
		} else {
			p.State = claimPanelClaimable
		}
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p.Claim = cur
	p.Holder = whoWords(cur.Kind, cur.Actor, "")
	p.Own = s.PersonClaimant().holds(cur)
	switch {
	case cur.State == lifecycle.ClaimSubmitted:
		p.State = claimPanelSubmitted
		return p, nil
	case cur.Kind == store.WriterChat:
		p.State = claimPanelChat
	case cur.State == lifecycle.ClaimReturned:
		p.State = claimPanelReturned
	default:
		p.State = claimPanelPerson
	}
	if wt, err := store.LiveWorktreeForFeature(ctx, s.Store.Pool, t.FeatureID); err == nil {
		p.Path, p.Branch = s.worktreeAbs(wt.Path), wt.Branch
	}
	if d, _ := s.contractDoc(ctx, t.FeatureID, "spec"); d != nil {
		p.SpecURL = "/ui/d/" + d.Path
	}
	if d, _ := s.contractDoc(ctx, t.FeatureID, "dev_plan"); d != nil {
		p.PlanURL = "/ui/d/" + d.Path
	}
	// The reviewer's comments, in a round after the first (FR-4.1, R20-6). A
	// send-back records no execution, so the task's own round decides, not the
	// latest execution's.
	if round, err := store.CurrentTaskRound(ctx, s.Store.Pool, t.ID); err == nil && round > 1 {
		if rc, err := s.reviewComments(ctx, s.Store.Pool, t.ID); err == nil {
			p.Comments = rc
		}
	}
	p.Activity = claimActivityWords(cur.LastActivity)
	return p, nil
}

// featureTaskRows lists a feature's tasks with their executors (FR-1.6), and,
// when a claim holds the working copy, the sentence that says its agents are
// waiting (FR-4.4).
func (s *Server) featureTaskRows(ctx context.Context, featureID uuid.UUID) ([]taskRow, string, error) {
	tasks, err := store.TasksForFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return nil, "", err
	}
	rows := make([]taskRow, 0, len(tasks))
	for i := range tasks {
		t := &tasks[i]
		line, err := s.executorLine(ctx, s.Store.Pool, t)
		if err != nil {
			return nil, "", err
		}
		r := taskRow{PublicID: t.PublicID, Title: t.Title, State: string(t.State), URL: "/ui/t/" + t.ID.String(),
			Icon: executorIcon(line.Kind), Line: line.Sentence}
		if line.Kind == "" {
			r.Line = ""
		}
		if cl, err := store.CurrentClaimFor(ctx, s.Store.Pool, "task", t.ID); err == nil {
			r.ClaimLabel = "Claimed by " + whoWords(cl.Kind, cl.Actor, "")
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, "", err
		}
		rows = append(rows, r)
	}
	var waiting string
	if open, err := store.OpenClaimForFeature(ctx, s.Store.Pool, featureID); err == nil {
		for i := range tasks {
			if tasks[i].ID == open.RefID {
				waiting = fmt.Sprintf("%s %s is claimed by %s, so this feature's agents are waiting.",
					taskNumber(&tasks[i]), tasks[i].Title, whoWords(open.Kind, open.Actor, ""))
			}
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, "", err
	}
	return rows, waiting, nil
}

// ---- The routes (FR-4.2) ----

// claimTaskFromForm reads the task's public ID from the form and finds it.
func (s *Server) claimTaskFromForm(w http.ResponseWriter, r *http.Request) (*store.Task, string, bool) {
	// These routes commit in the working copy and hand work to a paid agent, so
	// they refuse a post from another site, as the editor does (R16-9).
	if !sameOrigin(r) {
		http.Error(w, "Claims are only changed from Subutai's own pages.", http.StatusForbidden)
		return nil, "", false
	}
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return nil, "", false
	}
	ref := strings.TrimSpace(r.FormValue("task"))
	id, ok := ident.Parse(ref)
	if !ok || id.Shape != ident.ShapeTask {
		http.Error(w, "bad task id", http.StatusBadRequest)
		return nil, "", false
	}
	t, err := store.TaskByPublicID(r.Context(), s.Store.Pool, id.ID)
	if err != nil {
		s.notFoundOrErr(w, r, "task", ref, err)
		return nil, "", false
	}
	return t, ref, true
}

// refusalWords is what a banner says about a failed claim act: the refusal's
// own sentence, or the error.
func refusalWords(err error) string {
	if ref, ok := AsClaimRefusal(err); ok {
		return ref.Sentence
	}
	return err.Error()
}

// redirectToTask sends the browser back to the task's page after a claim act
// that worked, so a refresh doesn't post again. The notice travels as ?done,
// which taskDoneNotice turns into a fixed sentence; extra carries the few
// facts the sentence adds.
func redirectToTask(w http.ResponseWriter, r *http.Request, t *store.Task, done string, extra url.Values) {
	q := url.Values{"done": {done}}
	for k, v := range extra {
		q[k] = v
	}
	http.Redirect(w, r, "/ui/t/"+t.ID.String()+"?"+q.Encode(), http.StatusSeeOther)
}

// taskDoneNotice is the sentence a task page opens with after a claim act
// (?done), or "" when there is none.
//
// Nothing free in the URL reaches the sentence: the review's model is read
// from the task's latest review-code dispatch, not from the query.
func (s *Server) taskDoneNotice(ctx context.Context, t *store.Task, q url.Values) string {
	label := t.PublicID
	switch q.Get("done") {
	case "claimed":
		return fmt.Sprintf("You claimed %s. This feature's agents wait until you submit it.", label)
	case "renewed":
		return fmt.Sprintf("%s is still yours. The claim is renewed.", label)
	case "resumed":
		return fmt.Sprintf("You are working on %s again. The reviewer's comments are below.", label)
	case "submitted":
		notice := fmt.Sprintf("You submitted %s for code review.", label)
		var m string
		if err := s.Store.Pool.QueryRow(ctx, `
			SELECT model FROM dispatches WHERE ref_type = 'task' AND ref_id = $1 AND purpose = 'review-code'
			ORDER BY queued_at DESC, id DESC LIMIT 1`, t.ID).Scan(&m); err == nil && m != "" {
			notice += fmt.Sprintf(" The review runs on %s.", m)
		}
		if q.Get("revised") != "" {
			notice += " " + specRevisedNotice
		}
		return notice
	case "released":
		return fmt.Sprintf("You released %s. An agent can implement it now, and what is in the working copy is kept.", label)
	}
	return ""
}

// handleUITaskClaim claims a task for the person, renews their open claim or
// resumes one the code reviewer sent back (FR-4.2).
func (s *Server) handleUITaskClaim(w http.ResponseWriter, r *http.Request) {
	t, ref, ok := s.claimTaskFromForm(w, r)
	if !ok {
		return
	}
	res, err := s.ClaimWork(r.Context(), ref, s.PersonClaimant())
	if err != nil {
		s.renderTaskPage(w, r, t.ID, "", refusalWords(err))
		return
	}
	switch {
	case res.Renewed:
		redirectToTask(w, r, t, "renewed", nil)
	case res.Resumed:
		redirectToTask(w, r, t, "resumed", nil)
	default:
		redirectToTask(w, r, t, "claimed", nil)
	}
}

// handleUITaskSubmit submits the person's work for code review; the summary
// is required (FR-2.5, FR-4.2).
func (s *Server) handleUITaskSubmit(w http.ResponseWriter, r *http.Request) {
	t, ref, ok := s.claimTaskFromForm(w, r)
	if !ok {
		return
	}
	summary := strings.TrimSpace(r.FormValue("summary"))
	if summary == "" {
		s.renderTaskPage(w, r, t.ID, "", "Say what you did, in a sentence or two, so the code reviewer knows what to look for.")
		return
	}
	res, err := s.SubmitWork(r.Context(), ref, s.PersonClaimant(), summary)
	if err != nil {
		s.renderTaskPage(w, r, t.ID, "", refusalWords(err))
		return
	}
	extra := url.Values{}
	if res.Notice != "" {
		extra.Set("revised", "1")
	}
	redirectToTask(w, r, t, "submitted", extra)
}

// handleUITaskRelease releases the task's claim, the chat agent's or the
// person's, as the person (FR-4.3, FR-2.9).
func (s *Server) handleUITaskRelease(w http.ResponseWriter, r *http.Request) {
	t, ref, ok := s.claimTaskFromForm(w, r)
	if !ok {
		return
	}
	if err := s.ReleaseClaim(r.Context(), ref, s.uiActor()); err != nil {
		s.renderTaskPage(w, r, t.ID, "", refusalWords(err))
		return
	}
	redirectToTask(w, r, t, "released", nil)
}
