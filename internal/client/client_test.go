package client

import (
	"os"
	"path/filepath"
	"testing"
)

// SPEC-013 §3.2: commands find a project under either folder name.
func TestFindRepoRootEitherFolder(t *testing.T) {
	for _, folder := range []string{".subutai", ".cromwell"} {
		root := t.TempDir()
		sub := filepath.Join(root, "docs", "deep")
		if err := os.MkdirAll(filepath.Join(root, folder), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := FindRepoRoot(sub)
		if err != nil {
			t.Fatalf("%s: %v", folder, err)
		}
		if want, _ := filepath.EvalSymlinks(root); got != root && got != want {
			t.Errorf("%s: FindRepoRoot = %q, want %q", folder, got, root)
		}
	}
	if _, err := FindRepoRoot(t.TempDir()); err == nil {
		t.Error("no folder should be an error")
	}
}
