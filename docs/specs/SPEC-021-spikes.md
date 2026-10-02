# SPEC-021: Spikes

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. An independent
consistency review is recorded in
[REVIEW-021](../reviews/REVIEW-021-spikes.md), and §8 says how each finding
was dealt with. The choices Sam must confirm are listed in §6, DoD 9. The build
does not wait for approval (the M14 brief).
**Date:** 2026-10-02
**Roadmap milestone:** M14 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11. Roadmap decision 13 (§12) is part of the brief: **a hard stop, with a
project-wide default budget** that a spike can override.
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28). **§10 is the main source.** Also §4 (a spike is "a question with
a budget"), §5b (doing and judging stages), §7 (the `SPK-` prefix and the
default folder), §8 (tokens and unmeasured work), §11 (decisions pushed into
every dispatch) and **§17a item 7, accepted**: a human starts a spike in the
web UI, any executor runs it, and a chat or human spike carries a time box
instead of a token budget.
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
  (already made), and documents created with their entity;
- [SPEC-018](SPEC-018-decisions.md): decisions surfaced into every dispatch;
- [SPEC-019](SPEC-019-bugs.md): how M12 added an entity. A spike is **not**
  feature-shaped, and this spec says why (SD-1).

**Coordination:** M13 (executors) is built in parallel and merges first. It
owns migration `0014`, `claim_task`, `submit_task`, the executor on tasks,
claim expiry, the unclaimed-commit check and unmeasured tokens. This spec owns
migration `0015`. Stage 1 touches only these shared files: `mcp.go`'s tool
list, `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`, and `entity.html`
(one include). **Stage 2 (§4) is built only after M13 has merged into
`main`.**

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6). British spelling.

Four words are kept apart throughout:

- a **spike** is the entity, `SPK-003`;
- its **question** is the one sentence it exists to answer;
- its **findings** are its one document, `SPK-003-findings`;
- its **run** is the dispatch that does the work, purpose `run-spike`.

A spike's **budget** is a number of tokens. "Tokens used" always means every
token the provider reported for the spike's turns: input, output, cache reads
and cache writes, the same sum the run pages already show (SD-6).

## 0. Framing

Sometimes a design can't be written because nobody knows enough yet. Does the
API do what its documentation says? Is the approach fast enough to bother
with? DESIGN-010 §10 answers with a spike: a question with a budget, which
produces findings and never shipped code.

Today, the only way to send an agent to find something out is to make a
feature, which commits scope, writes a spec, and builds and merges code. That
is the wrong shape. A spike has no spec, no plan, no tasks and no merge. It
has a question, a budget, a throwaway working copy and one document.

So this spec adds a small, separate entity. A person creates a spike on an
initiative or a feature, and starts it from the web UI, seeing what will run
and the budget. One agent works on the question in a detached worktree and
writes its findings as it goes. The run stops hard at the budget. When it
ends, for whatever reason, the findings are written up and committed, and the
worktree is thrown away. A person reads the findings and closes the spike, as
answered or not; if it wasn't answered, they may ask again with a second,
separately budgeted spike.

## 1. Goal

**One claim, which the roadmap's done-when states directly:**

> A spike hits its budget and stops, and nothing can merge its code.

Four supporting claims:

> Only a person, in the web UI, can start a spike, and the start screen shows
> what will run and the budget before anything is committed.

> When a spike's run ends — with an answer, at its budget, at its turn limit
> or by failing — its findings are written up and committed, and its worktree
> is discarded.

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
6. Ending a run: the write-up and the discarded worktree (FR-6).
7. Closing a spike, and asking again (FR-7).
8. Where spikes show: pages, the timeline, runs and the Inbox (FR-8).
9. MCP: create, list and get (FR-9).
10. The starter pack and configuration (FR-10).

### Stage 2, specified now and built after M13 merges

11. Spikes run by the chat AI or a person, held to a time box through M13's
    claims, with unmeasured tokens (§4).

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| A spike as a milestone deliverable | Not planned. A spike ships nothing (SD-11). |
| Stopping a running spike by hand | §7. The budget is the stop in stage 1. |
| Withdrawing a start before the run begins | §7. Admission is immediate when a slot is free. |
| Editing a spike's question after it is created | §7. Close it and create another (SD-4). |
| Promoting a spike into a feature | Never. Promotion is one-way through a design (DESIGN-010 §10, SD-12). |
| Spikes owned by the project itself | §7. A spike hangs off an initiative or a feature, as §10 says. |
| A `relay_close_spike` tool for the chat agent | §7. Closing is a person's act in the web UI in this milestone (SD-10). |

### Scope decisions

- **SD-1 — A spike is its own table, not a feature row.** M12 made a bug a
  `features` row because everything after a bug's acceptance *is* the feature
  pipeline (SPEC-019 SD-1). A spike shares none of it: no spec, no plan, no
  tasks, no estimate, no gates G1 to G3, no merge, no milestone membership.
  Putting it in `features` would mean filtering it out of every one of those
  places, and each missed filter would be a path to building or merging a
  spike. A separate table, `spikes`, has none of those columns or
  relationships, so the feature code can't act on it by accident. This is the
  first half of SD-2.

- **SD-2 — No merge path, enforced by absence.** DESIGN-010 §10: "There is no
  merge path in the entity at all." Four facts make that true, and each is
  tested:
  1. **The worktree is detached** (`git worktree add --detach`). There is no
     branch to merge, push or open a pull request from.
  2. **Nothing in the spike's code calls a merge.** The merge
     (`mergeFeature`, `mergeBranch`) takes a feature and reads the
     `worktrees` table; a spike's worktree is recorded on its own row, not in
     `worktrees`, so no feature query can find it.
  3. **No rule emits a merge for a spike**, and no route, API or MCP tool
     exists that could ask for one.
  4. **The worktree is removed** when the run ends (FR-6), with `--force`, so
     whatever the agent left there, committed or not, is gone. A commit made
     on the detached `HEAD` is unreachable once the worktree goes, and git's
     own collection removes it in time.

  The findings are the only thing that leaves the run, and they are written
  to the main checkout by the server, never from the worktree (FR-4.3).

- **SD-3 — A spike hangs off an initiative, and optionally a feature.** As a
  bug does (SPEC-019 SD-3), every spike has an initiative: the one it was
  created on, or the initiative of the feature it was created on. The feature,
  when there is one, is its **feature**. Its findings live in that
  initiative's folder, and its page is reached from the owner's.

- **SD-4 — Five states, and the question is fixed.** A spike is:
  - `idea`: written down, not started;
  - `running`: started, with its run queued or running and its worktree live;
  - `ended`: the run is over, the findings are committed, the worktree is
    gone, and it waits for a person;
  - `closed`, as `answered` or `unanswered`: a person has read it.

  An `idea` can also be closed, unanswered, without running. `closed` is the
  only terminal state, and nothing reopens it. The question can't be edited
  after creation: it is what the budget was spent on, and a different question
  is a different spike. A wrongly worded idea is closed and created again.
  **Flagged for Sam.**

- **SD-5 — Started in the web UI only, like Send to development.** Starting
  spends tokens, so it is a person's act at the command centre (§17a item 7,
  DEC-006). There is a start screen and a `POST`, and no `/api` route, CLI
  verb or MCP tool that starts a spike. The MCP tool-set test names
  `start_spike` and `run_spike` as forbidden.

- **SD-6 — The budget counts every token, and is fixed at the start.** A
  spike's **budget** is set when it is started: the per-spike override if it
  has one, otherwise `spikes.default_token_budget` from `config.yaml`
  (roadmap decision 13). The start screen shows the figure and lets the person
  change it for this start. It is recorded on the row, and the config is not
  read again for that run. Tokens used are input + output + cache reads +
  cache writes, as `RunSummary.Tokens` counts them, so the number on the
  spike's page is the number the run page shows. Cache reads are cheap in
  money but they are tokens, and leaving them out would make the "hard stop"
  depend on caching luck. **Flagged for Sam:** the alternative is input +
  output only.

- **SD-7 — The stop is hard, and never begins a turn it can see would cross
  the budget.** The dispatcher checks the budget after every turn, and before
  every model call:
  - once the tokens used reach the budget, the run stops;
  - before a call, it stops if the tokens used plus a **lower bound** for the
    next call would pass the budget. The next call's input holds everything
    the last call read and wrote, so the lower bound is the last turn's input,
    cache reads, cache writes and output.

  A turn can still take the run past its budget by what it adds on top of
  that bound (the tool results it fed back, and what the model writes). That
  overrun is bounded by one turn and is shown honestly as the true figure,
  for example "41,210 of 40,000 tokens (stopped at the budget)". No polite
  question is asked: the run simply ends (DESIGN-010 §10). The spike's role
  has a turn cap too; reaching it ends the run the same way, as "stopped at
  its turn limit", not as a failure. **Flagged for Sam.**

- **SD-8 — The agent writes findings as it goes.** A hard stop can't ask the
  agent to write up, because that would spend more tokens. So the agent has a
  `save_findings` tool and is told to use it early and often. Each call
  replaces the findings' body in the main checkout (never the worktree). When
  the run stops at its budget, its turn limit or a failure, **the findings so
  far are the last saved body**, and the server completes them: it fills an
  empty Answer with "Not answered: the spike stopped at its budget before it
  reached an answer." (or the matching reason) and writes the closing section,
  **How this spike ended**, with the reason, the tokens used and the budget.
  A run that finishes normally calls `finish_spike` with its final findings,
  which are validated against the template first. **Flagged for Sam:** the
  alternative, a final no-tools turn to write up, breaks the hard stop.

- **SD-9 — The findings have the ordinary lifecycle, with a person as
  approver.** `findings` is a document type with its own template, owned by
  the spike, `SPK-003-findings`. It is created with the spike, from the
  template, with the question filled in, and committed (DESIGN-010 §7,
  "creating work creates its documents"). Its manifest says
  `approved_by: human` and names no reviewer role: the findings are read by
  the person who decides whether the question is answered, and no agent
  review is dispatched. It can be edited in the browser, submitted, approved,
  sent back and superseded like any other document.

- **SD-10 — A person closes the spike, and "answered" approves the findings.**
  The spike is done when a person reads the findings and says the question is
  answered (§10). On an ended spike the page offers:
  - **The question is answered**: closes the spike as `answered` and, if the
    findings aren't already approved, records the person's approving verdict
    on them in the same act. The findings must pass validation, as any
    approval does;
  - **Close without an answer**: closes it as `unanswered`, and leaves the
    findings as they are;
  - **Ask again with a new budget**: see SD-13.

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
  The closed spike's page says what to do instead: "To build on this, cite
  SPK-003-findings in a design, then create a feature in the normal way."

- **SD-13 — Asking again makes a new spike.** On an ended spike, **Ask again
  with a new budget** closes it as `unanswered`, creates a new spike with the
  same owner and question, a link back (`follows`), and the budget the person
  enters, and opens the new spike's start screen. Nothing is started until the
  person presses Start there. The new run's prompt includes the earlier
  findings, so it starts from what is already known. It is a separate spike
  with its own number, budget, findings and worktree (roadmap decision 13).

- **SD-14 — Open spikes block archiving their initiative.** G5 refuses to
  archive an initiative with open work under it. A spike that isn't `closed`
  is open work: a running one is spending tokens, and an ended one waits for a
  person. So it blocks archiving, with a sentence naming it. **Flagged for
  Sam.**

- **SD-15 — One run per spike.** The run is one dispatch row, idempotency key
  `run-spike:<spike id>`. Its attempts share the worktree and the budget: a
  retry after a failed attempt carries on with the tokens already used
  counted (FR-5.3). When the dispatcher gives up on it (exhausted), the run
  has failed and ends as in FR-6.

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
| `tokens_used` | tokens, 0 until the run starts, updated after every turn |
| `ended_how` | `answered`, `budget`, `turn_limit` or `failed`, set at the end of the run |
| `closed_as` | `answered` or `unanswered`, set when closed |
| `follows_id` | the spike this one asks again for, or null |
| `worktree_path` | the worktree's path relative to the repository, set at the start |
| `worktree_removed_at` | when the worktree was discarded |
| `created_by`, `created_via` | who created it, and `ui` or `mcp` |
| `started_by`, `started_at`, `ended_at`, `closed_by`, `closed_at` | who and when |

Checks the database enforces:
- `token_budget` is set exactly when the state isn't `idea`, unless the spike
  was closed from `idea`;
- `ended_how` is set exactly when the spike has run and ended (`ended`, or
  `closed` after running);
- `closed_as` is set exactly when the state is `closed`;
- a spike closed from `idea` is `unanswered`;
- budgets are positive, and `tokens_used` isn't negative;
- a spike doesn't follow itself.

`ref_type` gains `spike`, for dispatches, audit rows and document owners.
`document_type` gains `findings`. The documents table's owner check admits
`spike`. Adding an enum value and using it in the same transaction is refused
by Postgres, so the new owner check compares `owner_type::text`, and no row in
the migration uses the new values.

**FR-1.2 — IDs.** `ident.Kinds` gives `spike` its table, `spikes`.
`ident.DocTypes` gains `findings`. A spike's ID resolves wherever an entity's
does: `/ui/id/SPK-003`, `get_spike`, and the `on` argument of `create_spike`
doesn't take one (a spike can't own a spike). `OwnerPublicID` handles a spike,
so its findings are `SPK-003-findings`.

**FR-1.3 — Creating a spike.** One service method, `CreateSpike`, serves the
web UI and MCP, in one transaction with its audit row (`spike.created`):
1. it resolves the owner, which must be a live initiative or a feature that
   isn't `done` or `abandoned`, and refuses otherwise with a sentence;
2. it validates the question (non-blank after trimming, at most 500
   characters) and the override (a positive whole number, if given);
3. it fills and validates the findings from the template, **before** a number
   is minted;
4. it inserts the row, mints the findings' identity, writes the file at
   `docs/work/<INIT-ID>-<slug>/SPK-003-findings.md`, registers it as a draft
   owned by the spike, and marks it the spike's primary document;
5. after the commit, it commits the file (`subutai` as the author, as for a
   bug report) and signals the owner's page.

A person creates a spike from a **New spike** dialog on an initiative's or a
feature's page, with the question and an optional budget. The chat agent
creates one with `create_spike` (FR-9).

**FR-1.4 — Closing from `idea`.** An `idea` spike's page has **Close without
running**, which closes it as `unanswered`, with no run and no worktree.

**Acceptance:**
- creating from each surface gives `SPK-001`, `SPK-002` in order, with the
  findings file, its front matter, its draft state, its commit, and the audit
  row;
- a blank question, a 501-character question, a zero or negative override,
  a `done` feature and an archived initiative are each refused with a
  sentence, and no number is minted;
- the database refuses a row that breaks each check.

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

The template's Question holds the question; Answer and What we found are
placeholders; the optional sections are left out until written. The title is
"SPK-003: <question, shortened to 80 characters>".

**FR-2.2 — How this spike ended.** This section is the server's. When a run
ends (FR-6), the server writes it last, replacing any section the agent wrote
with that heading:

> The spike's agent concluded. It used 31,004 of its 40,000 tokens.

> The spike stopped at its budget. It used 41,210 of its 40,000 tokens. The
> findings above are what it had saved by then.

> The spike stopped at its turn limit of 60 turns, having used 18,220 of its
> 40,000 tokens. The findings above are what it had saved by then.

> The spike's run failed: <the last error, as one line>. It used 2,100 of its
> 40,000 tokens. The findings above are what it had saved by then.

**Acceptance:** `TestFindingsTemplate` fills and validates the template,
refuses a body without Answer, and shows the section written once, last, for
each ending.

### FR-3: Starting, in the web UI only

**FR-3.1 — The button.** An `idea` spike's page shows **Start this spike…**
as its primary action, linking to the start screen. Nothing else on any page
starts a spike.

**FR-3.2 — The start screen** (`GET /ui/spikes/start?spike=<uuid>`) shows,
before anything is committed, in the send screen's style:
- the question, and the owner;
- **who will run it**: "The spike runner (*model*) runs it", from the
  `run-spike` assignment and the model it would be dispatched on; or, when
  `run-spike` isn't assigned, "Nobody is assigned to run spikes, so this
  spike can't start. Assign `run-spike` to a role in `config.yaml`." and the
  button is disabled;
- **the budget**, as a number field, filled with the override, or with the
  project default and a line saying where it comes from: "The project's
  default budget, from `spikes.default_token_budget`.";
- what happens at the budget: "The run stops when it reaches this many
  tokens. Whatever findings it has saved by then are written up, and you
  decide whether to ask again.";
- **what the agent will be given**: the question; the decisions and
  conventions that will be pushed into its prompt (the same block a dispatch
  receives, FR-4.1), or "No decisions apply here."; the earlier findings, for
  a spike that asks again; and its tools by name;
- **where it works**: "It works in a throwaway copy of the code, made from
  the current main line. Nothing it writes there is kept: the copy is
  discarded when the run ends, and nothing can be merged.";
- free agent slots, as on the send screen;
- a token forecast from earlier spikes' runs, when there are at least three,
  as the send screen forecasts a step; otherwise "no forecast yet".

**FR-3.3 — Starting** (`POST /ui/spikes/start`, fields `spike` and
`budget`). The server re-checks that the spike is `idea`, that `run-spike` is
assigned and its model configured, and that the budget is a positive whole
number. Then:
1. it adds the worktree, detached at the main checkout's `HEAD`, at
   `<compartment>/worktrees/spk-<short id>`. If that fails, the start is
   refused with the reason and nothing changes;
2. in one transaction, it records the budget, `started_by`, `started_at` and
   `worktree_path`, moves the spike to `running`, enqueues the `run-spike`
   dispatch on `ref_type = 'spike'`, and writes `spike.started`, with the
   budget and where it came from. If the transaction fails, the worktree is
   removed;
3. it kicks the dispatcher and returns to the spike's page with a notice:
   "SPK-003 has started, with a budget of 40,000 tokens."

**FR-3.4 — No other route starts a spike.** No `/api/*` route, CLI verb, MCP
tool or agent tool moves a spike to `running` or enqueues `run-spike`.

**Acceptance:**
- `TestSpikeStartsFromTheWebUIOnly`: the screen renders each part (with and
  without decisions, with the override and with the default, with the role
  unassigned and the button disabled); the `POST` starts it once and a second
  `POST` is refused; `/api/spikes/start` and similar paths are 404 or 405;
  every MCP name in the forbidden list returns method-not-found; and
  `create_spike` leaves the spike an `idea` with no dispatch.

### FR-4: The run

**FR-4.1 — The prompt.** The planner's `run-spike` case assembles:
- the surfaced decisions and conventions, from the spike's initiative and
  its ancestors (SPEC-018 FR-6.1), first, as every dispatch has them;
- "# The question" and the question;
- "# Where it came from": the owner's name and description, and the paths of
  its current documents, so the agent can read them;
- for a spike that asks again, "# What the last spike found" and the earlier
  findings' body;
- "# Your budget": the budget, and the sentence "The run stops without warning
  when it reaches this many tokens. Save your findings with `save_findings`
  early and often: whatever you have saved when it stops is what is kept.";
- "# Your working copy": that it is a throwaway copy, that nothing written
  there is kept or merged, and that findings go through `save_findings`, never
  into a file.

The system prompt is the role's identity and the `run-spike` skill, as for
every role.

**FR-4.2 — The tools.** The role's tools, resolved as for any role: `read_file`,
`list_files`, `edit_file`, `write_file` and `run_command` work in the spike's
worktree; `save_findings` is new; `finish_spike` is the outcome. A
`toolhost.Context` for a spike carries the worktree and the spike's ID, and no
feature or task.

**FR-4.3 — `save_findings`.** Input: `findings`, the Markdown body (no front
matter). It replaces the body of the spike's findings file in the main
checkout, keeping the front matter. It doesn't validate, commit or change the
document's state. It refuses, as a tool error the agent reads, when the
findings aren't a draft any more ("A person has submitted these findings for
review, so they can't be changed now. Carry on and finish with what you
have.") and when it's called by anything but a `run-spike` dispatch. It
returns "Saved. If the run stops now, this is what is kept."

**FR-4.4 — `finish_spike`.** Input: `findings`, the final body. It is
validated against the `findings` manifest (an invalid body is returned to the
agent, as any invalid outcome is). On success the run ends as `answered`
(FR-6). "Answered" here means the agent concluded; whether the question *is*
answered is the person's call (SD-10).

**Acceptance:**
- `TestSpikePromptCarriesDecisionsAndQuestion`: a project decision and an
  initiative decision appear in the spike's transcript prompt, and a sibling
  initiative's doesn't; the question, the budget sentence, and the earlier
  findings for a spike that asks again are there;
- `TestSaveFindingsWritesTheMainCheckout`: the file in the main checkout
  changes, the worktree has no findings file, and a submitted document is
  refused.

### FR-5: The budget and the hard stop

**FR-5.1 — In the dispatcher.** `dispatch.Plan` gains an optional budget: the
limit, the tokens already used by earlier attempts, a callback that records a
turn's usage, and the outcome to return when the run stops early. When a
plan has one, `runLoop`:
- after each turn, calls the callback with the turn's usage, and stops if the
  tokens used (earlier attempts plus this attempt) have reached the limit;
- before each model call after the first, stops if the tokens used plus the
  lower bound (SD-7) would pass the limit;
- at the turn cap, stops instead of failing.

Stopping returns the plan's stop outcome, `{"ended":"budget"}` or
`{"ended":"turn_limit"}`, as a successful attempt, so the attempt's tokens are
recorded on the dispatch row as any success's are. Plans without a budget
behave exactly as now.

**FR-5.2 — Recording use.** The callback adds the turn's tokens to
`spikes.tokens_used` in its own short transaction, so the figure is right even
if the attempt later fails. The planner gives the loop `tokens_used` as the
earlier attempts' total.

**FR-5.3 — Retries.** A failed attempt is retried by the existing sweep, up to
`dispatch.max_attempts`, with the same worktree. The next attempt starts with
the tokens already used; if they have reached the budget, it stops before its
first call.

**Acceptance:**
- `TestSpikeStopsHardAtItsBudget` (mock provider): with a budget of 1,000 and
  turns of 150 tokens that never finish, the run stops after the turn that
  reaches the budget or the turn the lower bound rules out; no further
  request reaches the provider (the mock's script has unused steps); the
  spike is `ended` with `budget`; `tokens_used` equals the sum of the
  scripted usage; and the findings hold the last saved body and the ending
  section;
- `TestSpikeLowerBoundStopsBeforeTheCall`: a turn whose usage makes the next
  call certain to pass the budget ends the run without that call;
- `TestSpikeTurnLimitEndsTheRun`: reaching the turn cap ends it as
  `turn_limit`, not failed;
- `TestSpikeBudgetCarriesAcrossAttempts`: an attempt that fails after using
  tokens is retried, and the retry stops at the budget counting both;
- a dispatch without a budget is unchanged (the existing suite).

### FR-6: Ending a run

**FR-6.1 — When.** A run ends when its dispatch succeeds (with `finish_spike`
or a stop outcome) or when it is exhausted. The rules engine decides
`EndSpike` for a `run-spike` dispatch's success and for its exhaustion.

**FR-6.2 — What `EndSpike` does.** In order:
1. it writes the findings: the final body for `answered`, or the last saved
   body for the others, with an empty or placeholder Answer filled (SD-8),
   and the ending section (FR-2.2);
2. it commits the findings file in the main checkout, as `subutai`, with the
   message "SPK-003: findings (stopped at the budget)" or the matching
   reason;
3. in one transaction, it sets `ended_how` and `ended_at`, moves the spike to
   `ended`, updates the document's hash, and writes `spike.ended` with how it
   ended, the tokens used and the budget;
4. it discards the worktree: `git worktree remove --force`, then
   `worktree_removed_at` and `spike.worktree_discarded`.

`EndSpike` acts once: a second call on an `ended` spike does nothing.

**FR-6.3 — A discard that fails is retried.** If removing the worktree fails,
the spike still ends, the error is logged, and the heartbeat retries the
removal for every spike that isn't `running` and has a worktree path with no
removal time. At boot, a `running` spike whose worktree is missing gets it
re-added; any other spike's leftover worktree is removed.

**Acceptance:**
- `TestSpikeWorktreeIsDiscardedWhenItEnds`: for each ending (answered,
  budget, turn limit, failed), the worktree directory is gone, `git worktree
  list` doesn't list it, `worktree_removed_at` is set, and the audit row is
  written;
- `TestSpikeDiscardIsRetried`: a removal that fails once is completed by the
  next heartbeat.

### FR-7: Closing, and asking again

**FR-7.1 — On an ended spike's page:** the three actions of SD-10, each a
form posting to `POST /ui/spikes/close` with `spike` and `as`
(`answered`, `unanswered` or `again`), plus `budget` for `again`.

**FR-7.2 — Answered.** In one transaction: the findings, if not approved,
get the person's approving verdict through the document service (validation
first; a failure refuses the close with the validation's sentence); the spike
is closed as `answered`; `spike.closed` is written. The findings' commit, if
the approval changes the file, follows the document service's rule.

**FR-7.3 — Unanswered.** The spike is closed as `unanswered`. The findings
are left in whatever state they are.

**FR-7.4 — Again.** In one transaction: the spike is closed as `unanswered`,
and a new spike is created as in FR-1.3, with the same owner and question,
`follows_id` set, and the budget entered as its override. The response is the
new spike's start screen.

**FR-7.5 — Closing is final, and only in the web UI.** A `closed` spike shows
no actions. No API route or MCP tool closes a spike.

**Acceptance:**
- `TestPersonClosesASpike`: answered approves draft findings and closes; a
  spike whose findings fail validation can't be closed as answered, with
  the sentence; unanswered leaves the findings a draft; closing twice is
  refused; closing a `running` spike is refused;
- `TestAskingAgainMakesASecondSpike`: a new `idea` spike with its own number,
  the same question, the link both ways on the pages, the new budget, and
  the first spike closed.

### FR-8: Where spikes show

**FR-8.1 — The spike's page** (`/ui/s/<SPK-ID>`) shows:
- the question, the owner (linked), and the state as a sentence: "Not started
  yet.", "Running.", "Ended: stopped at its budget. It's waiting for you to
  read the findings.", "Closed: the question is answered.";
- **tokens against the budget**: a bar and "31,004 of 40,000 tokens", or the
  default budget before it starts ("It will have the project's default
  budget of 40,000 tokens, unless you change it when you start it.");
- the findings, linked to the document page, with their state;
- the worktree: "Its working copy is live." while running, "Its working copy
  was discarded on 2 October at 14:05." after;
- the actions for its state (FR-1.4, FR-3.1, FR-7);
- **its timeline**: Created, Started (with the budget), Ended (how, with the
  tokens), Working copy discarded, Closed (as what, by whom), each linking to
  the run where there is one;
- its run, linked to the run page and its transcript;
- the spike it follows, and the spike that follows it;
- for a closed, answered spike, SD-12's sentence.

**FR-8.2 — On the owner's page.** An initiative's and a feature's pages gain a
**Spikes** section listing their spikes, open ones first, each with its ID,
question, state and "tokens used of budget", and the **New spike** button. An
initiative lists only spikes it owns directly, not its features'. The markup
lives in `spike.html`; `entity.html` gains one include.

**FR-8.3 — On the feature's timeline.** A feature's timeline shows its
spikes' moments as "Spike SPK-003 started" and "Spike SPK-003 ended: stopped
at its budget", linking to the spike.

**FR-8.4 — Runs and transcripts.** A `run-spike` run's page names its purpose
"running a spike", its crumbs lead to the spike and its owner, and its
transcript is the ordinary transcript. The tokens on the run page are the
dispatch row's; the spike's page shows `tokens_used`, which includes failed
attempts.

**FR-8.5 — The Inbox.** The Inbox page shows a line when spikes have ended
and wait for a person: "2 spikes have ended and are waiting for you to read
their findings.", linking to the first, or to a list when there are several
(`/ui/spikes`). The Inbox badge still counts checkpoints only, as SPEC-019
SD-12 kept it.

**FR-8.6 — Not deliverables.** The milestone editor doesn't offer spikes, and
`add_milestone_member` refuses one: "A spike can't be a milestone
deliverable, because it ships nothing. Add the feature its findings led to
instead." (SD-11)

**Acceptance:** `TestSpikePagesShowTheWork` renders the spike's page in each
state, the owner sections, the feature's timeline moments, the run page's
crumbs, the Inbox line, and the refusal from `add_milestone_member`.

### FR-9: MCP

Three tools, in `mcp_spike_tools.go`, appended to the registry with one line:

- **`create_spike`** — "Write down a question to investigate as a spike, on an
  initiative or a feature. This creates the spike and its findings document;
  it does not start it. A person starts a spike from the web UI, where they
  see the budget." Arguments: `on` (an initiative or feature, by ID or path),
  `question`, and an optional `budget`. No quote: creating a spike is planning
  authoring (DEC-004). The result gives the ID, the findings' ID and path, and
  the sentence "A person can start it from its page in the web UI."
- **`list_spikes`** — optional `on` and `state`. Each spike's ID, question,
  state, owner, budget (or the default it would get), and tokens used.
- **`get_spike`** — by ID. Everything `list_spikes` gives, plus how it ended,
  how it was closed, the findings' ID, path and state, the run's ID, and the
  spikes it follows and is followed by.

`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` gains the three names in
its want list, and `start_spike`, `run_spike`, `close_spike`, `answer_spike`,
`merge_spike` and `promote_spike` in its forbidden list. `mcp.go`'s
`initialize` instructions gain one sentence: "You may write down a spike's
question with `create_spike`; only a person can start or close one."

**Acceptance:** `TestSpikeMCPTools`: each tool's result and refusals; a spike
created over MCP records `created_via = 'mcp'`; the tool-set test passes.

### FR-10: The starter pack and configuration

**FR-10.1 — Configuration.** `config.yaml` gains:

```yaml
spikes:
  default_token_budget: 200000
```

with the default filled when absent, refused when not positive, and read
fresh at each use (O-6). The generated config includes the section with a
comment, and `assignments` gains `run-spike: spike-runner`.

**FR-10.2 — The role and skill.** `roles/spike-runner.yaml` (model
`claude-sonnet-5`, skill `run-spike`, tools `read_file`, `list_files`,
`edit_file`, `write_file`, `run_command`, `save_findings`, turn cap 60) and
`skills/run-spike/SKILL.md`, which says:
- answer the question, and only the question;
- work in the throwaway copy, and treat what you write there as scaffolding;
- save findings after each thing you learn, because the run can stop at any
  turn;
- say what you found, how you found it out, and how sure you are; say plainly
  when the evidence is thin;
- finish with `finish_spike` when you have an answer, or when you can say
  why it can't be answered;
- never write findings into a file in the working copy.

**FR-10.3 — Known tools.** `config.KnownTools` gains `save_findings`. The
config loader refuses `save_findings` on a role that isn't assigned
`run-spike`, as it refuses `report_bug` where it can't be offered (SPEC-019
SD-10).

**FR-10.4 — Existing projects** copy `templates/findings/`,
`roles/spike-runner.yaml` and `skills/run-spike/` from a fresh `subutai init`
and add the assignment; until they do, the start screen says nobody is
assigned (FR-3.2).

**Acceptance:** config defaults and refusals; the loader's refusal; a fresh
`subutai init` has the files and the assignment.

## 4. Stage 2: spikes run by the chat AI or a person

**Built only after M13 merges into `main`.** M13's spec, SPEC-020, wasn't on
`main` when this was written, so the names below follow DEC-007 and the M14
brief. Where SPEC-020 settles a name or a mechanism differently, SPEC-020
wins, and the stage-2 build follows it and records the change.

**S2-1 — The executor.** A spike gains `executor`: `agent` (stage 1),
`chat` or `person`, recorded at the start and shown on its page and in
`get_spike` (DEC-007 decision 1).

**S2-2 — Starting for chat or a person.** The start screen gains a choice,
**Who runs it**: the spike runner (an agent, with a token budget), the chat
AI, or a person. For the chat AI or a person, the budget field becomes a
**time box**, filled from `spikes.default_time_box` (default 4 hours) or the
spike's override, and the screen says: "Work done in chat or by hand can't be
measured in tokens, so this spike is held to a time box instead. When the
time box runs out, the spike ends with whatever findings have been saved."
Starting is still a web-UI act. Starting for chat or a person adds the
worktree and moves the spike to `running`, with no dispatch, and waits for a
claim.

**S2-3 — Claiming.** The chat AI claims a started spike with M13's claim
mechanism (`claim_task`'s spike form, or `claim_spike` if SPEC-020 keeps
claims per entity). A person claims it in the web UI with **I'll run this
spike**. The claim returns the worktree's path, the question, the surfaced
decisions, the owner's documents and, for a spike that asks again, the
earlier findings: what a dispatched spike's prompt holds (FR-4.1). Only a
spike started for that executor can be claimed by it.

**S2-4 — Working.** The claimant edits the findings document in the main
checkout directly, or with `save_spike_findings` over MCP. The worktree is
theirs for scaffolding and is discarded at the end, as in stage 1.

**S2-5 — Submitting.** `submit_spike` over MCP, or **I've finished** in the
web UI, ends the run as `answered` (FR-6), after validating the findings.

**S2-6 — The time box.** The time box runs from the claim, not from
activity. When it expires, M13's claim expiry ends the run: `EndSpike` with
`ended_how = 'time_box'`, the findings so far completed as in SD-8 ("The
spike reached the end of its time box of 4 hours."), and the worktree
discarded. A spike started for chat or a person and never claimed ends the
same way when its time box, counted from the start, runs out. M13's
inactivity checkpoint applies to the claim as well, as a prompt to the
person, not as the stop.

**S2-7 — Unmeasured.** A chat or person spike's tokens are recorded as
*unmeasured*, using M13's marker, and its page shows "Tokens: unmeasured
(run in chat)" in place of the bar. Calibration and the start screen's
forecast leave it out.

**S2-8 — Judging stays closed.** Closing a spike is a person's act in the web
UI, whoever ran it (SD-10). The chat AI that ran a spike can't approve its
findings.

**S2-9 — Integration checks** (from the orchestration note): a spike run in
chat or by a person is held by M13's claim expiry; its executor is recorded;
a chat-run spike's tokens are marked unmeasured; and neither milestone's
tools escape the MCP tool-set test.

**Stage 2 acceptance:** a chat spike claimed over MCP, submitted, and closed
by a person; a person spike claimed in the UI; a time box that expires ends
the spike with its findings and discards its worktree; the executor and the
unmeasured marker on the page and in `get_spike`; and the tool-set test with
M13's and M14's tools together.

## 5. Non-functional requirements

- **NFR-1 — One service layer.** The UI handlers, the MCP tools and the rules
  call the same service methods (`CreateSpike`, `StartSpike`, `EndSpike`,
  `CloseSpike`), each in one transaction with its audit rows.
- **NFR-2 — The orchestrator stays code.** Nothing an agent or the chat AI
  does starts a spike. Creating a spike dispatches nothing.
- **NFR-3 — The seam holds.** Only the web UI starts or closes a spike, and
  the MCP tool-set test proves the absence of every tool that could.
- **NFR-4 — No merge path.** SD-2's four facts are each tested
  (`TestSpikeHasNoMergePath`): the worktree is detached and no branch is
  created for it; no `worktrees` row is written; `mergeFeature` given a
  spike's ID finds no feature and refuses; the rules emit no `MergeFeature`
  for a `run-spike` success; no route matching `merge`, `promote` or `land`
  exists for spikes; a commit the agent makes in the worktree (through
  `run_command`) isn't reachable from any branch after the end; the main
  checkout's history after the run holds only the findings commit; and
  `start_building` and `Start building` refuse a spike's ID.
- **NFR-5 — Contained templates.** New markup lives in `spike.html` (the
  page, the owner section, the dialog, the start screen). `entity.html` gains
  one include, `inbox.html` one line, and the timeline's rules a spike
  section.
- **NFR-6 — Coordination.** Migration `0015` only. The shared files are kept
  to `mcp.go`'s registry line and initialise sentence, the MCP tool-set test's
  two lists, and `entity.html`'s include. Outside `internal/server` and
  `internal/store`, the changes are small and named: the budget in
  `internal/dispatch`, the spike's context in `internal/toolhost`, the tool
  and the config section in `internal/config`, the registry in
  `internal/ident`, the template and skill in the starter pack, and the
  spike's moments in `internal/timeline`.
- **NFR-7 — Human prose** in every label, refusal, notice and tool
  description (D-6). British spelling.
- **NFR-8 — No typed paths.** Forms carry row ids in hidden fields; the only
  typed values are the question and the budget.
- **NFR-9 — Tested as before.** Integration tests with the mock provider
  against real Postgres cover every FR. `go vet ./...` and
  `go test -race -count=1 ./...` are clean, and the integration tests run
  rather than skip.

## 6. Definition of done

1. Every stage-1 FR's acceptance passes in the suite.
2. `go vet ./...`, `gofmt -l` (no new files) and `go test -race -count=1 ./...`
   are clean, with the integration tests seen to run.
3. **A browser walkthrough without an AI provider**, with Playwright and the
   pre-installed Chromium: create a spike, see its start screen with the
   budget, start it, see its findings and its tokens against the budget once
   it has ended, and close it. The run itself is driven by a scripted
   provider in the demo server or by the mock-provider tests, and the
   walkthrough, `docs/walkthrough-spec-021.md`, says which.
4. REVIEW-021 is recorded and its findings dealt with (§8).
5. At least one round of both code reviews, with no major findings open.
6. Verification: every acceptance criterion with cited evidence.
7. A handoff, `docs/notes/handoff-M14-2026-10-02.md`, with stage 2 listed as
   remaining work if M13 hasn't merged.
8. The roadmap's §11 marks M14 stage 1 done with a pointer to the handoff.
9. **Choices that need Sam's explicit yes:**
   1. a spike is its own table, not a feature row (SD-1);
   2. no merge path, by a detached worktree and the absence of any merge
      code (SD-2);
   3. a spike always has an initiative, and optionally a feature (SD-3);
   4. five states, closing is final, and the question is fixed once created
      (SD-4);
   5. the budget counts every token, cache reads included, and is fixed at
      the start (SD-6; the alternative is input and output only);
   6. the stop never begins a turn it can see would cross the budget, may
      overrun by one turn's unforeseeable part, and the turn limit ends a run
      the same way (SD-7);
   7. the agent saves findings as it goes, and the server completes them at
      a stop, with no write-up turn (SD-8);
   8. the findings are approved by a person, with no agent reviewer (SD-9);
   9. "The question is answered" approves the findings in the same act, and
      closing is in the web UI only, with no chat relay (SD-10);
   10. a spike can't be a milestone deliverable (SD-11);
   11. asking again closes the first spike and creates a second, which still
       needs a person to start it (SD-13);
   12. open spikes block archiving their initiative (SD-14);
   13. the run has its own role, `spike-runner`, on Sonnet, without
       `report_bug` (SD-16);
   14. the project default is 200,000 tokens (FR-10.1);
   15. stage 2's time box runs from the claim, defaults to four hours, and
       ends the spike when it expires (§4, S2-6).

## 7. Open questions carried forward

- **Stopping a running spike by hand.** The budget is the only stop in stage
  1. A **Stop now** button needs the dispatcher to cancel a running attempt,
  which it can't do cleanly today.
- **Withdrawing a start** before the run is admitted, as Withdraw does for a
  send.
- **A chat relay for closing a spike**, with the person's words (SD-10's
  alternative).
- **Project-level spikes**, above every initiative.
- **Whether 200,000 tokens is the right default.** With cache reads counted,
  a 40-turn run on a medium repository can pass that. The live runs will say.
- **A spike that edits documents.** The agent can only write its findings. A
  spike that wants to propose a design change says so in What to do next.
- **Existing projects need new files** (FR-10.4); a `subutai upgrade` would
  help every milestone.

## 8. Changes after review

*Filled in after REVIEW-021.*
