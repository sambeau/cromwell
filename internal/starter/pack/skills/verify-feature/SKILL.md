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

## Verifying a bug

When the specification is a **bug report**, its steps to reproduce are the
heart of it. For the criterion "The defect no longer reproduces", the evidence
is the reproduction followed against the fixed code — running the steps, or a
test that encodes them and now passes — and saying what happened. "The code
looks fixed" is not evidence. Check any other criteria the report lists in the
usual way.

## What is out of scope for your task

If you notice a defect that isn't among the criteria you are checking, report
it with `report_bug` and carry on; don't let it change your verdict on the
criteria. A person decides in triage whether it is fixed.

Call `submit_verification` once, with the `criteria` array (each with id,
met, and evidence) and your overall verdict.

**Evidence is required on every criterion, met or unmet, and this is enforced
rather than requested.** A verification with no criteria, or with a criterion
whose evidence is blank, is rejected and comes back to you — so does an
`approve` while any criterion is marked unmet. If you genuinely cannot judge a
criterion, `escalate` and say what you could not resolve; that is the honest
way out, and it is the only verdict exempt from the evidence rule.
