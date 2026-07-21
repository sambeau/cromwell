package server

// Phase-3 integration suite (SPEC-003): estimates and roll-ups, the AI
// estimator through the real dispatch loop, milestones with G4 and snapshot,
// roadmaps, and extended cost — driven over the HTTP API the CLI uses.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/provider"
	"cromwell/internal/store"
)

// markDone flips a feature to done directly (the full G1→G3 path is covered by
// the phase-2 suite; here we only need the terminal state for progress/G4).
func (h *harness) markDone(path string) {
	h.t.Helper()
	f, err := h.srv.featureByPath(context.Background(), path)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.srv.Store.Pool.Exec(context.Background(),
		`UPDATE features SET state='done' WHERE id=$1`, f.ID); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) createFeature(initiativePath, slug, name, description string) {
	h.t.Helper()
	code, out := h.call("POST", "/api/features", map[string]string{
		"initiative_path": initiativePath, "slug": slug, "name": name, "description": description})
	if code != 201 {
		h.t.Fatalf("create feature %s: %d %v", slug, code, out)
	}
}

// TestEstimateRollupWorstTier drives FR-1.2 and FR-2.1 over HTTP: a feature
// estimated as a considered unit, another as rough, and one left unestimated;
// the initiative rolls up to the worst tier with the `?` naming the gap.
func TestEstimateRollupWorstTier(t *testing.T) {
	h := newHarness(t)
	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Auth"}); code != 201 {
		t.Fatalf("initiative: %d %v", code, out)
	}
	h.createFeature("auth", "login", "Login", "the login form")
	h.createFeature("auth", "logout", "Logout", "the logout link")
	h.createFeature("auth", "reset", "Reset", "password reset")

	// login: considered (cites corpus); logout: rough; reset: unestimated.
	if code, _ := h.call("POST", "/api/estimate/set", map[string]any{
		"ref": "auth/login", "tokens": 3000, "cite_corpus": true}); code != 201 {
		t.Fatalf("set login estimate: %d", code)
	}
	if code, _ := h.call("POST", "/api/estimate/set", map[string]any{
		"ref": "auth/logout", "tokens": 5000}); code != 201 {
		t.Fatalf("set logout estimate: %d", code)
	}

	_, feat := h.call("GET", "/api/estimate?ref=auth/login", nil)
	if feat["tier"] != "considered" {
		t.Errorf("login tier = %v, want considered", feat["tier"])
	}

	_, init := h.call("GET", "/api/estimate?ref=auth", nil)
	if init["tokens"].(float64) != 8000 {
		t.Errorf("initiative tokens = %v, want 8000", init["tokens"])
	}
	if init["tier"] != "rough" {
		t.Errorf("initiative tier = %v, want rough (worst-tier taints)", init["tier"])
	}
	if init["complete"] != false {
		t.Errorf("initiative should be incomplete (reset unestimated): %v", init["complete"])
	}
	un, _ := init["unestimated"].([]any)
	if len(un) != 1 {
		t.Fatalf("unestimated = %v, want [reset]", init["unestimated"])
	}
	if first, _ := un[0].(map[string]any); first["name"] != "reset" {
		t.Errorf("unestimated names %v, want reset", un[0])
	}
}

// TestAIEstimatorConsidered drives FR-4.1: the estimator dispatch runs through
// the real loop, is shown a corpus neighbour, and its outcome lands as a
// considered estimate with a costed dispatch (FR-3.2 retrieval + FR-4.1).
func TestAIEstimatorConsidered(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Auth"}); code != 201 {
		t.Fatal("initiative")
	}
	// A completed, estimated neighbour sharing description terms with the target.
	h.createFeature("auth", "oauth-v1", "OAuth v1", "OAuth token exchange and refresh for sign in")
	h.call("POST", "/api/estimate/set", map[string]any{"ref": "auth/oauth-v1", "tokens": 4000, "cite_corpus": true})
	h.markDone("auth/oauth-v1")

	// The target, whose description shares terms with the neighbour.
	h.createFeature("auth", "oauth-v2", "OAuth v2", "OAuth token refresh rotation for sign in")

	h.mock.RespondOutcome("submit_estimate",
		`{"tokens": 4500, "rationale": "close to OAuth v1 (4000 est), a little more for rotation"}`,
		provider.Usage{Input: 1200, Output: 150})

	code, out := h.call("POST", "/api/estimate/ai", map[string]string{"ref": "auth/oauth-v2"})
	if code != 202 {
		t.Fatalf("estimate ai: %d %v", code, out)
	}

	ctx := context.Background()
	h.eventually("AI estimate recorded", func() bool {
		e, err := store.CurrentEstimate(ctx, h.srv.Store.Pool, "feature", h.featureID("auth/oauth-v2"))
		return err == nil && e.Tokens == 4500
	})
	e, _ := store.CurrentEstimate(ctx, h.srv.Store.Pool, "feature", h.featureID("auth/oauth-v2"))
	if string(e.Tier) != "considered" {
		t.Errorf("AI tier = %s, want considered (corpus had a neighbour)", e.Tier)
	}
	if e.DispatchID == nil {
		t.Error("AI estimate should reference its dispatch")
	}
	// The estimator was shown the neighbour as a reference point.
	if len(h.mock.Requests) != 1 {
		t.Fatalf("provider calls = %d", len(h.mock.Requests))
	}
	user := h.mock.Requests[0].Messages[0].Blocks[0].Text
	if !contains(user, "OAuth v1") || !contains(user, "4000") {
		t.Error("estimator prompt should carry the corpus reference point")
	}
	// The dispatch was costed (1200 in ×3/M + 150 out ×15/M = 0.00585).
	fc, err := store.FeatureCost(ctx, h.srv.Store.Pool, h.featureID("auth/oauth-v2"))
	if err != nil || fc <= 0 {
		t.Errorf("estimator dispatch should be costed: %v %v", fc, err)
	}
}

// TestAIEstimatorRoughWhenCorpusEmpty is the other half of FR-4.1: no corpus →
// the estimate is rough, honestly.
func TestAIEstimatorRoughWhenCorpusEmpty(t *testing.T) {
	h := newHarness(t)
	h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Auth"})
	h.createFeature("auth", "brandnew", "Brand new", "a wholly novel capability with no precedent")

	h.mock.RespondOutcome("submit_estimate",
		`{"tokens": 9000, "rationale": "pure judgement, no similar prior work"}`,
		provider.Usage{Input: 400, Output: 80})
	if code, _ := h.call("POST", "/api/estimate/ai", map[string]string{"ref": "auth/brandnew"}); code != 202 {
		t.Fatal("estimate ai")
	}
	ctx := context.Background()
	h.eventually("rough estimate recorded", func() bool {
		e, err := store.CurrentEstimate(ctx, h.srv.Store.Pool, "feature", h.featureID("auth/brandnew"))
		return err == nil && e.Tokens == 9000
	})
	e, _ := store.CurrentEstimate(ctx, h.srv.Store.Pool, "feature", h.featureID("auth/brandnew"))
	if string(e.Tier) != "rough" {
		t.Errorf("tier = %s, want rough (empty corpus)", e.Tier)
	}
}

// TestMilestoneLockAndSnapshot drives FR-5: live membership over an initiative,
// G4 refusal then pass, snapshot, post-lock freeze.
func TestMilestoneLockAndSnapshot(t *testing.T) {
	h := newHarness(t)
	h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Auth"})
	h.createFeature("auth", "login", "Login", "")
	h.createFeature("auth", "logout", "Logout", "")

	if code, _ := h.call("POST", "/api/milestones", map[string]string{"name": "v1"}); code != 201 {
		t.Fatal("create milestone")
	}
	// Adding the initiative includes both features transitively (FR-5.1).
	if code, out := h.call("POST", "/api/milestones/members", map[string]string{
		"milestone": "v1", "ref": "auth", "action": "add"}); code != 200 {
		t.Fatalf("add member: %d %v", code, out)
	}
	_, show := h.call("GET", "/api/milestone?ref=v1", nil)
	prog := show["progress"].(map[string]any)
	if prog["total"].(float64) != 2 || prog["done"].(float64) != 0 {
		t.Fatalf("progress = %v, want 2/0", prog)
	}

	// G4 refuses with nothing done.
	if code, out := h.call("POST", "/api/milestones/lock", map[string]string{"milestone": "v1"}); code != 409 {
		t.Fatalf("lock should be refused: %d %v", code, out)
	}

	// Finish one feature; lock now snapshots the two leaves.
	h.markDone("auth/login")
	if code, out := h.call("POST", "/api/milestones/lock", map[string]string{"milestone": "v1"}); code != 200 {
		t.Fatalf("lock after one done: %d %v", code, out)
	}
	_, show2 := h.call("GET", "/api/milestone?ref=v1", nil)
	if show2["milestone"].(map[string]any)["State"] != "locked" {
		t.Errorf("milestone should be locked: %v", show2["milestone"])
	}
	prog2 := show2["progress"].(map[string]any)
	if prog2["total"].(float64) != 2 || prog2["done"].(float64) != 1 {
		t.Errorf("snapshot progress = %v, want 2/1", prog2)
	}

	// Membership is frozen after lock (FR-5.2).
	if code, _ := h.call("POST", "/api/milestones/members", map[string]string{
		"milestone": "v1", "ref": "auth/logout", "action": "remove", "reason": "x"}); code != 409 {
		t.Errorf("post-lock membership change should be refused, got %d", code)
	}
}

// TestMilestoneDescopeBeforeLock drives FR-5.3: descope with a reason, then
// lock; the snapshot excludes the descoped feature and the removal is audited.
func TestMilestoneDescopeBeforeLock(t *testing.T) {
	h := newHarness(t)
	h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Auth"})
	h.createFeature("auth", "login", "Login", "")
	h.createFeature("auth", "sso", "SSO", "")
	h.call("POST", "/api/milestones", map[string]string{"name": "v1"})
	h.call("POST", "/api/milestones/members", map[string]string{"milestone": "v1", "ref": "auth/login", "action": "add"})
	h.call("POST", "/api/milestones/members", map[string]string{"milestone": "v1", "ref": "auth/sso", "action": "add"})
	h.markDone("auth/login")

	// Descope SSO with a reason, then lock.
	if code, _ := h.call("POST", "/api/milestones/members", map[string]string{
		"milestone": "v1", "ref": "auth/sso", "action": "remove", "reason": "slipped to v2"}); code != 200 {
		t.Fatal("descope")
	}
	if code, _ := h.call("POST", "/api/milestones/lock", map[string]string{"milestone": "v1"}); code != 200 {
		t.Fatal("lock after descope")
	}
	_, show := h.call("GET", "/api/milestone?ref=v1", nil)
	prog := show["progress"].(map[string]any)
	if prog["total"].(float64) != 1 {
		t.Errorf("snapshot should exclude the descoped feature: %v", prog)
	}
	// The descope is on the audit trail.
	found := false
	for _, e := range h.callList("GET", "/api/log?ref_type=milestone&limit=100") {
		if e["Kind"] == "milestone.member_removed" {
			found = true
		}
	}
	if !found {
		t.Error("descope should be audited as milestone.member_removed")
	}
}

// TestRoadmapOrdering drives FR-6.1: milestones ordered in a roadmap; the
// order persists.
func TestRoadmapOrdering(t *testing.T) {
	h := newHarness(t)
	h.call("POST", "/api/milestones", map[string]string{"name": "v1"})
	h.call("POST", "/api/milestones", map[string]string{"name": "v2"})
	h.call("POST", "/api/roadmaps", map[string]string{"name": "2026"})
	h.call("POST", "/api/roadmaps/entries", map[string]any{"roadmap": "2026", "milestone": "v2", "position": 1})
	h.call("POST", "/api/roadmaps/entries", map[string]any{"roadmap": "2026", "milestone": "v1", "position": 0})

	_, out := h.call("GET", "/api/roadmap?ref=2026", nil)
	entries, _ := out["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries = %v", out["entries"])
	}
	if entries[0].(map[string]any)["milestone"] != "v1" || entries[1].(map[string]any)["milestone"] != "v2" {
		t.Errorf("order = %v, want [v1, v2]", entries)
	}
}

// TestExtendedCostRollups drives FR-7: per-initiative, per-milestone, per-month.
func TestExtendedCostRollups(t *testing.T) {
	h := newHarness(t)
	h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Auth"})
	h.createFeature("auth", "login", "Login", "")
	ctx := context.Background()
	fid := h.featureID("auth/login")
	// Seed a costed dispatch on the feature.
	seedDispatchInto(h, "feature", fid, 0.0042)

	_, roll := h.call("GET", "/api/cost/rollup?ref=auth", nil)
	if roll["kind"] != "initiative" || !approxAny(roll["cost_usd"], 0.0042) {
		t.Errorf("initiative cost = %v, want 0.0042", roll)
	}

	months := h.callList("GET", "/api/cost/months")
	if len(months) != 1 || !approxAny(months[0]["CostUSD"], 0.0042) {
		t.Errorf("months = %v, want one month at 0.0042", months)
	}
	_ = ctx
}

// --- helpers ---

func (h *harness) featureID(path string) uuid.UUID {
	h.t.Helper()
	f, err := h.srv.featureByPath(context.Background(), path)
	if err != nil {
		h.t.Fatal(err)
	}
	return f.ID
}

func seedDispatchInto(h *harness, refType string, refID uuid.UUID, cost float64) {
	h.t.Helper()
	ctx := context.Background()
	err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		d, err := store.EnqueueDispatch(ctx, tx, "estimate", "estimator", "claude-sonnet-5", refType, refID, fmt.Sprintf("seed:%s", refID))
		if err != nil || d == nil {
			return fmt.Errorf("enqueue: %v", err)
		}
		if _, err := store.MarkDispatchRunning(ctx, tx, d.ID, nil); err != nil {
			return err
		}
		return store.MarkDispatchSucceeded(ctx, tx, d.ID, store.TokenUsage{Input: 100}, cost, map[string]string{})
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func approxAny(v any, want float64) bool {
	f, ok := v.(float64)
	if !ok {
		return false
	}
	d := f - want
	return d < 1e-9 && d > -1e-9
}
