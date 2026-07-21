package lifecycle

// Milestone lifecycle per DESIGN-001 §6 / DESIGN-003 §8. A milestone is open
// while its membership is live; locking (gate G4) freezes the resolved leaf
// set into a snapshot and is one-way — there is no unlock (vision §4: a locked
// milestone is the true record of what shipped). The transition itself is
// mechanical; the honesty lives in G4 and the snapshot (store.LockMilestone).

type MilestoneState string

const (
	MilestoneOpen   MilestoneState = "open"
	MilestoneLocked MilestoneState = "locked"
)
