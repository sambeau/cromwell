# REVIEW-020: Consistency review of SPEC-020 (Executors)

**Status:** Complete. The author's disposition of each finding belongs in §6
of this review and in [SPEC-020](../specs/SPEC-020-executors.md) §8. The
approval is Sam's.
**Date:** 2026-10-02
**Reviewer:** an independent Claude subagent, not the spec's author.
**Scope:** [SPEC-020](../specs/SPEC-020-executors.md) (first draft, commit `6ba8cef`), checked against:
- [DEC-007](../decisions/DEC-007-the-judgement-boundary.md), the primary authority;
- [DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1 and its notes, [DEC-005](../decisions/DEC-005-the-orchestration-boundary.md) and [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md);
- [DESIGN-010](../design/DESIGN-010-subutai.md) §5b, §5c, §8, §10 (for generality only) and §17b;
- [SPEC-017](../specs/SPEC-017-chat-as-a-proper-seat.md) (`document_writers`, `whoWords`) and [SPEC-019](../specs/SPEC-019-bugs.md) (a bug is a feature row);
- [How M13 and M14 are built](../notes/orchestration-M13-M14.md) (the integration check);
- the [writing guide](../research/writing-guide.md);
- the code at `6ba8cef`:
  - `internal/server`: `actions.go`, `actions_phase2.go`, `planner.go`, `server.go`, `documents.go`, `http.go`, `http_phase3.go`, `worktree_ops.go`, `ui_entity_actions.go`, `bugs.go`, `provenance.go`, `mcp.go`, `mcp_tools.go`, `ui_views.go` and `integration_mcp_test.go`;
  - `internal/store`: `dispatches.go`, `tasks.go`, `checkpoints.go`, `corpus.go`, `sends.go`, `ui_reads.go`, and migrations `0001`, `0002`, `0004` and `0011`;
  - `internal/dispatch/dispatch.go`, `internal/rules/rules.go` and `rules_phase2.go`, `internal/lifecycle/task.go`, `feature.go` and `devplan.go`, `internal/config/config.go`, `internal/starter/starter.go` and `cmd/subutai/main.go`.

## 1. What this review is

This is a consistency pass on the spec by a reviewer who did not write it. It checks:
- that SPEC-020 agrees with the binding decisions and the design;
- that what it says about the code is true;
- that its requirements can be built as written, without races or gaps in the state machine;
- that the claimable interface serves M14 without spike code;
- that the sixteen choices are the right ones and honestly presented.

It does not fix the spec.

**Overall.** The spec is careful and well argued. Its main shape is right: a claim is a hold on the feature's one working copy; submitting leads into the same dispatched code review; verification is never claimable; the chat agent gains exactly two tools. Most of its claims about the code are true. I checked `dispatchReadyTasks` moving ready to active at enqueue, `MutatingDispatchActiveForTask` checking only running dispatches, `MarkDispatchRunning` changing only a queued row, `returnTaskCode` auditing `task.review_comments` that `planImplement` never reads, the heartbeat order, one pending checkpoint per kind and ref, and the strict config loader.

The weak points are where a claim meets machinery the spec doesn't mention:
- feature abandonment, which transitions no tasks;
- failed and cancelled implement dispatches, and their checkpoint;
- the dispatcher paths that aren't transactional;
- a send-back arriving when the working copy is busy;
- the branch watch's reliance on the commit author.

Two choices aren't presented honestly enough. One is the empty default for the chat reviewer model, which departs from DEC-007's "default to a stronger model". The other is the gap in unmeasured exclusion for chat-written specs and plans. The claimable interface would not let M14 meet its own integration check without changing M13's code.

## 2. Material findings (must fix before build)

### R20-1 — Abandoning a feature transitions no tasks, so SD-16 has nothing to attach to, and an open claim outlives its feature

**Severity:** material. **Where:** SD-16, FR-2.2 (`abandon`), FR-5.1.

**What the spec says.** "A feature's abandonment [ends] the task's claim (`ended`, with the reason), in the same transaction as the task's transition."

**The problem.** Abandoning a feature doesn't transition its tasks at all. There are three call sites, and each does one `TransitionFeature(…, FeatAbandon, …)` and nothing else:
- the feature page (`ui_entity_actions.go:308`);
- `POST /api/features/abandon` (`http.go:288`);
- a bug's triage rejection (`bugs.go:469`).

The only task abandon in the code is `abandonTask`, reached only from the review-deadlock answer (`actions_phase2.go:289` is the sole caller of `TaskAbandon`). So, as written:
- an open claim on an abandoned feature stays `open`;
- the sweep goes on asking "Still working on this?" about dead work;
- `GCWorktrees` removes the worktree with `--force` under the claimant (`worktree_ops.go:113-125`), and the claim's next submit fails on a missing path.

"A person's abandon" and "the review deadlock's abandon" in SD-16 are also the same route.

**Suggested fix.** End every non-ended claim on the feature inside `store.TransitionFeature` when the event is `FeatAbandon`, so all three sites are covered by one change. Alternatively, name the three sites and test each one. Withdraw the claim's `claim-stale` checkpoint in the same transaction. Correct SD-16's list of routes.

### R20-2 — A send-back reopens a claim even when the working copy is busy, which breaks SD-4 or fails the verdict's transaction

**Severity:** material. **Where:** SD-4, SD-6, FR-2.1 (the one-open-claim-per-feature index), FR-2.8.

**The problem.** SD-4 says a `submitted` claim doesn't hold the copy, so while T01's claim is in review:
- an implementer may be *running* on T02; or
- a person may hold an `open` claim on T02.

If T01's reviewer then asks for changes, FR-2.8 puts T01's claim straight back to `open`:
- **In the first case,** two hands are in the copy: the claimant and a running agent, whose completion runs `git add -A` over the claimant's edits (`actions_phase2.go:595-605`).
- **In the second case,** the partial unique index (one `open` claim per `feature_id`) refuses the update. `returnTaskCode`'s transaction then fails, so the reviewer's verdict isn't applied, and the outcome action errors.

**Suggested fix.** Add a claim state, for example `returned`. A claim reopened by a send-back is `returned` (it holds no copy) and becomes `open` only when it can be: no running implement dispatch on the feature and no other open claim. The claimant's next `claim_task` (or the page's *Claim*) moves it to `open`, or refuses with "An agent is implementing T02 in this working copy; claim T01 again when it finishes." Add `returned` to the FR-2.2 machine, the FR-1.7 moments and the FR-6.5 tests. Alternatively, accept a send-back reopening only when the copy is free, and otherwise leave the claim `submitted` with a pending reopen. Either way, say it.

### R20-3 — FR-2.7's lock doesn't close the races, because the dispatcher's check and start aren't in one transaction, and other paths move tasks and dispatches without a lock

**Severity:** material. **Where:** FR-2.6, FR-2.7, SD-3, NFR-6.

**The problems.**
1. **`admit` isn't a transaction.** The feature-serialisation check runs on the pool (`dispatch.go:282`), and `MarkDispatchRunning` runs later in its own transaction (`dispatch.go:305-308`). "Admitting locks the feature row before checking" has nowhere to live. Consider a claim on T01 and the start of T02's queued dispatch:
   - the governor sees no claim;
   - the claim transaction sees no running dispatch and commits;
   - `MarkDispatchRunning` commits.

   Both win. The queued-row guard protects only the *same* task's dispatch.
2. **`dispatchReadyTasks` takes no lock and `TransitionTask` has no state guard.** `dispatchReadyTasks` reads the task without `FOR UPDATE` (`actions_phase2.go:175`). `TransitionTask` writes `UPDATE tasks SET state = $2 WHERE id = $1` with no `AND state = …` (`tasks.go:128`). Suppose a claim moves a ready task to `active` between that read and that write. The dispatcher's write still succeeds (active to active), and it enqueues an implement dispatch for the claimed task (`actions_phase2.go:185-188`). The governor holds it while the claim is open. Once the claim is submitted, the governor admits it: an implementer runs on a task in `review`, `commitWorktree` commits its work into the branch, and only then does `TaskImplemented` fail as illegal.
3. **Requeues ignore claims.** The retry sweep (`dispatch.go:566-570`) and the dispatch-failure *Retry* answer (`actions.go:212-218`) both call `RequeueDispatch`, which revives a `failed` row with no knowledge of claims (`dispatches.go:248-259`). See R20-4.
4. **Lock order isn't stated.** "In that order" is ambiguous. If one path locks the dispatch row and then the feature, while another locks the feature and then the dispatch row, they deadlock.

**Suggested fix.**
- Re-run the serialisation check, with the claim check, *inside* the `MarkDispatchRunning` transaction, after `SELECT … FROM features … FOR UPDATE`. Make that a new store function rather than relying on `admit`'s pool read.
- Make `dispatchReadyTasks` lock the feature, re-read the task `FOR UPDATE`, and skip any task that isn't `ready` or that has a non-ended claim.
- Add a state guard to `TransitionTask`, as `MarkDispatchRunning` has, so a lost race refuses rather than overwrites.
- Make `RequeueDispatch` refuse an implement dispatch whose task has a non-ended claim, and make the governor refuse to start an implement dispatch whose task isn't `active` or is claimed.
- State one lock order: the feature, then the task, then the dispatch rows.
- Add tests for each interleaving to FR-6.5.

### R20-4 — SD-3 says nothing about failed, retrying, exhausted or cancelled implement dispatches, and the dispatch-failure checkpoint can revive an agent under a claim

**Severity:** material. **Where:** SD-3, FR-2.3 step 2, FR-2.4.

**The problem.** An implement dispatch can be:
- `failed` and waiting out its back-off, which the retry sweep requeues on the same row (`dispatches.go:338-360`);
- `failed` and exhausted, with a pending `dispatch-failure` checkpoint whose *Retry* requeues it and whose *Cancel* marks it `cancelled` (`rules.go:808-820`, `dispatches.go:366-371`);
- stalled, where the stall sweep marks the row `failed` while the attempt's goroutine may still be running tools in the worktree (`dispatch.go:332-337`, `487-493`, `ErrAttemptGivenUp`).

SD-3's second case ("its only implement dispatch that isn't finished is still queued") doesn't say whether these count as finished. Each reading is wrong:
- **If they count as finished,** a claim is allowed, and the retry sweep or a *Retry* answer later queues an implementer for the claimed task. R20-3 point 2 then follows.
- **If they don't,** the most useful claim is refused: "the agent failed three times, let me do it in chat". A task whose failed dispatch was *cancelled* is stuck in `active` for ever today, and a claim is its natural way out.

FR-1.2 also says two rows in a round happen when "a claim takes over a queued dispatch whose earlier attempt never ran". That is backwards: an attempt that never ran has no `agent` row. It is a requeued dispatch whose earlier attempt *did* run that produces a second row.

**Suggested fix.**
- Make a task claimable when it is `active`, has no claim, and has no running implement dispatch. Exclude a stalled attempt whose goroutine is still alive.
- The claim cancels every implement dispatch of the task that is `queued` or `failed`, in its transaction.
- The claim withdraws the task's pending `dispatch-failure` checkpoint, with an audit row saying a person or the chat agent took the task.
- Give the refusal for a running attempt a sentence.
- Correct FR-1.2's sentence.
- Add a test: a task whose dispatch is exhausted is claimed; a later *Retry* answer, or the retry sweep, starts nothing.

### R20-5 — The execution record's uniqueness rule refuses an ordinary flow: release, then claim again in the same round

**Severity:** material. **Where:** FR-1.1 (the `(task_id, round)` constraint), FR-1.2, FR-1.3, SD-5, SD-7.

**The problem.** A claim released by a person queues an implement dispatch (SD-7). While that dispatch is still queued, anyone may claim the task again (SD-3). The task has had no send-back, so the new claim is in the same round, and `(task_id, round)` unique for claims refuses the insert. The same thing happens when a person releases a chat agent's claim and then claims the task in the UI, which is SD-5's own remedy for a chat session that disappeared.

FR-1.3's backfill also keys on `started_at`. `RequeueDispatch` resets `started_at` to NULL (`dispatches.go:251-252`), so an implement dispatch that ran, failed and now sits queued, or one cancelled after failing, may be missed or dated by its last attempt.

**Suggested fix.**
- Make claim rows unique on `claim_id`, and agent rows on `dispatch_id`. Let a round hold any number of rows.
- Take the round's executor for display as its latest row.
- Backfill from the `dispatch.running` audit rows, which record each start, rather than from `started_at`.

### R20-6 — A claim that takes over an agent's queued rework never sees the reviewer's comments

**Severity:** material. **Where:** FR-3.2 (`review_comments`: "on a reopened claim"), FR-4.1, SD-7, §7.

**The problem.** After a code reviewer sends an *agent's* task back, `returnTaskCode` queues a rework dispatch (`actions_phase2.go:437`), and SD-3's second case lets a claim take it over. That claim isn't "reopened", so FR-3.2 gives it no `review_comments`, and FR-4.1 shows them only "if it came back". The chat agent then reworks blind, which is the defect §7 reports for agents and FR-3.4's rationale exists to avoid. The reverse also holds: a person releasing a claim in round 2 or later hands blind rework to an agent until the §7 bug is fixed, and SD-7 doesn't say so.

**Suggested fix.** Return the latest code review's comments whenever the claimed round is greater than 1, whoever implemented the earlier round, and show them on the task page on the same rule. State SD-7's cost until the §7 bug is fixed.

### R20-7 — The task's base commit is stale for a claim that takes an `active` task, which defeats FR-2.5's "nothing changed" check and widens the review's diff

**Severity:** material. **Where:** FR-2.3 step 2, FR-2.5 step 2, SD-4; §7.

**The problem.** The base commit is set only when `dispatchReadyTasks` moves a ready task to `active` (`actions_phase2.go:167-182`). Every task made ready together gets the same head, and the governor runs them one at a time. So by the time a claim takes T02's queued dispatch, T01's work may already be committed after T02's base.

The code review diffs `base..HEAD` at review time (`planner.go:293`, `453-464`). As a result:
- FR-2.5's refusal "nothing has changed since … the task's base commit" passes without the claimant changing anything;
- the claimed task's review is shown T01's diff as well.

On a rework round the check is meaningless anyway, because round 1's commit is already past the base.

The same staleness affects dispatched implementers today, so it is also a bug-report candidate. Every task but the first in a batch is reviewed against a diff that includes its siblings'.

**Suggested fix.**
- When a claim starts round 1 of a task whose base is behind the branch head, reset the base to the head, before the transaction (NFR-6) and re-checked inside it.
- Record the round's starting head on the execution row, and have FR-2.5 step 2 compare against that.
- Add the batch base-commit defect to §7.

### R20-8 — The branch watch identifies Subutai by commit author, reads `rev-list` naively and watches only `active` features, so it misses the risky cases and flags harmless ones

**Severity:** material. **Where:** SD-10, FR-5.6, FR-5.7, FR-5.8.

**The problems.**
1. **The author is the wrong signal.** `commitWorktree` sets `--author "subutai <subutai@localhost>"` (`actions_phase2.go:601`), but the author survives `git commit --amend`, `rebase` and `cherry-pick` by a person. So a person can rewrite approved, reviewed code under Subutai's name, and anyone can pass `--author`. These are false negatives.
2. **History removed or rewritten isn't seen.** `git rev-list <watched_head>..<branch>` lists nothing after a `git reset --hard HEAD~3` that deletes reviewed work. A force-moved branch is never checked for still descending from `watched_head`.
3. **Merging `main` into the branch is a false positive.** A person who merges or rebases onto `main` to keep the branch current brings in every human commit made on `main` since. `rev-list` lists them all as "commits on FEAT-023's branch".
4. **Features in `review` aren't watched.** A commit made while the verifier runs, such as "just a quick fix", goes unflagged, and `mergeFeature` then merges it into `main`. This is the shortest path for unreviewed code to reach the main line. And when verification returns the feature to `active`, `watched_head` is stale, so commits made during review are reported late and as if new.
5. **The fallback is wrong.** "After the branch's first commit, for a row with none" walks back to the repository's root commit, which would report all of `main`'s history. `StartFeature` also inserts the worktree row *before* `git worktree add` creates the branch (`worktree_ops.go:60-74`), so FR-5.8 can't set `watched_head` "when a worktree is created" without a second write. The worktree-failure retry path needs one as well.
6. **An open claim absorbs everything.** Any commit made while any claim on the feature is open counts as that claim's activity, whoever made it. A chat agent committing for T05 while a person holds T03 is never flagged.
7. **SD-10's wording is wrong.** The branch is `subutai/<feature path>`, not `subutai/<worktree path>` (`worktree_ops.go:57`).

**Suggested fix.**
- Identify Subutai's commits by the hashes Subutai makes, setting `watched_head` (or recording the hash) under the feature lock right after each server commit. Keep the author only as a secondary check, and add the committer.
- Check `git merge-base --is-ancestor <watched_head> <branch>`. If it fails, raise the checkpoint as "the branch's history was rewritten".
- List with `git rev-list <watched_head>..<branch> --not <main>`, so commits already on `main` are excluded.
- Watch features in `active` *and* `review`.
- Set `watched_head` after `addWorktree` succeeds, and when it is null, treat the fork point (`git merge-base main <branch>`) as the start.
- Attribute a commit to an open claim only when its committer is the claim's (or note in SD-10 that it can't tell).
- List the false positives and negatives that remain in SD-10, so Sam's choice 10 is informed.

### R20-9 — Unmeasured exclusion misses chat-written and person-written specs and plans, and the corpus queries filter after their limit

**Severity:** material. **Where:** SD-13, FR-7.1, FR-7.2, FR-7.4; DEC-007 decision 8.

**The problems.**
1. **Chat-written contract documents are missing.** DEC-007 decision 8 marks *chat work* as unmeasured, and FR-7.1 considers only task executions. But SPEC-017's lane, which SD-12 recommends in place of claiming, lets the chat agent write a feature's spec or plan. A feature's actuals include its feature-ref dispatches: `write-spec`, `write-dev-plan` and the reviews (`corpus.go:36-43`). So a feature whose spec and plan were written in chat enters the estimator's corpus with no authoring cost, which reads as cheap rather than unknown. That is the exact error FR-7 exists to prevent. The same holds for a person's attached or adopted documents.
2. **The queries limit before they filter.** `RetrieveCorpus` and `RecentCalibration` apply `LIMIT` in SQL and compute actuals afterwards (`corpus.go:84-136`, `ui_reads.go:156-200`). Leaving unmeasured rows out "afterwards" returns fewer than five neighbours, sometimes none.
3. **FR-7.4's guard is ambiguous** when a round has two rows (R20-5). An agent round that began from a released claim's half-done work is a biased low sample. It should be left out, and "that task's execution in the dispatch's round" doesn't say so.
4. **The estimate API change is misnamed.** The API's field is `actual_tokens`, not `actual`, and it also returns `delta` (`http_phase3.go:57-58`, `99-104`). The CLI prints both (`cmd/subutai/main.go:645-646`). FR-7.2 changes neither.

**Suggested fix.**
- Define a feature as unmeasured also when the latest writing act on its current spec or plan is `chat` or `person` (from `document_writers`, SPEC-017), and say so in SD-13.
- Filter in SQL (`NOT EXISTS` an unmeasured execution or writer) before the `LIMIT`.
- Define FR-7.4 as "every execution in the dispatch's round is measured".
- Set `actual_tokens` to null and `delta` to null, add `unmeasured` and `measured_part`, and update the CLI.

### R20-10 — The claimable interface doesn't let M14 meet its own integration check without changing M13's code

**Severity:** material. **Where:** SD-2, FR-2.1, FR-2.9, FR-2.10, FR-5.3; DESIGN-010 §10; the orchestration note's integration check.

**The problems.** The orchestration note says that when M14 merges, it checks three things:
- a spike in chat is held by M13's claim expiry;
- a spike's executor is recorded;
- a chat-run spike's tokens are *unmeasured*.

The interface serves none of them as written.
1. **A time box isn't an inactivity expiry.** DESIGN-010 §10 says a chat or human spike "carries a time box … enforced through claim expiry". An actively worked spike never goes 24 hours without a change, so it is never stopped. `work_claims` has no deadline, and the sweep, which belongs to the claim service, has no hook for one.
2. **The executor record and the unmeasured marker are task-only.** `task_executions.task_id` covers tasks alone, and the marker lives there (FR-7.1).
3. **The checkpoint is task-shaped.** `claim-stale` is "on the task", and its *Release it to an agent* answer means "queue an implementer". For a spike, release means something else. The interface has no `onExpire` or question hook.
4. **Git work can't happen in `onSubmit(ctx, tx, …)`.** NFR-6 forbids git under a transaction, but the task's submit commits, and the interface gives it only a transactional hook. There is no "prepare outside the transaction" step.
5. **`feature_id` is undefined for a spike.** A spike with its own throwaway worktree must leave `feature_id` null, or it would block the feature's claims and hold the feature's implementers through the per-feature index. The spec doesn't say so.
6. **The `ref_type` enum is too narrow.** Audit rows and checkpoints use the `ref_type` enum (`0001_init.sql:11`), which has no `spike` and no `claim`. FR-2.10's `claim.activity` and `claim.ended` need a `ref_type` that exists. "M14 needs no change to `0014`" is true only of `work_claims`.

**Suggested fix.**
- Add a nullable `deadline_at` to `work_claims` in `0014`, and have the sweep raise a distinct, interface-supplied checkpoint when it passes.
- Split `onSubmit` into `prepareSubmit` (outside the transaction, git allowed) and `onSubmit` (inside it).
- Add `staleQuestion` and `onExpire` (or `releaseAnswer`) to the interface.
- Say that `feature_id` is "the feature whose working copy the claim holds", null for a spike.
- Write `claim.*` audit rows against the claimed item's `ref_type`, and say M14 adds `spike` to the enum in `0015`.
- Either generalise the execution record (`ref_type`, `ref_id`), or say plainly that M14 adds its own and FR-7 reads both.

### R20-11 — SD-14's empty default departs from DEC-007 decision 10 and DESIGN-010 §5b, and the choice doesn't say so

**Severity:** material, because the choice must be presented honestly. **Where:** SD-14, FR-8.1, DoD 8 (choice 14), DoD 9.

**The problem.** DEC-007 decision 10 is titled "Reviews **default** to a stronger model when the chat AI did the work". DESIGN-010 §5b says: "**By default** that review uses a stronger model". The decision's added detail makes it a per-project setting, but a setting that is empty by default means that, by default, the review does *not* use a stronger model. SD-14's reason ("Subutai can't know which model is stronger") is fair. It is still a departure, and choice 14 presents it as a setting, not as a reversal of the stated default. DoD 9 schedules no note on §5b or decision 10 for it.

SD-14 also omits a natural alternative. Decision 10's rationale ("a feature done entirely in chat has had one mind through the whole chain") argues at least as strongly for the *verifier* of a feature whose tasks were mostly chat-implemented. Neither SD-14 nor its alternatives mention verification.

**Suggested fix.**
- Say in SD-14 that the empty default departs from decision 10's "default to" and §5b's "by default". Offer the alternative of defaulting to the configured verifier's model, or to the most expensive configured model, which needs no judgement of strength.
- List verification of a chat-heavy feature as an alternative scope.
- Add DoD 9 notes on DEC-007 decision 10 and DESIGN-010 §5b.

## 3. Minor findings (should fix)

- **R20-12 — Reading comments by renewing contradicts SD-8.** *Where:* SD-8, FR-3.2, FR-3.4. SD-8 says reading isn't activity, "or a chat session that polls would … keep a dead claim alive". But `claim_task` is a renewal *and* the only way to read the comments, and FR-3.4 tells the agent to "call claim_task … later to see whether it came back". In review, that call is an `isError` refusal (FR-2.4), so the agent is told to poll with a call that errors. *Fix:* put `review_comments` and the claim's state in `get_feature`'s task entry (FR-3.6), which is a read. Make FR-3.4's `next` point there. Keep `claim_task`'s renewal for an intentional "I'm still on it".

- **R20-13 — Concurrent git in one worktree is unordered.** *Where:* NFR-6, FR-2.3, FR-2.5, FR-4.3, FR-5.1, FR-5.6. With git outside transactions, a submit, a release (from the page or a checkpoint answer), the sweep's fingerprint and a post-commit-triggered watch can all run git in the same worktree at once:
  - `index.lock` collisions are possible, including with the claimant's own `git commit`;
  - a submit can commit and then be refused because a release won, leaving a commit titled as a submission on an active task.

  FR-2.3 step 3 also computes `worktree_seen` inside the locked transaction, which NFR-6 forbids. *Fix:*
  - serialise git work per worktree with an in-process mutex or a session-level advisory lock held across "git, then transaction";
  - use `git --no-optional-locks status` for the fingerprint;
  - compute the fingerprint before the transaction;
  - make sweep updates conditional on `state = 'open'`;
  - run the branch watch from the post-commit handler asynchronously, so a server commit never waits on its own hook.

- **R20-14 — A spec revision in flight, and its *pause* answer, are undefined for an open claim.** *Where:* SD-3, FR-2.5. SD-3 refuses new claims while `spec_stale`, but says nothing about an open claim: can it submit, and is the claimant told its contract changed? The *pause* answer clears `spec_stale` exactly as *continue* does, without dispatching (`actions_phase2.go:572-584`; the comment at `rules.go:666` says pause "leaves the feature blocked"). After a pause, ready tasks are claimable, so the chat agent can resume work a person paused. A feature whose worktree failed to create (`worktree-failure`) is `active` with no working copy, and a claim would return a missing path. *Fix:*
  - allow submit during `spec_stale`, as an agent's completion is allowed, with a notice in the result;
  - refuse claims while a revision-in-flight checkpoint answered *pause* stands, or note the existing weakness as a bug candidate;
  - refuse claims when the live worktree's directory doesn't exist.

- **R20-15 — The record of the boundary isn't updated.** *Where:* Authority, FR-6.1, DoD 9.
  - DEC-004's "may not … drive any development-side lifecycle transition" is still in force. DEC-005 superseded only its third bullet, and DEC-007 superseded DEC-005's first. `claim_task` and `submit_task` move tasks through ready, active and review.
  - DEC-005's "construct or direct a dispatch" is touched by a claim that *cancels* a queued dispatch.
  - The tool-set test's header comment says the facet has "no tool that … transitions development lifecycle" (`integration_mcp_test.go:69-74`).
  - SPEC-017 says M13 "builds on [`document_writers`] for tasks rather than adding a second record". `task_executions` is a second table, and SD-1 should answer that sentence directly.

  *Fix:* cite DEC-004 in Authority, add DoD 9 notes on DEC-004 and DEC-005, update the test's header comment, and address SPEC-017's sentence in SD-1.

- **R20-16 — Round counting needs three more sentences.** *Where:* FR-1.1, FR-1.2, FR-1.4.
  - Say the count of `request_changes` transitions is read *after* the send-back's transition, in the same transaction.
  - Say how it relates to the round cap's count, which is the number of `review-code` dispatches (`actions.go:72`), so nobody "fixes" one to match the other.
  - Say which row "the round's row" is when a round has several: the one with the dispatch or claim being submitted.
  - Say whether `executorLine` mentions a released claim's partial work. Today "Implemented by the implementer" hides the person's half.

- **R20-17 — Some claims about the code and the documents are wrong.**
  - The task's `claim` event is fired by `dispatchReadyTasks`, in the server, not by "the dispatcher".
  - The header says "§17b (the three questions left to M13)", but DoD 9 says "the two questions settled".
  - NFR-3 and the coordination note understate the shared files. Beyond `mcp.go`, the test and `entity.html`, the spec changes:
    - `dispatch.go` (the governor) and `store/dispatches.go`;
    - `rules.go` (the checkpoint answers) and `ui_views.go` (`responseFor`);
    - `config.go` and the starter pack;
    - `server.go` (the heartbeat), `documents.go` and `http.go` (post-commit), and `worktree_ops.go`;
    - `corpus.go`, `sends.go`, `ui_reads.go` and `http_phase3.go`;
    - `mcp_tools.go` and `mcp.go`'s `get_feature` description;
    - the CLI.

    M14 touches several of these too.

- **R20-18 — Claim-stale checkpoint details.** *Where:* FR-5.1, FR-5.3, FR-5.4, FR-5.5, DoD 3.
  - Withdraw the checkpoint on *any* activity (a renewal, a send-back), not only a change the sweep sees.
  - Make an answer that arrives after the claim was submitted or ended a no-op with a notice, not an error.
  - Say what *Release* does when the claimant came back after the question was asked. At least show the latest activity in the Inbox.
  - `expiry_hours: 0` and an absent key are indistinguishable with a plain `int` (the loader defaults zero, as at `config.go:340-341`), so "refuses anything else" needs a pointer.
  - Say whether downtime counts towards expiry.
  - The one-hour floor means DoD 3's demo and the tests need an injectable clock or a backdated `last_activity_at`. Say which.

- **R20-19 — Costs of the executor's identity aren't stated.** *Where:* SD-5, SD-7, FR-9.1, FR-2.5 step 3.
  - Every chat session shares one MCP actor, so any chat window can submit or renew any chat claim.
  - When a person releases a chat claim, the chat agent isn't told, and may go on editing beside the newly queued implementer.
  - A chat session that disappears after finishing leaves the person only release-then-claim, which hits R20-5.
  - FR-9.1's "commit if you like" puts work-in-progress commits on the branch, where a concurrently running code review of another task diffs `base..HEAD` and sees them.
  - Submitted human work is git-authored by Subutai. That is deliberate, for the watch, but it is a choice about git history that SD-10 should name.

  *Fix:* state each as a known cost in the relevant SD, and prefer "don't commit; Subutai commits on submit" in the skill.

- **R20-20 — Prose.** Per DESIGN-008 D-6 and the writing guide:
  - "FEAT-023-T03 is done." says nothing about what to do instead, as FR-2.4 requires.
  - The badge "Claimed" (FR-1.6) and FR-7.3's "Unmeasured — at least 12,400 tokens, plus work done in chat or by a person" aren't sentences.
  - FR-5.7 uses curly quotes (the guide asks for straight ones) and opens a sentence with a numeral ("2 commits were made").
  - FR-5.7's "though the verifier will see it" overclaims. The verifier is given acceptance criteria only, by design (`planner.go:322`, L-4), and doesn't review diffs. "The verifier still checks the feature against its spec" is accurate.
  - One word, one meaning: the task event `claim` is fired for every dispatched implementer, while §"A note on prose" says "a dispatched agent never has a claim". Note the collision, or rename the new event family.
  - `claimable` in FR-3.6 is "true, or a sentence", a mixed JSON type. Use `claimable: bool` and `why_not: string`.
  - British spelling is consistent throughout. `initialize` is the protocol's name.

- **R20-21 — Tests.** *Where:* FR-6.5, FR-10.
  - Add the interleavings from R20-2, R20-3 and R20-4 to FR-6.5.
  - FR-10.1 step 2 needs the dispatcher held, or the queued T01 dispatch may start before the claim. Say whether T02 depends on T01, since "waits behind the claim" holds only if it is independent.
  - Give the feature an estimate, or step 8's corpus assertion is vacuous: `RetrieveCorpus` requires one (`corpus.go:97`, `104`).
  - Claim a bug's task (`BUG-007-T01`) once, so SPEC-019's IDs are exercised.

## 4. Checks that passed

- **The judgement boundary.** No tool lets the chat agent approve, review, verify, release, start building or answer a checkpoint. Verification has no claimable form (FR-6.3). Every submit queues a dispatched `review-code` (FR-6.4). The must-not-exist list is sensible. DEC-006 Amendment 1 holds: claiming spends nothing a person committed to, and is recoverable by release (SD-15).
- **The code claims in §0 and §2.** `dispatchReadyTasks` moves ready to active as it enqueues (`actions_phase2.go:152-197`). The governor serialises on running implement dispatches only (`dispatches.go:398-411`). `MarkDispatchRunning` changes only a queued row (`dispatches.go:106-118`). `returnTaskCode` audits comments as `task.review_comments` and `planImplement` never reads them (`actions_phase2.go:415-443`, `planner.go:218-264`). The heartbeat order is as described (`server.go:257-264`). Checkpoints are idempotent per kind and ref while pending (`0001_init.sql:188`), and `withdrawn` exists in the enum. The loader is strict and validates `routing` against `models` (`config.go:326-330`). `queueReview` applies `routing` over the role's model (`actions.go:276-280`). Linked worktrees share the main repository's hooks by default.
- **SD-3's reasoning** that almost nothing stays `ready` is right.
- **A claimed task is safe from re-decomposition.** It is `active`, and `PlanDecomposition` deletes only pending or ready tasks (`devplan.go:260`).
- **The review-deadlock and escalation paths** reach `approveTaskCode`, `returnTaskCode` or `abandonTask`, so FR-2.8 covers them once R20-1 and R20-2 are fixed.
- **Verification returning a feature** creates new `V` tasks and dispatches them through `dispatchReadyTasks`, so they are claimable under SD-3's second case, as intended.
- **SD-12, SD-13 and SD-15** are well argued. SD-9 matches DEC-007 decision 6's default.

## 5. The choices

- **Right as framed:** SD-1 (given R20-15's sentence about SPEC-017), SD-2 (given R20-10), SD-5, SD-9, SD-12, SD-13, SD-15 and SD-16 (given R20-1).
- **SD-3** needs R20-4's case added before Sam can answer it.
- **SD-4** is right, but its cost should include R20-2.
- **SD-6** is incomplete without R20-2's `returned` state.
- **SD-7** should state R20-6's cost: blind agent rework after a release in a later round.
- **SD-8** conflicts with FR-3.4 (R20-12).
- **SD-10** should list the remaining false positives and negatives, and the `review` state (R20-8).
- **SD-11** is reasonable.
- **SD-14** isn't honestly presented (R20-11).

**Decisions taken without saying so:**
- whether a task with a failed or cancelled dispatch can be claimed (R20-4);
- whether commits made while a feature is verified are watched (R20-8);
- that any commit made under any open claim is that claim's (R20-8);
- whether chat-written specs make a feature unmeasured (R20-9);
- whether an open claim may submit during a spec revision (R20-14);
- that human work is git-authored by Subutai (R20-19);
- the departures from DEC-004 and DEC-005's wording (R20-15).

**Summary:** 21 findings, 11 material (R20-1 to R20-11) and 10 minor (R20-12 to R20-21).

## 6. Disposition

*Added by the author, 2026-10-02.* All twenty-one findings were accepted and
dealt with in the revised spec. [SPEC-020](../specs/SPEC-020-executors.md) §8
maps each finding to the change it made. Three choices changed as a result:
SD-6 gained the `returned` state, SD-14's default now follows DEC-007
decision 10 (the most expensive configured model, with `same` to opt out),
and two new choices were added: SD-17 (the implementer is shown its
reviewer's comments, fixing the defect this review confirmed) and SD-18 (the
chat agent is told not to commit). The two defects the review found in
existing code (a batch's stale base commit; the revision-in-flight *pause*)
are listed in SPEC-020 §7 as bug-report candidates.

## Verdict

**Not ready to build as written.** The design is sound and the boundary is right, but the claim's state machine has five holes that a build would hit:
- feature abandonment (R20-1);
- a send-back into a busy copy (R20-2);
- races with the dispatcher (R20-3);
- failed dispatches (R20-4);
- the record's uniqueness rule (R20-5).

The branch watch needs a sounder way to tell Subutai's commits apart, and must cover features being verified (R20-8). Unmeasured exclusion must cover chat-written contracts (R20-9). The spike interface needs a deadline and a split submit (R20-10). The chat reviewer's default must be presented as the departure it is (R20-11). Each fix is local, and none changes the spec's shape. With them made and choice 14 restated, the spec should be ready for Sam.
