# Token estimation

Cromwell asks an agent to predict how many tokens a feature will consume, and the answer
keeps being wrong. This document asks whether that's fixable, what the research says about
why it isn't fixable the way we've been trying, and what to do instead.

Read it before changing anything about `internal/sizing`, the `estimate-work` skill, or the
corpus retrieval in `internal/store/corpus.go`. Every claim carries a source and a note on
how strongly it's evidenced. The recommendations are mine; the findings are the sources'.

---

## Table of contents

1. [The short version](#1-the-short-version)
2. [The question](#2-the-question)
3. [What the research found](#3-what-the-research-found)
4. [The points proposal, judged](#4-the-points-proposal-judged)
5. [What the state of the art actually is](#5-what-the-state-of-the-art-actually-is)
6. [Recommendations](#6-recommendations)
7. [What not to do](#7-what-not-to-do)
8. [Sources](#8-sources)

---

## 1. The short version

Four findings decide this, and they point the same way.

**Estimating a single task's tokens is close to hopeless, and that's now measured.** Frontier
models predicting their own token use on SWE-bench Verified reach a Pearson correlation of
at most 0.39, and every model tested underestimates. This isn't a prompt problem — giving
the model repo access, tool use, and a worked example didn't fix it.

**Human-perceived difficulty barely tracks agent token cost.** Kendall τb = 0.32. This is
the finding that decides the points question, because points are a difficulty scale. A
points-to-tokens conversion learned over time would be learning a weak relationship
precisely.

**Token usage is heavy-tailed and stochastic, so a point estimate is the wrong *shape* of
answer** — not merely an inaccurate one. The same agent on the same task, run again,
typically costs about twice as much on one run as another, and at the extreme 30×. No
estimator can fix variance that lives in the thing being measured.

**Ordering survives where scale collapses.** LLMs asked for story points predict *ranks*
respectably while getting the absolute scale wrong, and the fix for scale is anchors that
span the range rather than anchors that merely resemble the item.

So: keep tokens as the unit, stop treating the estimate as a number, and move the weight of
the system off prediction and onto two things that actually work — forecasting from
throughput, and controlling spend at runtime. The steel-manned version of the points idea
survives, but not as points: split the estimate into an ordering the model is decent at and
a scale factor learned from Cromwell's own corpus.

---

## 2. The question

Cromwell's `estimate-work` skill asks an agent to name a token figure for a feature or
task, informed by reference points retrieved from completed work. `internal/sizing` rolls
those figures up a tree and tags each with a confidence tier. The estimates have been poor.

The proposal on the table is to predict in a different metric — time, or abstract points —
and learn the conversion to tokens from real usage over time, storing points internally and
displaying tokens. The questions are whether that helps, whether tokens should be dropped
as a measurement, whether a hybrid is worth having, and what the state of the art is.

There is a prior constraint. Cromwell's unit of work is tokens, and money has been removed
from the human surface (DESIGN-008 D-4). Nothing here disturbs that: the argument below is
about what to *predict* and how to *represent* the prediction, not about switching units on
the human.

---

## 3. What the research found

### 3.1 Models cannot predict their own token consumption

**Evidence: strong.** This is a direct, large-scale measurement of exactly our problem.

Bai et al. (2026), from the Stanford Digital Economy Lab and Microsoft Research, ran eight
frontier models over 500 SWE-bench Verified instances in OpenHands, then repurposed each
agent to predict its own token cost before execution. The agent kept full tool access — it
could inspect the repository, run commands, and reason about execution paths first. The
prompt asked it to decompose the task into stages and estimate input and output tokens
separately, with a human-written worked example included.

| What they measured | Result |
|---|---|
| Pearson r, predicted vs actual | 0.05 to 0.39 across models |
| Best case | 0.39, Sonnet 4.5, output tokens |
| Best input-token case | 0.38, Kimi K2 |
| Direction of error | Systematic underestimation, every model |
| Where the bias is worst | Input tokens — predictions stay compressed as real values grow into the millions |

Two details matter more than the headline. First, **input tokens are consistently harder to
predict than output tokens**, because input growth is driven by context accumulation,
retrieval, and tool-driven exploration — the parts of a trajectory nobody can foresee.
Second, **the underestimation persisted when the in-context worked example was removed**
(their Appendix D), so it isn't an artefact of anchoring on the example.

The prediction is not free, either. Because predicting is itself an agentic task, the
authors report overhead as a ratio of prediction cost to task cost. Sonnet 3.7 and Sonnet 4
spent **more than 2× the task's own cost** on predicting it, and still didn't achieve the
best correlations. Sonnet 4.5 got the best correlation at 0.32× the task cost; GPT-5.2 kept
overhead below 6%. There is no monotonic relationship between what you spend predicting and
how good the prediction is.

The authors' own conclusion is that self-prediction is useful "as a coarse-grained signal of
relative cost and task difficulty" and for "early budget alerts", not for instance-level
numbers.

### 3.2 Perceived difficulty is a weak proxy for agent cost

**Evidence: strong, and decisive for the points question.**

SWE-bench Verified ships expert-estimated difficulty labels — how long a professional
developer would need (`<15 min`, `15 min–1 hour`, `1–4 hours`, `>4 hours`). Bai et al. tested
these against measured token consumption.

The association is real but modest: **Kendall τb = 0.32**. The distributions overlap heavily:

- **6.7%** of tasks labelled `<15 min` consumed more total tokens than the *average* `>1 hour` task.
- **11.1%** of `>1 hour` tasks consumed fewer tokens than the *average* `<15 min` task.

Their explanation is a Moravec's-paradox effect: tasks that look trivial to a human can
demand extensive exploration, and tasks that look hard can fall to a model that already
knows the pattern. Human effort and agent effort are different quantities that happen to
correlate loosely.

This is the finding that constrains the proposal in the brief. Time and points are both
scales of *perceived difficulty*. Predicting in one and learning a conversion to tokens
means learning a mapping across a τb = 0.32 relationship — and no amount of accumulated
real-world data makes a weak relationship strong. It makes the *conversion factor* precise
while the underlying coupling stays loose.

### 3.3 Token cost is heavy-tailed and irreducibly stochastic

**Evidence: strong for the agentic finding, medium for the distributional specifics.**

Bai et al. ran each problem four times per model:

- The most expensive problem cost about **7 million more tokens** than the cheapest.
- On the *same* problem with the *same* agent, the most expensive run typically **doubles**
  the cost of the cheapest, reaching **30×** at the extreme.
- **High-cost problems show larger cross-run variance** — agent behaviour destabilises as
  tasks get harder, so the cases where you most want a number are the cases where the number
  means least.

The popular write-ups lead with "30×", which is the extreme rather than the norm; the
typical figure is nearer 2×. That is still a floor on achievable accuracy that no estimator
can go below, because the variance is in the process, not the measurement.

The LLM-serving literature has converged independently on the distributional point, working
on output length rather than agent cost. Two 2026 papers:

- **ProD** (arXiv 2604.07931) argues output length is drawn from a prompt-conditioned
  distribution rather than being a scalar, characterises it as heavy-tailed with max-to-median
  ratios of 2.91×–4.03×, and argues the **conditional median** is the right target because
  the mean is dominated by a few long generations. Predicting a binned distribution rather
  than a point cut error by up to 25%.
- **Uncertainty-aware scheduling** (arXiv 2604.00499) fits output length with a **log-t
  distribution** (ν = 3.5), passing Kolmogorov–Smirnov goodness-of-fit on 93.1% of 1,000
  prompts, with mean skewness 3.10 and coefficient of variation 1.09. They schedule on a
  blend of the expectation and the 90% conditional value-at-risk, and beat point-estimate
  baselines by 2.31× on latency.

I verified the Bai et al. numbers against the paper text. The ProD and log-t figures come
from fetched summaries of those papers rather than a line-by-line read, so treat the exact
values as indicative and the direction as solid.

**What follows.** A point estimate is a category error against a heavy-tailed distribution.
The mean is a bad summary of one, the median is a good one, and the interesting question for
planning is a tail quantile.

### 3.4 Ordering survives where scale collapses

**Evidence: medium-strong. Verified against the paper's own tables.**

A 2026 paper on story point estimation with LLMs (arXiv 2603.06276) tested four models over
16 open-source projects. Its results split cleanly in two.

**LLMs rank better than they scale.** Zero-shot, with no training data at all, Kimi and
DeepSeek beat the supervised deep-learning baselines on both Pearson and Spearman
correlation. Kimi averaged rs = 0.4111 against the baseline regression model's 0.3133 —
and that baseline was trained on 80% of the labelled data. The authors note explicitly that
Gemini and OpenAI "struggled with the correct scales of story points for different projects
but still correctly predict the orders/ranks."

**But asking for explicit comparisons makes it worse, not better.** This is the paper's
most counter-intuitive result, and it contradicts the usual advice imported from human
estimation research. Pairwise orderings *derived* from direct numeric predictions were more
accurate than orderings the model produced when *asked to compare two items*:

| Model | Pairwise accuracy derived from direct estimates | Accuracy when asked to compare directly |
|---|---|---|
| DeepSeek | 0.7427 | 0.6306 |
| OpenAI | 0.6580 | 0.6058 |
| Kimi | 0.6486 | 0.5815 |
| Gemini | 0.5867 | 0.5562 |

Their conclusion: "LLMs do not find it easier to compare two items than evaluating one item
at a time", and may rely on a latent numerical representation even when generating
comparative decisions. Human-estimation practice — planning poker, relative sizing, t-shirts
— rests on the opposite premise, which holds for people and does not transfer.

**And anchor *selection* is a real lever.** The paper compared two few-shot strategies:
picking examples that span the project's full range of values (`Prompt 2-Scale`) against
picking examples matching the most frequent values (`Prompt 2-Count`). Range-spanning won on
average for all four models. Few-shot anchoring lifted DeepSeek from ρ = 0.3816 to 0.4532.

This is directly actionable, because `RetrieveCorpus` in `internal/store/corpus.go` currently
selects reference points by full-text rank over descriptions alone — nearest-neighbour by
similarity, with no attention to whether the retrieved set spans the range of outcomes. The
research says the second property matters and we don't have it.

### 3.5 What we are actually estimating is context accumulation

**Evidence: strong.**

Bai et al. broke cost down by token class and found **cache reads dominate both raw volume
and dollar cost in every phase** of a trajectory, by a wide margin — even though output
tokens are priced roughly 80× higher per token. The sheer accumulated context outweighs the
price difference. Non-cached input and cache-creation tokens track each other closely, as
new context enters and is immediately cached.

Cromwell's `tokenSum` in `internal/store/corpus.go` adds all four classes with equal weight:

```
COALESCE(input_tokens,0)+COALESCE(output_tokens,0)+COALESCE(cache_read_tokens,0)+COALESCE(cache_write_tokens,0)
```

That's a defensible definition of "work done", and it's consistent with counting tokens
rather than money. But it means the quantity being estimated is dominated by cache reads —
which is to say, by **how long the trajectory ran and how much context it had accumulated**,
not by how much thinking the work required. That is exactly the component the research says
is least predictable (§3.1, input tokens). We have pointed our estimator at the hardest
target.

This connects to the existing prefix-cache work: see
[prefix-cache-discipline.md](prefix-cache-discipline.md). Improving cache behaviour changes
the actuals, which invalidates the corpus. Any calibration scheme needs to cope with the
target moving when the dispatch path changes.

### 3.6 More tokens do not mean better results

**Evidence: strong.** Worth recording because it changes what an estimate is *for*.

Problems consuming more input tokens had **lower** overall accuracy, consistently across
models. Ranking four runs of the same problem by cost, accuracy **peaked at intermediate
cost** and saturated or fell beyond it. High-cost runs are dominated by redundancy —
repeated exploration, loops, re-reading.

So a large token spend is a **defect signal**, not a sign of hard work being done. An
overrunning dispatch is more likely stuck than thorough. That argues for treating estimates
as tripwires for intervention rather than as budgets to be filled.

### 3.7 The forecasting literature: count, don't score

**Evidence: medium. Strong practitioner consensus, thinner academic backing.**

Outside the LLM world, the agile forecasting community moved away from estimating items
individually. The argument: throughput — how many items actually completed per period — is
measured rather than guessed, and already embeds all the complexity that estimation tries to
anticipate. Monte Carlo simulation over historical throughput reportedly outperforms
estimate-derived velocity.

This has the same shape as **reference class forecasting** (Flyvbjerg), which consistently
beats bottom-up expert estimation on large projects by taking an "outside view" — the
distribution of comparable past cases — instead of reasoning up from the parts. The caution
in the literature is real: the reference class has to be genuinely similar, and a
poorly-vetted class is worse than judgement.

Jørgensen's reviews of software effort estimation give the other half. Expert judgement beat
formal models in ten of sixteen studies, and the two conditions that favoured experts were
**models not calibrated to the organisation using them** and **contextual information the
model didn't have**. His recommendation is that formal models should support expert judgement
rather than replace it.

That is a good description of where Cromwell should land: a corpus-calibrated outside view,
with the agent's judgement layered on it, not either alone.

---

## 4. The points proposal, judged

The proposal was to estimate in time or points, store internally in that metric, display
tokens, and learn the conversion from real usage.

**The core mechanism doesn't survive §3.2.** Points and time are difficulty scales, and
expert-rated difficulty correlates with agent token cost at τb = 0.32 with heavily
overlapping distributions. Learning the conversion doesn't help, because the problem isn't
an unknown constant of proportionality — it's that the relationship is weak and noisy. You'd
be fitting a precise multiplier to a loose coupling and inheriting all the noise, plus a new
layer of indirection to explain.

There's a second cost. Tokens are **measured for free** from the dispatch ledger, on every
dispatch, exactly. Points would have to be inferred and could never be observed directly, so
the feedback loop that makes calibration possible would get strictly weaker. Cromwell's one
genuine advantage here — a complete, honest record of actuals — argues for keeping the
predicted quantity and the measured quantity the same.

**But the instinct underneath it is right, and it's worth naming precisely.** The proposal
correctly identifies that the *number the model produces* and the *number we should show*
are different things, and that the bridge between them should be learned from real data
rather than trusted. §3.4 says why: the model's ordering is worth something and its absolute
scale isn't. So keep the two-representation idea and change what the internal representation
is. Not points — an **uncalibrated model estimate plus a scale correction learned from the
corpus**. Same architecture, evidenced content.

**Should tokens be dropped as a measurement?** No. Tokens are the honest unit of agent work
(DESIGN-008 D-4), they're measured exactly and free, and every alternative is a proxy for
them. What should be dropped is the *single scalar* — the estimate should be an interval, not
a number.

**Is a hybrid worth considering?** Yes, and it's the recommendation. Not a hybrid of metrics,
a hybrid of methods: an outside-view forecast from throughput as the default, the agent's
judgement as a refinement and an outlier detector, and runtime control as the real safety
net.

---

## 5. What the state of the art actually is

There isn't a mature state of the art for predicting agentic task cost. Bai et al. is the
first systematic study of the problem and its finding is essentially negative. What exists
is a set of transferable techniques from adjacent fields:

| Technique | Where it's proven | Applicability here |
|---|---|---|
| Distributional prediction — median plus quantiles, not a point | LLM serving (ProD, log-t scheduling) | High. Directly matches the heavy-tailed shape. |
| Conformal prediction / conformalised quantile regression | General ML, well established | High. Gives interval coverage guarantees from a calibration set, distribution-free, wraps any underlying predictor. |
| Reference class forecasting | Megaproject planning | High. Cromwell's corpus is a reference class. |
| Monte Carlo throughput forecasting | Agile practice | High, and cheap — we have per-task actuals. |
| Learned predictors from task features | LLM serving (proxy models, learning-to-rank) | Low near-term. Needs training data volume we won't have. |
| Runtime budget enforcement and early termination | Production practice, and Bai et al.'s own recommendation | High. This is where the leverage is. |

The honest summary is that the field's answer to "we can't predict this" has been to stop
trying to predict it precisely and to build controls that make bad predictions survivable.
Bai et al. say as much: consumption-based pricing "will likely stay the most practical option
until pre-execution estimation becomes more reliable", with budget-aware runtime policies to
contain volatility.

---

## 6. Recommendations

Ordered by value for effort. The first three are the substance; the rest are cheaper
follow-ons.

### R1. Replace the scalar estimate with an interval

**Stop storing one number.** Store a median and a spread — at minimum p50 and p80, and
prefer that the model produce them directly. Display a range in the UI. This is the single
change that most improves honesty, because it stops the system from making a claim it can't
support.

The confidence tiers in `internal/sizing` (`decomposed` / `considered` / `rough`) are
currently an ordinal stand-in for exactly this. They can stay as a provenance label, but
interval width is the real quantity and should be derived from evidence rather than asserted.

This changes `RollUp`. Summing medians up a tree is wrong: the median of a sum is not the sum
of medians for skewed distributions, and the p80 of a sum is emphatically not the sum of p80s
— adding tail quantiles compounds conservatism at every level. Roll up by **Monte Carlo over
the tree** instead: sample each leaf from its distribution, sum, repeat, and read the
quantiles off the result. `internal/sizing` is pure with no I/O, so this is a
self-contained change with a testable core.

**Cost.** A real change to the sizing engine and its stored schema, plus UI work. This is the
expensive recommendation, and it's the one worth paying for.

### R2. Forecast from throughput, not from per-item estimates

**Make the outside view the default.** Cromwell already records exact actuals per completed
task. That gives an empirical distribution of tokens-per-task which needs no estimation at
all. To forecast a feature: take its task count, sample that many draws from the historical
distribution, sum, repeat. Same for an initiative.

This is likely to beat per-task LLM estimation outright, for the reason §3.1 gives — the
model's per-item correlation tops out around 0.39, whereas the historical distribution is
*measured*. It also degrades gracefully: a feature that hasn't been decomposed yet can be
forecast from the distribution of tokens-per-feature instead.

Segment the distribution where the data supports it — by role, by task kind, by whether the
work is greenfield or a change — but only once there are enough samples per segment to mean
anything. The reference-class caution in §3.7 applies: a class that isn't genuinely similar
is worse than judgement.

**Cost.** Low. The data exists; this is a query and a simulation. Highest value per unit of
work on this list.

### R3. Move the weight onto runtime control

Given r ≤ 0.39, **an estimate is a tripwire, not a budget.** The useful products of an
estimate are: flag this feature as unusually expensive before starting it, alert when a
dispatch passes its predicted range, and stop a run that's clearly looping.

§3.6 sharpens this — high token consumption correlates with *lower* accuracy, so an overrun
is evidence of a stuck agent, not a thorough one. An overrun should trigger inspection, not
an automatic budget increase.

Two existing defects are really instances of this, both already recorded in the project's
research conformance notes: the **uncapped review loop** (`returnTaskCode` re-dispatches the
implementer with no round counter) and the **dollar-denominated runaway cap** in
`internal/config` `BudgetConfig.cap_usd`, which is money machinery on a token system and is
listed as open in DESIGN-008 §12 (Q-C). A per-task token budget with a hard round cap would
close both and is more valuable than any improvement to estimate accuracy.

**Cost.** Low to moderate, and it addresses a live gap rather than a hypothetical one.

### R4. Calibrate the model's number instead of trusting it

This is the surviving, evidenced form of the points proposal. Treat the agent's figure as an
**uncalibrated ordinal signal**, and learn the correction from the corpus.

Concretely: record the ratio of actual to estimate for every completed entity, and maintain
the distribution of that ratio. Apply its median as a multiplier and its spread as the basis
for the interval in R1. Since §3.1 establishes that models *systematically* underestimate,
this correction has a known sign and should recover a meaningful part of the error for
almost no work.

Keep the internal and displayed values distinct exactly as the proposal suggested — the
stored raw estimate, the applied correction, and the displayed calibrated interval should all
be inspectable, so a bad calibration is visible rather than mysterious.

Two cautions. Recalibrate when the dispatch path changes, because §3.5 means cache behaviour
moves the actuals. And the ratio distribution is itself heavy-tailed, so use the median
ratio, never the mean.

**Cost.** Low. A query over `estimates` joined to actuals, and a multiplier.

### R5. Fix anchor selection in the corpus retrieval

`RetrieveCorpus` ranks by `ts_rank` over descriptions and returns the top 5. §3.4 says
few-shot anchors should **span the range of outcomes**, and that range-spanning selection
beat frequency-matched selection for every model tested.

Change the selection to take a similar set *and* deliberately include high and low actuals,
so the agent sees the scale it's estimating on rather than a cluster. Cheap, and it's a
concrete gap between what the research recommends and what the code does.

While in there: the retrieval currently rewrites `plainto_tsquery` into an OR query and ranks
by overlap, which is a reasonable nearest-neighbour proxy but will return thin matches when
the corpus is small. Returning *few but well-spread* anchors is better than returning five
loosely-related ones.

**Cost.** Low. One query change.

### R6. Ask for the estimate more cheaply

§3.1 found no monotonic relationship between prediction overhead and accuracy — Sonnet 4.5
achieved the best correlation at 0.32× task cost while older models spent over 2× for worse
results. If Cromwell's estimation dispatches let an agent explore the repository extensively
before answering, that exploration is probably not buying accuracy.

Given the target project's ~1.3M-token corpus, an estimation pass that reads widely is
expensive in absolute terms. Prefer a bounded estimation dispatch with retrieved anchors over
an open-ended one with repo access.

**Cost.** Low. Mostly a matter of what the estimation dispatch is allowed to do.

### R7. Measure our own calibration

None of the above is worth much unless we can tell whether it's working. Cromwell has the
data to score itself, which most systems don't. Track, over completed work:

- **Interval coverage** — what fraction of actuals fell inside the predicted range. If p80
  intervals contain 80% of outcomes, the system is honest.
- **Spearman correlation** between estimate and actual — the ordering quality, which §3.4
  says is the part worth having.
- **Median ratio of actual to estimate** — the bias, and the input to R4.

Report these somewhere visible. A system that publishes its own calibration record earns
more trust from an honest wide interval than from a confident wrong number.

**Cost.** Low, and it should probably come first, since it establishes the baseline the other
changes are measured against.

---

## 7. What not to do

**Don't switch the estimated quantity to points or time.** §3.2. It adds indirection,
weakens the feedback loop, and learns a conversion across a τb = 0.32 relationship.

**Don't ask the model for pairwise comparisons.** §3.4 tested this directly and found
explicit comparison *worse* than direct estimation for all four models. The intuition is
imported from human estimation research and doesn't transfer. This is the most likely wrong
turn on the list, because it sounds right.

**Don't try to train a predictor from task features yet.** The serving literature does this
successfully, but with training-set sizes we won't have. Bai et al. deliberately didn't
attempt it, arguing that a separate predictor would have to reconstruct context the agent
already has.

**Don't spend more on estimating.** §3.1 and R6 — the extra spend doesn't buy accuracy, and
the money-for-accuracy trade is the trap this whole area invites.

**Don't chase a better estimation prompt as the primary fix.** The failure is measured across
eight frontier models with full tool access and worked examples. Prompt improvements operate
inside a ceiling of about r = 0.39.

---

## 8. Sources

Grouped by how much weight the recommendations put on them.

### Load-bearing

**Bai, Huang, Wang, Sun, Mihalcea, Brynjolfsson, Pentland, Pei (2026). "How Do AI Agents
Spend Your Money? Analyzing and Predicting Token Consumption in Agentic Coding Tasks."**
arXiv 2604.22750. Stanford Digital Economy Lab / Microsoft Research. Eight frontier models,
500 SWE-bench Verified instances, OpenHands, four runs per problem.
*Evidence: strong.* Verified against the paper text, not a summary. Limitation the authors
state: eight models is a slice of the landscape, and trajectory collection cost constrained
the sample.

- [arXiv](https://arxiv.org/abs/2604.22750) ·
  [Stanford DEL](https://digitaleconomy.stanford.edu/publication/how-do-ai-agents-spend-your-money-analyzing-and-predicting-token-consumption-in-agentic-coding-tasks/) ·
  [Microsoft Research](https://www.microsoft.com/en-us/research/publication/how-do-ai-agents-spend-your-money-analyzing-and-predicting-token-consumption-in-agentic-coding-tasks/) ·
  [Stanford summary](https://digitaleconomy.stanford.edu/news/how-are-ai-agents-spending-your-tokens/)

**"Story Point Estimation Using Large Language Models."** arXiv 2603.06276. Four LLMs, 16
open-source projects, four prompt strategies.
*Evidence: medium-strong.* Verified against the paper's result tables. Source of the
ordering-versus-scale finding, the negative result on pairwise comparison, and the
range-spanning anchor result. Limitation: story points are a human difficulty scale, so it
transfers to the *method* question, not to token cost directly.

- [arXiv PDF](https://arxiv.org/pdf/2603.06276)

### Supporting

**"Robust Length Prediction: A Perspective from Heavy-Tailed Prompt-Conditioned
Distributions."** arXiv 2604.07931. Heavy-tailed output length; conditional median as the
robust target; distributional prediction cuts error up to 25%.
*Evidence: medium.* Read via fetched summary rather than full text. Different domain — output
length in serving, not agent cost — so the direction transfers more reliably than the numbers.

- [arXiv](https://arxiv.org/html/2604.07931)

**"Scheduling LLM Inference with Uncertainty-Aware Output Length Predictions."**
arXiv 2604.00499. Log-t distribution fit, 93.1% KS pass rate, expectation blended with
CVaR@90.
*Evidence: medium.* Same caveats as above.

- [arXiv](https://arxiv.org/html/2604.00499v2)

**Romano, Patterson, Candès (2019). "Conformalized Quantile Regression."** The standard
method for distribution-free prediction intervals with finite-sample coverage guarantees,
wrapping any underlying predictor.
*Evidence: strong as method, untested here.* Cited as the principled route to R1's intervals
if the simple empirical approach proves insufficient.

- [arXiv](https://arxiv.org/abs/1905.03222)

**Jørgensen, M. "A Review of Studies on Expert Estimation of Software Development Effort."**
*Journal of Systems and Software* 70, 37–60. Expert judgement beat formal models in ten of
sixteen studies; the favouring conditions were uncalibrated models and expert-held context.
*Evidence: strong but dated, and about human effort.*

**Flyvbjerg on reference class forecasting.** The outside view beats bottom-up expert
estimation on large projects. Recent review of its limits: Tandfonline,
[10.1080/09537287.2025.2578708](https://www.tandfonline.com/doi/full/10.1080/09537287.2025.2578708).
*Evidence: strong in its domain, analogical here.*

### Practitioner consensus

Monte Carlo throughput forecasting over story-point velocity — Magennis and others. Widely
adopted, thin academic evidence, and the accuracy claims made for it are not independently
verified. *Evidence: weak-to-medium.* Cited for the method in R2, not for its accuracy claims.

- [Introduction to Monte Carlo Forecasting](https://observablehq.com/@troymagennis/introduction-to-monte-carlo-forecasting) ·
  [Monte Carlo forecasting in Scrum](https://www.scrum.org/resources/blog/monte-carlo-forecasting-scrum)

### Noted and set aside

Budget-enforcement tooling — gateway-level token budgets, per-session caps, circuit breakers,
model-tier routing — is now a standard production pattern and is what R3 amounts to. The
2026 literature here is mostly vendor material and single-author preprints; the pattern is
well-established, the published evidence is weak, and Cromwell needs a small amount of it
built rather than a survey.

---

## Related

- [prefix-cache-discipline.md](prefix-cache-discipline.md) — why cache behaviour moves the
  actuals that any calibration depends on.
- [agent-research-evidence-base.md](agent-research-evidence-base.md) — the wider findings on
  how agents are prompted and arranged.
- `docs/design/DESIGN-001-data-model-and-schema.md` §7 — the estimate and roll-up model these
  recommendations would change.
- `docs/design/DESIGN-008-the-workflow-surface.md` D-4 and §12 Q-C — tokens as the unit, and
  the open question about the dollar-denominated runaway cap.
