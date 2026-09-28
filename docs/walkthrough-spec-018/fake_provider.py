#!/usr/bin/env python3
"""A fake of the Messages API, for the SPEC-018 walkthrough. It is not a model:
every request is answered by calling the outcome tool the request offers, with
a fixed, valid input. Each request body is appended to a log, so the demo can
show exactly what the dispatched agent was sent.

    python3 fake_provider.py 8819 /var/tmp/m11/requests.jsonl
"""
import http.server
import json
import sys

PORT, LOG = int(sys.argv[1]), sys.argv[2]
INPUTS = {
    "submit_review": {"verdict": "approve", "reasoning": "The specification is testable and keeps to the project decisions it was given."},
    "submit_comments": {"reasoning": "Nothing to add.", "comments": []},
    "submit_estimate": {"tokens": 12000, "rationale": "A small form and its checks."},
}


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["content-length"])))
        with open(LOG, "a") as f:
            f.write(json.dumps(body) + "\n")
        tools = [t["name"] for t in body.get("tools", [])]
        name = next((t for t in tools if t.startswith("submit_")), tools[0] if tools else "none")
        reply = {
            "id": "msg_fake", "type": "message", "role": "assistant", "model": body.get("model", "fake"),
            "content": [{"type": "tool_use", "id": "toolu_fake", "name": name, "input": INPUTS.get(name, {})}],
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
