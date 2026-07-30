# DEC-005: The boundary is bypassing the orchestrator, not spawning agents

**Status:** **Accepted — Sam, 2026-07-29.** Drafted by Claude (Opus 5); the
author cannot be the approval gate, and was not. Sam's answers to the four
questions are recorded in "What Sam decided" below.
**Date drafted:** 2026-07-29
**Date accepted:** 2026-07-29
**Decided by:** Sam
**Supersedes:** [DEC-004](DEC-004-mcp-planning-authoring.md) in part — the third
bullet of its "may not" list, and the unresolved contradiction between its "may"
and "may not" lists
**Corrects:** [vision-v1](../vision/vision-v1.md) §7's characterisation of the
kanbanzai failure
**Evidence:** [alignment review](../notes/vision-alignment-review-2026-07-29.md) §2

## Decision

The line the MCP facet may not cross is **doing development work itself, or
reaching around the orchestrator to drive the pipeline by hand**. It is *not*
"causing agents to run".

**The facet may:**

- everything DEC-004 already granted on the planning side — create and edit
  initiatives and features, set titles and descriptions, attach documents and
  author document content;
- **ask the orchestrator to do something the orchestrator would do anyway** —
  submit a document for review, request a re-review, trigger indexing. The
  orchestrator decides whether, when and with what role; the caller is making a
  request, not issuing a dispatch.

**The facet may not:**

- **implement, review, verify or otherwise perform pipeline work itself.** This
  is the kanbanzai failure and it is the heart of this decision. A chat agent
  that writes the code instead of asking Cromwell to have it written has become
  the thing the architecture exists to prevent.
- **construct or direct a dispatch** — choose the role, the model, the prompt or
  the worktree for an agent run. Dispatch composition is the orchestrator's,
  always.
- **override a gate, or answer a checkpoint whose answer overrides a gate.**
  Unchanged from DEC-004, and the one prohibition that is about *authority*
  rather than about orchestration.
- **start a feature into active development.** Unchanged from DEC-004: the
  moment work crosses the planning/developing seam is a human act at the command
  centre.

## Context

DEC-004 contains a contradiction that has never been resolved. Its **may** list
ends by granting "everything the vision already granted it: query state, search,
help a human respond to a checkpoint, trigger a review" — while its **may not**
list forbids spawning reviewer agents and overriding gates. Triggering a review
spawns a reviewer. Responding to a checkpoint can override a gate. The document
permits and forbids the same two acts, ten lines apart.

That contradiction was not carelessness. It is what happens when a rule is drawn
from a misdiagnosis: the prohibition and the permission were reasoning about
different things, and neither was wrong on its own terms.

**The misdiagnosis.** Vision §7 records the kanbanzai failure as the
MCP-connected chat agent becoming a "rogue orchestrator", which reads as *the
chat agent caused agents to run*. The kanbanzai record says otherwise. The
failure was an AI agent *holding* the orchestrator role and forgetting the
orchestration skill — either performing the work itself, or dispatching
subagents and then losing context before closing the loop, leaving tasks queued
while the parent was marked done. Kanbanzai's own analysis concluded the drift
was "the *predicted behaviour* given the U-shaped attention curve", not a
model defect. The alignment review quotes both sources in full.

**So dispatching subagents was the cure.** It is what the orchestration skill
told the agent to do, and the failures are the cases where it stopped doing it.
A rule forbidding it inverts the lesson.

## Rationale: what makes a request safe is who decides, not who asks

Cromwell's orchestrator is code — deterministic Go over an event bus, with no
model attached and no context window (vision §8, [`internal/rules/`](../../internal/rules/)).
It cannot drift, cannot forget its constraints mid-run, and cannot be argued out
of a gate.

That changes what "trigger a review" means. In kanbanzai, an agent asking for a
review was an agent *deciding* to run one. In Cromwell, the request enters a rule
engine that decides whether the document is in a reviewable state, which role
applies, and whether a gate blocks it — and would have queued that same review
itself when the document transitioned. The caller supplies an intention. The
code supplies every decision that carries authority.

This is the principle: **the orchestrator's authority is safe to *invoke* exactly
because it is not delegable.** No caller can widen it, so the set of callers does
not need to be narrow. What must stay narrow is the set of actors who can
*bypass* it — write code into a worktree, compose a dispatch, force a
transition — because those are the acts that route around the deterministic
part.

Two prohibitions survive unchanged because they are not orchestration questions
at all. Gate override is an *authority* question: someone is deciding that a
safety rule should not apply, and that is a human judgement. Starting a feature
is a *seam* question: it is the moment planning becomes development, and the
vision reserves it for a human at the command centre.

## Consequences

- **`dispatch_review` becomes permissible**, and mostly redundant. The rules
  engine already queues a review when a document transitions to `reviewing`
  ([`rules.go:202`](../../internal/rules/rules.go:202)). The tool's real use is
  re-requesting a review that failed or was superseded. It should be specified as
  *request*, not *dispatch*, and named accordingly.

- **`checkpoint_respond` stays out in its general form**, on the surviving
  authority prohibition rather than the orchestration one. A blanket respond tool
  would let a chat agent answer a `gate-override` checkpoint with "override",
  which no reading of any decision here permits.

- **A read-only checkpoint tool is clearly permissible and should be built.** The
  vision's phrase was "*help* a human respond". Fetching the pending checkpoints,
  explaining what is blocked and why, and drafting a recommendation the human
  then acts on in the UI needs no authority at all, and delivers most of what
  that phrase was reaching for.

- **If a filtered respond tool is ever wanted**, it is a design change to be
  named, not a drift. Today the boundary is enforced by *omission* — the tool
  does not exist, and [`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`](../../internal/server/integration_mcp_test.go:75)
  fails the moment the advertised set changes, so widening the facet is always
  deliberate. A per-kind runtime filter is a weaker guarantee of a different
  kind, and swapping one for the other deserves its own decision.

- **The authoring gap is unblocked.** Nothing here forbids an author agent that
  drafts a spec or dev-plan, because such an agent is dispatched by the
  orchestrator like any other. Vision §2 places spec shaping on the planning
  side — "humans (with AI help)" — so an AI author is that half's specified
  assistance. This clears the way for the design-approval gate Sam wants, which
  the alignment review records as unbuilt (R-2).

- **Vision §7 should be annotated**, not rewritten. Its list of what the facet
  does not expose stands, but "raw agent spawning" needs a footnote saying what
  the kanbanzai failure actually was, so the next reader does not re-derive
  DEC-004's prohibition from the same sentence.

## Alternatives considered

- **Leave DEC-004 as written.** Rejected: it is internally contradictory, and the
  contradiction has already cost one round of analysis. It also forbids the
  authoring agent the vision specifies, which would block Sam's first gate on a
  rule drawn from a misreading.

- **Amend DEC-004 in place.** Rejected on process grounds. DEC-004 is accepted
  and dated, and the audit trail is this project's thesis. A superseding decision
  that says what changed and why is the honest record; a silent edit is not.

- **Permit checkpoint responses filtered by kind.** Deferred, not rejected. It
  may well be right, but it trades confinement-by-omission for a runtime check,
  and that trade should be made deliberately and on its own evidence rather than
  folded into this decision.

- **Draw the line at "no tool may cause a model to be invoked".** Rejected: it is
  crisp but wrong in both directions. It forbids a harmless re-review request,
  and permits a chat agent to write code into a worktree by hand, which is the
  actual failure.

## What Sam decided

1. **The reframing holds.** Bypassing the orchestrator is the line; causing
   agents to run is not.
2. **`dispatch_review` is not wanted as a named tool.** It is redundant — the
   rules engine already queues a review when a document transitions to
   `reviewing`. A re-request tool may be added later if a real need appears, and
   would be specified as a *request*, never a dispatch.
3. **Both surviving prohibitions confirmed:** no gate override (including via a
   checkpoint answer), and no starting a feature into development. Those remain
   human acts at the command centre.
4. **A read-only checkpoint tool is wanted in MCP slice 2.** Fetching pending
   checkpoints, explaining what is blocked and why, and drafting a
   recommendation the human then acts on in the UI. This is what "help a human
   respond" meant, and it needs no authority.

The decisive practical consequence: under DEC-004 as written, a chat agent could
not ask Cromwell to start writing specs — the tool would spawn agents and was
forbidden. That prohibition would have blocked the conversational path to Sam's
own design-approval gate. Under this decision it is a request to the
orchestrator, and permitted.
