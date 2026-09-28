# SPEC-016: Edit in the browser

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's, and Sam has said they
will approve the spec and the build together. An independent review is
recorded in [REVIEW-016](../reviews/REVIEW-016-edit-in-the-browser.md); §7
says how each finding was dealt with. Fourteen choices need Sam's explicit yes
(DoD 8).
**Date:** 2026-09-28
**Roadmap milestone:** M9 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11. Roadmap decision 11 (§12) is the brief's one fixed choice: *Save & commit*
is the main button.
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28): §7, "Editing in the browser", is the main source. Also §4
(successors), §5 (the cascade), §11 (decisions are never edited), §13 (people)
and §14 ("No rich text").
**Discussion:** [the response to the Subutai discussion](../notes/subutai-discussion-response-2026-07-31.md)
§6, "In-UI editing": optimistic locking, and the state and worktree
complications.
**Builds on:** [DESIGN-003](../design/DESIGN-003-document-lifecycle-and-gates.md)
(L-2: approved documents are immutable and change by successor),
[SPEC-011](SPEC-011-send-to-development.md) (Submit, Revise, Detach, the sent
mark, and the revise loop) and
[SPEC-015](SPEC-015-documents-with-identity.md) (`id:` and `revision:` in front
matter).
**Coordination:** M10 (the chat seat) runs in parallel and merges first. It
owns migration `0011`. This spec needs no migration. It doesn't touch
`internal/server/mcp*.go`, the MCP tool-set test, `plan.html`, or the
checklist and milestone files. The document page gains one include and the
button inside it.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal and notice a person reads is a full sentence
(DESIGN-008 D-6).

Three words are kept apart:

- **The file** is the Markdown file in the main working tree.
- **The text** is what is in the editor's text box.
- **The base** is the file as it was when the editor opened. The editor holds
  the base's hash, and a save compares it with the file.

## 0. Framing

Today the document page says "This document is written in your own editor".
Fixing a typo means leaving the browser, finding the file, editing it and
committing it. DESIGN-010 §7 adds a simple editor to the web UI for that
everyday case, and keeps everything else as it is: documents stay plain
Markdown files in git, and the editor works on the plain file (§14).

Three things make it more than a text box:

- **Two writers.** A person may have the same file open in vim. Neither the
  browser nor vim may silently overwrite the other, so the editor locks
  optimistically: it remembers what it loaded, and a save stops if the file has
  moved on.
- **The lifecycle.** An approved document is never changed in place
  (DESIGN-003 L-2). A document under review is being judged on a particular
  text. A spec or plan may be feeding work that is already running. The editor
  shows the state, says what an edit will do, and routes each case through the
  path that already exists.
- **Identity.** Since M8 a document's `id:` and `revision:` lines are how
  Subutai finds it wherever it moves. The editor keeps them intact.

## 1. Goal

A person can fix and polish documents without leaving the browser, and **a
browser edit and a vim edit can never silently overwrite each other.**

Done looks like this, with no AI provider:

1. A person opens a draft design, presses **Edit**, changes a paragraph, sees
   the preview, and presses **Save & commit**. The file changes, one commit
   holds just that file, authored by the configured operator, and the
   document's page shows the new text.
2. They open the editor again, change the same file in a shell meanwhile, and
   press **Save**. The save stops. The page shows what changed on disk and
   what they changed, keeps their text so they can copy it, and offers to
   reload.
3. They press **Edit** on an approved design. The editor says that editing it
   starts a successor draft, and that approving the successor supersedes the
   original and starts the cascade. They start the revision and edit the
   successor.

## 2. Scope

### In scope

1. **An Edit action** on the document page, opening a Markdown editor with a
   live preview (FR-1).
2. **Save & commit** and **Save** (FR-2).
3. **Optimistic locking**, with a readable line diff and a way forward (FR-3).
4. **The lifecycle**: drafts in place, approved documents by successor, and
   warnings for documents under review or feeding work in flight (FR-4).
5. **Identity**: the `id:` and `revision:` lines stay intact (FR-5).
6. **Reindexing** after a save, through the watcher's own path (FR-6).

### Out of scope

| What | Where it goes |
|---|---|
| Pushing to a remote, and the GitHub documents fast lane | M15 |
| Per-user authorship of commits | M15 (DESIGN-010 §13). Until then the author is the configured operator. |
| Editing through MCP | Not planned. The chat agent edits files directly, as it always has. |
| Rich text, WYSIWYG, or any format but the plain file | Never (DESIGN-010 §14). |
| Merging two versions for the person | Not planned. The editor shows the difference; the person merges. |
| Creating a new document from the editor | Not in this milestone. Creating work creates its documents (SPEC-015 FR-6), and Adopt takes existing files. |
| Editing a document's worktree copy | Never. Documents live in the main working tree (SPEC-009); worktrees are the agents'. |

### Scope decisions

**SD-1 — A plain text box, no library.** The editor is a `<textarea>`, in a
monospaced face, with a live preview beside it on a wide screen and below it on
a narrow one. The preview is rendered by the server with the same goldmark and
bluemonday renderer as the document page, so it looks exactly like the page
will. No editor library is vendored: a textarea is what "pleasant in vim" asks
for, it is accessible as it stands, and a syntax-highlighting editor is a lot of
vendored code for little gain on prose. A small script of our own
(`editor.js`, like `dialog.js`) adds three things a textarea lacks: Ctrl-S or
⌘-S to save, a copy button, and a warning before leaving with unsaved text.
Everything works without it. (Choice 1.)

**SD-2 — The editor edits the whole file, front matter included.** The
front matter holds the title, tags and, since M8, the identity lines. Hiding it
would mean a second editor for metadata, and a person fixing a title expects to
find it in the file. The identity lines are guarded instead (SD-10). The
preview shows the prose only, as the document page does. (Choice 2.)

**SD-3 — Save & commit is the main button.** Roadmap decision 11. It writes
the file and commits **that file only**, whatever else is staged or changed in
the working tree (`git commit --only`, after `git add` for a file git doesn't
track yet).

- **The author** is the configured operator, `server.ui_actor`, until per-user
  identity exists (DESIGN-010 §13). If `ui_actor` is already written as
  `Name <email>`, that is used as it stands. Otherwise the author is
  `<ui_actor> <<ui_actor>@localhost>`, with the email part made safe (letters,
  digits, dots, hyphens and underscores; anything else becomes a hyphen).
- **The committer** is `subutai <subutai@localhost>`, as for every other
  commit Subutai makes, so `git log --format='%an / %cn'` reads "operator /
  subutai": the operator's words, committed by the tool. It also means a
  project with no git identity configured can still commit. (Choice 3.)
- **The message** is a subject and a body:

  ```
  Edit FEAT-003-spec in the browser

  docs/work/INIT-001-auth/FEAT-003-spec.md, draft, revision 1.
  Saved from the Subutai web editor by operator.
  ```

  A document with no ID is named by its path. The person may write their own
  subject in an optional one-line box; the body is kept either way. (Choice 4.)

**SD-4 — Save writes the file only.** It leaves the commit to the person, for
someone who wants to batch several edits into one commit of their own.
DESIGN-010 §7 and the discussion response both ask for it.

**SD-5 — Save & commit won't sweep up someone else's uncommitted work.** If
the file already had uncommitted changes when the editor opened, Save & commit
would put them in a commit under the operator's name. So it is refused, with a
sentence and a way forward, when the base differs from the file as last
committed (or git doesn't track the file) **and** the base isn't something
Subutai wrote itself:

> docs/…/FEAT-003-spec.md has changes that aren't committed, made outside
> this editor. Commit or discard them first, or choose Save, which writes the
> file and leaves committing to you.

"Something Subutai wrote itself" means the text of the editor's own last
save of that document, or the working copy that Revise wrote for a successor
(FR-4.3), each identified by the hash recorded on the audit trail when it was
written. So Save followed later by Save & commit works, and so does editing
a fresh revision. Save alone is not refused: the person saw the whole file in
the editor, and Save commits nothing. (Choice 5.)

The check reads what the file holds against what `HEAD` holds, not the index:
Save & commit commits the file's content, so staged-but-uncommitted changes to
it count as uncommitted too.

**SD-6 — Optimistic locking by content hash.** The editor carries the base's
hash (the same SHA-256 hash the documents table uses) and the base itself. A
save, under a lock that serialises the editor's own saves, reads the file,
compares hashes, and refuses if they differ. Otherwise it writes the text to a
temporary file beside the original and renames it into place, keeping the
file's mode, so a reader never sees half a file. The window in which an
editor like vim could write between the check and the rename is a few
microseconds; it is the same window every editor has, and it isn't closed by
locking files, because vim doesn't take locks.

**SD-7 — A conflict shows the difference and keeps the person's text.**
"Never overwrite silently in either direction" means two things:

- the browser never overwrites a change made on disk since it opened; and
- a change made on disk never silently throws away what the person typed.

So a refused save re-renders the editor page, with:

- a sentence saying the file changed on disk since they opened it;
- **what changed on disk**: a line diff from the base to the file now;
- **what you changed**: a line diff from the base to their text;
- their text, still in the text box, with a **Copy your text** button; and
- **Reload from disk**, which opens the editor afresh on the file as it is
  now. It is a link, so their text is only lost when they choose it, and the
  page's leave warning asks first.

There is no "save mine anyway" button. Keeping a person's version over another
is a merge, which the person does by reloading and pasting. The editor also
checks the file every ten seconds while it is open, and says so at once if it
has changed, so the conflict is usually seen before the save rather than
after. (Choice 6.)

The diff is a plain line diff, computed on the server with a small longest
common subsequence routine (no library), and shown in unified form with three
lines of context: deleted lines struck and red, added lines green, both
marked `-` and `+` for readers who can't tell colours apart. Files too large
for it (more than four million line pairs) show both versions whole instead.

**SD-8 — The editor always shows the document's state, and each state has one
rule.**

| State | What Edit does |
|---|---|
| `draft` | Edits the file in place. |
| `reviewing` | Warns first, then edits in place; **saving takes the document back to draft and cancels its review** (SD-9). |
| `approved` | Doesn't edit it. Offers to **start a revision**: a successor draft through the existing Revise path, which the editor then opens (FR-4.3). If a revision is already open, Edit opens that. |
| `superseded` | No Edit button. A superseded document is a record. |

This is DESIGN-010 §7's list, turned into rules.

**SD-9 — Editing a document under review takes it back to draft.** A review is
a judgement of a particular text. Today, when a reviewing document changes on
disk, the reviewer's verdict is dropped as stale (SPEC-011 FR-3.5) and the
document is left in `reviewing` with no verdict coming: the spec gets stuck.
The editor does better than vim here, because it knows. Saving an edit to a
reviewing document therefore, first and in one transaction:

- cancels any queued review of it and clears any hold, as a person's
  send-back does;
- moves it back to `draft` by the lifecycle's existing `withdraw` event
  (`reviewing → draft`), audited with the cause "edited in the browser";

and then writes the file. A review already running finishes, and its verdict
is dropped as stale, as today. The person submits again when ready. No issue
is recorded: an edit is not an objection to be answered. For a sent feature's
spec or plan this leaves a draft with nothing against it, which belongs to
whoever is writing it (SPEC-011 FR-2.2), so nothing is dispatched. The editor
says this before the person starts (FR-4.2). (Choice 7.)

**SD-10 — The identity lines are kept intact.** A document with an ID can only
be saved if the text's front matter still reads `id: <its ID>` and
`revision: <its revision>`. Otherwise the save is refused:

> This document's ID is FEAT-003-spec, at revision 1, and the text you saved
> doesn't say so. Put back the lines `id: FEAT-003-spec` and `revision: 1` in
> its front matter and save again. They are how Subutai recognises this file
> wherever it moves.

Nothing is detached silently, and nothing is written. A document with no ID
can't gain one in the editor: adding an `id:` line is refused, pointing to
*Give it an ID* on the document page, which adopts it properly (SPEC-015
FR-5). A document with no ID whose file already declares one (the "this file
lost its ID" case of SPEC-015 FR-4.6, in reverse) must keep what it declares.
(Choice 8.)

**SD-11 — The editor stands aside while someone else holds the document.** It
refuses to open, or to save, with the existing sentences, when:

- **the author agent is at work on it** (a spec or plan whose authoring
  dispatch is queued or running). The agent writes the whole document, so a
  person's edit would be overwritten. This is the check Submit already makes
  (`refuseIfAuthorAtWork`);
- **a question in the Inbox governs it**, and the edit would change its state:
  a reviewing document with a `review-escalation` or `authoring-deadlock`
  question, or any document of a feature whose design-revision question is
  pending, when the edit would withdraw it from review or start a revision.
  This is the check every other act makes (`refuseIfPending`, SPEC-011 SD-11).
  A draft under an `authoring-deadlock` question may still be edited: editing
  the text answers nothing. (Choice 9.)

**SD-12 — Some approved documents can't be revised from the editor.**

- **An accepted decision is never edited** (DESIGN-010 §11). A new decision
  supersedes it, or a dated amendment is appended. Edit on an approved
  decision is refused with that sentence.
- **An approved document of a type with no template** (research, report, note,
  policy) makes a successor that can't be submitted, because Submit needs a
  template (the M8 handoff's follow-up 3). Starting one from the editor would
  strand it, so Edit says so instead, and names the way the file can be
  changed today. M10 lets a person rule on any document type; once it has
  merged, this refusal can go. (Choice 10.)

**SD-13 — Reindexing runs through the watcher's own path.** The brief asks to
check that the existing post-commit and heartbeat paths reindex an edited
document. **They don't both.** The post-commit hook does: it publishes
`document.file_changed` for every registered path a commit touches, and the
rule engine reindexes a draft or reviewing document. The boot catch-up scan
does too. **The heartbeat doesn't rehash documents at all**: it follows moved
documents by their IDs, but a file changed and not committed is reindexed only
at the next commit or restart. So after Save alone the index would be stale.

The editor therefore publishes the watcher's event itself, after every save,
with the commit hash when it made one. The rule engine then reindexes exactly
as for a vim edit that was committed, and the page, search and prompts read
the new sections. The post-commit hook still fires for Save & commit, finds
the hash already current, and does nothing more. Nothing is added to the
heartbeat. (Choice 11.)

**SD-14 — Warnings for documents that feed work in flight.** The editor shows
a warning, in a sentence that says what will happen, when the document belongs
to a feature that has been **sent to development** (the mark, or being built:
SPEC-011's `featureSent`), or is a **design** that features build from. The
cases are listed in FR-4.4. They are warnings, not refusals: the paths they
describe already exist and are safe. They appear on the editor page before the
text box, and the approved-document page shows its own before a revision is
started.

**SD-15 — Routes.** The document page is `/ui/d/<path>`, a wildcard, so the
editor has its own:

| Route | What it does |
|---|---|
| `GET /ui/edit/<path>` | The editor for the live document at that path. |
| `POST /ui/edit/save` | Save or Save & commit, by the button pressed. |
| `POST /ui/edit/preview` | The rendered preview of the text, as a fragment. |
| `POST /ui/edit/revise` | Starts a revision of an approved document and opens the successor's editor. |
| `GET /ui/frag/edit-fresh` | Whether the file still matches the base, as a fragment. |

Like the rest of the web UI, these act as the configured operator, on the
listener the project configured. The UI has no cross-site request protection
today; the editor adds none and takes none away (§6).

**SD-16 — The document page gains one include.** The banner "This document is
written in your own editor" is replaced by an include, `doc-edit`, in a new
template file, `edit.html`. It says the document can be edited here or in the
person's own editor, and holds the Edit button, labelled by state: "Edit",
"Edit — starts a revision", "Edit the open revision", or nothing for a
superseded document. That is the only change to the document page, so M10's
"written by" line merges cleanly beside it.

## 3. Requirements

### FR-1: The editor

**FR-1.1** `GET /ui/edit/<path>` shows the editor for the live document at
`<path>`. The page shows the document's ID and title, its state badge, its
type and revision, its owner, and its path, like the document page's head,
and a line saying what saving will do in this state (FR-4).

**FR-1.2** The text box holds the whole file (SD-2), monospaced, sized to the
window, with spell-checking off and no automatic capitalisation. A file that
starts with a byte-order mark is shown without it, and saved with it. The
browser sends line breaks as CRLF, so the text is saved with the file's own
line endings: LF unless the base used CRLF.

**FR-1.3** The preview renders the text's prose (front matter stripped) with
the document page's renderer, as the person types (after a short pause) and
when the page opens. It is labelled as a preview.

**FR-1.4** The buttons are **Save & commit** (primary), **Save**, and
**Cancel** (back to the document page). An optional one-line box, "Commit
message", is used by Save & commit (SD-3). Ctrl-S or ⌘-S presses Save.

**FR-1.5** While the editor is open, it checks every ten seconds whether the
file still matches the base, and shows a warning with a reload link if not
(SD-7).

**FR-1.6** A document whose file can't be read is refused: "The file for this
document can't be read from the working tree, so there's nothing to edit.
Restore it in git first."

**Acceptance:** the editor opens on a draft with its state shown and the whole
file in the text box; the preview fragment renders the text's prose; a CRLF
file keeps CRLF and an LF file keeps LF after a save from a browser's CRLF.

### FR-2: Save and Save & commit

**FR-2.1 — Save** checks, in order: the lifecycle (FR-4, SD-11), identity
(FR-5), and the lock (FR-3). Then it takes a reviewing document back to draft
(SD-9), writes the file (SD-6), audits `document.edited` with the path, the
base hash, the new hash, and whether it committed, and publishes the change
(FR-6). It re-renders the document page with "Saved. The file is written;
it isn't committed yet."

**FR-2.2 — Save & commit** does the same and then, if the file changed,
commits it alone (SD-3), after the check in SD-5. It re-renders the document
page with "Saved and committed as <short hash>." If the text is identical to
the file, nothing is written or committed, and the page says "Nothing changed,
so nothing was saved." If the commit fails after the write, the page says the
file was saved but not committed, and why.

**FR-2.3** A save that is refused re-renders the editor with the person's text
still in it, and the refusal as a sentence above it. Nothing is written.

**Acceptance:**
- Save writes the file, commits nothing, and the document page shows the new
  text;
- Save & commit makes one commit that touches only that file, with other
  staged and unstaged changes in the tree left as they were, the operator as
  author and `subutai` as committer, and the message of SD-3;
- a subject typed in the box becomes the commit's subject;
- Save & commit is refused on a file that had uncommitted changes when opened,
  and allowed after the editor's own Save and on a fresh revision's working
  copy.

### FR-3: Optimistic locking

**FR-3.1** The editor form carries the base's hash and the base. A save whose
base hash doesn't match the file's current hash is refused (SD-6), and the
editor re-renders as SD-7 says: the sentence, both diffs, the person's text,
Copy your text, and Reload from disk.

**FR-3.2** Two saves from the browser at once are serialised; the second sees
the first's write as a change on disk.

**FR-3.3** The freshness fragment says nothing while the file matches, and
"This file has changed on disk since you opened it. Saving will stop and show
you the difference." with a reload link when it doesn't.

**Acceptance:** a file changed in a shell after the editor opened is not
overwritten by Save or Save & commit; the refusal shows the shell's change and
the person's change; the file on disk is exactly the shell's version; the
freshness fragment reports the change.

### FR-4: The lifecycle

**FR-4.1 — Drafts** are edited in place (SD-8), unless the author agent is at
work on the document (SD-11).

**FR-4.2 — A document under review** opens with a warning:

> This document is under review. Saving a change takes it back to draft and
> cancels its review, because the review was of the text as submitted. Submit
> it again when you're done.

For an agent-reviewed type the warning adds: "A review already running will
finish, and its verdict will be set aside." Saving does what SD-9 says. It is
refused while a question in the Inbox governs the document (SD-11).

**FR-4.3 — An approved document** isn't edited. `GET /ui/edit/<path>` shows a
page that says:

> This document is approved, so it isn't changed in place. Editing it starts a
> revision: a successor draft with the same ID at revision <n+1>, in a working
> copy at <path>. The original stays approved, and in force, until the
> revision is approved; then the revision takes its place and the original is
> archived.

For a **design** it adds the cascade:

> Approving a revised design starts the cascade: the specifications written
> from this design are found, and each is either superseded or put to you in
> the Inbox, spec by spec.

For a spec or plan of a feature being built it adds the revision-in-flight
warning (FR-4.4). The page has one button, **Start a revision and edit it**,
which posts to `/ui/edit/revise`. That runs the same checks and the same
`ReviseDoc` as the document page's Revise button (SPEC-011 FR-9.2), and opens
the successor's editor with the notice "A revision was opened. The original
stays approved until this is." If a revision is already open, `GET` redirects
to its editor instead. Decisions and approved documents of untemplated types
are refused (SD-12).

**FR-4.4 — Work in flight** (SD-14). The warnings, each a sentence:

| When | The warning |
|---|---|
| A spec or plan of a feature that is sent, not yet being built | "FEAT-003 has been sent to development, and this <specification> is part of its contract. The next steps are written from the approved <specification>, so your change reaches them once it is approved." |
| A spec or plan of a feature being built (`active` or `review`) | "FEAT-003 is being built. Agents work in their own worktrees and won't see this change until it's approved and merged into their work. Submitting a revised <specification> stops new tasks starting until you answer a question in the Inbox." |
| A design that a sent or building feature builds from (its own, or its immediate parent initiative's) | "FEAT-003 and FEAT-004 build from this design and have been sent to development. A change to a draft or reviewing design reaches their specifications when it is approved; approving a revision starts the cascade." |
| A draft spec or plan with an open issue or major finding | "This draft is waiting for its author to address review findings. If you address them yourself, submit it when you're done." |

A document may show more than one. A document none of these applies to shows
none.

**Acceptance:**
- a reviewing document's editor shows the warning, and a save takes it to
  `draft`, cancels its queued review, and writes the file;
- an approved design's editor shows the revision and cascade text, and
  starting the revision creates a successor draft with the same ID at the
  next revision, whose editor opens;
- Edit on an approved document with an open revision goes to that revision;
- the sent and building warnings show for a sent feature's spec;
- the editor refuses while the author agent's dispatch is live, and on an
  approved decision.

### FR-5: Identity

**FR-5.1** A save of a document with an ID is refused unless the text carries
the same `id:` and `revision:` (SD-10). The refusal names both lines.

**FR-5.2** A save of a document with no ID is refused if the text declares an
`id:` its file didn't.

**FR-5.3** A refused save writes nothing and changes no state.

**Acceptance:** deleting the `id:` line, changing it, and changing the
revision are each refused with the sentence, the file and the row unchanged;
adding an `id:` to a document with none is refused; a save that keeps them
succeeds.

### FR-6: Reindexing

**FR-6.1** After a save that wrote the file, the editor publishes
`document.file_changed` for the document's path, with the commit hash when it
made one (SD-13). The rule engine reindexes it: its sections and title follow
the file.

**Acceptance:** after Save alone, with no commit, the document's sections
reflect the new text, and its content hash is the new file's hash; after Save
& commit, the same; the approved document behind an open revision raises no
integrity question.

### FR-7: The document page

**FR-7.1** The document page's include (SD-16) offers Edit by state and says
why when it doesn't: "A superseded document is a record, so it isn't edited."
or the SD-11 or SD-12 sentence.

**Acceptance:** each state's page shows the right label, and the superseded
page none.

## 4. Non-functional requirements

- **NFR-1 — Nothing existing breaks.** The suite passes unchanged except where
  a test asserted the old banner's words.
- **NFR-2 — Minimal writes.** A save writes the person's text and nothing
  else; Subutai adds no lines. A save that changes nothing writes nothing.
- **NFR-3 — Coordination.** No migration. New code in `edit.go`, a diff
  package, `edit.html` and `editor.js`; `entity.html` changes by one include;
  `partials.html` loads `editor.js`. None of the files M10 owns.
- **NFR-4 — No external libraries.** No new Go module and no new vendored
  script.
- **NFR-5 — Human prose** in every notice and refusal (D-6).
- **NFR-6 — Tested as before.** Integration tests against real Postgres and a
  real git repository cover every FR. `go vet ./...` and
  `go test -race -count=1 ./...` are clean, and the integration tests run
  rather than skip.

## 5. Definition of done

1. Every FR's acceptance passes in the suite: save; save and commit (a
   single-file commit with the right author); the conflict when the file
   changed on disk; editing an approved document, which creates a successor;
   the warnings; refusing to break an ID; and reindexing after a save.
2. `go vet ./...` and `go test -race -count=1 -v ./...` are clean, with the
   integration tests run.
3. **A browser walkthrough without an AI provider**, with Playwright and the
   pre-installed Chromium, from `/var/tmp/m9demo`: edit a draft and Save &
   commit, and show the commit; edit the same file in a shell, save in the
   browser, and show the conflict; edit an approved design and see the
   successor. Screenshots and `docs/walkthrough-spec-016.md`.
4. REVIEW-016, by an independent subagent, and this spec revised against it
   (§7).
5. A handoff, `docs/notes/handoff-M9-2026-09-28.md`.
6. The roadmap's §11 marks M9 done with a pointer to the handoff, and nothing
   else there changes.
7. Committed in logical steps to this session's own branch, and pushed.
8. **Fourteen choices need Sam's explicit yes.** Each is the recommendation.
   1. a plain textarea with a live server-rendered preview and a small script
      of our own; no editor library (SD-1);
   2. the editor edits the whole file, front matter included, with the
      identity lines guarded (SD-2);
   3. the commit author is `ui_actor` (as `Name <name@localhost>` unless
      written with an email), and the committer is `subutai` (SD-3);
   4. the commit message: "Edit <ID> in the browser", with the path, state and
      operator in the body, and an optional subject of the person's own
      (SD-3);
   5. Save & commit refuses a file that had uncommitted changes Subutai didn't
      write; Save doesn't (SD-5);
   6. a conflict offers Reload and Copy your text, and no "save mine anyway"
      (SD-7);
   7. saving an edit to a document under review takes it back to draft and
      cancels its review, with no issue recorded (SD-9);
   8. the identity lines can't be removed or changed in the editor, and an ID
      can't be added there; adopt is the way (SD-10);
   9. the editor stands aside while the author agent works on a document, and
      while an Inbox question governs a change of state (SD-11);
   10. an approved decision can't be edited, and an approved document of a
       type with no template can't be revised from the editor until M10's
       verdicts land (SD-12);
   11. every save publishes the watcher's change event, so a plain Save is
       reindexed at once; the heartbeat stays as it is (SD-13);
   12. the work-in-flight warnings are warnings, not refusals (SD-14);
   13. editing an approved document is a separate, explicit step, **Start a
       revision and edit it**, through the existing Revise (FR-4.3);
   14. the document page's "written in your own editor" banner gives way to
       the edit include, which still says the file can be edited anywhere
       (SD-16).

## 6. Open questions carried forward

- **Cross-site requests.** The web UI acts as the operator on any request that
  reaches its listener, with no token. The editor writes files and commits,
  which raises the stakes a little. A same-origin check on every `POST /ui/*`
  would be cheap; it belongs to the whole UI, not this milestone.
- **Untemplated revisions** (SD-12) wait for M10's verdict on any type.
- **Appending an amendment to a decision** from the browser (DESIGN-010 §11)
  would be a small, separate action.
- **Worktrees.** An approved change reaches an agent's worktree only through
  the existing freshness machinery. The editor warns (FR-4.4); it doesn't push
  anything into a worktree.
- **A pushed commit.** Save & commit commits locally. Pushing, and the GitHub
  documents fast lane, are M15.

## 7. Changes after review

*To be completed after REVIEW-016.*
