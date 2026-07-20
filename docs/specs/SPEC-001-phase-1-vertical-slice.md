# SPEC-001: Phase 1 — The Vertical Slice

**Status:** Approved and binding — 2026-07-20 ([REVIEW-001](../reviews/REVIEW-001-phase-1-package.md))
**Date:** 2026-07-02
**Parent design:** DESIGN-001, DESIGN-002, DESIGN-003, DESIGN-004
**Vision:** [vision-v1](../vision/vision-v1.md)

## 1. Goal

Prove Cromwell's three load-bearing claims end to end, in a real installation,
before any breadth is added:

1. **A code orchestrator** dispatches an agent-reviewer directly against the
   provider API — no chat intermediary (vision §8).
2. **Humans gate only on escalation** — a spec travels draft → reviewing →
   approved with no human in the loop unless the reviewer escalates (vision §9).
3. **The ledger is complete by construction** — every transition, gate
   evaluation, and dispatch is audited and costed from the first commit
   (vision §11).

Phase 1 is complete when a user can `cromwell init` a project, create an
initiative and a feature, write a spec, submit it, watch an agent-reviewer
approve it or escalate to a CLI inbox, and see the full audit trail and the
exact cost of the review.

## 2. Scope

### In scope

- `cromwell init`; embedded forward-only migrations; Supabase or plain-Postgres
  connection (DEC-002)
- Single `cromwell` binary: `serve` plus CLI subcommands (client → server over
  unix socket / HTTP)
- Entities: **initiatives** (nesting, archive), **features** (states `idea`,
  `ready`, `abandoned` only)
- Documents: registration, sectioning, FTS indexing, full lifecycle, comments
- Validation engine with template manifests for the `spec` type
- Agent-reviewer dispatch for specs via the Anthropic API, with the
  `submit_review` outcome tool and all three verdicts
- Revision flow for approved documents (supersession; the *in-flight blocking*
  half of DESIGN-003 §5.3 is N/A until phase 2's active features)
- Checkpoints with CLI inbox (`cromwell inbox`, `cromwell respond`)
- Audit log, dispatch/cost ledger with price snapshots, `cromwell status`
- Git watcher (post-commit hook → server) for document change detection,
  including approved-document integrity violations
- Gates **G1** (contract-approved: spec only — see D-1) and **G5**
  (initiative-archivable); governor checks: budget cap, provider rate limit
- Crash recovery: restart-safe dispatches, idempotent review dispatch

### Out of scope (deferred, with destination)

| Deferred | To |
|---|---|
| Web UI, MCP facet | Phase 3 |
| Tasks, dev-plans in G1, implementer dispatch, worktrees, tool host beyond outcome tools, verification (G3) | Phase 2 |
| Embeddings / semantic retrieval; section role classification | Phase 2 (FTS + direct attachment suffice for the slice) |
| Milestones, roadmaps, estimates, calibration | Phase 3 |
| Defects, jobs, checklists | Phase 3 |
| `cromwell upgrade`, kanbanzai migration importer | Later |

### Scope decisions

- **D-1: G1 is spec-only in phase 1.** DESIGN-003's G1 requires spec AND
  dev-plan. With dev-plans deferred, phase-1 G1 passes on spec approval alone,
  behind a single expression that phase 2 extends. The gate function, audit
  shape, and `idea → ready` mechanics are exercised for real.
- **D-2: One provider.** Anthropic only. The `provider.Provider` interface
  exists; a second implementation is deliberately not written until a real
  second provider is configured (interfaces designed against one consumer and
  one implementation are speculative).
- **D-3: No `cromwell upgrade`.** Nothing exists to upgrade from. `init` refuses
  to run twice.

## 3. Requirements

### FR-1: Installation

- **FR-1.1** `cromwell init` creates `.cromwell/` with the starter pack
  (config.yaml, `spec` template + validation manifest, `spec-reviewer` role,
  review skill — file formats and contents per DESIGN-004), configures the
  Postgres connection, applies all migrations,
  and installs the git post-commit hook.
  *AC:* running `init` in a fresh git repo, then `cromwell status`, reports a
  healthy install; all listed files exist; `schema_migrations` records the
  applied versions.
- **FR-1.2** `init` is refused when `.cromwell/` exists (message names the
  conflict). Migrations are transactional: a failing migration leaves no
  partial schema.
  *AC:* second `init` exits non-zero without modifying files or database; a
  deliberately broken migration in a test leaves `schema_migrations` and the
  schema at the prior version.
- **FR-1.3** Connection config supports a plain Postgres URL and a Supabase
  connection string equivalently; secrets come from environment variables.
  *AC:* the integration suite passes against both `postgres:16+pgvector` and a
  Supabase local stack; no secret material appears in `.cromwell/` or git.

### FR-2: Server and CLI

- **FR-2.1** `cromwell serve` runs the server: event bus, rule engine,
  lifecycle engine, dispatcher, heartbeat (30s), HTTP API on a unix socket
  (TCP configurable).
  *AC:* `serve` starts against an initialised project; `status` over the socket
  returns server health, queue depth, and pending checkpoint count.
- **FR-2.2** CLI subcommands are API clients only; apart from `init`'s
  migration run (DESIGN-002 §3), the CLI never connects to Postgres
  (DESIGN-002 O-1).
  *AC:* code review + a test asserting the CLI client packages import no
  `store`/pgx packages; CLI against a stopped server fails with a clear
  "server not running" message.

### FR-3: Entities

- **FR-3.1** Create, rename, nest, and archive initiatives; archiving honours
  gate G5 with the checkpoint-override path.
  *AC:* archiving an initiative whose subtree contains a non-terminal feature
  is refused and raises a `gate-override` checkpoint; answering it "override"
  with a reason archives and audits both.
- **FR-3.2** Create features under initiatives (`idea`); abandon with reason.
  `idea → ready` fires automatically when G1 passes; no CLI command can force
  it.
  *AC:* on spec approval, the feature is `ready` without further input, and an
  audit row `gate.evaluated {gate: G1, pass: true}` precedes the transition
  row.

### FR-4: Documents

- **FR-4.1** `cromwell doc add <path> --type spec --owner <feature>` registers
  a document; the indexer parses front matter and sections and populates FTS.
  *AC:* after registration, `cromwell search <term>` finds the document by
  section content.
- **FR-4.2** A git commit touching a registered document re-indexes it
  (watcher → event → index pass); `content_hash` and `indexed_at` update.
  *AC:* commit an edit, observe re-index within 5s of the hook firing; an
  edit committed while the server is stopped is re-indexed by the boot
  catch-up scan (DESIGN-002 §8) on the next start.
- **FR-4.3** A commit modifying an **approved** document's file raises a
  `document-integrity` checkpoint (DESIGN-003 §2).
  *AC:* editing an approved spec's file and committing produces the checkpoint
  with the offending commit hash in its context; the same checkpoint is raised
  by the boot catch-up scan when the edit was committed while the server was
  stopped.
- **FR-4.4** `cromwell revise <doc>` creates a successor draft with
  `supersedes_id` set; the successor's approval supersedes the predecessor
  atomically and re-points the canonical path.
  *AC:* after successor approval, exactly one non-superseded spec is owned by
  the feature; predecessor state is `superseded`; both transitions share a
  transaction (asserted via audit timestamps and a crash-injection test).

### FR-5: Lifecycle, validation, review

- **FR-5.1** The lifecycle engine implements DESIGN-003 §2 exactly: the six
  events, the four states, typed rejection of illegal transitions, audit rows
  in the same transaction as every transition. No force flag exists.
  *AC:* table-driven unit tests cover the full transition matrix (legal and
  illegal); grep-level check: no code path mutates `documents.state` outside
  the lifecycle engine.
- **FR-5.2** `cromwell submit <doc>` runs validation synchronously; failures
  block the transition and print a structured report; passes transition to
  `reviewing` and queue a reviewer dispatch. `cromwell validate` runs the same
  checks without submitting.
  *AC:* a spec missing its acceptance-criteria section is rejected with the
  section named; the same spec with the section added submits and produces a
  `queued` dispatch row.
- **FR-5.3** The reviewer dispatch assembles the prompt per DESIGN-002 §4
  (role, skill, owning feature context, ancestor-initiative documents by direct
  attachment, validation report, unresolved comments), calls the Anthropic
  API, and completes only via the `submit_review` outcome tool.
  *AC:* against a mock provider, the assembled prompt contains each required
  block in the deterministic order; a response with no outcome-tool call is
  treated as a failed attempt and retried.
- **FR-5.4** All three verdicts work end to end: `approve` → document
  `approved`, G1 re-evaluated; `request_changes` → document `draft`, comments
  inserted and shown by `cromwell doc comments`; `escalate` → checkpoint with
  reasoning, document stays `reviewing`, human answer maps to
  approve/request_changes.
  *AC:* one integration test per verdict, mock provider; one live smoke test
  (real API, tiny spec) executed before release, its cost visible in the
  ledger.
- **FR-5.5** Re-review after `request_changes` includes the unresolved comment
  thread in the prompt.
  *AC:* second-round prompt contains first-round comments.

### FR-6: Checkpoints

- **FR-6.1** `cromwell inbox` lists pending checkpoints (kind, question,
  context, age); `cromwell respond <id> <answer> [--reason]` records the
  response and wakes the rule engine.
  *AC:* an escalated review answered via `respond` advances the document
  within one event cycle; responder identity and reason are on the checkpoint
  row and in the audit log.
- **FR-6.2** Checkpoint creation is idempotent per (kind, ref) while pending.
  *AC:* replaying the triggering event does not create a second pending
  checkpoint.

### FR-7: Observability

- **FR-7.1** Every transition, gate evaluation, dispatch state change,
  checkpoint creation/response, and budget event writes an `audit_events` row
  in the same transaction as its effect. `cromwell log [--ref <entity>]` tails
  and filters the audit stream.
  *AC:* the FR-5.4 approve-path integration test asserts the complete expected
  audit sequence — submit, validation, dispatch queued/running/succeeded, gate
  G1, both transitions — with no gaps.
- **FR-7.2** Dispatches record model, token counts (including cache
  read/write), wall time, price snapshot, and computed cost. `cromwell cost
  [--ref <entity>]` rolls up cost by entity, feature, initiative, and total.
  *AC:* after the live smoke test, `cromwell cost` shows a non-zero cost for
  the review matching the snapshot prices × reported tokens.
- **FR-7.3** The governor enforces the project budget cap and per-provider
  rate limit; the hard cap queues dispatches and raises a `budget` checkpoint.
  *AC:* with the cap set below one review's projected cost, submission leaves
  the dispatch `queued` with reason `budget` and one pending checkpoint;
  raising the cap and responding releases it.

### FR-8: Resilience

- **FR-8.1** Restart-safe: killing the server mid-dispatch and restarting
  re-queues the stalled dispatch (heartbeat threshold) and completes the
  review; the idempotency key prevents a duplicate concurrent dispatch.
  *AC:* crash-injection integration test passes; final state shows one
  succeeded dispatch for that (document, content_hash), any stalled attempt
  marked failed-stalled.
- **FR-8.2** Provider 429/5xx are retried with backoff inside the dispatch;
  exhausted retries produce a `dispatch-failure` checkpoint, not a silent stall.
  *AC:* mock provider forced to fail produces the checkpoint after the
  configured attempts, with the error chain in its context.

## 4. Non-functional requirements

- **NFR-1** `submit` (validation + transition + dispatch enqueue) returns in
  < 1s; queued reviews start within one governor cycle (< 2s) when unblocked.
- **NFR-2** Rule engine and lifecycle engine are pure and fully unit-testable
  without Postgres, provider, or clock (DESIGN-002 §9); CI runs `go test
  -race ./...` plus the integration suite against plain Postgres (DEC-002).
- **NFR-3** Migrations in phase 1 create only the tables phase 1 uses
  (DESIGN-001 §11): entities (initiatives, features), documents +
  sections + comments, audit, dispatches + tool_calls, checkpoints,
  schema_migrations. No pgvector dependency yet (embedding column arrives with
  its phase-2 migration).
- **NFR-4** No secrets in git, in `.cromwell/`, in the audit log, or in error
  messages (API keys via environment only).

## 5. Definition of Done

1. All FR acceptance criteria pass in CI (mock provider) on plain Postgres and
   in the Supabase local stack.
2. The live smoke test — init, initiative, feature, spec, submit, real
   Anthropic review, approve or escalate+respond, feature `ready` — runs
   green against a Supabase-hosted project, and its full audit trail and cost
   are inspected by a human.
3. `go vet ./...` and `go test -race ./...` clean.
4. A `docs/walkthrough.md` records the smoke test session: every command, the
   inbox interaction, the audit log, the cost report.
5. Phase-2 entry criteria drafted: tasks + dev-plans + G1 extension,
   implementer dispatch + tool host, verification gate G3.
