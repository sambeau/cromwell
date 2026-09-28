package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"subutai/internal/config"
	"subutai/internal/store"
)

// SPEC-013 §3.1: the actor comes from X-Subutai-Actor, and for one release
// from a Cromwell binary's X-Cromwell-Actor.
func TestActorHeader(t *testing.T) {
	cases := []struct {
		headers map[string]string
		want    string
	}{
		{map[string]string{"X-Subutai-Actor": "sam"}, "sam"},
		{map[string]string{"X-Cromwell-Actor": "sam"}, "sam"},
		{map[string]string{"X-Subutai-Actor": "new", "X-Cromwell-Actor": "old"}, "new"},
		{map[string]string{}, "unknown"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/api/status", nil)
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := actor(r); got != c.want {
			t.Errorf("headers %v: actor %q, want %q", c.headers, got, c.want)
		}
	}
}

// SPEC-013 §3.2: a worktree made under .cromwell/ is found, and repaired in
// git, after the folder is renamed to .subutai/; nothing recreates .cromwell/.
func TestWorktreeFoundAfterFolderRename(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.setupFeatureWithSpec()
	fid := mustFeatureID(t, h, "auth/login")

	// A worktree as Cromwell made it: recorded, and registered with git,
	// under .cromwell/.
	recorded := filepath.Join(".cromwell", "worktrees", "feat-old")
	branch := "cromwell/auth/login"
	if err := h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.CreateWorktreeRow(ctx, tx, fid, recorded, branch, "sam")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "worktree", "add", "-q", "-b", branch, filepath.Join(h.root, recorded), "HEAD")

	// The owner renames the folder. The server here already reads .subutai/,
	// so the worktree moves into it.
	moved := filepath.Join(h.root, ".subutai", "worktrees", "feat-old")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(h.root, recorded), moved); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(h.root, ".cromwell")); err != nil {
		t.Fatal(err)
	}

	if got := h.srv.worktreeAbs(recorded); got != moved {
		t.Errorf("worktreeAbs(%s) = %q, want %q", recorded, got, moved)
	}
	if err := h.srv.ReconcileWorktrees(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h.root, ".cromwell")); !os.IsNotExist(err) {
		t.Error("reconciling should not recreate .cromwell/")
	}
	out, err := gitIn(h.root, "worktree", "list", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "worktree "+moved) {
		t.Errorf("git should know the worktree's new path after the repair:\n%s", out)
	}
	// And git in the worktree works.
	if _, err := gitIn(moved, "status", "--short"); err != nil {
		t.Errorf("git status in the moved worktree: %v", err)
	}
	// Agents working on the feature are sent to the moved worktree: their
	// tools, their commands, and the review diff all start from this root.
	cfg, err := h.srv.freshConfig()
	if err != nil {
		t.Fatal(err)
	}
	tctx, err := h.srv.toolContextForFeature(ctx, cfg, fid.String(), "", &config.Role{})
	if err != nil {
		t.Fatal(err)
	}
	if tctx.WorktreeRoot != moved {
		t.Errorf("agents' worktree root = %q, want %q", tctx.WorktreeRoot, moved)
	}
	// Once repaired, it isn't repaired again on the next start.
	if !h.srv.gitKnowsWorktree(moved) {
		t.Error("git should list the repaired worktree")
	}
}
