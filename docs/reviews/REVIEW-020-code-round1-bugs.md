# REVIEW-020 code, round 1: bugs and spec conformance

**Reviewer:** Claude, as the bug and spec-conformance reviewer for M13.
**Date:** 2026-10-02
**Diff:** `git diff main...claude/subutai-m13-executors`, at `f13614b`.
**Binding documents:** SPEC-020 (with §8's record of REVIEW-020's fixes),
DEC-007, the M13 dev plan.
**Scope:** correctness, races, transactions, state-machine holes, security,
the judgement boundary, and error handling. Each FR, NFR and SD is checked
against the code below. Code quality is another reviewer's job.

**What I ran:**
- `go build ./...` and `go vet ./...`: clean.
- `go test -count=1` for `store`, `lifecycle`, `config`, `rules` and
  `timeline`: green.
- The server's claim, MCP, UI, unmeasured and branch-watch integration tests:
  green.
- `go test -race -count=1 ./...` against Postgres on port 54351: the result
  is at the end of this review.

## Summary

The core is sound. These all hold:
- the lock order (feature, then task, then dispatch rows);
- the guarded `TransitionTask`;
- `StartImplementDispatch` re-checking under the feature's lock;
- `dispatchReadyTasks` skipping claimed tasks;
- `RequeueUnlessClaimed`;
- the per-working-copy mutex, held across "git, then the transaction";
- the conditional sweep update;
- abandonment ending claims inside `TransitionFeature`.

I found no deadlock cycle. I found no git under a transaction. I found no
route that lets the chat agent approve, review, verify, release, start
building or answer a checkpoint through MCP. No route skips the code review
or the verification.

The real problems are these:
- **The end-to-end and race tests (dev-plan task T7) were never merged.**
  FR-10.1 has no test. Most of FR-6.5's races are untested.
- **A returned claim's task page hides the reviewer's comments.**
- Several narrow races and edge cases strand a task, raise a false question,
  or let the branch watch miss a commit.

## Findings

### B1: major. The end-to-end proof and the race tests are missing (FR-10.1, FR-6.4, FR-6.5, NFR-5, DoD 1)

**Location:** `internal/server/*_test.go`. The branch log has no T7 merge
(`git log main..HEAD`).

**Problem:**
- `TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents` (FR-10.1) doesn't
  exist. Its two-task scenario isn't tested anywhere: T02's implementer runs
  while T01 is reviewed, T02's review uses the usual model, and the feature is
  left out of the corpus while a measured feature stays in.
- These FR-6.5 cases have no test:
  - a claim racing `dispatchReadyTasks` and `StartImplementDispatch`
    ("exactly one wins, and no implementer ever runs on a claimed task");
  - "a send-back while an agent runs on another task: the claim is
    `returned`, and resuming it is refused until the agent finishes";
  - submitting twice, and submitting a task in review;
  - "the sweep raises nothing" after a feature is abandoned:
    `TestFeatureAbandonEndsItsClaims` never runs `ClaimSweep`.
- FR-6.4 asks for a test that a claimed task reaches `done` *only* through
  the reviewer or a person's answer. No such test exists.
  `TestChatSendBackAndVerificationOverMCP` covers the happy path, with one
  task (`integration_mcp_claims_test.go:306`).

**Scenario:** a regression in `StartImplementDispatch`'s re-check, or in
`dispatchReadyTasks`' claim skip, would pass the suite. The dev plan names
that start as "the riskiest change", and says "T7's race tests … guard it".

**Fix:** merge T7. Two tests matter most:
- run `ClaimWork` and `StartImplementDispatch` concurrently on the same
  feature, many times, and assert that one wins and no agent execution exists
  for the claimed task;
- write FR-10.1 as specified.

### B2: major. A returned claim's task page doesn't show the reviewer's comments (FR-4.1, SD-6)

**Location:** `internal/server/ui_claims.go:184`.

**Problem:** the panel loads the comments only when the latest *execution*
is in round 2 or later. A send-back records no execution. The next round's
row is written only when the claim is resumed (`claims.go:388-393`). So the
latest execution is still in round 1 for as long as the claim is `returned`.
In that state, the claim's panel gets `Comments == nil`, and the
`claim-comments` include renders nothing.

FR-4.1 says: "A returned claim: the review comments, and Resume … or
Release". SD-6 says the comments are "on the task page".

**Scenario:**
1. A person claims T01 in the UI and submits it.
2. The reviewer sends it back.
3. The task page says "The code reviewer sent this back. Resume it…", but not
   what the reviewer asked for.
4. The person decides whether to resume or release it blind.

The comments appear only after Resume. MCP is right, because it uses
`CurrentTaskRound` (`mcp_claim_tools.go:182`, `claims.go:464`).

**Fix:** use `store.CurrentTaskRound(ctx, pool, t.ID) > 1`, as `mcpTaskEntry`
does. Add an integration assertion on the returned panel.

### B3: minor. The retry sweep can raise a `dispatch-failure` question about a task that was just claimed (FR-6.5, SD-3)

**Location:** `internal/store/dispatches.go:680`, and
`internal/dispatch/dispatch.go` `RetrySweep` (the `exhausted` loop).

**Problem:** `CancelIfTaskClaimed` returns `d.State == "failed"`. Suppose a
claim cancelled the dispatch after `FailedRetryable` listed it but before
this transaction. Then the dispatch is `cancelled`, the task is claimed, and
the function still returns `false`. The sweep falls through and publishes
`DispatchExhausted`. The rules then raise a `dispatch-failure` checkpoint on
a claimed task. A claim that lands between the publish and the rule's insert
has the same effect.

**Scenario:**
1. T1's dispatch has run out of attempts, and the heartbeat's `RetrySweep`
   reads it as exhausted.
2. The chat agent's `claim_task` commits now, and cancels the dispatch.
3. `CancelIfTaskClaimed` returns false.
4. The Inbox asks "Dispatch failed all attempts. Retry or cancel?" about a
   task the chat agent is working on. Either answer is a no-op.

**Fix:** return `true` whenever the task has a current claim, whatever the
dispatch's state. Have the rule that raises `dispatch-failure` re-check that
the dispatch is still `failed` and its task unclaimed. Alternatively, raise
it in the same transaction as the check.

### B4: minor. A submit can leave the task in `review` with no review queued

**Location:** `internal/server/claims_task.go:261-266`.

**Problem:**
- `onSubmit` moves the task to `review` and enqueues `review-code` with the
  key `review-code:<task>:<head>`.
- `EnqueueDispatch` returns `nil, nil` when a dispatch with that key is
  `queued`, `running` or `succeeded` (`dispatches.go:64-70`).
- `onSubmit` then returns `"", nil`. The transaction commits with the task in
  `review`, the claim `submitted`, and no live review.
- Nothing ever moves the task again: a submitted claim can't be released, and
  no review is coming.
- The result's `review.run_id` is `""` (`mcp_claim_tools.go:116`), and the
  chat agent is told "A code reviewer will review this."

**Scenario:** the head after the submit equals a commit already reviewed for
this task.
1. Round 1 is commit X, which the reviewer sends back.
2. Round 2 is commit Y, which is also sent back.
3. On resuming, the claimant runs `git reset --hard X`. FR-2.5's check passes,
   because the copy differs from `start_head` Y.
4. Nothing is dirty, so `prepareSubmit` makes no commit, and the head is X.
5. The key `review-code:T:X` exists in `succeeded`. No review is queued.

The case is contrived, but the failure is silent and permanent.

**Fix:** in `onSubmit`, treat `d == nil` as an error that rolls the
transaction back, with a refusal that says the work matches a reviewed
commit. Or make the key unique per submit, by adding the claim and round.

### B5: minor. Re-creating a worktree at boot moves the branch watch past commits it never judged (FR-5.6, FR-5.9)

**Location:** `internal/server/worktree_ops.go:201`, which calls
`startWatching` at `branch_watch.go:300-310`.

**Problem:** `ReconcileWorktrees` re-creates a missing worktree directory.
When the branch already exists, `addWorktree` checks it out (`:107-110`).
Then `startWatching` sets `watched_head` to the branch head unconditionally.
Two kinds of change are then never reported:
- any commit made on the branch since the last watch, for example from
  another checkout while the server was down;
- a rewrite of the branch.

FR-5.9 limits the head-at-start rule to a worktree that has no
`watched_head`.

**Scenario:**
1. The server is stopped.
2. The worktree directory is deleted, and someone commits to
   `subutai/auth/login` from another clone and pushes, or fetches into the
   repository.
3. On boot, the directory is re-created and `watched_head` jumps to the new
   head.
4. The unclaimed commits are never flagged.

**Fix:** in the reconcile path, call `startWatching` only when
`wt.WatchedHead == ""`, as `BackfillWatchedHeads` does.

### B6: minor. Claiming doesn't run the branch watch first, so a just-made unclaimed commit becomes the claim's work (SD-10, FR-5.7)

**Location:** `internal/server/claims.go:329-336` and `:344-428`. Compare
`submitLocked:565-567` and `releaseLocked:705-707`, which run the watch
first.

**Problem:** the watch drops new commits whenever the feature has an `open`
claim (`branch_watch.go:166-172`). A commit made before the claim but not yet
judged is therefore swallowed if the claim opens first. This happens in two
cases:
- the asynchronous post-commit watch hasn't taken the lock yet;
- the commit was made while the server was down, or the hook isn't
  installed, and the next heartbeat hasn't run.

FR-5.7 runs the watch before completion, submit and release, so that "a
person's commit made just before Subutai's is judged, not swallowed". A claim
has the same property and isn't covered.

**Scenario:**
1. With no claim, a person commits a fix on the feature's branch in the
   worktree.
2. Seconds later, before the heartbeat's watch runs, they press **Claim this
   task**.
3. The next watch sees an open claim and drops the commit. No
   `unclaimed-commit` notice is raised, and the commit is never reviewed as
   unclaimed work. It is reviewed only if it falls inside this task's diff,
   which it does, since the base is reset to the head *after* the commit.

**Fix:** call `s.watchBranchLogged(ctx, t.Worktree)` at the start of
`claimLocked`, before the head and fingerprint are read. Record the gap in
SPEC-020's FR-5.7.

### B7: minor. Between an implementer's success and its completion, the working copy looks free

**Location:**
- `claims_task.go:124` (`runningTask` counts only `state = 'running'`);
- `dispatches.go:406-430` (`ImplementHold`);
- `dispatch.go:386-399` (succeeded, then published);
- `actions_phase2.go:254-304` (the completion commits with `git add -A`).

**Problem:**
1. `MarkAttemptSucceeded` commits the dispatch as `succeeded`.
2. Only after that does the rules engine run `completeImplementation`, which
   commits the worktree.
3. In between, no check treats the copy as held:
   - a claim on a *sibling* task passes SD-3, because `RecentImplementActivity`
     looks only at the claimed task's own dispatches;
   - `StartImplementDispatch` can also start a sibling's implementer.
4. `completeImplementation` doesn't check for an open claim before its
   `git add -A`.

**Scenario:**
1. T02's implementer succeeds.
2. The chat agent claims T01 at once, and starts editing.
3. T02's completion is delayed, for example by a slow event loop. When it
   runs, it commits the chat agent's half-done T01 edits as T02's work, which
   T02's reviewer then reviews.

The window is usually milliseconds. The same window existed for two agents
before M13, but claims make human-speed edits possible inside it.

**Fix:** treat a succeeded implement dispatch whose task is still `active`
as holding the copy, in both `runningTask` and `ImplementHold`. Or have
`completeImplementation` refuse, and raise a checkpoint, when another task in
the feature has an open claim.

### B8: minor. A model error in the implementer's completion leaves Subutai's own commit unrecorded, so the watch flags it as unclaimed

**Location:** `internal/server/actions_phase2.go:284-290`.

**Problem:**
- `codeReviewModel` now runs inside the transaction, after the commit.
- On error, the closure returns `s.configErrorCheckpoint(...)`. That function
  writes its checkpoint in a separate transaction and returns `nil`.
- So the outer transaction commits having done nothing:
  - no transition;
  - no `RecordWorktreeCommit`;
  - no `submitted_at`;
  - no review.
- The task is left `active` with a succeeded dispatch.
- The next branch watch finds the unrecorded `subutai:` commit. If no claim is
  open, it raises `unclaimed-commit`, naming Subutai's own commit as work
  "nobody claimed".

**Scenario:** a typo in `routing.review-code`, or a `review-code` role on a
model that isn't configured.

**Fix:** resolve the model before the commit, as the code did before M13, or
record the commit hash before returning. Return the configuration error so
that the transaction rolls back, rather than committing an empty one.

### B9: minor. The claim routes that commit and release take cross-site posts

**Location:** `internal/server/ui_claims.go:289` (`/ui/task/submit`) and
`:316` (`/ui/task/release`). Compare `ui_edit.go:207-229`.

**Problem:** these routes run `git add -A` and commit in the working copy.
Release also hands the task to a paid agent. The editor refuses cross-site
posts for exactly this reason (R16-9, `sameOrigin`). The claim routes don't.

**Scenario:** any page the person visits auto-submits a form to
`http://localhost:<port>/ui/task/release` with a task ID. The chat agent's
claim is released, its half-done work is committed, and an implementer is
queued.

**Note:** this is not only a cross-site risk. A chat agent with shell access
on the same machine can post to these routes, and to `/ui/respond`, as "the
person". That gap predates M13, since the boundary is enforced by which MCP
tools exist, and per-user identity is M15. M13 widens what such a post can do.

**Fix:** call `refuseCrossSite` at the top of the three claim handlers.

### B10: minor. The claim routes render the page instead of redirecting (FR-4.2)

**Location:** `internal/server/ui_claims.go:266-327`.

**Problem:** FR-4.2 says "redirecting to the task page with a notice". The
handlers render the page in the response to the POST. `pushPageURL`
(`ui_entity_pages.go:278`) only helps htmx requests, and these are plain
forms.

**Scenario:** after **Submit for code review**, the browser's refresh posts
again. The person sees the error banner "… was already submitted and is in
code review". After **Release**, a refresh shows "has no claim to release".
Neither post does harm, because the service refuses, but the person sees
false errors.

**Fix:** redirect with 303 to `/ui/t/{id}`, carrying the notice or error in a
query parameter, as other entity actions do.

### B11: minor. The task page names the chat reviewer even when it isn't one, and judges by today's configuration (FR-8.4)

**Location:** `internal/server/ui_claims.go:131-138`.

**Problem:** `chatReviewedWith` prints "Reviewed with *m*, the project's
reviewer for the chat agent's work" whenever `cfg.ChatReviewModel(usual) ==
review.Model`. This goes wrong in three cases:
- with `chat_review_model: same`, it is true for every chat task;
- when the usual model is already the priciest, it is also true;
- after `config.yaml` changes, a past review is judged by the new setting.

**Fix:** record on the review dispatch, or in the `claim.submitted` audit
payload, that the chat reviewer model was applied, and show the sentence only
then.

### B12: minor. FR-1.6's plan page task list has no executor icons

**Location:** `internal/server/ui/templates/executors.html:149`
(`{{if eq .Kind "feature"}}`), and `ui_entity.go:647`.

**Problem:** FR-1.6 says the task list on "the feature page …, and the plan
page's" shows executor icons. Only the feature page gets the list. If the plan
page has no task list at all, record that and amend FR-1.6. If it has one,
it is missing the icons.

### B13: minor. Questions about a returned claim that was kept give the wrong time, and the claim panel shows a machine word (NFR-4)

**Location:**
- `claims_task.go:314-316`: the question for a `returned` claim says "was
  sent back by its code reviewer *N* ago", using `last_activity_at`. After a
  person answers **Keep the claim**, that is the time of the keep, not of the
  send-back.
- `executors.html:121`: "Last activity: {{.Claim.LastActivity}}" prints raw
  values such as `worktree` or `sent_back`. The Inbox already maps these to
  sentences (`ui.go` `claimLastActivity`).

**Fix:** for the time, read it from the `claim.sent_back` audit row, or word
the question as "has had no activity for *N* hours". For the panel, reuse the
Inbox's mapping.

## Conformance

"Met" means the code does what the requirement says, with the evidence
given. "Partly" or "Not met" refers to the finding.

### FR-1: The execution record

| Req | Status | Evidence |
|---|---|---|
| FR-1.1 | Met | `0014_executors.sql:49-75`: the checks, and unique `dispatch_id` and `(claim_id, round)`. `DeleteExecutionsFor` (`executions.go:146`) is never called, but only `pending` or `ready` tasks are deleted (`tasks.go:163`), and they have no executions. |
| FR-1.2 | Met | Agent: `dispatches.go:516-523`, with the conflict on the dispatch skipped (`executions.go:70-74`). Claim and resume: `claims.go:391,422`. Submit: `claims.go:597`, `actions_phase2.go:296`. |
| FR-1.3 | Met | `0014_executors.sql:101-115`. `TestMigration0014BackfillsAgentExecutions`. |
| FR-1.4 | Met | `claims.go:856-935`. `TestExecutorSentence`. |
| FR-1.5 | Met | `executors.html:11-24`, `entity.html` (`task-executor`). |
| FR-1.6 | Partly | Feature page: `ui_claims.go:195-231`, `executors.html:148-183`. The plan page has no icons (B12). |
| FR-1.7 | Met | `timeline.go:295-312`, `checkpointAbout`. |
| FR-1.8 | Met | `mcp_claim_tools.go:156-164`. |

### FR-2: Claims

| Req | Status | Evidence |
|---|---|---|
| FR-2.1 | Met | `0014_executors.sql:15-42`, with `claim` added to `ref_type` (`:7`). |
| FR-2.2 | Met | `lifecycle/claim.go:39-59`. Task `release`: `lifecycle/task.go`. |
| FR-2.3 | Met | `claims.go:344-436`. The head and fingerprint are read under the lock, outside the transaction (`:347-353`). Locks: `:361`. Cancellations and withdrawal: `claims_task.go:194-213`. Base reset: `:417-421`. Renewal: `:378-383`. A claim doesn't run the watch first (B6). |
| FR-2.4 | Met | `claims_task.go:76-145`. `TestClaimRefusals`. |
| FR-2.5 | Partly | `claims.go:540-616`. The lock, the holder and surface check, the no-change check against `start_head`, the trailers, the hash recorded, and the re-check in the transaction are all present. A deduplicated review strands the task (B4). |
| FR-2.6 | Met | Approve: `actions_phase2.go:357-366`. Send-back with no dispatch: `:497-506`. Abandon: `:331-337`. |
| FR-2.7 | Met in code, untested | `StartImplementDispatch` `dispatches.go:465-527`, `dispatchReadyTasks` `actions_phase2.go:180-196`, the guard `tasks.go:143-148`, `RequeueUnlessClaimed` `dispatches.go:623-651`. No concurrency test (B1). The window after an implementer's success is uncovered (B7). |
| FR-2.8 | Met | `claims.go:204-214`, used by claim, submit, release, the sweep (`claim_sweep.go:105`), the watch (`branch_watch.go:69`) and the completion (`actions_phase2.go:254`). `--no-optional-locks` at `claims.go:224-228`. No path takes the lock twice, and `watchBranchLocked` is called with it held. |
| FR-2.9 | Met | `claims.go:693-741`, `claims_task.go:269-309`. The stale question is withdrawn by `TransitionClaim` (`store/claims.go:219`). |
| FR-2.10 | Met, for M13 | `claims.go:166-195`. The documented differences are at `:161-165`. `resolveClaimRef` and `submitLocked` still assume a task: `t.Feature.ID` at `:576` and `t.Worktree.ID` at `:589`. M14 must change those. |
| FR-2.11 | Met | `store/claims.go:71-80`. `claim.activity`: `claim_sweep.go:126`, `store/claims.go:304`. |
| FR-2.12 | Met | `planner.go:258-268`, `claims.go:784-821`. |

### FR-3: The MCP tools

| Req | Status | Evidence |
|---|---|---|
| FR-3.1 | Met | `mcp_claim_tools.go:27-41`. |
| FR-3.2 | Met | `mcp_claim_tools.go:57-97`. |
| FR-3.3 | Met | `mcp_claim_tools.go:43-53`. |
| FR-3.4 | Partly | `mcp_claim_tools.go:99-125`. `run_id` can be `""` (B4). |
| FR-3.5 | Met | A `ClaimRefusal` is returned as the tool error. `TestClaimTaskToolRefusals`. |
| FR-3.6 | Met | `mcp_tools.go:448-456`, `mcp_claim_tools.go:148-218`. The `claimRefusalFor` read is read-only and is not activity (`claim_check.go:261-277`). |
| FR-3.7 | Met | `mcp.go:320-322`. |

### FR-4: The web UI

| Req | Status | Evidence |
|---|---|---|
| FR-4.1 | Partly | `ui_claims.go:142-190`, `executors.html:30-115`. A returned claim shows no comments (B2). |
| FR-4.2 | Partly | `ui.go` routes, and `ui_claims.go:266-327` with `kind: person`, `via: ui` and `ui_actor`. The handlers render instead of redirecting (B10). |
| FR-4.3 | Met | `ui_claims.go:316-327`. |
| FR-4.4 | Met | `ui_claims.go:219-229`, `executors.html:150`. |

### FR-5: The safety nets

| Req | Status | Evidence |
|---|---|---|
| FR-5.1 | Met | `server.go:270` runs the sweep after `StallSweep`. `claim_sweep.go:50-139`, with the conditional update at `store/claims.go:237-246`. |
| FR-5.2 | Met | `claims.go:223-253`. |
| FR-5.3 | Met | `claims_task.go:311-320`, `claim_sweep.go:145-174`. The question for a kept, returned claim gives the wrong time (B13). |
| FR-5.4 | Met | Withdrawal on every event (`store/claims.go:219-228`), on the sweep (`claim_sweep.go:130`) and on keep. A late answer gets a notice (`ui.go` `handleUIRespond`). |
| FR-5.5 | Met | `config.go:108-114,346-348`. |
| FR-5.6 | Partly | `branch_watch.go:93-192`. The heartbeat runs it (`server.go:271`), and the post-commit hook runs it asynchronously (`http.go:482-486`). A watch can be skipped by a reconcile (B5) or by a claim (B6). |
| FR-5.7 | Partly | Completion, submit and release record their hashes and run the watch first. A model error leaves a hash unrecorded (B8). A claim isn't covered (B6). |
| FR-5.8 | Met | `branch_watch.go:197-271`. |
| FR-5.9 | Partly | `worktree_ops.go:92`, `branch_watch.go:315-334`. The reconcile overwrites the head (B5). |
| FR-5.10 | Met | `ui_views.go:52-57,88-91`, `rules.go:823-828`, `rules_claims.go`. |

### FR-6: The boundary

| Req | Status | Evidence |
|---|---|---|
| FR-6.1 | Met | `integration_mcp_test.go:69-76,175-180`. |
| FR-6.2 | Met | `integration_mcp_test.go:207-214`. |
| FR-6.3 | Met | `claims.go:296-299`. The verify-after-claim test covers one task (`integration_mcp_claims_test.go:350-358`). |
| FR-6.4 | Met in code, partly tested | `onSubmit` always enqueues `review-code`, except in B4's case. The only approval is `approveTaskCode`. There is no negative test (B1). |
| FR-6.5 | Partly | Refusals are tested (`TestClaimRefusals`, `:457`), and so is the exhausted dispatch (`TestClaimOnAnExhaustedDispatchWithdrawsItsCheckpoint`). The races, the send-back while an agent runs, and the sweep after abandonment are untested (B1). B3 is a live race. |

### FR-7: Unmeasured actuals

| Req | Status | Evidence |
|---|---|---|
| FR-7.1 | Met | `unmeasured.go:167-195`. |
| FR-7.2 | Met | The filter comes before `LIMIT` in SQL: `corpus.go:115,123` and `ui_reads.go:177-178`. API: `http_phase3.go:103-113`. CLI: `main.go:651-657`. `TestCorpusAndCalibrationFilterBeforeLimit`. |
| FR-7.3 | Met | `entity.html` rail, `TestUnmeasuredActualReadsAsAtLeast`. |
| FR-7.4 | Met | `sends.go:157-166`, `TestPurposeTokenSamplesSkipsUnmeasuredRound`. |
| FR-7.5 | Met | `http_phase3.go:525-532`, `main.go:568-570`. |

### FR-8: The reviewer for the chat agent's work

| Req | Status | Evidence |
|---|---|---|
| FR-8.1 | Met | `config.go:350-353`. |
| FR-8.2 | Met | `config.go:121-139`: the highest output price, ties broken by name, and a single model is chosen. |
| FR-8.3 | Met | Submit: `claims_task.go:228`. Completion: `actions_phase2.go:285`. Rule: `claims_task.go:425-438`. Documents: `actions.go:289-298` wins over `routing` unless the setting is `same`. |
| FR-8.4 | Partly | `ui_claims.go:105-139`, which can mislabel (B11). The submit result names the model. |
| FR-8.5 | Met | `starter.go`. |

### FR-9: What the chat agent is told

| Req | Status | Evidence |
|---|---|---|
| FR-9.1 | Met | `chat-skills/work-a-task/SKILL.md`, embedded by `starter.go:30`. |
| FR-9.2 | Met | `TestWorkATaskSkillSaysWhatTheRulesSay`. |

### FR-10: The end-to-end proof

| Req | Status | Evidence |
|---|---|---|
| FR-10.1 | Not met | No test (B1). |
| FR-10.2 | Met | `TestPersonClaimsATaskInTheUI` (`integration_ui_claims_test.go:38`). |
| FR-10.3 | Met | `TestChatClaimsABugsTask` (`integration_mcp_claims_test.go:372`). |

### Non-functional requirements

| Req | Status | Evidence |
|---|---|---|
| NFR-1 | Met | MCP and the UI both call `ClaimWork`, `SubmitWork` and `ReleaseClaim`. Each act's changes are one transaction with its audit row. |
| NFR-2 | Met | The tool-set test. |
| NFR-3 | Met | The files changed match the list. The post-commit change is in `http.go`, not `documents.go`. `observe.go` is unchanged. |
| NFR-4 | Partly | One raw machine word in the claim panel (B13). |
| NFR-5 | Partly | Vet is clean and the touched packages are green. FR-10.1 and the race tests are missing (B1). The `-race` result is at the end of this review. |
| NFR-6 | Met | No git runs inside any `WithTx` closure in the diff. The dispatcher's `BranchHead` reads before its transaction (`dispatch.go:315-323`). |

### Scope decisions

| SD | Status | Evidence |
|---|---|---|
| SD-1 | Met | `executions`, and `whoWords` in `execWho`. |
| SD-2 | Met for M13 | `ref_type`/`ref_id`, `feature_id` nullable, `deadline_at`, and the `claim-deadline` sweep (`claim_sweep.go:86-94`). See FR-2.10 for the places that assume a task. |
| SD-3 | Met | `claims_task.go:76-145,183-216`. Stall window: `dispatches.go:597-605`. B7 is the window this misses. |
| SD-4 | Met | Unique index (`0014:40`), the refusal (`claims_task.go:136-143`), and the governor (`dispatches.go:406-430`, `TestGovernorHoldsImplementersWhileAClaimIsOpen`). |
| SD-5 | Met | `Claimant.holds` checks the kind, the actor and `via` (`claims.go:64-66`). There is no MCP release. |
| SD-6 | Partly | The send-back returns to the claim, with no dispatch (`actions_phase2.go:497-503`), and resuming checks the working copy. The task page hides the comments while the claim is returned (B2). |
| SD-7 | Met | `claims_task.go:269-309`, and the commit message at `:283`. |
| SD-8 | Met | The events set activity (`store/claims.go:192-193`), and so do the sweep's working-copy change and keep. Reads aren't activity. |
| SD-9 | Met | One `ClaimExpiry` for open and returned claims, by timestamp, with the fingerprint taken before expiry is judged (`claim_sweep.go:60-85`). |
| SD-10 | Partly | Hashes (`worktree_commits`), `--not main`, rewrites, and `active` and `review` are all handled. Gaps: B5, B6 and B8. |
| SD-11 | Met | One answer, `seen`, which routes to nothing. |
| SD-12 | Met | `resolveClaimRef` accepts only the task shape. |
| SD-13 | Met | `unmeasured.go`. |
| SD-14 | Met | `config.go:121-139`, plus the document review. |
| SD-15 | Met | No quote. The `via` is in the audit, and `task.taken_by_claim` records the cancellations. |
| SD-16 | Met | Feature abandonment ends claims inside `TransitionFeature` (`entities.go:252-259`). Task abandonment: `actions_phase2.go:336`. Approval: `:362`. Each is in the same transaction and withdraws the stale question. The sweep after abandonment is untested (B1). |
| SD-17 | Met | `planner.go:258-268`. |
| SD-18 | Met | The description, the rules and the skill all say it. A commit isn't refused. |

## The judgement boundary

No new MCP path lets the chat agent judge or advance work without a judge:
- **Approve, review or verify:** there is no tool.
  `approveTaskCode` is reached only from a review outcome or a person's
  Inbox answer.
- **Release:** UI only (`ui_claims.go:316`, `claim_sweep.go:201`).
- **Start building:** a claim requires an `active` feature
  (`claims_task.go:92-100`).
- **Answer a checkpoint:** there is no tool. One effect comes close: claiming
  an `active` task withdraws its `dispatch-failure` question. SD-3 specifies
  this, and SD-15 records the reasoning. The renewal withdrawing `claim-stale`
  is activity under SD-8.
- **Skip the code review:** every submit queues `review-code`, except in
  B4's stranding case, which strands the task rather than skipping the
  review.
- **Skip verification:** there is nothing to claim, and G2 and the verifier
  are unchanged.

Outside MCP, local HTTP posts to `/ui/*` act as the person (B9's note). That
predates M13 and belongs with M15's identities.

## Verdict

**Changes needed before merge: B1 and B2.**
- **B1:** merge T7, or write its tests here. FR-10.1 is the milestone's
  definition-of-done claim. The race tests are the only guard on the
  dispatcher's new start.
- **B2:** a one-line fix to an unmet acceptance criterion.

The minor findings should be fixed in the same round where they are cheap:
- B3: return true when the task is claimed;
- B5: start the watch only when there is no head;
- B6: run the watch first in a claim;
- B8: resolve the model before the commit;
- B9: refuse cross-site posts.

Otherwise record B4, B7 and B10 to B13 in the handoff, as known limitations
with owners. The concurrency design is sound as built: I found no
lock-order cycle, no git under a transaction, and no stranded verdict on
the main paths.

## Test result

`go test -race -count=1 ./...` passed for every package, against Postgres
on port 54351. `internal/server` took 205s. This confirms that the tests
which exist pass. It says nothing about the tests B1 finds missing.
