package lifecycle

// Task lifecycle per DESIGN-005 §5. A task is a feature-sized unit of
// execution: the shape mirrors the feature machine, but readiness is
// dependency-driven (pending → ready is automatic when deps are done)
// rather than a human start.

type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskReady     TaskState = "ready"
	TaskActive    TaskState = "active"
	TaskReview    TaskState = "review"
	TaskDone      TaskState = "done"
	TaskAbandoned TaskState = "abandoned"
)

// Terminal reports whether the state is terminal (feeds G2's task counts).
func (s TaskState) Terminal() bool {
	return s == TaskDone || s == TaskAbandoned
}

type TaskEvent string

const (
	// TaskReadyEvent fires automatically when every dependency is done
	// (DESIGN-005 §5). A task with no deps is ready at creation.
	TaskReadyEvent TaskEvent = "ready"
	// TaskClaim starts an implementer dispatch (dispatcher-driven).
	TaskClaim TaskEvent = "claim"
	// TaskImplemented fires when submit_implementation completes; the diff
	// goes to code review.
	TaskImplemented TaskEvent = "implemented"
	// TaskApprove / TaskRequestChanges are the code-review verdicts.
	TaskApprove        TaskEvent = "approve"
	TaskRequestChanges TaskEvent = "request_changes"
	// TaskAbandon is always human, always with a reason.
	TaskAbandon TaskEvent = "abandon"
)

var taskTransitions = map[TaskState]map[TaskEvent]TaskState{
	TaskPending: {
		TaskReadyEvent: TaskReady,
		TaskAbandon:    TaskAbandoned,
	},
	TaskReady: {
		TaskClaim:   TaskActive,
		TaskAbandon: TaskAbandoned,
	},
	TaskActive: {
		TaskImplemented: TaskReview,
		TaskAbandon:     TaskAbandoned,
	},
	TaskReview: {
		TaskApprove:        TaskDone,
		TaskRequestChanges: TaskActive, // implementer re-dispatched, worktree kept
		TaskAbandon:        TaskAbandoned,
	},
}

// TaskTransition returns the state event produces from state, or an
// *IllegalTransitionError (same contract as DocumentTransition).
func TaskTransition(state TaskState, event TaskEvent) (TaskState, error) {
	if next, ok := taskTransitions[state][event]; ok {
		return next, nil
	}
	return state, &IllegalTransitionError{Entity: "task", State: string(state), Event: string(event)}
}
