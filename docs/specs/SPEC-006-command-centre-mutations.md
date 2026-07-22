# SPEC-006: The Command Centre — the mutation slice (planning & review actions)

**Status:** Approved and binding — 2026-07-22 ([REVIEW-006](../reviews/REVIEW-006-command-centre-mutations-package.md)); approved by Sam as scoped, with request-changes carrying a single reason (FR-5, the §6 floor)
**Date:** 2026-07-22
**Parent design:** [DESIGN-007](../design/DESIGN-007-web-command-centre.md) §4
(mutations post to `/ui/*` handlers calling the same transactional service
methods), §8 (the operation catalogue — every mutation already exists as a
gated, audited service method), CC-4 (no new authority)
**Builds on:** [SPEC-004](SPEC-004-phase-4-web-command-centre.md) SD-1 (the
operational slice: read-everywhere + respond, live) — this is the deferred
second half
**Precondition:** [SPEC-006 entry criteria](../notes/spec-006-entry-criteria.md);
SPEC-004 done
**Vision:** [vision-v1](../vision/vision-v1.md) §7 (the command centre drives the
operations the engine exposes)

## 0. Framing: the command centre's second slice

SPEC-004 shipped the **operational** command centre — watch every surface live
and answer escalations from the browser, the loop that justifies a web UI. It
deliberately deferred every **planning and document-review mutation** (SPEC-004
SD-1): estimates, milestones, roadmaps, the tree lifecycle, and the human
review actions all still require the CLI.

**This spec is that deferred half: the mutation slice.** It renders the forms
for the operations DESIGN-007 §8 already catalogued and the phases 1–3 engine
already exposes as gated, audited service methods, turning the command centre
from "monitor and answer" into "operate." It is a **rendering increment, not new
capability**: nothing here is a new engine operation. Every form posts to a
`/ui/*` handler that calls the identical service method the matching `/api/*`
handler and the CLI already call (CC-2, CC-4).

The one boundary decision for review is the **membership of this slice** (§2):
which mutations it carries, and the two constraints that shape them — that the
UI raises **no new authority** (a blocked gate goes to the inbox as a checkpoint,
never an override, L-6), and that it adds **no new service method** (a mutation
whose service method does not already exist is out of scope by definition).

## 1. Goal

Prove a human can **run planning and review from the browser** — not just watch
and answer, but *drive*: size work, compose and lock milestones, order roadmaps,
move the initiative tree, and clear an escalated document review — without
touching the CLI, and with every action landing on the same audited,
gate-honouring path the CLI produces.

Concretely, the claims this slice demonstrates:

1. **Planning is driveable from the browser.** Set and AI-estimate work; create
   milestones, add and descope members, and lock (G4); create roadmaps and order
   their entries — each posting to the existing service method, each audited, the
   result reflected live.
2. **The tree lifecycle is driveable.** Create initiatives and features; start a
   feature (worktree + task dispatch) and abandon one; archive an initiative —
   with G5 routing a block to the inbox rather than forcing it.
3. **Review closes from the document view.** On an escalated review, approve or
   request changes from the document page, through the same
   checkpoint-response path the inbox uses — a second surface to one authority,
   not a second authority.

The slice is complete when an operator can take a seeded project from
unestimated work to a locked milestone on an ordered roadmap, start and manage
features, and clear an escalated review — against a real project, with no CLI.

## 2. Scope

### In scope — the mutations (each an existing service method, DESIGN-007 §8)

- **Estimates** (FR-1): `estimate set` (unit estimate, optional rationale,
  optional corpus citation) and `estimate ai` (dispatch the estimator; async),
  from the Planning tree.
- **Milestones** (FR-2): create; add member; remove member (descope); lock (G4).
- **Roadmaps** (FR-3): create; add entry at a position (and re-order).
- **Tree lifecycle** (FR-4): create initiative; create feature; feature start;
  feature abandon (reason required); initiative archive (G5 → checkpoint).
- **Document review** (FR-5): on a human-gated (escalated) review, approve /
  request changes from the document view — the existing checkpoint-response path.
- **The mutation UX contract** (FR-6): every form posts to `/ui/*`, returns the
  updated fragment, and the SSE hub refreshes affected regions; a service-layer
  rejection (validation, a failed gate, a conflict) is surfaced inline; and a
  gate that blocks routes correctly — a **G5** archive block raises a checkpoint
  to the inbox, while a **G4** lock block surfaces its 409 reason inline for the
  operator to descope (the CLI's path; G4 raises no checkpoint).

### Out of scope (deferred, with destination)

| Deferred | To |
|---|---|
| **Multi-user authentication & authorization** | A later spec (DESIGN-007 CC-6); this slice keeps acting as the single configured operator (`server.ui_actor`) |
| **Config editing in the UI** | Out of scope, possibly permanently (CC-5); config stays user-edited `.cromwell/` files, shown read-only |
| **Human document *commenting*** | No service method exists today (comments are authored by agent reviewers; the API exposes only a read). Adding human commenting is a new authority — its own increment, not this slice |
| **Document body editing** | Never — the UI is not an editor (CC-5); authoring stays in the editor + git |
| **MCP facet** | [SPEC-005](../specs/) — a different rendering with narrower authority |
| **Mutation polish** — command palette, bulk actions, drag-to-reorder roadmaps, undo | Production interaction work within this information architecture |

### Scope decisions

- **SD-1: Mutations only; the read surfaces are unchanged.** SPEC-004 built and
  proved every read and the respond path; this slice adds the remaining write
  forms over them. It does not revisit the reads.
- **SD-2: No new service operation, no new authority (inherits CC-4, L-6).**
  Every UI mutation is an existing gated, audited service method (each
  audit-in-transaction, O-3). A mutation whose method does not already exist is
  out of scope by definition — this is the membership test for the slice, not a
  gap to fill here. In particular the UI never overrides a gate, and honours each
  gate's actual shape: a blocked **G5** archive raises a checkpoint to the inbox;
  a blocked **G4** lock returns its 409 reason for the operator to descope
  (G4 raises no checkpoint) — neither is ever forced, exactly as the CLI does.
- **SD-3: Single operator identity (inherits CC-6, SPEC-004 SD-4).** All
  mutations act as `server.ui_actor` on the audit trail; real auth is a later
  spec, kept to the one seam. The mutation surface makes "who did this" matter
  more, which may raise auth's priority — noted, not addressed here.
- **SD-4: Document review is a second surface to the respond path, not a new
  action (inherits CC-4).** Approve / request-changes from the document view
  build the same `EscalationResponse` the inbox respond builds and post to the
  same `RespondCheckpoint` + `CheckpointResponded` path. The document view shows
  the action only when the document's review is human-gated (an open
  review-escalation checkpoint refs it).
- **SD-5: Realtime stays presentation-only (inherits SPEC-004 SD-5).** A mutation
  commits through its service method and returns the updated fragment
  synchronously; the SSE refresh of *other* regions is the same coarse,
  authority-free signal SPEC-004 established. A missed event costs staleness,
  never a lost or double mutation (idempotency and audit live in the service
  method, not the UI).

## 3. Requirements

Each FR names the existing service method / endpoint it renders, so the slice is
a form over a known operation, not a discovery.

### FR-1: Estimates from the Planning view

- **FR-1.1** From a feature or task node, the operator sets a unit estimate
  (tokens, optional rationale, optional "cite corpus"), posting to the same
  service path as `POST /api/estimate/set`; the node's roll-up re-renders through
  the sizing engine (SPEC-004 FR-4.1), and the new estimate is on the audit
  trail.
  *AC:* setting an estimate updates the rendered roll-up (tokens, tier) to match
  `sizing.RollUp` over `InitiativeSizingNode`, and records an
  `estimate.recorded` audit row.
- **FR-1.2** The operator dispatches the AI estimator for a node
  (`POST /api/estimate/ai`, async → 202); the node shows a pending state, and the
  estimate appears live when the dispatch completes (via the SSE refresh), tiered
  `considered` or `rough` from the corpus — never fabricated by the UI.
  *AC:* dispatching the estimator shows pending, then renders the returned
  estimate and tier when the dispatch succeeds, with no manual reload.

### FR-2: Milestones

- **FR-2.1** Create a milestone (name, optional target date), add a member
  (initiative transitive / feature / nested milestone), and remove a member
  (descope), posting to the `POST /api/milestones` / `/members` service paths;
  the milestone's live progress (SPEC-004 FR-4.2) re-renders.
  *AC:* creating, adding, and removing members change the rendered live progress
  to match `LiveProgress`, each with its audit row.
- **FR-2.2** Lock a milestone (G4). The lock succeeds only when the gate allows
  it (`POST /api/milestones/lock` → 200) and snapshots the resolved leaves; when
  the gate blocks (a member not done), the UI surfaces the block and the required
  descope inline rather than forcing the lock (the CLI's 409 path).
  *AC:* locking an eligible milestone renders it locked with snapshot progress; a
  blocked lock surfaces the reason and does not lock (no override, L-6).

### FR-3: Roadmaps

- **FR-3.1** Create a roadmap and add a milestone entry at a position
  (`POST /api/roadmaps` / `/entries`); the roadmap re-renders its entries in
  position order (SPEC-004 FR-4.2). Re-ordering an entry updates positions.
  *AC:* adding and re-ordering entries renders them in the new position order,
  matching `RoadmapEntries`, each audited.

### FR-4: Tree lifecycle

- **FR-4.1** Create an initiative (optional parent) and a feature under an
  initiative (`POST /api/initiatives` / `/features`); the Planning tree re-renders
  with the new node.
  *AC:* creating a node renders it in the tree with an empty (`?`) roll-up.
- **FR-4.2** Start a feature (`POST /api/features/start`): ready → active, the
  worktree is created, and initially-ready tasks dispatch; the dashboard queue
  reflects the new dispatches live. Abandon a feature
  (`POST /api/features/abandon`, reason required).
  *AC:* starting a feature transitions it to active and the queue shows its
  dispatches; abandoning requires a reason and transitions the feature.
- **FR-4.3** Archive an initiative (`POST /api/initiatives/archive`). When G5
  passes, the initiative archives; when it blocks (non-terminal features), a
  gate-override checkpoint is raised to the inbox and the UI reflects that — it
  never archives without the answered checkpoint (L-6, the SPEC-004 smoke path).
  *AC:* a blocked archive raises a checkpoint visible in the inbox and does not
  archive; answering `override` archives it (the existing respond path).

### FR-5: Document review from the document view

- **FR-5.1** When a document's review is human-gated (an open review-escalation
  checkpoint refs it), the document view (SPEC-004 FR-5.1) offers **approve** and
  **request changes**; both build the same `EscalationResponse` and post to the
  same `RespondCheckpoint` + `CheckpointResponded` path the inbox respond uses
  (SD-4). No such control appears when the document is not human-gated.
  *AC:* approving from the document view produces the same audited outcome as
  answering the checkpoint in the inbox (the document advances / returns for
  changes), and the follow-up fires; a non-escalated document shows no review
  control.

### FR-6: The mutation UX contract

- **FR-6.1** Every mutation posts to a `/ui/*` handler that calls the existing
  transactional service method (never self-HTTP to `/api/*`, NFR-2) and returns
  the updated fragment, so the acting region reflects the change without a full
  reload; the SSE hub refreshes other affected regions (SD-5).
  *AC:* a mutation's own region updates from its POST response; a second open
  view refreshes via SSE within one event.
- **FR-6.2** A service-layer rejection — a validation error, a failed gate, a
  409 conflict — is surfaced inline at the form, in the operator's words, without
  losing their input; it is never swallowed or shown as a raw 500.
  *AC:* a blocked G4 lock, an abandon without a reason, and a duplicate-slug
  create each render an inline, legible error and leave state unchanged.

## 4. Non-functional requirements

- **NFR-1** No new stack (inherits SPEC-004 NFR-1): forms are HTML + HTMX over
  the embedded templates; no Node, no bundler. `go vet ./...` and
  `go test -race ./...` stay clean.
- **NFR-2** Each mutation is a second rendering over the existing service method,
  not a client of `/api/*` (CC-2): no HTML handler issues HTTP to the JSON API.
- **NFR-3** No operation that is not an existing gated, audited service method
  (CC-4, SD-2); every mutation is audit-in-transaction (O-3); no gate override
  except through an answered checkpoint (L-6).
- **NFR-4** Handler tests drive the HTML mutation handlers directly (httptest,
  mock provider, real Postgres), asserting the audited outcome equals the CLI's
  and that a blocked gate raises a checkpoint rather than forcing the action — as
  the SPEC-004 suite does. A manual browser smoke covers the live experience.
- **NFR-5** No secrets in rendered forms, URLs, or errors (inherited); a
  service-layer error is surfaced without leaking internal detail that could
  carry secrets.

## 5. Definition of Done

1. All FR acceptance criteria pass in CI (mock provider) on plain Postgres and
   the Supabase local stack.
2. A live smoke extends the record: from the browser, take a seeded project from
   unestimated work → set/AI estimates → a milestone locked (G4) → an ordered
   roadmap → a feature started → an escalated review cleared — with a human
   confirming each mutation lands audited and gate-honouring. Recorded in a
   walkthrough.
3. `go vet ./...` and `go test -race ./...` clean.
4. The walkthrough records the session.
5. If a command-centre increment remains after this slice (e.g. auth, human
   commenting, velocity metrics), its entry criteria are noted; otherwise the
   command centre is declared feature-complete for its current scope.

## 6. Open questions carried into implementation

- **request-changes comments — decided (2026-07-22, Sam): a single reason.**
  Request-changes from the UI carries one free-text reason on the
  `EscalationResponse` (the honest floor); structured per-section comments are a
  later fidelity add, not this slice. FR-5.1 reads accordingly.
- **Estimator-pending UX.** `estimate ai` is async; how the pending node reads
  and how failure (an exhausted estimator dispatch) surfaces — inline, or via the
  inbox as any dispatch failure does.
- **Optimistic vs confirmed feedback.** How much the acting region shows before
  the SSE event round-trips (a tuning within SD-5, not a contract).
- **Roadmap re-ordering interaction.** Position edits via a control now;
  drag-to-reorder is later polish (out of scope §2), but the handler shape should
  not preclude it.
- **Whether feature/initiative *create* belongs here or reads as planning setup.**
  Create is a light mutation; if it complicates the slice, it can move to the
  same increment as human commenting. A membership tuning, flagged for the review.
