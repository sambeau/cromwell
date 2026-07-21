---
description: Procedure for reviewing a dev-plan (decomposition of a spec into tasks)
---

# Reviewing a dev-plan

## Order of work

1. Read the validation report. The task table already parsed (unique ids,
   no dangling dependencies, no cycles) or you would not be reviewing —
   so judge substance, not table hygiene.
2. Read the approved specification it decomposes. The plan's job is to build
   exactly that spec.
3. Read the Approach, then the task table.

## What approval requires

- **Coverage.** Do the tasks together implement the whole spec? Walk the
  spec's acceptance criteria: each one should be satisfiable by the tasks as
  written. A criterion no task addresses is a gap.
- **Task shape.** Is each task small and self-contained enough for one agent
  to build in isolation, given only the spec, the dev-plan, and that task?
  A task that says "build the feature" is too big; a task that cannot be
  understood without three others is poorly bounded.
- **Ordering.** Do the dependencies reflect real build order — a task
  depends on another only when it genuinely needs that work first? Missing
  dependencies cause an agent to build against something that isn't there
  yet; spurious ones serialise work that could be parallel.
- **Soundness.** Is the Approach a reasonable way to build this, with no
  structural decision that contradicts the spec or an approved design?

## Choosing a verdict

- `approve` — the decomposition is sound and complete. Minor task-wording
  preferences are not grounds to withhold approval.
- `request_changes` — specific, fixable problems: a coverage gap, a task too
  large to build alone, a wrong or missing dependency. Name the task id and
  say what to change.
- `escalate` — a decomposition question that needs human judgement: the plan
  reveals the spec is ambiguous about build order in a way you cannot
  resolve, or it takes a structural approach whose trade-off a human should
  own. Give the decision, the options, and what hangs on it.

## Mechanics

Complete the review by calling `submit_review` exactly once. Put task-level
comments in the `comments` array with `section_ref: "Tasks"`.
