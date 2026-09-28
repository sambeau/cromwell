# SPEC-015: Documents with identity

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. Sam has said they will
approve the spec and the build together. An independent review is recorded in
[REVIEW-015](../reviews/REVIEW-015-documents-with-identity.md). It found eight
material and fourteen smaller problems in the first draft; all are dealt with
in this revision, and §7 says how, finding by finding. Fifteen choices need
Sam's explicit yes (§5, DoD 8).
**Date:** 2026-09-28
**Roadmap milestone:** M8 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11. Roadmap decision 9 (§12) is the brief: prefixed IDs, identity in front
matter, and folders as the default for new documents only.
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28): §7 is the main source. Also §4 (documents and successors), §11
(decisions), §12 ("Bringing a project in") and §17a item 5 (the prefixes for
milestones, roadmaps, checklists and tasks, and the revision scheme; accepted).
**Research:** [Subutai and GitHub](../research/subutai-and-github.md) §7:
adopt in place, and never move an existing document.
**Builds on:** [DESIGN-003](../design/DESIGN-003-document-lifecycle-and-gates.md)
(the lifecycle, and revision by successor),
[DESIGN-004](../design/DESIGN-004-config-compartment.md) (templates),
[SPEC-009](SPEC-009-the-authoring-chain.md) (authored document paths, the
cascade and its archive) and [SPEC-011](SPEC-011-send-to-development.md)
(Submit, Revise and Detach).
**Coordination:** M5 (checklists and jobs) runs in parallel and merges first.
It owns migration `0009`; this spec owns `0010`.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6).

Two words are kept apart throughout:

- **ID** means the minted, human-readable identifier: `INIT-014`, `FEAT-023`,
  `FEAT-023-spec`, `DEC-005`.
- **row id** means the UUID primary key the database has always had. Row ids
  don't change.

Pages are addressed as before: initiatives and features by slug path,
documents by file path, and milestones, roadmaps and tasks by row id. The
stable link to anything with an ID is `/ui/id/<ID>` (SD-18), and a document's
old path redirects after a move (FR-4.5).

## 0. Framing

Today a document is known by its path. Move the file and Subutai loses it: the
row points at nothing, and the file at its new home is a stranger. Entities
have row ids and slugs but nothing a person would say out loud, so nobody can
write "see FEAT-023" and be understood.

DESIGN-010 §7 fixes both. Everything gets a short ID minted by the database.
A document's ID lives in its own front matter, so wherever the file goes,
Subutai finds it again. Existing projects are *adopted* where they sit:
Subutai writes two lines into each file's front matter and moves nothing.

What this spec adds:

- IDs for initiatives, features, milestones, roadmaps, checklists and tasks,
  minted from one sequence per prefix, with the existing rows numbered in the
  order they were created;
- document IDs and revision numbers, written into front matter;
- a watcher that follows a registered document to its new path;
- an *adopt* action, in the web UI and over MCP;
- a starter design document with each new initiative and feature, in a
  per-initiative folder;
- IDs on pages, in lists and in MCP results, and `/ui/id/<ID>`.

## 1. Goal

Every entity and document has a system-minted ID, and creating work creates
its documents. **Moving a file on disk doesn't detach it.**

Done looks like this, with no AI provider:

1. A person creates an initiative and a feature in the browser. Each page
   shows its ID, and each has a draft design document, already attached and
   committed, at `docs/work/INIT-001-auth/INIT-001-design.md` and
   `docs/work/INIT-001-auth/FEAT-001-design.md`.
2. They `git mv` the feature's design somewhere else and commit. The feature
   page still shows the document, at its new path.
3. They adopt an existing file, such as a decision called
   `docs/decisions/DEC-005-….md`, and it keeps the number `DEC-005`.
4. `/ui/id/FEAT-001` takes them to the feature.

## 2. Scope

### In scope

1. **Minting** (FR-1): a registry of prefixes, one Postgres sequence each, and
   a column default on each entity table.
2. **Backfill** (FR-2): migration `0010` numbers existing rows in creation
   order, including checklists if M5's table exists.
3. **Document IDs and revisions** (FR-3), in the database and in front matter,
   carried by successors, and used to name new archives.
4. **Identity in front matter** (FR-4): the watcher follows moves and renames.
5. **Adopt in place** (FR-5), in the UI and as the MCP tool `adopt_document`,
   including decisions that already carry a number.
6. **Creating work creates its documents** (FR-6), with an opt-out, and new
   features' authored specs and plans in the default folder.
7. **Showing IDs** (FR-7): titles, breadcrumbs, lists, MCP results,
   `/ui/id/<ID>`, and IDs accepted where MCP tools take a path.

### Out of scope

| What | Where it goes |
|---|---|
| The browser editor | M9 |
| The GitHub importer, and bulk adoption of a whole repository | M15a |
| Bug and spike entities. Their prefixes are reserved here (SD-2). | M12, M14 |
| Creating, viewing and surfacing decisions | M11. This spec only lets a decision be adopted and numbered. |
| Moving any existing document | Never. Existing documents stay where they are (research §7). |
| IDs in the checklist pages, the milestone and roadmap MCP results, and the relay tools | After M5 merges (SD-19). Those files belong to M5. |
| Per-user identity as the commit author | §13 of DESIGN-010. Until then Subutai commits as `subutai <subutai@localhost>`. |

### Scope decisions

**SD-1 — An ID is a column filled by a default, not by application code.**
Each entity table gains a text column `public_id`, `NOT NULL` and unique, with
the default `mint_ident('FEAT')` (and so on). `mint_ident` draws the next
number from that prefix's sequence and formats it. So every insert, from any
code path, gets an ID without asking for one, including inserts into tables
another session builds (checklists). The existing row ids stay the primary
keys, and every foreign key and URL that uses them is untouched.

*Why not a trigger:* DESIGN-001 §2.4 keeps triggers for bookkeeping only. A
column default is simpler still, and it can't be skipped.

**SD-2 — One registry of prefixes.** A new Go package, `internal/ident`, lists
every prefix once:

| Kind | Prefix | Table today |
|---|---|---|
| initiative | `INIT` | `initiatives` |
| feature | `FEAT` | `features` |
| bug | `BUG` | none yet (M12) |
| spike | `SPK` | none yet (M14) |
| decision | `DEC` | none: decisions are documents |
| milestone | `MS` | `milestones` |
| roadmap | `RM` | `roadmaps` |
| checklist | `CL` | `checklists`, from M5 |

Migration `0010` creates all eight sequences, `ident_<prefix>_seq`. Adding
bugs later is one registry line that already exists, plus the column default in
M12's migration. A test checks that the registry and the database's sequences
agree.

**SD-3 — The format.** The prefix, a hyphen, and the number padded to three
digits: `INIT-014`, `MS-004`, `DEC-008`. From 1,000 the number simply grows
(`FEAT-1000`); nothing is truncated. The number means nothing beyond "the next
one". It is never reused, and a rolled-back insert can leave a gap, which is
harmless.

**SD-4 — Tasks are numbered by their feature.** A task's ID is its feature's
ID, `-T`, and a two-digit number from the feature's own counter:
`FEAT-023-T03`. The counter is a new column, `features.task_seq`. `CreateTask`
increments it and writes the task's ID in the same statement. Abandoned tasks
keep their numbers, and a re-decomposition continues the count, so a number is
never given to two different tasks. Past 99 the number grows (`FEAT-023-T100`).
Bumping the counter touches the feature's row, so its `updated_at` moves when
tasks are created; nothing reads that column for display, and this is accepted.

**SD-5 — Backfill in creation order.** Migration `0010` numbers existing rows
by `created_at`, then row id. Roadmaps have no `created_at`, so they are
numbered by row id alone; row ids are UUIDv7, which are ordered by creation
time. Each sequence is then set past the highest number used. Checklists are
numbered only if the table exists (FR-2.3).

**SD-6 — A document's ID.** A document's ID is its owner's ID plus its type,
in the file-name form of the type (`dev_plan` becomes `dev-plan`):
`FEAT-023-spec`, `INIT-014-design`, `FEAT-023-dev-plan`.

- **A second live document** of the same owner and type takes `-2`, then `-3`:
  `INIT-014-design-2`. This is for the types where several make sense: design,
  research, report, note and policy. A feature has one current spec and one
  current plan, and the code reads "the current spec" as the newest live one,
  so adopt refuses a second live spec or plan (SD-11). (Choice 11.)
- **An ID with no live document** is reused by the next document of that owner
  and type, at the next revision. This is the cascade's case: when a design
  revision supersedes an idea feature's spec outright and a fresh one is
  written (SPEC-009 FR-9.4), the fresh spec is still `FEAT-023-spec`, now at
  revision 2. So "the current spec of FEAT-023" is always `FEAT-023-spec`, as
  DESIGN-010 §7 says.
- **A project-level document** uses `PROJECT` as its owner part:
  `PROJECT-design`, `PROJECT-note-2`. The project has no ID of its own, and
  DESIGN-010 doesn't give it one. (Choice 1.)
- **A shared ID is a line of revisions.** Several rows share an ID only as
  successive revisions: a successor made by Revise, or a fresh document after
  every earlier one was superseded. Unrelated documents never share one.
- **A decision** takes `DEC-nnn` from the decision sequence, with no owner or
  type part, because decisions are cited on their own (DESIGN-010 §7, §11).
  `decision` becomes a document type in migration `0010`. A decision belongs
  to the project or an initiative (§11), and adopt refuses any other owner.

**SD-7 — Where document IDs are stored.** `documents` gains `public_id` and
`revision`, both nullable, because a document registered before this spec, or
attached without adoption, has no ID (SD-12). `(public_id, revision)` is
unique. The ID is not unique on its own: while a revision is open, the
approved predecessor and its successor draft share it.

Minting a document ID reads what the owner already has and then inserts, so
two registrations at once could pick the same ID. Adopt and starter designs
take a transaction-scoped advisory lock on the owner and type while they
choose; anywhere else, the unique index refuses the loser with "FEAT-023-spec
revision 1 is already registered; try again".

**SD-8 — Front matter.** Subutai writes two keys, `id:` and `revision:`, and
nothing else. It edits the front matter as text, not by re-serialising the
YAML, so comments, key order, quoting and everything below the front matter
are kept byte for byte:

- an existing `id:` or `revision:` line at the top level is replaced;
- otherwise the two lines are inserted directly after the opening `---`;
- a file with no front matter gets a new block of just those two lines, then a
  blank line, then the file as it was. (DEC-001 to DEC-007 have none.)

A file has front matter when it starts, after an optional UTF-8 byte-order
mark, with a `---` line ending in LF or CRLF, and a later line is exactly
`---` (a CR before its LF allowed). The first such line closes the block; a
`---` further down the body is a thematic break, not a fence. Edits keep the
file's line endings. Only a top-level `id:` or `revision:` line is replaced;
an indented one belongs to another key and is left alone. A block that opens
and never closes is refused with a sentence rather than guessed at. These
rules are the identity editor's own: they recognise CRLF and BOM files that
`config.SplitFrontMatter` does not, so such a file never gets a second block.
Its title and sections are indexed as they are today.

**SD-9 — Archives are named by ID.** When a document with an ID is superseded
from now on, its file is archived as `docs/_superseded/<ID>.r<n>.md`, where
`n` is the revision it carried: `docs/_superseded/FEAT-023-spec.r1.md`. A
document with no ID is archived as it is today, with the row-id tail.
Existing archives are left as they are. If the target already exists, which
shouldn't happen, the row-id tail is added so nothing is overwritten.

**SD-10 — The watcher follows a document by its ID.** When a commit touches a
path, and the file there carries an `id:` and `revision:` belonging to a
registered document at a different path, Subutai updates that document's path
(FR-4). It doesn't detach it or register a second one. Three rules keep this
safe:

1. **A registered path is never taken over.** If the new path is already a
   registered document's path, nothing moves; the path wins.
2. **A copy isn't a move.** If the old path still exists and its file still
   carries the same ID and revision, the new file is a copy. Nothing moves, and
   the copy is recorded once on the audit trail.
3. **The database is the record.** The front matter is how a moved file is
   found. Editing the `id:` of a file at its registered path changes nothing.

Only Markdown files a commit touches are read. Moves made while the server was
down, and moves that arrive without a `post-commit` hook firing (a pull, a
merge or a rebase), are found by the check in FR-4.3, which runs at boot and on
every heartbeat. Superseded rows are followed too, so a moved archive stays
found. A copy is recorded once: by one `document.copy_ignored` audit row for
that path.

A document with no `id:` behaves exactly as today: it is known by its path,
and moving it leaves its row pointing at the old path.

**SD-11 — Adopt.** *Adopt* gives a file an ID, a type, a lifecycle state and
an owner, writes the ID into its front matter, and commits that change with
the tool's git author. It works on two kinds of file:

- **An unregistered file.** A person chooses its type and owner, and whether
  it is a `draft` or already `approved`.
- **A registered document with no ID.** Adopt gives it an ID and changes
  nothing else: not its type, owner or state. Its revision is 1: an earlier,
  superseded revision without an ID keeps its old archive name.

**Adopting as approved** records a verdict a person already gave outside
Subutai, so it is the adopting person's approval and is recorded as theirs,
as a `document.human_verdict` row like a direct approval's. It is allowed only
for a **design** and for the types with no template (**decision, research,
report, note, policy**), and only in the web UI. A spec or a plan is adopted
as a draft and goes through Submit and review like any other, because
approving one can release a feature through G1 and its plan is decomposed
into tasks. (Choice 10.) Nothing is validated on an approved adoption: the
file is a record of something already decided, and a brownfield design won't
have the template's headings. Approving a design by adoption has an
approval's consequences, through the same event: for a sent feature it can
release spec writing.

**Over MCP, adopt registers drafts only** (choice 2), because the chat agent
may relay a verdict but never give one (DEC-006 Amendment 1, DEC-007). A
draft of a type with no template can't be submitted, so the document page
offers a person **This was already approved** on such a draft (FR-5.8): the
same act as adopting it as approved, after the fact. (Choice 13.)

Adopt refuses, with a sentence saying why:

- a file with uncommitted changes, or one git doesn't track, so the commit
  holds nothing but Subutai's two lines (choice 3);
- a document already carrying an ID;
- a document in `reviewing`, because changing its content would make the
  review in flight stale (SPEC-011 FR-3.5);
- a document while a revision of it is open (it has a live successor, or it is
  a successor whose predecessor is still live), so the two can't end up with
  different IDs;
- a spec or plan draft whose author agent is at work on it, as Submit does
  (SPEC-011 FR-3.6);
- a second live spec or plan for a feature (SD-6);
- a file whose own front matter names an ID that doesn't fit the chosen owner
  and type, or one another document already has (choice 14). The sentence
  says to correct or remove the file's `id:` line. A project that uses its own
  `id:` key has to rename it first.

Adopt reads nothing else from the file's front matter: a `state:` or
`status:` key doesn't choose the lifecycle state. A file with no `title:` is
titled by its first level-1 heading, then by its path.

**Writing the identity lines is the one change Subutai makes to an approved
document or an accepted decision.** DESIGN-003 L-2 makes approved documents
immutable, and DESIGN-010 §11 says an accepted decision is never edited.
Adoption is the exception DESIGN-010 §7 ("touching only its front matter")
and research §7 provide for: only Subutai writes the lines, never the caller;
the stored hash is re-recorded in the same transaction, so no integrity
checkpoint is raised; the change is audited as `document.adopted`; and it is
committed on its own. It changes nothing a reader or an agent relies on. A
feature building against an approved spec is unaffected: its worktrees read
the spec's content, not its front matter. The chat agent may give an ID to a
registered approved document, because the act is mechanical, audited and
changes no verdict. (Choice 12.)

**SD-12 — Attach stays the escape hatch.** Attach registers a file by its path,
exactly as today. It writes nothing and gives no ID; the document page then
offers *Give it an ID*, which is adopt. Documents Subutai writes itself
(starter designs, authored specs and plans, successors of documents with IDs)
always get an ID. (Choice 4.)

**SD-13 — Detach takes the ID out of the file.** Detaching a draft that has an
ID also removes the `id:` and `revision:` lines from its file. Otherwise the
file would go on claiming an ID that now belongs to nobody, or later to a
different document. If the file had no other uncommitted changes, Subutai
commits the removal, so the checkout stays clean; otherwise the removal is
left uncommitted with the person's own changes. The notice says which. This
amends SPEC-011 FR-9.3, which left the file untouched. (Choice 5.)

A new document never writes over a detached file: an author then writes beside
it as `FEAT-023-spec.2.md`, not `FEAT-023-spec-2.md`, because `-2` in a name
reads as a second document's ID.

**SD-14 — The default home.** Documents Subutai creates for an initiative, or
for a feature under it, go in one flat folder per initiative:
`docs/work/<INIT-ID>-<slug>/`. A nested initiative gets its own folder beside
its parent's, not inside it, as DESIGN-010 §7 shows. Slugs can't be renamed
today, so the folder name is derived rather than stored. It is only a default:
a person may move the folder, and every document in it is still found by its
ID (SD-10). New documents for that initiative will then go to a fresh folder
at the default path. A folder that follows the moved documents is a follow-up
(§6).

**SD-15 — Starter designs, with an opt-out.** Creating an initiative or a
feature in the web UI or over MCP creates its design document from the
project's `design` template, in the default home, registered as a draft,
attached and committed (FR-6). There is an opt-out, *Start a design document*,
ticked by default in the UI and `design_document: false` over MCP.
(Choice 6.)

*Why an opt-out:* G0 lets a feature build from its parent initiative's design.
A feature meant to do that, or a placeholder feature added to a milestone,
doesn't need an empty design of its own.

The starter isn't marked as the main document. An **untouched starter** (still
a draft, and its file unchanged since Subutai wrote it) gives way: when a
person attaches or adopts another design for the same owner, that design is
marked as the main document, so the page shows it rather than the empty
template, and the notice says so. The starter stays attached, for the person
to detach. (Choice 11.)

A second, empty draft design mustn't hide an approved one from the agents
either. The reads that put a feature's designs into a spec author's or
reviewer's prompt take the newest *approved* design, not the newest design
filtered by state, so they agree with G0, which asks whether any approved
design exists (REVIEW-015 R15-2).

The HTTP API and the CLI create no document, as before. The CLI is being
retired (DEC-003), and scripts that call the API and then attach their own
design would otherwise gain a second one. (Choice 7.)

**SD-16 — New features' authored documents use the default home.** A feature
created after migration `0010` has its spec and plan written to
`docs/work/<INIT-ID>-<slug>/<FEAT-ID>-spec.md` and `…-dev-plan.md`. A feature
that existed before keeps SPEC-009's path, `docs/<initiative-path>/<slug>/spec.md`,
so its papers stay together. The migration marks the existing features with a
new column, `features.legacy_doc_paths`. Either way the authored document gets
an ID in its front matter.

**SD-17 — Showing IDs.** The ID goes before the name wherever the thing is
named for a person: page titles ("FEAT-023 Login form — Subutai"), the page
header, breadcrumbs, child lists, document lists, the documents page, the work
list, and MCP results. On a page it is a small, muted monospace label, so it
is there to cite without competing with the name. A document with no ID shows
none.

**SD-18 — `/ui/id/<ID>`** redirects to the page for that ID. It accepts any
entity ID, a task ID, a decision number, or a document ID, optionally with a
revision (`FEAT-023-spec.r1`). Lower-case is accepted there, because the route
only ever means an ID. A document ID with no revision names its approved
revision if it has a live one, otherwise its newest live revision, otherwise
its newest; the open revision is linked from that page, as today. An unknown ID gets the
not-found page with a sentence naming it. A checklist ID gets the not-found
page until M5's pages exist on this branch (SD-19).

**SD-19 — What waits for M5.** M5 owns `plan.html`, `ui_plan_actions.go`,
`mcp_plan_tools.go` and `mcp_relay.go`. This spec doesn't edit them. So until
M5 merges:

- the milestone and roadmap MCP tools *accept* `MS-` and `RM-` IDs (their
  lookups live in `http_phase3.go`), but their results don't *show* them;
- the relay tools take a path, as now;
- checklists get IDs in the database, but no page shows them;
- the milestone and roadmap lists on project and initiative pages, which
  `plan.html` renders, show no IDs (the milestone and roadmap *pages* do).

Each is a short follow-up after the merge, listed in the handoff.

**SD-20 — Coordination.** Migration `0010` only. M5's `0009` merges first, so
on the merged line `0010` runs after it. But the migrator fills gaps, so a
database that ran this branch first would apply `0010`, then `0009`, and its
checklists would never get IDs. So the checklist step is a function,
`ident_attach_checklists()`, that does nothing once done. `0010` calls it, and
so does every later run of the migrator. A database can only receive `0009`
through a run of the migrator (`serve` doesn't migrate; `init` and the test
harness do), and that same run gives the checklists their IDs, whichever order
the two migrations arrived in.
(Choice 15.) `entity.html` and the MCP
tool-set test are shared with M5, so changes there stay small: new markup goes
in its own partial file, `identity.html`, and the test gains one name.

## 3. Requirements

### FR-1: Minting

- **FR-1.1** Migration `0010` creates the eight sequences of SD-2, the function
  `mint_ident(prefix text) RETURNS text`, and a helper `ident_advance(prefix
  text, n bigint)` that moves a sequence to at least `n` (FR-5.5).
- **FR-1.2** `initiatives`, `features`, `milestones` and `roadmaps` gain
  `public_id text NOT NULL UNIQUE DEFAULT mint_ident('<PREFIX>')`.
- **FR-1.3** `tasks` gains `public_id text UNIQUE`, and `features` gains
  `task_seq integer NOT NULL DEFAULT 0`. `CreateTask` sets the task's ID from
  its feature's ID and counter in one statement (SD-4). `tasks.public_id` is
  `NOT NULL` after the backfill.
- **FR-1.4** The store's entity types carry `PublicID`, and every read that
  builds a page or an MCP result selects it.
- **FR-1.5** `internal/ident` holds the registry, `Format(prefix, n)`, and a
  parser that recognises an entity ID, a task ID, a decision number and a
  document ID (with an optional `.r<n>`).

*Acceptance:* creating two initiatives gives `INIT-001` and `INIT-002`; a
thousandth gives `INIT-1000`; a rolled-back create leaves a gap and nothing
else; a feature's third task is `FEAT-00n-T03`; the registry test passes.

### FR-2: Backfill

- **FR-2.1** Existing initiatives, features and milestones are numbered in
  `created_at` order (row id breaks ties); roadmaps by row id (SD-5). Each
  sequence continues from the highest number used.
- **FR-2.2** Existing tasks are numbered per feature, in `created_at`,
  `position`, row-id order, and each feature's `task_seq` is set to its count.
- **FR-2.3** Checklists: `ident_attach_checklists()` checks
  `to_regclass('checklists')`. If the table exists and has no `public_id`, it
  gains one with the `CL` default and existing checklists are numbered by row
  id. Otherwise nothing happens. `0010` calls it, and `store.Migrate` calls it
  after every run (SD-20).
- **FR-2.4** Existing features get `legacy_doc_paths = true` (SD-16); new ones
  default to `false`.
- **FR-2.5** Existing documents get no ID. Their files aren't touched (SD-12).
- **FR-2.6** `document_type` gains `decision`.

*Acceptance:* a test database with rows made before `0010` (applied up to
`0008`, rows inserted, then `0010` applied) gets IDs in creation order, with
per-feature task numbers, and the next create continues the sequence. The
checklist step runs cleanly without the table; with a table made before
`0010`; and with one made after it, as when `0009` arrives second.

### FR-3: Document IDs and revisions

- **FR-3.1** `documents` gains `public_id text`, `revision integer`, and a
  unique index on `(public_id, revision)` where `public_id` is set.
- **FR-3.2** Minting a document ID (SD-6) looks at the owner's documents of
  that type: the first of `base`, `base-2`, `base-3` … with no live document
  is used, at one more than the highest revision it has had (1 if none).
- **FR-3.3** Registration with an ID writes `id:` and `revision:` into the file
  (SD-8) *before* hashing and indexing it, so the stored hash is the file's.
- **FR-3.4** **Revise** on a document with an ID gives the successor the same
  ID and the next revision, and writes that revision into the working copy.
  Revise on a document with no ID works as today.
- **FR-3.5** When an agent writes an authored document (`fileAuthoredDocument`,
  including a successor it fills), Subutai writes the document's `id:` and
  `revision:` into the body before saving it. The agent never chooses them,
  as it never chooses the path.
- **FR-3.6** Archiving follows SD-9 on every path that supersedes a document:
  approval of a successor, the cascade's mechanical path, and a spec revision
  taking its plan with it.
- **FR-3.7** The prompt reads of a feature's designs take the newest approved
  design of each owner (SD-15).

*Acceptance:* a feature's spec is `FEAT-00n-spec` revision 1; Revise, submit
and approve makes the successor revision 2 at the canonical path and archives
`docs/_superseded/FEAT-00n-spec.r1.md`, whose front matter still says
revision 1. A cascade that supersedes an idea feature's spec outright is
followed by a fresh `FEAT-00n-spec` at revision 2. A document without an ID
revises and archives exactly as before.

### FR-4: Identity lives in front matter

- **FR-4.1** On every post-commit notification, for each Markdown path the
  commit touched that exists, Subutai reads the front matter. If it carries an `id:`
  and `revision:` that belong to a registered document at another path, it
  applies SD-10: update the path, or record the copy, or leave it.
- **FR-4.2** A move is recorded as `document.moved` (from, to), and the owner's
  page is told to refresh. The drift check then runs on the new path, so a move
  that also edits an approved document raises the integrity checkpoint, as any
  edit to an approved document does (DESIGN-003 §2). A move alone doesn't.
- **FR-4.3** At boot and on every heartbeat, Subutai looks for documents with
  an ID whose file is missing. That is one query and one `stat` per document
  when nothing is missing. If any are, it reads the front matter of the
  Markdown files git tracks, as they are in the working tree, once, to find
  them. A document still missing is looked for again only when `HEAD` has
  changed, so a deleted file doesn't cost a scan every heartbeat.
- **FR-4.4** A document with no ID is never matched by content or name. It
  keeps today's path identity (SD-10).
- **FR-4.5** The document page for a path with no document, where a document
  moved away from that path, redirects to its new path, so old links survive a
  move.
- **FR-4.6** A document page whose file no longer carries its registered ID
  says so, in a sentence, because its next move won't be followed.

*Acceptance:* `git mv` of a registered draft and commit: same row, new path,
one `document.moved` row, no second document. The same for a rename in the
same folder, for an approved document (no integrity checkpoint), for a move
made while the server was stopped, and for one that arrived with no hook
(found by the heartbeat's check). A copy leaves the original in place and adds
nothing but one audit row. A document without an ID, moved, is not followed.
The old path's page redirects.

### FR-5: Adopt in place

- **FR-5.1** A service method, `AdoptDocument`, used by the UI and by MCP,
  implements SD-11 in one transaction: register or update the row, write the
  front matter, index it, and audit `document.adopted` with the ID, type, owner,
  state and actor. The file is written inside the transaction and put back as
  it was if the transaction fails. It is committed as
  `subutai: adopt <path> as <ID>` after the transaction commits, so the
  post-commit hook sees no drift. If that commit fails, the document keeps its
  ID and the result says the change is left for the person to commit.
- **FR-5.2** **Owner and type.** The owner is the page it was adopted from, or
  over MCP an owner path or ID. The type is one of the document types; a
  `decision` must belong to the project or an initiative.
- **FR-5.3** **State.** `draft`, or `approved` for a design or a type with no
  template, in the UI; `draft` only over MCP (SD-11). Adopting as approved
  stamps `approved_at`, writes a `document.human_verdict` row, and has the
  follow-ups an approval has, through the same event.
- **FR-5.4** **An ID the file already carries** is kept if it is the one this
  owner and type could take (SD-6), and not already registered. For a
  decision, a well-formed `DEC-nnn` is kept, from the front matter or, failing
  that, the start of the file name (`DEC-005-the-orchestration-boundary.md`).
  An `id:` in the front matter that doesn't fit is refused; a file name that
  doesn't fit is ignored and an ID is minted. The revision is 1, or one more
  than the kept ID has had; the file's own `revision:` isn't trusted.
- **FR-5.5** Keeping a `DEC-nnn` moves the decision sequence past it, so the
  next decision minted is higher than any adopted.
- **FR-5.6** **In the UI:** the project, initiative and feature pages offer
  **Adopt a file…** beside Attach, with the file's path, its type and its
  state. A registered document with no ID offers **Give it an ID** on its page.
- **FR-5.7** **Over MCP:** `adopt_document(path, doc_type, owner_type,
  owner_path)`, returning the ID, revision, path and page URL. Its description
  says it is how to bring an existing file under Subutai, and that it only
  registers drafts.
- **FR-5.8** **This was already approved.** On the document page, a draft of a
  type with no template that has an ID offers a person this button. It is the
  approved adoption of SD-11 after the fact: a `document.human_verdict` row,
  `approved_at`, and the approval event. It is not offered over MCP.

*Acceptance:* adopting a committed Markdown file as a draft design of a
feature gives `FEAT-00n-design`, one commit by `subutai`, and a diff of
exactly the front-matter lines. Copies of this repository's DEC-001 to DEC-007
adopt as `DEC-001` to `DEC-007`, titled by their headings, and the next
decision is `DEC-008`. A document revised before `0010`, whose predecessor
has no ID, adopts at revision 1. Each refusal in SD-11 has a test. Over MCP a
file can only be adopted as a draft, because the tool has no state parameter.

### FR-6: Creating work creates its documents

- **FR-6.1** Creating an initiative or a feature (UI or MCP), unless opted out,
  creates its design document in the same transaction as the entity: the path
  is `docs/work/<INIT-ID>-<slug>/<ID>-design.md` (the feature's initiative's
  folder, for a feature); the ID is `<ID>-design`, revision 1.
- **FR-6.2** The body is the project's `design` template, read through the
  project folder in use (`.subutai/`, or `.cromwell/` under the M7 rules). Its
  front-matter `title:` and its `{{what is being designed}}` heading are set
  to the entity's name, and `owner:` to its path; the other placeholders stay
  for the person to fill in. A project with no `design` template gets a
  minimal file: front matter, a heading, and the three required headings.
- **FR-6.3** The file is written, registered and indexed before the
  transaction commits, and committed to git after it, with the tool's author.
  If the transaction fails, the file is removed. If an unregistered file
  already sits at that path, creation is refused rather than overwrite it.
- **FR-6.4** The opt-out (SD-15): a ticked *Start a design document* box in
  both create dialogs; `design_document` (default true) on `create_initiative`
  and `create_feature`.
- **FR-6.6** Attaching or adopting a design over an untouched starter marks the
  new design as the main document (SD-15).
- **FR-6.5** Authored specs and plans follow SD-16.

*Acceptance:* creating an initiative from the project page leaves a committed
`INIT-00n-design.md` in its folder, attached and marked draft; the notice says
so. With the box unticked, no file. The MCP tools do the same, and report the
document's ID and path. Attaching a real design afterwards makes it the page's
body.

### FR-7: Showing IDs

- **FR-7.1** Page titles and headers of initiatives, features, tasks,
  milestones, roadmaps and documents lead with the ID (SD-17).
- **FR-7.2** Breadcrumbs, child lists, document lists (on entity pages and the
  documents page) and the work list show IDs.
- **FR-7.3** `/ui/id/<ID>` (SD-18).
- **FR-7.4** MCP results that describe an initiative, a feature or a document
  include its `id` (and a document's `revision`).
- **FR-7.5** Where an MCP tool in `mcp.go` or `mcp_tools.go` takes an
  initiative or feature path, it also takes that entity's ID:
  `create_initiative.parent_path`, `create_feature.initiative_path`,
  `update_*.path`, `get_*.path`, and the `owner_path` of `attach_document`,
  `list_documents` and `adopt_document`. The milestone and roadmap lookups
  shared by the API and MCP also take `MS-` and `RM-` IDs (SD-19). Over MCP an
  ID is recognised only when written in capitals, as it is shown; slugs are
  lower-case, so an initiative slugged `init-001` is never mistaken for one.
  In a document result, `id` is the document's ID and `row_id` the row id that
  `id` used to carry.

*Acceptance:* render tests find the ID in each title and list; `/ui/id/…`
redirects for each kind; `get_feature(path: "FEAT-001")` returns the feature.

## 4. Non-functional requirements

- **NFR-1 — Nothing existing breaks.** Existing documents keep their paths,
  files and behaviour. The existing suite passes, changed only where a test
  asserted a whole title, list or archive name that now carries an ID.
- **NFR-2 — Minimal file changes.** Subutai's writes to a document's file are
  the two identity lines and nothing else (SD-8), tested byte for byte.
- **NFR-3 — Coordination.** Migration `0010` only; M5's files untouched
  (SD-19); new markup in `identity.html`; `entity.html` gains includes and the
  ID labels only.
- **NFR-4 — No typed entity paths.** The adopt form carries the owner's row id
  in a hidden field, like Attach; its only typed field is the file path.
- **NFR-5 — Human prose** in every notice, refusal and tool description (D-6).
- **NFR-6 — Tested as before.** Integration tests against real Postgres cover
  every FR. `go vet ./...` and `go test -race -count=1 ./...` are clean, and
  the integration tests run rather than skip.

## 5. Definition of done

1. Every FR's acceptance passes in the suite, including tests for minting and
   backfill, move and rename, adopt (with a pre-numbered `DEC-` file),
   create-with-entity, successor revision numbering, and the unchanged
   behaviour of documents with no ID.
2. `go vet ./...` and `go test -race -count=1 -v ./...` are clean, with the
   integration tests run.
3. **A browser walkthrough without an AI provider**, with Playwright and the
   pre-installed Chromium, from `/var/tmp/m8demo`: create an initiative and a
   feature and see their design documents with IDs; `git mv` one and commit,
   and see it still attached; adopt an existing file; IDs on pages; `/ui/id/…`.
   Screenshots and `docs/walkthrough-spec-015.md`.
4. REVIEW-015, by an independent subagent, and this spec revised against it
   (§7).
5. A handoff, `docs/notes/handoff-M8-2026-09-28.md`.
6. The roadmap's §11 marks M8 done with a pointer to the handoff, and nothing
   else there changes.
7. Committed in logical steps to this session's own branch, and pushed.
8. **Fifteen choices need Sam's explicit yes.** Each is the recommendation.
   The first nine were in the first draft; the review asked for the last six
   to be listed too.
   1. project-level documents are `PROJECT-<type>` (SD-6);
   2. over MCP, adopt registers drafts only (SD-11);
   3. adopt refuses a file with uncommitted changes, so its commit holds only
      Subutai's lines (SD-11);
   4. Attach stays path-only, with *Give it an ID* offered afterwards, and
      documents Subutai writes always get an ID (SD-12);
   5. Detach removes the ID from the file, committing that when the file was
      otherwise clean (SD-13);
   6. starter designs are on by default, for features as well as initiatives,
      with an opt-out in the UI and over MCP (SD-15). The alternative the
      review raised is to start a feature's design only when its initiative
      has no approved one;
   7. the HTTP API and CLI create no starter document (SD-15);
   8. existing features keep SPEC-009's paths for their authored documents;
      new features use the default home (SD-16);
   9. the default folder is derived, not stored, so moving it means new
      documents start a fresh one (SD-14);
   10. adopting as approved is for designs and the types with no template,
       in the UI only, and isn't validated; specs and plans are adopted as
       drafts (SD-11);
   11. a second live spec or plan is refused; other types may have `-2`; an
       untouched starter design gives way to a design attached or adopted
       after it (SD-6, SD-15);
   12. writing the two identity lines is the one sanctioned change to an
       approved document or accepted decision, and the chat agent may make it
       through `adopt_document` (SD-11);
   13. a person can record an adopted draft of a type with no template as
       already approved, from its page, not over MCP (SD-11, FR-5.8);
   14. a file whose front matter names an ID that doesn't fit is refused,
       rather than having its `id:` replaced (SD-11);
   15. the checklist step runs on every migration, so `0009` and `0010` can
       arrive in either order (SD-20).

## 6. Open questions carried forward

- **A folder that follows its documents.** If a person moves
  `docs/work/INIT-014-auth/`, new documents go back to the default path
  (SD-14). The alternative is to put new documents beside the initiative's
  design wherever it now lives. Worth doing once someone actually reorganises.
- **Bulk adoption.** Adopting Tickly's 93 files one at a time is tedious. M15a
  brings an importer with a dry run; it should call `AdoptDocument`.
- **A mismatched `id:`.** A person who edits the `id:` of a registered file is
  ignored (SD-10, rule 3). A later milestone could show the mismatch on the
  document page.
- **The M5 follow-ups** of SD-19.
- **More hooks.** `post-merge` and `post-rewrite` hooks would follow a pulled
  move at once rather than at the next heartbeat. They change what `init`
  installs and what `serve` refreshes (the M7 hook rules), so they are left
  for a milestone that touches those.
- **Refusing a second spec in Attach.** Adopt refuses a second live spec or
  plan (SD-6). Attach, which predates this spec, still allows one. It should
  refuse too; the change is small but alters Attach, so it is left for Sam to
  ask for.
- **Relaying "already approved".** If the chat agent should be able to relay a
  person's "that was already approved" for an adopted draft, it needs a line
  in DEC-006 Amendment 1's relay list (REVIEW-015 R15-5).

## 7. Changes after review

[REVIEW-015](../reviews/REVIEW-015-documents-with-identity.md) checked the
first draft (`6e9e092`). Each finding and what changed:

| Finding | What changed |
|---|---|
| **R15-1** Adopting as approved skipped every check | Allowed only for designs and untemplated types, in the UI; specs and plans adopt as drafts and go through review. It writes `document.human_verdict`. Not validated, and why (SD-11, FR-5.3, choice 10). |
| **R15-2** A second live document breaks "current document" reads | `-2` is for design, research, report, note and policy; adopt refuses a second live spec or plan (SD-6). The design prompt reads take the newest approved design (SD-15, FR-3.7). An untouched starter gives way to a later design (SD-15, FR-6.6, choice 11). Attach's own check is an open question (§6). |
| **R15-3** Identity lines in approved documents | Stated as the one sanctioned change, with the reasons and the mechanism; MCP may make it (SD-11, choice 12). |
| **R15-4** `0009` after `0010` | The checklist step is an idempotent function called by `0010` and by every migration run (SD-20, FR-2.3, choice 15). |
| **R15-5** Untemplated drafts can never be approved | *This was already approved* on the document page, for a person (SD-11, FR-5.8, choice 13); a relay is an open question (§6). |
| **R15-6** Documents revised before M8 could never be adopted | Refused only while a revision is open; a closed chain's head adopts at revision 1 (SD-11). |
| **R15-7** Moves that arrive by pull or merge | The missing-file check runs on every heartbeat (FR-4.3); more hooks are an open question (§6). |
| **R15-8** CRLF and BOM files | The editor's recognition rules are stated and tested (SD-8). |
| R15-9 Minting race | Advisory lock in adopt and starter designs; the refusal sentence elsewhere (SD-7). |
| R15-10 A foreign `id:` | Refused with a sentence saying what to do (SD-11, choice 14). |
| R15-11 Adopt's failure paths | File restored if the transaction fails; a failed commit is reported; an author at work refuses; front matter can't choose the state (SD-11, FR-5.1). |
| R15-12 Detach amends SPEC-011; numbered names | Stated; the removal is committed when the file was clean; numbered files are `.2.md` (SD-13). |
| R15-13 Document URLs break on a move | The old path redirects (FR-4.5); the note on prose corrected. |
| R15-14 Scan scope | Markdown only; working tree; re-scan only when `HEAD` moves; archives followed; "once" is an audit row (SD-10, FR-4.1, FR-4.3). |
| R15-15 `owner` vs `owner_path`; IDs vs slugs | `owner_path`; IDs recognised in capitals over MCP (FR-5.7, FR-7.5). |
| R15-16 Lists in `plan.html` | Added to SD-19. |
| R15-17 Starter details | Not primary; both titles; read through the project folder in use (SD-15, FR-6.2). |
| R15-18 Adopted files titled by path | First level-1 heading (SD-11). |
| R15-19 Which row a shared ID means | Approved first (SD-18); a shared ID is a line of revisions, and adopt sets the revision (SD-6, FR-5.4). |
| R15-20 Tasks past 99; `updated_at` | Stated (SD-4). |
| R15-21 A removed `id:` line | The document page says so (FR-4.6). |
| R15-22 Prose | Fixed as suggested. |
