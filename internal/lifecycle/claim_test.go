package lifecycle

import (
	"errors"
	"testing"
)

// TestClaimTransitionMatrix checks every (state, event) pair against
// SPEC-020 FR-2.2's table.
func TestClaimTransitionMatrix(t *testing.T) {
	states := []ClaimState{"", ClaimOpen, ClaimSubmitted, ClaimReturned, ClaimEnded}
	events := []ClaimEvent{ClaimEventClaim, ClaimEventRenew, ClaimEventSubmit, ClaimEventSendBack,
		ClaimEventResume, ClaimEventDone, ClaimEventRelease, ClaimEventAbandon}

	legal := map[ClaimState]map[ClaimEvent]ClaimState{
		"": {ClaimEventClaim: ClaimOpen},
		ClaimOpen: {
			ClaimEventRenew:   ClaimOpen,
			ClaimEventSubmit:  ClaimSubmitted,
			ClaimEventRelease: ClaimEnded,
			ClaimEventAbandon: ClaimEnded,
		},
		ClaimSubmitted: {
			ClaimEventSendBack: ClaimReturned,
			ClaimEventDone:     ClaimEnded,
			ClaimEventAbandon:  ClaimEnded,
		},
		ClaimReturned: {
			ClaimEventResume:  ClaimOpen,
			ClaimEventRelease: ClaimEnded,
			ClaimEventAbandon: ClaimEnded,
		},
		ClaimEnded: {},
	}
	for _, s := range states {
		for _, e := range events {
			next, err := ClaimTransition(s, e)
			if want, ok := legal[s][e]; ok {
				if err != nil || next != want {
					t.Errorf("(%q, %s): got (%q, %v), want (%q, nil)", s, e, next, err, want)
				}
				continue
			}
			var ill *IllegalTransitionError
			if !errors.As(err, &ill) || next != s {
				t.Errorf("(%q, %s): got (%q, %v), want an illegal transition", s, e, next, err)
			}
		}
	}
}

func TestClaimTerminal(t *testing.T) {
	if !ClaimEnded.Terminal() || ClaimOpen.Terminal() || ClaimReturned.Terminal() {
		t.Error("only ended is terminal")
	}
}
