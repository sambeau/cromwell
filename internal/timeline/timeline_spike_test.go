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
