# REVIEW-020 — M13 Executors, code review round 1: quality

Scope: `git diff main...claude/subutai-m13-executors` (66 files). Spec: SPEC-020; plan: devplan-M13-2026-10-02.

Tooling: `gofmt -l internal cmd` is clean and `go vet ./...` reports nothing. (`gofmt -l .` lists one file under `.claude/worktrees/`, which is git-excluded and not part of the branch.) No `TODO` or `FIXME` markers remain, including `TODO(M13 T4)`. No American spellings or curly quotes in the added lines. The templates use `&#39;` for apostrophes; that renders as a straight quote and matches `plan.html`.

Overall the code is consistent in voice and comment density with `provenance.go` and the surrounding server code. The problems are leftover seams from the parallel build: a half-adopted git-head helper, dead store functions, and test helpers that are bespoke where shared ones would do.

## Findings

### Q1 (major) — the git-head helper is adopted by some sites and bypassed by others
`headOf(path)` is defined in `internal/server/claims.go:282` (trimmed, "" on error) and used in `claims.go`, `claims_task.go`, `branch_watch.go` and `actions_phase2.go:273`. Two sibling sites written by a different task still hand-roll the same thing: `actions_phase2.go:174` (`head, _ = gitIn(..., "rev-parse", "HEAD")` then `strings.TrimSpace`) and `actions_phase2.go:454` (same, trimmed later inside the key call). `worktreeFingerprint` (`claims.go:224`) does a third rev-parse, deliberately with `--no-optional-locks`, which is fine but undocumented as the reason it differs. Separately, "head of the live worktree for a feature" exists as `branchHeadForTask` (`claims_task.go:371`) and as the inline `LiveWorktreeForFeature` + `gitIn` at `actions_phase2.go:172-175`. Suggest: replace the two `actions_phase2.go` sites with `headOf(s.worktreeAbs(wt.Path))`, and add a `featureHead(ctx, featureID)` that `branchHeadForTask` and the dispatch-feature code both call. Also move `headOf` out of the middle of the "working copy lock/fingerprint" section into a git-helpers spot (it is now used by four files).

### Q2 (major) — dead production code in the store
Four exported functions are referenced only from tests: `store.TaskUnmeasured` (`executions.go:136`; superseded by `store.Unmeasured` in `unmeasured.go`, which is what the server uses), `store.DeleteExecutionsFor` (`executions.go:146`), `store.RunningImplementInFeature` (`dispatches.go:609`) and `store.MutatingDispatchActiveForTask` (`dispatches.go:435`; the dispatcher now calls `ImplementHold`). Their tests keep them alive, which makes the dead surface look intentional. Delete them and move their test assertions onto the live functions (`Unmeasured`, `ImplementHold`). If `DeleteExecutionsFor` is meant for a later milestone, say so in its comment or drop it. `TaskUnmeasured` and `Unmeasured(task)` are two ways to ask the same question, which is the kind of duplication the parallel build produced.

### Q3 (major) — `MarkExecutionSubmitted` takes an anonymous struct, so every caller spells the type out
`executions.go:98` takes `by struct{ DispatchID, ClaimID *uuid.UUID }`. Callers (`claims.go:597`, `actions_phase2.go:298`, and three times in `claims_test.go`) must repeat the whole struct literal, and "exactly one of the two" is only enforced at run time. Replace with two functions, `MarkDispatchExecutionSubmitted(ctx, tx, dispatchID)` and `MarkClaimExecutionSubmitted(ctx, tx, claimID)`, over one private helper keyed on the column. The "neither given" error branch and its test then disappear.

### Q4 (major) — the "nothing happens after the answer" test is a 200 ms sleep
`integration_claim_sweep_test.go:311` (`time.Sleep(200 * time.Millisecond)`) waits for an answer's side effects, then asserts the claim is unchanged. A negative assertion after a fixed sleep proves little: it passes if the work was merely slow, and it makes the suite slower. If `respond` runs the answer synchronously, drop the sleep. If it is asynchronous, poll for the audit event or the checkpoint state that marks the answer as handled, and assert the claim after that. The same file's harness has `h.respond` returning a status code, so it is probably synchronous.

### Q5 (major) — `TestWithWorkingCopyIsOneAtATimePerPath` cannot fail reliably, and `claims_test.go` relies on a 5 ms sleep
`integration_claims_test.go:1049-1070`: eight goroutines contend, each holding the lock for a 2 ms sleep, with an overlap counter. This does detect a missing lock, but only probabilistically, and it never checks that two *different* paths run concurrently (the other half of the claim in the name, "per path"). Add a second path and assert the two can overlap (a channel handshake, no sleeps). `store/claims_test.go:94` sleeps 5 ms so `LastActivityAt` moves forward; pass an explicit time, or assert `!Before` and compare against a value read inside the transaction, rather than relying on the clock moving.

### Q6 (minor) — `claimable` carries the wrong kind of comment weight, and `refuse` is half-used
`claims.go:76` defines `refuse(format, args...)` (12 uses), but `claims.go:299`, `386`, `397`, `548`, `586` build `&ClaimRefusal{Sentence: ...}` directly. Use `refuse("%s", s)` or a one-argument `refusal(sentence string)` constructor consistently; the direct struct literals leak the type's shape to every caller.

### Q7 (minor) — misleading or undocumented small helpers
- `nilIfNotFound(c, err)` (`claims.go:618`) returns nil on *any* error, not only not-found. Rename (`claimOrNil`) or check `errors.Is(err, store.ErrNotFound)` and propagate the rest.
- `claimIDOr(id)` (`claim_sweep.go:230`) has no comment and a name that says nothing; it returns `uuid.Nil` on a malformed ID. Name it `parseClaimID`, or use `uuid.Parse` at the two call sites and refuse with the moved-on notice on error.
- `execWho` (`claims.go:872`) is a one-line wrapper over `whoWords` used in two places; call `whoWords(e.Kind, e.Actor, e.Model)` directly or add `Execution.Who()` in the store.
- `taskSuffix`/`taskNumber` (`claims_task.go:28-41`): `taskSuffix` is only used by `taskNumber`; fold it in.

### Q8 (minor) — two relative-time formatters
`ago(t)` in `ui.go:1162` returns "3h"; the new `agoWords(t, now)` in `claims.go:938` returns "3 hours ago". The template then uses `ago` followed by a literal " ago" (`executors.html`, `claim-facts`), producing "3h ago". Both are defensible, but they should sit side by side (both in `ui.go` or a `timefmt.go`), and the doc comments should point at each other so the next author picks one deliberately. `agoWords` also lives in the claim service file although it is presentation only.

### Q9 (minor) — repeated worker-slot release in `admit`
`dispatch.go` now has `<-dp.workers` at four exits inside one function (the `StartImplementDispatch` error, `StartQueued`, `StartCancelled`, and the shared `err != nil || !claimed` tail). Fold the implementer branch into a helper that returns `(claimed bool, reason string, err error)` and release the slot once, or use a single `defer` guarded by a `released` flag. `admit` is already the longest function in the file and this makes it longer.

### Q10 (minor) — long functions
`claimLocked` (`claims.go:344`, about 95 lines) and `watchBranchLocked` (`branch_watch.go:93`, about 100 lines) each do several named phases that are already marked by comments ("Step 1, outside any transaction..."). Extract the pre-transaction git reads in `claimLocked` (head, fingerprint, `os.Stat`) into `readClaimGit(t) (head, seen string)`, and the commit-classification loop in `watchBranchLocked` into its own function returning `unclaimedContext`. Not blocking.

### Q11 (minor) — test helpers: location and naming
- Pure unit tests (`TestExecutorSentence`, `TestWithWorkingCopyIsOneAtATimePerPath`, `TestWorktreeFingerprintChangesWithTheWorkingCopy`) sit in `integration_claims_test.go`. Move them to `claims_test.go` so the file name tells the reader whether a database is needed.
- General helpers `writeIn`, `gitOut`, `commitAs`, `asMap`, `asList`, `renderNamed`, `htmlHas` and `htmlLacks` are defined in feature-named test files, so the next feature will not find them. Collect them in `testhelpers_test.go`. `store`'s `seedFeature2` (`claims_test.go:399`) duplicates `seedFeature` in `tasks_test.go:14` with a different name; give it a descriptive name or parameterise `seedFeature`.
- `integration_claims_test.go` is 1139 lines and `claims.go` 955; the former is beyond what a reviewer can hold. Split by concern (claim, submit, release, governor, sweep) along the existing `// ----` section lines.

### Q12 (minor) — logging and error wrapping are thin in the new server code
`claim_sweep.go`, `branch_watch.go` and `claims.go` have no logging calls at all, yet `ClaimSweep` and `BranchWatchSweep` swallow or return per-item errors in a loop; check how `dispatch.RetrySweep` logs (`dp.Log.Error("retry sweep: requeue", "dispatch", d.ID, "err", err)`) and do the same so a failing sweep is visible. Most store errors are returned bare (`return err`), whereas `branch_watch.go:125/147` and `claims_task.go:244/286` wrap with a gerund phrase. Both are present in older code, but within one feature pick one: wrapping at the git boundaries (as these four do) is enough.

### Q13 (minor) — `mcpClaimTask` builds its result by hand
`mcp_claim_tools.go:57-97` builds an eight-key `map[string]any` inline, with the "expires" sentence formatted in the handler. Compare `mcp_bug_tools.go`, which keeps result shaping in small helpers. Move the map into `claimResultMap(res, entry, hours)` so the handler reads as parse, call, shape, and the shape is unit-testable. The `if res.Renewed { out["renewed"] = true }` pair can be one loop or a short helper.

### Q14 (minor) — template and CSS
`executors.html` follows the existing conventions (`panel`, `field`, `u-mt-4`, `action-row`, `count-pill`, icon sprites), and the copy is whole British sentences. Two nits: the `datetime`/`title` layout strings are repeated four times in `claim-facts`; add a `timeTag` template func (or a `{{define "time"}}`) as other templates do for timestamps. The new classes `claim-facts`, `claim-comments`, `task-executor` and `task-executor-mark` do not appear in any stylesheet under `ui/static`, so either they style nothing or the CSS was left out; remove them or add the rules.

### Q15 (minor) — comment style
Density matches `provenance.go` overall. A few places over-explain with spec numbers (FR-/SD-) in lieu of a reason; for example the `claimable` doc comment in `claims.go` spends a paragraph on how it differs from "the spec's sketch", which belongs in the devplan, not the code. `taskClaims`, `mcpClaimTools`, `mcpClaimTask`, `mcpSubmitTask` and `shortHash` have no doc comment while their neighbours do.

## Verdict

Approve with changes. Nothing here is a correctness failure, but Q1 to Q5 are cheap and worth doing before merge: they remove dead exports, collapse the duplicated git-head and execution-marking seams between T-tasks, and replace two tests that prove less than their names claim. Q6 to Q15 may be taken in the same pass or recorded for a follow-up.
