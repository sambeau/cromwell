# REVIEW-004: Consistency Review of the Phase-4 Package (DESIGN-007 + SPEC-004)

**Status:** Complete — **approved by Sam 2026-07-21, as scoped**
**Date:** 2026-07-21
**Reviewer:** Claude (Opus 4.8), authoring review — approval is Sam's, recorded below
**Scope:** [DESIGN-007](../design/DESIGN-007-web-command-centre.md) and
[SPEC-004](../specs/SPEC-004-phase-4-web-command-centre.md), against the vision
(§7, §11, §14), DEC-001/DEC-002, and the phase 1–3 code (the service layer,
the event bus, the config).

## 1. What this review is

An authoring consistency pass. I wrote both documents, so I cannot also be the
approval gate (REVIEW-002/003 §1) — this checks that the package hangs together
with the vision, the prior decisions, and the code as it actually is, and fixes
what it can. The **approval decision is Sam's**, recorded in §5.

## 2. Findings (fixed in this pass)

Three claims did not survive contact with the code; all three are corrected in
the drafts.

- **R4-1 — The realtime source cannot be a second bus consumer (correctness,
  material).** The draft described the SSE broadcaster as "one more bus consumer
  alongside the orchestrator." The in-process bus is **many-producers,
  one-consumer** (`bus.go`: a single channel, drained by the orchestrator at
  `server.go` `case ev := <-s.Bus.Events()`); a second reader would *steal*
  events. Corrected (DESIGN-007 §6, CC-3; SPEC-004 FR-7.1): realtime is served
  by a **broadcast hub** the orchestrator forwards each event to as it processes
  it, with the `Notifier` interface as the hub's subscribe seam. The proven bus
  is untouched; a slow client is dropped, never back-pressuring the
  orchestrator.
- **R4-2 — The live inbox depended on an event that does not exist
  (completeness).** FR-3.1 requires a checkpoint to appear "the moment it is
  raised," but checkpoint *creation* emits no in-process event (the bus has
  `checkpoint.responded`, not "raised"; the `pg_notify` trigger fires only on
  checkpoint *state changes*). Added FR-8.2 and the DESIGN-007 §6/§8 note: a
  lightweight **checkpoint-raised** signal emitted into the hub at the
  `store.CreateCheckpoint` call site — additive, no schema change.
- **R4-3 — A named service method did not exist (low).** DESIGN-007 §4 cited
  `s.RespondCheckpoint`; the respond path is `store.RespondCheckpoint` plus the
  `CheckpointResponded` bus publish (as `handleRespond` does). Corrected to the
  real names.

## 3. Checks that passed

- **Stack (CC-1) is consistent with the codebase.** The server already exposes
  a TCP listener distinct from the CLI socket (`config.ServerConfig.HTTP`, yaml
  `server.http`, used at `server.go` `net.Listen("tcp", …)`), so serving the UI
  there needs no new transport. The single-binary/`embed.FS` pattern matches
  migrations and the starter pack.
- **Two renderings over one service layer (CC-2)** matches the existing split:
  JSON handlers already call store/service functions directly; the HTML surface
  is a parallel set of handlers over the same functions, not a self-HTTP client.
- **No new authority (CC-4)** is consistent with L-6 and the vision's
  rogue-orchestrator warning (§7); respond is the only mutation in the slice and
  it is the existing checkpoint-response path.
- **Not-an-editor / config read-only (CC-5)** is consistent with vision §7 and
  the three-compartment model (§12).
- **Operation catalogue (DESIGN-007 §8) traces to real methods.** Every read and
  the respond mutation name existing functions — `InitiativeSizingNode`,
  `LiveProgress`/`SnapshotProgress`, `ListMilestones`, `RoadmapEntries`,
  `PendingCheckpoints`, `CommentsForDocument`, `CurrentDocForOwner`,
  `CostRollup`/`CostByMonth`/`*Cost`, `QueuedDispatches`, `AuditTail` — verified
  present.
- **SPEC-004 traces to DESIGN-007 and mirrors the phase discipline.** Each FR
  maps to a design section; scope lists out-of-scope items with destinations;
  SD-1 (the read-everywhere + respond slice boundary) is flagged as the §0
  central decision, exactly as SPEC-003 §0 flagged its slicing; the DoD mirrors
  phases 1–3 (CI on both stacks, a live smoke + walkthrough, vet/`-race`, and
  next-slice entry criteria).
- **Additive-only (NFR-1/3, CC-7).** The new reads and the checkpoint-raised
  signal are additive over the existing store; no schema change, no change to
  phase 1–3 tables.

## 4. Notes carried into implementation (not blocking)

- **HTMX is vendored, not built.** HTMX and its SSE extension ship as embedded
  static assets (single JS files via `embed.FS`) — no npm, no bundler,
  consistent with CC-1. Worth stating so "no new stack" is unambiguous.
- **Markdown rendering** for the read-only document body needs a Go
  Markdown→HTML choice (SPEC-004 §6 open question); fidelity target is headings,
  tables, code.
- **SSE reconnect/backfill** — a full refresh on reconnect is the honest floor
  (SPEC-004 §6); smarter backfill is a later refinement.
- **Live-region granularity** — coarse "refresh this panel" vs targeted fragment
  swaps is interaction tuning, not a contract (SPEC-004 §6).

## 5. Recommendation and decision

The package is internally consistent and complete for the operational slice as
scoped, and now matches the code it builds on. The **central scope decision** —
slicing the command centre so the *operational* surface (read-everywhere +
respond, live) comes before the planning/review mutations — is presented in
SPEC-004 §0 for exactly this choice, with the redirect (fold the planning
mutations into this slice) noted as a one-line alternative.

Two decisions for the reviewer:

1. **The slice boundary (SD-1).** Approve *read-everywhere + respond* as the
   first slice, or redirect to *read-everywhere + all-mutations*.
2. **Anything to deepen before build** — e.g. a visual/interaction wireframe
   pass, or the Markdown-renderer choice, if wanted before implementation.

**Recommended for approval as scoped.** I do not approve a package I authored.

_Decision (Sam): **Approved as scoped, 2026-07-21.** The first slice is the
operational command centre — read-everywhere + respond (SD-1); planning/review
mutations deferred to a later slice. Implementation may begin._
