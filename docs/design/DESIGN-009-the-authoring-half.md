# DESIGN-009: The Authoring Half — approving a design starts the chain

**Status:** **Draft for Sam.** Not approved. Authored by Claude (Opus 5); an
authoring consistency review follows, and the approval decision is Sam's.
**Date:** 2026-07-29
**Builds on:** [DEC-005](../decisions/DEC-005-the-orchestration-boundary.md)
(accepted 2026-07-29), [DEC-003](../decisions/DEC-003-cli-scope.md),
[DESIGN-003](DESIGN-003-document-lifecycle-and-gates.md) (document lifecycle),
[DESIGN-005](DESIGN-005-dev-plans-tasks-decomposition.md) (decomposition)
**Evidence:** [alignment review](../notes/vision-alignment-review-2026-07-29.md)
§5, §6, §6a · [research conformance audit](../notes/research-conformance-audit-2026-07-29.md)
§4, §5

---

## 1. What this document is for

Cromwell can review a specification. Nothing in Cromwell can write one.

Every dispatch purpose is a review, an implementation or an estimate:
`implement-task`, `review-code`, `verify-feature`, `estimate`,
`review-<doctype>`. The starter pack ships six roles and six skills, all of them
reviewing, implementing, estimating or verifying. There is no authoring role.

The vision expected otherwise — §5 says "the Spec is drafted by a human **or AI
author**", and §8 lists `spec-author = Z` in the model routing config. It was
designed for and never built. Consequently documents enter Cromwell by exactly
one route today: a human writes the file and submits it from the CLI.

This document designs the missing half: **who writes the documents, what starts
them, and where the human sits.**

## 2. The workflow this is built for

Sam's statement of the target, which this design takes as its spine:

- **Design documents are a discussion**, co-authored by human and AI.
- **Specs are a translation** of an approved design into formal language,
  **written by agents for agents.** Human-readable and human-checkable is
  valuable; a human writing one is not envisaged.
- **The human gate sits between design and spec.** Once a design is approved,
  speccing, implementation planning and decomposition proceed with no further
  human involvement.
- **The second human gate is the approval to begin implementation.**
- **Every other gate is exception handling** — something went wrong and needs a
  person.

Rendered as the two gates and what each releases:

```
   co-author the design
            │
   ╔════════▼════════╗
   ║  GATE 1 human   ║  approve the design
   ╚════════┬════════╝
            │  ← everything below runs unattended
     write spec  ──►  review spec  ──►  write dev-plan
                                            │
                                       decompose
            ┌───────────────────────────────┘
   ╔════════▼════════╗
   ║  GATE 2 human   ║  begin implementation   (existing: StartFeature)
   ╚════════┬════════╝
            │
     implement ──► review ──► verify ──► merge   (existing, built)
```

**Most of the right-hand half already exists.** Gate 2 is `StartFeature`,
reachable only once G1 passes. The implementation loop below it runs unattended
today. Agent-approval of specs with human escalation is already the design
(vision §9), so "no human spec approval" is not a change.

**What is missing is everything between gate 1 and gate 2**, plus gate 1 itself.

## 3. What already exists, and what has to be built

| Piece | State |
|---|---|
| Document lifecycle `draft → reviewing → approved` | **Built**, and type-agnostic |
| Agent review of a submitted document, with escalation to a human | **Built** |
| Spec and dev-plan templates, and mechanical validation | **Built** |
| Decomposition of an approved dev-plan into tasks | **Built** (DESIGN-005) |
| G1, and `StartFeature` as gate 2 | **Built** |
| A design document reaching `approved` doing *anything* | **Missing** — see §4 |
| A human approving a document directly | **Missing** — see §5 |
| Any role or purpose that *writes* a document | **Missing** — see §6 |
| `design` document template and reviewer | **Missing** |
| Submitting a document from anywhere but the CLI | **Missing** (DEC-003) |

Two of these are smaller than they look.

**Design documents already travel the lifecycle.** They can be attached, drafted,
submitted, reviewed and approved today. What is missing is only the *reaction*:
`decideDocumentTransition` ([`rules.go:226`](../../internal/rules/rules.go:226))
acts on `DocApproved` only for `spec` and `dev_plan` owned by a feature. A design
document reaching `approved` triggers nothing. **Gate 1 is a rule, not new
lifecycle machinery.**

**Decomposition is already wired to dev-plan approval.** An approved dev-plan
already fires `DecomposeDevPlan` and re-evaluates G1. So once a dev-plan gets
*written*, everything downstream of it works.

## 4. Gate 1: ready-to-spec, and where the orchestrator takes over

**Sam's framing, adopted (2026-07-29).** Product planning is not depth-first. A
human lays out broad structure — sub-initiatives barely fleshed out, design
documents half-written — and iterates width-first, breaking things into children
long before anything is specced. So the design must answer: *at what point does
the orchestrator take over?*

**The answer is one line: at design approval, and nowhere earlier.**

### 4.1 Everything above that line is planning, and needs no orchestrator

Laying out a product plan, creating sub-initiatives, breaking an epic into
features, drafting and revising design documents, deciding structure — **all of
it is planning work done by a human and their chat agent, with the orchestrator
uninvolved.** This is the planning side of the vision's seam (§2:
"human-driven, AI-assisted"), and it is exactly what DEC-004 and DEC-005 exist to
permit.

**This is already built.** SPEC-008 slice 1 gives the chat agent
`create_initiative`, `create_feature`, `update_*` for titles and descriptions,
and `attach_document`. A human and their chat agent can already build an
arbitrary tree of half-formed initiatives and features conversationally. Nothing
in this design changes that, and **no `decompose-design` orchestration is
proposed** — turning a plan into sub-plans is a planning act, and automating it
would push planning across the seam onto the AI-driven side.

So a design document that describes *how an epic should break into features*
does not trigger a decomposition. It informs one, and the human and chat agent
perform it.

### 4.2 Approval always means ready-to-spec

**Approving a design document is the single act that means "this is ready to be
specced".** There is no second kind of approval and no separate readiness flag.

**Ready-to-spec is computed, not stored**, which follows DESIGN-003's standing
rule that "every gate is an expression over current state — no parallel config":

> **G0 — spec-ready.** A feature is ready to be specced when its own primary
> design document is `approved`, **or** its *immediate parent* initiative's
> primary design document is `approved`.

**Inheritance stops at one level, deliberately.** A top-level design does not
release features buried under half-baked sub-initiatives — the exact failure Sam
named. Each level of the tree is an explicit decision: to release a
sub-initiative's features, approve that sub-initiative's design. The cost is
that a deep tree needs an approval per level; that is the point, not a
side-effect.

### 4.3 What fires, and when

Width-first planning means approvals, features and descriptions arrive in any
order, so this is better expressed as **one invariant** than as a list of rules:

> **Every feature that G0 admits, and that has a description, should have a
> current spec.**

Four events can newly satisfy that condition, and each is a trigger:

1. **A design document reaches `approved`.**
2. **A feature is created** under an initiative whose design is already approved.
3. **A feature gains a description** under an approved design.
4. **A feature's spec is superseded** — see §9, the revision cascade.

Triggers 2 and 3 are what make iteration work: approve the design once, then
flesh out the tree at your own pace and each feature specs itself the moment it
is ready.

Stating it as an invariant rather than a rule list matters for a second reason.
The orchestrator is event-driven with a heartbeat as its safety net (vision §8),
so an invariant is something the heartbeat can *reconcile* — a feature that
should have a spec and does not will eventually get one even if an event was
lost. A list of rules gives no such property.

**The description guard is silent, and this is a correction to the first draft.**
That draft raised a *checkpoint* for a feature with no description. That is wrong
on §2's own principle — checkpoints are for when something has gone wrong and
needs a person, and **in width-first planning an empty placeholder feature is
entirely normal**. Approving a design over a tree of five stub features would
have fired five checkpoints for an expected situation, producing noise at exactly
the moment the system should be quiet.

So: **an undescribed feature is not dispatched, not checkpointed, and not
complained about.** It waits, and rule 3 picks it up when someone describes it.
This also removes any need for an explicit "not yet" marker on a feature — a
deliberate placeholder simply stays undescribed.

**Idempotency:** any feature with a current spec in `draft`, `reviewing` or
`approved` is skipped. Re-approving a revised design specs only what is genuinely
unspecced. Re-speccing an existing feature is a *supersede*, §8.

## 5. Who approves a design

This is the one place the existing machinery genuinely cannot express the
workflow, and it needs a decision.

**Today there is exactly one way a document becomes approved**
([`actions.go:251`](../../internal/server/actions.go:251)): an agent reviewer
returns `approve`, or a human answers a `review-escalation` checkpoint that the
reviewer raised. **A human cannot approve a document directly.** Human approval
is reachable only if an agent first declines to decide.

That is precisely backwards for a design document, where the human decision is
the point rather than the fallback.

**Per-type approval authority (Sam, 2026-07-29).** Each document type declares
who may approve it:

- `design` → **human**. Submitting moves it to `reviewing`; an agent reviewer
  runs and posts its findings as **comments**, never a verdict; the human then
  approves or requests changes.
- `spec`, `dev_plan` → **agent**, exactly as today, escalating to a human when
  the reviewer is genuinely unsure.

This keeps the agent review of designs — a reviewer that reads a design against
its ancestors catches real gaps — while making approval a human act rather than a
rubber stamp the human never sees. It matches "design documents are a
discussion": the agent joins the discussion, the human closes it.

### 5.1 Two ways a human approves (Sam, 2026-07-29)

1. **In the UI** — an explicit Approve / Request changes action on the document
   page.
2. **By telling their chat agent**, which changes the status over MCP.

Both call the same gated, audited service method. This is new authority and must
be designed as such — audit-in-transaction, no force flag (NFR-3).

**Route 2 extends the MCP facet, and the extension should be named rather than
assumed.** DEC-005's permissions are planning authoring plus *requests the
orchestrator adjudicates*. Relaying a human's approval is neither: it is an
authority act performed on the human's word. It is consistent with the spirit —
the human decided, the agent is a conduit, and it is a design approval rather
than a gate override, which DEC-005 still forbids. But it is a third category,
and DEC-005 should gain a short amendment recording it rather than have it
arrive through a design document.

**One honest limitation.** Cromwell cannot verify that the human actually said
it. With per-user authentication still a later spec (CC-6), an approval arriving
over MCP is audited to `server.mcp_actor` — `chat-agent` by default — so the
trail will read "chat-agent approved", not "Sam approved". That is acceptable for
a single-operator project and unacceptable for a shared one. **Recommendation:**
ship route 2 now, record the limitation in the audit payload explicitly
(`via: mcp`), and treat per-user identity as the thing that upgrades it.

**Alternative considered.** A design-reviewer that always escalates would reuse
the existing path with no new authority, but it abuses escalation (whose meaning
is "I could not decide") and would make every design approval look like a
reviewer failure in the audit trail.

## 6. The authoring roles

Three new roles, three new skills, three new dispatch purposes.

| Purpose | Role | Writes | Reads |
|---|---|---|---|
| `write-spec` | `spec-author` | the feature's spec | the approved design, ancestor designs, the feature's description |
| `write-dev-plan` | `dev-plan-author` | the feature's dev-plan | the approved spec, the design |
| *(revision)* | the same two | a successor draft | the review comments that sent it back |

**`design-author` is deliberately not on this list.** Designs are co-authored in
conversation with the human's chat agent over MCP (which DEC-004 already permits
and DEC-005 confirms), not written by a dispatched agent. There is no gate
upstream of a design to trigger one, and inventing designs unattended is not the
workflow.

**Restructured role shape**, per Sam's ruling of 2026-07-29 and the research
conformance audit §5.1 — identity is a job title, and the reasoning lives in
anti-pattern `because` clauses:

```yaml
model: …
skill: write-spec
identity: "Senior requirements engineer"
vocabulary:
  - "acceptance criterion (Given/When/Then)"
  - "boundary condition"
  - "requirement traceability"
  …
anti_patterns:
  - name: "Untestable Requirement"
    detect: "subjective language ('fast', 'reliable') with no measurable criterion"
    because: "a requirement that cannot be verified passes every review and fails every deployment"
    resolve: "rewrite with a measurable threshold or observable behaviour a test can assert"
  …
tools: [read_file, list_files, search, write_document]
limits: {turn_cap: 20}
```

This requires extending the `Role` schema with `vocabulary` and `anti_patterns`
(audit C-3). `~/Dev/kanbanzai/.kbz/roles/spec-author.yaml` and
`.kbz/skills/write-spec/` are working prior art for both the role and the skill,
and should be adapted rather than reinvented — selectively, per audit §5.

### 6.1 How an author returns its work — and no worktrees

An earlier draft of this section said authors need "a write path into the
worktree". **That was wrong, and worktrees are not involved at all.**

**"Submit" here does not mean git commit.** In Cromwell's vocabulary `submit` is
the document lifecycle event `draft → reviewing`
([`document.go:21`](../../internal/lifecycle/document.go:21)). It is a state
transition, unrelated to version control.

**Documents already live in the main repository, never in a worktree.**
`readDocFile` resolves paths against `RepoRoot` and refuses anything that escapes
it ([`documents.go:22`](../../internal/server/documents.go:22)); worktrees are
created only for a feature's *code*. Authoring dispatches therefore carry a
**nil `ToolCtx`** — no worktree, exactly as document reviews and estimates do
today ([`planner.go:22`](../../internal/server/planner.go:22)). Sam's concern
about documents sitting in worktrees while being iterated on is answered by the
existing architecture; this design must simply not break it.

**The outcome tool.** `submit_document` takes the document body. The server, in
one transaction, writes the file to the type's conventional path in the main
repo, registers or updates the document row, validates it, and transitions it to
`reviewing`. The agent never chooses a path, never touches git, and cannot
half-create a document — consistent with the standing convention that implicit
context comes from the orchestrator, not the agent (vision §8).

### 6.2 Freshness in the UI, and why the hook is not enough

**This is the part that needs care.** Today, a changed document is noticed by a
**git post-commit hook** → `POST /api/hook/post-commit` → `DocumentFileChanged`.
(`internal/gitwatch/` is an empty directory; the watcher named in the vision's
architecture is served by the hook.) That is DESIGN-008 §5.5's loop and it works
well for a human: edit in your own editor, commit, watch the page update itself.

**It would not work for an agent-written document.** An agent writing a file
produces an *uncommitted* change, which the hook never sees, so the UI would show
nothing until a human happened to commit.

**Resolution: the server notifies because it knows it wrote.** `submit_document`
is a server-side transaction, so it emits the change notification directly rather
than waiting to be told by git. The commit that follows is for durability and
history, not for detection — the same pattern `takeOverCanonicalPath` already
uses when it archives a superseded document
([`actions.go:305`](../../internal/server/actions.go:305)). The document is
visible in the UI the moment it is written.

### 6.3 Editing stays in the human's editor

Vision §7 is explicit that the web UI "is not an editor", and DESIGN-008 §5.5
designs the loop around that. **Nothing here proposes in-browser editing.** The
loop is unchanged for human-authored documents.

What is new is a second class of document — agent-written specs and dev-plans —
which no human edits by hand in the normal course. When a human *does* want to
change one, it is a file in the repo and the existing edit-and-commit loop
applies unaltered.

## 7. The orchestration shape, and why it is not parallel

Google Research (2026) found that on tasks requiring strict sequential reasoning
— specification, design and planning are named explicitly — **every** multi-agent
variant tested degraded performance by 39–70%, because coordination fragments
the reasoning. The same body of work found centralised coordination *improves*
genuinely parallelisable work by ~81%.

The distinction is **within** versus **across**:

| Work | Structure | Shape |
|---|---|---|
| Writing one spec | sequential reasoning | **one agent, one pass** |
| Writing one dev-plan from an approved spec | sequential reasoning | **one agent, one pass** |
| Specs for several sibling features | independent | **parallel — one agent each** |
| Tasks from one dev-plan | parallelisable | orchestrator-workers *(built)* |

So the fan-out in §4 is **across features**, and each document is written by a
single agent in a single sequential pass. Within one feature the chain is strictly
ordered — spec approved, *then* dev-plan — which is a dependency rather than a
preference: a dev-plan decomposes a spec, so there is nothing to decompose until
the spec exists.

**Unblocked is not the same as concurrent.** Gate 1 releases a chain; the chain
runs itself in order. That is the "set it going and it does not stop" property,
and the existing worker pool (default 4, which is the researched saturation
point) already bounds the fan-out.

## 8. The spec-reviewer's bar has to change

With the human gate moved *before* the spec, **the spec-reviewer becomes the only
thing standing between an approved design and an implementation built from a
lossy translation of it.** That is a materially bigger job than it has today.

The current `review-spec` skill asks for **non-contradiction** — "no
contradiction with approved background documents". A spec that silently omitted
half of its design would pass. What the workflow needs is **faithful and complete
translation**: every design decision traceable into the spec.

This is the kanbanzai failure Sam describes — a spec review that became a
document validator — in a subtler form. Cromwell is already structurally
protected against the crude version: mechanical validation runs server-side and
its report is handed *to* the reviewer, who is told to cite mechanical failures
and stop rather than perform them. It is the *bar* that is wrong, not the
architecture.

**The change (confirmed by Sam, 2026-07-29): the reviewer must verify faithful
and complete translation.** That is the bar, stated in those terms. Concretely,
`review-spec` gains a coverage criterion — every material decision in the design
appears in the spec, or is explicitly and reasonably declared out of scope — and
the verdict cannot be `approve` while a design decision is unaccounted for.
`write-spec`'s "Missing Design Reference" and "Orphaned Requirement"
anti-patterns are the author-side mirror of the same rule.

**Fidelity is also an escalation ground.** "This spec does not represent the
design and I cannot tell whether that was deliberate" is now the highest-value
thing the reviewer can catch, and it is a judgement about intent that a human
should make. It belongs in the skill's escalation guidance explicitly.

## 9. The revision cascade: a changed design invalidates its specs

**Sam's ruling (2026-07-30):** *"If a design changes then its spec is no longer
valid. If there are multiple specs all need revalidation."* Implemented as a
checkpoint on design re-approval.

Note what the two halves mean together. **Revalidation is not optional** — every
spec under a revised design must be looked at, and a design revision may never
quietly leave a stale spec standing. What the human decides is not *whether* to
revalidate but *which specs the revision actually invalidates*, which is a
judgement about intent that no computation can make.

### 9.1 The checkpoint

When a successor `design` document is approved, and features in its G0 scope have
current specs, Cromwell raises **one** checkpoint of a new kind,
`design-revision`:

> *"The design for **auth** has been revised. Which of these specs does the
> revision invalidate?"* — listing each affected spec with its feature, and
> asking for a keep/invalidate decision on each.

**One checkpoint, not one per spec.** Four specs must not produce four
checkpoints; that is the noise mistake §4.3 already corrects for stub features,
and Sam's framing was explicitly a single "which of these 4" question.

**This is a new checkpoint shape and the cost should be named.** Every existing
checkpoint answers with one verb from a fixed set
([`ui_views.go:33`](../../internal/server/ui_views.go:33)) — approve/request_changes,
override/deny, retry/cancel. This one needs a **per-item** answer, so it needs a
purpose-built form in the inbox rather than the standard verb buttons. The
checkpoint's `Context` field is already free-form JSON, so the storage side
carries it without change; the UI is the work.

**It blocks.** Until the checkpoint is answered, the affected features have a
design and a spec that disagree, and nothing downstream should proceed on them.

### 9.2 What each answer does

- **Invalidate** — the spec is superseded. That removes the current spec, the
  §4.3 invariant is no longer satisfied, and trigger 4 fires a fresh `write-spec`
  against the revised design. The chain then runs as normal: new spec → review →
  dev-plan → decompose → gate 2.
- **Keep** — the spec stands, recorded against the checkpoint answer so the audit
  trail shows the revision was considered and deliberately did not invalidate it.

Answering "keep" for everything is legitimate — a design revision that only
clarifies wording invalidates nothing — and it is a recorded decision rather than
a silence.

### 9.3 Where this meets work already in flight

If an invalidated spec belongs to a feature that is `active` or in `review`, the
replacement spec is a successor for an in-flight feature, which is a case the
engine already handles: `MarkRevisionInFlight` raises a `revision-in-flight`
checkpoint asking the human what a mid-flight contract change means
([`rules.go:210`](../../internal/rules/rules.go:210)). So the cascade terminates
in existing machinery rather than needing new handling — a design revision
reaching in-flight work surfaces as two checkpoints in sequence, which is
correct: they are two different decisions.

**Left for the SPEC:** the mechanics of superseding a spec that has no successor
document yet. `takeOverCanonicalPath` handles supersede on *successor approval*
([`actions.go:305`](../../internal/server/actions.go:305)); invalidation
supersedes first and authors the replacement afterwards, which is the reverse
order. Also unresolved: whether a spec already in `draft` or `reviewing` when the
design is revised should appear in the checkpoint list or simply be cancelled.

## 10. Submission, and the CLI

DEC-003 cannot complete while `doc submit`, `doc revise` and `doc validate` are
CLI-only — they are the last uncovered verbs on the removal checklist.

This design closes most of it as a side effect. `submit_document` (§6) gives
agents a submission path; the same service method backs a **Submit** action on
the document page, and MCP gains a submission tool (permitted by DEC-005, since
submission is a request the orchestrator adjudicates). `doc revise` follows the
same shape for a successor draft.

`doc validate` remains uncovered and probably should: validation runs on submit,
and a standalone "check this file" affordance has no clear consumer. **SD-3**
below records that.

## 11. Staging

**Stage 1 — the chain.** The `design` template and per-type approval authority
(§5), G0 and the two rules (§4), `spec-author` + `write-spec`,
`dev-plan-author` + `write-dev-plan`, `submit_document` with direct UI
notification (§6), and the strengthened `review-spec` bar (§8). This is the
load-bearing claim: **an approved design produces specs, plans and tasks with no
human involvement, and stops at gate 2.**

**Stage 2 — the surfaces.** The document-page Approve / Request-changes, Submit
and Revise actions; the MCP approval and submission tools; and the DEC-005
amendment for approval relay (§5.1). Completes the DEC-003 checklist.

**The revision cascade (§9) belongs in stage 1**, not stage 2. It is tempting to
defer as an edge case, but a design that can be approved can be revised, and
shipping stage 1 without it means the first revision leaves stale specs standing
silently — the exact failure the cascade exists to prevent. Its per-item
checkpoint form is the one piece of new UI stage 1 needs.

Stage 1 is provable without stage 2 only if a human can approve a design
somehow — so stage 1 must include *one* approval route. **The UI action is the
one to build first**, because it needs no decision amendment and is the route
that will still be correct when per-user identity arrives.

## 12. Scope reductions

- **SD-1 — no `design-author` role.** Designs are co-authored conversationally
  over MCP. Revisit only if a want appears for Cromwell to draft designs
  unattended, which contradicts §2 as written.
- **SD-2 — no revision loop cap in this design.** The uncapped review→revise
  loop is a pre-existing defect (audit C-1) affecting code review too; it should
  be fixed once, for all loops, not invented here. **This design assumes C-1
  lands**, because gate 1 makes the unattended stretch longer and therefore the
  runaway more expensive.
- **SD-3 — `doc validate` stays uncovered.** Validation runs on submit; a
  standalone affordance has no consumer. Recorded so DEC-003 can close with it
  named rather than forgotten.
- **SD-4 — no project-conventions document here.** Audit C-6 is a separate
  decision and would change what authors receive in their prompt, but it does
  not block this chain.

## 13. Questions resolved (Sam, 2026-07-29)

All four questions in the first draft are answered and folded into the sections
above.

1. **Approval authority (§5)** — per-type authority confirmed. Designs are
   human-approved; the agent reviewer posts comments, never a verdict. **Two
   human routes:** the UI, and telling a chat agent which changes status over
   MCP. Route 2 extends the MCP facet beyond DEC-005's stated permissions and
   needs a short amendment to that decision (§5.1), plus an honest note that
   without per-user identity the trail reads "chat-agent approved".
2. **Scope (§4)** — resolved by a better model than the draft's. Approval
   **always** means ready-to-spec, expressed as computed gate **G0**, and
   inheritance stops at **one level** so a top-level design never releases
   features under half-baked sub-initiatives. Structure-building — decomposing
   plans into sub-initiatives and features — stays entirely in the planning
   layer, done by human and chat agent over MCP with **no orchestrator
   involvement**, and is already built.
3. **Outcome shape (§6)** — `submit_document`, and **no worktrees**. The draft's
   mention of worktrees was an error; documents live in the main repo and
   authoring dispatches carry a nil `ToolCtx`. "Submit" is the lifecycle event,
   not a git commit. The server notifies the UI directly because it knows it
   wrote, since the post-commit hook cannot see an uncommitted agent write.
4. **Fidelity (§8)** — confirmed: the reviewer verifies **faithful and complete
   translation**, and fidelity doubt is an explicit escalation ground.

## 14. Surfacing G0 (Sam, 2026-07-29)

Sam asked whether a "ready to spec" state occurs in practice, or whether a spec
is created immediately on approval and the label would never be seen. The answer
differs by entity, and working it through produced the §4.3 correction above.

**On a feature — no label.** G0-true-and-unspecced is transient: approval fires
the dispatch at once. The only lasting case is an undescribed feature, and an
undescribed feature is already self-evident on its page.

**On an initiative — yes, and it earns its place.** An initiative is never
specced, only its features are, so ready-to-spec is not a transient step but a
**permanent standing property**: it is true from approval onward and never gets
used up. It is also not visible at a glance — telling whether a branch is
released currently means finding its primary design document and checking its
state.

**Surface the consequence, not the state.** "Design approved — features added
here are specced automatically" tells a planner what will happen next; an
"approved" chip on a document only states a fact. On a width-first planning
surface, scanning a part-built tree for which branches are open is the whole job.

Both are cheap: G0 is computed, so nothing needs storing.

**Deferred to the planning surface, not built here.** Stage 1 is the chain, and
the chain works without a label. This belongs with Stage B's work on the
initiative page, and is recorded so it is not rediscovered.

## 15. Open questions

**None blocking.** The design's open ends are now confined to mechanics the SPEC
must work out, all recorded in place:

- §9.3 — superseding a spec *before* its replacement exists, which is the reverse
  of the order `takeOverCanonicalPath` implements; and whether a spec already in
  `draft` or `reviewing` when the design is revised joins the checkpoint list or
  is simply cancelled.
- §5.1 — DEC-005 needs a short amendment covering approval relay over MCP.
- §12 SD-2 — this design assumes audit C-1 (a cap on the review→revise loop)
  lands, because gate 1 lengthens the unattended stretch.
