# Walkthrough — SPEC-018, decisions

**Date:** 2026-09-28
**Spec:** [SPEC-018](specs/SPEC-018-decisions.md), **approved by Sam,
2026-09-28** (recorded 2026-10-02)
**Review:** [REVIEW-018](reviews/REVIEW-018-decisions.md)
**Handoff:** [M11 handoff](notes/handoff-M11-2026-09-28.md)

This is the definition-of-done demo (SPEC-018 DoD 6). It ran a throwaway
project on a real Postgres **with no AI provider**. The project lived at
`/var/tmp/m11demo`, a short path, because a unix socket path can't be longer
than 108 bytes. The person's side ran in the browser, with Playwright and the
pre-installed Chromium. The chat agent's side ran as plain MCP JSON-RPC calls.

The scripts are in [walkthrough-spec-018/](walkthrough-spec-018/):

- `setup.sh` builds `./cmd/subutai`, makes the project with `subutai init`,
  copies in this repository's DEC-001 to DEC-007 and commits them, and serves
  the project on `127.0.0.1:8818`.
- `fake_provider.py` stands in for the model. It is a forty-line fake of the
  Messages API on `127.0.0.1:8819`: it answers every request by calling the
  outcome tool it is offered, with a fixed input, and it logs each request it
  receives. The project's `providers.anthropic.base_url` points at it. So the
  one agent run in the walk, a spec review, is a real dispatch by the real
  binary, and the log shows exactly what the provider was sent.
- `walk.js` is the person in the browser, and the chat agent over MCP.

```sh
bash docs/walkthrough-spec-018/setup.sh
node docs/walkthrough-spec-018/walk.js docs/walkthrough-spec-018
```

The walk's own log is [walk.log](walkthrough-spec-018/walk.log). What the
reviewer was told is saved twice: from its transcript on the run page,
[told.txt](walkthrough-spec-018/told.txt), and from the fake provider's log,
[provider-received.txt](walkthrough-spec-018/provider-received.txt). The two
are identical. `list_decisions` for the auth branch is in
[list_decisions.json](walkthrough-spec-018/list_decisions.json).

## 1. This repository's decisions, adopted as they are

The walk adopts DEC-001 to DEC-007 through the web UI's adopt route, as
approved. Each keeps its number, and nothing below its two identity lines
changes. None records a ruling in its front matter, so each is told to agents
by its title, without the ID said twice.

![The viewer after adoption](walkthrough-spec-018/01-viewer-adopted.png)

The viewer reads DEC-004's and DEC-006's existing `## Amendment 1` headings
and lists them under each decision.

## 2. The project conventions

*Start the conventions document* writes the template to `docs/conventions.md`
and opens the editor. The walk writes three points in and submits it, and the
person accepts it. Its page says every agent is told all of it.

![The conventions in the editor](walkthrough-spec-018/02-conventions-editor.png)
![The conventions accepted](walkthrough-spec-018/03-conventions-accepted.png)

## 3. A new decision, and a ruling that is too long

*New decision…* asks for a title and who the decision binds. It mints DEC-008,
writes the template to `docs/decisions/DEC-008-sessions-live-in-postgres.md`,
commits it and opens the editor.

![The dialog](walkthrough-spec-018/04-new-decision-dialog.png)
![The template in the editor](walkthrough-spec-018/05-decision-in-the-editor.png)

With a 200-word ruling, Submit is refused, and the page says why. The panel
underneath shows exactly what agents would be told, and its size against the
caps.

![A 200-word ruling refused](walkthrough-spec-018/06-long-ruling-refused.png)

With a nine-word ruling it passes. A decision has no agent reviewer: it waits
for a person, who accepts it.

![Waiting for a person](walkthrough-spec-018/07-decision-waiting.png)
![Accepted](walkthrough-spec-018/08-decision-accepted.png)

The walk then makes DEC-009, a decision of the Authentication initiative,
which only that branch of the tree is told.

## 4. Supersession

*Supersede with a new decision*, on DEC-008's page, asks for a title and
starts DEC-010 with `supersedes: [DEC-008]` filled in. When the person accepts
DEC-010, DEC-008 is superseded. Its file stays where it is, unchanged, and its
page says who superseded it and that agents are no longer told it.

![The supersede dialog](walkthrough-spec-018/09-supersede-dialog.png)
![DEC-008, superseded](walkthrough-spec-018/10-superseded-decision.png)

## 5. An amendment, and an edit that is refused

*Append an amendment*, on DEC-010's page, opens revision 2 in the editor with a
dated `## Amendment 1 — {{what changed}} (2026-09-28)` skeleton appended.

![The skeleton](walkthrough-spec-018/11-amendment-skeleton.png)

A version that also rewrites a line of the accepted text is refused at Submit.

![The edit refused](walkthrough-spec-018/12-edit-refused.png)

A version that only appends, and restates the ruling in the front matter, is
accepted. Revision 1 is archived as `docs/_superseded/DEC-010.r1.md`, and
agents are told the restated ruling.

![The amendment accepted](walkthrough-spec-018/13-amendment-accepted.png)

The walk also uses *Record its ruling* on DEC-001: a revision that adds
`ruling:` and `reason:` and changes nothing else. From then on agents are told
DEC-001's ruling rather than its title.

## 6. The viewer

Every decision, with its owner, state, what agents are told, what it
supersedes and what superseded it:

![The viewer, all states](walkthrough-spec-018/14-viewer-all.png)

With an initiative chosen, the page shows the block a dispatch there receives,
and its size against the cap.

![What a dispatch in INIT-001 is told](walkthrough-spec-018/15-viewer-branch.png)

## 7. What an agent was told

The chat agent creates the login feature, writes its spec, adopts it and
submits it over MCP. The spec reviewer runs against the fake provider and
approves. Its run page shows what it was told:

![What it was told](walkthrough-spec-018/16-run-what-it-was-told.png)

In full ([told.txt](walkthrough-spec-018/told.txt)), the block reads:

```
# Project decisions and conventions

These are binding: people decided them. If the task, or a document you are
given, contradicts one, follow the decision and say so in your outcome; don't
work around it.

## Conventions

### Code

- Errors are full sentences a person can act on.
- No new dependency without a decision.

### Tools

- `go test ./...` must pass before anything is submitted.

## Decisions

- DEC-001: The server, the CLI and all tooling are written in Go. Why: One static binary, and the team's experience.
- DEC-002: State Store — Supabase-hosted Postgres, plain-Postgres compatible
- DEC-003: The CLI shrinks to the vision's three components
- DEC-004: The MCP facet may author the planning layer, but not drive development
- DEC-005: The boundary is bypassing the orchestrator, not spawning agents
- DEC-006: A human decides when development starts
- DEC-007: Who does the work is flexible; who judges it is not
- DEC-010: Sessions are signed cookies that expire after twelve idle hours; the server keeps no session state. Why: Any server can answer any request.
- DEC-009 (INIT-001 Authentication): Hash passwords with argon2id, with the library defaults. Why: It is the current recommendation.
```

- DEC-008 is absent: it was superseded.
- DEC-010 carries its amended ruling, and DEC-001 its recorded one.
- DEC-009 comes last, because the project's decisions come before an
  initiative's, whatever the numbers.

The block came to about 350 estimated tokens, against a cap of 1,500. The
whole first message was 2,101 characters, and the system prompt 5,166.

## The suite

`go vet ./...` is clean, and `go test -race -count=1 -v ./...` passes: 307
top-level tests. The only skips are the opt-in M6 demo and a store test that
skips by design where the checklists table exists. No integration test
skipped.

The M11 tests (`internal/server/integration_decisions_test.go`), with the mock
provider wherever an agent runs:

| Test | What it proves |
|---|---|
| `TestDispatchIsToldItsBranchsDecisions` | A spec review's transcript holds exactly the conventions, the project's decisions and its own initiative's; nothing from a sibling initiative, no draft, nothing superseded; and the block follows the project's name. |
| `TestSupersession` | The superseded decision's file is untouched and its page kept; a narrower owner is refused; an amendment in review goes back to draft and can't then be submitted; the ID can't be adopted again, by `id:` or file name; the viewer points each at the other. |
| `TestAmendment` | The editor refuses; the page offers Append and Supersede; an unfilled or editing amendment is refused; an appending one is accepted, archived and surfaced with its restated ruling. |
| `TestValidationKeepsTheRulingShort` | A 200-word ruling, a multi-line reason and 301-word conventions are refused; a file edited after Submit can't be accepted; adoption as approved checks the caps; *This was already approved* is offered on an adopted decision only. |
| `TestSurfacingCapIsEnforced` | With a count cap of two, the nearest and newest are told and the oldest project decision is named as left out, in the prompt and the viewer. |
| `TestThisRepositorysDecisionsAdopt` | DEC-001 to DEC-007 adopt as they are, are told by title once each in order, record no supersession, and DEC-008 is next; a hand edit never reaches a prompt; the boot backfill restores the block; Record its ruling on DEC-001 works. |
| `TestOneConventionsDocument` | A second conventions document is refused by Start, attach and adopt. |
| `TestDecisionToolsOverMCP` | `create_decision`, `list_decisions` and `get_decision`, with `submit_for_review`. |
| `TestPromptOrderAndExecutionDispatches` | Through a full implementation loop, the implementer, code reviewer, verifier and spec reviewer each receive the block after the project's name, and the task after the shared contract. |

Unit tests cover the block byte for byte (`internal/content/surface_test.go`),
the amendment check's matrix and the two rule kinds
(`internal/lifecycle/amend_test.go`), and the configuration
(`internal/config/config_test.go`).

## What the walk didn't show

- **A real model.** The fake provider isn't one. A live smoke against a real
  provider would show whether agents follow the block; that is for Sam's live
  run.
- **Caching.** DeepSeek would cache the block for dispatches of one role on
  one branch; Anthropic caches nothing until Subutai sets breakpoints. Neither
  is measured here.
- **Relaying an acceptance from chat.** It uses the existing `relay_verdict`,
  which the M10 suite covers for any document.
