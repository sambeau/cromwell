# DESIGN-002: The Orchestrator

**Status:** Draft for review
**Date:** 2026-07-02
**Parent:** [vision-v1](../vision/vision-v1.md) §7 (Architecture), §8 (Orchestrator), §11 (Observability)
**Depends on:** DESIGN-001 (schema), DEC-001 (Go), DEC-002 (Postgres/Supabase)

## 1. Purpose

Defines the server-side orchestrator: the event model, the dispatch pipeline, the
tool host, concurrency and budget governance, failure handling, and crash
recovery. The orchestrator is plain Go code executing workflow rules — it has no
model attached and cannot be talked into anything (vision §8). Its honesty
properties are structural: gates are functions, overrides require a human
checkpoint, and every action writes audit rows in the same transaction as its
effects.

## 2. Component map

```
┌──────────────────────────── cromwell server ────────────────────────────┐
│                                                                          │
│  event sources                 core                        effectors     │
│  ┌─────────────┐        ┌──────────────┐          ┌──────────────────┐  │
│  │ git watcher │──┐     │              │          │ dispatcher       │  │
│  ├─────────────┤  │     │  event bus   │          │  (worker pool)   │  │
│  │ pg LISTEN   │──┼────▶│      +       │─────────▶├──────────────────┤  │
│  ├─────────────┤  │     │  rule engine │          │ tool host        │  │
│  │ HTTP API    │──┤     │              │          ├──────────────────┤  │
│  ├─────────────┤  │     └──────┬───────┘          │ provider clients │  │
│  │ heartbeat   │──┘            │                  └──────────────────┘  │
│  └─────────────┘               ▼                                        │
│                     lifecycle engine (DESIGN-003)                       │
│                     store (pgx) ── audit ── ledger                      │
└──────────────────────────────────────────────────────────────────────────┘
```

- **Event bus** — in-process, typed events; every source normalises into it.
- **Rule engine** — pure functions: `(event, current state) → []Action`.
- **Lifecycle engine** — the single component allowed to change entity/document
  state (DESIGN-003 owns its rules).
- **Dispatcher** — turns `DispatchAgent` actions into provider API calls.
- **Tool host** — executes tool calls made by dispatched agents.

## 3. Event model

### Sources

| Source | Mechanism | Events |
|---|---|---|
| Git watcher | post-commit hook pings server HTTP endpoint; watcher diffs the commit for document paths; a boot-time catch-up scan covers commits made while the server was down (§8) | `DocumentFileChanged` |
| Postgres | `LISTEN cromwell_events` | mirror of state transitions (safety net; primary is in-process) |
| HTTP API | CLI and (later) web UI calls | `SubmitDocument`, `CheckpointResponded`, `ManualGo`, entity CRUD |
| Heartbeat | ticker, every 30s | `Tick` |

### Single-writer rule

All state mutation flows through the server process. The CLI never opens a
Postgres connection; it calls the server's HTTP API (unix socket locally). This
makes the in-process event bus complete — nothing changes state behind the
server's back except git commits, which the watcher covers. LISTEN/NOTIFY
remains wired (DESIGN-001 §10) so a future multi-process deployment changes
topology, not contracts.

### Heartbeat duties (vision §8)

On each `Tick`:

1. **Stall detection** — `dispatches` rows in `running` whose `heartbeat_at` is
   older than the stall threshold are marked `failed` (reason: stalled) and fed
   to the retry policy.
2. **Retry sweep** — failed dispatches with remaining attempts are re-queued
   with exponential backoff.
3. **Garbage collection** — worktrees for features in terminal states are
   removed (and their graph projects torn down); checkpoints past their TTL are
   marked `expired` and re-surfaced.
4. **Reconciliation** — recompute gates for entities whose gate inputs changed
   while the server was down (crash recovery, §8).

## 4. Dispatch pipeline

A `DispatchAgent` action runs through eight steps (vision §8, made concrete):

1. **Claim.** Insert a `dispatches` row (`queued`), then transition to `running`
   under the concurrency governor's approval (§6).
2. **Resolve role & skill.** Load role YAML from `.cromwell/roles/<role>.yaml`
   and skill from `.cromwell/skills/<skill>/SKILL.md`. Files are read fresh per
   dispatch — config edits take effect immediately, no server restart.
3. **Assemble prompt.** Deterministic assembly order (stable prefixes maximise
   provider prompt caching):
   `role identity → skill procedure → project context → auto-surfaced documents
   (with provenance lines) → entity context → task instruction`.
   Auto-surfacing queries the content store: direct attachments, ancestor-
   initiative docs, tag overlap, embedding similarity (vision §10) — each tier
   subject to a token budget with direct attachments never trimmed.
4. **Select model.** From role config, overridable per-purpose in
   `.cromwell/config.yaml` (e.g. first-pass reviews on a cheaper model).
5. **Call the provider.** Native API (Anthropic Go SDK first; provider interface
   for others). Streaming; agent turns loop through the tool host until the
   agent calls its **outcome tool** (§5) or hits the turn/token cap.
6. **Record consumption.** Token counts from the provider response, cost from
   the `price_snapshot` frozen at claim time, written to the dispatch row.
   `heartbeat_at` updated every turn.
7. **Process the outcome.** The structured outcome (a typed payload, never
   parsed prose) is handed to the rule engine, which emits follow-up actions —
   lifecycle transitions, comment insertion, checkpoint creation, next dispatch.
8. **Audit.** Steps 1, 5, 7 each write `audit_events` rows in the same
   transaction as their state effects.

### Structured outcomes

Every dispatched agent is given exactly one **outcome tool** matching its
purpose — e.g. a reviewer gets `submit_review(verdict, comments[], reasoning)`
with `verdict ∈ {approve, request_changes, escalate}`. The dispatch is complete
only when the outcome tool is called. Free-text final messages are recorded but
carry no workflow effect. This is the mechanism that makes agent output
machine-processable without prose parsing.

## 5. Tool host

Implements vision §8's tool surface.

- **Registry.** Tools are Go implementations registered by name:
  `read_file`, `edit_file`, `search_graph`, `run_tests`, `record_finding`,
  `update_task_status`, plus per-purpose outcome tools. A tool's body may call
  local code, the GitHub API, or an external MCP server (the server is the MCP
  *client* — agents never speak MCP).
- **Per-role profiles.** Role YAML declares `tools: [...]`. The dispatcher hands
  the provider only those definitions; the host additionally rejects any call
  outside the profile (defence in depth against model confusion).
- **Implicit context injection.** Each dispatch carries a `ToolContext`
  (worktree root, graph project, feature/task IDs) supplied by the orchestrator.
  Path arguments resolve relative to the worktree root and are **jail-checked**:
  a resolved path outside the worktree is an error, not an escalation. Graph
  queries are scoped to the worktree's graph project automatically.
- **Hash-anchored edits.** `read_file(hash_tag: true)` returns `line#hash |
  content`; `edit_file(hash_ref, new_text)` fails if the anchor drifted. Role
  tool-hints steer agents to this path; plain writes remain for new files.
- **Command whitelist.** No generic shell. `run_tests` and friends execute fixed
  argv templates from project config, in the worktree, with a timeout and output
  cap. Anything else requires escalation.
- **Ledger.** Every call writes a `tool_calls` row: name, sizes, latency,
  status (vision §11).

## 6. Concurrency and budget governance

A single governor approves the `queued → running` transition. Checks, in order:

1. **Budget** — projected cost (per-dispatch cap) added to the running
   project-period total must stay under the configured budget; at the warning
   threshold an audit event fires, at the hard cap dispatches queue and a
   `budget` checkpoint is raised.
2. **Provider rate limit** — token-bucket per provider.
3. **Feature serialisation** — at most one *mutating* dispatch per feature at a
   time (implementers, doc authors). Read-only dispatches (reviews, estimates)
   are exempt and run in parallel.
4. **Global worker cap** — bounded worker pool.

Each check either admits, or leaves the dispatch queued with a reason
(visible in `cromwell status`). Nothing is dropped.

## 7. Escalation

`Checkpoint` rows (DESIGN-001 §9) are created by: reviewer `escalate` verdicts,
exhausted retries, budget caps, gate-override requests, and always-human
operations (e.g. archiving an initiative with active features — vision §8).
Checkpoint creation is an ordinary action: audit-logged, idempotent per
(kind, ref) while pending. A `CheckpointResponded` event re-enters the rule
engine with the human's response attached; the rule that raised it defines the
resume behaviour.

## 8. Failure handling and crash recovery

- **Transient provider errors** (429/5xx/network): retry with exponential
  backoff within the dispatch, up to a turn budget.
- **Failed dispatches** (agent looped, invalid outcome, stall): retry policy per
  purpose (default: 2 retries, fresh context each attempt, `attempt`
  incremented). Exhaustion → `dispatch-failure` checkpoint.
- **Crash recovery:** all durable state is in Postgres; the server is
  stateless-restartable. On boot: re-queue `running` dispatches without recent
  heartbeats, replay pending checkpoints into the inbox, run the document
  catch-up scan (re-hash every registered document's file; drift from
  `content_hash` triggers a re-index, drift on an *approved* document raises
  the `document-integrity` checkpoint — this covers commits made while the
  server was down), run a full gate reconciliation pass, resume. In-flight provider calls at crash time are lost
  (their partial cost is unrecorded — accepted; the retry's cost is recorded).
- **Idempotency:** rule-engine actions carry deterministic idempotency keys
  (e.g. `review:<document_id>:<content_hash>`) so replay after crash or
  duplicate events cannot double-dispatch.

## 9. Package sketch

```
cromwell/
  cmd/cromwell/          # single binary: server + CLI subcommands
  internal/
    server/              # HTTP API, unix socket, lifecycle wiring
    bus/                 # event types, in-process bus, pg LISTEN bridge
    rules/               # rule engine: event → actions (pure, table-tested)
    lifecycle/           # DESIGN-003: transitions, gates, validation
    dispatch/            # pipeline, governor, retry policy
    toolhost/            # registry, profiles, context injection, jail
    provider/            # Provider interface; anthropic/, openai/
    store/               # pgx repositories, migrations (embed.FS)
    content/             # indexing, sectioning, retrieval, prompt assembly
    gitwatch/            # hook endpoint + commit diffing
    config/              # .cromwell/ loading: roles, skills, config.yaml
```

The rule engine and lifecycle engine are pure and synchronous — the entire
workflow logic is unit-testable with no Postgres, no provider, no clock.

## 10. Decisions recorded here

| # | Decision | Rationale |
|---|---|---|
| O-1 | Single-writer server; CLI via HTTP API, never direct DB | Completeness of the event stream; one enforcement point for gates |
| O-2 | Outcome tools, not parsed prose | Machine-processable agent results; no regex-on-prose fragility |
| O-3 | Audit rows share the transaction with their state change | Vision §11's "complete because it's a side effect" made literal |
| O-4 | Price snapshot frozen at dispatch claim | Exact retrospective costing (closes part of vision §14) |
| O-5 | Read-only dispatches exempt from feature serialisation | Vision §8 allows parallel reviews; only mutation needs the lock |
| O-6 | Config read fresh per dispatch | Editing roles/skills is the tuning loop; restarts would add friction |
