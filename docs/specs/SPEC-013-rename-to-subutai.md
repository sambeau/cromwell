# SPEC-013: Rename to Subutai

**Status:** **Approved — Sam, 2026-09-28**, with the build, and with the recommendation accepted on each of its four choices (DoD 7). It was drafted for Sam's approval as follows. Authored by Claude. An independent
check of this spec and of the finished diff, looking for surfaces it missed, is
recorded in [REVIEW-013](../reviews/REVIEW-013-rename-to-subutai.md). It found
three material and nine smaller problems in the first draft; all are dealt with
in this revision, and §6 says how. A second pass over the finished diff found one more material problem, fixed
in the code and recorded in §6. The author can't be the approval gate, so
the decision is Sam's. Four choices in it need his explicit yes (DoD 7).
**Date:** 2026-09-28
**Roadmap milestone:** M7 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) §1 ("Subutai
is the new name for Cromwell") and §15 item 1 ("Cromwell becomes Subutai, once,
at a clean boundary"), both approved and binding

---

## 1. Goal

One name everywhere, changed once. After this milestone the binary is
`subutai`, a new project keeps its configuration in `.subutai/`, its variables
are `SUBUTAI_*`, and every page, message, and commit the tool makes says
Subutai. A project made with Cromwell keeps working, unchanged, for one
release, and tells its owner exactly what to rename.

The roadmap's test is short: `subutai serve` works, and the suite is green.

## 2. Scope

**In scope:** the Go module, the command, every user-facing string in code, the
project folder, environment variables, the scripts, the CI workflow, the
session hook, the post-commit hook of existing projects, and the living
documents (README, manual testing guide, script help).

**Out of scope:**

- **Renaming the GitHub repository.** That is Sam's action; the steps are in
  the handoff.
- **Historical documents.** Decisions, designs, specs, reviews, walkthroughs
  (and their scripts, such as `docs/walkthrough-spec-011/setup.sh`), handoffs
  and notes, research, and vision documents keep "Cromwell" where they wrote
  it. So do the two design notes at the top of `docs/`,
  `design-brief-web-ui-treatment.md` and `design-system-changes-round-1.md`.
  They are the record. The living documents are the README, the manual testing
  guide, and the scripts' help.
- **Persisted data.** Audit rows, actor names, commit history, recorded branch
  names, and recorded worktree paths stay as they were written. Nothing is
  rewritten.
- **An `upgrade` command.** The compatibility rules below make one
  unnecessary for this change.

## 3. Surfaces

Each surface has a new name and a compatibility rule. "One release" is defined
in §4.

### 3.1 Code and the binary

| Surface | Was | Becomes | Compatibility |
|---|---|---|---|
| Go module | `cromwell` | `subutai` | None needed: the module path is local, not a repository URL. |
| The client's placeholder host for the socket | `http://cromwell` | `http://subutai` | None: it never leaves the process. |
| The pack lock's header comment | "Machine-managed by cromwell init/upgrade" | "subutai init/upgrade" | Existing projects keep their lock as written; it is only a comment. |
| Command directory | `cmd/cromwell` | `cmd/subutai` | None. `go build ./cmd/subutai` makes `subutai`. |
| CLI usage, help, and error messages | "cromwell …" | "subutai …" | None. |
| Server log line | `cromwell server listening` | `subutai server listening` | None. |
| Web UI title and brand | "— Cromwell", "Cromwell" | "— Subutai", "Subutai" | None. |
| Web UI prose (entity and review pages) | "Cromwell reads it…", "removes the document from Cromwell" | "Subutai …" | None. |
| Icon sprite id, stylesheet header | `cromwell-icons`, "Cromwell Design System" | `subutai-icons`, "Subutai Design System" | None: nothing refers to the id. |
| MCP `serverInfo.name` and instructions | `cromwell`, "Cromwell's planning surface" | `subutai`, "Subutai's planning surface" | The endpoint stays `/mcp`, so a client configured as `claude mcp add … cromwell http://…/mcp` keeps working. The name in that command is the client's own label; people may re-add it as `subutai` when they like. |
| Git author of the tool's commits | `cromwell <cromwell@localhost>` | `subutai <subutai@localhost>` | Past commits keep their author. |
| Commit-message prefix | `cromwell: …` | `subutai: …` | Past commits keep theirs. |
| Actor header on the API | `X-Cromwell-Actor` | `X-Subutai-Actor` | For one release the server also reads `X-Cromwell-Actor` when the new header is absent, so a Cromwell binary still calling the server (over the socket while the folder is `.cromwell/`, §3.2, or over `server.http`) is attributed. |
| Branch for a started feature | `cromwell/<path>` | `subutai/<path>` | A feature started before the rename keeps the branch recorded in its row; nothing derives a branch from the prefix. |
| Starter pack | the `review-design` chat skill says "Cromwell's design reviewer" | "Subutai's" | Existing projects keep their copies (the pack is copied once, at `init`). The roles, skills, and templates name no product. |
| Comments and identifiers | "cromwell" where it names the product | "Subutai" | Only where it reads naturally; nothing is renamed for its own sake. |

### 3.2 The project folder

- **`init` creates `.subutai/`.** It refuses to run when either `.subutai/` or
  `.cromwell/` exists.
- **Finding the folder.** Every command, and `serve`, looks for `.subutai/`
  first, then `.cromwell/`, walking up from the working directory as before.
  One resolver, `compat.ProjectFolder`, answers for the client, the server,
  and `init`; everything else derives its paths from the folder it returns.
- **Only `.cromwell/` exists:** it loads, and prints one line on standard
  error:

  > warning: this project's folder is .cromwell/, Subutai's old name. Stop the server and run `git mv .cromwell .subutai`. .cromwell/ is read until the next release.

- **Both exist:** `.subutai/` is used, and one line says so:

  > warning: both .subutai/ and .cromwell/ exist; using .subutai/. Delete .cromwell/ once you have moved anything you need out of it.

- **Everything written into the folder follows the folder in use**, never a
  hard-coded name: new worktrees (`<folder>/worktrees/…`), the pack lock, and
  the chat skills (`<folder>/chat-skills/`).
- **The default socket** is `.subutai/run/subutai.sock`. While the folder is
  still `.cromwell/`, it keeps its old name, `.cromwell/run/cromwell.sock`, so
  a Cromwell binary (its post-commit hook, say) still reaches the server. A
  socket set explicitly in `config.yaml` is used as written. After the folder
  is renamed, a leftover `cromwell.sock` in `run/` is ignored; the server
  removes only its own socket.
- **Recorded worktree paths are kept, and followed.** A worktree row keeps the
  relative path it was made with. A path recorded under `.cromwell/` whose
  directory is gone, in a project now using `.subutai/`, is looked for under
  `.subutai/` instead: `git mv` moves the whole folder, worktrees included.
  git records a worktree's absolute path, so on start the server runs
  `git worktree repair` (git 2.29 or later) on each worktree it finds moved
  that git doesn't already list at its new path. The agents' tools, commands,
  and review diffs resolve the worktree the same way. Nothing re-creates
  `.cromwell/`. So the folder can be renamed while features are building, as
  long as the server is stopped.
- The folder's own `.gitignore` (`run/`, `worktrees/`) is relative, so it
  moves with the folder.

### 3.3 Environment variables

| Was | Becomes |
|---|---|
| `CROMWELL_DATABASE_URL` | `SUBUTAI_DATABASE_URL` |
| `CROMWELL_TEST_DATABASE_URL` | `SUBUTAI_TEST_DATABASE_URL` |
| `CROMWELL_TEST_PGDATA`, `_PGPORT`, `_PGBIN` (`scripts/test-db.sh`) | `SUBUTAI_TEST_PGDATA`, `_PGPORT`, `_PGBIN` |
| `CROMWELL_M6_DEMO` (a skipped demo test) | `SUBUTAI_M6_DEMO` |

**The rule.** For one release, every `SUBUTAI_*` variable the code or the
scripts read falls back to its `CROMWELL_*` twin. The new name wins when both
are set. A fallback that is used prints one warning line naming both
variables:

> warning: CROMWELL_DATABASE_URL is Subutai's old name for SUBUTAI_DATABASE_URL; rename it. The old name is read until the next release.

**The database variable is named in `config.yaml`**, as `database.url_env`.
`init` now writes `SUBUTAI_DATABASE_URL`. A Cromwell project's config says
`CROMWELL_DATABASE_URL`; when `url_env` names either twin, the pair is treated
as above: `SUBUTAI_DATABASE_URL` first, then `CROMWELL_DATABASE_URL`. So an
old project works whichever of the two its owner has set, and a new project
works with an old shell. A `url_env` that names the old variable also gets a
line asking for `config.yaml` to be updated. A `url_env` naming anything else
is read as written, and so is every `api_key_env`.

**`init`** reads `SUBUTAI_DATABASE_URL`, with the same fallback.

**The test suite** reads `SUBUTAI_TEST_DATABASE_URL`, falling back to
`CROMWELL_TEST_DATABASE_URL`. It still skips the integration tests when
neither is set, and the skip message names both.

**`scripts/test-db.sh`** prints one line that exports *both* names, for one
release:

```
export SUBUTAI_TEST_DATABASE_URL="…" CROMWELL_TEST_DATABASE_URL="…"
```

That keeps a branch cut before the rename finding its database in a session
started after it, which matters because a missing variable makes the suite
skip silently. The SessionStart hook writes that line unchanged. The CI
workflow sets `SUBUTAI_TEST_DATABASE_URL` only; CI always runs the branch's
own code.

### 3.4 The post-commit hook of existing projects

`init` installs `.git/hooks/post-commit`, which runs the absolute path of the
binary that installed it. After the rename an existing project's hook still
runs the old `cromwell` binary, which may be gone, and which looks for the old
socket.

**The rule: `serve` keeps the hook pointing at a binary that works.** On
start, the `serve` command (in `cmd/subutai`, not `server.New`, which the tests
also call) reads the hook. It rewrites it for the running binary, and prints
one line, when all of these hold:

- the file is one `init` wrote, from either product: its exact four lines,
  with the executable and repository as Go-quoted strings (`%q`), matched by
  one regular expression and unquoted;
- and it is Cromwell's, or the executable it names no longer exists, or it
  names another repository;
- and the running binary isn't in Go's build cache (a path containing
  `/go-build`), which is gone when `go run` exits. In that case it says so
  instead.

> updated .git/hooks/post-commit to run /path/to/subutai

- **A hook naming another Subutai binary that exists** is left alone, so the
  hook doesn't flip between two installed copies.
- **A hook a person has edited** (anything but the exact shape) is left alone.
  If it still calls `hook post-commit`, `serve` prints one line saying which
  line to change.
- **No hook** is left alone: removing it is a person's choice.
- A hook that is already current is not touched, so this is silent on every
  normal start.

This is not only a compatibility rule. It also fixes a project whose binary
has moved, so it stays after the fallbacks are removed. Only the recognition
of the Cromwell version of the hook ends with them.

Why on `serve` and not an explicit upgrade step: the hook only matters while a
server runs, `serve` is the one command every project runs, and the rewrite
is safe because it touches only a file `init` wrote in a shape it recognises.
An explicit step would leave every existing project quietly broken until its
owner read the release notes.

### 3.5 Other surfaces checked

| Surface | Decision |
|---|---|
| Unix socket | `.subutai/run/subutai.sock`, or the old name while the folder is `.cromwell/` (§3.2). |
| Worktree directories | Under the folder in use (§3.2). |
| `docs/_superseded/` | Names no product. Unchanged. |
| Existing MCP client configuration | Keeps working (§3.1). |
| `scripts/smoke-project.sh` | `/tmp/subutai-smoke`, binary `/tmp/subutai`, socket `/tmp/subutai-smoke.sock`, database `subutai_smoke`, and the variables above. |
| Test databases | `subutai_server_test`, `subutai_store_test`. They are made on demand. |
| CI service database | Database and password `subutai`. |
| `pg_notify('cromwell_events', …)` in migration `0001` | **Kept.** Applied migrations are never edited, nothing listens on the channel, and renaming it would need a migration for no reader. Recorded as a follow-up. |
| Sam's Docker container `cromwell-pg-dev` and its password, named in the manual testing guide | **Kept.** It is a thing on Sam's machine, not in this repository. |
| Historical documents, and the vision file `Cromwell R2 Vision Subutai.md` | Kept (§2). |
| The GitHub repository `sambeau/cromwell` | Sam's action (§2). |

## 4. When the compatibility ends

The project doesn't tag releases yet. **"One release" means: the fallbacks are
removed in the first milestone after "Subutai usable"** (the roadmap's marker
after M7), so every project in use has at least that phase to move. All
fallback code lives in one package, `internal/compat`, plus the header
fallback in `http.go` and the old-marker recognition in the starter, each
marked `compat(M7)`, so the removal is a grep. `scripts/test-db.sh` and
`scripts/smoke-project.sh` carry the same marker.

What remains after removal: `.subutai/`, `SUBUTAI_*`, `X-Subutai-Actor`, and the
hook refresh. **Before removing it,** check that no live worktree row records a
path under `.cromwell/`, and that no project still has that folder.

**The roadmap's "in one commit".** M7 in the roadmap says to do the rename in
one commit. The rename itself is one commit; the compatibility code, a test
fix, and the documents are separate commits on the same branch, so each can be
read on its own. Nothing else was open while it ran.

## 5. Definition of done

1. `go build ./...` and `go vet ./...` are clean, and
   `go test -race -count=1 ./...` passes, with a verbose run showing the
   integration tests ran rather than skipped.
2. `git ls-files | xargs grep -il cromwell` finds nothing outside this
   allowlist, and REVIEW-013 justifies each file on it:
   - the historical documents of §2;
   - `internal/compat/`, and lines marked `compat(M7)`;
   - the tests of the compatibility rules;
   - `internal/store/migrations/0001_init.sql` (§3.5);
   - the living documents where they explain the old name: the README's
     "formerly" lines (the opening, and the project folder's row in the
     document map) and its "Coming from Cromwell" section; and the manual
     testing guide's dev container (name and password), the sentence saying
     `test-db.sh` exports the old variable too, and its history line.
3. Unit tests cover: folder discovery (new, old, both, neither), `init`
   refusing either folder, the default socket under each folder, the variable
   fallback (new, old, both, neither, and `url_env` naming either twin), the
   header fallback, a worktree recorded under `.cromwell/` found, repaired
   once, and handed to agents after the rename, and the hook refresh (Cromwell's, a missing binary,
   another repository, current, another installed binary, edited, unrelated,
   absent, `.git` as a file, and a `go run` binary).
4. **A fresh project:** `subutai init` a throwaway repository, `subutai serve`
   it, and a screenshot of the web UI shows Subutai.
5. **An old project:** made with the Cromwell binary built before the rename,
   then served by `subutai` with only `CROMWELL_*` variables set. It loads
   with the warnings of §3.2 and §3.3, `serve` refreshes the hook, and a
   commit reaches the new server through it. The Cromwell binary's own hook
   command reaches it too, over the kept socket name. Then the folder is
   renamed, and the project loads without the folder warning. The
   `test-db.sh` and smoke-project fallbacks are spot-checked by hand.
6. The walkthrough, `docs/walkthrough-spec-013.md`, records 4 and 5.
7. **Sam's choices.** Each is the author's recommendation:
   1. `serve` refreshes the post-commit hook (§3.4), rather than an explicit
      upgrade step.
   2. "One release" ends at the first milestone after "Subutai usable" (§4).
   3. `scripts/test-db.sh` exports both names for that release (§3.3).
   4. The `cromwell_events` notification channel is kept (§3.5).

## 6. Changes after review

[REVIEW-013](../reviews/REVIEW-013-rename-to-subutai.md) §2 checked the first
draft (commit `be36668`). Finding by finding:

| Finding | What changed |
|---|---|
| R13-1 (material): renaming the folder breaks live worktrees, and boot reconciliation makes it worse by re-creating `.cromwell/` | §3.2: a worktree recorded under `.cromwell/` is followed into `.subutai/` and repaired in git on start; `addWorktree` resolves through the same rule, so nothing re-creates `.cromwell/`. The warning no longer says "with no feature building". Tested (DoD 3). |
| R13-2 (material): the default socket changed name under the old folder, so a Cromwell hook failed silently, contradicting the header rationale | §3.2: under `.cromwell/` the default socket keeps its old name. §3.1's header rationale says which paths reach the server. |
| R13-3 (material): the hook refresh had no stated home, could point at a `go run` binary, and could flip between two installed binaries | §3.4: it runs in `cmd/subutai`'s `serve`; rewrites only Cromwell's hook, a missing binary, or another repository; never writes a build-cache path; says how the shape is matched. |
| R13-4 (minor): six places join `.cromwell` themselves | §3.2: one resolver, and every path derives from its answer. |
| R13-5 (minor): the client's placeholder host | Listed in §3.1. |
| R13-6 (minor): two root-level design notes and the walkthrough scripts unclassified | §2 names them historical. |
| R13-7 (minor): the pack lock's header | Listed in §3.1. |
| R13-8 (minor): the roadmap's "one commit" dropped | §4 says how the commits are split, and why. |
| R13-9 (minor): a stale `cromwell.sock` | §3.2: ignored; the server removes only its own socket. |
| R13-10 (note): DoD 2 deferred its allowlist | DoD 2 lists it. |
| R13-11 (note): DoD 3 missed cases | DoD 3 and DoD 5 name them. |
| R13-12 (note): the manual testing guide shows `serve --repo`, a flag that doesn't exist | Fixed in the guide, which this milestone rewrites anyway. |

### Second pass, over the finished diff

| Finding | What changed |
|---|---|
| R13-13 (material): the agents' tool context built the worktree root itself, so after the rename a feature started under Cromwell sent its tools, commands and review diff to the old path | `toolContextForFeature` resolves through `worktreeAbs`. `TestWorktreeFoundAfterFolderRename` checks the root, and fails without the fix. |
| R13-14 (minor): DoD 2's allowlist missed some living-document hits, and REVIEW-013 wasn't written yet | DoD 2 widened; REVIEW-013 written. |
| R13-15 (minor): the repair ran on every start; the git version it needs was unstated | It runs only when git doesn't list the new path; §3.2 names git 2.29. |
| R13-16 (minor): no test of `url_env`'s "neither set" error or of its `config.yaml` warning | Both tested. |
| R13-17 (note): `api_key_env` went through the variable fallback, which no rule covered | It is read as written again (§3.3). |
| R13-18 (note): a `.git` file made `serve` print "cannot update" on every start; a symlinked path counts as another repository | A `.git` file is treated as no hook, and tested. The symlink case rewrites a hook that stays valid; left as it is. |
