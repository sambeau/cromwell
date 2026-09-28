# REVIEW-013: Missed-surface check of SPEC-013 and the rename

**Status:** Complete. The author has dealt with every finding
([SPEC-013 §6](../specs/SPEC-013-rename-to-subutai.md#6-changes-after-review));
awaiting Sam's decision
**Date:** 2026-09-28
**Reviewer:** an independent Claude subagent, not the spec's author, in two
passes. It reported only; it edited nothing. Approval is Sam's.
**Scope:**
- **First pass:** SPEC-013's first draft (commit `be36668`), against
  [DESIGN-010](../design/DESIGN-010-subutai.md) §1 and §15 item 1, the
  [roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11 M7, and the
  code at `96d2b17`.
- **Second pass:** the finished diff, `96d2b17..097f24e`, and the revised
  spec.

## 1. What this review is

A check that the rename misses nothing, and that the compatibility rules hold
for a project made with Cromwell. The reviewer grepped the tree for `cromwell`,
looked for names derived from the folder or the binary that a grep can't
find, and read the compatibility code against the spec. It then checked that
each finding from the first pass was fixed in the code, not only in the text.

## 2. First pass: the spec

| # | Severity | Finding | Resolution |
|---|---|---|---|
| R13-1 | material | Renaming the folder breaks live worktrees: boot reconciliation would re-create `.cromwell/` at the recorded path and fail, because git still registers the moved worktree. | A worktree recorded under `.cromwell/` is followed into `.subutai/` and repaired in git; tested. |
| R13-2 | material | The default socket changed name even under `.cromwell/`, so a Cromwell hook failed silently, and the header fallback's stated reason didn't hold. | Under `.cromwell/` the socket keeps `cromwell.sock`; tested and shown in the walkthrough. |
| R13-3 | material | The hook refresh had no stated home (`server.New` is also the tests' entry point), could write a `go run` binary into the hook, and could flip between two installed binaries. | Lives in `serve`; rewrites only Cromwell's hook, a missing binary, or another repository; refuses build-cache paths; ten test cases. |
| R13-4 | minor | Six places joined `.cromwell` themselves. | One resolver, `compat.ProjectFolder`; the rest derive from it. One more join was found in the second pass (R13-13). |
| R13-5 | minor | The client's placeholder host `http://cromwell` wasn't listed. | Listed; renamed. |
| R13-6 | minor | Two design notes at the top of `docs/`, and the walkthrough scripts, weren't classified. | Named historical. |
| R13-7 | minor | The pack lock's header comment wasn't listed. | Listed; renamed. |
| R13-8 | minor | The roadmap's "in one commit" was dropped without comment. | SPEC-013 §4 says how the commits are split. |
| R13-9 | minor | A stale `cromwell.sock` after the rename. | Ignored; the server removes only its own socket. |
| R13-10 | note | DoD 2 deferred its allowlist to this review. | The allowlist is in DoD 2. |
| R13-11 | note | DoD 3 missed cases. | Added. |
| R13-12 | note | The manual testing guide showed `serve --repo`, a flag that doesn't exist. | Fixed. |

## 3. Second pass: the finished diff

The reviewer confirmed R13-2, R13-3, R13-5 to R13-7, and R13-9 to R13-12 fixed
in the code; R13-8 is text only. R13-1 and R13-4 were only partly fixed, which
is R13-13.

| # | Severity | Finding | Resolution |
|---|---|---|---|
| R13-13 | material | `toolContextForFeature` (`internal/server/planner.go`) built the worktree root itself. After the rename, a feature started under Cromwell would send its agents' tools, commands and review diff to the gone `.cromwell/` path, while the commit used the new one, so a task would fail or commit an empty diff. | Resolves through `worktreeAbs`. The test now checks the agents' root; it fails without the fix (commit `38eed4f`). |
| R13-14 | minor | DoD 2's allowlist missed some living-document hits (the manual testing guide's container password and dual-export sentence, the README's document-map row), and this review didn't exist yet. | Allowlist widened; this review written. |
| R13-15 | minor | The repair ran and logged on every start, since the row keeps its old path; the git version it needs was unstated. | Runs only when git doesn't list the new path; git 2.29 named. |
| R13-16 | minor | No test of `url_env`'s "neither set" error, or of the warning asking for `config.yaml` to change. | Both tested. |
| R13-17 | note | `api_key_env` went through the fallback, with no rule for it. | Read as written again. |
| R13-18 | note | A `.git` file made `serve` print "cannot update" every start; a symlinked repository path counts as another repository. | A `.git` file is no hook, tested. The symlink case rewrites a hook that stays valid; left alone. |

## 4. The remaining `cromwell` hits

`git ls-files | xargs grep -il cromwell`, less the historical documents of
SPEC-013 §2, leaves these. Each is on DoD 2's allowlist:

| File | Why it names Cromwell |
|---|---|
| `internal/compat/compat.go`, `compat_test.go` | The compatibility package and its tests. |
| `internal/client/client.go` | `FindRepoRoot`'s comment, marked `compat(M7)`. |
| `internal/config/config.go` | The `url_env` twin comment, marked `compat(M7)`. |
| `internal/server/http.go` | The `X-Cromwell-Actor` fallback, marked `compat(M7)`. |
| `internal/server/server.go` | A field comment, marked `compat(M7)`. |
| `internal/server/worktree_ops.go` | The worktree repair's comment, marked `compat(M7)`. |
| `internal/starter/starter.go` | The hook shape, the stale check, and `init`'s refusal, marked `compat(M7)`. |
| `internal/testdb/testdb.go` | The test database fallback and skip message, marked `compat(M7)`. |
| `internal/server/compat_test.go`, `internal/client/client_test.go`, `internal/config/config_test.go`, `internal/starter/starter_test.go` | Tests of the compatibility rules. |
| `internal/store/migrations/0001_init.sql` | The kept `cromwell_events` channel (SPEC-013 §3.5). |
| `scripts/test-db.sh`, `scripts/smoke-project.sh`, `.claude/hooks/session-start.sh` | Variable fallbacks and the dual export, marked `compat(M7)`. |
| `README.md` | The "formerly" lines and "Coming from Cromwell". |
| `docs/manual-testing.md` | Sam's container `cromwell-pg-dev` and its password, the dual-export sentence, and the history line. |

## 5. Checks that passed

- No hard-coded `.subutai` join remains in `internal/`: every path goes through
  `compat.ProjectFolder` or the server's `CompartmentRoot`, and `compat.Folder`
  is used only as `init`'s target.
- No product name in cookies, local storage, SSE event names (`changed`), or
  embedded paths (`//go:embed pack`).
- Branches are recorded per feature; nothing derives one from the prefix.
- The MCP endpoint is `/mcp`, independent of the name, so existing client
  configurations keep working.
- The variable fallback: the new name wins, the old warns once, other names
  are read as written. A file named `.cromwell` isn't taken for the folder.
- `init` refuses either folder, and reads its database variable with the
  fallback.
- The `hook` subcommand stays silent; under `.cromwell/`, the old binary
  reaches the new server over the kept socket name.
- The integration tests found their database through
  `CROMWELL_TEST_DATABASE_URL` alone, in the reviewer's own session.

## 6. Recommendation

Approve SPEC-013 with the build. Nothing open blocks it. The four choices in
DoD 7 are Sam's.
