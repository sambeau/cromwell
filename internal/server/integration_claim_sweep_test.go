package server

// The safety nets (SPEC-020 FR-5): the claim sweep, its two questions and
// their answers, and the branch watch with its notice. Sweeps are called
// directly; the heartbeat isn't running in the harness. Claims are made
// stale by backdating them in the database rather than waiting (FR-5.5).

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// pendingOf returns the pending checkpoint of a kind on an item, or nil.
func (h *harness) pendingOf(kind, refType string, refID uuid.UUID) *store.Checkpoint {
	h.t.Helper()
	cp, err := store.PendingCheckpointFor(context.Background(), h.srv.Store.Pool, kind, refType, refID)
	if err != nil {
		return nil
	}
	return cp
}

func (h *harness) checkpointState(id uuid.UUID) string {
	h.t.Helper()
	cp, err := store.GetCheckpoint(context.Background(), h.srv.Store.Pool, id)
	if err != nil {
		h.t.Fatal(err)
	}
	return cp.State
}

// staleClaim makes the claim's last activity (and its claiming) 26 hours old.
func (h *harness) staleClaim(c *store.Claim) {
	h.t.Helper()
	h.exec(`UPDATE work_claims SET last_activity_at = now() - interval '26 hours',
		claimed_at = now() - interval '26 hours' WHERE id = $1`, c.ID)
}

// claimedSetup starts the feature with the dispatcher held and claims T1 for
// the chat agent.
func (h *harness) claimedSetup() (store.Task, *ClaimResult) {
	h.t.Helper()
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	t1 := h.taskByLocal("T1")
	res, err := h.srv.ClaimWork(context.Background(), t1.PublicID, h.srv.ChatClaimant())
	if err != nil {
		h.t.Fatal(err)
	}
	return t1, res
}

func (h *harness) respond(cp *store.Checkpoint, verb string) (int, string) {
	h.t.Helper()
	return h.postForm("/ui/respond", map[string]string{"id": cp.ID.String(), "verb": verb})
}

func (h *harness) auditRefCount(kind string, refID uuid.UUID) int {
	h.t.Helper()
	var n int
	_ = h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_events WHERE kind = $1 AND ref_id = $2`, kind, refID).Scan(&n)
	return n
}

// commitAs makes a commit by Sam Phillips in dir, the way a person would.
func commitAs(t *testing.T, dir, file, subject string) string {
	t.Helper()
	writeIn(t, dir, file, subject+"\n")
	for _, args := range [][]string{
		{"add", "-A"},
		{"-c", "user.name=Sam Phillips", "-c", "user.email=sam@example.com", "commit", "-q", "-m", subject},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return gitOut(t, dir, "rev-parse", "HEAD")
}

func (h *harness) worktreeDir(featureID uuid.UUID) string {
	h.t.Helper()
	wt, err := store.LiveWorktreeForFeature(context.Background(), h.srv.Store.Pool, featureID)
	if err != nil {
		h.t.Fatal(err)
	}
	return h.srv.worktreeAbs(wt.Path)
}

// ---- The claim sweep ----

// TestClaimSweepRaisesTheStaleQuestionOnTheTask is FR-5.1 and FR-5.3.
func TestClaimSweepRaisesTheStaleQuestionOnTheTask(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()

	// A fresh claim isn't asked about, and its unchanged working copy is no
	// activity.
	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-stale", "task", t1.ID) != nil {
		t.Fatal("a fresh claim isn't stale")
	}
	if c := h.latestClaim(t1.ID); c.LastActivity != "claimed" {
		t.Fatalf("last activity = %s", c.LastActivity)
	}

	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "task", t1.ID)
	if cp == nil {
		t.Fatal("a claim idle for 26 hours is asked about, on the task")
	}
	want := t1.PublicID + ", Greeting helper, was claimed by the chat agent 1 day ago, and nothing has changed in its working copy for 26 hours. This feature's agents are waiting. Is someone still working on it?"
	if cp.Question != want {
		t.Fatalf("question = %q\nwant       %q", cp.Question, want)
	}
	for _, w := range []string{res.Claim.ID.String(), `"last_activity": "claimed"`, `"working_copy"`, `"actor"`} {
		if !strings.Contains(string(cp.Context), w) {
			t.Errorf("context lacks %s: %s", w, cp.Context)
		}
	}
	// One at a time while pending.
	h.srv.ClaimSweep(ctx)
	var n int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM checkpoints WHERE kind = 'claim-stale'`).Scan(&n)
	if n != 1 {
		t.Fatalf("claim-stale checkpoints = %d", n)
	}
}

// TestEditingTheWorkingCopyIsActivityAndWithdrawsTheQuestion is FR-5.1 step 1
// and FR-5.4.
func TestEditingTheWorkingCopyIsActivityAndWithdrawsTheQuestion(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "task", t1.ID)
	if cp == nil {
		t.Fatal("no question raised")
	}

	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	h.srv.ClaimSweep(ctx)

	if st := h.checkpointState(cp.ID); st != "withdrawn" {
		t.Fatalf("the question is %s, not withdrawn", st)
	}
	c := h.latestClaim(t1.ID)
	if c.LastActivity != "worktree" || time.Since(c.LastActivityAt) > time.Minute || c.WorktreeSeen == res.Claim.WorktreeSeen {
		t.Fatalf("claim = %+v", c)
	}
	if h.auditRefCount("claim.activity", t1.ID) != 1 {
		t.Error("the sweep audits the activity it sees")
	}
	if h.auditRefCount("checkpoint.withdrawn", t1.ID) != 1 {
		t.Error("the withdrawal is audited")
	}
	// Nothing more to see, nothing more audited.
	h.srv.ClaimSweep(ctx)
	if h.auditRefCount("claim.activity", t1.ID) != 1 || h.pendingOf("claim-stale", "task", t1.ID) != nil {
		t.Error("an unchanged working copy is no activity")
	}
}

// TestRenewingAClaimWithdrawsTheQuestion is FR-5.4 for the claim machine's own
// events.
func TestRenewingAClaimWithdrawsTheQuestion(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "task", t1.ID)
	if cp == nil {
		t.Fatal("no question raised")
	}
	if _, err := h.srv.ClaimWork(ctx, t1.PublicID, h.srv.ChatClaimant()); err != nil {
		t.Fatal(err)
	}
	if st := h.checkpointState(cp.ID); st != "withdrawn" {
		t.Fatalf("the question is %s", st)
	}
}

// TestKeepTheClaimRestartsTheClock is FR-5.3 and FR-5.10.
func TestKeepTheClaimRestartsTheClock(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "task", t1.ID)

	// The Inbox shows the question, the last activity and the three words.
	resp, page := h.getUI("/ui/frag/inbox")
	if resp != 200 {
		t.Fatalf("inbox: %d", resp)
	}
	for _, w := range []string{"Keep the claim", "Release it to an agent", "Last activity: claimed, 1 day ago."} {
		mustContain(t, "inbox", page, w)
	}

	if code, out := h.respond(cp, "keep"); code != 200 {
		t.Fatalf("respond: %d %s", code, out)
	}
	h.eventually("activity kept", func() bool { return h.latestClaim(t1.ID).LastActivity == "kept" })
	c := h.latestClaim(t1.ID)
	if c.State != lifecycle.ClaimOpen || time.Since(c.LastActivityAt) > time.Minute {
		t.Fatalf("claim = %+v", c)
	}
	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-stale", "task", t1.ID) != nil {
		t.Fatal("the clock restarted, so there is nothing to ask")
	}
	if h.auditRefCount("claim.activity", t1.ID) != 1 {
		t.Error("the audit trail says the claim was kept")
	}
}

// TestReleaseItToAnAgentEndsTheClaim is FR-5.3: released by the person who
// answered, with an implementer queued.
func TestReleaseItToAnAgentEndsTheClaim(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "task", t1.ID)

	if code, out := h.respond(cp, "release"); code != 200 {
		t.Fatalf("respond: %d %s", code, out)
	}
	h.eventually("claim released", func() bool { return h.latestClaim(t1.ID).State == lifecycle.ClaimEnded })
	c := h.latestClaim(t1.ID)
	if c.EndReason != "released" || c.EndedBy != h.srv.uiActor() {
		t.Fatalf("claim = %+v", c)
	}
	queued := 0
	for _, d := range h.dispatchesOf(t1.ID, "implement-task") {
		if d.State == "queued" {
			queued++
		}
	}
	if queued != 1 {
		t.Fatalf("an implementer is queued: %d", queued)
	}
	if st := h.checkpointState(cp.ID); st != "answered" {
		t.Fatalf("checkpoint = %s", st)
	}
}

// TestALateAnswerIsANoOp is FR-5.4: the question was withdrawn because the
// claimant came back, and the answer changes nothing.
func TestALateAnswerIsANoOp(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	h.staleClaim(res.Claim)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "task", t1.ID)
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	h.srv.ClaimSweep(ctx) // activity: the question is withdrawn

	code, body := h.respond(cp, "release")
	if code != 200 {
		t.Fatalf("a late answer isn't an error: %d %s", code, body)
	}
	mustContain(t, "late answer", body, claimMovedOnNotice)
	c := h.latestClaim(t1.ID)
	if c.State != lifecycle.ClaimOpen || c.LastActivity != "worktree" {
		t.Fatalf("nothing changed: %+v", c)
	}
	if st := h.checkpointState(cp.ID); st != "withdrawn" {
		t.Fatalf("checkpoint = %s", st)
	}
}

// TestAnAnswerAfterTheClaimWasSubmittedChangesNothing is FR-5.4's other half:
// the checkpoint is still pending (a deadline question isn't withdrawn by a
// submit), but the claim has moved on.
func TestAnAnswerAfterTheClaimWasSubmittedChangesNothing(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	h.exec(`UPDATE work_claims SET deadline_at = now() - interval '1 hour' WHERE id = $1`, res.Claim.ID)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-deadline", "task", t1.ID)
	if cp == nil {
		t.Fatal("no deadline question")
	}
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, h.srv.ChatClaimant(), "Added greet."); err != nil {
		t.Fatal(err)
	}
	if code, out := h.respond(cp, "release"); code != 200 {
		t.Fatalf("respond: %d %s", code, out)
	}
	time.Sleep(200 * time.Millisecond)
	if c := h.latestClaim(t1.ID); c.State != lifecycle.ClaimSubmitted {
		t.Fatalf("the submitted claim is untouched: %+v", c)
	}
}

// TestAReturnedClaimExpiresToo is SD-9, with the returned wording.
func TestAReturnedClaimExpiresToo(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	h.exec(`UPDATE work_claims SET state = 'returned', last_activity = 'sent_back',
		last_activity_at = now() - interval '26 hours' WHERE id = $1`, res.Claim.ID)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-stale", "task", t1.ID)
	if cp == nil {
		t.Fatal("a returned claim holds the task, so it expires")
	}
	want := t1.PublicID + ", Greeting helper, was sent back by its code reviewer 1 day ago, and nobody has resumed it. The task is waiting. Is someone still working on it?"
	if cp.Question != want {
		t.Fatalf("question = %q", cp.Question)
	}
	// Keeping a returned claim restarts its clock too.
	if code, out := h.respond(cp, "keep"); code != 200 {
		t.Fatalf("respond: %d %s", code, out)
	}
	h.eventually("kept", func() bool { return h.latestClaim(t1.ID).LastActivity == "kept" })
	if c := h.latestClaim(t1.ID); c.State != lifecycle.ClaimReturned {
		t.Fatalf("still returned: %+v", c)
	}
}

// TestTheDeadlineQuestion is FR-5.1 step 3.
func TestTheDeadlineQuestion(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-deadline", "task", t1.ID) != nil {
		t.Fatal("no deadline, no question")
	}
	h.exec(`UPDATE work_claims SET deadline_at = now() + interval '1 hour' WHERE id = $1`, res.Claim.ID)
	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-deadline", "task", t1.ID) != nil {
		t.Fatal("the deadline hasn't passed")
	}
	h.exec(`UPDATE work_claims SET deadline_at = now() - interval '1 hour' WHERE id = $1`, res.Claim.ID)
	h.srv.ClaimSweep(ctx)
	cp := h.pendingOf("claim-deadline", "task", t1.ID)
	if cp == nil {
		t.Fatal("the deadline has passed")
	}
	if want := t1.PublicID + ", Greeting helper, claimed by the chat agent, has passed its deadline. This feature's agents are waiting. Should it go on?"; cp.Question != want {
		t.Fatalf("question = %q", cp.Question)
	}
	// Going on ends the time box, so the question isn't asked again.
	if code, out := h.respond(cp, "keep"); code != 200 {
		t.Fatalf("respond: %d %s", code, out)
	}
	h.eventually("deadline cleared", func() bool { return h.latestClaim(t1.ID).DeadlineAt == nil })
	h.srv.ClaimSweep(ctx)
	if h.pendingOf("claim-deadline", "task", t1.ID) != nil {
		t.Fatal("asked again")
	}
}

// ---- The branch watch ----

// watchedFeature starts the feature and returns it with its working copy.
func (h *harness) watchedFeature() (*store.Feature, string) {
	h.t.Helper()
	h.claimsFeature(devPlanOneTask, 1)
	h.startHeld(1)
	f, err := h.srv.featureByPath(context.Background(), "auth/login")
	if err != nil {
		h.t.Fatal(err)
	}
	return f, h.worktreeDir(f.ID)
}

// TestAnUnclaimedCommitRaisesTheNotice is FR-5.6 to FR-5.8 and SD-11.
func TestAnUnclaimedCommitRaisesTheNotice(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()

	wt, _ := store.LiveWorktreeForFeature(ctx, h.srv.Store.Pool, f.ID)
	if wt.WatchedHead == "" || wt.WatchedHead != headOf(dir) {
		t.Fatalf("the watch starts at the branch head: %q", wt.WatchedHead)
	}
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Fatal("nothing was committed")
	}

	first := commitAs(t, dir, "quick.txt", "quick fix")
	h.srv.BranchWatchSweep(ctx)
	cp := h.pendingOf("unclaimed-commit", "feature", f.ID)
	if cp == nil {
		t.Fatal("a commit nobody claimed raises the notice, on the feature")
	}
	want := "One commit was made on " + f.PublicID + "'s branch while nobody had claimed a task: " +
		first[:7] + " by Sam Phillips, \"quick fix\". Work done outside a claim may have missed its code review, " +
		"and the verifier checks the feature against its spec, not each line. Next time, claim the task first, or ask the chat agent to."
	if cp.Question != want {
		t.Fatalf("question = %q\nwant       %q", cp.Question, want)
	}
	if !strings.Contains(string(cp.Context), first) {
		t.Errorf("context lacks the commit: %s", cp.Context)
	}

	// A second commit is added to the pending notice.
	second := commitAs(t, dir, "tidy.txt", "tidy")
	h.srv.BranchWatchSweep(ctx)
	again := h.pendingOf("unclaimed-commit", "feature", f.ID)
	if again == nil || again.ID != cp.ID {
		t.Fatalf("the pending notice is extended, not repeated: %+v", again)
	}
	for _, w := range []string{"Two commits were made on " + f.PublicID + "'s branch", first[:7] + " by Sam Phillips, \"quick fix\"; " + second[:7] + " by Sam Phillips, \"tidy\"."} {
		if !strings.Contains(again.Question, w) {
			t.Errorf("question lacks %q: %s", w, again.Question)
		}
	}
	// A sweep with nothing new changes nothing.
	h.srv.BranchWatchSweep(ctx)
	if p := h.pendingOf("unclaimed-commit", "feature", f.ID); p == nil || p.Question != again.Question {
		t.Fatal("a quiet sweep changed the notice")
	}

	// Its one answer.
	code, page := h.getUI("/ui/frag/inbox")
	if code != 200 {
		t.Fatalf("inbox: %d", code)
	}
	mustContain(t, "inbox", page, "I&#39;ve seen this")
	if code, out := h.respond(again, "seen"); code != 200 {
		t.Fatalf("respond: %d %s", code, out)
	}
	if st := h.checkpointState(again.ID); st != "answered" {
		t.Fatalf("checkpoint = %s", st)
	}
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Fatal("the commits were seen once")
	}
}

// TestACommitDuringAnOpenClaimIsNotFlagged is SD-10: it is the claim's work.
func TestACommitDuringAnOpenClaimIsNotFlagged(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	commitAs(t, res.WorkingCopy.Path, "mine.txt", "claimed work")
	h.srv.BranchWatchSweep(ctx)
	f, _ := h.srv.featureByPath(ctx, "auth/login")
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Fatal("a commit under an open claim isn't flagged")
	}
	wt, _ := store.LiveWorktreeForFeature(ctx, h.srv.Store.Pool, f.ID)
	if wt.WatchedHead != headOf(res.WorkingCopy.Path) {
		t.Fatal("the watch moved on")
	}
	// And it is activity on the claim.
	h.srv.ClaimSweep(ctx)
	if c := h.latestClaim(t1.ID); c.LastActivity != "worktree" {
		t.Fatalf("last activity = %s", c.LastActivity)
	}
}

// TestSubutaisOwnCommitsAreNotFlagged is FR-5.7: a submit's commit and a
// release's commit are known by hash.
func TestSubutaisOwnCommitsAreNotFlagged(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	f, _ := h.srv.featureByPath(ctx, "auth/login")
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	if _, err := h.srv.SubmitWork(ctx, t1.PublicID, h.srv.ChatClaimant(), "Added greet."); err != nil {
		t.Fatal(err)
	}
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Fatal("the submit's commit is Subutai's")
	}
}

// TestAReleasesCommitIsNotFlagged is the release half of FR-5.7.
func TestAReleasesCommitIsNotFlagged(t *testing.T) {
	h := newHarness(t)
	t1, res := h.claimedSetup()
	ctx := context.Background()
	f, _ := h.srv.featureByPath(ctx, "auth/login")
	writeIn(t, res.WorkingCopy.Path, "greet.go", "package main\n")
	if err := h.srv.ReleaseClaim(ctx, t1.PublicID, "sam"); err != nil {
		t.Fatal(err)
	}
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Fatal("the release's commit is Subutai's")
	}
}

// TestAPersonsCommitBeforeTheImplementersCompletionIsJudged is FR-5.7: the
// watch runs before Subutai commits, so the person's commit is judged and the
// completion's own commit is not.
func TestAPersonsCommitBeforeTheImplementersCompletionIsJudged(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()
	t1 := h.taskByLocal("T1")
	person := commitAs(t, dir, "sam.txt", "sam's change")
	writeIn(t, dir, "greet.go", "package main\n")
	d := h.dispatchesOf(t1.ID, "implement-task")[0]
	if err := h.srv.completeImplementation(ctx, t1.ID, d.ID, "Added greet."); err != nil {
		t.Fatal(err)
	}
	completion := headOf(dir)
	if completion == person {
		t.Fatal("the completion committed")
	}
	h.srv.BranchWatchSweep(ctx)
	cp := h.pendingOf("unclaimed-commit", "feature", f.ID)
	if cp == nil || !strings.Contains(cp.Question, person[:7]) || strings.Contains(cp.Question, completion[:7]) {
		t.Fatalf("only the person's commit is judged: %+v", cp)
	}
}

// TestACommitOnMainMergedIntoTheBranchIsNotFlagged is SD-10.
func TestACommitOnMainMergedIntoTheBranchIsNotFlagged(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()

	// Fast-forwarded onto a commit made on main.
	commitAs(t, h.root, "elsewhere.txt", "work on main")
	gitOut(t, dir, "merge", "--ff-only", h.srv.mainRef())
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Fatal("main's commit isn't the branch's")
	}

	// A branch with a commit of its own, seen, then main merged into it.
	commitAs(t, dir, "own.txt", "own work")
	h.srv.BranchWatchSweep(ctx)
	cp := h.pendingOf("unclaimed-commit", "feature", f.ID)
	if cp == nil {
		t.Fatal("own work is flagged")
	}
	h.respond(cp, "seen")
	commitAs(t, h.root, "more.txt", "more work on main")
	gitOut(t, dir, "-c", "user.name=Sam Phillips", "-c", "user.email=sam@example.com", "merge", "--no-edit", h.srv.mainRef())
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Fatal("neither main's commit nor the merge that brought it in is flagged")
	}
}

// TestARewrittenBranchRaisesTheNotice is FR-5.6 step 2.
func TestARewrittenBranchRaisesTheNotice(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()
	commitAs(t, dir, "quick.txt", "quick fix")
	h.srv.BranchWatchSweep(ctx)
	cp := h.pendingOf("unclaimed-commit", "feature", f.ID)
	if cp == nil {
		t.Fatal("no notice for the commit")
	}
	h.respond(cp, "seen")

	old := headOf(dir)
	gitOut(t, dir, "reset", "--hard", "HEAD~1")
	h.srv.BranchWatchSweep(ctx)
	cp = h.pendingOf("unclaimed-commit", "feature", f.ID)
	if cp == nil {
		t.Fatal("a branch that lost a commit the watch had seen is worth a notice")
	}
	if !strings.HasPrefix(cp.Question, f.PublicID+"'s branch was rewritten: commits Subutai had already seen are no longer on it.") ||
		!strings.Contains(cp.Question, old[:7]) {
		t.Fatalf("question = %q", cp.Question)
	}
}

// TestAFeatureInReviewIsWatched is SD-10: active and review are watched.
func TestAFeatureInReviewIsWatched(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()
	h.exec(`UPDATE features SET state = 'review' WHERE id = $1`, f.ID)
	commitAs(t, dir, "late.txt", "late fix")
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) == nil {
		t.Fatal("a feature being verified is watched")
	}
}

// TestThePostCommitHookTriggersTheWatch is FR-5.6: the watch runs after every
// post-commit call, without holding the response up.
func TestThePostCommitHookTriggersTheWatch(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	commitAs(t, dir, "hook.txt", "committed with the hook")
	if code, out := h.call("POST", "/api/hook/post-commit", map[string]any{}); code != 200 {
		t.Fatalf("hook: %d %v", code, out)
	}
	h.eventually("the notice", func() bool { return h.pendingOf("unclaimed-commit", "feature", f.ID) != nil })
}

// TestBootSetsWhereTheWatchStarts is FR-5.9: a worktree with no watched head
// starts at the branch head, and earlier history isn't reported.
func TestBootSetsWhereTheWatchStarts(t *testing.T) {
	h := newHarness(t)
	f, dir := h.watchedFeature()
	ctx := context.Background()
	commitAs(t, dir, "before.txt", "before the upgrade")
	h.exec(`UPDATE worktrees SET watched_head = NULL WHERE feature_id = $1`, f.ID)
	if err := h.srv.BackfillWatchedHeads(ctx); err != nil {
		t.Fatal(err)
	}
	wt, _ := store.LiveWorktreeForFeature(ctx, h.srv.Store.Pool, f.ID)
	if wt.WatchedHead != headOf(dir) {
		t.Fatalf("watched head = %q", wt.WatchedHead)
	}
	h.srv.BranchWatchSweep(ctx)
	if h.pendingOf("unclaimed-commit", "feature", f.ID) != nil {
		t.Fatal("history before M13 is never reported")
	}
}
