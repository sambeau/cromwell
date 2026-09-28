# DESIGN-010: Subutai

**Status:** **Approved — Sam, 2026-09-28.** *Amended the same day:* milestone
locking became a reversible *Mark as shipped*, which the chat agent may also
do (§5c, §6, §17a item 2; SPEC-010 SD-11; DEC-004 Amendment 1). *And again
with M5:* dated notes in §5c, §6 and §17a item 4 record how checklists and
jobs were built (SPEC-014). *And with M10:* a dated note in §5c records that
the chat agent submits its own drafts without a quote (SPEC-017). *And with
M9:* a dated note in §7 records how browser editing treats approved decisions
and untemplated types (SPEC-016). This is Draft 2, consistency-
reviewed (§19). The seven proposals in §17a were accepted with it. Drafted by
Claude; the author isn't the approval gate, and wasn't.
**Date:** 2026-09-28 (Draft 1: 2026-07-31)
**Author:** Claude, from the R2 discussion with Sam and the decisions of
2026-09-28.
**Binding decisions it rests on:**
- [DEC-006](../decisions/DEC-006-humans-start-development.md) and its
  Amendment 1: a human decides when development starts, and how specs are
  reviewed.
- [DEC-007](../decisions/DEC-007-the-judgement-boundary.md): who does the work
  is flexible; who judges it is not.
- The fourteen roadmap decisions accepted on 2026-09-28
  ([roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §12).

**Sources:**
- the [Subutai discussion document](../vision/Cromwell%20R2%20Vision%20Subutai.md);
- the [discussion response](../notes/subutai-discussion-response-2026-07-31.md);
- [Subutai and GitHub](../research/subutai-and-github.md);
- [vision-v1](../vision/vision-v1.md), which this document revises but does not
  replace.

**Reading notes.** Section numbers in this document are Draft 2's own. Anything
that went beyond an earlier decision is marked **(§17a)**. All of those were
accepted with the design on 2026-09-28.

---

## 1. What Subutai is

Subutai is a planning, workflow and orchestration system for small teams
building large software projects with AI agents. It takes work from the first
rough idea all the way to shipped, verified software, and it keeps an honest
record of everything that happened along the way.

Subutai is the new name for Cromwell. It is a revision, not a rewrite: think of
it as Cromwell, second edition. The engine underneath is unchanged. This
document clarifies the workflow, settles the vocabulary, and describes the
surfaces people actually touch: chat and the web.

## 2. The big picture: two worlds, and a human on the switch

Building software has two kinds of work in it, and they behave completely
differently.

**Designing is creative work.** Deciding what to build is a conversation. It
loops, it backtracks, and it changes its mind over coffee. You can't put it on
rails, and you shouldn't try. In Subutai, design happens as a conversation
between humans and a highly capable chat AI, at whatever pace the thinking
needs. This is where the human's judgement goes. Anyone who wants a say in
algorithms, data structures or architecture says it here, in the design.

**Development is disciplined work.** Once you know what to build, building it
has a strict shape:
- specify it;
- plan it;
- build it;
- review it;
- check it against the definition of done.

Every step has rules, and skipping steps is how quality dies. In Subutai,
development is run by the **orchestrator**. That is the most important design
decision in the system, so it gets its own paragraph.

**The orchestrator is software, not an AI.** It is plain, deterministic code
that watches for events, checks the rules, and hands work to AI agents at the
right moments. It has no conversation history to lose track of and no
attention span to exhaust, and it can't be talked out of a rule.

We learned this the hard way. Our previous system, kanbanzai, used an AI agent
as the orchestrator, and under a long enough workload it drifted:
- it did the work itself instead of delegating;
- or it delegated and then forgot to follow up;
- and the records then claimed things were done that weren't.

Code doesn't drift. Every safety rule in Subutai is enforced by code, not
promised by an AI.

**The handover is an approved design, and a human decides when it happens.**
Approving a design says "this is what we want". It starts nothing. When the
time is right, a person presses **Send to development**, and the agents take it
from there.

The button is there because starting agents uses things that have limits:
agent time, parallel jobs and token budget. Whether *now* is a good moment is
a judgement about resources, and it belongs to a person. The same goes for the
second button, **Start building**, which begins implementation once the plan
and estimate are ready. Both buttons are about *when*, not about whether the
work is any good. Judging quality is the reviewers' job (§5).

### Why the models split where they do

The most capable models are affordable on subscription chat plans, and
expensive through the API. The workhorse models that write specs, plans and
code are cheap through the API. So design naturally runs in chat with the
strongest models, with the human in the room, and development runs on API
agents chosen per role.

That is a convenient alignment, not a law of the system. Which model serves
which role is configuration, and it will change as pricing does.

## 3. The team

A Subutai team mixes humans and AI, in two groups.

**The product team** shapes what gets built:

- **Product manager**: a human.
- **Designer**: a human and a chat AI working together.
- **Project manager**: a chat AI, guided by a human. This is the agent that
  talks to Subutai on the humans' behalf. It:
  - creates initiatives and features;
  - drafts designs;
  - keeps milestones and roadmaps up to date;
  - checks status;
  - relays the humans' decisions (§5c).

**The development team** builds it:

- **The orchestrator**: deterministic software. It runs the show.
- **Spec author**: turns an approved design into a precise, testable spec.
- **Spec reviewer**: the border guard. It checks the spec is clear, complete
  and faithful to the design, and it is the spec's normal approver.
- **Planner**: reads the spec and writes the development plan, which is how the
  work breaks into tasks.
- **Plan reviewer**: checks the development plan.
- **Estimator**: sizes the work, in tokens.
- **Implementers**: write the code, one task at a time, each in its own
  isolated working copy.
- **Code reviewers**: inspect each task's changes against the spec and the
  plan.
- **Verifier**: the last agent in the chain. It checks the finished feature
  against its acceptance criteria, and must show evidence for every one. This
  step can never be skipped, by anyone.

Every orchestrated role has its own configured model, its own instructions, and
its own strictly limited set of tools. A reviewer has no tool that can edit a
file. Which model backs which role is yours to tune, per project.

**The design reviewer is retired (§17a).**
- It is built today (SPEC-009 FR-2.2). It comments on designs and never rules.
- A design gets its review in the conversation that writes it, with a human in
  the room.
- The `review-design` skill survives as a chat-side skill, for anyone who
  wants a cold read of a design on request.
- The removal lands in roadmap milestone M3.

## 4. The vocabulary

Subutai keeps a deliberately small set of concepts, in two groups: things that
*define* work, and things that *track* it.

### Work entities: where work is defined

| Term | What it is |
|---|---|
| **Project** | The top of the tree. Conceptually just the topmost initiative. |
| **Initiative** | A container for a piece of intent: a plan to do something. Initiatives nest. They hold documents, milestones and roadmaps, but deliberately have no lifecycle. Above the feature line, things simply *are*, until they're archived. |
| **Feature** | A buildable unit. This is the commitment point: creating a feature says "we intend to build this". Features have the full lifecycle, a spec, a plan, tasks and a worktree. |
| **Task** | One unit of implementation work inside a feature. |
| **Bug** | A reported problem. It is feature-shaped, with a triage step at the front (§9). |
| **Spike** | A question with a budget. It is throwaway investigation that produces findings, never shipped code (§10). |
| **Job** | A single task for a human: sign up for the account, choose the icon, find the API key. Ticked off by hand. |
| **Checklist** | A list of jobs. It is done when every job is ticked. |

### Tracking entities: where work is measured

| Term | What it is |
|---|---|
| **Deliverable** | Anything a milestone can contain: an initiative (meaning everything under it), a feature, a bug (§17a), a checklist, or another milestone. |
| **Milestone** | An *unordered* collection of deliverables that ship together. It is live while open. Marking it as shipped freezes a record of what shipped, so the record stays honest (§6). |
| **Roadmap** | An *ordered* list of **milestones**, and only milestones (DESIGN-008 D-11). The order means whatever the planner wants it to mean, and the system just preserves it. |

### Documents

Every piece of writing in the system is a **document**: designs, specs,
development plans, reviews, bug reports, spike findings, decisions and notes.
A document is a Markdown file in the project's git repository, tracked by
Subutai with a type, an ID and a lifecycle (`draft → reviewing → approved →
superseded`).

An approved document is never reopened. It is changed by writing a
**successor**, a new draft that replaces it when approved. That is how
revision works throughout this document. Section 7 covers how documents are
identified and where they live.

Two document types matter most, and the difference between them is the
difference between the two worlds:

- **A design** says what we want and why. It is written by humans, with AI
  help, for humans. Any initiative or feature can have one. A feature builds
  from its own design or its **immediate** parent initiative's (gate G0).
- **A spec** says exactly what will be built, for the agents who will build it.
  It is normally written by the spec author agent. It can also be written
  ahead in chat (§5a), and for a bug the bug report serves as the spec (§9).
  Only features and bugs have specs, because only they get built.

## 5. The workflow, end to end

There are **two routine buttons**, **Send to development** and **Start
building**, and both are about timing. Beyond them, a human steps in only:
- when they choose to (raising an issue, holding a spec);
- when the work needs a human decision by its nature (triaging a bug, closing
  a spike);
- or when something has gone wrong (checkpoints).

After approving a design and pressing the two buttons, a human's usual next
involvement is **manual testing** of what was built.

### Designing: the conversation

Work begins as an initiative and a conversation. The humans and the chat AI
talk through what's wanted and write a design together. They revise it, sleep
on it, and revise it again. Sub-initiatives and features get sketched out
underneath as the shape becomes clear.

When the design feels right, a human approves it, either on the document page
or by telling the chat agent. Approval is a record, not a trigger. Nothing
launches. Approved designs can sit ready until there is capacity to build them.

### The first button: Send to development

When it's the right time, a human presses **Send to development**.

**Where it appears:**
- on a feature;
- on an initiative, to send several of its features at once, chosen with
  checkboxes.

**When it can be pressed:**
- A feature can be sent when its own design or its *immediate* parent
  initiative's design is approved (gate G0), and it has a description.
- Otherwise the button explains what's missing.
- The button is in the web UI only. The chat agent can't press it.

**The send screen shows what's about to happen, before anyone commits:**
- the features being sent;
- each step, with the agent role and model that will run it, and whether it is
  already done;
- who will review each spec;
- a rough token forecast;
- how many agent slots are free.

Work that doesn't fit waits in the dispatch queue, as all dispatches do.

**From the button, development planning runs unattended:**

1. **The spec is written.** The spec author translates the approved design
   into precise, testable language. A spec already written in chat is used as
   it is, and this step is skipped (§5a).
2. **The spec is reviewed.** It must first pass the mechanical validation that
   every document gets on submission. Then the spec reviewer checks it is
   clear, testable, and a faithful, complete translation of the design, and
   approves it or sends it back to the author with findings.
3. **The spec is held for a human**, if the hold is on (below).
4. **The development plan is written**, breaking the feature into tasks.
5. **The plan is reviewed** by the plan reviewer.
6. **The work is decomposed into tasks and estimated**, in tokens.

Then the system stops and waits.

### How specs are reviewed

**Every spec is reviewed, normally by an agent.** The spec reviewer is the
normal approver. Its verdict moves the spec on without waiting for anyone. The
human's role is to **point out issues**, not to be the arbiter of truth.

**A human may raise an issue on a spec at any time before building starts**,
on the document page or by telling the chat agent. A human issue must be dealt
with: the reviewer can't approve until it has said how each issue was
addressed, or why it doesn't apply. Where the issue goes depends on the spec's
state:
- **For a feature that has been sent, and a spec in draft or review:** the spec
  goes back to the spec author with the issue.
- **For an approved spec:** the issue starts a successor. When the successor is
  approved, it supersedes the old spec, and the old plan is superseded with it.
- **For a feature not yet sent:** the issue is simply recorded on the spec.
  Nothing is dispatched, because nothing runs before Send. Whoever is working
  on the spec in chat addresses it, and the reviewer checks it when the review
  runs.

**A human may also approve a spec directly, or ask for an agent review.** Both
are there when wanted, and neither is expected.

**Holding specs for a human is optional, and off by default.** It can be set
per project or per send. A held spec waits after the agent review, and the
human then does one of three things:
- approves it;
- raises issues;
- hands it back with *let the reviewer decide*, after which the agent's verdict
  stands.

**There is always at least one reviewer.** A project may turn off the agent
spec review, but then every spec is held for a human after validation. In
that case the human either approves the spec or raises issues. They can still
ask for a one-off agent review. *Let the reviewer decide* isn't offered,
because there is no automatic reviewer. No setting lets a spec through
unreviewed.

### The second button: Start building

A human looks at the reviewed plan and the estimate, and presses **Start
building**. That button is in the web UI only, too. From here the loop runs:

- Implementers pick up tasks in an isolated working copy.
- Code reviewers inspect each task's changes.
- Revisions cycle until the reviewer finds no more *major* problems.

Minor findings, such as style, naming and tidiness, don't hold work hostage.
They are recorded as bug reports in the triage queue (§9), and they become
material for retrospectives. A finding that touches correctness,
completeness, scope or soundness is major, and majors always go back for
fixing.

When every task is done, the **verifier** checks the whole feature against the
spec's acceptance criteria. It must cite specific evidence for each one. "Looks
fine" is not evidence; "the login test covers valid and invalid credentials"
is. Only then does the feature merge and count as done.

**The GitHub exception.** On a project where developers merge their own pull
requests, tasks merge as they finish, and the verifier runs on the merged code
(§12).

After that comes the human's manual testing, and anything it finds is a bug
report.

### Exceptions, not check-ins

Humans are not asked to babysit any of this. The system interrupts a human only
through **checkpoints**. These are queued questions that appear in the web UI
inbox when something genuinely needs judgement:
- a reviewer escalates a disagreement;
- a dispatch keeps failing;
- a budget threshold trips;
- a merge conflicts;
- a revised design affects several specs;
- a claim goes quiet (§5b).

Answer the checkpoint and the machine continues. Checkpoints are answered in
the web UI. If checkpoints are rare, the system is healthy. If they're
frequent, something upstream needs fixing, and that's worth knowing too.

### When a design changes: the cascade

Designs get revised. When a successor design is approved, Subutai finds the
specs written from the design it supersedes:
- If there is only one, it is superseded, along with its plan.
- If there are several, one checkpoint asks, spec by spec, whether to keep it
  or redo it.

A superseded spec is rewritten against the new design, automatically, for
features already sent to development. A feature not yet sent is left without a
spec until someone sends it. Tasks are reconciled, not duplicated. A feature
already being built also gets the existing "revised while in progress"
checkpoint.

### 5a. Working ahead in chat

Sometimes a feature is small but tricky, and you'd rather work through part of
it with the strongest model in conversation than wait for the pipeline. Subutai
supports this directly, and the mechanism is simple: **development-planning
steps check what already exists; they don't insist on running.**

Before pressing Send, the conversation can:
- write the spec or the development plan itself, in which case that step is
  skipped;
- ask for an agent review of a spec or plan early. The same independent
  reviewer runs, and the verdict lands on the document.

The send screen shows which steps are already done. A well-prepared feature
flows straight through to Start building.

### 5b. Who does the work: executors

This is [DEC-007](../decisions/DEC-007-the-judgement-boundary.md). Any stage's
*work* can be done by a dispatched agent, by the chat AI in conversation with a
human, or by a human alone. We call whoever does the work its **executor**, and
the system records it.

A chat AI or human working on a task claims it first, works in the feature's
working copy, and submits it when done. That is the same doorway the
dispatched agents use, so the state always tells the truth.

The honest reason: if the system didn't allow it, people would do it anyway.
When a task is genuinely hard, the temptation to open a chat window and say
"just fix it" is real. Work done outside the system gets no review, no record
and no verification. Giving it a proper lane makes it visible and reviewed.

One rule makes this flexibility safe:

> **Who does the work is flexible. Who judges the work is not.**

**Doing stages** are writing a spec or plan, implementing a task, estimating,
and running a spike. Anyone may do them. **Judging stages** are spec review,
plan review, code review and verification. These belong to an independent
party, never to whoever did the work.
- The chat AI has no tool that can approve, review or verify anything on its
  own judgement.
- A chat-implemented task still gets an independent code review. By default
  that review uses a stronger model, because a feature done in chat has had one
  mind through the whole chain (DEC-007 decision 10).
- Verification against the definition of done is **always** done by the
  dispatched verifier. No actor, human or AI, can waive it.
- A claim with no activity raises a "still working on this?" checkpoint.
- A commit to an active feature with no claimed task is flagged.

### 5c. What the chat agent may do

The chat agent is a full member of the product team, with one firm boundary.
[DEC-006](../decisions/DEC-006-humans-start-development.md) Amendment 1 draws it
by *what an act does*, not which channel it comes through.

- **It may author planning structure,** under DEC-004: initiatives, features,
  milestones and roadmaps, descriptions and documents. Checklists and jobs are
  included too (§17a).

  > **Note, 2026-09-28 (Sam, accepted with [SPEC-014](../specs/SPEC-014-checklists-and-jobs.md) SD-7
  > and SD-10).** "Create checklists and jobs" is read as keeping them: the
  > chat agent may also rename, reorder and remove jobs, as planning, without
  > quoted words. Two limits stop that standing in for a tick: a ticked job's
  > title can't change, and the chat agent can't remove a checklist's last
  > unticked job. Relaying "a ticked job" includes unticking one, with the
  > person's words (DEC-006 Amendment 1, note of the same date).
- **It may relay a human's decisions:**
  - a verdict on any document;
  - an issue raised on a document;
  - a request for a review;
  - *let the reviewer decide* on a held spec;
  - a ticked job.

  Each relay quotes the human's words and is audited as arriving via chat.
- **It may claim and submit work** under DEC-007.

  > **Note, 2026-09-28 (Sam, accepted with [SPEC-017](../specs/SPEC-017-chat-as-a-proper-seat.md) SD-1).**
  > Submitting work includes a spec or plan the chat agent wrote with a
  > person: `submit_for_review` hands it to the independent reviewer without
  > quoting anyone, and the reviewer decides. It refuses a revision of a
  > feature being built. Asking for a *fresh* review of a document already in
  > review stays a relay, with the person's words (DEC-006 Amendment 1, note
  > of the same date).
- **It may never:**
  - press Send to development or Start building;
  - override a gate, including by answering a checkpoint that would;
  - hold a verdict of its own.

  Those either commit resources or remove a safeguard.
- **It doesn't answer checkpoints at all**, even the ones that don't override a
  gate, such as keep-or-redo. Checkpoints are UI acts. The chat agent can read
  them and help a human decide (DEC-005).
- **It may mark a milestone as shipped, and reopen it**
  ([DEC-004](../decisions/DEC-004-mcp-planning-authoring.md) Amendment 1).
  Shipping is a record that can be undone, not a development gate. This
  replaces Draft 2's "may not lock", which assumed locking was permanent.

## 6. Planning and tracking

This is how people plan the project and follow its progress. It is the human
side of Subutai's everyday use, so it has to be simple, visible and editable
from both the web UI and chat.

### Milestones

A milestone is a set of deliverables that ship together, such as "Payments
beta" or "Public launch". It contains any mix of:
- initiatives (everything under them, live: a feature added later joins
  automatically);
- features;
- bugs (§17a);
- checklists;
- other milestones.

It may carry a target date.

**A milestone shows progress two ways, because there are two honest
questions.**
- *How many deliverables are done?* A count, "3 of 4 done". This is what a
  person reads at a glance.
- *How much of the estimated work is done?* A token bar. A milestone can be
  "3 of 4 done" but only "40% of the tokens" when the last deliverable is the
  big one.

Both are shown. On the page a milestone looks like what it is: a checklist of
its deliverables (DESIGN-008 D-10).

**Marking a milestone as shipped** freezes it. A milestone's contents are
live, so without this a feature added later under an included initiative would
quietly change the record of what shipped. Shipping records exactly what the
milestone covered at that moment, and reports against that fixed list from
then on.
- It needs at least one deliverable done (gate G4, a record-keeping check).
- **It can be undone**: *Reopen* returns the milestone to live membership.
  This was changed from Draft 2's permanent lock on 2026-09-28 (SPEC-010
  SD-11).
- Deliverables not done when it is shipped are recorded as not shipped. They
  are not quietly dropped.
- A person does it in the web UI, and the chat agent may do it too (§5c).

### Roadmaps

A roadmap is an ordered list of milestones, and only milestones. The order
means whatever the planner wants: sequence, priority, or rough timing. On the
page it looks like a numbered list, each entry showing its milestone's progress
and target date.

### Checklists and jobs

A checklist is a list of **jobs**, meaning things only a human can do: get an
API key, sign a contract, choose an icon.
- A job has a title, an optional note, and a record of who ticked it and when
  (roadmap decision 8).
- Jobs are ticked on the checklist page, or by telling the chat agent, which
  relays it with the human's words (§5c).
- A checklist is done when every job is ticked.
- A checklist can be a milestone deliverable, so a release can't be called done
  while the human chores are still open.

> **Note, 2026-09-28 (Sam, accepted with
> [SPEC-014](../specs/SPEC-014-checklists-and-jobs.md) SD-1 to SD-6).** As
> built in M5:
>
> - **A checklist is done when it has at least one job** and every job is
>   ticked. An empty checklist isn't done.
> - **A checklist counts as one item** in a milestone's "X of Y done", however
>   many jobs it has. It carries no tokens, so the token bar ignores it.
> - **G4 counts items.** A done checklist is a done deliverable, so it can let
>   a milestone be marked as shipped on its own (DEC-004 Amendment 1, note of
>   the same date).
> - **An initiative in a milestone brings in its features, not the checklists
>   planned in it.** "Everything under it" (§4) is read as the work tree. A
>   checklist counts when it is added to the milestone itself.
> - **Shipping records a checklist that isn't done as not shipped**, as it
>   does a feature, and the shipped record never goes backwards: a checklist
>   done when the milestone shipped stays done in the record. The checklist
>   itself isn't frozen, and its page always shows it as it is now.
> - **Jobs are ticked on the checklist page and edited in a modal**, opened
>   from the owner's page or the checklist page. A job has one note, which a
>   note given with a tick or untick replaces.

### Ownership and editing

- **Ownership.** Milestones and roadmaps belong to the project or to an
  initiative, so planning sits at the level it's about (DESIGN-008 D-9).
  Checklists belong to them the same way (§17a). What a milestone
  *contains* can come from anywhere in the tree (D-12).
- **Editing in the web UI.** Each one is edited in a modal from its owner's
  page (roadmap decision 7). Members can be added from either end: from the
  milestone, or with "Add to a milestone…" on any feature or initiative.
- **Editing in chat.** The chat project manager can create and fill
  milestones, roadmaps and checklists in conversation.

## 7. Documents: identity, homes and editing

### Every entity has an ID, minted by the system

Subutai assigns IDs centrally, from the database, so numbers never clash no
matter who creates what, from chat or the UI (roadmap decision 9).

| Thing | Prefix | Example | Status |
|---|---|---|---|
| Initiative | `INIT-` | `INIT-014` | accepted |
| Feature | `FEAT-` | `FEAT-023` | accepted |
| Bug | `BUG-` | `BUG-007` | accepted |
| Spike | `SPK-` | `SPK-003` | accepted |
| Decision | `DEC-` | `DEC-008` | accepted |
| Milestone | `MS-` | `MS-004` | accepted (§17a) |
| Roadmap | `RM-` | `RM-001` | accepted (§17a) |
| Checklist | `CL-` | `CL-002` | accepted (§17a) |
| Task | `<feature>-T<n>` | `FEAT-023-T03` | accepted (§17a) |

The number means nothing beyond "the next one". A **decision** is a document,
not an entity, but it gets its own number sequence, because decisions are cited
on their own.

### Document IDs, and what happens when a document is revised (§17a)

- **A document's ID** is its owner's ID plus its type, for example
  `FEAT-023-spec`.
- **If an owner has more than one document of the same type**, such as two
  design notes on one initiative, the second and later ones take a number:
  `INIT-014-design-2`.
- **The ID names the document across its revisions.** A successor keeps the ID
  and increments a **revision** number in its front matter. So the current
  spec of `FEAT-023` is always `FEAT-023-spec`.
- **Superseded revisions are archived** with the revision in the file name
  (`FEAT-023-spec.r1.md`), much as Cromwell archives them today.

### Identity lives in front matter; location is free

Each document carries its ID in its front matter, and **that is its identity.**
Rename it, move it or reorganise the folder, and Subutai still recognises it.
Nothing detaches.

This matters most for existing projects. Their documents are already Markdown
in the repository, in their own folders, linked to each other and pasted into
old conversations. Subutai **adopts them where they sit**: an *adopt* action
gives an existing file an ID, a type, a lifecycle state and an owner, touching
only its front matter. Nothing is moved, and no link breaks.

### New documents get a sensible default home

Documents Subutai creates go in one folder per initiative, with the ID in the
file name. Related documents then group together at a glance:

```
docs/work/INIT-014-auth/
    INIT-014-design.md
    FEAT-023-spec.md
    FEAT-023-dev-plan.md
    BUG-031-report.md
```

This is a default, not a rule. A document created elsewhere, or moved later, is
just as much a Subutai document.

### Creating work creates its documents

Creating an initiative or feature, in the UI or via chat, creates its starter
design document from the template, in the right folder, already attached.
Attaching an existing file stays available for documents that already exist,
alongside *adopt*.

### Editing in the browser

The web UI includes a simple Markdown editor with preview, for the everyday
case of fixing and polishing documents without leaving the browser.

- **Save & commit** is the main button (roadmap decision 11). It writes the
  file and commits it.
  - On a project whose `main` is protected, it also lands the commit through
    the project's documents fast lane (Subutai and GitHub §7), so a paragraph
    edit doesn't wait on a full code-CI run.
  - The commit author is the signed-in person once per-user identity exists
    (§13). Until then it is the configured operator.
- **Save** alone writes the file and leaves the commit to you.
- **If the file changed on disk since you opened it**, because someone edited
  it in vim, saving stops and shows you the difference. It doesn't silently
  overwrite either version.
- **The editor always shows the document's lifecycle state.**
  - A draft is edited in place.
  - Editing an approved document creates a successor draft. When that is
    approved, it supersedes the original. For a design, that starts the
    cascade (§5).
  - The editor warns before you edit a document under review, or one that
    feeds work in progress.

  > **Note, 2026-09-28 (Sam, accepted with [SPEC-016](../specs/SPEC-016-edit-in-the-browser.md)
  > SD-9, SD-11 and SD-12).** An approved **decision** isn't revised in the
  > editor, because §11 says an accepted decision is never edited. An approved
  > document of a **type with no template** isn't revised there either, until
  > a person can rule on any type (M10), because its successor couldn't be
  > submitted. Saving a document under review takes it back to draft. The
  > editor stands aside, rather than warning, where the author agent is due to
  > rewrite the document.

Documents remain plain Markdown files in git throughout. The browser editor is
a convenience, never a requirement, and never a special format.

## 8. Watching the work

Subutai aims to answer two different questions well: *"how is it going?"* at a
glance, and *"what exactly happened?"* when you need to dig.

- **The timeline answers the first.** Each feature shows its journey as major
  moments, for example: *Sent to development · Spec approved · Plan approved ·
  Building (3 of 7 tasks) · Verifying · Done*. That is clean enough to read in
  a glance, without the noise of every internal event. Milestones and roadmaps
  (§6) give the same answer one level up.
- **Transcripts answer the second.** Every agent dispatch keeps its complete
  conversation:
  - the prompt the agent was given;
  - every tool it used;
  - everything it said;
  - what it concluded.

  Click into any timeline moment and read exactly what the agent did and why.
  When an estimate was wildly off or a review feels wrong, the answer is in the
  transcript.
- **Sizing is in tokens.** Work is estimated and measured in tokens, the actual
  unit of AI computation. Tokens are objective and additive, and they are
  recorded automatically. Completed work feeds its actual use back in, so
  estimates sharpen over time. Work done in chat uses subscription tokens the
  ledger can't see, so it is marked *unmeasured*, not counted as free.
- **Review health is visible.** Approval rates, findings per review and time to
  verdict are shown per reviewer. A reviewer that approves everything in
  seconds is a broken reviewer, and the numbers make that visible.
- **Everything is attributed.** Every artifact and verdict records who produced
  it: which agent role and model, the chat AI, or a human. Questions like "do
  specs with human issues bounce more at planning?" then have answers instead
  of guesses.

## 9. Bugs

Anyone, human or agent, can report a bug at any time, without ceremony.
- Agents file reports mid-work when they spot something out of scope, and the
  minor review findings from §5 land here too.
- Humans report through chat, through a form in the UI, or, on projects that
  use GitHub, by filing an ordinary GitHub issue labelled as a bug (§12).

Every report becomes a bug entity with a report document, attached to the
initiative or feature it came from.

**A bug is feature-shaped, with one extra step at the front: triage.**
- Reports wait in a dedicated **triage queue**, with a count on the inbox
  (roadmap decision 12).
- A human accepts or rejects each one. That is a commitment of scope, the same
  kind of act as creating a feature.
- Triage happens in the queue. The chat agent may relay a triage decision with
  the human's words (§17a).

**From acceptance onward, a bug travels the normal pipeline, with two
differences** (§17a):
- **Its gate to Send is acceptance.** A bug has no design, so an accepted bug
  with a report can be sent to development in place of G0.
- **Its report serves as its spec.** The spec-writing step is skipped. The spec
  reviewer reviews the report as a spec, checking that the reproduction and
  the acceptance criterion ("the defect no longer reproduces") are clear and
  testable.

After that it is the normal pipeline: plan, plan review, Start building,
implementation, code review and verification. There is no parallel machinery
and no second-class pipeline.

## 10. Spikes

Sometimes you can't design properly because you don't yet know enough. Does
that API do what its documentation claims? Is this approach fast enough to
bother with? The honest response is to go and find out, and the work of
finding out is a **spike**.

A spike is a question with a budget.
- It hangs off whatever initiative or feature raised it.
- It produces exactly one thing: a findings document.
- The code it writes along the way is scaffolding. It is real enough to answer
  the question, and never good enough to ship.

**Starting a spike (§17a).** Anyone may *create* a spike, including
the chat agent as planning structure. **Starting** one spends resources, so it
works like Send to development: a human starts it in the web UI. Any executor
may *run* it (DEC-007): a dispatched agent, the chat AI, or a human.

**The budget.** A dispatched spike carries a token budget, from a project-wide
default that can be overridden per spike (roadmap decision 13).
- **When the budget runs out, the spike stops.** That is a hard stop, not a
  polite question. Spikes are the easiest thing in software to let run for a
  fortnight.
- The findings so far are written up, and a human decides whether the question
  deserves a second, separately budgeted spike.
- A spike run in chat or by a human can't be metered in tokens. It carries a
  time box instead, enforced through claim expiry (§5b) (§17a).

**A spike can't merge, and that is enforced, not promised.** It gets a worktree
like any other work, and when it concludes the worktree is discarded. There is
no merge path in the entity at all.

Because a spike has no spec, the definition-of-done machinery doesn't apply to
it. It is done when someone reads the findings and says the question is
answered. The findings are an ordinary document, and they usually feed a
design.

**Promotion is deliberate and one-way.** A spike that starts looking like the
real implementation doesn't graduate into one. The findings inform a design, a
feature is created in the normal way, and the feature is built from scratch by
the normal pipeline. The scaffolding stays in the bin.

## 11. Decisions

Decisions are how the project remembers *why*. Each one is a document, with its
`DEC-nnn` number minted by the system, recording:
- what was decided;
- the reason;
- what it replaced.

An accepted decision is never edited. If the project changes its mind, a new
decision supersedes the old one, or a dated amendment is appended, as
DEC-006's Amendment 1 was. Either way the trail stays honest.

Decisions attach to the project or to an initiative, and they do their real
work automatically. **Accepted decisions on the relevant branch of the tree are
pushed into agents' prompts when work is dispatched.** This matters because of
a lesson we paid for twice: agents won't go and look things up. Any knowledge
system that relies on agents fetching from it will be written to and never
read. So Subutai doesn't have a knowledge base. It has documents, and it pushes
the relevant ones into the prompt.

**Only the ruling and its one-line reason are pushed, not the whole
document.** There is also a hard cap on how much any dispatch receives, so
surfacing stays affordable (the size is open, §17b). Project conventions use
the same mechanism. A simple viewer in the UI lists decisions for humans.

## 12. Working with GitHub

Many projects already live on GitHub, and their developers review code there.
Subutai works alongside it, without asking developers to change how they work.
It is optional. A project with no forge configured works exactly as the rest
of this document describes.

**Partition, don't synchronise.** Every fact has exactly one owner.

| Owned by Subutai | Owned by git and GitHub |
|---|---|
| Designs, specs, plans, and their approvals | Branches, commits, diffs |
| Lifecycle, breakdown, priority | Pull requests and code-review comments |
| Milestones, roadmaps, checklists | Continuous integration |
| Bug reports and triage | Whether code has merged |

The two are joined by an identifier, not by a sync.

- **Subutai projects an issue for each task**, so a developer has something to
  branch from and close. The issue contains the spec link and acceptance
  criteria, written in prose. The issue is a report, not a replica: Subutai
  rewrites its body and never reads state back from it.
- **Subutai observes** pull requests merging, and continuous-integration
  results.
- **Developer-filed bug issues arrive in the triage queue.** This is the only
  inbound flow.
- **Nothing is edited both ways**, and no approval ever happens in GitHub.

**Human work, human review.** When a human implements a task and a colleague
approves the pull request, that review is the independent code review
(DEC-007 decision 9). Subutai records both people, and doesn't ask an agent for
a second opinion.

**Verification is at the feature level, after merge.** When a feature's last
task has merged, the verifier checks the feature against its acceptance
criteria on the merged code. A failure becomes a checkpoint or a bug. It never
blocks a developer's pull request.

**Bringing a project in** is registration, not migration.
- An importer maps the project's existing issues and labels to Subutai
  entities, with a per-project label map and a dry run that reports what it
  would create before it creates anything.
- Existing documents are adopted where they sit (§7).
- Closed issues come in as archived entities, so the trail from shipped code to
  its intent survives.

## 13. People

Subutai is for small teams, so it needs to know who's who. **Per-user
identity** arrives before a second person uses a project, and in any case
before the Tickly pilot if more than one person takes part (roadmap decision
14). With it, approvals, issues, ticks and commits are recorded against the
person who made them.

Until then Subutai runs with a single operator. Anything relayed through the
chat agent is recorded as arriving via chat, with the human's words quoted.
That is honest, but it can't prove who spoke.

## 14. What Subutai deliberately doesn't do

- **No AI orchestrator.** The orchestrator is code, permanently. This is the
  founding lesson.
- **No self-approval, ever.** No actor judges its own work, and the chat AI
  holds no verdict of its own.
- **No unreviewed specs.** There is always at least one reviewer.
- **No skippable verification.** The definition-of-done check always runs, by
  the dispatched verifier, whoever did the work.
- **No committing resources from chat.** Send to development, Start building
  and starting a spike are human acts in the web UI. The chat agent may still
  claim work and ask for reviews (§5c).
- **No knowledge subsystem.** It has been tried twice and failed twice.
  Documents plus automatic surfacing do that job.
- **No rich text.** Documents are Markdown files in git. Every tool that touches
  them, including our own editor, works on the plain file.
- **No two-way sync with GitHub.** One owner per fact.
- **No multi-project installs.** One Subutai installation manages one project.

## 15. From Cromwell to Subutai: what actually changes

For readers who know Cromwell, here are the deltas:

1. **The name.** Cromwell becomes Subutai, once, at a clean boundary.
2. **A human starts development.** Approving a design no longer sets agents
   off; Send to development does. The spec author and plan author stay
   orchestrated. "Start work" is renamed Start building.
3. **Spec review is reshaped.** The agent reviewer is the normal approver, and
   there is always at least one reviewer. Humans raise issues, and can hold a
   spec for themselves. The design reviewer is retired.
4. **The chat agent grows up.** It relays human decisions, quoted and audited.
   It plans milestones, roadmaps and checklists, and it can claim work. It
   never judges, and never commits resources.
5. **Executors.** Work records who did it. Chat and humans can do any doing
   stage through claim and submit. Judging stays closed.
6. **Planning and tracking come into the UI.** Milestones and roadmaps become
   editable in the browser and from chat, and checklists and jobs arrive.
7. **Document management grows up.** System-minted IDs, identity in front
   matter, adopt-in-place, starter documents on creation, and the browser
   editor.
8. **Observability grows up.** Full dispatch transcripts, the feature timeline,
   review-health metrics, and attribution everywhere.
9. **Bugs, spikes and decisions become first-class.**
10. **GitHub, optionally.** Task issues projected out, merges observed, bugs
    triaged in, verification after merge.
11. **People.** Per-user identity before a second person joins.
12. **Everything else stands.** The orchestrator, the gates, the document
    lifecycle, worktree isolation, checkpoints, severity-gated review loops,
    token sizing and the audit trail are unchanged, by design.

## 16. The v1 line

**Subutai v1 is done when** everything in §15 is shipped, and one real project
(Tickly) has taken one feature from idea to merged code through Subutai, with a
written walkthrough. The
[roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11 breaks this
into milestones M0 to M15:
- the design reviewer's retirement and the decision on withdrawing a send land
  in M3;
- per-user identity lands with the pilot, in M15, when a second person takes
  part.

**After v1**, deliberately:
- estimates as ranges, and forecasting from past throughput (beyond the quick
  version on the roadmap's spare-hours track);
- a hard token budget enforced at Send;
- reading pull-request review comments into the record;
- retrospectives (audit C-9);
- multi-repository projects;
- a single button that runs straight from Send to building when the estimate is
  within budget.

## 17. Open items

### 17a. Proposals accepted with the design

These went beyond the decisions accepted earlier. **Sam accepted all seven with
the design, on 2026-09-28.**

1. **Retire the design reviewer** (§3), in M3.
2. ~~**The chat agent may not lock a milestone**~~ **Superseded the same day.**
   Sam made locking a reversible *Mark as shipped* (SPEC-010 SD-11). DEC-004
   Amendment 1 then lets the chat agent mark a milestone as shipped and reopen
   it (§5c, §6).
3. **Bugs as milestone deliverables** (§4, §6).
4. **Checklists owned by the project or an initiative**, and **the chat agent
   may create checklists and jobs** as planning structure (§5c, §6). *Built in
   M5 (SPEC-014); see the notes in §5c and §6.*
5. **ID prefixes for milestones, roadmaps, checklists and tasks**, and the
   **document revision scheme** (§7).
6. **The bug path** (§9): acceptance stands in for G0, the report is the spec,
   the spec reviewer reviews it, and the chat agent may relay a triage
   decision.
7. **Spikes** (§10): a human starts a spike in the UI, any executor runs it, and
   a chat or human spike carries a time box instead of a token budget.

### 17b. Open questions, settled in the spec of the milestone named

- **How agent-implemented work lands in a GitHub project.** It could be
  Subutai's own merge after verification, as today, or a pull request like a
  human's. (M15b)
- **When a projected issue is created**, and what happens to it if its task is
  abandoned. (M15b)
- **How big the decision surfacing cap is.** The proposal is about 150 words per
  decision and 1,500 tokens per dispatch, tuned against transcripts once M6
  exists. (M11)
- **Withdrawing a send** before its spec is written. (M3)
- **How a human issue is marked addressed** in the review loop. (M3)
- **What counts as activity on a claim**, and whether expiry differs for humans
  and the chat AI. (M13)
- **Whether the chat AI may claim estimation work.** (M13)

## 18. What changed from Draft 1

For Sam's re-read. Section numbers are Draft 1's where they say so.

**The workflow:**
- **"Planning" is renamed "Designing"**, and the handover is an approved design
  plus a human's **Send to development**. It is no longer a chat-written spec
  (DEC-006). Draft 1 §12 item 2 is reversed: the spec author stays
  orchestrated.
- **Button names.** Draft 1's "Submit to development" is now **Send to
  development**, and "Start" is now **Start building**. Both are timing
  controls. "Exactly two moments where the system waits" is now "two routine
  buttons".
- **Development planning** is re-listed. There is no separate "Validation" step,
  because validation runs on submission as it always has. The spec is written
  by the spec author, and there is an optional hold for a human.
- **Draft 1's per-run model override at Submit is dropped.** The send screen
  shows models but doesn't change them. Per-role models stay configurable per
  project. Say if the override is still wanted.
- **Spec review is rewritten for DEC-006 Amendment 1**: the agent reviewer is
  the normal approver, humans raise issues, there is an optional hold, and
  there is always at least one reviewer.
- **Minor review findings** now become bug reports in the triage queue.
  Draft 1 recorded them and dropped them.
- **New subsections:** the cascade in §5, and §5c, what the chat agent may do.
  §5c resolves Draft 1's contradiction between "tell the chat agent to approve"
  and "the chat AI has no approve tool".

**The team:** the design reviewer's retirement is carried over from Draft 1,
now marked as a proposal (§17a), because DEC-006 left it open.

**Vocabulary and planning:**
- **§6, planning and tracking, is new.** Draft 1 had these only as vocabulary.
- **Specs are also written in chat, or come from bug reports.**
- **Revision is described with successors throughout.** Approved documents are
  never reopened.

**Documents:**
- **§7 is revised for the GitHub research.** Identity lives in front matter,
  and location is free. Existing documents are adopted where they sit. The
  folder scheme is only a default for new documents.
- *Save & commit* is the main button, lands through the documents fast lane,
  and has a defined author.
- The ID table and the document revision scheme are new; parts are proposals.

**Bugs, spikes and decisions:**
- **§9, bugs:** a dedicated triage queue, GitHub-filed bugs come in, and there
  is a proposed send path.
- **§10, spikes:** the budget is a hard stop with a project default, and there
  are proposals for who starts a spike and how a chat-run spike is bounded.
  Draft 1's "anyone can run a spike" is kept, under DEC-007.
- **§11, decisions:** amendments are now a recognised way to change a
  decision.

**New sections:** §12, working with GitHub; §13, people; §16, the v1 line;
§17, open items. §14 has new entries: no unreviewed specs, no committing
resources from chat, and no two-way GitHub sync.

**Draft 1's §13 open questions are answered** from the decisions of
2026-09-28:
- the ID and folder scheme (§7);
- Save & commit (§7);
- reviewer models for chat-executed work (§5b, DEC-007 decision 10);
- bug triage (§9);
- the spike budget (§10);
- the formal decision records (DEC-006 and DEC-007 are written).

## 19. Consistency review

Draft 2 was reviewed on 2026-09-28 by an independent agent that hadn't written
it. The review was checked against DEC-004 to DEC-007, the roadmap decisions,
DESIGN-008, SPEC-009 as built, and the GitHub research. It raised eleven major
and nine minor findings. All twenty are addressed in this text:

- **The two unflagged contradictions** are now marked as proposals for Sam's
  ruling (§17a items 1 and 2):
  - the design reviewer's retirement;
  - the chat lock ban.
- **The missing mechanics** are now written down:
  - the hold step;
  - what happens when the agent review is off;
  - issues on unsent features;
  - successor vocabulary;
  - document revision IDs;
  - the GitHub merge-order exception;
  - the bug send path;
  - who starts a spike.
- **The v1 line** now places identity, the design reviewer and withdrawing a
  send in named milestones.
- **§18** now lists every change the review found missing.
