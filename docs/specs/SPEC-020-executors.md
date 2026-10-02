# SPEC-020: Executors

**Status:** **Draft — for Sam's approval.** Authored by Claude, the M13 lead.
The author can't be the approval gate, so the decision is Sam's. An
independent review is recorded in
[REVIEW-020](../reviews/REVIEW-020-executors.md), and §8 says how each finding
was dealt with. Sixteen choices need Sam's explicit yes; they are the scope
decisions in §3, listed in DoD 8. The build doesn't wait for approval.
**Date:** 2026-10-02
**Roadmap milestone:** M13 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11: "chat and humans can do work through the front door". *Done when:* "a
task done in chat gets the same review and verification as an agent's."
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28): **§5b** (executors), §5c (what the chat agent may do), §8
(unmeasured tokens, attribution) and §17b (the three questions left to M13).
**Authority:**
- [DEC-007](../decisions/DEC-007-the-judgement-boundary.md), all ten
  decisions except 9 (a colleague's pull-request review, which is M15b);
- [DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1:
  the chat agent never holds a verdict, never starts building, and never
  answers a checkpoint;
- [DEC-005](../decisions/DEC-005-the-orchestration-boundary.md), as DEC-007
  supersedes it in part: state is held by code, and the boundary is enforced
  by leaving tools out.

**Builds on:**
- [SPEC-011](SPEC-011-send-to-development.md): Start building stays a person's
  act;
- [SPEC-012](SPEC-012-see-the-work.md): the timeline and the run pages;
- [SPEC-017](SPEC-017-chat-as-a-proper-seat.md): `document_writers`, its
  writer vocabulary (`agent`, `chat`, `person`), and the one function that
  says who did something (`whoWords`). The executor record here is the same
  idea for tasks, and uses the same words;
- [SPEC-019](SPEC-019-bugs.md): a bug is a feature row, so a bug's tasks are
  claimed like any other.

**Coordination:** M14 (spikes) is built in parallel.
- This spec owns migration `0014`. M14 owns `0015`. **M13 merges first.**
- M14's stage 2 claims spikes through this spec's claims. The claim record and
  the server's claim service are general over what is claimed (FR-2.1,
  FR-2.9). This spec writes no spike code.
- Shared files are kept to the smallest change: `mcp.go`'s tool list, the
  tool-set test, and `entity.html` (two includes; the markup lives in a new
  `executors.html`).

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6). Tool descriptions and tool results are also
what the chat agent reads, so they carry the rules (DEC-007 decision 3).

Five words are kept apart throughout:

- **Executor**: whoever does a piece of work. One of a dispatched agent (with
  its role and model), the chat agent, or a person (DEC-007 decision 1).
  SPEC-017 calls the executor of a document its *writer*.
- **Execution**: one round of implementing a task, by one executor. A task
  sent back by its code review has a second round, which may have a different
  executor.
- **Claim**: a person's or the chat agent's hold on a piece of work, from
  *claim* to *submit*, *release* or the work ending. A dispatched agent never
  has a claim; its dispatch row is its hold.
- **Submit**: handing claimed work back. For a task, it commits the working
  copy and queues the independent code review.
- **Judge**: give a verdict: code review, verification, a document review.
  Executors never judge (DEC-007 decision 2).

---

## 0. Framing

Today a task is only ever implemented by a dispatched implementer. Start
building moves each ready task to `active` and queues an implement dispatch in
the same transaction (`dispatchReadyTasks`). The implementer works in the
feature's one worktree, calls `submit_implementation`, and the server commits
its work, moves the task to `review`, and queues a dispatched code review.
After the last task is approved, the dispatched verifier checks the feature
against its spec and merges it.

DEC-007 opens the doing to two more executors, the chat agent and a person,
and keeps every judgement where it is. This spec builds that, and nothing
else:

- a record of who executed each round of each task;
- a claim, which takes a task away from the dispatcher and gives its executor
  the working copy and the contract;
- a submit, which is the claimed task's only way forward, and leads into the
  same code review and verification an agent's work gets;
- a safety net for claims nobody is working on, and for commits nobody
  claimed;
- an honest ledger: chat and human work is *unmeasured*, not free;
- a stronger reviewer, if the project chooses one, for what the chat agent
  did.

## 1. Goal

**One claim, which the definition of done checks directly** (the roadmap's
done-when):

> A task claimed over MCP by the chat agent is worked in the feature's working
> copy and submitted. Its work is committed, and a dispatched code reviewer
> reviews it, with the project's chat reviewer model when one is set. A
> send-back returns it to the chat agent's claim, not to an implementer. Once
> approved, the feature's remaining tasks go on as before, and the dispatched
> verifier verifies the whole feature and merges it. The task's page, the
> timeline and `get_feature` say the chat agent implemented it, and its tokens
> say *unmeasured*.

Three supporting claims:

> A person can do the same from the task's page in the web UI.

> A claim with no activity for the project's expiry time asks a person
> "Still working on this?", and a commit to a feature's branch that no claim
> or Subutai made asks a person to look at it.

> The chat agent still holds no verdict: no tool lets it approve, review,
> verify, release a claim, start building or answer a checkpoint, and
> `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` names `claim_task` and
> `submit_task` on purpose.

## 2. The audit: what exists, and what is missing

Checked against DEC-007, DESIGN-010 §5b and the code on `main` at `0055f6b`.

| Asked for | What exists | What is missing |
|---|---|---|
| The executor of each task (DEC-007 d1) | Implement dispatches carry a role and model. The task page lists its runs. Nothing names a task's executor, and nothing could name anyone but an agent. | An execution record (FR-1). |
| Claim and submit (d3) | The task event `claim` (ready → active) exists, but only the dispatcher fires it. `submit_implementation` is the dispatched implementer's outcome tool. | Claims, the tools, the UI, and the rules for a claim's rework (FR-2, FR-3, FR-4). |
| Ready tasks to claim | Start building, and every approval, moves ready tasks to `active` with a queued implement dispatch in the same transaction. A task is almost never `ready` for long. | A claim must be able to take an `active` task whose implement dispatch hasn't started (SD-3). |
| One worktree per feature | The governor runs one implement dispatch per feature at a time (`MutatingDispatchActiveForTask`). The server commits with `git add -A`. | A claim edits the same worktree, so it must be serialised with dispatched implementers and with other claims (SD-4). |
| Judging stays closed (d2, d4, d5) | Code review and verification are dispatched. No MCP tool gives a verdict. | Tests that a claimed task gets both, and the must-not-exist names (FR-6). |
| Claims expire (d6) | The heartbeat fails stalled dispatches. Nothing watches a person or the chat agent. | A claim sweep and a checkpoint (FR-5.1–FR-5.5). |
| Unclaimed commits (d7) | The post-commit hook watches documents on the main checkout. Nothing watches feature branches. | A branch watch (FR-5.6–FR-5.9). |
| Unmeasured tokens (d8) | Actuals are summed from `dispatches`. A chat-implemented task would show only its review's tokens, which reads as cheap, not as unknown. | The marker, and calibration leaving it out (FR-7). |
| A chat reviewer model (d10) | `routing` overrides a purpose's model for everyone. | A per-project setting keyed on the executor (FR-8). |
| A chat-side skill (d3) | `chat-skills/review-design` (M3). | `chat-skills/work-a-task` (FR-9). |

Two things the audit found that this spec relies on but doesn't change:

- **A dispatched implementer is not shown its reviewer's comments on rework.**
  `returnTaskCode` audits them as `task.review_comments`, and `planImplement`
  doesn't read them. A claim's rework does show them (FR-3.4), because a
  person or the chat agent can't read the audit trail from the contract.
  Fixing the agents' side is a bug report (§7).
- **Code review is never a person's verdict by default.** People only answer
  its escalations and deadlocks. That stays so.

## 3. Scope

### In scope

1. The execution record for tasks, on the task page, the feature page's task
   list, the timeline and MCP results (FR-1).
2. Claims: the record, the service, and the state rules (FR-2).
3. `claim_task` and `submit_task` over MCP, and `get_feature` listing tasks
   (FR-3).
4. Claim, submit and release in the web UI, for a person (FR-4).
5. Claim expiry and the unclaimed-commit check (FR-5).
6. The boundary: what the chat agent still can't do, and the tests (FR-6).
7. Unmeasured actuals, left out of calibration and forecasting (FR-7).
8. The chat reviewer model (FR-8).
9. The `work-a-task` chat skill and the `initialize` instructions (FR-9).
10. The end-to-end proof (FR-10).

### Out of scope (with destination)

| Item | Destination |
|---|---|
| A colleague's pull-request review counting as the code review (DEC-007 d9) | M15b |
| Spikes, their claims and their time box | M14 (stage 2 uses FR-2.9) |
| Per-user identity: which person claimed | M15. A person is the configured `ui_actor`. |
| Claiming spec writing, plan writing or estimating | Not built (SD-12). Chat-written specs and plans already go through `submit_for_review` (SPEC-017). |
| A separate worktree per claim, so claims and agents work in parallel | Follow-up (§7) |
| Showing the dispatched implementer its review comments on rework | Bug report (§7) |
| Uncommitted edits made outside a claim | Follow-up (§7). Only commits are watched (SD-10). |

### Scope decisions

Each is a choice Sam confirms (DoD 8). The recommendation comes first.

- **SD-1 — The executor record is a table of executions, one row per round.**
  `task_executions` (FR-1.1) mirrors `document_writers`: the same `kind`
  vocabulary (`agent`, `chat`, `person`), actor, model and run, and the same
  sentence builder. A task's executor, as shown, is the executor of its latest
  round, and the line names an earlier one when it differs (FR-1.4). The table
  is the place the unmeasured marker lives (FR-7.1). *Alternatives:* columns on
  `tasks` (loses rounds with different executors), or deriving it from
  dispatches and claims at read time (two sources for every reader, and no
  place for the marker).

- **SD-2 — Claims are a general record.** `work_claims` (FR-2.1) keys on
  `ref_type` and `ref_id`, and the server's claim service dispatches on a
  small Go interface per claimable kind (FR-2.9). M13 registers `task`; M14
  registers `spike`. The database doesn't restrict `ref_type`, so M14 needs no
  change to `0014`.

- **SD-3 — What can be claimed.** A task can be claimed when its feature is
  `active` (a person has pressed Start building), its spec is not being
  revised (`spec_stale` is false), it has no claim, and either:
  - it is `ready`; or
  - it is `active`, and its only implement dispatch that isn't finished is
    still `queued`. The claim cancels that dispatch in the same transaction.

  A `pending` task (its dependencies aren't done), a task in `review`, a done
  or abandoned task, and a task whose implement dispatch is running are
  refused, each with a sentence (FR-2.4). *Why:* the dispatcher moves ready
  tasks to `active` as it queues them, so without the second case almost
  nothing could ever be claimed.

- **SD-4 — One hand in a feature's working copy at a time.** The feature has
  one worktree, and the server commits it with `git add -A`. So:
  - a claim is refused while an implement dispatch is *running* on any task of
    the same feature, or while another claim on the feature is `open`;
  - while a claim is `open`, the governor holds every implement dispatch of
    that feature in the queue, with the reason "A person or the chat agent is
    working in this feature's working copy." (FR-2.6);
  - a claim that is `submitted` (in review) doesn't hold the copy, so agents
    may work on the feature's other tasks while it is reviewed.

  The cost: a claim pauses the feature's dispatched implementation until it
  is submitted or released. That is why expiry matters (FR-5), and why the
  default expiry is a day, not a week. *Alternative:* a worktree per claim,
  merged on submit (§7).

- **SD-5 — Submitting is the only way forward, and the claim's own surface
  submits it.** A claim made over MCP is submitted over MCP; a claim made in
  the web UI is submitted there. Neither can submit the other's. The chat
  agent has no way to give a claim back: if it can't finish, it says so to the
  person, who releases the claim in the web UI (FR-4.3). *Why:* releasing
  hands the work to a dispatched implementer, which spends tokens; and "submit
  is the only way out" is the rule the chat agent is given. *Alternative:* a
  `release_task` tool for the chat agent.

- **SD-6 — A send-back returns to the claim.** When the code reviewer asks
  for changes on a claimed task, the task goes back to `active`, the claim
  goes back to `open` with its activity clock reset, and no implementer is
  dispatched. The executor sees the comments when they claim again (`claim_task`
  on a task they hold is a renewal, FR-3.2) or on the task page. The review
  round cap still applies: at the cap, a person decides, as now.

- **SD-7 — Releasing keeps the work.** A person may release any open claim,
  the chat agent's included, from the task page or by answering "Still working
  on this?". Anything uncommitted in the working copy is committed as
  "subutai: T03 — work left by *executor*, released", so the dispatched
  implementer starts from it and its code review sees it in the task's diff.
  The task stays `active`, and an implement dispatch is queued. *Alternative:*
  discard uncommitted work on release.

- **SD-8 — What counts as activity on a claim** (DESIGN-010 §17b):
  1. claiming it, and renewing it (`claim_task` again on a task you hold, or
     the page's *Claim* again);
  2. any change to the feature's working copy: a new commit on its branch, or
     a change to its uncommitted files, as the claim sweep sees them;
  3. submitting it;
  4. a send-back from code review, which reopens it;
  5. a person answering "Still working on this?" with *Keep it*.

  Reading is not activity: `get_feature`, `get_timeline` or viewing the page
  doesn't keep a claim alive. A chat session that polls would otherwise keep a
  dead claim alive for ever.

- **SD-9 — One expiry, for people and the chat agent alike**
  (DESIGN-010 §17b). `claims.expiry_hours`, default 24 (DEC-007 decision 6).
  Either way the question goes to a person, and a stale claim costs the same:
  the feature's agents wait. *Alternative:* separate settings for chat and
  people.

- **SD-10 — "An active feature's paths" means its branch.** DEC-007 decision
  7 asks for a checkpoint "when a commit touches an active feature's paths and
  no task is claimed". Features don't own file paths; a feature's code lives on
  its branch, `subutai/<worktree path>`. So: a commit on an active feature's
  branch that Subutai didn't make, made while no claim on the feature was
  `open`, raises the checkpoint. Subutai's own commits carry its author
  (`subutai <subutai@localhost>`). Commits on `main` aren't watched: they
  aren't any feature's work until a feature merges. Uncommitted edits aren't
  watched either (§7).

- **SD-11 — The unclaimed-commit checkpoint is a notice with one answer.** Its
  question names the commits and their authors and says what the risk is: the
  code may escape code review, though the verifier will see it. Its one answer
  is *I've seen it*. Reverting or adopting the commits stays a person's job in
  git. *Alternatives:* *Revert them* as a second answer; or turning the commits
  into a task that is reviewed.

- **SD-12 — The chat agent doesn't claim estimation, spec writing or plan
  writing** (DESIGN-010 §17b). `claim_task` claims tasks only.
  - **Estimation:** an estimate is a forecast of measured work, made against
    the calibration corpus of measured actuals. The chat agent can't see that
    corpus in the same form, its estimate can't be calibrated, and the
    estimator run it would replace costs a few thousand tokens. A person can
    already enter an estimate in the web UI. So it isn't useful.
  - **Spec and plan writing:** SPEC-017 already gives the chat agent a lane:
    write it, `attach_document`, `submit_for_review`.

- **SD-13 — Unmeasured covers people too.** A person's tokens aren't zero
  either; tokens don't describe their work. Executions by `chat` and `person`
  are `measured = false`. A task with any unmeasured round is unmeasured, and
  so is any feature or initiative that contains one, for calibration. Its
  measured part (code reviews, any agent rounds) is still shown, as "at least
  …" (FR-7.3).

- **SD-14 — The chat reviewer model applies to every review of the chat
  agent's work.** `claims.chat_review_model` (FR-8.1) is used:
  - for the code review of a task whose latest round was the chat agent's;
  - for the review of a spec, plan or bug report whose latest writer is the
    chat agent (`document_writers`, SPEC-017; SPEC-017 §6 named M13 for
    this).

  It is empty by default, which keeps the role's model (or `routing`'s).
  Subutai can't know which model is stronger for a project, so the starter
  config shows the setting commented, with an example. *Alternatives:* code
  review only; or defaulting to the verifier's model.

- **SD-15 — Claiming takes no quote.** Claiming and submitting are doing, not
  relaying (DESIGN-010 §5c: "It may claim and submit work"). A claim's worst
  case is pausing a feature's agents until a person releases it, which is
  small and recoverable (DEC-006 Amendment 1's test). The claim is audited
  `via: mcp`.

- **SD-16 — A claim ends when its task ends, by any route.** Approval, a
  person's abandon, the review deadlock's abandon, and a feature's
  abandonment all end the task's claim (`ended`, with the reason), in the same
  transaction as the task's transition.

## 4. Requirements

### FR-1: The execution record

**FR-1.1 — Migration `0014`** adds `task_executions`, one row per round of
implementing a task. It cascades on the task's deletion.

| Column | Meaning |
|---|---|
| `id` | row id |
| `task_id` | the task |
| `round` | 1 for the first round, then one more for each send-back |
| `kind` | `agent`, `chat` or `person` |
| `actor` | the role, the MCP actor, or the person (`ui_actor`) |
| `model` | the model, for an agent |
| `dispatch_id` | the implement run, for an agent |
| `claim_id` | the claim, for `chat` and `person` |
| `via` | `agent`, `mcp` or `ui` |
| `measured` | true for `agent`, false otherwise (FR-7) |
| `inferred` | true for a row backfilled from dispatches |
| `started_at`, `submitted_at` | when the round began, and when it was handed in |

The database checks:
- an `agent` row has a `dispatch_id` and `measured = true`;
- a `chat` or `person` row has a `claim_id` and `measured = false`;
- `(task_id, round, dispatch_id)` is unique for agents, and
  `(task_id, round)` is unique for claims.

**FR-1.2 — Where executions are recorded.** Each in the same transaction as
the act it describes.

| Act | Recorded |
|---|---|
| An implement dispatch starts running (`MarkDispatchRunning`, purpose `implement-task`) | a new `agent` row for the task's current round, with role, model and run |
| A retried attempt of the same dispatch | nothing new: same row |
| A claim (FR-2.3), or a claim reopened by a send-back (SD-6) | a new `chat` or `person` row |
| Submit (FR-2.5), or the implementer's completion | `submitted_at` on the round's row |
| A release (SD-7) | `submitted_at` stays empty; the next agent round is a new row |

The current round is one more than the number of `request_changes` task
transitions the task has had. Two rows in one round can happen only when a
claim is released, or a claim takes over a queued dispatch whose earlier
attempt never ran; both are kept.

**FR-1.3 — Backfill, in `0014`.** For every implement-task dispatch that ran
(`started_at` set), an `agent` row, `inferred = true`, its round counted from
the task's earlier `request_changes` transitions at the dispatch's start time.
No claims existed before `0014`, so there is nothing else to infer.

**FR-1.4 — How an executor is said.** One function, `executorLine`, built on
SPEC-017's `whoWords`, used by the task page, the feature page, MCP and the
timeline.
- An agent is "the implementer (*model*)", linking to its run. The chat agent
  is "the chat agent". A person is their name.
- The line reads across rounds, as the writer line does:
  - "Implemented by the chat agent."
  - "Being implemented by sam, who claimed it 3 hours ago."
  - "Implemented by the implementer (claude-sonnet-5); reworked by the chat
    agent."
  - "Not started." when there is no round.
- An inferred row adds nothing: before `0014` every executor was an agent, and
  the dispatch proves it.

**FR-1.5 — On the task page** (`/ui/t/{id}`). An include, `task-executor`,
directly under the page heading, in the new template file `executors.html`.
It shows the executor line, and, for an unmeasured task, the sentence "This
work was done in chat or by a person, so its tokens aren't measured."

**FR-1.6 — On the feature page's task list**, and the plan's task list, each
task shows a small executor badge: an agent icon, a chat icon or a person
icon, with the executor line as its title text. A claimed task's badge reads
"Claimed".

**FR-1.7 — In the timeline.** New moments, from new audit kinds (FR-2.10):
- "Task claimed by the chat agent: *T03 title*" (`task.claimed`);
- "Task submitted by the chat agent: *T03 title*" (`task.claim_submitted`);
- "Claim released by sam: *T03 title*" (`task.claim_released`);
- the existing "Code review sent back *…*" moment is unchanged.
Each carries `By` (chat or person) as SPEC-012's moments do. The checkpoint
moments for FR-5 read "Waiting for a person: is someone still working on a
task" and "Waiting for a person: a commit no task claimed".

**FR-1.8 — In MCP results.** `get_feature` gains `tasks` (FR-3.6). Each task
entry carries `executor`: `{ "sentence", "kind", "who", "model", "run_id",
"measured" }`, or `{ "sentence": "Not started." }`.

### FR-2: Claims

**FR-2.1 — Migration `0014`** adds `work_claims`.

| Column | Meaning |
|---|---|
| `id` | row id |
| `ref_type`, `ref_id` | what is claimed: `task` in M13; M14 adds `spike` |
| `feature_id` | the feature whose working copy it uses, when there is one |
| `kind` | `chat` or `person` |
| `actor` | the MCP actor, or the person |
| `via` | `mcp` or `ui` |
| `state` | `open`, `submitted` or `ended` |
| `end_reason` | `done`, `released`, `abandoned` |
| `ended_by` | who ended it, for `released` and `abandoned` |
| `claimed_at` | when |
| `last_activity_at`, `last_activity` | when activity was last seen, and what it was (SD-8: `claimed`, `renewed`, `worktree`, `submitted`, `sent_back`, `kept`) |
| `worktree_seen` | the fingerprint the sweep last saw (FR-5.2) |
| `submitted_at`, `ended_at` | when |

The database checks: one claim that isn't `ended` per `(ref_type, ref_id)`
(a partial unique index), and one `open` claim per `feature_id` (SD-4).
`end_reason` is set exactly when `state = 'ended'`.

**FR-2.2 — The claim's states** are a small machine in `internal/lifecycle`
(`claim.go`), with the same contract as the task machine
(`IllegalTransitionError`):

| From | Event | To |
|---|---|---|
| (none) | `claim` | `open` |
| `open` | `renew` | `open` |
| `open` | `submit` | `submitted` |
| `submitted` | `send_back` | `open` |
| `submitted` | `done` | `ended` |
| `open` | `release` | `ended` |
| `open`, `submitted` | `abandon` | `ended` |

The task machine is unchanged except one event, `release` (active → active,
recorded so the audit trail says what happened). A claim's `submit` fires the
task's existing `implemented` (active → review); a send-back fires the task's
existing `request_changes` (review → active). Anything out of order is
refused by whichever machine it breaks.

**FR-2.3 — Claiming** (`Server.ClaimWork`, called by MCP and the UI). In one
transaction, locking the feature row first (FR-2.7):
1. Check SD-3 and SD-4. On failure, refuse with FR-2.4's sentence.
2. If the task is `ready`, set its base commit to the branch head (as
   `dispatchReadyTasks` does) and fire `claim` (ready → active). If it is
   `active` with a queued implement dispatch, cancel that dispatch
   (`state = 'cancelled'`, only `WHERE state = 'queued'`; if no row changed,
   an agent has just started, and the claim is refused).
3. Insert the claim (`open`, activity `claimed`, `worktree_seen` the current
   fingerprint) and the execution row (FR-1.2).
4. Audit `task.claimed`, with `via`, `kind`, the claim, and the cancelled
   dispatch if any.

A claim by the claim's own executor on a task it holds `open` is a renewal:
it changes nothing but the activity (`renewed`), and returns the same result.

**FR-2.4 — Refusals, each a full sentence** that says what to do instead.
For example:
- "FEAT-023-T03 can't be claimed yet: it waits for T01 and T02 to be done.
  Claim one of those, or wait."
- "FEAT-023 isn't being built yet. A person starts building from the
  feature's page; you can't."
- "An agent is already implementing FEAT-023-T02 in this feature's working
  copy. Claim a task when it finishes; get_feature shows when."
- "FEAT-023-T03 is claimed by sam. Only a person can release a claim, in the
  web UI."
- "FEAT-023-T03 is in code review. It comes back to whoever implemented it if
  the reviewer asks for changes."
- "FEAT-023's spec is being revised. Claiming waits until a person decides
  whether building continues."
- "FEAT-023-T03 is done."

**FR-2.5 — Submitting** (`Server.SubmitWork`). It takes the claim's executor
and surface, the task, and a summary (required, at least one sentence). In
order:
1. Refuse unless the executor holds an `open` claim on the task from this
   surface (SD-5).
2. Refuse when the working copy has no change since the task's base commit,
   committed or not: "Nothing in the working copy has changed since you
   claimed FEAT-023-T03. Make the change, then submit; or ask a person to
   release the claim."
3. Commit the working copy as the implementer's completion does, with the
   message `subutai: T03 — title`, the summary, and two trailers:
   `Subutai-Executor: chat` (or `person`) and `Subutai-Claim: <id>`. The commit
   author stays Subutai's, so the branch watch knows it (SD-10).
4. In one transaction: the task's `implemented` (active → review), the
   claim's `submit`, `submitted_at` on the execution, the code review
   enqueued (FR-8), and the audit row `task.claim_submitted` with the summary.

The result names the review and its model, and says the executor can't
review, approve or verify it.

**FR-2.6 — The governor.** `admit` holds an `implement-task` dispatch while
the feature has an `open` claim, with the queue reason above (SD-4).
`MutatingDispatchActiveForTask` gains the claim check, so both the governor
and FR-2.3 ask one function.

**FR-2.7 — Races.** Claiming and admitting lock the feature row
(`SELECT … FOR UPDATE` on `features`) before checking, in that order, so a
claim and a dispatch starting at the same moment can't both win.
`MarkDispatchRunning` already changes only a `queued` row.

**FR-2.8 — The code review's verdict on a claimed task.** The rules engine is
unchanged. The server's executors of its actions read the task's claim:
- `approveTaskCode`: the claim's `done` (ended, `done`), in the same
  transaction.
- `returnTaskCode`: for a claimed task, the claim's `send_back` (activity
  `sent_back`), a new execution row for the next round, the comments audited
  as now, and **no implement dispatch**. For an unclaimed task, unchanged.
- The round cap's `review-deadlock` and the escalation are unchanged; a
  person's *Approve* or *Abandon* there ends the claim (SD-16).

**FR-2.9 — The claimable interface** (for M14). The claim service calls a
`claimable` per `ref_type`:

```go
type claimable interface {
    // load returns the item, its feature (if any) and its working copy.
    load(ctx, tx, refID) (*claimTarget, error)
    // claimRefusal says why it can't be claimed now, or "".
    claimRefusal(ctx, tx, t *claimTarget) string
    // onClaim, onSubmit and onRelease move the item's own state.
    onClaim(ctx, tx, t, c *store.Claim) error
    onSubmit(ctx, tx, t, c *store.Claim, summary string) error
    onRelease(ctx, tx, t, c *store.Claim) error
    // contract is what the claimant is given.
    contract(ctx, t) (map[string]any, error)
}
```

The record, the expiry sweep (FR-5) and the "Still working on this?"
checkpoint are the claim service's, not the task's. M14 registers `spike` with
its own working copy and its time box; this spec doesn't.

**FR-2.10 — Audit kinds:** `task.claimed`, `task.claim_renewed` (not a
timeline moment), `task.claim_submitted`, `task.claim_released`,
`claim.activity` (only when a sweep sees a change; not a moment), and
`claim.ended`.

### FR-3: The MCP tools

**FR-3.1 — `claim_task`.** One argument, `task`: the task's ID (`FEAT-023-T03`).

Its description says, in sentences:
- what it does: takes a task of a feature that is being built, so you, with
  the person, implement it in the feature's working copy instead of an agent;
- that the feature's other agents wait while you hold it, so submit or tell
  the person promptly;
- that the only way forward is `submit_task`; you can't release it, and a
  person can, in the web UI;
- that your work gets an independent code review, and the feature an
  independent verification, and you can't review, approve or verify;
- that a claim with no activity for the project's expiry time asks the person
  whether you are still working on it;
- to edit only inside the working copy it returns, and not to commit on other
  branches.

**FR-3.2 — Its result**:
- `task`: the task entry (FR-3.6);
- `feature`: its ID, path and name;
- `working_copy`: `{ "path" (absolute), "branch", "base_commit" }`;
- `contract`:
  - `spec`: `{ "id", "path", "body" }` (its spec type: a bug's report, per
    SPEC-019);
  - `dev_plan`: `{ "id", "path", "body" }`;
  - `task`: `{ "id", "title", "description" }`;
  - `decisions`: the surfaced block the implementer would be given (SPEC-018),
    as text, when there is one;
- `review_comments`: on a reopened claim, the latest code review's comments
  (severity, file, line, text) and its run's ID;
- `rules`: a list of sentences: the same rules as the description, plus the
  commands the project allows its implementers (`commands`), so the chat agent
  knows how the project builds and tests;
- `expires`: "If nothing changes in the working copy for 24 hours, the person
  will be asked whether you are still working on this.";
- `next`: "Implement the task in the working copy, run the project's build and
  tests, then call submit_task with a summary."

A renewal returns the same.

**FR-3.3 — `submit_task`.** Two arguments: `task` (ID) and `summary`
(required). Its description says it hands the work to an independent code
reviewer, that a send-back returns it to you with the reviewer's comments, and
that it is the only way forward.

**FR-3.4 — Its result**: the task entry (now `review`); `review`: `{ "run_id",
"model" }` of the queued code review; `commit`: the commit's hash; and
`next`: "A code reviewer will review this. Call claim_task on FEAT-023-T03
later to see whether it came back with comments; you can't review or approve
it yourself."

**FR-3.5 — Refusals** are FR-2.4's sentences, as tool errors (`isError`).

**FR-3.6 — `get_feature` gains `tasks`**, in plan order. Each entry: `id`,
`title`, `state`, `depends_on` (IDs), `executor` (FR-1.8), `claim` (`{ "kind",
"who", "state", "since", "last_activity" }` when there is one), `claimable`
(true, or a sentence from FR-2.4), and `url`. This is the only read change: the
chat agent needs it to find a task to claim.

**FR-3.7 — The `initialize` instructions** gain two sentences: that the chat
agent may claim a task of a feature being built, work it with the person, and
submit it, and that its work is reviewed and verified by others; and that the
`work-a-task` skill says how.

### FR-4: The web UI, for a person

**FR-4.1 — The task page** gains a claim panel, `task-claim`, in
`executors.html`, below the description:
- No claim, and claimable: "You can implement this task yourself instead of
  an agent. Claiming it pauses this feature's agents until you submit it." and
  a **Claim this task** button.
- Not claimable: the FR-2.4 sentence, and no button.
- A person's open claim: who and since, the last activity, the working copy's
  absolute path and branch, links to the spec, the plan and this task, the
  review comments if it came back, a **Submit for code review** form (summary,
  required), and **Release**.
- The chat agent's open claim: who, since, last activity and the working copy,
  and **Release** ("The chat agent is working on this. Releasing it gives the
  task to an agent, and keeps what is in the working copy.").
- A submitted claim: "Submitted by *who*; a code reviewer is reviewing it."

**FR-4.2 — Routes:** `POST /ui/task/claim`, `/ui/task/submit`,
`/ui/task/release`, each with the task's ID, redirecting back to the task page
with a notice or the refusal as an error banner. They call the same service
methods as MCP (NFR-1), with `kind: person`, `via: ui`, actor `ui_actor`.

**FR-4.3 — Release** (`Server.ReleaseClaim`, UI only) does SD-7: commit what
is uncommitted (if anything) with the "work left by" message and the trailers,
the claim's `release`, the task's `release` event, an implement dispatch
queued (`enqueueImplementTx`), and the audit row `task.claim_released` with
who released it. A submitted claim can't be released: "It is in code review.
Wait for the verdict."

**FR-4.4 — The feature page** shows the executor badges (FR-1.6). A feature
with an open claim says, under its timeline strip: "*T03 title* is claimed by
the chat agent, so this feature's agents are waiting."

### FR-5: The safety nets

**FR-5.1 — The claim sweep** runs on the heartbeat (`server.go`'s loop), after
`StallSweep`. For each `open` claim:
1. Compute the working copy's fingerprint (FR-5.2). If it differs from
   `worktree_seen`, record activity `worktree` and the new fingerprint.
2. If `now − last_activity_at ≥ claims.expiry_hours` and no `claim-stale`
   checkpoint is pending for it, raise one (FR-5.3).
3. If activity was just recorded and a `claim-stale` checkpoint is pending,
   withdraw it (`state = 'withdrawn'`), with an audit row: the claimant came
   back.

**FR-5.2 — The fingerprint** is a hash of the branch's head commit and the
output of `git status --porcelain=v1 -z` together with a hash of the working
tree's tracked and untracked changed files' contents. It changes on a commit,
an edit, a new file or a deletion. It is computed only for open claims, so its
cost is bounded by how many people and chat sessions are working at once.

**FR-5.3 — The `claim-stale` checkpoint**, on the task:
- Question: "FEAT-023-T03 (*title*) was claimed by the chat agent 26 hours
  ago, and nothing has changed in its working copy for 24 hours. This
  feature's agents are waiting. Is someone still working on it?"
- Answers: **Keep the claim** (activity `kept`, the clock restarts), and
  **Release it to an agent** (FR-4.3, released by the person who answered).
- Context: the claim, its executor, the last activity, the working copy.

The chat agent can't answer it (DEC-006 Amendment 1; no tool).

**FR-5.4 — Configuration:** `claims.expiry_hours`, an integer, default 24,
at least 1. The config loader refuses anything else.

**FR-5.5 — A claim's checkpoint ends with it.** Submitting, releasing or
ending a claim withdraws its pending `claim-stale` checkpoint.

**FR-5.6 — The branch watch.** Migration `0014` adds `worktrees.watched_head`.
The claim sweep, and the post-commit endpoint (so a commit in a worktree is
seen at once: linked worktrees share the main repository's hooks), check every
live worktree of an `active` feature:
1. List the commits on its branch after `watched_head` (or after the branch's
   first commit, for a row with none): `git rev-list --reverse
   <watched_head>..<branch>`, with author and committer.
2. Drop Subutai's own commits (author email `subutai@localhost`).
3. If any remain and the feature has no `open` claim, raise the checkpoint
   (FR-5.7). If it has one, they are the claim's activity, not a finding.
4. Set `watched_head` to the branch head.

Submitting and releasing a claim set `watched_head` to the head they commit,
so a claimant's own commits, made while the claim was open, are never
reported after it closes. The watch runs on every heartbeat and on every
post-commit call, so the window between a commit and the check is seconds.

**FR-5.7 — The `unclaimed-commit` checkpoint**, on the feature:
- Question: "2 commits were made on FEAT-023's branch while no task was
  claimed: a1b2c3d by Sam Phillips, “quick fix”; … Work done outside a claim
  may miss its code review, though the verifier will see it. Claim the task
  next time, or tell the chat agent to."
- One answer: **I've seen it** (SD-11).
- Context: the commits (hash, author, subject, time).

While one is pending, further unclaimed commits are added to its context
rather than raising a second (checkpoints are one per kind and ref while
pending).

**FR-5.8 — `watched_head` starts at the branch head** when a worktree is
created (`StartFeature`), and is set for live worktrees by the migration, so
history before M13 is never reported.

**FR-5.9 — The Inbox** shows both kinds with their answers; `responseFor`
gains the two kinds, and the rules engine routes their answers:
`claim-stale` to keep or release, `unclaimed-commit` to nothing.

### FR-6: The boundary

**FR-6.1 — The advertised tool set gains exactly `claim_task` and
`submit_task`**, each with a comment in
`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` citing DEC-007 decision 3
and DESIGN-010 §5c.

**FR-6.2 — The must-not-exist list gains:** `release_task`,
`approve_task`, `review_task`, `review_code`, `verify_feature`,
`submit_verification`, `submit_implementation`, `claim_verification`,
`claim_review`, and `claim_estimate`. `submit_implementation` is the
dispatched implementer's outcome tool, and must never be offered to chat.

**FR-6.3 — Verification can never be claimed.** `claim_task` refuses
anything that isn't a task ID, including a feature's ID: "Only tasks can be
claimed. Verification is always done by Subutai's verifier." There is no
verification task to claim. A test asserts that a feature whose every task was
claimed and submitted still gets a dispatched `verify-feature` run, and that
no claim exists on the feature.

**FR-6.4 — Code review can never be skipped.** Submitting always queues a
dispatched `review-code`. A test asserts that a claimed task can't reach
`done` except through the reviewer's approval, or a person's answer at the
round cap or an escalation.

**FR-6.5 — Out-of-order acts are refused**, each tested: submitting
without a claim, submitting someone else's claim, submitting twice, submitting
a task in review, claiming a pending task, claiming during a running implement
dispatch, claiming a second task in the same feature, and releasing a
submitted claim.

### FR-7: Unmeasured actuals

**FR-7.1 — The marker** is `task_executions.measured` (FR-1.1). A task is
unmeasured when any of its executions is. A feature or initiative is
unmeasured when any task within it is (SD-13).

**FR-7.2 — `store.ActualTokens`** returns the tokens and whether the entity is
unmeasured. Its callers:
- `RetrieveCorpus` (the estimator's calibration corpus) leaves unmeasured
  entities out;
- `RecentCalibration` (the dashboard's calibration) leaves them out;
- the estimate-against-actual API returns `"actual": null,
  "unmeasured": true, "measured_part": n` for them.

**FR-7.3 — Wherever an actual is shown** for an unmeasured task, feature or
initiative, it reads "Unmeasured — at least 12,400 tokens, plus work done in
chat or by a person", not a number on its own.

**FR-7.4 — Forecasting** (`PurposeTokenSamples`, the send screen's per-step
forecast) samples dispatches by purpose. Chat and human work makes no
implement dispatch, so it can't enter the `implement-task` sample. The query
gains a guard all the same: a dispatch on a task is sampled only if that
task's execution in the dispatch's round is measured. A test proves an
unmeasured round isn't sampled.

**FR-7.5 — Cost is unchanged.** Cost rollups sum dispatches, which are real
money. An unmeasured feature's cost line adds "plus work done in chat or by a
person, which isn't measured".

### FR-8: The chat reviewer model

**FR-8.1 — Configuration:** `claims.chat_review_model`, a model name, empty
by default. The loader refuses a name that isn't in `models`.

**FR-8.2 — Where it applies** (SD-14):
- `SubmitWork` and any later code review of the task: when the task's latest
  execution is `chat`, the code review's model is `chat_review_model`, if set.
- `queueReview` for a spec, plan or bug report: when the document's latest
  writer act (`wrote`, `revised`, `added`) is `chat`, likewise.

Otherwise the model is as now: the role's, or `routing`'s override. When set,
the chat model wins over `routing` for that review.

**FR-8.3 — It is visible.** The review's run already shows its model. The
claim's submit result names it (FR-3.4), and the task page says "Reviewed with
*model*, the project's reviewer for the chat agent's work."

**FR-8.4 — The starter config** gains a commented `claims:` block:

```yaml
# Work a person or the chat agent claims (DEC-007). A claim with no
# activity for expiry_hours asks a person whether it is still being worked
# on. chat_review_model, when set, reviews the chat agent's work with a
# stronger model than usual.
claims:
  expiry_hours: 24
  # chat_review_model: claude-opus-5
```

### FR-9: What the chat agent is told

**FR-9.1 — `chat-skills/work-a-task/SKILL.md`**, next to `review-design`,
installed by `subutai init` the same way. It says:
- when to use it: the person wants to do a task with you rather than leave it
  to an agent;
- the path: `get_feature` to find a claimable task; `claim_task`; read the
  contract; work only in the working copy; run the project's commands; commit
  if you like; `submit_task` with a summary;
- what you may not do: review, approve or verify; edit outside the working
  copy; claim a second task in the same feature; start building; answer the
  Inbox;
- that a send-back comes back to you, and `claim_task` shows the comments;
- that if you can't finish, tell the person, who releases it in the web UI;
- that your tokens aren't measured, and your work is reviewed by an
  independent reviewer, possibly a stronger model.

**FR-9.2** The `claim_task` result's `rules` and the skill say the same
things; a test checks that every rule in the result appears in the skill's
text, so they can't drift.

### FR-10: The end-to-end proof

**FR-10.1 — `TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents`**, with
the mock provider against real Postgres:
1. A feature with a two-task plan is started (a person presses Start
   building over the API).
2. The chat agent claims T01 over MCP while its implement dispatch is
   queued. The dispatch is cancelled; T02's waits behind the claim.
3. It writes a file in the working copy and submits.
4. The code reviewer (mock) asks for changes; the task returns to the claim,
   and no implementer runs. `claim_task` renews it and shows the comments.
5. It edits and submits again; the reviewer approves, using
   `chat_review_model` (asserted on the dispatch row).
6. T02 is dispatched to the implementer and approved.
7. The verifier (mock) runs and approves; the feature merges.
8. The task page, `get_feature` and the timeline say T01 was implemented by
   the chat agent and T02 by the implementer; T01's and the feature's actuals
   are unmeasured; the feature is not in the calibration corpus.

**FR-10.2 — `TestPersonClaimsATaskInTheUI`**: claim, submit with a summary,
the review queued with the normal model, release of another claim, and the
refusals as banners.

## 5. Non-functional requirements

- **NFR-1 — One service layer.** MCP and the UI call `ClaimWork`,
  `SubmitWork` and `ReleaseClaim`, each act in one transaction with its audit
  row.
- **NFR-2 — The boundary holds** (FR-6). The advertised set gains exactly two
  tools.
- **NFR-3 — Coordination.** Migration `0014` only. `entity.html` gains two
  includes; the markup is in `executors.html`. `mcp.go` gains one
  `append(…, s.mcpClaimTools()...)`, with the tools in `mcp_claim_tools.go`.
- **NFR-4 — Human prose**, in British English, in every sentence a person or
  the chat agent reads.
- **NFR-5 — Tested as before:** integration tests with the mock provider
  against real Postgres for every FR; `go vet ./...`, `gofmt -l` on the files
  touched, and `go test -race -count=1 ./...` clean, with the integration
  tests running.
- **NFR-6 — No git work under a database lock.** The fingerprint, the branch
  watch and the submit's commit run outside transactions; the transaction
  re-checks the claim's state after.

## 6. Open questions carried forward

- **A worktree per claim**, so claims and agents work in one feature at once.
- **Uncommitted edits outside a claim.** The next agent's `git add -A`
  sweeps them into its commit, where its code review sees them, but nobody is
  told. Watching for a dirty working copy between runs needs care about the
  moment between an agent finishing and Subutai committing.
- **Which person claimed**, with M15's identities, and per-person expiry.
- **A time box for spikes** (M14): a deadline on the claim, beside the
  inactivity expiry.

## 7. Bug-report candidates found while writing this

- **A dispatched implementer is not shown its code review's comments on
  rework.** `planImplement` reads the spec, the plan and the task, but not the
  `task.review_comments` audit row `returnTaskCode` writes. The implementer
  reworks blind.

## 8. Changes after review

*(Filled in after REVIEW-020.)*

## Definition of done

1. Every FR's acceptance passes in the suite.
2. `go vet ./...` is clean, `gofmt -l` is empty, and `go test -race -count=1
   -v ./...` is green, with the integration tests running.
3. **A demo with no AI provider.**
   - A fresh build serves a throwaway project from `/var/tmp/m13demo`.
   - Over plain MCP JSON-RPC: claim a task, edit in the working copy, submit.
   - Playwright with the pre-installed Chromium shows the executor badge, a
     person's claim in the UI, the "Still working on this?" checkpoint, and the
     unclaimed-commit checkpoint.
   - The dispatched code review and verification are proved by the
     mock-provider tests (FR-10), and the walkthrough says so.
   - Screenshots, and `docs/walkthrough-spec-020.md`.
4. A handoff note, `docs/notes/handoff-M13-2026-10-02.md`.
5. The roadmap's §11 marks M13 done, with a pointer to the handoff.
6. REVIEW-020 is recorded and its findings are dealt with (§8).
7. Sam approves this spec with the build.
8. **Sixteen choices need Sam's explicit yes:**
   1. The executor record is `task_executions`, one row per round (SD-1).
   2. Claims are a general record, for M14's spikes too (SD-2).
   3. A claim can take a ready task, or an active one whose implement
      dispatch is still queued, cancelling it (SD-3).
   4. One hand in a feature's working copy: a claim pauses the feature's
      dispatched implementers until it is submitted or released (SD-4).
   5. The claim's own surface submits it, and the chat agent can't release a
      claim (SD-5).
   6. A send-back returns to the claim, not to an implementer (SD-6).
   7. Releasing commits what was left, and hands the task to an agent (SD-7).
   8. What counts as activity: claiming and renewing, changes to the working
      copy, submitting, a send-back, and *Keep the claim*; reading doesn't
      (SD-8).
   9. One expiry, 24 hours by default, for people and the chat agent alike
      (SD-9).
   10. "An active feature's paths" means its branch (SD-10).
   11. The unclaimed-commit checkpoint is a notice with one answer (SD-11).
   12. The chat agent doesn't claim estimation, spec or plan writing (SD-12).
   13. A person's work is unmeasured too (SD-13).
   14. The chat reviewer model applies to code reviews and document reviews
       of the chat agent's work, and is empty by default (SD-14).
   15. Claiming takes no quote (SD-15).
   16. A claim ends when its task ends, by any route (SD-16).
9. **Dated notes, on approval:** DESIGN-010 §17b (the two questions settled,
   SD-8, SD-9, SD-12), §5b (SD-4's pause and SD-10's reading), and DEC-007
   ("Not decided here", answered by SD-8, SD-9 and SD-12; decision 7, read as
   SD-10).
