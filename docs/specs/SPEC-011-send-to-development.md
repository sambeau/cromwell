# SPEC-011: Send to development

**Status:** **Approved — Sam, 2026-09-28**, with the build, and with the recommendation accepted on each of its eight choices (DoD 7). It was drafted for Sam's approval as follows. Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. Sam has said they will
approve the spec and the build together. An independent consistency review is
recorded in [REVIEW-011](../reviews/REVIEW-011-send-to-development.md). It
found five material and fourteen smaller problems in the first draft; all are
dealt with in this revision, and §7 says how, finding by finding. Eight choices
need Sam's explicit yes (§5, DoD 7).
**Date:** 2026-09-28
**Roadmap milestone:** M3 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11
**Authority:** [DEC-006](../decisions/DEC-006-humans-start-development.md),
**with Amendment 1**, which replaces its decisions 5, 6 and 8. This is the
brief.
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28): §2, §3 (the design reviewer is retired), §5 (the workflow, "The
first button", "How specs are reviewed", "When a design changes"), §5a, §5c,
§14, and §17b (withdrawing a send, and how a human issue is marked addressed,
are settled here)
**Changes:** [SPEC-009](SPEC-009-the-authoring-chain.md) FR-2.2 (the design
reviewer is retired), FR-2.3 and FR-2.4 (a person may now approve a spec or
plan directly, as Amendment 1 decision 5 allows), FR-4.1, FR-4.2, FR-4.3,
FR-4.7 and FR-9.4, as DEC-006 says. FR-3, FR-8 and the rest of FR-9 are kept
as built.
**Context only:** [DEC-007](../decisions/DEC-007-the-judgement-boundary.md).
Executors and claims are M13, not this spec.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, gate reason, checkpoint question and tool
description a person will read is written in full sentences (DESIGN-008 D-6).

## 0. Framing

SPEC-009 built the chain that turns an approved design into a reviewed spec, a
reviewed plan and tasks. Its trigger was approving the design. DEC-006 keeps
the chain and moves the trigger: approving a design records what we want, and
a person presses **Send to development** when there is capacity to spend on
it. Amendment 1 then settles how specs are reviewed: an agent reviewer is the
normal approver, humans point out issues, and there is always at least one
reviewer.

Most of the machinery exists. What this spec adds is:

- a *sent* mark on features, and the button and screen that set it;
- the invariants rewritten to need the mark;
- the missing half of the review loop: a spec or plan sent back goes back to
  its author agent (SPEC-009 as built leaves it in draft with nobody to revise
  it);
- human issues, the hold, and agent review switched off;
- four relay tools for the chat agent;
- Submit, Revise and Detach on the document page;
- the design reviewer retired;
- and one label renamed.

## 1. Goal

**One claim, which the definition of done checks directly:**

> Approving a design starts nothing. Pressing Send to development on a feature
> produces a reviewed spec, a reviewed plan, tasks and an estimate, and then
> stops at Start building. If the spec is held, it waits for a person between
> its review and the plan.

Three supporting claims:

> No configuration lets a spec through unreviewed.

> A human's issue on a spec is never dropped: the reviewer can't approve until
> it has said how each one was addressed, or why it doesn't apply.

> The chat agent can carry a human's verdict, issue, review request or release
> of a held spec, quoting the human, and can do nothing that sends, starts
> building, locks, overrides a gate or answers a checkpoint.

## 2. Scope

### In scope

1. The sent mark, and the invariants and triggers rewritten around it (FR-1,
   FR-2).
2. The review-and-revise loop for authored documents (FR-3).
3. Send to development: the button, the send screen, and Withdraw (FR-4).
4. Spec review under Amendment 1: project settings, the hold, direct human
   approval, requested reviews (FR-5).
5. Human issues and how they are marked addressed (FR-6).
6. Automatic estimation as the chain's last step (FR-7).
7. The chat relay tools (FR-8).
8. Submit, Revise and Detach on the document page (FR-9).
9. Retiring the design reviewer (FR-10).
10. The starter pack enables the authoring chain, and gate 2 is renamed Start
    building (FR-11).

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| Executors, claims, `claim_task` and `submit_task` | M13 (DEC-007) |
| Checklists, jobs, and the tick-a-job relay tool | M5 |
| Transcripts and the timeline | M6 |
| Renaming Cromwell to Subutai | M7. User-facing text in this spec says "Cromwell" where it already does, and adds no new product name. |
| Document IDs and the revision naming scheme | M8 |
| A hard token budget at the button | DEC-006 "Alternatives": deferred until forecasting from past throughput is better |
| Per-user identity for relays | Before the Tickly pilot (roadmap decision 14). Relays are audited to the configured MCP actor, as DEC-006 records. |
| An API or CLI route for sending | **Nowhere.** Sending is a web UI act (DEC-006 decision 2). |

### Scope decisions

- **SD-1 — The mark is a row, not a state or a column.** A feature gains a
  `feature_sends` row: who sent it, when, and the send's settings (the hold).
  DEC-006 says the mark is not a lifecycle state, and a separate table keeps
  every existing `features` scan unchanged. A feature is sent at most once at a
  time; Withdraw deletes the row, and the audit trail keeps the history.

- **SD-2 — Both invariants need the mark, not only the spec invariant.**
  DEC-006 names FR-4.1. But DESIGN-010 §5 says "nothing runs before Send", and
  a spec approved early in chat (§5a) would otherwise set the plan writer off
  on a feature nobody sent. So the dev-plan invariant also needs the mark, and
  so does the estimate (FR-7).

  A `ready` feature can be sent too. A feature whose spec and plan were both
  written and approved in chat reaches `ready` by G1 without anyone pressing
  Send; sending it runs what is left (the estimate), which is how DESIGN-010
  §5a's "a well-prepared feature flows straight through" reads.

- **SD-3 — Building counts as sent.** A feature that is `active` or in
  `review` is treated as sent, whether or not it has a mark. Start building is
  a stronger commitment than Send, and a feature started before M3, or with
  documents written by hand, must still get its spec rewritten when the design
  is revised mid-build (SPEC-009 FR-9.6). A `ready` feature without a mark is
  *not* treated as sent: DESIGN-010 §5 says a feature not yet sent is left
  without a spec until someone sends it.

- **SD-4 — A document sent back goes back to its author agent, as an
  invariant.** SPEC-009 as built has no revise loop: a reviewer's
  `request_changes` moves an authored spec to `draft`, and nothing ever writes
  it again. Amendment 1 says "the spec goes back to the spec agent", so this
  spec builds the loop for specs and plans (FR-3). It is part of the invariant
  rather than a reaction to one transition, so Send, the heartbeat and every
  re-check pick it up (REVIEW-011 R11-1): a current draft that **waits for its
  author** — one carrying an open human issue or a major review finding — does
  not satisfy the invariant for a sent feature. The loop is bounded by the
  existing `dispatch.max_review_rounds`, after which an `authoring-deadlock`
  checkpoint asks a person whether to allow another round.

- **SD-5 — The hold catches an approval, not a send-back.** When the agent
  reviewer asks for changes, the spec goes straight back to its author, held
  or not. Holding a send-back would only delay the fix: the human can already
  raise issues or approve directly at any time. So "waits after the agent
  review" is read as "waits instead of being approved".

- **SD-6 — How a human issue is marked addressed** (DESIGN-010 §17b). The
  answer lives in the review outcome, as C-1's severity does:
  - Issues are rows in `document_comments` with `is_issue` set. They are always
    must-address.
  - The spec reviewer's `submit_review` gains an `issues` list. Each entry names
    an open issue and says `addressed` or `does_not_apply`, with a note, or
    `not_addressed`.
  - An `approve` verdict must answer every open issue with `addressed` or
    `does_not_apply`. This is checked inside the reviewer's turn, as severity
    is, so a reviewer that forgets can correct itself.
  - An answered issue is stored as resolved, with the note, the dispatch and
    the time. `not_addressed` leaves it open, and it makes the verdict a
    send-back.
  - A person who approves — directly, or by answering a review escalation —
    settles the open issues by that decision, and the record says so. Any
    approval that carries no agent review is a person's.

  This was chosen over a separate "issues" table because the comment thread is
  already where review findings live, already persists across resubmission,
  and already reaches the reviewer's prompt. It was chosen over letting the
  author mark issues addressed because nobody judges their own work (DEC-007).

- **SD-7 — An issue on an approved spec opens a successor, not an
  invalidation.** Amendment 1 says the approved spec "reopens for revision" and
  DESIGN-010 §5 says "the issue starts a successor. When the successor is
  approved, it supersedes the old spec, and the old plan is superseded with
  it." So the approved spec stays current until the successor is approved;
  then the approval supersedes it and its approved plan together, and a `ready`
  feature returns to `idea` so the plan is rewritten. In the store's terms the
  successor draft becomes the feature's *current* spec at once (the newest
  live document), while the predecessor stays `approved` and in force; the
  current spec not being approved is what makes Start building refuse
  (FR-6.7).

- **SD-8 — Existing projects keep their design reviewer.** A project whose
  design manifest still names `reviewer_role: design-reviewer` keeps getting
  its comments, which never rule. Config is strict and authoritative here
  (DESIGN-004 F-1), and silently ignoring a setting a project wrote down is the
  kind of drift the strict parser exists to prevent. It does no harm: the
  reviewer comments and never rules, and it runs on design submission, not on
  Send. A project retires it by deleting the line. New projects don't have it.

- **SD-9 — The chat-side `review-design` skill lives in
  `.cromwell/chat-skills/`.** The starter pack ships it at
  `chat-skills/review-design/SKILL.md`. `chat-skills/` is where DEC-007's
  `work-a-task` will go too. The orchestrator never reads that directory: it
  is for the human to hand to their chat AI.

- **SD-10 — Withdraw is built, and small.** A send can be withdrawn while none
  of its work has started: every authoring dispatch since the send is still
  queued, and no spec or plan has been written by one. Withdrawing cancels the
  queued dispatches and removes the mark. Once an agent has started, Abandon
  is the way out, as today. This is worth having because sends wait in the
  queue when slots are full, which is exactly when a person may change their
  mind. **Flagged for Sam.**

- **SD-11 — No act gets round a pending question.** A checkpoint is answered
  in the Inbox, and the chat agent doesn't answer checkpoints (DESIGN-010 §5c).
  So every act in FR-5.5 and FR-6, from either surface, is refused on a
  document with a pending question about it, with a sentence pointing to the
  Inbox: a `review-escalation` or `authoring-deadlock` checkpoint that refs the
  document, or a `design-revision` checkpoint that lists its feature. Without
  this, raising an issue or asking for a fresh review would answer the
  question by the back door (R11-4).

- **SD-12 — The forecast is honest or absent.** The send screen forecasts each
  planning step as the median tokens of this project's past successful
  dispatches of that purpose, when there are at least three. Otherwise it says
  "no forecast yet". `internal/sizing` sizes implementation work, not planning
  steps, so it can't forecast these; the screen shows the feature's
  implementation estimate beside the forecast when there is one, and says what
  it is.

- **SD-13 — The settings live in `config.yaml`.** A `spec_review` block holds
  `agent` (on by default) and `hold` (off by default). There is no settings
  page yet; the file is the project's settings today.

- **SD-14 — Issues can be raised on specs and plans, and noted on designs.**
  DESIGN-010 §5 talks about specs. The plan reviewer uses the same loop, so
  plans get the same treatment; the one extra piece is that a successor plan
  approved on a `ready` feature is re-decomposed (FR-6.6), which SPEC-009 did
  only for features being built. A design in `draft` or `reviewing` may carry
  an issue as a note for its human approver, with no routing; an approved
  design is changed by revising it, so an issue on one is refused with that
  advice.

- **SD-15 — A relayed or UI "send back" of a spec is a human issue and a
  return to draft.** A person's objection must be addressed (SD-6), so the
  reason is recorded as an issue, and the document goes back to `draft`
  whether or not the feature is sent. For a sent feature the author then
  revises it; for an unsent one it waits for whoever is writing it (R11-8).

- **SD-16 — The CLI and API can still start building.** DESIGN-010 §5 says
  Start building is in the web UI only. `POST /api/features/start`, which
  `cromwell feature start` calls, predates it and is due to go with the CLI
  under DEC-003. This spec doesn't remove it; it records the departure for Sam
  (R11-3). Sending, which is new, has no API route.

## 3. Requirements

### FR-1: The sent mark

**FR-1.1** Migration `0007` adds `feature_sends`: `feature_id` (primary key,
references `features`), `sent_by`, `sent_at`, and `hold` (boolean, the send's
hold after FR-5.2's rules). It is additive, as every migration is.

**FR-1.2** A feature is **sent** when it has a `feature_sends` row, or when it
is `active` or in `review` (SD-3). One predicate, `featureSent`, answers this
for every caller.

**FR-1.3** Sending writes the row and a `feature.sent` audit row in one
transaction, with the actor and the send's settings, and publishes a new
`FeatureSent` event after commit.

**FR-1.4** The feature page shows the mark: "Sent to development by *actor*,
*time* ago", and "Held for a person after review" when the send's hold is on.

**Acceptance:** sending a feature writes one row and one audit row; sending it
again is refused with a sentence; the page shows who sent it.

### FR-2: The invariants and their triggers

**FR-2.1 — The spec invariant** (replaces SPEC-009 FR-4.1):

> Every **sent** feature that G0 admits, and that has a non-empty description,
> has a current spec that isn't waiting for its author.

**FR-2.2 — The dev-plan invariant** (replaces FR-4.2, SD-2):

> Every **sent** feature with an approved spec has a current dev-plan that
> isn't waiting for its author.

A document **waits for its author** when it is the current `draft` and carries
an open human issue or a major review finding (SD-4). Such a draft is revised
by the author agent (FR-3). Any other draft — one written by hand or in chat and
not yet submitted, with nothing against it — belongs to whoever is writing it,
and satisfies the invariant.

**FR-2.2a — The estimate** (FR-7) is owed by a sent feature, in `idea` or
`ready`, with an approved plan, when the project assigns an `estimate` role and
no estimate has been queued since the later of the send and the plan's
approval.

**FR-2.3 — Triggers** (replaces FR-4.3):

| Event | What it does |
|---|---|
| **Send to development** | Sets off the invariants for the feature. **The new trigger.** |
| A design reaching `approved` | Re-checks the invariants over its scope. Starts nothing for an unsent feature. |
| A feature being created, or gaining a description | Re-checks. Starts nothing for an unsent feature. |
| A spec reaching `approved` | Re-checks the dev-plan invariant. |
| A plan reaching `approved` | Decomposes (as built), re-checks G1 and the estimate. |
| A spec or plan sent back to `draft` | Re-checks, which runs the revise loop (FR-3). |
| A human issue on a draft | Re-checks, likewise. |
| Detach | Re-checks the feature (FR-9.3). |
| An `authoring-deadlock` answered retry | One more round (FR-3.3). |
| Withdraw | Nothing to start; queued work is cancelled (FR-4.6). |
| A change to `spec_review` in `config.yaml` | Nothing at once. Agent review is read when a spec is submitted and when a verdict arrives, so the change applies from then, including to work already sent. The per-send hold is fixed when the send is made. |

**FR-2.4 — The heartbeat sweep covers sent features only** (replaces FR-4.7).
Its never-attempted boundary is kept, and read for a revision as "not attempted
since the draft last changed": a failed revision has a dispatch-failure
checkpoint governing its retry, as a failed first draft does.

**FR-2.5 — The cascade rewrites only for sent features** (replaces FR-9.4).
FR-9.1 and FR-9.2 still choose which specs to retire, for every feature. A
retired spec on a sent feature is rewritten automatically; one on an unsent
feature leaves the feature without a spec until it is sent. A `ready` feature
that loses its contract returns to `idea`, as built, whether or not it is
sent.

**FR-2.6** G0's refusal reasons become sentences a person can act on, because
the Send button shows them as they stand. For example: "Neither this feature's
design nor its initiative's design is approved yet. A feature can be sent to
development once one of them is."

**Acceptance:**
- approving a design over described features dispatches nothing, raises no
  checkpoint, and writes no spec (`TestApprovingADesignStartsNothing`);
- creating and describing a feature under an approved design dispatches
  nothing;
- sending one of two features dispatches `write-spec` for that one only, and
  replaying the event dispatches nothing more;
- the heartbeat sweep fills a gap in a sent feature and leaves an unsent one
  alone;
- the SPEC-009 cascade test passes with sent features, and a second case shows
  an unsent feature's spec retired and not rewritten.

### FR-3: The review-and-revise loop

**FR-3.1** When a sent feature's current spec or plan waits for its author
(FR-2.1), the orchestrator dispatches the author again: `write-spec` or
`write-dev-plan`, whichever the type needs. The usual cause is a send-back,
by the reviewer or a person; an early review in chat that sent a spec back
before the feature was sent is picked up by Send the same way.

**FR-3.2** A revising author's prompt carries the current draft, every open
review finding, and every open human issue, and asks for the whole revised
file. `submit_document` fills the existing draft in place, as it already does
for a cascade successor, and submits it.

**FR-3.3** The loop is bounded. When the document has already had
`dispatch.max_review_rounds` reviews, the orchestrator raises an
`authoring-deadlock` checkpoint instead ("*Spec* for *feature* has been sent
back *n* times. Allow the author another round, or stop and edit it
yourself?"), answered `retry` or `cancel`. Retry dispatches one more round;
cancel leaves the draft for a person, who can edit it and press Submit.

**FR-3.4** A draft with nothing against it (written by hand or in chat) is not
the loop's business. Its author submits it when ready. A draft with an issue
against it, on a sent feature, *is* the loop's business, whoever wrote it:
DESIGN-010 §5 sends it "back to the spec author" (R11-6).

**FR-3.5** A review verdict is applied only to the content it reviewed: one that
arrives for a document no longer in `reviewing`, or whose content has changed
since the review was queued, is dropped.

**FR-3.6** While an author dispatch for the feature is queued or running,
Submit and *Ask for an agent review* on that document are refused ("Its author
is revising it now"), so a person's submission can't race the author's.

**Acceptance:** a reviewer's send-back on a sent feature's spec dispatches a
second `write-spec`, whose prompt carries the finding; the revised spec is
resubmitted and approved. At the round cap a checkpoint is raised instead, and
answering retry dispatches once more. A send-back on an unsent feature's spec
dispatches nothing.

### FR-4: Send to development

**FR-4.1 — On the feature page.** An unsent feature in `idea` or `ready` shows
**Send to development** as its primary action (a `ready` one beside Start
building). When it can't be sent, the button is
disabled in a rail card with the reason beside it, in G0's words or "This
feature has no description yet. Describe what it should do before sending it,
because the spec is written from that description." A sent feature shows its
mark instead, and **Withdraw** while FR-4.6 allows it.

**FR-4.2 — On the initiative page.** An initiative with at least one direct
child feature shows **Send features to development…** in its header. The send
screen lists its direct child features, each with a checkbox; one that can't
be sent is listed unchecked and disabled, with its reason.

**FR-4.3 — The send screen** (`GET /ui/send?feature=…` or
`?initiative=…`) shows, before anything is committed:
- the features, with checkboxes;
- for each feature, each step: write the spec, review the spec, hold for a
  person, write the plan, review the plan, estimate. Each shows the role and
  model that will run it, or "you" for the hold, and says **already done** when
  its document exists (a spec written in chat is used as it is, DESIGN-010 §5a);
- **who will review each spec**, in a sentence built from the spec manifest's
  `approved_by` and `spec_review`: "The spec reviewer (*model*) approves it",
  "The spec reviewer checks it, then it waits for you", or "You review it;
  agent review is switched off for this project";
- a warning when a step's role isn't assigned in `config.yaml` (a project from
  before M3 has the authoring roles commented out): "Nobody is assigned to
  write the spec, so it won't be written until you write it or assign
  `write-spec`" (R11-9);
- a rough token forecast per step and in total (SD-12), or "no forecast yet";
- free agent slots: `dispatch.workers` less the dispatches running now, and
  that work which doesn't fit waits in the queue;
- the hold, as a checkbox defaulting to the project setting, forced on and
  disabled when agent review is off (FR-5.1), with a line saying the hold is
  fixed at the send but the agent-review setting is read as the work runs.

A step is **already done** when its document exists and doesn't wait for its
author: for writing, the current document exists; for a review, it is
approved; for the estimate, the feature has one newer than its plan.

**FR-4.4 — Sending** (`POST /ui/send`). For each ticked feature, the server
re-checks the preconditions (G0, description, not already sent, state `idea` or
`ready`),
then writes the mark (FR-1.3). A feature that fails is reported by name and
reason; the others are sent. The response is the page the person came from,
with a notice naming what was sent.

**FR-4.5 — No other route sends.** There is no `/api/*` route, no CLI verb and
no MCP tool that writes the mark (NFR-3).

**FR-4.6 — Withdraw** (SD-10). `POST /ui/send/withdraw` removes the mark while
every authoring dispatch for the feature since the send is still `queued`, and
no spec or plan row was created by one. It cancels those queued dispatches and
audits `feature.send_withdrawn`, in one transaction. Otherwise it is refused:
"Work on this feature has already started, so the send can't be withdrawn.
Abandon the feature if it should stop."

**Acceptance:**
- the button refuses with G0's reason, then with the description reason, and
  then works;
- the send screen renders each part for one feature and for an initiative's
  three, with one disabled;
- a `POST` sends the ticked features only;
- withdrawing a send whose `write-spec` is still queued cancels it and clears
  the mark, and withdrawing after the dispatch has run is refused.

### FR-5: Spec review under Amendment 1

**FR-5.1 — Project settings** (SD-13), in `config.yaml`:

```yaml
spec_review:
  agent: true   # the agent spec review; on by default
  hold: false   # hold every spec for a person after its review; off by default
```

When `agent` is false, the hold is on for every spec, whatever `hold` or the
send says. **No combination lets a spec through unreviewed** (Amendment 1,
decision 6).

**FR-5.2 — Whether a spec is held.** A spec is held when agent review is off,
or when its feature's send has the hold on, or, for an unsent feature, when the
project's `hold` is on.

**FR-5.3 — Submission.** A submitted spec is validated as today. Then:
- with agent review on, its review is queued as today;
- with agent review off, no review is queued. The spec waits in `reviewing`,
  held for a person.

**FR-5.4 — The agent's approval, held.** When the spec reviewer approves a held
spec, the spec stays in `reviewing`, the reviewer's reasoning and minor
findings join its thread, and the hold records which review approved it. A
send-back or an escalation is not held (SD-5).

**FR-5.5 — What a person can do**, on the document page and, except where
noted, by relay (FR-8):

| Act | When | What happens |
|---|---|---|
| **Approve** | the spec is `reviewing` | Approved at once, the person's verdict. Open issues are settled by it (SD-6), and any hold is cleared. This is new authority for agent-approved types: SPEC-009 FR-2.3 and FR-2.4 gave a person a spec only on escalation. |
| **Send back** | the spec is `reviewing` | SD-15: the reason becomes a human issue and the spec returns to `draft`. |
| **Raise an issue** | before building (FR-6) | FR-6. |
| **Ask for an agent review** | `reviewing`, with no review queued or running; or `draft` (where it means Submit) | A fresh review is queued, even with agent review switched off (a one-off). Its approval of a held spec is held again. |
| **Let the reviewer decide** | the spec is held after an agent approval, agent review is on, and no issue is open | The held approval stands: the spec is approved, attributed to the reviewer, and the release is audited to the person. |

With agent review off, **Let the reviewer decide** isn't offered, because no
automatic reviewer has given a verdict. It is refused if forced.

Every act in this table is refused while a question about the document is
pending (SD-11).

**FR-5.4a — Where a hold lives.** Migration `0007` adds `document_holds`:
`document_id` (primary key), `dispatch_id` (the review whose approval is held,
or null when none is), and `created_at`. A hold is written when a held spec is
submitted with agent review off or approved by its reviewer, and cleared by any
approval or any return to `draft`.

**FR-5.6** The document page says plainly when a spec is held: "This spec is
waiting for you. The spec reviewer approved it; it approves only when you
approve it, or let the reviewer decide." With agent review off: "This spec is
waiting for you, because agent review is switched off for this project."

**FR-5.7** Plans follow the same rules as specs for Approve, Ask for an agent
review and issues. There is no hold for plans: DEC-006 holds specs only.

**Acceptance:** with the hold on for one send, the reviewer's approval leaves
the spec `reviewing` and held, and *let the reviewer decide* approves it and
lets the plan be written; direct approval approves it; with agent review off,
submission queues no review, *let the reviewer decide* is refused, and a
requested review runs once and is held; no test configuration approves a spec
without a reviewer or a person.

### FR-6: Human issues

**FR-6.1 — Raising one.** A person may raise an issue on a spec or plan whose
feature is `idea` or `ready`, or on a design in `draft` or `reviewing` (SD-14). Building has started otherwise, and the answer is
a revision, which the document page offers. An issue has a body, an optional
section, the channel it came by (`ui` or `mcp`), and, by relay, the human's
quoted words. It is stored in `document_comments` with `is_issue` set
(migration `0007` adds `is_issue`, `via`, `quote`, and the four
`addressed_*` columns).

**FR-6.2 — Where it goes** (DESIGN-010 §5):

| The document | The feature | What happens |
|---|---|---|
| `draft` | sent | Recorded, and the author is dispatched to revise the draft (FR-3.1's dispatch, unless one is already queued or running). |
| `reviewing` | sent | Recorded, and the document goes back to `draft` with the issue (a person's send-back). Queued reviews are cancelled. FR-3 then runs. |
| `approved` | sent | A successor draft is opened, the issue is recorded on it, and the author is dispatched to fill it (SD-7). |
| any of those | not sent | Recorded only. For an approved document a successor draft is opened to carry it. Nothing is dispatched. |

**FR-6.3 — How it is addressed** (SD-6). The reviewer's prompt lists every open
issue with its id, words and author, before the document. The reviewer
answers each in `submit_review`'s `issues` list. An `approve` that leaves an
issue unanswered, or answered `not_addressed`, is rejected inside the turn with
a sentence naming the issue. A `request_changes` may answer some and leave
others open.

**FR-6.4 — A verdict can't jump an issue.** An agent approval applied to a
document whose open issues it didn't answer (because an issue arrived while the
reviewer was working) is not applied. A fresh review is queued instead, for a
sent or an unsent feature alike: the review was already running at someone's
request, and this completes it honestly rather than starting new work.

**FR-6.5 — The record.** The document page shows each issue with who raised
it, the channel and the quote, and, once addressed, the reviewer's answer.

**FR-6.6 — When the successor is approved.** Approving a successor spec whose
feature is `idea` or `ready` supersedes the predecessor as today and, in the
same transaction, supersedes the feature's approved plan (FR-9.4a's rule) and
returns a `ready` feature to `idea`. The plan invariant then rewrites the plan
for a sent feature. Approving a successor plan on a `ready` feature
re-decomposes it (`ReDecomposeDevPlan`, which reconciles tasks by local id) and
re-checks the estimate.

**FR-6.8 — Detach keeps the record.** Detaching a draft with open issues is
allowed, behind a confirm step that says so, and the `document.detached` audit
row carries each dropped issue's words, so none disappears silently.

**FR-6.7** Start building refuses while a feature's current spec or plan is not
approved: "This feature's specification is being revised, so building can't
start until the revision is approved."

**Acceptance:**
- an issue on a sent feature's reviewing spec sends it back, the author's
  revision prompt carries the issue, and the reviewer's approval without an
  answer is rejected in its turn, then accepted with one;
- an issue on an approved spec of a ready, sent feature opens a successor that
  the author fills; its approval supersedes the old spec and plan, returns the
  feature to `idea`, and writes a new plan;
- an issue on an unsent feature's spec dispatches nothing;
- an issue on an active feature's spec is refused;
- direct approval settles open issues and says so.

### FR-7: Estimation ends the chain

**FR-7.1** The estimate is part of the invariants (FR-2.2a): when a sent
feature's plan is approved and decomposed, or a feature with an approved plan
is sent, the orchestrator queues one estimate for the feature. An unassigned
role is silence, as for the authoring purposes.

**FR-7.2** The feature reaches `ready` by G1, as today, and waits for Start
building. Start building doesn't wait for the estimate: the estimate informs
the person's decision, and it isn't a gate.

**Acceptance:** the mock-provider run from Send ends with the feature `ready`,
tasks decomposed, one estimate recorded, and no implementation dispatch.

### FR-8: The chat relay tools

**FR-8.1** Four tools, in `internal/server/mcp_relay.go`, each over the same
service method the document page calls:

| Tool | Arguments | Does |
|---|---|---|
| `relay_verdict` | `path`, `verdict` (`approve` or `send_back`), `reason` (for a send-back), `quote` | A person's verdict on any document in `reviewing`. A send-back of a spec or plan is recorded as a human issue (FR-6). |
| `relay_issue` | `path`, `issue`, optional `section`, `quote` | FR-6.1. |
| `relay_review_request` | `path`, `quote` | FR-5.5's *Ask for an agent review*. For a design, refused unless the project still has a design reviewer (SD-8), with advice to use the `review-design` chat skill. |
| `relay_release_hold` | `path`, `quote` | FR-5.5's *Let the reviewer decide*. |

**FR-8.2 — The quote.** Every tool requires `quote`: the human's words, as they
said them. An empty or missing quote is refused. The quote and `via: mcp` are
stored on the audit row, and on the issue for `relay_issue`.

**FR-8.3 — The boundary.** No relay can send, withdraw, start building, lock,
override a gate or answer a checkpoint (SD-11). The chat agent has no tool that
holds a verdict of its own: every relay's description says it carries a
person's decision and must quote them. `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
names the four, and its must-not-exist list adds `send_to_development`,
`withdraw_send`, `start_building` and `answer_checkpoint`.

**FR-8.4** The `initialize` instructions say the agent may carry a person's
verdicts, issues, review requests and releases, quoting them, and that sending
and starting building are a person's acts in the web UI.

**FR-8.5** The `initialize` instructions no longer say the agent "can't run an
agent": a relayed review request does queue one. They say instead that it can't
send, start building, lock or answer the Inbox.

**Acceptance:** over `POST /mcp`, each tool works on its happy path and leaves
an audit row with `via: mcp` and the quote; a missing quote, a draft verdict, an
escalated document, an issue on a document under a pending question, and a
release with agent review off each fail with a sentence; the advertised set is
exact.

### FR-9: Submit, Revise and Detach on the document page

**FR-9.1 — Submit** (a `draft`): runs `SubmitDoc`. A validation failure is
shown on the page, issue by issue, and the document stays in `draft`.

**FR-9.2 — Revise** (an `approved` document with no live successor): runs
`ReviseDoc` and opens the successor's page, which says where the working copy
is and that the original stays approved until the revision is.

**FR-9.3 — Detach** (a `draft` only), behind a confirm step: removes the
document's registration (its row, sections, comments and any hold), leaves the
file on disk, and audits `document.detached` with the path, type and owner. A
detached spec or plan on a sent feature lets the invariants re-check. An author
agent never writes over a file it didn't register: when the conventional path
is taken by an unregistered file, the authored document goes beside it with a
numbered name (R11-10).

**Acceptance:** each action works from the page in the browser; Detach on a
non-draft and Revise on a revised document are refused with sentences.

### FR-10: Retiring the design reviewer

**FR-10.1** The starter pack drops `roles/design-reviewer.yaml` and
`skills/review-design/`, and the design manifest drops `reviewer_role`.

**FR-10.2** A manifest's `reviewer_role` becomes optional where `approved_by:
human`, and stays required where the agent approves. A submitted document with
no reviewer role queues no review: it waits in `reviewing` for a person, with
no agent comments.

**FR-10.3** A project that still names a design reviewer keeps it (SD-8).

**FR-10.4** The skill's content ships at `chat-skills/review-design/SKILL.md`
(SD-9), reworded for a chat AI reading a design with a human.

**Acceptance:** a fresh project's submitted design queues no dispatch and
waits for a person; a project with the old manifest still gets comments; the
pack no longer ships the role.

### FR-11: The starter pack and the second button

**FR-11.1** The generated `config.yaml` assigns `write-spec: spec-author` and
`write-dev-plan: dev-plan-author`, uncommented, with a comment saying nothing
runs until someone presses Send to development. It also writes the
`spec_review` block with its defaults and a comment.

**FR-11.2** Gate 2's button and its blocked card read **Start building**. Its
reasons say "Building can start once…".

**Acceptance:** the render test finds "Start building" and not "Start work";
a fresh `init` has the two assignments live.

## 4. Non-functional requirements

- **NFR-1 — One service layer.** The UI handlers and the relay tools call the
  same service methods, in-process, each in one transaction with its audit
  row (O-3).
- **NFR-2 — The orchestrator stays code.** Every dispatch this spec adds is
  decided by the rules engine or the reconciler, never by an agent.
- **NFR-3 — The seam holds.** Only a `POST` from the web UI writes the sent
  mark. A test asserts no MCP tool, and no `/api/*` route, can. Starting
  building is not reachable over MCP, as before; its API route is SD-16's
  recorded departure.
- **NFR-4 — No unreviewed specs.** A test runs every combination of the two
  settings and the per-send hold, and asserts no spec reaches `approved`
  without an agent approval or a person's.
- **NFR-5 — Human prose** in every label, reason, refusal, checkpoint question
  and tool description (D-6). No currency anywhere.
- **NFR-6 — Contained templates.** New markup lives in its own partial files,
  `send.html` (the send screen and the feature's send card) and `review.html`
  (the document page's actions and review panel). `entity.html`, which also
  holds the document page, gains only the includes, the issue marks on
  comments, and the renamed label.
- **NFR-7 — Coordination.** Migration `0007` only. No change to
  `internal/dispatch/`: the review outcome tool is defined there, and the
  `issues` field is added in `internal/server` by extending a copy of its
  schema; the payload is parsed in `internal/rules`, as before.
- **NFR-9 — No typed paths.** The send screen and the document page's new
  forms carry ids in hidden fields, and the typed-path scan covers them.
- **NFR-8 — Tested as before.** Integration tests with the mock provider
  against real Postgres cover every FR. `go vet ./...` and
  `go test -race -count=1 ./...` clean. The SPEC-009 authoring and cascade
  tests pass, adjusted to send before they expect a dispatch.

## 5. Definition of done

1. Every FR's acceptance passes in the suite.
2. `go vet ./...` and `go test -race -count=1 ./...` are clean.
3. **A browser walkthrough without an AI provider**, with Playwright and the
   pre-installed Chromium: approving a design dispatches nothing; the Send
   button refuses with a reason and then works; the send screen renders;
   Submit, Revise and Detach work; "Start building" is labelled. Screenshots
   and `docs/walkthrough-spec-011.md`.
4. **A live smoke checklist for Sam** in the handoff, using
   `scripts/smoke-project.sh`: approve a design and see nothing start; press
   Send; watch spec, review, plan, review and estimate; see it stop at Start
   building. Then the cascade smoke (SPEC-009 DoD 3): revise the design,
   re-approve, and watch the sent feature's spec superseded and rewritten.
5. A handoff note, `docs/notes/handoff-M3-2026-09-28.md`.
6. The roadmap's §11 marks M3 done with a pointer to the handoff.
7. **Eight choices need Sam's explicit yes:**
   1. Withdraw is built, small (SD-10);
   2. the dev-plan invariant and the estimate need the mark too, a `ready`
      feature can be sent (SD-2), and building counts as sent (SD-3);
   3. the hold catches approvals only (SD-5);
   4. existing projects keep their design reviewer (SD-8);
   5. how an issue is marked addressed (SD-6), and that an issue on an
      approved spec opens a successor rather than retiring it at once (SD-7);
   6. Start building stays reachable from the CLI and API until DEC-003
      retires them (SD-16);
   7. issues on approved plans are supported, with re-decomposition (SD-14);
   8. a "send back" of a spec is a human issue plus a return to draft
      (SD-15).

## 6. Open questions carried forward

- **A settings page.** `spec_review` lives in `config.yaml`, which a person
  edits by hand. A web settings page is a later, wider question.
- **The estimate is of the whole feature**, not its tasks. Estimating each task
  separately would give a decomposed tier; the estimator's single feature
  estimate is enough for Start building's decision today.
- **Issues on documents other than specs, plans and draft designs** (research
  notes, decisions) aren't handled. Nothing reviews them.
- **Review findings are never marked resolved.** A reviewer's findings pile
  up across rounds, and the author is told some may be dealt with already. A
  reviewer that could resolve the previous round's findings would give a
  cleaner thread; it is a C-1 follow-up.
- **Start building can be pressed before the estimate lands** (FR-7.2).
- **A feature part-way through the July chain when M3 lands** — spec
  approved, plan not written — waits until someone sends it. That is DEC-006's
  intent; the handoff says so.
- **The review round cap is shared** with code review
  (`dispatch.max_review_rounds`). A separate setting may be wanted once the
  live runs show how often documents loop.

## 7. Changes after review

How each [REVIEW-011](../reviews/REVIEW-011-send-to-development.md) finding was
dealt with.

| Finding | What changed |
|---|---|
| R11-1 (material) | SD-4 and FR-2.1/2.2 make the revise loop part of the invariants: a draft that waits for its author (open issue or major finding) doesn't satisfy them, so Send, the sweep and every re-check pick it up. FR-2.4 reads the sweep's boundary for revisions. FR-4.3 defines "already done". |
| R11-2 (material) | The header lists SPEC-009 FR-2.3 and FR-2.4 as changed, and FR-5.5 says direct approval is new authority for agent-approved types, from Amendment 1 decision 5. |
| R11-3 (material) | NFR-3 now covers the sent mark only. SD-16 records that the CLI and API can still start building, as a departure from DESIGN-010 §5 for Sam (DoD 7.6). |
| R11-4 (material) | SD-11 now refuses every person's act, from either surface, on a document with a pending escalation, authoring-deadlock or design-revision question. FR-5.5 and FR-8's acceptance say so. |
| R11-5 (material) | FR-6.6 re-decomposes a successor plan approved on a `ready` feature and re-checks the estimate. SD-14 no longer says "at no extra cost". Sam is asked (DoD 7.7). |
| R11-6 | FR-3.4 says an issue on a sent feature's hand-written draft does go to the author, as DESIGN-010 §5 says. FR-3.6 refuses Submit and a review request while the author is at work. |
| R11-7 | FR-6.1 includes design drafts; FR-6.4 explains the re-review for unsent features; SD-6 says an escalation approval settles issues; FR-6.8 records dropped issues on Detach; FR-5.4a says any return to draft clears a hold. |
| R11-8 | SD-15 defines a send-back of a spec, and FR-5.5 adds its UI twin. Sam is asked (DoD 7.8). |
| R11-9 | FR-4.3 warns on the send screen when a step's role isn't assigned. |
| R11-10 | FR-9.3: an author never writes over an unregistered file. |
| R11-11 | NFR-7 says where the schema lives and where the field is added. |
| R11-12 | SD-7 says the successor is the current document at once, and why that blocks Start building. |
| R11-13 | Built: the Inbox offers retry and cancel on `authoring-deadlock`, and the rules engine acts on retry. |
| R11-14 | FR-5.4a specifies `document_holds`. NFR-6 names `review.html` and what `entity.html` gains. |
| R11-15 | FR-2.2a and FR-7 make the estimate part of the invariants, owed after a send or a new plan; FR-4.3 defines its "done"; FR-7.2 and §6 say Start building doesn't wait for it. |
| R11-16 | FR-2.3's table adds the plan's approval, issues on drafts, Detach, the deadlock retry, Withdraw and a settings change. |
| R11-17 | FR-3.5 drops a verdict whose content has changed since its review was queued. |
| R11-18 | FR-8.5 corrects the `initialize` text. §6 records that findings pile up. |
| R11-19 | The status line points here, and the Goal mentions the hold. |
| §4 notes | FR-4.3's reviewer sentence reads `approved_by`; NFR-9 adds the typed-path scan; the snapshot carries held and open issues so rules stay pure; FR-2.3 says when settings are read; the upgrade case is in §6 and the handoff. |
