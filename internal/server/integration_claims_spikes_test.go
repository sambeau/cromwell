package server

import (
	"context"
	"strings"
	"testing"
)

// TestSpikeCantBeClaimedYet joins M13 and M14, built in parallel: a spike is
// run only by a dispatched agent until stage 2 (SPEC-021 §4) makes it
// claimable through M13's interface (SPEC-020 FR-2.10), so claim_task given a
// spike's ID refuses it as it refuses anything that isn't a task, and
// nothing is claimed.
func TestSpikeCantBeClaimedYet(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	sp, err := h.srv.CreateSpike(ctx, "initiative", in, "Which queue should we use?", nil, "sam", "ui")
	if err != nil {
		t.Fatal(err)
	}
	_, isErr, text := h.callTool("claim_task", map[string]any{"task": sp.PublicID})
	// Stage 2 points claim_task at claim_spike (SPEC-021 FR-13.5); T4 replaces
	// this test with TestSpikeIsClaimedOnlyByItsExecutor.
	if want := sp.PublicID + " is a spike, not a task. Use claim_spike to run it."; !isErr || !strings.Contains(text, want) {
		t.Errorf("claim_task on %s: want %q, got %v %q", sp.PublicID, want, isErr, text)
	}
	var claims int
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM work_claims`).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if claims != 0 {
		t.Errorf("%d claims recorded; a spike can't be claimed before stage 2", claims)
	}
}
