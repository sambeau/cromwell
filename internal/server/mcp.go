package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// The MCP facet, slice 1 (SPEC-008): the interface a human's chat agent uses to
// author Cromwell's planning layer — creating initiatives and features, setting
// their titles and descriptions in human prose, and attaching documents.
//
// It is a THIRD rendering over the same phase 1–3 service layer the web UI and
// the CLI use (DESIGN-007 §4, SD-4): every tool calls an existing service
// method directly, in-process, in one transaction with its audit row (O-3). No
// handler here issues HTTP to /api/* (NFR-2), and the facet gains no power the
// UI lacks (NFR-3).
//
// **The authority line is enforced by omission** (DEC-004, SD-2, FR-4): there is
// no tool to start a feature into development, transition development-side
// lifecycle, override a gate, or spawn an agent. You cannot call what is not
// registered, which is stronger and simpler than a runtime deny-check.
//
// The protocol is JSON-RPC 2.0 over the existing TCP listener (SD-5) — the
// Streamable HTTP transport's single-POST shape, hand-rolled rather than
// vendored, so the one-binary story holds with no new dependency (NFR-1).

// mcpProtocolVersion is the MCP revision this facet implements.
const mcpProtocolVersion = "2025-06-18"

// --- JSON-RPC envelopes ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // absent for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes used here.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603
)

// --- Tool definitions ---

// mcpTool is one advertised tool: its name, the plain-language description the
// chat agent reads (and relays to a person, NFR-5), its JSON Schema, and the
// handler that runs it.
type mcpTool struct {
	Name        string
	Description string
	Schema      map[string]any
	Handler     func(r *http.Request, args map[string]any) (any, error)
}

// mcpTools builds the tool registry. This function IS the DEC-004 boundary: the
// set it returns is exactly the authoring writes (FR-2) and the authoring reads
// (FR-3), plus the milestone and roadmap tools of SPEC-010 (mcpPlanTools), plus
// the four relay tools of SPEC-011 (mcpRelayTools, which carry a person's
// quoted decision and never send or start building), and nothing else. A
// development-side or gate tool would have to be added here to exist at all,
// which is what makes the line reviewable.
func (s *Server) mcpTools() []mcpTool {
	return append([]mcpTool{
		// --- The writes (FR-2) ---
		{
			Name: "create_initiative",
			Description: "Create an initiative — a branch of the project's planning tree that can " +
				"hold features or further sub-initiatives. Give it a short slug for its address, a " +
				"human-readable name, and a description in plain prose explaining what the work is " +
				"for. To nest it, pass the parent initiative's path; leave the parent out to create " +
				"a top-level initiative.",
			Schema: objectSchema(map[string]any{
				"slug":        stringProp("A short lower-case identifier used in the initiative's address, for example \"auth\"."),
				"name":        stringProp("The human-readable name, for example \"Authentication\"."),
				"description": stringProp("A short description in plain prose, written for a person to read."),
				"parent_path": stringProp("Optional. The path of the parent initiative, for example \"auth/basic\". Omit for a top-level initiative."),
			}, "slug", "name"),
			Handler: s.mcpCreateInitiative,
		},
		{
			Name: "create_feature",
			Description: "Create a feature under an initiative. The feature starts as an idea; it is " +
				"not started into development — a person does that from the command centre when its " +
				"specification has been approved.",
			Schema: objectSchema(map[string]any{
				"initiative_path": stringProp("The path of the initiative that will own this feature, for example \"auth\"."),
				"slug":            stringProp("A short lower-case identifier used in the feature's address, for example \"login\"."),
				"name":            stringProp("The human-readable name, for example \"Login form\"."),
				"description":     stringProp("A short description in plain prose, written for a person to read."),
			}, "initiative_path", "slug", "name"),
			Handler: s.mcpCreateFeature,
		},
		{
			Name: "update_initiative",
			Description: "Change an initiative's name or description. Pass only the fields you want " +
				"to change; anything you leave out is kept as it is. The description is the human " +
				"summary shown at the top of the initiative's page, so write it in plain prose.",
			Schema: objectSchema(map[string]any{
				"path":        stringProp("The path of the initiative, for example \"auth/basic\"."),
				"name":        stringProp("Optional. A new human-readable name."),
				"description": stringProp("Optional. A new description, in plain prose."),
			}, "path"),
			Handler: s.mcpUpdateInitiative,
		},
		{
			Name: "update_feature",
			Description: "Change a feature's name or description. Pass only the fields you want to " +
				"change; anything you leave out is kept as it is. The description is the human " +
				"summary shown at the top of the feature's page, so write it in plain prose.",
			Schema: objectSchema(map[string]any{
				"path":        stringProp("The path of the feature, for example \"auth/login\"."),
				"name":        stringProp("Optional. A new human-readable name."),
				"description": stringProp("Optional. A new description, in plain prose."),
			}, "path"),
			Handler: s.mcpUpdateFeature,
		},
		{
			Name: "attach_document",
			Description: "Register a Markdown file that already exists in the repository as a " +
				"document belonging to an initiative or a feature. This records and indexes the " +
				"file; it does not write it. Write the file with your own editing tools and commit " +
				"it first, then attach it here. A document of type \"design\" becomes the body of " +
				"the owning entity's page.",
			Schema: objectSchema(map[string]any{
				"path":       stringProp("The file's path within the repository, for example \"docs/design/login.md\"."),
				"owner_type": stringProp("Which kind of thing owns the document: \"project\", \"initiative\" or \"feature\"."),
				"owner_path": stringProp("The path of the owning initiative or feature. Omit when the owner is the project."),
				"doc_type":   stringProp("The kind of document: design, research, note, report, spec, dev_plan or policy. Defaults to design."),
			}, "path", "owner_type"),
			Handler: s.mcpAttachDocument,
		},

		// --- The reads authoring needs (FR-3) ---
		{
			Name: "get_tree",
			Description: "Return the project's planning tree — every initiative with the features " +
				"beneath it, each with its path, name and state. Use this to see where to create " +
				"something and what already exists.",
			Schema:  objectSchema(map[string]any{}),
			Handler: s.mcpGetTree,
		},
		{
			Name: "get_initiative",
			Description: "Return one initiative in detail: its name, description, the " +
				"sub-initiatives and features directly beneath it, and the documents attached to it.",
			Schema: objectSchema(map[string]any{
				"path": stringProp("The path of the initiative, for example \"auth\"."),
			}, "path"),
			Handler: s.mcpGetInitiative,
		},
		{
			Name:        "get_feature",
			Description: "Return one feature in detail: its name, description, lifecycle state, and the documents attached to it.",
			Schema: objectSchema(map[string]any{
				"path": stringProp("The path of the feature, for example \"auth/login\"."),
			}, "path"),
			Handler: s.mcpGetFeature,
		},
		{
			Name:        "list_documents",
			Description: "List the documents attached to an initiative or a feature, with each one's kind and review state.",
			Schema: objectSchema(map[string]any{
				"owner_type": stringProp("Which kind of thing owns the documents: \"project\", \"initiative\" or \"feature\"."),
				"owner_path": stringProp("The path of the owning initiative or feature. Omit when the owner is the project."),
			}, "owner_type"),
			Handler: s.mcpListDocuments,
		},
	}, append(s.mcpPlanTools(), s.mcpRelayTools()...)...)
}

// mcpToolNames is the sorted list of advertised tool names — the assertion
// surface for the DEC-004 line (FR-4.1).
func (s *Server) mcpToolNames() []string {
	tools := s.mcpTools()
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names
}

// --- Schema helpers ---

func objectSchema(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// --- Transport (FR-1.1) ---

// mcpRoutes mounts the MCP endpoint on the same mux as /api/* and /ui/*. Like
// the web UI, it is reachable only over the TCP listener: the CLI (the socket's
// only client) never requests it. With server.http unset the port is simply
// closed and the facet is unavailable, exactly as SPEC-004 FR-1.1 describes for
// the UI.
func (s *Server) mcpRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /mcp", s.handleMCP)
	// A GET is the Streamable HTTP transport's optional server-to-client stream.
	// This slice has no server-initiated messages, so it is declined cleanly
	// rather than left to look like a broken endpoint.
	mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "this server does not open a server-to-client stream", http.StatusMethodNotAllowed)
	})
}

// handleMCP is the JSON-RPC entry point. It answers initialize, tools/list and
// tools/call, and rejects everything else as an unknown method — including any
// development-side tool name, which is refused by the MCP layer itself rather
// than by a bespoke deny-check (FR-4.2).
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeRPCError(w, nil, rpcParseError, "the request body could not be read")
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPCError(w, nil, rpcParseError, "the request was not valid JSON")
		return
	}
	if req.JSONRPC != "2.0" {
		writeRPCError(w, req.ID, rpcInvalidRequest, "every request must set jsonrpc to \"2.0\"")
		return
	}

	// Notifications (no id) expect no response body.
	isNotification := len(req.ID) == 0
	if isNotification {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "initialize":
		writeRPCResult(w, req.ID, map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "cromwell", "version": "1"},
			"instructions": "Cromwell's planning surface. You can create and shape initiatives, " +
				"features, titles, descriptions and document attachments, and plan with milestones " +
				"and roadmaps: create them, fill them, order them, and mark a milestone as shipped " +
				"when the person says it has gone out. You can also carry a person's " +
				"decisions on documents — a verdict, an issue, a request for an agent review, or " +
				"letting the reviewer decide on a held specification — always quoting their words; " +
				"you hold no verdict of your own. You cannot send work to development, start " +
				"building, override a gate or answer the Inbox — a person does " +
				"those from the command centre.",
		})
	case "ping":
		writeRPCResult(w, req.ID, map[string]any{})
	case "tools/list":
		tools := s.mcpTools()
		out := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			out = append(out, map[string]any{
				"name": t.Name, "description": t.Description, "inputSchema": t.Schema,
			})
		}
		writeRPCResult(w, req.ID, map[string]any{"tools": out})
	case "tools/call":
		s.handleMCPToolCall(w, r, req)
	default:
		writeRPCError(w, req.ID, rpcMethodNotFound, fmt.Sprintf("this server does not support the method %q", req.Method))
	}
}

// handleMCPToolCall dispatches to a registered tool. An unregistered name — a
// development or gate tool, say — is rejected here as unknown; there is no
// permission layer to consult, because the tool does not exist (FR-4.2).
func (s *Server) handleMCPToolCall(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeRPCError(w, req.ID, rpcInvalidParams, "the tool call parameters were not valid")
		return
	}
	for _, t := range s.mcpTools() {
		if t.Name != params.Name {
			continue
		}
		result, err := t.Handler(r, params.Arguments)
		if err != nil {
			// A tool-level failure is reported as an unsuccessful tool result
			// rather than a protocol error, so the chat agent can read the
			// message, adjust, and try again (NFR-5, NFR-7).
			writeRPCResult(w, req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			})
			return
		}
		encoded, jerr := json.MarshalIndent(result, "", "  ")
		if jerr != nil {
			writeRPCError(w, req.ID, rpcInternalError, "the result could not be encoded")
			return
		}
		writeRPCResult(w, req.ID, map[string]any{
			"content":           []map[string]any{{"type": "text", "text": string(encoded)}},
			"structuredContent": result,
		})
		return
	}
	writeRPCError(w, req.ID, rpcMethodNotFound,
		fmt.Sprintf("there is no tool called %q on this server", params.Name))
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

// --- Argument helpers ---

// argString reads a string argument, trimmed. ok is false when absent or empty.
func argString(args map[string]any, key string) (string, bool) {
	v, present := args[key]
	if !present {
		return "", false
	}
	str, isStr := v.(string)
	if !isStr {
		return "", false
	}
	str = strings.TrimSpace(str)
	return str, str != ""
}

// argOptionalString distinguishes "absent" from "present but empty", which is
// what makes a partial update possible: an explicitly empty description is a
// deliberate clear, an omitted one leaves the field alone (FR-2.3).
func argOptionalString(args map[string]any, key string) *string {
	v, present := args[key]
	if !present {
		return nil
	}
	str, isStr := v.(string)
	if !isStr {
		return nil
	}
	trimmed := strings.TrimSpace(str)
	return &trimmed
}

// mcpActor is the single seam for the chat agent's identity (SD-3), parallel to
// uiActor. Every MCP mutation is attributed to it on the audit trail (NFR-6).
func (s *Server) mcpActor() string {
	if cfg, err := s.freshConfig(); err == nil && cfg.Server.MCPActor != "" {
		return cfg.Server.MCPActor
	}
	return "chat-agent"
}
