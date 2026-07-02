# Cromwell — Vision (Draft v1)

**Status:** Draft for review
**Author:** Sam + AI assistant
**Date:** 2026
**Scope:** Greenfield successor to kanbanzai. Combined product vision (Part 1) and architectural sketch (Part 2).

---

## 1. Introduction

### Why a new system

Kanbanzai works, but the past year of dogfooding has shown its limits. Three honest observations:

- The chat-based orchestrator is structurally unreliable. Long contexts cause latency, and the conflict of interest between "make progress" and "respect gates" leads the orchestrator to invent shortcuts and use overrides to push work through. The P44 pipeline autopsy made this concrete.
- Treating YAML files in git as the canonical state for high-frequency workflow transitions produces real performance and reliability problems. Merge conflicts, slow reads, opaque audit trails.
- The vocabulary and tooling are stratified by historical accident rather than design. Plan/Batch/Feature/Spec/Dev-plan/Task is more concepts than the domain needs, and the relationships between them have been patched several times.

Rather than patch further, this document describes a fresh design that absorbs what kanbanzai got right and replaces what it got wrong.

### What this document is

A combined product vision and architectural sketch for **Cromwell**, a planning, workflow, and orchestration system for small teams building large software projects with AI agents.

Part 1 (sections 2–6) describes what Cromwell is from the outside: the vocabulary, the planning model, the way work flows from concept to ship.

Part 2 (sections 7–12) describes how Cromwell works internally: the server architecture, the orchestrator, the agent-reviewer mechanism, the content store, observability, and bootstrapping.

The two parts are inseparable. Part 1 makes promises about honesty, snappiness, and traceability that only hold if Part 2 is built correctly.

### What this document is not

A detailed specification. Specific schemas, APIs, UI mockups, and migration tooling are out of scope and will be addressed in dedicated design documents downstream. Section 14 lists known open questions.

---

# Part 1: What Cromwell Is

## 2. Concept

### One sentence

Cromwell is a document-led, specification-centred workflow system that takes a software project from initial concept through to shipped features, with humans driving planning and AI agents driving development.

### Two halves, one pivot-point

```
        PLANNING                  ←  spec  →                 DEVELOPING
   (human-driven, AI-assisted)                        (AI-driven, human-overseen)
```

The **specification** is the contract. Before the spec is approved, humans (with AI help) shape what's being built. After the spec is approved, AI agents (with human oversight) build it. The spec defines the handoff.

This is the same idea kanbanzai has, but Cromwell takes it further by making both halves first-class. Kanbanzai grew development-side tooling first and bolted on strategic planning. Cromwell starts with both.

### Document-led

The main interaction between humans and Cromwell is through documents. Initiatives have descriptions and design docs. Features have specs and dev-plans. Decisions are notes. Skills, roles, and templates are Markdown.

Cromwell is the system that manages those documents, runs the workflow around them, and orchestrates the agents that produce and consume them. The documents themselves stay where developers expect them: as Markdown files in the project repo, edited in whatever editor the human prefers.

### Single-install, single-project

One Cromwell installation manages one project. Multi-project planning (cross-project roadmaps, organisation-wide views) is a hosted-Cromwell concern, out of scope for the core product.

---

## 3. The Vocabulary

Cromwell has a small number of concepts. Each one means exactly one thing.

### Work entities (where work is *defined*)

| Term | What it is | State |
|---|---|---|
| **Initiative** | A strategic container. Nestable. Has documents and a dashboard. | `archived` bit only |
| **Feature** | A buildable unit with a specification. The boundary between planning and developing. | Full lifecycle |
| **Spec** | A document type. Describes what a Feature will do. | Document lifecycle |
| **Dev-plan** | A document type. Describes how the Feature will be built. | Document lifecycle |
| **Task** | A unit of implementation work. | Existing kanbanzai-style lifecycle |
| **Defect** | A reported problem. Feature-shaped pipeline but with triage states. | Custom lifecycle |
| **Note** | A short piece of project knowledge — a decision, a constraint, an observation. | Document lifecycle |
| **Job** | A single human task in a checklist (e.g. "register API key"). | `pending` / `done` |
| **Checklist** | A bundle of Jobs. | Computed from Jobs |

### Tracking entities (where work is *measured*)

| Term | What it is | State |
|---|---|---|
| **Milestone** | A named query + target date + description. Tracks deliverables shipping together. | `open` / `locked` |
| **Roadmap** | An ordered sequence of Milestones. | Computed |

### Documents (cross-cutting)

All documents — specs, dev-plans, designs, notes, research, reports — share one lifecycle: `draft → reviewing → approved → superseded`. All are indexed and searchable. All can be attached to any work entity.

That's the entire vocabulary. **Nine work-and-document concepts, two tracking concepts, one lifecycle.**

### Things deliberately not in the vocabulary

- **Plan** as an entity name (the word now only appears as the document type "dev-plan")
- **Batch** as a grouping layer (initiatives nest; milestones group for shipping)
- **Strategic-plan** as a separate entity (it's just a top-level Initiative)
- **Stage-bindings** as a configuration layer (the lifecycle of documents and Features *is* the gate system)
- **Knowledge** as a separate concept (it's a document of type "note")
- **Decision** as a separate entity (it's a document of type "note" with a `decision` tag)

Every one of these collapses to something simpler in Cromwell.

---

## 4. The Flow

### Strategic shaping

Work starts as an Initiative. The team writes a top-level description, perhaps a design document, and maybe nests sub-initiatives as the scope clarifies.

Initiatives don't have a lifecycle. They exist as filing-system containers — namespaces with documents and dashboards. They are archived when abandoned. Otherwise they just *are*, and the work happening underneath them gives them their character.

```
Initiative: Authentication           (no state; tree of work)
├── Initiative: Basic auth band
│   ├── Initiative: Email/password
│   └── Initiative: Basic passkey
├── Initiative: Enhanced auth band
│   ├── Initiative: SMS 2FA
│   └── Initiative: WebAuthn
└── Initiative: Federation           (archived; no longer in scope)
```

### Crossing the boundary: from Initiative to Feature

When the team is ready to actually build something, they create a **Feature** under the relevant leaf Initiative. This is the moment of commitment: above Features is exploration; at and below Features is execution.

A Feature has:
- A **Spec** (what it does, in user-facing terms, with acceptance criteria)
- A **Dev-plan** (how it will be built, decomposed into tasks)
- **Tasks** (the implementation units)
- A lifecycle (idea → ready → active → review → done)

Creating a Feature is the only explicit handoff in the system. Above it, things are flexible and aspirational. Below it, the contract is real.

### Producing the specification

The Spec is drafted by a human or AI author. Once submitted, it is:

1. **Validated** synchronously — structure check, required sections, link integrity. Mechanical. Pass/fail.
2. **Reviewed** asynchronously by an agent-reviewer — is it clear, complete, testable? Outcomes: approve / request_changes / escalate.

If the reviewer approves, the Spec advances. If it requests changes, comments go back to the author. If it escalates (genuinely uncertain or disagreement with the author beyond what comments can resolve), a human is pinged via a checkpoint in the inbox.

**The human gate fires only on escalation, not by default.** This is the change that makes the pipeline snappy.

### Producing the dev-plan

Same flow. Drafted, validated, agent-reviewed, possibly escalated. When approved, tasks are decomposed and the Feature enters `active`.

### Implementation

Tasks are claimed by implementer agents. Each task runs in its own dispatch — a self-contained agent call with assembled context (spec sections, dev-plan sections, relevant notes, role conventions). When the task finishes, it transitions to review.

### Code review

A reviewer agent inspects the diff against the spec and dev-plan. Same three outcomes: approve / request_changes / escalate.

### Merge and ship

When all tasks for a Feature are approved, the Feature transitions to `done` and merges. Done Features become available for inclusion in milestones, dashboards, retrospectives.

### Tracking what's shipping

Milestones group Features (and other deliverables) for shipping. A milestone references its members **live** — adding a new Feature under a referenced Initiative includes that Feature in the milestone automatically. Members can be any combination of Initiatives (transitive), Features (direct), Checklists, or nested Milestones.

When a milestone ships, the team **locks** it. Locking snapshots the membership to a flat list of leaves. Progress against that frozen membership continues to compute (so a member finishing late updates the historical view), but the *set* of what was promised is fixed.

If the team needs to ship without something, they remove it from the milestone before locking. **No fudging.** A locked milestone is the true record of what was shipped, including what was descoped along the way (visible in the milestone's notes and history).

### Roadmaps

A roadmap is an ordered list of milestones. The ordering is whatever the planner wants it to mean — sequence, priority, schedule, theme. The system makes no claims about semantics; it just preserves the order.

---

## 5. Sizing and Cost

### Tokens as the unit

Cromwell sizes work in **tokens** — the actual unit of AI computation. Not story points, not t-shirt sizes, not hours. Tokens.

Three reasons:

- **They're objective and free.** Every provider API returns token counts. We capture them without extra instrumentation.
- **They're interruption-immune.** A task that pauses, resumes, retries — tokens just accumulate. Sum at the end.
- **They map directly to cost.** No other unit lets you answer "what did this feature cost?" honestly.

### Estimates and confidence

Each estimate carries a confidence tier, assigned automatically from the evidence behind it:

| Tier | Rule | Colour |
|---|---|---|
| **Decomposed** | The estimate is the sum of child entity estimates. Arithmetic, not judgement. | 🟢 |
| **Considered** | Single-unit estimate, but the system consulted the reference corpus of similar past work. | 🟡 |
| **Rough** | Pure judgement from reading the description. No decomposition, no corpus lookup. | 🔴 |

When estimates roll up to a parent (Initiative, Milestone), the parent's tier is the **worst tier in the tree** — a single rough leaf taints the whole estimate. Honest.

Unestimated work shows as `?` and is listed explicitly so humans can see what's missing.

### Actuals and calibration

When work completes, the system records actual token consumption alongside the estimate. The growing corpus of (description, estimate, actual) tuples is used to anchor future estimates: when the system needs to estimate a new Feature, it retrieves the nearest neighbours by description-embedding similarity and shows the agent reference points before asking for a number.

Over time, estimates sharpen and the rough/considered tiers carry meaningful predictive power.

### Cost

Cost is derived from tokens × per-model price table (input and output priced separately; cached input tokens priced separately again where the provider supports caching). The price table is project config.

Dashboards show cost rolled up at any level:

- Cost per Task
- Cost per Feature
- Cost per Initiative (sum of all descendant Features)
- Cost per Milestone
- Cost per Roadmap
- Cost-per-month for the whole project

This unlocks real planning conversations: "auth has cost $342 so far," "the Q3 milestone is projected at $410 with $500 budget," "review tasks are 40% of our spend, should we use a cheaper model for first-pass reviews?"

### Human work and sizing

Human-authored work (writing specs, designs, reviews) is **excluded from sizing**. Cromwell measures AI execution; humans are the planners. If a project wants to track human time separately, it can use an optional "human hours" tag on tasks, but this is orthogonal to the sizing/cost system.

---

## 6. Documents Everywhere

### The unified content store

Every piece of written content in Cromwell is a **document**. Specs, dev-plans, designs, research reports, retrospectives, notes (formerly "knowledge entries") — all share one shape:

- A type (`spec`, `dev-plan`, `design`, `research`, `report`, `note`, `policy`)
- A lifecycle (`draft → reviewing → approved → superseded`)
- An owner (an Initiative, a Feature, or the project)
- A path on disk (Markdown file in git)
- An indexed representation (sections, fragments, embeddings, tags) in Postgres

Knowledge is not a separate subsystem. A "note" is a short document. It has the same lifecycle and retrieval machinery as everything else. This collapses one of the most confusing parts of kanbanzai (knowledge vs. documents vs. doc-intel) into one model.

### Auto-surfacing

When the orchestrator assembles a prompt for an agent working on a Feature, relevant documents are **automatically included** based on:

- Direct attachment (the Feature's spec and dev-plan)
- Ancestry (designs and notes on parent Initiatives)
- Tag overlap (notes tagged with the same concepts)
- Embedding similarity (notes whose content is semantically close to the spec)

The agent doesn't have to know to ask. The relevant material is just in context. This fixes the kanbanzai problem where 86 knowledge entries were contributed but only 12 ever retrieved — retrieval was opt-in and agents didn't remember to look.

### Validation vs. review

The distinction is sharp and applies to all documents:

- **Validation** is mechanical and synchronous. Required sections present? Links resolve? Schema correct? Pass/fail. Reported on save. No judgement.
- **Review** is semantic and asynchronous. Is this spec actually clear? Is this dev-plan a sensible decomposition? Runs as an agent-reviewer call. Three outcomes: `approve`, `request_changes` (with comments), `escalate` (defer to human).

Review can include validation results (a reviewer might cite a validation failure as a reason to request changes), but they're separate operations.

---

# Part 2: How Cromwell Works

## 7. Architecture

### Three components, three boundaries

```
┌─────────────────────────────────────────────────────────┐
│                  Cromwell Server                         │
│                                                          │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────┐ │
│  │ Orchestrator│  │ State (PG)   │  │ Dispatch       │ │
│  │             │──│              │──│ (provider APIs)│ │
│  └─────────────┘  └──────────────┘  └────────────────┘ │
│         ▲                ▲                  ▲           │
│  ┌──────┴────────┐  ┌────┴──────┐  ┌───────┴────────┐ │
│  │ Web UI        │  │ MCP facet │  │ Git watcher    │ │
│  │ (dashboards,  │  │ (chat     │  │ (document      │ │
│  │  checkpoints, │  │  agents)  │  │  changes →     │ │
│  │  planning)    │  │           │  │  events)       │ │
│  └───────────────┘  └───────────┘  └────────────────┘ │
└─────────────────────────────────────────────────────────┘
       ▲                  ▲                  ▲
       │                  │                  │
   Browser           Editor / Chat        Git repo
   (humans)          (assistants)         (documents)
```

Three sources of truth, each owning a different concern:

| Source | Owns | Examples |
|---|---|---|
| **Postgres** | Entity state, document index, audit, cost ledger, calibration corpus | Initiatives, Features, milestones, document lifecycle, token consumption |
| **Git repo** | Documents and code | `.md` files, source code, tests |
| **Project config (`.cromwell/`)** | Skills, roles, templates, schema overrides | Markdown skill files, YAML role files, prompt templates |

These don't overlap. Postgres has no Markdown content. Git has no entity records. Config files have no state. This separation is critical for keeping the system understandable.

### Postgres (with Supabase as recommended host)

The canonical state store. Schema includes entities (initiatives, features, milestones, etc.), document records (with paths to the on-disk files), full-text and embedding indexes over document content, audit events, token/cost ledger, calibration corpus, checkpoints, and worktree tracking.

Supabase is recommended (not required) because it provides managed Postgres, realtime push (powers the live web UI), auth, and backups out of the box. A local install can run plain Postgres.

### Server

A long-running process (Go or Node, decision deferred) that:

- Hosts the orchestrator
- Serves the web UI (HTTP)
- Serves the MCP facet (stdio or HTTP)
- Watches the git repo for document changes
- Listens to Postgres for state changes (via Supabase realtime or LISTEN/NOTIFY)
- Dispatches sub-agents to provider APIs

### Web UI

The primary human interface. Browser-based. Live-updated via realtime push. Used for:

- Strategic planning (initiative tree, milestone composition, roadmap ordering)
- Status monitoring (queue, in-flight work, cost burn, recent events)
- Checkpoint inbox (review requests, escalations, decisions)
- Document review (read drafts, leave comments, approve)
- Configuration (price tables, model routing, budgets)

The web UI is **not an editor**. Documents and code are edited in whatever editor the human prefers, with files in git. The UI is the command centre, not the workspace.

### MCP facet

A read/poke interface for AI assistants in editors and chat (Claude Desktop, Zed, Cursor). Exposes tools like:

- `cromwell.status(entity_id)` — query state
- `cromwell.search(query)` — content search
- `cromwell.find(concept|role|tag)` — doc-intel queries
- `cromwell.checkpoint_respond(id, response)` — help a human respond
- `cromwell.dispatch_review(doc_id)` — trigger a review (orchestrator runs it)

What it does **not** expose: direct entity mutation, gate overrides, raw agent spawning. Those are orchestrator concerns; humans drive them via the web UI. This separation prevents the kanbanzai failure mode where the MCP-connected chat agent became a rogue orchestrator.

---

## 8. The Orchestrator

### What it does

A server-side process that watches for events, decides what should happen next, and dispatches sub-agents to do it. It is **not** a chat agent. It has no model attached. It is plain code executing the workflow rules.

### Trigger model

Event-driven primarily, heartbeat as safety net.

Events that wake the orchestrator:

- Document state transitions (`draft → submitted`, `submitted → reviewed`)
- Git commits in document-bearing directories
- Task transitions (`active → done`, `done → review`)
- Human checkpoint responses
- Manual "go" actions from the web UI

Heartbeat (every ~30 seconds):

- Catch stalled work (agent process died, no event fired)
- Run retry policies for transient failures
- Garbage-collect abandoned worktrees and stale checkpoints

### Dispatch

When the orchestrator decides an agent should run, it:

1. Looks up the role and skill for the work type (from `.cromwell/roles/` and `.cromwell/skills/`)
2. Assembles the prompt: spec sections, dev-plan sections, relevant notes (auto-surfaced from the content store), role identity, skill procedure
3. Selects the model (from project config: implementer = X, reviewer = Y, spec-author = Z)
4. Calls the provider API directly (Anthropic, OpenAI, etc.)
5. Records the dispatch in the audit log
6. Records token consumption and cost in the ledger
7. Processes the response: updates entity state, files diffs, queues follow-up work

No MCP intermediation, no chat round-trip. Direct API call. This is the snappiness lever.

### Concurrency

The orchestrator dispatches in parallel by default, subject to:

- **Per-project budget caps** (don't burn $200 in a runaway loop)
- **Per-provider rate limits** (respect API quotas)
- **Worktree isolation** (each Feature has its own branch and directory; work on different Features can't conflict)
- **Within-Feature serialisation** (only one agent per Feature at a time by default, to avoid two agents editing the same files)

In practice: five Features can be in active development simultaneously, each with its own agent. Multiple specs can be reviewed in parallel (reviews are read-only). The single-machine server can drive a small team's worth of work without breaking sweat.

### Escalation

When the orchestrator can't proceed, it creates a **checkpoint**: a queued question for a human, surfaced in the web UI inbox. Common triggers:

- Agent-reviewer returned `escalate`
- Dispatch failed repeatedly (provider error, agent looped, etc.)
- Budget threshold reached
- Conflict detected (e.g. two milestones with incompatible targets)
- Human gate explicitly required (some operations are always human, e.g. archiving an Initiative with active Features under it)

The human responds in the UI; the orchestrator wakes and continues.

### Tool surface for dispatched agents

Dispatched agents need to *do things* — read files, search the codebase, query state, edit code. The orchestrator gives each agent a curated **toolset defined server-side**, exposed via the provider's native tool-use protocol (Anthropic `tool_use`, OpenAI function calling). The Cromwell server is the tool host: when an agent calls `read_file`, the server executes it inside the right worktree, returns the result, and the next turn happens.

Key properties:

- **Per-role tool profiles.** Each role in `.cromwell/roles/` declares the tools it can use (e.g. `tools: [read_file, search_graph, edit_file, post_finding]`). A `reviewer-security` agent has no `edit_file`. An `implementer` has no `merge_pr`. The orchestrator enforces the profile — the agent cannot escalate beyond its declared tools.
- **Implicit context injection.** Tools receive worktree, graph project, and Feature context from the orchestrator, not the agent. An agent calls `search_graph(query=…)` without knowing which graph project to target; the server injects it from the Feature's worktree record. Same for `edit_file(path, hash_ref, new_text)` — the worktree root is server-provided. This eliminates a whole class of "agent forgot to pass the right scope" bugs and saves tokens on every call.
- **Implementation is hidden.** A tool's body may call local Go code, an external MCP server, Supabase, or the GitHub API. The agent sees only the tool name and arguments. The Cromwell server is an MCP *client* for external services like codebase-memory-mcp, re-exposing their capabilities through curated tools.
- **Inherited investments carry over.** Two kanbanzai patterns are first-class in Cromwell:
  - **Per-worktree graph projects** (codebase-memory-mcp). The orchestrator derives the project name from the worktree path, triggers indexing on worktree creation, and tears down the index on garbage collection — all transparent to the agent. Graph queries stay scoped to the right branch state without the agent learning the naming convention.
  - **Hash-anchored read/edit protocol.** `read_file(hash_tag: true)` returns `line#hash| content` tags; `edit_file(hash_ref: "47#a3", new_text: …)` validates the line hasn't drifted since the read. This sidesteps stale-read races and "agent edited the wrong block" failures without forcing large context quotes. Role tool hints nudge agents toward this path; fuzzy fallback remains for greenfield writes.

What agents do **not** get:

- **Raw database access.** Postgres is the state store; agents interact through purpose-built tools (`update_task_status`, `record_finding`), not SQL. Raw DB from agents is a footgun.
- **Unfettered shell.** Tool profiles whitelist specific commands (e.g. `run_tests`) rather than expose a generic `shell` tool. Agents that want to do unusual things must escalate.
- **MCP client capabilities themselves.** Agents don't speak MCP. The server does, on their behalf. This keeps the audit trail centralised and the agent surface narrow.

The `cromwell-mcp` facet (Section 7) is the *inbound* direction: editor assistants like Zed or Cursor calling into Cromwell. The tool surface described here is the *outbound* direction: Cromwell calling out to capabilities and re-exposing them to dispatched agents. Two distinct surfaces, often confused.

ACP (Agent Communication Protocol) is explicitly out of scope. ACP is for peer-to-peer agent coordination. Cromwell's orchestrator is code, not an agent, and dispatched agents are leaf executors, not peers. If multi-agent collaboration patterns emerge later (e.g. reviewer agent mid-review querying the implementer agent), that's a separate design problem.

---

## 9. Review and Gates

### Document lifecycle as the gate system

Cromwell does **not** have a separate "stages and stage-bindings" system. Gates are computed from document state.

- Can this Feature start active work? → spec.approved AND dev-plan.approved
- Can this milestone be locked? → at least one member is `done`
- Can this Initiative be archived? → no active Features under it (or override with reason)

Every gate is an expression over current state. No parallel YAML config. The lifecycle of documents and Features *is* the gate system.

### Agent-reviewers

For every document type that has a `reviewing` state, a corresponding reviewer agent exists. Reviewers are first-class roles in `.cromwell/roles/`, configured the same as any other agent (model, prompt template, tool access).

When a document is submitted for review:

1. The orchestrator validates it (synchronous, mechanical)
2. If validation passes, the orchestrator dispatches the reviewer agent
3. The reviewer reads the document, the parent entity context, and ancestor docs
4. The reviewer returns one of: `approve`, `request_changes` (with comments), `escalate` (with reasoning)

`approve` → document advances to `approved` and downstream work unblocks.

`request_changes` → comments go back to the author (human or AI). The author addresses them and resubmits.

`escalate` → a checkpoint is queued for a human. The reviewer's reasoning is included.

### Why this fixes the human-gate latency problem

In kanbanzai, every spec and dev-plan needed a human gate by default. That was slow and humans were often the bottleneck. In Cromwell, most documents are approved by the agent-reviewer without humans being involved. **Humans are pulled in only when the reviewer escalates** — uncertainty, disagreement with the author beyond what comments can resolve, or domain decisions that genuinely need human judgement.

The escalation rate is itself a measurable signal. If reviewers escalate too often, the review prompt needs work or the document templates are unclear. If they escalate rarely, the system is working as intended.

---

## 10. Knowledge and Content

### One content store

As described in Section 6, all documents — long and short, human-written and AI-generated, code and prose — share one model: type, lifecycle, owner, path, indexed representation. Knowledge is not a separate subsystem.

### Indexing

When a document is saved (git commit or web UI edit), the orchestrator:

1. Parses the document into sections (heading-based)
2. Extracts metadata (front matter, tags)
3. Classifies sections by role (requirement, decision, rationale, constraint, example, etc.) using an agent classifier
4. Generates embeddings for sections and the document as a whole
5. Updates the Postgres index

This replaces the doc-intel subsystem in kanbanzai. There's no separate indexing tool, no separate API. Indexing is something the orchestrator does in response to document events.

### Retrieval

When the orchestrator assembles a prompt, it queries the content store for relevant material:

- Direct: documents attached to the current Feature
- Hierarchical: documents on ancestor Initiatives
- Tag-based: notes sharing tags with the current entity
- Semantic: notes whose embedding is close to the current spec/dev-plan

Retrieved material is injected into the prompt automatically, with provenance (e.g. "Note from auth/basic/notes/session-handling.md, approved 2026-04-12"). The agent does not need to call a `knowledge get` tool — the material is just there.

### Solving the kanbanzai knowledge problem

Kanbanzai's `knowledge` system had 86 contributions and 12 reads — written far more than retrieved. The root cause was that retrieval required the agent to explicitly call `knowledge get`, and they didn't remember to. Cromwell's auto-surfacing model means notes contributed to the project are actually used downstream, which restores the value of contributing them.

---

## 11. Observability and Cost

### Audit log

Every consequential event is recorded as a row in an append-only audit table:

- Document transitions (state changes, with author and timestamp)
- Entity transitions
- Dispatches (which agent, which role, which model)
- Gate evaluations (which gate, pass/fail, reason)
- Checkpoint creations and responses
- Budget warnings

This replaces kanbanzai's `.kbz/audit/events.jsonl`, which captured only 4 events over a 27-day period. In Cromwell, the audit log is automatic and complete because it's a side effect of every orchestrator action.

### Token and cost ledger

Every agent dispatch records:

- Model, input tokens, output tokens, cached tokens (where applicable)
- Wall time start/end
- Cost (computed from price table at dispatch time)
- Owning entity (Task, Feature, Initiative)

The ledger powers the cost rollups described in Section 5 and feeds the calibration corpus for future estimates.

### Tool call ledger

Alongside the dispatch ledger, every tool call within a dispatch is recorded: tool name, argument size, result size, latency, success or retry. This enables proxy-metric analysis — turns-to-completion per task shape, retry rates per tool, frequent tool sequences (`grep` repeatedly followed by `read_file` is a signal that a structural tool would shortcut the path).

The ledger does **not**, on its own, prove that any given tool "saves tokens" relative to an alternative. A ledger captures what happened, not the counterfactual. For causal claims about tool ROI, two complementary techniques apply:

- **Holdout cohorts** for production decisions. Disable a tool for a defined slice of dispatches and compare aggregate tokens-per-task, turn count, and success rate between cohorts. Needed when deciding whether to keep an expensive tool or promote an experimental one. Costs real tokens during the experiment and requires enough volume to be statistically meaningful.
- **Spot benchmarks** for engineering hygiene. Maintain a small set of representative tasks (find-definition, trace-callers, locate-implementation). Periodically run them with and without specific tools, publish the delta, watch for regressions when tool implementations change.

The most useful question the ledger answers is not "how much do graph tools save?" but "which tools are agents avoiding, and why?" If a tool is shipped and rarely chosen, the fix is usually a better role tool hint, not a different tool.

### Dashboard

The web UI surfaces:

- **Live queue:** what's in flight, what's queued, what's escalated
- **Recent activity:** event stream from the audit log
- **Cost burn:** running totals at any aggregation level, with budget warnings
- **Velocity:** features-per-week, tokens-per-feature, escalation rates
- **Calibration health:** how well recent actuals matched estimates

### Retrospective

Cromwell preserves kanbanzai's retrospective concept: agents and humans can flag observations as they go (workflow friction, tool gaps, things that worked well), and the system clusters them into themes that can be reviewed and acted on.

With the audit log, cost ledger, and calibration data feeding in, retrospectives become much richer than they were in kanbanzai. "Auth/basic shipped 30% under estimate" is a real, computable fact, not a feeling.

---

## 12. Bootstrapping

### Three clean compartments

| Compartment | Contains | How it changes |
|---|---|---|
| `.cromwell/` (project files) | Skills, roles, templates, schema overrides | User-edited; committed to git |
| Postgres | Entity state, document index, audit, ledger | Cromwell-edited; migrated by schema versions |
| Git repo (everything else) | Documents, code, tests | User-edited; committed normally |

These three sources of truth do not overlap. There is no Markdown in the database. There are no entity records in files. There is no state in skill configs.

### Init

`cromwell init` runs once per project. It:

1. Creates `.cromwell/` and copies the starter pack (default skills, roles, templates)
2. Configures the Postgres connection (or starts an embedded Postgres for local dev)
3. Runs schema migrations to create empty tables
4. Registers a git hook to watch document changes

After init, the project files are authoritative. Cromwell will never overwrite them.

### Upgrade

`cromwell upgrade` does not merge files. It surfaces diffs.

Three categories:

- **New defaults** — files in the new starter pack that don't exist in the project. Offer to copy in.
- **Modified defaults** — files where the shipped version has changed since the project's was copied. Show diff. Let user decide.
- **User-customised** — files the user has edited. Leave alone. Surface that an upstream version exists.

No automatic merge. No silent overwrites. This eliminates the entire class of "what got changed by the upgrade" confusion that plagued kanbanzai.

### Schema migrations

The only thing Cromwell modifies automatically on upgrade is the Postgres schema. Migrations are versioned and forward-only. The user-facing experience is one command: `cromwell upgrade`, and the database is brought to the new version.

### Skills and roles

Skills (`.cromwell/skills/*/SKILL.md`) and roles (`.cromwell/roles/*.yaml`) stay as files in the project. They were one of the better parts of kanbanzai and survive structurally unchanged. Some skills (notably the document-writing ones) will be revised based on lessons learned, but the mechanism is the same.

Stage-bindings (the YAML mapping of stages to roles and skills) does **not** survive. As described in Section 9, lifecycle and gates live on documents and Features; there's no separate stage system to bind.

---

## 13. Migration (sketch only)

Migrating from kanbanzai to Cromwell is desirable but not required. Cromwell is being designed as a clean break; if no migration path exists, projects can start fresh in Cromwell.

If migration is undertaken, the sketch is:

- Read `.kbz/state/` YAML, transform to Cromwell entity rows, insert into Postgres.
- Map kanbanzai's Strategic-plan → Initiative (top-level), Batch → discard (subsume into Initiative tree), Feature → Feature, Task → Task.
- Documents stay in place; register them in Cromwell's content store.
- Knowledge entries become `type: note` documents.
- History (audit, transitions) imported best-effort into Cromwell's audit log.

A one-shot importer tool would handle this. Detailed design out of scope for this document.

---

## 14. Open Questions

The following decisions are deliberately deferred to subsequent design documents:

### Web UI design
Layout, navigation, interaction patterns. Considered tractable; not blocking.

### Defect entity details
The name ("Defect" used here as a placeholder), the triage states, and how it differs structurally from Feature beyond the lifecycle states.

### Server language and runtime
Go vs Node vs other. Affects deployment story but not the model.

### Embedded vs hosted Postgres
For local single-developer use, an embedded Postgres (or even SQLite with periodic Postgres sync) might be appropriate. For teams, Supabase. The default is open.

### Multi-model price table format
How prices are configured, how often they're updated, whether historical prices are snapshotted at dispatch time for accurate retrospective costing.

### Specific skill revisions
The existing kanbanzai skills work but some — particularly the document-writing ones — would benefit from updates reflecting the validation-vs-review separation, the agent-reviewer mechanism, and the unified content model.

### Concurrency policies in detail
Per-Feature serialisation, per-provider rate limits, budget gate behaviours — the rough shape is decided; the specifics are config.

### Caching strategy detail
How prompt caching is exploited for cost savings (separate from how it's recorded for sizing).

### Migration tooling completeness
If migration from kanbanzai is undertaken, the importer needs its own design.

---

## 15. Summary

Cromwell is:

- A document-led planning and orchestration system for human-AI collaborative software development
- Vocabulary of nine work-and-document concepts and two tracking concepts, each meaning exactly one thing
- Strategic layer (nestable Initiatives with no lifecycle) above a tactical layer (Features with full lifecycle)
- Computed milestones and roadmaps that reflect reality without fudging
- Tokens as the unit of sizing and cost — honest, additive, model-agnostic
- Server + web UI + MCP facet, with Postgres for state and git for documents
- A code-based orchestrator that dispatches agents directly, not a chat tool
- Agent-reviewers for non-code artefacts, with humans pulled in only on escalation
- One unified content store for all documents (specs, designs, notes, etc.) with auto-surfaced retrieval
- Three clean configuration compartments with no merge confusion on upgrade

The intent is a system that is honest about progress, snappy in operation, cheap to extend, and resistant to the failure modes that bit kanbanzai.

---

*End of draft v1. Comments and revisions welcome.*
