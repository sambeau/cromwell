# DESIGN-005: Dev-plans, Tasks, and Decomposition

**Status:** Draft for review
**Date:** 2026-07-20
**Parent:** [vision-v1](../vision/vision-v1.md) §3, §4; [DESIGN-003](DESIGN-003-document-lifecycle-and-gates.md) §5 (revision), §6 (feature lifecycle), §8 (gates)
**Depends on:** DESIGN-001 (schema), DESIGN-003 (lifecycle), DESIGN-004 (compartment)
**Extended by:** DESIGN-006 (tool host, which executes the tasks this document defines)

## 1. Purpose

Defines the second document type and the work items it produces: the
**dev-plan**, its review, its decomposition into **tasks**, the extension of
gate G1 to require it, the task lifecycle, and the revision-in-flight
blocking that DESIGN-003 §5.3 specified but phase 1 marked N/A. This is the
"how it will be built" half of the contract (vision §3); DESIGN-006 covers
who builds it.

This document reuses phase 1's machinery wherever it fits — the document
lifecycle, validation, agent-review, and gate mechanisms are unchanged. The
new surface is: one document type, one decomposition step, the task entity's
lifecycle, and three gate wirings (G1 extension, G2, and the revision block).

## 2. The dev-plan document type

A dev-plan is a document (DESIGN-001 §5) of type `dev_plan`, owned by a
feature, following the same lifecycle as every other document
(`draft → reviewing → approved → superseded`). It ships in the starter pack
as `templates/dev_plan/` with a template, a manifest, and a
`dev-plan-reviewer` role (DESIGN-004 §5–7).

The dev-plan's distinguishing structure is a **task table**: a Markdown
table under a required `## Tasks` heading, one row per task, that
decomposition (§4) parses into task rows. The manifest enforces it with a
new validation rule kind `table_parses` (DESIGN-004 §7, F-6; reserved there
for exactly this):

```yaml
# templates/dev_plan/manifest.yaml
type: dev_plan
reviewer_role: dev-plan-reviewer
front_matter:
  required: [title, type, owner]
sections:
  order: strict
  required:
    - heading: Approach
    - heading: Tasks
rules:
  - kind: table_parses
    section: Tasks
    columns: [id, title, depends_on, description]
```

The task table's columns are fixed by the rule: `id` (a short local
identifier unique within the plan, e.g. `T1`), `title`, `depends_on` (a
comma-separated list of task `id`s in the same plan, or empty), and
`description`. `table_parses` fails validation (mechanically, at submit —
DESIGN-003 §3) if the table is missing, malformed, has unknown columns,
duplicate ids, a `depends_on` referencing an id not in the table, or a
dependency cycle. The dependency graph is therefore known-acyclic before a
human or reviewer ever reads the plan.

Review is unchanged from phase 1: on submit, validation runs; on pass, the
`dev-plan-reviewer` is dispatched with the same three verdicts. The reviewer
receives the feature's **approved spec** as a background document (it cannot
sensibly judge a decomposition without the contract it decomposes), the
feature context, and ancestor documents — the existing auto-surfacing tiers
(DESIGN-002 §4 step 3) already cover this once the spec is a direct
attachment of the feature.

## 3. Gate G1, extended

Phase 1 narrowed G1 to spec-only (SPEC-001 D-1) behind a flag the code
already carries: `G1(specApproved, devPlanRequired, devPlanApproved)`. Phase
2 flips `devPlanRequired` to true. G1 now reads, exactly as
DESIGN-003 §8 always specified it:

> **G1 contract-approved:** current spec `approved` AND current dev-plan
> `approved` (current = the non-superseded doc of that type owned by the
> feature).

No new gate function, no new audit shape — the phase-1 expression extends in
place, which is why it was written with the flag. A feature reaches `ready`
only when both halves of its contract are approved.

## 4. Decomposition

Decomposition is the single new orchestrator step this document adds. It is
a pure transformation with an audited effect, triggered by one event.

**Trigger.** When a dev-plan transitions to `approved` (the same
`DocumentTransitioned{To: approved}` event the rule engine already handles),
and the document's type is `dev_plan` and its owning feature is in `idea`,
the rule engine emits a `DecomposeDevPlan` action alongside the existing
`EvaluateContractGate`. Order matters and is guaranteed by the action list:
decomposition runs first, so the tasks exist before G1 advances the feature
to `ready`.

**Transformation.** The parsed task table (already validated acyclic) maps
to `tasks` rows (DESIGN-001 §4):

- `position` = row order in the table.
- `title`, `description` = table cells.
- `depends_on` = the table's `depends_on` ids resolved to the `tasks.id`
  UUIDs of the same decomposition (a two-pass insert: create rows, then set
  `depends_on` arrays).
- `state` = `pending`.

Each created task writes a `task.created` audit row; the decomposition as a
whole writes a `devplan.decomposed` row carrying the task count. All in one
transaction (O-3).

**Re-decomposition on revision.** When an *approved* dev-plan is revised
(DESIGN-003 §5) and its successor is approved, the task set is
re-derived — but existing task state is never silently discarded
(DESIGN-003 §5.4). The reconciliation rule:

- A table row whose `id` matches a task that is `pending` or `ready` is
  updated in place (title/description/deps refreshed).
- A table row with no matching prior task is created.
- A prior task with no matching row that is `pending`/`ready` is deleted.
- A prior task with no matching row that is `active`/`review`/`done` is
  **kept** and surfaced in the `revision-in-flight` checkpoint (§6) as a
  mismatch for human decision — its work already happened; the system will
  not pretend otherwise.

Matching is by the table's local `id`, which is therefore recorded on the
task row (a new `local_id` column, §7). This makes re-decomposition stable
across revisions rather than positional and fragile.

## 5. Task lifecycle

Tasks use the `task_state` enum already in the schema (DESIGN-001 §3):
`pending → ready → active → review → done`, plus `abandoned`. The shape
mirrors the feature lifecycle deliberately — a task is a feature-sized unit
of execution — but with dependency-driven readiness instead of a human
start.

```
        deps satisfied        claim           impl done       approved
┌─────────┐ ──────────▶ ┌───────┐ ──────▶ ┌────────┐ ──────▶ ┌────────┐ ─────▶ ┌──────┐
│ pending │  (auto)     │ ready │ (disp.) │ active │  (code  │ review │ (code  │ done │
└─────────┘             └───────┘         └────────┘  review)└────────┘ review)└──────┘
     │                      │                  │  request_changes  │
     └──────────────────────┴──── abandon ─────┴───────────────────┘──▶ abandoned
```

| State | Meaning |
|---|---|
| `pending` | Created by decomposition; one or more dependencies not yet `done` |
| `ready` | All `depends_on` tasks are `done`; eligible for implementer dispatch |
| `active` | An implementer dispatch is running (worktree work in progress) |
| `review` | Implementation finished; code-review dispatch judges the diff |
| `done` | Code review approved; the task's changes are on the feature branch |
| `abandoned` | Closed without completing (always human, always with reason) |

**Readiness (`pending → ready`)** fires automatically: whenever a task
transitions to `done`, the rule engine re-evaluates the readiness of every
`pending` task in the same feature that lists it in `depends_on`; those whose
dependencies are all `done` advance to `ready` and become dispatchable. A
task with no dependencies is `ready` the moment decomposition creates it.
Tasks with `state = ready` and the feature `active` are what the dispatcher
picks up (DESIGN-006 §4).

**Code review (`active → review → done`)** reuses the phase-1 agent-review
machinery with a new purpose `review-code`: the reviewer receives the spec,
the dev-plan, the task, and the diff (DESIGN-006 §5), and returns the same
three verdicts. `request_changes` returns the task to `active` with the
comments and re-dispatches the implementer (the diff is amended, not
restarted from scratch — the worktree persists). `escalate` raises a
checkpoint. This is the same `submit_review` outcome tool; only the assembled
context differs.

## 6. Revision-in-flight blocking (resolves DESIGN-003 §5.3, deferred from phase 1)

DESIGN-003 §5.3 requires that revising a spec or dev-plan of a feature in
`active`/`review` must not let work run against a silently changed contract.
Phase 1 had no active features, so this was N/A. Phase 2 makes it real.

The mechanism, exactly as DESIGN-003 §5.3 specified, now implementable:

1. When a successor spec or dev-plan is **submitted** (not approved) for a
   feature in `active` or `review`, the lifecycle engine sets
   `features.spec_stale = true` (the column DESIGN-001 §4 added for this),
   audited.
2. `spec_stale` blocks *new* task dispatches: the dispatcher's readiness
   query excludes tasks of a stale feature, and G2 (tasks-complete) includes
   `NOT spec_stale` (DESIGN-003 §8, already written that way). Running
   dispatches finish; nothing new starts.
3. A `revision-in-flight` checkpoint is raised asking the human to **pause**
   (leave the feature blocked until the revision lands) or **continue** (the
   revision applies to later work only). The checkpoint carries the list of
   in-flight and completed tasks so the human sees what is at risk.
4. On the successor's approval, re-decomposition (§4) runs and its mismatches
   (kept `active`/`done` tasks with no matching row) are surfaced;
   `spec_stale` is cleared once the human resolves the checkpoint.

`spec_stale` is a single boolean, not a workflow state, precisely because the
*decision* about what to do belongs to the human via the checkpoint — the
flag only guarantees that no new work commits to the old contract while that
decision is pending.

## 7. Schema additions

Phase 2's migration (`0002_*.sql`) adds only what phase 2 uses (DESIGN-001
§11 policy):

- `features.spec_stale boolean NOT NULL DEFAULT false` — DESIGN-001 §4 already
  documents this column against its phase-2 migration; it ships here.
- `tasks.local_id text` — the dev-plan-local identifier (`T1`) that makes
  re-decomposition stable (§4). Unique per feature among non-abandoned tasks.
- The `tasks` and `worktrees` tables themselves (DESIGN-001 §4, §9), created
  by this migration since phase 1 shipped neither.

No enum changes: `task_state` and `feature_state` were defined complete in
migration 0001. Enum values are only ever appended (DESIGN-001 §11); none
need appending.

## 8. What this document does not cover

- **How a task is executed** — the implementer dispatch, the worktree, the
  tool host, the diff, code review context assembly, verification, and merge
  are DESIGN-006.
- **Estimates on tasks** — sizing (vision §5) is phase 3; the `estimates`
  table is not created in phase 2.
- **Defects** — the defect pipeline (DESIGN-003 §9) reuses this task
  machinery but its triage design is deferred (vision §14).

## 9. Decisions recorded here

| # | Decision | Rationale |
|---|---|---|
| DP-1 | Dev-plan carries a fixed-column task table; `table_parses` validates it (incl. acyclicity) before review | Decomposition is mechanical and safe; the reviewer judges substance, not table hygiene |
| DP-2 | Decomposition runs on dev-plan approval, before G1 advances the feature | Tasks exist the moment the feature is `ready`; no window where a ready feature has no tasks |
| DP-3 | Tasks carry the dev-plan-local `id` (`local_id`); re-decomposition matches on it | Stable identity across revisions; positional matching would corrupt on reorder |
| DP-4 | Re-decomposition never deletes started/done tasks; mismatches go to a checkpoint | DESIGN-003 §5.4's honesty rule; work that happened is never erased silently |
| DP-5 | Task readiness is dependency-driven and automatic; only `ready` tasks of an `active`, non-stale feature dispatch | The dependency graph drives the loop; `spec_stale` is the one brake, held by a human decision |
| DP-6 | Code review reuses the phase-1 `submit_review` outcome and verdicts, with `review-code` context | One review mechanism for all artefacts; only the assembled context differs (vision §9) |
