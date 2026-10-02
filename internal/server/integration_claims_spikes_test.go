package server

import (
	"context"
	"strings"
	"testing"

	"subutai/internal/store"
)

// TestSpikeIsClaimedOnlyByItsExecutor is SPEC-021 FR-13.9, which replaces
// M13 and M14's stop-gap test that a spike couldn't be claimed at all: a
// spike is claimed only by the executor it was started for, over the surface
// that executor uses. An agent spike and an idea are refused by claim_spike; a
// person's spike is refused over MCP; a chat spike is refused in the web UI;
// claim_task given a spike's ID points to claim_spike; and the chat spike is
// claimed over MCP.
func TestSpikeIsClaimedOnlyByItsExecutor(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	claims := func() int {
		var n int
		if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM work_claims`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	idea := h.newSpike(in, "Which queue should we use?", nil)
	if got := h.refusalOf("claim_spike", map[string]any{"spike": idea.PublicID}); !strings.Contains(got, idea.PublicID+" isn't running, so it can't be claimed.") {
		t.Errorf("claim an idea = %q", got)
	}

	agent, _ := h.startSpikeQuiet(h.newSpike(in, "Does the agent run it?", nil), 0)
	if got := h.refusalOf("claim_spike", map[string]any{"spike": agent.PublicID}); !strings.Contains(got, agent.PublicID+" is run by the spike runner, an agent, so it can't be claimed.") {
		t.Errorf("claim an agent spike = %q", got)
	}

	person := h.startedSpike(store.ExecutorPerson, "Is it quick enough by hand?")
	if got := h.refusalOf("claim_spike", map[string]any{"spike": person.PublicID}); !strings.Contains(got, person.PublicID+" is to be run by a person in the web UI, not in chat.") {
		t.Errorf("claim a person spike over MCP = %q", got)
	}

	chat := h.startedSpike(store.ExecutorChat, "Does the queue keep its order?")
	if _, err := h.srv.ClaimSpike(ctx, chat.PublicID, h.srv.PersonClaimant()); err == nil || !strings.Contains(err.Error(), chat.PublicID+" is to be run in chat, not in the web UI.") {
		t.Errorf("claim a chat spike in the UI = %v", err)
	}

	for _, sp := range []*store.Spike{idea, agent, person, chat} {
		if got := h.refusalOf("claim_task", map[string]any{"task": sp.PublicID}); !strings.Contains(got, sp.PublicID+" is a spike, not a task. Use claim_spike to run it.") {
			t.Errorf("claim_task on %s = %q", sp.PublicID, got)
		}
	}
	if n := claims(); n != 0 {
		t.Fatalf("%d claims recorded after only refusals", n)
	}

	got := h.toolOK("claim_spike", map[string]any{"spike": chat.PublicID})
	if asMap(t, got["spike"])["id"] != chat.PublicID {
		t.Errorf("claim_spike = %v", got)
	}
	if n := claims(); n != 1 {
		t.Errorf("%d claims recorded after the chat spike's claim, want 1", n)
	}
}
