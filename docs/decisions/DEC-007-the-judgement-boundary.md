# DEC-007: Who does the work is flexible; who judges it is not

**Status:** **Accepted — Sam, 2026-09-28** (roadmap decision 10, accepting the
proposal in the [status report](../notes/subutai-status-and-roadmap-2026-09-28.md#dec-007-who-does-the-work-is-flexible-who-judges-it-is-not)
§6). Drafted by Claude from that accepted proposal. Where this document adds
detail the proposal didn't have, it says so, so Sam can check it.
**Date accepted:** 2026-09-28
**Decided by:** Sam
**Supersedes:** [DEC-005](DEC-005-the-orchestration-boundary.md) in part: the
first bullet of its "may not" list ("implement, review, verify or otherwise
perform pipeline work itself").
**Builds on:**
[DEC-006](DEC-006-humans-start-development.md) and its Amendment 1, which
settled that the chat agent may *carry* a human's verdict but never hold one of
its own.
**Evidence:**
- the [discussion response](../notes/subutai-discussion-response-2026-07-31.md)
  §2c;
- the [alignment review](../notes/vision-alignment-review-2026-07-29.md) §2 (the
  corrected kanbanzai diagnosis);
- [Subutai and GitHub](../research/subutai-and-github.md) §8.

## Decision

**Any stage's work may be done by a dispatched agent, by the chat AI, or by a
human. No stage's work may be judged by whoever did it, and no path skips a
gate.**

1. **Every piece of work records its executor.** An executor is one of:
   - a dispatched agent, with its role and model;
   - the chat AI;
   - a human.

   It is shown on the task and document pages and in the audit trail.

2. **Doing stages and judging stages are different.**
   - **Doing stages** produce something: writing a spec, writing a development
     plan, implementing a task, estimating, running a spike. Any executor may
     do them.
   - **Judging stages** give a verdict: spec review, plan review, code review,
     verification. Only an independent party may do them:
     - a dispatched agent reviewer or verifier;
     - or, for documents, a human verdict (DEC-006 Amendment 1).

     The executor of the work can never judge it.

3. **There is one way in and one way out.**
   - To take work, you **claim** it. That returns the working copy (for a task,
     the feature's worktree) and the contract (the spec and the task).
   - To hand it back, you **submit** it.
   - The state machine refuses anything out of order, exactly as it does for
     dispatched agents.

   For the chat AI these are MCP tools (`claim_task`, `submit_task`). The rules
   are written into the tool descriptions and results, because that is what an
   agent reliably reads. A chat-side `work-a-task` skill backs them up.

4. **The chat AI never holds a verdict.** No MCP tool lets it approve, send
   back, review or verify on its own judgement. That is enforced by leaving the
   tools out, and checked by the MCP tool-set test. It may *relay* a human's
   verdict, under DEC-006 Amendment 1.

5. **Verification always runs, by the dispatched verifier.** It checks the
   finished feature against every acceptance criterion in the spec, with
   evidence for each. No executor, human or AI, can skip it, waive it or do it
   in chat.

6. **Claims expire.** A claimed task with no submission and no activity for a
   configured time raises a "still working on this?" checkpoint. This is the
   same safety net the heartbeat gives dispatched work.
   - *Added detail:* the proposed default is 24 hours of inactivity, set per
     project. The implementation spec will define what counts as activity.

7. **Work outside a claim gets noticed.** The git watcher raises a checkpoint
   when a commit touches an active feature's paths and no task is claimed.

8. **Chat work is marked as unmeasured.** Subscription tokens don't reach the
   ledger, so work done by the chat AI records its actuals as *unmeasured*, not
   zero. Calibration and forecasting leave unmeasured work out of their
   figures.

9. **A colleague's pull-request review counts** when a human did the task. If
   the project uses GitHub and a human implements a task, a different person's
   approving pull-request review is the independent code review. Subutai
   records that reviewer, and doesn't dispatch a second opinion. The verifier
   still runs at feature level (Subutai and GitHub §8).

10. **Reviews default to a stronger model when the chat AI did the work.** A
    feature done entirely in chat has had one mind through the whole chain, so
    the independent review carries more of the weight.
    - *Added detail:* this is a per-project configuration default: a reviewer
      model to use when the executor is the chat AI. It is not an
      architectural rule.

## Context

DEC-005 drew its first prohibition from kanbanzai's failure: a chat agent that
"writes the code instead of asking Cromwell to have it written has become the
thing the architecture exists to prevent". DEC-005's own context section
corrected the diagnosis behind that rule. Kanbanzai failed because **an AI held
the orchestration role and dropped it**. The state lived in the same drifting
context as the work, so when the context filled up, the loop was abandoned and
the records lied. The prohibition on *doing work* was standing in for the real
rule.

The practical pressure is real too. When a feature is small but hard, the
tempting move is to open a chat window with the strongest model and say "just
fix it". If Subutai forbids that, it still happens, just outside the system,
with no claim, no review, no verification and no record. Giving that path a
proper lane is safer than a prohibition that won't hold.

## Rationale

**In Subutai the state is held by code, so the real rule can be enforced
directly.** A chat agent that claims a task, works and submits hasn't taken
over orchestration:
- the gates still fire;
- G2 still refuses a feature whose tasks aren't finished;
- the heartbeat still notices stalls;
- the verifier still runs.

The kanbanzai failure can't recur through this path, because the state no
longer lives in anyone's context.

**Judging stays closed so that doing can be open.** Every safeguard that
matters is a judgement: a review, a verification, a gate. Keeping judgement
with independent parties, and never with the executor, means that opening up
who does the work weakens nothing that protects quality.

**This matches DEC-006.** DEC-006 keeps the acts that commit resources (Send to
development, Start building) with a human at the command centre. DEC-007 lets
the chat AI and humans do work once it has started. They don't overlap: one
decides *when*, the other decides *who may do the work*.

## Consequences

- **Tasks and documents gain an executor field.** Actor and model attribution
  becomes explicit rather than implied by the dispatch row.
- **New MCP tools: `claim_task` and `submit_task`.** The MCP tool-set test names
  them.
- **The heartbeat extends to chat and human claims**, and the git watcher gains
  the unclaimed-commit check.
- **The ledger gains an `unmeasured` marker**, and calibration leaves those
  rows out.
- **Configuration gains** the claim-expiry time and a reviewer-model override
  for chat-executed work.
- **The pull-request review rule** takes effect with the GitHub projection
  (roadmap M15).
- **This is built in roadmap milestone M13 (Executors).** Nothing here is
  needed before then.

## The honest costs

- **The chat agent's tools can't be restricted.** A dispatched implementer has
  only its tools. A chat agent in an editor has the whole filesystem, and
  nothing stops it editing outside a claim. The skill, the tools' own guidance,
  the human in the conversation, and the unclaimed-commit checkpoint reduce the
  risk. They don't remove it. That is accepted, because today the same risk
  exists with no sanctioned path and no safety net at all.
- **The token ledger has holes.** Chat work is permanently unmeasured. That is
  the price of subscription-backed work.
- **A human approving a spec they helped write is close to judging their own
  work.** DEC-006 Amendment 1 allows a human verdict on any document, and this
  decision keeps that. It is a visible, recorded human choice, and the
  attribution makes it answerable in a retrospective. Normally a spec's author
  is the dispatched spec agent anyway.

## Alternatives considered

- **Keep DEC-005's prohibition.** Rejected. It forbids the proxy, not the
  failure, and it pushes the "just fix it" case outside the system, where
  nothing checks it.
- **Allow chat execution, with chat self-review for small tasks.** Rejected.
  That is the rubber stamp in its purest form: the co-author approving its own
  work.
- **Allow chat to verify when a human supervised the work.** Rejected. The
  definition-of-done check is the one step no actor can skip, and keeping it
  dispatched is what makes that claim true.

## Not decided here

- **Exactly what counts as activity on a claim**, and whether expiry differs
  for humans and for the chat AI. This is for the M13 spec.
- **Whether the chat AI may claim estimation work.** It's allowed under
  decision 2, but it would produce unmeasured estimates of measured work. The
  M13 spec should say whether that's useful.
