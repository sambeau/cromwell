# DEC-003: The CLI shrinks to the vision's three components

**Status:** Accepted
**Date:** 2026-07-22
**Decided by:** Sam
**Resolves:** the undesigned growth of `cmd/cromwell` beyond
[vision-v1](../vision/vision-v1.md) §7's component diagram
**Supersedes:** [SPEC-001](../specs/SPEC-001-phase-1-vertical-slice.md) FR-2.2
in part (the CLI as the general-purpose API client)

## Decision

The `cromwell` binary keeps only the subcommands that operate the *process*:

| Command | Why it stays |
|---|---|
| `init` | Bootstraps `.cromwell/`, runs migrations (vision §12). Nothing else can — it runs before a server exists. |
| `serve` | Starts the server. |
| `status` | Liveness and version. A process check, not a workflow surface. |
| `hook` | The git watcher's inbound edge (`post-commit`). A component in the §7 diagram. |
| `upgrade` | Vision §12's migration/diff surface. Not yet built; reserved. |

Every other subcommand — `initiative`, `feature`, `task`, `doc`, `validate`,
`submit`, `revise`, `inbox`, `respond`, `log`, `cost`, `estimate`, `milestone`,
`roadmap`, `search` — is **removed**. Their capability moves to the two human
and machine surfaces the vision actually names: the **web UI** for humans
([DESIGN-008](../design/DESIGN-008-the-workflow-surface.md)) and the **MCP
facet** for editor and chat assistants (its authoring slice is
[SPEC-008](../specs/SPEC-008-mcp-facet-planning-authoring.md)).

The JSON API at `/api/*` **survives unchanged**, as the programmatic HTTP/JSON
surface for scripting, CI and external clients. It is no longer the CLI's
backend. Note that the MCP facet does **not** consume `/api/*` either: it is a
third rendering over the same service layer, calling service methods directly
(SPEC-008; this refines DESIGN-007 §4, which had loosely cast the JSON API as the
facet's backend). `internal/client` survives as the transport for the retained
commands.

## Context

Vision §7 names three boundaries onto the server: Web UI, MCP facet, git
watcher. A general-purpose CLI is not among them. It arrived in SPEC-001
FR-2.1/2.2 for a legitimate reason — phase 1 had no web UI, and the vertical
slice needed *some* human surface to prove the engine through — and was never
revisited once the real surfaces arrived.

The cost of not revisiting it became concrete in phase 4. The command centre's
planning surface was built as one HTML form per CLI subcommand, with free-text
fields for the path arguments those subcommands take
([planning.html:70](../../internal/server/ui/templates/planning.html)). The CLI
did not merely coexist with the UI; it supplied the UI's shape. A verb-and-
argument surface is the correct shape for a shell and the wrong shape for a
browser, and while both exist as peers the browser keeps losing.

## Rationale

- **It restores the vision's architecture.** Three components, three
  boundaries, each with a distinct job. The CLI becomes what the diagram
  implies: the way you start and hook the server, not a way to drive work.
- **It removes the gravitational pull on the UI.** With no CLI verb to mirror,
  the UI is free to be object-shaped (DESIGN-008 §4): you navigate to a thing
  and act on it, rather than naming a thing in a form field.
- **It does not weaken the machine surface.** Scripting, CI and external HTTP
  access were the honest arguments for keeping the CLI. Those are served by
  `/api/*`, which remains. Assistant access is served by the MCP facet — a
  separate rendering over the same service layer (SPEC-008), not a client of
  `/api/*`.
- **It shrinks a real maintenance surface.** ~880 lines of argument parsing,
  output formatting and flag handling in `cmd/cromwell/main.go`, plus its share
  of `internal/client`, for a surface the product does not claim to have.

## Consequences

### Sequencing: the UI leads, the CLI follows

**The removal must not land before the web UI covers the capability.** Deleting
`cromwell milestone lock` while the only other way to lock a milestone is a
typed-path form is a capability regression dressed as cleanup. The order is
binding:

1. The redesigned UI and the MCP authoring slice land and, **across Stage A
   (SPEC-007) and Stage B**, cover every operation the removed commands expose.
   (Stage A covers browsing and most lifecycle actions and document attachment;
   milestone/roadmap editing and estimates arrive in Stage B — so the
   `milestone`, `roadmap` and `estimate` commands wait for it.)
2. The removal lands as a separate, mechanical commit with the coverage
   demonstrated.

Until step 2, the CLI stays as-is. This decision fixes the destination, not the
date.

### One capability the UI must grow first

Document **registration** is CLI-only today. The git watcher detects drift in
documents that are already registered
([documents.go:274](../../internal/server/documents.go)) but cannot register a
new one — that is `cromwell doc add`. So `doc add`'s capability has no home
until the UI provides one.

This is not an obstacle; it is a requirement DESIGN-008 must satisfy anyway.
A document-led interface whose central act is "attach a document to this
initiative" needs registration as a first-class action on the entity page. The
gap and the redesign point the same way.

The alternative — having the watcher auto-register any Markdown file matching a
convention — is **rejected**. It would make git the writer of entity state,
crossing the compartment boundary the vision draws in §12 (Postgres holds
records; git holds content). Registration is an act of intent and belongs to a
human at a surface, not to a path pattern.

### Documentation and provenance

- `docs/manual-testing.md` is already UI-driven; it references the CLI only for
  build and serve, which survive.
- Phase 1–3 walkthroughs record CLI transcripts as the honest history of how
  those phases were proven. They are **not** rewritten. They are historical
  records, and a superseded surface in a dated walkthrough is accurate.
- No test shells out to the removed commands; `internal/client/boundary_test.go`
  (the CLI-imports-no-store assertion) survives against the retained ones.

### What this closes off

Headless scripted workflow driving via a shell — `cromwell feature start X` in
a Makefile — is no longer supported. The replacement is an HTTP call to
`/api/*` or an MCP tool call. This is a deliberate narrowing: the surfaces that
remain are the ones the product is designed around and reviewed against.

## Alternatives considered

- **Keep the CLI as a first-class equal surface.** Every workflow operation
  exists in both CLI and UI, kept in parity. Rejected: it doubles the design
  surface for every future capability, and — as phase 4 demonstrated — the
  parity requirement drags the UI toward command shapes rather than dragging
  the CLI toward anything.
- **Keep the CLI, stop letting it shape the UI by discipline alone.** Rejected
  as insufficiently load-bearing. The phase-4 outcome *was* the disciplined
  outcome: DESIGN-007 §4's "two renderings over one service layer" is sound
  architecture, and the forms still came out CLI-shaped. A convention that has
  already failed once under good intentions is not a control.
- **Deprecate rather than delete.** Rejected as the worst of both: the
  maintenance surface stays, the shaping pressure stays, and the product
  acquires a second answer to "how do I do this?" that it does not stand
  behind.

## Addendum — what SPEC-009 Stage 1 covers (2026-07-30)

SPEC-009's Definition of Done asks for this checklist to record what the
authoring chain covers of the removal and what it does not:

- **Design approval and request-changes** are UI-covered: the document page's
  Approve and Ask-for-changes actions (SPEC-009 FR-2.3) are gated, audited
  service methods with no CLI involvement. This was never a CLI verb — it is
  new capability — but it removes the last workflow *decision* that had no
  human surface.
- **`doc submit` and `doc revise` remain CLI-only for human-written
  documents.** Agents submit through `submit_document`, and the revision
  cascade opens successor drafts itself, but a person submitting or revising a
  document they wrote by hand still uses the CLI. The document-page Submit and
  Revise actions are Stage 2, and the removal of these verbs waits for them.
- **`doc add` (registration) is UI- and MCP-covered** since SPEC-007/008; the
  design template makes no difference to that.
- **`doc validate` stays uncovered deliberately** (DESIGN-009 SD-3, SPEC-009
  scope table): validation runs on submit and inside the authoring loop, and a
  standalone check has no consumer. It closes with the verb's removal, not
  with a replacement.
- **`respond` gains no new gap.** The design-revision checkpoint's per-spec
  answer is a UI form; the CLI's generic respond cannot express it and is not
  taught to. One more reason the verb's removal is overdue rather than
  blocked.
