# DESIGN-003: Document Lifecycle and Gates

**Status:** Approved — 2026-07-20 ([REVIEW-001](../reviews/REVIEW-001-phase-1-package.md)). *Dated notes, 2026-09-28:* who may `withdraw` (§2), from SPEC-016; and how a
decision is superseded (§2) and where its file stays (§5), from
[SPEC-018](../specs/SPEC-018-decisions.md).
**Date:** 2026-07-02
**Parent:** [vision-v1](../vision/vision-v1.md) §3, §4, §6, §9
**Depends on:** DESIGN-001 (schema), DESIGN-002 (orchestrator)

## 1. Purpose

Defines the contract semantics at the heart of Cromwell: the document lifecycle,
validation vs review, the feature lifecycle, gates as computed expressions, and
the two flows the vision left undefined — **revision of an approved document
mid-flight** and **close-out verification**. There is no stage-bindings layer;
this document *is* the gate system's specification (vision §9).

## 2. Document lifecycle (canonical)

Vision §3/§6 declare `draft → reviewing → approved → superseded`; vision §8's
event list says `draft → submitted` and `submitted → reviewed`. Resolution:
**states are the four canonical ones; the words "submit" and "review" name
events, not states.**

```
             submit                    approve
  ┌───────┐ ───────▶ ┌───────────┐ ───────────▶ ┌──────────┐   supersede   ┌────────────┐
  │ draft │          │ reviewing │              │ approved │ ────────────▶ │ superseded │
  └───────┘ ◀─────── └───────────┘              └──────────┘               └────────────┘
             request_changes
                       │ escalate
                       ▼
                 (checkpoint; state stays 'reviewing' until human responds)
```

| Event | Transition | Actor | Side effects |
|---|---|---|---|
| `submit` | draft → reviewing | author (human via CLI/API, or agent) | validation runs synchronously first (§3); on pass, reviewer dispatch queued |
| `approve` | reviewing → approved | agent-reviewer outcome, or human checkpoint response | downstream gates re-evaluate; if this doc supersedes another, predecessor → superseded in the same transaction |
| `request_changes` | reviewing → draft | agent-reviewer outcome, or human | comments inserted; author notified |
| `escalate` | (none) | agent-reviewer outcome | checkpoint created with reviewer reasoning; human response maps to approve / request_changes |
| `supersede` | approved → superseded | lifecycle engine only | fired only by a successor's approval — never directly by a user |
| `withdraw` | reviewing → draft | author | cancels pending review dispatch |

> **Note, 2026-09-28 (Sam, accepted with [SPEC-016](../specs/SPEC-016-edit-in-the-browser.md)
> SD-9, choice 7).** A person who saves an edit to a reviewing document in the
> browser editor withdraws it, whoever wrote it. The queued review is
> cancelled, any hold cleared, and the document submitted again when ready.

> **Note, 2026-09-28 (Sam, accepted with [SPEC-018](../specs/SPEC-018-decisions.md)
> SD-6).** A decision is also superseded by the engine when a *different*
> decision that names it in `supersedes:` is accepted, as a plan is superseded
> with its spec (SPEC-011 FR-6.6). A person never fires `supersede` directly.

Terminal states: `superseded`. `approved` documents are immutable — the git
watcher flags any file change to an approved document's path as an **integrity
violation** (checkpoint, kind `document-integrity`), because the honest path to
changing an approved document is revision (§5).

Every transition writes an `audit_events` row in the same transaction
(DESIGN-002 O-3). Illegal transitions are rejected by the lifecycle engine with
a typed error — there is no force flag; overrides happen only through
checkpoints, answered by humans, and are themselves audited.

## 3. Validation (mechanical, synchronous)

Runs at `submit`; failure blocks the transition and returns a report. No
judgement, no model, no dispatch (vision §6).

Checks, per document type, driven by a `validation:` manifest in the type's
template directory (`.cromwell/templates/<type>/`):

1. Front matter present and well-formed (type, title, owner ref).
2. Required sections present (by heading), in order where the manifest says so.
3. Internal links resolve (relative paths exist in the repo; entity refs exist).
4. No unresolved template placeholders (`TODO`, `{{...}}`).
5. Type-specific rules — e.g. a spec's acceptance criteria section must contain
   at least one criterion; a dev-plan's task table must parse.

Validation also runs on demand (`cromwell validate <path>`) so authors can check
before submitting. Validation results are recorded on the document row's audit
trail; reviewers see the latest report (vision §6: review may cite validation).

## 4. Review (semantic, asynchronous)

On `submit` passing validation, the orchestrator dispatches the reviewer role
configured for the document type (`.cromwell/roles/`, e.g. `spec-reviewer`).
The reviewer receives: the document, its validation report, the owning entity's
context, ancestor documents, and unresolved comments from prior rounds. Its
outcome tool is `submit_review(verdict, comments[], reasoning)` (DESIGN-002 §4).

Re-review after `request_changes` is a fresh dispatch with the comment thread
included, so the reviewer can verify its comments were addressed rather than
re-reviewing cold.

**Escalation-rate telemetry:** the ratio of `escalate` to total verdicts per
document type is computed from the dispatch ledger and surfaced in status
output — the vision's health signal for review-prompt quality (§9).

## 5. Revision of an approved document (resolves vision gap)

Approved documents are never edited. To change one:

1. `cromwell revise <doc>` creates a **successor**: a new document row
   (`draft`, `supersedes_id` → predecessor), and a working copy of the file
   (same path convention with a version suffix until approval).
2. The successor goes through the normal lifecycle: validate → review →
   approve. On approval, predecessor → `superseded` atomically, and the
   lifecycle engine performs the repo file operations itself — successor
   moved to the canonical path, predecessor's file archived under
   `docs/_superseded/` — in a single server-authored commit, which the git
   watcher re-indexes like any other. The states and the files change
   together; no human file shuffling.

   > **Note, 2026-09-28 (Sam, accepted with
   > [SPEC-018](../specs/SPEC-018-decisions.md) SD-6).** A decision superseded
   > by another decision keeps its file where it is, as a record. Only an
   > earlier revision of the same decision is archived.

3. **Impact on in-flight work:** if the revised document is a spec or dev-plan
   of a feature in `active` or `review`, the successor's *submission* (not
   approval) immediately:
   - flags the feature `spec_stale = true` (audit-logged),
   - blocks *new* task dispatches for that feature (running dispatches finish),
   - raises a checkpoint (kind `revision-in-flight`) asking the human:
     **pause** the feature until the revision lands, or **continue** under the
     old contract (revision applies to later work).
4. On the successor's approval, task decomposition is re-validated against the
   new dev-plan: unstarted tasks may be replaced; started/done tasks are never
   silently deleted — mismatches surface in the checkpoint for human decision.

The contract is therefore always honest: work is never running against a spec
that has silently changed underneath it, and the change itself goes through
the same review gate as the original.

## 6. Feature lifecycle

```
          plan approved                 all tasks done              verified
┌──────┐ ───────────────▶ ┌───────┐ ─▶ ┌────────┐ ──▶ ┌────────┐ ─────────▶ ┌──────┐
│ idea │    (gate G1)     │ ready │    │ active │     │ review │  (gate G3) │ done │
└──────┘                  └───────┘    └────────┘     └────────┘            └──────┘
    │                        │  start      (gate G2)      │
    └────────────────────────┴────────────── abandon ─────┘──▶ abandoned
```

| State | Meaning |
|---|---|
| `idea` | Created under an initiative; spec/dev-plan being produced |
| `ready` | Contract approved (G1); awaiting start |
| `active` | Worktree + branch created; tasks being dispatched |
| `review` | All tasks done (G2); feature-level verification running |
| `done` | Verified (G3) and merged |
| `abandoned` | Explicitly closed without shipping (always human, always with reason) |

`idea → ready` fires automatically when G1 passes. `ready → active` is a human
"start" action (or auto-start if project config opts in) — starting work is a
resource commitment, so it defaults to deliberate.

## 7. Close-out verification (resolves vision gap)

The vision's flow ended at "all tasks approved → done → merge", dropping
kanbanzai's Definition-of-Done verification. Cromwell keeps verification, as
the `review → done` gate:

1. When the last task completes, the feature enters `review`.
2. The orchestrator dispatches a **verifier** role — clean context, read-only
   tools — with the spec's acceptance criteria as its checklist. Outcome tool:
   `submit_verification(criteria: [{id, met, evidence}], verdict)` with the
   standard three verdicts.
3. `approve` → merge the feature branch, transition to `done`, tear down the
   worktree (heartbeat GC). `request_changes` → unmet criteria become new tasks
   (feature returns to `active`). `escalate` → checkpoint.

Merging happens *after* verification approval and *before* `done` — a feature
is done only when its branch is in main and its acceptance criteria have been
checked by an agent that didn't write the code.

## 8. Gate catalogue

Gates are named, pure Go functions over current state — no YAML, no config
layer (vision §9). Each evaluation writes an `audit_events` row
(`gate.evaluated`, pass/fail, reason).

| Gate | Guards | Expression |
|---|---|---|
| **G1** contract-approved | feature `idea → ready` | current spec `approved` AND current dev-plan `approved` (current = non-superseded doc owned by the feature) |
| **G2** tasks-complete | feature `active → review` | every task of the feature is `done` or `abandoned`, at least one `done`, AND NOT `spec_stale` |
| **G3** verified | feature `review → done` | latest verification dispatch outcome = approve AND branch merged |
| **G4** milestone-lockable | milestone `open → locked` | at least one resolved member `done` |
| **G5** initiative-archivable | initiative archive | no non-terminal features in subtree (override via checkpoint with reason) |
| **G6** dispatchable | any dispatch `queued → running` | governor checks: budget, rate, serialisation (DESIGN-002 §6) |

New gates are added by adding a function and a row here; the catalogue in this
document is the registry of record.

## 9. Defect lifecycle (sketch)

Defects follow a feature-shaped pipeline with triage states prepended
(`reported → triaged → accepted`), then reuse the feature machinery (`active →
review → done`) with a lighter contract: a defect needs an approved *note*
describing reproduction and intended fix, not a full spec/dev-plan. `rejected`
is the triage-out terminal state. Full design deferred per vision §14; the
schema (DESIGN-001 §4) already accommodates this shape.

## 10. Decisions recorded here

| # | Decision | Rationale |
|---|---|---|
| L-1 | Four document states; submit/review are events | Fixes vision §8 vocabulary drift |
| L-2 | Approved documents immutable; change = supersession | Honest audit trail; enables the revision flow |
| L-3 | Revision mid-flight blocks new dispatches at *submission*, human decides pause/continue | Work never runs against a silently changing contract |
| L-4 | Close-out verification retained as gate G3 with a clean-context verifier | Kanbanzai lesson worth keeping; the vision's omission was a gap, not a decision |
| L-5 | `ready → active` is human-triggered by default | Starting work spends money; deliberate by default, config can automate |
| L-6 | No force flags anywhere; overrides only via answered checkpoints | The kanbanzai override failure mode is structurally excluded |
