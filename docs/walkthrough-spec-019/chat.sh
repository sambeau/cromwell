#!/usr/bin/env bash
# The chat side of the SPEC-019 walkthrough, as plain MCP JSON-RPC calls to
# POST /mcp: what a chat AI would send. Run after setup.sh. Prints each call
# and the parts of its answer that matter.
set -euo pipefail
MCP=http://127.0.0.1:8819/mcp
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
print("  %d tools; new in M12: %s" % (len(names), ", ".join(n for n in names if n in ("report_bug","list_bugs","get_bug","relay_triage"))))'

echo "== Plan the work"
call create_initiative '{"slug":"greet","name":"Greetings","description":"Say hello to people, politely.","design_document":false}' '{"id": r["id"]}'
call create_feature '{"initiative_path":"INIT-001","slug":"time","name":"Time-aware greeting","description":"Greet people by the time of day.","design_document":false}' '{"id": r["id"], "path": r["path"]}'

echo "== Report a bug the person mentioned, with no quote: reporting is planning"
call report_bug '{"on":"FEAT-001","title":"The evening greeting says good morning","steps":"Set the clock to 19:00\nOpen the home page","expected":"It says good evening.","actual":"It says good morning.","notes":"Log line: {{.Hour}} = 19, TODO check the time zone"}' \
  '{"id": r["id"], "triage": r["triage"], "report": r["report"], "next": r["next"]}'

echo "== A relay without the person's words is refused"
call relay_triage '{"bug":"BUG-001","decision":"accept"}'
