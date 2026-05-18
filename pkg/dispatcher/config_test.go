package dispatcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	workflow := filepath.Join(dir, "wf.iter")
	if err := os.WriteFile(workflow, []byte("workflow x:\n  done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body = strings.ReplaceAll(body, "{{WORKFLOW}}", "wf.iter")
	cfgPath := filepath.Join(dir, "iterion.dispatcher.yaml")
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func TestConfigLoadNative(t *testing.T) {
	p := writeConfig(t, `name: smoke
workflow: {{WORKFLOW}}
tracker:
  kind: native
polling:
  interval_ms: 12345
agent:
  max_concurrent: 3
dispatch:
  vars:
    user_prompt: "Issue {{ issue.identifier }}: {{ issue.title }}"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Name != "smoke" {
		t.Fatalf("name: %q", cfg.Name)
	}
	if cfg.Tracker.Kind != "native" {
		t.Fatalf("kind: %q", cfg.Tracker.Kind)
	}
	if cfg.Polling.IntervalMS != 12345 {
		t.Fatalf("polling interval: %d", cfg.Polling.IntervalMS)
	}
	if cfg.Agent.MaxConcurrent != 3 {
		t.Fatalf("max concurrent: %d", cfg.Agent.MaxConcurrent)
	}
	if !filepath.IsAbs(cfg.Workflow) {
		t.Fatalf("workflow path should be absolute: %s", cfg.Workflow)
	}
}

func TestConfigDefaults(t *testing.T) {
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: native
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Polling.IntervalMS != DefaultPollingInterval {
		t.Fatalf("default polling: %d", cfg.Polling.IntervalMS)
	}
	if cfg.Agent.MaxConcurrent != DefaultMaxConcurrent {
		t.Fatalf("default max concurrent: %d", cfg.Agent.MaxConcurrent)
	}
	if cfg.Stall.TimeoutMS != DefaultStallTimeoutMS {
		t.Fatalf("default stall: %d", cfg.Stall.TimeoutMS)
	}
}

func TestConfigMissingWorkflow(t *testing.T) {
	p := writeConfig(t, `workflow: ./nope.iter
tracker:
  kind: native
`)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "workflow") {
		t.Fatalf("expected workflow-not-found error, got %v", err)
	}
}

func TestConfigUnknownTracker(t *testing.T) {
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: weirdo
`)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "weirdo") {
		t.Fatalf("expected unsupported tracker error, got %v", err)
	}
}

func TestConfigGitHubValidation(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_secret")
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: github
  github:
    repo: SocialGouv/iterion
    token: $GITHUB_TOKEN
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tracker.GitHub == nil || cfg.Tracker.GitHub.Token != "ghp_secret" {
		t.Fatalf("env not expanded: %+v", cfg.Tracker.GitHub)
	}
	if cfg.Tracker.GitHub.ClaimedLabel != DefaultGitHubClaimedLabel {
		t.Fatalf("default claimed_label not applied: %q", cfg.Tracker.GitHub.ClaimedLabel)
	}
}

func TestConfigGitHubBadRepo(t *testing.T) {
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: github
  github:
    repo: bare
`)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("expected owner/repo error, got %v", err)
	}
}

func TestConfigForgejoValidation(t *testing.T) {
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: forgejo
  forgejo:
    host: https://codeberg.org
    repo: forgejo/forgejo
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tracker.Forgejo.Host == "" {
		t.Fatal("host empty after load")
	}
}

func TestConfigTemplateValidation(t *testing.T) {
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: native
dispatch:
  vars:
    bad: "hello {{issue.notreal}}"
`)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "notreal") {
		t.Fatalf("expected template field error, got %v", err)
	}
}

func TestConfigHooksValidation(t *testing.T) {
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: native
hooks:
  after_create:
    script: "echo ok"
    path: "/bin/true"
`)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "script/path") {
		t.Fatalf("expected hook validation error, got %v", err)
	}
}

func TestConfigWorkspacePersistEnum(t *testing.T) {
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: native
workspace:
  persist: forever
`)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "persist") {
		t.Fatalf("expected persist enum error, got %v", err)
	}
}

func TestConfigHomeExpansion(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	p := writeConfig(t, `workflow: {{WORKFLOW}}
tracker:
  kind: native
workspace:
  root: "~/iterion-ws"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasPrefix(cfg.Workspace.Root, "/tmp/fakehome/") {
		t.Fatalf("home not expanded: %s", cfg.Workspace.Root)
	}
}
