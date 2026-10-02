package server

// Spikes in the web UI (SPEC-021 FR-1.3, FR-1.4, FR-3, FR-7.1, FR-8): the
// spike's page, the Spikes section on an initiative's or feature's page and
// its New spike dialog, the start screen, the list at /ui/spikes, and the
// close forms. Every POST calls a service method in spikes.go, the same one
// the MCP tools call. Starting and closing are here and nowhere else (SD-5,
// SD-10): there is no /api route for either.

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"subutai/internal/dispatch"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// spikeRow is one line in a Spikes list.
type spikeRow struct {
	PublicID string
	Question string
	URL      string
	Badge    spikeBadge
	// Tokens is "tokens used of budget" in words; for a chat or person spike,
	// its time box.
	Tokens string
	// Executor is who runs it as a word: "agent", "chat" or "by hand"; empty
	// for an idea (SPEC-021 FR-16.3).
	Executor string
	// Owner is where the spike hangs, for the list page; empty on an owner's
	// own page.
	Owner    string
	OwnerURL string
}

// spikeBadge is a spike's state as a mark and a word.
type spikeBadge struct {
	Key   string // a lifecycle family the pages already use
	Icon  string
	Label string
}

func spikeBadgeFor(sp *store.Spike) spikeBadge {
	switch sp.State {
	case store.SpikeRunning:
		return spikeBadge{"active", "active", "Running"}
	case store.SpikeEnded:
		return spikeBadge{"review", "review", "Ended, waiting to be read"}
	case store.SpikeClosed:
		if sp.ClosedAs == store.SpikeAnswered {
			return spikeBadge{"done", "done", "Answered"}
		}
		return spikeBadge{"abandoned", "abandoned", "Closed without an answer"}
	}
	return spikeBadge{"idea", "idea", "Not started"}
}

// spikeExecutorWord is the executor as a word beside a spike's state.
func spikeExecutorWord(executor string) string {
	switch executor {
	case store.ExecutorAgent:
		return "agent"
	case store.ExecutorChat:
		return "chat"
	case store.ExecutorPerson:
		return "by hand"
	}
	return ""
}

// tokensOfBudget says "31,004 of 40,000 tokens", or that nothing has started.
func tokensOfBudget(sp *store.Spike) string {
	if unmeasuredExecutor(sp.Executor) {
		if sp.TimeBoxHours == nil {
			return "Not started"
		}
		return fmt.Sprintf("Time box: %d %s", *sp.TimeBoxHours, plural(*sp.TimeBoxHours, "hour", "hours"))
	}
	if sp.TokenBudget == nil {
		return "Not started"
	}
	return groupThousands(sp.TokensUsed) + " of " + groupThousands(*sp.TokenBudget) + " tokens"
}

// spikeCloseForm is what the page's one close-button form is given.
type spikeCloseForm struct {
	Spike       uuid.UUID
	As          string
	Label, Icon string
	Primary     bool
}

// codeSpans shows a sentence with its `backticked` words as code: the text is
// escaped first, so only the marks the sentence carried become markup. A
// sentence with an unmatched backtick is shown as it is.
func codeSpans(s string) template.HTML {
	parts := strings.Split(template.HTMLEscapeString(s), "`")
	if len(parts)%2 == 0 {
		return template.HTML(strings.Join(parts, "`"))
	}
	var b strings.Builder
	for i, p := range parts {
		if i%2 == 1 {
			b.WriteString("<code>" + p + "</code>")
		} else {
			b.WriteString(p)
		}
	}
	return template.HTML(b.String())
}

// spikeWhen is a moment in words: "2 October at 14:05".
func spikeWhen(t time.Time) string { return t.Format("2 January at 15:04") }

// spikeOwnerInfo is the entity a spike hangs on. Path is empty when it can't be
// found, and every field is empty when the owner itself can't be read. URL is
// the owner's page, "" when Path is.
type spikeOwnerInfo struct {
	ID, Name, Type, Path, URL string
}

func (s *Server) spikeOwnerRef(ctx context.Context, sp *store.Spike) spikeOwnerInfo {
	typ, id := sp.Owner()
	if typ == "feature" {
		f, err := store.GetFeature(ctx, s.Store.Pool, id)
		if err != nil {
			return spikeOwnerInfo{}
		}
		path, _ := s.featurePath(ctx, f)
		return spikeOwnerInfo{ID: f.PublicID, Name: f.Name, Type: typ, Path: path, URL: "/ui/f/" + path}
	}
	in, err := store.GetInitiative(ctx, s.Store.Pool, id)
	if err != nil {
		return spikeOwnerInfo{}
	}
	path, _ := s.initiativePath(ctx, in.ID)
	return spikeOwnerInfo{ID: in.PublicID, Name: in.Name, Type: typ, Path: path, URL: "/ui/i/" + path}
}

// spikeOwnerOf names the entity a spike hangs on and links to it.
func (s *Server) spikeOwnerOf(ctx context.Context, sp *store.Spike) (name, url string) {
	o := s.spikeOwnerRef(ctx, sp)
	if o.Path == "" {
		return "", ""
	}
	return o.ID + " " + o.Name, o.URL
}

func (s *Server) spikeRows(ctx context.Context, spikes []store.Spike, withOwner bool) []spikeRow {
	out := make([]spikeRow, 0, len(spikes))
	for i := range spikes {
		sp := &spikes[i]
		r := spikeRow{PublicID: sp.PublicID, Question: sp.Question, URL: "/ui/s/" + sp.PublicID,
			Badge: spikeBadgeFor(sp), Tokens: tokensOfBudget(sp), Executor: spikeExecutorWord(sp.Executor)}
		if withOwner {
			r.Owner, r.OwnerURL = s.spikeOwnerOf(ctx, sp)
		}
		out = append(out, r)
	}
	return out
}

// ---- On an owner's page (FR-8.2) ----

// spikePageParts fills an initiative's or feature's Spikes section: an
// initiative lists the spikes it owns directly, a feature its own, open ones
// first, and either offers New spike unless it is archived, done or abandoned.
func (s *Server) spikePageParts(ctx context.Context, page *entityPage) error {
	switch page.Kind {
	case "initiative":
		spikes, err := store.ListSpikes(ctx, s.Store.Pool, store.SpikeFilter{InitiativeID: &page.ID, DirectOnly: true})
		if err != nil {
			return err
		}
		page.Spikes = s.spikeRows(ctx, spikes, false)
		in, err := store.GetInitiative(ctx, s.Store.Pool, page.ID)
		if err != nil {
			return err
		}
		page.CanCreateSpike = !in.Archived
	case "feature":
		spikes, err := store.ListSpikes(ctx, s.Store.Pool, store.SpikeFilter{FeatureID: &page.ID})
		if err != nil {
			return err
		}
		page.Spikes = s.spikeRows(ctx, spikes, false)
		page.CanCreateSpike = !page.IsTerminal
	}
	return nil
}

// ---- The spike's page (FR-8.1) ----

type spikeMoment struct {
	Label string
	At    time.Time
	Icon  string
	URL   string // the run, where there is one
}

type spikeLink struct {
	PublicID string
	URL      string
	Question string
}

type spikePage struct {
	ID       uuid.UUID
	PublicID string
	Question string
	Badge    spikeBadge
	Crumbs   []crumb

	OwnerName, OwnerURL string
	StateSentence       string

	// ExecutorLine says who runs, or ran, the spike (FR-16.1); empty before the
	// start.
	ExecutorLine string
	// Unmeasured is set for a chat or person spike: it has a time box and its
	// tokens aren't measured, so the token bar is replaced (FR-16.1).
	Unmeasured     bool
	TimeBoxLine    string
	UnmeasuredLine string
	// ClosedBy is "Closed by sam, who also ran it." (FR-16.4), or "".
	ClosedBy string
	// Claim is the panel for a running chat or person spike.
	Claim *spikeClaimPanel

	HasBudget  bool
	Pct        int
	TokensLine string // "31,004 of 40,000 tokens"
	BudgetLine string // before the start: what the budget will be
	OverLine   string // when the run went past its budget
	TokensNote string

	Running bool
	// ByAgent is set for an agent spike (or one not yet started), whose draft
	// is the agent's.
	ByAgent   bool
	Draft     string
	DraftWhen string

	FindingsTitle string
	FindingsURL   string
	FindingsState string

	WorktreeLine string

	CanStart    bool
	CanCloseRun bool
	// AgainBudget is an agent spike's token budget; ShowAgainBudget is false
	// for a chat or person spike, whose Ask again has no budget (FR-15.5).
	AgainBudget     int64
	ShowAgainBudget bool
	// BuildOnSentence says how to build on an answered spike's findings.
	BuildOnSentence string

	Timeline   []spikeMoment
	RunURL     string
	RunNote    string
	Follows    *spikeLink
	FollowedBy *spikeLink

	Notice string
	Error  string
}

func (p spikePage) headTitle() string   { return p.PublicID }
func (p spikePage) headCrumbs() []crumb { return p.Crumbs }

func spikeStateSentence(sp *store.Spike) string {
	switch sp.State {
	case store.SpikeRunning:
		return "This spike is running."
	case store.SpikeEnded:
		return "This spike's run has ended: " + store.SpikeEndingOf(sp.EndedHow).Phrase + ". It's waiting for you to read the findings."
	case store.SpikeClosed:
		if sp.ClosedAs == store.SpikeAnswered {
			return "This spike is closed: the question is answered."
		}
		return "This spike is closed: the question wasn't answered."
	}
	return "This spike hasn't started yet."
}

// Claim panel states for a running chat or person spike (FR-16.1).
const (
	spikePanelFree = "free" // a person spike nobody holds: I'll run this spike
	spikePanelAsk  = "ask"  // a chat spike nobody holds
	spikePanelMine = "mine" // a person's own open claim: the editor
	spikePanelHeld = "held" // held by the chat agent, or by someone else
)

// spikeClaimPanel is the claim panel of a running chat or person spike.
type spikeClaimPanel struct {
	State    string
	SpikeID  uuid.UUID
	PublicID string
	Holder   string
	Since    string
	Activity string
	Path     string
	TimeLeft string
	Draft    string
}

// spikeClaimPanelFor decides what the page offers to whoever runs a running
// chat or person spike, from the facts the page read once.
func (s *Server) spikeClaimPanelFor(sp *store.Spike, facts spikeRunFacts) *spikeClaimPanel {
	if sp.State != store.SpikeRunning || !unmeasuredExecutor(sp.Executor) {
		return nil
	}
	p := &spikeClaimPanel{SpikeID: sp.ID, PublicID: sp.PublicID}
	cur := facts.Claim
	if !facts.held() {
		if sp.Executor == store.ExecutorChat {
			p.State = spikePanelAsk
		} else {
			p.State = spikePanelFree
		}
		return p
	}
	now := time.Now()
	p.Holder = whoWords(cur.Kind, cur.Actor, "")
	p.Since = agoWords(cur.ClaimedAt, now)
	p.Activity = claimActivityWords(cur.LastActivity) + ", " + agoWords(cur.LastActivityAt, now)
	p.State = spikePanelHeld
	if s.PersonClaimant().holds(cur) && cur.State == lifecycle.ClaimOpen {
		p.State = spikePanelMine
		if sp.WorktreePath != "" {
			p.Path = s.worktreeAbs(sp.WorktreePath)
		}
		if sp.DeadlineAt != nil {
			p.TimeLeft = "The time box ends at " + spikeClock(*sp.DeadlineAt) + ": " + spikeLeftWords(sp.DeadlineAt.Sub(now)) + "."
		}
		p.Draft = sp.Draft
	}
	return p
}

func numField(m map[string]any, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

func strField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (s *Server) spikePageData(ctx context.Context, sp *store.Spike, notice, errMsg string) (*spikePage, error) {
	p := &spikePage{ID: sp.ID, PublicID: sp.PublicID, Question: sp.Question,
		Badge: spikeBadgeFor(sp), Notice: notice, Error: errMsg,
		StateSentence: spikeStateSentence(sp), Running: sp.State == store.SpikeRunning}
	p.OwnerName, p.OwnerURL = s.spikeOwnerOf(ctx, sp)

	crumbs, err := s.initiativeCrumbs(ctx, sp.InitiativeID)
	if err != nil {
		return nil, err
	}
	if sp.FeatureID != nil {
		if f, err := store.GetFeature(ctx, s.Store.Pool, *sp.FeatureID); err == nil {
			if path, err := s.featurePath(ctx, f); err == nil {
				crumbs = append(crumbs, crumb{ID: f.PublicID, Label: f.Name, URL: "/ui/f/" + path, Kind: "feature"})
			}
		}
	}
	p.Crumbs = append(crumbs, crumb{ID: sp.PublicID, Label: shortQuestion(sp.Question), Kind: "spike", Here: true})

	p.ByAgent = sp.Executor == store.ExecutorAgent || sp.Executor == ""
	// Who ran it is read once, for the executor line, the claim panel and the
	// closer's sentence.
	facts, err := readSpikeRunFacts(ctx, s.Store.Pool, sp)
	if err != nil {
		return nil, err
	}
	if sp.Executor != "" {
		p.ExecutorLine = spikeExecutorSentence(sp, facts, time.Now()).Sentence
	}
	p.Claim = s.spikeClaimPanelFor(sp, facts)
	p.ClosedBy = spikeClosedBySentence(sp, facts)

	// Tokens against the budget, or, for a chat or person spike, its time box.
	if unmeasuredExecutor(sp.Executor) {
		p.Unmeasured = true
		p.TimeBoxLine = spikeTimeBoxLine(sp, time.Now())
		p.UnmeasuredLine = spikeUnmeasuredLine(sp)
	} else if sp.TokenBudget != nil {
		budget := *sp.TokenBudget
		p.HasBudget = true
		p.TokensLine = tokensOfBudget(sp)
		if budget > 0 {
			p.Pct = int(sp.TokensUsed * 100 / budget)
			if p.Pct > 100 {
				p.Pct = 100
			}
		}
		if sp.TokensUsed > budget {
			p.OverLine = fmt.Sprintf("It used %s tokens, which is more than its budget of %s.",
				groupThousands(sp.TokensUsed), groupThousands(budget))
		}
		p.TokensNote = "These tokens count every model call the run made, including attempts that failed and were tried again."
	} else if cfg, err := s.freshConfig(); err == nil {
		b, src := spikeBudgetFor(cfg, sp)
		if src == store.BudgetFromOverride {
			p.BudgetLine = fmt.Sprintf("It will have a budget of %s tokens, set when it was written down, unless you change it when you start it.", groupThousands(b))
		} else {
			p.BudgetLine = fmt.Sprintf("It will have the project's default budget of %s tokens, unless you change it when you start it.", groupThousands(b))
		}
	}

	// The draft while it runs, the findings once it has ended.
	if p.Running {
		p.Draft = strings.TrimSpace(sp.Draft)
		if sp.DraftSavedAt != nil {
			p.DraftWhen = spikeWhen(*sp.DraftSavedAt)
		}
	}
	findingsID := sp.PublicID + "-findings"
	if sp.State == store.SpikeEnded || sp.State == store.SpikeClosed {
		if d, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "findings", "spike", sp.ID); err == nil {
			p.FindingsTitle, p.FindingsURL, p.FindingsState, findingsID = d.Title, "/ui/d/"+d.Path, string(d.State), d.PublicID
		}
	}

	// The worktree.
	switch {
	case sp.WorktreeRemovedAt != nil:
		p.WorktreeLine = "Its working copy was discarded on " + spikeWhen(*sp.WorktreeRemovedAt) + "."
	case sp.State == store.SpikeRunning:
		p.WorktreeLine = "Its working copy is live."
	case sp.WorktreePath != "":
		p.WorktreeLine = "Its working copy hasn't been discarded yet; it is removed as soon as the run's ending is recorded."
	}

	// What a person can do.
	switch sp.State {
	case store.SpikeIdea:
		p.CanStart = true
	case store.SpikeEnded:
		p.CanCloseRun = true
		if sp.TokenBudget != nil {
			p.AgainBudget = *sp.TokenBudget
		}
		p.ShowAgainBudget = !unmeasuredExecutor(sp.Executor)
	case store.SpikeClosed:
		if sp.ClosedAs == store.SpikeAnswered {
			p.BuildOnSentence = "To build on this, cite " + findingsID + " in a design, then create a feature in the normal way."
		}
	}

	// Its own timeline, from its audit rows.
	run, runs := "", []store.RunSummary(nil)
	if rs, err := store.RunsForRef(ctx, s.Store.Pool, "spike", sp.ID); err == nil && len(rs) > 0 {
		runs = rs
		run = "/ui/run/" + rs[len(rs)-1].ID.String()
	}
	p.RunURL = run
	if len(runs) > 0 {
		last := runs[len(runs)-1]
		p.RunNote = "Read what the agent did, turn by turn."
		if last.Attempt > 1 {
			p.RunNote += fmt.Sprintf(" The run was tried %d times.", last.Attempt)
		}
	}
	hist, err := store.SpikeHistory(ctx, s.Store.Pool, sp.ID)
	if err != nil {
		return nil, err
	}
	for _, e := range hist {
		pl := map[string]any{}
		_ = json.Unmarshal(e.Payload, &pl)
		m := spikeMoment{At: e.OccurredAt, Icon: "spike"}
		switch e.Kind {
		case "spike.created":
			how := "in the web UI"
			if strField(pl, "via") == "mcp" {
				how = "from chat"
			}
			m.Label = "Created by " + e.Actor + " " + how + "."
		case "spike.started":
			if h := int(numField(pl, "time_box_hours")); h > 0 {
				m.Label = fmt.Sprintf("Started by %s, with a time box of %d %s.", e.Actor, h, plural(h, "hour", "hours"))
			} else {
				m.Label = fmt.Sprintf("Started by %s, with a budget of %s tokens.", e.Actor, groupThousands(numField(pl, "budget")))
			}
			m.Icon, m.URL = "run", run
		case "spike.ended":
			m.Label = "Ended: " + store.SpikeEndingOf(strField(pl, "how")).Phrase + "."
			// A chat or person spike's tokens weren't measured: nothing to count.
			if !unmeasuredExecutor(sp.Executor) {
				if b := numField(pl, "token_budget"); b > 0 {
					m.Label += fmt.Sprintf(" It used %s of its %s tokens.", groupThousands(numField(pl, "tokens_used")), groupThousands(b))
				} else {
					m.Label += fmt.Sprintf(" It used %s tokens.", groupThousands(numField(pl, "tokens_used")))
				}
			}
			m.Icon, m.URL = "check", run
		case "spike.worktree_discarded":
			m.Label, m.Icon = "Working copy discarded.", "close"
		case "spike.code_kept":
			var refs []string
			if raw, ok := pl["refs"].([]any); ok {
				for _, r := range raw {
					if rs, ok := r.(string); ok {
						refs = append(refs, "`"+rs+"`")
					}
				}
			}
			m.Icon = "alert"
			if why := strField(pl, "couldnt_check"); why != "" && len(refs) == 0 {
				m.Label = "Subutai couldn't check whether code from this spike was kept: " + why + "."
			} else {
				m.Label = "Code was kept on " + joinWords(refs) + ", outside its working copy. A person decides what to do with it."
			}
		case "spike.closed":
			if strField(pl, "as") == store.SpikeAnswered {
				m.Label = "Closed as answered, by " + e.Actor + "."
			} else {
				m.Label = "Closed without an answer, by " + e.Actor + "."
			}
			m.Icon = "approve"
		default:
			continue
		}
		p.Timeline = append(p.Timeline, m)
	}

	// The spike it follows and the spike that follows it.
	link := func(o *store.Spike) *spikeLink {
		return &spikeLink{PublicID: o.PublicID, URL: "/ui/s/" + o.PublicID, Question: o.Question}
	}
	if sp.FollowsID != nil {
		if o, err := store.GetSpike(ctx, s.Store.Pool, *sp.FollowsID); err == nil {
			p.Follows = link(o)
		}
	}
	if o, err := store.SpikeFollowedBy(ctx, s.Store.Pool, sp.ID); err == nil {
		p.FollowedBy = link(o)
	}
	return p, nil
}

func (s *Server) renderSpike(w http.ResponseWriter, r *http.Request, sp *store.Spike, notice, errMsg string) {
	p, err := s.spikePageData(r.Context(), sp, notice, errMsg)
	if err != nil {
		s.uiError(w, err)
		return
	}
	pushPageURL(w, r, "/ui/s/"+sp.PublicID)
	s.render(w, "page-spike", s.page(r.Context(), "browse", p))
}

func (s *Server) handleUISpike(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sp, err := store.SpikeByPublicID(r.Context(), s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "spike", id, err)
		return
	}
	s.renderSpike(w, r, sp, spikeNoticeFor(sp, r.URL.Query().Get("did")), "")
}

// Starting and closing redirect to the spike's page, so a reload doesn't post
// again (FR-3.3, FR-7.1). "did" names what was done; the page words it from
// the spike as it is, and says nothing the spike's state doesn't bear out.
const (
	spikeDidStart      = "started"
	spikeDidAnswered   = "answered"
	spikeDidUnanswered = "unanswered"
	// What a person did to a running chat or person spike's claim (FR-16.1).
	spikeDidClaimed  = "claimed"
	spikeDidSaved    = "saved"
	spikeDidFinished = "finished"
	spikeDidReleased = "released"
)

func spikeNoticeFor(sp *store.Spike, did string) string {
	switch {
	case did == spikeDidStart && sp.StartedAt != nil && unmeasuredExecutor(sp.Executor) && sp.TimeBoxHours != nil:
		h := *sp.TimeBoxHours
		start := fmt.Sprintf("%s has started, with a time box of %d %s.", sp.PublicID, h, plural(h, "hour", "hours"))
		if sp.Executor == store.ExecutorChat {
			return start + " Ask the chat agent to run it."
		}
		return start + " Claim it below when you are ready."
	case did == spikeDidStart && sp.StartedAt != nil && sp.TokenBudget != nil:
		return sp.PublicID + " has started, with a budget of " + groupThousands(*sp.TokenBudget) + " tokens."
	case did == spikeDidClaimed && sp.State == store.SpikeRunning && sp.Executor == store.ExecutorPerson:
		return "You are running " + sp.PublicID + ". Its working copy is below."
	case did == spikeDidSaved && sp.State == store.SpikeRunning && sp.Draft != "":
		return "Your findings are saved."
	case did == spikeDidFinished && sp.State == store.SpikeEnded && sp.EndedHow == store.SpikeConcluded:
		return "You finished " + sp.PublicID + ". Read its findings below, then say whether they answer the question."
	case did == spikeDidReleased && sp.State == store.SpikeRunning:
		return "You released the claim on " + sp.PublicID + ". The spike keeps running, and its draft and working copy are kept."
	case did == spikeDidAnswered && sp.State == store.SpikeClosed && sp.ClosedAs == store.SpikeAnswered:
		return sp.PublicID + " is closed: the question is answered."
	case did == spikeDidUnanswered && sp.State == store.SpikeClosed && sp.ClosedAs == store.SpikeUnanswered:
		return sp.PublicID + " is closed without an answer."
	}
	return ""
}

// redirectToSpike sends a person to a spike's page after an act of theirs.
func redirectToSpike(w http.ResponseWriter, r *http.Request, sp *store.Spike, did string) {
	http.Redirect(w, r, "/ui/s/"+sp.PublicID+"?did="+did, http.StatusSeeOther)
}

// ---- The list (FR-8.5) ----

type spikesPage struct {
	Rows   []spikeRow
	Open   int
	Notice string
	Error  string
}

func (spikesPage) headTitle() string { return "Spikes" }
func (spikesPage) headCrumbs() []crumb {
	return []crumb{{Label: "Home", URL: "/ui"}, {Label: "Spikes", Kind: "spike", Here: true}}
}

func (s *Server) handleUISpikes(w http.ResponseWriter, r *http.Request) {
	spikes, err := store.ListSpikes(r.Context(), s.Store.Pool, store.SpikeFilter{})
	if err != nil {
		s.uiError(w, err)
		return
	}
	p := &spikesPage{Rows: s.spikeRows(r.Context(), spikes, true)}
	for i := range spikes {
		if spikes[i].State != store.SpikeClosed {
			p.Open++
		}
	}
	s.render(w, "page-spikes", s.page(r.Context(), "browse", p))
}

func (s *Server) handleFragSpikesLine(w http.ResponseWriter, r *http.Request) {
	n, err := store.EndedSpikesCount(r.Context(), s.Store.Pool)
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-spikes-line", endedSpikesLine(n))
}

// endedSpikesLine is the Inbox's line about spikes waiting to be read (FR-8.5).
func endedSpikesLine(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "1 spike has ended and is waiting for you to read its findings."
	}
	return fmt.Sprintf("%d spikes have ended and are waiting for you to read their findings.", n)
}

// ---- The start screen (FR-3.2) ----

type startScreen struct {
	ID       uuid.UUID
	PublicID string
	Question string
	Crumbs   []crumb

	OwnerName, OwnerURL string
	// Started is set when the spike isn't an idea any more: there is no form.
	Started bool

	Runner  string // "The spike runner runs it, on model."
	Model   string
	Refusal string // why the spike runner can't run it, when it can't
	// CanStart is false when the chosen runner is refused: the Start button is
	// then disabled and says why. The chat agent and a person need no runner,
	// so a refused spike runner is never the only choice offered.
	CanStart bool

	// Executor is the radio that is chosen: agent, chat or person. TimeBox is
	// what the time box field holds.
	Executor string
	TimeBox  string
	// TimeBoxDefault is the project's default, in hours.
	TimeBoxDefault int

	Budget int64
	// BudgetTyped is what a refused start had in the budget field.
	BudgetTyped string
	BudgetNote  string

	Block     string // the decisions and conventions the agent will be given
	PrevID    string
	PrevBody  string
	Tools     []string
	Workers   int
	Running   int
	Free      int
	Forecast  string
	Notice    string
	Error     string
	SpikePath string
}

func (p startScreen) headTitle() string   { return "Start " + p.PublicID }
func (p startScreen) headCrumbs() []crumb { return p.Crumbs }

// startForm is what a person had chosen on the start screen, to show it again
// when a start was refused. The zero value is the defaults.
type startForm struct {
	Executor, Budget, TimeBox string
}

func (s *Server) startScreenFor(ctx context.Context, sp *store.Spike, errMsg string, form startForm) (*startScreen, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	p := &startScreen{ID: sp.ID, PublicID: sp.PublicID, Question: sp.Question, Error: errMsg,
		Started: sp.State != store.SpikeIdea, SpikePath: "/ui/s/" + sp.PublicID}
	p.OwnerName, p.OwnerURL = s.spikeOwnerOf(ctx, sp)
	crumbs, err := s.initiativeCrumbs(ctx, sp.InitiativeID)
	if err != nil {
		return nil, err
	}
	p.Crumbs = append(crumbs, crumb{ID: sp.PublicID, Label: shortQuestion(sp.Question), URL: p.SpikePath, Kind: "spike"},
		crumb{Label: "Start this spike", Here: true})

	role, model, refusal := s.spikeRunnerFor(cfg)
	if refusal != "" {
		p.Refusal = refusal
	} else {
		p.Runner = "The spike runner runs it, on " + model + "."
		p.Model = model
		if r, _, err := s.roleAndSkill(role); err == nil {
			p.Tools = append(p.Tools, r.Tools...)
		}
	}
	p.TimeBoxDefault = cfg.SpikeDefaultTimeBoxHours()
	p.TimeBox = strconv.Itoa(p.TimeBoxDefault)
	p.Executor = store.ExecutorAgent
	if p.Refusal != "" {
		p.Executor = store.ExecutorChat
	}
	if form.Executor == store.ExecutorChat || form.Executor == store.ExecutorPerson || (form.Executor == store.ExecutorAgent && p.Refusal == "") {
		p.Executor = form.Executor
	}
	p.CanStart = !p.Started && (p.Executor != store.ExecutorAgent || p.Refusal == "")
	if form.TimeBox != "" {
		p.TimeBox = form.TimeBox
	}
	for _, t := range []string{dispatch.SaveFindingsTool().Name, dispatch.FinishSpikeTool().Name} {
		if !slices.Contains(p.Tools, t) && p.Refusal == "" {
			p.Tools = append(p.Tools, t)
		}
	}

	b, src := spikeBudgetFor(cfg, sp)
	p.Budget = b
	if form.Budget != "" {
		p.BudgetTyped = form.Budget
	}
	if src == store.BudgetFromDefault {
		p.BudgetNote = "This is the project's default budget, from `spikes.default_token_budget`."
	}

	block, err := s.surfacedBlock(ctx, surfaceScope{InitiativeID: &sp.InitiativeID})
	if err != nil {
		return nil, err
	}
	p.Block = strings.TrimSpace(block)
	p.PrevID, p.PrevBody = s.lastSpikeFindings(ctx, sp)

	p.Workers = cfg.Dispatch.Workers
	p.Running, _ = store.CountRunningDispatches(ctx, s.Store.Pool)
	if p.Free = p.Workers - p.Running; p.Free < 0 {
		p.Free = 0
	}
	if n, ok := s.stepForecast(ctx, "run-spike"); ok {
		p.Forecast = fmt.Sprintf("Rough forecast: about %s tokens, from the median of this project's earlier spike runs.", humanTokens(n))
	} else {
		p.Forecast = "There aren't enough earlier spikes to forecast this one yet."
	}
	return p, nil
}

func (s *Server) renderStartScreen(w http.ResponseWriter, r *http.Request, sp *store.Spike, errMsg string, form ...startForm) {
	var f startForm
	if len(form) > 0 {
		f = form[0]
	}
	p, err := s.startScreenFor(r.Context(), sp, errMsg, f)
	if err != nil {
		s.uiError(w, err)
		return
	}
	pushPageURL(w, r, "/ui/spikes/start?spike="+sp.ID.String())
	s.render(w, "page-spike-start", s.page(r.Context(), "browse", p))
}

func (s *Server) handleUISpikeStart(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("spike")))
	if err != nil {
		http.Error(w, "bad spike id", http.StatusBadRequest)
		return
	}
	sp, err := store.GetSpike(r.Context(), s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "spike", id.String(), err)
		return
	}
	s.renderStartScreen(w, r, sp, "")
}

// parseTimeBoxField reads the time box a person typed: nothing, which means
// the project's default, or a whole number of hours from 1 to 168. A typed 0
// is refused (FR-12.5).
func parseTimeBoxField(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, ErrSpikeTimeBox
	}
	return n, checkSpikeTimeBox(n)
}

// parseBudgetField reads the budget a person typed: nothing, or a positive
// whole number of tokens, with or without thousands commas.
func parseBudgetField(raw string) (*int64, error) {
	raw = strings.NewReplacer(",", "", " ", "", "_", "").Replace(strings.TrimSpace(raw))
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		n = 0 // not a number: the budget's own refusal says so
	}
	return &n, checkSpikeBudget(&n)
}

// handleUISpikeStartPost starts a spike (FR-3.3). It is the only route that
// starts one (SD-5).
func (s *Server) handleUISpikeStartPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	id, err := uuid.Parse(strings.TrimSpace(r.FormValue("spike")))
	if err != nil {
		http.Error(w, "bad spike id", http.StatusBadRequest)
		return
	}
	sp, err := store.GetSpike(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "spike", id.String(), err)
		return
	}
	// An older form with no choice means the spike runner, as in stage 1.
	form := startForm{Executor: strings.TrimSpace(r.FormValue("executor")), Budget: strings.TrimSpace(r.FormValue("budget")),
		TimeBox: strings.TrimSpace(r.FormValue("time_box"))}
	if form.Executor == "" {
		form.Executor = store.ExecutorAgent
	}
	req := SpikeStartRequest{Executor: form.Executor}
	switch form.Executor {
	case store.ExecutorAgent:
		budget, err := parseBudgetField(form.Budget)
		if err != nil {
			s.renderStartScreen(w, r, sp, err.Error(), form)
			return
		}
		if budget != nil {
			req.Budget = *budget
		}
	case store.ExecutorChat, store.ExecutorPerson:
		hours, err := parseTimeBoxField(form.TimeBox)
		if err != nil {
			s.renderStartScreen(w, r, sp, err.Error(), form)
			return
		}
		req.TimeBoxHours = hours
	default:
		s.renderStartScreen(w, r, sp, "A spike is run by the spike runner, the chat agent or a person.")
		return
	}
	started, err := s.StartSpike(ctx, id, req, s.uiActor())
	if err != nil {
		s.renderStartScreen(w, r, sp, err.Error(), form)
		return
	}
	redirectToSpike(w, r, started, spikeDidStart)
}

// ---- Creating and closing (FR-1.3, FR-1.4, FR-7) ----

// handleUISpikeCreate creates a spike from the New spike dialog on an
// initiative's or a feature's page, and returns to that page.
func (s *Server) handleUISpikeCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ownerType := r.FormValue("owner_type")
	ownerID, err := uuid.Parse(strings.TrimSpace(r.FormValue("owner_id")))
	if err != nil || (ownerType != "initiative" && ownerType != "feature") {
		http.Error(w, "bad owner", http.StatusBadRequest)
		return
	}
	budget, err := parseBudgetField(r.FormValue("budget"))
	if err != nil {
		s.renderEntity(w, r, ownerType, ownerID, "", "The spike wasn't created. "+err.Error())
		return
	}
	sp, err := s.CreateSpike(r.Context(), ownerType, ownerID, r.FormValue("question"), budget, s.uiActor(), "ui")
	if err != nil {
		s.renderEntity(w, r, ownerType, ownerID, "", "The spike wasn't created. "+err.Error())
		return
	}
	s.renderEntity(w, r, ownerType, ownerID,
		sp.PublicID+" was written down. Nothing runs until you start it from its page.", "")
}

// handleUISpikeClose closes a spike as answered or unanswered, or closes it
// and asks again (FR-7). Asking again redirects to the new spike's start
// screen; nothing starts until the person presses Start there.
func (s *Server) handleUISpikeClose(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ctx := r.Context()
	id, err := uuid.Parse(strings.TrimSpace(r.FormValue("spike")))
	if err != nil {
		http.Error(w, "bad spike id", http.StatusBadRequest)
		return
	}
	cur, err := store.GetSpike(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "spike", id.String(), err)
		return
	}
	as := strings.TrimSpace(r.FormValue("as"))
	var budget *int64
	// Asking again after a chat or person spike has no budget: the new spike's
	// executor and limit are chosen on its own start screen (FR-15.5).
	if as == SpikeCloseAgain && !unmeasuredExecutor(cur.Executor) {
		if budget, err = parseBudgetField(r.FormValue("budget")); err != nil {
			s.renderSpike(w, r, cur, "", err.Error())
			return
		}
	}
	res, err := s.CloseSpike(ctx, id, as, budget, s.uiActor())
	if err != nil {
		if again, gerr := store.GetSpike(ctx, s.Store.Pool, id); gerr == nil {
			cur = again
		}
		s.renderSpike(w, r, cur, "", err.Error())
		return
	}
	switch as {
	case SpikeCloseAgain:
		http.Redirect(w, r, "/ui/spikes/start?spike="+res.ID.String(), http.StatusSeeOther)
	case store.SpikeAnswered:
		redirectToSpike(w, r, res, spikeDidAnswered)
	default:
		redirectToSpike(w, r, res, spikeDidUnanswered)
	}
}

// ---- Running a spike by hand, and releasing a claim (FR-16.1, FR-17.3) ----

// spikeClaimFromForm reads the spike's ID from the form and finds it.
// These routes change a claim and end a spike a person finished, so they
// refuse a post from another site, as the task routes do. None of them can
// start, close or reopen a spike (FR-17.3): each calls the service method the
// MCP tool of the same name calls, and a spike in the wrong state is a
// refusal banner.
func (s *Server) spikeClaimFromForm(w http.ResponseWriter, r *http.Request) (*store.Spike, bool) {
	if !sameOrigin(r) {
		http.Error(w, "Claims are only changed from Subutai's own pages.", http.StatusForbidden)
		return nil, false
	}
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return nil, false
	}
	ref := strings.TrimSpace(r.FormValue("spike"))
	id, err := uuid.Parse(ref)
	if err != nil {
		http.Error(w, "bad spike id", http.StatusBadRequest)
		return nil, false
	}
	sp, err := store.GetSpike(r.Context(), s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "spike", ref, err)
		return nil, false
	}
	return sp, true
}

// renderSpikeAgain shows the spike's page as it now stands, with a refusal.
func (s *Server) renderSpikeAgain(w http.ResponseWriter, r *http.Request, sp *store.Spike, err error) {
	if fresh, gerr := store.GetSpike(r.Context(), s.Store.Pool, sp.ID); gerr == nil {
		sp = fresh
	}
	s.renderSpike(w, r, sp, "", refusalWords(err))
}

func (s *Server) handleUISpikeClaim(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.spikeClaimFromForm(w, r)
	if !ok {
		return
	}
	if _, err := s.ClaimSpike(r.Context(), sp.PublicID, s.PersonClaimant()); err != nil {
		s.renderSpikeAgain(w, r, sp, err)
		return
	}
	redirectToSpike(w, r, sp, spikeDidClaimed)
}

func (s *Server) handleUISpikeDraft(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.spikeClaimFromForm(w, r)
	if !ok {
		return
	}
	if _, err := s.SaveSpikeFindings(r.Context(), sp.PublicID, s.PersonClaimant(), r.FormValue("findings")); err != nil {
		s.renderSpikeAgain(w, r, sp, err)
		return
	}
	redirectToSpike(w, r, sp, spikeDidSaved)
}

func (s *Server) handleUISpikeSubmit(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.spikeClaimFromForm(w, r)
	if !ok {
		return
	}
	// The draft in the editor is what is submitted; a blank one falls back to
	// the saved draft.
	if _, err := s.SubmitSpike(r.Context(), sp.PublicID, s.PersonClaimant(), r.FormValue("findings")); err != nil {
		s.renderSpikeAgain(w, r, sp, err)
		return
	}
	redirectToSpike(w, r, sp, spikeDidFinished)
}

func (s *Server) handleUISpikeRelease(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.spikeClaimFromForm(w, r)
	if !ok {
		return
	}
	if err := s.ReleaseSpikeClaim(r.Context(), sp.PublicID, s.uiActor()); err != nil {
		s.renderSpikeAgain(w, r, sp, err)
		return
	}
	redirectToSpike(w, r, sp, spikeDidReleased)
}
