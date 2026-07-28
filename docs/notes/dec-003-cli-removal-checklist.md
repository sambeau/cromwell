# DEC-003 CLI-removal checklist

**Written:** 2026-07-26, on finishing Stage A (SPEC-007) and the MCP authoring
slice (SPEC-008).
**Decision being tracked:** [DEC-003](../decisions/DEC-003-cli-scope.md) — the
`cromwell` binary keeps only `init`, `serve`, `status`, `hook` (and a reserved
`upgrade`); every other subcommand is removed once the web UI and the MCP facet
cover it.

**Nothing has been deleted yet.** DEC-003's sequencing is explicit: removal waits
until Stages A *and* B demonstrably cover the commands. This checklist records
what is covered now, so the eventual removal commit is a matter of checking boxes
rather than re-deriving the argument.

## Legend

- **Covered** — a person can do this from the browsable UI, or a chat agent can
  do it over MCP, without the CLI.
- **Stage B** — waits on milestone/roadmap editing or the work list.
- **Later slice** — waits on a later MCP slice (read/poke tools).

## The commands

| Command | UI (SPEC-007) | MCP (SPEC-008) | Status |
|---|---|---|---|
| `initiative add` | New initiative, from the project or a parent initiative page | `create_initiative` | **Covered** |
| `initiative archive` | Archive, from the initiative's own page (G5 raises the checkpoint as before) | — (deliberately absent, DEC-004) | **Covered** |
| `feature add` | New feature, from the initiative page | `create_feature` | **Covered** |
| `feature start` | Start work, from the feature page (disabled with a reason when gated) | — (never, DEC-004) | **Covered** |
| `feature abandon` | Abandon with a required reason, from the feature page | — | **Covered** |
| `doc add` | Attach a document, from any entity page | `attach_document` | **Covered** |
| `estimate set` | Set estimate, from the entity rail | — (later slice) | **Covered** |
| `estimate ai` | Ask the estimator agent, from the entity rail | — (never: it spawns an agent) | **Covered** |
| `inbox` | The Inbox view | — (later slice) | **Covered** |
| `respond` | Respond from the Inbox; approve / request changes from a document page | — (later slice) | **Covered** |
| `log` | Recent activity on each entity page; recent events on Home | — | **Covered** for the per-entity and recent cases; a full filterable audit view is not built. Confirm with Sam whether that is needed before removal. |
| `cost` | Replaced by the **Work** view, in tokens (D-4). The money-denominated report is gone from the surface by design, not missing. | — | **Covered**, with the deliberate change of unit noted. |
| `milestone create/add/remove/lock` | Displayed read-only in Stage A | — | **Stage B** |
| `roadmap create/add/list` | Displayed read-only in Stage A | — | **Stage B** |
| `task list/show` | Plain read-only task page at `/ui/t/<id>`; no list or filter | — | **Stage B** (the filterable work list and task detail) |
| `doc validate` | Not in the UI | — | **Not covered.** Validation runs on submit; a standalone "check this file" affordance does not exist. Decide whether it needs one or whether submit-time validation suffices. |
| `doc submit` | Not in the UI | — | **Not covered.** Submitting a document for review is still CLI-only; it belongs on the document page. Worth folding into Stage B. |
| `doc revise` | Not in the UI | — | **Not covered.** Creating a successor draft is CLI-only. |
| `doc comments` | Comments render on the document page | — | **Covered** for reading. |
| `search` | Not in the UI | — (later slice: content search) | **Not covered.** |

## What this means for the removal commit

Three groups block a clean deletion today:

1. **Stage B work** — milestones, roadmaps, tasks. Known and planned.
2. **The document lifecycle verbs** — `doc validate`, `doc submit`, `doc revise`.
   These were not in SPEC-007's scope and are not in Stage B's as currently
   drafted. They are the surprise on this list: a person can *attach* and *read*
   and *review* a document from the UI, but cannot *submit* one for review
   without the CLI. **This should be added to Stage B's scope** (or a small
   slice of its own) before DEC-003 can complete.
3. **`search`** — waits on the later MCP read/poke slice, and arguably wants a
   UI surface too.

Everything else in the table is already covered and could be deleted the day the
above land.

## Verified facts that make removal safe

- **No test shells out to the CLI** (verified again on 2026-07-26): the suites
  drive the HTTP handlers and service layer directly, so deleting subcommands
  breaks no test.
- **The git watcher only detects drift in already-registered documents**
  (`internal/server/documents.go`, `publishIfDrifted`), which is why "attach a
  document" had to exist in the UI first. It now does, in both surfaces.
- **`internal/client` stays** — it is the transport for the retained commands.
- **`/api/*` stays** — the programmatic surface for scripting and CI, unchanged.
