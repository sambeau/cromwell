# SPEC-018: Decisions

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. Sam has said they will
approve the spec and the build together. An independent review is recorded in
[REVIEW-018](../reviews/REVIEW-018-decisions.md); §7 says how each finding was
dealt with. Fourteen choices need Sam's explicit yes (DoD 8).
**Date:** 2026-09-28
**Roadmap milestone:** M11 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md)
§11: "an agent's transcript shows the relevant decisions in its prompt."
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) (approved
2026-09-28): **§11** is the brief. Also §4 (successors), §7 (IDs: `DEC-` is
its own sequence; default homes; the dated note on decisions in the editor)
and §17b (the size of the surfacing cap, settled here).
**Research:**
- [Context for dispatched agents](../research/context-for-dispatched-agents.md)
  §3 and the [evidence base](../research/agent-research-evidence-base.md) §4.1:
  agents don't fetch, so what matters is pushed; keep the pushed block small.
- [Prefix-cache discipline](../research/prefix-cache-discipline.md) §6 to §9:
  order a prompt by how many dispatches share each part. It records a defect
  (volatile content ahead of shared content) that this spec fixes (SD-10).
- The [research conformance audit](../notes/research-conformance-audit-2026-07-29.md),
  **C-6**: a project conventions document, pushed into every dispatch, with a
  hard size cap at validation.

**Builds on:** [SPEC-015](SPEC-015-documents-with-identity.md) (`DEC-nnn`
minting and adopting existing decisions),
[SPEC-016](SPEC-016-edit-in-the-browser.md) (SD-12: an accepted decision isn't
revised in the editor), [SPEC-017](SPEC-017-chat-as-a-proper-seat.md) (who
wrote and who judged; SD-8 names M11 for the untemplated types) and
[SPEC-012](SPEC-012-see-the-work.md) (transcripts: "what it was told").
**Follow-ups taken up:** the M8 handoff's follow-up 3 (Revise on a decision
makes a successor that can't be submitted), for decisions; SPEC-017 SD-8.
**Coordination:** M12 (bugs) runs in parallel. This spec owns migration
`0012`, and M12 owns `0013`; M11 merges first. This spec owns prompt assembly
(`planner.go`'s prompt parts, `review_prompt.go`, `content/assemble.go`). The
starter pack and the MCP tool-set test are shared, and merged by hand.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6).

Four words are kept apart:

- **accepted** is how a decision's `approved` state is said to a person, as
  DESIGN-010 §11 says it. In the database it is still `approved`.
- **surfaced** means pushed into a dispatch's prompt by Subutai. Nothing here
  is fetched by an agent.
- **amendment** is a dated section appended to an accepted decision, made as
  a new revision of the same `DEC-nnn`.
- **supersession** is a *different* decision, with its own number, replacing
  an accepted one.

## 0. Framing

Subutai already has decision documents in name. M8 made `decision` a document
type, minted `DEC-nnn` numbers, and adopted this repository's DEC-001 to
DEC-007. But a decision has no template, so a new one can't be submitted,
revising one makes a successor nobody can approve, and nothing tells an agent
any decision exists. An implementer on a Go project is not told the project
chose Go.

DESIGN-010 §11 asks for three things: decisions as proper documents that are
never edited, only amended or superseded; the ruling and a one-line reason
pushed into every relevant dispatch, under a hard cap; and a viewer. It adds
that project conventions use the same mechanism, which is the research
audit's C-6.

## 1. Goal

**The project remembers why, and agents are told.**

Done looks like this, with no AI provider:

1. A person creates a decision in the browser. It gets the next `DEC-` number
   and opens in the editor on the template. They write a short ruling and a
   one-line reason, submit it, and approve it. A 200-word ruling is refused
   at submit with a sentence saying so.
2. A dispatch for a feature under that initiative runs on the mock provider.
   Its transcript's "what it was told" shows the ruling and reason, and the
   project conventions, and nothing from other branches of the tree.
3. They supersede the decision with a new one. The old one leaves every
   later prompt; the viewer shows each pointing at the other.
4. They amend another decision. The amendment is appended, dated, and
   approved; the text above it is unchanged byte for byte.
5. `/ui/decisions`, and `list_decisions` over MCP, show every decision with
   its state, what it supersedes and what superseded it.
6. This repository's DEC-001 to DEC-007 still adopt as they are.

## 2. What exists today

| What | Where it stands |
|---|---|
| `decision` document type, `DEC-nnn` numbers | Built in M8 (SPEC-015 SD-6, migration `0010`). A decision belongs to the project or an initiative; adopt refuses a feature. |
| Adopting DEC-001 to DEC-007 | Built in M8, as approved in the web UI, because a type **with no template** may be adopted as approved (`approvableByAdoption`). Giving decisions a template would silently end that; SD-3 keeps it. |
| Submit, review, approve | Types with a template only. A design (`approved_by: human`, no reviewer role) waits in review for a person, who approves in the UI or by `relay_verdict`. |
| Revise on a decision | Refused in the editor and on the page (SPEC-016 SD-12), pointing at supersession or amendment, neither of which exists. |
| Anything pushed into prompts | Review dispatches receive approved documents owned by ancestor initiatives, **whole**, as background. That would push an initiative's decisions as essays (SD-9). Execution dispatches receive only the spec and plan. |
| Transcripts | Built in M6. The assembled system and user prompts are stored whole, and the run page shows them under "What it was told". |

## 3. Scope

### In scope

1. **A decision template** in the starter pack, submitted and approved by a
   person (FR-1), and **creating a decision** in the web UI and over MCP
   (FR-2).
2. **Amendment and supersession** (FR-3), and Revise doing the right thing.
3. **The surfaced portion**: two front-matter fields, capped at validation on
   every route to accepted (FR-4).
4. **A conventions document**: its own type and template, one per project,
   capped, approved by a person (FR-5).
5. **Surfacing** into every dispatch, under a per-dispatch cap, recorded in
   the transcript (FR-6), with the prompt order fixed for the cache (SD-10).
6. **A viewer**: `/ui/decisions`, `list_decisions` and `get_decision` (FR-7).
7. **This repository**: DEC-001 to DEC-007 adopt as they are, proved by test
   (FR-8).

### Out of scope

| What | Where it goes |
|---|---|
| Bugs, executors, spikes, retrospectives | M12, M13, M14; C-9 |
| A human verdict path for **note, research, report, policy** (SPEC-017 SD-8) | Not built. SD-12 says why and recommends the follow-up. |
| Surfacing decisions into the chat agent's context | Nothing is pushed into chat; the chat agent reads with `list_decisions`. The chat prompt isn't Subutai's to assemble (context research §3, "an unresolved edge"). |
| An agent reviewer for decisions in the starter pack | Not shipped (SD-2). A project may add one. |
| Amending over MCP | The web UI only (SD-6). The chat agent can draft a superseding decision. |
| Changing the per-feature `decision` owner rule | Unchanged: project or initiative. |
| Anthropic `cache_control` breakpoints | The prefix-cache research's item 2. Separate, and in the provider, not the prompt. |

### Scope decisions

**SD-1 — The surfaced portion is two front-matter fields, `ruling:` and
`reason:`.** (Choice 1.)

- **Why front matter, not a fixed section.** The pushed text is then exactly
  what the author wrote for pushing, separable from the essay by
  construction. It can be checked field by field. An amendment can restate it
  without rewriting the body, which is what lets the body be append-only
  (SD-5). And a decision adopted from elsewhere, with its own headings, needs
  no heading of ours to be surfaced.
- **What surfaces:** `DEC-008: <ruling> Why: <reason>`, on one line per
  decision.
- **A decision with no `ruling:`** — every adopted decision, including
  DEC-001 to DEC-007 — surfaces by its **title**, which in practice is a
  ruling in brief ("DEC-001: Server Language — Go"). The viewer marks it "No
  ruling recorded; its title is surfaced", and **Record its ruling** (SD-5)
  adds one.
- The alternative, a fixed `## Ruling` section, reads better in a plain
  Markdown viewer. It loses the three properties above.

**SD-2 — A person approves a decision. No agent reviewer comments first.**
(Choice 2.)

- The manifest is `approved_by: human` with no `reviewer_role`. A submitted
  decision waits in review for a person, exactly as a design does, and is
  approved in the web UI or relayed from chat with `relay_verdict`. DEC-006
  Amendment 1, decision 8 already lets the chat agent relay "a human's
  verdict on any document", so no new relay is needed.
- **Why no reviewer:** designs had one and it was retired in M3 (DESIGN-010
  §17a item 1). A decision is a person's ruling, usually made in the
  conversation that led to it. The mechanical parts a reviewer might catch
  (length, a missing reason, superseding the wrong thing) are checked at
  validation instead (FR-4, FR-3.6).
- **A project that wants one** sets `reviewer_role` in its
  `templates/decision/manifest.yaml`. With `approved_by: human` that gives a
  comments-only review through the existing machinery (SPEC-009 FR-2.2). No
  code is needed.

**SD-3 — Decisions stay adoptable as approved, and gain "This was already
approved".** SPEC-015 SD-11 allows approved adoption for designs and types
with no template. Decisions now have a template, so the rule becomes
**design, decision, or a type with no template**. *This was already approved*
(SPEC-015 FR-5.8), offered on an adopted draft of an untemplated type, is
also offered on an adopted decision draft. An adopted decision won't have the
template's headings, so it could never pass Submit. Both routes check the
surfaced fields' caps (FR-4.3), and no other part of the template.

**SD-4 — Where a new decision lives.** A project decision is written to
`docs/decisions/DEC-nnn-<slug>.md`, beside this repository's own. An
initiative's goes in the initiative's default home,
`docs/work/<INIT-ID>-<slug>/DEC-nnn-<slug>.md`, as DESIGN-010 §7 says of
documents Subutai creates. The slug comes from the title. Neither is a rule:
the file is found by its ID wherever it moves (SPEC-015 SD-10). (Choice 3.)

**SD-5 — An amendment is a new revision that only appends.** (Choice 4.)

- **Append an amendment**, on an accepted decision's page, opens a revision
  exactly as Revise does (same `DEC-nnn`, next revision, a working copy at
  `….rev.md`), with a skeleton appended:
  `## Amendment <n> — {{what changed}} (<today>)` and a placeholder
  paragraph. `n` is one more than the number of `Amendment` headings already
  in the file. It opens in the browser editor.
- **Submitting it checks that it only appends** (FR-3.3): the accepted text
  is kept byte for byte, and only a dated amendment follows it. Front matter
  may change only in `ruling:` and `reason:` (and the identity lines), so an
  amendment that changes the ruling restates it for the prompt.
- **Record its ruling** is the one case with nothing appended: a decision
  with no `ruling:` may gain `ruling:` and `reason:` in a revision whose body
  is unchanged. Recording what a decision already says changes nothing it
  says. The same append-only check applies, so nothing else can change. This
  is how DEC-001 to DEC-007 get proper surfaced text.
- **A person approves it**, as any decision. Approval supersedes the previous
  revision, archiving it as `docs/_superseded/DEC-006.r1.md` (SPEC-015
  SD-9), exactly as any revision is.
- **Why a revision, not an edit in place:** DESIGN-003 L-2 keeps approved
  documents immutable, and DESIGN-010 §11 says an accepted decision is never
  edited. A revision keeps both true: the accepted text is never changed, and
  the amendment is itself accepted by a person before it counts, as DEC-006's
  Amendment 1 was. It reuses the lifecycle, the archive and the audit trail as
  they are.

**SD-6 — Supersession is a new decision naming the old in `supersedes:`.**
(Choice 5.)

- **Supersede with a new decision**, on an accepted decision's page, creates
  a new decision draft (FR-2) with the same owner and `supersedes: [DEC-005]`
  already filled in, and opens it in the editor. The chat agent can do the
  same with `create_decision` and `supersedes`.
- **Accepting the new decision supersedes the old one** in the same
  transaction: its state becomes `superseded` and the link is recorded. The
  old file **stays where it is and is not changed**, because other documents
  cite it and it is a record. Only an amendment's older revisions are
  archived, because they are replaced by a newer copy of the same text.
- **Which decisions a decision may supersede** (checked at submit, FR-3.6):
  accepted ones, owned by the same owner or by one below it. A project
  decision may supersede an initiative's; an initiative's decision may not
  retire a project-wide one, because that would change the rules for every
  other branch of the tree.
- Amending from chat is left out (Out of scope): the amendment is written in
  the browser editor, and appending is a small act a person does there.

**SD-7 — Revise on a decision does the right thing.** The page's Revise
button is replaced, on an accepted decision, by **Append an amendment** and
**Supersede with a new decision** (with **Record its ruling** when it has
none). The editor's refusal (SPEC-016 SD-12) names both. `ReviseDoc` itself —
reached by the HTTP API or the CLI's `doc revise` — opens an amendment
(SD-5), so no route makes a successor that can't be submitted. This closes
the M8 handoff's follow-up 3 for decisions.

**SD-8 — The caps, tuned against real prompt sizes.** (Choice 6.)

The measured numbers:

- The role system prompts in the starter pack are 600 to 1,500 tokens.
- The first turn of a real spec or plan review is 1,700 to 2,000 input
  tokens (walkthrough, phase 2 and the first live run). An implement dispatch
  runs to about 14,000 tokens over all its turns, and every turn resends the
  prompt.
- The context research says compliance falls away past about nineteen
  requirements, and asks for "five to fifteen things that actually change
  what the agent writes".

At DESIGN-010 §17b's proposal of 150 words per decision, 1,500 tokens holds
five or six decisions, which is too few. So the defaults tighten the
per-decision size and keep the per-dispatch cap:

| Cap | Default | Where it is enforced |
|---|---|---|
| A decision's `ruling:` | 75 words | Validation (the decision manifest) |
| A decision's `reason:` | 25 words, on one line | Validation |
| The conventions document's body | 300 words | Validation (the conventions manifest) |
| One dispatch's surfaced block | 1,500 tokens, estimated | At assembly (FR-6.4) |

- 100 words is about 130 tokens. A typical ruling and reason run to about 50
  words, so the block holds the conventions (at most about 400 tokens) and
  roughly 12 to 15 decisions. That is the research's range.
- **Tokens are estimated** as a quarter of the block's characters, rounded
  up, because Subutai has no tokeniser. For English prose that errs slightly
  high, which is the safe direction.
- The word caps live in the manifests as rules (FR-4.1), so a project can
  change them by editing its own templates. The dispatch cap is
  configuration, `surfacing.max_tokens` in `config.yaml` (FR-6.5).
- **Adding it up:** 1,500 tokens is 75 to 90 per cent more than a review's
  first turn today, and about 10 per cent of an implement dispatch. It is
  also the part of the prompt most shared across dispatches (SD-10), so it is
  cached after the first dispatch on a branch.

**SD-9 — What happens when the cap is hit.** (Choice 7.)

- **The conventions document always goes in.** It is capped at validation
  well below the dispatch cap, and the configuration refuses a dispatch cap
  too small to hold it (FR-6.5).
- **Decisions are admitted nearest first:** those of the dispatch's own
  initiative, then its parent's, outwards, then the project's. Within one
  owner, **newest accepted first.** The first decision that doesn't fit stops
  that owner's admissions and every owner further out, so what is left out is
  always the far end of the tree, and oldest first within it.
- **Why nearest first:** the nearer decision is more specific, and it is the
  one an agent is least likely to infer from the code. Project-wide decisions
  ("the server is in Go") are usually visible in the codebase already.
- **What is left out is said, in three places:**
  1. **In the prompt**, as the block's last line: "Left out for space:
     DEC-001 (Server Language — Go), DEC-002 (…)." The IDs and titles cost
     little and let an agent with `read_file` look one up if it bears on the
     task.
  2. **In the transcript**, because it is part of the prompt.
  3. **In the viewer**: a decision left out on any branch of the tree is
     marked "Left out of some dispatches for space", with a banner saying
     which branches are over the cap (FR-7.2).
- **What is never done:** truncating a ruling mid-sentence. A ruling is
  whole or absent.

**SD-9a — Which dispatches receive it: every one.** Implement, code review,
verify, estimate, spec and plan writing, and document review all receive the
block. (Choice 8.)

- **Verification is a fair question**, because it is meant to have clean
  context (DESIGN-003 L-4). The block isn't the implementation's story,
  though. It is project law, and a verifier judging "does this meet the
  contract" is better for knowing, say, that the project forbids a
  dependency.
- **A dispatch whose subject has no initiative** — a review of a
  project-level document — receives the project's decisions and conventions
  only.
- **A review of a decision or of the conventions document** receives the
  surfaced block like any other. It never receives the document under review
  twice: an accepted decision under amendment is left out of its own
  amendment's review.

**SD-10 — Where the block goes, and fixing the prompt order.** (Choice 9.)

- The block is the **first thing in the user message**, after the project's
  name, under `# Project decisions and conventions`. Its order is fixed:
  the conventions, then the project's decisions, then each initiative's from
  the outermost in, and within one owner by number ascending.
- **Why there.** The system prompt stays role-only and byte-stable for every
  dispatch of a role (research §6, "what's already right"). The block is
  shared by every dispatch on its branch, whatever the role, so it opens the
  user message. Rendering widest-shared first means dispatches on two sibling
  initiatives share the prefix up to their first difference, and a new
  decision appends at the end of its owner's run rather than shifting
  everything.
- **The defect this fixes.** Implement and code-review prompts put the task
  first, ahead of the spec and plan, so the shared contract was re-billed on
  every task. Spec and plan writing put the feature ahead of its
  initiative's design. The prefix-cache research's recommended order is
  adopted:

  | Purpose | New order of the user message |
  |---|---|
  | implement-task | project, surfaced block, spec, dev-plan, how to work, **task** |
  | review-code | project, surfaced block, spec, dev-plan, **task**, diff, instruction |
  | verify-feature | project, surfaced block, spec, feature, instruction |
  | estimate | project, surfaced block, work, reference points, instruction |
  | write-spec, write-dev-plan | project, surfaced block, initiative's design, feature's design, (approved spec), structure, **feature**, revision context, instruction |
  | review-&lt;type&gt; | project, surfaced block, background documents, feature, task, validation, comments, issues, document |

  The task moves into the recency slot, which is what the attention research
  asks for too, so the change costs nothing on either axis.
- The block is recorded nowhere but the prompt. The transcript already
  stores the prompt whole, so "what it was told" shows it with no new
  plumbing.

**SD-11 — Decisions and conventions stop arriving as whole background
documents.** Review dispatches today receive every approved document owned by
an ancestor initiative, whole. An initiative's decision would then arrive
twice: as an essay in the background and as a ruling in the block. Decisions
and conventions are excluded from the background documents. (Folded into
choice 8.)

**SD-12 — The other four untemplated types keep their current path.**
SPEC-017 SD-8 named M11 to decide "what reviewing a decision means, and the
same answer serves the others". The answer for decisions is a template with a
person's verdict. For a note, research, report or policy the same answer
would mean four templates nobody has asked for. So:

- they stay as they are: adoptable as approved, *This was already approved*
  in the web UI, and not revisable in the editor (SPEC-016 SD-12);
- **recommended follow-up:** a *floor-only* verdict path for untemplated
  types (submit checks only the non-configurable floor: parse, placeholders,
  links; a person decides), which would also let `relay_verdict` carry
  "already approved" from chat.

(Choice 10.)

**SD-13 — The conventions document.** One per project. (Choice 11.)

- Type `conventions`, owner **the project only**, ID `PROJECT-conventions`,
  default path `docs/conventions.md`. A second live one is refused, as a
  second live spec is.
- **The whole body is surfaced**, below its front matter and without its
  title heading. It is short by construction: 300 words at validation. The
  template suggests headings for code style, architecture, key interfaces
  and tools, and says five to fifteen points.
- Approved by a person, like a decision. Unlike a decision, it is revised
  normally (Revise, edit, submit, approve): conventions are expected to
  change, and nothing cites a version of them.
- **Created** from the decisions page (*Start the conventions document*) when
  the project has none, or adopted from an existing file. Adopting one as
  approved checks its size (FR-5.3).

**SD-14 — What is stored.** Migration `0012` adds one type value, and two
tables. (Choice 12.)

- `document_type` gains `conventions`.
- **`surfaced_texts`**: for each accepted decision and conventions revision,
  the text that is pushed, recorded at the moment of acceptance. Surfacing
  reads this table, not the file, so a hand edit to an accepted decision (an
  integrity question, raised as today) can never reach a prompt, and a
  dispatch reads no files for it.
- **`decision_supersessions`**: which decision superseded which, recorded
  when the superseding decision is accepted.
- A server started on an existing database fills `surfaced_texts` for
  accepted decisions and conventions that lack a row, from their files, if
  each file's hash is still the one recorded. One that has drifted is left
  out and logged; its integrity question already exists.

**SD-15 — Creating a decision over MCP is planning authoring.** (Choice 13.)
`create_decision` writes a draft from the template and registers it, which
DEC-004 allows ("attach documents to entities, and author document
content"). The chat agent then fills the file in and hands it in with
`submit_for_review` (SPEC-017), and a person accepts it on the page or by a
relayed verdict. `list_decisions` and `get_decision` are reads. No new relay
tool is added.

**SD-16 — The one change outside prompt assembly in shared review code.**
Accepting a decision has to supersede what it names, and record its surfaced
text, in the approval's own transaction. `approveDocumentAs` in
`internal/server/actions.go` gains one call, `s.onAccepted(ctx, tx, doc)`,
beside its predecessor handling. Nothing else there changes. (M12 owns the
review-outcome handling; the handoff names the line.) The two other routes
to accepted, adoption and *This was already approved*, call the same
function.

## 4. Requirements

### FR-1: The decision template

**FR-1.1** The starter pack gains `templates/decision/manifest.yaml` and
`template.md`:

```yaml
type: decision
approved_by: human
front_matter:
  required: [title, type, owner, ruling, reason]
sections:
  order: strict
  required:
    - heading: Context
    - heading: Alternatives considered
  optional:
    - heading: Consequences
rules:
  - kind: max_words
    field: ruling
    max: 75
  - kind: max_words
    field: reason
    max: 25
  - kind: one_line
    field: reason
```

The template's front matter carries `title`, `type: decision`, `owner`,
`ruling`, `reason`, and `supersedes: []`, each with a `{{…}}` placeholder that
says what belongs there and how short it must be. The body is the heading,
then the three sections. What it supersedes is the `supersedes:` field.

**FR-1.2** Two new rule kinds join the registry (`lifecycle.RuleKinds`), and
are listed in DESIGN-004's registry note:

- `max_words` with `max:` and exactly one of `field:` (a front-matter field),
  `section:` (a section's content) or `body: true` (everything below the
  front matter, less a leading level-1 heading). It fails with, for example:
  "The ruling is 112 words; a decision's ruling is at most 75, because it is
  pushed into every agent's prompt. Keep the ruling itself here, and put the
  argument in Context."
- `one_line` with `field:`: the field holds no line break.

Words are runs of non-space characters.

**FR-1.3** A decision is submitted, validated and put in review like any
templated document. With no reviewer role it waits in review, and the page
says "A person decides whether to accept this decision." Approve and Send
back are offered, as for a design. The chat agent relays either with
`relay_verdict` and the person's words.

**FR-1.4** Pages and notices say *accepted* for a decision's approved state:
the state chip, the viewer, and the approval notice ("DEC-008 is accepted.
Agents will be told its ruling from their next dispatch.").

### FR-2: Creating a decision

**FR-2.1** `/ui/decisions` has **New decision…**, a dialog with a title, an
owner (the project, or an initiative chosen from a list), and optionally
decisions it supersedes (from the accepted ones the owner may supersede,
SD-6). It:

1. mints the next `DEC-nnn`;
2. writes the template to the default home (SD-4), with `title`, `type`,
   `owner` and `supersedes` filled in, and `id:` and `revision: 1`;
3. registers the draft, records the writer, and commits the file as the
   tool's git author;
4. opens it in the browser editor.

An initiative page's documents panel has the same action, with its owner
fixed.

**FR-2.2** A file that already exists at the path is never written over: the
dialog refuses with a sentence naming the file.

**FR-2.3** `create_decision` over MCP takes `title`, `owner` (`project`, or an
initiative's path or ID) and optional `supersedes` (a list of `DEC-` IDs).
It does FR-2.1 steps 1 to 3 and returns the document entry and a sentence:
"DEC-008 is a draft at docs/decisions/DEC-008-…md. Fill in its ruling,
reason and context, then hand it in with submit_for_review; a person accepts
it." Its writer is the chat agent. It is added to the tool-set test under
DEC-004.

### FR-3: Amendment and supersession

**FR-3.1 — Append an amendment** (SD-5), offered on an accepted decision's
page when no revision of it is open. It makes the successor through the same
path as Revise, appends the skeleton, and opens the editor. `ReviseDoc` on a
decision does the same (SD-7).

**FR-3.2 — Record its ruling**, offered instead of Append when the accepted
decision has no `ruling:`. It makes the successor with `ruling: "{{…}}"` and
`reason: "{{…}}"` added to the front matter and the body unchanged.

**FR-3.3 — Only appending passes.** When a decision that revises a
predecessor is validated (at Validate and Submit), the checks are these, and
not the template's section checks, because an adopted decision won't have the
template's headings:

1. **The accepted text is kept.** The predecessor's body is read from its
   file, and its hash is checked against the one recorded; the successor's
   body must begin with that body exactly, trailing blank lines aside.
2. **Only a dated amendment follows**: a level-2 heading beginning
   `Amendment` and ending in a date in brackets, `(2026-09-30)`, then text.
   The one exception is Record its ruling (SD-5): nothing follows, and the
   predecessor had no `ruling:`.
3. **Front matter** differs only in `ruling:`, `reason:`, `id:` and
   `revision:`. `title`, `type`, `owner` and `supersedes` can't change.
4. The **floor checks** apply to the whole file (parse, placeholders, links),
   and the **surfaced-field caps** (FR-4.1) to `ruling:` and `reason:` if
   present.

Each failure is a sentence, for example: "An accepted decision is never
edited: the text of DEC-006 revision 1 must be kept exactly, and an
amendment added after it. The first difference is on line 42."

**FR-3.4 — Supersede with a new decision**, offered on an accepted
decision's page. It is FR-2.1 with the owner and `supersedes` filled in.

**FR-3.5 — Accepting** (SD-16), in the approval's transaction:

1. `surfaced_texts` records the accepted revision's ruling and reason, or its
   title if it has no ruling (FR-6.2).
2. For each ID in `supersedes:`, the current accepted revision of that
   decision is moved to `superseded` by the lifecycle engine, and
   `decision_supersessions` records the pair. Its file is not moved or
   changed. The audit row names the superseding decision.
3. An open amendment of a decision superseded this way is left as a draft.
   Submitting it is refused: "DEC-005 was superseded by DEC-008, so it can't
   be amended. Detach this draft."

**FR-3.6 — Supersession checks at submit.** Each ID in `supersedes:` must name
a decision that is accepted now, and owned by the same owner as the new
decision or one below it. Otherwise: "DEC-005 isn't an accepted decision, so
this can't supersede it", or "DEC-003 belongs to the project, and a decision
of INIT-004 can't supersede it: that would change the rules for the whole
project. Make this a project decision, or leave DEC-003 out."

**FR-3.7 — The editor and the page agree.** On an accepted decision the
editor's refusal reads: "An accepted decision is never edited. Append a dated
amendment, or supersede it with a new decision, from its page." The page
offers exactly FR-3.1 or FR-3.2, and FR-3.4.

### FR-4: The surfaced portion is short by construction

**FR-4.1** The decision manifest's rules (FR-1.1) cap `ruling:` at 75 words
and `reason:` at 25 words on one line. Validation at submit enforces them, so
an over-long ruling can't reach review, and so can't be accepted by the normal
route.

**FR-4.2** A draft decision's page shows the surfaced line as agents will see
it, and its size in words, next to the caps.

**FR-4.3** The routes to accepted that skip Submit check the same caps:
adopting a decision as approved, and *This was already approved*. They read
the caps from the project's decision manifest. A decision with no `ruling:`
passes, and is surfaced by its title. Refusal: "This decision's ruling is 112
words, and the project's cap is 75, so it can't be recorded as accepted:
agents would be told all of it. Adopt it as a draft and shorten the ruling
first."

### FR-5: The conventions document

**FR-5.1** The starter pack gains `templates/conventions/`:

```yaml
type: conventions
approved_by: human
front_matter:
  required: [title, type, owner]
sections:
  order: any
  required: []
rules:
  - kind: max_words
    body: true
    max: 300
```

The template's front matter is `title: "Project conventions"`,
`type: conventions`, `owner: project`. Its body is a heading, a sentence
saying everything below it is pushed into every agent's prompt, and suggested
sections (Code, Architecture, Key interfaces, Tools), each a short list of
placeholders.

**FR-5.2** One live conventions document per project. Attach and adopt refuse
a second ("This project already has its conventions document, at
docs/conventions.md. Revise that one instead."), and refuse any owner but the
project.

**FR-5.3** Adopting a conventions document as approved is allowed in the web
UI (SD-3's list gains `conventions`), and checks its size (FR-4.3).

**FR-5.4** `/ui/decisions` shows the accepted conventions at the top, with its
state, size and a link; or, when there are none, *Start the conventions
document*, which writes the template to `docs/conventions.md`, registers and
commits it, and opens the editor.

### FR-6: Surfacing

**FR-6.1 — What a dispatch receives.** For a dispatch whose subject is a
task, a feature, or a document owned by a feature or an initiative, the
**branch** is the project and each initiative from the root down to the
subject's initiative. The block holds:

1. the project's accepted conventions, if any;
2. every accepted decision owned by the project or an initiative on the
   branch, except the document under review itself (SD-9a).

Superseded decisions are never included: they are not accepted. A decision
with an open amendment is included as accepted, until the amendment is.

**FR-6.2 — The line for a decision.** `- DEC-008 (INIT-004 Auth): <ruling>
Why: <reason>` — the owner in brackets only for an initiative's decision.
With no recorded ruling, the title: `- DEC-001: Server Language — Go`.

**FR-6.3 — The block.**

```
# Project decisions and conventions

These are binding. They were decided by people, and they override anything
in the task that contradicts them; if one seems wrong for this task, say so
in your outcome rather than working around it.

## Conventions

<the conventions body>

## Decisions

- DEC-001: Server Language — Go
- …

Left out for space: DEC-002 (Postgres via Supabase). Their files are in the
repository.
```

With no conventions and no decisions the block is absent entirely, so a
project that uses neither sends the same prompt as before.

**FR-6.4 — The cap** (SD-9). The block's estimated tokens, including its
headings and the left-out line, must not exceed `surfacing.max_tokens`.
Admission is nearest owner first, newest accepted first within an owner; the
first that doesn't fit ends admission for its owner and every owner further
out. Rendering order is SD-10's. The same inputs always produce the same
bytes.

**FR-6.5 — Configuration.**

```yaml
surfacing:
  max_tokens: 1500   # default
```

Absent, the default applies. A value below 600 is refused at load ("…is too
small to hold the conventions document, which may be up to 300 words"). A
value of 0 is refused too; there is no "off", because a project with nothing
accepted sends nothing.

**FR-6.6 — Every purpose** calls one function, `s.surfacedBlock(ctx, branch,
exclude)`, and places the result per SD-10. `content.AssembleReviewPrompt`
gains a `Surfaced string` input placed after the project name.

**FR-6.7 — Background documents** in review prompts exclude `decision` and
`conventions` (SD-11).

### FR-7: The viewer

**FR-7.1 — `/ui/decisions`**, linked from the navigation beside Documents. A
table, one row per decision ID (its newest revision), with:

- the ID and title, linking to the document page;
- its owner (the project, or the initiative's ID and name);
- its state (draft, in review, accepted, superseded);
- the surfaced line, or "No ruling recorded; its title is surfaced";
- **Supersedes** and **Superseded by**, each a list of linked IDs;
- **Amended**, when its revision is above 1, with the dates of its
  amendments.

Filters, as links that keep each other: owner (all, the project, or one
initiative and its ancestors, meaning "what its dispatches are told"), and
state (accepted by default; all; drafts and in review; superseded).

**FR-7.2** With an initiative chosen, the page says what a dispatch there
receives, and its estimated size against the cap. Where the cap leaves any
decision out, a banner names the branch and the decisions left out; each such
row carries "Left out of some dispatches for space".

**FR-7.3 — `list_decisions`** over MCP: optional `owner` (`project`, or an
initiative's path or ID, meaning that initiative's branch) and `state`
(`accepted` by default, `all`, `open`, `superseded`). Each entry: `id`,
`title`, `owner`, `state` (with `accepted` for approved), `ruling`, `reason`,
`supersedes`, `superseded_by`, `revision`, `path`. With an owner, the result
also gives `conventions` (path and state) and `left_out`, the IDs the cap
leaves out on that branch.

**FR-7.4 — `get_decision`**: `id` (a `DEC-` ID). The entry above, plus the
file's text, the amendments' headings and dates, and `written_by` and
`last_verdict` as other document results have them.

**FR-7.5** Both are reads, advertised and named in the tool-set test.

### FR-8: This repository

**FR-8.1** An integration test copies DEC-001 to DEC-007 into a test project,
adopts each as approved in the web UI route, and checks:

- each keeps its number and its bytes below the identity lines;
- each is accepted, with its title as its surfaced text;
- a dispatch on that project receives all seven titles, in number order;
- `create_decision` then mints DEC-008.

**FR-8.2** Record its ruling on one of them (FR-3.2) passes the append-only
check with the body unchanged, and after acceptance the dispatch receives the
recorded ruling instead of the title.

## 5. Non-functional requirements

- **NFR-1 — Deterministic.** The block is a pure function of the accepted
  rows and the cap. The unit tests render it from fixed inputs and compare
  bytes.
- **NFR-2 — Cheap.** Building the block is one query for the branch's
  surfaced texts and one for conventions; no file is read at dispatch time.
- **NFR-3 — Unchanged when unused.** A project with no accepted decisions and
  no conventions sends prompts that differ from today's only in SD-10's
  reordering.
- **NFR-4 — Words for people.** Every refusal is a sentence that says what to
  do (DESIGN-008 D-6).
- **NFR-5 — No knowledge base.** Nothing here offers agents a way to query
  decisions. What they need is pushed (evidence base §4.1).

## 6. Open questions carried forward

- **A floor-only verdict path** for note, research, report and policy (SD-12).
- **Per-role surfacing.** Every role gets the same block. If some decisions
  only matter to, say, the implementer, a `roles:` field could narrow them.
  Not proposed until a real project asks.
- **Measuring the cap in use.** The dispatch rows already record cache reads;
  a read of block size against `cache_read_tokens` after a real run would
  confirm SD-10's claim, as research §7 item 1 asks.
- **Amending from chat.** Left out (SD-6); easy to add if wanted.

## 7. Changes after review

*Filled in after REVIEW-018.*

## Definition of done

1. `go vet ./...` is clean, and `go test -race -count=1 ./...` passes with
   the integration tests run, not skipped.
2. With the mock provider, tests prove:
   1. a dispatch's transcript contains exactly the right decisions and
      conventions for its branch, and none from a sibling branch;
   2. a superseded decision is left out;
   3. the cap is enforced, admitting nearest first and naming what it left
      out;
   4. validation refuses an over-long ruling, a multi-line reason and an
      over-long conventions document, at submit and on the routes that skip
      it;
   5. an amendment that only appends is accepted, and one that changes the
      accepted text is refused;
   6. supersession moves the old decision to superseded without touching its
      file, and refuses a narrower owner;
   7. the new prompt orders (SD-10).
3. DEC-001 to DEC-007 adopt as they are (FR-8).
4. The viewer and both MCP reads work, and the tool-set test names the three
   new tools.
5. A demo with no AI provider, in a browser: create and accept a decision,
   supersede it, show the viewer, and show a mock dispatch's transcript
   containing the ruling. Recorded in `docs/walkthrough-spec-018.md`.
6. A handoff note.
7. The roadmap's §11 marks M11 done, pointing at the handoff.
8. **Sam's yes to fourteen choices:**
   1. the surfaced portion is `ruling:` and `reason:` front matter, and a
      decision without them surfaces by title (SD-1);
   2. a person accepts a decision, with no agent reviewer in the pack
      (SD-2);
   3. new decisions live in `docs/decisions/` for the project and in the
      initiative's folder otherwise (SD-4);
   4. an amendment is an append-only revision, accepted by a person, with
      Record its ruling as the one no-append case (SD-5);
   5. supersession is a new decision's `supersedes:`, taking effect on
      acceptance, leaving the old file untouched, and never retiring a
      decision of a wider owner (SD-6);
   6. the caps: 75-word ruling, 25-word one-line reason, 300-word
      conventions, 1,500 estimated tokens per dispatch (SD-8);
   7. at the cap, nearest owner first and newest first, and the left-out
      line in the prompt and the viewer (SD-9);
   8. every dispatch purpose receives the block, and decisions leave the
      background documents (SD-9a, SD-11);
   9. the block opens the user message, and the prompt order is fixed for
      the cache (SD-10);
   10. note, research, report and policy keep their current path, with the
       floor-only verdict path as the recommended follow-up (SD-12);
   11. one conventions document per project, revised normally (SD-13);
   12. the surfaced text is recorded at acceptance and read from the
       database (SD-14);
   13. `create_decision`, `list_decisions` and `get_decision` over MCP, with
       no new relay (SD-15);
   14. one call added to `approveDocumentAs` (SD-16).
