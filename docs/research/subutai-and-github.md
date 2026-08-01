# Subutai and GitHub: dividing the work

Tickly is managed through GitHub today, and it doesn't fit. Design documents travel as pull
requests, approvals are label moves, and the product team works in a tool built for people
who write code. This report works out how Subutai and GitHub should divide a project between
them, so the product team can leave GitHub without the developers noticing.

The short answer: **partition, don't synchronise.** Every fact gets exactly one owner. Subutai
owns intent — designs, specs, approvals, lifecycle, breakdown, priority. GitHub and git own
code — branches, pull requests, continuous integration, merges. The two are joined by a single
identifier, not by a sync. Nothing is duplicated because nothing is co-owned.

The good news is larger than expected. **Tickly has already built a Subutai-shaped workflow on
top of GitHub, written its state machine down, and specified the tool that should replace it.**
`docs/workflow-state.md` describes a label state machine and then says it should become "a thin
command-line tool (`wf`) that performs these moves and refuses illegal ones — the same contract,
executed by code instead of by feel." That tool is Subutai. The migration is mostly recognition,
not invention.

---

## Table of contents

1. [What Tickly already is](#1-what-tickly-already-is)
2. [Where the friction actually comes from](#2-where-the-friction-actually-comes-from)
3. [The principle: partition, not synchronise](#3-the-principle-partition-not-synchronise)
4. [The division of ownership](#4-the-division-of-ownership)
5. [What each side's day looks like](#5-what-each-sides-day-looks-like)
6. [Bugs, the one case that crosses](#6-bugs-the-one-case-that-crosses)
7. [Documents: adopt in place, don't migrate](#7-documents-adopt-in-place-dont-migrate)
8. [Verification without getting in the way](#8-verification-without-getting-in-the-way)
9. [Upgrading Tickly specifically](#9-upgrading-tickly-specifically)
10. [What this asks of Subutai's design](#10-what-this-asks-of-subutais-design)
11. [Open questions](#11-open-questions)
12. [Sources](#12-sources)

---

## 1. What Tickly already is

Read the current setup as a prototype of Subutai rather than as an obstacle to it. The
vocabulary lines up almost exactly.

| Tickly label | Subutai entity | Match |
|---|---|---|
| `epic` — a body of work, owns one design doc | **Initiative** — holds documents, no lifecycle | Near-exact |
| `feature` — one capability, owns one spec | **Feature** — the commitment point, has the lifecycle | Exact |
| `task` — one branch, one pull request | **Task** — one unit of implementation | Exact |
| `bug` — a corrective task | **Bug** — feature-shaped, with triage | Exact |
| `doc` — documentation work | No direct equivalent | Becomes a Task or a Job |
| `spike` — throwaway, never merges | No equivalent | See §11 |

The stage labels map just as cleanly onto the feature lifecycle, and both run at the Feature
level:

| Tickly stage | Subutai |
|---|---|
| `stage:design` | Design document being written |
| `stage:spec` | Spec being written in the planning conversation |
| `stage:plan` | Intake: planning, plan review, estimation |
| `stage:implement` | Implementing |
| `stage:review` | Reviewing, then verification |
| *(issue closed)* | Done |

Two structural decisions Tickly has already made are worth calling out, because they are the
ones that make the split possible.

**Approval is already separated from merging.** `docs/workflow.md` says design documents land
through a pull request but "**the merge is not the approval** — approval is Sam moving the Epic
to `stage:spec`, and no status line ever lives in the file." That is precisely Subutai's model:
the file is content, the verdict is state, and they live in different places. Tickly implements
it awkwardly, on labels, but the conceptual work is done.

**Content and state are already distinguished.** `workflow-state.md` ends with a section headed
"What is content, not state (never encoded here)", which assigns design documents, specs, prose,
and requirements to disk and reserves the label table for "the mechanical state a tool could one
day own." That sentence is the seam this whole report is about, and Tickly drew it first.

GitHub's native sub-issue graph is in use too: Epic #63 (Payments) has five Feature children,
#64 through #68. The tree exists; it is just held in a system that shows it poorly.

## 2. Where the friction actually comes from

The friction is not vagueness about process. Tickly's process is unusually well documented. It
comes from running prose through machinery built for code, and the cost is measurable.

**Documentation dominates the pull-request traffic.** Of the fifteen most recently merged pull
requests, seven were documentation-only — specs, design documents, decision records, planning
corpora. Each carried the full ceremony of a code change.

**The ceremony is expensive for prose.** Issue #77 records the cause: `main` requires the
`verify` check, and `verify` takes about five minutes because it starts the whole database stack
— "even when the change is three paragraphs of English." Decision 0011 assumed that check took
about a minute, which is why its "land it early and iterate in place" rule "has never actually
worked in practice." Design documents park on a branch for days because every revision costs a
full round trip.

**The tracker presents badly.** Issue #77 also records the diagnosis that matters most here:
GitHub is "a perfectly good place to *store* state and a poor place to *see* it." That framing
is right, and it is the reason the answer is not "move everything" — it is "move the seeing, and
the deciding, and leave the storing of code where it is."

**The remedies already proposed are Subutai in miniature.** Decision 0012, currently open as
PR #78, proposes a fast lane for documents, a generated `STATUS.md` whose first section is
everything waiting on Sam, and a rule that anything a human reads is written in sentences. A
status page generated from issues is a dashboard; a queue of things waiting on Sam is an inbox.
Subutai has both, properly. This has a practical consequence for right now, in §9.

## 3. The principle: partition, not synchronise

The question "can we do it without duplication?" contains an assumption worth dismantling. It
suggests the choice is between one store and two. The real choice is **one owner per fact.**

Three architectures are available, and only the third works.

**Move everything to Subutai.** The product team's problem disappears and the developers' begins.
Pull requests would have nothing to close, branch names would lose the issue number they are
built on, and code review would leave the place where the diff lives. This violates the harder
of the two constraints — don't get in the developers' way — and it would be quietly ignored.

**Synchronise the two.** Bidirectional sync between trackers is a well-known way to produce two
sources of truth, a conflict-resolution problem nobody wants, and drift that surfaces as
mysterious contradictions. Every fact would have two owners, which is the definition of the
problem, not a solution to it.

**Partition, and project.** Each fact has exactly one authoritative home. Subutai projects a
minimal issue into GitHub for each Task, so the developer has something to branch from and
close. GitHub emits events — a pull request merged, a check passed — which Subutai observes.
Neither writes the other's authority.

The projected issue is a **report, not a replica.** Subutai regenerates its body and never reads
it back for state, so it cannot drift into disagreement. If someone edits the body, the edit is
overwritten; comments are untouched and Subutai ignores them. This is the same relationship a
printed invoice has to the accounting system: the paper can be wrong without the books being
wrong.

So nothing is duplicated, because nothing is shared. The only datum crossing the boundary is an
identifier, and it already exists — Tickly's branch convention is `<type>/<issue>-<slug>`, so
the issue number is already the join key between intent and code.

## 4. The division of ownership

| Fact | Owner | Why |
|---|---|---|
| What we're building, and why (design) | Subutai | Prose for humans; needs revision without ceremony |
| What exactly will be built (spec) | Subutai | The handover artifact; written in the planning conversation |
| Approvals, verdicts, reviews of intent | Subutai | The product team's act, and Sam wants it out of GitHub |
| Lifecycle stage | Subutai | The state machine `workflow-state.md` already specifies |
| Task breakdown | Subutai | Produced by the planner during intake |
| Priority, milestones, roadmap | Subutai | The product team's view of the work |
| Bug reports and triage | Subutai | A product decision, not a code one |
| Branch, commits, diff | git | Where code lives |
| Pull request and code review comments | GitHub | Where developers already review code |
| Continuous-integration status | GitHub | Already wired, already trusted |
| Merged or not merged | GitHub | The one unambiguous signal that code landed |

Two rules keep the partition honest.

**Subutai never asks a developer to record state.** Every label move, stage transition, and
status update leaves the developer's workflow. This is the change that makes the developer's day
shorter rather than longer, and it is what buys the goodwill for everything else.

**GitHub never holds an approval.** A projected issue may *display* that a spec was approved and
link to it, but the verdict lives in Subutai and the issue is only reporting it. No product-team
act happens through a pull request.

## 5. What each side's day looks like

### The developer

Today, per `docs/workflow.md` and the pull-request template: read the issue, decode its labels,
find the spec in `docs/specs/`, branch as `feat/42-slug`, work, run `make verify`, open a pull
request against the template, attach screenshots, get review, merge, then update the stage
labels and the issue notes.

After: read the issue — now written in prose, with the spec linked inline and the acceptance
criteria already in the body — branch as `feat/42-slug`, work, run `make verify`, open the pull
request, get review, merge.

**The bookkeeping step disappears and nothing is added.** `make verify`, the pull-request
template, the compatibility checklist, the visibility-matrix rule, screenshots against the
mockups — every engineering gate developers already respect stays exactly where it is, owned by
them. The last line of `workflow.md`'s prohibitions, "let issue state drift: finish work, update
the issue, same sitting", stops being a rule they can break, because it stops being their job.

This is the argument to make to a developer: it is strictly less process, not more.

### The product team

Today: labels, sub-issue trees, a Projects board, design documents as pull requests, approvals
as label moves, and a five-minute check standing between a paragraph edit and `main`.

After: the conversation happens in chat; documents are written and revised in chat or the
Subutai editor and land in git without a pull request; approval is a button on the document page
or a sentence to the chat agent; the dashboard shows the tree, the timeline, and the inbox of
things waiting on a human. GitHub is not opened.

## 6. Bugs, the one case that crosses

Bugs are the interesting case because they originate on both sides, and Sam asked specifically
whether managing them in Subutai means also managing them in GitHub.

**The report and the triage never touch GitHub.** A bug is reported through chat or a form,
becomes a Bug entity with a report document, and a human accepts or rejects it. That is a
product decision, and DESIGN-010 §8 already has it right.

**Only the fix is projected.** Once a bug is accepted and broken into tasks, each Task projects
an issue so the developer has something to branch from and close, exactly as for a feature. The
developer sees what is broken, how to reproduce it, and the acceptance criterion. They never see
the triage.

**One narrow inbound lane is needed**, because developers find bugs too, and they will file them
where they are. A GitHub issue labelled `bug` that Subutai did not create should be pulled in as
an untriaged Bug report, appearing in the triage queue like any other. This is the only inbound
flow in the design, it is low-volume, and it fails safe: the worst case is a bug report that
needs triaging.

So the answer to "would bugs need to be managed in GitHub as well?" is no. They are reported and
decided in Subutai, and only their *fixes* appear in GitHub, as ordinary task issues. A
developer-filed issue is an inbox item, not a parallel system.

## 7. Documents: adopt in place, don't migrate

This is where the largest change to DESIGN-010 is needed, and it only becomes visible when you
try the upgrade on a real project.

**The documents do not need migrating at all.** Tickly's designs, specs, decisions, and glossary
are already Markdown in the repository — 93 tracked Markdown files, 8,304 lines. Subutai's
document model is Markdown in the repository. The upgrade is *registration*: assign each file an
ID, a type, a lifecycle state, and an owning entity. Nothing moves and nothing is copied.

**But DESIGN-010 §6 as drafted would move all of them.** It proposes
`docs/work/INIT-014-auth/FEAT-023-spec.md` — identifier-prefixed filenames in per-initiative
folders. Tickly's documents live at `docs/design/<slug>.md` and `docs/specs/<slug>.md`, and they
cross-reference each other heavily. Applying the scheme would rewrite every relative link,
detach every `git blame`, and break every URL anyone has ever pasted into a conversation. For a
greenfield project the scheme is tidy; for a brownfield one it is hostile, and Tickly is the
project Subutai exists to manage.

The fix is already half-written in §6: "The ID is also written inside each file's frontmatter,
so if you rename or move a file, Subutai recognises it by its ID and nothing breaks." Promote
that from a convenience to the rule.

> **Identity lives in frontmatter. Location is free.** The folder-and-prefix scheme becomes the
> default for documents Subutai creates, not a requirement for documents it manages.

That single change turns the upgrade from a repository-wide rewrite into an additive pass that
touches only frontmatter.

**Landing documents without a pull request** is the other half of the problem, and it is what
removes the five-minute tax. Subutai's `Save & commit` should commit and push a document change
directly. `main` is protected, so in practice this means one of:

- **A path-filtered fast lane** — `verify` skipped for prose-only changes, plus auto-merge. This
  is decision 0012's first item, it is a small change to `.github/workflows/ci.yml`, and it is
  invisible to developers. Recommended, and worth doing now regardless of Subutai.
- **A documents branch** that Subutai owns and merges on its own schedule. More moving parts, no
  clear benefit over the fast lane.

Either way the product team never sees a pull request. The mechanism is Subutai's business.

## 8. Verification without getting in the way

DESIGN-010 is emphatic that the definition-of-done check "always runs, performed by the
dispatched verifier" and "cannot be waived by any actor, human or AI." Read naively against a
GitHub project, that rule is exactly the kind of arbitrary insertion that developers ignore: it
would mean an agent gating every pull request a human wrote.

The resolution is to be precise about *what* is verified, and it is already implicit in the
design.

**Verification is a Feature-level act, not a Task-level one.** DESIGN-010 §5 says the verifier
"checks the whole feature against the spec's acceptance criteria" when every task is done. So:

- Developers merge task pull requests exactly as they do now. Their gates are `make verify`,
  continuous integration, screenshots, and human code review. Subutai blocks none of it.
- When the last task in a Feature is merged, Subutai runs the verifier against the Feature's
  acceptance criteria, on already-merged code.
- A failure produces a checkpoint for Sam or a Bug — not a blocked developer.

This preserves the rule that verification is never skipped while ensuring it never sits in a
developer's path. It runs after the code has landed, which is also when the acceptance criteria
can honestly be checked.

The same reasoning applies to code review. Subutai's code reviewer is for agent-implemented
tasks. When a human implements a task and a colleague reviews the pull request, that *is* the
independent review, and DESIGN-010 §5b's executor model already accommodates it: the executor is
recorded as human, and the judgement rule — nobody judges their own work — is satisfied by the
colleague. Subutai should record the reviewing human from the pull request rather than dispatch a
second opinion nobody asked for.

## 9. Upgrading Tickly specifically

### What to import, and what to leave

| Data | Volume | Action |
|---|---|---|
| Open issues | 21 | **Import.** Map by label to entity type and lifecycle state |
| Sub-issue tree | Epic #63 → 5 features | **Import.** Becomes the initiative-feature parentage |
| Closed issues | 23 | **Import as archived**, with their GitHub link. No documents, no lifecycle |
| Pull-request conversations | All | **Leave.** Code review belongs to GitHub |
| Design docs, specs, decisions | 93 files | **Adopt in place** (§7). Register, don't move |
| Labels | 21 | **Retire the `stage:*` group.** Keep type labels for the projection |
| Projects board | 1 | **Abandon.** Subutai's dashboard replaces it |

Importing closed issues as archived entities is cheap and preserves the trail from a merged pull
request back to the intent behind it. Leaving them out would break that trail for twenty-three
pieces of shipped work.

### The label map must be configurable

Tickly's labels are Tickly's. Another project will use `type: bug` or `kind/feature` or nothing
at all. The importer needs a declarative mapping — label to entity type, label to lifecycle
state — supplied per project, with a dry run that reports what it would create before it creates
anything. This is the difference between an importer that works once and one that works for
Sam's other repositories.

### One thing worth deciding now, before Subutai exists

Decision 0012 is open as PR #78 and proposes three things: a fast lane for documents, a generated
`STATUS.md`, and readable prose in anything a human reads. If Subutai is coming, **only the first
is still worth building.**

- **The fast lane: build it.** It fixes a live, measured, five-minute problem, it helps
  immediately, and Subutai wants it afterwards anyway (§7).
- **`STATUS.md`: don't build it.** It is a dashboard generated from issues, which is what
  Subutai's UI is. Building it means writing a poor version of the thing that replaces it.
- **Readable prose in issues: partly moot.** Once Subutai writes the projected issue bodies, they
  are generated to a house style. The rule is still worth stating for the humans and the chat
  agents, but it does not need enforcing machinery.

This is worth raising before #78 merges.

## 10. What this asks of Subutai's design

Seven changes, in rough order of how load-bearing they are.

1. **A forge projection layer.** Outbound: Task to issue, created and maintained by Subutai.
   Inbound: pull request merged, and check status, as observed events. Configurable per project,
   optional, and degrading to absent — a Subutai project with no forge configured must work
   exactly as it does today.

2. **Documents adopt in place.** Frontmatter identifier becomes the identity; path becomes free.
   The §6 folder scheme demotes to a default for new documents. Needs an "adopt existing
   document" flow that assigns an ID and lifecycle to a file already on disk.

3. **Verification at the Feature level, after merge.** Stated explicitly in the design, so nobody
   implements a per-pull-request gate. Human code review on a pull request satisfies the
   independent-review requirement for human-executed tasks.

4. **An importer**, with a per-project label map and a dry run.

5. **A bug inbound lane** — developer-filed GitHub issues arrive as untriaged Bug reports.
   Extends §8.

6. **Attribution from the forge.** When a human implements and a colleague reviews, Subutai learns
   both from the pull request and records them as executor and reviewer. This is §5b's executor
   model reaching across the boundary, and it is what keeps the ledger honest when work happens
   in GitHub.

7. **Milestones and roadmap as the product team's home.** Subutai already has both. They replace
   the Projects board rather than mirroring it, and no projection is needed — milestones are
   product-team facts with no developer-side counterpart.

Two things this deliberately does *not* ask for: no bidirectional sync, and no GitHub-side
approval surface. Both are ruled out by §3.

## 11. Open questions

- ~~Spikes have no Subutai entity.~~ **Settled 2026-08-01: DESIGN-010 §9 adds Spike as a work
  entity** — a question with a token budget, producing a findings document, with no merge path
  in the entity at all, so "throwaway code never ships" becomes a property of the code rather
  than a rule in prose. Tickly's `spike` label maps to it directly. What remains open is
  whether a spike's projected issue is worth creating at all, given that its output is a
  document rather than a branch anyone reviews.

- **Who creates the projected issue, and when.** At task creation, or when a task is claimed?
  Creating early gives developers visibility of what's coming; creating late keeps the issue list
  short. Probably a per-project setting.

- **What happens to a projected issue when its task is abandoned.** Close it, or delete it? Closing
  leaves a confusing artifact; deleting loses the trail.

- **Whether Subutai should read pull-request review comments.** They contain real findings that
  might belong in the record. Reading them means parsing prose written for humans, which is the
  kind of integration that seems cheap and is not. Recommend not, initially.

- **Multi-repository projects.** Tickly is one repository, so this does not bite yet. DESIGN-010
  §10 says one installation manages one project; it does not say a project is one repository.

## 12. Sources

### Tickly, examined directly 2026-08-01

- `docs/workflow.md` — the five stages, the hard rules, backflow, the two loop speeds, and the
  design-document landing policy where merge is explicitly not approval.
- `docs/workflow-state.md` — the label state machine, the legal transitions and their gates, the
  content-versus-state split, and the anticipated `wf` tool.
- `.github/PULL_REQUEST_TEMPLATE.md` — the engineering gates developers already own.
- GitHub issues and pull requests via `gh`: 44 issues (21 open, 23 closed), 21 labels, the
  Epic #63 sub-issue tree, and the last fifteen merged pull requests.
- Issue #77 and PR #78 (decision 0012) — the friction, measured and already diagnosed, including
  the five-minute `verify` and the "good place to store state, poor place to see it" framing.

### Cromwell and Subutai

- `docs/design/DESIGN-010-subutai.md` — §2 the two worlds, §4 the vocabulary, §5 the workflow and
  its two gates, §5b executors, §6 documents, §8 bugs, §10 the deliberate omissions.
- [codebase-memory-mcp-review.md](codebase-memory-mcp-review.md) §5.1 — the Tickly profile:
  15,250 lines of source, 8,304 lines of Markdown, eight workspaces.
