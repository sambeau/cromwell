package timeline

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var t0 = time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

func ev(min int, kind string, payload map[string]any) Event {
	return Event{ID: uuid.New(), At: at(min), Actor: "orchestrator", Kind: kind, Payload: payload}
}

func docEv(min int, kind, docType string, doc uuid.UUID, payload map[string]any) Event {
	e := ev(min, kind, payload)
	e.RefType, e.RefID, e.DocType = "document", doc, docType
	return e
}

func labels(ms []Moment) string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Label)
	}
	return strings.Join(out, " · ")
}

// TestTimelineMapping is FR-5.2 and FR-5.3 over synthetic events: the table
// picks the major moments, counts rounds, attributes people, and leaves the
// rest as detail.
func TestTimelineMapping(t *testing.T) {
	spec, plan, task := uuid.New(), uuid.New(), uuid.New()
	human := ev(9, "document.transition", map[string]any{"event": "approve"})
	human.RefType, human.RefID, human.DocType, human.Actor = "document", plan, "dev_plan", "sam"
	taskBack := ev(14, "task.transition", map[string]any{"event": "request_changes"})
	taskBack.RefType, taskBack.RefID, taskBack.Label = "task", task, "Greeting helper"
	taskBack2 := taskBack
	taskBack2.At = at(15)

	events := []Event{
		ev(0, "feature.created", nil),
		ev(1, "feature.sent", nil),
		docEv(2, "document.registered", "spec", spec, map[string]any{"type": "spec"}),
		docEv(3, "document.transition", "spec", spec, map[string]any{"event": "submit"}),
		docEv(4, "document.transition", "spec", spec, map[string]any{"event": "request_changes"}),
		docEv(5, "document.transition", "spec", spec, map[string]any{"event": "approve"}),
		ev(5, "gate.evaluated", nil),
		docEv(6, "document.registered", "dev_plan", plan, nil),
		human,
		ev(10, "devplan.decomposed", map[string]any{"created": 3.0}),
		ev(11, "feature.transition", map[string]any{"event": "contract_approved"}),
		ev(12, "feature.transition", map[string]any{"event": "start"}),
		taskBack,
		taskBack2,
		ev(16, "checkpoint.created", map[string]any{"kind": "review-deadlock"}),
		ev(17, "checkpoint.responded", map[string]any{"response": map[string]any{"decision": "approve"}}),
		ev(18, "feature.transition", map[string]any{"event": "tasks_complete"}),
		ev(19, "feature.transition", map[string]any{"event": "verified"}),
	}
	events[16].Actor = "sam"

	ms := Build(events, nil, Options{Progress: Progress{Done: 2, Total: 3}})
	want := "Created · Sent to development · Spec written · Spec sent back (round 1) · Spec approved · " +
		"Plan written · Plan approved · Broken into 3 tasks · Ready to build · Building (2 of 3 tasks done) · " +
		"Code review sent back “Greeting helper” (round 1) · Code review sent back “Greeting helper” (round 2) · " +
		"Waiting for a person: review keeps finding problems · A person decided: approve · Verifying · Done"
	if got := labels(ms); got != want {
		t.Fatalf("moments:\n got %s\nwant %s", got, want)
	}
	// The submit and the gate check are detail, in the phase they fall in.
	if len(ms[2].Events) != 2 || ms[2].Events[1].Str("event") != "submit" {
		t.Errorf("Spec written's phase should hold the submit: %+v", ms[2].Events)
	}
	if len(ms[4].Events) != 2 || ms[4].Events[1].Kind != "gate.evaluated" {
		t.Errorf("Spec approved's phase should hold the gate check: %+v", ms[4].Events)
	}
	if ms[6].Actor != "sam" || ms[4].Actor != "" {
		t.Errorf("people are named, the orchestrator is not: %q %q", ms[6].Actor, ms[4].Actor)
	}

	// Without M3's event there is simply no such moment.
	noSend := append(append([]Event{}, events[:1]...), events[2:]...)
	if got := labels(Build(noSend, nil, Options{})); strings.Contains(got, "Sent to development") {
		t.Errorf("no sent event, no moment: %s", got)
	}
}

// TestTimelineNewKindIsOneEntry is FR-5.3: registering a new kind of event is
// one table entry.
func TestTimelineNewKindIsOneEntry(t *testing.T) {
	saved := Rules
	t.Cleanup(func() { Rules = saved })
	Rules = append(Rules, Rule{Kind: "feature.held", Make: fixed("Held for a person", "review", "clock")})

	ms := Build([]Event{ev(0, "feature.created", nil), ev(1, "feature.held", nil)}, nil, Options{})
	if got := labels(ms); got != "Created · Held for a person" {
		t.Errorf("got %s", got)
	}
}

// TestTimelinePhases is FR-5.4 and SD-6: a run belongs to the phase current
// when it was queued; runs before the first moment belong to it.
func TestTimelinePhases(t *testing.T) {
	events := []Event{
		ev(1, "feature.created", nil),
		ev(10, "feature.transition", map[string]any{"event": "start"}),
		ev(20, "feature.transition", map[string]any{"event": "tasks_complete"}),
	}
	runs := []Run{
		{Purpose: "verify-feature", QueuedAt: at(21)},
		{Purpose: "write-spec", QueuedAt: at(0)},
		{Purpose: "implement-task", QueuedAt: at(11)},
		{Purpose: "review-code", QueuedAt: at(12)},
	}
	ms := Build(events, runs, Options{Progress: Progress{Done: 1, Total: 1}})
	purposes := func(m Moment) string {
		var p []string
		for _, r := range m.Runs {
			p = append(p, r.Purpose)
		}
		return strings.Join(p, ",")
	}
	if got := purposes(ms[0]); got != "write-spec" {
		t.Errorf("Created holds %s", got)
	}
	if got := purposes(ms[1]); got != "implement-task,review-code" {
		t.Errorf("Building holds %s", got)
	}
	if got := purposes(ms[2]); got != "verify-feature" {
		t.Errorf("Verifying holds %s", got)
	}
}

// TestTimelineLine is FR-6.1: repeated moments fold into one mark with a
// count, and the last mark is the current phase.
func TestTimelineLine(t *testing.T) {
	back := "Code review sent back"
	ms := []Moment{
		{Label: "Created"}, {Label: "Building (1 of 2 tasks done)"},
		{Label: "Code review sent back “A” (round 1)", Fold: back}, {Label: "Code review sent back “B” (round 1)", Fold: back},
		{Label: "Code review sent back “A” (round 2)", Fold: back}, {Label: "Verifying"},
	}
	line := Line(ms)
	if len(line) != 4 || line[2].Label != "Code review sent back" || line[2].Count != 3 || line[2].Index != 2 {
		t.Fatalf("line = %+v", line)
	}
	if !line[3].Current || line[2].Current {
		t.Error("only the last mark is current")
	}
	if Build(nil, nil, Options{}) != nil {
		t.Error("nothing happened, no moments")
	}
	// A single foldable moment keeps its full label.
	one := Line(ms[1:3])
	if one[1].Label != "Code review sent back “A” (round 1)" || one[1].Count != 1 {
		t.Errorf("a lone moment keeps its label: %+v", one[1])
	}
}

// TestTimelineCauses is FR-5.5: a verdict moment leads to the run that caused
// it, not only the runs in its phase; a person's act has no agent cause; the
// chat agent is attributed as itself.
func TestTimelineCauses(t *testing.T) {
	spec, task := uuid.New(), uuid.New()
	fin := func(m int) *time.Time { t := at(m); return &t }
	review1 := Run{ID: uuid.New(), Purpose: "review-spec", Role: "spec-reviewer", State: "succeeded", RefType: "document", RefID: spec, QueuedAt: at(2), FinishedAt: fin(3)}
	review2 := Run{ID: uuid.New(), Purpose: "review-spec", Role: "spec-reviewer", State: "succeeded", RefType: "document", RefID: spec, QueuedAt: at(5), FinishedAt: fin(6)}
	codeReview := Run{ID: uuid.New(), Purpose: "review-code", Role: "code-reviewer", State: "succeeded", RefType: "task", RefID: task, QueuedAt: at(9), FinishedAt: fin(10)}
	failed := Run{ID: uuid.New(), Purpose: "implement-task", Role: "implementer", State: "failed", RefType: "task", RefID: task, QueuedAt: at(12), FinishedAt: fin(13)}

	back := docEv(4, "document.transition", "spec", spec, map[string]any{"event": "request_changes"})
	back.Actor = "spec-reviewer"
	approve := docEv(7, "document.transition", "spec", spec, map[string]any{"event": "approve"})
	approve.Actor = "spec-reviewer"
	taskBack := ev(11, "task.transition", map[string]any{"event": "request_changes"})
	taskBack.RefType, taskBack.RefID, taskBack.Actor = "task", task, "code-reviewer"
	stuck := ev(14, "checkpoint.created", map[string]any{"kind": "dispatch-failure"})
	stuck.RefType, stuck.RefID = "task", task
	answered := ev(15, "checkpoint.responded", map[string]any{"response": map[string]any{"retry": true}})
	answered.RefType, answered.RefID, answered.Actor = "task", task, "sam"
	chat := ev(16, "feature.sent", nil)
	chat.Actor = "chat-agent"

	ms := Build([]Event{ev(0, "feature.created", nil), back, approve, taskBack, stuck, answered, chat},
		[]Run{review1, review2, codeReview, failed}, Options{ChatActor: "chat-agent"})
	want := []struct {
		by    string
		cause uuid.UUID
	}{
		{BySystem, uuid.Nil}, {ByAgent, review1.ID}, {ByAgent, review2.ID}, {ByAgent, codeReview.ID},
		{ByAgent, failed.ID}, {ByPerson, uuid.Nil}, {ByChat, uuid.Nil},
	}
	for i, w := range want {
		got := uuid.Nil
		if ms[i].Cause != nil {
			got = ms[i].Cause.ID
		}
		if ms[i].By != w.by || got != w.cause {
			t.Errorf("%s: by %s cause %v, want %s %v", ms[i].Label, ms[i].By, got, w.by, w.cause)
		}
	}
	if ms[5].Actor != "sam" || ms[6].Actor != "chat-agent" {
		t.Error("people and the chat agent are named")
	}
}

// TestTimelinePlacesRunsByTheirQueueRow is SD-6's tie rule: a run queued in
// the same transaction as a moment (same timestamp, later id) belongs to that
// moment, and a retry stays where the run was first queued.
func TestTimelinePlacesRunsByTheirQueueRow(t *testing.T) {
	task := uuid.New()
	impl := Run{ID: uuid.New(), Purpose: "implement-task", QueuedAt: at(30)} // requeued much later
	back := ev(10, "task.transition", map[string]any{"event": "request_changes"})
	back.RefType, back.RefID = "task", task
	queued := ev(10, "dispatch.queued", map[string]any{"dispatch_id": impl.ID.String()})
	requeued := ev(20, "dispatch.queued", map[string]any{"dispatch_id": impl.ID.String()})
	events := []Event{
		ev(0, "feature.created", nil),
		ev(5, "feature.transition", map[string]any{"event": "start"}),
		back, queued,
		ev(15, "feature.transition", map[string]any{"event": "tasks_complete"}),
		requeued,
	}
	ms := Build(events, []Run{impl}, Options{})
	if len(ms[2].Runs) != 1 || ms[2].Runs[0].ID != impl.ID {
		t.Errorf("the implementer re-dispatched with the sendback belongs to it: %s holds %d", ms[2].Label, len(ms[2].Runs))
	}
	if len(ms[3].Runs) != 0 {
		t.Error("a later queue row doesn't move the run")
	}
}

// TestRelayedVerdictIsAPersons is SPEC-017 FR-2.7: an approval the chat agent
// relayed is the person's, not the chat agent's; the chat agent's own acts
// stay its own.
func TestRelayedVerdictIsAPersons(t *testing.T) {
	spec := uuid.New()
	written := docEv(0, "document.registered", "spec", spec, nil)
	written.Actor = "chat-agent"
	relayed := docEv(1, "document.transition", "spec", spec,
		map[string]any{"event": "approve", "verdict_by": "person", "via": "mcp"})
	relayed.Actor = "chat-agent"
	ms := Build([]Event{written, relayed}, nil, Options{ChatActor: "chat-agent"})
	if len(ms) != 2 {
		t.Fatalf("moments = %s", labels(ms))
	}
	if ms[0].By != ByChat || ms[0].Relayed {
		t.Errorf("the chat agent's own act: by %s, relayed %v", ms[0].By, ms[0].Relayed)
	}
	if ms[1].By != ByPerson || !ms[1].Relayed || ms[1].Actor != "" {
		t.Errorf("a relayed approval: by %s, relayed %v, actor %q", ms[1].By, ms[1].Relayed, ms[1].Actor)
	}
}

// TestClaimMoments is SPEC-020 FR-1.7: a claim, its submission and its release
// are moments attributed from the payload's kind; renewals, resumptions and
// the claim's ending are detail.
func TestClaimMoments(t *testing.T) {
	task := uuid.New()
	claim := func(min int, kind, actor, label string, p map[string]any) Event {
		e := ev(min, kind, p)
		e.Actor, e.RefType, e.RefID, e.Label = actor, "task", task, label
		return e
	}
	events := []Event{
		claim(0, "claim.claimed", "chat-agent", "Add login", map[string]any{"kind": "chat", "via": "mcp"}),
		claim(1, "claim.renewed", "chat-agent", "Add login", map[string]any{"kind": "chat"}),
		claim(2, "claim.submitted", "chat-agent", "Add login", map[string]any{"kind": "chat"}),
		claim(3, "claim.released", "sam", "Add login", map[string]any{"kind": "chat"}),
		claim(4, "claim.claimed", "sam", "Add login", map[string]any{"kind": "person", "via": "ui"}),
		claim(5, "claim.resumed", "sam", "Add login", map[string]any{"kind": "person"}),
		claim(6, "claim.ended", "orchestrator", "Add login", map[string]any{"kind": "person"}),
	}
	ms := Build(events, nil, Options{ChatActor: "chat-agent"})
	want := "Claimed by the chat agent: Add login · Submitted by the chat agent: Add login · " +
		"Released by sam: Add login · Claimed by sam: Add login"
	if got := labels(ms); got != want {
		t.Fatalf("moments:\n got %s\nwant %s", got, want)
	}
	for i, by := range []string{ByChat, ByChat, ByPerson, ByPerson} {
		if ms[i].By != by {
			t.Errorf("moment %d (%s) is by %s, want %s", i, ms[i].Label, ms[i].By, by)
		}
	}
	if ms[3].Actor != "sam" || ms[0].Actor != "" {
		t.Errorf("a person is named and the chat agent isn't: %q, %q", ms[3].Actor, ms[0].Actor)
	}
	if ms[0].Icon != "chat" || ms[3].Icon != "owner" {
		t.Errorf("icons: %s, %s", ms[0].Icon, ms[3].Icon)
	}
	if len(ms[3].Events) != 3 {
		t.Errorf("the resumption and the ending stay as detail: %d events", len(ms[3].Events))
	}
}
