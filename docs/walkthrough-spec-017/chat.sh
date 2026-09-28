#!/usr/bin/env bash
# The chat side of the SPEC-017 walkthrough, as plain MCP JSON-RPC calls to
# POST /mcp: what a chat AI would send. Run after setup.sh. Prints each call
# and the parts of its answer that matter.
set -euo pipefail
MCP=http://127.0.0.1:8817/mcp
REPO=/var/tmp/m10demo
n=0
call() { # call <tool> <json-args> [python expression over the result r]
  n=$((n+1))
  local body
  body=$(printf '{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"%s","arguments":%s}}' "$n" "$1" "$2")
  echo "» $1 $2"
  curl -s --noproxy '*' -H 'Content-Type: application/json' -d "$body" "$MCP" | python3 -c '
import json,sys
resp=json.load(sys.stdin)
res=resp.get("result",{})
if res.get("isError"):
    print("  refused:", res["content"][0]["text"]); sys.exit(0)
r=res.get("structuredContent",{})
expr=sys.argv[1] if len(sys.argv)>1 else ""
out=eval(expr) if expr else r
print("  " + (json.dumps(out, ensure_ascii=False) if not isinstance(out,str) else out))
' "${3:-}"
}
rpc() {
  n=$((n+1))
  curl -s --noproxy '*' -H 'Content-Type: application/json' -d "{\"jsonrpc\":\"2.0\",\"id\":$n,\"method\":\"$1\"}" "$MCP"
}

echo "== The tools a chat agent sees"
rpc tools/list | python3 -c '
import json,sys
names=[t["name"] for t in json.load(sys.stdin)["result"]["tools"]]
print("  %d tools; new in M10: %s" % (len(names), ", ".join(n for n in names if n in ("submit_for_review","get_timeline","get_agent_run"))))'

echo "== Plan the work"
call create_initiative '{"slug":"greet","name":"Greetings","description":"Say hello to people, politely."}' \
  '{"id": r["id"], "design": r["design_document"]["path"], "written_by": r["design_document"]["written_by"]["sentence"]}'
call create_feature '{"initiative_path":"INIT-001","slug":"time","name":"Time-aware greeting","description":"Greet people by the time of day: good morning, good afternoon or good evening.","design_document":false}' \
  '{"id": r["id"], "path": r["path"]}'

echo "== Write the design with the person, and submit it"
DESIGN=$(ls "$REPO"/docs/work/INIT-001-*/INIT-001-design.md)
python3 - "$DESIGN" <<'PY'
import sys
p=sys.argv[1]
s=open(p).read()
head=s.split("\n---\n",1)[0]+"\n---\n"
open(p,"w").write(head+"""
# Greetings

## What this is for

People like being greeted in a way that fits the moment.

## The shape of it

A small greeting module, with one feature per kind of greeting.

## Decisions

- Greetings are plain text, in British English.
""")
PY
(cd "$REPO" && git add -A && git commit -qm "Greetings design")
call submit_for_review '{"document":"INIT-001-design"}' 'r["done"]'

echo "  (the person approves the design in the web UI; see walk.js)"
