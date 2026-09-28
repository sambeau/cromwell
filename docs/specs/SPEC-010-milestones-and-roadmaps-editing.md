# SPEC-010: Milestones and roadmaps you can edit

**Status:** **Draft — for Sam's approval.** Authored by Claude. An independent
consistency review, [REVIEW-010](../reviews/REVIEW-010-milestones-and-roadmaps-editing.md),
found six material and twelve smaller problems in the first draft. All of them
are dealt with in this revision; §7 says how, finding by finding. The author
can't be the approval gate, so the decision is Sam's. Sam has said he will
approve the spec and the build together.
**Revised after Sam's decision (2026-09-28):** locking a milestone is now
called **marking it as shipped**, and it **can be undone** (SD-11, FR-5). §8
records what changed. Three other choices still need Sam's yes (§5, DoD 6).
**Date:** 2026-09-28
**Roadmap milestone:** M4 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11
**Parent design:** [DESIGN-008](../design/DESIGN-008-the-workflow-surface.md)
§5.1a (ownership and the two ends of membership), §5.1b (two lists), §6 (where
each action goes), §9 (Stage B), decisions D-9 to D-13
**Entry criteria:** [Stage B entry criteria](../notes/spec-007-stage-b-entry-criteria.md).
This spec answers its questions 1 to 3 and defers 4 and 5 (§2).
**Authority:** [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) (the
chat agent may author the planning layer, and may not touch gates),
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

The engine is mostly there. SPEC-006 built and tested the operations this spec
needs: `CreateMilestone`, `AddMember`, `RemoveMember`, `LockMilestone` (gate
G4), `CreateRoadmap` and `SetRoadmapEntry`, each audited in its own
transaction. SPEC-007 then deliberately hid them from the web UI while the page
anatomy was rebuilt (its SD-1). This spec puts them back, on the pages where
DESIGN-008 says they belong, and gives the chat agent matching tools.

It is mostly a rendering increment. There are five store changes: three in
FR-1, the candidate read in FR-3.3, and reopening a shipped milestone in
FR-5.4. G4's refusal reasons are also rewritten as sentences (FR-5.1).
Everything else is forms, two editors in a modal, and MCP tools over existing
methods.

**What milestones are for.** Milestones and roadmaps are for people: they
plan releases and report progress. Nothing in the agent workflow reads them.
The orchestrator, the dispatcher, the agents' prompts and the development gates
never look at a milestone, and no work starts, stops or waits because of one.
That shapes the choices below: none of these acts can affect what agents do.

## 1. Goal

**One claim, which the definition of done checks directly:**

> A person can build a two-milestone roadmap for an initiative entirely in the
> browser — create both milestones, fill them from both ends, put them on a
> roadmap in order, and mark one as shipped — and the chat agent can do the
> same over MCP, except marking it as shipped.

Two supporting claims:

> Every change, from either surface, is on the audit trail under the actor who
> made it.

> G4 still stops a milestone being marked as shipped when nothing in it is
> done, with no way round it. The chat agent can create, fill and order
> milestones and roadmaps, but it can't mark one as shipped or reopen it.

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
5. **Marking a milestone as shipped** (G4), and **reopening** it, with the
   gate's reason in plain words when it refuses (FR-5).
6. **The two editors** in a modal (FR-6).
7. **MCP tools** for everything above except marking as shipped and reopening
   (FR-7).

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| Checklists and jobs (entry criterion 4) | **M5**, the next milestone. FR-3.4 keeps the picker ready for a fourth member type. |
| The size roll-up tree and the filterable work list (entry criterion 5) | **Unscheduled.** No roadmap milestone holds them yet. Both are read surfaces with no editing; the handoff records the gap so they don't drop out of sight. |
| Renaming, re-describing or deleting a milestone or roadmap | **Not specified here**; no service method exists. It is the most likely first follow-up (§6), and it matters more now that the chat agent can create them. |
| Moving a milestone or roadmap to a different owner | Not specified here. Ownership is set at creation. |
| Drag-to-reorder on a roadmap | Polish, as SPEC-006 §2 already said. The handler takes a target position (FR-4.2), so drag can use it later unchanged. |
| MCP tools to mark a milestone as shipped or reopen it | **Not built.** See SD-4. Adding them needs a short amendment to DEC-004. |
| Reopening from the API or CLI | Not built. The CLI is being retired (DEC-003), and the web UI is enough. |
| Anything in DESIGN-010 or DEC-007 | M2. |

### Scope decisions

- **SD-1 — Stage B entry criteria 1 to 3 are answered here; 4 and 5 are
  not.** The entry criteria asked whether checklists should be their own slice.
  They are: M5. The roll-up tree and work list are read surfaces with a
  presentation question, and belong with other read work, which isn't
  scheduled yet.

- **SD-2 — Owner-scoped creation is a signature change, and the CLI keeps
  defaulting to the project.** `CreateMilestone` and `CreateRoadmap` gain an
  owner (type and optional id). The JSON API and therefore the CLI pass the
  project, exactly as today, until DEC-003 retires the CLI. Only the web UI and
  MCP create initiative-owned plans.

- **SD-3 — Five store changes, beyond the one the entry criteria
  expected.** The entry criteria said nothing but owner-scoped creation was
  needed in the store. Building the editors showed three more gaps, the picker
  needs a read, and Sam's decision to make shipping reversible needs an undo
  (`UnlockMilestone`, FR-5.4):
  - **Reordering needs dense positions.** `SetRoadmapEntry` stores whatever
    integer it is given, so two entries can share a position, and "move up" can
    silently do nothing. A new `PlaceRoadmapEntry` puts a milestone at a place
    in the current order and renumbers the whole roadmap 1…n in one
    transaction, with one audit row. `SetRoadmapEntry` stays for the API.
  - **There is no way to take a milestone off a roadmap.** Without one, a
    mistaken placement is permanent. A new `RemoveRoadmapEntry` deletes the
    entry and audits it. Placing it again undoes it, so it sits on both
    surfaces.
  - **A milestone can't contain itself**, directly or through another
    milestone. `AddMember` now refuses that. The resolver already tolerates
    cycles, but a cycle has no planning meaning, and a picker on both ends makes
    one easy to create by accident.
  - **`MemberCandidates`** is the milestone-side picker's read (FR-3.3).

- **SD-4 — For now, the chat agent can't mark a milestone as shipped or
  reopen it.** Before Sam's decision (SD-11) this was about safety: locking
  couldn't be undone, and a chat agent that misread "we're done with beta" would
  have made a permanent mistake. Now that shipping can be undone, that reason
  has gone, and the only thing in the way is wording. DEC-004 says the chat
  agent "may not … touch gates", and G4 is technically a gate, even though it
  guards a report rather than any agent work.

  So this build leaves both acts out, and the tool-set test lists
  `lock_milestone` and `unlock_milestone` as tools that must not exist. **The
  recommendation is to allow them later**, with a one-line amendment to
  DEC-004 that says G4 is a record-keeping check, not a development gate. It is
  small enough to do whenever the chat agent needs it. No quoted words would be
  needed, because a mistake is undone by reopening.

- **SD-5 — The chat agent edits milestones and roadmaps as ordinary
  planning.** DEC-004 lets the chat agent create and edit planning material
  without asking first. Its list names initiatives, features, titles,
  descriptions and documents, but not milestones and roadmaps, because they
  weren't in the chat agent's reach when it was written. This spec treats them
  like the rest. They say what ships together and in what order. Editing them
  doesn't start work, spend tokens or pass a gate, and everything the ten tools
  do can be undone, except creating a milestone. (A stray milestone is clutter
  until rename and delete exist; see §6.)

  The alternative would be to treat them as *relay* tools under DEC-006
  Amendment 1: tools that pass on a decision a person made, and must store that
  person's exact words. That would add a field to six tools and a step to every
  planning conversation, for acts that are easy to reverse. Sam is asked to
  confirm this reading (§5). If he does, a one-line amendment to DEC-004 naming
  milestones and roadmaps would stop the next person having to argue it again.

- **SD-6 — Two places where the build differs from DESIGN-008.** Both need
  Sam's acceptance. If he gives it, DESIGN-008 §5.1a should get a short dated
  note so the design and the build agree.
  1. **Taking something out of a milestone from its own page.** The design
     says you only remove members inside the milestone's editor, and that the
     "milestones it is part of" list on a feature's page is read-only. The M4
     brief asked for adding *and removing* from both ends, so a feature's or
     initiative's "Add to a milestone…" dialog also lists the milestones it is
     in, each with **Take it out…** (FR-3.1). The downside is that someone can
     remove a feature without seeing the rest of the milestone. That is a mild
     risk: it can be undone, a reason can be given, and it is logged.
  2. **An Edit button on the milestone's and roadmap's own pages.** The design
     says you edit from the owner's page (the project or initiative). This
     build also puts Edit on the milestone and roadmap pages (FR-6.2), because
     someone who has clicked through to a milestone expects to change it there.
     The owner's page is still the main place to edit.

- **SD-7 — The search is a plain name and path query, not the full-text
  index.** Entry criterion 3 asked which. The full-text index is over *document
  sections* (`document_sections.fts`, queried with `websearch_to_tsquery`), not
  over initiatives, features or milestones, so "reusing" it would mean matching
  entities by the prose of their documents. That finds the wrong things: a
  feature whose design mentions "login" is not the feature called Login.
  English stemming and whole-word matching also suit prose, not slugs: "auth"
  should find `auth/basic`. The candidate set is small (a project has hundreds
  of entities, not millions), so a case-insensitive substring match over name
  and path is fast without an index, always current, and matches what a person
  types. FR-3.3 specifies it.

- **SD-8 — The editors are fragments loaded by HTMX; the small dialogs stay
  inline.** DESIGN-008 §5.1a asks for "a native `<dialog>` element and a
  loaded fragment". Entry criterion 2 asked for the routes, how the dialog
  closes and what it swaps back; FR-6 answers all three. The milestone editor
  and the roadmap editor are loaded, because they hold state that changes as
  you work. The two create dialogs and the member-side dialog are inline
  `<dialog>` elements like every existing modal. The create dialogs need
  nothing from the server. The member-side dialog needs one new read per page
  (the open milestones this entity could join, `milestoneChoices`), which is
  cheap because milestones are few. Loading it as a fragment too would save
  that read and match the brief's wording more literally; it is a small
  change if Sam prefers it.

- **SD-9 — Which response a post gets is decided by explicit signals.** Every
  page's `<body>` has `hx-boost`, so an ordinary form post arrives through
  HTMX too, with `HX-Request: true` and `HX-Boosted: true`. So:
  - a post from inside an editor (`HX-Request` without `HX-Boosted`) gets the
    editor back, and the dialog stays open;
  - a post from the member-side dialog carries `from=member` and gets the
    member's page back;
  - any other post gets the owner's page back, with a notice.

  **The editors need script**, as the inbox and the dashboard already do: the
  Edit button is an `hx-get`, and a `<dialog>` only opens when script calls
  `showModal()`. The create dialogs and the member-side dialog use the same
  `dialog.js` opener every existing modal uses.

- **SD-10 — A roadmap may include milestones that belong somewhere else.**
  Every roadmap belongs to the project or to one initiative. The design settled
  that a milestone can *contain* work from anywhere (D-12), but said nothing
  about which milestones a roadmap can list. This build allows any: the
  Authentication roadmap can show the project-wide "Launch" milestone, so a
  team can show a release it depends on without copying it. The placer lists
  the owner's own milestones first ("Planned in …") and the rest after
  ("Planned elsewhere") (FR-4.2). The cost is that a roadmap can show a
  milestone its owner doesn't control, which someone else might change or mark
  as shipped; and when delete arrives, deleting a milestone must account for
  roadmaps elsewhere that list it. The alternative, allowing only the owner's
  own milestones, would push people to duplicate milestones, and the copies
  would drift. Sam is asked to confirm it (§5).

- **SD-11 — Locking is "mark as shipped", and it can be undone.** Decided by
  Sam on 2026-09-28, after asking what locking is for.

  *Why it exists.* A milestone's contents are live: add an initiative to it,
  and every feature under that initiative counts, including ones created later.
  That is useful while working towards a release, and wrong afterwards. If v1
  shipped with 10 of 10 features done and someone later adds four features
  under one of its initiatives, v1 would read "10 of 14" and the record of what
  shipped would be wrong. Locking fixes that: it records exactly which features
  the milestone covered at that moment, and reports against that fixed list
  from then on.

  *Why it no longer needs to be permanent.* The vision (§4) made locking
  permanent so that a locked milestone would be "the true record" of what
  shipped. But milestones are for people, and nothing in the agent workflow
  reads them (§0). The audit trail already records who marked what as shipped,
  and when. Allowing it to be reopened, and logging that too, keeps the history
  honest while letting a mistake be fixed in one click.

  *What changes.* People see **Mark as shipped** and **Reopen**; the stored
  state and the audit kinds keep their old names (`locked`,
  `milestone.locked`, `milestone.unlocked`), so nothing in the data has to be
  migrated. The confirmation step that warned "this is permanent" is gone,
  because the act can now be undone. This departs from vision §4, which should
  get a dated note saying so.

## 3. Requirements

Each requirement names the service method it renders. "Owner page" means the
project page (`/ui/project`) or an initiative page (`/ui/i/<path>`).

### FR-1: Owner-scoped creation and the store additions

- **FR-1.1** `CreateMilestone(ctx, tx, ownerType, ownerID, name, description,
  targetDate, actor)` and `CreateRoadmap(ctx, tx, ownerType, ownerID, name,
  actor)` take an owner: `project` with a nil id, or `initiative` with an
  initiative's id. Any other combination, or a blank name, is refused before
  anything is written. The web UI and MCP check the name first and say what is
  wrong in a sentence. The audit row records the owner. A roadmap's
  `roadmap.created` row is filed against its owner, so it shows in that
  owner's activity. An archived initiative may still own a new plan; nothing
  here refuses it.
  *AC:* creating each kind with each owner kind stores the owner columns as
  given and writes one audit row naming the owner; a bad owner or a blank name
  writes nothing (`TestOwnerScopedCreation`).
- **FR-1.2** The JSON API (`POST /api/milestones`, `POST /api/roadmaps`) passes
  the project, so the CLI's `milestone create` and `roadmap create` behave
  exactly as before (SD-2).
  *AC:* the API-level phase 3 tests pass unchanged. (The store's own tests were
  updated for the new signature.)
- **FR-1.3** `PlaceRoadmapEntry(ctx, tx, roadmapID, milestoneID, place, actor)`
  puts the milestone into the roadmap's current order, inserting it or moving
  it if it is already there. A `place` from 1 to n puts it there; any other
  value, including 0, puts it at the end. It then renumbers every entry 1…n
  and writes one `roadmap.entry_set` audit row with the final place.
  *AC:* placing first, moving down, placing with 0 and placing past the end
  all give the expected order with dense positions, including on a roadmap
  whose old positions tie (`TestPlaceAndRemoveRoadmapEntries`).
- **FR-1.4** `RemoveRoadmapEntry(ctx, tx, roadmapID, milestoneID, actor)`
  deletes the entry, renumbers the rest, and writes a `roadmap.entry_removed`
  audit row. Removing a milestone that isn't on the roadmap is a not-found
  error.
  *AC:* after removal the milestone is gone from `RoadmapEntries`, the rest keep
  their relative order, the audit row exists, and a second removal is
  not-found.
- **FR-1.5** `AddMember` refuses to put a milestone inside itself, or inside a
  milestone it already contains (directly or through nesting). Nothing is
  written.
  *AC:* adding A to A, A to B when B is in A, and A to C when C is in B in A all
  fail; adding an unrelated milestone still works
  (`TestMilestoneCannotContainItself`).

### FR-2: Creating from the owner's page

- **FR-2.1** The project page and every initiative page always show the
  "own plan" section, even when it is empty. Its empty state says what
  milestones and roadmaps are for, and the section offers **New milestone** and
  **New roadmap**. The same two actions are in the page's "more" menu as **New
  milestone here** and **New roadmap here**.
  *AC:* an initiative with no plan renders "Nothing is planned here yet" and
  both create forms (`TestPlanEditorsRender`, `TestUIPlanEditing`).
- **FR-2.2** **New milestone** opens a dialog asking for a name, an optional
  target date and an optional description. It posts to
  `POST /ui/milestone/new` with the owner carried in hidden fields
  (`owner_type`, `id`), calls `CreateMilestone` with that owner, and
  re-renders the owner's page with a notice naming the new milestone.
- **FR-2.3** **New roadmap** is the same with a name only, posting to
  `POST /ui/roadmap/new` and calling `CreateRoadmap`.
- **FR-2.4** A rejection (a blank name, a malformed date) re-renders the owner's
  page with the reason in a full sentence and writes nothing.
  *AC for FR-2.2 to FR-2.4:* creating from an initiative page stores that
  initiative as the owner and shows the new item in its plan section and not
  in the project's; each is audited to the UI actor; a blank name and a bad
  date are refused in sentences (`TestUIPlanEditing`).

### FR-3: Composing a milestone from both ends

- **FR-3.1 From the member's side.** Feature and initiative pages have **Add to
  a milestone…** in the "more" menu. The milestone page has it as a button in
  its header, beside **Edit this milestone**; it has no "more" menu, and gains
  a notice and error banner so it can answer an action. The action opens a
  dialog with two parts:
  1. *The milestones this is directly part of*, each linked, with a **Take it
     out…** step on each open one (SD-6). It asks for an optional reason,
     which is recorded on the audit trail, as the descope path always has
     (SPEC-003 FR-5.3).
  2. *Add it to a milestone*: a select of the open milestones it could join,
     grouped by where each is planned ("Planned in Project", "Planned in
     Authentication"). It leaves out shipped milestones, ones it is already
     directly in, and, on a milestone page, the milestone itself and any
     milestone it contains (FR-1.5).

  Both post to `POST /ui/milestone/member/add` or `/remove` with
  `milestone_id`, `member_type`, `member_id` and `from=member`, and the
  response is the member's page with a notice (SD-9). `renderEntity` gains a
  milestone case for this.
- **FR-3.2 From the milestone's side.** The milestone editor (FR-6) lists the
  current members as a checklist (D-10), each with the same **Take it out…**
  step and optional reason, and below it an **Add to this milestone** picker.
  With the search box empty, the picker lists candidates from the owner's
  subtree: for an initiative-owned milestone, that initiative, everything
  under it, and the milestones planned anywhere in that subtree; for a
  project-owned milestone, the whole project. Typing in the search box widens
  to the whole project (§5.1a). Each candidate shows its kind, name and path,
  and has an **Add** button.
- **FR-3.3 The candidate read.** `MemberCandidates(ctx, q, milestoneID,
  scopeRoot, query, limit)` returns initiatives (not archived), features (not
  abandoned, and not under an archived initiative) and milestones, each with
  kind, id, name and path. It leaves out what is already a direct member, the
  milestone itself, and any milestone that already contains it (FR-1.5). With
  an empty query it is limited to `scopeRoot`'s subtree (nil means the whole
  project); with a query it matches the project-wide set by case-insensitive
  substring over name and path, with `%` and `_` in the query matched
  literally (SD-7). The picker shows at most 50 and says so when there are
  more.
  *AC:* the default excludes work outside the subtree, current members, the
  milestone itself and a milestone that contains it; a search by name or by
  path reaches outside the subtree; `%_` matches literally; a limit of 2 over
  the project reports that there are more (`TestMemberCandidates`).
- **FR-3.4 Room for a fourth member type.** The candidate read is a union of
  one branch per member type, and the picker renders any kind by its icon and
  label. Adding checklists in M5 means one more branch, one more icon and
  widening the `milestone_members` CHECK, with no change to the picker's shape.
  *AC:* checked by review, not by a test: the union and the template are
  generic over `Kind`.
- **FR-3.5** Adding or removing on a shipped milestone is refused by
  `AddMember`/`RemoveMember` as today. The editor doesn't offer either on a
  shipped milestone; it says why, and offers to reopen it (FR-5.4).
  *AC for FR-3:* a feature added from its own page appears in the milestone and
  in the feature's rail; an initiative from another tree added in the editor
  appears in the checklist; removal from either end removes it and records the
  reason; a milestone can't be added to one it contains; a shipped milestone's
  editor offers no add or remove (`TestUIPlanEditing`,
  `TestPlanEditorsRender`).

### FR-4: Ordering a roadmap

- **FR-4.1** The roadmap editor (FR-6) shows the roadmap as a numbered list
  (D-10). Each entry has **Up**, **Down** and **Take off**; the first has no Up
  and the last no Down.
- **FR-4.2** Below the list, **Place a milestone** offers a select of the
  milestones not yet on this roadmap, grouped as "Planned in *owner*" and
  "Planned elsewhere" (SD-10), and a place: "At the end" (the default),
  "First", or "After *name*" for each entry but the last. It posts to
  `POST /ui/roadmap/entry/place` with `roadmap_id`, `milestone_id` and
  `place`, calling `PlaceRoadmapEntry`. Up and Down post the same route with
  the entry's new place.
- **FR-4.3** **Take off** posts to `POST /ui/roadmap/entry/remove` and calls
  `RemoveRoadmapEntry`. The milestone itself is untouched.
  *AC for FR-4:* placing two milestones and moving the second to first gives
  that order in the store and on the roadmap page; taking one off and placing
  it again works; each act is audited; the editor offers no Up on the first
  entry (`TestUIPlanEditing`, `TestPlanEditorsRender`).

### FR-5: Marking a milestone as shipped, and reopening it

In the code and the audit trail these acts keep their old names, lock and
unlock (SD-11). Everything a person reads says "mark as shipped", "shipped" and
"reopen"; the state label for `locked` is **Shipped**.

- **FR-5.1** The milestone editor has a **Mark as shipped** section. It first
  evaluates G4 over the live membership, exactly as `LockMilestone` will. When
  G4 would refuse, the button is disabled and G4's reason is shown beside it
  (DESIGN-008 §5.2). **G4's refusal reasons are full sentences** in
  `lifecycle.G4`, counting what the gate counts — resolved features — so the
  web UI, the chat agent, the API and the audit trail all say the same thing.
  There are three:
  - nothing resolves to a feature: "This milestone can't be marked as shipped
    yet, because nothing in it comes down to a feature. Add the work it is
    meant to deliver first."
  - one feature, not done: "…because its one feature isn't done. Marking it as
    shipped records what actually went out, so at least one feature has to be
    finished first."
  - several, none done: "…because none of its 3 features is done. Marking it
    as shipped records what actually went out, so at least one has to be
    finished first."
- **FR-5.2** When G4 would pass, the section explains in a sentence what the
  act does — it records the N features the milestone covers (M of them done)
  and stops its contents changing, so work added later under its initiatives
  doesn't rewrite the record — and says it can be reopened. Below that is a
  plain **Mark as shipped** button. There is no separate confirmation step,
  because the act can be undone (SD-11).
- **FR-5.3** `POST /ui/milestone/lock` calls `LockMilestone` as the UI actor.
  On success the editor says the milestone is marked as shipped and what its
  record holds. If G4 refuses at the moment of posting (the membership changed
  in between), its reason is shown inline and nothing changes. Any other
  failure, such as the milestone already being shipped, is reported as itself,
  not as a G4 refusal. There is no way round G4 (L-6) and no checkpoint
  (SPEC-006 R6-1).
- **FR-5.4 Reopening.** A shipped milestone's editor says it is shipped and
  offers **Reopen this milestone…**: a short disclosure that explains that
  reopening throws the shipped record away and makes the contents live again,
  with an optional reason and a **Reopen it** button. It posts to
  `POST /ui/milestone/unlock`, which calls the new
  `UnlockMilestone(ctx, tx, milestoneID, reason, actor)`. That deletes the
  snapshot, sets the milestone back to open, and writes a `milestone.unlocked`
  audit row carrying the reason, when it had been shipped, and the ids of the
  features the snapshot held — so the history still shows what it shipped
  with. Reopening an open milestone is refused in a sentence. A reopened
  milestone can be changed and marked as shipped again.
  *AC for FR-5:* every G4 refusal is a sentence about features (`TestG4`); a
  milestone G4 would refuse shows the disabled button and reason and offers no
  form; one that G4 allows is marked as shipped with a `milestone.locked` audit
  row by the UI actor; its editor then offers only Reopen; reopening removes
  the snapshot, keeps the old snapshot and the reason in the audit row, and
  lets the milestone change and ship again; reopening an open one is refused
  (`TestUnlockMilestone`, `TestUIPlanEditing`, `TestPlanEditorsRender`).

### FR-6: The editors

- **FR-6.1 Routes.** One fragment route per editable object:
  `GET /ui/m/{id}/edit` returns the milestone editor and `GET /ui/r/{id}/edit`
  the roadmap editor, each a `<dialog class="modal modal--wide"
  data-autoshow data-refresh-on-close>` wrapping an inner `<div
  id="…-editor">`. `GET /ui/m/{id}/candidates?q=` returns the picker's list on
  its own, for the search box.
- **FR-6.2 Opening.** Owner pages show an **Edit** button beside each
  milestone and roadmap in the plan section; milestone and roadmap pages show
  one in their header (SD-6). The button uses `hx-get` into a `#modal-slot`
  that every page now carries. **`dialog.js` gains a listener** that calls
  `showModal()` on any `dialog[data-autoshow]` HTMX inserts. Escape, focus
  trapping and focus return stay the browser's job.
- **FR-6.3 Acting inside it.** Every form in an editor posts with HTMX and
  targets the editor's inner `<div>`, so the dialog stays open and shows the
  result — the updated list, a notice, or an inline error — and a person can
  make several changes in one sitting.
- **FR-6.4 Closing.** The editor has a **Done** button. **`dialog.js` gains a
  second listener**: when an editor closes by any means (Done, Escape, the
  backdrop) and it recorded a change (it carries `data-changed`), the page
  reloads so the plan underneath is current. With no change, closing removes
  the dialog and nothing else.
  *AC:* checked in a browser, in the Playwright walkthrough (DoD 2).
- **FR-6.5 Responses.** As SD-9: an editor post gets the editor, a
  member-side post the member's page, and any other post the owner's page.
  *AC for FR-6:* the fragment routes return the dialog and not a whole page; an
  editor post returns the editor fragment marked changed; a boosted post
  returns a full page; a member-side post returns the member's page
  (`TestUIPlanEditing`).

### FR-7: The chat agent's tools

All tools name milestones and roadmaps by id, or by exact name where that name
is unique (the existing `milestoneByRef`), and initiatives and features by
path, as the SPEC-008 tools do. Each write runs its store method in one
transaction as the MCP actor, is audited there (DEC-004, O-3), and signals the
SSE hub as SPEC-008 FR-5.1 does. (Entity pages don't yet refresh from that
signal; see §6.) Store refusals are turned into sentences in the handler, as
SPEC-008's `attachError` does, so the agent can relay them.

- **FR-7.1 `create_milestone`** — `name`, optional `description`,
  `target_date` (YYYY-MM-DD) and `owner_path` (an initiative's path). Leaving
  `owner_path` out means the project, which is unambiguous, as leaving out
  `create_initiative`'s `parent_path` means the top level. Returns id, name,
  owner, progress and page address.
- **FR-7.2 `create_roadmap`** — `name`, optional `owner_path`. Returns id,
  name, owner, its (empty) milestones and page address.
- **FR-7.3 `add_milestone_member`** — `milestone`, `member_type` (`feature`,
  `initiative` or `milestone`) and `member` (a path, or a milestone's id or
  name). The explicit type means an initiative and a feature with similar
  paths are never confused.
- **FR-7.4 `remove_milestone_member`** — the same, plus an optional `reason`
  recorded on the audit trail.
- **FR-7.5 `place_roadmap_entry`** — `roadmap`, `milestone` and an optional
  `position` counting from 1 (left out, it means the end). It places or moves,
  so it is also the reorder tool. Returns the roadmap in its new order.
- **FR-7.6 `remove_roadmap_entry`** — `roadmap` and `milestone`.
- **FR-7.7 `list_milestones` and `list_roadmaps`** — optional `owner_type`
  (`project` or `initiative`) and `owner_path`, the pair `list_documents`
  already uses. With them, only that plan's; without them, every one, each
  with its owner. A chat agent needs to resolve "the beta milestone" wherever
  it is planned, much as `get_tree` returns the whole tree, and `GET
  /api/milestones` already lists them all. D-9's "no global list" is about the
  web UI's navigation, not the agent's lookups.
- **FR-7.8 `get_milestone` and `get_roadmap`** — one in detail. For a
  milestone: owner, state (`open` or `shipped`), target date, members (type,
  name, path or id, done), progress as `items_done`/`items_total` and
  `tokens_done`/`tokens_estimated`. An open milestone also has a `shipping`
  object with `could_mark_shipped_now`, `why_not` (G4's sentence) and `how`
  (that a person does it in the web UI); a shipped one has `shipped_at`. For a
  roadmap: owner and milestones in order, each with its position, state and
  progress.
- **FR-7.9 The boundary.** There is no tool to mark a milestone as shipped or
  to reopen one (SD-4). The advertised set is the SPEC-008 nine plus these
  ten, and `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` names every one
  and lists `lock_milestone` and `unlock_milestone` among the names that must
  be unknown. The `initialize` instructions mention milestones and roadmaps,
  and say that marking a milestone as shipped is a person's act.
  *AC for FR-7:* over `POST /mcp`, the agent builds a two-milestone roadmap for
  an initiative, fills it by id and by name, refuses a loop, reorders the
  roadmap and reads it back with `could_mark_shipped_now` false and G4's
  sentence; bad owners, bad dates, bad member types, removing what isn't there
  and changing a shipped milestone each fail with a sentence; a shipped
  milestone reads as `shipped` with `shipped_at`; each write leaves an audit
  row whose actor is the MCP actor; `lock_milestone` and `unlock_milestone`
  are method-not-found (`TestMCPPlanTools`,
  `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`).

## 4. Non-functional requirements

- **NFR-1 — No new stack.** HTML, HTMX and the existing `dialog.js`, which gains
  two small listeners (FR-6.2, FR-6.4). No new dependency, no bundler. `go vet
  ./...` and `go test -race ./...` stay clean.
- **NFR-2 — One service layer.** UI handlers and MCP tools call the store
  methods directly in a transaction. Neither issues HTTP to `/api/*`.
- **NFR-3 — No way round G4.** G4 keeps its shape: a blocking inline reason,
  no checkpoint, no force (L-6). Reopening (FR-5.4) is the one new act, and it
  is logged with the old snapshot. The chat agent can edit plans but can't
  mark a milestone as shipped or reopen it (SD-4, SD-5).
- **NFR-4 — No typed entity path.** Every entity in a form is carried by id in
  a hidden field or chosen from a list (SPEC-007 SD-6). Fields are named
  `milestone_id`, `roadmap_id`, `member_id`, `member_type` and `place`; the
  picker's search box is a search, named `q`, and what it finds is then chosen,
  not typed. `TestUIBrowseAndDrive`'s scan doesn't reach the new pages, so the
  new tests scan them: the owner pages, a feature page, a milestone page, a
  roadmap page, both editors and the candidate list, in a state where the
  picker and placer have options, against a banned list that adds `member`
  and `owner_path` to the original.
- **NFR-5 — Two lists, not tables** (D-10). A milestone is edited as a
  checklist and a roadmap as a numbered list. No table markup; the render test
  checks it.
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
   add members from both ends, reorder, mark one as shipped, and reopen it.
   This also checks FR-6.4. Screenshots saved.
3. **The same over MCP**, by JSON-RPC calls to `POST /mcp`, except marking as
   shipped, which is shown to be unknown.
4. Both recorded in `docs/walkthrough-spec-010.md`.
5. `go vet ./...` and `go test -race -count=1 ./...` clean.
6. A handoff note records what was built, what was decided, and what needs
   Sam.

**Decided by Sam, 2026-09-28:** locking becomes "mark as shipped", and it can
be undone (SD-11).

**Still waiting on Sam** — each with a recommendation:

1. **The chat agent can't mark a milestone as shipped or reopen it, for now**
   (SD-4). Now that shipping can be undone, there is no safety reason to keep
   it out of chat; only DEC-004's wording ("may not touch gates") stands in the
   way. *Recommendation:* leave it out of this build, and allow it later with a
   one-line amendment to DEC-004 when the chat agent needs it.
2. **Two places where the build differs from DESIGN-008 §5.1a** (SD-6): a
   member can be taken out of a milestone from its own page, and milestone and
   roadmap pages have their own Edit button. *Recommendation:* accept both, and
   add a dated note to the design.
3. **The chat agent edits milestones and roadmaps as ordinary planning**
   (SD-5), not as relayed human decisions that must quote the person's words.
   *Recommendation:* confirm it, and add a line to DEC-004 naming them.
4. **A roadmap can include milestones that belong somewhere else** (SD-10).
   *Recommendation:* allow it, as built.

## 6. Open questions carried forward

- **Renaming, describing and deleting milestones and roadmaps.** Out of scope
  (§2), but a typo in a milestone's name is now fixable only in the database,
  and a stray milestone created by the chat agent can't be removed. This is
  the most likely first follow-up, and it is worth doing soon now that two
  surfaces can create them. `UpdateEntityFields` could take a milestone, and a
  delete could be limited to milestones that are empty and on no roadmap.
- **Whether owner pages should refresh live** when the chat agent edits their
  plan. The MCP tools signal the SSE hub, but entity pages only listen on the
  document page (`#doc-live`). That is a wider SPEC-007 question.
- **A refused "mark as shipped" leaves no audit row.** `LockMilestone` writes
  `gate.evaluated` and then returns an error, so the transaction rolls the row
  back. That predates this spec. Keeping the evaluation would mean auditing it
  in a separate transaction; now that shipping is a reversible record, it
  matters little.
- **`POST /api/milestones/lock` reports every failure as a G4 409**, including
  "already locked". The UI handler doesn't copy that; the API is left alone
  because the CLI is being retired.
- **Vision §4 still says a locked milestone can never change.** SD-11 departs
  from that on Sam's decision; the vision should get a dated note.
- **The tool-set test will also change in M3**, whose relay tools edit the
  same list. Whichever lands second merges the two.

## 7. Changes after review

How each [REVIEW-010](../reviews/REVIEW-010-milestones-and-roadmaps-editing.md)
finding was dealt with.

| Finding | What changed |
|---|---|
| R10-1 (material) | The Goal and NFR-3 now say the chat agent gains no gate or lifecycle act and nothing that freezes or destroys planning state. SD-4 says plainly that creation can't be undone either, and why that harm is smaller. §6 records the rename and delete follow-up and suggests doing it soon. |
| R10-2 (material) | SD-4 rests on DEC-004's "may not touch gates" and D-13 first, with irreversibility second, and says locking is a third kind of act that Amendment 1's UI-only list doesn't name. A lock tool now needs its own decision. |
| R10-3 (material) | SD-6 records removal from the member's side and Edit on the milestone and roadmap pages as deviations from DESIGN-008 for Sam to accept, with a dated note to the design once he does. |
| R10-4 (material) | G4's refusal reasons are rewritten as sentences in `lifecycle.G4` itself, counting features; FR-5.1 lists all three, and `TestG4` checks them. The FR-5.1 example no longer says "deliverables". |
| R10-5 (material) | SD-9 and FR-6.5 decide the response by `HX-Boosted` and `from=member`, not `HX-Request`, and drop the claim that the editor works without script. `TestUIPlanEditing` posts both ways and checks a member-side post. |
| R10-6 (material) | NFR-4 no longer leans on `TestUIBrowseAndDrive`. The new tests scan the milestone and roadmap pages, both editors and the candidate list with a wider banned list. |
| R10-7 | SD-5 now gives the real argument and asks Sam to confirm the widening (DoD 6). |
| R10-8 | The list tools take `owner_type` and `owner_path`, like `list_documents`. The create tools keep `owner_path` alone, where leaving it out is unambiguous, as `create_initiative` does. |
| R10-9 | FR-3.1 says the milestone page gains header buttons and a banner, and `renderEntity` a milestone case. SD-8 names the new per-page read. |
| R10-10 | FR-3.3 excludes milestones that contain this one; FR-3.2 and the code agree that the default covers milestones planned anywhere in the subtree. |
| R10-11 | FR-1.3's rule is now "1 to n puts it there; anything else puts it at the end". |
| R10-12 | `document_sections.fts`; four store changes, named in §0 and SD-3; FR-1.2's AC limited to the API-level tests; the `dialog.js` listeners described as additions, and its header comment updated. |
| R10-13 | SD-10 records that roadmap entries may cross owners, and why. Sam is asked to confirm it. |
| R10-14 | ACs added or named for FR-2.1, FR-3.3, FR-3.4 (by review), FR-6.4 (in the browser walkthrough), FR-7's errors and FR-7.8's lock fields. |
| R10-15 | FR-7 now says it signals the hub as SPEC-008 does, and that entity pages don't refresh from it yet. |
| R10-16 | The status line now says the findings were dealt with, and points here. |
| R10-17 | SD-8 names inline dialogs as a choice and offers the fragment alternative. |
| R10-18 | Entry criterion 5 is recorded as unscheduled, here and in the handoff. |
| §4 notes | The blank-name checks in the UI and MCP give sentences; an archived initiative may own a plan (FR-1.1); both ends of removal ask for the same optional reason; the refused-lock audit, the API's 409 and the M3 test collision are in §6. |

## 8. Changes after Sam's decision on locking (2026-09-28)

Sam asked what locking is for, given that milestones are a reporting tool and
nothing in the agent workflow uses them. The answer (SD-11) is that it stops a
shipped release's record drifting as new work lands under its initiatives —
which is worth keeping, but not worth making permanent. Sam chose to keep it
and make it reversible. What changed:

| Where | What changed |
|---|---|
| SD-11 (new) | Records the decision and the reasoning. |
| §0 | Says plainly that milestones are for people and nothing in the agent workflow reads them. |
| FR-5 | "Lock" becomes **Mark as shipped**, with no permanent-warning confirmation. New FR-5.4: **Reopen**, backed by `UnlockMilestone`, which keeps the old snapshot and the reason in the audit row. |
| FR-5.1 | G4's sentences say "can't be marked as shipped yet". |
| SD-3 | Five store changes, not four. |
| SD-4 | Rewritten. The safety argument for keeping the act out of chat has gone; only DEC-004's wording remains, and the recommendation is now to allow it later with a one-line amendment. This replaces the reasoning recorded against R10-1 and R10-2 in §7. |
| FR-7.8, FR-7.9 | The chat agent sees `shipped` and a `shipping` object; `unlock_milestone` joins `lock_milestone` as a tool that must not exist. |
| SD-5, SD-6, SD-10, DoD 6 | Rewritten in plainer language, with what each choice means and a recommendation. |
