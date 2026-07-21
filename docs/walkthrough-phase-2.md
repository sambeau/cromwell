# Phase-2 Live Smoke Test — Session Record

**Status:** Complete — satisfies SPEC-002 Definition of Done items 2 and 4
**Date:** 2026-07-21
**Operator:** Sam (commands run by Claude on Sam's behalf)
**Database:** Supabase-hosted Postgres (session pooler, eu-central-1)
**Provider:** DeepSeek via its Anthropic-compatible gateway, model
`deepseek-v4-flash`, for all five roles

This is the real thing: a real Supabase database, a real provider, and — the
point of phase 2 — **real code written, reviewed, verified, and merged by
agents.** The whole loop cost less than half a cent.

## 1. What the test proves

SPEC-002 §1's three claims, end to end:

1. **The tool host makes an agent a safe author** — a live implementer read,
   wrote, and ran commands inside an isolated worktree and produced a real
   diff (33 tool calls across the loop: 15 `read_file`, 12 `list_files`, 5
   `run_command`, 1 `write_file`).
2. **The whole loop runs on escalation-only gates** — an approved spec and
   dev-plan decomposed into a task; the task was implemented, code-reviewed
   (once sent back, once approved), the feature verified and merged, with no
   human decision required at any gate.
3. **A clean-context verifier gated the merge** — a verifier that did not
   write the code checked it against each acceptance criterion, running the
   tests itself for evidence.

## 2. The feature

A deliberately small, real task: add a Python module `greet.py` exposing
`greet() -> "Welcome to Cromwell"`. Three acceptance criteria (file exists,
function defined with no args, returns the exact string). The dev-plan
decomposed it into one task, `T1`. `config.yaml` defined a `run_tests`
command the agents could run:

```yaml
commands:
  run_tests:
    argv: ["python3", "-c", "import greet; assert greet.greet() == 'Welcome to Cromwell'; print('PASS')"]
```

## 3. Planning gates (live reviews)

- **Spec review** — approved by the live reviewer in ~16s. Its reasoning:
  the spec is *"clear, complete, and testable … acceptance criteria are
  simple yes/no checks an implementer or verifier can run without
  interpretation."* Cost $0.0006. Feature stayed `idea` — the spec is only
  half the phase-2 contract (FR-2.1).
- **Dev-plan review** — approved; on approval the orchestrator decomposed the
  task table into task `T1` (audit: `devplan.decomposed`), then G1 (now
  spec AND dev-plan) passed and the feature advanced to `ready`.

## 4. The implementation loop

`cromwell feature start greeting/welcome` created the worktree on branch
`cromwell/greeting/welcome` and dispatched the implementer.

- **Implement (round 1)** — the implementer wrote `greet.py` correctly and
  ran `run_tests` to check itself. Running the tests, though, caused Python
  to generate a `__pycache__/greet.cpython-314.pyc` bytecode file, which the
  server-authored task commit swept in.
- **Code review (round 1) → request_changes.** The reviewer approved the
  substance but caught the artifact:

  > *"Remove `__pycache__/greet.cpython-314.pyc` from the diff. This is a
  > Python bytecode cache artifact that was not requested by the task …
  > out of scope."*

  This is exactly the scope discipline the `review-code` skill teaches —
  correct code, but an out-of-scope file, sent back.
- **Implement (round 2) → code review → approve → verify → merge.** The
  implementer's rework produced no further change; the reviewer approved;
  the verifier ran the tests, read the file, and confirmed all three
  criteria with evidence:

  > *"run_tests executes `assert greet.greet() == 'Welcome to Cromwell'`
  > and prints PASS."*

  On the verifier's approval (G3) the branch merged into `main` and the
  feature reached `done`.

The merged `main` has `greet.py`, and the acceptance check passes:

```
$ python3 -c "import greet; print(greet.greet())"
Welcome to Cromwell
```

## 5. Cost

```
$ cromwell cost
task       …4c4f47a48  dispatches=4  tokens=57638  $0.0023
feature    …0604cc4f47 dispatches=1  tokens=15787  $0.0007   (verification)
document   …41040ec919d dispatches=1  tokens=2964  $0.0006   (spec review)
document   …8ad1a984310 dispatches=1  tokens=2676  $0.0006   (dev-plan review)
total: $0.0043
```

Seven dispatches, every one costed from its frozen price snapshot; the whole
feature — planning through merge — cost **$0.0043**.

## 6. Two bugs this session found (both fixed, both with regression tests)

The value of a live run is that it exercises real provider behaviour the mock
can't. Two genuine bugs surfaced, neither reachable by the (passing)
integration suite:

1. **Parallel tool calls.** DeepSeek returned several `tool_use` blocks in a
   single turn; the agent loop answered only the first, so the next request
   was rejected (`tool_use ids … without tool_result blocks`, HTTP 400). The
   loop now emits one `tool_result` per `tool_use` block. Regression:
   `TestParallelToolCalls` (fails on the old loop). This was the
   `dispatch-failure` checkpoint seen mid-session; after the fix, retrying
   the checkpoint resumed the loop cleanly to `done`.

2. **Code review saw only the last commit.** `taskDiff` diffed
   `HEAD~1..HEAD`. On the rework, the second implement commit was empty, so
   the reviewer saw an empty diff and approved — never seeing the
   `__pycache__` artifact introduced in the task's *first* commit. A reviewer
   that can't see part of the change is a broken gate. Tasks now record a
   `base_commit` at first dispatch (migration 0003); review diffs
   `base..HEAD`, the task's whole contribution across rework.

**Honest note on the merged tree:** the run that reached `done` did so on the
pre-fix code, so the `__pycache__` artifact is present on this smoke
project's `main`. The `base_commit` fix prevents the class going forward (a
project's own `.gitignore` would also exclude it); the merged `greet.py` is
correct and passes its test. The session is recorded as it happened, warts
included.

## 7. Definition of Done status (SPEC-002 §5)

| # | Criterion | Status |
|---|---|---|
| 1 | All FR acceptance criteria pass in CI on plain Postgres and the Supabase local stack | ✅ `go test -race ./...` green on both (Supabase-stack run closed SPEC-001 DoD 1b too) |
| 2 | Live smoke test: a real task implemented → code-reviewed → done, feature verified → merged → done, ledger and cost inspected | ✅ This document |
| 3 | `go vet ./...` and `go test -race ./...` clean | ✅ |
| 4 | Walkthrough records the session | ✅ This document |
| 5 | Phase-3 entry criteria drafted | ✅ [phase-3-entry-criteria.md](notes/phase-3-entry-criteria.md) |

## 8. Operational notes

- Same environment gotchas as phase 1 apply (Supabase session pooler for
  IPv4; short `server.socket` path on macOS).
- The shipped roles default to `claude-sonnet-5`; for this run all five were
  pointed at `deepseek-v4-flash` and the provider set to DeepSeek's gateway —
  a two-line config change, no code change (the `base_url` + one-wire-protocol
  design, D-2 / provider `base_url`).
- Secrets stayed in the environment throughout (`CROMWELL_DATABASE_URL`,
  `DEEPSEEK_API_KEY`); nothing secret was written to `.cromwell/` or git.
