# Research conformance audit — Cromwell against the kanbanzai research corpus

**Date:** 2026-07-29
**Author:** Claude (Opus 5)
**Status:** Draft for Sam. **Not approved.**
**Companion to:** [alignment review](vision-alignment-review-2026-07-29.md), which
audits Cromwell against the kanbanzai *lessons*. This audits it against the
kanbanzai *research* — the academic and industry findings gathered during the
3.0 design work.

**Sources audited against:**

- `refs/prompt-engineering-guide.md` — the 10 Principles, distilled from 17 papers
- `research-ai-agent-best-practices-research.md` — principle-by-principle analysis
- `research-skill-authoring-best-practices.md` — the skill-authoring synthesis
- `research-agent-orchestration-research.md` — 12 sources on orchestration
- `research-orchestration-landscape-2025.md` — the build-vs-adopt survey

---

## 1. Headline

**Cromwell implements the research's single most important recommendation
better than kanbanzai ever did, and misses its single highest-ROI one entirely.**

The orchestration research's top recommendation — "enforce lifecycle gates at the
tool level", the thing it calls "the single highest-impact change" — is
Cromwell's foundation rather than a to-do. Meanwhile the prompt research's
headline finding, vocabulary routing, appears nowhere in Cromwell's roles or
skills.

The two are unrelated halves of the system: the *machinery* is aligned, the
*prompts* are not.

## 2. What Cromwell gets right

### 2.1 Enforceable constraints — the research's top recommendation, already built

Every source that compared advisory instructions with enforced ones found
enforcement wins: MetaGPT (ICLR 2024) reduced cascading errors ~40% with
verification gates; Masters et al. (DAI 2025) formalised hard constraints (ℋ)
whose violation terminates the workflow; Google Research (2026) measured error
amplification at 4.4× with a centralised validator versus 17.2× without.

The orchestration report's recommendation 4.1 spells out what kanbanzai should
have done:

> Make the lifecycle state machine *architecturally enforceable*, not just
> advisory … This is the single highest-impact change because it converts the
> entire class of "agents skip steps" failures from a quality problem into an
> impossibility.

Cromwell does exactly this. G1–G5 are Go functions
([`gates.go`](../../internal/lifecycle/gates.go)); the orchestrator is
deterministic code with no model attached; there are no force flags; every
override runs through an answered checkpoint. The gates are not instructions an
agent is trusted to honour — they are conditions code evaluates.

**This is the strongest alignment in the audit, and it is load-bearing. Nothing
in the roadmap should weaken it.**

### 2.2 Separation of generation from evaluation

P6 ("self-evaluation fails: the generator shares the evaluator's biases") is
implemented deliberately, not incidentally. The implementer and code-reviewer are
distinct roles with distinct dispatches, and the verifier's identity says outright
that it "did not build this code and you are not shown how it was built — that is
deliberate."

### 2.3 Per-role tool scoping — ahead of the research's ask

P9 and recommendation R7 ask for "adaptive MCP tool filtering … expose only
relevant tool definitions to each sub-agent role". Kanbanzai never built it.

Cromwell's role profiles declare tools explicitly, and the config layer makes a
mismatch a *startup error* rather than a runtime accident: a role bound to a
read-only purpose that declares `edit_file` fails validation
([`compartment.go:40`](../../internal/config/compartment.go:40)). The
spec-reviewer, dev-plan-reviewer and estimator have `tools: []` — they cannot
touch the filesystem at all.

This is the research's recommendation implemented as a hard constraint. It also
happens to satisfy the orchestration report's 3.3 finding that specification
agents "should have *no access* to implementation tools".

### 2.4 Structured outcomes, not freeform prose

MetaGPT's core mechanism — structured intermediate artefacts verified before the
next stage — appears as the outcome tools. A reviewer cannot end its turn with
an opinion; it must call `submit_review` with a typed verdict, and a verifier
must call `submit_verification` with a per-criterion array. The turn loop fails
the dispatch if the outcome tool is never called
([`dispatch.go:440`](../../internal/dispatch/dispatch.go:440)).

Document templates (`templates/spec`, `templates/dev_plan`) supply the other half
— the forcing function the research says distributes effort across sections
rather than allowing rush-to-implementation.

### 2.5 Identities are PRISM-compliant

PRISM (2024) found flattery actively degrades output by activating marketing
text, and that brief real-job-title identities beat elaborate personas. Cromwell's
six identities contain **no flattery whatsoever** — no "world-class", no
"expert", no superlatives. They are plain descriptions of responsibility.

They also, unprompted, follow P5's "explain why" rule. From
[`spec-reviewer.yaml`](../../internal/starter/pack/roles/spec-reviewer.yaml):

> Approving weak specs pushes their cost downstream where it is far more
> expensive; escalating routine calls wastes the humans' attention and defeats
> the system's purpose. Both failure modes matter equally.

That is a BECAUSE clause explaining a consequence chain, which the research says
is what makes a rule generalise to adjacent cases.

They are longer than PRISM's <50 tokens (roughly 90–150). I do **not** think that
is a defect: the token limit was measured against *persona* padding, and this
content is constraint and rationale, which other findings actively want. Worth
knowing, not worth fixing.

### 2.6 Worker concurrency defaults to exactly the researched optimum

DeepMind's scaling work found team performance saturates at 4 agents and degrades
past it. Cromwell's `Dispatch.Workers` defaults to **4**
([`config.go:215`](../../internal/config/config.go:215)). Whether by instinct or
inheritance, it is the right number.

### 2.7 Prompt ordering is attention-compatible — a correction

The alignment review speculated that Cromwell's prompt assembly, being ordered for
provider caching, might conflict with the attention findings. **Having read the
assembly, it does not.** The order is:

| Position | Content | Attention |
|---|---|---|
| System, top | Role identity, then skill procedure | High ✅ |
| User, early | Project, background documents | High |
| User, middle | Feature context | Low — but it is short |
| User, late | Task instruction, validation report, prior comments | Rising |
| User, last | **The document or diff under review** | High ✅ |

Caching wants stable content first and volatile content last. Attention wants
constraints first and the thing being acted on last. **They want the same
ordering**, and Cromwell gets both. The artefact under judgement lands in the
highest-attention position at the end, which is exactly right.

One caveat: background documents are the longest element and sit early, so as an
initiative's document corpus grows they will push the feature context and task
instruction deeper into the attention valley. Worth watching, not yet a problem.

## 3. Where Cromwell does not conform

### 3.1 No vocabulary routing — the highest-ROI gap

This is the research's headline finding, stated in the prompt guide as "the #1
quality lever" and in the best-practices report as "the single most impactful
finding across the research". Ranjan et al. (2024) showed domain vocabulary acts
as a routing signal selecting which knowledge clusters the model activates. The
recommendation is 15–30 precise domain terms per role or skill, each passing the
15-year-practitioner test.

**Cromwell has none.** Not one of the six roles or six skills contains a
vocabulary payload. The identities describe *responsibility* ("you inspect one
task's diff and decide whether it correctly implements that task") but never
supply *domain terms*.

The consequence is concrete. A Go code-reviewer whose prompt never says
"goroutine leak", "error wrapping with `%w`", "table-driven tests", "context
cancellation", "interface segregation" is routed — per the research — toward
generic blog-post-level review rather than Go engineering knowledge. The skill
tells it *what to check*; nothing tells it *what expertise to check with*.

Kanbanzai identified this as its own top recommendation (R1) and never shipped it.
Cromwell has inherited the gap.

### 3.2 No named anti-patterns, no examples, no retrieval anchors

The research-backed skill architecture is:

```
frontmatter → vocabulary payload → anti-pattern watchlist → procedure
→ output format → 2–3 BAD/GOOD examples → questions this skill answers
```

Cromwell's six skills are all `frontmatter → procedure → verdict guidance →
mechanics`. Three of the seven sections are absent across every skill:

- **Named anti-patterns** — zero. The research says named patterns ("the
  eager-loading trap") activate expert clusters where unnamed problems ("don't
  load too much") get generic responses.
- **BAD/GOOD examples** — zero. LangChain found 3 well-chosen examples match 9,
  and Anthropic's guidance is that input/output examples "train understanding
  better than abstract instructions".
- **Retrieval anchors** — zero, and this one is arguably moot: Cromwell's skills
  are *dispatched*, not *retrieved*, so there is no search step for anchors to
  serve. I would skip anchors and spend the effort on vocabulary and examples.

The skills are well-written — clear, concrete, appropriately terse, with real
guidance like "'Works well' is not a criterion; 'returns within 2 seconds for
inputs up to 10k rows' is." That single line is doing the work an examples section
would do, which suggests the authors' instincts were right and only the structure
is missing.

### 3.3 No iteration cap on the review loop — a real runaway risk

Microsoft's maker-checker pattern and the skill-authoring synthesis both specify
the same control: **maximum 2–3 revision cycles, then escalate to a human.** The
research is explicit that an iteration cap exists "to prevent infinite refinement
loops".

Cromwell has no such cap. `returnTaskCode`
([`actions_phase2.go:365`](../../internal/server/actions_phase2.go:365))
transitions the task back to active and re-dispatches the implementer
unconditionally. Implement → review → request_changes → implement → … has no
round counter and no escalation threshold.

The only backstop is the **global period budget cap**
([`dispatch.go:250`](../../internal/dispatch/dispatch.go:250)) — a project-wide
spend ceiling, not a per-task loop guard. A task can churn many rounds before it
trips, and when it does it stops *all* work, not the pathological task.

Note `Dispatch.MaxAttempts = 3` exists but governs something else entirely:
retries of a *failed* dispatch (a provider error), not rounds of a *successful*
review that keeps requesting changes.

This is the one finding I would treat as a defect rather than an enhancement. It
is also thematically the kanbanzai failure family — work that spins without a
human noticing.

### 3.3a How the loop should terminate (Sam, 2026-07-30)

The research prescribes a round cap. Sam's practice suggests the cap is the
*wrong primary control*, and the reason is an observation about which failure
actually occurs:

> In practice the loop does not stall on disagreement. It stalls on the reviewer
> querying finer and finer detail at a more and more pernickety level.

A round cap applied to that failure escalates **trivia to a human**, which is
noise and breaks the standing rule that human interruptions are for things going
wrong. The pattern Sam reports working:

1. Find all major and minor issues.
2. Fix all of them.
3. Repeat while any *major* issues are found.
4. Stop when only minor issues remain, and drop them.

**The two controls are complementary, not alternatives.** Severity-gated
termination ends the common case correctly and without a human; the hard cap
catches genuine disagreement — majors recurring round after round — which is the
case the research was actually describing. Keep both.

The termination argument holds: by round *n* with no majors found, the surviving
minors have passed *n* reviews without anyone judging them major.

**What it requires.** `ReviewComment` is `{section_ref, body}`
([`rules.go:115`](../../internal/rules/rules.go:115)) — severity is a new field.
More interestingly, **the verdict stops being a free choice and becomes
derived**: any major → `request_changes`; no majors → `approve`; `escalate`
remains free and orthogonal. That removes a class of reviewer inconsistency and
doubles as a rubber-stamp defence — a reviewer cannot approve while holding a
major finding, which attacks §3.4 from the other side.

**Two risks.**

- **Severity inflation** — everything marked major, the loop never terminates,
  the cap fires every time and nothing improves. Kanbanzai anticipated this:
  `reviewer.yaml` carries a *Severity Inflation* anti-pattern triggering above
  30% of findings at the top tier. Port it.
- **Severity deflation** — the inverse, and worse: a real correctness bug marked
  minor is dropped silently at step 4. That is rubber-stamping in a new costume.
  Mitigate by defining *major* concretely rather than by feel — `review-code`
  already carries the right four criteria, so a failure of **Correct, Complete,
  In scope or Sound** is major and style, naming and tidiness are minor.

**Dropped minors are recorded, not discarded.** They are exactly the material
Sam reports retrospectives being valuable for — complete the cycle, then go back
and pick up what was noted along the way. Logging them against the task costs
nothing now and feeds C-9.

### 3.4 Rubber-stamp approval is half-defended

The MAST taxonomy names rubber-stamp approval (FM-3.1) as **the single most
common quality failure in multi-agent systems**, and the research's counter is
structural: require specific evidence per dimension, not a clean verdict.

Cromwell is genuinely good here at the design level. `verify-feature` demands
per-criterion evidence and gives the calibrating example — "'Met — test TestLogin
covers valid and invalid credentials' is evidence; 'looks fine' is not" — and the
outcome tool takes a `criteria` array with an `evidence` field. That is the
research's recommendation, expressed as a schema rather than a plea.

**But nothing enforces it.** `ParseVerificationOutcome`
([`rules_phase2.go:132`](../../internal/rules/rules_phase2.go:132)) validates only
that the verdict is one of three strings. An `approve` with an **empty criteria
array**, or with every `evidence` field blank, parses successfully and merges the
feature. The gate that the research calls the most important one is advisory
precisely where Cromwell is elsewhere strict.

`review-spec` is weaker still: it asks for "one or two sentences" of reasoning on
approve, which is exactly the low bar FM-3.1 describes.

### 3.5 No review-quality observability

The research gives a concrete rubber-stamp detector: approval rate above 85%
combined with very short review times. It recommends tracking approval rate,
findings per review, and time to verdict per reviewer.

Cromwell's *structural* audit trail is strong — every transition and dispatch is
audited in-transaction, and token consumption is recorded per dispatch. But
nothing aggregates review behaviour, so a reviewer that has approved forty
consecutive diffs in nine seconds each is invisible. The data to compute this
largely exists in the audit and dispatch tables; no read surfaces it.

### 3.6 Effort expectations are capped but never communicated

Anthropic's multi-agent work found agents "struggle to judge appropriate effort",
and specifically that they under-invest in specification because writing
documents does not feel productive. The recommendation is to state expected
effort in the prompt: "expect 5–15 tool calls".

Cromwell has `turn_cap` per role (6–40), which is a *ceiling enforced by code* —
a hard constraint, which the research generally prefers to advice. But the agent
is never told the number, and a ceiling is not a floor: nothing discourages a
reviewer from approving after one turn. The research's concern is under-investment,
and a cap does not address it.

## 4. The finding that changes the roadmap

**Google Research (Kim & Liu, 2026): on tasks requiring strict sequential
reasoning, *every* multi-agent variant tested degraded performance by 39–70%.**
Specification, design and planning are named as exactly these tasks. The
skill-authoring synthesis states the rule bluntly: "specification and design work
should never be parallelised."

Sam's first gate is "approve the design → orchestration of spec writing and
implementation planning, **parallelisation etc**". Read naively, that is the
pattern the research says degrades quality by up to 70%.

**The gate is still right; the parallelism needs placing.** The same body of
research found centralised multi-agent coordination *improves* parallelisable work
by ~81%. The distinction is whether the sub-tasks are genuinely independent:

| Work | Structure | Pattern |
|---|---|---|
| Writing one spec | Sequential reasoning | **One agent, no parallelism** |
| Writing one dev-plan from an approved spec | Sequential reasoning | **One agent, no parallelism** |
| Specs for several independent sibling features | Independent | **Parallel — one agent each** |
| Reviewing a spec | Evaluative | Maker-checker, already built |
| Implementing tasks from one dev-plan | Parallelisable | Orchestrator-workers, already built |

So the design-approval gate should fan out **across features**, and each
individual document should be written by a single agent in one sequential pass.
That is a design constraint for the R-2 package, and it is better to know it now
than to discover it in a walkthrough.

## 5. The prior art: kanbanzai shipped the research template

**Added after Sam pointed to `~/Dev/kanbanzai/.kbz/roles` and `.kbz/skills`.**
This materially changes the size of C-3 and C-4 below, and de-risks R-2.

Kanbanzai's roles are not prose. They are the research-backed template expressed
as a schema:

```yaml
id: spec-author
inherits: base
identity: "Senior requirements engineer"     # real job title, <50 tokens
vocabulary:                                   # the routing signal
  - "acceptance criteria (Given/When/Then)"
  - "equivalence partition"
  - "INVEST criteria (Independent, Negotiable, …)"
anti_patterns:
  - name: "Untestable Requirement"            # named — activates expert clusters
    detect: "…subjective language ('fast') with no measurable criterion"
    because: "…unenforceable — passes every review and fails every deployment"
    resolve: "Rewrite with a measurable threshold…"
tools: [...]
```

Every element the research prescribes is present and correctly used: real job
titles ("Senior application security engineer", "Methodical close-out auditor"),
9–15 vocabulary terms per role, and anti-patterns in exactly the
detect → BECAUSE → resolve form. `reviewer-security.yaml` is the textbook case,
carrying "OWASP Top 10 (2021)", "STRIDE threat model", "CVSS v3.1 scoring",
"insecure direct object reference (IDOR)" — the precise vocabulary the research
uses as its worked example of routing depth.

**Three of these findings bear directly on gaps identified above.**

**The rubber-stamp fix already exists as a prompt.** `reviewer.yaml` carries an
anti-pattern named "Rubber-stamp Approval", citing MAST FM-3.1 by name, whose
resolution is: "clearance requires at least one finding or an explicit
per-dimension rationale for why none exist." That is C-2's fix, written a year
ago. Cromwell has neither this prompt-level defence nor the schema-level
enforcement.

**Not everything should port, and one role proves it.** `verifier.yaml` has an
anti-pattern called "Accepting Orchestrator Assurances", explaining that "the
orchestrator is the worst agent to trust for close-out — by end of cycle its
context is saturated and close-out steps are exactly what it forgets." That is
the abandonment failure written as a warning to another agent. **In Cromwell it
is obsolete**: the orchestrator is code with no context to saturate. Porting it
would teach agents to guard against a failure that cannot occur, spending
attention budget on nothing. Any port must be selective and reasoned, not
wholesale.

**`base.yaml` uses the schema loosely and should not be copied as-is.** Its
`vocabulary` list holds conventions ("Spec is law — implement what the
specification requires") rather than domain terms. The specialist roles use the
field correctly. If Cromwell adopts the schema, it should adopt the specialists'
reading.

### 5.1 What this means for Cromwell's role schema

Cromwell's `Role` struct ([`compartment.go:44`](../../internal/config/compartment.go:44))
has `Model`, `Skill`, `Identity`, `Tools`, `ToolHints`, `Limits` — and no
`vocabulary`, no `anti_patterns`, no `inherits`. Adding the first two is a small
additive change, though note `strictUnmarshal` rejects unknown fields, so it has
to be a deliberate schema extension rather than something a project can add on
its own today.

**A real decision sits inside it.** Cromwell's identities are multi-paragraph
prose; kanbanzai's are a job title plus structured fields. Cromwell's prose is
good — the spec-reviewer's "Approving weak specs pushes their cost downstream
where it is far more expensive" is a genuine BECAUSE clause — but the research
is explicit that structured metadata "outperforms prose for identity declaration
because it is unambiguous and compact". Two options:

- **Additive** — keep the prose identity, add `vocabulary` and `anti_patterns`
  beside it. Lower risk, preserves good writing, but leaves the identity above
  the researched token budget and the reasoning scattered.
- **Restructured** — `identity` becomes the job title, and the existing prose
  relocates into anti-pattern `because` clauses where the research says such
  reasoning belongs. Truer to the evidence, and nothing well-written is lost —
  it moves.

**Sam's ruling (2026-07-29): restructured — follow the research.** `identity`
becomes the job title; the existing prose relocates into anti-pattern `because`
clauses. Nothing well-written is deleted, it moves to where the evidence says
that kind of reasoning belongs.

**One constraint kanbanzai did not face.** Its roles were written for kanbanzai —
a Go project — so `implementer-go.yaml` could carry "goroutine leak (context
cancellation, defer cleanup)" and "error wrapping (%w verb, sentinel vs opaque
errors)". Cromwell's starter pack ships into *any* project, so language-specific
vocabulary cannot be baked into the default `implementer`. Kanbanzai answered
this with **two layers**, not one — see §5.2, which is the more important half of
this finding.

### 5.2 The second layer: Cromwell tells agents nothing about the project

**Added after Sam pointed to `.kbz/context/roles`.** This is a distinct system
from `.kbz/roles`, and the distinction is the answer to the generic-versus-
project-specific problem above.

Kanbanzai ran two layers:

| Layer | Location | Holds | Scope |
|---|---|---|---|
| **Role** | `.kbz/roles/` | Job title, vocabulary, anti-patterns, tools | Portable — who the agent *is* |
| **Context profile** | `.kbz/context/roles/` | Conventions, architecture summary, key interfaces, packages | Per-project — what this *codebase* is |

The context profiles are short (21–27 lines) and concrete. `base.yaml` gives every
agent, before it reads a single file:

> "Error handling: wrap errors with fmt.Errorf and %w for context propagation;
> return errors, never panic" · "Tests: table-driven with t.TempDir()" ·
> "No init() functions; no global mutable state" · "Spec is law: if code
> contradicts the specification, surface the conflict — do not resolve silently"

plus an `architecture.summary` and a list of `key_interfaces` naming the real
types an agent will meet. `developer.yaml` inherits it and adds package scope and
Go conventions.

**Cromwell has no equivalent, and the gap is total.** An implementer's prompt
([`planner.go:199`](../../internal/server/planner.go:199)) is exactly four
things: the task, the spec, the dev-plan, and a paragraph on which tools to use.
The starter pack has `roles/`, `skills/` and `templates/` — and no third
directory. Nothing in the config layer mentions conventions or architecture.

The `implement-task` skill says "Match what is already there: a task is not a
licence to restyle the project" — so Cromwell *knows* conventions matter, and
then makes every implementer rediscover them by reading code, on every dispatch,
paying tokens each time and arriving at a different answer each time. This is the
"institutional memory" principle (P5) and the research's R4 recommendation
("proactive knowledge surfacing … automatically include relevant always/never
rules in context packets") with nothing behind it.

**Two things make this more interesting than a missing feature.**

First, it corrects the framing above: the starter pack does *not* need to solve
project-specific vocabulary through role inheritance. The generic roles ship
vocabulary about the *discipline* ("acceptance criteria", "boundary condition"),
and a project-owned context layer supplies vocabulary about the *codebase*
("wrap with %w", "MarshalCanonicalYAML"). Those are different things and want
different homes. Role `inherits` becomes optional rather than necessary.

Second, **Cromwell may already have the right home for it and not be using it.**
Vision §10 collapses kanbanzai's knowledge subsystem into documents — "a note is
a short document", with the same lifecycle and retrieval machinery — and the
review path already auto-surfaces approved ancestor documents with provenance
([`assemble.go:63`](../../internal/content/assemble.go:63)). So the natural
Cromwell answer is probably not a new YAML format but **a project-level
conventions document, approved like any other, auto-surfaced into dispatch
prompts** — which would satisfy the vision's own claim to have replaced the
knowledge subsystem.

Worth noting alongside it: the implementer and code-reviewer paths surface **no
ancestor documents at all**, only the spec and dev-plan. That may well be
deliberate — the spec is the contract and the vision calls it the handover — but
it means the document-surfacing machinery that exists on the review path has
never been extended to the execution path, and a conventions document would need
that extension.

I would treat this as its own decision rather than folding it into C-3.

#### What Sam's own experience adds — push, never pull

Sam's account of what happened in kanbanzai, which is primary evidence from the
person who ran it:

- **The knowledge system was dropped** — agents ignored it, preferring to search
  the documents directly.
- **Project memory** was written but barely ever read.
- **Retrospectives worked well** — "what went wrong, what went right, what should
  we change" hardened the system and let cycles complete and then be revisited to
  pick up recorded bugs.

Two of these are the same failure and it is not the one the vision names. Vision
§10 diagnoses kanbanzai's knowledge problem as *opt-in retrieval* — "86 knowledge
entries were contributed but only 12 ever retrieved". Sam's account sharpens it:
agents did not simply forget to look, **they looked somewhere else**. They
searched the documents, which is the interface they already knew. That is
SWE-agent's ACI finding exactly — tools that match what the model has seen in
training beat purpose-built novel interfaces.

The design rule that follows is sharper than "make retrieval automatic":

> **Anything an agent must choose to fetch will not be fetched. Conventions must
> be pushed into the prompt. And where an agent does pull, it should pull the way
> it already knows how — by searching files.**

This constrains C-6 in two directions, and it means Cromwell should not build a
knowledge subsystem, a memory store, or a design-document graph. That experiment
has been run twice. If a knowledge graph is ever wanted again, Sam's position is
that it should be a separate system or an existing one, not a Cromwell subsystem.

**A consequence worth naming.** Cromwell's implementer profile is
`[read_file, list_files, edit_file, write_file, run_command]` — it has **no
search tool**. `search_graph` is deferred, and there is no `grep`. So the one
behaviour agents demonstrably fall back on is unavailable to Cromwell's
implementer, which is told to "match what is already there" and given only
directory listing and file reads to do it with. Adding a search tool is cheap and
works with the grain of what agents actually do.

#### Retrospectives: in the vision, not in the code

Vision §14 says Cromwell "preserves kanbanzai's retrospective concept" and argues
it should be *richer* here, because the audit log, cost ledger and calibration
data make observations computable — "Auth/basic shipped 30% under estimate" as a
fact rather than a feeling.

**Nothing of it is built.** There is no retrospective entity, no signal capture,
no clustering; the only occurrences of the word in the repository are in the
vision itself. Given this is the one part of kanbanzai's knowledge machinery that
Sam reports actually worked, it is the strongest candidate of anything in this
audit for a future slice — and unlike the knowledge system it is *push* by
nature: observations are recorded at the moment of friction, by whoever hits it,
rather than retrieved later by someone who has to remember to ask.

Not scoped here. Flagged as C-9.

### 5.3 Prior art for the authoring gap (R-2)

`.kbz/skills/` contains `write-spec`, `write-design` and `write-dev-plan` — the
authoring half Cromwell lacks entirely. `write-spec/SKILL.md` is complete: a
dual-register description (expert and natural registers), triggers, `roles`,
`stage`, `constraint_level: high`, a 21-term defined vocabulary, six named
anti-patterns, and a `references/` directory holding examples and quality
criteria as progressive-disclosure level 3. `write-design` additionally ships
`scripts/validate-design-structure.sh` — deterministic structural validation,
the Hardening Principle applied to the authoring step.

Cromwell already has the server-side equivalent of that script (validation runs
before review and its report is injected into the reviewer's prompt), so the
gap is the skill and role, not the checking.

**This substantially de-risks R-2.** The authoring design does not start from a
blank page; it starts from a working skill, a working role, and a template, all
of which need adapting to Cromwell's document lifecycle rather than inventing.

## 6. Recommendations

Ordered by evidence strength times effort. These are proposals, not decisions.

| # | Recommendation | Basis | Size |
|---|---|---|---|
| **C-1** | **Severity-gated termination, with a hard cap as backstop** (§3.3a). Findings gain a severity; the loop repeats while *major* findings exist and stops when only minors remain; a hard round cap escalates genuine disagreement. Leftover minors are recorded, not discarded. | Microsoft maker-checker; Sam's practice; kanbanzai's Severity Inflation anti-pattern | Small–medium; closes the runaway *and* sharpens the verdict |
| **C-2** | **Enforce the evidence contract**: reject a verification `approve` with an empty criteria array or blank evidence, the same way an unparseable outcome is rejected today. | MAST FM-3.1 (#1 quality failure) | Small |
| **C-3** | **Extend the role schema** with `vocabulary` and `anti_patterns` (and decide on `inherits`), then populate the six roles — adapting kanbanzai's, not writing from scratch. Decide additive-vs-restructured identity first (§5.1). | Ranjan et al. 2024; the research's #1 lever; prior art in `.kbz/roles` | Medium — smaller than it was, given §5 |
| **C-4** | **Add named anti-patterns and one BAD/GOOD pair** per skill. Reuse the calibration lines the skills already contain, and the kanbanzai skills where they still apply. | LangChain; Anthropic; P5/P10 | Medium |
| **C-5** | **Shape the R-2 authoring design around the sequential penalty** — parallel across features, single-agent per document — and start from `.kbz/skills/write-spec` rather than a blank page (§5.2). | Google Research 2026; prior art | Design constraint, no code |
| **C-6** | **A project-conventions document, auto-surfaced into every dispatch prompt, with a hard size cap enforced at validation** (§5.2). Push, never pull; small by construction. Requires extending document surfacing to the execution path. **Not** a knowledge system. | P5; research R4; `.kbz/context/roles`; Sam's push-not-pull evidence | Medium |
| **C-6a** | **Give the implementer a search tool.** It has none, and searching files is the one retrieval behaviour agents reliably fall back on. | SWE-agent ACI; Sam's evidence | Small |
| **C-9** | **Design a retrospective mechanism** — the one part of kanbanzai's knowledge machinery that worked. Push by nature. Richer here than in kanbanzai because the audit log, ledger and calibration data make observations computable. Vision §14 promises it; nothing is built. | Vision §14; Sam's evidence | Its own slice |
| **C-7** | **Surface review metrics** — approval rate, findings per review, time to verdict. The data is mostly in the audit and dispatch tables. | MAST; observability imperative | Medium |
| **C-8** | **State effort expectations** in dispatched prompts alongside the existing turn cap. | Anthropic multi-agent | Small |

**Sequencing.** C-1, C-2 and C-6a are small and independent of everything else.
C-5 is free and must land before R-2 is written. C-3, C-4 and C-6 belong together
as one prompt-and-context slice, since where conventions live changes what the
roles should carry. C-7 and C-8 can wait. C-9 is its own slice and its timing is
Sam's.

## 7. What this audit did not cover

- **The estimation machinery** against kanbanzai's estimation research.
- **Context budget utilisation** (the 15–40% zone). Cromwell has no token
  estimation for assembled prompts, so there is nothing to measure against; that
  is itself a small finding, deliberately left out of §3 as speculative.
- **Whether the document templates** match the research's output-template
  guidance in detail. I read the manifests but did not audit section-by-section.
- **The MCP facet's tool descriptions** as an ACI. SWE-agent's finding — that
  interface design affects performance as much as model capability — applies to
  the nine MCP tools, and they were written for a chat agent that has to choose
  between them. Worth a pass of its own.
