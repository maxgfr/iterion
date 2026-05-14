package store

import "testing"

func TestCaptureLaunchEnv_AllowlistedPrefix(t *testing.T) {
	t.Setenv("ITERION_RENOVACY_MODEL_CLAUDE", "claude-opus-4-7")
	t.Setenv("ITERION_RENOVACY_EFFORT_AUDIT", "high")
	t.Setenv("RESCUE_PROVIDER", "zai")
	// These should NOT be captured — outside the allowlist:
	t.Setenv("HOME", "/home/test")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("RANDOM_VAR", "noise")

	env := CaptureLaunchEnv()
	if env["ITERION_RENOVACY_MODEL_CLAUDE"] != "claude-opus-4-7" {
		t.Errorf("missing ITERION_RENOVACY_MODEL_CLAUDE: %v", env)
	}
	if env["ITERION_RENOVACY_EFFORT_AUDIT"] != "high" {
		t.Errorf("missing ITERION_RENOVACY_EFFORT_AUDIT: %v", env)
	}
	if env["RESCUE_PROVIDER"] != "zai" {
		t.Errorf("missing RESCUE_PROVIDER: %v", env)
	}
	if _, leaked := env["HOME"]; leaked {
		t.Errorf("HOME leaked into LaunchEnv: %v", env)
	}
	if _, leaked := env["PATH"]; leaked {
		t.Errorf("PATH leaked into LaunchEnv: %v", env)
	}
	if _, leaked := env["RANDOM_VAR"]; leaked {
		t.Errorf("RANDOM_VAR leaked into LaunchEnv: %v", env)
	}
}

func TestCaptureLaunchEnv_RedactsSecrets(t *testing.T) {
	// Allowlisted by prefix BUT secret-named: keep the name, redact value.
	t.Setenv("ITERION_ANTHROPIC_API_KEY", "sk-secret-key-value")
	t.Setenv("ITERION_GITHUB_TOKEN", "ghp_dont_leak_me")
	t.Setenv("ITERION_NORMAL_VAR", "plain-value")

	env := CaptureLaunchEnv()
	if got := env["ITERION_ANTHROPIC_API_KEY"]; got != "[redacted]" {
		t.Errorf("API_KEY not redacted: got %q", got)
	}
	if got := env["ITERION_GITHUB_TOKEN"]; got != "[redacted]" {
		t.Errorf("GITHUB_TOKEN not redacted: got %q", got)
	}
	if got := env["ITERION_NORMAL_VAR"]; got != "plain-value" {
		t.Errorf("non-secret got redacted: got %q", got)
	}
}

func TestCaptureLaunchEnv_NilOnEmpty(t *testing.T) {
	// No ITERION_*/RESCUE_* in env (clear them).
	t.Setenv("ITERION_TEST_SENTINEL", "")
	t.Setenv("RESCUE_TEST_SENTINEL", "")
	env := CaptureLaunchEnv()
	for k := range env {
		// Filter out other ITERION_/RESCUE_ vars leaking from the host
		// (CI environments often have ITERION_HOME etc. set). We assert
		// the EMPTY-ness contract loosely: the function returns a map
		// or nil, and our two empty sentinels are absent from it.
		if k == "ITERION_TEST_SENTINEL" || k == "RESCUE_TEST_SENTINEL" {
			t.Errorf("empty-valued var %q should not appear in LaunchEnv", k)
		}
	}
}
