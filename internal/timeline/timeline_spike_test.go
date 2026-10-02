package timeline

import (
	"testing"

	"github.com/google/uuid"
)

// TestTimelineSpikeMoments is SPEC-021 FR-8.3: a spike on the feature adds
// "started" and "ended" moments that name the spike and say how it ended, and
// the second is the system's, not a person's.
func TestTimelineSpikeMoments(t *testing.T) {
	spike := uuid.New()
	started := ev(2, "spike.started", map[string]any{"budget": 40000})
	started.Actor, started.RefType, started.RefID, started.Label = "sam", "spike", spike, "SPK-003"
	ended := ev(5, "spike.ended", map[string]any{"how": "budget"})
	ended.Actor, ended.RefType, ended.RefID, ended.Label = "subutai", "spike", spike, "SPK-003"
	ms := Build([]Event{ev(0, "feature.created", nil), started, ended}, nil, Options{})
	if got, want := labels(ms), "Created · Spike SPK-003 started · Spike SPK-003 ended: it stopped at its budget"; got != want {
		t.Fatalf("moments = %q, want %q", got, want)
	}
	if ms[1].By != ByPerson || ms[1].Actor != "sam" || ms[2].By != BySystem {
		t.Errorf("by = %s %q, %s", ms[1].By, ms[1].Actor, ms[2].By)
	}
	if ms[2].RefType != "spike" || ms[2].RefName != "SPK-003" {
		t.Errorf("ref = %s %q", ms[2].RefType, ms[2].RefName)
	}
	for how, want := range map[string]string{
		"concluded": "it reached a conclusion", "turn_limit": "it stopped at its turn limit", "failed": "its run failed",
	} {
		e := ended
		e.Payload = map[string]any{"how": how}
		if got := Build([]Event{e}, nil, Options{})[0].Label; got != "Spike SPK-003 ended: "+want {
			t.Errorf("%s: %q", how, got)
		}
	}
}

// TestSpikeMomentsAreAsides is SPEC-021 FR-8.3 as the first review found it
// wanting: a spike beside a feature takes none of the feature's runs or
// events and is never where the feature is now.
func TestSpikeMomentsAreAsides(t *testing.T) {
	spike := uuid.New()
	started := ev(2, "spike.started", map[string]any{"budget": 40000})
	started.RefType, started.RefID, started.Label = "spike", spike, "SPK-003"
	ended := ev(6, "spike.ended", map[string]any{"how": "concluded"})
	ended.RefType, ended.RefID, ended.Label = "spike", spike, "SPK-003"
	impl := Run{ID: uuid.New(), Purpose: "implement-task", QueuedAt: at(4)}
	queued := ev(4, "dispatch.queued", map[string]any{"dispatch_id": impl.ID.String()})
	note := ev(5, "task.created", nil)
	events := []Event{
		ev(0, "feature.created", nil),
		ev(1, "feature.transition", map[string]any{"event": "start"}),
		started, queued, note, ended,
	}
	ms := Build(events, []Run{impl}, Options{})
	if got, want := labels(ms), "Created · Building (0 of 0 tasks done) · Spike SPK-003 started · Spike SPK-003 ended: it reached a conclusion"; got != want {
		t.Fatalf("moments = %q, want %q", got, want)
	}
	if len(ms[1].Runs) != 1 || len(ms[2].Runs) != 0 || len(ms[3].Runs) != 0 {
		t.Errorf("runs under the feature's own moment: %d, under the spike's: %d, %d", len(ms[1].Runs), len(ms[2].Runs), len(ms[3].Runs))
	}
	if len(ms[2].Events) != 1 || len(ms[3].Events) != 1 {
		t.Errorf("an aside holds only its own event: %d, %d", len(ms[2].Events), len(ms[3].Events))
	}
	if !ms[2].Aside || !ms[3].Aside || ms[1].Aside || ms[2].State != "idea" {
		t.Errorf("aside = %v %v %v, state %q", ms[1].Aside, ms[2].Aside, ms[3].Aside, ms[2].State)
	}
	if got := CurrentIndex(ms); got != 1 {
		t.Errorf("current = %d, want the feature's own moment, 1", got)
	}
	line := Line(ms)
	for i, s := range line {
		if s.Current != (i == 1) || s.Aside != (i >= 2) {
			t.Errorf("strip %d (%s): current %v, aside %v", i, s.Label, s.Current, s.Aside)
		}
	}
}
