# SPEC-013: Rename to Subutai

**Status:** **Draft — for Sam's approval.** Authored by Claude. An independent
check of this spec and of the finished diff, looking for surfaces it missed, is
recorded in [REVIEW-013](../reviews/REVIEW-013-rename-to-subutai.md). The
author can't be the approval gate, so the decision is Sam's. Four choices in it
need his explicit yes (DoD 7).
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
- **Historical documents.** Decisions, specs, reviews, walkthroughs, handoffs,
  research, and vision documents keep "Cromwell" where they wrote it. They are
  the record.
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
| Command directory | `cmd/cromwell` | `cmd/subutai` | None. `go build ./cmd/subutai` makes `subutai`. |
| CLI usage, help, and error messages | "cromwell …" | "subutai …" | None. |
| Server log line | `cromwell server listening` | `subutai server listening` | None. |
| Web UI title and brand | "— Cromwell", "Cromwell" | "— Subutai", "Subutai" | None. |
| Web UI prose (entity and review pages) | "Cromwell reads it…", "removes the document from Cromwell" | "Subutai …" | None. |
| Icon sprite id, stylesheet header | `cromwell-icons`, "Cromwell Design System" | `subutai-icons`, "Subutai Design System" | None: nothing refers to the id. |
| MCP `serverInfo.name` and instructions | `cromwell`, "Cromwell's planning surface" | `subutai`, "Subutai's planning surface" | The endpoint stays `/mcp`, so a client configured as `claude mcp add … cromwell http://…/mcp` keeps working. The name in that command is the client's own label; people may re-add it as `subutai` when they like. |
| Git author of the tool's commits | `cromwell <cromwell@localhost>` | `subutai <subutai@localhost>` | Past commits keep their author. |
| Commit-message prefix | `cromwell: …` | `subutai: …` | Past commits keep theirs. |
| Actor header on the API | `X-Cromwell-Actor` | `X-Subutai-Actor` | For one release the server also reads `X-Cromwell-Actor` when the new header is absent, so a Cromwell binary still calling the socket is attributed. |
| Branch for a started feature | `cromwell/<path>` | `subutai/<path>` | A feature started before the rename keeps the branch recorded in its row; nothing derives a branch from the prefix. |
| Starter pack | the `review-design` chat skill says "Cromwell's design reviewer" | "Subutai's" | Existing projects keep their copies (the pack is copied once, at `init`). The roles, skills, and templates name no product. |
| Comments and identifiers | "cromwell" where it names the product | "Subutai" | Only where it reads naturally; nothing is renamed for its own sake. |

### 3.2 The project folder

- **`init` creates `.subutai/`.** It refuses to run when either `.subutai/` or
  `.cromwell/` exists.
- **Finding the folder.** Every command, and `serve`, looks for `.subutai/`
  first, then `.cromwell/`, walking up from the working directory as before.
- **Only `.cromwell/` exists:** it loads, and prints one line on standard
  error:

  > warning: this project's folder is .cromwell/, Subutai's old name. Stop the server and, with no feature building, run `git mv .cromwell .subutai`. .cromwell/ is read until the next release.

- **Both exist:** `.subutai/` is used, and one line says so:

  > warning: both .subutai/ and .cromwell/ exist; using .subutai/. Delete .cromwell/ once you have moved anything you need out of it.

- **Everything written into the folder follows the folder in use**, never a
  hard-coded name: the default socket (`<folder>/run/subutai.sock`), new
  worktrees (`<folder>/worktrees/…`), the pack lock, and the chat skills
  (`<folder>/chat-skills/`). A socket set explicitly in `config.yaml` is used
  as written.
- **Recorded worktree paths are kept.** A worktree made before the rename is
  found at the relative path its row records. That is why the warning says to
  rename with no feature building: git records a worktree's absolute path, so
  moving the folder under a live worktree detaches it. The walkthrough shows
  the rename on an idle project.
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
is read as written.

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

**The rule: `serve` keeps the hook pointing at itself.** On start, `serve`
reads the hook. If the file is one `init` wrote (either product's version,
recognised by its exact shape) and runs a different executable or repository,
`serve` rewrites it for the running binary and prints one line:

> updated .git/hooks/post-commit to run /path/to/subutai

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
| Unix socket | Default `<folder>/run/subutai.sock` (§3.2). A Cromwell binary can't reach a Subutai server's socket; after `serve` refreshes the hook (§3.4), nothing calls it. |
| Worktree directories | Under the folder in use (§3.2). |
| `docs/_superseded/` | Names no product. Unchanged. |
| Existing MCP client configuration | Keeps working (§3.1). |
| `scripts/smoke-project.sh` | `/tmp/subutai-smoke`, binary `/tmp/subutai`, socket `/tmp/subutai-smoke.sock`, database `subutai_smoke`, and the variables above. |
| Test databases | `subutai_server_test`, `subutai_store_test`. They are made on demand. |
| CI service database | Database and password `subutai`. |
| `pg_notify('cromwell_events', …)` in migration `0001` | **Kept.** Applied migrations are never edited, nothing listens on the channel, and renaming it would need a migration for no reader. Recorded as a follow-up. |
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
hook refresh.

## 5. Definition of done

1. `go build ./...` and `go vet ./...` are clean, and
   `go test -race -count=1 ./...` passes, with a verbose run showing the
   integration tests ran rather than skipped.
2. A case-insensitive grep for `cromwell` outside the historical documents
   finds only hits that are justified in REVIEW-013: compatibility code, its
   tests, and the kept surfaces of §3.5.
3. Unit tests cover: folder discovery (new, old, both, neither), the variable
   fallback (new, old, both, neither, and `url_env` naming either twin), the
   header fallback, and the hook refresh (ours and stale, ours and current,
   Cromwell's, edited, absent).
4. **A fresh project:** `subutai init` a throwaway repository, `subutai serve`
   it, and a screenshot of the web UI shows Subutai.
5. **An old project:** made with the Cromwell binary built before the rename,
   then served by `subutai` with only `CROMWELL_*` variables set. It loads
   with the warnings of §3.2 and §3.3, `serve` refreshes the hook, and a
   commit reaches the new server through it. Then the folder is renamed, and
   the project loads without the folder warning.
6. The walkthrough, `docs/walkthrough-spec-013.md`, records 4 and 5.
7. **Sam's choices.** Each is the author's recommendation:
   1. `serve` refreshes the post-commit hook (§3.4), rather than an explicit
      upgrade step.
   2. "One release" ends at the first milestone after "Subutai usable" (§4).
   3. `scripts/test-db.sh` exports both names for that release (§3.3).
   4. The `cromwell_events` notification channel is kept (§3.5).
