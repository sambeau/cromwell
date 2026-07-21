# REVIEW-003: Consistency Review of SPEC-003

**Status:** Complete — **approved by Sam 2026-07-21, as scoped**
**Date:** 2026-07-21
**Reviewer:** Claude (Fable 5), authoring review — approval is Sam's, recorded below
**Scope:** SPEC-003, against the approved DESIGN-001/003/004 and the phase-1/2 code

## 1. What this review is

An authoring consistency pass (I wrote SPEC-003, so I cannot also be the
approval gate — REVIEW-002 §1). It checks that the spec hangs together with
the existing approved schema and gates and fixes what it can. Sam reviewed the
package and approved it as scoped; DESIGN-007 was declined (the existing
DESIGN-001 §6–7 schema and DESIGN-003 §8 gate make the spec self-sufficient).

## 2. Finding (fixed in this pass)

- **R3-1 — Corpus retrieval relied on entity tags that do not exist (low).**
  SD-1 and FR-3.2 specified `considered`-tier retrieval "by full-text + tag
  overlap", but estimates are on features and tasks, which carry a
  `description` and no `tags` column (only documents have `tags`). *Fix:*
  retrieval is full-text rank over entity descriptions; the tag-overlap claim
  is dropped (noted as a documents-only concern). The spec is now
  implementable exactly as written.

## 3. Checks that passed

- **Estimates**: FR-1's row shape matches DESIGN-001 §7 (`ref_type`,
  `ref_id`, `tokens`, `tier`, `rationale`, `dispatch_id`; latest-is-current
  with history). Tiers match vision §5 exactly.
- **Worst-tier roll-up** (FR-2): matches vision §5's "the parent's tier is the
  worst tier in the tree" and "`?` for unestimated, listed explicitly".
- **Calibration** (FR-3): actuals from the dispatch ledger by owner; the
  corpus as a view, not a table (DESIGN-001 §7) — consistent, and the
  embeddings deferral (SD-1) is handled without schema debt (the corpus data
  is recorded regardless).
- **AI estimator** (FR-4): the `estimate` purpose binds via `assignments:`
  (DESIGN-004 §5 F-4, the non-document resolution path); `submit_estimate` is
  a code-defined outcome tool (O-2) — same pattern as phase-2's outcome tools.
- **Milestones/G4** (FR-5): live membership + snapshot-on-lock matches
  DESIGN-001 §6 (`milestone_members`, `milestone_snapshots`); G4's expression
  ("at least one resolved member `done`") matches DESIGN-003 §8. `member_type`
  reuses the `ref_type` enum, which already admits `milestone`/`checklist`, so
  deferring checklist members (SD-2) is additive.
- **Cost roll-ups** (FR-7): extend the phase-1 ledger rollup; the frozen
  price snapshots (O-4) make historical per-initiative/month totals exact.
- **Migrations** (NFR-3): `0004` adds only phase-3 tables — consistent with
  the forward-only, ship-only-what-you-use policy (DESIGN-001 §11).

## 4. Notes carried into implementation (not blocking)

- **Enum creation.** `estimate_tier` and `milestone_state` are defined in
  DESIGN-001 §3 but were not shipped by migrations 0001–0003 (only the subset
  in use). Migration `0004` creates them, exactly as `0002` created
  `task_state`. Enum values are only ever appended.
- **FTS over entity descriptions.** The `considered`-tier corpus (SD-1) needs
  full-text search over `features.description` / `tasks.description`; phase 1's
  FTS index is on `document_sections`, so `0004` (or the query) provides FTS
  over entity descriptions. An implementation choice, not a contract.

## 5. Recommendation and decision

The package is internally consistent and complete for the planning slice as
scoped. The central scope decision — slicing the command centre so the
planning engine (this spec) comes before the web UI (SPEC-004) and MCP facet
(SPEC-005) — was presented in §0 for exactly this choice. **Sam approved it as
scoped on 2026-07-21**, and declined a companion DESIGN-007. Phase-3
(planning-slice) implementation may begin.
