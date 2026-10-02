# REVIEW-018: Decisions

**Status:** Complete, and disposed of by the author in
[SPEC-018](../specs/SPEC-018-decisions.md) §7. **Sam approved SPEC-018 and the
build on 2026-09-28, with all fifteen choices.** The verdict below is of the
first draft.
**Reviewed:** [SPEC-018: Decisions](../specs/SPEC-018-decisions.md), draft of 2026-09-28 (commit `9e3b8f2`). I checked it against DESIGN-010 §4, §7, §11 and §17b; DESIGN-003; DESIGN-004; SPEC-015, SPEC-016 and SPEC-017; DEC-004 and DEC-006; the context, evidence-base and prefix-cache research; audit item C-6; the M8 handoff; and the code at `HEAD`.
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent
**Verdict:** **Revise before approval.** The design is sound: rulings in front matter, amendment as a revision, supersession as a new decision, and a pushed block under a cap. But there are eleven material findings. Several are defects the code would turn into wrong behaviour. The worst is that a superseded decision can be adopted back into every prompt (R18-1). Others are claims the code or the approved documents contradict. The spec also depends on the manifest and validation machinery in ways it doesn't specify.

> Line numbers are for `HEAD` (`9e3b8f2`). While I reviewed, a builder had uncommitted changes in the working tree, including `0012_decisions.sql`, `lifecycle/validate.go` and `config.go`. Where those changes bear on a finding, I say so.

*How each finding was dealt with is in SPEC-018 §7.*

## Summary

The spec covers what DESIGN-010 §11 asks for: decisions as documents, never edited, only amended or superseded; the ruling and reason pushed under a hard cap; conventions by the same mechanism; and a viewer. It settles §17b with a worked argument.

The problems fall into five groups:

1. **Supersession leaves a hole in identity.** A superseded decision's file stays at its path and is no longer "live". So adopt, attach and file-name numbering will bring it back (R18-1). The approved lifecycle design also says supersede happens only on a successor's approval, and archives the file. The spec departs from both without saying so (R18-2).
2. **The append-only amendment check doesn't fit the documents it must serve.**
   - The precedent DESIGN-010 §11 names, DEC-006 Amendment 1, is not append-only (R18-3).
   - Floor link checks on text that can never change make DEC-003 and DEC-005 impossible to amend (R18-4).
   - How the check replaces the manifest isn't specified, and the manifest's required fields contradict FR-3.3 (R18-5).
3. **Acceptance is not safe.** Nothing re-validates or checks the hash at acceptance (R18-7). Amendment and supersession can race, or collide in state, with no rule for what happens (R18-6). Decisions attached without an ID can now be accepted (R18-8). The claim that follow-up 3 is closed is false for the HTTP route (R18-9).
4. **The cap arithmetic misreads the research it cites (R18-10), and the cap's own rules can't all hold at once (R18-11).**
5. **Smaller gaps:** surfaced titles, ordering ties, viewer rows, cache claims, partial supersession in this repository, conventions edge cases, the Definition of done, and prose.

## Findings

### R18-1 (material) — A superseded decision can be adopted again and returns to every prompt

**What the spec says.** SD-6 and FR-3.5.2: on accepting the new decision, the old one becomes `superseded`, and "its file is not moved or changed".

**What's wrong.** Every identity check in the code treats `superseded` as "not live":

- `store.LiveDocumentByPath` (`internal/store/documents.go:93`) excludes superseded rows.
- `store.IdentityUse` (`internal/store/identity.go:41-46`) reports `live = bool_or(state <> 'superseded')`.
- The `documents_live_path` index (`0001_init.sql:85`) covers only non-superseded rows.

So after DEC-008 supersedes DEC-005, `docs/decisions/DEC-005-….md` is still there, still carries `id: DEC-005`, and belongs to no live document. What happens if a person adopts it in the web UI as approved:

1. `AdoptDocument` (`identity.go:301`) finds no live row.
2. `checkAdoptRequest` passes, because SD-3 keeps decisions approvable by adoption.
3. `adoptIdentity` (`identity.go:609-640`) reads the declared `DEC-005`, sees `live=false`, and returns **DEC-005 revision 2**.
4. It is registered as approved, and under SD-16 `onAccepted` records its surfaced text.

DEC-005 is then back in every prompt, while `decision_supersessions` still says DEC-008 replaced it. The same happens to any new file named `DEC-005-*.md`, because `nameTaken` only fires when the ID is live. That is exactly the case the existing test at `integration_identity_test.go:434` guards. Attach (`registerDocBy`) will also register a new draft at the old path.

Separately, `CatchUpScan` hashes only non-superseded rows (`documents.go:329`). So hand edits to a superseded decision that stays in place are never flagged, despite "is not changed".

**Recommended fix.** Do one of these:

- Archive the superseded decision's file as SPEC-011 FR-6.6 already does for a plan superseded with its spec (`supersedePlanWithSpec`, `review_send.go:209`, and `archiveInvalidatedFiles`).
- Or keep it in place and add explicit refusals. Adopt, attach and ID reuse refuse any `DEC-` ID with a row in `decision_supersessions` ("DEC-005 was superseded by DEC-008 and is a record; it can't be adopted again."). Extend the integrity scan to superseded decisions.

Add a test for each route.

### R18-2 (material) — Supersession departs from DESIGN-003 without a dated note, and other approved documents change silently too

**What the spec says.** SD-6 and FR-3.5.2 have "the lifecycle engine" supersede a different document. SD-3 and FR-4.3 add validation to approved adoption. FR-1.2 adds rule kinds.

**What's wrong.** The spec changes approved documents in four places and records none of them as a departure:

- **DESIGN-003 §2.** The transition table says `supersede` is "fired only by a successor's approval — never directly by a user". A superseding decision is not a successor: it has no `supersedes_id`. Its code comment agrees (`lifecycle/document.go` on `DocumentTransition`).
- **DESIGN-003 §5 and DESIGN-010 §7.** The predecessor's file is archived under `docs/_superseded/`. SD-6 deliberately keeps it in place.
- **SPEC-015 SD-11.** "Nothing is validated on an approved adoption". FR-4.3 now validates the caps.
- **DESIGN-004 §7.** New rule kinds "are listed here". FR-1.2 says so, but no step in the Definition of done does it.

**Where it's handled.** SD-3 isn't among the fourteen choices, although it reverses an approved choice (SPEC-015 choice 10). The Definition of done has no step for dated notes.

**Recommended fix.** Add DoD steps for dated notes on DESIGN-003 §2 and §5, SPEC-015 SD-11, DESIGN-004 §7, and DESIGN-010 §17b ("settled in SPEC-018"). Make SD-3 a choice. Name the SPEC-011 FR-6.6 plan supersession as the precedent, or say why this case differs.

### R18-3 (material) — The append-only rule would refuse the amendment DESIGN-010 §11 cites as its model

**What the spec says.** SD-5 and FR-3.3.1–2: the accepted text is kept byte for byte, and only a dated `## Amendment …` section may follow. SD-5 says this is "as DEC-006's Amendment 1 was".

**What's wrong.** DEC-006's Amendment 1 changed the text above it in several places:

- the **Status** block at lines 3–7 ("Amended the same day: Amendment 1 replaces decisions 5, 6 and 8");
- inline markers at lines 67, 76, 87 and 101 ("*(Replaced by Amendment 1.)*").

Later dated notes were also inserted inside the amendment, before `### Rationale` (lines 352 and 362), not at the end. DEC-004 has the same shape: its Status line mentions Amendment 1, and a note sits at line 143.

Under FR-3.3 none of this is possible. A reader of the file (or an agent following "Their files are in the repository") would read decision 5 of DEC-006 as still standing.

**Recommended fix.** Do one of these:

- Say plainly that Subutai's form is stricter than the repository's past practice, and that the viewer and the restated `ruling:` are the only places the change shows.
- Or allow a narrowly defined marker, such as front matter `amended: [dates]`, which the renderer shows as a banner.

Either way, amend SD-5's "as DEC-006's Amendment 1 was" to describe the difference. Also say whether a dated *note*, as opposed to an amendment, is supported. This repository uses them.

### R18-4 (material) — Floor checks on text that can't change make DEC-003 and DEC-005 impossible to amend

**What the spec says.** FR-3.3.4: the floor checks (parse, placeholders, links) apply to the whole file.

**What's wrong.** `linkChecker` (`documents.go:35`) calls `os.Stat` on each relative target. At `HEAD`, three links in this repository's decisions don't resolve:

- DEC-003 links `../../internal/server/ui/templates/planning.html`, which no longer exists.
- DEC-005 links `../../internal/rules/rules.go:202` and `../../internal/server/integration_mcp_test.go:75`. The `:line` suffix makes the path fail.

These links sit in accepted text that FR-3.3.1 forbids changing. So DEC-003 and DEC-005 can never be amended, and can never have their ruling recorded (FR-3.2).

FR-8.2's test has the same trap. It copies DEC-001 to DEC-007 into a test project, where DEC-003 to DEC-007 have 6 to 13 relative links each and none resolve. Only DEC-001 and DEC-002 have none.

**Recommended fix.** Apply the link and placeholder checks only to the appended text and the changed front-matter fields. Record that as a named, narrow departure from DESIGN-004 F-6: the inherited text was already accepted. Have FR-8.2 name DEC-001 or DEC-002, or copy the link targets.

### R18-5 (material) — How the amendment check replaces the manifest is unspecified, and the manifest contradicts it

**What the spec says.** FR-1.1 requires front matter `[title, type, owner, ruling, reason]`. FR-3.3 applies "these checks … and not the template's section checks". FR-3.3.3 says `title`, `type` and `owner` can't change.

**What's wrong.**

- **Required fields.** An adopted decision's front matter is only `id` and `revision`: `SetIdentity` adds exactly those, as the existing test at `integration_identity_test.go` asserts. If the manifest's `front_matter.required` still applies, every amendment of DEC-001 to DEC-007 fails on missing `title`, `type`, `owner` and `ruling`. Adding them is forbidden by FR-3.3.3. FR-3.3 drops only the *section* checks, so the builder must guess.
- **Placeholders.** `lifecycle.Validate` checks front-matter placeholders only for fields in `Required` (`validate.go`, check 4). If the manifest is bypassed, the `{{…}}` placeholders FR-3.2 puts in `ruling` and `reason` go unchecked, unless the spec says to check them.
- **Many callers, no predecessor.** `lifecycle.Validate(manifest, raw, …)` is called from `ValidateDoc` (`documents.go:152`), `submitDocWith` (`:182`), `SubmitFromChat` (through `submitDocWith`) and `buildReview` (`:420`). None of these has the predecessor. `buildReview` puts the report into the reviewer's prompt, so SD-2's "no code is needed" for a project-added reviewer is false: amendments of adopted decisions would show spurious failures.
- **Normalisation.** "Byte for byte" and "begin with that body exactly" don't say whether CRLF and a BOM are normalised. `content.Parse` normalises both. The browser editor keeps the file's own endings (`fileFromText`, `edit.go`), but an external editor may not.
- **Front-matter comparison.** It isn't said whether front matter is compared as text or as parsed YAML. Reordering keys or changing quotes would fail a text comparison.
- **`one_line`.** After YAML parsing, a folded or plain multi-line scalar has no line break left. FR-1.2 "holds no line break" should say whether the check reads the source lines or the parsed value.

**Recommended fix.** Define one function, for example `validateDecisionRevision(manifest, predecessorRaw, successorRaw)`, and use it at every call site above. Specify each of these:

- which manifest parts it skips: all of `front_matter.required` and `sections`;
- which it keeps: the `rules` on `ruling` and `reason`, plus explicit placeholder checks on those two fields;
- that the body comparison is on the parsed, normalised body (`Doc.Body`);
- that front matter is compared as parsed maps, ignoring `id`, `revision`, `ruling` and `reason`.

### R18-6 (material) — What happens when amendment and supersession collide, or run concurrently, is unspecified

**What the spec says.** FR-3.5.3: an open amendment of a superseded decision "is left as a draft", and submitting it is refused. FR-3.6 checks `supersedes:` at submit.

**What's wrong.**

- **The amendment may already be in review.** If so, it isn't a draft. Its later approval runs `TransitionDocument(predecessor, DocSupersede)` (`actions.go:377`) on a predecessor that is already `superseded`. That fails with the raw error `document event "supersede" is illegal in state "superseded"`, which is not a sentence and isn't handled.
- **Two decisions can supersede the same one.** Both pass FR-3.6 at submit. At the second acceptance there is no accepted revision left. FR-3.5.2 doesn't say whether to refuse or skip.
- **No locking.**
  - `approveDocumentAs` reads the predecessor outside the transaction (`actions.go:335`).
  - `TransitionDocument` issues an unconditional `UPDATE … SET state` with no state guard (`store/documents.go:109-121`).
  - An amendment approval and a superseding acceptance running at the same time can both commit. DEC-005 revision 2 then stays accepted while DEC-008 records that it superseded DEC-005.

**Recommended fix.** In `onAccepted`:

- take a transaction advisory lock per `DEC-` ID, as `lockIdentity` does;
- re-read each target with `FOR UPDATE`;
- refuse with a sentence if a target is no longer accepted.

In the same transaction, withdraw any open amendment that is in review back to draft. Have amendment approval refuse if its ID appears in `decision_supersessions`. Add tests.

### R18-7 (material) — The surfaced text is taken at acceptance without re-validating, and adoption has an ordering trap

**What the spec says.** FR-3.5.1 records the ruling and reason at acceptance. FR-4.1 says an over-long ruling "can't be accepted by the normal route". FR-8.1 expects adopted decisions to surface by their title.

**What's wrong.**

- **Edits after submission are accepted unchecked.** `DirectApprove` and `approveDocumentAs` (`review_send.go:380`, `actions.go:326`) neither re-validate nor compare the file's hash with `content_hash`. A file that changes on disk while `reviewing` is simply re-indexed (`rules.go`, `decideFileChanged`). So a vim edit after Submit can put a 500-word ruling into `surfaced_texts`. `buildReview` protects its own case ("changed on disk since submission; re-submit"); acceptance has no equivalent.
- **Adoption would record the path.** In `AdoptDocument` the title is set from the first heading *after* `recordAlreadyApproved` has run in the same transaction (`identity.go:376` against `:381`). An `onAccepted` hooked into `recordAlreadyApproved` would record the file's path as the surfaced text for DEC-001 to DEC-007.

**Recommended fix.** At acceptance, check that the file's hash still matches `content_hash`, and re-run the caps; refuse with a sentence otherwise. Specify that `onAccepted` runs after the title is settled, or that it derives the title itself.

### R18-8 (material) — A decision attached without an ID can now be accepted, but everything assumes `DEC-nnn`

**What the spec says.** FR-6.2 renders `- DEC-008 …`. FR-7.1 has one row per decision ID. FR-3.6 matches `supersedes:` by ID.

**What's wrong.** Attach registers without an ID. `registerDocBy` passes `""` (`documents.go:65-80`), and SPEC-015 SD-12 says attach stays path-only. Until now an attached decision couldn't be submitted, because it had no template. With FR-1 it can be submitted and accepted with no number. The surfacing line, the viewer and `get_decision` have nothing to name it by.

**Recommended fix.** Either mint or adopt an ID when a decision is attached, or refuse Submit and acceptance for a decision without one: "Give this decision its number by adopting it, then submit it."

### R18-9 (material) — The claim that "no route makes a successor that can't be submitted" is false for the HTTP route

**What the spec says.** SD-7: "`ReviseDoc` itself — reached by the HTTP API or the CLI's `doc revise` — opens an amendment … This closes the M8 handoff's follow-up 3 for decisions."

**What's wrong.** `handleRevise` (`http.go:361`) calls `reviseDocBy` directly, not `ReviseDoc`, and doesn't call `reviseRefusal`. The same core is used by:

- the page's Revise (`ui_send.go`);
- the editor (`StartRevisionForEdit`, `edit.go`);
- `RaiseIssue` (`review_send.go`);
- authoring (`authoring.go`).

Changing only `ReviseDoc` would leave `subutai revise` making plain copies, which the append-only check would then refuse. SD-7 also doesn't say what `ReviseDoc` opens for a decision with no `ruling:`: an amendment, or Record its ruling. And follow-up 3 names "a note" too: the API still revises untemplated notes, research, reports and policies into successors that can't be submitted. SD-12 doesn't say so.

Separately, `RaiseIssue` on an accepted human-approved type answers "This design is approved…" (`review_send.go:538`). With decisions and conventions now human-approved, it would say that about them.

**Recommended fix.**

- Put the decision branch in `reviseDocBy`.
- Apply `reviseRefusal`, or its decision variant, in `handleRevise`.
- Say which revision a decision with no ruling gets.
- Make the `RaiseIssue` wording type-aware.
- State that follow-up 3 remains open for the four untemplated types on the API route.

### R18-10 (material) — The cap arithmetic misreads the research it cites

**What the spec says.** SD-8: the block holds the conventions and "roughly 12 to 15 decisions. That is the research's range." It also says the chars-÷-4 estimate "errs slightly high, which is the safe direction".

**What's wrong.**

- **The count exceeds the research.** The context research §3 ("Keep the pushed block small") and the evidence base §4.1 say compliance degrades past **about nineteen requirements**, and ask for five to fifteen things *in the conventions block*. Conventions of five to fifteen points plus twelve to fifteen decisions is 17 to 30 requirements, which goes past the cliff. The research's range is for the whole pushed block, not for decisions alone.
- **The estimate errs low, not high.** Four characters per token is roughly right for English on older tokenisers. Claude's tokeniser usually yields *more* tokens per character, and code identifiers and non-English text more still.
- **Characters is undefined.** "Characters" could mean Go `len`, which counts bytes (an em dash is 3), or runes.

**Recommended fix.** Restate the argument against a total of about fifteen to nineteen items. That points to a default nearer 1,000 tokens, or to counting items as well as tokens. Say that the estimate is nominal and may err low. Define it as `ceil(utf8.RuneCountInString(block)/4)`, or use a divisor of 3.5.

### R18-11 (material) — The cap's own invariants can't all hold

**What the spec says.**

- SD-9: "The conventions document always goes in."
- FR-6.4: the block, "including its headings and the left-out line, must not exceed" the cap.
- FR-6.5: below 600 is refused; `0` is refused too; absent means the default.

**What's wrong.**

- **300 words doesn't bound tokens.** FR-1.2 counts a word as any run of non-space characters. A 300-word conventions document with identifiers or paths can exceed 1,000 tokens.
- **The config can't know the manifest's cap.** The project can raise the conventions cap in its own manifest. The config check can't see that, so the 600 floor guarantees nothing.
- **The left-out line is unbounded.** With many decisions left out, the line alone can exceed the cap. The rule is then unsatisfiable, and it's circular, because admission changes the line.
- **YAML can't tell absent from `0`.** Absent and `0` are the same value for an `int`. The working-tree `SurfacingMaxTokens` already treats `0` as the default, contrary to FR-6.5.

**Recommended fix.**

- Say what wins when the conventions alone exceed the cap. For example, still include them and raise a checkpoint, or check the conventions' *estimated tokens* at validation against the configured cap.
- Bound the left-out line ("…and 23 more; see /ui/decisions") and give the admission algorithm with room reserved for that line.
- Either use `*int` or drop "0 is refused".

### R18-12 (minor) — Surfacing by title would repeat the ID

**What the spec says.** SD-1 and FR-6.2: `- DEC-001: Server Language — Go`. SD-9's example is "DEC-002 (Postgres via Supabase)".

**What's wrong.** Adopted titles already begin with the ID: the existing test asserts `strings.HasPrefix(d.Title, "DEC-001:")`. Rendering `<ID>: <title>` gives `DEC-001: DEC-001: Server Language — Go`. DEC-002's actual title is "DEC-002: State Store — Supabase-hosted Postgres, plain-Postgres compatible", not the example's.

**Recommended fix.** Specify stripping a leading `<ID>` followed by `:`, `—` or `-` from the title, and correct the examples.

### R18-13 (minor) — The admission order isn't deterministic as written

**What the spec says.** SD-9 and FR-6.4: nearest owner first, "newest accepted first" within an owner, and "the same inputs always produce the same bytes" (NFR-1).

**What's wrong.**

- **Ties.** DEC-001 to DEC-007, adopted in one sitting, have near-identical `approved_at`, and no tie-break is given.
- **Which acceptance counts.** For an amended decision, "newest accepted" could mean its first acceptance or its latest revision's. With the latest, amending an old decision promotes it.
- **Nested initiatives.** "Nearest" is clear for a feature's initiative, but the spec should say plainly that the branch is the subject's initiative and its ancestors only, never its descendants.

**Recommended fix.** Order by the first acceptance of the `DEC-` ID, descending, with the number descending as a tie-break. Put the branch definition in FR-6.1.

### R18-14 (minor) — The viewer's "newest revision" row hides accepted decisions that have an open amendment

**What the spec says.** FR-7.1: one row per ID, "its newest revision", filtered to accepted by default. It shows **Amended** "when its revision is above 1".

**What's wrong.** While an amendment is open, the newest revision is a draft, so the default filter hides a decision that is still accepted and still surfaced. And revision above 1 doesn't mean amended: Record its ruling raises the revision, and SPEC-015 re-adoption gives `maxRev+1`.

**Recommended fix.** Show the current accepted revision, with an "amendment open" marker. Derive **Amended** from the `## Amendment` headings, not from the revision number.

### R18-15 (minor) — The cache claims are overstated, and one "unchanged" claim is inaccurate

**What the spec says.**

- SD-10: the block "is shared by every dispatch on its branch, whatever the role".
- SD-8: it is "cached after the first dispatch on a branch".
- NFR-3: prompts differ "only in SD-10's reordering".

**What's wrong.**

- **The prefix is role-specific.** The tool definitions and the role's system prompt come before the user message, so the block is a shared prefix only among dispatches of the same role.
- **Anthropic caches nothing today.** Caching there is explicit, and Subutai sets no `cache_control` (prefix-cache research §6; this spec's Out of scope table).
- **Implement, code review, verify and estimate prompts have no `# Project` heading today.** SD-10's table adds one, which is more than a reordering.
- **The left-out line invites fetching.** Inviting an agent to `read_file` a left-out decision cuts against NFR-5 and evidence base §4.1. Review and estimate dispatches have no tools to do it anyway.

**Recommended fix.** Restate the cache claims per role, and conditional on the provider. List the added heading in NFR-3. Drop the `read_file` invitation, or limit it to purposes that have the tool.

### R18-16 (minor) — This repository's "supersedes in part" isn't modelled

**What the spec says.** Supersession is all or nothing, through `supersedes:`.

**What's wrong.** DEC-003, DEC-005 and DEC-007 each record partial supersession in their headers: DEC-005 supersedes DEC-004 "in part", and DEC-007 supersedes DEC-005 "in part". After adoption, DEC-004 and DEC-005 surface by their full titles, with no link in the viewer. DEC-004's title, "…may not drive development", is partly superseded, and every agent will be told it.

**Recommended fix.** Say explicitly that partial supersession is out of scope and is expressed by recording a ruling (FR-3.2) that states what still stands. Or add an informational `refines:` or `amends_in_part:` relation that the viewer shows. State in FR-8 that these adopt with no `supersedes`.

### R18-17 (minor) — Conventions edge cases are unspecified

**What the spec says.** SD-13: "a second live one is refused, as a second live spec is". FR-5.4 offers *Start the conventions document* "when there are none".

**What's wrong.**

- **The precedent doesn't exist for attach.** Attach doesn't refuse a second live spec today (M8 handoff follow-up 2).
- **"None" is ambiguous.** It could mean none accepted or none live. A draft may already exist, or `docs/conventions.md` may exist unregistered, and FR-2.2's refusal isn't repeated here.
- **The template's own sentence would be pushed.** The sentence "everything below it is pushed" sits below the title, so it would itself go into every prompt.
- **Heading levels clash.** The body's `##` headings nest badly under the block's `## Conventions`.

**Recommended fix.** Specify the refusal on each route. Define "none" as no live conventions document, and refuse on an existing file. Put the template's note in an HTML comment that surfacing strips, or put it above the title. Demote or indent the body's headings when surfacing.

### R18-18 (minor) — Concurrent "Append an amendment" can delete a working copy

**What the spec says.** FR-3.1: Append is offered "when no revision of it is open".

**What's wrong.** That check isn't atomic. `reviseDocBy` writes the fixed path `….rev.md` *before* its transaction (`documents.go:262`). A concurrent second call fails on `documents_public_revision` or `documents_live_path`, then runs `os.Remove(workingPath)` (`:288`). That deletes the file the first, successful call registered. The bug predates this spec, but the spec makes the path busier: Append, Record its ruling and `ReviseDoc`.

**Recommended fix.** Take the identity lock before writing, and only remove a file this call created. Or name the working copy with a unique suffix.

### R18-19 (minor) — The Definition of done misses things the brief and the spec require

**What's wrong.** Point by point:

- **The demo.** DoD 5 says "a demo … in a browser". Earlier specs (for example SPEC-017's DoD) name **Playwright** and a screenshots folder. The demo also omits the amendment (Goal 4), the refused 200-word ruling (Goal 1) and DEC-001 to DEC-007 in the viewer (Goal 6).
- **Missing tests:**
  - adopt or attach refusing a superseded decision (R18-1);
  - a hand edit to an accepted decision never reaching a prompt (SD-14);
  - the start-up backfill;
  - the config refusal (FR-6.5);
  - an open amendment of a superseded decision, in both draft and review (FR-3.5.3, R18-6);
  - one conventions document per project (FR-5.2);
  - Record its ruling on an adopted decision with CRLF line endings.
- **Missing steps:**
  - the dated notes (R18-2);
  - updating DESIGN-004's registry note (FR-1.2);
  - updating `submit_for_review`'s description, which names only "a specification, a dev-plan or a design" (`mcp_relay.go`);
  - updating the existing prompt-order tests that SD-10 will break.

**Recommended fix.** Add each item to the Definition of done.

### R18-20 (minor) — Prose and wording

**What's wrong.** Point by point:

- "Left out of some dispatches for space" (SD-9, FR-7.2) is a fragment, contrary to the spec's own note on prose. Use "This decision is left out of some dispatches, for lack of space."
- FR-3.5.3's "Detach this draft" leaves an orphan `….rev.md` file in the repository. Say what to do with it.
- NFR-5 says "nothing here offers agents a way to query decisions", but `list_decisions` and `get_decision` do, for the chat agent. Say "dispatched agents".
- FR-6.5's refusal is quoted as "…is too small…"; give the whole sentence.
- The block's preamble says decisions "override anything in the task". A verifier judges against the spec, so say what an agent should do when an approved spec contradicts a decision: report it, as the sentence already asks.

**Recommended fix.** Make the edits above.
