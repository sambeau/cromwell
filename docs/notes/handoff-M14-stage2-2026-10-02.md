# Handoff — M14, spikes (stage 2)

**Date:** 2026-10-02
**Roadmap milestone:** M14 in the
[status report and roadmap](subutai-status-and-roadmap-2026-09-28.md) §11
**Branch:** `claude/subutai-m14-stage2-spikes`, from `main` at `36b7fed` (M13
and M14 stage 1 merged), pushed. No pull request, and nothing pushed to
`main`, as the brief asked. The integrator merges it.
**Spec:** [SPEC-021](../specs/SPEC-021-spikes.md) §4, **draft, for Sam's
approval**, checked in [REVIEW-021](../reviews/REVIEW-021-spikes.md) §6
**Development plan:** [dev-plan-M14-stage2](dev-plan-M14-stage2.md)
**Walkthrough:** [walkthrough-spec-021-stage2](../walkthrough-spec-021-stage2.md)
**Verification:** [VERIFICATION-021-stage2](../reviews/VERIFICATION-021-stage2.md)
**Migration:** `0016_spike_executors.sql`.

---

## Where it stands

A spike can now be run by the chat agent or by a person, as well as by the
spike runner. A person still starts every spike in the web UI, and now
chooses **Who runs it** on the start screen:
- **the spike runner**: an agent, with a token budget, as in stage 1;
- **the chat agent**: it claims the spike over MCP with `claim_spike`, saves
  findings with `save_spike_findings`, and hands them in with
  `submit_spike`;
- **a person**: from the spike's page, with **I'll run this spike**, a draft
  editor and **I've finished**.

A chat or person spike has a **time box** of whole hours (default 4) instead
of a token budget. Its deadline is fixed at the start and carried by M13's
claim as its `deadline_at`. When the deadline passes, claimed or not, the
heartbeat ends the spike as `time_box`: the findings are written from the
saved draft, committed, and the worktree is discarded, exactly as a
dispatched spike ends at its budget. M13's inactivity question
(`claim-stale`, "is someone still working on it?") still applies before the
deadline. The claim's `feature_id` is null, and the claim ends `expired` at
the time box, a new end reason.

Who ran a spike is recorded twice: who **may** run it on the spike's row
(`executor`), and who **did** in M13's `executions`, including an `agent`
row for a dispatched run, written when the run starts. Chat and person work
is **unmeasured**: its executions are `measured = false`, `tokens_used`
stays 0, and every surface says the tokens weren't measured rather than
showing 0; `get_spike` gives `tokens_used: null`.

Only a person starts or closes a spike, in the web UI (DEC-006 Amendment 1),
whoever ran it. The MCP tool-set test lists the three new tools on purpose
and forbids seven more names, alongside stage 1's `start_spike` and
`close_spike`.

**The orchestration note's four integration checks are four tests, and all
pass**: `TestChatSpikeIsHeldByItsClaimAndDeadline`,
`TestSpikeExecutorIsRecorded`, `TestChatSpikeTokensAreUnmeasured` and
`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`. So do the stage's
end-to-end tests, `TestChatSpikeEndToEnd` and `TestPersonSpikeEndToEnd`.
`TestSpikeCantBeClaimedYet`, which pinned stage 1's refusal, is replaced by
`TestSpikeIsClaimedOnlyByItsExecutor`.

**Nothing waits on Sam to merge**, by the orchestration note's rule. SPEC-021
waits for Sam's approval, now with twenty-three choices (DoD 10, choices 15
to 23 are stage 2's).

## Commits

| Commits | What they are |
|---|---|
| `2a1393f` | SPEC-021 §4 rewritten as a full stage-2 section |
| `827bec8`, `6b8745d`, `8cc5ac3` | The dev plan; REVIEW-021 §6 (the spec review) and the spec revised against it |
| `6bb71f8` | T1: migration `0016`, the store, the claim machine's `expire`, the config, the start's new shape, the lock helpers |
| `d94b172` | T3: the time box ends a spike; who ran it |
| `faa9890` (merged at `8e4ec84`, `794d743`) | T2: claiming a spike |
| `587a265` | T4: `claim_spike`, `save_spike_findings`, `submit_spike`, the `run-a-spike` chat skill |
| `153bb15` (merged at `654d8b4`) | T5: the start screen's **Who runs it**, and running a spike by hand |
| `0dd4279` (merged at `2296ade`) | T6: the integration checks and the end-to-end tests |
| `79f27e0`, `6ae7577`, `3967c3d` | Code-review round 1, and its fixes |
| `c5dbdaf`, `c039a0a`, `ab981c3` (merged at `e3f5a29`) | Code-review round 2, and its fixes |
| `a8c1233`, `faed33a` | Code-review round 3: no major on either side |
| `0a7cf5a`, `cba0efb` and later | The walkthrough |
| the rest | The roadmap mark, the plan's status, verification and this note |

## How it was built

As [How M13 and M14 are built](orchestration-M13-M14.md) says:

| Stage | Who | What came of it |
|---|---|---|
| 1. Spec | Lead | SPEC-021 §4 as SD-17 to SD-25 and FR-11 to FR-18, on SPEC-020's names |
| 2. Spec review | Opus subagent | REVIEW-021 §6: 5 material and 9 minor findings, all taken (SPEC-021 §8, "Stage 2"); SD-26 and SD-27 added |
| 3. Dev plan | Lead | Six tasks; the foundations' and wave 2's Go API fixed in advance |
| 4. Implementation | Sonnet subagents, one worktree and one Postgres cluster each | T1 alone; T2 and T3 together; T4 and T5 together; T6 |
| 5. Review cycle | Opus (bugs and conformance) and Sonnet (quality), in parallel | Round 1: 2 + 8 majors; round 2: 1 + 2 majors, and 2 from the walkthrough; round 3: **no major** on either side |
| 6. Verification | Opus subagent | VERIFICATION-021-stage2 |
| 7. Definition of done | Lead | Below |

## Definition of done

| Check | Result | Evidence |
|---|---|---|
| `git_status_clean` | **pass** | `git status --short` prints nothing on `claude/subutai-m14-stage2-spikes` after the last commit, pushed. |
| `tests_pass` | **pass** | TESTS_PLACEHOLDER |
| `contract_documents_approved` | **pending: Sam** | SPEC-021 is a draft, with stage 2's choices 15 to 23 added to DoD 10. |
| `all_tasks_terminal` | **pass** | T1 to T6 and both fixer passes are done and merged ([dev plan](dev-plan-M14-stage2.md), Status). |
| `reviews_present` | **pass** | Spec review: REVIEW-021 §6. Code reviews, three rounds of each: [round 1 bugs](../reviews/REVIEW-021-stage2-code-round1-bugs.md), [round 1 quality](../reviews/REVIEW-021-stage2-code-round1-quality.md), [round 2 bugs](../reviews/REVIEW-021-stage2-code-round2-bugs.md), [round 2 quality](../reviews/REVIEW-021-stage2-code-round2-quality.md), [round 3 bugs](../reviews/REVIEW-021-stage2-code-round3-bugs.md), [round 3 quality](../reviews/REVIEW-021-stage2-code-round3-quality.md). Round 3 found **no major** on either side. |
| `verification_passed` | VERIFY_PLACEHOLDER | VERIFY_EVIDENCE_PLACEHOLDER |
| `worktree_removed` | **pass** | `git worktree list` shows only `/home/user/cromwell`. Every subagent worktree was merged and removed, and its branch deleted. |
| `branch_merged` | **pending: the integrator** | Pushed to `claude/subutai-m14-stage2-spikes`, not to `main`. |

## What was built

**The database (FR-11).** Migration `0016` backfills `executor = 'agent'` on
every started spike and one inferred `agent` execution per `run-spike`
dispatch that ran, **then** replaces stage 1's budget-only checks: a started
spike has an executor; an agent spike has a budget and no time box; a chat
or person spike has a time box and a deadline and no budget; `time_box` is
only a chat or person ending, `budget` and `turn_limit` only an agent's.
`work_claims` gains the end reason `expired`.

**Starting (FR-12).** The start screen's **Who runs it**, the time-box field
(1 to 168 hours, default `spikes.default_time_box_hours`), and the start
notices. A chat or person start queues nothing and makes the detached
worktree straight after the commit, under its lock. The Start button is
disabled only when the findings template is missing.

**Claiming (FR-13).** `spike` is M13's second claimable. The interface gains
`lock`, `rules` and `releaseConsequence`. `ClaimSpike` takes the worktree's
lock (by the spike's ID, so it serialises with a start), re-reads the spike,
refuses before it makes anything, then goes through M13's `claimLocked`.
Only the executor chosen at the start may claim. A claim after the deadline,
including a renewal, is refused, and so is one after a submit. `SubmitSpike`
validates the findings as `finish_spike` does, holds the worktree's lock and
`spikeEndMu` through its ending, and ends the spike `concluded` on a context
that a cancelled request can't stop.

**Releasing and the inactivity question (FR-14).** A person releases a
spike's claim; the spike keeps running and its executor may claim it again.
The Inbox reads the release answer's words from the checkpoint, so a spike
reads **Release the claim** and a task's wording is unchanged. The sweep
judges a spike's deadline first and ends it, so `claim-deadline` is never
raised for a spike.

**The time box (FR-15).** `EndSpike(time_box)`, refused for an agent spike;
a spike with any claim ended `done` always ends `concluded`. Reconciliation
never treats a chat or person spike as "run was lost", keeps its directory,
and ends it at its deadline. The findings say "The spike reached the end of
its time box of 4 hours." and that the tokens weren't measured. The leak
check runs as before, and fails closed for a chat or person spike whose
working copy was made (recorded by `spike.worktree_made`).

**Where it shows (FR-16).** The executor line, the time-box line and the
unmeasured line on the spike's page; the person's claim panel and draft
editor; the chat claim's panel; the executor word in the lists;
`get_spike`'s `executor`, `measured`, `time_box_hours`, `deadline` and
`claim`; "Closed by sam, who also ran it."

**The boundary (FR-17).** Three new tools; seven more forbidden names; four
new `POST` routes, same-origin, none of which starts, closes or reopens a
spike in any state (`TestSpikeClaimRoutesNeverStartCloseOrReopen`).

## Choices for Sam

Stage 2 adds seven choices to SPEC-021's DoD 10 (now twenty-three). Each is
recorded where it stands in the spec:

15. the executor is chosen on the start screen and recorded on the spike,
    and who did the work in `executions` (SD-17);
16. a time box in whole hours, default 4, from 1 to 168, set on the start
    screen with no per-spike override at creation (SD-18);
17. a new MCP tool, `claim_spike`, rather than `claim_task` taking a spike
    (SD-19);
18. the deadline ends the spike without a question, and M13's inactivity
    question still applies before it (SD-20). This replaces choice 14;
19. a new claim end reason, `expired` (SD-21);
20. releasing a spike's claim only frees it for its executor to claim again
    (SD-22);
21. submitting ends the run with no reviewer (SD-23);
22. the person who ran a spike may close it and approve its findings
    (SD-26);
23. the leak check keeps reporting any new tag or stash, so a long time box
    may report a person's ordinary ones (FR-15.4).

## Decisions taken in the build

- **Every time a person or the chat agent reads is in UTC, and says so**
  ("17:04 UTC on 3 October"). The first build mixed UTC and the server's
  local time on one page.
- **A done claim is final.** Once a submit has been accepted, no claim or
  renewal is taken on the spike, and any ending concludes it. Round 1 found
  that a cancelled request could otherwise leave a spike to end at its time
  box after its findings were accepted.
- **`spike.worktree_made`** is audited when a chat or person spike's
  worktree is made, so the leak check knows the difference between a working
  copy that vanished (fail closed) and one that was never made.
- **A test-only hook, `Server.spikeHook`**, nil in production, makes the
  race tests deterministic.
- **A chat or person spike's page says "This spike has ended"**, not "This
  spike's run has ended", and its draft panel "If the time box ends now".
- **The agent spike's `executions` row is written when the run is marked
  running**, as M13 writes an implementer's, by `RecordAgentExecution`,
  which now takes the reference type.

## Where the brief or the first draft changed

- **S2-3's "`claim_task`'s spike form, or `claim_spike` if SPEC-020 keeps
  claims per entity"**: SPEC-020 does, so it is `claim_spike` (choice 17).
  `claim_task` given a spike's ID points to it.
- **S2-2's `spikes.default_time_box`** is `default_time_box_hours`, an
  integer, as M13's `claims.expiry_hours` is.
- **S2-6's hard expiry and the brief's "the claim sweep already raises
  `claim-deadline`"**: the sweep doesn't raise it for a spike; it ends the
  spike, because the time box is a stop, not a question (SD-20, choice 18).
- **DEC-007 decision 2** (whoever does the work can't judge it) and a person
  closing a spike they ran: SD-26 reads closing as the asker's reading of
  the answer, not an independent judging stage, and records it (choice 22).

## A live smoke checklist for Sam

With a real chat client connected over MCP, on the smoke project:

1. Create a spike on a feature, open its start screen, choose **The chat
   agent** and a time box of 1 hour, and start it.
2. In chat, ask the agent to run `SPK-…`. Check that it calls `claim_spike`,
   works in the working copy the result names, and saves findings with
   `save_spike_findings` early. Check the spike's page shows "Being run in
   chat by the chat agent" and the time left.
3. Ask it to submit. Check the findings are in the template's sections, the
   spike has ended, its findings are committed, and `git worktree list` and
   `git branch -a` are as before the start.
4. Start a second chat spike with a 1-hour time box, let the agent claim it
   and save something, then leave it. After the hour, check it ended at its
   time box with the saved draft, and that the claim ended `expired`.
5. Start a spike for **You**, run it from its page (claim, save, finish),
   and close it as answered. Check the page says you also ran it.
6. Ask the chat agent to close a spike, or approve its findings without your
   words. Check it can't.
7. Check `get_spike` over MCP shows `"measured": false` and `"tokens_used":
   null` for the chat spike.

## Bug-report candidates (minor findings still open)

From the three review rounds and the verifier, ready for M12's triage queue.
None is a major.

1. **The cancelled-request path of a submit is untested**: the ending runs on
   `context.WithoutCancel`, but no test cancels a request between the claim's
   `done` and the ending (there is no deterministic seam). Reconciliation
   would conclude the spike in any case.
2. **`TestAClaimThatWaitedOnAnEndingMakesNothing`'s name claims more than it
   proves**: it shows the claim read a running spike, not that it waited on
   the lock (round 3, S2Q3-3).
3. **`TestAClaimOvertakenByAStartWaitsForTheStartsWorktree` ends with a
   300 ms negative wait**, and leaks its goroutine and hook if it fails
   (S2Q3-2).
4. **The test seam's points are bare strings**; a typed value would catch a
   typo (S2Q3-1).
5. **Test helpers are spread over six files**, and
   `integration_claims_spikes_test.go` and `integration_spikes_claims_test.go`
   are each other's reverse (S2Q2-3, S2Q2-4).
6. **Package-level names without a spike prefix** (`executorPhrase`,
   `tokensNotMeasured`, `unmeasuredExecutor`, `noSubmitRelease`), and the
   executor's words are still said in more than one place (S2Q2-10).
7. **The UI routes resolve the spike again by public ID** after reading it
   from the form, because the service takes a public ID (S2Q11, S2Q2-12).
8. **The start screen's executor choice** in `startScreenFor` is hard to read
   (S2Q12).
9. **`spikeExecutorSentence` takes the actor from the latest execution and the
   claim time from the latest claim**; no test pins that they agree
   (S2Q2-14).
10. **`spikeActionWords`' field names**, and a silent garbled sentence for an
    unknown action (S2Q3-5); **`spikeLeftWords`' dead branch** (S2Q3-6); the
    person check made twice in `CloseSpike` (S2Q3-7); near-identical names
    (`startedViaScreen`, `startViaScreen`; `closerRanIt`,
    `closerRanItAudit`) (S2Q3-8).
11. **One `lacks(…, "disabled")` over the whole page** remains in a UI test
    (S2Q3-9), and **the two Ask again forms duplicate their inputs** (S2Q3-10).
12. **The start screen still shows the spike runner's panels** under a chosen
    **You** or **The chat agent**. FR-12.1 allows it, labelled, but it is busy
    (the walkthrough).
13. **`claim-stale` with the defaults rarely fires before a 4-hour deadline**
    (24-hour expiry). SD-20 says so; a project may want a shorter expiry for
    spikes.
VERIFY_CANDIDATES_PLACEHOLDER

## Traps for the next session

- **A chat or person spike has no dispatch.** Code that reads a spike's run
  must ask its executor first; reconciliation does.
- **Three in-process locks, in one order** (SD-27): the worktree's lock, then
  `spikeEndMu`, then rows (the spike, then its claim). Neither lock is
  re-entrant: code holding both calls `endSpikeLocked`, not `EndSpike`.
  `ensureSpikeWorktree` takes `spikeEndMu` itself, so its callers mustn't
  hold it.
- **`ClaimSpike` locks the worktree path built from the spike's ID**, not the
  row's `worktree_path`, so it serialises with a start that hasn't committed.
- **Any claim ended `done` makes a spike's ending `concluded`**
  (`store.ClaimEndedDoneFor`), not only the latest claim.
- **`Server.spikeHook` is a test seam.** Leave it nil outside tests.
- **Times are said in UTC.** Tests compare with UTC times.
- **Parallel test runs need separate Postgres clusters** (`SUBUTAI_TEST_PGDATA`
  and `SUBUTAI_TEST_PGPORT`) or `SUBUTAI_TEST_DB_SUFFIX`; agent worktrees are
  cut from `main`, so merge the branch into them first. An agent that changes
  the main checkout (a reviewer once left it detached) must work in its own
  `git worktree add` copy instead.
- **A worktree the harness made for an agent may stay locked** after the
  agent finishes; when it is clean and its branch is merged, `git worktree
  remove -f -f` is safe.
- **This branch should be deleted after the merge.** If this session can't,
  the command is `git push origin --delete claude/subutai-m14-stage2-spikes`.
