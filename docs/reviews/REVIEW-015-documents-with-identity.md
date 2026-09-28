# REVIEW-015: Consistency Review of SPEC-015 (Documents with identity)

**Status:** Complete. The author has dealt with every finding (§6, and
[SPEC-015 §7](../specs/SPEC-015-documents-with-identity.md#7-changes-after-review));
awaiting Sam's decision.
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author. Approval is
Sam's.
**Scope:** [SPEC-015](../specs/SPEC-015-documents-with-identity.md) (first
draft, commit `6e9e092` on `claude/subutai-m8-documents-identity`), against
[DESIGN-010](../design/DESIGN-010-subutai.md) (§4, §5c, §7, §11, §12 "Bringing
a project in", §17a item 5),
[Subutai and GitHub](../research/subutai-and-github.md) §7,
[DESIGN-003](../design/DESIGN-003-document-lifecycle-and-gates.md) (§2, §5,
L-2), [DESIGN-004](../design/DESIGN-004-config-compartment.md) (templates),
[SPEC-009](../specs/SPEC-009-the-authoring-chain.md),
[SPEC-011](../specs/SPEC-011-send-to-development.md) (FR-5.5, FR-6, FR-9),
[DEC-003](../decisions/DEC-003-cli-scope.md),
[DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) with Amendment 1,
[DEC-006](../decisions/DEC-006-humans-start-development.md) with Amendment 1,
[DEC-007](../decisions/DEC-007-the-judgement-boundary.md), the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11 (M8) and §12 decision 9, the
[M7 handoff](../notes/handoff-M7-2026-09-28.md), the milestone owner's brief,
and the code at `6e9e092`: `internal/store/migrations/*.sql`,
`internal/store/migrate.go`, `documents.go`, `entities.go`, `tasks.go`,
`tracking.go`, `sends.go`; `internal/server/documents.go`, `actions.go`,
`authoring.go`, `review_send.go`, `planner.go`, `mcp.go`, `mcp_tools.go`,
`ui.go`, `ui_entity_actions.go`, `http_phase3.go`,
`integration_mcp_test.go`; `internal/rules/rules.go`;
`internal/content/parse.go`; `internal/config/compartment.go`;
`internal/lifecycle/validate.go`; `internal/starter/pack/templates/`;
`cmd/subutai/main.go`.

## 1. What this review is

An authoring consistency pass by a reviewer who did not write the spec. It
checks that SPEC-015 agrees with the approved documents it builds on, that what
it says about the code is true, that its requirements can be tested, and that
it covers the brief. It looks hard at three things the brief makes central:
whether adopt, or anything else here, gives the chat agent or a person a way
round review; whether the watcher's rules for moves and copies hold up; and
whether the migration is safe given M5's parallel `0009`. It does not fix the
spec.

**Timing.** While this review was running, the build started in the working
tree: `internal/store/migrations/0010_identity.sql`, `internal/ident/`,
`internal/content/identity.go`, and edits to `internal/store/documents.go`,
`entities.go`, `tasks.go`, `tracking.go`, `owner_reads.go` and
`internal/server/documents.go`. Findings are about the spec as committed.
Where the in-flight code already answers or differs from the spec, that is
noted.

The spec is careful and mostly accurate. Its claims about the code check out
with the exceptions noted in R15-13 and R15-16. The material findings are
about paths the spec opens without saying how they meet the rules around them.

## 2. Material findings (must fix before approval)

### R15-1 — Adopting as approved is a new way to reach `approved`, and it skips every check the other ways have

**What the spec says.** SD-11 and FR-5.3: a person adopting an unregistered
file may choose `approved`. This "is that person's approval ... and it has the
same consequences as approving it any other way: a spec adopted as approved can
let its feature pass G1." The follow-ups run "through the same event".

**The problem.** Every other route to `approved` goes through `reviewing`, and
DESIGN-003 §2's transition table has no other: "`approve` reviewing →
approved ... there is no force flag". The routes that exist today each carry
checks that adopt-as-approved would skip:

- **Validation.** `SubmitDoc` validates against the type's manifest before a
  document can reach `reviewing` (`internal/server/documents.go:154-199`).
  Adopt-as-approved would accept a spec with no acceptance criteria, or a
  dev-plan whose task table doesn't parse. The approval event then queues
  `DecomposeDevPlan` on an idea feature (`internal/rules/rules.go:481-495`),
  and decomposition expects the table to parse.
- **The pending-question check.** `DirectApprove` refuses while a
  review-escalation or design-revision question is waiting in the Inbox
  (`review_send.go:341-360`, `372-386`). Adopt has no such check.
- **One current contract document.** Adopt as approved can register a second
  live spec or dev-plan on a feature that already has one. The reads that
  decide "the current spec" take the newest non-superseded row
  (`internal/store/documents.go:97`, used by `evaluateContractGate`,
  `neededAuthoring` and `fileAuthoredDocument`). So the adopted file silently
  becomes the contract. On an `active` feature, no `revision-in-flight`
  question is raised (that runs only for a successor's submission,
  `rules.go:421-427`), so DESIGN-003 §5.3's promise that "work is never
  running against a spec that has silently changed underneath it" is broken.
- **A human verdict record.** `DirectApprove` writes `document.human_verdict`
  before approving. FR-5.3 says only "audited as the person's approval".

DEC-006 Amendment 1 does let a human approve a spec directly, and DESIGN-010
§7 does say adopt gives "a lifecycle state". So the act is allowed in
principle. What is missing is the rule that makes it the *same* act as a
direct approval rather than a shortcut round it.

**Evidence.** Spec SD-11, FR-5.3; DESIGN-003 §2, §5.3; DEC-006 Amendment 1
decisions 5 and 6 ("No setting lets a spec through unreviewed"); code as cited.

**Suggested fix.** Say that adopting as approved:

1. validates the file against its type's manifest when there is one, and
   refuses with the report if it fails;
2. makes the same pending-question checks as `DirectApprove`, and records
   `document.human_verdict`;
3. is refused for a spec or dev-plan when the owner already has a live
   document of that type, and for any spec or dev-plan on a feature that is
   past `ready` (the honest route there is Revise).

Alternatively, allow adopt-as-approved only for designs, decisions and the
untemplated types, and have specs and dev-plans adopted as drafts and then
submitted. Whichever is chosen, list it as a choice (§5 below), because it
is a policy decision about review, not a detail.

### R15-2 — A second live document of the same type breaks the "current document" reads, and starter designs make two live designs routine

**What the spec says.** SD-6: "A second live document of the same owner and
type takes `-2`". SD-15: every new initiative and feature gets a starter
design by default. SD-15's own rationale notes that "an empty draft would also
become the page's body in place of a design the person is about to attach".

**The problem.** The code assumes one live document per owner and type, apart
from an approved document and its successor. With two unrelated live
documents:

- **Specs and dev-plans.** As in R15-1: the newest one becomes the contract,
  whatever its state. A draft `FEAT-023-spec-2` hides an approved
  `FEAT-023-spec` from `evaluateContractGate` and the authoring invariant.
- **Designs.** `approvedDesigns` (`internal/server/planner.go:548-566`) and
  `buildReview` (`internal/server/documents.go:388`) take the *newest* design
  and include it only if approved. G0, by contrast, asks whether *any*
  approved design exists (`authoring.go:47-53`). So an initiative with an
  approved `INIT-014-design` and a newer draft `INIT-014-design-2` passes G0,
  yet its approved design is left out of every spec author's and reviewer's
  prompt. The author then writes a spec with no design in front of it.
- **The page body.** The body is the primary design, or "the first attached
  design document" (migration `0005`, comment on `is_primary`). With the
  default opt-in, a person who creates an initiative and then attaches or
  adopts their real design gets the untouched template as the page body. The
  spec names this as a reason for the opt-out, but leaves the default case
  unfixed.

**Evidence.** Spec SD-6, SD-15, FR-6; code as cited.

**Suggested fix.**

- Allow `-2` only for types where several are meaningful (design, research,
  note, report, policy). Refuse a second live spec or dev-plan on a feature,
  in adopt and in Attach alike.
- Make the design reads agree with G0: the newest *approved* design, not the
  newest design filtered by state.
- Say what happens to an untouched starter design when a person attaches or
  adopts a design for the same owner. For example: the new document is made
  primary, or an untouched starter (its hash unchanged since creation) is
  detached automatically, with a notice.

### R15-3 — Writing `id:` and `revision:` into an approved document is an edit the approved documents forbid, and the spec doesn't claim an exception

**What the spec says.** SD-11: adopt covers files adopted as approved, and
"a registered document with no ID" (in any state except `reviewing`) is given
an ID. FR-5.1 re-indexes in the transaction and commits afterwards, "so the
post-commit hook sees no drift". The acceptance adopts DEC-001 to DEC-007,
which are accepted decisions with no front matter.

**The problem.** DESIGN-003 §2 and L-2: "`approved` documents are immutable —
the git watcher flags any file change to an approved document's path as an
integrity violation". DESIGN-010 §11: "An accepted decision is never edited".
DEC-004 Amendment 1 was added "as an amendment rather than an edit, as DEC-005
asks of accepted decisions". The spec avoids the integrity checkpoint by
updating the stored hash in the same transaction. That is the mechanism, but
the spec never says this is a sanctioned exception to L-2, who may make it,
or why it is honest.

DESIGN-010 §7 ("touching only its front matter") and research §7 give a good
basis for the exception. The spec should cite them and draw the line itself.
Two points are open:

- **Over MCP.** SD-11 says "over MCP, adopt registers drafts only". It doesn't
  say whether `adopt_document` may give an ID to a document that is *already
  registered and approved*. If it may, the chat agent changes the bytes of an
  approved spec, and commits the change. That isn't a verdict, but it is a
  change to an approved document made on the agent's initiative.
- **A feature in development.** Giving an ID to the approved spec of an
  `active` feature changes the file on the main checkout while tasks run in
  worktrees against the old bytes. That is harmless, but the spec should say
  so.

**Evidence.** DESIGN-003 §2, §10 L-2; DESIGN-010 §11; DEC-004 Amendment 1
status line; `internal/rules/rules.go:615-630` (`decideFileChanged`).

**Suggested fix.** Add a scope decision: "Writing the two identity lines is
the one change Subutai makes to an approved document. Only Subutai writes
them, never the caller. The stored hash is re-recorded in the same
transaction, the change is audited as `document.adopted`, and the change is
committed on its own. This is the adoption DESIGN-010 §7 and research §7
provide for, and it isn't an edit in L-2's sense." Then say plainly whether
`adopt_document` may give an ID to a registered approved document. The
recommendation is yes, since the act is mechanical and audited, but it should
be Sam's call (§5).

### R15-4 — If `0009` is applied after `0010`, checklists never get IDs, and nothing notices

**What the spec says.** FR-2.3: the checklist block runs only if
`to_regclass('checklists')` is non-null. The acceptance checks "the block runs
cleanly both with and without the table".

**The problem.** The migrator fills gaps: it applies every unapplied migration
in version order, "a database that received 0008 before 0007 existed still
gets 0007 later" (`internal/store/migrate.go:106-110`). Any database that runs
this branch before M5 merges will apply `0010` (no checklists table, so the
block does nothing) and then, after the merge, apply `0009`. That creates
`checklists` with no `public_id` column, no default and no backfill. Checklists
then have no IDs for good, and `/ui/id/CL-…`, the registry test and any
future `NOT NULL` read fail in ways nobody will link to the order the
migrations ran in. The databases at risk are exactly the ones this work
creates: the test database, the `/var/tmp/m8demo` walkthrough and Sam's
development database. The in-flight `0010_identity.sql` says only "recreate
such a database".

**Evidence.** `internal/store/migrate.go:48-57`, `106-131`; spec FR-2.3;
in-flight `0010_identity.sql`, checklist block comment.

**Suggested fix.** Put the checklist block in a function created by `0010`
(say `ident_attach_checklists()`), which does nothing if it has already run.
Call it from `0010`, and again from the server at boot, after `Migrate`, so
the order the migrations ran in doesn't matter. Or have boot refuse to start,
with a sentence, when `checklists` exists without `public_id`. Add an
acceptance case: apply `0010`, then `0009`, then boot, and the checklists
have IDs.

### R15-5 — A draft of a type with no template can never be approved, so adopting decisions over MCP records accepted decisions as drafts for good

**What the spec says.** Over MCP, adopt registers drafts only (SD-11, choice
2). `decision` becomes a document type (FR-2.6). DEC-001 to DEC-007 must be
adoptable (FR-5 acceptance).

**The problem.** Only `design`, `spec` and `dev_plan` have templates in the
starter pack (`internal/starter/pack/templates/`). `SubmitDoc` needs the
type's manifest and fails without one (`internal/server/documents.go:159`),
and `DirectApprove` needs the document to be in `reviewing`. So a `decision`,
`note`, `research`, `report` or `policy` adopted as a draft can never be
submitted, and so never approved, by anyone. In practice:

- The chat agent, which is DEC-004's main planning surface and the natural
  way to adopt Tickly's 93 files, can only record accepted decisions and
  settled research as drafts. They then stay drafts for good, which is untrue.
- M11 pushes *accepted* decisions into prompts (DESIGN-010 §11). A decision
  adopted over MCP would be invisible to it.
- A relayed verdict, which DEC-006 Amendment 1 allows, can't help either,
  because it needs `reviewing`.

**Evidence.** `internal/starter/pack/templates/` (three types);
`internal/server/documents.go:154-165`; `review_send.go:372-386`; DESIGN-010
§11; DEC-006 Amendment 1 decision 8.

**Suggested fix.** Say how an adopted draft of an untemplated type reaches
`approved`. There are two ways:

- the UI's document page offers *Approve* on a draft of an untemplated type,
  as the person's verdict, which the chat agent may then relay under DEC-006
  Amendment 1; or
- `adopt_document` takes a `state` of `approved` together with the person's
  quoted words, as a relay. That needs an entry on DEC-006 Amendment 1's
  relay list, because "adding a relay tool that isn't on this list needs a
  decision".

Either way, choice 2's wording should say what MCP-adopted existing documents
end up as, and how they get out of `draft`.

### R15-6 — "A successor whose predecessor has no ID" blocks every document that was revised before M8, for good

**What the spec says.** SD-11 refuses "a document with an open successor, or
a successor whose predecessor has no ID, so a revision chain is never split
between two IDs."

**The problem.** A document keeps its `supersedes_id` after its approval.
Existing predecessors are superseded, with no ID, and can't be adopted.
Read literally, then, the second clause refuses every document that has ever
been revised, whatever its state, and nothing can ever lift the refusal.
Many of the documents a real project most wants to cite are exactly these
current revisions. If "open" was meant to cover both clauses, the sentence
doesn't say so. Nor does it say what revision an adopted successor gets
(1, or its position in the chain).

**Evidence.** Spec SD-11; `internal/store/documents.go` (`supersedes_id` is
never cleared); `internal/server/actions.go:314-422` (approval supersedes the
predecessor and keeps the link).

**Suggested fix.** Refuse only while a revision is *open* (a live successor
exists). Allow the approved head of a closed chain to be adopted, and give it
revision *n*, where *n* is its position in the chain (the predecessors,
unnumbered and archived, keep their row-id archive names). Or give it
revision 1 and say so. Add an acceptance case for adopting a document that
was revised before `0010`.

### R15-7 — The watcher sees only commits made in the operator's checkout, so moves that arrive by pull, merge or rebase stay detached until a restart

**What the spec says.** Goal: "Moving a file on disk doesn't detach it."
FR-4.1: "on every post-commit notification". FR-4.3: the boot catch-up scan
finds moves made while the server was down.

**The problem.**

- **Hooks.** The only hook installed is `post-commit`
  (`internal/starter/starter.go:108`, `cmd/subutai/main.go:95-103`). `git pull`
  (fast-forward or merge), `git merge` and `git rebase` don't run
  `post-commit`. A merge commit that does run it (a conflicted merge
  committed by hand) shows nothing to `diff-tree --name-only -r HEAD`
  without `-m` (`internal/server/documents.go:244`).
- **Copy then delete.** SD-10 rule 2 treats a copy as not a move. If the
  person then deletes the original in a later commit, that commit touches
  only the old path, so FR-4.1 reads nothing, and the row points at a missing
  file until the next boot.

On a team that shares a repository (the GitHub case in DESIGN-010 §12, and
Tickly), a colleague's `git mv` reaches the operator by pull. So the
milestone's done criterion holds only for moves made in the operator's own
checkout.

**Evidence.** Code as cited; spec Goal, FR-4.1, FR-4.3, SD-10 rule 2.

**Suggested fix.** Run FR-4.3's check ("registered documents with an ID
whose file is missing; if any, scan") from the heartbeat
(`internal/server/server.go:243`) as well as at boot. It costs one query
when nothing is missing. Also consider installing `post-merge` and
`post-rewrite` hooks that call the same endpoint. State the limitation in the
spec either way, and add an acceptance case for a move that arrives by a
fast-forward pull.

### R15-8 — The front-matter editor would give a CRLF or BOM file a second front-matter block

**What the spec says.** SD-8: "a file with no front matter gets a new block of
just those two lines, then a blank line, then the file as it was". NFR-2
tests this byte for byte.

**The problem.** Whether a file "has front matter" will, in practice, be
decided by `config.SplitFrontMatter`, which requires the file to start with
exactly `"---\n"` (`internal/config/compartment.go:153-156`). A file with
Windows line endings (`"---\r\n"`) or a UTF-8 byte-order mark counts as
having none. SD-8 would then put a new block above the existing one. The
result is byte-for-byte "minimal", but it changes the file's meaning. Its
real `title:`, `type:` and `owner:` become body text, the parse no longer
sees them, and validation of the adopted file fails. Adopt is the path for
brownfield repositories, where such files exist. The same question applies
to an `id:` that is not a plain scalar (quoted, or a block scalar), and to
an `id:` nested under another key.

**Evidence.** `internal/config/compartment.go:152-167`;
`internal/content/parse.go:44-52`; spec SD-8, NFR-2.

**Suggested fix.** Specify the editor's recognition rules: an optional BOM,
then `---` followed by LF or CRLF. Edits keep the file's line ending. Only a
top-level `id:` or `revision:` with a plain or quoted scalar value is
replaced, and any other form is refused with a sentence. A thematic break
(`---`) later in the body is not a fence. Add these cases to NFR-2's tests,
and have `SplitFrontMatter` accept CRLF, or refuse such a file, rather than
treat it as bare.

## 3. Smaller findings (should fix)

### R15-9 — Minting a document ID can race

FR-3.2 reads the owner's documents and then inserts. Two registrations for the
same owner and type at once (a double-clicked adopt, or an adopt beside an
author's filing) choose the same ID and revision. The unique index
`(public_id, revision)` stops one of them, but with a raw constraint error.
Entity and task IDs don't have this problem, because a sequence or a
row-locking `UPDATE … RETURNING` serialises them. **Fix:** lock the owner's row
(`SELECT … FOR UPDATE`) or take an advisory lock on the owner and type while
minting, or retry once on a unique violation. Say which, and give the refusal
sentence.

### R15-10 — An `id:` that doesn't fit is refused, with no way forward

FR-5.4 refuses a file whose front-matter `id:` doesn't fit the owner and type.
Two common cases hit this:

- a project with its own `id:` key (Tickly numbers its decisions);
- a person who copied an approved document to start a new one, carrying its
  `id:` along (SD-10 rule 2 treats the copy correctly, but adopt then refuses
  it as "already registered elsewhere").

The person is left to edit the front matter by hand, which R15-8's rules would
then have to accept. **Fix:** decide between refusing and offering to replace
the key, and list it as a choice. If the foreign key is kept, name where
Subutai's ID goes instead.

### R15-11 — Adopt's failure paths, and a race with the author, are not specified

- FR-5.1 writes the file inside the transaction. If the transaction then
  fails, the file must be put back. FR-6.3 says this for the starter design;
  FR-5 doesn't.
- If the git commit fails after the transaction (for example, `index.lock` is
  held), the row and the file agree but the change is uncommitted. The next
  adopt of that file is then refused as "uncommitted". Say what the notice
  says, and whether a retry commits the change.
- *Give it an ID* on a spec or plan draft whose author agent is at work would
  race `fileAuthoredDocument`. Refuse it as `refuseIfAuthorAtWork` does
  (`review_send.go:736-749`).
- Say that adopt ignores any `state:` or `status:` key in the file's front
  matter, so the file itself can't choose its lifecycle state.

### R15-12 — Detach now edits the file, which changes SPEC-011 FR-9.3, and the new paths clash with the `-2` names

SPEC-011 FR-9.3 says Detach "leaves the file on disk". SD-13 changes that, and
should say it amends FR-9.3. It also leaves the operator's checkout dirty. A
later adopt of the same file is then refused as uncommitted, which the notice
should explain.

Separately, SPEC-011 FR-9.3 has an author write "beside [a detached file] with
a numbered name". Under SD-16 that gives `FEAT-023-spec-2.md` for a document
whose ID is `FEAT-023-spec`, while SD-6 uses `-2` in *IDs* to mean a second
document. Name the numbered file by its ID and revision instead (for example,
`FEAT-023-spec.r2.md`), or say that the file name doesn't follow the ID in
this case.

### R15-13 — Document pages are addressed by file path, so a move breaks every link to the page

The note on prose says "row ids don't change, and URLs that use them keep
working". But entity and document pages are addressed by slug path and file
path: `/ui/i/{path…}`, `/ui/f/{path…}`, `/ui/d/{path…}`
(`internal/server/ui.go:609-611`). Only milestone, roadmap, task and run pages
use row ids. After a move, `/ui/d/<old path>` is a not-found page. That is a
visible detachment of exactly the kind the Goal rules out. **Fix:** have the
document page for an unknown path look up the latest `document.moved` from
that path and redirect, or say that `/ui/id/…` is the stable link and correct
the note on prose.

### R15-14 — The scans need their scope pinned down

- FR-4.1 reads the front matter of "each path the commit touched that exists".
  Limit it to Markdown files, so that a large vendored commit isn't read file
  by file.
- FR-4.3 reads "every tracked Markdown file". Say whether that means the
  working tree or `HEAD`. Reading the working tree follows an *uncommitted*
  move at boot that the live watcher would ignore.
- A document with an ID whose file was deleted, not moved, triggers the full
  scan on every boot, and on every heartbeat if R15-7's fix is taken. Record
  a document as missing once, and show it on its page, rather than scanning
  for it again and again.
- Say whether superseded rows (the archives) are followed when moved.
- SD-10 rule 2 records a copy "once". Say how "once" is known (for example,
  by an existing `document.copied` audit row for that path).

### R15-15 — The MCP parameters don't agree, and an ID can be mistaken for a slug

- FR-5.7 names the parameter `owner`. FR-7.5 names it `owner_path`. The brief
  says `owner`, and the existing tools use `owner_path`
  (`internal/server/mcp.go:148`). Pick one.
- FR-7.5 lets `path` take an ID. Slugs are lower-case with hyphens, so an
  initiative slugged `init-001` is ambiguous if IDs are matched without regard
  to case, as SD-18 does for `/ui/id/`. Say that MCP matches IDs in upper case
  only, or that an existing slug path wins.

### R15-16 — Milestone and roadmap lists on entity pages are in M5's `plan.html`

FR-7.2 puts IDs in "child lists" on entity pages. The milestone and roadmap
lists on project and initiative pages are rendered by `plan-section` in
`internal/server/ui/templates/plan.html`, which M5 owns. The milestone and
roadmap *pages* are in `entity.html` (`page-milestone`, `page-roadmap`), so
FR-7.1 is fine. **Fix:** add these lists to SD-19's deferred items.

### R15-17 — The starter design needs a few more details

- Is the starter marked primary? This matters for R15-2.
- FR-6.2 sets "the title". Say whether that means the front-matter `title:`,
  the `# {{what is being designed}}` heading, or both. The template has both
  (`internal/starter/pack/templates/design/template.md:2,7`).
- Name how the template is loaded: through `CompartmentRoot`, so that a
  project still under `.cromwell/` finds its template (the M7 compatibility
  rules).
- Say what the MCP `create_*` result reports when the git commit fails.

### R15-18 — An adopted file with no title is titled by its path

`RegisterDocForOwner` falls back to the path when the front matter has no
`title:` (`internal/server/documents.go:68`). DEC-001 to DEC-007 have no front
matter, so they would be listed as `docs/decisions/DEC-001-server-language-go.md`.
**Fix:** fall back to the first level-1 heading, and add that to FR-5.

### R15-19 — Which row does a shared ID mean, and what stops unrelated documents sharing one?

- While a revision is open, `FEAT-023-spec` names two live rows. SD-18 doesn't
  say where `/ui/id/FEAT-023-spec` goes. It should be the approved one, with
  the open revision shown on its page.
- The database allows two *unrelated* documents to share an ID at different
  revisions. Only FR-3.2's application code prevents it. Adopting a file that
  carries `id: FEAT-023-spec` and `revision: 3` isn't covered. Say that adopt
  sets the revision, and consider a check that a shared ID is always a
  predecessor–successor chain.

### R15-20 — Task numbers: past 99, and the feature's `updated_at`

SD-4 gives tasks "a two-digit number". Say that it grows past 99 (`-T100`), as
SD-3 says for entities. Also note that bumping `features.task_seq` fires the
features `updated_at` trigger on every task created, so a feature looks
"edited" whenever it is decomposed. Either accept that or say so.

### R15-21 — A draft whose `id:` line is removed is quietly no longer followed

SD-10 rule 3 makes the database the record, which is right. But a person or a
chat agent rewriting a draft without the `id:` line leaves a document that
will be detached by its next move, with no warning. FR-3.5 re-asserts the
lines only for author agents. **Fix:** re-assert the lines when a document
with an ID is submitted, or show "this file no longer carries its ID" on the
document page (compare §6's third open question).

### R15-22 — Prose

- SD-17 says "the ID leads" and also that "the name still reads first to the
  eye". Say which one is meant.
- FR-5's acceptance: "MCP refuses nothing but drafts, because it has no state
  parameter" says the opposite of what is meant. Try: "Over MCP, a file can
  only be adopted as a draft, because the tool has no state parameter."
- FR-1.3: "It is `NOT NULL` after the backfill". Say which column "it" is
  (`tasks.public_id`).
- The Status line says "Nine choices need Sam's explicit yes". §5 below
  suggests more.
- Spelling is British throughout, and "ID" and "row id" are kept apart
  consistently, apart from R15-13's point about URLs.

## 4. Checks that passed

- **The chat agent's boundary.** `adopt_document` over MCP registers drafts
  only. `create_initiative` and `create_feature` gain a starter design, which
  is authoring under DEC-004 ("author document content"). Neither is a relay,
  so DEC-006 Amendment 1's "adding a relay tool ... needs a decision" doesn't
  apply. Nothing here lets the chat agent approve, send, start building,
  answer a checkpoint or override a gate. `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
  gains one name, as SD-20 says. R15-3 and R15-5 are the only open points on
  this boundary.
- **Commit ordering.** FR-5.1 and FR-6.3 commit to git after the transaction,
  so the post-commit hook compares against the new hash. That matches
  `fileAuthoredDocument` and `commitDocument`
  (`internal/server/authoring.go:737-825`). Approval takeover and the
  cascade's mechanical archive update rows before `git mv`, so the watcher
  finds each archived file at its row's path and moves nothing
  (`actions.go:314-457`, `authoring.go:550-664`).
- **Successors.** Every successor is made by `ReviseDoc` (Revise, `RaiseIssue`
  on an approved spec, the cascade's in-flight route), so FR-3.4 covers them
  all. The takeover moves the predecessor's own file to the archive, so the
  archive keeps `revision: 1`, as FR-3's acceptance says. FR-3.5 covers an
  author filling an existing draft.
- **Claims about the code.** Roadmaps have no `created_at`, and row ids are
  UUIDv7 (`internal/store/store.go:50`, with a v4 fallback only if the clock
  fails). Slugs can't be renamed (`UpdateEntityFields` changes only name and
  description). G0 accepts the feature's own design or its immediate parent's
  (`authoring.go:59-64`). Triggers are for bookkeeping only (`0001` comment,
  DESIGN-001 §2.4). The archive name today carries the row id's last eight
  characters (`actions.go:424-431`). The authored path is
  `docs/<initiative-path>/<slug>/<type>.md` (`authoring.go:795-814`). The CLI
  still has `initiative` and `feature` commands, which DEC-003 removes.
- **Minting.** A column default drawing on a sequence can't be skipped by any
  insert, and doesn't race. The in-flight `0010` backfills in the order SD-5
  gives, pauses the `updated_at` triggers, and adds the `decision` enum value
  without using it in the same transaction, which Postgres allows.
- **Brief coverage.** Every item in the brief has a home in SD-1 to SD-20 or
  FR-1 to FR-7. The out-of-scope list matches it, and SD-19 keeps M5's four
  files untouched, apart from R15-16.

## 5. The choices

The nine in DoD 8 are the right kind of question, and the recommendations are
reasonable. Some comments:

- **Choice 1 (`PROJECT-<type>`).** This is fine. The ID parser must not treat
  `PROJECT` as a prefix, and the registry test should say so.
- **Choice 2 (MCP drafts only).** This is right under DEC-006 Amendment 1, but
  it can't stand alone. It needs R15-5's answer about how a draft of an
  untemplated type is ever approved.
- **Choice 5 (Detach removes the ID).** The alternative is to commit the
  removal with the tool's author, which keeps the checkout clean (R15-12).
  Offer both.
- **Choice 6 (starter designs on by default).** Consider a default per kind:
  on for initiatives, and for a feature only when its initiative has no
  approved design. The spec's own rationale (G0 lets a feature build from its
  parent's design) points that way. It would also avoid most of R15-2's
  two-designs cases.

Decisions that the spec takes silently and should list for Sam:

1. **The limits on adopting as approved** (R15-1): which types, and which
   checks.
2. **Which types may have a second live document** (R15-2).
3. **Writing the identity lines into approved documents and accepted
   decisions**, as a stated exception to DESIGN-003 L-2, and whether the chat
   agent may give an ID to a registered approved document (R15-3).
4. **How MCP-adopted documents of untemplated types reach approved** (R15-5).
   If the answer is a relay, it needs a line in DEC-006 Amendment 1.
5. **A foreign `id:` key: refuse, or replace** (R15-10).
6. **How to make the `0009`/`0010` order safe** (R15-4). This is a technical
   choice, but it affects M5's owner and the development databases, so it
   should be agreed.

## 6. Disposition

*By the author, 2026-09-28.* Every finding was accepted, and SPEC-015 was
revised; its §7 maps each finding to what changed. In short:

- **R15-1, R15-5, R15-10:** adopting as approved is limited to designs and the
  untemplated types, in the UI, and records a `document.human_verdict`. Specs
  and plans are adopted as drafts. A person can record an adopted untemplated
  draft as already approved from its page. A foreign `id:` is refused with a
  sentence. All three are listed as choices (10, 13, 14).
- **R15-2:** a second live spec or plan is refused by adopt; the design prompt
  reads now take the newest approved design; an untouched starter gives way to
  a design attached or adopted after it (choice 11). Attach's own refusal is
  left as an open question, since it changes pre-existing behaviour.
- **R15-3:** the identity lines are stated as the one sanctioned change to an
  approved document, and the chat agent may make it (choice 12).
- **R15-4:** taken as suggested: an idempotent `ident_attach_checklists()`,
  called by `0010` and by every migration run (choice 15).
- **R15-6:** refused only while a revision is open.
- **R15-7:** the missing-file check runs on every heartbeat, re-scanning only
  when `HEAD` moves. Extra hooks are left as an open question, because they
  change the M7 hook rules.
- **R15-8:** the editor's rules are in SD-8; the implementation already
  handled CRLF and a BOM, and the tests cover both.
- **R15-9 to R15-22:** as the review suggested, except R15-12's numbered
  names, which use `.2.md`, and R15-15, which keeps `owner_path`.

**The spec's choice 6** keeps starter designs on by default for features, and
records the review's alternative beside it for Sam.
