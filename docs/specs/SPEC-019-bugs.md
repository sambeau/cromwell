# SPEC-019: Bugs

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. Sam has said they will
approve the spec and the build together. An independent consistency review is
recorded in [REVIEW-019](../reviews/REVIEW-019-bugs.md). It found five
material and fifteen smaller problems in the first draft; all are dealt with
in this revision, and §7 says how, finding by finding. Nineteen choices need
Sam's explicit yes (§5, DoD 8).
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
**Authority:** [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md)
(the chat agent may "create and edit … features (as ideas)", so reporting a
bug is planning authoring);
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

"The reporter" is whoever the configured UI or MCP actor is, until per-user
identity exists (roadmap decision 14), so "Reported by Sam" means the UI
actor's configured name.

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
milestones — is the code that already runs features. It reads one extra fact
wherever it asks for a feature's spec: for a bug, that is its report (FR-4.0).

## 1. Goal

**One claim, which the definition of done checks directly:**

> An agent files a bug mid-task. A person accepts it in the triage queue and
> sends it to development. The spec reviewer reviews its report as a spec, the
> plan is written and reviewed, and it is estimated. A person presses Start
> building; the fix is implemented, code-reviewed, and verified as "the defect
> no longer reproduces", and it merges.

Three supporting claims:

> Anyone can report a bug — a person in the web UI, the chat agent, an agent
> at work, or the code reviews' minor findings — and none of them can decide
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
| Reopening a rejected or duplicate bug | §6. A new report does the job today (choice 16). |
| Bugs owned by the project itself, above every initiative | §6. A bug hangs off an initiative or a feature, as §9 says. |
| A setting to stop filing minor findings as bugs | §6, once the live runs show the volume |
| Per-user identity for reporters and triagers | Before the Tickly pilot (roadmap decision 14). |

### Scope decisions

- **SD-1 — A bug is a feature row with a kind, not a separate table.** The
  test the brief sets is "no parallel machinery". Every stage after acceptance
  keys off a `features` row: the sent mark (`feature_sends`), the spec and plan
  (documents owned by a feature), tasks, the worktree and branch, dispatches
  with `ref_type = 'feature'`, estimates, gates G1 to G3, the timeline,
  milestone leaves and snapshots. A separate `bugs` table sharing the
  lifecycle would need a twin of each, or a polymorphic reference in each; a
  kind column needs neither. So `features` gains `kind` (`feature` or `bug`),
  and a bug's own facts live in a one-to-one side table, `bugs`, as the sent
  mark lives in `feature_sends` (SPEC-011 SD-1).

  The cost is paid in two places, and REVIEW-019 R19-1 showed the first
  draft understated the second:
  - at the reads that mean *features only*, which filter on kind (FR-1.6);
  - at the thirty or so places where the code asks for "the feature's spec"
    by the literal type `spec`. Each now asks for the spec type of the row's
    kind (FR-4.0, Appendix A).

  The alternative REVIEW-019 weighed — a bug's report as a `spec` document
  with a template chosen by kind — would remove the second cost but
  contradicts DESIGN-010 §4 and §7, which name bug reports as their own kind
  of document. **Flagged for Sam.**

- **SD-2 — Triage is a column on the side table, not a feature state.**
  DESIGN-003's feature machine is unchanged. A reported bug is an `idea` with
  `triage = 'reported'`. Accepting it leaves it an `idea`, now eligible to be
  sent. Rejecting it, or marking it as a duplicate, abandons the feature row in
  the same transaction, with the reason, so every existing count of open work
  (G5, milestones, lists) reads it as finished without learning about triage.
  Triage is final in this milestone: nothing un-rejects a bug (choice 16).

- **SD-3 — A bug hangs off an initiative, and optionally a feature.** A
  feature row needs an initiative, so a bug always has one: the initiative it
  was reported on, or the initiative of the feature it was reported on. The
  feature, when there is one, is recorded as its *origin*. A bug filed by an
  agent or from review findings has the dispatch's feature as its origin —
  which may itself be a bug, when the defect was found while fixing another.

- **SD-4 — The report's ID is `BUG-007-bug-report`.** A document's ID is its
  owner's ID plus its type's slug (SPEC-015 FR-3.1), and the type is
  `bug_report`, as the brief names the template. DESIGN-010 §7's folder
  example shows `BUG-031-report.md`; making it so would need `report` to mean
  two types, and `report` is already a value of the `document_type` enum. The
  example is read as illustrative, and a dated note in §7 says so on approval.
  **Flagged for Sam.**

- **SD-5 — The built-in criterion is enforced, not just offered.** Every
  report's Acceptance criteria must have a list item that **starts** "The
  defect no longer reproduces", checked by a new manifest rule,
  `contains_text`, at submission like every other structural rule. Case and
  spacing are forgiven, and an item may run on ("…: the steps now give the
  expected behaviour"); the phrase inside another item ("It works, and the
  defect no longer reproduces") doesn't count. The template and every
  generated report start with it. A person may add criteria; they can't
  remove that one. DESIGN-004 lists rule kinds, so a dated note there records
  the new one on approval.

- **SD-6 — A bug's documents are reviewed once the bug is accepted, and Send
  submits the report.** Reviewing spends agent time, and accepting is the
  commitment of scope (DESIGN-010 §9). So no document a bug owns — its report,
  or a plan written ahead in chat — can be submitted while the bug is
  `reported`, or once triage has closed it. The check is in the service that
  every submission path reaches (R19-4). Once the bug is accepted, a person or
  the chat agent may submit its report early, as with a spec (§5a). **Send
  submits it** when it is still a draft with nothing against it: for a bug,
  sending *is* "review this and go on", and there is no author agent whose
  turn it would otherwise be. This departs, for bugs only, from SPEC-017
  FR-5.2's reading that nothing submits a draft nobody has submitted.

- **SD-7 — A report sent back goes to the spec author to revise.** When the
  spec reviewer or a person sends back a sent bug's report, the draft waits
  for its author, exactly as a spec does (SPEC-011 SD-4), and the author is
  the spec author, dispatched as `write-spec` on the bug. It revises the
  report in place. Its prompt says it is revising a bug report, which
  translates no design, and never to invent reproduction steps the report
  and findings don't support (SD-16). If the loop doesn't converge, the round
  cap raises the `authoring-deadlock` question as it does for a spec, and a
  person edits the report. This keeps one loop, not two. **Flagged for Sam:**
  the alternative is to leave a sent-back report for a person always.

- **SD-8 — The spec reviewer reviews the report with its own skill.** The
  `bug_report` manifest names `spec-reviewer`, so the review dispatch is
  `review-bug_report`, run by the same role and model. What it checks
  differently (reproduction and acceptance clear and testable, one defect per
  report, and no design to be faithful to) is written into the `review-spec`
  skill, which gains a section for bug reports, rather than into prompt
  assembly, which M11 owns. Holds, human issues, direct approval, *let the
  reviewer decide* and agent review switched off apply to a report exactly as
  to a spec, because the report *is* the spec (DEC-006 Amendment 1). A
  project's `routing` may name `review-bug_report` separately; otherwise the
  role's model runs it.

- **SD-9 — Minor code-review findings become one bug per feature, filed when
  it merges.** DESIGN-010 §5 records minor findings "as bug reports in the
  triage queue". Three groupings were weighed:
  - **per finding**, with de-duplication: a typical approving review carries
    two to five minors, so this floods the queue;
  - **per approving review**: one bug per task, but filed while the task's
    code is on the feature's unmerged branch, where a fix can't be built
    (R19-2);
  - **per feature, at the merge** (recommended): every approving review's
    minors across the feature's tasks become **one** bug on it, filed when the
    feature merges. It is one queue entry per feature, the code the findings
    describe is on the main line when anyone can act on it, and findings on
    work that is abandoned are never filed.

  De-duplication across features isn't attempted: a person marks a
  duplicate in one act. Minor findings from spec and plan reviews stay on the
  document's thread, where they already are: they are about a document, not
  a defect in the software. Minors raised on a send-back round and not
  addressed by the implementer ride back with the majors, and aren't filed,
  as today. **Flagged for Sam, as the brief asks.**

- **SD-10 — Agents file with a tool, capped per run.** Implementers, code
  reviewers and verifiers get a `report_bug` tool through their role profiles.
  It files the report and returns, and the agent carries on with its own task:
  the tool's description, and each role's skill, say plainly that the agent
  must not fix what it reports. A run — one dispatch row, across its retried
  attempts — may file at most three reports; the fourth call is refused with
  a sentence, as a guard against an agent that starts reporting instead of
  working. A report whose title matches, ignoring case and surrounding space,
  a `reported` bug on the same origin is refused as a duplicate, naming it.
  Authors, document reviewers and the estimator aren't offered worktree tools
  at all, so the config loader refuses `report_bug` on a role bound to one,
  rather than letting it be ignored without a word (R19-10).

- **SD-11 — The chat agent reports without a quote and relays triage with
  one.** Reporting is planning authoring (DEC-004): it creates an idea and
  commits nothing, like `create_feature`. Triage is a person's decision
  (DESIGN-010 §9), so the chat agent may only carry it, with the person's
  words, through one relay tool, `relay_triage`. DEC-006 Amendment 1 says a
  relay not on its list needs a decision; DESIGN-010 §17a item 6, accepted by
  Sam, is that decision, and a dated note in DEC-006 records it on approval.

  **One consequence departs from Amendment 1's test** (R19-5). Amendment 1
  admits a relay because "everything on the relay list is small and
  recoverable". A relayed *accept* passes: it starts nothing, because Send
  stays in the web UI. A relayed *reject* or *duplicate* abandons the bug's
  row, and with triage final (SD-2) the only remedy for a mistaken one is a
  new report, which loses the thread. It spends nothing and loses no work,
  and the quote makes a mistaken relay easy to spot, so the recommendation is
  to accept it as it stands (choice 16). The alternatives are a web-UI-only
  *Reopen triage*, or relaying accept alone.

- **SD-12 — The queue is its own page, and the count shows in the navigation
  and on the Inbox.** Roadmap decision 12 asks for "a dedicated queue, with a
  count on the inbox". The queue is `/ui/triage`, in the main navigation
  beside the Inbox, with its own count badge. The Inbox page shows a line
  above its questions — "3 bug reports are waiting for triage." — linking to
  the queue. The Inbox's own attention badge still counts checkpoints only,
  because a checkpoint means work is stopped and a report doesn't. R19-18
  suggests a second, neutral count inside the Inbox's navigation item
  instead; the Triage item beside it says the same thing without two numbers
  in one place. **Flagged for Sam.**

- **SD-13 — A bug joins a milestone as itself, once accepted.** DESIGN-010
  §17a item 3 makes bugs deliverables. A bug is added to a milestone like a
  feature, from either end, and counts as one item, with its tokens in the
  bar. It must be accepted first: a reported bug isn't committed scope, and a
  rejected one never will be. **An initiative in a milestone brings in its
  features, not its bugs**, as it doesn't bring in its checklists (SPEC-014
  SD-4): otherwise a *live* milestone would grow with every bug accepted
  under its initiatives, and a release's scope would be set by triage rather
  than by whoever plans the milestone. This reads DESIGN-010 §6's "everything
  under them" narrowly for bugs, so a dated note in §6 records it on approval.
  A done bug is a done item, so it can let a milestone be marked as shipped
  (G4 counts items: DEC-004 Amendment 1, note of 2026-09-28); a note there
  records that too. **Flagged for Sam.**

- **SD-14 — Bugs count as open work under their initiative.** G5 refuses to
  archive an initiative while any feature row under it is not terminal. A
  reported or accepted bug is open work, so it blocks archiving too. Rejected
  and duplicate bugs are abandoned (SD-2), so they don't. **Flagged for Sam.**

- **SD-15 — The initiative's send screen lists features only.** An accepted
  bug is sent from its own page. The initiative's "Send features to
  development…" screen keeps to features, as its name says; a person sending
  bugs is sending one fix at a time, which is how they are triaged.

- **SD-16 — Small changes in M11's files, named.** `planner.go` is M11's.
  These changes are unavoidable, each is a line or two calling into this
  spec's code, and the handoff names them for the merge:
  - `contractBody` reads a bug's report where it reads a feature's spec, so
    the dev-plan author, the implementer, the code reviewer and the verifier
    are given the report;
  - `planAuthor` writes the row's spec type (the report, for a bug), heads
    the prompt "The bug to write for", replaces the approved designs with a
    short note that this is a bug report and not to invent what it doesn't
    say, and describes the `contains_text` rule in its structure list.

  `review_prompt.go` is untouched: what the reviewer checks differently is in
  its skill (SD-8).

- **SD-17 — A bug found in unmerged work waits for it to merge.** A bug's
  worktree is made from the main line. A bug reported on a feature that is
  being built (`active` or `review`) may describe code only on that feature's
  branch, so a fix built from the main line would fix nothing, and the
  verifier would confirm a defect absent from code where it never existed.
  So such a bug can be accepted but not sent until its origin has merged;
  the send card says so (R19-2). An origin that is an `idea`, `ready`, `done`
  or `abandoned` holds nothing back. **Flagged for Sam.**

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
    or a review), `source_task_id` (the task, for an agent at work on one),
    `reported_at`;
  - `duplicate_of` (references `features`, nullable);
  - `decided_by`, `decided_via` (`ui` or `mcp`), `decided_quote`,
    `decided_reason`, `decided_at`.
- Checks: a duplicate has `duplicate_of` and nothing else does, and it isn't
  itself; a decided bug has `decided_by`, `decided_via` and `decided_at`, and
  an undecided one has none; `decided_via = 'mcp'` needs a quote; a rejection
  needs a reason; an agent or review report names its run.
- Indexes on `bugs (triage, reported_at)` for the queue and on the origin.

`store.CreateBug` is the only writer of a bug row and its side row, in one
transaction, which a test covers; the schema doesn't also enforce the pairing
(R19-20). `DecideTriage` is the only writer of a decision, and it names an
original only after resolving it as a bug.

**FR-1.2 — The registry.** `internal/ident`'s `bug` entry names the
`features` table. A task of a bug is numbered from the bug's ID,
`BUG-007-T01`, and `ident.Parse` reads it as a task. `/ui/id/BUG-007`, and
every tool that takes a feature's ID, accept a bug's (R19-9).

**FR-1.3 — Creating a bug** (`store.CreateBug`) inserts the feature row with
`kind = 'bug'`, a `BUG-` ID, a slug made from the ID (`bug-007`), the title as
its name, and the report's Summary as its description; then the `bugs` row;
then a `feature.created` row (with `kind: bug`, so everything that reads a
feature's history reads a bug's) and a `bug.reported` row naming the
reporter, the origin, the run and the channel. Both audit rows have
`ref_type = 'feature'`; the old `defect` value of `ref_type` stays unused.
One transaction. It publishes `FeatureCreated` after commit, so the
invariants re-check as for any new feature, and find nothing to do (FR-4.2).

A feature's slug may not look like a bug's (`bug-` and digits), so the two
never clash (R19-6). Every refusal of a report runs before anything is
minted; a creation that fails for another reason skips a number, which is
accepted, as it is for features.

**FR-1.4 — The report.** Creating a bug writes its report from the project's
`bug_report` template at the default home:
`docs/work/<INIT-ID>-<slug>/BUG-007-bug-report.md`. It is registered as a
`draft`, owned by the bug, marked as the bug's main document, given its ID
and revision 1, recorded as written by the reporter (SPEC-017 FR-2), and
committed on its own, as a starter design is (SPEC-015 FR-6).

The report is filled from the reporter's fields in one pass, so a field that
happens to contain `{{notes}}` isn't read as a placeholder (R19-3):
- **Summary** is the reporter's, or, when none is given, the title as a
  sentence;
- **Steps to reproduce** becomes a numbered list, one step per non-empty
  line, unless it is a list already;
- **Actual** and **Notes** are quoted text: a log line, template braces, the
  word TODO or a line starting `#` is fenced, so it stays text;
- an optional section with nothing in it is left out, heading and all.

The filled report is validated before anything is minted. A report that
wouldn't pass its template's checks is refused with its problems, as a
sentence the reporter can act on, and nothing is created. A project without
the template is refused with a sentence saying where to copy it from.

Validation (DESIGN-003 §3, check 4) no longer counts `{{…}}` or `TODO` inside
fenced code as an unfinished placeholder, for any document type: fenced code
is quoted material, and the parser already treats it as text (choice 18).

**FR-1.5 — The template** `templates/bug_report` has front matter `title`,
`type` and `owner`, and these sections in order:

| Section | Required | What it holds |
|---|---|---|
| Summary | yes | One or two sentences: what is wrong. |
| Steps to reproduce | yes | A numbered list someone else can follow. At least one item. |
| Expected | yes | What should happen. |
| Actual | yes | What happens instead. |
| Acceptance criteria | yes | At least one item, and one must start "The defect no longer reproduces" (SD-5). |
| Where it was found | no | The feature, task or run it came from. |
| Notes | no | Anything else: logs, a guess at the cause. |

The manifest names `spec-reviewer` and agent approval, and adds the rules
`min_list_items` on Steps to reproduce and Acceptance criteria, and
`contains_text` on Acceptance criteria.

**FR-1.6 — Features only, where the word means it.** These reads leave bugs
out:
- an initiative's child features, on its page, in the rail, in
  `get_initiative` and `get_tree`;
- Home's features by state and the Work list, which show features; bugs are
  in the triage queue and on their initiative's and feature's pages;
- the initiative's send screen (SD-15);
- the design cascade's and the reconciler's initiative scope, because a bug
  has no spec for a design to revise;
- an initiative's features resolved into a milestone (SD-13).

Everything else reads every row: G5 (SD-14), the heartbeat sweep,
dispatches, the initiative's token roll-up, the corpus and cost roll-ups, and
`GET /api/features` by path. The milestone candidates list shows accepted
bugs only (FR-5.5).

**Acceptance:** a bug created from each surface gets the next `BUG-` number, a
`bug-nnn` slug, a registered and committed draft report at the default path
with its ID, as its main document, and one `bug.reported` audit row; a
feature created afterwards still gets the next `FEAT-` number; a feature
slugged `bug-009` is refused; a report missing a part is refused and creates
nothing; the database refuses a `bug` row with a `FEAT-` ID and a relayed
decision with no quote; `BUG-007-T01` and `BUG-007-bug-report` parse; a
chat-reported Notes holding braces and TODO is fenced and validates.

### FR-2: Triage

**FR-2.1 — The lifecycle.**

| From | To | Who | What else happens |
|---|---|---|---|
| `reported` | `accepted` | a person in the web UI, or relayed | The bug can now be sent (FR-4.1) and joined to a milestone (SD-13). |
| `reported` | `rejected` | likewise, with a reason | The feature row is abandoned with the reason, in the same transaction. |
| `reported` | `duplicate` | likewise, naming the original | The feature row is abandoned, with the reason "marked in triage as a duplicate of BUG-004". |

*Duplicate* is a third outcome beside §9's accept and reject: it is a
rejection that says which bug to follow instead (choice 17). Nothing moves a
bug out of `accepted`, `rejected` or `duplicate` in this milestone. An
accepted bug that shouldn't be fixed after all is abandoned, as a feature is.
A **reported** bug can't be abandoned round triage, from the web UI or the
API: "This bug hasn't been triaged yet. Reject it in the triage queue
instead, with a reason, so whoever reported it can see why." (R19-4).

**FR-2.2 — One service method**, `DecideTriage(bug, decision, reason,
duplicateOf, act)`, serves the web UI and the relay. It writes the `bugs` row's
decision columns, the abandonment where there is one, and a `bug.triaged` audit
row with the decision, the reason, the channel and any quote, in one
transaction. Refusals, each a sentence:
- the bug isn't `reported` ("BUG-004 was already accepted on 28 September
  2026, so there is nothing to triage.");
- a rejection with no reason ("Say why it's rejected, so whoever reported it
  can see.");
- a duplicate naming itself, a feature rather than a bug, a bug that is
  itself a duplicate (the refusal names its original), or nothing;
- a relay with no quote (FR-6.3).

**FR-2.3 — The triage queue** (`GET /ui/triage`) lists every `reported` bug
still an `idea`, oldest first. Each entry shows:
- the ID and title, linking to the bug;
- who reported it and how, as a sentence: "Reported by Sam in the web UI.",
  "Reported by the chat agent.", "Reported by the implementer
  (claude-sonnet-5) while working on FEAT-003-T02.", or "Minor findings from
  the code review of FEAT-003, by the code reviewer (claude-sonnet-5).";
- when, and where it hangs: the origin feature when there is one, and the
  initiative;
- the report's Summary, Expected and Actual, with a link to the whole report;
- three actions, each saying what it will do: **Accept it**; **Reject it**,
  with a reason field; **Mark as a duplicate**, with a field for the
  original's ID.

Below the queue, **Recently triaged** lists the last twenty decisions, with
who made each, and any reason or quote. An empty queue says "Nothing is
waiting for triage".

**FR-2.4 — The counts** (SD-12). The main navigation gains **Triage**, just
after Inbox, with a live count of reported bugs, refreshed by the same
server-sent event the Inbox badge uses. The Inbox page shows "1 bug report is
waiting for triage." or "3 bug reports are waiting for triage." above its
questions when there are any, linking to the queue. Reporting and triage
both fire the change signal, so an open page updates.

**FR-2.5 — On the bug's page**, a triage card in the rail shows the triage
state, who reported it, and its origin. While the bug is `reported`, the
card says nothing runs until a person decides and links to its entry in the
queue, where the decision is made; the send card and *Add to a milestone…*
aren't shown, and *Abandon* isn't offered. Once decided, the card says who
decided, when, how, and the reason or quote; a duplicate links to its
original. (The triage form also accepts a decision posted from the bug's
page, returning there, which the queue's link doesn't need but a future
button may.)

**Acceptance:** accepting, rejecting and marking a duplicate each work, and
each refusal fires with its sentence; a rejected bug's feature row is
`abandoned` and its timeline has one moment for it; the queue lists only
reported bugs, oldest first; the Triage badge and the Inbox line show the
count and update; abandoning a reported bug through the API is refused.

### FR-3: Reporting

**FR-3.1 — Reporting in the web UI.** The initiative and feature pages gain
**Report a bug…** in their menu, opening a dialog with Title, Steps to
reproduce, What should happen and What happens instead (all required), and
Notes (optional). Posting it (`POST /ui/bugs`) creates the bug on that
initiative, or on that feature as its origin (SD-3), reported by the UI actor
as a person, and opens the bug's page with "BUG-007 was reported. It waits in
the triage queue until someone accepts or rejects it." A bug's own page
offers **Report a bug…** too, with the bug as origin.

**FR-3.2 — Reporting from chat** is `report_bug` (FR-6.1). It needs no quote:
reporting is planning authoring (SD-11).

**FR-3.3 — Reporting by an agent at work** (SD-10). `report_bug` is a worktree
tool a role profile may declare, with arguments `title`, `steps`, `expected`,
`actual` and optional `notes`. It is not a mutating tool, so a read-only role
may declare it. It files the bug with:
- origin: the dispatch's feature;
- reporter: `agent`, the dispatch's role, and its run; and the task, when
  the run is on one;
- the report's *Where it was found* section: the task or feature, the role,
  and the run.

It returns "Filed BUG-007: *title*. It waits for a person to triage it. Carry
on with your own task; don't fix it here." Refusals, returned to the agent as
tool errors, all before anything is minted:
- a fourth report in one run;
- a title matching a `reported` bug on the same origin, naming it;
- a missing field.

The dispatch loop puts the run's ID and role on the tool context, so the tool
knows who is filing.

**FR-3.4 — Minor findings from code review** (SD-9). When a code review
approves a task with minor findings, the approval's audit row
(`task.review_minor_findings`) records them with the review's run, as
before. When the feature merges, every such row across its tasks becomes one
bug:
- title: "Minor review findings on FEAT-003: *feature name*";
- origin: the feature; reporter: `review`, the code reviewer's role and the
  latest review run that raised findings;
- the report lists each finding under Actual, grouped by task, with its
  location; Steps to reproduce says to read the feature's changes, now on the
  main line, at each place a finding names; Expected says each finding is
  dealt with, or recorded as not worth doing.

It is filed after the merge commits, best effort: a failure is logged and
the merge stands. A feature with no minors files nothing, and nor does a
person's approval of an escalated review, which carries none.

**Acceptance:** a person reports from an initiative page and from a feature
page; the chat agent reports over MCP; a mock implementer calls `report_bug`
and its run finishes normally, with the bug in the queue naming the run; the
fourth call in a run and a duplicate title are refused; a mock code review
that approves with two minors files, at the merge, one bug listing both; a
build with none files nothing; every reported bug starts `reported` and
nothing is dispatched for it.

### FR-4: From acceptance, the normal pipeline

**FR-4.0 — The one rule** (R19-1). Wherever the code asks whether a document
is a feature's spec, it asks `lifecycle.IsSpecType`, true for `spec` and
`bug_report`; wherever it asks whether a document is either half of a
contract, it asks `lifecycle.IsContractType`. Wherever it reads "the
feature's spec", it reads the spec type of the row's kind (`specTypeOf`), so
a bug's is its report. Appendix A lists every site and what it now does.
A bug has no `spec`, and a feature has no `bug_report`: every registration
path — attach, adopt, authored documents, starter documents, reports —
refuses the wrong one with a sentence (R19-4).

**FR-4.1 — Send, for a bug** (DESIGN-010 §9, §17a item 6). `sendReadiness`
asks a bug's triage first, then the feature's usual state check, and
replaces G0 and the description check with:
- "This bug hasn't been triaged yet. A person accepts it in the triage queue
  before it can be sent to development." while `reported`;
- "This bug was rejected in triage, so it won't be sent to development." or
  "This bug was marked as a duplicate in triage, so it won't be sent to
  development. Send the bug it repeats." once decided that way;
- "This bug has no report attached, and its report is its specification.
  Attach or write one before sending it." when it has no current report;
- "This bug was reported on FEAT-003 while it is being built, so the code it
  describes may not be on the main line yet. It can be sent once FEAT-003 has
  merged." while its origin is `active` or `review` (SD-17).

Not already sent, and state `idea` or `ready`, are the feature's
preconditions. The mark, the hold, the `FeatureSent` event and the rule that
only the web UI's send form writes the mark are the feature's.

**FR-4.2 — The invariants, for a bug.** Invariant 1 reads the bug's current
report where a feature's reads its spec, with one difference: a bug with no
report is owed nothing, because nothing writes a fresh report. A report that
waits for its author is revised by `write-spec` (SD-7). Invariant 2 and the
estimate are unchanged: a bug with an approved report is owed a dev-plan, then
an estimate.

**FR-4.3 — Send submits the report** (SD-6), in this order:
1. if the bug's report is a draft with nothing against it, it is validated;
   if it fails, nothing is sent, and the send screen names the bug and the
   report's problems;
2. the mark is written, as for a feature;
3. the report is submitted in the sending person's name, which queues its
   review.

If step 3 fails after the mark, which validation in step 1 makes unlikely,
the failure is logged and the report stays a draft with nothing against it;
a person can press Submit on it. A report already in review or approved is
left as it is; a draft that waits for its author is revised (FR-4.2).

**FR-4.4 — Documents wait for triage.** Submitting any document a bug owns is
refused while the bug is `reported` ("A bug's documents are reviewed once the
bug has been accepted in triage.") or closed by triage ("This bug was
rejected in triage, so its documents aren't reviewed."). The check is in the
submission service, so the document page, `submit_for_review`,
`POST /api/docs/submit`, the editor and the authoring loop all meet it. The
document page says why its Submit button isn't there.

**FR-4.5 — The report is reviewed as a spec** (SD-8). Its review dispatch is
`review-bug_report`, by the manifest's `spec-reviewer`. Everything SPEC-011
gives a spec applies to a report: the hold, agent review switched off, human
issues and how they are answered, direct approval, sending back, asking for a
fresh review, *let the reviewer decide*, and the verdict's attribution. The
document page says "This bug report is waiting for you" where a spec's says
"This specification…". Approving it re-checks the invariants and G1, as a
spec's approval does. A successor report approved on a `ready` bug supersedes
its plan and returns it to `idea`, as a successor spec does (SPEC-011 FR-6.6).
An issue raised on an approved report opens a successor.

**FR-4.6 — What the agents read.** Wherever an agent is given a feature's
spec — the dev-plan author, the implementer, the code reviewer and the
verifier — a bug's agents are given its approved report instead, under the
same heading. The verifier checks every acceptance criterion in it, the
built-in one included, with evidence; the `verify-feature` skill says that
for a bug, the evidence for "the defect no longer reproduces" is the
reproduction followed against the fixed code, or a test that encodes it and
passes.

**FR-4.7 — Withdraw, for a bug** (R19-12). A bug's send set off its report's
review, so Withdraw needs that review to be still queued too, as well as any
authoring runs. Withdrawing cancels it, clears any hold, and returns the
report to `draft`, as it was before the send.

**FR-4.8 — The rest is the feature's.** G1 reads the report as the spec half
of the contract (FR-4.0). Start building, decomposition, implementation, code
review, the review round cap, verification, rework on unmet criteria, the
merge and the worktree clean-up run as for a feature. A bug's branch is named
from its slug, and its tasks from its ID.

**Acceptance:**
- sending refuses a reported, a rejected and an invalid-report bug, and one
  whose origin is being built, each with its sentence, and then works for an
  accepted one;
- a reported bug's report can't be submitted over MCP or the API;
- the send screen for a bug says the specification step is done by the
  report, and that the spec reviewer reviews the report as a specification;
- sending submits the draft report and queues `review-bug_report`, and
  dispatches no `write-spec`;
- the reviewer's send-back dispatches `write-spec`, which revises the report
  in place with a prompt carrying the finding, the bug-report note and the
  `contains_text` rule, and no designs; its resubmission is approved;
- a held report waits, and *let the reviewer decide* releases it; with agent
  review off, no agent reviews it; a person's issue on it in review sends it
  back;
- withdrawing a bug while its report's review is queued cancels the review
  and returns the report to draft;
- **the end-to-end run** (the milestone's done-when): a mock implementer on
  one feature files a bug and finishes its task; the code review approves
  with two minors, which are filed as one bug at the merge; a person accepts
  the agent's bug in the UI and sends it; the report is reviewed and approved
  by the spec reviewer; the plan is written, reviewed and decomposed; the bug
  is estimated and `ready`; Start building dispatches the implementer; the
  code review approves; the verifier is given the report and approves with
  evidence for "The defect no longer reproduces"; the bug is `done`, merged,
  and its task is `BUG-001-T01`; the bug's runs are exactly the feature
  pipeline's purposes, less `write-spec`, with `review-bug_report` for
  `review-spec` (NFR-4).

### FR-5: Where bugs show

**FR-5.1 — The bug's page** is the feature page at its slug path
(`/ui/f/<initiative>/bug-007`) and at `/ui/id/BUG-007`, with:
- the bug icon in the header and the breadcrumb, and its ID;
- the report as the page's body;
- the triage card (FR-2.5), then, once accepted, the send card and Start
  building, as a feature's;
- its tasks, documents and timeline, as a feature's;
- *Abandon this bug* in the menu, once it is triaged.

**FR-5.2 — Lists.** An initiative's page lists the open bugs hanging off it
directly, and a feature's page the open bugs reported on it, in a **Bugs**
section: each with its ID, title, and "Waiting for triage" or its lifecycle
state. *Open* means reported or accepted, and not `done` or `abandoned`. The
section is left out when there are none.

**FR-5.3 — The timeline.** A bug's timeline starts with "Reported", "Reported
by an agent at work" (the agent's act) or "Reported from a code review's
minor findings", then "Accepted in triage", "Rejected in triage" or "Marked
as a duplicate in triage", attributed as any moment is, a relayed decision
as the person's. The report's writing isn't a second moment, and nor is an
abandonment triage caused. After that the report's moments read "Report
approved", "Report sent back" and so on, where a feature's read "Spec …",
and the rest are a feature's. A run reviewing a report reads "Reviewing the
bug report".

**FR-5.4 — Review health.** The spec reviewer's reviews of reports count in
its numbers, and its line names both purposes: reviewing the specification,
and reviewing the bug report.

**FR-5.5 — Milestones** (SD-13). "Add to a milestone…" appears on an accepted
bug's page. The milestone edit modal's candidates include accepted bugs. MCP's
`add_milestone_member` and `remove_milestone_member` take `member_type:
"bug"` with a `BUG-` ID; `member_type: "feature"` with a `BUG-` ID works too.
Either way the bug is stored as a `feature` member, because that is what its
row is, and `get_milestone` lists it with its `BUG-` ID. Adding a bug that
isn't accepted is refused: "That bug hasn't been accepted in triage, so it
isn't committed work yet. Accept it first." A bug in a milestone counts as
one item and carries its tokens in the bar; shipping records it as shipped or
not shipped, as it does a feature.

**Acceptance:** the bug's page renders its triage card waiting and decided,
with the bug icon; the initiative and feature pages list open bugs; the
initiative's feature list leaves bugs out; the timeline of the end-to-end run
shows the reported, accepted, sent, report-approved, plan-approved and done
moments; review health names the bug report review; a milestone takes an
accepted bug and refuses a reported one, counting it as one item, and a
milestone holding an initiative doesn't pick up that initiative's bugs.

### FR-6: MCP

**FR-6.1 — `report_bug`** (authoring, SD-11). Arguments: `on` (an initiative
or feature, by path or ID, or a bug by ID), `title`, optional `summary`,
`steps`, `expected`, `actual`, optional `notes`. It creates the bug reported
by the chat agent, and returns its ID, page, report path and "It waits in the
triage queue for a person to accept or reject it." Its description says the
chat agent may report on a person's behalf or its own, and that whether it is
fixed is the person's decision.

**FR-6.2 — `list_bugs` and `get_bug`** (reads). `list_bugs` takes optional
`triage` (a state, or `open`) and `on` (an initiative or feature); it returns
each bug's ID, title, triage state, lifecycle state, origin and page.
`get_bug` takes a `BUG-` ID and returns that, plus who reported it and how,
the triage decision with its channel, reason and quote, the report's ID, path
and state, and, for a duplicate, the original.

**FR-6.3 — `relay_triage`** (a relay, SD-11). Arguments: `bug`, `decision`
(`accept`, `reject` or `duplicate`), `reason` (required to reject),
`duplicate_of` (required for a duplicate), and `quote`. It calls
`DecideTriage` with `via: mcp` and the quote, and the audit row carries both,
with `verdict_by: person`. A missing or empty quote is refused before
anything is looked up. Its description says it carries a person's decision,
must quote them, must never be used on the agent's own judgement, and that
accepting starts no work.

**FR-6.4 — The boundary.** No MCP tool sends a bug, starts building it,
reopens a triage decision, or decides one without a quote. The existing
feature tools take a bug's ID: reads, `update_feature` (a bug's title and
description are planning, as a feature's are) and attaching documents, within
FR-4.0's ownership rule. `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
names the four new tools, each with a comment saying why it is allowed, and
its must-not-exist list adds `accept_bug`, `reject_bug`, `triage_bug` and
`send_bug`. The `initialize` instructions gain a sentence: the agent may
report bugs and read them, and carry a person's decision to accept or reject
one, quoting them; whether a bug is fixed is theirs.

**Acceptance:** over `POST /mcp`, each tool works on its happy path; a relay
leaves the `bug.triaged` audit row with `via: mcp` and the quote; a relay with
no quote and a rejection with no reason fail with a sentence; a relayed
decision shows on the bug's page as the person's, with their words; the
advertised set is exact.

### FR-7: The starter pack

**FR-7.1** The pack adds `templates/bug_report/` (FR-1.5).

**FR-7.2** `roles/implementer.yaml`, `roles/code-reviewer.yaml` and
`roles/verifier.yaml` add `report_bug` to their tools, with a line in their
tool hints. The config loader refuses `report_bug` on a role bound to a
purpose that isn't offered worktree tools, or named as a document type's
reviewer (SD-10).

**FR-7.3** The `implement-task`, `review-code` and `verify-feature` skills
gain a short section: what is out of scope for your task, report it with
`report_bug`, and never fix it in passing. `review-spec` gains the section
for reviewing a bug report (SD-8), `write-spec` the section for revising one
(SD-7), and `verify-feature` FR-4.6's section on verifying a bug.

**FR-7.4** A project made before M12 has none of this until it copies the
files from a fresh `init`. Its agents simply have no `report_bug` tool, a
report can't be created (the template is missing, and the refusal says where
to find it), and nothing else changes. The handoff says how to upgrade.

**Acceptance:** a fresh `init` has the template and the three roles with the
tool; the config loader accepts `report_bug` on the code reviewer and refuses
it on the spec reviewer; a report in a project without the template is
refused with a sentence.

## 4. Non-functional requirements

- **NFR-1 — One service layer.** The UI handlers, the MCP tools and the agent
  tool call the same service methods (`ReportBug`, `DecideTriage`), each in
  one transaction with its audit rows (O-3).
- **NFR-2 — The orchestrator stays code.** Nothing an agent reports is
  dispatched until a person accepts it and sends it. Filing a report
  dispatches nothing.
- **NFR-3 — The seam holds.** Only the web UI sends a bug. No agent tool and no
  MCP tool changes a triage decision without a person's quoted words, and a
  database check refuses a relayed decision without one.
- **NFR-4 — No second pipeline.** A test asserts the end-to-end run's
  dispatches are exactly the feature pipeline's purposes, less `write-spec`,
  with `review-bug_report` where a feature has `review-spec`.
- **NFR-5 — Human prose** in every label, refusal, notice and tool
  description (D-6). British spelling.
- **NFR-6 — Contained templates.** New markup lives in `bug.html` (the queue,
  the triage card, the report dialog, the bugs list, the counts).
  `entity.html`, `inbox.html`, `partials.html`, `review.html` and `icons.html`
  gain only includes, the menu item, the inbox line, the nav item, the held
  panel's wording and the bug icon.
- **NFR-7 — Coordination.** Migration `0013` only. In M11's files, only
  SD-16's changes, to `planner.go`. Outside `internal/server`, `internal/store`
  and `internal/rules`, the changes are small and named: the tool's
  definition and the run on the tool context (`internal/dispatch`,
  `internal/toolhost`), `KnownTools` and the loader's check
  (`internal/config`), the rule kind and the fence rule
  (`internal/lifecycle`), the registry (`internal/ident`) and the timeline's
  rules (`internal/timeline`). The MCP tool-set test's list is merged by hand
  with M11's.
- **NFR-8 — No typed paths.** Every new form carries row ids in hidden fields;
  the only typed value is a duplicate's original, by its ID.
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
8. **Nineteen choices need Sam's explicit yes:**
   1. a bug is a feature row with a kind, and its own facts sit in a side
      table (SD-1);
   2. triage is a column, and rejecting or marking a duplicate abandons the
      row; triage is final (SD-2);
   3. a bug always has an initiative, and optionally an origin feature (SD-3);
   4. the report's ID is `BUG-007-bug-report`, not §7's `BUG-007-report`
      (SD-4);
   5. the built-in criterion is enforced by a new `contains_text` rule, as a
      list item that starts with it (SD-5);
   6. a bug's documents are reviewed only once it is accepted, and Send
      submits its report (SD-6);
   7. a sent-back report goes to the spec author, bounded by the round cap
      (SD-7);
   8. the review differs by skill, not by prompt (SD-8);
   9. **minor code-review findings: one bug per feature, filed when it
      merges; no de-duplication; spec and plan review minors stay on their
      threads** (SD-9; the alternatives are one per approving review, or one
      per finding);
   10. agents file with a tool, at most three per run, refusing a duplicate
       title; the loader refuses the tool where it can't be offered (SD-10);
   11. chat reports without a quote and relays triage with one (SD-11);
   12. the queue is its own page with its own count, and the Inbox shows a
       line; the Inbox badge counts checkpoints only (SD-12);
   13. only accepted bugs join milestones, and an initiative doesn't bring in
       its bugs (SD-13);
   14. bugs block archiving their initiative until finished (SD-14);
   15. a bug reported on a feature being built waits for it to merge before
       it can be sent (SD-17);
   16. a relayed rejection or duplicate is final like any other, which departs
       from DEC-006 Amendment 1's "recoverable" test (SD-11; the alternatives
       are a web-UI *Reopen triage*, or relaying accept only);
   17. *duplicate* as a third triage outcome beside §9's accept and reject
       (FR-2.1);
   18. validation ignores `{{…}}` and TODO inside fenced code, for every
       document type (FR-1.4);
   19. Home and the Work list show features, and bugs appear in the triage
       queue and on their initiative's and feature's pages (FR-1.6).
9. **On approval**, dated notes record:
   - in DEC-006 Amendment 1: `relay_triage` as the relay DESIGN-010 §17a item
     6 allowed, and choice 16;
   - in DESIGN-010 §9: how the bug was built; §6: SD-13's reading of
     "everything under them"; §7: SD-4's report ID;
   - in DEC-004 Amendment 1's note on G4: a done bug is a done item;
   - in DESIGN-004: the `contains_text` rule kind, and the fence rule.

## 6. Open questions carried forward

- **Reopening a triage decision.** A rejected bug that turns out to matter is
  reported again today. An *Undo* within some window may be wanted.
- **Project-level bugs.** A bug found in "the whole thing" hangs off the
  initiative closest to it. If that's often awkward, the feature row's
  initiative would need to become optional, which is a larger change.
- **Turning off minor findings as bugs.** If the queue fills with them, a
  project setting, or filing them already rejected as "noted", may be wanted.
- **A spec reviewer reviewing a report has no design to hold it against.**
  It judges clarity and testability only. Whether the plan reviewer should see
  the origin feature's spec is a question for the live runs.
- **Existing projects need new files** (FR-7.4). A `subutai upgrade` that
  copies missing starter files would help every milestone, not just this one.
- **Bugs in the rail, on Home and in the Work list.** They show features only
  (choice 19). If people want bugs there too, it is a presentation change.
- **The cap of three reports per run is a guess.** A per-role setting may be
  wanted once the live runs show how agents use the tool.
- **A bug reported by a person on a feature being built** may be about code
  already on the main line; SD-17 holds it back anyway. A person who knows
  better can reject it and report it on the initiative instead; a "this is on
  the main line" override may be wanted.

## 7. Changes after review

How each [REVIEW-019](../reviews/REVIEW-019-bugs.md) finding was dealt with.

| Finding | What changed |
|---|---|
| R19-1 (material) | FR-4.0 states the rule; Appendix A lists every site and what it does now; SD-1's cost statement and SD-16 and NFR-7 are corrected; the acceptance lists a test for each part (Start building, issues, the hold, agent review off, the successor, the revision in place). |
| R19-2 (material) | Both remedies: minor findings are filed per feature when it merges (SD-9, FR-3.4, choice 9), and any bug whose origin is being built waits for the merge (SD-17, FR-4.1, choice 15). |
| R19-3 (material) | FR-1.4: a Summary (the reporter's, or the title as a sentence), Steps as a numbered list, Actual and Notes fenced when they hold structure or braces, one-pass filling, empty optional sections left out, and validation before anything is minted. FR-1.4 and choice 18: fenced code isn't a placeholder. |
| R19-4 (material) | FR-4.4 puts the triage check in the submission service, for every document a bug owns. FR-2.1 refuses Abandon on a reported bug, in the store, for every path. FR-4.0 adds the ownership rule. FR-2.3 lists only reported bugs still an `idea`. |
| R19-5 (material) | SD-11 states the departure, and choice 16 puts it to Sam with the alternatives. |
| R19-6 | FR-1.3: feature slugs like `bug-009` are refused; every refusal runs before minting; gaps are accepted. |
| R19-7 | FR-1.3 names `ref_type = 'feature'`, keeps `feature.created` (hidden on a bug's timeline) and `defect` unused; FR-5.3 folds triage's abandonment and the report's writing into their moments; FR-5.1 has the bug icon. |
| R19-8 | FR-5.5 says "bug" is stored as a `feature` member and what `get_milestone` shows; SD-13 gives the right reason, and DoD 9 adds the notes on §6 and G4. |
| R19-9 | FR-1.2 and FR-6.4: bug IDs resolve everywhere a feature's do, and say which tools take a bug. |
| R19-10 | NFR-7 lists every file touched; SD-10 and FR-7.2 add the loader's refusal, the title rule and the cap per dispatch row. |
| R19-11 | FR-3.4 files after the merge commits, best effort, with the audit row as the record; `ApproveTaskCode` carries the review's run. |
| R19-12 | FR-4.3 gives the order and the failure case; FR-4.7 extends Withdraw; FR-4.1 and the acceptance define the send screen for a bug; SD-8 says how routing applies. |
| R19-13 | SD-16 adds the prompt heading, the bug-report note in place of the designs, and the rule's description, all in `planAuthor`. |
| R19-14 | SD-5 defines the rule as a list item that starts with the text, and DoD 9 adds the DESIGN-004 note. |
| R19-15 | FR-1.6 decides every read the review listed, and FR-5.2 defines *open*. |
| R19-16 | SD-4 now rests on the enum, and DoD 9 adds the §7 note. |
| R19-17 | The DEC-004 citation is corrected; DoD 9 lists every note; choice 17 is *duplicate*; FR-2.5 says where the decision is made. |
| R19-18 | SD-12 answers the alternative; FR-2.4 says reporting and triage fire the change signal. |
| R19-19 | FR-2.3 and FR-2.4 give full sentences; the note on prose says what "Reported by Sam" relies on. |
| R19-20 | FR-1.1 states the single writers, and a test covers the pairing. |

## Appendix A: where "the spec" is read, and what it reads now

Each site asks `IsSpecType` or `IsContractType`, or reads the row's spec type.
None stays spec-only by design.

| Site | What it does | For a bug |
|---|---|---|
| `rules.go`, document transitions | revision in flight; send-back to the author; plan invariant and G1 on approval | fires for `bug_report` |
| `rules_phase2.go`, `write-spec` outcome | files the authored document | files it as the report (`FeatureSnap.SpecType`) |
| `authoring.go`, `neededAuthoring` | invariant 1 | reads the report; owes nothing without one |
| `actions.go`, `evaluateContractGate` | G1 | reads the report |
| `actions.go`, `queueReview` | agent review off holds the spec | holds the report |
| `actions.go`, `approveDocumentAs` | a person's send-back is an issue | likewise |
| `review_send.go`, `specHeld` | the hold | applies to the report |
| `review_send.go`, `supersedePlanWithSpec` | a successor spec supersedes the plan | likewise for a successor report |
| `review_send.go`, `RaiseIssue` | issues on specs and plans | and on reports |
| `review_send.go`, `refuseIfAuthorAtWork`, `afterSubmission`, `docTypeWords`, `authorPurpose` | guards and wording | cover reports |
| `review_send.go`, `sendReadiness`, `SendToDevelopment`, `withdrawable`, `WithdrawSend` | the send | FR-4.1, FR-4.3, FR-4.7 |
| `worktree_ops.go`, `StartFeature`; `ui_entity.go`, start card (`currentDocApproved`) | Start building needs an approved spec | reads the report |
| `planner.go`, `contractBody`, `planAuthor` | what agents are given; what the author writes | SD-16 |
| `ui_send.go`, `sendFeatureView`, `docActionsFor` | the send screen; the document page's review acts | FR-4.1, FR-4.5 |
| `edit.go`, `authorDueRefusal` and its notes | the editor stands aside for the author | covers reports |
| `identity.go`, adopt's one-live-spec rule; `registerInTx` | one contract document per type | covers reports; FR-4.0's ownership rule |
| `documents.go`, `submitDocWith` | submission | FR-4.4's triage check |
| `timeline.go`, `docNoun`, `plannedDocs` | moments | "Report …" |
| `observe.go`, `runPurpose` | run wording | "Reviewing the bug report" |
| `ui.go`, `docIcon` | the document icon | the spec's icon |
