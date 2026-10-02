package server

// The time box ends a spike, and the page says who ran it (SPEC-021 FR-15,
// FR-12.4, FR-16.1, FR-16.4).

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// checkEndedAtTimeBox checks what FR-15's acceptance asks of a spike that ended at
// its time box: the draft and the sentences in the findings, committed, the
// worktree discarded.
func (h *harness) checkEndedAtTimeBox(sp *store.Spike, sentences ...string) string {
	h.t.Helper()
	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeTimeBox {
		h.t.Fatalf("state = %s, how = %q", cur.State, cur.EndedHow)
	}
	if cur.WorktreeRemovedAt == nil {
		h.t.Error("the worktree wasn't discarded")
	}
	if _, err := os.Stat(h.spikeDir(sp)); !os.IsNotExist(err) {
		h.t.Errorf("the worktree directory is still there: %v", err)
	}
	doc, text := h.findingsOf(sp)
	for _, want := range append([]string{"Yes, it can be done.", "The saved draft says it works.",
		"The spike reached the end of its time box of 4 hours."}, sentences...) {
		if !strings.Contains(text, want) {
			h.t.Errorf("the findings don't say %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "tokens.") && strings.Contains(text, "It used") {
		h.t.Errorf("the findings state tokens used:\n%s", text)
	}
	if out := h.gitOut("ls-tree", "--name-only", "HEAD", "--", doc.Path); strings.TrimSpace(out) == "" {
		h.t.Errorf("the findings %s weren't committed", doc.Path)
	}
	return text
}

// FR-15.1, FR-15.3: a heartbeat ends a claimed chat spike past its deadline as
// time_box, and the claim ends expired.
func TestHeartbeatEndsAClaimedChatSpikeAtItsTimeBox(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.spikeWithDraft(store.ExecutorChat, "chat", 4)
	h.pastDeadline(sp)

	h.srv.ReconcileSpikes(ctx)

	text := h.checkEndedAtTimeBox(sp, "It ran in chat, so its tokens weren't measured.")
	if strings.Contains(text, "run was lost") {
		t.Errorf("a chat spike's run was lost:\n%s", text)
	}
	if c := h.latestSpikeClaim(sp); c.State != lifecycle.ClaimEnded || c.EndReason != "expired" || c.EndedBy != "" {
		t.Errorf("claim = %+v", c)
	}
	var tokens bool
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT tokens_used = 0 FROM spikes WHERE id = $1`, sp.ID).Scan(&tokens); err != nil || !tokens {
		t.Errorf("tokens_used changed (%v)", err)
	}
	// Once is enough: a second heartbeat finds nothing to do.
	h.srv.ReconcileSpikes(ctx)
	if n := h.auditKinds("spike.ended", sp.ID); n != 1 {
		t.Errorf("%d spike.ended rows", n)
	}
}

// FR-15.1: the same for an unclaimed chat spike, and for a person spike.
func TestHeartbeatEndsAnUnclaimedSpikeAtItsTimeBox(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	chat := h.spikeWithDraft(store.ExecutorChat, "", 4)
	person := h.spikeWithDraft(store.ExecutorPerson, "", 4)
	held := h.spikeWithDraft(store.ExecutorPerson, "sam", 4)
	for _, sp := range []*store.Spike{chat, person, held} {
		h.pastDeadline(sp)
	}

	h.srv.ReconcileSpikes(ctx)

	h.checkEndedAtTimeBox(chat, "It ran in chat, so its tokens weren't measured.")
	h.checkEndedAtTimeBox(person, "It was run by hand, so its tokens weren't measured.")
	h.checkEndedAtTimeBox(held, "It was run by hand, so its tokens weren't measured.")
	if c := h.latestSpikeClaim(held); c.EndReason != "expired" {
		t.Errorf("the person's claim = %+v", c)
	}
	// A time box of one hour is singular.
	one := h.spikeWithDraft(store.ExecutorChat, "", 1)
	h.pastDeadline(one)
	h.srv.ReconcileSpikes(ctx)
	if _, text := h.findingsOf(one); !strings.Contains(text, "its time box of 1 hour.") {
		t.Errorf("the findings:\n%s", text)
	}
}

// FR-15.1, FR-12.4: a running chat spike before its deadline is left alone,
// with its directory, and is not "lost".
func TestReconcileLeavesARunningChatSpikeBeforeItsDeadline(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.spikeWithDraft(store.ExecutorChat, "chat", 4)
	other := h.spikeWithDraft(store.ExecutorPerson, "", 4)

	h.srv.ReconcileSpikes(ctx)

	for _, s := range []*store.Spike{sp, other} {
		cur := h.getSpike(s.ID)
		if cur.State != store.SpikeRunning || cur.WorktreeRemovedAt != nil {
			t.Errorf("%s: state %s, removed %v", s.PublicID, cur.State, cur.WorktreeRemovedAt)
		}
		if _, err := os.Stat(h.spikeDir(s) + "/.git"); err != nil {
			t.Errorf("%s: the directory was taken: %v", s.PublicID, err)
		}
	}
	if c := h.latestSpikeClaim(sp); c.State != lifecycle.ClaimOpen {
		t.Errorf("claim = %+v", c)
	}
}

// FR-15.2: a running chat spike whose claim ended done is ended concluded by
// reconciliation, even when its deadline has also passed.
func TestReconcileEndsADoneClaimConcludedBeforeTheDeadline(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	chat := h.spikeWithDraft(store.ExecutorChat, "chat", 4)
	person := h.spikeWithDraft(store.ExecutorPerson, "sam", 4)
	for _, sp := range []*store.Spike{chat, person} {
		c := h.latestSpikeClaim(sp)
		err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
			if err := store.TransitionClaim(ctx, tx, c, lifecycle.ClaimEventSubmit, c.Actor, "", nil); err != nil {
				return err
			}
			return store.TransitionClaim(ctx, tx, c, lifecycle.ClaimEventDone, c.Actor, "", nil)
		})
		if err != nil {
			t.Fatal(err)
		}
		h.pastDeadline(sp)
	}

	h.srv.ReconcileSpikes(ctx)

	for _, c := range []struct {
		sp   *store.Spike
		want string
	}{
		{chat, "The chat agent concluded the spike. It ran in chat, so its tokens weren't measured."},
		{person, "sam concluded the spike by hand. It was run by hand, so its tokens weren't measured."},
	} {
		cur := h.getSpike(c.sp.ID)
		if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeConcluded || cur.WorktreeRemovedAt == nil {
			t.Fatalf("%s: state %s, how %q, removed %v", c.sp.PublicID, cur.State, cur.EndedHow, cur.WorktreeRemovedAt)
		}
		_, text := h.findingsOf(c.sp)
		if !strings.Contains(text, c.want) || strings.Contains(text, "end of its time box") {
			t.Errorf("the findings don't say %q:\n%s", c.want, text)
		}
		if got := h.latestSpikeClaim(c.sp); got.EndReason != "done" {
			t.Errorf("the claim's end reason = %q", got.EndReason)
		}
	}
	// Asked for the time box when the claim is done, EndSpike still concludes.
	late := h.spikeWithDraft(store.ExecutorChat, "chat", 4)
	c := h.latestSpikeClaim(late)
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.TransitionClaim(ctx, tx, c, lifecycle.ClaimEventSubmit, c.Actor, "", nil); err != nil {
			return err
		}
		return store.TransitionClaim(ctx, tx, c, lifecycle.ClaimEventDone, c.Actor, "", nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.EndSpike(ctx, late.ID, store.SpikeTimeBox, ""); err != nil {
		t.Fatal(err)
	}
	if cur := h.getSpike(late.ID); cur.EndedHow != store.SpikeConcluded {
		t.Errorf("a done claim ended %q", cur.EndedHow)
	}
}

// FR-6.3, FR-15.2: a branch made in the worktree before the deadline is
// reported, as stage 1's leak check does; a worktree deleted and pruned fails
// closed.
func TestTimeBoxEndingRunsTheLeakCheck(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	made := h.spikeWithDraft(store.ExecutorChat, "chat", 4)
	abs := h.srv.worktreeAbs(made.WorktreePath)
	for _, args := range [][]string{
		{"config", "user.email", "t@example.com"}, {"config", "user.name", "T"},
		{"commit", "--allow-empty", "-m", "spike work"}, {"branch", "keep-tb"},
	} {
		if _, err := gitIn(abs, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	h.pastDeadline(made)

	gone := h.spikeWithDraft(store.ExecutorPerson, "sam", 4)
	if err := os.RemoveAll(h.srv.worktreeAbs(gone.WorktreePath)); err != nil {
		t.Fatal(err)
	}
	h.gitOut("worktree", "prune")
	h.pastDeadline(gone)

	h.srv.ReconcileSpikes(ctx)

	_, text := h.findingsOf(made)
	if !strings.Contains(text, "Code from this spike was kept on `keep-tb`") {
		t.Errorf("the findings don't report the branch:\n%s", text)
	}
	if out := h.gitOut("branch", "--list", "keep-tb"); !strings.Contains(out, "keep-tb") {
		t.Error("the branch was deleted")
	}
	_, text = h.findingsOf(gone)
	if want := "Subutai couldn't check whether code from this spike was kept: the working copy was removed, so git's record of it is gone."; !strings.Contains(text, want) {
		t.Errorf("the findings don't say %q:\n%s", want, text)
	}
	if cur := h.getSpike(gone.ID); cur.EndedHow != store.SpikeTimeBox || cur.State != store.SpikeEnded {
		t.Errorf("state %s, how %q", cur.State, cur.EndedHow)
	}
}

// FR-15.2: the executor decides what a spike can end as.
func TestEndSpikeRefusesAWayItsExecutorCantHave(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	agent := h.newSpike(in, "An agent's question", nil)
	if _, _, err := h.startAgentSpikeQuiet(agent); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.EndSpike(ctx, agent.ID, store.SpikeTimeBox, ""); err == nil || !strings.Contains(err.Error(), "token budget, not a time box") {
		t.Errorf("time_box on an agent spike: %v", err)
	}
	if cur := h.getSpike(agent.ID); cur.State != store.SpikeRunning {
		t.Errorf("the agent spike is %s", cur.State)
	}
	chat := h.spikeWithDraft(store.ExecutorChat, "", 4)
	for _, how := range []string{store.SpikeBudget, store.SpikeTurnLimit} {
		if err := h.srv.EndSpike(ctx, chat.ID, how, ""); err == nil || !strings.Contains(err.Error(), "time box, not a token budget") {
			t.Errorf("%s on a chat spike: %v", how, err)
		}
	}
	if cur := h.getSpike(chat.ID); cur.State != store.SpikeRunning {
		t.Errorf("the chat spike is %s", cur.State)
	}
	if err := h.srv.EndSpike(ctx, h.newSpike(in, "An idea", nil).ID, store.SpikeTimeBox, ""); err == nil {
		t.Error("an idea has no run to end")
	}
}

func (h *harness) startAgentSpikeQuiet(sp *store.Spike) (*store.Spike, *store.Dispatch, error) {
	started, err := h.srv.startSpike(context.Background(), sp.ID, SpikeStartRequest{Executor: store.ExecutorAgent}, "sam", false)
	return started, nil, err
}

// closedBySentence is the page's "Closed by ..." sentence for sp.
func (h *harness) closedBySentence(sp *store.Spike) string {
	h.t.Helper()
	facts, err := readSpikeRunFacts(context.Background(), h.srv.Store.Pool, sp)
	if err != nil {
		h.t.Fatal(err)
	}
	return spikeClosedBySentence(sp, facts)
}

// FR-16.4, SD-26: the person who ran a spike by hand may close it, and the
// audit row and the page's sentence say so.
func TestCloserWhoRanTheSpikeIsRecorded(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ranByMe := h.spikeWithDraft(store.ExecutorPerson, "sam", 4)
	other := h.spikeWithDraft(store.ExecutorPerson, "pat", 4)
	chat := h.spikeWithDraft(store.ExecutorChat, "chat", 4)
	for _, sp := range []*store.Spike{ranByMe, other, chat} {
		h.pastDeadline(sp)
	}
	h.srv.ReconcileSpikes(ctx)

	closed, err := h.srv.CloseSpike(ctx, ranByMe.ID, store.SpikeAnswered, nil, "sam")
	if err != nil {
		t.Fatal(err)
	}
	var ran *bool
	row := func(id uuid.UUID) (v *bool) {
		var s *string
		if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT payload->>'closer_ran_it' FROM audit_events
			WHERE kind = 'spike.closed' AND ref_id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		if s != nil {
			b := *s == "true"
			v = &b
		}
		return v
	}
	if ran = row(ranByMe.ID); ran == nil || !*ran {
		t.Errorf("closer_ran_it = %v", ran)
	}
	if got, want := h.closedBySentence(closed), "Closed by sam, who also ran it."; got != want {
		t.Errorf("sentence = %q, want %q", got, want)
	}

	// Someone else closing it, or closing a chat spike, isn't "who ran it".
	for _, sp := range []*store.Spike{other, chat} {
		closed, err := h.srv.CloseSpike(ctx, sp.ID, store.SpikeUnanswered, nil, "sam")
		if err != nil {
			t.Fatal(err)
		}
		if ran = row(sp.ID); ran != nil {
			t.Errorf("%s: closer_ran_it = %v", sp.PublicID, *ran)
		}
		if got := h.closedBySentence(closed); got != "" {
			t.Errorf("%s: sentence = %q", sp.PublicID, got)
		}
	}
}

// FR-15.5: asking again after a chat or person spike makes a spike with no
// budget override, whatever the dialog sent.
func TestAskAgainAfterAChatSpikeHasNoBudget(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for _, executor := range []string{store.ExecutorChat, store.ExecutorPerson} {
		sp := h.spikeWithDraft(executor, "", 4)
		h.pastDeadline(sp)
		h.srv.ReconcileSpikes(ctx)
		next, err := h.srv.CloseSpike(ctx, sp.ID, SpikeCloseAgain, nil, "sam")
		if err != nil {
			t.Fatal(err)
		}
		if next.ID == sp.ID || next.State != store.SpikeIdea || next.BudgetOverride != nil || next.FollowsID == nil || *next.FollowsID != sp.ID {
			t.Errorf("%s: new spike = %+v", executor, next)
		}
		if old := h.getSpike(sp.ID); old.State != store.SpikeClosed || old.ClosedAs != store.SpikeUnanswered {
			t.Errorf("%s: old spike = %s, %q", executor, old.State, old.ClosedAs)
		}
	}
}
