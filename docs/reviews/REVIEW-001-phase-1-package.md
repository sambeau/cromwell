# REVIEW-001: Approval Review of the Phase-1 Planning Package

**Status:** Complete — verdict **approve** (after fixes applied in this pass)
**Date:** 2026-07-20
**Reviewer:** Claude (Fable 5), review delegated by Sam
**Scope:** DESIGN-001, DESIGN-002, DESIGN-003, DESIGN-004, SPEC-001
**Not in scope:** vision-v1 (remains a draft reference by design; downstream
documents carry authority), DEC-001/DEC-002 (already Accepted)

## 1. Verdict

**Approve.** The package is internally consistent, complete for its stated
phase-1 scope, and every scope cut has a recorded destination and rationale.
Two genuine defects were found and fixed in this pass (§3); with those fixes,
no contradiction between any two documents remains that would surface during
phase-1 implementation.

## 2. Method

Full read of all five documents plus vision-v1, followed by pairwise
consistency checks: every cross-reference chased, every mechanism that two
documents both describe compared, every SPEC-001 acceptance criterion checked
for implementability against the designs it cites.

## 3. Findings (both fixed in this pass)

- **R-1 — Single-writer rule vs `cromwell init` (medium).** DESIGN-002 §3 and
  O-1 stated the CLI never opens a Postgres connection, but FR-1.1 requires
  `init` to apply migrations before any server exists — a direct, unstated
  contradiction. FR-2.2's acceptance criterion ("the CLI binary path imports
  no store/pgx") was also unsatisfiable, since the single binary contains
  `serve`. *Fix:* DESIGN-002 §3 now documents the exception (`init`/`upgrade`
  touch schema and `schema_migrations` only, never workflow state); FR-2.2
  carves out `init` and asserts on CLI *client packages* instead of the
  binary.
- **R-2 — Revision flow had no actor for the file operations (medium).**
  DESIGN-003 §5 said the successor "takes over the canonical path and the
  predecessor's file is archived" without naming who performs the git
  operations — an implementer would hit this immediately in FR-4.4. *Fix:*
  the lifecycle engine performs the move/archive in a single server-authored
  commit at approval time, which the watcher re-indexes like any other
  commit.

## 4. Checks that passed

- Document lifecycle: four states, six events (DESIGN-003 §2) — matches
  FR-5.1's count; `escalate` leaving state at `reviewing` matches FR-5.4.
- Revision timing vs the `documents_live_path` partial unique index: the
  successor holds a suffixed path until the same transaction that supersedes
  the predecessor, so uniqueness on the canonical path is never violated.
- G2's `spec_stale` term now has its schema column (features table), marked
  for the phase-2 migration; G1's phase-1 narrowing (spec-only) is an
  explicit recorded decision (D-1) with the extension point named.
- NFR-3's phase-1 table set is closed under foreign-key references
  (document_comments → dispatches included; worktrees correctly excluded).
- Prompt assembly order (DESIGN-002 §4 step 3) matches DESIGN-004's stated
  positions for identity and skill; purpose strings (`review-spec`) are
  consistent across DESIGN-001, DESIGN-004, and SPEC-001.
- Checkpoint idempotency per (kind, ref) while pending: identical in
  DESIGN-002 §7 and FR-6.2.
- Governor checks (budget, rate, serialisation, workers) each have a
  config.yaml counterpart in DESIGN-004 §4; price snapshot mechanism agrees
  between DESIGN-001 §8, DESIGN-002 O-4, and DESIGN-004 F-3.
- Starter-pack file list: FR-1.1 and DESIGN-004 §10 agree exactly.
- Boot catch-up scan: DESIGN-002 §8 behaviour is exercised by FR-4.2/FR-4.3
  acceptance criteria, including the approved-document integrity case.
- No-secrets rule: NFR-4, DEC-002, and DESIGN-004 F-2 agree (env-var names
  in config, values in the environment).

## 5. Authority and provenance

This review was conducted by an AI assistant at Sam's explicit direction; Sam
remains the accountable owner of the package and can supersede this approval
at any time. From this point the package adopts the discipline it specifies
for itself (DESIGN-003 L-2): approved documents are not silently edited —
material changes go through an explicit, visible revision commit that states
what changed and why.
