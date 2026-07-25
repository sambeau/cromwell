# SPEC-007: The Workflow Surface — Stage A (document-led browsing)

**Status:** **Approved and binding — 2026-07-25** (Sam, after the authoring
consistency review).
**Date:** 2026-07-24 (approved 2026-07-25)
**Parent design:** [DESIGN-008](../design/DESIGN-008-the-workflow-surface.md)
(the workflow surface: thing-first pages, the entity page as its main design
document, tokens as the unit of work, parent-owned milestones and roadmaps, the
two-Markdown-lists display)
**Also depends on:** [DEC-003](../decisions/DEC-003-cli-scope.md) (the CLI
shrinks — this spec supplies the UI capability that must land *before* the CLI
removal), [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) (the chat
agent authors the planning layer over MCP — why creation/description authoring is
primarily a companion MCP concern, §0), and the parts of
[DESIGN-007](../design/DESIGN-007-web-command-centre.md) DESIGN-008 §3 keeps
(server-rendered HTMX in the one binary; renderings over one service layer; the
SSE realtime boundary).
**Vision:** [vision-v1](../vision/vision-v1.md) §2 (document-led), §4 (the flow),
§5 (tokens as the unit)
**Supersedes on landing:** the flat five-view information architecture of
SPEC-004 FR-1/FR-4 and the global Actions panel of SPEC-006. Their service
methods survive unchanged; only their rendering is replaced.
**Precondition:** phases 1–3 and the SPEC-004/006 command centre are built,
merged, and green. This spec rebuilds the human surface over that engine; it
adds no orchestration capability.

---

## A note on prose

Per DESIGN-008 D-6, every string this spec puts in front of a human — page
headings, descriptions, button labels, the plain-words reason a disabled action
gives — is written in full, clear sentences with terms explained. That is a
testable property of the build, not a nicety (NFR-7). This spec, being read by a
human, follows the rule too.

## 0. Framing: the first slice of the redesign

DESIGN-008 replaces the command centre's whole surface, in two stages. **This
spec is Stage A: document-led browsing.** It delivers the load-bearing claim of
the redesign on its own —

> **A person can understand and drive the project by browsing it, page to page,
> without typing an entity's path anywhere.**

— and it is the slice that makes the interface *thing-first* instead of
*verb-first* (DESIGN-008 §2). Stage B (milestone and roadmap *editing*, the
size-roll-up tree, the filterable work list, task detail) is a later spec and is
out of scope here.

**The central boundary for review** is that Stage A makes milestones and
roadmaps **owned and displayed, but not yet editable** in the UI (SD-1). It is a
named deferral with a destination (Stage B), not a gap.

**Companion deliverable.** Titles and descriptions — and, primarily, the
creation of initiatives and features — are authored by the human's **chat agent
through the MCP facet**, per [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md)
(which refines the vision's MCP rule: the chat agent may author the planning
layer, but may not drive development or touch gates). That MCP authoring slice is
a **peer of this UI**, landing alongside it (DESIGN-008 §6a), and is specced
separately. Stage A is not blocked on it — the UI keeps convenience create/edit
actions (§FR-5, "both" surfaces author) — but the two together are what deliver
the planning half.

## 1. Goal

Prove the browse-and-drive claim against a real project:

1. **The project is a browsable place.** Every initiative, feature and document
   is a page at a real, readable URL (`/ui/i/auth/login`), reached by breadcrumb,
   child list, or the nav rail — never by typing a path into a form.
2. **The page is its document.** An entity's page shows its main design
   document, rendered, as the body; its status, size in tokens, relations and
   actions frame that writing.
3. **Actions live on their things.** Setting an estimate, creating a child,
   starting a feature, attaching a document, approving a review — each happens on
   the page of the thing it acts on, with the thing implied by where you are.
4. **Work is measured in tokens; money is absent from the surface.** Every size
   and roll-up is in tokens with its confidence tier; no dollar figure appears on
   any page.

Stage A is done when an operator opens the redesigned surface, browses from the
project down to a feature reading design documents as page bodies, corrects a
description in place, attaches a document, and starts a feature — all by
navigating, with no path typed and no CLI.

## 2. Scope

### In scope

- **The shell and navigation** (DESIGN-008 §2, §5): a persistent nav rail; the
  initiative tree as navigation chrome; breadcrumbs carrying ancestry; real
  per-entity URLs with browser history; HTMX panel swaps for in-view moves.
- **Entity pages** for Project, Initiative, Feature and Document, all sharing the
  one anatomy (DESIGN-008 §5.2): breadcrumbs, description, body, status/size/
  actions rail, and relation sections.
- **The document-as-body rendering** (DESIGN-008 §4, §5.2): the entity's main
  design document rendered read-only via the existing goldmark + bluemonday path
  (`renderMarkdown` / `readDocFile`); main-document selection per Q-B.
- **The description and title** (DESIGN-008 §5.3 tier 1, §5.4): reuse the
  existing `description` column; display description and title as the human
  summary. The primary way they are *set* is the chat agent via an MCP tool
  (SD-2); the UI keeps a light in-place edit as a convenience. **No server-side
  generator** and **no human-edited flag** — there is nothing to guard against
  clobbering, because a human or their chat agent writes them deliberately.
- **Actions relocated onto entity pages** (DESIGN-008 §6): set estimate, AI
  estimate, new sub-initiative, new feature, start feature, abandon feature,
  archive initiative, and document approve / request-changes — each the existing
  gated, audited service method, moved onto the relevant page.
- **Attach a document** (DESIGN-008 §6; DEC-003): a UI action registering a
  Markdown file as an entity's document, over the existing register-document
  service path — the capability that must exist before `cromwell doc add` is
  removed.
- **The work-first Home and the tokens-only surface** (DESIGN-008 §7, §8): Home
  re-weighted to what-needs-doing; the Cost view becomes the **Work** view in
  tokens; no money on any rendered page.
- **Parent-owned milestones and roadmaps, displayed read-only** (DESIGN-008
  §5.1a): an additive migration gives them an owner; the owning entity's page
  lists the ones parented to it; entity pages list the milestones they are a
  member of, with click-through; project-level ones appear on Home.
- **The two-Markdown-lists display** (DESIGN-008 §5.1b): a milestone renders as
  an unordered checklist showing done/not-done, a roadmap as an ordered numbered
  list; milestone completion shows both the ticked count and a token bar.
- **Plain read-only Milestone, Roadmap and Task pages** so links resolve.
- **The Inbox, restyled** (behaviour kept from SPEC-004): each checkpoint gains a
  permalink and a link to the object it concerns.
- **Additive store reads and one forward-only migration** (§FR-10): no change to
  the meaning of any phase 1–3 table.

### Out of scope (deferred, with destination)

| Deferred | To |
|---|---|
| **Milestone & roadmap *editing*** — composing members, ordering, locking, and the create actions | **Stage B** (a later spec). The service methods exist; Stage A owns and displays, Stage B edits via the modal (DESIGN-008 §5.1a, §9) |
| **A server-side description generator** — a dispatch that summarises a design document into prose | **Dropped, not deferred** (SD-2, DESIGN-008 D-5/D-13). Descriptions are written by the chat agent over MCP (SPEC-008) and editable in the UI; there is no server-side generator to build |
| **The size-roll-up tree, the filterable work list, task detail, velocity** | **Stage B** (DESIGN-008 §9) |
| **In-page editing of design documents** (as opposed to the description) | Deferred goal (DESIGN-008 §5.3 tier 2); V1 uses the editor-and-watcher loop (§5.5), which already works |
| **Removing the shrunk CLI subcommands** | After Stage A **and** Stage B demonstrably cover them (DEC-003 sequencing). This spec supplies the coverage; the removal is a separate later commit |
| **The money machinery underneath** (USD ledger, price table, dollar cap) | Kept **dormant** (DESIGN-008 Q-C); this spec removes money from the *surface* only, changes no engine money code, and runs no migration against it |
| **Multi-user authentication** | A later spec; Stage A keeps the single configured operator (DESIGN-007 CC-6) |
| **Visual/production design** (colour, type, spacing, theming, mobile) | Production work within this information architecture (DESIGN-008 §10) |

### Scope decisions

- **SD-1 — Stage A owns and *displays* milestones and roadmaps; it does not
  *edit* them.** The parent relationship and the two-list display land here so
  browsing is complete and links resolve; composing, ordering and locking are
  Stage B. This is the §0 boundary. One-line redirect: pull the Stage-B editing
  modals into this slice.
- **SD-2 — Titles and descriptions are authored by the chat agent (MCP), shown
  in the UI, with a UI convenience edit; no server-side generator.** The chat
  agent that creates an initiative during a planning conversation writes its
  title and description in the same move and sets them with an MCP tool
  (companion slice, §0). Stage A *displays* them and offers a light in-place edit
  so a human at the command centre can fix a word without switching to the chat
  agent. The earlier idea of a server-side summariser dispatch is dropped
  entirely, not deferred — a subagent calling an API to summarise is the wrong
  shape for the planning half.
- **SD-3 — Server-rendered, one binary** (inherits DESIGN-007 CC-1 via
  DESIGN-008 §3): no SPA, no Node, no separate build or deploy. HTML + HTMX + SSE
  via `embed.FS`.
- **SD-4 — Single configured operator** (inherits CC-6): one actor for the
  browser session; real auth is a later spec.
- **SD-5 — Tokens on the surface; money dormant underneath** (DESIGN-008 D-4,
  Q-C): the UI shows only tokens; the engine's money code is untouched, not
  removed.
- **SD-6 — No entity addressed by a typed path anywhere in the UI** (DESIGN-008
  D-1). This is a hard rule, not a preference: the review checks for it (NFR-8).

## 3. Requirements

### FR-1: The shell and navigation

- **FR-1.1** Every initiative, feature and document is reachable at a real URL
  built from its readable path — `/ui/i/<initiative-path>`,
  `/ui/f/<feature-path>`, `/ui/d/<document-path>` — resolving through the
  existing path lookups (`InitiativeBySlugPath` and the feature-by-path service
  method); the project is `/ui`. Each works on direct load and via browser
  back/forward.
  *AC:* visiting each URL directly renders the entity; an unknown path returns a
  clear not-found page, not a 500.
- **FR-1.2** A persistent nav rail carries the top-level views (Home, Browse,
  Inbox, Documents, Work) and the initiative tree as navigation chrome;
  breadcrumbs across the top of an entity page carry its ancestry
  (`InitiativeAncestors`) and are the primary way up. In-view moves swap the main
  panel via HTMX, but every state has a URL.
  *AC:* the breadcrumb of `/ui/f/auth/login` shows Project ▸ … ▸ the feature, and
  each crumb links to that ancestor's page.

### FR-2: The entity page

- **FR-2.1** Project, Initiative and Feature pages share one layout (DESIGN-008
  §5.2): breadcrumbs; the description as a short human summary at the top of the
  body; the main design document rendered as the body; a rail of status, size in
  tokens (with tier and the `?` for unestimated work), and the actions valid for
  the entity in its state; and relation sections — children, documents,
  milestones-and-roadmaps parented here, milestones this entity is a member of,
  and activity from the audit log. Relation sections are stacked on the page with
  anchor links (not tabs, DESIGN-008 §5.2).
  *AC:* a seeded feature page renders each region from the corresponding store
  read; the size figure equals the sizing roll-up, not a re-derivation.
- **FR-2.2** An action whose gate is not satisfied is shown **disabled with the
  reason in plain words**, never hidden and never a silent failure.
  *AC:* a feature whose spec is still in review shows *Start work* disabled with a
  human-readable reason; the reason names what must happen first.

### FR-3: The document-as-body rendering

- **FR-3.1** The entity's **main design document** renders as the page body,
  read-only, through the existing `renderMarkdown` / `readDocFile` path (front
  matter stripped, goldmark + bluemonday). The main document is the one marked
  primary for the entity, or, if none is marked, the first attached document of
  type `design` (DESIGN-008 Q-B). A *make this the main document* action sets the
  mark. If the entity has no design document, the body is a clear empty state
  that says so and offers *Attach a document*.
  *AC:* an entity with a design document shows its rendered body; marking a
  different document primary changes the body; an entity with none shows the
  empty state, not a blank panel.
- **FR-3.2** All of an entity's documents are listed in its Documents section
  with lifecycle state, each linking to its document page; the body featuring one
  does not hide the rest.
  *AC:* an entity with three documents lists all three; one is the body, three
  are in the section.

### FR-4: The description and title

- **FR-4.1** Initiatives and features display their title and `description` (the
  existing column) as the human summary on their page and wherever they are
  listed in a parent's children. A light in-place edit lets the operator update
  either; saving writes the text, attributed to the operator and audited like any
  mutation. There is no human-edited flag and no generator (SD-2); the primary
  authoring path is the chat agent's MCP tool (companion slice).
  *AC:* editing a description on the page persists it and writes an audit row; the
  edited text appears in the parent's child list; the same field is what the MCP
  set-description tool writes.
- **FR-4.2** Description and title are the only human-facing text editable in the
  UI; design-document bodies are not editable here (they use the
  editor-and-watcher loop, DESIGN-008 §5.5), and specifications and development
  plans are never human-editable (DESIGN-008 §5.3).
  *AC:* no edit control renders on a spec or dev-plan body; the description has
  one.

### FR-5: Actions relocated onto entity pages

- **FR-5.1** Each of these existing service methods is driven from the page of
  the entity it acts on, with the entity implied by location and no path typed:
  set estimate and AI estimate (initiative/feature rail); new sub-initiative
  (parent page; project root for top-level); new feature (initiative page); start
  feature, abandon feature (feature page); archive initiative (initiative page);
  document approve / request-changes (document page, where human-gated). Each
  calls the same gated, audited method the CLI uses, in-transaction with its
  audit row (O-3), acting as the configured operator.
  *AC:* each action produces the same audited outcome as its CLI equivalent;
  archiving an initiative with active features still raises the human checkpoint
  it does today; a blocked feature-start still returns the inline gate reason, not
  a 500.
- **FR-5.2** The global Actions panel and every free-text `ref`/`path` input from
  SPEC-006 are removed.
  *AC:* no template renders a form field into which an entity path is typed.

### FR-6: Attach a document

- **FR-6.1** An *Attach a document* action on an initiative or feature page
  registers a Markdown file (by its path in the repo) as a document owned by that
  entity, over the existing register-document service path (the one
  `cromwell doc add` uses). This is a new UI action, not a new authority.
  *AC:* attaching a document makes it appear in the entity's Documents section and
  eligible to be the body; the audit trail records the registration; the
  capability equals `doc add` (so the CLI command can later be removed, DEC-003).

### FR-7: The work-first Home and the tokens-only surface

- **FR-7.1** Home leads with what needs doing: checkpoints awaiting the operator,
  dispatches in flight, features grouped by lifecycle state, and what is
  unestimated; plus the project's top-level design writing, its top initiatives,
  and its project-level milestones and roadmaps as entry points. There is no
  dollar figure and no budget bar on Home.
  *AC:* Home renders the waiting/in-flight/unestimated summaries from store reads;
  no rendered element shows currency.
- **FR-7.2** The former Cost view becomes the **Work** view: the roll-ups per
  initiative, feature, milestone and project counted in **tokens**, not money.
  Across every page, sizes and roll-ups are tokens with confidence tier and the
  `?` for unestimated work.
  *AC:* the Work view's figures equal the token roll-ups; a repository-wide search
  of rendered templates finds no currency formatting.

### FR-8: Parent-owned milestones and roadmaps (display)

- **FR-8.1** Milestones and roadmaps gain an owner (an initiative or the project)
  via the migration (FR-10); existing rows default to project-owned. An entity's
  page lists the milestones and roadmaps parented **directly** to it (read-only in
  Stage A); project-level ones appear on Home. Ownership does not constrain
  membership (DESIGN-008 D-12).
  *AC:* a milestone owned by an initiative renders on that initiative's page and
  not on any other entity's; a milestone whose members span two trees still lists
  all its members.
- **FR-8.2** An entity lists the milestones and roadmaps it is a **member** of as
  short titles that click through, read-only, showing direct memberships only
  (DESIGN-008 §5.1a); this is distinct from the ones parented to it.
  *AC:* a feature that is a member of a milestone owned elsewhere shows that
  milestone as a click-through title in its "member of" list.
- **FR-8.3** A milestone renders as an unordered checklist (each member done or
  not) and a roadmap as an ordered numbered list of milestones; milestone
  completion shows **both** the ticked count (X of Y) and a token bar (finished
  tokens over estimated), per DESIGN-008 §5.1b, D-10.
  *AC:* a milestone with 3 of 4 members done but the fourth being the largest
  shows "3 of 4" and a token bar below 75%.

### FR-9: Plain read-only Milestone, Roadmap and Task pages

- **FR-9.1** Milestone (`/ui/m/<id>`), Roadmap (`/ui/r/<id>`) and Task
  (`/ui/t/<id>`) have plain read-only pages sufficient for links to resolve —
  name, description, members/entries, progress in tokens — reached by
  click-through, not from any global list. There is no "all milestones" or "all
  roadmaps" index (DESIGN-008 §5.1a).
  *AC:* clicking a milestone from a member lands on its read-only page; no nav
  element lists all milestones or all roadmaps.

### FR-10: Additive store reads and one migration

- **FR-10.1** One forward-only migration (`0005`) adds, additively and with
  sensible defaults for existing rows: an owner (`owner_type` + nullable
  `owner_id`, matching the documents pattern; existing rows → project) on
  `milestones` and `roadmaps`; and a primary-document mark on `documents`. No
  phase 1–3 column changes meaning. (No human-edited flag — SD-2.)
  *AC:* the migration applies forward on a populated database; existing
  milestones become project-owned; `go test -race ./...` stays green.
- **FR-10.2** The service layer gains additive reads: documents for an owner;
  milestones and roadmaps owned by an entity; milestones an entity is a member
  of; and the primary-document resolution. Each is covered by a handler test.
  *AC:* each read returns correct data against a seeded database.

### FR-11: The Inbox, restyled

- **FR-11.1** The Inbox keeps its SPEC-004 behaviour (pending checkpoints,
  respond with any accepted answer kind, live badge over SSE); each checkpoint
  gains a permalink (`/ui/inbox/<id>`) and a link to the object it concerns.
  *AC:* the respond path is unchanged and still audited; a checkpoint's object
  link navigates to that entity's page.

## 4. Non-functional requirements

- **NFR-1 — No new stack.** No Node, npm, bundler or separate deploy; templates
  embedded via `embed.FS`; `go vet ./...` and `go test -race ./...` stay clean
  (DESIGN-007 CC-1).
- **NFR-2 — A rendering over the service layer, not an API client.** HTML
  handlers call service and store functions directly; no HTML handler issues HTTP
  to `/api/*` (CC-2). The `/api/*` surface is unchanged and survives as the
  programmatic HTTP/JSON surface (scripting, CI, external clients); the MCP facet
  is a *third* rendering over the same service layer (SPEC-008), not a client of
  `/api/*` either.
- **NFR-3 — No new authority.** Every mutation is an existing gated, audited
  service method; no gate override except through an answered checkpoint; every
  mutation audit-in-transaction (CC-4, O-3, DESIGN-008 D-8). The new UI mutations
  in this spec — set description, mark primary document, attach document — either
  are existing service paths or are additive methods that follow the same
  audit-in-transaction rule.
- **NFR-4 — Tested as the phase 1–3 suites are.** Handler tests drive the HTML
  handlers and SSE stream directly (httptest) with the mock provider against real
  Postgres; a manual browser smoke covers the live experience.
- **NFR-5 — Realtime unchanged.** The SSE/`Notifier` boundary from SPEC-004 is
  reused as-is; live regions on Home and the Inbox refresh through the normal
  service path (presentation-only).
- **NFR-6 — Migration is forward-only and additive.** No destructive change; the
  compatibility boundary (plain Postgres, DEC-002) holds; CI runs against plain
  Postgres.
- **NFR-7 — Human-facing prose (D-6).** All UI copy — headings, descriptions,
  labels, gate reasons, empty states — is written in full, clear sentences with
  terms explained. A reviewer reads the rendered copy against this bar.
- **NFR-8 — No typed entity paths (SD-6, D-1).** No rendered form takes an entity
  path as free text; the review greps the templates to confirm.

## 5. Definition of Done

1. Every FR acceptance criterion passes in CI (mock provider) on plain Postgres
   and the Supabase local stack.
2. **A human-confirmed live smoke of the browse-and-drive claim:** against a real
   project, an operator opens the surface and — without typing a path or touching
   the CLI — browses from the project down to a feature reading design documents
   as page bodies, corrects a description in place, attaches a document, and
   starts a feature, watching the page stay live. Recorded in a walkthrough with
   screenshots.
3. A repository-wide check confirms no currency on any rendered page and no
   typed-path form field (NFR-7 money check, NFR-8 path check).
4. `go vet ./...` and `go test -race ./...` clean.
5. Entry criteria for **Stage B** (milestone/roadmap editing, the roll-up tree,
   the work list, task detail) drafted, and the DEC-003 CLI-removal checklist
   started (which retired commands Stage A now covers, which await Stage B).

## 6. Open questions carried into implementation

- **Live-region granularity on entity pages** — which parts of an entity page
  refresh on which event kinds (a feature starting, a document changing on
  disk), and whether a coarse panel refresh suffices versus targeted swaps. A
  tuning, not a contract.
- **Main-document override edge cases** — what happens to the marked primary
  document when it is superseded, or when the only design document is removed. The
  floor is: fall back to the first design-type document, else the empty state.
- **The UI description edit and the MCP set-description tool share one field** —
  both write the entity's `description` column (FR-4.1); confirm the UI edit and
  the SPEC-008 tool use the same additive service method so they cannot diverge.
