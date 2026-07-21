# DESIGN-007: The Web Command Centre

**Status:** Approved — 2026-07-21 ([REVIEW-004](../reviews/REVIEW-004-phase-4-web-command-centre-package.md)); Sam approved as scoped
**Date:** 2026-07-21
**Parent:** [vision-v1](../vision/vision-v1.md) §7 (web UI), §11 (dashboard),
§14 (web-UI design deferred to a design doc)
**Depends on:** [DEC-001](../decisions/DEC-001-server-language-go.md) (Go, one
binary), [DEC-002](../decisions/DEC-002-postgres-via-supabase.md) (the
`Notifier` boundary for live updates), [DESIGN-002](DESIGN-002-orchestrator.md)
§2–3 (the in-process event bus), and the phase 1–3 HTTP service layer.

## 1. Purpose

The command centre is the vision's **primary human interface** (§7): a
browser surface for planning, monitoring, answering checkpoints, and reviewing
documents. Phases 1–3 built the whole engine it renders — the initiative tree,
the dispatch/cost ledger, the checkpoint inbox, the document lifecycle, and the
planning layer (estimates, milestones, roadmaps) — and exposed every operation
over an HTTP service layer. Nothing new needs to happen in the model for the
command centre to exist; it needs a *rendering*.

This document fixes the architecture of that rendering: how the UI is built and
served, how it stays live, what it may and may not do, and how it maps onto the
operations that already exist. It designs the **mature** command centre (§11's
full dashboard, all planning surfaces, realtime); [SPEC-004](../specs/SPEC-004-phase-4-web-command-centre.md)
scopes the first buildable slice on top of it.

It deliberately does **not** design the visual language (typography, colour,
component styling) beyond the information architecture and interaction model —
that is production work, not architecture.

## 2. The one principle: a command centre, not a workspace

The vision is emphatic (§7): *"The web UI is not an editor. Documents and code
are edited in whatever editor the human prefers, with files in git. The UI is
the command centre, not the workspace."* This is the load-bearing constraint,
and it simplifies everything downstream:

- The UI **reads** documents and **comments/approves** on them; it never edits
  their bodies. Authoring stays in the user's editor, in git.
- The UI **shows** configuration (prices, routing, budgets); it never writes
  `.cromwell/` — that compartment is user-edited files (vision §12).
- The UI **drives** the operations the engine already exposes — answer a
  checkpoint, set an estimate, lock a milestone, start a feature — and no
  others. It gains no new authority (CC-4).

A command centre is therefore **read-heavy and mutation-light**: dense status
surfaces plus a bounded set of discrete actions, each of which already exists
as a gated, audited service operation. That workload shapes the stack choice.

## 3. Architecture: server-rendered HTML in the one binary (CC-1)

The command centre is **server-rendered HTML with HTMX**, served by the
existing `cromwell` binary. Go `html/template` files are embedded via
`embed.FS` (exactly as migrations and the starter pack are), rendered by HTML
handlers, and progressively enhanced with HTMX for partial updates and
Server-Sent Events (§6) for live regions. There is **no SPA, no Node, no npm,
no separate build, and no separate deployment.**

This is the veteran choice *for this system*, against the alternative of a
React/Svelte SPA:

- **It matches the workload.** A read-heavy, form-light command centre that is
  "not an editor" is precisely what server-rendered HTML with HTMX does best.
  The interactivity the vision actually asks for — live status regions, a
  checkpoint inbox, planning forms, an initiative tree — is fragment swaps and
  event-driven region updates, not a stateful client application.
- **It preserves the single-binary story (DEC-001).** Cromwell is one Go
  binary that already serves HTTP. The UI becomes more templates and handlers
  in that binary, shipped and versioned with it — no second project, no build
  pipeline, no deploy skew between API and UI.
- **It ages well.** There is no client framework to hold a version, no
  transpile step, no `node_modules`. The dependency surface stays inside Go's
  module graph. Where a view genuinely needs richer client behaviour (a
  draggable roadmap, a live chart), it is added as a small, self-contained
  progressive enhancement — a local cost, not a global stack.

The server already supports a TCP listener (`server.http` in `config.yaml`,
distinct from the unix socket the CLI uses); the command centre binds there.
The socket remains the CLI's transport; the UI and the JSON API share the TCP
listener.

## 4. Two renderings over one service layer (CC-2)

The phase 1–3 handlers marshal store/service results to JSON for the CLI. The
command centre is a **second rendering of the same service layer**, not a
client of the JSON API:

```
            ┌────────────── one service layer (server pkg + store) ──────────────┐
  CLI  ───▶ │  /api/*   JSON handlers  ─┐                                          │
            │                            ├─▶  featureByPath, RollUp, LiveProgress, │
  Browser ▶ │  /ui/*    HTML handlers  ─┘     LockMilestone, RespondCheckpoint …   │
            └────────────────────────────────────────────────────────────────────┘
```

HTML handlers call the same store functions and server methods the JSON
handlers do (`store.InitiativeSizingNode`, `store.LiveProgress`,
`store.RespondCheckpoint` + the `CheckpointResponded` publish, …) and render
templates. They do **not** issue HTTP
calls back to `/api/*` — self-HTTP would double the request path, blur error
handling, and couple the UI to the wire format. The JSON API stays as the
programmatic surface (CLI today, MCP facet in SPEC-005); the HTML surface is
the human one. Shared read logic that both renderings need (e.g. assembling a
dashboard summary) lives in one service function returning a struct, rendered
as JSON or HTML at the edge.

Mutations from the UI post to `/ui/*` handlers that call the same transactional
service methods (each already audit-in-transaction, O-3) and return the updated
fragment. No mutation path exists in the UI that does not already exist as a
service operation (CC-4).

## 5. Information architecture and navigation

Five top-level views, from the vision's §7 and §11 lists, reachable from a
persistent left nav. Each maps to service operations that already exist
(§8 catalogue).

1. **Dashboard** (home) — the situational picture (§11): the live queue
   (in-flight, queued, escalated), the recent event stream (audit log), cost
   burn with budget warnings, and calibration health (recent estimate-vs-actual
   deltas). Velocity (features/week, tokens/feature, escalation rate) is a
   later addition; the ledger already holds its inputs.
2. **Planning** — the initiative → feature → task tree, with each node's
   estimate roll-up (tokens, tier, the `?` for unestimated work); milestone
   composition and live/locked progress; roadmap ordering. This is where the
   planning layer (phase 3) becomes visible and driveable.
3. **Inbox** — pending checkpoints with their context and options, and the
   respond action. This is the escalation-only human gate (vision §6) made
   immediate: a badge that lights, a queue that clears.
4. **Documents** — the documents owned across the tree, each with its
   lifecycle state, its rendered body (read-only), and its comment thread;
   the human review actions where a document is human-gated (approve /
   request changes on an escalated review).
5. **Cost** — the roll-ups extended in phase 3: per initiative, feature,
   milestone, roadmap, and month, over the frozen-price ledger.

Navigation is a left rail (the five views) plus breadcrumbs into the
initiative tree. Deep links are real URLs (`/ui/initiatives/auth/login`), so
the browser's history and address bar work; HTMX swaps the main panel for
in-view navigation without a full reload, but every state has a URL.

## 6. Staying live: SSE behind the `Notifier` boundary (CC-3)

The vision's "live-updated via realtime push" (§7) is served by **Server-Sent
Events**, fed by the in-process event bus (DESIGN-002 §2), behind the
`Notifier` interface DEC-002 mandates:

```go
// Notifier is the UI's realtime boundary. The single-server deployment wraps
// the in-process bus; a hosted multi-process deployment can implement it over
// Supabase Realtime with no change above this line (DEC-002).
type Notifier interface {
    Subscribe(ctx context.Context) (<-chan Event, func())
}
```

A `/ui/events` SSE handler subscribes and streams typed events
(`dispatch.succeeded`, `document.transition`, `feature.started`,
`checkpoint.responded`, …) to the browser. One gap is worth naming: checkpoint
*creation* currently emits no in-process event (the bus has
`checkpoint.responded` but no "raised"; the `pg_notify` trigger fires only on
checkpoint *state changes*). The live inbox needs to light the moment a
checkpoint is raised, so this slice adds a lightweight **checkpoint-raised**
signal into the hub — a small additive emission at the `store.CreateCheckpoint`
call site, not a schema change (§8, CC-7). HTMX's SSE extension binds live regions — the queue, the inbox badge,
the event stream, the cost figure — to those event kinds, re-fetching or
swapping the affected fragment when a relevant event arrives.

The bus stays exactly as it is: **many producers, one consumer** — the
orchestrator (DESIGN-002 §3). A second reader on the same channel would *steal*
events from the orchestrator, so the SSE source is **not** a second bus
consumer. Instead, a small **broadcast hub** fans events out to the connected
browsers, and the orchestrator — the bus's sole consumer — forwards each event
to the hub as it processes it. The hub holds one buffered channel per SSE
connection and delivers non-blocking (a slow or dead client is dropped, never
back-pressures the orchestrator). The `Notifier` interface *is* this hub's
subscribe seam:

```
producers ─▶ bus ─▶ orchestrator.handle(ev) ─▶ hub.Broadcast(ev) ─▶ Notifier.Subscribe ─▶ /ui/events (SSE) ─▶ browser
                         (sole consumer)          (fan-out)
```

The single-server hub is the primary source (the `pg_notify` mirror remains the
cross-process safety net). Supabase Realtime is a *different `Notifier`
implementation* for a future hosted, multi-process topology, subscribing to DB
changes instead of the in-process hub — the UI templates and handlers above the
interface do not change (DEC-002's "isolated at the UI boundary").

Live updates are **presentation only**. An SSE message never carries authority
or mutates state; it tells the browser "something changed, refresh this
region," and the region re-reads through the normal service path. A dropped or
missed event costs a stale region until the next event or a manual refresh,
never a wrong action.

## 7. Identity and access (CC-6)

Phase 1–3 carry the actor in the `X-Cromwell-Actor` header; the CLI sets it.
For the command centre's first cut, the operator is a **single configured
identity** — the browser session carries one actor, set from config — because
the command centre today is a single-operator surface and real multi-user
authentication is an orthogonal concern that should not gate the planning and
monitoring value. Multi-user auth (accounts, per-user identity on the audit
trail, authorization) is a **later spec**; the audit log already records an
actor string per event, so adding real identities later is additive, not a
migration. The design keeps the actor a single seam (one place sets it) so that
later work replaces one thing.

## 8. The operation catalogue: what the UI renders and drives

Everything the command centre does already exists as a service operation. The
reads:

| View | Reads (existing) |
|---|---|
| Dashboard | `Store.QueuedDispatches`, running count, `AuditTail`, `CostRollup`/`CostByMonth`, estimate-vs-actual per recent entity |
| Planning | `InitiativeBySlugPath`, `InitiativeSizingNode`+`sizing.RollUp`, `ListMilestones`/`LiveProgress`/`SnapshotProgress`, `RoadmapEntries` |
| Inbox | `PendingCheckpoints` |
| Documents | `CurrentDocForOwner`/live-doc list, document body from git, `CommentsForDocument` |
| Cost | `InitiativeCost`/`FeatureCost`/`MilestoneCost`/`RoadmapCost`/`CostByMonth` |

The mutations, each an existing gated/audited service operation the UI posts to:

- Planning: create initiative/feature; `estimate set`/`estimate ai`; milestone
  create / add member / remove member (descope) / lock (G4); roadmap create /
  add entry; feature start / abandon; initiative archive (G5 → checkpoint).
- Inbox: respond to a checkpoint (approve / request_changes / override / retry
  / …), which the rule engine already turns into follow-up actions.
- Documents: on a human-gated (escalated) review, approve / request changes —
  the same checkpoint-response path.

**Additive read endpoints the UI needs (CC-7).** The CLI never needed a few
reads a browser does; these are additive over the existing store, no schema
change, and SPEC-004 enumerates them:

- a **document body** read (render an approved/draft doc's Markdown) — today
  only comments are exposed, not the body;
- **aggregate list** views (all live documents; all milestones with progress in
  one call) to avoid N calls from a page;
- a **dashboard summary** assembling queue + cost + calibration in one read;
- a **checkpoint-raised signal** into the broadcast hub at the
  `store.CreateCheckpoint` call site, so the live inbox lights on creation (§6)
  — an event emission, not a read, but likewise additive and no schema change.

## 9. What this document does not cover

- **Visual/production design** — component styling, colour, typography, exact
  layout. This design fixes information architecture and interaction; the look
  is production work within it.
- **Multi-user authentication and authorization** — deferred (CC-6), a later
  spec; the actor seam is preserved for it.
- **The MCP facet** — the read/poke surface for editor assistants is
  [SPEC-005](../specs/), a different rendering of the same service layer with
  its own (narrower) authority. Named here only to fix the boundary: the web UI
  drives mutations; the MCP facet does not.
- **Config editing in the UI** — configuration is user-edited files in
  `.cromwell/` (vision §12); the UI shows it read-only. Making it writable would
  cross the compartment boundary and is out of scope, possibly permanently.
- **Offline/PWA, mobile-first layout, theming** — not architecture; later if
  wanted.

## 10. Decisions recorded here

| # | Decision | Rationale |
|---|---|---|
| CC-1 | Server-rendered HTML + HTMX + SSE, embedded in the one Go binary; no SPA, no Node, no separate deploy | The command centre is read-heavy and form-light ("not an editor"); this matches the workload, preserves the single-binary story (DEC-001), and ages without a client-framework version to hold |
| CC-2 | The UI is a second rendering over the same service layer, not a client of the JSON API; `/api/*` stays for machines, `/ui/*` for humans | One service layer, two edges; avoids self-HTTP, keeps error handling and transactions in one place |
| CC-3 | Realtime via SSE off a broadcast hub the orchestrator (the bus's sole consumer) forwards to — not a second bus consumer — behind the `Notifier` interface; Supabase Realtime is an alternate impl; updates are presentation-only | Preserves the proven many-producers/one-consumer bus (DESIGN-002 §3); DEC-002's isolated UI boundary; a slow client is dropped, never back-pressures the orchestrator; a missed event costs staleness, never a wrong action |
| CC-4 | The UI drives only operations that already exist as gated, audited service methods; it gains no new authority and cannot override gates except through answered checkpoints (L-6) | Prevents the kanbanzai rogue-orchestrator failure (vision §7); the UI is a renderer of the engine, not a second engine |
| CC-5 | The UI is not an editor: it reads documents and comments/approves; it shows config read-only; it never writes `.cromwell/` or document bodies | vision §7 and the three-compartment model (§12); authoring stays in the editor + git |
| CC-6 | Identity is a single configured operator actor for the first cut; multi-user auth is a later spec, kept to one seam | Auth is orthogonal to the planning/monitoring value and should not gate it; the audit actor string makes real identities additive |
| CC-7 | A small set of additive read endpoints (document body, aggregate lists, dashboard summary) is added over the existing store; no schema change | Browsers need reads the CLI did not; additive and enumerated for SPEC-004 |

## 11. Revisions

- 2026-07-21 — initial draft for review.
- 2026-07-21 — approved as scoped (REVIEW-004); the realtime mechanism (CC-3,
  §6) corrected to a broadcast hub after the review found the bus is
  single-consumer.
