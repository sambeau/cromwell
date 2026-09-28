#!/usr/bin/env bash
# Set up the SPEC-018 walkthrough: a throwaway project in /var/tmp/m11demo,
# served on 127.0.0.1:8818 against a local Postgres. No AI provider is used:
# the provider's base_url points at fake_provider.py on 127.0.0.1:8819, which
# answers every request by calling the offered outcome tool and logs what it
# was sent. This repository's DEC-001 to DEC-007 are copied in, committed and
# left for the walk to adopt. Run from the repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m11_demo?sslmode=disable"
for f in /var/tmp/m11demo.pid /var/tmp/m11fake.pid; do
  if [ -f "$f" ]; then kill "$(cat "$f")" 2>/dev/null || true; fi
done
sleep 1
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m11_demo WITH (FORCE)" -c "CREATE DATABASE m11_demo"
mkdir -p /var/tmp/m11 && rm -f /var/tmp/m11/requests.jsonl
go build -o /var/tmp/m11/subutai ./cmd/subutai
HERE="$(pwd)"
rm -rf /var/tmp/m11demo && mkdir -p /var/tmp/m11demo
cd /var/tmp/m11demo
git init -q && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
SUBUTAI_DATABASE_URL="$DB" /var/tmp/m11/subutai init
python3 - <<'PY'
p='.subutai/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: SUBUTAI_DATABASE_URL\n","database:\n  url_env: SUBUTAI_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8818\n  ui_actor: sam\n",1)
s=s.replace("    api_key_env: ANTHROPIC_API_KEY\n","    api_key_env: ANTHROPIC_API_KEY\n    base_url: http://127.0.0.1:8819\n",1)
open(p,'w').write(s)
PY
mkdir -p docs/decisions && cp "$HERE"/docs/decisions/DEC-00*.md docs/decisions/
git add -A && git commit -qm "subutai init; the project's decisions so far"
nohup python3 "$HERE/docs/walkthrough-spec-018/fake_provider.py" 8819 /var/tmp/m11/requests.jsonl >/var/tmp/m11/fake.log 2>&1 &
echo $! > /var/tmp/m11fake.pid
NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost SUBUTAI_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy \
  nohup /var/tmp/m11/subutai serve >/var/tmp/m11/serve.log 2>&1 &
echo $! > /var/tmp/m11demo.pid
sleep 3
curl -sf --noproxy '*' -o /dev/null http://127.0.0.1:8818/ui && echo "serving on 127.0.0.1:8818"
