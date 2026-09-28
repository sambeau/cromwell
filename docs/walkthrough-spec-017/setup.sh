#!/usr/bin/env bash
# Set up the SPEC-017 walkthrough: a throwaway project in /var/tmp/m10demo,
# served on 127.0.0.1:8817 against a local Postgres. The provider key is a
# dummy, so no model runs: the steps that need one are proved by the
# mock-provider tests (see the walkthrough). Agent spec review is switched off
# in this project, so a spec waits for a person rather than a model. Run from
# the repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m10_demo?sslmode=disable"
if [ -f /var/tmp/m10demo.pid ]; then kill "$(cat /var/tmp/m10demo.pid)" 2>/dev/null || true; sleep 1; fi
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m10_demo WITH (FORCE)" -c "CREATE DATABASE m10_demo"
mkdir -p /var/tmp/m10
go build -o /var/tmp/m10/subutai ./cmd/subutai
rm -rf /var/tmp/m10demo && mkdir -p /var/tmp/m10demo
cd /var/tmp/m10demo
git init -q && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
SUBUTAI_DATABASE_URL="$DB" /var/tmp/m10/subutai init
python3 - <<'PY'
p='.subutai/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: SUBUTAI_DATABASE_URL\n","database:\n  url_env: SUBUTAI_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8817\n  ui_actor: sam\n",1)
s=s.replace("  agent: true","  agent: false",1)
open(p,'w').write(s)
PY
git add -A && git commit -qm "subutai init"
SUBUTAI_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy nohup /var/tmp/m10/subutai serve >/var/tmp/m10/serve.log 2>&1 &
echo $! > /var/tmp/m10demo.pid
sleep 3
curl -sf -o /dev/null http://127.0.0.1:8817/ui && echo "serving on 127.0.0.1:8817"
