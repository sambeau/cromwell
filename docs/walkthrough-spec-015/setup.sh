#!/usr/bin/env bash
# Set up the SPEC-015 browser walkthrough: a throwaway project in
# /var/tmp/m8demo, served on 127.0.0.1:8815 against a local Postgres, with a
# dummy provider key. No AI provider is used. Run from the repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m8_demo?sslmode=disable"
if [ -f /var/tmp/m8demo.pid ]; then kill "$(cat /var/tmp/m8demo.pid)" 2>/dev/null || true; sleep 1; fi
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m8_demo WITH (FORCE)" -c "CREATE DATABASE m8_demo"
mkdir -p /var/tmp/m8
go build -o /var/tmp/m8/subutai ./cmd/subutai
rm -rf /var/tmp/m8demo && mkdir -p /var/tmp/m8demo
REPO="$PWD"
cd /var/tmp/m8demo
git init -q && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
SUBUTAI_DATABASE_URL="$DB" /var/tmp/m8/subutai init
python3 - <<'PY'
p='.subutai/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: SUBUTAI_DATABASE_URL\n","database:\n  url_env: SUBUTAI_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8815\n  ui_actor: sam\n",1)
open(p,'w').write(s)
PY
# Two existing files to adopt where they sit: this repository's own DEC-005,
# exactly as it is, and a design note written before Subutai came along.
mkdir -p docs/decisions docs/notes
cp "$REPO/docs/decisions/DEC-005-the-orchestration-boundary.md" docs/decisions/
cat > docs/notes/session-tokens.md <<'MD'
---
title: "Session tokens"
---

# Session tokens

Written before this project used Subutai. Tokens live for a day and are
rotated on every sign-in.
MD
git add -A && git commit -qm "existing documents"
SUBUTAI_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy nohup /var/tmp/m8/subutai serve >/var/tmp/m8/serve.log 2>&1 &
echo $! > /var/tmp/m8demo.pid
sleep 3
curl -sf -o /dev/null http://127.0.0.1:8815/ui && echo "serving on 127.0.0.1:8815"
