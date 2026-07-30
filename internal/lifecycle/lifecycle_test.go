package lifecycle

import (
	"errors"
	"testing"
)

// TestDocumentTransitionMatrix exercises every (state, event) pair — the
// complete matrix, legal and illegal, per FR-5.1.
func TestDocumentTransitionMatrix(t *testing.T) {
	states := []DocumentState{DocDraft, DocReviewing, DocApproved, DocSuperseded}
	events := []DocumentEvent{DocSubmit, DocApprove, DocRequestChanges, DocEscalate, DocSupersede, DocWithdraw}

	// legal[state][event] = expected next state; absent = illegal.
	legal := map[DocumentState]map[DocumentEvent]DocumentState{
		DocDraft: {
			DocSubmit: DocReviewing,
		},
		DocReviewing: {
			DocApprove:        DocApproved,
			DocRequestChanges: DocDraft,
			DocEscalate:       DocReviewing, // no state change; checkpoint is the side effect
			DocWithdraw:       DocDraft,
		},
		DocApproved: {
			DocSupersede: DocSuperseded,
		},
		DocSuperseded: {}, // terminal: everything illegal
	}

	covered := 0
	for _, s := range states {
		for _, e := range events {
			covered++
			next, err := DocumentTransition(s, e)
			want, ok := legal[s][e]
			if ok {
				if err != nil {
					t.Errorf("(%s, %s): unexpected error %v", s, e, err)
				}
				if next != want {
					t.Errorf("(%s, %s): got %s, want %s", s, e, next, want)
				}
				continue
			}
			var ill *IllegalTransitionError
			if !errors.As(err, &ill) {
				t.Errorf("(%s, %s): want IllegalTransitionError, got %v", s, e, err)
				continue
			}
			if ill.Entity != "document" || ill.State != string(s) || ill.Event != string(e) {
				t.Errorf("(%s, %s): error names wrong transition: %+v", s, e, ill)
			}
			if next != s {
				t.Errorf("(%s, %s): illegal transition must not move state; got %s", s, e, next)
			}
		}
	}
	if covered != len(states)*len(events) {
		t.Fatalf("matrix not fully covered: %d cells", covered)
	}
}

func TestFeatureTransitionMatrix(t *testing.T) {
	states := []FeatureState{FeatIdea, FeatReady, FeatActive, FeatReview, FeatDone, FeatAbandoned}
	events := []FeatureEvent{FeatContractApproved, FeatStart, FeatTasksComplete, FeatVerified, FeatAbandon}

	legal := map[FeatureState]map[FeatureEvent]FeatureState{
		FeatIdea: {
			FeatContractApproved: FeatReady,
			FeatAbandon:          FeatAbandoned,
		},
		FeatReady: {
			FeatStart:   FeatActive,
			FeatAbandon: FeatAbandoned,
		},
		FeatActive: {
			FeatTasksComplete: FeatReview,
			FeatAbandon:       FeatAbandoned,
		},
		FeatReview: {
			FeatVerified: FeatDone,
			FeatAbandon:  FeatAbandoned,
		},
		FeatDone:      {}, // terminal
		FeatAbandoned: {}, // terminal
	}

	for _, s := range states {
		for _, e := range events {
			next, err := FeatureTransition(s, e)
			want, ok := legal[s][e]
			if ok {
				if err != nil || next != want {
					t.Errorf("(%s, %s): got (%s, %v), want (%s, nil)", s, e, next, err, want)
				}
				continue
			}
			var ill *IllegalTransitionError
			if !errors.As(err, &ill) {
				t.Errorf("(%s, %s): want IllegalTransitionError, got %v", s, e, err)
			}
			if next != s {
				t.Errorf("(%s, %s): illegal transition must not move state; got %s", s, e, next)
			}
		}
	}
}

func TestTerminal(t *testing.T) {
	for s, want := range map[FeatureState]bool{
		FeatIdea: false, FeatReady: false, FeatActive: false,
		FeatReview: false, FeatDone: true, FeatAbandoned: true,
	} {
		if s.Terminal() != want {
			t.Errorf("%s.Terminal() = %v, want %v", s, s.Terminal(), want)
		}
	}
}

func TestG1(t *testing.T) {
	cases := []struct {
		spec, required, plan bool
		pass                 bool
	}{
		{false, false, false, false}, // no spec: fail (phase 1)
		{true, false, false, true},   // phase 1: spec alone passes (D-1)
		{true, true, false, false},   // phase 2: dev-plan missing fails
		{true, true, true, true},     // phase 2: both approved passes
		{false, true, true, false},   // spec missing always fails
	}
	for _, c := range cases {
		got := G1(c.spec, c.required, c.plan)
		if got.Pass != c.pass {
			t.Errorf("G1(%v,%v,%v).Pass = %v, want %v (reason %q)",
				c.spec, c.required, c.plan, got.Pass, c.pass, got.Reason)
		}
		if got.Gate != GateG1 || got.Reason == "" {
			t.Errorf("G1 result malformed: %+v", got)
		}
	}
}

func TestG4(t *testing.T) {
	cases := []struct {
		resolved, done int
		pass           bool
	}{
		{0, 0, false}, // empty milestone: nothing to lock
		{3, 0, false}, // live but nothing finished
		{3, 1, true},  // one member done: lockable (FR-5.2)
		{1, 1, true},
	}
	for _, c := range cases {
		got := G4(c.resolved, c.done)
		if got.Pass != c.pass {
			t.Errorf("G4(%d,%d).Pass = %v, want %v (reason %q)",
				c.resolved, c.done, got.Pass, c.pass, got.Reason)
		}
		if got.Gate != GateG4 || got.Reason == "" {
			t.Errorf("G4 result malformed: %+v", got)
		}
	}
}

func TestG5(t *testing.T) {
	if r := G5(0); !r.Pass {
		t.Errorf("G5(0) should pass: %+v", r)
	}
	if r := G5(2); r.Pass || r.Reason == "" {
		t.Errorf("G5(2) should fail with reason: %+v", r)
	}
}

// G0 is what "approving a design means ready to spec" reduces to. The rule
// that matters most is the one that stops: inheritance is one level, so a
// top-level design never releases features under half-formed sub-initiatives.
func TestG0SpecReady(t *testing.T) {
	cases := []struct {
		name              string
		own, parent, pass bool
	}{
		{"own design approved", true, false, true},
		{"parent initiative's design approved", false, true, true},
		{"both approved", true, true, true},
		{"neither — nothing releases it", false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := G0(c.own, c.parent)
			if g.Pass != c.pass {
				t.Errorf("G0(%v,%v).Pass = %v, want %v", c.own, c.parent, g.Pass, c.pass)
			}
			if g.Gate != GateG0 || g.Reason == "" {
				t.Errorf("every gate result carries its name and a reason a person can read: %+v", g)
			}
		})
	}
	// The one-level rule is enforced by the caller passing only the immediate
	// parent, so the gate itself cannot see a grandparent — asserted here so
	// the signature is not "helpfully" widened later.
	if g := G0(false, false); g.Pass {
		t.Error("G0 must not pass on anything but its own or its immediate parent's design")
	}
}
