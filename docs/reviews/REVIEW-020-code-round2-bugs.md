# REVIEW-020 code, round 2: bugs and spec conformance

**Reviewer:** Claude, as the bug and spec-conformance reviewer for M13.
**Date:** 2026-10-02
**Diff:** `git diff 2a799f8..HEAD` on `claude/subutai-m13-executors`, at
`02ef933`. T7's tests (`integration_executors_e2e_test.go`,
`integration_executors_races_test.go`) were read as well.
**Binding documents:** SPEC-020 (with FR-1.6 amended), round 1
(`REVIEW-020-code-round1-bugs.md`, B1 to B13).

**What I ran**, against my own Postgres on port 54352:
- `go build ./...` and `go vet ./...`: clean. `gofmt -l internal cmd`: empty.
- `go test -race -count=1 ./...`: every package passed. `internal/server`
  took 224s and `internal/store` took 16s.

## Summary

All thirteen round-1 findings are addressed:
- Eleven are fixed outright, with a test for each.
- B3 is fixed for the race it named, but a narrower window is still open
  (B15).
- B7's new hold is sound while a completion is on its way. It doesn't cover a
  completion that never happens (B14).

The new code takes locks in the same order as before: the feature, then the
task, then the dispatch. The working-copy mutex is never taken twice. No git
runs inside a transaction. No new path crosses the judgement boundary.

I found no new major defect. The new findings, B14 to B19, are all minor.

## Round-1 findings

| # | Sev. | Status | Evidence |
|---|---|---|---|
| B1 | major | **Fixed** | T7 is merged (`2a799f8`). `TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents` follows FR-10.1's seven steps and ends with FR-6.4's audit check (`assertNoClaimedTaskApprovedWithoutReview`). Each race runs 20 times with a staggered lag: `TestClaimRacingTheDispatcherHasOneWinner`, `TestClaimRacingDispatchReadyTasks`, `TestClaimOnOneTaskWhileASiblingStarts`, `TestSendBackWhileAnAgentRunsOnAnotherTask`, `TestAbandonedFeatureWithAnOpenClaimRaisesNothing` (which runs `ClaimSweep`) and `TestTransitionTaskStateGuard`. All green under `-race`. B19 notes two weak assertions. |
| B2 | major | **Fixed** | `ui_claims.go:182` decides by `store.CurrentTaskRound(...) > 1`, not by the latest execution's round. `executors.html` renders `claim-comments` for the chat agent's returned claim. `TestReturnedClaimShowsTheReviewersCommentsOnTheTaskPage` and `TestPersonsReturnedClaimShowsTheReviewersComments` check the panel before any resume. |
| B3 | minor | **Partly fixed** | `CancelIfTaskClaimed` now returns `true` whenever the task has a claim, whatever the dispatch's state (`dispatches.go:704-710`, `TestCancelIfTaskClaimedReportsAClaimThatGotThereFirst`). The second window round 1 named is still open: a claim that lands between this check and the rule's checkpoint insert. See B15. |
| B4 | minor | **Fixed** | See "B4 in detail" below. |
| B5 | minor | **Fixed** | `worktree_ops.go:201-205` calls `startWatching` on reconcile only when `wt.WatchedHead == ""`. `TestReconcileKeepsTheWatchedHead` makes a commit, deletes the directory, reconciles, and then the commit is flagged. |
| B6 | minor | **Fixed** | `claimLocked` runs `watchBranchLogged` first (`claims.go:368-370`). It doesn't deadlock: `watchBranchLogged` calls `watchBranchLocked` directly (`branch_watch.go:61-65`), which never touches `copyLocks`. Only `WatchBranch` and `BackfillWatchedHeads` take the mutex, and neither is reachable from inside a `withWorkingCopy` closure. The watch's own transaction finishes before the claim's begins, so the two never nest. `TestAClaimJudgesACommitMadeJustBefore`. |
| B7 | minor | **Fixed for the normal path** | See "B7 in detail" below. The window lets go of a completion that failed for good. See B14. |
| B8 | minor | **Fixed** | See "B8 in detail" below. |
| B9 | minor | **Fixed for the three claim routes** | `claimTaskFromForm` returns 403 when `!sameOrigin(r)` (`ui_claims.go:238`), and all three handlers go through it. `TestUIClaimRoutesRedirectAndRefuseCrossSitePosts` sends both `Sec-Fetch-Site: cross-site` and a foreign `Origin`. `/ui/respond` can still release a claim from another site. See B16. |
| B10 | minor | **Fixed** | A successful act now returns a 303 to `/ui/t/{id}?done=…`, and `taskDoneNotice` builds the sentence (`ui_claims.go:269-305`, `ui_entity_pages.go:247-254`). A refusal still renders the page with the error banner, which FR-4.2 allows. The test refreshes the page twice and gets no error banner. B17 is a small side effect. |
| B11 | minor | **Fixed** | `codeReviewModel` returns `chatReviewer`, which is true only when the chat rule picked a model other than the usual one (`claims_task.go:446-452`). The submit records `model` and `chat_reviewer` in `claim.submitted` (`claims.go:620`). `auditClaim` adds `claim_id` (`store/claims.go:94`). `chatReviewedWith` reads that payload for the latest execution's claim (`ui_claims.go:110-135`). The tests: the sentence still shows after a later `chat_review_model: same`, and isn't shown when `same` was set at submit. |
| B12 | minor | **Resolved by the spec** | FR-1.6 is amended: only the feature page lists tasks, because no plan page lists any. I confirmed this: only `executors.html` iterates `.Tasks`. NFR-3 still says "the feature and plan task lists". See B18. |
| B13 | minor | **Fixed** | `claimActivityWords` (`claims.go:285-303`) gives the activity in words for the panel, the Inbox and `get_feature` (`last_activity_what`). The question about a returned claim now gives idle hours, not the time since the send-back (`claims_task.go:330`). FR-5.3's example sentence wasn't updated to match. See B18. |

### B4 in detail: the review key's round

**Enqueue sites.** `CodeReviewIdempotencyKey` (`rules_phase2.go:232`) has
exactly two callers:
- `completeImplementation` (`actions_phase2.go:309`);
- `taskClaims.onSubmit` (`claims_task.go:271`).

Both read `CurrentTaskRound` in the transaction, after the `implemented`
transition. That transition doesn't change the count. No other code enqueues
`review-code`:
- a retry reuses the dispatch row (`RequeueDispatch`);
- no checkpoint answer re-enqueues a review.

**Parsers.** Nothing parses or rebuilds the key:
- The only key-prefix test is `actions.go:107`. It checks `"review:"`, which
  `"review-code:"` doesn't match, and only for documents.
- `rules/rules_phase2.go` uses the key only to deduplicate on insert.
- No SQL matches `idempotency_key` with `LIKE`.

**Why the round makes the key unique.** `CurrentTaskRound` counts the task's
`request_changes` transitions. In `lifecycle/task.go`, `request_changes` is
the only way from `review` back to `active`. So every submission (agent or
claim) is in a different round, and the key `(task, commit, round)` can't
repeat.

**The `d == nil` branch.** In `onSubmit` this branch now refuses, and the
transaction rolls back (`claims_task.go:275-280`). It can't be reached in
practice. If it ever were, its advice "try once more" would hit the same key
again.

**Test:** `TestResubmittingAReviewedCommitStillQueuesAReview` is round 1's
scenario exactly: reset to X in round 3, and a third review is queued with a
`run_id`.

### B7 in detail: is the 15-minute window sound?

**Where it is checked.** `ImplementPendingCompletion`
(`dispatches.go:417-434`) is checked in three places:
- the claim's refusal (`claims_task.go:130`), for both a new claim and a
  resume;
- `ImplementHold` (`dispatches.go:468`), so both `admit`'s pool filter and
  `StartImplementDispatch` see it under the feature's lock.

**Ordering.**
- In both the refusal and the hold, the "running" check runs before the
  "pending" check, in a separate later statement. A dispatch that moves from
  `running` to `succeeded` in between is therefore seen by one check or the
  other.
- The task leaves `active` only in the completion's own transaction, after
  the commit, so the hold lasts until the work is committed.

**Can it wrongly hold the feature?** Only briefly, and only after a
completion has already gone wrong:
- A completion that fails after its commit (the transaction errored) holds
  the feature for the rest of the 15 minutes, with nothing left to protect.
- So does a send-back whose `enqueueImplementTx` fails, because the old
  success is still the latest implement dispatch.

These cases are covered:
- a task that is claimed (`NOT EXISTS work_claims … <> 'ended'`), or whose
  completion stepped back because of that claim;
- a queued rework (only the latest dispatch counts);
- a task in `review`, `done` or `abandoned`.

**Can it wrongly release the feature?** Yes, when the completion never
commits. A succeeded dispatch can be left with its work uncommitted in three
ways:
- the completion hit a configuration error (now raised before the commit:
  B8);
- the server stopped between `MarkAttemptSucceeded` and the event being
  handled (the bus is in memory, and nothing re-publishes it);
- the event loop errored.

None of these is retried, and the `config-error` answer routes to nothing.
After 15 minutes the hold lapses silently, with the implementer's work still
uncommitted in the copy. B14 has the consequence.

**A slow completion.** A completion delayed past 15 minutes (for example, an
orchestrator backlog) has the same release. In that case
`completeImplementation` checks for a claim on its own task only
(`actions_phase2.go:253`), not on a sibling. Round 1's alternative fix (refuse
when another task in the feature has an open claim) wasn't taken.

**Verdict on the window:** sound for the window round 1 described (success,
then a completion milliseconds later). A completion that failed for good was
handed straight to a sibling before M13, and is now handed over 15 minutes
later.

### B8 in detail: the ordering in `completeImplementation`

Under the working-copy mutex, in this order (`actions_phase2.go:250-326`):
1. Check for a claim on the task. If there is one, return with nothing
   committed.
2. `freshConfig`, the `review-code` role, and `codeReviewModel`. Every config
   error is settled here, before any git.
3. `watchBranchLogged` (FR-5.7).
4. `commitWorktree`, then `headOf`.
5. One transaction:
   - `LockFeatureAndTask` (the feature, then the task);
   - `RecordWorktreeCommit`;
   - `TransitionTask(implemented)`;
   - `MarkDispatchExecutionSubmitted`;
   - `CurrentTaskRound`;
   - `EnqueueDispatch`.
6. If step 5 fails, record the commit in its own transaction, so the watch
   never calls Subutai's commit unclaimed.

**What holds:**
- The commit's hash is recorded on both the success path and the failure
  path.
- A configuration error makes no commit, so the watch has nothing to flag.
- No git runs inside a transaction.
- The lock order is kept.

**Test:** `TestACompletionWithAConfigErrorMakesNoCommit`. The cost is B14:
the work stays uncommitted, and the task stays stuck.

## New findings

### B14: minor. A completion that never commits stops holding the copy after 15 minutes, and a sibling then takes its work

**Location:**
- `store/dispatches.go:404-434`: the window;
- `server/actions_phase2.go:250-326`: the completion;
- the `config-error` answer, which routes to nothing in
  `rules.decideCheckpointResponded`.

**Problem.** Nothing retries a completion, so a failed one is permanent. Once
`PendingCompletionWindow` passes:
- `ImplementHold` and the claim refusal no longer see the task;
- the implementer's uncommitted work is still in the working copy;
- the task is still `active`, with a succeeded dispatch.

**Scenario:**
1. T1's implementer succeeds.
2. T1's completion hits a configuration error. After the B8 fix, the
   checkpoint is raised and nothing is committed.
3. Fifteen minutes later, T2's queued implementer starts in the same copy.
4. T2's completion runs `git add -A`, and commits T1's work as T2's, under
   T2's review.

A person who claims T2 instead gets the same effect: their submit commits
T1's work as theirs. T1 itself is stranded in `active`. Answering the
`config-error` question changes nothing.

The same happens after a crash between `MarkAttemptSucceeded` and the event
being handled.

Before M13 this hand-over happened at once, so the change isn't a
regression. But the window is what decides the outcome, and nobody is told
when it lapses.

**Fix:** any of these three:
- Keep holding the copy past the window when it is dirty and its last
  implementer's task is still `active`, and raise a checkpoint ("T1's
  finished work was never committed") instead of letting go silently.
- Route the `config-error` answer on a task to run the completion again.
- Have `completeImplementation` refuse, and raise a checkpoint, when another
  task in the feature has a claim that hasn't ended (round 1's alternative for
  B7). This also covers the slow-completion case.

### B15: minor. The `dispatch-failure` question can still be raised about a task that was just claimed (B3's residue)

**Location:** the exhausted loop in `dispatch.go` `RetrySweep`, and
`rules.go:395` (`DispatchExhausted` leads to `RaiseCheckpoint`).

**Problem.** `CancelIfTaskClaimed` commits `false`, and the sweep then
publishes `DispatchExhausted`. A claim that commits after that check but
before the rule's insert does two things:
- it cancels the failed dispatch;
- it withdraws a checkpoint that doesn't exist yet.

The rule then raises "Dispatch … failed all attempts. Retry or cancel?" about
a claimed task.

**Effects of each answer:**
- **Retry** is a harmless no-op, with an audit row (`RequeueUnlessClaimed`).
- **Cancel** reaches `MarkDispatchCancelled`. That returns "dispatch not
  failed", which the event loop only logs.

**Fix:** have the executor of `RaiseCheckpoint{CPKind: "dispatch-failure"}`
re-check, inside its transaction, that the dispatch is still `failed` and its
task unclaimed. Alternatively, make Cancel on an already-cancelled dispatch a
no-op.

### B16: minor. `/ui/respond` accepts cross-site posts, and the `claim-stale` answer releases a claim

**Location:** `handleUIRespond` (no `refuseCrossSite`), and
`ReleaseClaimAnswered` (`claim_sweep.go:201`).

**Problem.** B9 protected the three claim routes. But answering a pending
`claim-stale` or `claim-deadline` question with "Release it to an agent" does
the same thing as `/ui/task/release`:
- it commits the working copy;
- it queues a paid implementer.

That answer is new in M13. A page on another site can post it, given the
checkpoint's ID.

**Fix:** call `s.refuseCrossSite(w, r)` at the top of `handleUIRespond`. That
also covers the Inbox's older answers, such as Retry and Approve.

### B17: minor. The task page's `?done` notice prints the `model` query parameter

**Location:** `ui_claims.go` `taskDoneNotice` (`q.Get("model")`) and
`ui_entity_pages.go:247-254`.

**Problem:** for example,
`/ui/t/<id>?done=submitted&model=…anything…` renders "You submitted X for
code review. The review runs on …anything…" on Subutai's own page.
`html/template` escapes the value, so this isn't XSS. It does let a link put
any sentence on a trusted page. A bookmarked or shared link also repeats the
notice long after the fact.

**Fix:** name the model from the task's latest `review-code` dispatch, or
from the `claim.submitted` row, and not from the URL. Accept only the known
`done` values, as the code already does.

### B18: minor. The spec text lags the code in two places (NFR-3, FR-5.3)

**Problem:**
- **NFR-3** still lists "the feature and plan task lists" among the shared
  files. The amended FR-1.6 says the plan page lists no tasks.
- **FR-5.3**'s example for a returned claim, "… was sent back by its code
  reviewer 26 hours ago, and nobody has resumed it", no longer matches the
  code. The code says "… was sent back by its code reviewer, and nothing has
  happened to it for N hours", which is B13's fix.

**Fix:** amend both sentences.

### B19: minor. Two of T7's assertions are weaker than the spec's words

**Location:**
- `integration_executors_e2e_test.go`, steps 3 and 4;
- `assertNoClaimedTaskApprovedWithoutReview`.

**Problem:**
- **Step 3.** FR-10.1 says "T02's implementer runs while T01 is reviewed".
  The test only waits until both have happened. It doesn't assert that T02's
  implement dispatch started while T01 was in `review`. One of these would
  pin it:
  - compare T02's `started_at` with T01's review dispatch's `finished_at`;
  - or hold T01's review and see T02 start.
- **FR-6.4's check** accepts any answered checkpoint on the task as the
  person's decision. The spec names only the round cap and an escalation. An
  answered `claim-stale` would satisfy it. The check should restrict
  `c.kind` to `review-deadlock` and `review-escalation`.

## Conformance, updated rows

These are the rows round 1 marked "Partly" or "Not met". Rows not listed are
unchanged from round 1.

| Req | Status | Evidence |
|---|---|---|
| FR-10.1 | **Met** | `TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents` covers all seven steps: the queue reason, both review models asserted on the dispatch rows, the merge, the executor lines, unmeasured actuals, and the corpus including the measured feature but not this one. The concurrency in step 3 is only implied (B19). |
| FR-1.6 | **Met** (as amended) | The feature page's "Tasks in this feature" list (`executors.html`, `featureTaskRows`). |
| FR-2.5 | **Met** | The review key is unique per round, and an impossible duplicate is refused with a rollback (B4). |
| FR-3.4 | **Met** | `review.run_id` is always a queued dispatch. |
| FR-4.1 | **Met** | A returned claim shows the comments, for the chat agent and for a person (B2). |
| FR-4.2 | **Met** | 303 to the task page with a notice; a refusal shows as a banner; cross-site posts are refused (B9, B10). B17 is a nit. |
| FR-5.6 | **Met** | The reconcile keeps `watched_head` (B5), and a claim runs the watch first (B6). |
| FR-5.7 | **Met** | Completion, submit, release and claim all run the watch first. The completion records its hash whatever the transaction does, and makes no commit on a configuration error (B8). |
| FR-5.9 | **Met** | The reconcile starts the watch only where there is no head (B5). |
| FR-6.5 | **Met, one narrow residue** | Every listed case is tested: the refusals, submitting twice or in review, the exhausted dispatch, the send-back while an agent runs, the claim racing both starts, and the sweep after abandonment. B15 is a narrow window left in the exhausted-dispatch case. |
| FR-8.4 | **Met** | The sentence comes from what the submit recorded (B11). |
| NFR-4 | **Met** | No raw activity word is printed (B13). The Inbox labels the claim checkpoints' context "Details". |
| NFR-5 | **Met** | Vet is clean, gofmt is empty, and `go test -race -count=1 ./...` is green with the integration tests, against Postgres on port 54352. |
| SD-6 | **Met** | The comments are on the task page, in the claim result and in `get_feature` while the claim is returned. |
| SD-10 | **Met, with B14's caveat** | B5, B6 and B8 are closed. Subutai's own commits are always recorded. B14's orphaned work is uncommitted, not an unclaimed commit, but it can end up in another task's commit. |

Related rows: FR-2.7 and FR-2.8 are now **Met and tested**: T7's races, and
`TestAClaimWaitsForAnImplementersCompletion`. B14 is the remaining edge. SD-3
and SD-4 now include the pending completion in the hold.

## The new diff: other checks

- **Lock order.** No new path breaks the order of feature, then task, then
  dispatch:
  - the completion: `LockFeatureAndTask`, then inserts;
  - `CancelIfTaskClaimed`: unchanged;
  - `ImplementHold` and `ImplementPendingCompletion`: reads only.
- **The working-copy mutex.** No closure that holds it calls `WatchBranch`,
  `BackfillWatchedHeads`, `ClaimWork`, `SubmitWork` or `ReleaseClaim`.
  `claimLocked`, `submitLocked`, `releaseLocked` and `completeImplementation`
  call only `watchBranchLogged`.
- **Git and transactions.**
  - `claimLocked`'s watch runs before its transaction.
  - The completion's claim check is a pool read under the mutex. Claims take
    the same mutex, so it can't be overtaken.
  - The fallback transaction runs no git.
- **The dispatcher's `admit`.** It now gives back the worker slot from a
  single `defer`, keyed on `started`. Every early return, including
  `StartLost`, gives it back, and the started goroutine still releases its
  own.
- **The judgement boundary.** Nothing changed: no new MCP tool, and no new
  route that approves, reviews, verifies or answers.
  - The new `onSubmit` refusal makes a submit fail rather than skip review.
  - The `?done` notice is display only.

## Verdict

**No major finding is open. The branch can merge.**
- Round 1's two majors (B1, B2) are fixed and tested.
- So are B4 to B13, except for the edges noted.

Record B14 to B19 in the M13 handoff as known limitations. The cheapest
worth doing now:
- B16: one line in `handleUIRespond`;
- B17: read the model from the database, not the URL;
- B18: the spec's wording.

B14 deserves an owner. It is a pre-existing hand-over of orphaned work that
M13's window now delays but doesn't report.
