// Package config loads the .cromwell/ compartment (DESIGN-004): project
// config, roles, skills, template manifests, and the pack lock. Parsing is
// strict — unknown keys are errors (F-1) — and every error names the file,
// the field where possible, and the reason (DESIGN-004 §9). Secrets are
// env-var names here, values in the environment (F-2). This package does no
// I/O beyond reading the compartment and the named env vars.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Error is a config-compartment error: deterministic, never retried;
// dispatch-time occurrences raise `config-error` checkpoints (F-8).
type Error struct {
	File   string
	Field  string // may be empty when the error is file-level
	Reason string
}

func (e *Error) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %s", e.File, e.Reason)
	}
	return fmt.Sprintf("%s: %s: %s", e.File, e.Field, e.Reason)
}

func errf(file, field, format string, args ...any) *Error {
	return &Error{File: file, Field: field, Reason: fmt.Sprintf(format, args...)}
}

// strictUnmarshal decodes YAML rejecting unknown keys (F-1).
func strictUnmarshal(file string, data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return errf(file, "", "%v", err)
	}
	return nil
}

// Config is .cromwell/config.yaml (DESIGN-004 §4).
type Config struct {
	Version   int                 `yaml:"version"`
	Database  DatabaseConfig      `yaml:"database"`
	Server    ServerConfig        `yaml:"server"`
	Budget    BudgetConfig        `yaml:"budget"`
	Providers map[string]Provider `yaml:"providers"`
	Models    map[string]Model    `yaml:"models"`
	// Routing overrides the role's model per dispatch purpose (model only —
	// identity, skill, and tools stay with the role).
	Routing map[string]string `yaml:"routing"`
	// Assignments maps non-document dispatch purposes to roles (F-4); empty
	// in phase 1.
	Assignments map[string]string  `yaml:"assignments"`
	Dispatch    DispatchConfig     `yaml:"dispatch"`
	Commands    map[string]Command `yaml:"commands"`
	// SpecReview is how specs are reviewed (SPEC-011 FR-5.1, DEC-006
	// Amendment 1): the agent reviewer is the normal approver, and a project
	// may hold every spec for a person after it. Absent, both take their
	// defaults.
	SpecReview SpecReviewConfig `yaml:"spec_review"`

	// Transcripts bounds what each agent run's transcript stores, and for how
	// long (SPEC-012 FR-2). Optional: every field has a default.
	Transcripts TranscriptConfig `yaml:"transcripts"`
}

// SpecReviewConfig is the project's spec-review settings. Agent is a pointer
// because it defaults to on: an absent key must read as true.
type SpecReviewConfig struct {
	Agent *bool `yaml:"agent"`
	Hold  bool  `yaml:"hold"`
}

// AgentSpecReview reports whether the agent spec review runs (default on).
func (c *Config) AgentSpecReview() bool {
	return c.SpecReview.Agent == nil || *c.SpecReview.Agent
}

// HoldSpecs reports the project's hold. With agent review off it is forced
// on, whatever the file says: there is always at least one reviewer
// (Amendment 1, decision 6).
func (c *Config) HoldSpecs() bool {
	return c.SpecReview.Hold || !c.AgentSpecReview()
}

type DatabaseConfig struct {
	URLEnv string `yaml:"url_env"`
}

type ServerConfig struct {
	Socket           string `yaml:"socket"`
	HTTP             string `yaml:"http"`
	HeartbeatSeconds int    `yaml:"heartbeat_seconds"`
	// UIActor is the single configured operator identity the web command
	// centre acts as (DESIGN-007 CC-6, SD-4); multi-user auth is a later spec.
	UIActor string `yaml:"ui_actor"`
	// MCPActor is the single configured identity the chat agent authors as over
	// the MCP facet (SPEC-008 SD-3), parallel to UIActor. Every MCP mutation is
	// attributed to it on the audit trail, which is what makes a second
	// authoring surface safe (NFR-6). Per-user auth is a later spec; this is the
	// seam it will grow from.
	MCPActor string `yaml:"mcp_actor"`
}

type BudgetConfig struct {
	Period            string  `yaml:"period"` // monthly | weekly | total
	CapUSD            float64 `yaml:"cap_usd"`
	WarnFraction      float64 `yaml:"warn_fraction"`
	PerDispatchCapUSD float64 `yaml:"per_dispatch_cap_usd"`
}

type Provider struct {
	APIKeyEnv string `yaml:"api_key_env"`
	// BaseURL points the Anthropic wire protocol at a compatible endpoint
	// (e.g. DeepSeek's https://api.deepseek.com/anthropic). Empty = the
	// provider's default endpoint. Not a secret; lives in config.
	BaseURL string     `yaml:"base_url"`
	Rate    RateConfig `yaml:"rate"`
}

type RateConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
}

type Model struct {
	Provider     string       `yaml:"provider"`
	PricePerMTok PricePerMTok `yaml:"price_per_mtok"`
}

// PricePerMTok is USD per million tokens (F-3); snapshotted into
// dispatches.price_snapshot at claim time (O-4).
type PricePerMTok struct {
	Input      float64 `yaml:"input" json:"input"`
	Output     float64 `yaml:"output" json:"output"`
	CacheRead  float64 `yaml:"cache_read" json:"cache_read"`
	CacheWrite float64 `yaml:"cache_write" json:"cache_write"`
}

type DispatchConfig struct {
	Workers     int `yaml:"workers"`
	MaxAttempts int `yaml:"max_attempts"`
	TurnCap     int `yaml:"turn_cap"`
	// MaxReviewRounds bounds the implement→review→revise loop. Severity
	// gating ends the ordinary case; this is the backstop for genuine
	// disagreement, where major findings recur every round. At the cap a
	// human decides instead (audit §3.3a).
	MaxReviewRounds int `yaml:"max_review_rounds"`
	StallSeconds    int `yaml:"stall_seconds"`
}

// TranscriptConfig limits the size of stored transcripts and sets how long
// they are kept (SPEC-012 FR-2). An entry over its limit is cut in the middle
// with a marker, never dropped (SD-4). A pointer distinguishes "not set" from
// an explicit 0 for retention, where 0 means keep for ever; for the byte
// limits, 0 means the default.
type TranscriptConfig struct {
	MaxPromptBytes     int  `yaml:"max_prompt_bytes"`
	MaxToolResultBytes int  `yaml:"max_tool_result_bytes"`
	MaxEntryBytes      int  `yaml:"max_entry_bytes"`
	MaxAttemptBytes    int  `yaml:"max_attempt_bytes"`
	RetentionDays      *int `yaml:"retention_days"`
}

// Transcript defaults (SPEC-012 FR-2.1).
const (
	DefaultMaxPromptBytes     = 1 << 20
	DefaultMaxToolResultBytes = 32 << 10
	DefaultMaxEntryBytes      = 256 << 10
	DefaultMaxAttemptBytes    = 4 << 20
	DefaultRetentionDays      = 180
)

// Retention returns the configured retention in days; 0 keeps transcripts for
// ever.
func (t TranscriptConfig) Retention() int {
	if t.RetentionDays == nil {
		return DefaultRetentionDays
	}
	return *t.RetentionDays
}

// Command is a tool-host argv whitelist entry (DESIGN-004 §4, DESIGN-006
// §4.6): a fixed argv template, a timeout, and an output cap.
type Command struct {
	Argv           []string `yaml:"argv"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
	OutputCapBytes int      `yaml:"output_cap_bytes"`
}

const configFile = "config.yaml"

// LoadConfig reads and validates .cromwell/config.yaml. Defaults are applied
// for optional operational settings; structural fields are required.
func LoadConfig(root string) (*Config, error) {
	file := filepath.Join(root, configFile)
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, errf(configFile, "", "cannot read: %v", err)
	}
	var c Config
	if err := strictUnmarshal(configFile, data, &c); err != nil {
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	var errs []error
	add := func(field, format string, args ...any) {
		errs = append(errs, errf(configFile, field, format, args...))
	}

	if c.Version != 1 {
		add("version", "unsupported config format version %d (this build supports 1)", c.Version)
	}
	if c.Database.URLEnv == "" {
		add("database.url_env", "required: name of the env var holding the Postgres URL")
	}
	if c.Server.Socket == "" {
		c.Server.Socket = ".cromwell/run/cromwell.sock"
	}
	if c.Server.HeartbeatSeconds == 0 {
		c.Server.HeartbeatSeconds = 30
	}
	if c.Server.UIActor == "" {
		c.Server.UIActor = "operator"
	}
	if c.Server.MCPActor == "" {
		c.Server.MCPActor = "chat-agent"
	}
	switch c.Budget.Period {
	case "monthly", "weekly", "total":
	case "":
		add("budget.period", "required: monthly | weekly | total")
	default:
		add("budget.period", "unknown period %q: monthly | weekly | total", c.Budget.Period)
	}
	if c.Budget.CapUSD <= 0 {
		add("budget.cap_usd", "must be > 0")
	}
	if c.Budget.WarnFraction <= 0 || c.Budget.WarnFraction > 1 {
		add("budget.warn_fraction", "must be in (0, 1]")
	}
	if c.Budget.PerDispatchCapUSD <= 0 {
		add("budget.per_dispatch_cap_usd", "must be > 0")
	}
	if len(c.Models) == 0 {
		add("models", "at least one model is required")
	}
	for name, m := range c.Models {
		if _, ok := c.Providers[m.Provider]; !ok {
			add("models."+name+".provider", "unknown provider %q", m.Provider)
		}
	}
	for name, p := range c.Providers {
		if p.APIKeyEnv == "" {
			add("providers."+name+".api_key_env", "required")
		}
		if p.Rate.RequestsPerMinute <= 0 {
			add("providers."+name+".rate.requests_per_minute", "must be > 0")
		}
	}
	for purpose, model := range c.Routing {
		if _, ok := c.Models[model]; !ok {
			add("routing."+purpose, "unknown model %q", model)
		}
	}
	if c.Dispatch.Workers == 0 {
		c.Dispatch.Workers = 4
	}
	if c.Dispatch.MaxAttempts == 0 {
		c.Dispatch.MaxAttempts = 3
	}
	if c.Dispatch.TurnCap == 0 {
		c.Dispatch.TurnCap = 30
	}
	if c.Dispatch.MaxReviewRounds == 0 {
		c.Dispatch.MaxReviewRounds = 3
	}
	if c.Dispatch.StallSeconds == 0 {
		c.Dispatch.StallSeconds = 120
	}
	t := &c.Transcripts
	for _, f := range []struct {
		name string
		v    int
	}{
		{"max_prompt_bytes", t.MaxPromptBytes},
		{"max_tool_result_bytes", t.MaxToolResultBytes},
		{"max_entry_bytes", t.MaxEntryBytes},
		{"max_attempt_bytes", t.MaxAttemptBytes},
	} {
		if f.v < 0 {
			add("transcripts."+f.name, "must not be negative")
		}
	}
	if t.RetentionDays != nil && *t.RetentionDays < 0 {
		add("transcripts.retention_days", "must not be negative; 0 keeps transcripts for ever")
	}
	// 0, like leaving a byte limit out, means its default.
	if t.MaxPromptBytes == 0 {
		t.MaxPromptBytes = DefaultMaxPromptBytes
	}
	if t.MaxToolResultBytes == 0 {
		t.MaxToolResultBytes = DefaultMaxToolResultBytes
	}
	if t.MaxEntryBytes == 0 {
		t.MaxEntryBytes = DefaultMaxEntryBytes
	}
	if t.MaxAttemptBytes == 0 {
		t.MaxAttemptBytes = DefaultMaxAttemptBytes
	}
	return join(errs)
}

// DatabaseURL resolves the connection string from the environment (F-2).
func (c *Config) DatabaseURL() (string, error) {
	v := os.Getenv(c.Database.URLEnv)
	if v == "" {
		return "", errf(configFile, "database.url_env",
			"environment variable %s is not set", c.Database.URLEnv)
	}
	return v, nil
}

// APIKey resolves a provider's API key from the environment (F-2).
func (c *Config) APIKey(provider string) (string, error) {
	p, ok := c.Providers[provider]
	if !ok {
		return "", errf(configFile, "providers", "unknown provider %q", provider)
	}
	v := os.Getenv(p.APIKeyEnv)
	if v == "" {
		return "", errf(configFile, "providers."+provider+".api_key_env",
			"environment variable %s is not set", p.APIKeyEnv)
	}
	return v, nil
}
