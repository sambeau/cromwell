# Alignment review — Cromwell against the kanbanzai lessons and vision-v1

**Date:** 2026-07-29
**Author:** Claude (Opus 5)
**Status:** Draft for Sam. **Not approved.** Its main recommendation is
[DEC-005](../decisions/DEC-005-the-orchestration-boundary.md), which is proposed,
not accepted.
**Trigger:** Sam's correction that the kanbanzai failure was *orchestration
abandonment*, not a chat agent spawning agents — and his question of how much of
what worked in kanbanzai we have kept.

---

## 1. Why this review exists

Cromwell's vision names kanbanzai's failure in one sentence (§7): the
MCP-connected chat agent "became a rogue orchestrator". [DEC-004](../decisions/DEC-004-mcp-planning-authoring.md)
turned that sentence into a rule — the MCP facet "may not spawn implementer or
reviewer agents".

Sam's account is different, and the kanbanzai record agrees with Sam. This
review checks the diagnosis against the primary sources, then asks the wider
question: of the things that worked in kanbanzai, what did Cromwell keep, and
where has it drifted from what it set out to do?

The short answer is that **the engine is sound and the most important lesson was
learned correctly**, but one rule is drawn from a misdiagnosis, and one half of
the vision was never built.

## 2. The corrected diagnosis

Two kanbanzai documents record what happened.

**`research-orchestration-abandonment-silent-dod-pass.md`** (2026-05-10)
describes a batch marked done with two queued tasks on one feature and nine on
another. Its root-cause sequence:

> Orchestrator dispatches sub-agents … loses context before completing the
> orchestration loop … Remaining tasks stay `queued` — sub-agents may have
> completed work but `finish()` was never called … Override to `done` bypasses
> child-completion validation.

**`research-kanbanzai-mcp-architecture-and-issues.md:395`** names the mechanism.
Recording an incident where the orchestrator drifted from its stated purpose and
stopped for confirmation despite an explicit prohibition, it concludes that the
orchestrator's identity and constraints sat in the *middle* of an ever-growing
context window, and therefore:

> Drift is not a model defect — it is the *predicted behaviour* given the
> U-shaped attention curve documented by Liu et al. (2024).

So the failure had two shapes, and both are the same underlying fault:

1. **The orchestrator did the work itself** instead of dispatching subagents.
2. **The orchestrator abandoned the loop** — dispatched, then forgot to finish.

The fault was **an AI agent holding the orchestration role and forgetting the
orchestration skill.** Dispatching subagents was not the failure. It was the
behaviour that got forgotten.

**Consequence for the record.** Vision §7's "rogue orchestrator" phrase compresses
this into something that reads as "the chat agent spawned agents", and DEC-004
inherits the compression as a prohibition. That prohibition forbids the wrong
thing. §6 below and DEC-005 propose the repair.

## 3. What Cromwell got right, and it is the important one

Vision §8 makes the orchestrator **code**:

> A server-side process that watches for events, decides what should happen
> next, and dispatches sub-agents to do it. It is **not** a chat agent. It has
> no model attached. It is plain code executing the workflow rules.

This is built. [`internal/rules/`](../../internal/rules/) is deterministic Go
over an event bus. It has no context window, so it cannot drift, cannot forget
its identity mid-run, and cannot be talked out of its constraints. **Both
kanbanzai failure modes are structurally impossible rather than merely
prohibited**, which is the difference between a fix and a rule.

The specific safety net the abandonment report asked for as its top priority —
"hard gate: block done with non-terminal children" — is also built. G2
([`gates.go:47`](../../internal/lifecycle/gates.go:47)) refuses to let a feature
leave `active` unless every task is done or otherwise terminal, and refuses a
feature with no tasks at all. A silent DoD pass of the kanbanzai kind cannot
happen.

This is the single most valuable thing carried over from the kanbanzai
experience, and it was carried correctly.

## 4. Inventory: what we kept

| Kanbanzai investment | Status in Cromwell | Evidence |
|---|---|---|
| **Skills** | **Kept.** Six skills ship in the starter pack, assembled into the system prompt as the procedure half. | [`internal/starter/pack/skills/`](../../internal/starter/pack/skills/), [`assemble.go:9`](../../internal/content/assemble.go:9) |
| **Roles** | **Kept and tightened.** Six roles, each declaring a tool profile the agent cannot escalate beyond — a reviewer has no `edit_file`, and a read-only purpose whose role declares a mutating tool is a config error. | [`internal/starter/pack/roles/`](../../internal/starter/pack/roles/), [`compartment.go:40`](../../internal/config/compartment.go:40) |
| **Hash-anchored read/edit** | **Kept.** `read_file(hash_tag)` returns line anchors; `edit_file(hash_ref)` validates the line has not drifted. The implement-task skill mandates it. | [`toolexec.go:94`](../../internal/server/toolexec.go:94), [`implement-task/SKILL.md`](../../internal/starter/pack/skills/implement-task/SKILL.md) |
| **codebase-memory-mcp** | **Not kept.** `search_graph` is named-but-deferred — declaring it in a role profile is a *config error* until the integration lands. The `graph_project` column exists but is always NULL. | [`compartment.go:22`](../../internal/config/compartment.go:22), [`0002_tasks_worktrees.sql:36`](../../internal/store/migrations/0002_tasks_worktrees.sql:36) |
| **U-shaped prompt construction** | **Kept in effect, though the stated rationale is caching.** Caching wants stable content first and volatile content last; attention wants constraints first and the artefact last. They want the same order, and the assembled prompt gets both — identity and skill open it, the document or diff under review closes it. See the [research conformance audit](research-conformance-audit-2026-07-29.md) §2.7. | [`assemble.go:9`](../../internal/content/assemble.go:9) |

Two notes on this table.

**The codebase-memory gap is a promise outstanding, not a silent drop.** Vision
§8 lists per-worktree graph projects as an "inherited investment" that is
"first-class in Cromwell". It is deferred under a named marker (SD-1), which is
the honest way to defer — but it is still promised and unbuilt, and it was one of
the parts Sam names as having worked well.

**The U-shaped finding was worth a second look, and it came out well.** Kanbanzai
concluded that attention-position was the *root cause* of orchestrator drift.
Cromwell removes the orchestrator from the model entirely, so that risk is gone;
and the assembled prompts dispatched agents receive turn out to be
attention-compatible already, because caching and attention want the same
ordering. The [research conformance audit](research-conformance-audit-2026-07-29.md)
covers this, along with the parts of that research Cromwell does *not* implement —
principally vocabulary routing.

## 5. Where we have drifted: nothing can write a document

This is the significant finding.

Every dispatch purpose Cromwell has is a review, an implementation, or an
estimate:

```
implement-task    review-code    verify-feature    estimate    review-<doctype>
```

The starter pack matches: six roles and six skills, all of them reviewing,
implementing, estimating or verifying. **There is no authoring role, and no
authoring dispatch purpose.** Cromwell can review a spec that already exists.
Nothing in Cromwell can write one.

The vision expected otherwise, in two places:

- §5: "The Spec is drafted by a human **or AI author**." (line 146)
- §8: model routing config is illustrated as
  "implementer = X, reviewer = Y, **spec-author = Z**". (line 385)

An author agent does not cross the planning/developing seam. Vision §2 puts spec
*shaping* firmly on the planning side — "humans (with AI help) shape what's being
built" — so an AI author is the "AI help" that half was always specified to have.
This is a gap in what was built, not a boundary Cromwell chose to draw.

**The same gap, seen from the other end.** The DEC-003 removal checklist records
`doc submit`, `doc revise` and `doc validate` as **not covered** by any UI or MCP
surface. Documents therefore enter Cromwell by exactly one route today: a human
or chat agent writes the file and submits it from a terminal. Authoring and
submission are one piece of work, and the CLI cannot retire until both are done.

## 6. The gate model against what Sam actually wants

Sam's stated intent: **two approval gates that matter**, and every other gate an
exception-handling device for when something needs human input.

1. **Approve the design** → starts orchestration of spec writing and
   implementation planning, parallelised.
2. **Approve the plan / start implementation** → runs to completion without
   stopping: implement, test, review, revise, DoD check.

Measured against that:

**Gate 2 substantially exists and works.** Once a feature is started, the rules
engine runs the loop with no human in it: dispatch ready tasks → `implement-task`
→ `review-code` → approve or return → all tasks terminal (G2) → `verify-feature`
against acceptance criteria → merge (G3). Humans are pulled in only when a
reviewer or verifier *escalates*, or when an agent returns an unparseable
outcome. That is the "set it going and don't stop" behaviour, and it is real.

**Gate 1 does not exist.** There is no design-approval gate, and nothing to
dispatch if there were — see §5. G1 is the nearest thing, but it fires when the
spec and dev-plan are **already approved**, which is the *end* of the work Sam
wants gate 1 to *start*. The orchestration between design-approved and
spec-approved is the missing middle of the system.

**The exception gates are already the right shape.** Ten checkpoint kinds exist
— `review-escalation`, `verification-escalation`, `gate-override`,
`dispatch-failure`, `revision-in-flight`, `budget`, `config-error`,
`merge-conflict`, `worktree-failure` and `document-integrity`. Every one of them
is raised because something needs a human, and none is a routine approval step.
Six are answerable from the inbox
([`ui_views.go:33`](../../internal/server/ui_views.go:33)); the rest are raised
when an operation fails. This matches Sam's intent exactly, though it appears to
have arrived there without being checked against it.

So the gate model is two-thirds right, and the missing third is the same missing
piece as §5.

### 6a. Sam's clarification of the target workflow (2026-07-29)

Recorded because it settles several open questions and narrows R-2:

- **Design documents are a discussion, co-authored** by human and AI.
- **Specs are a translation** of an approved design into formal language,
  **written by agents for agents**. Human-readable and human-checkable is useful;
  a human writing one is not envisaged.
- **The human gate sits between design and spec — not after the spec.** Once a
  design is approved, speccing, implementation planning and decomposition all
  proceed without further human involvement.
- **The second human gate is the approval to begin implementation.**
- Everything else is exception handling.

**Most of this is already how Cromwell works.** Vision §9 already makes documents
agent-approved, with humans pulled in only on escalation, so "no human spec
approval" is the existing design rather than a change. And the second gate exists:
`StartFeature` is a human action, reachable only once G1 has passed.

**What is missing is the first gate, and its mechanism is closer than expected.**
Design documents can already be attached and can already travel the document
lifecycle to `approved`. But `decideDocumentTransition`
([`rules.go:226`](../../internal/rules/rules.go:226)) reacts to approval only for
`spec` and `dev_plan` on features — **a design document reaching `approved`
triggers nothing today.** Gate 1 is therefore a rule on design approval plus the
authoring roles from §5, not new lifecycle machinery. The starter pack ships no
`design` template and no design-reviewer role, which R-2 must add.

**One risk this clarification raises, and it is not the one Sam asked about.** In
kanbanzai the spec review degraded into a document validator — checking headings
and checklists rather than judging whether the spec faithfully represents the
design. Cromwell is structurally protected against that exact drift: mechanical
validation runs server-side and its report is handed *to* the reviewer, who is
told to cite mechanical failures and stop rather than perform them
([`review-spec/SKILL.md`](../../internal/starter/pack/skills/review-spec/SKILL.md)).

But the *bar* is wrong for the workflow above. The skill's consistency criterion
asks for **non-contradiction** — "no contradiction with approved background
documents". A spec that silently omitted half its design would pass that. What
Sam wants is **faithful and complete translation**: every design decision
traceable into the spec. With the human gate moved *before* the spec, the
spec-reviewer becomes the only check that the design survived translation, so
this criterion carries much more weight than it does today. R-2 must strengthen
it, and `write-spec`'s "Missing Design Reference" and "Orphaned Requirement"
anti-patterns (§5.3) are the shape of the fix.

## 7. Recommendations

**R-1 — Redraw the DEC-004 line.** It prohibits agent spawning, which is the
cure rather than the disease. Proposed as
[DEC-005](../decisions/DEC-005-the-orchestration-boundary.md), which supersedes
DEC-004 in part. **Sam's to accept or redirect.**

**R-2 — Design the authoring half.** An author role and dispatch purpose, a
design-approval gate that starts it, and the parallelisation Sam wants. This
needs its own design document and should not be started until R-1 is settled,
because R-1 determines whether the chat agent may ask for it. It should absorb
the `doc submit` / `doc revise` gap in the same package, since they are one
problem. **Constraint from the research** (conformance audit §4): the
parallelism belongs *across* independent features, never *within* the writing of
one document — parallelising sequential reasoning work measured 39–70% worse.
**Prior art exists** (conformance audit §5.2): `~/Dev/kanbanzai/.kbz/` holds a
working `spec-author` role and `write-spec` / `write-design` / `write-dev-plan`
skills, so this package adapts rather than invents.

**R-3 — Decide the fate of the codebase-memory-mcp integration.** It is promised
in the vision as inherited and is currently a config error to declare. Either
schedule it or downgrade the promise; leaving it as a named deferral indefinitely
is the thing the project's own discipline warns against.

**R-4 — Two defects, then a prompt-quality slice.** The
[research conformance audit](research-conformance-audit-2026-07-29.md) found an
uncapped review→revise loop and an evidence contract that is asked for but never
enforced (C-1, C-2 — both small, both real). Behind them sits the larger gap:
none of the six roles or skills carries a vocabulary payload, named
anti-patterns or worked examples, which the research calls its highest-ROI lever
(C-3, C-4).

*This supersedes an earlier draft item asking whether prompt-assembly ordering
conflicts with the attention research. It does not — §4 of this review now
records why.*

**Sequencing.** R-1 first, alone, because it is a decision and it gates R-2. Then
R-2 as a full planning package, shaped by the sequential-penalty constraint. R-3
is independent. R-4's two defects (C-1, C-2) are small enough to go whenever;
its prompt-quality half is its own slice.

None of this displaces the two outstanding DoD live smokes, which still need a
database and a person, and which remain the only thing blocking SPEC-007 and
SPEC-008 from being called done.

## 8. What this review did not examine

- The sizing, calibration and estimation machinery (phase 3) against kanbanzai's
  estimation work — not looked at.
- Kanbanzai's document-intelligence and knowledge subsystem, which vision §10
  claims to replace with auto-surfaced documents. The vision's claim that
  retrieval-by-default fixes the "86 contributed, 12 retrieved" problem is
  plausible but unverified in practice.
- Whether the skills themselves are still the right procedures, as opposed to
  merely present. Vision §14 flags that the document-writing skills in
  particular need revision.
