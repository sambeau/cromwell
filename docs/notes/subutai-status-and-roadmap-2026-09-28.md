# Subutai: where we are, and the road to done

**Date:** 2026-09-28
**Author:** Claude, from a review of the repository as it stands at `2ce24b3`
**For:** Sam, and anyone picking the project back up
**Status:** Status report and proposed roadmap. The roadmap is a proposal, not a
decision.

---

## Executive summary

**Subutai is Cromwell, second edition.** It is the planning, workflow and
orchestration system this repository has been building, with the same engine and
a clearer workflow, a new name, and a set of new capabilities. It is a revision,
not a rewrite.

**The engine is built and it works.** Between 21 July and 1 August the project
went from nothing to a working system: about 17,000 lines of Go, 155 tests, a
web command centre, an MCP facet for chat agents, and a pipeline that has taken
real features from approved documents to merged code against a live AI
provider. The code builds, `go vet` is clean, and the non-database tests pass
today.

**Subutai itself is designed but not decided, and nothing Subutai-specific is
built.** The Subutai design, [DESIGN-010](../design/DESIGN-010-subutai.md), is
Draft 1 and unapproved. The two decisions it depends on (DEC-006 and DEC-007)
are not written. There is no Subutai specification, and none of the design's
nine headline changes exist in code.

**The work stopped at a decision point, not a build point.** The last commits
(31 July to 1 August) are all design and research. The project paused so the
design could be settled, and it hasn't been settled since. **The bottleneck is
decisions, not code**, and the plan below is built around that.

**One thing makes restarting urgent rather than optional.** The most recent
code puts the start of orchestration at *design approval*. An approved design
automatically sets agents writing the spec. Subutai moves that point to the
*spec*: people write the spec in chat, and orchestration starts when they press
**Submit to development**. So the newest code points the opposite way to the
design. The longer that stands, the harder the codebase is to reason about.

**The path to done is thirteen small milestones in four phases.** Each one has
a single goal, a plain test for "done", and is sized to fit a short sprint
alongside other work. The first two need only Sam's decisions and no code, and
they unblock almost everything else. A parallel track of small, independent
jobs is there for spare hours.

**The proposed finish line:** DESIGN-010 is approved, every change it lists is
shipped, and one real Tickly feature has gone through Subutai from idea to
merged code.

---

## Contents

1. [What Subutai is](#1-what-subutai-is)
2. [The design in brief](#2-the-design-in-brief)
3. [What is built](#3-what-is-built)
4. [What is not built](#4-what-is-not-built)
5. [Loose ends from the last push](#5-loose-ends-from-the-last-push)
6. [Readiness: what can start now, what needs a decision](#6-readiness-what-can-start-now-what-needs-a-decision)
7. [Honest risks](#7-honest-risks)
8. [The roadmap](#8-the-roadmap)
9. [How to keep it moving](#9-how-to-keep-it-moving)
10. [Decisions waiting on Sam](#10-decisions-waiting-on-sam)
11. [Sources](#11-sources)

---

## 1. What Subutai is

Subutai is a planning, workflow and orchestration system for small teams
building large software projects with AI agents. It takes work from a rough idea
to shipped, verified software, and it keeps an honest record of what happened
along the way.

It exists because of a lesson from its predecessor, kanbanzai, which used an AI
agent as the orchestrator. Under enough load that agent drifted. It did the work
itself instead of delegating, or delegated and forgot to follow up, and the
records claimed things were done that weren't. Subutai's founding rule is that
**the orchestrator is ordinary, deterministic code**. It can't lose track, and
it can't be talked out of a rule.

Subutai splits the work into two worlds:

- **Planning is a conversation.** Humans and a strong chat AI (on a
  subscription plan) work out what to build and write the design and the spec
  together, at whatever pace the thinking needs.
- **Development is governed.** Once the spec is handed over, the orchestrator
  runs a fixed pipeline of API agents: spec review, development planning, plan
  review, estimation, implementation, code review, and verification against
  the definition of done.

The **spec is the handover**. Everything before it is flexible, and everything
after it follows the rules.

## 2. The design in brief

DESIGN-010 describes the target. In summary:

| Area | What the design says |
|---|---|
| **Two human gates** | *Submit to development* hands over the spec and starts intake. *Start* begins implementation. Humans are otherwise only interrupted by exceptions (checkpoints). |
| **Intake** | Validation → spec review (the "border guard") → development plan → plan review → estimation, then stop and wait for *Start*. |
| **Working ahead in chat** | Intake steps check state instead of always running. A dev-plan written in chat, or a review asked for early, is simply skipped at submit time. |
| **Executors** | Anyone may *do* a stage (a dispatched agent, the chat AI, or a human) through claim → work → submit. **Nobody may judge their own work**, and verification is always done by the dispatched verifier. |
| **Vocabulary** | Work: Project, Initiative, Feature, Task, Bug, Spike, Job, Checklist. Tracking: Deliverable, Milestone (unordered), Roadmap (ordered milestones). |
| **Documents** | System-minted IDs (`INIT-014`, `FEAT-023`, `BUG-007`, `DEC-006`), identity in front matter, creating an entity creates its starter document, and a simple Markdown editor in the browser with *Save* and *Save & commit*. |
| **Observability** | Full transcripts of every agent dispatch, a feature timeline of major moments, review-health numbers, and attribution on everything. |
| **Bugs** | Feature-shaped, with a human triage step at the front. Agents can file them, and minor review findings become bug reports. |
| **Spikes** | A question with a token budget. It produces a findings document and has no merge path at all. |
| **Decisions** | `DEC-nnn` documents, never edited once accepted, and pushed automatically into agents' context. |
| **Deliberately not** | No AI orchestrator, no self-approval, no skippable verification, no knowledge base, no rich text, no multi-project installs. |

A later research note, [Subutai and GitHub](../research/subutai-and-github.md),
tested the design against Tickly, the first real project Subutai is meant to
manage. It proposes seven more changes. The biggest are these:

- **Documents are adopted where they sit.** Identity lives in front matter, and
  location is free. As drafted, DESIGN-010 §6 would move every existing document.
- **GitHub keeps the code, and Subutai keeps the intent.** Subutai projects a
  simple issue for each task and watches for merged pull requests. Nothing is
  synchronised both ways.
- **Verification happens at the feature level, after merge.** So it never
  blocks a developer's pull request.

These changes are not yet folded into DESIGN-010.

## 3. What is built

Everything below is Cromwell as it stands. It all carries over into Subutai.

| Built | Evidence |
|---|---|
| Deterministic orchestrator: events, rules engine, dispatcher, heartbeat | `internal/rules`, `internal/dispatch`, `internal/bus` |
| Document lifecycle with validation, agent reviews, severity-gated review loops, evidence-checked verification | SPEC-001, SPEC-002; commits `a116faa`, `0718edc` |
| Features, dev-plans, task decomposition, gates G0 to G3 | SPEC-002, SPEC-009; `internal/lifecycle` |
| Tool host: jailed per-feature worktrees, hash-anchored edits, whitelisted commands, merge to `main` | SPEC-002, DESIGN-006; `internal/toolhost` |
| Token sizing, estimates with confidence, calibration from actuals | SPEC-003; `internal/sizing` |
| Milestones (with frozen snapshots) and roadmaps | SPEC-006; migration 0004 |
| Web command centre (Go templates and htmx): dashboard, inbox, work, documents, initiative, feature, task, milestone and roadmap pages, approve and send-back on documents | SPEC-004, SPEC-006, SPEC-007 |
| MCP facet with nine planning tools (create and update initiatives and features, attach documents, read the tree). **No approve, submit or start tool, deliberately.** | SPEC-008; `internal/server/mcp.go` |
| Authoring chain: design approval sets off automatic spec and dev-plan writing, the revision cascade, freshness, and the heartbeat safety net | SPEC-009 Stage 1, code-complete 30 July |
| Audit log and token and cost ledger, by construction | Phase 1 onward |
| A research corpus of eleven reference documents | `docs/research/` |

**Proven live** against a real provider: the phase 1 and phase 2 loops, the
planning slice, and SPEC-009's main claim. An approved design produced a spec,
a reviewed dev-plan and three tasks for 15,976 tokens, then stopped at gate 2
([walkthrough](../walkthrough-spec-009-stage1.md)).

**Health today:** `go build ./...` and `go vet ./...` pass. `go test -short
./...` passes every package. The 64 Postgres integration tests skip without a
database and need the local Supabase stack to run.

## 4. What is not built

Measured against DESIGN-010 §12, the list of what Subutai changes:

| # | Subutai change | State in code |
|---|---|---|
| 1 | The name | **Not started.** The module, binary, `.cromwell/` folder and `CROMWELL_*` variables are all still Cromwell. |
| 2 | Move the seam to *Submit to development* | **Not started, and the code points the other way.** Design approval triggers spec authoring (`internal/rules/rules.go`, `internal/server/authoring.go`). The `spec-author` role is still in the starter pack. There is no Submit action. |
| 3 | Spec reviewer as border guard; retire the design reviewer | **Partial.** A spec reviewer exists and already checks fidelity to the design, but it isn't the first step of an intake. The design reviewer is still shipped. |
| 4 | Executors (claim and submit by chat or human) | **Not started.** Tasks have no executor field, and there is no claim path outside the dispatcher. |
| 5 | Pre-completion (intake skips finished steps) | **Mostly free.** Gates already read document state, and authoring only runs when a document is missing. It needs the Submit screen to show it. |
| 6 | Document IDs, front-matter identity, create-with-entity, browser editor | **Not started.** A document's identity is its path. Documents can only be attached, not created. The UI says documents are "written in your own editor". |
| 7 | Transcripts, timeline, review health, attribution | **Minimal.** Tool calls, tokens, role and model are stored. Full conversations are kept in memory only and then lost. Pages show only the last ten audit events. |
| 8 | Bugs, spikes and decisions as first-class things | **Not started.** There is a reserved `defect` value and nothing else. |
| — | Checklists and jobs | **Not started.** Enum values only, marked deferred. |
| — | GitHub partition, adopt-in-place, importer (from the GitHub research) | **Not started, and not yet in the design.** |

## 5. Loose ends from the last push

Small items left open when work paused. Each one needs a ruling or a short
session.

- **SPEC-009 DoD 3, the revision-cascade smoke, never ran.** It tests
  re-authoring a spec after a design changes. Under Subutai specs aren't
  re-authored by agents, so the test as written is moot. **Recommend: waive it
  formally**, and design the cascade's Subutai form (flag the spec as stale and
  ask a human) as part of moving the seam.
- **SPEC-007 and SPEC-008 live smokes are waiting on Sam's confirmation.**
- **There is no way to detach a document that was attached by mistake.** During
  the last smoke it had to be removed by hand in the database.
- **You can't submit a draft document from the web UI.** The last smoke had to
  use the API directly.
- **The README is well out of date.** It says phases 1 and 2 are complete and
  phase 3 is next, but phases 3 and 4 and SPEC-006 to SPEC-009 have shipped
  since. It doesn't mention Subutai.
- **The smoke-test project lived in `/tmp`** and is almost certainly gone. The
  local database setup needs checking on a fresh checkout.
- **Research backlog, still open** (from the
  [conformance audit](research-conformance-audit-2026-07-29.md) and the
  research notes):
  - The implementer still has no search tool (C-6a).
  - The surviving roles lack vocabulary and named anti-patterns (C-3/C-4). Only
    the roles Subutai retires have them.
  - Project conventions aren't surfaced into prompts (C-6).
  - There are no review metrics (C-7) and no retrospectives (C-9).
  - The runaway cap is in dollars on a token system.
  - A dispatch-prompt ordering defect defeats the prompt cache.
  - Estimates are single numbers where the research recommends ranges.

## 6. Readiness: what can start now, what needs a decision

| Ready to build now (design is clear enough) | Ready for a spec once one ruling is made | Needs design work first |
|---|---|---|
| Agent transcripts, and a viewer for them | Moving the seam (needs DEC-006) | Executors and claim/submit (needs DEC-007, which revises DEC-005) |
| Feature timeline | Document IDs and front-matter identity (needs the ID scheme settled) | GitHub partition: projection, importer, inbound bugs |
| Review-health metrics | Create-with-entity and adopt-in-place | Spikes (what happens when the budget trips) |
| Detach, and Submit/Revise on the document page | Browser editor (needs *Save* vs *Save & commit* as the default) | Checklists and jobs (in the vocabulary, but no design section) |
| Implementer search tool, prompt-cache ordering fix, a token-denominated cap | Decisions as documents with prompt surfacing (needs the size cap) | Per-user identity (see risk 5) |
| The rename (mechanical, once decided) | Bugs (needs the triage surface chosen) | |

## 7. Honest risks

1. **The code and the design disagree about where orchestration starts.** This
   is the main reason to restart with the seam decision. Until it's made, every
   change to the authoring chain might be building on something that will be
   removed.

2. **The revision could turn into a rewrite.** DESIGN-010 plus the GitHub note
   come to roughly sixteen changes. The discussion response warned against the
   pause "growing into a rewrite", and the roadmap guards against it with a
   clear cut line (§8) and one slice at a time.

3. **DESIGN-010 has a few internal contradictions** that will cause trouble at
   spec time if they aren't fixed first:
   - §5 lets you approve a design "by telling the chat agent", but §5b and §11
     say the chat AI never gets a tool that can approve anything. After the
     seam moves, approving a design is harmless, so a narrow `approve_design`
     relay is probably fine. It needs to be said explicitly.
   - §6 puts documents in per-initiative folders, while the GitHub research
     says documents must be adopted where they already sit.
   - §5 has code review on every task, while the GitHub research has human pull
     request review standing in for it when a human did the work.
   - Checklists and jobs are in the vocabulary but have no section of their
     own.

4. **Knowledge fades over a pause.** Eight weeks have passed. The handoff notes
   are good, which helps, but the local environment and the smoke project need
   rebuilding before any live test.

5. **Subutai is single-user, but it's for small teams.** DESIGN-010 assumes
   "you as the author" on commits and a human verdict recorded as such. The
   system has one operator identity. Per-user identity hasn't been designed,
   and it matters before a second person uses it.

6. **Chat-executed work leaves holes in the token ledger.** The design accepts
   this and marks such work as unmeasured. Estimation needs to respect that
   flag, or its calibration will slowly get worse.

## 8. The roadmap

### How it's shaped

- **Thirteen milestones in four phases.** Each has one goal, a plain test for
  "done", and what it depends on.
- **Sizes are in sprints.** A sprint here means one short, focused block of
  work, roughly one to three AI-assisted build sessions plus Sam's review. That
  is **S** (one sprint), **M** (two) or **L** (three, and split before starting).
  Past pace suggests this is realistic: SPEC-009 Stage 1 was built in about two
  days.
- **Decision milestones cost Sam's reading time, not build time.** They are
  the cheapest milestones and they unblock the most.
- **Spec before code, every time.** Each build milestone starts with a short
  spec and its consistency review, as the project already does.
- **Phase C and phase D can be reordered** once phase B is done. Phase A must
  come first.

### Phase A: restart and decide

**M0 — Restart** · S · no dependencies
*Goal: make the project safe to pick up again.*
- Confirm the full test suite (with the database) is green on a fresh checkout,
  and rebuild the smoke project.
- Close the SPEC-007 and SPEC-008 smokes, and formally waive SPEC-009 DoD 3
  (§5).
- Update the README status and link this report.

*Done when:* `go test -race ./...` is green against the local database, the
README is honest, and the open smokes are closed.

**M1 — Decide the seam (DEC-006)** · S · decision only
*Goal: settle where orchestration starts.*
- Write and accept DEC-006:
  - Orchestration begins at *Submit to development*.
  - Specs are written in chat.
  - Spec review is the first step of intake.
  - Design approval is a record, not a trigger, and may be relayed from chat.
- Rule on DESIGN-010's §5/§5b contradiction as part of this.

*Done when:* DEC-006 is accepted. This single decision unblocks M3, M4 and M8.

**M2 — Finish the design (DESIGN-010 draft 2, DEC-007)** · M · decisions only
*Goal: an approved Subutai design with a clear cut line.*
- Fold in the GitHub research: adopt-in-place, feature-level verification after
  merge, and the optional forge projection.
- Fix the contradictions in §7.
- Answer the §13 open questions. Each already has a recommended default in the
  discussion response.
- Mark what is v1 and what comes later.
- Write DEC-007 (the judgement boundary: *who does the work is flexible; who
  judges it is not*). It partly supersedes DEC-005.

*Done when:* DESIGN-010 is approved and DEC-007 is accepted.

### Phase B: the Subutai core

**M3 — Move the seam** · M · needs M1
*Goal: orchestration starts when a human submits the spec.*
- Retire the design-approval trigger, the `spec-author` role, orchestrated
  `write-spec`, and the design reviewer.
- Add **Submit to development** (web UI first).
- Intake runs validation, spec review, dev-plan, plan review and estimation,
  then stops at gate 2.
- Add document-page Submit, Revise and Detach.
- Rework the revision cascade to flag a stale spec rather than rewrite it.

*Done when:* a live smoke shows that submitting a spec produces a reviewed plan,
tasks and an estimate, then stops and waits for *Start*.

**M4 — Rename to Subutai** · S · needs M1
*Goal: one name everywhere, changed once.*
- Change the module, binary, config folder (reading the old `.cromwell/` for
  one release), environment variables and UI copy, in one deliberate commit.

*Done when:* `subutai init` and `subutai serve` work, and the suite is green.

**M5 — See the work** · M · no dependencies (can start now)
*Goal: answer "how is it going?" and "what exactly happened?".*
- Store every dispatch's full transcript and add a viewer.
- Add a per-feature timeline of major moments, linking down to transcripts.
- Add review-health numbers per reviewer (C-7).
- Split into M5a (transcripts), M5b (timeline) and M5c (metrics) if time is
  short.

*Done when:* for any feature, you can see its journey in one line of major
moments, and click through to what each agent was told, did and concluded.

**M6 — Documents with identity** · M · needs M2
*Goal: every document has a system ID, and creating work creates its
documents.*
- Mint IDs from database sequences.
- Write the ID into front matter, and track moves by ID rather than path.
- Creating an initiative or feature creates its starter document from the
  template.
- Add an "adopt existing document" action.
- Make per-initiative folders the default for new documents only.

*Done when:* a new feature arrives with its spec file already in place and
attached, and moving that file on disk doesn't detach it.

**M7 — Edit in the browser** · S · needs M6
*Goal: fix and polish documents without leaving the UI.*
- Build a Markdown editor with preview, *Save* and *Save & commit*.
- Refuse to overwrite a file that changed on disk, and show the difference.
- Warn before editing a document that is under review or feeding work in
  progress.

*Done when:* a document edited in the browser and one edited in vim can't
silently overwrite each other.

**M8 — Chat as a proper seat at the table** · M · needs M3
*Goal: the chat AI can prepare work, and humans can rule on any document.*
- Add MCP tools to submit documents and to request an early review.
- Allow a human verdict on any document type.
- Add the Submit screen: each intake step with its role, model and "already
  done" status, and per-run model overrides.
- Write a chat-side skill for writing specs, built from the retired
  `write-spec`.

*Done when:* a feature whose spec and plan were prepared in chat flows straight
through to gate 2, with every artifact showing who produced it.

**Phase B finish line: "Subutai usable".** From here the product team can plan
in chat, hand over a spec with one button, and watch the work through to done.

### Phase C: complete the model

**M9 — Executors** · M · needs M2, M8
*Goal: chat and humans can do any stage's work through the front door.*
- Record an executor on every task.
- Add MCP `claim_task` and `submit_task`, and a chat-side `work-a-task` skill.
- Extend the heartbeat to stale claims, and flag commits to an active feature
  made without a claim.
- Make verification never chat-executable.
- Mark chat token use as unmeasured.

*Done when:* a task done in chat gets the same independent code review and
verification as a dispatched one, and the UI shows who did it.

**M10 — Decisions** · S · needs M6
*Goal: the project remembers why, and agents are told.*
- Add a `decision` document type with `DEC-nnn` IDs and a viewer.
- Push approved decisions on a feature's branch of the tree into dispatch
  prompts, with a hard size cap. This is the same mechanism as project
  conventions (C-6), so build both together.

*Done when:* a dispatched agent's transcript shows the relevant decisions in
its prompt.

**M11 — Bugs** · M · needs M6
*Goal: anyone can report a bug, and humans decide which ones get fixed.*
- Add a bug entity with a report template and a human triage step.
- Give reviewers and verifiers a `report_bug` tool, and turn minor review
  findings into bug reports.
- Add a UI form and an MCP route.
- Accepted bugs travel the normal pipeline.

*Done when:* an agent-filed bug is triaged by a human, fixed by the pipeline
and verified as no longer reproducing.

**M12 — Spikes, checklists and jobs** · M · needs M2, M6
*Goal: finish the vocabulary.*
- Spikes: a token budget, a findings document, a worktree that is thrown away,
  and no merge path.
- Checklists of human jobs that count towards milestones.

*Done when:* a spike runs out its budget and asks a human, and a milestone
won't complete while a checklist has an unticked job.

### Phase D: real-world adoption

**M13 — GitHub and the Tickly pilot** · L, split into three · needs M6, M11
*Goal: Subutai runs a real project alongside GitHub, without the developers
noticing.*
- **M13a:** an importer with a per-project label map and a dry run, and
  adopt-in-place registration of Tickly's existing documents.
- **M13b:** the forge projection. Each task gets a GitHub issue, merged pull
  requests are observed, developer-filed bugs arrive for triage, and
  verification runs after merge.
- **M13c:** the pilot. One real Tickly feature goes from idea to merged code
  through Subutai, with a walkthrough written up.

*Done when:* the pilot walkthrough is recorded. **This is the finish line for
Subutai v1.**

### At a glance

```
Phase A  M0 Restart ─┬─ M1 Seam decision ── M2 Design approved
                     │
Phase B              ├─ M5 See the work (can start any time)
                     ├─ M3 Move the seam ── M8 Chat seat
                     ├─ M4 Rename
                     └─ M6 Documents ── M7 Editor
                                          ── "Subutai usable"
Phase C  M9 Executors · M10 Decisions · M11 Bugs · M12 Spikes & checklists
Phase D  M13a Import ── M13b GitHub projection ── M13c Tickly pilot = v1
```

Adding up the sizes gives roughly **24 to 26 sprints**, if M13's three parts
take three to five sprints between them. At one sprint a fortnight that's about
a year. At one a week, about six months. The "Subutai usable" point at the end
of phase B is about 14 sprints in. Phase A is four sprints, and most of that is
Sam's reading time.

### The spare-hours track

These are small, independent jobs that need no decision. Pick one up whenever
an hour or two frees up, but never at the expense of the current milestone.

- Give the implementer a search tool (C-6a).
- Fix the dispatch-prompt ordering so the prompt cache works
  ([prefix-cache note](../research/prefix-cache-discipline.md)).
- Replace the dollar runaway cap with a per-task token cap and a round limit.
- Forecast from past throughput instead of per-task guesses (token-estimation
  R2). The data already exists.
- Add vocabulary and anti-patterns to the surviving roles (C-3/C-4).
- Add effort expectations to dispatch prompts (C-8).

## 9. How to keep it moving

The project moved fast when Sam's attention was on it: fifty commits in twelve
days. It stopped completely when that attention went elsewhere. The build side
is fast, and the scarce resource is Sam's decisions and reviews. So the plan
protects those.

1. **One milestone at a time.** Finish it, write the handoff note, then choose
   the next one. Only the spare-hours track runs alongside.
2. **Keep a queue of decisions for Sam** at the top of this document (§10), each
   written so it can be answered in a few minutes, with a recommended answer.
   Most of Subutai's open questions already have one.
3. **A small, fixed weekly slot.** Thirty minutes to clear the decision queue
   and approve whatever is waiting keeps the build side unblocked, even in a
   busy week.
4. **Every build session ends with a handoff note**, as the project already
   does. That's what made this report possible after eight weeks away.
5. **Protect the cut line.** New ideas go into "after v1" unless they block the
   pilot.
6. **Keep this document current.** Tick milestones off here as they finish, so
   the next person to return finds an up-to-date picture.

## 10. Decisions waiting on Sam

In the order they unblock work. Each has a recommendation from the existing
documents.

| # | Decision | Recommendation | Unblocks |
|---|---|---|---|
| 1 | DEC-006: orchestration starts at *Submit to development*, specs are written in chat, and spec review is the first step of intake | Accept, as written up in the discussion response §2a to §2b | M3, M4, M8 |
| 2 | May the chat agent relay a human's design approval? | Yes, narrowly, once design approval no longer triggers anything, and audited as "via chat" | M3, M8 |
| 3 | Waive SPEC-009 DoD 3 (the cascade smoke)? | Yes, it tests a flow Subutai removes | M0 |
| 4 | ID and folder scheme | `INIT-`/`FEAT-`/`BUG-`/`DEC-`/`SPK-` IDs, identity in front matter, per-initiative folders as the default for new documents only | M6 |
| 5 | DEC-007: the judgement boundary (executors) | Accept, as written up in the discussion response §2c | M9 |
| 6 | Editor default: *Save* or *Save & commit* | *Save & commit* as the main button, *Save* as the secondary one | M7 |
| 7 | Bug triage surface | A dedicated triage queue, with a count in the inbox | M11 |
| 8 | Spike budget: checkpoint or hard stop; default or per spike | Hard stop with a project-wide default, overridable per spike | M12 |
| 9 | Stronger reviewer models for chat-executed work? | Yes, as a configured default, not built into the architecture | M9 |
| 10 | Per-user identity: in v1 or after? | Before the Tickly pilot if a second person will use it | M13 |

## 11. Sources

- [Subutai discussion document](../vision/Cromwell%20R2%20Vision%20Subutai.md)
- [DESIGN-010: Subutai](../design/DESIGN-010-subutai.md) (Draft 1)
- [Response to the Subutai discussion](subutai-discussion-response-2026-07-31.md)
- [Subutai and GitHub](../research/subutai-and-github.md)
- [Handoff: SPEC-009 Stage 1 code-complete](handoff-2026-07-30-stage1-complete.md)
- [Walkthrough: SPEC-009 Stage 1 live](../walkthrough-spec-009-stage1.md)
- [Research conformance audit](research-conformance-audit-2026-07-29.md)
- [DEC-005: the orchestration boundary](../decisions/DEC-005-the-orchestration-boundary.md)
- [Token estimation research](../research/token-estimation.md)
- The code itself at `2ce24b3`. Build, vet and short tests were run on
  2026-09-28.
