# DEC-006: A human decides when development starts

**Status:** **Accepted — Sam, 2026-09-28.** Drafted by Claude. The author
can't be the approval gate, and wasn't.
**Date drafted:** 2026-09-28
**Date accepted:** 2026-09-28
**Decided by:** Sam
**Amends:**
- [DEC-005](DEC-005-the-orchestration-boundary.md): adds a third kind of thing
  the chat facet may do (relaying a human act that starts nothing), and extends
  its "no starting development" prohibition to the new button.
- [SPEC-009](../specs/SPEC-009-the-authoring-chain.md): FR-4.1, FR-4.3 and
  FR-4.7 (what sets spec writing off) and FR-9 (when the cascade rewrites).

**Revises:**
- [DESIGN-010](../design/DESIGN-010-subutai.md) Draft 1, §5, §5a and §12 items
  2 and 3. These were never accepted.
- [DESIGN-009](../design/DESIGN-009-the-authoring-half.md) §4, where approving a
  design meant starting the chain.

**Withdraws:** the recommendation in the
[discussion response](../notes/subutai-discussion-response-2026-07-31.md) §2a
that the chat AI writes the spec.

**Evidence:**
- the [status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §6;
- the [SPEC-009 Stage 1 walkthrough](../walkthrough-spec-009-stage1.md).

## Decision

**Approving a design records what we want. It starts nothing. Development
starts when a human presses Send to development.**

1. **Approving a design is a record, not a trigger.** A design moves to
   `approved` exactly as it does today: a human decides and an agent reviewer
   only comments. But nothing is dispatched because of it.

2. **Send to development starts the agents.**
   - The button appears on a feature whose design is approved. That can be the
     feature's own design or its immediate parent initiative's, which is the
     existing G0 rule.
   - It also appears on an initiative, to send several of its features at once.
     The send screen lists them with checkboxes.
   - A feature can only be sent when G0 passes and it has a description.
     Otherwise the button explains why, in G0's own words.
   - Sending is a web UI act only. **No MCP tool can send.**

3. **The send screen shows the cost before anyone commits:**
   - the features being sent;
   - the steps that will run, each with its role and model, and whether it is
     already done;
   - a rough token forecast;
   - how many agent slots are free (the project's `dispatch.workers`, 4 by
     default).

   Work that doesn't fit waits in the dispatch queue, as all dispatches do
   today.

4. **After the button, the orchestrator runs development planning:**
   1. It writes the spec.
   2. It reviews the spec. The reviewer checks it is faithful to the design,
      which is SPEC-009's fidelity bar.
   3. Human spec approval, if it is on.
   4. It writes the development plan.
   5. It reviews the plan.
   6. It decomposes the plan into tasks and estimates the work.

   Then it stops. **Start building**, the existing second gate, is still the
   human act that begins implementation. **A step whose document already
   exists is skipped**, so a spec or plan written in chat is used as it is.

5. **Human spec approval is optional, and off by default.**
   - A project setting turns it on for every spec, and the send screen can turn
     it on for a single send.
   - When it is on, the agent review still runs and its findings still appear.
     But the spec only becomes `approved` when a human approves it on the
     document page.
   - A human's *request changes* sends the spec back to the spec agent, as a
     reviewer's would.
   - Human spec approval is a UI act. The chat agent can't relay it, because it
     is a verdict on an agent's work.

6. **The agent spec review is on by default, and a project may turn it off.**
   - If a project turns off both the agent review and human approval, a spec
     that passes validation is approved without review.
   - The send screen says so in plain words ("this spec will not be reviewed"),
     and the audit trail records it as a project setting, not as an approval.

7. **A revised design cascades only into work that has been sent.**
   - SPEC-009 FR-9 is unchanged in how it chooses: one affected spec is
     retired directly, and several raise one keep-or-redo checkpoint.
   - What changes is when it rewrites. A retired spec is rewritten
     automatically only if its feature has been sent to development.
   - For an unsent feature, the retired spec simply leaves the feature without
     one until someone sends it.

8. **The chat agent may relay a human's act when that act starts nothing.**
   - This is a third kind of MCP permission, alongside DEC-005's planning
     authoring and requests the orchestrator decides on.
   - Two acts qualify: **approving or sending back a design**, and **ticking a
     job on a checklist** (when checklists exist).
   - Each relay is audited with `via: mcp`.
   - The only relay tools are ones named in this list, and adding one needs a
     decision.

9. **The buttons are named Send to development and Start building.** Today's
   "Start work" is renamed.

## Context

When the orchestrator takes over has been answered three ways in three months.

- **SPEC-009 as built (July).** Approving a design set the agents writing specs
  at once (FR-4.3). The live run on 31 July showed it working: 15,976 tokens
  from approval to gate 2, with no further human act.
- **The Subutai discussion (31 July).** It moved the seam to the spec. The chat
  AI would write the spec and orchestration would start when the spec was
  submitted. The argument was that the spec matters most and deserves the
  strongest model, which is affordable on a chat subscription.
- **Sam, 28 September.** Two observations settle it:
  - **A well-crafted design normally makes a tight spec.** Once the design
    conversation has done the thinking, writing the spec is careful
    translation. An API agent does that well, and the fidelity check exists to
    catch it when it doesn't.
  - **Starting agents costs something that has limits.** It uses agent slots,
    parallel jobs and token budget. Whether *now* is a good moment is a
    judgement about resources, and it belongs to a person. July's automatic
    trigger made that decision for them, at the moment of approval, when
    nobody was thinking about capacity.

The two halves of July's build were never in conflict. The chain (spec agent,
review, plan agent, review, decomposition) was right, and only its trigger was
wrong. This decision keeps the chain and moves the trigger to a button.

## Rationale

**Approval and commitment are different acts.** Approving a design answers
"is this what we want?". Sending it answers "should we spend on it now?". July
fused them, so a design couldn't be approved without spending. That pushed
people to delay approving designs they agreed with, which is the wrong
incentive. Separating them lets a backlog of approved designs exist, ready to
be sent in whatever order and at whatever pace capacity allows.

**The human act sits at the point of spending.** DEC-005 kept one prohibition
because it is "a *seam* question: the moment planning becomes development".
Under July's trigger that seam was crossed by approving a design, an act the
chat agent was about to be allowed to relay. Under this decision it is crossed
by the button, and the button is UI-only. So the rule DEC-005 protected holds
more cleanly than it did.

**Once approval starts nothing, relaying it is safe.** DESIGN-009 §5.1 wanted
chat approval of designs, and identified it as new authority that needed a
DEC-005 amendment. At the time, relaying an approval meant relaying a trigger.
Now it relays a record, so the amendment is small. That is the same test
applied to ticking a job: the human did the thing, the chat agent reports it,
and nothing downstream moves on its own.

**Spec review stays by default because it is the cheapest check we have.** In
the live run the review cost about 2,900 tokens, against about 4,900 to write
the spec. It is the only independent check between an approved design and
everything built from it. Sam's experience is that a good design usually makes
a good spec. The review is for the times it doesn't, and "usually" is exactly
the case a cheap check exists for. Projects that disagree can turn it off, and
the send screen makes that visible.

**Human spec review is off by default.** It is there for the people who
insist on it, and for a feature where someone wants to look before the plan is
written. It costs a human's attention on every spec. The design conversation
has already had that attention, and spending it twice by default doesn't earn
its keep.

## Consequences

**Kept from SPEC-009 Stage 1, unchanged:**
- the `spec-author` and `dev-plan-author` roles and their skills;
- G0;
- the fidelity bar (FR-8);
- per-type approval authority (FR-2);
- `submit_document`;
- the revision cascade's selection rules (FR-9.1 to FR-9.2);
- the heartbeat safety net;
- decomposition and G1.

**Changed:**
- **Features gain a "sent to development" mark**: who sent it, when, and any
  per-send settings. The mark is not a new lifecycle state. A sent feature is
  still `idea` until its contract is approved, exactly as now. The
  implementation spec can revisit that if it proves awkward.
- **The spec invariant (FR-4.1) gains a condition.** It now reads: every sent
  feature that G0 admits and that has a description has a current spec.
- **The triggers (FR-4.3) change.**
  - Approving a design, creating a feature, and adding a description no longer
    set spec writing off on their own. They still re-check the invariant,
    which now needs the mark.
  - Sending is a new trigger.
- **The heartbeat sweep (FR-4.7) only fills gaps in sent features.**
- **The cascade (FR-9.4) only rewrites for sent features**, as in decision 7.
- **The starter pack ships `write-spec` and `write-dev-plan` switched on.**
  Leaving them commented out was the safety catch while approval was the
  trigger. The button replaces it. This settles the third Stage 2 entry
  criterion in the
  [handoff](../notes/handoff-2026-07-30-stage1-complete.md).
- **"Start work" becomes Start building** in the UI.

**Added:**
- the Send to development button on feature and initiative pages, with the
  send screen;
- the project setting and per-send switch for human spec approval, and the
  project setting that turns agent spec review off;
- two MCP relay tools: one to approve or send back a design, and later one to
  tick a job, both audited `via: mcp`.

`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` must be updated on
purpose, and it keeps failing on any tool this decision doesn't name.

**Satisfied:** the second Stage 2 entry criterion (a DEC-005 amendment for
approval relay) is met by decision 8.

**Not changed:** Start building (gate 2), implementation, code review,
verification and merge.

**The honest limitation carried forward.** Cromwell can't verify that the human
actually said what the chat agent relays. Until per-user identity exists, a
relayed approval is audited to the configured MCP actor with `via: mcp`. That is
acceptable for a single operator, and per-user identity is scheduled before the
Tickly pilot (roadmap decision 14). Nothing relayed under this decision starts
work, which bounds the cost of a mistaken relay: a design shows as approved
when it wasn't, and someone notices before they press Send.

## Alternatives considered

- **Keep July's trigger: approval starts the chain.** This is the simplest
  option, and it is proven live. Rejected because it takes the spending
  decision away from a person, and it made relaying approval from chat
  equivalent to starting work from chat.
- **The chat AI writes the spec, and orchestration starts at the spec (31
  July).** Rejected. It puts the most mechanical step on the most expensive
  model and on the human's time, and it discards working code for little gain.
  It survives as an option, not a rule: a spec written in chat is used and the
  step skipped (decision 4).
- **Approve the spec, then press a separate button to start planning.** This
  would be three human acts before implementation instead of two. Rejected. The
  optional human spec approval gives the people who want it that extra stop,
  without making everyone pay for it.
- **Let the chat agent press Send.** Rejected. It is the seam DEC-004 and
  DEC-005 both reserve for a human at the command centre. The resource
  judgement is the human's, and a chat agent under "make progress" pressure is
  the wrong party to make it.
- **Enforce a token budget at the button.** Deferred, not rejected. The send
  screen shows the forecast, and the existing runaway cap still applies. A hard
  refusal at send time needs better forecasts than we have (see
  [token-estimation](../research/token-estimation.md)) and can be added once
  forecasting from past throughput exists.

## Not decided here

- **Withdrawing a send** before its spec is written. Abandon exists, but
  "un-send" might be wanted. Decide it when the button is built.
- **Whether a project may turn off both reviews.** Decision 6 allows it,
  visibly. If that proves a foot-gun, requiring at least one review is a small
  follow-up.
- **The design reviewer's future.** The 31 July discussion leaned towards
  retiring it. This decision leaves it as it is: it comments and never rules.
- **Who may do a stage's work**, including a human or the chat AI implementing
  a task. That is [DEC-007](../notes/subutai-status-and-roadmap-2026-09-28.md#6-dec-006-and-dec-007-in-detail),
  accepted in principle on 2026-09-28, and its document is still to be written.
