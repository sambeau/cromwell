# VERIFICATION-020: Executors (M13)

**Verifier:** Claude (Opus), the M13 verifier. Independent of the author and
the reviewers. No code was changed.
**Date:** 2026-10-02
**Branch:** `claude/subutai-m13-executors`, at `6731417` ("Merge the round-2
fixes (B16, B17, Q19, Q20)"). The working tree was clean.
**Binding spec:** [SPEC-020](../specs/SPEC-020-executors.md).
**Context only, not evidence:** REVIEW-020 (spec review), and the round-1 and
round-2 code reviews (bugs and quality).

Every test named below was seen passing in my own run (§ Test run). A
`file:line` is in production code on this branch. A screenshot reference is to
a PNG in `docs/walkthrough-spec-020/` that I opened and looked at.

## The four things given special scrutiny

### 1. The roadmap's done-when: "a task done in chat gets the same review and verification as an agent's"

**Test:** `TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents`
(`internal/server/integration_executors_e2e_test.go:123`). It passed, in 2.30s.

I read all of it. It proves the claim, for these reasons:
- **Real path.** The chat agent's acts go through the MCP JSON-RPC endpoint
  (`h.toolOK("claim_task" …)`, `"submit_task"`, `"get_feature"`). They don't
  call the service directly.
- **Real dispatches.** The provider is a `roleRouter` that sends each request
  to the mock for its role, keyed by the outcome tool and, for reviews, by the
  model (`review:<model>`). So:
  - the chat agent's reviews *must* reach the provider as `claude-sonnet-5`;
  - the agent's T02 review *must* reach it as `claude-haiku-4-5`;
  - any request without a script fails the test (`rr.missed()`, line 244).
- **The reviews.** Line 225 asserts that T01 has exactly two `review-code`
  dispatches, both on the chat reviewer model. Line 228 asserts that T02 has
  one, on the usual model.
- **The verification.** Line 232 asserts one succeeded `verify-feature`
  dispatch. Line 224 asserts the feature reaches `done`. Lines 236 to 240
  assert both files are merged into the root checkout.
- **The send-back.** Lines 193 to 203 assert three things after the
  reviewer's `request_changes`:
  - the claim is `returned`;
  - the task is `active`;
  - every T01 implement dispatch is `cancelled`, so no implementer was queued.

  The implement script holds exactly one round of responses (T02's). So an
  implementer run for T01 would also have exhausted it.
- **Comments, executors and actuals.**
  - `get_feature` carries the comments (line 206), and the resumed claim
    returns them (line 211).
  - The task page, `get_feature` and the timeline name the chat agent for T01
    and the implementer for T02 (lines 250 to 269).
  - `store.Unmeasured` is asserted for T01, T02, both features and the
    initiative (lines 286 to 295).
  - The calibration corpus excludes the chat feature and includes the measured
    one (lines 296 to 307).
- **FR-6.4.** The test ends with `assertNoClaimedTaskApprovedWithoutReview`.
  For every `approve` transition of a task that was ever claimed, it requires
  one of two things before the approval:
  - a succeeded `review-code` dispatch;
  - or an answered checkpoint on the task.

**Gaps (why FR-10.1 is "partly met" below):**
- Step 3's "T02's implementer runs **while** T01 is reviewed" isn't asserted.
  The test waits until both have happened (line 193), and nothing orders
  T02's `started_at` against T01's review.
- The FR-6.4 helper accepts *any* answered checkpoint on the task. The spec
  names only the round cap and an escalation. In this run, T01's approval is
  backed by two succeeded reviews, so the check holds. As a general oracle,
  though, it is looser than its comment says.

REVIEW-020 round 2 (B19) noted both gaps. I confirmed them by reading the
code. Neither weakens the done-when itself: same review (dispatched, with a
stronger model), same verification (dispatched `verify-feature`), and the
merge.

### 2. The judgement boundary (FR-6)

**Test:** `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
(`internal/server/integration_mcp_test.go:77`). It passed.

- **The advertised set.** It compares `tools/list` with an exact sorted list,
  using `reflect.DeepEqual`. That list gains exactly `claim_task` and
  `submit_task`, with a comment citing DEC-007 decision 3 and DESIGN-010 §5c
  (lines 186 to 192). The header comment (lines 69 to 76) has the FR-6.1
  wording.
- **The forbidden names.** Each of FR-6.2's eleven names is called through
  `tools/call`, and each must fail with JSON-RPC `method-not-found` (lines 207
  to 222): `release_task` … `claim_feature`. So their absence is enforced by
  the dispatcher of tool calls, not just by the listing.
- **Verification can't be claimed (FR-6.3).** `TestClaimTaskToolRefusals`
  refuses three IDs with the exact sentence "Only tasks can be claimed.
  Verification is always done by Subutai's verifier.": a feature's ID, a
  feature's path, and `BUG-001`. `resolveClaimRef` (`claims.go:316`) accepts
  only `ident.ShapeTask`. Two tests show a feature whose every task was
  claimed still gets a dispatched verifier:
  - `TestChatSendBackAndVerificationOverMCP` (one task, claimed and submitted
    twice; `verify-feature` succeeded = 1);
  - the end-to-end test.
- **Checkpoints.** No tool answers a checkpoint. The Inbox answers go through
  `/ui/respond`, which refuses cross-site posts (round-2 fix B16, commit
  `95b94ac`).

### 3. Unmeasured entities are left out before `LIMIT` (FR-7.2)

**Production code:**
- `store/corpus.go`: `RetrieveCorpus` adds `AND NOT unmeasuredFeatureSQL(...)`
  and `AND NOT unmeasuredTaskSQL(...)` inside each arm of the `UNION ALL`,
  before `ORDER BY rank … LIMIT $2`.
- `store/ui_reads.go`: `RecentCalibration` puts the same predicates in its
  `WHERE`, before `LIMIT $1`.
- `store/unmeasured.go:18-46` defines the predicates once, with `EXISTS`
  sub-queries over:
  - `executions.measured`;
  - the latest `wrote`/`revised`/`added` writer on the live spec, or bug
    report, and the dev plan.

**Test:** `TestCorpusAndCalibrationFilterBeforeLimit`
(`internal/store/unmeasured_test.go:161`). It passed. It proves the ordering,
not just the exclusion:
- It seeds five measured tasks, and four unmeasured ones that match the query
  more strongly and are estimated later.
- It asserts both lists return five rows, all measured. With `LIMIT 5`, a
  filter applied after the limit would return only one or two rows, because
  the four stronger unmeasured rows rank first in the corpus and are newest in
  calibration. So `len(rows) != 5` would fail.
- It then marks a feature's spec as written in chat and asserts that
  calibration drops the feature.

### 4. Claim expiry (FR-5)

**Production code:**
- `claim_sweep.go:25` (`ClaimSweep`) is called on the heartbeat after
  `StallSweep` (`server.go:270`).
- For an open claim, it fingerprints the working copy *first*, under the
  copy's lock (`claim_sweep.go:60-72`), so work done while the server was down
  is seen before expiry is judged (SD-9).
- It then judges expiry for `open` and `returned` claims (`:77`).
- It raises the checkpoint only after re-reading the claim under
  `LockClaim` and re-checking that it is still due (`:145-174`).
- Every claim event, and *Keep the claim*, withdraws a pending `claim-stale`
  (`store/claims.go`, `TransitionClaim` and `KeepClaim`).

**Tests (all passed):**
- `TestClaimSweepRaisesTheStaleQuestionOnTheTask`:
  - a fresh claim isn't asked about;
  - a claim backdated 26 hours is asked about, with the exact question and its
    context;
  - a second sweep doesn't raise a second checkpoint.
- `TestEditingTheWorkingCopyIsActivityAndWithdrawsTheQuestion`: an edit gives
  activity `worktree`, withdraws the question, and writes both audits.
- `TestRenewingAClaimWithdrawsTheQuestion`.
- `TestKeepTheClaimRestartsTheClock`, through `/ui/respond` and the rules
  engine. It also checks the Inbox's answers and "Last activity".
- `TestReleaseItToAnAgentEndsTheClaim`: released by the person who answered,
  with an implementer queued.
- `TestALateAnswerIsANoOp` and `TestAnAnswerAfterTheClaimWasSubmittedChangesNothing`
  (FR-5.4).
- `TestAReturnedClaimExpiresToo` (SD-9, with the returned wording).
- `TestTheDeadlineQuestion` (FR-2.10's deadline).
- `TestAbandonedFeatureWithAnOpenClaimRaisesNothing` (SD-16).
- Screenshot `06-still-working.png` shows the real Inbox card in the demo:
  - the question for a claim backdated 25 hours;
  - "Last activity: claimed, 1 day ago.";
  - **Keep the claim** and **Release it to an agent**.

## Criteria

Verdicts: **met**, **partly met** (what is missing is said), **not met**, and
**pending** (Sam's approval).

### FR-1: The execution record

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-1.1 `executions` table and checks | **Partly met** | The table, its checks and its unique indexes match the spec (`migrations/0014_executors.sql:49-75`). `TestExecutions` and `TestClaimUniqueness` pass. **Missing:** "when a task is deleted … its rows are deleted with it, by the service". `store.DeleteTask` (`store/tasks.go:165`) deletes only the task. This is harmless today, because only pending and ready tasks are deleted and they never have rows. But the clause isn't built. |
| FR-1.2 Where executions are recorded | **Met** | Agent row at the start: `StartImplementDispatch` calls `RecordAgentExecution` (`store/dispatches.go:503`; `ON CONFLICT DO NOTHING` for retries, `store/executions.go:65`). Claim and resume rows: `claims.go:418,447`. `submitted_at`: `MarkClaimExecutionSubmitted` and `MarkDispatchExecutionSubmitted`. Tests: `TestClaimTakesAnActiveTaskFromTheQueue` (one chat row; a renewal adds none), `TestPlanImplementShowsTheReviewersCommentsInRoundTwo` (two agent rows, rounds 1 and 2), `TestExecutions`. |
| FR-1.3 Backfill in 0014 | **Met** | `0014_executors.sql:101-115`. `TestMigration0014BackfillsAgentExecutions` passed. |
| FR-1.4 `executorLine` | **Met** | `claims.go:884,904`. `TestExecutorSentence` passed. It covers all five example sentences, plus the "same executor again" case. |
| FR-1.5 Task page `task-executor` | **Met** | `executors.html:11-24`, `entity.html:806`. Tests: `TestTaskExecutorRenders`, `TestPersonClaimsATaskInTheUI`. Screenshot 02 shows "Implemented by the chat agent." and the unmeasured sentence. |
| FR-1.6 Feature page task list, icon, claim label | **Met** | The `feature-tasks` include (`executors.html:158-190`), with `aria-label` and `title` set to the line. Tests: `TestFeatureTasksRender`, and `TestPersonClaimsATaskInTheUI` (`aria-label="Being implemented by …"`, "Claimed by …"). Screenshots 03 and 09. |
| FR-1.7 Timeline moments | **Met** | `timeline/timeline.go`: the `claim.claimed`, `claim.submitted` and `claim.released` rules, and the checkpoint names. `TestClaimMoments` passed. The end-to-end test asserts the claim, submit and send-back moments. Screenshot 09 shows "Claimed by …", "Submitted by …", "Waiting for a person: is someone still working on a task" and "Waiting for a person: commits nobody claimed". |
| FR-1.8 `executor` in MCP results | **Met** | `mcp_claim_tools.go:159`. `TestClaimTaskToolResultAndRenewal` asserts the "Nobody has started" form. The end-to-end test asserts `kind`, `measured` and `sentence`. |

### FR-2: Claims

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-2.1 `work_claims`, indexes, `claim` ref type | **Met** | `0014_executors.sql:7,15-42`. `TestClaimUniqueness` and `TestClaimEndReasons` passed. |
| FR-2.2 Claim machine; task `release` event | **Met** | `lifecycle/claim.go:39-68`, `lifecycle/task.go:42`. `TestClaimTransitionMatrix` and `TestClaimTerminal` passed. |
| FR-2.3 Claiming | **Met** | `claims.go:336-462`: the fingerprint and head before the transaction; then `LockFeatureAndTask`, the refusal, `onClaim` (it cancels `queued` and `failed` dispatches and withdraws `dispatch-failure`), the base reset in round 1 (`:443`), and the execution. `TestClaimTakesAnActiveTaskFromTheQueue` passed. *Note:* the cancelled dispatch IDs are audited on a sibling row, `task.taken_by_claim`, in the same transaction (`claims_task.go:215`). The spec puts them in `claim.claimed`'s payload. |
| FR-2.4 Refusals as sentences | **Met** | `claims_task.go:77-152`. `TestClaimRefusals` and `TestClaimTaskToolRefusals` passed. The pending, not-built, agent-running, stalled-attempt, spec-stale, held-by-a-person, in-review and done refusals are tested word for word. The "working copy is missing" and pending-`revision-in-flight` refusals are in code only (`claims_task.go:121-126`, `:109`). |
| FR-2.5 Submitting | **Met** | `claims.go:533-644`. `TestSubmitCommitsAndQueuesTheReviewWithTheChatModel` covers the trailers, the hash recorded and the review queued. `TestSubmitTaskToolResultAndRefusals` covers the summary, no change, twice and in review. `TestResubmittingAReviewedCommitStillQueuesAReview`. The spec-revision notice is code only (`claims.go:637`). |
| FR-2.6 Verdict on a claimed task | **Met** | `actions_phase2.go:382` (approve ends the claim, `done`), `:357` (abandon), `:521` (send-back, no implementer). `TestSendBackReturnsToTheClaimAndApprovalEndsIt`, `TestAbandonedTaskEndsItsClaim` and the end-to-end test passed. |
| FR-2.7 Races and one lock order | **Met** | `StartImplementDispatch` (`store/dispatches.go:503`); `dispatchReadyTasks` locks and skips; the `TransitionTask` guard (`store/tasks.go`, `ErrStaleState`); `RequeueUnlessClaimed`. These tests passed: `TestClaimRacingTheDispatcherHasOneWinner` (claim won 16, dispatcher 4, of 20), `TestClaimOnOneTaskWhileASiblingStarts` (16 and 4), `TestClaimRacingDispatchReadyTasks`, and `TestTransitionTaskStateGuard` (both the store and server versions). |
| FR-2.8 One hand in git | **Met** | `withWorkingCopy` (`claims.go:205`). The fingerprint uses `--no-optional-locks` (`:224`). `TestWithWorkingCopyIsOneAtATimePerPath` and `TestAClaimWaitsForAnImplementersCompletion` passed. |
| FR-2.9 Release | **Met** | `claims.go:701-772`, `claims_task.go:284-324`. `TestReleaseKeepsTheWorkAndQueuesAnImplementer` passed. It asserts the "work left by the chat agent, released" message and both trailers. |
| FR-2.10 `claimable` interface | **Met** | `claims.go:167-197`; `task` is registered. The signatures differ slightly from the sketch: `refusal` takes the claimant; `onClaim` and `onSubmit` return the cancelled IDs and the review run; `onRelease` takes `by`. The contract is preserved. |
| FR-2.11 Audit kinds | **Met** | `store/claims.go:71-80`, and `claim.activity` in `claim_sweep.go:126` and `KeepClaim`. Asserted in the sweep tests (`auditRefCount`). |
| FR-2.12 Reviewer's comments to the implementer | **Met** | `planner.go` (the "What the code reviewer asked for" section) and `reviewComments` (`claims.go:812`), used by MCP, the panel and the planner. `TestPlanImplementShowsTheReviewersCommentsInRoundTwo` passed. |

### FR-3: The MCP tools

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-3.1 `claim_task` description | **Met** | `mcp_claim_tools.go:29-41`. Every listed point is present. |
| FR-3.2 Its result | **Met** | `claimResultMap` (`mcp_claim_tools.go:85`). `TestClaimTaskToolResultAndRenewal` asserts `task`, `feature`, `working_copy` (absolute, branch, base equals head), `contract`, `rules` (in order, with the commands), `expires`, `next` and the renewal. `chat-output.txt` shows the same. |
| FR-3.3 `submit_task` | **Met** | `mcp_claim_tools.go:44-55`. |
| FR-3.4 Its result | **Met** | `TestSubmitTaskToolResultAndRefusals`: `commit` equals the head, the task is in `review`, `review.run_id` and `model` match the dispatch, and the `next` sentence is exact. |
| FR-3.5 Refusals as `isError` | **Met** | `refusalOf` asserts `isError`, in every MCP refusal test. |
| FR-3.6 `get_feature` `tasks` | **Met** | `mcp_tools.go:448-456`. Tests: `TestClaimTaskToolResultAndRenewal` (plan order, `depends_on`, `claimable`, `why_not` matching the refusal, `url`), `TestChatSendBackAndVerificationOverMCP` (`review_comments`), `TestPersonsClaimIsNotTheChatAgents` (`released_by`). |
| FR-3.7 `initialize` instructions | **Met** | `mcp.go:320-322`. In code only; no test asserts the text. |

### FR-4: The web UI

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-4.1 Claim panel, six states | **Met** | `executors.html:30-118`. Tests: `TestTaskClaimPanelRenders`, `TestPersonClaimsATaskInTheUI`, `TestReturnedClaimShowsTheReviewersCommentsOnTheTaskPage`, `TestPersonsReturnedClaimShowsTheReviewersComments`. Screenshots 04 (claimable), 05 (a person's open claim: path, branch, spec and plan links, Submit, Release), 07 (submitted) and 02 (submitted by the chat agent). |
| FR-4.2 Routes | **Met** | `ui.go:715` and onwards; `ui_claims.go:315-371`, using `PersonClaimant()` (`kind: person`, `via: ui`, `ui_actor`). Tests: `TestPersonClaimsATaskInTheUI` (refusals as banners), `TestUIClaimRoutesRedirectAndRefuseCrossSitePosts`. |
| FR-4.3 Release by `ui_actor` | **Met** | `ui_claims.go:366`. `TestPersonClaimsATaskInTheUI` asserts `EndedBy == uiActor()`. |
| FR-4.4 Icons, labels, waiting sentence | **Met** | `ui_claims.go:222`. `TestPersonClaimsATaskInTheUI` asserts "… is claimed by …, so this feature's agents are waiting." for a person and for the chat agent, and its absence after the release. |

### FR-5: The safety nets

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-5.1 Claim sweep on the heartbeat | **Met** | `claim_sweep.go:25-96`; `server.go:270`. See special scrutiny 4. |
| FR-5.2 Fingerprint | **Met** | `claims.go:224-256`. `TestWorktreeFingerprintChangesWithTheWorkingCopy` passed. |
| FR-5.3 `claim-stale` question and answers | **Met** | `claims_task.go:326-335`. The sweep tests assert the exact text and the two answers. Screenshot 06. *Note:* it says "claimed … 1 day ago", where the spec's example says "26 hours ago". The spec's example lags the code (B18). |
| FR-5.4 Activity withdraws; a late answer is a no-op | **Met** | `TransitionClaim` and `KeepClaim` withdraw. The notice is `claimMovedOnNotice`. Tests: `TestEditingTheWorkingCopy…`, `TestRenewingAClaim…`, `TestALateAnswerIsANoOp`, `TestAnAnswerAfterTheClaimWasSubmittedChangesNothing`. |
| FR-5.5 `claims.expiry_hours` | **Met** | `config.go:98-118,347`. `TestClaimsConfig` passed: 24 by default; 0 and -3 refused. |
| FR-5.6 Branch watch | **Met** | `branch_watch.go:75-197`. It runs on the heartbeat (`server.go:271`) and asynchronously on post-commit (`http.go:482-486`). Tests: `TestAnUnclaimedCommitRaisesTheNotice`, `TestARewrittenBranchRaisesTheNotice`, `TestACommitOnMainMergedIntoTheBranchIsNotFlagged`, `TestAFeatureInReviewIsWatched`, `TestThePostCommitHookTriggersTheWatch`. *Note:* it uses `git log --no-merges` in place of `rev-list`, so a merge commit that brings `main` in is never flagged. This is deliberate, and tested. |
| FR-5.7 Subutai's commits recorded | **Met** | `RecordWorktreeCommit` at completion, submit and release. The watch runs first (`claims.go:369,592,734`; `actions_phase2.go:280`). Tests: `TestSubutaisOwnCommitsAreNotFlagged`, `TestAReleasesCommitIsNotFlagged`, `TestAPersonsCommitBeforeTheImplementersCompletionIsJudged`, `TestAClaimJudgesACommitMadeJustBefore`. |
| FR-5.8 `unclaimed-commit` checkpoint | **Met** | `branch_watch.go:199-279`. `TestAnUnclaimedCommitRaisesTheNotice` asserts the exact question, the extension of a pending notice ("Two commits …") and **I've seen this**. Screenshot 08. |
| FR-5.9 `watched_head` start and boot backfill | **Met** | `worktree_ops.go:92,204`; `branch_watch.go:316`; `server.go:152`. `TestBootSetsWhereTheWatchStarts` and `TestReconcileKeepsTheWatchedHead` passed. |
| FR-5.10 Inbox and routing | **Met** | `ui_views.go` (`answerOptions` and `responseFor`); `rules.go:823-828`; `rules_claims.go:48`. The keep and release answers are exercised end to end through `/ui/respond` in the sweep tests. Screenshots 06 and 08. |

### FR-6: The boundary

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-6.1 Exactly two new tools, with the header comment | **Met** | `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`. See special scrutiny 2. |
| FR-6.2 Must-not-exist list | **Met** | The same test, lines 207 to 222; each name returns method-not-found. `chat-output.txt` shows four of them refused in the demo. |
| FR-6.3 Verification never claimable | **Met** | `claims.go:312-333`. `TestClaimTaskToolRefusals`, `TestClaimRefusals` and `TestChatSendBackAndVerificationOverMCP` passed, and the end-to-end test counts `verify-feature` succeeded = 1. |
| FR-6.4 Code review can't be skipped | **Met** | Submit always enqueues `review-code`, or rolls back (`claims_task.go:258-282`). `assertNoClaimedTaskApprovedWithoutReview` passed. *Note:* the helper accepts any answered checkpoint, not only `review-deadlock` and the escalation (B19). |
| FR-6.5 Out-of-order acts and races | **Met** | Every listed case is tested and passed: `TestSubmitTaskToolResultAndRefusals`, `TestPersonsClaimIsNotTheChatAgents`, `TestClaimRefusals` (from the other surface), `TestSecondClaimInAFeatureIsRefused`, `TestSubmitCommitsAndQueuesTheReviewWithTheChatModel` (releasing a submitted claim), `TestSendBackWhileAnAgentRunsOnAnotherTask`, `TestClaimOnAnExhaustedDispatchWithdrawsItsCheckpoint`, the three race tests, and `TestAbandonedFeatureWithAnOpenClaimRaisesNothing`. |

### FR-7: Unmeasured actuals

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-7.1 The marker | **Met** | `store/unmeasured.go:18-74`. Tests: `TestChatExecutionMakesTaskFeatureInitiativeUnmeasured`, `TestSpecWriterMakesFeatureUnmeasured`, `TestBugReportIsTheBugsSpec`. |
| FR-7.2 `ActualTokens`, corpus and calibration before `LIMIT`, API and CLI | **Met** | See special scrutiny 3. API: `http_phase3.go:57-62,103` and `TestEstimateAPIReportsUnmeasured` (`actual_tokens` and `delta` null; `unmeasured`; `measured_part`). CLI: `cmd/subutai/main.go:654` and `TestPrintEstimateUnmeasured`. |
| FR-7.3 "Not measured: at least …" | **Met** | `entity.html:258`. The ui_render test asserts "Not measured: at least 12,400 tokens, plus work done in chat or by a person.", and that there is no "at least 0". Screenshot 03's rail. |
| FR-7.4 Forecast sampling | **Met** | `store/sends.go` (`PurposeTokenSamples`). `TestPurposeTokenSamplesSkipsUnmeasuredRound` passed. |
| FR-7.5 Cost unchanged, with a note | **Met** | `http_phase3.go:526-531`. `TestEstimateAPIReportsUnmeasured` asserts the cost and the note. |

### FR-8: The reviewer for the chat agent's work

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-8.1 `claims.chat_review_model` | **Met** | `config.go:121,349-352`. `TestClaimsConfig` passed: an unknown model is refused, and `same` is accepted. |
| FR-8.2 Default: the priciest output price, ties by name | **Met** | `config.go:121-140`. `TestClaimsConfig`. The end-to-end test asserts `claude-sonnet-5` on the dispatch rows. |
| FR-8.3 Where it applies | **Met** | `codeReviewModel` (`claims_task.go:439`), used by submit and by completion; `queueReview` (`actions.go:289-298`). Tests: `TestSubmitCommitsAndQueuesTheReviewWithTheChatModel`, `TestPersonsSubmitIsReviewedWithTheUsualModel`, `TestQueueReviewUsesTheChatModelForAChatWrittenSpec`, and the end-to-end test. |
| FR-8.4 Visible on the task page | **Met** | `executors.html:20-22`. `TestChatAgentsWorkNamesItsReviewer` and `TestAPersonsOrUnchangedChatReviewerIsNotNamedAsTheChatReviewer` passed. The sentence is shown only when the chat model differs from the usual one. *Walkthrough inaccuracy:* the walkthrough says screenshot 02 shows "which model reviews it". It doesn't, because in the demo the chat model equals the usual one. |
| FR-8.5 Starter `claims:` block | **Met** | `starter/starter.go:92-99`. The text matches the spec. |

### FR-9 and FR-10

| Criterion | Verdict | Evidence |
|---|---|---|
| FR-9.1 `work-a-task` skill | **Met** | `starter/pack/chat-skills/work-a-task/SKILL.md`. I read it, and it covers every listed point. `TestWorkATaskSkillSaysWhatTheRulesSay` reads it from the `.subutai/chat-skills/` that `subutai init` installed. |
| FR-9.2 Rules match the skill | **Met** | The same test checks every rule in `claimRuleSentences` (`claims.go:518`) against the skill's text. |
| FR-10.1 End-to-end test | **Partly met** | `TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents` passed, and it proves the done-when (special scrutiny 1). **Missing:** step 3's "T02's implementer runs while T01 is reviewed" isn't asserted, only that both happen. |
| FR-10.2 `TestPersonClaimsATaskInTheUI` | **Met** | Passed. It covers the claim, the submit with a summary, the review on the usual model (`haiku`), the release of the chat agent's claim, and the refusals as banners. |
| FR-10.3 A bug's task claimed and submitted over MCP | **Met** | `TestChatClaimsABugsTask` passed (`BUG-nnn-T01`, claimed and submitted, with the bug itself refused). It has a different name from the spec's `TestBugTaskCanBeClaimed`. The end-to-end file's header says so. |

### Non-functional requirements

| Criterion | Verdict | Evidence |
|---|---|---|
| NFR-1 One service layer | **Met** | MCP (`mcp_claim_tools.go:61,110`) and the UI (`ui_claims.go:320,349,366`) both call `ClaimWork`, `SubmitWork` and `ReleaseClaim`. Each act's database changes are in one `WithTx`, with their audit rows. |
| NFR-2 The boundary holds | **Met** | FR-6.1 and FR-6.2. |
| NFR-3 The files changed, for M14 | **Partly met** | The list doesn't match the diff. It **omits** files this branch changes: `store/entities.go` (where `TransitionFeature` ends claims), `store/checkpoints.go`, `store/worktrees.go`, `store/unmeasured.go`, `server/ui_entity.go`, `server/branch_watch.go`, `server/claims_task.go`, `server/claim_check.go`, `rules/rules_claims.go`, `rules/rules_phase2.go`, `ui/templates/inbox.html`, `icons.html` and `app.css`. It **names** `server/documents.go` and `server/observe.go`, which aren't changed. Its stated purpose is M14's merge, so this matters for coordination. |
| NFR-4 Human prose, British English | **Met** | I read every sentence quoted above. Spellings such as "serialised" and "labelled" are British. Activity words are rendered in words (`claimActivityWords`, `claims.go:290`). |
| NFR-5 Tested as before | **Met** | My run: vet is clean, `gofmt -l` is empty, and the race suite is green, with the integration tests running against Postgres (§ Test run). |
| NFR-6 No git under a database transaction | **Met** | In claim, submit, release, completion, the sweep and the watch, git runs before `WithTx`, under `withWorkingCopy`, and the transaction re-checks the state (`claims.go:364-462,566-644,721-772`; `actions_phase2.go:252-325`; `claim_sweep.go:103-139`; `branch_watch.go:92-197`). |

### Scope decisions, as built

| Criterion | Verdict | Evidence |
|---|---|---|
| SD-1 Executions table with SPEC-017's vocabulary | **Met** | `0014:49-75`. The words are `agent`/`chat`/`person`, built on `whoWords` (`claims.go:904`). |
| SD-2 General claims; `deadline_at`; nullable `feature_id` | **Met** | No foreign key on `ref_id`; `deadline_at`; `feature_id` nullable (`0014:15-35`). The interface is at `claims.go:167`. `TestTheDeadlineQuestion` passed. |
| SD-3 What can be claimed | **Met** | `claims_task.go:77-152`. `TestClaimTakesAnActiveTaskFromTheQueue`, `TestClaimRefusals` (including the stalled-attempt window) and `TestClaimOnAnExhaustedDispatchWithdrawsItsCheckpoint`. |
| SD-4 One hand in the working copy | **Met** | The partial unique index (`0014:40`) and `ImplementHold`'s `QueueReasonClaimOpen`. `TestGovernorHoldsImplementersWhileAClaimIsOpen` passed. The end-to-end test asserts the exact queue reason. |
| SD-5 Own surface submits; chat can't release | **Met** | `Claimant.holds` checks kind, actor and via. There is no `release_task` tool. `TestClaimRefusals` covers a submit from the other surface. |
| SD-6 A send-back returns to the claim | **Met** | `actions_phase2.go:517-523`. The end-to-end test and `TestChatSendBackAndVerificationOverMCP` cover it. |
| SD-7 Releasing keeps the work | **Met** | `claims_task.go:284-324`. `TestReleaseKeepsTheWorkAndQueuesAnImplementer`. |
| SD-8 What counts as activity | **Met** | Claimed, renewed, resumed, worktree, submitted, sent_back and kept are all set. Reads change nothing: `get_feature` is a pure read. The sweep tests cover it. |
| SD-9 One expiry, downtime counted | **Met** | `ClaimExpiry()` applies to both kinds. The fingerprint is taken before expiry is judged (`claim_sweep.go:60-72`). `TestAReturnedClaimExpiresToo`. |
| SD-10 The branch is the feature's paths | **Met** | See FR-5.6 to FR-5.9. The stated false negative stands (`TestACommitDuringAnOpenClaimIsNotFlagged`). |
| SD-11 A notice with one answer; merge not held | **Met** | `answerOptions` returns `seen` only, and the rule returns nil (`rules.go:826-828`). Screenshot 08. |
| SD-12 No estimation, spec or plan claims | **Met** | Only task IDs resolve (`claims.go:316`). `claim_estimate` and `claim_feature` don't exist (FR-6.2). |
| SD-13 What is unmeasured | **Met** | See FR-7.1. The end-to-end test covers the task, the feature and the initiative. |
| SD-14 Stronger reviewer by default; `same` opts out | **Met** | See FR-8.1 to FR-8.3. |
| SD-15 No quote | **Met** | The `claim_task` schema has only `task` (`mcp_claim_tools.go:38-40`). Audits carry `via: mcp`. |
| SD-16 A claim ends with its task or feature | **Met** | `endTaskClaim` (`claims_task.go:367`); `EndClaimsForFeature` in `TransitionFeature` (`store/entities.go:256`). `TestFeatureAbandonEndsItsClaims`, `TestAbandonedTaskEndsItsClaim`, `TestEndClaimsForFeature` and `TestAbandonedFeatureWithAnOpenClaimRaisesNothing` passed. |
| SD-17 Comments on rework | **Met** | See FR-2.12. |
| SD-18 Told not to commit | **Met** | `claimRuleSentences[1]`, the `claim_task` description and the skill's step 6. A claimant's own commit isn't refused (`prepareSubmit` only commits when the copy is dirty). |

### Definition of done

| Criterion | Verdict | Evidence |
|---|---|---|
| DoD 1 Every FR's acceptance passes in the suite | **Partly met** | The suite is green. Two acceptance clauses aren't fully shown: FR-10.1 step 3's concurrency, and FR-1.1's deletion clause. Several sentences are proved by code only, not by a test: FR-3.7, the FR-2.5 revision notice, and two FR-2.4 refusals. |
| DoD 2 vet, gofmt, race suite green, integration tests running | **Met** | § Test run. `go list ./...` has no packages outside `internal/` and `cmd/`, so my run is `./...`. |
| DoD 3 Demo with no AI provider | **Met** | `docs/walkthrough-spec-020.md`, `setup.sh`, `walk.js` and `chat-output.txt` (plain MCP JSON-RPC claim, a refused second claim, submit, a refused second submit, four absent tools). Screenshots 01 to 09, all opened. Together they show the executor icon (03, 09), a person's claim (05), the stale question with a backdated claim (06) and the unclaimed-commit notice (08). The walkthrough says the review and verification are proved by the mock tests. *Defects in the write-up:* it links `notes/handoff-M13-2026-10-02.md`, which doesn't exist. And its claim that screenshot 02 names the review model isn't borne out (see FR-8.4). |
| DoD 4 Handoff note `docs/notes/handoff-M13-2026-10-02.md` | **Not met** | The file doesn't exist on the branch. The orchestration note puts it after verification (stage 7). |
| DoD 5 Roadmap §11 marks M13 done | **Not met** | `notes/subutai-status-and-roadmap-2026-09-28.md:728` still reads "**M13 — Executors** · M · needs M2, M10", with no done mark or handoff pointer. |
| DoD 6 REVIEW-020 recorded, findings dealt with (§8) | **Met** | `docs/reviews/REVIEW-020-executors.md`, and SPEC-020 §8's table (R20-1 to R20-21). The code reviews' rounds 1 and 2 are recorded too. Round 2 ends "No major finding is open", with the minor findings B14 to B19 still open. |
| DoD 7 Sam approves the spec with the build | **Pending** | Sam's act. |
| DoD 8 Sam's yes on the eighteen choices | **Pending** | Sam's act. SD-1 to SD-18 are built as written (above). |
| DoD 9 Dated notes on approval | **Pending** | These follow the approval. |

## Test run

**Commands, in `/home/user/cromwell`:**

```
eval "$(SUBUTAI_TEST_PGDATA=/var/tmp/pgdata-vf SUBUTAI_TEST_PGPORT=54371 CLAUDE_CODE_REMOTE=true scripts/test-db.sh)"
go test -race -count=1 -v ./internal/... ./cmd/...
go vet ./...
gofmt -l internal cmd
```

**Postgres:** a fresh cluster. `test-db.sh` reported "initialising a cluster in
/var/tmp/pgdata-vf", then "starting on port 54371". When I had finished, I
stopped it with SIGINT to the PID in `/var/tmp/pgdata-vf/postmaster.pid`, and
the pid file was gone.

**Results:**
- The exit status was 0, and every package was `ok`. Packages with no test
  files: `bus`, `dispatch`, `provider`, `provider/anthropic` and `testdb`.
- **413 top-level tests passed** (496 `--- PASS` lines, counting subtests), and
  **0 failed**.
- **2 skips:**
  - `TestDemoM6` (`internal/server/demo_m6_test.go:32`): "set
    SUBUTAI_M6_DEMO to a directory to run the SPEC-012 demo". This is an
    opt-in demo, not an integration test.
  - `TestChecklistBackfillRunsOnlyWithTheTable`
    (`internal/store/identity_test.go:186`): "a checklists table exists on
    this branch; the with-table case below covers it". It skipped after
    connecting to the database (0.35s), so it isn't a missing-database skip.
- **The integration tests ran; none skipped for want of a database.**
  - `internal/server` took 231.3s, and `internal/store` 18.5s.
  - The race tests logged their winners: "claim won 16, dispatcher won 4 of
    20" and "claim won 16, sibling won 4 of 20".
- **Time:** the `go test` run took 249s by the wall clock (start and end
  stamps), including the build.
- `go vet ./...` exited 0, with no output. `gofmt -l internal cmd` printed
  nothing.

## Tally

| Verdict | Count |
|---|---|
| Met | 85 |
| Partly met | 4: FR-1.1, FR-10.1, NFR-3, DoD 1 |
| Not met | 2: DoD 4, DoD 5 |
| Pending (Sam) | 3: DoD 7, 8, 9 |

## Final verdict

**The build meets SPEC-020. The milestone isn't done.**

- **The build meets SPEC-020** in all but four small respects.
  - The roadmap's done-when is proved by a test I read and saw pass. A task
    claimed and worked in chat goes through the same dispatched code review,
    on a stronger model by default, and the same dispatched verification and
    merge as an agent's task.
  - The judgement boundary holds by absence, and is tested.
  - Unmeasured work is excluded before the `LIMIT`, and that is proved by a
    test that would fail otherwise.
  - Claim expiry works end to end, through the Inbox.
- **The partial items are small, and none is a defect in behaviour:**
  - an untested concurrency clause in FR-10.1;
  - an unbuilt but currently unreachable deletion clause in FR-1.1;
  - NFR-3's file list, which needs correcting before M14 merges against it.
- **The milestone isn't done.** The M13 handoff note (DoD 4) and the
  roadmap's done mark (DoD 5) don't exist yet, and the walkthrough already
  links the missing handoff. Sam's approval and notes (DoD 7 to 9) are
  pending.

**Recommendation:**
- Write the handoff and mark the roadmap. In the handoff, carry the open minor
  findings B14 to B19 as bug reports, as the orchestration note asks.
- Correct NFR-3's file list.
- Optionally, tighten the two assertions B19 names. These are the FR-10.1
  partial and the FR-6.4 note.
- Then put it to Sam.

## Addendum, by the lead (2026-10-02)

After this record was written:
- **DoD 4 and DoD 5 are met.** The handoff,
  [handoff-M13-2026-10-02](../notes/handoff-M13-2026-10-02.md), was written,
  and the roadmap's §11 marks M13 done, pointing to it.
- **NFR-3 is corrected.** SPEC-020's file list now matches the diff.
- **FR-1.1 is corrected.** The sentence about deleting executions with their
  task was wrong, not the code. No task with executions is ever deleted, and
  the spec now says so.
- **The walkthrough's sentence on screenshot 02 is corrected.**
- **FR-10.1 step 3 stays partly met.** The end-to-end test lets T02's
  implementer run while T01 is reviewed, but doesn't assert it. The handoff
  lists it as a bug-report candidate, with the FR-6.4 helper's loose check.
