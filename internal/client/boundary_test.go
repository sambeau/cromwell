package client

// FR-2.2: the CLI client packages never touch Postgres. The single binary
// contains serve and init, so the boundary is enforced at the package
// level: this package's dependency closure must not contain store or pgx.

import (
	"os/exec"
	"strings"
	"testing"
)

func TestClientImportBoundary(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "subutai/internal/client").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if dep == "subutai/internal/store" || strings.Contains(dep, "jackc/pgx") {
			t.Errorf("client package must not depend on %s (FR-2.2, DESIGN-002 O-1)", dep)
		}
	}
}
