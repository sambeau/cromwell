# Verification: SPEC-021 stage 1 (M14 spikes)

Branch `claude/subutai-m14-spikes` at `ac33241`, working tree clean. No repository files were edited.

## Suite run

- `SUBUTAI_TEST_DB_SUFFIX=_verify`, `SUBUTAI_TEST_DATABASE_URL=postgres://postgres@localhost:54329/postgres?sslmode=disable`
- `go vet ./...`: clean (exit 0).
- `gofmt -l internal cmd`: no files listed.
- `go test -race -count=1 -v ./...`: exit 0, every package `ok`. Log: `scratchpad/verify-test.log`.
  - Top-level tests: **386 passed, 0 failed, 2 skipped**. There were also 86 subtests, all passing.
  - SKIP `TestDemoM6` (internal/server/demo_m6_test.go:32): "set SUBUTAI_M6_DEMO to a directory to run the SPEC-012 demo". This was already the case before M14.
  - SKIP `TestChecklistBackfillRunsOnlyWithTheTable` (internal/store/identity_test.go:186): "a checklists table exists on this branch; the with-table case below covers it". This was also already the case before M14.
  - All spike integration tests ran against real Postgres and passed, and none was skipped. They are in `internal/server`, which took 184 s: TestCreatingASpike, TestWritingFindings, TestSpikeWithATodoQuestionConcludes, TestSpikePromptCarriesDecisionsAndQuestion, TestSaveFindingsKeepsADraft, TestSpikeWorktreeIsMadeByThePlanner, TestSpikeStopsHardAtItsBudget, TestSpikeSavesOnTheTurnThatCrossesTheBudget, TestSpikeFinishOnTheTurnThatCrossesTheBudget, TestSpikeTurnLimitEndsTheRun, TestSpikeBudgetCarriesAcrossAttempts, TestSpikeWorktreeIsDiscardedWhenItEnds, TestExhaustedSpikeEndsWithoutRetryQuestion, TestSpikeEndIsReconciled, TestSpikeEndingResumesFromAnOrphanFileAndAStagedOne, TestLeftoverSpikeWorktreeIsRemoved, TestPersonClosesASpike, TestAskingAgainMakesASecondSpike, TestSpikeHasNoMergePath (5.18 s, with 12 subtests), TestOpenSpikesBlockArchivingTheirInitiative, TestArchiveOverrideWithAnOpenSpike, TestSpikeAppendixSites, TestSpikeStartsFromTheWebUIOnly, TestSpikePagesShowTheWork, TestSpikeUICreateCloseAndAgain, TestSpikeStartScreenForecast, TestSpikeMCPTools, TestSpikeMCPInstructions and TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet. The store tests (13 spike tests in `internal/store`) also ran against Postgres.

File abbreviations: **IT** = internal/server/integration_spikes_test.go, **UIT** = internal/server/integration_spikes_ui_test.go, **MT** = internal/server/integration_spikes_mcp_test.go.

## FR-1: The spike entity

| Criterion | Verdict | Evidence |
|---|---|---|
| UI on an initiative, UI on a feature, then MCP gives SPK-001, SPK-002, SPK-003 in order | PASS | TestCreatingASpike IT:194-210 asserts IDs and order through the service, with `created_via` ui/ui/mcp. The real form path is in TestSpikeUICreateCloseAndAgain UIT:388-413, which gives SPK-001 and SPK-002 from `POST /ui/spikes`. The real MCP tool is in TestSpikeMCPTools MT:49-61. |
| ...with the owner | PASS | IT:208 (initiative) and IT:215 (feature on s2, none on s1) |
| ...with the audit row | PASS | IT:211 (`spike.created` = 1 per spike) |
| ...with `created_via` | PASS | IT:221. MCP path: MT:88 |
| Blank question refused with a sentence | PASS | IT:250 and IT:258-263 (exact sentence) |
| 501-character question refused | PASS | IT:251 |
| Zero override refused | PASS | IT:252 |
| Negative override refused | PASS | IT:253 |
| `done` feature refused | PASS | IT:254 |
| Archived initiative refused | PASS | IT:255 |
| Next spike still gets the next number | PASS | IT:264-267 (SPK-004). Also UIT:435-438 and MT:107 |
| DB refuses a row breaking each named check | PASS | IT:270-289 covers all 12 named constraints in 0015_spikes.sql:76-95. The assertion requires the constraint name in the error. |
| Closing an idea leaves it closed, unanswered, with no findings | PASS | IT:291-304. UI form: UIT:441-451 |

## FR-2: The findings document

| Criterion | Verdict | Evidence |
|---|---|---|
| Built for each ending (concluded, budget over and under, turn limit, failed) | PASS | TestWritingFindings IT:318-333 and IT:335-338 |
| ...from a full draft, an empty draft, and a draft with its own How this spike ended and Question | PASS | IT:334 (`full`, `empty`, `own sections`, plus `disordered` and `bare`) |
| Each result validates against the manifest | PASS | IT:339-342 |
| The ending section appears once and last | PASS | IT:343-348 |
| The question comes from the row | PASS | IT:352-354 (and the draft's "A different question" is dropped) |
| (Fill-ins of SD-8) | PASS | IT:369-378 |

Note: the code adds an ending that FR-2.3 doesn't list, for a budget stop under the budget: "The spike stopped at its budget, because its next step would have gone over. It used 900 of its 1,000 tokens. ..." (IT:327-328). It is consistent with SD-7, but it isn't spec text.

## FR-3: Starting, in the web UI only

| Criterion | Verdict | Evidence |
|---|---|---|
| Screen renders each part: question and owner | PASS | TestSpikeStartsFromTheWebUIOnly UIT:83-84 |
| ...who will run it, with the model | PASS | UIT:85 |
| ...the budget field and default line | PASS | UIT:86 |
| ...what happens at the budget | PASS | UIT:87 |
| ...decisions, or "No decisions apply here." | PASS | UIT:88 (none), UIT:95-99 (with decisions) |
| ...earlier findings for an ask-again spike | PASS | TestSpikeUICreateCloseAndAgain UIT:500-501 |
| ...tools by name | PASS | UIT:91 |
| ...where it works | PASS | UIT:89 |
| ...free slots and the forecast | PASS | UIT:90. Forecast with three or more runs: TestSpikeStartScreenForecast UIT:537-543 |
| With the override | PASS | UIT:102-105 |
| Role unassigned: the sentence and a disabled button | PASS | UIT:108-112, set against `lacks "disabled"` at UIT:92. The POST is also refused (UIT:113-120). |
| The POST starts it once | PASS | UIT:125-134 (303, the notice, `TokenBudget`=40,000) |
| A second POST is refused with the sentence | PASS | UIT:139-143 (no new run) |
| `/api/spikes/start`, `/api/spikes/<id>/start` and `/api/spike/start` return 404 or 405 | PASS | UIT:155-167 (404 for POST, GET and PUT) |
| Every forbidden MCP name returns method-not-found | PASS | integration_mcp_test.go:204-216 (list at :204-206, code check at :213-214) |
| `create_spike` leaves an idea with no dispatch | PASS | MT:83-94 |

## FR-4: The run

| Criterion | Verdict | Evidence |
|---|---|---|
| Project and initiative decisions are in the prompt | PASS | TestSpikePromptCarriesDecisionsAndQuestion IT:495-506 |
| A sibling initiative's decision is not | PASS | IT:507-509 |
| The question and the budget sentence are there | PASS | IT:497-499 |
| Earlier findings for an ask-again spike | PASS | IT:536-543 |
| `save_findings`: the draft changes on the row | PASS | TestSaveFindingsKeepsADraft IT:563-575 |
| ...no file changes in the main checkout | PASS | IT:577-579 and IT:583-585 |
| ...the worktree's tree is unchanged | PASS | IT:580-582 |
| ...a call from another purpose is refused | PASS | IT:588-591 |
| Worktree detached at `base_commit` | PASS | TestSpikeWorktreeIsMadeByThePlanner IT:633-638 |
| No branch made | PASS | IT:639-641 |
| No `worktrees` row | PASS | IT:642-645 |
| (StartSpike doesn't make the worktree; the planner does) | PASS | IT:621-624 |

## FR-5: The budget and the hard stop

| Criterion | Verdict | Evidence |
|---|---|---|
| Budget 1,000 with 150-token calls lets exactly 6 calls through | PASS | TestSpikeStopsHardAtItsBudget IT:746-748 |
| The further scripted steps stay unused | PASS | IT:749-751 |
| Ended with `budget`, `tokens_used` = 900 | PASS | IT:752-754 |
| The page says "900 of 1,000 tokens" | PASS | This is not in this test. The identical scenario (8×150, budget 1,000) is rendered in TestSpikePagesShowTheWork UIT:231-245, which asserts "900 of 1,000 tokens". |
| The findings hold the last draft and the ending section | PASS | IT:755-761 |
| The transcript's last entry is the stop | PASS | IT:773-777 |
| A save on the crossing turn is kept | PASS | TestSpikeSavesOnTheTurnThatCrossesTheBudget IT:795-805 (and its other tool didn't run, IT:807-811) |
| `finish_spike` on the crossing turn concludes | PASS | TestSpikeFinishOnTheTurnThatCrossesTheBudget IT:826-833 |
| The turn cap ends the run as `turn_limit`, not failed | PASS | TestSpikeTurnLimitEndsTheRun IT:852-873 (run `succeeded`, stop entry) |
| Across attempts: retried, the retry's prompt has the draft, and the stop counts both | PASS | TestSpikeBudgetCarriesAcrossAttempts IT:890-922 |
| A dispatch with no budget is unchanged | PASS | TestNoBudgetTurnCapStillFails internal/dispatch/budget_test.go:251-258, and the rest of the suite passes |

## FR-6: Ending a run

| Criterion | Verdict | Evidence |
|---|---|---|
| For each ending: the worktree directory is gone | PASS | TestSpikeWorktreeIsDiscardedWhenItEnds IT:967-969 (all four endings, IT:940-952) |
| ...`git worktree list` doesn't list it | PASS | IT:970-972 |
| ...`worktree_removed_at` is set | PASS | IT:973-975 |
| ...the audit row is written | PASS | IT:976-981 |
| Exhausted: no `dispatch-failure` checkpoint | PASS | TestExhaustedSpikeEndsWithoutRetryQuestion IT:1018-1025 |
| ...the run is `cancelled` | PASS | IT:1014-1017, and it stays so after more sweeps (IT:1028-1034) |
| ...`POST /api/respond` has nothing to retry | PASS | IT:1043-1045 (status of 400 or more) |
| Reconcile: a spike left running after its run succeeded is ended by the heartbeat | PASS | TestSpikeEndIsReconciled IT:1055-1079. Concluded, exhausted, cancelled and retry-pending cases: IT:1082-1126 |
| ...an EndSpike that failed after writing its findings completes without writing them twice | PASS | IT:1130-1161. Also TestSpikeEndingResumesFromAnOrphanFileAndAStagedOne IT:1168-1239 |
| `TestLeftoverSpikeWorktreeIsRemoved` | PASS | IT:1279-1303 (a leftover git knows, one it has forgotten, an ended spike's left-behind worktree; a running spike's survives) |
| (FR-6.3 amended leak check) | PASS | See NFR-4. The code matches the amended text: admin reflog at spikes_leak.go:282-311, `refs_at_start` diff at :175-191, fails closed at :127-153 |

## FR-7: Closing, and asking again

| Criterion | Verdict | Evidence |
|---|---|---|
| Answered closes an ended spike | PASS | TestPersonClosesASpike IT:1327-1336 |
| Unanswered closes an ended spike | PASS | IT:1337-1340 |
| The findings' state is unchanged | PASS | IT:1342-1344. UI: UIT:477-479 |
| Closing twice is refused with the sentence | PASS | IT:1350-1353 |
| Closing a running spike is refused with the sentence | PASS | IT:1357-1366 |
| Ask again: a new `idea` with its own number | PASS | TestAskingAgainMakesASecondSpike IT:1405-1407 |
| ...the same question | PASS | IT:1405 |
| ...linked both ways on the pages | PASS | UIT:503-504 (the first page links the new one, and the new one links back). The row link is at IT:1408-1410 |
| ...the new budget | PASS | IT:1414-1416 |
| ...the first spike closed | PASS | IT:1417-1420 |
| ...the earlier findings in the new run's prompt | PASS | IT:1433-1436 |
| (Redirects to the new start screen) | PASS | UIT:483-486 |

## FR-8: Where spikes show

| Criterion | Verdict | Evidence |
|---|---|---|
| The spike's page in each state | PASS | TestSpikePagesShowTheWork: idea UIT:197-204, running UIT:213-217, ended at budget UIT:243-252, concluded UIT:261-263, turn limit UIT:322-323, closed answered UIT:344-346, closed unanswered UIT:354-355 |
| ...with the draft while running | PASS | UIT:226-229 |
| Owner sections (initiative direct only, feature) | PASS | UIT:267-274 |
| `/ui/spikes` | PASS | UIT:326-328 |
| The feature's timeline moments | PASS | UIT:278-280 (and internal/timeline/timeline_spike_test.go:19-35) |
| ...the feature's token total unchanged | PASS | UIT:281-286 (`ActualTokens` and `FeatureRuns`) |
| Run page purpose, crumbs and outcome sentence | PASS | UIT:305-309 and UIT:320-321. Also IT:1913-1922 |
| The Inbox line | PASS | UIT:332-352 (plural, singular and empty; badge unchanged) |
| The refusal from `add_milestone_member` | PASS | MT:205-209 (MCP). REST: MT:210-213. UI form: MT:214-228 |
| FR-8.1 timeline "Code kept" moment (spec text, not in the acceptance list) | **PARTIAL** | It is built at ui_spikes.go:394-402 and no test renders it. **Defect:** when the leak check fails closed, `refs` is `[]`, so the label reads "Code was kept on , outside its working copy. ...". That is a broken sentence, and it claims code was kept when the check couldn't tell. The label is also rendered as plain text (spike.html:255), so the backticks around ref names show literally. |

## FR-9: MCP

| Criterion | Verdict | Evidence |
|---|---|---|
| `create_spike` result and refusals | PASS | MT:49-80 and MT:97-106 (`next` sentence at :58) |
| `list_spikes` result and refusals | PASS | MT:125-136 |
| `get_spike` result and refusals | PASS | MT:139-202 (findings, run, follows and followed_by, draft) |
| Created over MCP records `created_via = 'mcp'` | PASS | MT:55 and MT:88 |
| The tool-set test passes (want and forbidden lists) | PASS | integration_mcp_test.go:173-182 and :204-216, test PASS |
| (`initialize` sentence) | PASS | TestSpikeMCPInstructions MT:252. The code has no backticks around `create_spike`, which is a formatting difference only |

## FR-10: The starter pack and configuration

| Criterion | Verdict | Evidence |
|---|---|---|
| Config default | PASS | TestSpikeConfig internal/config/config_test.go:526-528 |
| Config refusal of a non-positive value | PASS | config_test.go:530-536 (0 and -5) |
| Loader: `save_findings` on a role with another purpose | PASS | config_test.go:568-574. The unassigned role loads at :562-565 (amended FR-10.3). The sentence is extended; see the copy check |
| Loader: `report_bug` on the spike runner | PASS | config_test.go:577-582 |
| A fresh `subutai init` has the files and the assignment | PASS | TestStarterHasSpikePack internal/starter/starter_test.go:264-291. Caveat: `packRoot` (:226-257) copies the embedded pack and `generatedConfig` rather than running the `init` command. |
| (Role: Sonnet, the tools, a cap of 40; SKILL.md bullets) | PASS | internal/starter/pack/roles/spike-runner.yaml; skills/run-spike/SKILL.md covers all six FR-10.2 points |

## Non-functional requirements

| NFR | Verdict | Evidence |
|---|---|---|
| NFR-1 One service layer | PASS | UI: ui_spikes.go:684 (StartSpike), :712 (CreateSpike), :748 (CloseSpike). MCP: mcp_spike_tools.go:98. Rules: spikes_end.go:34 (EndSpike). Conditional updates with audit in a transaction: store/spikes.go:277, :338, :378 |
| NFR-2 The orchestrator stays code | PASS | IT:227-229 and MT:91-93 (creating dispatches nothing). No agent tool starts a spike (the tool list at IT:660-667) |
| NFR-3 The seam holds | PASS | integration_mcp_test.go:204-216. The only spike POST routes are `/ui/spikes*` (ui.go:712-718) |
| NFR-4: detached, no branch, no `worktrees` row | PASS | IT:633-645 and IT:1486-1507 |
| NFR-4: `mergeFeature` refuses a spike ID | PASS | IT:1510-1512 |
| NFR-4: rules emit no `MergeFeature` for a run-spike success | PASS | IT:1514-1525 |
| NFR-4: no merge, promote or land route | PASS | IT:1531-1539, and by construction ui.go:712-718 |
| NFR-4: `/api/features/start` and the feature page refuse a spike ID | PASS | IT:1540-1547 |
| NFR-4: main history holds only the findings commit | PASS | IT:1490-1500 |
| NFR-4 leak: a branch at the newest commit | PASS | IT:1553-1588 (findings, audit, checkpoint text, branch not deleted) |
| NFR-4 leak: a branch at an earlier commit, then another commit | PASS | IT:1590 and :1596-1606 |
| NFR-4 leak: a branch, then a checkout of the base commit | PASS | IT:1591 |
| NFR-4 leak: a tag | PASS | IT:1592 |
| NFR-4 leak: a stash | PASS | IT:1593 |
| NFR-4 leak: each raises `spike-code-kept` | PASS | IT:1569-1573 and IT:1604 |
| NFR-4: a main that moved isn't reported | PASS | IT:1609-1636 |
| NFR-4: a broken reflog is reported as a check that couldn't run | PASS | IT:1696-1714 (deleted, truncated, last line cut). Amendments: vanished worktree IT:1637-1665, damaged copy before a retry IT:1666-1695 |
| NFR-5 Contained templates | PASS (note) | The page, owner section, dialog, start screen and list are in spike.html. entity.html:142 has one include and inbox.html:25 one line. Also touched: observe.html (stop entry, the spike outcome line, aside classes for FR-5.1 and FR-8.3/8.4) and icons.html (+1). NFR-5 doesn't list these, but they are small. |
| NFR-6 Coordination | PASS | Only migration `0015_spikes.sql` was added. Shared-file changes are small, as listed in the `git diff --stat` |
| NFR-7 Human prose | **PARTIAL** | The 50+ sampled sentences match (below). The defect is the code-kept timeline label with empty refs (FR-8.1 row above). |
| NFR-8 No typed paths | PASS | spike.html:71-72, 224-225, 299 and 381-382 use hidden row ids. The only typed fields are question (:62) and budget (:67, :227, :323) |
| NFR-9 Tested as before | PASS | vet clean, race suite green, integration tests ran (above) |

## UI copy check (spec text against non-test source)

I grepped 52 quoted sentences or fragments from §2 and §3 against non-test source. All are present, either verbatim or as format strings with the figures filled in. Examples:
- "The spike runner runs it, on" (ui_spikes.go)
- "Nobody is assigned to run spikes, so this spike can't start." (spikes.go)
- "The run stops when it reaches this many tokens. Whatever findings it has saved by then are written up, and you decide whether to ask again." (spike.html)
- "No decisions apply here." (spike.html)
- "It works in a throwaway copy of the code, made from the current main line. Nothing it writes there is kept: the copy is discarded when the run ends, and Subutai never merges it." (spike.html)
- "There aren't enough earlier spikes to forecast this one yet." (ui_spikes.go)
- "This spike has already been started." (spikes.go)
- "An earlier attempt at this spike stopped. Build on these findings rather than starting again." (spikes_plan.go)
- "Saved. If the run stops now, this is what is kept." (spikes.go)
- "The run stopped here because it reached its budget of" (dispatch.go)
- "This spike can't be closed now: it is still running, or it is already closed." (spikes.go)
- "This spike hasn't started yet." / "This spike is running." / "It's waiting for you to read the findings." / "This spike is closed: the question is answered." (ui_spikes.go)
- "The agent hasn't saved any findings yet." / "Its working copy is live." / "Its working copy was discarded on" (spike.html and ui_spikes.go)
- "To build on this, cite ... in a design, then create a feature in the normal way." (ui_spikes.go)
- "It concluded." / "It stopped at its budget." / "It stopped at its turn limit." (store/spike_endings.go)
- "A spike can't be a milestone deliverable, because it ships nothing. Add the feature its findings led to instead." (mcp_spike_tools.go)
- The `create_spike` description, verbatim (mcp_spike_tools.go:34-36)
- "A person can start it from its page in the web UI." (mcp_spike_tools.go)
- "Not answered: the spike stopped at its budget before it reached an answer." / "Nothing was saved before the run stopped." / "The findings above are what it had saved by then."
- "Code from this spike was kept on ... Subutai hasn't deleted it; the Inbox asks what to do."
- The checkpoint question "...A spike's code is never merged. Delete the branch, or keep it knowing it won't be built from." (spikes_leak.go:79-80)
- "I've dealt with it" (ui.go)
- "A spike's only document is its findings, which its run writes." (spikes.go)
- The prompt headings "# What the last spike found", "# What you have saved so far", "# Where it came from", "# Your working copy" (spikes_plan.go)
- The commit message "SPK-NNN: findings (stopped at the budget)" (spikes_end.go:186 with spike_endings.go:33)

Deviations, all minor:
1. The loader refusal adds a clause. The spec has "save_findings is for the spike runner, so role X can't be offered it." and the code (compartment.go:439) has "... can't be offered it while it is assigned <purpose>."
2. The `initialize` sentence has no backticks around `create_spike` (mcp.go:320).
3. For a check that couldn't run, the checkpoint question is a different, longer sentence ("Subutai couldn't check whether code from SPK-x was kept outside its working copy: <reason>. ..."). The spec gives no checkpoint text for that case.
4. There is an extra FR-2.3 ending for an under-budget stop, "because its next step would have gone over".

## Walkthrough (DoD 8)

docs/walkthrough-spec-021.md, with 17 screenshots. I viewed 04-start-screen.png (question, runner and model, slots, forecast, budget, decisions, tools), 07-stopped-at-its-budget.png ("19,944 of 20,000 tokens", findings link, worktree discarded, the three close actions), 15-closed-answered.png (SD-12 sentence, asks-again link) and 16-feature-timeline.png (spike moments as asides). mcp-output.txt shows `start_spike` gets -32601. repo-after.txt shows only the findings commits, the main worktree and `master`.

## Summary

- **PASS: 141**, counting split criteria and NFR bullets in the tables above.
- **PARTIAL: 2.** One is the FR-8.1 "Code kept" timeline moment (spec text that is outside the acceptance list). The other is NFR-7, which has the same root cause.
- **FAIL: 0.**

Every §3 acceptance criterion and every §5 NFR has cited evidence.

### PARTIALs and what would close them

1. **FR-8.1 / NFR-7, the "Code kept" timeline label on the spike page** (internal/server/ui_spikes.go:394-402). When the leak check fails closed, the audit payload has `refs: []` and `couldnt_check: <reason>`. The label then reads "Code was kept on , outside its working copy. A person decides what to do with it." That is an ungrammatical sentence, and it is wrong, because the check couldn't tell. The backticks around ref names also render literally (spike.html:255 prints `.Label` as plain text). No test renders this moment. To close it: when `couldnt_check` is set, render "Subutai couldn't check whether code from this spike was kept: <reason>."; drop the backticks or render them as `<code>`; and add assertions to TestSpikePagesShowTheWork (or to a NFR-4 subtest) for both the ref and the couldn't-check forms of the moment.

### Minor notes, not failures

- The FR-5 hard-stop test doesn't render the page itself. The "900 of 1,000 tokens" assertion is in TestSpikePagesShowTheWork, on an identical scenario.
- FR-10 "fresh `subutai init`" is proved through `packRoot`, which copies the embedded pack and `generatedConfig`, not by running the CLI command.
- The FR-3.3 `spike.started` audit payload (the budget and where it came from) is counted in TestStartSpikeIsConditional but its fields aren't asserted. That isn't in an acceptance list.
- NFR-5: observe.html and icons.html carry small spike-related changes that NFR-5 doesn't list.
- The quoted sentences that differ are listed under the copy check above.
