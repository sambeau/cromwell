# DESIGN-010: Subutai

**Status:** Draft 1, for Sam to read and revise. Not approved.
**Date:** 2026-07-31
**Author:** Claude (Fable 5), from the R2 discussion with Sam
**Sources:** the [Subutai discussion document](../vision/Cromwell%20R2%20Vision%20Subutai.md),
the [discussion response](../notes/subutai-discussion-response-2026-07-31.md),
and [vision-v1](../vision/vision-v1.md), which this document revises but does
not replace.

---

## 1. What Subutai is

Subutai is a planning, workflow, and orchestration system for small teams
building large software projects with AI agents. It takes work from the first
rough idea all the way to shipped, verified software, and it keeps an honest
record of everything that happened along the way.

Subutai is the new name for Cromwell, and it is a revision, not a rewrite —
think of it as Cromwell, second edition. The engine underneath is unchanged.
What this document does is clarify the workflow, settle the vocabulary, and
describe the surfaces (chat and web) that people actually touch.

## 2. The big picture: two worlds, one handover

Building software has two kinds of work in it, and they behave completely
differently.

**Planning is creative work.** Deciding what to build is a conversation — it
loops, it backtracks, it changes its mind over coffee. You cannot put it on
rails, and you shouldn't try. In Subutai, planning happens as a conversation
between humans and a highly capable chat AI, at whatever pace the thinking
needs.

**Development is disciplined work.** Once you know what to build, building it
has a strict shape: plan it, build it, review it, test it, check it against
the definition of done. Every step has rules, and skipping steps is how
quality dies. In Subutai, development is run by the **orchestrator** — and
this is the most important design decision in the system, so it gets its own
paragraph.

**The orchestrator is software, not an AI.** It is plain, deterministic code
that watches for events, checks the rules, and hands work to AI agents at the
right moments. It has no conversation history to lose track of, no attention
span to exhaust, and it cannot be talked out of a rule. We learned this
lesson the hard way: our previous system, kanbanzai, used an AI agent as the
orchestrator, and under a long enough workload it would drift — doing the
work itself instead of delegating, or delegating and then forgetting to
follow up, leaving the records claiming things were done that weren't. Code
doesn't drift. Every safety rule in Subutai is enforced by code, not
promised by an AI.

**The handover between the two worlds is the specification.** The planning
conversation ends by producing a spec: a precise description of what will be
built, written for the agents who will build it. Handing that spec to
development is the moment casual becomes contractual. Everything before the
spec is flexible; everything after it is governed.

### Why the split lands where it does

Two reasons, one principled and one happily practical.

The principled reason: research and our own experience agree that
deterministic work should be managed by deterministic code, and open-ended
work shouldn't be. The spec is where work stops being open-ended.

The practical reason: the split also matches how AI is paid for. The most
capable models — the ones you want doing your thinking — are affordable
through subscription chat plans and painfully expensive through the API. The
workhorse models that implement and review code are cheap through the API.
So planning naturally runs on subscription chat with the best minds, and
development runs on API agents chosen per role. This is a pleasant economic
alignment, not a law of the system: which model serves which role is
configuration, and it will change as pricing does.

## 3. The team

A Subutai team mixes humans and AI, in two groups.

**The product team** shapes what gets built:

- **Product manager** — human.
- **Designer** — human and chat AI working together.
- **Project manager** — a chat AI, guided by a human. This is the agent that
  talks to Subutai on the humans' behalf: creating initiatives, drafting
  documents, checking status, and preparing work for handover.

**The development team** builds it:

- **The orchestrator** — deterministic software. Runs the show.
- **Spec reviewer** — the border guard. The first agent to see an incoming
  spec, checking that it is clear, complete, and faithful to the approved
  design. Can send it back.
- **Planner** — reads the spec and writes the development plan: how the work
  breaks into tasks.
- **Plan reviewer** — checks the development plan the same way the spec
  reviewer checks the spec.
- **Estimator** — reads the plan and estimates the work, in tokens.
- **Implementers** — write the code, one task at a time, each in its own
  isolated working copy.
- **Code reviewers** — inspect each task's changes against the spec and plan.
- **Verifier** — the last agent in the chain. Checks the finished feature
  against its acceptance criteria and must show evidence for every one. This
  step can never be skipped, by anyone.

Every orchestrated role has its own configured model, its own instructions,
and its own strictly limited set of tools — a reviewer physically has no
tool that can edit a file. Which model backs which role is yours to tune,
per project and even per run.

## 4. The vocabulary

Subutai keeps a deliberately small set of concepts, in two groups: things
that *define* work, and things that *track* it.

### Work entities — where work is defined

| Term | What it is |
|---|---|
| **Project** | The top of the tree. Conceptually just the topmost initiative. |
| **Initiative** | A container for a piece of intent — a plan to do something. Initiatives nest. They hold documents and dashboards, but deliberately have no lifecycle: above the feature line, things simply *are*, until they're archived. |
| **Feature** | A buildable unit with a specification. This is the commitment point: creating a feature says "we intend to build this." Features have the full lifecycle, tasks, and a worktree. |
| **Task** | One unit of implementation work inside a feature. What an implementer picks up. |
| **Bug** | A reported problem. Feature-shaped, with a triage step at the front. (Section 8.) |
| **Spike** | A question with a budget. Throwaway investigation that produces findings, never shipped code. (Section 9.) |
| **Job** | A single task for a human — sign up for the account, choose the icon, find the API key. Ticked off by hand. |
| **Checklist** | A bundle of jobs. Complete when every box is ticked. |

### Tracking entities — where work is measured

| Term | What it is |
|---|---|
| **Deliverable** | Anything a milestone can contain: an initiative, a feature, a checklist, or another milestone. |
| **Milestone** | An *unordered* collection of deliverables that ship together. Live while open; frozen when locked, so the record of what shipped is honest. |
| **Roadmap** | An *ordered* list of milestones. The order means whatever the planner wants it to mean — the system just preserves it. |

### Documents

Every piece of writing in the system — designs, specs, development plans,
reviews, bug reports, decisions, notes — is a **document**: a Markdown file
in the project's git repository, tracked by Subutai with a type, an ID, and a
lifecycle (`draft → reviewing → approved → superseded`). Section 6 covers how
they're named and where they live.

Two document types matter most, and the difference between them is the
difference between the two worlds:

- A **design** says what we want and why, written by humans (with AI help)
  for humans. Any initiative or feature can have one.
- A **spec** says exactly what will be built, written for the agents who will
  build it. Only features have specs, because only features get built.

## 5. The workflow, end to end

Here is the whole journey, from idea to done. There are exactly **two
moments where the system stops and waits for a human decision**. Everything
else either flows on its own or interrupts a human only when something has
actually gone wrong.

### Planning: the conversation

Work begins as an initiative and a conversation. The humans and the chat AI
talk through what's wanted, write a design document together, revise it,
sleep on it, revise it again. Sub-initiatives and features get sketched out
underneath as the shape becomes clear.

When the design feels right, a human approves it. Approving a design is a
record, not a trigger — nothing launches. You can approve it from the
document page in the web UI, or just tell the chat agent; either way it's
noted, and the conversation continues.

Then the chat AI writes the spec, translating the approved design into the
precise, testable language the development side needs. This is deliberate:
the spec is the most important document in the system — every downstream
step relies on it — so it gets written by the strongest model available, in
conversation, with the human right there. Not delegated to a cheaper agent.

### Gate one: submit to development

When the spec is ready, a human presses **Submit to development**. This is
the handover — the first of the two gates, and the moment orchestration
begins.

The submit screen shows exactly what's about to happen: each intake step,
which agent role will run it, with which model, and whether any step is
already satisfied (more on that in Section 5a). The human can raise or lower
any model for this run before pressing go.

From the button, the intake runs unattended:

1. **Validation** — a mechanical structure check. Required sections, working
   links. Instant pass/fail.
2. **Spec review** — the border guard reads the spec cold, checking that it's
   clear, testable, and a faithful, complete translation of the design. This
   reviewer exists because the spec's authors — the human and the chat AI
   together — share the same blind spots; a fresh pair of eyes that wasn't in
   the room is the cheapest insurance in the system. If the review requests
   changes, the spec goes back to the planning conversation with comments.
3. **Development planning** — the planner writes the development plan,
   breaking the feature into tasks.
4. **Plan review** — a second reviewer checks the plan.
5. **Estimation** — the estimator sizes the work, in tokens.

Then the system stops and waits.

### Gate two: start implementation

A human looks at the reviewed plan and the estimate, and presses **Start**.
This is the second gate — the last routine human act until the feature is
done.

From here the loop runs: implementers pick up tasks in an isolated working
copy, code reviewers inspect each task's changes, and revisions cycle until
the reviewer finds no more *major* problems. (Minor findings — style, naming,
tidiness — don't hold work hostage: they're recorded and dropped, and become
material for bug reports and retrospectives. A finding that touches
correctness, completeness, scope, or soundness is major, and majors always go
back for fixing.)

When every task is done, the **verifier** checks the whole feature against
the spec's acceptance criteria — and must cite specific evidence for each
one. "Looks fine" is not evidence; "the login test covers valid and invalid
credentials" is. Only then does the feature merge and count as done.

### Exceptions, not check-ins

Humans are not asked to babysit any of this. The system interrupts a human
only through **checkpoints** — queued questions that appear in the web UI
inbox when something genuinely needs judgement: a reviewer escalates a
disagreement, a dispatch keeps failing, a budget threshold trips, a merge
conflicts. Answer the checkpoint and the machine continues. If checkpoints
are rare, the system is healthy; if they're frequent, something upstream
needs fixing, and that's worth knowing too.

### 5a. Working ahead in chat

Sometimes a feature is small but tricky — exactly the kind of thing you'd
rather work through with the strongest model in conversation than push
through the standard pipeline. Subutai supports this directly, and the
mechanism is simple: **the intake steps check state, they don't insist on
running.**

Before pressing Submit, the planning conversation can:

- **Write the development plan itself.** A plan authored in chat and
  submitted simply means the planner agent has nothing to do.
- **Request reviews early.** The chat agent can ask the orchestrator to
  review the spec or the plan right now, before any button is pressed. The
  same independent reviewer runs; the verdict lands on the document.
- **Hand the verdict to a human.** When the human has been deep in the work
  and is confident, they can approve the spec or plan directly in the UI —
  a human verdict, recorded as such, standing in for the agent review.

When Submit is pressed, the submit screen shows which steps are already
satisfied, and the intake skips them. A well-prepared feature can flow
straight through to gate two.

### 5b. Who does the work: executors

We take this one step further, and it's worth explaining why.

Any stage's *work* — including implementation itself — can be done by a
dispatched agent, by the chat AI in conversation with a human, or by a human
alone. We call whoever does the work the **executor** of that task, and the
system records it. A chat AI or human working a task claims it first,
works in the feature's working copy, and submits it when done — the same
doorway the dispatched agents use, so the state always tells the truth.

The honest reason for this: if the system *didn't* allow it, people would do
it anyway. When a task is genuinely hard, the temptation to open a chat
window and say "just fix it" is real — and if that work happens outside the
system, it happens with no review, no record, and no verification. Better to
give the expensive-model-plus-human path a proper lane than to pretend a
prohibition would hold. With a lane, ad-hoc work becomes a normal, visible,
reviewed part of development instead of an invisible exception.

One rule makes all this flexibility safe, and it is the sharpest line in
Subutai:

> **Who does the work is flexible. Who judges the work is not.**

Anyone may *do* a stage. Nobody may judge their own work, and no path skips
a gate. The chat AI never gets a tool that can approve anything — a chat-
implemented task still gets an independent code review, a chat-written spec
still faces the border guard (or an explicit human verdict), and the final
verification against the definition of done is **always** performed by the
dispatched verifier, is never optional, and cannot be waived by any actor,
human or AI. Judging stays closed so that doing can be open.

## 6. Documents: names, homes, and editing

### Every document has an ID, minted by the system

Subutai assigns IDs centrally — `INIT-014`, `FEAT-023`, `BUG-007`,
`DEC-006` — so numbers never clash no matter who creates what, from chat or
the UI. Document filenames carry their entity's ID, so everything belonging
to a piece of work groups together at a glance:

```
docs/work/INIT-014-auth/
    INIT-014-design.md
    FEAT-023-spec.md
    FEAT-023-dev-plan.md
    BUG-031-report.md
```

Files are grouped in one folder per initiative — the arrangement humans
actually find things in — with the document's type in its name. The ID is
also written inside each file's frontmatter, so if you rename or move a
file, Subutai recognises it by its ID and nothing breaks.

### Creating work creates its documents

Creating an initiative or feature — in the UI or via chat — creates its
starter document from the template, in the right folder, already attached.
No more typing file paths into a text box; that remains only as the escape
hatch for attaching a file that already exists.

### Editing in the browser

The web UI includes a simple Markdown editor with preview, for the everyday
case of fixing and polishing documents without leaving the browser.

- **Save** writes the file to disk, exactly as if you'd edited it in your
  own editor. **Save & commit** also commits that file to git, with you as
  the author.
- If the file changed on disk since you opened it (someone edited it in
  vim), saving stops and shows you the difference rather than silently
  overwriting either version.
- The editor always shows the document's lifecycle state, and warns before
  you edit something that's currently under review or feeding in-flight
  work — editing is allowed, but the consequences (a spec revision while
  agents are mid-task triggers the revision machinery) deserve a heads-up.

Documents remain plain Markdown files in git throughout. The browser editor
is a convenience, never a requirement, and never a special format.

## 7. Watching the work

Subutai aims to answer two different questions well: *"how is it going?"*
at a glance, and *"what exactly happened?"* when you need to dig.

**The timeline** answers the first. Each feature shows its journey as major
moments — *Spec submitted · Spec approved · Planning · Implementing (3 of 7
tasks) · Verifying · Done* — clean enough to read in a glance, without the
noise of every internal event.

**Transcripts** answer the second. Every agent dispatch keeps its complete
conversation: the prompt the agent was given, every tool it used, everything
it said, and what it concluded. Click into any timeline moment and read
exactly what the agent did and why. This is the debugging view — when an
estimate was wildly off or a review feels wrong, the answer is in the
transcript.

**Sizing is in tokens.** Work is estimated and measured in tokens — the
actual unit of AI computation — because tokens are objective, additive, and
recorded automatically. Estimates carry a confidence rating based on how they
were made, and completed work feeds its actual consumption back in, so
estimates sharpen over time. (One caveat we accept openly: work done in chat
burns subscription tokens the ledger can't see, so chat-executed tasks are
marked as unmeasured rather than pretending they were free.)

**Review health is visible.** Approval rates, findings per review, and time
to verdict are surfaced per reviewer — a reviewer that approves everything
in seconds is a broken reviewer, and the numbers make it visible.

**Everything is attributed.** Every artifact and verdict records who
produced it — which agent role and model, the chat AI, or a human — so
questions like "do human-approved specs bounce more often at planning?" have
answers instead of vibes.

## 8. Bugs

Anyone — human or agent — can report a bug, at any time, without ceremony.
Agents file reports mid-work when they spot something out of scope (the
minor review findings that get recorded-and-dropped land here too). Humans
report through chat or a UI form. Every report becomes a bug entity with a
report document, attached to the initiative or feature it came from.

A bug is feature-shaped with one extra step at the front: **triage**. A
human looks at the report and accepts or rejects it — that's the human
involvement the workflow guarantees, and it's the same kind of act as
creating a feature: a commitment of scope. From acceptance onward, a bug
travels the normal pipeline; its report document serves as its spec (with a
built-in acceptance criterion: the defect no longer reproduces), and the fix
passes the same reviews and the same verification as any feature. No
parallel machinery, no second-class pipeline.

## 9. Spikes

Sometimes you can't write a spec because you don't yet know enough. Does that
API do what its documentation claims? Is this approach fast enough to bother
with? Will these two services actually talk to each other? The honest response
is to go and find out, and the work of finding out is a **spike**.

A spike is a question with a budget. It hangs off whatever initiative or
feature raised it, it carries a token cap agreed up front, and it produces
exactly one thing: a findings document. The code it writes along the way is
scaffolding — real enough to answer the question, never good enough to ship.

**A spike cannot merge, and that is enforced rather than promised.** It gets a
worktree like any other work, and when the spike concludes the worktree is
discarded. There is no merge path in the entity at all, so no agent can decide
that its throwaway code looks good enough to keep and no human can do it by
accident. This is the same principle as everywhere else in Subutai: a rule that
matters is a property of the code, not a line in a document that an agent may
or may not have read.

Because a spike has no spec, it has nothing to verify against, so the
definition-of-done machinery doesn't apply to it. What replaces it is simpler
and human: the spike is done when someone reads the findings and says the
question is answered. The findings document itself is an ordinary Subutai
document with the ordinary lifecycle, so if it's going to inform a design it
can be reviewed like anything else.

Anyone can run a spike — a dispatched agent, the chat AI, or a human at a
terminal — under the same executor rules as any other work (Section 5b). The
budget is what keeps it honest: spikes are the single easiest thing in software
to let run for a fortnight, and a cap that trips into a checkpoint is cheaper
than a conversation about where the week went.

**Promotion is deliberate and one-way.** A spike that starts looking like the
real implementation doesn't graduate into one. The findings inform a design or
a spec, a feature is created in the normal way, and the feature is built from
scratch by the normal pipeline. The scaffolding stays in the bin. This costs a
little rework and buys the guarantee that nothing reaches production without
having gone through the gates — which is the whole point of having them.

## 10. Decisions

Decisions are how the project remembers *why*. Each one is a document —
`DEC-006`, with its ID minted by the system — recording what was decided,
the reason, and what it replaced. An accepted decision is never edited; if
the project changes its mind, a new decision supersedes the old one, and
the trail stays honest.

Decisions attach to the project or to an initiative, and they do their real
work automatically: **approved decisions on the relevant branch of the tree
are pushed into agents' context when work is dispatched.** This matters
because of a lesson we paid for twice: agents will not go and look things
up. Any knowledge system that relies on agents fetching from it will be
written to and never read. So Subutai doesn't have a knowledge base — it has
documents, and it pushes the relevant ones into the prompt. For that to stay
affordable, the surfaced portion of a decision is kept short: the ruling and
its reason, not the essay.

A simple viewer in the UI lists decisions for humans; the IDs keep them easy
to cite in conversation, reviews, and commit messages.

## 11. What Subutai deliberately doesn't do

- **No AI orchestrator.** The orchestrator is code, permanently. This is the
  founding lesson.
- **No self-approval, ever.** No actor judges its own work, and the chat AI
  has no tool that can approve, override, or verify anything.
- **No skippable verification.** The definition-of-done check always runs,
  performed by the dispatched verifier, whoever did the work.
- **No knowledge subsystem.** Tried twice, failed twice. Documents plus
  automatic surfacing do that job.
- **No rich text.** Documents are Markdown files in git. Every tool that
  touches them — including our own editor — works on the plain file.
- **No multi-project features.** One Subutai installation manages one
  project.

## 12. From Cromwell to Subutai: what actually changes

For readers who know Cromwell — a summary of the deltas, because the list is
shorter than the discussion that produced it:

1. **The name.** Cromwell becomes Subutai, once, at a clean boundary.
2. **The seam moves.** Orchestration used to begin when a design was
   approved (design approval triggered spec-writing agents). Now the spec is
   written in the planning conversation, and orchestration begins at *Submit
   to development*. The spec-authoring agent roles retire; their skills
   become chat-side material. Design approval becomes a record rather than a
   trigger, so approving via chat is fine.
3. **The spec reviewer is kept and promoted** to border guard — the intake's
   first step. The design reviewer is retired (a design gets its human
   review in the conversation that writes it).
4. **Executors.** Tasks record who did the work; chat and humans can work
   any stage through the claim/submit doorway. Judging stays closed.
5. **Pre-completion.** Intake steps check state instead of insisting on
   running; work done ahead in chat is recognised and skipped over.
6. **Document management grows up.** System-minted IDs, per-initiative
   folders, create-with-entity, frontmatter identity, and the browser
   editor.
7. **Observability grows up.** Full dispatch transcripts, the feature
   timeline, review-health metrics, attribution everywhere.
8. **Bugs, spikes, and decisions become first-class**, as described above.
   Spikes in particular turn a rule that used to live in prose — throwaway code
   never ships — into a property of the entity: a spike has no merge path.
9. **Everything else stands.** The orchestrator, the gates, the document
   lifecycle, worktree isolation, checkpoints, severity-gated review loops,
   token sizing, the audit trail — unchanged, by design.

## 13. Open questions for revision

Left deliberately open for Sam's read-through:

- **The exact ID and folder scheme** — the shape shown in Section 6 is a
  proposal; the prefixes and layout deserve a quick round of their own.
- **Save & commit defaults** — whether plain Save or Save & commit is the
  primary action in the editor.
- **Reviewer models for chat-executed work** — whether reviews should
  default to a stronger model when the executor was the chat AI (the
  independent review carries more weight in that case).
- **Bug triage surface** — inbox checkpoint, dedicated queue, or both.
- **What a spike's budget does when it trips.** Section 9 assumes a checkpoint
  — the spike pauses and asks whether to continue. The alternative is a hard
  stop. Checkpoint is the friendlier default and the easier one to abuse; a
  hard stop is the one that actually protects an afternoon. Also open: whether
  the budget has a project-wide default or must be set per spike.
- **How much of a decision gets pushed** into agent context, and the size
  cap that keeps surfacing affordable.
- **The formal decision records** — the seam move, the executor model, and
  the judgement boundary each revise accepted decisions (notably DEC-005)
  and should be written up as proper decision documents once this design is
  approved.
