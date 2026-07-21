package dispatch

import "cromwell/internal/provider"

// Tool definitions offered to dispatched agents (DESIGN-006 §4.4). The
// outcome tools complete a dispatch; the file/command tools have worktree
// side effects executed by the ToolExecutor. A role's profile names which of
// these it may call (DESIGN-004 §5); the dispatcher offers only the named
// ones plus the purpose's outcome tool.

func obj(props map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

// ImplementationOutcomeTool completes an implement-task dispatch.
func ImplementationOutcomeTool() provider.ToolDef {
	return provider.ToolDef{
		Name:        "submit_implementation",
		Description: "Call this once the task is fully implemented and the code builds. Summarise what you changed. This completes the task.",
		InputSchema: obj(map[string]any{
			"summary":       str("What you implemented and any noteworthy decisions"),
			"files_changed": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Paths you created or edited"},
		}, "summary"),
	}
}

// VerificationOutcomeTool completes a verify-feature dispatch.
func VerificationOutcomeTool() provider.ToolDef {
	return provider.ToolDef{
		Name:        "submit_verification",
		Description: "Report whether each acceptance criterion is met, then give an overall verdict. This completes verification.",
		InputSchema: obj(map[string]any{
			"criteria": map[string]any{
				"type": "array",
				"items": obj(map[string]any{
					"id":       str("The acceptance-criterion identifier or its text"),
					"met":      map[string]any{"type": "boolean"},
					"evidence": str("What you checked to decide"),
				}, "id", "met"),
			},
			"verdict":   map[string]any{"type": "string", "enum": []string{"approve", "request_changes", "escalate"}, "description": "approve: every criterion met. request_changes: one or more unmet. escalate: a human must judge."},
			"reasoning": str("Why you reached this verdict"),
		}, "criteria", "verdict", "reasoning"),
	}
}

// fileTools and commandTool are the worktree tool definitions.
func toolDef(name string) (provider.ToolDef, bool) {
	switch name {
	case "read_file":
		return provider.ToolDef{
			Name:        "read_file",
			Description: "Read a file in the worktree. Set hash_tag true to get line anchors (NN#hash|) for hash-anchored editing.",
			InputSchema: obj(map[string]any{
				"path":     str("Worktree-relative path"),
				"hash_tag": map[string]any{"type": "boolean", "description": "Return line-anchored content for editing"},
			}, "path"),
		}, true
	case "list_files":
		return provider.ToolDef{
			Name:        "list_files",
			Description: "List files in the worktree (or a subdirectory), respecting .gitignore.",
			InputSchema: obj(map[string]any{"dir": str("Worktree-relative directory, empty for the root")}),
		}, true
	case "edit_file":
		return provider.ToolDef{
			Name:        "edit_file",
			Description: "Replace one anchored line. Read with hash_tag first to get the anchor. Fails if the line drifted since your read — then re-read.",
			InputSchema: obj(map[string]any{
				"path":     str("Worktree-relative path"),
				"hash_ref": str("The NN#hash anchor of the line to replace"),
				"new_text": str("The replacement text for that line (may be multiple lines)"),
			}, "path", "hash_ref", "new_text"),
		}, true
	case "write_file":
		return provider.ToolDef{
			Name:        "write_file",
			Description: "Create or overwrite a whole file. Use for new files; prefer edit_file for changes to existing files.",
			InputSchema: obj(map[string]any{
				"path":    str("Worktree-relative path"),
				"content": str("The full file contents"),
			}, "path", "content"),
		}, true
	case "run_command":
		return provider.ToolDef{
			Name:        "run_command",
			Description: "Run one of the project's allowed commands (e.g. build, run_tests) in the worktree.",
			InputSchema: obj(map[string]any{
				"name": str("The allowed command name"),
				"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Extra arguments, if the command permits"},
			}, "name"),
		}, true
	}
	return provider.ToolDef{}, false
}

// ProfileToolDefs resolves a role's declared tool names to definitions,
// skipping unknown names (config load already rejected those).
func ProfileToolDefs(names []string) []provider.ToolDef {
	var defs []provider.ToolDef
	for _, n := range names {
		if d, ok := toolDef(n); ok {
			defs = append(defs, d)
		}
	}
	return defs
}
