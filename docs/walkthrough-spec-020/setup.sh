#!/usr/bin/env bash
# Set up the SPEC-020 walkthrough: a throwaway project in /var/tmp/m13demo,
# served on 127.0.0.1:8820 against a local Postgres. The provider key is a
# dummy, so no model runs: the dispatched steps (code review, verification)
# are proved by the mock-provider tests (see the walkthrough). The project's
# spending limit is spent before it starts, so dispatched work waits in the
# queue, where it can be seen, instead of calling a model.
#
# It plans a feature with a spec and a two-task plan, submits both, and has a
# person approve each directly, so the feature is ready to build. Pressing
# Start building is left to the walkthrough. Run from the repository root.
set -euo pipefail
PG="${PG:-postgres://postgres@localhost:54329}"
DB="$PG/m13_demo?sslmode=disable"
API=http://127.0.0.1:8820
if [ -f /var/tmp/m13demo.pid ]; then kill "$(cat /var/tmp/m13demo.pid)" 2>/dev/null || true; sleep 1; fi
psql "$PG/postgres" -qc "DROP DATABASE IF EXISTS m13_demo WITH (FORCE)" -c "CREATE DATABASE m13_demo"
mkdir -p /var/tmp/m13
go build -o /var/tmp/m13/subutai ./cmd/subutai
rm -rf /var/tmp/m13demo && mkdir -p /var/tmp/m13demo
cd /var/tmp/m13demo
git init -q -b main && git config user.name Demo && git config user.email demo@example.com
git commit -qm init --allow-empty
SUBUTAI_DATABASE_URL="$DB" /var/tmp/m13/subutai init >/dev/null
python3 - <<'PY'
p='.subutai/config.yaml'
s=open(p).read()
s=s.replace("database:\n  url_env: SUBUTAI_DATABASE_URL\n","database:\n  url_env: SUBUTAI_DATABASE_URL\n\nserver:\n  http: 127.0.0.1:8820\n  ui_actor: sam\n  heartbeat_seconds: 2\n",1)
open(p,'w').write(s)
PY
mkdir -p docs/specs docs/plans
cat > docs/specs/greeting.md <<'MD'
---
title: Time-aware greeting
type: spec
owner: greet/time
---

# Time-aware greeting

## Overview

The home page greets people by the time of day.

## Behaviour

Before noon it says good morning; from noon to six, good afternoon; after
six, good evening. The greeting is plain text.

## Acceptance criteria

- At 09:00 the greeting is "Good morning"
- At 15:00 the greeting is "Good afternoon"
- At 19:00 the greeting is "Good evening"
MD
cat > docs/plans/greeting.md <<'MD'
---
title: Time-aware greeting — dev plan
type: dev_plan
owner: greet/time
---

# Time-aware greeting — dev plan

## Approach

A pure function chooses the greeting from the hour, and the page calls it.

## Tasks

| id | title | depends_on | description |
|----|-------|------------|-------------|
| T1 | Greeting by the hour | | Add greet.go with Greeting(hour int) string, returning the three greetings |
| T2 | Show it on the home page | | Add home.go that prints Greeting for the current hour |
MD
git add -A && git commit -qm "subutai init, and the greeting's spec and plan"
# Spend the budget, so the governor holds dispatched work in the queue.
psql "$DB" -qc "INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key, cost_usd, finished_at)
  VALUES (gen_random_uuid(), 'succeeded', 'estimate', 'estimator', 'claude-sonnet-5', 'project', gen_random_uuid(), 'demo-spent', 1000, now())"
SUBUTAI_DATABASE_URL="$DB" ANTHROPIC_API_KEY=dummy nohup /var/tmp/m13/subutai serve >/var/tmp/m13/serve.log 2>&1 &
echo $! > /var/tmp/m13demo.pid
sleep 3
post() { curl -sf --noproxy '*' -H 'Content-Type: application/json' -d "$2" "$API$1" >/dev/null; }
post /api/initiatives '{"slug":"greet","name":"Greetings"}'
post /api/features '{"initiative_path":"greet","slug":"time","name":"Time-aware greeting"}'
for doc in "docs/specs/greeting.md spec" "docs/plans/greeting.md dev_plan"; do
  set -- $doc
  post /api/docs "{\"path\":\"$1\",\"type\":\"$2\",\"owner_type\":\"feature\",\"owner_ref\":\"greet/time\"}"
  post /api/docs/submit "{\"path\":\"$1\"}"
  id=$(psql "$DB" -tAc "SELECT id FROM documents WHERE path = '$1'")
  # A person approves it directly, as the document page's Approve does.
  curl -sf --noproxy '*' -o /dev/null -d "doc_id=$id" "$API/ui/document/approve"
done
sleep 2
psql "$DB" -tAc "SELECT public_id || ' ' || state FROM features"
psql "$DB" -tAc "SELECT public_id || ' ' || state FROM tasks ORDER BY position"
echo "serving on $API"
