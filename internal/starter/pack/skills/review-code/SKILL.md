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

## Classify every finding

Each comment carries a severity, and the severity — not your patience —
decides whether the work goes back.

- **major** — one of the four bars above is not met: it is wrong, incomplete,
  out of scope, or it breaks the build or tests. The work must change.
- **minor** — style, naming, a tidier way to express something. Worth saying,
  not worth another round.

Classify honestly in both directions. Marking everything major stalls the
task until a human has to break the tie; marking a real defect minor lets it
through, and nobody downstream will catch it for you. If you are unsure
whether something is major, ask whether you would be comfortable shipping it
as it stands — if not, it is major.

## Choosing a verdict

The verdict follows from the findings; it is not a separate judgement.

- `approve` — no major findings. Say so **even if minor findings remain**:
  minor findings are recorded and do not send work back. A round of review
  that finds only minor things is a round that is finished.
- `request_changes` — at least one major finding. Name the file and what is
  wrong. The implementer will amend this same diff, so be precise about what
  must change.
- `escalate` — the diff reveals a problem a human should judge: the task as
  specified cannot be implemented as written, or the right fix would exceed
  the task's scope in a way that needs a planning decision. Give the problem
  and the options.

## Mechanics

Complete the review by calling `submit_review` exactly once, with comments in
the `comments` array.

## What is out of scope for your task

You will sometimes notice a defect that this task's diff didn't cause:
a bug in code nearby, a wrong message, a missing check. **Don't fix it in
passing**, and don't hold this task's verdict hostage to it: judge the diff against its task. Report it with
`report_bug` — a title, steps someone else could follow, what should happen
and what happens instead — and carry on. It goes into the triage queue, where
a person decides whether it is fixed. Report only real defects you can
describe; a run may file at most three.
