# SPEC-008: The MCP Facet — slice 1, planning authoring

**Status:** **Approved and binding — 2026-07-25** (Sam, after the authoring
consistency review). Companion to
[SPEC-007](SPEC-007-workflow-surface-stage-a.md).
**Date:** 2026-07-25
**Parent decision:** [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md)
(the MCP facet may author the planning layer, but may not drive development or
touch gates)
**Parent design:** [DESIGN-008](../design/DESIGN-008-the-workflow-surface.md)
§6a (two surfaces, split at the pivot), and
[DESIGN-007](../design/DESIGN-007-web-command-centre.md) §4 (one service layer,
many renderings) and §7 (the MCP facet architecture)
**Vision:** [vision-v1](../vision/vision-v1.md) §7 (the MCP facet as a component)
**Refines the boundary in:** vision §7 and DESIGN-007 §9, both of which say "the
MCP facet does not drive mutations" — narrowed by DEC-004 to "no *development-
pipeline* or *gate* mutations; planning authoring is allowed." Also refines
DESIGN-007 §4, which cast the JSON API as the MCP facet's *backend*: this facet
is a third rendering over the service layer, not a client of `/api/*` (SD-4).
**Supersedes:** the placeholder "SPEC-005 — the MCP facet" reference used in
earlier docs. The MCP facet is delivered in slices; this is the first. The
remaining read/poke tools (content search, doc-intel find, checkpoint respond,
dispatch review — vision §7) are a later slice.
**Precondition:** phases 1–3 and the SPEC-004/006 service layer are built and
green. This spec adds a consumer of that layer; it adds no orchestration
capability and no new authority.

---

## A note on prose

This is a specification — its detailed tool contracts are agent-facing and may
be terse (DESIGN-008 D-6). But the *strings the tools expose to a human's chat
agent*, and through it to the human — tool descriptions, parameter help, error
messages — are human-facing and must be written in plain, clear language, since
they shape what the chat agent says back to the person (NFR-5). This document,
being read by a human, follows the readable style too.

## 0. Framing: the MCP facet, in slices, and why this slice is first

Cromwell's vision has three surfaces onto the server (§7): the web UI, the git
watcher, and the **MCP facet** — the interface a human's chat agent (Claude
Desktop, Zed, Cursor, and the like) uses to work with Cromwell. The facet was
long treated as a "later" thing. [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md)
changed that: because planning in Cromwell is a *conversation*, the chat agent
is the primary way a project's planning structure is created, and so the facet
is the planning half's **input** surface — a peer of the browsing UI, not its
successor.

The full facet is large (authoring, plus content search, doc-intel lookups,
checkpoint responding, review dispatch). **This spec is its first slice:
planning authoring** — the tools by which a chat agent creates and shapes
initiatives, features, titles, descriptions, and document attachments. It is
scoped to exactly the authority DEC-004 grants and no more, and it is the
companion that makes [SPEC-007](SPEC-007-workflow-surface-stage-a.md)'s browsing
UI have something, created the intended way, to browse.

**The central boundary for review** is the DEC-004 line, made physical here: the
facet exposes authoring tools and *no* tool that starts a feature into
development, overrides a gate, or spawns an agent. The enforcement is the
absence of those tools, not a runtime check — you cannot call what is not
there.

## 1. Goal

Prove that a human's chat agent can build a project's planning layer through
Cromwell, safely:

1. **The chat agent can author planning structure.** Connected to Cromwell over
   MCP, it can create initiatives and features, set their titles and
   descriptions in human prose, and attach documents — the same gated, audited
   service operations the UI and CLI use.
2. **It authors as a third rendering, not a new authority.** Every tool calls an
   existing service method, in-transaction with its audit row; the facet gains
   no power the UI lacks (DESIGN-008 D-8).
3. **It cannot cross the pivot.** There is no tool to start a feature into
   development, transition development lifecycle, override a gate, or spawn an
   agent. The DEC-004 line holds because the tools that would cross it do not
   exist.
4. **The two surfaces stay in sync.** When the chat agent creates an initiative,
   an open web UI page reflects it live, because the facet publishes the same
   events every mutation does.

This slice is done when a chat agent, in a normal planning conversation, can
stand up an initiative tree with features, titles and descriptions and attached
documents against a real project — and a human watches it appear live in the
browsing UI — while the facet demonstrably offers no development-side tool.

## 2. Scope

### In scope

- **The MCP server**, served by the existing `cromwell` binary over its TCP
  listener (`server.http`), as a third rendering over the phase 1–3 service
  layer (DESIGN-007 §4). No new CLI subcommand (respects
  [DEC-003](../decisions/DEC-003-cli-scope.md)).
- **Authoring tools** (the DEC-004 planning layer):
  - create an initiative (under a parent, or at the top level);
  - create a feature (under an initiative);
  - update an initiative's or feature's name/title and description;
  - attach an existing document to an initiative or feature.
- **The reads an agent needs to author well**: browse the initiative → feature
  tree, get one entity's detail (state, description, children, documents), and
  list an entity's documents. Enough to know where to create and what exists —
  not the full read/poke catalogue.
- **Authority confinement to DEC-004**: the toolset is curated so that no
  development-pipeline or gate operation is reachable.
- **Audit and actor**: every mutation is audit-in-transaction (O-3), attributed
  to a configured MCP actor (§SD-3).
- **Realtime coherence**: authoring mutations publish the same events UI/CLI
  mutations do, so open UI pages update live (SPEC-004 SSE).
- **Human-facing tool text**: tool and parameter descriptions and error messages
  written in plain language (NFR-5).

### Out of scope (deferred, with destination)

| Deferred | To |
|---|---|
| **The read/poke MCP tools** — content search, doc-intel `find`, `checkpoint_respond`, `dispatch_review` (vision §7) | A later MCP facet slice. This slice carries only the reads authoring needs |
| **Any development-side or gate tool** — start a feature, transition dev lifecycle, override a gate, spawn an agent | **Never, by DEC-004.** These stay UI-human-driven; their absence here is the enforcement |
| **Estimates over MCP** (set an estimate; AI-estimate) | A later slice. Manual set is planning, but the AI-estimate spawns the estimator agent (a dev-side act), so estimates are kept out of the first authoring slice to keep the line crisp |
| **Archive / abandon over MCP** | A later slice. Archiving raises a G5 checkpoint and abandoning is destructive; kept to the UI for now while the authoring slice proves the pattern |
| **Writing document *content*** (the `.md` files) | Not the facet's job — the chat agent writes files with its own editor/file tools and commits them; the git watcher indexes them; this facet only *attaches* (registers) them. Git owns content; the facet owns the entity/registration layer (vision §12 compartments) |
| **Per-user authentication of the chat agent** | A later spec (CC-6). This slice uses one configured MCP actor, as the UI uses one operator |
| **stdio transport / a spawned bridge** | Optional later addition; this slice serves MCP over the existing TCP listener |

### Scope decisions

- **SD-1 — This is the authoring slice of the MCP facet, not the whole facet.**
  Authoring is the DEC-004-granted, DESIGN-008-companion capability and the one
  that unblocks the browsing UI's intended creation path. The read/poke tools
  are a coherent later slice. One-line redirect: fold the read/poke tools in
  here.
- **SD-2 — Authority is confined by omission.** The DEC-004 line is enforced by
  *which tools exist*: no development or gate tool is defined, so none can be
  called. This is stronger and simpler than a runtime allow/deny check, and it
  matches how the vision confines dispatched-agent tools by profile (vision §8).
- **SD-3 — One configured MCP actor.** All authoring is attributed to a single
  configured actor (`server.mcp_actor`, default e.g. `chat-agent`), parallel to
  the UI's single operator (SPEC-004 SD-4). Per-user auth is a later spec; the
  actor seam is preserved for it.
- **SD-4 — Third rendering, one service layer.** MCP tool handlers call the same
  service methods the UI and CLI do (`CreateInitiative`, `CreateFeature`,
  `RegisterDoc`, and the additive update-entity method); they do not re-implement
  logic and do not call `/api/*` over HTTP (DESIGN-007 §4, CC-2).
- **SD-5 — Transport is MCP over the existing TCP listener.** No new CLI verb
  (DEC-003); the single-binary story holds; a stdio bridge is a later option.

## 3. Requirements

### FR-1: The MCP server

- **FR-1.1** With `server.http` set, the `cromwell` binary serves an MCP
  endpoint over that TCP listener, alongside the web UI and the JSON API; with no
  TCP listener configured, the MCP facet is simply unavailable and the socket
  CLI is unaffected (parallel to SPEC-004 FR-1.1).
  *AC:* an MCP client handshakes with the server and lists tools; unset, the port
  is closed and the rest of the server runs.
- **FR-1.2** The facet advertises exactly the tools this slice defines (FR-2,
  FR-3) — the authoring writes and the authoring reads — and no others.
  *AC:* the advertised tool list contains the authoring and read tools and no
  development, gate, dispatch, or override tool.

### FR-2: Authoring tools (the writes)

Each tool calls the named existing service method, in one transaction with its
audit row (O-3), as the configured MCP actor (SD-3), and publishes the same
event the UI/CLI mutation publishes (FR-5).

- **FR-2.1 create_initiative** — creates an initiative under a given parent path,
  or at the top level when no parent is given; takes slug, human name/title, and
  an optional description. Calls `store.CreateInitiative`.
  *AC:* the initiative exists with the given fields; a duplicate slug under the
  same parent returns a clear, human-readable error, not a raw constraint
  violation.
- **FR-2.2 create_feature** — creates a feature under a given initiative path;
  takes slug, name/title, optional description. Calls `store.CreateFeature`. The
  feature is created as an idea (its normal starting state); it is **not** started
  into development (DEC-004).
  *AC:* the feature exists in its initial state under the initiative; there is no
  parameter and no tool that advances its state.
- **FR-2.3 update_initiative / update_feature** — updates an entity's name/title
  and/or description by path. Calls an additive service method that writes the
  fields in-transaction with an audit row. (The `description` column already
  exists; this is the same field a human edits in the UI and the same one FR-4 of
  SPEC-007 displays.)
  *AC:* updating a description persists it and writes an audit row; the change is
  visible in the UI and to a subsequent read tool.
- **FR-2.4 attach_document** — registers an existing Markdown file (by repo path)
  as a document owned by a given initiative or feature, with a document type.
  Calls `s.RegisterDoc`. This is the same capability as the UI's "attach a
  document" and the retired `cromwell doc add` (DEC-003).
  *AC:* the attached document appears in the entity's document list and is
  eligible to be its main document; the registration is audited.

### FR-3: Authoring reads

- **FR-3.1 get_tree / list_initiatives** — returns the initiative → feature tree
  (or a subtree) with names, paths and states, enough for the agent to know where
  to create.
  *AC:* the returned structure matches the store's tree reads for a seeded
  project.
- **FR-3.2 get_initiative / get_feature** — returns one entity's detail: title,
  description, state, children, and attached documents, by path.
  *AC:* the returned detail matches the entity's store record.
- **FR-3.3 list_documents** — returns the documents attached to an entity with
  type and lifecycle state.
  *AC:* matches the store read for a seeded entity.

### FR-4: The authority line, enforced by omission

- **FR-4.1** The facet defines **no** tool that starts a feature into
  development, transitions development-side lifecycle, overrides or forces a gate,
  responds to a gate on the development side, or spawns an implementer/reviewer/
  estimator agent (DEC-004).
  *AC:* a review of the tool registry confirms none of these exists; a test
  asserts the advertised tool set is exactly the FR-2/FR-3 set.
- **FR-4.2** Because the confinement is by omission, no runtime permission layer
  is required for it; a tool that is not registered cannot be invoked.
  *AC:* an MCP call naming an unregistered tool (e.g. `start_feature`) is
  rejected by the MCP layer as unknown, not by a bespoke deny-check.

### FR-5: Realtime coherence with the UI

- **FR-5.1** Every authoring mutation publishes the same in-process event its UI/
  CLI counterpart does, so an open UI page updates live (SPEC-004 SSE).
  *AC:* creating an initiative over MCP updates an open browsing page within one
  event, with no reload.

### FR-6: A small additive service method

- **FR-6.1** The service layer gains one additive method: update an initiative's
  or feature's name/title and description, in-transaction with an audit row. No
  schema change (the columns exist). It is shared by this facet and the UI's
  in-place edit (SPEC-007 FR-4).
  *AC:* the method updates the fields and audits; both the MCP tool and the UI
  edit call it; `go test -race ./...` stays green.

## 4. Non-functional requirements

- **NFR-1 — No new stack, one binary.** The MCP server ships in the `cromwell`
  binary; no new external service, no new CLI verb (DEC-003). `go vet ./...` and
  `go test -race ./...` stay clean.
- **NFR-2 — Third rendering, not an API client.** MCP handlers call service/store
  functions directly; no handler issues HTTP to `/api/*` (DESIGN-007 §4, CC-2).
- **NFR-3 — No new authority.** Every mutation is an existing (or additive,
  same-shape) gated, audited service method; audit-in-transaction (O-3); the
  facet cannot exceed DEC-004 because the tools are curated (SD-2). No gate
  override, no agent spawn, ever.
- **NFR-4 — Tested as the phase 1–3 suites are.** Tool handlers are tested
  directly against the service layer with the mock provider and real Postgres; a
  manual smoke drives a real chat-agent client end to end.
- **NFR-5 — Human-facing tool text.** Tool descriptions, parameter help, and
  error messages are plain, clear language (DESIGN-008 D-6), because the chat
  agent relays them to a person. Errors say what went wrong and what to do, not a
  raw database or protocol fault.
- **NFR-6 — Auditable second authoring surface.** Because a chat agent can now
  create structure, every MCP mutation must be on the audit trail exactly like a
  UI or CLI one, attributed to the MCP actor — this is what makes a second
  authoring surface safe (DEC-004 consequences).
- **NFR-7 — Idempotency-friendly errors.** A create that collides with an
  existing slug fails cleanly and re-runnably (the agent can read, adjust, retry);
  no partial writes (the mutation is one transaction).

## 5. Definition of Done

1. Every FR acceptance criterion passes in CI (mock provider) on plain Postgres
   and the Supabase local stack.
2. **A human-confirmed live smoke:** a real chat-agent MCP client, in a planning
   conversation, creates an initiative tree with features, sets titles and
   descriptions in human prose, and attaches a document — against a real project
   — while a human watches it appear live in the SPEC-007 browsing UI. The same
   session confirms the client is offered no development or gate tool. Recorded in
   a walkthrough.
3. A test asserts the advertised tool set is exactly the authoring + read set of
   FR-2/FR-3 (the DEC-004 line, FR-4).
4. `go vet ./...` and `go test -race ./...` clean.
5. Entry criteria for the next MCP slice (the read/poke tools: search, find,
   checkpoint respond, dispatch review) drafted.

## 6. Open questions carried into implementation

- **The MCP Go implementation** — which MCP server library or hand-rolled
  handler, and the exact transport (Streamable HTTP is the assumption on the
  existing TCP listener). A build choice, not a contract.
- **How the chat agent discovers the endpoint and actor** — configuration on the
  client side (URL, and that it is a local trusted server) versus a small
  discovery affordance. The single-actor model (SD-3) keeps this simple for now.
- **Whether update_initiative/update_feature should be one tool or two** (a
  single `update_entity` taking a kind, versus per-kind tools) — an ergonomics
  call for the agent, decided in build.
- **Path versus id in tool arguments** — this slice assumes human-readable paths
  (`auth/login`), matching the UI; confirm nothing in the agent workflow needs
  opaque ids instead.
