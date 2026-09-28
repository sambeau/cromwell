#!/usr/bin/env bash
# Set up the SPEC-016 browser walkthrough: a throwaway project in
# /var/tmp/m9demo, served on 127.0.0.1:8816 against a local Postgres, with a
# dummy provider key. No AI provider is used. Run from the repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m9_demo?sslmode=disable"
if [ -f /var/tmp/m9demo.pid ]; then kill "$(cat /var/tmp/m9demo.pid)" 2>/dev/null || true; sleep 1; fi
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m9_demo WITH (FORCE)" -c "CREATE DATABASE m9_demo"
mkdir -p /var/tmp/m9
go build -o /var/tmp/m9/subutai ./cmd/subutai
rm -rf /var/tmp/m9demo && mkdir -p /var/tmp/m9demo
cd /var/tmp/m9demo
git init -q && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
SUBUTAI_DATABASE_URL="$DB" /var/tmp/m9/subutai init
python3 - <<'PY'
p='.subutai/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: SUBUTAI_DATABASE_URL\n","database:\n  url_env: SUBUTAI_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8816\n  ui_actor: sam\n",1)
open(p,'w').write(s)
PY
git add -A && git commit -qm "subutai init"
SUBUTAI_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy nohup /var/tmp/m9/subutai serve >/var/tmp/m9/serve.log 2>&1 &
echo $! > /var/tmp/m9demo.pid
sleep 3
curl -sf -o /dev/null http://127.0.0.1:8816/ui && echo "serving on 127.0.0.1:8816"
