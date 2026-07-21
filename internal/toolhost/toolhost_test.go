package toolhost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCtx(t *testing.T) *Context {
	t.Helper()
	root := t.TempDir()
	return &Context{
		WorktreeRoot: root,
		Profile:      map[string]bool{"read_file": true, "edit_file": true},
		Commands: map[string]CommandSpec{
			"run_tests": {Argv: []string{"go", "test", "./..."}, TimeoutSeconds: 60, OutputCapBytes: 100},
		},
	}
}

func TestResolvePathJail(t *testing.T) {
	c := testCtx(t)

	// In-worktree path resolves.
	if _, err := c.ResolvePath("src/main.go"); err != nil {
		t.Errorf("in-worktree path should resolve: %v", err)
	}
	// Traversal escapes.
	if _, err := c.ResolvePath("../../etc/passwd"); err == nil {
		t.Error("traversal should be rejected")
	}
	// Absolute path rejected.
	if _, err := c.ResolvePath("/etc/passwd"); err == nil {
		t.Error("absolute path should be rejected")
	}
}

func TestResolvePathSymlinkEscape(t *testing.T) {
	c := testCtx(t)
	// A symlink inside the worktree pointing outside must be caught.
	outside := t.TempDir()
	link := filepath.Join(c.WorktreeRoot, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := c.ResolvePath("escape/secret.txt"); err == nil {
		t.Error("path through an outward symlink should be rejected")
	}
}

func TestHashAnchoredEdit(t *testing.T) {
	content := "line one\nline two\nline three"
	tagged := TagLines(content)
	if !strings.Contains(tagged, "1#") || !strings.Contains(tagged, "| line one") {
		t.Fatalf("tagged output malformed:\n%s", tagged)
	}

	// Extract the anchor for line 2 from the tagged output.
	var ref string
	for _, l := range strings.Split(tagged, "\n") {
		if strings.Contains(l, "| line two") {
			ref = strings.SplitN(l, "|", 2)[0]
			ref = strings.TrimSpace(ref)
		}
	}
	if ref == "" {
		t.Fatal("could not find anchor for line two")
	}

	got, err := ApplyEdit(content, ref, "LINE TWO EDITED")
	if err != nil {
		t.Fatalf("edit with fresh anchor should succeed: %v", err)
	}
	if got != "line one\nLINE TWO EDITED\nline three" {
		t.Errorf("edit applied wrong: %q", got)
	}

	// A drifted anchor fails.
	if _, err := ApplyEdit("different\ncontent\nnow", ref, "x"); err == nil {
		t.Error("stale anchor should fail")
	}
	// Malformed ref fails.
	if _, err := ApplyEdit(content, "notanumber", "x"); err == nil {
		t.Error("malformed ref should fail")
	}
	// Out-of-range line fails.
	if _, err := ApplyEdit("one line", "9#abcd", "x"); err == nil {
		t.Error("out-of-range line should fail")
	}
}

func TestCommandWhitelist(t *testing.T) {
	c := testCtx(t)
	spec, argv, err := c.ResolveCommand("run_tests", nil)
	if err != nil || spec.TimeoutSeconds != 60 || strings.Join(argv, " ") != "go test ./..." {
		t.Errorf("run_tests resolve wrong: %v %v %v", spec, argv, err)
	}
	if _, _, err := c.ResolveCommand("rm", []string{"-rf", "/"}); err == nil {
		t.Error("non-whitelisted command must be rejected")
	}
	_, argv, _ = c.ResolveCommand("run_tests", []string{"-run", "TestX"})
	if strings.Join(argv, " ") != "go test ./... -run TestX" {
		t.Errorf("extra args should append: %v", argv)
	}
}

func TestCapOutput(t *testing.T) {
	if got := CapOutput([]byte("short"), 100); got != "short" {
		t.Errorf("under cap should pass through: %q", got)
	}
	got := CapOutput([]byte(strings.Repeat("x", 200)), 50)
	if len(got) < 50 || !strings.Contains(got, "truncated") {
		t.Errorf("over cap should truncate with marker: %q", got)
	}
}

func TestInProfile(t *testing.T) {
	c := testCtx(t)
	if !c.InProfile("read_file") || c.InProfile("write_file") {
		t.Error("profile enforcement wrong")
	}
}
