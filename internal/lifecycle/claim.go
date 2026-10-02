package lifecycle

// The claim machine (SPEC-020 FR-2.2): a person or the chat agent holding a
// task instead of an agent. The contract matches the task machine's: legality
// is decided here and nowhere else, and an illegal pair is an
// *IllegalTransitionError.

type ClaimState string

const (
	ClaimOpen      ClaimState = "open"
	ClaimSubmitted ClaimState = "submitted"
	// ClaimReturned holds the task but not the working copy (SPEC-020 SD-6):
	// the code review sent the work back and the executor hasn't resumed.
	ClaimReturned ClaimState = "returned"
	ClaimEnded    ClaimState = "ended"
)

// Terminal reports whether the claim has ended.
func (s ClaimState) Terminal() bool { return s == ClaimEnded }

type ClaimEvent string

const (
	// ClaimEventClaim is the first event, from no state at all.
	ClaimEventClaim    ClaimEvent = "claim"
	ClaimEventRenew    ClaimEvent = "renew"
	ClaimEventSubmit   ClaimEvent = "submit"
	ClaimEventSendBack ClaimEvent = "send_back"
	ClaimEventResume   ClaimEvent = "resume"
	ClaimEventDone     ClaimEvent = "done"
	ClaimEventRelease  ClaimEvent = "release"
	ClaimEventAbandon  ClaimEvent = "abandon"
)

// claimStart is the "state" a claim has before it exists.
const claimStart ClaimState = ""

var claimTransitions = map[ClaimState]map[ClaimEvent]ClaimState{
	claimStart: {
		ClaimEventClaim: ClaimOpen,
	},
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
}

// ClaimTransition returns the state event produces from state ("" for the
// first event, claim), or an *IllegalTransitionError.
func ClaimTransition(from ClaimState, e ClaimEvent) (ClaimState, error) {
	if next, ok := claimTransitions[from][e]; ok {
		return next, nil
	}
	return from, &IllegalTransitionError{Entity: "claim", State: string(from), Event: string(e)}
}
