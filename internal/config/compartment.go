package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func join(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// KnownTools is the set of tool names a role profile may declare (the source
// of truth is DESIGN-006 §4.4; the names are the contract). An unknown tool
// in a profile is a config error (FR-6.1). search_graph is named-but-
// deferred (SD-1): declaring it is an error until the integration lands.
func KnownTools() map[string]bool {
	return map[string]bool{
		"read_file": true, "list_files": true,
		"edit_file": true, "write_file": true, "run_command": true,
	}
}

// MutatingTools is the subset that writes to the worktree. Roles bound to
// read-only purposes (verification, review) must declare none (DESIGN-006
// §4.4, FR-1.4).
func MutatingTools() map[string]bool {
	return map[string]bool{"edit_file": true, "write_file": true}
}

// readOnlyPurposes are dispatch purposes whose role must not mutate the
// worktree.
var readOnlyPurposes = []string{"review-code", "verify-feature"}

// Role is roles/<name>.yaml (DESIGN-004 §5). The filename is the role name
// (F-9). The outcome tool is part of the dispatch purpose, defined in code —
// never listed in Tools (F-5).
type Role struct {
	Name string `yaml:"-"`
	// Identity is the role's job title, kept short. The reasoning that used
	// to live in a paragraph of prose belongs in an anti-pattern's Because
	// clause, where the research says it generalises to adjacent cases
	// (SPEC-009 FR-10.2).
	Model    string `yaml:"model"`
	Skill    string `yaml:"skill"`
	Identity string `yaml:"identity"`
	// Vocabulary is the routing signal: 15–30 precise domain terms that
	// determine which knowledge the model reaches for. This is the single
	// highest-ROI element of a prompt per the research, and Cromwell had
	// none of it before SPEC-009 (audit §3.1).
	Vocabulary []string `yaml:"vocabulary"`
	// AntiPatterns are named failure modes. Naming one activates expert
	// knowledge where an unnamed problem gets a generic answer.
	AntiPatterns []AntiPattern `yaml:"anti_patterns"`
	Tools        []string      `yaml:"tools"`
	ToolHints    string        `yaml:"tool_hints"`
	Limits       *Limits       `yaml:"limits"`
}

// AntiPattern is one named failure mode. Because is load-bearing: a rule with
// a reason generalises to cases the author never listed, and a rule without
// one cannot be evaluated or pruned later.
type AntiPattern struct {
	Name    string `yaml:"name"`
	Detect  string `yaml:"detect"`
	Because string `yaml:"because"`
	Resolve string `yaml:"resolve"`
}

type Limits struct {
	TurnCap int `yaml:"turn_cap"`
}

// LoadRole reads roles/<name>.yaml fresh (O-6).
func LoadRole(root, name string) (*Role, error) {
	rel := filepath.Join("roles", name+".yaml")
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, errf(rel, "", "cannot read: %v", err)
	}
	var r Role
	if err := strictUnmarshal(rel, data, &r); err != nil {
		return nil, err
	}
	r.Name = name
	var errs []error
	if r.Model == "" {
		errs = append(errs, errf(rel, "model", "required"))
	}
	if strings.TrimSpace(r.Identity) == "" {
		errs = append(errs, errf(rel, "identity", "required"))
	}
	// Both new fields are optional so the roles that predate SPEC-009 keep
	// loading unchanged (FR-10.3). What is not optional is an anti-pattern
	// that is half-written: one missing its name or its reason is worse than
	// absent, because it consumes attention budget and teaches nothing.
	for i, ap := range r.AntiPatterns {
		where := fmt.Sprintf("anti_patterns[%d]", i)
		if strings.TrimSpace(ap.Name) == "" {
			errs = append(errs, errf(rel, where+".name", "required — an unnamed anti-pattern routes to generic advice"))
		}
		if strings.TrimSpace(ap.Because) == "" {
			errs = append(errs, errf(rel, where+".because", "required — a rule without a reason cannot generalise or be pruned"))
		}
	}
	return &r, join(errs)
}

// Skill is skills/<name>/SKILL.md (DESIGN-004 §6): YAML front matter with a
// required description, body injected verbatim at prompt position 2.
type Skill struct {
	Name        string
	Description string
	Body        string
}

type skillFrontMatter struct {
	Description string `yaml:"description"`
}

// LoadSkill reads skills/<name>/SKILL.md fresh (O-6).
func LoadSkill(root, name string) (*Skill, error) {
	rel := filepath.Join("skills", name, "SKILL.md")
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, errf(rel, "", "cannot read: %v", err)
	}
	fm, body, err := SplitFrontMatter(string(data))
	if err != nil {
		return nil, errf(rel, "", "%v", err)
	}
	var meta skillFrontMatter
	if err := strictUnmarshal(rel, []byte(fm), &meta); err != nil {
		return nil, err
	}
	if meta.Description == "" {
		return nil, errf(rel, "description", "required")
	}
	return &Skill{Name: name, Description: meta.Description, Body: strings.TrimSpace(body)}, nil
}

// SplitFrontMatter separates a leading `---` YAML block from the Markdown
// body. The front matter is required (a document or skill without it fails
// validation, not parsing — but here at config level it is a load error).
func SplitFrontMatter(content string) (frontMatter, body string, err error) {
	const fence = "---"
	rest, ok := strings.CutPrefix(content, fence+"\n")
	if !ok {
		return "", "", fmt.Errorf("missing front matter: file must start with %q", fence)
	}
	fm, body, ok := strings.Cut(rest, "\n"+fence+"\n")
	if !ok {
		// Allow a fence that closes the file with no trailing newline.
		if trimmed, ok2 := strings.CutSuffix(rest, "\n"+fence); ok2 {
			return trimmed, "", nil
		}
		return "", "", fmt.Errorf("unterminated front matter: closing %q not found", fence)
	}
	return fm, body, nil
}

// Manifest is templates/<type>/manifest.yaml (DESIGN-004 §7): validation
// rules and the type's workflow binding.
type Manifest struct {
	Type         string          `yaml:"type"`
	ReviewerRole string          `yaml:"reviewer_role"`
	FrontMatter  FrontMatterReqs `yaml:"front_matter"`
	Sections     SectionReqs     `yaml:"sections"`
	Rules        []Rule          `yaml:"rules"`
}

type FrontMatterReqs struct {
	Required []string `yaml:"required"`
}

type SectionReqs struct {
	Order    string       `yaml:"order"` // strict | any
	Required []SectionRef `yaml:"required"`
	Optional []SectionRef `yaml:"optional"`
}

type SectionRef struct {
	Heading string `yaml:"heading"`
	Level   int    `yaml:"level"` // 0 = any level
}

// Rule references a registered rule kind (F-6); the registry lives in the
// validation engine, and Load verifies kinds against the set the caller
// passes in. Fields beyond kind/section are per-kind: min for
// min_list_items, columns for table_parses (documentary — the columns are
// fixed by the rule).
type Rule struct {
	Kind    string   `yaml:"kind"`
	Section string   `yaml:"section"`
	Min     int      `yaml:"min"`
	Columns []string `yaml:"columns"`
}

// LoadManifest reads templates/<docType>/manifest.yaml fresh (O-6).
func LoadManifest(root, docType string) (*Manifest, error) {
	rel := filepath.Join("templates", docType, "manifest.yaml")
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, errf(rel, "", "cannot read: %v", err)
	}
	var m Manifest
	if err := strictUnmarshal(rel, data, &m); err != nil {
		return nil, err
	}
	var errs []error
	if m.Type != docType {
		errs = append(errs, errf(rel, "type", "is %q but the directory says %q", m.Type, docType))
	}
	if m.ReviewerRole == "" {
		errs = append(errs, errf(rel, "reviewer_role", "required"))
	}
	switch m.Sections.Order {
	case "strict", "any":
	case "":
		m.Sections.Order = "strict"
	default:
		errs = append(errs, errf(rel, "sections.order", "unknown order %q: strict | any", m.Sections.Order))
	}
	return &m, join(errs)
}

// PackLock is pack.lock.yaml (DESIGN-004 §8): machine-managed provenance for
// upgrade's three-way classification. config.yaml is never tracked here (F-7).
type PackLock struct {
	PackVersion string     `yaml:"pack_version"`
	Files       []PackFile `yaml:"files"`
}

type PackFile struct {
	Path          string `yaml:"path"`
	ShippedSHA256 string `yaml:"shipped_sha256"`
}

const packLockFile = "pack.lock.yaml"

func LoadPackLock(root string) (*PackLock, error) {
	data, err := os.ReadFile(filepath.Join(root, packLockFile))
	if err != nil {
		return nil, errf(packLockFile, "", "cannot read: %v", err)
	}
	var pl PackLock
	if err := strictUnmarshal(packLockFile, data, &pl); err != nil {
		return nil, err
	}
	return &pl, nil
}

func (pl *PackLock) Marshal() ([]byte, error) {
	return yaml.Marshal(pl)
}

// Compartment is the fully loaded .cromwell/ directory.
type Compartment struct {
	Root      string
	Config    *Config
	Roles     map[string]*Role
	Skills    map[string]*Skill
	Manifests map[string]*Manifest // by document type
	PackLock  *PackLock
}

// Load reads and cross-validates the whole compartment — used at serve boot
// and by status for the config-health report (DESIGN-004 §9). knownRuleKinds
// is the validation engine's registry; an unknown rule kind is a config
// error. All errors are collected, not just the first.
func Load(root string, knownRuleKinds map[string]bool) (*Compartment, error) {
	var errs []error
	c := &Compartment{
		Root:      root,
		Roles:     map[string]*Role{},
		Skills:    map[string]*Skill{},
		Manifests: map[string]*Manifest{},
	}

	cfg, err := LoadConfig(root)
	if err != nil {
		errs = append(errs, err)
	}
	c.Config = cfg

	for _, name := range listDir(root, "roles", ".yaml") {
		r, err := LoadRole(root, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c.Roles[name] = r
	}
	for _, name := range listSubdirs(root, "skills") {
		s, err := LoadSkill(root, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c.Skills[name] = s
	}
	for _, docType := range listSubdirs(root, "templates") {
		m, err := LoadManifest(root, docType)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c.Manifests[docType] = m
	}
	if pl, err := LoadPackLock(root); err != nil {
		errs = append(errs, err)
	} else {
		c.PackLock = pl
	}

	// Cross-file references (DESIGN-004 §9).
	knownTools, mutating := KnownTools(), MutatingTools()
	if cfg != nil {
		for name, r := range c.Roles {
			rel := filepath.Join("roles", name+".yaml")
			if _, ok := cfg.Models[r.Model]; !ok {
				errs = append(errs, errf(rel, "model", "unknown model %q (not in config.yaml models)", r.Model))
			}
			if r.Skill != "" {
				if _, ok := c.Skills[r.Skill]; !ok {
					errs = append(errs, errf(rel, "skill", "unknown skill %q (no skills/%s/SKILL.md)", r.Skill, r.Skill))
				}
			}
			// Every declared tool must be a known tool (FR-6.1).
			for _, tool := range r.Tools {
				if !knownTools[tool] {
					errs = append(errs, errf(rel, "tools", "unknown tool %q", tool))
				}
			}
		}
		for purpose, role := range cfg.Assignments {
			r, ok := c.Roles[role]
			if !ok {
				errs = append(errs, errf(configFile, "assignments."+purpose, "unknown role %q", role))
				continue
			}
			// A role bound to a read-only purpose must not mutate the
			// worktree (FR-1.4): a verifier or code-reviewer with edit_file
			// is a config error naming the role file.
			if isReadOnlyPurpose(purpose) {
				for _, tool := range r.Tools {
					if mutating[tool] {
						errs = append(errs, errf(filepath.Join("roles", role+".yaml"),
							"tools", "role is bound to read-only purpose %q but declares mutating tool %q", purpose, tool))
					}
				}
			}
		}
	}
	for docType, m := range c.Manifests {
		if _, ok := c.Roles[m.ReviewerRole]; !ok {
			errs = append(errs, errf(filepath.Join("templates", docType, "manifest.yaml"),
				"reviewer_role", "unknown role %q", m.ReviewerRole))
		}
		for i, rule := range m.Rules {
			if knownRuleKinds != nil && !knownRuleKinds[rule.Kind] {
				errs = append(errs, errf(filepath.Join("templates", docType, "manifest.yaml"),
					fmt.Sprintf("rules[%d].kind", i), "unknown rule kind %q", rule.Kind))
			}
		}
	}

	return c, join(errs)
}

func listDir(root, sub, ext string) []string {
	entries, err := os.ReadDir(filepath.Join(root, sub))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			names = append(names, strings.TrimSuffix(e.Name(), ext))
		}
	}
	return names
}

func listSubdirs(root, sub string) []string {
	entries, err := os.ReadDir(filepath.Join(root, sub))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func isReadOnlyPurpose(purpose string) bool {
	for _, p := range readOnlyPurposes {
		if p == purpose {
			return true
		}
	}
	return false
}
