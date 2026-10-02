# Walkthrough — SPEC-021, spikes

**Date:** 2026-10-02
**Spec:** [SPEC-021](specs/SPEC-021-spikes.md), draft for Sam's approval
**Review:** [REVIEW-021](reviews/REVIEW-021-spikes.md)
**Handoff:** [M14 handoff](notes/handoff-M14-2026-10-02.md)

This is the definition-of-done demo (SPEC-021 DoD 8). It ran a throwaway
project on a real Postgres **with no AI provider**. The project lived at
`/var/tmp/m14demo`, a short path, because a unix socket path can't be longer
than 108 bytes. The person's side ran in the browser, with Playwright and the
pre-installed Chromium. The chat agent's side ran as plain MCP JSON-RPC calls,
which is what a chat AI sends.

**How a spike ran without a model.** The provider's `base_url` points at
[`fake_provider.py`](walkthrough-spec-021/fake_provider.py), a small HTTP
server that speaks the Messages API and plays the spike's agent from a
script. It isn't a model, and it decides nothing about Subutai's behaviour:
- a first spike saves a draft with `save_findings`, then keeps listing files
  and never finishes, so it runs into its budget;
- a spike that asks again (its prompt holds "# What the last spike found")
  saves its findings and finishes with `finish_spike`.

Its token counts are a quarter of each request's size, so the totals grow
the way a real conversation's do. Everything else, from the start screen to
the hard stop, the write-up, the discarded worktree and the close, is the
real server.

The scripts are in [walkthrough-spec-021/](walkthrough-spec-021/):

```sh
bash docs/walkthrough-spec-021/setup.sh
node docs/walkthrough-spec-021/walk.js docs/walkthrough-spec-021
```

The chat agent's calls are kept in
[mcp-output.txt](walkthrough-spec-021/mcp-output.txt), and the demo
repository's history, worktrees and branches after the walk in
[repo-after.txt](walkthrough-spec-021/repo-after.txt).

## What the chat agent can and can't do

The chat agent plans an initiative and a feature, and writes a spike's
question down with `create_spike`. The spike is `SPK-001`, an idea, with the
project's default budget of 1,000,000 tokens. It then tries `start_spike`,
and the server answers "there is no tool called "start_spike" on this
server". Starting a spike spends tokens, so only a person can do it, in the
web UI (SD-5).

## 1. A person writes a spike down

On the feature's page, **New spike** opens a dialog with the question and an
optional budget. Sam asks whether the list endpoint is fast enough to fetch a
10,000-item list whole, with a budget of 20,000 tokens.

![The New spike dialog](walkthrough-spec-021/01-new-spike-dialog.png)

The feature's page now lists both spikes in its **Spikes** section.

![Spikes on the feature](walkthrough-spec-021/02-spikes-on-the-feature.png)

The spike's own page says it hasn't started yet, and offers **Start this
spike…** and **Close without running**.

![The spike, not started](walkthrough-spec-021/03-spike-not-started.png)

## 2. The start screen

Before anything is committed, the start screen shows the question, who will
run it ("The spike runner runs it, on claude-sonnet-5."), the free agent
slots, the forecast (there aren't enough earlier spikes yet), the budget and
what happens when it runs out, what the agent will be given (the question,
the decisions that apply, and its tools), and where it works: a throwaway
copy of the code that is discarded when the run ends and is never merged.

![The start screen](walkthrough-spec-021/04-start-screen.png)

![The start screen, lower](walkthrough-spec-021/05-start-screen-budget.png)

## 3. It runs, and stops hard at its budget

Pressing **Start** records the budget, moves the spike to running and queues
its run, then redirects to its page: "SPK-002 has started, with a budget of
20,000 tokens."

![Started](walkthrough-spec-021/06-started.png)

The scripted agent saves a draft on its first turn, then keeps working and
never finishes. The run stops after seven calls, at 19,944 tokens, because the
next call could only have taken it over 20,000. The page says the run has
ended at its budget and is waiting for Sam, shows the tokens against the
budget, links the findings, and says the working copy was discarded.

![Stopped at its budget](walkthrough-spec-021/07-stopped-at-its-budget.png)

![Tokens and findings](walkthrough-spec-021/08-tokens-and-findings.png)

The findings are an ordinary draft document, `SPK-002-findings`. They hold the
question, the draft the agent saved, and the server's **How this spike
ended**: "The spike stopped at its budget, because its next step would have
gone over. It used 19,944 of its 20,000 tokens. The findings above are what
it had saved by then."

![The findings at the stop](walkthrough-spec-021/09-findings-at-the-stop.png)

The run's transcript ends with the stop: "The run stopped here because it
reached its budget of 20,000 tokens."

![The run, stopped](walkthrough-spec-021/10-run-stopped.png)

## 4. Asking again, with a second budget

The findings don't answer the question. Sam enters a new budget of 60,000 and
presses **Ask again with a new budget**. That closes `SPK-002` without an
answer, writes down `SPK-003` with the same question, and opens its start
screen, which shows that the agent will be given what the last spike found.

![Ask again](walkthrough-spec-021/11-ask-again.png)

![The second spike's start screen](walkthrough-spec-021/12-second-spike-start-screen.png)

Sam starts it. This time the agent finishes, at 4,236 tokens.

![The second spike concluded](walkthrough-spec-021/13-second-spike-concluded.png)

![Its findings](walkthrough-spec-021/14-findings-concluded.png)

## 5. Closing it

Sam reads the findings and presses **The question is answered**. The spike is
closed, and its page says how to build on it: "To build on this, cite
SPK-003-findings in a design, then create a feature in the normal way."
Closing doesn't approve the findings; they keep their own lifecycle.

![Closed as answered](walkthrough-spec-021/15-closed-answered.png)

## 6. Where spikes show

On the feature's timeline, each spike's start and end is a muted aside. The
feature's own moment, Created, stays current, and its runs and tokens don't
include the spikes'.

![The feature's timeline](walkthrough-spec-021/16-feature-timeline.png)

`/ui/spikes` lists every spike, open ones first.

![The spikes list](walkthrough-spec-021/17-spikes-list.png)

Finally the chat agent reads `SPK-003` with `get_spike`: closed as answered,
concluded, 4,236 tokens of 60,000, following `SPK-002`, with its findings'
ID, path and state.

## 7. No code was kept

After the walk, the demo repository's history holds the project's own commits
and the two findings commits, and nothing else. `git worktree list` shows only
the main checkout, and the only branch is `master`
([repo-after.txt](walkthrough-spec-021/repo-after.txt)).

## What the demo doesn't show, and where it is proved instead

- **The leak check**: a run that keeps code on a branch, a tag or a stash, or
  that deletes its own working copy. That is `TestSpikeHasNoMergePath`, with a
  subtest for each way.
- **Turn limits, failures, retries and restarts**: `TestSpikeTurnLimitEndsTheRun`,
  `TestSpikeWorktreeIsDiscardedWhenItEnds`,
  `TestExhaustedSpikeEndsWithoutRetryQuestion`,
  `TestSpikeBudgetCarriesAcrossAttempts` and `TestSpikeEndIsReconciled`.
- **Decisions in the prompt**: `TestSpikePromptCarriesDecisionsAndQuestion`.
  The demo project has no decisions, so its start screen says none apply.
