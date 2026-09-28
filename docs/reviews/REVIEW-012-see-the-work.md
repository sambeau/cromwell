# REVIEW-012: Consistency Review of SPEC-012 (see the work)

**Status:** Complete. The author has dealt with every finding (§6, and
[SPEC-012 §7](../specs/SPEC-012-see-the-work.md#7-changes-after-review));
approved by Sam, 2026-09-28 (§5)
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author. Approval is
Sam's, recorded in §5.
**Scope:** [SPEC-012](../specs/SPEC-012-see-the-work.md) (draft, commit
`a882adf`), against
[DESIGN-010](../design/DESIGN-010-subutai.md) §8 (binding), with §5 and §17;
[DESIGN-008](../design/DESIGN-008-the-workflow-surface.md) §5.2, D-4, D-6 and
D-10, and the round-4 principles cited in the templates;
the [discussion response](../notes/subutai-discussion-response-2026-07-31.md)
§5; the [status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11, M6; the [conformance audit](../notes/research-conformance-audit-2026-07-29.md)
item C-7; [DEC-006](../decisions/DEC-006-humans-start-development.md); the
M6 brief; the [writing guide](../research/writing-guide.md); and the code at
`a882adf`: the dispatcher, the store (dispatches, migrations, audit, documents,
tasks, and checkpoints), the rule engine's outcome parsing, the UI handlers and
templates, the heartbeat, the config, and the provider types.

## 1. What this review is

An authoring consistency pass by a reviewer who did not write the spec. It
checks that SPEC-012 agrees with the designs and decisions it builds on, that
what it says about the code is true, that it covers the M6 brief, and that its
requirements can be tested. It does not fix the spec; the author will fold the
findings in and record that in SPEC-012 §7.

One thing about timing. The transcript half of the build was committed while
this review was running (`7627497`: migration 0008, the recorder, the error
entry in `MarkDispatchFailed`, cutting and cleaning, the config section, and
the pruning sweep). Findings are about the spec, and code citations are to
`a882adf` unless marked otherwise. Where the new code already answers or
sharpens a finding, that is said, so the author can decide which of the two
is right.

Citations of the form `spec:NN` are line numbers in SPEC-012 at `a882adf`.

## 2. Findings

### R12-1 — A moment doesn't lead to the run that caused it, so the Goal's click-through fails exactly where it matters (material)

**What is wrong.** The Goal is that a person can "click from any moment
through to exactly what each agent was told, did and concluded" (spec:60-62),
and the brief asks for review rounds and escalations as moments, "each linking
to dispatches and transcripts". SD-6 and FR-5.4 attach a run to "the phase
that was current when it was queued" (spec:136-141, 375-378). A phase *begins*
at its moment, so the run that *caused* a moment always sits in the phase
before it:

- the code review that sent a task back was queued during *Building*, so it is
  not inside *Code review sent back (round 1)*; that moment's phase holds the
  re-dispatched implementer and the next review;
- the spec review that approved the spec sits under *Spec written*, not under
  *Spec approved*;
- the verifier that sent the feature back sits under *Verifying*, not under
  *Verification sent it back*.

So the three moments a person most wants to open ("why was this sent back?")
lead to the wrong runs. FR-5.2 also says each moment records "who caused it:
an agent run, the orchestrator, or the person named on the audit row"
(spec:363-365), but it never says how a moment is tied to a run, and the audit
rows don't carry one.

**Evidence.** The transitions an agent verdict causes are audited under the
role name with no dispatch id: `TransitionDocument(..., DocApprove, actor,
nil)` (`internal/server/actions.go:287`); `DocRequestChanges, a.Actor,
{"comments": n}` (`actions.go:392`); `TaskRequestChanges, a.Actor,
{"comments": n}` (`actions_phase2.go:417`). The payload that
`TransitionDocument` writes holds only `from`, `to` and `event`
(`internal/store/documents.go:98-124`). Escalations are the exception: the
escalation checkpoint's context carries `dispatch_id`
(`internal/rules/rules.go:517-523`), but the `checkpoint.created` audit row
carries only `checkpoint_id`, `kind` and `question`
(`internal/store/checkpoints.go:61-62`).

**Recommended fix.** Give every moment an optional **cause**, separate from its
phase, and add an FR that says how the cause is found. Two workable rules:

1. For verdict moments (approve, request changes, rework, escalation): the
   latest succeeded review or verification run on the same ref that finished
   at or before the event (the verdict is applied in the transaction that
   follows `dispatch.succeeded`), and for an escalation, the checkpoint's
   `context.dispatch_id`.
2. Going forward, add `dispatch_id` to the payload at the call sites above.
   `actions.go` and `actions_phase2.go` are not on M3's list in NFR-7, but
   `returnForChanges` and the approval path are close to what M3 changes, so
   say so in the handoff.

Then extend FR-5.4's acceptance criteria: the *Code review sent back* moment
links to the review run that sent it back, and *Spec approved* links to the
approving review. Say how the chat AI is attributed while you are there
(R12-10).

### R12-2 — The write design doesn't survive the stall sweep failing a live run (material)

**What is wrong.** SD-3 and FR-1.5 put the `error` entry inside
`MarkDispatchFailed`'s transaction (spec:114-120, 215-218). FR-1.6 has the loop
write its own entries as it goes (spec:219-223). The spec never says how the
two writers share `seq`, and the only stall test simulates a *dead* process
(spec:233-235). But the stall sweep can fail a run whose loop is still alive,
and then:

1. **The two writers race for the same `seq`.** The loop numbers entries in
   memory; the error entry has to take "the next" number from the table. A
   unique violation in either place loses entries, or, if it lands in the
   failure transaction, rolls back the failure itself.
2. **A transcript write can stop a failure being recorded.** That contradicts
   NFR-4 ("A transcript never fails a run", spec:466). If the insert fails for
   any reason, `MarkDispatchFailed` returns an error, the dispatch stays
   `running`, and the stall sweep tries again on every tick.
3. **The live loop keeps writing after its attempt was declared dead.** Its
   entries land after the `error` entry that FR-3.5 says ends the attempt
   (spec:303-304), while the retry sweep may already have started attempt N+1.
4. **The old loop can then report success on the new attempt's row.**
   `MarkDispatchSucceeded` guards only `state = 'running'`, so once the retry
   has been claimed, the stale loop's outcome is recorded against it. That bug
   isn't new, but M6 makes it visible: attempt N's transcript ends in an error,
   attempt N+1's is half-written, and the row says "succeeded".

This is not a corner case. The loop refreshes the heartbeat once per turn, and
the default stall threshold is 120 seconds, so one long tool call (a test run)
or a model call that goes through its transient-retry backoff is enough.

**Evidence.** Heartbeat once per turn: `internal/dispatch/dispatch.go:387`.
Four tries with backoff `BackoffBase << i` (2, 4, 8 and 16 seconds, plus four
model calls): `dispatch.go:470-488`. Default stall of 120 seconds:
`internal/config/config.go:232`. The stall sweep fails whatever is `running`
with an old heartbeat: `dispatch.go:541-560`. `MarkDispatchSucceeded`'s guard
is `WHERE id = $1 AND state = 'running'` (`internal/store/dispatches.go:166`).
`RequeueDispatch` resets the same row to `queued` (`dispatches.go:211-216`).

In the committed build (`7627497`), `appendErrorEntry` takes
`MAX(seq) + 1` and inserts with `ON CONFLICT DO NOTHING`, which fixes (2) for
conflicts but quietly drops the error entry that FR-1.6's stall AC requires.
The loop's batch (`AppendTranscript`) runs as one implicit transaction under
pgx, so a single conflicting `seq` drops the whole turn.

**Recommended fix.** Add an FR for concurrent writers:

- allocate `seq` so that two writers can't collide (a database sequence, or
  order by the UUIDv7 `id` and drop `seq` from the unique key);
- write the error entry so that it can never fail the transaction that records
  the failure (a savepoint, or a separate best-effort write after commit);
- have the recorder stop writing, or mark what it writes as "after this run
  was given up on", once the dispatch is no longer running its attempt;
- say that the stale-success bug is out of scope, or guard
  `MarkDispatchSucceeded` on the attempt as well.

Add an AC in which the stall sweep fails a run *while its loop is still
running*, then the loop writes another turn and finishes.

### R12-3 — Two of SD-9's three warnings fire on healthy reviewers by construction (material)

**What is wrong.** SD-9's rules are presented as the rubber-stamp detector and
listed for Sam's approval (spec:162-171, 497). Two of them flag reviewers
whose behaviour is forced by the code, not by laziness:

- **"Finds almost nothing" flags every good verifier.** FR-7.2 counts a
  verification's findings as its unmet criteria (spec:420-421). A verifier
  can't approve with an unmet criterion, so every approval has zero findings.
  A verifier that approves 90% of features and marks one or two criteria unmet
  when it sends one back averages about 0.1 to 0.2 findings per review, which
  is under SD-9's one-in-five threshold. It will be flagged even when it is
  doing exactly what it should.
- **"Approves quickly" measures model latency for document reviews.** A spec
  or plan review is a single model call: the plan offers only the outcome tool
  and no tool context. Its "time to verdict" is one call's latency, so 30
  seconds says which model it is, not how carefully it read. A fast model that
  approves good specs will be flagged.

**Evidence.** Approval with an unmet criterion is refused:
`internal/rules/rules_phase2.go:183-188`. Document reviews get only
`ReviewOutcomeTool` or `CommentsOutcomeTool` and a nil `ToolCtx`
(`internal/server/planner.go:36-59`). A review approve can carry only minor
findings (`rules.go:250-255`), so "findings" on approvals are minors by
definition.

**Recommended fix.** Rework SD-9 before it goes to Sam:

- For verification, don't use findings. Use evidence, such as criteria
  reported per verification, or approvals whose criteria list is shorter than
  the spec's acceptance criteria.
- For single-call reviews, drop the time rule, or state it relative to the
  size of the prompt (seconds per 10,000 input tokens).
- Say whether "finding" in the second rule means majors and minors together.
- Consider a minimum sample larger than three verdicts before any warning.

Update FR-7.3's AC with a verifier that approves 9 of 10 and must *not* be
flagged.

### R12-4 — "M3's sent event is one mapping entry" isn't yet true, and the spec leaves the rest to chance (material)

**What is wrong.** The brief requires the design to take M3's future "sent"
event as one mapping entry. FR-5.3 claims that (spec:366-374), and §6 says
that if M3 "files the event against an initiative for a multi-feature send,
the one table entry changes" (spec:502-504). It wouldn't be enough. FR-5.1
reads only events filed against the feature, its documents, and its tasks
(spec:332-335), so an initiative-filed send is never read and no table entry
can map it. The query would have to change as well. The mapping's "When"
column would also need the event's payload to say which features were sent.

DEC-006 makes the multi-feature send real: the button "also appears on an
initiative, to send several of its features at once" (DEC-006 decision 2).
M3 is also adding other events that belong on a timeline, and the spec
doesn't mention them: a human issue raised on a spec, a spec held for a
person, and "let the reviewer decide" (DESIGN-010 §5, "How specs are
reviewed"). DESIGN-010 §8's own example of attribution is "do specs with human
issues bounce more at planning?".

**Evidence.** DEC-006, decision 2 and "Changed" ("Features gain a 'sent to
development' mark: who sent it, when"). DESIGN-010 §5 and §8. SPEC-012 FR-5.1,
FR-5.3 and §6.

**Recommended fix.** Turn the open question into a coordination requirement
that the handoff hands to M3: *"sent" is audited once per feature, against the
feature, as `feature.sent`, even when the send is made from an initiative.*
With that, the one-entry claim holds. Correct §6 to say so. List the other M3
events the timeline expects (human issue raised, held for a person, let the
reviewer decide) as future one-line entries, so that M3 names them to fit.
Make FR-5.4's AC robust to a *Sent to development* moment appearing at the
start (R12-12).

### R12-5 — The timeline's placement is two edits, not one, departs from DESIGN-008 §5.2 unrecorded, and shares a file with M3 (minor)

**What is wrong.**

1. FR-6.1 puts the line of moments "under its heading" (spec:389-393). FR-6.5
   says `entity.html` "changes by one include, and the feature page loads the
   timeline instead of 'Recent activity'" (spec:402-404). "Recent activity" is
   the last relation section at the bottom of the page. The page head is at the
   top. That is at least two places in `page-entity`, not one include.
2. DESIGN-008 §5.2 draws the page as breadcrumbs, then the body, then the
   rail, with activity among the sections across the bottom. D-2 makes the
   design document the first thing a person reads. A strip of moments above
   the body is a reasonable addition, and DESIGN-010 §8 argues for it, but it
   is a change to §5.2's anatomy and should be named as one.
3. `entity.html` holds `page-entity`, `page-task` and `page-entity-document`.
   M3 changes the page head of `page-entity` (Start work becomes Start building,
   and Send to development is added) and the actions on `page-entity-document`.
   FR-4.1, FR-4.2 and FR-6 edit all three.

**Evidence.** `internal/server/ui/templates/entity.html:14-44` (page head, with
the "Start work" button at 31-37), 175-195 (Recent activity), 744 (task page)
and 777 (document page). DESIGN-008 §5.2 and D-2. DEC-006 decision 9.

**Recommended fix.** Say exactly where the line and the detail sit. Record the
line above the body as an addition to §5.2 for Sam's yes (DoD 6). Put the
task-page and document-page run lists in their own partial file too, so each
of the three pages changes by a one-line include, placed outside the page head
and the document actions. Name in the handoff the lines of `entity.html` that
M6 touches, so M3 can merge around them.

### R12-6 — Which phase a run belongs to is underspecified: same-transaction ties and retries (minor)

**What is wrong.** "Each run belongs to the phase that was current when it was
queued" (spec:375-377) leaves two things open that decide the FR-5.4 AC:

- **Ties.** A moment and the run it triggers are usually written in one
  transaction, and `occurred_at` and `queued_at` are both `now()`, the
  transaction's start time. The code-review sendback and the implementer it
  re-dispatches share one timestamp, as do a document approval and the next
  authoring run.
- **Retries.** `RequeueDispatch` sets `queued_at = now()`, so a retried run's
  `queued_at` is its latest requeue, which may fall in a later phase.

**Evidence.** `internal/store/store.go` `AuditTail` notes that `occurred_at` is
transaction time and uses the UUIDv7 id as the tie-breaker (`store.go:92-104`).
`RequeueDispatch`: `dispatches.go:213-215`. `returnTaskCode` transitions the
task and enqueues the implementer in one transaction
(`actions_phase2.go:414-429`).

**Recommended fix.** Define the order as `(occurred_at, id)` over audit rows,
and place each run by its **first** `dispatch.queued` audit row, not by
`dispatches.queued_at`. Say that a run queued in the same transaction as a
moment belongs to that moment. Add both cases to `TestTimelineMapping`.

### R12-7 — Money can reach the timeline through checkpoint questions (minor)

**What is wrong.** SD-12 promises "No money, anywhere new" and argues it from
two facts: no new read selects `cost_usd`, and no new template can print it
(spec:186-188). But FR-5.2's *Waiting for a person: what about* moment
(spec:359) naturally reads the checkpoint's question, and the budget
checkpoint's question is a dollar sentence. It is filed against the
dispatch's ref, which is a document, a task, or the feature, so it lands in
the feature's timeline. The money test checks template source, not rendered
output, so it wouldn't catch this. The same audit stream also carries
`cost_usd` in every `dispatch.succeeded` payload.

**Evidence.** `"Budget cap $%.2f reached ($%.2f spent this %s period)..."`
(`internal/dispatch/dispatch.go:260-263`), stored as the audit payload's
`question` (`internal/store/checkpoints.go:61-62`). `cost_usd` in
`dispatch.succeeded` (`dispatches.go:180-185`). `TestRenderedTemplatesCarryNoCurrency`
scans template files only (`internal/server/ui_render_test.go:204-229`). The
inbox and the dashboard already print the question verbatim
(`inbox.html:39`, `dashboard.html:34`), which is an existing leak outside M6.

**Recommended fix.** Label *Waiting for a person* from the checkpoint **kind**
in words ("the budget limit was reached", "a reviewer asked a person to
decide"), not from the question. Label *A person decided* from the verb, as
`verbLabel` already does. State in SD-12 that no audit payload field is
rendered raw. Add a rendered-output assertion to `TestUIFeatureTimeline` with a
budget checkpoint in the fixture. Note the existing inbox leak in the handoff.

### R12-8 — The live refreshes will close what the reader has opened (minor)

**What is wrong.** FR-3.4 makes each tool call a closed disclosure
(spec:298-301), and FR-3.7 refreshes "its conversation every five seconds"
(spec:309-310). If the refresh replaces the conversation, every disclosure the
reader has opened snaps shut every five seconds while a run is live. FR-6.4
reloads the timeline on the live signal (spec:400-401), and the live signal
fires on *every* bus event in the project. If the swap covers FR-6.2's "What
happened at each step" disclosure, it closes that too.

**Evidence.** The dashboard's live regions use `hx-swap="innerHTML"`
(`dashboard.html:17, 154-164`), and none of them holds a disclosure. The SSE
handler sends one generic `changed` event for every bus event
(`internal/server/ui.go:917-924`).

**Recommended fix.** For the transcript, append only the new entries (poll
with an `after=<seq>` cursor and `hx-swap="beforeend"`). For the timeline,
refresh only the line and the moment list, and keep the open state of each
`<details>`, for example by swapping per moment or with `hx-preserve`. Add an
AC that the refresh doesn't close an open disclosure (a Playwright step in the
demo is enough).

### R12-9 — The size limits can remove the outcome and cut the prompt, against the Goal's "exactly" (minor)

**What is wrong.**

1. FR-2.3 replaces every entry after the attempt budget is used with a marker
   (spec:265-268). That includes the `outcome` entry, which is the one entry
   FR-3.2 puts first.
2. `max_entry_bytes` applies to the system prompt and the user prompt
   (spec:254). A review prompt carries the whole document, and often the
   design, so a big spec with its design can pass 256 KiB and be cut. The Goal
   says "exactly what each agent was told" (spec:60-62), and §6 relies on the
   kept prompts to check decisions in prompts for M11 (spec:508-509).
3. FR-2.1 says what 0 means for `retention_days` only. It doesn't say what 0
   means for the three byte limits: no limit, or the default.

**Evidence.** spec:252-256 and 265-271. In the build, 0 means "use the
default" for the byte limits (`internal/config/config.go` in `7627497`, the
defaulting after `validate`), and the budget test is `used + len > max`
rather than "reached".

**Recommended fix.** Exempt `outcome` and `error` from the budget, and give
the prompts their own, larger limit (or none). Say in the Goal or FR-3.3 that a
cut prompt is marked as cut. State what 0 means for each byte field. Consider
storing the system prompt once per role and content hash, since it repeats
unchanged on every run.

### R12-10 — SD-8 narrows DESIGN-010 §8 without saying so, and attribution leaves out the chat AI (minor)

**What is wrong.**

1. DESIGN-010 §8 says approval rates, findings per review, and time to verdict
   "are shown per reviewer", and that "every artifact and verdict records who
   produced it: which agent role and model, the chat AI, or a human". SD-8
   limits review health to agents (spec:151-160). The reasons are sound, and
   the choice is listed for Sam (spec:495), but it is a narrowing of an
   approved, binding design, and it should say so in those words. The argument
   against humans also only rules out *time to verdict*: approval rate and
   findings per review are well defined for a person's verdicts.
2. FR-5.2's attribution names "an agent run, the orchestrator, or the person
   named on the audit row" (spec:363-365). DESIGN-010 §8 also names the chat
   AI, and MCP acts are audited to the configured `mcp_actor`. A moment the
   chat agent caused would be shown as a person.
3. SD-8 says the comments-only review is "the design reviewer's". The code
   gives the comments-only tool to *any* document type whose approval belongs
   to a person, not only designs.

**Evidence.** DESIGN-010 §8, "Review health is visible" and "Everything is
attributed". `MCPActor` defaults to `chat-agent` (`internal/config/config.go:179-181`).
`humanApprovalType` picks the comments-only plan by type
(`internal/server/planner.go:44-53`).

**Recommended fix.** Rewrite SD-8 as a recorded scoping of §8 and ask Sam to
accept it as such. Add the chat AI as a fourth attribution, recognised by the
configured MCP actor or by `via: mcp` once M3 writes it. Say "a review of a
human-approved document type" rather than "the design reviewer".

### R12-11 — Small factual errors about the code and the spec's own references (minor)

**What is wrong.**

1. SD-3 says that without the error entry, "the reason an earlier attempt
   failed would be lost the moment it was retried" (spec:118-120). It
   wouldn't: `MarkDispatchFailed` writes the reason into the
   `dispatch.failed` audit row's payload, and `RequeueDispatch` doesn't touch
   the audit trail. The error entry is still worth having, because it puts the
   reason where the viewer reads it, but the justification should say that.
2. Scope item 5 cites FR-1.6 for real latency (spec:85). It is FR-1.8.
3. FR-5.2's "When" column mixes target states ("to ready", "to done", "to
   abandoned") with loose event names ("tasks complete", "contract
   invalidated") (spec:350-356). The payload's `event` values are
   `contract_approved`, `start`, `tasks_complete`, `verified`, `rework`,
   `abandon` and `contract_invalidated`. A mapping table that is meant to be
   the single point of change should use them.
4. The `tool_calls` ledger that FR-3.6 shows for pre-M6 runs (spec:305-306) has
   no `attempt` column, and `seq` restarts at 1 on every attempt, so a retried
   run's ledger rows repeat sequence numbers and can't be split by attempt.

**Evidence.** (1) `dispatches.go:205-206` and 209-232. (2) spec:241-244. (3)
`internal/lifecycle/feature.go:26-47`; `internal/store/entities.go:196-202`.
(4) `internal/store/migrations/0001_init.sql:137-147`; `seq := 0` per
`runLoop` call (`dispatch.go:385`).

**Recommended fix.** Correct each. For (4), say in FR-3.6 that the ledger of a
pre-M6 run is shown as one list, not per attempt.

### R12-12 — Missing or untestable acceptance criteria (minor)

**What is wrong.** Several requirements have no AC, and one AC will break when
M3 lands:

- FR-3.7 (the running page follows itself) and FR-6.4 (the timeline follows
  the live signal) have none.
- FR-6.1's collapsing of repeated labels, its marking of the current phase,
  and what a moment's "state" is ("shows its state by icon and word",
  spec:392-393) are not defined or tested. A moment has no state of its own;
  presumably past or current is meant.
- FR-5.2's "who caused it" (spec:363-365) is not asserted anywhere.
- FR-8.1's "sorts to the top" and the window switch are not in
  `TestUIReviewHealth`.
- FR-2.4 doesn't say how often pruning runs. The heartbeat ticks every 30
  seconds by default; the build paces it hourly, which the spec should say.
- `TestTimelineOfAFullLoop` (spec:379-383) lists the moments exactly, starting
  with *Created*. Once M3 emits `feature.sent`, a *Sent to development* moment
  appears and the test fails for a reason that isn't a bug.
- NFR-2 and NFR-3 set no threshold, only a report. That is acceptable if it is
  intended; say that they are reported, not gated.

**Evidence.** spec:309-315, 389-407, 436-449, 272-277, 379-383 and 456-465.
Heartbeat default of 30 seconds: `internal/config/config.go:174`.

**Recommended fix.** Add the missing ACs, define a moment's state, and write
the full-loop AC as "these moments, in this order, with any M3 moment allowed
before *Spec written*".

### R12-13 — The migration-runner change protects only databases that run the new runner (minor)

**What is wrong.** SD-10 is right about the old runner (spec:173-180): it
refuses gaps and applies only versions above the highest. The fix works on
any branch that has it. But M3's branch still has the old runner. A shared
database that has had M6's `0008` applied, and is then used from M3's branch,
has `MAX(version) = 8`, so the old runner skips `0007` with no error, and M3's
tests or smoke run fail for no visible reason. `SchemaVersion` reports the
maximum, so a database with a gap also reports "8" in the status output.
"0007 and 0008 touch different tables" can't be checked until 0007 exists.

**Evidence.** Old runner at `a882adf`: `internal/store/migrate.go:51` (the
contiguity check) and 84 (`m.version <= current`). `SchemaVersion` is
`MAX(version)` (`migrate.go:108-115`), used by status (`internal/server/http.go:81`).

**Recommended fix.** In the handoff, tell M3 to take the runner change
before it runs against any database that M6 has touched, or to use a fresh
database. Have `SchemaVersion`, or the status output, report a missing version
below the maximum. Turn "they touch different tables" into a check made when
the second of the two merges.

### R12-14 — Prose: missing serial commas, headings that aren't sentences, and raw engine text on human pages (minor)

**What is wrong.**

1. **The serial comma is missing** throughout, against the writing guide
   ("Keep the serial comma"). For example: "D-6 (human prose) and D-10"
   (spec:13-14); "Every heading, label, empty state and notice" (spec:25);
   "told, did and concluded" (spec:62); "Executors, claims and chat-done work"
   (spec:94); "a nudge, the outcome or an error" (spec:102); "outcome, tokens
   and tool ledger" (spec:131); "`outcome` and `error`" (spec:200); "the stop
   reason and that turn's token counts" (spec:207); "broken UTF-8 and a
   multi-byte rune" (spec:239); "outcome, tokens or `tool_calls`" (spec:275);
   "approved, sent back and escalated" (spec:419); "rounds and medians"
   (spec:430); "`internal/starter/` or the document page's actions"
   (spec:474).
2. **"A note on prose" promises more than the spec does.** It says every
   heading and label "is a full, plain sentence" (spec:25-27). The spec's own
   headings are phrases: "Agent runs on this task", "Agent reviews of this
   document", "What happened at each step", and "How the reviewers are
   doing". Phrases are what the existing pages use ("Tasks in this feature",
   "Recent activity"), and D-6's "full, clear sentences" has been read that
   way so far. Say "plain words, with terms explained" rather than promise
   sentences.
3. **Raw engine text would reach people.** FR-3.2 shows a failed run's reason
   (spec:293), and FR-3.5 ends each attempt with its error. Those strings are
   Go error text ("stalled: no heartbeat", "provider unavailable after 4
   tries: ..."). FR-5.2's *Waiting for a person: what about* would show the
   engine's question text (R12-7).
4. **"Sendback"** (spec:362, 384 and 484) is internal jargon; "sent back" is
   what the labels say.
5. **D-10 is cited as a parent but never applied** (spec:13-14). Review health
   is one row per reviewer with seven figures (spec:436-441), which is the
   dense table D-10 and §5.1b warn against. A table may be honest here, since
   the data really is tabular, but the spec should say that, or lay each
   reviewer out as a sentence or two.

**Evidence.** Writing guide, "Spelling and style" (line 10) and "The serial
comma" (line 528). DESIGN-008 D-6 and D-10. `entity.html:105` and 180.

**Recommended fix.** Add the serial commas. Reword the prose note. Specify
that a failure is shown as a plain sentence chosen by kind (stalled, provider
unavailable, turn limit reached, refused at admission), with the raw text
beneath it in a disclosure. Replace "sendback". Say how D-10 applies to the
review-health page.

### R12-15 — The phase detail repeats every run as audit events as well (minor)

**What is wrong.** SD-7 drops "Recent activity" partly because "keeping both
would show the same events twice on one page, which design round 4 set out to
stop" (spec:147-148). But FR-6.2 lists, inside each moment, "the phase's runs
and events" (spec:394-396), and SD-7 says that means "every audit event in its
phase ... and every run" (spec:144-145). Each run already has three or four
audit rows (`dispatch.queued`, `dispatch.running`, and `dispatch.succeeded` or
`dispatch.failed`, plus `dispatch.requeued`), so every run is shown once as a
run and three or four more times as events. That is the duplication SD-7
cites round 4 against, in the same section.

**Evidence.** `dispatches.go:72, 128, 180, 205 and 230`; the activity labels
for them at `internal/server/ui.go:353-364`.

**Recommended fix.** Leave `dispatch.*` events out of a phase's event list,
since the run rows carry the same facts. The detail is then still a superset of
the old list in everything but those rows.

### R12-16 — The same-label collapse can't trigger for the example it gives (note)

FR-6.1 collapses "moments with the same label in a row", with the example
*"Code review sent back ×3"* (spec:390-392). But FR-5.2's label for that
moment carries the task and the round number (*Code review sent back task
(round n)*, spec:358), so no two in a row share a label. Say whether the
collapse compares the moment's kind, and what the collapsed label shows.

### R12-17 — The status line describes a §7 that doesn't exist yet (note)

The status says §7 records how each finding was dealt with (spec:4-6), and §7
says "Filled in after REVIEW-012" (spec:514). That will be true once the
author responds. Until then, the status line is ahead of the document. The
same note was made on SPEC-010 (R10-16).

## 3. Checks that passed

- **The claims about how things work today hold**, apart from those in R12-11.
  - `runLoop` keeps the conversation in a local `msgs` slice and returns
    without storing it (`dispatch.go:384-445`); only the outcome, the tokens
    and the ledger survive (`dispatches.go:156-186`, 350-356).
  - The ledger's latency is always 0 (`dispatch.go:463`).
  - The feature page's "Recent activity" is the last ten rows filed against
    the feature itself (`internal/server/ui_entity.go:589`).
  - `max_review_rounds` bounds only the code-review loop (`config.go:122-126`;
    `rules_phase2.go:304`).
  - Retries reuse the row with `attempt + 1`, and `RequeueDispatch` clears
    `error` (`dispatches.go:209-232`), so keying by attempt (SD-2) is needed.
  - The migration runner at `a882adf` refuses gaps and applies only above the
    maximum (`migrate.go:51`, 84), as SD-10 says.
  - The indexes NFR-5 relies on exist: `audit_events (ref_type, ref_id,
    occurred_at)` and `dispatches (ref_type, ref_id)` (`0001_init.sql:130`,
    170).
  - The UI actor is one configured name until M15 (`config.go:176-178`).
  - A comment with no severity counts as major (`rules.go:195`), as FR-7.2
    says.
  - Document review purposes are `review-<type>` (`actions.go:230`), code
    review is `review-code`, and verification is `verify-feature`, so FR-7.1's
    filter picks up exactly the reviewer runs.
  - `devplan.decomposed` is written before G1 fires, so *Broken into n tasks*
    precedes *Ready to build* in FR-5.4's AC (`rules.go:426-440`;
    `actions.go:509-520`).
- **Postgres text is made safe** (FR-1.7): NUL bytes and invalid UTF-8 are
  replaced before writing, and the AC tests a rune on the cut boundary.
- **The brief is covered.** Complete conversation, written as the run
  proceeds, partial transcripts on failure, configurable size and retention
  with defaults, truncation with a marker, the viewer from task pages,
  document pages and the timeline, the timeline as a read model with rounds
  and escalations as moments, its own partial, a reasoned decision on the
  Activity list (SD-7), the "sent" entry (subject to R12-4), and review health
  per role and model with rubber stamps singled out (subject to R12-3).
- **The coordination lines are kept.** Migration `0008`, never `0007`. NFR-7
  names exactly the brief's off-limits paths. Review health can read the
  outcomes through the exported `rules.Parse*` functions and `IsMajor`, with no
  change to `internal/rules/`. No MCP tools, executors or rename work.
- **D-4 is respected in intent.** Tokens per turn and per run, `cost_usd`
  never selected, and the existing currency test kept (subject to R12-7).
- **SD-7's decision is right.** Folding "Recent activity" into the timeline on
  feature pages, and keeping it on project and initiative pages, follows round
  4's rule against saying the same thing twice. The detail covers the
  feature's documents and tasks, which the old list couldn't.
- **The storage shape is sound.** One row per entry, appended and never
  rewritten (SD-1), is the only design that leaves a partial transcript after
  a crash. The `(dispatch_id, attempt, seq)` key matches how retries work
  (subject to R12-2 on how `seq` is allocated).
- **The scoping of review health to agents is argued honestly**, including the
  retirement of the comments-only design review in M3 (DESIGN-010 §17a item
  1). R12-10 is about how it is framed, not whether it is right.

## 4. Notes carried into implementation (not blocking)

- **Transcripts keep secrets that were ephemeral before.** A tool that reads a
  `.env` file or prints an environment variable now stores it in Postgres and
  shows it on a page. A single local operator can accept that, but the spec
  should say it was considered, and the handoff should mention it.
- **The document page can link reviews without a new list.** Review comments
  already carry `dispatch_id` (`0001_init.sql:152`). Each review's comments
  could link to its run, which would answer "why did the reviewer say this?"
  where the reader already is. FR-4.2's section is still useful for approvals
  with no comments. The new section also needs to sit inside `#doc-live`
  (`entity.html:793`) to stay live, and that region holds the actions M3 is
  changing.
- **The run that wrote a document isn't reachable from the document.** Authoring
  runs are filed against the feature (`internal/server/authoring.go:210`), so
  only the timeline leads to them. A "written by" link on the document page
  would help, without touching the actions.
- **Deleted tasks drop out of the timeline.** Re-decomposition deletes pending
  tasks outright (`internal/store/tasks.go:131-137`), so their audit rows can
  no longer be joined to the feature. They never had runs, so little is lost,
  but the handoff should say so.
- **Batched writes are all or nothing under pgx.** `AppendTranscript` sends a
  batch with no explicit transaction, and pgx runs that as one implicit
  transaction, so one bad row loses the whole turn (see R12-2).
- **`content_bytes` is measured after cleaning** in the build, not before, and
  a cut entry is its limit plus the marker. Both are fine, but FR-2.2's "the
  size before any cut" should say which.
- **The pruned-then-retried edge.** A run that is pruned and later retried from
  a checkpoint will have `transcript_pruned_at` set and new entries. The viewer
  should show the new attempt, not the "pruned" notice.
- **Heartbeat during long tool calls.** The root cause of R12-2 is that the
  loop refreshes the heartbeat once per turn. Refreshing it around long tool
  calls and provider retries would make false stalls rare. That is an engine
  change, but a small one.
- **The build started before approval.** As with SPEC-010, the transcript half
  was committed before the spec was approved. Sam has said he will approve the
  spec and the build together, which is his call. It means the fixes for R12-2
  and R12-9 touch committed code.

## 5. Recommendation and decision

The spec is well aimed and well argued. It covers the brief, keeps off M3's
ground, chooses a storage shape that meets the partial-transcript requirement,
and makes the Activity-list decision with reasons. It isn't ready to approve
as written. Four findings are material:

- R12-1: the moments a person most wants to open lead to the wrong runs, and
  the audit rows can't yet say which run caused a moment.
- R12-2: the write design has no answer for the stall sweep failing a live
  run, and its in-transaction error entry can stop a failure being recorded.
- R12-3: two of the three warnings Sam is asked to approve fire on healthy
  verifiers and single-call reviewers by construction.
- R12-4: the "one entry for M3's sent event" claim depends on how M3 files the
  event, and that should be agreed with M3, not left open.

All four can be fixed in the text of the spec, plus a few lines of code. None
needs a new design.

Decisions for Sam once the findings are dealt with, in addition to the five
the spec lists (DoD 6):

1. **The line of moments above the page body** (R12-5), as a recorded addition
   to DESIGN-008 §5.2.
2. **Review health counts agent verdicts only, for now** (R12-10), as a
   scoping of DESIGN-010 §8, not just a spec choice.
3. **The reworked warning rules** (R12-3), in place of SD-9 as drafted.
4. **The requirement handed to M3**: "sent" is audited once per feature,
   against the feature (R12-4).

**Recommended for approval once R12-1 to R12-4 are fixed.** The reviewer does
not approve.

_Decision (Sam, 2026-09-28): **approved**, with the build, and with all nine choices in SPEC-012 DoD 6 as recommended, including the four this review raised._

## 6. Author's response (2026-09-28)

Every finding is accepted, and dealt with in the spec, the code, or both.
[SPEC-012 §7](../specs/SPEC-012-see-the-work.md#7-changes-after-review) maps
each finding to its change. In short:

- **R12-1.** Each moment an agent caused now carries its **cause**, the run
  whose verdict produced it, found from the run's purpose, its ref, and its
  finish time (FR-5.5). The phase's runs are kept as well, under "What ran
  after it". Writing `dispatch_id` into the verdict audit rows is proposed to
  M3 rather than done, because the call sites sit beside M3's changes.
- **R12-2.** Taken further than the recommendation: `seq` is no longer
  unique, the failure entry is written in a savepoint, the loop checks its own
  attempt before and after each model call and stops when it was given up on,
  and success is recorded only for the current attempt
  (`MarkAttemptSucceeded`). The new test holds a model call open across a
  stall and checks the late approval isn't applied.
- **R12-3.** Speed now counts only with a terse answer (under 300 output
  tokens); "finds almost nothing" reads review verdicts only; the minimum
  sample is five. The recommended "seconds per 10,000 input tokens" was
  considered and not taken: output length says more directly whether the
  reviewer reasoned, and is easier to state on the page.
- **R12-4.** Adopted as recommended, as a requirement handed to M3.
- **R12-5.** One correction to the finding: "Recent activity" needed no
  template edit. The feature page stops loading it, and the section's
  existing `{{if .Activity}}` hides it. So `entity.html` changes by three
  one-line includes (timeline, task runs, document runs), all named in the
  handoff. The placement above the body is recorded as SD-14 for Sam.
- **R12-6 to R12-17.** Adopted as recommended. For R12-8, the transcript polls
  a status line rather than appending entries, which is simpler and closes
  nothing; for R12-13, the handoff carries the instruction to M3.
- **§4 notes.** Secrets are SD-13, with a choice for Sam. The comment-to-run
  link and a "written by" link on documents are follow-ups.

— the author
