# REVIEW-021, stage 2 code, round 3: code quality

Date: 2026-10-02

## Scope

The stage 2 code of SPEC-021 §4 (M14), `git diff 36b7fed..e3f5a29 -- . ':!docs'`, on branch `claude/subutai-m14-stage2-spikes`, with attention on the round 2 fixes (`git diff c039a0a..e3f5a29`). Earlier reviews: `REVIEW-021-stage2-code-round1-quality.md` and `-round2-quality.md` (S2Q2-1 to S2Q2-14). This round judges whether the two round 2 majors are fixed, what the fixes introduced (the `spikeHook` seam, `spikeBoxSentence`, `spikeActionWords`, `closerRanIt`, the `.triage-no-field` CSS and template, the test helpers), and whether any round 2 minor is really a major. The checkout was not changed.

`gofmt -l internal cmd` prints nothing and `go vet ./...` is clean at e3f5a29. The tests were read, not run.

Severity as before: **major** is a bug-in-waiting, structure a maintainer must untangle, a misleading name or comment, or a test that does not test what it says. **minor** is the rest.

## Round 2's majors

| Round 2 | Fixed? | Evidence |
|---|---|---|
| S2Q2-1 Start button's `disabled` branch unreachable | Fixed | `CanStart` now means something that can happen: `startScreenFor` sets `p.TemplateRefusal = s.findingsTemplateRefusal()` and `p.CanStart = !p.Started && p.TemplateRefusal == ""` (`ui_spikes.go:769-770`), and `startSpike` uses the same `findingsTemplateRefusal` (`spikes.go:257`, `:333`), so the screen and the POST cannot disagree. The button's `disabled aria-describedby="start-template"` points at a sentence that is rendered only when there is a refusal (`spike.html:480-485`). The field's comment now says what it is. `TestSpikeStartIsDisabledWithoutAFindingsTemplate` moves the template away and checks the button, the id and the sentence, then restores it and checks the button is enabled; `TestSpikeStartScreenOffersWhoRunsIt` checks the button is enabled with nobody assigned to run spikes; the stage 1 test's comment and assertion now say "the agent choice disabled" and read the radio's tag with `radioTag`, not the bare word over the page. (Two small leftovers: S2Q3-4, S2Q3-9.) |
| S2Q2-2 sleep-based race tests | Fixed | `Server.spikeHook` with three named points (`server.go:65-69`, `spikes_claims.go:394-407`). `TestAClaimThatWaitedOnAnEndingMakesNothing` and `TestASubmitThatLosesToTheTimeBoxIsRefused` now wait on `hookReached`/`await` (a channel with a 30 s timeout that fails the test) and have no `time.Sleep`. `spikeHookSubmitChecked` sits after the submit's first looks and before `withSpikeEndLocks` (`spikes_claims.go:569`), so a late submit can no longer be refused at step 1; `spikeHookClaimRead` sits after the claim's first read, so the claim test now has read the spike `running` before the ending is released. A new test, `TestAClaimOvertakenByAStartWaitsForTheStartsWorktree`, uses the seam to pin the lock-path fix (`ClaimSpike` now locks `worktreeAbs(spikeWorktreeRel(sp.ID))` whatever the first read saw). One sleep remains, as a negative wait (S2Q3-2). |

Of the round 2 minors, S2Q2-3 (in part: `go_` is `proceed`, `startAgentSpikeQuiet` returns two values, the `s2` names are gone from the start helpers), S2Q2-5 (`closerRanIt`), S2Q2-6 (`submitted` renamed `done`, `spikeSubmitted` removed), S2Q2-7 (`spikeBoxSentence`; the past-tense case is handled), S2Q2-8 (no review IDs remain in `internal`), S2Q2-9 (`CloseSpike` reads facts only for a person spike; `spikeResult` logs the failed read), S2Q2-11 (`spikeActionWords`, in part) and S2Q2-13 (the repeated `ShowAgainBudget` blocks are two forms; the capitalised "The spike runner" is fixed; the page assertions use `radioTag`) are done or mostly done. S2Q2-4, S2Q2-10, S2Q2-12 and S2Q2-14's test case are not done, and none of them is major.

## Findings

### S2Q3-1 (minor): the seam is sound, but its points are bare strings and one is used by one test

`spikes_claims.go:394-407`, `server.go:65-69`.

The seam is small and the right shape: one nil-checked field, one three-line method, three call sites, a comment on the field saying it is nil in production and set by a test before the server is used, and a comment on the constants. It is not documented as test-only in the name, and the point is a `string`, so a typo in a test's `case` compiles and the hook never fires (the test then times out at 30 s, with a message that names what it waited for, which is the fallback). `spikeHookClaimLocked` is used by one test only.

Fix: `type spikePoint string` for the constants and the hook's parameter, so a misspelt point does not compile; add "test seam" to the field's comment's first words (`// spikeHook is a test seam: it is nil in production.`). Keep `claim-locked` only while that test needs it.

### S2Q3-2 (minor): the overtaken-start test ends with a negative wait and leaks its goroutines on failure

`integration_spikes_races_test.go:359-365`, `:327-345`.

After `close(resume)` the test waits 300 ms for `claimLocked` not to fire, which is a sleep standing for "the claim is blocked on the lock". It cannot fail a correct implementation, but it cannot prove the claim was blocked either: a claim that is slow to reach the lock passes. It is the right kind of check for a negative, and it is bounded, so this is minor. The fix being tested is also proved by the later assertions (one worktree, no code-kept checkpoint), which fail if the claim ran beside the start. Separately, a `t.Fatal` between the claim's goroutine starting and `close(resume)` leaves it blocked in the hook for good, and `h.srv.spikeHook` stays set for the rest of the test.

Fix: say in the comment that the wait is only a bound on a negative and that the assertions after it are the proof; start `defer close(resume)`-style cleanup (a `sync.Once`-guarded close) with `t.Cleanup` so a failing test does not strand the goroutine.

### S2Q3-3 (minor): `TestAClaimThatWaitedOnAnEndingMakesNothing` still cannot say the claim reached the lock

`integration_spikes_races_test.go:113-125`.

Its comment says "the claim reads the spike again only once the ending is over", which is true however slowly the claim goes on. The race it names is a claim that read the spike `running` and then met an ending, and that is now guaranteed. Whether the claim was already waiting on the lock or arrived after the ending is not observed, and the two cases take different paths in the code (the lock's queue, or a free lock). The test name says "waited".

Fix: name it for what is proved (`TestAClaimThatReadTheSpikeRunningAndMetItsEnding`) or add the `spikeHookClaimLocked` point's complement (a "claim-waiting" point cannot be placed, since the wait is inside `Lock`), and keep the name only if the comment says the order after the first read is not controlled.

### S2Q3-4 (minor): `CanStart`'s comment omits `Started`, and two structs share the name

`ui_spikes.go:694-699`, `spike.html:262` and `:480`.

The comment says `CanStart` "is false only when nothing can start: the findings template or its manifest is missing", but the assignment is `!p.Started && ...`, so it is also false for a started spike (the button is in the `{{else}}` of `{{if .Started}}`, so that case never renders it). `spikePageData`'s `CanStart` (the Start link on the spike page, `spike.html:262`) is a different fact with the same name, and `entity.html` has a third.

Fix: say "or the spike has started" in the comment, or drop `!p.Started` from the assignment, since the template already branches on it; rename the start screen's field `Startable` or `TemplateMissing` (its refusal is the thing that matters).

### S2Q3-5 (minor): `spikeActionWords` has odd field names, and an unknown action fails silently

`spikes_claims.go:335-340`, `:347-356`.

`nothing` holds a whole clause ("there are no findings to save"), and `object` is a pronoun phrase ("its findings", "it"); a reader sees `words.nothing` in a Sprintf and has to look it up. `spikeHolderRefusal` still builds "there is nothing to "+action and "there is nothing to %s" from the verb separately, so the same idea is said by the table in one place and by `action` in two. A third action that is missing from the map gives empty strings and the sentence "You haven't claimed SP-1, so . Claim it first." with no error.

Fix: name the fields `noDraft` and `itsObject`, or build all four from `verb` and `object`; look the action up with `words, ok := ...` and panic or return a generic sentence when it is missing, since the call sites are fixed strings.

### S2Q3-6 (minor): `spikeLeftWords` has a dead branch, and `spikeTimeLeft` repeats `spikeBoxSentence`'s test

`spikes_executor.go:218-221`, `:236-241`, `spikes_claims.go:68-76`.

`spikeLeftWords`'s only caller is now `spikeTimeBoxLine`, which calls it only when `left > 0`, so its `d <= 0` "no time left" branch cannot run (and its doc comment still promises it). `spikeTimeBoxLine` still words the past case itself ("ended at"), beside `spikeBoxSentence`, so the "ended at X" sentence is said in two places and the "ends at X, in N" in two. `spikeTimeLeft` tests `!sp.DeadlineAt.After(now)` and `spikeBoxSentence` tests `left <= 0`: the same fact in two spellings.

Fix: drop the branch and the words from `spikeLeftWords`'s comment, or let `spikeTimeBoxLine` call `spikeBoxSentence` and trim its trailing full stop; have `spikeTimeLeft` choose its tail from one `ended := !deadline.After(now)`.

### S2Q3-7 (minor): the person check is made twice in `CloseSpike`

`spikes.go:388-395`, `spikes_executor.go:176-182`.

`CloseSpike` now tests `cur.Executor == store.ExecutorPerson` to avoid reading the facts, and `closerRanIt` begins with `sp.Executor != store.ExecutorPerson || who == ""` as well. They agree today, but a change to the rule (for example a chat closer who ran it) would have to be made in two places, one of which is silent because it merely skips a read.

Fix: say in the comment above the `if` that it is only to spare the read, and that `closerRanIt` is the rule; or make `readSpikeRunFacts` cheap for a spike that is not a person's and call `closerRanIt` unconditionally.

### S2Q3-8 (minor): helper names close to production names, and a helper doc comment that needs a verb

`integration_spikes_stage2_e2e_test.go:62-63`, `integration_spikes_timebox_test.go:264`.

`closerRanItAudit` (a harness method) sits beside the production function `closerRanIt`, and a search for one finds both. `startedViaScreen` and `startViaScreen` differ by one letter, and the first calls the second. `closerRanItAudit`'s comment starts "is the spike.closed audit row's" without its subject. The helpers that round 2 asked to be gathered (`spikeCodeKeptCheckpoints`, `endClaimDone`, `holdEndLocks`, `hookReached`, `await`, `radioTag`, `startButton`) are still beside their tests, and `radioTag` and `startButton` are in the stage 2 UI file but used by the stage 1 UI test in another file.

Fix: `closerRanItAudit` to `auditCloserRanIt`; `startedViaScreen` to `startChatSpikeViaScreen`; move `radioTag` and `startButton` to `integration_spikes_helpers_test.go`.

### S2Q3-9 (minor): the start screen test still asserts the bare word "disabled" over the page

`integration_spikes_ui_stage2_test.go:59`.

`lacks(t, "the start screen", screen, "disabled")` remains in `TestSpikeStartScreenOffersWhoRunsIt` and passes only while no script, hint or class on the page contains the word. The radio and the button are now checked by their tags just below, so it adds only brittleness (round 2's S2Q2-13).

Fix: delete it; the two tag checks say the same thing about the elements.

### S2Q3-10 (minor): the `.triage-no-field` form repeats the hidden fields of the other form

`spike.html:285-301`, `app.css:2319`.

The two "again" forms now share two hidden inputs and the button's markup, in two `<form>` elements with different layout classes. The CSS and the template are otherwise clean: the class is used, it sits beside `.triage-with-field`, and the test asserts each page has its own. A third variant would need a third copy.

Fix: one form, with `class="u-mt-4 {{if .ShowAgainBudget}}triage-with-field{{else}}triage-no-field{{end}}"` and the field and hint chosen inside it; this is round 2's S2Q2-13 point, now only half taken.

## Round 2 minors checked for major

None of S2Q2-3, -4, -10, -11, -12, -13 or -14 is a major. S2Q2-12 (`startScreenFor`'s three-way condition for the executor, the spike read three times in each UI handler) is still untidy but is structure that works and is tested; S2Q2-14 (the executor sentence and the held claim's actor) is pinned by `spikeRanBy`'s new comment, though no test case has the execution's actor differ from the claim's.

## Summary

Both round 2 majors are fixed with evidence: Start's `disabled` branch is reachable and tested in both states (S2Q2-1), and the two race tests wait on a named fact through a small, documented, nil-in-production seam (S2Q2-2). The seam, `spikeBoxSentence`, `closerRanIt`, `spikeActionWords` and the `.triage-no-field` form are minimal and sensibly named. Ten new findings, all minor (S2Q3-1 to S2Q3-10). **No major is open.**
