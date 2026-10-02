# M14 (spikes), round 2: bugs and spec-conformance review

Branch `claude/subutai-m14-spikes` at `00e918e`. Fix diff reviewed: `20be941..HEAD`. Contract: SPEC-021, with FR-6.3 and FR-10.3 as amended.

How I checked:
- `go vet ./...` is clean.
- `SUBUTAI_TEST_DB_SUFFIX=_rev2 go test -race -count=1 ./...` passes in every package. The server package took 197s, so the integration tests ran.
- I reproduced the leak-check gaps with plain git in the scratchpad.
- I also ran two throwaway server tests and then deleted them. One drove the leak check through `runLeakScript` with four new scripts; the other built findings for a question that begins with `#`. `git status` is clean.

## Findings

### 1. major: if the agent deletes its own working copy and anything then runs `git worktree prune`, the leak check fails open
**Where:**
- `internal/server/spikes_leak.go:124-127`
- the prunes at `spikes_end.go:216` (`removeSpikeDir`, run by every other spike's discard), `spikes_end.go:248` (`ensureSpikeWorktree`) and `worktree_ops.go:148` (every feature worktree removal).

**What is wrong:** when git has no admin dir for the worktree and the directory is missing, `leakedRefs` returns "nothing kept" with no error. Two cases reach that state:
- The agent removes its worktree directory, then any prune runs before the spike ends (another spike ending is enough). Git then deletes `.git/worktrees/<name>`, reflog included.
- The agent removes only the worktree's `.git` file, and a retry follows. `ensureSpikeWorktree` sees no `.git`, runs RemoveAll and prune, and makes a fresh worktree with a fresh reflog. FR-6.3's "a damaged `.git` file in the worktree doesn't hide it" fails here.

FR-6.3 says that when the reflog can't be read, the check fails closed.

**Evidence:** a throwaway subtest ran the script `c a && git branch keep-gone && d="$PWD" && cd / && rm -rf "$d" && git -C <root> worktree prune`. Result: `audit=0 cps=0`, and the findings say nothing. Branch `keep-gone` holds the commit. Plain git confirms that prune removes the admin dir.

**Fix:**
- `EndSpike` only checks a spike that is still `running` (`spikes_end.go:75`). So when no admin dir is found, "never made" is the only benign case. Fail closed when the planner has made the worktree. `sp.TokensUsed > 0` is a sufficient signal, or record a `worktree_made_at` in `ensureSpikeWorktree`.
- In `ensureSpikeWorktree`, when a worktree has to be made again for a running spike that already used tokens, record that (or run the leak check first) instead of silently pruning the old reflog.
- Add a test.

### 2. minor: the reflog filter trusts text the agent controls, and commits made without moving HEAD are invisible
**Where:** `spikes_leak.go:285-289` (the `checkout:` / `reset:` skip), and the reliance on HEAD's reflog in general.

**What is wrong:**
- The reflog message comes from `GIT_REFLOG_ACTION` (or `update-ref -m`). With `export GIT_REFLOG_ACTION=checkout`, every commit is logged as `checkout: …` and is skipped.
- `git commit-tree` plus `git update-ref refs/heads/x` never touches HEAD's reflog.

**Evidence:** throwaway subtests, both with `audit=0 cps=0`:
- `export GIT_REFLOG_ACTION=checkout; c a && git branch keep-spoof && git checkout -q --detach HEAD~1`: no leak reported.
- `t=$(git commit-tree -p HEAD -m sneaky $(git write-tree)) && git update-ref refs/heads/keep-ct $t`: no leak reported.

Both are deliberate evasions. FR-6.3's stated limit covers only code committed outside the worktree.

**Fix (either):**
- Strengthen the check. Don't use the message to drop an entry. Drop a checkout or reset target only when it is reachable from a ref that existed in `refs_at_start`, at its current tip. Also report any **new** ref that holds a commit not reachable from `base_commit`, from `refs_at_start`, or from the current tips of refs that existed at the start. That catches `commit-tree` onto a new branch.
- Or state these limits in FR-6.3 next to the DEC-007 sentence.

### 3. minor: a false positive when the agent merges, rebases or pulls a `main` that moved
**Where:** `spikes_leak.go:287`. Only `checkout:` and `reset:` entries are skipped.

**What is wrong:** `merge X: Fast-forward`, `rebase (start): checkout X`, `pull: Fast-forward` and similar entries name commits made elsewhere. They are taken as made in the worktree, so `main` is reported as kept code with a checkpoint. FR-6.3 says that a `main` which moved without the worktree's commits isn't a leak.

**Evidence:** a throwaway subtest committed on main from outside the worktree, then ran `git merge --ff-only <main>` in the worktree. The findings said "Code from this spike was kept on `master`", with one audit row and one checkpoint.

**Fix:** this is the same fix as finding 2. Decide "made here" by reachability from the start refs and the current tips of refs that existed at the start, not by the reflog message. Add the merge case to `TestSpikeHasNoMergePath`.

### 4. minor: with `core.logAllRefUpdates=false`, every spike reports "couldn't check"
**Where:** `spikes_leak.go:271-274`.

**What is wrong:** a repository with `core.logAllRefUpdates=false` gets no `logs/HEAD` from `git worktree add`. Every spike then fails closed with "the working copy's HEAD reflog couldn't be read", and a checkpoint is raised. I verified the missing file with git 2.43.

**Fix (either):**
- Create the log yourself: run `git -C <wt> update-ref --create-reflog -m 'subutai: spike start' HEAD <base>` after `worktree add`, or touch `logs/HEAD`.
- Or set `-c core.logAllRefUpdates=always` on the worktree (`git -C wt config --worktree`), and say so in the start-up checks.

### 5. minor: the leak check passes every start ref's SHA on the command line
**Where:** `spikes_leak.go:147-151`.

**What is wrong:** each start ref adds about 41 bytes to `rev-list … --not base <every sha in refs_at_start>`. A repository with tens of thousands of refs (remote branches, tags) passes Linux's `ARG_MAX` of about 2 MB and gets E2BIG. The check then fails closed for every spike that committed anything. Separately, a `rev-list` is spawned once per changed ref (`:171`). That is fine at normal scale.

**Fix:** feed the SHAs through `git rev-list --stdin`. Deduplicating the SHAs first also helps.

### 6. minor: a crafted `?did=` can show a false notice
**Where:** `internal/server/ui_spikes.go:457-467` (`spikeNoticeFor`).

**What is wrong:**
- `did=answered` checks only `State == closed`. On a spike closed as unanswered, `/ui/s/SPK-003?did=answered` says "SPK-003 is closed: the question is answered.", and `did=unanswered` does the reverse.
- `did=started` on an ended or closed spike says it "has started".

There is no open redirect: the target is built from the server-side `PublicID`, and the notice text is server-built and escaped.

**Fix:**
- Compare `sp.ClosedAs` with `store.SpikeAnswered` / `store.SpikeUnanswered`.
- Show `started` only while `State == running` (or not `idea`).

### 7. minor: findings for a TODO or `{{` question are written in a form that doesn't validate
**Where:** `spikes.go:653-658` and `:694-701`.

**What is wrong:** the fix for round-1 finding 1 is correct for the run: `finish_spike` concludes, the question is kept verbatim, and the agent's own TODO still fails (`checkFindings` replaces only the question). But the file written to disk still fails `lifecycle.Validate`: `section "Question" contains TODO`, and the front-matter title holds `{{…}}`. A person who presses Validate on that document, or any later check, sees a fault they can only fix by changing the question.

There is also a nit. A question that begins with `# ` (for example "# of workers we need for the import?") is written as a level-1 heading inside the Question section and in the title line. It validates, but it renders wrong.

**Fix:**
- Accept and document this in FR-2.1 ("the Question is the person's words and may not validate").
- Or have the findings manifest skip the placeholder check for the Question section.
- Escape a leading `#` (for example with `\#`) in the Question body.

### 8. minor: round-1 finding 9 (tokens added after the end) is unchanged
**Where:** `internal/store/spikes.go:297-305`, and the note at `ui_spikes.go:299`.

**What is wrong:** `AddSpikeTokens` still adds to an ended spike. The page note mentions retried attempts, not a call that lands after the findings recorded the total. The page and the findings can still disagree.

**Fix:** either accept this in the spec and add a phrase to the TokensNote ("…and a call still under way when the run ended"), or guard the update with `AND state = 'running'` and record late usage elsewhere.

### 9. minor: the spike-code-kept checkpoint payload carries `"refs": null` when the check couldn't run
**Where:** `spikes_leak.go:92-93`.

**What is wrong:** `RecordSpikeCodeKept` normalises nil to `[]` for the audit row, but the checkpoint payload gets `kept.Refs` raw, which is `null`. Nothing reads the field today, so this is a nit. The checkpoint also doesn't record `couldnt_check`.

**Fix:** use the same normalised slice, and add `couldnt_check`.

### 10. minor: leftovers from the tidy-up
- **`store.OpenSpikesUnderInitiative`** (`internal/store/spikes.go:484-…`) is now dead code, kept alive only by `TestOpenSpikesUnderInitiative`. It repeats the subtree CTE in `spike_open_ids.go`. Delete it, or make one call the other.
- **`ui_spikes.go:301`** compares `src == "override"` where `store.BudgetFromOverride` exists.
- **`observe.go:750-757`** (`conclude`) still has its own switch over `ended_how` ("It concluded." and so on), outside the shared `SpikeEnding` table.
- **`mcp_spike_tools.go:208-209`** keeps its own "given at the start" source.
- **`ui_spikes.go:107`** (`spikeOwnerRef`) still branches on `sp.FeatureID` instead of `sp.Owner()`.
- **`handleUISpikeStartPost` and `handleUISpikeClose`** (`ui_spikes.go:665`, `:738`) still answer a bad id with `http.Error(400)`, while the GETs answer 404. This was quality minor 21, so it is listed here for completeness only.

## Things checked in the fix diff that hold
- **Timeline asides** (`timeline.go`, `observe.go:286`):
  - `Build` routes non-moment events and runs past asides to the last journey moment (`lastMain`, `first`, the `k = first` fallback). `Line` marks the strip whose last moment is `CurrentIndex`, and folding is unaffected because an aside's fold is its own label.
  - The bug timeline tests (`timeline_bug_test.go`) and the existing feature tests pass, and `TestSpikeMomentsAreAsides` covers runs, events, `Current` and `Strip.Aside`.
  - One edge case: when every moment is an aside, the "before" events land on the last aside. That is harmless, because a feature always has `feature.created`.
- **`code` template func** (`ui_spikes.go:81-95`): it HTML-escapes first, then wraps backtick pairs, and leaves odd counts as literal backticks. It is used only in element content. A question or refusal can't inject markup.
- **Redirects:** post/redirect/get is in place for start (`:689`) and close (`:760-762`). A start that fails re-renders with the error, which is fine.
- **Leak-check discovery:** `worktreeAdminDir` matches `gitdir` with symlinks resolved, `worktreeHead` reads from the admin dir, and a non-SHA or unborn branch fails closed. The `head != last` consistency check catches truncation and a cut last line (tested). `--not base refs_at_start…` is correct for the stated semantics. Tags and `refs/stash` are always reported when changed. That is per spec, though a person tagging a release during a spike will be reported too.
- **Migration 0015** isn't on main, so editing it in place is fine. `refs_at_start jsonb` round-trips: nil marshals to JSON `null`, `scanSpike` reads that back as a nil map, and the check fails closed on nil.
- **`store.StartSpike(SpikeStart)`:** the budget source uses the constants. The refs snapshot is taken before the transaction, and a failure refuses the start in a sentence.
- **Loader** (`compartment.go:437-440`, `otherPurpose`): an unassigned role loads. A role with another purpose, alone or beside `run-spike`, is refused with a deterministic name. Both are tested.
- **`SpikeConfig *int64`:** an absent value defaults in one place, and 0 or a negative is refused. The starter now writes the section uncommented and validates.
- **G5:** it names the IDs ("SPK-002 and SPK-004 are still open"). Both archive routes use `OpenSpikeIDsUnderInitiative`, and the override is tested (`TestArchiveOverrideWithAnOpenSpike`).
- **`dispatch.Thousands`:** handles negatives (`-1,000`), and `groupThousands` delegates to it.
- **Nudge-turn budget fix** (`dispatch.go:561-566`): it uses the database `spent` from the crossing call, and `TestBudgetCrossedByANudgeOnTheLastTurnIsABudgetStop` covers it. No extra model call is possible.
- **Orphan file** (`isOrphanFindings`): it reuses the deterministic path only when the file carries this findings ID and is untracked. The staged case is handled by `ls-tree HEAD`, and `commitPath` failure returns false. Both are tested (`TestSpikeEndingResumesFromAnOrphanFileAndAStagedOne`).
- **`EndSpike`** is serialised by `spikeEndMu` and calls `spikeKept` only for a running spike, so the leak check can't double-record.

## Round-1 verification

### Bugs report

| # | Finding | Status | Evidence |
|---|---|---|---|
| 1 | TODO / `{{` question can't conclude | **Fixed** | `checkFindings` validates with a stand-in question (`spikes.go:647-658`); `buildFindings` no longer defuses title or Question. Tests: `TestSpikeWithATodoQuestionConcludes`, `TestWritingFindings` :402-417. Residual: the file on disk doesn't validate (new finding 7). |
| 2 | Spike moments take over the feature's current position and runs | **Fixed** | `Rule.Aside`, `lastMain`, `CurrentIndex`, `Strip.Aside` (`timeline.go:429-536, 600-631`); `observe.go:286`. Test: `TestSpikeMomentsAreAsides`. |
| 3 | Leak check misses common ways, fails open | **Partially** | Cases a, b and c (an earlier branch, a checkout of base, tag, stash) and d (git errors, a damaged `.git` file) are fixed and tested, and the main-moved false positive is fixed for `checkout`. Remaining: fail-open after the worktree is deleted and pruned (new finding 1); reflog-message spoof and `commit-tree` (new finding 2); merge, rebase or pull false positive (new finding 3). |
| 4 | Staged findings never committed | **Fixed** | `ls-tree --name-only HEAD` and returning false on a `commitPath` error (`spikes_end.go:176-191`); tested. |
| 5 | Orphan findings file after a crash | **Fixed** | `isOrphanFindings` and the deterministic path (`spikes.go:711-714, 738-752`); tested. |
| 6 | Loader too strict for FR-10.4 | **Fixed** | `otherPurpose` (`compartment.go:437-440, 530-540`); spec amended; `TestSpikeToolsLoaderChecks`. |
| 7 | Start and close render instead of redirecting | **Fixed** | `redirectToSpike` with 303 (`ui_spikes.go:470-472, 689, 760-762`). New, minor: a crafted `did` (finding 6). |
| 8 | Wrong `ended_how` on a nudge at the turn cap | **Fixed** | `dispatch.go:561-566` plus a unit test. |
| 9 | Tokens added after the end | **Not fixed** | `AddSpikeTokens` unchanged; the page note doesn't explain it (finding 8). |
| 10 | Copy deviations | **Fixed** | "all of its N tokens" (`spikes.go:390`); starter section uncommented; `code` func renders backticks as `<code>`; G5 names IDs; `spikeRefRe` is case-sensitive `^SPK-\d+$`. |
| 11 | Weak or missing tests | **Fixed** | `initialize` sentence (`TestSpikeMCPInstructions`); badge (`integration_spikes_ui_test.go:331-335`); `_values` checks (`integration_spikes_test.go:271-272`); forecast (`TestSpikeStartScreenForecast`); G5 override (`TestArchiveOverrideWithAnOpenSpike`); `stopEntry` now `t.Fatalf`; leak cases a and b plus tag, stash, moved main and broken reflog. The no-budget path is unchanged (adequate). |

### Quality report majors

| # | Finding | Status | Evidence |
|---|---|---|---|
| 1 | `icons.html` `i-document` broken | **Fixed** | `git diff main` shows only the added `i-spike` line. |
| 2 | `StartSpike` budget-source comment | **Fixed** | `store.BudgetFrom*` constants, documented on `SpikeStart.BudgetSource`. |
| 3 | Duplicate `commas` | **Fixed** | `dispatch.Thousands`; `groupThousands` delegates to it (`ui.go:1079`). |
| 4 | `postValuesGetLocation` duplicate | **Fixed** | Gone from the tests. |
| 5 | Bare `time.Sleep` | **Fixed** | No `time.Sleep` in the spike tests. |
| 6 | `startSpikeQuiet` reimplements `StartSpike` | **Fixed** | `startSpike(…, kick=false)` seam (`spikes.go:196-201`); the helper only marks the dispatch running. |
| 7 | `spikeDir` hand-built short ID | **Fixed** | `store.ShortID("spk", sp.ID)` (`integration_spikes_test.go:158-160`). |
| 8 | `spikeSettled` / `sweepUntilSettled` duplication | **Fixed** | `settle(sp, sweep)`. |
| 9 | Wrong comment name | **Fixed** | `spikeEndedWords` was removed in favour of `SpikeEndingOf`. |
| 10 | "How a run ended" switched in five places | **Partially** | `store/spike_endings.go` is used by the findings, the commit, the page and the timeline. `observe.go:750-757` `conclude` keeps its own switch. |
| 11 | Dead branch and duplicated placeholder rule | **Fixed** | `needsFill`, shared (`spikes.go:495`, `spikes_plan.go:185`). |
| 12 | Dead `removed` and prune | **Fixed** | Removed from `ReconcileSpikes`. |
| 13 | Spike lookup on every milestone add | **Fixed** | `memberType == "spike"` first; `GetSpike` only on a lookup error (`ui_plan_actions.go:508-520`). |
| 14 | `spikeResult` duplicates the owner and budget lookups | **Mostly fixed** | It uses `spikeOwnerRef` and `spikeBudgetFor`. The "given at the start" branch is still local (`mcp_spike_tools.go:208`). |
| 15 | Owner derivation repeated | **Mostly fixed** | `Spike.Owner()` is used in `notifySpikeChanged`, `CloseSpike` and `spikeOwnerPath`. `spikeOwnerRef` still branches on `FeatureID` (`ui_spikes.go:107`). |
| 16 | Raw SQL in the server, errors swallowed | **Fixed** | `store.SpikeKeptRecord`; database errors propagate (`spikes_leak.go:68-74`). |
| 17 | `SpikeConfig` custom unmarshaller | **Fixed** | `*int64`; the default is applied once (`config.go:89-103, 310`). |
| 18 | `spikePage` dead fields and names | **Mostly fixed** | Dead fields removed; `BuildOnSentence` and `FollowedBy` renamed. The `StartScreen.Workers/Running/Free` trio is kept (it was optional). |

## Acceptance coverage
Every FR acceptance criterion now has a test, apart from those that are by construction or by inspection (NFR-1, NFR-5, NFR-6, NFR-8), as in round 1. These still lack one:
- the three leak-check gaps above (findings 1–3);
- a crafted-`did` assertion;
- a test that the findings file on disk for a TODO question is accepted, or is knowingly invalid.
