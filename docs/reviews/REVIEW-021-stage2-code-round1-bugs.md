# REVIEW-021, stage 2 code, round 1: bugs and spec conformance

Date: 2026-10-02

## Scope

The stage 2 code of SPEC-021 §4 (M14), `git diff 36b7fed..654d8b4 -- . ':!docs'`, on branch `claude/subutai-m14-stage2-spikes`. It was read against SPEC-021 §4 (SD-17 to SD-27, FR-11 to FR-18 and the stage 2 NFRs), stage 1 (§3), SPEC-020, the plan `docs/notes/dev-plan-M14-stage2.md`, DEC-006 Amendment 1 and DEC-007.

`go vet ./...` is clean. `go test -race -count=1 -run 'Spike|Claim' ./internal/server/` passes, as do the store, lifecycle, config and timeline packages. The findings marked "reproduced" were shown with throwaway tests in a scratch copy of the repository; none were left in the tree.

Severity: **major** is wrong behaviour, a race that can corrupt state or deadlock, a spec requirement not met, or a security hole. **minor** is everything else worth fixing.

## Findings

### S2B1 (major): a claim that waited on the ending's lock remakes the ended spike's worktree, and raises a false "couldn't check" question

`internal/server/spikes_claims.go:397-420` (`ClaimSpike`), with `internal/server/spikes_end.go:379-406` (`makeSpikeWorktree`).

`ClaimSpike` reads the spike, judges `spikeStateRefusal` on that read, and only then takes the worktree's lock and calls `ensureSpikeWorktree(ctx, sp)` with the same, unlocked read. Nothing is looked at again under the lock. If an ending (the heartbeat's or the sweep's time box, or a submit) holds the lock when the claim arrives, the claim waits; when it gets the lock, the ending has discarded the worktree and set `worktree_removed_at`, but the claim still holds its old "running" copy of the row. `makeSpikeWorktree` finds no `.git`, and because `spikeHadWorkingCopy` is true for every started chat spike, it runs `spikeKept`. The ending's own check found nothing, so no `spike.code_kept` record exists; this check now finds neither the directory nor git's record of it, records "the working copy was removed, so git's record of it is gone", and raises a `spike-code-kept` checkpoint. It then runs `git worktree add --detach` and makes a fresh worktree for the ended spike. Only after all that does `claimLocked` refuse with "SPK-001 has ended."

Failure scenario (reproduced): a chat spike past its deadline; the ending holds `withSpikeEndLocks`; the chat agent calls `claim_spike` (a renewal, say, a moment before the time box ends). The claim is refused, but the Inbox now has "Subutai couldn't check whether code from SPK-001 was kept outside its working copy" on a spike whose check had passed, and `.subutai/worktrees/spk-…` exists again until the next heartbeat's leftover sweep removes it. The same happens to a person's second click on **I'll run this spike** while their own submit is ending the spike. SD-27 says the locks exist "so a discard can't interleave with a remake"; they stop the interleaving, but not a remake straight after the discard.

Fix: inside the `withWorkingCopy` callback, re-read the spike (`store.GetSpike`) before anything is made, and refuse there with `spikeStateRefusal` and `spikeDeadlineRefusal` (and skip the make when `WorktreeRemovedAt` is set). Pass the fresh row, not the pre-lock one, to `ensureSpikeWorktree`. Judging the deadline before the make also stops a claim that is going to be refused from remaking a worktree the ending will discard a moment later. Add a test that holds `withSpikeEndLocks`, starts a claim, ends the spike, and asserts no worktree and no `spike-code-kept` checkpoint.

### S2B2 (major): a submit whose ending is interrupted can be undone by a new claim, and the spike then ends at its time box

`internal/server/spikes_claims.go:523-565` (`SubmitSpike` steps 4 and 5), `internal/server/spikes_claims.go:139-161` (`spikeClaims.refusal`), `internal/server/spikes_end.go:220-234` (`settleSpikeClaim`) and `internal/server/spikes_end.go:503-516` (`timeBoxedEnding`).

FR-13.7 says that if the server stops between step 4 (the claim ends `done`) and step 5 (the ending), reconciliation finishes it. But step 5 runs on the request's context, so it needn't take a server stop: a browser that navigates away, or an MCP client that gives up on a slow submit (the leak check and the commit run in step 5), cancels `ctx`, `endSpikeLocked` fails, and the spike stays `running` with its claim ended `done`. In that state nothing refuses a new claim: `spikeClaims.refusal` sees no current claim and lets one be made. Both reconciliation and the ending decide "a submit got there first" from `LatestClaimFor` alone, so once a newer claim exists the `done` claim is invisible.

Failure scenario (reproduced): a person presses **I've finished**; the request is cancelled after the claim's transaction commits. They reload the page, which shows a running person spike with no claim and offers **I'll run this spike**; they press it. The new claim is open, so the next heartbeat doesn't conclude the spike. When the deadline passes, the spike ends `time_box`, and its findings say "The spike reached the end of its time box" over the findings that were submitted and accepted. FR-15's acceptance says this must never happen ("never findings that say `time_box` after a submit was accepted").

Fix, both parts:
- run step 5 on `context.WithoutCancel(ctx)`, so a client going away can't stop an ending whose claim has already ended;
- treat "a claim of this spike ended `done`" as final wherever it is read, not only when it is the latest: `spikeClaims.refusal` and `renewalRefusal` refuse a claim of such a spike ("SPK-003 has been submitted, and is ending. A person reads its findings on its page."), and `settleSpikeClaim` and `timeBoxedEnding` look for any `done` claim (a small `store.SpikeHasDoneClaim`). Add a test that commits step 4, skips step 5, tries a claim, and reconciles.

### S2B3 (minor): a chat or person spike whose start couldn't make the worktree gets a false "couldn't check" question at its first claim

`internal/server/spikes_end.go:355-361` (`spikeHadWorkingCopy`), `internal/server/spikes.go:314-321` and `:327-335` (the start's make), `internal/server/spikes_end.go:387-394`.

FR-12.2 allows the start's make to fail ("the start still stands, and the first claim makes it"). But `spikeHadWorkingCopy` is true for every started chat or person spike, whether or not its worktree was ever made, so that first claim runs `spikeKept` on a directory that never existed. The check fails closed: it audits `spike.code_kept` with `couldnt_check` and raises a `spike-code-kept` checkpoint. The audit record is then read back by the ending, so the spike's findings also say the check couldn't run.

Failure scenario (reproduced, by removing the worktree and pruning before the first claim): the start's `git worktree add` fails (a full disk, or a lock left in `.git/worktrees`); the person sees the warning only in the log, asks the chat agent to claim the spike, and the Inbox asks them to look for kept code in a working copy nobody ever had. The spike's committed findings carry the same paragraph.

Fix: record that the worktree was made. The simplest is an audit row (`spike.worktree_made`) written by `makeSpikeWorktree` after a successful `git worktree add`, and `spikeHadWorkingCopy` for a chat or person spike reading "has such a row" instead of `StartedAt != nil`. A `worktree_made_at` column would do too, but would need a migration beyond `0016`. Note in FR-13.3 that the predicate is "the worktree was made", not "the spike started".

### S2B4 (minor): no test for FR-15's "a submit and a deadline ending, run at once"

`internal/server/integration_spikes_timebox_test.go`, `internal/server/integration_spikes_claims_test.go`.

FR-15's acceptance asks for a test that runs a submit and a deadline ending at once and sees either the submitted findings and `concluded`, or a refused submit saying the findings were lost and `time_box`. No test exercises it: no test asserts `spikeTimeBoxEndedBeforeSubmit` ("The time box ended before this arrived, …"), and `TestSpikeLocksAreTakenInOrder` only checks that the two locks can be taken one after the other. The refusal branch at `spikes_claims.go:536-541` is untested, and S2B2 shows this is where the invariant can break.

Fix: add the test. Hold `withSpikeEndLocks` in one goroutine and end the spike `time_box` once a `SubmitSpike` (past step 1) is waiting; assert the refusal's sentence, `time_box`, the claim `expired`, and the draft in the findings. Then the reverse order, asserting `concluded` and the claim `done`.

## Checked and found right

- **Lock order (SD-27), traced for re-entry.** `ClaimSpike` takes the worktree's lock, then `ensureSpikeWorktree` takes `spikeEndMu` briefly, then `claimLocked` takes the spike's row and then its claim; nothing it calls takes either in-process lock again. `SubmitSpike` takes both through `withSpikeEndLocks` and ends through `endSpikeLocked`, which takes neither. `EndSpike`, `endSpikeAction` (from the rules), `ReconcileSpikes` (its `EndSpike` calls and its `finish` pass) and the sweep's `endAtDeadline` each take the locks once, from a caller holding none (`sweepFingerprint` releases the worktree's lock before step 2). `planSpike`'s `ensureSpikeWorktree` holds no lock when it takes `spikeEndMu`. No path holds `spikeEndMu` and then asks for a worktree lock.
- **Row lock order.** Every spike path locks the spike, then its claim, then touches checkpoints: `SaveSpikeFindings`, `SubmitSpike`, `ReleaseSpikeClaim`, `recordSpikeEnd`/`settleSpikeClaim` and `claimLocked` through `spikeClaims.lock`. The sweep's transactions lock only the claim. No spike path locks a feature, and the task path never locks a spike.
- **No git in a transaction (NFR-11).** The start makes the worktree after the commit; the leak check runs before the ending's transaction; `claimLocked` reads the head and fingerprint before its transaction.
- **Conditional updates.** `StartSpike` is conditional on `idea`; `EndSpikeState` on `running`; every claim change goes through `TransitionClaim`'s guarded update; the ending reads the claim under the spike's row lock, so a submit and an ending can't both win.
- **Migration 0016.** The backfill runs before the checks, so it applies to a database holding stage-1 spikes (an idea, a running one, an ended one, a closed one that ran). The checks admit every transition stage 2 makes: a chat or person start (no budget, a time box and a deadline), `time_box` only for chat and person, `budget` and `turn_limit` only for an agent, closing an idea (no executor), closing a chat spike (unanswered or answered), and ask-again (a new idea with no override). `work_claims_end_reason_check` is the name Postgres gave 0014's unnamed check, and `expired` is admitted.
- **Agent spikes and M13 regressions.** The dispatcher writes a `run-spike` run's agent execution once, in the transaction that marks it running; a retry writes nothing. `taskClaims.lock` keeps the feature-then-task lock (a task's target always has a feature). The task's sweep path, rules, Inbox words (including a question raised before `release_consequence` existed) and `claim_task`'s FR-6.3 sentence are unchanged; a spike's ID gets FR-13.5's sentence.
- **Refusals.** FR-13.1's sentences, in its order (state, executor, deadline, holder, working copy); a renewal after the deadline is refused; save and submit refuse a non-holder, a missing claim and a past deadline; submit honours a step-1 pass after the deadline; `EndSpike(time_box)` on an agent spike is refused.
- **The boundary (DEC-006 Amendment 1, DEC-007, FR-17).** MCP gains exactly `claim_spike`, `save_spike_findings` and `submit_spike`; nothing over MCP starts, closes, releases, extends or ends a spike, and the must-not-exist list carries FR-17.2's names. Release is only in the web UI and the Inbox.
- **The new POST routes.** All four check `sameOrigin`, take the spike's UUID from a hidden field, and call the same service methods as MCP; none can start, close or reopen a spike.
- **Closing by the person who ran it (SD-26), and ask-again (FR-15.5).** `closer_ran_it` is audited only for a person spike whose latest claim was the closer's; ask-again after a chat or person spike creates the new spike with no override, and the dialog has no budget field.
- **The findings (FR-15.3) and the time-box ending table.** The `time_box` entry has every field; the chat and person endings say no token count and that the tokens weren't measured.
- The stray apostrophe at `spike.html:237` is real, and is already raised as S2Q1 in the quality review.
- FR-18's named tests (`TestChatSpikeIsHeldByItsClaimAndDeadline`, `TestSpikeExecutorIsRecorded`, `TestChatSpikeTokensAreUnmeasured`, `TestChatSpikeEndToEnd`, `TestPersonSpikeEndToEnd`) don't exist yet; the plan gives them to T6, which is still pending, so they are not counted here.
