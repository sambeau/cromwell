---
title: "Spikes, stage 1 — dev plan"
type: dev_plan
owner: "M14"
---

# Spikes, stage 1 — dev plan

## Approach

This is the development plan for [SPEC-021](../specs/SPEC-021-spikes.md)
stage 1, in Subutai's own dev-plan shape, as the
[orchestration note](orchestration-M13-M14.md) asks. The lead orchestrates;
each task is built by a Sonnet implementer in its own git worktree, and the
lead merges each one when it is done.

The work falls into three waves. The first three tasks touch disjoint
packages, so they run together. The service layer needs all three. The web UI
and MCP then build on the service layer in parallel.

1. **Foundations, in parallel:** the migration and store (T1), the budget in
   the dispatcher (T2), and the starter pack and configuration (T3).
2. **The service layer** (T4): creating, starting, the run's plan and tools,
   ending, discarding, closing, the rules, the sweeps, and their integration
   tests.
3. **Surfaces, in parallel:** the web UI and timeline (T5), and MCP (T6).

Every task runs `go vet ./...`, `gofmt -l` on the files it touched, and the
tests of the packages it touched, with `SUBUTAI_TEST_DATABASE_URL` set so the
integration tests run.

## Tasks

| id | title | depends_on | description |
|----|-------|------------|-------------|
| T1 | Migration 0015 and the spike store | | `internal/store/migrations/0015_spikes.sql`, `internal/store/spikes.go` (+ tests), `internal/ident` (Kinds table, `findings` doc type), `store.OwnerPublicID` spike case, `lifecycle` doc-type constant. FR-1.1, FR-1.2. |
| T2 | The budget in the dispatcher | | `internal/dispatch` only: the optional budget on `Plan`, the checks in `runLoop`, the stop outcome as a success, the per-turn usage callback, the turn cap as a stop. Unit tests with the mock provider. FR-5.1. |
| T3 | Starter pack and configuration | | `internal/config` (`spikes.default_token_budget`, `KnownTools` gains `save_findings`, the loader's refusal), `internal/starter` (generated config, assignment), `pack/templates/findings`, `pack/roles/spike-runner.yaml`, `pack/skills/run-spike`. FR-2.1, FR-10. |
| T4 | The spike service | T1, T2, T3 | `internal/server/spikes.go` (CreateSpike, StartSpike, EndSpike, CloseSpike, ask again), the `run-spike` planner case, `save_findings` and `finish_spike` in the tool host and executor, the rules for success and exhaustion, worktree add/discard, the heartbeat and boot sweeps, G5. Integration tests for FR-1 to FR-7 and NFR-4. |
| T5 | Web UI and timeline | T4 | `spike.html`, the spike page, owner sections, the New spike dialog, the start screen and its `POST`, the close forms, the Inbox line, `/ui/spikes`, the run page's purpose and crumbs, the feature timeline's spike moments (`internal/timeline`), the milestone refusal. One include in `entity.html`. FR-3, FR-7.1, FR-8. |
| T6 | MCP | T4 | `internal/server/mcp_spike_tools.go`, the registry line and instructions sentence in `mcp.go`, the tool-set test's want and forbidden lists, `add_milestone_member`'s refusal. FR-9. |

## Status

Every task is done and merged into `claude/subutai-m14-spikes`, and every
implementer's worktree is removed. Three rounds of fixes from the code reviews
(F1 and F2 after round 1, F3 after round 2) followed the same way. See the
[M14 handoff](handoff-M14-2026-10-02.md).

| id | Status | Merged as |
|----|--------|-----------|
| T1 | done | `6178a4e` |
| T2 | done | `8ad5f31` |
| T3 | done | `65989ae` |
| T4 | done | `f0b4e13`, `a7461d2`, `aee1616` |
| T5 | done | `4ee4495`, `c380550`, `15a308d` |
| T6 | done | `724dabe` |

## Risks

- **Enum values in a migration.** Postgres refuses to use a value added by
  `ALTER TYPE … ADD VALUE` in the same transaction, so the owner check must
  compare text (FR-1.1).
- **The mock provider is one FIFO.** Tests that run spikes beside other
  dispatches must script in order.
- **Worktree removal in tests** runs real `git`, so tests check the directory
  and `git worktree list`, not just the row.
