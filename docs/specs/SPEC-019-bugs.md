# SPEC-019: Bugs

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. Sam has said they will
approve the spec and the build together. An independent review is recorded in
[REVIEW-019](../reviews/REVIEW-019-bugs.md); §7 says how each finding was dealt
with. Fourteen choices need Sam's explicit yes (§5, DoD 8).
**Date:** 2026-09-28
**Roadmap milestone:** M12 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11. Roadmap decision 12 (§12) is part of the brief: a dedicated triage queue,
with a count on the inbox.
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28): **§9 is the main source**. Also §4 (a bug is feature-shaped, and
a milestone deliverable), §5 (minor review findings "are recorded as bug
reports in the triage queue"), §5c (what the chat agent may do), §6
(deliverables), §7 (the `BUG-` prefix and the default folder), and **§17a
item 6, accepted**: acceptance stands in for G0, the report is the spec, the
spec reviewer reviews it, and the chat agent may relay a triage decision.
**Authority:** [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) and
its Amendment 1 (reporting is planning authoring);
[DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1
(relays quote the person; Send stays in the web UI);
[DEC-007](../decisions/DEC-007-the-judgement-boundary.md) (who does the work,
and who judges it).
**Builds on:** [SPEC-011](SPEC-011-send-to-development.md) (the send path, the
invariants, the review loop), [SPEC-014](SPEC-014-checklists-and-jobs.md) and
[SPEC-010](SPEC-010-milestones-and-roadmaps-editing.md) (how a deliverable
joins a milestone), [SPEC-015](SPEC-015-documents-with-identity.md) (`BUG` is
reserved, with its sequence made), and
[SPEC-017](SPEC-017-chat-as-a-proper-seat.md) (who wrote, who judged).
**Out of scope here:** bugs filed as GitHub issues ([Subutai and
GitHub](../research/subutai-and-github.md) §6), which belong to M15b.
**Coordination:** M11 (decisions) runs in parallel and merges first. It owns
migration `0012` and prompt assembly (`planner.go`'s prompt parts and
`review_prompt.go`). This spec owns migration `0013`, the review-outcome
handling (`internal/rules`, `internal/server/actions*.go`) and the inbox.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6).

Three words are kept apart throughout:

- a **bug** is the entity, `BUG-007`;
- its **report** is its document, `BUG-007-bug-report`, which serves as its
  spec;
- **triage** is the decision a person makes about a reported bug: accept it,
  reject it, or mark it as a duplicate of another.

## 0. Framing

Today Subutai has no way to say "this is broken". A person who finds a defect
writes it down somewhere else, and an agent that trips over one either fixes
it in passing, outside its task, or forgets it. Minor code-review findings are
kept on the audit trail and nobody reads them.

DESIGN-010 §9 says what a bug is: a reported problem, feature-shaped, with one
extra step at the front. Anyone may report one. A person decides whether it is
fixed, because accepting a bug commits scope, the same kind of act as creating
a feature. From acceptance it travels the normal pipeline, with two
differences: acceptance stands in for G0, and the report stands in for the
spec.

So this spec adds very little machinery. A bug *is* a feature row, marked as a
bug, with a small table beside it for the facts only a bug has: who reported
it, where it came from, and how it was triaged. Everything after acceptance —
the send, the review loop, the plan, the estimate, Start building, the tasks,
code review, verification, the merge, the timeline, review health and
milestones — is the code that already runs features, reading one extra fact:
for a bug, the contract's first half is its report.

## 1. Goal

**One claim, which the definition of done checks directly:**

> An agent files a bug mid-task. A person accepts it in the triage queue and
> sends it to development. The spec reviewer reviews its report as a spec, the
> plan is written and reviewed, and it is estimated. A person presses Start
> building; the fix is implemented, code-reviewed, and verified as "the defect
> no longer reproduces", and it merges.

Three supporting claims:

> Anyone can report a bug — a person in the web UI, the chat agent, an agent
> at work, or a code reviewer's minor findings — and none of them can decide
> whether it is fixed.

> The chat agent can carry a person's triage decision, quoting them, and
> nothing else about triage.

> A bug uses the same send, review, plan, build and verify code as a feature.
> There is no second pipeline.

## 2. Scope

### In scope

1. The bug entity, its report and its template (FR-1).
2. Triage: the lifecycle, the queue, and the count on the inbox (FR-2).
3. Reporting from the web UI, from chat, by agents at work, and from minor
   code-review findings (FR-3).
4. From acceptance, the normal pipeline (FR-4).
5. Where bugs show: pages, lists, the timeline, review health and milestones
   (FR-5).
6. MCP tools (FR-6).
7. The starter pack (FR-7).

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| Bugs filed as GitHub issues, and their label map | M15b |
| Decisions, and pushing them into prompts | M11 (in parallel) |
| Executors: claiming a bug's tasks from chat | M13 |
| Spikes | M14 |
| Reopening a rejected or duplicate bug | §6. A new report does the job today. |
| Bugs owned by the project itself, above every initiative | §6. A bug hangs off an initiative or a feature, as §9 says. |
| A setting to stop filing minor findings as bugs | §6, once the live runs show the volume |
| Per-user identity for reporters and triagers | Before the Tickly pilot (roadmap decision 14). Acts are audited to the configured UI or MCP actor, as today. |

### Scope decisions

- **SD-1 — A bug is a feature row with a kind, not a separate table.** The
  test the brief sets is "no parallel machinery". Every stage after acceptance
  keys off a `features` row: the sent mark (`feature_sends`), the spec and plan
  (documents owned by a feature), tasks, the worktree and branch, dispatches
  with `ref_type = 'feature'`, estimates, gates G1 to G3, the timeline,
  milestone leaves and snapshots. A separate `bugs` table sharing the
  lifecycle would need a twin of each, or a polymorphic reference in each; a
  kind column needs neither. So `features` gains `kind` (`feature` or `bug`),
  and a bug's facts live in a one-to-one side table, `bugs`, in the same way
  the sent mark lives in `feature_sends` (SPEC-011 SD-1). The cost is paid
  once, at the reads that mean *features only* — an initiative's child list,
  the send screen's checkboxes, the rail, the design cascade — which filter on
  kind (FR-1.6). **Flagged for Sam.**

- **SD-2 — Triage is a column on the side table, not a feature state.**
  DESIGN-003's feature machine is unchanged. A reported bug is an `idea` with
  `triage = 'reported'`. Accepting it leaves it an `idea`, now eligible to be
  sent. Rejecting it, or marking it as a duplicate, abandons the feature row in
  the same transaction, with the reason, so every existing count of open work
  (G5, milestones, lists) reads it as finished without learning about triage.
  Triage is final in this milestone: nothing un-rejects a bug (§6).

- **SD-3 — A bug hangs off an initiative, and optionally a feature.** A
  feature row needs an initiative, so a bug always has one: the initiative it
  was reported on, or the initiative of the feature it was reported on. The
  feature, when there is one, is recorded as its *origin*. A bug filed by an
  agent or from review findings has the origin of the dispatch's feature —
  which may itself be a bug, when the defect was found while fixing another.

- **SD-4 — The report's ID is `BUG-007-bug-report`.** A document's ID is its
  owner's ID plus its type's slug (SPEC-015 FR-3.1), and the type is
  `bug_report`, as the brief names the template. DESIGN-010 §7's folder
  example shows `BUG-031-report.md`; making it so would need `report` to mean
  two types, and the starter pack already has a `report` type. The example
  is read as illustrative. **Flagged for Sam.**

- **SD-5 — The built-in criterion is enforced, not just offered.** Every
  report's Acceptance criteria must include "The defect no longer reproduces",
  checked by a new manifest rule, `contains_text`, at submission, like every
  other structural rule. The template and every generated report start with
  it. A person may add more criteria; they can't remove that one.

- **SD-6 — A report is reviewed once the bug is accepted and sent.**
  Reviewing spends agent time, and accepting is the commitment of scope
  (DESIGN-010 §9), so a report can't be submitted while its bug is
  `reported`. Once the bug is accepted, a person or the chat agent may submit
  it early, as with a spec (§5a). **Send submits it** when it is still a draft
  with nothing against it: for a bug, sending *is* "review this and go on",
  and there is no author agent whose turn it would otherwise be. A report that
  fails validation stops the send with the reasons.

- **SD-7 — A report sent back goes to the spec author to revise.** When the
  spec reviewer or a person sends back a sent bug's report, the draft waits
  for its author, exactly as a spec does (SPEC-011 SD-4), and the author is
  the spec author, dispatched as `write-spec` on the bug. It is told it is
  revising a bug report, and never to invent reproduction steps it can't
  support. If the loop doesn't converge, the round cap raises the
  `authoring-deadlock` question as it does for a spec, and a person edits the
  report. This keeps one loop, not two. **Flagged for Sam:** the alternative is
  to leave a sent-back report for a person always.

- **SD-8 — The spec reviewer reviews the report with its own skill.** The
  `bug_report` manifest names `spec-reviewer`, so the review dispatch is
  `review-bug_report`, run by the same role and model. The difference in what
  it checks (reproduction and acceptance clear and testable, and no design to
  be faithful to) is written into the `review-spec` skill, which gains a
  section for bug reports, rather than into prompt assembly, which M11 owns.
  Holds, human issues, direct approval, *let the reviewer decide* and agent
  review switched off all apply to a report exactly as to a spec, because the
  report *is* the spec (DEC-006 Amendment 1).

- **SD-9 — Minor code-review findings become one bug per approving review.**
  When a code reviewer approves a task with minor findings, those findings are
  filed as **one** bug report listing them all, on the task's feature, with
  the reviewer as reporter. One per finding would flood the queue: a typical
  approving review carries two to five minors. And a code review's minors
  arrive only once per task, on the approval, because minors on a
  send-back ride back to the implementer. De-duplication across reviews isn't
  attempted: two reviews of different tasks rarely raise the same text, and
  a person triaging marks a duplicate in one act. Minor findings from spec and
  plan reviews stay on the document's thread, where they already are: they
  are about a document, not a defect in the software. **Flagged for Sam, as
  the brief asks.**

- **SD-10 — Agents file with a tool, capped per run.** Implementers, code
  reviewers and verifiers get a `report_bug` tool through their role profiles.
  It files the report and returns, and the agent carries on with its own task:
  the tool's description, and each role's skill, say plainly that the agent
  must not fix what it reports. A run may file at most three reports; the
  fourth call is refused with a sentence, as a guard against an agent that
  starts reporting instead of working. A report whose title matches an open,
  untriaged bug on the same origin is refused as a duplicate, naming it. Spec
  and plan reviewers and the authors aren't given the tool: they read and
  write documents, not code, and what they find belongs on the document's
  thread.

- **SD-11 — The chat agent reports without a quote and relays triage with
  one.** Reporting is planning authoring (DEC-004): it creates an idea and
  commits nothing, like `create_feature`. Triage is a person's decision
  (DESIGN-010 §9), so the chat agent may only carry it, with the person's
  words, through one relay tool, `relay_triage`. DEC-006 Amendment 1 says a
  relay not on its list needs a decision; DESIGN-010 §17a item 6, accepted by
  Sam, is that decision, and a dated note in DEC-006 records it on approval
  (DoD 9).

- **SD-12 — The queue is its own page, and the count shows twice.** Roadmap
  decision 12 asks for "a dedicated queue, with a count on the inbox". The
  queue is `/ui/triage`, in the main navigation, with its own count badge.
  The Inbox page shows a line above its questions — "3 bug reports are waiting
  for triage" — linking to the queue. The Inbox's own badge still counts
  checkpoints only, because a checkpoint means work is stopped and a report
  doesn't. **Flagged for Sam.**

- **SD-13 — A bug joins a milestone as itself, once accepted.** DESIGN-010
  §17a item 3 makes bugs deliverables. A bug is added to a milestone like a
  feature, from either end, and counts as one item, with its tokens in the
  bar. It must be accepted first: a reported bug isn't committed scope, and a
  rejected one never will be. **An initiative in a milestone brings in its
  features, not its bugs**, as it doesn't bring in its checklists (SPEC-014
  SD-4): otherwise every accepted bug under an initiative would silently
  enlarge a milestone, and a shipped milestone's record would depend on
  triage done later. **Flagged for Sam.**

- **SD-14 — Bugs count as open work under their initiative.** G5 refuses to
  archive an initiative while any feature row under it is not terminal. A
  reported or accepted bug is open work, so it blocks archiving too, and G5's
  reason says "features and bugs". Rejected and duplicate bugs are abandoned
  (SD-2), so they don't.

- **SD-15 — The initiative's send screen lists features only.** An accepted
  bug is sent from its own page. The initiative's "Send features to
  development…" screen keeps to features, as its name says; a person sending
  bugs is sending one fix at a time, which is how they are triaged.

- **SD-16 — Minimal changes in M11's files.** Two are unavoidable, and each
  is one small call into this spec's code, named in the handoff:
  `contractBody` in `planner.go` reads a bug's report where it reads a
  feature's spec, and `planAuthor` asks which document type `write-spec`
  writes for the feature. Nothing else in prompt assembly changes;
  everything bug-specific the agents are told is in the skills.

## 3. Requirements

### FR-1: The bug entity

**FR-1.1 — Migration `0013`** is additive:

- `document_type` gains `bug_report`.
- `features` gains `kind text NOT NULL DEFAULT 'feature'`, checked to be
  `feature` or `bug`, and a check that a bug's ID starts `BUG-` and a
  feature's doesn't. The column default of `public_id` stays
  `mint_ident('FEAT')`; a bug's insert names `mint_ident('BUG')` itself
  (FR-1.3).
- A new table, `bugs`, one row per bug row:
  - `feature_id` (primary key, references `features`);
  - `origin_feature_id` (references `features`, nullable): the feature it was
    reported on (SD-3);
  - `triage` (`reported`, `accepted`, `rejected` or `duplicate`; default
    `reported`);
  - `reporter_kind` (`person`, `chat`, `agent` or `review`), `reported_by`
    (the actor), `reported_dispatch_id` (the run that filed it, for an agent
    or a review), `source_task_id` (the task under review, for review
    findings), `reported_at`;
  - `duplicate_of` (references `features`, nullable);
  - `decided_by`, `decided_via` (`ui` or `mcp`), `decided_quote`,
    `decided_reason`, `decided_at`.
- Checks: a duplicate has `duplicate_of` and nothing else does; a decided bug
  has `decided_by`, `decided_via` and `decided_at`, and an undecided one has
  none; `decided_via = 'mcp'` needs a quote; a rejection needs a reason; an
  agent or review report names its run.
- An index on `bugs (triage, reported_at)` for the queue.

**FR-1.2 — The registry.** `internal/ident`'s `bug` entry names the
`features` table. A task of a bug is numbered from the bug's ID,
`BUG-007-T01`, and `ident.Parse` reads it as a task.

**FR-1.3 — Creating a bug** (`store.CreateBug`) inserts the feature row with
`kind = 'bug'`, a `BUG-` ID, a slug made from the ID (`bug-007`, so it never
clashes with a feature's and never needs inventing), the title as its name,
and the report's summary as its description; then the `bugs` row; then a
`bug.reported` audit row naming the reporter, the origin and the channel. One
transaction. It publishes `FeatureCreated` after commit, so the invariants
re-check as for any new feature (they find nothing to do: FR-4.2).

**FR-1.4 — The report.** Creating a bug writes its report from the
`bug_report` template, filled from the reporter's fields, at the default
home: `docs/work/<INIT-ID>-<slug>/BUG-007-bug-report.md`. It is registered as
a `draft`, owned by the bug, marked as the bug's main document, given its ID
and revision 1, recorded as written by the reporter (SPEC-017 FR-2), and
committed on its own, as a starter design is (SPEC-015 FR-6). If the file
can't be written, the bug isn't created, and the reporter is told why.

**FR-1.5 — The template** `templates/bug_report` has front matter `title`,
`type` and `owner`, and these sections in order:

| Section | Required | What it holds |
|---|---|---|
| Summary | yes | One or two sentences: what is wrong. |
| Steps to reproduce | yes | A numbered list someone else can follow. At least one item. |
| Expected | yes | What should happen. |
| Actual | yes | What happens instead. |
| Acceptance criteria | yes | At least one item, and it must include "The defect no longer reproduces" (SD-5). |
| Where it was found | no | The feature, task or run it came from. |
| Notes | no | Anything else: logs, a guess at the cause. |

The manifest names `spec-reviewer` and agent approval, and adds the rules
`min_list_items` on Steps to reproduce and Acceptance criteria, and
`contains_text` on Acceptance criteria.

**FR-1.6 — Features only, where the word means it.** These reads filter on
`kind = 'feature'`:
- an initiative's child features, on its page, in the rail, in
  `get_initiative` and `get_tree`;
- the initiative's send screen (SD-15);
- the design cascade's and the reconciler's initiative scope, because a bug
  has no spec for a design to revise;
- an initiative's features resolved into a milestone (SD-13).

Everything else reads every row: G5 (SD-14), the heartbeat sweep, dispatches,
the corpus and cost roll-ups, and the work list.

**Acceptance:** a bug created from each surface gets the next `BUG-` number, a
`bug-nnn` slug, a registered and committed draft report at the default path
with its ID, and one `bug.reported` audit row; a feature created afterwards
still gets the next `FEAT-` number; the database refuses a `bug` row with a
`FEAT-` ID and a relayed decision with no quote; `BUG-007-T01` parses as a
task.

### FR-2: Triage

**FR-2.1 — The lifecycle.**

| From | To | Who | What else happens |
|---|---|---|---|
| `reported` | `accepted` | a person, in the queue or on the bug's page; or relayed | The bug can now be sent (FR-4.1) and joined to a milestone (SD-13). |
| `reported` | `rejected` | likewise, with a reason | The feature row is abandoned with the reason, in the same transaction. |
| `reported` | `duplicate` | likewise, naming the original | The feature row is abandoned, with the reason "duplicate of BUG-004". |

Nothing moves a bug out of `accepted`, `rejected` or `duplicate` in this
milestone. An accepted bug that shouldn't be fixed after all is abandoned, as
a feature is.

**FR-2.2 — One service method**, `DecideTriage(bug, decision, reason,
duplicateOf, act)`, serves the web UI and the relay. It writes the `bugs` row's
decision columns, the abandonment where there is one, and a `bug.triaged` audit
row with the decision, the reason, the channel and any quote, in one
transaction. Refusals, each a sentence:
- the bug isn't `reported` ("BUG-004 was already accepted on …, so there is
  nothing to triage.");
- a rejection with no reason ("Say why it's rejected, so whoever reported it
  can see.");
- a duplicate naming itself, a feature rather than a bug, a bug that is
  itself a duplicate (name its original instead, and the refusal names it),
  or nothing;
- a relay with no quote (FR-6.3).

**FR-2.3 — The triage queue** (`GET /ui/triage`) lists every `reported` bug,
oldest first. Each entry shows:
- the ID and title, linking to the bug;
- where it hangs: the initiative, and the origin feature when there is one;
- who reported it and how: "Reported by Sam in the web UI", "Reported by the
  chat agent", "Reported by the implementer (claude-sonnet-5) while working on
  FEAT-003-T02", with a link to the run, or "Minor findings from the code
  review of FEAT-003-T02";
- when;
- the report's Summary, Expected and Actual, read from the file;
- three actions: **Accept**; **Reject**, with a reason field; **Duplicate
  of…**, with a field for the original's ID.

Below the queue, **Recently triaged** lists the last twenty decisions, with
who made each and any quote. An empty queue says "Nothing is waiting for
triage."

**FR-2.4 — The counts** (SD-12). The main navigation gains **Triage**, between
Inbox and Documents, with a live count of reported bugs, refreshed by the same
server-sent event the Inbox badge uses. The Inbox page shows "*n* bug
report(s) waiting for triage" above its questions when *n* > 0, linking to
the queue.

**FR-2.5 — On the bug's page**, a triage card replaces the send card while
the bug is `reported`, with the same three actions. Once decided, the card
says who decided, when, how, and the quote or reason; a duplicate links to
its original.

**Acceptance:** accepting, rejecting and marking a duplicate each work from
the queue and from the bug's page, and each refusal fires with its sentence;
a rejected bug's feature row is `abandoned`; the queue lists only reported
bugs, oldest first; the Triage badge and the Inbox line show the count and
update when a bug is triaged.

### FR-3: Reporting

**FR-3.1 — Reporting in the web UI.** The initiative and feature pages gain
**Report a bug…** in their header, opening a dialog with Title, Steps to
reproduce, Expected and Actual (all required), and Notes (optional). Posting
it (`POST /ui/bugs`) creates the bug on that initiative, or on that feature as
its origin (SD-3), reported by the UI actor as a person, and opens the bug's
page with "BUG-007 was reported. It waits in the triage queue until someone
accepts or rejects it." A bug's own page offers **Report a bug…** too, with
the bug as origin.

**FR-3.2 — Reporting from chat** is `report_bug` (FR-6.1). It needs no quote:
reporting is planning authoring (SD-11).

**FR-3.3 — Reporting by an agent at work** (SD-10). `report_bug` is a worktree
tool a role profile may declare, with arguments `title`, `steps`, `expected`,
`actual` and optional `notes`. It is not a mutating tool, so a read-only role
may declare it. It files the bug with:
- origin: the dispatch's feature;
- reporter: `agent`, the dispatch's role, and its run;
- the report's *Where it was found* section: the task (for an implementer or
  code reviewer) or the feature (for a verifier), and the run.

It returns "Filed BUG-007: *title*. It waits for a person to triage it. Carry
on with your own task; don't fix it here." Refusals, returned to the agent as
tool errors:
- a fourth report in one run;
- a title matching an open, `reported` bug on the same origin, naming it;
- an empty field.

The dispatch loop records the run's ID on the tool context, so the tool knows
who is filing. This is the only change to `internal/dispatch` besides the
tool's definition.

**FR-3.4 — Minor findings from code review** (SD-9). When a code review
approves a task and its outcome carries minor findings, the approval's
transaction also files one bug:
- title: "Minor review findings on FEAT-003-T02: *task title*";
- origin: the task's feature; reporter: `review`, the reviewer's role, the
  review run, and the task;
- the report lists each finding under Actual, with its section reference;
  Steps to reproduce says to read the task's changes at the finding's
  location; Expected says each finding is dealt with.

The `task.review_minor_findings` audit row is kept, now naming the bug. An
approval with no minors files nothing. A person's approval of an escalated
code review carries no minors and files nothing.

**Acceptance:** a person reports from an initiative page and from a feature
page; the chat agent reports over MCP; a mock implementer calls `report_bug`
and its run finishes normally, with the bug in the queue naming the run; the
fourth call in a run and a duplicate title are refused; a mock code review that
approves with two minors files one bug listing both; one with none files
nothing; every reported bug starts `reported` and nothing is dispatched for it.

### FR-4: From acceptance, the normal pipeline

**FR-4.1 — Send, for a bug** (DESIGN-010 §9, §17a item 6). A bug shows the
send card once accepted. `sendReadiness` for a bug replaces G0 and the
description check with:
- "This bug hasn't been triaged yet. A person accepts it in the triage queue
  before it can be sent to development." while `reported`;
- "This bug was rejected in triage, so it won't be sent to development." or
  "…was marked as a duplicate of BUG-004…" once decided that way;
- "This bug has no report attached, and its report is its specification.
  Attach or write one before sending it." when it has no current report.

The other preconditions — not already sent, state `idea` or `ready` — are the
feature's. The mark, the hold, Withdraw and the `FeatureSent` event are the
feature's. There is no route that sends a bug but the web UI's send form.

**FR-4.2 — The invariants, for a bug.** Invariant 1 reads the bug's current
report where a feature's reads its spec, with one difference: a bug with no
report is owed nothing, because nothing writes a fresh report. A report that
waits for its author is revised by `write-spec` (SD-7). Invariant 2 and the
estimate are unchanged: a bug with an approved report is owed a dev-plan, then
an estimate.

**FR-4.3 — Send submits the report** (SD-6). Sending a bug whose report is a
draft with nothing against it validates the report first and, if it passes,
writes the mark and submits the report in the sending person's name, which
queues its review. If it fails, nothing is sent, and the send screen names the
bug and the report's problems. A report already in review or approved is left
as it is; a draft that waits for its author is revised (FR-4.2).

**FR-4.4 — Submitting a report** is refused while its bug is `reported`, from
the document page and from `submit_for_review`: "A bug's report is reviewed
once the bug has been accepted in triage."

**FR-4.5 — The report is reviewed as a spec** (SD-8). Its review dispatch is
`review-bug_report`, by the manifest's `spec-reviewer`. Everything SPEC-011
gives a spec applies to a report: the hold, agent review switched off, human
issues and how they are answered, direct approval, sending back, asking for a
fresh review, *let the reviewer decide*, and the verdict's attribution.
Approving it re-checks the invariants and G1, as a spec's approval does. A
successor report approved on a `ready` bug supersedes its plan and returns it
to `idea`, as a successor spec does (SPEC-011 FR-6.6). An issue raised on an
approved report opens a successor.

**FR-4.6 — What the agents read.** Wherever an agent is given a feature's
spec — the dev-plan author, the implementer, the code reviewer and the
verifier — a bug's agents are given its approved report instead, under the
same heading. The verifier checks every acceptance criterion in it, the
built-in one included, with evidence; the `verify-feature` skill says that
for a bug, the evidence for "the defect no longer reproduces" is the
reproduction steps followed against the fixed code, or a test that encodes
them and passes.

**FR-4.7 — The rest is unchanged.** G1 reads the report as the spec half of
the contract. Start building, decomposition, implementation, code review,
the review round cap, verification, rework on unmet criteria, the merge and
the worktree clean-up run as for a feature. A bug's branch is named from its
slug.

**Acceptance:**
- sending refuses a reported, a rejected and a reportless bug, each with its
  sentence, and then works for an accepted one;
- the send screen for a bug says the specification step is done by the
  report, and that the spec reviewer reviews the report;
- sending submits the draft report and queues `review-bug_report`, and
  dispatches no `write-spec`;
- an invalid report stops the send with its problems;
- the reviewer's send-back dispatches `write-spec` to revise the report, whose
  prompt carries the finding; its resubmission is reviewed and approved;
- a held report waits, and *let the reviewer decide* releases it;
- **the end-to-end run** (the milestone's done-when): a mock implementer on
  one feature files a bug; a person accepts it in the UI and sends it; the
  report is reviewed and approved; the plan is written, reviewed and
  decomposed; the bug is estimated and `ready`; Start building dispatches the
  implementer; the code review approves; the verifier is given the report and
  approves with evidence for "The defect no longer reproduces"; the bug is
  `done` and merged.

### FR-5: Where bugs show

**FR-5.1 — The bug's page** is the feature page at its slug path
(`/ui/f/<initiative>/bug-007`) and at `/ui/id/BUG-007`, with:
- the bug icon and "Bug" where a feature says "Feature", and its ID;
- the report as the page's body;
- the triage card (FR-2.5), then the send card and Start building, as a
  feature's;
- "Reported on *feature*", linking to its origin, and who reported it;
- its tasks, documents and timeline, as a feature's.

**FR-5.2 — Lists.** An initiative's page and a feature's page gain a
**Bugs** section listing the open bugs hanging off it (not rejected, not
duplicate, not done), each with its ID, title, triage state or lifecycle
state, and a link. The section is left out when there are none.

**FR-5.3 — The timeline.** A bug's timeline starts with "Reported by *who*"
and then "Accepted in triage by *who*", "Rejected in triage" or "Marked as a
duplicate of BUG-004", each with the channel and any quote. After that the
report's moments read "Report submitted", "Report approved" and so on, where a
feature's read "Spec …", and the rest are a feature's.

**FR-5.4 — Review health.** The spec reviewer's reviews of reports count in
its numbers, under their own purpose, `review-bug_report`, beside
`review-spec`.

**FR-5.5 — Milestones** (SD-13). "Add to a milestone…" appears on an accepted
bug's page. The milestone edit modal's candidates include accepted bugs. MCP's
`add_milestone_member` and `remove_milestone_member` take `member_type: "bug"`
with a `BUG-` ID. Adding a bug that isn't accepted is refused: "BUG-007 hasn't
been accepted in triage, so it isn't committed work yet. Accept it first." A
bug in a milestone is listed with its ID and the bug icon, counts as one item,
and carries its tokens in the bar; shipping records it as shipped or not
shipped as it does a feature.

**Acceptance:** the bug's page renders each card in each triage state; the
initiative and feature pages list open bugs and leave out closed ones; the
rail and the initiative's feature list leave bugs out; the timeline of the
end-to-end run shows the reported, accepted, report-approved and done moments;
review health shows `review-bug_report` under the spec reviewer; a milestone
takes an accepted bug and refuses a reported one, counts it, and a milestone
holding an initiative doesn't pick up that initiative's bugs.

### FR-6: MCP

**FR-6.1 — `report_bug`** (authoring, SD-11). Arguments: `on` (an initiative
or feature, by path or ID, or a bug by ID), `title`, `steps`, `expected`,
`actual`, optional `notes`. It creates the bug reported by the chat agent,
and returns its ID, page, report path and "It waits in the triage queue for a
person to accept or reject it." Its description says the chat agent may
report on a person's behalf or its own, and that whether it is fixed is the
person's decision.

**FR-6.2 — `list_bugs` and `get_bug`** (reads). `list_bugs` takes optional
`triage` (a state, or `open` for reported or accepted and not finished) and
`on` (an initiative or feature); it returns each bug's ID, title, triage state,
lifecycle state, origin and page. `get_bug` takes a `BUG-` ID and returns
that, plus the reporter, the triage decision with its channel, reason and
quote, the report's path and review state, and, for a duplicate, the
original.

**FR-6.3 — `relay_triage`** (a relay, SD-11). Arguments: `bug`, `decision`
(`accept`, `reject` or `duplicate`), `reason` (required to reject),
`duplicate_of` (required for a duplicate), and `quote`. It calls
`DecideTriage` with `via: mcp` and the quote. A missing or empty quote is
refused before anything is looked up. Its description says it carries a
person's decision, must quote them, and must never be used on the agent's own
judgement.

**FR-6.4 — The boundary.** No MCP tool sends a bug, starts building it,
reopens a triage decision, or decides one without a quote.
`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` names the four new tools,
each with a comment saying why it is allowed, and its must-not-exist list adds
`accept_bug`, `reject_bug`, `triage_bug` and `send_bug`. The `initialize`
instructions gain a sentence: the agent may report bugs and read them, and may
carry a person's triage decision with their words; accepting a bug is theirs.

**Acceptance:** over `POST /mcp`, each tool works on its happy path; a relay
leaves the `bug.triaged` audit row with `via: mcp` and the quote; a relay with
no quote, a rejection with no reason, and a duplicate of a duplicate each fail
with a sentence; the advertised set is exact.

### FR-7: The starter pack

**FR-7.1** The pack adds `templates/bug_report/` (FR-1.5).

**FR-7.2** `roles/implementer.yaml`, `roles/code-reviewer.yaml` and
`roles/verifier.yaml` add `report_bug` to their tools, with a line in their
tool hints.

**FR-7.3** The `implement-task`, `review-code` and `verify-feature` skills
gain a short section: what is out of scope for your task, report with
`report_bug`, and never fix in passing. `review-spec` gains the section for
reviewing a bug report (SD-8), and `write-spec` the section for revising one
(SD-7). `verify-feature` gains FR-4.6's sentence.

**FR-7.4** A project made before M12 has none of this until it copies the
files from a fresh `init`. Its agents simply have no `report_bug` tool, its
reports can't be created (the template is missing, and the refusal says so),
and nothing else changes. The handoff says how to upgrade.

**Acceptance:** a fresh `init` has the template and the three roles with the
tool; the config loader accepts `report_bug` on a read-only role; creating a
bug in a project without the template is refused with a sentence.

## 4. Non-functional requirements

- **NFR-1 — One service layer.** The UI handlers, the MCP tools and the agent
  tool call the same service methods (`ReportBug`, `DecideTriage`), each in
  one transaction with its audit row (O-3).
- **NFR-2 — The orchestrator stays code.** Nothing an agent reports is
  dispatched until a person accepts it and sends it. Filing a report
  dispatches nothing.
- **NFR-3 — The seam holds.** Only the web UI sends a bug. No agent tool and no
  MCP tool changes a triage decision without a person's quoted words, and a
  database check refuses a relayed decision without one.
- **NFR-4 — No second pipeline.** A test asserts the end-to-end run's
  dispatches are exactly the feature pipeline's purposes, minus the first
  `write-spec`, with `review-bug_report` where a feature has `review-spec`.
- **NFR-5 — Human prose** in every label, refusal, notice and tool
  description (D-6).
- **NFR-6 — Contained templates.** New markup lives in `bug.html` (the queue,
  the triage card, the report dialog, the bugs list). `entity.html`,
  `inbox.html` and `partials.html` gain only includes, the header button, the
  inbox line and the nav item.
- **NFR-7 — Coordination.** Migration `0013` only. In M11's files, only
  SD-16's two calls. In `internal/dispatch`, only the tool's definition and
  the run's ID on the tool context. The MCP tool-set test's list is merged by
  hand with M11's.
- **NFR-8 — No typed paths.** Every new form carries row ids or IDs in hidden
  fields, and the typed-path scan covers them.
- **NFR-9 — Tested as before.** Integration tests with the mock provider
  against real Postgres cover every FR. `go vet ./...` and
  `go test -race -count=1 ./...` are clean, and the integration tests run
  rather than skip.

## 5. Definition of done

1. Every FR's acceptance passes in the suite, including the end-to-end run.
2. `go vet ./...` and `go test -race -count=1 ./...` are clean.
3. **A browser walkthrough without an AI provider**, with Playwright and the
   pre-installed Chromium: report a bug in the UI; see it in the triage queue
   with the counts; accept it; send it, and see its report submitted for
   review. Relay a triage decision over MCP. Screenshots and
   `docs/walkthrough-spec-019.md`. The dispatched steps are covered by the
   mock-provider tests, and the walkthrough says so.
4. A live smoke checklist for Sam in the handoff.
5. A handoff note, `docs/notes/handoff-M12-2026-09-28.md`.
6. The roadmap's §11 marks M12 done with a pointer to the handoff.
7. REVIEW-019 is recorded and its findings dealt with (§7).
8. **Fourteen choices need Sam's explicit yes:**
   1. a bug is a feature row with a kind, and its own facts sit in a side
      table (SD-1);
   2. triage is a column, and rejecting or marking a duplicate abandons the
      row; triage is final (SD-2);
   3. a bug always has an initiative, and optionally an origin feature (SD-3);
   4. the report's ID is `BUG-007-bug-report`, not §7's `BUG-007-report`
      (SD-4);
   5. the built-in criterion is enforced by a new `contains_text` rule (SD-5);
   6. a report is reviewed only once its bug is accepted, and Send submits it
      (SD-6);
   7. a sent-back report goes to the spec author, bounded by the round cap
      (SD-7);
   8. the review differs by skill, not by prompt (SD-8);
   9. **minor code-review findings: one bug per approving review, no
      cross-review de-duplication, and spec and plan review minors stay on
      their threads** (SD-9);
   10. agents file with a tool, at most three per run, refusing a duplicate
       title (SD-10);
   11. chat reports without a quote and relays triage with one (SD-11);
   12. the queue is its own page with its own count, and the Inbox shows a
       line; the Inbox badge counts checkpoints only (SD-12);
   13. only accepted bugs join milestones, and an initiative doesn't bring in
       its bugs (SD-13);
   14. bugs block archiving their initiative until finished (SD-14).
9. **On approval**, a dated note in DEC-006 Amendment 1 records
   `relay_triage` as the relay DESIGN-010 §17a item 6 allowed, and one in
   DESIGN-010 §9 records how the bug was built.

## 6. Open questions carried forward

- **Reopening a triage decision.** A rejected bug that turns out to matter is
  reported again today. An *Undo* within some window may be wanted.
- **Project-level bugs.** A bug found in "the whole thing" hangs off the
  initiative closest to it. If that's often awkward, the feature row's
  initiative would need to become optional, which is a larger change.
- **Turning off minor findings as bugs.** If the queue fills with them, a
  project setting, or filing them already rejected as "noted", may be wanted.
  The live runs will say.
- **A spec reviewer reviewing a report has no design to hold it against.**
  It judges clarity and testability only. Whether the plan reviewer should see
  the origin feature's spec is a question for the live runs.
- **Existing projects need new files** (FR-7.4). A `subutai upgrade` that
  copies missing starter files would help every milestone, not just this one.
- **Bugs in the rail.** The project-structure rail shows features only. If
  people want bugs there too, it is a presentation change.

## 7. Changes after review

*To be filled in from REVIEW-019.*
