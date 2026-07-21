---
description: Procedure for implementing one task from a dev-plan in a worktree
---

# Implementing a task

## Orient before you write

1. Read the task: exactly what is being asked, and its boundary. If the task
   description conflicts with the spec, the spec wins — implement to the
   spec and note the discrepancy in your summary.
2. `list_files` and `read_file` to learn the code you are changing. Match
   what is already there: a task is not a licence to restyle the project.
3. Plan the smallest change that fully satisfies the task.

## Write

- Change only what the task requires. Do not fix unrelated things, refactor
  surrounding code, or start the next task — those muddy the review and the
  diff.
- Read a file with `hash_tag: true` before editing it, then `edit_file` with
  the anchor. If an edit fails because the anchor drifted, re-read and try
  again. Use `write_file` for brand-new files only.
- Never edit the specification or the dev-plan. They are the contract.

## Check before you finish

- Run the project's build command via `run_command`; fix what you broke.
- Run the tests if the project has a test command. A task that leaves the
  build or tests broken is not done.

## Finish

Call `submit_implementation` once, with a summary of what you changed and the
list of files. The summary is what the code reviewer reads first — make it
tell them what to look for. Do not call it until the code builds.
