# SPEC-004 Entry Criteria — Web UI (the command centre's visible surface)

**Status:** Draft — satisfies SPEC-003 DoD item 5
**Date:** 2026-07-21
**Parent:** [SPEC-003](../specs/SPEC-003-phase-3-planning-and-tracking.md) §0
(the command centre, sliced), which deferred the web UI to its own spec (SD-3,
DEC-001).

## Where this sits

SPEC-003 sliced the command centre so the **planning-and-tracking engine**
shipped first — sizing, tiers, calibration, milestones, roadmaps, cost — as a
CLI- and HTTP-exposed engine with no new stack. That engine is now built and
proven. SPEC-004 is the next slice: the **web UI**, the primary human surface
(vision §7), rendering what the engine and phases 1–2 already produce over the
same HTTP API. The MCP facet remains SPEC-005.

## SPEC-004 (web UI) may start when

1. **SPEC-003 is fully done** — every acceptance criterion green in CI on
   plain Postgres and the Supabase local stack, and the live smoke test
   recorded in [walkthrough-phase-3.md](../walkthrough-phase-3.md) with its
   ledger, estimates, and calibration inspected by a human (SPEC-003 DoD 1–4).

2. **A SPEC-004 exists and is approved**, scoped to the web UI as a distinct
   front-end project consuming the existing HTTP API (DEC-001 — no new
   mutation surface; the UI drives the same endpoints the CLI does). The spec
   should decide, at least:
   - **The read surfaces** (vision §7): the initiative → feature → task tree;
     the live dispatch queue and event/audit stream; the **estimate roll-ups**
     (tokens, tier, the `?` for unestimated work) and **cost dashboards** now
     that real data exists (`/api/estimate`, `/api/cost/*`); **milestone**
     progress and **roadmap** ordering (`/api/milestone`, `/api/roadmap`).
   - **The write surfaces** already exposed by the API: the checkpoint inbox
     with respond; document submit/revise; `estimate set`/`estimate ai`;
     milestone create/add/remove/lock; roadmap create/add. No gate override
     except through the answered-checkpoint path (L-6).
   - **Realtime**: the event stream behind the `Notifier` boundary (DEC-002),
     Supabase realtime in the hosted deployment, polling as the local
     fallback. No business logic in the client — it renders server state.
   - **Auth and multi-user**: out of scope for a first slice, or explicitly
     decided. The API's `X-Cromwell-Actor` header is the only identity today.

3. **The API gaps the UI needs are named** — SPEC-003 exposed the planning
   engine through task-shaped endpoints; a browser surface will want list and
   aggregate reads the CLI did not (e.g. "all open milestones with progress",
   "the estimate roll-up for the whole tree", paginated audit). These are
   additive HTTP handlers over the existing store, and SPEC-004 should
   enumerate them rather than discover them mid-build.

## Carry-overs to fold in or defer (from SPEC-003)

- **Embedding-based corpus retrieval** (SD-1/SD-4): the `considered` tier
  retrieves by full-text rank today; embeddings are still deferred. The corpus
  records `(description, estimate, actual)` in full, so this is a later
  retrieval-mechanism swap, not a data migration. Not a web-UI concern, but
  the UI will make retrieval quality visible, which may raise its priority.
- **Corpus retrieval ranking** (SPEC-003 §6): FT-rank is tuned against real
  data during use; the UI surfacing estimate-vs-actual deltas is where poor
  reference points become obvious.
- **The remaining command-centre entities** — MCP facet (SPEC-005), defects,
  jobs/checklists (and thus checklist milestone membership, SD-2) — stay in
  their own specs; the web UI should be built so adding them later is
  additive, not a rework.

## Not blocking

The planning engine stands on its own via the CLI; SPEC-004 is a rendering
slice, not a dependency of anything already shipped. It starts when the team
chooses the visible command centre as the next increment and writes the spec.
