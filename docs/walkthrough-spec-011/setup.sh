#!/usr/bin/env bash
# Set up the SPEC-011 browser walkthrough: a throwaway project in
# /var/tmp/m3demo, served on 127.0.0.1:8811 against a local Postgres, with a
# dummy provider key. No AI provider is used. Run from the repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m3_demo?sslmode=disable"
pkill -f "cromwell-m3 serve" || true
sleep 1
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m3_demo WITH (FORCE)" -c "CREATE DATABASE m3_demo"
go build -o /var/tmp/cromwell-m3 ./cmd/cromwell
rm -rf /var/tmp/m3demo && mkdir -p /var/tmp/m3demo && cd /var/tmp/m3demo
git init -q && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
CROMWELL_DATABASE_URL="$DB" /var/tmp/cromwell-m3 init
python3 - <<'PY'
p='.cromwell/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: CROMWELL_DATABASE_URL\n","database:\n  url_env: CROMWELL_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8811\n  socket: /tmp/m3demo.sock\n  ui_actor: sam\n",1)
open(p,'w').write(s)
PY
mkdir -p docs/greet
cat > docs/greet/design.md <<'MD'
---
title: "Greetings"
type: design
owner: "greet"
---

# Greetings

## What this is for

A small Go package that greets people and tells them the time.

## The shape of it

Two functions, `Hello(name)` and `Now()`, each in its own file.

## Decisions

- Every time is in UTC, because the package has no idea where its caller is.
MD
cat > docs/greet/notes.md <<'MD'
---
title: "Stray notes"
type: note
owner: "greet"
---

# Stray notes

Attached to the wrong place by mistake.
MD
git add -A && git commit -qm "demo docs"
CROMWELL_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy nohup /var/tmp/cromwell-m3 serve >/var/tmp/m3demo-serve.log 2>&1 &
sleep 3
curl -sf -o /dev/null http://127.0.0.1:8811/ui && echo "serving on 127.0.0.1:8811"
