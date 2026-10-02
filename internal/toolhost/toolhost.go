// Package toolhost executes tool calls on behalf of a dispatched agent
// (DESIGN-006 §4). It is the server-side boundary that keeps a mutating
// agent safe: every path is jailed to the worktree, every edit is
// hash-anchored, every command is whitelisted, and every tool must be in the
// dispatch's declared profile. The host holds no cross-turn state beyond the
// worktree on disk; the pure decision logic (jailing, anchoring, lookup,
// profile) is testable without a provider (NFR-1).
package toolhost

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CommandSpec is a whitelisted command from config.yaml `commands:`
// (DESIGN-004 §4, extended in DESIGN-006 §4.6).
type CommandSpec struct {
	Argv           []string
	TimeoutSeconds int
	OutputCapBytes int
}

// Context is injected per dispatch (DESIGN-006 §4.2); the agent never sees or
// influences it.
type Context struct {
	WorktreeRoot string
	FeatureID    string
	TaskID       string
	Commands     map[string]CommandSpec
	Profile      map[string]bool // tool names this dispatch's role may call
	// DispatchID and Role name the run, set by the dispatch loop, so a tool
	// that records something — report_bug (SPEC-019 FR-3.3) — knows who did.
	DispatchID string
	Role       string
}

// InProfile reports whether the role may call the named tool. The outcome
// tool is always permitted (it is added by the dispatcher, not the profile).
func (c *Context) InProfile(tool string) bool {
	return c.Profile[tool]
}

// ---- Path jailing (DESIGN-006 §4.3) ----

// ResolvePath resolves a repo-relative path against the worktree root and
// verifies it stays inside, resolving symlinks so a link pointing out is
// caught. A path that escapes is an error (a tool error to the agent, never
// an escalation).
func (c *Context) ResolvePath(rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be relative to the worktree", rel)
	}
	root, err := filepath.Abs(c.WorktreeRoot)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(root, rel)

	// Resolve symlinks on the deepest existing ancestor, then re-append the
	// non-existent tail (a file about to be created has no link to resolve).
	resolved := resolveExisting(joined)
	rootResolved := resolveExisting(root)
	if resolved != rootResolved && !strings.HasPrefix(resolved, rootResolved+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the worktree", rel)
	}
	return joined, nil
}

// resolveExisting returns EvalSymlinks of the longest existing prefix of p,
// with the non-existing tail re-appended.
func resolveExisting(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(resolveExisting(parent), filepath.Base(p))
}

// ---- Hash-anchored read/edit (DESIGN-006 §4.5) ----

// lineHash is the short anchor for one line's bytes.
func lineHash(line string) string {
	sum := sha256.Sum256([]byte(line))
	return hex.EncodeToString(sum[:])[:4]
}

// TagLines renders content with `NN#hhhh| ` prefixes for hash-anchored
// editing. Line numbers are 1-based.
func TagLines(content string) string {
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%d#%s| %s\n", i+1, lineHash(line), line)
	}
	return b.String()
}

// AnchorError describes a drifted edit anchor so the agent knows to re-read.
type AnchorError struct {
	Ref    string
	Detail string
}

func (e *AnchorError) Error() string { return fmt.Sprintf("anchor %q: %s", e.Ref, e.Detail) }

// ApplyEdit replaces the anchored line in content with newText, after
// verifying the anchor still matches. hashRef is "NN#hhhh". newText may span
// multiple lines (it replaces exactly the one anchored line). Returns the new
// content. A drifted anchor is an *AnchorError.
func ApplyEdit(content, hashRef, newText string) (string, error) {
	numStr, wantHash, ok := strings.Cut(hashRef, "#")
	if !ok {
		return "", &AnchorError{Ref: hashRef, Detail: "must be of the form N#hash"}
	}
	n, err := strconv.Atoi(strings.TrimSpace(numStr))
	if err != nil || n < 1 {
		return "", &AnchorError{Ref: hashRef, Detail: "line number is not a positive integer"}
	}
	lines := strings.Split(content, "\n")
	if n > len(lines) {
		return "", &AnchorError{Ref: hashRef, Detail: fmt.Sprintf("file has %d lines; re-read", len(lines))}
	}
	if got := lineHash(lines[n-1]); got != wantHash {
		return "", &AnchorError{Ref: hashRef, Detail: "line changed since your read; re-read the file"}
	}
	lines[n-1] = newText
	return strings.Join(lines, "\n"), nil
}

// ---- Command whitelist (DESIGN-006 §4.6) ----

// ResolveCommand looks up a whitelisted command and appends any extra args.
// An unknown name is an error (a tool error to the agent).
func (c *Context) ResolveCommand(name string, extraArgs []string) (CommandSpec, []string, error) {
	spec, ok := c.Commands[name]
	if !ok {
		return CommandSpec{}, nil, fmt.Errorf("command %q is not in the project's allowed commands", name)
	}
	argv := append(append([]string{}, spec.Argv...), extraArgs...)
	return spec, argv, nil
}

// CapOutput truncates command output to the spec's cap, appending a marker.
func CapOutput(out []byte, cap int) string {
	if cap <= 0 || len(out) <= cap {
		return string(out)
	}
	return string(out[:cap]) + "\n…[output truncated at " + strconv.Itoa(cap) + " bytes]"
}
