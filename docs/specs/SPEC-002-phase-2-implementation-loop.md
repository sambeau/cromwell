# SPEC-002: Phase 2 — The Implementation Loop

**Status:** Draft for review
**Date:** 2026-07-20
**Parent design:** DESIGN-005 (dev-plans, tasks, decomposition), DESIGN-006 (tool host, worktrees, dispatch); builds on DESIGN-001/002/003/004
**Vision:** [vision-v1](../vision/vision-v1.md) §4, §8, §9
**Precondition:** [phase-2 entry criteria](../notes/phase-2-entry-criteria.md); phase 1 complete ([walkthrough](../walkthrough.md))

## 1. Goal

Prove Cromwell's implementation loop end to end: turn an approved contract
into merged, verified code, with humans gating only on escalation. Phase 1
proved the code orchestrator, escalation-only gates, and the complete ledger
on a read-only path (spec review). Phase 2 proves the three claims that only
a *mutating* path can prove:

1. **The tool host turns an agent into a safe author** — a dispatched
   implementer reads, edits (hash-anchored), and runs whitelisted commands
   inside an isolated worktree, unable to touch anything outside it, and
   produces a real diff (vision §8).
2. **The whole loop runs on escalation-only gates** — an approved spec and
   dev-plan decompose into tasks; tasks are implemented, code-reviewed, and
   the feature verified and merged, with no human in the loop unless a
   reviewer or verifier escalates (vision §9).
3. **Close-out verification gates the merge** — a clean-context verifier that
   did not write the code checks the feature against its acceptance criteria
   before it merges (DESIGN-003 §7, L-4).

Phase 2 is complete when a user can take a feature that already has an
approved spec, write and approve a dev-plan, watch it decompose into tasks,
watch implementer agents produce and get code-review approval on real diffs,
watch a verifier check the feature and merge it, and see the full audit
trail, the tool-call ledger, and the exact cost of every dispatch.

## 2. Scope

### In scope

- **Dev-plan document type**: `templates/dev_plan/` (template, manifest with
  the `table_parses` rule, `dev-plan-reviewer` role); full document lifecycle
  and agent-review, reusing phase-1 machinery
- **G1 extension**: contract-approved now requires spec AND dev-plan
  (DESIGN-005 §3 — flips the phase-1 flag)
- **Decomposition**: dev-plan approval creates tasks from its task table;
  re-decomposition on revision with the non-destructive reconciliation rules
  (DESIGN-005 §4)
- **Task lifecycle**: `pending → ready → active → review → done` / `abandoned`,
  dependency-driven readiness (DESIGN-005 §5)
- **Worktrees**: created on `ready → active` (human start), GC'd on terminal
  states, reconciled on boot (DESIGN-006 §3)
- **Tool host**: registry, per-role profiles enforced twice, ToolContext
  injection, path jailing, hash-anchored `read_file`/`edit_file`,
  `write_file`, `list_files`, `run_command` over the `commands:` whitelist,
  `tool_calls` ledger (DESIGN-006 §4)
- **Implementer dispatch** (`implement-task`): `submit_implementation`
  outcome, server-authored task commit, governor check 3 (per-feature
  mutating serialisation) made real (DESIGN-006 §5)
- **Code review** (`review-code`): reuses `submit_review` and the three
  verdicts against the diff; `request_changes` re-dispatches the implementer
  against the kept worktree (DESIGN-006 §5)
- **Verification and merge** (`verify-feature`, gate G3): clean-context
  verifier, `submit_verification` outcome, merge-then-done, unmet criteria
  become tasks, merge-conflict checkpoint (DESIGN-006 §6)
- **Revision-in-flight blocking**: `spec_stale`, the `revision-in-flight`
  checkpoint, re-decomposition mismatch surfacing (DESIGN-005 §6)
- **Migration `0002`**: `tasks` and `worktrees` tables, `features.spec_stale`,
  `tasks.local_id`
- **CLI additions**: `task list`, `feature start`, and the new checkpoint
  kinds surfaced in `inbox`/`respond`

### Out of scope (deferred, with destination)

| Deferred | To |
|---|---|
| Graph-project integration (`search_graph`, per-worktree indexing) | Phase 2.x (DESIGN-006 §8, SD-1) |
| Agent-driven merge-conflict resolution | Later (DESIGN-006 §6, SD-2) |
| Embeddings / semantic retrieval; section role classification | Later in phase 2 or phase 3 (direct attachments suffice for the slice, as in SPEC-001) |
| Estimates, sizing, calibration | Phase 3 |
| Web UI, MCP facet | Phase 3 |
| Milestones, roadmaps; defects, jobs, checklists | Phase 3 |
| `cromwell upgrade`; kanbanzai migration | Later |

### Scope decisions

- **SD-1: Graph-project integration deferred.** `search_graph` is
  named-but-unimplemented; declaring it in a role profile is a config error
  until it lands. The `worktrees.graph_project` column and the ToolContext
  field exist now, so the later integration changes an implementation, not a
  contract (DESIGN-006 §8, TH-9).
- **SD-2: Merge conflicts are human-resolved.** A non-clean merge raises a
  `merge-conflict` checkpoint; no forced merge. Agent rebasing is a later
  refinement (DESIGN-006 §6, TH-8).
- **SD-3: One provider, three roles.** Anthropic-compatible endpoints only
  (inherits SPEC-001 D-2). Implementer, code-reviewer, and verifier are
  distinct roles that may use distinct models via `assignments:` and
  `routing:` — the config surface for this already exists (DESIGN-004 §4–5).
- **SD-4: Direct attachments only.** As in SPEC-001, prompt assembly uses
  direct attachment and ancestry, not embeddings — sufficient to prove the
  loop; semantic retrieval is deferred.

## 3. Requirements

### FR-1: Dev-plan document type

- **FR-1.1** The starter pack ships `templates/dev_plan/` (template,
  manifest with `reviewer_role: dev-plan-reviewer` and a `table_parses` rule
  over the `Tasks` section), a `dev-plan-reviewer` role, and a
  `review-dev-plan` skill. `cromwell upgrade` does not exist yet, so these
  are added to the pack and appear in a fresh `init`.
  *AC:* after `init`, `doc add … --type dev_plan` registers a dev-plan;
  validation runs the `table_parses` rule.
- **FR-1.2** `table_parses` validates the task table mechanically: present,
  well-formed, fixed columns (`id, title, depends_on, description`), unique
  `id`s, every `depends_on` id present, no cycles. Failure blocks submit and
  names the defect.
  *AC:* a dev-plan with a dependency cycle is rejected at submit with the
  cycle named; a well-formed one submits and dispatches the reviewer.
- **FR-1.3** Dev-plan review reuses the phase-1 flow with the feature's
  approved spec as a background document.
  *AC:* the assembled reviewer prompt contains the approved spec; the three
  verdicts behave exactly as for specs (integration-tested, mock provider).
- **FR-1.4** The starter pack ships the phase-2 execution roles and their
  skills — `implementer`, `code-reviewer`, `verifier` — each with a tool
  profile (DESIGN-006 §4.4), and `config.yaml` ships an `assignments:` map
  binding the purposes `implement-task → implementer`, `review-code →
  code-reviewer`, `verify-feature → verifier` (DESIGN-004 §5 F-4). Their
  content is authored and reviewed like code (§6); the tool profiles are
  validated at load (a `verifier` with a mutating tool is a config error).
  *AC:* a fresh `init` yields the three roles, their skills, and the
  `assignments:` entries; loading a verifier profile containing `edit_file`
  fails with a config error naming the file.

### FR-2: Contract gate (G1 extended)

- **FR-2.1** G1 now requires spec `approved` AND dev-plan `approved`;
  `idea → ready` fires only when both hold.
  *AC:* approving only the spec leaves the feature `idea`; approving the
  dev-plan then advances it to `ready`, with a `gate.evaluated {gate: G1,
  pass: true}` row citing both documents.

### FR-3: Decomposition

- **FR-3.1** On dev-plan approval, tasks are created from the task table
  before G1 advances the feature; each task records its dev-plan-local `id`
  in `local_id`, its `depends_on` resolved to task UUIDs, `state = pending`.
  *AC:* approving a dev-plan with N table rows yields N `pending` tasks with
  correct dependencies; a `devplan.decomposed {count: N}` audit row precedes
  the G1 evaluation.
- **FR-3.2** Re-decomposition (successor dev-plan approved) reconciles
  non-destructively: `pending`/`ready` tasks updated or deleted to match the
  new table; `active`/`review`/`done` tasks with no matching row are kept and
  surfaced as mismatches.
  *AC:* revising a dev-plan to drop a not-yet-started task removes it;
  revising to drop a `done` task keeps it and lists it in the
  `revision-in-flight` checkpoint.

### FR-4: Task lifecycle and readiness

- **FR-4.1** Tasks follow `pending → ready → active → review → done` /
  `abandoned` (DESIGN-005 §5); illegal transitions are typed rejections, no
  force flag (as for documents/features in FR-5.1 of SPEC-001).
  *AC:* table-driven unit tests cover the task transition matrix.
- **FR-4.2** Readiness is automatic: a task with all `depends_on` tasks
  `done` becomes `ready`; completing a task re-evaluates its dependents.
  Only `ready` tasks of an `active`, non-`spec_stale` feature are
  dispatchable.
  *AC:* a two-task chain (T2 depends on T1) dispatches T1 first; T2 becomes
  `ready` only when T1 is `done`.
- **FR-4.3** `cromwell task list <feature-path>` shows tasks with state,
  dependencies, and the owning dispatch if active.
  *AC:* after decomposition, `task list` shows all tasks and their states.

### FR-5: Worktrees

- **FR-5.1** `cromwell feature start <path>` (or auto-start if configured)
  transitions `ready → active` and creates a worktree (branch + directory)
  in the same transaction as the state change, with the git op following the
  commit; failure raises a checkpoint, never a half-started feature.
  *AC:* starting a `ready` feature creates a `worktrees` row and a git branch
  `cromwell/<initiative>/<feature>`; a forced git failure leaves the feature
  `ready` and raises a checkpoint.
- **FR-5.2** Reaching a terminal state GC's the worktree (heartbeat), stamps
  `removed_at`, and leaves an abandoned feature's branch intact.
  *AC:* a merged feature's worktree is removed within one heartbeat; the
  branch of an abandoned feature still exists.
- **FR-5.3** On boot, `worktrees` rows are reconciled against `git worktree
  list`; a row with no worktree (crash before git op) is re-created.
  *AC:* deleting a worktree directory out of band and restarting re-creates
  it for a still-active feature.

### FR-6: Tool host

- **FR-6.1** The agent loop offers the role's declared tool profile plus the
  purpose's outcome tool; the host rejects any call outside the profile.
  *AC:* a verifier role with `edit_file` in its profile is a config error;
  an agent that calls a tool outside its profile gets a tool error, and the
  attempt is audited.
- **FR-6.2** Path arguments resolve under the worktree root and are
  jail-checked (including via symlink); an escaping path is a tool error.
  *AC:* `read_file("../../etc/passwd")` and a symlink-out both return a tool
  error; the worktree root is never influenced by the agent.
- **FR-6.3** `read_file(hash_tag: true)` returns line-anchored content;
  `edit_file(hash_ref, new_text)` fails if the anchor drifted since the read.
  *AC:* editing with a stale anchor fails with a re-read message; editing
  with a current anchor succeeds and the change lands in the worktree.
- **FR-6.4** `run_command(name)` runs only whitelisted commands from
  `commands:` in the worktree, with timeout and output cap; unknown command,
  timeout, and non-zero exit are all tool results, not crashes.
  *AC:* `run_command("run_tests")` runs the configured argv; a name not in
  `commands:` returns a tool error; output past the cap is truncated.
- **FR-6.5** Every tool call writes a `tool_calls` row (tool, sizes, latency,
  status).
  *AC:* after an implement-task dispatch, `tool_calls` has one row per call
  with correct status values.

### FR-7: Implementer dispatch

- **FR-7.1** `implement-task` assembles spec + dev-plan + the task + tool
  profile, runs the tool host until `submit_implementation`, commits the
  worktree changes to the feature branch (server-authored, message names the
  task), and transitions the task `active → review`.
  *AC:* an implement-task dispatch (mock provider scripting edits + outcome)
  produces a commit on the branch and moves the task to `review`.
- **FR-7.2** Governor check 3: at most one mutating dispatch per feature at a
  time; read-only dispatches (review, verify) remain parallel-exempt.
  *AC:* two `ready` tasks of one feature do not run implementers
  concurrently; two features' implementers do run concurrently.

### FR-8: Code review

- **FR-8.1** `review-code` dispatches the `code-reviewer` read-only against
  the task diff with the spec and dev-plan; `submit_review` verdicts:
  `approve` → task `done` and readiness re-eval; `request_changes` → task
  `active`, comments attached, implementer re-dispatched against the same
  worktree; `escalate` → checkpoint.
  *AC:* one integration test per verdict (mock provider); the
  `request_changes` path amends the diff rather than restarting.

### FR-9: Verification and merge (G3)

- **FR-9.1** When G2 passes (all tasks `done`/`abandoned`, ≥1 `done`, NOT
  `spec_stale`), the feature enters `review` and a clean-context `verifier`
  is dispatched with the spec's acceptance criteria and read-only tools (no
  dev-plan, no implementation narrative).
  *AC:* the verifier prompt contains the acceptance criteria and not the
  dev-plan; its tool profile has no `edit_file`/`write_file`.
- **FR-9.2** `submit_verification` verdicts: `approve` (G3) → merge the
  branch to main (server-authored), feature `done`, worktree GC'd;
  `request_changes` → unmet criteria become tasks, feature `active`;
  `escalate` → checkpoint.
  *AC:* approve merges the branch and marks the feature `done` with the
  branch in main; request_changes creates tasks for the unmet criteria and
  returns the feature to `active`.
- **FR-9.3** A non-clean merge raises a `merge-conflict` checkpoint naming
  the conflicting paths; the feature stays `review`, no forced merge.
  *AC:* advancing main under an in-flight feature so its branch conflicts,
  then approving verification, raises the checkpoint and does not merge.

### FR-10: Revision-in-flight blocking

- **FR-10.1** Submitting a successor spec or dev-plan for a feature in
  `active`/`review` sets `spec_stale = true` (audited), blocks new task
  dispatches, and raises a `revision-in-flight` checkpoint listing in-flight
  and completed tasks with a pause/continue choice.
  *AC:* revising a spec of an active feature stops new task dispatches within
  one event cycle and raises the checkpoint; running dispatches finish.
- **FR-10.2** On the successor's approval, re-decomposition runs (FR-3.2) and
  `spec_stale` clears once the checkpoint is resolved.
  *AC:* after approval and checkpoint resolution, the feature dispatches
  tasks again and `spec_stale` is false.

## 4. Non-functional requirements

- **NFR-1** The tool host's pure logic (path jailing, hash anchoring,
  command lookup, profile enforcement) is unit-tested without a provider or a
  real worktree where practical; the agent loop is integration-tested with
  the mock provider driving scripted tool calls.
- **NFR-2** `go vet ./...` and `go test -race ./...` clean; CI runs the suite
  against plain Postgres (DEC-002) and — closing SPEC-001's remaining DoD
  item — the Supabase local stack.
- **NFR-3** Migration `0002` creates only phase-2 tables/columns (`tasks`,
  `worktrees`, `features.spec_stale`, `tasks.local_id`); no changes to
  phase-1 tables beyond the added column; forward-only (DESIGN-001 §11).
- **NFR-4** No secrets in git, `.cromwell/`, the audit log, tool-call
  results, or error messages (inherits SPEC-001 NFR-4); worktree paths and
  branch names contain no secret material.
- **NFR-5** Worktree operations never corrupt the main repo: all git
  mutations are in worktrees or server-authored commits/merges; a failed git
  op leaves a recoverable state and a checkpoint (DESIGN-006 §3, §6).

## 5. Definition of Done

1. All FR acceptance criteria pass in CI (mock provider) on plain Postgres
   and the Supabase local stack (this also closes SPEC-001 DoD 1b / FR-1.3).
2. The live smoke test extends the phase-1 walkthrough: for a feature with an
   approved spec, write and approve a dev-plan, watch decomposition, and take
   at least one real task through implement → code-review → done and the
   feature through verify → merge → done against a live provider, with the
   tool-call ledger and per-dispatch cost inspected by a human. Recorded in
   `docs/walkthrough.md` (or `walkthrough-phase-2.md`).
3. `go vet ./...` and `go test -race ./...` clean.
4. The walkthrough records the session: every command, each dispatch, the
   diffs produced, the audit and tool-call ledgers, the cost.
5. Phase-3 entry criteria drafted (command centre: web UI, MCP facet,
   milestones, estimates, defects).

## 6. Open questions carried into implementation

- **Implementer skill quality** is as load-bearing here as the reviewer
  skill was in phase 1: it drives how often implementers succeed without
  escalation, and how cleanly. It is authored during implementation and
  reviewed like code (as the phase-1 reviewer content was).
- **Task commit granularity** (one commit per implement-task dispatch vs. per
  outcome) is fixed at one-commit-per-dispatch here; whether re-dispatch on
  `request_changes` amends via a new commit or `--amend` is an implementation
  detail settled during build, not a contract.
- **Verifier evidence format** (`criteria: [{id, met, evidence}]`) may gain
  structure (e.g. a command-output reference) once real verifications run;
  the outcome-tool schema is versionable without a contract change.
