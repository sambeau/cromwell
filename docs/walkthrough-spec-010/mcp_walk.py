"""SPEC-010 DoD 3: the same two-milestone roadmap for an initiative, over MCP."""
import json
import urllib.request

URL = "http://127.0.0.1:8810/mcp"
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


init = rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                          "clientInfo": {"name": "walkthrough", "version": "1"}})
print("instructions:", init["result"]["instructions"])
names = sorted(t["name"] for t in rpc("tools/list")["result"]["tools"])
print(f"\n{len(names)} tools:", ", ".join(names))

out = tool("create_feature", initiative_path="billing", slug="refunds", name="Refunds",
           description="A person can get their money back for an invoice that was charged in error.")
print("  ", out["path"], out["state"])

beta = tool("create_milestone", name="Billing beta", owner_path="billing", target_date="2027-01-31",
            description="Invoices go out, and a mistaken charge can be refunded.")
print("  ", beta["id"], "owner:", beta["owner"])
ga = tool("create_milestone", name="Billing GA", owner_path="billing")
print("  ", ga["id"], "owner:", ga["owner"])
tool("create_milestone", name="Oops", owner_path="payments")

rm = tool("create_roadmap", name="Billing plan", owner_path="billing")
print("  ", rm["id"], "owner:", rm["owner"])

for args in [
    dict(milestone="Billing beta", member_type="feature", member="billing/invoices"),
    dict(milestone="Billing beta", member_type="feature", member="billing/refunds"),
    dict(milestone="Billing GA", member_type="milestone", member="Billing beta"),
    dict(milestone="Billing GA", member_type="initiative", member="auth"),
]:
    out = tool("add_milestone_member", **args)
    if out:
        print("  ", out["change"], f"→ {out['items_done']} of {out['items_total']} items done")
tool("add_milestone_member", milestone="Billing beta", member_type="milestone", member="Billing GA")
out = tool("remove_milestone_member", milestone="Billing GA", member_type="initiative", member="auth",
           reason="Sign-in has its own release plan.")
print("  ", out["change"])

for args in [dict(milestone="Billing GA"), dict(milestone="Billing beta"),
             dict(milestone="Billing beta", position=1)]:
    out = tool("place_roadmap_entry", roadmap="Billing plan", **args)
    print("   order:", [f"{m['position']}. {m['name']}" for m in out["milestones"]])

lst = tool("list_milestones", owner_type="initiative", owner_path="billing")
print("   billing's milestones:", [m["name"] for m in lst["milestones"]])
everything = tool("list_milestones")
print("   every milestone:", [(m["name"], m["owner"].get("path", "project"), m["state"]) for m in everything["milestones"]])

detail = tool("get_milestone", milestone="Billing beta")
print("   members:", [(m["type"], m.get("path"), m["done"]) for m in detail["members"]])
print("   lock:", json.dumps(detail["lock"], indent=None))

road = tool("get_roadmap", roadmap="Billing plan")
print("   roadmap:", [f"{m['position']}. {m['name']} ({m['state']})" for m in road["milestones"]])

tool("lock_milestone", milestone="Billing beta")
tool("get_milestone", milestone="Auth beta")  # locked in the browser half
auth_beta = tool("get_milestone", milestone="Auth beta")
print("   Auth beta state:", auth_beta["state"], "locked_at:", auth_beta.get("locked_at"))
tool("add_milestone_member", milestone="Auth beta", member_type="feature", member="billing/refunds")
