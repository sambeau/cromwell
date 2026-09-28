# REVIEW-019: Consistency review of SPEC-019 (Bugs)

**Status:** Complete. The author's disposition of each finding is in §6, and
in [SPEC-019](../specs/SPEC-019-bugs.md) §7. The approval is Sam's.
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author.
**Scope:** [SPEC-019](../specs/SPEC-019-bugs.md) (first draft, commit `fb62157`), checked against:
- [DESIGN-010](../design/DESIGN-010-subutai.md): §4, §5, §5c, §6, §7, §9 and §17a item 6;
- the [status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md): §11 (M12) and §12 decision 12;
- [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) with Amendment 1;
- [DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1 and its notes;
- [DEC-007](../decisions/DEC-007-the-judgement-boundary.md);
- [SPEC-010](../specs/SPEC-010-milestones-and-roadmaps-editing.md), [SPEC-011](../specs/SPEC-011-send-to-development.md) (FR-2, FR-3, FR-5, FR-6), [SPEC-014](../specs/SPEC-014-checklists-and-jobs.md) SD-4, [SPEC-015](../specs/SPEC-015-documents-with-identity.md) and [SPEC-017](../specs/SPEC-017-chat-as-a-proper-seat.md);
- the code at `fb62157`:
  - `internal/store`: migrations `0001`, `0004`, `0009`, `0010` and `0011`, plus `migrate.go`, `entities.go`, `tracking.go`, `sends.go`, `identity.go`, `ui_reads.go`, `owner_reads.go` and `estimates.go`;
  - `internal/rules`: `rules.go` and `rules_phase2.go`;
  - `internal/server`: `authoring.go`, `review_send.go`, `ui_send.go`, `planner.go`, `documents.go`, `actions.go`, `actions_phase2.go`, `edit.go`, `identity.go`, `ui_identity.go`, `ui_entity.go`, `ui_service.go`, `ui_nav.go`, `ui.go`, `observe.go`, `worktree_ops.go`, `toolexec.go`, `mcp_tools.go`, `mcp_plan_tools.go`, `http.go` and `integration_mcp_test.go`;
  - `internal/dispatch/tools.go`, `internal/toolhost/toolhost.go`, `internal/ident/ident.go`, `internal/lifecycle/feature.go` and `validate.go`, `internal/timeline/timeline.go`, `internal/reviewhealth/reviewhealth.go`, `internal/config/compartment.go`, `internal/starter/pack/` and the UI templates.

## 1. What this review is

This is a consistency pass on the spec by a reviewer who did not write it. It checks four things:
- that SPEC-019 agrees with the approved documents;
- that what it says about the code is true;
- that its requirements can be built without guessing and tested;
- that its end-to-end claim would actually run.

It also looks at the DEC-004, DEC-006 and DEC-007 boundaries, and at whether the flagged choices are argued honestly. It does not fix the spec.

**Timing.** While this review was running, the build started in the working tree:
- new files: `0013_bugs.sql`, `internal/store/bugs.go`, `internal/lifecycle/contract.go` and `templates/bug_report/`;
- edits: `ident.go`, `config/compartment.go`, `lifecycle/validate.go`, `rules.go`, `rules_phase2.go`, `actions.go`, `documents.go`, `entities.go`, `tracking.go`, `owner_reads.go` and `ui_reads.go`.

The findings are about the spec as committed, with line numbers taken from `HEAD`. Where the in-flight code already answers a finding, that is noted, so the spec can catch up with it.

**Overall.** The spec is well organised, and its main choice (a bug is a `features` row with a kind) is sound. It gets several subtle things right: the relay with a quote, the database check, keeping minors from spec and plan reviews on their threads, and the precedent for milestones. Its weakest claim is FR-4.7's "The rest is unchanged". The code tests `type == "spec"` in about thirty places, and several of them would stop a bug outright, including Start building. So the end-to-end run in FR-4 would not work as written. The other material findings are about:
- where a fix for review findings is built;
- where the report's Summary comes from;
- routes around triage;
- whether a relayed rejection can be undone.

## 2. Material findings (must fix before approval)

### R19-1 — "The rest is unchanged" isn't true: about thirty places treat `spec` specially, and several would stop a bug, including Start building

**What the spec says.**
- FR-4.7: "Start building, decomposition, implementation, code review, … run as for a feature."
- FR-4.5: "Everything SPEC-011 gives a spec applies to a report: the hold, agent review switched off, human issues …, direct approval, sending back, asking for a fresh review, …". It adds that a successor report "supersedes its plan".
- SD-1 says the cost of the kind column "is paid once, at the reads that mean *features only*".
- SD-16 and NFR-7 say only two calls change in M11's files.

**The problem.** The code decides "is this the feature's spec?" with a literal `"spec"` all over the place. For a bug, whose spec is a `bug_report`, each of these does the wrong thing:

- **Start building refuses every bug.** `StartFeature` refuses unless `currentDocApproved(ctx, "spec", f.ID)` (`internal/server/worktree_ops.go:54-56`). The start card's reason reads the same way (`ui_entity.go:600`). **The end-to-end run in FR-4's acceptance stops here.**
- **Nobody can raise an issue on a report, approve it directly, send it back or ask for a review.** `RaiseIssue` refuses anything that isn't a spec, plan or design (`review_send.go:545-546`). The document page gives no review actions to any other type (`ui_send.go:480-482`).
- **The hold and "agent review switched off" apply only to specs.** `specHeld` returns false for any other type (`review_send.go:51`). The "agent review off, so hold it" path tests `a.DocType == "spec"` (`actions.go:258`), and so does its sentence (`review_send.go:747`). So on a project with agent spec review switched off, a bug report would go to the agent reviewer anyway. That breaks "no setting lets a spec through unreviewed" (DEC-006 Amendment 1, decision 6).
- **A successor report doesn't supersede the plan.** `supersedePlanWithSpec` returns early unless `spec.Type == "spec"` (`review_send.go:192`).
- **An approved report doesn't move the bug on.** The document-transition rules run the revision-in-flight check, the send-back to the author, the plan invariant and G1 only for `spec` and `dev_plan` (`internal/rules/rules.go:423, 440, 478, 486`). G1 itself reads `spec` (`actions.go:647`).
- **A sent-back report would be filed as a `spec`.** The `write-spec` outcome is always filed as a `spec` (`rules_phase2.go:265`), and `planAuthor` always writes one (`planner.go:459`).
- **Guards that don't apply to reports:** `refuseIfAuthorAtWork` (`review_send.go:842`), `authorDueRefusal` and its notes (`edit.go:146, 175, 575, 588`), adopt's "one live spec" rule (`identity.go:444`), and `registerDocBy`'s "spec must have a template" rule (`documents.go:66`).
- **The send screen assumes a spec** (`ui_send.go:140-230`). FR-4's acceptance ("the send screen for a bug says the specification step is done by the report") has no FR behind it.
- **Labels:** the timeline's `plannedDocs` and `docNoun` (`internal/timeline/timeline.go:171-185`); `runPurpose` (`observe.go:61-75`); `docIcon` (`ui.go:484-492`).

**Suggested fix.**
1. Add one rule to FR-4: wherever the code asks whether a document is the feature's spec, it asks `IsSpecType`; wherever it reads "the feature's spec", it reads the type for the row's kind.
2. List the sites as an appendix, saying which change and which stay spec-only.
3. Give each a test.
4. Correct SD-1's cost statement, SD-16 and NFR-7.

### R19-2 — Fixes for review findings, and for defects found in unmerged work, would be built from `main`, where the code they describe doesn't exist yet

**What the spec says.** SD-9 and FR-3.4 file minor code-review findings when the task is approved, with the task's feature as origin. SD-10 and FR-3.3 let implementers, code reviewers and verifiers file bugs mid-run. FR-4.7 builds a bug "as for a feature".

**The problem.** Minor findings describe a task's diff on the origin feature's unmerged branch. A verifier's report is about the finished but unmerged feature. A bug's worktree is branched from the main checkout's `HEAD` (`worktree_ops.go:105`). So if a person sends such a bug while its origin is still `active` or `review`:
- its implementer is asked to fix code that isn't in its worktree;
- its verifier confirms that "the defect no longer reproduces" on code where it never existed, which is trivially true and meaningless;
- the real defect then merges with the origin, unfixed.

The end-to-end run in FR-4 has exactly this shape. A mock provider hides the problem.

**Suggested fix.** Choose one, and list it as a choice.
- **(a) File review findings when the origin merges.** Collect every approving review's minors across the feature's tasks into one bug, filed when the feature merges (G3). One entry per feature answers SD-9's worry about volume better; findings on abandoned work are never filed; and the file write leaves the task-approval transaction (R19-11).
- **(b) Refuse to send while the origin is unmerged**, with a sentence such as "BUG-007 was found in FEAT-003's unmerged work. It can be sent once FEAT-003 has merged."

Either way, say what happens to a report an implementer or verifier files about the origin's own unmerged work.

### R19-3 — The report's Summary has no source, and what reporters type can't pass validation reliably

- **Nothing supplies a Summary**, though it is required (FR-1.5) and becomes the row's description, which the estimator and the corpus read.
- **"Steps to reproduce" may not become a list**, but `min_list_items` requires one. A person who types a paragraph is told only at Send, after triage.
- **Ordinary bug text can fail validation.** Validation refuses any section containing `TODO` or `{{…}}` (`internal/lifecycle/validate.go:96-110`). Stack traces, logs and Go template text are what an Actual section holds.
- **A heading in a field can break the report**, and a field containing `{{notes}}` is ambiguous with the template's placeholders.

**Suggested fix.** Add `summary` to the dialog and tools (generated for review findings); say how Steps become a list; keep Actual and Notes as literal text; validate the generated report when the bug is created and refuse with its problems; leave out empty optional sections.

### R19-4 — Several routes go round triage or leave it inconsistent

- **Other routes submit documents**: `POST /api/docs/submit` (`http.go:64`), the editor's save-and-submit and `fileAuthoredDocument` all reach `SubmitDoc`. A refusal in two handlers is bypassed.
- **A reported bug's plan isn't covered.** A dev-plan written in chat for a reported bug could be submitted and reviewed, the spending SD-6 exists to prevent.
- **Abandon isn't refused while `reported`.** The bug page inherits Abandon (`ui_entity_actions.go:307`), and there is `POST /api/features/abandon`. That leaves `triage = 'reported'` on an abandoned row; the queue would go on listing it, and `DecideTriage` would fail to abandon it again.
- **Wrong document types aren't refused**: a `spec` on a bug, or a `bug_report` on a feature, where `IsSpecType` rules would fire.

**Suggested fix.** Put the triage check in the service, for every document a bug owns; refuse Abandon on a reported bug; add an ownership rule for the two types; make the queue's query `triage = 'reported' AND state = 'idea'`.

### R19-5 — A relayed rejection can't be undone, which departs from DEC-006 Amendment 1's test, and the spec doesn't say so

DEC-006 Amendment 1 admits a relay because "everything on the relay list is small and recoverable". A relayed *accept* passes: it starts nothing. A relayed *reject* or *duplicate* abandons a feature row permanently, and the only remedy is a new report, which loses the history. §17a item 6 lets the chat agent "relay a triage decision"; making triage final is this spec's choice, and with the relay it creates the one relayed act that can't be recovered.

**Suggested fix.** Choose, and list it as choice 15: (a) a web-UI-only *Reopen triage*; (b) relay accept only; or (c) accept it as it stands and state the consequence in SD-11 and the DEC-006 note.

## 3. Smaller findings (should fix)

- **R19-6 — Slugs and gaps.** A feature slugged `bug-007` makes creating BUG-007 on that initiative fail, and every failed creation skips a number. Refuse `bug-<digits>` slugs for features, run every refusal before minting, and say gaps are accepted.
- **R19-7 — Audit rows and the timeline.** Say which `ref_type` the bug rows use (`'defect'` is unused), whether `CreateBug` writes `feature.created` (the timeline renders it as "Created"), and whether a rejection's "Abandoned" is a second moment. There is no `i-bug` icon.
- **R19-8 — Milestones.** `member_type` is a `ref_type` limited to four values; say "bug" is a surface name stored as `feature`, and what `get_milestone` reports. A done bug can now satisfy G4: record it. SD-13's reason is wrong — shipping freezes the record; the honest reason is that a *live* milestone would grow with every accepted bug. SD-13 reads DESIGN-010 §6 narrowly and needs a note there.
- **R19-9 — Bug IDs.** `featureByRef` (`identity.go:976`) and `urlForID` (`ui_identity.go:44-57`) don't accept `BUG-`. Say which existing feature tools accept a bug.
- **R19-10 — The agent tool.** `toolhost.Context` has no run ID; NFR-7 miscounts the files touched (`toolhost`, `config`, `toolexec.go`). Authors and document reviewers are never offered profile tools, so `report_bug` declared on one is silently ignored; the loader should refuse it. Define title matching, whether the cap is per dispatch row, and retries.
- **R19-11 — Filing inside the approval transaction** ties a file write to task approval; a disk or git error would stall the task. `ApproveTaskCode` carries no run ID. File after the approval commits, best effort.
- **R19-12 — Send, Withdraw and the send screen.** State the order of the mark and the report's submission and what happens if the second fails. Withdraw checks only authoring runs, so a bug whose report review is queued can be withdrawn and the review runs anyway. Define the send screen's steps for a bug. Say whether routing for `review-spec` applies to `review-bug_report`.
- **R19-13 — Prompts.** A `write-spec` revision of a report is told the document translates a design, and the spec author's anti-patterns say the same. The prompt's rule list doesn't describe `contains_text`. Leave the designs out for a bug, word the heading by kind, and describe the rule — or say the skill overrides the role.
- **R19-14 — `contains_text`** needs a definition (substring or list item; case; line breaks) and a line in DESIGN-004. A substring match lets "The defect no longer reproduces on Safari" count.
- **R19-15 — FR-1.6 is incomplete**: the home page's features by state (`ui_service.go:114`), the initiative estimate roll-up (`estimates.go:212`), `sentBuildersOf` (`edit.go:629`), milestone candidates, and `GET /api/features`. "Open" should be `state NOT IN ('done', 'abandoned')`.
- **R19-16 — SD-4** says the starter pack has a `report` type; it doesn't. `report` is a value of the `document_type` enum.
- **R19-17 — Citations and notes.** Reporting as authoring rests on DEC-004's own "create … features (as ideas)", not Amendment 1. DoD 9 also needs notes for DESIGN-010 §6 and §7, DEC-004 (G4) and DESIGN-004. *Duplicate* is a third outcome beyond §9's accept and reject, and triage on the bug's page goes beyond "Triage happens in the queue"; list both.
- **R19-18 — "A count on the inbox"** reads most naturally as the Inbox's navigation item. Consider a second, neutral count there. Say that reporting and triage fire `notifyEntityChanged`.
- **R19-19 — Prose.** "*n* bug report(s) waiting for triage" isn't a sentence; the queue's review-findings line should be one; "Reported by Sam" relies on the configured UI actor being a name.
- **R19-20 — Schema.** Nothing keeps a bug row and its side row together, or makes `duplicate_of` point at a bug. Add a constraint trigger, or state that `CreateBug` is the only writer and test it.

## 4. Checks that passed

- **The chat agent's boundary, apart from R19-5.** `report_bug` creates an idea, as `create_feature` does (DEC-004). `relay_triage` needs a quote, checked in the service and by the database. DESIGN-010 §17a item 6 is the decision DEC-006 Amendment 1 asks for before a new relay. No MCP tool sends a bug, starts one or gives a verdict. DEC-007 holds.
- **The migration.** Gaps in version numbers are allowed (`migrate.go:50-58`), so `0013` before M11's `0012` is safe. `ident_bug_seq` exists. Adding the enum value unused in the same transaction follows `0010`.
- **Task IDs.** `BUG-007-T01` follows once `ident.Parse` accepts it.
- **G5** already counts every row under the initiative.
- **SD-9's claims about the code.** Minors reach the rules only on approval; a person's approval of an escalated review carries none. Minors raised on a send-back round that the implementer doesn't address are never filed.
- **SD-8.** The reviewer's skill comes from its role, so a section in `review-spec` reaches reviews of reports. Review health groups by role and model, so `review-bug_report` appears under the spec reviewer.
- **The navigation** placement in FR-2.4 is accurate.
- **Coverage of the brief.** Every M12 bullet, and decision 12, has a home.

## 5. The choices

- **SD-1** is right. The alternative (the report as a `spec` with a template chosen by kind) contradicts DESIGN-010 §4 and §7, which name bug reports as their own kind of document.
- **SD-6** changes SPEC-017 FR-5.2's "nobody submits an unsubmitted draft" for bugs; say so.
- **SD-7** is fine, given R19-13.
- **SD-9:** R19-2(a) is a better alternative. Offer both.
- **SD-10:** three is a guess.
- **SD-12:** see R19-18. **SD-13:** right, on the wrong reason.

**Decisions taken without saying so:** whether a relayed rejection can be undone (R19-5); where the fix for a defect in unmerged work is built (R19-2); whether a reported bug can be abandoned (R19-4); the third outcome, *duplicate* (R19-17); which feature tools accept a bug (R19-9).

**Summary:** 20 findings, 5 material (R19-1 to R19-5) and 15 minor (R19-6 to R19-20).

## 6. Disposition

By the author, after the review. Every finding was accepted; the spec's §7
says what changed for each, and the code follows the revised spec.

| Finding | Disposition |
|---|---|
| R19-1 | Fixed. FR-4.0 states the rule; Appendix A lists every site. |
| R19-2 | Fixed, both ways: minor findings are filed when the origin merges (SD-9, choice 9), and any bug whose origin is being built waits for it to merge (FR-4.1, choice 15). |
| R19-3 | Fixed: a Summary (generated when not given), Steps made a list, literal text kept literal, validation at creation. |
| R19-4 | Fixed in the service: every document a bug owns waits for acceptance; Abandon refused while reported; an ownership rule for `spec` and `bug_report`; the queue reads `idea` rows only. |
| R19-5 | Choice 16, recommendation (c), stated in SD-11. |
| R19-6 to R19-20 | Fixed as §7 of the spec records. |
