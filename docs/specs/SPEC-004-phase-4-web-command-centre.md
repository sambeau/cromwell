# SPEC-004: Phase 4 — The Web Command Centre (operational slice)

**Status:** Approved and binding — 2026-07-21 ([REVIEW-004](../reviews/REVIEW-004-phase-4-web-command-centre-package.md)); the SD-1 slice (read-everywhere + respond) approved by Sam as scoped
**Date:** 2026-07-21
**Parent design:** [DESIGN-007](../design/DESIGN-007-web-command-centre.md)
(the command-centre architecture: server-rendered HTMX in the one binary, the
`Notifier`/SSE realtime boundary, the two-renderings-one-service-layer model)
**Vision:** [vision-v1](../vision/vision-v1.md) §7 (web UI), §11 (dashboard)
**Precondition:** [SPEC-004 entry criteria](../notes/spec-004-entry-criteria.md);
phases 1–3 complete and merged

## 0. Framing: the command centre in slices

The command centre (vision §7) is the primary human surface: dashboards, the
checkpoint inbox, planning surfaces, document review, and configuration. That
is a large surface — five views, realtime, and a set of mutations — and a
veteran team would not build it as one undifferentiated push, exactly as
phases 1–3 were sliced.

**This spec is the first command-centre slice: the *operational* command
centre** — read every surface, live, and drive the one interaction that is the
command centre's reason to exist: **answering escalations**. Concretely: a live
dashboard, a live checkpoint inbox with respond, and read-only planning,
documents, and cost views, all updated in realtime.

It is chosen first because it delivers a *complete, load-bearing capability* on
its own — **"operate Cromwell from the browser: watch the whole system live,
get pulled in only on escalation, answer, watch it resume"** — which is the
precise value the vision claims for the web UI (§7's "why this fixes the
human-gate latency problem"). Every read across all five views is already
built; the respond loop is already built. This slice renders them.

**The central scope decision for review is the slice boundary**: this slice is
*read-everywhere + respond*. The **planning and document-review mutations**
(estimate set/ai, milestone create/lock, roadmap add, document approve/request-
changes from the UI) are deferred to the next command-centre slice (§2), even
though their engine and CLI already exist. That is a focus decision, not a
capability gap — those mutations are fully driveable from the CLI today, and
their UI forms are a coherent second increment. If the reviewer would rather
the first slice also carry the planning mutations (making it *read-everywhere +
all-mutations*), that is a one-line redirect here and this spec absorbs FR-8's
deferral list.

## 1. Goal

Prove that a human can **run Cromwell from the browser** without touching the
CLI: see the whole system's live state, and drive the escalation-only human
loop. Concretely, the claims this slice demonstrates:

1. **The command centre is live.** A browser shows the queue, the event
   stream, cost burn, and calibration health, and they update by realtime push
   as the orchestrator works — no manual refresh (vision §7).
2. **The escalation loop closes in the browser.** A checkpoint appears in the
   inbox the moment it is raised; the human reads its context and responds
   (approve, request changes, override, retry, …); the orchestrator resumes —
   all without leaving the page (vision §6).
3. **Every surface is visible.** The initiative → feature → task tree with
   honest estimate roll-ups, milestones with progress, roadmaps, documents with
   their bodies and comment threads, and cost at every level are all readable
   in the browser, rendering the phases 1–3 engine faithfully.

Phase 4 (this slice) is complete when an operator can open the command centre,
watch a document review or an implementation loop run live, answer a checkpoint
it raises, and read the resulting planning and cost state — against a real
project, with no CLI.

## 2. Scope

### In scope

- **The UI shell** (DESIGN-007 §3–5): server-rendered HTML + HTMX embedded in
  the `cromwell` binary, served on the TCP listener; a persistent left nav over
  the five views; real per-view URLs with browser history.
- **Dashboard** (vision §11): live queue (in-flight / queued / escalated),
  recent event stream (audit log), cost burn with budget warnings, calibration
  health (recent estimate-vs-actual deltas).
- **Inbox**: pending checkpoints with context and options; **respond** (every
  answer kind the rule engine accepts); a live badge/count.
- **Planning (read)**: the initiative → feature → task tree with each node's
  estimate roll-up (tokens, tier, the `?` for unestimated work); milestones with
  live/locked progress; roadmaps in order.
- **Documents (read)**: the documents across the tree with lifecycle state, the
  rendered (read-only) body, and the comment thread.
- **Cost (read)**: roll-ups per initiative, feature, milestone, roadmap, and
  month.
- **Realtime** (DESIGN-007 §6): SSE behind the `Notifier` interface, fed by the
  in-process bus; live regions refresh on relevant event kinds.
- **Additive read endpoints** (DESIGN-007 §8, CC-7): document body, aggregate
  list views, and a dashboard summary — additive over the existing store, no
  schema change.

### Out of scope (deferred, with destination)

| Deferred | To |
|---|---|
| **Planning & review mutations in the UI** (estimate set/ai, milestone create/add/remove/lock, roadmap create/add, feature start/abandon, initiative archive, document approve/request-changes) | The next command-centre slice (a later spec). Engine + CLI already exist; the UI forms are the increment |
| **Multi-user authentication & authorization** | A later spec (DESIGN-007 CC-6); the first cut uses a single configured operator actor |
| **Config editing in the UI** | Out of scope, possibly permanently — config is user-edited `.cromwell/` files (vision §12); the UI shows it read-only |
| **MCP facet** (read/poke for editor assistants) | SPEC-005 — a different rendering of the same service layer |
| **Visual/production design** (styling, colour, theming, mobile-first, PWA) | Production work within this information architecture; not architecture |
| **Velocity metrics** (features/week, tokens/feature, escalation rate) | A dashboard addition once the operational slice lands; inputs already in the ledger |

### Scope decisions

- **SD-1: The first slice is read-everywhere + respond, not all-mutations.**
  The command centre's defining loop is watch → get-pulled-in → answer →
  resume; that is fully delivered by live reads plus the checkpoint respond
  path, both already built. Planning/review mutations are a coherent second
  increment and remain CLI-driveable meanwhile. This is the §0 decision.
- **SD-2: Server-rendered, one binary (inherits DESIGN-007 CC-1).** No SPA, no
  Node, no separate deploy. HTML + HTMX + SSE via `embed.FS`.
- **SD-3: The UI drives no new authority (inherits CC-4).** Respond is the only
  mutation, and it is the existing checkpoint-response service path; the UI
  cannot override a gate except through an answered checkpoint (L-6).
- **SD-4: Single configured operator identity (inherits CC-6).** One actor for
  the browser session; real auth is a later spec, kept to one seam.
- **SD-5: Realtime is presentation-only (inherits CC-3).** An SSE message
  triggers a region refresh through the normal service path; it never carries
  authority or mutates state. A missed event costs staleness, never a wrong
  action.

## 3. Requirements

### FR-1: The UI shell and navigation

- **FR-1.1** The `cromwell` binary serves the command centre on its configured
  TCP listener (`server.http`); with no TCP listener configured the UI is
  simply unavailable and the CLI/socket is unaffected.
  *AC:* with `server.http` set, a browser loads the command centre; unset, the
  server still runs for the CLI and the UI port is closed.
- **FR-1.2** A persistent left nav reaches all five views; each view has a real
  URL and works on direct load and via browser back/forward; in-view navigation
  swaps the main panel without a full reload but every state has a URL.
  *AC:* visiting `/ui/…` for each view directly renders it; back/forward
  restores prior views.

### FR-2: Dashboard

- **FR-2.1** The dashboard shows the live queue (in-flight, queued with reason,
  escalated), the recent event stream from the audit log, cost burn with the
  budget cap and warning threshold, and calibration health (recent
  estimate-vs-actual deltas).
  *AC:* against a seeded project the dashboard's figures match the underlying
  store reads (queue = `QueuedDispatches`, cost = `CostRollup` total, etc.).

### FR-3: Inbox and respond

- **FR-3.1** The inbox lists pending checkpoints with kind, question, context,
  and age; a live count/badge reflects the pending total.
  *AC:* raising a checkpoint makes it appear in the inbox; answering it removes
  it and decrements the badge.
- **FR-3.2** The human can respond to a checkpoint with any answer the rule
  engine accepts (approve, request_changes, override, deny, retry, cancel,
  proceed, continue, pause, or a free answer), with an optional reason; the
  response feeds the same `CheckpointResponded` path the CLI uses, and the
  orchestrator resumes.
  *AC:* responding in the UI produces the same audited outcome as the CLI
  `respond`, and the follow-up action fires.

### FR-4: Planning (read)

- **FR-4.1** The initiative → feature → task tree renders, each node showing its
  estimate roll-up: total tokens, tier, and — where incomplete — the `?` and the
  named unestimated descendants (SPEC-003 FR-2), computed through the sizing
  engine, not re-derived.
  *AC:* a seeded tree's rendered roll-up matches `sizing.RollUp` over
  `InitiativeSizingNode`.
- **FR-4.2** Milestones render with live progress (open) or snapshot progress
  (locked) and their resolved members; roadmaps render their milestones in
  order.
  *AC:* a locked milestone shows its frozen snapshot progress; a roadmap shows
  its entries in position order.

### FR-5: Documents (read)

- **FR-5.1** Documents across the tree render with lifecycle state, the
  read-only rendered body (from git via the additive body endpoint), and the
  comment thread; no edit control is present (DESIGN-007 CC-5).
  *AC:* a registered document shows its current body and its comments; there is
  no way to edit the body from the UI.

### FR-6: Cost (read)

- **FR-6.1** Cost roll-ups render per initiative (transitive), feature,
  milestone (resolved members), roadmap, and month, from the frozen-price
  ledger (SPEC-003 FR-7).
  *AC:* the rendered figures equal the corresponding `*Cost`/`CostByMonth`
  reads for a seeded ledger.

### FR-7: Realtime

- **FR-7.1** A `/ui/events` SSE stream, fed by a broadcast hub the orchestrator
  forwards events to (not a second consumer of the single-consumer bus) behind
  the `Notifier` interface (DESIGN-007 §6), drives live regions: the dashboard
  queue and event stream, the inbox badge, and the cost figure refresh when a
  relevant event arrives, with no manual reload. A slow or disconnected browser
  is dropped by the hub and never back-pressures the orchestrator.
  *AC:* a dispatch completing or a checkpoint being raised updates the relevant
  region in an open page within one event, without a reload.
- **FR-7.2** Realtime is presentation-only: an SSE message triggers a region
  re-read through the normal service path and never mutates state (SD-5).
  *AC:* with the SSE stream disconnected, every view still renders correct
  current state on load and manual refresh.

### FR-8: Additive read endpoints

- **FR-8.1** The store/service layer gains a document-body read, aggregate list
  reads (all live documents; all milestones with progress), and a dashboard
  summary read — additive, no schema change, no change to phase 1–3 tables.
  *AC:* each new read returns correct data and is covered by a handler test.
- **FR-8.2** A lightweight **checkpoint-raised** signal is emitted into the
  broadcast hub at the `store.CreateCheckpoint` call site, so the live inbox
  lights on creation (checkpoint creation emits no in-process event today,
  DESIGN-007 §6) — additive, no schema change.
  *AC:* raising a checkpoint delivers a hub event that updates the open inbox
  within one event, with no reload.

## 4. Non-functional requirements

- **NFR-1** No new stack: no Node, npm, bundler, or separate deployment; the UI
  ships in the one Go binary with templates embedded via `embed.FS` (DESIGN-007
  CC-1). `go vet ./...` and `go test -race ./...` stay clean.
- **NFR-2** The UI is a second rendering over the existing service layer, not a
  client of `/api/*`; no HTML handler issues HTTP to the JSON API (CC-2).
- **NFR-3** The UI performs no operation that is not an existing gated, audited
  service method; no gate override except through an answered checkpoint (CC-4,
  L-6). Every mutation remains audit-in-transaction (O-3).
- **NFR-4** Handler tests drive the HTML handlers and the SSE stream directly
  (httptest), asserting rendered structure and live-region behaviour with the
  mock provider against real Postgres, as the phase 1–3 suites do; a manual
  browser smoke covers the live experience.
- **NFR-5** No secrets in rendered pages, page URLs, or the event stream
  (inherited); the config view redacts secret material and shows only names.
- **NFR-6** The realtime boundary is the `Notifier` interface; the SSE
  implementation is single-server; a Supabase Realtime implementation is
  possible later with no change above the interface (DEC-002).

## 5. Definition of Done

1. All FR acceptance criteria pass in CI (mock provider) on plain Postgres and
   the Supabase local stack.
2. A live smoke extends the record: open the command centre against a real
   project, watch a document review (or an implementation loop) run live,
   answer a checkpoint it raises from the browser, and read the resulting
   planning and cost state — with a human confirming the live updates. Recorded
   in a walkthrough with screenshots.
3. `go vet ./...` and `go test -race ./...` clean.
4. The walkthrough records the session.
5. Entry criteria for the next command-centre slice (the planning/review
   mutations) drafted.

## 6. Open questions carried into implementation

- **Live-region granularity** — which regions refresh on which event kinds, and
  whether a coarse "something changed, refresh the panel" is enough versus
  targeted fragment swaps. An interaction-quality tuning, not a contract.
- **Document body rendering** — Markdown-to-HTML in the browser view: the
  renderer and how much fidelity (headings, tables, code) the read view needs.
- **SSE reconnect and backfill** — how a reconnecting browser catches up on
  events missed while disconnected (a full refresh on reconnect is the honest
  floor; smarter backfill is a refinement).
- **Whether the planning/review mutation slice should be one spec or two**
  (planning mutations vs document-review actions) — decided when that slice is
  scoped, informed by how this one lands.
