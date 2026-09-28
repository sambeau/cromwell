#!/usr/bin/env bash
# Build a throwaway smoke project for live runs of the authoring chain
# (SPEC-009) and the implementation loop. It makes no AI calls itself.
#
#   eval "$(scripts/test-db.sh)"      # or set SMOKE_DATABASE_URL yourself
#   scripts/smoke-project.sh
#
# What it builds, in $SMOKE_DIR:
#   - a git repository, initialised with `cromwell init`;
#   - every role on $SMOKE_MODEL, served by $SMOKE_PROVIDER at $SMOKE_BASE_URL;
#   - the write-spec and write-dev-plan assignments enabled;
#   - the design template (shipped by init) and a one-sentence example design,
#     docs/greet/time/design.md, committed but not yet registered;
#   - the web UI on $SMOKE_HTTP.
#
# Environment (all optional):
#   SMOKE_DIR           project directory          (default /tmp/cromwell-smoke)
#   SMOKE_BIN           where to build cromwell    (default /tmp/cromwell)
#   SMOKE_DATABASE_URL  Postgres URL for the project; must be local.
#                       Default: CROMWELL_TEST_DATABASE_URL's server, with the
#                       database cromwell_smoke.
#   SMOKE_PROVIDER      provider name in config    (default deepseek)
#   SMOKE_BASE_URL      Anthropic-compatible endpoint; empty for Anthropic's own
#                       (default https://api.deepseek.com/anthropic)
#   SMOKE_API_KEY_ENV   env var holding the key    (default DEEPSEEK_API_KEY)
#   SMOKE_MODEL         model for every role       (default deepseek-chat)
#   SMOKE_PRICE_IN      USD per million input tokens  (default 0.27)
#   SMOKE_PRICE_OUT     USD per million output tokens (default 1.10)
#   SMOKE_HTTP          web UI listen address      (default 127.0.0.1:8801)
#   SMOKE_SOCKET        CLI socket; keep it short  (default /tmp/cromwell-smoke.sock)
#   SMOKE_FORCE=1       remove an existing project and recreate its database
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"

SMOKE_DIR="${SMOKE_DIR:-/tmp/cromwell-smoke}"
SMOKE_BIN="${SMOKE_BIN:-/tmp/cromwell}"
SMOKE_PROVIDER="${SMOKE_PROVIDER:-deepseek}"
SMOKE_BASE_URL="${SMOKE_BASE_URL-https://api.deepseek.com/anthropic}"
SMOKE_API_KEY_ENV="${SMOKE_API_KEY_ENV:-DEEPSEEK_API_KEY}"
SMOKE_MODEL="${SMOKE_MODEL:-deepseek-chat}"
SMOKE_PRICE_IN="${SMOKE_PRICE_IN:-0.27}"
SMOKE_PRICE_OUT="${SMOKE_PRICE_OUT:-1.10}"
SMOKE_HTTP="${SMOKE_HTTP:-127.0.0.1:8801}"
SMOKE_SOCKET="${SMOKE_SOCKET:-/tmp/cromwell-smoke.sock}"

die() { echo "smoke-project: $*" >&2; exit 1; }
log() { echo "smoke-project: $*" >&2; }

# --- The database -----------------------------------------------------------

if [ -z "${SMOKE_DATABASE_URL:-}" ]; then
	[ -n "${CROMWELL_TEST_DATABASE_URL:-}" ] ||
		die "set SMOKE_DATABASE_URL, or run: eval \"\$(scripts/test-db.sh)\""
	base="${CROMWELL_TEST_DATABASE_URL%%\?*}"
	query=""
	case "$CROMWELL_TEST_DATABASE_URL" in *\?*) query="?${CROMWELL_TEST_DATABASE_URL#*\?}" ;; esac
	SMOKE_DATABASE_URL="${base%/*}/cromwell_smoke${query}"
fi

# Never a remote database: the smoke project drops and recreates its own.
case "$SMOKE_DATABASE_URL" in
	*@localhost[:/]* | *@127.0.0.1[:/]* | *@\[::1\][:/]*) ;;
	*) die "SMOKE_DATABASE_URL must point at localhost; refusing $SMOKE_DATABASE_URL" ;;
esac

dbname="${SMOKE_DATABASE_URL%%\?*}"
dbname="${dbname##*/}"
admin_url="${SMOKE_DATABASE_URL/\/$dbname/\/postgres}"
[[ "$dbname" =~ ^[a-z_][a-z0-9_]*$ ]] || die "unexpected database name '$dbname'"

# --- The project directory --------------------------------------------------

if [ -e "$SMOKE_DIR" ]; then
	[ "${SMOKE_FORCE:-}" = 1 ] || die "$SMOKE_DIR exists; set SMOKE_FORCE=1 to rebuild it"
	log "removing $SMOKE_DIR"
	rm -rf "$SMOKE_DIR"
fi

exists="$(psql "$admin_url" -tAc "SELECT 1 FROM pg_database WHERE datname = '$dbname'")"
if [ "$exists" = 1 ]; then
	[ "${SMOKE_FORCE:-}" = 1 ] || die "database $dbname exists; set SMOKE_FORCE=1 to recreate it"
	log "dropping database $dbname"
	psql -q "$admin_url" -c "DROP DATABASE \"$dbname\" WITH (FORCE)"
fi
psql -q "$admin_url" -c "CREATE DATABASE \"$dbname\""

# --- Build and init ---------------------------------------------------------

log "building $SMOKE_BIN"
(cd "$REPO" && go build -o "$SMOKE_BIN" ./cmd/cromwell)

mkdir -p "$SMOKE_DIR"
cd "$SMOKE_DIR"
git init -q
git config user.name >/dev/null || git config user.name "Smoke Operator"
git config user.email >/dev/null || git config user.email "smoke@example.com"
git commit -qm "init" --allow-empty

CROMWELL_DATABASE_URL="$SMOKE_DATABASE_URL" "$SMOKE_BIN" init >&2

# --- Config: one provider, one model, the chain switched on -----------------

base_url_line=""
[ -n "$SMOKE_BASE_URL" ] && base_url_line="    base_url: $SMOKE_BASE_URL"

cat >.cromwell/config.yaml <<YAML
version: 1

database:
  url_env: CROMWELL_DATABASE_URL

server:
  http: $SMOKE_HTTP
  socket: $SMOKE_SOCKET
  ui_actor: sam

budget:
  period: monthly
  cap_usd: 5.00
  warn_fraction: 0.8
  per_dispatch_cap_usd: 0.50

providers:
  $SMOKE_PROVIDER:
    api_key_env: $SMOKE_API_KEY_ENV
$base_url_line
    rate:
      requests_per_minute: 50

models:
  $SMOKE_MODEL:
    provider: $SMOKE_PROVIDER
    price_per_mtok:
      input: $SMOKE_PRICE_IN
      output: $SMOKE_PRICE_OUT
      cache_read: 0
      cache_write: 0

assignments:
  implement-task: implementer
  review-code: code-reviewer
  verify-feature: verifier
  estimate: estimator
  write-spec: spec-author
  write-dev-plan: dev-plan-author

commands:
  build:
    argv: ["true"]
    timeout_seconds: 300
    output_cap_bytes: 65536
  run_tests:
    argv: ["true"]
    timeout_seconds: 600
    output_cap_bytes: 65536
YAML

for role in .cromwell/roles/*.yaml; do
	sed -i.bak "s/^model: .*/model: $SMOKE_MODEL/" "$role" && rm -f "$role.bak"
done

[ -f .cromwell/templates/design/manifest.yaml ] ||
	die "init did not install the design template"

# --- The example design: one sentence, in the template's shape --------------

mkdir -p docs/greet/time
cat >docs/greet/time/design.md <<'MD'
---
title: "Tell the time"
type: design
owner: "greet/time"
---

# Tell the time

## What this is for

A small Go package with one function, `Now`, that returns the current date
and time.

## The shape of it

`Now()` returns a `time.Time`.

## Decisions

- One function and no configuration, because the point is to exercise the
  workflow, not the code.
MD

git add .cromwell docs
git commit -qm "smoke project: config and example design"

cat >&2 <<EOF

Smoke project ready in $SMOKE_DIR (database $dbname).

Next, by hand:

  export CROMWELL_DATABASE_URL="$SMOKE_DATABASE_URL"
  export $SMOKE_API_KEY_ENV=...          # the provider key; costs real money
  cd $SMOKE_DIR && $SMOKE_BIN serve      # UI at http://$SMOKE_HTTP/ui

  # in a second terminal, same CROMWELL_DATABASE_URL:
  cd $SMOKE_DIR
  $SMOKE_BIN initiative add greet --name "Greetings"
  $SMOKE_BIN feature add greet/time --name "Tell the time" \\
      --description "Tell a program the current date and time."
  $SMOKE_BIN doc add docs/greet/time/design.md --type design --owner greet/time
  $SMOKE_BIN submit docs/greet/time/design.md   # it waits for you; no agent reviews a design

Then approve the design from its page in the UI, and see that nothing starts.
Press Send to development on the feature to start the agents. See
docs/notes/handoff-M3-2026-09-28.md for the full smoke checklist.
EOF
