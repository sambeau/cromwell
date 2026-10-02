# M14 (spikes), round 1: bugs and spec-conformance review

Branch `claude/subutai-m14-spikes`, `git diff main...HEAD` (internal/, cmd/). Contract: SPEC-021 stage 1.

How I checked:
- `go vet ./...` is clean.
- `SUBUTAI_TEST_DB_SUFFIX=_rev1 go test -race -count=1 ./...` passes in every package, and the integration tests ran (server took 178s).
- Two throwaway reproductions (a timeline unit test and a server integration test) were run and then deleted. `git status` is clean.

## Findings

### 1. major: a question containing "TODO" or "{{…}}" can never conclude
**Where:** `internal/server/spikes_plan.go:184-205` (`validateFinishSpike`), with `buildFindings(…, false)` at `spikes.go:487`.

**What is wrong:** `finish_spike` is checked by building the whole document with `defuseText=false`. That document includes the server-owned title and the Question section, both taken from the row. If the person's question holds the word TODO or a `{{…}}` pair, every `finish_spike` call fails validation. The agent cannot fix this, because the offending text is not in its findings. The run then burns its budget or turn limit and ends as `budget` or `turn_limit` instead of `concluded`.

**Evidence:** I ran a throwaway integration test using `plan.ValidateOutcome(goodFindings)`:
- with the question "Can we delete the TODO markers in the auth module?", it returned `section "Question" contains TODO.`;
- with the question "Does the template engine render {{name}} safely?", it returned `front-matter field "title" contains an unresolved {{...}} placeholder; section "Question" contains an unresolved {{...}} placeholder.`

A related side effect: `writeFindings` falls back to `defuse`, which rewrites the question itself in the written findings ("to-do", "{ {"). So the Question section is no longer "the question from the row" (FR-2.1).

**Fix:**
- In `buildFindings`, always defuse (or escape) the server-owned parts: the title and the Question section. Apply `defuseText` only to the agent's sections.
- Or have `validateFinishSpike` check only the agent's sections.
- Add a test with a TODO question that concludes.

### 2. major: spike moments become the feature's "current" position and take over its later runs
**Where:** `internal/timeline/timeline.go:237-246`, with `Build` (`timeline.go:497-516`), `Line`, and `observe.go:287`. This is accepted deviation (e). I judge it a defect.

**What is wrong:**
- `spike.started` and `spike.ended` are ordinary moments with states `active` and `review`.
- `Build` gives every later run and every later non-moment event to the last moment.
- `Line` and the detail view mark the last moment as `Current` ("(where it is now)").

As a result:
- The feature's own implement, review and verify runs queued after a spike starts are listed under "Spike SPK-003 started: What ran after it".
- A feature that is `done` (or in design) shows "Spike SPK-003 ended: …" as where it is now, with `data-state="review"`.

**Evidence:** a throwaway timeline test with feature.created, feature.sent, spike.started, a dispatch.queued for an implement-task run, then spike.ended gave:
- "Spike SPK-003 started" state=active runs=1 (the feature's implement-task run);
- `Line` current = "Spike SPK-003 ended: it stopped at its budget", state review.

A spike created on an active feature can end after the feature merges, so this happens in practice.

**Fix:**
- Mark spike rules as asides, for example `Rule.Aside`.
- In `Build`, don't add aside moments to `momentAt` (so runs and events keep going to the last pipeline moment).
- Give asides a neutral state, or inherit the previous moment's state.
- In `Line` and `observe.go`, choose `Current` from the last non-aside moment.
- Add a test.

### 3. major: the leak check misses common ways to keep code, and fails open
**Where:** `internal/server/spikes_end.go:327-354` (`leakedRefs`). The finding is against SD-2 fact 5, NFR-4 and the Goal: "a run that tries to keep its code is caught".

**What is wrong:** it only runs `for-each-ref --contains <newest commit>`, and only when the worktree's HEAD ≠ base.

**Evidence:** reproduced with plain git in the scratchpad:
- (a) Commit A, `git branch keep-this`, then commit B on the detached HEAD. The newest is B, and `for-each-ref --contains B` is empty, so the leak is missed.
- (b) Commit, branch, then `git checkout --detach <base>`. HEAD == base, so the check is skipped entirely and the leak is missed.
- (c) `git stash` (refs/stash is shared), `git tag`, or `git update-ref` without moving HEAD are all missed.
- (d) Every git error (`rev-parse`, `rev-list`, `for-each-ref`, a missing or corrupted `.git` file, which `write_file` can overwrite) returns nil with only a log line, so the check fails open.

There is also a false positive. If the agent runs `git checkout --detach main` after main has moved on (for example, another spike's findings commit), the "commits made there" are main's own. `main` is then reported as kept code, with a checkpoint.

The implementation follows the spec's literal FR-6.3 text, so the spec is weak here too.

**Fix:**
- Record a snapshot of the refs (`for-each-ref --format='%(objectname) %(refname)'`) when the planner makes the worktree, or at least read the worktree's HEAD reflog (`git -C wt reflog --format=%H HEAD`, which is per worktree and survives resets).
- At the end, report every ref, `refs/stash` included, that contains any commit from the reflog or from `rev-list <all reflog heads> --not <base> <snapshot tips>`.
- Read HEAD through the main repository's admin directory (`.git/worktrees/spk-…/HEAD`) rather than trusting the worktree's `.git` file.
- Treat a git error as "couldn't check": audit it and raise the checkpoint, rather than staying silent.
- Amend FR-6.3 to match.

### 4. minor: findings that are staged but not committed are never committed
**Where:** `internal/server/spikes_end.go:194-201` (`commitSpikeFindings`). This is deviation (c).

**What is wrong:** `git ls-files -- path` lists the index, not HEAD. `commitPath` runs `git add`, then `git commit -- path`. If the commit fails after the add (for example, `index.lock` contention with another `commitDocument` running at the same moment, or a hook), the file is staged. From then on, `ls-files` is non-empty and reconciliation never commits it. It then gets swept into whoever commits next.

**Fix:** test against HEAD with `git ls-tree --name-only HEAD -- path` (empty means never committed), or use `git diff --quiet HEAD -- path` as the spec says. Return false when `commitPath` fails, so the notice isn't sent.

### 5. minor: a crash between writing the findings file and the commit leaves an orphan file
**Where:** `internal/server/spikes.go:599-617` and `spikes_end.go:100-129`.

**What is wrong:** `writeFindings` writes the file, then registers it in the transaction. If the process dies before the commit, `undo` never runs. The re-run picks `freePath`, which gives `SPK-003-findings.2.md`, with the same stamped ID `SPK-003-findings`. The orphan stays in the main checkout as an untracked file carrying a duplicate identity, which the move or identity sweeps may then report.

**Fix:** write to the deterministic path, overwriting when no document row owns it (the transaction decides). Alternatively, write a temp file and rename it inside the commit hook.

### 6. minor: the loader's strict reading of FR-10.3 breaks FR-10.4's upgrade path and the start screen's "unassigned" state
**Where:** `internal/config/compartment.go:434-441`.

**What is wrong:** `save_findings` on any role not assigned `run-spike` makes `config.Load` fail. A project that copies `roles/spike-runner.yaml` but hasn't added the assignment yet can't boot. FR-10.4 says the start screen should say "nobody is assigned" in that case. So can a person who comments out `run-spike` to switch spikes off. Conversely, a role assigned `run-spike` and also another purpose may carry `save_findings`. The spec's "isn't assigned `run-spike` only" reads as forbidding exactly that.

**Fix:** refuse `save_findings` when the role is assigned any purpose other than `run-spike`. Leave unassigned role files alone, since they're never offered tools.

### 7. minor: the start and close POSTs render the page instead of redirecting
**Where:** `internal/server/ui_spikes.go:640` and `:713-718`.

**What is wrong:** FR-3.3 says start "redirects to the spike's page with the notice", and FR-7.1 says close "Each redirects". Both handlers render a 200 page. Only "again" sends a 303. Reloading after a start posts again and shows "This spike has already been started." as an error.

**Fix:** use post/redirect/get with the notice in a flash or query parameter, as the bug triage pattern does, or record this as an accepted UI convention.

### 8. minor: wrong `ended_how` on a nudge turn at the turn cap
**Where:** `internal/dispatch/dispatch.go:508-517` and `561-563`.

**What is wrong:** on the last allowed turn, if the model answers with no tool call (a nudge) and that call crossed the budget, the loop exits and records `turn_limit`. The findings then say "stopped at its turn limit … having used 1,100 of its 1,000 tokens". No extra call is made, so the hard stop holds.

**Fix:** before returning `StopTurnLimit`, return `StopBudget` if `spent >= b.Limit`.

### 9. minor: tokens can be added to a spike after it has ended
**Where:** `internal/store/spikes.go:271-280` (`AddSpikeTokens`).

**What is wrong:** an attempt that was given up on (stalled, then exhausted, then `EndSpike`) still adds its in-flight call to `tokens_used` after the findings and the `spike.ended` audit recorded the total. The page then disagrees with the findings. This is consistent with "a paid call is always counted", but the mismatch isn't explained.

**Fix:** either accept this and have the page say the total may include a late call, or record late usage separately.

### 10. minor: copy deviations
- **`spikes.go:353`:** "It used all 1,000 tokens of it." is ungrammatical. Suggest "It used all of its 1,000 tokens."
- **`starter.go:95-100`:** FR-10.1 says "the generated config includes the section with a comment". The section is fully commented out. This works, because the default applies, but it isn't what the spec says.
- **Literal backticks in UI copy:** `ErrNoSpikeRunner` and the BudgetNote put literal backticks into HTML ("Assign \`run-spike\`…"). The spec's code formatting shows up as raw backticks on the page. Render them as `<code>`, or drop them.
- **`lifecycle/gates.go:143`:** SD-14 says G5's "refusal names them". The reason gives a count ("1 open spike(s) in subtree"), not the IDs.
- **`spikeRefRe` (`mcp_spike_tools.go:23`):** this matches an initiative slug such as `spk-001`. Such an initiative can't be used as `on`, or added to a milestone by path. This is a nit.

### 11. minor: weak or missing tests
- **`initialize` sentence:** the sentence added to `initialize` (FR-9, "You may write down a spike's question…") is never asserted.
- **Inbox badge:** "the Inbox badge still counts checkpoints only" (FR-8.5) is not asserted.
- **Two named checks untested:** `spikes_ended_how_values` and `spikes_closed_as_values` aren't exercised in `TestCreatingASpike` (FR-1 says "each named check").
- **Forecast:** the forecast with three or more earlier spikes (FR-3.2) is untested.
- **G5 override:** SD-14's override behaviour is untested.
- **`stopEntry` tolerance:** `budget_test.go`'s `stopEntry` tolerates a missing `stop` row ("migration may not admit kind 'stop' yet"). The migration is in this branch, so the stop entry should be required.
- **Leak test:** NFR-4's leak test covers only the newest-commit case (see finding 3). Add cases (a) and (b).
- **No-budget path:** for "a dispatch without a budget is unchanged", the only budget-free unit test is `TestNoBudgetTurnCapStillFails`. The rest relies on the existing suite, which is adequate.

## The hard stop: checked and holds

`runLoop` (`dispatch.go:452-563`) does the following, in order:
- reads the database total before every call, and stops at `≥ limit`, or at `turn>0 && total+lastCall > limit`;
- runs `Add` straight after the call, before `stillCurrent`, so a call paid for by an attempt that was given up on is still counted;
- concludes on a valid outcome;
- runs only the early tools (`save_findings`) once `spent ≥ limit`;
- stops after the turn.

Specific cases:
- **Nudge turns:** they skip the post-turn check, but the next pre-call check stops the run, so no extra call is made.
- **Invalid outcome on the crossing turn:** it is answered, and then the run stops at the budget.
- **Parallel tool calls:** they run in block order. A `run_command` placed before a valid `finish_spike` under the budget still runs, which is a harmless ordering difference from FR-5.1 steps 3 and 6.
- **No budget:** with `Budget == nil`, the only change is the transcript `"stop"` kind, which nothing emits. Behaviour is unchanged.

There is no path that starts a new call after the total has reached the limit, except the inherent window in which two attempts overlap between `stillCurrent` and the call.

## Seams: checked and hold
- **No other route starts or closes a spike.** No /api route, MCP tool or agent tool does it. `/api/respond` and the Inbox can't: no `dispatch-failure` is raised for `run-spike`, and `spike-code-kept` answers to a no-op.
- **`save_findings`** needs both the profile and `SpikeID`, which only `planSpike` sets.
- **Merge, promote and feature start:** these routes don't exist, or refuse a spike's ID.
- **Milestones:** a spike is refused at three entry points and by the `milestone_members` check in the database.
- **Templates** use `html/template`, so the question and draft are escaped.
- **CSRF:** the posture is the same as the rest of /ui.
- **Leak paths:** apart from finding 3, the only way a spike's code leaves is inside its findings, by design.

## Deviations (a) to (h)
- **(a)** Acceptable, and necessary for the 900-of-1,000 case. Fix the wording in finding 10.
- **(b)** Acceptable.
- **(c)** A defect. See finding 4.
- **(d)** Acceptable. A run for a spike that isn't running is unreachable in practice, and the plan check runs before the worktree is made.
- **(e)** A defect. See finding 2.
- **(f)** Acceptable, and matches the triage line.
- **(g)** Acceptable. Currency symbols and money helpers are still banned.
- **(h)** Acceptable, with the database check behind it.

## Regressions in shared code
- **Config loader:** an existing project's config still loads, because there's no `save_findings` role and no `run-spike` assignment. The one new failure mode is the partial upgrade in finding 6.
- **`FeatureHistory`:** feature rows are unchanged. Only `spike.started` and `spike.ended` are added, which feeds finding 2.
- **"subutai" as a system actor:** on main, no audit rows use the actor "subutai", so nothing that already exists changes.
- **G5 callers, `ownerCrumb`, `resolveOwner`, `mcpResolveOwner`, `scopeForDocument` and `urlForID`:** each only adds a `spike` case.
- **`rules.go`:** returns early only for `Purpose == "run-spike"`.
- **`observe.go`:** adds only a spike branch.

## Acceptance-criteria coverage

| Criterion | Test (file) | Status |
|---|---|---|
| FR-1: numbering, owner, audit, `created_via` | TestCreatingASpike (integration_spikes_test.go); TestSpikeUICreateCloseAndAgain (…_ui_test.go); TestSpikeMCPTools (…_mcp_test.go) | covered |
| FR-1: refusals, and the next number still follows | TestCreatingASpike | covered |
| FR-1: the database refuses each named check | TestCreatingASpike, TestSpikeChecksRefuseBadRows (store/spikes_test.go) | weak: `_values` checks missing (finding 11) |
| FR-1.4: closing an idea | TestCreatingASpike, TestSpikeUICreateCloseAndAgain | covered |
| FR-2: TestWritingFindings | TestWritingFindings | covered. Misses a question containing TODO or `{{` (finding 1) |
| FR-3: the screen's parts, with and without decisions, override and default, unassigned | TestSpikeStartsFromTheWebUIOnly | covered. Forecast with three or more spikes missing |
| FR-3: the POST starts once, and a second is refused | TestSpikeStartsFromTheWebUIOnly; TestStartSpikeIsConditional (store) | covered |
| FR-3: /api start routes give 404 or 405 | TestSpikeStartsFromTheWebUIOnly | covered |
| FR-3: forbidden MCP names give method-not-found | TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet | covered |
| FR-3: `create_spike` leaves an idea | TestSpikeMCPTools | covered |
| FR-4: the prompt carries decisions and the question | TestSpikePromptCarriesDecisionsAndQuestion | covered |
| FR-4: `save_findings` keeps a draft | TestSaveFindingsKeepsADraft | covered ("another purpose" simulated by a ToolCtx with no SpikeID) |
| FR-4: the planner makes the worktree | TestSpikeWorktreeIsMadeByThePlanner | covered |
| FR-5: TestSpikeStopsHardAtItsBudget | TestSpikeStopsHardAtItsBudget (+ unit TestBudgetStopsBeforeACallThatCannotFit) | covered |
| FR-5: saves on the crossing turn | TestSpikeSavesOnTheTurnThatCrossesTheBudget (+ unit) | covered |
| FR-5: finish on the crossing turn | TestSpikeFinishOnTheTurnThatCrossesTheBudget (+ unit) | covered |
| FR-5: turn limit | TestSpikeTurnLimitEndsTheRun (+ unit) | covered |
| FR-5: budget across attempts | TestSpikeBudgetCarriesAcrossAttempts; unit TestBudgetAlreadySpentMakesNoCalls | covered |
| FR-5: no budget is unchanged | TestNoBudgetTurnCapStillFails + existing suite | weak but adequate |
| FR-6: worktree discarded for each ending | TestSpikeWorktreeIsDiscardedWhenItEnds | covered |
| FR-6: exhausted run asks no retry question | TestExhaustedSpikeEndsWithoutRetryQuestion | covered |
| FR-6: ending is reconciled | TestSpikeEndIsReconciled | covered. Orphan-file crash window untested (finding 5) |
| FR-6: leftover worktree removed | TestLeftoverSpikeWorktreeIsRemoved | covered |
| FR-7: a person closes a spike | TestPersonClosesASpike, TestSpikeUICreateCloseAndAgain | covered |
| FR-7: asking again makes a second spike | TestAskingAgainMakesASecondSpike, TestSpikeUICreateCloseAndAgain | covered |
| FR-8: TestSpikePagesShowTheWork (pages, owner sections, list, timeline, run page, Inbox line) | TestSpikePagesShowTheWork | covered. Badge-only assertion missing. The timeline test didn't catch finding 2 |
| FR-8.6: `add_milestone_member` refusal | TestSpikeMCPTools | covered |
| FR-9: TestSpikeMCPTools and the tool-set test | TestSpikeMCPTools, TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet | covered. `initialize` sentence missing |
| FR-10: config default and refusal | TestSpikeConfig (config_test.go) | covered |
| FR-10: the loader's two refusals | TestSpikeToolsLoaderChecks | covered (but see finding 6) |
| FR-10: a fresh init has the files and the assignment | TestStarterHasSpikePack, TestFindingsTemplateValidates | covered. The generated `spikes:` section is commented out |
| SD-14: G5 | TestOpenSpikesBlockArchivingTheirInitiative, TestG5CountsOpenSpikes | covered. Override untested |
| Appendix A sites | TestSpikeAppendixSites | covered |
| NFR-1: one service layer | by construction. UI, MCP and rules all reach CreateSpike, StartSpike, EndSpike and CloseSpike | no dedicated test |
| NFR-2: creating dispatches nothing | TestCreatingASpike, TestSpikeMCPTools, TestSpikeUICreateCloseAndAgain | covered |
| NFR-3: the seam holds | tool-set test, TestSpikeStartsFromTheWebUIOnly, TestPersonClosesASpike | covered |
| NFR-4: no merge path | TestSpikeHasNoMergePath | covered. Leak cases too narrow (finding 3) |
| NFR-5: contained templates | inspection. spike.html holds the new markup; observe.html also gained two small blocks | no test (nit) |
| NFR-6: coordination | inspection: migration 0015 only | n/a |
| NFR-7: human prose | sentences asserted across the UI and MCP tests | partial (finding 10) |
| NFR-8: no typed paths | inspection: hidden row ids | n/a |
| NFR-9: vet and race clean, integration tests run | my run: vet clean, all packages ok with -race | covered |
