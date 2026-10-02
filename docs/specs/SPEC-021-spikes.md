# SPEC-021: Spikes

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. An independent
consistency review is recorded in
[REVIEW-021](../reviews/REVIEW-021-spikes.md). It found nine material and ten
smaller problems in the first draft. All are dealt with in this revision, and
§8 says how, finding by finding. The choices Sam must confirm are listed in
§6, DoD 10. The build does not wait for approval (the M14 brief).
**Stage 2 revision (2026-10-02):** §4 is rewritten as a full stage-2 section
(SD-17 to SD-25, FR-11 to FR-18) on top of M13's SPEC-020, now on `main`. It
adds choices 15 to 21, and is reviewed in REVIEW-021 §6.
**Date:** 2026-10-02
**Roadmap milestone:** M14 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11. Roadmap decision 13 (§12) is part of the brief: **a hard stop, with a
project-wide default budget** that a spike can override.
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28). **§10 is the main source.** Also:
- §4: a spike is "a question with a budget";
- §5b: doing and judging stages;
- §7: the `SPK-` prefix and the default folder;
- §8: tokens, and unmeasured work;
- §11: decisions pushed into every dispatch;
- **§17a item 7, accepted**: a human starts a spike in the web UI, any
  executor runs it, and a chat or human spike carries a time box instead of
  a token budget.

**Authority:**
- [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md): the chat agent
  may author planning structure, so it may create a spike;
- [DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1:
  acts that commit resources stay in the web UI;
- [DEC-007](../decisions/DEC-007-the-judgement-boundary.md): running a spike
  is a doing stage, so any executor may do it.

**Builds on:**
- [SPEC-011](SPEC-011-send-to-development.md): the web-UI-only button and the
  send screen;
- [SPEC-012](SPEC-012-see-the-work.md): transcripts, runs and the token
  ledger;
- [SPEC-015](SPEC-015-documents-with-identity.md): IDs, the `SPK` sequence
  (already made), and document identity;
- [SPEC-018](SPEC-018-decisions.md): decisions surfaced into every dispatch;
- [SPEC-019](SPEC-019-bugs.md): how M12 added an entity. A spike is **not**
  feature-shaped, and this spec says why (SD-1).

**Coordination:** M13 (executors) is built in parallel and merges first. It
owns migration `0014`, `claim_task`, `submit_task`, the executor on tasks,
claim expiry, the unclaimed-commit check and unmeasured tokens. This spec owns
migration `0015`. NFR-6 lists every shared file this spec touches and where
M13 is likely to touch the same one. **Stage 2 (§4) is built only after M13
has merged into `main`.**

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6). British spelling.

Five words are kept apart throughout:

- a **spike** is the entity, `SPK-003`;
- its **question** is the one sentence it exists to answer;
- its **draft** is the findings text the run has saved so far, held on the
  spike's row while the run is live;
- its **findings** are its one document, `SPK-003-findings`, written from the
  draft when the run ends;
- its **run** is the dispatch that does the work, purpose `run-spike`.

A run **concludes** when its agent finishes with an answer, or with a reason
the question can't be answered. A spike is **answered** only when a person
says so. The two are never the same word.

A spike's **budget** is a number of tokens. "Tokens used" always means every
token the provider reported for the spike's model calls: input (which
includes cache reads and writes) plus output. That is the sum the run pages
already show (`RunSummary.Tokens`).

## 0. Framing

Sometimes a design can't be written because nobody knows enough yet. Does the
API do what its documentation says? Is the approach fast enough to bother
with? DESIGN-010 §10 answers with a spike: a question with a budget, which
produces findings and never shipped code.

Today, the only way to send an agent to find something out is to make a
feature. That commits scope, writes a spec, and builds and merges code, which
is the wrong shape. A spike has no spec, no plan, no tasks and no merge. It
has a question, a budget, a throwaway working copy and one document.

So this spec adds a small, separate entity:

1. A person, or the chat agent, writes down a question as a spike on an
   initiative or a feature.
2. A person starts it from the web UI, after seeing what will run and the
   budget.
3. One agent works on the question in a detached worktree, and saves its
   findings as it goes.
4. The run stops hard at the budget. When it ends, for whatever reason, the
   findings are written up as a document and committed, and the worktree is
   thrown away.
5. A person reads the findings and closes the spike, as answered or not. If
   it wasn't answered, they may ask again with a second, separately budgeted
   spike.

## 1. Goal

**One claim, which the roadmap's done-when states directly:**

> A spike hits its budget and stops. Subutai has no path that merges a
> spike's code, and a run that tries to keep its code is caught.

Four supporting claims:

> Only a person, in the web UI, can start a spike. The start screen shows what
> will run and the budget before anything is committed.

> When a spike's run ends — concluded, at its budget, at its turn limit or by
> failing — its findings are written up and committed, and its worktree is
> discarded. A restart at any point doesn't leave it stuck.

> A spike is done when a person reads its findings and says the question is
> answered. The findings are an ordinary document.

> The spike's agent is told the project's decisions, like every other
> dispatch.

## 2. Scope

### In scope (stage 1, built now)

1. The spike entity, its states and its ID (FR-1).
2. The `findings` template and document (FR-2).
3. Starting a spike, in the web UI only (FR-3).
4. The run: the prompt, the tools and the worktree (FR-4).
5. The budget and the hard stop (FR-5).
6. Ending a run: the write-up, the leak check, the discarded worktree, and
   reconciliation (FR-6).
7. Closing a spike, and asking again (FR-7).
8. Where spikes show: pages, the timeline, runs and the Inbox (FR-8).
9. MCP: create, list and get (FR-9).
10. The starter pack and configuration (FR-10).

### Stage 2, built after M13 merged

11. Spikes run by the chat AI or a person, held to a time box through their
    claim, with unmeasured tokens (§4, FR-11 to FR-18).

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| A spike as a milestone deliverable | Not planned. A spike ships nothing (SD-11). |
| Stopping a running spike by hand | §7. The budget is the stop in stage 1. |
| Withdrawing a start before the run begins | §7. Admission is immediate when a slot is free. |
| Editing a spike's question after it is created | §7. Close it and create another (SD-4). |
| Promoting a spike into a feature | Never. Promotion is one-way, through a design (DESIGN-010 §10, SD-12). |
| Spikes owned by the project itself | §7. A spike hangs off an initiative or a feature, as §10 says. |
| A chat relay for closing a spike | §7. Closing is a person's act in the web UI in this milestone (SD-10). |

### Scope decisions

- **SD-1 — A spike is its own table, not a feature row.** M12 made a bug a
  `features` row because everything after a bug's acceptance *is* the feature
  pipeline (SPEC-019 SD-1). A spike shares none of it: no spec, no plan, no
  tasks, no estimate, no gates G1 to G3, no merge, and no milestone
  membership. Putting it in `features` would mean filtering it out of every
  one of those places, and each missed filter would be a path to building or
  merging a spike. A separate table, `spikes`, has none of those columns or
  relationships, so the feature code can't act on it by accident. This is the
  first half of SD-2.

- **SD-2 — No merge path: none in Subutai, and a leak is caught.**
  DESIGN-010 §10: "There is no merge path in the entity at all", "enforced,
  not promised". Five facts make that true, and each is tested:
  1. **The worktree is detached** (`git worktree add --detach`). Subutai
     makes no branch for it, so there is nothing to merge, push or open a
     pull request from.
  2. **Nothing in the spike's code calls a merge.** The merge
     (`mergeFeature`, `mergeBranch`) takes a feature and reads the
     `worktrees` table. A spike's worktree is recorded on its own row, not in
     `worktrees`, so no feature query can find it.
  3. **No rule emits a merge for a spike**, and no route, API or MCP tool
     exists that could ask for one.
  4. **The worktree is removed** when the run ends (FR-6), with `--force`.
     Whatever the agent left there, committed or not, is gone, and a commit
     on the detached `HEAD` is unreachable afterwards.
  5. **A run that keeps its code is caught.** The agent can run code it wrote
     through the project's whitelisted commands, and that code could create a
     branch, which would outlive the worktree because a linked worktree
     shares the repository's refs. So `EndSpike` checks, before the discard,
     whether any ref holds a commit made in the worktree (FR-6.3). If one
     does, it says so in the findings, audits it, and raises a checkpoint
     naming the ref. Subutai doesn't delete the ref itself, because a person
     should see what happened.

  The findings are the only thing that leaves the run. The server writes
  them to the main checkout from the draft on the spike's row; nothing is
  ever read back from the worktree (FR-4.3, FR-6.2).

- **SD-3 — A spike hangs off an initiative, and optionally a feature.** As a
  bug does (SPEC-019 SD-3), every spike has an initiative: the one it was
  created on, or the initiative of the feature it was created on. The feature,
  when there is one, is its **feature**. Its findings live in that
  initiative's folder, and its page is reached from the owner's.

- **SD-4 — Four states, two ways to close, and a fixed question.** A spike
  is:
  - `idea`: written down, not started;
  - `running`: started, with its run queued or running;
  - `ended`: the run is over, the findings are committed, the worktree is
    gone, and it waits for a person;
  - `closed`, either `answered` or `unanswered`: a person has read it.

  An `idea` can also be closed, as `unanswered`, without running. `closed` is
  the only terminal state, and nothing reopens it. The question can't be
  edited after creation: it is what the budget was spent on, and a different
  question is a different spike. A wrongly worded idea is closed and created
  again. **Flagged for Sam.**

- **SD-5 — Started in the web UI only, like Send to development.** Starting
  spends tokens, so it is a person's act at the command centre (§17a item 7,
  DEC-006). There is a start screen and a `POST`. There is no `/api` route,
  CLI verb or MCP tool that starts a spike. The MCP tool-set test names
  `start_spike` and `run_spike` as forbidden.

- **SD-6 — The budget counts every token, is fixed at the start, and has a
  default of 1,000,000.** A spike's **budget** is set when it is started:
  the per-spike override if it has one, otherwise
  `spikes.default_token_budget` from `config.yaml` (roadmap decision 13). The
  start screen shows the figure and lets the person change it for this start.
  It is recorded on the row, and the config isn't read again for that run.

  **The arithmetic** (REVIEW-021 R21-6). Every model call re-sends the whole
  conversation, so tokens used grow with the *square* of the turns. The M6
  demo's scripted turns read 6,410, then 7,180, then 8,020 input tokens
  (`demo_m6_test.go`). A spike that starts with about 6,000 tokens of prompt
  and adds about 1,500 a turn uses about 230,000 tokens in 15 turns,
  590,000 in 25, and 1,000,000 in about 33. So:
  - **the default is 1,000,000 tokens**, which is roughly 30 to 35 turns of
    a modest spike, and about three US dollars on Sonnet at today's prices;
  - **the role's turn cap is 40**, so a spike with small turns can't run
    for ever below its budget.

  The provider doesn't use prompt caching today, so cache reads are near
  zero and "input and output" is the whole figure. A truly different measure
  would count only *new* tokens per call (output, plus each call's growth in
  input), which grows in a straight line. It is fairer to long spikes, but
  doesn't match the ledger or the bill. **Flagged for Sam**, with that as
  the alternative.

- **SD-7 — The stop is hard, and never begins a call it can see would cross
  the budget.** The dispatcher keeps the spike's running total in the
  database, adding each call's usage as soon as the call returns, and
  compares the **database's** total, so overlapping attempts see each
  other's spending (R21-4). It stops:
  - before any call, once the total has reached the budget;
  - before any call after an attempt's first, if the total plus a **lower
    bound** for the next call would pass the budget. The next call's input
    holds everything the last call read and wrote, so the lower bound is the
    last call's input plus its output.

  A call can still take the run past its budget by what it adds on top of
  that bound: the tool results fed back to it, and what the model writes. The
  overrun is bounded by one call and shown honestly, as "It used 41,210
  tokens, which is more than its budget of 40,000." No question is asked: the
  run simply ends (DESIGN-010 §10). Reaching the role's turn cap ends the run
  the same way, as "stopped at its turn limit", not as a failure. **Flagged
  for Sam.**

- **SD-8 — The agent saves a draft as it goes, on the spike's row.** A hard
  stop can't ask the agent to write up, because that would spend more
  tokens. So the agent has a `save_findings` tool and is told to use it early
  and often. Each call replaces the spike's **draft**, a column on its row.
  It doesn't touch any file (R21-1). When the run ends at its budget, its
  turn limit or by failing, **the findings so far are the draft**, and the
  server completes them:
  - an empty or missing Answer becomes "Not answered: the spike stopped at
    its budget before it reached an answer." (or the matching reason);
  - an empty or missing What we found becomes "Nothing was saved before the
    run stopped.";
  - the closing section, **How this spike ended**, gives the reason, the
    tokens used and the budget.

  A run that concludes calls `finish_spike` with its final findings, which
  are validated against the template first. **Flagged for Sam:** the
  alternative, a final no-tools turn to write up, breaks the hard stop.

- **SD-9 — The findings are created when the run ends, and have the ordinary
  lifecycle with a person as approver.** `findings` is a document type with
  its own template, owned by the spike, `SPK-003-findings`. It is created
  **when the run ends**, from the draft, and committed. Before then the draft
  is on the spike's row, where the page shows it. So nothing can submit,
  approve, edit or review a half-written document while the agent is still
  writing it (R21-1). DESIGN-010 §7's "creating work creates its documents"
  names initiatives and features; a spike's one document is what its run
  produces, so it is created by the run. A spike closed without running has
  no findings. The manifest says `approved_by: human` and names no reviewer
  role, as `design`'s does: the findings are read by the person who decides
  whether the question is answered, and no agent review is dispatched. Once
  written, they can be edited in the browser, submitted, approved, sent back
  and superseded like any other document. **Flagged for Sam.**

- **SD-10 — A person closes the spike, and closing doesn't approve the
  findings.** The spike is done when a person reads the findings and says the
  question is answered (§10). On an ended spike the page offers:
  - **The question is answered**: closes the spike as `answered`;
  - **Close without an answer**: closes it as `unanswered`;
  - **Ask again with a new budget**: see SD-13.

  The findings keep their own lifecycle: a person approves them, or not, on
  the document page, as with any document. Tying approval to closing would
  need a submission and an approval in separate transactions, and gains
  nothing the document page doesn't already do (R21-8).

  Closing is in the web UI only in this milestone. The chat agent may read a
  spike and help a person decide, but it doesn't close one, even with quoted
  words: that would be a new relay, and DEC-006 Amendment 1 requires a
  decision for each. The MCP tool-set test names `close_spike` and
  `answer_spike` as forbidden. **Flagged for Sam:** the alternative is a
  `relay_close_spike` tool with the person's words.

- **SD-11 — A spike is not a milestone deliverable.** It ships nothing, and
  a milestone is a collection of things that ship together (DESIGN-010 §4).
  `add_milestone_member` and the milestone editor refuse a spike with a
  sentence. A spike's findings can still be cited by a design that leads to
  a deliverable.

- **SD-12 — Promotion is one-way, through a design.** No action turns a
  spike into a feature, copies its worktree, or carries its code anywhere.
  A closed spike's page says what to do instead: "To build on this, cite
  SPK-003-findings in a design, then create a feature in the normal way."

- **SD-13 — Asking again makes a new spike.** On an ended spike, **Ask again
  with a new budget** closes it as `unanswered`. It then creates a new spike
  with the same owner and question, a link back (`follows`), and the budget
  the person enters as its override, and redirects to the new spike's start
  screen. Nothing is started until the person presses Start there. The new
  run's prompt includes the earlier findings, so it starts from what is
  already known. It is a separate spike, with its own number, budget, findings
  and worktree (roadmap decision 13).

- **SD-14 — Open spikes block archiving their initiative.** G5 refuses to
  archive an initiative with open work under it. A spike that isn't `closed`
  is open work: a running one is spending tokens, and an ended one waits for
  a person. So G5 counts open spikes in the same subtree it counts features
  in, and its refusal names them. A G5 override (an answered gate-override
  checkpoint) leaves spikes as they are: a running one finishes, and an ended
  one can still be closed from its page. Abandoning a feature doesn't touch
  its spikes: a running spike finishes and is read like any other.
  **Flagged for Sam.**

- **SD-15 — One run per spike, and its attempts build on each other.** The
  run is one dispatch row, with idempotency key `run-spike:<spike id>`. Its
  attempts share the worktree, the budget and the draft. An attempt after the
  first is told what has been saved so far (FR-4.1), so it builds on it
  rather than overwriting it (R21-5). When the dispatcher gives up on the run
  (exhausted), it has failed. It ends as in FR-6, with no "Retry or cancel?"
  question (R21-2).

- **SD-16 — The run has its own role.** The `run-spike` purpose is assigned
  to a new role, `spike-runner`, with its own skill, `run-spike`. Its tools are
  the implementer's file and command tools, plus `save_findings`, with
  `finish_spike` as its outcome. It doesn't get `report_bug`: a spike is
  finding something out, and what it finds goes in its findings. The role's
  model is Sonnet by default, as the implementer's is; a project changes it in
  the role file or with `routing`. A project without the role assigned can't
  start a spike, and the start screen says so.

## 3. Requirements (stage 1)

### FR-1: The spike entity

**FR-1.1 — The table.** Migration `0015_spikes.sql` adds `spikes`:

| Column | What it holds |
|---|---|
| `id` | uuid, the primary key |
| `public_id` | `mint_ident('SPK')`, unique, not null |
| `initiative_id` | the owning initiative, not null |
| `feature_id` | the feature it was created on, or null |
| `question` | the question, non-blank, at most 500 characters |
| `state` | `idea`, `running`, `ended` or `closed` |
| `budget_override` | tokens, or null for the project default |
| `token_budget` | tokens, set at the start (SD-6), null before |
| `tokens_used` | tokens, 0 until the run starts, added to after every call |
| `draft` | the findings text saved so far, or null |
| `draft_saved_at` | when the draft was last saved |
| `ended_how` | `concluded`, `budget`, `turn_limit` or `failed`, set when the run ends |
| `end_note` | for `failed`, the last error as one line |
| `closed_as` | `answered` or `unanswered`, set when closed |
| `follows_id` | the spike this one asks again for, or null |
| `base_commit` | the commit the worktree was made from |
| `refs_at_start` | every ref and its commit when the spike started (FR-6.3) |
| `worktree_path` | the worktree's path relative to the repository, set at the start |
| `worktree_removed_at` | when the worktree was discarded |
| `created_by`, `created_via` | who created it, and `ui` or `mcp` |
| `started_by`, `started_at`, `ended_at`, `closed_by`, `closed_at` | who and when |

Checks the database enforces:
- `closed_as` is set exactly when the state is `closed`;
- `ended_how` is set exactly when the state is `ended`, or `closed` after a
  run (`started_at` is set);
- `token_budget` and `started_at` are set whenever the state is `running` or
  `ended`;
- a spike closed without a run (`started_at` null) is `unanswered`;
- budgets are positive, and `tokens_used` isn't negative;
- a spike doesn't follow itself.

Stage 2 adds an executor and a time box (§4), and its migration replaces the
checks above that assume a token budget. Stage 1's checks are named, so they
can be dropped by name.

`ref_type` gains `spike`, for dispatches, audit rows, checkpoints and document
owners. `document_type` gains `findings`. The documents table's owner check
(unnamed in `0001`, so dropped by its generated name) is replaced by a named
check that admits `spike`. Postgres refuses to use an enum value in the
transaction that added it, so the new check compares `owner_type::text`, and
no row in the migration uses the new values.

**FR-1.2 — IDs, and every place that switches on an owner.** `ident.Kinds`
gives `spike` its table, `spikes`. `ident.DocTypes` gains `findings`. A
spike's ID resolves wherever an entity's does: `/ui/id/SPK-003` and
`get_spike`. Appendix A lists every place that switches on an owner or
reference type and what each does for a spike (R21-13).

**FR-1.3 — Creating a spike.** One service method, `CreateSpike`, serves the
web UI and MCP, in one transaction with its audit row (`spike.created`):
1. it resolves the owner, which must be a live initiative or a feature that
   isn't `done` or `abandoned`, and refuses otherwise with a sentence;
2. it validates the question (non-blank after trimming, at most 500
   characters) and the override (a positive whole number, if given);
3. it inserts the row and writes the audit row;
4. after the commit, it signals the owner's page.

Every refusal happens before a number is minted. A person creates a spike
from a **New spike** dialog on an initiative's or a feature's page, with the
question and an optional budget. The chat agent creates one with
`create_spike` (FR-9).

**FR-1.4 — Closing from `idea`.** An `idea` spike's page has **Close without
running**, which closes it as `unanswered`, with no run, no worktree and no
findings.

**Acceptance:** `TestCreatingASpike`:
- creating from the UI on an initiative and on a feature, and from MCP,
  gives `SPK-001`, `SPK-002` and `SPK-003` in order, with the owner, the
  audit row and `created_via`;
- a blank question, a 501-character question, a zero or negative override,
  a `done` feature and an archived initiative are each refused with a
  sentence, and the next spike still gets the next number;
- the database refuses a row that breaks each named check;
- closing an `idea` spike leaves it `closed`, `unanswered`, with no findings.

### FR-2: The findings document

**FR-2.1 — The template.** `templates/findings/` in the starter pack:

```yaml
type: findings
approved_by: human
front_matter:
  required: [title, type, owner]
sections:
  order: strict
  required:
    - heading: Question
    - heading: Answer
    - heading: What we found
  optional:
    - heading: How we found out
    - heading: What to do next
    - heading: How this spike ended
```

The template's Question is filled from the spike. The title is "SPK-003:
<the question, shortened to 80 characters>". The draft holds the sections
after Question; the server always writes Question itself, from the row, and
drops any Question section in the draft.

**FR-2.2 — Writing the findings.** `writeFindings` builds the file from the
template's front matter, the question, the draft (or `finish_spike`'s body),
SD-8's fill-ins, and How this spike ended. It writes the file at
`docs/work/<INIT-ID>-<slug>/SPK-003-findings.md` (or the next free name),
registers it as a draft owned by the spike, marks it the spike's primary
document, and records `subutai` as its writer. It is used only by `EndSpike`.

**FR-2.3 — How this spike ended.** This section is the server's. It is
written last, replacing any section with that heading in the draft:

> The agent reached a conclusion. It used 31,004 of its 40,000 tokens.

> The spike stopped at its budget. It used 41,210 tokens, which is more than
> its budget of 40,000. The findings above are what it had saved by then.

> The spike stopped at its turn limit of 40 turns, having used 18,220 of its
> 40,000 tokens. The findings above are what it had saved by then.

> The spike's run failed: <the last error, as one line>. It used 2,100 of its
> 40,000 tokens. The findings above are what it had saved by then.

When FR-6.3's leak check finds a ref, a paragraph follows: "Code from this
spike was kept on `keep-this`, outside its working copy. Subutai hasn't
deleted it; the Inbox asks what to do."

**Acceptance:** `TestWritingFindings` builds findings for each ending, from a
full draft, an empty draft and a draft with its own How this spike ended and
Question sections. Each result validates against the manifest, holds the
section once and last, and has the question from the row.

### FR-3: Starting, in the web UI only

**FR-3.1 — The button.** An `idea` spike's page shows **Start this spike…**
as its primary action, linking to the start screen. Nothing else on any page
starts a spike.

**FR-3.2 — The start screen** (`GET /ui/spikes/start?spike=<uuid>`) shows,
before anything is committed, in the send screen's style:
- the question, and the owner;
- **who will run it**: "The spike runner runs it, on *model*.", from the
  `run-spike` assignment and the model it would be dispatched on. When
  `run-spike` isn't assigned: "Nobody is assigned to run spikes, so this
  spike can't start. Assign `run-spike` to a role in `config.yaml`." and the
  button is disabled;
- **the budget**, as a number field, filled with the override, or with the
  project default and the line "This is the project's default budget, from
  `spikes.default_token_budget`.";
- what happens at the budget: "The run stops when it reaches this many
  tokens. Whatever findings it has saved by then are written up, and you
  decide whether to ask again.";
- **what the agent will be given**: the question; the decisions and
  conventions that will be pushed into its prompt (the same block a dispatch
  receives, FR-4.1), or "No decisions apply here."; the earlier findings, for
  a spike that asks again; and its tools by name;
- **where it works**: "It works in a throwaway copy of the code, made from
  the current main line. Nothing it writes there is kept: the copy is
  discarded when the run ends, and Subutai never merges it.";
- free agent slots, as on the send screen;
- a forecast from earlier spikes' runs, when there are at least three, as
  the send screen forecasts a step; otherwise "There aren't enough earlier
  spikes to forecast this one yet."

**FR-3.3 — Starting** (`POST /ui/spikes/start`, fields `spike` and `budget`).
The server checks that `run-spike` is assigned and its model configured, and
that the budget is a positive whole number. Then, in one transaction:
1. it moves the spike from `idea` to `running` with a conditional update
   (`… WHERE state = 'idea'`). When no row changes, the start is refused: "This
   spike has already been started.";
2. it records the budget, `started_by`, `started_at`, `base_commit` (the main
   checkout's `HEAD`) and `worktree_path`
   (`<compartment>/worktrees/spk-<short id>`);
3. it enqueues the `run-spike` dispatch on `ref_type = 'spike'`;
4. it writes `spike.started`, with the budget and where it came from.

After the commit, it kicks the dispatcher and redirects to the spike's page
with the notice "SPK-003 has started, with a budget of 40,000 tokens."

The worktree isn't made here. The planner makes it, before the run's first
call (FR-4.2). The database is committed first, as `StartFeature` does, and
the worktree follows from it (R21-3).

**FR-3.4 — No other route starts a spike.** No `/api/*` route, CLI verb, MCP
tool or agent tool moves a spike to `running` or enqueues `run-spike`.
Answering a checkpoint can't either: a `run-spike` dispatch never raises
"Retry or cancel?" (FR-6.1).

**Acceptance:** `TestSpikeStartsFromTheWebUIOnly`:
- the screen renders each part: with and without decisions, with the override
  and with the default, and with the role unassigned and the button disabled;
- the `POST` starts it once, and a second `POST` is refused with the sentence;
- `/api/spikes/start`, `/api/spikes/<id>/start` and `/api/spike/start` are
  404 or 405;
- every MCP name in the forbidden list returns method-not-found;
- `create_spike` leaves the spike an `idea`, with no dispatch.

### FR-4: The run

**FR-4.1 — The prompt.** The planner's `run-spike` case refuses, as a
permanent failure, a spike that isn't `running`. Otherwise it assembles:
- the surfaced decisions and conventions, from the spike's initiative and its
  ancestors (SPEC-018 FR-6.1), first, as every dispatch has them;
- "# The question" and the question;
- "# Where it came from": the owner's name and description, and the paths of
  its current documents. These are read in the working copy, as committed;
- for a spike that asks again, "# What the last spike found" and the earlier
  findings' body;
- when the spike has a draft (a later attempt), "# What you have saved so
  far", the draft, and "An earlier attempt at this spike stopped. Build on
  these findings rather than starting again.";
- "# Your budget": the budget, and "The run stops without warning when it
  reaches this many tokens. Save your findings with `save_findings` early and
  often: whatever you have saved when it stops is what is kept.";
- "# Your working copy": that it is a throwaway copy of the main line, that
  nothing written there is kept or merged, and that findings are saved only
  with `save_findings`, never in a file.

The system prompt is the role's identity and the `run-spike` skill, as for
every role.

**FR-4.2 — The worktree and tools.** Before returning the plan, the planner
makes sure the worktree exists: if the path has no `.git`, it removes
anything at the path and runs `git worktree add --detach <path>
<base_commit>`. A failure is the attempt's failure, so it is retried and, if
it keeps failing, the run fails and ends (FR-6). The role's tools are
resolved as for any role: `read_file`, `list_files`, `edit_file`,
`write_file` and `run_command` work in the worktree; `save_findings` is new;
`finish_spike` is the outcome. A `toolhost.Context` for a spike carries the
worktree and the spike's ID, and no feature or task.

**FR-4.3 — `save_findings`.** Input: `findings`, Markdown, the sections
after Question. It replaces the spike's `draft` and sets `draft_saved_at`,
on a spike that is `running`. It touches no file. It refuses, as a tool error
the agent reads, when it's called by anything but a `run-spike` dispatch. It
returns "Saved. If the run stops now, this is what is kept."

**FR-4.4 — `finish_spike`.** Input: `findings`, the final text. It is
validated as FR-2.2 would write it (an invalid body is returned to the agent,
as any invalid outcome is). A valid call ends the run as `concluded` (FR-6).

**Acceptance:**
- `TestSpikePromptCarriesDecisionsAndQuestion`: a project decision and an
  initiative decision appear in the spike's transcript prompt, and a sibling
  initiative's doesn't. The question and the budget sentence are there, and
  so are the earlier findings for a spike that asks again;
- `TestSaveFindingsKeepsADraft`: the draft changes on the row, no file
  changes in the main checkout, the worktree's tree is unchanged, and a
  `save_findings` call from another purpose is refused;
- `TestSpikeWorktreeIsMadeByThePlanner`: the worktree is detached at
  `base_commit`, and no branch and no `worktrees` row is created for it.

### FR-5: The budget and the hard stop

**FR-5.1 — In the dispatcher.** `dispatch.Plan` gains an optional budget:
- the limit;
- a callback that adds a call's usage to the spike's total and returns the
  new total (`UPDATE spikes SET tokens_used = tokens_used + $1 WHERE id = $2
  RETURNING tokens_used`);
- a reader for the current total;
- the outcomes to return when the run stops early.

When a plan has one, `runLoop` works in this order:
1. **before every call**, it reads the total; if the total has reached the
   limit, it stops with `budget`. After an attempt's first call, it also
   stops with `budget` if the total plus the lower bound (SD-7) would pass the
   limit;
2. it makes the call, and **straight away** calls the callback with the
   call's usage, before checking whether the attempt is still current, so
   a call that was paid for is always counted;
3. if the turn holds a valid `finish_spike`, the run concludes;
4. it runs the turn's `save_findings` calls;
5. if the total has now reached the limit, it stops with `budget`;
6. otherwise it runs the turn's other tools and goes round again.

At the turn cap, it stops with `turn_limit` instead of failing.

Stopping returns the stop outcome, `{"ended":"budget"}` or
`{"ended":"turn_limit"}`, as a successful attempt. The attempt's tokens are
then recorded on the dispatch row, as any success's are, and the transcript
gains a final entry: "The run stopped here because it reached its budget of
40,000 tokens." (or "its turn limit of 40 turns"). Plans without a budget
behave exactly as now.

**FR-5.2 — Retries.** A failed attempt is retried by the existing sweep, up
to `dispatch.max_attempts`, with the same worktree, budget and draft. Its
first check reads the total, so an attempt that starts with the budget used
up stops before its first call.

**Acceptance:**
- `TestSpikeStopsHardAtItsBudget` (mock provider): with a budget of 1,000 and
  calls that each use 150 tokens and never finish, exactly six calls reach the
  provider (the lower bound rules out a seventh, because 900 + 150 > 1,000).
  The script's further steps stay unused; the spike is `ended` with `budget`;
  `tokens_used` is 900; the page says "900 of 1,000 tokens"; the findings
  hold the last draft and the ending section; and the transcript's last entry
  is the stop;
- `TestSpikeSavesOnTheTurnThatCrossesTheBudget`: a turn that saves and
  crosses the budget keeps its save;
- `TestSpikeFinishOnTheTurnThatCrossesTheBudget`: a valid `finish_spike` on
  that turn ends the run as `concluded`;
- `TestSpikeTurnLimitEndsTheRun`: reaching the turn cap ends it as
  `turn_limit`, not failed;
- `TestSpikeBudgetCarriesAcrossAttempts`: an attempt that fails after using
  tokens is retried; the retry's prompt holds the first attempt's draft; and
  the retry stops at the budget, counting both;
- a dispatch without a budget is unchanged (the existing suite, and a unit
  test in `internal/dispatch`).

### FR-6: Ending a run

**FR-6.1 — When.** A run ends when its dispatch succeeds (with
`finish_spike` or a stop outcome) or when it is exhausted. The rules engine
returns `EndSpike` for both. For a `run-spike` dispatch, exhaustion returns
`EndSpike` **instead of** the `dispatch-failure` checkpoint, and `EndSpike`
marks the exhausted dispatch `cancelled`, so the retry sweep stops
republishing it (R21-2).

**FR-6.2 — What `EndSpike` does.** Each step is safe to run again, so
`EndSpike` can resume from wherever an earlier call stopped:
1. **the leak check** (FR-6.3), if the worktree is still there;
2. **the findings**: unless the spike already has a findings document,
   `writeFindings` writes the file and registers it. This and step 3 share a
   transaction, after the file is written;
3. **the state**: moves the spike from `running` to `ended`, with
   `ended_how`, `end_note` and `ended_at`, and writes `spike.ended`, with how
   it ended, the tokens used and the budget. A spike that isn't `running` is
   left as it is;
4. **the commit**: commits the findings file in the main checkout as
   `subutai`, with the message "SPK-003: findings (stopped at the budget)" or
   the matching reason, if it differs from `HEAD`;
5. **the discard**: unless `worktree_removed_at` is set, runs `git worktree
   remove --force` (or removes the directory and runs `git worktree prune` if
   git no longer knows it), then sets `worktree_removed_at` and writes
   `spike.worktree_discarded`.

**FR-6.3 — The leak check.** *(Amended after the first code review: the
first wording checked only the newest commit, and failed open.)* When the
spike starts, `StartSpike` records every ref and its commit
(`refs_at_start`). Before the discard, `EndSpike` finds:
- **the commits made in the worktree**: every commit the worktree's own
  `HEAD` reflog records as made there (a checkout or a reset only moves
  `HEAD`, so those entries don't count), read from the repository's
  administrative copy (`.git/worktrees/<name>/logs/HEAD`, so a damaged `.git`
  file in the worktree doesn't hide it), less everything reachable from
  `base_commit` or from any ref in `refs_at_start`;
- **the refs that changed during the spike**: new, or moved, since
  `refs_at_start`.

A changed ref is a leak when it contains one of the worktree's commits, or
when it is `refs/stash` or a tag. If the check can't run, because git fails or
the reflog can't be read, it **fails closed**: it is treated as a leak whose
ref is unknown.

Every leak:
- is named in How this spike ended (FR-2.3), or, when the check failed, the
  section says "Subutai couldn't check whether code from this spike was kept:
  <the reason>.";
- is audited as `spike.code_kept`, with the refs;
- raises a `spike-code-kept` checkpoint on the spike, which asks: "Code from
  SPK-003 was kept on `keep-this`, outside its working copy. A spike's code is
  never merged. Delete the branch, or keep it knowing it won't be built
  from." The answer is "I've dealt with it", and it changes nothing in
  Subutai.

A ref that moved for another reason, such as a feature merging into `main`
during the spike, isn't a leak, because it holds none of the worktree's
commits.

If git has no record of a working copy that was made (the spike has used
tokens, so its first call ran after the planner made it), the check fails
closed: an agent that deleted its working copy can't hide what it kept. For
the same reason, a retry that finds a used working copy damaged runs the check
on it, and records what it finds, before making it again. *(Amended after the
second code review.)*

**What the check can't see**, accepted as DEC-007's "honest costs" accept the
same limits for chat work, because the agent is a colleague that has been told
not to keep code, not an adversary:
- code the agent's scripts commit somewhere other than the worktree;
- commits made without moving the worktree's `HEAD` (`git commit-tree` and
  `git update-ref`), or recorded under a misleading reflog message;
- a repository with `core.logAllRefUpdates` switched off, where every spike
  reports that the check couldn't run (the safe direction).

It may also report too much: an agent that merges a newer `main` into its
working copy makes `main` look like kept code. That raises a checkpoint a
person dismisses; it never deletes anything.

**FR-6.4 — Reconciliation.** At boot, and on every heartbeat:
- a `running` spike whose run is `succeeded`, `cancelled`, or `failed` with
  no attempts left gets `EndSpike`, with how it ended read from the run's
  outcome, or `failed` with the run's error;
- an `ended` or `closed` spike with a worktree path and no removal time gets
  step 5 again;
- a directory under `<compartment>/worktrees/spk-*` that no `running` spike
  names is removed, and `git worktree prune` is run.

**Acceptance:**
- `TestSpikeWorktreeIsDiscardedWhenItEnds`: for each ending (concluded,
  budget, turn limit, failed), the worktree directory is gone, `git worktree
  list` doesn't list it, `worktree_removed_at` is set, and the audit row is
  written;
- `TestExhaustedSpikeEndsWithoutRetryQuestion`: no `dispatch-failure`
  checkpoint exists, the run is `cancelled`, and `POST /api/respond` has
  nothing to retry;
- `TestSpikeEndIsReconciled`: a spike left `running` after its run succeeded
  (the event never handled) is ended by the next heartbeat, and a spike whose
  `EndSpike` failed after writing its findings is completed without writing
  them twice;
- `TestLeftoverSpikeWorktreeIsRemoved`.

### FR-7: Closing, and asking again

**FR-7.1 — On an ended spike's page:** the three actions of SD-10, each a
form posting to `POST /ui/spikes/close` with `spike` and `as` (`answered`,
`unanswered` or `again`), plus `budget` for `again`. Each redirects.

**FR-7.2 — Answered and unanswered.** In one transaction, a conditional
update moves the spike from `ended` (or, for `unanswered`, from `idea`) to
`closed`, with `closed_as`, `closed_by` and `closed_at`, and writes
`spike.closed`. When no row changes, the close is refused: "This spike can't
be closed now: it is still running, or it is already closed." The findings
are left in whatever state they are.

**FR-7.3 — Again.** In one transaction, the spike is closed as `unanswered`
and a new spike is created as in FR-1.3, with the same owner and question,
`follows_id` set, and the budget entered as its override. The response
redirects to the new spike's start screen.

**FR-7.4 — Closing is final, and only in the web UI.** A `closed` spike shows
no actions. No API route or MCP tool closes a spike.

**Acceptance:**
- `TestPersonClosesASpike`: answered and unanswered each close an ended
  spike; the findings' state is unchanged; closing twice and closing a
  `running` spike are refused with the sentence;
- `TestAskingAgainMakesASecondSpike`: a new `idea` spike with its own number,
  the same question, the link both ways on the pages, the new budget, the
  first spike closed, and the earlier findings in the new run's prompt.

### FR-8: Where spikes show

**FR-8.1 — The spike's page** (`/ui/s/<SPK-ID>`) shows:
- the question, the owner (linked), and the state as a sentence: "This
  spike hasn't started yet.", "This spike is running.", "This spike's run
  has ended: it stopped at its budget. It's waiting for you to read the
  findings.", "This spike is closed: the question is answered.";
- **tokens against the budget**: a bar and "31,004 of 40,000 tokens", or,
  before it starts, "It will have the project's default budget of 1,000,000
  tokens, unless you change it when you start it.";
- while running, the draft as saved so far, with when it was saved, or "The
  agent hasn't saved any findings yet.";
- once ended, the findings, linked to the document page, with their state;
- the worktree: "Its working copy is live." while running, and "Its working
  copy was discarded on 2 October at 14:05." after;
- the actions for its state (FR-1.4, FR-3.1, FR-7);
- **its timeline**: Created, Started (with the budget), Ended (how, with the
  tokens), Working copy discarded, Code kept (if FR-6.3 found a leak), and
  Closed (as what, by whom). Each links to the run where there is one;
- its run, linked to the run page and its transcript;
- the spike it follows, and the spike that follows it;
- for a spike closed as answered, SD-12's sentence.

**FR-8.2 — On the owner's page.** An initiative's and a feature's pages gain
a **Spikes** section listing their spikes, open ones first. Each shows its ID,
question, state and "tokens used of budget". The section has the **New
spike** button. An initiative lists only spikes it owns directly, not its
features'. The markup lives in `spike.html`; `entity.html` gains one include.

**FR-8.3 — On the feature's timeline.** A feature's timeline shows its
spikes' moments as "Spike SPK-003 started" and "Spike SPK-003 ended: it
stopped at its budget", each linking to the spike. This needs the store's
feature history to read the feature's spikes' audit rows. Spike runs are
**not** added to the feature's runs or token totals, and never to
`ActualTokens` or calibration: a spike's tokens are its own, on its own page.

**FR-8.4 — Runs and transcripts.** A `run-spike` run's page names its purpose
"running a spike". Its crumbs lead to the spike and its owner. Its transcript
is the ordinary transcript, ending with the stop entry when there is one. Its
outcome is shown as a sentence: "It concluded.", "It stopped at its budget.",
or "It stopped at its turn limit." The tokens on the run page are the
dispatch row's; the spike's page shows `tokens_used`, which includes failed
attempts, and says so.

**FR-8.5 — The Inbox and the spikes list.** `/ui/spikes` lists every spike,
open ones first, in the owner section's shape. The Inbox page shows a line
when spikes have ended and wait for a person, linking to `/ui/spikes`: "2
spikes have ended and are waiting for you to read their findings." (or "1
spike has ended …"). The Inbox badge still counts checkpoints only, as
SPEC-019 SD-12 kept it.

**FR-8.6 — Not deliverables.** The milestone editor doesn't offer spikes, and
`add_milestone_member` refuses one: "A spike can't be a milestone
deliverable, because it ships nothing. Add the feature its findings led to
instead." (SD-11)

**Acceptance:** `TestSpikePagesShowTheWork` renders:
- the spike's page in each state, with the draft while running;
- the owner sections, and `/ui/spikes`;
- the feature's timeline moments, with the feature's token total unchanged;
- the run page's purpose, crumbs and outcome sentence;
- the Inbox line;
- the refusal from `add_milestone_member`.

### FR-9: MCP

Three tools, in `mcp_spike_tools.go`, appended to the registry with one line:

- **`create_spike`**: "Write down a question to investigate as a spike, on an
  initiative or a feature. This creates the spike; it does not start it. A
  person starts a spike from the web UI, where they see the budget, and its
  findings are written when its run ends." Arguments: `on` (an initiative or
  feature, by ID or path), `question`, and an optional `budget`. No quote:
  creating a spike is planning authoring (DEC-004). The result gives the ID
  and the sentence "A person can start it from its page in the web UI."
- **`list_spikes`**: optional `on` and `state`. Gives each spike's ID,
  question, state, owner, budget (or the default it would get) and tokens
  used.
- **`get_spike`**: by ID. Gives everything `list_spikes` gives, plus the
  draft while running, how it ended, how it was closed, the findings' ID,
  path and state, the run's ID, and the spikes it follows and is followed by.

`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` gains the three names in
its want list. Its forbidden list gains `start_spike`, `run_spike`,
`close_spike`, `answer_spike`, `merge_spike`, `promote_spike`,
`save_findings` and `finish_spike`. `mcp.go`'s `initialize` instructions gain
one sentence: "You may write down a spike's question with `create_spike`;
only a person can start or close one."

**Acceptance:** `TestSpikeMCPTools`: each tool's result and refusals; a spike
created over MCP records `created_via = 'mcp'`; and the tool-set test passes.

### FR-10: The starter pack and configuration

**FR-10.1 — Configuration.** `config.yaml` gains:

```yaml
spikes:
  default_token_budget: 1000000
```

The default is filled when the section is absent, a value that isn't
positive is refused, and the value is read fresh at each use (O-6). The
generated config includes the section with a comment, and `assignments`
gains `run-spike: spike-runner`.

**FR-10.2 — The role and skill.** `roles/spike-runner.yaml` has model
`claude-sonnet-5`, skill `run-spike`, tools `read_file`, `list_files`,
`edit_file`, `write_file`, `run_command` and `save_findings`, and a turn cap
of 40. `skills/run-spike/SKILL.md` says:
- answer the question, and only the question;
- work in the throwaway copy, and treat what you write there as scaffolding;
- save findings after each thing you learn, because the run can stop at any
  call;
- say what you found, how you found it out, and how sure you are; say plainly
  when the evidence is thin;
- finish with `finish_spike` when you have an answer, or when you can say
  why the question can't be answered;
- never write findings into a file in the working copy, and never try to keep
  the code.

**FR-10.3 — Known tools, and the loader.** `config.KnownTools` gains
`save_findings`. `run-spike` joins `isWorktreePurpose`. The loader checks **every role file**, assigned or not
*(amended after the first code review, which found the first wording broke
FR-10.4's upgrade path)*:
- `save_findings` is refused on a role assigned any purpose other than
  `run-spike`: "save_findings is for the spike runner, so role X can't be
  offered it." A role file that isn't assigned yet loads, so a project can
  copy `spike-runner.yaml` before it adds the assignment;
- `report_bug` is refused on a role assigned `run-spike`: "A spike's agent
  puts what it finds in its findings, so role X can't be offered
  report_bug."

**FR-10.4 — Existing projects** copy `templates/findings/`,
`roles/spike-runner.yaml` and `skills/run-spike/` from a fresh `subutai init`
and add the assignment. Until they do, the start screen says nobody is
assigned (FR-3.2).

**Acceptance:** the config default and refusal; the loader's two refusals;
and a fresh `subutai init` has the files and the assignment.

## 4. Stage 2: spikes run by the chat AI or a person

**Built on M13**, which merged into `main` at `73504e0`. The first draft of
this section (S2-1 to S2-9) was written before SPEC-020 existed on `main`, and
said SPEC-020's names would win. They do: this revision uses SPEC-020's
claim record (`work_claims`), its execution record (`executions`), its
claimable interface (FR-2.10), its claim sweep (FR-5.1) and its unmeasured
marker (`executions.measured`, FR-7.1). Where it changes the first draft, §8
says so, finding by finding, under "Stage 2".

Stage 2 is a second way to run a spike. Everything in §3 still holds for a
spike run by the spike runner, and the stage-1 tests still pass unchanged
except `TestSpikeCantBeClaimedYet`, which stage 2 replaces on purpose
(FR-13.9).

### Stage 2's scope decisions

- **SD-17 — The executor is chosen at the start, and recorded twice.** A
  spike's **executor** is who may run it: `agent` (the spike runner), `chat`
  (the chat agent) or `person`. A person chooses it on the start screen, and
  it is recorded on the spike's row, because it must be known before anyone
  claims the spike: it decides who may. Who actually did the work is
  recorded, as for a task, in M13's `executions`: one `agent` row for a
  dispatched run, and one `chat` or `person` row for each claim. The row says
  who may run it; the executions say who did (DEC-007 decision 1).

- **SD-18 — A time box, not a budget, for chat and person spikes.** Work in
  chat or by hand can't be measured in tokens (DEC-007, DESIGN-010 §10), so a
  chat or person spike carries a **time box** of whole hours instead of a
  token budget. It is set on the start screen, filled from
  `spikes.default_time_box_hours` (4). The **deadline** is the start plus the
  time box, fixed once, and never moved (R21-9: one clock). A spike has a
  budget or a time box, never both. The time box has no per-spike override
  at creation: a person sets it on the start screen, which is where the
  executor is chosen too. **Flagged for Sam.**

- **SD-19 — Claimed through M13's claim service, with its own tools.**
  `spike` is a second claimable (SPEC-020 FR-2.10), beside `task`. The chat
  agent claims a spike with a new tool, `claim_spike`, because SPEC-020 keeps
  claiming per kind (`claim_task` takes only a task, FR-6.3). A person claims
  one in the web UI with **I'll run this spike**. Only the executor chosen at
  the start may claim: a chat spike over MCP, a person spike in the web UI.
  The claim holds no feature's working copy (`feature_id` is null): it holds
  the spike's own throwaway worktree, made at the start. **Flagged for Sam:**
  the alternative was `claim_task` accepting a spike's ID, which would blur
  the task tool's description and its FR-6.3 refusal.

- **SD-20 — The time box is the claim's hard expiry, and ends the spike.**
  DESIGN-010 §10 says a chat or person spike's time box is "enforced through
  claim expiry". DEC-007 decision 6 makes claim expiry an inactivity
  question. Both hold, as the first draft's S2-6 said:
  - **the deadline is hard.** The claim carries the spike's deadline as its
    `deadline_at`. When the deadline passes, claimed or not, the heartbeat
    ends the spike as `time_box`: its findings are written from what was
    saved, and its worktree is discarded, exactly as a dispatched spike ends
    at its budget. No question is asked, so M13's `claim-deadline` checkpoint
    is never raised for a spike: the claim sweep, finding a spike's claim
    past its deadline, ends the spike instead;
  - **M13's inactivity question still applies** before the deadline. A
    claim with no activity for `claims.expiry_hours` raises `claim-stale` on
    the spike, with **Keep the claim** and **Release the claim**. With the
    defaults (24 hours, and a 4-hour time box) it only fires on a long time
    box. **Flagged for Sam.**

- **SD-21 — A claim that ends at the time box has its own end reason.**
  M13's claim ends `done`, `released` or `abandoned`. None fits a time box
  that ran out: the work wasn't abandoned, and it wasn't handed in. The claim
  machine gains an event, `expire` (`open`, `submitted` or `returned` →
  `ended`), and `work_claims.end_reason` gains `expired`. **Flagged for Sam.**

- **SD-22 — Releasing a spike's claim frees it, and nothing else.** A
  released task goes to an agent. A spike's executor was fixed at the start,
  so a released spike's claim just ends: the spike keeps running, its draft
  and worktree are kept, and its executor may claim it again before the
  deadline. Only a person releases, in the web UI (SPEC-020 SD-5).

- **SD-23 — Submitting ends the run as concluded.** `submit_spike`, or **I've
  finished** in the web UI, validates the findings against the template
  exactly as `finish_spike` does (FR-4.3), saves them as the draft, ends the
  claim (`submit`, then `done`, in one transaction), and then ends the spike
  as `concluded` through `EndSpike` (FR-6), which writes, commits and
  discards as for any spike. There is no reviewer: a person reads the
  findings, as for any spike (SD-9).

- **SD-24 — Unmeasured means not counted, not zero.** A chat or person spike
  adds nothing to `tokens_used`, and its executions are `measured = false`
  (SPEC-020 FR-7.1). Every place that shows a spike's tokens says it wasn't
  measured instead of showing 0. The start screen's forecast samples only
  `run-spike` dispatches, so a chat or person spike can't enter it.

- **SD-25 — Judging stays a person's.** Starting and closing a spike stay in
  the web UI (DEC-006 Amendment 1, SD-5, SD-10), whoever runs it. The chat
  agent that ran a spike can't close it, approve its findings, release its
  claim, or move its deadline. The MCP tool-set test proves those tools don't
  exist. Approving the findings stays what SD-9 makes it: a person's act on
  the document page, or the chat agent's relay of a person's quoted verdict
  (DEC-006 Amendment 1), never the chat agent's own judgement.

- **SD-26 — The person who ran a spike may close it.** DEC-007 decision 2
  says whoever does the work can't judge it. Closing a spike and approving
  its findings aren't an independent judging stage: they are the person who
  asked the question saying whether it is answered (DESIGN-010 §10, SD-10),
  which only a person can say, and a one-person project has no one else. So
  the person who ran a spike by hand may close it and approve its findings.
  The `spike.closed` audit row records `closer_ran_it: true` when the closer
  is the spike's person executor, and the page says "Closed by sam, who also
  ran it." **Flagged for Sam:** the alternative is refusing that close and
  saying what a one-person project does instead.

- **SD-27 — Three in-process locks, in one order.** A chat or person spike's
  worktree is touched by the start, a claim, the claim sweep, a submit and
  the ending. Every path takes, in this order and only those it needs:
  **the spike's working-copy lock** (`withWorkingCopy` on its worktree
  path), then **`spikeEndMu`**, then **row locks** (the spike, then its
  claim). The start's make, a claim's remake, the sweep's fingerprint, a
  submit, and every ending (rules, heartbeat, sweep) hold the working-copy
  lock, so a discard can't interleave with a remake.

### FR-11: The executor and the time box in the database

**FR-11.1 — Migration `0016_spike_executors.sql`.** It adds to `spikes`:

| Column | What it holds |
|---|---|
| `executor` | `agent`, `chat` or `person`; null before the start |
| `time_box_hours` | whole hours, set at the start of a chat or person spike |
| `deadline_at` | the start plus the time box, for a chat or person spike |

In this order, so the new checks validate rows that already satisfy them:
1. **Backfill** `executor = 'agent'` on every spike with `started_at` set,
   and one inferred `agent` row in `executions` (`ref_type` `spike`, round 1,
   `measured`, `inferred`, started at the first `dispatch.running` audit
   row) for every `run-spike` dispatch that has one, as `0014` did for tasks.
2. **Replace** stage 1's named checks that assume a token budget:
   - `spikes_started` becomes, keyed on `started_at` rather than the state:
     a spike with `started_at` set has an `executor`; a spike that is
     `running` or `ended` has `started_at`; an `agent` spike with
     `started_at` has `token_budget`; a `chat` or `person` spike with
     `started_at` has `time_box_hours` and `deadline_at`;
   - `spikes_ended_how_values` gains `time_box`;
   - a new `spikes_ended_how_executor`: `budget` and `turn_limit` need
     `executor IS NOT NULL AND executor = 'agent'`, and `time_box` needs
     `executor IS NOT NULL AND executor IN ('chat', 'person')`;
   - a new `spikes_executor_values` (null, or one of the three), and
     `spikes_time_box_positive` (`time_box_hours` is null or 1 to 168);
   - a new `spikes_one_limit`: an `agent` spike has no `time_box_hours` and
     no `deadline_at`, and a `chat` or `person` spike has no `token_budget`.
3. **Replace** `work_claims`' unnamed end-reason value check (Postgres
   named it `work_claims_end_reason_check`) with a named
   `work_claims_end_reason_values` that allows `expired` (SD-21). The named
   `work_claims_end_reason` (ended exactly when there is a reason) is kept.

The migration names no new enum value. Each new check is named.

**FR-11.2 — The store.** `store.Spike` gains `Executor`, `TimeBoxHours` and
`DeadlineAt`. `store.StartSpike` takes the executor, and for chat or person
the time box, and sets `deadline_at = now() + time box` in its conditional
update; its audit row records the executor and the budget or the time box.

**Acceptance:**
- a store test inserts each bad combination the checks name and sees each
  refused: a running chat spike without a deadline; an agent spike ended as
  `time_box`; a chat spike ended at `budget`; both limits on one row; a time
  box of 0; a claim ended `expired` is accepted;
- the migration runs on a database holding stage-1 spikes (an idea, a
  running one, an ended one and a closed one that ran), and leaves each
  started one with `executor = 'agent'` and one inferred execution row;
- a closed spike that ran without an executor, and an agent spike with a
  deadline, are refused;
- `StartSpike` for a chat spike with a 4-hour time box records a deadline
  4 hours after `started_at`, to the second.

### FR-12: Starting for the chat agent or a person

**FR-12.1 — The start screen** (`/ui/spikes/start`) gains **Who runs it**,
three radio choices, in this order:
- **The spike runner**: "An agent runs it on *model*, with a token budget."
  Disabled, with stage 1's refusal beside it, when nobody can run spikes;
- **The chat agent**: "You or the chat agent ask it to run SPK-003 in a chat
  session. It claims the spike over MCP.";
- **You**: "You run it by hand, from this spike's page."

The spike runner is chosen by default when it can run; otherwise the chat
agent. Choosing the chat agent or a person (without script, by the form's
own fields; with script, by showing and hiding them) replaces the budget
field with **Time box, in hours**, filled from the default, and shows: "Work
done in chat or by hand can't be measured in tokens, so this spike is held
to a time box instead. When the time box runs out, the spike ends with
whatever findings have been saved." The forecast, workers and tools panels
describe the spike runner, and say so.

**FR-12.2 — Starting** (`POST /ui/spikes/start`, `executor` and either
`budget` or `time_box` fields). `Server.StartSpike` takes the executor:
- **agent**: as stage 1. The `executions` row (`agent`, the role, the
  model, the run, round 1, `measured`) is written when the dispatcher marks
  the `run-spike` dispatch running, in the same transaction, as SPEC-020
  FR-1.2 does for an implementer, and as the backfill reads it; a retried
  attempt writes nothing new. `store.RecordAgentExecution` gains the
  reference type, so one helper serves both;
- **chat or person**: the same checks as stage 1 except the spike runner's
  (the findings template and its manifest are still required); the time box
  must be a whole number of hours from 1 to 168, refused otherwise with "A
  spike's time box is a whole number of hours, from 1 to 168."; the
  transaction moves the spike to `running` with the executor, the time box
  and the deadline, and **queues nothing**. After it commits, the worktree is
  made at the base commit, detached, through `ensureSpikeWorktree` under the
  worktree's lock (SD-27), as the planner makes it for a run (FR-4.2). If
  making it fails, the start still stands, and the first claim makes it
  (FR-13.3).

**FR-12.3 — Configuration.** `spikes.default_time_box_hours`, an integer
pointer: absent means 4; otherwise 1 to 168, and the loader refuses anything
else with a sentence. The starter config documents it, commented, beside
`default_token_budget`.

**FR-12.4 — Reconciliation knows a chat or person spike has no run.**
`ReconcileSpikes` doesn't end a running chat or person spike for having no
dispatch, and keeps its directory. It ends one in two cases only, checked in
this order (FR-15.2): a claim of it ended `done` (any claim, not only the
latest; a submit that stopped before its ending), which ends it `concluded`; then its deadline has passed
(FR-15.1), which ends it `time_box`. The claim sweep ends one too, at the
deadline (FR-14.4).

**FR-12.5 — What the start says.** The notice after starting a chat spike:
"SPK-003 has started, with a time box of 4 hours. Ask the chat agent to run
it." For a person spike: "SPK-003 has started, with a time box of 4 hours.
Claim it below when you are ready." NFR-8's typed values gain the time box.

**Acceptance:**
- the start screen shows the three choices, the time box field and the
  sentence, and a refused or disabled spike runner;
- starting a chat spike queues no dispatch, makes a detached worktree, and
  leaves no branch;
- a time box of 0, 169 or "four" is refused with the sentence, and the spike
  stays an idea;
- with no spike runner assigned, a chat or person spike still starts;
- a heartbeat after the start leaves the running chat spike and its
  directory alone;
- a dispatched spike's run, marked running, writes its `agent` execution
  row once, and a retried attempt doesn't write a second;
- the start notices.

### FR-13: Claiming a spike

**FR-13.1 — The claimable.** `spikeClaims` implements SPEC-020's interface,
registered under `spike`:
- `load`: the spike; no feature; the worktree's absolute path;
- `refusal`, each a full sentence that says what to do:
  - not running: "SPK-003 isn't running, so it can't be claimed. A person
    starts a spike from its page in the web UI." (an idea), or "SPK-003 has
    ended. A person reads its findings on its page." (ended or closed);
  - the wrong executor: "SPK-003 is run by the spike runner, an agent, so it
    can't be claimed." / "SPK-003 is to be run by a person in the web UI, not
    in chat." / "SPK-003 is to be run in chat, not in the web UI. Ask the chat
    agent to run it.";
  - a spike whose claim already ended `done` (a submit whose ending was cut
    short): "SPK-003 has been submitted, and is ending. A person reads its
    findings on its page.";
  - past the deadline: "SPK-003's time box ended at 17:04 UTC, so it is ending
    with the findings that were saved.";
  - held by someone else: SPEC-020's `heldBySentence`;
  - the worktree missing after FR-13.3's remake: "SPK-003's working copy
    couldn't be made, so it can't be claimed yet: …";
- `onClaim`: sets the claim's `deadline_at` to the spike's, and cancels
  nothing;
- `staleQuestion`: "SPK-003, *question*, was claimed by the chat agent 26
  hours ago, and nothing has changed in its working copy or its findings for
  24 hours. Its time box ends at 17:04 UTC on 3 October. Is someone still working
  on it?", with the release answer **Release the claim**;
- `deadlineQuestion`: never asked (SD-20); it returns the same words as the
  stale question, for completeness;
- `prepareSubmit`, `onSubmit`, `prepareRelease`, `onRelease`: as FR-13.6 and
  FR-14;
- `contract`: FR-13.4.

**FR-13.2 — What the spike shares with the task's claim, and what it
doesn't.** Exactly:
- **claiming and renewing** go through SPEC-020's `claimLocked`. The
  claimable gains two methods: `lock(ctx, tx, t)`, which for a task is
  `LockFeatureAndTask` and for a spike locks the spike's row (`SELECT … FOR
  UPDATE`), so `claimLocked` holds no test of the kind; and `rules()`, the
  claimant's rules (the task's `claimRuleSentences`, or FR-13.4's).
  `claimResult` skips the task's round, review comments and base commit when
  the target has no task, and takes the rules from the claimable. The
  change notice goes to the claimed item's own kind (`spike`, and its
  owner), not `task`. The branch watch and the base-commit reset are
  skipped, as they already are for a claim without a feature;
- **a renewal is refused after the deadline**, with the deadline's sentence:
  for a spike, `claimLocked` runs the claimable's refusal on a renewal too
  (its refusal for a renewal checks only the state and the deadline);
- **submitting and releasing** don't go through SPEC-020's `submitLocked` and
  `releaseLocked`, which commit, compare the working copy with its start,
  queue a review or an implementer, and assume a feature. They are the
  spike's own methods (FR-13.7, FR-14.1), which use the same store functions
  (`LockCurrentClaimFor`, `TransitionClaim`, `MarkClaimExecutionSubmitted`).
  The claimable's `prepareSubmit`, `onSubmit`, `prepareRelease` and
  `onRelease` for a spike return an error saying so, and nothing calls them;
- **`resolveClaimRef`** stays task-only. `ClaimSpike` resolves a spike's ID
  itself.

The lock order is SD-27's, and nothing that locks a feature also locks a
spike. The claim's execution row is `ref_type` `spike`, round 1,
`measured = false` (FR-1.2 of SPEC-020): every claim of a spike, including a
claim after a release, is its own row.

**FR-13.3 — Claiming** (`Server.ClaimSpike(ctx, ref, who)`, used by
`claim_spike` and the UI). Before the claim, outside any transaction and
holding the worktree's lock, the spike is read again and refused if it has
ended, been submitted or passed its deadline (nothing is made for a spike
that is over), and then the worktree is made if it is missing, as the
planner does (FR-4.2). Whether what it kept must be checked and recorded
first is one predicate, `spikeHadWorkingCopy`: true for an agent spike that
has used tokens (stage 1's rule), and for a chat or person spike whose
worktree was made, which is audited as `spike.worktree_made` when it is made
(at the start, or at the first claim if the start's make failed). The
predicate is "the worktree was made", not "the spike started". The leak check's "couldn't
check" path (FR-6.3) uses the same predicate in place of `TokensUsed > 0`,
so a chat spike whose working copy vanished fails closed, as round 2 of
the stage-1 review required. Then SPEC-020's claim, renewal or refusal. A claim by the holder of an
open claim renews it.

**FR-13.4 — What a claim returns** (`claim_spike`'s result, and the person's
panel):
- `spike`: `get_spike`'s entry (FR-16.2);
- `working_copy`: `{ "path" (absolute), "base_commit", "detached": true }`;
- `contract`:
  - `question`;
  - `where_it_came_from`: FR-4.1's "Where it came from" section, as text;
  - `decisions`: the surfaced block (SPEC-018), when there is one;
  - `earlier_findings`: `{ "spike", "body" }` for a spike that asks again;
  - `draft`: the findings saved so far, when there are any;
  - `findings_template`: the template's sections, with what each is for;
- `deadline`: the deadline (RFC 3339), and `time_left`: "The time box ends at
  17:04 UTC, in 3 hours 12 minutes. Then the spike ends with whatever
  findings you have saved.";
- `rules`: sentences, said once in Go so the chat skill can be checked
  against them:
  - "Work only inside the working copy named in this result. It is a
    throwaway copy: it is discarded when the spike ends, and nothing in it is
    kept or merged.";
  - "Don't commit, branch, tag or stash there: a spike keeps no code, and
    Subutai reports any it finds kept.";
  - "Save your findings with save_spike_findings early and often. When the
    time box ends, whatever you have saved is what is kept.";
  - "When you have an answer, or know the question can't be answered, call
    submit_spike with your findings in the template's sections.";
  - "A person reads the findings and decides whether the question is
    answered. You can't close the spike, release your claim or move its time
    box, and you can't approve its findings on your own judgement: only a
    person's verdict, in their words, approves them.";
  - then the commands the project allows, as `claim_task`'s rules list them;
- `next`: "Work on the question in the working copy, save findings as you
  go with save_spike_findings, and call submit_spike when you are done."

**FR-13.5 — `claim_spike`**, one argument, `spike` (`SPK-003`). Its
description says, in sentences: what it does; that a person started the
spike for the chat agent to run, with a time box; the rules above; that
the time box ends the spike whether or not it has finished; that calling it
again renews the claim. `claim_task` given a spike's ID refuses with "SPK-003
is a spike, not a task. Use claim_spike to run it." in place of FR-6.3's
sentence; for anything else, FR-6.3's sentence is unchanged.

**FR-13.6 — Saving the draft.** `Server.SaveSpikeFindings(ctx, ref, who,
text)`, used by `save_spike_findings` and the UI's draft editor:
- the claimant must hold the spike's `open` claim, from this surface, and the
  deadline must not have passed; otherwise refuse, saying who holds it, or
  that the time box has ended;
- the text must not be blank;
- in one transaction, locking the spike then the claim and re-checking the
  holder, the state and the deadline: the draft replaced
  (`store.SaveSpikeDraft`), the claim's activity `findings` recorded, a
  `claim.activity` audit row, and a pending `claim-stale` withdrawn, as the
  sweep does for a working-copy change (SPEC-020 FR-5.4). One store helper
  does the last three. `claimActivityWords` says "findings saved".

`save_spike_findings` takes `spike` and `findings`. Its result: "Saved. If
the time box ends now, this is what is kept." and the time left.

**FR-13.7 — Submitting.** `Server.SubmitSpike(ctx, ref, who, findings)`,
used by `submit_spike` and **I've finished**:
1. Refuse unless the claimant holds the `open` claim, from this surface, and
   the deadline hasn't passed (the same sentences as FR-13.6).
2. The findings are the text given, or, when it is blank, the saved draft.
   Validate them as `finish_spike` does (`validateFinishSpike`); a refusal
   names what is missing.
3. Take the worktree's lock, then `spikeEndMu` (SD-27), and hold both
   through step 5, so no ending can fall between them.
4. In one transaction, locking the spike then the claim, and re-checking
   that the claim is still this claimant's and `open`: the draft saved; the
   claim's `submit` then `done` (activity `submitted`); `submitted_at` on
   the claim's execution; the audit row `claim.submitted` with no commit and
   no review. The deadline is **not** judged again here: a submit that
   passed step 1 is honoured, even if the deadline passes while it runs. If
   an ending won the locks first, the claim is no longer open, and the
   submit is refused with the time box's sentence; the findings it carried
   are lost, and the refusal says so ("The time box ended before this
   arrived, so the spike ended with the findings saved before it.").
5. Then the ending, as `concluded` (an internal `endSpikeLocked` that doesn't
   take `spikeEndMu` again). If the server stops between 4 and 5,
   reconciliation finishes it (FR-12.4).

`submit_spike` takes `spike` and `findings` (optional). Its result: the spike
entry (now `ended`), the findings document's ID and path, and `next`: "A
person reads the findings on the spike's page and decides whether the
question is answered. You can't close the spike or approve its findings."

**FR-13.8 — The chat agent is told.** The `initialize` instructions gain a
sentence: a person may start a spike for the chat agent to run; it claims
the spike with `claim_spike`, saves findings with `save_spike_findings`,
submits with `submit_spike`, and a person reads and closes it. A new chat
skill, `chat-skills/run-a-spike/SKILL.md`, says how, and a test checks it
carries every rule `claim_spike` returns, as `work-a-task`'s test does.

**FR-13.9 — `TestSpikeCantBeClaimedYet` is replaced** by
`TestSpikeIsClaimedOnlyByItsExecutor`: an agent spike and an idea are
refused by `claim_spike`; a person spike is refused over MCP; a chat spike
is refused in the UI; `claim_task` given a spike's ID points to
`claim_spike`; and the chat spike is claimed over MCP.

**Acceptance:**
- a chat spike claimed over MCP returns the working copy, the contract, the
  deadline and the rules; the claim has `feature_id` null and `deadline_at`
  equal to the spike's; its execution row is `chat`, `spike`, round 1,
  unmeasured;
- each refusal above, by test;
- claiming again renews; claiming after a release is a new claim and a new
  execution row;
- a missing worktree is made by the claim;
- `save_spike_findings` replaces the draft and withdraws a pending
  `claim-stale`; it is refused for someone else's claim, without a claim,
  and after the deadline;
- `submit_spike` with incomplete findings is refused and changes nothing;
  with good findings, the spike ends `concluded`, the findings are committed,
  the worktree is gone, the claim ended `done`, and the execution submitted;
- `submit_spike` with no findings uses the saved draft;
- the chat skill test, and the `initialize` sentence.

### FR-14: Releasing, and the inactivity question

**FR-14.1 — Release** (`Server.ReleaseSpikeClaim(ctx, ref, by)`, web UI
only). An `open` claim ends `released`, audited with who released it. The
spike keeps running; its draft and worktree are kept (SD-22). Nothing is
committed and nothing is queued.

**FR-14.2 — The answers.** `claim-stale` on a spike is answered in the Inbox
with **Keep the claim** (SPEC-020's `KeepClaimAnswered`, unchanged) or
**Release the claim** (`ReleaseClaimAnswered`, which now releases a spike's
claim through FR-14.1 instead of ignoring it). An answer to a claim that has
moved on is a no-op with SPEC-020's notice.

The Inbox reads the release answer's words and their consequence from the
checkpoint's context, not from fixed task words: `release_label` (which the
sweep already stores) and a new `release_consequence`, which the claimable
returns beside the label. For a task they are unchanged ("Release it to an
agent"; "The claim ends, the work in the working copy is kept, and an agent
takes the task."). For a spike: "Release the claim"; "The claim ends. The
spike keeps running, its draft and working copy are kept, and its executor
can claim it again before the time box ends." A checkpoint raised before
this change, with no `release_consequence`, falls back to the task's words.
The timeline's checkpoint moment for a spike reads "Waiting for a person: is
someone still working on a spike".

**FR-14.3 — Activity.** Saving findings (FR-13.6), a change in the working
copy (the sweep's fingerprint, SPEC-020 FR-5.2, which works in a detached
worktree), a renewal and a submit are a spike claim's activity.

**FR-14.4 — The claim sweep and the deadline.** For a claimable that
implements an optional interface, `deadlineEnder` (the spike's does), the
sweep judges the deadline **first**: past it, it ends the spike (FR-15.1)
and skips steps 2 and 3, so it never raises `claim-stale` a moment before
the ending withdraws it, and never raises `claim-deadline`. The task's path
is unchanged.

**Acceptance:**
- a person releases a chat spike's claim; the spike is still running, and
  the chat agent can claim it again;
- with `last_activity_at` backdated past the expiry, the sweep raises
  `claim-stale` on the spike with the question above; saving findings
  withdraws it; **Release the claim** releases it; **Keep the claim** keeps
  it;
- the Inbox shows a spike's `claim-stale` with **Release the claim** and the
  spike's consequence, and a task's with its words unchanged;
- with both the expiry and the deadline passed, one sweep ends the spike and
  raises no `claim-stale`; no `claim-deadline` checkpoint is ever raised for
  a spike.

### FR-15: The time box ends the spike

**FR-15.1 — The deadline.** On every heartbeat, `ReconcileSpikes` ends each
running `chat` or `person` spike whose deadline has passed:
`EndSpike(time_box)`. So does the claim sweep, for a claimed one (FR-14.4).
`EndSpike` is serialised, and its state change is conditional, so the two
can't both write findings.

**FR-15.2 — Ending a chat or person spike.** `EndSpike` accepts `time_box`,
and only for a `chat` or `person` spike. Every ending of a chat or person
spike takes the worktree's lock before `spikeEndMu` (SD-27). In the
transaction that moves the spike to `ended`, after locking the spike, it
looks at its claims: if any claim of it ended `done` (not only the latest),
the spike ends as `concluded`, never `time_box` (a submit got there first);
otherwise any claim
on it that hasn't ended is ended with the claim machine's `expire` (SD-21),
audited, and its pending `claim-stale` withdrawn. Everything else is FR-6:
the leak check first, the findings from the draft, the commit, the discard.

**FR-15.4 — The leak check over a long time box.** FR-6.3's check reports a
new or moved tag, and any stash, as possibly kept code. Over a time box of
up to a week, a person's ordinary tags and stashes in the main checkout are
likely, and would each be reported. Stage 2 keeps the rule, which fails
safe, and FR-6.3's accepted limits say a long time box makes false reports
likely; the `spike-code-kept` answer, "I've dealt with it", costs a person a
look. **Flagged for Sam** with choice 2.

**FR-15.3 — The findings say how it ended.** For `time_box`, SD-8's
completion gives:
- an empty Answer: "Not answered: the spike reached the end of its time box
  before it reached an answer.";
- an empty What we found: "Nothing was saved before the time box ended.";
- **How this spike ended**: "The spike reached the end of its time box of 4
  hours." For a `concluded` chat or person spike: "The chat agent concluded
  the spike." or "sam concluded the spike by hand."

Neither says a number of tokens. Both add: "It ran in chat, so its tokens
weren't measured." or "It was run by hand, so its tokens weren't measured."

`store.spikeEndings` gains a `time_box` entry with every field the other
endings have: the timeline's phrase ("it reached its time box"), the page's run
line ("The spike reached the end of its time box."), the commit message's
words ("time box ended"), the lead and the not-answered sentence above.

**FR-15.5 — Asking again after a chat or person spike.** The close dialog's
third choice on such a spike reads **Ask again**, with no budget field
(`AgainBudget` reads the token budget, which it hasn't): the new spike's
executor, and its budget or time box, are chosen on its own start screen.
It is created with no budget override.

**Acceptance:**
- with `deadline_at` backdated, one heartbeat ends a claimed chat spike as
  `time_box`: its findings hold the saved draft and the time-box sentence,
  they are committed, the worktree is discarded, the claim ended `expired`;
- the same for an unclaimed chat spike, and a person spike;
- a spike past its deadline whose leak check finds a branch made in its
  worktree reports it, as stage 1's leak check does;
- a running chat spike whose claim ended `done` is ended `concluded` by
  reconciliation, even when its deadline has also passed;
- a submit and a deadline ending, run at once, leave either the submitted
  findings and `concluded`, or a refused submit whose refusal says the
  findings were lost and `time_box`, never findings that say `time_box`
  after a submit was accepted;
- a chat spike whose working copy was deleted and pruned before its time box
  ends reports that the check couldn't run (fails closed);
- `EndSpike(time_box)` on an agent spike is refused.

### FR-16: Where it shows

**FR-16.1 — The spike's page.**
- **The executor line**, under the heading: "Run by the spike runner
  (claude-sonnet-5)." / "To be run in chat. Waiting for the chat agent to
  claim it." / "Being run in chat by the chat agent, who claimed it 2 hours
  ago." / "Being run by hand by sam." / "Run in chat by the chat agent." /
  "Run by hand by sam." It is built from the executions and the latest
  claim, by one function shared with `get_spike`.
- **The limit**: an agent spike keeps stage 1's budget bar. A chat or person
  spike shows the time box instead: "Time box: 4 hours, ending at 17:04 UTC on
  3 October (3 hours 12 minutes left)." while running, and "Time box: 4 hours."
  after. In place of the token bar: "This spike ran in chat, so its tokens
  weren't measured." (or "by hand").
- **A running person spike** shows, to the person: **I'll run this spike**
  when unclaimed; when they hold the claim, the working copy's absolute path,
  the time left, the draft editor (the draft, **Save findings**), **I've
  finished** and **Release the claim**. Each is a `POST` (`/ui/spikes/claim`,
  `/ui/spikes/draft`, `/ui/spikes/submit`, `/ui/spikes/release`), same-origin,
  with the spike's ID in a hidden field, redirecting to the page with a
  notice or a refusal banner.
- **A running chat spike** shows who holds it, since when, the last
  activity, and **Release the claim**; unclaimed, it says "Ask the chat agent
  to run SPK-003. It claims the spike with claim_spike."
- The ended and closed views are stage 1's, with the time-box and unmeasured
  lines above.

**FR-16.2 — `get_spike` and `list_spikes`.** Each spike entry gains
`executor` (`{ "kind", "sentence", "who", "model", "run_id" }`),
`measured` (false for chat and person), and, for chat and person,
`time_box_hours`, `deadline` and `claim` (`{ "kind", "who", "state", "since",
"last_activity" }` for the latest claim). For an unmeasured spike,
`tokens_used` is null rather than 0.

**FR-16.3 — The spikes list** (`/ui/spikes` and the owner's section) shows
the executor beside the state, as a word: "agent", "chat" or "by hand".

**FR-16.4 — Closing by the person who ran it** (SD-26). The close is
allowed, the audit row carries `closer_ran_it`, and the closed spike's page
says "Closed by sam, who also ran it."

**Acceptance:** each line above, by a page test and a `get_spike` test; the
time-box line and the unmeasured line on a chat spike's page; a person spike
closed by the person who ran it, with the audit field and the sentence; the
ask-again dialog on a chat spike has no budget field.

### FR-17: The boundary

**FR-17.1 — The advertised tool set gains exactly `claim_spike`,
`save_spike_findings` and `submit_spike`**, each with a comment in
`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` citing DEC-007 decision 3
(running a spike is doing), DESIGN-010 §10 and this section.

**FR-17.2 — The must-not-exist list gains** `release_spike`,
`extend_spike`, `set_time_box`, `end_spike`, `stop_spike`,
`approve_findings` and `claim_spike_review`. `start_spike`, `close_spike`,
`answer_spike`, `save_findings` and `finish_spike` stay on it: the
dispatched runner's tools are never offered to chat.

**FR-17.3 — No new route starts or closes a spike.** The four new `POST`
routes claim, save, submit and release. A test posts each with a spike in
every state and checks none starts, closes or reopens it.

**Acceptance:** the tool-set test passes with M13's and M14's tools
together, and lists the three new tools on purpose.

### FR-18: The integration checks

The orchestration note's four checks, each a named test:
- `TestChatSpikeIsHeldByItsClaimAndDeadline`: a chat spike's claim carries
  the deadline; inactivity raises `claim-stale`; the deadline ends the spike;
- `TestSpikeExecutorIsRecorded`: an agent, a chat and a person spike each
  record their executor on the row and in `executions`;
- `TestChatSpikeTokensAreUnmeasured`: the chat spike's executions are
  `measured = false`, `tokens_used` stays 0 and `get_spike` says null and
  `measured: false`, and the start screen's forecast ignores it;
- the tool-set test (FR-17).

And the stage's own end-to-end: `TestChatSpikeEndToEnd`, a chat spike
started in the UI, claimed over MCP, findings saved, submitted, and closed by
a person as answered; `TestPersonSpikeEndToEnd`, the same by hand in the UI.

### Stage 2's non-functional requirements

- **NFR-10 — One service layer.** MCP and the UI call `ClaimSpike`,
  `SaveSpikeFindings`, `SubmitSpike` and `ReleaseSpikeClaim`; they share
  SPEC-020's claim record functions, and none touches the task path.
- **NFR-11 — No git in a transaction**, as SPEC-020 NFR-6. The worktree is
  made outside the start's transaction, and the leak check runs before
  `EndSpike`'s.
- **NFR-12 — Migration `0016` only.** Shared files: `claims.go` (the
  registry, `lock` and `rules` on the interface, `claimResult`),
  `claim_sweep.go` (the `deadlineEnder` hook, the release answer, the
  release consequence), `lifecycle/claim.go` (`expire`), `store/claims.go`,
  `store/executions.go` (the reference type on `RecordAgentExecution`),
  `internal/dispatch/dispatch.go` (the agent row when a `run-spike` run
  starts), `ui.go` and the Inbox template (the release words), `timeline`
  (the spike's claim moment), `mcp.go` (the registry line and the
  `initialize` sentence), and the tool-set test.

### Stage 2 definition of done

The orchestration note's checks, applied to stage 2, with
`verification_passed` meaning every acceptance criterion in §4 has cited
evidence, plus:
- a browser walkthrough without an AI provider: start a chat spike and a
  person spike from the start screen, claim the chat spike over MCP, save
  and submit findings, see the time-box and unmeasured lines, run the person
  spike by hand from its page, and close both. It is
  `docs/walkthrough-spec-021-stage2.md`;
- a handoff, `docs/notes/handoff-M14-stage2-2026-10-02.md`, and the
  roadmap's §11 line for M14 updated.

**Stage 2's choices for Sam** (DoD 10 continues):
15. the executor is chosen on the start screen and recorded on the spike,
    and who did the work in `executions` (SD-17);
16. a time box in whole hours, default 4, from 1 to 168, set on the start
    screen with no per-spike override at creation; `default_time_box_hours`
    rather than the first draft's `default_time_box` (SD-18);
17. a new MCP tool, `claim_spike`, rather than `claim_task` taking a spike
    (SD-19);
18. the deadline ends the spike without a question, and M13's inactivity
    question still applies before it (SD-20). This replaces choice 14;
19. a new claim end reason, `expired` (SD-21);
20. releasing a spike's claim only frees it for its executor to claim again
    (SD-22);
21. submitting ends the run with no reviewer, as a dispatched spike's
    conclusion does (SD-23);
22. the person who ran a spike may close it and approve its findings
    (SD-26);
23. the leak check keeps reporting any new tag or stash, so a long time box
    may report a person's ordinary ones (FR-15.4).

## 5. Non-functional requirements

- **NFR-1 — One service layer.** The UI handlers, the MCP tools and the rules
  call the same service methods (`CreateSpike`, `StartSpike`, `EndSpike`,
  `CloseSpike`). Each state change is one conditional update in a
  transaction with its audit row.
- **NFR-2 — The orchestrator stays code.** Nothing an agent or the chat AI
  does starts a spike. Creating a spike dispatches nothing.
- **NFR-3 — The seam holds.** Only the web UI starts or closes a spike, and
  the MCP tool-set test proves the absence of every tool that could.
- **NFR-4 — No merge path.** `TestSpikeHasNoMergePath` checks SD-2's facts:
  - the worktree is detached, and no branch and no `worktrees` row is created
    for it;
  - `mergeFeature`, given a spike's ID, finds no feature and refuses;
  - the rules emit no `MergeFeature` for a `run-spike` success;
  - no route matching `merge`, `promote` or `land` exists for spikes;
  - `POST /api/features/start` and the feature page refuse a spike's ID,
    because it isn't a feature;
  - after the run, the main checkout's history holds only the findings
    commit;
  - **the leak check**: the test's config whitelists a script as a command.
    The agent runs it, and it keeps code in each of the ways FR-6.3 names: a
    branch at the newest commit; a branch at an earlier commit, followed by
    another commit; a branch, then a checkout of `base_commit`; a tag; and a
    stash. Each is reported, and the `spike-code-kept` checkpoint exists. A
    `main` that moved during the spike without the worktree's commits isn't
    reported, and a broken reflog is reported as a check that couldn't run.
- **NFR-5 — Contained templates.** New markup lives in `spike.html`: the
  page, the owner section, the dialog, the start screen and the list.
  `entity.html` gains one include, `inbox.html` one line, and the timeline's
  rules a spike section.
- **NFR-6 — Coordination.** Migration `0015` only. Shared files, each with a
  small, named change; M13 is likely to touch those marked (M13) too, and
  the integrator should expect a conflict there:
  - `internal/server/mcp.go`: the registry line and the instructions sentence
    (M13);
  - `internal/server/integration_mcp_test.go`: the want and forbidden lists
    (M13);
  - `internal/server/ui/templates/entity.html`: one include (M13);
  - `internal/dispatch/dispatch.go`: the budget in `runLoop` (M13, for
    unmeasured tokens);
  - `internal/config/config.go` and `compartment.go`: the `spikes` section,
    the known tool and the loader's checks (M13, for claim expiry);
  - `internal/server/server.go`: the boot and heartbeat duties (M13, for the
    heartbeat);
  - `internal/rules/rules.go`: `EndSpike` on success and exhaustion;
  - `internal/server/planner.go` and `toolexec.go`: the `run-spike` case and
    the tool;
  - `internal/store/observe_reads.go`: the feature's history;
  - `internal/lifecycle/gates.go` callers: G5's count;
  - `internal/ident`, `internal/toolhost`, `internal/timeline`, and the
    starter pack.
- **NFR-7 — Human prose** in every label, refusal, notice and tool
  description (D-6). British spelling.
- **NFR-8 — No typed paths.** Forms carry row ids in hidden fields; the only
  typed values are the question, the budget, the time box (stage 2) and a
  person's findings in the draft editor (stage 2).
- **NFR-9 — Tested as before.** Integration tests with the mock provider
  against real Postgres cover every FR. `go vet ./...` and
  `go test -race -count=1 ./...` are clean, and the integration tests run
  rather than skip.

## 6. Definition of done

These are the orchestration note's checks, plus this spec's own.

1. **`git_status_clean`**: nothing is left uncommitted on the branch.
2. **`tests_pass`**: `go vet ./...` is clean, `gofmt -l .` lists no file
   this milestone touched, and `go test -race -count=1 -v ./...` passes with
   the integration tests seen to run.
3. **`contract_documents_approved`**: Sam approves this spec.
4. **`all_tasks_terminal`**: every task in the
   [development plan](../notes/dev-plan-M14-spikes.md) is done or explicitly
   dropped.
5. **`reviews_present`**: REVIEW-021 is recorded and its findings dealt with
   (§8), and at least one round of both code reviews has no major finding
   open.
6. **`verification_passed`**: every acceptance criterion in §3 and §5 has
   cited evidence.
7. **`worktree_removed`**: `git worktree list` shows only the main checkout.
8. **A browser walkthrough without an AI provider**, with Playwright and the
   pre-installed Chromium: create a spike, see its start screen with the
   budget, start it, see its findings and its tokens against the budget once
   it has ended, and close it. The run itself is driven by a scripted
   provider, and the walkthrough, `docs/walkthrough-spec-021.md`, says how.
9. A handoff, `docs/notes/handoff-M14-2026-10-02.md`, with stage 2 listed as
   remaining work if M13 hasn't merged; and the roadmap's §11 marks M14
   stage 1 done with a pointer to it.
10. **Choices that need Sam's explicit yes:**
    1. a spike is its own table, not a feature row (SD-1);
    2. no merge path, by a detached worktree and the absence of any merge
       code; a run that keeps code on a branch is caught and reported, not
       deleted (SD-2);
    3. a spike always has an initiative, and optionally a feature (SD-3);
    4. four states, closing is final, and the question is fixed once created
       (SD-4);
    5. the budget counts every token, is fixed at the start, and defaults to
       **1,000,000** with a turn cap of 40 (SD-6; the alternative is counting
       only new tokens per call);
    6. the stop never begins a call it can see would cross the budget, may
       overrun by one call's unforeseeable part, and the turn limit ends a
       run the same way (SD-7);
    7. the agent saves a draft as it goes, and the server completes it at a
       stop, with no write-up turn (SD-8);
    8. the findings are created when the run ends, and approved by a person,
       with no agent reviewer (SD-9);
    9. closing doesn't approve the findings, and closing is in the web UI
       only, with no chat relay (SD-10);
    10. a spike can't be a milestone deliverable (SD-11);
    11. asking again closes the first spike and creates a second, which still
        needs a person to start it (SD-13);
    12. open spikes block archiving their initiative (SD-14);
    13. the run has its own role, `spike-runner`, on Sonnet, without
        `report_bug` (SD-16);
    14. stage 2's time box is a deadline fixed at the start and carried by
        the claim as its hard expiry, with M13's inactivity checkpoint still
        applying, and defaults to four hours (§4, S2-6).

## 7. Open questions carried forward

- **Stopping a running spike by hand.** The budget is the only stop in stage
  1. A **Stop now** button needs the dispatcher to cancel a running attempt,
  which it can't do cleanly today.
- **Withdrawing a start** before the run is admitted, as Withdraw does for a
  send.
- **A chat relay for closing a spike**, with the person's words (SD-10's
  alternative).
- **Project-level spikes**, above every initiative.
- **Whether 1,000,000 tokens is the right default.** The live runs will say,
  and prompt caching, once the provider uses it, will change the arithmetic.
- **A spike that edits documents.** The agent can only write its findings. A
  spike that wants to propose a design change says so in What to do next.
- **Existing projects need new files** (FR-10.4). A `subutai upgrade` would
  help every milestone.

## 8. Changes after review

How each [REVIEW-021](../reviews/REVIEW-021-spikes.md) finding was dealt with.

| Finding | What changed |
|---|---|
| R21-1 (material) | The review's simpler design: the draft lives on the spike's row (SD-8, FR-4.3), and the findings document is created when the run ends (SD-9, FR-2.2). Nothing can act on half-written findings, so no refusals are needed in the document service. |
| R21-2 (material) | FR-6.1: exhaustion returns `EndSpike` instead of the checkpoint, and the run is marked `cancelled`. FR-4.1: the planner refuses a spike that isn't running. FR-3.4 and a test. |
| R21-3 (material) | FR-3.3 commits the database first, with a conditional update; FR-4.2 has the planner make the worktree; FR-6.2 makes every step re-runnable; FR-6.4 reconciles at boot and on the heartbeat. Tests for each. |
| R21-4 (material) | SD-7 and FR-5.1: the total is the database's, returned by the update; the callback runs straight after the call; the order of a turn is fixed; the check runs before every call. Two tests for the crossing turn. |
| R21-5 (material) | FR-4.1: a later attempt is given the draft, with a sentence to build on it. SD-15 and a test. |
| R21-6 (material) | SD-6 gives the arithmetic; the default is 1,000,000 and the turn cap 40; choice 5's alternative is the real one (new tokens per call); the cache note is corrected. |
| R21-7 (material) | SD-2 fact 5 and FR-6.3: `base_commit` on the row, the leak check before the discard, the finding in the findings, an audit row and a checkpoint. The goal is reworded. NFR-4's test whitelists a script. |
| R21-8 (material) | Option (b): closing doesn't approve the findings (SD-10, FR-7.2, choice 9). |
| R21-9 (material) | S2-6: the time box is one deadline fixed at the start and carried by the claim as its hard expiry, distinct from M13's inactivity checkpoint, which still applies. S2-1 and FR-1.1: stage 2's migration replaces stage 1's named checks. Choice 14. |
| R21-10 | Moot for the findings, which aren't in the worktree. FR-4.1 says the owner's documents are read as committed, and FR-4.3's test checks the worktree is unchanged. |
| R21-11 | The findings aren't made from placeholders at creation. SD-8 fills Answer and What we found, and `TestWritingFindings` validates each ending. |
| R21-12 | `ended_how = 'concluded'`; "four states, two ways to close"; the prose note keeps *concludes* and *answered* apart. |
| R21-13 | Appendix A lists every site; FR-8.3 names the store change and keeps spike runs out of the feature's tokens and `ActualTokens`. |
| R21-14 | FR-10.3: `run-spike` joins `isWorktreePurpose`; every role file is checked; `report_bug` is refused on the spike runner. FR-9 forbids `save_findings` and `finish_spike` over MCP. |
| R21-15 | SD-14 says what G5 counts, what an override does, and what abandoning a feature does. |
| R21-16 | FR-5.1 adds the transcript's stop entry, and FR-8.4 the run page's outcome sentence. |
| R21-17 | FR-5's budget test is exact: six calls, 900 tokens. NFR-4 names `POST /api/features/start` and the feature page. |
| R21-18 | NFR-6 lists every shared file and the likely overlaps with M13. §6 has the orchestration note's checks, including the development plan. |
| R21-19 | The sentences are rewritten; FR-8.5 defines `/ui/spikes` and links there; FR-7.3 redirects. |

**After the code reviews** (REVIEW-021 §5), two requirements were amended,
each marked where it stands:
- **FR-6.3, the leak check**, twice. Round 1 found it saw only the newest
  commit and failed open, so it now records the refs at the start, reads the
  worktree's own reflog, counts only commits made there, and fails closed.
  Round 2 found a vanished working copy hid its reflog, so the check now fails
  closed then too, runs before a retry remakes a damaged copy, and names the
  limits it accepts.
- **FR-10.3, the loader**. Round 1 found the first wording refused a copied
  `spike-runner.yaml` before its assignment, which broke FR-10.4's upgrade
  path.

**Stage 2** ([REVIEW-021](../reviews/REVIEW-021-spikes.md) §6). The first
draft's S2-1 to S2-9 became SD-17 to SD-25 and FR-11 to FR-18, using
SPEC-020's names: `claim_spike` rather than `claim_task`'s spike form, the
claim's `deadline_at`, `executions.measured`, and `spikes.default_time_box_hours`
rather than `default_time_box`. Then the review's fourteen findings:

| Finding | What changed |
|---|---|
| R21-S2-1 (material) | FR-13.7: the submit holds the worktree's lock and `spikeEndMu` through its ending, and a submit that passed its check is honoured; FR-15.2: an ending of a spike any of whose claims ended `done` is `concluded`; FR-12.4: reconciliation's cases, in order. |
| R21-S2-2 (material) | FR-13.2 says exactly what a spike shares with the task's claim: `lock` and `rules` on the interface, `claimResult` without a task; the spike's own submit and release. |
| R21-S2-3 (material) | FR-13.3: one predicate, `spikeHadWorkingCopy`, replaces `TokensUsed > 0` in the remake and the leak check. |
| R21-S2-4 (material) | FR-14.2: the Inbox reads `release_label` and a new `release_consequence` from the checkpoint; the timeline's spike moment; NFR-12. |
| R21-S2-5 (material) | SD-26 (choice 22): the person who ran a spike may close it, recorded; SD-25 and the claim's rule allow the relay of a person's verdict. |
| R21-S2-6 | FR-13.2: a renewal after the deadline is refused. |
| R21-S2-7 | FR-13.6: the save locks and re-checks, audits and withdraws, in one helper. |
| R21-S2-8 | SD-27: the order of the three locks; the start's make under the worktree's lock. |
| R21-S2-9 | FR-11.1: backfill first; the checks keyed on `started_at`, written with `IS NOT NULL`; no deadline on an agent spike; the end-reason check named. |
| R21-S2-10 | FR-12.2: the agent row when the run starts, as SPEC-020 FR-1.2; one helper with a reference type. |
| R21-S2-11 | FR-14.4: the deadline first, then nothing else. |
| R21-S2-12 | FR-15.4 (choice 23): the rule stays and fails safe; the limit is named. |
| R21-S2-13 | FR-15.3's `time_box` ending in full; FR-15.5, asking again without a budget; FR-12.5, the start notices. |
| R21-S2-14 | The stage 2 definition of done asks for §4's evidence; NFR-8 names the time box and the draft. |

## Appendix A: where an owner or reference type is switched on

Each place that knows only the project, initiatives and features, and what it
does for a spike.

| Site | What it does for a spike |
|---|---|
| `store.OwnerPublicID` (`identity.go`) | Returns the spike's `public_id`, so the findings are `SPK-003-findings`. |
| `ownerCrumb` (`ui_entity_pages.go`) | The findings' crumbs lead to the spike, then its owner. |
| `resolveOwner` (`documents.go`) | Resolves a spike for documents it owns. Attaching or adopting a document to a spike is refused: "A spike's only document is its findings, which its run writes." |
| `mcpResolveOwner` (`mcp_tools.go`) | Refuses a spike as an owner for `attach_document` and `adopt_document`, with the same sentence. |
| `scopeForDocument` (`decisions.go`) | A spike-owned document's branch is the spike's initiative. |
| `snapshot` (`actions.go`) | A spike-owned document has no owning feature; document rules that need one don't fire. |
| `checkContractOwner` (`identity.go`) | `findings` isn't a contract type; nothing changes. |
| `featureRefs` (`observe_reads.go`) | Gains the feature's spikes' audit rows, for the timeline only (FR-8.3). |
| `runContext` and `runPurpose` (`observe.go`) | Crumbs to the spike and its owner, and "running a spike". |
| `isWorktreePurpose` (`compartment.go`) | Gains `run-spike`. |
| The G5 callers | Count open spikes in the initiative's subtree. |
| `handleUIByID` (`ui.go`) | `SPK-003` redirects to `/ui/s/SPK-003`. |
