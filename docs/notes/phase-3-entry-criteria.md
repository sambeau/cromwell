# Phase-3 Entry Criteria (draft)

**Status:** Draft — satisfies SPEC-002 DoD item 5
**Date:** 2026-07-21

Phase 3 (the command centre — README phase plan: web UI, MCP facet,
milestones, roadmaps, estimates/calibration, defects, checklists) may start
when:

1. **Phase 2 is fully done** — every SPEC-002 acceptance criterion green in
   CI on plain Postgres and the Supabase local stack, and the live smoke
   test recorded in [walkthrough-phase-2.md](../walkthrough-phase-2.md) with
   its ledgers and cost inspected by a human. (Both hold as of 2026-07-21.)

2. **A SPEC-003 exists and is approved**, scoped to the command centre. The
   ambitious cut is large; a veteran team would break it into a phase-3
   vertical slice first. Candidate slice (to be decided in SPEC-003):
   - **Web UI** as the primary human surface (vision §7): the initiative
     tree, the live queue and event stream, the checkpoint inbox with
     respond, document review, and cost dashboards — read + the mutations the
     CLI already exposes, over the same HTTP API. Realtime via Supabase
     (behind the `Notifier` boundary, DEC-002).
   - **MCP facet** (vision §7): the read/poke surface for editor assistants
     (`cromwell.status/search/find`, checkpoint respond, dispatch a review) —
     explicitly *not* entity mutation or gate override.
   - **Estimates, sizing, cost calibration** (vision §5): the `estimates`
     table, token estimates with confidence tiers, roll-ups with worst-tier
     propagation, and the actuals→calibration corpus. The dispatch/cost
     ledger that feeds it already exists and is complete.
   - **Milestones and roadmaps** (vision §4): live membership, locking with
     snapshot, gate G4 (milestone-lockable), ordered roadmaps.

3. **Phase-2 carry-overs folded in or explicitly deferred** (from the
   phase-2 audit and SPEC-002 scope decisions):
   - **Graph-project integration** (SD-1, DESIGN-006 §8): `search_graph` and
     per-worktree codebase-memory-mcp indexing. Named-but-unimplemented in
     phase 2; a natural early phase-3 (or phase-2.x) item since the schema
     and ToolContext fields exist.
   - **Agent-driven merge-conflict resolution** (SD-2): human-resolved in
     phase 2; revisit whether an agent rebase is worth it.
   - **Embeddings / semantic retrieval; section role classification** (SD-4):
     deferred from both phases; the `document_sections.embedding` column and
     its migration are still pending.
   - **Defects** (DESIGN-003 §9, vision §14): the triage design is still a
     sketch; it reuses the phase-2 task machinery.
   - **Checkpoint TTL expiry** (DESIGN-002 §3 duty 3): still unexercised.
   - **`cromwell upgrade`** (DESIGN-004 §8): needs a second pack version to
     exist; the pack.lock three-way classification is designed but untested
     against a real upgrade.

## Phase-2 audit summary (against SPEC-002)

Every phase-2 FR is implemented and tested; the deliberate deferrals above
are the only open items, each with a recorded destination. Two design
refinements were made during implementation and are worth carrying into
SPEC-003's review:

- **Code-review diff base** (`tasks.base_commit`, migration 0003): review
  diffs a task's whole contribution across rework, not just the last commit.
- **Multi-tool-turn handling**: the agent loop answers every `tool_use`
  block in a turn (parallel tool calls), required by real providers.

Both were surfaced by the live smoke test and are covered by regression
tests; neither changes a phase-2 contract.
