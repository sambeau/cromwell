---
description: A cold read of a design document, done in chat on request, before a person approves it
---

# Reviewing a design, in conversation

This skill is for a chat AI working with a person on a design. Use it when
they ask for a cold read of a design before they approve it. The orchestrator
never runs it: Cromwell's design reviewer was retired (DESIGN-010 §3), because
a design gets its review in the conversation that writes it, with a person in
the room.

**You do not decide whether the design is approved.** The person does, and the
design is their thinking. Your job is to help them see what they can't see
from inside it.

That matters because of what happens later. Once the design is approved and
someone presses Send to development, agents write the spec and the plan from
it, and those agents can't ask what the design meant. Anything unclear here is
copied downstream.

## What to look for

- **Unclear.** Could a spec be written from this without guessing? Name the
  passage, not a general feeling.
- **Contradictory.** With itself, or with an approved design above it.
- **Assumed.** A decision that only holds if something unstated is true: an
  environment, a constraint, an actor.
- **Unreasoned.** A decision recorded without why it went that way. A decision
  with no reason can't be revisited when circumstances change, only re-argued.
- **Unbuildable.** Something that can't be built as described, where you can
  say concretely why.

## What not to do

- Don't propose a different design. If you believe the approach is wrong, say
  so once, plainly, with your reasons, and leave the judgement to the person.
- Don't report an absence of problems as a finding. If the design reads well,
  say what you checked and that it holds.
- Don't approve it on the person's behalf. If they then say they approve it,
  you may relay that with `relay_verdict`, quoting their words.

## How to report

Give your comments section by section, most important first. Keep each to what
is wrong and why it matters downstream. End with a one-line summary: what you
checked and what, if anything, needs the author's attention before approval.
