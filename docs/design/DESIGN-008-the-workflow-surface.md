# DESIGN-008: The Workflow Surface

**Status:** **Approved by Sam — 2026-07-25** (after the authoring consistency
review). Revised across several rounds of his feedback: money dropped from the
surface, parent-owned milestones and roadmaps, the two-list display, and the
chat-agent authoring model.
**Date:** 2026-07-22 (revised 2026-07-25)
**Parent:** [vision-v1](../vision/vision-v1.md) §2 (document-led), §3
(vocabulary), §4 (the flow), §14 (**"Web UI design: layout, navigation,
interaction patterns"** — the open question this document closes)
**Depends on:** [DEC-003](../decisions/DEC-003-cli-scope.md) (the CLI shrinks
to starting and hooking the server),
[DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) (the chat agent may
author the planning layer over MCP — see §6a),
[DESIGN-007](DESIGN-007-web-command-centre.md) §2, §3, §6, §7 (the parts we
keep — see §3)
**Revises the vision:** §5 and §8's treatment of cost in money — Cromwell
measures *work*, and the unit of work is tokens; money is dropped from the human
surface (§7). And, via [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md),
§7's blanket "the MCP facet exposes no direct entity mutation" — narrowed so the
chat agent may author the planning layer (§6a).
**Supersedes:** [DESIGN-007](DESIGN-007-web-command-centre.md) §5 (information
architecture) and §8 (the list of forms), and with them
[SPEC-004](../specs/SPEC-004-phase-4-web-command-centre.md) FR-1/FR-4 and
[SPEC-006](../specs/SPEC-006-command-centre-mutations.md)'s Actions panel; and
DESIGN-007 §4's framing of the JSON API as the MCP facet's *backend* — the MCP
facet is a third rendering over the service layer, not a client of `/api/*`
(§3, SPEC-008).

---

## A note on how this document is written

This is a document for people. Cromwell's own rule — set out in §4 and repeated
because it matters — is that anything a human reads should be written in plain,
descriptive prose, with technical terms and abbreviations spelled out. That
rule applies to this design document too, so where earlier drafts leaned on
internal shorthand (the `CC-4` rule, the `G4`/`G5` gates, `SSE`, `MCP`), those
are now explained on first use. Terse, compressed writing is for the documents
that only agents read — specifications and development plans. Not for this.

## 1. What this document is for

The vision left one thing deliberately unfinished. Its section 14 lists the web
interface's "layout, navigation, interaction patterns" as an open question to
be settled in a later design document. That later document was never written.
The one that came closest, DESIGN-007, said plainly in its own first section
that it was designing the *plumbing* of the interface — how pages are built and
served — and "deliberately not the visual language beyond the information
architecture and interaction model." So the question of what the interface
should actually feel like to use stayed open through two rounds of building.

We shipped anyway, and the result is well-built underneath and wrong on top.
This document settles the open question: what the things in the interface are,
where they live, how a person moves between them, and how they act on them.

It redesigns **one layer** — the web pages, their addresses, and the way you
navigate them. Everything underneath (the workflow engine, the gates that guard
each step, the audit trail, the way work is dispatched to agents, the sizing
maths, the live-update machinery) is untouched and not in question. That is the
whole reason a full redesign of every screen is a reasonable amount of work
rather than starting over: we are replacing the top layer of a sound building.

## 2. What went wrong, plainly

The planning screen today is thirteen forms stacked on top of each other
([planning.html](../../internal/server/ui/templates/planning.html)), one for
each thing the system can do. To act on a feature — say, to start it — you type
its path, `auth/login`, into a text box in a panel that floats above a tree in
which that same feature is already sitting there, named, one click away.

That is a **verb-first** interface. It asks "which action do you want?" and then
"and what should I do it to?" You can see exactly where it came from: the engine
offers a set of actions, the old command-line tool wrapped each action in a
command, and DESIGN-007 quite sensibly said the web interface should be "a
second rendering of the same service layer." What nobody caught is that
*re-rendering the list of actions* and *designing an interface* are not the same
job. We re-rendered the list of actions.

A tool for planning and tracking work — Linear, Jira, Monday, and everything in
that family — is **thing-first** instead. It asks "which thing?" and then shows
you what you can do to it. You are always somewhere, looking at something, and
the buttons in front of you are the ones that make sense for the thing you are
looking at. You never address something by typing its name.

Everything below follows from flipping that one thing around:

- **Every entity is a place.** It has a web address, a page, a heading, a body,
  and a history. You go to it.
- **Every action lives on the thing it acts on.** The floating panel of forms
  is not shrunk or tidied. It is deleted. Its thirteen forms become buttons on
  the pages of the things they change, where the thing is wherever you already
  are and never something you type.

## 3. What we keep from DESIGN-007

The plumbing was never the problem, and re-deciding it would waste the work.
Kept as-is:

- **The interface is a command centre, not a workspace** (DESIGN-007 §2). The
  interface *reads* and *reviews* documents; it does not edit their bodies.
  Writing happens in the author's own editor, in git. As §5.3 shows, this ends
  up being a strength, not a limitation.
- **Server-rendered HTML pages with HTMX, all inside the one Go program**
  (§3). No separate JavaScript application, no Node, no build step. The new plan
  is *more* pages and *fewer* forms, which suits plain server-rendered HTML
  better, not worse. (HTMX is a small library that lets a page swap in a piece
  of fresh HTML from the server without a full reload — enough interactivity for
  a command centre, without a front-end framework.)
- **One engine underneath, several ways of showing it** (§4). The same internal
  service functions are rendered three ways: the JSON API at `/api/*` (the
  programmatic surface, for scripting and external HTTP clients), the human web
  pages at `/ui/*`, and — new — the Model Context Protocol facet for the chat
  agent (the MCP authoring slice, [SPEC-008](../specs/SPEC-008-mcp-facet-planning-authoring.md)).
  Each calls the service layer directly; none calls another over HTTP. Per
  [DEC-003](../decisions/DEC-003-cli-scope.md), the old command-line tool is no
  longer one of them. (This refines DESIGN-007 §4, which had cast the JSON API as
  the MCP facet's *backend*; the facet is its own rendering, not a client of
  `/api/*`.)
- **The pages stay live by themselves** (§6). When something changes, the server
  pushes an update to any open page using Server-Sent Events — a standard
  browser feature where the server can send messages to a page that is already
  open. This matters a lot in the new design (§5.3).
- **One actor, for now** (§7). Multi-user sign-in is still out of scope; every
  action is attributed to the single configured operator.
- **The interface gains no new power.** Every button maps to an action the
  engine already offers, already guarded by the same rules and already written
  to the audit trail in the same database transaction. *The redesign moves
  buttons around; it does not touch who is allowed to do what, or what gets
  recorded.* This is worth saying loudly, because it is what makes the work
  cheap and safe: the risk here is almost entirely about how things look and
  where they sit, not about correctness.

Replaced: DESIGN-007 §5 (the five flat screens and their navigation) and §8
(the catalogue of actions as a flat list of forms).

## 4. The idea: the project is a place you browse

The first thing Cromwell says about itself is that it is **document-led** — "the
main interaction between humans and Cromwell is through documents" (vision §2).
Today's interface does not honour that. Documents are tucked away in a tab. The
tree shows names and token counts. The actual writing — the thing people made,
the thing the whole system exists to look after — is somewhere else.

The new interface turns that the right way up:

> **An entity's page *is* its main document, wrapped in its context.**

Go to an initiative and you see its design document, rendered and readable, as
the body of the page. Its status, its size in tokens, its children, its other
documents and its recent activity sit around that writing as a frame; they do
not push it aside. This is the single thing that makes Cromwell's planning
surface different from a generic issue tracker, and it is why the instinct
"more like web pages than an actual tree" is the right one. The project becomes
a small, living, browsable website about itself, whose pages happen to have
buttons.

### Two kinds of writing, and who each is for

This is the principle Sam asked to run through everything, so it gets stated
once, here, as a rule:

- **Human-facing writing** — page headings, the short description of an
  initiative or feature, button labels, notices, and the design documents
  themselves — is written **for designers and product people**. Full
  descriptive sentences. Technical terms and abbreviations explained rather than
  assumed. A heading tells you what a thing *is*, in words a smart non-specialist
  would follow.
- **Agent-facing writing** — specifications and development plans — is written
  **for other agents**. It can be as terse, dense and jargon-heavy as it needs
  to be, because its readers are machines doing implementation work.

This split has a direct consequence for the design (see §5.2): a person browsing
the project reads the human-facing writing, and the agent-facing documents are
available but never the thing put in front of a human first. It also binds the
chat agent that *writes* descriptions (§5.4): it must write in the human-facing
style, not summarise a spec in spec-language.

## 5. The object model

### 5.1 What gets a page

Anything in Cromwell's vocabulary (vision §3) that a person would want to send
someone a link to.

| Thing | Web address | The body of the page is | Stage |
|---|---|---|---|
| **Project** | `/ui` | The project's top-level design writing, its top initiatives, and its project-level milestones and roadmaps | A |
| **Initiative** | `/ui/i/<path>` | Its main design document, rendered | A |
| **Feature** | `/ui/f/<path>` | Its main design document, rendered | A |
| **Document** | `/ui/d/<path>` | The rendered document and its comment thread | A |
| **Checkpoint** (a question waiting for a human) | `/ui/inbox/<id>` | The question, its context, and the answers on offer | A |
| **Milestone** | `/ui/m/<id>` | Its description, what it contains, and how far along it is. Reached by clicking through from its parent entity or from a member — **not** from any global list (§5.1a) | B |
| **Roadmap** | `/ui/r/<id>` | Its milestones, in order. Reached from its parent entity — no global list | B |
| **Task** | `/ui/t/<id>` | The record of the agent run, the code diff, and any findings | B |

Web addresses use the readable entity paths the system already uses
(`auth`, `auth/login`), not scrambled identifiers — so the address bar stays
meaningful and `/ui/i/auth/login` means what it looks like it means. Every view
has a real address. HTMX swaps panels *within* a page, but moving between things
is real navigation you can bookmark and share.

In Stage A, the milestone, roadmap and task pages are **present but plain** — a
simple read-only page, enough that links from other pages resolve and nothing is
a dead end. Their full working surfaces come in Stage B (§8).

### 5.1a Milestones and roadmaps belong to an entity

A milestone and a roadmap each have **two** relationships, and keeping them
distinct is the whole of this section:

- **What it contains** — its members. A milestone reaches *down* the tree to
  group the initiatives and features shipping together, live (vision §4: "a
  milestone references its members live"); a roadmap orders milestones. This is
  the existing model and it is unchanged.
- **Where it belongs** — its **parent entity**. This is new. Every milestone and
  every roadmap is owned by exactly one entity: an **initiative**, or the
  **project** itself.

The parent relationship exists so that **planning can be local**. An initiative
can have its own roadmap of its own milestones — a plan for *its* corner of the
tree — kept separately from that initiative's own state of work. A team planning
the authentication initiative lays out its milestones on the authentication
page, not in a global pile mixed with everyone else's. Local planning lives in
the entity tree, at the level it belongs to. The project, as the root entity,
holds the project-wide roadmap and milestones — the ones that matter across the
whole effort.

This was Sam's intent from the start, under-expressed in the early designs, and
it is a deliberate change from the previous draft of this section (which made
them flat, project-only objects). It is a small, additive change to the data
model, and it follows a pattern the codebase already uses: **documents** are
already owned by `project` / `initiative` / `feature` via an `owner_type` plus a
nullable `owner_id`, where the project is the null-id root
([documents table](../../internal/store/migrations/0001_init.sql)). Milestones
and roadmaps gain the same pair of columns (owner ∈ {project, initiative}). No
new tables; the live membership model is untouched and orthogonal.

**How this shows up in the interface:**

- **A parent entity's page manages the milestones and roadmaps parented
  directly to it** — and *only* those directly parented to it. From the
  authentication initiative you manage authentication's milestones; you do not
  reach down and edit a sub-initiative's milestones from here. Each level owns
  its own. (§5.2 gives this its own section on the page.)
- **The project's milestones and roadmaps appear on the project page** (`/ui`),
  since the project is their parent.
- **There is no global "all milestones" or "all roadmaps" list.** Those screens
  would be noise — an undifferentiated pile the user has to sort in their head.
  Importance is expressed by *where* a roadmap is parented: the ones on the
  project page are the project-level-important ones, because a human chose to
  put them there.
- **An entity shows the milestones and roadmaps it is a *member* of** — "this
  feature is part of the Q3 milestone" — as a short list of titles that click
  through, not displayed in full (§5.2). For V1 this shows only the milestones
  it is *directly* a member of; showing every ancestor milestone it rolls up
  into is a later refinement, not needed now.

So the relationship is visible from both ends and they do not collide: *down*
from a milestone to its members, and *up* from a member to the milestones it
sits in — while *ownership* is a third, separate line that says which entity's
plan this milestone is part of.

**Ownership does not constrain membership.** A milestone owned by one initiative
can contain members from anywhere in the tree — a frontend milestone that
depends on a backend feature, say. This is the normal case, not an edge case
(vision §4: members can be "any combination of Initiatives, Features…"), and
constraining a milestone to its owner's subtree would force people to invent
throwaway parent initiatives just to group two trees. Ownership answers *whose
plan is this* (and whose page it lives on); membership answers *what ships
together*. They are orthogonal, and there is no subtree rule.

**Editing.** Composing a milestone (its members), ordering a roadmap, locking a
milestone — all of this happens **from the parent entity's page**, and since
each is just a list, a small **modal (pop-up)** is enough. HTMX does this cleanly
with a native `<dialog>` element and a loaded fragment; no extra library.

Because membership is unconstrained, members are added from **both ends**, which
also keeps every picker short:

- **From the member's side** (the object-shaped way, D-1): on any entity, an
  *"Add to a milestone…"* action picks *which milestone* — a short list, since
  there are few milestones. Cross-tree membership happens naturally here, because
  you add from wherever the thing lives; no global list of candidate members ever
  appears.
- **From the milestone's side**, when composing a plan: the add-member picker
  **defaults to the owner's subtree** — the common, local case, short and
  relevant — with a **search box to reach anything in the project** for the
  cross-tree member.
- **Removal** is always from the milestone's modal, which lists only its
  *current* members — inherently short however far the membership spans.

(All editing is Stage B; in Stage A these are read-only, and the owner column
simply lets the relationships *display*.)

### 5.1b The display is two lists — the same two lists Markdown has

This is the shape of the original idea and it should stay visible in the build.
A milestone and a roadmap are, at heart, the two kinds of list everyone already
knows from Markdown:

- **A milestone is an *unordered* list** — a bulleted list of its deliverables,
  each one marked done or not-done. Completion falls straight out of the list:
  the ticked items over the total.
- **A roadmap is an *ordered* list** — a numbered list of its milestones, in the
  planner's chosen order, each showing how far along it is.

(We settled that a roadmap orders *milestones* specifically, not arbitrary
deliverables — but it is still, on the page, an ordered list. The two-list
symmetry is a *display* truth even where the contents differ.)

The point of saying this out loud: **the simple interface falls out of the
structure, and we should let it.** A milestone renders as a checklist. A roadmap
renders as a numbered list. When Jobs and Checklists arrive (Stage B, §9), a
checklist is *literally* a list of checkboxes a human ticks by hand — no special
widget, the plain thing. We should resist dressing these up into dense tables
and dashboards (which is what the current build did); the list *is* the
interface.

**Milestone completion is shown two ways at once**, because there are two honest
questions:

- *How many deliverables are done?* — the checklist count, X of Y ticked. This
  is the completion a person reads at a glance, and it is what "checked off"
  means.
- *How much of the estimated work is done?* — a token bar, the finished tokens
  over the estimated total (tokens being the unit of work, §7). A milestone can
  be "3 of 4 done" but only "40% of the tokens," because the last deliverable is
  the big one. Both are true; showing both is honest.

### 5.2 What a page looks like

One layout for every entity, so that learning one page teaches you all of them:

```
┌──────────────────────────────────────────────────────────────────┐
│ Project ▸ Authentication ▸ Basic auth ▸ Email-and-password login   │  breadcrumbs
├───────────────────────────────────────────┬──────────────────────┤
│ # Email-and-password login                 │  STATUS  in progress │
│                                            │  SIZE    46,263      │
│ A short, human-readable summary of what    │          tokens 🟡   │
│ this is and why — two or three sentences.  │  DONE     51,004     │
│                                            │          tokens      │
│ <then the main design document, rendered   │  ──────────────────  │
│  from Markdown — the real writing, at a    │  Start work          │
│  comfortable reading width>                │  Estimate size       │
│                                            │  Attach a document   │
│                                            │  ⋯ more              │
├───────────────────────────────────────────┴──────────────────────┤
│  Children · Documents · Milestones · Activity                     │  sections
└──────────────────────────────────────────────────────────────────┘
```

- **Breadcrumbs** across the top carry the ancestry and are the main way back
  up. They are the browsable tree, experienced one level at a time.
- **The description** — one short paragraph in plain language — sits at the top
  of the body. It is the human-facing summary (§4), and it is the text that also
  shows up wherever this thing is listed elsewhere (in a parent's list of
  children, in search results, in a breadcrumb's tooltip). Where it comes from
  is §5.4.
- **The body** is the entity's **main design document**, rendered from Markdown
  at reading width — the featured document, the writing a person came to read.
  Specifications and development plans are *not* the body: those are agent-facing
  (§4) and live in the Documents section, reachable but never the first thing
  shown. If there is no design document yet, the body is a clear empty state that
  says so and offers "Attach a document" — so the *absence* of writing is
  visible rather than a blank space.
- **The rail** on the right is status, size-in-tokens (§7) and actions. The
  actions are exactly the ones that make sense for this thing in its current
  state. An action whose gate is not yet satisfied is shown **disabled with the
  reason in plain words** ("Can't start work yet — the design document is still
  in review"), never hidden and never a silent failure.
- **The sections** across the bottom are the entity's relations: its children;
  all of its documents; the **milestones and roadmaps parented here** — the
  entity's own local plan, managed from this page (create, and edit via a modal;
  Stage B); the **milestones it is a member of** — a short list of titles that
  click through, read-only (§5.1a); and its activity drawn from the audit trail.

**Are the sections tabs, links, or something else?** For Stage A they are
**stacked sections on the one page**, each with an anchor you can link to
(`/ui/f/auth/login#documents`), with a small jump-menu to skip between them. That
keeps the page a *document* — one place you scroll — rather than an app with
hidden panels, which fits the document-led idea. If a page later grows so heavy
that stacking becomes unwieldy, individual sections can become HTMX-loaded tabs
without changing anything else. Starting stacked is the more honest default.

> **Note, 2026-09-28 ([SPEC-012](../specs/SPEC-012-see-the-work.md) SD-14,
> approved by Sam).** A feature's page now carries its **timeline** between
> the page head and the body: one line of major moments, with a closed
> disclosure holding the detail, per DESIGN-010 §8. On feature pages it
> replaces the Activity section, whose events it carries as its detail level
> (SD-7). The design document is still the body. Project and initiative pages
> are unchanged.

### 5.3 Editing: what a person can change, and how

DESIGN-007's rule that the interface never edits document *bodies* is not a wall
around all editing — it is a line between two kinds of content, and the line is
exactly the human/agent split from §4.

There are three tiers, and V1 (Stage A) handles them differently:

1. **The description** (the short human summary) lives in the **database**, not
   in a git file. It is genuinely human-facing content, it is short, and editing
   it crosses no boundary — so **Stage A lets a person edit it in the interface.**
   A small inline edit on the page, saved to the database, attributed to the
   operator like any other action. This is the one thing V1 makes editable, and
   it is the right one: an AI-written summary that a human can correct in place.

2. **Human-authored design documents** (an initiative's or feature's main
   document) are Markdown files in git. The eventual goal is to let a person edit
   these in the interface too, since humans write them. For V1 we do **not** —
   we use the loop in §5.5 instead (edit in your own editor; the page updates
   itself). This is decision D-3, kept.

3. **Agent-authored documents** — specifications and development plans — are
   **never** editable by a human in the interface, now or later. These are
   written by agents, for agents. A person can read them, comment on them, and
   approve or reject them, but should stay out of writing them. (Vision §4: the
   specification is the contract the agents build against; a human hand-editing
   it defeats the point.)

So: descriptions are editable now; human design documents are edited via the
editor-and-git loop now and perhaps in-page later; agent documents are
read-only to humans forever.

### 5.4 Where the description comes from

Initiatives and features need that one-paragraph human summary (vision §4).
There is no server-side generator for it, and there should not be — a subagent
calling an API to summarise is the wrong shape for the planning half. Instead:

- **The chat agent writes it, conversationally.** Planning in Cromwell is a
  conversation with the human's chat agent (Claude Desktop, Zed, Cursor…), which
  creates the initiative or feature *and* writes its title and description in the
  same move, in the human-facing prose style of §4, and sets them through an MCP
  tool. This is the primary path, and it is why the description reads like a
  designer wrote it: a designer's assistant did.
- **A human can revise it, either way.** They can ask the chat agent to change
  it, or fix a word directly in the UI with a light in-place edit (§5.3 tier 1).

The `description` column already exists on initiatives and features, so this is
not a schema question. The MCP authoring tool that sets it is a peer deliverable
to the browsing UI (§6a), per [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md).

### 5.5 The editing loop for design documents

For the human-authored design documents that V1 does not edit in-page, the loop
turns DESIGN-007's "not a workspace" rule into the nicest interaction in the
product:

1. The page shows the document's location in the project, with a one-click
   "copy path".
2. The person edits the file in their own editor and commits it to git.
3. The git watcher (one of the three components in the vision's architecture)
   notices the file changed
   ([documents.go](../../internal/server/documents.go)).
4. The server pushes an update, and the open page re-renders the document in
   place.

Edit in your editor, watch the page update itself. Every piece of this already
exists and is proven; nothing new has to be built to get it. For a system whose
documents live in git and whose humans have real editors, this beats a text box
in a browser that would only fight the tools people already use.

## 6. The actions, moved onto their things

Here is where each of the thirteen forms goes. In every case: the thing being
acted on is wherever you already are (never typed), the underlying engine action
is unchanged, and the guarding rules and audit trail are unchanged.

| The form today | Where it goes |
|---|---|
| Set a size estimate | An inline control in the size area of the initiative or feature page |
| Ask the AI to estimate | A button next to it; the page shows the estimate being worked out, live |
| New sub-initiative | A button on the *parent's* page (parent implied). At the project root for top-level ones |
| New feature | A button on the initiative's page |
| Start work on a feature | The main button on the feature page; disabled with the reason until it is allowed |
| Abandon a feature | Under "more" on the feature page; still asks for a reason |
| Archive an initiative | Under "more" on the initiative page; still raises the human-approval checkpoint it does today |
| New milestone | On its **parent entity's** page — the initiative it plans, or the project — in the milestones-parented-here section (Stage B) |
| Add or remove a milestone member | Both ends (§5.1a): "Add to a milestone…" on any entity's page (picks the milestone); and the member list, with add (subtree-default + search) and remove, in a modal from the milestone on its parent's page (Stage B) |
| Lock a milestone | The milestone, on its parent's page; if it is not yet allowed, an inline explanation, exactly as today (Stage B) |
| New roadmap | On its **parent entity's** page — the initiative or the project (Stage B) |
| Order a roadmap | A modal opened from the roadmap (on its parent's page): the ordered milestone list (Stage B) |
| Approve / request changes on a document | Stays on the document page. It was already thing-first — the one form that was right |

Plus one **new** action, needed because
[DEC-003](../decisions/DEC-003-cli-scope.md) retires the command-line tool that
used to do it:

| New | Where |
|---|---|
| **Attach a document** — register a Markdown file as this entity's document | The initiative and feature pages. This fills the gap left by removing the old `doc add` command, and it is the central act of a document-led interface anyway |

The floating Actions panel and every "type the path here" box are deleted.

### 6a. Two surfaces, split at the pivot point

The web UI is not the only way a human acts on Cromwell. The **chat agent**, via
the MCP facet (the interface an assistant like Claude Desktop or Zed uses), is
the other — and the two divide along Cromwell's planning/developing seam, per
[DEC-004](../decisions/DEC-004-mcp-planning-authoring.md):

- **The chat agent (MCP) authors the planning layer.** Because planning is a
  conversation, the chat agent is where structure is *made*: it creates
  initiatives and features, writes their titles and descriptions, and attaches
  and drafts documents. This is the primary authoring path.
- **The web UI browses, reviews, and drives lifecycle.** It is where you *look
  at* the project, judge it, and act on it: read documents as pages, start a
  feature into development, answer a gate checkpoint, approve or request changes
  on a review. The development-side human authority lives here and only here — a
  chat agent may not start features, override gates, or spawn agents (DEC-004,
  the anti-rogue-orchestrator line).

The two surfaces overlap on purpose in one place: **the UI keeps the create and
edit actions in §6 as a convenience**, so a human already at the command centre
can add a child or fix a description without switching to their chat agent. Both
call the same gated, audited service methods (D-8); neither has authority the
other lacks. Authoring is *primarily* the chat agent's job and *also* available
in the UI — not one or the other.

The consequence for the roadmap: the MCP authoring tools are a **peer
deliverable** to this UI, landing alongside Stage A, not a later afterthought.
Without them there is no conversational way to create the things the UI browses,
and the vision's three components (Web UI, MCP facet, git watcher) were always
meant to be peers.

## 7. Work is measured in tokens. Full stop.

Cromwell exists to capture **a unit of work**, and for AI agents the honest unit
of work is **tokens** — not money, not time. Earlier drafts (and the vision's
§5 and §8) also carried a *money* reading of "cost": dollars per feature, a
monthly dollar budget, price tables. That was a conflation of two different
ideas under one word. This workflow does not need money as a metric, and money
is **removed from the human surface**:

- **Everything is shown in tokens.** Every page, every list, every roll-up shows
  the estimated size and the actual work done in tokens, with the confidence
  marker (🟢 decomposed from children, 🟡 considered against past work, 🔴 a rough
  guess) and a plain `?` for anything not yet estimated (vision §5). Tokens are
  the number a planner reads.
- **There is no money view, no dollar figure, no dollar budget on any page.** The
  screen that used to be "Cost" becomes **"Work"** — the same roll-ups (per
  initiative, per feature, per milestone, per project) but counted in tokens.
- **The dashboard's headline changes** from *what has this cost* to **what needs
  doing**: what is waiting on you, what is running, what is unestimated, what is
  at risk of slipping.

One loose end this creates belongs to the engine, not to this surface: the
engine still has money machinery underneath — a US-dollar cost ledger, a price
table, and a dollar-denominated spending cap that stops a runaway agent loop.
Sam's call (§12, Q-C) is to **leave that machinery dormant** — it may be useful
later — while the interface shows only tokens. Whether the runaway-loop guard is
eventually re-expressed in tokens, or the price table dropped, is a separate
engine decision with its own migration, deliberately not taken here. What §7
settles is the surface: **the human sees tokens, and only tokens.**

## 8. The five screens, re-cast

Every screen becomes a way *into* the web of things, rather than a dead-end
surface.

1. **Home** (`/ui`) — the project's own page. Work-first: questions waiting for
   you, agent runs in flight, features grouped by where they are in their life,
   what still needs estimating. Plus the project's top-level design writing and
   its top initiatives, as the way into browsing — and the **project-level
   milestones and roadmaps** parented here (§5.1a), each linking through to
   itself. (Milestones and roadmaps owned by initiatives appear on those
   initiatives' pages, not here.)
2. **Browse** (`/ui/i/…`) — replaces the old *Planning* screen. The entity pages
   from §5, moved between by breadcrumb, by a page's list of children, and by a
   tree in the side rail. Here the tree is **just navigation furniture**, not a
   place you work. The size-roll-up tree (the analytical view) comes back in
   Stage B.
3. **Inbox** (`/ui/inbox`) — the questions waiting for a human. Kept as it works
   today, restyled. It is the backbone of the whole human-in-the-loop idea, and
   its per-question answer forms are correct. Each question gains a shareable
   link and a link to the thing it is about.
4. **Documents** (`/ui/documents`) — an index of every document across the
   project, filterable by state and kind, each row linking to that document's
   page. The document page itself gains its provenance (which entity owns it,
   where it is in git, which commit) and keeps its review buttons.
5. **Work** (`/ui/work`) — formerly *Cost*. The roll-ups, counted in tokens
   (§7): where the work is, estimated versus actual, per initiative, feature,
   milestone and project.

## 9. Staging

**Stage A — document-led browsing** ([SPEC-007](../specs/SPEC-007-workflow-surface-stage-a.md),
with the MCP authoring slice [SPEC-008](../specs/SPEC-008-mcp-facet-planning-authoring.md)
as its peer, §6a):

the shell and navigation; entity pages for Project, Initiative, Feature and
Document; breadcrumbs and real web addresses; documents rendered as page bodies;
the description, written by the chat agent and editable in the UI; the
edit-in-your-editor loop; the actions moved onto their things (§6); "Attach a
document"; the re-weighted, work-first Home. Milestones and roadmaps get their
**parent-owner column** (the
additive migration, §5.1a) and are **displayed** read-only — the project's on
the project page, an initiative's on its page, and the "member of…" lists on
entity pages — so the relationships are visible and links resolve. The Inbox
and Work screens restyled.

**Stage B — the working surfaces** (a later package):

milestone and roadmap **editing via modals from their parent's page** —
composing members, ordering, locking — with the milestone rendered as a
checklist and completion shown two ways (count and tokens, §5.1b); **Checklists
and Jobs** built here (lifting the SD-2 deferral), so a milestone can include a
list of human-ticked jobs as a deliverable, edited as plain checkboxes; the
initiative tree brought back as the size-roll-up analytical view; a filterable
list of work across features and tasks; task pages with the run detail; velocity
on Home.

**Then** — the [DEC-003](../decisions/DEC-003-cli-scope.md) removal of the old
command-line tool, once Stages A and B demonstrably cover everything the retired
commands did.

Stage A is the slice that has to stand on its own. Its claim: **a person can
understand and drive the project by browsing it, without typing an entity's path
anywhere.** That is a claim you can check, and it is the right bar for the slice.

## 10. What this document does not cover

- **The visual look** — fonts, colour, spacing, the styling of individual
  components. This document fixes the structure and the interaction. The visual
  layer is production work and belongs in the specification and the build.
- **Multi-user sign-in** — still deferred; still a single configured operator.
- **In-page editing of design documents** — the goal (§5.3 tier 2), but not V1.
  V1 edits descriptions in-page and design documents via the editor loop.
- **The fate of the money machinery underneath** — the engine's dollar ledger,
  price table and spending cap are left dormant (§12 Q-C); whether to re-express
  or remove them is a later engine decision, out of scope here.
- **The chat agent's full MCP toolset** — this document names the *authoring*
  half of the MCP facet (§6a), specced as [SPEC-008](../specs/SPEC-008-mcp-facet-planning-authoring.md)
  and a peer of the browsing UI. The facet's other tools (content search,
  doc-intel lookups, checkpoint responding, review dispatch — vision §7) are a
  later MCP slice, out of scope here.
- **Jobs and Checklists** — scheduled for Stage B (§9): a checklist is a
  deliverable a milestone can contain, rendered as plain human-ticked checkboxes
  (§5.1b). Not in Stage A.
- **Defects** — a vocabulary member not yet built. The page layout in §5.2 is
  meant to take it on later without a new pattern; nothing here blocks it.

## 11. Decisions recorded here

- **D-1 — Thing-first, not action-first.** Every entity is a page with a real
  address; every action lives on the thing it acts on; nothing is addressed by
  typing its path.
- **D-2 — An entity's page is its main design document.** The human-facing
  writing is the body; status and relations are the frame. The specification and
  development plan are agent-facing and never the body (Sam's Q1: design-first,
  because this interface is for the humans who do design).
- **D-3 — Design documents are edited in the author's editor**, with the git
  watcher and live update doing the rest — not in a browser text box. **The
  short description is the exception**: it lives in the database and *is* editable
  in-page from V1.
- **D-4 — Work is measured in tokens; money is removed from the human surface.**
  No dollar figure, budget or cost view on any page. This revises vision §5/§8.
- **D-5 — Descriptions are written by the chat agent, conversationally**, in
  plain human prose, when it creates the entity (§5.4) — not by a server-side
  generator (that idea is dropped). A person can revise them via the chat agent
  or a light in-place UI edit.
- **D-6 — Human-facing text is written for humans; agent-facing text may be
  terse.** Headings, descriptions, labels and notices are full, clear sentences
  with terms explained. Specs and dev-plans are the only place compressed
  agent-to-agent writing belongs. This applies to the interface's copy, to the
  chat agent when it writes titles and descriptions (§5.4), and to the people and
  agents writing project documentation.
- **D-7 — The tree is navigation, not a work surface.** Browsing is one page at
  a time with breadcrumbs; the roll-up tree returns as an analytical view in
  Stage B.
- **D-8 — The redesign moves buttons, not authority.** Same engine actions, same
  guarding rules, same audit trail, same actor. The risk is presentational.
- **D-9 — Milestones and roadmaps have a parent entity** (§5.1a): an initiative
  or the project, so that planning is local to the level it belongs to. They
  keep their live, downward membership (unchanged, orthogonal). Each parent
  manages only the milestones and roadmaps parented *directly* to it, edited in
  a modal from that page. No global list of all milestones or roadmaps. An
  additive data-model change mirroring how documents are owned.
- **D-10 — The display is two lists, the two Markdown ones** (§5.1b). A milestone
  renders as an unordered checklist; a roadmap as an ordered numbered list. The
  simple interface falls out of the structure — checklists are literally
  checkboxes — and should not be dressed up into dense tables. Milestone
  completion is shown two ways at once: the ticked count (X of Y) and a token
  bar (how much estimated work is done).
- **D-11 — Roadmap contents stay milestones-only** (Sam's call). A roadmap
  orders milestones, not arbitrary deliverables — the vision §3 model, kept over
  restoring the original's fully-symmetric "list of deliverables." No migration.
  The two-list *display* symmetry (D-10) still holds. **Checklists and Jobs land
  in Stage B** as milestone deliverables (§9), lifting the SD-2 deferral.
- **D-12 — Ownership does not constrain membership** (§5.1a, §13). A milestone
  may contain members from anywhere in the tree, not just its owner's subtree;
  ownership and membership are orthogonal. Pickers are kept short by editing
  membership from both ends (add-from-member; subtree-default-plus-search on the
  milestone side), not by a constraint.
- **D-13 — Two surfaces, split at the pivot** (§6a,
  [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md)). The chat agent
  (MCP) authors the planning layer — creating initiatives and features, writing
  titles and descriptions, attaching documents; the web UI browses, reviews and
  drives lifecycle, and holds the development-side human authority (starting
  features, gates, reviews) that MCP must never touch. The UI also keeps the
  create/edit actions as a convenience. Descriptions have no server-side
  generator (D-5 is revised: the chat agent writes them conversationally). The
  MCP authoring tools are a peer deliverable to Stage A.

## 12. Resolved questions (Sam, 2026-07-24)

These were open in the previous draft and are now settled. Recorded here so the
reasoning survives into SPEC-007.

- **Q-A — The description field: confirmed, then simplified.** A short,
  human-prose description on initiatives and features, shown in the UI. The
  `description` column already exists, so there is no schema addition. It is
  written by the chat agent when it creates the entity (§5.4, D-5) and editable
  by a human in the UI. The earlier idea of a server-side generator and a
  human-edited flag was dropped once authoring moved to the chat agent (D-13):
  there is nothing to guard against clobbering, because a human or their agent
  writes it deliberately.
- **Q-B — "Main design document": confirmed.** The featured body is the first
  design-type document attached, overridable by a "make this the main document"
  action on the entity page.
- **Q-C — The money machinery: keep it dormant.** Sam's call: leave the dollar
  cost ledger, price table and spending-cap in the engine — they may be useful
  later — but the interface shows **only tokens** (§7). Sam also noted a real
  maintenance cost in the money path regardless: AI providers change prices
  often, so any dollar figure carries a price-table-freshness burden the token
  count does not. That is another reason tokens lead the surface. No engine
  change now; no migration; option (a) of the three.
- **Q3 — the plain Stage-A milestone page: confirmed.** A read-only view (name,
  description, members, progress in tokens) is enough for Stage A so links
  resolve; the composition/locking surface (a modal from the parent's page) is
  Stage B. To be iterated on once the design is tested in use.
- **Parent ownership of milestones and roadmaps: adopted** (§5.1a, D-9). Sam
  confirmed this was his original intent — localised planning in the entity
  tree — even though the early designs left it implicit and the data model
  (correctly, for membership) stored them flat. The interface and a small
  additive migration bring the parent relationship in. Editing is a modal from
  the parent (HTMX `<dialog>`), not in-place.

## 13. Resolved: membership is not constrained by ownership (Q-D)

Q-D asked whether a milestone's members must lie within its owner's subtree. The
answer is **no** (Sam, 2026-07-24). A milestone is cross-cutting by definition;
a frontend milestone may depend on a backend feature, and a subtree rule would
force throwaway parent initiatives just to group two trees. Ownership (§5.1a)
answers *whose plan this is* and where it lives; membership answers *what ships
together*. They are orthogonal — no constraint, and so no rule for the engine to
enforce. The picker is kept short instead by editing membership from both ends
(§5.1a): add-from-the-member for cross-tree, and a subtree-default-plus-search
picker on the milestone side. Recorded as D-12.

**With Q-D resolved, the design is closed.** It is realised by two specs —
[SPEC-007](../specs/SPEC-007-workflow-surface-stage-a.md) (the browsing UI) and
its peer [SPEC-008](../specs/SPEC-008-mcp-facet-planning-authoring.md) (the MCP
authoring slice) — over the decisions [DEC-003](../decisions/DEC-003-cli-scope.md)
and [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md). The whole package
goes through one authoring consistency review, then Sam's approval before any
code.
