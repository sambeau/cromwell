package starter

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"subutai/internal/config"
	"subutai/internal/lifecycle"
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

	t.Run(".git as a file is no hook", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if note, err := RefreshHook(root, self); note != "" || err != nil {
			t.Errorf("note %q, err %v", note, err)
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

// packRoot lays the shipped pack and the generated config out as init does,
// without needing a database.
func packRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	err := fs.WalkDir(packFS, "pack", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("pack", path)
		data, err := packFS.ReadFile(path)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(generatedConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	lock, err := (&config.PackLock{PackVersion: PackVersion}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pack.lock.yaml"), lock, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// SPEC-021 FR-10: a fresh init has the spike files and the assignment, and
// the whole starter compartment loads.
func TestStarterHasSpikePack(t *testing.T) {
	root := packRoot(t)
	for _, f := range []string{"templates/findings/manifest.yaml", "templates/findings/template.md",
		"roles/spike-runner.yaml", "skills/run-spike/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	c, err := config.Load(root, nil)
	if err != nil {
		t.Fatalf("starter compartment should load: %v", err)
	}
	if c.Config.Assignments["run-spike"] != "spike-runner" {
		t.Errorf("assignment: %v", c.Config.Assignments)
	}
	if c.Config.SpikeDefaultTokenBudget() != config.DefaultSpikeTokenBudget {
		t.Errorf("budget %d", c.Config.SpikeDefaultTokenBudget())
	}
	r := c.Roles["spike-runner"]
	if r.Skill != "run-spike" || r.Limits == nil || r.Limits.TurnCap != 40 || !slices.Contains(r.Tools, "save_findings") {
		t.Errorf("role: %+v", r)
	}
	m := c.Manifests["findings"]
	if m == nil || m.ApprovedBy != "human" || m.ReviewerRole != "" {
		t.Fatalf("findings manifest: %+v", m)
	}
}

// SPEC-021 FR-2.1: a findings document with only the required sections, and
// one with every section, validate against the shipped manifest; a missing
// Answer does not.
func TestFindingsTemplateValidates(t *testing.T) {
	c, err := config.Load(packRoot(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	m := c.Manifests["findings"]
	head := "---\ntitle: \"SPK-001: Can it work?\"\ntype: findings\nowner: SPK-001\n---\n\n# SPK-001: Can it work?\n\n## Question\n\nCan it work?\n\n"
	min := head + "## Answer\n\nYes.\n\n## What we found\n\nIt did.\n"
	full := min + "\n## How we found out\n\nWe ran it.\n\n## What to do next\n\nBuild it.\n\n## How this spike ended\n\nThe agent reached a conclusion.\n"
	for name, doc := range map[string]string{"minimum": min, "full": full} {
		if rep := lifecycle.Validate(m, doc, nil); !rep.Valid {
			t.Errorf("%s: %+v", name, rep)
		}
	}
	if rep := lifecycle.Validate(m, head+"## What we found\n\nIt did.\n", nil); rep.Valid {
		t.Error("a missing Answer should be invalid")
	}
}
