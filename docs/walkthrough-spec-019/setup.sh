#!/usr/bin/env bash
# Set up the SPEC-019 walkthrough: a throwaway project in /var/tmp/m12demo,
# served on 127.0.0.1:8819 against a local Postgres. The provider key is a
# dummy, so no model runs: the dispatched steps are proved by the
# mock-provider tests (see the walkthrough). The project's spending limit is
# spent before it starts, so work that is sent waits in the queue, where it
# can be seen, rather than calling a model. Run from the repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m12_demo?sslmode=disable"
if [ -f /var/tmp/m12demo.pid ]; then kill "$(cat /var/tmp/m12demo.pid)" 2>/dev/null || true; sleep 1; fi
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m12_demo WITH (FORCE)" -c "CREATE DATABASE m12_demo"
mkdir -p /var/tmp/m12
go build -o /var/tmp/m12/subutai ./cmd/subutai
rm -rf /var/tmp/m12demo && mkdir -p /var/tmp/m12demo
cd /var/tmp/m12demo
git init -q && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
SUBUTAI_DATABASE_URL="$DB" /var/tmp/m12/subutai init
python3 - <<'PY'
p='.subutai/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: SUBUTAI_DATABASE_URL\n","database:\n  url_env: SUBUTAI_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8819\n  ui_actor: sam\n",1)
open(p,'w').write(s)
PY
git add -A && git commit -qm "subutai init"
# Spend the budget, so the governor holds sent work in the queue.
psql "$DB" -qc "INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key, cost_usd, finished_at)
  VALUES (gen_random_uuid(), 'succeeded', 'estimate', 'estimator', 'claude-sonnet-5', 'project', gen_random_uuid(), 'demo-spent', 1000, now())"
SUBUTAI_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy nohup /var/tmp/m12/subutai serve >/var/tmp/m12/serve.log 2>&1 &
echo $! > /var/tmp/m12demo.pid
sleep 3
curl -sf --noproxy '*' -o /dev/null http://127.0.0.1:8819/ui && echo "serving on 127.0.0.1:8819"
