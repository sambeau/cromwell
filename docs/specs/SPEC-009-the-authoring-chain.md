# SPEC-009: The Authoring Chain — Stage 1

**Status:** **Draft — not approved, not binding.** Authored by Claude (Opus 5).
An authoring consistency review follows; the approval decision is Sam's.
**Date:** 2026-07-30
**Parent design:** [DESIGN-009](../design/DESIGN-009-the-authoring-half.md)
(the authoring half: G0 as a computed spec-ready gate, the two human gates, the
authoring roles, the revision cascade)
**Also depends on:** [DEC-005](../decisions/DEC-005-the-orchestration-boundary.md)
(accepted — the boundary is bypassing the orchestrator, not causing agents to
run), [DEC-003](../decisions/DEC-003-cli-scope.md) (the CLI shrinks; this spec
supplies part of the capability that must land first),
[DESIGN-003](../design/DESIGN-003-document-lifecycle-and-gates.md) (the document
lifecycle and the rule that gates are expressions over current state),
[DESIGN-005](../design/DESIGN-005-dev-plans-tasks-decomposition.md)
(decomposition, which this chain terminates in)
**Vision:** [vision-v1](../vision/vision-v1.md) §2 (the planning/developing
seam), §5 (the spec is drafted by a human *or AI author*), §8 (the orchestrator
is code; implicit context comes from the orchestrator, not the agent), §9 (agent
reviewers, humans on escalation)
**Evidence:** [alignment review](../notes/vision-alignment-review-2026-07-29.md)
§5, §6a · [research conformance audit](../notes/research-conformance-audit-2026-07-29.md)
§4, §5

---

## A note on prose

Requirements are written for the person who will build this and the person who
will check it. Headings, labels, gate reasons and tool text that a human will
read are written in full sentences (DESIGN-008 D-6). Terse agent-to-agent writing
belongs in the skills this spec creates, not in this spec.

## 0. Framing

Cromwell can review a specification; nothing in it can write one. Every dispatch
purpose is a review, an implementation or an estimate, and the starter pack ships
six roles all of which review, implement, estimate or verify. Documents therefore
enter Cromwell by exactly one route: a human writes the file and submits it from
the CLI.

This spec builds the missing half, and with it the first of Sam's two human
gates. It is **Stage 1 of DESIGN-009 §11** — the chain itself. The surfaces that
make the chain reachable from more than one place (MCP approval and submission,
document-page Submit and Revise) are Stage 2 and are out of scope here.

## 1. Goal

**One load-bearing claim, and the whole spec exists to prove it:**

> A human approves a design document. Without any further human involvement,
> Cromwell writes a specification for each feature that design releases, reviews
> it, writes a dev-plan, decomposes it into tasks — and stops, waiting for a
> human to begin implementation.

Two supporting claims:

> A design that is later revised never leaves a stale specification standing
> silently.

> Nothing about planning structure — creating initiatives, features, or breaking
> a plan into sub-plans — requires the orchestrator. That work stays
> conversational and is already built.

## 2. Scope

### In scope

1. **The `design` document type** — template, manifest, mechanical validation.
2. **Per-type approval authority**, and the human approve action in the UI.
3. **G0**, the computed spec-ready gate, with one-level inheritance.
4. **The spec-authoring invariant** and its four triggers.
5. **`spec-author` and `dev-plan-author`** roles, and the `write-spec` and
   `write-dev-plan` skills and dispatch purposes.
6. **`submit_document`**, the authoring outcome tool, and the direct UI
   notification that goes with it.
7. **The strengthened `review-spec` bar** — faithful and complete translation.
8. **The revision cascade** and the `design-revision` checkpoint.
9. **The role schema extension** — `vocabulary` and `anti_patterns` — to the
   extent the new roles need it.

### Out of scope (deferred, with destination)

| Deferred | Destination |
|---|---|
| MCP tools for approving and submitting documents | **Stage 2** (DESIGN-009 §11), and the DEC-005 amendment in §5.1 must land with them |
| Document-page Submit and Revise actions | **Stage 2** |
| A "ready to spec" affordance on the initiative page | **Stage B** of the workflow surface (DESIGN-009 §14) |
| Retrofitting `vocabulary` and `anti_patterns` onto the six existing roles | **The prompt-quality slice** (audit C-3, C-4). This spec extends the schema and uses it for the two new roles only. |
| A cap on the review→revise loop | **Audit C-1.** See SD-1 — this spec *assumes* it lands. |
| `doc validate` as a standalone affordance | **Nowhere.** DESIGN-009 SD-3: validation runs on submit and a standalone check has no consumer. Recorded so DEC-003 can close with it named. |
| A `design-author` role | **Nowhere.** DESIGN-009 SD-1: designs are co-authored conversationally. |
| A project-conventions document surfaced into prompts | **Audit C-6**, a separate decision. It would change what authors receive but does not block this chain. |

### Scope decisions

- **SD-1 — audit C-1 is a precondition of this spec, built separately and
  first.** The review→revise loop is currently uncapped: `returnTaskCode`
  re-dispatches unconditionally, and the only backstop is a project-wide budget
  cap that halts everything when it trips. Gate 1 lengthens the unattended
  stretch considerably, so a runaway becomes proportionally more expensive.

  C-1's shape is settled (audit §3.3a, Sam 2026-07-30): **severity-gated
  termination** — findings carry a severity, the loop repeats while *major*
  findings exist, and stops when only minors remain — with a **hard round cap**
  as a backstop for genuine disagreement, and leftover minors recorded rather
  than discarded.

  **It is not built here.** It affects code review today, independently of the
  authoring chain, and is worth having before this spec lands rather than
  because of it. It also touches the review outcome schema, which this spec
  otherwise leaves alone. Treat "C-1 has landed" as a precondition, not an
  assumption this spec carries.

  Note the same loop governs `review-spec`, so C-1's severity gate applies to
  document review too — and under FR-8 a missing design decision is definitionally
  a *major* finding.
- **SD-2 — the design reviewer posts comments and never a verdict.** It is a
  reviewer dispatch like any other, but its outcome cannot approve. This is the
  smallest change that makes design approval a human act without abusing
  escalation.
- **SD-3 — one approval route in Stage 1: the UI.** DESIGN-009 §5.1 specifies
  two, but the MCP route needs a DEC-005 amendment and is audited to
  `chat-agent` rather than to a person until per-user identity lands. The UI
  route needs neither and will still be correct afterwards.
- **SD-4 — `write-dev-plan` produces the dev-plan only.** Decomposition into
  tasks is already built and already fires on dev-plan approval
  (`DecomposeDevPlan`); nothing new is needed and nothing new is specified.

## 3. Requirements

### FR-1: The `design` document type

**FR-1.1** A `design` template ships in the starter pack with a manifest, in the
same shape as the existing `spec` and `dev_plan` templates.

**FR-1.2** Mechanical validation for a design checks structure only — required
sections present, links resolvable — never quality. Validation and review stay
separate, as they already are for specs.

**Acceptance:** a design document can be created, validated, submitted, reviewed
and approved end to end. A design missing a required section fails validation
with a message naming the section.

### FR-2: Per-type approval authority

**FR-2.1** Each document type declares who may approve it. `design` → **human**.
`spec` and `dev_plan` → **agent**, unchanged from today.

**FR-2.2** Submitting a design moves it to `reviewing` and dispatches a design
reviewer as normal. The reviewer completes with a **`submit_comments`** outcome
tool — comments and reasoning, and **no verdict field**. The document remains in
`reviewing` regardless of what the reviewer found; only a human moves it (SD-2).

Naming a distinct outcome tool matters: reusing `submit_review` and discarding
its verdict would leave a tool whose stated contract the system silently ignores,
which is exactly the kind of drift the confinement-by-omission principle avoids.
A design reviewer that has no way to express a verdict cannot accidentally be
given authority by a later change.

**FR-2.3** A human approves or requests changes on the document's page in the UI.
Both call gated, audited service methods, audit-in-transaction, with no force
path.

**FR-2.4** An agent-approvable document is unaffected: `spec` and `dev_plan`
continue to be approved by their reviewer, escalating to a human when the
reviewer is unsure.

**Acceptance:** a submitted design is reviewed by an agent whose comments appear
on the page, and remains in `reviewing` until a human acts. A test asserts no
code path allows an agent verdict to approve a `design`.

### FR-3: G0, the spec-ready gate

**FR-3.1** G0 is a pure function over current state, in `internal/lifecycle`,
unit-tested without Postgres or a provider:

> A feature is spec-ready when its own primary design document is `approved`, or
> its **immediate parent** initiative's primary design document is `approved`.

**FR-3.2** Inheritance stops at one level. A design approved two or more levels
above does not admit the feature.

**FR-3.3** G0 returns a plain-words reason when it does not pass, in the same
shape as G1–G5, so any surface can explain the state to a person.

**FR-3.4** G0's inputs come from one additive store read: the primary document of
a given type for an owner, alongside the existing `CurrentDocForOwner`. The gate
function itself takes booleans and stays free of store access, as G1–G5 do.

**Acceptance:** table-driven tests cover own-design-approved, parent-approved,
grandparent-approved (must not pass), no design, and design in `draft` or
`reviewing`.

### FR-4: The two authoring invariants

The chain is expressed as **two** invariants the orchestrator maintains, not as a
sequence of steps. DESIGN-009 §4.3 states the first; the second is implied by its
§2 diagram and §6 table but was never written down, and without it the chain
stalls after a spec is approved.

**FR-4.1 — the spec invariant.**

> Every feature that G0 admits, and that has a non-empty description, has a
> current spec.

**FR-4.2 — the dev-plan invariant.**

> Every feature with an `approved` spec has a current dev-plan.

Together these carry a feature from design approval to decomposition: an approved
design produces a spec; an approved spec produces a dev-plan; an approved
dev-plan already fires `DecomposeDevPlan` and `EvaluateContractGate`, which is
built (DESIGN-005) and needs nothing here. G1 then admits the feature to `ready`,
where it waits for gate 2.

**FR-4.3 — triggers.** Evaluation is event-driven:

| Invariant | Triggered by |
|---|---|
| Spec | a design reaching `approved`; a feature being created; a feature gaining a description; a feature's spec being superseded |
| Dev-plan | a spec reaching `approved`; a feature's dev-plan being superseded |

**FR-4.4 — two of these events do not exist and must be added.** The bus carries
document transitions, dispatch outcomes, checkpoints and a heartbeat, but
**nothing for an entity being created or edited**. `notifyEntityChanged` is a
presentation-only signal to the SSE hub (SPEC-008) and is not consumed by the
rules engine. `FeatureCreated` and `FeatureDescribed` are therefore new domain
events, published in the same transaction as the change that causes them.

**FR-4.5** A feature with an empty description is **not dispatched, not
checkpointed, and not reported as a problem.** A placeholder feature is a normal
state in width-first planning.

**FR-4.6** Idempotency: a feature whose spec (or dev-plan) is currently `draft`,
`reviewing` or `approved` is skipped. Dispatches carry an idempotency key so a
repeated event cannot double-dispatch.

**FR-4.7** Because these are invariants rather than a sequence of rules, the
existing heartbeat (`bus.Tick`) reconciles them: a feature that should have a
spec or dev-plan and does not eventually gets one even if an event was lost. This
is the safety net vision §8 describes, and it is why FR-4.4's new events are a
latency improvement rather than a correctness requirement.

**Acceptance:** approving a design over a tree containing described features,
undescribed features, and a sub-initiative with its own unapproved design
dispatches `write-spec` for exactly the described features at the admitted level,
raises no checkpoints, and is unchanged when the same event is replayed. Each
resulting spec, once approved, produces exactly one dev-plan.

### FR-5: The authoring roles and skills

**FR-5.1** Two roles ship in the starter pack: `spec-author` and
`dev-plan-author`, in the restructured shape (FR-10) — a job-title identity, a
vocabulary payload, and named anti-patterns with detect/because/resolve.

**FR-5.2** Two skills ship: `write-spec` and `write-dev-plan`. Both are adapted
from `~/Dev/kanbanzai/.kbz/skills/`, **selectively** — content that assumes an AI
orchestrator that can lose context does not apply here and must not be carried
over.

**FR-5.3** Two dispatch purposes: `write-spec` and `write-dev-plan`. Both are
read-only with respect to the worktree and carry a **nil `ToolCtx`** (NFR-2).

**FR-5.4** A `write-spec` prompt contains the approved design, the ancestor
design documents, and the feature's name and description. A `write-dev-plan`
prompt contains the approved spec and the design.

**Acceptance:** both roles load and validate; a role declaring a mutating tool
for an authoring purpose is a config error, as it already is for review purposes.

### FR-6: `submit_document`, the authoring outcome tool

**FR-6.1** An authoring dispatch ends by calling `submit_document` with the
document body. It is the dispatch's outcome tool; failing to call it fails the
dispatch, as with every other purpose.

**FR-6.2** The server, in one transaction, writes the file to the type's
conventional path in the **main repository**, registers or updates the document
row, validates it, and transitions it to `reviewing`.

**FR-6.3** The agent never chooses a path, never touches git, and cannot
half-create a document. Path and owner come from the orchestrator (vision §8).

**FR-6.4** A validation failure is returned to the agent as an unsuccessful tool
result in plain language so it can correct and resubmit within its turn cap —
the convention the MCP facet already uses.

**Acceptance:** a `write-spec` dispatch produces a registered, validated spec in
`reviewing` at the conventional path, owned by the right feature. An invalid body
is rejected with a readable message and does not create a document row.

### FR-7: Freshness — the server notifies because it knows it wrote

**FR-7.1** `submit_document` emits the document-changed notification directly.

**FR-7.2** It does not rely on the git post-commit hook. The hook observes
*commits*; an agent write is uncommitted at the moment it happens, so the hook
would never see it. (`internal/gitwatch/` is an empty directory; the watcher
named in the vision's architecture is served by the hook.)

**FR-7.3** The document is committed for durability and history, following the
pattern `takeOverCanonicalPath` already uses, with `cromwell` as the author. The
commit is not the detection mechanism.

**Acceptance:** an open document page updates when an agent writes the document,
with no commit having been made by a human and no hook invocation.

### FR-8: The spec reviewer verifies faithful and complete translation

**FR-8.1** `review-spec` gains a coverage criterion: every material decision in
the design appears in the spec, or is explicitly and reasonably declared out of
scope.

**FR-8.2** A verdict of `approve` is not available while a design decision is
unaccounted for.

**FR-8.3** Fidelity doubt — "this spec does not represent the design and I cannot
tell whether that was deliberate" — is named in the skill as an explicit
escalation ground.

**FR-8.4** The existing separation holds: mechanical validation runs server-side
and its report is handed *to* the reviewer, which cites failures and stops rather
than performing the checks itself.

**Rationale, and why this requirement is load-bearing.** With the human gate
before the spec, the spec reviewer is the only thing between an approved design
and an implementation built from a lossy translation of it. Its current bar is
non-contradiction, which a spec omitting half its design would pass.

**Acceptance:** a spec that contradicts nothing but silently omits a design
decision is not approved.

### FR-9: The revision cascade

**FR-9.1 — one affected spec: no checkpoint.** When a **successor** `design` is
approved and exactly **one** current spec falls in its G0 scope, that spec is
invalidated automatically. There is no triage to do: the design changed, and
there is only one candidate, so asking a human "which of these one specs does
this invalidate?" is a question with no information in it (Sam, 2026-07-30).

The cost of this rule is accepted deliberately: a design revision that only
clarifies wording will still force a spec revision pass. That is the cheaper
error, because the alternative is a spec silently derived from superseded
reasoning.

**FR-9.2 — two or more affected specs: one checkpoint.** Which specs a revision
invalidates is then a judgement, and a human or their chat agent may need to read
the specs before answering. Cromwell raises exactly **one** checkpoint of a new
kind, `design-revision`, listing every affected spec with its feature.

**FR-9.2a** The answer is **per spec**: keep or invalidate. This is a new
checkpoint shape — every existing kind answers with one verb from a fixed set —
so it needs a purpose-built form in the inbox. The checkpoint's `Context` field
is already free-form JSON and needs no change.

**FR-9.2b** The count that decides between FR-9.1 and FR-9.2 is the number of
affected **specs**, not designs. A single spec informed by several design
documents still takes the no-checkpoint path.

**FR-9.3** The checkpoint blocks. Until it is answered, the affected features
have a design and a spec that disagree, and no downstream work proceeds on them.

**FR-9.4** **Invalidate** supersedes the spec. That leaves the feature without a
current spec, so FR-4's invariant dispatches a fresh `write-spec` against the
revised design.

**FR-9.4a — invalidating a spec invalidates its dev-plan.** A dev-plan is a
decomposition of one spec; when that spec is superseded the dev-plan is stale by
definition. Unlike design→spec, this needs no judgement and raises no checkpoint —
it is 1:1 and mechanical, so the same principle that removes the checkpoint in
FR-9.1 removes it here.

Without this the cascade stops one step short: a replacement spec would be
written and approved, FR-4.2's dev-plan invariant would already be satisfied by
the *old* dev-plan, and the feature would carry a plan derived from superseded
reasoning. The task layer beneath needs nothing new — `ReDecomposeDevPlan`
already reconciles tasks non-destructively when a successor dev-plan is approved
(DESIGN-005 §4).

So the full cascade is: **design → spec** (judgement, checkpoint only when more
than one spec is affected) **→ dev-plan** (mechanical) **→ tasks** (existing
re-decomposition).

**FR-9.5** **Keep** is recorded against the checkpoint answer, so the audit trail
shows the revision was considered and deliberately did not invalidate that spec.

**FR-9.6** Where an invalidated spec belongs to a feature that is `active` or in
`review`, the replacement is a successor for an in-flight feature and the
existing `MarkRevisionInFlight` path applies unchanged. Two checkpoints in
sequence is correct — they are two different decisions.

**Acceptance:** revising and re-approving a design over three specced features
raises one checkpoint listing three specs. Answering invalidate on two supersedes
exactly those two **and their dev-plans**, and produces two new `write-spec`
dispatches; the third is untouched and its "keep" appears in the audit trail. The
same revision over a *single* specced feature raises **no** checkpoint and
invalidates that spec and its dev-plan directly.

### FR-10: The role schema extension

**FR-10.1** `Role` gains `vocabulary []string` and `anti_patterns`, each
anti-pattern carrying `name`, `detect`, `because` and `resolve`.

**FR-10.2** `identity` is a job title for the new roles, with reasoning carried
in `because` clauses (Sam's ruling, 2026-07-29).

**FR-10.3** Both fields are optional, so the six existing roles keep loading
unchanged. Retrofitting them is out of scope here.

**FR-10.4** The assembled system prompt places identity, then vocabulary, then
anti-patterns, then the skill procedure. `identityWithSkill` grows accordingly.
This is compatible with the existing ordering, which is stable-prefix-first for
provider caching, and with the attention findings — both want constraints early
and the artefact under work last (audit §2.7).

**Acceptance:** a role with the new fields loads and its prompt contains them in
the specified order; a role without them loads unchanged; an unknown field is
still a config error.

## 4. Non-functional requirements

- **NFR-1 — no new authority beyond the named human approval.** Every other
  mutation is an existing gated, audited service method. FR-2.3 adds one, and it
  is audited in the same transaction as its state change (O-3), with no force
  path.
- **NFR-2 — authoring dispatches carry no worktree.** Documents live in the main
  repository; worktrees exist for a feature's code. Authoring dispatches carry a
  nil `ToolCtx`, as document reviews and estimates already do. **A test asserts
  this**, because it is the property most likely to be eroded by a later change.
- **NFR-3 — one agent per document, in one pass.** Writing a spec or a dev-plan
  is sequential reasoning; parallelising it measured 39–70% worse. Fan-out is
  **across** features only, bounded by the existing worker pool.
- **NFR-4 — the orchestrator stays code.** No dispatch decides what to dispatch
  next; the rules engine does.
- **NFR-5 — human-facing prose** in the checkpoint question, the approval
  actions, and every gate reason (DESIGN-008 D-6).
- **NFR-6 — inherited invariants hold.** No currency on any rendered page; no
  entity addressed by a typed path. Both are enforced by existing tests that must
  keep passing.
- **NFR-7 — `go vet ./...` and `go test -race ./...` clean.** Pure logic
  (G0, the invariant's predicate) unit-tested without Postgres or a provider;
  the chain integration-tested with the mock provider against real Postgres.

## 5. Definition of Done

1. Every FR acceptance criterion passes in CI with the mock provider, on plain
   Postgres and the Supabase local stack.
2. **A human-confirmed live smoke of the load-bearing claim:** against a real
   project and a real provider, an operator approves a design document and —
   without touching the CLI or intervening again — watches specifications be
   written and reviewed, dev-plans written, tasks decomposed, and the chain stop
   at gate 2. Recorded in a walkthrough, with the token cost of the run.
3. **A second live smoke of the revision cascade:** revising the design raises
   one checkpoint; invalidating a spec produces a replacement written against the
   revised design.
4. A test asserts authoring dispatches carry a nil `ToolCtx` (NFR-2), and a test
   asserts no path lets an agent verdict approve a `design` (FR-2).
5. `go vet ./...` and `go test -race ./...` clean.
6. Entry criteria for **Stage 2** drafted, and the DEC-003 removal checklist
   updated with what this spec covers and what it does not.

## 6. Open questions carried into implementation

These are mechanics, not design questions. Each has a named owner in the design.

1. **Superseding a spec before its replacement exists** (DESIGN-009 §9.3).
   `takeOverCanonicalPath` supersedes on *successor approval* — the reverse
   order. Invalidation supersedes first and authors afterwards.
2. **A spec already in `draft` or `reviewing`** when its design is revised: does
   it join the FR-9.1 checkpoint list, or is it simply cancelled? The
   implementation should pick one and record which.
3. **Whether `write-dev-plan` should read the task-shaped parts of the spec
   differently from the prose parts.** DESIGN-005 defines the dev-plan's task
   table as the decomposition source, so the author is writing to a structure the
   engine will parse. The template should carry that weight, but how strictly the
   author is held to it is an implementation judgement.

*(A third question in the first draft asked where a design's conventional path
lies for `submit_document`. It was premature: designs are human-written and
attached, and never pass through `submit_document` in Stage 1, which only serves
`write-spec` and `write-dev-plan`. It returns if Stage 2 lets an agent author a
design, which DESIGN-009 SD-1 currently forbids.)*
