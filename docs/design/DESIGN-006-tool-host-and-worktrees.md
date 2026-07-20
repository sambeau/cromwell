# DESIGN-006: The Tool Host, Worktrees, and Implementation Dispatch

**Status:** Draft for review
**Date:** 2026-07-20
**Parent:** [vision-v1](../vision/vision-v1.md) §8 (tool surface); [DESIGN-002](DESIGN-002-orchestrator.md) §4–5 (dispatch, tool host sketch)
**Depends on:** DESIGN-002 (dispatch pipeline), DESIGN-005 (tasks), DESIGN-004 (role tool profiles, `commands:`)

## 1. Purpose

Makes concrete the tool surface DESIGN-002 §5 sketched: the mechanism by
which a dispatched agent *does things* — reads and edits files in an
isolated worktree, runs whitelisted commands — under server control. It
defines the worktree lifecycle, the tool registry and its jailing and
hash-anchoring, the mutating-dispatch governor lock, the implementer and
verifier dispatch purposes, code review of diffs, and merge.

This is the largest new surface in phase 2 and the reason phase 2 exists: it
is what turns an approved contract into merged code. Everything here builds
on the phase-1 dispatch pipeline (DESIGN-002 §4) unchanged — the agent loop,
the outcome-tool discipline, the ledger, retries, and crash recovery already
exist. What is new is that the loop now offers the agent *more than one
tool*, and those tools have side effects on a worktree.

## 2. The shape of an implementation dispatch

```
feature ready ──(human start, L-5)──▶ worktree created (branch + dir)
                                              │
              ┌───────────── per ready task ──┴───────────────┐
              ▼                                                 │
   implement-task dispatch                                      │
   (implementer role, tool host: read_file/edit_file/          │
    run_command + submit_implementation)                        │
              │ diff on the feature branch                      │
              ▼                                                 │
   review-code dispatch (reviewer, read-only + submit_review)   │
     approve ─▶ task done ─▶ readiness re-eval ────────────────┘ (next tasks)
     request_changes ─▶ task active, implementer re-dispatched (worktree kept)
              │
   all tasks done (G2) ─▶ feature review
              ▼
   verify-feature dispatch (verifier, clean context, read-only + submit_verification)
     approve (G3) ─▶ merge branch to main ─▶ feature done ─▶ worktree GC
     request_changes ─▶ unmet criteria become tasks ─▶ feature active
```

Every box is a phase-1-style dispatch: claimed under the governor, prompt
assembled deterministically, run through the tool host until its outcome
tool fires, recorded in the ledger with cost. The only structural novelty is
the tool host between the model and the world.

## 3. Worktrees

A worktree isolates one feature's work (vision §8: "each Feature has its own
branch and directory; work on different Features can't conflict").

**Creation.** When a feature transitions `ready → active` (the human `start`
action, L-5), the lifecycle engine creates a worktree in the same
transaction as the state change:

1. `git worktree add <path> -b <branch>` off the current main, where
   `<branch> = cromwell/<initiative-path>/<feature-slug>` and `<path>` is
   under a configurable worktrees root (default `.cromwell/worktrees/`,
   gitignored).
2. Insert a `worktrees` row (DESIGN-001 §9): `feature_id`, `path`, `branch`,
   `graph_project` (NULL in phase 2 — see §8), `created_at`.
3. Record `features.branch = <branch>`.

The git operation is a side effect *after* the DB commit, mirroring the
revision file-takeover pattern (DESIGN-003 §5, as implemented): if the git
step fails, a checkpoint surfaces it rather than leaving a half-started
feature. Worktree creation is idempotent by `feature_id` — a crash between
commit and git op is repaired on boot by reconciling worktrees rows against
`git worktree list`.

**Teardown.** When a feature reaches a terminal state (`done` after merge, or
`abandoned`), the heartbeat GC (DESIGN-002 §3 duty 3) removes the worktree
(`git worktree remove`), tears down its graph project if any (§8), and stamps
`worktrees.removed_at`. Teardown never discards unmerged work silently: an
`abandoned` feature's branch is left in the repo (not deleted) and the
checkpoint that abandoned it records the branch name.

## 4. The tool host

The tool host is the server component that executes tool calls on behalf of a
dispatched agent (DESIGN-002 §5). Phase 1 shipped exactly one tool per
dispatch — the outcome tool. Phase 2 adds ordinary tools and the machinery
that keeps them safe.

### 4.1 The agent loop, generalised

The phase-1 loop (DESIGN-002 §4 step 5) already alternates model turn → tool
call → tool result → model turn until the outcome tool fires or the turn cap
is hit. Phase 2 changes only what tools are offered: the dispatcher hands the
model the role's declared tool profile (DESIGN-004 §5, `tools:`) plus the
purpose's outcome tool. When the model calls a non-outcome tool, the host
executes it and returns the result as the next turn's input; when it calls
the outcome tool, the dispatch completes. Free-text turns without a tool call
are nudged, exactly as in phase 1.

### 4.2 ToolContext and injection

Each dispatch carries a `ToolContext` the orchestrator populates and the
agent never sees (vision §8, "implicit context injection"):

```
ToolContext {
  WorktreeRoot  string     // absolute; every path argument resolves under this
  FeatureID     uuid
  TaskID        uuid       // the task being implemented (implement-task only)
  Commands      map[string]CommandSpec  // whitelisted argv templates from config
}
```

The agent calls `edit_file(path, ...)` with a repo-relative path; the host
resolves it against `WorktreeRoot`. The agent never passes, and cannot
influence, which worktree it operates in. This eliminates a whole class of
"agent targeted the wrong scope" errors and saves tokens on every call
(vision §8).

### 4.3 Path jailing

Every path argument is resolved to an absolute path and checked to lie under
`WorktreeRoot` (the phase-1 `readDocFile` jail, generalised and already
proven). A resolved path outside the worktree — via `..`, an absolute path,
or a symlink — is a tool **error** returned to the agent, not an escalation
and not a crash (DESIGN-002 §5). Symlinks are resolved before the check so a
symlink inside the worktree pointing out is caught.

### 4.4 The phase-2 tool set

| Tool | Profile | Effect |
|---|---|---|
| `read_file(path, hash_tag?)` | implementer, reviewer, verifier | Returns file contents; with `hash_tag`, each line prefixed `NN#hash\| ` |
| `list_files(dir?)` | all | Lists worktree files (git-tracked + untracked, respecting .gitignore) |
| `edit_file(path, hash_ref, new_text)` | implementer only | Replaces the anchored line(s); fails if the anchor drifted (§4.5) |
| `write_file(path, content)` | implementer only | Creates or overwrites a whole file (new files, no anchor) |
| `run_command(name, args?)` | implementer, verifier | Runs a whitelisted command (§4.6) in the worktree |
| `submit_implementation(summary, files_changed[])` | implementer outcome | Completes an implement-task dispatch |
| `submit_review(verdict, comments[], reasoning)` | reviewer outcome | Phase-1 tool, reused for code review |
| `submit_verification(criteria[], verdict, reasoning)` | verifier outcome | Completes a verify-feature dispatch |

Profiles are declared per role in `.cromwell/roles/` (DESIGN-004 §5) and
enforced twice: the dispatcher offers the model only the profile's tools, and
the host rejects any call outside the profile (defence in depth, DESIGN-002
§5). A `reviewer` has no `edit_file`; a `verifier` has no `edit_file` and no
`write_file` — verification is read-only by construction (DESIGN-003 §7).

### 4.5 Hash-anchored edits

The kanbanzai read/edit protocol (vision §8), carried forward: `read_file`
with `hash_tag: true` returns each line as `NN#hhhh| content`, where `hhhh` is
a short hash of the line's bytes. `edit_file(hash_ref: "47#a3f9", new_text)`
validates that line 47 still hashes to `a3f9` before replacing it; if the
file changed since the read (another turn, a `run_command` that rewrote it),
the anchor mismatches and the edit fails with a message telling the agent to
re-read. This sidesteps stale-read races without forcing large context quotes
(vision §8). Role tool-hints (DESIGN-004 §5) steer implementers toward
`read_file(hash_tag: true)` before editing; `write_file` remains for new
files where there is nothing to anchor.

### 4.6 Command whitelist

There is no generic shell (vision §8). `run_command(name, args?)` looks
`name` up in the `commands:` map from `config.yaml` (DESIGN-004 §4, shape
reserved there) — each entry a fixed argv template, a timeout, and an output
cap:

```yaml
commands:
  run_tests:   {argv: ["go", "test", "./..."], timeout_seconds: 600, output_cap_bytes: 65536}
  build:       {argv: ["go", "build", "./..."], timeout_seconds: 300}
  gofmt:       {argv: ["gofmt", "-w", "."],     timeout_seconds: 60}
```

`args`, when the template permits, are appended positionally and are
themselves jail-checked if they name paths. A command not in the map is a
tool error. Commands run in the worktree with a timeout and truncated output
(the cap protects the context window and the ledger). An unknown command, a
timeout, or a non-zero exit are all returned to the agent as tool results —
the agent decides what to do, and the outcome tool is still how the dispatch
ends.

### 4.7 Ledger

Every tool call writes a `tool_calls` row (DESIGN-001 §8, already in the
phase-1 schema): dispatch, sequence, tool name, argument and result byte
sizes, latency, status (`ok` | `error` | `retried`). This is the proxy-metric
substrate vision §11 describes; phase 2 populates it for the first time.

## 5. Implementer dispatch and code review

**Purpose `implement-task`.** Bound to an `implementer` role via
`config.yaml` `assignments:` (DESIGN-004 §5, F-4 — the non-document
resolution path, empty in phase 1, used here). Prompt assembly (DESIGN-002 §4
step 3) offers: the implementer identity and skill, the feature's approved
spec and dev-plan (direct attachments), the specific task and its
description, and the tool profile. The implementer reads, edits, runs the
build/tests via `run_command`, and calls `submit_implementation(summary,
files_changed[])` when the task is complete. On completion the host commits
the worktree's changes to the feature branch with a message naming the task
(a server-authored commit, as with revisions), and the task transitions
`active → review`.

**Governor check 3 — within-feature mutating serialisation.** DESIGN-002 §6
check 3 ("at most one mutating dispatch per feature at a time") was a stub in
phase 1 (all phase-1 dispatches were read-only). Phase 2 makes it real: an
`implement-task` dispatch is mutating; the governor admits at most one per
feature at a time, so two implementers never edit the same worktree
concurrently. Read-only dispatches (code review, verification) remain exempt
and run in parallel (O-5). This is why tasks within a feature serialise while
different features run fully in parallel.

**Code review (`review-code`).** When a task enters `review`, the
orchestrator dispatches the `code-reviewer` role read-only against the diff.
Prompt assembly offers the spec, the dev-plan, the task, and the task's diff
(`git diff` of the task's commit against the branch point), plus a read-only
tool profile so the reviewer can inspect surrounding code. Outcome is the
phase-1 `submit_review` with the same three verdicts (DP-6):

- `approve` → task `done`; readiness re-evaluation (DESIGN-005 §5) may make
  dependent tasks `ready`.
- `request_changes` → task back to `active`, comments attached, implementer
  re-dispatched against the same worktree (the diff is amended).
- `escalate` → checkpoint.

## 6. Verification and merge (gate G3)

DESIGN-003 §7 defined close-out verification; phase 2 implements it.

When the last task of a feature reaches `done`, G2 passes (all tasks
`done`/`abandoned`, at least one `done`, NOT `spec_stale` — DESIGN-003 §8)
and the feature enters `review`. The orchestrator dispatches the `verifier`
role with **clean context** — it receives the spec's acceptance criteria as
its checklist and read-only tools, but *not* the dev-plan or the
implementation narrative: an agent that did not write the code checks the
result against the contract (DESIGN-003 §7, L-4). Outcome tool
`submit_verification(criteria: [{id, met, evidence}], verdict, reasoning)`:

- `approve` (G3) → the lifecycle engine merges the feature branch into main
  (a server-authored merge commit), transitions the feature to `done`, and
  the heartbeat GC tears down the worktree. Merge happens *after* verification
  approval and *before* `done` (DESIGN-003 §7).
- `request_changes` → each unmet criterion becomes a new task (appended to
  the feature, `pending`/`ready` per its deps), the feature returns to
  `active`, and the loop resumes.
- `escalate` → checkpoint.

**Merge conflicts.** If the feature branch does not merge cleanly into main
(main advanced under it), the merge is not forced: a `merge-conflict`
checkpoint is raised naming the conflicting paths, and the feature stays in
`review`. Resolving conflicts is a human action in phase 2 (the human merges
main into the branch and responds); an agent-driven rebase is a later
refinement, not load-bearing for the slice.

## 7. Failure, crash, and idempotency

All of phase 1's resilience machinery (DESIGN-002 §8) applies unchanged,
with these phase-2 specifics:

- **Idempotency keys** extend the phase-1 scheme: `implement:<task_id>:<n>`
  where `n` is the task's re-dispatch count, `review-code:<task_id>:<commit>`,
  `verify:<feature_id>:<branch_head>`. A crash mid-implementation re-queues
  the same key; the partial worktree edits are discarded by resetting the
  branch to its last committed state before the retry (fresh context each
  attempt, DESIGN-002 §8).
- **Worktree reconciliation on boot** (DESIGN-002 §8 catch-up, extended):
  `worktrees` rows are reconciled against `git worktree list` — a row with no
  worktree (crash before git op) triggers re-creation; a worktree with no row
  (crash before DB commit — impossible given commit-then-git ordering, but
  checked) is logged.
- **The tool host is stateless** across turns except for the worktree on
  disk, which is durable; a dispatch that dies mid-turn loses only its
  in-flight model call, and the retry re-reads the (committed) worktree
  state.

## 8. Deferred: the graph-project integration

Vision §8 lists per-worktree codebase-memory-mcp graph projects as an
"inherited investment": the orchestrator derives a project name from the
worktree, indexes on creation, tears down on GC, and exposes `search_graph`
to agents. **This is deliberately deferred past the phase-2 slice** (scope
decision in SPEC-002). The reasoning:

- It is an *optimization on a working tool host*, not a load-bearing part of
  proving the implementation loop. An implementer with `read_file`,
  `list_files`, and `run_command` can complete real tasks; `search_graph`
  makes it cheaper and faster, which matters at scale but proves nothing new
  in the slice.
- The `worktrees.graph_project` column and the ToolContext field exist from
  the start (they are in the schema and the struct), so adding the
  integration later changes an implementation, not a contract — the true
  subset property the Bootstrap Principle demands.

`search_graph` is therefore a named-but-unimplemented tool in phase 2:
declaring it in a role profile is a config error until the integration
lands, exactly as an unknown tool would be.

## 9. What this document does not cover

- **Estimates and calibration** on tasks/dispatches (vision §5) — phase 3.
- **The graph-project integration** (§8) — a phase-2.x increment.
- **Agent-driven merge-conflict resolution** (§6) — human-resolved in phase 2.
- **Multi-agent collaboration** (an implementer querying a reviewer mid-task,
  vision §8's ACP note) — explicitly out of scope, as the vision states.

## 10. Decisions recorded here

| # | Decision | Rationale |
|---|---|---|
| TH-1 | The phase-1 agent loop generalises to multiple tools with no structural change; only the offered tool set grows | The loop, outcome-tool discipline, ledger, and retries are already proven; phase 2 adds tools, not a new pipeline |
| TH-2 | ToolContext is injected; agents pass repo-relative paths and never name a worktree | Eliminates wrong-scope errors, saves tokens (vision §8); the phase-1 path jail generalises directly |
| TH-3 | Two mutating tools (`edit_file` hash-anchored, `write_file` for new files); verifier and reviewer get neither | Read/write separation is a tool-profile fact, enforced twice; verification read-only by construction (L-4) |
| TH-4 | No generic shell; `run_command` dispatches fixed argv templates from `commands:` | vision §8's footgun avoidance; unusual actions must escalate |
| TH-5 | Governor check 3 (per-feature mutating serialisation) becomes real; reviews/verification stay parallel-exempt | One worktree, one writer; O-5's read-only exemption still holds |
| TH-6 | Implementer completion, task commits, and the final merge are server-authored git operations after their DB commit | Same commit-then-git ordering proven by the revision takeover; failures surface as checkpoints, never half-states |
| TH-7 | Verifier runs clean-context, read-only, against acceptance criteria only | DESIGN-003 §7 / L-4: the checker did not write the code and judges the contract, not the implementation story |
| TH-8 | Merge conflicts raise a checkpoint; no forced merge, human-resolved in phase 2 | Honest handling of a real hazard; agent rebasing is a later refinement, not load-bearing |
| TH-9 | Graph-project integration deferred; `search_graph` named-but-unimplemented, schema/context fields present | It optimizes a working tool host rather than proving the loop; deferring it keeps the slice a true subset (Bootstrap Principle) |
