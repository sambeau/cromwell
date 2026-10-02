#!/usr/bin/env bash
# Set up the SPEC-021 stage 2 walkthrough: a throwaway project in
# /var/tmp/m14s2demo, served on 127.0.0.1:8830 against a local Postgres.
# No AI provider is used. The provider's base_url points at fake_provider.py
# on 127.0.0.1:8831 only so the server boots; nothing is dispatched to it.
# The heartbeat is shortened to 5 seconds so a time box ends quickly.
# Run from the repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m14s2_demo?sslmode=disable"
for f in /var/tmp/m14s2demo.pid /var/tmp/m14s2fake.pid; do
  if [ -f "$f" ]; then kill "$(cat "$f")" 2>/dev/null || true; fi
done
sleep 1
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m14s2_demo WITH (FORCE)" -c "CREATE DATABASE m14s2_demo"
mkdir -p /var/tmp/m14s2 && rm -f /var/tmp/m14s2/requests.jsonl
go build -o /var/tmp/m14s2/subutai ./cmd/subutai
HERE="$(pwd)"
rm -rf /var/tmp/m14s2demo && mkdir -p /var/tmp/m14s2demo
cd /var/tmp/m14s2demo
git init -q && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
SUBUTAI_DATABASE_URL="$DB" /var/tmp/m14s2/subutai init
python3 - <<'PY'
p='.subutai/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: SUBUTAI_DATABASE_URL\n","database:\n  url_env: SUBUTAI_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8830\n  ui_actor: sam\n  heartbeat_seconds: 5\n",1)
s=s.replace("    api_key_env: ANTHROPIC_API_KEY\n","    api_key_env: ANTHROPIC_API_KEY\n    base_url: http://127.0.0.1:8831\n",1)
open(p,'w').write(s)
PY
git add -A && git commit -qm "subutai init"
nohup python3 "$HERE/docs/walkthrough-spec-021-stage2/fake_provider.py" 8831 /var/tmp/m14s2/requests.jsonl >/var/tmp/m14s2/fake.log 2>&1 &
echo $! > /var/tmp/m14s2fake.pid
NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost SUBUTAI_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy \
  nohup /var/tmp/m14s2/subutai serve >/var/tmp/m14s2/serve.log 2>&1 &
echo $! > /var/tmp/m14s2demo.pid
sleep 3
curl -sf --noproxy '*' -o /dev/null http://127.0.0.1:8830/ui && echo "serving on 127.0.0.1:8830"
