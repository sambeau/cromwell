# The evidence base for agent systems

This is the reference for *why* Cromwell's agents are built the way they are. It gathers
the findings from roughly twenty academic papers and industry engineering reports into one
place, states how strong the evidence is for each, and records what each finding implies
for a system like this one.

Read it when you're about to change how an agent is prompted, how work is decomposed, how
many agents run at once, or where a gate sits. The companion documents apply these
findings: [roles-skills-and-prompts.md](roles-skills-and-prompts.md) covers how to write
the prompts, [orchestration-patterns.md](orchestration-patterns.md) covers how to arrange
the agents, and [definition-of-done-and-ready.md](definition-of-done-and-ready.md) covers
the gates.

Every claim here carries a source. Where the evidence is weak, that's stated. Nothing in
this document is a rule — the rules live in the code and the starter pack. This is the
reasoning behind them.

---

## Table of contents

1. [How a model reads a prompt](#1-how-a-model-reads-a-prompt)
2. [How agent systems succeed](#2-how-agent-systems-succeed)
3. [How agent systems fail](#3-how-agent-systems-fail)
4. [What agents actually retrieve](#4-what-agents-actually-retrieve)
5. [Where Cromwell stands against the evidence](#5-where-cromwell-stands-against-the-evidence)
6. [Sources](#6-sources)

---

## 1. How a model reads a prompt

The first cluster of findings is about the model itself: what it attends to, what routes it
to the right knowledge, and how much you can put in front of it before quality falls off.
These findings are architectural. You can't prompt your way around them.

### 1.1 Attention is U-shaped, so position carries weight

Transformers don't attend evenly across their context. Liu et al. ("Lost in the Middle",
2024) measured a 30%+ accuracy drop when the critical information sat in the middle of the
window rather than at either end. Wu et al. (MIT, 2025) traced the cause to causal masking
and rotary positional encoding — it's a property of the architecture, not a quirk of one
model family.

A second effect compounds it. The last item in a sequence has outsized influence on what
the model generates. If a section contains three examples, the third one shapes the output
most.

**What follows from this.** Order content by attention value, not by narrative
convenience:

| Position | Attention | What belongs here |
|----------|-----------|-------------------|
| Top | High | Identity, vocabulary, hard constraints |
| Near top | High | Named anti-patterns |
| Middle | Low | Procedure — numbered steps survive the dip; prose doesn't |
| Near bottom | Rising | Output format, examples |
| Bottom | High | The artefact under review, retrieval anchors |

Numbered imperative steps go in the middle precisely because their structure carries signal
that prose would lose. Within any section, put the most important item last.

One clarification worth recording, because it has been raised and settled: attention
ordering and prompt caching want the *same* order. Stable content first, the volatile
artefact last. There is no conflict between them.

**Evidence strength: strong.** Two independent peer-reviewed studies, plus Anthropic's
"Effective Context Engineering" (2025) reporting the same pattern from production systems.

### 1.2 Vocabulary is the routing signal

This is the single highest-return finding in the corpus. Ranjan et al. ("One Word Is Not
Enough", 2024) showed that specific domain terminology acts as a routing signal determining
which regions of the model's knowledge it reaches for. This isn't a metaphor for
"clearer wording" — it's a measurable difference in what the model produces.

"Review the security" routes to blog-post-level advice. "Perform an OWASP Top 10 audit,
apply STRIDE threat modelling to every trust boundary, check for CWE-89 and CWE-352"
routes to security engineering.

The working test, derived from the PRISM persona framework (2024), is the **fifteen-year
practitioner test**: would a senior expert with fifteen years in the domain use this exact
term when talking to a peer? If yes, it belongs in the vocabulary payload. If it sounds
like a job advert, cut it.

The same research found that **brief identities beat elaborate ones**. Persona descriptions
under fifty tokens produce better output than personas over a hundred. Flattery actively
degrades output, because superlatives ("world-class expert", "you excel at") activate
motivational and marketing text from the training distribution. Competence is declared
through vocabulary and named failure modes, not adjectives.

**What follows from this.** Every role carries fifteen to thirty precise domain terms.
Every skill carries its own task-specific terms on top. These are not definitions for a
human reader — they're priming for the model. Strip general terms it already knows.

**Evidence strength: strong.** Peer-reviewed (Ranjan et al.) plus a validated framework
(PRISM).

### 1.3 Context is scarce, and irrelevant tokens are not free

Anthropic's context engineering research (2025) established the **attention budget**: every
token competes with every other token for attention weight. Irrelevant tokens don't sit
harmlessly to one side; they degrade performance on the relevant ones.

The measured sweet spot is **15–40% context window utilisation**. Below roughly 10%, the
model is under-grounded and hallucination risk rises. Above roughly 60%, attention dilution
dominates and quality falls.

Stale context is worse than absent context, because it actively misdirects. A convention
that was true six months ago and is quietly wrong now will be followed.

**What follows from this.** Assemble the minimum a dispatched agent needs to start, and
measure it. If a prompt routinely exceeds 40% of the window, the work unit is too large,
not the prompt too small.

**Evidence strength: strong.** Anthropic's guidance is derived from production systems and
the utilisation range is consistent across several independent sources.

### 1.4 Examples teach better than rules

LangChain's few-shot research (2024) found that **three well-chosen examples match nine** in
effectiveness — quality and relevance matter far more than quantity. Anthropic's
consistency guidance puts it directly: input/output examples train understanding better
than abstract instructions.

Format is not neutral either. Voyce's XML/Markdown comparative study (2025) attributed up
to **40% of performance variance to prompt format alone**.

**What follows from this.** Every skill earns at least one BAD/GOOD example pair, with a
sentence explaining *why* each is bad or good. Put the best GOOD example last. Use
concrete, realistic content — never `foo`, never "example placeholder here".

**Evidence strength: moderate to strong.** Industry-sourced and widely replicated, but not
peer-reviewed.

### 1.5 More rules make compliance worse past a threshold

Vaarta Analytics (2026) measured what they call the n=19 cliff: at nineteen or more
requirements in a prompt, accuracy drops *below* what the same prompt achieves with five.
The instinct to fix a problem by adding one more rule usually makes things worse.

**What follows from this.** Five to fifteen precise requirements, not nineteen fuzzy ones.
When a prompt isn't working, try removing content before adding it. Read the transcripts —
the problem is often not what you assume.

**Evidence strength: moderate.** Industry research, not peer-reviewed, but consistent with
the "less is more" pattern across PRISM, the attention budget work, and the multi-agent
scaling studies.

### 1.6 Explain why, not just what

Zamfirescu-Pereira et al. (CHI 2023, "Why Johnny Can't Prompt") found that a positive
instruction combined with a negative constraint is stronger than either alone. The
mechanism that makes the combination work is the reason attached to it.

"Always X" covers the case you wrote down. "Do X because Y" generalises to the adjacent
cases you didn't. A rule without its reason also can't be pruned later, because nobody
remembers why it was added.

Anthropic's own skill-creator guidance says the same thing from the other direction:

> Try to explain to the model why things are important in lieu of heavy-handed musty MUSTs.

If you find yourself writing ALWAYS or NEVER in capitals, that's a signal to reframe and
explain the consequence instead.

**What follows from this.** Cromwell's `AntiPattern` struct already encodes this: `name`,
`detect`, `because`, `resolve`. The `because` field is the load-bearing one. It must
explain the consequence chain, not restate the detection signal.

**Evidence strength: strong** for the CHI 2023 finding. The claim that *naming* an
anti-pattern additionally activates expert knowledge is inferred from the vocabulary
routing work — plausible, not directly tested.

---

## 2. How agent systems succeed

The second cluster is about arranging multiple agents and the interfaces between them.

### 2.1 Enforceable constraints beat advisory instructions

Every source that compares "telling agents what to do" with "preventing agents from doing
the wrong thing" finds the second wins, decisively.

- **MetaGPT** (Hong et al., ICLR 2024) encoded standard operating procedures as prompt
  sequences with verification gates between stages. Agents *must* produce a structured
  artefact that passes verification before the next stage begins. This reduced cascading
  hallucination errors by roughly 40% against free dialogue.
- **Microsoft's orchestration patterns** (2026) use programmatic checks on intermediate
  steps: the choice of next agent is deterministic, not left to agent discretion.
- **Masters et al.** (DAI 2025) formalise the distinction between hard constraints (ℋ —
  violation terminates the workflow) and soft constraints (𝒮 — violation incurs a penalty).
  The most reliable workflows put hard constraints on critical ordering and soft
  constraints on quality preferences.
- **Google Research** (Kim & Liu, 2026) found that centralised orchestration contained
  error amplification to 4.4×, where independent agents working in parallel without a
  coordinator amplified errors 17.2×. The orchestrator acts as a validation bottleneck.

The implication is stark. If a lifecycle gate is enforced by instruction — "you should
complete the specification before implementation" — agents will skip it when the task seems
simple or the context gets long. If it's enforced by the tool refusing to operate,
compliance is guaranteed.

This is the research's top recommendation, and it's the thing Cromwell got right first: the
orchestrator is plain Go in `internal/rules/` with no model attached, and the gates in
`internal/lifecycle/gates.go` are pure functions over current state. Code cannot drift or
forget.

**Evidence strength: strong.** Four independent sources, three peer-reviewed, converging.

### 2.2 Decomposition quality is the critical path

Several independent sources reach the same conclusion: how well a goal is broken into
structured sub-tasks predicts overall success better than any other factor.

Masters et al. put it most precisely:

> Performance gains correlate almost linearly with the quality of the induced task graph —
> underlining that structure learning, not raw language generation, is the critical path.

Anthropic's multi-agent report describes the failure mode from the other side: without
detailed task descriptions, subagents "duplicate work, leave gaps, or fail to find
necessary information". Google's predictive model found decomposability was its strongest
single feature.

**What follows from this.** Invest disproportionately in the dev-plan stage. A weak
decomposition produces weak code no matter how good the specification was — which is
exactly what Cromwell's `dev-plan-reviewer` role already tells the model.

Concretely, a decomposition is worth checking for: does every task have a clear
description? Are dependencies declared? Is each task small enough for one agent working
alone? Are there gaps — a missing test task, a missing integration task?

**Evidence strength: strong.** Two independent academic sources.

### 2.3 Architecture must match task structure

This is the most counter-intuitive finding in the corpus, and the one that most constrains
Cromwell's design.

Google Research (Kim & Liu, 2026) evaluated 180 agent configurations across five
architectures and four benchmarks. The results split sharply by task type:

- **Parallelisable tasks** — independent analyses, multi-file code changes: centralised
  multi-agent coordination improved performance by **80.9%** over a single agent.
- **Sequential reasoning tasks** — planning, specification, design: **every** multi-agent
  variant tested *degraded* performance by **39–70%**. Communication overhead fragmented the
  reasoning and consumed the cognitive budget the task needed.
- **Tool-heavy tasks** — sixteen or more tools: coordination overhead grows
  disproportionately with tool count.

Their predictive model identifies the right architecture for 87% of unseen tasks using just
two properties: sequential dependency and tool density.

**What follows from this, and it's the rule that shapes any future authoring design.**
Parallelism must fan out **across independent features**, never *within* the writing of one
document. Writing a specification, a design, or a dev-plan stays one agent making one
sequential pass. Implementing the tasks from one approved dev-plan is genuinely
parallelisable and gains around 81% — which Cromwell already does.

**Evidence strength: strong.** The largest quantitative study in the corpus.

### 2.4 Interface design matters as much as model capability

SWE-agent (Yang et al., 2024) coined the term **agent-computer interface** and demonstrated
the core claim: purpose-built interfaces for agents dramatically outperformed giving the
same agents raw shell access to the same functionality. Not a better model — better tools.

Anthropic reports the same from two directions. Their SWE-bench team "spent more time
optimizing our tools than the overall prompt". Their multi-agent research team built an
agent that used each tool dozens of times and rewrote the descriptions based on the
failures, achieving a **40% decrease in task completion time**.

The design principles that fall out of this:

1. **Put yourself in the model's shoes.** If the description and parameters aren't obvious,
   the tool will be misused.
2. **Keep formats close to training data.** Agents have seen humans use command lines, write
   Markdown, and edit files. Tools that match those patterns beat novel formats.
3. **Eliminate formatting overhead.** Don't make agents maintain accurate counts, compute
   diffs, or produce valid JSON for complex nested structures. That work consumes reasoning
   capacity.
4. **Make wrong usage hard.** Constrain parameter spaces, provide defaults, reject invalid
   combinations at the tool level rather than trusting the agent to avoid them.
5. **Give each tool one distinct purpose.** Overlapping tools cause wrong selection. If two
   tools are easily confused, merge them or make the distinction unmistakable.

There's a corollary that matters for Cromwell specifically. **Agents fall back to the
interface they already know.** When a purpose-built tool is unfamiliar and searching files
is familiar, they search files. A role whose tool profile contains no search tool at all
has had its most reliable fallback removed.

**Evidence strength: strong.** Peer-reviewed plus two independent industry reports with
measured outcomes.

### 2.5 Agents can't judge effort, so tell them

Anthropic's multi-agent research found this plainly:

> Agents struggle to judge appropriate effort for different tasks, so we embedded scaling
> rules in the prompts.

Their rules were explicit: simple tasks get one agent and three to ten tool calls; complex
tasks get ten or more subagents with clearly divided responsibilities.

The reason this matters is motivational rather than cognitive. Writing code *feels*
productive because it produces something that runs. Writing a specification produces a
document that doesn't do anything. Without an explicit budget, agents under-invest in the
sequential thinking stages and over-invest in implementation.

**What follows from this.** Each dispatch purpose declares an expected effort range, and it
appears early in the prompt where attention is high:

- Specification: 5–15 tool calls. Read the design, check related decisions, draft each
  required section.
- Dev-plan: 5–10 tool calls. Read the spec, decompose with dependencies, size the tasks.
- Implementation: 10–50 tool calls per task. Read the spec section, implement, test,
  iterate.
- Review: 5–10 tool calls. Read the artefact, check against criteria, produce a verdict.

**Evidence strength: moderate.** A single well-documented industry source, but the mechanism
is clear and the intervention is cheap.

### 2.6 Stronger models orchestrate proactively; weaker ones communicate reactively

Masters et al. compared manager agents built on two model generations and found
*qualitatively* different action patterns, not just better answers:

| Behaviour | Proactive orchestrator | Reactive communicator |
|---|---|---|
| Task decomposition | 14.5× more frequent | Rare |
| Task refinement | 7.8× more frequent | Rare |
| Dependency tracking | 26× more frequent | Rare |
| Messaging | 0.4× (less) | 2.4× more frequent |
| Status checking | 0.1× (much less) | 10× more frequent |
| No-ops | 0.1× (much less) | 9× more frequent |

The proactive pattern builds structured chains: decompose, refine, assign. The reactive
pattern loops: message, message, check status.

Their conclusion is the important part: "Stronger reasoning models support more proactive
orchestration, but reasoning alone is insufficient." System design carries the rest. If
decomposition is the path of least resistance, weaker models follow it too.

**Evidence strength: strong,** and directly measured. Note that this is the failure mode
Cromwell avoids structurally rather than by prompting — a code orchestrator has no
reactive-communicator mode to fall into.

### 2.7 Team size saturates, and cost grows faster than output

DeepMind's multi-agent scaling data (2025) is unambiguous about the economics:

- A five-agent team costs seven times the tokens and produces 3.1× the output — an
  efficiency ratio of 0.44.
- At seven or more agents, output often *degrades* below a four-agent team while costing
  twelve times as much.
- **Team effectiveness saturates at four.** Diminishing returns start at three.
- If a single well-prompted agent reaches more than 45% of optimal performance, adding
  agents yields little.

The prescribed pattern is **cascade escalation**: start at Level 0 (one agent with tools),
escalate to Level 1 (worker plus reviewer), then Level 2 (a three-to-five agent team). Never
skip levels.

One refinement matters. The saturation point of four applies to **specialists sharing
coordination overhead** — a panel of reviewers all examining the same code from different
angles. Independent tasks are different: implementing five tasks from one dev-plan touches
five separate concerns and pays no coordination tax, so it's limited by task count, not by
a fixed cap.

Cromwell's `Dispatch.Workers` defaults to four, which is exactly the saturation point.

**Evidence strength: strong** for the scaling numbers. The distinction between specialist
panels and independent tasks is inferred from Google's alignment principle rather than
directly measured.

### 2.8 Specialists find what generalists miss

A panel of brief specialists outperforms one elaborate generalist. The mechanism is
vocabulary routing (§1.2): each specialist's terms activate a different knowledge cluster,
so each reads the same artefact through a different lens and finds different things.

The corollary is that **self-evaluation fails** — the generator shares the evaluator's
blind spots. Generation must be separated from evaluation, which Cromwell enforces
structurally: no executor verdicts its own work.

A second corollary is groupthink. Agents sharing a base model share blind spots, so more
agents does not automatically mean more perspectives. Diversity has to come from the
prompts.

**Practical ordering:** run the deterministic checks first — build, lint, tests — and only
then spend tokens on judgement. There's no point paying a reviewer to notice that the
build is broken.

**Evidence strength: moderate to strong.** The vocabulary mechanism is peer-reviewed; the
specific panel-versus-generalist detection rates quoted in industry sources are less well
controlled.

---

## 3. How agent systems fail

### 3.1 The MAST failure taxonomy

The MAST taxonomy (2024–25, developed alongside Cemri et al.'s "Why Do Multi-Agent LLM
Systems Fail?") catalogues fourteen distinct failure modes in three families. Most are
invisible without deliberate instrumentation.

**Communication failures**

| Mode | Description | Detection |
|---|---|---|
| FM-1.1 Message loss | Output not received by the next agent | Hash handoff payloads |
| FM-1.2 Misinterpretation | Instruction understood differently than intended | Require structured formats |
| FM-1.3 Information overload | Too much context degrades performance | Monitor token counts |
| FM-1.4 Stale context | Outdated information in the active window | Timestamp all context |

**Coordination failures**

| Mode | Description | Detection |
|---|---|---|
| FM-2.1 Deadlock | Agents waiting on each other | Time out every operation |
| FM-2.2 Race condition | Output depends on execution order | Sequence-number handoffs |
| FM-2.3 Role confusion | Overlapping or unclear responsibilities | Single-responsibility identities |
| FM-2.4 Authority vacuum | No agent has the authority to decide | Explicit decision owners |
| FM-2.5 Resource contention | Several agents modifying one resource | Lock or queue shared resources |

**Quality failures**

| Mode | Description | Detection |
|---|---|---|
| FM-3.1 Rubber-stamp approval | Reviewer approves without scrutiny | Require specific citations |
| FM-3.2 Error cascading | One mistake amplified downstream | Validate at each handoff |
| FM-3.3 Lowest-common-denominator output | Quality settles at the median | Domain-specific quality bars |
| FM-3.4 Groupthink | Agents converge on one approach | Diversity in specialist prompts |
| FM-3.5 Regression | A later agent undoes earlier work | Immutable completed sections |

**FM-3.1 is the most frequently observed quality failure.** Language models are
sycophantic; approval is the path of least resistance. The structural counter is to make a
clean verdict cost something: require per-criterion evidence for clearance, so "no issues
found" is not an acceptable output but "I checked these five things, here is what I saw"
is.

Cromwell attacks this from two directions. The verdict is *derived* from finding severity
rather than chosen, so a reviewer holding a major finding cannot approve. And the evidence
contract is enforced at parse time rather than merely requested in the prompt.

### 3.2 Orchestration abandonment

This one isn't from the literature — it's from running a predecessor system, and it's the
reason Cromwell's orchestrator is code.

An AI agent acting as orchestrator dispatched subagents, then lost the thread before
closing the loop. Completion was never called. Tasks sat queued. The batch was eventually
overridden to done with eleven tasks incomplete — a silent pass through the definition of
done.

The root cause is §1.1. The orchestrator's identity and its completion constraints sat in
the *middle* of an ever-growing context window. Drift wasn't a model defect; it was the
predicted behaviour given the attention curve.

A second shape of the same failure: the orchestrator stops orchestrating and starts
implementing.

**The lesson is not "don't spawn agents".** Spawning was the cure. The lesson is that the
loop-closing logic must not live in a context window. In Cromwell it's event-driven Go, and
gate G2 blocks a feature moving to review unless every task is terminal — which is exactly
the check that was missed.

### 3.3 The pernickety review loop

Also from practice rather than the literature, and it corrects the research's prescription.

The reviewed literature (Microsoft's maker-checker pattern) prescribes an iteration cap to
prevent infinite refinement. That handles genuine disagreement. But the failure actually
observed is different: the loop doesn't stall on disagreement, it stalls on the reviewer
querying finer and finer detail at a more and more pernickety level.

A plain round cap handles that badly, because it escalates trivia to a human — noise, and a
breach of the rule that human interruption is for things going wrong.

**The two controls are complementary, not alternatives.**

1. Find all major and minor issues.
2. Fix them.
3. Repeat while any **major** issue is found.
4. Stop when only minors remain, and drop them.
5. Keep a hard iteration cap as a separate backstop for the genuine-disagreement case.

Two guards are needed on the severity judgement itself. **Severity deflation** is the
dangerous one — a real defect marked minor is silently dropped at step 4, which is
rubber-stamping in a new costume. Define *major* concretely rather than by feel: a failure
of correctness, completeness, scope, or soundness is major; style, naming, and tidiness are
minor. **Severity inflation** is the milder inverse; a useful trip-wire is any review where
more than 30% of findings sit at the top tier.

Dropped minors should be recorded, not discarded. They are exactly the material a
retrospective picks up once the cycle completes.

---

## 4. What agents actually retrieve

A short section, because the conclusion is mostly negative and hard-won.

### 4.1 Anything an agent must choose to fetch will not be fetched

A predecessor system built a full knowledge subsystem: contribution, confidence scoring,
staleness detection, lifecycle management. It was dropped, because agents didn't use it.
Eighty-six entries contributed, twelve retrieved. A project memory was written and barely
read.

The obvious reading is that agents forgot to look. The accurate reading is sharper: **they
looked somewhere else.** They searched the documents directly, because that was the
interface they already knew. This is SWE-agent's finding (§2.4) arriving from a different
direction — a purpose-built novel tool loses to a familiar one.

**What follows from this.** Conventions and always/never rules must be *pushed* into the
assembled prompt, never left behind a retrieval tool. Where an agent does need to pull, let
it pull the way it already knows — by searching files. And keep the pushed block small: the
n=19 cliff and the attention budget both bite. A conventions block, not a corpus.

The one knowledge feature that did work was **retrospectives** — what went wrong, what went
right, what should change. They hardened the system and let a completed cycle be revisited
to pick up problems noted along the way.

### 4.2 Progressive disclosure is the token lever that does work

Where retrieval *is* built, the pattern that pays is returning an outline first and full
content only on request. Measured against reading whole files, section-level retrieval
delivered a 5–10× token reduction on targeted queries:

| Approach | Typical cost for a three-document question |
|---|---|
| Search, then read whole files | 15,000–60,000 tokens |
| Outline, then read two or three targeted sections | 1,500–3,000 tokens |

Three findings from the retrieval literature are worth keeping even though Cromwell doesn't
build a retrieval system:

- **Heading-based chunking beats fixed-size chunking** for Markdown. Fixed-size chunks split
  requirements across boundaries and mix unrelated sections; headings preserve authorial
  intent (Gao et al., 2024).
- **No single retrieval method dominates.** Keyword search (BM25) beats dense embedding
  retrieval on domain-specific technical text where exact terminology matters; dense
  retrieval wins on open-domain paraphrase. Hybrid beats either by 15–20% recall on
  technical corpora (Thakur et al., BEIR, 2021; Gao et al., 2023).
- **For small corpora, keyword search is competitive.** Below roughly a thousand documents
  the vocabulary is constrained enough that keyword matching has high precision, and the
  marginal value of embeddings is small (Lin, 2022).

The practical consequence for Cromwell is that grep is not a poor substitute for a
retrieval system at this scale — it's a reasonable one, provided the roles that need to
search actually have a search tool.

**Deliberately not built:** a knowledge subsystem, a memory store, or a design-document
graph. That experiment has run twice. If a document knowledge graph is wanted again, it
should be a separate or pre-existing system, not a Cromwell component.

---

## 5. Where Cromwell stands against the evidence

An honest scorecard, so that neither the strengths get regressed nor the gaps get forgotten.

### Conforms — do not regress these

| Finding | How Cromwell implements it |
|---|---|
| Enforceable constraints beat advisory ones (§2.1) | Deterministic Go orchestrator in `internal/rules/`; gates as pure functions in `internal/lifecycle/gates.go` |
| Separate generation from evaluation (§2.8) | No executor verdicts its own work; verification is always dispatched |
| Tool scoping per role (§1.3, §2.4) | Per-role tool profiles enforced as config errors — an agent cannot escalate beyond them |
| Brief identities, no flattery (§1.2) | Role `identity` fields carry job titles and reasoning, not superlatives |
| Reasons attached to rules (§1.6) | `AntiPattern.Because` is a first-class field |
| Team size at the saturation point (§2.7) | `Dispatch.Workers` defaults to four |
| Structured outcome tools (§2.4) | Dispatches complete through typed outcome tools, not free prose |
| Prompt ordering (§1.1) | Stable content first, artefact under review last |
| Loop termination (§3.3) | Severity-gated verdict plus a hard cap |
| Evidence contract (§3.1) | Enforced at parse time, not merely requested |

### Partial or open

- **Vocabulary payloads** exist on roles now, but not every role carries them and skills
  don't carry task-specific vocabulary of their own. The reviewer roles are furthest along.
- **Named anti-patterns** likewise: strong on the review roles, thin elsewhere.
- **BAD/GOOD examples** are largely absent from the starter skills. This is the cheapest
  remaining win from §1.4.
- **Project conventions are not pushed** into implementer prompts. An implementer receives
  the task, the spec, the dev-plan, and a paragraph on tools — no conventions, no
  architecture summary, no key interfaces. The `implement-task` skill says "match what is
  already there", so every implementer rediscovers the project's conventions by reading
  code on every dispatch: paying tokens each time and arriving somewhere slightly different
  each time. The fix that fits Cromwell's design is an approved project-conventions
  *document*, auto-surfaced into dispatch prompts — not a new knowledge subsystem (§4.1).
- **Retrospectives are unbuilt.** They're the one knowledge feature with a track record of
  working, and Cromwell's audit log and token ledger would make the observations
  computable.
- **Retrieval anchors** ("questions this skill answers") are moot here. Cromwell's skills
  are dispatched, not retrieved, so there's nothing for an anchor to match against.

---

## 6. Sources

### Peer-reviewed

| Source | Year | Finding used here |
|---|---|---|
| Vaswani et al., "Attention Is All You Need" | 2017 | Transformer architecture; n² pairwise attention |
| Zamfirescu-Pereira et al., "Why Johnny Can't Prompt" (CHI) | 2023 | Positive and negative constraints together are strongest |
| Hong et al., MetaGPT (ICLR 2024) | 2023 | Structured artefacts with verification gates reduce errors ~40% |
| Jimenez et al., SWE-bench (ICLR 2024) | 2023 | Benchmark and evaluation methodology for SE agents |
| Bordes et al.; Lewis et al.; Karpukhin et al. | 2013–20 | Foundations of embedding and retrieval-augmented generation |
| Liu et al., "Lost in the Middle" | 2024 | 30%+ accuracy drop for mid-context critical information |
| Ranjan et al., "One Word Is Not Enough" | 2024 | Vocabulary specificity routes to domain knowledge clusters |
| Yang et al., SWE-agent (Princeton) | 2024 | Interface design affects performance as much as model capability |
| Zhang et al., AutoCodeRover (ISSTA 2024) | 2024 | Structured, iterative context search beats single-pass loading |
| Guo et al., "LLM-based Multi-Agents: A Survey" | 2024 | Communication taxonomy; role profiling; memory mechanisms |
| Thakur et al., BEIR | 2021 | No single retrieval method dominates across domains |
| Gao et al., "RAG for LLMs: A Survey" | 2024 | Heading-based chunking; multi-stage retrieval pipelines |
| Wu et al., MIT Position Bias | 2025 | Causal masking and RoPE cause the U-shaped attention curve |
| Masters et al., "Orchestrating Human-AI Teams" (DAI 2025) | 2025 | Decomposition quality is the critical path; hard vs soft constraints; proactive vs reactive orchestration |
| Cemri et al., "Why Do Multi-Agent LLM Systems Fail?" | 2025 | Failure-mode taxonomy underlying MAST |
| Kim & Liu, Google Research scaling study | 2026 | Sequential penalty of 39–70%; alignment principle of +81%; error amplification 4.4× vs 17.2× |

### Industry, empirically grounded

| Source | Year | Finding used here |
|---|---|---|
| PRISM persona framework | 2024 | Sub-50-token identities optimal; flattery degrades output |
| LangChain few-shot research | 2024 | Three well-chosen examples match nine |
| MAST failure taxonomy | 2024–25 | Fourteen failure modes; rubber-stamp approval as the top quality failure |
| Anthropic, "Building Effective Agents" | 2024 | Agent-computer interface; tool poka-yoke; simple composable patterns |
| Anthropic, "How We Built Our Multi-Agent Research System" | 2025 | Effort scaling; delegation quality; 40% speedup from tool description rewrites; subagent output to the filesystem |
| Anthropic, "Effective Context Engineering" | 2025 | Attention budget; progressive disclosure; 15–40% utilisation |
| Anthropic, Skill Authoring Best Practices | 2025 | Conciseness; degrees of freedom; checklists; feedback loops |
| Anthropic, skill-creator meta-skill | 2025 | Create → test → review → improve loop; explanatory tone over imperatives |
| Anthropic, guardrails guides | 2025 | Output formats; permission to admit uncertainty; external knowledge restriction |
| DeepMind multi-agent scaling | 2025 | Saturation at four agents; superlinear coordination cost; the 45% threshold |
| Voyce, XML/Markdown comparative study | 2025 | Format alone accounts for up to 40% performance variance |
| Microsoft, AI Agent Orchestration Patterns | 2026 | Maker-checker loops; sequential gates; context compaction between agents |
| Vaarta Analytics, "Prompt Engineering Is System Design" | 2026 | At n=19 requirements, accuracy drops below n=5 |
| IBM, "What is AI Agent Orchestration?" | 2025 | Vocabulary for orchestration types; taxonomy of challenges |

### Worth reading next

- **MA-Gym** (github.com/DeepFlow-research/manager_agent_gym) — the open evaluation
  framework released with Masters et al. Its action taxonomy maps closely onto a workflow
  tool surface, and it could be adapted to evaluate orchestration changes directly.
- **Wang et al., MegaAgent** (ACL 2025 Findings) — a large-scale multi-agent system built
  *without* predefined standard operating procedures. A useful counterpoint to MetaGPT.
- **Google DeepMind's ADAS** work on automatically designing agent architectures.
- **Instruction-following benchmarks** (IFEval, FollowBench) — a related but distinct field
  from orchestration, relevant to how instructions should be structured for compliance.
