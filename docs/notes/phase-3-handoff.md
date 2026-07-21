# Phase-3 Handoff — Where We Are, What's Next

**Date:** 2026-07-21
**Purpose:** Orient a fresh context to begin phase-3 (SPEC-003) implementation
without re-deriving what phases 1–2 established.

---

## 1. What Cromwell is (one paragraph)

Cromwell is a document-led, specification-centred workflow system that takes a
software project from concept to shipped features — humans drive planning, AI
agents drive development. A **code orchestrator** (not a chat agent) watches
events and dispatches agents directly to provider APIs. Humans gate **only on
escalation**. Every action is **audited and costed by construction**. State
lives in Postgres, documents live in git, config lives in `.cromwell/`. See
[README](../../README.md) and [vision-v1](../vision/vision-v1.md).

## 2. Status

**Phases 1 and 2 are delivered** — built, tested (`go test -race ./...` green
on plain Postgres and the Supabase local stack), and each proven with a live
smoke test against a real provider.

- **Phase 1** ([SPEC-001](../specs/SPEC-001-phase-1-vertical-slice.md)):
  `init`, server + CLI, initiatives/features, the document lifecycle with
  validation and a real agent-reviewer, the escalation inbox, the complete
  audit + cost ledger. Live proof: [walkthrough](../walkthrough.md).
- **Phase 2** ([SPEC-002](../specs/SPEC-002-phase-2-implementation-loop.md)):
  dev-plans with validated task tables, decomposition, the two-part contract
  gate (G1), the **tool host** (jailed paths, hash-anchored edits, whitelisted
  commands, per-role profiles), implementer dispatch in per-feature worktrees,
  code review of diffs, close-out verification (G3), and merge. Live proof:
  [walkthrough-phase-2](../walkthrough-phase-2.md) — a task carried from
  approved contract to merged code by agents, $0.0043.

**Phase 3 is approved and next.**
[SPEC-003](../specs/SPEC-003-phase-3-planning-and-tracking.md) — the
planning-and-tracking slice — is **approved and binding**
([REVIEW-003](../reviews/REVIEW-003-phase-3-planning-package.md), Sam approved
2026-07-21, as scoped, no DESIGN-007). **Implementation has not started.**

## 3. The codebase (Go, single binary)

`cmd/cromwell` is the one binary: `serve` runs the server; every other
subcommand is an HTTP-over-unix-socket client (except `init`, which applies
migrations directly). Packages under `internal/`:

| Package | Owns |
|---|---|
| `lifecycle` | **Pure** state machines (document/feature/task), gates G1/G2/G3/G5, validation, dev-plan parsing/decomposition. No I/O — fully unit-tested. |
| `rules` | **Pure** decision engine: `Decide(event, snapshot) → []Action`. No I/O. |
| `bus` | Typed in-process events. |
| `store` | pgx repositories; every state change writes its audit row in the **same transaction** (O-3). Migrations embedded via `embed.FS`. |
| `config` | `.cromwell/` compartment loader (config.yaml, roles, skills, manifests, pack.lock); strict parsing; tool-profile validation. |
| `content` | Markdown parse/section/hash; prompt assembly. |
| `provider` | `Provider` interface; `anthropic` impl (also drives DeepSeek via `base_url`); scriptable `Mock`. |
| `dispatch` | The generalised agent loop, governor (budget/rate/serialisation/workers), cost, retries, tool-call ledger. |
| `toolhost` | **Pure** tool-host logic: path jailing, hash anchoring, command whitelist, profiles. |
| `server` | Wires it together: orchestrator loop, HTTP API, planners (prompt+tools per purpose), tool executor, action executors, worktree ops, git watcher. |
| `starter` | Embedded starter pack + `init`. |
| `client` | CLI HTTP client (import-boundary tested: no store/pgx). |

**The load-bearing pattern:** an event enters the bus → the orchestrator loads
a minimal `Snapshot` → `rules.Decide` returns coarse `Action`s → the server's
action executors run them, each transactional with its audit rows, publishing
follow-up events. Pure logic (lifecycle, rules, toolhost) is tested without
Postgres or a provider; the server integration suite drives the whole thing
with a mock provider against real Postgres and real git worktrees.

## 4. Conventions that matter (learned, some the hard way)

- **Audit in the same transaction** as the state change it records (O-3).
- **Commit-then-git ordering**: DB transaction commits first, then the git
  operation (worktree add, task commit, merge); a git failure raises a
  checkpoint, never a half-state.
- **Idempotency keys** on every dispatch (`review:…`, `implement:…:<n>`,
  `review-code:…:<commit>`, `verify:…:<head>`) via a partial unique index over
  live states, so replay/crash never double-dispatches.
- **Kick discipline**: after enqueuing a dispatch inside a transaction, call
  `Dispatcher.Kick()` *after* the commit. The kick uses a generation counter
  so a kick issued after a commit can never be lost to coalescing (a real bug
  we fixed — without it, dispatches sat until the heartbeat).
- **Answer every tool_use block** in a turn — providers return parallel tool
  calls (a real bug from the live smoke test; regression: `TestParallelToolCalls`).
- **No force flags anywhere**: overrides happen only through answered
  checkpoints (L-6).
- **House doc style**: prose, complete sentences, decision records (D-x/SD-x
  in specs, O-x/L-x/F-x/etc. in designs); approved docs are revised via a
  visible logged edit (a §Revisions entry), never silently. Authority runs
  spec → design → decision.
- **The approval gate is real**: author a planning package → do an authoring
  consistency review → **the human approves before any code**. I never
  self-approve a package I wrote. This is how all three phases started.

## 5. What phase 3 is (SPEC-003), in suggested build order

The planning slice proves Cromwell **measures and plans honestly**: tokens as
the unit, confidence tiers with worst-tier roll-up, actuals-fed calibration,
milestones with honest locking, roadmaps. It builds entirely on the **complete
ledger** and DESIGN-001's existing estimate/milestone schema — no new stack.
Read [SPEC-003](../specs/SPEC-003-phase-3-planning-and-tracking.md) in full;
the FRs, in a sensible dependency order:

1. **Migration `0004` + store** — `estimates`, `milestones`,
   `milestone_members`, `milestone_snapshots`, `roadmaps`, `roadmap_entries`.
   Create the `estimate_tier` and `milestone_state` enums here (they were
   defined in DESIGN-001 §3 but not shipped — same as `task_state` in `0002`).
   Add FTS over `features.description`/`tasks.description` for corpus
   retrieval. (NFR-3; REVIEW-003 §4.)
2. **Pure roll-up + tier logic** (`lifecycle` or a new `sizing` package):
   worst-tier propagation, `?` for unestimated, decomposed-sum arithmetic —
   pure, unit-tested (FR-1, FR-2, NFR-1).
3. **Actuals + calibration corpus** — sum actuals from the dispatch ledger by
   owner/subtree; the corpus view; FTS retrieval for the `considered` tier
   (FR-3, SD-1).
4. **AI estimator** — the `estimate` dispatch purpose + `estimator` role +
   `submit_estimate` outcome tool + `estimate-work` skill in the starter pack
   (FR-4; reuse the phase-2 dispatch/planner pattern).
5. **Milestones + G4** — live membership resolution (initiative transitive /
   feature / nested milestone), progress, lock-with-snapshot, descope-before-
   lock; G4 gate function (FR-5). Milestone/roadmap resolution logic is pure
   and unit-testable.
6. **Roadmaps** (FR-6) and **extended cost roll-ups** (FR-7:
   per-initiative/milestone/roadmap/month).
7. **CLI**: `estimate`, `milestone`, `roadmap`, `cost` (extended).
8. **Integration suite** (mock provider) + a **live smoke test** and its
   walkthrough (DoD 2/4), then draft the **web-UI (SPEC-004) entry criteria**
   (DoD 5).

Scope decisions already made (SPEC-003 §2): web UI → SPEC-004, MCP facet →
SPEC-005, defects/checklists/jobs → later, embeddings still deferred
(considered-tier uses FTS, SD-1), milestone members exclude checklists (SD-2).

## 6. Environment and how to work

- **Build**: `go build ./cmd/cromwell`. **Vet+test**:
  `CROMWELL_TEST_DATABASE_URL=<plain-postgres-url> go test -race ./...`.
- **Dedicated test Postgres**: a `postgres:16` container `cromwell-pg-dev` is
  running on **port 54329** (`postgres://postgres:cromwell@localhost:54329/…`).
  The suite creates isolated per-package test databases on it; do not point it
  at a real project. (`internal/testdb` handles this.)
- **CI**: `.github/workflows/ci.yml` runs vet + `go test -race` against a
  Postgres service on every push.
- **Live smoke tests** (DoD 2) need real credentials in the environment:
  `CROMWELL_DATABASE_URL` (a Supabase-hosted project) and a provider key.
  Phase 2 used a **DeepSeek** key via its Anthropic-compatible gateway
  (`base_url: https://api.deepseek.com/anthropic`), which is cheap and
  billing-capped — point all roles at `deepseek-v4-flash` in a scratch
  project's `config.yaml`.
- **Gotchas** (both cost us a round in live tests): Supabase **direct**
  connections are IPv6-only — use the **session pooler**
  (`aws-0-<region>.pooler.supabase.com:5432`, session mode for pgx). And on
  macOS, unix socket paths cap at ~104 bytes — set a short
  `server.socket` (e.g. `/tmp/cromwell.sock`) for deep scratch paths.
- **Scratchpad**: keep throwaway projects/scripts in the session scratchpad,
  not the repo.

## 7. Starting point for the phase-3 session

The disciplined loop restarts here: SPEC-003 is approved, so implementation
may begin at §5 step 1 (migration 0004 + store). Suggested first move: read
SPEC-003 and DESIGN-001 §6–7, then scaffold migration 0004 and the pure
roll-up logic with tests, exactly as phases 1–2 began with their pure cores.

Git is clean; the latest commits are the phase-2 delivery and the SPEC-003
approval. `git log --oneline` shows the trail.
