# Entry criteria — the MCP facet, slice 2 (the read and poke tools)

**Written:** 2026-07-26, on finishing the authoring slice (SPEC-008).
**Purpose:** what a slice-2 package must settle, and what it inherits.

Slice 1 gave the chat agent the *planning authoring* tools. Slice 2 is the rest
of the facet the vision names (§7): content search, doc-intel lookups,
responding to checkpoints, and dispatching a review.

## What slice 1 already gives you

- **The transport and protocol are built.** A hand-rolled JSON-RPC 2.0 endpoint
  at `POST /mcp` over the existing TCP listener, answering `initialize`, `ping`,
  `tools/list` and `tools/call`, with no new dependency (`internal/server/mcp.go`).
  Adding a tool is one entry in `mcpTools()` plus a handler.
- **The actor seam.** `server.mcp_actor` (default `chat-agent`), parallel to
  `ui_actor`. Every mutation is audited to it.
- **The error convention.** Tool failures come back as an unsuccessful *tool
  result* with plain-language text, not a protocol error, so the agent can read
  it, adjust and retry. Constraint violations are translated (`isUniqueViolation`).
- **The live-signal convention.** `notifyEntityChanged` broadcasts a
  presentation-only event so open UI pages refresh.
- **The boundary test.** `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
  asserts the advertised set exactly. **Slice 2 must update that list
  deliberately** — it is designed to fail when a tool is added, so that widening
  the facet is always a conscious act.

## What slice 2 has to decide

1. **Where the DEC-004 line falls for each new tool.** This is the whole of the
   review. `search` and doc-intel `find` are pure reads and clearly fine.
   The other two are not obviously so:
   - **`checkpoint_respond`** — answering a checkpoint is how a gate override
     happens. DEC-004 says the facet may not "override a gate" or "respond to a
     gate on the development side". A blanket respond tool would breach that.
     If it is included at all it must be *filtered by checkpoint kind*, and that
     filter is a runtime check — a departure from SD-2's confinement-by-omission,
     which deserves an explicit decision rather than a drift.
   - **`dispatch_review`** — spawns an agent. DEC-004 forbids spawning agents.
     On its face this belongs on the never list, and the vision's mention of it
     predates DEC-004. **Surface this conflict to Sam explicitly**; do not
     resolve it by writing the tool.
2. **Whether reads need pagination or a result cap.** Content search over a large
   corpus could return more than a chat agent's context can hold. The authoring
   reads did not need this; search does.
3. **Whether to add MCP *resources* and *prompts*.** The protocol has both, and a
   document corpus is a natural fit for resources. Slice 1 advertises only the
   `tools` capability. Adding resources changes the `initialize` response and the
   client's mental model, so it is a design decision, not a build detail.
4. **stdio transport.** Deferred from slice 1. Some clients want a spawned
   process rather than an HTTP endpoint. Cheap to add; decide if it is wanted.
5. **Per-user authentication** (CC-6). Still a later spec for both surfaces. The
   single-actor seam is preserved for it.

## What must stay true

- **Third rendering, one service layer** (SD-4, NFR-2): tool handlers call
  service methods directly; no HTTP to `/api/*`.
- **No new authority** (NFR-3): every mutation is an existing gated, audited
  method, audit-in-transaction.
- **Confinement by omission** (SD-2): if a capability must be *partially*
  allowed, that is a design change worth naming, not a quiet runtime filter.
- **Human-facing tool text** (NFR-5): descriptions and errors are relayed to a
  person by the chat agent, so they are written for one.

## Precondition

Slice 1 merged and green, which it is. The live smoke drove a real MCP client
against a real project; the walkthrough records it.
