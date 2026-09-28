package server

// The Send to development suite (SPEC-011), with the mock provider against
// real Postgres, reusing the phase-1 harness and the authoring helpers.

import (
	"os"
	"path/filepath"
)

// legacyDesignReviewer turns the test project into one made before SPEC-011:
// the design manifest names a design reviewer, and the role and its skill
// exist. Such a project keeps its reviewer (SD-8).
func (h *harness) legacyDesignReviewer() {
	h.t.Helper()
	comp := filepath.Join(h.root, ".cromwell")
	write := func(rel, body string) {
		p := filepath.Join(comp, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			h.t.Fatal(err)
		}
	}
	write("roles/design-reviewer.yaml", "model: claude-sonnet-5\nskill: review-design\nidentity: \"Senior engineer reading a design\"\ntools: [read_file, list_files]\nlimits:\n  turn_cap: 12\n")
	write("skills/review-design/SKILL.md", "---\ndescription: Comment on a design\n---\n\n# Reviewing a design\n\nComment; there is no verdict.\n")
	manifest := filepath.Join(comp, "templates/design/manifest.yaml")
	data, err := os.ReadFile(manifest)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append([]byte("reviewer_role: design-reviewer\n"), data...), 0o644); err != nil {
		h.t.Fatal(err)
	}
}
