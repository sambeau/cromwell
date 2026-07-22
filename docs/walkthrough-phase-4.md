# Phase-4 Session Record — The Web Command Centre (operational slice)

**Status:** Implemented and green — satisfies SPEC-004 Definition of Done items
1, 3, 4, and 5; item 2 (the human-confirmed live smoke) demonstrated in-session
against a seeded project and open for Sam's confirmation.
**Date:** 2026-07-22
**Operator:** Sam (commands run by Claude on Sam's behalf)
**Automated-test database:** dedicated dev Postgres (`cromwell-pg-dev`, 54329)
**In-session smoke database:** a scratch Postgres DB on the same dev container
**Provider:** none required — this slice renders existing state and drives the
realtime loop from orchestrator events, so the smoke needs no LLM.

This records the first command-centre slice ([SPEC-004](specs/SPEC-004-phase-4-web-command-centre.md),
[DESIGN-007](design/DESIGN-007-web-command-centre.md)): the **operational**
command centre — read every surface live and answer escalations from the
browser, with no CLI. It is server-rendered HTML + HTMX + SSE embedded in the
one Go binary, a second rendering over the same service layer as `/api/*`, with
realtime via a broadcast hub behind the `Notifier` boundary.

## 1. What the slice proves (SPEC-004 §1)

1. **The command centre is live.** A browser shows the queue, the event stream,
   cost burn against budget, and calibration health, and they update by realtime
   push — no manual refresh.
2. **The escalation loop closes in the browser.** A checkpoint appears the
   moment it is raised; the human reads its context and answers; the
   orchestrator resumes — all without leaving the page.
3. **Every surface is visible.** The initiative → feature → task tree with
   honest worst-tier roll-ups, milestones with progress, roadmaps in order,
   documents with rendered bodies and comments, and cost at every level.

## 2. How it was built (build order, infrastructure first)

The infrastructure the views hang off came first, tests-first where there was
pure logic — exactly as phases 1–3 began with their pure cores.

1. **The SSE broadcast hub** (`internal/notify`) — the `Notifier` interface and
   a `Hub` that fans events to per-connection buffered channels, non-blocking
   (a slow browser is dropped, never back-pressures the orchestrator). Unit
   tested for delivery, unsubscribe, context cancellation, and the
   slow-subscriber drop (CC-3, SD-5). The orchestrator — the bus's **sole**
   consumer — forwards each processed event to the hub; the hub is never a
   second bus reader (the review's correction to DESIGN-007 §6).
2. **The additive read layer** (`internal/store/ui_reads.go`,
   `internal/server/ui_service.go`) — root/child initiatives, the live-document
   list, milestones-with-progress, recent-calibration, running dispatches, and
   the assembled `DashboardSummary` — all additive over the existing tables, no
   schema change (CC-7, FR-8.1). Plus a small dependency-free Markdown renderer
   for the document read view, escaping-tested against injection (NFR-5).
3. **The checkpoint-raised signal** (FR-8.2) — checkpoint creation emits no
   workflow event, so a presentation-only `checkpoint.raised` is broadcast
   straight to the hub at every `store.CreateCheckpoint` call site (the
   rules-driven funnel, the direct HTTP/worktree/phase-2 sites, and the
   dispatcher's budget checkpoint via an optional hook). The live inbox lights
   on creation.
4. **The UI shell + serving** (`internal/server/ui.go`, `ui/templates`,
   vendored `ui/static`) — `embed.FS` templates, a persistent left rail over the
   five views, real per-view URLs with HTMX `hx-boost` navigation, vendored
   HTMX 2.0.4 + its SSE extension, and the dual-listen change: the socket is
   always the CLI's transport, and the TCP listener (`server.http`), when
   configured, additionally serves the JSON API and the UI (FR-1.1).
5. **The five views + the one mutation** — Dashboard, Inbox (respond), Planning
   (read), Documents (read), Cost (read), each a template over an existing
   service read. Respond maps the UI's verb to the kind's structured response
   payload and posts to the same `RespondCheckpoint` + `CheckpointResponded`
   path the CLI uses — no new authority (CC-4).

## 3. Tests (SPEC-004 DoD 1, 3; NFR-4)

- **`internal/notify`** — hub unit tests (delivery, unsubscribe, cancellation,
  non-blocking slow-subscriber drop).
- **`internal/server` template render test** — every page and fragment rendered
  with representative data, so a template bug is a fast unit failure.
- **`internal/server` markdown tests** — structure plus the load-bearing
  security assertions: HTML is escaped, `javascript:` links are rejected.
- **`internal/server` UI handler integration suite** (real Postgres, mock
  provider, `httptest`): all five views render on direct load (FR-1.2, FR-2,
  FR-4, FR-5, FR-6); the document body renders with no edit control (FR-5.1,
  CC-5); a raised checkpoint appears in the inbox, the badge counts it, and a UI
  respond drives the same path as the CLI and clears it (FR-3); the SSE stream
  delivers a `checkpoint.raised` signal live (FR-7, FR-8.2); the vendored assets
  are served (CC-1).
- `go vet ./...` and `go test -race ./...` clean on the dev Postgres.

## 4. The in-session live smoke (DoD 2)

A scratch project was seeded via the CLI (two initiatives, three features, two
unit estimates, a milestone, a roadmap, and a registered spec), the server was
started with `server.http: 127.0.0.1:8799`, and the command centre was driven in
a browser:

- **Dashboard** rendered the queue (idle), cost against the $50 monthly cap, the
  live audit stream, and calibration — all from the store reads.
- **Planning** rendered the tree with honest roll-ups: `auth` at **9.2k
  [rough, decomposed]** — the worst-tier taint from its `rough` leaves, Σ
  marking the arithmetic sum — while `billing` showed **?** with its unestimated
  feature named. Milestone `v1` (open, 0/2) and roadmap `2026` rendered in order.
- **Documents** rendered the spec's Markdown body (headings, bold, inline code,
  a safe link, lists) with the front matter stripped, the comment thread, and
  **no edit control**.
- **Cost** rendered per-initiative, per-feature, per-milestone, per-roadmap, and
  per-month roll-ups.
- **The realtime loop, end to end:** with the Inbox open and untouched, a
  `cromwell initiative archive billing` from the CLI hit gate G5, raised a
  gate-override checkpoint, and — with no reload — the SSE push lit the inbox
  badge to **1** and rendered the checkpoint card ("raised 0s ago"). Clicking
  **override** in the browser cleared the inbox live; the audit trail records
  `checkpoint.responded {override:true}` by `sam@smoke`, then `initiative.archived`
  by `sam@smoke`, and `billing.archived = t`. Watch → get pulled in → answer →
  resume, entirely from the browser, with the operator identity on the trail.

The gate-override path exercises the whole realtime + respond loop without an
LLM. A provider-driven smoke (watching a real agent review escalate live, as in
phases 2–3) remains available for Sam to confirm against a Supabase + DeepSeek
scratch project — the mechanism is identical; only the event source differs.

## 5. Decisions and notes carried out of the build

- **Dual-listen consequence.** Now that the socket is always bound (the CLI's
  transport) alongside the optional TCP listener, a project on a deep path must
  set a short `server.socket` (the macOS ~104-byte `sun_path` limit — the
  phase-3 handoff's gotcha, now load-bearing whenever `server.http` is set).
- **Vendored HTMX, per CC-1.** HTMX 2.0.4 and its SSE extension are vendored
  under `internal/server/ui/static` and embedded; no Node, no build step.
- **Realtime is coarse by design.** The SSE handler emits one generic `changed`
  event carrying the event kind; live regions re-read their fragment through the
  normal service path on it (the "something changed, refresh the panel" option
  SPEC-004 §6 allows). Targeted per-kind fragment swaps are a later tuning, not a
  contract change.
- **Respond verb → schema mapping.** The rule engine reads a per-kind structured
  payload (`decision` / `override` / `retry` / `continue`), not a uniform
  answer; `responseFor` is the single place the UI's verbs become that schema,
  so the browser drives the identical audited path the CLI does.

## 6. Definition of Done

1. **FR acceptance criteria pass in CI (mock provider).** ✓ — the UI handler
   suite on the dev Postgres.
2. **Live smoke with a human confirming.** Demonstrated in-session (§4); the
   human-confirmed record is Sam's to sign off.
3. **`go vet` and `go test -race` clean.** ✓
4. **The walkthrough records the session.** ✓ — this document.
5. **Entry criteria for the next slice drafted.** ✓ —
   [spec-005 entry criteria](notes/spec-005-entry-criteria.md) (the
   planning/review mutation slice).
