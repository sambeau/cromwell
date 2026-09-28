# Subutai: where we are, and the road to done

**Date:** 2026-09-28 (revision 3, same day)
**Author:** Claude, from a review of the repository as it stands at `2ce24b3`
**For:** Sam, and anyone picking the project back up
**Status:** Status report and proposed roadmap. The roadmap is a proposal, not a
decision.

**Revision 3:**
- **Sam accepted all fourteen recommendations in §12 on 2026-09-28.**
- DEC-006 is written up as
  [DEC-006](../decisions/DEC-006-humans-start-development.md), which completes
  M1.
- DEC-007 is accepted in principle, and its document is written in M2.

**What changed in revision 2**, after Sam's comments:

- Orchestration now starts **after the design, when a human presses a button**.
  It no longer starts after a chat-written spec. §6 sets out DEC-006 and
  DEC-007 in detail.
- The revision cascade is **kept**. It is explained in plain terms in §7.
- Milestones, roadmaps and checklists get their own section (§8). They move up
  the roadmap, to the first build work.
- Work runs faster, as background cloud sessions, with decisions answered as
  they arrive (§10).

---

## Executive summary

**Subutai is Cromwell, second edition.** It is the planning, workflow and
orchestration system this repository has been building. The engine stays the
same, and the workflow is clearer, with a new name and some new capabilities.
It is a revision, not a rewrite.

**The engine is built and it works.** Between 21 July and 1 August the project
went from nothing to a working system:

- about 17,000 lines of Go and 155 tests;
- a web command centre;
- an MCP facet for chat agents;
- a pipeline that has taken real features from approved documents to merged
  code against a live AI provider.

Today the full test suite, database tests included, passes under the race
detector in this cloud environment.

**Subutai itself is designed but not decided, and nothing Subutai-specific is
built.**

- The Subutai design, [DESIGN-010](../design/DESIGN-010-subutai.md), is Draft 1
  and unapproved.
- The two decisions it depends on, DEC-006 and DEC-007, are not written.

**The work stopped at a decision point, not a build point.** The last commits
are all design and research. The project paused so the design could be settled,
and it hasn't been settled since. **The bottleneck is decisions, not code.**

**Sam's steer settles the biggest open question, and in the cheapest way.**
Agents write the spec from an approved design, and a human decides when that
starts. That keeps almost all of the most recent code, SPEC-009 Stage 1, which
already writes specs from approved designs. What changes is the trigger. Today
approving a design sets the agents off automatically. Instead, approval becomes
a record, and a person presses **Send to development** when time, agents and
token budget allow.

**Milestones, roadmaps and checklists are half built.** The data model and the
server behind milestones and roadmaps are done and tested. But the web UI can
only *show* them. Creating, filling, ordering and locking them needs the
command line or the API. Checklists and jobs aren't built at all. The roadmap
now puts all of this first, because it needs no decisions and it is what a
human uses to plan and follow the project.

**The path to done is sixteen small milestones in four phases.** Each has one
goal and a plain test for "done". The finish line is:

- DESIGN-010 is approved;
- every change it lists is shipped;
- one real Tickly feature has gone through Subutai from idea to merged code.

---

## Contents

1. [What Subutai is](#1-what-subutai-is)
2. [The design in brief](#2-the-design-in-brief)
3. [What is built](#3-what-is-built)
4. [What is not built](#4-what-is-not-built)
5. [Loose ends from the last push](#5-loose-ends-from-the-last-push)
6. [DEC-006 and DEC-007 in detail](#6-dec-006-and-dec-007-in-detail)
7. [The cascade smoke, explained](#7-the-cascade-smoke-explained)
8. [Milestones, roadmaps and checklists](#8-milestones-roadmaps-and-checklists)
9. [Honest risks](#9-honest-risks)
10. [How to keep it moving](#10-how-to-keep-it-moving)
11. [The roadmap](#11-the-roadmap)
12. [Decisions waiting on Sam](#12-decisions-waiting-on-sam)
13. [Sources](#13-sources)

---

## 1. What Subutai is

Subutai is a planning, workflow and orchestration system for small teams
building large software projects with AI agents. It takes work from a rough idea
to shipped, verified software, and it keeps an honest record of what happened.

It exists because of a lesson from its predecessor. Kanbanzai used an AI agent
as its orchestrator, and under enough load that agent drifted. It did the work
itself instead of delegating, or delegated and then forgot to follow up. The
records then claimed things were done that weren't. So Subutai's founding rule
is that **the orchestrator is ordinary, deterministic code**. It can't lose
track, and it can't be talked out of a rule.

Subutai splits the work into two worlds:

- **Planning is a conversation.** Humans and a strong chat AI (on a
  subscription plan) work out what to build and write the design together.
- **Development is governed.** When a human sends an approved design to
  development, the orchestrator runs API agents through a fixed pipeline:
  - writing the spec, then reviewing it;
  - development planning, then reviewing the plan;
  - estimation;
  - implementation, then code review;
  - verification against the definition of done.

## 2. The design in brief

| Area | What the design says (with revision 2's steer) |
|---|---|
| **Two human gates** | **Send to development** starts the agents on an approved design. **Start building** begins implementation once the plan and estimate are ready. Otherwise humans only hear about exceptions, through checkpoints. |
| **Development planning** | Spec writing → spec review → [optional human spec approval] → dev-plan → plan review → estimate, then stop and wait for *Start building*. |
| **Working ahead in chat** | The planning steps check what already exists. A spec or plan written in chat is used as it is, and that step is skipped. |
| **Executors** | Anyone may *do* a stage: a dispatched agent, the chat AI, or a human. **Nobody may judge their own work.** Verification always runs. |
| **Vocabulary** | Work: Project, Initiative, Feature, Task, Bug, Spike, Job, Checklist. Tracking: Deliverable, Milestone (unordered), Roadmap (ordered milestones). |
| **Documents** | System-minted IDs (`INIT-014`, `FEAT-023`), identity in front matter, starter documents created with each entity, and a browser editor. |
| **Observability** | Full agent transcripts, a feature timeline, review-health numbers, and attribution on everything. |
| **Bugs · Spikes · Decisions** | Bugs are feature-shaped, with human triage. Spikes are budgeted questions with no merge path. Decisions are pushed into agents' prompts. |
| **GitHub** (later research) | Partition, don't synchronise. Subutai owns intent and GitHub owns code. Documents are adopted where they sit, and verification runs after merge. **This is not yet folded into the design.** |

## 3. What is built

| Built | Evidence |
|---|---|
| Deterministic orchestrator: events, rules engine, dispatcher, heartbeat | `internal/rules`, `internal/dispatch`, `internal/bus` |
| Document lifecycle with validation, agent reviews, severity-gated review loops and evidence-checked verification | SPEC-001, SPEC-002 |
| Features, dev-plans, task decomposition, and gates G0 to G5 | `internal/lifecycle/gates.go` |
| Tool host: jailed per-feature worktrees, hash-anchored edits, whitelisted commands, merge to `main` | SPEC-002, DESIGN-006 |
| Token sizing, estimates with confidence, and calibration from actuals | SPEC-003, `internal/sizing` |
| **Milestones and roadmaps: data, server, API, CLI, and read-only pages** | SPEC-003, SPEC-006; see §8 |
| Web command centre (Go templates and htmx) | SPEC-004, SPEC-006, SPEC-007 |
| MCP facet: nine planning tools, and deliberately no approve, submit or start tool | SPEC-008 |
| **Authoring chain**: agents write and review the spec and dev-plan from an approved design; the revision cascade; the heartbeat safety net | SPEC-009 Stage 1 |
| Audit log and token ledger, by construction | Phase 1 onward |

**Proven live:**
- The phase 1 and phase 2 loops.
- The planning slice.
- SPEC-009's main claim: an approved design produced a spec, a reviewed
  dev-plan and three tasks, for 15,976 tokens, then stopped at gate 2
  ([walkthrough](../walkthrough-spec-009-stage1.md)).

**Health today:** `go vet` is clean. `go test -race ./...` passes in full
against a local PostgreSQL 16 in this cloud container.

## 4. What is not built

| Subutai change | State in code |
|---|---|
| The name | **Not started.** |
| A human button that starts development | **Not started.** Today approving a design starts spec writing automatically, if the project has turned authoring on. |
| Optional human approval of specs | **Not started.** Specs are approved by an agent only. |
| Milestone and roadmap editing in the UI; checklists and jobs | **Not started.** See §8. |
| Executors (claim and submit by chat or a human) | **Not started.** |
| Document IDs, front-matter identity, create-with-entity, browser editor | **Not started.** |
| Transcripts, timeline, review health | **Minimal.** Tool calls and tokens are stored. Conversations are held in memory and then lost. |
| Bugs, spikes, decisions as first-class things | **Not started.** |
| GitHub partition, importer, adopt-in-place | **Not started, and not yet designed.** |

## 5. Loose ends from the last push

- **The cascade smoke (SPEC-009 DoD 3) never ran.** Revision 1 of this report
  said to waive it. **That was wrong under Sam's steer.** Keep it and run it
  (§7).
- **The SPEC-007 and SPEC-008 live smokes are waiting on Sam's confirmation.**
- **Documents are awkward to manage from the UI.**
  - A mis-attached document can't be detached.
  - A draft can't be submitted from the document page.
- **The README is well out of date.** It stops at phase 2 and doesn't mention
  Subutai.
- **The smoke project lived in `/tmp`** and needs rebuilding.
- **The cloud environment has no setup step for Postgres.** It worked by hand
  today, so a setup script or a SessionStart hook should do it automatically.
- **The research backlog is still open:**
  - the implementer's search tool;
  - role vocabulary;
  - conventions pushed into prompts;
  - review metrics;
  - a token-based runaway cap;
  - the prompt-cache ordering defect;
  - estimate ranges.

## 6. DEC-006 and DEC-007 in detail

Both are **proposals for Sam to accept, change or reject**. Each one below is
written the way the decision record would read.

### DEC-006: Humans decide when development starts

**Accepted 2026-09-28.** The decision record is
[DEC-006](../decisions/DEC-006-humans-start-development.md). What follows is the
proposal as it was put to Sam.

**The question.** When does the orchestrator take over, and who says so?

**Where this has been.**
- *SPEC-009 as built:* approving a design sets the agents off at once.
- *31 July discussion:* the chat AI writes the spec, and orchestration starts
  when the spec is submitted.
- *Sam's steer, 28 September:* agents write the spec, and a human starts it.

Sam's steer takes the best of both. The strong design conversation stays in
chat. Spec writing, which a good design makes mechanical, goes to cheaper API
agents. And nothing spends agent time or tokens until a person chooses.

**The decision, as proposed:**

1. **Approving a design is a record, not a trigger.** It says "this is what we
   want". It starts nothing, so it can be done safely in the UI or by telling
   the chat agent. The chat agent relays it and the audit trail records "via
   chat".

2. **A human starts development by pressing *Send to development*.**
   - It appears on a feature whose design is approved. That design can be its
     own or its parent initiative's.
   - It also appears on an initiative, to send several of its features at once.
   - It is a web UI button only. The chat agent has no tool that can press it.
     This keeps DEC-005's rule that crossing into development is a human act.

3. **The button exists because resources are limited.** Before confirming, the
   send screen shows:
   - which features are going;
   - the roles and models that will run;
   - a rough token forecast;
   - how many agent slots are free (the project's worker limit, 4 by default).

   The person can send now, or queue work to start as slots free up.

4. **What runs after the button.**
   - The agents write the spec.
   - An agent reviews it against the design.
   - The agents write the development plan, and an agent reviews it.
   - The estimator sizes the work.
   - Everything stops at **Start building**, the second human gate, which is
     unchanged.
   - Any step whose document already exists is skipped. So a spec written in
     chat for a tricky feature is simply used.

5. **Human spec review is optional.**
   - A project setting decides whether specs also wait for a human to approve
     them, after the agent review. The proposed default is **off**, because a
     good design normally makes a tight spec.
   - The send screen can switch it on for a single feature.
   - The agent spec review stays on by default. It is cheap (about 2,900
     tokens in the last live run, against about 4,900 to write the spec). It is
     also the only check between the design and everything built from it. A
     project can switch it off if Sam prefers.

6. **When an approved design is revised, the specs written from it are
   revised too** (the cascade, §7). Features already sent to development are
   rewritten automatically. Features not yet sent are left alone until
   someone sends them.

**What this changes in the code.** This is a small change, because most of
SPEC-009 Stage 1 stays:

- **Kept:**
  - the spec-writing and plan-writing agents;
  - the spec-ready gate (G0);
  - the fidelity check;
  - the cascade;
  - the heartbeat safety net.
- **Changed:**
  - A feature gains a "sent to development" mark.
  - The spec-writing rule waits for that mark as well as an approved design.
  - Approving a design no longer triggers anything on its own.
  - The heartbeat only fills gaps in features that were sent.
- **Added:**
  - the button and its send screen;
  - the optional human spec approval;
  - a narrow MCP tool so the chat agent can relay a design approval.

**What it supersedes.** Nothing that was accepted. DESIGN-010 §5 and §12
(items 2 and 3) are still Draft 1, and they get rewritten to match. The
discussion response's §2a recommendation (chat writes the spec) is withdrawn.

**Small choices inside it** (defaults proposed; see §12):
- the button names;
- the human spec approval default;
- whether the agent spec review can be switched off;
- whether cascade rewrites wait for a new send.

### DEC-007: Who does the work is flexible; who judges it is not

**Accepted in principle 2026-09-28.** The decision document is written in M2.

**The question.** May the chat AI, or a human, do a stage's work themselves,
for example implement a tricky task, rather than leaving it to a dispatched
agent?

**Why it comes up.** When a feature is small but hard, the tempting move is to
open a chat window and say "just fix it". If that work happens outside Subutai,
it gets no review, no record and no verification. Giving it a proper lane
inside the system is safer than a rule people will quietly break.

**The decision, as proposed:**

1. **Every piece of work has an executor**, and the executor is recorded. It
   can be a dispatched agent (with its role and model), the chat AI, or a
   human.
2. **There is only one way in and one way out.**
   - To take a task, you **claim** it. That gives you the working copy and the
     contract.
   - To hand it back, you **submit** it.
   - The state machine refuses anything out of order, exactly as it does for
     agents.
3. **Nobody judges their own work.**
   - The chat AI never gets a tool that approves, reviews, verifies or
     overrides anything.
   - A task done in chat still gets an independent code review by a
     dispatched agent.
   - A spec written in chat still gets the agent spec review, or an explicit
     human verdict.
4. **Verification always runs, and always by the dispatched verifier.** It
   checks every acceptance criterion with evidence. No human or AI can skip it
   or do it in chat.
5. **Claims expire.** A claimed task with no activity raises a "still working
   on this?" checkpoint. This is the same safety net agents already have for
   abandoned work.
6. **Work outside a claim gets noticed.** A commit that touches an active
   feature's files without a claimed task raises a checkpoint.
7. **Chat work is marked as unmeasured.** Subscription tokens are invisible to
   the ledger, so the estimates must not treat that work as free.
8. **Pull-request review counts** (from the GitHub research). When a human
   does a task and a colleague reviews the pull request, that review is the
   independent one. Subutai records it rather than asking an agent for a second
   opinion.

**What it supersedes.** DEC-005's first prohibition, which says the chat facet
may not "implement, review, verify or otherwise perform pipeline work". The
reasoning: kanbanzai failed because the *state* lived in the same drifting AI
context as the work, not because an AI did work. In Subutai the state is held by
code, so the real rule can be enforced directly.

**The honest costs:**
- **Tool limits can't be enforced on a chat agent.** It has the whole
  filesystem, where a dispatched agent has only its tools. The skill, the
  tools' own guidance, the human in the conversation and the unclaimed-commit
  checkpoint reduce this risk but don't remove it.
- **A feature done entirely in chat has had one mind through the whole chain.**
  So reviews should default to a stronger model when the executor was the chat
  AI.
- **A human approving a spec they co-wrote is close to self-judging.** It is
  allowed as a recorded human choice, and the attribution makes it visible.

**How it relates to DEC-006.** They don't overlap. DEC-006 decides *when*
development starts, and only a human can start it. DEC-007 decides *who may do
the work* once it has started.

**When it's needed.** Not until the executors milestone (M13). DEC-006 is
needed first.

## 7. The cascade smoke, explained

**"Smoke" means a live test run.** A smoke is a run on a real project against a
real AI provider, with a person watching. It's different from the automated
tests, which use a scripted fake AI. Each Cromwell spec lists the live runs its
definition of done needs. SPEC-009 needs two:

- **DoD 2, done on 31 July:** approve a design and watch specs, plans and tasks
  appear.
- **DoD 3, never run:** the cascade.

**"Cascade" means the revision cascade.** It is what happens when an approved
design changes *after* agents have already written specs from it. Without it,
those specs would silently describe the old design.

The cascade works like this:

1. The revised design is approved.
2. Subutai finds every spec written from it.
3. **If only one spec is affected**, that spec is retired straight away,
   together with its development plan. There's nothing to ask about.
4. **If several are affected**, one inbox question lists them. For each, you
   answer *keep* (the change doesn't affect it) or *redo*.
5. Each redone spec is rewritten by the spec agent against the new design. It
   is reviewed as normal, and a new plan follows. Existing tasks are
   reconciled, not duplicated.
6. If a feature is already being built, the existing "revised while in
   progress" question is asked as well.

**Should we keep it? Yes.** Revision 1 of this report suggested waiving the
live test, because it assumed specs would be written in chat, where nothing
would rewrite them. Under Sam's steer, agents write specs from designs, so the
cascade is exactly what stops a revised design leaving stale specs behind.

- The code is built.
- The automated test `TestDesignRevisionCascade` passes.
- Only the live run is missing.

**One small change goes with DEC-006.** Features already sent to development
are rewritten automatically. Features not yet sent are left until someone sends
them, so nothing spends tokens without a person's say-so.

**Recommendation:** run the cascade smoke once DEC-006's button is in (M3), so
it tests the flow as it will ship.

## 8. Milestones, roadmaps and checklists

These are how people plan and follow the project, so they need to be solid
early.

### What they are

- **A milestone** is an unordered set of things that ship together: features,
  whole initiatives, other milestones and, later, checklists. Together these
  are called *deliverables*.
  - Membership is live. A new feature under an included initiative joins
    automatically.
  - While open, a milestone shows progress two ways: *X of Y deliverables
    done*, and *how much of the estimated token work is done*.
  - **Locking** freezes it. The system takes a permanent snapshot of what
    actually shipped. It can only lock once at least one deliverable is done
    (gate G4), and it can never be unlocked.
- **A roadmap** is an ordered list of milestones. The order means whatever the
  planner wants, such as sequence or priority. Each entry shows its milestone's
  progress.
- **A checklist** is a list of **jobs**: things a *human* has to do, such as
  sign up for an account, find an API key or choose an icon. Each job is ticked
  by hand. A checklist can be a milestone deliverable, so a release can't be
  called done while the human chores are still open.
- **Ownership.** Milestones and roadmaps belong to the project or to an
  initiative, so planning sits at the level it's about. What they *contain* can
  come from anywhere in the tree.

### What's built today

| Piece | State |
|---|---|
| Milestone and roadmap data, including snapshots and owners | **Built** (migrations 0004, 0005) |
| Server logic: create, add and remove members, lock (G4), order roadmaps | **Built and tested**, all audited |
| API and command line | **Built.** `cromwell milestone …` and `cromwell roadmap …` |
| Web pages for a milestone and a roadmap, with two-way progress | **Built, read-only** |
| Milestones and roadmaps listed on the project and initiative pages | **Built, read-only** |
| **Creating or editing a milestone or roadmap in the web UI** | **Not built.** It was deliberately left to the workflow surface's "Stage B", which was never specced |
| **Chat tools for milestones and roadmaps** | **Not built.** The chat project manager can't create or fill them |
| **Checklists and jobs** | **Not built.** The schema reserves the member type and nothing more |
| The size roll-up tree and the filterable work list | **Not built.** Also Stage B |

So today a person can *see* milestones and roadmaps in the UI, but has to use
the command line to create or change one. The command line is itself scheduled
for retirement (DEC-003). For the part of Subutai that humans plan with, that's
the most visible gap.

### Where they sit in the plan

Revision 1 of this report put checklists late, bundled with spikes. Revision 2
moves all of this to the front:

- **M4 — Milestones and roadmaps you can edit.** Create, fill, reorder and
  lock them from the web UI (from the owner's page, and "add to milestone" from
  any feature or initiative). Add matching chat tools so the chat project
  manager can do the same.
- **M5 — Checklists and jobs.** Human to-do lists that count towards
  milestones, ticked in the UI or by telling the chat agent. Ticking is
  recorded as a human act "via chat".
- **M6 — See the work.** The feature timeline and transcripts, which tell you
  how each deliverable is actually progressing.

**None of these three need a decision from Sam before starting.** The
[Stage B entry criteria](spec-007-stage-b-entry-criteria.md) already list what
has to be settled, and every server method already exists. They can run as
background sessions while DEC-006 is being decided.

**DESIGN-010 needs one more section** on planning and tracking. Today it only
lists these terms in its vocabulary table. That section goes into M2.

## 9. Honest risks

1. **The code and the design disagree until DEC-006 is made.** Now that the
   direction is clear, this is small, but it should be written down before
   anyone changes the authoring chain.
2. **The revision could turn into a rewrite.** DESIGN-010 plus the GitHub note
   add up to about sixteen changes. A clear v1 cut line and small milestones
   guard against this.
3. **DESIGN-010 needs a Draft 2** that matches DEC-006. It also has to resolve
   three conflicts with the GitHub research:
   - per-initiative folders, versus adopting documents where they already sit;
   - per-task code review, versus human pull-request review;
   - verification per task, versus after merge.
4. **Subutai is single-user, but it's for small teams.** Per-user identity
   isn't designed. It matters before a second person uses it.
5. **Chat work leaves holes in the token ledger.** Estimation must respect the
   "unmeasured" mark.
6. **Parallel background work can collide.** Two sessions touching the same
   files make merge pain. The plan pairs up only milestones that touch
   different parts of the code.

## 10. How to keep it moving

The project moved fast when it had Sam's attention: fifty commits in twelve
days. It stopped completely when that attention went elsewhere. Building by
agents is fast. The scarce resource is Sam's decisions and reviews, so the plan
keeps those small and puts them first.

**Work in the background, in the cloud.**

- **Each build milestone runs as its own cloud session.** It writes the spec,
  builds, runs the full test suite, and opens a pull request. The full suite,
  database tests included, ran green in this cloud container today.
- **Add a setup step so every session starts Postgres automatically**, before
  handing work off to background sessions.
- **Run two lanes at once when they touch different code.** One lane is
  decisions and the authoring chain. The other is planning surfaces and
  observability.

**Keep Sam's part quick and asynchronous.**

- **Decisions arrive as short, answerable questions**, each with a
  recommendation. Answer when they arrive; there's no need to wait for a weekly
  slot.
- **Each milestone ends in one pull request and one handoff note.** Reviewing a
  milestone means reading the note, trying the feature, and merging.
- **Live smokes are the only thing that needs Sam present.** Batch them, one
  session per few milestones.

**Protect the shape.**

- New ideas go "after v1" unless they block the Tickly pilot.
- Tick milestones off in this document as they finish.

**What that means for speed.** Revision 1 assumed about one sprint a
fortnight. With background sessions and prompt answers, a small milestone can
realistically turn around in a few days, and two lanes can run at once. Here
are the scenarios:

| Pace | "Subutai usable" (end of phase B) | Subutai v1 (Tickly pilot) |
|---|---|---|
| Decisions within a day or two, two lanes | about 2 months | about 4 months |
| Decisions weekly, one lane | about 4 months | about 7 months |
| Revision 1's pace (one sprint a fortnight) | about 8 months | about 13 months |

These are estimates, not promises. Sam's review time sets the pace far more
than build time does.

## 11. The roadmap

**Sixteen milestones in four phases.** Sizes are in sprints:

- **S** is one sprint;
- **M** is two sprints;
- **L** is three or more, and is split before starting.

A sprint is a short focused block: one to three background build sessions, plus
Sam's review. **Lane** says which of the two parallel lanes a milestone belongs
to.

### Phase A: restart and decide

**M0 — Restart** · S · ✅ **done 2026-09-28**, see the [M0 handoff](handoff-M0-2026-09-28.md)
*Goal: make the project safe to pick up again.*
- Add a cloud setup step that starts Postgres. The suite already passes by
  hand.
- Rebuild the smoke project.
- Close the SPEC-007 and SPEC-008 smokes.
- Update the README.

*Done when:* a fresh cloud session can run the full suite unaided, and the
README is honest.

**M1 — Decide DEC-006** · S · decision · ✅ **done 2026-09-28**
*Goal: settle when development starts, and who starts it.*
- Accept, change or reject §6's DEC-006, including its small choices.

*Done when:* DEC-006 is accepted. This unblocks M3. **Done:**
[DEC-006](../decisions/DEC-006-humans-start-development.md) was accepted with
the recommended defaults.

**M2 — Design Draft 2, approved** · M · decision
*Goal: an approved Subutai design with a clear v1 cut line.*
- Rewrite DESIGN-010 §5 and §12 to match DEC-006.
- Add a planning-and-tracking section.
- Fold in the GitHub research.
- Answer the §13 open questions.
- Write DEC-007.

*Done when:* DESIGN-010 is approved and DEC-007 is accepted.

**In progress (2026-09-28):**
- [DEC-007](../decisions/DEC-007-the-judgement-boundary.md) is written up from
  the accepted proposal.
- [DESIGN-010 Draft 2](../design/DESIGN-010-subutai.md) is written and
  consistency-reviewed. All twenty review findings are addressed (§19). It is
  waiting for Sam's approval.
  - Its §17a lists seven proposals that go beyond the accepted decisions.
    Approving the draft approves them, unless Sam says otherwise.
  - Its §18 lists what changed from Draft 1.

### Phase B: the Subutai core

**M3 — Send to development** · M · needs M1 · lane 1
*Goal: a human decides when agents start work on a design.*
- Approving a design no longer triggers anything.
- Add the *Send to development* button on features and initiatives, with the
  send screen: features, models, token forecast and free slots.
- Spec writing waits for the button.
- Human issues on specs (must-address), and an optional hold for a human.
- Chat relay tools for human verdicts, issues and review requests.
- Add Submit, Revise and Detach on the document page.
- Retire the design reviewer, if Sam confirms DESIGN-010 §17a item 1.
- Decide whether a send can be withdrawn.

*Done when:* a live run shows that approving a design starts nothing, and that
pressing *Send* produces a reviewed spec, plan, tasks and an estimate, then
stops. The cascade smoke (§7) runs in the same session.

**M4 — Milestones and roadmaps you can edit** · M · no dependencies · lane 2
*Goal: plan the project from the web UI and from chat.*
- Create a milestone or roadmap from a project or initiative page.
- Add and remove members from either end.
- Reorder roadmap entries.
- Lock a milestone, showing why when G4 refuses.
- Add chat tools for the same actions, except locking. Locking stays in the UI
  pending Sam's ruling on DESIGN-010 §17a item 2.

*Done when:* a person can build a two-milestone roadmap for an initiative
entirely in the browser, and the chat agent can do the same.

**M5 — Checklists and jobs** · S · needs M4 · lane 2
*Goal: human chores count towards milestones.*
- Add checklists and jobs with a table and pages.
- Tick jobs in the UI, or relay them from chat, with the tick audited.
- A checklist can be a milestone deliverable.
- Progress and the lock gate count jobs.

*Done when:* a milestone won't show as done, and won't lock as complete, while
one of its jobs is unticked.

**M6 — See the work** · M · no dependencies · lane 2
*Goal: answer "how is it going?" and "what exactly happened?".*
- Store and show a full transcript for every agent dispatch.
- Add a per-feature timeline of major moments.
- Add review-health numbers.

*Done when:* from a feature's timeline you can click through to what any agent
was told, did and concluded.

**M7 — Rename to Subutai** · S · needs M1 · either lane, run alone
*Goal: one name everywhere, changed once.*
- Change the module, binary, config folder (still reading `.cromwell/` for one
  release), environment variables and UI copy.
- Do it in one commit, when no other session is open.

*Done when:* `subutai serve` works and the suite is green.

**M8 — Documents with identity** · M · needs M2 · lane 1
*Goal: every document has an ID, and creating work creates its documents.*
- Mint IDs from the database and store them in front matter.
- Create a starter document with each new entity.
- Adopt existing documents where they sit.

*Done when:* moving a file on disk doesn't detach it.

**M9 — Edit in the browser** · S · needs M8 · lane 1
*Goal: fix documents without leaving the UI.*
- A Markdown editor with preview, *Save*, and *Save & commit*.
- Refuse to overwrite a file that changed on disk.
- Warn when editing a document that has work in flight.

*Done when:* browser edits and vim edits can't silently overwrite each other.

**M10 — Chat as a proper seat** · S · needs M3 · lane 1
*Goal: the chat AI prepares work, and humans can rule on any document.*
- Add chat tools to submit a document and to ask for an early review.
- Allow a human verdict on any document type.
- The send screen shows which steps are already done.

*Done when:* a feature whose spec was written in chat skips spec writing and
flows to *Start building*, and every document shows who wrote it.

**The "Subutai usable" line.** The product team can plan milestones and
roadmaps, design in chat, send work to development when resources allow, and
follow it through to done.

### Phase C: complete the model

**M11 — Decisions** · S · needs M8
*Goal: the project remembers why, and agents are told.*
- `DEC-nnn` documents with a viewer.
- Relevant decisions and conventions pushed into prompts, with a size cap.

*Done when:* an agent's transcript shows the relevant decisions in its prompt.

**M12 — Bugs** · M · needs M8
*Goal: anyone reports, humans decide.*
- Human triage.
- Agents can file bugs, and minor review findings become bug reports.
- Accepted bugs travel the normal pipeline.

*Done when:* an agent-filed bug is triaged, fixed and verified.

**M13 — Executors** · M · needs M2, M10
*Goal: chat and humans can do work through the front door.*
- Claim and submit, with the executor recorded.
- Stale claims and unclaimed commits are flagged.
- Verification is never done in chat.

*Done when:* a task done in chat gets the same review and verification as an
agent's.

**M14 — Spikes** · S · needs M8
*Goal: a budgeted question that can't ship code.*
- A token budget.
- A findings document.
- A throwaway worktree, and no merge path.

*Done when:* a spike hits its budget and stops, and nothing can merge its code.

### Phase D: real-world adoption

**M15 — GitHub and the Tickly pilot** · L, split into three · needs M8, M12
- **M15a — Import and adopt:** an importer with a label map and a dry run, and
  registering Tickly's documents where they sit.
- **M15b — GitHub projection:** an issue for each task, merged pull requests
  observed, developer-filed bugs arriving for triage, and verification after
  merge.
- **M15c — The pilot:** one real Tickly feature from idea to merged code,
  written up. Per-user identity lands before the pilot if a second person
  takes part (decision 14).

*Done when:* the pilot walkthrough is recorded. **This is Subutai v1.**

### At a glance

```
Phase A   M0 Restart ── M1 DEC-006 ── M2 Design approved
                  │
Phase B   lane 1: M3 Send to development ── M10 Chat seat     M8 Documents ── M9 Editor
          lane 2: M4 Milestones & roadmaps ── M5 Checklists    M6 See the work
          alone:  M7 Rename
                                              ══ "Subutai usable"
Phase C   M11 Decisions · M12 Bugs · M13 Executors · M14 Spikes
Phase D   M15a Import ── M15b GitHub ── M15c Tickly pilot  ══ v1
```

**Suggested first moves, in parallel:**
- Sam reads §6 and answers DEC-006 (M1).
- A background session does M0, then starts M4.

### The spare-hours track

These are small, independent jobs for any idle session:

- the implementer's search tool;
- the prompt-cache ordering fix;
- a per-task token cap in place of the dollar cap;
- forecasting from past throughput;
- vocabulary and anti-patterns for the surviving roles;
- effort expectations in prompts.

## 12. Decisions waiting on Sam

**All fourteen are accepted, as recommended (Sam, 2026-09-28).** Decisions 3,
4 and 6 were then amended the same day by
[DEC-006 Amendment 1](../decisions/DEC-006-humans-start-development.md#amendment-1--reviews-and-relay-2026-09-28). Decisions 1 to
6 are recorded in [DEC-006](../decisions/DEC-006-humans-start-development.md).
The rest take effect in the milestones named. The table is kept as the record,
and new decisions get added below it as they arise.

| # | Decision | Recommendation | Unblocks |
|---|---|---|---|
| 1 | DEC-006 as in §6 | Accept | M3 |
| 2 | Button names | *Send to development* (gate 1) and *Start building* (gate 2) | M3 |
| 3 | Human spec approval: default on or off? | *Amended:* agent review is the normal approver. Humans point out issues at any time. Holding a spec for a human is optional, off by default, per project or per send | M3 |
| 4 | Can the agent spec review be switched off? | *Amended:* yes per project, but then every spec waits for a human. There is always at least one reviewer | M3 |
| 5 | After a design revision, rewrite specs automatically? | Yes, for features already sent. Otherwise wait for a send | M3 |
| 6 | May chat relay design approval and tick jobs? | *Amended:* chat may relay any human verdict, issue, review request or job tick, quoting the human. Never Send, Start building or a gate override | M3, M5 |
| 7 | Milestone and roadmap editing: modal or page? | Modal from the owner's page, as DESIGN-008 already says | M4 |
| 8 | Do jobs carry a note, owner or due date? | A note and who ticked it, for now. Owner and date later | M5 |
| 9 | ID and folder scheme | Prefixed IDs, identity in front matter, folders only as the default for new documents | M8 |
| 10 | DEC-007 as in §6 | Accept, before M13 | M13 |
| 11 | Editor's main button | *Save & commit* | M9 |
| 12 | Bug triage surface | A dedicated queue, with a count in the inbox | M12 |
| 13 | Spike budget behaviour | Hard stop, with a project default | M14 |
| 14 | Per-user identity in v1? | Yes, before the pilot if a second person joins | M15 |

## 13. Sources

- [Subutai discussion document](../vision/Cromwell%20R2%20Vision%20Subutai.md)
- [DESIGN-010: Subutai](../design/DESIGN-010-subutai.md) (Draft 1)
- [Response to the Subutai discussion](subutai-discussion-response-2026-07-31.md)
- [Subutai and GitHub](../research/subutai-and-github.md)
- [SPEC-009: the authoring chain](../specs/SPEC-009-the-authoring-chain.md): FR-4 (the invariants), FR-9 (the cascade), DoD
- [Handoff: SPEC-009 Stage 1 code-complete](handoff-2026-07-30-stage1-complete.md)
- [Walkthrough: SPEC-009 Stage 1 live](../walkthrough-spec-009-stage1.md)
- [DESIGN-008: the workflow surface](../design/DESIGN-008-the-workflow-surface.md): §5.1b, §9, D-9 to D-12
- [Workflow surface Stage B entry criteria](spec-007-stage-b-entry-criteria.md)
- [DEC-005: the orchestration boundary](../decisions/DEC-005-the-orchestration-boundary.md)
- [Research conformance audit](research-conformance-audit-2026-07-29.md)
- The code at `2ce24b3`. `go test -race ./...` was run in full against
  PostgreSQL 16 on 2026-09-28.
