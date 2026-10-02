---
description: Procedure for running a spike, answering one question in a throwaway copy
---

# Running a spike

## Answer the question

1. Read the question, and what the prompt says about where it came from.
   Answer that question and only that question. If you notice something
   else worth knowing, put one line about it in "What to do next" and carry
   on.
2. If the prompt shows findings from an earlier attempt, build on them
   rather than starting again.

## Work in the throwaway copy

- Your working copy is scaffolding. Read, edit, write and run whatever helps
  you find out; none of it is kept or merged.
- Never try to keep the code. Don't make branches, don't commit to keep
  anything, and don't push.
- Never write your findings into a file in the working copy. Findings go in
  `save_findings` and `finish_spike`, and nowhere else.

## Save as you go

The run can stop without warning at any call: at the budget, at the turn
limit, or on a failure. Whatever you have saved is what is kept. So call
`save_findings` after each thing you learn, not once at the end. Each call
replaces the last, so always send the whole of the findings so far.

Write these sections, in this order, and leave out any you have nothing to
put in:

- **Answer**: short and direct. One or two sentences that someone could act
  on. While you don't have an answer yet, say what you know so far.
- **What we found**: the facts, as plainly as you can give them. What you
  saw, with the file, the command or the number.
- **How we found out**: what you read and ran, so a person can check it.
- **What to do next**: what you would do with this answer, and any open
  question it leaves.

Don't write a "Question" section; the server adds it. Don't write "How this
spike ended" either; the server writes that last.

## Say how sure you are

Say how sure you are of the answer, and why. Say plainly when the evidence is
thin: when you ran something once, when you read the code but didn't try it,
or when the question turned on something you couldn't reach. A clear "I don't
know, and here is what would tell us" is a good finding.

## Finish

Call `finish_spike` once, when you have an answer, or when you can say why the
question can't be answered. Its `findings` input holds the sections after
Question, as above. If the body is invalid you are told why; fix it and call
again.
