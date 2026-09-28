# REVIEW-010: Consistency Review of SPEC-010 (milestones and roadmaps you can edit)

**Status:** Complete. The author has dealt with every finding (§6, and
[SPEC-010 §7](../specs/SPEC-010-milestones-and-roadmaps-editing.md#7-changes-after-review));
**SPEC-010 approved by Sam, 2026-09-28**
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author. Approval is
Sam's, recorded in §5.
**Scope:** [SPEC-010](../specs/SPEC-010-milestones-and-roadmaps-editing.md)
(draft, commit `60c5d61`), against
[DESIGN-008](../design/DESIGN-008-the-workflow-surface.md) (§5.1a, §5.1b, §5.2,
§6, §6a, §9; D-1, D-6, D-8 to D-13),
[DEC-004](../decisions/DEC-004-mcp-planning-authoring.md),
[DEC-005](../decisions/DEC-005-the-orchestration-boundary.md),
[DEC-006](../decisions/DEC-006-humans-start-development.md) with Amendment 1,
the [Stage B entry criteria](../notes/spec-007-stage-b-entry-criteria.md),
[SPEC-006](../specs/SPEC-006-command-centre-mutations.md) and
[REVIEW-006](REVIEW-006-command-centre-mutations-package.md) (the G4 shape),
[SPEC-008](../specs/SPEC-008-mcp-facet-planning-authoring.md) (MCP conventions),
the M4 brief in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md),
and the code: the store, the phase 3 API, the entity pages and templates,
`dialog.js`, the MCP facet and its tests, and gate G4.

## 1. What this review is

An authoring consistency pass by a reviewer who did not write the spec. It
checks that SPEC-010 agrees with the design and decisions it builds on, that
what it says about the code is true, that it answers the Stage B entry
criteria, and that its requirements can be tested. It does not fix the spec;
the author will fold the findings in and record that.

One thing about timing. The store half of the spec was committed while this
review was running (`78aff1c`: owner-scoped creation, `PlaceRoadmapEntry`,
`RemoveRoadmapEntry`, the nesting guard and `MemberCandidates`), and more was
in the working tree. Findings are about the spec. Where the code already
disagrees with the spec, that is noted so the author can decide which one is
right.

## 2. Findings

### R10-1 — "No act that can't be undone" is false, so SD-4's argument rests on a wrong premise (material)

**What is wrong.** The Goal says "the chat agent gains no act that can't be
undone" (spec:65-67). NFR-3 says "The chat agent gains only reversible planning
acts" (spec:412-414). SD-4 says locking "is the one act in this slice that
can't be undone" and "every other act here can be reversed by its opposite"
(spec:129-132). But `create_milestone` and `create_roadmap` (FR-7.1, FR-7.2)
have no opposite. The spec's own out-of-scope table says there is no rename,
re-describe or delete method (spec:92), and §6 says a typo in a milestone's
name "is now fixable only in the database" (spec:450-452). A chat agent that
creates a stray milestone leaves it on an owner's page for good.

**Evidence.** `internal/store/tracking.go` has no delete or update for
milestones or roadmaps; spec:92 and spec:450-452 say the same.

**Recommended fix.** Restate the consequence test honestly. Creation can't be
undone but does little harm: an empty, unreferenced milestone, which is clutter
rather than lost information. Locking can't be undone and does real harm: it
freezes membership and writes a permanent snapshot. Reword the Goal and NFR-3
to "the chat agent gains no act that freezes or destroys planning state", or
something equally accurate. Record in §6 that stray creations can't be removed
until the rename and delete follow-up lands. Also consider whether that
follow-up should come before, not after, handing creation to the chat agent.

### R10-2 — SD-4 cites the wrong line from Amendment 1, misses the stronger grounds, and understates how to overrule it (material)

**What is wrong.**

1. SD-4 says irreversibility is "exactly the line Amendment 1 draws"
   (spec:137-139). It isn't. Amendment 1 asks "how bad and how recoverable a
   misuse would be", and the acts it keeps in the UI are the ones that "either
   spend resources or remove a safeguard" (DEC-006:357-367). A lock does
   neither. It is a new category (irreversible, but it spends nothing and
   unguards nothing), and the spec should say that plainly rather than claim
   the amendment already covers it.
2. SD-4 leaves out the grounds that actually decide this. DEC-004's decision
   line says the facet "may not drive the development pipeline **or touch
   gates**" (DEC-004:16). A lock tool would be the first MCP tool whose act a
   gate (G4) decides. DESIGN-008 D-13 also puts "drives lifecycle" on the web
   UI side (DESIGN-008:501-506, 690-698), and a lock is the milestone's only
   lifecycle transition (`open → locked`, `internal/lifecycle/gates.go:17`).
3. SD-4 says that if Sam wants the tool, "it should be a relay tool carrying
   the human's quoted words" (spec:139-140). That implies approving this spec
   would be enough. It wouldn't. Amendment 1 says "Adding a relay tool that
   isn't on this list needs a decision" (DEC-006:348), and because of the
   DEC-004 "touch gates" line, a lock relay would also narrow DEC-004.

**Recommended fix.** Rewrite SD-4 to rest on DEC-004 "touch gates" and D-13
first, with irreversibility as the consequence argument second. Say that a lock
tool, if wanted, needs its own decision that amends DEC-006 Amendment 1's relay
list and qualifies DEC-004, not just an overrule in this spec's approval.

### R10-3 — SD-6 says it satisfies DESIGN-008 when it departs from it (material)

**What is wrong.** DESIGN-008 §5.1a says "**Removal** is always from the
milestone's modal" (DESIGN-008:290-291). §5.2 says the "milestones it is a
member of" list is "read-only" (DESIGN-008:375-377). SD-6 adds a **Take it
out** button to the member's own dialog (FR-3.1, spec:248-250) and then says
"Both are satisfied" (spec:153). They aren't. The design says *always*, and the
spec adds a second place. The M4 brief does ask for removal from both ends
(roadmap note, M4 bullets), so the change is justified. It is still a
deviation, and it needs Sam's approval rather than an argument that there is no
conflict.

The same is true, to a lesser degree, of FR-6.2's **Edit** button in the
milestone and roadmap page headers (spec:340-342). DESIGN-008 says editing
happens "from the parent entity's page" in §5.1a (273-276), §6 (473-477), §9
(599-600), D-9 (671-672) and Q3 (723-726).

**Recommended fix.** Rewrite SD-6 as a recorded deviation: "DESIGN-008 §5.1a
says removal is only from the milestone's modal. The M4 brief asks for removal
from both ends. This spec adds it on the member's side and asks Sam to accept
the change." Do the same for the Edit button on the milestone and roadmap
pages. List both in the handoff (DoD 6) as decisions for Sam, and add a short
dated note to DESIGN-008 §5.1a once he accepts them, so the design and the
build don't silently disagree.

### R10-4 — G4's reasons are not plain words, and FR-5.1's example counts the wrong thing (material)

**What is wrong.** FR-5.1 and FR-5.3 promise G4's reason "in plain words"
(spec:313-316, 326-328). FR-7.8 hands "G4's reason" to the chat agent to relay
(spec:392-394). NFR-6 requires every gate reason to be human prose (spec:423).
The reasons G4 actually returns are terse fragments: "milestone has no resolved
members" and "no member done yet (3 resolved, none complete)"
(`internal/lifecycle/gates.go:105-115`). The spec never says where the plain
words come from: rewriting G4 (which also changes the API's 409 body and the
`gate.evaluated` audit payload) or a mapping in the UI (which FR-7.8 would then
bypass). The example also says "none of its 3 deliverables is done"
(spec:315). G4 counts *resolved leaf features*, not deliverables
(`tracking.go` `LockMilestone` → `LiveProgress` → `ResolveMembers`). A
milestone with one initiative member that covers three features has one
deliverable and three resolved members. FR-5.2 counts features correctly ("the
N features this milestone currently resolves to", spec:320), so FR-5.1 and
FR-5.2 disagree with each other. The empty-milestone case, which G4 reports
separately, has no wording at all.

**Recommended fix.** Rewrite G4's two refusal reasons in
`internal/lifecycle/gates.go` as full sentences that count features. For
example: "This milestone can't be locked yet, because none of the 3 features it
covers is finished." and "This milestone can't be locked yet, because nothing
has been added to it." That way the UI, MCP, API and audit trail all say the
same thing. Add an AC for both cases, and correct the FR-5.1 example.

### R10-5 — The HTMX-or-plain-post split doesn't survive `hx-boost`, and the "works without HTMX" promise isn't true of the editor (material)

**What is wrong.**

1. SD-9 and FR-6.5 say a handler returns the editor fragment "under HTMX" and
   the owner's page "without it" (spec:177-182, 353-355). Every page's `<body>`
   has `hx-boost="true"` (`internal/server/ui/templates/partials.html:26`), so
   the "plain" forms are sent by HTMX too, with `HX-Request: true`. The
   member-side dialog (FR-3.1) posts to the same `/ui/milestone/member/add` and
   `/remove` routes as the editor. If the handler checks `HX-Request`, a
   boosted member-side post gets back the editor fragment, and boost swaps that
   fragment in as the whole page body.
2. FR-6.5 says the fragment routes "return a usable page-less fragment" without
   HTMX. The fragment is a `<dialog>` with no `open` attribute (FR-6.1), so it
   doesn't render at all unless script calls `showModal()`. The Edit button is
   an `hx-get` (FR-6.2), so without script it does nothing. The picker's
   project-wide search (FR-3.2) has no non-HTMX path either. So SD-9's "nothing
   breaks if the script fails to load" is untrue for the editor, which is the
   heart of the slice.

**Recommended fix.** Choose the response by an explicit signal: `HX-Target`
equal to the editor's `id`, or the `from=member` field FR-3.1 already carries.
Don't use `HX-Request`. Add an AC that a boosted member-side post re-renders
the member's page. Then either drop the no-script claim for the editor and say
it needs HTMX, as the inbox and dashboard already do, or make
`GET /ui/m/{id}/edit` without HTMX return the full milestone page with the
editor open inline. Whichever you choose, make FR-6's AC test it.

### R10-6 — NFR-4 says the existing test covers the new forms; it doesn't (material)

**What is wrong.** NFR-4 says the new field names are chosen "so
`TestUIBrowseAndDrive`'s banned-name check covers them" (spec:417-418). FR-3's
AC says the no-typed-path rule "holds on every page and fragment"
(spec:288-289). The check only scans `/ui`, `/ui/project`, `/ui/i/auth`,
`/ui/f/auth/login` and `/ui/work`
(`internal/server/integration_ui_test.go:154-163`). It never loads a milestone
page, a roadmap page or either new `/edit` fragment, and those are where the
member picker, the search box and the roadmap placer live. As things stand,
the "what must stay true" rule from the entry criteria would not be enforced
where this slice adds forms.

**Recommended fix.** Make NFR-4 require that the banned-name loop is extended
to `/ui/m/{id}`, `/ui/r/{id}`, `/ui/m/{id}/edit` and `/ui/r/{id}/edit` in a
seeded state where the picker and placer have options, and put that in FR-3's
and FR-6's ACs.

### R10-7 — SD-5 reaches the right answer on thin evidence; the widening should be put to Sam (minor)

**What is wrong.** None of the ten tools is a relay. None carries a human's
verdict, issue or tick, and none starts, spends or gates anything. So SD-5 is
right that DEC-006 Amendment 1's relay rules don't apply (see §3). But its
grounds are weaker than it suggests.

- DEC-004's list of what the facet may do names initiatives, features, titles,
  descriptions and documents (DEC-004:20-25). DEC-005 repeats that list
  (DEC-005:131-133), and so does D-13 (DESIGN-008:690-698). None of them names
  milestones or roadmaps. The code files them under "tracking" (`tracking.go`,
  `0004_planning_and_tracking.sql`).
- "DESIGN-008 §6 lists them with the planning actions" (spec:144-145) is not
  evidence. The §6 table lists every action, including Start work and Archive.

DEC-005 asks that widening the facet always be deliberate (DEC-005:228-233).
The test is how that happens mechanically, but the judgement belongs to Sam.

**Recommended fix.** Keep the conclusion. Replace the §6 citation with the
actual argument: milestones and roadmaps are what DEC-004's headline calls "the
planning layer", and nothing in them crosses the planning/developing seam.
Then list "confirm milestones and roadmaps are planning layer under DEC-004" as
an explicit decision for Sam in the handoff (DoD 6), next to SD-4.

### R10-8 — `owner_path: "project"` is ambiguous and breaks SPEC-008's owner convention (minor)

**What is wrong.** FR-7.7 uses `owner_path` with the string `"project"` to mean
the project (spec:384-386). FR-7.1 and FR-7.2 mean the project by leaving
`owner_path` out (spec:370-372). A top-level initiative whose slug is
`project` would collide with the first form. SPEC-008's tools use
`owner_type` + `owner_path` (`attach_document` and `list_documents`,
`internal/server/mcp.go` tool schemas; `mcpResolveOwner`,
`internal/server/mcp_tools.go:252`).

**Recommended fix.** Use `owner_type` (`project` | `initiative`) plus
`owner_path` in FR-7.1, FR-7.2 and FR-7.7, resolved with the existing
`mcpResolveOwner`. For `list_*`, leaving both out means "every one".

### R10-9 — The milestone page has none of the chrome FR-3.1 assumes, and SD-8 undercounts what the member dialog needs (minor)

**What is wrong.** FR-3.1 puts **Milestones…** in the "more" menu of milestone
pages and re-renders "the member's page" (spec:245-258). But `page-milestone`
has no rail and no `entity-menu` (`entity.html:629-692`), and `renderEntity`
has no milestone case (`internal/server/ui_entity_actions.go:27-62`). SD-8 says
the member-side dialog needs "no server state beyond what the page already
has" (spec:173-175). In fact every feature, initiative and milestone page would
need a new read: all open milestones, grouped by owner, minus the ones the
entity is already in and, for a milestone, the ones that contain it. Today the
page only carries `MemberOf`.

**Recommended fix.** Say that the milestone page gains a "more" menu, and that
`renderEntity` gains milestone (and roadmap, for FR-6.5) cases. Correct SD-8 to
name the new read. Alternatively, load the member-side dialog as a fragment
too, which removes the per-page cost and matches the brief (see R10-17).

### R10-10 — FR-3.3 leaves out the cycle exclusion and describes a different default scope from the code (minor)

**What is wrong.** FR-3.1 excludes, on the member's side, any milestone the
current milestone contains (spec:254-255). FR-3.3's candidate read excludes
only "this one" (spec:269), so on the milestone's side the picker would offer
milestones that already contain this one, and FR-1.5 would then refuse them.
The committed `MemberCandidates` does exclude them (its `around` CTE,
`internal/store/owner_reads.go`). There is also a mismatch in scope. FR-3.2's
default includes "the other milestones it owns", meaning the initiative's own
(spec:263-264). The code includes milestones owned anywhere in the subtree.

**Recommended fix.** Add "and any milestone that already contains it (FR-1.5)"
to FR-3.3. Decide whether the default scope is the owner's own milestones or
the whole subtree's, and make the spec and the code agree.

### R10-11 — FR-1.3's `place` rule contradicts itself (minor)

**What is wrong.** FR-1.3 says `place` is clamped to 1…n and also that
"`place` 0 means at the end" (spec:206-208). Clamping 0 gives 1, which is the
start. Negative values aren't covered. The committed code treats `place ≤ 0` or
`place > n` as the end (`tracking.go` `PlaceRoadmapEntry`).

**Recommended fix.** "`place` from 1 to n puts the milestone there; any other
value, including 0, puts it at the end." Add a case to the AC.

### R10-12 — Small factual errors about the code and the store changes (minor)

- SD-7 names the index `sections.fts` (spec:161). The table is
  `document_sections` (`internal/store/migrations/0001_init.sql:100-103`),
  queried with `websearch_to_tsquery` (`internal/store/documents.go:166-172`).
  SD-7's conclusion stands.
- §0 says the store changes "are small and listed in FR-1" (spec:48-49). SD-3
  says "two small store additions" and then lists three (spec:111-126). FR-3.3
  adds a fourth, `MemberCandidates`. Say "four store changes: three in FR-1
  and the candidate read in FR-3.3".
- FR-1.2's AC says "the existing phase 3 tests pass unchanged" (spec:203). The
  signature change has already forced edits to `tracking_test.go`,
  `owner_reads_test.go` and `corpus_cost_test.go`. Limit the claim to the
  API-level phase 3 tests.
- FR-6.2 says "`dialog.js` calls `showModal()` on any `dialog[data-autoshow]`"
  (spec:343-344) as if it already did. Neither that nor FR-6.4's
  reload-on-close exists (`internal/server/ui/static/dialog.js`). Say both are
  additions, and update the file's header comment, which currently promises it
  does "nothing else".

### R10-13 — Roadmaps across owners are a new choice, not settled by D-12 (minor)

**What is wrong.** FR-4.2 lets a roadmap include milestones "Planned
elsewhere" (spec:296-298). D-12 removes the ownership constraint on *milestone
membership* (DESIGN-008:685-689). It says nothing about roadmap entries.
§5.1a's example is "its own roadmap of its own milestones"
(DESIGN-008:221-222). Allowing cross-owner entries is probably right, but it is
a choice.

**Recommended fix.** Add a short scope decision that roadmap entries, like
milestone members, are not limited by ownership, and give the reason.

### R10-14 — Missing or untestable acceptance criteria (minor)

- **FR-2.1** (the plan section always shown, its empty-state copy, the two menu
  entries) has no AC. Today the section is conditional
  (`entity.html:167`), so this is a real change that needs a test.
- **FR-3.3**: nothing tests the 50-row cap and its "there are more" message,
  `%` and `_` being matched literally, or direct members being excluded.
- **FR-3.4** can only be checked by review. Say so, rather than leaving it
  without an AC.
- **FR-6.4** (reload on close when `data-changed` is set) can only be checked
  in a browser. Tie it to the Playwright walkthrough (DoD 2) explicitly.
- **FR-7**: "errors are full sentences" (spec:366-367) has no AC, and the store
  errors are not sentences. Examples: `milestone name %q is ambiguous (%d
  matches); use its id`, `milestone is locked; membership is frozen at lock
  (FR-5.2)`, and the lower-case `ErrMilestoneCycle`. SPEC-008 maps errors in
  the handler (`initiativeCreateError`, `attachError`); say FR-7 does the same,
  and test one refusal of each kind.
- **FR-7.8**: the lock-eligibility and G4-reason fields are not in FR-7's AC.

### R10-15 — FR-7 promises live pages that §6 says don't exist (minor)

**What is wrong.** FR-7 says each write signals the SSE hub "so an open page
follows" (spec:365-366). §6 says entity pages "don't yet listen for it outside
the document page" (spec:453-456). That is true: the only `sse:changed`
trigger on an entity page is `#doc-live` (`entity.html:783-784`).

**Recommended fix.** Reword FR-7: "signals the SSE hub, as SPEC-008 FR-5.1
does. Entity pages don't yet refresh from it (§6)."

### R10-16 — The status line says this review's findings are already folded in (note)

The header says "its findings are folded in below" (spec:3-5). That was written
before this review existed. Change it to say so only once the findings have
actually been dealt with, and record which were fixed and how, as
REVIEW-006 §2 does.

### R10-17 — The create and member-side dialogs aren't HTMX-loaded, which departs from the brief's wording (note)

The M4 brief asks for "htmx-loaded `<dialog>` modals". SD-8 loads only the two
editors that way and keeps the create dialogs and the member-side dialog inline
(spec:170-175). That is a reasonable choice for the create dialogs. For the
member-side dialog, R10-9 suggests loading it too. Either way, name it as a
choice for Sam rather than letting it pass unremarked.

### R10-18 — Entry criterion 5 is deferred without a destination (note)

§2 sends the roll-up tree and work list to "a later Stage B slice"
(spec:91). No milestone in the roadmap schedules them; the roadmap only lists
them as "Not built. Also Stage B". Name a milestone, or record in the handoff
that they are unscheduled, so they don't drop out of sight.

## 3. Checks that passed

- **Entry criteria 1 to 3 are answered.**
  - Owner-scoped creation, with the CLI still defaulting to the project: SD-2,
    FR-1.1 and FR-1.2.
  - The modal's routes, how it closes and what it swaps back: FR-6.1, FR-6.3
    and FR-6.4, subject to R10-5.
  - Both ends of the picker, and full-text index versus a plain query: FR-3.1
    to FR-3.3 and SD-7.
  - Criterion 4 (checklists) is deferred to M5 with a destination.
  - Criterion 5 is deferred without one (R10-18).
- **The "what must stay true" list is kept**, apart from the test coverage gap
  in R10-6. No new table markup or currency (NFR-5, NFR-6). G4 is a blocking
  inline reason with no checkpoint and no force (FR-5.3, NFR-3). This matches
  REVIEW-006 R6-1 and `handleLockMilestone`'s 409 (`http_phase3.go:285-311`).
- **None of the MCP tools is a relay.** None carries a human's verdict, issue
  or tick, and none starts work, spends resources or decides a gate. So DEC-006
  Amendment 1's quoted-words rule and its "needs a decision" clause don't apply
  to them (R10-7 is about the evidence, not the conclusion).
- **Taking members out can't be used to get past G4.** G4 passes when at least
  one resolved feature is done (`gates.go:105-115`). Removing a member can
  never raise the done count, so `remove_milestone_member` can't turn a refusal
  into a pass. Adding a done feature could make a lock *possible*, but the lock
  itself stays a human act in the UI.
- **Leaving `lock_milestone` out is enforced as DEC-005 requires.** An
  unregistered tool is refused as method-not-found (`-32601`, `mcp.go`
  `handleMCPToolCall`), and naming it in the forbidden loop of
  `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`
  (`integration_mcp_test.go:118-121`) makes the absence a regression guard.
  The count is right: SPEC-008 has nine tools (`integration_mcp_test.go:101-111`)
  and FR-7 adds ten.
- **An unscoped `list_milestones` doesn't contradict D-9.** D-9 and §5.1a are
  about the interface's navigation ("those screens would be noise",
  DESIGN-008:248-252). `GET /api/milestones` already lists all milestones, and
  `milestoneByRef` already resolves names project-wide. The agent needs the
  same lookup. The member-side "Add to a milestone…" list is sanctioned by
  §5.1a itself ("a short list, since there are few milestones").
- **Ownership and membership are kept apart.** Creation carries the owner. The
  picker defaults to the owner's subtree and search widens it to the whole
  project (FR-3.2), as D-12 and §13 require. Migration 0005's CHECKs
  (`owner_type` is project or initiative, and `owner_id` is null exactly when
  the owner is the project) match FR-1.1's refusal rule.
- **The claims about existing code hold**, except for those in R10-12.
  - The six service methods exist.
  - `milestoneByRef` and `roadmapByRef` exist (`http_phase3.go:347, 438`).
  - `SetRoadmapEntry` really does allow tied positions: no unique index on
    `position` (`0004_planning_and_tracking.sql:71-77`).
  - The resolver really does tolerate cycles (`ResolveMembers`' `visited` map).
  - The hidden fields `owner_type` and `id` match the existing attach handler
    (`ui_entity_actions.go:119-128`, `formID`).
- **The new field names avoid the banned list**: `milestone_id`, `roadmap_id`,
  `member_id`, `member_type`, `place` and `q`
  (`integration_ui_test.go:158-159`).
- **SD-7's reasoning is sound.** The full-text index is over document prose,
  with English stemming, so it can't find entities by name or slug.
- **Scope matches the M4 brief**: owner-scoped creation, UI create, add and
  remove from both ends, reorder, lock, and MCP tools to create, add, remove,
  place, reorder, list and read, with no lock tool. Checklists are out, with
  FR-3.4 keeping room for them. The two additions beyond the brief,
  `RemoveRoadmapEntry` and the nesting guard, are small, justified and
  recorded (SD-3).
- **Prose.** British spelling throughout. The human-facing strings (the FR-5.1
  and FR-5.2 copy and the empty states) are full sentences, as D-6 requires.
  The only prose problem worth raising is the G4 wording in R10-4.

## 4. Notes carried into implementation (not blocking)

- **The in-flight code differs from the spec in small ways.**
  - The blank-name errors are lower-case fragments ("a milestone needs a
    name"). FR-1.1 specifies full sentences.
  - `checkPlanOwner` doesn't catch an initiative id that doesn't exist, so the
    foreign key fails with a raw error. It also doesn't say whether an archived
    initiative may own a new plan. Decide both.
- **A refused lock leaves no audit row.** `LockMilestone` writes
  `gate.evaluated` and then returns an error, so `WithTx` rolls the row back
  (`tracking.go` `LockMilestone`; `store.go` `WithTx`). That isn't new, but
  FR-5.3 is the first human-facing refusal path. Decide whether the evaluation
  should be kept, for example by auditing it in a separate transaction.
- **`handleLockMilestone` reports every failure as a G4 409**, including
  "milestone already locked" (`http_phase3.go:305-308`). The UI handler
  shouldn't copy that.
- **What M5 will need.** The `milestone_members` CHECK
  (`0004_planning_and_tracking.sql:53`) has to admit `checklist`, and the
  `member_type` enum on `add_milestone_member` grows. `ref_type` already has
  the value.
- **The tool-set test will collide with M3.** M3's relay tools (DEC-006
  Amendment 1) also edit `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`.
  FR-7.9's "nine plus these ten" is true only if M4 lands first. Whichever
  lands second updates the list.
- **An open detail in the member list.** Removing from the milestone's side
  (FR-3.2) doesn't say whether it asks for the optional reason that the
  member's side records. Make the two ends consistent.
- **The build started before approval.** The store layer was committed before
  the spec was approved. The spec says Sam will approve the spec and the build
  together, and that is his call. It does mean the fixes in §2 may touch code
  that has already been committed.

## 5. Recommendation and decision

The spec is well aimed. It covers the M4 brief, answers entry criteria 1 to 3,
keeps the G4 shape, and keeps the chat agent on the planning side of the seam.
It isn't ready to approve as written. Six findings are material:

- R10-1 and R10-2: the argument for leaving out the lock tool rests on a false
  premise and the wrong clause.
- R10-3: two deviations from DESIGN-008 are presented as conformance.
- R10-4: G4's reasons are not the plain words the spec promises.
- R10-5: the HTMX response split breaks under `hx-boost`, and the no-script
  promise isn't true of the editor.
- R10-6: the no-typed-path test doesn't reach the new forms.

All six can be fixed in the text of the spec and a few lines of code. None
needs a new design.

Decisions for Sam once the findings are dealt with:

1. **No lock tool over MCP (SD-4).** Recommended as argued in R10-2. If Sam
   wants one, it needs its own decision amending DEC-006 Amendment 1's relay
   list and qualifying DEC-004's "touch gates".
2. **Removal from the member's side, and Edit on the milestone and roadmap
   pages (R10-3).** Accept them as deliberate deviations from DESIGN-008
   §5.1a, and note them in the design.
3. **Milestones and roadmaps count as DEC-004's planning layer (R10-7)**, so
   the ten tools are authoring, not relay.
4. **Roadmap entries may cross owners (R10-13).**

**Recommended for approval once R10-1 to R10-6 are fixed.** The reviewer does
not approve.

_Decision (Sam), 2026-09-28: the four decisions above are settled.
(1) Locking became a reversible "Mark as shipped" (SPEC-010 SD-11), and the
chat agent may mark as shipped and reopen, under DEC-004 Amendment 1, with
two tools added for it (SPEC-010 FR-7.10). (2) Both deviations from DESIGN-008 §5.1a are
accepted, with a dated note in the design. (3) Milestones and roadmaps are
planning layer, recorded in DEC-004 Amendment 1. (4) Roadmap entries may cross
owners. **SPEC-010 and the build approved, 2026-09-28.**_

## 6. Author's response (2026-09-28)

_Added by the spec's author, not the reviewer. The findings above are left as
the reviewer wrote them._

Every finding was accepted. The spec's own
[§7](../specs/SPEC-010-milestones-and-roadmaps-editing.md#7-changes-after-review)
records the change made for each one. In short:

- **R10-1, R10-2:** SD-4 now rests on DEC-004's "may not touch gates" and D-13,
  with irreversibility second, and admits that creation can't be undone
  either. A lock tool, if wanted, needs its own decision.
- **R10-3:** the two departures from DESIGN-008 §5.1a are recorded as
  deviations for Sam to accept (SD-6).
- **R10-4:** fixed at the source, as recommended. `lifecycle.G4`'s refusals are
  full sentences that count features (commit `3719e36`), and `TestG4` checks
  them. The UI and MCP show G4's own reason rather than a translation.
- **R10-5:** the code already chose the response by `HX-Boosted` and
  `from=member` rather than `HX-Request`; the spec now says so, drops the
  no-script claim for the editors, and `TestUIPlanEditing` posts both ways.
- **R10-6:** the new tests scan every new page and fragment with a wider
  banned list (`typedPathFieldNames`).
- **R10-8:** the list tools take `owner_type` and `owner_path`. The create tools
  keep `owner_path` alone; the spec says why.
- **R10-10:** the spec now matches the code: containing milestones are
  excluded, and the default includes milestones planned anywhere in the
  subtree.
- **§4 notes:** removal asks for the same optional reason from both ends; the
  refused-lock audit, the API's 409 and the M3 collision are carried in the
  spec's §6.

The four decisions in §5 are unchanged, and are listed again in the handoff.
