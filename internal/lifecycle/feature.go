package lifecycle

// Feature lifecycle per DESIGN-003 §6. The full machine is implemented even
// though phase 1 only drives idea → ready and abandon (SPEC-001 §2): the
// early system is a true subset of the mature design, not a divergent
// prototype.

type FeatureState string

const (
	FeatIdea      FeatureState = "idea"
	FeatReady     FeatureState = "ready"
	FeatActive    FeatureState = "active"
	FeatReview    FeatureState = "review"
	FeatDone      FeatureState = "done"
	FeatAbandoned FeatureState = "abandoned"
)

// Terminal reports whether the state is terminal (feeds gate G5).
func (s FeatureState) Terminal() bool {
	return s == FeatDone || s == FeatAbandoned
}

type FeatureEvent string

const (
	// FeatContractApproved fires automatically when G1 passes (DESIGN-003 §6).
	FeatContractApproved FeatureEvent = "contract_approved"
	// FeatStart is human-triggered by default (L-5); phase 2.
	FeatStart FeatureEvent = "start"
	// FeatTasksComplete fires when G2 passes; phase 2.
	FeatTasksComplete FeatureEvent = "tasks_complete"
	// FeatVerified fires when G3 passes (verification approved, branch merged); phase 2.
	FeatVerified FeatureEvent = "verified"
	// FeatRework fires when verification requests changes: the feature returns
	// to active to address unmet criteria (DESIGN-006 §6).
	FeatRework FeatureEvent = "rework"
	// FeatAbandon is always human, always with a reason (DESIGN-003 §6).
	FeatAbandon FeatureEvent = "abandon"
	// FeatContractInvalidated is contract_approved's inverse, fired only by
	// the engine when the revision cascade supersedes a ready feature's
	// contract (SPEC-009 FR-9.4): a feature whose spec no longer stands is an
	// idea again, which is what lets the authoring invariant write it a fresh
	// one. Never human-triggered — a person who wants a feature out of ready
	// abandons it or revises its documents.
	FeatContractInvalidated FeatureEvent = "contract_invalidated"
)

var featTransitions = map[FeatureState]map[FeatureEvent]FeatureState{
	FeatIdea: {
		FeatContractApproved: FeatReady,
		FeatAbandon:          FeatAbandoned,
	},
	FeatReady: {
		FeatStart:               FeatActive,
		FeatContractInvalidated: FeatIdea,
		FeatAbandon:             FeatAbandoned,
	},
	FeatActive: {
		FeatTasksComplete: FeatReview,
		FeatAbandon:       FeatAbandoned,
	},
	FeatReview: {
		FeatVerified: FeatDone,
		FeatRework:   FeatActive, // verification requested changes (DESIGN-006 §6)
		FeatAbandon:  FeatAbandoned,
	},
}

// FeatureTransition returns the state event produces from state, or an
// *IllegalTransitionError (same contract as DocumentTransition).
func FeatureTransition(state FeatureState, event FeatureEvent) (FeatureState, error) {
	if next, ok := featTransitions[state][event]; ok {
		return next, nil
	}
	return state, &IllegalTransitionError{Entity: "feature", State: string(state), Event: string(event)}
}
