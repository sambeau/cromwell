# VERIFICATION-021, stage 2: spikes run in chat or by hand (M14 stage 2)

**Date:** 2026-10-02
**Verifier:** the M14 stage 2 verifier, independent of the author and the
reviewers. No code was changed and nothing was committed.
**Commit verified:** `faed33a` on `claude/subutai-m14-stage2-spikes` ("REVIEW-021
stage 2: bugs review, round 3 (no major)"), checked out as a separate worktree
at `/var/tmp/ver2`.
**Binding spec:** [SPEC-021](../specs/SPEC-021-spikes.md) §4 (FR-11 to FR-18,
SD-17 to SD-27, NFR-10 to NFR-12, the stage 2 definition of done).
**Also checked:** the four integration checks of
[orchestration-M13-M14](../notes/orchestration-M13-M14.md), and that stage 1's
acceptance still holds.

Every test named below was seen passing in my own run (next section). A
`file:line` is in production code at `faed33a`; a test is cited as
`file:line` of its function and, where it matters, of the assertion. A
screenshot is a PNG in `docs/walkthrough-spec-021-stage2/` that I opened.

**Screenshots.** At `faed33a`, screenshots 06, 13 and 16 still show the
walkthrough's finding 1 (the broken **Ask again** layout) and the "run has
ended" wording, both since fixed in code and pinned by
`TestPersonRunsASpikeThroughTheRoutes` (`integration_spikes_ui_stage2_test.go:205-207`)
and `TestChatSpikePageSaysNothingOfARun` (`:411`). The branch head after
`faed33a` (`0e4535b`) changes only documents: a draft handoff, the dev plan,
the walkthrough text and the regenerated screenshots, `mcp-output.txt`, and
06b removed. I opened the regenerated `16-time-box-ended.png` there: it shows
"This spike has ended: it reached its time box.", "Run in chat by the chat
agent.", "Time box: 4 hours.", the unmeasured line and a correctly drawn **Ask
again**. Where a screenshot is cited below, its content is the same at both
commits unless noted.

## Test run

Run in `/var/tmp/ver2` against a private Postgres
(`SUBUTAI_TEST_PGDATA=/var/tmp/pgdata-ver`, port 54371, started by
`scripts/test-db.sh`).

| Check | Result |
|---|---|
| `go vet ./...` | clean (exit 0, no output) |
| `gofmt -l internal cmd` | empty |
| `go test -race -count=1 -v ./...` | green: every package `ok`, exit 0 |
| Top-level `--- PASS` | 544 |
| Top-level `--- FAIL` | 0 |
| Top-level `--- SKIP` | 2 |
| `internal/server` duration | 300.4 s (`ok subutai/internal/server 300.414s`) |
| `internal/store` duration | 21.0 s |

The two skips, with their reasons as the log gives them:
- `TestDemoM6` (`internal/server/demo_m6_test.go:32`): "set SUBUTAI_M6_DEMO to a directory to run the SPEC-012 demo". An opt-in demo, not a test of this stage.
- `TestChecklistBackfillRunsOnlyWithTheTable` (`internal/store/identity_test.go:186`): "a checklists table exists on this branch; the with-table case below covers it". A branch-dependent case, by design.

**The integration tests ran.** `scripts/test-db.sh` exported `SUBUTAI_TEST_DATABASE_URL=postgres://postgres@localhost:54371/postgres`, and no test skipped for want of a database: `internal/server` took 300 s and `internal/store` 21 s, and every database-backed test cited below (for example `TestChatSpikeIsHeldByItsClaimAndDeadline`, 0.70 s; `TestMigration0016BackfillsSpikes`, 0.25 s; `TestSpikeHasNoMergePath`, 5.20 s) has a `--- PASS` line in the log. Every test this document names, except the removed `TestSpikeCantBeClaimedYet`, was checked by script against the log's top-level `--- PASS` lines.

**Stage 1 still holds.** `TestSpikeStopsHardAtItsBudget`
(`internal/server/integration_spikes_test.go:733`) and `TestSpikeHasNoMergePath`
(`:1465`) passed, with every other stage 1 spike test (107 top-level tests with "Spike" in the name passed, none failed). Stage 1's
`TestSpikeCantBeClaimedYet` no longer exists (no match in the tree), replaced on
purpose by `TestSpikeIsClaimedOnlyByItsExecutor` (FR-13.9).

## FR-11: the executor and the time box in the database

| Criterion | Result | Evidence |
|---|---|---|
| FR-11.1 columns `executor`, `time_box_hours`, `deadline_at` | PASS | `internal/store/migrations/0016_spike_executors.sql:13-15` |
| FR-11.1 order: backfill first, then checks | PASS | backfill at `0016…sql:23` and `:30-41` (inferred `agent` row at the first `dispatch.running`, `ON CONFLICT … DO NOTHING`), checks from `:50` |
| FR-11.1 `spikes_started` keyed on `started_at`; `spikes_ended_how_values` gains `time_box`; `spikes_ended_how_executor`; `spikes_executor_values`; `spikes_time_box_positive`; `spikes_one_limit` | PASS | `0016…sql:50-84`, each named |
| FR-11.1 `work_claims_end_reason_values` replaces the unnamed check, allows `expired`; `work_claims_end_reason` kept | PASS | `0016…sql:93-95`; `TestSpikeExecutorChecks` (`internal/store/spike_executors_test.go:207-214`) sees a made-up reason refused by `work_claims_end_reason_values` and a null reason by `work_claims_end_reason` |
| FR-11.1 no new enum value | PASS | the migration uses only `ALTER TABLE … text` columns and `CHECK`s (`0016…sql:1-4`, whole file read) |
| Acc. 1: each bad combination refused (running chat spike without a deadline; agent ended `time_box`; chat ended `budget`; both limits; time box 0); a claim ended `expired` accepted | PASS | `TestSpikeExecutorChecks` (`spike_executors_test.go:129`): table at `:143-162` asserts each named constraint in the error; `expired` accepted and audited `claim.expired` at `:184-206` |
| Acc. 2: migration on a database with stage 1 spikes (idea, running, ended, closed that ran) leaves each started one `agent` with one inferred execution | PASS | `TestMigration0016BackfillsSpikes` (`spike_executors_test.go:322`): migrates to 0015, seeds the four plus a closed unrun spike, applies 0016, asserts `executor = 'agent'` and one inferred, measured, round-1 row started at the first `dispatch.running` (`:402-417`); idea and unrun stay null (`:418-426`) |
| Acc. 3: a closed spike that ran without an executor, and an agent spike with a deadline, refused | PASS | same test `:431-438` (`spikes_started`, `spikes_one_limit`); also `TestSpikeExecutorChecks` `:157`, `:159` |
| Acc. 4 / FR-11.2: `StartSpike` for a chat spike with 4 hours records a deadline 4 h after `started_at`, to the second; audit row records executor and limit | PASS | `TestStartSpikeForChatSetsTheDeadline` (`spike_executors_test.go:35`): `:44` deadline − start truncated to the second = 4 h; `:52` audit has `"executor": "chat"`, `"time_box_hours": 4` and no budget; code `internal/store/spikes.go:317-318` (same `now()`) |

## FR-12: starting for the chat agent or a person

| Criterion | Result | Evidence |
|---|---|---|
| FR-12.1 three radios in order, with the three sentences | PASS | `TestSpikeStartScreenOffersWhoRunsIt` (`integration_spikes_ui_stage2_test.go:41`, `:47-55`); screenshot `03-start-screen-chat.png` |
| FR-12.1 runner chosen by default when it can run; otherwise disabled with stage 1's refusal and the chat agent chosen | PASS | same test `:56-71` ("Nobody is assigned to run spikes, so this spike can't start.") |
| FR-12.1 time box field from the default and the sentence; panels describe the spike runner and say so | PASS | same test `:53-55` (`value="4"`, the full sentence, "describe the spike runner"); `03-start-screen-chat.png` |
| FR-12.2 agent: execution row written when the run is marked running, once; a retry writes nothing | PASS | `TestSpikeRunWritesItsAgentExecutionOnce` (`integration_spike_executors_test.go:122`, `:134-155`); `TestRecordAgentExecutionForASpike` (`spike_executors_test.go:287`) |
| FR-12.2 chat/person: queues nothing, worktree made after commit, detached, at the base | PASS | `TestChatSpikeStartMakesAWorktreeAndQueuesNothing` (`integration_spike_executors_test.go:23`): no dispatch `:40`, HEAD = base `:47`, not on a branch `:50`, no branch made `:53`; code `internal/server/spikes.go:298-330` (tx returns before enqueue; `makeStartedSpikeWorktree` after commit) |
| FR-12.2 time box must be 1 to 168, refused with the sentence | PASS | `TestSpikeTimeBoxIsChecked` (`integration_spike_executors_test.go:86`, `:91-99`); edges 1 and 168 accepted `:104-110` |
| FR-12.2 a failed make leaves the start standing; the first claim makes it | PASS | `TestAFailedStartMakeThenAClaimRaisesNoQuestion` (`integration_spikes_races_test.go:261`) |
| FR-12.3 `spikes.default_time_box_hours`: absent is 4, 1 to 168, else a sentence; starter documents it | PASS | `TestSpikeTimeBoxConfig` (`internal/config/config_test.go:602`); loader `internal/config/config.go:403-404`; starter `internal/starter/starter.go:108-114` |
| FR-12.4 reconciliation leaves a running chat/person spike and its directory alone; ends it only on a `done` claim (then) or the deadline | PASS | `TestReconcileLeavesARunningChatSpikeBeforeItsDeadline` (`integration_spikes_timebox_test.go:108`); `TestReconcileEndsADoneClaimConcludedBeforeTheDeadline` (`:132`); order in `timeBoxedEnding` (`internal/server/spikes_end.go:509-522`) |
| FR-12.5 notices for chat and person | PASS | `TestSpikeStartsInChatAndByHandFromTheScreen` (`integration_spikes_ui_stage2_test.go:111`, `:136`, `:149`); screenshot `04-started-in-chat.png`, `10-person-spike-started.png` |
| FR-12.5 / NFR-8 the typed time box is kept on a refused start | PASS | code `internal/server/ui_spikes.go:771-772` (refusal re-renders with `form.TimeBox`); not asserted by a test |
| Acc.: screen shows choices, field, sentence, refused/disabled runner | PASS | `TestSpikeStartScreenOffersWhoRunsIt` (above) |
| Acc.: chat start queues no dispatch, makes a detached worktree, no branch | PASS | `TestChatSpikeStartMakesAWorktreeAndQueuesNothing` (above) |
| Acc.: 0, 169, "four" refused with the sentence; spike stays an idea | PASS | `TestSpikeStartsInChatAndByHandFromTheScreen` `:119-128` (also "-1", "2.5") through `POST /ui/spikes/start` |
| Acc.: with no spike runner assigned, chat and person still start | PASS | same test `:116` comments the runner out, then starts both `:130-153` |
| Acc.: a heartbeat after the start leaves the chat spike and directory alone | PASS | `TestReconcileLeavesARunningChatSpikeBeforeItsDeadline` (`integration_spikes_timebox_test.go:108-128`) |
| Acc.: the dispatched run writes its `agent` row once | PASS | `TestSpikeRunWritesItsAgentExecutionOnce` (above) |
| Acc.: the start notices | PASS | `TestSpikeStartsInChatAndByHandFromTheScreen` (above) |

## FR-13: claiming a spike

| Criterion | Result | Evidence |
|---|---|---|
| FR-13.1 `spikeClaims` registered under `spike` | PASS | `internal/server/claims.go:212-214`; methods `internal/server/spikes_claims.go:80-253` |
| FR-13.1 refusal: idea | PASS | `TestClaimSpikeRefusals` (`integration_spikes_claims_test.go:101`, `:110`) |
| FR-13.1 refusal: ended or closed | PASS | `TestSubmittingSpikeFindingsConcludesTheSpike` (`integration_spikes_claims_test.go:423`); `TestAClaimThatWaitedOnAnEndingMakesNothing` (`integration_spikes_races_test.go:127`) |
| FR-13.1 refusals: the wrong executor, all three sentences | PASS | `TestClaimSpikeRefusals` `:116`, `:121`, `:124` |
| FR-13.1 refusal: claim already ended `done` | PASS | `TestADoneClaimIsFinal` (`integration_spikes_races_test.go:161`, `:171`) |
| FR-13.1 refusal: past the deadline | PASS | `TestClaimSpikeRefusals` `:144`; no claim left `:145` |
| FR-13.1 refusal: held by someone else (`heldBySentence`) | PASS | `TestClaimSpikeRefusals` `:138` |
| FR-13.1 refusal: the worktree couldn't be made | PASS | `TestClaimSpikeRefusals` `:150-159` |
| FR-13.1 `onClaim` sets the claim's deadline to the spike's | PASS | `TestClaimSpikeReturnsTheWorkingCopyAndTheContract` (`integration_spikes_claims_test.go:83`) |
| FR-13.1 `staleQuestion` wording; `deadlineQuestion` never asked | PASS | `TestTheSweepAsksAboutASpikeClaim` (`:471-473` exact sentence; `:481` no `claim-deadline`); screenshot `18-inbox-claim-stale.png` |
| FR-13.2 `lock` and `rules` on the interface; renewal refused after the deadline | PASS | `claims.go:195`, `:199`, `:419`, `:537`; `TestRenewingASpikeClaim` (`integration_spikes_claims_test.go:185`, `:201-206`) |
| FR-13.2 submit/release are the spike's own methods; `resolveClaimRef` task-only; `claim_task` points to `claim_spike` | PASS | `spikes_claims.go:539`, `:632`; `claims.go:352`; `TestClaimSpikeRefusals` `:162-165` |
| FR-13.2 every claim is its own `spike`, round 1, unmeasured execution row | PASS | `TestReleasingASpikeClaimKeepsTheSpikeRunning` (`:248`, two rows, two claim IDs) |
| FR-13.3 read again under the worktree's lock; nothing made for a spike that is over | PASS | `TestAClaimThatWaitedOnAnEndingMakesNothing` (`integration_spikes_races_test.go:105`); `TestAClaimPastTheDeadlineMakesNothing` (`:141`); `TestAClaimOvertakenByAStartWaitsForTheStartsWorktree` (`:307`) |
| FR-13.3 `spikeHadWorkingCopy` is "the worktree was made"; leak check fails closed with it | PASS | `spikes_end.go:342`; `TestAFailedStartMakeThenAClaimRaisesNoQuestion` (`races_test.go:281`, `:291`); `TestTimeBoxEndingRunsTheLeakCheck` (`integration_spikes_timebox_test.go:226-229`) |
| FR-13.4 result: `spike`, `working_copy` (absolute, base, detached), `contract` (question, where it came from, template; draft when present), `deadline`, `time_left`, `rules`, `next` | PASS | `TestClaimSpikeOverMCP` (`integration_spikes_mcp_test.go:265`, keys `:269`, working copy `:276`, `next` `:291`); `TestClaimSpikeReturnsTheWorkingCopyAndTheContract` (`integration_spikes_claims_test.go:24`, template sections `:58`, time left `:61`, rules `:67-76`); draft in the contract `TestReleasingASpikeClaimKeepsTheSpikeRunning` `:251` |
| FR-13.4 the five rule sentences, then the commands | PASS | `spikes_claims.go:37-43` match the spec word for word; trailing rules asserted to be commands `integration_spikes_claims_test.go:72-76` |
| FR-13.5 `claim_spike` description and argument | PASS | `internal/server/mcp_spike_tools.go:50-63` (what it does, a person started it with a time box, the rules, the time box ends it regardless, calling again renews) |
| FR-13.6 save: holder, surface, deadline, blank refused; one transaction; activity `findings`; `claim.activity`; `claim-stale` withdrawn | PASS | `TestSavingSpikeFindings` (`integration_spikes_claims_test.go:279`); `TestRecordSpikeFindingsActivity` (`internal/store/spike_executors_test.go:229`) |
| FR-13.6 tool result "Saved. If the time box ends now, this is what is kept." and time left | PASS | `TestSaveAndSubmitSpikeOverMCP` (`integration_spikes_mcp_test.go:307`) |
| FR-13.7 submit: refusals, draft fallback, validation, `submit` then `done`, `submitted_at`, `claim.submitted` without commit or review, ending as `concluded` | PASS | `TestSubmittingIncompleteSpikeFindingsChangesNothing` (`:337`); `TestSubmittingSpikeFindingsConcludesTheSpike` (`:381`, payload `:416-420`); `TestSubmittingWithNoFindingsUsesTheDraft` (`:429`) |
| FR-13.7 step 4: a submit that loses to an ending is refused, saying the findings were lost | PASS | `TestASubmitThatLosesToTheTimeBoxIsRefused` (`integration_spikes_races_test.go:202`, `:221`) |
| FR-13.7 `submit_spike` result: entry, findings ID and path, `next` | PASS | `TestSaveAndSubmitSpikeOverMCP` (`integration_spikes_mcp_test.go:314-328`) |
| FR-13.8 `initialize` sentence; `run-a-spike` skill carries every rule | PASS | `TestSpikeMCPInstructions` (`integration_spikes_mcp_test.go:258`); `TestRunASpikeSkillSaysWhatTheRulesSay` (`:407`) |
| FR-13.9 `TestSpikeIsClaimedOnlyByItsExecutor` replaces `TestSpikeCantBeClaimedYet` | PASS | `internal/server/integration_claims_spikes_test.go:18` (agent and idea refused, person refused over MCP, chat refused in the UI, `claim_task` points to `claim_spike`, chat claimed over MCP); the old test is gone |
| Acc.: MCP claim returns working copy, contract, deadline, rules; claim `feature_id` null, `deadline_at` equal; execution chat, spike, round 1, unmeasured | PASS | `TestClaimSpikeReturnsTheWorkingCopyAndTheContract` `:78-89`; `TestClaimSpikeOverMCP` |
| Acc.: each refusal, by test | PASS | the refusal rows above |
| Acc.: claiming again renews; after a release, a new claim and execution | PASS | `TestRenewingASpikeClaim` `:189-199`; `TestReleasingASpikeClaimKeepsTheSpikeRunning` `:244-250` |
| Acc.: a missing worktree is made by the claim | PASS | `TestClaimingASpikeMakesAMissingWorktree` (`:257`) |
| Acc.: save replaces the draft, withdraws `claim-stale`; refused for someone else, without a claim, after the deadline | PASS | `TestSavingSpikeFindings` `:285-333` |
| Acc.: incomplete submit refused, nothing changes; good submit concludes, commits, removes the worktree, claim `done`, execution submitted | PASS | `TestSubmittingIncompleteSpikeFindingsChangesNothing` `:361-365`; `TestSubmittingSpikeFindingsConcludesTheSpike` `:393-412` |
| Acc.: submit with no findings uses the draft | PASS | `TestSubmittingWithNoFindingsUsesTheDraft` `:442-448` |
| Acc.: the chat skill test and the `initialize` sentence | PASS | as FR-13.8 |

## FR-14: releasing, and the inactivity question

| Criterion | Result | Evidence |
|---|---|---|
| FR-14.1 release ends the claim `released`, audited with who; spike running, draft and worktree kept; nothing committed or queued | PASS | `TestReleasingASpikeClaimKeepsTheSpikeRunning` (`integration_spikes_claims_test.go:211`, `:226-242`) |
| FR-14.2 Keep and Release answers; answer to a moved-on claim is a no-op | PASS | `TestTheSweepAsksAboutASpikeClaim` (`:485-517`). The no-op is asserted as "no error"; SPEC-020's notice text is not asserted for a spike |
| FR-14.2 Inbox reads `release_label` and `release_consequence`; old checkpoints fall back to the task's words | PASS | `TestInboxSpikeClaimStaleSaysReleaseTheClaim` (`integration_spikes_ui_stage2_test.go:304`, fallback `:321-323`); `TestInboxTaskClaimStaleKeepsItsWords` (`:326`); context stored `claim_sweep.go:184`; `TestATaskClaimQuestionCarriesTheTaskConsequence` (`integration_spikes_claims_test.go:525`) |
| FR-14.2 the timeline's moment for a spike | PASS | `TestSpikeClaimStaleMoment` (`internal/timeline/timeline_claim_test.go:11`); `internal/timeline/timeline.go:339-340` |
| FR-14.3 activity: saving findings, a renewal, a submit | PASS | `TestSavingSpikeFindings` `:311` (`findings`); `TestRenewingASpikeClaim` `:194` (`renewed`); submit ends the claim (`TestSubmittingSpikeFindingsConcludesTheSpike` `:407`) |
| FR-14.3 activity: a change in the spike's detached working copy (the sweep's fingerprint) | PARTIAL | the sweep fingerprints `t.Path` for any claimable (`claim_sweep.go` step 1, after `:71`), and the spike's `load` returns the worktree's absolute path (`spikes_claims.go:80`), but no test changes a file in a spike's worktree and sees the claim's activity move or a `claim-stale` withdrawn |
| FR-14.4 `deadlineEnder` judged first; no `claim-stale` before the ending; never `claim-deadline` | PASS | `claim_sweep.go:23-26`, `:71-73`; `spikes_claims.go:248` |
| Acc.: a person releases a chat spike's claim; still running; the chat agent can claim again | PASS | `TestReleasingASpikeClaimKeepsTheSpikeRunning`; through the UI route `TestChatSpikePageUnclaimedClaimedAndReleased` (`integration_spikes_ui_stage2_test.go:255-263`) |
| Acc.: backdated activity raises `claim-stale` with the question; saving withdraws it; Release releases; Keep keeps | PASS | `TestTheSweepAsksAboutASpikeClaim` (`integration_spikes_claims_test.go:454`); `TestSavingSpikeFindings` `:289-316` |
| Acc.: the Inbox shows the spike's words, a task's unchanged | PASS | `TestInboxSpikeClaimStaleSaysReleaseTheClaim`, `TestInboxTaskClaimStaleKeepsItsWords`; screenshot `18-inbox-claim-stale.png` |
| Acc.: expiry and deadline both passed: one sweep ends the spike, no `claim-stale`; never `claim-deadline` | PASS | `TestTheSweepEndsASpikeAtItsDeadline` (`integration_spikes_claims_test.go:543`, `:555-564`) |

## FR-15: the time box ends the spike

| Criterion | Result | Evidence |
|---|---|---|
| FR-15.1 every heartbeat ends running chat/person spikes past their deadline; the sweep too; serialised and conditional | PASS | `ReconcileSpikes` `spikes_end.go:427-466`; `store.SpikesPastDeadline` (`internal/store/spikes.go:350-355`) tested by `TestSpikesPastDeadline` (`spike_executors_test.go:77`); `EndSpike` under `withSpikeEndLocks` (`spikes_end.go:104-115`); second heartbeat writes nothing (`integration_spikes_timebox_test.go:72-75`) |
| FR-15.2 `time_box` only for chat/person; worktree lock before `spikeEndMu`; any `done` claim means `concluded`; an unended claim is expired, audited, `claim-stale` withdrawn | PASS | `checkSpikeEnding` (`spikes_end.go:80-95`); `settleSpikeClaim` (`:208-233`); withdrawal in `TransitionClaim` (`internal/store/claims.go:230-242`); `TestEndSpikeRefusesAWayItsExecutorCantHave` (`integration_spikes_timebox_test.go:236`) |
| FR-15.3 empty Answer and What we found sentences for `time_box` | PASS | `store.SpikeEndingOf(time_box).NotAnswered` asserted in `TestSpikesPastDeadline` (`spike_executors_test.go:120-124`); "Nothing was saved before the time box ended." at `internal/server/spikes.go:472-480` (code only: no test ends a time-boxed spike with an empty draft) |
| FR-15.3 How this spike ended: time box of N hours; "The chat agent concluded the spike." / "sam concluded the spike by hand."; no token count; the unmeasured sentence | PASS | `TestHeartbeatEndsAnUnclaimedSpikeAtItsTimeBox` (`integration_spikes_timebox_test.go:79`, singular hour `:98-103`); `TestReconcileEndsADoneClaimConcludedBeforeTheDeadline` `:157-158`; `TestChatSpikeTokensAreUnmeasured` (`integration_spikes_stage2_e2e_test.go:241-247`, no token count by regex); screenshot `17-time-box-findings.png`, `07-chat-findings.png` |
| FR-15.3 `spikeEndings` gains `time_box` with phrase, run line, commit words, lead, not-answered | PASS | `TestSpikesPastDeadline` (`spike_executors_test.go:120-124`); `internal/store/spike_endings.go:45` |
| FR-15.4 the leak check keeps reporting tags and stashes (flagged choice) | PASS | unchanged rule; `TestTimeBoxEndingRunsTheLeakCheck` shows it runs on a time-box ending |
| FR-15.5 **Ask again** with no budget field; created with no budget override | PASS | template `internal/server/ui/templates/spike.html:285-300`; `TestAskAgainAfterAChatSpikeHasNoBudget` (`integration_spikes_timebox_test.go:334`, chat and person); `TestPersonSpikeAskAgainHasNoBudget` (`integration_spikes_ui_stage2_test.go:219`); screenshot `16-time-box-ended.png` at `0e4535b` |
| Acc.: backdated deadline, one heartbeat ends a claimed chat spike `time_box`; draft and time-box sentence in the findings, committed, worktree gone, claim `expired` | PASS | `TestHeartbeatEndsAClaimedChatSpikeAtItsTimeBox` (`integration_spikes_timebox_test.go:52`), checks in `checkEndedAtTimeBox` `:22-48`, claim `:64`; screenshots `15-time-box-before.png`, `16-time-box-ended.png`, `17-time-box-findings.png` |
| Acc.: the same for an unclaimed chat spike and a person spike | PASS | `TestHeartbeatEndsAnUnclaimedSpikeAtItsTimeBox` (`:79-96`) |
| Acc.: past the deadline, a branch made in the worktree is reported | PASS | `TestTimeBoxEndingRunsTheLeakCheck` (`:194`, `:219-225`) |
| Acc.: a `done` claim is ended `concluded` by reconciliation, even past the deadline | PASS | `TestReconcileEndsADoneClaimConcludedBeforeTheDeadline` (`:132`); `TestADoneClaimIsFinal` (`integration_spikes_races_test.go:161`, a later claim does not undo it) |
| Acc.: a submit and a deadline ending at once: either `concluded` with the submitted findings, or a refused submit saying the findings were lost and `time_box` | PASS | both orders forced with hooks: `TestASubmitThatLosesToTheTimeBoxIsRefused` (`races_test.go:202`) and `TestATimeBoxEndingAfterASubmitChangesNothing` (`:237`) |
| Acc.: a working copy deleted and pruned fails closed | PASS | `TestTimeBoxEndingRunsTheLeakCheck` `:210-232` |
| Acc.: `EndSpike(time_box)` on an agent spike refused | PASS | `TestEndSpikeRefusesAWayItsExecutorCantHave` `:244` |

## FR-16: where it shows

| Criterion | Result | Evidence |
|---|---|---|
| FR-16.1 executor line built by one function shared with `get_spike` | PASS | `spikeExecutorSentence` (`internal/server/spikes_executor.go:125`) called by the page (`ui_spikes.go:398`) and by `get_spike`/`list_spikes` (`mcp_spike_tools.go:338`) |
| FR-16.1 the six executor sentences | PASS | `TestSpikeExecutorSentences` (`internal/server/spikes_executor_test.go:17`) asserts every one, plus "Not started yet." and the unclaimed-then-ended case |
| Acc.: each executor line "by a page test and a `get_spike` test" | PARTIAL | page tests cover "To be run in chat…" (`integration_spikes_ui_stage2_test.go:138`, `:246`), "Being run in chat by the chat agent" (`:252`), "To be run by hand…" (`:150`), "Being run by hand by" (`:181`), "Run by hand by" (`:202`). No page test asserts "Run by the spike runner (*model*)." or "Run in chat by the chat agent." (the latter is seen in screenshots `06-chat-spike-ended.png` and `16-time-box-ended.png`). Over MCP, `get_spike`'s `sentence` is asserted only for the claimed chat spike (`integration_spikes_mcp_test.go:358`); the other kinds are asserted by `kind` only (`integration_spikes_stage2_e2e_test.go:190`). The shared function makes divergence unlikely, but the criterion asks for both tests per line |
| FR-16.1 the limit: time box line while running and after; unmeasured line in place of the token bar | PASS | `TestSpikeTimeBoxAndUnmeasuredLines` (`spikes_executor_test.go:67`, exact sentences); page: `TestChatSpikePageUnclaimedClaimedAndReleased` (`integration_spikes_ui_stage2_test.go:246`), `TestChatSpikeTokensAreUnmeasured` (`integration_spikes_stage2_e2e_test.go:237-239`, no token bar); screenshots `05-chat-spike-claimed.png`, `14-person-spike-closed.png` |
| FR-16.1 running person spike: **I'll run this spike**; when held, path, time left, editor, **Save findings**, **I've finished**, **Release the claim**; four same-origin `POST`s with the ID in a hidden field | PASS | `TestSpikeStartsInChatAndByHandFromTheScreen` `:150`; `TestPersonRunsASpikeThroughTheRoutes` (`:169`, `:179-184`); routes `internal/server/ui.go:741-744`; same-origin and bad-ID refusals `TestSpikeClaimRoutesNeverStartCloseOrReopen` `:380-392`; screenshots `11-person-claimed-before-saving.png`, `12-person-saved-draft.png`. A success redirects with a notice (`ui_spikes.go:1044`); a refusal re-renders the page with a banner rather than redirecting (`:1027-1032`) |
| FR-16.1 running chat spike: holder, since when, last activity, **Release the claim**; unclaimed sentence | PASS | `TestChatSpikePageUnclaimedClaimedAndReleased` (`:240-253`); screenshot `05-chat-spike-claimed.png` |
| FR-16.2 `executor` {kind, sentence, who, model, run_id}, `measured`, `time_box_hours`, `deadline`, `claim` {kind, who, state, since, last_activity}; `tokens_used` null when unmeasured | PASS | `TestSpikeExecutorFieldsOverMCP` (`integration_spikes_mcp_test.go:350`, `:357-402`); `model` and `run_id` in `TestSpikeExecutorSentences` `:62`; `mcp_spike_tools.go:338-356` |
| FR-16.3 the spikes list shows "agent", "chat" or "by hand" | PASS | `TestSpikeListsShowTheExecutor` (`integration_spikes_ui_stage2_test.go:270`, both `/ui/spikes` and the initiative page); screenshot `20-spikes-list.png` |
| FR-16.4 / SD-26 the person who ran it may close it; `closer_ran_it`; "Closed by sam, who also ran it." | PASS | `TestCloserWhoRanTheSpikeIsRecorded` (`integration_spikes_timebox_test.go:282`); `TestPersonSpikeEndToEnd` (`integration_spikes_stage2_e2e_test.go:365-368`); screenshot `14-person-spike-closed.png` |
| Acc.: the time-box and unmeasured lines on a chat spike's page | PASS | `TestSpikeStartsInChatAndByHandFromTheScreen` `:138-139`; `TestChatSpikeTokensAreUnmeasured` `:238` |
| Acc.: a person spike closed by who ran it, with the audit field and the sentence | PASS | as FR-16.4 |
| Acc.: the ask-again dialog on a chat spike has no budget field | PASS | the template's branch is one for chat and person (`ShowAgainBudget = !unmeasuredExecutor(sp.Executor)`, `ui_spikes.go:465`; `spike.html:285-300`); page test on a person spike `TestPersonRunsASpikeThroughTheRoutes` `:203-207`; the chat spike's page in `16-time-box-ended.png` at `0e4535b` shows **Ask again** with no field |

## FR-17: the boundary

| Criterion | Result | Evidence |
|---|---|---|
| FR-17.1 the advertised set gains exactly `claim_spike`, `save_spike_findings`, `submit_spike`, with a comment citing DEC-007 decision 3, DESIGN-010 §10 and FR-17.1 | PASS | `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` (`internal/server/integration_mcp_test.go:77`; the comment and three names in its `want` list, exact `reflect.DeepEqual`) |
| FR-17.2 must-not-exist list gains the seven names; stage 1's five stay | PASS | same test, forbidden list (`release_spike` … `claim_spike_review`; `start_spike`, `close_spike`, `answer_spike`, `save_findings`, `finish_spike`), each answered method-not-found |
| FR-17.3 no new route starts, closes or reopens a spike; each posted with a spike in every state | PASS | `TestSpikeClaimRoutesNeverStartCloseOrReopen` (`integration_spikes_ui_stage2_test.go:342`): idea, running agent, running chat, ended, closed × four routes, state/executor/closed_as unchanged. A running person spike is exercised through the routes in `TestPersonRunsASpikeThroughTheRoutes`, where only the submit's ending (not a close) follows |
| Acc.: the tool-set test passes with M13's and M14's tools together, and lists the three on purpose | PASS | same test passed; M13's `claim_task`/`submit_task` and M14's six spike tools are in one list |

## FR-18: the integration checks

| Criterion | Result | Evidence |
|---|---|---|
| `TestChatSpikeIsHeldByItsClaimAndDeadline` | PASS | `integration_spikes_stage2_e2e_test.go:79`: started via `POST /ui/spikes/start`, claim's `deadline_at` = spike's (`:89`), inactivity raises `claim-stale` and no `claim-deadline` (`:98-106`), a save withdraws it (`:109-115`), the heartbeat's two duties end it `time_box` with the worktree gone, findings committed, claim `expired` (`:118-139`) |
| `TestSpikeExecutorIsRecorded` | PASS | `:144`: agent row tied to the dispatch, measured (`:153-162`); chat row tied to the claim, unmeasured (`:165-172`); person row via `ui` (`:175-182`); `spikes.executor` and `get_spike` for all three (`:185-196`) |
| `TestChatSpikeTokensAreUnmeasured` | PASS | `:202`: executions unmeasured with no dispatch, `tokens_used` 0, `get_spike` null and `measured: false`, page line and no bar, findings without a count, forecast samples and screen line unchanged (`:216-266`) |
| The tool-set test | PASS | as FR-17 |
| `TestChatSpikeEndToEnd` | PASS | `:271`: created over MCP, started in the UI, claimed, saved and submitted over MCP; one commit touching only the findings, no branch, worktrees as before; `close_spike` over MCP is method-not-found; closed as answered in the UI |
| `TestPersonSpikeEndToEnd` | PASS | `:334`: the whole run through the four routes and the close; "Closed by …, who also ran it."; `closer_ran_it` true; one unmeasured person execution |

## Stage 2's scope decisions, where testable

| Decision | Result | Evidence |
|---|---|---|
| SD-17 executor on the row; who did it in `executions` | PASS | `TestSpikeExecutorIsRecorded` |
| SD-18 time box, deadline fixed once, never both limits | PASS | `TestStartSpikeForChatSetsTheDeadline`; `spikes_one_limit` in `TestSpikeExecutorChecks` |
| SD-19 `claim_spike`; only the chosen executor, on its surface; `feature_id` null | PASS | `TestSpikeIsClaimedOnlyByItsExecutor`; `TestClaimSpikeReturnsTheWorkingCopyAndTheContract` `:79` |
| SD-20 deadline hard and ends the spike without a question; inactivity question before it | PASS | `TestChatSpikeIsHeldByItsClaimAndDeadline`; `TestTheSweepEndsASpikeAtItsDeadline` |
| SD-21 `expire` event and `expired` reason | PASS | `internal/lifecycle/claim.go:34-63`; lifecycle `claim_test.go` table; `TestSpikeExecutorChecks` `:184-206` |
| SD-22 release frees only | PASS | `TestReleasingASpikeClaimKeepsTheSpikeRunning` |
| SD-23 submit ends as concluded, no reviewer | PASS | `TestSubmittingSpikeFindingsConcludesTheSpike` (`claim.submitted` without review `:416-420`) |
| SD-24 unmeasured, not zero; forecast samples only `run-spike` | PASS | `TestChatSpikeTokensAreUnmeasured` |
| SD-25 the chat agent can't close, approve, release or move the deadline | PASS | tool-set test's forbidden list; `TestChatSpikeEndToEnd` `:315-321` |
| SD-26 who ran it may close; `closer_ran_it`; the sentence | PASS | `TestCloserWhoRanTheSpikeIsRecorded`; `TestSpikeRanByIsOneAnswer` (`spikes_executor_test.go:137`) |
| SD-27 working-copy lock, then `spikeEndMu`, then rows (spike, then claim) | PASS | `withSpikeEndLocks` (`spikes_end.go:104-115`); row order `recordSpikeEnd` (`:150-157`, `LockSpike` before `settleSpikeClaim`); claim/submit/start races against an ending holding the locks: `TestAClaimThatWaitedOnAnEndingMakesNothing`, `TestASubmitThatLosesToTheTimeBoxIsRefused`, `TestAClaimOvertakenByAStartWaitsForTheStartsWorktree`; no deadlock: `TestSpikeEndingUnderItsLocksDoesNotDeadlock` (`integration_spike_executors_test.go:164`, which says itself that it doesn't prove the order) |

## Stage 2's non-functional requirements

| Requirement | Result | Evidence |
|---|---|---|
| NFR-10 one service layer: MCP and the UI call `ClaimSpike`, `SaveSpikeFindings`, `SubmitSpike`, `ReleaseSpikeClaim` | PASS | UI `ui_spikes.go:1040`, `:1052`, `:1066`, `:1078`; MCP `mcp_spike_tools.go:246`, `:277`, `:300`; release has no MCP tool by design |
| NFR-11 no git in a transaction: worktree made after the start's commit; leak check before `EndSpike`'s transaction | PASS | `spikes.go:298-330` (tx, then `makeStartedSpikeWorktree`); `recordSpikeEnd` calls `spikeKept` before `WithTx` (`spikes_end.go:143-150`) |
| NFR-12 migration `0016` only; shared files as listed | PASS | `internal/store/migrations/` ends at `0016_spike_executors.sql`; the stage's changes to `claims.go`, `claim_sweep.go`, `lifecycle/claim.go`, `store/claims.go`, `store/executions.go`, `dispatch`, `ui.go`, `inbox.html`, `timeline.go`, `mcp.go` and the tool-set test are the ones the evidence above cites |

## The orchestration note's four integration checks

| Check | Result | Evidence |
|---|---|---|
| A spike run in chat or by a person is held by M13's claim expiry | PASS | `TestChatSpikeIsHeldByItsClaimAndDeadline`; `TestTheSweepAsksAboutASpikeClaim` |
| A spike's executor is recorded | PASS | `TestSpikeExecutorIsRecorded` |
| A chat-run spike's tokens are marked unmeasured | PASS | `TestChatSpikeTokensAreUnmeasured` |
| Neither milestone's tools escape the MCP tool-set test | PASS | `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` |

## Stage 2 definition of done

| Item | Result | Evidence |
|---|---|---|
| `tests_pass`: vet, gofmt, the race suite green, integration tests run | PASS | the test run above |
| `verification_passed`: every §4 criterion has cited evidence | PARTIAL | this document: two PARTIALs below |
| Browser walkthrough without an AI provider: chat and person spikes started, chat claimed over MCP, findings saved and submitted, time-box and unmeasured lines, the person spike run by hand, both closed | PASS | `docs/walkthrough-spec-021-stage2.md` §1-7; screenshots `03`-`14`, `15`-`17`, `18`-`20`; `mcp-output.txt` |
| Handoff `docs/notes/handoff-M14-stage2-2026-10-02.md` | PARTIAL | absent at `faed33a`. A draft was committed after it (`71bf456`, "Handoff for M14 stage 2: draft, before verification"); it was not part of what I verified |
| Roadmap §11 line for M14 updated | PASS | `docs/notes/subutai-status-and-roadmap-2026-09-28.md:737` marks stage 2 done and links the stage 2 handoff (which exists only from `71bf456`) |
| Stage 1's acceptance still holds | PASS | the stage 1 tests passed, `TestSpikeStopsHardAtItsBudget` and `TestSpikeHasNoMergePath` among them |

## FAILs and PARTIALs, and what would close each

There are no FAILs.

1. **FR-16 acceptance: each executor line by a page test and a `get_spike`
   test (PARTIAL).** Add, to an existing page test, the ended chat spike's
   "Run in chat by the chat agent." and an agent spike's "Run by the spike
   runner (*model*)."; and in `TestSpikeExecutorFieldsOverMCP` or
   `TestSpikeExecutorIsRecorded`, assert `executor.sentence` for the agent,
   unclaimed chat, person and ended cases, not only `kind`.
2. **FR-14.3: a change in a spike's working copy is claim activity
   (PARTIAL).** Add a test that claims a chat spike, backdates the claim's
   activity, writes a file in its detached worktree, runs `ClaimSweep`, and
   sees the activity recorded and no `claim-stale` raised (or a pending one
   withdrawn).
3. **Definition of done: the handoff (PARTIAL at `faed33a`).** Closed by the
   handoff the lead has drafted since (`71bf456`), once it is finalised with
   this verification's result.

Minor observations, not criteria: the empty-draft time-box sentences
(FR-15.3) and the typed time box kept on a refused start (NFR-8) are proved
by code only; the moved-on answer's notice (FR-14.2) is asserted as a no-op,
not by its words.

## After verification: the PARTIALs closed (lead, 2026-10-02)

Each PARTIAL above was closed after this run, on the branch:

1. **FR-16's executor lines.** `TestSpikeExecutorIsRecorded`
   (`integration_spikes_stage2_e2e_test.go`) now asserts, for the agent, the
   claimed chat and the claimed person spike, that `get_spike`'s
   `executor.sentence` starts with "Run by the spike runner (*model*).",
   "Being run in chat by the chat agent, who claimed it" and "Being run by
   hand by", and that the spike's page carries the same sentence; then,
   after the chat spike is submitted, that both say "Run in chat by the chat
   agent.". The unclaimed chat and person sentences were already asserted on
   the page (lines cited above).
2. **FR-14.3.** `TestSpikeWorkingCopyChangeIsClaimActivity` claims a chat
   spike, lets its claim go stale and sees `claim-stale` raised, writes a
   file in the detached worktree, runs the sweep again, and sees the claim's
   last activity become `worktree` and the question withdrawn.
3. **The handoff** is
   [handoff-M14-stage2-2026-10-02](../notes/handoff-M14-stage2-2026-10-02.md),
   finalised with this result.

Both new tests pass, and the full suite was run again on the final commit;
the handoff records that run.
