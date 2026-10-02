// Package timeline turns a feature's audit stream into its journey: a short
// line of major moments, each opening a phase that holds the runs and events
// that happened in it (SPEC-012 FR-5, DESIGN-010 §8).
//
// Everything here is pure — no store, no clock — so the mapping is
// table-testable. Which events become moments is decided by one table, Rules:
// a new kind of event needs one entry and nothing else (FR-5.3).
package timeline

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Event is one audit row about the feature, its documents or its tasks.
type Event struct {
	ID      uuid.UUID
	At      time.Time
	Actor   string
	Kind    string
	RefType string
	RefID   uuid.UUID
	Payload map[string]any
	// DocType is the document's type when the event is about a document, and
	// Label its title or the task's title.
	DocType string
	Label   string
}

// Str reads a string field from the payload, or "".
func (e Event) Str(key string) string {
	if v, ok := e.Payload[key].(string); ok {
		return v
	}
	return ""
}

// Num reads a number field from the payload, or 0.
func (e Event) Num(key string) int {
	if v, ok := e.Payload[key].(float64); ok {
		return int(v)
	}
	return 0
}

// ParsePayload decodes a raw audit payload; a malformed one reads as empty.
func ParsePayload(raw []byte) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	return m
}

// Run is one agent run on the feature, its documents or its tasks.
type Run struct {
	ID         uuid.UUID
	Purpose    string
	Role       string
	Model      string
	State      string
	RefType    string
	RefID      uuid.UUID
	Label      string // the document's or task's title
	Attempt    int
	QueuedAt   time.Time
	FinishedAt *time.Time
	Tokens     int64
	Verdict    string // from the outcome, when it has one
}

// Progress is the feature's task count as it is now, for "Building (x of y)".
type Progress struct {
	Done  int
	Total int
}

// Moment is one step of the journey and the phase it opens.
type Moment struct {
	Label string
	// State selects the mark and hue, from the lifecycle family the pages
	// already use: idea, ready, active, review, done, abandoned, draft.
	State string
	Icon  string // sprite symbol, without the "i-" prefix
	At    time.Time
	// Fold is what the one-line view shows when several moments of the same
	// kind come in a row ("Code review sent back ×3").
	Fold string
	// Aside is set for a moment beside the journey, not on it (a spike): the
	// feature's later runs and events are not grouped under it.
	Aside bool

	// By says who caused the moment (DESIGN-010 §8, "everything is
	// attributed"): ByAgent, with Cause naming the run; ByPerson or ByChat,
	// with Actor naming them; or BySystem, for the orchestrator's own steps.
	By    string
	Actor string
	// Relayed marks a person's verdict the chat agent carried, with their
	// words (SPEC-017 SD-5): By is ByPerson, and the person is unnamed until
	// per-user identity exists.
	Relayed bool
	Cause   *Run
	RefType string
	RefID   uuid.UUID
	RefName string

	Runs   []Run   // runs queued in this phase (SD-6)
	Events []Event // every audit event in this phase, the detail level (SD-7)
}

// Who caused a moment.
const (
	BySystem = "system"
	ByAgent  = "agent"
	ByPerson = "person"
	ByChat   = "chat"
)

// Context is what a rule may read beyond the event itself.
type Context struct {
	Progress Progress
	// rounds counts sendbacks per document or task, so each moment can say
	// which round it was.
	rounds map[uuid.UUID]int
}

// Round records one more sendback of ref and returns its number.
func (c *Context) Round(ref uuid.UUID) int {
	c.rounds[ref]++
	return c.rounds[ref]
}

// Rule maps one kind of event to a moment. When is optional; a nil When
// matches every event of the kind. Make returns the label, the state and the
// icon. Fold, if set, is the label the one-line view uses for a run of such
// moments; "{doc}" in it becomes the document's kind. CausedBy lists the purposes of the runs that can cause the moment
// (a prefix such as "review-" matches all of them; "*" matches any run, even
// one that failed): the cause is the latest such run on the same document or
// task, or on the feature, that finished at or before the event (FR-5.5).
type Rule struct {
	Kind     string
	When     func(Event) bool
	Make     func(Event, *Context) (label, state, icon string)
	Fold     string
	CausedBy []string
	// Agentive, if set, says an event was an agent's act even though its
	// actor ran no run on this feature: a bug an agent filed while working
	// on another (SPEC-019 FR-5.3).
	Agentive func(Event) bool
	// Aside marks a moment that is told beside the feature's journey and
	// does not become part of it: it groups no runs or events and is never
	// where the feature is now (a spike, SPEC-021 FR-8.3).
	Aside bool
}

func reportedByAgent(e Event) bool {
	k := e.Str("reporter_kind")
	return k == "agent" || k == "review"
}

func fixed(label, state, icon string) func(Event, *Context) (string, string, string) {
	return func(Event, *Context) (string, string, string) { return label, state, icon }
}

func docIs(types ...string) func(Event) bool {
	return func(e Event) bool {
		for _, t := range types {
			if e.DocType == t {
				return true
			}
		}
		return false
	}
}

func and(a, b func(Event) bool) func(Event) bool {
	return func(e Event) bool { return a(e) && b(e) }
}

func field(key, value string) func(Event) bool {
	return func(e Event) bool { return e.Str(key) == value }
}

// docNoun names a document type the way a person says it.
func docNoun(docType string) string {
	switch docType {
	case "spec":
		return "Spec"
	case "dev_plan":
		return "Plan"
	case "design":
		return "Design"
	case "bug_report":
		// A bug's report is its spec (SPEC-019 FR-5.3).
		return "Report"
	default:
		return "Document"
	}
}

var plannedDocs = docIs("design", "spec", "dev_plan", "bug_report")

// writtenDocs are the documents whose writing is a moment. A bug's report is
// written as it is reported, so "Reported" already says it.
var writtenDocs = docIs("design", "spec", "dev_plan")

func not(f func(Event) bool) func(Event) bool { return func(e Event) bool { return !f(e) } }

func has(key string) func(Event) bool {
	return func(e Event) bool { _, ok := e.Payload[key]; return ok }
}

// Rules is the mapping table (FR-5.2). Order matters only within one kind:
// the first rule whose When matches decides.
var Rules = []Rule{
	{Kind: "feature.created", When: not(field("kind", "bug")), Make: fixed("Created", "idea", "feature")},

	// A bug's first moments (SPEC-019 FR-5.3): who reported it, and how a
	// person triaged it. Rejecting or marking a duplicate abandons the row in
	// the same act, so that abandonment isn't a second moment.
	{Kind: "bug.reported", Agentive: reportedByAgent,
		Make: func(e Event, _ *Context) (string, string, string) {
			switch e.Str("reporter_kind") {
			case "review":
				return "Reported from a code review's minor findings", "idea", "bug"
			case "agent":
				return "Reported by an agent at work", "idea", "bug"
			}
			return "Reported", "idea", "bug"
		}},
	{Kind: "bug.triaged", Make: func(e Event, _ *Context) (string, string, string) {
		switch e.Str("decision") {
		case "accepted":
			return "Accepted in triage", "ready", "approve"
		case "duplicate":
			return "Marked as a duplicate in triage", "abandoned", "state-abandoned"
		}
		return "Rejected in triage", "abandoned", "request-changes"
	}},

	// A spike on the feature (SPEC-021 FR-8.3): when it starts and when its
	// run ends. The event's label is the spike's ID. The feature's own
	// pipeline goes on as it was, so these moments are asides: they say what
	// happened, in a neutral state, and take no runs or events from the
	// feature; a spike's tokens are its own and never join the feature's.
	{Kind: "spike.started", Aside: true, Make: func(e Event, _ *Context) (string, string, string) {
		return "Spike " + e.Label + " started", "idea", "spike"
	}},
	{Kind: "spike.ended", Aside: true, Make: func(e Event, _ *Context) (string, string, string) {
		return "Spike " + e.Label + " ended: " + spikeEndWords(e.Str("how")), "idea", "spike"
	}},

	// M3's "sent to development" event (DEC-006). Nothing emits it yet; a
	// feature without one simply has no such moment (FR-5.3).
	{Kind: "feature.sent", Make: fixed("Sent to development", "active", "start")},

	{Kind: "document.registered", When: writtenDocs, CausedBy: []string{"write-"},
		Make: func(e Event, _ *Context) (string, string, string) {
			return docNoun(e.DocType) + " written", "draft", "edit"
		}},
	{Kind: "document.revision_created", When: plannedDocs, CausedBy: []string{"write-"},
		Make: func(e Event, _ *Context) (string, string, string) {
			return docNoun(e.DocType) + " revised", "draft", "edit"
		}},
	{Kind: "document.transition", When: and(plannedDocs, field("event", "request_changes")),
		Fold: "{doc} sent back", CausedBy: []string{"review-"},
		Make: func(e Event, c *Context) (string, string, string) {
			return fmt.Sprintf("%s sent back (round %d)", docNoun(e.DocType), c.Round(e.RefID)), "abandoned", "request-changes"
		}},
	{Kind: "document.transition", When: and(plannedDocs, field("event", "approve")), CausedBy: []string{"review-"},
		Make: func(e Event, _ *Context) (string, string, string) {
			return docNoun(e.DocType) + " approved", "done", "approve"
		}},

	{Kind: "devplan.decomposed", Make: func(e Event, _ *Context) (string, string, string) {
		n := e.Num("created")
		if n == 0 {
			return "Tasks updated from the plan", "ready", "children"
		}
		return fmt.Sprintf("Broken into %d %s", n, plural(n, "task", "tasks")), "ready", "children"
	}},

	{Kind: "feature.transition", When: field("event", "contract_approved"), Make: fixed("Ready to build", "ready", "state-ready")},
	{Kind: "feature.transition", When: field("event", "start"), Make: func(_ Event, c *Context) (string, string, string) {
		return fmt.Sprintf("Building (%d of %d %s done)", c.Progress.Done, c.Progress.Total, plural(c.Progress.Total, "task", "tasks")), "active", "state-active"
	}},
	{Kind: "feature.transition", When: field("event", "tasks_complete"), CausedBy: []string{"review-code"},
		Make: fixed("Verifying", "review", "state-review")},
	{Kind: "feature.transition", When: field("event", "rework"), CausedBy: []string{"verify-feature"},
		Make: fixed("Verification sent it back", "abandoned", "request-changes")},
	{Kind: "feature.transition", When: field("event", "verified"), CausedBy: []string{"verify-feature"},
		Make: fixed("Done", "done", "state-done")},
	{Kind: "feature.transition", When: and(field("event", "abandon"), not(has("triage"))), Make: fixed("Abandoned", "abandoned", "state-abandoned")},
	{Kind: "feature.transition", When: field("event", "contract_invalidated"),
		Make: fixed("Back to an idea, because its design changed", "idea", "state-idea")},
	{Kind: "feature.spec_stale", Make: fixed("Spec out of date", "idea", "alert")},

	{Kind: "task.transition", When: field("event", "request_changes"),
		Fold: "Code review sent back", CausedBy: []string{"review-code"},
		Make: func(e Event, c *Context) (string, string, string) {
			return fmt.Sprintf("Code review sent back %s (round %d)", quoted(e.Label), c.Round(e.RefID)), "abandoned", "request-changes"
		}},

	{Kind: "checkpoint.created", Fold: "Waiting for a person", CausedBy: []string{"*"},
		Make: func(e Event, _ *Context) (string, string, string) {
			return "Waiting for a person: " + checkpointAbout(e.Str("kind")), "review", "question"
		}},
	{Kind: "checkpoint.responded", Fold: "A person decided",
		Make: func(e Event, _ *Context) (string, string, string) {
			return "A person decided: " + answerWords(e.Payload["response"]), "done", "check"
		}},
}

// spikeEndWords says how a spike's run ended, to follow "Spike SPK-003 ended: ".
func spikeEndWords(how string) string {
	switch how {
	case "concluded":
		return "it reached a conclusion"
	case "budget":
		return "it stopped at its budget"
	case "turn_limit":
		return "it stopped at its turn limit"
	case "failed":
		return "its run failed"
	}
	return "its run is over"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func quoted(s string) string {
	if s == "" {
		return "a task"
	}
	return "“" + s + "”"
}

// checkpointAbout names what a checkpoint asks a person about.
func checkpointAbout(kind string) string {
	switch kind {
	case "review-escalation":
		return "a reviewer escalated"
	case "verification-escalation":
		return "the verifier escalated"
	case "review-deadlock":
		return "review keeps finding problems"
	case "dispatch-failure":
		return "an agent run keeps failing"
	case "merge-conflict":
		return "the merge conflicts"
	case "budget":
		return "the spending limit was reached"
	case "config-error":
		return "the configuration has an error"
	case "design-revision":
		return "a revised design affects this"
	case "revision-in-flight":
		return "a document changed while work was running"
	case "document-integrity":
		return "a document changed unexpectedly"
	case "gate-override":
		return "a rule blocked progress"
	case "worktree-failure":
		return "the working copy could not be made"
	case "":
		return "a question"
	default:
		return "a question (" + kind + ")"
	}
}

// answerWords says what a person answered, from the response payload that
// the inbox writes (responseFor): a decision, a yes or no, or an answer verb.
func answerWords(v any) string {
	m, _ := v.(map[string]any)
	if d, ok := m["decision"].(string); ok {
		switch d {
		case "approve":
			return "approve"
		case "request_changes":
			return "ask for changes"
		case "abandon":
			return "stop work"
		default:
			return d
		}
	}
	if b, ok := m["retry"].(bool); ok {
		if b {
			return "try again"
		}
		return "cancel"
	}
	if b, ok := m["override"].(bool); ok {
		if b {
			return "override the rule"
		}
		return "keep the rule"
	}
	if b, ok := m["continue"].(bool); ok {
		if b {
			return "carry on"
		}
		return "pause"
	}
	if a, ok := m["answer"].(string); ok && a != "" {
		return a
	}
	return "answered"
}

// Options carries what Build needs beyond the stream itself.
type Options struct {
	Progress Progress
	// ChatActor is the configured identity the chat agent acts as, so its
	// acts are attributed to it rather than to a person.
	ChatActor string
}

// Build turns the feature's events and runs into its moments (FR-5.4).
// Events must be in the order they happened, ties broken by id, as the store
// returns them; runs may be in any order.
//
// Each run belongs to the phase current when it was queued, found from its
// first dispatch.queued audit row, so a run queued in the same transaction as
// a moment belongs to that moment, and a retry stays where the run began.
// Runs and events before the first moment belong to the first moment.
func Build(events []Event, runs []Run, opt Options) []Moment {
	ctx := &Context{Progress: opt.Progress, rounds: map[uuid.UUID]int{}}
	roles := map[string]bool{}
	for _, r := range runs {
		roles[r.Role] = true
	}

	var moments []Moment
	momentAt := []int{}             // the event index of each moment
	lastMain := -1                  // the last moment that is not an aside
	queuedAt := map[uuid.UUID]int{} // a run's first dispatch.queued event index
	var before []Event
	for i, e := range events {
		if e.Kind == "dispatch.queued" {
			if id, err := uuid.Parse(e.Str("dispatch_id")); err == nil {
				if _, seen := queuedAt[id]; !seen {
					queuedAt[id] = i
				}
			}
		}
		rule, ok := match(e)
		if !ok {
			if lastMain < 0 {
				before = append(before, e)
			} else {
				m := &moments[lastMain]
				m.Events = append(m.Events, e)
			}
			continue
		}
		label, state, icon := rule.Make(e, ctx)
		m := Moment{Label: label, State: state, Icon: icon, At: e.At, Fold: rule.Fold, Aside: rule.Aside,
			RefType: e.RefType, RefID: e.RefID, RefName: e.Label}
		m.Fold = strings.ReplaceAll(m.Fold, "{doc}", docNoun(e.DocType))
		if m.Fold == "" {
			m.Fold = label
		}
		switch {
		case rule.Agentive != nil && rule.Agentive(e):
			m.By = ByAgent
		case e.Actor == "orchestrator" || e.Actor == "subutai" || roles[e.Actor]:
			m.By = BySystem
			if c := cause(rule, e, runs); c != nil {
				m.By, m.Cause = ByAgent, c
			}
		case opt.ChatActor != "" && e.Actor == opt.ChatActor && e.Str("via") == "mcp" && e.Str("verdict_by") == "person":
			// A relayed verdict is the person's, not the chat agent's.
			m.By, m.Relayed = ByPerson, true
		case opt.ChatActor != "" && e.Actor == opt.ChatActor:
			m.By, m.Actor = ByChat, e.Actor
		default:
			m.By, m.Actor = ByPerson, e.Actor
		}
		m.Events = append(m.Events, e)
		moments = append(moments, m)
		momentAt = append(momentAt, i)
		if !rule.Aside {
			lastMain = len(moments) - 1
		}
	}
	if len(moments) == 0 {
		if len(events) == 0 && len(runs) == 0 {
			return nil
		}
		// Nothing major has happened; one moment holds the detail.
		at := time.Time{}
		if len(events) > 0 {
			at = events[0].At
		}
		moments = append(moments, Moment{Label: "Started", Fold: "Started", State: "idea", Icon: "feature", At: at, By: BySystem})
		momentAt = append(momentAt, -1)
	}
	first := 0 // events before the first moment on the journey belong to it
	for first < len(moments)-1 && moments[first].Aside {
		first++
	}
	moments[first].Events = append(before, moments[first].Events...)

	sorted := append([]Run(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].QueuedAt.Before(sorted[j].QueuedAt) })
	for _, r := range sorted {
		k := 0
		if qi, ok := queuedAt[r.ID]; ok {
			for j, mi := range momentAt {
				if mi <= qi && !moments[j].Aside {
					k = j
				}
			}
		} else {
			// No queue row in the stream (an older run): fall back to time.
			for j := range moments {
				if !moments[j].At.After(r.QueuedAt) && !moments[j].Aside {
					k = j
				}
			}
		}
		if moments[k].Aside {
			k = first // queued before any moment on the journey
		}
		moments[k].Runs = append(moments[k].Runs, r)
	}
	return moments
}

// CurrentIndex is the moment that says where the feature is now: the last one
// on the journey, not an aside. When every moment is an aside it is the last.
func CurrentIndex(moments []Moment) int {
	for i := len(moments) - 1; i >= 0; i-- {
		if !moments[i].Aside {
			return i
		}
	}
	return len(moments) - 1
}

func match(e Event) (Rule, bool) {
	for _, r := range Rules {
		if r.Kind != e.Kind {
			continue
		}
		if r.When != nil && !r.When(e) {
			continue
		}
		return r, true
	}
	return Rule{}, false
}

// cause finds the run that led to a moment (FR-5.5): the latest run of one of
// the rule's purposes, on the event's own document or task or on the feature,
// that finished at or before the event. Outside "*", only runs that succeeded
// count, since only a finished answer moves work on.
func cause(rule Rule, e Event, runs []Run) *Run {
	if len(rule.CausedBy) == 0 {
		return nil
	}
	var best *Run
	for i := range runs {
		r := &runs[i]
		if r.FinishedAt == nil || r.FinishedAt.After(e.At) {
			continue
		}
		if r.RefID != e.RefID && r.RefType != "feature" {
			continue
		}
		if !purposeMatches(rule.CausedBy, r.Purpose) {
			continue
		}
		if !(len(rule.CausedBy) == 1 && rule.CausedBy[0] == "*") && r.State != "succeeded" {
			continue
		}
		if best == nil || r.FinishedAt.After(*best.FinishedAt) {
			best = r
		}
	}
	if best == nil {
		return nil
	}
	c := *best
	return &c
}

func purposeMatches(patterns []string, purpose string) bool {
	for _, p := range patterns {
		if p == "*" || p == purpose || (strings.HasSuffix(p, "-") && strings.HasPrefix(purpose, p)) {
			return true
		}
	}
	return false
}

// Strip is one mark on the one-line view (FR-6.1): consecutive moments of the
// same kind are shown once, by their fold label, with a count.
type Strip struct {
	Label   string
	State   string
	Icon    string
	Count   int
	Index   int // the first moment it stands for, for the anchor
	Current bool
	// Aside is set for a mark beside the journey, shown more quietly.
	Aside bool
	last  int // the last moment it stands for
}

// Line folds the moments into the one-line view. The last mark is where the
// feature is now, unless asides follow it: the mark of the last moment on the
// journey is.
func Line(moments []Moment) []Strip {
	var out []Strip
	for i, m := range moments {
		fold := m.Fold
		if fold == "" {
			fold = m.Label
		}
		if n := len(out); n > 0 && moments[out[n-1].Index].Fold == fold && fold != m.Label {
			out[n-1].Count++
			out[n-1].Label = fold
			out[n-1].last = i
			continue
		}
		out = append(out, Strip{Label: m.Label, State: m.State, Icon: m.Icon, Count: 1, Index: i, Aside: m.Aside, last: i})
	}
	cur := CurrentIndex(moments)
	for i := range out {
		out[i].Current = out[i].last == cur
	}
	return out
}
