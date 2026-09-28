package compat

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureWarnings points Stderr at a buffer and forgets earlier warnings, so
// each test sees its own.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := Stderr
	Stderr = &buf
	ResetWarnings()
	t.Cleanup(func() { Stderr = old })
	return &buf
}

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProjectFolder(t *testing.T) {
	cases := []struct {
		name     string
		dirs     []string
		want     string
		has      bool
		warnings string // a phrase the warning must contain; empty for none
	}{
		{"new", []string{Folder}, Folder, true, ""},
		{"old", []string{LegacyFolder}, LegacyFolder, true, "git mv .cromwell .subutai"},
		{"both", []string{Folder, LegacyFolder}, Folder, true, "both .subutai/ and .cromwell/ exist; using .subutai/"},
		{"neither", nil, Folder, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := captureWarnings(t)
			root := t.TempDir()
			mkdirs(t, root, c.dirs...)
			if got := ProjectFolder(root); got != c.want {
				t.Errorf("ProjectFolder = %q, want %q", got, c.want)
			}
			if got := HasFolder(root); got != c.has {
				t.Errorf("HasFolder = %v, want %v", got, c.has)
			}
			out := buf.String()
			if c.warnings == "" && out != "" {
				t.Errorf("unexpected warning: %q", out)
			}
			if c.warnings != "" && !strings.Contains(out, c.warnings) {
				t.Errorf("warning %q should contain %q", out, c.warnings)
			}
			if strings.Count(out, "\n") > 1 {
				t.Errorf("the warning should be one line: %q", out)
			}
			// Asked again, it stays quiet.
			ProjectFolder(root)
			if buf.String() != out {
				t.Errorf("a warning should print once per process")
			}
		})
	}
}

func TestHasFolderIgnoresAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, LegacyFolder), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if HasFolder(root) {
		t.Error("a file named .cromwell is not a project folder")
	}
}

func TestDefaultSocket(t *testing.T) {
	if got := DefaultSocket(Folder); got != filepath.Join(".subutai", "run", "subutai.sock") {
		t.Errorf("DefaultSocket(.subutai) = %q", got)
	}
	// Under the old folder the old name is kept, so a Cromwell binary's
	// post-commit hook still reaches the server.
	if got := DefaultSocket(LegacyFolder); got != filepath.Join(".cromwell", "run", "cromwell.sock") {
		t.Errorf("DefaultSocket(.cromwell) = %q", got)
	}
}

func TestGetenv(t *testing.T) {
	const newName, oldName = "SUBUTAI_M7_TEST_URL", "CROMWELL_M7_TEST_URL"
	cases := []struct {
		name     string
		newV     string
		oldV     string
		ask      string
		want     string
		wantWarn bool
	}{
		{"new only", "n", "", newName, "n", false},
		{"old only", "", "o", newName, "o", true},
		{"both: new wins", "n", "o", newName, "n", false},
		{"neither", "", "", newName, "", false},
		{"asked by the old name, new set", "n", "", oldName, "n", false},
		{"asked by the old name, old set", "", "o", oldName, "o", true},
		{"asked by the old name, both set", "n", "o", oldName, "n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := captureWarnings(t)
			t.Setenv(newName, c.newV)
			t.Setenv(oldName, c.oldV)
			if got := Getenv(c.ask); got != c.want {
				t.Errorf("Getenv(%s) = %q, want %q", c.ask, got, c.want)
			}
			warned := buf.String()
			if c.wantWarn != (warned != "") {
				t.Errorf("warning = %q, want one: %v", warned, c.wantWarn)
			}
			if c.wantWarn && !strings.Contains(warned, oldName+" is Subutai's old name for "+newName) {
				t.Errorf("the warning should name both variables: %q", warned)
			}
		})
	}
}

func TestGetenvOtherNamesAreReadAsWritten(t *testing.T) {
	buf := captureWarnings(t)
	t.Setenv("ANTHROPIC_API_KEY", "k")
	if got := Getenv("ANTHROPIC_API_KEY"); got != "k" {
		t.Errorf("Getenv(ANTHROPIC_API_KEY) = %q", got)
	}
	if _, _, ok := Twins("ANTHROPIC_API_KEY"); ok {
		t.Error("a name with neither prefix has no twin")
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected warning: %q", buf.String())
	}
}

func TestWorktreePath(t *testing.T) {
	root := t.TempDir()
	recorded := filepath.Join(".cromwell", "worktrees", "feat-1")

	// Still under .cromwell/: used as recorded.
	mkdirs(t, root, recorded)
	if got, moved := WorktreePath(root, Folder, recorded); moved || got != filepath.Join(root, recorded) {
		t.Errorf("present at its recorded path: got %q moved=%v", got, moved)
	}
	// The folder is renamed: found under .subutai/.
	if err := os.Rename(filepath.Join(root, ".cromwell"), filepath.Join(root, ".subutai")); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".subutai", "worktrees", "feat-1")
	if got, moved := WorktreePath(root, Folder, recorded); !moved || got != want {
		t.Errorf("after the rename: got %q moved=%v, want %q", got, moved, want)
	}
	// A project still using .cromwell/ never maps.
	if got, moved := WorktreePath(root, LegacyFolder, recorded); moved || got != filepath.Join(root, recorded) {
		t.Errorf("under the old folder: got %q moved=%v", got, moved)
	}
	// A path recorded under .subutai/ is used as written.
	rec := filepath.Join(".subutai", "worktrees", "feat-2")
	if got, moved := WorktreePath(root, Folder, rec); moved || got != filepath.Join(root, rec) {
		t.Errorf("new path: got %q moved=%v", got, moved)
	}
}
