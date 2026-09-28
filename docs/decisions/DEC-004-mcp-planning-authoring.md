# DEC-004: The MCP facet may author the planning layer, but not drive development

**Status:** Accepted. **Amended 2026-09-28:**
[Amendment 1](#amendment-1--milestones-and-roadmaps-2026-09-28) adds
milestones and roadmaps to the planning layer.
**Date:** 2026-07-24
**Decided by:** Sam
**Refines:** [vision-v1](../vision/vision-v1.md) §7 (the MCP facet's "no direct
entity mutation" rule)
**Context for:** [DESIGN-008](../design/DESIGN-008-the-workflow-surface.md) (the
UI/MCP division of labour), [SPEC-007](../specs/SPEC-007-workflow-surface-stage-a.md)
(the browsing UI) and the forthcoming MCP authoring slice

## Decision

The MCP facet — the interface the human's chat agent (Claude Desktop, Zed,
Cursor, and the like) uses to work with Cromwell — **may create and edit the
planning layer**, and **may not drive the development pipeline or touch gates**.

**It may:**

- create and edit initiatives and features (as ideas, before they cross into
  development);
- set and update titles and descriptions;
- attach documents to entities, and author document content;
- everything the vision already granted it: query state, search, help a human
  respond to a checkpoint, trigger a review.

**It may not:**

- start a feature into active development, or drive any development-side
  lifecycle transition;
- override a gate, or force any transition a gate is blocking;
- spawn implementer or reviewer agents, or otherwise act as the orchestrator.

## Context

Vision §7 draws a firm line around the MCP facet:

> "What it does **not** expose: direct entity mutation, gate overrides, raw
> agent spawning. Those are orchestrator concerns; humans drive them via the web
> UI. This separation prevents the kanbanzai failure mode where the
> MCP-connected chat agent became a rogue orchestrator."

That line, taken literally, forbids the chat agent from creating an initiative —
which is exactly what the planning half of Cromwell wants it to do. During the
DESIGN-008 work it became clear that planning is a *conversational* act: you
tell your chat agent "set up an authentication initiative with email-and-password
and passkey under it," and it builds that structure, named and described, in one
move. Forbidding the chat agent from creating structure would push all authoring
back into web forms — the very verb-shaped pattern the redesign exists to remove.

So the vision's rule and the design's need appear to conflict. They don't, once
you see that they sit on opposite sides of Cromwell's pivot point.

## Rationale: the line is the pivot point

Cromwell has one architectural seam (vision §2): **planning** is human-driven and
AI-assisted; **developing** is AI-driven and human-overseen; the specification is
the handover between them.

The "rogue orchestrator" the vision guards against is a chat agent reaching
across that seam to drive the *developing* half — pushing work through gates,
spawning agents, forcing transitions. That was the kanbanzai failure, and it
stays firmly prohibited.

Creating an initiative and writing its description is on the *planning* side of
the seam — the side the vision itself calls "human-driven, AI-assisted." A chat
agent doing this is not orchestrating anything; it is the assistance the planning
half was always meant to have. The vision's blanket phrase "direct entity
mutation" conflated two very different things: **authoring planning structure**
(safe, and wanted) and **driving the development pipeline** (the actual danger).

This decision separates them. The refined rule reads: *the MCP facet may author
the planning layer; it may not exercise development-pipeline or gate authority.*
The anti-rogue-orchestrator intent is preserved exactly — no gate overrides, no
agent spawning, no forcing development transitions — while conversational
planning becomes possible.

## Consequences

- **Two surfaces author the planning layer**, and that is deliberate: the chat
  agent via MCP (the primary, conversational path) and the web UI (a convenience
  for a human already at the command centre). Both call the same gated, audited
  service methods; neither gains authority the other lacks (DESIGN-008 D-8).
- **The web UI keeps the development-side human authority.** Starting a feature
  into development, answering a gate checkpoint, approving or requesting changes
  on an escalated review — these stay human acts driven from the UI, never from
  MCP. This is the half of the vision's line that does not move.
- **The MCP facet is promoted from "later" to a peer deliverable.** If the chat
  agent is the primary way planning structure is created, the facet is the
  planning half's *input* surface, and a slice of it (the authoring tools) lands
  alongside the browsing UI (SPEC-007), not after it. The vision's three
  components — Web UI, MCP facet, git watcher — are peers, as §7's diagram always
  showed.
- **Every MCP authoring call is audited** the same way a UI or CLI mutation is
  (audit-in-transaction, O-3), so the chat agent's actions are on the trail like
  anyone else's. The audit story is what makes a second authoring surface safe.

## Alternatives considered

- **Keep the vision's line as written** — MCP stays read-and-poke only; creation
  happens in the UI and CLI. Rejected: it forces planning authoring into web
  forms, contradicting the conversational-planning model and re-introducing the
  verb-shaped UI the redesign removes.
- **Let MCP also drive some development lifecycle** (e.g. start a feature).
  Rejected: it steps back toward the rogue-orchestrator risk the vision named,
  for little gain — the human at the command centre is the right driver for the
  moment work crosses into development.

## Amendment 1 — milestones and roadmaps (2026-09-28)

**Status:** **Accepted — Sam, 2026-09-28.** Drafted by Claude, on Sam's
instruction, during M4 ([SPEC-010](../specs/SPEC-010-milestones-and-roadmaps-editing.md)).
Added as an amendment rather than an edit, as DEC-005 asks of accepted
decisions.

**Milestones and roadmaps are part of the planning layer.** The chat agent may
create them, change what a milestone contains, and place, move and remove
milestones on a roadmap, as it does with initiatives and features. These are
ordinary planning edits, not relayed human decisions, so they don't need the
person's quoted words (DEC-006 Amendment 1).

**Gate G4 is a record-keeping check, not a development gate**, so "may not
touch gates" doesn't cover it. G4 only stops a milestone being marked as
shipped when none of its features is done. Nothing in the agent workflow reads
milestones: no work starts, stops or waits because of one. So the chat agent
may also mark a milestone as shipped and reopen it. Both acts can be undone
(SPEC-010 SD-11), and both are logged under the chat agent's name. G4 itself
still applies to it exactly as it does to a person.

The rule for every other gate is unchanged. G0 to G3 (which guard a feature's
path into and through development), G5 (archiving an initiative), and
anything that starts, stops or releases agent work stay out of the chat
agent's reach.

This amendment permits the two milestone tools; it doesn't build them. When
they are added, `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` must name
them on purpose, as DEC-005 requires.
