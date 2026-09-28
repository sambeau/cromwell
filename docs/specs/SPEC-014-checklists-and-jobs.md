# SPEC-014: Checklists and jobs

**Status:** **Draft — for Sam's approval.** Authored by Claude; the approval
decision is Sam's, as the author can't be the approval gate. Sam approves the
spec and the build together. The choices he must confirm are listed in the
definition of done (§5).
**Date:** 2026-09-28
**Roadmap milestone:** M5 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) §4 (Job,
Checklist, Deliverable), §5c (the chat agent may create checklists and jobs,
and relay a ticked job), §6 (milestones, *Mark as shipped*, checklists and
jobs, ownership and editing) and §17a item 4;
[DESIGN-008](../design/DESIGN-008-the-workflow-surface.md) D-9 to D-12
**Authority:**
[DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1,
decision 8 (a ticked job is a relayed human act, quoted and audited `via:
mcp`); [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) Amendment 1
(milestones and roadmaps are planning; DESIGN-010 §5c and §17a item 4 put
checklists and jobs with them); [DEC-005](../decisions/DEC-005-the-orchestration-boundary.md)
(widening the chat facet is always deliberate)
**Builds on:** [SPEC-010](SPEC-010-milestones-and-roadmaps-editing.md), whose
picker, editors, member-side dialog and MCP tools this spec extends, and the
[M4 handoff](../notes/handoff-M4-2026-09-28.md) follow-up 2
**Roadmap decision 8:** a job carries a title, an optional note, and who
ticked it and when. Owner and due date come later.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, empty state, notice, gate reason and tool description
a person will read is written in full sentences (DESIGN-008 D-6).

## 0. Framing

Some of the work in a release can only be done by a person: get an API key,
sign a contract, choose an icon. Subutai has no way to record it, so a
milestone can look finished while the chores that stand between it and a
release are still open.

DESIGN-010 names the answer. A **job** is one of those chores, ticked by hand.
A **checklist** is a list of jobs, and it is done when every job is ticked. A
checklist can be a milestone deliverable, so it counts towards the milestone
like a feature does.

The schema already reserves the words: `checklist` and `job` are in the
`ref_type` enum from migration 0001, and SPEC-003 SD-2 deferred them. This
spec adds the two tables, lets a checklist into a milestone, and gives people
and the chat agent the tools to keep them.

**What checklists are for.** Like milestones, they are for people. Nothing in
the agent workflow reads them: no work starts, stops or waits because of one.
The only thing a checklist changes is what a milestone reports.

**D-10 applies literally.** A checklist is a list of checkboxes. It is shown as
one, and not dressed up.

## 1. Goal

**One claim, which the definition of done checks directly:**

> A milestone that contains a checklist doesn't show as complete while one of
> its jobs is unticked, and does once every job is ticked — whether the ticks
> come from the checklist page or are relayed from chat with the person's
> words.

Two supporting claims:

> Every change to a checklist or a job, and every tick and untick, from either
> surface, is on the audit trail under the actor who made it. A relayed tick
> carries the person's quoted words.

> The chat agent can build and keep checklists as planning, but the only way it
> can tick or untick a job is by relaying a person's words.

## 2. Scope

### In scope

1. **The data** (FR-1): migration `0009` adds `checklists` and `jobs`, and
   admits `checklist` as a milestone member and a snapshot leaf.
2. **Completion** (FR-2): when a checklist is done, how it counts in a
   milestone's progress, in gate G4 and in marking a milestone as shipped.
3. **The web UI** (FR-3 to FR-5): creating a checklist from the owner's page,
   the checklist page with its checkboxes, the checklist editor, and adding a
   checklist to a milestone from either end.
4. **The chat agent's tools** (FR-6): seven authoring tools and one relay tool,
   `relay_tick_job`.

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| Entity IDs such as `CL-002` | **M8**, which mints IDs for everything. Checklists and jobs are named by uuid, or by exact name or title, until then. |
| Renaming, re-describing or deleting a checklist | Not specified here, as for milestones and roadmaps (SPEC-010 §6). The handoff records it with the milestone follow-up, so both are done together. |
| A job's owner or due date | Later, as roadmap decision 8 says. |
| Moving a job to another checklist, or a checklist to another owner | Not specified. Remove and add again. |
| Owner and checklist pages refreshing live when the chat agent changes them | The same open question as SPEC-010 §6: entity pages don't listen to the SSE hub. |
| Bugs, spikes, documents | Other milestones. |

### Scope decisions

- **SD-1 — A checklist counts as one item, not one per job.** A milestone's
  "X of Y items done" counts its resolved leaves. Today every leaf is a
  feature; now a checklist is a leaf too, done or not done as a whole.
  DESIGN-010 §6 counts *deliverables* ("3 of 4 done"), and a checklist is one
  deliverable, however many jobs it has. Counting each job would let a
  ten-job checklist outweigh ten features. The roadmap's "progress and the
  shipping gate count jobs" is read as "jobs count, through their checklist".
  The checklist's own page shows its jobs ticked ("3 of 5 jobs ticked").

- **SD-2 — An initiative in a milestone doesn't bring in the checklists it
  owns.** An initiative member resolves to every feature under it, live. A
  checklist counts only when it is added to the milestone itself, directly or
  through a nested milestone. Ownership says whose page a checklist is planned
  on, and never constrains membership (D-12). Milestones are the same: a
  milestone owned by an initiative isn't part of a milestone that contains
  that initiative. The alternative, pulling in every checklist an initiative
  owns, would let a checklist made for some other purpose silently hold a
  release open. **For Sam to confirm.**

- **SD-3 — A done checklist can satisfy G4 on its own.** G4 lets a milestone be
  marked as shipped when at least one of its items is done. DESIGN-010 §6 says
  "at least one deliverable done", and §4 names a checklist as a deliverable,
  so a done checklist counts. A milestone made only of human chores ("Launch
  paperwork": the contract, the domain, the store listing) can then be marked
  as shipped when its checklist is done. G4 is a record-keeping check, not a
  development gate (DEC-004 Amendment 1), and it already lets a milestone ship
  with one of five features done; this changes what it counts, not how strict
  it is.

  G4's refusal sentences change from counting features to counting items, the
  same word the progress line uses (FR-2.4). DEC-004 Amendment 1 describes G4
  as stopping shipping "when none of its features is done", because features
  were the only leaves when it was written. It is an accepted decision, so
  this spec doesn't edit it; the handoff records the difference and suggests a
  dated note. **For Sam to confirm.** The alternative is to make G4 count
  features only, so a milestone of checklists alone could never be marked as
  shipped.

- **SD-4 — Shipping records an unticked checklist as not shipped, as it does a
  feature.** Marking a milestone as shipped snapshots its resolved leaves,
  checklists included. The `milestone.locked` audit row gains `not_shipped`:
  each leaf, feature or checklist, that wasn't done at that moment, by type,
  id and name. After shipping, progress is measured against the snapshot and
  each leaf's current state, as SPEC-003 FR-5.2 already does for features. So
  a job ticked later moves a shipped milestone's progress on, exactly as a
  feature finished later does, and the audit row still says it wasn't shipped.
  A checklist in a shipped milestone isn't frozen: jobs can still be ticked,
  added and removed.

- **SD-5 — Ticking happens on the checklist page; editing happens in a modal.**
  DESIGN-010 §6 says jobs "are ticked on the checklist page", and that
  milestones, roadmaps and checklists are each "edited in a modal from its
  owner's page". The brief asked for a page where you can tick, add, rename,
  reorder and remove. Both are met:
  - `/ui/c/{id}` is the checklist page: its jobs as plain checkboxes, ticked
    and unticked there, with an optional note (FR-4).
  - Adding, renaming, reordering and removing jobs happen in the **checklist
    editor**, a modal loaded by HTMX like the milestone editor (FR-5). The
    owner's page opens it with **Edit** beside each checklist. The checklist
    page also has an **Edit this checklist** button in its header, following
    SPEC-010 SD-6, which Sam accepted for milestone and roadmap pages.

- **SD-6 — A job has one note.** The note says something about the job: where
  the key is kept, why the contract is late. It can be set when the job is
  added, changed in the editor, or given when ticking or unticking. A note
  given with a tick or untick replaces the job's note; leaving it blank keeps
  the note as it was. Every note change is on the audit trail, so nothing is
  lost. Keeping one note, rather than a note per tick, keeps a job a single
  line on the page, as D-10 asks.

- **SD-7 — Only `relay_tick_job` ticks, and it needs the person's words.**
  DEC-006 Amendment 1 lists "a ticked job" among the acts the chat agent may
  relay, quoting the person, and DESIGN-010 §5c repeats it. The authoring
  tools can create a checklist and add, rename, move and remove jobs, as
  planning under DEC-004 and §17a item 4, without quoted words. None of them
  can tick.

  One consequence is worth stating. Removing a job is planning, so the chat
  agent could make a checklist done by removing its unticked jobs, much as
  `remove_milestone_member` can make a milestone complete by taking out its
  unfinished features. Both are audited under the chat agent's name with the
  reason given, and both can be undone by adding the work back. The
  alternative, making removal of an unticked job a relay act too, would add a
  quote to a planning edit and put a relay on a list DEC-006 says needs a
  decision to extend. **For Sam to confirm.**

- **SD-8 — Who ticked a job is the actor, how, and the words.** A job stores
  `ticked_by` (the UI actor or the MCP actor), `ticked_at`, `ticked_via`
  (`ui` or `mcp`) and, for a relay, `ticked_quote`. The checklist page shows
  all of them: "Ticked by chat-agent, relaying: “I've got the API key”". The
  audit row carries the same. Nobody's identity is proved, as DEC-006
  Amendment 1 says; the quote makes a mistaken relay easy to spot.

- **SD-9 — An empty checklist is not done.** A checklist with no jobs has
  nothing ticked, so it counts as an unfinished item. That stops a checklist
  created as a placeholder counting as finished work.

## 3. Requirements

"Owner page" means the project page (`/ui/project`) or an initiative page
(`/ui/i/<path>`), as in SPEC-010.

### FR-1: The data

- **FR-1.1 Migration `0009_checklists_and_jobs.sql`**, forward-only and
  additive (DESIGN-001 §11):
  - `checklists`: `id`, `name` (not blank), `description` (default empty),
    `owner_type` and `owner_id` with the same two CHECKs milestones and
    roadmaps carry (the project with a null id, or an initiative), and
    `created_at`.
  - `jobs`: `id`, `checklist_id` (references `checklists`), `title` (not
    blank), `note` (default empty), `position` (1…n within the checklist),
    `ticked_by`, `ticked_at`, `ticked_via` and `ticked_quote` (all null while
    unticked), and `created_at`. CHECKs: `ticked_by` and `ticked_at` are null
    together, and `ticked_via` is null, `ui` or `mcp`. Index on
    `(checklist_id, position)`.
  - `milestone_members_member_type_check` admits `checklist`, and
    `milestone_snapshots_leaf_type_check` admits `checklist`.
  *AC:* the migration applies on top of 0008 and on a fresh database, and the
  CHECKs refuse a checklist with a bad owner (`TestMigrations`, `TestChecklistStore`).
- **FR-1.2 Checklists in the store** (`internal/store/checklists.go`):
  `CreateChecklist(ctx, tx, ownerType, ownerID, name, description, actor)`
  refuses a blank name or a bad owner before writing, and audits
  `checklist.created` against the checklist with the owner.
  `GetChecklist`, `ChecklistByName` (an ambiguous name is an error naming
  the count, like `MilestoneByName`), `ListChecklists` and
  `ChecklistsOwnedBy` read them.
- **FR-1.3 Jobs in the store.** Each writes in the caller's transaction and
  audits against the **checklist** (`ref_type` `checklist`), with `job_id` and
  the title in the payload, so the checklist's history is one query:
  - `AddJob(ctx, tx, checklistID, title, note, actor)` puts the job at the end
    (`job.added`).
  - `EditJob(ctx, tx, jobID, title, note, actor)` changes the title and note
    (`job.edited`, with the old and new title and note). A blank title is
    refused.
  - `PlaceJob(ctx, tx, jobID, place, actor)` moves the job to a 1-based place
    and renumbers the checklist 1…n; any place outside 1…n means the end, as
    `PlaceRoadmapEntry` does (`job.moved`).
  - `RemoveJob(ctx, tx, jobID, reason, actor)` deletes the job and renumbers
    the rest (`job.removed`, with the title, whether it was ticked, and the
    reason).
  - `TickJob(ctx, tx, jobID, tick, note, via, quote, actor)` ticks or unticks
    it (`job.ticked` or `job.unticked`, with `via`, `quote` when given, and
    the note when given). A non-blank note replaces the job's note (SD-6).
    Ticking a ticked job, or unticking an unticked one, is refused with
    `ErrJobAlreadyTicked` or `ErrJobNotTicked`, and nothing is written.
  - `Jobs(ctx, q, checklistID)` returns them in order, and
    `ChecklistStatus(ctx, q, checklistID)` returns the counts: jobs, ticked,
    and done (SD-9).
  *AC:* each write stores what it says and leaves one audit row with the right
  kind and actor; positions stay dense through add, move and remove; a blank
  name or title is refused; ticking twice and unticking an unticked job are
  refused; a tick with a quote stores `ticked_via` and `ticked_quote`, and an
  untick clears all four tick fields (`TestChecklistStore`).

### FR-2: Completion

- **FR-2.1 A checklist is done** when it has at least one job and every job
  is ticked (SD-9).
- **FR-2.2 Resolution.** A milestone's resolved leaves are now its features
  and its checklists. A checklist member resolves to itself; a nested
  milestone contributes its own checklists, recursively; an initiative
  contributes features only (SD-2). `ResolveMembers` keeps returning feature
  ids, because the cost and token roll-ups read it; a new `ResolveChecklists`
  returns the checklists.
- **FR-2.3 Progress.** `store.Progress` gains `Checklists` (the resolved or
  snapshotted checklist ids). `Total` and `Done` count features and checklists
  together, so every "X of Y items done" in the UI, the chat tools and the API
  includes them (SD-1). `Leaves` stays the feature ids, so the token bar and
  the cost roll-ups are unchanged: a checklist carries no tokens.
- **FR-2.4 G4 counts items.** `LockMilestone` passes the item counts to G4, so
  a done checklist can satisfy it alone (SD-3). The refusal sentences say
  "items" and name both kinds:
  - nothing resolves: "This milestone can't be marked as shipped yet, because
    nothing in it comes down to a feature or a checklist. Add the work it is
    meant to deliver first."
  - one item, not done: "…because the one item in it isn't done. Marking it as
    shipped records what actually went out, so at least one item has to be
    finished first."
  - several, none done: "…because none of its 3 items is done. Marking it as
    shipped records what actually went out, so at least one has to be finished
    first."
- **FR-2.5 Shipping.** `LockMilestone` snapshots the resolved checklists with
  `leaf_type = 'checklist'` beside the features, and its audit row gains
  `checklists` (the count) and `not_shipped` (SD-4). `SnapshotProgress` reads
  both kinds. `UnlockMilestone`'s audit row keeps the snapshot's checklists as
  `snapshot_checklists` beside the existing `snapshot`.
- **FR-2.6 Showing it.** In a milestone's checklist of members (its page and
  its editor), a checklist is a row with the checklist icon, its name linked
  to `/ui/c/{id}`, ticked when it is done, and "3 of 5 jobs ticked" beside it.
  `get_milestone` lists it with its id and `done`.
  *AC for FR-2:* a milestone with a done feature and an unticked checklist
  reads "1 of 2 items done", and "2 of 2" once every job is ticked; an empty
  checklist is not done; an initiative member doesn't bring in a checklist it
  owns, and a nested milestone does bring in its checklists; a milestone whose
  only member is a done checklist can be marked as shipped, and one whose only
  member is an unticked checklist is refused in the "one item" sentence;
  shipping snapshots the checklist and names it in `not_shipped` when unticked;
  ticking it afterwards moves the shipped milestone's progress on; reopening
  keeps the snapshot's checklists in the audit row; the token totals don't
  change when a checklist is added (`TestChecklistProgressAndShipping`,
  `TestG4`).

### FR-3: Creating a checklist from the owner's page

- **FR-3.1** The owner's plan section (SPEC-010 FR-2.1) lists the checklists
  planned there, below the milestones: each with the checklist icon, its name
  linked to its page, "3 of 5 jobs ticked", and an **Edit** button that opens
  the checklist editor (FR-5). The section's count includes them. Its lede and
  empty state mention checklists: "Milestones, roadmaps and checklists planned
  at this level."
- **FR-3.2** **New checklist** sits beside **New milestone** and **New
  roadmap**. It opens a dialog asking for a name and an optional description,
  and posts to `POST /ui/checklist/new` with the owner in hidden fields
  (`owner_type`, `id`). It calls `CreateChecklist` and re-renders the owner's
  page with a notice: "Checklist created: *name*. Choose Edit beside it to add
  its jobs." A blank name is refused in a sentence and nothing is written.
  *AC for FR-3:* creating from an initiative page stores that initiative as
  the owner and lists the checklist in its plan section and not the project's,
  audited to the UI actor; a blank name is refused (`TestUIChecklists`).

### FR-4: The checklist page

- **FR-4.1** `GET /ui/c/{id}` shows, in the page head: the checklist icon and
  name, "Planned in *owner*" linked, the milestones it is directly part of
  (linked), and "3 of 5 jobs ticked", or "Done: every job is ticked." It has
  two header buttons: **Edit this checklist** (`hx-get` into `#modal-slot`,
  FR-5) and **Add to a milestone…** (FR-4.4). The description, if any, is the
  lede. A bad or unknown id is the not-found page.
- **FR-4.2 The jobs, as checkboxes** (D-10). One row per job, in order. The
  checkbox is a button in a form posting to `POST /ui/job/tick` with `job_id`
  and `tick` (`true` or `false`); its accessible name is "Tick *title*" or
  "Untick *title*". The row shows the title, the note beneath it if there is
  one, and for a ticked job who ticked it and when, with the relayed words
  when it came from chat (SD-8). No table markup.
- **FR-4.3 With a note.** Each row has a small disclosure, **Tick with a
  note…** or **Untick with a note…**, holding a note field and the button.
  It posts to the same route with `note` (SD-6).
- **FR-4.4 Add to a milestone…** is SPEC-010's member-side dialog with
  `member_type` `checklist`: the milestones it is directly in, each with
  **Take it out…**, and a select of the open milestones it could join. Posts
  carry `from=member`, and `renderEntity` gains a checklist case, so the
  response is the checklist page with a notice.
- **FR-4.5 Responses.** A tick or untick re-renders the checklist page with a
  notice ("Ticked: *title*." or "Unticked: *title*.") or, when refused, the
  reason in a sentence ("That job is already ticked." or "That job isn't
  ticked."). An empty checklist says: "This checklist has no jobs yet, so it
  isn't done. Choose Edit this checklist to add them."
  *AC for FR-4:* ticking and unticking from the page change the job and write
  `job.ticked` and `job.unticked` rows by the UI actor with `via: ui`; a note
  given with a tick replaces the job's note and a blank one keeps it; ticking a
  ticked job is refused in a sentence; the page shows who ticked each job, and
  a relayed tick shows the quote; adding the checklist to a milestone from its
  page puts it in the milestone and taking it out records the reason; an
  unknown id is a 404 (`TestUIChecklists`, `TestChecklistPagesRender`).

### FR-5: The checklist editor

- **FR-5.1 Route.** `GET /ui/c/{id}/edit` returns a `<dialog class="modal
  modal--wide" data-autoshow data-refresh-on-close>` wrapping `<div
  id="checklist-editor">`, exactly like the milestone editor (SPEC-010 FR-6),
  so the existing `dialog.js` listeners open it and reload the page after a
  change.
- **FR-5.2 What it holds.** The jobs as a numbered list in their order, each
  with **Up** (not on the first), **Down** (not on the last), **Change…** (a
  disclosure with the title and note), and **Remove…** (a disclosure with an
  optional reason). Below the list, **Add a job**: a title and an optional
  note. A ticked job shows that it is ticked, but the editor has no tick: that
  is the page's job (SD-5).
- **FR-5.3 Routes.** `POST /ui/job/add` (`checklist_id`, `title`, `note`),
  `POST /ui/job/edit` (`job_id`, `title`, `note`), `POST /ui/job/move`
  (`job_id`, `place`) and `POST /ui/job/remove` (`job_id`, `reason`), each
  calling its store method as the UI actor.
- **FR-5.4 Responses**, as SPEC-010 SD-9: a post from inside the editor (HTMX
  without `HX-Boosted`) gets the editor back, marked changed, with a notice or
  an inline error; any other post gets the checklist page. A blank title is
  refused in a sentence: "A job needs a title, such as “Get the API key”."
  *AC for FR-5:* the fragment route returns the dialog and not a whole page;
  adding, changing, moving and removing from the editor each return the
  editor marked changed and write their audit row by the UI actor; the first
  job has no Up and the last no Down; a boosted post returns the checklist
  page; a blank title is refused (`TestUIChecklists`,
  `TestChecklistPagesRender`).

### FR-6: The milestone side

- **FR-6.1 The picker.** `MemberCandidates` gains a fourth branch: checklists,
  with an empty path, planned in the scope's subtree when the search box is
  empty and anywhere when searching, as milestones are (SPEC-010 FR-3.4). The
  picker shows them with their icon and "checklist". Current direct members are
  left out, as for every kind.
- **FR-6.2 The routes.** `POST /ui/milestone/member/add` and `/remove` accept
  `member_type` `checklist`. `AddMember` and `RemoveMember` need no change.
- **FR-6.3 The icon.** `icons.html` gains `i-checklist`, and `app.css` colours
  `data-entity="checklist"` like the other planning kinds.
  *AC for FR-6:* the editor's picker lists a checklist planned in its owner's
  subtree and not one planned elsewhere until searched for; adding it from
  the picker puts it in the milestone; a checklist already in the milestone
  isn't offered (`TestMemberCandidates`, `TestUIChecklists`).

### FR-7: The chat agent's tools

All tools name a checklist by id, or by exact name where that name is unique,
and a job by id, or by its exact title within the named checklist. Owners are
named by `owner_path` as the SPEC-010 tools do. Each write runs its store
method in one transaction as the MCP actor, is audited there, and signals the
SSE hub. Store refusals come back as sentences.

**Authoring**, under DEC-004 and DESIGN-010 §5c and §17a item 4. No quoted
words are needed:

- **FR-7.1 `create_checklist`**: `name`, optional `description`,
  `owner_path` (left out, the project) and `jobs` (an optional list of titles,
  added in order). Returns the checklist as `get_checklist` does.
- **FR-7.2 `add_job`**: `checklist`, `title`, optional `note` and
  `position` (counting from 1; left out, the end).
- **FR-7.3 `rename_job`**: `checklist`, `job`, `title`, and optional `note`,
  which replaces the note when given.
- **FR-7.4 `move_job`**: `checklist`, `job` and `position`.
- **FR-7.5 `remove_job`**: `checklist`, `job` and an optional `reason`.
- **FR-7.6 `list_checklists`**: optional `owner_type` and `owner_path`, the
  pair `list_milestones` uses. Each with its owner, jobs ticked and total, and
  whether it is done.
- **FR-7.7 `get_checklist`**: one in detail: owner, description, done, the
  jobs in order (id, position, title, note, ticked, and for a ticked job
  `ticked_by`, `ticked_at`, `ticked_via` and `ticked_quote`), the milestones
  it is directly in, and its page address.
- **FR-7.8 Milestones.** `add_milestone_member` and `remove_milestone_member`
  accept `member_type` `checklist`, with `member` the checklist's id or exact
  name. `get_milestone` lists a checklist member with its id and `done`.
  `mark_milestone_shipped`'s description says "at least one thing in it — a
  feature or a checklist — is done" (SD-3).

**The relay**, under DEC-006 Amendment 1 decision 8:

- **FR-7.9 `relay_tick_job`**: `checklist`, `job`, `ticked` (a boolean:
  `true` to tick, `false` to untick), an optional `note` and a required
  `quote`, the person's words. It calls `TickJob` with `via: mcp` and the
  quote, as the MCP actor, following `internal/server/mcp_relay.go`. Without a
  quote it is refused in the same sentence the other relays use, and nothing
  is written. Its description says to use it only when the person has said
  the job is done (or not done), and to quote them. It returns the job and the
  checklist's status. It lives in `mcp_relay.go` beside the other relays, and
  that file's header gains it.
- **FR-7.10 The boundary.** The advertised set is SPEC-010's twenty-five plus
  these eight, and `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` names
  each on purpose (DEC-005). `tick_job`, `untick_job` and `set_job_ticked`
  are added to that test's list of names that must not exist, because
  ticking without a quote isn't an act the chat agent has. The `initialize`
  instructions mention checklists, and that ticking a job is relayed with the
  person's words.
  *AC for FR-7:* over `POST /mcp`, the agent creates a checklist for an
  initiative with two jobs, adds, renames, moves and removes jobs, lists and
  gets it, adds it to a milestone and reads the milestone back with the
  checklist counted and not done; `relay_tick_job` without a quote is refused
  and writes nothing; with a quote it ticks, and the job and audit row carry
  `via: mcp`, the quote and the MCP actor; unticking works the same way;
  ticking a ticked job, an unknown job, a bad owner and a bad member type each
  fail with a sentence; each write leaves an audit row by the MCP actor
  (`TestMCPChecklistTools`, `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`).

## 4. Non-functional requirements

- **NFR-1 — No new stack.** HTML, HTMX and the existing `dialog.js`, which
  needs no change. `go vet ./...` and `go test -race ./...` stay clean.
- **NFR-2 — One service layer.** UI handlers and MCP tools call the store
  methods directly in a transaction. Neither issues HTTP to `/api/*`.
- **NFR-3 — No new gate, and no way round G4.** G4 keeps its shape (L-6). The
  chat agent gains no gate and no development act; its only new relay is the
  tick (SD-7).
- **NFR-4 — No typed entity path.** Every entity in a form is carried by id in
  a hidden field or chosen from a list (SPEC-007 SD-6). New fields are
  `checklist_id`, `job_id`, `title`, `note`, `tick`, `place` and `reason`. The
  render test scans the checklist page, the editor and an owner page with a
  checklist, against SPEC-010's banned list.
- **NFR-5 — Lists, not tables** (D-10). The page is a list of checkboxes and
  the editor a numbered list. The render test checks there is no table.
- **NFR-6 — Human prose** (D-6) in every label, empty state, notice, error,
  gate reason and tool description, with British spelling. No currency
  (`TestRenderedTemplatesCarryNoCurrency`).
- **NFR-7 — Audited both ways.** Every mutation from either surface writes its
  audit row in the same transaction (O-3), attributed to the UI actor or the
  MCP actor, with `via` on every tick and untick.
- **NFR-8 — Tested as before.** Integration tests drive each UI handler and
  each MCP tool against real Postgres, and the render tests cover the new
  templates.
- **NFR-9 — Shared files.** `entity.html` is shared with M8, so this spec
  changes nothing there beyond includes. The plan section, the create dialog
  and the member dialog live in `plan.html`, and the checklist page and editor
  in a new `checklist.html`.

## 5. Definition of done

1. Every FR's acceptance criteria pass in the suite, and the existing suite,
   including `TestUIBrowseAndDrive` and the render tests, still passes.
2. **A browser walkthrough without an AI provider.** Build `./cmd/subutai`,
   `init` a throwaway project, `serve` it from a short path, and with
   Playwright and the pre-installed Chromium: create a checklist from an
   initiative page, add jobs in the editor, put the checklist in a milestone,
   and show that the milestone isn't complete; then tick the jobs on the
   checklist page and show that it is. Screenshots saved.
3. **A relayed tick over MCP**, by JSON-RPC to `POST /mcp`, with a quote, and
   the quote shown on the checklist page.
4. Both recorded in `docs/walkthrough-spec-014.md`.
5. `go vet ./...` and `go test -race -count=1 ./...` clean, with the
   integration tests run, not skipped.
6. A handoff note records what was built, what was decided, and what needs
   Sam.

**For Sam to confirm:**

1. **A done checklist can satisfy G4 on its own** (SD-3), and G4's sentences
   count items. DEC-004 Amendment 1 says "features"; a dated note there would
   bring it into line.
2. **An initiative in a milestone doesn't bring in the checklists it owns**
   (SD-2).
3. **A checklist counts as one item**, not one per job (SD-1).
4. **Ticking on the page, editing in a modal**, with Edit on both the owner's
   page and the checklist page (SD-5).
5. **Removing a job is planning**, so the chat agent may remove an unticked
   job without quoted words (SD-7).
6. **One note per job**, replaced by a note given with a tick or untick (SD-6).
7. **Shipping doesn't freeze a checklist**: its jobs can still be ticked, and a
   later tick moves a shipped milestone's progress on, as a later-finished
   feature does (SD-4).

## 6. Open questions carried forward

- **Renaming and deleting checklists**, with milestones and roadmaps (SPEC-010
  §6). A delete could be limited to checklists in no milestone.
- **Owner and checklist pages don't refresh live** when the chat agent changes
  them (SPEC-010 §6).
- **A nested milestone shows as done in its parent when it is shipped**, not
  when everything in it is done. That predates this spec; with checklists it
  means a shipped milestone holding an unticked checklist shows as ticked in
  its parent. The parent's own count is right, because it resolves the nested
  milestone's leaves.
