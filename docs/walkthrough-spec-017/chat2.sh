#!/usr/bin/env bash
# The second half of the chat side: after the person has approved the design
# in the browser. Writes the spec in chat, adopts it, submits it, carries the
# person's approval (agent review is off in this demo project, so the spec
# waits for a person), and then plans a milestone, a roadmap and a checklist
# to show their IDs.
set -euo pipefail
MCP=http://127.0.0.1:8817/mcp
REPO=/var/tmp/m10demo
n=100
call() {
  n=$((n+1))
  local body
  body=$(printf '{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"%s","arguments":%s}}' "$n" "$1" "$2")
  echo "» $1 $2"
  curl -s --noproxy '*' -H 'Content-Type: application/json' -d "$body" "$MCP" | python3 -c '
import json,sys
res=json.load(sys.stdin).get("result",{})
if res.get("isError"):
    print("  refused:", res["content"][0]["text"]); sys.exit(0)
r=res.get("structuredContent",{})
expr=sys.argv[1] if len(sys.argv)>1 else ""
out=eval(expr) if expr else r
print("  " + (json.dumps(out, ensure_ascii=False) if not isinstance(out,str) else out))
' "${3:-}"
}

echo "== Write the spec in chat, and add it"
mkdir -p "$REPO/docs/specs"
cat > "$REPO/docs/specs/time-greeting.md" <<'MD'
---
title: Time-aware greeting
type: spec
owner: greet/time
---

# Time-aware greeting

## Overview

The greeting names the part of the day: morning, afternoon or evening.

## Behaviour

Before noon the greeting is "Good morning". From noon until six it is "Good
afternoon". From six onwards it is "Good evening". The time is the person's
local time.

## Acceptance criteria

- At 09:00 the greeting is "Good morning"
- At 14:00 the greeting is "Good afternoon"
- At 19:00 the greeting is "Good evening"
MD
(cd "$REPO" && git add -A && git commit -qm "Spec for the time-aware greeting, written in chat")
call adopt_document '{"path":"docs/specs/time-greeting.md","doc_type":"spec","owner_type":"feature","owner_path":"FEAT-001"}' \
  '{"id": r["id"], "state": r["state"], "written_by": r["written_by"]["sentence"]}'

echo "== Submit it, with no quote"
call submit_for_review '{"document":"FEAT-001-spec"}' 'r["done"]'
echo "== Submitting it again is refused: a fresh review is the person's call"
call submit_for_review '{"document":"FEAT-001-spec"}'
echo "== A relay still needs the person's words"
call relay_verdict '{"path":"FEAT-001-spec","verdict":"approve"}'
call relay_verdict '{"path":"FEAT-001-spec","verdict":"approve","quote":"Yes, that spec is exactly right. Approve it."}' \
  '{"done": r["done"], "last_verdict": r["last_verdict"]["sentence"]}'

echo "== Read the feature back"
call get_feature '{"path":"FEAT-001"}' '[{"id": d.get("id"), "written_by": d["written_by"]["sentence"], "last_verdict": d.get("last_verdict",{}).get("sentence")} for d in r["documents"]]'
call get_timeline '{"feature":"FEAT-001"}' '[m["label"] + " — " + m["who"] for m in r["moments"]]'

echo "== Plan a milestone, a roadmap and a checklist: IDs come back"
call create_milestone '{"name":"Greetings beta"}' '{"id": r["id"], "row_id": r["row_id"]}'
call add_milestone_member '{"milestone":"MS-001","member_type":"feature","member":"greet/time"}' 'r.get("done", r)'
call create_roadmap '{"name":"The road to hello"}' '{"id": r["id"]}'
call place_roadmap_entry '{"roadmap":"RM-001","milestone":"MS-001"}' 'r.get("done", r)'
call create_checklist '{"name":"Launch jobs","jobs":["Choose the typeface","Ask legal about the word evening"]}' '{"id": r["id"], "jobs": r["jobs_total"]}'
call add_milestone_member '{"milestone":"MS-001","member_type":"checklist","member":"CL-001"}' 'r.get("done", r)'
call relay_tick_job '{"checklist":"CL-001","job":"Choose the typeface","ticked":true,"quote":"We went with the humanist sans."}' 'r["done"]'
call get_milestone '{"milestone":"MS-001"}' '[(m.get("id"), m["type"], m["name"]) for m in r["members"]]'
