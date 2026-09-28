package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"cromwell/internal/store"
)

// The MCP facet suite (SPEC-008 NFR-4): the tool handlers driven over the real
// JSON-RPC endpoint against real Postgres, as the phase 1–3 suites are.

// rpc performs a JSON-RPC call against the MCP endpoint and returns the decoded
// response.
func (h *harness) rpc(method string, params any) rpcResponse {
	h.t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	enc, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	resp, err := http.Post(h.api.URL+"/mcp", "application/json", bytes.NewReader(enc))
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		h.t.Fatalf("decode MCP response: %v", err)
	}
	return out
}

// callTool invokes a tool and returns its structured result plus whether the
// tool reported an error.
func (h *harness) callTool(name string, args map[string]any) (map[string]any, bool, string) {
	h.t.Helper()
	resp := h.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	if resp.Error != nil {
		h.t.Fatalf("tools/call %s returned a protocol error: %s", name, resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		h.t.Fatalf("tools/call %s: unexpected result shape %T", name, resp.Result)
	}
	isErr, _ := result["isError"].(bool)
	text := ""
	if content, ok := result["content"].([]any); ok && len(content) > 0 {
		if first, ok := content[0].(map[string]any); ok {
			text, _ = first["text"].(string)
		}
	}
	structured, _ := result["structuredContent"].(map[string]any)
	return structured, isErr, text
}

// TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet is the DEC-004 line, made
// testable (SPEC-008 FR-4.1, DoD 3). The facet must advertise exactly the
// authoring writes and authoring reads — and no tool that starts a feature into
// development, transitions development lifecycle, overrides a gate, or spawns an
// agent. The enforcement is the absence of those tools, so this test IS the
// enforcement's regression guard.
func TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet(t *testing.T) {
	h := newHarness(t)

	resp := h.rpc("tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list: %s", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	rawTools := result["tools"].([]any)

	var got []string
	for _, rt := range rawTools {
		tool := rt.(map[string]any)
		name := tool["name"].(string)
		got = append(got, name)
		// NFR-5: every tool's description is written for a person to read.
		desc, _ := tool["description"].(string)
		if len(desc) < 40 {
			t.Errorf("tool %s has a thin description (%q); tool text is human-facing (NFR-5)", name, desc)
		}
		if tool["inputSchema"] == nil {
			t.Errorf("tool %s advertises no input schema", name)
		}
	}
	sort.Strings(got)

	want := []string{
		// SPEC-008: the planning-tree authoring slice.
		"attach_document",
		"create_feature",
		"create_initiative",
		"get_feature",
		"get_initiative",
		"get_tree",
		"list_documents",
		"update_feature",
		"update_initiative",
		// SPEC-010: milestones and roadmaps, as planning authoring under
		// DEC-004 Amendment 1 (SD-5). Each is named here on purpose.
		"add_milestone_member",
		"create_milestone",
		"create_roadmap",
		"get_milestone",
		"get_roadmap",
		"list_milestones",
		"list_roadmaps",
		"place_roadmap_entry",
		"remove_milestone_member",
		"remove_roadmap_entry",
		// Marking a milestone as shipped and reopening it: permitted by DEC-004
		// Amendment 1 because G4 is a record-keeping check, and both can be
		// undone (SD-4, SD-11). Added deliberately, as DEC-005 asks.
		"mark_milestone_shipped",
		"reopen_milestone",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("advertised tools = %v\nwant exactly %v (DEC-004, FR-4.1)", got, want)
	}

	// FR-4.2: a development-side name is rejected by the MCP layer as unknown,
	// not by a bespoke deny-check — because it does not exist.
	for _, forbidden := range []string{
		"start_feature", "transition_feature", "override_gate", "spawn_agent",
		"dispatch_review", "respond_checkpoint", "archive_initiative", "set_estimate",
	} {
		resp := h.rpc("tools/call", map[string]any{"name": forbidden, "arguments": map[string]any{}})
		if resp.Error == nil {
			t.Errorf("tool %q was callable; the DEC-004 line requires it not to exist", forbidden)
			continue
		}
		if resp.Error.Code != rpcMethodNotFound {
			t.Errorf("tool %q: error code %d, want method-not-found (%d)", forbidden, resp.Error.Code, rpcMethodNotFound)
		}
	}
}

// TestMCPInitializeHandshake covers FR-1.1: a client handshakes and lists tools.
func TestMCPInitializeHandshake(t *testing.T) {
	h := newHarness(t)
	resp := h.rpc("initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "1"},
	})
	if resp.Error != nil {
		t.Fatalf("initialize: %s", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	if result["protocolVersion"] != mcpProtocolVersion {
		t.Errorf("protocolVersion = %v, want %s", result["protocolVersion"], mcpProtocolVersion)
	}
	if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Error("the server should advertise the tools capability")
	}
}

// TestMCPAuthoringSlice covers FR-2 and FR-3: a chat agent stands up an
// initiative tree with features, titles and descriptions and an attached
// document — each through the same gated, audited service method the UI uses,
// attributed to the MCP actor (NFR-6) — and reads it back.
func TestMCPAuthoringSlice(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// FR-2.1: create a top-level initiative, then a nested one under it.
	out, isErr, msg := h.callTool("create_initiative", map[string]any{
		"slug": "auth", "name": "Authentication",
		"description": "How a person proves who they are when they sign in.",
	})
	if isErr {
		t.Fatalf("create_initiative: %s", msg)
	}
	if out["path"] != "auth" {
		t.Errorf("path = %v, want auth", out["path"])
	}
	if _, isErr, msg := h.callTool("create_initiative", map[string]any{
		"slug": "basic", "name": "Basic sign-in", "parent_path": "auth",
	}); isErr {
		t.Fatalf("nested create_initiative: %s", msg)
	}

	// FR-2.1 AC: a duplicate slug returns a clear, human-readable error rather
	// than a raw constraint violation (NFR-7).
	_, isErr, msg = h.callTool("create_initiative", map[string]any{"slug": "auth", "name": "Again"})
	if !isErr {
		t.Error("a duplicate slug should fail")
	}
	if !strings.Contains(msg, "already exists") || strings.Contains(msg, "SQLSTATE") {
		t.Errorf("duplicate-slug error should be plain language; got %q", msg)
	}

	// FR-2.2: create a feature. It starts as an idea and is NOT started into
	// development — the DEC-004 line.
	out, isErr, msg = h.callTool("create_feature", map[string]any{
		"initiative_path": "auth", "slug": "login", "name": "Login form",
		"description": "The form a person fills in to sign in.",
	})
	if isErr {
		t.Fatalf("create_feature: %s", msg)
	}
	if out["state"] != "idea" {
		t.Errorf("new feature state = %v, want idea (DEC-004: MCP does not start work)", out["state"])
	}

	// FR-2.3: update the description through the SAME additive service method
	// the UI's in-place edit calls (FR-6.1), leaving the name untouched.
	newDesc := "The screen where a returning person signs in with an email address and a password."
	out, isErr, msg = h.callTool("update_feature", map[string]any{
		"path": "auth/login", "description": newDesc,
	})
	if isErr {
		t.Fatalf("update_feature: %s", msg)
	}
	if out["name"] != "Login form" {
		t.Errorf("name should be unchanged by a description-only update; got %v", out["name"])
	}
	if out["description"] != newDesc {
		t.Errorf("description = %v, want the updated prose", out["description"])
	}

	// NFR-6: every mutation is on the audit trail, attributed to the MCP actor.
	f, err := h.srv.featureByPath(ctx, "auth/login")
	if err != nil {
		t.Fatal(err)
	}
	events, err := h.srv.Store.AuditTail(ctx, "feature", &f.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	var sawUpdate, sawCreate bool
	for _, e := range events {
		if e.Actor != "chat-agent" {
			t.Errorf("audit row %s attributed to %q, want the configured MCP actor", e.Kind, e.Actor)
		}
		switch e.Kind {
		case "feature.updated":
			sawUpdate = true
		case "feature.created":
			sawCreate = true
		}
	}
	if !sawCreate || !sawUpdate {
		t.Errorf("expected feature.created and feature.updated on the audit trail; got %v", events)
	}

	// FR-2.4: attach a document that exists in the repository.
	designPath := "docs/design/login.md"
	if err := os.MkdirAll(filepath.Join(h.root, "docs/design"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, designPath),
		[]byte("# Login design\n\nAn email field, a password field, and a button.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr, msg = h.callTool("attach_document", map[string]any{
		"path": designPath, "owner_type": "feature", "owner_path": "auth/login", "doc_type": "design",
	})
	if isErr {
		t.Fatalf("attach_document: %s", msg)
	}
	if out["type"] != "design" {
		t.Errorf("document type = %v, want design", out["type"])
	}

	// FR-2.4 AC: the attached document is eligible to be the entity's main
	// document — the browsing UI now renders it as the feature page's body.
	_, uiBody := h.getUI("/ui/f/auth/login")
	mustContain(t, "attached design doc is the page body", uiBody, "An email field, a password field")

	// Attaching a file that is not there explains what to do (NFR-5).
	_, isErr, msg = h.callTool("attach_document", map[string]any{
		"path": "docs/design/missing.md", "owner_type": "feature", "owner_path": "auth/login",
	})
	if !isErr {
		t.Error("attaching a non-existent file should fail")
	}
	if !strings.Contains(msg, "no file at") {
		t.Errorf("missing-file error should explain the problem; got %q", msg)
	}

	// FR-3.1: the tree read shows what was authored, enough to know where to create.
	out, isErr, msg = h.callTool("get_tree", nil)
	if isErr {
		t.Fatalf("get_tree: %s", msg)
	}
	inits := out["initiatives"].([]any)
	if len(inits) != 1 {
		t.Fatalf("get_tree returned %d roots, want 1", len(inits))
	}
	root := inits[0].(map[string]any)
	if root["path"] != "auth" || root["name"] != "Authentication" {
		t.Errorf("root = %v, want the auth initiative", root)
	}
	if len(root["features"].([]any)) != 1 {
		t.Errorf("auth should have one feature; got %v", root["features"])
	}
	if len(root["initiatives"].([]any)) != 1 {
		t.Errorf("auth should have one sub-initiative; got %v", root["initiatives"])
	}

	// FR-3.2 / FR-3.3: entity detail and the document list match the store.
	out, _, _ = h.callTool("get_feature", map[string]any{"path": "auth/login"})
	if out["description"] != newDesc {
		t.Errorf("get_feature description = %v, want the updated prose", out["description"])
	}
	if len(out["documents"].([]any)) != 1 {
		t.Errorf("get_feature should list the attached document; got %v", out["documents"])
	}
	out, _, _ = h.callTool("list_documents", map[string]any{
		"owner_type": "feature", "owner_path": "auth/login",
	})
	docs, err := store.DocumentsForOwner(ctx, h.srv.Store.Pool, "feature", &f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out["documents"].([]any)) != len(docs) {
		t.Errorf("list_documents returned %d, store has %d", len(out["documents"].([]any)), len(docs))
	}

	// NFR-5: a path that does not exist is explained, and points at the read tool.
	_, isErr, msg = h.callTool("get_feature", map[string]any{"path": "auth/nope"})
	if !isErr || !strings.Contains(msg, "get_tree") {
		t.Errorf("an unknown path should explain itself and suggest get_tree; got isError=%v %q", isErr, msg)
	}
}

// TestMCPMutationReachesTheUILive covers FR-5.1: an authoring mutation over MCP
// publishes the same kind of signal a UI mutation does, so an open browsing page
// reflects it — the two surfaces stay in sync.
func TestMCPMutationReachesTheUILive(t *testing.T) {
	h := newHarness(t)

	// Subscribe as a browser would, then author over MCP.
	ch, unsub := h.srv.Hub.Subscribe(context.Background())
	defer unsub()

	if _, isErr, msg := h.callTool("create_initiative", map[string]any{
		"slug": "billing", "name": "Billing", "description": "How the product charges for itself.",
	}); isErr {
		t.Fatalf("create_initiative: %s", msg)
	}

	select {
	case ev := <-ch:
		if ev.EventKind() != "initiative.changed" {
			t.Errorf("event kind = %q, want initiative.changed", ev.EventKind())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("an MCP authoring mutation should signal the UI within the timeout (FR-5.1)")
	}

	// And the new initiative is immediately browsable in the UI.
	code, body := h.getUI("/ui/i/billing")
	if code != 200 {
		t.Fatalf("GET /ui/i/billing after MCP create: %d", code)
	}
	mustContain(t, "MCP-authored initiative is browsable", body, "How the product charges for itself")
}
