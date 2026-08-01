# Writing roles, skills, and prompts

This is the practical guide to the two files that shape every dispatched agent: the role
that says who the agent is, and the skill that says what it's doing right now. It also
covers the prompt-construction principles behind them, so the same reasoning can be applied
anywhere a model is given instructions.

The evidence behind every recommendation lives in
[agent-research-evidence-base.md](agent-research-evidence-base.md). This document assumes
that evidence and gets on with the work.

The one-sentence version:

> The words you choose, where you place them, and what rules you surface matter more than
> how many agents you deploy or how long your prompts are.

---

## Table of contents

1. [How roles and skills divide the work](#1-how-roles-and-skills-divide-the-work)
2. [Writing a role](#2-writing-a-role)
3. [Writing a skill](#3-writing-a-skill)
4. [Matching freedom to risk](#4-matching-freedom-to-risk)
5. [Content rules for both](#5-content-rules-for-both)
6. [Developing and testing a skill](#6-developing-and-testing-a-skill)
7. [Common mistakes](#7-common-mistakes)
8. [The authoring checklist](#8-the-authoring-checklist)

---

## 1. How roles and skills divide the work

Cromwell assembles a dispatched agent's system prompt from two files:

- A **role** (`roles/<name>.yaml`) — who the agent is. Identity, domain vocabulary, named
  anti-patterns, the tools it may use, and its limits. Portable between projects.
- A **skill** (`skills/<name>/SKILL.md`) — what it's doing right now. The procedure, what
  the output must contain, how to complete the dispatch.

The assembled order is fixed, and both prompt caching and the attention curve want the same
thing from it:

```
identity  →  vocabulary  →  anti-patterns  →  procedure  →  [the artefact under work]
```

Stable content first, so the provider's cache stays warm. Constraints early and the
artefact last, so the material that needs the most attention gets it.

Roles and skills compose by addition. The same `review-code` skill is used by every
reviewer; the specialisation comes from the role's vocabulary, not from a different
procedure. Four specialists reading the same diff through four vocabularies find four
different sets of things.

### One constraint that is specific to Cromwell

**The whole skill body is pushed into the system prompt on every dispatch.** There is no
on-demand loading, no `references/` directory the agent pulls from when it needs detail,
and no triggering decision to influence. Cromwell's skills are dispatched, not retrieved.

Three consequences follow, and they change the usual advice:

- **Every byte is paid, every time.** Length discipline matters more here than in a system
  where detail can be deferred to a file the model may never open. Overflow material has no
  cheaper tier to fall into — it has to be cut, not moved.
- **Descriptions don't need to be persuasive.** Guidance written for retrieved skills says
  to make descriptions "a little bit pushy" to combat under-triggering. That problem doesn't
  exist here. The `description` field is documentation for the human reading the
  compartment.
- **Retrieval anchors are pointless.** A "questions this skill answers" section exists to
  be matched against a query. Nothing queries a Cromwell skill.

---

## 2. Writing a role

A role is a small YAML file. Every field earns its place.

```yaml
model: claude-sonnet-5
skill: review-code
identity: |
  You are the code reviewer for this project. You inspect one task's diff
  against the specification and the dev-plan and decide whether it correctly
  and completely implements that task.
vocabulary:
  - "cyclomatic complexity"
  - "error wrapping with %w"
  # ...
anti_patterns:
  - name: "Rubber-stamp Approval"
    detect: "A verdict of approve with no findings and no evidence of what was checked"
    because: "Approval is the path of least resistance for a language model, and an unexamined approval is indistinguishable from an examined one downstream"
    resolve: "Either name a finding or state, per criterion, what you checked and what you saw"
tools: [read_file, list_files]
limits:
  turn_cap: 15
```

### Identity

A real job title and the shape of the judgement being asked for. Under fifty tokens of
actual identity, though a short paragraph explaining what this agent is *for* earns its
keep — the reviewer roles in the starter pack do this well.

What doesn't belong: superlatives, praise, or backstory. "World-class", "expert",
"you excel at", "with decades of experience" all measurably degrade output by activating
marketing prose. Competence is declared through vocabulary and anti-patterns.

> **Good.** You are the dev-plan reviewer for this project. A dev-plan you approve is
> decomposed into tasks that implementer agents build in isolation, so a weak decomposition
> produces weak code no matter how good the spec was.

> **Bad.** You are an extraordinarily talented, world-class software architect with decades
> of experience across every major programming language, known for your brilliant insights
> and exceptional attention to detail.

The good version says what the agent is for and why the judgement matters. The bad version
says nothing and routes the model to job adverts.

### Vocabulary

Fifteen to thirty precise domain terms. These are the routing signal — the single
highest-return element of the whole file.

Apply the fifteen-year practitioner test to each one: would a senior expert with fifteen
years in this domain use this exact term speaking to a peer? Include it if yes. Cut it if
it's a term the model already applies to everything.

Terms should be specific to the *domain*, not to the workflow. "Review dimension" and
"finding classification" are workflow vocabulary and belong in the skill if anywhere;
"goroutine leak", "boundary value analysis", and "insecure direct object reference" are
domain vocabulary and belong in the role.

Vocabulary is per-project, because routing needs tuning to the actual stack. A Go project
and a React project need different implementer vocabularies; a desktop application with a
Rust shell needs both, plus different security vocabularies for the browser surface and the
inter-process boundary.

One limitation worth naming when generating vocabulary from a codebase: analysis discovers
*implementation* vocabulary — the technologies and idioms present in the source. It cannot
discover *expertise* vocabulary — what a specialist reviewer needs to know about that isn't
in any source file. OWASP, STRIDE, and CVSS aren't in the code. Automated discovery gets
you perhaps sixty per cent; a human curates the rest.

### Anti-patterns

Five to ten named failure modes. Cromwell's schema has four fields and `because` is the
load-bearing one.

- **name** — Name it. "The Eager-Loading Trap" routes to specific knowledge; "don't load too
  much data" routes to generic advice.
- **detect** — The observable signal. Something the agent can check its own output against.
- **because** — The consequence chain. Not a restatement of `detect`; the thing that goes
  wrong downstream, which is what lets the model generalise to cases you didn't list.
- **resolve** — What to do instead.

> **Good.**
> *Silent Assumption.* Detect: a decision that only works if something unstated is true — an
> environment, a constraint, an actor. Because: the specification will be written from this
> document by an agent that shares none of the author's context, so an assumption left in
> someone's head becomes a defect much further downstream. Resolve: name the assumption and
> ask for it to be stated.

> **Bad.**
> *Assumptions.* Detect: unstated assumptions. Because: assumptions are bad. Resolve: don't
> make assumptions.

The good version's `because` traces a chain to a real consequence. The bad version restates
its own detection signal three times.

Some anti-patterns are portable and some aren't. When porting from another system, check
whether the failure mode still exists. An anti-pattern warning an agent not to trust an
orchestrator's assurances is obsolete in Cromwell, because Cromwell's orchestrator is code
and cannot be wrong about what it dispatched.

### Tools and limits

The tool profile is enforced — an agent cannot escalate beyond it, and naming a tool that
doesn't exist is a configuration error. This is a stronger guarantee than most systems
offer, and it's worth using deliberately.

Two things to weigh. Every tool definition in a prompt consumes attention budget, so a
reviewer that never decomposes shouldn't carry a decomposition tool. But the opposite error
is real too: agents fall back to searching files when a purpose-built path is unfamiliar,
so a role with no search tool at all has had its most reliable fallback removed.

`turn_cap` is a cost control, not a quality one. Set it from the effort budget for the
purpose, with headroom.

---

## 3. Writing a skill

A skill is `skills/<name>/SKILL.md`: YAML front matter with a `description`, then a body
that becomes the `# Procedure` section of the system prompt.

### Section order

Follow the attention curve. The role has already delivered identity, vocabulary, and
anti-patterns by the time the skill body starts, so the skill picks up at the procedure and
runs to the output contract.

| Order | Section | Why here |
|---|---|---|
| 1 | Task-specific vocabulary, if any | Highest position available to the skill |
| 2 | Task-specific anti-patterns, if any | Constraints before instructions |
| 3 | Checklist, for low- and medium-freedom work | Makes compliance visible before the steps begin |
| 4 | Order of work | Numbered steps survive the attention dip |
| 5 | What the output must contain | Rising attention |
| 6 | Examples | Recency bias — put the best one last |
| 7 | Mechanics: how to complete the dispatch | Last thing read, first thing needed at the end |

Only include a section if it earns its bytes. Most skills won't need their own vocabulary;
the role usually carries enough.

### Order of work

Five to ten numbered steps. Not twenty. Each step is one action, phrased imperatively, and
starts with a verb.

Branching belongs in the steps as IF/THEN, not in prose beside them:

```
4. IF any input is missing → STOP. Report what is missing. Do not proceed.
5. IF the spec is ambiguous for any part of this task → STOP and report the ambiguity.
   Do not infer intent.
```

Where a step can fail in a way the agent should fix rather than report, make the loop
explicit — validate, fix, re-validate, and only then continue. A linear sequence lets an
agent walk past a failure.

### What the output must contain

Freeform output is the root cause of inconsistent specifications and plans. Define:

- The required sections, and what each must contain.
- Cross-reference requirements — a specification cites its design, a dev-plan cites its
  specification.
- The format for anything that will be checked later: acceptance criteria, findings,
  severities.

The template is itself a forcing function. An agent that has to fill eight sections cannot
finish after two tool calls, which is the mechanism that prevents rushing to the part that
feels productive.

Match template strictness to freedom level (§4). A specification template is strict. A
design document template is a suggested shape.

### Examples

At least one BAD/GOOD pair, with a sentence on *why* each is what it is. Three well-chosen
examples do the work of nine, so curate rather than accumulate. Best example last.

Use realistic content. `foo`, `bar`, and "example placeholder here" teach nothing, because
the model pattern-matches against the shape of real output.

### Mechanics

End with how the dispatch completes: which outcome tool, called once, with what in it. This
is the last thing in the prompt and the last thing the agent does — the positions agree.

---

## 4. Matching freedom to risk

Anthropic's framing is a robot on a path. A narrow bridge with cliffs on both sides has one
safe way forward. An open field has many. Give instructions to match.

| Freedom | When | What the skill provides |
|---|---|---|
| **Low** | Fragile operations where wrong execution is expensive to undo | Exact sequences, exact tool calls, no discretion |
| **Medium** | A preferred pattern exists, some variation is fine | Templates with defined required content |
| **High** | Many valid approaches; context decides | Principles, vocabulary, and constraints — then trust the agent |

Mapped onto Cromwell's purposes:

| Purpose | Freedom | Why |
|---|---|---|
| `write-spec`, `write-dev-plan` | Medium | Template exists; content varies per feature |
| `review-code`, `review-spec`, `review-dev-plan` | Medium | Structured criteria, but severity is judgement |
| `verify-feature` | Low | The evidence contract is checked mechanically |
| `implement-task` | High | Many valid implementations; the spec is the constraint |
| `review-design` | High | Reading a human's thinking; over-constraining produces noise |
| `estimate-work` | Medium | Defined output, judged input |

The cost of over-constraining creative work is mediocre output. The cost of
under-constraining fragile work is broken state. Applying uniform medium freedom to
everything gets both wrong.

Freedom level also tells you where enforcement belongs. Low-freedom work should be gated by
code that refuses invalid operations, not by instructions the agent is trusted to follow.
Medium-freedom work is warned about and reviewed. High-freedom work is guided by vocabulary
and anti-patterns and not gated at all.

---

## 5. Content rules for both

### The novelty test

Before writing any line, ask: does the model already know this?

- **Include:** this project's lifecycle states, its transition rules, its conventions, its
  tool call sequences, its vocabulary, its named failure modes.
- **Exclude:** what a state machine is, what a lifecycle transition means, how Go
  interfaces work, how to write a test, what YAML is.

If a paragraph would be true of any system, delete it. If it explains a general concept
*and* a project-specific rule, keep only the rule. Two hundred tokens of project-specific
content beats eight hundred tokens of general explanation with the same two hundred buried
inside.

### Explanatory tone, not imperative

Anti-patterns carry `because` clauses, and the same applies to everything else. Prefer "do
X because Y" over "ALWAYS do X". Capitalised MUSTs are a signal to reframe.

This isn't a ban on clear conventions — it's a ban on *unexplained* ones. A reason turns a
brittle rule into a principle the model can extend.

### Consistent terminology

One term, one concept, throughout. If the vocabulary says "finding", never write "issue" or
"problem" in the procedure. If it says "acceptance criteria", never write "requirements" or
"success conditions". Inconsistent terminology inside a prompt undermines the routing the
vocabulary was there to do.

### An uncertainty protocol, positioned early

Every skill that produces work output needs an explicit instruction that stopping is
allowed, placed where attention is high:

> If the specification is ambiguous, incomplete, or contradictory for any part of this
> task, stop and report the ambiguity. Do not infer intent and do not make undocumented
> design decisions. The cost of asking is low; the cost of guessing wrong is high.

Without this, the model's default is to produce *something* rather than nothing. Explicit
permission to admit uncertainty measurably reduces invention.

### Ground decisions in the contract

Implementation and authoring skills should require the agent to cite the specific section
that justifies a non-trivial choice. If no section covers it, that's an assumption to flag,
not a decision to make quietly.

The review skills already have the mirror of this rule as the *Missing Spec Anchor*
anti-pattern: a finding without a spec anchor is an opinion, not a conformance gap.

### No time-sensitive content

Don't write dates or version numbers into a skill. Use "the current method" and "the
previous method". A skill that names a date is stale the moment the date passes, and stale
context is worse than absent context.

### Give a default, not a menu

Offering three approaches without saying which to use costs tokens and produces variance.
Give the default path, and mention alternatives only as escape hatches with a stated
condition.

---

## 6. Developing and testing a skill

Skills aren't documentation. They're executable context that directly shapes output, and
they deserve the same rigour as code. The loop below is deliberately light — a small skill
edit might be fifteen minutes — but zero evaluation is never acceptable.

1. **Find the gap.** Run agents on representative tasks *without* the skill, or with the
   current one. Write down what they actually get wrong: skipped steps, wrong output shape,
   generic vocabulary, invented requirements.
2. **Write the minimum that addresses it.** Only the observed gaps. Don't document imagined
   problems, and don't explain things the model already handles.
3. **Define three to five test scenarios.** Concrete task descriptions of the kind a real
   dispatch would carry.
4. **Run and compare.** Execute with and without the change. Does it close the specific gaps
   from step 1? Does it introduce new ones — wasted turns on a section the agent ignores?
5. **Iterate, and prefer cutting.**
   - If the skill doesn't improve output, cut content rather than add it.
   - If agents ignore a section, it's either in the attention dead zone or explaining
     something they already know. Reposition or delete.
   - If every run performs the same multi-step operation by hand, that's a signal the
     operation should become a tool, not a paragraph.
   - If an issue is stubborn, try a different metaphor or a different pattern of working
     before adding another constraint.
   - When a skill fails consistently, hand the skill text and a concrete failure to an agent
     and ask why it didn't work. Models diagnose their own prompt failures well, and this
     often surfaces ordering and vocabulary problems a human reader misses.
6. **Read the transcripts, not just the outputs.** If the skill is making the model waste
   turns on unproductive work, that's visible in the transcript and invisible in the result.

The generalisation rule matters most. A skill is used many times across many prompts, so
resist fiddly overfitted patches aimed at one bad run.

---

## 7. Common mistakes

**Explaining what the model already knows.** A skill that defines "state machine" or
explains how YAML works has spent its budget on nothing. Delete every paragraph that would
be true of any system.

**Uniform constraint levels.** Medium freedom everywhere means fragile operations are
under-constrained and creative work is over-constrained. Both degrade.

**Missing examples.** Abstract rules without examples are weaker than examples without
rules. Invest more effort in two excellent examples than in twenty rules.

**Over-specification.** Past roughly nineteen requirements, compliance drops below what five
would have achieved. The instinct to add one more rule to fix a problem usually makes it
worse.

**Unexplained imperatives.** "ALWAYS do X" covers the case you wrote. "Do X because Y"
covers the ones you didn't. A rule without its reason also can't be pruned, because nobody
remembers why it exists.

**Vocabulary that is really a glossary.** Vocabulary terms are routing signals, not
definitions for a reader. A list of workflow nouns with explanations attached is a glossary
and does no routing.

**A `because` that restates the `detect`.** "Detect: the agent skips the tests. Because:
skipping tests is bad." No consequence chain, no generalisation.

**Copying an anti-pattern that no longer applies.** Ported material carries assumptions from
the system it came from. Check each one against how Cromwell actually works.

---

## 8. The authoring checklist

Use this before committing a new or revised role or skill.

### Role

- [ ] `identity` is a real job title plus, at most, a short paragraph on what the judgement
      is for.
- [ ] No superlatives, praise, or backstory anywhere in the file.
- [ ] `vocabulary` has 15–30 terms, each passing the fifteen-year practitioner test.
- [ ] Vocabulary terms are domain terms, not workflow nouns.
- [ ] `anti_patterns` has 5–10 entries with `detect`, `because`, and `resolve`.
- [ ] Every `because` names a consequence rather than restating the detection signal.
- [ ] `tools` contains what the role needs and nothing else — and includes a search tool if
      the role will need to find things.
- [ ] `limits.turn_cap` matches the effort budget for the purpose, with headroom.
- [ ] Any ported anti-pattern still describes a failure mode that exists here.

### Skill

- [ ] Section order follows the attention curve: constraints, then procedure, then output,
      then examples, then mechanics.
- [ ] Every paragraph passes the novelty test — nothing the model already knows.
- [ ] The procedure has 5–10 steps, each one action, each starting with a verb.
- [ ] Branching is expressed as IF/THEN inside the steps.
- [ ] Fragile steps have an explicit validate → fix → re-validate loop.
- [ ] Freedom level matches the risk, and the procedure style matches the freedom level.
- [ ] The output contract states required sections, cross-references, and any format that
      will be checked downstream.
- [ ] At least one BAD/GOOD example pair, each with a sentence saying why.
- [ ] The best GOOD example is last.
- [ ] Examples use realistic content.
- [ ] An uncertainty instruction appears early.
- [ ] Terminology matches the role's vocabulary exactly, throughout.
- [ ] Tone is explanatory; no capitalised MUSTs.
- [ ] No dates or version numbers.
- [ ] The mechanics section names the outcome tool and says it is called exactly once.
- [ ] Every byte earns its place — the whole body ships on every dispatch.

### Testing

- [ ] Three or more realistic test scenarios exist.
- [ ] Output was compared with and without the change.
- [ ] The change demonstrably closes the gap it was written for.
- [ ] Transcripts were read, not just final outputs.
