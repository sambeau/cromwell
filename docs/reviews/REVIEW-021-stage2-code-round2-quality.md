# REVIEW-021, stage 2 code, round 2: code quality

Date: 2026-10-02

## Scope

The stage 2 code of SPEC-021 §4 (M14), `git diff 36b7fed..3967c3d -- . ':!docs'`, on branch `claude/subutai-m14-stage2-spikes`, with attention on the round 1 fixes (`git diff 6ae7577..3967c3d`). Round 1 is `REVIEW-021-stage2-code-round1-quality.md` (S2Q1 to S2Q19). This round judges whether its majors are fixed, what the fixes introduced, and what round 1 missed: duplication, dead code, naming, comments, test quality, the spike page's markup, spelling. It does not judge behaviour against the spec.

`gofmt -l internal cmd` prints nothing and `go vet ./...` is clean at 3967c3d.

Severity: **major** is a bug-in-waiting, structure a maintainer must untangle, a misleading name or comment, or a test that does not test what it says. **minor** is the rest.

## Round 1's majors

| Round 1 | Fixed? | Evidence |
|---|---|---|
| S2Q1 stray apostrophe in the draft panel | Fixed | `spike.html` draft panel line now begins `<p class="t-meta">`; the "no findings" sentence is now parallel for both cases (`Whoever is running it hasn't saved any findings yet`). |
| S2Q2 two time-left helpers, two zones | Fixed | `countUnit` and the second formatter are gone. `spikeSpanWords`/`spikeLeftWords` (`spikes_executor.go:192-212`) are the one place a span is formatted, built on `plural`; `spikeClock` is UTC everywhere. `TestSpikeDeadlinesAreSaidInUTC` sets `time.Local` to a fixed zone and checks the page line and the chat sentence agree. The page's panel no longer says the MCP-voiced "Then the spike ends..." sentence. (A residue is S2Q2-7.) |
| S2Q3 executor said three times | Fixed | `spikeRanBy` (`spikes_executor.go:92`) is used by the executor sentence, `settleSpikeClaim` (`spikes_end.go:226-230`) and `spikeRunBy`; `executorPhrase` and `tokensNotMeasured` replace the three "tokens weren't measured" sentences; `TestSpikeRanByIsOneAnswer` pins the release-then-second-claim case. |
| S2Q4 repeated refusal sentences, unused parameters | Fixed | `spikeStateWords` (`spikes_claims.go:86`) is called by both `spikeStateRefusal` and `spikeHolderRefusal`; the `judgeDeadline` flag is replaced by `spikeActRefusal`. The remaining `_ bool` and unused `ctx`/`tx` are the interface's own. |
| S2Q5 claim read repeatedly per page and result | Fixed | `spikeRunFacts`/`readSpikeRunFacts` (`spikes_executor.go:53-85`) is read once in `spikePageData`, `spikeResult`, `CloseSpike` and `settleSpikeClaim`; `spikeExecutorSentence`, `spikeRunBy` and `spikeClaimPanelFor` take it. (A residue is S2Q2-9.) |
| S2Q6 `EndSpike` validates twice; two "done" rules | Fixed | The two checks are one `checkSpikeEnding` with a comment saying why it runs before the locks and again under them (`spikes_end.go:70-73`). The rule is `store.ClaimEndedDoneFor` (`store/claims.go:148`), used by `settleSpikeClaim` and `timeBoxedEnding` through `spikeSubmitted`. |
| S2Q7 Start screen's disabled state and dead `CanStart` | **Not fixed** | `CanStart` is read again and the template has `disabled` back, but the branch can never render. See S2Q2-1. |
| S2Q8 stale and misleading comments | Fixed | The "skip until T3" and "DEPENDS ON T3" comments are gone; `claims.go:212` says "the registry of claimables by ref_type"; `wantExactRefusal` is now `wantRefusalSentence` with a comment naming the other helper correctly; the three task methods have doc comments; `spikes_executor.go`'s header covers what it holds and `unmeasuredExecutor` moved into it; `EndSpike`'s and `EndSpikeState`'s comments are re-wrapped. |

Of the minors, S2Q9 (empty branch), S2Q13 (`spikeTimeLeft`), S2Q10 (the embedded `noSubmitRelease`), S2Q15's helper merge (in part), S2Q16's rename of the lock test, S2Q19's `CloseSpike` and range-check merges are done. S2Q11, S2Q12 and S2Q14 are only partly done (S2Q2-12, S2Q2-11).

## New findings

### S2Q2-1 (major): the Start button's `disabled` branch is unreachable, and its comment and a test comment say otherwise

`ui/templates/spike.html:479`, `ui_spikes.go:689-692` and `:762`, `integration_spikes_ui_test.go:109`.

`startScreenFor` sets `p.Executor = chat` whenever the runner is refused (`:757-759`), and the form can only override that to chat or person, since the agent radio is disabled. So `p.CanStart = !p.Started && (p.Executor != agent || p.Refusal == "")` is always `!p.Started`. The button sits in the `{{else}}` of `{{if .Started}}`, so `{{if not .CanStart}} disabled aria-describedby="start-runner"{{end}}` can never render. The field's comment ("the Start button is then disabled and says why") is false, and the stage 1 test comment at `integration_spikes_ui_test.go:109` ("the sentence, and the button disabled") describes a button that is now enabled; its `wants(..., "disabled")` passes only because the agent radio is disabled. The round 1 fix restored the markup without restoring the meaning, and the stage 2 design (chat is always available) means there is nothing for it to disable.

Fix: delete `startScreen.CanStart`, its assignment and the `disabled` branch, and say in the comment on `Refusal` that the Start button is always enabled because the chat agent and a person need no runner. Change the stage 1 test comment, and assert the disabled radio by its element (`value="agent"` with `disabled`) rather than the bare word "disabled" over a whole page.

### S2Q2-2 (major): the race tests sleep, and one cannot tell a waiting goroutine from a late one

`integration_spikes_races_test.go:98` and `:190`.

`TestAClaimThatWaitedOnAnEndingMakesNothing` starts a claim, sleeps 300 ms and releases the ending's locks. If the goroutine has not reached the lock by then (a loaded CI machine), the claim runs after the ending and is refused with the same sentence, so the test passes without having exercised the race it is named for; on a fast machine it passes either way too, since nothing observes that the claim was waiting. `TestASubmitThatLosesToTheTimeBoxIsRefused` is worse: a late submit hits step 1's "has ended" refusal, not `spikeTimeBoxEndedBeforeSubmit`, so a slow machine fails it. Both are sleep-based and neither proves what its comment says ("waits on the locks").

Fix: wait for the fact, not the clock. Poll `pg_locks`/`pg_stat_activity` for the claim's or submit's transaction waiting on the spike row, or give `withSpikeEndLocks` a test-only hook that signals "waiting", or poll `h.srv.spikeEndMu.TryLock()`-style state for the worktree lock with a deadline. For the claim test, assert from inside the hook that the claim has passed its first read of the spike (still `running`) before the ending is released.

### S2Q2-3 (minor): test helpers are still spread over six files, with generic and numbered names

`integration_spikes_helpers_test.go` was made, but helpers for the same tests still live elsewhere: `spikeCodeKeptCheckpoints`, `endClaimDone`, `holdEndLocks` in `integration_spikes_races_test.go:21-76`; `checkEndedAtTimeBox`, `startAgentSpikeQuiet`, `closedBySentence` in `integration_spikes_timebox_test.go:22,264,270`; `s2StartInChat`, `s2StartInUI`, `heartbeatDuties`, `worktreePaths`, `s2CloserRanIt` in `integration_spikes_stage2_e2e_test.go:24-76`; `spikeForm` in `integration_spikes_ui_stage2_test.go:18`; `jsonText` in `integration_spikes_mcp_test.go:236`. The `s2` prefix is a milestone's name used as a namespace on `harness`, and says nothing a reader can use. `startAgentSpikeQuiet` returns `(started, nil, err)`: the dispatch result is always nil, so the signature lies. `holdEndLocks` names a channel `go_` to avoid the keyword. `spikeWithDraft`'s doc comment starts mid-line and is longer than its neighbours (`integration_spikes_helpers_test.go:86-89`).

Fix: move the shared ones into the helpers file and keep only single-file helpers beside their tests; rename `s2StartInChat` to `startedViaScreen`, `s2CloserRanIt` to `closerRanIt`; make `startAgentSpikeQuiet` return two values; rename `go_` to `proceed`.

### S2Q2-4 (minor): two test files whose names are each other's reverse

`integration_claims_spikes_test.go` (stage 1, claim-related spike rules) and `integration_spikes_claims_test.go` (stage 2, claiming a spike) differ only in word order. A maintainer looking for a claim test cannot tell which file to open.

Fix: rename the stage 2 file to `integration_spike_claiming_test.go` and say in each file's header comment what the other holds.

### S2Q2-5 (minor): `spikeRanBy` and `spikeRunBy` differ by one letter and answer different questions

`spikes_executor.go:92` returns `(kind, actor string)`; `:176` returns a `bool` for "is who the person who ran it by hand". Either is easy to call for the other, and the compiler will catch a wrong one only half the time (`_, actor := spikeRanBy(...)` against `spikeRunBy(...) == who`). The round 1 fix made them share an answer, which was right, but the names now read as typos of each other.

Fix: rename the bool one `closerRanIt(sp, facts, closer)`, which is what `store.CloseSpike`'s parameter calls it.

### S2Q2-6 (minor): `settleSpikeClaim` shadows the `submitted` type, and `spikeSubmitted` is a pass-through

`spikes_end.go:208-216` declares `submitted, err := spikeSubmitted(...)`. `submitted` is already a type in `claims.go` (it is in `noSubmitRelease`'s signatures, same package). It compiles and then reads as an error to anyone searching for the type. `spikeSubmitted` (`spikes_claims.go:121`) is one line over `store.ClaimEndedDoneFor`, with the same arguments plus a spike that it only unwraps.

Fix: call the variable `done`, and either drop `spikeSubmitted` and call `store.ClaimEndedDoneFor(ctx, q, "spike", sp.ID)` or keep it and say why (it names the rule for readers of the server code).

### S2Q2-7 (minor): the time box's sentence is still built in three places, and one reads badly

`spikeTimeLeft` (`spikes_claims.go:57`), `spikeTimeBoxLine` (`spikes_executor.go:222`) and `spikeClaimPanelFor` (`ui_spikes.go:344`, `"The time box ends at " + spikeClock(...) + ": " + spikeLeftWords(...) + "."`) each assemble "ends at X, N left" in their own words. Past the deadline the panel says "The time box ends at 16:04 UTC: no time left.", which is wrong in tense. The span words are shared; the sentences are not.

Fix: one `spikeBoxSentence(deadline, now, voice)` or give the panel `spikeTimeBoxLine`'s tail; handle the past case in one place ("The time box ended at X.").

### S2Q2-8 (minor): review identifiers in code comments

Eight comments cite `S2Q2`, `S2Q3`, `S2B1` to `S2B4` (`spikes_executor_test.go:106,134,160`, `integration_spikes_races_test.go:77,113,131,172,231`). Review IDs mean nothing once the review documents are archived, and the comments describe the case less well than a sentence would.

Fix: replace each tag with the rule it tests (for example "a claim that waited on an ending finds the spike ended"); the tests already say so in the line after the tag.

### S2Q2-9 (minor): `readSpikeRunFacts` reads more than its callers use, and one caller drops its error

`spikes_executor.go:64` always reads the executions, then the claim for a chat or person spike. `CloseSpike` (`spikes.go:378`) needs only the actor of a person spike, and for an agent spike reads a list of executions to throw it away; `settleSpikeClaim` only runs for chat or person spikes. `list_spikes` still reads per spike. In `spikeResult` (`mcp_spike_tools.go:333-337`) a failed read sets `facts = spikeRunFacts{}` and drops the error with no log, so a database fault reads as "To be run by the spike runner."

Fix: return early from `CloseSpike` for a non-person spike before the read; log the error in `spikeResult`; have `list_spikes` read executions in one query for its page (`ExecutionsForAll(ids)`), when the list grows.

### S2Q2-10 (minor): generic names in package `server`, and the executor words in three places

`executorPhrase`, `tokensNotMeasured`, `unmeasuredExecutor`, `noSubmitRelease` and `errSpikeOwnMethod` are exported to the whole package with no `spike` prefix, while neighbours (`spikeClock`, `spikeSpanWords`) have one. In a package with tasks, bugs and claims, `tokensNotMeasured` reads as general. The executor's words are still said by `executorPhrase` ("in chat", "by hand", `spikes_executor.go:30`), `spikeExecutorWord` ("chat", "by hand", `ui_spikes.go:69`) and the two refusal sentences in `spikeStateRefusal` (`spikes_claims.go:106,110`).

Fix: prefix them `spikeExecutorPhrase`, `spikeTokensNotMeasured`, `spikeUnmeasured`; put `spikeExecutorWord` beside `spikeExecutorPhrase` so the two lists of words sit together.

### S2Q2-11 (minor): `spikeHolderRefusal` branches on a string twice, and `deadlineQuestion` is still five lines for nothing

`spikes_claims.go:333-358`: the "save"/"submit" strings are chosen with `if action == "save"` in two places, giving four near-identical `Sprintf`s, and `action` is also spliced into "there is nothing to %s" and "isn't open, so there is nothing to %s". A third action would need four more branches. `deadlineQuestion` (`:218`) is still implemented and never reached (the sweep ends a spike before asking, `claim_sweep.go:67-74`); it calls `staleQuestion` with a duration from `time.Since`, which is not what a deadline question is.

Fix: a small table `spikeActions = map[string]struct{ need, noun string }` or two arguments (`verb`, `object`) so the sentences are built once; make `deadlineQuestion` part of an optional `deadlineAsker` interface so a spike need not carry it.

### S2Q2-12 (minor): S2Q11 and S2Q12 are only half done

`spikeClaimFromForm` now returns `(sp, ok)`, but each handler still calls `ClaimSpike(ctx, sp.PublicID, ...)`, which resolves the same spike by public ID again (`resolveSpikeRef`), and `renderSpikeAgain` reads it a third time. `startScreenFor` (`ui_spikes.go:755-761`) is unchanged: a default, a refusal override and a three-way condition for the executor; `handleUISpikeStartPost` and `startScreenFor` both default an empty value.

Fix: as round 1 said: `chosenExecutor(form, refused bool) string` with a switch; let the service methods take the spike's ID so the routes do not round-trip through the public ID.

### S2Q2-13 (minor): markup left over from round 1's list, and a brittle page assertion

- `spike.html` budget field: the `required` that stage 1 had was not restored (S2Q7's second point) and the hint now says "This field is read only when the spike runner runs it", which is the right place to say it, but the field is hidden by script only; without script, both limit fields show.
- The "again" form has two consecutive `{{if .ShowAgainBudget}}` blocks (`spike.html:288-299`); merge them so the input, button and hint for each case are together.
- The disabled agent radio's hint now says "The reason is under The spike runner, below." and points at `start-runner` through `aria-describedby`, which is good, but `The spike runner` is capitalised mid-sentence as a title; say "the spike runner's panel, below".
- `integration_spikes_ui_stage2_test.go:41-43,52-54` assert `value="agent" aria-describedby="start-ex-agent-hint" checked` and `lacks(..., "disabled")` over a whole page: they depend on attribute order and on the word "disabled" appearing nowhere else, including in a script or a hint. Read the radio element and check its attributes.

### S2Q2-14 (minor): `spikeExecutorSentence` and the page disagree on the chat claimant's actor

`spikes_executor.go:144-152`: `actor` comes from the latest execution of the kind (else the claim), while `ClaimedAt` and `held()` come from the latest claim. They agree because each claim writes one execution, but nothing in the function or the test says so, and `TestSpikeExecutorSentences` builds the two independently. A later change that records an execution at another moment (for example when work first happens) would make the sentence name one person and date another.

Fix: say in `spikeRanBy`'s comment that each claim records one execution, and add a test case with the execution's actor different from the held claim's, which pins which one the held sentence names.

## Summary

Seven of round 1's eight majors are really fixed, with the shared pieces (`spikeRunFacts`, `spikeRanBy`, `spikeSpanWords`, `spikeStateWords`, `ClaimEndedDoneFor`, `checkSpikeEnding`) in the right places and tests that pin them. S2Q7 is not: the Start screen's `disabled` branch is dead and its comments say it works (S2Q2-1). The new tests for the races use sleeps and cannot tell a waiting goroutine from a late one (S2Q2-2). Two majors are open (S2Q2-1, S2Q2-2); the rest are minor.
