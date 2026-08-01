# Response to the Subutai discussion document

**Date:** 2026-07-31
**Author:** Claude (Fable 5)
**Status:** Draft for Sam. **Not approved.**
**Responds to:** [Cromwell R2 Vision Subutai](../vision/Cromwell%20R2%20Vision%20Subutai.md)
**Grounded in:** [vision-v1](../vision/vision-v1.md), the
[alignment review](vision-alignment-review-2026-07-29.md), the
[research conformance audit](research-conformance-audit-2026-07-29.md),
[DEC-005](../decisions/DEC-005-the-orchestration-boundary.md), and
[SPEC-009](../specs/SPEC-009-the-authoring-chain.md) Stage 1 as built.

---

## 1. Overall position

The document's own framing is right, and it is worth saying so first: **this is
a clarification, not a redesign.** Almost everything in it is either already
built, already decided, or purely additive — UI surfaces, naming conventions,
two new entity plans (bugs, decisions), and observability. The engine — the
deterministic orchestrator, the gates, the document lifecycle, the dispatch
machinery — is untouched, and nothing in the document argues it should be
otherwise.

Three parts of the document are restatements of decisions already made, which
is a good sign — the vision is converging rather than churning:

- The chat/API division of labour is DEC-005's boundary in different words:
  chat agents plan and request; the orchestrator decides and dispatches.
- "Orchestration initiated from the UI so the AI agent does not attempt a
  quick fix" is DEC-005's surviving prohibition and the kanbanzai lesson,
  correctly stated.
- Human approval of documents by button is SPEC-009 Stage 1, code-complete as
  of yesterday.

What is genuinely new: the formal vocabulary (§3 below, which needs finishing),
chat as an *approval* surface (§2, the one real conflict), in-UI document
management and editing (§6), bugs and decisions (§7, §8), and richer progress
observability (§5).

One meta-observation before the detail. The document pauses development to
revise the design — but the revision it describes mostly *validates* what was
just built. Stage 1 of the authoring chain survives Subutai intact except for
one question, which is the next section. I would not let this pause grow into
a rewrite; the draft design document this discussion produces should be short,
and most of its sections should say "unchanged."

## 2. The one real collision: who presses go

Two wishes in the document, plus one thing already built, form a triangle that
cannot all hold:

1. *"A human should be able to approve a document by pressing a button in the
   UI **or telling an AI Chat agent** that a document is approved."*
2. *"Orchestration will be initiated by pressing a button in the UI to ensure
   that the AI Agent does not attempt to do a 'quick fix' or orchestrate the
   work themselves."*
3. **As built (SPEC-009 FR-4.3):** design approval *is* the orchestration
   trigger. Approving a design starts spec authoring, dev-plan authoring, and
   decomposition with no further human act. That was the point — gate 1 is one
   gate, not two.

Here is the sharpening that makes this a single question rather than two: **the
only document type a human approves is the gate-opening one.** Specs and
dev-plans are agent-approved (FR-2.1: only `design` routes to a human). So
"approve a document via chat" does not mean some general convenience — it means
precisely "open gate 1 from chat." Wishes 1 and 2 are the same question wearing
two hats, and they point in opposite directions.

Three ways to resolve it:

- **A — Approval is the button, UI-only (keep Stage 1's SD-3).** Chat gets
  everything up to the gate: submit, revise, summarise the design, draft the
  approval rationale, deep-link the human straight to the approve button. The
  click itself is the human's, at the command centre.
- **B — Chat approval allowed, and it triggers.** The MCP facet gains an
  approve tool (Stage 2 already anticipated this, with a DEC-005 amendment).
  The audit trail records "human, relayed via chat agent."
- **C — Decouple approve from start.** Approval can come from anywhere; a
  separate "begin" button starts the chain. This is wish 2 read literally.

**My recommendation is A, and the reason is kanbanzai.** The failure family the
whole architecture exists to prevent is an agent under "make progress" pressure
inventing a shortcut. A chat agent with an `approve_design` tool is one
hallucinated or over-eager call away from opening the gate and launching an
unattended run of authoring, review, and decomposition. The defence-by-omission
pattern DEC-005 praises — the tool does not exist, and a test fails if the
advertised set changes — is the strongest guarantee Cromwell has, and it is
exactly suited here. Meanwhile the cost of A is one click, on an act that is by
design rare and momentous (it is the moment of commitment). The document's own
wish 2 is arguing for A; wish 1 was, I think, written with routine documents in
mind, without noticing that no routine document has a human approval.

C is the fallback if chat approval matters more than I think it does — but note
it *adds a human act to gate 1*, which cuts against the two-gates intent
recorded in the alignment review §6a, and it re-creates the split ("approved
but not started") that invites state drift.

Whatever the ruling, it should be recorded as a decision (DEC-006 material),
because Stage 2's planned MCP approval tools hang on it.

### 2a. Sam's clarification (2026-07-31): orchestration begins at the spec, not the design

Sam's response to §2 and §9 resets the premise, and the Subutai document said it
plainly all along — "Product planning, design, **specification** → Chat-based;
Development Planning, review, implementation → API based" — I read it without
registering that it moves the seam. The clarified model:

- **The Chat AI writes the spec.** The design conversation culminates in the
  spec; there is no orchestrated step between design and spec. The reasoning is
  twofold and both halves are strong: the spec is the most important document
  in the system — the one everything downstream relies on — so it deserves the
  best thinkers (Fable/Opus in chat, affordable on subscription), not a
  cost-optimised API model; and the design process is already review-revise
  with a human, so by approval the human has reviewed the final design — an
  orchestrated design-reviewer interrupts a flow that has its own checking
  built in.
- **Orchestration occurs after the spec.** The pivot the vision always claimed
  — planning → *spec* → developing — becomes literally where the orchestrator
  starts. Stage 1 as built starts it one document earlier (design approval
  triggers spec authoring); that entry point moves.

**This dissolves §2's triangle rather than resolving it.** Design approval
stops being a mechanical trigger, so approving a design via chat becomes
harmless — it is state metadata, not a gate-opening act. Wish 1 and wish 2
coexist. The button that starts orchestration is the act of handing the spec
to development.

**Where exactly the button sits — recommendation.** Keep two gates total by
making the handover one act: **"Submit to development"** — simultaneously the
spec's handover and the orchestration start. From there the intake runs
unattended — validation, spec review, dev-plan authoring, dev-plan review,
estimation — and stops at gate 2 (start implementation), exactly as today. The
alternative (spec approval and a separate begin-planning button) adds a third
human act for no protection the intake review doesn't already give.

**One firm recommendation survives from the old §2 thinking, relocated: the
spec-reviewer becomes the border guard, and must not be removed.** The Subutai
document's orchestrated-roles list omits a spec reviewer, and with chat
authoring the spec the temptation is to drop it — the human was there, the
best model wrote it. But this is exactly where generator/evaluator separation
(P6) matters most: the chat agent *co-wrote* the spec and the human co-shaped
it; both share its blind spots. The first orchestrated act on submission
should be the existing spec review — fidelity to the design (the Stage 1
fidelity bar, FR-8, built for precisely this) and quality of the contract —
with `request_changes` flowing back to the chat/human side. That gives
development a formal way to refuse an ambiguous contract, which any contractor
should have. It is evaluation, not generation — cheap on an API model — and it
is the one check standing between the planning conversation and everything
built on it.

**Consequences for Stage 1 as built:**

- **Retire** the spec-authoring invariant and its four triggers, the
  `spec-author` role, and `write-spec` as an *orchestrated* purpose. The
  skill's content survives as a chat-side skill — the register it was
  originally written for in kanbanzai.
- **Keep** everything else, re-entered from the new seam: per-type approval
  authority (FR-2 — design approval stays human, it just stops triggering),
  spec-ready inheritance (FR-3), the dev-plan invariant and decomposition
  chain, the revision cascade, freshness, the fidelity bar, G0, the heartbeat.
  The bulk of Stage 1 is trigger rewiring plus removal, not rebuild.
- **Do not run the load-bearing Stage 1 smoke as designed.** It smokes the
  flow being removed ("approve a design, watch specs appear"). The pause came
  before the smoke — good timing, not wasted work. The revised smoke: submit a
  spec, watch dev-plan → tasks → estimates appear, stop at gate 2.
- **Stage 2's MCP submit tools move onto the critical path**, since chat
  submitting the spec is now the workflow's central act. DEC-005 already
  permits it — submitting a document for review is "asking the orchestrator to
  do something it would do anyway."
- Parallel spec fan-out across sibling features moves to the planning side and
  becomes human-paced. Consistent with the sequential-penalty research (one
  author, one document, one pass) and with planning being "slow and messy by
  nature."

This should be recorded as the decision it is — it revises SPEC-009's premise,
not just a tool list. DEC-006 material: *the orchestration entry point is the
submitted spec; specs are chat-authored; the spec review is the intake gate.*

### 2b. Sam's second clarification (2026-07-31): pre-completing intake stages from chat

Sam's ask: for complicated features, the chat agent (human supervising) should
be able to *reach into* the spec-review and dev-plan stages and **pre-complete
them ad-hoc**, so that when the orchestration button is pressed the
orchestrator sees those steps already done. Alongside it, the human should be
able to raise the model quality of the orchestrated intake per feature —
a higher-thinking dev-planner, a different reviewer.

**The important observation: this needs almost no new architecture, because it
decomposes into three mechanisms, two of which DEC-005 already grants.** The
phrase "reach into the stages" sounds like a boundary change; taken apart, it
is not.

1. **Pre-authoring the dev-plan — already granted, already how the invariants
   work.** Document authoring is planning-side work the facet has always been
   allowed (DEC-004/005). And Stage 1's invariants are *absence-triggered* —
   they dispatch an author only when the artifact is missing. A dev-plan
   written in chat and submitted suppresses the orchestrated `write-dev-plan`
   with no code change to speak of. The chat agent writing a dev-plan for a
   complicated feature is not an exception path; it is the same path the spec
   now takes (§2a), extended one document further when wanted.

2. **Pre-running the review — already granted in principle.** DEC-005's rule:
   the facet may "ask the orchestrator to do something the orchestrator would
   do anyway." Submitting a document for review *is* that. The chat agent
   submits the spec (or dev-plan) early; the orchestrator dispatches the same
   independent reviewer it would have dispatched at button time; the verdict
   lands on the document. Nothing about intake review is tied to the button —
   the button's job is starting *development*, and a document already
   `approved` at button time is simply a gate already satisfied.

3. **The human verdict — the one genuinely new piece, and it is small.** When
   the human and chat have done deep work on a complicated feature and the
   human is confident, they should be able to approve the spec or dev-plan
   *directly*, as a human verdict in the UI, skipping the agent review. This
   is an extension of FR-2's per-type approval authority: today `design` →
   human and `spec`/`dev-plan` → agent; the change is that **human authority
   suffices for any document type** — a human verdict is never wrong to
   accept. The chat agent's critique becomes advice the human reads before
   clicking; the *verdict* is the human's. What must not exist is a chat-agent
   verdict tool: the co-author approving its own contract is the rubber stamp
   in its purest form, and it stays impossible by omission, exactly as DEC-005
   left it.

Then "the orchestrator sees steps already complete" costs nothing, because it
is what vision §9 already built: **gates are expressions over document state,
with no separate stage tracker.** A feature whose spec and dev-plan are both
`approved` at button time flows through intake with nothing to do and stops at
gate 2. This proposal is, quietly, strong validation of that design choice —
a stage-binding system would have made pre-completion a nightmare of state
synchronisation; state-expression gates make it free.

**The model-quality knob.** Per-feature (or per-run) role→model overrides,
chosen at the button. The natural UI: the "Submit to development" action shows
the intake steps it is about to run — each with its role, model, and
already-satisfied status — and lets the human bump a model before launch.
That one screen delivers the whole spectrum Sam is describing: routine
features take the configured defaults untouched; complicated ones arrive with
chat-authored artifacts, chat-advised human verdicts, or upgraded reviewer
models, in any mix.

**One honesty requirement.** Attribution must survive the mix: the audit trail
and document page should show *who* produced each artifact and verdict —
orchestrated agent (role, model), chat-authored, or human verdict — so a
retrospective can ask "do human-verdict specs bounce more often at dev-plan
review?" and get an answer. Pre-completion without attribution would blur the
one record the system promises to keep honest.

DEC-006 grows a clause: *the planning side may pre-produce any intake artifact
and pre-request any intake review; verdicts come only from an independent
agent reviewer or from a human; per-run model overrides are chosen at the
button.*

### 2c. Sam's third clarification (2026-07-31): the chat agent may complete any stage — except judgement

Sam's ask, extending §2b past the intake: could the chat agent complete *any*
stage ad-hoc — including implementation — using MCP tools to coordinate and
keep state correct? The motivating case is the small, tricky feature where the
work is better done by an expensive model in conversation with a human. The
motivating *insight* is about temptation: if the sanctioned pipeline feels
heavyweight for exactly those features, the human will eventually open a chat
session and say "just do it" — and that work then happens entirely outside the
system. No state, no review, no audit, no record. Better to give the fancy-AI
path a lane inside the workflow than to pretend prohibition will hold. DoD
must never be skippable.

**This collides with DEC-005's first prohibition head-on — and the corrected
kanbanzai diagnosis is on Sam's side.** DEC-005 forbids the facet to
"implement, review, verify or otherwise perform pipeline work itself," calling
that the kanbanzai failure. But the diagnosis this project itself corrected
(alignment review §2, DEC-005's own context section) says the failure was
never "an AI did the work" — it was **an AI holding the orchestration role and
dropping it**: state tracking lived in the same drifting context as the work,
so when the context saturated, the loop was abandoned and the state lied. In
Subutai the state machine is code. It does not live in anyone's context. A
chat agent that claims a task, works, and submits has not taken over
orchestration — the gates still fire, G2 still refuses a feature with
non-terminal tasks, the heartbeat still notices stalls. The prohibition on
*doing work* was a proxy for the real rule, and with state tools the real rule
can be enforced directly:

> **Who does the work is flexible; what judges the work is not.**
> Any executor may complete any stage's *work*. No executor may judge its own
> work, and no path skips a gate. Doing is open; judging is closed.

**The generalisation worth naming: executors, not exceptions.** Once
implementation can be done by a dispatched agent *or* a chat agent under human
supervision, the honest model is that a task has an **executor** — dispatched
agent (role, model), chat agent, or plain human, because a human implementing
a task themselves is the same case and Cromwell never accommodated it either.
The pipeline becomes executor-agnostic and gate-strict. That is also the
"slack in the workflow" Sam asks for: not a loosened rule, but a first-class
seat.

**The non-negotiables, concretely:**

- **No verdicts from the chat agent, ever** — §2b's rule, now load-bearing at
  every stage. The chat implements a task; the orchestrated code review still
  runs on the diff. The chat pre-authored the spec; the intake review still
  runs. A verdict tool never enters the MCP facet.
- **DoD is structurally unskippable, and already is.** Sam wonders whether DoD
  "might always have to be a non-skippable step" — it already is the one thing
  no actor can skip: G2/G3 are code-evaluated gates, there are no force
  flags, and every override routes through an answered checkpoint. The rule to
  *add* is that `verify-feature` is never chat-executable and its per-criterion
  evidence contract holds regardless of who implemented (C-2 finishes this).
  Keep verification dispatched, always.
- **State discipline through the tools:** `claim_task` → work → `submit_task`
  is the only path. Claiming returns the worktree path and the contract;
  submitting is the only exit; the state machine rejects out-of-order calls as
  it does for everyone. Per push-not-pull and the ACI finding, the rules
  should live *in the tool descriptions and results* — the one surface the
  agent reliably reads — backed by a chat-side `work-a-task` skill.
- **The claim is visible and expires.** Executor badge in the UI; the existing
  heartbeat extends to chat claims — a claimed task with no submission and no
  activity raises a "still working on this?" checkpoint. The actual kanbanzai
  failure (abandonment) gets the same net for chat executors as for dispatched
  ones.

**The honest costs, named rather than waved at:**

- **The token ledger gets holes.** Chat work burns subscription tokens the
  ledger cannot see. Record executor and mark actuals as unmeasured; the
  calibration corpus must flag these rather than treat absence as zero, or
  estimation quietly degrades. This is a real, permanent cost of the feature.
- **Tool profiles cannot be enforced on the chat agent.** A dispatched
  implementer physically has no tools beyond its profile; a chat agent in an
  editor has the filesystem. Nothing prevents it editing outside the claim, in
  the main tree, or without claiming at all. The mitigations are the skill,
  the tools' own guidance, the human sitting in the conversation — and one
  mechanical net worth building: the git watcher already sees document
  changes; extend it to flag commits touching an active feature's paths with
  no claimed task, as a checkpoint. Residual risk remains and is accepted —
  today the same risk exists with *no* sanctioned path and *no* net.
- **One mind through the whole chain.** A feature whose spec, dev-plan and
  implementation are all chat-produced has had a single intelligence (plus the
  human) end to end; the independent reviews become the entire defence. The
  §2b model knob is the counterweight — it is reasonable to default reviewer
  models *up* when the executor is chat, since review is then carrying more of
  the system's integrity. Config, not architecture.

**Decision impact.** This supersedes DEC-005's first prohibition in part, two
days after acceptance — which is fine, and the supersession mechanism exists
precisely for this, but it should be done consciously as its own decision
(DEC-006 or DEC-007): *the boundary is judgement and state authority, not
labour. The facet may perform any stage's work under claim/submit discipline;
it may never hold a verdict, skip a gate, or write state except through the
tools.* The kanbanzai lesson survives fully intact — indeed this is its
cleanest statement yet: the failure was never who did the work; it was that
the state and the work shared one fallible context. They no longer do.

## 3. The vocabulary: finish the formal definitions

The Formal Definitions section is the most valuable part of the document and
also the least finished. Specific problems:

**Roadmap is defined twice, differently.** The Workflow section says a roadmap
is "an ordered list of *milestones*"; Formal Definitions says "an ordered
collection of **deliverables**." Pick one. I recommend **ordered milestones**
(vision-v1's definition): a roadmap answers "what ships in what order," and
milestones are the shipping unit. Letting roadmaps hold arbitrary deliverables
makes a roadmap a milestone-with-order, which blurs the one distinction between
the two.

**Feature is missing entirely.** It appears in the introduction ("familiar
development concepts: development plan, feature, task") and then never again —
not in the deliverable union, not in the formal definitions. Vision-v1 lets
milestones reference Features directly. Either:

- features are deliverables (add them to the union), or
- features are reached only transitively through their parent initiative.

The phrase "creating a new initiative (or sub-initiative, or feature etc.)" and
"two documents per piece of work (initiative or feature)" hint at a third
possibility — that Subutai is quietly collapsing Feature into Initiative, a
feature being merely a leaf initiative. **I recommend against the collapse.**
The stateless-initiative / full-lifecycle-feature split is one of Cromwell's
best calls: above the feature line things are exploratory and free; at it, the
contract begins. A feature carries a lifecycle, tasks, a worktree, a branch —
an initiative deliberately carries none of that. Merging them puts lifecycle
state back into the exploratory layer, which is where kanbanzai's vocabulary
went wrong the first time. Keep Feature, add it to the deliverable union, and
give the development half (Feature, Task, Spec, Dev-plan, Bug) the same formal
treatment the planning half now has.

**"Two documents per piece of work (initiative or feature)" needs one
qualifier.** A design can live at any level; a spec only has meaning at the
feature level, because a spec's consumer is the development pipeline and only
features are buildable. As built (FR-3.1), a feature is spec-ready off its own
design *or its immediate parent initiative's* — that inheritance is worth
keeping and worth stating in the vocabulary. The sentence should read: a design
per initiative or feature; a spec per feature.

**Project as "the top-level initiative"** is fine and costs nothing — it is a
name for a row that already exists, not a new entity.

## 4. The chat/API economic split: true today, so keep it as config

The observation is correct and worth recording: high-thinking models are
affordable on time-boxed plans and punishing through the API, so the
human-chat/orchestrated-API division happens to align with a payment division.
Two cautions:

**Do not promote it from observation to architecture.** The load-bearing
division is *determinism* — ad-hoc design work versus rule-governed development
work — which the document itself states. The payment alignment is incidental
and will not necessarily survive pricing changes. The right home for it is what
already exists: per-role model and provider config. Nothing new to build.

**One avenue worth investigating, since cost is the driver:** a pluggable
dispatch backend. Today dispatch calls provider APIs directly. A backend that
drives a headless CLI session (`claude -p` style) could, if the plan's terms
permit it, run some orchestrated work under a subscription rather than the API
meter. I flag it as *worth checking*, not as a recommendation — the terms
question is real and Sam should verify it — but the dispatch layer costs little
to keep pluggable, and it also keeps the door open for local models.

**Sam (2026-07-31): wanted as a config-option investigation**, with the first
candidates being the spec reviewer and possibly the dev-plan author — the two
orchestrated roles where thinking quality matters most.

**Closed same day.** Anthropic's billing change (effective 2026-06-15) moved
`claude -p` and Agent SDK usage to a separate "Agent SDK credit" pool, no
longer covered by interactive Pro/Max subscription quotas. The avenue's whole
point was riding the subscription; it is dead. The dispatch layer staying
pluggable remains cheap and keeps the local-model door open, but no
subscription-backed backend exists to plug in. The want behind it — the best
thinkers on the intake stages without API pricing — is instead answered by
pre-completion (§2b), which brings those stages into the chat session where
the subscription models already live.

This also connects to the tokens-not-money direction already in flight: the
human surface shows tokens; where the tokens are bought is config.

## 5. Observability: both asks are cheap, because the data already flows through the server

The document asks two questions; both have good answers.

**"Would it be possible to keep a complete log of the whole conversation with
the agent?"** Yes, and cheaply — because of an architectural fact: every
dispatched agent's every message and tool call already passes through the
Cromwell server. The server assembles the prompt, hosts the tools, and receives
the responses. Persisting the full transcript (assembled prompt, each turn,
each tool call and result, final outcome) against the existing dispatch row is
a write path plus a viewer, not a new subsystem. This is the single highest
value-per-effort item in the document: it answers "what actually happened," it
is the debugging surface for the estimate-versus-reality question, and it feeds
the retrospective mechanism (C-9) when that arrives. Size/retention is config.

**"A summary of major milestones in the development process."** The audit log
already records every transition; what is missing is a *read model* — a phase
timeline per feature ("Spec authoring → Spec review → Spec approved →
Implementing (3/7 tasks) → Verifying") derived by filtering the audit stream to
major transitions, with each entry linking down to its dispatches and their
transcripts. Two levels: timeline for clarity, transcript for depth. Noise
disappears because the noise stays in the detail level.

These two belong in one slice, and C-7 (review metrics — approval rates,
findings per review, time to verdict) is the same shape of work over the same
tables. Bundle them.

## 6. Documents: creation, names, folders, editing

### Creation with the entity — yes

Creating an initiative or feature should create its placeholder design (or
spec) from the template, in the standard place, already attached. The authoring
chain does exactly this server-side for specs and dev-plans, so the mechanism
exists; this extends it to human-initiated creation in the UI and chat. The
path-in-a-text-box flow stays as the escape hatch for pre-existing files, not
the main road.

### IDs — the server is the minting authority, which dissolves the clash problem

Kanbanzai's ID iterations were fighting a distributed problem: multiple actors
inventing names. Subutai does not have that problem — **every creation path
(UI, MCP, orchestrator) already goes through one server backed by one
database.** Mint IDs from Postgres sequences and the clash problem is gone by
construction.

Suggested scheme, matching what the project already does by hand:

- Entities: `INIT-014`, `FEAT-023`, `BUG-007`, `DEC-006` — short prefix,
  monotonic number, no meaning encoded in the number.
- Documents: `<entity-id>-<type>[-<slug>].md`, e.g.
  `FEAT-023-spec.md`, `INIT-014-design-auth.md`. Related documents share the
  entity prefix, which gives the visual grouping the document asks for, for
  free, in any file listing.
- **The stable key lives in frontmatter, not the filename.** Write the
  document's ID into its frontmatter at creation. Then a rename or move is
  reconciled by the watcher against the ID, not the path, and humans can
  reorganise without breaking attachment. The path is a location; the ID is the
  identity.

### Folders — group by initiative, as kanbanzai learned

Agreed with the document's lean: per-initiative folders with human-readable
names, e.g. `docs/work/INIT-014-auth/`, feature documents inside their parent
initiative's folder (a subfolder per feature only if volume demands it — start
flat, stay shallow). Type-prefixed filenames inside an initiative-grouped
folder give both groupings at once: the folder answers "what belongs to this
work," the prefix answers "what kind of thing is this."

### In-UI editing — feasible; the complications are real but all small

The vision-v1 line "the web UI is not an editor" was a scoping call, not a
principle, and it is right to revisit it. Direct answers to the questions
asked:

- **How would it work?** Save writes the Markdown file to disk in the main
  working tree. The existing watcher/indexing path picks it up like any other
  edit. The editor is a textarea (or minimal CodeMirror) with preview — resist
  building a rich editor; the files must stay pleasant to edit in vim.
- **How does it interact with Git?** Recommend: **Save writes the file; "Save &
  commit" commits that file** with a structured message and the human as
  author. Plain save leaves the tree dirty for the human's own commit
  discipline. Auto-commit-on-every-save is possible but produces noise history;
  offering both buttons costs nothing.
- **Concurrent edits** (UI versus editor-on-disk): optimistic locking. The UI
  holds the content hash it loaded; save compares against the file's current
  hash and refuses with a diff view if it moved. For a small team this fires
  rarely; when it fires it must not silently clobber.
- **The complication you may be missing: state and worktrees.** Editing a
  document that is `reviewing`, or whose feature has in-flight dispatches,
  intersects the lifecycle — the revision-in-flight checkpoint and the Stage 1
  freshness machinery already govern this; the editor just needs to *display*
  document state and warn before editing a non-draft document. And agents work
  in per-feature worktrees: a mid-flight edit in the main tree is not visible
  to an agent's worktree until the freshness machinery reacts. None of this is
  new risk created by the editor — it exists today for on-disk edits — but the
  editor makes it easier to do casually, so the UI should surface state where
  vim never would.

CRUD, approve, and the rest of the "normal web workflow UI" functions follow
from the above plus what SPEC-006/007 already built. Nothing structural blocks
it.

## 7. Bugs: a plan

The kanbanzai consensus (feature-shaped, attached to the originating
initiative) is right. Concretely:

- **Entity:** `Bug` — a feature with a triage prefix on its lifecycle:
  `reported → accepted → ready → active → review → done`, plus terminal
  `rejected` / `duplicate`. From `accepted` onward it *is* the feature
  pipeline; no parallel machinery.
- **One pipeline, not a variant.** Rather than inventing a lightweight bug
  pipeline, let the bug report document serve as the spec (the template
  differs: reproduction, expected/actual, acceptance criterion "no longer
  reproduces"). Small fixes get small dev-plans; the machinery is unchanged.
- **Agents file reports without a human** — a `report_bug` tool available to
  reviewers and verifiers for out-of-scope findings. This also closes a loop
  the conformance audit left open: **C-1's dropped minor findings become bug
  reports in `reported`** instead of rows in an audit table nobody reads.
  Recorded-not-discarded, with a lifecycle.
- **Humans gate two points**, exactly as the document asks: triage
  (`reported → accepted` is a human act — it is scope commitment, the same
  seam as feature creation) and the existing gate 2 (starting the fix).
  Everything between is the normal agent pipeline.
- Humans report via chat (the PM agent creates the entity through MCP) or the
  UI form; both routes end at the same server call.

## 8. Decisions: a plan

Smaller than it looks, because vision-v1 already made the call: a decision is a
document, not an entity. What Subutai adds:

- **Type and ID:** document type `decision`, server-minted `DEC-nnn`, so the
  IDs this project currently manages by hand become managed. Lifecycle maps
  onto the existing document lifecycle; `approved` reads as *accepted*, and
  supersession is already a state. Accepted decisions are never edited — they
  are superseded, which is this project's own practice (DEC-005 superseding
  DEC-004 is the worked example).
- **Attachment:** to the project or to an initiative. Project-level for
  architecture-wide rulings; initiative-level for local ones.
- **Their workflow role — this is the important part:** decisions are the
  answer to the document's own question, and the answer comes from the
  push-not-pull lesson. A decision no agent reads is agentic memory in name
  only; kanbanzai proved agents will not fetch. So: **approved decisions on the
  ancestor chain are auto-surfaced into dispatch prompts** — which is exactly
  the C-6 machinery (the conventions document, extended document surfacing on
  the execution path). Decisions and conventions are the same mechanism with
  different types. The human-facing viewer is just a filtered document list.
- Hard size discipline applies (C-6's cap): surfaced decisions spend prompt
  budget on every dispatch, so the *decision* section that gets surfaced should
  be the ruling and its one-line reason, not the full alternatives-considered
  prose.

## 9. Keep the design-reviewer, as advisory

**Withdrawn (2026-07-31) — see §2a.** Sam's clarification answers both halves
of the argument below: the design process is itself review-revise with the
human, so the final version has had its human review by the time it is
approved; and an orchestrated reviewer interrupts the chat flow that now
carries through to the spec. The generator/evaluator concern (P6) is real but
belongs at the *spec* boundary, where §2a relocates it — the spec-reviewer as
border guard. The design-reviewer role and `review-design` skill should be
removed from the orchestrated set. If a cold read of a design is ever wanted,
`review-design`'s content works as an on-demand chat-side skill the human can
invoke — available, never interposed. The original argument is left below for
the record.

The document suggests removing the design-reviewer role. Stage 1 shipped one,
and I would keep it — not as a gate, but as what it already is: an advisory
pass whose output lands in front of the human *before* they approve. The
rationale for removal (design is already co-authored with a capable chat agent)
runs into P6 from the research: **the co-author shares the author's biases** —
it helped write the thing, so it is the wrong evaluator of it. A cheap
Sonnet-class read by an agent that was not in the room, whose anti-patterns are
already tuned to exactly the right failures ("Silent Assumption",
"Reason-free Decision", and pointedly *not* rewriting the design), costs
pennies and runs before the one approval in the system that opens the
unattended gate. The role's existing identity even encodes the humility the
document wants: the human approves; the reviewer only reports.

If it proves noisy in practice, demote or drop it then — with smoke-test
evidence rather than in advance of it.

## 10. What the revision must not drop

A pause to revise is the moment things fall silently off the table. For the
record, still live and still wanted:

- **The two Stage 1 live smokes** — still the only thing between SPEC-009
  Stage 1 and "done," and unaffected by anything in the Subutai document.
- **The C-backlog** from the conformance audit, particularly: C-3/C-4
  vocabulary and anti-patterns for the *original six* roles (the new authoring
  roles already have them — the old roles are now the inconsistency the
  document itself notices in "Skills and Roles"); **C-6a, the implementer's
  search tool, still missing** (checked today: `implementer.yaml` remains
  `[read_file, list_files, edit_file, write_file, run_command]`); C-6
  conventions surfacing, which §8 above folds decisions into; C-1a (severity on
  document comments); C-9 retrospectives.
- **The rename itself is cheap; do it once, at a boundary.** New documents say
  Subutai now; the code/module/binary rename should ride one deliberate commit
  (probably with Stage 2) rather than dribbling through the tree.

## 11. Suggested shape of the draft design document

The discussion document's goal is a draft design. Suggested structure, in
which most sections are short because they record "unchanged":

1. **Unchanged core** (one page): orchestrator-as-code, gates, document
   lifecycle, dispatch, DEC-001..005 carried forward. Subutai is Cromwell R2.
2. **Vocabulary** — the finished formal definitions, both halves (§3 above).
3. **DEC-006: the approval surface** — the §2 ruling, whichever way it goes.
4. **Document management** — IDs, frontmatter keys, per-initiative folders,
   create-with-entity, the editor (§6).
5. **Observability** — transcripts and the phase timeline (§5).
6. **Bugs** (§7) and **Decisions** (§8).
7. **Out of scope, explicitly:** knowledge subsystems (run twice, failed
   twice), multi-project, rich editing, ACP.

Sequencing instinct: the §2 decision first (it gates Stage 2's MCP tools and
is pure ruling, no code); then the Stage 1 smokes *before* any Subutai code,
because they validate the chain everything above sits on; then document
management and observability as the first Subutai slices — they are the ones
the manual smoke testing actually asked for.
