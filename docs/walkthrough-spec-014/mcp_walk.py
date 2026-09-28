"""SPEC-014 DoD 4: a checklist kept over MCP, and a tick relayed with the person's words.

Run after `walk.js . a`, against the same server. Plain JSON-RPC to POST /mcp.
"""
import json
import urllib.request

URL = "http://127.0.0.1:8815/mcp"
n = 0


def rpc(method, params=None):
    global n
    n += 1
    body = {"jsonrpc": "2.0", "id": n, "method": method}
    if params is not None:
        body["params"] = params
    req = urllib.request.Request(URL, json.dumps(body).encode(), {"Content-Type": "application/json"})
    with urllib.request.urlopen(req) as r:
        return json.load(r)


def tool(tool_name, **args):
    resp = rpc("tools/call", {"name": tool_name, "arguments": args})
    shown = " ".join(f"{k}={json.dumps(v)}" for k, v in args.items())
    print(f"\n→ {tool_name} {shown}")
    if "error" in resp:
        print(f"  protocol error {resp['error']['code']}: {resp['error']['message']}")
        return None
    res = resp["result"]
    if res.get("isError"):
        print(f"  refused: {res['content'][0]['text']}")
        return None
    return res["structuredContent"]


def jobs(cl):
    for j in cl["jobs"]:
        mark = "x" if j["ticked"] else " "
        extra = f" — ticked by {j['ticked_by']} via {j['ticked_via']}" if j["ticked"] else ""
        if j.get("ticked_quote"):
            extra += f", quoting “{j['ticked_quote']}”"
        print(f"  {j['position']}. [{mark}] {j['title']}{extra}")


init = rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                          "clientInfo": {"name": "walkthrough", "version": "1"}})
print("instructions:", init["result"]["instructions"])
names = sorted(t["name"] for t in rpc("tools/list")["result"]["tools"])
print(f"\n{len(names)} tools:", ", ".join(names))

# Planning: a checklist with its first jobs, and a milestone to put it in.
cl = tool("create_checklist", name="Store listing", owner_path="auth",
          description="What the app store needs before sign-in can ship.",
          jobs=["Write the store description", "Take the screenshots"])
jobs(cl)
cl = tool("add_job", checklist="Store listing", title="Choose the age rating", position=2)
jobs(cl)
m = tool("create_milestone", name="Sign-in GA", owner_path="auth")
tool("add_milestone_member", milestone="Sign-in GA", member_type="feature", member="auth/login")
m = tool("add_milestone_member", milestone="Sign-in GA", member_type="checklist", member="Store listing")
print(f"  {m['change']} Items: {m['items_done']} of {m['items_total']} done")

# The relay: refused without the person's words; with them, recorded as theirs.
tool("relay_tick_job", checklist="Store listing", job="Write the store description", ticked=True)
out = tool("relay_tick_job", checklist="Store listing", job="Write the store description", ticked=True,
           note="Approved by marketing.", quote="The store description is written and marketing signed it off")
print(f"  {out['done']}; checklist {out['checklist']['jobs_ticked']} of {out['checklist']['jobs_total']} ticked")
tool("relay_tick_job", checklist="Store listing", job="Choose the age rating", ticked=True,
     quote="We went with 4+")
out = tool("relay_tick_job", checklist="Store listing", job="Choose the age rating", ticked=False,
           quote="Hold on, legal want 12+ because of the chat feature")
print(f"  {out['done']}")
out = tool("relay_tick_job", checklist="Store listing", job="Choose the age rating", ticked=True,
           note="12+, on legal's advice.", quote="Age rating is done, it's 12+")
print(f"  {out['done']}")

# Planning tools can't stand in for a tick.
tool("rename_job", checklist="Store listing", job="Choose the age rating", title="Take the screenshots")
tool("remove_job", checklist="Store listing", job="Take the screenshots")

# The milestone reads back with the checklist counted and not done.
m = tool("get_milestone", milestone="Sign-in GA")
print(f"  items: {m['items_done']} of {m['items_total']} done")
for mem in m["members"]:
    print(f"  - {mem['type']} {mem['name']}: {'done' if mem['done'] else 'not done'}")
print("  shipping:", json.dumps(m["shipping"]))
jobs(tool("get_checklist", checklist="Store listing"))
