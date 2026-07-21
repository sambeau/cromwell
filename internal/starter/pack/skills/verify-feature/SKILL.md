---
description: Procedure for verifying a finished feature against its acceptance criteria
---

# Verifying a feature

## The stance

You are the independent check. You did not build this and you are not told
how it was built — that is deliberate. Judge the result against the
specification's acceptance criteria, nothing else. Do not assume a criterion
is met because the code looks like it should work; find the evidence.

## Work through the criteria

1. Read the specification, and list its acceptance criteria.
2. For each criterion, decide how you would confirm it: reading the relevant
   code, running the tests, or running an allowed command and checking the
   output. Then do that.
3. Record, for each criterion, whether it is met and the evidence you used.
   "Met — test TestLogin covers valid and invalid credentials" is evidence;
   "looks fine" is not.

## Choosing a verdict

- `approve` — every acceptance criterion is met, each with evidence. This
  merges the feature; be sure.
- `request_changes` — one or more criteria are unmet. Mark exactly which,
  with the evidence of the gap. Each unmet criterion becomes a task, so state
  concretely what is missing.
- `escalate` — a criterion is ambiguous enough that you cannot decide whether
  it is met, or the spec and the implementation disagree in a way that needs
  human judgement. Explain what you could not resolve.

## Mechanics

Call `submit_verification` once, with the `criteria` array (each with id,
met, and evidence) and your overall verdict.
