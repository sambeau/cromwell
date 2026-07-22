# SPEC-006 Session Record — The Command Centre's Mutation Slice

**Status:** Implemented and green — satisfies SPEC-006 Definition of Done items
1, 3, 4, 5; item 2 (the human-confirmed live smoke) demonstrated in-session and
open for Sam's confirmation.
**Date:** 2026-07-22
**Operator:** Sam (commands run by Claude on Sam's behalf)
**Automated-test database:** dedicated dev Postgres (`cromwell-pg-dev`, 54329)
**Provider:** mock for CI; none needed for the mutation smoke (the document-review
path uses a mock-escalated review in the handler suite).

This records the command centre's second slice ([SPEC-006](specs/SPEC-006-command-centre-mutations.md),
approved [REVIEW-006](reviews/REVIEW-006-command-centre-mutations-package.md)):
the planning and review **mutations**, turning the command centre from
"monitor and answer" (SPEC-004) into "operate." No new design doc — DESIGN-007
§4/§8 already fixed the mutation architecture; this is a rendering increment.

## 1. What the slice proves (SPEC-006 §1)

1. **Planning is driveable from the browser** — set and AI-estimate work; create
   milestones, add and descope members, lock (G4); create roadmaps and order
   entries — each posting to the existing service method, each audited, the
   result reflected live.
2. **The tree lifecycle is driveable** — create initiatives and features; start
   and abandon features; archive an initiative, with G5 routing a block to the
   inbox rather than forcing it.
3. **Review closes from the document view** — approve / request changes on an
   escalated review, through the same checkpoint-response path the inbox uses.

## 2. How it was built (the pattern, then its replication)

The estimate mutation established the pattern the rest follow:

- a `/ui/*` POST handler calls the **same gated, audited service method** the CLI
  does (`store.RecordEstimate`, `CreateMilestone`, `AddMember`/`RemoveMember`,
  `LockMilestone`, `CreateRoadmap`, `SetRoadmapEntry`, `CreateInitiative`,
  `CreateFeature`, `StartFeature`, `TransitionFeature`, `ArchiveInitiative`, and
  `RespondCheckpoint`) — never self-HTTP (CC-2), never new authority (CC-4),
  acting as the single operator `server.ui_actor` (SD-3);
- `renderPlanningBody` re-reads the planning view and re-renders the swappable
  `frag-planning`, with an inline notice or error banner so a rejection never
  surfaces as a raw 500 (FR-6.2);
- the Planning page carries an **Actions panel** of forms that `hx-post` and
  swap `#planning-body`; the document view carries the review controls, shown
  only when the document is human-gated.

**Gate fidelity (the REVIEW-006 R6-1 correction, honoured in code):**

- **G5** archive block raises a **gate-override checkpoint to the inbox**
  (`notifyCheckpointRaised`), and the initiative is *not* archived until the
  override is answered (L-6).
- **G4** lock block is **inline** — the gate reason is surfaced for the operator
  to descope, with **no checkpoint** (the CLI's 409 path). The two are not
  conflated.

**Document review is a second surface, not a second authority (SD-4):** approve /
request-changes build the same `EscalationResponse` and post to the same
`RespondCheckpoint` + `CheckpointResponded` path as the inbox respond; the
controls appear only when an open `review-escalation` checkpoint refs the
document (a filter over `PendingCheckpoints`, not a new read). request-changes
carries a single reason (FR-5 floor, Sam's decision).

## 3. Tests (SPEC-006 DoD 1, 3; NFR-4)

New handler integration tests (real Postgres, mock provider, `httptest`):

- **`TestUIEstimateMutation`** — a valid set updates the roll-up (tokens, tier)
  and records the same `estimate.recorded` audit row as the CLI; cite-corpus
  lifts the tier to considered; a zero/oversized/initiative ref is surfaced
  inline, not as a 500.
- **`TestUIPlanningMutations`** — initiative/feature create; milestone create +
  add member; a **blocked G4 lock surfaces inline (no checkpoint)** and does not
  lock; roadmap create + entry; abandon requires a reason.
- **`TestUIArchiveRaisesCheckpoint`** — a G5-blocked archive raises a checkpoint
  to the inbox (badge → 1) and does **not** archive the initiative (L-6).
- **`TestUIDocumentReview`** — approving an escalated review from the document
  view advances the document via the same path as the inbox respond.

The template render smoke covers the planning envelope and the document fragment
(with and without the review controls). `go vet ./...` and `go test -race ./...`
clean on the dev Postgres.

## 4. The in-session live smoke (DoD 2)

Against a seeded scratch project (initiative `auth`, feature `auth/login`),
served with `server.http`, the command centre was driven in a browser:

- On **Planning**, the Actions panel rendered every mutation form. Entering
  `auth/login` / `2500` in **Set estimate** and submitting swapped the planning
  body with no reload: a green banner "estimate set: auth/login = 2500 tokens
  (rough)", and the tree roll-up updated live to **auth 2.5k [rough, Σ]**,
  **login 2.5k [rough]** — through the sizing engine, not a re-derivation.

The remaining mutations' live behaviour (G4 inline block, G5→inbox, review
approve/request-changes) is exercised by the handler suite against real
Postgres; a provider-driven review smoke (watching a real escalation cleared
from the document view) remains available for Sam to confirm, as in phases 2–4.

## 5. Notes and decisions carried out of the build

- **Actions-panel UX.** The first mutation slice uses a ref-based Actions panel
  (you name `auth/login`, as the CLI does), keeping the read tree clean.
  Node-attached inline controls (an "estimate" affordance on each tree row) are
  a later polish, not architecture.
- **Roadmap re-order is a position-set**, not an insert-and-shift
  (`SetRoadmapEntry` upserts on `(roadmap, milestone)`); drag-to-reorder is later
  polish (SPEC-006 §2/§6).
- **request-changes = single reason** (Sam, 2026-07-22); structured per-section
  comments are a later fidelity add (SPEC-006 §6, folded into FR-5).
- **No new authority, no new read.** Every mutation is an existing service
  method; the one lookup the document view needs (the open review checkpoint) is
  a filter over `PendingCheckpoints`.

## 6. Definition of Done

1. **FR acceptance criteria pass in CI (mock provider).** ✓ — the mutation
   handler suite on the dev Postgres.
2. **Live smoke with a human confirming.** Demonstrated in-session (§4); the
   full human-confirmed record is Sam's to sign off.
3. **`go vet` and `go test -race` clean.** ✓
4. **The walkthrough records the session.** ✓ — this document.
5. **Next increment noted or feature-complete declared.** The command centre now
   reads every surface live, answers escalations, and drives every planning and
   review mutation the engine exposes. What remains is **not** command-centre
   scope for its current definition: multi-user auth (CC-6), human document
   commenting (needs a new service method), config editing (CC-5, likely never),
   and velocity metrics (a dashboard addition). The **MCP facet** (SPEC-005) is a
   separate consumer. The command centre is **feature-complete for its approved
   scope**; further work is new specs, not this slice.
