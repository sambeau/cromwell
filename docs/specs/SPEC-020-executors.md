# SPEC-020: Executors

**Status:** **Draft — for Sam's approval.** Authored by Claude, the M13 lead.
The author can't be the approval gate, so the decision is Sam's. An
independent review is recorded in
[REVIEW-020](../reviews/REVIEW-020-executors.md). It found eleven material and
ten minor problems in the first draft. All are dealt with in this revision,
and §8 says how, finding by finding. Eighteen choices need Sam's explicit
yes: the scope decisions in §3, listed in DoD 8. The build doesn't wait for
approval.
**Date:** 2026-10-02
**Roadmap milestone:** M13 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11: "chat and humans can do work through the front door". *Done when:* "a
task done in chat gets the same review and verification as an agent's."
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28): **§5b** (executors), §5c (what the chat agent may do), §8
(unmeasured tokens and attribution), §10 (spikes, for generality only) and
§17b (the two questions it leaves to M13: what counts as activity on a
claim, with whether expiry differs for people and the chat agent; and whether
the chat agent may claim estimation).
**Authority:**
- [DEC-007](../decisions/DEC-007-the-judgement-boundary.md), all ten decisions
  except 9 (a colleague's pull-request review, which is M15b);
- [DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1:
  the chat agent never holds a verdict, never starts building, and never
  answers a checkpoint;
- [DEC-005](../decisions/DEC-005-the-orchestration-boundary.md), as DEC-007
  supersedes its first prohibition: state is held by code, and the boundary is
  enforced by leaving tools out;
- [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md): its "may not …
  drive any development-side lifecycle transition" is read, after DEC-007
  decision 3, as not covering claiming and submitting. Claim and submit move a
  task only along the path a dispatched implementer's start and completion
  already move it, and every judgement on that path stays dispatched (DoD 9).

**Builds on:**
- [SPEC-011](SPEC-011-send-to-development.md): Start building stays a person's
  act;
- [SPEC-012](SPEC-012-see-the-work.md): the timeline and the run pages;
- [SPEC-017](SPEC-017-chat-as-a-proper-seat.md): `document_writers`, its
  vocabulary (`agent`, `chat`, `person`), and the one function that says who
  did something (`whoWords`). SD-1 says how the executor record relates to it;
- [SPEC-018](SPEC-018-decisions.md): the surfaced block an implementer is
  given;
- [SPEC-019](SPEC-019-bugs.md): a bug is a feature row, so a bug's tasks are
  claimed like any other.

**Coordination:** M14 (spikes) is built in parallel.
- This spec owns migration `0014`. M14 owns `0015`. **M13 merges first.**
- M14's stage 2 claims spikes through this spec's claims. The claim record,
  the execution record and the claim service are general over what is claimed
  (SD-2, FR-2.10). This spec writes no spike code. M14 adds `spike` to the
  `ref_type` enum in `0015`.
- Files M14 is likely to touch too (NFR-3 lists every file this spec
  changes): `mcp.go`'s tool list, the tool-set test, `entity.html` (two
  includes; the markup lives in a new `executors.html`), `config.go` and the
  starter config, `server.go`'s heartbeat, `rules.go`'s checkpoint answers,
  `ui_views.go`'s `responseFor`, and `dispatch.go`'s governor.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6). Tool descriptions and tool results are also
what the chat agent reads, so they carry the rules (DEC-007 decision 3).

These words are kept apart throughout:

- **Executor**: whoever does a piece of work. One of a dispatched agent (with
  its role and model), the chat agent, or a person (DEC-007 decision 1).
  SPEC-017 calls the executor of a document its *writer*.
- **Execution**: one executor's part in one round of implementing a task. A
  task sent back by its code review has another round, which may have a
  different executor.
- **Claim**: a person's or the chat agent's hold on a piece of work, from
  *claim* to *submit*, *release* or the work ending. A dispatched agent never
  has a claim; its dispatch row is its hold. *Note:* the task machine already
  has an event called `claim` (ready → active), which `dispatchReadyTasks`
  fires for every dispatched implementer. It means "work on this task has
  started", whoever does it. This spec keeps the name, and calls the new
  record's events `claim.*`.
- **Submit**: handing claimed work back. For a task, it commits the working
  copy and queues the independent code review.
- **Judge**: give a verdict: a code review, verification, a document review.
  Executors never judge (DEC-007 decision 2).

---

## 0. Framing

Today a task is only ever implemented by a dispatched implementer. Start
building, and every later approval, moves each ready task to `active` and
queues an implement dispatch in the same transaction (`dispatchReadyTasks`).
The implementer works in the feature's one worktree and calls
`submit_implementation`. The server commits its work, moves the task to
`review`, and queues a dispatched code review. After the last task is
approved, the dispatched verifier checks the feature against its spec, and
Subutai merges it.

DEC-007 opens the doing to two more executors, the chat agent and a person,
and keeps every judgement where it is. This spec builds that:

- a record of who executed each round of each task;
- a claim, which takes a task from the dispatcher and gives its executor the
  working copy and the contract;
- a submit, which is the claimed task's only way forward, and leads into the
  same code review and verification an agent's work gets;
- a safety net for claims nobody is working on, and for commits nobody
  claimed;
- an honest ledger: chat and human work is *unmeasured*, not free;
- a stronger reviewer, by default, for what the chat agent did.

## 1. Goal

**One claim, which the definition of done checks directly** (the roadmap's
done-when):

> A task claimed over MCP by the chat agent is worked in the feature's working
> copy and submitted. Subutai commits it, and a dispatched code reviewer
> reviews it, with the project's reviewer for the chat agent's work. A
> send-back returns it to the chat agent, not to an implementer. Once it is
> approved, the feature's other tasks go on as before, and the dispatched
> verifier verifies the whole feature, which then merges. The task's page,
> the timeline and `get_feature` say the chat agent implemented it, and its
> tokens and its feature's are *unmeasured*.

Three supporting claims:

> A person can do the same from the task's page in the web UI.

> A claim with no activity for the project's expiry time asks a person "Is
> someone still working on this?" A commit on a feature's branch that Subutai
> didn't make, made while nobody held a claim, asks a person to look at it.

> The chat agent still holds no verdict. No tool lets it approve, review,
> verify, release a claim, start building or answer a checkpoint.
> `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` names `claim_task` and
> `submit_task` on purpose.

## 2. The audit: what exists, and what is missing

Checked against DEC-007, DESIGN-010 §5b and the code on `main` at `0055f6b`.

| Asked for | What exists | What is missing |
|---|---|---|
| The executor of each task (DEC-007 d1) | Implement dispatches carry a role and model. The task page lists its runs. Nothing names a task's executor, and nothing could name anyone but an agent. | An execution record (FR-1). |
| Claim and submit (d3) | The task event `claim` (ready → active) exists, but only `dispatchReadyTasks` fires it. `submit_implementation` is the dispatched implementer's outcome tool. | Claims, the tools, the UI, and the rules for a claim's rework (FR-2 to FR-4). |
| Tasks to claim | `dispatchReadyTasks` moves ready tasks to `active` with a queued implement dispatch in one transaction. A task is almost never `ready` for long. Failed dispatches are retried on the same row; an exhausted one raises a `dispatch-failure` checkpoint; a cancelled one leaves its task `active` for ever. | A claim must be able to take an `active` task that no agent is running (SD-3). |
| One worktree per feature | The governor runs one implement dispatch per feature at a time, checked on the pool before a separate transaction starts it. `dispatchReadyTasks` takes no lock, and `TransitionTask` has no state guard. The server commits with `git add -A`. | A claim edits the same worktree, so claims and dispatches must be serialised properly (SD-4, FR-2.7). |
| Feature abandonment | It transitions only the feature, at three sites (the feature page, the API, a bug's triage). Tasks are left as they were. | Claims must end with the feature (SD-16). |
| Judging stays closed (d2, d4, d5) | Code review and verification are dispatched. No MCP tool gives a verdict. | Tests that a claimed task gets both, and the must-not-exist names (FR-6). |
| Claims expire (d6) | The heartbeat fails stalled dispatches. Nothing watches a person or the chat agent. | A claim sweep and a checkpoint (FR-5.1 to FR-5.5). |
| Unclaimed commits (d7) | The post-commit hook watches documents on the main checkout. Nothing watches feature branches. Linked worktrees share the main repository's hooks. | A branch watch (FR-5.6 to FR-5.10). |
| Unmeasured tokens (d8) | Actuals are summed from `dispatches`. A chat-implemented task shows only its review's tokens, which reads as cheap, not unknown. A feature whose spec was written in chat shows no authoring cost. | The marker, and calibration leaving it out (FR-7). |
| A stronger reviewer for chat work (d10) | `routing` overrides a purpose's model for everyone. | A setting keyed on the executor, with a default (FR-8). |
| A chat-side skill (d3) | `chat-skills/review-design` (M3). | `chat-skills/work-a-task` (FR-9). |

Three things found while auditing that this spec builds on:

- **A dispatched implementer isn't shown its reviewer's comments on rework.**
  `returnTaskCode` audits them as `task.review_comments`, and `planImplement`
  doesn't read them. A claimant must see them, and an implementer taking over
  a released claim's rework must too, so this spec fixes it for both
  (FR-2.12).
- **A batch's base commit is stale.** Every task made ready together gets the
  same base commit, and they run one at a time, so the second task's review
  diff includes the first task's work. A claim resets its own base (FR-2.3).
  The agents' side is a bug report (§7).
- **Code review is never a person's verdict by default.** People only answer
  its escalations and deadlocks. That stays so.

## 3. Scope

### In scope

1. The execution record, on the task page, the feature page's task list, the
   timeline and MCP results (FR-1).
2. Claims: the record, the service, the state rules and the races (FR-2).
3. `claim_task` and `submit_task` over MCP, and `get_feature` listing tasks
   (FR-3).
4. Claim, submit and release in the web UI, for a person (FR-4).
5. Claim expiry and the branch watch (FR-5).
6. The boundary: what the chat agent still can't do, and the tests (FR-6).
7. Unmeasured actuals, left out of calibration and forecasting (FR-7).
8. The reviewer for the chat agent's work (FR-8).
9. The `work-a-task` chat skill and the `initialize` instructions (FR-9).
10. The end-to-end proof (FR-10).

### Out of scope (with destination)

| Item | Destination |
|---|---|
| A colleague's pull-request review counting as the code review (DEC-007 d9) | M15b |
| Spikes, their claims and their time box | M14 (it uses SD-2 and FR-2.10) |
| Per-user identity: which person claimed, and which chat session | M15. A person is the configured `ui_actor`; every chat session is the one MCP actor (SD-5). |
| Claiming spec writing, plan writing or estimating | Not built (SD-12) |
| A separate worktree per claim, so claims and agents work in parallel | Follow-up (§6) |
| A stronger *verifier* for a feature done mostly in chat | Follow-up (§6, SD-14's alternative) |
| Holding a merge while an unclaimed commit waits for a person | Follow-up (§6) |
| Uncommitted edits made outside a claim | Follow-up (§6). Only commits are watched (SD-10). |
| A batch's stale base commit, for agents | Bug report (§7) |

### Scope decisions

Each is a choice Sam confirms (DoD 8). The recommendation comes first.

- **SD-1 — The executor record is a table of executions.** `executions`
  (FR-1.1) has the same vocabulary as `document_writers` (`agent`, `chat`,
  `person`; actor, model, run, `via`), and the same sentence builder. SPEC-017
  said M13 would build on `document_writers` for tasks "rather than adding a
  second record". It builds on its vocabulary, its words and its code, but
  needs its own table: a document's writer is attached to a document row, a
  task isn't a document, and a task's record must carry the claim and the
  unmeasured marker. It is keyed on `ref_type` and `ref_id`, so a spike's
  executions can use it (SD-2). A task's executor, as shown, is its latest
  round's, and the line names an earlier executor when it differs (FR-1.4).
  *Alternatives:* columns on `tasks` (loses rounds with different executors);
  deriving the executor from dispatches and claims at read time (two sources
  for every reader, and no place for the marker).

- **SD-2 — Claims are general.** `work_claims` and `executions` key on
  `ref_type` and `ref_id`. The claim service calls a small Go interface per
  claimable kind (FR-2.10), which covers what differs: the working copy,
  refusals, what submit does in git and in the database, what release means,
  and the stale question. The claim record carries an optional `deadline_at`
  for a time box (DESIGN-010 §10), which M13 never sets, and a `feature_id`
  that is null when the claim doesn't hold a feature's working copy. M13
  registers `task`; M14 registers `spike`.

- **SD-3 — What can be claimed.** A task can be claimed when:
  - its feature is `active` (a person has pressed Start building);
  - its spec isn't being revised: `spec_stale` is false and no
    `revision-in-flight` checkpoint is pending;
  - its working copy exists on disk;
  - it has no claim that hasn't ended;
  - no implement dispatch is running on any task of the feature, and none of
    the task's implement dispatches has heartbeated within the stall window
    (an attempt the stall sweep gave up on may still be running tools);
  - no other claim on the feature is `open` (SD-4);
  - and it is either `ready`, or `active` with no running implement dispatch.

  A claim on an `active` task cancels every implement dispatch of the task
  that is `queued` or `failed`, and withdraws the task's pending
  `dispatch-failure` checkpoint, in its transaction. So a task whose agent
  failed three times, or whose failed dispatch a person cancelled, can be
  taken in chat. A `pending` task, a task in `review`, and a done or abandoned
  task are refused, each with a sentence (FR-2.4). *Why:* `dispatchReadyTasks`
  moves ready tasks to `active` as it queues them, so without the `active`
  case almost nothing could be claimed.

- **SD-4 — One hand in a feature's working copy at a time.** The feature has
  one worktree, and Subutai commits it with `git add -A`. So:
  - a claim is refused while an implement dispatch is running on any task of
    the feature, or while another claim on the feature is `open`;
  - while a claim is `open`, the governor holds every implement dispatch of
    that feature in the queue, with the reason "A person or the chat agent is
    working in this feature's working copy.";
  - a claim that is `submitted` (in review) or `returned` (sent back, not yet
    resumed) doesn't hold the copy, so agents may work on the feature's other
    tasks meanwhile.

  *The cost:* an open claim pauses the feature's dispatched implementation
  until it is submitted or released. That is why expiry matters (FR-5), and
  why the default is a day. And a claim sent back while an agent is running
  waits for the agent to finish before it can resume (SD-6). *Alternative:* a
  worktree per claim, merged on submit (§6).

- **SD-5 — Submitting is the only way forward, and a claim's own surface
  submits it.** A claim made over MCP is submitted over MCP, and one made in
  the web UI is submitted there. The chat agent has no way to give a claim
  back: if it can't finish, it says so to the person, who releases the claim
  in the web UI (FR-4.3). *Why:* releasing hands the work to a dispatched
  implementer, which spends tokens, and "submit is the only way forward" is
  the rule the chat agent is given. *Known cost:* until M15, every chat
  session is the one MCP actor, so any chat window can renew or submit any
  chat claim. *Alternative:* a `release_task` tool for the chat agent.

- **SD-6 — A send-back returns to the claim.** When the code reviewer asks
  for changes on a claimed task, the task goes back to `active`, the claim
  becomes `returned`, and no implementer is dispatched. A `returned` claim
  holds the task but not the working copy. Its executor resumes it by
  claiming the task again (`claim_task`, or the page's **Resume**), which
  makes it `open` if the copy is free, and otherwise says what is in the way.
  The comments are in the claim's result, on the task page and in
  `get_feature`. The review round cap still applies: at the cap, a person
  decides, as now.

- **SD-7 — Releasing keeps the work.** A person may release any claim that is
  `open` or `returned`, the chat agent's included, from the task page or by
  answering "Is someone still working on this?". Anything uncommitted in the
  working copy is committed as "subutai: T03 — work left by the chat agent,
  released". The task stays `active`, and an implement dispatch is queued.
  The implementer is given the latest code review's comments (FR-2.12), and
  its code review sees the released work in the task's diff. *Known cost:* the
  chat agent isn't told at once; its next `claim_task`, `submit_task` or
  `get_feature` says the claim was released, and by whom. *Alternative:*
  discard uncommitted work on release.

- **SD-8 — What counts as activity on a claim** (DESIGN-010 §17b):
  1. claiming it, and claiming it again (renewing an `open` claim, or
     resuming a `returned` one);
  2. any change to the feature's working copy while the claim is `open`: a
     new commit on its branch, or a change to its uncommitted files, as the
     claim sweep sees them;
  3. submitting it;
  4. a send-back from code review, which makes it `returned`;
  5. a person answering "Is someone still working on this?" with **Keep the
     claim**.

  Reading isn't activity: `get_feature`, `get_timeline` and viewing the page
  don't keep a claim alive. A chat session that polls would otherwise keep a
  dead claim alive for ever. That is why the review's comments and the
  claim's state are in `get_feature`, a read (FR-3.6), and claiming again is
  kept for a deliberate "I'm still on it".

- **SD-9 — One expiry, for people and the chat agent alike**
  (DESIGN-010 §17b). `claims.expiry_hours`, default 24 (DEC-007 decision 6).
  It applies to `open` and `returned` claims: both hold the task. Either way
  the question goes to a person, and a stale claim costs the same: the task,
  and while `open` the feature's agents, wait. Time while the server is down
  counts; the sweep looks at the working copy before it judges expiry, so
  work done while the server was down is seen first. *Alternative:* separate
  settings for chat and people.

- **SD-10 — "An active feature's paths" means its branch.** DEC-007 decision
  7 asks for a checkpoint "when a commit touches an active feature's paths and
  no task is claimed". Features don't own file paths; a feature's code lives on
  its branch, `subutai/<feature path>`. So the watch covers the branches of
  features being built or verified (`active` and `review`):
  - **Subutai's own commits** are known by their hashes, recorded as Subutai
    makes them (FR-5.7), not by their author, which anyone can set.
  - **Commits already on `main`** (a person merging or rebasing onto it) are
    left out.
  - **A rewritten branch** (a reset or rebase that drops commits Subutai saw)
    raises the checkpoint, worded for that.
  - **A commit made while a claim on the feature is `open`** is that claim's
    work. Subutai can't tell who made it until M15, so a commit by someone
    else while a claim is open isn't flagged (a known false negative).
  - **Commits on `main`** aren't watched: they aren't any feature's work.
  - **Uncommitted edits** aren't watched (§6).
  - **A person's submitted work is committed by Subutai**, with trailers that
    name the executor and the claim. The person's own commits on the branch
    keep their author.

- **SD-11 — The unclaimed-commit checkpoint is a notice with one answer.** Its
  question names the commits and their authors and says the risk plainly: the
  work may have missed its code review, and the verifier checks the feature
  against its spec, not each line. Its one answer is **I've seen this**.
  Reverting or adopting the commits stays a person's job in git, and the
  merge isn't held (§6). *Alternatives:* **Revert them** as a second answer;
  turning the commits into a task that is reviewed; holding the merge.

- **SD-12 — The chat agent doesn't claim estimation, spec writing or plan
  writing** (DESIGN-010 §17b). `claim_task` claims tasks only.
  - **Estimation:** an estimate is a forecast of measured work, made against
    the calibration corpus of measured actuals. The chat agent's estimate
    can't be calibrated, and the estimator run it would replace costs a few
    thousand tokens. A person can already enter an estimate in the web UI. So
    it isn't useful.
  - **Spec and plan writing:** SPEC-017 already gives the chat agent a lane:
    write it, `attach_document`, `submit_for_review`. That lane's work is
    unmeasured too (SD-13).

- **SD-13 — What is unmeasured.** Tokens don't describe a person's work
  either, so executions by `chat` and `person` are `measured = false`. For
  calibration and forecasting:
  - a task is unmeasured when any of its executions is;
  - a feature is unmeasured when any of its tasks is, or when the latest
    writing act on its current spec or plan (`document_writers`) is `chat` or
    `person`, since its actuals would then lack the authoring run;
  - an initiative is unmeasured when any feature or task in it is.

  The measured part (code reviews, agent rounds, verification) is still shown,
  as "at least" a number (FR-7.3).

- **SD-14 — The chat agent's work is reviewed by a stronger model by
  default** (DEC-007 decision 10, DESIGN-010 §5b). `claims.chat_review_model`
  (FR-8.1):
  - **absent** (the default): the configured model with the highest output
    price, from `models`. Price is a judgement Subutai can make without
    knowing which model is stronger, and a project that configures one model
    gets that model;
  - **a model name**: that model;
  - **`same`**: the reviewer's usual model, as if no chat work were involved.

  It applies to the code review of a task whose latest round is the chat
  agent's, and to the review of a spec, plan or bug report whose latest
  writing act is the chat agent's (SPEC-017 §6 named M13 for this).
  *Alternatives:* empty by default, which keeps the usual model and so departs
  from decision 10's "default to"; code review only; also verifying a feature
  done mostly in chat with the stronger model (§6).

- **SD-15 — Claiming takes no quote.** Claiming and submitting are doing, not
  relaying (DESIGN-010 §5c: "It may claim and submit work"). A claim's worst
  case is pausing a feature's agents until a person releases it, which is
  small and recoverable (DEC-006 Amendment 1's test). It is audited `via: mcp`.
  Cancelling a queued dispatch to take the task is the claim's own act, not a
  chat agent directing a dispatch (DEC-005); a note says so (DoD 9).

- **SD-16 — A claim ends when its task or feature ends, by any route.**
  - A task's approval ends its claim (`done`).
  - A task's abandonment (the review deadlock's *Abandon*, the only route
    today) ends it (`abandoned`).
  - A feature's abandonment, at any of its three sites, ends every claim on
    the feature (`abandoned`): it is done in `store.TransitionFeature` on the
    `abandon` event, so all three are covered by one change.

  Each is in the same transaction as the transition, and withdraws the
  claim's pending `claim-stale` checkpoint.

- **SD-17 — The implementer is shown its reviewer's comments on rework.**
  FR-2.12 fixes the gap the audit found, for agents as well as claims, because
  a released claim in a later round would otherwise hand an agent blind
  rework. *Alternative:* comments for claims only, and the agents' side as a
  bug report.

- **SD-18 — The chat agent is told not to commit.** The skill and the tools
  say: edit the working copy, and Subutai commits when you submit. A
  work-in-progress commit on the branch would show up in another task's review
  diff, if that review ran meanwhile. A commit isn't refused; it is still the
  claim's work.

## 4. Requirements

### FR-1: The execution record

**FR-1.1 — Migration `0014`** adds `executions`, one row per executor's part
in a round.

| Column | Meaning |
|---|---|
| `id` | row id |
| `ref_type`, `ref_id` | what was executed: `task` in M13 |
| `round` | 1 for the first round, then one more for each send-back |
| `kind` | `agent`, `chat` or `person` |
| `actor` | the role, the MCP actor, or the person (`ui_actor`) |
| `model` | the model, for an agent |
| `dispatch_id` | the implement run, for an agent |
| `claim_id` | the claim, for `chat` and `person` |
| `via` | `agent`, `mcp` or `ui` |
| `measured` | true for `agent`, false otherwise (FR-7) |
| `start_head` | the branch head when the execution began |
| `inferred` | true for a row backfilled by the migration |
| `started_at`, `submitted_at` | when it began, and when it was handed in |

The database checks:
- an `agent` row has a `dispatch_id` and `measured = true`;
- a `chat` or `person` row has a `claim_id` and `measured = false`;
- `dispatch_id` is unique among agent rows, and `(claim_id, round)` among
  claim rows: each run is one row, and a claim has one row per round, since a
  `returned` claim resumed in the next round is a new row. A round may hold
  any number of rows: a claim released and the task claimed again is two.

When a task is deleted (re-decomposition deletes only `pending` and `ready`
tasks, which have no executions), its rows are deleted with it, by the
service, since `ref_id` has no foreign key.

**FR-1.2 — Where executions are recorded.** Each in the same transaction as
the act it describes.

| Act | Recorded |
|---|---|
| An implement dispatch starts running (FR-2.7's start) | a new `agent` row for the task's current round, with role, model, run and the branch head |
| A retried attempt of the same dispatch | nothing new: the dispatch already has its row |
| A claim (FR-2.3), and a `returned` claim resumed | a new `chat` or `person` row for the current round |
| A renewal of an `open` claim | nothing new |
| Submit (FR-2.5), or the implementer's completion | `submitted_at` on the row of the claim or run being submitted |
| A release (SD-7) | the row stays without `submitted_at`; the agent's round is a new row when its dispatch starts |

**The current round** is one more than the number of `request_changes`
transitions the task has had, counted inside the transaction that records the
row, after any transition it makes. It is a different number from the review
round cap's, which counts `review-code` dispatches (`actions.go`); they
measure different things, and neither should be "fixed" to match the other.

**FR-1.3 — Backfill, in `0014`.** One `agent` row, `inferred = true`, for
every implement-task dispatch with at least one `dispatch.running` audit row,
started at the first such row. Its round is one more than the task's
`request_changes` transitions before that time. No claims existed before
`0014`, so there is nothing else to infer.

**FR-1.4 — How an executor is said.** One function, `executorLine`, built on
SPEC-017's `whoWords`, used by the task page, the feature page, MCP and the
timeline.
- An agent is "the implementer (*model*)", linking to its run. The chat agent
  is "the chat agent". A person is their name.
- The line reads across rounds, as the writer line does. It names the latest
  round's executor, an earlier round's when it differs, and a released claim
  whose work an agent finished:
  - "Implemented by the chat agent."
  - "Being implemented by sam, who claimed it 3 hours ago."
  - "Implemented by the implementer (claude-sonnet-5), from work the chat
    agent started."
  - "Implemented by the implementer (claude-sonnet-5), then reworked by the
    chat agent."
  - "Nobody has started this task yet."
- An inferred row adds nothing: before `0014`, every executor was an agent,
  and the dispatch proves it.

**FR-1.5 — On the task page** (`/ui/t/{id}`). An include, `task-executor`,
directly under the page heading, in the new template file `executors.html`.
It shows the executor line. For an unmeasured task, it adds "This work was
done in chat or by a person, so its tokens aren't measured."

**FR-1.6 — On the feature page's task list**, and the plan page's, each task
shows an executor icon (agent, chat or person), with the executor line as its
accessible name and title text. A task with a claim that hasn't ended shows a
short label beside it, "Claimed by the chat agent" or "Claimed by sam".

**FR-1.7 — In the timeline.** New moments, from new audit kinds (FR-2.11):
- "Claimed by the chat agent: *T03 title*" (`claim.claimed`);
- "Submitted by the chat agent: *T03 title*" (`claim.submitted`);
- "Released by sam: *T03 title*" (`claim.released`);
- the existing "Code review sent back *…*" moment is unchanged, and a
  resumption isn't a moment.

Each carries `By` (chat or person), as SPEC-012's moments do. The checkpoint
moments read "Waiting for a person: is someone still working on a task" and
"Waiting for a person: commits nobody claimed".

**FR-1.8 — In MCP results.** `get_feature` gains `tasks` (FR-3.6). Each task
entry carries `executor`: `{ "sentence", "kind", "who", "model", "run_id",
"measured" }`, or `{ "sentence": "Nobody has started this task yet." }`.

### FR-2: Claims

**FR-2.1 — Migration `0014`** adds `work_claims`.

| Column | Meaning |
|---|---|
| `id` | row id |
| `ref_type`, `ref_id` | what is claimed: `task` in M13 |
| `feature_id` | the feature whose working copy the claim holds; null when it holds none (a spike's) |
| `kind` | `chat` or `person` |
| `actor` | the MCP actor, or the person |
| `via` | `mcp` or `ui` |
| `state` | `open`, `submitted`, `returned` or `ended` |
| `end_reason` | `done`, `released` or `abandoned` |
| `ended_by` | who ended it, for `released` and `abandoned` |
| `claimed_at` | when |
| `last_activity_at`, `last_activity` | when activity was last seen, and what it was (SD-8: `claimed`, `renewed`, `resumed`, `worktree`, `submitted`, `sent_back`, `kept`) |
| `worktree_seen` | the fingerprint the sweep last saw (FR-5.2) |
| `deadline_at` | an optional time box (SD-2); M13 never sets it |
| `submitted_at`, `ended_at` | when |

The database checks:
- one claim that isn't `ended` per `(ref_type, ref_id)`, by a partial unique
  index;
- one `open` claim per `feature_id`, by a partial unique index (SD-4);
- `end_reason` is set exactly when `state = 'ended'`.

The migration also adds `claim` to the `ref_type` enum, for the audit rows
that are about the claim itself (FR-2.11).

**FR-2.2 — The claim's states** are a small machine in `internal/lifecycle`
(`claim.go`), with the same contract as the task machine
(`IllegalTransitionError`):

| From | Event | To |
|---|---|---|
| (none) | `claim` | `open` |
| `open` | `renew` | `open` |
| `open` | `submit` | `submitted` |
| `submitted` | `send_back` | `returned` |
| `returned` | `resume` | `open` |
| `submitted` | `done` | `ended` |
| `open`, `returned` | `release` | `ended` |
| `open`, `submitted`, `returned` | `abandon` | `ended` |

The task machine gains one event, `release` (active → active), so the audit
trail says what happened. A claim's `submit` fires the task's existing
`implemented` (active → review), and a send-back fires its existing
`request_changes` (review → active). Anything out of order is refused by
whichever machine it breaks.

**FR-2.3 — Claiming** (`Server.ClaimWork`, called by MCP and the UI).
1. Outside any transaction, holding the working copy's lock (FR-2.8): read
   the branch head and compute the working copy's fingerprint (FR-5.2).
2. In one transaction, taking locks in the order of FR-2.7:
   1. Check SD-3 and SD-4 (for a resumption, only the working-copy checks). On
      failure, refuse with FR-2.4's sentence.
   2. For a `ready` task, fire the task's `claim` (ready → active). For an
      `active` task, cancel its `queued` and `failed` implement dispatches
      (`state = 'cancelled'`, only `WHERE state IN ('queued', 'failed')`), and
      withdraw its pending `dispatch-failure` checkpoint, with an audit row
      saying who took the task.
   3. In round 1, when the task has no earlier execution, set its base commit
      to the head read in step 1, if the branch still has that head (FR-2.5's
      check then compares with the work the claimant does, not a sibling's).
   4. Insert the claim (`open`, activity `claimed`, `worktree_seen` from step
      1), or move a `returned` claim to `open` (`resume`), and insert the
      execution row.
   5. Audit `claim.claimed` (or `claim.resumed`) on the task, with `via`,
      `kind`, the claim, and the dispatches cancelled.

A claim by the claim's own executor (same `kind`, `actor` and `via`) on a
task it holds `open` is a renewal: it changes the activity to `renewed`, and
returns the same result.

**FR-2.4 — Refusals.** Each is a full sentence that says what to do instead.
For example:
- "FEAT-023-T03 can't be claimed yet: it waits for T01 and T02 to be done.
  Claim one of those, or wait."
- "FEAT-023 isn't being built yet. A person starts building from the
  feature's page; you can't."
- "An agent is implementing FEAT-023-T02 in this feature's working copy.
  Claim a task when it has finished; get_feature shows when."
- "FEAT-023-T02 is claimed by sam, in this feature's working copy. Only one
  task in a feature can be worked at a time; wait for it to be submitted."
- "FEAT-023-T03 is claimed by sam. Only a person can release a claim, in the
  web UI."
- "FEAT-023-T03 is in code review. If the reviewer asks for changes, it comes
  back to whoever implemented it; get_feature shows the comments."
- "FEAT-023's spec is being revised. Claiming waits until a person decides
  whether building continues."
- "FEAT-023-T03 is already done. Choose another task from get_feature."
- "FEAT-023's working copy is missing. A person needs to retry creating it
  from the Inbox."

**FR-2.5 — Submitting** (`Server.SubmitWork`). It takes the claim's executor
and surface, the task, and a summary (required, at least one sentence).
1. Holding the working copy's lock (FR-2.8), check that the executor holds an
   `open` claim on the task from this surface (SD-5); otherwise refuse,
   saying whether the claim was released and by whom.
2. Refuse when nothing in the working copy differs from the execution's
   `start_head`, committed or not: "Nothing in the working copy has changed
   since you claimed FEAT-023-T03. Make the change, then submit; or ask a
   person to release the claim."
3. Commit the working copy as the implementer's completion does, with the
   message `subutai: T03 — title`, the summary, and two trailers:
   `Subutai-Executor: chat` (or `person`) and `Subutai-Claim: <id>`. Record
   the commit's hash as Subutai's (FR-5.7).
4. In one transaction, re-checking the claim is still `open`: the task's
   `implemented` (active → review), the claim's `submit`, `submitted_at` on
   the execution, the code review enqueued with its model (FR-8), and the
   audit row `claim.submitted` with the summary and the commit.
5. If step 4 finds the claim no longer open (a release won the lock), the
   commit stands as the released work (SD-7) and the submit is refused with
   the release's sentence. The lock makes this a narrow case, not a normal
   one.

A submit is allowed while the feature's spec is being revised, as an agent's
completion is, and its result says so: "The spec is being revised; the
reviewer reviews this against the approved spec."

The result names the review and its model, and says the executor can't
review, approve or verify it.

**FR-2.6 — The code review's verdict on a claimed task.** The rules engine is
unchanged. The server's executors of its actions read the task's claim:
- `approveTaskCode`: the claim's `done`, in the same transaction.
- `returnTaskCode`: for a task with a `submitted` claim, the claim's
  `send_back` (activity `sent_back`), the comments audited as now, and **no
  implement dispatch**. For an unclaimed task, unchanged.
- The round cap's `review-deadlock` and the escalation are unchanged. A
  person's *Approve* there ends the claim with `done`, and *Abandon* with
  `abandoned` (SD-16).

**FR-2.7 — Races, and one lock order.** Every path that changes a claim, a
task's state, or an implement dispatch's state takes row locks in this order:
**the feature, then the task, then the dispatch rows**.
- **Starting an implement dispatch** becomes one store function,
  `StartImplementDispatch`, used by the dispatcher in place of
  `MarkDispatchRunning` for `implement-task`. In one transaction it locks the
  feature (`SELECT … FOR UPDATE`), then re-checks, refusing to start when: an
  implement dispatch is running on the feature; the feature has an `open`
  claim; the task has a claim that hasn't ended; or the task isn't `active`.
  A refusal leaves the dispatch queued with the reason, or cancels it when the
  task is claimed. It then marks the dispatch running and records the
  execution row (FR-1.2). `admit`'s pool check stays as a cheap first filter.
- **`dispatchReadyTasks`** locks the feature, re-reads each task `FOR
  UPDATE`, and skips a task that isn't `ready` or has a claim.
- **`TransitionTask`** gains a state guard (`WHERE id = $1 AND state =
  $from`), and refuses when no row changed, so a lost race refuses rather than
  overwrites.
- **`RequeueDispatch`** (the retry sweep and a *Retry* answer) refuses an
  implement dispatch whose task has a claim that hasn't ended, and cancels it
  instead, with an audit row. A *Retry* answer on a dispatch already cancelled
  by a claim is a no-op with that audit row, not an error.
- **The claim service** takes the same locks in the same order (FR-2.3).

**FR-2.8 — One hand in git, too.** Git work in a feature's working copy is
serialised per working copy by an in-process lock (the server is one
process): claiming, submitting, releasing, the claim sweep's fingerprint, the
branch watch, and the implementer's completion each hold it across "git, then
the transaction". The fingerprint uses `git --no-optional-locks`, so it never
takes git's index lock from under the claimant. No git runs inside a database
transaction (NFR-6).

**FR-2.9 — Release** (`Server.ReleaseClaim`; UI only, SD-5) does SD-7, under
the working copy's lock: commit anything uncommitted with the "work left by"
message and the trailers (recorded as Subutai's, FR-5.7); then in one
transaction, the claim's `release`, the task's `release` event, an implement
dispatch queued (`enqueueImplementTx`), the claim's pending `claim-stale`
checkpoint withdrawn, and the audit row `claim.released` with who released
it. A submitted claim can't be released: "It is in code review. Wait for the
verdict."

**FR-2.10 — The claimable interface** (SD-2, for M14). The claim service
calls a `claimable` per `ref_type`:

```go
type claimable interface {
    // load returns the item, the feature whose working copy it holds (or
    // nil), and the working copy's path.
    load(ctx, q, refID) (*claimTarget, error)
    // refusal says why it can't be claimed or resumed now, or "".
    refusal(ctx, tx, t *claimTarget, resume bool) string
    // onClaim moves the item's own state, inside the claim's transaction.
    onClaim(ctx, tx, t, c *store.Claim) error
    // prepareSubmit does the git work, outside any transaction, holding the
    // working copy's lock; onSubmit records it, inside the transaction.
    prepareSubmit(ctx, t, c *store.Claim, summary string) (submitted, error)
    onSubmit(ctx, tx, t, c *store.Claim, s submitted) error
    // prepareRelease and onRelease do the same for a release.
    prepareRelease(ctx, t, c *store.Claim) (released, error)
    onRelease(ctx, tx, t, c *store.Claim, r released) error
    // staleQuestion is the "still working on this?" question and its
    // release answer's words; deadlineQuestion the same for a time box.
    staleQuestion(t, c *store.Claim, idle time.Duration) (question, releaseLabel string)
    deadlineQuestion(t, c *store.Claim) (question, releaseLabel string)
    // contract is what the claimant is given.
    contract(ctx, t, c *store.Claim) (map[string]any, error)
}
```

The record, the activity rules, the sweep, the stale and deadline checkpoints
(FR-5), and the execution record are the claim service's. The sweep raises a
`claim-deadline` checkpoint when `deadline_at` passes, with the interface's
question; M13 never sets a deadline, so only M14 exercises it. Audit rows
about the work go on the claimed item (`task`); rows about the claim alone go
on `claim`.

**FR-2.11 — Audit kinds:** `claim.claimed`, `claim.renewed` (not a timeline
moment), `claim.resumed` (not a moment), `claim.submitted`, `claim.sent_back`
(not a moment: the code review's own moment covers it), `claim.released`,
`claim.activity` (only when the sweep sees a change; not a moment), and
`claim.ended`.

**FR-2.12 — The implementer is shown its reviewer's comments** (SD-17).
`planImplement`, in a round after the first, adds a section "What the code
reviewer asked for" after the task, with the latest `task.review_comments`
audit row's comments (severity, file, line, text). One function reads them,
and the claim's result, the task page and `get_feature` use it too.

### FR-3: The MCP tools

**FR-3.1 — `claim_task`.** One argument, `task`: the task's ID
(`FEAT-023-T03`, or `BUG-007-T01` for a bug's task).

Its description says, in sentences:
- what it does: it takes a task of a feature that is being built, so that you
  and the person implement it in the feature's working copy instead of an
  agent;
- that the feature's other agents wait while you hold it, so submit promptly,
  or tell the person if you can't finish;
- that the only way forward is `submit_task`; you can't release a claim, and a
  person can, in the web UI;
- that your work gets an independent code review, and the feature an
  independent verification, and you can't review, approve or verify it;
- that a claim with no activity for the project's expiry time asks the person
  whether anyone is still working on it;
- to edit only inside the working copy it returns, and not to commit: Subutai
  commits when you submit;
- that calling it again on a task you hold renews the claim, or resumes it
  after a send-back.

**FR-3.2 — Its result**:
- `task`: the task entry (FR-3.6);
- `feature`: its ID, path and name;
- `working_copy`: `{ "path" (absolute), "branch", "base_commit" }`;
- `contract`:
  - `spec`: `{ "id", "path", "body" }`, the feature's spec type (a bug's
    report, per SPEC-019);
  - `dev_plan`: `{ "id", "path", "body" }`;
  - `task`: `{ "id", "title", "description" }`;
  - `decisions`: the surfaced block the implementer would be given
    (SPEC-018), as text, when there is one;
- `review_comments`: in a round after the first, the latest code review's
  comments (severity, file, line, text) and its run's ID, whoever
  implemented the earlier round (FR-2.12);
- `rules`: a list of sentences: the description's rules, plus the commands
  the project allows its implementers (`commands`), so that the chat agent
  knows how the project builds and tests;
- `expires`: "If nothing changes in the working copy for 24 hours, the person
  will be asked whether anyone is still working on this.";
- `next`: "Implement the task in the working copy, run the project's build and
  tests, then call submit_task with a summary."

A renewal or a resumption returns the same.

**FR-3.3 — `submit_task`.** Two arguments: `task` (ID) and `summary`
(required). Its description says it hands the work to an independent code
reviewer, that a send-back returns it to you with the reviewer's comments,
that it is the only way forward, and that you can't approve your own work.

**FR-3.4 — Its result**: the task entry (now `review`); `review`: `{ "run_id",
"model" }` of the queued code review; `commit`: the commit's hash; and
`next`: "A code reviewer will review this. get_feature shows its state and
any comments. If it comes back, call claim_task to resume it. You can't
review or approve it yourself."

**FR-3.5 — Refusals** are FR-2.4's sentences, as tool errors (`isError`).

**FR-3.6 — `get_feature` gains `tasks`**, in plan order. Each entry: `id`,
`title`, `state`, `depends_on` (IDs), `executor` (FR-1.8), `claim` (`{ "kind",
"who", "state", "since", "last_activity", "released_by" }` for the latest
claim, ended or not), `review_comments` (FR-2.12, when the task is in a round
after the first), `claimable` (a boolean), `why_not` (FR-2.4's sentence, when
it can't be claimed), and `url`. Its description gains a sentence about the
tasks. This is the only read change: the chat agent needs it to find a task,
and to follow one it submitted without renewing it (SD-8).

**FR-3.7 — The `initialize` instructions** gain two sentences: that the chat
agent may claim a task of a feature being built, work it with the person, and
submit it, and that its work is reviewed and verified by others; and that the
`work-a-task` skill says how.

### FR-4: The web UI, for a person

**FR-4.1 — The task page** gains a claim panel, `task-claim`, in
`executors.html`, below the description:
- **No claim, and claimable:** "You can implement this task yourself instead
  of an agent. Claiming it pauses this feature's agents until you submit it."
  and a **Claim this task** button.
- **Not claimable:** FR-2.4's sentence, and no button.
- **A person's open claim:** who and since, the last activity, the working
  copy's absolute path and branch, links to the spec, the plan and this task,
  the review comments in a later round, a **Submit for code review** form
  (summary, required), and **Release**.
- **A returned claim:** the review comments, and **Resume** (the claimant's)
  or **Release**.
- **The chat agent's open or returned claim:** who, since, the last activity
  and the working copy, and **Release**, with "The chat agent is working on
  this. Releasing it gives the task to an agent, and keeps what is in the
  working copy."
- **A submitted claim:** "Submitted by the chat agent. A code reviewer is
  reviewing it."

**FR-4.2 — Routes:** `POST /ui/task/claim` (claim, renew or resume),
`/ui/task/submit` and `/ui/task/release`, each with the task's ID,
redirecting to the task page with a notice, or with the refusal as an error
banner. They call the same service methods as MCP (NFR-1), with `kind:
person`, `via: ui` and actor `ui_actor`.

**FR-4.3 — Release** is FR-2.9, released by `ui_actor`.

**FR-4.4 — The feature page** shows the executor icons and claim labels
(FR-1.6). A feature with an open claim says, under its timeline strip:
"*T03 title* is claimed by the chat agent, so this feature's agents are
waiting."

### FR-5: The safety nets

**FR-5.1 — The claim sweep** runs on the heartbeat, after `StallSweep`. For
each claim that is `open` or `returned`:
1. For an `open` claim, compute the working copy's fingerprint (FR-5.2),
   holding its lock. If it differs from `worktree_seen`, record activity
   `worktree` and the new fingerprint, conditionally on the claim still being
   `open`.
2. If `now − last_activity_at ≥ claims.expiry_hours` and no `claim-stale`
   checkpoint is pending for the claimed item, raise one (FR-5.3).
3. If `deadline_at` has passed and no `claim-deadline` checkpoint is pending,
   raise one (FR-2.10).

**FR-5.2 — The fingerprint** is a hash of the branch head and the output of
`git --no-optional-locks status --porcelain=v1 -z --untracked-files=all`,
together with the size and modification time of each file that status lists.
It changes on a commit, an edit, a new file or a deletion. It is computed only
for open claims, so its cost is bounded by how many people and chat sessions
are working at once.

**FR-5.3 — The `claim-stale` checkpoint**, on the claimed task:
- Question (the task's `staleQuestion`): "FEAT-023-T03, *title*, was claimed
  by the chat agent 26 hours ago, and nothing has changed in its working copy
  for 24 hours. This feature's agents are waiting. Is someone still working
  on it?" (For a `returned` claim: "… was sent back by its code reviewer 26
  hours ago, and nobody has resumed it. …")
- Answers: **Keep the claim** (activity `kept`; the clock restarts) and
  **Release it to an agent** (FR-2.9, released by the person who answered).
- Context: the claim, its executor, the last activity and when, and the
  working copy. The Inbox shows the last activity, so a person can see if the
  claimant came back.

The chat agent can't answer it (DEC-006 Amendment 1; there is no tool).

**FR-5.4 — Activity withdraws the question.** Any activity (SD-8) on a claim
withdraws its pending `claim-stale` checkpoint (`state = 'withdrawn'`), with
an audit row: the claimant came back. Submitting, releasing and ending the
claim withdraw it too. An answer that arrives after its checkpoint was
withdrawn, or after its claim was submitted or ended, is a no-op with a
notice: "That claim has moved on since the question was asked; nothing was
changed."

**FR-5.5 — Configuration:** `claims.expiry_hours`, an integer pointer:
absent means 24; otherwise at least 1, and the loader refuses 0 or less.
Tests and the demo backdate `last_activity_at` in the database rather than
wait.

**FR-5.6 — The branch watch.** Migration `0014` adds `worktrees.watched_head`
and `worktree_commits` (`worktree_id`, `hash`, `act`: `implementation`,
`submit`, `release`; `at`), the commits Subutai made on a feature's branch.
The watch runs on every heartbeat, and asynchronously after every
post-commit call (so a commit in a worktree is seen at once; the post-commit
handler never waits for it). For each live worktree of a feature that is
`active` or `review`, holding the working copy's lock:
1. If `watched_head` is null, start from the fork point (`git merge-base main
   <branch>`).
2. If `watched_head` isn't an ancestor of the branch head (`git merge-base
   --is-ancestor`), the branch was rewritten: raise the checkpoint, worded for
   that (FR-5.8).
3. List `git rev-list --reverse <watched_head>..<branch> --not main`, with
   author and subject.
4. Drop the hashes in `worktree_commits`.
5. If any remain and the feature has no `open` claim, raise the checkpoint
   (FR-5.8). If it has one, they are that claim's activity (SD-10).
6. Set `watched_head` to the branch head.

**FR-5.7 — Recording Subutai's commits.** The implementer's completion,
submit and release each record their commit's hash in `worktree_commits` in
their transaction. They hold the working copy's lock across the commit and
the transaction, and run the watch first, so a person's commit made just
before Subutai's is judged, not swallowed.

**FR-5.8 — The `unclaimed-commit` checkpoint**, on the feature:
- Question: "Two commits were made on FEAT-023's branch while nobody had
  claimed a task: a1b2c3d by Sam Phillips, "quick fix"; e4f5a6b by Sam
  Phillips, "tidy". Work done outside a claim may have missed its code
  review, and the verifier checks the feature against its spec, not each
  line. Next time, claim the task first, or ask the chat agent to." For a
  rewritten branch: "FEAT-023's branch was rewritten: commits Subutai had
  already seen are no longer on it. …"
- One answer: **I've seen this** (SD-11).
- Context: the commits (hash, author, subject, time), or the old and new
  heads.

While one is pending, more unclaimed commits are added to its context and
question, rather than raising a second: checkpoints are one per kind and item
while pending.

**FR-5.9 — `watched_head` starts at the branch head** once `addWorktree` has
created the branch (in `StartFeature`, and in the worktree-failure retry). A
migration can't run git, so at boot Subutai sets it to the branch head for
every live worktree that has none, as SPEC-017's provenance backfill runs at
boot. History before M13 is never reported.

**FR-5.10 — The Inbox** shows `claim-stale`, `claim-deadline` and
`unclaimed-commit` with their answers. `responseFor` gains the three kinds,
and the rules engine routes their answers: `claim-stale` and
`claim-deadline` to keep or release; `unclaimed-commit` to nothing.

### FR-6: The boundary

**FR-6.1 — The advertised tool set gains exactly `claim_task` and
`submit_task`**, each with a comment in
`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` citing DEC-007 decision 3
and DESIGN-010 §5c. The test's header comment changes from "no tool that …
transitions development lifecycle" to say that the only development-side
moves are claiming and submitting a task, which DEC-007 decision 3 allows, and
that no tool judges.

**FR-6.2 — The must-not-exist list gains:** `release_task`,
`approve_task`, `review_task`, `review_code`, `verify_feature`,
`submit_verification`, `submit_implementation`, `claim_verification`,
`claim_review`, `claim_estimate` and `claim_feature`. `submit_implementation`
is the dispatched implementer's outcome tool, and must never be offered to
chat.

**FR-6.3 — Verification can never be claimed.** `claim_task` refuses
anything that isn't a task's ID, including a feature's or a bug's: "Only
tasks can be claimed. Verification is always done by Subutai's verifier."
There is no verification task to claim. A test asserts that a feature whose
every task was claimed and submitted still gets a dispatched `verify-feature`
run.

**FR-6.4 — Code review can never be skipped.** Submitting always queues a
dispatched `review-code`. A test asserts that a claimed task reaches `done`
only through the reviewer's approval, or a person's answer at the round cap
or an escalation.

**FR-6.5 — Out-of-order acts and races are refused**, each tested:
- submitting without a claim, someone else's claim, from the other surface,
  twice, or a task in review;
- claiming a pending task, a task in review, a task during a running
  implement dispatch on its feature, and a second task in a feature with an
  open claim;
- releasing a submitted claim;
- a send-back while an agent runs on another task: the claim is `returned`,
  and resuming it is refused until the agent finishes;
- a claim on a task whose dispatch is exhausted: the `dispatch-failure`
  checkpoint is withdrawn, and a later *Retry* answer, or the retry sweep,
  starts nothing;
- a claim racing `dispatchReadyTasks` and `StartImplementDispatch`: exactly
  one wins, and no implementer ever runs on a claimed task;
- a feature abandoned with an open claim: the claim ends, and the sweep raises
  nothing.

### FR-7: Unmeasured actuals

**FR-7.1 — The marker** is `executions.measured` (FR-1.1), and, for a
feature's spec and plan, the latest writing act's kind in `document_writers`.
SD-13 says what is unmeasured.

**FR-7.2 — `store.ActualTokens`** returns the tokens and whether the entity
is unmeasured. Its callers:
- `RetrieveCorpus` (the estimator's calibration corpus) and
  `RecentCalibration` (the dashboard's) leave unmeasured entities out **in
  SQL, before their `LIMIT`**, with a `NOT EXISTS` on unmeasured executions
  and writers, so that they still return their full count of neighbours;
- the estimate-against-actual API returns `"actual_tokens": null` and
  `"delta": null`, with `"unmeasured": true` and `"measured_part": n`; the CLI
  prints "unmeasured (at least n tokens)" in place of both.

**FR-7.3 — Wherever an actual is shown** for an unmeasured task, feature or
initiative, it reads "Not measured: at least 12,400 tokens, plus work done in
chat or by a person." rather than a number on its own.

**FR-7.4 — Forecasting** (`PurposeTokenSamples`, the send screen's per-step
forecast) samples dispatches by purpose. Chat and human work makes no
implement dispatch, so it can't enter the `implement-task` sample, but an
agent round that finished a released claim's half-done work is a biased low
sample. The query samples an `implement-task` dispatch only if every
execution in its task's round is measured. A test proves an unmeasured round
isn't sampled.

**FR-7.5 — Cost is unchanged.** Cost rollups sum dispatches, which are real
money. An unmeasured feature's cost line adds "It also includes work done in
chat or by a person, which isn't measured."

### FR-8: The reviewer for the chat agent's work

**FR-8.1 — Configuration:** `claims.chat_review_model` (SD-14): absent, a
model name, or `same`. The loader refuses a name that isn't in `models`.

**FR-8.2 — The default** is the model in `models` with the highest
`price_per_mtok.output`, ties broken by name. A project with one model gets
that model.

**FR-8.3 — Where it applies:**
- the code review queued by `SubmitWork`, and any code review of a task whose
  latest round's latest execution is `chat`;
- `queueReview` for a spec, plan or bug report whose latest writing act
  (`wrote`, `revised` or `added`) is `chat`.

Otherwise the model is as now: the role's, or `routing`'s override. Unless it
is `same`, the chat reviewer model wins over `routing` for those reviews.

**FR-8.4 — It is visible.** The review's run already shows its model. The
submit's result names it (FR-3.4), and the task page says "Reviewed with
*model*, the project's reviewer for the chat agent's work."

**FR-8.5 — The starter config** gains a commented `claims:` block:

```yaml
# Work a person or the chat agent claims instead of an agent (DEC-007). A
# claim with no activity for expiry_hours asks a person whether anyone is
# still working on it. The chat agent's work is reviewed with
# chat_review_model: by default the most expensive model below; name a
# model, or say "same" to use each reviewer's usual model.
claims:
  expiry_hours: 24
  # chat_review_model: same
```

### FR-9: What the chat agent is told

**FR-9.1 — `chat-skills/work-a-task/SKILL.md`**, next to `review-design`,
installed by `subutai init` the same way. It says:
- when to use it: the person wants to do a task with you rather than leave it
  to an agent;
- the path: `get_feature` to find a task you can claim; `claim_task`; read the
  contract; work only in the working copy; run the project's commands; don't
  commit; `submit_task` with a summary;
- what you may not do: review, approve or verify; edit outside the working
  copy; claim a second task in the same feature; start building; answer the
  Inbox;
- that a send-back comes back to you: `get_feature` shows the comments, and
  `claim_task` resumes it;
- that if you can't finish, you tell the person, who releases it in the web
  UI;
- that your tokens aren't measured, and your work is reviewed by an
  independent reviewer, by default a stronger model.

**FR-9.2** The `claim_task` result's `rules` and the skill say the same
things. A test checks that every rule in the result appears in the skill's
text, so they can't drift.

### FR-10: The end-to-end proof

**FR-10.1 — `TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents`**, with
the mock provider against real Postgres, the dispatcher held where a step
needs it:
1. A feature with an estimate and a two-task plan, T01 and T02 independent,
   is started (a person presses Start building over the API).
2. With the dispatcher held, the chat agent claims T01 over MCP. T01's queued
   implement dispatch is cancelled; T02's waits behind the claim, with the
   queue reason.
3. It writes a file in the working copy and submits. T02's implementer runs
   while T01 is reviewed.
4. The code reviewer (mock) asks for changes. T01's claim is `returned`, and
   no implementer runs for it. `get_feature` shows the comments; `claim_task`
   resumes it and returns them.
5. It edits and submits again. The reviewer approves, with the chat reviewer
   model (the default, the priciest configured model, asserted on the
   dispatch row). T02's review uses the usual model.
6. The verifier (mock) runs and approves; the feature merges.
7. The task page, `get_feature` and the timeline say T01 was implemented by
   the chat agent and T02 by the implementer. T01's and the feature's actuals
   are unmeasured, and the feature isn't in the calibration corpus, though a
   measured feature with an estimate is.

**FR-10.2 — `TestPersonClaimsATaskInTheUI`**: claim, submit with a summary,
the review queued with the usual model, release of the chat agent's claim, and
the refusals as banners.

**FR-10.3 — `TestBugTaskCanBeClaimed`**: a bug's task (`BUG-007-T01`) is
claimed and submitted over MCP.

## 5. Non-functional requirements

- **NFR-1 — One service layer.** MCP and the UI call `ClaimWork`,
  `SubmitWork` and `ReleaseClaim`. Each act's database changes are one
  transaction with its audit row.
- **NFR-2 — The boundary holds** (FR-6). The advertised set gains exactly two
  tools.
- **NFR-3 — The files changed**, for M14's merge:
  - new: `0014_executors.sql`, `lifecycle/claim.go`, `store/claims.go`,
    `store/executions.go`, `server/claims.go`, `server/claim_sweep.go`,
    `server/mcp_claim_tools.go`, `server/ui_claims.go`,
    `ui/templates/executors.html`, `chat-skills/work-a-task/SKILL.md`, and
    tests;
  - shared: `lifecycle/task.go` (one event), `store/tasks.go` (the guard),
    `store/dispatches.go`, `dispatch/dispatch.go` (the start), `rules/rules.go`
    (checkpoint answers), `server/actions.go` and `actions_phase2.go`,
    `server/planner.go` (FR-2.12), `server/server.go` (the heartbeat),
    `server/documents.go` and `http.go` (post-commit), `server/worktree_ops.go`,
    `server/mcp.go` (one `append`, `initialize`), `server/mcp_tools.go`
    (`get_feature`), `server/ui.go` (routes), `server/ui_views.go`
    (`responseFor`), `server/ui_entity_pages.go`, `ui/templates/entity.html`
    (two includes) and the feature and plan task lists, `server/observe.go`
    and `timeline/timeline.go`, `store/corpus.go`, `store/sends.go`,
    `store/ui_reads.go`, `server/http_phase3.go`, `cmd/subutai/main.go`,
    `config/config.go` and `starter/starter.go`, and the tool-set test.
- **NFR-4 — Human prose**, in British English, in every sentence a person or
  the chat agent reads.
- **NFR-5 — Tested as before:** integration tests with the mock provider
  against real Postgres for every FR; `go vet ./...` clean, `gofmt -l` empty,
  and `go test -race -count=1 ./...` green, with the integration tests
  running.
- **NFR-6 — No git under a database transaction.** Git runs before the
  transaction, holding the working copy's lock (FR-2.8), and the transaction
  re-checks the state it depends on.

## 6. Open questions carried forward

- **A worktree per claim**, so claims and agents work in one feature at once.
- **Uncommitted edits outside a claim.** The next agent's `git add -A`
  sweeps them into its commit, where its code review sees them, but nobody is
  told. Watching for a dirty working copy between runs needs care about the
  moment between an agent finishing and Subutai committing.
- **Holding the merge** while an `unclaimed-commit` checkpoint is pending on a
  feature in `review`. Today the verifier may approve and Subutai merge before
  the person looks.
- **A stronger verifier** for a feature whose tasks were mostly done in chat
  (SD-14's alternative): DEC-007 decision 10's reasoning, "one mind through the
  whole chain", applies at least as strongly there.
- **Which person claimed, and which chat session**, with M15's identities.
- **A time box for spikes** (M14), through `deadline_at` and
  `deadlineQuestion`.

## 7. Bug-report candidates found while writing this

- **A batch's base commit is stale.** `dispatchReadyTasks` gives every task
  made ready together the same base commit, but they run one at a time, so
  each later task's code review diffs its siblings' work as well as its own.
  The fix is to set the base when the task's first implement dispatch starts.
- **The revision-in-flight *pause* answer clears `spec_stale`,** exactly as
  *continue* does, but doesn't dispatch. The next approval in the feature then
  dispatches its ready tasks anyway, so a pause doesn't hold. Claims are
  refused while the checkpoint is pending (SD-3), but not after a *pause*.

## 8. Changes after review

How each [REVIEW-020](../reviews/REVIEW-020-executors.md) finding was dealt
with.

| Finding | What changed |
|---|---|
| R20-1 (material) | SD-16 ends every claim on a feature in `store.TransitionFeature` on `abandon`, covering all three sites, and withdraws the stale checkpoint. The route list is corrected. FR-6.5 tests it. |
| R20-2 (material) | A new claim state, `returned`, holds the task but not the working copy (FR-2.1, FR-2.2, SD-6). Resuming it checks the copy is free. FR-6.5 tests a send-back while an agent runs. |
| R20-3 (material) | FR-2.7: one lock order (feature, task, dispatch); `StartImplementDispatch` re-checks under the feature's lock inside its transaction; `dispatchReadyTasks` locks and skips claimed tasks; `TransitionTask` gains a state guard; `RequeueDispatch` cancels a claimed task's dispatch. FR-6.5 tests the races. |
| R20-4 (material) | SD-3 lets a claim take any `active` task no agent is running, cancelling `queued` and `failed` dispatches and withdrawing the `dispatch-failure` checkpoint; a recently heartbeating attempt refuses. A *Retry* after that is a no-op (FR-2.7). FR-1.2's sentence is corrected. |
| R20-5 (material) | FR-1.1: uniqueness on `dispatch_id` and `claim_id`, any number of rows per round. FR-1.3 backfills from `dispatch.running` audit rows. |
| R20-6 (material) | Review comments are given whenever the round is after the first, whoever did the earlier round (FR-3.2, FR-3.6, FR-4.1), and the implementer gets them too (FR-2.12, SD-17). |
| R20-7 (material) | FR-2.3 step 3 resets a round-1 claim's base commit to the head; each execution records `start_head`, and FR-2.5's check compares with it. The agents' batch defect is in §7. |
| R20-8 (material) | SD-10 and FR-5.6 to FR-5.9: Subutai's commits are known by recorded hashes; rewritten branches are caught; `--not main`; `active` and `review` are watched; the fork point and `addWorktree` start the watch; the remaining false negative (a commit by someone else under an open claim) is stated. The branch name is corrected. |
| R20-9 (material) | SD-13 and FR-7.1: a feature whose spec or plan was last written in chat or by a person is unmeasured. FR-7.2 filters in SQL before the limit, and names the API's fields and the CLI. FR-7.4 requires every execution in the round to be measured. |
| R20-10 (material) | SD-2 and FR-2.10: `executions` is keyed on `ref_type` and `ref_id`; `deadline_at` and `deadlineQuestion`; `staleQuestion` and a release label; `prepareSubmit` and `prepareRelease` outside the transaction; `feature_id` null for a spike; `claim` added to the enum, and M14 adds `spike`. |
| R20-11 (material) | SD-14 now follows DEC-007 decision 10: by default, the most expensive configured model; `same` opts out. The empty default is an alternative, described as the departure it would be. A stronger verifier is in §6. |
| R20-12 | Comments and the claim's state are in `get_feature` (FR-3.6), a read; FR-3.4's `next` points there; claiming again is the deliberate renewal (SD-8). |
| R20-13 | FR-2.8: a per-working-copy lock across git and the transaction; `--no-optional-locks`; the fingerprint before the transaction; the sweep's update conditional on `open`; the post-commit watch asynchronous; FR-2.5 step 5 covers a release winning. |
| R20-14 | FR-2.5 allows a submit during a revision, with a notice. SD-3 refuses claims while a `revision-in-flight` checkpoint is pending, and when the working copy is missing. The *pause* weakness is in §7. |
| R20-15 | DEC-004 is cited in Authority; DoD 9 adds notes on DEC-004 and DEC-005; FR-6.1 changes the test's header comment; SD-1 answers SPEC-017's sentence. |
| R20-16 | FR-1.2 says when the round is counted, how it differs from the round cap's count, and which row is submitted. FR-1.4 names a released claim's part. |
| R20-17 | "`dispatchReadyTasks` fires it", not the dispatcher; the header names §17b's two questions; NFR-3 lists every file. |
| R20-18 | FR-5.4 withdraws on any activity and makes a late answer a no-op; FR-5.3 shows the last activity; FR-5.5 makes the expiry a pointer; SD-9 says downtime counts; tests backdate. |
| R20-19 | SD-5 (one MCP actor), SD-7 (the chat agent learns of a release on its next call), SD-18 (don't commit) and SD-10 (Subutai commits a person's submitted work) state the costs. |
| R20-20 | Refusals say what to do; the badge is a sentence; FR-7.3 is a sentence; straight quotes; no opening numeral; the verifier's claim is accurate; the `claim` event's name is explained; `claimable` and `why_not`. |
| R20-21 | FR-6.5 lists the races; FR-10.1 holds the dispatcher, makes T01 and T02 independent, and gives the feature an estimate; FR-10.3 claims a bug's task. |

## Definition of done

1. Every FR's acceptance passes in the suite.
2. `go vet ./...` is clean, `gofmt -l` is empty, and `go test -race -count=1
   -v ./...` is green, with the integration tests running.
3. **A demo with no AI provider.**
   - A fresh build serves a throwaway project from `/var/tmp/m13demo`.
   - Over plain MCP JSON-RPC: claim a task, edit in the working copy, submit.
   - Playwright with the pre-installed Chromium shows the executor icon, a
     person's claim in the UI, the "Is someone still working on this?"
     checkpoint (its claim backdated), and the unclaimed-commit checkpoint.
   - The dispatched code review and verification are proved by the
     mock-provider tests (FR-10), and the walkthrough says so.
   - Screenshots, and `docs/walkthrough-spec-020.md`.
4. A handoff note, `docs/notes/handoff-M13-2026-10-02.md`.
5. The roadmap's §11 marks M13 done, with a pointer to the handoff.
6. REVIEW-020 is recorded and its findings are dealt with (§8).
7. Sam approves this spec with the build.
8. **Eighteen choices need Sam's explicit yes:**
   1. The executor record is `executions`, one row per executor's part in a
      round, with SPEC-017's vocabulary (SD-1).
   2. Claims and executions are general, for M14's spikes too, with an
      optional deadline (SD-2).
   3. A claim can take a ready task, or an active one no agent is running,
      cancelling its queued or failed dispatches (SD-3).
   4. One hand in a feature's working copy: an open claim pauses the
      feature's dispatched implementers (SD-4).
   5. A claim's own surface submits it, and the chat agent can't release a
      claim (SD-5).
   6. A send-back returns to the claim, as `returned`, not to an implementer
      (SD-6).
   7. Releasing commits what was left, and hands the task to an agent (SD-7).
   8. What counts as activity: claiming, renewing and resuming, changes to
      the working copy, submitting, a send-back, and *Keep the claim*; reading
      doesn't (SD-8).
   9. One expiry, 24 hours by default, for people and the chat agent alike,
      with downtime counted (SD-9).
   10. "An active feature's paths" means its branch, watched while it is
       built or verified, with Subutai's commits known by hash (SD-10).
   11. The unclaimed-commit checkpoint is a notice with one answer, and the
       merge isn't held (SD-11).
   12. The chat agent doesn't claim estimation, spec or plan writing (SD-12).
   13. A person's work is unmeasured too, and so is a feature whose spec or
       plan was written in chat or by a person (SD-13).
   14. The chat agent's work is reviewed by default with the most expensive
       configured model, code and documents alike; `same` opts out (SD-14).
   15. Claiming takes no quote (SD-15).
   16. A claim ends when its task or feature ends, by any route (SD-16).
   17. The implementer is shown its reviewer's comments on rework (SD-17).
   18. The chat agent is told not to commit (SD-18).
9. **Dated notes, on approval:**
   - DESIGN-010 §17b: the two questions settled (SD-8, SD-9, SD-12);
   - DESIGN-010 §5b: SD-4's pause, SD-10's reading, and SD-14's default;
   - DEC-007: "Not decided here", answered by SD-8, SD-9 and SD-12; decision
     7, read as SD-10; decision 10's default, as SD-14;
   - DEC-004: its "drive any development-side lifecycle transition" read
     after DEC-007 decision 3, for claim and submit;
   - DEC-005: a claim cancelling a queued dispatch isn't directing one
     (SD-15).
