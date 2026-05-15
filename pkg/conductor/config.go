package conductor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v2"
)

// Config is the parsed iterion.conductor.yaml. SourcePath is the
// resolved absolute path of the file on disk and is set by Load (not
// from YAML).
type Config struct {
	Name      string          `yaml:"name,omitempty"`
	Workflow  string          `yaml:"workflow"`
	Tracker   TrackerConfig   `yaml:"tracker"`
	Dispatch  DispatchConfig  `yaml:"dispatch,omitempty"`
	Polling   PollingConfig   `yaml:"polling,omitempty"`
	Agent     AgentConfig     `yaml:"agent,omitempty"`
	Workspace WorkspaceConfig `yaml:"workspace,omitempty"`
	Hooks     Hooks           `yaml:"hooks,omitempty"`
	Stall     StallConfig     `yaml:"stall,omitempty"`
	Server    ServerConfig    `yaml:"server,omitempty"`

	SourcePath string `yaml:"-"`
}

// TrackerConfig is the discriminated tracker definition. Kind selects
// which sibling block (Native, GitHub, Forgejo) is consulted.
type TrackerConfig struct {
	Kind    string                `yaml:"kind"`
	Native  *NativeTrackerConfig  `yaml:"native,omitempty"`
	GitHub  *GitHubTrackerConfig  `yaml:"github,omitempty"`
	Forgejo *ForgejoTrackerConfig `yaml:"forgejo,omitempty"`
}

// NativeTrackerConfig is intentionally empty in v1 — the native store
// is configured via board.json, not here.
type NativeTrackerConfig struct{}

// GitHubTrackerConfig configures the github_issues adapter.
type GitHubTrackerConfig struct {
	Repo          string                   `yaml:"repo"`
	Token         string                   `yaml:"token,omitempty"`
	StateMapping  map[string]LabelSelector `yaml:"state_mapping,omitempty"`
	ClaimedLabel  string                   `yaml:"claimed_label,omitempty"`
	IncludeLabels []string                 `yaml:"include_labels,omitempty"`
	ExcludeLabels []string                 `yaml:"exclude_labels,omitempty"`
}

// ForgejoTrackerConfig configures the forgejo (Gitea-compatible) adapter.
type ForgejoTrackerConfig struct {
	Host          string                   `yaml:"host"`
	Repo          string                   `yaml:"repo"`
	Token         string                   `yaml:"token,omitempty"`
	StateMapping  map[string]LabelSelector `yaml:"state_mapping,omitempty"`
	ClaimedLabel  string                   `yaml:"claimed_label,omitempty"`
	IncludeLabels []string                 `yaml:"include_labels,omitempty"`
	ExcludeLabels []string                 `yaml:"exclude_labels,omitempty"`
}

// LabelSelector restricts which issues map to a given workflow state.
type LabelSelector struct {
	LabelsInclude []string `yaml:"labels_include,omitempty"`
	LabelsExclude []string `yaml:"labels_exclude,omitempty"`
}

// DispatchConfig describes how issue fields flow into a workflow run.
// Vars maps workflow `vars:` names to template strings using
// {{issue.x}} / {{conductor.y}} references.
type DispatchConfig struct {
	Vars        map[string]string `yaml:"vars,omitempty"`
	Attachments map[string]string `yaml:"attachments,omitempty"`
}

// PollingConfig sets the tick cadence.
type PollingConfig struct {
	IntervalMS int `yaml:"interval_ms,omitempty"`
}

// AgentConfig sets concurrency and retry caps.
type AgentConfig struct {
	MaxConcurrent       int            `yaml:"max_concurrent,omitempty"`
	MaxConcurrentByState map[string]int `yaml:"max_concurrent_by_state,omitempty"`
	MaxTurns            int            `yaml:"max_turns,omitempty"`
	MaxRetryBackoffMS   int            `yaml:"max_retry_backoff_ms,omitempty"`
}

// WorkspaceConfig controls where per-issue workspaces live.
type WorkspaceConfig struct {
	Root    string `yaml:"root,omitempty"`
	Persist string `yaml:"persist,omitempty"` // keep | cleanup_on_done | cleanup_on_terminal
}

// StallConfig is the inactivity timeout for running issues.
type StallConfig struct {
	TimeoutMS int `yaml:"timeout_ms,omitempty"`
}

// ServerConfig opens the conductor's HTTP/WS surface.
type ServerConfig struct {
	Port int `yaml:"port,omitempty"`
}

// Defaults applied to optional fields after parse.
const (
	DefaultPollingInterval     = 30_000
	DefaultMaxConcurrent       = 4
	DefaultMaxTurns            = 20
	DefaultMaxRetryBackoffMS   = 300_000
	DefaultStallTimeoutMS      = 600_000
	DefaultGitHubClaimedLabel  = "iterion-claimed"
	DefaultForgejoClaimedLabel = "iterion-claimed"
)

// Load reads and parses a conductor config from path. Relative paths
// inside the config (workflow, workspace.root, hooks.path) are resolved
// against the config file's directory. Environment variable references
// like "$GITHUB_TOKEN" or "${GITHUB_TOKEN}" are expanded in string
// fields that accept secrets. The returned config has defaults applied.
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("config: resolve path: %w", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", abs, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", abs, err)
	}
	cfg.SourcePath = abs
	cfg.applyEnvAndPaths()
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Polling.IntervalMS == 0 {
		c.Polling.IntervalMS = DefaultPollingInterval
	}
	if c.Agent.MaxConcurrent == 0 {
		c.Agent.MaxConcurrent = DefaultMaxConcurrent
	}
	if c.Agent.MaxTurns == 0 {
		c.Agent.MaxTurns = DefaultMaxTurns
	}
	if c.Agent.MaxRetryBackoffMS == 0 {
		c.Agent.MaxRetryBackoffMS = DefaultMaxRetryBackoffMS
	}
	if c.Stall.TimeoutMS == 0 {
		c.Stall.TimeoutMS = DefaultStallTimeoutMS
	}
	if c.Tracker.Kind == "github" && c.Tracker.GitHub != nil && c.Tracker.GitHub.ClaimedLabel == "" {
		c.Tracker.GitHub.ClaimedLabel = DefaultGitHubClaimedLabel
	}
	if c.Tracker.Kind == "forgejo" && c.Tracker.Forgejo != nil && c.Tracker.Forgejo.ClaimedLabel == "" {
		c.Tracker.Forgejo.ClaimedLabel = DefaultForgejoClaimedLabel
	}
}

func (c *Config) applyEnvAndPaths() {
	dir := filepath.Dir(c.SourcePath)
	c.Workflow = resolveRelPath(dir, expandEnv(c.Workflow))
	if c.Workspace.Root != "" {
		c.Workspace.Root = expandHome(expandEnv(c.Workspace.Root))
		if !filepath.IsAbs(c.Workspace.Root) {
			c.Workspace.Root = filepath.Join(dir, c.Workspace.Root)
		}
	}
	c.Hooks.AfterCreate = expandHookPaths(dir, c.Hooks.AfterCreate)
	c.Hooks.BeforeRun = expandHookPaths(dir, c.Hooks.BeforeRun)
	c.Hooks.AfterRun = expandHookPaths(dir, c.Hooks.AfterRun)
	c.Hooks.BeforeRemove = expandHookPaths(dir, c.Hooks.BeforeRemove)

	if c.Tracker.GitHub != nil {
		c.Tracker.GitHub.Token = expandEnv(c.Tracker.GitHub.Token)
	}
	if c.Tracker.Forgejo != nil {
		c.Tracker.Forgejo.Token = expandEnv(c.Tracker.Forgejo.Token)
	}
}

// Validate checks fields are coherent. Workflow file existence is
// checked here; deeper compile-time validation of the .iter is the
// caller's responsibility (typically performed by Conductor.Start).
func (c *Config) Validate() error {
	if c.Workflow == "" {
		return errors.New("config: workflow is required")
	}
	if _, err := os.Stat(c.Workflow); err != nil {
		return fmt.Errorf("config: workflow %s: %w", c.Workflow, err)
	}
	switch c.Tracker.Kind {
	case "":
		return errors.New("config: tracker.kind is required (native | github | forgejo)")
	case "native":
		// nothing to validate beyond presence
	case "github":
		if c.Tracker.GitHub == nil {
			return errors.New("config: tracker.kind=github requires tracker.github block")
		}
		if c.Tracker.GitHub.Repo == "" {
			return errors.New("config: tracker.github.repo is required")
		}
		if !githubRepoRe.MatchString(c.Tracker.GitHub.Repo) {
			return fmt.Errorf("config: tracker.github.repo %q must be owner/repo", c.Tracker.GitHub.Repo)
		}
	case "forgejo":
		if c.Tracker.Forgejo == nil {
			return errors.New("config: tracker.kind=forgejo requires tracker.forgejo block")
		}
		if c.Tracker.Forgejo.Host == "" || c.Tracker.Forgejo.Repo == "" {
			return errors.New("config: tracker.forgejo requires host and repo")
		}
	default:
		return fmt.Errorf("config: tracker.kind %q not supported (native | github | forgejo)", c.Tracker.Kind)
	}
	if err := c.Hooks.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if c.Workspace.Persist != "" {
		switch c.Workspace.Persist {
		case "keep", "cleanup_on_done", "cleanup_on_terminal":
		default:
			return fmt.Errorf("config: workspace.persist %q invalid", c.Workspace.Persist)
		}
	}
	if c.Agent.MaxConcurrent < 1 {
		return errors.New("config: agent.max_concurrent must be >= 1")
	}
	for state, cap := range c.Agent.MaxConcurrentByState {
		if cap < 0 {
			return fmt.Errorf("config: agent.max_concurrent_by_state[%s] must be >= 0", state)
		}
	}
	for k, v := range c.Dispatch.Vars {
		if _, err := ParseTemplate(v); err != nil {
			return fmt.Errorf("config: dispatch.vars[%s]: %w", k, err)
		}
	}
	for k, v := range c.Dispatch.Attachments {
		if _, err := ParseTemplate(v); err != nil {
			return fmt.Errorf("config: dispatch.attachments[%s]: %w", k, err)
		}
	}
	return nil
}

// PollingInterval returns the polling cadence as a time.Duration after
// defaults are applied.
func (c *Config) PollingInterval() time.Duration {
	return time.Duration(c.Polling.IntervalMS) * time.Millisecond
}

// MaxRetryBackoff returns the retry backoff cap as a time.Duration.
func (c *Config) MaxRetryBackoff() time.Duration {
	return time.Duration(c.Agent.MaxRetryBackoffMS) * time.Millisecond
}

// StallTimeout returns the stall timeout as a time.Duration. Zero
// disables stall detection.
func (c *Config) StallTimeout() time.Duration {
	if c.Stall.TimeoutMS <= 0 {
		return 0
	}
	return time.Duration(c.Stall.TimeoutMS) * time.Millisecond
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

var githubRepoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func expandEnv(s string) string {
	if s == "" {
		return s
	}
	return os.Expand(s, func(name string) string {
		return os.Getenv(name)
	})
}

func expandHome(s string) string {
	if !strings.HasPrefix(s, "~") {
		return s
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return s
	}
	return filepath.Join(home, strings.TrimPrefix(s, "~"))
}

func resolveRelPath(dir, p string) string {
	if p == "" {
		return p
	}
	p = expandHome(p)
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

func expandHookPaths(dir string, h *Hook) *Hook {
	if h == nil || h.Path == "" {
		return h
	}
	out := *h
	out.Path = resolveRelPath(dir, h.Path)
	return &out
}
