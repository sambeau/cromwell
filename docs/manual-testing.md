# Manual testing — how to run Cromwell and drive the command centre by hand

A quick path from a clean checkout to a live web command centre you can click
around in. Not a user manual — just enough to exercise the UI and the CLI
against a throwaway project.

## What you need

- **Go** (this repo builds with the toolchain in `go.mod`).
- **Postgres 16.** A dev container is already running for the test suite —
  `cromwell-pg-dev` on port **54329** (`postgres` / `cromwell`). You can reuse it
  for manual play by creating a dedicated database (below), or point at any
  Postgres you like. Without Docker, `eval "$(scripts/test-db.sh)"` starts a
  local cluster on 54329 from the installed PostgreSQL binaries and sets
  `CROMWELL_TEST_DATABASE_URL` (user `postgres`, no password). Claude Code
  cloud sessions do this automatically through the SessionStart hook.
- **No LLM provider is needed** for most of the surface. Reads, estimates,
  milestones, roadmaps, the tree lifecycle, and the archive → checkpoint →
  respond loop all work without one. Only the *agent* flows (a document review
  escalating, an implementation loop) need a real provider key — see the last
  section.

## 1. Build

```sh
go build -o /tmp/cromwell ./cmd/cromwell
```

(Anywhere on `PATH` is fine; `/tmp/cromwell` keeps it out of the repo.)

## 2. Make a throwaway project

```sh
# a dedicated database on the dev container (skip if you have your own Postgres)
docker exec cromwell-pg-dev psql -U postgres -c "CREATE DATABASE cromwell_manual"

export CROMWELL_DATABASE_URL="postgres://postgres:cromwell@localhost:54329/cromwell_manual"
export ANTHROPIC_API_KEY=sk-unused        # a placeholder is fine unless you dispatch agents

# a git repo is required — documents live in git
mkdir -p /tmp/cromwell-play && cd /tmp/cromwell-play
git init -q && git commit -qm init --allow-empty

# init applies the schema and writes .cromwell/ (run it from inside the project)
/tmp/cromwell init
```

## 3. Turn on the web UI

`init` does not enable the TCP listener; the UI is served there. Add a
`server:` block to `.cromwell/config.yaml`:

```yaml
server:
  http: 127.0.0.1:8799        # the web UI + JSON API live here
  socket: /tmp/cromwell.sock  # keep this SHORT (see Gotchas)
  ui_actor: you@example.com   # the identity your browser actions are audited as
```

## 4. Start the server

```sh
cd /tmp/cromwell-play
/tmp/cromwell serve --repo /tmp/cromwell-play
```

You should see it listening on both the unix socket (the CLI) and
`127.0.0.1:8799` (the UI). Leave it running; open a second terminal for the CLI.

Sanity check:

```sh
curl -s http://127.0.0.1:8799/api/status
```

## 5. Seed something to look at

In the second terminal (same `CROMWELL_DATABASE_URL` exported). The CLI talks to
the running server over TCP automatically because `server.http` is set:

```sh
export CROMWELL_DATABASE_URL="postgres://postgres:cromwell@localhost:54329/cromwell_manual"
C=/tmp/cromwell

$C initiative add auth   --name "Authentication"
$C initiative add billing --name "Billing"
$C feature add auth/login --name "Login form"
$C feature add auth/sso   --name "Single sign-on"
$C feature add billing/invoices --name "Invoices"

$C estimate set auth/login 1200 --rationale "email+password form"
$C estimate set auth/sso   8000 --rationale "oauth dance"

$C milestone create v1
$C milestone add v1 auth/login
$C milestone add v1 auth/sso
$C roadmap create 2026
$C roadmap add 2026 v1 --position 0
```

Register a document so the Documents view has a body to render:

```sh
mkdir -p docs/specs
cat > docs/specs/login.md <<'MD'
---
title: Login form
type: spec
owner: auth/login
---

# Login form

## Overview

Users sign in with an **email address** and password.

## Behaviour

- Valid credentials create a session cookie.
- Invalid credentials show an inline error.
MD
git add -A && git commit -qm "login spec"
$C doc add docs/specs/login.md --type spec --owner auth/login
```

## 6. Drive the command centre

Open **http://127.0.0.1:8799/ui**.

**Reads — click through the five views:**

- **Dashboard** — the live queue, cost against the budget cap, the recent event
  stream, calibration health.
- **Planning** — the initiative → feature → task tree with roll-ups (note
  `auth` rolling up to the *worst* tier of its leaves, and `billing` showing
  `?` because `invoices` is unestimated), milestones with progress, roadmaps in
  order.
- **Documents** — the list; click one to see its rendered Markdown body (tables,
  images, code) and comment thread. There is no edit control — that's deliberate.
- **Cost** — roll-ups per initiative / feature / milestone / roadmap / month.
- **Inbox** — empty for now.

**Mutations — the "Actions" panel on the Planning page:**

- **Set estimate** — `ref` = `auth/login`, `tokens` = `2500`, submit. The tree
  roll-up updates in place with a green banner, no reload. Tick "cites corpus"
  to see the tier become `considered` instead of `rough`.
- **New milestone / Milestone member / Lock milestone** — create one, add
  `auth/login`, then try to **Lock** it. It refuses inline ("G4: …") because no
  member is done — you'd descope the unfinished members first. It never forces
  the lock.
- **New initiative / feature, Start / Abandon feature** — abandon refuses
  without a reason.
- A bad input (zero tokens, an unknown ref) shows a red banner inline — never a
  raw error page.

**Realtime — watch the inbox light live.** With the **Inbox** open in the
browser, run this in the terminal:

```sh
$C initiative archive billing --reason "manual test"
```

`billing` has a non-terminal feature, so gate G5 blocks the archive and raises a
checkpoint. **Without touching the browser**, the Inbox badge lights and the
checkpoint card appears (that's the SSE push). Click **override** to answer it —
the inbox clears live and the orchestrator archives `billing`. Check the audit
trail:

```sh
$C log --limit 5
```

You'll see `checkpoint.responded` then `initiative.archived`, both stamped with
your `ui_actor`.

## 7. (Optional) The agent flows — needs a real provider

To watch a document review *escalate* into the inbox, or run an implementation
loop, the server has to dispatch a real agent. Point a provider at a cheap,
billing-capped model in `.cromwell/config.yaml` (e.g. DeepSeek via its
Anthropic-compatible gateway: `base_url: https://api.deepseek.com/anthropic`),
set its key in the environment, then `submit` a document. When the reviewer
escalates, the checkpoint appears in the Inbox *and* the Documents view offers
**approve / request changes** on that document. This costs real (small) money and
is not needed to exercise everything above.

## 8. The smoke project, rebuilt with one script

The live smokes of the authoring chain ([SPEC-009 walkthrough](walkthrough-spec-009-stage1.md))
ran against a smoke project in `/tmp/cromwell-smoke`. That project is gone.
`scripts/smoke-project.sh` rebuilds it:

```sh
eval "$(scripts/test-db.sh)"     # or set SMOKE_DATABASE_URL to a local Postgres
scripts/smoke-project.sh
```

It makes no AI calls. It:

- builds `/tmp/cromwell` and runs `cromwell init` in `/tmp/cromwell-smoke`,
  against a database called `cromwell_smoke`;
- puts every role on one model, by default `deepseek-chat` through DeepSeek's
  Anthropic-compatible gateway, keyed by `DEEPSEEK_API_KEY`;
- turns on the `write-spec` and `write-dev-plan` assignments, so that once a
  feature is sent to development its spec, and then its dev-plan, are written
  for it;
- checks the design template is installed (`init` ships it);
- commits a one-sentence example design, `docs/greet/time/design.md`, in the
  template's shape;
- serves the UI on `127.0.0.1:8801`.

It prints the commands to run next: start the server, create `greet` and
`greet/time`, register the design, and submit it. Submitting costs nothing: no
agent reviews a design, so it waits for you to approve it on its page, and
approving it starts nothing either. The first step that costs money is pressing
**Send to development** on the feature (SPEC-011). The full smoke checklist is
in the [M3 handoff](notes/handoff-M3-2026-09-28.md).

To use another model or provider, set these before running it:

| Variable | Default |
|---|---|
| `SMOKE_MODEL` | `deepseek-chat` |
| `SMOKE_PROVIDER` | `deepseek` |
| `SMOKE_BASE_URL` | `https://api.deepseek.com/anthropic` (set it empty for Anthropic's own endpoint) |
| `SMOKE_API_KEY_ENV` | `DEEPSEEK_API_KEY` |
| `SMOKE_PRICE_IN`, `SMOKE_PRICE_OUT` | `0.27`, `1.10` USD per million tokens |
| `SMOKE_DIR`, `SMOKE_BIN` | `/tmp/cromwell-smoke`, `/tmp/cromwell` |
| `SMOKE_HTTP`, `SMOKE_SOCKET` | `127.0.0.1:8801`, `/tmp/cromwell-smoke.sock` |

For example, on Anthropic:

```sh
SMOKE_MODEL=claude-sonnet-5 SMOKE_PROVIDER=anthropic SMOKE_BASE_URL= \
  SMOKE_API_KEY_ENV=ANTHROPIC_API_KEY SMOKE_PRICE_IN=3 SMOKE_PRICE_OUT=15 \
  scripts/smoke-project.sh
```

It refuses to overwrite an existing project or database; set `SMOKE_FORCE=1`
to rebuild both. It also refuses a database URL that isn't on `localhost`.

## Gotchas

- **Keep `server.socket` short.** On macOS the unix socket path caps at ~104
  bytes, and the socket is *always* bound now (it's the CLI's transport) — a
  deep project path will fail to bind with "invalid argument". `/tmp/cromwell.sock`
  is safe.
- **No `server.http` → no UI.** Without it the server runs for the CLI over the
  socket only; the web UI port is simply closed.
- **`init` runs once, from inside the project**, and needs `CROMWELL_DATABASE_URL`
  set. A second `init` refuses by design.
- **Throwaway only.** Don't point `CROMWELL_DATABASE_URL` at anything real, and
  don't reuse the test database name — the automated suite drops and recreates
  its own schemas.

## Tear down

```sh
# Ctrl-C the server, then:
docker exec cromwell-pg-dev psql -U postgres -c "DROP DATABASE cromwell_manual"
rm -rf /tmp/cromwell-play /tmp/cromwell.sock
```
