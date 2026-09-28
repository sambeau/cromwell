# SPEC-010: Milestones and roadmaps you can edit

**Status:** **Draft — for Sam's approval.** Authored by Claude. The consistency
review is [REVIEW-010](../reviews/REVIEW-010-milestones-and-roadmaps-editing.md);
its findings are folded in below. The author can't be the approval gate, so the
decision is Sam's. Sam has said the spec and the build will be approved
together.
**Date:** 2026-09-28
**Roadmap milestone:** M4 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11
**Parent design:** [DESIGN-008](../design/DESIGN-008-the-workflow-surface.md)
§5.1a (ownership and the two ends of membership), §5.1b (two lists), §6 (where
each action goes), §9 (Stage B), decisions D-9 to D-13
**Entry criteria:** [Stage B entry criteria](../notes/spec-007-stage-b-entry-criteria.md).
This spec answers its questions 1 to 3 and defers 4 and 5 (§2).
**Authority:** [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) (the
chat agent may author the planning layer),
[DEC-005](../decisions/DEC-005-the-orchestration-boundary.md) (widening the
facet is always deliberate), and
[DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1 (acts
are sorted by consequence, not by channel)
**Builds on:** [SPEC-006](SPEC-006-command-centre-mutations.md) FR-2 and FR-3,
which built and tested the service methods, and
[SPEC-008](SPEC-008-mcp-facet-planning-authoring.md), the MCP facet's first slice

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, empty state, gate reason and tool description a person
will read is written in full sentences (DESIGN-008 D-6).

## 0. Framing

People plan Subutai with milestones and roadmaps. Today they can see them in
the web UI, but they have to use the command line to create or change one, and
the chat project manager can't touch them at all. The command line is due to
be retired (DEC-003), so this is the most visible gap on the planning side.

The engine is already there. SPEC-006 built and tested every operation this
spec needs: `CreateMilestone`, `AddMember`, `RemoveMember`, `LockMilestone`
(gate G4), `CreateRoadmap` and `SetRoadmapEntry`, each audited in its own
transaction. SPEC-007 then deliberately hid them from the web UI while the page
anatomy was rebuilt (its SD-1). This spec puts them back, on the pages where
DESIGN-008 says they belong, and gives the chat agent matching tools.

It is mostly a rendering increment. The store changes are small and listed in
FR-1; everything else is forms, a modal and MCP tools over existing methods.

## 1. Goal

**One claim, which the definition of done checks directly:**

> A person can build a two-milestone roadmap for an initiative entirely in the
> browser — create both milestones, fill them from both ends, put them on a
> roadmap in order, and lock one — and the chat agent can do the same over MCP,
> except the lock.

Two supporting claims:

> Every change, from either surface, is on the audit trail under the actor who
> made it.

> Nothing here lets anyone do what they couldn't do before. G4 still blocks a
> lock it should block, with no way round it, and the chat agent gains no act
> that can't be undone.

## 2. Scope

### In scope

1. **Owner-scoped creation** (FR-1): milestones and roadmaps can be owned by the
   project or by an initiative.
2. **Creating** a milestone or a roadmap from a project or initiative page
   (FR-2).
3. **Composing a milestone** from both ends: "Add to a milestone…" on any
   feature, initiative or milestone page, and a member picker on the milestone
   side (FR-3).
4. **Ordering a roadmap**: placing, moving and taking off milestones (FR-4).
5. **Locking a milestone** (G4), with a built-in confirm step and the gate's
   reason in plain words (FR-5).
6. **The modal** that carries the milestone and roadmap editors (FR-6).
7. **MCP tools** for everything above except locking (FR-7).

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| Checklists and jobs (entry criterion 4) | **M5**, the next milestone. FR-3.4 keeps the picker ready for a fourth member type. |
| The size roll-up tree and the filterable work list (entry criterion 5) | A later Stage B slice. Both are read surfaces with no editing. |
| Renaming, re-describing or deleting a milestone or roadmap | Not specified here; no service method exists. Recorded as a follow-up in the handoff. |
| Moving a milestone or roadmap to a different owner | Not specified here. Ownership is set at creation. |
| Drag-to-reorder on a roadmap | Polish, as SPEC-006 §2 already said. The handler shape (FR-4.2) takes a target position, so drag can use it later unchanged. |
| An MCP tool to lock a milestone | **Deliberately not built.** See SD-4. Sam may overrule it. |
| Anything in DESIGN-010 or DEC-007 | M2. |

### Scope decisions

- **SD-1 — Stage B entry criteria 1 to 3 are answered here; 4 and 5 are
  not.** The entry criteria asked whether checklists should be their own slice.
  They are: M5. The roll-up tree and work list are read surfaces with a
  presentation question, and belong with other read work.

- **SD-2 — Owner-scoped creation is a signature change, and the CLI keeps
  defaulting to the project.** `CreateMilestone` and `CreateRoadmap` gain an
  owner (type and optional id). The JSON API and therefore the CLI pass the
  project, exactly as today, until DEC-003 retires the CLI. Only the web UI and
  MCP create initiative-owned plans.

- **SD-3 — Two small store additions beyond the entry criteria, both
  recorded.** The entry criteria said nothing but owner-scoped creation was
  needed in the store. Building the roadmap editor showed two gaps:
  - **Reordering needs dense positions.** `SetRoadmapEntry` stores whatever
    integer it is given, so two entries can share a position, and "move up" can
    silently do nothing. A new `PlaceRoadmapEntry` puts a milestone at a
    1-based place in the current order and renumbers the whole roadmap 1…n in
    one transaction, with one audit row. `SetRoadmapEntry` stays for the API.
  - **There is no way to take a milestone off a roadmap.** Without one, a
    mistaken placement is permanent. A new `RemoveRoadmapEntry` deletes the
    entry and audits it. It is reversible (place it again), so it sits on both
    surfaces.
  - A third, smaller change hardens `AddMember`: **a milestone can't contain
    itself, directly or through another milestone.** The resolver already
    tolerates cycles, but a cycle has no planning meaning, and a picker on both
    ends makes one easy to create by accident.

- **SD-4 — No MCP tool locks a milestone.** DEC-006 Amendment 1 sorts acts by
  consequence: "how bad and how recoverable a misuse would be". Locking is the
  one act in this slice that can't be undone. It freezes what the milestone
  promised, forever, and there is no unlock. Every other act here can be
  reversed by its opposite: add and remove, place and take off. So the lock
  stays a web UI act, where a person sees the confirm step (FR-5.2). The tool
  set excludes it by omission, and `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
  names `lock_milestone` as a tool that must not exist.
  **This is a judgement Sam may overrule.** The case for a lock tool is that
  G4 still guards it and the human might genuinely have said "lock it". The
  case against is that a mistaken relay can't be recovered, which is exactly
  the line Amendment 1 draws. If Sam wants it, it should be a *relay* tool
  carrying the human's quoted words, as Amendment 1 requires of relays.

- **SD-5 — The MCP tools are planning authoring under DEC-004, not relay.**
  DEC-004 lets the chat agent "create and edit the planning layer". Milestones
  and roadmaps are planning structure: DESIGN-008 §6 lists them with the
  planning actions, and none of them starts, spends or gates anything. So the
  new tools need no new decision, and they don't carry quoted human words the
  way DEC-006 relay tools do. Widening the tool set is still deliberate: the
  test names each new tool (DEC-005).

- **SD-6 — Membership is edited from both ends, removal included.**
  DESIGN-008 §5.1a has additions from both ends and says "removal is always
  from the milestone's modal". The M4 brief asks for add *and remove* from both
  ends. Both are satisfied: the milestone's modal can remove any member, and
  the member's own "Milestones" dialog lists the milestones it is directly in,
  each with a way to take it out. That list is inherently short, which was the
  design's reason for putting removal on the milestone side, so nothing the
  design protected is lost.

- **SD-7 — The search is a plain name and path query, not the full-text
  index.** Entry criterion 3 asked which. The full-text index is over *document
  sections* (`sections.fts`), not over initiatives, features or milestones, so
  "reusing" it would mean matching entities by the prose of their documents.
  That finds the wrong things: a feature whose design mentions "login" is not
  the feature called Login. English stemming and whole-word matching also suit
  prose, not slugs: "auth" should find `auth/basic`. The candidate set is small
  (a project has hundreds of entities, not millions), so a case-insensitive
  substring match over name and path is fast without an index, always
  current, and matches what a person types. FR-3.3 specifies it.

- **SD-8 — The modal is a fragment loaded by HTMX; the create forms stay
  inline.** DESIGN-008 §5.1a asks for "a native `<dialog>` element and a
  loaded fragment". Entry criterion 2 asked for the routes, how the dialog
  closes and what it swaps back. FR-6 answers all three. The create dialogs and
  the member-side dialog need no server state beyond what the page already
  has, so they are inline `<dialog>` elements like every existing modal.

- **SD-9 — Every form also works as a plain form post.** Each editor form
  carries both `hx-post` and a plain `method="post" action=…`. Under HTMX the
  handler returns the editor fragment; without it, the handler re-renders the
  owner's page with a notice, which is the existing pattern (SPEC-007 FR-5).
  This keeps one handler per act, keeps the tests simple, and means nothing
  breaks if the script fails to load.

## 3. Requirements

Each requirement names the service method it renders. "Owner page" means the
project page (`/ui/project`) or an initiative page (`/ui/i/<path>`).

### FR-1: Owner-scoped creation and the store additions

- **FR-1.1** `CreateMilestone(ctx, tx, ownerType, ownerID, name, description,
  targetDate, actor)` and `CreateRoadmap(ctx, tx, ownerType, ownerID, name,
  actor)` take an owner: `project` with a nil id, or `initiative` with an
  initiative's id. Any other combination is refused before anything is
  written. A blank name is refused with "A milestone needs a name." (or
  "roadmap"). The audit row records the owner.
  *AC:* creating each kind with each owner kind stores the owner columns as
  given and writes one audit row naming the owner; a bad owner or a blank name
  writes nothing.
- **FR-1.2** The JSON API (`POST /api/milestones`, `POST /api/roadmaps`) passes
  the project, so the CLI's `milestone create` and `roadmap create` behave
  exactly as before (SD-2).
  *AC:* the existing phase 3 tests pass unchanged.
- **FR-1.3** `PlaceRoadmapEntry(ctx, tx, roadmapID, milestoneID, place, actor)`
  puts the milestone at the 1-based `place` in the roadmap's current order
  (inserting it, or moving it if it is already there), clamps `place` to
  1…n, renumbers every entry 1…n, and writes one `roadmap.entry_set` audit row
  with the place. `place` 0 means "at the end".
  *AC:* placing, moving up, moving down and moving to the end all produce the
  expected order with dense positions, including on a roadmap whose existing
  positions tie.
- **FR-1.4** `RemoveRoadmapEntry(ctx, tx, roadmapID, milestoneID, actor)`
  deletes the entry, renumbers the rest, and writes a `roadmap.entry_removed`
  audit row. Removing a milestone that isn't on the roadmap is a not-found
  error.
  *AC:* after removal the milestone is gone from `RoadmapEntries`, the rest keep
  their relative order, and the audit row exists.
- **FR-1.5** `AddMember` refuses to put a milestone inside itself, or inside a
  milestone it already contains (directly or through nesting), with a
  plain-words reason. Nothing is written.
  *AC:* adding A to A, and adding A to B when B is inside A, both fail with the
  reason; adding an unrelated milestone still works.

### FR-2: Creating from the owner's page

- **FR-2.1** The project page and every initiative page always show the
  "own plan" section, even when it is empty. Its empty state says what
  milestones and roadmaps are for and offers **New milestone** and **New
  roadmap**. The same two actions are in the page's "more" menu.
- **FR-2.2** **New milestone** opens a dialog asking for a name, an optional
  target date and an optional description. It posts to
  `POST /ui/milestone/new` with the owner carried in hidden fields
  (`owner_type`, `id`), calls `CreateMilestone` with that owner, and
  re-renders the owner's page with a notice naming the new milestone.
- **FR-2.3** **New roadmap** is the same with a name only, posting to
  `POST /ui/roadmap/new` and calling `CreateRoadmap`.
- **FR-2.4** A rejection (blank name, malformed date) re-renders the owner's
  page with the reason in a full sentence and writes nothing.
  *AC for FR-2:* creating each from the project page and from an initiative
  page stores the right owner and shows the new item in that page's plan
  section, and only there; each is audited to the UI actor.

### FR-3: Composing a milestone from both ends

- **FR-3.1 From the member's side.** Feature, initiative and milestone pages
  have a **Milestones…** action in the "more" menu. It opens a dialog with two
  parts:
  1. *The milestones this is directly part of*, each linked, with a **Take it
     out** button on each open one (SD-6). An optional reason is recorded on
     the audit trail, as the descope path always has (FR-5.3 of SPEC-003).
  2. *Add to a milestone*: a select of the open milestones it could join,
     grouped by where each is planned ("Planned in Project", "Planned in
     Authentication"). It excludes locked milestones, ones it is already
     directly in, and, on a milestone page, the milestone itself and any
     milestone it contains (FR-1.5).
  Both post to `POST /ui/milestone/member/add` or `/remove` with
  `milestone_id`, `member_type`, `member_id` and `from=member`; the response
  re-renders the member's page with a notice.
- **FR-3.2 From the milestone's side.** The milestone editor (FR-6) lists the
  current members as a checklist (D-10), each with a **Take it out** button,
  and below it an **Add to this milestone** picker. With the search box empty,
  the picker lists candidates from the owner's subtree: for an
  initiative-owned milestone, that initiative, everything under it, and the
  other milestones it owns; for a project-owned milestone, the whole project.
  Typing in the search box widens to the whole project (§5.1a). Each candidate
  shows its kind, name and path, and has an **Add** button.
- **FR-3.3 The candidate read.** A new read, `MemberCandidates(ctx, q,
  milestoneID, scopeRoot, query, limit)`, returns initiatives (not archived),
  features (not abandoned) and milestones (other than this one), each with
  kind, id, name and path. With an empty query it is limited to `scopeRoot`'s
  subtree (nil means the whole project); with a query it matches the
  project-wide set by case-insensitive substring over name and path, with
  `%` and `_` in the query treated literally (SD-7). It leaves out what is
  already a direct member. The picker shows at most 50 and says so when there
  are more.
- **FR-3.4 Room for a fourth member type.** The candidate read is a union of
  one branch per member type, and the picker renders any kind by its icon and
  label. Adding checklists in M5 means one more branch and one more icon, with
  no change to the picker's shape.
- **FR-3.5** Adding or removing on a locked milestone is refused by
  `AddMember`/`RemoveMember` as today; the editor doesn't offer either on a
  locked milestone, and says why ("This milestone is locked, so what it
  contains is fixed.").
  *AC for FR-3:* a feature added from its own page appears in the milestone's
  checklist; an initiative added from the milestone's picker appears in its
  own page's rail as a milestone it is part of; removal from either end
  removes the row and writes `milestone.member_removed`; a search for part of
  a name finds an entity outside the owner's subtree; the no-typed-path rule
  (NFR-4) holds on every page and fragment.

### FR-4: Ordering a roadmap

- **FR-4.1** The roadmap editor (FR-6) shows the roadmap as an ordered list
  (D-10). Each entry has **Move up**, **Move down** and **Take off the
  roadmap**; the first has no Move up and the last no Move down.
- **FR-4.2** Below the list, **Place a milestone** offers a select of the
  milestones not yet on this roadmap, grouped as "Planned here" (same owner as
  the roadmap) and "Planned elsewhere", and a place: "At the end" (the
  default), "First", or "After *name*" for each entry. It posts to
  `POST /ui/roadmap/entry/place` with `roadmap_id`, `milestone_id` and
  `place`, calling `PlaceRoadmapEntry`. Move up and Move down post the same
  route with the entry's new place.
- **FR-4.3** **Take off the roadmap** posts to `POST /ui/roadmap/entry/remove`
  and calls `RemoveRoadmapEntry`. The milestone itself is untouched.
  *AC for FR-4:* placing two milestones and moving the second up renders them
  in the new order on the editor, the roadmap page and the owner page, and
  `RoadmapEntries` agrees; each act is audited.

### FR-5: Locking a milestone

- **FR-5.1** The milestone editor has a **Lock this milestone** section. It
  first computes G4 over the live membership, as `LockMilestone` will. When G4
  would refuse, the button is disabled and its reason is shown in plain words
  beside it (DESIGN-008 §5.2): for example, "This milestone can't be locked
  yet, because none of its 3 deliverables is done. Locking records what
  actually shipped, so at least one has to be finished first."
- **FR-5.2** When G4 would pass, the section is a disclosure (`<details>`)
  titled **Lock this milestone…**. Opening it shows the confirm step, built
  into the page rather than a browser pop-up: "Locking is permanent. It takes
  a snapshot of the N features this milestone currently resolves to, and
  freezes its membership. It can't be unlocked." and a danger button **Lock it
  permanently**. The form carries `confirm=permanent`; the handler refuses a
  post without it.
- **FR-5.3** `POST /ui/milestone/lock` calls `LockMilestone` as the UI actor.
  On success the editor shows the milestone as locked with its snapshot
  progress. If G4 refuses at the moment of posting (the membership changed in
  between), the reason is shown inline in the same plain words and nothing is
  locked. There is no force path (L-6) and no checkpoint (SPEC-006 R6-1).
  *AC for FR-5:* locking an eligible milestone renders it locked with a
  `milestone.locked` audit row; an ineligible one shows the disabled button
  and reason; a post without the confirm value, or one G4 refuses, leaves it
  open.

### FR-6: The modal

- **FR-6.1 Routes.** One fragment route per editable object:
  `GET /ui/m/{id}/edit` returns the milestone editor, and
  `GET /ui/r/{id}/edit` the roadmap editor. Each is a `<dialog class="modal"
  data-autoshow>` wrapping an inner `<div id="…-editor">`.
- **FR-6.2 Opening.** Owner pages show an **Edit** button beside each
  milestone and roadmap in the plan section; milestone and roadmap pages show
  one in their header. The button uses `hx-get` into a page-level
  `#modal-slot`, and `dialog.js` calls `showModal()` on any `dialog[data-autoshow]`
  HTMX inserts. Escape, focus trapping and focus return stay the browser's job.
- **FR-6.3 Acting inside it.** Every form in the editor posts with HTMX and
  targets the editor's inner `<div>`, so the dialog stays open and shows the
  result — the updated list, a notice, or an inline error — and a person can
  compose several changes in one sitting.
- **FR-6.4 Closing.** The editor has a **Done** button. When the dialog closes
  by any means (Done, Escape, the backdrop), and the editor recorded a change
  (it carries `data-changed`), the page reloads so the owner's plan section is
  current. With no change, closing simply closes.
- **FR-6.5 Without HTMX.** The same routes answer a plain form post by
  re-rendering the owner's page with a notice (SD-9), and the fragment routes
  return a usable page-less fragment.
  *AC for FR-6:* the fragment routes return 200 with the dialog; an HTMX post
  returns the editor fragment, not a full page; a plain post returns the
  owner's page.

### FR-7: The chat agent's tools

All tools resolve milestones and roadmaps by id, or by exact name where that
name is unique (the existing `milestoneByRef`), and initiatives and features by
path, as the SPEC-008 tools do. Each write runs its store method in one
transaction as the MCP actor, is audited there (DEC-004, O-3), and signals the
SSE hub so an open page follows (SPEC-008 FR-5.1). Errors are full sentences a
chat agent can relay (SPEC-008 NFR-5).

- **FR-7.1 `create_milestone`** — `name`, optional `description`,
  `target_date` (YYYY-MM-DD) and `owner_path` (an initiative path; omitted
  means the project). Returns id, name, owner and page address.
- **FR-7.2 `create_roadmap`** — `name`, optional `owner_path`. Returns id,
  name, owner and page address.
- **FR-7.3 `add_milestone_member`** — `milestone`, `member_type` (`feature`,
  `initiative` or `milestone`) and `member` (a path, or a milestone id or
  name). The explicit type avoids guessing between an initiative and a feature
  that share a path shape.
- **FR-7.4 `remove_milestone_member`** — the same, plus an optional `reason`
  recorded on the audit trail.
- **FR-7.5 `place_roadmap_entry`** — `roadmap`, `milestone` and an optional
  1-based `position` (omitted means the end). It places or moves, so it is
  also the reorder tool.
- **FR-7.6 `remove_roadmap_entry`** — `roadmap` and `milestone`.
- **FR-7.7 `list_milestones` and `list_roadmaps`** — optional `owner_path`;
  with it, only what that initiative owns (or `"project"` for the project's
  own); without it, every one, each with its owner. A chat agent needs to
  resolve "the beta milestone" wherever it is planned, much as `get_tree`
  returns the whole tree. D-9's "no global list" is about the web UI's
  navigation, not the agent's lookups.
- **FR-7.8 `get_milestone` and `get_roadmap`** — one in detail: for a
  milestone, its owner, state, target date, members (kind, name, path, done)
  and progress both ways (items and tokens), plus whether it could be locked
  now and G4's reason, so the agent can tell the person; for a roadmap, its
  owner and its milestones in order, each with its progress.
- **FR-7.9 The boundary.** There is no `lock_milestone` tool (SD-4). The
  advertised set is the SPEC-008 nine plus these ten, and
  `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` names every one and lists
  `lock_milestone` among the names that must be unknown. The `initialize`
  instructions mention milestones and roadmaps, and say that locking is a
  person's act in the web UI.
  *AC for FR-7:* over `POST /mcp`, the agent builds a two-milestone roadmap for
  an initiative, fills both milestones, reorders the roadmap and reads it back;
  each write leaves an audit row whose actor is the MCP actor; a call to
  `lock_milestone` is method-not-found.

## 4. Non-functional requirements

- **NFR-1 — No new stack.** HTML, HTMX and the existing `dialog.js`; no new
  dependency, no bundler. `go vet ./...` and `go test -race ./...` stay clean.
- **NFR-2 — One service layer.** UI handlers and MCP tools call the store
  methods directly in a transaction. Neither issues HTTP to `/api/*`.
- **NFR-3 — No new authority.** G4 keeps its shape: a blocking inline reason,
  no checkpoint, no force (L-6). The chat agent gains only reversible planning
  acts (SD-4, SD-5).
- **NFR-4 — No typed entity path.** Every entity in a form is carried by id in
  a hidden field or chosen from a list (SD-6 of SPEC-007). Fields are named
  `milestone_id`, `roadmap_id`, `member_id`, never `milestone`, `roadmap`,
  `ref` or `path`, so `TestUIBrowseAndDrive`'s banned-name check covers them.
  The picker's search box is a search, named `q`; what it finds is then
  chosen, not typed.
- **NFR-5 — Two lists, not tables** (D-10). A milestone is edited as a
  checklist and a roadmap as an ordered list. No new table markup.
- **NFR-6 — Human prose** (D-6) in every label, empty state, notice, error,
  gate reason and tool description. No currency anywhere
  (`TestRenderedTemplatesCarryNoCurrency`).
- **NFR-7 — Audited both ways.** Every mutation from either surface writes its
  audit row in the same transaction (O-3), attributed to the UI actor or the
  MCP actor.
- **NFR-8 — Tested as before.** Integration tests drive each UI handler and
  each MCP tool against real Postgres (SPEC-006 NFR-4, SPEC-008 NFR-4), and the
  render tests cover the new templates.

## 5. Definition of done

1. Every FR's acceptance criteria pass in the suite, and the existing suite,
   including `TestUIBrowseAndDrive` and the render tests, still passes.
2. **A browser walkthrough without an AI provider.** Build `./cmd/cromwell`,
   `init` a throwaway project, `serve` it, and with Playwright build a
   two-milestone roadmap for an initiative entirely in the browser: create,
   add members from both ends, reorder, and lock one. Screenshots saved.
3. **The same over MCP**, by JSON-RPC calls to `POST /mcp`, except the lock,
   which is shown to be unknown.
4. Both recorded in `docs/walkthrough-spec-010.md`.
5. `go vet ./...` and `go test -race -count=1 ./...` clean.
6. A handoff note records what was built, what was decided, and what needs
   Sam: this spec's approval and SD-4.

## 6. Open questions carried into implementation

- **Renaming and describing a milestone or roadmap.** Out of scope (§2), but a
  typo in a milestone's name is now fixable only in the database. It is the
  most likely first follow-up; `UpdateEntityFields` could take a milestone.
- **Whether the owner page should refresh live** when the chat agent edits its
  plan. The MCP tools signal the SSE hub, but entity pages don't yet listen for
  it outside the document page. That is a wider SPEC-007 question and is left
  as it is.
