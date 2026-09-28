package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"subutai/internal/dispatch"
	"subutai/internal/toolhost"
)

// Execute runs one non-outcome tool call in the worktree (DESIGN-006 §4).
// Every tool error is returned to the agent (isErr=true), never raised — the
// agent decides what to do next and still ends via its outcome tool.
func (s *Server) Execute(ctx context.Context, tctx *toolhost.Context, name string, input json.RawMessage) (string, bool) {
	switch name {
	case "read_file":
		return s.toolReadFile(tctx, input)
	case "list_files":
		return s.toolListFiles(tctx, input)
	case "edit_file":
		return s.toolEditFile(tctx, input)
	case "write_file":
		return s.toolWriteFile(tctx, input)
	case "run_command":
		return s.toolRunCommand(ctx, tctx, input)
	case "report_bug":
		return s.toolReportBug(ctx, tctx, input)
	default:
		return fmt.Sprintf("unknown tool %q", name), true
	}
}

var _ dispatch.ToolExecutor = (*Server)(nil)

func (s *Server) toolReadFile(tctx *toolhost.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Path    string `json:"path"`
		HashTag bool   `json:"hash_tag"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "invalid arguments: " + err.Error(), true
	}
	abs, err := tctx.ResolvePath(args.Path)
	if err != nil {
		return err.Error(), true
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "cannot read: " + err.Error(), true
	}
	if args.HashTag {
		return toolhost.TagLines(string(data)), false
	}
	return string(data), false
}

func (s *Server) toolListFiles(tctx *toolhost.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Dir string `json:"dir"`
	}
	_ = json.Unmarshal(input, &args)
	base, err := tctx.ResolvePath(args.Dir)
	if err != nil {
		return err.Error(), true
	}
	// git ls-files (tracked) + untracked, respecting .gitignore, scoped to base.
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = base
	out, err := cmd.Output()
	if err != nil {
		// Fall back to a plain directory listing outside a git subtree.
		entries, derr := os.ReadDir(base)
		if derr != nil {
			return "cannot list: " + derr.Error(), true
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		return strings.Join(names, "\n"), false
	}
	return string(out), false
}

func (s *Server) toolEditFile(tctx *toolhost.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Path    string `json:"path"`
		HashRef string `json:"hash_ref"`
		NewText string `json:"new_text"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "invalid arguments: " + err.Error(), true
	}
	abs, err := tctx.ResolvePath(args.Path)
	if err != nil {
		return err.Error(), true
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "cannot read for edit: " + err.Error(), true
	}
	updated, err := toolhost.ApplyEdit(string(data), args.HashRef, args.NewText)
	if err != nil {
		return err.Error(), true // AnchorError: tells the agent to re-read
	}
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return "cannot write: " + err.Error(), true
	}
	return fmt.Sprintf("edited %s", args.Path), false
}

func (s *Server) toolWriteFile(tctx *toolhost.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "invalid arguments: " + err.Error(), true
	}
	abs, err := tctx.ResolvePath(args.Path)
	if err != nil {
		return err.Error(), true
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "cannot create parent: " + err.Error(), true
	}
	if err := os.WriteFile(abs, []byte(args.Content), 0o644); err != nil {
		return "cannot write: " + err.Error(), true
	}
	return fmt.Sprintf("wrote %s", args.Path), false
}

func (s *Server) toolRunCommand(ctx context.Context, tctx *toolhost.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Name string   `json:"name"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "invalid arguments: " + err.Error(), true
	}
	spec, argv, err := tctx.ResolveCommand(args.Name, args.Args)
	if err != nil {
		return err.Error(), true
	}
	timeout := time.Duration(spec.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = tctx.WorktreeRoot
	out, runErr := cmd.CombinedOutput()
	body := toolhost.CapOutput(out, spec.OutputCapBytes)
	if runCtx.Err() == context.DeadlineExceeded {
		return body + "\n[command timed out after " + timeout.String() + "]", true
	}
	if runErr != nil {
		// A non-zero exit is information for the agent, not a dispatch failure.
		return body + "\n[exit: " + runErr.Error() + "]", true
	}
	return body, false
}
