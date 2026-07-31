# Walkthrough: SPEC-009 Stage 1 — the authoring chain, live

**Date:** 2026-07-31
**Project:** the throwaway smoke project (`/tmp/cromwell-smoke`, DeepSeek via
its Anthropic-compatible gateway, every role on `deepseek-chat`)
**Covers:** SPEC-009 DoD 2, the load-bearing claim. DoD 3 (the revision
cascade) is still to run.

This is the honest record, warts included. Two runs happened, one of them by
accident.

## Run zero: the safety net fired before anyone asked it to

The smoke server was rebuilt onto the Stage 1 binary at 23:31 UTC on
2026-07-30. Within one heartbeat, the new reconciliation sweep (FR-4.7) found
what the previous session had left deliberately standing: `greet/hello` with
an approved spec and no dev-plan. It dispatched `write-dev-plan` unprompted at
23:32:12; DeepSeek wrote the plan, the reviewer approved it forty seconds
later, and decomposition plus G1 carried the feature to `ready`.

Sam then pressed Start, and the existing implementation loop ran both tasks
through implement → code review → verify: **`greet/hello` is `done`**. So the
dev-plan half of the chain and the whole implementation loop were exercised
live before the design half — by the safety net doing its job on a genuinely
lost event, which is a better test of FR-4.7 than any we could have staged.

## The way in: gate 1, the long way round

Sam attached two design documents from the app and immediately found two
surface gaps, both real:

1. **A mis-attached document cannot be detached.** Registration has no
   inverse anywhere — UI, MCP, API or CLI. The wrong attachment was removed
   by hand in the database. A Detach action (gated to `draft`) is proposed
   but unbuilt.
2. **A draft cannot be submitted from the app**, so the Approve panel — which
   only appears in `reviewing` — was unreachable. Submission is Stage 2
   surface work; for this smoke it went through `/api/docs/submit`.

The design itself started as one sentence. It was expanded into the
template's shape (front matter, three sections) to pass mechanical
validation, and submitted. Then the FR-2 loop ran exactly as designed:

- The **design reviewer commented and could not rule** — `submit_comments`
  has no verdict field. Its better comment: "returns the current date and
  time" doesn't say what *now* means across timezones.
- **Sam sent it back** from the document page (Ask for changes → `draft`).
- Sam edited the file — his own Decisions now, including "the value is the
  runtime's local clock", answering the reviewer — committed it, and the git
  hook reindexed the draft.
- Resubmitted; the reviewer took a second round against the revision; **Sam
  approved from the app's own endpoint** (audited to the UI actor).

## The run: approval to gate 2, untouched

From the approval, with no further human act of any kind:

| Step | Tokens (in+out) |
|---|---|
| `write-spec` → `docs/greet/time/spec.md`, submitted to review | 4,852 |
| `review-spec` — approved, checked against the design under the FR-8 fidelity bar | 2,902 |
| `write-dev-plan` → `docs/greet/time/dev_plan.md` | 5,484 |
| `review-dev_plan` — approved | 2,738 |
| Decomposition | (engine, no model) |

**Total: 15,976 tokens** (~$0.006 at DeepSeek prices) from approval to stop.
The feature ended `ready` with three tasks — scaffold the package, implement
`Now`, write the acceptance tests — the first already dispatchable, and the
chain **stopped at gate 2**, which is the claim: it waits for a person to
start work, and nothing in it can press that button.

The two pre-approval review rounds cost a further 4,117 tokens.

## What this proves, and what it doesn't

Proven live: G0, both authoring invariants, the FR-4.7 sweep, `submit_comments`
confinement, human-only design approval, `submit_document` filing at the
conventional path, the FR-8-barred spec review, decomposition, and the stop at
gate 2 — all against a real provider, none against mocks.

Not yet proven live: the revision cascade (DoD 3). The natural next run:
revise the approved design, re-approve, and watch the single affected spec be
invalidated without a checkpoint (FR-9.1) and re-authored — then, if wanted,
spec a second feature first to see the multi-spec checkpoint form (FR-9.2).

Gaps recorded for the backlog: Detach, and the document-page Submit (already
Stage 2 scope).
