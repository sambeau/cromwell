# Prefix-cache discipline: lessons from Reasonix

Reasonix is a DeepSeek-native terminal coding agent, written in Go, whose entire
architecture is organised around one idea: keep the prompt prefix byte-stable so the
provider's cache keeps firing. It reports 85–99% cache hit rates on real sessions, and one
published single-day workload of 435 million input tokens cost about $12 instead of about
$61.

Cromwell runs against DeepSeek too, so the question is worth asking properly: what have they
found, is any of it new, and does any of it apply here?

The short answer: **the mechanism is not new, the discipline is, and one of their findings
exposes a real and fixable defect in how Cromwell orders its dispatch prompts.** There is
also a sting in the tail — Cromwell measures work in tokens rather than money, and cache
optimisation does not reduce the token count at all. That changes what the win actually is.

---

## Table of contents

1. [What Reasonix is](#1-what-reasonix-is)
2. [How DeepSeek's cache actually works](#2-how-deepseeks-cache-actually-works)
3. [What Reasonix does about it](#3-what-reasonix-does-about-it)
4. [Is any of this new?](#4-is-any-of-this-new)
5. [The counter-argument](#5-the-counter-argument)
6. [How Cromwell stands today](#6-how-cromwell-stands-today)
7. [What's worth taking](#7-whats-worth-taking)
8. [What isn't worth taking](#8-what-isnt-worth-taking)
9. [How this sits against our dispatch-context research](#9-how-this-sits-against-our-dispatch-context-research)
10. [Sources](#10-sources)

---

## 1. What Reasonix is

A terminal coding agent built only for DeepSeek, distributed as a single static Go binary
with no Node dependency, MIT-licensed, with a CLI and TUI, a desktop app, a browser UI, and
a VS Code extension over one local engine. Configuration lives in a `reasonix.toml` rather
than in code. External tools run as subprocess JSON-RPC handlers compatible with the Model
Context Protocol; built-in tools register at compile time.

The architectural claim is narrow and explicit: it is designed around *prefix-cache
stability*, and being DeepSeek-only is presented as foundational rather than as a limitation
to be fixed later. Their own field-guide coverage notes that porting the design to another
provider would mean rearchitecting, because the append-only logging, the prefix
partitioning, and the cost-escalation logic are all calibrated to DeepSeek's specific
behaviour.

Several structural choices are familiar from Cromwell: subagents run in isolated child
sessions and return only their final answer, tools are scoped per profile, and a planner and
an executor can run as two separate models. The convergence is worth noting, because it
arrived from a different direction.

---

## 2. How DeepSeek's cache actually works

Worth getting from the primary source, because the third-party write-ups are inconsistent on
the details.

**It is automatic and on by default.** DeepSeek runs context caching on disk for all users.
There is no `cache_control` marker to set — the system detects overlapping prefixes between
requests and serves the matched portion from cache. On the Anthropic-compatible endpoint,
`cache_control` is explicitly documented as **ignored**, which is not a problem: there is
nothing to opt into.

**Matching is on the prefix, and it must be exact.** A request hits only where its opening
tokens match a stored prefix unit. One changed character anywhere invalidates everything from
that point onward. This is the property the whole design rests on.

**Granularity is coarser than a byte.** The cache persists at prefix *units* — at the end of
user input, at the end of model output, at fixed token intervals for long inputs, and at
detected common prefixes. So "byte-identical" is the right discipline but slightly the wrong
mental model: a single changed byte doesn't invalidate that byte, it invalidates every unit
from the one containing it onward.

**It is best-effort, and it takes seconds to build.** DeepSeek documents no hit-rate
guarantee, and notes that sliding-window attention affects matching.

**The time-to-live is generous, and the third-party numbers are wrong.** DeepSeek's own
documentation says an unused cache is cleared "usually within a few hours to a few days".
One widely-cited write-up claims a 5–15 minute window. That claim contradicts the primary
source and should not be planned around in either direction — but the documented behaviour
means cross-request caching survives long gaps, which matters a great deal for a system like
Cromwell that dispatches in bursts rather than holding one long conversation.

**The price gap is very large.** From DeepSeek's pricing page, per million tokens:

| Model | Cache hit input | Cache miss input | Output | Ratio |
|---|---|---|---|---|
| `deepseek-v4-flash` | $0.0028 | $0.14 | $0.28 | 50× |
| `deepseek-v4-pro` | $0.003625 | $0.435 | $0.87 | 120× |

Note that Reasonix's headline "input-token cost collapses to about one-fifth" is a *blended
session* figure, not the per-token ratio. The per-token ratio is 50× to 120×. Both numbers
are true; they measure different things.

**The native API reports the split; the compatibility layer may not.** DeepSeek's own
chat-completions response returns `prompt_cache_hit_tokens` and `prompt_cache_miss_tokens`
as required fields. The Anthropic-compatible endpoint's documented usage fields do not
include `cache_creation_input_tokens` or `cache_read_input_tokens`. This matters for
Cromwell and is picked up in §6.

---

## 3. What Reasonix does about it

### 3.1 Three regions with different mutability

The core design. Context is partitioned into three zones with different rules, and the rules
are enforced by the framework rather than by convention.

| Region | Lifetime | Rule |
|---|---|---|
| **Pinned prefix** | Frozen at session start | System prompt, tool schemas, few-shot examples, persistent memory. Hashed at construction and never mutated mid-session |
| **Append-only log** | Grows monotonically | Every assistant turn and tool result appended in strict order. The `append()` method refuses any mutation; no rewrites, no reordering |
| **Volatile scratch** | Reset every turn | Transient reasoning, never sent upstream |

Their claim is that this discipline alone produces 85–95% hit rates on real sessions. The
part worth noticing is where the enforcement lives: code paths that try to mutate the prefix
or reorder the log are rejected *at the framework level, not caught in review*. That is the
same instinct as making a gate a pure function rather than an instruction.

What breaks the cache, in their accounting: injected timestamps, reordered history, variable
whitespace in tool-schema serialisation, mid-session edits to instructions, and — most
awkwardly — the compaction command itself.

### 3.2 Compaction is the enemy, and the answer is tiering

This is the sharpest problem they address, because compaction inherently rewrites history
and rewriting history is exactly what destroys a prefix.

Their response is a graded ladder rather than a single compaction step, keyed off how full
the context is:

- Below a `tool_result_snip_ratio` (default 0.6): leave the session alone apart from a soft
  notice.
- At the snip ratio: archive and shorten stale tool results before the recent tail, using
  deterministic head and tail markers.
- At a `compact_ratio` (default 0.8): archive stale tool results and prune them to short
  placeholders — *before* any summarising call.
- Only if pruning still leaves the prompt over threshold does summary compaction run.

Two supporting rules are good design independent of caching. A fact the user states in a
normal-sized turn is kept verbatim and never summarised away, at any point, across any
number of compactions. And everything dropped is archived one message per line, so the full
history stays traceable.

The insight generalises past caching: **prune deterministically before you summarise
probabilistically.** Pruning is cheap, reversible, and auditable. Summarising is none of
those.

### 3.3 Don't enumerate anything dynamic in a tool schema

A subtle one, from their subagent documentation. The parent model selects a subagent profile
at call time *without the profile names being listed in the tool schema* — explicitly for
prompt-cache stability.

The reasoning is worth extracting because it applies far beyond subagents: any tool schema
that enumerates a set which can grow — available profiles, allowed commands, known document
types, configured roles — becomes a volatile element sitting inside the pinned prefix.
Adding one entry re-bills every subsequent request in the session. A free-form string
parameter validated at dispatch time costs a little discoverability and buys prefix
stability.

### 3.4 Separate sessions rather than one mixed session

Where a planner model and an executor model both work on a task, they run in separate
sessions that never mix, so neither model's prefix is disturbed by the other's turns. Both
grow append-only. Model switching inside one conversation is treated as a cache-breaking
cost to be designed away rather than absorbed.

### 3.5 Four repair passes for DeepSeek's specific failure modes

Not cache-related, but the most immediately transferable material in the project, because
Cromwell talks to the same model family. All four run automatically with no configuration:

- **Schema flattening.** Schemas with more than about ten leaf parameters or more than two
  levels of nesting cause DeepSeek to silently omit arguments. Reasonix presents schemas
  flattened (`user.profile.name`) and re-nests on dispatch.
- **Reasoning-trace scavenging.** Tool calls sometimes leak into `<think>` blocks instead of
  being emitted as tool calls. They recover them by regex and JSON parsing.
- **Truncation recovery.** When a JSON payload hits the token limit mid-structure, they close
  braces, trim trailing commas, and fill dangling keys rather than failing the turn.
- **Storm breaking.** A sliding-window deduplication that detects an identical (tool,
  arguments) pair repeating and breaks the loop.

The first of these is a genuine, checkable claim about the model, and it is the one most
worth testing against Cromwell's own tool schemas.

---

## 4. Is any of this new?

Being precise, because the marketing and the engineering are different sizes.

**Prefix caching is not new.** DeepSeek shipped disk-based context caching in 2024, and
automatic prefix caching is standard across major providers. Anthropic's version is explicit
(you place `cache_control` breakpoints); DeepSeek's and OpenAI's are automatic.

**"Byte-identical prefix" is not a discovery.** It is simply how every prefix cache works,
documented by each provider. Any system that appends and never rewrites gets cache hits by
accident; any system that injects a timestamp at the top loses them by accident.

**What is new is treating prefix stability as a structural invariant.** Not "we try not to
break the cache" but "the log type refuses mutation, the prefix is hashed at construction,
and a code path that violates this fails at the framework level". Elevating a performance
property into an enforced invariant is a real design contribution, and it is the same move
Cromwell made when it turned workflow rules into pure gate functions instead of instructions.

**Three specific findings look genuinely novel, or at least under-published:**

1. **Tiered pruning before summarisation.** The framing of compaction as the natural enemy of
   caching, and the graded deterministic-first ladder that answers it, is better engineering
   than "summarise at 80% full".
2. **Tool schemas must not enumerate dynamic sets.** Simple, easy to violate, and rarely
   stated.
3. **Cache hit rate as the primary observability signal.** Not a billing curiosity but the
   metric that tells you whether your context discipline is holding. A falling hit rate is an
   early warning that something is mutating context that shouldn't be.

**And one claim deserves scepticism.** The 99.82% single-day figure is a single anecdotal
workload, self-reported, on an unstated task mix. The 85–95% range from the architecture
description is the number to reason with.

---

## 5. The counter-argument

Worth recording, because it is the strongest thing written about Reasonix and it cuts
against adopting the philosophy wholesale.

Cache-first is not automatically quality-first. Established agents avoid *accidentally*
breaking prefixes, but they sometimes break them *deliberately*, because the new context
shape works better — reshaping context, starting a fresh plan, dropping stale assumptions.
Reversing that hierarchy risks repeating a failed strategy cheaply.

The question that follows is a good one: does the agent know when cache hits are helping and
when they are hiding failure? A high hit rate on a run that is going in circles is not a win.
It is the same trap as a review gate with a 0% rejection rate — the number looks healthy
precisely because nothing is happening.

This maps onto something Cromwell already has an opinion about. Our review-loop research
concluded that the failure to guard against is a loop querying finer and finer detail, and
that a hard iteration cap is the backstop for genuine disagreement. A cache-first agent has
a matching failure mode: the cheaper a loop is, the longer it runs before anyone notices. If
cache hit rate is instrumented, it should be read alongside round counts and rework counts,
never alone.

---

## 6. How Cromwell stands today

I audited the dispatch path against Reasonix's invariants. The result is better than
expected in three places and has one concrete defect.

### What's already right

**The system prompt is deterministic and byte-stable.** `roleSystemPrompt`
([internal/server/planner.go:348](internal/server/planner.go:348)) builds identity, then
vocabulary, then anti-patterns, then the skill body — all from compartment files, with no
timestamps, no session identifiers, and no dynamic ordering. Every dispatch of a given role
produces the same bytes. That is the pinned prefix, and it is already correct.

**The in-dispatch loop is append-only.** The turn loop in
[internal/dispatch/dispatch.go:386](internal/dispatch/dispatch.go:386) only ever appends the
assistant message and then the tool results. Nothing is rewritten or reordered. Reasonix's
central invariant holds here already, by construction rather than by intent.

**Tool ordering is deterministic.** `ProfileToolDefs` resolves the role's declared `tools`
slice in order and appends the outcome tool. No map iteration, so no randomised ordering.
This is worth stating explicitly because Go map iteration is randomised, and a schema list
built from a map would silently bust the cache on every single request.

**No dynamic enumeration in tool schemas.** `run_command` takes a free-form command name
rather than enumerating the project's allowed commands. That happens to be exactly the
pattern Reasonix arrived at deliberately.

**Cromwell's dispatch model is structurally better placed than a single long session.**
Because every dispatch of a role shares one system prefix, and DeepSeek's documented cache
lifetime is hours to days, the prefix is warm across many separate dispatches. Reasonix has
to work to keep one session stable; Cromwell gets cross-dispatch reuse for free.

### The defect: volatile content sits ahead of shared content

In `planImplement` ([internal/server/planner.go:220](internal/server/planner.go:220)), the
user message is assembled in this order:

```
# Task to implement      ← different for every task
# Specification          ← identical for every task of the feature
# Dev-plan               ← identical for every task of the feature
# How to work            ← constant boilerplate
```

The one volatile block is first. Everything shared sits behind it, so the cacheable prefix
ends at the close of the system block and the whole contract is re-billed at cache-miss
price on every task dispatch.

`planCodeReview` ([internal/server/planner.go:264](internal/server/planner.go:264)) has the
same shape, and it is worse there, because the implement → review → request-changes loop
re-sends the contract on every round.

`planDocument` has the same problem across features: the feature name and description come
before the approved ancestor designs, and those designs are identical for every feature under
one initiative — which is precisely the fan-out the authoring invariant produces.

**The fix is reordering, and it costs nothing.** Put the shared contract first, the constant
boilerplate next, and the task-specific material last:

```
# Specification          ← shared prefix, cacheable
# Dev-plan               ← shared prefix, cacheable
# How to work            ← constant
# Task to implement      ← volatile, and in the recency slot
```

This is strictly better on both axes. The contract becomes a genuine shared prefix across
every dispatch for that feature, and the task moves *into* the high-attention end position
that boilerplate currently occupies. Our own prompt research says the artefact under work
belongs last; this ordering finally does that.

### The telemetry reads correctly — verified against live responses

Cromwell records cache tokens: `CacheRead` and `CacheWrite` flow from the provider through
to `cache_read_tokens` and `cache_write_tokens` on the dispatch row, and the price table has
`cache_read` and `cache_write` rates. The plumbing is all there.

An earlier draft of this section suspected the plumbing was dry. Those values are read from
`CacheReadInputTokens` and `CacheCreationInputTokens` in
[internal/provider/anthropic/anthropic.go:84](internal/provider/anthropic/anthropic.go:84),
which are Anthropic's field names, and DeepSeek's published docs for the compatible endpoint
don't mention either one — its native API names the split `prompt_cache_hit_tokens` and
`prompt_cache_miss_tokens`. The reasonable fear was that our cache columns were silently
zero for every DeepSeek dispatch.

**They are not. The suspicion is refuted.** Checked on 2026-08-01, two ways.

First, directly against the endpoint. Two identical requests to
`https://api.deepseek.com/anthropic/v1/messages` carrying a stable ~1,400-token system
block, one after the other, return usage objects in Anthropic's own vocabulary:

```json
// first call — cold
{ "input_tokens": 1392, "cache_creation_input_tokens": 0,
  "cache_read_input_tokens": 0, "output_tokens": 1, "service_tier": "standard" }

// second call — warm
{ "input_tokens": 112, "cache_creation_input_tokens": 0,
  "cache_read_input_tokens": 1280, "output_tokens": 1, "service_tier": "standard" }
```

DeepSeek's gateway translates its native field names into Anthropic's on the way out. The
`prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` naming belongs to the native endpoint
only, and never reaches our SDK.

Second, through Cromwell itself. Two estimator dispatches run back to back on the standing
smoke project (rebuilt binary, `cromwell_smoke`, `deepseek-chat`) produced these rows:

| purpose  | input | output | cache_read | cache_write |
|----------|-------|--------|------------|-------------|
| estimate |   966 |    369 |          0 |           0 |
| estimate |    68 |    324 |        896 |           0 |

The first ran on a cold prefix — the estimator role hadn't been dispatched in over a day, and
DeepSeek expires unused entries after hours to days. The second read 896 tokens from cache.
Note that these were two *different* features under one initiative, so what the second run
reused was the shared role prefix, which is exactly the cross-dispatch reuse claimed above.
The whole smoke history shows the same thing: cache reads in the thousands to tens of
thousands of tokens on implement, review and verify dispatches.

Three things follow, and they change what is worth doing next.

**Cache reads are already being recorded, so the reordering work can be measured.** There is
a working instrument. The defect above can be fixed and the fix demonstrated on real numbers
rather than argued from first principles.

**`cache_write_tokens` is structurally always zero on DeepSeek, and that is correct rather
than broken.** DeepSeek's cache is automatic — there is no explicit write step to report and
no separate write charge, since a miss is billed at the ordinary input rate. The
`cache_write: 0.28` line in the smoke config's price table is therefore never applied. It is
harmless, but nobody should read a zero in that column as a fault. On Anthropic proper the
column would carry real values, which is the next paragraph's subject.

**The accounting downstream is sound.** DeepSeek's `input_tokens` *excludes* the cached
portion — 112 + 1280 in the probe above reconstructs the 1,392 of the cold call — so
`Cost` ([internal/dispatch/dispatch.go:66](internal/dispatch/dispatch.go:66)), which prices
input and cache-read separately and adds them, neither double-counts nor loses tokens. The
`ActualTokens` roll-up ([internal/store/dispatches.go:279](internal/store/dispatches.go:279))
sums all four columns, so cached tokens still count as work done, which is the behaviour the
tokens-not-money stance wants.

There is a second, quieter issue on the Anthropic side. Anthropic's caching is *explicit* —
it only caches where you place a `cache_control` breakpoint, and Cromwell sets none. So
against Anthropic proper, Cromwell currently pays full price on every request, including for
a system prefix that is identical across hundreds of dispatches. That is a one-line change
with a real saving, and unlike the DeepSeek work it does not depend on reordering anything.

### The sting: tokens are not money

Cromwell measures work in tokens, deliberately, and money is being removed from the human
surface.

Cache hits do not reduce token count. A cached prompt token is still a prompt token; it is
billed differently, not counted differently. So none of this work changes the headline
number on the Work view, and none of it changes the `ActualTokens` figures that feed the
calibration corpus.

What it changes is cost — which is the axis being de-emphasised. That does not make it
worthless: it still affects the real bill, and it still affects the dollar-denominated
runaway-loop cap in the budget config, which is one of the money mechanisms whose fate is
still open. But it does mean the honest justification for this work is "the same work costs
less", not "the work gets smaller", and it should not be sold internally as the latter.

---

## 7. What's worth taking

Ranked by value over effort.

**1. Reorder the dispatch prompts.** The telemetry was the prerequisite here and it has now
been verified working (§6), so this can go ahead and be measured. Move the shared contract
ahead of the volatile task material in `planImplement`, `planCodeReview`, and `planDocument`.
Small, safe, improves both cache behaviour and attention ordering, and `cache_read_tokens`
before and after proves whether it worked.

**2. Add `cache_control` breakpoints for Anthropic.** Cromwell currently gets no caching at
all on Anthropic because it never asks for it. A breakpoint at the end of the system block
and after the tool definitions is the standard placement.

**3. Test the schema-flattening claim.** Reasonix reports that DeepSeek silently omits
arguments from schemas deeper than about two levels or wider than about ten leaves. Cromwell's
outcome tools — particularly the verification tool with its per-criterion evidence array —
are the ones to check. Silent argument omission on the evidence field would defeat the
evidence contract in a way that looks like a model failure rather than a schema problem. This
is worth a deliberate smoke test.

**4. Adopt "prune deterministically before summarising probabilistically".** Cromwell does
not compact today, because dispatches are short-lived. But the principle should be written
down before it is needed, and it applies to any future work on long-running or resumed
sessions.

**5. Keep dynamic enumeration out of tool schemas.** Cromwell already complies. Record it as
a rule so a future change that helpfully enumerates the allowed commands, the document types,
or the available roles doesn't quietly break a prefix that has been stable for a year.

**6. Surface cache hit rate, but never alone.** The telemetry does work, so the number is
available to show. It belongs next to round counts and rework counts. A rising hit rate with
a rising round count is a loop running cheaply, not a system working well.

---

## 8. What isn't worth taking

**The single-session architecture.** Reasonix's three-region model exists because it holds
one long conversation. Cromwell's dispatches are short, isolated, and stateless between runs,
with durable state in Postgres rather than in a context window. That is a stronger position,
not a weaker one, and the immutable-prefix machinery would be solving a problem Cromwell does
not have.

**The provider lock-in.** Being DeepSeek-only is load-bearing for them and would be a
regression here. Cromwell's per-role model assignment is more valuable than a single-provider
optimisation.

**Cache-first as a philosophy.** Cache discipline is worth having as a constraint on prompt
construction. It is not worth having as a value that outranks getting the work right. If a
change makes a dispatch prompt clearly better and breaks a prefix, make the change.

**Their memory system.** Durable searchable facts with human approval, immutable IDs, and
monotonic revisions is a well-built version of exactly the thing that has already been tried
twice here and gone unread. Nothing about their implementation changes the finding that
agents ignore what they must choose to fetch.

---

## 9. How this sits against our dispatch-context research

The user's question was how this compares with our own work on what a dispatched agent
receives. The two lines of research converge more than they conflict, and the convergence is
informative.

**Where they agree.** Both isolate the child completely and return only its outcome to the
parent. Both scope tools per profile or role. Both put stable content first and the artefact
under work last. Both separate a planner from an executor rather than switching models
mid-conversation. Reasonix arrived at these from cost pressure; our research arrived at them
from the attention curve and from the orchestration-abandonment failure. Two different
pressures, one answer.

**This settles a question that was already half-settled.** Our conformance work concluded
that caching and attention want the same prompt order — stable content first, artefact under
review last — and that an earlier worry about the two conflicting was mistaken. Reasonix is
independent confirmation from a system built entirely on the cost axis. The note can be
promoted from "we checked, they agree" to "a system optimised purely for cache converged on
the ordering our attention research prescribes".

**Where Reasonix adds something we did not have.** Our research ordered the assembled packet
by attention value alone. It had no concept of *shared-across-dispatches* content, because
there was no reason to care which parts of a prompt two different dispatches had in common.
Cache adds that second axis, and it produces a rule our research does not contain:

> Within a dispatch prompt, order by how many dispatches share the content. What every
> dispatch of this role shares goes first. What every dispatch for this feature shares goes
> next. What only this dispatch has goes last.

That rule happens to agree with the attention ordering rather than fighting it, which is why
the fix in §6 is free. But it is a genuinely new consideration, and it is the one thing here
worth folding into the existing research.

**Where our position is stronger.** Reasonix's parent is a model holding a conversation, so
its subagent isolation is a defence against polluting that conversation. Cromwell's parent is
code holding a database, so there is no parent context to pollute — the failure mode is
absent rather than defended against. Their design has to work to prevent something Cromwell
made impossible.

**Where our position is weaker.** They measure. Cache hit rate is their primary observability
signal, and they can tell within a session whether their context discipline is holding.
Cromwell has the columns and, as of the 2026-08-01 check, genuinely has the numbers in them —
but nothing reads them back. The gap is no longer instrumentation, it is that the figure
reaches no view, no roll-up and no alarm, so a prefix that quietly stops caching would go
unnoticed until the bill arrived.

---

## 10. Sources

### Primary — DeepSeek

- [Context caching guide](https://api-docs.deepseek.com/guides/kv_cache) — how the cache
  works, prefix-unit granularity, best-effort behaviour, and the "hours to days" lifetime.
- [Pricing](https://api-docs.deepseek.com/quick_start/pricing) — the cache-hit and
  cache-miss input rates quoted in §2.
- [Anthropic API compatibility](https://api-docs.deepseek.com/guides/anthropic_api) —
  `cache_control` is ignored; the cache usage fields are not documented on this endpoint.
- [Create chat completion](https://api-docs.deepseek.com/api/create-chat-completion) —
  `prompt_cache_hit_tokens` and `prompt_cache_miss_tokens` in the native usage object.

### Primary — Reasonix

- [reasonix.io](https://reasonix.io/) — the product claims, including the "90%+ cache hit"
  and "~1/5 input-token cost" figures.
- [github.com/esengine/DeepSeek-Reasonix](https://github.com/esengine/DeepSeek-Reasonix) —
  repository and README.
- [SPEC.md](https://github.com/esengine/DeepSeek-Reasonix/blob/main-v2/docs/SPEC.md) — the
  engineering spec: tiered compaction ratios, the verbatim-fact rule, archival, subagent
  isolation, fleet concurrency defaults.
- [SUBAGENT_PROFILES.md](https://github.com/esengine/DeepSeek-Reasonix/blob/main-v2/docs/SUBAGENT_PROFILES.md)
  — isolated child agents, tool scoping, and the note about keeping profile names out of the
  tool schema for prompt-cache stability.

### Secondary — analysis and claims

- [How a DeepSeek-only agent framework hit 85% prefix cache rate](https://dev.to/esengine/how-a-deepseek-only-agent-framework-hit-85-prefix-cache-rate-and-saved-93-vs-claude-5c9g)
  — the author's own write-up: the three-region architecture and the four tool-repair passes.
- [CodeWhale issue #2264](https://github.com/Hmbown/CodeWhale/issues/2264) — another project
  proposing to adopt the architecture; a useful outside summary of the invariants. Note that
  its 5–15 minute cache TTL claim contradicts DeepSeek's own documentation.
- [Reasonix shows the next coding agent fight is cache discipline](https://www.developersdigest.tech/blog/deepseek-reasonix-cache-first-coding-agents)
  — the counter-argument in §5.
- [Agent Harness Field Guide: Reasonix](https://agents.buttonscli.com/field-guide/reasonix)
  — independent assessment, the 99.82% anecdote, and the provider lock-in trade-off.

### Cromwell code referenced

- [internal/server/planner.go:348](internal/server/planner.go:348) — `roleSystemPrompt`.
- [internal/server/planner.go:220](internal/server/planner.go:220) — `planImplement` user
  message ordering.
- [internal/server/planner.go:264](internal/server/planner.go:264) — `planCodeReview` user
  message ordering.
- [internal/dispatch/dispatch.go:386](internal/dispatch/dispatch.go:386) — the append-only
  turn loop.
- [internal/provider/anthropic/anthropic.go:84](internal/provider/anthropic/anthropic.go:84)
  — where cache usage is read from the response.
