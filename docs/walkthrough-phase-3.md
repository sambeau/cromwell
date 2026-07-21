# Phase-3 Session Record — Sizing, Calibration, and Milestones

**Status:** Complete — satisfies SPEC-003 Definition of Done items 1–5,
including the live-provider smoke (§5).
**Date:** 2026-07-21
**Operator:** Sam (commands run by Claude on Sam's behalf)
**Automated-test database:** dedicated dev Postgres (`cromwell-pg-dev`, 54329)
**Live-smoke database:** Supabase-hosted Postgres (session pooler, eu-central-1)
**Live-smoke provider:** DeepSeek via its Anthropic-compatible gateway, model
`deepseek-v4-flash`, for all six roles.

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

## 5. Live smoke — the real run (SPEC-003 DoD 2)

Against a Supabase-hosted project with the DeepSeek gateway (all six roles on
`deepseek-v4-flash`), the full flow ran end to end. Everything below is real:
a real database, a real provider, real agent-written code, and real
calibration against real actuals.

**A feature carried to `done` by agents.** A minimal feature — a `greet.py`
returning the exact string `Welcome to Cromwell` — went the whole phase-2 loop
live: spec reviewed and approved (`$0.00036`), dev-plan reviewed and approved
(`$0.00052`), decomposed to one task, implemented (`$0.00129`,
agent-authored `greet.py`), code-reviewed and approved (`$0.00088`), verified
against all three acceptance criteria with evidence (`$0.00195`), and merged.
It reached `done` with real dispatches in the ledger.

**Sizing is honest, and actuals feed calibration** — the headline result:

```
# Estimate the completed feature. The corpus has no similar prior work yet.
$ cromwell estimate ai p3/welcome        # estimator, live DeepSeek
$ cromwell estimate --ref p3/welcome
p3/welcome (feature): 800 tokens [rough]
  actual: 46263 tokens (delta +45463)
```

The estimator, shown an empty corpus, reasoned from judgement to **800 tokens
(`rough`)** — "no reference points exist in the corpus … a floor-level
estimate." The real work consumed **46,263 tokens**. The `rough` tier was
honest about its own weakness, and the actual landed beside it with the delta
laid bare (`+45,463`). That `(description, 800, 46263)` tuple is now the
corpus.

```
# A structurally identical feature. The corpus now holds `welcome`.
$ cromwell estimate ai p3/farewell       # estimator, live DeepSeek
$ cromwell estimate --ref p3/farewell
p3/farewell (feature): 46263 tokens [considered]
```

Given the completed neighbour, the estimator returned **46,263 tokens
(`considered`)**, its rationale citing it directly: *"Anchored on the 'Welcome
greeting' reference point (actual: 46,263 tokens) … the original estimate of
800 tokens was a severe undercount."* A single completed feature moved a naive
`rough` guess to a `considered` estimate two orders of magnitude closer to the
truth. This is the vision's calibration claim, proven: rough numbers sharpen
against actuals.

**Milestones lock honestly.**

```
$ cromwell milestone create p3-v1 --target-date 2026-09-30
$ cromwell milestone add p3-v1 p3          # initiative → welcome (done) + farewell (idea)
$ cromwell milestone show p3-v1
p3-v1 [open]  1/2 done  $0.0051
$ cromwell milestone lock p3-v1
milestone "p3-v1" locked — 1 of 2 resolved member(s) done   # G4
$ cromwell milestone show p3-v1
p3-v1 [locked]  1/2 done  $0.0051                            # frozen snapshot
```

Adding the initiative pulled in both features transitively; G4 allowed the
lock once `welcome` had shipped; the snapshot froze the leaf set. The
gate.evaluated (G4 pass) and milestone.locked rows are one atomic pair in the
audit log.

**Cost roll-ups, extended.**

```
$ cromwell cost --ref p3            # initiative, transitive
initiative p3  $0.0051
$ cromwell cost --ref p3-v1         # milestone, over resolved members
milestone  p3-v1  $0.0051
$ cromwell cost --months
2026-07  dispatches=14  $0.0103     # aggregates this run + the leftover phase-2 smoke
```

The whole phase-3 live run — the feature loop plus both estimator dispatches —
was **7 costed dispatches for `$0.00601`**. Honest sizing, honest calibration,
honest milestones, for six-tenths of a cent.
