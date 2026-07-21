# REVIEW-002: Consistency Review of the Phase-2 Planning Package

**Status:** Complete — **approved by Sam 2026-07-21, as scoped (SD-1 accepted)**
**Date:** 2026-07-20 (review); 2026-07-21 (approval)
**Reviewer:** Claude (Fable 5), authoring review — the human approval gate is Sam's, below
**Scope:** DESIGN-005, DESIGN-006, SPEC-002, against the approved phase-1 package
**Not in scope:** re-approving DESIGN-001/002/003/004 or SPEC-001 (already approved; touched only by the recorded revision in §3)

## 1. What this review is — and is not

Unlike REVIEW-001, this is an **authoring** review: I wrote the documents,
so I can check them for internal consistency and completeness but I cannot
also be the approval gate. The whole system exists to prevent the "make
progress vs respect gates" conflict of interest (vision §1, §8); self-
approving a package I authored would be exactly that failure. So this pass
verifies the package hangs together and fixes what it can; **the approval
decision is Sam's.**

## 2. Findings (fixed in this pass)

- **R2-1 — Execution roles had no starter-pack requirement (medium).**
  SPEC-002 FR-7/8/9 dispatch `implementer`, `code-reviewer`, and `verifier`
  roles, but only FR-1.1 (dev-plan) required the pack to ship a role, and
  nothing required the `assignments:` map that binds the non-document
  purposes to those roles (DESIGN-004 F-4). An implementer would reach
  FR-7 with no role to dispatch. *Fix:* added FR-1.4 requiring the three
  roles, their skills, tool profiles, and the `assignments:` entries, with a
  config-error AC for a mis-declared verifier profile.
- **R2-2 — `tasks.local_id` absent from the approved schema (low).**
  DESIGN-005 §4/§7 (DP-3) needs a dev-plan-local id on tasks for stable
  re-decomposition, but DESIGN-001's approved `tasks` DDL had no such column.
  *Fix:* added `local_id` to the DDL with a note, and a DESIGN-001 §13
  revision entry — the same sanctioned revision mechanism used for the
  phase-1 dispatch columns, not a silent edit (L-2).

## 3. Recorded revision to an approved document

DESIGN-001 (Approved) gained `tasks.local_id` and a §13 revision entry
noting that `tasks`, `worktrees`, and `features.spec_stale` ship in the
phase-2 migration. This is a visible, logged revision, consistent with the
L-2 discipline for changing approved documents.

## 4. Non-blocking observation carried into implementation

- **A jailed implementer could, in principle, edit a registered approved
  document** (a spec or dev-plan file exists inside the feature's worktree,
  and the tool jail permits it). This is not a blocking gap for the slice:
  the implementer skill will forbid editing documents, code review inspects
  the diff, and the on-merge integrity check on `main` (DESIGN-003 §2) is the
  backstop. A hard tool-host guard that refuses edits to registered
  approved-document paths is a reasonable later refinement but would be
  over-engineering for proving the loop. Recorded here so implementation
  treats it deliberately rather than discovering it.

## 5. Checks that passed

- **Task lifecycle vs enum**: DESIGN-005 §5's `pending → ready → active →
  review → done`/`abandoned` matches the `task_state` enum frozen in
  migration 0001 (DESIGN-001 §3); no enum change needed.
- **Worktrees**: DESIGN-001 §9's table (including the nullable
  `graph_project`) supports DESIGN-006 §3 exactly; graph_project stays NULL
  under the SD-1 deferral, so the column is present-but-unused, matching the
  Bootstrap "true subset" property (TH-9).
- **G1 extension**: DESIGN-005 §3 flips the `devPlanRequired` flag the
  phase-1 `G1(spec, required, plan)` function already carries (SPEC-001 D-1);
  no new gate function or audit shape.
- **G2 / G3**: DESIGN-003 §8's expressions (`NOT spec_stale` in G2;
  verification-approve-then-merged in G3) are consumed unchanged by
  DESIGN-005 §6 and DESIGN-006 §6; the approve → merge → done ordering
  satisfies G3's "branch merged" term at the point the `review → done`
  transition is evaluated.
- **Config surface**: `assignments:` and `commands:` were reserved in
  DESIGN-004 §4 (F-4) as provisional-until-phase-2; DESIGN-006 §4.6 adds
  `output_cap_bytes` to a `commands:` entry, within that provisional
  allowance — no conflict with the approved DESIGN-004.
- **Governor check 3**: DESIGN-002 §6's per-feature mutating-serialisation
  check, a stub in phase 1 (all read-only), is activated by DESIGN-006 §5;
  the read-only exemption (O-5) still holds for review and verification.
- **Commit-then-git ordering**: DESIGN-006 §3/§5/§6's worktree creation, task
  commits, and merge reuse the exact ordering proven by the phase-1 revision
  file-takeover (DB commit first, git op after, failure → checkpoint).
- **Idempotency**: the phase-2 keys (`implement:<task>:<n>`,
  `review-code:<task>:<commit>`, `verify:<feature>:<head>`) extend the
  phase-1 scheme (DESIGN-002 §8) without collision.
- **Verification independence**: DESIGN-006 §6's clean-context, read-only
  verifier is faithful to DESIGN-003 §7 / L-4.

## 6. Recommendation

The package is internally consistent and complete for the phase-2 slice as
scoped. One scope reduction is proposed for Sam's explicit decision:

- **SD-1 (graph-project integration deferred).** `search_graph` and
  per-worktree indexing are deferred past the slice because they optimize a
  working tool host rather than prove the implementation loop; the schema and
  context fields exist so the later integration is additive. This is the one
  place the ambitious version (vision §8's full inherited-investment tool
  surface) is deliberately trimmed. Enumerated here per the planning
  discipline for Sam to accept or reject.

All other scope cuts (SD-2 human merge-conflict resolution, SD-4 direct
attachments only) mirror cuts already accepted in SPEC-001 and are low-risk.

Recommended: approve the package (optionally overriding SD-1) so phase-2
implementation can begin.
