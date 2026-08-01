# Orchestration patterns

How to arrange agents around a piece of work: how many, in what topology, with what gates
between them, and when to run one agent instead of four.

The evidence for each claim is in
[agent-research-evidence-base.md](agent-research-evidence-base.md). The prompts themselves
are covered in [roles-skills-and-prompts.md](roles-skills-and-prompts.md), and the gate
content in [definition-of-done-and-ready.md](definition-of-done-and-ready.md).

---

## Table of contents

1. [Choose the pattern from the task, not the ambition](#1-choose-the-pattern-from-the-task-not-the-ambition)
2. [The stage table](#2-the-stage-table)
3. [Patterns that the field has validated](#3-patterns-that-the-field-has-validated)
4. [Keeping context from growing](#4-keeping-context-from-growing)
5. [Why build the orchestrator rather than adopt one](#5-why-build-the-orchestrator-rather-than-adopt-one)
6. [The landscape, briefly](#6-the-landscape-briefly)

---

## 1. Choose the pattern from the task, not the ambition

More agents is not better. The optimal architecture follows from two properties of the
work: whether the reasoning is sequential, and how many tools it needs.

| Task structure | Pattern | Agents | Example |
|---|---|---|---|
| Sequential reasoning, low tool density | Single agent, no delegation | 1 | Writing a specification or a design |
| Sequential with decision points | Single agent plus human gates | 1 + human | Design work |
| Evaluative, independent criteria | Maker-checker, or a specialist panel | 1–4 | Code review, plan review |
| Parallelisable, high tool density | Orchestrator with workers | 1 + N | Implementing tasks from one dev-plan |

**The rule that governs everything else:** parallelising sequential reasoning degraded
performance by 39–70% in every variant tested. Parallelising genuinely independent work
gained around 81%.

So parallelism fans out **across independent features**, never *within* the writing of one
document. One specification is one agent making one sequential pass. Five tasks from one
approved dev-plan are five agents.

Two related constraints:

- **Coordination overhead grows with tool count.** Past roughly sixteen tools, the tax of
  coordinating several agents rises disproportionately. Scope tools per role.
- **Start at the lowest level that works.** One agent with tools, then worker plus reviewer,
  then a small team. Never skip levels. If a single well-prompted agent reaches more than
  45% of optimal, adding agents buys little.

---

## 2. The stage table

The workflow's shape, with the pattern each stage should use. Cromwell encodes most of this
already — the rules engine maps events to dispatch purposes, the gates in
`internal/lifecycle/gates.go` enforce the prerequisites, and role assignments bind a purpose
to a role and skill. This table is the reasoning behind those bindings, in one place.

| Stage | Purpose | Pattern | Human gate | Effort budget |
|---|---|---|---|---|
| Design | — (chat-authored) | Single agent, sequential | Yes — a human approves a design | — |
| Design review | `review-design` | Single agent | Comments only, no verdict | 5–10 tool calls |
| Specification | `write-spec` | Single agent, sequential | Yes — approval is a gate | 5–15 tool calls |
| Spec review | `review-spec` | Single agent | Verdict | 5–10 tool calls |
| Dev-plan | `write-dev-plan` | Single agent, sequential | Yes — approval is a gate | 5–10 tool calls |
| Dev-plan review | `review-dev-plan` | Single agent | Verdict | 5–10 tool calls |
| Implementation | `implement-task` | Orchestrator with workers, parallel | No | 10–50 tool calls per task |
| Code review | `review-code` | One reviewer per task diff | Verdict | 5–10 tool calls |
| Verification | `verify-feature` | Single agent, always dispatched | Verdict, unskippable | 5–15 tool calls |
| Estimation | `estimate-work` | Single agent | No | 3–8 tool calls |

Three properties of the table are worth stating explicitly.

**Every authoring stage is single-agent.** Not because delegation is hard to build, but
because it makes the output worse.

**Worker count is capped where agents share coordination overhead, and uncapped where they
don't.** A panel of specialists all reading the same artefact saturates at four —
`Dispatch.Workers` defaults to exactly that. Implementation tasks from one dev-plan are
independent by construction, touch different files, and pay no coordination tax, so they're
limited by task count rather than by a fixed cap.

**Effort budgets belong early in the prompt.** An agent that sees "5–15 tool calls,
single-agent, do not delegate" before it reads the procedure calibrates its whole approach.
Agents cannot judge effort on their own, and writing code feels more productive than
writing a document, so without an explicit budget they under-invest in the thinking stages.

---

## 3. Patterns that the field has validated

Six patterns recur across independently built production systems. They're worth adopting
rather than rediscovering.

### 3.1 Short-lived agents with context pushed to them

Agents should be short-lived, start with the minimum they need, and not accumulate a long
history. This prevents context pollution and stops hallucinations compounding across a long
run.

The usual second half of this pattern is "and they query a shared knowledge store on
demand". **That half does not work**, and Cromwell has the evidence twice over: agents
ignore anything they must choose to fetch, and go searching files instead. See
[agent-research-evidence-base.md §4.1](agent-research-evidence-base.md#41-anything-an-agent-must-choose-to-fetch-will-not-be-fetched).

So the Cromwell form of the pattern is: **short-lived agents with context pushed to them.**
The dispatch prompt carries what the agent needs. Where it must pull, let it pull the way
it already knows — by reading and searching files.

There's an open gap here. Implementer dispatches carry the task, the spec, the dev-plan, and
a paragraph on tools — no project conventions, no architecture summary, no key interfaces.
The `implement-task` skill says "match what is already there", so every implementer
rediscovers the project's conventions from scratch on every dispatch, paying tokens each
time and landing somewhere slightly different each time. The fix that fits the design is an
approved project-conventions **document**, auto-surfaced into dispatch prompts. The review
path already auto-surfaces approved ancestor documents; the execution path currently
surfaces none.

### 3.2 Decomposition gates dispatch

Every mature system in the survey learned the same lesson: never dispatch an agent to a
task that isn't already granular, unambiguous, and specified. A task must have a
description, acceptance criteria, and resolved dependencies before it becomes dispatchable.

This is a process constraint, and Cromwell enforces it structurally — G1 blocks a feature
becoming ready until its contract is approved, and tasks come from an approved dev-plan.

It also justifies spending disproportionate effort on the dev-plan stage, since
decomposition quality predicts overall success better than any other factor. Worth checking
in a plan review: does every task have a clear description? Are dependencies declared? Is
each task sized for one agent working alone? Are there gaps — a missing test task, a
missing integration task, an orphaned dependency?

### 3.3 Claiming must be atomic

With a shared work queue, two orchestration paths can read the same unclaimed task and both
try to start it. The fix is a single state transition that only one claimant can win, with
a clear refusal for the loser.

This matters more now that work can be executed by a dispatched agent, by a chat agent, or
by a human. The claim discipline is what makes the three interchangeable without collisions.

### 3.4 Tell the receiving agent what it isn't getting

It isn't enough for the assembly step to respect a budget. The agent should know what was
trimmed and why, so it can ask for the missing piece rather than proceed on an incomplete
picture and invent the gaps.

Any context assembly that drops material should say so — what was cut, and roughly how much
of it.

### 3.5 Orchestration is tools plus code, not a framework

The orchestration state machine should be plain code with a tool surface, not a separate
daemon, a Python framework, or a durable-workflow engine. This keeps behaviour transparent
and auditable, and avoids a meta-framework abstraction between the work and the thing doing
it.

Cromwell goes one step further than the systems that validated this pattern. In those, an
AI agent calls orchestration tools in a conversation loop. In Cromwell the orchestrator is
event-driven Go with no model attached, which removes the failure mode entirely: code
cannot drift and cannot forget to close the loop.

That distinction is the whole reason the design is the way it is. The predecessor system's
orchestrator *was* an agent, and it abandoned dispatched work mid-run because its identity
and its completion constraints sat in the middle of an ever-growing context window. The
cure was not "stop spawning agents" — spawning was fine. The cure was moving the
loop-closing logic somewhere it cannot be forgotten.

### 3.6 Maker-checker with two termination controls

A maker produces, a checker evaluates against stated criteria, and the work goes back if it
fails. The checker needs explicit acceptance criteria to make consistent decisions, and the
loop needs a way to end.

The literature prescribes an iteration cap. That's necessary but not sufficient, because
the failure actually observed isn't disagreement — it's a reviewer querying finer and finer
detail. A plain cap escalates trivia to a human, which is noise.

So use both controls:

- **A severity gate ends the common case without a human.** Repeat while any *major* finding
  exists; stop when only minors remain and drop them. The verdict becomes derived rather
  than chosen: any major finding means request changes, no majors means approve. A useful
  side effect is that a reviewer holding a major finding cannot approve, which defends
  against rubber-stamping from the other direction.
- **A hard iteration cap catches genuine disagreement** — majors recurring round after
  round. That's the case the maker-checker research was actually describing.

Define *major* concretely so severity isn't a matter of feel. Cromwell's `review-code`
criteria map straight onto it: a failure of **correct, complete, in scope, or sound** is
major; style, naming, and tidiness are minor.

Guard both directions. Severity deflation — a real defect marked minor and silently dropped
— is rubber-stamping in a new costume. Severity inflation is milder; a reasonable trip-wire
is any review putting more than 30% of findings at the top tier.

Record dropped minors rather than discarding them. They are exactly what a retrospective
picks up once the cycle completes.

---

## 4. Keeping context from growing

When one session dispatches work repeatedly, its own context grows with every result it
absorbs. Three techniques keep it inside the useful band:

1. **Summarise after each dispatch completes.** Reduce the outcome to two or three sentences
   and an identifier. Don't retain full agent output in the coordinating context.
2. **Write progress to a document and start fresh.** Past roughly 60% window utilisation,
   write a progress summary and begin a new session that reads it.
3. **Sequence single-feature contexts.** Don't try to coordinate several features in one
   session. Finish one feature's work, write the summary, start the next.

A related pattern from Anthropic's multi-agent work: **let agent output bypass the
coordinator where it can.** An agent that writes its detailed result to a file and passes
back a lightweight reference improves both fidelity and performance, because the large
output never travels through a conversation history.

For a code orchestrator this is mostly free — the state lives in Postgres, not in a context
window. It matters most for chat-side work and for any session doing several things in a
row.

---

## 5. Why build the orchestrator rather than adopt one

This question was settled before Cromwell was built, and the reasoning holds. Recording it
so it isn't reopened without new information.

The load-bearing asset is the context a dispatch carries and the gates that decide whether
a dispatch happens at all. An external orchestration framework can call into that, but then
it's just a caller — the system is still doing all the work, and the framework has added a
dependency, an abstraction layer, and its own assumptions about state management, agent
identity, and session lifecycle.

Every general-purpose framework in the survey provides the loop and none of the semantics.
None of them models a document lifecycle, a specification that can go stale, a gate over
current state, or a contract that must be approved before work begins. That's the part
worth having.

The one thing the field did have first was **agent dispatch with git worktree isolation**,
and the container-per-agent and worktree-per-feature models both proved the isolation
approach sound. Cromwell has that.

---

## 6. The landscape, briefly

A condensed survey, kept because it's useful to know what exists before building something
new, and because several of these are worth watching.

### Frameworks and SDKs

| Tool | What it's best at | What it doesn't do |
|---|---|---|
| **mcp-agent** | The most protocol-native orchestration framework. Implements orchestrator-workers, parallel/map-reduce, router, evaluator-optimiser, swarm. Optional durable execution with pause and resume. Token counting with threshold callbacks. | No workflow semantics, no document management, no git integration. It's a library you compose. |
| **LangGraph** | The most mature durability story. Graph and node architecture with state checkpointing, crash recovery, human-in-the-loop interruption and resume. | Graph-first rather than protocol-first. No workflow ontology. |
| **OpenAI Agents SDK** | Clean agent, handoff, guardrail, and session model with good protocol client integration. | No workflow semantics. Optional session persistence only. |
| **AutoGen** | The largest ecosystem; layered APIs; a no-code studio. | State lives in conversation history. No context budgeting. |
| **CrewAI** | Role-based autonomous teams; event-driven flows with typed state. | Standalone by design — you fight the architecture to integrate anything. |

### Orchestration servers

- **Agent-MCP** — the closest existing thing to task tracking plus dispatch plus context
  assembly in one server. SQLite-backed, short-lived agents scoped to a single task, a hard
  limit of ten concurrent agents, file-level locking rather than worktrees. Its
  ephemeral-agent model is the validated pattern in §3.1. Small community, restrictive
  licence.
- **Network-AI** — a race-condition-safe blackboard with an explicit state machine, budget
  tracking, and audit log queries. The lesson worth taking is that budget tracking must be a
  first-class feature rather than something bolted on later.
- **claude-task-master** — requirements document to task breakdown to multi-provider
  dispatch, with selective tool loading.
- **dagger/container-use** — container isolation per agent plus branch isolation. The
  container analogue of worktree-per-feature.
- **kagan** — a terminal Kanban board and web dashboard that dispatches coding agents into
  git worktrees, in autonomous or paired mode. Structurally the closest existing tool. It
  has flat Kanban statuses, no document lifecycle, no gates over current state, and no
  contract that can go stale — an agent it dispatches starts with a task description, where
  a Cromwell dispatch starts with an approved specification, a dev-plan, a role identity,
  domain vocabulary, and named failure modes. Those aren't equivalent starts.
- **context-rot-detection** — context health monitoring: utilisation, degradation signals,
  recovery recommendations. Confirms that context quality decay is a real operational
  problem worth measuring.

### Where Cromwell sits

| Capability | Cromwell | Frameworks | Orchestration servers | kagan |
|---|---|---|---|---|
| Work tracking | Yes | No | Yes | Yes |
| Agent dispatch | Yes | Yes | Yes | Yes |
| Git worktree isolation | Yes | No | No | Yes |
| Deterministic, model-free orchestrator | Yes | No | No | No |
| Document lifecycle with approval | Yes | No | No | No |
| Gates as pure functions over state | Yes | Partial | Partial | No |
| Role and skill context assembly | Yes | Partial | Partial | No |
| Enforced per-role tool profiles | Yes | Partial | No | No |
| Token accounting | Yes | Partial | Partial | No |
| Document knowledge graph | No, deliberately | No | Some | No |

### One observation worth keeping

kagan built its dashboard before its context layer. Cromwell did the inverse, and the
command centre has a much deeper substrate to surface as a result. The general point stands
for anything added next: the read surface should follow the model, not lead it.
