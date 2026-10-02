package server

// The words for who runs a spike and what limit it has (SPEC-021 FR-16.1,
// FR-16.4). These are pure: no database. Times are in UTC whatever the
// machine's zone.

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

func TestSpikeExecutorSentences(t *testing.T) {
	now := time.Date(2026, 10, 3, 13, 0, 0, 0, time.UTC)
	id := uuid.New()
	exec := func(kind, actor, model string) store.Execution {
		e := store.Execution{Kind: kind, Actor: actor, Model: model}
		if kind == store.ExecutorAgent {
			e.DispatchID = &id
		}
		return e
	}
	claim := func(kind, actor string, state lifecycle.ClaimState, ago time.Duration) *store.Claim {
		return &store.Claim{Kind: kind, Actor: actor, State: state, ClaimedAt: now.Add(-ago)}
	}
	for _, c := range []struct {
		name  string
		sp    store.Spike
		execs []store.Execution
		claim *store.Claim
		want  string
		kind  string
		meas  bool
	}{
		{"idea", store.Spike{State: store.SpikeIdea}, nil, nil, "Not started yet.", "", true},
		{"agent before the run", store.Spike{State: store.SpikeRunning, Executor: "agent"}, nil, nil, "To be run by the spike runner.", "agent", true},
		{"agent", store.Spike{State: store.SpikeEnded, Executor: "agent"}, []store.Execution{exec("agent", "spike-runner", "the-model")}, nil, "Run by the spike runner (the-model).", "agent", true},
		{"chat unclaimed", store.Spike{State: store.SpikeRunning, Executor: "chat"}, nil, nil, "To be run in chat. Waiting for the chat agent to claim it.", "chat", false},
		{"chat released", store.Spike{State: store.SpikeRunning, Executor: "chat"}, []store.Execution{exec("chat", "chat", "")},
			claim("chat", "chat", lifecycle.ClaimEnded, time.Hour), "To be run in chat. Waiting for the chat agent to claim it.", "chat", false},
		{"chat claimed", store.Spike{State: store.SpikeRunning, Executor: "chat"}, []store.Execution{exec("chat", "chat", "")},
			claim("chat", "chat", lifecycle.ClaimOpen, 2*time.Hour), "Being run in chat by the chat agent, who claimed it 2 hours ago.", "chat", false},
		{"person unclaimed", store.Spike{State: store.SpikeRunning, Executor: "person"}, nil, nil, "To be run by hand. Waiting for a person to claim it.", "person", false},
		{"person claimed", store.Spike{State: store.SpikeRunning, Executor: "person"}, []store.Execution{exec("person", "sam", "")},
			claim("person", "sam", lifecycle.ClaimOpen, time.Hour), "Being run by hand by sam.", "person", false},
		{"chat ran", store.Spike{State: store.SpikeEnded, Executor: "chat"}, []store.Execution{exec("chat", "chat", "")},
			claim("chat", "chat", lifecycle.ClaimEnded, time.Hour), "Run in chat by the chat agent.", "chat", false},
		{"person ran", store.Spike{State: store.SpikeClosed, Executor: "person"}, []store.Execution{exec("person", "sam", "")},
			claim("person", "sam", lifecycle.ClaimEnded, time.Hour), "Run by hand by sam.", "person", false},
		{"nobody came", store.Spike{State: store.SpikeEnded, Executor: "chat"}, nil, nil, "It was to be run in chat, but nobody claimed it.", "chat", false},
	} {
		sp := c.sp
		got := spikeExecutorSentence(&sp, spikeRunFacts{Execs: c.execs, Claim: c.claim}, now)
		if got.Sentence != c.want || got.Kind != c.kind || got.Measured != c.meas {
			t.Errorf("%s: %+v, want %q kind %q measured %v", c.name, got, c.want, c.kind, c.meas)
		}
	}
	if got := spikeExecutorSentence(&store.Spike{State: store.SpikeEnded, Executor: "agent"}, spikeRunFacts{Execs: []store.Execution{exec("agent", "spike-runner", "m")}}, now); got.RunID != id.String() || got.Model != "m" {
		t.Errorf("run id and model = %+v", got)
	}
}

func TestSpikeTimeBoxAndUnmeasuredLines(t *testing.T) {
	end := time.Date(2026, 10, 3, 17, 4, 0, 0, time.UTC)
	hours := 4
	running := &store.Spike{State: store.SpikeRunning, Executor: "chat", TimeBoxHours: &hours, DeadlineAt: &end}
	now := end.Add(-(3*time.Hour + 12*time.Minute))
	if got, want := spikeTimeBoxLine(running, now), "Time box: 4 hours, ending at 17:04 UTC on 3 October (3 hours 12 minutes left)."; got != want {
		t.Errorf("running = %q, want %q", got, want)
	}
	for d, want := range map[time.Duration]string{
		time.Hour: "1 hour left", 42 * time.Minute: "42 minutes left", time.Minute: "1 minute left",
		30 * time.Second: "less than a minute left", 0: "no time left", -time.Hour: "no time left",
		2*time.Hour + time.Minute: "2 hours 1 minute left",
	} {
		if got := spikeLeftWords(d); got != want {
			t.Errorf("%v = %q, want %q", d, got, want)
		}
	}
	ended := *running
	ended.State = store.SpikeEnded
	if got := spikeTimeBoxLine(&ended, now); got != "Time box: 4 hours." {
		t.Errorf("ended = %q", got)
	}
	one := 1
	ended.TimeBoxHours = &one
	if got := spikeTimeBoxLine(&ended, now); got != "Time box: 1 hour." {
		t.Errorf("one hour = %q", got)
	}
	agent := &store.Spike{State: store.SpikeRunning, Executor: "agent"}
	if spikeTimeBoxLine(agent, now) != "" || spikeUnmeasuredLine(agent) != "" || spikeUnmeasuredLine(&store.Spike{}) != "" {
		t.Error("an agent spike has no time box line and no unmeasured line")
	}
	if got := spikeUnmeasuredLine(&store.Spike{Executor: "chat"}); got != "This spike ran in chat, so its tokens weren't measured." {
		t.Errorf("chat = %q", got)
	}
	if got := spikeUnmeasuredLine(&store.Spike{Executor: "person"}); got != "This spike was run by hand, so its tokens weren't measured." {
		t.Errorf("person = %q", got)
	}
}

// A deadline is said in UTC, with the zone named, whatever the machine's
// zone, and the page's line and the chat agent's sentence say the same hour.
func TestSpikeDeadlinesAreSaidInUTC(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("Elsewhere", 3600)
	defer func() { time.Local = saved }()

	end := time.Date(2026, 10, 3, 17, 4, 0, 0, time.Local) // 16:04 UTC
	hours := 4
	sp := &store.Spike{State: store.SpikeRunning, Executor: "person", TimeBoxHours: &hours, DeadlineAt: &end}
	now := end.Add(-(3*time.Hour + 12*time.Minute))
	if got, want := spikeTimeBoxLine(sp, now), "Time box: 4 hours, ending at 16:04 UTC on 3 October (3 hours 12 minutes left)."; got != want {
		t.Errorf("page line = %q, want %q", got, want)
	}
	if got, want := spikeTimeLeft(sp, now), "The time box ends at 16:04 UTC, in 3 hours 12 minutes. Then the spike ends with whatever findings you have saved."; got != want {
		t.Errorf("chat sentence = %q, want %q", got, want)
	}
	if got, want := spikeTimeLeft(sp, end.Add(time.Second)), "The time box ended at 16:04 UTC. The spike ends with whatever findings were saved."; got != want {
		t.Errorf("after the deadline = %q, want %q", got, want)
	}
	if got, want := spikeTimeLeft(sp, end.Add(-30*time.Second)), "The time box ends at 16:04 UTC, in less than a minute. Then the spike ends with whatever findings you have saved."; got != want {
		t.Errorf("in the last minute = %q, want %q", got, want)
	}
	if got := spikeDeadlineRefusal(&store.Spike{PublicID: "SPK-001", DeadlineAt: &end}, end.Add(time.Minute)); got != "SPK-001's time box ended at 16:04 UTC, so it is ending with the findings that were saved." {
		t.Errorf("refusal = %q", got)
	}
}

// Who ran a spike is worked out once, so the page's sentence, the
// findings and the closer's test agree after a release and a second claim by
// someone else.
func TestSpikeRanByIsOneAnswer(t *testing.T) {
	sp := &store.Spike{State: store.SpikeClosed, Executor: store.ExecutorPerson, ClosedBy: "sam"}
	execs := []store.Execution{{Kind: "person", Actor: "sam"}, {Kind: "person", Actor: "pat"}}
	later := &store.Claim{Kind: "person", Actor: "pat", State: lifecycle.ClaimEnded}
	facts := spikeRunFacts{Execs: execs, Claim: later}
	if kind, who := spikeRanBy(sp, execs, later); kind != "person" || who != "pat" {
		t.Errorf("ran by %q %q", kind, who)
	}
	if closerRanIt(sp, facts, "sam") || !closerRanIt(sp, facts, "pat") {
		t.Error("the closer test names someone other than who the sentence names")
	}
	if got := spikeClosedBySentence(sp, facts); got != "" {
		t.Errorf("sam closed it, but pat ran it: %q", got)
	}
	// With no execution row, the latest claim's actor stands.
	if _, who := spikeRanBy(sp, nil, later); who != "pat" {
		t.Errorf("from the claim: %q", who)
	}
	if _, who := spikeRanBy(&store.Spike{}, execs, later); who != "" {
		t.Errorf("a spike with no executor was run by %q", who)
	}
}

// The sentence about tokens is written once.
func TestTokensNotMeasuredIsOneSentence(t *testing.T) {
	for _, c := range []struct{ executor, subject, want string }{
		{"chat", "This spike", "This spike ran in chat, so its tokens weren't measured."},
		{"person", "It", "It was run by hand, so its tokens weren't measured."},
		{"agent", "It", ""},
	} {
		if got := tokensNotMeasured(c.executor, c.subject); got != c.want {
			t.Errorf("%s/%s = %q, want %q", c.executor, c.subject, got, c.want)
		}
	}
}
