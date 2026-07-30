---
description: Procedure for writing a feature specification from an approved design
---

# Writing a specification

You are translating, not inventing. A human approved the design; nobody
approved this specification, and nobody will — it goes to an agent reviewer
whose job is to check that the translation is faithful and complete. What you
write becomes a binding contract that implementers build to and a verifier
checks against.

## Order of work

1. Read the design end to end before writing anything. Read the ancestor
   designs too: this feature sits inside them and must not contradict them.
2. List the design's decisions — every one that bears on what gets built.
   This list is what your specification has to account for.
3. Read the feature's own name and description. They narrow the design to the
   slice this feature covers.
4. Write the specification against the template's sections.
5. Before you finish, walk your list from step 2. Every decision must appear
   in the specification or be explicitly out of scope. Anything left over is
   the gap the reviewer will find.

## What a finished specification has

- **Behaviour a person could not misread.** Error paths, empty states and
  boundary conditions are written, not implied. If you find yourself writing
  "handles errors appropriately", you have written nothing.
- **Acceptance criteria that are checks.** Each one names an input, an action
  and an observable outcome. "Works well" is not a criterion; "returns within
  2 seconds for inputs up to 10k rows" is.
- **A boundary.** What this feature deliberately does not do, where that is
  not obvious.

## When the design does not say

You will find questions the design did not answer. You have three honest
options, and inventing an answer is not among them:

- If the design implies an answer, follow the implication and say in your
  reasoning that you did.
- If it is a detail below the design's altitude — a name, an ordering with no
  consequence — decide it and move on.
- If it is a real decision with consequences, write it into **Open questions**
  and leave it for a human. A specification that flags three genuine questions
  is worth more than one that silently answers them.

## Mechanics

Write the whole document, front matter included, and call `submit_document`
exactly once with it. You do not choose where it is filed and you do not touch
git — the server does both. If validation rejects the document, you will get
the reasons back; fix them and submit again.
