---
description: Procedure for estimating the token cost of a feature or task
---

# Estimating work in tokens

## The unit

You estimate **tokens** — the tokens the AI agents will consume carrying this
work from contract to merged code: the implementer's context and edits, the
code reviewer's reading, the verifier's checks, and the rework these prompt.
Not wall-clock time, not human hours. Tokens.

## Use the reference points

You are given reference points from completed work: for each, its
description, what it was estimated at, and what it actually consumed. These
are your calibration.

1. Find the reference points whose work most resembles what you are
   estimating — similar shape, similar surface area, similar uncertainty.
2. Note where their **actuals** diverged from their estimates. If similar
   work consistently ran over, expect the same here.
3. Reason from the nearest anchors to a number for this work, adjusting for
   how it is bigger or smaller than them. Say which anchors you used.

If you were given no reference points, the corpus has nothing similar yet.
Estimate from judgement, and say so — this is a rougher number, and the
system records it as such.

## Reason honestly

- A feature broken into well-understood parts is more predictable than one
  described in a sentence. Reflect that in how tight your number is.
- Do not anchor on a round number because it feels safe. Anchor on the
  evidence.
- Over-estimating hides slippage as much as under-estimating does. Aim for
  the honest middle, not a comfortable cushion.

## Mechanics

Call `submit_estimate` exactly once, with `tokens` (a positive whole number)
and `rationale` — your reasoning, naming the reference points you leaned on.
Do not state a confidence tier; the system assigns it from your evidence.
