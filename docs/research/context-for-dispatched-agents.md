# Context for dispatched agents

An agent you dispatch knows only what you tell it. Not what's in the project's instruction
file, not what tools the project prefers, not what the conventions are — only the contents
of the message that started it.

This document records how that failure was found, why prose-based fixes don't hold, and what
Cromwell does instead. It also covers the part that's easy to forget: different hosts inject
different things, so you can't rely on the platform to fill any gap.

---

## 1. The failure, as observed

During a remediation cycle on a predecessor project, several agents were dispatched in
parallel to work on different packages. All of them had a codebase knowledge graph
available — indexed, ready, and considerably faster than the alternative for structural
questions like finding callers or tracing dependencies.

None of them used it. Every one fell back to `grep`, path search, and reading files line by
line, including for exactly the structural questions the graph answered directly.

The cause turned out to be mundane. Dispatched agents never saw the project instruction
file. The dispatching agent had full knowledge of the graph — its project name, when to
prefer it, what to fall back to — and simply didn't pass any of it on, because nothing
required it to.

The obvious repair was to write the requirement down: a section in the instruction file
saying that every dispatch message must carry the graph context, plus a recursive clause
saying that an agent which itself dispatches must pass the same block forward.

It worked. It also had every weakness a prose fix has:

- It depends on the dispatching agent reading and following the instruction every single
  time — which is the behaviour that failed in the first place.
- The boilerplate has to be pasted into each message by hand.
- When the project name or the tool preferences change, the fix is a prose edit with no
  structured source of truth behind it.
- Nothing verifies that propagation happened. The failure is silent.
- It doesn't compose. A second project with different tools needs a different block, written
  again.

The general shape of the problem: **context was being delivered as a flat document that is
either entirely present or entirely absent.** There was no mechanism to deliver the relevant
part of it to the agent that needed it.

---

## 2. Why prose propagation can't be the answer

Two properties make an instruction-file fix structurally weak, independent of how well it's
written.

**It's all or nothing.** An instruction file is one blob. The dispatching agent gets the
whole thing; the dispatched agent gets none of it. There's no mechanism to extract the parts
a particular agent needs for a particular task, so the choice is between flooding the
context and starving it.

**It depends on an agent remembering.** Anything that must be recalled and re-typed under
pressure, in a context window that keeps growing, will eventually not be. This is the same
attention dynamic that caused a predecessor orchestrator to abandon dispatched work
mid-run: the instruction was in the middle of a long context, which is where attention is
weakest.

The conclusion the experience validated is that **context must be delivered structurally,
not by an instruction chain.** Which agent gets what should be a property of the system, not
a thing someone remembered to do.

It also validated a smaller point worth stating separately: **tool availability is
project-level context.** It applies to every agent working on the project regardless of
role, and it belongs at the same layer as coding style and architectural conventions rather
than being rediscovered per session.

---

## 3. What Cromwell does instead

Cromwell doesn't have this failure, because there's no dispatching agent to forget. The
orchestrator is event-driven Go, and the system prompt is assembled by
`roleSystemPrompt` from the role and skill bound to the dispatch purpose:

```
identity  →  vocabulary  →  anti-patterns  →  procedure  →  [contract and artefact]
```

Every dispatch of a given purpose gets the same assembled context, because the assembly is
code. The tool profile comes from the role and is enforced — an agent cannot escalate beyond
it, and naming a tool that doesn't exist is a configuration error rather than a silent
absence. Propagation depth doesn't arise, because dispatched agents don't dispatch.

That covers *who the agent is* and *what it's doing*. It does not yet cover *what this
codebase is like*.

### The layer that's missing

There are two distinct kinds of project context, and it's worth keeping them apart:

| Layer | Answers | Portable? |
|---|---|---|
| **Role** | Who is this agent — job title, domain vocabulary, named failure modes | Yes, across projects |
| **Project conventions** | What is this codebase — conventions, architecture, key interfaces, available tools | No, per project |

Cromwell has the first layer and not the second. An implementer's dispatch carries the task,
the specification, the dev-plan, and a paragraph about tools. No conventions, no
architecture summary, no key interfaces. The `implement-task` skill says "match what is
already there", so every implementer rediscovers the project's conventions by reading code —
paying tokens on every dispatch and arriving somewhere slightly different each time.

That split, rather than deeper role inheritance, is the honest answer to "how does the
starter pack work on any project". A role is portable. Conventions are not, and pretending
otherwise produces either generic roles or unportable ones.

**The fix that fits Cromwell's design is a document, not a new subsystem.** Documents are
already the unit of approved knowledge, and the review path already surfaces approved
ancestor documents into prompts automatically. An approved project-conventions document
pushed into dispatch prompts uses machinery that exists. What it needs is extending that
surfacing to the execution path — implementer and code-reviewer dispatches currently receive
no ancestor documents at all, only the spec and dev-plan.

The thing not to build is a knowledge subsystem or a document graph. That experiment has run
twice; see
[agent-research-evidence-base.md §4.1](agent-research-evidence-base.md#41-anything-an-agent-must-choose-to-fetch-will-not-be-fetched).

### Keep the pushed block small

The temptation, once there's a channel for conventions, is to fill it. Resist. Compliance
degrades past roughly nineteen requirements, and every token competes with every other token
for attention. A conventions block, not a corpus. Five to fifteen things that actually
change what the agent writes.

### An unresolved edge

Cromwell's structural guarantee holds for *dispatched* agents. It doesn't hold for a chat
agent doing the same work, where the tool profile can't be enforced and the assembled
prompt isn't Cromwell's to construct.

The practical net for that case isn't a gate but a watcher: flag commits touching an active
feature's paths with no claim behind them. Detection rather than prevention, which is the
right trade when prevention isn't available.

---

## 4. Don't rely on the host to inject anything

A second, quieter version of the same problem: platforms differ in what they put into a
model's context before your message arrives. Some inject a project instruction file
automatically. Some inject tool descriptions. Some do neither. Building on an assumption
here produces a system that works on one provider and silently degrades on another.

The rule is simple and worth stating as a rule: **assume nothing is injected.** Whatever the
agent needs, put it in the prompt you construct.

Cromwell already works this way — `roleSystemPrompt` builds the whole system half from the
compartment, and the tool schemas go out with the request. That's the correct posture and
it's what makes provider changes cheap.

### Model providers versus agent platforms

The distinction that causes most of the confusion:

- A **model provider** exposes a stateless inference API. It has no filesystem awareness, no
  concept of a project instruction file, and no native tool discovery. Everything is the
  caller's responsibility. DeepSeek is one of these — it offers OpenAI-compatible and
  Anthropic-compatible endpoints, and adds nothing on top of the raw inference response.
- An **agent platform** wraps a model with a runtime that may inject instruction files,
  discover tools, and manage sessions.

When a question comes up about "does this provider inject the instruction file", it's almost
always really a question about the *client* in front of it. A provider adds nothing; a
client may add a great deal. Answer the question about the layer that actually does the
work.

For a system that calls the inference API directly, as Cromwell does, this is
straightforward: there is no injection, and there's nothing to configure around. The prompt
is entirely what the code assembled.

### A practical note on compatibility endpoints

Providers offering an Anthropic-compatible endpoint let existing tooling point at them with
a base URL change and a different key. That's genuinely useful for cost and for testing
against a second provider, and it's how Cromwell's smoke environment reaches DeepSeek.

What it doesn't guarantee is behavioural parity. Tool-call shapes, parallel tool calls,
streaming semantics, and how strictly a schema is honoured all vary. Live smoke tests
against a real provider find bugs a mock cannot — parallel tool calls and whole-task review
diffs are two that have been found this way. Compatibility means the request is accepted,
not that the behaviour is identical.
