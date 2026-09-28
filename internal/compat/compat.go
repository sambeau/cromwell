// Package compat holds the rules that keep a project made with Cromwell,
// Subutai's old name, working for one release (SPEC-013 §3-4): the
// .cromwell/ folder, the CROMWELL_* environment variables, and worktrees
// recorded under .cromwell/. Everything here is marked compat(M7) and is
// removed in the first milestone after "Subutai usable" (SPEC-013 §4);
// what stays is Folder, HasFolder's first check, and Getenv's first read.
package compat

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// Folder is the project folder `init` creates.
	Folder = ".subutai"
	// LegacyFolder is Cromwell's project folder. compat(M7)
	LegacyFolder = ".cromwell"

	envPrefix       = "SUBUTAI_"
	legacyEnvPrefix = "CROMWELL_" // compat(M7)
)

// Stderr receives the deprecation warnings. Tests may replace it.
var Stderr io.Writer = os.Stderr

var (
	warnMu sync.Mutex
	warned = map[string]bool{}
)

// Warn prints one warning line, once per process however often it is asked.
func Warn(msg string) {
	warnMu.Lock()
	defer warnMu.Unlock()
	if warned[msg] {
		return
	}
	warned[msg] = true
	fmt.Fprintln(Stderr, "warning: "+msg)
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// HasFolder reports whether dir holds a project folder under either name.
func HasFolder(dir string) bool {
	return isDir(filepath.Join(dir, Folder)) || isDir(filepath.Join(dir, LegacyFolder))
}

// ProjectFolder returns the name of the folder the project at repoRoot uses:
// .subutai/ when it exists, otherwise .cromwell/ with a warning (compat(M7)).
// When both exist, .subutai/ wins, with a warning. When neither exists it
// returns Folder, and loading fails where it always did.
func ProjectFolder(repoRoot string) string {
	current := isDir(filepath.Join(repoRoot, Folder))
	legacy := isDir(filepath.Join(repoRoot, LegacyFolder))
	switch {
	case current && legacy:
		Warn("both .subutai/ and .cromwell/ exist; using .subutai/. " +
			"Delete .cromwell/ once you have moved anything you need out of it.")
		return Folder
	case legacy:
		Warn("this project's folder is .cromwell/, Subutai's old name. " +
			"Stop the server and run `git mv .cromwell .subutai`. " +
			".cromwell/ is read until the next release.")
		return LegacyFolder
	}
	return Folder
}

// DefaultSocket is the socket path, relative to the repository, used when
// config.yaml sets none. Under .cromwell/ it keeps Cromwell's name, so a
// Cromwell binary's post-commit hook still reaches the server until `serve`
// refreshes the hook (compat(M7)).
func DefaultSocket(folder string) string {
	if folder == LegacyFolder {
		return filepath.Join(LegacyFolder, "run", "cromwell.sock")
	}
	return filepath.Join(folder, "run", "subutai.sock")
}

// Twins returns the SUBUTAI_ and CROMWELL_ forms of a variable name, and
// false for a name with neither prefix.
func Twins(name string) (current, legacy string, ok bool) {
	switch {
	case strings.HasPrefix(name, envPrefix):
		return name, legacyEnvPrefix + strings.TrimPrefix(name, envPrefix), true
	case strings.HasPrefix(name, legacyEnvPrefix):
		return envPrefix + strings.TrimPrefix(name, legacyEnvPrefix), name, true
	}
	return name, "", false
}

// Getenv reads an environment variable. A name with either prefix is read
// as its SUBUTAI_ form first, then its CROMWELL_ form with a warning
// (compat(M7)); the new name wins when both are set. Any other name is read
// as written.
func Getenv(name string) string {
	current, legacy, ok := Twins(name)
	if !ok {
		return os.Getenv(name)
	}
	if v := os.Getenv(current); v != "" {
		return v
	}
	if v := os.Getenv(legacy); v != "" {
		Warn(fmt.Sprintf("%s is Subutai's old name for %s; rename it. "+
			"The old name is read until the next release.", legacy, current))
		return v
	}
	return ""
}

// WorktreePath resolves a worktree path recorded relative to repoRoot. A
// worktree recorded under .cromwell/ whose folder has since been renamed to
// folder is found under folder instead (compat(M7)). The second result says
// whether the path was moved.
func WorktreePath(repoRoot, folder, recorded string) (string, bool) {
	abs := filepath.Join(repoRoot, recorded)
	prefix := LegacyFolder + string(filepath.Separator)
	if folder == LegacyFolder || !strings.HasPrefix(filepath.Clean(recorded), prefix) {
		return abs, false
	}
	if _, err := os.Stat(abs); err == nil {
		return abs, false
	}
	return filepath.Join(repoRoot, folder, strings.TrimPrefix(filepath.Clean(recorded), prefix)), true
}
