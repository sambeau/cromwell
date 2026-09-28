package starter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoWithHook makes a directory with .git/hooks, and writes body as the
// post-commit hook when it isn't empty.
func repoWithHook(t *testing.T, body string) (root, hookPath string) {
	t.Helper()
	root = t.TempDir()
	hooks := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath = filepath.Join(hooks, "post-commit")
	if body != "" {
		if err := os.WriteFile(hookPath, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root, hookPath
}

// anExecutable returns the path of a file that exists, standing in for a
// binary.
func anExecutable(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// cromwellHook is the hook Cromwell's init wrote, byte for byte.
const cromwellHook = `#!/bin/sh
# Installed by cromwell init: notify the server of document changes
# (FR-4.2). Silent no-op when the server is not running.
exec %q hook post-commit --repo %q >/dev/null 2>&1 || true
`

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// SPEC-013 §3.4.
func TestRefreshHook(t *testing.T) {
	self := anExecutable(t, "subutai")

	t.Run("Cromwell's hook is rewritten", func(t *testing.T) {
		old := anExecutable(t, "cromwell")
		root, hook := repoWithHook(t, "")
		if err := os.WriteFile(hook, []byte(fmt.Sprintf(cromwellHook, old, root)), 0o755); err != nil {
			t.Fatal(err)
		}
		note, err := RefreshHook(root, self)
		if err != nil || !strings.Contains(note, "updated .git/hooks/post-commit to run "+self) {
			t.Fatalf("note %q, err %v", note, err)
		}
		if got, want := read(t, hook), fmt.Sprintf(hookScript, self, root); got != want {
			t.Errorf("hook is\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("a hook naming a binary that is gone is rewritten", func(t *testing.T) {
		root, hook := repoWithHook(t, "")
		gone := filepath.Join(t.TempDir(), "subutai-moved")
		if err := os.WriteFile(hook, []byte(fmt.Sprintf(hookScript, gone, root)), 0o755); err != nil {
			t.Fatal(err)
		}
		if note, _ := RefreshHook(root, self); !strings.Contains(note, "updated") {
			t.Errorf("note %q", note)
		}
		if !strings.Contains(read(t, hook), self) {
			t.Error("the hook should run the current binary")
		}
	})

	t.Run("a hook naming another repository is rewritten", func(t *testing.T) {
		root, hook := repoWithHook(t, "")
		if err := os.WriteFile(hook, []byte(fmt.Sprintf(hookScript, self, "/elsewhere")), 0o755); err != nil {
			t.Fatal(err)
		}
		if note, _ := RefreshHook(root, self); !strings.Contains(note, "updated") {
			t.Errorf("note %q", note)
		}
	})

	t.Run("a current hook is left alone, silently", func(t *testing.T) {
		root, hook := repoWithHook(t, "")
		body := fmt.Sprintf(hookScript, self, root)
		if err := os.WriteFile(hook, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		if note, err := RefreshHook(root, self); note != "" || err != nil {
			t.Errorf("note %q, err %v", note, err)
		}
		if read(t, hook) != body {
			t.Error("the hook changed")
		}
	})

	t.Run("another installed Subutai binary is left alone", func(t *testing.T) {
		other := anExecutable(t, "subutai")
		root, hook := repoWithHook(t, "")
		body := fmt.Sprintf(hookScript, other, root)
		if err := os.WriteFile(hook, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		if note, _ := RefreshHook(root, self); note != "" {
			t.Errorf("note %q", note)
		}
		if read(t, hook) != body {
			t.Error("the hook should not flip between two installed binaries")
		}
	})

	t.Run("an edited hook is left alone, with the line to change", func(t *testing.T) {
		old := anExecutable(t, "cromwell")
		root, hook := repoWithHook(t, "")
		body := fmt.Sprintf(cromwellHook, old, root) + "echo also mine\n"
		if err := os.WriteFile(hook, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		note, err := RefreshHook(root, self)
		if err != nil || !strings.Contains(note, "edited") || !strings.Contains(note, "change its exec line to run "+self) {
			t.Errorf("note %q, err %v", note, err)
		}
		if read(t, hook) != body {
			t.Error("an edited hook must not be rewritten")
		}
	})

	t.Run("an unrelated hook is left alone, silently", func(t *testing.T) {
		root, hook := repoWithHook(t, "#!/bin/sh\necho hello\n")
		if note, err := RefreshHook(root, self); note != "" || err != nil {
			t.Errorf("note %q, err %v", note, err)
		}
		if read(t, hook) != "#!/bin/sh\necho hello\n" {
			t.Error("the hook changed")
		}
	})

	t.Run("no hook stays no hook", func(t *testing.T) {
		root, hook := repoWithHook(t, "")
		if note, err := RefreshHook(root, self); note != "" || err != nil {
			t.Errorf("note %q, err %v", note, err)
		}
		if _, err := os.Stat(hook); !os.IsNotExist(err) {
			t.Error("a hook was created")
		}
	})

	t.Run("a go run binary never goes into the hook", func(t *testing.T) {
		old := anExecutable(t, "cromwell")
		root, hook := repoWithHook(t, "")
		body := fmt.Sprintf(cromwellHook, old, root)
		if err := os.WriteFile(hook, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		temp := filepath.Join(os.TempDir(), "go-build123", "b001", "exe", "subutai")
		note, _ := RefreshHook(root, temp)
		if !strings.Contains(note, "serve from a built binary") {
			t.Errorf("note %q", note)
		}
		if read(t, hook) != body {
			t.Error("the hook should not point at Go's build cache")
		}
	})

	t.Run("paths that need quoting round-trip", func(t *testing.T) {
		odd := anExecutable(t, `sub "utai"`)
		root, hook := repoWithHook(t, "")
		if err := os.WriteFile(hook, []byte(fmt.Sprintf(cromwellHook, odd, root)), 0o755); err != nil {
			t.Fatal(err)
		}
		if note, _ := RefreshHook(root, self); !strings.Contains(note, "updated") {
			t.Errorf("note %q", note)
		}
	})
}

// SPEC-013 §3.2: init refuses a project that already has either folder,
// before it needs a database.
func TestInitRefusesEitherFolder(t *testing.T) {
	for _, folder := range []string{".subutai", ".cromwell"} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, folder), 0o755); err != nil {
			t.Fatal(err)
		}
		err := Init(context.Background(), root, "/usr/bin/true")
		if err == nil || !strings.Contains(err.Error(), "already has a project folder") {
			t.Errorf("%s: err %v", folder, err)
		}
	}
}
