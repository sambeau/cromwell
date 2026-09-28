package timeline

import (
	"testing"

	"github.com/google/uuid"
)

// TestTimelineOfABug is SPEC-019 FR-5.3: a bug starts with who reported it and
// how it was triaged; its report's moments read "Report …"; an abandonment
// that triage caused isn't a second moment; an agent's report is the agent's.
func TestTimelineOfABug(t *testing.T) {
	rep := uuid.New()
	created := ev(0, "feature.created", map[string]any{"kind": "bug"})
	reported := ev(0, "bug.reported", map[string]any{"reporter_kind": "agent"})
	reported.Actor = "implementer"
	events := []Event{
		created, reported,
		docEv(0, "document.registered", "bug_report", rep, nil),
		ev(1, "bug.triaged", map[string]any{"decision": "accepted", "via": "ui"}),
		ev(2, "feature.sent", nil),
		docEv(3, "document.transition", "bug_report", rep, map[string]any{"event": "approve"}),
	}
	ms := Build(events, nil, Options{})
	if got, want := labels(ms), "Reported by an agent at work · Accepted in triage · Sent to development · Report approved"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if ms[0].By != ByAgent {
		t.Errorf("an agent's report is the agent's act; By = %s", ms[0].By)
	}

	rejected := Build([]Event{
		ev(0, "bug.reported", map[string]any{"reporter_kind": "person"}),
		ev(1, "bug.triaged", map[string]any{"decision": "rejected"}),
		ev(1, "feature.transition", map[string]any{"event": "abandon", "triage": "rejected"}),
	}, nil, Options{})
	if got := labels(rejected); got != "Reported · Rejected in triage" {
		t.Errorf("a rejection is one moment; got %s", got)
	}
}
