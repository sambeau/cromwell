# REVIEW-021: Consistency review of SPEC-021 (Spikes)

**Status:** Complete. The author's disposition of each finding belongs in
[SPEC-021](../specs/SPEC-021-spikes.md) §8. The spec awaits Sam's approval.
**Date:** 2026-10-02
**Reviewer:** an independent Opus subagent, not the spec's author.
**Scope:** [SPEC-021](../specs/SPEC-021-spikes.md) (first draft, commit `e1daef9`), checked against:
- [DESIGN-010](../design/DESIGN-010-subutai.md): §4, §5b, §5c, §7, §8, §10, §11 and §17a item 7;
- [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md), [DEC-006](../decisions/DEC-006-humans-start-development.md) with Amendment 1 and its notes, and [DEC-007](../decisions/DEC-007-the-judgement-boundary.md);
- the [status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md): §11 (M14) and §12 decision 13;
- [how M13 and M14 are built](../notes/orchestration-M13-M14.md);
- [SPEC-011](../specs/SPEC-011-send-to-development.md), [SPEC-012](../specs/SPEC-012-see-the-work.md), [SPEC-015](../specs/SPEC-015-documents-with-identity.md), [SPEC-018](../specs/SPEC-018-decisions.md) and [SPEC-019](../specs/SPEC-019-bugs.md);
- the code at `e1daef9`:
  - `internal/dispatch/dispatch.go`;
  - `internal/store`: `dispatches.go`, `checkpoints.go`, `identity.go`, `observe_reads.go`, `corpus.go`, `migrate.go`, and migrations `0001`, `0005`, `0010`, `0012` and `0013`;
  - `internal/server`: `planner.go`, `toolexec.go`, `worktree_ops.go`, `actions.go`, `actions_phase2.go`, `review_send.go`, `documents.go`, `edit.go`, `identity.go`, `decisions.go`, `observe.go`, `ui_send.go`, `ui_entity_pages.go`, `mcp.go`, `mcp_relay.go`, `mcp_tools.go`, `http.go`, `server.go` and `integration_mcp_test.go`;
  - `internal/rules/rules.go` and `rules_phase2.go`, `internal/lifecycle` (`document.go`, `gates.go`, `validate.go`), `internal/toolhost/toolhost.go`, `internal/ident/ident.go`, `internal/config/compartment.go`, `internal/bus/bus.go`, `internal/timeline/timeline.go` and the starter pack.

## 1. What this review is

This is a consistency pass by a reviewer who did not write the spec. It checks:
- that SPEC-021 agrees with the approved documents;
- that what it says about the code is true;
- that its requirements can be built without guessing, and tested;
- that its two central claims, the hard stop and no merge path, would hold.

It doesn't fix the spec.

**Overall.** The spec's shape is right. A spike as its own table (SD-1) is the
correct call, and the reasoning against reusing `features` is sound. The
web-UI-only start, the detached worktree, the per-turn token callback and the
`approved_by: human` findings all fit the code that exists. Several claims
check out exactly (§4 below).

The weak point is **the findings document while a run is live**. The spec
treats it as an ordinary draft that the agent overwrites in the main checkout.
But a person, the chat agent and the CLI can all still submit, approve or edit
that draft, and the server writes it again when the run ends. The existing
rules then treat the result as tampering with an approved document.

Other material findings:
- the exhaustion path, which would raise the generic Retry-or-cancel question
  and let a person restart an ended spike;
- the lack of any recovery if the server stops between a run ending and
  `EndSpike` running;
- how the budget is counted when two attempts overlap, and what a retry does
  to the saved findings;
- a default budget that, given how tokens are counted, stops most spikes
  after about ten turns;
- a "no merge path" claim stronger than the mechanism;
- "The question is answered", which can't be done in one transaction as
  written;
- a stage-2 time box that contradicts what DEC-007 says claim expiry is.

## 2. Material findings (must fix before approval)

### R21-1 — The findings document can be submitted, approved or edited by others while the run writes to it, and `EndSpike` would then overwrite a document under review or approved

**Severity:** material. **Where:** SD-8, SD-9, FR-4.3, FR-6.2 step 1, FR-7.

**What the spec says.** The findings are an ordinary draft from creation
(SD-9). `save_findings` replaces the body in the main checkout without
committing, and refuses once the document "isn't a draft any more" (FR-4.3).
`EndSpike` writes the final body and the ending section, then commits
(FR-6.2).

**The problem.** Nothing stops anyone else acting on that draft during the
run, and the spec never says what `EndSpike` does when it finds the document
in a state other than draft.
- **A person can submit and approve it in the web UI.** The document page
  offers Submit for any draft (`internal/server/ui_send.go:475`). The findings
  are a human-approval type, so the approval panel follows
  (`ui_send.go:509-514`, `actions.go:162-165`).
- **The chat agent can submit it and relay an approval.** `submit_for_review`
  and `relay_verdict` are on the advertised set
  (`integration_mcp_test.go:147, 152`). `SubmitFromChat` checks no owner type
  (`review_send.go:756-798`), and `relay_verdict` calls `DirectApprove` for
  any document in review (`mcp_relay.go:256`). One chat submission is enough
  to make every later `save_findings` fail for the rest of the run, so the
  chat agent can end a spike's useful work without starting or closing
  anything.
- **The CLI can submit it** through `POST /api/docs/submit`
  (`http.go:64`).
- **When the run ends, `EndSpike` writes the file regardless.** For an
  approved document, the watcher's rule raises a `document-integrity`
  checkpoint: "Approved document … was modified on disk. Revert the file or
  create a revision." (`internal/rules/rules.go:626-641`). For a document in
  review, the text changes under the review, which SPEC-016 SD-9 forbids:
  `SaveEdit` withdraws first (`edit.go:277-283`), but `EndSpike` doesn't.
- **The browser editor and `save_findings` collide.**
  - `save_findings` leaves the file dirty and records no hash. A person's
    **Save & commit** is then refused with "has changes that aren't
    committed, made outside this editor" (`edit.go:265-271`), because
    `subutaiWrote` doesn't recognise the text (`edit.go:362-391`).
  - A plain **Save** succeeds, and the agent's next `save_findings` silently
    replaces it, because `save_findings` has no base hash.
  - The same happens to anything a person wrote in the draft while the spike
    was still an `idea`. The first `save_findings` replaces the whole body.

**Suggested fix.** The simpler design is to keep the live findings out of the
document until the run ends:
1. `save_findings` writes to a column on the spike (`findings_draft`), not
   the file. The main checkout is never dirty mid-run, and the editor, the
   watcher and the document lifecycle never see a half-written body.
2. The spike's page shows the latest saved draft while the spike is
   `running`.
3. `EndSpike` writes the file once and commits it.
4. While the spike is `idea` or `running`, the findings document refuses
   submit, approve, send-back and edit, each with a sentence ("This spike is
   still running, so its findings can't be changed or reviewed until it
   ends."). The refusals sit in `docActionsFor`, `SubmitFromChat`,
   `submitDocWith` (which covers `/api/docs/submit`), `DirectApprove`,
   `SendBack` and `editRefusal`.
5. The planner puts any text a person wrote in the draft before the start
   into the prompt, so it isn't lost.

If the document-backed design is kept instead:
- add the refusals in step 4 anyway;
- give `save_findings` a hash check and an audit row that `subutaiWrote`
  recognises;
- say what `EndSpike` does for each document state.

Add a test for each refusal and for each surface.

### R21-2 — Exhaustion raises the generic Retry-or-cancel question, and Retry would restart an ended spike from the web UI or the CLI

**Severity:** material. **Where:** SD-15, FR-3.4, FR-6.1.

**What the spec says.** "The rules engine decides `EndSpike` for a
`run-spike` dispatch's … exhaustion" (FR-6.1). No `/api` route or other
surface "moves a spike to `running` or enqueues `run-spike`" (FR-3.4).

**The problem.**
- Today every `DispatchExhausted` becomes a `dispatch-failure` checkpoint,
  "Retry or cancel?" (`internal/rules/rules.go:395-402`). The spec adds
  `EndSpike` but doesn't remove the checkpoint.
- Answering Retry re-queues the same dispatch row (`actions.go:212-219`,
  `store.RequeueDispatch`). That can be done from the Inbox or from the CLI
  through `POST /api/respond` (`http.go:68`).
- By then `EndSpike` has ended the spike and discarded its worktree. The
  retried run then fails in the planner, or worse, runs against a spike that
  is `ended`. That is the CLI route FR-3.4 says doesn't exist.
- An exhausted dispatch also stays `failed`, so `RetrySweep` publishes
  `DispatchExhausted` again on **every heartbeat** (`dispatch.go:580-590`,
  `store.FailedRetryable` at `dispatches.go:365-388`) until someone cancels
  it. `EndSpike` being idempotent hides that, but the spike's run row stays
  "failed, awaiting a decision" forever.

**Suggested fix.**
1. For `purpose = run-spike`, the rules return `EndSpike` *instead of* the
   checkpoint.
2. `EndSpike`, in its transaction, marks an exhausted run `cancelled`
   (`store.MarkDispatchCancelled`), so the sweep stops publishing it.
3. The planner refuses a `run-spike` whose spike isn't `running`, as a
   permanent failure, as a guard.
4. Test that no `dispatch-failure` checkpoint exists after an exhausted
   spike, and that `/api/respond` can't requeue one.

### R21-3 — A spike can stick in `running` for good: the end is driven by an in-memory event, and there is no reconciler

**Severity:** material. **Where:** FR-3.3, FR-6.1 to FR-6.3, "server restart
mid-run".

**The problem.**
- **The bus is in-process** (`internal/bus/bus.go:1-4`). `DispatchSucceeded`
  is published after the success commits (`dispatch.go:348-366`). If the
  server stops after that commit and before the rules run `EndSpike`, the
  dispatch is `succeeded` and the spike is `running` for ever:
  - nothing re-publishes the event;
  - boot recovery covers stalled dispatches, documents, worktree rows and G1,
    but nothing for this case (`server.go:131-146`).
- **The same happens if `EndSpike` fails part-way.** For example, if the
  findings commit fails in step 2, the action's error is logged and nothing
  retries it.
- **FR-3.3 reverses the established order.** It adds the git worktree
  *before* the transaction. `StartFeature` deliberately commits the database
  first and adds the worktree after, raising a checkpoint if git fails
  (`worktree_ops.go:33-90`, DESIGN-006 §3). With the spec's order:
  - a crash between the two leaves `worktrees/spk-<id>` on disk with no row
    naming it;
  - the next **Start** for that spike then fails in `git worktree add`,
    because the path exists, and is "refused with the reason", for ever.
- **FR-6.3's boot step can't be built as written.** It says "any other
  spike's leftover worktree is removed", but nothing says how a leftover is
  found when no row records it.
- **Two concurrent `POST`s** both pass the `idea` check. The second fails
  only because git refuses the existing path, with git's message rather
  than a sentence.

**Suggested fix.** Add **FR-6.4, reconciliation**, run at boot and on the
heartbeat:
- a `running` spike whose `run-spike` dispatch is `succeeded`, `cancelled`,
  or `failed` with no attempts left gets `EndSpike`;
- `EndSpike` is safe to re-run from any step: it commits the file only if
  it differs from `HEAD`, moves the state only from `running`, and
  discards the worktree only if `worktree_removed_at` is null;
- a directory under `worktrees/spk-*` that no live spike names is removed.

For the start, either follow `StartFeature`'s order (move to `running`,
then add the worktree, then raise a checkpoint on failure), or keep the
spec's order with two changes:
- the transaction moves the state conditionally
  (`UPDATE … WHERE state = 'idea'`) and refuses with a sentence when no row
  changed;
- the worktree step removes a stale directory at the path first.

Test a restart between the dispatch success and `EndSpike`.

### R21-4 — The budget check reads a stale total when attempts overlap, and the order of work in the last turn isn't defined

**Severity:** material. **Where:** SD-7, FR-5.1 to FR-5.3.

**What the spec says.** "The planner gives the loop `tokens_used` as the
earlier attempts' total". The loop stops when "earlier attempts plus this
attempt" reach the limit. The checks run "before each model call *after the
first*" (FR-5.1). A retry "stops before its first call" if the budget is
already used (FR-5.3).

**The problem.**
- **Two attempts can run at once.** The stall sweep fails an attempt whose
  heartbeat is old (`dispatch.go:593-612`), and the retry sweep re-queues it
  while the old goroutine is still inside a long model call. The old
  attempt only notices at its next `stillCurrent` (`dispatch.go:404, 421`).
  Both spend tokens, and both add to `spikes.tokens_used`. But the new
  attempt checks against *planner snapshot + its own usage*, so it misses
  what the old one spent after the snapshot.
- **Where the callback sits matters.** If it is placed after the
  `stillCurrent` at line 421, a given-up attempt's last call, already paid
  for, is never counted.
- **FR-5.1 and FR-5.3 disagree.** FR-5.1 exempts the first call from the
  checks, and FR-5.3 needs a check before it.
- **What happens on the turn that crosses the budget is undefined.** The
  existing loop handles the outcome tool first and runs the other tools
  after (`dispatch.go:452-477`). The spec only says "after each turn …
  stops". If the stop is checked before the turn's tool calls run:
  - a `save_findings` in the very turn that crossed the budget, often the
    agent's best write-up, is dropped;
  - a valid `finish_spike` in that turn ends the run as `budget` rather than
    as concluded.

**Suggested fix.**
1. The callback is
   `UPDATE spikes SET tokens_used = tokens_used + $1 WHERE id = $2 RETURNING tokens_used`,
   called straight after `completeWithRetry` returns and before
   `stillCurrent`. The loop compares the **returned database total** with
   the limit, so overlapping attempts see each other.
2. Fix the order for a turn:
   1. record the usage;
   2. if the turn holds a valid `finish_spike`, end as concluded;
   3. run any `save_findings` calls in the turn;
   4. if the total has reached the limit, stop with `budget`;
   5. otherwise run the other tools.
3. Before **every** call, stop if the total has reached the limit. After
   the first call of an attempt, also stop if total plus the lower bound
   would pass it.
4. Add tests for a turn that saves and crosses the budget, and for a
   finishing turn that crosses it.

### R21-5 — A retry starts from a fresh context, and its first `save_findings` overwrites what the failed attempt saved

**Severity:** material. **Where:** FR-4.1, FR-5.3, SD-15.

**The problem.** Each attempt starts with a fresh context
(`store.RequeueDispatch`, `internal/store/dispatches.go:246-247`). The planner builds the
same prompt every time. FR-4.1 includes earlier findings only for a spike
that **asks again**, never for a retry. So attempt 2 knows nothing of
attempt 1's work. Its first `save_findings`, which replaces the whole body,
destroys attempt 1's findings. The hard stop then keeps the second, poorer
body. That breaks the second supporting claim: "its findings are written
up".

**Suggested fix.** For attempt 2 and later, the prompt gains "# What you have
saved so far", holding the current saved findings, and a sentence: "An
earlier attempt at this spike stopped. Build on these findings rather than
starting again." Add an acceptance case to `TestSpikeBudgetCarriesAcrossAttempts`:
the retry's prompt holds attempt 1's saved findings.

### R21-6 — The default budget would stop most spikes after about ten turns, and choices 5 and 14 are put to Sam without the arithmetic

**Severity:** material. **Where:** SD-6, SD-7, FR-10.1, FR-10.2, §6 DoD 9.5
and 9.14, §7.

**The problem.**
- **Every call is charged for the whole conversation again.** "Tokens used"
  sums every call's input, output, cache reads and cache writes. Each call
  re-sends the whole conversation, so the sum grows with the *square* of the
  number of turns.
- **The demo scripts show it.** Input per call climbs 6,410, then 7,180,
  then 8,020 (`internal/server/demo_m6_test.go:79-93`). A spike that reads
  a few files will pass 15,000 to 25,000 tokens of context within a handful
  of turns.
- **200,000 tokens is roughly 10 to 15 turns.** With a context of about
  5,000 tokens plus about 3,000 a turn, the running total passes 200,000
  between turn 10 and turn 12. So:
  - the turn cap of 60 (FR-10.2) can't be reached;
  - §7's "a 40-turn run … can pass that" understates the gap by an order of
    magnitude.
- **The SD-6 alternative isn't what it seems.** The provider client sets no
  `cache_control` anywhere (`internal/provider` has no reference to it), so
  cache reads are close to zero today. "Input and output only" therefore
  counts the same quadratic total, not a smaller one. The alternative offered
  to Sam isn't the real one.

**Suggested fix.**
1. Put the arithmetic in SD-6, using a real transcript's per-turn usage.
2. Set the default from it: either about 1,000,000 tokens with a turn cap
   near 40, or 200,000 with a stated expectation of about 10 turns and a
   turn cap to match.
3. Restate choice 5's alternative honestly. A truly different measure would
   be *new* tokens per call (output plus the growth in input), which grows
   roughly linearly. Name it if Sam should weigh it.

### R21-7 — "Nothing can merge its code" is stronger than the mechanism, and NFR-4's commit test can't run with the starter commands

**Severity:** material. **Where:** §1 goal, SD-2, NFR-4.

**The problem.**
- **The agent can run code it wrote.** `run_command` runs only whitelisted
  commands, but it appends the agent's arguments (`toolhost.go:142-150`) and
  runs them in the worktree (`toolexec.go:141-171`). Whitelisted build and
  test commands run whatever code the agent wrote there. A `go test`
  executes test files the agent created, and a `make` executes a Makefile it
  edited. So the agent can run `git` indirectly.
- **Branches made in a linked worktree survive it.** A linked worktree
  shares the repository's refs. A `git branch keep` or `git push` run from
  agent code creates a branch that outlives `git worktree remove --force`.
  SD-2 fact 4 ("unreachable once the worktree goes") then fails, and
  anyone, a person or another feature, could merge that branch.
- **The fact is true of Subutai's own code, not of everything.** SD-2's
  facts 1 to 3 hold for Subutai itself. "Enforced, not promised"
  (DESIGN-010 §10) needs an end-of-run check, not only absence.
- **NFR-4's test can't run as written.** It needs "a commit the agent makes
  in the worktree (through `run_command`)". The starter's commands are
  `["true"]` (`internal/starter/starter.go:94-102`), so the test needs its
  own whitelist entry, and the spec doesn't say so.

**Suggested fix.**
1. Record the start commit on the row.
2. At `EndSpike`, before the discard:
   - if the worktree's `HEAD` moved, run
     `git for-each-ref --contains <worktree HEAD>`;
   - any ref it lists is a leak. Audit it, raise a checkpoint naming the
     ref, and say so in "How this spike ended".
3. Word the goal as "Subutai has no path that merges a spike's code, and a
   run that tries to keep its code is caught".
4. Give NFR-4's test a whitelisted command that runs a script, which makes
   a commit and a branch. Assert that the branch is reported.

### R21-8 — "The question is answered" can't approve draft findings "in one transaction" through the document service

**Severity:** material. **Where:** SD-10, FR-7.2.

**The problem.**
- **The transition table has no path from draft to approved.** A draft can
  only be submitted (`internal/lifecycle/document.go:44-57`).
- **No approval entry point will take a draft.** `DirectApprove` refuses
  anything not in review: "Only a document in review can be approved …
  Submit it for review first." (`review_send.go:442-444`).
  `HumanApproveDocument` relies on the same table (`actions.go:600-613`).
- **The approval runs in its own transaction.** `approveDocumentAs` opens
  its own transaction and publishes `DocumentTransitioned` after it commits
  (`actions.go:326-437`), so it can't share one with closing the spike.

So FR-7.2 needs a submit and then an approval: two transitions, two
transactions and two bus events.

**Suggested fix.** Choose one of these:
- **(a) Sequence it honestly.**
  1. Validate.
  2. Submit the findings if they are a draft.
  3. Approve them with `DirectApprove` and the person's verdict.
  4. Close the spike in its own transaction.

  If step 3 succeeds and step 4 fails, the reconciler of R21-3 closes the
  spike, because its findings are approved and it is `ended`.
- **(b) Decouple closing from approval (simpler).** "The question is
  answered" closes the spike, and the findings follow their own lifecycle.
  DESIGN-010 §10 only requires that "someone reads the findings and says the
  question is answered".

Either way, drop "in one transaction" from FR-7.2 and NFR-1 for this act.

### R21-9 — Stage 2 makes claim expiry a hard stop, which isn't what DEC-007 says expiry is, and the stage-1 schema blocks stage-2 rows

**Severity:** material. **Where:** §4, S2-6; FR-1.1; §6 DoD 9.15.

**The problem.**
- **Expiry is a checkpoint, not a stop.** DEC-007 decision 6 defines claim
  expiry: no activity for a configured time "raises a 'still working on
  this?' checkpoint". DESIGN-010 §5b says the same. S2-6 says "M13's claim
  expiry ends the run", and runs the time box "from the claim, not from
  activity".
  - That is a different mechanism: a fixed deadline, not inactivity.
  - It is a stop, not a question.
  - M13 is specified, by DEC-007, to build the inactivity checkpoint, not
    this.
  - DESIGN-010 §10's "enforced through claim expiry" was written with
    DEC-007's meaning.
- **There are two clocks.** S2-6 runs one from the claim, and one from the
  start for a spike nobody claims.
- **The stage-1 checks would refuse stage-2 rows.** FR-1.1 requires
  `token_budget` whenever the state isn't `idea`, and limits `ended_how` to
  four values. A chat or person spike has no token budget and ends as
  `time_box`.

**Suggested fix.**
1. Say plainly that the time box is a **spike deadline** the heartbeat
   enforces, set at the start (one clock), and not M13's expiry. M13's
   inactivity checkpoint still applies to the claim.
2. Put choice 15 to Sam on that basis.
3. Either write FR-1.1's checks so the `executor` and `time_box` columns can
   be added in a later migration, or say now that stage 2's migration
   replaces them.

## 3. Minor findings

### R21-10 — The worktree holds a committed copy of the findings, so one acceptance check is false and the agent can be misled

**Severity:** minor. **Where:** FR-1.3 step 5, FR-3.3 step 1, FR-4.1, FR-4.3
acceptance.

- The findings file is committed when the spike is created, and the worktree
  is made from `HEAD`. So the worktree contains the template copy at the
  same path. `TestSaveFindingsWritesTheMainCheckout`'s "the worktree has no
  findings file" will fail.
- The agent may read or edit that stale copy and believe it has saved.
- FR-4.1's "paths of its current documents" resolve inside the worktree, at
  `HEAD`. They don't show changes in the main checkout that aren't
  committed.

Fix:
- assert instead that the worktree's copy is unchanged;
- tell the agent in "# Your working copy" that the findings file there is a
  stale copy and that `save_findings` is the only way to save;
- say that the documents it reads are as committed.

### R21-11 — Template placeholders fail validation, and `EndSpike` fills only the Answer

**Severity:** minor. **Where:** FR-1.3 step 3, FR-2.1, SD-8.

- Validation rejects any unresolved `{{…}}` or `TODO` in a section
  (`internal/lifecycle/validate.go:98-114`). So a freshly created findings
  file with placeholder Answer and What we found **can't pass** "fills and
  validates the findings from the template". The spec must mean a narrower
  check (front matter and identity), and should say so.
- SD-8 fills an empty or placeholder Answer but leaves What we found. A run
  that never saved therefore ends with findings that can't be closed as
  answered, and the spec doesn't say that this is intended.

Define:
- what counts as a placeholder (the template's own `{{…}}` text);
- that `EndSpike` fills both required sections ("Nothing was saved before
  the run stopped.").

### R21-12 — "Answered" means two different things, and "five states" lists four

**Severity:** minor. **Where:** SD-4, FR-1.1, FR-4.4, FR-8.1, §6 DoD 9.4.

- `ended_how = 'answered'` means "the agent concluded". FR-4.4 admits as
  much: "whether the question *is* answered is the person's call".
- `closed_as = 'answered'` means a person says it is answered.

One word, one meaning (writing guide §11). A page reading "Ended: answered"
next to an **The question is answered** button will mislead. Rename the first
to `concluded` ("Ended: the agent reached a conclusion.").

SD-4 and DoD 9.4 say "five states", but the `state` column has four values.
Say "four states, and two ways to close".

### R21-13 — Places that switch on owner or reference type are missing

**Severity:** minor. **Where:** FR-1.2, FR-8.3, FR-8.4, NFR-6.

A document owned by a `spike` reaches code that knows only the project,
initiatives and features:
- `ownerCrumb` falls back to **Project**, so the findings' crumbs are wrong
  (`ui_entity_pages.go:158-185`);
- `store.OwnerPublicID` (`internal/store/identity.go:21-37`, named in FR-1.2);
- `resolveOwner` (`documents.go:103-121`);
- `mcpResolveOwner` (`mcp_tools.go:326-349`);
- `scopeForDocument` (`decisions.go:398-406`);
- `snapshot` (`actions.go:51`).

FR-8.3 also needs a **store** change, not only one in `internal/timeline`.
`FeatureHistory` and `FeatureRuns` read through the `featureRefs` CTE
(`internal/store/observe_reads.go:51-61`), which must gain the feature's
spikes. Say:
- whether `run-spike` runs then appear in the feature's list of runs and
  its token totals;
- that they stay out of `ActualTokens` (`internal/store/corpus.go:27-57`,
  correct today) and so out of calibration.

List these sites as an appendix, as REVIEW-019 R19-1 recommended for bugs.

### R21-14 — Config checks: `run-spike` isn't a worktree purpose, and the `save_findings` check can be bypassed

**Severity:** minor. **Where:** SD-16, FR-10.3, FR-9.

- `isWorktreePurpose` names only `implement-task`, `review-code` and
  `verify-feature` (`internal/config/compartment.go:44-46`).
  - If a project adds `report_bug` to `spike-runner`, the loader's refusal
    names the wrong reason: "never offered worktree tools"
    (`compartment.go:449-455`).
  - If the check were relaxed, `toolReportBug` would run with no feature in
    its context.

  Add `run-spike` to `isWorktreePurpose`, and add an explicit refusal of
  `report_bug` for `run-spike` with its own sentence.
- FR-10.3's check walks `cfg.Assignments` (`compartment.go:431`). A role
  that declares `save_findings` but isn't assigned to anything passes it.
  Check every role file instead.
- Add `save_findings` and `finish_spike` to the MCP forbidden list. They are
  the dispatched agent's tools, as `submit_review` is (`integration_mcp_test.go:195`).

### R21-15 — G5 can be overridden, and the spec doesn't say what happens to an open spike then

**Severity:** minor. **Where:** SD-14.

- G5 "is overridable only via an answered gate-override checkpoint"
  (`internal/lifecycle/gates.go:133-134`). The spec doesn't say what an
  override does to a running or ended spike under that initiative.
- It doesn't say whether G5 counts spikes in the whole subtree, as it counts
  features, or only directly owned ones.
- It doesn't say what abandoning a feature does to its open spikes. Nothing
  blocks the abandonment, and the spike runs on, owned by an abandoned
  feature.

### R21-16 — The transcript and the run page don't say a run stopped at its budget

**Severity:** minor. **Where:** FR-5.1, FR-8.4.

- A failed attempt's transcript ends with why it failed (SPEC-012 SD-3,
  `store.MarkDispatchFailed`). A budget or turn-limit stop isn't a tool call,
  so as specified the transcript just stops.
- Add an entry: "The run stopped here because it reached its budget of
  40,000 tokens."
- Say how the run page shows the outcome `{"ended":"budget"}`.

### R21-17 — Some acceptance criteria aren't deterministic or don't match the code

**Severity:** minor. **Where:** FR-5 and NFR-4 acceptance.

- `TestSpikeStopsHardAtItsBudget`: "stops after the turn that reaches the
  budget or the turn the lower bound rules out" allows two results. With a
  budget of 1,000 and 150-token turns, the lower bound stops it before call
  7. State it exactly: six calls, `tokens_used` = 900, and the page says
  "900 of 1,000".
- NFR-4: "`start_building` … refuse[s] a spike's ID". `start_building`
  doesn't exist as a tool; it is already forbidden by name. Say what is
  meant instead: `StartFeature` and `/api/features/start` refuse a spike's
  ID because it isn't a feature path.

### R21-18 — The coordination claim and the definition of done don't match the build process

**Severity:** minor. **Where:** header "Coordination", NFR-6, §6.

- "Stage 1 touches only these shared files: `mcp.go`'s tool list, [the
  test], and `entity.html`" isn't accurate.
  - Stage 1 also changes `dispatch.go`, `config.go` (the `spikes` section),
    `compartment.go`, `server.go` (boot and heartbeat duties), `rules.go`,
    `toolexec.go`, `planner.go` and `observe_reads.go`.
  - DEC-007's consequences put M13 in several of the same files: the
    executor field, unmeasured tokens in the ledger and calibration, the
    claim-expiry configuration and the heartbeat.

  List the likely overlaps, so that integration expects them.
- §6 leaves out three checks the orchestration note requires:
  - the development plan (stage 3), and `all_tasks_terminal`;
  - `git_status_clean`;
  - `worktree_removed`.

  "`gofmt -l` (no new files)" is unclear; it means the command lists no
  files.

### R21-19 — Prose

**Severity:** minor. **Where:** FR-2.2, FR-3.2, FR-7.4, FR-8.5, S2-7.

- **Not full sentences, against DESIGN-008 D-6:** "no forecast yet" (FR-3.2)
  and "Tokens: unmeasured (run in chat)" (S2-7). Suggest "There aren't
  enough earlier spikes to forecast this one yet." and "This spike ran in
  chat, so its tokens weren't measured."
- **"It used 41,210 of its 40,000 tokens"** reads as a contradiction. Use
  "It used 41,210 tokens, which is more than its budget of 40,000."
- **"The spike's agent concluded."** says nothing. Use "The agent reached a
  conclusion. It used 31,004 of its 40,000 tokens."
- **FR-8.5's link is ambiguous.** "linking to the first, or to a list when
  there are several (`/ui/spikes`)": say which, and specify the `/ui/spikes`
  page, which no FR defines.
- **FR-7.4 renders a screen from a POST.** "The response is the new spike's
  start screen" should be a redirect to it, so that a reload doesn't create
  a third spike.

## 4. Claims checked and found true

- The `SPK` sequence already exists (`ident.Kinds`, `internal/ident/ident.go:38`; migration `0010`, comment at line 14).
- Postgres refuses a new enum value used in the transaction that added it, and the migration runner wraps each file in one transaction (`internal/store/migrate.go:130-150`). Comparing `owner_type::text` avoids the problem, and `0013` shows the same pattern.
- The documents owner check is the unnamed `CHECK (owner_type IN ('project','initiative','feature'))` in `0001_init.sql:81`, and needs dropping by its generated name.
- `approved_by: human` with no reviewer role is supported and already used by `design` (`internal/config/compartment.go:277-289`, `templates/design/manifest.yaml`). Such a type gets the human approval panel.
- `RunSummary.Tokens()` is input + output, where input already includes cache reads and writes (`internal/store/observe_reads.go:45-47, 89`). SD-6's "the same sum the run pages show" holds.
- `MarkDispatchFailed` records no usage (`internal/store/dispatches.go:221-245`), so tokens from a failed attempt reach no ledger. FR-5.2's per-turn callback is the right fix, and FR-8.4 is honest about the difference.
- Surfacing takes an initiative scope (`surfaceScope`, `internal/server/decisions.go:381-394`), so FR-4.1's decisions block needs no new mechanism.
- `mergeFeature` takes a feature id and reads `LiveWorktreeForFeature` (`internal/server/actions_phase2.go:448-457`). A spike's id can't reach a merge through it.
- The MCP tool-set test checks for method-not-found on forbidden names (`internal/server/integration_mcp_test.go:199-208`), so FR-3's acceptance is testable as written.
