package rules

import (
	"encoding/json"

	"github.com/google/uuid"

	"subutai/internal/bus"
)

// The answers to the claim safety nets (SPEC-020 FR-5.3, FR-5.10). The rule
// engine stays pure: it names the move, and the server makes it.

// ClaimResponse is the payload of an answer to claim-stale, claim-deadline or
// unclaimed-commit, written by the Inbox's responseFor: keep | release, or
// seen for an unclaimed commit.
type ClaimResponse struct {
	Answer string `json:"answer"`
	Reason string `json:"reason,omitempty"`
}

// KeepClaim records activity "kept" on the claim a question asked about, so
// its clock restarts (FR-5.3). ClearDeadline also ends a time box that has
// passed, for an answer to claim-deadline.
type KeepClaim struct {
	RefType       string
	RefID         uuid.UUID
	ClaimID       string
	Actor         string
	ClearDeadline bool
}

func (KeepClaim) ActionKind() string { return "keep_claim" }

// ReleaseClaim releases the claim a question asked about to an agent, by the
// person who answered (FR-5.3, FR-2.9).
type ReleaseClaim struct {
	RefType string
	RefID   uuid.UUID
	ClaimID string
	Actor   string
}

func (ReleaseClaim) ActionKind() string { return "release_claim" }

// decideClaimAnswered routes an answer to a claim question. An unclaimed
// commit's one answer, "I've seen this", leads to nothing (SD-11).
func decideClaimAnswered(e bus.CheckpointResponded) []Action {
	var r ClaimResponse
	if err := json.Unmarshal(e.Response, &r); err != nil {
		return nil
	}
	var c struct {
		ClaimID string `json:"claim_id"`
	}
	_ = json.Unmarshal(e.Context, &c)
	switch r.Answer {
	case "keep":
		return []Action{KeepClaim{RefType: e.RefType, RefID: e.RefID, ClaimID: c.ClaimID,
			Actor: e.RespondedBy, ClearDeadline: e.Kind == "claim-deadline"}}
	case "release":
		return []Action{ReleaseClaim{RefType: e.RefType, RefID: e.RefID, ClaimID: c.ClaimID, Actor: e.RespondedBy}}
	}
	return nil
}
