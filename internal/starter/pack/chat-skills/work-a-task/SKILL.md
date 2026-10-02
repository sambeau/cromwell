---
description: Implement a task of a feature that is being built, in chat with a person, instead of leaving it to an agent
---

# Working a task, in conversation

This skill is for a chat AI working with a person who wants to do a task with
you rather than leave it to an agent. Use it only when they ask. Subutai's
implementer agents never use it.

You may claim a task of a feature that is being built, work it with the
person in the feature's working copy, and submit it. You may not judge it: an
independent reviewer reviews your work, and an independent verifier verifies
the feature.

## The path

1. **Find a task.** Call `get_feature` for the feature. Each task shows its
   state, who is implementing it, its latest claim and whether you can claim
   it now (`claimable`); if you can't, `why_not` says why in a sentence.
2. **Claim it.** Call `claim_task` with the task's ID, such as `FEAT-023-T03`.
   The result gives you the working copy, the contract (the approved spec and
   plan, the task and any decisions you are told), the rules, and the commands
   the project allows.
3. **Read the contract.** The spec and plan are what the work is judged
   against. If the task is a bug's, the report is the spec.
4. **Work only in the working copy.** Make the change with the person.
5. **Run the project's commands.** Run the build and tests before you submit.
6. **Don't commit.** Subutai commits your work when you submit it. A
   work-in-progress commit on the branch can show up in another task's review.
7. **Submit.** Call `submit_task` with a summary of what you did, in a
   sentence or two, written for the reviewer.

These are the rules the claim gives you. They are repeated here so that you
have them before you claim:

- Edit files only inside the working copy named in this result.
- Don't commit: Subutai commits your work when you submit it.
- Run the project's build and tests before you submit.
- Submitting with a summary is the only way forward, and you can't release the claim yourself: if you can't finish, tell the person, who can release it in the web UI.
- Your work gets an independent code review and the feature an independent verification, and you can't review, approve or verify it.
- Only one task in a feature can be worked at a time, and the feature's agents wait while you hold the claim, so submit promptly.

## What you may not do

- Review, approve or verify anything, your own work included.
- Edit outside the working copy.
- Claim a second task in the same feature while you hold one.
- Start building a feature. A person does that from the command centre.
- Answer the Inbox. A person does that.

## When the reviewer sends it back

The task comes back to you with the reviewer's comments. `get_feature` shows
the task's state and the comments; reading it doesn't keep your claim alive.
Call `claim_task` again to resume the task: the result carries the comments.
Fix what they ask, run the commands, and submit again. Calling `claim_task` on
a task you already hold is also how you say you are still on it.

## If you can't finish

You can't give a claim back. Tell the person plainly what is done and what
isn't. They release the claim in the web UI, which keeps what is in the
working copy and hands the task to an agent. A claim with no activity in the
working copy for the project's expiry time asks the person whether anyone is
still working on it.

## What to expect

Your tokens aren't measured, so the task is left out of Subutai's estimates
and actuals. Your work is reviewed by an independent reviewer, by default a
stronger model than the usual one. You never see the review's verdict until
it comes back to you or the task is done.
