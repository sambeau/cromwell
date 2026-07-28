# Walkthrough — the workflow surface (SPEC-007) and the MCP facet (SPEC-008)

**Date:** 2026-07-26
**Specs:** [SPEC-007](specs/SPEC-007-workflow-surface-stage-a.md) (Stage A,
document-led browsing) and [SPEC-008](specs/SPEC-008-mcp-facet-planning-authoring.md)
(the MCP facet, slice 1 — planning authoring), built together as peers per
DESIGN-008 §6a.
**Status:** implemented; `go vet ./...` and `go test -race ./...` clean against
plain Postgres. The human-confirmed live smoke (DoD 2 of each spec) is recorded
below from a real driven session, and is Sam's to confirm.

---

## What was built, and what it proves

The old command centre was verb-shaped: thirteen global forms, one per CLI
subcommand, with entities addressed by typing their path into a box. This slice
replaces that surface with a **browsable place**. The engine underneath is
untouched — no orchestration capability was added, and every mutation still runs
through the same gated, audited service method the CLI has always used.

The load-bearing claim, and it holds:

> A person can understand and drive the project by browsing it, page to page,
> without typing an entity's path anywhere.

Alongside it, the chat agent can now *author* the planning layer over MCP — so
the structure a person browses is created the intended way, in conversation.

## The shape of the surface

| Address | What it is |
|---|---|
| `/ui` | Home, led by what needs doing |
| `/ui/project` | The project as an entity page |
| `/ui/i/<path>` | An initiative, e.g. `/ui/i/auth` |
| `/ui/f/<path>` | A feature, e.g. `/ui/f/auth/login` |
| `/ui/d/<path>` | A document at its repository path |
| `/ui/m/<id>`, `/ui/r/<id>`, `/ui/t/<id>` | Milestone, roadmap, task — plain read-only pages so links resolve |
| `/mcp` | The MCP endpoint for the chat agent |

`/ui/planning`, `/ui/cost` and `/ui/document?path=` still resolve: they redirect
to `/ui/project`, `/ui/work` and `/ui/d/<path>` respectively, so nothing that was
bookmarked breaks.

Every entity page has one anatomy (DESIGN-008 §5.2): breadcrumbs across the top,
the description and then **the main design document rendered as the body**, a
**rail** on the right carrying status, size in tokens and the actions valid for
that thing in its state, and the relation sections stacked below with a jump
menu.

## The live session

Driven against a real project on plain Postgres, with the server running in one
terminal and a browser open on the surface.

### 1. The chat agent authors the planning layer (SPEC-008 DoD 2)

An MCP client handshook and listed tools. It was offered **exactly nine**:

```
create_initiative   create_feature   update_initiative   update_feature
attach_document     get_tree         get_initiative      get_feature
list_documents
```

It then created an initiative and a feature and set their descriptions in human
prose — the same service methods the UI calls:

```
create_initiative  slug=billing  name="Billing and payments"
  → {"path":"billing","url":"/ui/i/billing"}
create_feature     initiative_path=billing slug=invoices name="Invoices"
  → {"path":"billing/invoices","state":"idea","url":"/ui/f/billing/invoices"}
update_feature     path=auth/login description="The screen where a returning
                   person signs in with an email address and a password."
  → name unchanged; only the description was written
```

Two things worth noting. The new feature came back in state `idea` — the facet
creates work, it never starts it. And the update was genuinely partial: the name
survived a description-only write, because the shared service method takes each
field as optional.

**The DEC-004 line, tested against the live server:**

```
start_feature       -> there is no tool called "start_feature" on this server
override_gate       -> there is no tool called "override_gate" on this server
spawn_agent         -> there is no tool called "spawn_agent" on this server
archive_initiative  -> there is no tool called "archive_initiative" on this server
```

Rejected by the MCP layer as unknown methods, not by a permission check — the
tools do not exist, so they cannot be called. That is the enforcement (SD-2).

### 2. A person browses and drives it (SPEC-007 DoD 2)

Opening `/ui` gave Home: nothing in flight, the project sized at **9.2k tokens**,
*Invoices* listed under "not yet estimated", features grouped by state,
initiatives (including the one the chat agent had just made), and the
project-level milestone and roadmap. No dollar figure anywhere.

From there, by clicking only:

- **Home → Authentication.** The initiative page rendered its attached design
  document as the body — headings, prose and a GFM table, through the existing
  goldmark + bluemonday path. The rail showed 9.2k tokens with the `rough` tier.
  Below, "Inside" listed both features as cards with their own sizes and the
  descriptions the chat agent had written.
- **Authentication → Login form.** Breadcrumbs carried `Project ▸ Authentication ▸
  Login form`, each crumb a link. The rail showed **STATUS idea**, **SIZE 1.2k
  tokens rough**, and *Start work* **disabled** with the reason in plain words:
  "Work can start once this feature's specification is approved, which moves it
  to ready. It is still an idea." Not hidden, not a silent failure (FR-2.2).
- The body was the empty state — the feature had a spec but no design document —
  so the *absence* of writing was visible rather than a blank panel.
- **Attached a document from the rail.** Entering `docs/design/login.md` and
  pressing Attach returned the page with a notice, *"Document attached:
  docs/design/login.md"*, the document now rendered as the page body, and the
  document listed in the Documents section marked **main**. This is the
  capability that had to exist before `cromwell doc add` can be retired (DEC-003).
- **The milestone page** showed the two-list display: *"0 of 2 members finished —
  0 of 9.2k tokens"* with a token bar, over an unordered checklist of members,
  each clicking through to its own page.
- **Recent activity** on the feature page showed `chat-agent feature.updated`
  beside the operator's own actions — the MCP mutation on the audit trail,
  attributed to its configured actor. That is what makes a second authoring
  surface safe (NFR-6).

At no point was an entity path typed into a form. The only free-text path on any
page is the attach-a-document field, and it names a **file in the repository**;
it is called `file_path` precisely so the rule can be checked absolutely.

## The checks

```
$ go vet ./...            # clean
$ go test -race ./...     # all packages ok
```

Repository-wide, against the live server:

- **No currency on any rendered page.** `/ui`, `/ui/project`, `/ui/i/auth`,
  `/ui/f/auth/login`, `/ui/work`, `/ui/documents`, `/ui/inbox` and a milestone
  page: zero matches for a currency figure. The template test asserts the same
  over the embedded templates, and the `usd`/`usd2`/`clampPct` helpers have been
  deleted — money cannot reach the surface because there is no longer a way to
  format it. The engine's ledger, price table and dollar cap are untouched and
  dormant (Q-C, SD-5).
- **No typed entity path.** No template renders `name="ref"`, `name="path"`,
  `name="parent_path"`, `name="initiative_path"`, `name="milestone"` or
  `name="roadmap"`. Asserted in `TestUIBrowseAndDrive` with no exemptions.

New tests: `TestUIBrowseAndDrive`, `TestUIEntityActions`,
`TestUIMilestoneAndRoadmapPages`, `TestUIWorkViewIsTokensOnly`,
`TestGatedActionShowsPlainReason`, `TestRenderedTemplatesCarryNoCurrency`,
`TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet`, `TestMCPAuthoringSlice`,
`TestMCPInitializeHandshake`, `TestMCPMutationReachesTheUILive`,
`TestOwnerReadsAndPrimaryDoc`, `TestUpdateEntityFields`.

## A defect found while building, and fixed

**Top-level initiative slugs were not unique.** The 0001 constraint is
`UNIQUE (parent_id, slug)`, and SQL treats NULLs as distinct — so any number of
parent-less initiatives could share a slug. Under the old UI that was merely
untidy. Under this one it breaks the central promise: `/ui/i/auth` would be
ambiguous, and `InitiativeBySlugPath` would return whichever row it met first.

Migration 0005 adds a partial unique index on `(slug) WHERE parent_id IS NULL`.
If a project has already accumulated duplicate root slugs the migration fails and
names them, which is the honest outcome — those rows cannot both be addressable,
and one has to be renamed before the surface can work.

## What this slice deliberately did not do

- **Milestone and roadmap editing** (SD-1). They are owned and displayed here;
  composing members, ordering and locking are Stage B. The service methods
  already exist.
- **A server-side description generator.** Dropped, not deferred (SD-2, D-13).
  Descriptions come from the chat agent or a human.
- **Removing the CLI subcommands** (DEC-003). Stage A covers some of them; the
  checklist in `docs/notes/dec-003-cli-removal-checklist.md` records which, and
  which still wait on Stage B. Nothing was deleted.
- **Visual and production design.** This is the information architecture with
  enough skin to read it, in light and dark.

## Where to pick it up

- Stage B entry criteria: `docs/notes/spec-007-stage-b-entry-criteria.md`
- The CLI-removal checklist: `docs/notes/dec-003-cli-removal-checklist.md`
- The next MCP slice (read/poke tools): `docs/notes/spec-008-slice-2-entry-criteria.md`
