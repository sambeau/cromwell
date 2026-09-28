# REVIEW-011: Consistency Review of SPEC-011 (Send to development)

**Status:** Complete; awaiting the author's response (§6) and Sam's decision
(§5)
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author. Approval is
Sam's, recorded in §5.
**Scope:** [SPEC-011](../specs/SPEC-011-send-to-development.md) (draft, commit
`0a90e97`), against
[DEC-004](../decisions/DEC-004-mcp-planning-authoring.md),
[DEC-005](../decisions/DEC-005-the-orchestration-boundary.md),
[DEC-006](../decisions/DEC-006-humans-start-development.md) with Amendment 1
(which replaces its decisions 5, 6 and 8),
[DEC-007](../decisions/DEC-007-the-judgement-boundary.md),
[DESIGN-010](../design/DESIGN-010-subutai.md) (§2, §3, §5 with "The first
button", "How specs are reviewed" and "When a design changes", §5a, §5c, §14,
§17b), [SPEC-009](../specs/SPEC-009-the-authoring-chain.md) as built, the M3
brief in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md),
and the code at `0a90e97`: `internal/server/authoring.go`, `actions.go`,
`documents.go`, `planner.go`, `mcp.go`, `ui_views.go`, `http.go`,
`worktree_ops.go`, `internal/rules/rules.go`, `internal/config/compartment.go`,
`internal/dispatch/`, `internal/starter/` and `internal/store/migrations/`.

## 1. What this review is

An authoring consistency pass by a reviewer who did not write the spec. It
checks that SPEC-011 agrees with the decisions and the approved design it
builds on, that what it says about the code is true, that its requirements can
be tested, and that it covers the M3 brief. It looks hard at two questions the
brief makes central: is there any path by which a spec is approved without a
reviewer, and is there any path by which the chat agent can send, start
building, lock, override a gate, answer a checkpoint or hold a verdict of its
own. It does not fix the spec.

One thing about timing. While this review was running, the build had already
started in the working tree: migration `0007_send_to_development.sql`,
`internal/store/sends.go`, changes to `compartment.go`, `config.go` and
`documents.go`, and the starter pack's design reviewer removed. Findings are
about the spec as committed. Where the in-flight code already answers or
differs from the spec, that is noted.

## 2. Findings

### R11-1 — A draft that needs its author is only restarted by a transition event, so a feature can stall after Send (material)

**What is wrong.** The spec invariant is "has a current spec" (FR-2.1,
spec:242-245). A current spec in `draft` satisfies it. The revise loop fires
only on the event of a document moving from `reviewing` to `draft` on a sent
feature (FR-3.1, spec:290-293). So any draft that got into that state *before*
the feature was sent, or whose event was lost, is never picked up:

- **An issue on an unsent feature's approved spec.** FR-6.2 opens a successor
  draft to carry it and dispatches nothing (spec:447). When the feature is
  later sent, the invariant is satisfied by that draft, FR-3.1 has no
  transition to react to, and FR-3.4 says a never-submitted draft "is not the
  loop's business" (spec:307-308). Nothing writes it.
- **An early agent review that sends a chat-written spec back** (DESIGN-010
  §5a). The feature is unsent, so the send-back dispatches nothing (FR-3
  acceptance, spec:316-317). Pressing Send later does nothing either.
- **A lost event.** SPEC-009's whole argument for invariants is that the
  heartbeat recovers a lost trigger (SPEC-009 FR-4.7). The revise loop is an
  event-only rule with no invariant behind it, so the heartbeat can't recover
  it.

In all three cases the send screen would show the spec step as **already
done**, because "its document exists" (FR-4.3, spec:337-339), and the feature
would sit with nobody working on it.

**Evidence.** `neededAuthoring` returns `""` as soon as a current spec exists
and is not approved (`internal/server/authoring.go:86-105`).
`ReconcileAuthoringSweep` reaches only what `neededAuthoring` names
(`authoring.go:141-153`). `CurrentDocForOwner` returns the newest
non-superseded row, so a successor draft counts as the current spec
(`internal/store/documents.go:90-94`).

**Recommended fix.** Express the revise loop as part of the invariant, not as a
reaction to one event. For example: "a sent feature whose current spec or plan
is a `draft` that has open review findings or open human issues, and has had
no author dispatch since the latest of them, needs its author." Then Send, the
heartbeat and the transition all find it through `neededAuthoring`. Say what
"already done" means on the send screen: a submitted or approved document, not
any row. Show a draft as "draft, waiting for its author" or "draft, will be
revised", whichever the rule produces. Add an acceptance case for each of the
three bullets above.

### R11-2 — Direct human approval of a spec or plan needs SPEC-009 FR-2.3 and FR-2.4 changed, and the spec doesn't say so (material)

**What is wrong.** FR-5.5 lets a person approve a spec directly on the document
page (spec:407), FR-5.7 extends that to plans (spec:420-421), and
`relay_verdict` carries a person's approval of "any document in `reviewing`"
(spec:504). Today the one human approval path refuses both types: "a spec is
decided by its agent reviewer; it comes to you only when the reviewer
escalates". That is SPEC-009 FR-2.3 and FR-2.4 as built ("An agent-approvable
document is unaffected"). The spec's **Changes** line lists SPEC-009 FR-2.2
but not FR-2.3 or FR-2.4 (spec:20-22), and DEC-006's own Consequences list
"per-type approval authority (FR-2)" as kept unchanged. Amendment 1 does change
it, so the spec is right to allow direct approval. But the change to the
authority rule, which is the heart of what "who may approve" means, is left
implicit.

**Evidence.** `HumanApproveDocument` and `HumanReturnDocument` both refuse any
type whose manifest isn't `approved_by: human`
(`internal/server/actions.go:452-477`).

**Recommended fix.** Add a requirement that restates approval authority under
Amendment 1. For example: "`approved_by: agent` now means the agent reviewer is
the normal approver, and a person may also approve directly. `approved_by:
human` is unchanged: no agent verdict can approve such a document." Name
`HumanApproveDocument` as the method that widens, keep the SPEC-009 test that
no agent verdict approves a design, and add SPEC-009 FR-2.3 and FR-2.4 to the
**Changes** line.

### R11-3 — NFR-3 is false today: the API and the CLI can start building (material)

**What is wrong.** NFR-3 says "Only a `POST` from the web UI writes the sent
mark or starts building. A test asserts no MCP tool, and no `/api/*` route,
can" (spec:585-587). But `POST /api/features/start` exists and `cromwell
feature start <path>` calls it. DESIGN-010 §5 also says the Start building
button "is in the web UI only, too", so the gap is with the design as well. The
spec neither removes the route nor narrows the claim, so the promised test
would fail on the first run.

**Evidence.** `internal/server/http.go:55` (`POST /api/features/start`),
`cmd/cromwell/main.go:34` and `:296-298` (the CLI verb),
`internal/server/worktree_ops.go:33` (`StartFeature`).

**Recommended fix.** Decide which is true and say it. Either remove the API
route and the CLI verb in this milestone and keep NFR-3 as written, or narrow
NFR-3 to "no MCP tool can send or start building, and no API route or CLI verb
can send". In the second case, record the CLI start as a known departure from
DESIGN-010 §5 for Sam to accept. The chat-agent boundary holds either way,
because the chat agent has no route to `/api/*`. What matters is that the spec
doesn't promise a test that can't pass.

### R11-4 — SD-11 only stops relayed verdicts, so other relays can get round a pending checkpoint (material)

**What is wrong.** SD-11 is titled "A relay can't reach a document whose review
has escalated", but its text and FR-8's acceptance only refuse a *verdict*
(spec:195-198, 526). Two other relays can have the same effect as answering
the checkpoint:

- **`relay_review_request`** is allowed on a spec "in `reviewing`, with no
  review queued or running" (FR-5.5, spec:409). An escalated spec is exactly
  that. Escalation leaves the document in `reviewing` and ends the dispatch. A
  fresh review whose `approve` then lands would approve the document while
  the human's escalation question is still unanswered in the Inbox.
- **`relay_issue`** on an escalated spec of a sent feature sends it back to
  `draft` (FR-6.2, spec:445). That is the checkpoint's "request changes"
  answer, reached from chat. The checkpoint is left pending on a document it
  no longer describes, and answering it "approve" later fails, because the
  transition from `draft` is illegal.

The same shape applies to the other checkpoints that refer to a document:
`relay_review_request` on a draft is a Submit (spec:409), which sidesteps a
pending `authoring-deadlock` question (FR-3.3). `relay_issue` on an approved
spec listed in a pending `design-revision` checkpoint opens a successor under
a spec the checkpoint may be about to invalidate. DESIGN-010 §5c says the chat
agent "doesn't answer checkpoints at all". That needs to hold in effect as
well as in name.

**Evidence.** Escalation leaves the state at `reviewing` and raises
`review-escalation` without a transition (`internal/rules/rules.go:374-375`,
`516-523`). A human `request_changes` answer maps to `ReturnForChanges`
(`rules.go:613-617`), which is what FR-6.2's send-back does.

**Recommended fix.** Widen SD-11 into a rule every relay obeys: "A relay is
refused on a document that a pending checkpoint refers to, or lists
(`review-escalation`, `authoring-deadlock`, `design-revision`), with a sentence
pointing to the Inbox." Add each case to FR-8's acceptance. Then decide what
the same acts do in the UI, where a person may legitimately do them: either
refuse them too until the checkpoint is answered, or have them resolve the
checkpoint explicitly, with an audit row saying so. Don't leave a pending
checkpoint on a document that has moved on.

### R11-5 — An approved successor plan on a `ready` feature is never re-decomposed, so SD-14's "no extra cost" is not true (material)

**What is wrong.** SD-14 extends issues to plans "at no extra cost"
(spec:212-214). An issue on an approved plan opens a successor (FR-6.2,
spec:446). FR-6.6 defines what happens when a *successor spec* is approved on
an `idea` or `ready` feature (spec:463-467). It says nothing for a successor
*plan*. In the rules as built, a dev-plan approval decomposes only for an
`idea` feature, and re-decomposes only for an `active` or `review` one. A
`ready` feature gets neither. Its tasks stay those of the superseded plan.
FR-6.7 then lets Start building through, because the current plan is approved
again (spec:469-471). The feature would be built from tasks that don't match
its approved plan. A person's **Revise** on an approved plan (FR-9.2) reaches
the same place.

**Evidence.** `decideDocumentTransition`'s `DocApproved` branch:
`DecomposeDevPlan` only when `f.State == FeatIdea`, `ReDecomposeDevPlan` only
for `FeatActive` or `FeatReview` (`internal/rules/rules.go:428-449`).

**Recommended fix.** Add a requirement next to FR-6.6: approving a successor
plan for a `ready` feature re-decomposes it (`ReDecomposeDevPlan`, which
already reconciles tasks without destroying them), or returns the feature to
`idea` and decomposes afresh. Add an acceptance case. If that is more than
this milestone wants, refuse issues and Revise on approved plans instead, and
say so in SD-14.

### R11-6 — FR-3.4 and FR-6.2 disagree about hand-written drafts, and Submit races the revise loop (minor)

**What is wrong.**

1. FR-3.4 says a never-submitted draft, written by hand or in chat, "is not the
   loop's business" (spec:307-308). FR-6.2's first row says an issue on a
   sent feature's `draft` dispatches the author (spec:444). For a hand-written
   draft, that sends an agent to overwrite a person's work.
   `fileAuthoredDocument` writes the agent's body over the file at the
   draft's path.
2. FR-9.1 **Submit**, and `relay_review_request` on a draft (which "means
   Submit", spec:409), can be pressed while the author's revise dispatch is
   queued. The author then finds a current spec in `reviewing`, refuses with
   "nothing to author", and the dispatch fails into a `dispatch-failure`
   checkpoint.

**Evidence.** `internal/server/authoring.go:572-598` (fills the current draft in
place; refuses any other state).

**Recommended fix.** In FR-6.2, dispatch the author only for a draft the loop
owns: one that has been submitted before, or an orchestrator-opened successor.
Record the issue on a hand-written draft for whoever is writing it. Say that
Submit on a draft with a queued or running author dispatch is either refused
with a sentence or cancels the dispatch, and test whichever is chosen.

### R11-7 — The issue rules have small contradictions and one path that drops an issue (minor)

**What is wrong.**

- **Designs.** SD-14 lets a design in `draft` or `reviewing` carry an issue
  (spec:214-216). FR-6.1 allows issues only "on a spec or plan whose feature is
  `idea` or `ready`" (spec:432-434), and FR-6.2's table is keyed on a feature,
  which an initiative's design doesn't have.
- **Unsent features.** FR-6.2 says nothing is dispatched for an unsent feature
  (spec:447). FR-6.4 queues a fresh review when an issue arrives mid-review
  (spec:456-458), whether or not the feature is sent.
- **Escalation answers.** FR-5.5 says a direct approval settles open issues
  (spec:407). It doesn't say whether a person's `approve` on a
  `review-escalation` checkpoint does too. It is the same kind of act.
- **Detach.** FR-9.3 deletes the document's comments (spec:538-541), which
  includes open human issues. Goal claim 2 says "A human's issue on a spec is
  never dropped" (spec:69-70). Detaching a successor opened to carry an issue
  (SD-7) is the obvious way to withdraw a revision, and it would silently
  discard the issue.
- **The hold.** FR-6.2 sends a held spec back to `draft`. It doesn't say the
  hold record is cleared, as FR-5.5 says for Approve.

**Recommended fix.** Make FR-6.1 match SD-14, and give designs their own row
(recorded as a note for the human approver, with no state change). Say FR-6.4
applies only to sent features, or say that it is the one dispatch allowed on
an unsent one. Treat an escalation `approve` like a direct approval. Refuse
Detach while open issues exist, or copy them into the `document.detached`
audit payload and say so on the confirm step. Clear the hold on any send-back.

### R11-8 — `relay_verdict`'s send-back has no UI twin for specs, and for an unsent feature it doesn't send anything back (minor)

**What is wrong.** NFR-1 says the relays call the same service methods as the
document page (spec:580-582). FR-5.5's table of what a person can do with a
spec has Approve, Raise an issue, Ask for an agent review and Let the reviewer
decide. It has no **Send back** (spec:405-411). `relay_verdict` has
`send_back`, which for a spec or plan "is recorded as a human issue"
(spec:504). By FR-6.2, an issue on an unsent feature's `reviewing` spec is
"Recorded only" (spec:447). So when the human says "send it back", the spec
stays in `reviewing`. The tool's verb and its effect disagree, and the chat
agent will report something that didn't happen.

**Recommended fix.** Either drop `send_back` for specs and plans from
`relay_verdict` (keep it for designs, which do go back to draft) and let
`relay_issue` carry the human's objection, or add a UI **Send back** for specs
and plans and define its effect for each FR-6.2 row. Whichever is chosen, make
the relay's result sentence say what actually happened.

### R11-9 — Send on a project without the authoring roles assigned does nothing, silently (minor)

**What is wrong.** FR-11.1 turns `write-spec` and `write-dev-plan` on for *new*
projects. Existing projects have them commented out. `queueAuthoring` treats an
unassigned purpose as silence, which was right when approval was the trigger.
Under this spec, a person on an existing project can press Send, see a mark,
and have nothing happen. FR-4.3 says each step "shows the role and model that
will run it" (spec:336-339) but doesn't say what it shows when there is no
role.

**Evidence.** `internal/starter/starter.go:68-74` at `0a90e97` (commented
lines); `internal/server/authoring.go:190-199` (silence).

**Recommended fix.** On the send screen, show an unassigned step as "No agent is
assigned to write the spec in this project, so you write it", and add a
sentence to the handoff telling existing projects to uncomment the two lines.
Consider refusing Send when neither authoring role is assigned and no document
exists, because then the mark means nothing.

### R11-10 — Detach says it leaves the file on disk, but the author then overwrites it (minor)

**What is wrong.** FR-9.3 removes a draft's registration, "leaves the file on
disk", and "lets the invariants re-check" on a sent feature (spec:538-541).
The re-check dispatches `write-spec`. That writes to the conventional path,
which is usually the file just detached, and so overwrites the copy the person
chose to keep. Detach while an author dispatch is running has a similar
problem: the author finds no current draft and registers a new one at the
same path.

**Evidence.** `authoredDocPath` and `os.WriteFile` in `fileAuthoredDocument`
(`internal/server/authoring.go:583-598`, `617-623`). The live-path index
excludes only superseded rows, so re-registration at the same path succeeds
(`internal/store/migrations/0001_init.sql:84-86`).

**Recommended fix.** Refuse Detach while an author dispatch for that document
is queued or running. On a sent feature, either move the detached file aside
(as the cascade archives to `docs/_superseded/`), or say plainly on the
confirm step that the author will write a new document at that path.

### R11-11 — NFR-7 says the review outcome schema is built in `internal/server`; it isn't (minor)

**What is wrong.** SD-6 adds an `issues` list to `submit_review` (spec:147-149).
NFR-7 says there is "No change to `internal/dispatch/` (the relay and review
outcome schemas are built in `internal/server`)" (spec:596-598). The
`submit_review` tool definition lives in `internal/dispatch`, and the payload
type and its parser live in `internal/rules`, which code review shares.

**Evidence.** `dispatch.ReviewOutcomeTool` (`internal/dispatch/dispatch.go:30-61`),
used by `Plan` (`internal/server/planner.go:56-60`). `rules.ReviewOutcome` and
`ParseReviewOutcome` (`internal/rules/rules.go:197-266`).

**Recommended fix.** Say what will actually happen. Either the document-review
plan gets a server-built tool definition that adds `issues` (leaving
`internal/dispatch` alone and code review unchanged), or NFR-7 allows the one
change to `dispatch.go`. Also say whether `rules.ReviewOutcome` gains the field
or a document-only variant is added, so the code-review path isn't affected by
accident.

### R11-12 — SD-7 says the approved spec "stays current"; in the code it stops being current (minor)

**What is wrong.** SD-7 says "the approved spec stays current until the
successor is approved" (spec:168-169). In the code, "current" means
`CurrentDocForOwner`, the newest non-superseded row, and that becomes the
successor draft as soon as it is opened. FR-6.7 depends on the code's meaning
("while a feature's current spec or plan is not approved", spec:469). The
approved spec stays *live* and *approved*, not current. That is the reading
that makes FR-6.7 work, and the wording should match it, because "current" is
a term the invariants use.

Separately, Amendment 1 says an issue on an approved spec "reopens it for
revision. Its development plan is retired automatically". SD-7 retires the plan
only when the successor is approved, following DESIGN-010 §5, which was
approved later and says exactly that. That is a fair reading, and DoD 7.5
already puts it to Sam. See §3.

**Evidence.** `internal/store/documents.go:90-94`.

**Recommended fix.** Reword SD-7: "The approved spec stays approved and live,
and its plan with it, until the successor is approved. While the successor is
open, the feature's current spec is the successor draft, so Start building
refuses (FR-6.7)."

### R11-13 — The `authoring-deadlock` checkpoint has no wiring in the inbox or the rules (minor)

**What is wrong.** FR-3.3 names a new checkpoint answered `retry` or `cancel`
(spec:300-306). The inbox offers verbs per kind, and an unknown kind falls to
"acknowledge", which the engine ignores. The rules engine has no case for the
new kind. Neither change is named, so the checkpoint would render with the
wrong button and do nothing.

**Evidence.** `answerOptions` and `responseFor`
(`internal/server/ui_views.go:34-90`); `decideCheckpointResponded`
(`internal/rules/rules.go:598-712`).

**Recommended fix.** Add to FR-3.3: the inbox offers **Allow another round**
and **Stop, I'll edit it**; `responseFor` maps them to a payload; and the rules
turn `retry` into one author dispatch. Make FR-3's acceptance answer the
checkpoint through the inbox.

### R11-14 — The hold's storage and the document page's markup are not where the spec says (minor)

**What is wrong.**

- FR-1.1 lists `feature_sends` as migration 0007's table (spec:222-224), and
  FR-6.1 lists the comment columns. But FR-5.4 says "the hold records which
  review approved it" (spec:397-399), and FR-9.3 deletes "any hold"
  (spec:539-540). Where that record lives isn't specified. The in-flight
  migration adds a `document_holds` table for it.
- NFR-6 says new markup goes in `send.html` and "`entity.html` gains only the
  includes and the renamed label" (spec:593-595). The document page is
  `page-entity-document` in `entity.html` (`entity.html:777`). FR-5.6's
  notices, FR-6.5's issue record and FR-9's three buttons all go there.

**Recommended fix.** Name `document_holds` (document, the held approval's
dispatch or null, created at) in FR-1.1. Either name a second partial for the
document-page additions, or say that `send.html` holds them too.

### R11-15 — The estimate step has three loose ends (minor)

**What is wrong.**

- G1 moves the feature to `ready` when the plan is approved, before the
  estimate lands (FR-7.1 and FR-7.2, spec:486-492). Start building can be
  pressed without an estimate, although DESIGN-010 §5 has the human "look at
  the reviewed plan and the estimate".
- A feature prepared fully in chat (DESIGN-010 §5a) reaches `ready` without
  being sent. FR-4.4 only sends `idea` features (spec:350-351), and FR-7.1 only
  estimates sent ones, so that feature can never get its estimate from the
  chain.
- After a re-plan (FR-6.6), "no estimate dispatch since it was sent" means no
  new estimate.
- The send screen says a step is **already done** "when its document exists"
  (spec:338). An estimate isn't a document.

**Recommended fix.** Show "estimate pending" in the Start building card while
one is queued. Either queue the estimate when a sent feature reaches `ready`
by any route, or allow Send on an unsent `ready` feature to run just the
estimate. Estimate again when a plan is replaced. Define "already done" for
the estimate as "the feature has an estimate".

### R11-16 — The trigger table leaves out some re-checks (minor)

**What is wrong.** FR-2.3 replaces SPEC-009 FR-4.3's table (spec:251-259). It
leaves out events that the spec elsewhere says re-check or dispatch: Withdraw,
Detach (FR-9.3), an `authoring-deadlock` retry (FR-3.3), and a
`design-revision` answer (which already reconciles). It also leaves out a
change to `spec_review`. A spec left waiting in `reviewing` with no review,
because agent review was off, gets no review when agent review is turned back
on, and on a sent feature with the send's hold off it is then not held either.
Nothing moves it.

**Recommended fix.** Complete the table. For the settings change, say that the
send screen and the document page read the setting live, and that a spec
waiting with no agent verdict while agent review is on offers **Ask for an
agent review**. Or have the heartbeat queue one.

### R11-17 — FR-3.5 checks only the state, not which submission was reviewed (note)

A verdict for a document "no longer in `reviewing`" is ignored (spec:310-311).
A slow review can finish after the document has been sent back, revised and
resubmitted. That review's `approve` then lands on a document that is in
`reviewing` again, with content it never read. FR-6.4 catches the case where
an issue caused the send-back. Checking that the dispatch reviewed the current
content hash catches every case, and the key already carries it
(`rules.ReviewIdempotencyKey`, `internal/rules/rules.go:301-303`).

### R11-18 — Two texts need updating with the relays (note)

- The `initialize` instructions say the agent "cannot start work, change a
  gate, lock a milestone, or run an agent" (`internal/server/mcp.go:269-273`).
  With `relay_review_request`, "run an agent" becomes untrue. FR-8.4 should
  rewrite the whole sentence, not only add to it.
- FR-3.2 gives the revising author "every open review finding" (spec:295-296).
  Nothing marks an ordinary finding resolved (only issues get `addressed_*`),
  so by the third round the author and the reviewer both see every earlier
  finding as open (`CommentsForDocument` with `unresolvedOnly`,
  `internal/store/documents.go:242-250`). Say whether findings from earlier
  rounds are resolved on resubmission, or labelled by round.

### R11-19 — The status line and the Goal claim more than is there (note)

- The header says "§7 says how each finding was dealt with" (spec:6-7). There
  is no §7 yet. Change it once there is, as REVIEW-010 R10-16 asked of
  SPEC-010.
- The Goal says pressing Send produces a reviewed spec and plan "and then stops
  at Start building" (spec:61-63). With the hold on, or agent review off, it
  stops at the hold first. Add "unless the spec is held for a person".

## 3. Checks that passed

- **Approval starts nothing.** The design-approval rule
  (`rules.go:404-419`) keeps reconciling over its scope. With `featureSent` in
  `neededAuthoring`, an unsent feature gets nothing, as DEC-006 decision 1 and
  DESIGN-010 §2 require. `TestApprovingADesignStartsNothing` tests it directly.
- **The sweep and the cascade apply to sent features only**, as DEC-006
  Consequences and decision 7 require (FR-2.4, FR-2.5). FR-9.1 and FR-9.2 still
  choose for every feature. The claim that a `ready` feature returns to `idea`
  "as built" is true (`authoring.go:432-435`).
- **SD-2 and SD-3 are sound, and correctly flagged.** Without the mark on the
  dev-plan invariant, a spec approved early in chat would set the plan writer
  off, because spec approval reconciles (`rules.go:436-438`). DEC-006 names only
  FR-4.1, so this is an extension, and DoD 7.2 puts it to Sam. Treating
  `active` and `review` as sent keeps the in-flight branch of the cascade
  (`authoring.go:375-387`) behaving as today.
- **No combination of settings lets a spec through unreviewed**, apart from
  the escalation gap in R11-4, which concerns checkpoints rather than settings.
  Agent review off forces the hold. A one-off or FR-6.4 review of a held spec
  is held again. **Let the reviewer decide** is refused when no automatic
  reviewer is on. The `authoring-deadlock` checkpoint offers no "approve" (unlike
  `review-deadlock`). FR-10.2 keeps `reviewer_role` required wherever the agent
  approves. NFR-4's combinatorial test is the right check.
- **Amendment 1 is followed.** The agent reviewer is the normal approver.
  Human issues are must-address, and routed by state as DESIGN-010 §5 lists.
  Direct approval and requested reviews are available and neither is
  expected. The hold is optional, off by default, per project and per send.
  Agent review can be off, with the hold then forced on and **let the
  reviewer decide** not offered. The send screen says who reviews each spec.
- **SD-5 (the hold catches approvals only) is a fair reading** of "waits after
  the agent review", and DoD 7.3 puts it to Sam.
- **SD-7 follows DESIGN-010 §5.** Amendment 1's "reopens for revision; its
  plan is retired automatically" is less precise about timing. DESIGN-010,
  approved later the same day, says the successor supersedes the spec and plan
  when it is approved. DoD 7.5 puts the choice to Sam. The wording fix is in
  R11-12.
- **SD-6 answers DESIGN-010 §17b as Amendment 1 suggested**, in the comment
  and severity machinery. Its claims hold: comments persist across
  resubmission and reach the reviewer's prompt (`documents.go:413-421`), and
  severity is checked inside the reviewer's turn (`planner.go:56-68`). The
  reviewer, not the author, marks an issue addressed, which keeps DEC-007's
  rule.
- **The chat boundary is drawn by omission, as DEC-005 and DEC-007 require.**
  The four relays match Amendment 1's list, minus the checklist tick, which is
  deferred to M5 with a destination. Each takes the human's quoted words and
  is audited `via: mcp`. `send_to_development`, `withdraw_send`,
  `start_building` and `answer_checkpoint` join the must-not-exist list, next to
  the existing `start_feature`, `respond_checkpoint`, `override_gate` and
  `lock_milestone` (`integration_mcp_test.go:133-138`). Apart from R11-4, no
  relay sends, withdraws, starts building, locks or overrides a gate. The
  honest limitation (a quote can't prove the human said it) is DEC-006's own,
  and is carried in the out-of-scope table.
- **Send is web UI only.** FR-4.5 rules out an API route, a CLI verb and an MCP
  tool for sending. The limit on Start building is R11-3.
- **The send screen covers DEC-006 decision 3 and DESIGN-010 §5**: features
  with checkboxes, steps with role, model and "already done" (see R11-1 and
  R11-15 for what that means), who reviews each spec, a token forecast or "no
  forecast yet", and free slots from `dispatch.workers`, which defaults to 4
  (`config.go:243-245`). SD-12's forecast is honest about what `internal/sizing`
  can't do.
- **Withdraw (SD-10) answers DEC-006's "Not decided here" and DESIGN-010
  §17b** with a small, bounded rule, and is flagged for Sam.
- **The claims about SPEC-009 as built hold.** There is no revise loop: a
  send-back leaves a draft that `neededAuthoring` never touches.
  `submit_document` fills an existing draft in place (`authoring.go:565-582`).
  `dispatch.max_review_rounds` exists, defaulting to 3 (`config.go:146-150`,
  `252-254`). The estimate purpose and its idempotent enqueue exist
  (`actions_phase3.go`, `enqueueEstimate`). G0's reason is a fragment today
  (`lifecycle/gates.go:38-47`), so FR-2.6 is a real change.
- **Retiring the design reviewer keeps existing projects working.**
  `reviewer_role` is required today (`compartment.go:230-232`), so FR-10.2's
  relaxation is needed and correctly limited to `approved_by: human`. SD-9's
  `chat-skills/` directory is safe: the compartment loader reads only named
  subdirectories (`compartment.go:395`, `409`).
- **The rename is complete in scope.** Both "Start work" labels
  (`entity.html:36`, `290`) and the "Work can start once…" reasons
  (`ui_entity.go:605-630`) are covered by FR-11.2 and its render test.
- **The M3 brief is covered.** Every item maps to a requirement: sent mark
  (FR-1), approval stops triggering (FR-2), sweep and cascade for sent only
  (FR-2.4, FR-2.5), starter enables authoring (FR-11.1), Send on feature and
  initiative pages with the send screen (FR-4), no MCP send (FR-4.5, FR-8.3),
  agent review as normal approver, human issues, direct approval, requested
  review and the hold (FR-5, FR-6), agent review off (FR-5.1, FR-5.5), relays
  (FR-8), Submit, Revise and Detach (FR-9), retiring the design reviewer
  (FR-10), the rename (FR-11.2), and withdraw (SD-10).
- **Prose.** British spelling throughout, and every refusal and notice quoted is
  a full sentence, as D-6 requires.

## 4. Notes carried into implementation (not blocking)

- **The in-flight migration already goes further than FR-1.1.** It adds
  `document_holds` (R11-14), and a partial index on open issues. Keep the spec
  and the migration in step.
- **Put FR-6.7's refusal in `StartFeature`**, next to the pending
  design-revision check (`worktree_ops.go:42-49`), not in the UI handler, so
  every route that can start a feature refuses.
- **Upgrading projects mid-chain.** A feature part-way through the July chain
  when M3 lands (spec approved, plan not yet written) stalls until someone
  sends it. That is correct under DEC-006, but say it in the handoff so Sam
  isn't surprised on the live project.
- **The "who reviews this spec" sentence** should come from the spec manifest's
  `approved_by` as well as from `spec_review`. A project that has set specs to
  `approved_by: human` would otherwise be told "The spec reviewer approves it".
- **The typed-path scan.** Add the send screen and the document page's new
  forms to the `typedPathFieldNames` scan (`ui_plan_render_test.go:162`), as
  REVIEW-010 R10-6 did for M4.
- **Rules stay pure.** FR-5.3, FR-5.4, FR-6.4 and FR-3.1 each need facts the
  snapshot doesn't carry today: sent, held, agent review on, open issue ids
  and the round count. Add them to `rules.Snapshot` rather than reading the
  store from a rule.
- **Settings are read when the send is made.** The per-send hold is frozen in
  `feature_sends.hold`, but agent review is read live. Say this on the send
  screen, so a person knows that changing `config.yaml` later affects work
  already sent.

## 5. Recommendation and decision

The spec is well aimed. It follows DEC-006 with Amendment 1 closely, it
correctly finds the missing half of SPEC-009's review loop, and it keeps the
chat agent's boundary by omission. Its scope decisions are mostly sound and
the ones that go beyond the decisions are flagged for Sam. It isn't ready to
approve as written. Five findings are material:

- **R11-1:** a draft that needs its author is only restarted by a transition,
  so a feature can stall after Send, and the send screen would call the step
  done.
- **R11-2:** direct human approval of specs and plans changes SPEC-009's
  approval authority, and the spec leaves that change implicit.
- **R11-3:** NFR-3 promises a test that no API route can start building, and
  one can.
- **R11-4:** relays other than a verdict can get round a pending escalation,
  deadlock or design-revision checkpoint.
- **R11-5:** a successor plan approved on a `ready` feature leaves the old
  tasks in place.

All five can be fixed in the spec's text, with small code consequences. None
needs a new design or a new decision, except that R11-3 may need Sam to accept
the CLI start as a departure from DESIGN-010 §5.

Decisions for Sam once the findings are dealt with, in addition to the five in
the spec's DoD 7:

1. **Start building from the CLI (R11-3):** remove it, or accept it as a
   departure from DESIGN-010 §5.
2. **Issues on approved plans (R11-5, SD-14):** support them with
   re-decomposition, or leave them out of this milestone.
3. **What a relayed "send back" of a spec means (R11-8).**

**Recommended for approval once R11-1 to R11-5 are fixed.** The reviewer does
not approve.

_Decision (Sam): pending._

## 6. Author's response

To be written by the author.
