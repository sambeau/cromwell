package sizing

import (
	"testing"

	"github.com/google/uuid"
)

func ref(t, name string) Ref { return Ref{Type: t, ID: uuid.New(), Name: name} }

func unit(tokens int64, tier Tier) *UnitEstimate { return &UnitEstimate{Tokens: tokens, Tier: tier} }

func TestWorseTier(t *testing.T) {
	cases := []struct{ a, b, want Tier }{
		{TierDecomposed, TierConsidered, TierConsidered},
		{TierConsidered, TierRough, TierRough},
		{TierDecomposed, TierRough, TierRough},
		{TierRough, TierRough, TierRough},
		{TierDecomposed, TierDecomposed, TierDecomposed},
	}
	for _, c := range cases {
		if got := WorseTier(c.a, c.b); got != c.want {
			t.Errorf("WorseTier(%s,%s) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}

func TestLeafEstimatedAndUnestimated(t *testing.T) {
	est := RollUp(Node{Ref: ref("task", "T1"), Estimate: unit(500, TierConsidered)})
	if !est.Estimated || est.Tokens != 500 || est.Tier != TierConsidered || !est.Complete {
		t.Fatalf("estimated leaf: %+v", est)
	}
	if len(est.Unestimated) != 0 {
		t.Errorf("estimated leaf should list nothing unestimated: %+v", est.Unestimated)
	}

	un := RollUp(Node{Ref: ref("task", "T2")})
	if un.Estimated || un.Complete || len(un.Unestimated) != 1 || un.Unestimated[0].Name != "T2" {
		t.Fatalf("unestimated leaf: %+v", un)
	}
}

// FR-1.2 AC: a feature whose tasks are all estimated rolls up to a decomposed
// feature estimate by arithmetic. Under the "worst-tier taints" decision the
// derivation is flagged Decomposed and the confidence Tier is the worst leaf.
func TestFeatureDecomposesFromTasks(t *testing.T) {
	f := Node{Ref: ref("feature", "login"), Children: []Node{
		{Ref: ref("task", "T1"), Estimate: unit(1200, TierConsidered)},
		{Ref: ref("task", "T2"), Estimate: unit(800, TierConsidered)},
	}}
	r := RollUp(f)
	if r.Tokens != 2000 {
		t.Errorf("tokens = %d, want 2000 (arithmetic sum)", r.Tokens)
	}
	if !r.Decomposed {
		t.Error("feature summed from tasks should be flagged Decomposed")
	}
	if !r.Complete {
		t.Error("all tasks estimated → complete")
	}
	if r.Tier != TierConsidered {
		t.Errorf("tier = %s, want considered (worst leaf)", r.Tier)
	}
}

// FR-1.2: a worse task taints the decomposed feature (worst-tier).
func TestDecomposedFeatureTaintedByRoughTask(t *testing.T) {
	f := Node{Ref: ref("feature", "login"), Children: []Node{
		{Ref: ref("task", "T1"), Estimate: unit(1200, TierConsidered)},
		{Ref: ref("task", "T2"), Estimate: unit(800, TierRough)},
	}}
	r := RollUp(f)
	if r.Tier != TierRough {
		t.Errorf("tier = %s, want rough (one rough task taints the whole)", r.Tier)
	}
	if r.Tokens != 2000 || !r.Decomposed || !r.Complete {
		t.Errorf("unexpected rollup: %+v", r)
	}
}

// FR-1.2: a unit-estimated feature (no decomposed children) keeps its own tier.
func TestUnitEstimatedFeature(t *testing.T) {
	considered := RollUp(Node{Ref: ref("feature", "a"), Estimate: unit(5000, TierConsidered)})
	if considered.Tier != TierConsidered || considered.Decomposed {
		t.Errorf("unit considered: %+v", considered)
	}
	rough := RollUp(Node{Ref: ref("feature", "b"), Estimate: unit(5000, TierRough)})
	if rough.Tier != TierRough || rough.Decomposed {
		t.Errorf("unit rough: %+v", rough)
	}
}

// FR-2.1 AC: an initiative with two estimated features (one considered, one
// rough) and one unestimated feature rolls up to a rough total plus a `?`, and
// names the unestimated feature.
func TestInitiativeWorstTierAndUnestimatedListing(t *testing.T) {
	init := Node{Ref: ref("initiative", "auth"), Children: []Node{
		{Ref: ref("feature", "f1"), Estimate: unit(3000, TierConsidered)},
		{Ref: ref("feature", "f2"), Estimate: unit(4000, TierRough)},
		{Ref: ref("feature", "f3")}, // unestimated
	}}
	r := RollUp(init)
	if r.Tokens != 7000 {
		t.Errorf("tokens = %d, want 7000 (known part only)", r.Tokens)
	}
	if r.Tier != TierRough {
		t.Errorf("tier = %s, want rough (worst of considered+rough)", r.Tier)
	}
	if r.Complete {
		t.Error("one unestimated feature → not complete (the `?`)")
	}
	if len(r.Unestimated) != 1 || r.Unestimated[0].Name != "f3" {
		t.Fatalf("unestimated listing = %+v, want [f3]", r.Unestimated)
	}
	if !r.Decomposed {
		t.Error("initiative summed from features is a decomposition")
	}
}

// Deeper tree: worst tier propagates from a leaf task through its feature to
// the initiative, and unestimated leaves bubble up transitively.
func TestNestedPropagation(t *testing.T) {
	init := Node{Ref: ref("initiative", "root"), Children: []Node{
		{Ref: ref("feature", "decomposed"), Children: []Node{
			{Ref: ref("task", "t1"), Estimate: unit(1000, TierConsidered)},
			{Ref: ref("task", "t2"), Estimate: unit(500, TierRough)}, // taints everything above
		}},
		{Ref: ref("feature", "partial"), Children: []Node{
			{Ref: ref("task", "t3"), Estimate: unit(2000, TierConsidered)},
			{Ref: ref("task", "t4")}, // unestimated leaf
		}},
	}}
	r := RollUp(init)
	if r.Tier != TierRough {
		t.Errorf("tier = %s, want rough (deep rough task taints root)", r.Tier)
	}
	if r.Tokens != 3500 {
		t.Errorf("tokens = %d, want 3500 (1000+500+2000)", r.Tokens)
	}
	if r.Complete {
		t.Error("t4 unestimated → root not complete")
	}
	if len(r.Unestimated) != 1 || r.Unestimated[0].Name != "t4" {
		t.Fatalf("unestimated = %+v, want [t4]", r.Unestimated)
	}
}

// A feature decomposed into tasks that are all unestimated is itself
// unestimated and lists its tasks, not itself.
func TestFeatureAllTasksUnestimated(t *testing.T) {
	f := Node{Ref: ref("feature", "x"), Children: []Node{
		{Ref: ref("task", "t1")},
		{Ref: ref("task", "t2")},
	}}
	r := RollUp(f)
	if r.Estimated {
		t.Errorf("no task estimated → feature unestimated: %+v", r)
	}
	if len(r.Unestimated) != 2 {
		t.Errorf("both tasks unestimated should be listed: %+v", r.Unestimated)
	}
}

// A direct unit estimate stands when the node has no estimated children; its
// unestimated children are subsumed (the feature was estimated as a unit).
func TestUnitEstimateSubsumesUnestimatedChildren(t *testing.T) {
	f := Node{Ref: ref("feature", "x"), Estimate: unit(9000, TierRough), Children: []Node{
		{Ref: ref("task", "t1")},
	}}
	r := RollUp(f)
	if !r.Estimated || r.Tokens != 9000 || r.Tier != TierRough {
		t.Fatalf("unit estimate should stand: %+v", r)
	}
	if !r.Complete || len(r.Unestimated) != 0 {
		t.Errorf("unit estimate subsumes unestimated children: %+v", r)
	}
}
