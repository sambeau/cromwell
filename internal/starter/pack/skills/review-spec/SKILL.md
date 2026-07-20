---
description: Procedure for reviewing a feature specification
---

# Reviewing a specification

## Order of work

1. Read the validation report first. If mechanical checks failed, the
   submission should not have reached you — cite the failures in a
   `request_changes` verdict and stop.
2. Read the background documents and the feature context. The spec must fit
   its initiative: a spec can be internally flawless and still contradict an
   approved design above it.
3. Read the spec end to end before judging any part of it.
4. If there are unresolved comments from a prior round, check each one
   against the current text: addressed, or not? Do not re-litigate what you
   already approved; do not let an unaddressed comment slide.

## What approval requires

Approve only when every question below is a yes:

- **Clear.** Could an implementer who has never spoken to the author build
  this without guessing? Every term with a project-specific meaning is
  defined or linked.
- **Complete.** The stated scope has no silent gaps: error paths, empty
  states, and boundary conditions that the feature obviously must handle are
  specified, not implied.
- **Testable.** Each acceptance criterion is a check someone could run and
  get a yes/no answer. "Works well" is not a criterion; "returns within 2
  seconds for inputs up to 10k rows" is.
- **Consistent.** No internal contradictions, and no contradiction with
  approved background documents.

## Choosing a verdict

- `approve` — the bar above is met. Minor wording preferences are not
  grounds to withhold approval; note them in a comment if useful, then
  approve anyway.
- `request_changes` — there are specific, fixable defects. Every comment
  must name the section and state what is wrong and what would resolve it.
  The author should be able to fix the spec from your comments alone,
  without asking follow-up questions.
- `escalate` — reserve this for what genuinely needs a human: a scope or
  product decision the spec takes a position on without authority to; a
  conflict with an approved document that comments cannot resolve; a second
  round where the author disputes your comments rather than addressing
  them. Escalation must include reasoning a human can act on in one
  reading: the decision needed, the options, and what hangs on it.

Do not escalate to avoid deciding. Uncertainty about whether prose is clear
enough is your call to make — that is the job.

## Mechanics

Complete the review by calling `submit_review` exactly once. Comments go in
the `comments` array with `section_ref` set to the heading they concern;
whole-document comments leave it empty. `reasoning` is always required —
one or two sentences for approve, more for escalate.
