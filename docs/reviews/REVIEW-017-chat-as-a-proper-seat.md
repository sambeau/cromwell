# REVIEW-017: Consistency Review of SPEC-017 (Chat as a proper seat)

**Status:** Complete, and disposed of by the author (§6). Awaiting Sam's
decision.
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author. Approval is
Sam's.
**Scope:** [SPEC-017](../specs/SPEC-017-chat-as-a-proper-seat.md) (first
draft, commit `b6f560f` on `claude/subutai-m10-chat-seat`), against
[DESIGN-010](../design/DESIGN-010-subutai.md) §5, §5a, §5b, §5c and §8,
[DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) with Amendment 1,
[DEC-005](../decisions/DEC-005-the-orchestration-boundary.md),
[DEC-006](../decisions/DEC-006-humans-start-development.md) with Amendment 1
(decision 8 and its dated note),
[DEC-007](../decisions/DEC-007-the-judgement-boundary.md),
[SPEC-011](../specs/SPEC-011-send-to-development.md) (SD-2, FR-2, FR-3, FR-5.5),
[SPEC-015](../specs/SPEC-015-documents-with-identity.md) (SD-18, SD-19, FR-5.8),
the [M3](../notes/handoff-M3-2026-09-28.md),
[M6](../notes/handoff-M6-2026-09-28.md) and
[M8](../notes/handoff-M8-2026-09-28.md) handoffs, and the
[roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11 (M10). Code
read at `b6f560f` (the same code as `2f40650`): `internal/server/mcp.go`,
`mcp_relay.go`, `mcp_tools.go`, `mcp_plan_tools.go`, `mcp_checklist_tools.go`,
`review_send.go`, `actions.go`, `actions_phase2.go`, `documents.go`,
`identity.go`, `authoring.go`, `ui_send.go`, `ui_entity_actions.go`,
`observe.go`, `ui_identity.go`, `http.go`; `internal/rules/rules.go`,
`rules_phase2.go`, `rules_send.go`; `internal/timeline/timeline.go`;
`internal/store/documents.go`, `identity.go`, `checklists.go`, `sends.go`,
`checkpoints.go`, `store.go`, migrations `0001` to `0010`;
`internal/config/config.go`; `internal/client/client.go`; the templates in
`internal/server/ui/templates/`; and
`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` in
`internal/server/integration_mcp_test.go`.

## 1. What this review is

A consistency pass by a reviewer who did not write the spec. It checks that
SPEC-017 agrees with the documents that bind it, that what it says about the
code is true, and that its requirements can be built and tested as written. It
looks hardest at the seven questions in the brief: whether SD-1 fits DEC-006
Amendment 1 and DESIGN-010 §5c; whether FR-2.2 and FR-2.3 cover every writer
and every verdict; whether the FR-2.8 backfill can be done from the audit trail
as it is; the timeline claims; the ID claims; whether the end-to-end path
works; and what the §2 audit missed. It does not fix the spec.

**Timing.** While this review was running, the build started in the working
tree: `internal/store/migrations/0011_writers_and_verdicts.sql`,
`internal/store/provenance.go`, `internal/server/provenance.go`,
`ui/templates/provenance.html`, and edits to `actions.go`, `review_send.go`,
`identity.go`, `documents.go`, `authoring.go`, the MCP files and
`timeline.go`. Findings are about the spec as committed and the code at
`b6f560f`; line numbers are from that commit. Where the in-flight code already
answers or confirms a finding, that is noted.

The spec is well aimed. Its framing is right: most of M10 was built in M3, and
what is left is the chat agent handing in its own work, a record of who wrote
and who judged, and the M6 and M8 follow-ups. The end-to-end path it sets out
to prove does work in the code (§4). The material findings are about the
verdict record, which the spec describes as a simple hook into existing
functions but which the code can't supply as written, and about two places
where the case put to Sam is incomplete.

## 2. Material findings (must fix before approval)

### R17-1 — SD-1's case to Sam understates what a submission without a quote can start

**What the spec says.** SD-1: `submit_for_review` needs no quote. "The worst
misuse is the chat agent submitting a draft the person hadn't finished. That
costs one spec review, about 2,900 tokens ... It starts nothing else: the plan
needs the sent mark, and the reviewer can send it back."

**Does it conflict with the binding documents?** On its face, yes, and the
spec says so. DEC-006 Amendment 1 decision 8 lists "a request for an agent
review" as a relay, and "every relay carries the human's words, quoted".
DESIGN-010 §5c repeats the list. SD-1 reads submission as a DEC-005 request
instead, and DEC-005 does name "submit a document for review" as something the
facet may ask for. The reading is defensible, the spec presents it as Sam's
choice (DoD 8.1), and it proposes dated notes, as SPEC-014 SD-10 did. That part
is honest.

Two things weaken the case as written:

- **The anchor.** SD-1 leans on DEC-007 ("one way out: you submit it"; "the
  chat agent is an executor of spec writing"). That wording is a paraphrase:
  DEC-007 decision 3 says "To hand it back, you submit it", after a claim, and
  DEC-007 says its machinery "is built in roadmap milestone M13 ... Nothing
  here is needed before then". The closer text is DESIGN-010 §5c's own bullet,
  "It may claim and submit work under DEC-007", which sits outside the relay
  list. SD-1 doesn't cite it.
- **The consequences.** "It starts nothing else" is true only for a spec on an
  unsent feature. The code does more in three other cases:
  1. **A spec on a sent feature.** The reviewer's approval re-runs the
     invariant, which queues `write-dev-plan` (`rules.go:486-488`,
     `authoring.go:102-177`). DEC-006 Amendment 1 prices that at about 8,000
     tokens. It is the reviewer's decision, not the chat agent's, but the
     misuse case (an unfinished draft) can reach it.
  2. **A successor spec or plan on a feature being built.** Submitting it
     raises `revision-in-flight` and sets `spec_stale`: "New task dispatches
     are blocked" until a person answers (`rules.go:421-427`,
     `actions_phase2.go:519-548`). The chat agent can't open a successor
     itself, but it can submit one a person opened with Revise and is still
     editing. That pauses a feature under construction, which is more than a
     review's tokens.
  3. **A plan on an unsent feature.** Its approval decomposes it into tasks,
     and with an approved spec G1 moves the feature to `ready` without a Send
     (`rules.go:478-495`; SPEC-011 SD-2 accepts this).

**Evidence.** Spec SD-1, DoD 8.1; DEC-006 Amendment 1 decision 8 and its
Rationale ("the line is drawn by consequence"); DESIGN-010 §5c; DEC-005
"The facet may"; DEC-007 decision 3 and Consequences; code as cited.

**Suggested fix.** Rewrite SD-1's misuse paragraph to list the three cases,
and cite DESIGN-010 §5c's "claim and submit work" alongside DEC-005. Then
either accept them in the choice put to Sam, or narrow the tool: refuse a
successor on an `active` or `review` feature ("A revision of a feature that is
being built is a person's call: they submit it on its page."). The second keeps
the Amendment 1 test of consequence clean, and loses nothing the milestone
needs.

### R17-2 — The FR-2.8 backfill finds no person's verdict, because none is in the same transaction as its transition

**What the spec says.** FR-2.8: a verdict is a person's "when the same
transaction has a `document.human_verdict` or a checkpoint answer"; otherwise
"a verdict by an unknown giver is skipped". It reads only `document.transition`
rows with event `approve` or `request_changes`.

**The problem.** `audit_events.occurred_at` is `now()`, the transaction's time
(`0001_init.sql:163`; `store.go:99-100`), so "same transaction" means equal
timestamps. No person's verdict in the trail today is written that way:

| Path | What the trail holds |
|---|---|
| Direct approval, UI or relay | `document.human_verdict` in its own transaction (`review_send.go:383`, `auditAct`), then the transition in `approveDocument`'s (`actions.go:334`), with a `nil` payload. |
| Send-back of a design | `document.human_verdict` in its own transaction (`review_send.go:416-420`), then `returnForChanges` (`actions.go:453-454`). |
| Send-back of a spec or plan | No `human_verdict` at all. The transition's payload carries `via`, `quote` and `cause: "human issue"` (`review_send.go:444-445`). |
| An issue that returns a sent spec | Exactly the same transition payload as a send-back (`review_send.go:543`, via `returnWithIssue`), so the two can't be told apart afterwards. |
| An escalation answer | `checkpoint.responded` in `RespondCheckpoint`'s transaction (`checkpoints.go:112`); the transition comes later, from the bus, in another. |
| A design decided through `POST /ui/document/review` | `HumanApproveDocument` or `HumanReturnDocument` (`ui_entity_actions.go:498-523`), with no `human_verdict` row. |
| Adopting as approved, and *This was already approved* | `document.human_verdict` and no transition at all (`identity.go:465-477`, `store/identity.go:161`). |
| A held agent approval | `document.held`, with the review's `dispatch_id` (`store/sends.go:210-221`), and no transition. |
| A release | `document.hold_released` in its own transaction, then an approval by the role (`review_send.go:728-731`). |

So, as written, the backfill records every agent approval and send-back,
misses every held approval, loses who released a hold, and skips every
person's verdict. The in-flight `0011` has already swapped "same transaction"
for time windows and reads `document.held`, which confirms the problem.

**Evidence.** Spec FR-2.8, SD-7, FR-2.2 acceptance ("infers each case it
can"); code as cited.

**Suggested fix.** Specify the matching as the trail allows:

- a person's approval or design send-back: the latest `document.human_verdict`
  on the same document by the same actor, before the transition and after the
  previous transition;
- a spec or plan send-back: the transition's own `via` and `quote`; record it
  as a person's send-back, and say plainly that an issue raised on a sent
  spec in review can't be told apart from a send-back before `0011`;
- an escalation answer: the latest `checkpoint.responded` of kind
  `review-escalation` on the document, by the same actor, before the
  transition;
- held approvals from `document.held` with a `dispatch_id`; releases from the
  `document.hold_released` just before an agent's approval;
- approvals by adoption from `document.human_verdict` with `already_approved`;
- a person's decision on a design with no `human_verdict` row: a person when
  the actor passes SD-7's person test (R17-8), else skipped.

And add `human_verdict` rows to the two paths that lack them, so the forward
record doesn't depend on the backfill's inference (R17-3).

### R17-3 — The verdict record needs `via`, the quote, the release and the escalated run, and the approve and send-back paths don't carry them

**What the spec says.** FR-2.3 records every verdict "in the approve and
send-back paths every verdict already goes through", with `via` "from the
surface: `ui` for the document page, `inbox` for an escalation answer, `mcp`
for a relay, with its quote". FR-2.2: the rules engine changes by "one field"
on `FileAuthoredDocument`. FR-2.7: "The transition's audit row carries `via:
mcp`, and the timeline reads it."

**The problem.** `approveDocument` and `returnForChanges` receive only
`rules.ApproveDocument` and `rules.ReturnForChanges`: an actor, comments and a
dispatch id (`rules.go:40-61`). None of what the record needs reaches them:

- **The relay's `via` and quote** live in `relayAct`, which `DirectApprove`
  and `SendBack` drop before calling into those paths (`review_send.go:386`,
  `422`). The approve transition's payload is `nil` (`actions.go:334`), so
  FR-2.7's "the transition's audit row carries `via: mcp`" is true today only
  for a spec or plan send-back, not for an approval or a design's send-back.
- **An escalation's surface.** The answer reaches `approveDocument` from
  `decideCheckpointResponded` with only `RespondedBy` (`rules.go:700-711`). It
  can be given in the Inbox, on the document page (`/ui/document/review`,
  `ui_entity_actions.go:438-490`), with `subutai respond`, or with
  `POST /api/respond` (`http.go:68`). So `via: inbox` is often wrong, and
  "Approved by sam, in the Inbox" (FR-2.4) would be false for an answer given
  on the document page. `returnForChanges` also stamps every escalation issue
  `via: "ui"` (`actions.go:464`), whatever the surface.
- **The run behind an escalation.** The escalated review's `dispatch_id` is in
  the checkpoint's context (`rules.go:609`), but the answer's actions don't
  carry it. M6 follow-up 2 asks for "the review behind a verdict", and an
  escalation answer is exactly the case where a person rules on a review.
- **A release** calls `approveDocument` with the role and the dispatch
  (`review_send.go:731`); the person who released it isn't passed.

So `internal/rules` needs more than one field: `Via`, `Quote` and the release
on `ApproveDocument` and `ReturnForChanges`, and a surface on
`bus.CheckpointResponded`. `document_verdicts.via` also needs `api` (the CLI
and `/api/respond`).

**Evidence.** Spec FR-2.1 (`via`), FR-2.2 (last line), FR-2.3, FR-2.4, FR-2.7;
M6 handoff follow-up 2 and its proposal to write `dispatch_id` into the verdict
transitions' audit payload; code as cited.

**Suggested fix.** Say which structs gain which fields, and that
`CheckpointResponded` carries the surface (`ui`, `api`) and the escalated
review's `dispatch_id`. Replace `inbox` with "on the reviewer's escalation",
and let the sentence name the surface only when it is known. Write `via`,
`quote` and `dispatch_id` onto the transition's audit payload as well as into
`document_verdicts`, as M6 proposed: the audit trail stays the source of
truth, the timeline can read cause exactly rather than by time, and a future
backfill has nothing to infer.

### R17-4 — "An ID names the current revision" makes the relays and `submit_for_review` act on the wrong document while a revision is open

**What the spec says.** FR-3.1: the relays accept a document's ID as `path`,
and "an ID names the current revision". FR-1.1: `submit_for_review` takes a
path or an ID.

**The problem.** An ID without a revision resolves through
`CurrentDocumentByPublicID`, which prefers the approved revision while a newer
one is open (`store/identity.go:101-109`; SPEC-015 SD-18). That is right for a
page link, and wrong for these acts, which all need a draft or a document in
review:

- `relay_verdict` or `relay_review_request` on `FEAT-003-spec` while its
  successor is in review reaches the approved one, and `DirectApprove` refuses
  ("Only a document in review can be approved; this one is approved").
- `submit_for_review` on `FEAT-003-spec` after a person pressed Revise reaches
  the approved one and, under FR-1.2, answers "This document is already in
  review", which is false.
- `relay_issue` on the approved one works, because `RaiseIssue` follows the
  live successor (`review_send.go:574-579`). The others don't.

Revisions are the common case for these acts: an issue on an approved spec
opens one (SPEC-011 SD-7), and so does Revise.

**Evidence.** Spec FR-1.1, FR-1.2, FR-3.1; SPEC-015 SD-18; code as cited. The
ID parser already reads a `.r<n>` revision suffix (`internal/ident/ident.go:148`).

**Suggested fix.** Say that for the relays and `submit_for_review`, an ID
without a revision names the open revision when there is one, and otherwise
the current document; accept `FEAT-003-spec.r2` for an exact row; and give
FR-1.2 a separate sentence for an approved document ("This document is
approved. To change it, a person revises it, or raises an issue on it.") and
for a superseded one.

### R17-5 — "A human verdict on any document type" is deferred to a milestone that only covers decisions

**What the spec says.** SD-8: "Verdicts on documents with no template wait for
M11." The out-of-scope table and DoD 8.7 say the same.

**The problem.** The roadmap's M10 lists "Allow a human verdict on any document
type" as one of its three bullets. The untemplated types are decision, note,
research, report and policy (the starter pack ships templates only for design,
spec and dev-plan). M11 is "Decisions", and the M8 handoff's follow-up 3 asks
M11 to "give *decisions* a template or a verdict path". Four of the five types
have no destination.

The spec also misses what already exists. SPEC-015 FR-5.8 lets a person mark a
draft of an untemplated type as *This was already approved*, in the web UI,
recorded as a `document.human_verdict` (`identity.go:465-518`). That is a
human verdict on those types today, in the UI. What is missing is the chat
route, and the M8 handoff's follow-up 5 ("Relaying 'already approved' from
chat needs a line in DEC-006 Amendment 1") is neither taken up nor deferred.

**Evidence.** Roadmap §11 M10 and M11; M8 handoff follow-ups 3 and 5;
SPEC-015 FR-5.8; spec SD-8, §2 row 3, §3 out-of-scope table.

**Suggested fix.** Say that the roadmap bullet is met in the web UI for
untemplated types by SPEC-015 FR-5.8, and that FR-2.3 records it. Then put
follow-up 5 to Sam as a choice: build `relay_verdict` for an untemplated
draft as "already approved", with a dated line in DEC-006 Amendment 1, or
defer it with a named destination. If M11 is to own it, say so for all five
types, not only decisions.

## 3. Smaller findings (should fix)

### R17-6 — The timeline already reads `chat-agent` when `server.mcp_actor` is unset

§2 says the timeline "misses the chat agent altogether when `server.mcp_actor`
isn't set", and FR-2.7 says its acts "are credited to a person called
`chat-agent`". Neither is true. `LoadConfig` fills the default before it
returns: `if c.Server.MCPActor == "" { c.Server.MCPActor = "chat-agent" }`
(`internal/config/config.go:241-243`), and `featureTimeline` passes that value
as `ChatActor` (`observe.go:278-281`). The chat agent is missed only when the
configuration fails to load. FR-2.7's acceptance ("with `server.mcp_actor`
unset") passes today. The real hazard is a project that sets `mcp_actor` and
`ui_actor` to the same name: the timeline then credits every person's act to
the chat agent, and SD-7's rules can't separate them. **Fix:** drop the claim;
keep "read it from `mcpActor()`" only as a tidy-up; and say what happens when
the two actors are equal (refuse the configuration, or warn on the page).

### R17-7 — FR-2.2 misses two writers, and a successor loses its first writer

- **A successor opened by an issue.** `RaiseIssue` on an approved spec or plan
  calls `ReviseDoc` with the issue's actor (`review_send.go:581`). A relayed
  issue would read "Revision opened by the chat agent", which is SD-5's
  mistake in another place: the person raised it. Say it records
  `opened_revision` by a person, relayed, or by "an issue".
- **The cascade.** FR-2.2 says `system`, but the code passes the person who
  answered the design-revision question as the actor
  (`authoring.go:525`, `554`). Say the writer row is `system` whatever actor is
  passed.
- **`revised` has no registration to share a transaction with.** When an
  author fills an existing draft, `fileAuthoredDocument` registers nothing
  (`authoring.go:757-763`); only `SubmitDoc` runs. Say the row goes in the
  submission's transaction.
- **Successors.** The writer line reads one document row's acts. A successor's
  first act is `opened_revision`, and once it is approved it takes the
  canonical path and is the page people see. So a chat-written spec, revised
  once, reads "Revision opened by sam" and no longer says who wrote it. Say
  whether the line carries the predecessor's first writer ("Written by the
  chat agent. Revision opened by sam.").

### R17-8 — SD-7's rules leave every document added from the CLI "not recorded", and a SQL migration can't read the configured actors

The CLI sends `$USER` as the actor (`internal/client/client.go:83`), which is
neither `operator` nor ever seen `via: ui`. So every document added with
`doc add`, the route the M3 smoke checklist uses, reads "Who wrote this wasn't
recorded". The heuristics exist because `0011` is SQL and can't read
`config.yaml`. But the actors in the trail are a small, known set: role names
(from `dispatches.role`), the system actors (`orchestrator`,
`lifecycle-engine`), the MCP actor and people. "Guessing a person from an
unfamiliar actor name would be wrong as often as right" overstates it. **Fix:**
run the backfill in Go after the migrations, as `attachLateTables` already
does (`internal/store/migrate.go:94-107`), where `mcp_actor` and `ui_actor`
are known; treat any other actor that is not a role or a system actor as a
person, marked inferred. That gets much closer to the roadmap's "every
document shows who wrote it".

### R17-9 — `submit_for_review`: the held case, the audit row, and the plan

- FR-1.3 has no sentence for a spec whose approval will be held by the
  project's default hold (`specHeld` reads `HoldSpecs()` for an unsent
  feature, `review_send.go:50-67`). Add "Its review is queued; the approval
  will wait for a person."
- FR-1.1 audits `document.submitted_from_chat` beside `SubmitDoc`, which has
  its own transaction (`documents.go:154-199`). NFR-1 asks for one
  transaction. Put the marker on the transition's payload instead
  (`TransitionDocument` takes one).
- Say what a plan submitted on an unsent feature does (decomposes, and may
  make the feature `ready` before Send; R17-1), and add a sentence for it in
  FR-1.3.

### R17-10 — `get_agent_run`: attempts, applied verdicts, and where transcripts go

- `buildRunPage` quietly falls back to the latest attempt when the one asked
  for is out of range (`observe.go:502`). FR-4's acceptance wants a refusal, so
  the tool needs its own check first.
- A run's verdict isn't always the one that counted: a stale verdict is
  dropped (`rules.go:557`), an approval that missed a new issue re-queues the
  review (`rules.go:582-583`), and a held one waits. With `document_verdicts`
  keyed by `dispatch_id`, the result can say "This verdict was applied",
  "held" or "not applied, because …". Otherwise the chat agent will report a
  verdict that didn't happen.
- SD-10 says the tool gives "the same view" the web UI gives. It also sends
  that view to the chat agent's provider, which is a new place for a tool
  result holding a secret to go. Say so in DoD 8.9, and consider leaving tool
  results out of the transcript unless asked.

### R17-11 — The timeline and the page will describe the same act differently

- A document added in the UI is "Added by sam" on its page (SD-4) but "Spec
  written", by a person, in the timeline (`timeline.go:192-195`).
- The timeline reads audit rows, not the new tables. Relayed verdicts from
  before `0011` will still read as the chat agent's own there, even after the
  backfill fills `document_verdicts`.

Say whether the timeline takes its attribution from the new tables, or accept
the difference and say so.

### R17-12 — FR-3 needs store work it doesn't mention

`store.Checklist` has no `PublicID`, and `checklistCols` doesn't read the
column (`internal/store/checklists.go:24-33`); there is no
`ChecklistByPublicID`; `checklistByRef` takes a row id or a name only
(`mcp_checklist_tools.go:31-36`); and `ui_identity.go:72` still says
checklists have no page. None of it is hard, but FR-3 reads as display-only.
List it, and have `relay_tick_job`'s `checklist` description mention the ID.

### R17-13 — Overlaps with M13 and M6 that should be named

- DEC-007's Consequences say "Tasks and documents gain an executor field".
  `document_writers` is that record for documents. Say so, so M13 builds on it
  rather than adding a second.
- DEC-007 decision 10 (a stronger reviewer model when the chat AI did the
  work) now has the data it needs. Name M13 as its destination in §6.
- M6 follow-up 4 has two halves: a "written by" link, which this spec takes,
  and a link from each review comment to its run, which it neither takes nor
  defers.

### R17-14 — Small corrections

- The status line points to "§5, DoD 8" for the nine choices; §5 is the
  non-functional requirements. The choices are in §3.
- §2 row 2 cites SPEC-011 FR-3.1 for "submitting a spec or plan queues its
  review, for a sent or unsent feature alike". FR-3.1 is the revise loop. The
  behaviour is true, and comes from `rules.go:411-433` and `queueReview`, which
  doesn't check the sent mark (`actions.go:241-291`).
- The Authority list cites DEC-004 *Amendment 1* for authoring the planning
  layer; Amendment 1 adds milestones and roadmaps, and the base decision grants
  document authoring and "trigger a review".
- FR-5.2's "a draft nobody has submitted" means "a draft that doesn't wait for
  its author". An escalation answered "ask for changes" with no reason returns
  a draft with nothing against it (`rules.go:707-711`), which was submitted.
- `create_feature`'s description still says a person starts the feature "when
  its specification has been approved" (`mcp.go:104-107`). FR-7 is the place
  to bring it in line with Send.
- FR-2.6 lists "the relays" among the tools whose document entry gains
  `written_by`, but the relays return `relayResult`, which has no document
  entry (`mcp_relay.go`, `relayResult`). Say it gains one.

## 4. Checks that passed

- **The end-to-end path (FR-5, FR-6).** Submitting a chat-written spec on an
  unsent feature queues `review-spec`: the rule queues a review on any
  `draft → reviewing` (`rules.go:411-433`), and `queueReview` checks the
  reviewer role and the agent-review setting, not the sent mark
  (`actions.go:241-291`). The reviewer's approval is applied unless the
  project holds specs by default (`specHeld`, `review_send.go:50-67`). An
  approved spec on an unsent feature stays put: `ReconcileAuthoring` finds
  nothing owed without the mark (`authoring.go:117-121`), the heartbeat sweeps
  sent features only, and G1 fails for want of a plan, quietly
  (`actions.go:557-596`). After Send, `neededAuthoring` sees an approved spec
  and asks for `write-dev-plan`, so no `write-spec` is queued; the plan's
  approval decomposes, re-runs G1 and queues the estimate
  (`rules.go:478-495`), and nothing implements until Start building. The send
  screen already marks both spec steps done (`ui_send.go:146-183`).
  `adopt_document` needs the file committed and clean
  (`identity.go:296-313`), which FR-6.1 step 1 does.
- **The ID claims (FR-3).** Milestone, roadmap and checklist MCP results give
  the row id as `id` (`mcp_plan_tools.go:180`, `210`;
  `mcp_checklist_tools.go:127`). Milestone and roadmap lookups take `MS-` and
  `RM-` (`http_phase3.go:348-360`, `446-456`); checklist lookups don't.
  `/ui/id/CL-…` finds nothing (`ui_identity.go:71-73`). The milestone and
  roadmap pages show their IDs (`entity.html:640`, `723`); `plan.html` and
  `checklist.html` don't. The relays take a path only
  (`mcp_relay.go`, `relayTarget`).
- **The timeline's current attribution**, apart from R17-6. A relayed verdict
  is audited to the MCP actor and so shown as the chat agent's
  (`timeline.go:395-396`); an agent's verdict is shown with its run as cause;
  an escalation answer shows as the person's.
- **The boundary.** Apart from SD-1 (R17-1), nothing here lets the chat agent
  hold a verdict, send, start building, override a gate or answer a
  checkpoint. `get_timeline` and `get_agent_run` read what the web UI already
  shows, and DEC-004 grants "query state". NFR-2's additions to the must-not
  list are sensible; `submit_document` is the dispatched author's outcome tool
  and must never be advertised to chat.
- **Stale and cancelled verdicts.** A verdict for a document that has moved on
  produces no action (`rules.go:557`), and a person's send-back cancels a
  queued review (`review_send.go:458-483`), so neither reaches the approve or
  send-back paths, and FR-2.3 rightly records nothing for them (R17-10 is
  about saying so).
- **Detach.** `store.DetachDocument` deletes the `documents` row
  (`store/sends.go:304-334`), so `ON DELETE CASCADE` does remove the new rows
  with the registration, as FR-2.1 says.
- **Coordination.** `0011` is free, and the runner fills gaps
  (`migrate.go:127-134`), so M9's `0012` is safe in either merge order.

## 5. The choices

The nine in DoD 8 are the right kind of question. Some comments:

- **Choice 1 (no quote to submit).** Put R17-1's three cases in front of Sam,
  and offer the narrower tool as the alternative to "keep the quote".
- **Choice 3 ("Written by the chat agent" for what it adds).** Reasonable, but
  the chat agent attaching a file the person wrote ("attach my notes") will
  read as the chat agent's. The spec's §6 names edits after registration;
  name this case too.
- **Choice 5 (held approvals as verdicts).** Right, and the backfill can
  support it from `document.held` (R17-2).
- **Choice 6 (infer only where clear).** Offer R17-8's Go backfill as the
  alternative. It is the difference between most documents saying who wrote
  them and many saying "not recorded".
- **Choice 7 (untemplated types wait for M11).** Needs R17-5's correction
  before it can be asked.
- **Choice 9 (transcripts to chat).** Say that the transcript leaves Subutai
  for the chat provider (R17-10).

Decisions the spec takes silently and should list for Sam:

1. **Relaying "already approved" from chat** for untemplated types (R17-5,
   M8 follow-up 5). If yes, it needs a line in DEC-006 Amendment 1.
2. **What an ID means for an act** while a revision is open (R17-4).
3. **What happens when `mcp_actor` equals `ui_actor`** (R17-6).

## 6. Disposition

*By the author, 2026-09-28.* All fourteen findings are accepted, and none is
rejected. [SPEC-017 §7](../specs/SPEC-017-chat-as-a-proper-seat.md#7-changes-after-review)
says how each was dealt with. In short:

- **R17-1.** `submit_for_review` now refuses a revision on a feature being
  built. SD-1 lists the four things a submission can start, and cites
  DESIGN-010 §5c.
- **R17-2 and R17-8.** The backfill matches by time window, and runs in two
  passes: SQL for what the trail pins down, then Go at boot with the
  configured actor names.
- **R17-3.** The server gained `approveDocumentAs` and `returnForChangesAs`.
  `internal/rules` changes by one field only. An escalation answer records the
  review it ruled on, and its sentence names no surface.
- **R17-4.** An ID in a relay names its newest revision that isn't
  superseded (SD-11).
- **R17-5.** The roadmap bullet holds in the web UI through SPEC-015 FR-5.8.
  Relaying "already approved" from chat is put to Sam, with M11 named for all
  five types (SD-8).
- **R17-6.** The false claims are removed. The configuration refuses one
  actor name for both surfaces (SD-12).

The three silent decisions in §5 are now choices 7, 10 and 11 in the spec's
DoD 8.
