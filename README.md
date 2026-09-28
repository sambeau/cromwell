# Subutai

A document-led, specification-centred workflow system that takes a software
project from initial concept to shipped features — humans driving planning,
AI agents driving development. Greenfield successor to kanbanzai.

Subutai was formerly called **Cromwell**. The historical documents keep the
old name, and projects made with Cromwell keep working for one release (see
[Coming from Cromwell](#coming-from-cromwell)).

**Status:** the engine is built and proven live. Phases 1 to 4, SPEC-006,
SPEC-007, SPEC-008 and SPEC-009 Stage 1 have shipped: the implementation loop,
planning and tracking, the web command centre, the MCP facet for planning, and
the authoring chain, in which agents write and review the spec and dev-plan
from an approved design. It is now being revised as **Subutai**: the same
engine with a clearer workflow and a new name. Where that stands, and the
milestones to finish it, are in the
[Subutai status and roadmap](docs/notes/subutai-status-and-roadmap-2026-09-28.md).
The design ([DESIGN-010](docs/design/DESIGN-010-subutai.md)) is approved, and
**phase B, the Subutai core, is merged (2026-09-28)**, with its specs
(SPEC-010 to SPEC-017) approved:
- M3: approving a design starts nothing, and a human presses **Send to
  development**;
- M4 and M5: milestones, roadmaps and checklists, editable in the browser and
  from chat;
- M6: transcripts, timelines and review health;
- M7: the rename to Subutai;
- M8: an ID for everything, and documents adopted where they sit;
- M9: editing in the browser;
- M10: the chat agent's seat, and who wrote every document.

What stands between this and "Subutai usable" is the live smoke runs against a
real provider. Their checklists are in the M0, M3 and M6 handoffs.

Build: `go build ./cmd/subutai` · Test: `eval "$(scripts/test-db.sh)"` then
`go test -race -count=1 ./...` (without `SUBUTAI_TEST_DATABASE_URL` the
integration tests skip silently; Claude Code cloud sessions set it through
the SessionStart hook) · Start: `subutai init` then `subutai serve` (needs
`SUBUTAI_DATABASE_URL` and a provider key) · Smoke project:
`scripts/smoke-project.sh` ([manual testing](docs/manual-testing.md)).

## Document map

Authority runs top to bottom (spec binds, design explains, decisions record):

| Document | What it is |
|---|---|
| [vision-v1](docs/vision/vision-v1.md) | Product vision and architectural sketch (draft v1) |
| [DEC-001](docs/decisions/DEC-001-server-language-go.md) | Server language: **Go** |
| [DEC-002](docs/decisions/DEC-002-postgres-via-supabase.md) | State store: **Supabase-hosted Postgres**, plain-Postgres compatible |
| [DEC-003](docs/decisions/DEC-003-cli-scope.md) | The CLI shrinks to `init`, `serve`, `status` and `hook`; the rest moves to the UI and MCP ([removal checklist](docs/notes/dec-003-cli-removal-checklist.md)) |
| [DEC-004](docs/decisions/DEC-004-mcp-planning-authoring.md) | The MCP facet may author the planning layer, but not drive development |
| [DEC-005](docs/decisions/DEC-005-the-orchestration-boundary.md) | The boundary is bypassing the orchestrator, not spawning agents (supersedes DEC-004 in part) |
| [DEC-006](docs/decisions/DEC-006-humans-start-development.md) | Subutai: approving a design starts nothing; a human presses **Send to development**. Amendment 1: how specs are reviewed, and what chat may relay |
| [DEC-007](docs/decisions/DEC-007-the-judgement-boundary.md) | Who does the work is flexible; who judges it is not (supersedes DEC-005 in part) |
| [DESIGN-001](docs/design/DESIGN-001-data-model-and-schema.md) | Data model and Postgres schema |
| [DESIGN-002](docs/design/DESIGN-002-orchestrator.md) | The orchestrator: events, dispatch, tool host, governance |
| [DESIGN-003](docs/design/DESIGN-003-document-lifecycle-and-gates.md) | Document lifecycle, feature lifecycle, gate catalogue |
| [DESIGN-004](docs/design/DESIGN-004-config-compartment.md) | The project folder, `.subutai/` (formerly `.cromwell/`): config, roles, skills, templates, pack lock |
| [DESIGN-005](docs/design/DESIGN-005-dev-plans-tasks-decomposition.md) | Phase 2: dev-plans, tasks, decomposition, G1 extension, revision-in-flight |
| [DESIGN-006](docs/design/DESIGN-006-tool-host-and-worktrees.md) | Phase 2: the tool host, worktrees, implementer/verifier dispatch, merge |
| [DESIGN-007](docs/design/DESIGN-007-web-command-centre.md) | Phase 4: the web command centre |
| [DESIGN-008](docs/design/DESIGN-008-the-workflow-surface.md) | The workflow surface: browsable, document-led pages, and the MCP facet |
| [DESIGN-009](docs/design/DESIGN-009-the-authoring-half.md) | The authoring half: agents write specs and dev-plans from an approved design |
| [DESIGN-010](docs/design/DESIGN-010-subutai.md) | Subutai: the revised workflow (**approved**, Draft 2) |
| [SPEC-001](docs/specs/SPEC-001-phase-1-vertical-slice.md) | Phase 1: the vertical slice (**binding**) |
| [SPEC-002](docs/specs/SPEC-002-phase-2-implementation-loop.md) | Phase 2: the implementation loop (**binding**) |
| [SPEC-003](docs/specs/SPEC-003-phase-3-planning-and-tracking.md) | Phase 3 (first slice): sizing, calibration, milestones (**binding**) |
| [SPEC-004](docs/specs/SPEC-004-phase-4-web-command-centre.md) | Phase 4: the web command centre, operational slice (**binding**) |
| [SPEC-006](docs/specs/SPEC-006-command-centre-mutations.md) | The command centre's mutation slice: planning and review actions (**binding**) |
| [SPEC-007](docs/specs/SPEC-007-workflow-surface-stage-a.md) | The workflow surface, Stage A: document-led browsing (**binding**) |
| [SPEC-008](docs/specs/SPEC-008-mcp-facet-planning-authoring.md) | The MCP facet, slice 1: planning authoring (**binding**) |
| [SPEC-009](docs/specs/SPEC-009-the-authoring-chain.md) | The authoring chain, Stage 1 (**binding**) |
| [SPEC-013](docs/specs/SPEC-013-rename-to-subutai.md) | The rename to Subutai, and what stays compatible (**binding**) |
| [REVIEW-001](docs/reviews/REVIEW-001-phase-1-package.md) | Approval review of the phase-1 package: findings, fixes, verdict |
| [REVIEW-002](docs/reviews/REVIEW-002-phase-2-package.md) | Consistency review of the phase-2 package: findings, fixes, recommendation |
| [REVIEW-003](docs/reviews/REVIEW-003-phase-3-planning-package.md) | Consistency review + approval of SPEC-003 |
| [REVIEW-004](docs/reviews/REVIEW-004-phase-4-web-command-centre-package.md) | Consistency review + approval of DESIGN-007 and SPEC-004 |
| [REVIEW-006](docs/reviews/REVIEW-006-command-centre-mutations-package.md) | Consistency review + approval of SPEC-006 |
| [REVIEW-009](docs/reviews/REVIEW-009-authoring-chain-package.md) | Consistency review + approval of SPEC-009 |
| [REVIEW-013](docs/reviews/REVIEW-013-rename-to-subutai.md) | Independent check of SPEC-013 and the rename for missed surfaces |
| [walkthrough](docs/walkthrough.md) | Phase-1 live smoke-test session: commands, audit trail, cost |
| [walkthrough-phase-2](docs/walkthrough-phase-2.md) | Phase-2 live smoke test: a task implemented, reviewed, verified, merged |
| [walkthrough-phase-3](docs/walkthrough-phase-3.md) | Phase 3: sizing, calibration and milestones |
| [walkthrough-phase-4](docs/walkthrough-phase-4.md) | Phase 4: the web command centre |
| [walkthrough-spec-006](docs/walkthrough-spec-006.md) | SPEC-006: the command centre's mutation slice |
| [walkthrough-spec-007-008](docs/walkthrough-spec-007-008.md) | SPEC-007 and SPEC-008: the workflow surface and the MCP facet |
| [walkthrough-spec-009-stage1](docs/walkthrough-spec-009-stage1.md) | SPEC-009 Stage 1: approval to gate 2, live, for 15,976 tokens |
| [walkthrough-spec-013](docs/walkthrough-spec-013.md) | SPEC-013: a fresh Subutai project, and a Cromwell project carried across |
| [manual testing](docs/manual-testing.md) | Running Subutai by hand, and rebuilding the smoke project |
| [phase-2 entry criteria](docs/notes/phase-2-entry-criteria.md) | What had to be true before phase 2 started |
| [phase-3 entry criteria](docs/notes/phase-3-entry-criteria.md) | What must be true before phase 3 starts; phase-2 audit summary |
| [Subutai status and roadmap](docs/notes/subutai-status-and-roadmap-2026-09-28.md) | Where the Subutai revision stands (2026-09-28), and the milestone plan to finish it |
| [research](docs/research/README.md) | The research behind the design, and the writing guide |

## Phase plan

- **Phase 1 — the vertical slice** (SPEC-001) ✅ **delivered**: `init`,
  server + CLI, initiatives/features, document lifecycle with
  validation and a real agent-reviewer, escalation inbox, complete audit and
  cost ledger. Proved the three load-bearing claims: code orchestrator,
  escalation-only human gates, ledger-by-construction.
- **Phase 2 — the implementation loop** (SPEC-002) ✅ **delivered**: dev-plans
  with validated task tables, decomposition, full gate G1, the tool host
  (jailed, hash-anchored, whitelisted commands), implementer dispatch in
  per-feature worktrees, code review of diffs, close-out verification (G3),
  and merge — a task carried from approved contract to merged code by agents.
- **Phase 3 — planning and tracking** (SPEC-003) ✅ **delivered**: token
  sizing, estimates with confidence tiers, calibration from actuals,
  milestones and roadmaps.
- **Phase 4 — the web command centre** (SPEC-004, SPEC-006) ✅ **delivered**:
  read-everywhere views and checkpoint responses, then the planning and review
  actions from the browser.
- **The workflow surface and the MCP facet** (SPEC-007 Stage A, SPEC-008
  slice 1) ✅ **delivered**: a browsable, document-led UI, and nine MCP tools
  a chat agent uses to author the planning layer. Their human-confirmed live
  smokes still await Sam.
- **The authoring chain** (SPEC-009 Stage 1) ✅ **delivered**: an approved
  design produces a reviewed spec, dev-plan and tasks with no further human
  act, and stops at gate 2; proven live for 15,976 tokens. The revision-cascade
  smoke (DoD 3) has not run.
- **Subutai** (in progress): the revision planned in the
  [roadmap](docs/notes/subutai-status-and-roadmap-2026-09-28.md) §11.
  - **Merged:**
    - Send to development ([DEC-006](docs/decisions/DEC-006-humans-start-development.md), SPEC-011);
    - milestone and roadmap editing (SPEC-010);
    - transcripts, the feature timeline and review health (SPEC-012).
  - **Built and approved:** the rename to Subutai (SPEC-013).
  - **Next:** checklists, document identity and the editor, executors, bugs,
    spikes, decisions, and GitHub adoption.

## Coming from Cromwell

A project made with Cromwell loads under Subutai as it is, until the next
release, and says what to rename:

- **The project folder.** `.cromwell/` is read when there is no `.subutai/`.
  Stop the server and run `git mv .cromwell .subutai`, then commit. A feature
  that is building keeps its worktree: the server finds it in the renamed
  folder and repairs it in git on start.
- **Environment variables.** Each `CROMWELL_*` variable is read when its
  `SUBUTAI_*` twin is unset. Rename them, and change `database.url_env` in
  `config.yaml` to `SUBUTAI_DATABASE_URL`.
- **The post-commit hook.** `subutai serve` points the hook `init` installed
  at itself. A hook you have edited is left alone, with a note saying which
  line to change.
- **Chat clients.** The MCP endpoint is still `/mcp`, so a client added as
  `cromwell` keeps working. Re-add it as `subutai` when you like.

History stays as it was: past audit rows, actor names, commits by
`cromwell <cromwell@localhost>`, and `cromwell/…` branches are not rewritten.
The rules, and when they end, are in
[SPEC-013](docs/specs/SPEC-013-rename-to-subutai.md).

## Design notes

Three deliberate resolutions of gaps in vision v1, made in DESIGN-003 and
flagged for review there: document states vs events (L-1), revision of
approved documents mid-flight (L-2/L-3), and retained close-out verification
(L-4).
