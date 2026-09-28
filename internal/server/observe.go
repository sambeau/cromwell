package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/reviewhealth"
	"subutai/internal/store"
	"subutai/internal/timeline"
)

// Seeing the work (SPEC-012): transcript retention, and the shared helpers
// behind the transcript viewer, the feature timeline and review health.

// pruneEvery paces the retention sweep. Retention is counted in days, so
// running it on every heartbeat would only repeat a query that finds nothing.
const pruneEvery = time.Hour

// PruneTranscriptsSweep is a heartbeat duty (SPEC-012 FR-2.4): it deletes the
// transcripts of runs that finished longer ago than the configured retention,
// at most once an hour. Outcomes, tokens and the tool ledger are kept.
func (s *Server) PruneTranscriptsSweep(ctx context.Context, now time.Time) {
	if !s.lastPrune.IsZero() && now.Sub(s.lastPrune) < pruneEvery {
		return
	}
	s.lastPrune = now
	cfg, err := s.freshConfig()
	if err != nil {
		return
	}
	n, err := s.Store.PruneTranscripts(ctx, cfg.Transcripts.Retention())
	if err != nil {
		s.Log.Error("transcript retention", "err", err)
		return
	}
	if n > 0 {
		s.Log.Info("transcript retention", "entries_removed", n, "retention_days", cfg.Transcripts.Retention())
	}
}

// ---- Words for runs (D-6) ----

// runPurpose says what a run was for, in the words a person uses, with the
// title of the thing it was about where that helps.
func runPurpose(purpose, label string) string {
	about := ""
	if label != "" {
		about = " “" + label + "”"
	}
	switch purpose {
	case "write-spec":
		return "Writing the specification"
	case "write-dev-plan":
		return "Writing the development plan"
	case "review-spec":
		return "Reviewing the specification" + about
	case "review-dev_plan":
		return "Reviewing the development plan" + about
	case "review-design":
		return "Commenting on the design" + about
	case "implement-task":
		return "Building" + orTask(about)
	case "review-code":
		return "Reviewing the code of" + orTask(about)
	case "verify-feature":
		return "Checking the feature against its specification"
	case "estimate":
		return "Estimating the size"
	}
	if strings.HasPrefix(purpose, "review-") {
		return "Reviewing the document" + about
	}
	return strings.ReplaceAll(purpose, "-", " ")
}

func orTask(about string) string {
	if about == "" {
		return " a task"
	}
	return about
}

// verdictLabel names a verdict in words.
func verdictLabel(v string) string {
	switch v {
	case "approve":
		return "Approved"
	case "request_changes":
		return "Asked for changes"
	case "escalate":
		return "Escalated to a person"
	default:
		return v
	}
}

// verdictState picks the lifecycle hue for a verdict badge.
func verdictState(v string) string {
	switch v {
	case "approve":
		return "done"
	case "request_changes":
		return "abandoned"
	case "escalate":
		return "review"
	default:
		return "idea"
	}
}

// humanDuration says how long something took, in words.
func humanDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "under a millisecond"
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < time.Minute:
		n := int(d.Seconds() + 0.5)
		return fmt.Sprintf("%d %s", n, plural(n, "second", "seconds"))
	case d < time.Hour:
		n := int(d.Minutes() + 0.5)
		return fmt.Sprintf("%d %s", n, plural(n, "minute", "minutes"))
	default:
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
}

// humanBytes says how big some text is.
func humanBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d bytes", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// runRow is one run in a list: on a task or document page (FR-4), or in a
// timeline phase (FR-6.3).
type runRow struct {
	ID       uuid.UUID
	URL      string
	What     string
	State    string
	Verdict  string
	Tokens   int64
	Role     string
	Model    string
	Attempt  int
	When     time.Time
	Took     string
	HasTaken bool
}

func runRowFrom(r store.RunSummary, label string) runRow {
	row := runRow{
		ID: r.ID, URL: "/ui/run/" + r.ID.String(), What: runPurpose(r.Purpose, label),
		State: r.State, Verdict: r.Verdict, Tokens: r.Tokens(), Role: r.Role, Model: r.Model,
		Attempt: r.Attempt, When: r.QueuedAt,
	}
	if r.StartedAt != nil {
		row.When = *r.StartedAt
		if r.FinishedAt != nil {
			row.Took, row.HasTaken = humanDuration(r.FinishedAt.Sub(*r.StartedAt)), true
		}
	}
	return row
}

// runRowsFor lists the runs on one entity, for its page (FR-4.1, FR-4.2).
func (s *Server) runRowsFor(ctx context.Context, refType string, refID uuid.UUID) ([]runRow, error) {
	runs, err := store.RunsForRef(ctx, s.Store.Pool, refType, refID)
	if err != nil {
		return nil, err
	}
	out := make([]runRow, 0, len(runs))
	for _, r := range runs {
		// The page already says what the run was about.
		out = append(out, runRowFrom(r, ""))
	}
	return out, nil
}

// ---- The feature timeline (FR-5, FR-6) ----

type timelineView struct {
	FeatureID uuid.UUID
	Line      []timeline.Strip
	Moments   []momentView
}

type momentView struct {
	timeline.Moment
	Index   int
	RefURL  string
	Current bool
	// Cause is the run that led to this moment, when an agent did (FR-5.5).
	Cause  *runRow
	Runs   []runRow
	Events []eventRow
}

type eventRow struct {
	Actor string
	Kind  string
	About string
	At    time.Time
}

// featureTimeline builds a feature's journey from its audit stream and runs.
func (s *Server) featureTimeline(ctx context.Context, featureID uuid.UUID) (*timelineView, error) {
	history, err := store.FeatureHistory(ctx, s.Store.Pool, featureID)
	if err != nil {
		return nil, err
	}
	runs, err := store.FeatureRuns(ctx, s.Store.Pool, featureID)
	if err != nil {
		return nil, err
	}
	tasks, err := store.TasksForFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return nil, err
	}
	var progress timeline.Progress
	for _, t := range tasks {
		if t.State == lifecycle.TaskAbandoned {
			continue
		}
		progress.Total++
		if t.State == lifecycle.TaskDone {
			progress.Done++
		}
	}

	paths := map[uuid.UUID]string{}
	events := make([]timeline.Event, 0, len(history))
	for _, h := range history {
		e := timeline.Event{ID: h.ID, At: h.OccurredAt, Actor: h.Actor, Kind: h.Kind, RefType: h.RefType,
			Payload: timeline.ParsePayload(h.Payload), DocType: h.DocType, Label: h.Label}
		if h.RefID != nil {
			e.RefID = *h.RefID
		}
		if h.Path != "" {
			paths[e.RefID] = h.Path
		}
		events = append(events, e)
	}
	byID := map[uuid.UUID]store.RunSummary{}
	truns := make([]timeline.Run, 0, len(runs))
	for _, r := range runs {
		byID[r.ID] = r
		truns = append(truns, timeline.Run{ID: r.ID, Purpose: r.Purpose, Role: r.Role, Model: r.Model,
			State: r.State, RefType: r.RefType, RefID: r.RefID, Label: r.Label, Attempt: r.Attempt,
			QueuedAt: r.QueuedAt, FinishedAt: r.FinishedAt, Tokens: r.Tokens(), Verdict: r.Verdict})
	}

	chatActor := ""
	if cfg, err := s.freshConfig(); err == nil {
		chatActor = cfg.Server.MCPActor
	}
	moments := timeline.Build(events, truns, timeline.Options{Progress: progress, ChatActor: chatActor})
	view := &timelineView{FeatureID: featureID, Line: timeline.Line(moments)}
	for i, m := range moments {
		mv := momentView{Moment: m, Index: i, Current: i == len(moments)-1}
		if m.Cause != nil {
			label := byID[m.Cause.ID].Label
			if m.Cause.RefType == "feature" {
				label = ""
			}
			row := runRowFrom(byID[m.Cause.ID], label)
			mv.Cause = &row
		}
		switch m.RefType {
		case "document":
			if p := paths[m.RefID]; p != "" {
				mv.RefURL = "/ui/d/" + p
			}
		case "task":
			mv.RefURL = "/ui/t/" + m.RefID.String()
		}
		for _, r := range m.Runs {
			label := byID[r.ID].Label
			if r.RefType == "feature" {
				label = "" // the page is the feature's own
			}
			mv.Runs = append(mv.Runs, runRowFrom(byID[r.ID], label))
		}
		for _, e := range m.Events {
			// Runs are listed on their own, with links; their lifecycle rows
			// would only say the same thing again.
			if strings.HasPrefix(e.Kind, "dispatch.") {
				continue
			}
			mv.Events = append(mv.Events, eventRow{Actor: e.Actor, Kind: e.Kind, About: e.Label, At: e.At})
		}
		view.Moments = append(view.Moments, mv)
	}
	return view, nil
}

func (s *Server) handleFragTimeline(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.URL.Query().Get("feature"))
	if err != nil {
		http.Error(w, "a feature id is required", http.StatusBadRequest)
		return
	}
	view, err := s.featureTimeline(r.Context(), id)
	if err != nil {
		s.uiError(w, err)
		return
	}
	// Only the one-line view refreshes live: swapping the detail would close
	// whatever the reader has opened in it (R12-8).
	s.render(w, "feature-timeline-line", view)
}

// handleFragRunProgress is the small status line a running run's page polls
// (FR-3.7). It says how far the run has got, and stops polling once it has
// ended; the conversation itself is never swapped, so nothing the reader has
// opened closes under them.
func (s *Server) handleFragRunProgress(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "not a run", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	d, err := store.GetDispatch(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "agent run", id.String(), err)
		return
	}
	turns, err := store.TranscriptTurns(ctx, s.Store.Pool, d.ID, d.Attempt)
	if err != nil {
		s.uiError(w, err)
		return
	}
	shown, _ := strconv.Atoi(r.URL.Query().Get("shown"))
	s.render(w, "run-progress", runProgress{ID: d.ID, State: d.State, Turns: turns, Shown: shown})
}

// runProgress is the running page's status line.
type runProgress struct {
	ID    uuid.UUID
	State string
	Turns int
	Shown int // turns on the page as it was loaded
}

// failureWords says why a run failed in a sentence a person can read, chosen
// by the kind of failure; the engine's own text is shown beneath it (R12-14).
func failureWords(raw string) string {
	switch {
	case strings.HasPrefix(raw, "stalled"):
		return "The run stopped sending signs of life, so it was given up on. Its process may have died, or a single step took longer than the project allows."
	case strings.Contains(raw, "provider unavailable"):
		return "The model's service didn't answer, even after several tries."
	case strings.HasPrefix(raw, "turn cap"):
		return "The agent used every turn it was allowed without giving its answer."
	case strings.Contains(raw, "not in config.yaml"):
		return "The model this run was assigned isn't in the project's configuration."
	case strings.Contains(raw, "provider"):
		return "The model's service refused the request."
	case strings.Contains(raw, "context canceled"):
		return "The server stopped while the run was going."
	default:
		return "The run failed."
	}
}

// ---- The transcript viewer (FR-3) ----

type runPage struct {
	Run       store.Dispatch
	Heading   string
	Crumbs    []crumb
	About     crumb // the task, document or feature the run was about
	Feature   crumb // the feature it belongs to, when there is one
	Attempts  []int
	Attempt   int
	IsLatest  bool
	Running   bool
	Took      string
	TokensIn  int64
	TokensOut int64

	HasTranscript bool
	Pruned        *time.Time
	System        *store.TranscriptEntry
	Prompt        *store.TranscriptEntry
	Turns         []turnView
	Conclusion    conclusion
	Ledger        []store.ToolCallRow
}

func (p runPage) headTitle() string   { return p.Heading }
func (p runPage) headCrumbs() []crumb { return p.Crumbs }

type turnView struct {
	N         int
	HasTokens bool
	In, Out   int64
	Latency   string
	Items     []turnItem
}

// turnItem is one thing that happened in a turn: the agent's text, a tool
// call with its result, a nudge, the outcome, or the error that ended it.
type turnItem struct {
	Kind    string // text | tool | nudge | outcome | error
	Entry   store.TranscriptEntry
	Arg     string // a tool call's main argument, for its summary line
	Result  *store.TranscriptEntry
	Latency string
}

type conclusion struct {
	Kind      string // verdict | implementation | document | error | running | queued | other
	Verdict   string
	Plain     string // a failure in words, above the raw error
	Reasoning string
	Findings  []finding
	Criteria  []criterion
	Summary   string
	Files     []string
	Words     int
	Error     string
	Raw       string
}

type finding struct {
	Severity string
	Section  string
	Body     string
}

type criterion struct {
	ID       string
	Met      bool
	Evidence string
}

func (s *Server) handleUIRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.uiNotFound(w, r, "agent run", r.PathValue("id"))
		return
	}
	ctx := r.Context()
	d, err := store.GetDispatch(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "agent run", id.String(), err)
		return
	}
	page, err := s.buildRunPage(ctx, d, r.URL.Query().Get("attempt"))
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-run", s.page(ctx, "browse", page))
}

func (s *Server) buildRunPage(ctx context.Context, d *store.Dispatch, attemptParam string) (*runPage, error) {
	p := &runPage{Run: *d, Running: d.State == "running"}
	about, feature, label := s.runContext(ctx, d)
	p.About, p.Feature = about, feature
	p.Heading = runPurpose(d.Purpose, label)
	if feature.URL != "" && feature.URL != about.URL {
		p.Crumbs = append(p.Crumbs, crumb{Label: feature.Label, URL: feature.URL, Kind: "feature"})
	}
	if about.URL != "" {
		p.Crumbs = append(p.Crumbs, about)
	}
	p.Crumbs = append(p.Crumbs, crumb{Label: "Agent run", Here: true})

	attempts, err := store.TranscriptAttempts(ctx, s.Store.Pool, d.ID)
	if err != nil {
		return nil, err
	}
	p.Attempts = attempts
	p.Attempt = d.Attempt
	if n, err := strconv.Atoi(attemptParam); err == nil && n >= 1 && n <= d.Attempt {
		p.Attempt = n
	}
	p.IsLatest = p.Attempt == d.Attempt
	if !p.IsLatest {
		// Only the latest attempt can still be running or have succeeded;
		// an earlier one ended in failure, which is why there was another.
		p.Run.State = "failed"
		p.Running = false
	}

	entries, err := store.Transcript(ctx, s.Store.Pool, d.ID, p.Attempt)
	if err != nil {
		return nil, err
	}
	p.HasTranscript = len(entries) > 0
	if !p.HasTranscript {
		if p.Pruned, err = store.TranscriptPrunedAt(ctx, s.Store.Pool, d.ID); err != nil {
			return nil, err
		}
		if p.Ledger, err = store.ToolCallsFor(ctx, s.Store.Pool, d.ID); err != nil {
			return nil, err
		}
	}
	p.buildTurns(entries)

	if p.IsLatest {
		if d.InputTokens != nil {
			p.TokensIn = *d.InputTokens
			if d.CacheReadTokens != nil {
				p.TokensIn += *d.CacheReadTokens
			}
			if d.CacheWriteTokens != nil {
				p.TokensIn += *d.CacheWriteTokens
			}
		}
		if d.OutputTokens != nil {
			p.TokensOut = *d.OutputTokens
		}
		if d.StartedAt != nil && d.FinishedAt != nil {
			p.Took = humanDuration(d.FinishedAt.Sub(*d.StartedAt))
		}
	} else if n := len(entries); n > 1 {
		p.Took = humanDuration(entries[n-1].CreatedAt.Sub(entries[0].CreatedAt))
	}
	if p.TokensIn == 0 && p.TokensOut == 0 {
		// A failed attempt records no totals on the run; its turns do.
		for _, t := range p.Turns {
			p.TokensIn += t.In
			p.TokensOut += t.Out
		}
	}
	p.Conclusion = p.conclude(d, entries)
	return p, nil
}

// runContext finds what a run was about and the feature it belongs to.
func (s *Server) runContext(ctx context.Context, d *store.Dispatch) (about, feature crumb, label string) {
	featureCrumb := func(id uuid.UUID) crumb {
		f, err := store.GetFeature(ctx, s.Store.Pool, id)
		if err != nil {
			return crumb{}
		}
		path, err := s.featurePath(ctx, f)
		if err != nil {
			return crumb{}
		}
		return crumb{Label: f.Name, URL: "/ui/f/" + path, Kind: "feature"}
	}
	switch d.RefType {
	case "feature":
		feature = featureCrumb(d.RefID)
		about = feature
	case "task":
		if t, err := store.GetTask(ctx, s.Store.Pool, d.RefID); err == nil {
			label = t.Title
			about = crumb{Label: t.Title, URL: "/ui/t/" + t.ID.String(), Kind: "task"}
			feature = featureCrumb(t.FeatureID)
		}
	case "document":
		if doc, err := store.GetDocument(ctx, s.Store.Pool, d.RefID); err == nil {
			label = doc.Title
			about = crumb{Label: doc.Title, URL: "/ui/d/" + doc.Path, Kind: "document"}
			if doc.OwnerType == "feature" && doc.OwnerID != nil {
				feature = featureCrumb(*doc.OwnerID)
			}
		}
	case "initiative":
		if in, err := store.GetInitiative(ctx, s.Store.Pool, d.RefID); err == nil {
			if path, err := s.initiativePath(ctx, in.ID); err == nil {
				about = crumb{Label: in.Name, URL: "/ui/i/" + path, Kind: "initiative"}
			}
		}
	}
	return about, feature, label
}

// buildTurns groups entries into the prompts and the turns, pairing each tool
// call with its result (FR-3.3, FR-3.4).
func (p *runPage) buildTurns(entries []store.TranscriptEntry) {
	results := map[string]*store.TranscriptEntry{}
	answered := map[string]bool{} // calls shown as the outcome, not as a tool
	for i := range entries {
		if entries[i].Kind == store.EntryToolResult && entries[i].ToolUseID != "" {
			results[entries[i].ToolUseID] = &entries[i]
		}
		if entries[i].Kind == store.EntryOutcome && entries[i].ToolUseID != "" {
			answered[entries[i].ToolUseID] = true
		}
	}
	var cur *turnView
	ensure := func(n int) *turnView {
		if cur == nil || cur.N != n {
			p.Turns = append(p.Turns, turnView{N: n})
			cur = &p.Turns[len(p.Turns)-1]
		}
		return cur
	}
	for i := range entries {
		e := entries[i]
		switch e.Kind {
		case store.EntrySystem:
			p.System = &entries[i]
		case store.EntryPrompt:
			p.Prompt = &entries[i]
		case store.EntryTurn:
			t := ensure(e.Turn)
			if e.InputTokens != nil && e.OutputTokens != nil {
				t.HasTokens = true
				t.In = *e.InputTokens
				if e.CacheRead != nil {
					t.In += *e.CacheRead
				}
				if e.CacheWrite != nil {
					t.In += *e.CacheWrite
				}
				t.Out = *e.OutputTokens
			}
			if e.LatencyMs != nil {
				t.Latency = humanDuration(time.Duration(*e.LatencyMs) * time.Millisecond)
			}
		case store.EntryText:
			t := ensure(e.Turn)
			t.Items = append(t.Items, turnItem{Kind: "text", Entry: e})
		case store.EntryToolCall:
			if answered[e.ToolUseID] {
				continue
			}
			item := turnItem{Kind: "tool", Entry: e, Arg: mainArg(e.Content), Result: results[e.ToolUseID]}
			if item.Result != nil && item.Result.LatencyMs != nil {
				item.Latency = humanDuration(time.Duration(*item.Result.LatencyMs) * time.Millisecond)
			}
			t := ensure(e.Turn)
			t.Items = append(t.Items, item)
		case store.EntryToolResult:
			if e.ToolUseID == "" || results[e.ToolUseID] != &entries[i] {
				t := ensure(e.Turn)
				t.Items = append(t.Items, turnItem{Kind: "tool", Entry: e, Result: &entries[i]})
			}
		case store.EntryNudge:
			t := ensure(e.Turn)
			t.Items = append(t.Items, turnItem{Kind: "nudge", Entry: e})
		case store.EntryOutcome:
			t := ensure(e.Turn)
			t.Items = append(t.Items, turnItem{Kind: "outcome", Entry: e, Arg: e.ToolName})
		case store.EntryError:
			t := ensure(e.Turn)
			t.Items = append(t.Items, turnItem{Kind: "error", Entry: e})
		}
	}
}

// mainArg picks the argument that best names a tool call on its summary line:
// a path, a command, or a search.
func mainArg(input string) string {
	var m map[string]any
	if json.Unmarshal([]byte(input), &m) != nil {
		return ""
	}
	for _, k := range []string{"path", "command", "name", "query", "pattern"} {
		if v, ok := m[k].(string); ok && v != "" {
			if len(v) > 80 {
				v = v[:80] + "…"
			}
			return v
		}
	}
	return ""
}

// conclude works out the conclusion panel (FR-3.2).
func (p *runPage) conclude(d *store.Dispatch, entries []store.TranscriptEntry) conclusion {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Kind == store.EntryError {
			return conclusion{Kind: "error", Error: entries[i].Content, Plain: failureWords(entries[i].Content)}
		}
	}
	if !p.IsLatest {
		return conclusion{Kind: "other", Raw: "This attempt ended without recording why."}
	}
	switch d.State {
	case "queued":
		return conclusion{Kind: "queued"}
	case "running":
		return conclusion{Kind: "running"}
	case "failed", "cancelled":
		msg := "The run stopped without recording why."
		if d.Error != nil && *d.Error != "" {
			msg = *d.Error
		}
		return conclusion{Kind: "error", Error: msg, Plain: failureWords(msg)}
	}
	if len(d.Outcome) == 0 {
		return conclusion{Kind: "other", Raw: "The run finished without an outcome."}
	}
	var o struct {
		Verdict   string `json:"verdict"`
		Reasoning string `json:"reasoning"`
		Comments  []struct {
			SectionRef string `json:"section_ref"`
			Body       string `json:"body"`
			Severity   string `json:"severity"`
		} `json:"comments"`
		Criteria []struct {
			ID       string `json:"id"`
			Met      bool   `json:"met"`
			Evidence string `json:"evidence"`
		} `json:"criteria"`
		Summary      string   `json:"summary"`
		FilesChanged []string `json:"files_changed"`
		Body         string   `json:"body"`
	}
	_ = json.Unmarshal(d.Outcome, &o)
	switch {
	case o.Verdict != "":
		c := conclusion{Kind: "verdict", Verdict: o.Verdict, Reasoning: o.Reasoning}
		for _, cm := range o.Comments {
			sev := cm.Severity
			if sev == "" {
				sev = "major"
			}
			c.Findings = append(c.Findings, finding{Severity: sev, Section: cm.SectionRef, Body: cm.Body})
		}
		for _, cr := range o.Criteria {
			c.Criteria = append(c.Criteria, criterion{ID: cr.ID, Met: cr.Met, Evidence: cr.Evidence})
		}
		return c
	case d.Purpose == "implement-task":
		return conclusion{Kind: "implementation", Summary: o.Summary, Files: o.FilesChanged}
	case o.Body != "":
		return conclusion{Kind: "document", Words: len(strings.Fields(o.Body))}
	case len(o.Comments) > 0 || o.Reasoning != "":
		// A comments-only review (the design reviewer): findings, no verdict.
		c := conclusion{Kind: "verdict", Reasoning: o.Reasoning}
		for _, cm := range o.Comments {
			c.Findings = append(c.Findings, finding{Severity: cm.Severity, Section: cm.SectionRef, Body: cm.Body})
		}
		return c
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, d.Outcome, "", "  ") != nil {
		return conclusion{Kind: "other", Raw: string(d.Outcome)}
	}
	return conclusion{Kind: "other", Raw: pretty.String()}
}

// prettyJSON indents a tool's JSON input for reading; anything else is shown
// as it is.
func prettyJSON(s string) string {
	var b bytes.Buffer
	if json.Indent(&b, []byte(s), "", "  ") != nil {
		return s
	}
	return b.String()
}

// ---- Review health (FR-7, FR-8) ----

type reviewHealthPage struct {
	Window    string
	Windows   []windowChoice
	Reviewers []reviewerView
	Reviews   int
	Flagged   int
	Rules     reviewRules
}

func (reviewHealthPage) headTitle() string { return "How the reviewers are doing" }
func (reviewHealthPage) headCrumbs() []crumb {
	return []crumb{{Label: "Home", URL: "/ui"}, {Label: "Reviewers", Here: true}}
}

type windowChoice struct {
	Key, Label string
	Selected   bool
}

type reviewerView struct {
	reviewhealth.Reviewer
	What     string
	Warnings []string
}

// reviewRules carries SD-9's thresholds into the page's explanation, so the
// words and the code can't drift apart.
type reviewRules struct {
	MinVerdicts  int
	HighPct      int
	LowPct       int
	FastSeconds  int
	TerseOutput  int
	FewPerReview string
}

func warningText(kind string) string {
	switch kind {
	case reviewhealth.WarnFastApprover:
		return "Approves almost everything, quickly, and says almost nothing about why. A reviewer that decides in seconds with a line of reasoning is unlikely to have read the work."
	case reviewhealth.WarnFindsNothing:
		return "Approves almost everything, and finds almost nothing, not even small things. It may be waving work through."
	case reviewhealth.WarnNeverApproves:
		return "Hardly ever approves. Work it reviews goes round and round, so check whether its findings are real."
	}
	return kind
}

func reviewerPurposes(purposes []string) string {
	var words []string
	for _, p := range purposes {
		words = append(words, strings.ToLower(runPurpose(p, "")[:1])+runPurpose(p, "")[1:])
	}
	return strings.Join(words, "; ")
}

func (s *Server) reviewHealth(ctx context.Context, window string) (*reviewHealthPage, error) {
	since := time.Time{}
	switch window {
	case "30d":
		since = time.Now().AddDate(0, 0, -30)
	case "90d":
		since = time.Now().AddDate(0, 0, -90)
	default:
		window = "all"
	}
	runs, err := store.ReviewRuns(ctx, s.Store.Pool, since)
	if err != nil {
		return nil, err
	}
	reviews := make([]reviewhealth.Review, 0, len(runs))
	for _, r := range runs {
		verdict, majors, minors := reviewhealth.FromOutcome(r.Outcome)
		var took time.Duration
		if r.StartedAt != nil && r.FinishedAt != nil {
			took = r.FinishedAt.Sub(*r.StartedAt)
		}
		reviews = append(reviews, reviewhealth.Review{Purpose: r.Purpose, Role: r.Role, Model: r.Model,
			Ref: r.RefID, Verdict: verdict, Majors: majors, Minors: minors, Duration: took,
			Tokens: r.Tokens, Output: r.Output})
	}
	page := &reviewHealthPage{
		Window: window, Reviews: len(reviews),
		Windows: []windowChoice{
			{Key: "all", Label: "All time", Selected: window == "all"},
			{Key: "90d", Label: "The last 90 days", Selected: window == "90d"},
			{Key: "30d", Label: "The last 30 days", Selected: window == "30d"},
		},
		Rules: reviewRules{
			MinVerdicts: reviewhealth.MinVerdicts,
			HighPct:     int(reviewhealth.HighApproval * 100),
			LowPct:      int(reviewhealth.LowApproval * 100),
			FastSeconds: int(reviewhealth.FastVerdict.Seconds()),
			TerseOutput: reviewhealth.TerseOutput,
			FewPerReview: "one finding in every " +
				strconv.Itoa(int(math.Round(1/float64(reviewhealth.FewFindingsPerReview)))) + " reviews",
		},
	}
	for _, r := range reviewhealth.Summarise(reviews) {
		v := reviewerView{Reviewer: r, What: reviewerPurposes(r.Purposes)}
		for _, w := range r.Warnings {
			v.Warnings = append(v.Warnings, warningText(w))
		}
		if r.Flagged() {
			page.Flagged++
		}
		page.Reviewers = append(page.Reviewers, v)
	}
	return page, nil
}

func (s *Server) handleUIReviewHealth(w http.ResponseWriter, r *http.Request) {
	page, err := s.reviewHealth(r.Context(), r.URL.Query().Get("window"))
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "page-review-health", s.page(r.Context(), "home", page))
}

func (s *Server) handleFragReviewHealth(w http.ResponseWriter, r *http.Request) {
	page, err := s.reviewHealth(r.Context(), "all")
	if err != nil {
		s.uiError(w, err)
		return
	}
	s.render(w, "frag-review-health", page)
}

// The template helpers these pages add. They join uiFuncs before the
// templates are parsed (loadUITemplates runs at server start, after init).
// None formats money (D-4).
func init() {
	for name, fn := range map[string]any{
		"verdictLabel": verdictLabel,
		"verdictState": verdictState,
		"humanBytes":   humanBytes,
		"prettyJSON":   prettyJSON,
		"groupNum":     groupThousands,
		"dur":          humanDuration,
		"f1":           func(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) },
		"failureWords": failureWords,
		"progressOf": func(p *runPage) runProgress {
			n := 0
			for _, t := range p.Turns {
				if t.N > 0 {
					n++
				}
			}
			return runProgress{ID: p.Run.ID, State: p.Run.State, Turns: n, Shown: n}
		},
	} {
		uiFuncs[name] = fn
	}
}
