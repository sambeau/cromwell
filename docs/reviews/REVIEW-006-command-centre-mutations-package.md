# REVIEW-006: Consistency Review of the Mutation-Slice Package (SPEC-006)

**Status:** Complete — **approved by Sam 2026-07-22, as scoped**
**Date:** 2026-07-22
**Reviewer:** Claude (Opus 4.8), authoring review — approval is Sam's, recorded in §5
**Scope:** [SPEC-006](../specs/SPEC-006-command-centre-mutations.md), against
[DESIGN-007](../design/DESIGN-007-web-command-centre.md) (§4, §8, CC-4),
[SPEC-004](../specs/SPEC-004-phase-4-web-command-centre.md) (the SD-1 deferral it
picks up), and the phase 1–3 code (the mutation service methods, the gates, the
respond path).

## 1. What this review is

An authoring consistency pass. I wrote SPEC-006, so I cannot also be its approval
gate (REVIEW-002/003/004 §1) — this checks that the spec hangs together with the
design it builds on and the code as it actually is, and fixes what it can. There
is **no new design doc**: DESIGN-007 §4 and §8 already fixed the mutation
architecture (forms post to `/ui/*` handlers calling the existing transactional
service methods, CC-4), exactly as SPEC-003 was approved spec-only against
DESIGN-001. The **approval decision is Sam's**, recorded in §5.

## 2. Findings (fixed in this pass)

- **R6-1 — G4 and G5 were conflated; G4 raises no checkpoint (correctness,
  material).** The draft's §2 and SD-2 said "a blocked G4 lock or G5 archive
  raises a checkpoint to the inbox." Only **G5** archive does that
  (`handleArchiveInitiative` → `store.CreateCheckpoint("gate-override", …)`,
  verified in the SPEC-004 smoke). **G4** lock refusal is a **409 carrying the
  reason** with no checkpoint (`http_phase3.go` `handleLockMilestone`: "G4 refusal
  is a 409 carrying the reason (no force path)"), and the operator must descope
  before locking. Corrected in §2 (FR-6 bullet), SD-2, and confirmed consistent
  with FR-2.2 (which already described the inline-descope path). This is the one
  claim that did not survive contact with the code.

## 3. Checks that passed

- **Every mutation names an existing service method / endpoint (SD-2, the
  membership test).** Verified present in the code:
  - estimates — `POST /api/estimate/set`, `POST /api/estimate/ai` (async, 202);
  - milestones — `POST /api/milestones`, `/members`, `/lock` (G4 via
    `store.LockMilestone`);
  - roadmaps — `POST /api/roadmaps`, `/entries` (`store.SetRoadmapEntry`, an
    upsert on `(roadmap_id, milestone_id)` — so re-order is a position-set, not
    an insert-and-shift; FR-3.1 and the §6 open question read that way);
  - tree — `POST /api/initiatives`, `/archive` (G5), `POST /api/features`,
    `/start`, `/abandon`;
  - document review — the `store.RespondCheckpoint` + `CheckpointResponded`
    path (`rules.decideCheckpointResponded` `review-escalation`), the same one
    the SPEC-004 inbox respond already drives.
  No FR requires a service method that does not exist; the one candidate that
  would have (human document *commenting*) is correctly placed **out of scope**
  (§2) precisely because no such method exists — the membership test doing its
  job.
- **No new authority (CC-4, L-6).** Every mutation is an existing
  gated/audited method; the UI never forces a gate. G5 routes to the inbox; G4
  surfaces inline; respond is the existing path. Consistent with the vision's
  rogue-orchestrator warning (§7) and SPEC-004 CC-4.
- **Second rendering, not self-HTTP (CC-2, NFR-2).** FR-6.1 requires the `/ui/*`
  mutation handlers to call the service methods directly, mirroring the read
  handlers SPEC-004 built — not to issue HTTP to `/api/*`.
- **Document review is a second surface, not a second authority (SD-4).**
  Approve / request-changes from the document view builds the same
  `EscalationResponse` and posts to the same respond path as the inbox; it
  appears only when an open `review-escalation` checkpoint refs the document — a
  filter over `PendingCheckpoints`, not a new read.
- **Realtime stays presentation-only (SD-5).** Mutations commit synchronously
  through their service method and return the updated fragment; the SSE refresh
  of other regions is the same authority-free signal SPEC-004 established.
  Idempotency and audit live in the service method, so a missed/duplicated event
  cannot cause a lost or double mutation.
- **Traces to DESIGN-007 and mirrors the phase discipline.** Each FR maps to a
  §8 catalogue entry; scope lists out-of-scope items with destinations; the
  central boundary (slice membership + the two constraints) is flagged as the §0
  decision, as SPEC-003 §0 / SPEC-004 §0 did; the DoD mirrors the prior slices
  (CI on both stacks, a live smoke + walkthrough, vet/`-race`, next-slice entry
  criteria or a feature-complete declaration).
- **Additive-only.** The slice adds `/ui/*` mutation handlers and their forms
  over existing methods; no schema change, no change to phase 1–3 tables, no new
  service operation.

## 4. Notes carried into implementation (not blocking)

- **request-changes comment fidelity** (SPEC-006 §6) — `EscalationResponse`
  carries structured `comments`; the honest floor is a single reason, structured
  per-section comments a fidelity add. Decide at build.
- **Estimator-pending UX** — `estimate ai` is async (202); the pending node and
  the failure path (an exhausted estimator dispatch surfaces as a dispatch
  failure in the inbox) want a small interaction decision.
- **Numbering.** This slice is **SPEC-006**, not SPEC-005: DESIGN-007 §9 already
  reserved SPEC-005 for the MCP facet, and renumbering an approved assignment is
  more disruptive than taking the next free number. Build order is
  priority-driven, not numeric.
- **create vs. planning-setup membership** (SPEC-006 §6) — initiative/feature
  *create* is a light mutation; if it complicates the slice it can move to the
  same later increment as human commenting. Flagged for the reviewer.

## 5. Recommendation and decision

The spec is internally consistent and complete for the mutation slice as scoped,
and — after R6-1 — matches the gates and service methods it builds on. Every
mutation renders an operation that already exists as a gated, audited method, so
the slice is a rendering increment with no new authority, exactly the model
DESIGN-007 §4/§8 fixed.

Decisions for the reviewer:

1. **Slice membership.** Approve the mutation set as scoped (estimates,
   milestones, roadmaps, tree lifecycle, escalated-review actions), or adjust —
   e.g. defer initiative/feature *create* to a later increment (§6), or pull
   human document commenting forward (which would require a new service method
   and thus a small design note, breaking the spec-only shape).
2. **request-changes fidelity** — single reason now vs. structured per-section
   comments, if a preference exists before build.

**Recommended for approval as scoped.** I do not approve a package I authored.

_Decision (Sam): **Approved as scoped, 2026-07-22.** The full mutation set —
estimates, milestones (incl. G4 lock), roadmaps, tree lifecycle (create,
start, abandon, archive G5), and escalated-review approve/request-changes from
the document view. request-changes carries a **single reason** (FR-5, the §6
floor); structured per-section comments are a later add. Implementation may
begin._
