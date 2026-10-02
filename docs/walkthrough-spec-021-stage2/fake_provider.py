#!/usr/bin/env python3
"""A fake of the Messages API, for the SPEC-021 walkthrough. It is not a model.

A spike's run is answered like this:
- its first turn saves a draft with save_findings;
- a first spike then keeps listing files and never finishes, so it runs into
  its token budget and is stopped there;
- a spike that asks again (its prompt has "# What the last spike found")
  finishes on its second turn with finish_spike.

Any other dispatch is answered by calling the outcome tool it is offered.
Each request body is appended to a log, so the demo can show what was sent.

    python3 fake_provider.py 8821 /var/tmp/m14/requests.jsonl
"""
import http.server
import json
import sys

PORT, LOG = int(sys.argv[1]), sys.argv[2]

DRAFT = """## Answer

Not yet: the first page of results came back in 180 ms, but nothing past it was measured.

## What we found

- The list endpoint pages at 100 items, and the first page came back in 180 ms.
"""

FINAL = """## Answer

Yes. The list endpoint returns 10,000 items in 1.4 seconds across 100 pages, well inside the two-second target.

## What we found

- Pages of 100 items each came back in 120 to 190 ms.
- Nothing slowed down past the first page.

## How we found out

A throwaway script in the working copy timed every page of a 10,000-item list.

## What to do next

Write the design for the import screen on the assumption that the list is fast enough to fetch whole.
"""


def text_of(body):
    out = []
    for m in body.get("messages", []):
        c = m.get("content")
        if isinstance(c, str):
            out.append(c)
        else:
            for b in c or []:
                if b.get("type") == "text":
                    out.append(b.get("text", ""))
    return "\n".join(out)


def turns(body):
    return sum(1 for m in body.get("messages", []) if m.get("role") == "assistant")


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["content-length"])))
        with open(LOG, "a") as f:
            f.write(json.dumps(body) + "\n")
        tools = [t["name"] for t in body.get("tools", [])]
        n = turns(body)
        if "save_findings" in tools:
            again = "# What the last spike found" in text_of(body)
            if n == 0:
                name, args = "save_findings", {"findings": FINAL if again else DRAFT}
            elif again:
                name, args = "finish_spike", {"findings": FINAL}
            else:
                name, args = "list_files", {"path": "."}
        else:
            name = next((t for t in tools if t.startswith("submit_")), tools[0] if tools else "none")
            args = {}
        reply = {
            "id": "msg_fake", "type": "message", "role": "assistant", "model": body.get("model", "fake"),
            "content": [{"type": "tool_use", "id": "toolu_fake_%d" % n, "name": name, "input": args}],
            "stop_reason": "tool_use", "stop_sequence": None,
            "usage": {"input_tokens": len(json.dumps(body)) // 4, "output_tokens": 40},
        }
        out = json.dumps(reply).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)

    def log_message(self, *args):
        pass


http.server.HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
