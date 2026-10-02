# M14 (spikes), round 3: bugs and spec-conformance review

Branch `claude/subutai-m14-spikes` at `4a565d3`. Fix diff reviewed: `00e918e..HEAD`, plus a final pass over `git diff main...HEAD -- internal cmd`. Contract: SPEC-021, with FR-6.3 as amended.

**Verdict: no major defects remain.** Round-2 finding 1 is fixed, and the fix holds up. Every other round-2 minor is either fixed or written into FR-6.3 as an accepted limit, except 7 (findings on disk for a TODO question) and 8 (tokens added after the end). I found three new minor issues.

How I checked:
- `go vet ./...` is clean.
- `SUBUTAI_TEST_DB_SUFFIX=_rev3 go test -race -count=1 ./...` passes in every package. The server package took 182s, so the integration tests ran.
- I tested `rev-list --stdin` with `^sha` lines, including a negated blob tag and a missing object, using plain git in the scratchpad (now deleted).
- I edited no repository files. `git status` is **not** clean, but not because of this review. Another session is running `docs/walkthrough-spec-021/walk.js` (pids 19295 and 20000, started 12:06) and is rewriting `docs/walkthrough-spec-021/*.png`. I left those files alone.

## New findings

### N1. minor: the retry-time record hides leaks made after the working copy is remade
**Where:**
- `internal/server/spikes_leak.go:68-70`: `spikeKept` returns any earlier `spike.code_kept` record unchanged.
- `spikes_end.go:247-254`: the check that runs before the remake.

**What happens:**
1. A retry finds a used working copy damaged. The check finds a leak (say `keep-1`) and records it.
2. The worktree is remade. Its old reflog is pruned and a new one starts.
3. In the new working copy, the agent commits and branches `keep-2`.
4. At the end of the run, `spikeKept` finds the earlier record and returns it without checking again.

The findings and the checkpoint name only `keep-1`. A person is still warned that code was kept, so this is not fail-open, but the list of refs is incomplete. The same short-circuit happens when the retry check could not run at all.

There is a second, more contrived gap. When the retry check finds nothing, nothing is recorded, and the old reflog is pruned anyway. Commits made before the remake can then be put on a ref by their SHA from the new working copy, and nothing reports them. That needs deliberate evasion, close to the limits FR-6.3 already accepts.

**Fix:** at the end of the run, always run the check against the current working copy. Merge its refs with the earlier record's, and do not raise a second checkpoint while one is still pending; `CreateCheckpoint` is already idempotent for that. Alternatively, before the remake, add the old reflog's "made" SHAs to a stored set that the end-of-run check also uses.

### N2. minor: the check before a remake doesn't re-read the spike under `spikeEndMu`, so a racing ending can raise a false "couldn't check" checkpoint
**Where:** `spikes_end.go:247-251`. `sp` was read by `planSpike` (`spikes_plan.go:101-108`) before the lock was taken.

**What happens:**
1. An attempt is given up while it is still in `planSpike`, for example after a stall during planning.
2. `DispatchExhausted` triggers `EndSpike`. That runs the check, finds nothing, so records nothing, then discards the working copy and its admin dir.
3. The old goroutine then sees no `.git` and calls `spikeKept`. There is no earlier record, and git has no admin dir.
4. Because `TokensUsed > 0`, the check reports "the working copy was removed". It writes an audit row and raises a `spike-code-kept` checkpoint on a spike that has already ended cleanly.
5. It then remakes a worktree for the ended spike. The leftover sweep removes that on the next heartbeat. The plan itself is stopped by `stillCurrent` before any model call.

The window is narrow, and the race already existed for the remake. The spurious checkpoint is new.

**Deadlock:** none. `ensureSpikeWorktree` is called only from `planSpike` and from tests. `planSpike` is never reached while `spikeEndMu` is held: `EndSpike`, `recordSpikeEnd`, `finishSpikeEnding` and the reconcile `finish` closure don't plan or dispatch synchronously. Go's mutex is not re-entrant, but nothing re-enters it.

**Fix:** inside the lock, run `cur, err := store.GetSpike(...)`. If `cur.State != store.SpikeRunning`, return an error ("… is not running") before running the check or the remake. Pass `cur` to `spikeKept`, so `TokensUsed` is current too.

### N3. minor (nit): `escapeLeadingHash` handles only `#`
**Where:** `spikes.go:639-646`.

**What happens:** a question that begins with `- `, `* `, `+ `, `> `, `1. `, `|` or `===` still renders as a list, a quote, a table or a setext underline inside the Question section. `#` was the case actually reported, and the fix is correct for it. The title line `# # of workers…` renders correctly.

**Fix:** escape the first character when it is one of the Markdown block starters, or accept the behaviour.

## Round-2 finding 1: the fix is sound

The `?did=` change and `escapeLeadingHash` were part of this item's diff. Their verdicts are the round-2 table's rows 6 and 7 and new finding N3.

- **`leakedRefs` fails closed** (`spikes_leak.go:131-142`).
  - A missing admin dir plus a missing directory plus `TokensUsed > 0` gives "the working copy was removed, so git's record of it is gone".
  - `TokensUsed > 0` is a sound signal. `runLoop` calls `b.Add` (which runs `AddSpikeTokens`) after every model call and before any tool runs (`dispatch.go:481-487` and `:530`), and the planner makes the worktree before the first call. No tool can run in a working copy while `tokens_used` is still 0.
  - One caveat: the signal relies on the provider reporting usage. Only the Anthropic provider exists, and it always does.
  - `EndSpike` re-reads the spike under the lock (`spikes_end.go:71`), so `TokensUsed` is current there.
  - Tested by "a working copy deleted and pruned away", including the `couldnt_check` payload.
- **The check before a remake** (`spikes_end.go:247-254`). It runs only when `.git` is missing and tokens were used.
  - If the `.git` file was removed but the admin dir survives, `worktreeAdminDir` still finds it through `gitdir`, so the reflog is read before `prune` deletes it.
  - Tested by "a damaged working copy that a retry would remake": one audit row and one checkpoint.
  - No deadlock. There is a race, which is N2. The incomplete record is N1.
- **De-duplication.** `spikeKept` returns the earlier `SpikeKeptRecord`, so the end of the run writes neither a second audit row nor a second checkpoint.
  - `CreateCheckpoint` was **not changed** by M14. `ON CONFLICT … DO NOTHING` returning `nil, nil` is how it already behaved on main. Only the comment at `spikes_leak.go:99` is new.
  - `notifyCheckpointRaised(nil)` is a no-op (`ui.go:1055`).
  - No other kind of checkpoint is affected.
- **`gitInWithStdin` and `rev-list --stdin`.** Lines of SHAs and `^sha` give the same set as `<made…> --not base refs…`.
  - Verified with plain git. A `^` on an annotated tag or a blob is accepted.
  - A start-ref object that has since been garbage-collected gives "bad object", and the check fails closed. That was already true when the SHAs were passed as arguments.
  - Stderr is captured and quoted in the reason.
- **Payload.** `refs` is now `[]`, never `null`, and `couldnt_check` is added. The test asserts both.
- **`Run` field and `observe.go` `conclude`.**
  - The change is inside `if d.Purpose == "run-spike"`, and runs only after the `failed`/`cancelled`/`queued`/`running` and `!IsLatest` returns. Non-spike run pages are unchanged.
  - The concluded, budget and turn-limit sentences are word-for-word as before, matching FR-8.4.
  - New: a succeeded run-spike dispatch whose spike ended `failed`, or ended in an unknown way, now says "Its run failed." or "Its run is over." Before, it fell through to the outcome JSON. This is harmless, and more accurate.

## Verification table: round 2's findings

| # | Sev | Finding | Status | Evidence |
|---|---|---|---|---|
| 1 | major | Fails open when the working copy is deleted and pruned, or remade on a retry | **Fixed** | `spikes_leak.go:131-142`, `spikes_end.go:247-254`, FR-6.3 amended, two new subtests. Residuals: N1 and N2 (minor). |
| 2 | minor | Reflog-message spoof; `commit-tree` / `update-ref` | **Accepted limit** | FR-6.3 "What the check can't see", bullet 2. |
| 3 | minor | False positive on merge, rebase or pull of a newer main | **Accepted limit** | FR-6.3 "It may also report too much…". The checkpoint is dismissable. |
| 4 | minor | `core.logAllRefUpdates=false` means every spike reports "couldn't check" | **Accepted limit** | FR-6.3 bullet 3 ("the safe direction"). |
| 5 | minor | Start-ref SHAs on the command line (E2BIG) | **Fixed** | `gitInWithStdin` and `rev-list --stdin` (`planner.go:482-495`, `spikes_leak.go:160-165`). One `rev-list` is still spawned per changed ref (`:187`), which is fine at normal scale. |
| 6 | minor | A crafted `?did=` shows a false notice | **Fixed** | `answered`/`unanswered` compare `ClosedAs`; `started` needs `StartedAt` (`ui_spikes.go:461-470`); `TestSpikeNoticeSaysOnlyWhatHappened`. `did=started` on an ended or closed spike that did run still says "has started, with a budget of …". That is true, so I accept it. |
| 7 | minor | Findings for a TODO or `{{` question don't validate on disk; leading `#` | **Partly fixed** | The leading `#` is escaped and tested. The TODO/`{{` file on disk still fails `lifecycle.Validate`. FR-2.1 doesn't accept this, and the manifest doesn't exempt the Question. **Open.** |
| 8 | minor | `AddSpikeTokens` adds to an ended spike; the page note doesn't explain it | **Not fixed** | `store/spikes.go:297-305` is unchanged, and so is the TokensNote at `ui_spikes.go:302`. **Open.** |
| 9 | minor | Checkpoint payload `"refs": null`, no `couldnt_check` | **Fixed** | `spikes_leak.go:86-92`; asserted in a test. |
| 10a | minor | Dead `OpenSpikesUnderInitiative` | **Fixed** | Deleted, and its test is reduced to `TestEndedSpikesCount`. |
| 10b | minor | `src == "override"` | **Fixed** | `store.BudgetFromOverride` and `BudgetFromDefault` (`ui_spikes.go:306`, `:595`). |
| 10c | minor | `conclude` has its own switch | **Fixed** | `SpikeEndingOf(...).Run`. |
| 10d | minor | MCP's own budget-source words | **Fixed** | `spikeBudgetSourceWords` (`mcp_spike_tools.go:194-199`). |
| 10e | minor | `spikeOwnerRef` branches on `FeatureID` | **Fixed** | It uses `sp.Owner()` and carries `URL` (`ui_spikes.go:109-125`). |
| 10f | minor | The POSTs answer a bad id with 400 | **Resolved** | Unknown id gives 404 through `notFoundOrErr`, malformed gives 400; `TestSpikePostsAnswerBadIDs`. |

## Final adversarial pass (`main...HEAD -- internal cmd`): no new major defects

- **The hard stop** (`dispatch.go:452-560`).
  - Before every call, the run stops when `spent >= Limit`, or after turn 0 when `spent + lastCall > Limit`.
  - Every paid call is added before its tools run. On the crossing turn, only `EarlyTools` run. An outcome on the crossing turn concludes, as FR-5 says. A failed validation on the crossing turn then stops.
  - A nudge on the last turn is still a budget stop.
  - Known and accepted: a call can overshoot the budget by up to its own size, and a retried attempt's first call has no `lastCall` estimate (`turn > 0`). Both are bounded by `spent >= Limit`.
- **No-merge path.**
  - The worktree is made `--detach`, so it has no branch.
  - `isWorktreePurpose` adds `run-spike` only for profile tools.
  - `decideDispatchSucceeded` and `DispatchExhausted` route `run-spike` to the spike rules before anything in phase 2, so no merge or verification action can fire.
  - Milestone membership refuses spikes in the API, MCP and UI forms. `resolveOwner` and `mcpResolveOwner` refuse attaching documents to a spike.
- **Start and close only from the web UI.**
  - The only callers of `StartSpike` and `CloseSpike` are `ui_spikes.go:684` and `:748`.
  - No `/api` route or MCP tool starts or closes a spike.
  - The MCP `initialize` text says so.
- **Regressions to features and bugs.**
  - `FeatureHistory` adds only `spike.started` and `spike.ended` (`WHERE e.ref_type <> 'spike' OR …`). `FeatureRuns` and the token totals are untouched.
  - The timeline `CurrentIndex` and aside logic leave the current moment of a feature without spikes where it was. The feature and bug timeline tests pass.
  - G5 adds open spike IDs through `G5Open`, on both archive routes.
  - The changes in `groupThousands` and `memberFromForm` keep their behaviour.
  - `answerOptions` adds only the `spike-code-kept` kind.
  - The full suite passes.

## Minor findings still open across all rounds (bug-report candidates)

1. **N1:** the record made before a remake is final, so leaks made after the remake aren't named. A clean pre-remake check also drops the old reflog's commits (`spikes_leak.go:68-70`, `spikes_end.go:247-254`).
2. **N2:** the check before a remake doesn't re-read the spike's state under `spikeEndMu`, so a racing `EndSpike` can raise a spurious "couldn't check" checkpoint on an ended spike, and a worktree is remade for it (`spikes_end.go:247-251`).
3. **N3:** only a leading `#` is escaped in the findings' Question. Other Markdown block starters still render as blocks (`spikes.go:639-646`).
4. **R2-7 / R1-1 residual:** the findings file on disk for a question containing TODO or `{{` fails `lifecycle.Validate`. Either accept this in FR-2.1, or exempt the Question section in the findings manifest.
5. **R2-8 / R1-9:** `AddSpikeTokens` still adds to an ended spike (`store/spikes.go:297-305`), so the page's total can differ from the findings. Either guard the update with `AND state = 'running'`, or extend the TokensNote.
6. **R2-5 residual (scale):** one `rev-list` is spawned per changed ref (`spikes_leak.go:187`). Fine today; batch it if repositories with many moving refs matter.
7. **Quality-round leftover:** the `StartScreen.Workers/Running/Free` trio is kept. It was optional.

These are accepted limits written into FR-6.3, not bugs: the reflog-message spoof and `commit-tree`/`update-ref`; commits made outside the worktree; `core.logAllRefUpdates=false`; and the false positive when the agent merges a newer main.
