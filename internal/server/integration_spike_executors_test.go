package server

// Stage 2's foundations (SPEC-021 FR-11, FR-12): the start for the chat agent
// or a person, and the agent execution a spike runner's run writes.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"subutai/internal/config"
	"subutai/internal/store"
)

// FR-12.2: starting a chat spike queues nothing, makes a detached worktree at
// the base commit under the main checkout, leaves no branch, and records the
// time box and the deadline.
func TestChatSpikeStartMakesAWorktreeAndQueuesNothing(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	branchesBefore := h.gitOut("branch", "--list")
	sp := h.newSpike(in, "Can the chat agent run this?", nil)

	started, err := h.srv.StartSpike(ctx, sp.ID, SpikeStartRequest{Executor: store.ExecutorChat, TimeBoxHours: 4}, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if started.State != store.SpikeRunning || started.Executor != store.ExecutorChat || started.TokenBudget != nil ||
		started.TimeBoxHours == nil || *started.TimeBoxHours != 4 || started.DeadlineAt == nil ||
		started.DeadlineAt.Sub(*started.StartedAt).Truncate(time.Second) != 4*time.Hour {
		t.Fatalf("started = %+v", started)
	}
	var runs int
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM dispatches WHERE ref_type = 'spike' AND ref_id = $1`, sp.ID).Scan(&runs); err != nil || runs != 0 {
		t.Errorf("dispatches = %d (%v), want none", runs, err)
	}
	abs := h.srv.worktreeAbs(started.WorktreePath)
	if _, err := os.Stat(abs + "/.git"); err != nil {
		t.Fatalf("the worktree wasn't made: %v", err)
	}
	if head, _ := gitIn(abs, "rev-parse", "HEAD"); strings.TrimSpace(head) != started.BaseCommit {
		t.Errorf("worktree HEAD = %s, want %s", strings.TrimSpace(head), started.BaseCommit)
	}
	if _, err := gitIn(abs, "symbolic-ref", "-q", "HEAD"); err == nil {
		t.Error("the worktree is on a branch, not detached")
	}
	if got := h.gitOut("branch", "--list"); got != branchesBefore {
		t.Errorf("a branch was made:\nbefore %q\nafter  %q", branchesBefore, got)
	}
	if !strings.Contains(h.worktreeList(), "spk-") {
		t.Errorf("git doesn't list the worktree:\n%s", h.worktreeList())
	}
	// Nothing was left in the leak check's way: no checkpoint was raised.
	var cps int
	if err := h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM checkpoints WHERE kind = 'spike-code-kept'`).Scan(&cps); err != nil || cps != 0 {
		t.Errorf("%d spike-code-kept checkpoints after a start (%v)", cps, err)
	}
	// Nothing was written to the executions: the claim does that.
	if execs, err := store.ExecutionsFor(ctx, h.srv.Store.Pool, "spike", sp.ID); err != nil || len(execs) != 0 {
		t.Errorf("executions = %+v, %v", execs, err)
	}
	if !spikeHadWorkingCopy(started) {
		t.Error("a started chat spike has had a working copy")
	}

	// A person spike starts the same way, with the project's default time box
	// when none is given.
	p := h.newSpike(in, "Can I run this by hand?", nil)
	pp, err := h.srv.StartSpike(ctx, p.ID, SpikeStartRequest{Executor: store.ExecutorPerson}, "sam")
	if err != nil {
		t.Fatal(err)
	}
	if pp.Executor != store.ExecutorPerson || pp.TimeBoxHours == nil || *pp.TimeBoxHours != config.DefaultSpikeTimeBoxHours {
		t.Errorf("person spike = %+v", pp)
	}
}

// FR-12.2: a time box outside 1 to 168 is refused with the sentence, and the
// spike stays an idea.
func TestSpikeTimeBoxIsChecked(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "How long?", nil)
	for _, hours := range []int{-1, 169, 1000} {
		_, err := h.srv.StartSpike(ctx, sp.ID, SpikeStartRequest{Executor: store.ExecutorChat, TimeBoxHours: hours}, "sam")
		if !errors.Is(err, ErrSpikeTimeBox) || err.Error() != "A spike's time box is a whole number of hours, from 1 to 168." {
			t.Errorf("%d hours: %v", hours, err)
		}
		if cur := h.getSpike(sp.ID); cur.State != store.SpikeIdea || cur.Executor != "" {
			t.Errorf("after %d hours the spike is %s, executor %q", hours, cur.State, cur.Executor)
		}
	}
	if _, err := os.Stat(h.spikeDir(sp)); !os.IsNotExist(err) {
		t.Errorf("a refused start made a worktree: %v", err)
	}
	// The edges are allowed, and an agent spike takes no time box.
	for i, hours := range []int{1, 168} {
		s := h.newSpike(in, "Edge "+string(rune('a'+i)), nil)
		got, err := h.srv.StartSpike(ctx, s.ID, SpikeStartRequest{Executor: store.ExecutorPerson, TimeBoxHours: hours}, "sam")
		if err != nil || got.TimeBoxHours == nil || *got.TimeBoxHours != hours {
			t.Errorf("%d hours: %+v, %v", hours, got, err)
		}
	}
	// Starting twice is refused as before.
	if _, err := h.srv.StartSpike(ctx, sp.ID, SpikeStartRequest{Executor: store.ExecutorChat, TimeBoxHours: 2}, "sam"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.srv.StartSpike(ctx, sp.ID, SpikeStartRequest{Executor: store.ExecutorChat}, "sam"); !errors.Is(err, ErrSpikeStarted) {
		t.Errorf("second start: %v", err)
	}
}

// FR-12.2: the agent execution of a dispatched spike is written when the run
// is marked running, once, and a retried attempt writes nothing new.
func TestSpikeRunWritesItsAgentExecutionOnce(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Who ran this?", nil)
	h.mock.RespondOutcome("finish_spike", findingsJSON(goodFindings), tiny)
	started := h.startSpike(sp, 0)
	if started.Executor != store.ExecutorAgent || started.TokenBudget == nil || started.DeadlineAt != nil {
		t.Fatalf("started = %+v", started)
	}
	sp = h.spikeSettled(sp)

	execs, err := store.ExecutionsFor(ctx, h.srv.Store.Pool, "spike", sp.ID)
	if err != nil || len(execs) != 1 {
		t.Fatalf("executions = %+v, %v", execs, err)
	}
	e := execs[0]
	if e.Kind != store.ExecutorAgent || e.Round != 1 || !e.Measured || e.Inferred || e.Via != "agent" || e.DispatchID == nil {
		t.Errorf("execution = %+v", e)
	}
	runs := h.runsFor("spike", sp.ID)
	if len(runs) != 1 || *e.DispatchID != runs[0].ID || e.Actor != runs[0].Role || e.Model != runs[0].Model {
		t.Errorf("execution %+v doesn't match the run %+v", e, runs)
	}
	// The retried attempt: the same dispatch marked running again.
	err = h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.RecordAgentExecution(ctx, tx, "spike", sp.ID, runs[0].ID, runs[0].Role, runs[0].Model, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	if execs, _ = store.ExecutionsFor(ctx, h.srv.Store.Pool, "spike", sp.ID); len(execs) != 1 {
		t.Errorf("a retry wrote a second row: %+v", execs)
	}
}

// SD-27: ending takes the worktree's lock and then spikeEndMu, and the
// reconciliation's finish does too; neither deadlocks with the other, with the
// remake, or with a lock the caller already holds in the stated order.
func TestSpikeLocksAreTakenInOrder(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := h.spikeInitiative("pf")
	sp := h.newSpike(in, "Locks?", nil)
	started, err := h.srv.StartSpike(ctx, sp.ID, SpikeStartRequest{Executor: store.ExecutorChat, TimeBoxHours: 1}, "sam")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		// A caller holding the worktree's lock may remake it (which takes
		// spikeEndMu for the check) and then end under both locks.
		done <- h.srv.withWorkingCopy(h.srv.worktreeAbs(started.WorktreePath), func() error {
			if _, err := h.srv.ensureSpikeWorktree(ctx, started); err != nil {
				return err
			}
			h.srv.spikeEndMu.Lock()
			defer h.srv.spikeEndMu.Unlock()
			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the locks deadlocked")
	}
	// EndSpike and the sweep both run through withSpikeEndLocks.
	if err := h.srv.EndSpike(ctx, started.ID, store.SpikeFailed, "Stopped by the test."); err != nil {
		t.Fatal(err)
	}
	if cur := h.getSpike(started.ID); cur.State != store.SpikeEnded || cur.WorktreeRemovedAt == nil {
		t.Errorf("state = %s, removed %v", cur.State, cur.WorktreeRemovedAt)
	}
	h.srv.ReconcileSpikes(ctx)
}
