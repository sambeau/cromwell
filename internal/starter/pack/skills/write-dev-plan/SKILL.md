---
description: Procedure for decomposing an approved specification into buildable tasks
---

# Writing a dev-plan

The tasks you write are built by separate implementer agents, each working
alone in a worktree with the specification, the plan, and nothing else. None
of them sees the others' work. A weak decomposition therefore produces weak
code no matter how good the specification was — the quality of this breakdown
is the strongest single predictor of how the feature turns out.

## Order of work

1. Read the specification end to end, then its acceptance criteria again on
   their own. Those criteria are what the finished feature is checked against.
2. Read the design for the intent behind the specification.
3. Find the seams — the boundaries the work naturally splits along. Layers,
   interfaces, behaviours. Not files.
4. Write the tasks, then the ordering.
5. Before you finish, map every acceptance criterion to the task that
   satisfies it. A criterion with no task is the gap the reviewer will find.

## What each task needs

- **Small enough to build alone.** One sitting, one agent, no need to read
  three other tasks to understand it.
- **A description that stands by itself.** The implementer has the spec and
  the plan, not your reasoning.
- **Honest dependencies.** Depend on another task only where this one
  genuinely cannot be built or tested first. Two tasks that edit the same
  file must be ordered, or they will conflict.

## Ordering

Dependencies are a build order, not a wish. The engine dispatches every task
whose dependencies are met, in parallel, so:

- an ordering you did not need costs parallelism on every run;
- an ordering you left out costs a conflict, and surfaces as a confusing code
  review rather than as the planning mistake it was.

## Mechanics

Write the whole document, front matter included, with the Tasks section as the
table the template defines — the engine parses that table into real tasks, so
its columns and ids are load-bearing. Call `submit_document` exactly once. You
do not choose the path and you do not touch git. If validation rejects it, fix
what it reports and submit again.
