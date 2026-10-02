package server

// The helpers the spike claim, time box and page tests share (SPEC-021
// FR-12 to FR-16): starting a chat or person spike, claiming it, backdating
// its deadline and reading its latest claim.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/store"
)

// startedSpike writes down a question and starts it for an executor with a
// four-hour time box, without kicking the dispatcher.
func (h *harness) startedSpike(executor, question string) *store.Spike {
	h.t.Helper()
	in := h.spikeInitiative("sp" + strings.ToLower(uuid.NewString()[:6]))
	sp := h.newSpike(in, question, nil)
	started, err := h.srv.startSpike(context.Background(), sp.ID, SpikeStartRequest{Executor: executor, TimeBoxHours: 4}, "sam", false)
	if err != nil {
		h.t.Fatal(err)
	}
	return started
}

func (h *harness) chatSpike() *store.Spike {
	return h.startedSpike(store.ExecutorChat, "Does the queue keep its order?")
}

// claimSpike claims as the chat agent, and fails the test if refused.
func (h *harness) claimSpike(sp *store.Spike) *SpikeClaimResult {
	h.t.Helper()
	res, err := h.srv.ClaimSpike(context.Background(), sp.PublicID, h.srv.ChatClaimant())
	if err != nil {
		h.t.Fatalf("claim %s: %v", sp.PublicID, err)
	}
	return res
}

func (h *harness) spikeExecs(sp *store.Spike) []store.Execution {
	h.t.Helper()
	es, err := store.ExecutionsFor(context.Background(), h.srv.Store.Pool, "spike", sp.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return es
}

func (h *harness) latestSpikeClaim(sp *store.Spike) *store.Claim {
	h.t.Helper()
	c, err := store.LatestClaimFor(context.Background(), h.srv.Store.Pool, "spike", sp.ID)
	if err != nil {
		h.t.Fatalf("latest claim of %s: %v", sp.PublicID, err)
	}
	return c
}

// pastDeadline backdates the spike's deadline, and its claim's, by an hour.
func (h *harness) pastDeadline(sp *store.Spike) {
	h.t.Helper()
	h.exec(`UPDATE spikes SET deadline_at = now() - interval '1 hour' WHERE id = $1`, sp.ID)
	h.exec(`UPDATE work_claims SET deadline_at = now() - interval '1 hour' WHERE ref_type = 'spike' AND ref_id = $1`, sp.ID)
}

// wantRefusalSentence checks an error is a ClaimRefusal with exactly the
// sentence; wantRefusal matches a part of one.
func wantRefusalSentence(t *testing.T, err error, want string) {
	t.Helper()
	r, ok := AsClaimRefusal(err)
	if !ok {
		t.Fatalf("want the refusal %q, got %v", want, err)
	}
	if r.Sentence != want {
		t.Fatalf("refusal = %q\nwant      %q", r.Sentence, want)
	}
}

const spikeDraft = "## Answer\n\nYes, it can be done.\n\n## What we found\n\nThe saved draft says it works.\n"

// spikeWithDraft starts a spike run by executor, with a saved draft and, when who is
// set, a claim held by them (as claim_spike or the page's claim would make it:
// feature nil, a deadline of the spike's, an execution row).
func (h *harness) spikeWithDraft(executor, who string, hours int) *store.Spike {
	h.t.Helper()
	ctx := context.Background()
	in := h.spikeInitiative("sp" + strings.ToLower(uuid.NewString()[:6]))
	sp := h.newSpike(in, "Does the time box hold?", nil)
	started, err := h.srv.startSpike(ctx, sp.ID, SpikeStartRequest{Executor: executor, TimeBoxHours: hours}, "sam", false)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := store.SaveSpikeDraft(ctx, h.srv.Store.Pool, started.ID, spikeDraft); err != nil {
		h.t.Fatal(err)
	}
	if who != "" {
		h.claimedBy(started, who)
	}
	return h.getSpike(started.ID)
}

// claimedBy makes a claim on the spike as claim_spike (chat, over MCP) or the
// page (a person, in the web UI) would: no feature, the spike's deadline, and
// an execution row.
func (h *harness) claimedBy(sp *store.Spike, who string) *store.Claim {
	h.t.Helper()
	ctx := context.Background()
	via := "mcp"
	if sp.Executor == store.ExecutorPerson {
		via = "ui"
	}
	var c *store.Claim
	err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if c, err = store.CreateClaim(ctx, tx, "spike", sp.ID, nil, sp.Executor, who, via, ""); err != nil {
			return err
		}
		if err := store.SetClaimDeadline(ctx, tx, c.ID, *sp.DeadlineAt); err != nil {
			return err
		}
		_, err = store.RecordClaimExecution(ctx, tx, "spike", sp.ID, c, "")
		return err
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return c
}
