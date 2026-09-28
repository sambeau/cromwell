# SPEC-018: Decisions

**Status:** **Draft — for Sam's approval.** Authored by Claude. The author
can't be the approval gate, so the decision is Sam's. Sam has said they will
approve the spec and the build together. An independent review is recorded in
[REVIEW-018](../reviews/REVIEW-018-decisions.md). It found eleven material and
nine smaller problems in the first draft. All are dealt with in this revision,
and §7 says how, finding by finding. Fifteen choices need Sam's explicit yes
(DoD 9).
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
wrote and who judged; SD-8 names M11 for the untemplated types),
[SPEC-011](SPEC-011-send-to-development.md) (FR-6.6: the engine supersedes a
plan with its spec, the precedent for SD-6) and
[SPEC-012](SPEC-012-see-the-work.md) (transcripts: "what it was told").
**Follow-ups taken up:** the M8 handoff's follow-up 3 (Revise on a decision
makes a successor that can't be submitted), for decisions; SPEC-017 SD-8.
**Coordination:** M12 (bugs) runs in parallel. This spec owns migration
`0012`, and M12 owns `0013`; M11 merges first. This spec owns prompt assembly
(`planner.go`'s prompt parts, `review_prompt.go`, `content/assemble.go`). The
starter pack, `ident.DocTypes`, `mcpTools`' list and the MCP tool-set test are
shared, and merged by hand.

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every label, refusal, notice and tool description a person reads is
a full sentence (DESIGN-008 D-6).

Four words are kept apart:

- **accepted** is how a decision's `approved` state is said to a person, as
  DESIGN-010 §11 says it. In the database it is still `approved`.
- **surfaced** means pushed into a dispatched agent's prompt by Subutai.
  Nothing here is fetched by a dispatched agent.
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
   one-line reason, submit it, and accept it. A 200-word ruling is refused
   at submit with a sentence saying so.
2. A dispatch for a feature under that initiative runs on the mock provider.
   Its transcript's "what it was told" shows the ruling and reason, and the
   project conventions, and nothing from other branches of the tree.
3. They supersede the decision with a new one. The old one leaves every
   later prompt; the viewer shows each pointing at the other.
4. They amend another decision. The amendment is appended, dated, and
   accepted; the text above it is unchanged.
5. `/ui/decisions`, and `list_decisions` over MCP, show every decision with
   its state, what it supersedes and what superseded it.
6. This repository's DEC-001 to DEC-007 still adopt as they are.

## 2. What exists today

| What | Where it stands |
|---|---|
| `decision` document type, `DEC-nnn` numbers | Built in M8 (SPEC-015 SD-6, migration `0010`). A decision belongs to the project or an initiative; adopt refuses a feature. |
| Adopting DEC-001 to DEC-007 | Built in M8, as approved in the web UI, because a type **with no template** may be adopted as approved (`approvableByAdoption`). Giving decisions a template would silently end that; SD-3 keeps it. |
| Submit, review, approve | Types with a template only. A design (`approved_by: human`, no reviewer role) waits in review for a person, who approves in the UI or by `relay_verdict`. |
| Revise on a decision | Refused in the editor and on the page (SPEC-016 SD-12), pointing at supersession or amendment, neither of which exists. The HTTP API's revise makes a plain copy. |
| Anything pushed into prompts | Review dispatches receive approved documents owned by ancestor initiatives, **whole**, as background. That would push an initiative's decisions as essays (SD-11). Execution dispatches receive only the spec and plan. |
| Transcripts | Built in M6. The assembled system and user prompts are stored whole, and the run page shows them under "What it was told". |

## 3. Scope

### In scope

1. **A decision template** in the starter pack, submitted and accepted by a
   person (FR-1), and **creating a decision** in the web UI and over MCP
   (FR-2).
2. **Amendment and supersession** (FR-3), and Revise doing the right thing on
   every route.
3. **The surfaced portion**: two front-matter fields, capped at validation on
   every route to accepted (FR-4).
4. **A conventions document**: its own type and template, one per project,
   capped, accepted by a person (FR-5).
5. **Surfacing** into every dispatch, under per-dispatch caps, recorded in the
   transcript (FR-6), with the prompt order fixed for the cache (SD-10).
6. **A viewer**: `/ui/decisions`, `list_decisions` and `get_decision` (FR-7).
7. **This repository**: DEC-001 to DEC-007 adopt as they are, proved by test
   (FR-8).

### Out of scope

| What | Where it goes |
|---|---|
| Bugs, executors, spikes, retrospectives | M12, M13, M14; C-9 |
| A human verdict path for **note, research, report, policy** (SPEC-017 SD-8), and their API revise (the M8 handoff's follow-up 3 for those types) | Not built. SD-12 says why and recommends the follow-up. |
| **Partial supersession** ("supersedes DEC-004 in part", as DEC-005 and DEC-007 record in their headers) | Not modelled (R18-16). A decision that still stands in part records a ruling (FR-3.2) saying what still stands. DEC-001 to DEC-007 adopt with no `supersedes`. |
| **Dated notes** inside an accepted decision, and status lines above it, as this repository has written them | Not supported (SD-5). A note is written as an amendment. |
| Surfacing decisions into the chat agent's context | Nothing is pushed into chat; the chat agent reads with `list_decisions`. The chat prompt isn't Subutai's to assemble (context research §3, "an unresolved edge"). |
| An agent reviewer for decisions in the starter pack | Not shipped (SD-2). A project may add one. |
| Amending over MCP | The web UI only (SD-6). The chat agent can draft a superseding decision. |
| An integrity check on a superseded decision's file | Not added. Surfacing never reads it (SD-14), and its row stays a record (SD-6). |
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
  ruling in brief. A title that already starts with its own ID, as this
  repository's do ("DEC-001: Server Language — Go"), loses that prefix, so the
  line reads `DEC-001: Server Language — Go`, not the ID twice (R18-12). The
  viewer marks such a decision "No ruling is recorded; its title is
  surfaced", and **Record its ruling** (SD-5) adds one.
- The alternative, a fixed `## Ruling` section, reads better in a plain
  Markdown viewer. It loses the three properties above.

**SD-2 — A person accepts a decision. No agent reviewer comments first.**
(Choice 2.)

- The manifest is `approved_by: human` with no `reviewer_role`. A submitted
  decision waits in review for a person, exactly as a design does, and is
  accepted in the web UI or relayed from chat with `relay_verdict`. DEC-006
  Amendment 1, decision 8 already lets the chat agent relay "a human's
  verdict on any document", so no new relay is needed.
- **Why no reviewer:** designs had one and it was retired in M3 (DESIGN-010
  §17a item 1). A decision is a person's ruling, usually made in the
  conversation that led to it. The mechanical parts a reviewer might catch
  (length, a missing reason, superseding the wrong thing) are checked at
  validation instead (FR-4, FR-3.6).
- **A project that wants one** sets `reviewer_role` in its
  `templates/decision/manifest.yaml`. With `approved_by: human` that gives a
  comments-only review through the existing machinery (SPEC-009 FR-2.2). The
  reviewer's validation report comes from the same function Submit uses
  (FR-3.3), so an amendment isn't reported against the new-decision template.

**SD-3 — Decisions and conventions stay adoptable as approved, and decisions
gain "This was already approved".** (Choice 3.)

- SPEC-015 SD-11 allows approved adoption for designs and types with no
  template. Decisions now have a template, so the rule becomes **design,
  decision, conventions, or a type with no template**.
- *This was already approved* (SPEC-015 FR-5.8), offered on an adopted draft
  of an untemplated type, is also offered on an adopted decision draft. An
  adopted decision won't have the template's headings, so it could never pass
  Submit.
- **One narrow change to SPEC-015 SD-11**, which says nothing is validated on
  an approved adoption: both routes now check the surfaced fields' caps
  (FR-4.3), and nothing else. Otherwise a 500-word ruling could reach every
  prompt by adoption. It needs a dated note on SD-11 (DoD 8).

**SD-4 — Where a new decision lives.** A project decision is written to
`docs/decisions/DEC-nnn-<slug>.md`, beside this repository's own. An
initiative's goes in the initiative's default home,
`docs/work/<INIT-ID>-<slug>/DEC-nnn-<slug>.md`, as DESIGN-010 §7 says of
documents Subutai creates. The slug comes from the title. Neither is a rule:
the file is found by its ID wherever it moves (SPEC-015 SD-10). (Choice 4.)

**SD-5 — An amendment is a new revision that only appends.** (Choice 5.)

- **Append an amendment**, on an accepted decision's page, opens a revision
  exactly as Revise does (same `DEC-nnn`, next revision, a working copy at
  `….rev.md`), with a skeleton appended:
  `## Amendment <n> — {{what changed}} (<today>)` and a placeholder
  paragraph. `n` is one more than the number of `## Amendment` headings
  already in the file. It opens in the browser editor.
- **Submitting it checks that it only appends** (FR-3.3): the accepted text is
  kept, and only a dated amendment follows it. Front matter may change only in
  `ruling:` and `reason:` (and the identity lines), so an amendment that
  changes the ruling restates it for the prompt.
- **Record its ruling** is the one case with nothing appended: a decision
  with no `ruling:` may gain `ruling:` and `reason:` in a revision whose body
  is unchanged. Recording what a decision already says changes nothing it
  says. The same append-only check applies, so nothing else can change. This
  is how DEC-001 to DEC-007 get proper surfaced text.
- **A person accepts it**, as any decision. Acceptance supersedes the
  previous revision, archiving it as `docs/_superseded/DEC-006.r1.md`
  (SPEC-015 SD-9), exactly as any revision is.
- **Why a revision, not an edit in place:** DESIGN-003 L-2 keeps approved
  documents immutable, and DESIGN-010 §11 says an accepted decision is never
  edited. A revision keeps both true: the accepted text is never changed, and
  the amendment is itself accepted by a person before it counts.
- **This is stricter than the repository's past practice** (R18-3). DEC-006's
  Amendment 1, which DESIGN-010 §11 names, also changed the status block at
  the top and marked decisions 5, 6 and 8 "*(Replaced by Amendment 1.)*" in
  place, and later dated notes were inserted inside the amendment. Subutai
  allows none of that, because "the accepted text is kept" has to be
  checkable. A reader sees what changed in three places instead: the
  amendment itself, the restated `ruling:` that agents are told, and the
  viewer, which lists a decision's amendments by heading and date. A dated
  note is written as an amendment.

**SD-6 — Supersession is a new decision naming the old in `supersedes:`.**
(Choice 6.)

- **Supersede with a new decision**, on an accepted decision's page, creates
  a new decision draft (FR-2) with the same owner and `supersedes: [DEC-005]`
  already filled in, and opens it in the editor. The chat agent can do the
  same with `create_decision` and `supersedes`.
- **Accepting the new decision supersedes the old one** in the same
  transaction: its state becomes `superseded` and the link is recorded.
- **The old file stays where it is and is not changed.** Other documents cite
  decisions by path (this repository's specs link to `docs/decisions/…`), and
  a superseded decision is a record, not a draft replaced by a newer copy of
  itself. Only an amendment's older revisions are archived, because a newer
  copy of the same text replaces them.
- **Because its file stays, its ID is closed** (R18-1). A decision ID whose
  every revision is superseded can't be adopted again, by its declared `id:`
  or by its file name: "DEC-005 was superseded by DEC-008 and is kept as a
  record, so docs/decisions/DEC-005-….md can't be adopted as DEC-005 again.
  Start a new decision instead."
- **Which decisions a decision may supersede** (checked at submit and again at
  acceptance, FR-3.6): accepted ones whose owner is the new decision's owner
  or lies below it in the tree. A project decision may supersede an
  initiative's; an initiative's decision may not retire a project-wide one,
  because that would change the rules for every other branch of the tree.
- **Two departures from DESIGN-003, named** (R18-2):
  - §2 says `supersede` is fired only by a successor's approval. A superseding
    decision isn't a successor: it has its own number. The engine fires it,
    never a person, in the new decision's acceptance, as it already does for
    a plan superseded with its spec (SPEC-011 FR-6.6).
  - §5 archives a superseded document's file. A superseded decision's file
    stays in place, for the reason above.

  Both need dated notes (DoD 8).
- Amending from chat is left out (Out of scope): the amendment is written in
  the browser editor, and appending is a small act a person does there.

**SD-7 — Revise on a decision does the right thing, on every route.**

- The page's Revise button is replaced, on an accepted decision, by **Append
  an amendment** (or **Record its ruling**, when it has no `ruling:`) and
  **Supersede with a new decision**. The editor's refusal (SPEC-016 SD-12)
  names both.
- The decision branch sits in `reviseDocBy`, the core every route shares: the
  page, the editor, the HTTP API's revise, and the CLI's `doc revise`. A
  decision with a ruling gets the amendment skeleton; one without gets the
  ruling fields to fill in. So no route makes a successor that can't be
  submitted (R18-9). This closes the M8 handoff's follow-up 3 for decisions;
  it stays open for the four untemplated types (SD-12).
- `reviseDocBy` also refuses to write a working copy where one already exists,
  so a second, concurrent revision can never delete the first one's file
  (R18-18).
- Raising an issue on an accepted decision says what to do: "This decision is
  accepted, and an accepted decision is never edited. Append a dated
  amendment, or supersede it with a new decision, from its page."

**SD-8 — The caps, tuned against real prompt sizes and the research.**
(Choice 7.)

The measured numbers:

- The role system prompts in the starter pack are 600 to 1,500 tokens.
- The first turn of a real spec or plan review is 1,700 to 2,000 input
  tokens (walkthrough, phase 2 and the first live run). An implement dispatch
  runs to about 14,000 tokens over all its turns, and every turn resends the
  prompt.
- The context research says compliance falls away past about **nineteen
  requirements in the whole pushed block**, and asks for "five to fifteen
  things that actually change what the agent writes" (R18-10).

At DESIGN-010 §17b's proposal of 150 words per decision, 1,500 tokens holds
about five decisions beside the conventions, which is too few to be useful and
spends the budget on length rather than on distinct rules. So the defaults cap
the size of each decision, count decisions as well as tokens, and keep the
per-dispatch token cap:

| Cap | Default | Where it is enforced |
|---|---|---|
| A decision's `ruling:` | 75 words | Validation (the decision manifest) |
| A decision's `reason:` | 25 words, on one line | Validation |
| The conventions document's body | 300 words, and at most half the dispatch's token cap | Validation (the conventions manifest, and FR-5.5) |
| Decisions in one dispatch | 10 | At assembly (FR-6.4) |
| One dispatch's surfaced block | 1,500 tokens, estimated | At assembly (FR-6.4) |

- **The count cap is what keeps the block near the research's line.** A
  conventions document of five to ten points and ten decisions is fifteen to
  twenty things, the research's range.
- **The token cap bounds the cost.** Ten decisions at the 100-word maximum and
  a full conventions document could come to about 2,000 tokens; typical
  decisions of about 50 words bring ten of them and the conventions to about
  1,100. The token cap is the backstop for long ones.
- **Tokens are estimated** as the block's characters (Unicode code points)
  divided by 3.5, rounded up. Subutai has no tokeniser. The figure is nominal:
  current tokenisers give three and a half to four characters of English per
  token, and fewer for identifiers and paths, so it can err low on text full
  of code names.
- The word caps live in the manifests as rules (FR-1.2), so a project can
  change them by editing its own templates. The dispatch caps are
  configuration, `surfacing.max_tokens` and `surfacing.max_decisions` in
  `config.yaml` (FR-6.5).
- **What it costs.** 1,500 tokens is 75 to 90 per cent more than a review's
  first turn today, and about 10 per cent of an implement dispatch. For
  dispatches of the same role on the same branch it is part of a shared
  prefix (SD-10), so a provider that caches prefixes automatically, as
  DeepSeek does, bills it at the cache rate after the first. Anthropic caches
  nothing until Subutai sets breakpoints (Out of scope).

**SD-9 — What happens when a cap is hit.** (Choice 8.)

- **The conventions document always goes in.** It is held at validation to
  half the dispatch's token cap (FR-5.5), so there is always room for
  decisions.
- **Decisions are admitted nearest first:** those of the dispatch's own
  initiative, then its parent's, outwards, then the project's. Within one
  owner, **most recently accepted first**, by the ID's *first* acceptance, so
  amending an old decision doesn't promote it; ties go to the higher number
  (R18-13). The first decision that doesn't fit, or the eleventh, stops
  admission for its owner and every owner further out, so what is left out is
  always the far end of the tree, oldest first within it.
- **Why nearest first:** the nearer decision is more specific, and it is the
  one an agent is least likely to infer from the code. Project-wide decisions
  ("the server is in Go") are usually visible in the codebase already.
- **What is left out is said, in three places:**
  1. **In the prompt**, as the block's last line: "Left out for space:
     DEC-002 (State Store — Supabase-hosted Postgres, plain-Postgres
     compatible), …." It names at most ten, then "and 4 more", so the line
     can't outgrow the cap. It invites no fetching (R18-15).
  2. **In the transcript**, because it is part of the prompt.
  3. **In the viewer**: a decision left out on any branch of the tree says
     "This decision is left out of some dispatches, for lack of space", and a
     banner names each such decision and the branches where it happens
     (FR-7.2).
- **What is never done:** truncating a ruling mid-sentence. A ruling is
  whole or absent.

**SD-9a — Which dispatches receive it: every one.** Implement, code review,
verify, estimate, spec and plan writing, and document review all receive the
block. (Choice 9.)

- **Verification is a fair question**, because it is meant to have clean
  context (DESIGN-003 L-4). The block isn't the implementation's story,
  though. It is project law, and a verifier judging "does this meet the
  contract" is better for knowing, say, that the project forbids a
  dependency.
- **A dispatch whose subject has no initiative** — a review of a
  project-level document — receives the project's decisions and conventions
  only.
- **A review of a decision** never receives that decision in the block, so a
  decision under amendment isn't told to its own reviewer as settled.

**SD-10 — Where the block goes, and fixing the prompt order.** (Choice 10.)

- The block comes **straight after the project's name**, at the head of the
  user message, under `# Project decisions and conventions`. Its order is
  fixed: the conventions, then the project's decisions, then each
  initiative's from the outermost in, and within one owner by number
  ascending.
- **Why there.** The system prompt stays role-only and byte-stable for every
  dispatch of a role (research §6, "what's already right"). The tools and the
  system prompt come first in a request, so the block is a shared prefix only
  among dispatches **of the same role** on the same branch (R18-15). Within
  that, rendering widest-shared first means dispatches of one role on two
  sibling initiatives share the prefix up to their first difference, and a new
  decision appends at the end of its owner's run rather than shifting
  everything before it.
- **The defect this fixes.** Implement and code-review prompts put the task
  first, ahead of the spec and plan, so the shared contract was re-billed on
  every task. Spec and plan writing put the feature ahead of its
  initiative's design. The prefix-cache research's recommended order is
  adopted:

  | Purpose | New order of the user message |
  |---|---|
  | implement-task | project, block, spec, dev-plan, how to work, **task** |
  | review-code | project, block, spec, dev-plan, **task**, diff, instruction |
  | verify-feature | project, block, spec, feature, instruction |
  | estimate | project, block, work, reference points, instruction |
  | write-spec, write-dev-plan | project, block, initiative's design, feature's design, (approved spec), structure, **feature**, revision context, instruction |
  | review-&lt;type&gt; | project, block, background documents, feature, task, validation, comments, issues, document |

  The task moves into the recency slot, which is what the attention research
  asks for too, so the change costs nothing on either axis. Implement, code
  review, verify and estimate prompts gain the `# Project` heading they
  lacked, which every purpose now shares.
- The block is recorded nowhere but the prompt. The transcript already
  stores the prompt whole, so "what it was told" shows it with no new
  plumbing.

**SD-11 — Decisions and conventions stop arriving as whole background
documents.** Review dispatches today receive every approved document owned by
an ancestor initiative, whole. An initiative's decision would then arrive
twice: as an essay in the background and as a ruling in the block. Decisions
and conventions are excluded from the background documents. (Part of
choice 9.)

**SD-12 — The other four untemplated types keep their current path.**
SPEC-017 SD-8 named M11 to decide "what reviewing a decision means, and the
same answer serves the others". The answer for decisions is a template with a
person's verdict. For a note, research, report or policy the same answer
would mean four templates nobody has asked for. So:

- they stay as they are: adoptable as approved, *This was already approved*
  in the web UI, and not revisable in the editor (SPEC-016 SD-12). The HTTP
  API's revise still makes successors of them that can't be submitted: the M8
  handoff's follow-up 3 stays open for these four types;
- **recommended follow-up:** a *floor-only* verdict path for untemplated
  types (submit checks only the non-configurable floor: parse, placeholders,
  links; a person decides), which would also let `relay_verdict` carry
  "already approved" from chat.

(Choice 11.)

**SD-13 — The conventions document.** One per project. (Choice 12.)

- Type `conventions`, owner **the project only**, ID `PROJECT-conventions`,
  default path `docs/conventions.md`.
- **One live conventions document per project**, refused on every route that
  would make a second: attach, adopt and *Start the conventions document*. A
  file already at `docs/conventions.md` is never written over; the sentence
  says to adopt it instead (R18-17).
- **The whole body is surfaced**, below its front matter and without its
  title heading, with its own headings moved one level down so they sit under
  the block's `## Conventions`. The template carries no note about itself,
  because anything in the body is pushed. It suggests headings for code,
  architecture, key interfaces and tools.
- Accepted by a person, like a decision. Unlike a decision, it is revised
  normally (Revise, edit, submit, accept): conventions are expected to change,
  and nothing cites a version of them.

**SD-14 — What is stored.** Migration `0012` adds one type value and two
tables. (Choice 13.)

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

**SD-15 — Creating a decision over MCP is planning authoring.** (Choice 14.)
`create_decision` writes a draft from the template and registers it, which
DEC-004 allows ("attach documents to entities, and author document
content"). The chat agent then fills the file in and hands it in with
`submit_for_review` (SPEC-017), and a person accepts it on the page or by a
relayed verdict. `list_decisions` and `get_decision` are reads. No new relay
tool is added.

**SD-16 — Acceptance re-checks, and is the one change in shared review
code.** (Choice 15.)

- Accepting a decision has to record its surfaced text and supersede what it
  names in the approval's own transaction. `approveDocumentAs` in
  `internal/server/actions.go` gains one call, `s.onAccepted(ctx, tx, doc)`,
  beside its predecessor handling. Nothing else there changes. (M12 owns the
  review-outcome handling; the handoff names the line.) Adoption as approved
  and *This was already approved* call the same code.
- **What acceptance checks** (R18-6, R18-7, R18-8), each refused with a
  sentence:
  1. a decision has a number;
  2. the file's hash is still the one submitted, so nobody's later edit is
     accepted unseen;
  3. the document still passes the same validation Submit ran (FR-3.3 for an
     amendment), including the caps;
  4. an amendment's decision is still accepted, re-read under a lock;
  5. each decision it supersedes is still accepted, re-read under a lock.

  A transaction-scoped lock on each decision ID involved serialises an
  amendment's acceptance against a supersession of the same decision, so
  they can't both commit.
- The adoption routes check only a number and the caps (SD-3), because they
  record a verdict given outside Subutai.

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
are listed in DESIGN-004's registry note (DoD 8):

- `max_words` with `max:` and exactly one of `field:` (a front-matter field),
  `section:` (a section's content) or `body: true` (everything below the
  front matter, less a leading level-1 heading). Loading a manifest refuses
  any other shape. It fails with, for example: "The ruling is 112 words; it
  can be at most 75, because it is pushed into agents' prompts. Keep the rule
  itself here, and put the argument in Context."
- `one_line` with `field:`: the field's **parsed** value holds no line break.
  A YAML literal block (`|`) keeps its breaks and fails; a folded or plain
  scalar is already one line and passes (R18-5).

Words are runs of non-space characters.

**FR-1.3** A decision is submitted, validated and put in review like any
templated document. With no reviewer role it waits in review, and the page
says "A person decides whether to accept this decision." Accept and Ask for
changes are offered, as for a design. The chat agent relays either with
`relay_verdict` and the person's words.

**FR-1.4** Pages say *accepted* for a decision's approved state: the state
badge, the viewer, and the decision panel (FR-4.2).

### FR-2: Creating a decision

**FR-2.1** `/ui/decisions` has **New decision…**, a dialog with a title, an
owner (the project, or an initiative chosen from a list), and optionally the
decisions it supersedes. It:

1. mints the next `DEC-nnn`;
2. writes the template to the default home (SD-4), with `title`, `owner` and
   `supersedes` filled in, and `id:` and `revision: 1`;
3. registers the draft, records the writer, and commits the file as the
   tool's git author;
4. opens it in the browser editor.

A `supersedes` it can't take is refused before anything is written (FR-3.6).

**FR-2.2** A file that already exists at the path is never written over: the
dialog refuses with a sentence naming the file.

**FR-2.3** `create_decision` over MCP takes `title`, `owner` (`project`, or an
initiative's path or ID) and optional `supersedes` (a list of `DEC-` IDs).
It does FR-2.1 steps 1 to 3 and returns the document entry and a sentence:
"DEC-008 is a draft at docs/decisions/DEC-008-…md. Fill in its ruling,
reason, context and alternatives, then hand it in with submit_for_review; a
person accepts it." Its writer is the chat agent. `submit_for_review`'s
description names decisions and the conventions.

### FR-3: Amendment and supersession

**FR-3.1 — Append an amendment** (SD-5), offered on an accepted decision's
page when no revision of it is open. It makes the successor through
`reviseDocBy`, appends the skeleton, and opens the editor.

**FR-3.2 — Record its ruling**, offered instead of Append when the accepted
decision has no `ruling:`. It makes the successor with
`ruling: "{{…}}"` and `reason: "{{…}}"` added at the end of the front matter,
in the file's own line endings, and the body unchanged.

**FR-3.3 — Only appending passes.** A decision that revises a predecessor is
validated by one function, used by Validate, Submit, the reviewer's report and
acceptance alike (R18-5). It **skips** the manifest's `front_matter.required`
and `sections`, because an adopted decision has neither, and applies:

1. **The accepted text is kept.** The predecessor's file is read, and its hash
   is checked against the one recorded. Both files are parsed, which removes a
   byte-order mark and turns CRLF into LF; the successor's body must begin
   with the predecessor's body exactly, trailing blank lines and spaces
   aside.
2. **Only a dated amendment follows**: text whose first line is a level-2
   heading beginning `Amendment` and ending in a date in brackets,
   `(2026-09-30)`. The one exception is Record its ruling (SD-5): nothing
   follows, the predecessor had no `ruling:`, and the successor has one.
3. **Front matter**, compared as parsed maps, differs only in `ruling:`,
   `reason:`, `id:` and `revision:`. `title`, `type`, `owner` and
   `supersedes` can't change, whether present or absent.
4. **The floor checks apply to what is new** (R18-4): placeholders and links
   in the appended text, and placeholders in `ruling:` and `reason:`. The
   accepted text was checked when it was accepted, and an adopted decision's
   old links may no longer resolve (DEC-003 and DEC-005 each have one that
   doesn't), so re-checking it would make it impossible to amend. This is a
   narrow, named departure from DESIGN-004 F-6's floor.
5. **The surfaced-field rules** (FR-1.2) apply to `ruling:` and `reason:`,
   and a ruling needs a reason.

Each failure is a sentence, for example: "An accepted decision is never
edited: the text of DEC-006 revision 1 must be kept exactly, and an amendment
added after it. The first difference is on line 42."

**FR-3.4 — Supersede with a new decision**, offered on an accepted
decision's page. It is FR-2.1 with the owner and `supersedes` filled in.

**FR-3.5 — Accepting** (SD-16), in the acceptance's transaction, after the
checks SD-16 lists:

1. `surfaced_texts` records the accepted revision's ruling and reason, or its
   title if it has none (FR-6.2). The title is taken from the file's first
   heading when the row's title is still its path (R18-7).
2. For each ID in `supersedes:`, that decision's accepted revision, locked, is
   moved to `superseded` by the lifecycle engine, and `decision_supersessions`
   records the pair. Its file is not moved or changed. If it is no longer
   accepted, acceptance is refused: "DEC-005 was already superseded by
   DEC-009, so accepting this would supersede nothing. Take DEC-005 out of
   supersedes, and submit it again."
3. An open amendment of a decision superseded this way goes back to draft if
   it was in review. Submitting or accepting it is then refused: "DEC-005 was
   superseded by DEC-008, so it can't be amended. Detach this draft; its
   working copy can then be deleted."

**FR-3.6 — Supersession checks**, at submit and again at acceptance. Each ID
in `supersedes:` must name a decision that is accepted now, owned by the same
owner as the new decision or by one below it. Otherwise: "DEC-005 isn't an
accepted decision, so this can't supersede it", or "DEC-003 belongs to the
project, and a decision of INIT-004 Auth can't supersede it: that would change
the rules for everything under the project. …"

**FR-3.7 — The editor and the page agree.** On an accepted decision the
editor's refusal reads: "An accepted decision is never edited. Append a dated
amendment, or supersede it with a new decision, from its page." The page
offers exactly FR-3.1 or FR-3.2, and FR-3.4.

**FR-3.8 — A superseded decision is closed** (SD-6, R18-1). Adopt refuses a
file whose declared `id:`, or whose name, claims a decision ID whose every
revision is superseded.

**FR-3.9 — A decision without a number** (attached, not adopted) fails
validation and can't be accepted: "This decision has no number yet. Give it
its number with Give it an ID, then submit it." (R18-8.)

### FR-4: The surfaced portion is short by construction

**FR-4.1** The decision manifest's rules (FR-1.1) cap `ruling:` at 75 words
and `reason:` at 25 words on one line. Validation at submit enforces them, and
acceptance re-checks them (SD-16), so an over-long ruling can't be accepted by
the normal route, even if the file is edited during review.

**FR-4.2** A decision's page has a panel, *What agents are told*, showing the
surfaced line exactly as a prompt carries it and its size in words against
the caps. For a superseded decision it says which decision superseded it. A
conventions document's page shows its size against its cap.

**FR-4.3** The routes to accepted that skip Submit check the same caps:
adopting a decision or the conventions as approved, and *This was already
approved*. They read the caps from the project's own manifests. A decision
with no `ruling:` passes, and is surfaced by its title. Refusal: "It can't be
recorded as accepted: The ruling is 112 words; it can be at most 75, … Adopt
it as a draft and shorten it first."

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
`type: conventions`, `owner: project`. Its body is a heading and suggested
sections (Code, Architecture, Key interfaces, Tools), each a short list of
placeholders.

**FR-5.2** One live conventions document per project (SD-13). Attach, adopt
and *Start the conventions document* refuse a second: "This project already
has its conventions document, at docs/conventions.md. Revise that one
instead." Each refuses any owner but the project.

**FR-5.3** Adopting a conventions document as approved is allowed in the web
UI (SD-3), and checks its size (FR-4.3).

**FR-5.4** `/ui/decisions` shows the conventions document at the top, with
its state and a link; or, when there is no live one, *Start the conventions
document*, which writes the template to `docs/conventions.md`, registers and
commits it, and opens the editor. An existing file there is refused, never
written over.

**FR-5.5** Validating the conventions also checks their estimated tokens
against half of `surfacing.max_tokens`: "The conventions come to about 820
tokens, and they can use at most 750, half of a dispatch's cap of 1500, so
that decisions still fit. Shorten them." (R18-11.)

### FR-6: Surfacing

**FR-6.1 — The branch.** A dispatch's subject is a task, a feature, or a
document. Its **branch** is the project, and the initiative its subject
belongs to (a task's or a feature's initiative, a feature-owned document's
feature's initiative, or an initiative-owned document's initiative) **with
that initiative's ancestors**, never its descendants or siblings (R18-13).
The block holds:

1. the project's accepted conventions, if any;
2. every accepted decision owned by the project or an initiative on the
   branch, except a decision under review (SD-9a).

Superseded decisions are never included: they are not accepted. A decision
with an open amendment is included as accepted, until the amendment is.

**FR-6.2 — The line for a decision.** `- DEC-008 (INIT-004 Auth): <ruling>
Why: <reason>`, with the owner in brackets only for an initiative's decision.
With no recorded ruling, the title less any leading copy of its own ID:
`- DEC-001: Server Language — Go`.

**FR-6.3 — The block.**

```
# Project decisions and conventions

These are binding: people decided them. If the task, or a document you are
given, contradicts one, follow the decision and say so in your outcome; don't
work around it.

## Conventions

<the conventions body, its headings one level down>

## Decisions

- DEC-001: Server Language — Go
- …

Left out for space: DEC-002 (State Store — …), and 4 more.
```

With no conventions and no decisions the block is absent entirely.

**FR-6.4 — The caps** (SD-8, SD-9). Admission follows SD-9's order. A
decision is admitted if the whole block, rendered with it and with the
left-out line for everything not yet admitted, stays within
`surfacing.max_tokens`, and fewer than `surfacing.max_decisions` are already
admitted. The first that fails ends admission. The left-out line names at
most ten. Rendering order is SD-10's. The same inputs always produce the same
bytes (NFR-1).

**FR-6.5 — Configuration.**

```yaml
surfacing:
  max_tokens: 1500     # default
  max_decisions: 10    # default
```

Absent, or `0`, means the default: YAML can't tell them apart (R18-11). A
`max_tokens` below 600 is refused at load: "surfacing.max_tokens: 400 is too
small to hold the conventions and any decisions; use at least 600, or leave it
out for the default of 1500." A negative `max_decisions` is refused.

**FR-6.6 — Every purpose** builds the block with one function and places it
per SD-10. `content.AssembleReviewPrompt` gains a `Surfaced` input placed
after the project's name.

**FR-6.7 — Background documents** in review prompts exclude `decision` and
`conventions` (SD-11).

### FR-7: The viewer

**FR-7.1 — `/ui/decisions`**, linked from the navigation beside Documents. A
table, one row per decision ID, showing its **accepted revision** while one
exists, so a decision with an open amendment still shows as accepted, marked
"accepted, with an amendment draft" (R18-14). Each row has:

- the ID and title, linking to the document page, and under them the
  amendments by heading and date, read from the file's `## Amendment`
  headings (not from the revision number);
- its owner (the project, or the initiative's ID and name);
- its state (draft, in review, accepted, superseded);
- the surfaced line, or "No ruling is recorded; its title is surfaced";
- **Supersedes** and **Superseded by**, each a list of linked IDs.

Filters, as links that keep each other: state (accepted by default; drafts and
in review; superseded; all), and owner (everything; the project; or one
initiative, meaning its branch: what its dispatches are told).

**FR-7.2** With an owner chosen, the page shows the block a dispatch there
receives, and its estimated size against the cap. A banner names every
decision the caps leave out on any branch, and where; each such row says
"This decision is left out of some dispatches, for lack of space."

**FR-7.3 — `list_decisions`** over MCP: optional `owner` (`project`, or an
initiative's path or ID, meaning its branch) and `state` (`accepted` by
default, `open`, `superseded`, `all`). Each entry: `id`, `title`, `owner`,
`state`, `ruling`, `reason`, `told_as` (the surfaced line), `supersedes`,
`superseded_by`, `amendments`, `revision`, `path`. With an owner, the result
also gives `left_out`, `told_tokens` and `max_tokens`; and `conventions` (ID,
path, state) whenever there is one.

**FR-7.4 — `get_decision`**: `id` (a `DEC-` ID). The entry above, plus the
file's text, and `written_by` and `last_verdict` as other document results
have them.

**FR-7.5** All three MCP tools are named in the tool-set test, and
`accept_decision`, `amend_decision` and `supersede_decision` are named among
the tools that must not exist.

### FR-8: This repository

**FR-8.1** An integration test copies DEC-001 to DEC-007 into a test project,
adopts each as approved through the web UI's route, and checks:

- each keeps its number, and its bytes below the identity lines;
- each is accepted, with its title as its surfaced text, and none records a
  supersession (R18-16);
- a dispatch on that project receives all seven titles, each once, in number
  order;
- `create_decision` then mints DEC-008.

**FR-8.2** Record its ruling on DEC-001 (FR-3.2) passes the append-only check
with the body unchanged, and after acceptance a dispatch receives the recorded
ruling instead of the title.

## 5. Non-functional requirements

- **NFR-1 — Deterministic.** The block is a pure function of the accepted
  rows and the caps. The unit tests render it from fixed inputs and compare
  bytes.
- **NFR-2 — Cheap.** Building the block is a query for the branch's accepted
  decisions and one for the conventions; no file is read at dispatch time.
- **NFR-3 — Unchanged when unused.** A project with no accepted decisions and
  no conventions sends prompts that differ from today's only in SD-10's
  reordering and the `# Project` heading it adds.
- **NFR-4 — Words for people.** Every refusal is a sentence that says what to
  do (DESIGN-008 D-6).
- **NFR-5 — No knowledge base.** Nothing here offers a *dispatched* agent a
  way to query decisions, and the block invites no fetching. What they need
  is pushed (evidence base §4.1). The chat agent, which is not dispatched,
  reads them with `list_decisions` and `get_decision`.

## 6. Open questions carried forward

- **A floor-only verdict path** for note, research, report and policy (SD-12).
- **Partial supersession** (Out of scope).
- **Per-role surfacing.** Every role gets the same block. If some decisions
  only matter to, say, the implementer, a `roles:` field could narrow them.
  Not proposed until a real project asks.
- **Measuring the cap in use.** The dispatch rows already record cache reads;
  a read of block size against `cache_read_tokens` after a real run would
  confirm SD-10's claim, as research §7 item 1 asks.
- **Amending from chat.** Left out (SD-6); easy to add if wanted.

## 7. Changes after review

| Finding | What changed |
|---|---|
| R18-1 (material) | SD-6 and FR-3.8: a decision ID whose every revision is superseded is closed; adopt refuses it by declared `id:` or file name. Attach can't bring it back either: an attached decision has no number, so it can't be submitted (FR-3.9). The integrity scan isn't extended (Out of scope): surfacing never reads the file. |
| R18-2 (material) | SD-6 names both departures from DESIGN-003 (§2 and §5) with the SPEC-011 FR-6.6 precedent; SD-3 names the change to SPEC-015 SD-11 and is now a choice. DoD 8 lists the dated notes, DESIGN-004's registry note and DESIGN-010 §17b. |
| R18-3 (material) | SD-5 says plainly that Subutai's form is stricter than DEC-006's, where the change shows instead, and that a dated note is written as an amendment. |
| R18-4 (material) | FR-3.3 item 4: floor checks apply only to what is new, a named departure from DESIGN-004 F-6. FR-8.2 names DEC-001. |
| R18-5 (material) | FR-3.3 defines the one function every call site uses, what it skips and keeps, normalised body comparison, and front matter as parsed maps. FR-1.2 says `one_line` reads the parsed value. SD-2 notes the reviewer's report uses it too. |
| R18-6 (material) | SD-16 and FR-3.5: per-ID locks, re-reads under lock, refusal when a target or an amendment's decision is no longer accepted, and an amendment in review withdrawn to draft. |
| R18-7 (material) | SD-16: acceptance checks the hash and re-runs validation, including the caps. FR-3.5 item 1: the title comes from the file's heading when the row's is still its path. |
| R18-8 (material) | FR-3.9: a decision without a number fails validation and can't be accepted. |
| R18-9 (material) | SD-7: the decision branch is in `reviseDocBy`, which every route shares, including the HTTP API's; a decision without a ruling gets Record its ruling; follow-up 3 stays open for the four untemplated types (SD-12); raising an issue on an accepted decision says what to do. |
| R18-10 (material) | SD-8 restated against about nineteen requirements for the whole block: a count cap of ten decisions joins the token cap. The estimate is characters over 3.5, stated as nominal, and "characters" means code points. |
| R18-11 (material) | FR-5.5 holds the conventions to half the token cap at validation, so they always fit with room for decisions. The left-out line names at most ten. FR-6.4 gives the admission rule with the line included. `0` means the default. |
| R18-12 (minor) | SD-1 and FR-6.2: a leading copy of the ID is dropped from a surfaced title; the examples use real titles. |
| R18-13 (minor) | SD-9: ordered by the ID's first acceptance, ties to the higher number. FR-6.1 defines the branch. |
| R18-14 (minor) | FR-7.1: the accepted revision is shown while an amendment is open, and amendments are read from headings. |
| R18-15 (minor) | SD-8 and SD-10: cache claims are per role and conditional on the provider. NFR-3 names the added heading. The left-out line invites no fetching. |
| R18-16 (minor) | Out of scope: partial supersession, expressed by recording a ruling; FR-8.1 checks the seven adopt with no supersession. |
| R18-17 (minor) | SD-13 and FR-5.2 to FR-5.5: refusals on every route, "none" means no live document, an existing file is refused, the template has no note about itself, and headings move one level down. |
| R18-18 (minor) | SD-7: `reviseDocBy` refuses to write where a working copy already exists, so it never removes another revision's file. |
| R18-19 (minor) | The Definition of done now names Playwright and screenshots, the amendment, the refused ruling and DEC-001 to DEC-007 in the demo, and the missing tests and steps. |
| R18-20 (minor) | The fragment, "Detach this draft", NFR-5, FR-6.5's sentence and the block's preamble are reworded. |

## Definition of done

1. `go vet ./...` is clean, and `go test -race -count=1 ./...` passes with
   the integration tests run, not skipped.
2. **Unit tests** prove the block byte for byte (order, caps, left-out line,
   demoted headings, the ID prefix dropped), the two rule kinds, the
   amendment check's matrix (kept text, CRLF and BOM, appended heading, front
   matter, Record its ruling, links only in the new part), and the
   configuration refusal.
3. **Integration tests with the mock provider** prove:
   1. a dispatch's transcript contains exactly the right decisions and
      conventions for its branch, and none from a sibling branch;
   2. a superseded decision is left out, its file untouched, and it can't be
      adopted again;
   3. the caps are enforced, admitting nearest first and naming what they
      left out;
   4. validation refuses an over-long ruling, a multi-line reason and
      over-long conventions, at submit and on the routes that skip it, and
      acceptance refuses a file edited after submission;
   5. an amendment that only appends is accepted, and one that changes the
      accepted text is refused;
   6. supersession refuses a narrower owner, and an open amendment of a
      superseded decision can't then be accepted;
   7. a hand edit to an accepted decision never reaches a prompt;
   8. the start-up backfill records adopted decisions;
   9. one conventions document per project;
   10. the new prompt orders (SD-10).
4. DEC-001 to DEC-007 adopt as they are, and Record its ruling works on one
   (FR-8).
5. The viewer and all three MCP tools work, and the tool-set test names them.
6. **A demo with no AI provider**, driven with Playwright and the
   pre-installed Chromium, with screenshots: adopt DEC-001 to DEC-007 and see
   them in the viewer; create a decision, see a 200-word ruling refused, fix
   it, accept it; supersede it; amend another; and show a mock dispatch's
   transcript containing the ruling. Recorded in
   `docs/walkthrough-spec-018.md`.
7. A handoff note, and the roadmap's §11 marking M11 done with a pointer to
   it.
8. **On approval, dated notes** (worded in the handoff) on:
   - DESIGN-003 §2 and §5 (SD-6's two departures);
   - SPEC-015 SD-11 (SD-3's caps on approved adoption);
   - DESIGN-004 §7 (the `max_words` and `one_line` rule kinds) and F-6 (FR-3.3
     item 4);
   - DESIGN-010 §17b ("settled in SPEC-018 SD-8") and §11 (SD-5's stricter
     amendment).
9. **Sam's yes to fifteen choices:**
   1. the surfaced portion is `ruling:` and `reason:` front matter, and a
      decision without them surfaces by title (SD-1);
   2. a person accepts a decision, with no agent reviewer in the pack
      (SD-2);
   3. decisions and conventions stay adoptable as approved, with the caps
      checked, a narrow change to SPEC-015 SD-11 (SD-3);
   4. new decisions live in `docs/decisions/` for the project and in the
      initiative's folder otherwise (SD-4);
   5. an amendment is an append-only revision, accepted by a person, with
      Record its ruling as the one no-append case, stricter than this
      repository's past practice (SD-5);
   6. supersession is a new decision's `supersedes:`, taking effect on
      acceptance, leaving the old file in place and its ID closed, and never
      retiring a decision of a wider owner (SD-6);
   7. the caps: 75-word ruling, 25-word one-line reason, 300-word conventions
      within half the token cap, ten decisions and 1,500 estimated tokens per
      dispatch (SD-8);
   8. at the caps, nearest owner first and most recently accepted first, with
      the left-out line in the prompt and the viewer (SD-9);
   9. every dispatch purpose receives the block, and decisions leave the
      background documents (SD-9a, SD-11);
   10. the block follows the project's name at the head of the user message,
       and the prompt order is fixed for the cache (SD-10);
   11. note, research, report and policy keep their current path, with the
       floor-only verdict path as the recommended follow-up (SD-12);
   12. one conventions document per project, revised normally (SD-13);
   13. the surfaced text is recorded at acceptance and read from the
       database (SD-14);
   14. `create_decision`, `list_decisions` and `get_decision` over MCP, with
       no new relay (SD-15);
   15. acceptance re-checks under locks, and one call is added to
       `approveDocumentAs` (SD-16).
