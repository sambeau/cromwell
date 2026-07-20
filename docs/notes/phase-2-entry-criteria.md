# Phase-2 Entry Criteria (draft)

**Status:** Draft — satisfies SPEC-001 DoD item 5
**Date:** 2026-07-20

Phase 2 (the implementation loop — README phase plan) may start when:

1. **Phase 1 is fully done** — every SPEC-001 acceptance criterion green in
   CI on plain Postgres and the Supabase local stack, and the live smoke
   test recorded in `docs/walkthrough.md` with its audit trail and cost
   inspected by a human.
2. **A SPEC-002 exists and is approved**, scoped to:
   - **Tasks and dev-plans**: `dev_plan` template + manifest, task
     decomposition from the approved dev-plan, `tasks` table wired to the
     lifecycle engine's existing task machinery; G1 extended to require the
     dev-plan (flip the `devPlanRequired` flag — the expression already
     exists, D-1).
   - **Implementer dispatch + tool host**: worktree creation per feature
     (`worktrees` table), per-role tool profiles, implicit context
     injection with path jailing, hash-anchored read/edit, command
     whitelist (`commands:` in config.yaml — shape already reserved),
     within-feature mutating-dispatch serialisation (governor check 3).
   - **Close-out verification (G3)**: verifier role with clean context and
     read-only tools, `submit_verification` outcome tool, merge-then-done
     ordering per DESIGN-003 §7.
   - **Revision-in-flight blocking** (DESIGN-003 §5.3): the `spec_stale`
     column ships with its migration; successor submission blocks new task
     dispatches and raises the `revision-in-flight` checkpoint.
3. **Phase-1 learnings are folded back**: escalation-rate telemetry from
   the dispatch ledger reviewed once against real usage; reviewer skill
   revised if the rate is pathological in either direction (vision §9).

Carry-overs deliberately not blocking phase-2 entry: embeddings/semantic
retrieval and section role classification (phase-2 scope but can land late
in it), checkpoint TTL expiry (DESIGN-002 §3 duty 3, unexercised in
phase 1), `cromwell upgrade` (needs a second pack version to exist).
