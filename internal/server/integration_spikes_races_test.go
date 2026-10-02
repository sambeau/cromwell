package server

// Endings that race a claim or a submit, and a submit that is cut short
// (SPEC-021 FR-13.3, FR-13.7, FR-15): what the locks (SD-27) and the rule that
// a done claim is final are for.

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// spikeCodeKeptCheckpoints counts the leak check's checkpoints.
func (h *harness) spikeCodeKeptCheckpoints() int {
	h.t.Helper()
	var n int
	if err := h.srv.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM checkpoints WHERE kind = 'spike-code-kept'`).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

// endClaimDone makes a submit's step 4 and nothing after it: the holder's
// claim ends done, as SubmitSpike's transaction does, and the spike is left
// running, as it is when the request is lost before step 5.
func (h *harness) endClaimDone(sp *store.Spike, who string) {
	h.t.Helper()
	ctx := context.Background()
	err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		held, err := store.LockCurrentClaimFor(ctx, tx, "spike", sp.ID)
		if err != nil {
			return err
		}
		if err := store.TransitionClaim(ctx, tx, held, lifecycle.ClaimEventSubmit, who, "submitted", nil); err != nil {
			return err
		}
		if err := store.MarkClaimExecutionSubmitted(ctx, tx, held.ID); err != nil {
			return err
		}
		return store.TransitionClaim(ctx, tx, held, lifecycle.ClaimEventDone, who, "", nil)
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

// holdEndLocks runs fn holding the spike's ending locks, as an ending does. It
// returns once the locks are held; the returned release lets fn's ending run,
// and waits for it.
func (h *harness) holdEndLocks(sp *store.Spike, ending func() error) (release func()) {
	h.t.Helper()
	held, proceed, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- h.srv.withSpikeEndLocks(sp, func() error {
			close(held)
			<-proceed
			return ending()
		})
	}()
	<-held
	return func() {
		h.t.Helper()
		close(proceed)
		if err := <-done; err != nil {
			h.t.Fatalf("the ending: %v", err)
		}
	}
}

// hookReached makes the server's test hook tell the returned channel, once,
// when a claim or a submit reaches point.
func (h *harness) hookReached(point string) <-chan struct{} {
	h.t.Helper()
	reached := make(chan struct{})
	var once sync.Once
	h.srv.spikeHook = func(p string) {
		if p == point {
			once.Do(func() { close(reached) })
		}
	}
	return reached
}

// await waits for a hook's channel, failing the test if it takes too long.
func (h *harness) await(reached <-chan struct{}, what string) {
	h.t.Helper()
	select {
	case <-reached:
	case <-time.After(30 * time.Second):
		h.t.Fatalf("timed out waiting for %s", what)
	}
}

// A claim that waits on an ending's lock finds the spike ended when it gets
// the lock, and makes nothing: no worktree, and no question about code kept
// outside a working copy that was discarded.
func TestAClaimThatWaitedOnAnEndingMakesNothing(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.chatSpike()
	if _, err := os.Stat(h.spikeDir(sp)); err != nil {
		t.Fatalf("the start made no worktree: %v", err)
	}

	release := h.holdEndLocks(sp, func() error {
		return h.srv.endSpikeLocked(ctx, sp.ID, store.SpikeTimeBox, "")
	})
	read := h.hookReached(spikeHookClaimRead)
	claim := make(chan error, 1)
	go func() {
		_, err := h.srv.ClaimSpike(ctx, sp.PublicID, h.srv.ChatClaimant())
		claim <- err
	}()
	// The claim has read the spike running. The ending still holds the locks,
	// so the claim reads the spike again only once the ending is over.
	h.await(read, "the claim's first read")
	release()

	wantRefusalSentence(t, <-claim, sp.PublicID+" has ended. A person reads its findings on its page.")
	if _, err := os.Stat(h.spikeDir(sp)); !os.IsNotExist(err) {
		t.Errorf("the claim made the ended spike's worktree again: %v", err)
	}
	if strings.Contains(h.worktreeList(), "spk-") {
		t.Errorf("git lists a worktree for an ended spike:\n%s", h.worktreeList())
	}
	if n := h.spikeCodeKeptCheckpoints(); n != 0 {
		t.Errorf("%d spike-code-kept checkpoints after a claim that lost to an ending", n)
	}
}

// A claim that is going to be refused for its deadline doesn't remake a
// worktree the ending will discard.
func TestAClaimPastTheDeadlineMakesNothing(t *testing.T) {
	h := newHarness(t)
	sp := h.chatSpike()
	h.pastDeadline(sp)
	if err := os.RemoveAll(h.spikeDir(sp)); err != nil {
		t.Fatal(err)
	}
	_, err := h.srv.ClaimSpike(context.Background(), sp.PublicID, h.srv.ChatClaimant())
	if _, ok := AsClaimRefusal(err); !ok || !strings.Contains(err.Error(), "time box ended at") {
		t.Fatalf("claim past the deadline: %v", err)
	}
	if _, err := os.Stat(h.spikeDir(sp)); !os.IsNotExist(err) {
		t.Errorf("a refused claim made the worktree: %v", err)
	}
}

// A submit whose ending was cut short (its claim ended done, the spike
// still running) can't be undone by a new claim. The claim is refused, and
// reconciliation ends the spike concluded even when a claim was made after the
// submit and the deadline has passed.
func TestADoneClaimIsFinal(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.spikeWithDraft(store.ExecutorPerson, h.srv.PersonClaimant().Actor, 4)
	h.endClaimDone(sp, h.srv.PersonClaimant().Actor)
	if cur := h.getSpike(sp.ID); cur.State != store.SpikeRunning {
		t.Fatalf("state = %s", cur.State)
	}

	_, err := h.srv.ClaimSpike(ctx, sp.PublicID, h.srv.PersonClaimant())
	wantRefusalSentence(t, err, sp.PublicID+" has been submitted, and is ending. A person reads its findings on its page.")
	if c := h.latestSpikeClaim(sp); c.State != lifecycle.ClaimEnded || c.EndReason != "done" {
		t.Errorf("the refused claim left %+v", c)
	}

	// A claim that got in some other way is expired by the ending, not
	// preferred to the submit.
	h.claimedBy(h.getSpike(sp.ID), h.srv.PersonClaimant().Actor)
	h.pastDeadline(sp)
	h.srv.ReconcileSpikes(ctx)

	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeConcluded {
		t.Fatalf("state = %s, how = %q; want ended, concluded", cur.State, cur.EndedHow)
	}
	_, text := h.findingsOf(sp)
	if strings.Contains(text, "reached the end of its time box") || !strings.Contains(text, "Yes, it can be done.") {
		t.Errorf("the findings after a submit:\n%s", text)
	}
	if c := h.latestSpikeClaim(sp); c.State != lifecycle.ClaimEnded {
		t.Errorf("the later claim = %+v", c)
	}
	if cur.WorktreeRemovedAt == nil {
		t.Error("the worktree wasn't discarded")
	}
}

// FR-15's acceptance: a submit and a deadline ending, run at once. The ending
// takes the locks first, so the submit, which has passed its first look, is
// refused with the sentence that says the findings were lost; the
// spike is time_box, the claim expired, and the draft is what was kept.
func TestASubmitThatLosesToTheTimeBoxIsRefused(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.spikeWithDraft(store.ExecutorPerson, h.srv.PersonClaimant().Actor, 4)

	release := h.holdEndLocks(sp, func() error {
		return h.srv.endSpikeLocked(ctx, sp.ID, store.SpikeTimeBox, "")
	})
	checked := h.hookReached(spikeHookSubmitChecked)
	submit := make(chan error, 1)
	go func() {
		_, err := h.srv.SubmitSpike(ctx, sp.PublicID, h.srv.PersonClaimant(), goodFindings)
		submit <- err
	}()
	// The submit has passed its first looks, and the ending still holds the
	// locks it needs.
	h.await(checked, "the submit's first looks")
	release()

	wantRefusalSentence(t, <-submit, spikeTimeBoxEndedBeforeSubmit)
	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeTimeBox {
		t.Errorf("state = %s, how = %q", cur.State, cur.EndedHow)
	}
	if c := h.latestSpikeClaim(sp); c.State != lifecycle.ClaimEnded || c.EndReason != "expired" {
		t.Errorf("claim = %+v", c)
	}
	_, text := h.findingsOf(sp)
	if !strings.Contains(text, "The saved draft says it works.") || strings.Contains(text, "The header is read in one place") {
		t.Errorf("the findings are the draft, not the lost submit:\n%s", text)
	}
}

// The reverse order: the submit wins, and a time box ending that comes after
// it leaves the spike concluded with the submitted findings.
func TestATimeBoxEndingAfterASubmitChangesNothing(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sp := h.spikeWithDraft(store.ExecutorPerson, h.srv.PersonClaimant().Actor, 4)
	if _, err := h.srv.SubmitSpike(ctx, sp.PublicID, h.srv.PersonClaimant(), goodFindings); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.EndSpike(ctx, sp.ID, store.SpikeTimeBox, ""); err != nil {
		t.Fatal(err)
	}
	cur := h.getSpike(sp.ID)
	if cur.State != store.SpikeEnded || cur.EndedHow != store.SpikeConcluded {
		t.Errorf("state = %s, how = %q", cur.State, cur.EndedHow)
	}
	if c := h.latestSpikeClaim(sp); c.State != lifecycle.ClaimEnded || c.EndReason != "done" {
		t.Errorf("claim = %+v", c)
	}
	if _, text := h.findingsOf(sp); !strings.Contains(text, "The header is read in one place") {
		t.Errorf("the findings lost the submitted text:\n%s", text)
	}
}

// A start whose worktree couldn't be made leaves a spike that never had
// a working copy, so its first claim makes one without asking where code went.
func TestAFailedStartMakeThenAClaimRaisesNoQuestion(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	root := h.srv.worktreesRoot()
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	// A file where the folder should be: the start's make fails.
	if err := os.WriteFile(root, []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := h.spikeInitiative("pf")
	started, err := h.srv.StartSpike(ctx, h.newSpike(in, "Is a failed make a loss?", nil).ID,
		SpikeStartRequest{Executor: store.ExecutorChat, TimeBoxHours: 2}, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if had, err := h.srv.spikeHadWorkingCopy(ctx, started); err != nil || had {
		t.Fatalf("a spike whose worktree was never made had a working copy: %v, %v", had, err)
	}

	if _, err := h.srv.ClaimSpike(ctx, started.PublicID, h.srv.ChatClaimant()); err != nil {
		t.Fatal(err)
	}
	if n := h.spikeCodeKeptCheckpoints(); n != 0 {
		t.Errorf("%d spike-code-kept checkpoints after a claim that made the first worktree", n)
	}
	if had, err := h.srv.spikeHadWorkingCopy(ctx, started); err != nil || !had {
		t.Errorf("after the claim made it: %v, %v", had, err)
	}

	if err := h.srv.EndSpike(ctx, started.ID, store.SpikeTimeBox, ""); err != nil {
		t.Fatal(err)
	}
	if _, text := h.findingsOf(started); strings.Contains(text, "couldn't check") {
		t.Errorf("the findings say the check couldn't run:\n%s", text)
	}
}

// A claim whose first read sees an idea, and that is then overtaken by the
// spike's start, takes the worktree's lock all the same, so it waits for the
// start's make: it neither makes the worktree beside the start's nor raises a
// question about code kept outside a working copy.
func TestAClaimOvertakenByAStartWaitsForTheStartsWorktree(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("ov")
	idea := h.newSpike(in, "Does a claim wait for a start?", nil)
	lockPath := h.srv.worktreeAbs(h.srv.spikeWorktreeRel(idea.ID))

	// Hold the worktree's lock, as a start's make does.
	locked, free, held := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		held <- h.srv.withWorkingCopy(lockPath, func() error {
			close(locked)
			<-free
			return nil
		})
	}()
	h.await(locked, "the test's lock")

	// The claim reads the idea, and stops at the hook until the start has
	// committed.
	var once sync.Once
	read, resume, claimLocked := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	h.srv.spikeHook = func(point string) {
		switch point {
		case spikeHookClaimRead:
			once.Do(func() { close(read); <-resume })
		case spikeHookClaimLocked:
			claimLocked <- struct{}{}
		}
	}
	claim := make(chan error, 1)
	go func() {
		_, err := h.srv.ClaimSpike(ctx, idea.PublicID, h.srv.ChatClaimant())
		claim <- err
	}()
	h.await(read, "the claim's first read")

	// The start commits, then waits for the lock to make the worktree.
	start := make(chan error, 1)
	go func() {
		_, err := h.srv.StartSpike(ctx, idea.ID, SpikeStartRequest{Executor: store.ExecutorChat, TimeBoxHours: 2}, "sam")
		start <- err
	}()
	deadline := time.Now().Add(30 * time.Second)
	for h.getSpike(idea.ID).State != store.SpikeRunning {
		if time.Now().After(deadline) {
			t.Fatal("the start never committed")
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(resume)
	select {
	case <-claimLocked:
		t.Fatal("the claim took no lock on the worktree, and went on beside the start's make")
	case <-time.After(300 * time.Millisecond):
	}
	close(free)
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	if err := <-start; err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := <-claim; err != nil {
		t.Fatalf("claim: %v", err)
	}

	if _, err := os.Stat(h.spikeDir(idea)); err != nil {
		t.Errorf("no worktree after the start and the claim: %v", err)
	}
	if n := strings.Count(h.worktreeList(), "spk-"); n != 1 {
		t.Errorf("git lists %d spike worktrees, want 1:\n%s", n, h.worktreeList())
	}
	if n := h.spikeCodeKeptCheckpoints(); n != 0 {
		t.Errorf("%d spike-code-kept checkpoints", n)
	}
	if c := h.latestSpikeClaim(idea); c.State != lifecycle.ClaimOpen {
		t.Errorf("claim = %+v", c)
	}
}
