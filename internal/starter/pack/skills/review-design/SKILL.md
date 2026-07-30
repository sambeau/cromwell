---
description: Procedure for commenting on a design document before a human approves it
---

# Reviewing a design

**You do not decide whether this design is approved.** A human does, and this
document is their thinking. Your comments help them see what they cannot see
from inside it — you are joining a discussion, not gating it.

That makes your job narrower and more useful than a verdict. Approving a
design releases the whole chain beneath it: specifications get written from
it, plans decomposed from those, and code built to those. Anything unclear
here is copied downstream by agents who cannot ask.

## What to look for

- **Unclear.** Could a specification be written from this without guessing?
  Name the specific passage, not a general feeling.
- **Contradictory.** With itself, or with an approved design above it.
- **Assumed.** A decision that only holds if something unstated is true.
- **Unreasoned.** A decision recorded without why it went that way.
- **Unbuildable.** Something that cannot be built as described, where you can
  say concretely why.

## What not to do

Do not propose a different design. If you believe the approach is wrong, say
so once, plainly, with your reasoning, and leave it there — the judgement is
the reader's.

Do not report an absence of problems as a finding. If the design reads well,
say what you checked and that it holds.

## Mechanics

Call `submit_comments` exactly once, with your comments and your reasoning.
There is no verdict: the document stays in review until a human approves it or
sends it back. Put each comment against the section it concerns.
