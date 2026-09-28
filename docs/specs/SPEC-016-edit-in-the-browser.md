# SPEC-016: Edit in the browser

**Status:** **Approved — Sam, 2026-09-28**, with the build, and with the recommendation accepted on each of its eighteen choices (DoD 8). It was drafted for Sam's approval as follows. Authored by Claude. The author
can't be the approval gate, so the decision is Sam's, and Sam has said they
will approve the spec and the build together. An independent review is
recorded in [REVIEW-016](../reviews/REVIEW-016-edit-in-the-browser.md). It
found eleven material and nine smaller problems in the first draft. All are
dealt with in this revision, and §7 says how, finding by finding. Eighteen
choices need Sam's explicit yes (DoD 8).
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
checklist and milestone files. The document page gains one include, with the
button inside it.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every notice, refusal and warning a person reads is a full sentence
(DESIGN-008 D-6). Button labels are short, as elsewhere in the UI.

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
  optimistically: it remembers what it loaded, and a save stops if the file,
  or the document's state or place, has moved on.
- **The lifecycle.** An approved document is never changed in place
  (DESIGN-003 L-2). A document under review is being judged on a particular
  text. A spec or plan may be feeding work that is already running, or be due
  to be rewritten by its author agent. The editor shows the state, says what an
  edit will do, stands aside where an edit would be lost or would answer a
  question by the back door, and routes each case through a path that already
  exists.
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
   original and starts the cascade. They start the revision, edit the
   successor, and when it is approved it takes the original's place.

## 2. Scope

### In scope

1. **An Edit action** on the document page, opening a Markdown editor with a
   live preview (FR-1).
2. **Save & commit** and **Save** (FR-2).
3. **Optimistic locking**, with a readable line diff and a way forward (FR-3).
4. **The lifecycle**: drafts in place, approved documents by successor, and
   warnings or refusals for documents under review, feeding work in flight, or
   due to their author (FR-4).
5. **Identity**: the `id:` and `revision:` lines stay intact (FR-5).
6. **Reindexing** after a save, through the watcher's own path (FR-6).
7. **Four repairs the editor makes necessary** (SD-17 to SD-20): task prompts
   read the approved contract; the approval of a revision commits exactly its
   own files; CRLF files parse; and the editor refuses cross-site posts and
   files outside the repository.

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
| Cross-site protection for the rest of the web UI | A follow-up (§6). SD-20 protects the editor's own posts. |

### Scope decisions

**SD-1 — A plain text box, no library.** The editor is a `<textarea>`, in a
monospaced face, with a live preview beside it on a wide screen and below it on
a narrow one. The preview is rendered by the server with the same goldmark and
bluemonday renderer as the document page, so it looks exactly like the page
will. No editor library is vendored: a textarea is what "pleasant in vim" asks
for, it is accessible as it stands, and a syntax-highlighting editor is a lot of
vendored code for little gain on prose. A small script of our own
(`editor.js`, delegated like `dialog.js`) adds three things a textarea lacks:
Ctrl-S or ⌘-S to save, a copy button, and a question before leaving with
unsaved text. Saving, the preview and the conflict page all work without it.
(Choice 1.)

**SD-2 — The editor edits the whole file, front matter included.** The
front matter holds the title, tags and, since M8, the identity lines. Hiding it
would mean a second editor for metadata, and a person fixing a title expects to
find it in the file. The identity lines are guarded instead (SD-10). The
preview shows the prose only, as the document page does. (Choice 2.)

**SD-3 — Save & commit is the main button.** Roadmap decision 11. It writes
the file and commits **that file only**, whatever else is staged, changed or
untracked in the working tree: `git add` of the one path, then
`git commit --only -- <path>`.

- **The author** is the configured operator, `server.ui_actor`, until per-user
  identity exists (DESIGN-010 §13). If `ui_actor` is already written as
  `Name <email>`, that is used as it stands. Otherwise the author is
  `<ui_actor> <<ui_actor>@localhost>`: angle brackets are dropped from the
  name, and in the email part anything but letters, digits, dots, hyphens and
  underscores becomes a hyphen.
- **The committer** is `subutai <subutai@localhost>`, set for this commit by
  `GIT_COMMITTER_NAME` and `GIT_COMMITTER_EMAIL` in its environment. **This is
  new**: Subutai's other commits pass only `--author`, and their committer is
  whoever git's configuration names. So `git log --format='%an / %cn'` reads
  "operator / subutai", the operator's words committed by the tool, and the
  editor can commit in a repository with no git identity configured. Subutai's
  other commits are left as they are (§6). (Choice 3.)
- **The message** is a subject and a body, each passed with its own `-m`:

  ```
  Edit FEAT-003-spec in the browser

  docs/work/INIT-001-auth/FEAT-003-spec.md, draft, revision 1.
  Saved from the Subutai web editor by operator.
  ```

  A document with no ID is named by its path. The state is the one the
  document has after the save, so a document taken back from review reads
  "draft". The person may write their own subject in an optional one-line box;
  line breaks in it become spaces, and the body is kept either way. (Choice 4.)
- **If the commit fails** after the file was written (a pre-commit hook, a
  merge in progress, commit signing), the editor unstages the path again, so
  nothing is left for another commit to sweep up, and the document page says
  the text was saved but not committed, and why.

**SD-4 — Save writes the file only.** It leaves the commit to the person, for
someone who wants to batch several edits into one commit of their own.
DESIGN-010 §7 and the discussion response both ask for it.

**SD-5 — Save & commit won't sweep up someone else's uncommitted work.** If
the file already had uncommitted changes when the editor opened, Save & commit
would put them in a commit under the operator's name. So it is refused when
the base differs from what `HEAD` holds for the path, or git doesn't track the
file, **and** the base isn't something Subutai wrote itself:

> docs/…/FEAT-003-spec.md has changes that aren't committed, made outside
> this editor. Commit or discard them first, or choose Save, which writes the
> file and leaves committing to you.

"Something Subutai wrote itself" is one of two things, each known by the hash
recorded on the audit trail when it was written:

- **the editor's last save of that document, made over a base that was itself
  clean or Subutai's own.** A save over someone else's uncommitted change
  doesn't launder it: Save followed by Save & commit is still refused
  (R16-12). A save records whether its base qualified.
- **the working copy Revise wrote for a successor**, while no later editor
  save has replaced it. Revise now records the working copy's hash in its
  `document.revision_created` audit row. That is a one-line code change.

So Save on a clean file followed later by Save & commit works, and so does
editing a fresh revision. Save alone is never refused on these grounds: the
person saw the whole file in the editor, and Save commits nothing.

The check reads the file against `HEAD`, not the index: Save & commit commits
the file's content, so staged-but-uncommitted changes to it count as
uncommitted too. (Choice 5.)

**SD-6 — Optimistic locking by content hash, state and path.** The editor form
carries the document's row id, the base's hash (the same SHA-256 the
documents table uses), the state and path the editor showed, and the base
itself (for the diff only). A save:

1. finds the document by its row id and takes the path from the row, never from
   the form;
2. **refuses if the document's state or path differs from what the editor
   showed** (it was submitted, approved, withdrawn or moved while the editor
   was open), with a sentence saying which, the person's text kept, and a way
   to open the editor afresh (R16-3);
3. under a lock that serialises the editor's own saves, reads the file and
   **refuses if its hash differs from the base's hash** (SD-7). The hash posted
   by the form is compared, never a re-hash of the posted base, which the
   browser has re-encoded;
4. refuses if the file isn't there, rather than create one;
5. otherwise writes the text to a temporary file beside the original and
   renames it into place, keeping the file's permission bits, so a reader never
   sees half a file.

The rename replaces the file's inode, as vim's default does; a hard link to the
file would be separated from it. The window in which another editor could write
between the check and the rename is a few microseconds. It is the same window
every editor has, and file locks wouldn't close it, because vim doesn't take
them.

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
  now.

The Save buttons are not offered on this page: a save from it would stop
again, because its base is out of date. There is no "save mine anyway". Keeping
one version over another is a merge, which the person does by copying, reloading
and pasting. The editor also checks the file every ten seconds while it is open,
and says at once if it has changed, so the conflict is usually seen before the
save rather than after. (Choice 6.)

**Leaving with unsaved text.** Every page in the UI is boosted by htmx, so
following a link doesn't unload the page and the browser's own "leave this
page?" never fires. `editor.js` therefore asks on any link click, and on any
other boosted request, while the text has unsaved changes; the browser's own
question still covers closing the tab or reloading. Without the script, links
simply leave (R16-8).

**The diff** is a plain line diff, computed on the server with a small longest
common subsequence routine in a new package, `internal/textdiff` (no
library), and shown in unified form with three lines of context: removed lines
struck through and red, added lines green, both marked `-` and `+` for readers
who can't tell the colours apart. Lines are compared without their carriage
returns, so the browser's CRLF doesn't make every line differ. Files too large
for it (more than four million pairs of differing lines) show both versions
whole instead.

**SD-8 — The editor always shows the document's state, and each state has one
rule.**

| State | What Edit does |
|---|---|
| `draft` | Edits the file in place. |
| `reviewing` | Warns first, then edits in place; **saving takes the document back to draft and cancels its review** (SD-9). |
| `approved` | Doesn't edit it. Offers to **start a revision**: a successor draft through the existing Revise path, which the editor then opens (FR-4.3). If a revision is already open, Edit opens that. |
| `superseded` | No Edit button. A superseded document is a record. |

This is DESIGN-010 §7's list, turned into rules. SD-11 and SD-12 add the
cases where the editor stands aside.

**SD-9 — Editing a document under review takes it back to draft.** A review is
a judgement of a particular text. Today, when a reviewing document changes on
disk, the reviewer's verdict is dropped as stale (SPEC-011 FR-3.5) and the
document is left in `reviewing` with no verdict coming: the spec gets stuck.
The editor does better than vim here, because it knows. Saving a changed text
for a reviewing document therefore, first and in one transaction:

- cancels any queued review of it and clears any hold, as a person's
  send-back does;
- moves it back to `draft` by the lifecycle's existing `withdraw` event
  (`reviewing → draft`), audited with the cause "edited in the browser";

and then writes the file. A review already running finishes, and its verdict
is dropped as stale, as today. The person submits again when ready. No issue
is recorded: an edit is not an objection to be answered.

**Whose withdrawal this is.** DESIGN-003 §2 gives `withdraw` to a document's
author. Here the operator withdraws a document an agent may have written. That
extends the authority to whoever edits the document in the browser, and it is
stated here so it is a decision, not an accident (R16-17). The document page
already shows the state as `draft` with Submit offered, and the save's notice
says "It went back to draft, so submit it again when you're done."

**When nothing is dispatched.** A withdrawn spec or plan on a sent feature is a
draft that belongs to whoever is writing it (SPEC-011 FR-2.2), so nothing is
dispatched, **provided it carries no open issue or major finding**. One that
does would wait for its author, and the author would be sent back to it; the
editor refuses that case before any change (SD-11). (Choice 7.)

**SD-10 — The identity lines are kept intact.** A document with an ID can only
be saved if the text's front matter still reads `id: <its ID>` and
`revision: <its revision>`. Otherwise the save is refused:

> This document's ID is FEAT-003-spec, at revision 1, and the text you saved
> doesn't say so. Put back the lines "id: FEAT-003-spec" and "revision: 1" in
> its front matter and save again. They are how Subutai recognises this file
> wherever it moves.

Nothing is detached silently, and nothing is written. The lines may move within
the front matter; only their values are checked.

A document with **no** ID can't gain one in the editor: adding an `id:` line to
a file that had none is refused, pointing to *Give it an ID* on the document
page, which adopts it properly (SPEC-015 FR-5):

> This document has no ID yet, and an ID can't be added by editing it. Take
> the id: line out and save, then use Give it an ID on the document's page,
> which registers the ID properly.

Correcting or removing an `id:` line such a file already carries is allowed,
because that is what SPEC-015's adopt refusal tells the person to do with a
foreign `id:` (SPEC-015 SD-11, choice 14; R16-10). (Choice 8.)

**SD-11 — The editor stands aside where an edit would be lost, or would answer
a question by the back door.** It refuses to open, and to save, with a
sentence, when:

- **the author agent is at work on the document**: a spec or plan whose
  authoring dispatch is queued or running (`refuseIfAuthorAtWork`, the check
  Submit makes). The agent writes the whole document, so the edit would be
  lost;
- **the author agent is due to revise it**: a sent feature's spec or plan
  carrying an open human issue or an unresolved major finding (it "waits for
  its author", SPEC-011 FR-2.1). The heartbeat sends the author back to any
  such draft that has changed since its last attempt, and a save changes it, so
  the author would rewrite the person's edit within a heartbeat (R16-4). The
  exception is while an `authoring-deadlock` question about the document is
  pending: then the author isn't sent, and the person may take the draft over.
  For an unsent feature no author will come, so the draft is editable, with a
  warning (FR-4.4);
- **a question in the Inbox governs a change of state the edit would make**:
  - a reviewing document with a `review-escalation` or `authoring-deadlock`
    question about it, or whose feature has a design-revision question
    pending (`refuseIfPending`, which Approve, Send back and the issue acts
    already make, SPEC-011 SD-11). Saving would withdraw it from review;
  - a reviewing **revised** spec or plan whose feature has a
    `revision-in-flight` question pending. Its submission paused new tasks;
    withdrawing it would leave that question about a submission that no
    longer exists (R16-6);
  - starting a revision of an approved document under the same pending
    questions as the first case.

A draft under an `authoring-deadlock` question may still be edited: editing
the text answers nothing. (Choice 9.)

**SD-12 — Some approved documents can't be revised from the editor, which
departs from DESIGN-010 §7.** DESIGN-010 §7 says "editing an approved document
creates a successor draft". Two kinds are refused instead, and this is a
deliberate, named departure (R16-18):

- **An accepted decision is never edited** (DESIGN-010 §11, which is more
  specific):

  > An accepted decision is never edited. If the project changes its mind, a
  > new decision supersedes this one, or a dated amendment is appended to it.

- **An approved document of a type with no template** (research, report, note,
  policy) makes a successor that can't be submitted, because Submit needs a
  template (the M8 handoff's follow-up 3). Starting one would strand it:

  > A revision of an approved note can't be submitted for review yet, because
  > documents of this type have no template, so it can't be started here.
  > Until a person can rule on any type of document, change it by writing a
  > new note.

  M10 lets a person rule on any document type. Once it has merged, this
  refusal can go.

The document page's **Revise** button gains the same refusals, so the page and
the editor agree (R16-14). (Choice 10.)

**SD-13 — Reindexing runs through the watcher's own path.** The brief asks to
check that the existing post-commit and heartbeat paths reindex an edited
document. **The post-commit hook does; the heartbeat doesn't.**

- The post-commit hook publishes `document.file_changed` for every registered
  path a commit touches, and the rule engine reindexes a draft or reviewing
  document. The boot catch-up scan does too.
- The heartbeat doesn't rehash documents at all. It follows moved documents by
  their IDs, but a file changed and not committed is reindexed only at the
  next commit or restart. So after Save alone the index would be stale.

The editor therefore publishes the watcher's event itself, after every save,
with the commit hash when it made one. The rule engine then reindexes exactly
as for a committed vim edit, and the page, search and prompts read the new
sections. For Save & commit, the post-commit hook also fires; it causes at
most one more, harmless, reindex. Nothing is added to the heartbeat.
(Choice 11.)

Two gaps in that path, which a vim edit shares, are closed (R16-7):

- **CRLF and a byte-order mark.** The section parser accepted only LF front
  matter, so a CRLF file was never reindexed (the M8 handoff's follow-up 6).
  `content.Parse` now reads a file with CRLF line endings or a byte-order mark
  like any other. The document page's own reading of front matter is left as
  it is, because it is shared with M10's work; a CRLF file's page still shows
  its front matter as text (§6).
- **A file that can't be parsed** still has its new hash recorded, sections
  unchanged, so the watcher stops treating it as changed.

**SD-14 — Warnings for documents that feed work in flight.** The editor shows
a warning, in a sentence that says what will happen, when a document belongs
to a feature that has been **sent to development** (the mark, or being built:
SPEC-011's `featureSent`), or is a **design** whose revision would start the
cascade over sent features' specifications. The cases are listed in FR-4.4.
They are warnings, not refusals: once SD-11's refusals and SD-17's repair are
in place, the paths they describe exist and are safe. (Choice 12.)

**SD-15 — Routes.** The document page is `/ui/d/<path>`, a wildcard, so the
editor has its own:

| Route | What it does |
|---|---|
| `GET /ui/edit/<path>` | The editor for the live document at that path. A moved document's old path redirects, as the document page does. |
| `POST /ui/edit/save` | Save or Save & commit, by the button pressed. |
| `POST /ui/edit/preview` | The rendered preview of the text, as a fragment. |
| `POST /ui/edit/revise` | Starts a revision of an approved document and redirects to the successor's editor. |
| `GET /ui/frag/edit-fresh` | Whether the file still matches the base, as a fragment. |

These act as the configured operator, as the rest of the web UI does.

**SD-16 — The document page gains one include.** The banner "This document is
written in your own editor" is replaced by an include, `doc-edit`, in a new
template file, `edit.html`. It says the document can be edited here or in the
person's own editor, and holds the Edit button, labelled by state: "Edit",
"Edit — starts a revision", "Edit the open revision", or none, with a
sentence, for a superseded document. That is the only change to the document
page, so M10's "written by" line merges cleanly beside it. (Choice 14.)

**SD-17 — Task prompts read the approved contract** (R16-5). The prompts for
implementing, reviewing and verifying a task take the feature's spec and plan
from `contractBodies`, whose comment says "current approved spec". It read the
newest *live* spec, which, once a revision is open, is the draft successor. So
every save to a revision reached the next task's prompt before review. That
predates M9, but the editor makes it casual. `contractBodies` now reads the
newest approved spec and plan, falling back to the newest live one only when
none is approved. Then FR-4.4's "being built" warning is true. (Choice 15.)

**SD-18 — Approving a revision commits exactly its own files** (R16-1). When
a revision is approved, Subutai moves the original to the archive and the
working copy to the original's path, and commits. That step used `git mv` on
the working copy, which fails if the working copy was never committed, and a
`git commit` with no paths, which committed whatever was staged: the index's
older text rather than the saved one, and anything else a person had staged.
Now it stages the working copy first, then moves both files, and commits only
the paths it moved, with `--only`. So a revision edited with Save alone, and
never committed, is approved cleanly with its saved text. The cascade's own
archive commit (`archiveInvalidatedFiles`) has the same shape and is left for a
follow-up (§6). (Choice 17.)

**SD-19 — Revise records the working copy's hash.** `document.revision_created`
audit rows gain a `hash` field (SD-5). Nothing else reads it.

**SD-20 — The editor refuses cross-site posts and files outside the
repository** (R16-9). Until now a forged `POST /ui/*` could at worst trigger a
lifecycle act. The editor writes and commits files, and the lock is no defence,
because a document's hash in a shared repository can be computed. So:

- `POST /ui/edit/save` and `POST /ui/edit/revise` are refused with `403` when
  the browser says the request is cross-site: a `Sec-Fetch-Site` other than
  `same-origin` or `none`, or an `Origin` whose host isn't the request's. A
  request with neither header, which no current browser sends with a form post,
  is let through; a script on the machine could write the file directly
  anyway.
- The document is found by row id and its path comes from the row (SD-6).
- The file written must be a regular file, not a symbolic link, in a directory
  that resolves (through any symbolic links) to somewhere inside the
  repository. Otherwise:

  > The file for this document, docs/…, isn't a plain file inside the
  > repository, so the editor won't write it.

The rest of the web UI keeps its current exposure; §6 proposes the same check
for every `POST /ui/*`. (Choice 18.)

## 3. Requirements

### FR-1: The editor

**FR-1.1** `GET /ui/edit/<path>` shows the editor for the live document at
`<path>`. The page shows the document's ID and title, its state badge, its
type and revision, its owner and its path, like the document page's head, and
a sentence stating its state (SD-8), followed by any warnings (FR-4).

**FR-1.2** The text box holds the whole file (SD-2), monospaced, sized to the
window, with spell-checking off and no automatic capitalisation. The template
emits a line break straight after `<textarea>`, because HTML drops one there,
so a file that starts with a blank line keeps it. Text travels as follows:

- A file that starts with a byte-order mark is shown without it, and saved with
  it.
- The browser sends line breaks as CRLF. The text is saved with LF, unless the
  file used CRLF anywhere, in which case every line is saved with CRLF. A file
  with mixed endings therefore comes back with one kind.
- "Nothing changed" compares the file bytes after the line endings and the
  byte-order mark are restored.

**FR-1.3** The preview renders the text's prose (front matter stripped) with
the document page's renderer, after normalising CRLF to LF and dropping a
byte-order mark, as the person types (after a short pause). The page opens
with the preview already rendered. It is labelled as a preview.

**FR-1.4** The buttons are **Save & commit** (primary), **Save**, and
**Cancel** (back to the document page). An optional one-line box, "Commit
message", is used by Save & commit (SD-3). Ctrl-S or ⌘-S presses Save.

**FR-1.5** While the editor is open, it checks every ten seconds whether the
file still matches the base, and shows a warning with a reload link if not
(SD-7).

**FR-1.6** A document whose file can't be read is refused: "The file for this
document can't be read from the working tree, so there's nothing to edit.
Restore it in git first."

**Acceptance:**
- the editor opens on a draft with its state shown and the whole file in the
  text box;
- the preview fragment renders a CRLF text's prose, without the front matter,
  and without a script it contained;
- an LF file stays LF after a save from a browser's CRLF;
- a CRLF file with a byte-order mark keeps both after a save.

### FR-2: Save and Save & commit

**FR-2.1 — Save** checks, in order:

1. the state and path the editor showed (SD-6);
2. the lifecycle (FR-4, SD-11, SD-12);
3. that the file is a plain file inside the repository (SD-20);
4. the lock (FR-3);
5. identity (FR-5).

Then it takes a reviewing document back to draft (SD-9), writes the file
(SD-6), audits `document.edited` with the path, the base hash, the new hash,
whether it wrote, whether its base was clean or Subutai's own, whether it
withdrew the document, and the commit, and publishes the change (FR-6). It
re-renders the document page with "Saved. The file is written; it isn't
committed yet."

**FR-2.2 — Save & commit** does the same and then commits the file alone
(SD-3), after the check in SD-5. It re-renders the document page with "Saved
and committed as <short hash>." If the text is identical to the file and the
file matches `HEAD`, nothing is written or committed, and the page says
"Nothing changed, so nothing was saved." If the text is identical but the file
holds Subutai's own uncommitted save, that is committed: "Committed as <short
hash>. The file already held your text." If the commit fails, SD-3 says what
happens.

**FR-2.3** A save that is refused re-renders the editor with the person's text
still in it, and the refusal as a sentence above it. Nothing is written.

**FR-2.4** Like every other act in the UI, a save re-renders its result rather
than redirecting. A browser refresh that re-posts the form is harmless: its
base hash no longer matches the file, so it stops as a conflict and writes
nothing.

**Acceptance:**
- Save writes the file, commits nothing, and the document page shows the new
  text;
- Save & commit makes one commit that touches only that file, with other
  staged and unstaged changes in the tree left as they were, the operator as
  author and `subutai` as committer, and the message of SD-3;
- a subject typed in the box becomes the commit's subject;
- Save & commit works in a repository with no git identity configured;
- a refused commit leaves the file saved, nothing committed, and nothing
  staged;
- Save & commit is refused on a file that had uncommitted changes made
  outside the editor, on an untracked file, after the editor's Save over
  such changes, and after a vim edit that followed the editor's own save; it
  is allowed after the editor's own Save over a clean file and on a fresh
  revision's working copy.

### FR-3: Optimistic locking

**FR-3.1** A save whose base hash doesn't match the file's current hash is
refused (SD-6), and the editor re-renders as SD-7 says: the sentence, both
diffs, the person's text, Copy your text, and Reload from disk.

**FR-3.2** Two saves from the browser at once are serialised; the second sees
the first's write as a change on disk.

**FR-3.3** The freshness fragment says nothing while the file matches, and
"This file has changed on disk since you opened it. Saving will stop and show
you the difference." with a reload link when it doesn't.

**FR-3.4** A save whose document changed state or moved since the editor
opened is refused with a sentence naming the change, the text kept, and a way
to open the editor again (SD-6).

**Acceptance:**
- a file changed in a shell after the editor opened is not overwritten by Save
  or Save & commit;
- the refusal shows the shell's change and the person's change;
- the file on disk is exactly the shell's version, and nothing is committed or
  audited as an edit;
- the freshness fragment reports the change;
- a document submitted while its editor was open isn't withdrawn by the save;
- a document moved while its editor was open isn't written at its old path.

### FR-4: The lifecycle

**FR-4.1 — Drafts** are edited in place (SD-8), unless SD-11 says the editor
stands aside.

**FR-4.2 — A document under review** opens with a warning:

> This document is under review. Saving a change takes it back to draft and
> cancels its review, because the review was of the text as submitted. Submit
> it again when you're done.

For an agent-reviewed type the warning adds: "A review already running will
finish, and its verdict will be set aside." Saving does what SD-9 says. It is
refused as SD-11 says.

**FR-4.3 — An approved document** isn't edited. `GET /ui/edit/<path>` shows a
page that says:

> This document is approved, so it isn't changed in place. Editing it starts a
> revision: a successor draft with the same ID at revision <n+1>, in a working
> copy at <path>. The original stays approved, and in force, until the
> revision is approved; then the revision takes its place and the original is
> archived.

For a **design** it adds what the cascade actually does (R16-15):

> Approving a revised design starts the cascade. Subutai finds the approved
> specifications written from this design. If there is one, it is superseded,
> or, for a feature being built, rewritten as a revision; if there are
> several, one question in the Inbox asks, spec by spec, whether to keep or
> redo each. A feature not yet sent to development is left without a
> specification until it is sent.

For a **spec** of a feature in `idea` or `ready` it adds:

> Approving a revised specification also supersedes this feature's approved
> plan, so the plan is written again, and a feature that was ready to build
> goes back to waiting for it.

Any FR-4.4 warnings follow. The page has one button, **Start a revision and
edit it**, which posts to `/ui/edit/revise`. That refuses as SD-11 and SD-12
say, otherwise runs the same `ReviseDoc` as the document page's Revise button
(SPEC-011 FR-9.2), and redirects to the successor's editor with the notice "A
revision was opened. The original stays approved until this is." If a
revision is already open, `GET` redirects to its editor instead. When the
revision is approved, SD-18 moves it into place.

**FR-4.4 — Warnings.** Each is a sentence:

| When | The warning |
|---|---|
| A spec or plan of a feature that is sent, not yet being built | "FEAT-003 has been sent to development, and this specification is part of its contract. The next steps are written from the approved specification, so your change reaches them once it is approved." |
| A spec or plan of a feature being built (`active` or `review`) | "FEAT-003 is being built. Its tasks are given the approved specification, so a change reaches them only once a revision is approved. Submitting a revised specification stops new tasks starting until you answer a question in the Inbox." |
| An approved design, or a revised design, that a sent or building feature builds from (its own, or its immediate parent initiative's) | "FEAT-003 builds from this design and has been sent to development, so approving a revision of it starts the cascade over their specifications." |
| A draft spec or plan with an open issue or major finding, on a feature not sent | "This draft has open review findings. Its feature hasn't been sent to development, so no author agent will revise it: address them yourself, and submit it when you're done." |
| The same, on a sent feature, while an `authoring-deadlock` question is pending | "This draft still has review findings, and the Inbox asks whether to give its author agent another round. If you address them yourself, submit it when you're done, and answer the question so the author isn't sent back to it." |

A document may show more than one. A document none of these applies to shows
none. (The sent case without a pending question is refused, SD-11.)

**Acceptance:**
- a reviewing document's editor shows the warning, and a save takes it to
  `draft`, audited as a `withdraw`;
- an approved design's editor shows the revision and cascade text, and
  starting the revision creates a successor draft with the same ID at the
  next revision, whose editor opens;
- a revision edited with Save alone, then approved, takes the original's path
  with the saved text, in a commit of exactly the two moved paths, leaving an
  unrelated staged file staged, and raising no integrity question;
- Edit on an approved document with an open revision goes to that revision;
- the sent, building and design warnings show when they should, and not before
  the feature is sent;
- the unsent and deadlock warnings show, and a sent feature's waiting draft is
  refused;
- the editor refuses while the author agent's dispatch is live, on a
  reviewing document with a review-escalation question, on a revised spec
  whose feature has a revision-in-flight question, on an approved decision and
  an approved note, and on a superseded document.

### FR-5: Identity

**FR-5.1** A save of a document with an ID is refused unless the text carries
the same `id:` and `revision:` (SD-10). The refusal names both lines.

**FR-5.2** A save of a document with no ID is refused if the text declares an
`id:` its file didn't. A declared `id:` may be corrected or removed.

**FR-5.3** A refused save writes nothing and changes no state.

**Acceptance:** deleting the `id:` line, changing it, changing the revision,
and deleting both are each refused with the sentence, the file and the row
unchanged; reordering them is allowed; adding an `id:` to a document with none
is refused; removing a declared foreign `id:` is allowed; a save that keeps
them succeeds.

### FR-6: Reindexing

**FR-6.1** After a save, the editor publishes `document.file_changed` for the
document's path, with the commit hash when it made one (SD-13). The rule engine
reindexes it: its sections, title and content hash follow the file.

**FR-6.2** A file with CRLF line endings or a byte-order mark is reindexed like
any other; a file that can't be parsed has its hash recorded (SD-13).

**Acceptance:** after Save alone, with no commit, the document's sections
reflect the new text and its content hash is the new file's hash; the same for
a CRLF file with a byte-order mark; the approved document behind an open
revision raises no integrity question.

### FR-7: The document page

**FR-7.1** The document page's include (SD-16) offers Edit by state, and for a
superseded document says why not: "A superseded document is a record, so it
isn't edited."

**Acceptance:** a draft's page links to its editor; an approved document's page
says "Edit — starts a revision", and "Edit the open revision" once one is open.

### FR-8: Task prompts

**FR-8.1** `contractBodies` reads the newest approved spec and plan, falling
back to the newest live one when none is approved (SD-17).

**Acceptance:** after a revision of an approved spec is opened and its working
copy changed, the spec a task prompt would carry is still the approved one.

## 4. Non-functional requirements

- **NFR-1 — Nothing existing breaks.** The suite passes unchanged except where
  a test asserted the old banner's words.
- **NFR-2 — Minimal writes.** A save writes the person's text and nothing
  else; Subutai adds no lines. A save that changes nothing writes nothing.
- **NFR-3 — Coordination.** No migration. New code in `edit.go`,
  `ui_edit.go`, `internal/textdiff`, `edit.html` and `editor.js`.
  `entity.html` changes by one include; `partials.html` loads `editor.js`;
  `ui.go` gains the routes; small, marked repairs in `actions.go`
  (takeover, reindex), `planner.go` (`contractBodies`), `ui_send.go`
  (Revise's refusals), `content/parse.go` (CRLF) and `store/documents.go`
  (the revision hash, `RecordContentHash`). None of the files M10 owns.
- **NFR-4 — No external libraries.** No new Go module and no new vendored
  script.
- **NFR-5 — Human prose** in every notice, warning and refusal (D-6).
- **NFR-6 — Tested as before.** Integration tests against real Postgres and a
  real git repository cover every FR. `go vet ./...` and
  `go test -race -count=1 ./...` are clean, and the integration tests run
  rather than skip.

## 5. Definition of done

1. Every FR's acceptance passes in the suite: save; save and commit (a
   single-file commit with the right author); the conflict when the file
   changed on disk; editing an approved document, which creates a successor,
   through to its approval; the warnings and refusals; refusing to break an
   ID; and reindexing after a save.
2. `go vet ./...` and `go test -race -count=1 -v ./...` are clean, with the
   integration tests run.
3. **A browser walkthrough without an AI provider**, with Playwright and the
   pre-installed Chromium, from `/var/tmp/m9demo`: edit a draft and Save &
   commit, and show the commit; edit the same file in a shell, save in the
   browser, and show the conflict; edit an approved design and see the
   successor; and type, then follow a link, and see the leave question.
   Screenshots and `docs/walkthrough-spec-016.md`.
4. REVIEW-016, by an independent subagent, and this spec revised against it
   (§7).
5. A handoff, `docs/notes/handoff-M9-2026-09-28.md`.
6. The roadmap's §11 marks M9 done with a pointer to the handoff, and nothing
   else there changes.
7. Committed in logical steps to this session's own branch, and pushed.
8. **Eighteen choices need Sam's explicit yes.** Each is the recommendation.
   The first fourteen were in the first draft; the review asked for the last
   four to be listed too.
   1. a plain textarea with a live server-rendered preview and a small script
      of our own; no editor library (SD-1);
   2. the editor edits the whole file, front matter included, with the
      identity lines guarded (SD-2);
   3. the commit author is `ui_actor` (as `Name <name@localhost>` unless
      written with an email), and the committer is `subutai`, set for the
      editor's commits only (SD-3);
   4. the commit message: "Edit <ID> in the browser", with the path, state and
      operator in the body, and an optional subject of the person's own
      (SD-3);
   5. Save & commit refuses a file that had uncommitted changes Subutai didn't
      write, including after the editor's Save over them; Save doesn't (SD-5);
   6. a conflict offers Reload and Copy your text, and no "save mine anyway"
      (SD-7);
   7. saving an edit to a document under review takes it back to draft and
      cancels its review, with no issue recorded, and the person who edits
      may withdraw a document an agent wrote (SD-9);
   8. the identity lines can't be removed or changed in the editor, and an ID
      can't be added there; a foreign `id:` may be corrected (SD-10);
   9. the editor stands aside while the author agent works on a document or
      is due to revise it, and while an Inbox question governs a change of
      state (SD-11);
   10. an approved decision can't be edited, and an approved document of a
       type with no template can't be revised, from the editor or the Revise
       button, until M10's verdicts land (SD-12);
   11. every save publishes the watcher's change event, so a plain Save is
       reindexed at once; the heartbeat stays as it is (SD-13);
   12. the work-in-flight warnings are warnings, not refusals (SD-14);
   13. editing an approved document is a separate, explicit step, **Start a
       revision and edit it**, through the existing Revise (FR-4.3);
   14. the document page's "written in your own editor" banner gives way to
       the edit include, which still says the file can be edited anywhere
       (SD-16);
   15. task prompts read the approved spec and plan, not an open revision's
       draft (SD-17);
   16. a sent feature's draft that waits for its author is refused, unless
       the Inbox asks about another round (SD-11);
   17. approving a revision stages its working copy and commits only the
       paths it moved (SD-18);
   18. the editor's posts refuse cross-site requests, and the editor writes
       only plain files inside the repository (SD-20).

## 6. Open questions carried forward

- **Cross-site requests for the whole UI.** SD-20 protects the editor. The
  same check on every `POST /ui/*` would be a few lines, and would stop a
  forged page triggering any act as the operator.
- **Subutai's other commits** pass only `--author`, so they fail in a
  repository with no git identity, and their committer is whoever git's
  configuration names. Setting the committer as the editor does would be a
  small change.
- **The cascade's archive commit** (`archiveInvalidatedFiles`) commits
  whatever is staged, as the takeover did before SD-18. The same fix applies.
- **A CRLF file's document page** still shows its front matter as text, because
  `config.SplitFrontMatter` accepts only LF and the page is shared with M10.
  Parsing and reindexing are fixed (SD-13).
- **Untemplated revisions** (SD-12) wait for M10's verdict on any type.
- **Appending an amendment to a decision** from the browser (DESIGN-010 §11)
  would be a small, separate action.
- **A withdrawn document on a sent feature** waits for the person to resubmit
  it. The document page shows it as a draft with Submit offered, but nothing
  on the feature page says the feature is waiting for a person. A line there
  would help (R16-17).
- **Worktrees.** An approved change reaches an agent's worktree only through
  the existing freshness machinery. The editor warns (FR-4.4); it doesn't push
  anything into a worktree.
- **A pushed commit.** Save & commit commits locally. Pushing, and the GitHub
  documents fast lane, are M15.
- **The browser's back button** after typing in the editor restores the
  previous page from htmx's history without asking. The typed text is lost.
  The leave question covers links, forms, reloads and closing the tab.

## 7. Changes after review

[REVIEW-016](../reviews/REVIEW-016-edit-in-the-browser.md) checked the first
draft (`8a3f74b`) against the approved documents and the code. Every finding
is dealt with:

| Finding | What changed |
|---|---|
| R16-1 (material) | SD-19 records the working copy's hash, so SD-5's exemption for a fresh revision works. SD-18 makes the approval takeover stage the working copy and commit only its moved paths, so a revision saved but never committed is approved with its saved text, and nothing else staged is swept in. FR-4.4 tests it end to end. `archiveInvalidatedFiles` is a follow-up (§6). |
| R16-2 (material) | SD-3 states the mechanism (`GIT_COMMITTER_*` in the commit's environment), says it is new and for the editor's commits only, and FR-2.2 tests a repository with no git identity. |
| R16-3 (material) | SD-6: the form carries the row id and the state and path shown; a save refuses a changed state or path (FR-3.4), takes the path from the row, and never creates a file. Tested for "submitted while open" and "moved while open"; "approved while open" is the same check. |
| R16-4 (material) | SD-11 refuses a sent feature's draft that waits for its author, except under an `authoring-deadlock` question; SD-9 says the withdrawal case is covered by that refusal; FR-4.4's fourth row is split into unsent and deadlock, both true. Choice 16. |
| R16-5 (material) | SD-17 and FR-8: `contractBodies` reads the approved spec and plan, with a test; the "being built" warning is rewritten to match. Choice 15. |
| R16-6 (material) | SD-11 refuses to withdraw a revised spec or plan whose feature has a `revision-in-flight` question pending, with a test. |
| R16-7 (material) | FR-1.3 normalises the preview. SD-13: `content.Parse` reads CRLF and a byte-order mark, and an unparseable file's hash is recorded. The document page's front-matter reading is left to a follow-up (§6), and FR-6's acceptance tests a CRLF file with a byte-order mark. |
| R16-8 (material) | SD-7: `editor.js` asks on link clicks and other boosted requests while the text is unsaved; DoD 3's walkthrough shows it. The back button remains (§6). |
| R16-9 (material) | SD-20: a same-origin check on the editor's posts, the path from the row, and a plain-file-inside-the-repository check; tested with a cross-site post and a symlink. The rest of the UI is a follow-up (§6). |
| R16-10 (material) | SD-10 and FR-5.2: only *adding* an `id:` is refused for a document with no ID; correcting or removing a declared one is allowed, with a test. |
| R16-11 (material) | The acceptance lists now cover each item: the revision end to end, every warning row, the pending-question and revision-in-flight refusals, SD-5 on an untracked file and after a vim edit, the byte-order mark, other staged files, a failed commit, a repository with no identity, state changes while open, cross-site posts and symlinks. The running review's dropped verdict is SPEC-011 FR-3.5's existing behaviour, already tested there; the editor's part, cancelling a queued review, reuses `cancelQueuedReviews`. |
| R16-12 (minor) | SD-5: an editor save counts as Subutai's own only if its base was clean or Subutai's; tested. |
| R16-13 (minor) | SD-13 says "at most one more, harmless, reindex". |
| R16-14 (minor) | SD-11 names which acts make which checks. The document page's Revise gains SD-12's refusals and the pending-question check (SD-12). |
| R16-15 (minor) | FR-4.3's cascade sentence matches the code; the design warning shows only for approved and revised designs; an approved spec's page says its plan will be superseded. |
| R16-16 (minor) | FR-1.2 pins line endings, the byte-order mark, the leading newline and "nothing changed"; SD-6 says the posted hash is compared; SD-7 says diffs ignore carriage returns; SD-3 strips line breaks from the subject; FR-2.4 explains why a re-post is safe rather than redirecting, which keeps to the UI's existing pattern. |
| R16-17 (minor) | SD-9 states the extension of the withdraw authority. The line on the feature page is a follow-up (§6), because the feature page is shared with M10. |
| R16-18 (minor) | SD-12 names the departure from DESIGN-010 §7 and gives both sentences. |
| R16-19 (minor) | SD-3: a failed commit unstages the path and says why; SD-6 notes the inode. A gitignored path fails `git add`, which is reported the same way. |
| R16-20 (minor) | "They don't both" is gone; the prose note covers notices, warnings and refusals, not button labels; FR-2 explains the re-render. |
