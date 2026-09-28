# REVIEW-014: Consistency Review of SPEC-014 (checklists and jobs)

**Status:** Complete. The author has dealt with every finding (§6, and
[SPEC-014 §7](../specs/SPEC-014-checklists-and-jobs.md#7-changes-after-review)).
SPEC-014 waits on Sam's approval.
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author. Approval is
Sam's, recorded in §5.
**Scope:** [SPEC-014](../specs/SPEC-014-checklists-and-jobs.md) (draft, commit
`1a2ac54`), checked against:

- [DESIGN-010](../design/DESIGN-010-subutai.md) §4, §5c, §6 and §17a item 4
- [DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1,
  decision 8
- [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) Amendment 1
- [SPEC-010](../specs/SPEC-010-milestones-and-roadmaps-editing.md) and the
  [M4 handoff](../notes/handoff-M4-2026-09-28.md)
- the [status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
  §11 (M5) and §12 (decision 8)
- [DESIGN-008](../design/DESIGN-008-the-workflow-surface.md) D-9 to D-12
- the [writing guide](../research/writing-guide.md)
- the code: the store's tracking, owner-read and cost roll-up code, gate G4,
  the migrations, the UI plan and entity handlers and templates, `dialog.js`,
  the MCP facet and relay tools, the phase 3 API and CLI, and the tests the
  spec names.

## 1. What this review is

This is an authoring consistency pass by a reviewer who did not write the spec.
It checks four things:

- that SPEC-014 agrees with the approved documents it builds on;
- that what it says about the code is true;
- that it covers the M5 brief;
- that its requirements can be tested.

It does not fix the spec.

**A note on timing.** The build was running while this review was written. The
working tree already holds `0009_checklists_and_jobs.sql` and
`internal/store/checklists.go`, and edits to `tracking.go`, `owner_reads.go`,
`gates.go` and some tests. They changed between the reviewer's reads. The
findings below are about the spec. Line numbers are at `HEAD` unless marked as
the working tree. Where the build already does better than the spec says, the
finding says so, so the author can write it into the spec.

## 2. Findings

### R14-1 — After shipping, a checklist can move backwards, which features can't; SD-4 and Sam's choice 7 describe only the forward direction (material)

**What is wrong.** SD-4 says a job ticked after shipping moves a shipped
milestone's progress on, "exactly as a feature finished later does"
(spec:148-152). It also says a checklist in a shipped milestone "isn't frozen:
jobs can still be ticked, added and removed". The two cases are not alike:

- **A feature's `done` state is terminal.** `FeatureState.Terminal`
  (`internal/lifecycle/feature.go:19-21`) means a shipped milestone's progress
  over features can only go up.
- **A checklist's `done` state can go back.** It stops being done when:
  - a job is unticked (FR-4, FR-7.9);
  - a job is added (FR-5.3, and `add_job` from the chat agent, which needs no
    quote);
  - its last job is removed (SD-9).

`SnapshotProgress` reads each leaf's current state. So a milestone shipped at
"2 of 2 items done" can later read "1 of 2", and nothing marks the change. The
snapshot stores only the checklist's id, not its jobs or whether it was done
(FR-2.5). The `not_shipped` list names only the checklists that were not done.
Nothing records which jobs made a done checklist done when it shipped.

This conflicts with DESIGN-010 §4 and §6. There, shipping "freezes a record of
what shipped, so the record stays honest", and "reports against that fixed list
from then on". It also means an authoring tool (`add_job`), used by the chat
agent without quoted words, can change the record of a shipped release. That
cuts against the reason DEC-004 Amendment 1 gave for letting the agent touch
shipping at all.

Sam's choice 7 (spec:527-529) presents only the harmless direction.

**Recommended fix.** Put both directions to Sam and pick one:

- **(a) Freeze a shipped checklist's done state.** While any shipped milestone
  holds the checklist, refuse an untick, an add or a remove, with a sentence
  that says to reopen the milestone first.
- **(b) Record the done state at shipping.** An additive column
  `milestone_snapshots.done_at_lock`, or the job ids and ticks in the
  `milestone.locked` payload. The shipped view then reports the recorded state.
- **(c) Accept the regression.** Say so in SD-4. Stop saying "exactly as a
  feature", and reword choice 7 to include it.

Whichever is chosen, add an acceptance criterion for it. For example: a job
added to a done checklist in a shipped milestone does, or doesn't, change that
milestone's count.

### R14-2 — `rename_job` on a ticked job lets an authoring tool effectively tick (material)

**What is wrong.** SD-7 says only `relay_tick_job` ticks, and "None of them can
tick" (spec:175-180). That is true of the tick columns. It is not true in
effect.

- `rename_job` (FR-7.3) and `EditJob` (FR-1.3) can change the title of a
  ticked job. The person's tick then vouches for work they never ticked.
- The same route gives the chat agent a two-step tick with no quote:
  1. Rename any ticked job to the title of an unticked one.
  2. Remove the unticked original with `remove_job`.
- `rename_job` with `note` also overwrites the note given with a human tick,
  including a relayed tick's context. The page then shows the agent's text
  beside the person's quote (SD-6, SD-8).

The audit trail keeps the history. But the checklist page, the "X of Y" count
and G4 all read the job row, not the history.

**Recommended fix.** Pick one of these:

- Refuse a title change on a ticked job from every surface ("Untick it first,
  or add a new job"). This is the simplest.
- Make a title change untick the job, with its own audit row.
- Keep the title at tick time (`ticked_title`) and show "ticked as ..." when it
  differs.

Then restate SD-7 so that no authoring tool can change what a tick vouches for.
Add acceptance criteria: renaming a ticked job over MCP is refused, or unticks
it; and a tick's note isn't overwritten by `rename_job` without trace.

### R14-3 — SD-7's authority and alternatives are presented unfairly (material)

**What is wrong.**

1. **The written authority is narrower than the spec claims.** SD-7 rests
   removal and rename on DEC-004 and §17a item 4 (spec:177-179). But:
   - §17a item 4 says the chat agent "may create checklists and jobs";
   - DESIGN-010 §6 says it can "create and fill" checklists;
   - DEC-004 Amendment 1 names milestones and roadmaps only.

   No accepted document says the chat agent may remove or rename a job. The
   spec's authority line (spec:18-21) says §5c and §17a "put checklists and
   jobs with them", which reads more into them than they say.
2. **The analogy is weak.** SD-7 compares removing unticked jobs to
   `remove_milestone_member`. But DEC-006 Amendment 1 made the tick itself a
   quoted relay act, and no such status applies to a milestone's
   completeness. Removing every unticked job from a checklist that has one
   tick gives exactly the observable result the relay rule protects: the
   checklist done, the milestone's count up, and G4 satisfied.
3. **The choice has only two sides.** It offers "planning" or "a new relay".
   Middle options exist:
   - refuse `remove_job` over MCP when it would leave the checklist done;
   - refuse MCP removal of an unticked job from a checklist that is in an open
     milestone;
   - allow removal but not of the last unticked job.

**Recommended fix.** Say plainly that job removal and rename by the chat agent
extend §17a item 4, and need Sam's decision, recorded as a dated note on
DEC-004 Amendment 1 or DESIGN-010 §5c. Present at least one middle option in
SD-7 and in choice 5. With R14-2 fixed, rename becomes safe and needs only a
mention.

### R14-4 — Relaying an untick isn't on DEC-006 Amendment 1's list, and the spec doesn't notice (material)

**What is wrong.** DEC-006 Amendment 1, decision 8, lists "a ticked job"
(DEC-006:338), and its Consequences list "tick a job" (DEC-006:387-392). It
also says "Adding a relay tool that isn't on this list needs a decision"
(DEC-006:348). DESIGN-010 §5c and §6 say the same: "a ticked job".

No approved document mentions relaying an untick. FR-7.9 has `relay_tick_job`
carry both (`ticked: false`), and the spec treats that as covered. The brief
asked for both, but the brief says the document wins where they conflict. The
point isn't among the choices put to Sam.

**Recommended fix.** Add an SD that states the reading: an untick corrects a
tick; it touches the same record, starts nothing and can be undone, so it
passes the Amendment's consequence test. Add it to "For Sam to confirm", with a
dated note on DEC-006 if he agrees. The alternative is to leave unticking to
the web UI and make the relay tick-only.

### R14-5 — Sentences that count "features" survive in places the spec doesn't list, and the approved SPEC-010 text it overrides isn't named (material)

**What is wrong.** FR-2.3 makes `Progress.Total` and `Progress.Done` count
items. Several human-facing sentences take those numbers and still say
"features". The spec doesn't list them:

- **`plan.html:316`** (SPEC-010 FR-5.2), the Mark as shipped explanation:
  "That records the {{.LeafCount}} features it covers ({{.LeafDone}} of them
  done so far)". `LeafCount` is `prog.Total` (`ui_plan_actions.go:151`), so it
  will count checklists as features. The single-item branch says "one feature
  it covers, which is done" even when that item is a checklist.
- **`ui_plan_actions.go:620-622`** (SPEC-010 FR-5.3), the notice after
  shipping: "Its record holds the %d features it covers, %d of them done", or
  "its one feature, which is done", fed by `prog.Total`.
- **`plan.html:289`**, a shipped milestone's editor: "Progress is measured
  against the features it covered when it shipped."
- **`mcp_plan_tools.go:587-604`**, the `add_milestone_member` and
  `remove_milestone_member` descriptions and `member_type` hints. They name
  feature, initiative and milestone only. FR-7.8 says the tools accept
  `checklist`, but not that their text changes.
- **`mcp_plan_tools.go:102`** and **`ui_plan_actions.go:526`**, refusals that
  list the member types allowed. The build has since updated the second one.

These are SPEC-010 FR-5.1 to FR-5.3 requirements, in an approved spec, and
SPEC-014 changes them (G4's sentences in FR-2.4, and implicitly the two
above). DESIGN-010 §4 says an approved document is changed by a successor, not
edited silently.

**Recommended fix.**

- Add an FR, or extend FR-2.4, listing every sentence above with its new
  wording. Use "items", matching the progress line and G4.
- Add a short "Changes to SPEC-010" table naming FR-5.1, FR-5.2 and FR-5.3.
- Add a render or integration assertion that the Mark as shipped explanation
  for a milestone with one checklist says nothing about features.

### R14-6 — FR-2.6 can't be built without editing `entity.html` beyond includes, which NFR-9 forbids (material)

**What is wrong.** FR-2.6 requires two things:

- a checklist row on the milestone **page** with "3 of 5 jobs ticked" beside
  it;
- a link to `/ui/c/{id}`.

The milestone page's member rows are inline markup in `entity.html` (the
`page-milestone` template, `entity.html:681-694`), not an include. Its
right-hand slot prints `{{.Kind}}` (`entity.html:692`), and `.Kind` also picks
the icon and `data-entity`. Adding the jobs count means editing that markup.
NFR-9 (spec:491-494) and the brief limit `entity.html` changes to includes,
because M8 shares the file.

A second, smaller gap: the handoff says New milestone and New roadmap also sit
in the owner page's "more" menu (`entity.html:344-347`). The spec doesn't say
whether New checklist goes there. It can't, under NFR-9, and the spec should
say that on purpose.

**Recommended fix.** Choose one:

- Move the milestone member row into a `milestone-member-row` partial in
  `plan.html`. That replaces the inline markup with an include, as a one-off
  agreed with the M8 session, and the spec should say so.
- Drop the jobs count from the milestone page, keep it in the editor
  (`plan.html`), and say so.

Either way, state in FR-3.2 that New checklist is not added to the "more" menu.

### R14-7 — Half of M5's "done when" is missing, and SD-3 loosens G4 more than it admits (material)

**What is wrong.**

1. **The Goal drops the shipping half.** The roadmap's M5 "done when" is: "a
   milestone won't show as done, **and can't be marked shipped as complete**,
   while one of its jobs is unticked" (roadmap:657-658). The spec's single
   Goal claim (spec:64-67) keeps only the first half. The DoD walkthrough
   (spec:500-505) never ships a milestone with an unticked checklist. So
   nothing checks how the second half is met: `not_shipped`, SD-4.
2. **SD-3's premise is wrong.** SD-3 says counting checklists "changes what
   [G4] counts, not how strict it is" (spec:130-132). It makes G4 less strict.
   A milestone with four unfinished features could not be marked as shipped
   before. With one done checklist ("Buy the domain") it now can, with no code
   done. DEC-004 Amendment 1 handed `mark_milestone_shipped` to the chat agent
   partly on the strength of G4 as then described ("none of its features is
   done"). A relayed tick now satisfies it.
3. **The choice has only two sides.** It offers "items" or "features only".
   There is at least one middle option: count checklists only when the
   milestone resolves to no features. That still lets "Launch paperwork" ship.

**Recommended fix.**

- Add the shipping half to the Goal. Map it to SD-4's `not_shipped`, or, if
  Sam reads "as complete" more strictly, to a refusal.
- Add a DoD step: ship a milestone with a done feature and an unticked
  checklist, and show the checklist in `not_shipped`.
- Correct SD-3's claim, and add the middle option to choice 1.

### R14-8 — Stale pages, removed jobs and ambiguous references aren't specified (material)

**What is wrong.**

- **A removed job strands the page.** The tick form posts only `job_id` and
  `tick` (FR-4.2), and the editor forms post only `job_id` (FR-5.3). §6
  (spec:535-536) admits that pages don't refresh when the chat agent changes
  them, so a stale page is the normal case. If the chat agent removes a job
  and the person then ticks it on an open page, `GetJob` returns
  `ErrNotFound`. The handler has no checklist id to re-render, so the spec
  doesn't say what happens. It will be a not-found page or an error, not a
  sentence.
- **The editor route has no error case.** FR-5.1 doesn't say what an unknown
  id does on `GET /ui/c/{id}/edit`.
- **A job id isn't checked against its checklist.** FR-7 names a job "by id,
  or by its exact title within the named checklist". It doesn't require the
  id's job to belong to that checklist. `relay_tick_job` could then tick a job
  on another checklist while the quote talks about this one.
- **Ambiguous titles aren't handled.** Duplicate job titles are allowed, since
  there is no unique constraint in FR-1.1. The spec defines the ambiguity
  error for checklist names (FR-1.2) but not for job titles. The build has
  `JobByTitle`, which refuses with a count, but the spec should require it,
  especially for a relay. A relay must never tick "the first of two".

**Recommended fix.**

- Carry `checklist_id` in every job form, and check that the job belongs to
  it.
- When the job no longer exists, re-render the checklist page with a sentence
  ("That job is no longer on this checklist; the page has been brought up to
  date").
- Say that an unknown id on the editor route returns not-found.
- Require that a job named by id belongs to the named checklist.
- Require an ambiguous job title to be refused with the count and a pointer
  to the id.
- Add acceptance criteria for each.

### R14-9 — SD-2 leaves out the design text that argues the other way (minor)

**What is wrong.** SD-2 cites D-12 (spec:117-118). But D-12 is about ownership
not *limiting* what a milestone may contain. It says nothing about what an
initiative member *resolves to*. The design text that bears on it points the
other way:

- DESIGN-010 §4 defines an initiative deliverable as "meaning everything under
  it";
- §6 says checklists "belong to" initiatives;
- the purpose of checklists is that "a release can't be called done while the
  human chores are still open".

A reader of SD-2 sees only the downside of the alternative.

**Recommended fix.** Quote §4 and §6 in SD-2. Say that "everything under it"
is being read as the work tree (features), not the plan (milestones,
roadmaps, checklists). Keep the recommendation, but let Sam see what it
departs from.

### R14-10 — Concurrency and integrity rules the build relies on aren't in the spec (minor)

**What is wrong.** FR-1.3 requires dense positions and says a double tick is
refused. It doesn't say how either is made safe when two writers act at once.
The build does it by taking a `FOR UPDATE` lock on the checklist row
(`lockChecklist`) and on the job row in `TickJob` (working tree,
`checklists.go`). The spec should require this, so a later change can't drop
it.

FR-1.1's CHECKs also leave some invariants to code:

- `ticked_via` and `ticked_quote` can be set while `ticked_at` is null;
- a `mcp` tick can have no quote.

`TickJob` accepts `via: "mcp"` with a blank quote. Only the tool refuses it.

**Recommended fix.**

- Add to FR-1.3: "Every job write locks the checklist row. `TickJob` locks the
  job row, so concurrent ticks can't both succeed and positions can't
  collide."
- Add CHECKs: `ticked_via` and `ticked_quote` are null unless `ticked_at` is
  set, and `ticked_via = 'mcp'` implies `ticked_quote IS NOT NULL`. Or say why
  not.
- Have `TickJob` refuse `via: mcp` without a quote, so the rule holds below
  the tool too.

### R14-11 — The audit and activity feed need more than the spec says (minor)

**What is wrong.**

- **`eventLabel` has no case for the new kinds** (`ui.go:307-389`):
  `checklist.created` and `job.added`, `.edited`, `.moved`, `.removed`,
  `.ticked` and `.unticked`. The default case prints fragments such as "job
  ticked", which breaks D-6.
- **FR-1.2 audits `checklist.created` against the checklist.** It will not
  appear in the owner's activity. `roadmap.created`, by contrast, is filed
  against its owner so that it does appear (`tracking.go` `CreateRoadmap`).
  The spec should pick one on purpose.
- **The `milestone.locked` row will mix units** (FR-2.5). `leaves` stays a
  feature count, but `done_at_lock` becomes an item count. Say which is which,
  or add `features_done`.
- **FR-2.5 says `UnlockMilestone` keeps `snapshot` "beside"
  `snapshot_checklists`.** At `HEAD` its query reads every `leaf_type`
  (`tracking.go:450-451`). The spec must say `snapshot` becomes features only,
  as the build does.

**Recommended fix.** Add the `eventLabel` sentences to FR-1 or NFR-6, and add
a test that the activity feed renders them. Settle the audit target for
`checklist.created`. Define the `milestone.locked` fields.

### R14-12 — Other readers of milestones the spec doesn't mention (minor)

**What is wrong.**

- **The dashboard shows the project's own plan** (`dashboard.html:124-147`),
  milestones and roadmaps. Project-owned checklists won't appear there.
- **The API and CLI.** `GET /api/milestone` and `subutai milestone show` will
  count checklists in `total` and `done` (`http_phase3.go:342`,
  `cmd/subutai/main.go:795-796`). But `POST /api/milestones/members` can't add
  one, because `resolveMemberRef` handles features, initiatives and
  milestones only (`http_phase3.go:272-283`). That is acceptable, since the
  CLI is being retired, but the spec should say so.
- **`get_milestone` builds a member's `path`** by stripping `/ui/f/` and
  `/ui/i/` from the row's URL (`mcp_plan_tools.go:453`). A checklist row
  would report `path: "/ui/c/<uuid>"` unless it is handled. FR-2.6 asks for
  the id, but should name this code.

**Recommended fix.** Add one line each to scope (in or out) and to FR-2.6.

### R14-13 — Small factual errors and prose slips (minor)

- **FR-1.1's acceptance criterion names `TestMigrations`, which doesn't
  exist.** The migration tests are `TestMigrateFromEmpty` and its neighbours
  in `internal/store/migrate_test.go`.
- **FR-7.10 says "SPEC-010's twenty-five".** The 25 are SPEC-008's nine,
  SPEC-010's twelve and SPEC-011's four
  (`integration_mcp_test.go:101-135`).
- **FR-1.3 names `ChecklistStatus`.** The build has `GetChecklistStatus` and a
  `ChecklistStatus` type. Align them.
- **The `mcp_relay.go` header is already stale.** It says no tool "locks a
  milestone" (`mcp_relay.go:8-14`), but `mark_milestone_shipped` does. FR-7.9
  says the header gains `relay_tick_job`. It should also correct that
  sentence.
- **FR-7.2 conflicts with the "one audit row" acceptance criterion.**
  `add_job` with `position` implies `AddJob` followed by `PlaceJob`, which
  writes two audit rows (`job.added`, `job.moved`), while FR-7's acceptance
  criterion reads as one per write. Say which.
- **SD-1's arithmetic is wrong.** "A ten-job checklist outweigh[s] ten
  features" is false: at ten each they weigh the same. Something like "a
  ten-job checklist would outweigh three features" is meant.
- **FR-7.8 uses a third word.** The `mark_milestone_shipped` description says
  "at least one thing in it". Use "item", the word G4 and the progress line
  use (writing guide, "One word, one meaning").
- **SD-8 shows the raw actor id.** "Ticked by chat-agent, relaying: ..."
  displays the configured actor name. If the MCP actor is configured, it will
  be whatever string was configured. Say that the page shows the actor as
  stored, or give a label such as "the chat agent".

## 3. Checks that passed

- **The brief is covered.** Every item in the M5 brief appears:
  - the migration;
  - ticking and unticking, audited;
  - done means at least one job and every job ticked;
  - counted as an item, not in the token bar;
  - `not_shipped` on shipping;
  - the G4 decision recorded;
  - New checklist, `/ui/c/{id}`, Add to a milestone… and the picker branch;
  - seven authoring tools and one relay, with the tool-set test updated.

  Deferring add, rename, reorder and remove to a modal (SD-5) departs from the
  brief's wording. It follows DESIGN-010 §6's "edited in a modal from its
  owner's page", and it is put to Sam.
- **The constraint names are right.** `0004` declares both CHECKs inline and
  unnamed. Postgres names them `milestone_members_member_type_check` and
  `milestone_snapshots_leaf_type_check`, as FR-1.1 says. `ref_type` already
  holds `checklist` and `job` (`0001_init.sql:11`).
- **Keeping `Progress.Leaves` as features is the right call.** The token bar
  (`ui_entity.go:275`), `MilestoneCost` (`http_phase3.go:340`), the cost
  roll-up endpoint (`http_phase3.go:460-466`) and `RoadmapCost`
  (`cost_rollups.go:79-85`) all read `Leaves` or `ResolveMembers`, and stay
  correct unchanged. `ResolveMembers` keeping its signature protects them.
- **G4 changes in one place.** The editor preview (`ui_plan_actions.go:146-150`),
  `get_milestone`'s `shipping` block (`mcp_plan_tools.go:459-463`), the API
  lock and `LockMilestone` all get their counts from `LiveProgress`. Changing
  `Progress` changes all of them together, and none is left counting only
  features.
- **`AddMember` and `RemoveMember` need no store change**, as FR-6.2 says.
  They are type-agnostic. The callers' type switches (`memberFromForm`,
  `mcpMemberArg`) do need a checklist case. FR-6.2 and FR-7.8 imply this, and
  the build has started on it.
- **`dialog.js` works on attributes, not ids** (`data-autoshow`,
  `data-refresh-on-close`, `data-changed`). So NFR-1's "needs no change"
  holds, provided the checklist editor copies the milestone editor's markup.
- **The relay pattern fits the existing relays.** `relayAct.payload` (`via`,
  `quote`) in `review_send.go:28-43` is the right model for `job.ticked`. The
  missing-quote refusal sentence in `relayTarget` can be reused.
- **The prose is British and in full sentences.** No American spellings were
  found. Notices, gate reasons and empty states are complete sentences.

## 4. Notes carried into implementation (not blocking)

- **The chat agent can create clutter it can't remove.** R10-1's point about
  undeletable creations now extends to checklists. A chat-created empty
  checklist added to a milestone keeps that milestone short of complete (SD-9)
  until someone takes it out. The rename and delete follow-up grows more
  urgent.
- **Nested milestones read "done" in their parent when shipped** (spec §6).
  With R14-1 unresolved, the parent's row can say done while the shipped
  child's checklist has since been unticked.
- **Existing tests and docs quote the old G4 sentences.** `TestG4`,
  `integration_plan_test.go`, `ui_plan_render_test.go` and
  `docs/walkthrough-spec-010.md` do. The build is updating the tests. The
  walkthrough is a record of what happened, so it should be left as it is.
- **Migration version.** The working-tree migration says 0010 belongs to M8.
  The spec should say so too, so the two lines don't collide.

## 5. Recommendation and decision

The spec is well aimed and mostly accurate about the code. The progress and G4
design in particular is sound, and cheap to build. It isn't ready to approve
as written. Eight findings are material:

- **R14-1:** a shipped checklist's state can go backwards, which the spec
  presents as parity with features.
- **R14-2:** `rename_job` lets the chat agent effectively tick without a
  quote.
- **R14-3:** removal and rename by the agent go beyond the written authority,
  and the choice is presented with only two sides.
- **R14-4:** relaying an untick isn't on DEC-006's list.
- **R14-5:** "features" sentences remain in the shipping UI and tools, and
  the changes to SPEC-010 aren't named.
- **R14-6:** FR-2.6 needs a non-include edit to `entity.html`.
- **R14-7:** M5's "done when" is only half covered, and SD-3's "not stricter"
  claim is wrong.
- **R14-8:** stale-page and job-reference errors are unspecified.

All of them can be fixed in the spec and a small amount of code. None needs a
new design.

Decisions for Sam once the findings are dealt with, beyond the seven the spec
already lists:

1. **What a shipped record says about a checklist that changes later**
   (R14-1: freeze, record, or accept).
2. **Whether the chat agent may remove and rename jobs**, and with which
   limits (R14-2, R14-3).
3. **Whether relaying an untick is inside DEC-006 Amendment 1** (R14-4).
4. **G4's counting rule**, with the "features first" middle option (R14-7).

**Recommended for approval once R14-1 to R14-8 are fixed.** The reviewer does
not approve.

_Decision (Sam): pending._

## 6. How the findings were dealt with

_Added by the spec's author, not the reviewer. The findings above are left as
the reviewer wrote them._

Every finding was accepted. The spec's own
[§7](../specs/SPEC-014-checklists-and-jobs.md#7-changes-after-review) records
the change made for each one. In short:

- **R14-1:** option (b). The snapshot records whether each checklist was done
  when the milestone shipped (`milestone_snapshots.done_at_lock`), and the
  shipped record counts it done if it was done then or is done now. A shipped
  record can't go backwards, exactly as with features. A job can still be
  ticked, added or removed on the checklist itself.
- **R14-2:** the first option. A ticked job's title can't change, from any
  surface; its note can, and every note change is audited.
- **R14-3:** SD-7 now says plainly that renaming, moving and removing jobs by
  the chat agent go beyond §17a item 4's "create", and asks Sam. It builds a
  middle option: `remove_job` refuses to remove a checklist's last unticked
  job, so the chat agent can't finish a checklist that way.
- **R14-4:** a new SD-10 reads an untick as inside DEC-006 Amendment 1, and
  asks Sam, with the tick-only alternative.
- **R14-5:** FR-2.4 lists every sentence that changed, and a new "Changes to
  SPEC-010" table names SPEC-010 FR-5.1 to FR-5.3 and FR-7.3, FR-7.4 and
  FR-7.10.
- **R14-6:** the second option. The jobs count shows in the milestone editor,
  not on the milestone page, which is left untouched. New checklist is not in
  the "more" menu, on purpose.
- **R14-7:** the Goal has the shipping half, a DoD step ships a milestone with
  an unticked checklist, SD-3 now says it loosens G4, and the "features first"
  option is put to Sam.
- **R14-8:** every job form carries `checklist_id`; a job that has gone, or is
  on another checklist, is answered on the checklist page with a sentence; an
  unknown editor id is a 404; a job named by id must belong to the named
  checklist; an ambiguous title is refused with the count.
- **R14-9 to R14-13:** SD-2 quotes §4 and §6; the CHECKs and locks are
  required; `TickJob` refuses a chat tick without words; the activity
  sentences, the audit targets and the `milestone.locked` fields are
  specified; the dashboard and the API are named as out of scope; the factual
  slips are corrected.
