# REVIEW-016: Consistency Review of SPEC-016 (Edit in the browser)

**Status:** Complete. The author dealt with every finding (§6). Sam approved
SPEC-016 and the build on 2026-09-28, with all eighteen choices.
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author. Approval is
Sam's.
**Scope:** [SPEC-016](../specs/SPEC-016-edit-in-the-browser.md) (first draft,
commit `8a3f74b`), against
[DESIGN-010](../design/DESIGN-010-subutai.md) §4, §5, §7, §11, §13 and §14;
[DESIGN-003](../design/DESIGN-003-document-lifecycle-and-gates.md) §2 and L-2;
[the discussion response](../notes/subutai-discussion-response-2026-07-31.md)
§6; [SPEC-011](../specs/SPEC-011-send-to-development.md) (FR-2, FR-3.5, FR-3.6,
FR-9, SD-11); [SPEC-015](../specs/SPEC-015-documents-with-identity.md) (SD-8,
SD-10, SD-11) and the [M8 handoff](../notes/handoff-M8-2026-09-28.md); the
[roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11 (M9) and §12
decision 11; and the code at `8a3f74b`: `internal/server/documents.go`,
`server.go`, `actions.go`, `actions_phase2.go`, `review_send.go`,
`ui_send.go`, `authoring.go`, `identity.go`, `planner.go`, `ui.go`,
`ui_entity.go`; `internal/rules/rules.go`; `internal/content/identity.go`,
`parse.go`; `internal/config/compartment.go`; `internal/lifecycle/document.go`;
`internal/store/documents.go`, `dispatches.go`, `checkpoints.go`,
`migrations/0001_init.sql`; `internal/server/ui/templates/entity.html`,
`partials.html`.

## 1. What this review is

A consistency pass by a reviewer who did not write the spec. It checks the spec
against the approved documents, checks each claim it makes about the code, and
looks for lifecycle holes, silent overwrites, identity holes, git hazards and
untested requirements. It does not fix the spec.

**Timing.** `internal/server/edit.go` and `ui_edit.go` appeared in the working
tree while this review ran. Findings are about the spec as committed.

The spec is well organised, and most of its claims about the code are true
(§4). The material problems are in three places: the revision flow, which
breaks at approval; what happens when state changes under an open editor; and
several warnings that don't say what will actually happen.

## 2. Material findings (must fix before approval)

### R16-1 (material) — A revision edited in the browser can't be approved, and Save & commit is refused on it

**What's wrong.** `ReviseDoc` writes the working copy `<name>.rev.md` and never
commits it (`internal/server/documents.go:243-246`), so git doesn't track it.
Approving the successor runs `git mv <successor> <canonical>`
(`actions.go:421-444`), which fails on an untracked file. The approval is then
already committed in the database, and a `document-integrity` question is
raised (`actions.go:390-405`). So the spec's headline flow (Goal 3, DoD 3)
breaks unless the working copy is committed first. The spec provides for that
only through SD-5's exemption, which rests on "the hash recorded on the audit
trail" when Revise wrote the copy. No such hash exists: `RegisterDocument`
audits `document.revision_created` with `path`, `type` and `supersedes` only
(`internal/store/documents.go:73-80`). As written, Save & commit on a fresh
revision is therefore refused, and Save alone leaves the file untracked.

Two related hazards follow:

- Even when the successor is tracked but was edited with Save alone, the
  takeover's `git commit` has no pathspec. It commits the index, which holds the
  older text, not the approved working-tree text. It also sweeps in anything
  else that is staged, under `subutai`'s name.
- The same applies to `archiveInvalidatedFiles` (`authoring.go:656`).

**Fix.**

- Record the working copy's hash when Revise writes it, by adding it to the
  `document.revision_created` payload. That is a code change, so say so.
- Make the editor commit a successor's working copy on its first save, or make
  Submit of a successor refuse an untracked or dirty working copy with a
  sentence.
- Better still, make the takeover `git add` the successor and commit exactly
  the two paths (`--only`).
- Add an acceptance test: start a revision in the editor, Save, submit,
  approve. The takeover must succeed, and the takeover commit must hold only
  the two paths.

### R16-2 (material) — "The committer is `subutai`, as for every other commit Subutai makes" is false

**What's wrong.** SD-3 claims that the committer is `subutai` and that a
project with no git identity can still commit. Every Subutai commit passes only
`--author` (`authoring.go:851`, `actions.go:430`, `authoring.go:656`).
Nothing sets `GIT_COMMITTER_NAME` or `GIT_COMMITTER_EMAIL`, and nothing passes
`-c user.name`. A search of `internal/` and `cmd/` finds none. So today the
committer is whoever git config names. In a repository with no identity
configured, `git commit` fails with "Committer identity unknown". An
implementer copying `commitPath` would fail FR-2.2's committer acceptance, or
pass it only because the test repository happens to be configured.

**Fix.** State the mechanism: run the editor's commit with
`GIT_COMMITTER_NAME=subutai` and `GIT_COMMITTER_EMAIL=subutai@localhost` in its
environment. Say that this is new. Say whether Subutai's other commits should
follow; if they should, that is a separate small change. Test it in a
repository with no `user.name` or `user.email`, and check `%cn`.

### R16-3 (material) — The lock is on the file's content, not the document's state or path, so a save can land on a document that changed state

**What's wrong.** A save compares the base hash with the file (SD-6). Several
changes of state leave the content alone:

- **A draft is submitted** while the editor is open, by chat's
  `submit_document`, by a person in another tab, or by `fileAuthoredDocument`.
  The save then finds a reviewing document and withdraws it (SD-9). The person
  never saw FR-4.2's warning.
- **A draft is submitted and then approved by the agent reviewer.** The hash is
  still equal, so the save writes into an approved document in place. That
  breaks DESIGN-003 L-2, and the watcher raises `document-integrity`
  (`rules.go:615-629`).
- **A reviewing successor is approved** while open. The takeover moves
  `<name>.rev.md` to the canonical path (`actions.go:421`). A save addressed by
  path either finds no live document, or, if it writes by path, recreates
  `.rev.md` as a stray file.
- **A document is moved** with `git mv` and followed by its ID (SPEC-015
  SD-10). The same thing happens to the old path.

SD-15 doesn't say what `POST /ui/edit/save` carries to identify the document.

**Fix.** The form carries the document's row ID, the state and path shown when
the editor opened, and the base hash. A save first refuses, with a sentence and
a reload link, if the row's state or path differs from what was shown. The
target path always comes from the row. A save never creates a file that isn't
there (FR-1.6 covers opening, not saving). Add acceptance tests for "submitted
while open", "approved while open" and "moved while open".

### R16-4 (material) — Editing a draft that waits for its author sets off the author, so SD-9 and FR-4.4's fourth warning say untrue things

**What's wrong.** A sent feature's draft spec or plan with an open human issue
or an unresolved major finding "waits for its author" (`waitsForAuthor`,
`authoring.go:180-187`). The heartbeat sweep dispatches the author unless it
was "attempted since the draft last changed". It reads that as
`d.queued_at >= doc.updated_at` (`authoring.go:283-296`), and any update to
the row, including the reindex after a save, bumps `updated_at` (the
`documents_updated_at` trigger, `migrations/0001_init.sql:87`). The author
then rewrites the whole file (`fileAuthoredDocument`, `authoring.go:786`)
within one heartbeat, 30 seconds by default.

That has two consequences:

- **FR-4.4's fourth row** tells the person "If you address them yourself,
  submit it when you're done." In practice the author is dispatched within 30
  seconds, and from then on the editor is refused (SD-11). The sentence is true
  only under an `authoring-deadlock` question, or for an unsent feature, where
  nothing will ever come and "waiting for its author" is itself false.
- **SD-9** says a withdrawn sent spec is "a draft with nothing against it … so
  nothing is dispatched". A reviewing spec can carry open issues (SPEC-011
  FR-6.1 allows them in `reviewing`) and unresolved major findings from earlier
  rounds. Withdrawn, it waits for its author, and the sweep dispatches the
  author over the person's edit.

**Fix.** Decide the rule, and make the sentences match it. Either:

- refuse to edit a sent feature's draft that waits for its author, except while
  an `authoring-deadlock` question is pending ("The author will revise this
  draft; answer the question in the Inbox to edit it yourself"); or
- say plainly that the author will be sent back to it.

In either case, SD-9's sentence must cover the case of open issues. Split the
fourth warning by sent or unsent, and by deadlock. Test that an edit to a
waiting draft on a sent feature does what the sentence says.

### R16-5 (material) — The "being built" warning is false: agents read a revision's draft, straight from the main tree

**What's wrong.** FR-4.4's second row says: "Agents work in their own worktrees
and won't see this change until it's approved and merged into their work." The
prompts for `implement-task`, `review-code` and verification take the spec from
`contractBodies` (`planner.go:214, 257, 302, 485`). That calls
`CurrentDocForOwner`, which returns the newest live spec
(`internal/store/documents.go:97-101`), and reads its file from the main
working tree (`planner.go:383-395`). Once a revision is open, the newest live
spec is the draft successor. Every Save therefore reaches the next task's
prompt at once, before submission, review or `spec_stale`. The code's own
comment says "current approved spec", so this is a bug that predates M9. But
the editor makes it casual, and the spec's warning tells the person the
opposite. SD-14 also calls these paths "already … safe".

**Fix.** In M9, make `contractBodies` read the approved spec and plan
(`CurrentApprovedDocForOwner` exists), with a test that an open revision's
draft text is not in a task prompt. Then the warning is true. If that is left
for later, the warning must say that new tasks will read the draft as saved.

### R16-6 (material) — Withdrawing a reviewing successor on a building feature strands the revision-in-flight question

**What's wrong.** Submitting a successor spec or plan while its feature is
`active` or `review` sets `spec_stale` and raises a `revision-in-flight`
question on the feature (`rules.go:418-427`, `actions_phase2.go:519-549`).
`refuseIfPending` doesn't consider that kind (`review_send.go:341-360`), so
SD-11 lets the editor withdraw the successor. The result:

- the feature stays blocked;
- the Inbox asks about "a revised spec … submitted" that is now a draft;
- answering *Continue* resumes tasks against the draft (R16-5); and
- resubmitting raises the question again.

**Fix.** Either add `revision-in-flight` to the SD-11 refusals for a reviewing
successor of that feature, or define what the withdrawal does to the flag and
the question. Update FR-4.4's second-row sentence either way, and test it.

### R16-7 (material) — CRLF files and files with no front matter are never reindexed, and every preview shows the front matter as text

**What's wrong.**

- **Reindexing.** `reindexDocument` parses with `content.Parse`
  (`actions.go:606-612`). That uses `config.SplitFrontMatter`, which accepts
  only `---\n` (`compartment.go:152-166`), so a CRLF file or a file with no
  front matter fails. The failure is logged and returns `nil` without
  re-recording the hash. That breaks two acceptance criteria. FR-1's saves a
  CRLF file, and FR-6's expects "its content hash is the new file's hash". The
  M8 handoff lists this as follow-up 6. SD-13's "exactly as for a vim edit" is
  true, but the vim edit fails too.
- **Preview.** Browsers submit textarea line breaks as CRLF (the spec says so
  in FR-1.2). The preview renders the posted text through the same
  `SplitFrontMatter` (`ui_entity.go:258`), so it fails for every document,
  LF files included, and shows the front matter as prose. FR-1's acceptance
  ("renders the text's prose") would fail.

**Fix.** Normalise CRLF to LF, and drop a BOM, before rendering the preview.
Either make `SplitFrontMatter` and `Parse` accept CRLF and a BOM in M9, or
narrow FR-1's and FR-6's acceptance and say why. A reindex that can't parse
should still re-record the content hash, so the watcher stops re-flagging the
file.

### R16-8 (material) — The leave warning can't work, because every page is boosted

**What's wrong.** SD-1 and SD-7 rely on "a warning before leaving with unsaved
text" as the only guard on the person's text: Cancel, Reload from disk, and
every navigation link. The body carries `hx-boost="true"` (`partials.html:26`).
Boosted links and forms are swapped in by htmx without unloading the page, so
`beforeunload` never fires. The person's text is then thrown away silently,
which is the thing SD-7 promises won't happen.

**Fix.** Put `hx-boost="false"` on the editor page's body or container.
Alternatively, have `editor.js` cancel `htmx:beforeRequest` for navigation away
while the text is dirty. Add a walkthrough step that types, clicks a
navigation link, and sees the prompt.

### R16-9 (material) — Cross-site requests can now overwrite and commit documents

**What's wrong.** §6 defers request forgery to "the whole UI". Until now the
worst a forged `POST /ui/*` could do was trigger a lifecycle act. The editor
writes arbitrary text into a file and commits it under the operator's name. The
lock is no defence: the base hash of a document in any public or shared
repository can be computed by an attacker, and a plain HTML form post needs no
preflight. That is new risk, created by this milestone.

There is also a path hazard. The file is resolved with `filepath.Abs` alone
(`documents.go:22-33`), so a registered path through a symlinked directory
writes the temporary file and the rename outside the repository.

**Fix.**

- Refuse `POST /ui/edit/*` unless `Origin` (or `Sec-Fetch-Site`) is same-origin.
  That is a few lines, and it can later move to all of `/ui/*`.
- Identify the document by row ID (R16-3), and never accept a path from the
  form.
- Resolve the path with `filepath.EvalSymlinks` and refuse anything outside
  the repository, or anything that is a symlink.
- Test a cross-origin post and a symlinked document.

### R16-10 (material) — SD-10 blocks the way forward that SPEC-015 gives for a foreign `id:`

**What's wrong.** SD-10 says a document with no ID "whose file already declares
one … must keep what it declares". Adopt refuses a file whose own `id:` doesn't
fit, and its sentence tells the person to "correct or remove the file's `id:`
line" (SPEC-015 SD-11, choice 14, approved). The editor would refuse exactly
that edit. The clause also cites FR-4.6 "in reverse", which is unclear:
FR-4.6 is about a registered ID missing from its file.

**Fix.** For a document with no ID, refuse only *adding* an `id:` where the
base had none. Removing or correcting an existing one is allowed. Test both.

### R16-11 (material) — The acceptance criteria leave required behaviour untested

**What's wrong.** The brief's list is covered in outline, but these are
missing:

- **The revision end to end:** edit, Save, submit and approve, so that the
  takeover succeeds (R16-1).
- **The warnings:** only the "sent" and "building" rows have acceptance. The
  design row, the findings row and the reviewing warning's extra sentence
  have none.
- **SD-11:** the pending-question refusal (a `review-escalation` on a reviewing
  document, and a design-revision question when starting a revision) is
  untested. Only the author-at-work refusal is listed.
- **SD-5:** there is no test on an untracked file, and none for "Save by the
  editor, then an edit in vim, then Save & commit" being refused.
- **FR-1.2:** there is no BOM round trip.
- **SD-3:** there is no test with other staged *and* untracked files, none of
  a failed commit's message, and none of a repository with no git identity
  (R16-2).
- **SD-9:** there is no test that a running review's verdict is dropped after
  the edit.
- **R16-3 and R16-9:** state changes while the editor is open, cross-origin
  posts and symlinks are untested.

**Fix.** Add each of these to the relevant FR's acceptance, so DoD 1 covers
them.

## 3. Minor findings (should fix)

### R16-12 (minor) — SD-5 can be passed in two clicks

**What's wrong.** Save isn't refused on a dirty file. Its hash then counts as
"something Subutai wrote itself", so Save followed by Save & commit commits
someone else's uncommitted changes under the operator's name. That is the
thing SD-5 exists to stop.

**Fix.** Either exempt an editor save only when its own base was clean, or
state in SD-5 that the person accepted those changes by saving them.

### R16-13 (minor) — "The post-commit hook finds the hash already current" isn't guaranteed

**What's wrong.** The editor's event goes through the asynchronous bus. The
hook's `publishIfDrifted` (`documents.go:321-334`) can run before the reindex
lands and publish a second `document.file_changed`. That is harmless: it
causes a second reindex.

**Fix.** Say "at most one more reindex" rather than "does nothing more".

### R16-14 (minor) — The claims about the checks other acts make are overstated

**What's wrong.**

- SD-11 calls `refuseIfPending` "the check every other act makes".
  `handleDocSubmit` doesn't make it (`ui_send.go:523-553`), and neither does
  `handleDocRevise` (`ui_send.go:555-574`). SPEC-011 SD-11 covers FR-5.5 and
  FR-6 only.
- FR-4.3 says `/ui/edit/revise` runs "the same checks … as the document page's
  Revise button". Those checks are only "approved" and "no live successor". So
  the editor would be stricter than the page, which still revises decisions and
  untemplated types (SD-12), and SD-16 says the page otherwise stays as it is.

**Fix.** State the differences, or give `handleDocRevise` the same refusals.

### R16-15 (minor) — The cascade and design sentences are loose

**What's wrong.**

- **The cascade.** FR-4.3's sentence says "each is either superseded or put to
  you in the Inbox, spec by spec". The code (`authoring.go:414-504, 550-640`)
  does something narrower:
  - only *approved* specs are affected;
  - if there is one, it is superseded (idea or ready), or rewritten as a
    revision (building);
  - if there are several, one question is asked;
  - unsent features are left without a spec (DESIGN-010 §5).
- **FR-4.4's design row.** It says a draft design's approval "reaches their
  specifications". That is true only for a successor design; a second,
  non-successor design runs no cascade.
- **Approved specs.** For an approved spec of a sent, ready feature, the page
  doesn't say that approving the revision supersedes its plan and returns the
  feature to `idea` (`review_send.go:183-220`).

**Fix.** Correct the three sentences.

### R16-16 (minor) — Line endings, the BOM and the textarea need pinning down

**What's wrong.** Several details of how text travels between the file and the
form are left open:

- The posted base comes back newline-normalised by the browser, so the lock
  must compare the posted *hash*, never a re-hash of the posted base.
- Both diffs must be taken after normalising to LF, or every line differs.
- A file with mixed endings is rewritten line by line.
- HTML drops one newline straight after `<textarea>`, so the template must
  emit one, or a file that starts with a blank line loses it (NFR-2).
- "Nothing changed" must compare after the BOM and line endings are restored.
- A custom subject needs CR and LF stripped, and the message passed with
  `-m` or `-F`.
- A browser refresh after a save re-posts the form.

**Fix.** Pin each of these in FR-1.2 and SD-3. Redirect after a successful
save.

### R16-17 (minor) — The withdrawal needs its authority stated, and its stall shown

**What's wrong.** DESIGN-003 §2 gives `withdraw` to the author. SD-9 has the
operator withdraw a spec the agent wrote. That is reasonable, but it is new, and
the spec doesn't say so. After the withdrawal, nothing on the feature page or
in the Inbox shows that a sent feature is waiting for the person to resubmit.

**Fix.** State the extension of authority. Add a line to the document page and
the feature page: "Taken back to draft for editing by <actor>. Submit it to
continue."

### R16-18 (minor) — SD-12 departs from DESIGN-010 §7, and its sentence is missing

**What's wrong.** DESIGN-010 §7 says that "editing an approved document creates
a successor draft". Refusing to do so for untemplated types departs from that
approved design. The departure is sound, given M8 follow-up 3, but it should
be named as one. SD-12 also says the refusal "names the way the file can be
changed today" without giving the sentence.

**Fix.** Name the deviation, and write the sentence out.

### R16-19 (minor) — Git and file hazards on a failed commit and on the rename

**What's wrong.**

- If `git add` succeeds and the commit then fails (a pre-commit hook, a merge
  in progress, `commit.gpgsign`), the file stays staged. The next takeover or
  archive commit, which has no pathspec, then sweeps it in.
- A gitignored path fails `git add`.
- Renaming into place replaces the inode, which loses hard links, ownership
  and ACLs.

**Fix.** On a failed commit, unstage what the editor staged
(`git reset -q -- <path>`). Name the likely causes in the failure sentence.
Note the inode behaviour.

### R16-20 (minor) — Prose

**What's wrong.**

- "They don't both." (SD-13) is clumsy.
- "A note on prose" says every label is a full sentence, but the buttons are
  not ("Edit — starts a revision"). Say it applies to notices and refusals.
- FR-2.1 and FR-2.2 "re-render the document page", which conflicts with R16-16's
  redirect.

Spelling is British throughout.

**Fix.** Reword those three.

## 4. Checks that passed

These claims are true:

- The heartbeat doesn't rehash documents (`server.go:236-256`). The post-commit
  hook and the boot scan publish drift (`documents.go:267-334`).
- Rules reindex draft and reviewing documents (`rules.go:615-629`).
- `withdraw` (`reviewing → draft`) exists (`lifecycle/document.go:48-53`).
- Stale verdicts are dropped by the content hash in the review key
  (`actions.go:102-110`, `rules.go:555-558`).
- `cancelQueuedReviews` and `refuseIfAuthorAtWork` behave as described.
- `ReviseDoc` stamps the same ID at the next revision.
- `liveSuccessor` finds an open revision.
- An untemplated successor can't be recorded as already approved
  (`canRecordAlreadyApproved` requires no predecessor).
- `commitPath` already commits one path with `--only` semantics.
- No migration is needed.

## 5. The choices

The fourteen are the right questions:

- **Choice 3** needs R16-2's mechanism.
- **Choice 5** needs R16-12's caveat.
- **Choice 7** needs R16-4, R16-6 and R16-17's consequences written into it.
- **Choice 12** ("warnings, not refusals") is sound only once the warnings are
  true (R16-4, R16-5, R16-15).

Two decisions are taken silently, and should be listed for Sam:

- whether to fix `contractBodies` in M9 (R16-5); and
- whether to refuse editing drafts that wait for their author on sent features
  (R16-4).

## 6. Disposition

*Completed by the author.* Every finding, material and minor, was accepted and
fixed in the spec and, where it named behaviour, in the code, with a test. The
two decisions §5 said were taken silently are now choices 15 and 16, and the
two the review's fixes added are choices 17 and 18. SPEC-016 §7 maps each
finding to what changed. Three parts were left for follow-ups, and the spec's
§6 names them:

- `archiveInvalidatedFiles`'s commit (R16-1);
- the document page's own reading of CRLF front matter (R16-7); and
- a line on the feature page for a withdrawn document (R16-17).

The first touches the cascade, beyond this milestone. The other two touch
pages shared with M10.
