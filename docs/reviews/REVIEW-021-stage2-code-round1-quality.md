# REVIEW-021, stage 2 code, round 1: code quality

Date: 2026-10-02

## Scope

The stage 2 code of SPEC-021 §4 (M14), `git diff 36b7fed..654d8b4 -- . ':!docs'`, on branch `claude/subutai-m14-stage2-spikes`. Read against the spec and M13's `claims.go`, `ui_claims.go` and `mcp_claim_tools.go`, and stage 1's spike files. The review judges simplicity, reuse, idiom, naming, comments, dead code, error handling, tests and the spike page's markup. It does not judge whether the behaviour meets the spec; other reviewers do that.

`gofmt -l` prints nothing and `go vet ./...` is clean on the checked-out tip.

Severity: **major** is duplication or structure a maintainer would have to untangle, misleading comments or names, tests that do not test what they say, or a visible defect. **minor** is the rest.

## Findings

### S2Q1 (major): a stray apostrophe in the spike page's draft panel

`internal/server/ui/templates/spike.html:237`. The line begins `'    <p class="t-meta">{{if .ByAgent}}The agent{{else}}Whoever is running it{{end}} last saved this on ...`. The leading `'` is rendered as text at the top of the "findings so far" panel on every running spike that has a draft. The tests assert substrings, so none caught it.

Fix: delete the apostrophe. Add an assertion in the page test that the panel's text does not begin with a quote, or compare the panel's rendered `<p>` exactly.

### S2Q2 (major): two "time left" helpers, two ways of formatting a time, and two time zones on one page

`spikes_claims.go:58-87` (`spikeTimeLeft`, `countUnit`) and `spikes_executor.go:134-153` (`spikeLeftWords`, plus `spikeTimeBoxLine` at 119-132) both turn a deadline into hours and minutes. They are written differently (`countUnit` against the existing `plural` in `observe.go:155`; one loops with a `parts` slice and a special case for under a minute inside the minutes branch, the other has the cleaner order), they word the zero case differently, and they format the clock differently: `spikeTimeLeft` and `spikeDeadlineWords` use `.UTC()`, `spikeTimeBoxLine` uses `.Local()`. On the page of a person-run spike, `#spike-timebox-line` says "ending at 17:04" in the server's local time and `#spike-claim-left` (from `spikeTimeLeft`, `ui_spikes.go:349`) says "ends at 16:04 UTC" for the same deadline. A person will see two different times for one moment. The UI's own `spikeWhen` (`ui_spikes.go:125`) uses the time as given, with no zone.

Fix: one function, `spikeLeftWords(d)`, built on `plural`, and delete `countUnit`. Build the MCP sentence (FR-13.4) and the page's sentence from it. Pick one zone per audience: UTC with a "UTC" suffix for MCP results (the chat agent has no zone), the page's own zone for the page. Do not show the MCP-voiced "Then the spike ends with whatever findings you have saved." sentence on the person's page; give the panel `spikeLeftWords` and the clock only.

### S2Q3 (major): the spike runner's executor is said in three places, and "tokens weren't measured" in three more

Who ran a spike is worked out three times: `spikeExecutorSentence` (`spikes_executor.go:51-117`, the page and `get_spike`), `settleSpikeClaim` returning `who` from the latest claim for the findings (`spikes_end.go:220-246`, then `unmeasuredEndedSentence`, `spikes.go:505-533`), and `spikeRunBy` (`spikes_executor.go:171-182`) for the closer. They choose the actor differently: the sentence prefers the latest execution of the kind, then the claim; the findings use the latest claim's actor; the closer test uses the claim only. A claim and an execution can disagree after a release and a second claim by someone else. The "ran in chat / by hand, so its tokens weren't measured" sentence is likewise written in `spikeUnmeasuredLine` (`spikes_executor.go:158`), `spikeTokensNotMeasured` (`spikes.go:536`), and the timeline's empty `if` branch in `ui_spikes.go` (S2Q9). The chat and by-hand branches are repeated again in `spikeNoticeFor`, `spikeExecutorWord` and `spikeExecutorSentence`.

Fix: one `spikeRanBy(execs, claim)` (kind, actor) used by the sentence, the findings and the closer test, so the three cannot disagree. One `executorPhrase(kind)` returning "in chat" / "by hand" and a single `tokensNotMeasured(kind)`, used by the page line and the findings.

### S2Q4 (major): `spikeHolderRefusal` repeats `spikeStateRefusal`, and the claim refusal ignores its `resume` argument

`spikes_claims.go:107-125` and `322-346`. Both begin with the same two sentences for "idea" and "ended or closed", so a future change to the wording must be made twice, and the tests (`wantExactRefusal`) pin both. `spikeClaims.refusal` (`:139`) takes `resume bool` and never uses it, as does `renewalRefusal`'s `tx` and `ctx`. `spikeDeadlineRefusal` and `spikeHolderRefusal`'s `judgeDeadline` flag are two ways of saying "judge the deadline", and the flag is only ever `true` or `false` by call site.

Fix: make `spikeHolderRefusal` call a shared `spikeStateWords(sp)` for the two state sentences, and have `spikeStateRefusal` call the same. Drop unused parameters where the interface allows, and note the interface's own signature where it does not.

### S2Q5 (major): claim loading and the claim's "latest" read are repeated for one page and one MCP result

A render of a spike's page reads the latest claim in `spikeExecutor` (`spikes_executor.go:36`), in `spikeClaimPanelFor` via `currentSpikeClaim` (`ui_spikes.go:329`) and again in `spikeClosedBySentence` through `spikeRunBy` (`spikes_executor.go:171`). `spikeResult` (`mcp_spike_tools.go:333-358`) reads it twice, once inside `spikeExecutor` and once for the `claim` object, and ignores both errors differently (the first falls back to a sentence with no claim; the second silently drops the object). `list_spikes` runs this per spike, so a page of twenty is forty claim reads and twenty execution reads.

Fix: read the executions and latest claim once into a small `spikeRunFacts` value and pass it to `spikeExecutorSentence`, `spikeRunBy` and the panel. `spikeExecutor` and `spikeRunBy` then take the facts instead of a `Querier`.

### S2Q6 (major): `EndSpike` validates twice, and `timeBoxedEnding` repeats `settleSpikeClaim`

`spikes_end.go:62-92` and `128-143`. `EndSpike` calls `checkSpikeHow` and `checkSpikeHowFor`, then `endSpikeLocked` calls both again, one read of the spike later. The first pair is only there to refuse before taking the locks, which a comment should say; as written it reads as a mistake. Separately, `timeBoxedEnding` (`:503-517`) decides "latest claim ended done means concluded" with its own claim read, and `settleSpikeClaim` (`:220-246`) decides the same inside the ending's transaction. If the rule changes (for example to include a submitted claim) the two will drift.

Fix: name the pre-lock check in a comment ("cheap refusal before the locks; endSpikeLocked checks again under them"), or drop the first pair. Give the rule one function, `claimEndedDone(c *store.Claim) bool`, used by both.

### S2Q7 (major): the Start screen drops its disabled state, and `CanStart` is now dead

`ui/templates/spike.html` (the start form's final button) lost `{{if not .CanStart}}disabled aria-describedby="start-runner"{{end}}`, and `ui_spikes.go:691` and `:752` still compute `startScreen.CanStart`, which no template reads. The `required` on the budget field was also dropped, so a person can post the form with an empty budget (handled server-side, but it was a deliberate stage 1 choice). `p.CanStart = !p.Started` was moved out of the `else`, so it no longer reflects whether a runner exists, which is what it was for.

Fix: delete the `CanStart` field and its assignment, or use it. Since the radio for the spike runner is disabled when there is no runner, state in a comment that the button is always enabled and refusal is by the server.

### S2Q8 (major): stale and misleading comments

- `integration_spikes_claims_test.go:3-5` says tests that need T3's ending "say so and skip until it exists"; there is no skip, and T3 is merged. `:611-612` says "DEPENDS ON T3 ... the last assertions are skipped"; nothing is skipped. Delete both.
- `claims.go:212`: "M13 registers task; M14 registers spike." is a release-note, not a description. Say "The registry by ref_type: task and spike."
- `integration_spikes_claims_test.go:75` comments `wantExactRefusal` as "wantRefusal checks an error ...", naming the other helper.
- `claims_task.go:337-345` and `claims.go` add three interface methods (`lock`, `rules`, `releaseConsequence`) with no doc comments on the task's implementations, while the spike's have them.
- `spikes_executor.go:3-6` says the file holds "the executor line, the time box line and the unmeasured line"; it also holds the closer sentence and `spikeRunBy`, which belong with FR-16.4, not FR-16.1. `unmeasuredExecutor` lives in `spikes_end.go:86` though it is used by nearly every file; move it here.
- `EndSpike`'s doc comment (`spikes_end.go:55-62`) has one line far past the others, from a hand edit; rewrap.
- `store/spikes.go` `EndSpikeState`'s comment was re-wrapped mid-sentence ("... note (FR-6.2 step 3). changed is false when / the spike wasn't running").

### S2Q9 (minor): an empty `if` branch holding only a comment

`ui_spikes.go` (the `spike.ended` case, `if unmeasuredExecutor(sp.Executor) { // Its tokens weren't measured ... } else if ...`). staticcheck flags an empty branch (SA9003), and it reads as unfinished. Invert: `if !unmeasuredExecutor(sp.Executor) { if b := ... }`, or compute the tokens sentence in a small function that returns "" for an unmeasured spike.

### S2Q10 (minor): interface methods that only return errors, and a question that is never asked

`spikes_claims.go:182-198` implements `prepareSubmit`, `onSubmit`, `prepareRelease` and `onRelease` solely to return `errSpikeOwnMethod`; `deadlineQuestion` (`:213-217`) is never reached because the sweep ends a spike before asking (`claim_sweep.go:67-74`) and its comment says as much. Five methods, 25 lines, exist to satisfy an interface. Following `renewalRefuser` and `deadlineEnder`, which are already optional interfaces, split `claimable` into the always-needed part and optional `submitter`, `releaser` and `deadlineAsker` parts, and type-assert where they are used. If the interface stays, a single embedded `noSubmitRelease` struct provides the five and removes the repetition.

### S2Q11 (minor): `spikeClaimFromForm` returns a value its caller already has

`ui_spikes.go:995-1017`. It returns `(sp, sp.PublicID, true)`, so each of four handlers destructures `sp` and `ref` where `sp.PublicID` is enough; it then calls `ClaimSpike(ctx, ref, ...)`, which resolves the same spike by public ID again. The task helper returns a ref because the task's ref is what it parsed; the spike helper parsed a UUID. `renderSpikeAgain` also re-reads the spike. Two reads of the same row per request.

Fix: return `(*store.Spike, bool)`. Optionally let `ClaimSpike` and the others take the spike's ID so the web routes do not round-trip through the public ID. The four handlers are otherwise the same shape; a small `spikeClaimAct(w, r, did, func(sp) error)` would collapse them.

### S2Q12 (minor): the start screen's executor choice is hard to read

`ui_spikes.go:752-768`. The default, the refusal override and the form override are three assignments with a four-clause condition: `form.Executor == chat || form.Executor == person || (form.Executor == agent && p.Refusal == "")`. `handleUISpikeStartPost` defaults an empty executor to agent, then `startScreenFor` handles the same empty value again. `renderStartScreen(..., form ...startForm)` is a variadic standing in for an optional argument.

Fix: a `chosenExecutor(form, refused bool) string` with a switch; pass `startForm` as a value and use the zero value.

### S2Q13 (minor): `spikeTimeLeft` is hard to follow

`spikes_claims.go:60-81`. The `mins == 0` case sits inside the minutes branch's condition (`m > 0 || len(parts) == 0`), and reads as a case of the wrong branch. Moving it before the loop, as `spikeLeftWords` does, fixes this and is another reason to merge them (S2Q2).

### S2Q14 (minor): `SubmitSpike`'s recheck is convoluted

`spikes_claims.go:535-545`. The block `if locked.State != running || held == nil || held.ID != cur.ID { if r := spikeHolderRefusal(...); r != "" && locked.State == running { return r }; return timeBoxEnded }` then calls `spikeHolderRefusal` again two lines later with the same arguments. It also tells a person that "the time box ended before this arrived" when the spike was in fact closed or ended by something else (for example a person's release followed by an end). Compute the refusal once; say the time-box sentence only when the ending was the time box (`locked.EndedHow == time_box`).

### S2Q15 (minor): test helpers are duplicated across the new test files

`integration_spikes_claims_test.go:21-83` defines `startedSpike`, `chatSpike`, `claimSpike`, `latestSpikeClaim`, `pastDeadline` and `wantExactRefusal`; `integration_spikes_timebox_test.go:21-90` defines `tbStart`, `tbClaim`, `tbLatestClaim` and `tbPastDeadline` doing the same. `tbPastDeadline` repeats `pastDeadline` with two raw `Pool.Exec` calls where the harness already has `h.exec` (`integration_claims_test.go:84`). `tbLatestClaim` is `latestSpikeClaim` without the spike's ID in the message. The `tb` prefix is a generic name for a helper on `harness`, and the repeated `uuid.NewString()[:4]` and `[:6]` initiative names differ for no reason. `integration_spikes_ui_stage2_test.go:18` adds `spikeForm`, and `integration_spikes_mcp_test.go:` `jsonText`, which `integration_mcp_test.go` may already offer. `wantExactRefusal` sits beside the existing `wantRefusal` (`integration_claims_test.go:180`, a substring match) with a name that does not say how it differs.

Fix: one file, `integration_spikes_helpers_test.go`, with a single set (`startedSpike(executor, hours, who)`, `latestSpikeClaim`, `pastDeadline`, `claimedBy`), named for what they do; use `h.exec`. Rename `wantExactRefusal` to `wantRefusalSentence`.

### S2Q16 (minor): tests that assert by string where a field exists, and one pure test in an integration file

`TestSpikeExecutorSentences` and `TestSpikeTimeBoxAndUnmeasuredLines` (`integration_spikes_timebox_test.go:409-480`) are table tests of pure functions (good) but live in a file and under a name for integration tests that need the database; they build a `time.Local` date and compare "17:04" in whatever zone the machine has, which works only because the function also uses Local (see S2Q2; once the zone is fixed the test must set it). `TestSpikeLocksAreTakenInOrder` (`integration_spike_executors_test.go:161-199`) does not check an order: it takes the locks in the right order once and asserts there is no deadlock inside twenty seconds, so a wrong order in the code would pass unless two goroutines raced. Rename to `TestSpikeEndingUnderItsLocksDoesNotDeadlock`, or have two goroutines take them in the two orders and assert which one the code uses. Many page tests check `strings.Contains` on a whole page for phrases (this is how S2Q1 escaped); where a field or element id exists (`#spike-claim-mine`, `#spike-timebox-line`) read that element's text and compare it whole.

### S2Q17 (minor): markup consistency and accessibility in `spike.html`

- The start screen has two headings for one thing: the panel title "Who runs it" and a `<legend>` "Who runs it" immediately inside it (`spike.html`, `#start-executors`). Screen readers say it twice. Drop the panel title, or make the legend the panel's only heading.
- `<label class="radio-row" for="start-ex-agent">` wraps its input and also has `for`; keep one. The "agent" radio is `disabled` when there is no runner while its hint, which gives the reason, is described by `aria-describedby`: good, but the refusal text also appears again in the "The spike runner" panel (`#start-runner`), so the reason is said twice on one screen.
- The two limit fields are hidden by script only; without script both show (the comment says so), but the labels "(for the spike runner)" and "(for the chat agent or you)" then sit on an optional field whose value is ignored. Say in the hint which one is read.
- `<button ... aria-describedby="again-budget-hint">` and `<p id="again-budget-hint">` now carry a budget id for a form with no budget field in the person case; rename the id to `again-hint`. The two hint texts are hand-duplicated inside one `{{if}}` on one line; split into two `<p>` blocks under the `if`.
- The release forms put a `<p>` after the button inside the `<form>` in one place and outside it in another (`held` against `mine`); pick one.
- Buttons use `btn--primary` for "I'll run this spike" and "I've finished"; "I've finished" and "Save findings" share a `<form>` and rely on `formaction`. That works, but a screen-reader user hears two submit buttons for one form with no group label; add `aria-describedby` to the finish button saying it ends the spike.

### S2Q18 (minor): spelling and wording in user-facing sentences

British spelling is held throughout, with one exception for consistency: "Whoever is running it ... last saved this" and "No one has saved any findings yet" do not match the agent case's "The agent hasn't saved any findings yet" in tense or person; make the three sentences parallel. "I'll run this spike" and "I've finished" speak in the person's voice, while every other button on the page is an imperative ("Release the claim", "Start this spike"); keep one voice. `spikeStateRefusal` says "is run by the spike runner, an agent, so it can't be claimed" while the rest of the codebase calls it "the spike runner" alone; drop ", an agent".

### S2Q19 (minor): small naming and structure points

- `store.CloseSpike` is now a one-line wrapper called by nothing in the repository but its own tests; `CloseSpikeRanIt` is the name that says the least. Make `CloseSpike` take the flag (a `SpikeCloseOpts` struct if more are coming) and update the tests, rather than keep two functions.
- `SpikeStartRequest.Budget` and `.TimeBoxHours` both use 0 for "the default" while `startSpike` also validates ranges for the same fields in two places (`StartSpike` and `parseTimeBoxField`, `parseBudgetField`); the web routes' parse functions can return the request's fields already validated and `startSpike` keeps the single check, or the reverse, but not both.
- `ErrSpikeTimeBox` is an error value with a capitalised sentence ending in a full stop, which the codebase's lints avoid for ordinary errors; it matches the stage 1 `ErrSpikeStarted` style, so this is only worth noting if the rest are changed.
- `spikeClaims{s *Server}` methods mix value receivers (`sc`) and a registry that builds a new value each call; fine, but `spikeClaimRules()` on `Server` and `rules()` on the claimable are two names for one thing (`rules()` returns `sc.s.spikeClaimRules()`). Call one from the other, or delete `spikeClaimRules`.

## Summary

The package is correct to the extent the tests show, and `gofmt` and `go vet` are clean. The cost of building in parallel is in the seams: duplicated time and executor wording (S2Q2, S2Q3), repeated refusal sentences (S2Q4), repeated claim reads (S2Q5), duplicated test helpers (S2Q15), and one visible defect in the template (S2Q1). The first three majors, S2Q1 to S2Q3, are worth fixing before merge; S2Q4 to S2Q8 are worth a pass in the same round.
