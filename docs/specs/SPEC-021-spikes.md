# SPEC-021: Spikes

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. An independent
consistency review is recorded in
[REVIEW-021](../reviews/REVIEW-021-spikes.md). It found nine material and ten
smaller problems in the first draft. All are dealt with in this revision, and
§8 says how, finding by finding. The choices Sam must confirm are listed in
§6, DoD 10. The build does not wait for approval (the M14 brief).
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

### Stage 2, specified now and built after M13 merges

11. Spikes run by the chat AI or a person, held to a time box through their
    claim, with unmeasured tokens (§4).

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

**FR-6.3 — The leak check.** If the worktree's `HEAD` differs from
`base_commit`, `EndSpike` lists the commits made there (`git rev-list HEAD
--not <base_commit>`) and, for the newest, the refs that contain it (`git
for-each-ref --contains`). Every ref it finds is a leak:
- it is named in How this spike ended (FR-2.3);
- `spike.code_kept` is audited, with the refs;
- a `spike-code-kept` checkpoint on the spike asks: "Code from SPK-003 was
  kept on `keep-this`, outside its working copy. A spike's code is never
  merged. Delete the branch, or keep it knowing it won't be built from."
  The answer is "I've dealt with it", and it changes nothing in Subutai.

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
`save_findings`. `run-spike` joins `isWorktreePurpose`. The loader checks
**every role file**, assigned or not:
- `save_findings` is refused on a role that isn't assigned `run-spike` only:
  "save_findings is for the spike runner, so role X can't be offered it.";
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

**Built only after M13 merges into `main`.** M13's spec, SPEC-020, wasn't on
`main` when this was written, so the names below follow DEC-007 and the M14
brief. Where SPEC-020 settles a name or a mechanism differently, SPEC-020
wins; the stage-2 build follows it and records the change.

**S2-1 — The executor.** A spike gains `executor`: `agent` (stage 1),
`chat` or `person`. It is recorded at the start and shown on the spike's page
and in `get_spike` (DEC-007 decision 1). Stage 2's migration adds
`executor`, `time_box` and `deadline`, and replaces stage 1's named checks:
a token budget is required only for an `agent` spike, and `ended_how` gains
`time_box`.

**S2-2 — Starting for chat or a person.** The start screen gains a choice,
**Who runs it**: the spike runner (an agent, with a token budget), the chat
AI, or a person. For the chat AI or a person:
- the budget field becomes a **time box**, filled from
  `spikes.default_time_box` (4 hours) or the spike's override;
- the screen says "Work done in chat or by hand can't be measured in tokens,
  so this spike is held to a time box instead. When the time box runs out,
  the spike ends with whatever findings have been saved.";
- starting is still a web-UI act. It records the **deadline** (the start
  plus the time box), moves the spike to `running` with no dispatch, makes
  the worktree, and waits for a claim.

**S2-3 — Claiming.** The chat AI claims a started spike with M13's claim
mechanism: `claim_task`'s spike form, or `claim_spike` if SPEC-020 keeps
claims per entity. A person claims it in the web UI with **I'll run this
spike**. The claim carries the spike's deadline as its expiry. It returns
what a dispatched spike's prompt holds (FR-4.1): the worktree's path, the
question, the surfaced decisions, the owner's documents, and, for a spike
that asks again, the earlier findings. Only the executor chosen at the start
can claim it.

**S2-4 — Working.** The claimant saves the draft with `save_spike_findings`
over MCP, or in a draft editor on the spike's page. The worktree is theirs
for scaffolding, and it is discarded at the end, as in stage 1.

**S2-5 — Submitting.** `submit_spike` over MCP, or **I've finished** in the
web UI, ends the run as `concluded` (FR-6), after validating the findings.

**S2-6 — The time box is held through the claim.** DESIGN-010 §10 says a
chat or human spike's time box is "enforced through claim expiry". DEC-007
decision 6 defines claim expiry as an inactivity checkpoint, "still working on
this?". Both hold:
- **the time box is the claim's hard expiry.** A spike's claim carries one
  deadline, fixed at the start, and the heartbeat enforces it. When the
  deadline passes, claimed or not, `EndSpike` runs with `ended_how =
  'time_box'`. The draft is completed as in SD-8: "The spike reached the end
  of its time box of 4 hours." The worktree is discarded;
- **M13's inactivity checkpoint still applies** to the claim before the
  deadline, as a prompt to the person, not as the stop.

There is one clock, from the start (R21-9).

**S2-7 — Unmeasured.** A chat or person spike's tokens are recorded as
*unmeasured*, using M13's marker. Its page shows "This spike ran in chat, so
its tokens weren't measured." (or "by hand") in place of the bar.
Calibration and the start screen's forecast leave it out.

**S2-8 — Judging stays closed.** Closing a spike is a person's act in the web
UI, whoever ran it (SD-10). The chat AI that ran a spike can't approve its
findings or close it.

**S2-9 — Integration checks** (from the orchestration note):
- a spike run in chat or by a person is held by its claim's expiry;
- its executor is recorded;
- a chat-run spike's tokens are marked unmeasured;
- neither milestone's tools escape the MCP tool-set test.

**Stage 2 acceptance:**
- a chat spike claimed over MCP, submitted, and closed by a person;
- a person spike claimed in the UI;
- a deadline that passes ends the spike with its findings, and discards its
  worktree;
- the executor and the unmeasured marker on the page and in `get_spike`;
- the tool-set test passes with M13's and M14's tools together.

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
    The agent runs it, and it commits in the worktree and creates a branch
    there. After the end, the worktree's commit is reachable from no ref but
    that branch, the findings name it, and the `spike-code-kept` checkpoint
    exists.
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
  typed values are the question and the budget.
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
