# REVIEW-021, stage 2 code, round 3: bugs and spec conformance

Date: 2026-10-02

## Scope

The whole stage 2 code of SPEC-021 §4 (M14), `git diff 36b7fed..e3f5a29 -- . ':!docs'`, on branch `claude/subutai-m14-stage2-spikes`, with special attention to the round 2 fixes, `git diff c039a0a..e3f5a29` (commit ab981c3). It was read against SPEC-021 §4 (as revised in ab981c3), DEC-006 Amendment 1 and DEC-007, round 2's two reviews and the walkthrough's findings (`docs/walkthrough-spec-021-stage2.md`, "What the walkthrough found").

In a private worktree at e3f5a29 and a private Postgres, `go vet ./...` is clean, `gofmt -l internal/` prints nothing, and `go test -race -count=3 -run 'Spike|Claim' ./internal/server/` passes (322 s). The five tests added or changed for the round 2 fixes also pass `-race -count=6 -cpu 1`. With `ClaimSpike`'s old lock path restored as a throwaway change, `TestAClaimOvertakenByAStartWaitsForTheStartsWorktree` fails ("the claim took no lock on the worktree"), so it tests the fix. The change was then reverted.

Severity, as before: **major** is wrong behaviour, a race that can corrupt state or deadlock, a spec requirement not met, or a security hole. **minor** is everything else worth fixing.

## Round 2's majors, and the walkthrough's W1 and W2

| Finding | Status | Evidence |
|---|---|---|
| S2B2-1: a claim that races the start makes the worktree without its lock | **Fixed** | `ClaimSpike` (`spikes_claims.go:443-447`) now locks `s.worktreeAbs(s.spikeWorktreeRel(sp.ID))` whatever the first read saw. That is the same key the start (`makeStartedSpikeWorktree`, `spikes.go:350-351`, where `WorktreePath` was set to `spikeWorktreeRel(sp.ID)` at `:295`), every ending (`withSpikeEndLocks`), the sweep and the release take, since all go through `worktreeAbs` and `filepath.Clean`. Both orders are now safe. If the claim locks first and reads an idea, it is refused "isn't running". If it reads a running spike, it makes the worktree as a first make, because `spike.worktree_made` isn't recorded yet, so no leak check runs. The start's make then finds `.git` and only records. If the start locks first, the claim finds `.git` and makes nothing. `TestAClaimOvertakenByAStartWaitsForTheStartsWorktree` covers it, and fails without the fix. |
| S2Q2-1: Start's `disabled` branch unreachable | **Fixed** | `CanStart` now means "nothing can start": `!p.Started && p.TemplateRefusal == ""` (`ui_spikes.go:769-770`). A refused spike runner no longer touches it. The button's `aria-describedby="start-template"` points at the paragraph that `{{if .TemplateRefusal}}` renders (`spike.html:480-485`), which exists whenever the button is disabled. `startSpike` refuses with the same sentences through the shared `findingsTemplateRefusal` (`spikes.go:257`, `:336-344`), so the screen and the POST agree. `TestSpikeStartIsDisabledWithoutAFindingsTemplate` covers it. The stage 1 test now asserts the agent radio by its tag (`radioTag`) and the button by its own (`startButton`). |
| S2Q2-2 and S2B2-2: race tests ordered by sleeps | **Fixed** | `Server.spikeHook` (`server.go:65-69`) is called through `atSpikePoint` (`spikes_claims.go:401-407`) at `claim-read`, `claim-locked` and `submit-checked`. In `TestAClaimThatWaitedOnAnEndingMakesNothing` the claim's first read is awaited while the ending holds both locks, so that read is shown to be stale. In `TestASubmitThatLosesToTheTimeBoxIsRefused` the submit is awaited past step 1 before the ending is released, so it can't meet "has ended" instead. Both are now deterministic. |
| S2B2-3: FR-12.4, FR-15.2 and the comment said "latest claim" | **Fixed** | Both requirements and `ReconcileSpikes`'s comment (`spikes_end.go:420-423`) now say any claim. The pass-through `spikeSubmitted` is gone, and its three callers call `store.ClaimEndedDoneFor` directly. |
| W1: **Ask again** with no field drawn over its hint | **Fixed** | The template now has two forms. The fieldless one uses `.triage-no-field` (`spike.html:294-301`, `app.css:2319`), a flex column with the button above the hint. Each form has exactly one `#again-hint`, because only one renders. `TestPersonRunsASpikeThroughTheRoutes` asserts the class, and the walkthrough's `06b-ask-again-fixed.png` shows it. |
| W2: "run" wording on a chat or person spike; capital after a colon | **Fixed** | `spikeStateSentence` says "This spike has ended: …" for an unmeasured executor (`ui_spikes.go:286-292`). The draft panel says "If the time box ends now" (`spike.html:238`), matching what `save_spike_findings` returns. The time-box phrase is "it reached its time box" (`store/spike_endings.go:43`), so the page, the moment ("Ended: it reached its time box.") and the timeline ("Spike … ended: it reached its time box") read correctly. The worktree line says "the spike's ending". `TestChatSpikePageSaysNothingOfARun` covers it. See S2B3-1 for the spec text this left behind. |

## Findings

### S2B3-1 (minor): FR-15.4 still quotes the time box's timeline phrase as "Reached its time box"

`docs/specs/SPEC-021-spikes.md:1490`; `internal/store/spike_endings.go:43`.

W2's fix changed `spikeEndings[time_box].Phrase` to "it reached its time box", in line with the other endings' phrases, which all follow "ended: ". FR-15.4 still says the `time_box` entry's timeline phrase is ("Reached its time box"). The spec and the code now disagree, and a check of FR-15.4 against the code would flag the right behaviour as wrong, or "fix" it back to the capitalised phrase that W2 removed.

Fix: in FR-15.4, change the quoted phrase to ("it reached its time box").

No other new problems were found.

## Checked and found right

- **The `spikeHook` seam.** It is nil in production: nothing outside the tests assigns it, and `atSpikePoint` checks for nil before calling. It is a plain field with no synchronisation. That is race-free, because each test assigns it before the `go` statement that starts the claim or submit, and the only callers (`ClaimSpike`, `SubmitSpike`) are reached from MCP and UI handlers, never from the heartbeat, the sweep or the dispatcher. The race detector agrees over `-count=3` and `-count=6 -cpu 1`. Only `claim-locked` fires under a lock (the worktree's). The one hook that handles it sends on a channel buffered to one, from the only claim in its test, so it can't block while holding the lock. The `claim-read` hook that blocks on `resume` fires before any lock is taken. A failing test can leak a goroutine but can't hang the run, because no leaked goroutine holds a pool connection.
- **Lock order (SD-27), traced again for every path.**
  - `ClaimSpike`: worktree lock, then pool reads (`sc.load`, `spikeBarredRefusal`), then `ensureSpikeWorktree`. That reads `spike.worktree_made` on the pool, takes `spikeEndMu` only around `spikeKept`, and records the audit row in a short transaction of its own. Then `claimLocked`, whose transaction locks the spike row, then the claim. Nothing in it retakes the worktree lock or holds a transaction while taking an in-process lock.
  - `SubmitSpike`: step 1 on the pool, then `withSpikeEndLocks` (worktree lock, then `spikeEndMu`), the step 4 transaction (rows), and `endSpikeLocked` with neither lock taken again.
  - `EndSpike`, the rules' `endSpikeAction`, the sweep's `endAtDeadline` (called outside `sweepFingerprint`'s lock, `claim_sweep.go:73` against `:80`) and reconciliation's `finishSpikeEnding` each take the locks once, from callers that hold none.
  - `makeStartedSpikeWorktree` takes only the worktree lock, after the start's transaction has committed.
  - `ReleaseClaim` and `sweepFingerprint` on a spike take only the worktree lock, on `t.Path`, which is the same key.

  No path takes `spikeEndMu` before the worktree lock, or either in-process lock inside a transaction.
- **The start's make after a claim's.** `makeStartedSpikeWorktree` makes from the start's own row without reading it again. Now that the claim waits on the same lock, a claim can make the worktree first. The start's make then finds `.git` and only records `spike.worktree_made` (`spikes_end.go:383-385`), which is idempotent in effect, because the read is `EXISTS`. For it to remake the worktree of a spike that has ended, a whole claim, save and submit would have to finish between the claim releasing the lock and the waiting start taking it. Even then, reconciliation removes a spike directory that no running spike names. This is not a practical failure.
- **`findingsTemplateRefusal`.** It is the old two checks moved verbatim, with the same sentences. `startSpike` keeps its order: the runner's refusal first for an agent start, then the template. The start screen calls it on each render, which costs a file read and a manifest load and is harmless.
- **`CloseSpike`'s `closerRanIt`.** The run facts are now read only for a person's spike. `closerRanIt` returns false for any other executor, so the result is unchanged and a read is saved.
- **The new wording sites.**
  - `spikeBoxSentence` is the one time-box sentence. `spikeTimeLeft` (MCP's `time_left`) still matches FR-13.4's example word for word.
  - The claim panel's line now uses that sentence.
  - `spikeTimeBoxLine` says "ended at" rather than "ending at … (no time left)" once the deadline has passed.
  - `spikeHolderRefusal`'s table gives the same four sentences as before ("there are no findings to save" / "its findings"; "there is nothing to submit" / "it").
  - The start screen's hint, "the reason is in the spike runner's panel", points at a panel that is shown.
  - `spikeResult` now logs a failed read of the run facts rather than dropping it.
- **The CSS.** `.triage-no-field` is a flex column with `align-items: flex-start`, so the button keeps its own width and border, and the hint wraps below it. The 640 px media rule only narrows `.triage-with-field`, and the new class needs no narrow-screen rule.
- **The boundary (DEC-006 Amendment 1, DEC-007, FR-17).** ab981c3 adds no MCP tool and no route; `mcp_spike_tools.go` gains only a log line. Over MCP the chat agent still has only `claim_spike`, `save_spike_findings` and `submit_spike` for running a spike. Start, release and close stay in the web UI, and `close_spike` over MCP is still method-not-found (`TestChatSpikeEndToEnd`).
- **M13 and agent spikes.** The task claim path, the dispatcher, the planner's `ensureSpikeWorktree` and the agent leak-check predicate are untouched. An agent spike's claim now takes its worktree's lock before being refused for its executor. It makes nothing under that lock, so it can't disturb the planner.
