# SPEC-014: Checklists and jobs

**Status:** **Draft — for Sam's approval.** Authored by Claude; the approval
decision is Sam's, as the author can't be the approval gate. Sam approves the
spec and the build together. An independent consistency review,
[REVIEW-014](../reviews/REVIEW-014-checklists-and-jobs.md), found eight
material and five smaller problems in the first draft. All of them are dealt
with in this revision; §7 says how, finding by finding. The choices Sam must
confirm are listed in the definition of done (§5).
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
mcp`); DESIGN-010 §5c and §17a item 4 (the chat agent may create checklists
and jobs, as planning); [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md)
Amendment 1 (milestones and roadmaps are planning; G4 is a record-keeping
check); [DEC-005](../decisions/DEC-005-the-orchestration-boundary.md)
(widening the chat facet is always deliberate). Where this spec goes beyond
them, it says so and asks Sam (SD-3, SD-7, SD-10).
**Builds on:** [SPEC-010](SPEC-010-milestones-and-roadmaps-editing.md), whose
picker, editors, member-side dialog and MCP tools this spec extends, and the
[M4 handoff](../notes/handoff-M4-2026-09-28.md) follow-up 2. It changes some of
SPEC-010's wording; §8 lists where.
**Roadmap decision 8:** a job carries a title, an optional note, and who
ticked it and when. Owner and due date come later.
**Migration:** `0009`. Version `0010` is reserved for M8, which runs in
parallel; M5 merges first.

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

**Two claims, which the definition of done checks directly.** Together they
are the roadmap's M5 "done when": a milestone won't show as done, and can't be
marked shipped as complete, while one of its jobs is unticked.

> A milestone that contains a checklist doesn't show as complete while one of
> its jobs is unticked, and does once every job is ticked — whether the ticks
> come from the checklist page or are relayed from chat with the person's
> words.

> Marking such a milestone as shipped while a job is unticked records the
> checklist as not shipped, just as it does a feature that isn't done.

Two supporting claims:

> Every change to a checklist or a job, and every tick and untick, from either
> surface, is on the audit trail under the actor who made it. A relayed tick
> carries the person's quoted words.

> The chat agent can build and keep checklists as planning, but the only way it
> can tick or untick a job is by relaying a person's words, and no planning
> tool can make a checklist done in its place.

## 2. Scope

### In scope

1. **The data** (FR-1): migration `0009` adds `checklists` and `jobs`, and
   admits `checklist` as a milestone member and a snapshot leaf.
2. **Completion** (FR-2): when a checklist is done, how it counts in a
   milestone's progress, in gate G4 and in marking a milestone as shipped.
3. **The web UI** (FR-3 to FR-6): creating a checklist from the owner's page,
   the checklist page with its checkboxes, the checklist editor, and adding a
   checklist to a milestone from either end.
4. **The chat agent's tools** (FR-7): seven authoring tools and one relay
   tool, `relay_tick_job`.

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| Entity IDs such as `CL-002` | **M8**, which mints IDs for everything. Checklists and jobs are named by uuid, or by exact name or title, until then. |
| Renaming, re-describing or deleting a checklist | Not specified here, as for milestones and roadmaps (SPEC-010 §6). The handoff records it with the milestone follow-up, so both are done together. |
| A job's owner or due date | Later, as roadmap decision 8 says. |
| Moving a job to another checklist, or a checklist to another owner | Not specified. Remove and add again. |
| Owner and checklist pages refreshing live when the chat agent changes them | The same open question as SPEC-010 §6: entity pages don't listen to the SSE hub. |
| Checklists on the dashboard | The dashboard shows the project's own milestones and roadmaps. Project-owned checklists don't appear there; they are on the project page. |
| Adding a checklist to a milestone through the JSON API or the command line | Not built. `POST /api/milestones/members` still takes features, initiatives and milestones only, because the command line is being retired (DEC-003). `GET /api/milestone` and `subutai milestone show` do count checklists in their totals, because they read the same progress. |
| "New checklist here" in the owner page's "more" menu | Not added, on purpose: that menu is in `entity.html`, which M8 shares (NFR-9). New checklist sits in the plan section beside the other two. |
| Bugs, spikes, documents | Other milestones. |

### Scope decisions

- **SD-1 — A checklist counts as one item, not one per job.** A milestone's
  "X of Y items done" counts its resolved leaves. Today every leaf is a
  feature; now a checklist is a leaf too, done or not done as a whole.
  DESIGN-010 §6 counts *deliverables* ("3 of 4 done"), and a checklist is one
  deliverable, however many jobs it has. Counting each job would let a
  ten-job checklist outweigh three features in the count. The roadmap's
  "progress and the shipping gate count jobs" is read as "jobs count, through
  their checklist". The checklist's own page shows its jobs ("3 of 5 jobs
  ticked"). **For Sam to confirm.**

- **SD-2 — An initiative in a milestone doesn't bring in the checklists it
  owns.** An initiative member resolves to every feature under it, live. A
  checklist counts only when it is added to the milestone itself, directly or
  through a nested milestone.

  The design text points both ways, so here it is. DESIGN-010 §4 says an
  initiative deliverable means "everything under it", §6 says checklists
  "belong to" initiatives, and a checklist exists so that "a release can't be
  called done while the human chores are still open". Read together, that
  could mean an initiative brings in its checklists. This spec reads
  "everything under it" as the work tree — the features — and not the plan
  planned there, which is milestones, roadmaps and checklists. A milestone
  owned by an initiative isn't part of a milestone that contains that
  initiative either. Ownership says whose page a checklist is planned on
  (D-9); it doesn't decide what a milestone contains.

  The reason for this reading: a checklist planned in an initiative for some
  other purpose ("Things to do before the offsite") would otherwise hold open
  every release that contains the initiative, without anyone having chosen
  that. The cost is that a person must add a checklist to a milestone to make
  it count, which the picker makes one click. **For Sam to confirm.**

- **SD-3 — A done checklist can satisfy G4 on its own.** G4 lets a milestone be
  marked as shipped when at least one of its items is done. DESIGN-010 §6 says
  "at least one deliverable done", and §4 names a checklist as a deliverable,
  so a done checklist counts. A milestone made only of human chores ("Launch
  paperwork": the contract, the domain, the store listing) can then be marked
  as shipped when its checklist is done.

  **This loosens G4.** Before, a milestone with four unfinished features
  couldn't be marked as shipped. Now, if it also holds a done checklist, it
  can, with no code done. DEC-004 Amendment 1 let the chat agent mark
  milestones as shipped partly because G4, as it then stood, needed a done
  feature; now a relayed tick can satisfy it. G4 is a record-keeping check,
  not a development gate, and the shipped record still names every unfinished
  item as not shipped (SD-4), so the harm is small. But it is a real change.

  There are three ways to count:
  1. **Items** (recommended, and built): features and checklists alike, as the
     design's "deliverable" says.
  2. **Features first**: a done checklist counts only when the milestone
     resolves to no features at all. "Launch paperwork" can still ship, and a
     milestone with features still needs one of them done.
  3. **Features only**: a milestone of checklists alone could never be marked
     as shipped.

  G4's refusal sentences change from counting features to counting items, the
  word the progress line uses (FR-2.4). DEC-004 Amendment 1 describes G4 as
  stopping shipping "when none of its features is done", because features
  were the only leaves when it was written. It is an accepted decision, so this
  spec doesn't edit it; the handoff suggests a dated note once Sam decides.
  **For Sam to confirm.**

- **SD-4 — Shipping records an unticked checklist as not shipped, and a shipped
  record never goes backwards.** Marking a milestone as shipped snapshots its
  resolved leaves, checklists included, and records for each checklist whether
  it was done at that moment (`milestone_snapshots.done_at_lock`). The
  `milestone.locked` audit row gains `not_shipped`: each leaf, feature or
  checklist, that wasn't done, by type, id and name.

  After shipping, progress is measured against the snapshot. A feature's done
  is terminal, so its record only moves forward. A checklist's isn't: a job can
  be unticked or added later. So a checklist counts as done in the shipped
  record if it was done when the milestone shipped **or** is done now. A job
  ticked after shipping moves the record on, as a feature finished after
  shipping does; a job unticked or added after shipping doesn't move it back.

  The checklist itself isn't frozen: its jobs can still be ticked, unticked,
  added and removed, and its own page always shows its current state. Freezing
  it was the alternative (REVIEW-014 R14-1(a)); it would stop a person
  recording a chore that turned out to need redoing, just because a release
  that included it had shipped. **For Sam to confirm.**

- **SD-5 — Ticking happens on the checklist page; editing happens in a modal.**
  DESIGN-010 §6 says jobs "are ticked on the checklist page", and that
  milestones, roadmaps and checklists are each "edited in a modal from its
  owner's page". The brief asked for a page where you can tick, add, rename,
  reorder and remove. Both are met:
  - `/ui/c/{id}` is the checklist page: its jobs as plain checkboxes, ticked
    and unticked there, with an optional note (FR-4).
  - Adding, changing, reordering and removing jobs happen in the **checklist
    editor**, a modal loaded by HTMX like the milestone editor (FR-5). The
    owner's page opens it with **Edit** beside each checklist. The checklist
    page also has an **Edit this checklist** button in its header, following
    SPEC-010 SD-6, which Sam accepted for milestone and roadmap pages.
  **For Sam to confirm.**

- **SD-6 — A job has one note.** The note says something about the job: where
  the key is kept, why the contract is late. It can be set when the job is
  added, changed in the editor, or given when ticking or unticking. A note
  given with a tick or untick replaces the job's note; leaving it blank keeps
  the note as it was. Every note change is on the audit trail, with the old
  note, so nothing is lost. Keeping one note, rather than a note per tick,
  keeps a job a single line on the page, as D-10 asks. **For Sam to confirm.**

- **SD-7 — Only `relay_tick_job` ticks, and no planning tool can make a
  checklist done in its place.** DEC-006 Amendment 1 lists "a ticked job"
  among the acts the chat agent may relay, quoting the person, and DESIGN-010
  §5c repeats it. None of the authoring tools ticks.

  **What the written authority covers, and what goes beyond it.** DESIGN-010
  §17a item 4 says the chat agent "may create checklists and jobs", and §6
  says it can "create and fill" them. DEC-004 Amendment 1 names milestones and
  roadmaps only. So creating a checklist and adding jobs are covered.
  Renaming, moving and removing jobs are not named anywhere. This spec gives
  them to the chat agent as planning, like SPEC-010's milestone and roadmap
  edits, and asks Sam to confirm it.

  Two limits stop those tools standing in for a tick:
  - **A ticked job's title can't change**, from any surface. The tick is for
    the job as it was; renaming it would make the tick vouch for something
    nobody ticked. Its note can still change, and the change is audited with
    the old note (REVIEW-014 R14-2).
  - **The chat agent can't remove a checklist's last unticked job.**
    `remove_job` refuses when removing an unticked job would leave every
    remaining job ticked, because that makes the checklist done without anyone
    ticking anything — exactly the result the relay rule protects. The refusal
    tells the agent to relay the person's word if the job is done, or to leave
    removal to the person on the checklist's page (REVIEW-014 R14-3).

  Otherwise removal is planning, as `remove_milestone_member` is: audited
  under the chat agent's name with the reason, and undone by adding the job
  back. The alternatives are to make any removal of an unticked job a relay
  act, which needs a decision under DEC-006 Amendment 1, or to keep removal
  out of chat altogether. **For Sam to confirm**, with a dated note on
  DESIGN-010 §5c if he agrees.

- **SD-8 — Who ticked a job is the actor, how, and the words.** A job stores
  `ticked_by` (the UI actor or the MCP actor, as configured), `ticked_at`,
  `ticked_via` (`ui` or `mcp`) and, for a relay, `ticked_quote`. The checklist
  page shows them as stored: "Ticked by chat-agent 2 minutes ago, relaying the
  words “I've got the API key”." The audit row carries the same. Nobody's
  identity is proved, as DEC-006 Amendment 1 says; the quote makes a mistaken
  relay easy to spot.

- **SD-9 — An empty checklist is not done.** A checklist with no jobs has
  nothing ticked, so it counts as an unfinished item. That stops a checklist
  created as a placeholder counting as finished work.

- **SD-10 — Relaying an untick is inside DEC-006 Amendment 1.** The Amendment
  lists "a ticked job"; it doesn't mention unticking, and says a relay tool not
  on its list needs a decision. This spec reads an untick as the same act in
  the other direction: it touches the same record, starts nothing, can be
  undone, and corrects a tick that was wrong ("the key was revoked"). So it
  passes the Amendment's test of consequence, and `relay_tick_job` carries
  both, with the person's words either way. The alternative is a tick-only
  relay, with unticking left to the web UI. **For Sam to confirm**, with a
  dated note on DEC-006 Amendment 1 if he agrees.

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
    unticked), and `created_at`. Index on `(checklist_id, position)`. CHECKs:
    - `ticked_by` and `ticked_at` are null together;
    - `ticked_via` is null, `ui` or `mcp`;
    - `ticked_via` and `ticked_quote` are null unless `ticked_at` is set;
    - a tick through `mcp` has a quote.
  - `milestone_members_member_type_check` admits `checklist`, and
    `milestone_snapshots_leaf_type_check` admits `checklist`.
  - `milestone_snapshots` gains `done_at_lock boolean`, set for a checklist
    leaf and null for a feature, with a CHECK saying so (SD-4).
  *AC:* the migration applies on a fresh database (`TestMigrateFromEmpty`);
  the table refuses a project owner with an id, a chat tick without a quote,
  and a surface on an unticked job (`TestChecklistStore`).
- **FR-1.2 Checklists in the store** (`internal/store/checklists.go`):
  `CreateChecklist(ctx, tx, ownerType, ownerID, name, description, actor)`
  refuses a blank name or a bad owner before writing, and audits
  `checklist.created` against the checklist, with the owner in the payload —
  as `milestone.created` is filed against its milestone. `GetChecklist`,
  `ChecklistByName` (a shared name is refused with the count and a pointer to
  the id), `ListChecklists` and `ChecklistsOwnedBy` read them.
  `GetChecklistStatus` returns a `ChecklistStatus`: jobs, ticked, and
  `Done()` (SD-9).
- **FR-1.3 Jobs in the store.** Each writes in the caller's transaction and
  audits against the **checklist** (`ref_type` `checklist`), with `job_id` and
  the title in the payload, so the checklist's history is one query and
  survives the job's removal:
  - `AddJob(ctx, tx, checklistID, title, note, actor)` puts the job at the end
    (`job.added`).
  - `EditJob(ctx, tx, jobID, title, note, actor)` changes the title and note
    (`job.edited`, with the old title and note). A blank title is refused
    (`ErrJobTitleBlank`). A ticked job's title can't change
    (`ErrJobTickedRename`, SD-7); its note can.
  - `PlaceJob(ctx, tx, jobID, place, actor)` moves the job to a 1-based place
    and renumbers the checklist 1…n; any place outside 1…n means the end, as
    `PlaceRoadmapEntry` does (`job.moved`).
  - `RemoveJob(ctx, tx, jobID, reason, actor)` deletes the job and renumbers
    the rest (`job.removed`, with the title, note, whether it was ticked, and
    the reason).
  - `TickJob(ctx, tx, jobID, tick, note, via, quote, actor)` ticks or unticks
    it (`job.ticked` or `job.unticked`, with `via`, the quote when given, and
    the note when given). A non-blank note replaces the job's note (SD-6).
    Ticking a ticked job or unticking an unticked one is refused
    (`ErrJobAlreadyTicked`, `ErrJobNotTicked`), and so is a tick through `mcp`
    without a quote (`ErrRelayNeedsQuote`), so the rule holds below the tool.
    Nothing is written when refused.
  - `Jobs(ctx, q, checklistID)` returns them in order; `JobByTitle` finds one
    by exact title within a checklist and refuses a shared title with the
    count; `WouldFinishChecklist` says whether removing an unticked job would
    leave the checklist done (SD-7).
  - **Concurrency.** Every write that renumbers takes a row lock on the
    checklist, and `TickJob` and `EditJob` lock the job's row, so two writers
    can't both tick a job or collide on positions.
  *AC:* each write stores what it says and leaves one audit row with the right
  kind and actor; positions stay dense through add, move and remove; a blank
  name or title is refused; ticking twice, unticking an unticked job, a chat
  tick without words and renaming a ticked job are refused; a tick with a
  quote stores `ticked_via` and `ticked_quote`, and an untick clears all four
  tick fields; a shared title is refused as ambiguous (`TestChecklistStore`).

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
- **FR-2.4 G4 counts items, and the shipping sentences say so.**
  `LockMilestone` passes the item counts to G4, so a done checklist can
  satisfy it alone (SD-3). Every sentence that counted features now counts
  items:
  - G4's refusals (`lifecycle.G4`):
    - nothing resolves: "This milestone can't be marked as shipped yet,
      because nothing in it comes down to a feature or a checklist. Add the
      work it is meant to deliver first."
    - one item, not done: "…because the one item in it isn't done. Marking it
      as shipped records what actually went out, so at least one item has to
      be finished first."
    - several, none done: "…because none of its 3 items is done. Marking it as
      shipped records what actually went out, so at least one has to be
      finished first."
  - The editor's explanation beside **Mark as shipped**: "That records the 4
    items it covers (1 of them done so far) — features and checklists alike —
    and stops what it contains from changing … Anything not done yet is
    recorded as not shipped."
  - The notice after shipping: "Its record holds the 2 items it covers, 1 of
    them done, and later work won't change it. The one not done is recorded as
    not shipped."
  - A shipped milestone's editor: "Progress is measured against the items it
    covered when it shipped, and anything not done then is recorded as not
    shipped."
  - The chat tools: `mark_milestone_shipped` says it records "which items the
    milestone covers now — its features and checklists", and is refused "until
    at least one item is done"; `add_milestone_member` and
    `remove_milestone_member` name `checklist` among the member types; a bad
    member type is refused naming all four.
- **FR-2.5 Shipping.** `LockMilestone` snapshots the resolved checklists with
  `leaf_type = 'checklist'` and `done_at_lock` beside the features.
  `SnapshotProgress` counts a snapshotted checklist done if it was done at
  shipping or is done now (SD-4). The `milestone.locked` audit row keeps
  `leaves` (the number of features), and gains `checklists` (the number of
  checklists) and `not_shipped`; `done_at_lock` counts items of both kinds.
  `UnlockMilestone`'s audit row keeps `snapshot` as the features, and gains
  `snapshot_checklists`.
- **FR-2.6 Showing it.** In a milestone's editor, a checklist member is a row
  with the checklist icon, its name linked to `/ui/c/{id}`, ticked when it is
  done, and "3 of 5 jobs ticked · checklist" beside it. On the milestone's own
  page the row has the same icon, link and tick, and "checklist" beside it:
  that page's markup is in `entity.html`, which this spec doesn't change
  (NFR-9). `get_milestone` lists a checklist member with its `id` and `done`,
  and no `path`.
  *AC for FR-2:* a milestone with a done feature and an unticked checklist
  reads "1 of 2 items done", and "2 of 2" once every job is ticked; an empty
  checklist is not done; an initiative member doesn't bring in a checklist it
  owns, and a nested milestone does bring in its checklists; a milestone whose
  only member is an unticked checklist is refused in the "one item" sentence,
  and can be marked as shipped once it is done; shipping snapshots the
  checklist and names it in `not_shipped` when unticked; ticking it afterwards
  moves the shipped milestone's progress on, and unticking or adding a job to
  a checklist that was done at shipping doesn't move it back; reopening keeps
  the snapshot's checklists in the audit row; the token totals don't change
  when a checklist is added (`TestChecklistProgressAndShipping`, `TestG4`,
  `TestPlanEditorsRender`, `TestChecklistPagesRender`).

### FR-3: Creating a checklist from the owner's page

- **FR-3.1** The owner's plan section (SPEC-010 FR-2.1) lists the checklists
  planned there, below the milestones: each with the checklist icon, its name
  linked to its page, its status ("3 of 5 jobs ticked", "No jobs yet" or
  "Done: every job is ticked"), and an **Edit** button that opens the
  checklist editor (FR-5). The section's count includes them. Its lede and
  empty state mention checklists.
- **FR-3.2** **New checklist** sits beside **New milestone** and **New
  roadmap** in the plan section (not in the "more" menu; see §2). It opens a
  dialog asking for a name and an optional description, and posts to `POST
  /ui/checklist/new` with the owner in hidden fields (`owner_type`, `id`). It
  calls `CreateChecklist` and re-renders the owner's page with a notice:
  "Checklist created: *name*. Choose Edit beside it to add its jobs." A blank
  name is refused in a sentence and nothing is written.
  *AC for FR-3:* creating from an initiative page stores that initiative as
  the owner and lists the checklist in its plan section and not the project's,
  audited to the UI actor; a blank name is refused (`TestUIChecklists`,
  `TestChecklistPagesRender`).

### FR-4: The checklist page

- **FR-4.1** `GET /ui/c/{id}` shows, in the page head: the checklist icon and
  name, a state badge, "Planned in *owner*" linked, its status, and "Part of"
  the milestones it is directly in, linked. It has two header buttons: **Edit
  this checklist** (`hx-get` into `#modal-slot`, FR-5) and **Add to a
  milestone…** (FR-4.4). The description, if any, is the lede. A bad or
  unknown id is the not-found page, with a 404.
- **FR-4.2 The jobs, as checkboxes** (D-10). One row per job, in order. The
  checkbox is a button in a form posting to `POST /ui/job/tick` with
  `checklist_id`, `job_id` and `tick` (`true` or `false`); it has the
  checkbox role, its checked state, and the accessible name "Tick *title*" or
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
  isn't done."
- **FR-4.6 Stale pages.** Pages don't refresh live, so a person may act on a
  job someone else has just removed. Every job form, on the page and in the
  editor, carries `checklist_id` as well as `job_id`. A job that no longer
  exists, or isn't on that checklist, is answered on the checklist's page,
  brought up to date, with: "That job is no longer on this checklist, so
  nothing was changed. The page has been brought up to date."
- **FR-4.7 The address bar.** Every page body is `hx-boost`ed, so after a
  form post HTMX shows the post's route (such as `/ui/job/tick`) as the
  page's address, and an editor that reloads after a change (SPEC-010 FR-6.4)
  then lands on `/ui`. A page rendered in answer to a boosted post — the
  checklist page, the milestone page, and the project, initiative and feature
  pages — sends `HX-Push-Url` with its own address. This was found in the
  walkthrough and fixes the same path for SPEC-010's milestones and roadmaps.
  *AC for FR-4:* ticking and unticking from the page change the job and write
  `job.ticked` and `job.unticked` rows by the UI actor with `via: ui`; a note
  given with a tick replaces the job's note; ticking a ticked job and
  unticking an unticked one are refused in sentences; the page shows who
  ticked each job, and a relayed tick shows the quote; ticking a removed job
  gets the brought-up-to-date sentence; adding the checklist to a milestone
  from its page puts it in the milestone and taking it out records the
  reason; an unknown id is a 404; a boosted post that creates a checklist
  or ticks a job answers with `HX-Push-Url` set to the page it rendered
  (`TestUIChecklists`, `TestChecklistPagesRender`).

### FR-5: The checklist editor

- **FR-5.1 Route.** `GET /ui/c/{id}/edit` returns a `<dialog class="modal
  modal--wide" data-autoshow data-refresh-on-close>` wrapping `<div
  id="checklist-editor">`, exactly like the milestone editor (SPEC-010 FR-6),
  so the existing `dialog.js` listeners open it and reload the page after a
  change. An unknown id is the not-found page, with a 404.
- **FR-5.2 What it holds.** The jobs as a numbered list in their order, each
  with **Up** (not on the first), **Down** (not on the last), **Change…** (a
  disclosure with the title and note; for a ticked job, **Change the note…**,
  with the title shown as fixed), and **Remove…** (a disclosure with an
  optional reason). Below the list, **Add a job**: a title and an optional
  note. A ticked job shows that it is ticked, but the editor has no tick: that
  is the page's job (SD-5).
- **FR-5.3 Routes.** `POST /ui/job/add` (`checklist_id`, `title`, `note`),
  `POST /ui/job/edit` (`checklist_id`, `job_id`, `title`, `note`), `POST
  /ui/job/move` (`checklist_id`, `job_id`, `place`) and `POST /ui/job/remove`
  (`checklist_id`, `job_id`, `reason`), each calling its store method as the
  UI actor. A person can remove any job, including the last unticked one: the
  limit in SD-7 is on the chat agent.
- **FR-5.4 Responses**, as SPEC-010 SD-9: a post from inside the editor (HTMX
  without `HX-Boosted`) gets the editor back, marked changed, with a notice or
  an inline error; any other post gets the checklist page. Refusals are
  sentences: "A job needs a title, such as “Get the API key”." and "A ticked
  job's title can't change, because the tick is for the job as it was.
  Untick it first, or add a new job."
  *AC for FR-5:* the fragment route returns the dialog and not a whole page,
  and a 404 for an unknown id; adding, changing, moving and removing from the
  editor each return the editor marked changed and write their audit row by
  the UI actor; the first job has no Up and the last no Down; a boosted post
  returns the checklist page; a blank title and renaming a ticked job are
  refused (`TestUIChecklists`, `TestChecklistPagesRender`).

### FR-6: The milestone side

- **FR-6.1 The picker.** `MemberCandidates` gains a fourth branch: checklists,
  with an empty path, planned in the scope's subtree when the search box is
  empty and anywhere when searching, as milestones are (SPEC-010 FR-3.4). The
  picker shows them with their icon and "checklist". Current direct members are
  left out, as for every kind.
- **FR-6.2 The routes.** `POST /ui/milestone/member/add` and `/remove` accept
  `member_type` `checklist`. `AddMember` and `RemoveMember` need no change;
  the handler's member lookup gains a checklist case.
- **FR-6.3 The icon.** `icons.html` gains `i-checklist`, and `app.css` colours
  `data-entity="checklist"` like the other planning kinds.
  *AC for FR-6:* the editor's picker lists a checklist planned in its owner's
  subtree and not one planned elsewhere until searched for; adding it from
  the picker puts it in the milestone; a checklist already in the milestone
  isn't offered (`TestChecklistCandidates`, `TestUIChecklists`).

### FR-7: The chat agent's tools

All tools name a checklist by id, or by exact name where no other checklist
shares it. A job is named by id, which must belong to the named checklist, or
by its exact title within that checklist; a title two jobs share is refused
with the count, so a relay never ticks "the first of two". Owners are named by
`owner_path` as the SPEC-010 tools do. Each write runs its store methods in one
transaction as the MCP actor, is audited there, and signals the SSE hub. Store
refusals come back as sentences.

**Authoring**, as planning (SD-7). No quoted words are needed:

- **FR-7.1 `create_checklist`**: `name`, optional `description`,
  `owner_path` (left out, the project) and `jobs` (an optional list of titles,
  added in order). Returns the checklist as `get_checklist` does.
- **FR-7.2 `add_job`**: `checklist`, `title`, optional `note` and
  `position` (counting from 1; left out, the end). With a position it writes
  two audit rows, `job.added` and `job.moved`.
- **FR-7.3 `rename_job`**: `checklist`, `job`, `title`, and optional `note`,
  which replaces the note when given. Refused for a ticked job whose title
  would change (SD-7).
- **FR-7.4 `move_job`**: `checklist`, `job` and `position`.
- **FR-7.5 `remove_job`**: `checklist`, `job` and an optional `reason`.
  Refused when it would leave the checklist done (SD-7), in a sentence that
  says what to do instead.
- **FR-7.6 `list_checklists`**: optional `owner_type` and `owner_path`, the
  pair `list_milestones` uses. Each with its owner, jobs ticked and total, and
  whether it is done.
- **FR-7.7 `get_checklist`**: one in detail: owner, description, done, the
  jobs in order (id, position, title, note, ticked, and for a ticked job
  `ticked_by`, `ticked_at`, `ticked_via` and `ticked_quote`), the milestones
  it is directly in, and its page address.
- **FR-7.8 Milestones.** `add_milestone_member` and `remove_milestone_member`
  accept `member_type` `checklist`, with `member` the checklist's id or exact
  name, and their descriptions say an initiative doesn't bring in its
  checklists (SD-2). `get_milestone` lists a checklist member with its id and
  `done`. `mark_milestone_shipped`'s description counts items (FR-2.4).

**The relay**, under DEC-006 Amendment 1 decision 8 and SD-10:

- **FR-7.9 `relay_tick_job`**: `checklist`, `job`, `ticked` (a boolean:
  `true` to tick, `false` to untick), an optional `note` and a required
  `quote`, the person's words. It calls `TickJob` with `via: mcp` and the
  quote, as the MCP actor, following `internal/server/mcp_relay.go`. Without a
  quote, or without `ticked`, it is refused in a sentence and nothing is
  written. Its description says to use it only when the person has said the
  job is done (or not done), and to quote them. It returns the job and the
  checklist's status. It lives in `mcp_relay.go` beside the other relays; that
  file's header gains it, and its stale claim that no tool marks a milestone
  as shipped is corrected.
- **FR-7.10 The boundary.** The advertised set was twenty-five: SPEC-008's
  nine, SPEC-010's twelve and SPEC-011's four. It is now thirty-three, with
  these eight, and `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` names
  each on purpose (DEC-005). `tick_job`, `untick_job` and `set_job_ticked`
  are added to that test's list of names that must not exist. The
  `initialize` instructions mention checklists, and that a job's tick is
  relayed with the person's words.
  *AC for FR-7:* over `POST /mcp`, the agent creates a checklist for an
  initiative with two jobs, adds, renames, moves and removes jobs, lists and
  gets it, adds it to a milestone and reads the milestone back with the
  checklist counted and not done; `relay_tick_job` without a quote is refused
  and writes nothing; with a quote it ticks, and the job and audit row carry
  `via: mcp`, the quote and the MCP actor; unticking works the same way;
  ticking a ticked job, renaming a ticked job, a job id from another
  checklist, an unknown checklist, a bad owner and a bad member type each fail
  with a sentence; removing the last unticked job is refused, and relaying its
  tick makes the checklist done; the relayed words show on the checklist page;
  each write leaves an audit row by the MCP actor (`TestMCPChecklistTools`,
  `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`).

## 4. Non-functional requirements

- **NFR-1 — No new stack.** HTML, HTMX and the existing `dialog.js`, which
  needs no change. `go vet ./...` and `go test -race ./...` stay clean.
- **NFR-2 — One service layer.** UI handlers and MCP tools call the store
  methods directly in a transaction. Neither issues HTTP to `/api/*`.
- **NFR-3 — No new gate, and no way round G4.** G4 keeps its shape (L-6). The
  chat agent gains no gate and no development act; its only new relay is the
  tick and untick (SD-7, SD-10).
- **NFR-4 — No typed entity path.** Every entity in a form is carried by id in
  a hidden field or chosen from a list (SPEC-007 SD-6). New fields are
  `checklist_id`, `job_id`, `title`, `note`, `tick`, `place` and `reason`. The
  tests scan the checklist page, the editor, an owner page with a checklist and
  the milestone editor with a checklist in it, against SPEC-010's banned list.
- **NFR-5 — Lists, not tables** (D-10). The page is a list of checkboxes and
  the editor a numbered list. The render test checks there is no table.
- **NFR-6 — Human prose** (D-6) in every label, empty state, notice, error,
  gate reason and tool description, with British spelling and one word for
  one meaning ("item" for what a milestone counts). The activity feed reads
  the new audit kinds as sentences: "created a checklist", "added a job to a
  checklist", "changed a job on a checklist", "reordered a checklist",
  "removed a job from a checklist", "ticked a job" and "unticked a job". No
  currency (`TestRenderedTemplatesCarryNoCurrency`).
- **NFR-7 — Audited both ways.** Every mutation from either surface writes its
  audit row in the same transaction (O-3), attributed to the UI actor or the
  MCP actor, with `via` on every tick and untick.
- **NFR-8 — Tested as before.** Integration tests drive each UI handler and
  each MCP tool against real Postgres, and the render tests cover the new
  templates.
- **NFR-9 — Shared files.** `entity.html` is shared with M8, so this spec
  changes nothing there. The plan section, the create dialog and the
  milestone editor live in `plan.html`, and the checklist page and editor in a
  new `checklist.html`. `mcp.go` changes only in the registry line and the
  `initialize` instructions, not in `create_initiative` or `create_feature`.

## 5. Definition of done

1. Every FR's acceptance criteria pass in the suite, and the existing suite,
   including `TestUIBrowseAndDrive` and the render tests, still passes.
2. **A browser walkthrough without an AI provider.** Build `./cmd/subutai`,
   `init` a throwaway project, `serve` it from a short path, and with
   Playwright and the pre-installed Chromium: create a checklist from an
   initiative page, add jobs in the editor, put the checklist in a milestone
   beside a feature, and show that the milestone isn't complete; then tick the
   jobs on the checklist page and show that it is. Screenshots saved.
3. **Shipping with a job open.** In the same walkthrough, a second milestone
   holding a done feature and an unticked checklist is marked as shipped, and
   the notice and the audit row record the checklist as not shipped.
4. **A relayed tick over MCP**, by JSON-RPC to `POST /mcp`, with a quote, and
   the quote shown on the checklist page.
5. All of it recorded in `docs/walkthrough-spec-014.md`.
6. `go vet ./...` and `go test -race -count=1 ./...` clean, with the
   integration tests run, not skipped.
7. A handoff note records what was built, what was decided, and what needs
   Sam.

**For Sam to confirm:**

1. **G4 counts items**, so a done checklist can satisfy it on its own, which
   loosens it (SD-3). The alternatives are "features first" and "features
   only". DEC-004 Amendment 1 says "features"; a dated note there would bring
   it into line.
2. **An initiative in a milestone doesn't bring in the checklists it owns**
   (SD-2), reading DESIGN-010 §4's "everything under it" as the work tree.
3. **A checklist counts as one item**, not one per job (SD-1).
4. **Ticking on the page, editing in a modal**, with Edit on both the owner's
   page and the checklist page (SD-5).
5. **The chat agent may rename, move and remove jobs as planning**, which goes
   beyond DESIGN-010 §17a item 4's "create", with two limits: a ticked job's
   title can't change, and it can't remove a checklist's last unticked job
   (SD-7).
6. **Relaying an untick is inside DEC-006 Amendment 1** (SD-10).
7. **One note per job**, replaced by a note given with a tick or untick (SD-6).
8. **A shipped record never goes backwards, and the checklist isn't frozen**
   (SD-4): a checklist done at shipping stays done in the record; its jobs can
   still change on its own page.

## 6. Open questions carried forward

- **Renaming and deleting checklists**, with milestones and roadmaps (SPEC-010
  §6). A delete could be limited to checklists in no milestone. It matters
  more now: an empty checklist the chat agent created and added to a milestone
  keeps that milestone short of complete until someone takes it out.
- **Owner and checklist pages don't refresh live** when the chat agent changes
  them (SPEC-010 §6). FR-4.6 makes a stale page safe, not current.
- **A nested milestone shows as done in its parent when it is shipped**, not
  when everything in it is done. That predates this spec. The parent's own
  count is right, because it resolves the nested milestone's leaves.

## 7. Changes after review

How each [REVIEW-014](../reviews/REVIEW-014-checklists-and-jobs.md) finding
was dealt with.

| Finding | What changed |
|---|---|
| R14-1 (material) | SD-4 rewritten: the snapshot records `done_at_lock` for each checklist, and a shipped record counts it done if it was done then or is done now, so it never goes backwards. FR-1.1 and FR-2.5 add the column; FR-2's AC tests an untick and an added job after shipping. Choice 8 states both directions. |
| R14-2 (material) | A ticked job's title can't change, from any surface (`ErrJobTickedRename`); its note can, audited with the old note. SD-7, FR-1.3, FR-5.2, FR-5.4 and FR-7.3 say so, with ACs. |
| R14-3 (material) | SD-7 says plainly what §17a item 4 covers and what goes beyond it, adds the "last unticked job" limit on `remove_job`, and names the alternatives. The authority line no longer overreads §5c and §17a. Choice 5 rewritten. |
| R14-4 (material) | New SD-10 reads an untick as inside DEC-006 Amendment 1 and asks Sam, with the tick-only alternative. Choice 6. |
| R14-5 (material) | FR-2.4 lists every sentence that changed from "features" to "items"; FR-7.8 names the tool text; §8 lists the SPEC-010 requirements changed. The render tests check the new wording. |
| R14-6 (material) | FR-2.6: the jobs count is in the milestone editor, not on the milestone page, and `entity.html` is untouched. §2 says New checklist isn't in the "more" menu, on purpose. |
| R14-7 (material) | The Goal has the shipping half of M5's "done when"; DoD 3 ships a milestone with an open job; SD-3 says it loosens G4 and gives three ways to count. Choice 1 rewritten. |
| R14-8 (material) | FR-4.6: every job form carries `checklist_id`, and a gone job gets a sentence on the refreshed page. FR-5.1: an unknown editor id is a 404. FR-7: a job id must belong to the named checklist, and a shared title is refused with the count. ACs added. |
| R14-9 | SD-2 quotes DESIGN-010 §4 and §6 and says which reading it takes. |
| R14-10 | FR-1.1 adds two CHECKs; FR-1.3 requires the locks; `TickJob` refuses a chat tick without words. |
| R14-11 | NFR-6 lists the activity sentences; FR-1.2 says `checklist.created` is filed against the checklist, as `milestone.created` is; FR-2.5 defines the `milestone.locked` and `milestone.unlocked` fields. |
| R14-12 | §2 names the dashboard and the JSON API as out of scope; FR-2.6 says `get_milestone` gives a checklist's id and no path. |
| R14-13 | The ACs name real tests; FR-7.10 counts the tools correctly; FR-1.2 names `GetChecklistStatus`; FR-7.9 corrects the relay file's header; FR-7.2 says a position writes two rows; SD-1's arithmetic; "item" throughout; SD-8 says the actor is shown as stored. §4's migration note is in the header. |

## 8. Changes to SPEC-010

SPEC-010 is approved, so this spec supersedes the parts it changes rather than
editing it.

| SPEC-010 | What SPEC-014 changes |
|---|---|
| FR-3.3, FR-3.4 | `MemberCandidates` has the fourth branch FR-3.4 left room for: checklists. |
| FR-5.1 | G4's three refusals count items and name features and checklists (FR-2.4). |
| FR-5.2 | The explanation beside **Mark as shipped** counts items, and says anything not done yet is recorded as not shipped. |
| FR-5.3 | The notice after shipping counts items, and says how many are recorded as not shipped. |
| FR-5.4 | `milestone.unlocked` keeps the snapshot's checklists as `snapshot_checklists`. |
| FR-6.4 | Closing an editor after a change reloads the page the person was on, because pages rendered for a boosted post send `HX-Push-Url` (FR-4.7). |
| FR-7.3, FR-7.4 | `add_milestone_member` and `remove_milestone_member` take `member_type` `checklist`. |
| FR-7.8 | `get_milestone` lists checklist members with their id. |
| FR-7.9 | The advertised tool set is thirty-three. |
| FR-7.10 | `mark_milestone_shipped`'s description counts items. |
