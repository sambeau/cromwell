# Cromwell

A document-led, specification-centred workflow system that takes a software
project from initial concept to shipped features — humans driving planning,
AI agents driving development. Greenfield successor to kanbanzai.

**Status:** phase 1 complete. The vertical slice runs end to end against a
Supabase-hosted project and a live provider API — see the session record in
[walkthrough](docs/walkthrough.md). One Definition-of-Done item remains:
running the automated suite against the Supabase local stack as well as
plain Postgres (FR-1.3), a CI configuration task.

Build: `go build ./cmd/cromwell` · Test: `CROMWELL_TEST_DATABASE_URL=<plain
postgres url> go test -race ./...` · Start: `cromwell init` then
`cromwell serve` (needs `CROMWELL_DATABASE_URL`, `ANTHROPIC_API_KEY`).

## Document map

Authority runs top to bottom (spec binds, design explains, decisions record):

| Document | What it is |
|---|---|
| [vision-v1](docs/vision/vision-v1.md) | Product vision and architectural sketch (draft v1) |
| [DEC-001](docs/decisions/DEC-001-server-language-go.md) | Server language: **Go** |
| [DEC-002](docs/decisions/DEC-002-postgres-via-supabase.md) | State store: **Supabase-hosted Postgres**, plain-Postgres compatible |
| [DESIGN-001](docs/design/DESIGN-001-data-model-and-schema.md) | Data model and Postgres schema |
| [DESIGN-002](docs/design/DESIGN-002-orchestrator.md) | The orchestrator: events, dispatch, tool host, governance |
| [DESIGN-003](docs/design/DESIGN-003-document-lifecycle-and-gates.md) | Document lifecycle, feature lifecycle, gate catalogue |
| [DESIGN-004](docs/design/DESIGN-004-config-compartment.md) | The `.cromwell/` compartment: config, roles, skills, templates, pack lock |
| [DESIGN-005](docs/design/DESIGN-005-dev-plans-tasks-decomposition.md) | Phase 2: dev-plans, tasks, decomposition, G1 extension, revision-in-flight |
| [DESIGN-006](docs/design/DESIGN-006-tool-host-and-worktrees.md) | Phase 2: the tool host, worktrees, implementer/verifier dispatch, merge |
| [SPEC-001](docs/specs/SPEC-001-phase-1-vertical-slice.md) | Phase 1: the vertical slice (**binding**) |
| [SPEC-002](docs/specs/SPEC-002-phase-2-implementation-loop.md) | Phase 2: the implementation loop (**binding**) |
| [REVIEW-001](docs/reviews/REVIEW-001-phase-1-package.md) | Approval review of the phase-1 package: findings, fixes, verdict |
| [REVIEW-002](docs/reviews/REVIEW-002-phase-2-package.md) | Consistency review of the phase-2 package: findings, fixes, recommendation |
| [walkthrough](docs/walkthrough.md) | Phase-1 live smoke-test session: commands, audit trail, cost |
| [phase-2 entry criteria](docs/notes/phase-2-entry-criteria.md) | What must be true before phase 2 starts |

## Phase plan

- **Phase 1 — the vertical slice** (SPEC-001): `cromwell init`, server + CLI,
  initiatives/features, document lifecycle with validation and a real
  agent-reviewer over the Anthropic API, escalation inbox, complete audit and
  cost ledger. Proves the three load-bearing claims: code orchestrator,
  escalation-only human gates, ledger-by-construction.
- **Phase 2 — implementation loop**: tasks and dev-plans (full gate G1),
  implementer dispatch with worktrees and the tool host, close-out
  verification (G3), embeddings and semantic retrieval.
- **Phase 3 — command centre**: web UI, MCP facet, milestones, roadmaps,
  estimates and calibration, defects, checklists.

## Design notes

Three deliberate resolutions of gaps in vision v1, made in DESIGN-003 and
flagged for review there: document states vs events (L-1), revision of
approved documents mid-flight (L-2/L-3), and retained close-out verification
(L-4).
