#!/usr/bin/env bash
# SessionStart hook for Claude Code cloud sessions: start the test database
# and export CROMWELL_TEST_DATABASE_URL, so `go test -race ./...` runs the
# integration tests rather than skipping them.
#
# Does nothing on a developer's own machine, and nothing when the PostgreSQL
# binaries are absent. Safe to run repeatedly; quick when already running.
set -euo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
	exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/../..}"

if ! ls -d /usr/lib/postgresql/*/bin/initdb >/dev/null 2>&1 \
	&& ! command -v initdb >/dev/null 2>&1; then
	echo "session-start: no PostgreSQL binaries; integration tests will skip" >&2
	exit 0
fi

line="$(scripts/test-db.sh)"
if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
	grep -qxF "$line" "$CLAUDE_ENV_FILE" 2>/dev/null || echo "$line" >>"$CLAUDE_ENV_FILE"
fi

# Warm the module cache so the first test run doesn't download.
go mod download >/dev/null 2>&1 || true
