package ident

import "testing"

func TestFormat(t *testing.T) {
	for _, c := range []struct {
		got, want string
	}{
		{Format("INIT", 14), "INIT-014"},
		{Format("FEAT", 1000), "FEAT-1000"},
		{TaskID("FEAT-023", 3), "FEAT-023-T03"},
		{TaskID("FEAT-023", 120), "FEAT-023-T120"},
		{DocumentID("FEAT-023", "dev_plan", 1), "FEAT-023-dev-plan"},
		{DocumentID("INIT-014", "design", 2), "INIT-014-design-2"},
		{DocumentID(ProjectOwner, "note", 1), "PROJECT-note"},
		{ArchiveName("FEAT-023-spec", 1), "FEAT-023-spec.r1.md"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	for _, c := range []struct {
		in    string
		shape Shape
		id    string
		rev   int
	}{
		{"INIT-014", ShapeEntity, "INIT-014", 0},
		{"feat-023", ShapeEntity, "FEAT-023", 0},
		{"MS-004", ShapeEntity, "MS-004", 0},
		{"CL-002", ShapeEntity, "CL-002", 0},
		{"FEAT-023-T03", ShapeTask, "FEAT-023-T03", 0},
		{"DEC-005", ShapeDecision, "DEC-005", 0},
		{"DEC-005.r2", ShapeDecision, "DEC-005", 2},
		{"FEAT-023-spec", ShapeDocument, "FEAT-023-spec", 0},
		{"FEAT-023-dev-plan", ShapeDocument, "FEAT-023-dev-plan", 0},
		{"FEAT-023-Spec.r1", ShapeDocument, "FEAT-023-spec", 1},
		{"INIT-014-design-2", ShapeDocument, "INIT-014-design-2", 0},
		{"PROJECT-design", ShapeDocument, "PROJECT-design", 0},
	} {
		r, ok := Parse(c.in)
		if !ok || r.Shape != c.shape || r.ID != c.id || r.Revision != c.rev {
			t.Errorf("Parse(%q) = %+v, %v; want shape %d id %q rev %d", c.in, r, ok, c.shape, c.id, c.rev)
		}
	}
	for _, bad := range []string{"", "auth/login", "FEAT-23", "XYZ-001", "FEAT-023-unknowntype", "FEAT-023-spec-1", "docs/a.md", "DEC-005-spec"} {
		if r, ok := Parse(bad); ok {
			t.Errorf("Parse(%q) should fail, got %+v", bad, r)
		}
	}
	r, _ := Parse("FEAT-023-dev-plan")
	if r.DocType != "dev_plan" || r.Entity != "FEAT-023" || r.Kind.Name != "feature" {
		t.Errorf("parts: %+v", r)
	}
}

func TestDecisionFromFileName(t *testing.T) {
	for in, want := range map[string]string{
		"DEC-005-the-orchestration-boundary.md": "DEC-005",
		"DEC-012.md":                            "DEC-012",
		"dec-001-server-language-go.md":         "DEC-001",
		"DEC-5-short.md":                        "",
		"notes.md":                              "",
		"DEC-0050x.md":                          "",
	} {
		if got := DecisionFromFileName(in); got != want {
			t.Errorf("%s: got %q want %q", in, got, want)
		}
	}
}
