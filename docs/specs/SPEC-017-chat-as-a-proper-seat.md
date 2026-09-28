# SPEC-017: Chat as a proper seat

**Status:** **Approved — Sam, 2026-09-28**, with the build, and with the
recommendation accepted on each of its eleven choices (DoD 8). Choice 1's
dated notes are in DEC-006 Amendment 1 and DESIGN-010 §5c. It was drafted for
Sam's approval as follows. Authored by Claude. The author can't be the
approval gate, so the decision is Sam's. An independent review is recorded in
[REVIEW-017](../reviews/REVIEW-017-chat-as-a-proper-seat.md). It found five
material and nine smaller problems in the first draft. All are dealt with in
this revision, and §7 says how, finding by finding. Eleven choices need Sam's
explicit yes: the scope decisions in §3, listed in DoD 8.
**Date:** 2026-09-28
**Roadmap milestone:** M10 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11: "a feature whose spec was written in chat skips spec writing and flows to
*Start building*, and every document shows who wrote it."
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28): §5 (Send, "How specs are reviewed"), **§5a** (working ahead in
chat), **§5c** (what the chat agent may do), and §8 ("Everything is
attributed").
**Authority:**
- [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md): the chat agent may
  author the planning layer, including document content, and may "query
  state";
- [DEC-005](../decisions/DEC-005-the-orchestration-boundary.md): the chat agent
  may "ask the orchestrator to do something the orchestrator would do anyway —
  submit a document for review, request a re-review";
- [DEC-006](../decisions/DEC-006-humans-start-development.md) **Amendment 1**,
  decision 8: the relay list, and "adding a relay tool that isn't on this list
  needs a decision";
- [DEC-007](../decisions/DEC-007-the-judgement-boundary.md) decision 1: every
  piece of work records its executor, shown on document pages and in the audit
  trail.

**Builds on:** [SPEC-011](SPEC-011-send-to-development.md) (the relay tools,
the send screen's "already done", direct human approval),
[SPEC-012](SPEC-012-see-the-work.md) (transcripts and the timeline) and
[SPEC-015](SPEC-015-documents-with-identity.md) (IDs; its SD-19 left work for
after M5).
**Follow-ups taken up:**
- from the [M6 handoff](../notes/handoff-M6-2026-09-28.md): follow-up 1
  (`get_timeline`, `get_agent_run`), follow-up 2 (the review behind a
  verdict), and the "written by" half of follow-up 4;
- from the [M8 handoff](../notes/handoff-M8-2026-09-28.md): follow-up 1
  (SD-19).

**Coordination:** M9 (the browser editor) runs in parallel.
- This spec owns migration `0011`. M9 uses `0012` if it needs one.
- **M10 merges first.**
- The document page is shared: this spec adds one include to it.
- M9 doesn't touch the MCP files.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6).

Three words are kept apart throughout:

- **Writer** is whoever wrote a document: a dispatched agent, the chat agent, a
  person, or Subutai itself. DEC-007 calls this the document's *executor*.
  `document_writers` is DEC-007's "executor field" for documents, and M13
  builds on it for tasks rather than adding a second record.
- **Verdict** is a decision on a document in review: approve, or send back.
  Only a reviewer agent or a person gives one (DEC-007 decision 2).
- **Relay** is the chat agent carrying a person's decision, with their words
  quoted (DEC-006 Amendment 1 decision 8).

## 0. Framing

M3 built most of this milestone.
- The chat agent can relay a person's verdict, issue, review request, release
  of a held spec and ticked job.
- The send screen says which steps are already done.
- A person can approve a spec or plan directly.
- The machinery for working ahead already holds. A spec written in chat and
  approved before Send satisfies the spec invariant, so Send starts at the
  plan (SPEC-011 SD-2).

What is left is smaller, and it is what makes the seat *proper*:

- The chat agent can't hand in the work it did without a person's quoted
  words. A spec it has written can only reach review through
  `relay_review_request`, which is a relay.
- Nothing records who wrote a document, or who gave a verdict, in a form a
  page can show. DESIGN-010 §8 and DEC-007 decision 1 both ask for it. The
  audit trail holds pieces of it, in separate transactions, and without the
  review behind a person's ruling.
- M8 left IDs off the relay tools and the milestone, roadmap and checklist
  results.
- M6 proposed two read-only tools, so the chat agent can answer "how is it
  going?" and "why did the reviewer say that?".
- Nothing proves the §5a path end to end.

## 1. Goal

**One claim, which the definition of done checks directly:**

> The chat agent writes a spec, adds it, and submits it without quoting
> anyone. The agent reviewer approves it. A person presses Send, and the send
> screen says spec writing and review are already done. The plan is written
> and reviewed, the work is estimated, and it stops at Start building. The
> spec's page says it was written by the chat agent and approved by the spec
> reviewer.

Two supporting claims:

> Every document registered from now on records its writer. Every verdict
> records who gave it and how: an agent's review run, a person in the web UI,
> a person answering a reviewer's escalation, or a person relayed from chat
> with their words.

> The chat agent still holds no verdict of its own, and can't send, start
> building, override a gate or answer a checkpoint.

## 2. The audit: what M10 still needed

Checked against the roadmap's M10, DESIGN-010 §5a, §5c and §8, and the code
on `claude/lucid-cerf-ft8jbj` at `2f40650`.

| Asked for | Where | What exists | What is missing |
|---|---|---|---|
| Chat tools to submit a document | Roadmap M10; DEC-005; §5a | `relay_review_request` submits a draft, but needs a person's quote. | A way to submit without a quote (FR-1). |
| Chat tools to ask for an early review | Roadmap M10; §5a | Submitting a spec or plan queues its review, for a sent or unsent feature alike: the rules queue a review on any submission, and `queueReview` doesn't read the sent mark. `relay_review_request` asks for a fresh review, with a quote. | Nothing beyond FR-1: submitting *is* asking for the early review. Whether a fresh review should still need a quote is SD-2. |
| A human verdict on any document type | Roadmap M10 | A person can approve or send back any document in review, in the UI or by relay (SPEC-011 FR-5.5, FR-8). A draft of a type with no template (decision, note, research, report, policy) can't be submitted. A person can still record it as approved in the web UI (*This was already approved*, SPEC-015 FR-5.8). | Relaying that from chat (the M8 handoff's follow-up 5). SD-8. |
| The send screen shows steps already done | Roadmap M10; §5 | Built in M3 (SPEC-011 FR-4.3): "already written (approved)", "already approved". | It doesn't say who wrote the document or that the step is skipped. A spec left as an unsubmitted draft reads as "already written" and then waits forever (FR-5). |
| Spec written in chat skips spec writing | §5 step 1; §5a | Works: a current spec that doesn't wait for its author satisfies the invariant (SPEC-011 FR-2.1). | No test proves the whole path (FR-6). |
| Every artifact records who produced it | §8; DEC-007 decision 1 | Audit rows name an actor: a role name, the MCP actor or a person. Relays add `via` and the quote, on a row separate from the transition. The timeline tells agent, chat and person apart. | There is no record a page can show. An agent's approval doesn't record the review run behind it. A person's answer to an escalation doesn't record the review it ruled on (M6 follow-up 2). The timeline counts a relayed verdict as the chat agent's own (FR-2.7). A project that gives the chat agent and the web UI the same actor name can't tell them apart at all (SD-12). |
| Relay tools accept IDs | M8 follow-up 1 (SD-19) | `path` only. | FR-3.1. |
| `MS-`, `RM-`, `CL-` IDs in MCP results | M8 follow-up 1 | Results give the row id as `id`. Lookups take `MS-` and `RM-`, but not `CL-`. `store.Checklist` doesn't read its `public_id`, and there is no lookup by it. | FR-3.2. |
| IDs on checklist pages and `plan.html` lists | M8 follow-up 1 | Milestone and roadmap pages show their IDs. Checklists and the plan lists don't. | FR-3.3. |
| `/ui/id/CL-…` | M8 follow-up 1 | Answers "not found". | FR-3.4. |
| `get_timeline`, `get_agent_run` | M6 follow-up 1 | Neither exists. | FR-4. |
| Executors, claims | §5b; DEC-007 | Nothing. | **M13, out of scope.** |
| Per-user identity | §13 | Relays are recorded against the configured MCP actor. | **M15, out of scope.** |

## 3. Scope

### In scope

1. Submitting for review from chat, without a quote (FR-1).
2. Writers and verdicts: recorded, inferred for existing documents, and shown
   (FR-2).
3. The M8 follow-ups: IDs in and out of the MCP tools, on pages, and by URL
   (FR-3).
4. `get_timeline` and `get_agent_run` (FR-4).
5. The send screen names the writer and the skip, and warns about an
   unsubmitted draft (FR-5).
6. The end-to-end proof with the mock provider (FR-6).
7. The chat agent's instructions (FR-7).

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| Executors, claiming and submitting tasks | M13 (DEC-007) |
| A stronger reviewer when the chat agent did the work (DEC-007 decision 10) | M13, which can read `document_writers` |
| Per-user identity: naming the person behind a relay | M15 |
| The browser editor | M9, in parallel |
| Relaying "already approved" from chat, for documents with no template | M11, for all five types (SD-8) |
| Crediting edits made after registration | §6. Subutai sees a changed file, not who changed it. |
| Linking each review comment to its run (the other half of M6 follow-up 4) | The spare-hours track (§6) |

### Scope decisions

- **SD-1 — Submitting is a planning act, and needs no quote.** A new tool,
  `submit_for_review`, submits a draft for review as the chat agent. The name
  keeps clear of `submit_document`, the outcome tool dispatched authors call.
  - **Why it needs no quote.**
    - DEC-005 names "submit a document for review" as asking the
      orchestrator to do what it would do anyway. The rules engine decides
      whether a review runs, which role and which model.
    - DESIGN-010 §5c says the chat agent "may claim and submit work", outside
      its relay list. Writing a spec in chat is doing work (§5a, §5b).
    - DEC-007's claim-and-submit machinery for tasks is M13. This is the
      document half, which needs no claim.
  - **What a submission can start.** DEC-006 Amendment 1 draws the line by
    consequence, so here is the whole list:
    1. **A spec on an unsent feature:** one spec review, about 2,900 tokens.
       The reviewer's approval starts nothing more, because the plan needs
       the sent mark.
    2. **A spec on a sent feature:** the review. If the *reviewer* approves
       it, plan writing follows (about 8,000 tokens), as the pipeline always
       does after a spec is approved.
    3. **A plan on an unsent feature whose spec is approved:** the review. On
       the reviewer's approval the plan is decomposed into tasks, and the
       feature becomes `ready`, as SPEC-011 SD-2 already accepts. Building
       still needs Start building.
    4. **A revision on a feature being built:** this would raise a
       `revision-in-flight` question and pause new task runs. **The tool
       refuses this case** (FR-1.2). A person submits such a revision on its
       page.

    In every case the verdict is the reviewer's, and the reviewer can send
    the draft back. Nothing sends, builds or overrides a gate.
  - **What it changes.** DESIGN-010 §5c and DEC-006 Amendment 1 decision 8 put
    "a request for a review" on the relay list, which needs a quote. For a
    draft, submitting and requesting a review are one act (SPEC-011 FR-5.5:
    "draft, where it means Submit"). So this spec reads the relay list as
    covering the *fresh* review of a document already in review (SD-2), and
    reads submission as DEC-005 planning. **That changes what the chat agent
    may do, so it is Sam's choice** (DoD 8.1). If Sam agrees, the handoff
    proposes dated notes for DEC-006 Amendment 1 and DESIGN-010 §5c, as
    earlier milestones did.
  - **The alternative** is to keep the quote. Then `submit_for_review` isn't
    built, and `relay_review_request` stays the only route. The end-to-end
    proof would quote a person at step 3.

- **SD-2 — A fresh review of a document already in review keeps its quote.**
  `relay_review_request` stays as it is for a document in `reviewing`. A
  fresh review there either duplicates one the orchestrator already queued,
  or overrides the project's choice to switch agent review off. Both are a
  person's call about spending, and the tool is on Amendment 1's list. On a
  draft it still works, with its quote, as before. Its description now points
  to `submit_for_review` for that case (DoD 8.2).

- **SD-3 — A writer is recorded when Subutai sees the writing.** A new table
  holds one row per writing act Subutai knows about:
  - an author agent writing a document, or revising one;
  - a document added by attach or adopt;
  - a starter design;
  - a revision opened by Revise, by an issue on an approved document, or by
    the cascade.

  Subutai doesn't see edits made to a file after that. The page says who
  *added* or *wrote* the document, not who last touched it (§6).

- **SD-4 — What the chat agent adds is credited as written by it.** When the
  chat agent attaches or adopts a file, the page says "Written by the chat
  agent". Its own tools tell it to write a file and then add it, so this is
  usually true. When a person adds a file in the web UI or the CLI, the page
  says "Added by *sam*", because people adopt files written long before
  Subutai knew them.
  - The asymmetry is deliberate, and it is Sam's choice (DoD 8.3).
  - It is wrong in one case: a person asks the chat agent to "attach my
    notes", and the notes then read as the chat agent's.
  - The alternative is "Added by" for everyone. That would make the roadmap's
    test ("the spec shows written by the chat agent") false.

- **SD-5 — A relayed verdict is a person's, carried by the chat agent.** It is
  recorded as a person's verdict, `via: mcp`, with the quote. The MCP actor is
  recorded as the relayer. Until M15 there is no name for the person, so the
  page says "Approved by a person, relayed by the chat agent: “…”". The
  timeline stops counting the chat agent as the one who approved (FR-2.7).
  This follows DEC-006 Amendment 1's "carrying a judgement isn't judging"
  (DoD 8.4).

- **SD-6 — A verdict records the review run behind it.**
  - An agent's verdict records its run, role and model.
  - A person's verdict records how it came: the web UI, relayed from chat, or
    as the answer to a reviewer's escalation, with the review it ruled on.
  - An escalation can be answered in the Inbox, on the document page, or
    through the API, and the answer doesn't say which. So the sentence says
    "answering the reviewer's escalation" and names no surface.
  - A held agent approval is recorded too, marked held.
  - *Let the reviewer decide* records the reviewer's approval again, with the
    person who released it (DoD 8.5).
  - The transition's audit row carries the same facts (`verdict_by`, `via`,
    `dispatch_id`), so the audit trail stays the source of truth.

  This closes M6 follow-up 2.

- **SD-7 — Existing documents are inferred from the audit trail, or marked
  "not recorded".** Every inferred row is marked *inferred*, and the page adds
  "(from the audit trail)". There are two passes.
  - **Migration `0011`**, in SQL, records what the trail pins down:
    - an author agent, when the registering role matches an authoring run on
      the feature that finished just before;
    - Subutai's own revisions;
    - an agent's verdict, with its review run;
    - a held approval (`document.held` names the run);
    - who released a hold;
    - a person's verdict, when a `document.human_verdict` or an escalation
      answer came just before it;
    - approvals by adoption.
  - **The server, when it starts** (`BackfillProvenance`), does the rest for
    documents older than `0011`. Telling the chat agent from a person needs
    the configured actor names, which SQL can't read.
    - The MCP actor is the chat agent.
    - Subutai's own names are Subutai.
    - Any other actor that isn't a role is a person. That includes the CLI's
      `$USER`, so documents added with `doc add` are credited too.
    - A role's act that can't be tied to a run is left unrecorded rather
      than guessed.

  Before `0011`, an issue raised on a sent spec in review left exactly the
  same record as a person's send-back, so both read as a send-back
  (DoD 8.6).

- **SD-8 — A person's verdict on an untemplated document stays in the web UI
  for now.**
  - A decision, note, research, report or policy has no template, so it
    can't be submitted for review.
  - A person can still record a draft of one as approved in the web UI
    (*This was already approved*, SPEC-015 FR-5.8). FR-2.3 records that as
    their verdict. So the roadmap's "a human verdict on any document type"
    holds in the web UI.
  - What is missing is carrying that from chat (the M8 handoff's follow-up
    5). That would be a new relay, and it needs a line in DEC-006 Amendment 1.
  - This spec doesn't build it. It names **M11** as the destination for all
    five types: M11 decides what reviewing a decision means, and the same
    answer serves the others (DoD 8.7).

- **SD-9 — MCP results name things by ID.** This follows SPEC-015 FR-7.4 for
  documents.
  - A milestone's, roadmap's or checklist's `id` becomes its ID (`MS-004`),
    and its row id moves to `row_id`.
  - Every lookup still accepts row ids, IDs and exact names, so a chat agent
    that saved an old result still works.
  - A job has no ID, and keeps its row id as `id` (DoD 8.8).

- **SD-10 — The read tools return compact answers.**
  - `get_agent_run` returns the conclusion in full, and whether the run's
    verdict was applied.
  - Its transcript is cut to size: the prompt and each message to 2,000
    characters, and at most 60 turns, keeping the first and the last.
  - Tool results are left out unless the chat agent asks for them, and are
    then cut to 500 characters each.
  - Every cut says so, and gives the run's page address, where the whole
    thing is.
  - A transcript can show what a tool read, including secrets (M6 choice 8).
    This tool sends it to the chat agent's provider, a new place for it to go.
    Leaving tool results out by default keeps most of that back (DoD 8.9).

- **SD-11 — An ID in a relay names the revision in front of the person.** For
  the relays and `submit_for_review`, an ID without a revision names its
  newest revision that isn't superseded. That is the one in review or draft
  while a revision is open, and otherwise the one in force. A page link, by
  contrast, names the one in force (SPEC-015 SD-18). `FEAT-003-spec.r2` names
  a revision exactly (DoD 8.10).

- **SD-12 — The chat agent and the web UI must have different actor names.**
  The record tells the chat agent from a person by these names. With one name
  for both, every person's act would read as the chat agent's, and every
  relay as a person's own. The configuration refuses it, with a sentence, as
  the strict parser refuses any setting it can't honour (DESIGN-004 F-1)
  (DoD 8.11).

## 4. Requirements

### FR-1: Submitting for review from chat

**FR-1.1** A new MCP tool, `submit_for_review`, sits in
`internal/server/mcp_relay.go` beside the relays. It has one argument,
`document`: the document's path or its ID (SD-11). It runs the same submission
as the document page's Submit, as the MCP actor. The submission's own audit
row carries `via: mcp` and `submitted_from_chat`, in the same transaction
(NFR-1). It takes no quote.

**FR-1.2** It refuses each of these with a sentence:
- a document in review: "This document is already in review. A fresh review
  is a person's call: relay it with `relay_review_request` and their words."
- an approved document: "This document is approved. To change it, a person
  revises it on its page, or raises an issue on it, which you can relay."
- a superseded revision named exactly;
- a revision of a feature that is being built (SD-1, case 4);
- a document whose type has no template (SD-8);
- a draft whose author agent is revising it (SPEC-011 FR-3.6);
- a document with a question about it waiting in the Inbox (SPEC-011 SD-11);
- a draft that fails validation. The refusal lists each problem, so the chat
  agent can fix the file and try again.

**FR-1.3** Its result is the document entry (FR-2.6), and a sentence saying
what happens next, read from the project's settings:
- a spec or plan with an agent reviewer: "Its review is queued. The spec
  reviewer (*model*) decides whether the specification is approved; you
  don't."
- a spec the project or its send holds for a person: "…reviews the
  specification, and then it is held for a person, who approves it or lets
  the reviewer decide. Neither verdict is yours."
- a spec with agent review switched off: "Agent review is switched off for
  this project, so the specification waits for a person to approve it."
- a plan: the reviewer decides, and "once it is approved, it is broken into
  tasks, and with an approved specification the feature is ready to build.
  Building starts only when a person presses Start building." (SD-1, case 3)
- a design: "It is in review, and waits for a person to approve it, on its
  page or by telling you."

**FR-1.4** Its description says three things:
- it hands the chat agent's own work to the reviewer;
- the reviewer's verdict isn't the chat agent's to give;
- it doesn't send the feature to development.

**FR-1.5** `relay_review_request`'s description says to use
`submit_for_review` for a draft the chat agent wrote. The relay is for a
person asking for a fresh review of a document already in review (SD-2).

**Acceptance:**
- over `POST /mcp`, `submit_for_review` moves a chat-written spec draft to
  `reviewing` and queues `review-spec`, with no quote;
- the transition's audit row has `via: mcp`;
- each refusal in FR-1.2 is a sentence;
- `relay_review_request` without a quote is still refused.

### FR-2: Writers and verdicts

**FR-2.1 — Migration `0011`** adds two tables. Both cascade on the document's
deletion, so Detach removes them with the rest of the registration.

`document_writers`: one row per writing act (SD-3).

| Column | Meaning |
|---|---|
| `id` | row id |
| `document_id` | the document |
| `act` | `wrote`, `revised`, `added`, `started` or `opened_revision` |
| `writer_kind` | `agent`, `chat`, `person` or `system` |
| `actor` | the role, the MCP actor, the person, or `subutai` |
| `model` | the model, for an agent |
| `dispatch_id` | the run, for an agent |
| `via` | `agent`, `mcp`, `ui` or `api` |
| `inferred` | true for a row inferred from the audit trail (SD-7) |
| `at` | when |

`document_verdicts`: one row per verdict (SD-6).

| Column | Meaning |
|---|---|
| `id` | row id |
| `document_id` | the document |
| `verdict` | `approve` or `send_back` |
| `held` | true for an agent approval held for a person |
| `giver_kind` | `agent` or `person` |
| `actor` | the role, or the person (for a relay, the MCP actor) |
| `model`, `dispatch_id` | the review run, for an agent; for a person's answer to an escalation, the review it ruled on |
| `via` | `ui`, `escalation`, `mcp`, `api`, or empty for an agent |
| `quote` | a relay's quoted words; the database refuses a relayed verdict without one |
| `released_by`, `released_via`, `released_quote` | who let the reviewer decide |
| `inferred` | as above |
| `at` | when |

**FR-2.2 — Where writers are recorded.** Each is recorded in the same
transaction as the registration it describes.

| Act | Recorded as |
|---|---|
| An author agent writes a new spec or plan | `wrote`, `agent`, with its role, model and run |
| An author agent fills an existing draft (a revision, or a chat draft sent back after Send) | `revised`, `agent`, as above, in the submission's transaction; `wrote` if the draft has no writer yet |
| Attach over MCP | `added`, `chat`, the MCP actor, `via: mcp` |
| Adopt over MCP, of a file not yet registered | `added`, `chat`, as above |
| Attach or adopt in the web UI | `added`, `person`, the UI actor, `via: ui` |
| `doc add` through the API | `added`, `person`, the request's actor, `via: api` |
| A starter design | `started`, by whoever created the entity (chat, person or API) |
| Revise, in the web UI or through the API | `opened_revision`, by the person, `via: ui` or `api` |
| An issue on an approved document, which opens a revision | `opened_revision`, by the person who raised it. Relayed, it is the person's act, `via: mcp`. |
| The design cascade | `opened_revision`, `system`, whoever answered the question that let it run |

To carry the run and model, the rules engine's `FileAuthoredDocument` action
gains the dispatch's id (`internal/rules`, one field). Nothing else in
`internal/rules` changes:
- The server's approve and send-back paths gain variants that take the
  verdict to record: `approveDocumentAs` and `returnForChangesAs`.
- The service methods that know the surface call those: the document page,
  the relays, and the release.
- What the rules engine hands over is an agent's verdict, or a person's answer
  to an escalation. The server completes it: the run's model from its
  dispatch, and the review an escalation answer ruled on from the answered
  question's context.

**FR-2.3 — Where verdicts are recorded.** They are recorded in the approve
and send-back paths every verdict already goes through:
- `approveDocument`: an agent's approval (with its run), a person's direct
  approval, an escalation answer, and a release (the reviewer's approval, with
  `released_*`);
- the held approval (`holdDocument`), with `held` set;
- `returnForChanges`: an agent's send-back, a person's on a design, and an
  escalation answered "ask for changes";
- a person's send-back of a spec or plan (SPEC-011 SD-15), which goes through
  `returnWithIssue`. An issue raised on a document in review also goes back
  through `returnWithIssue`, but it is an issue, not a verdict, and records
  none;
- approving on adoption, and *This was already approved* (SPEC-015 FR-5.3 and
  FR-5.8): a person's approval, `via: ui`.

A person's act records `via` from the surface:
- `ui` for the document page;
- `escalation` for an answer to a reviewer's escalation, with the review it
  ruled on;
- `mcp` for a relay, with its quote.

The transition's audit row carries `verdict_by`, `via` and `dispatch_id` too
(SD-6).

**FR-2.4 — How a writer and a verdict are said.** One function builds each
sentence, and the page, the MCP results and the send screen all use it.
- **Who:**
  - an agent is "the *role in words* (*model*)", such as "the spec author
    (deepseek-chat)", and links to its run;
  - the chat agent is "the chat agent";
  - a person is their name, and a person's act relayed from chat is "a
    person, relayed by the chat agent" (SD-5);
  - `system` is "Subutai".
- **The writer line** reads across the document's revisions. It gives the
  first act, the latest revision opened, and the last writer since who was
  someone else. For example: "Written by the chat agent. Revised by the spec
  author (deepseek-chat)." or "Written by the chat agent. Revision opened by
  sam."
  - `wrote`: "Written by …"
  - `added`: "Written by the chat agent" for `chat`, "Added by *sam*" for a
    person (SD-4)
  - `started`: "Started from the template by …"
  - `opened_revision`: "Revision opened by …"
  - none: "Who wrote this wasn't recorded."
  - An inferred row adds "(from the audit trail)".
- **The verdict line** is the latest verdict:
  - "Approved by the spec reviewer (deepseek-chat)."
  - "Approved by the spec reviewer (deepseek-chat), and held for a person."
  - "Approved by the spec reviewer (deepseek-chat); a person let the reviewer
    decide, relayed by the chat agent: “…”."
  - "Approved by sam." / "Approved by sam, answering the reviewer's
    escalation."
  - "Approved by a person, relayed by the chat agent: “…”." (SD-5)
  - "Sent back by …", in the same forms.
  - No verdict: no line.

**FR-2.5 — On the document page.** There is one include, `doc-written-by`,
directly under the page heading, in a new template file `provenance.html`. It
shows the writer line and the verdict line, each with its time. It links an
agent writer to its run, and a verdict to the review behind it.
`entity.html` gains only the include (M9 coordination).

**FR-2.6 — In MCP results.** Every document entry gains two fields: in
`get_feature`, `get_initiative`, `list_documents`, `attach_document` and
`adopt_document`, and in a create's starter design.
- `written_by`: `{ "sentence", "kind", "who", "act", "model", "run_id" }`, or
  `{ "sentence": "Who wrote this wasn't recorded." }`;
- `last_verdict`: `{ "sentence", "verdict", "kind", "who", "via", "quote",
  "run_id", "held" }`, when there is one.

`submit_for_review` and the relays used to return only a path and a state.
They now return the full document entry, with a `done` sentence.

**FR-2.7 — In the timeline.**
- A document's approval or send-back that was relayed is credited to "a
  person, relayed by the chat agent", not to the chat agent. The transition's
  audit row now carries `verdict_by: person` and `via: mcp`, and the timeline
  reads them.
- The timeline reads the chat agent's name from `mcpActor()`, the one place
  every other surface reads it. The configuration already defaults it, so
  this is a tidy-up, not a fix.
- The timeline still reads the audit trail, not the new tables. So a document
  a person added in the web UI is "Added by sam" on its page and "Spec
  written, by sam" in the timeline. And a verdict relayed before `0011` still
  reads there as the chat agent's own. Both are accepted: the timeline is
  about moments, and its words for them are shorter (§6).

**FR-2.8 — Backfill** (SD-7), in two passes. Both mark their rows inferred.
- **In migration `0011`**, for every document:
  - **writers:**
    - an author agent, when the first registration's actor is the role of a
      succeeded `write-spec` or `write-dev-plan` run on its feature that
      finished within ten minutes before;
    - Subutai, when a revision's actor is one of its own names.
  - **verdicts,** from each `document.transition` with event `approve` or
    `request_changes`:
    - an agent, when the actor is the role of a succeeded `review-<type>` run
      on the document that finished at or before it, with the release just
      before it, if any;
    - a person, when a `document.human_verdict` by the same actor, or an
      answer to the document's review escalation, came within ten minutes
      before.
  - **held approvals** from `document.held` with a run, and **approvals by
    adoption** from `document.human_verdict` with `already_approved`.
- **When the server starts**, for documents registered before `0011` with
  nothing recorded:
  - writers, from the first registration's actor;
  - a person's verdicts, from the transition's actor and its own `via` and
    `quote`.

  Both follow SD-7's rules.

**Acceptance:**
- an authored spec records its role, model and run, and its page links to the
  run;
- a spec attached over MCP says "Written by the chat agent", and one attached
  in the UI says "Added by" the UI actor;
- each of these records the right row and reads as FR-2.4 says: an agent
  approval, a held approval, a release, a direct approval, a relayed approval
  with its quote, an escalation answer, and a relayed send-back;
- an issue raised on a document in review records no verdict;
- a revision opened by a relayed issue credits the person, relayed;
- the backfill, run on rows written as they were before `0011`, infers each
  case it can, in both passes, and leaves the rest "not recorded";
- a project whose `mcp_actor` equals its `ui_actor` is refused;
- a relayed approval's timeline moment says "a person, relayed by the chat
  agent", and a chat-written spec's says "by the chat agent".

### FR-3: IDs, as M8 left them

**FR-3.1 — The relay tools accept IDs.**
- `relay_verdict`, `relay_issue`, `relay_review_request` and
  `relay_release_hold` accept a document's ID (`FEAT-003-spec`, `DEC-005`)
  wherever they take its path, as the `path` argument. The description says
  so.
- `relay_tick_job` accepts a `CL-` ID for its checklist.
- An ID names the revision SD-11 says.

**FR-3.2 — MCP results show IDs** (SD-9).
- Milestone, roadmap and checklist entries give `id` as the ID and `row_id`
  as the row id, everywhere they appear: in their own tools, a roadmap's
  milestones, a checklist's milestones, and a milestone's items.
- A feature or initiative item gains its ID beside its path.
- Checklist lookups accept `CL-` IDs, as milestone and roadmap lookups already
  accept theirs.

This needs store work: `store.Checklist` reads its `public_id`, and
`store.ChecklistByPublicID` finds a checklist by it.

**FR-3.3 — Pages show IDs.**
- The checklist page's heading and the checklist editor show its `CL-` ID, as
  the milestone and roadmap pages do.
- These show each item's ID before its name, with the same `ident` template
  the other lists use: `plan.html`'s lists of milestones, roadmaps and
  checklists, the roadmap spine, and a milestone's list of what it delivers.

**FR-3.4 — `/ui/id/CL-…`** redirects to the checklist's page.

**Acceptance:**
- each relay works with an ID in `path`, and `relay_tick_job` works with
  `CL-001`;
- `get_milestone`, `get_roadmap` and `get_checklist` return `MS-`, `RM-` and
  `CL-` IDs, and still accept row ids;
- the checklist page and the plan section render their IDs;
- `/ui/id/CL-001` redirects, and `/ui/id/CL-999` is not found.

### FR-4: Reading the work from chat

**FR-4.1 — `get_timeline`** has one argument, `feature` (its path or ID). It
returns the feature's journey, from the same `featureTimeline` the page uses:
- `line`: the one-line view's labels, with counts;
- `moments`, each with:
  - `label` and `at`;
  - `by` (`agent`, `person`, `chat` or `system`), and `who` in words;
  - `relayed`, when a person's verdict came through chat;
  - `about`: the document or task, with its ID;
  - `cause`: the run that led to it (`run_id`, `what`, `role`, `model`,
    `verdict`);
  - `runs`: each run in that phase (`run_id`, `what`, `state`, `verdict`,
    `tokens`, `attempt`);
- `url`: the feature's page.

**FR-4.2 — `get_agent_run`** has three arguments:
- `run_id`;
- an optional `attempt`, which defaults to the latest. One out of range is
  refused, because the page quietly shows the latest instead;
- an optional `include_tool_results`, false unless asked (SD-10).

It returns, from the same `buildRunPage` the run's page uses:
- what the run was, what it was about and its feature, its state, role,
  model, tokens in and out, and how long it took;
- which attempts exist;
- the conclusion: the verdict, reasoning, findings, criteria, summary and
  files, or the failure in words and the error;
- for a review, what became of its verdict: applied, held, ruled on by a
  person after an escalation, or not applied and why, read from
  `document_verdicts`;
- the transcript, cut as SD-10 says: the prompt, then each turn's text and
  tool calls, each with `truncated` set when cut;
- `url`: the run's page.

**FR-4.3 — Both are read-only**, and authoring-safe under DEC-004: they read
what the web UI already shows. `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
names them, with a comment saying why they are there.

**Acceptance:**
- after the end-to-end run (FR-6), `get_timeline` lists "Spec written" by the
  chat agent, "Spec approved" caused by a `review-spec` run, "Sent to
  development", and the plan's moments;
- `get_agent_run` on the spec's review returns its verdict, says it was
  applied, and gives a transcript without tool results;
- a missing run, an attempt out of range, and a feature that doesn't exist
  are each refused with a sentence;
- a long transcript is cut, and says so.

### FR-5: The send screen

**FR-5.1** A step that is already done says who did it, from FR-2.4:
- "Already written by the chat agent. This step is skipped."
- "Approved by the spec reviewer (deepseek-chat). This step is skipped."
- With no writer recorded: "Already written. This step is skipped."

**FR-5.2** A current spec or plan that is a draft, and doesn't wait for its
author, still counts as "already written" (SPEC-011 FR-2.1). But nothing will
submit it, so its review step warns: "This specification is a draft nobody
has submitted, so it won't be reviewed until someone submits it, on its page
or from chat." A draft returned by an escalation answer with no reason reads
the same, though it was submitted once; the advice is still right.

**Acceptance:**
- the send screen for a feature whose spec the chat agent wrote and the
  reviewer approved says both steps are skipped, naming the chat agent and
  the reviewer;
- a feature with an unsubmitted chat draft shows the warning.

### FR-6: The end-to-end proof

**FR-6.1** An integration test with the mock provider. Every chat step goes
over `POST /mcp`:
1. the chat agent writes a spec file for a feature under an approved design,
   and commits it;
2. it adds the file with `adopt_document` (and, in a second case,
   `attach_document`);
3. it calls `submit_for_review`, with no quote, and the scripted reviewer
   approves;
4. a person presses Send, through `POST /ui/send`;
5. the send screen, fetched before the press, says spec writing and review are
   skipped, and no `write-spec` run is ever queued;
6. the plan is written, reviewed and decomposed, and the estimate runs;
7. the feature is `ready`, and no implementation run is queued;
8. the spec's page says "Written by the chat agent" and "Approved by the spec
   reviewer", and `get_feature` says the same in `written_by` and
   `last_verdict`.

**Acceptance:** the test passes, and the demo (DoD 3) shows steps 5 and 8 in
the browser.

### FR-7: What the chat agent is told

**FR-7.1** The `initialize` instructions add two things:
- the chat agent can write a spec or plan itself, add it, and submit it for
  review, and the reviewer decides;
- it can read a feature's timeline and an agent run.

**FR-7.2** They keep saying it holds no verdict of its own, and can't send,
start building, override a gate or answer the Inbox.

**FR-7.3** `create_feature`'s description stops saying a person starts a
feature "when its specification has been approved". It says instead that
nothing runs until a person sends the feature to development.

## 5. Non-functional requirements

- **NFR-1 — One service layer.** The MCP tools call the same methods the web
  UI calls, in-process, each in one transaction with its audit row (O-3).
- **NFR-2 — The boundary holds.**
  - The advertised tool set gains exactly `submit_for_review`, `get_timeline`
    and `get_agent_run`.
  - The must-not list still includes `send_to_development`, `withdraw_send`,
    `start_building` and `answer_checkpoint`.
  - It gains `approve_document`, `submit_document` and `submit_review`. The
    outcome tools of dispatched agents must never be offered to chat.
- **NFR-3 — Coordination.** Migration `0011` only. The document page gains one
  include, and its markup lives in `provenance.html`. No change to M9's files
  beyond that include and the document page's view struct.
- **NFR-4 — Human prose** in every sentence a person reads (D-6).
- **NFR-5 — Tested as before.**
  - Integration tests with the mock provider against real Postgres cover
    every FR.
  - `go vet ./...` and `go test -race -count=1 ./...` are clean.
  - The integration tests run rather than skip.

## 6. Open questions carried forward

- **Edits after registration aren't credited.** A person may rewrite the chat
  agent's draft in vim, and the page still says the chat agent wrote it. Git
  knows the commit author, but commits made by hand often carry the same
  identity as the chat agent's. A later milestone could add "edited by" from
  commits Subutai didn't make.
- **The person behind a relay is unnamed** until M15.
- **Relaying "already approved" from chat** (SD-8) waits for M11.
- **A milestone-level timeline** (M6 follow-up 3) isn't built here.
- **The timeline and the page word the same act differently** (FR-2.7). If
  that grates, the timeline could read the new tables.
- **A stronger reviewer when the chat agent did the work** (DEC-007 decision
  10) now has the data it needs, in `document_writers`. Destination: M13.
- **Linking each review comment to its run** on the document page (the other
  half of M6 follow-up 4). Comments already carry `dispatch_id`. The change
  would touch the comment thread M9 shares. Destination: the spare-hours
  track.

## 7. Changes after review

How each [REVIEW-017](../reviews/REVIEW-017-chat-as-a-proper-seat.md) finding
was dealt with, in the spec and in the build.

| Finding | What changed |
|---|---|
| R17-1 (material) | SD-1 lists what a submission can start, in four cases, and cites DESIGN-010 §5c's "submit work" alongside DEC-005. The tool refuses a revision on a feature being built (FR-1.2), which keeps Amendment 1's test of consequence clean. FR-1.3 says what a plan's approval does. |
| R17-2 (material) | FR-2.8 matches by time window, not by transaction. It reads held approvals, releases and approvals by adoption. It says a send-back and an issue can't be told apart before `0011`. The forward record no longer depends on inference: every verdict is written to `document_verdicts` in its own transaction. |
| R17-3 (material) | FR-2.2 says what changes where: one field in `internal/rules`, and the server's `approveDocumentAs` and `returnForChangesAs`, called by the surfaces that know how an act came. An escalation answer records the review it ruled on, from the answered question's context, and its sentence names no surface (SD-6). Transitions carry `verdict_by`, `via` and `dispatch_id` on their audit rows. |
| R17-4 (material) | SD-11: for the relays and `submit_for_review`, an ID names its newest revision that isn't superseded, and `.r<n>` names one exactly. FR-1.2 has separate refusals for approved and superseded documents. |
| R17-5 (material) | SD-8 says the roadmap bullet holds in the web UI through SPEC-015 FR-5.8, which FR-2.3 now records as a verdict. It puts relaying "already approved" from chat (M8 follow-up 5) to Sam, with M11 named for all five types. |
| R17-6 | The false claims about the timeline are gone (§2, FR-2.7). SD-12 refuses one actor name for both the chat agent and the web UI. |
| R17-7 | FR-2.2 credits a revision opened by a relayed issue to the person, and a cascade revision to Subutai whoever answered. `revised` goes in the submission's transaction. The writer line reads across revisions (FR-2.4). |
| R17-8 | SD-7 and FR-2.8 add a second pass at boot, with the configured actor names. It credits the CLI's actors as people. |
| R17-9 | FR-1.3 covers the held spec and the plan. The chat marker rides on the submission's transition, in its transaction (FR-1.1). |
| R17-10 | FR-4.2 refuses an attempt out of range, says what became of a review's verdict, and leaves tool results out unless asked. SD-10 says the transcript goes to the chat agent's provider. |
| R17-11 | FR-2.7 accepts the difference between the timeline and the page, and says so; §6 records it. |
| R17-12 | FR-3.2 lists the store work. `relay_tick_job`'s description mentions the ID. |
| R17-13 | The note on prose names `document_writers` as DEC-007's executor field for documents. The out-of-scope table names M13 for DEC-007 decision 10, and the spare-hours track for linking comments to their runs. |
| R17-14 | The status line points to §3. §2 cites the rules and `queueReview` for reviews on unsent features. The Authority list cites DEC-004 itself. FR-5.2 says what "nobody has submitted" means. FR-7.3 corrects `create_feature`. FR-2.6 says the relays now return a document entry. |

## Definition of done

1. Every FR's acceptance passes in the suite.
2. `go vet ./...` is clean, and `go test -race -count=1 -v ./...` is green,
   with the integration tests running.
3. **A demo with no AI provider.**
   - A fresh build serves a throwaway project from `/var/tmp/m10demo`.
   - The chat path runs over plain MCP JSON-RPC calls.
   - Playwright with the pre-installed Chromium shows the send screen
     skipping spec writing, and the "written by" lines.
   - Steps that need a model are proved by the mock-provider tests, and the
     walkthrough says which.
   - Screenshots, and `docs/walkthrough-spec-017.md`.
4. A handoff note, `docs/notes/handoff-M10-2026-09-28.md`.
5. The roadmap's §11 marks M10 done, with a pointer to the handoff.
6. REVIEW-017 is recorded and its findings are dealt with (§7).
7. Sam approves this spec with the build. **Done, 2026-09-28.**
8. **Eleven choices need Sam's explicit yes.** *All eleven accepted, Sam,
   2026-09-28.*
   1. `submit_for_review` needs no quote, because submitting is planning
      under DEC-005 and DESIGN-010 §5c's "submit work". What it can start is
      listed, and a revision on a feature being built is refused (SD-1). If
      yes, dated notes go in DEC-006 Amendment 1 and DESIGN-010 §5c.
   2. `relay_review_request` keeps its quote, for a fresh review of a
      document already in review (SD-2).
   3. What the chat agent adds reads "Written by the chat agent", and what a
      person adds reads "Added by" them (SD-4).
   4. A relayed verdict is a person's, carried by the chat agent (SD-5).
   5. A held approval is recorded as a verdict. A release records the
      reviewer's approval, with who released it. An escalation answer records
      the review it ruled on, and names no surface (SD-6).
   6. Existing documents are inferred in two passes, the second at boot with
      the configured actor names. Anything else says "not recorded" (SD-7).
   7. Relaying "already approved" from chat, for documents with no template,
      waits for M11, for all five types (SD-8).
   8. MCP results give IDs as `id` and row ids as `row_id` for milestones,
      roadmaps and checklists (SD-9).
   9. `get_agent_run` returns a transcript cut to size, with tool results
      only on request. What it returns goes to the chat agent's provider
      (SD-10).
   10. An ID in a relay or in `submit_for_review` names its newest revision
       that isn't superseded (SD-11).
   11. The configuration refuses one actor name for both the chat agent and
       the web UI (SD-12).
