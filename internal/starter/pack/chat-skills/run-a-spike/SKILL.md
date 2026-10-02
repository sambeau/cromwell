---
description: Run a spike a person has started for you, in chat, to answer a question within a time box
---

# Running a spike, in conversation

This skill is for a chat AI working with a person who has started a spike for
you to run. Use it only when they ask. Subutai's spike runner agent never uses
it.

A spike is a question to answer before building anything. A person starts it
from the web UI, chooses the chat agent to run it, and gives it a time box.
You claim it, investigate in a throwaway working copy, save your findings as
you go, and submit them. You may not judge them: a person reads the findings
and decides whether the question is answered.

## The path

1. **Find the spike.** Call `list_spikes` with the state `running`. A spike
   to be run in chat says so in its `executor`, and shows no claim yet.
2. **Claim it.** Call `claim_spike` with the spike's ID, such as `SPK-003`.
   The result gives you the working copy, the contract (the question, where it
   came from, any decisions you are told, any earlier findings and the
   findings template), the deadline, the rules and the commands the project
   allows.
3. **Read the contract.** The question is what you are answering. The
   template's sections are what the findings are written in.
4. **Work only in the working copy.** Read, run and measure there, with the
   person. It is a throwaway copy.
5. **Save as you go.** Call `save_spike_findings` early and often, with the
   whole findings text each time. It replaces what was saved.
6. **Submit.** When you have an answer, or know the question can't be
   answered, call `submit_spike` with your findings, or with none to submit
   what you last saved.

These are the rules the claim gives you. They are repeated here so that you
have them before you claim:

- Work only inside the working copy named in this result. It is a throwaway copy: it is discarded when the spike ends, and nothing in it is kept or merged.
- Don't commit, branch, tag or stash there: a spike keeps no code, and Subutai reports any it finds kept.
- Save your findings with save_spike_findings early and often. When the time box ends, whatever you have saved is what is kept.
- When you have an answer, or know the question can't be answered, call submit_spike with your findings in the template's sections.
- A person reads the findings and decides whether the question is answered. You can't close the spike, release your claim or move its time box, and you can't approve its findings on your own judgement: only a person's verdict, in their words, approves them.

## The time box

The spike ends when its time box does, whether you have finished or not, with
whatever findings you have saved. The claim's `time_left` says how long is
left. A submit that arrives after the time box ended is lost, so save before
you stop to think. Calling `claim_spike` again on a spike you hold renews the
claim and returns your draft.

## What good findings look like

Write them in the template's sections: the answer first, in a sentence or two,
then what you found, how you found it, and what to do next. Say plainly when
the answer is "we can't tell", and why. A submit that is missing a required
section is refused and nothing changes, so you can fix it and submit again.

## What you may not do

- Close the spike, approve its findings, or decide the question is answered.
- Release your claim, extend the time box or end the spike early.
- Edit outside the working copy, or commit anything.
- Start a spike. A person does that from its page in the web UI.

## If you can't finish

You can't give a claim back. Tell the person plainly what you found and what
you didn't. They can release the claim in the web UI, which keeps the draft
and the working copy until the time box ends. A claim with no activity for the
project's expiry time asks the person whether anyone is still working on it.

## What to expect

Your tokens aren't measured, so the spike is left out of Subutai's estimates
and actuals. Its findings become a document that a person reads and, if they
agree, closes the spike with.
