package dispatch

import "subutai/internal/provider"

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
					"evidence": str("What you read, ran or observed to decide — 'test TestLogin covers valid and invalid credentials', not 'looks fine'. Required for every criterion, met or unmet."),
				}, "id", "met", "evidence"),
			},
			"verdict":   map[string]any{"type": "string", "enum": []string{"approve", "request_changes", "escalate"}, "description": "approve: every criterion met. request_changes: one or more unmet. escalate: a human must judge."},
			"reasoning": str("Why you reached this verdict"),
		}, "criteria", "verdict", "reasoning"),
	}
}

// EstimateOutcomeTool completes an estimate dispatch (SPEC-003 FR-4). The
// agent returns a token count and its reasoning; the system assigns the
// confidence tier from the corpus evidence, so the tool takes no tier.
func EstimateOutcomeTool() provider.ToolDef {
	return provider.ToolDef{
		Name:        "submit_estimate",
		Description: "Submit your token estimate for this work. Give the number of tokens you expect the work to consume and your reasoning, citing any reference points you were shown. This completes the estimate — call it exactly once.",
		InputSchema: obj(map[string]any{
			"tokens":    map[string]any{"type": "integer", "description": "Estimated tokens the work will consume (a positive whole number)"},
			"rationale": str("Your reasoning, citing the reference points you used, if any"),
		}, "tokens", "rationale"),
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
	case "report_bug":
		return ReportBugTool(), true
	case "save_findings":
		return SaveFindingsTool(), true
	}
	return provider.ToolDef{}, false
}

// ReportBugTool files a bug report in the triage queue mid-work (SPEC-019
// FR-3.3). It is not an outcome tool: the agent carries on with its own task.
// Whether the bug is fixed is a person's decision, made in triage.
func ReportBugTool() provider.ToolDef {
	return provider.ToolDef{
		Name: "report_bug",
		Description: "Report a defect you have noticed that is outside your own task: something broken that this task " +
			"didn't ask you to change. It goes into the triage queue, where a person decides whether it is fixed. " +
			"Don't fix it yourself, and don't let it change what you do for your task: report it, then carry on. " +
			"Report only real defects, with steps someone else could follow; a run may file at most three.",
		InputSchema: obj(map[string]any{
			"title":    str("A short name for the defect, as a person would say it: \"Login accepts an empty password\""),
			"steps":    str("How to make it happen, as numbered steps someone else could follow"),
			"expected": str("What should happen"),
			"actual":   str("What happens instead"),
			"notes":    str("Optional: where in the code you saw it, a guess at the cause, a log line"),
		}, "title", "steps", "expected", "actual"),
	}
}

// SaveFindingsTool keeps a spike's draft findings (SPEC-021 FR-4.3). It is not
// an outcome tool: each call replaces the draft on the spike's row and the run
// carries on. The run can stop at any call, and what was saved last is kept.
func SaveFindingsTool() provider.ToolDef {
	return provider.ToolDef{
		Name: "save_findings",
		Description: "Save your findings so far. Each call replaces the last, so give the whole of what you know, not just what is new. " +
			"Do this early and often: the run can stop at any call, and whatever you have saved when it stops is what is kept. " +
			"Write the sections after the question: Answer, What we found, and, if they help, How we found out and What to do next. " +
			"This doesn't end the run, and it writes no file.",
		InputSchema: obj(map[string]any{
			"findings": str("The findings so far, in Markdown, as level-2 sections (## Answer, ## What we found, ...), without the Question section"),
		}, "findings"),
	}
}

// FinishSpikeTool completes a run-spike dispatch (SPEC-021 FR-4.4).
func FinishSpikeTool() provider.ToolDef {
	return provider.ToolDef{
		Name: "finish_spike",
		Description: "Finish the spike with your final findings, when you have an answer to the question, or when you can say why it can't be answered. " +
			"Give the whole of the findings, not just what changed since you last saved. They are checked against the findings template, " +
			"and if they don't fit you will be told why and can call this again. This ends the run, so call it once.",
		InputSchema: obj(map[string]any{
			"findings": str("The final findings, in Markdown, as level-2 sections: ## Answer, ## What we found, and, if they help, ## How we found out and ## What to do next. Leave out the Question section."),
		}, "findings"),
	}
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

// DocumentOutcomeTool completes an authoring dispatch (SPEC-009 FR-6). The
// agent returns the document body and nothing else: it does not choose a path,
// does not touch git, and cannot half-create a document. Path and owner come
// from the orchestrator, which is the standing convention for implicit context
// (vision §8).
func DocumentOutcomeTool() provider.ToolDef {
	return provider.ToolDef{
		Name: "submit_document",
		Description: "Submit the finished document. Give the whole file, front matter included, exactly as it should be written. " +
			"This completes your work — call it once. If the document fails validation you will be told why and can submit a corrected one.",
		InputSchema: obj(map[string]any{
			"body":      str("The complete document, starting with its YAML front matter"),
			"reasoning": str("How you arrived at it, and anything a reader should know — open questions you left, decisions you had to make"),
		}, "body", "reasoning"),
	}
}

// CommentsOutcomeTool completes a design review (SPEC-009 FR-2.2). It has no
// verdict field, deliberately: a design is approved by a human, and a reviewer
// with no way to express a verdict cannot be handed that authority by a later
// change. This is the same confinement-by-omission the MCP facet uses.
func CommentsOutcomeTool() provider.ToolDef {
	return provider.ToolDef{
		Name:        "submit_comments",
		Description: "Submit your comments on this design. There is no verdict: a human decides whether the design is approved. Call this once.",
		InputSchema: obj(map[string]any{
			"comments": map[string]any{
				"type": "array",
				"items": obj(map[string]any{
					"section_ref": str("Heading of the section the comment is about; leave empty for the whole document"),
					"body":        str("What you found, specifically"),
				}, "body"),
			},
			"reasoning": str("Your overall reading of the design — what you checked, and what stands"),
		}, "reasoning"),
	}
}
