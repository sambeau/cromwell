#!/usr/bin/env bash
# Start a throwaway PostgreSQL for the integration tests, and print the
# CROMWELL_TEST_DATABASE_URL that points at it.
#
#   eval "$(scripts/test-db.sh)"
#   go test -race -count=1 ./...
#
# Idempotent: if the cluster is already running it only prints the URL.
# Needs the PostgreSQL server binaries (initdb, pg_ctl) installed locally.
# The cluster trusts local connections, so it is for tests only.
#
# Environment (all optional):
#   CROMWELL_TEST_PGDATA  data directory      (default /var/tmp/pgdata)
#   CROMWELL_TEST_PGPORT  TCP port            (default 54329)
#   CROMWELL_TEST_PGBIN   directory of initdb (default: found automatically)
#
# All progress goes to stderr; stdout carries only the export line.
set -euo pipefail

PGDATA_DIR="${CROMWELL_TEST_PGDATA:-/var/tmp/pgdata}"
PGPORT="${CROMWELL_TEST_PGPORT:-54329}"
SOCKDIR="$(dirname "$PGDATA_DIR")"
URL="postgres://postgres@localhost:${PGPORT}/postgres?sslmode=disable"

log() { echo "test-db: $*" >&2; }

find_pgbin() {
	if [ -n "${CROMWELL_TEST_PGBIN:-}" ]; then
		echo "$CROMWELL_TEST_PGBIN"
		return
	fi
	if command -v pg_config >/dev/null 2>&1; then
		local d
		d="$(pg_config --bindir 2>/dev/null || true)"
		if [ -x "$d/initdb" ]; then
			echo "$d"
			return
		fi
	fi
	local d
	for d in $(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V -r); do
		if [ -x "$d/initdb" ]; then
			echo "$d"
			return
		fi
	done
	if command -v initdb >/dev/null 2>&1; then
		dirname "$(command -v initdb)"
		return
	fi
	return 1
}

PGBIN="$(find_pgbin)" || {
	log "no PostgreSQL server binaries found (initdb, pg_ctl); set CROMWELL_TEST_PGBIN"
	exit 1
}

# As root, the cluster runs as the postgres user: initdb refuses root.
if [ "$(id -u)" = 0 ]; then
	PGUSER_OS=postgres
	as_pg() { runuser -u postgres -- "$@"; }
else
	PGUSER_OS="$(id -un)"
	as_pg() { "$@"; }
fi

if [ ! -s "$PGDATA_DIR/PG_VERSION" ]; then
	log "initialising a cluster in $PGDATA_DIR"
	mkdir -p "$PGDATA_DIR"
	if [ "$(id -u)" = 0 ]; then
		chown postgres: "$PGDATA_DIR"
	fi
	chmod 700 "$PGDATA_DIR"
	as_pg "$PGBIN/initdb" -D "$PGDATA_DIR" -U postgres -A trust >/dev/null
fi

if as_pg "$PGBIN/pg_ctl" -D "$PGDATA_DIR" status >/dev/null 2>&1; then
	log "already running on port $PGPORT"
else
	# Something else on the port (another Postgres, say a docker container)
	# would take our connections; refuse rather than test against it.
	if "$PGBIN/pg_isready" -q -h localhost -p "$PGPORT" 2>/dev/null; then
		log "port $PGPORT is taken by another server; set CROMWELL_TEST_PGPORT"
		exit 1
	fi
	log "starting on port $PGPORT (as $PGUSER_OS)"
	as_pg "$PGBIN/pg_ctl" -D "$PGDATA_DIR" -l "$PGDATA_DIR/server.log" -w \
		-o "-p $PGPORT -k $SOCKDIR" start >/dev/null
fi

echo "export CROMWELL_TEST_DATABASE_URL=\"$URL\""
