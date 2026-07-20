// Package starter ships the embedded starter pack and implements
// `cromwell init` (FR-1.1/1.2, vision §12): copy the pack, generate
// config.yaml, write the pack lock with shipped hashes (DESIGN-004 §8),
// install the post-commit hook, and apply migrations.
package starter

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"cromwell/internal/config"
	"cromwell/internal/store"
)

//go:embed pack
var packFS embed.FS

// PackVersion identifies the shipped starter pack in pack.lock.yaml.
const PackVersion = "0.1.0"

const generatedConfig = `version: 1

database:
  url_env: CROMWELL_DATABASE_URL

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
`

const hookScript = `#!/bin/sh
# Installed by cromwell init: notify the server of document changes
# (FR-4.2). Silent no-op when the server is not running.
exec %q hook post-commit --repo %q >/dev/null 2>&1 || true
`

// Init runs once per project. It refuses to run twice (D-3) and applies all
// migrations transactionally.
func Init(ctx context.Context, repoRoot, executable string) error {
	compRoot := filepath.Join(repoRoot, ".cromwell")
	if _, err := os.Stat(compRoot); err == nil {
		return fmt.Errorf(".cromwell/ already exists at %s; init runs once per project (use upgrade when it exists in a later phase)", compRoot)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return fmt.Errorf("%s is not a git repository (cromwell manages documents in git)", repoRoot)
	}

	// The database must be reachable before we write anything, so a failed
	// init leaves no partial .cromwell/ behind (FR-1.2).
	dbURL := os.Getenv("CROMWELL_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("CROMWELL_DATABASE_URL is not set (secrets live in the environment, never in files — NFR-4)")
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
		".gitignore":  "run/\n",
	}
	lockBytes, err := lock.Marshal()
	if err != nil {
		cleanup()
		return err
	}
	writes["pack.lock.yaml"] = "# Machine-managed by cromwell init/upgrade; do not edit (DESIGN-004 §8).\n" + string(lockBytes)
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
