package lifecycle

import (
	"strings"
	"testing"

	"subutai/internal/config"
)

func decisionManifest() *config.Manifest {
	return &config.Manifest{Type: "decision", Rules: []config.Rule{
		{Kind: "max_words", Field: "ruling", Max: 75},
		{Kind: "max_words", Field: "reason", Max: 25},
		{Kind: "one_line", Field: "reason"},
	}}
}

// An adopted decision: identity lines only, a link that no longer resolves.
const adopted = "---\nid: DEC-003\nrevision: 1\n---\n\n# DEC-003: The CLI\n\n## Decision\n\nKeep the CLI small. See [gone](../gone.md).\n"

func TestDecisionRevisionMatrix(t *testing.T) {
	m := decisionManifest()
	never := func(string) bool { return false }
	cases := []struct {
		name, prev, next string
		ok               bool
		want             string
	}{
		{"amendment appended", adopted,
			strings.Replace(adopted, "revision: 1", "revision: 2", 1) + "\n## Amendment 1 — narrower (2026-09-30)\n\nOnly the server keeps a CLI.\n", true, ""},
		{"CRLF and BOM keep the text", adopted,
			"\uFEFF" + strings.ReplaceAll(strings.Replace(adopted, "revision: 1", "revision: 2", 1), "\n", "\r\n") + "\r\n## Amendment 1 — narrower (2026-09-30)\r\n\r\nText.\r\n", true, ""},
		{"accepted text changed", adopted,
			strings.Replace(adopted, "Keep the CLI small.", "Keep the CLI tiny.", 1) + "\n## Amendment 1 — x (2026-09-30)\n\nT.\n", false, "never edited"},
		{"nothing appended", adopted, strings.Replace(adopted, "revision: 1", "revision: 2", 1), false, "Nothing was appended"},
		{"undated heading", adopted, adopted + "\n## Amendment 1 — no date\n\nT.\n", false, "dated amendment heading"},
		{"placeholder left", adopted, adopted + "\n## Amendment 1 — {{what changed}} (2026-09-30)\n\nT.\n", false, "placeholder"},
		{"a new broken link is caught", adopted, adopted + "\n## Amendment 1 — x (2026-09-30)\n\nSee [x](../x.md).\n", false, "does not resolve"},
		{"record its ruling", adopted,
			strings.Replace(adopted, "revision: 1\n", "revision: 2\nruling: \"Keep the CLI small.\"\nreason: \"The web UI is the surface.\"\n", 1), true, ""},
		{"ruling without a reason", adopted,
			strings.Replace(adopted, "revision: 1\n", "revision: 2\nruling: \"Keep the CLI small.\"\n", 1), false, "needs a reason"},
		{"front matter can't change", adopted,
			strings.Replace(adopted, "revision: 1\n", "revision: 2\nowner: project\n", 1) + "\n## Amendment 1 — x (2026-09-30)\n\nT.\n", false, "can't change the decision's owner"},
		{"over-long ruling", adopted,
			strings.Replace(adopted, "revision: 1\n", "revision: 2\nruling: \""+strings.Repeat("word ", 80)+"\"\nreason: \"Why.\"\n", 1), false, "80 words"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := ValidateDecisionRevision(m, "DEC-003 revision 1", c.prev, c.next, never)
			if r.Valid != c.ok {
				t.Fatalf("valid = %v, want %v: %v", r.Valid, c.ok, r.Issues)
			}
			if c.want != "" && !strings.Contains(r.String(), c.want) {
				t.Errorf("issues %q don't say %q", r.String(), c.want)
			}
		})
	}
}

func TestSurfacedRuleKinds(t *testing.T) {
	m := decisionManifest()
	m.FrontMatter.Required = []string{"title", "ruling", "reason"}
	doc := func(ruling, reason string) string {
		return "---\ntitle: T\nruling: " + ruling + "\nreason: " + reason + "\n---\n\n# T\n"
	}
	if r := Validate(m, doc("Use Go.", "One binary."), nil); !r.Valid {
		t.Fatalf("a short ruling should pass: %v", r.Issues)
	}
	r := Validate(m, doc(strings.Repeat("x ", 200), "One binary."), nil)
	if r.Valid || !strings.Contains(r.String(), "The ruling is 200 words; it can be at most 75") {
		t.Errorf("a 200-word ruling must be refused: %v", r.Issues)
	}
	r = Validate(m, doc("Use Go.", "|\n  one\n  two"), nil)
	if r.Valid || !strings.Contains(r.String(), "one line") {
		t.Errorf("a literal-block reason must be refused: %v", r.Issues)
	}
	conv := &config.Manifest{Type: "conventions", Rules: []config.Rule{{Kind: "max_words", Body: true, Max: 5}}}
	r = Validate(conv, "---\ntitle: C\n---\n\n# Project conventions\n\n- one two three four five six\n", nil)
	if r.Valid || !strings.Contains(r.String(), "7 words below its title") {
		t.Errorf("conventions over the cap must be refused, the title not counted: %v", r.Issues)
	}
}
