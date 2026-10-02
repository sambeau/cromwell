// Package starter ships the embedded starter pack and implements
// `subutai init` (FR-1.1/1.2, vision §12): copy the pack, generate
// config.yaml, write the pack lock with shipped hashes (DESIGN-004 §8),
// install the post-commit hook, and apply migrations.
package starter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5"

	"subutai/internal/compat"
	"subutai/internal/config"
	"subutai/internal/store"
)

//go:embed pack
var packFS embed.FS

// PackVersion identifies the shipped starter pack in pack.lock.yaml.
const PackVersion = "0.1.0"

const generatedConfig = `version: 1

database:
  url_env: SUBUTAI_DATABASE_URL

budget:
  period: monthly
  cap_usd: 50.00
  warn_fraction: 0.8
  per_dispatch_cap_usd: 1.00

providers:
  anthropic:
    api_key_env: ANTHROPIC_API_KEY
    rate:
      requests_per_minute: 50

models:
  claude-sonnet-5:
    provider: anthropic
    price_per_mtok:
      input: 3.00
      output: 15.00
      cache_read: 0.30
      cache_write: 3.75
  claude-haiku-4-5:
    provider: anthropic
    price_per_mtok:
      input: 1.00
      output: 5.00
      cache_read: 0.10
      cache_write: 1.25

# Non-document dispatch purposes bind to roles here (DESIGN-004 §5 F-4).
assignments:
  implement-task: implementer
  review-code: code-reviewer
  verify-feature: verifier
  estimate: estimator
  # The authoring chain (SPEC-009, SPEC-011). Nothing here runs until a
  # person presses Send to development on a feature: then an agent writes
  # the specification, and once it is approved, the dev-plan. A spec or plan
  # you have already written is used as it is. Comment a line out to write
  # that document by hand instead.
  write-spec: spec-author
  write-dev-plan: dev-plan-author
  # A spike answers one question in a throwaway copy of the project and
  # keeps only its findings (SPEC-021). A person starts one in the web UI.
  run-spike: spike-runner

# How specifications are reviewed (SPEC-011, DEC-006 Amendment 1). The agent
# spec reviewer is the normal approver. Set hold to true to have every spec
# wait for a person after its review; the send screen can also hold a single
# send. Setting agent to false turns the agent review off, and then every
# spec waits for a person: no setting lets a spec through unreviewed.
spec_review:
  agent: true
  hold: false

# Spikes (SPEC-021). default_token_budget is the most a spike may use unless
# it is started with its own budget. It is a hard stop: every token counts,
# including the context re-sent on each turn, and the run ends when the total
# reaches it. 1,000,000 is roughly 30 turns of a modest spike.
spikes:
  default_token_budget: 1000000

# Tool-host command whitelist (DESIGN-006 §4.6). Edit these to your project's
# build and test commands; implementers and the verifier may run only these.
commands:
  build:
    argv: ["true"]
    timeout_seconds: 300
    output_cap_bytes: 65536
  run_tests:
    argv: ["true"]
    timeout_seconds: 600
    output_cap_bytes: 65536
`

const hookScript = `#!/bin/sh
# Installed by subutai init: notify the server of document changes
# (FR-4.2). Silent no-op when the server is not running.
exec %q hook post-commit --repo %q >/dev/null 2>&1 || true
`

// hookShape matches a hook that init wrote, from either product, and
// captures the product and the two %q-quoted arguments. Recognising
// Cromwell's version is compat(M7).
var hookShape = regexp.MustCompile(`\A#!/bin/sh\n` +
	`# Installed by (subutai|cromwell) init: notify the server of document changes\n` +
	`# \(FR-4\.2\)\. Silent no-op when the server is not running\.\n` +
	`exec ("(?:[^"\\]|\\.)*") hook post-commit --repo ("(?:[^"\\]|\\.)*") >/dev/null 2>&1 \|\| true\n\z`)

// RefreshHook keeps the post-commit hook init installed pointing at the
// running binary (SPEC-013 §3.4). It rewrites the hook only when init wrote
// it (its exact shape), and only when it is Cromwell's, names an executable
// that no longer exists, or names another repository. A hook a person has
// edited, or removed, is left alone. It never points the hook at a binary
// in Go's build cache, which is gone when `go run` exits.
//
// The note says what happened, for the caller to print; it is empty when
// there is nothing to say.
func RefreshHook(repoRoot, executable string) (note string, err error) {
	hookPath := filepath.Join(repoRoot, ".git", "hooks", "post-commit")
	data, err := os.ReadFile(hookPath)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		// No hook, or .git is a file (a linked worktree or submodule),
		// where init couldn't have installed one either.
		return "", nil
	}
	if err != nil {
		return "", err
	}
	m := hookShape.FindSubmatch(data)
	if m == nil {
		if bytes.Contains(data, []byte("hook post-commit")) {
			return fmt.Sprintf("warning: .git/hooks/post-commit has been edited, so it was left alone; "+
				"if it runs an old binary, change its exec line to run %s", executable), nil
		}
		return "", nil
	}
	product := string(m[1])
	oldExe, err1 := strconv.Unquote(string(m[2]))
	oldRepo, err2 := strconv.Unquote(string(m[3]))
	if err1 != nil || err2 != nil {
		return "", nil
	}
	_, statErr := os.Stat(oldExe)
	stale := product == "cromwell" || statErr != nil || oldRepo != repoRoot // "cromwell": compat(M7)
	if !stale {
		return "", nil
	}
	if strings.Contains(executable, string(filepath.Separator)+"go-build") {
		return fmt.Sprintf("warning: .git/hooks/post-commit runs %s; serve from a built binary to update it", oldExe), nil
	}
	if err := os.WriteFile(hookPath, []byte(fmt.Sprintf(hookScript, executable, repoRoot)), 0o755); err != nil {
		return "", err
	}
	return fmt.Sprintf("updated .git/hooks/post-commit to run %s", executable), nil
}

// Init runs once per project. It refuses to run twice (D-3) and applies all
// migrations transactionally.
func Init(ctx context.Context, repoRoot, executable string) error {
	compRoot := filepath.Join(repoRoot, compat.Folder)
	if compat.HasFolder(repoRoot) { // either name: compat(M7)
		return fmt.Errorf("%s already has a project folder (.subutai/ or .cromwell/); init runs once per project (use upgrade when it exists in a later phase)", repoRoot)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return fmt.Errorf("%s is not a git repository (subutai manages documents in git)", repoRoot)
	}

	// The database must be reachable before we write anything, so a failed
	// init leaves no partial .subutai/ behind (FR-1.2).
	dbURL := compat.Getenv("SUBUTAI_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("SUBUTAI_DATABASE_URL is not set (secrets live in the environment, never in files — NFR-4)")
	}
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("cannot connect to Postgres: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	cleanup := func() { _ = os.RemoveAll(compRoot) }

	// 1. Copy the starter pack, recording shipped hashes for the lock.
	var lock config.PackLock
	lock.PackVersion = PackVersion
	err = fs.WalkDir(packFS, "pack", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("pack", path)
		data, err := packFS.ReadFile(path)
		if err != nil {
			return err
		}
		dst := filepath.Join(compRoot, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lock.Files = append(lock.Files, config.PackFile{
			Path: filepath.ToSlash(rel), ShippedSHA256: hex.EncodeToString(sum[:]),
		})
		return nil
	})
	if err != nil {
		cleanup()
		return err
	}

	// 2. Generated files: config.yaml (user-owned from birth, never
	// pack-tracked — F-7), .gitignore for runtime files, the lock itself.
	writes := map[string]string{
		"config.yaml": generatedConfig,
		// run/ holds the socket; worktrees/ holds linked worktrees — neither
		// belongs in git, and worktrees must be ignored so server-authored
		// commits never sweep worktree files into the main tree (DESIGN-006 §3).
		".gitignore": "run/\nworktrees/\n",
	}
	lockBytes, err := lock.Marshal()
	if err != nil {
		cleanup()
		return err
	}
	writes["pack.lock.yaml"] = "# Machine-managed by subutai init/upgrade; do not edit (DESIGN-004 §8).\n" + string(lockBytes)
	for rel, body := range writes {
		if err := os.WriteFile(filepath.Join(compRoot, rel), []byte(body), 0o644); err != nil {
			cleanup()
			return err
		}
	}

	// 3. Post-commit hook.
	hookPath := filepath.Join(repoRoot, ".git/hooks/post-commit")
	if err := os.WriteFile(hookPath, []byte(fmt.Sprintf(hookScript, executable, repoRoot)), 0o755); err != nil {
		cleanup()
		return err
	}

	// 4. Migrations, each transactional (FR-1.2).
	if err := store.Migrate(ctx, conn); err != nil {
		cleanup()
		_ = os.Remove(hookPath)
		return err
	}

	return nil
}
