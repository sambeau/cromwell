# REVIEW-021, stage 2 code, round 2: bugs and spec conformance

Date: 2026-10-02

## Scope

The whole stage 2 code of SPEC-021 §4 (M14), `git diff 36b7fed..3967c3d -- . ':!docs'`, on branch `claude/subutai-m14-stage2-spikes`, with special attention to the round 1 fixes, `git diff 6ae7577..3967c3d`. It was read against SPEC-021 §4 (as revised in 3967c3d), SPEC-020, DEC-006 Amendment 1 and DEC-007, and against round 1's two reviews.

`go vet ./...` is clean and `gofmt -l internal/` prints nothing. In a private Postgres, `go test -race -count=3 -run 'Spike|Claim' ./internal/server/` passes (329 s), as does `go test -count=1 ./...`. The two sleep-based race tests were also run `-race -count=8 -cpu 1` with eight busy loops on four cores; they passed. The finding marked "reproduced" was shown with a throwaway test in a private worktree, since removed.

Severity, as round 1: **major** is wrong behaviour, a race that can corrupt state or deadlock, a spec requirement not met, or a security hole. **minor** is everything else worth fixing.

## Round 1's majors

| Round 1 | Status | Evidence |
|---|---|---|
| S2B1: a claim that waited on an ending remakes the ended spike's worktree and raises a false question | **Fixed for the case raised**, but the fix brought in S2B2-1 | `ClaimSpike` (`spikes_claims.go:426-449`) now reloads the spike under the worktree lock and refuses through `spikeBarredRefusal` before `ensureSpikeWorktree`, which it passes the fresh row. `TestAClaimThatWaitedOnAnEndingMakesNothing` and `TestAClaimPastTheDeadlineMakesNothing` cover it. |
| S2B2: a submit whose ending is cut short can be undone by a new claim | **Fixed** | Step 5 runs on `context.WithoutCancel(ctx)` (`spikes_claims.go:591`). `store.ClaimEndedDoneFor` looks at every claim. It is used by `spikeBarredRefusal`, which judges both the claim and the renewal (`:129-137`, `:173`, `:180`), by `settleSpikeClaim` (`spikes_end.go:208-217`) and by `timeBoxedEnding` (`:508-519`). `TestADoneClaimIsFinal` commits step 4 only, gets the new refusal, forces a later claim in, reconciles past the deadline, and sees `concluded` with the submitted findings. |
| S2Q1: stray apostrophe | **Fixed** | `spike.html:238`. |
| S2Q2: two time-left helpers, two zones | **Fixed** | `spikeSpanWords` and `spikeLeftWords` (`spikes_executor.go`) are the only formatters. `countUnit` is gone. Every clock is `spikeClock` (UTC, with "UTC" written after it). The person's panel no longer carries the MCP sentence (`ui_spikes.go:344-346`). |
| S2Q3: executor said in several places | **Fixed** | `spikeRanBy` serves the sentence, the findings (`settleSpikeClaim`) and the closer test (`spikeRunBy`). `executorPhrase` and `tokensNotMeasured` give the wording, and `spikeTokensNotMeasured` is gone. |
| S2Q4: duplicate state sentences; unused `resume` | **Fixed** | `spikeStateWords` is shared. `resume` is `_`, with a comment. The `judgeDeadline` flag is replaced by `spikeActRefusal`. |
| S2Q5: claim read repeatedly per render | **Fixed** | `readSpikeRunFacts` is read once per page (`ui_spikes.go:388`) and once per `spikeResult`. |
| S2Q6: `EndSpike` validates twice; the done rule written twice | **Fixed** | `checkSpikeEnding` is one function, and its pre-lock call carries a comment. The done rule lives only in `store.ClaimEndedDoneFor`. |
| S2Q7: `CanStart` dead; Start lost its disabled state | **Fixed, with a caveat** | The template reads `CanStart` (`spike.html:479`). However, `startScreenFor` never leaves `Executor` at `agent` when `Refusal` is set (`ui_spikes.go:753-761`), so the `disabled` branch can't render, and the field's comment (`:689-691`) describes a case that can't occur. This is harmless. Not restoring `required` on the budget field is correct, because a hidden required field would block a chat or person start. |
| S2Q8: stale and misleading comments | **Fixed**, with one comment missed (S2B2-3) | The test-file headers, `claims.go:212`, the task methods' doc comments, `EndSpike`'s and `EndSpikeState`'s wrapping, and `unmeasuredExecutor`'s home are all corrected. |

Round 1's minors S2B3 (the predicate is now "the worktree was made", audited as `spike.worktree_made`; tested by `TestAFailedStartMakeThenAClaimRaisesNoQuestion`) and S2B4 (`TestASubmitThatLosesToTheTimeBoxIsRefused`, `TestATimeBoxEndingAfterASubmitChangesNothing`) are also fixed. S2B4's test is judged in S2B2-2.

## Findings

### S2B2-1 (major): a claim that races the start makes the worktree without the worktree's lock, destroys the start's working copy, and raises false "couldn't check" questions

`internal/server/spikes_claims.go:420-447` (`ClaimSpike`), with `internal/server/spikes.go:338-345` (`makeStartedSpikeWorktree`) and `internal/server/spikes_end.go:378-412` (`makeSpikeWorktree`).

S2B1's fix takes the lock path from the spike as first read (`lockPath` is `""` when `sp.WorktreePath` is empty) and judges the state only on the second read, under that lock. An idea has no worktree path, because `StartSpike` sets it. So when the first read sees an idea, `withWorkingCopy("")` takes no lock. If the start commits before the second read, that read sees a running chat spike with a path, `spikeBarredRefusal` passes, and `ensureSpikeWorktree` runs with no lock, alongside the start's own `makeSpikeWorktree`, which holds the lock the claim should have taken. Both stat, `os.RemoveAll`, `git worktree prune` and `git worktree add` the same directory. Before the fix, the pre-lock `spikeStateRefusal` refused an idea outright, so this path is new in 3967c3d. It breaks SD-27's rule that a make holds the worktree's lock.

Failure scenario (reproduced). A throwaway test ran `StartSpike(chat)` and a `ClaimSpike` loop at once, 80 times:
- about one claim in four was refused with "SPK-021's working copy couldn't be made, so it can't be claimed yet: … fatal: '…/spk-edcf7e' already exists";
- twice the claim's `RemoveAll` removed the worktree the start had just made, and its own `add` then failed. The start returned success and recorded `spike.worktree_made`, but no working copy was left;
- three `spike-code-kept` checkpoints were raised, each saying "Subutai couldn't check whether code from SPK-002 was kept outside its working copy: the working copy's HEAD reflog couldn't be read (no such file or directory)". These spikes had never been worked in. Some came from the claim's leak check reading git's admin directory while the start's `add` was still writing it. Others came from the retry claim, which found the made-but-missing worktree that `spikeHadWorkingCopy` now reports as "had".

In use, the window is the chat agent calling `claim_spike` at the moment a person presses **Start this spike**, for example when the agent retries after "isn't running". The results are the false Inbox question S2B1 fixed and that question's paragraph in the findings, a refused claim, or a running spike with no working copy.

Fix: the path is a pure function of the spike's ID (`spikeWorktreeRel(sp.ID)`, `spikes.go:196`), so take the lock on `s.worktreeAbs(s.spikeWorktreeRel(sp.ID))` whatever the first read says. Alternatively, refuse with `spikeStateRefusal` on the first read when it is an idea, as before the fix. The first fix is better, because it doesn't depend on which state the first read happened to see. Add a test that holds the lock on that path, starts the spike through the store, calls `ClaimSpike` on the idea's public ID, and asserts the claim waits.

### S2B2-2 (minor): the two new race tests order their goroutines with sleeps; one can fail on a slow machine, the other can pass without testing anything

`internal/server/integration_spikes_races_test.go:98` and `:190`.

`TestASubmitThatLosesToTheTimeBoxIsRefused` sleeps 500 ms and assumes that `SubmitSpike` has passed step 1 (`resolveSpikeRef`, `currentSpikeClaim`, `validateFinishSpike`) and is waiting on the locks. If the goroutine is scheduled late, the ending runs first and step 1 sees an ended spike. The submit is then refused with "SPK-… has ended. A person reads its findings on its page." rather than `spikeTimeBoxEndedBeforeSubmit`, and the test fails. `TestAClaimThatWaitedOnAnEndingMakesNothing` sleeps 300 ms. If its claim is late it reads the spike after the ending, which the old code also handled, so it passes without exercising the wait it is named for. Neither failed in this review's runs, including under load with `-cpu 1`, so this is a risk and not an observed flake.

Fix: replace the sleeps with a signal. A test-only hook on `Server` (for example `testHook func(point string)`, nil in production) can be called in `SubmitSpike` after step 2 and in `ClaimSpike` after the first read. The test then releases the ending only once the hook has fired, so it doesn't depend on timing.

### S2B2-3 (minor): FR-12.4 and FR-15.2, and the reconciliation comment, still say "its latest claim ended done"

`docs/specs/SPEC-021-spikes.md:1182` (FR-12.4) and `:1462` (FR-15.2); `internal/server/spikes_end.go:421-422`.

S2B2's fix made the rule "any claim of the spike ended done" (`store.ClaimEndedDoneFor`), and FR-13.1 gained the matching refusal. The two requirements that state the rule, and `ReconcileSpikes`'s doc comment, still say "latest claim". As written, the spec allows a later claim to hide the submit, which is the behaviour S2B2 removed. FR-12.4's and FR-15.2's acceptance would be checked against the wrong rule.

Fix: in both places, say "a claim of it ended `done` (any claim, not only the latest)", and update the comment.

## Checked and found right

- **Lock order (SD-27), traced again after the fixes.** `ClaimSpike` takes the worktree's lock (the right one once the spike is running; see S2B2-1 for an idea). It then reads the spike and every claim on the pool, and calls `ensureSpikeWorktree`, which reads `spike.worktree_made` on the pool, takes `spikeEndMu` only around `spikeKept`, and writes the audit row in a short transaction of its own. It then calls `claimLocked`, which takes the rows. Nothing in that chain takes the worktree's lock or `spikeEndMu` again, and no transaction is open while either in-process lock is taken. `SubmitSpike` takes both locks through `withSpikeEndLocks` on a running spike's path. Its step 4 transaction and `endSpikeLocked` take neither lock again, and `endSpikeLocked`'s `spikeKept` reads the pool outside any transaction. `EndSpike`, the rules' `endSpikeAction`, the sweep's `endAtDeadline` and reconciliation each take the locks once, from callers that hold none.
- **`spikeHadWorkingCopy` reading the database.** Every caller (`makeSpikeWorktree`'s check and `leakedRefs`) runs outside a transaction and uses the pool, so it can't deadlock against a row lock or exhaust the pool. For an agent spike it is still `TokensUsed > 0`, with no query, so stage 1 and the planner are unchanged. `recordSpikeWorktreeMade` writes nothing for an agent spike.
- **`store.RecordSpikeWorktreeMade`.** It checks and then inserts without a lock, but every caller holds the worktree's lock (apart from S2B2-1's race), and a duplicate row would be harmless because the read is `EXISTS`. The new audit kind falls to the `default: continue` case on the spike page's timeline, so it is not shown.
- **`context.WithoutCancel` in `SubmitSpike`.** It covers only step 5, after the claim's transaction has committed. If the commit itself is cut off, the result is left to reconciliation, which now concludes the spike because no new claim can be made.
- **`store.ClaimEndedDoneFor`** is only called for spikes. M13's task claims, `taskClaims.lock`, `claimRules` and `claim_task`'s sentences are unchanged by the fixes. The task path never calls it, and a re-asked spike is a new row, so an old `done` claim can't reach it.
- **The refusals.** FR-13.1's order (state, executor, submitted, deadline, holder, working copy) matches the revised spec, as do the new sentence "SPK-003 has been submitted, and is ending. A person reads its findings on its page." and the times with "UTC" after them. Submit's step 4 refuses with the time-box sentence only when the spike ended `time_box`, and otherwise with the holder's sentence or "Your claim on SPK-003 changed while this was arriving…".
- **`settleSpikeClaim`** expires an open claim even when a done claim exists, so nothing is left open on an ended spike. The actor it names comes from `spikeRanBy`, which agrees with the page and with `closer_ran_it`.
- **The start screen.** `parseTimeBoxField` and `parseBudgetField` now share `checkSpikeTimeBox` and `checkSpikeBudget` with `startSpike`, and a refused field still returns its own sentence.
- **FR-18's named tests exist and assert what FR-18 lists.** In `TestChatSpikeIsHeldByItsClaimAndDeadline`, the claim's deadline equals the spike's, `claim-stale` is raised and then withdrawn by a save, `claim-deadline` is never raised, and the deadline ends the spike `time_box` with the claim `expired` and the worktree gone. `TestSpikeExecutorIsRecorded` covers the row, `executions` and `get_spike` for all three executors. `TestChatSpikeTokensAreUnmeasured` checks `measured = false`, null `tokens_used`, the unmeasured lines, and that the forecast and its samples are unchanged. `TestChatSpikeEndToEnd` and `TestPersonSpikeEndToEnd` go start to close through the real routes and tools, check that the only commit is the findings and that no branch or worktree is left, and that `close_spike` over MCP is method-not-found.
- **The boundary (DEC-006 Amendment 1, DEC-007, FR-17).** The fixes add no tool and no route. MCP still has only `claim_spike`, `save_spike_findings` and `submit_spike` for running a spike. Release, start and close stay in the web UI.
- **Agent spikes and M13 regressions.** The dispatcher, the planner's `ensureSpikeWorktree` and the agent's leak-check predicate are untouched by the fixes. The full suite (`go test ./...`) passes.
