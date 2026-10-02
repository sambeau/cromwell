#!/usr/bin/env bash
# Set up the SPEC-021 walkthrough: a throwaway project in /var/tmp/m14demo,
# served on 127.0.0.1:8820 against a local Postgres. No AI provider is used:
# the provider's base_url points at fake_provider.py on 127.0.0.1:8821, which
# plays a spike's agent from a script and logs what it was sent. Run from the
# repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m14_demo?sslmode=disable"
for f in /var/tmp/m14demo.pid /var/tmp/m14fake.pid; do
  if [ -f "$f" ]; then kill "$(cat "$f")" 2>/dev/null || true; fi
done
sleep 1
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m14_demo WITH (FORCE)" -c "CREATE DATABASE m14_demo"
mkdir -p /var/tmp/m14 && rm -f /var/tmp/m14/requests.jsonl
go build -o /var/tmp/m14/subutai ./cmd/subutai
HERE="$(pwd)"
rm -rf /var/tmp/m14demo && mkdir -p /var/tmp/m14demo
cd /var/tmp/m14demo
git init -q && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
SUBUTAI_DATABASE_URL="$DB" /var/tmp/m14/subutai init
python3 - <<'PY'
p='.subutai/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: SUBUTAI_DATABASE_URL\n","database:\n  url_env: SUBUTAI_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8820\n  ui_actor: sam\n",1)
s=s.replace("    api_key_env: ANTHROPIC_API_KEY\n","    api_key_env: ANTHROPIC_API_KEY\n    base_url: http://127.0.0.1:8821\n",1)
open(p,'w').write(s)
PY
git add -A && git commit -qm "subutai init"
nohup python3 "$HERE/docs/walkthrough-spec-021/fake_provider.py" 8821 /var/tmp/m14/requests.jsonl >/var/tmp/m14/fake.log 2>&1 &
echo $! > /var/tmp/m14fake.pid
NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost SUBUTAI_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy \
  nohup /var/tmp/m14/subutai serve >/var/tmp/m14/serve.log 2>&1 &
echo $! > /var/tmp/m14demo.pid
sleep 3
curl -sf --noproxy '*' -o /dev/null http://127.0.0.1:8820/ui && echo "serving on 127.0.0.1:8820"
