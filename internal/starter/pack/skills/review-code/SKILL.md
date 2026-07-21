---
description: Procedure for reviewing a task's code diff against the contract
---

# Reviewing a task's diff

## Order of work

1. Read the task, then the spec and dev-plan sections it draws on. Know what
   "done" means for this task before you judge the diff.
2. Read the diff. Then read the surrounding code the diff touches — a change
   can be locally plausible and globally wrong.

## What approval requires

- **Correct.** Does the diff do what the task asked, without bugs? Check the
  edge cases the spec calls for.
- **Complete.** Does it fully implement the task, or does it stop short?
- **In scope.** Does it change only what the task required? A diff that also
  refactors unrelated code, edits the spec or dev-plan, or starts another
  task should go back — scope creep is a defect even when the extra code is
  fine.
- **Sound.** Does the project still build and pass its tests? Does the code
  fit the surrounding style and idioms?

## Choosing a verdict

- `approve` — the diff correctly and completely implements the task, in
  scope, and leaves the project sound.
- `request_changes` — specific, fixable defects. Name the file and what is
  wrong. The implementer will amend this same diff, so be precise about what
  must change.
- `escalate` — the diff reveals a problem a human should judge: the task as
  specified cannot be implemented as written, or the right fix would exceed
  the task's scope in a way that needs a planning decision. Give the problem
  and the options.

## Mechanics

Complete the review by calling `submit_review` exactly once, with comments in
the `comments` array.
