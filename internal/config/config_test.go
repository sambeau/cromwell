package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"subutai/internal/compat"
)

const validConfig = `version: 1
database:
  url_env: SUBUTAI_DATABASE_URL
budget:
  period: monthly
  cap_usd: 100.00
  warn_fraction: 0.8
  per_dispatch_cap_usd: 2.00
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
`

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// validCompartment builds a minimal, fully consistent .subutai/ directory.
func validCompartment(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "config.yaml", validConfig)
	write(t, root, "roles/spec-reviewer.yaml", `model: claude-sonnet-5
skill: review-spec
identity: |
  You are the specification reviewer.
tools: []
`)
	write(t, root, "skills/review-spec/SKILL.md", `---
description: Procedure for reviewing a specification
---

# Reviewing a specification

Read the validation report first.
`)
	write(t, root, "templates/spec/manifest.yaml", `type: spec
reviewer_role: spec-reviewer
front_matter:
  required: [title, type, owner]
sections:
  order: strict
  required:
    - heading: Overview
    - heading: Acceptance criteria
rules:
  - kind: min_list_items
    section: Acceptance criteria
    min: 1
`)
	write(t, root, "templates/spec/template.md", "---\ntitle: \"{{title}}\"\n---\n")
	write(t, root, "pack.lock.yaml", `pack_version: 0.1.0
files:
  - path: roles/spec-reviewer.yaml
    shipped_sha256: abc
`)
	return root
}

var testRuleKinds = map[string]bool{"min_list_items": true}

func TestLoadValidCompartment(t *testing.T) {
	root := validCompartment(t)
	c, err := Load(root, testRuleKinds)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Config.Dispatch.Workers != 4 || c.Config.Server.HeartbeatSeconds != 30 {
		t.Errorf("defaults not applied: %+v", c.Config.Dispatch)
	}
	if c.Roles["spec-reviewer"].Model != "claude-sonnet-5" {
		t.Errorf("role not loaded: %+v", c.Roles)
	}
	if c.Skills["review-spec"].Description == "" || !strings.Contains(c.Skills["review-spec"].Body, "validation report") {
		t.Errorf("skill not parsed: %+v", c.Skills["review-spec"])
	}
	if c.Manifests["spec"].ReviewerRole != "spec-reviewer" {
		t.Errorf("manifest not loaded: %+v", c.Manifests)
	}
	if c.PackLock.PackVersion != "0.1.0" {
		t.Errorf("pack lock not loaded: %+v", c.PackLock)
	}
}

// F-1: unknown keys are errors, and the error names the file.
func TestUnknownKeyRejected(t *testing.T) {
	root := validCompartment(t)
	write(t, root, "roles/spec-reviewer.yaml", "model: claude-sonnet-5\nidentity: x\nmodle_typo: oops\n")
	_, err := Load(root, testRuleKinds)
	if err == nil {
		t.Fatal("unknown key should be an error")
	}
	if !strings.Contains(err.Error(), "spec-reviewer.yaml") {
		t.Errorf("error should name the file: %v", err)
	}
}

// DESIGN-004 §9: dangling cross-references are config errors naming file and field.
func TestDanglingReferences(t *testing.T) {
	root := validCompartment(t)
	write(t, root, "roles/spec-reviewer.yaml", "model: no-such-model\nskill: no-such-skill\nidentity: x\n")
	write(t, root, "templates/spec/manifest.yaml", "type: spec\nreviewer_role: no-such-role\n")
	_, err := Load(root, testRuleKinds)
	if err == nil {
		t.Fatal("dangling refs should be errors")
	}
	for _, want := range []string{"no-such-model", "no-such-skill", "no-such-role"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

// SPEC-009 FR-2.1: each document type declares who may approve it. Absence
// means agent — the authority every type had before the field existed — and a
// value outside agent|human is a config error naming the field.
func TestManifestApprovalAuthority(t *testing.T) {
	root := validCompartment(t)

	m, err := LoadManifest(root, "spec")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.ApprovedBy != "agent" || m.HumanApproval() {
		t.Errorf("absent approved_by should default to agent: %+v", m.ApprovedBy)
	}

	write(t, root, "templates/spec/manifest.yaml", "type: spec\nreviewer_role: spec-reviewer\napproved_by: human\n")
	m, err = LoadManifest(root, "spec")
	if err != nil {
		t.Fatalf("load human: %v", err)
	}
	if !m.HumanApproval() {
		t.Errorf("approved_by: human should read as human approval: %+v", m.ApprovedBy)
	}

	write(t, root, "templates/spec/manifest.yaml", "type: spec\nreviewer_role: spec-reviewer\napproved_by: committee\n")
	if _, err := LoadManifest(root, "spec"); err == nil || !strings.Contains(err.Error(), "approved_by") {
		t.Errorf("unknown authority should be a config error naming the field: %v", err)
	}
}

func TestUnknownRuleKind(t *testing.T) {
	root := validCompartment(t)
	write(t, root, "templates/spec/manifest.yaml", `type: spec
reviewer_role: spec-reviewer
rules:
  - kind: no_such_rule
`)
	_, err := Load(root, testRuleKinds)
	if err == nil || !strings.Contains(err.Error(), "no_such_rule") {
		t.Errorf("unknown rule kind should be an error naming the kind: %v", err)
	}
}

func TestConfigFieldValidation(t *testing.T) {
	root := t.TempDir()
	write(t, root, "config.yaml", "version: 1\ndatabase:\n  url_env: X\nbudget:\n  period: fortnightly\n  cap_usd: 0\n  warn_fraction: 2\n  per_dispatch_cap_usd: 1\nproviders: {}\nmodels: {}\n")
	_, err := LoadConfig(root)
	if err == nil {
		t.Fatal("invalid config should fail")
	}
	for _, want := range []string{"budget.period", "budget.cap_usd", "budget.warn_fraction", "models"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name field %q: %v", want, err)
		}
	}
}

// SPEC-012 FR-2.1: the transcripts section is optional, every field has a
// default, retention 0 means keep for ever, and a negative value is an error
// naming its field.
func TestTranscriptConfigDefaults(t *testing.T) {
	root := validCompartment(t)
	c, err := Load(root, testRuleKinds)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	tc := c.Config.Transcripts
	if tc.MaxToolResultBytes != DefaultMaxToolResultBytes || tc.MaxEntryBytes != DefaultMaxEntryBytes ||
		tc.MaxAttemptBytes != DefaultMaxAttemptBytes || tc.Retention() != DefaultRetentionDays {
		t.Errorf("defaults not applied: %+v retention %d", tc, tc.Retention())
	}

	cfg, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "config.yaml", string(cfg)+"transcripts:\n  retention_days: 0\n  max_tool_result_bytes: 100\n")
	c2, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c2.Transcripts.Retention() != 0 || c2.Transcripts.MaxToolResultBytes != 100 {
		t.Errorf("explicit values not kept: %+v", c2.Transcripts)
	}

	write(t, root, "config.yaml", string(cfg)+"transcripts:\n  retention_days: -1\n  max_attempt_bytes: -5\n")
	_, err = LoadConfig(root)
	if err == nil {
		t.Fatal("negative values should fail")
	}
	for _, want := range []string{"transcripts.retention_days", "transcripts.max_attempt_bytes"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name field %q: %v", want, err)
		}
	}
}

// F-2: env-var indirection, and a clear message when the variable is unset.
func TestEnvIndirection(t *testing.T) {
	root := validCompartment(t)
	c, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUBUTAI_DATABASE_URL", "postgres://x")
	url, err := c.DatabaseURL()
	if err != nil || url != "postgres://x" {
		t.Errorf("DatabaseURL = %q, %v", url, err)
	}
	os.Unsetenv("SUBUTAI_DATABASE_URL")
	t.Setenv("CROMWELL_DATABASE_URL", "")
	if _, err := c.DatabaseURL(); err == nil || !strings.Contains(err.Error(), "SUBUTAI_DATABASE_URL") {
		t.Errorf("unset env should error naming the variable: %v", err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	key, err := c.APIKey("anthropic")
	if err != nil || key != "sk-test" {
		t.Errorf("APIKey = %q, %v", key, err)
	}
}

// SPEC-013 §3.3: url_env naming either twin reads the pair, the new name
// first, so an old project works with a new shell and a new one with an old
// shell.
func TestDatabaseURLTwins(t *testing.T) {
	for _, urlEnv := range []string{"SUBUTAI_DATABASE_URL", "CROMWELL_DATABASE_URL"} {
		c := &Config{}
		c.Database.URLEnv = urlEnv
		t.Setenv("SUBUTAI_DATABASE_URL", "")
		t.Setenv("CROMWELL_DATABASE_URL", "postgres://old")
		if got, err := c.DatabaseURL(); err != nil || got != "postgres://old" {
			t.Errorf("url_env %s, old variable set: %q, %v", urlEnv, got, err)
		}
		t.Setenv("SUBUTAI_DATABASE_URL", "postgres://new")
		if got, err := c.DatabaseURL(); err != nil || got != "postgres://new" {
			t.Errorf("url_env %s, both set: the new name should win, got %q, %v", urlEnv, got, err)
		}
		t.Setenv("CROMWELL_DATABASE_URL", "")
		if got, err := c.DatabaseURL(); err != nil || got != "postgres://new" {
			t.Errorf("url_env %s, new variable set: %q, %v", urlEnv, got, err)
		}
	}
	// Neither set: the error names the new variable, whichever url_env says.
	for _, urlEnv := range []string{"SUBUTAI_DATABASE_URL", "CROMWELL_DATABASE_URL"} {
		c := &Config{}
		c.Database.URLEnv = urlEnv
		t.Setenv("SUBUTAI_DATABASE_URL", "")
		t.Setenv("CROMWELL_DATABASE_URL", "")
		if _, err := c.DatabaseURL(); err == nil || !strings.Contains(err.Error(), "SUBUTAI_DATABASE_URL is not set") {
			t.Errorf("url_env %s, neither set: %v", urlEnv, err)
		}
	}
	// Any other name is read as written, with no twin.
	c := &Config{}
	c.Database.URLEnv = "MY_DB"
	t.Setenv("SUBUTAI_DATABASE_URL", "postgres://new")
	if _, err := c.DatabaseURL(); err == nil || !strings.Contains(err.Error(), "MY_DB") {
		t.Errorf("url_env MY_DB unset should error naming it: %v", err)
	}
}

// SPEC-013 §3.3: a url_env naming the old variable asks for config.yaml to
// be changed; one naming the new variable says nothing.
func TestDatabaseURLAsksForTheConfigChange(t *testing.T) {
	var buf bytes.Buffer
	old := compat.Stderr
	compat.Stderr = &buf
	compat.ResetWarnings()
	t.Cleanup(func() { compat.Stderr = old })
	t.Setenv("SUBUTAI_DATABASE_URL", "postgres://new")

	c := &Config{}
	c.Database.URLEnv = "SUBUTAI_DATABASE_URL"
	if _, err := c.DatabaseURL(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected warning: %q", buf.String())
	}
	c.Database.URLEnv = "CROMWELL_DATABASE_URL"
	if _, err := c.DatabaseURL(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "database.url_env names CROMWELL_DATABASE_URL") ||
		!strings.Contains(buf.String(), "change it to SUBUTAI_DATABASE_URL") {
		t.Errorf("warning %q should ask for config.yaml's url_env to change", buf.String())
	}
}

// SPEC-013 §3.2: the default socket follows the folder being read.
func TestDefaultSocketFollowsTheFolder(t *testing.T) {
	for folder, want := range map[string]string{
		".subutai":  ".subutai/run/subutai.sock",
		".cromwell": ".cromwell/run/cromwell.sock",
	} {
		root := filepath.Join(t.TempDir(), folder)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, root, "config.yaml", validConfig)
		c, err := LoadConfig(root)
		if err != nil {
			t.Fatal(err)
		}
		if c.Server.Socket != want {
			t.Errorf("%s: default socket %q, want %q", folder, c.Server.Socket, want)
		}
	}
}

func TestSplitFrontMatter(t *testing.T) {
	fm, body, err := SplitFrontMatter("---\na: 1\n---\nbody\n")
	if err != nil || fm != "a: 1" || strings.TrimSpace(body) != "body" {
		t.Errorf("got (%q, %q, %v)", fm, body, err)
	}
	if _, _, err := SplitFrontMatter("no front matter"); err == nil {
		t.Error("missing front matter should error")
	}
	if _, _, err := SplitFrontMatter("---\na: 1\n"); err == nil {
		t.Error("unterminated front matter should error")
	}
}

// FR-6.1: an unknown tool in a role profile is a config error.
func TestUnknownToolRejected(t *testing.T) {
	root := validCompartment(t)
	write(t, root, "roles/spec-reviewer.yaml", "model: claude-sonnet-5\nskill: review-spec\nidentity: x\ntools: [read_file, teleport]\n")
	_, err := Load(root, testRuleKinds)
	if err == nil || !strings.Contains(err.Error(), "teleport") {
		t.Errorf("unknown tool should error naming it: %v", err)
	}
}

// FR-1.4: a role bound to a read-only purpose may not declare a mutating tool.
func TestVerifierCannotMutate(t *testing.T) {
	root := validCompartment(t)
	// Add a verifier role with edit_file and bind it to verify-feature.
	write(t, root, "roles/verifier.yaml", "model: claude-sonnet-5\nidentity: verify\ntools: [read_file, edit_file]\n")
	base, _ := os.ReadFile(filepath.Join(root, "config.yaml"))
	write(t, root, "config.yaml", string(base)+"\nassignments:\n  verify-feature: verifier\n")
	_, err := Load(root, testRuleKinds)
	if err == nil || !strings.Contains(err.Error(), "edit_file") || !strings.Contains(err.Error(), "verifier.yaml") {
		t.Errorf("verifier with a mutating tool should error naming file and tool: %v", err)
	}
}

// The role schema gained a vocabulary payload and named anti-patterns
// (SPEC-009 FR-10). Both are optional so roles predating them keep loading,
// but a half-written anti-pattern is rejected: one without a name routes to
// generic advice, and one without a reason cannot generalise or be pruned.
func TestRoleVocabularyAndAntiPatterns(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "roles"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, "roles", name+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("full", `
model: m
identity: "Senior requirements engineer"
vocabulary: ["acceptance criterion", "boundary condition"]
anti_patterns:
  - name: "Untestable Requirement"
    detect: "subjective language with no measurable criterion"
    because: "a requirement nobody can check passes every review and fails every deployment"
    resolve: "replace the adjective with a threshold"
`)
	r, err := LoadRole(dir, "full")
	if err != nil {
		t.Fatalf("a fully-formed role must load: %v", err)
	}
	if len(r.Vocabulary) != 2 || len(r.AntiPatterns) != 1 || r.AntiPatterns[0].Because == "" {
		t.Errorf("fields did not round-trip: %+v", r)
	}

	// A role written before SPEC-009 has neither field and must still load.
	write("legacy", "model: m\nidentity: \"Senior code reviewer\"\n")
	if _, err := LoadRole(dir, "legacy"); err != nil {
		t.Errorf("both fields are optional: %v", err)
	}

	write("nameless", "model: m\nidentity: i\nanti_patterns:\n  - because: \"x\"\n")
	if _, err := LoadRole(dir, "nameless"); err == nil {
		t.Error("an anti-pattern with no name must be rejected")
	}
	write("reasonless", "model: m\nidentity: i\nanti_patterns:\n  - name: \"X\"\n")
	if _, err := LoadRole(dir, "reasonless"); err == nil {
		t.Error("an anti-pattern with no because must be rejected")
	}
}
