# Phase-3 Session Record — Sizing, Calibration, and Milestones

**Status:** Implementation complete, tested, and CLI-smoke-verified; the
live-provider smoke (SPEC-003 DoD 2) is the remaining human step, specified in
§5 below.
**Date:** 2026-07-21
**Operator:** Sam (implementation by Claude on Sam's behalf)
**Test database:** dedicated dev Postgres (`cromwell-pg-dev`, port 54329)
**Provider (automated tests):** the scriptable `Mock` — the AI estimator runs
through the real dispatch loop against it.

This records the phase-3 build: the planning-and-tracking engine SPEC-003
scoped as the command centre's first slice. It measures and plans honestly —
tokens as the unit, worst-tier roll-ups, actuals-fed calibration, milestones
that lock without fudging — built entirely on the phase-1/2 ledger and the
DESIGN-001 schema, no new stack.

## 1. What the slice proves (SPEC-003 §1)

1. **Sizing is honest** — work is sized in tokens; every estimate carries a
   tier assigned from evidence; roll-ups propagate the **worst** tier in the
   tree; unestimated work shows as `?`, listed explicitly. A review-time
   decision settled the one genuine ambiguity in the spec: when a decomposed
   parent contains a `rough` child, the parent reads `rough` — "no estimate
   ever looks more confident than its weakest input" wins over "decomposition
   is a confidence gain" (recorded in `internal/sizing/sizing.go`).
2. **Actuals feed calibration** — completed work records actual token
   consumption from the dispatch ledger beside its estimate; the corpus of
   `(description, estimate, actual)` is a view over the ledger; the estimator
   retrieves nearest neighbours by Postgres full-text rank over descriptions
   (SD-1 — no embeddings, no changes to phase-1/2 tables).
3. **Milestones reflect reality without fudging** — a milestone references its
   members live (initiatives transitive, features, nested milestones); gate G4
   allows locking only once a member is done; locking snapshots the resolved
   leaves atomically; the only way to lock without something is to remove it
   first, and that descope is on the audit trail.

## 2. How it was built (build order, all green)

The pure core first, exactly as phases 1–2 began:

- **Migration `0004`** — `estimates`, `milestones`, `milestone_members`,
  `milestone_snapshots`, `roadmaps`, `roadmap_entries`, and the
  `estimate_tier`/`milestone_state` enums; no changes to phase-1/2 tables
  (NFR-3). Corpus full-text search runs at query time over
  `features.description`/`tasks.description`.
- **`internal/sizing`** — the pure roll-up + worst-tier engine, unit-tested
  without Postgres (FR-1, FR-2, NFR-1).
- **`internal/lifecycle` G4** — the pure milestone-lockable gate, alongside
  G1–G3/G5.
- **Store** — estimates (latest-is-current + history), the sizing-tree
  loaders, milestone resolution/progress/atomic lock-with-snapshot, roadmaps,
  actuals-by-ownership, the FTS corpus, and the extended cost roll-ups
  (per-initiative/feature/milestone/roadmap/month).
- **AI estimator** — an `estimate` dispatch purpose, the `estimator` role and
  `estimate-work` skill in the starter pack, the `submit_estimate` outcome
  tool, and the outcome routed through the pure rules engine to record the
  row. The tier is assigned by the system from corpus evidence, never by the
  agent (FR-1.2/FR-4.1).
- **CLI + HTTP** — `estimate` (roll-up / `set` / `ai`), `milestone`
  (create/add/remove/lock/list/show), `roadmap` (create/add/show), and
  extended `cost` (`--ref`, `--months`).

## 3. Automated verification

- `go vet ./...` clean; `go test -race ./...` green on the dev Postgres —
  every package including the phase-1/2 suites (no regressions).
- Pure unit tests encode the FR-1.2 / FR-2.1 acceptance criteria in
  `internal/sizing` and the G4 truth table in `internal/lifecycle`.
- Store integration tests cover latest-is-current estimates, the sizing
  tree roll-up and `?` listing, milestone resolve/progress/lock/descope,
  roadmap ordering, actuals-by-ownership, FTS corpus retrieval, and the
  extended cost roll-ups against real Postgres.
- Server integration tests (mock provider, real HTTP + real starter pack)
  drive: the worst-tier roll-up over the API; the **AI estimator through the
  real dispatch loop** producing a `considered` estimate when the corpus has a
  neighbour and a `rough` one when it is empty, each with a costed dispatch;
  milestone lock (G4 refusal → pass → snapshot → post-lock freeze); descope;
  roadmap ordering; and extended cost.

## 4. Real-binary CLI smoke (deterministic flows)

Beyond the tests, the built binary was driven end-to-end against a scratch
project on the dev Postgres — the actual `cromwell` CLI, server, socket, and
audit log. Transcript (abridged to the phase-3 surface):

```
$ cromwell estimate --ref auth/login
auth/login (feature): 3000 tokens [considered]

$ cromwell estimate --ref auth
auth (initiative): 8000 + ? tokens [rough, decomposed]
  unestimated:
    ? feature reset

$ cromwell milestone add v1 auth        # initiative → its features, transitively
$ cromwell milestone show v1
v1 [open]  0/3 done  $0.0000

$ cromwell milestone lock v1
refused by G4: no member done yet (3 resolved, none complete)
# … login marked done …
$ cromwell milestone lock v1
milestone "v1" locked — 1 of 3 resolved member(s) done
$ cromwell milestone show v1
v1 [locked]  1/3 done  $0.0000
$ cromwell milestone add v1 auth/logout
error: milestone is locked; membership is frozen at lock (FR-5.2)

$ cromwell roadmap show y2026
roadmap y2026:
  0. v1    [locked]
  1. v2    [open]

$ cromwell log --ref milestone --limit 2
… gate.evaluated   {"gate":"G4","pass":true,"reason":"1 of 3 resolved member(s) done"}
… milestone.locked {"leaves":3,"done_at_lock":1}
```

This confirms the worst-tier roll-up with the `?` listing, transitive
milestone membership, the G4 gate both ways, the atomic
`gate.evaluated`→`milestone.locked` audit pair (NFR-5), post-lock freeze, and
roadmap ordering — all through the real binary, no provider needed.

## 5. Live smoke to run (SPEC-003 DoD 2)

The one thing an automated/mock run cannot prove is a **real provider** pricing
a real estimate and the actual landing beside it. To close DoD 2, run against a
Supabase-hosted project with the DeepSeek gateway (as phase 2 did), and a human
inspects the ledger and calibration:

1. Scratch project → `cromwell init` (Supabase session-pooler URL; short
   `server.socket`, e.g. `/tmp/cromwell.sock`, for the macOS 104-byte limit).
   `config.yaml`: point `assignments.estimate` at `estimator` (shipped) and
   route it at `deepseek-v4-flash`.
2. Create a feature; `cromwell estimate ai <feature>` — the estimator prices
   it. With an empty corpus this lands `rough`.
3. Carry a small feature to `done` (the phase-2 loop), so its
   `(description, estimate, actual)` enters the corpus.
4. `cromwell estimate ai <similar-feature>` — confirm it now cites the
   completed one and lands `considered`; `cromwell estimate --ref <feature>`
   shows estimate vs actual and the delta.
5. Group the features into a milestone, lock after one ships, read the honest
   snapshot; inspect `cromwell cost --ref <milestone>` and the audit log.

Record the transcript, ledger, and calibration here to complete DoD 2/4, as
[walkthrough-phase-2.md](walkthrough-phase-2.md) did for phase 2.
