// Package sizing is the pure roll-up and confidence-tier engine for estimates
// (SPEC-003 FR-1, FR-2; vision §5). It has no I/O: the store loads an entity
// tree with its current unit estimates, sizing rolls it up, and the CLI
// renders the result. Roll-ups are computed, never stored (DESIGN-001 §7).
//
// Confidence semantics (SPEC-003 REVIEW decision, 2026-07-21 — "worst-tier
// taints"): tiers propagate by the worst tier in the subtree, so "no estimate
// ever looks more confident than its weakest input" (FR-1 goal). A parent
// whose estimate is the arithmetic sum of estimated children is flagged
// Decomposed (the derivation, FR-1.2's "by arithmetic"); its confidence Tier
// is still the worst leaf tier — decomposition is a derivation, not a
// confidence gain. A single unestimated leaf leaves the sum incomplete (the
// `?`), and the unestimated leaves are listed explicitly (FR-2.1).
package sizing

import "github.com/google/uuid"

// Tier is an estimate's confidence tier (DESIGN-001 §3, vision §5), assigned
// from the evidence, never the caller's say-so (FR-1.2).
type Tier string

const (
	TierDecomposed Tier = "decomposed" // arithmetic sum of child estimates
	TierConsidered Tier = "considered" // unit estimate informed by the corpus
	TierRough      Tier = "rough"      // unit estimate from judgement alone
)

// tierRank orders tiers by confidence; a higher rank is worse (less
// confident). rough > considered > decomposed.
func tierRank(t Tier) int {
	switch t {
	case TierDecomposed:
		return 0
	case TierConsidered:
		return 1
	case TierRough:
		return 2
	default:
		return 2 // unknown tiers are treated as the least trustworthy
	}
}

// WorseTier returns the less-confident of two tiers — the worst-tier
// propagation primitive (FR-2.1).
func WorseTier(a, b Tier) Tier {
	if tierRank(a) >= tierRank(b) {
		return a
	}
	return b
}

// Ref identifies an entity in a roll-up tree, so unestimated work can be named
// in the `?` listing (FR-2.1).
type Ref struct {
	Type string // "initiative" | "feature" | "task"
	ID   uuid.UUID
	Name string // human label for the listing
}

// UnitEstimate is a node's own current estimate — a single stored row
// (considered or rough; a stored estimate is never decomposed, DESIGN-001 §7).
type UnitEstimate struct {
	Tokens int64
	Tier   Tier
}

// Node is one entity in the roll-up tree: its ref, its own current unit
// estimate (nil if none), and its children (features under an initiative,
// tasks under a feature). Tasks are leaves.
type Node struct {
	Ref      Ref
	Estimate *UnitEstimate
	Children []Node
}

// Rollup is the computed size of a node and its subtree.
type Rollup struct {
	Ref         Ref
	Tokens      int64 // sum of estimated descendants' tokens (the known part)
	Tier        Tier  // worst tier among estimated leaves in the subtree
	Estimated   bool  // at least one estimated leaf contributed
	Decomposed  bool  // Tokens is an arithmetic sum of children (vs a unit estimate)
	Complete    bool  // no unestimated leaves in the subtree (the sum is whole)
	Unestimated []Ref // leaves with no estimate, listed explicitly (the `?`)
}

// RollUp computes a node's size bottom-up. A node with estimated children is a
// decomposition (sum the children, worst-tier over them); a node with none
// falls back to its own unit estimate; a node with neither is unestimated and
// lists itself. A direct unit estimate is superseded once the node has been
// decomposed into estimated children (the decomposition is the finer truth,
// DESIGN-001 §7).
func RollUp(n Node) Rollup {
	if len(n.Children) == 0 {
		if n.Estimate != nil {
			return Rollup{Ref: n.Ref, Tokens: n.Estimate.Tokens, Tier: n.Estimate.Tier,
				Estimated: true, Complete: true}
		}
		return Rollup{Ref: n.Ref, Unestimated: []Ref{n.Ref}}
	}

	var tokens int64
	var tier Tier
	var unestimated []Ref
	estimated := 0
	for _, c := range n.Children {
		cr := RollUp(c)
		if cr.Estimated {
			tokens += cr.Tokens
			if estimated == 0 {
				tier = cr.Tier
			} else {
				tier = WorseTier(tier, cr.Tier)
			}
			estimated++
		}
		unestimated = append(unestimated, cr.Unestimated...)
	}

	if estimated == 0 {
		// Nothing beneath is estimated. A direct unit estimate on this node
		// stands and subsumes its (unestimated) children; otherwise the
		// unestimated leaves bubble up — the missing work is at the leaf
		// level, listed at the same granularity whether or not a sibling
		// happens to be estimated.
		if n.Estimate != nil {
			return Rollup{Ref: n.Ref, Tokens: n.Estimate.Tokens, Tier: n.Estimate.Tier,
				Estimated: true, Complete: true}
		}
		return Rollup{Ref: n.Ref, Unestimated: unestimated}
	}

	return Rollup{
		Ref: n.Ref, Tokens: tokens, Tier: tier, Estimated: true,
		Decomposed: true, Complete: len(unestimated) == 0, Unestimated: unestimated,
	}
}
