# REVIEW-009: Consistency Review of the Authoring-Chain Package

**Status:** Complete — **awaiting Sam's approval**, which is recorded in §5.
**Date:** 2026-07-30
**Reviewer:** Claude (Opus 5), authoring review — I wrote both documents under
review, so this is not and cannot be the approval gate.
**Scope:** [SPEC-009](../specs/SPEC-009-the-authoring-chain.md) against
[DESIGN-009](../design/DESIGN-009-the-authoring-half.md),
[DEC-005](../decisions/DEC-005-the-orchestration-boundary.md),
[DEC-003](../decisions/DEC-003-cli-scope.md), and the phase 1–3 code as it
actually is.

---

## 1. What this review is

An authoring consistency pass, following REVIEW-002/003/004/006. I wrote
DESIGN-009 and SPEC-009, so I cannot also be their approval gate — that is the
conflict of interest the whole system exists to prevent. This pass checks that
the spec hangs together with the design it builds on, with the decisions it
depends on, and with the code as it stands, and fixes what it can. **The
approval decision is Sam's.**

Seven findings, four of them material. All are fixed in the spec; this records
what was wrong and why, so the reasoning is not lost. (The seventh, R9-7, came
out of Sam's answer to §5 rather than the pass itself, and is recorded there.)

## 2. Material findings (fixed)

### R9-1 — The chain had no dev-plan trigger, and would have stalled

**The most serious finding.** DESIGN-009 §2's diagram shows
`write spec → review spec → write dev-plan`, and §6's table lists a
`write-dev-plan` purpose and a `dev-plan-author` role. But **no rule anywhere
said what causes a dev-plan to be written.** §4.3's invariant governs specs only.

Traced against the code, the consequence is concrete. On a spec reaching
`approved`, `decideDocumentTransition` fires `EvaluateContractGate` and nothing
else. G1 requires spec **and** dev-plan approved, so it fails. The feature stays
in `idea`, no dev-plan is ever authored, and the chain stops one step after the
gate that was supposed to release it.

The load-bearing claim in SPEC-009 §1 — an approved design produces specs, plans
and tasks unattended — would have been false on the first run.

**Fixed** by restating FR-4 as **two invariants**: the spec invariant from the
design, and a dev-plan invariant ("every feature with an approved spec has a
current dev-plan"). Stating both as invariants rather than steps also makes the
handover explicit: an approved dev-plan already fires `DecomposeDevPlan` and
`EvaluateContractGate`, so the chain terminates in machinery that exists.

**Why the design missed it:** §4.3 was written to answer "what does approving a
design release?", and the dev-plan sits one step beyond that question. The
diagram carried the intent; no requirement carried the rule.

### R9-2 — Two of the invariant's triggers have no event to fire on

FR-4 listed "a feature being created" and "a feature gaining a description" as
triggers. **Neither event exists.** The bus carries document transitions,
dispatch outcomes, checkpoints, `FeatureStarted` and a heartbeat — there is no
event for an entity being created or edited. `notifyEntityChanged`, which the MCP
authoring tools call, is a **presentation-only** signal to the SSE hub (SPEC-008)
that the rules engine never sees.

Without new events, two of the four triggers are silently inert, and a feature
described after its design was approved would never be specced.

**Fixed** by naming `FeatureCreated` and `FeatureDescribed` as new domain events
published in the same transaction as the change (FR-4.4).

**A mitigating discovery worth recording:** `bus.Tick` exists, so the heartbeat
can reconcile both invariants. That makes the new events a *latency* improvement
rather than a correctness requirement — the chain would eventually self-heal.
FR-4.7 now says so explicitly, which also strengthens the invariant framing: it
is reconcilable in a way a rule list would not be.

### R9-3 — Nothing said how a design reviewer returns its work

FR-2.2 said the design reviewer "records comments and cannot approve or request
changes as a verdict" — a statement of intent with no mechanism. Every other
dispatch purpose ends in a named outcome tool whose absence fails the dispatch.
An implementer would have had to invent one, and the obvious shortcut — reuse
`submit_review` and ignore its verdict — is the worst option available: it leaves
a tool whose advertised contract the system silently discards.

**Fixed** by specifying a distinct **`submit_comments`** outcome tool with no
verdict field. This follows the confinement-by-omission principle the MCP facet
already uses: a reviewer with no way to express a verdict cannot be given
authority by accident later.

## 3. Minor findings (fixed)

- **R9-4 — a premature open question.** The draft asked where a design's
  conventional path lies for `submit_document`. Designs are human-written and
  attached; they never pass through `submit_document` in Stage 1, which serves
  only `write-spec` and `write-dev-plan`. Removed, with a note recording when it
  would return.
- **R9-5 — G0's store access was unstated.** G1–G5 are pure functions over
  booleans with no store access. G0 needs "the primary document of type X for
  owner Y", which does not exist as a read. Added as FR-3.4, additive alongside
  `CurrentDocForOwner`, keeping the gate function itself pure.
- **R9-6 — `identityWithSkill` was not named.** FR-10.4 described a prompt
  ordering without saying which function changes to produce it. Named, and tied
  to the audit's finding that caching and attention want the same order.

## 4. Checked and found consistent

Recorded so a later reader knows these were examined, not overlooked.

- **DEC-005 boundary.** Nothing in the spec lets an agent compose a dispatch,
  override a gate, or start a feature. The authoring dispatches are orchestrator-
  composed like every other.
- **The seam (vision §2).** Spec authoring sits on the planning side, where the
  vision puts "humans with AI help". No dispatch decides what to dispatch next.
- **DEC-003.** SPEC-009 covers submission for agent-authored documents; the
  human-facing `doc submit` / `doc revise` remain Stage 2, and `doc validate`
  is recorded as deliberately uncovered. The checklist can close with all three
  named.
- **No worktree for documents.** Verified against `readDocFile`, which resolves
  against `RepoRoot` and rejects escapes; NFR-2 asserts the property in a test
  because it is the one most likely to erode.
- **Sequential penalty.** NFR-3 keeps one agent per document, fan-out across
  features only, bounded by the existing worker pool (default 4, the researched
  saturation point).
- **Idempotency and audit-in-transaction.** Consistent with the existing
  conventions; no new force path is introduced.

## 5. The observation, and Sam's answer (2026-07-30)

**Raised:** approving a revised design produces two human interactions in a row —
the approval, then the `design-revision` checkpoint asking which specs it
invalidates. Tempting to fold the second into the first.

**Sam's ruling: keep them separate, *unless* only one spec is affected.** With
several specs a human or their chat agent may need to read them before answering,
so the checkpoint belongs in the inbox where it can wait. With a single spec
there is no triage — the design changed and there is one candidate — so the spec
is invalidated directly and no checkpoint is raised. FR-9.1/9.2 now say so.

### R9-7 — a seventh finding, surfaced by applying that rule consistently

Sam's principle is *where there is no ambiguity, do not ask*. Applying it to the
rest of the cascade exposed a gap of the same shape as R9-1: **the cascade
stopped at the spec.**

Invalidate a spec, and a replacement is written and approved — but the feature
already has a current dev-plan, so FR-4.2's dev-plan invariant is satisfied and
no new plan is authored. The feature would carry a dev-plan decomposed from a
*superseded* spec, and nothing would flag it.

**Fixed** as FR-9.4a: invalidating a spec invalidates its dev-plan. This needs no
checkpoint, because unlike design→spec it is 1:1 and mechanical — a dev-plan is a
decomposition of exactly one spec. The task layer below needs nothing new, since
`ReDecomposeDevPlan` already reconciles tasks non-destructively on successor
dev-plan approval.

The complete cascade is now **design → spec** (judgement; checkpoint only when
more than one spec is affected) **→ dev-plan** (mechanical) **→ tasks** (existing).

**Why both R9-1 and R9-7 were missed the same way:** DESIGN-009 was written
forwards, from the gate outward, and each specified what the *next* step
produces. Neither the forward chain past the spec nor the backward invalidation
past the spec had an owner. Worth remembering when the Stage 2 package is
written.

## 6. What this review did not do

- It did not check the spec against the *kanbanzai* skills it proposes adapting.
  FR-5.2 says the adaptation must be selective; whether any particular
  anti-pattern survives the move is an implementation judgement.
- It did not size the work. The `design-revision` checkpoint needs a per-item
  inbox form, which is the only new UI in Stage 1 and the least predictable part.

## 7. Approval

**Not approved by this review.** SPEC-009 is a draft until Sam approves it, and
this reviewer authored it. Recommended for approval subject to Sam's view on the
observation in §5 and the SD-1 scoping question (whether audit C-1, the review
loop cap, folds into this spec's implementation or lands separately).
