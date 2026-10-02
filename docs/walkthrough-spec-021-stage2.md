# Walkthrough — SPEC-021 stage 2, spikes run in chat or by hand

**Date:** 2026-10-02
**Spec:** [SPEC-021](specs/SPEC-021-spikes.md) §4 (FR-12 to FR-16), stage 2 definition of done
**Stage 1:** [walkthrough-spec-021.md](walkthrough-spec-021.md)

This ran a throwaway project on a real Postgres **with no AI provider**, on the
branch `claude/subutai-m14-stage2-spikes`. The project lived at
`/var/tmp/m14s2demo`, a short path because a unix socket path can't be longer
than 108 bytes. The person's side ran in the browser, with Playwright and the
pre-installed Chromium. The chat agent's side ran as plain MCP JSON-RPC calls,
which is what a chat AI sends.

Stage 2 spikes have no run to script, so nothing here is faked: the chat agent
and the person do the work themselves. `fake_provider.py` is still started, and
the config's `base_url` still points at it, but only so the server boots;
nothing is dispatched to it. To make a time box end in seconds rather than
hours, the walkthrough shortens `server.heartbeat_seconds` to 5 and backdates
two timestamps in SQL (steps 6 and 7).

The scripts are in [walkthrough-spec-021-stage2/](walkthrough-spec-021-stage2/):

```sh
bash docs/walkthrough-spec-021-stage2/setup.sh
node docs/walkthrough-spec-021-stage2/walk.js docs/walkthrough-spec-021-stage2
```

The chat agent's calls, with their full results, are in
[mcp-output.txt](walkthrough-spec-021-stage2/mcp-output.txt), and the demo
repository's history, worktrees and branches after the walk in
[repo-after.txt](walkthrough-spec-021-stage2/repo-after.txt).

Four spikes were used:

| Spike | Written by | Run by | How it ended |
|---|---|---|---|
| SPK-001 | the chat agent | the chat agent | submitted, then closed as answered |
| SPK-002 | a person, on the feature's page | the person | submitted, then closed as answered by the person who ran it |
| SPK-003 | the chat agent | the chat agent | the time box ran out |
| SPK-004 | the chat agent | the chat agent | still running, with a claim-stale question in the Inbox |

## 1. The chat agent plans; a person adds a spike

The chat agent creates the initiative `imports`, the feature `bulk-import` and
`SPK-001` with `create_spike`. The result says the spike is an idea and that "A
person can start it from its page in the web UI." A person then opens **New
spike** on the feature's page and writes `SPK-002`.

![The New spike dialog](walkthrough-spec-021-stage2/01-new-spike-dialog.png)

![Both spikes on the feature](walkthrough-spec-021-stage2/02-spikes-on-the-feature.png)

## 2. The start screen, for the chat agent

On `SPK-001`'s start screen, **Who runs it** offers three choices in the
order the spec gives: **The spike runner** ("An agent runs it on the
configured model, with a token budget."), **The chat agent** and **You**. The
chat agent is chosen here. The budget field has been replaced by **Time box,
in hours**, filled with the project's default of 4, and the page says: "Work
done in chat or by hand can't be measured in tokens, so this spike is held to a
time box instead. When the time box runs out, the spike ends with whatever
findings have been saved." The panels below say they describe the spike runner.

![The start screen, with the chat agent chosen](walkthrough-spec-021-stage2/03-start-screen-chat.png)

Pressing **Start this spike** gives the notice "SPK-001 has started, with a
time box of 4 hours. Ask the chat agent to run it." The executor line reads "To
be run in chat. Waiting for the chat agent to claim it.", and the Running it
panel tells the person to ask the chat agent, which "claims the spike with
`claim_spike`".

![Started, waiting for the chat agent](walkthrough-spec-021-stage2/04-started-in-chat.png)

## 3. The chat agent claims it

The chat agent tries `start_spike` and `close_spike` and the server answers
"there is no tool called ... on this server" for each (JSON-RPC -32601), so a
person is still the only one who starts or closes a spike. `claim_task` given
the spike's ID is refused with "SPK-001 is a spike, not a task. Use claim_spike
to run it."

`claim_spike` then returns:
- `working_copy`: `/var/tmp/m14s2demo/.subutai/worktrees/spk-353dd2`, detached,
  at the base commit;
- `contract`: three keys, `question`, `where_it_came_from` and
  `findings_template` (six sections, each with what it is for and whether it is
  required). `decisions`, `earlier_findings` and `draft` are left out because
  this spike has none;
- `deadline`: `2026-10-02T18:37:13Z`, and `time_left`: "The time box ends at
  18:37 UTC, in 3 hours 59 minutes. Then the spike ends with whatever findings
  you have saved.";
- `rules`: seven sentences, the five of FR-13.4 and then one for each command
  the project allows (`build`, `run_tests`);
- `spike`, with `executor.sentence` "Being run in chat by the chat agent, who
  claimed it just now.", `measured: false` and `tokens_used: null`.

It then calls `save_spike_findings`: "Saved. If the time box ends now, this is
what is kept." The spike's page shows the executor line, the time-box line
("Time box: 4 hours, ending at 18:37 UTC on 2 October (3 hours 59 minutes
left)."), the unmeasured line, who holds the claim, **Release the claim**, and
the saved draft.

![The chat spike, claimed](walkthrough-spec-021-stage2/05-chat-spike-claimed.png)

## 4. Submitting, and closing

The chat agent first submits a one-section write-up. It is refused: "The
findings can't be submitted yet: the findings need a What we found section with
something in it. Nothing was changed." It then submits findings in the
template's sections (Answer, What we found, How we found out, What to do next)
and the spike ends as concluded. The page shows "Run in chat by the chat agent."
and "This spike ran in chat, so its tokens weren't measured." in place of the
token bar. The working copy was discarded.

![The ended chat spike](walkthrough-spec-021-stage2/06-chat-spike-ended.png)

The findings document holds the question, the chat agent's sections and
Subutai's **How this spike ended**: "The chat agent concluded the spike. It ran
in chat, so its tokens weren't measured."

![The findings](walkthrough-spec-021-stage2/07-chat-findings.png)

A person presses **The question is answered**. The page says how to build on it,
and the time-box line stays as "Time box: 4 hours."

![Closed as answered](walkthrough-spec-021-stage2/08-chat-spike-closed.png)

## 5. A person runs a spike by hand

`SPK-002` is started for **You**, with a time box of 2 hours. The notice reads
"SPK-002 has started, with a time box of 2 hours. Claim it below when you are
ready."

![The start screen, for a person](walkthrough-spec-021-stage2/09-start-screen-person.png)

![Started for a person](walkthrough-spec-021-stage2/10-person-spike-started.png)

**I'll run this spike** claims it. The panel gives the working copy's path, the
time left and an empty draft editor, with **Save findings**, **I've finished**
and **Release the claim**.

![Claimed, before saving](walkthrough-spec-021-stage2/11-person-claimed-before-saving.png)

After pasting findings into the editor and pressing **Save findings**, the
notice says "Your findings are saved." and the editor holds them.

![After saving](walkthrough-spec-021-stage2/12-person-saved-draft.png)

**I've finished** submits them: "You finished SPK-002. Read its findings below,
then say whether they answer the question."

![Finished](walkthrough-spec-021-stage2/13-person-spike-ended.png)

The same person closes it as answered. The page says "Run by hand by sam." and
"Closed by sam, who also ran it."

![Closed by the person who ran it](walkthrough-spec-021-stage2/14-person-spike-closed.png)

## 6. The time box runs out

`SPK-003` is started for the chat agent, claimed, and a draft saved. Then
`spikes.deadline_at` and `work_claims.deadline_at` are set to a minute ago in
SQL, and the page is reloaded until the next heartbeat (5 seconds) ends it.

![Before: claimed, with a draft](walkthrough-spec-021-stage2/15-time-box-before.png)

The spike has ended with the draft kept: "Reached its time box", and its
findings hold the saved Answer and What we found, then "The spike reached the
end of its time box of 4 hours. It ran in chat, so its tokens weren't
measured." The working copy was discarded and the findings committed
([repo-after.txt](walkthrough-spec-021-stage2/repo-after.txt)). `get_spike`
reports the claim as `ended`, `ended_how: "time_box"`, and a further
`claim_spike` is refused with "SPK-003 has ended. A person reads its findings
on its page."

![Ended at its time box](walkthrough-spec-021-stage2/16-time-box-ended.png)

![Its findings](walkthrough-spec-021-stage2/17-time-box-findings.png)

## 7. The Inbox, and the list

`SPK-004` is claimed by the chat agent, and then `work_claims.last_activity_at`
and `claimed_at` are set to 26 hours ago in SQL. On the next heartbeat the
claim sweep raises a `claim-stale` question, which reads as FR-13.1 gives it:
"SPK-004, ..., was claimed by the chat agent 1 day ago, and nothing has changed
in its working copy or its findings for 26 hours. Its time box ends at 18:37
UTC on 2 October. Is someone still working on it?" The answers are **Keep the
claim** and **Release the claim**, with the spike's consequence: "The claim
ends. The spike keeps running, its draft and working copy are kept, and its
executor can claim it again before the time box ends."

The "1 day ago" and "26 hours" are the SQL's backdating; the time box still has
nearly four hours left, so the stale question rather than the deadline applies.

![The Inbox](walkthrough-spec-021-stage2/18-inbox-claim-stale.png)

![The stale spike's page](walkthrough-spec-021-stage2/19-stale-spike-page.png)

The spikes list shows the executor in words beside each state: "chat", "chat",
"chat" and "by hand". Each row says "Time box: 4 hours" in place of tokens.

![The spikes list](walkthrough-spec-021-stage2/20-spikes-list.png)

## What the walkthrough found

Everything above matched the spec. These are the things that looked wrong or
worth a second look. No code was changed.

1. **The Ask again button is broken on a chat or person spike's page.** FR-15.5
   says the third choice reads **Ask again** with no budget field. With no
   field, the form (`class="triage-with-field"`, a two-column grid built for a
   label, a field and a button) puts the button in the narrow first column and
   the hint text runs over it: the word "Ask again" is drawn on top of "This
   closes the spike...", and the button has no visible border. It is the same
   on every ended chat or person spike. It is visible in
   [06](walkthrough-spec-021-stage2/06-chat-spike-ended.png),
   [13](walkthrough-spec-021-stage2/13-person-spike-ended.png) and
   [16](walkthrough-spec-021-stage2/16-time-box-ended.png). It can still be
   pressed, so it works, but it looks broken. The cause is the grid in
   `internal/server/ui/static/app.css` (`.triage-with-field`), which the
   spike's template uses without the field the grid assumes.

   ![The broken Ask again button](walkthrough-spec-021-stage2/16-time-box-ended.png)

2. **"Run" wording on a spike that had no run.** The ended chat or person
   spike's page says "This spike's run has ended: it reached a conclusion." and,
   after a time box, "This spike's run has ended: Reached its time box." The
   second has a capital letter after the colon, and neither is true of a
   spike that nobody ran in the spike runner's sense. The running draft panel
   also says "If the run stops now, this is what is kept." Stage 1's sentences
   were reused. They read oddly beside "Run in chat by the chat agent."

3. **The start screen's spike runner panels stay on screen for a chat or
   person spike.** FR-12.1 says they "describe the spike runner, and say so",
   and they do (the first panel says so), but the page still shows "The spike
   runner runs it, on its model", "4 of 4 agent slots are free now" and
   the spike runner's tools beneath a chosen **You**. It is accurate but busy;
   a person choosing **You** is shown three panels that don't apply to them
   ([09](walkthrough-spec-021-stage2/09-start-screen-person.png)).

Not shown: `claim_spike` after a release, a renewal, the leak check over a time
box, and the stale answers **Keep the claim** and **Release the claim** being
pressed. Those are proved by the stage 2 tests.
