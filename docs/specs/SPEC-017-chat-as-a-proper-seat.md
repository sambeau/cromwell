# SPEC-017: Chat as a proper seat

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. Sam has said they will
approve the spec and the build together. An independent review is recorded in
[REVIEW-017](../reviews/REVIEW-017-chat-as-a-proper-seat.md), and §7 says how
each finding was dealt with. Nine choices need Sam's explicit yes (§5, DoD 8).
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
- [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) and its Amendment 1:
  the chat agent may author the planning layer;
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
**Follow-ups taken up:** the [M6 handoff](../notes/handoff-M6-2026-09-28.md)
follow-ups 1 (`get_timeline`, `get_agent_run`), 2 (the review behind a verdict)
and 4 (a "written by" link to the authoring run); the
[M8 handoff](../notes/handoff-M8-2026-09-28.md) follow-up 1 (SD-19).
**Coordination:** M9 (the browser editor) runs in parallel. This spec owns
migration `0011`; M9 uses `0012` if it needs one. **M10 merges first.** The
document page is shared: this spec adds one include to it. M9 doesn't touch
the MCP files.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6).

Three words are kept apart throughout:

- **Writer** is whoever wrote a document: a dispatched agent, the chat agent, a
  person, or Subutai itself. DEC-007 calls this the document's *executor*.
- **Verdict** is a decision on a document in review: approve, or send back.
  Only a reviewer agent or a person gives one (DEC-007 decision 2).
- **Relay** is the chat agent carrying a person's decision, with their words
  quoted (DEC-006 Amendment 1 decision 8).

## 0. Framing

M3 built most of this milestone. The chat agent can relay a person's verdict,
issue, review request, release of a held spec and ticked job. The send screen
says which steps are already done. A person can approve a spec or plan
directly. And the machinery for working ahead already holds: a spec written in
chat and approved before Send satisfies the spec invariant, so Send starts at
the plan (SPEC-011 SD-2).

What is left is smaller, and it is what makes the seat *proper*:

- The chat agent can't hand in the work it did without a person's quoted
  words. A spec it has written can only reach review through
  `relay_review_request`, which is a relay.
- Nothing records who wrote a document, or who gave a verdict in a form a
  page can show. DESIGN-010 §8 and DEC-007 decision 1 both ask for it.
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

> Every document registered from now on records its writer, and every verdict
> records who gave it and how: an agent's review run, a person in the web UI,
> or a person relayed from chat with their words.

> The chat agent still holds no verdict of its own, and can't send, start
> building, override a gate or answer a checkpoint.

## 2. The audit: what M10 still needed

Checked against the roadmap's M10, DESIGN-010 §5a, §5c and §8, and the code
on `claude/lucid-cerf-ft8jbj` at `2f40650`.

| Asked for | Where | What exists | What is missing |
|---|---|---|---|
| Chat tools to submit a document | Roadmap M10; DEC-005; §5a | `relay_review_request` submits a draft, but needs a person's quote. | A way to submit without a quote (FR-1). |
| Chat tools to ask for an early review | Roadmap M10; §5a | Submitting a spec or plan queues its review, for a sent or unsent feature alike (SPEC-011 FR-3.1). `relay_review_request` asks for a fresh one, with a quote. | Nothing beyond FR-1: submitting *is* asking for the early review. Whether a fresh review should still need a quote is SD-2. |
| A human verdict on any document type | Roadmap M10 | A person can approve or send back any document in review, in the UI or by relay (SPEC-011 FR-5.5, FR-8). | Types with no template (decision, note, research, report, policy) can't be submitted, so they never reach review. The M8 handoff gives this to M11 (decisions). Left there (SD-8). |
| The send screen shows steps already done | Roadmap M10; §5 | Built in M3 (SPEC-011 FR-4.3): "already written (approved)", "already approved". | It doesn't say who wrote the document or that the step is skipped. A spec left as an unsubmitted draft reads as "already written" and then waits forever (FR-5). |
| Spec written in chat skips spec writing | §5 step 1; §5a | Works: a current spec that doesn't wait for its author satisfies the invariant (SPEC-011 FR-2.1). | No test proves the whole path (FR-6). |
| Every artifact records who produced it | §8; DEC-007 decision 1 | Audit rows name an actor: a role name, the MCP actor or a person. Relays add `via` and the quote. The timeline tells agent, chat and person apart. | No record a page can show. An agent's approval doesn't record the review run behind it (M6 follow-up 2). The timeline counts a relayed verdict as the chat agent's own, and misses the chat agent altogether when `server.mcp_actor` isn't set (FR-2.7). |
| Relay tools accept IDs | M8 follow-up 1 (SD-19) | `path` only. | FR-3.1. |
| `MS-`, `RM-`, `CL-` IDs in MCP results | M8 follow-up 1 | Results give the row id as `id`. Lookups take `MS-` and `RM-`; not `CL-`. | FR-3.2. |
| IDs on checklist pages and `plan.html` lists | M8 follow-up 1 | Milestone and roadmap pages show their IDs; checklists and the plan lists don't. | FR-3.3. |
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
| Per-user identity: naming the person behind a relay | M15 |
| The browser editor | M9, in parallel |
| Verdicts on documents with no template | M11 (the M8 handoff's follow-up 3), SD-8 |
| Crediting edits made after registration | §6. Subutai sees a changed file, not who changed it. |

### Scope decisions

- **SD-1 — Submitting is a planning act, and needs no quote.** A new tool,
  `submit_for_review`, submits a draft for review as the chat agent. The name
  keeps clear of `submit_document`, the outcome tool dispatched authors call.
  - **Why it needs no quote.** DEC-005 names "submit a document for review" as
    asking the orchestrator to do what it would do anyway. The rules engine
    decides whether a review runs, which role and which model. DEC-007 gives
    every executor "one way out: you submit it", and the chat agent is an
    executor of spec writing (§5b).
  - **The worst misuse** is the chat agent submitting a draft the person
    hadn't finished. That costs one spec review, about 2,900 tokens (DEC-006
    Amendment 1, "Alternatives"). It starts nothing else: the plan needs the
    sent mark, and the reviewer can send it back.
  - **What it changes.** DESIGN-010 §5c and DEC-006 Amendment 1 decision 8 put
    "a request for a review" on the relay list, which needs a quote. For a
    draft, submitting and requesting a review are one act (SPEC-011 FR-5.5:
    "draft, where it means Submit"). So this spec reads the relay list as
    covering the *fresh* review of a document already in review (SD-2), and
    submission as DEC-005 planning. **That is a change to what the chat agent
    may do, so it is Sam's choice** (DoD 8.1). If Sam agrees, the handoff
    proposes a dated note for DEC-006 Amendment 1 and DESIGN-010 §5c, as
    earlier milestones did.
  - **The alternative** is to keep the quote, in which case
    `submit_for_review` isn't built and `relay_review_request` stays the one
    route. The end-to-end proof then quotes a person at step 3.

- **SD-2 — A fresh review of a document already in review keeps its quote.**
  `relay_review_request` stays as it is for a document in `reviewing`. A
  fresh review there either duplicates one the orchestrator already queued, or
  overrides the project's choice to switch agent review off. Both are a
  person's call about spending, and the tool is on Amendment 1's list. On a
  draft it still works, with its quote, as before; its description now points
  to `submit_for_review` for that case (DoD 8.2).

- **SD-3 — A writer is recorded when Subutai sees the writing.** A new table
  holds one row per writing act Subutai knows about:
  - an author agent writing a document, or revising one;
  - a document added by attach or adopt;
  - a starter design;
  - a revision opened by Revise.

  Subutai doesn't see edits made to a file after that. The page says who
  *added* or *wrote* the document, not who last touched it (§6).

- **SD-4 — What the chat agent adds is credited as written by it.** When the
  chat agent attaches or adopts a file, the page says "Written by the chat
  agent". Its own tools tell it to write a file and then add it, so this is
  usually true. When a person adds a file in the web UI or the CLI, the page
  says "Added by *sam*", because people adopt files written long before
  Subutai knew them. The asymmetry is deliberate, and it is Sam's choice
  (DoD 8.3). The alternative is "Added by" for everyone, which would make the
  roadmap's test ("the spec shows written by the chat agent") false.

- **SD-5 — A relayed verdict is a person's, carried by the chat agent.** It is
  recorded as a person's verdict, `via: mcp`, with the quote and the MCP actor
  as the relayer. Until M15 there is no name for the person, so the page says
  "Approved by a person, relayed by the chat agent: “…”". The timeline stops
  counting the chat agent as the one who approved (FR-2.7). This follows
  DEC-006 Amendment 1's "carrying a judgement isn't judging" (DoD 8.4).

- **SD-6 — A verdict records the review run behind it.** An agent's verdict
  records its dispatch, role and model; a person's records how it came (web
  UI, the Inbox, or chat). A held agent approval is recorded too, marked held,
  and *let the reviewer decide* records the reviewer's approval again, with
  the person who released it (DoD 8.5). This closes M6 follow-up 2.

- **SD-7 — Existing documents are inferred from the audit trail, or marked
  "not recorded".** Migration `0011` fills the two tables for documents that
  already exist, from `audit_events` and `dispatches`, and marks each row
  *inferred*. It can tell apart:
  - an agent, when a document was registered by the role of an authoring run
    on its feature that finished just before;
  - the chat agent, when the actor is `chat-agent` (the default MCP actor) or
    any actor the trail shows relaying `via: mcp`;
  - a person, when the actor is `operator` (the default web UI actor), or
    any actor the trail shows acting `via: ui`.

  Anything else is left unrecorded, and the page says "Who wrote this wasn't
  recorded". Guessing a person from an unfamiliar actor name would be wrong as
  often as right (DoD 8.6).

- **SD-8 — Verdicts on documents with no template wait for M11.** A decision
  or note can't be submitted, because Submit validates against a template. The
  roadmap's "human verdict on any document type" is met for every type that
  can reach review. Giving the others a path means deciding what a decision's
  review is, which is M11's question (DoD 8.7).

- **SD-9 — MCP results name things by ID.** Following SPEC-015 FR-7.4 for
  documents, a milestone's, roadmap's or checklist's `id` becomes its ID
  (`MS-004`), and its row id moves to `row_id`. Every lookup still accepts row
  ids, IDs and exact names, so a chat agent that saved an old result still
  works. A job has no ID and keeps its row id as `id` (DoD 8.8).

- **SD-10 — The read tools return compact answers.** `get_agent_run` returns
  the conclusion in full and a transcript cut to size: the prompt and each
  message to 2,000 characters, each tool result to 500, and at most 60 turns,
  first and last kept. Every cut says so and gives the run's page address,
  where the whole thing is. A transcript can show what a tool read, including
  secrets (M6 choice 8); this tool gives the same view to the chat agent the
  person is already talking to (DoD 8.9).

## 4. Requirements

### FR-1: Submitting for review from chat

**FR-1.1** A new MCP tool, `submit_for_review`, in `internal/server/mcp_relay.go`
beside the relays, with one argument, `document`: the document's path or its
ID (`FEAT-003-spec`). It calls the same `SubmitDoc` the document page's Submit
calls, as the MCP actor, and audits `document.submitted_from_chat` with
`via: mcp`. It takes no quote.

**FR-1.2** It refuses, each with a sentence:
- a document that isn't a draft ("This document is already in review. A
  fresh review is a person's call: relay it with `relay_review_request` and
  their words.");
- a document whose type has no template (SD-8);
- a draft whose author agent is revising it (SPEC-011 FR-3.6);
- a document with a question about it waiting in the Inbox (SPEC-011 SD-11);
- a draft that fails validation, listing each problem, so the chat agent can
  fix the file and try again.

**FR-1.3** Its result is the document entry (FR-2.6) and a sentence saying
what happens next, read from the project's settings:
- a spec or plan with an agent reviewer: "Its review is queued. The spec
  reviewer (*model*) decides; you don't.";
- a spec with agent review switched off: "Agent review is switched off for
  this project, so the spec waits for a person to approve it.";
- a design: "It waits for a person to approve it, on its page or by telling
  you."

**FR-1.4** Its description says that it hands the chat agent's own work to
the reviewer, that the reviewer's verdict isn't the chat agent's to give, and
that it doesn't send the feature to development.

**FR-1.5** `relay_review_request`'s description says to use
`submit_for_review` for a draft the chat agent wrote, and that the relay is
for a person asking for a fresh review of a document already in review (SD-2).

**Acceptance:** over `POST /mcp`, `submit_for_review` moves a chat-written
spec draft to `reviewing` and queues `review-spec`, with no quote; the audit
row has `via: mcp`; each refusal in FR-1.2 is a sentence;
`relay_review_request` without a quote is still refused.

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
| `inferred` | true for a row migration `0011` inferred (SD-7) |
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
| `model`, `dispatch_id` | the review run, for an agent |
| `via` | `ui`, `inbox`, `mcp`, or empty for an agent |
| `quote` | a relay's quoted words |
| `released_by`, `released_via`, `released_quote` | who let the reviewer decide |
| `inferred` | as above |
| `at` | when |

**FR-2.2 — Where writers are recorded.** Each in the same transaction as the
registration it describes.

| Act | Recorded as |
|---|---|
| An author agent writes a new spec or plan | `wrote`, `agent`, its role, model and run |
| An author agent fills an existing draft (a revision, or a chat draft sent back after Send) | `revised`, `agent`, as above; `wrote` if the draft has no writer yet |
| Attach over MCP | `added`, `chat`, the MCP actor, `via: mcp` |
| Adopt over MCP, of a file not yet registered | `added`, `chat`, as above |
| Attach or adopt in the web UI | `added`, `person`, the UI actor, `via: ui` |
| `doc add` through the API | `added`, `person`, the request's actor, `via: api` |
| A starter design | `started`, by whoever created the entity (chat, person or API) |
| Revise | `opened_revision`, by whoever pressed it; `system` when the cascade opened it |

To carry the run and model, the rules engine's `FileAuthoredDocument` action
gains the dispatch's id (`internal/rules`, one field).

**FR-2.3 — Where verdicts are recorded.** In the approve and send-back
paths every verdict already goes through:
- `approveDocument`: an agent's approval (with its run), a person's direct
  approval, an escalation answered in the Inbox, and a release (the
  reviewer's approval, with `released_*`);
- the held approval (`holdDocument`), with `held` set;
- `returnForChanges`: an agent's send-back, a person's on a design, and an
  escalation answered "ask for changes";
- a person's send-back of a spec or plan (SPEC-011 SD-15), which goes through
  `returnWithIssue`. An issue raised on a document in review also goes back
  through `returnWithIssue`, but it is an issue, not a verdict, and records
  none.
- approving on adoption (*This was already approved*, SPEC-015 FR-5.8): a
  person's approval, `via: ui`.

A person's act records `via` from the surface: `ui` for the document page,
`inbox` for an escalation answer, `mcp` for a relay, with its quote.

**FR-2.4 — How a writer and a verdict are said.** One function builds each
sentence, used by the page, the MCP results and the send screen.
- Who: an agent is "the *role in words* (*model*)", such as "the spec author
  (deepseek-chat)", and links to its run; the chat agent is "the chat agent";
  a person is their name; `system` is "Subutai".
- The writer line joins the first act with the last different writer:
  "Written by the chat agent. Revised by the spec author (deepseek-chat)."
  - `wrote`: "Written by …"
  - `added`: "Written by the chat agent" for `chat`, "Added by *sam*" for a
    person (SD-4)
  - `started`: "Started from the template by …"
  - `opened_revision`: "Revision opened by …"
  - none: "Who wrote this wasn't recorded."
  - An inferred row adds "(from the audit trail)".
- The verdict line is the latest verdict:
  - "Approved by the spec reviewer (deepseek-chat)."
  - "Approved by the spec reviewer (deepseek-chat), and held for a person."
  - "Approved by the spec reviewer (deepseek-chat); a person let the reviewer
    decide, relayed by the chat agent: “…”."
  - "Approved by sam." / "Approved by sam, in the Inbox."
  - "Approved by a person, relayed by the chat agent: “…”." (SD-5)
  - "Sent back by …", in the same forms.
  - No verdict: no line.

**FR-2.5 — On the document page**, one include, `doc-written-by`, directly
under the page heading, in a new template file `provenance.html`. It shows the
writer line and the verdict line, each with its time, and links an agent to
its run. `entity.html` gains only the include (M9 coordination).

**FR-2.6 — In MCP results.** Every document entry (`get_feature`,
`get_initiative`, `list_documents`, `attach_document`, `adopt_document`,
`submit_for_review`, the relays) gains:
- `written_by`: `{ "kind", "who", "sentence", "model", "run_id" }`, or
  `{ "sentence": "Who wrote this wasn't recorded." }`;
- `last_verdict`: `{ "verdict", "kind", "who", "sentence", "via", "quote",
  "run_id", "held" }`, when there is one.

**FR-2.7 — In the timeline.**
- The timeline reads the chat agent's actor from `mcpActor()`, so its acts are
  credited to the chat agent when `server.mcp_actor` isn't set. Today they are
  credited to a person called `chat-agent`.
- A document's approval or send-back that was relayed is credited to "a
  person, relayed by the chat agent", not to the chat agent. The transition's
  audit row carries `via: mcp`, and the timeline reads it.
- A "Spec written" or "Plan written" moment by the chat agent says "by the
  chat agent", as a moment by an agent already names its run.

**FR-2.8 — Backfill** (SD-7), in migration `0011`, for every document with no
rows:
- **writers**, from its first `document.registered` or
  `document.revision_created` row: an agent when the actor is the role of a
  succeeded `write-spec` or `write-dev-plan` run on its feature that finished
  within a minute before; the chat agent or a person by SD-7's rules; else
  nothing.
- **verdicts**, from each `document.transition` row with event `approve` or
  `request_changes`: an agent when the actor is the role of a succeeded
  `review-<type>` run on the document that finished at or before it (taking
  that run's model and id); a person when the same transaction has a
  `document.human_verdict` or a checkpoint answer (taking its `via` and
  quote); else a verdict by an unknown giver is skipped.

**Acceptance:**
- an authored spec records its role, model and run, and its page links to the
  run;
- a spec attached over MCP says "Written by the chat agent", and one attached
  in the UI says "Added by" the UI actor;
- an agent approval, a held approval, a release, a direct approval, a relayed
  approval with its quote, an escalation answer, and a relayed send-back each
  record the right row and read as FR-2.4 says;
- an issue raised on a document in review records no verdict;
- the backfill, run on rows written as they were before `0011`, infers each
  case it can and leaves the rest "not recorded";
- a relayed approval's timeline moment says "a person, relayed by the chat
  agent", and a chat-written spec's says "by the chat agent" with
  `server.mcp_actor` unset.

### FR-3: IDs, as M8 left them

**FR-3.1 — The relay tools accept IDs.** `relay_verdict`, `relay_issue`,
`relay_review_request` and `relay_release_hold` accept a document's ID
(`FEAT-003-spec`, `DEC-005`) wherever they take its path, as the `path`
argument; the description says so. `relay_tick_job` accepts a `CL-` ID for its
checklist. An ID names the current revision.

**FR-3.2 — MCP results show IDs** (SD-9). Milestone, roadmap and checklist
entries give `id` as the ID and `row_id` as the row id, everywhere they appear:
their own tools, a roadmap's milestones, a checklist's milestones, and a
milestone's items. Checklist lookups accept `CL-` IDs, as milestone and roadmap
lookups already accept theirs.

**FR-3.3 — Pages show IDs.** The checklist page's heading and the checklist
editor show its `CL-` ID, as the milestone and roadmap pages do. `plan.html`'s
lists of milestones, roadmaps and checklists show each one's ID before its
name, with the same `ident` template the other lists use.

**FR-3.4 — `/ui/id/CL-…`** redirects to the checklist's page.

**Acceptance:** each relay works with an ID in `path`; `relay_tick_job` works
with `CL-001`; `get_milestone`, `get_roadmap` and `get_checklist` return
`MS-`, `RM-` and `CL-` IDs and still accept row ids; the checklist page and the
plan section render their IDs; `/ui/id/CL-001` redirects, and
`/ui/id/CL-999` is not found.

### FR-4: Reading the work from chat

**FR-4.1 — `get_timeline`**, with one argument, `feature` (path or ID). It
returns the feature's journey, from the same `featureTimeline` the page uses:
- `line`: the one-line view's labels, with counts;
- `moments`: each with `label`, `at`, `by` (`agent`, `person`, `chat`,
  `system`), `who` in words, `relayed` when a person's verdict came through
  chat, `about` (the document or task, with its ID), `cause` (the run that led
  to it: `run_id`, `what`, `role`, `model`, `verdict`), and `runs` (each run in
  that phase: `run_id`, `what`, `state`, `verdict`, `tokens`, `attempt`);
- `url`: the feature's page.

**FR-4.2 — `get_agent_run`**, with `run_id` and an optional `attempt`
(defaulting to the latest). It returns, from the same `buildRunPage` the run's
page uses:
- what the run was, what it was about and its feature, state, role, model,
  tokens in and out, and how long it took;
- which attempts exist;
- the conclusion: verdict, reasoning, findings, criteria, summary, files, or
  the failure in words and the error;
- the transcript, cut as SD-10 says: the prompt, then each turn's text and
  tool calls, each with `truncated` set when cut;
- `url`: the run's page.

**FR-4.3 — Both are read-only**, and authoring-safe under DEC-004: they read
what the web UI already shows. `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
names them, with a comment saying why they are there.

**Acceptance:** after the end-to-end run (FR-6), `get_timeline` lists "Spec
written" by the chat agent, "Spec approved" caused by a `review-spec` run,
"Sent to development", and the plan's moments; `get_agent_run` on the spec's
review returns its verdict and a transcript; a missing run, an attempt out of
range, and a feature that doesn't exist are refused with sentences; a long
transcript is cut and says so.

### FR-5: The send screen

**FR-5.1** A step that is already done says who did it, from FR-2.4:
- "Already written by the chat agent. This step is skipped."
- "Already approved by the spec reviewer (deepseek-chat). This step is
  skipped."
- With no writer recorded: "Already written. This step is skipped."

**FR-5.2** A current spec or plan that is a draft nobody has submitted, and
that doesn't wait for its author, is still "already written" (SPEC-011
FR-2.1), and its review step warns: "This specification is a draft nobody has
submitted, so it won't be reviewed until someone submits it, on its page or
from chat."

**Acceptance:** the send screen for a feature whose spec the chat agent wrote
and the reviewer approved says both steps are skipped, naming the chat agent
and the reviewer; a feature with an unsubmitted chat draft shows the warning.

### FR-6: The end-to-end proof

**FR-6.1** An integration test with the mock provider, over `POST /mcp` for
every chat step:
1. the chat agent writes a spec file for a feature under an approved design,
   and commits it;
2. it adds the file with `adopt_document` (and, in a second case,
   `attach_document`);
3. it calls `submit_for_review`, with no quote; the scripted reviewer
   approves;
4. a person presses Send, through `POST /ui/send`;
5. the send screen, fetched before the press, says spec writing and review are
   skipped; no `write-spec` run is ever queued;
6. the plan is written, reviewed and decomposed, and the estimate runs;
7. the feature is `ready`, and no implementation run is queued;
8. the spec's page says "Written by the chat agent" and "Approved by the spec
   reviewer", and `get_feature` says the same in `written_by` and
   `last_verdict`.

**Acceptance:** the test passes, and the demo (DoD 3) shows steps 5 and 8 in
the browser.

### FR-7: What the chat agent is told

**FR-7.1** The `initialize` instructions add that the chat agent can write a
spec or plan itself, add it, and submit it for review, and that the reviewer
decides; and that it can read a feature's timeline and an agent run.

**FR-7.2** They keep saying it holds no verdict of its own, and can't send,
start building, override a gate or answer the Inbox.

## 5. Non-functional requirements

- **NFR-1 — One service layer.** The MCP tools call the same methods the web
  UI calls, in-process, each in one transaction with its audit row (O-3).
- **NFR-2 — The boundary holds.** The advertised tool set gains exactly
  `submit_for_review`, `get_timeline` and `get_agent_run`. The must-not list
  still includes `send_to_development`, `withdraw_send`, `start_building`,
  `answer_checkpoint`, and gains `approve_document` and `submit_document`
  (the agent outcome tool's name must never be advertised to chat).
- **NFR-3 — Coordination.** Migration `0011` only. The document page gains
  one include; its markup lives in `provenance.html`. No change to M9's
  files beyond that include and the document page's view struct.
- **NFR-4 — Human prose** in every sentence a person reads (D-6).
- **NFR-5 — Tested as before.** Integration tests with the mock provider
  against real Postgres cover every FR. `go vet ./...` and
  `go test -race -count=1 ./...` are clean, and the integration tests run
  rather than skip.

## 6. Open questions carried forward

- **Edits after registration aren't credited.** A person may rewrite the chat
  agent's draft in vim, and the page still says the chat agent wrote it. Git
  knows the commit author, but commits made by hand often carry the same
  identity as the chat agent's. A later milestone could add "edited by" from
  commits Subutai didn't make.
- **The person behind a relay is unnamed** until M15.
- **Verdicts on untemplated documents** (SD-8) wait for M11.
- **A milestone-level timeline** (M6 follow-up 3) isn't built here.

## 7. Changes after review

See [REVIEW-017](../reviews/REVIEW-017-chat-as-a-proper-seat.md). This section
is filled in when the review's findings are dealt with.

## Definition of done

1. Every FR's acceptance passes in the suite.
2. `go vet ./...` is clean, and `go test -race -count=1 -v ./...` is green,
   with the integration tests running.
3. **A demo with no AI provider.** A fresh build serves a throwaway project
   from `/var/tmp/m10demo`. The chat path runs over plain MCP JSON-RPC calls.
   Playwright with the pre-installed Chromium shows the send screen skipping
   spec writing, and the "written by" lines. Steps that need a model are
   proved by the mock-provider tests, and the walkthrough says which.
   Screenshots and `docs/walkthrough-spec-017.md`.
4. A handoff note, `docs/notes/handoff-M10-2026-09-28.md`.
5. The roadmap's §11 marks M10 done, with a pointer to the handoff.
6. REVIEW-017 is recorded and its findings dealt with (§7).
7. Sam approves this spec with the build.
8. **Nine choices need Sam's explicit yes:**
   1. `submit_for_review` needs no quote, because submitting is planning
      under DEC-005 (SD-1). If yes, dated notes go in DEC-006 Amendment 1 and
      DESIGN-010 §5c.
   2. `relay_review_request` keeps its quote, for a fresh review of a
      document already in review (SD-2).
   3. What the chat agent adds reads "Written by the chat agent", and what a
      person adds reads "Added by" them (SD-4).
   4. A relayed verdict is a person's, carried by the chat agent (SD-5).
   5. A held approval is recorded as a verdict, and a release records the
      reviewer's approval with who released it (SD-6).
   6. Existing documents are inferred only where the trail is clear, and
      otherwise say "not recorded" (SD-7).
   7. Verdicts on documents with no template wait for M11 (SD-8).
   8. MCP results give IDs as `id` and row ids as `row_id` for milestones,
      roadmaps and checklists (SD-9).
   9. `get_agent_run` returns a transcript cut to size, and can show what a
      tool read (SD-10).
