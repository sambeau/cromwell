# Research

Reference material that informs how Cromwell is built, and how it's written about. None of
it is a design or a specification — the binding documents live in `docs/design/`,
`docs/decisions/`, and `docs/specs/`. This folder holds the reasoning behind them.

Most of it comes from a research corpus assembled on a predecessor project, covering roughly
twenty academic papers and industry engineering reports on agent orchestration, prompting,
and skill design. It's been consolidated, brought up to date against how Cromwell actually
works, and stripped of everything that only made sense in the older system.

---

## What's here

### Agents

**[agent-research-evidence-base.md](agent-research-evidence-base.md)** — Why agents are
built the way they are. What a model attends to, what routes it to the right knowledge, how
multi-agent systems succeed and fail, and how Cromwell measures against all of it. Every
claim carries a source and a note on how strong the evidence is. Read this before changing
anything about how agents are prompted or arranged.

**[roles-skills-and-prompts.md](roles-skills-and-prompts.md)** — How to write a role and a
skill. Identity, vocabulary payloads, named anti-patterns, tool profiles, procedure
structure, examples, and the checklist to run before committing either.

**[orchestration-patterns.md](orchestration-patterns.md)** — How to arrange agents around a
piece of work: how many, in what topology, with what gates between them. Includes the stage
table, the patterns the field has validated, and a survey of what else exists.

**[context-for-dispatched-agents.md](context-for-dispatched-agents.md)** — Why an agent you
dispatch knows only what you tell it, why prose-based fixes for that don't hold, and what
Cromwell still doesn't push into a dispatch prompt.

**[definition-of-done-and-ready.md](definition-of-done-and-ready.md)** — A worked proposal
for what must be true before work closes, and before it starts, read against the gates
Cromwell already enforces.

**[prefix-cache-discipline.md](prefix-cache-discipline.md)** — What the Reasonix project has
worked out about keeping a prompt prefix stable enough for DeepSeek's cache to fire, how
much of it is genuinely new, and an audit of Cromwell's own dispatch path against it.
Includes one concrete defect in how dispatch prompts are ordered.

**[retrieval-and-document-intelligence.md](retrieval-and-document-intelligence.md)** — The
current state of retrieval-augmented generation, what Claude ships built-in, and whether
Cromwell should have a retrieval system for its own documents. The answer turns on a
finding: most of one is already built and wired to nothing but the search box.

**[token-estimation.md](token-estimation.md)** — Why Cromwell's token estimates keep being
wrong, and what the research says can be done about it. Frontier models predict their own
token use at r ≤ 0.39 and human-perceived difficulty tracks agent cost at τb = 0.32, which
between them rule out both the current approach and the obvious alternative. Recommends
intervals over point estimates, throughput forecasting over per-item estimation, and runtime
control over better prediction.

**[codebase-memory-mcp-review.md](codebase-memory-mcp-review.md)** — Whether the plan to
integrate a code knowledge graph still holds up, tested first-hand against Cromwell's own
codebase. Keep it, but turn the integration around: the orchestrator should query and push
rather than handing agents a tool the predecessor's agents ignored.

### Writing

**[writing-guide.md](writing-guide.md)** — How we plan, structure, and write documentation.
Covers audience, planning, structure, prose, punctuation, and the document types this
project produces. The human-facing versus agent-facing split is in §1.

**[editing-ai-prose.md](editing-ai-prose.md)** — An editing pass for removing the
characteristic tics of machine-generated text. Use it over a draft written to the writing
guide, not instead of it.

---

## How they relate

The agent documents build on each other. The evidence base holds the findings and their
sources; the other three apply them. If a recommendation anywhere seems arbitrary, the
reasoning is in the evidence base with a citation attached.

The two writing documents are a pair: one says how to write, the other says what to strip
out afterwards.

---

## What was deliberately left out

Three things in the original corpus aren't here, and it's worth recording why so they don't
get rebuilt by accident.

**A document knowledge graph.** The predecessor project designed and partly built one —
section-level indexing, a typed graph, full-text search, a standalone retrieval server. The
lesson worth keeping is that agents ignored the opt-in retrieval system and searched the
files directly instead. That lesson is in the evidence base (§4). The design isn't, because
the decision here is not to build one; if a knowledge graph is ever wanted again, it should
be a separate or pre-existing system rather than a Cromwell component.

**A knowledge subsystem and a project memory store.** Same reasoning. Contribution,
confidence scoring, and staleness detection all worked as designed, and the entries went
unread. Conventions have to be pushed into the prompt, not left behind a tool.

**Predecessor-specific implementation detail.** Package layouts, tool names, index sizes,
and migration plans for a system Cromwell doesn't share. Where a number carried a lesson —
the token cost of reading whole files, the ratio of knowledge entries contributed to
retrieved — the number survived and the surrounding detail didn't.

---

## Keeping this current

These are reference documents, so they go stale quietly. Two habits keep them honest:

- When a finding here is contradicted by something observed in a real run, update the
  document rather than remembering the exception. A walkthrough that disagrees with the
  evidence base is a signal that one of them is wrong.
- When a gap named here gets closed — the missing conventions block, retrospectives, the
  thin examples in the starter skills — edit the document that names it as open. A research
  note that describes a solved problem as unsolved sends the next reader in the wrong
  direction.
