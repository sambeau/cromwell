# REVIEW-020 — M13 Executors, code review round 2: quality

Scope: `git diff 2a799f8..HEAD` (31 files, plus the two T7 test files `integration_executors_e2e_test.go` and `integration_executors_races_test.go`). Round 1 is `REVIEW-020-code-round1-quality.md`.

Tooling: `gofmt -l internal cmd` prints nothing and `go vet ./...` reports nothing.

## Round-1 findings

| # | Sev | Claimed | Verdict | Note |
|---|-----|---------|---------|------|
| Q1 | major | fixed | Fixed | `actions_phase2.go` now uses `headOf` and a new `featureHead(ctx, featureID)`, which `dispatchReadyTasks` and `branchHeadForTask` both call. Both moved next to `gitIn` in `planner.go`, and `headOf`'s comment says why `worktreeFingerprint` differs (`--no-optional-locks`). The only remaining raw `rev-parse HEAD` sites are in other features' code (`documents.go`, `identity.go`), which is out of scope. |
| Q2 | major | fixed | Fixed | `TaskUnmeasured`, `DeleteExecutionsFor`, `RunningImplementInFeature` and `MutatingDispatchActiveForTask` are gone, and nothing references them. |
| Q3 | major | fixed | Fixed | `MarkDispatchExecutionSubmitted` and `MarkClaimExecutionSubmitted` over one private `markExecutionSubmitted`. The "neither given" branch is gone. Well done. |
| Q4 | major | fixed | Fixed | No `time.Sleep` remains in `integration_claim_sweep_test.go`. |
| Q5 | major | fixed | Fixed | `TestWithWorkingCopyIsOneAtATimePerPath` is now a unit test using channel handshakes and `TryLock`, and it checks that a second path is not held up. The store test no longer sleeps. One nit in Q17. |
| Q6 | minor | fixed | Fixed, clumsily | The direct `&ClaimRefusal{}` literals are gone, but every call is now `refuse("%s", sentence)` (see Q16). |
| Q7 | minor | fixed | Mostly | `claimIDOr` is now `parseClaimID`, with a comment. `execWho` is removed. `nilIfNotFound` is renamed `claimOrNil` and documented honestly, though it still swallows every error, not only not-found. `taskSuffix` stays, which is right because `claims_task.go:166` also uses it. Accepted. |
| Q8 | minor | fixed | Fixed | The doc comments of `ago` and `agoWords` point at each other, and both sit together in `ui.go`. |
| Q9 | minor | fixed | Fixed | `admit` uses one deferred release guarded by `started`; the four `<-dp.workers` exits are gone. |
| Q10 | minor | left | Accepted | `claimLocked` and `watchBranchLocked` are still long, and phase-commented. Not blocking. |
| Q11 | minor | left | Partly done, accepted | The three unit tests moved to `claims_test.go`, and `integration_claims_test.go` is down from 1139 to 990 lines. The shared helpers (`writeIn`, `gitOut`, `commitAs`, `asMap`, `htmlHas`) are still in feature-named files, and `seedFeature2` keeps its name. See Q22 for new instances. |
| Q12 | minor | left | Accepted | The sweeps already log through `s.Log` at their per-item error sites; error wrapping is still uneven in the claim service. Not blocking. |
| Q13 | minor | fixed | Fixed | `claimResultMap` is extracted and the handler reads as parse, call, shape. |
| Q14 | minor | fixed | Fixed | `{{define "time-tag"}}` replaces the four repeated layouts, and the CSS rules for `claim-facts`, `claim-comments` and `task-executor*` now exist in `app.css`, using existing tokens (`--sp-*`, `--bw`, `--c-line`). |
| Q15 | minor | fixed | Fixed | The `claimable` comment is trimmed, and `shortHash` is documented. `taskClaims`, `mcpClaimTools` and `mcpClaimTask` have doc comments too. |

## New findings

None is major.

### Q16 (minor) — `refuse("%s", x)` repeated nine times
`claims.go` (`resolveClaimRef`, `claimLocked` twice, `submitLocked` twice and others) now calls `refuse("%s", sentence)` wherever it holds a finished sentence. It works, but the `"%s"` is noise at every site and `go vet` printf checks treat it as a format wrapper. Add `refusal(sentence string) error` (and have `refuse` call it), or have `refuse` accept a plain sentence when it has no args.

### Q17 (minor) — an inverted empty branch in the lock test
`claims_test.go:74-` has `if l := ...; !l.mu.TryLock() { // Held, as it should be. } else { l.mu.Unlock(); t.Fatal(...) }`. Write it positively: `if l.mu.TryLock() { l.mu.Unlock(); t.Fatal(...) }`.

### Q18 (minor) — `CancelIfTaskClaimed` doc comment left as one over-long line
`store/dispatches.go`: the edit appended "(whether or not this call was the one to cancel it)" without re-wrapping, so one comment line runs to about 140 columns. The rest of the file wraps at about 80. Also, `claimActivityWords` in `claims.go` maps five of its seven cases to themselves; a `map[string]string` of only the three differing words, with the activity as the fallback, would be shorter. The fallback is now the vague "something else" rather than the raw word, which is a fair choice but should be a deliberate one.

### Q19 (minor) — T7 race tests: coverage by probability, never confirmed
`lagFor` staggers by `i%5 * 12ms` so that each side wins in some iterations, but the tests only `t.Logf` the win counts. If the claim's git work ever takes more than 48 ms (a slow CI box) or less than the dispatcher's start, one side wins every iteration and the test still passes with half its value lost, and nobody sees it. Either assert that both sides won at least once across the 20 iterations (risky as a flake source, so a `t.Skip`-style note is better), or log a warning with `t.Logf("only one side ever won; the race was not exercised")`. The fixed sleeps themselves are acceptable here, as the assertions hold whichever side wins, but the comment on `lagFor` should say that coverage depends on timing and the invariants do not.

### Q20 (minor) — `TestTransitionTaskStateGuard` can hang on a failed read
The closure calls `read.Done()` only after `GetTask` succeeds. If either `GetTask` errors, the other goroutine waits on `read.Wait()` for ever and the test hangs rather than fails. Use `defer`-safe signalling (call `read.Done()` on the error path too) or a `sync.Once`-guarded barrier with a timeout.

### Q21 (minor) — duplication and leftover steps in the race tests
- `SELECT count(*) FROM work_claims WHERE ref_type = 'task' AND ref_id = $1 AND state <> 'ended'` appears in three tests, and the matching "live implement dispatches" query twice. Add `h.openClaimCount(taskID)` and `h.liveImplementDispatches(taskID)`.
- `TestClaimRacingDispatchReadyTasks` calls `resetTask` (which already deletes the task's executions and requeues the dispatch) and then deletes the executions again, deletes the dispatch, and sets the task `ready`. The `resetTask` call is mostly redundant there; a purpose-built `resetToReady` would say what the test means.
- `TestClaimRacingTheDispatcherHasOneWinner` and `TestClaimOnOneTaskWhileASiblingStarts` share an identical "claim won / dispatcher won" scaffold and the same final `t.Logf`; a small `raceTally` helper would remove the counters.

### Q22 (minor) — more shared helpers in feature-named test files
This continues Q11. `runTogether`, `countWhere`, `startDispatch` and `resetTask` (generic harness methods) sit in `integration_executors_races_test.go`, `roleRouter` (a reusable provider double) in `integration_executors_e2e_test.go`, and `postNoFollow` in `integration_claims_round1_test.go`, whose name ("round1") says when it was written rather than what it tests. Rename that file by concern (for example `integration_claims_ui_and_rails_test.go`) or fold its tests into the files whose subjects they match, and collect the harness methods in a shared `harness_test.go`.

### Q23 (minor) — e2e test: one 170-line function, and a few weak or sloppy assertions
`TestChatClaimedTaskIsReviewedAndVerifiedLikeAnAgents` is long, but its seven numbered step comments make it readable and it mirrors FR-10.1, so splitting is optional. Smaller points:
- `_ = h.srv.Store.Pool.QueryRow(...).Scan(&verifies)` discards an error; use `h.countWhere`, which the sibling file already has.
- `if n := strings.Count(feat, "Submitted by the chat agent: First"); n < 2` is a weak bound; if the page shows each moment once, assert `!= 2`.
- The `rr.script("review:"+sonnet). // the chat agent's work...` chain has its comment on the first line, which makes gofmt indent the continuation lines oddly (`RespondOutcome` ends up far to the right). Move the comment above the statement.
- Step 6's comment says "the verifier runs and approves, and the feature merges" but the first assertion that follows is on review models; reorder or reword.
- The 200-line shape depends on `h.eventually` polling, which is right here (the dispatcher is real), and no bare sleeps are used.

### Q24 (minor) — small template and file-order nits
- `entity.html`: the new `{{if .DoneUnmeasured}} ... {{else}} ... {{end}}` nests an `{{if .Size.Estimated}}` and two `{{end}}`s on the same line as unrelated markup; indenting the else branch would make the nesting readable. The inline `{{/* ... */}}` comment inside the branch is fine.
- `claims.go`: `specRevisedNotice` and `claimActivityWords` were inserted into the "working copy" helper section where `headOf` used to be, ahead of the "Resolving what is claimed" header. They belong under their own header or next to the other messages and presentation helpers.
- `dispatch.go`: the `started` flag plus `defer` is good, but the comment ("The slot is the worker's once it starts; every other exit gives it back.") is the only explanation of why `started` flips just before the goroutine; that is enough.

## Positives

- Comment density and voice match the surrounding code; all new comments are full British-English sentences, and the new UI copy ("sent back by its code reviewer", "a change in the working copy") is whole sentences.
- The bug-fix code (`ImplementPendingCompletion`, the commit-before-config-check reorder in `completeImplementation`, the round in `CodeReviewIdempotencyKey`) is well commented with reasons rather than spec numbers.
- The round-1 and T7 tests use `eventually` and handshakes rather than sleeps; the only `time.Sleep`s are the deliberate lags in the race tests (Q19).

## Verdict

Approve. No major finding is open: every major from round 1 (Q1 to Q5) is properly fixed, and so are Q6 to Q9 and Q13 to Q15. Q10 to Q12 are knowingly left and acceptable. Q16 to Q24 are nits for a follow-up pass; Q19 and Q20 (race-test observability and a possible hang) are the two most worth taking if anyone is touching those files.
