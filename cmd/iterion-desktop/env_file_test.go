//go:build desktop

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReloadIterionEnvFile_ClearsCommentedKey reproduces the dogfood
// case: the launching shell pre-loaded OPENAI_API_KEY (via direnv on a
// symlinked ~/.iterion/env → repo/.env), and the user later commented
// the line out in the file. Reload must clear the var even though
// applyDotenvFile didn't track it at boot ("shell wins" skipped it).
func TestReloadIterionEnvFile_ClearsCommentedKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env")

	// Simulate the boot state: env file has the key active.
	if err := os.WriteFile(envPath, []byte("OPENAI_API_KEY=initial-from-file\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("ITERION_ENV_FILE", envPath)

	// Reset package globals between tests.
	dotenvMu.Lock()
	dotenvAppliedKeys = nil
	dotenvMu.Unlock()

	// Simulate the shell pre-loading the key (e.g. direnv).
	t.Setenv("OPENAI_API_KEY", "shell-loaded-value")

	// Boot-time load: shell wins, the file value is skipped, key is NOT
	// tracked. This is the case my naive reload couldn't recover from.
	loadIterionEnvFile()
	if got := os.Getenv("OPENAI_API_KEY"); got != "shell-loaded-value" {
		t.Fatalf("after boot load: env = %q, want shell-loaded-value", got)
	}
	dotenvMu.Lock()
	tracked := append([]string(nil), dotenvAppliedKeys...)
	dotenvMu.Unlock()
	for _, k := range tracked {
		if k == "OPENAI_API_KEY" {
			t.Fatalf("OPENAI_API_KEY should NOT be tracked (shell-set), but got %v", tracked)
		}
	}

	// User comments the line out.
	if err := os.WriteFile(envPath, []byte("# OPENAI_API_KEY=initial-from-file\n"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	// Refresh must clear the key — the new reload semantics treat the
	// file as the source of truth.
	ReloadIterionEnvFile()
	if got, present := os.LookupEnv("OPENAI_API_KEY"); present {
		t.Fatalf("after reload: OPENAI_API_KEY should be unset, got %q", got)
	}
}

// TestReloadIterionEnvFile_PreservesShellOnlyKeys verifies that a key
// the shell set which does NOT appear anywhere in the file (active or
// commented) is left alone on reload — the shell still owns it.
func TestReloadIterionEnvFile_PreservesShellOnlyKeys(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env")
	if err := os.WriteFile(envPath, []byte("# nothing here\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("ITERION_ENV_FILE", envPath)
	dotenvMu.Lock()
	dotenvAppliedKeys = nil
	dotenvMu.Unlock()

	t.Setenv("MY_SHELL_VAR", "shell-only")
	loadIterionEnvFile()

	ReloadIterionEnvFile()
	if got := os.Getenv("MY_SHELL_VAR"); got != "shell-only" {
		t.Errorf("shell-only key was wrongly unset on reload: got %q", got)
	}
}

// TestReloadIterionEnvFile_RereadsActiveKey verifies that a value
// changed in the file is picked up by reload.
func TestReloadIterionEnvFile_RereadsActiveKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env")
	if err := os.WriteFile(envPath, []byte("MY_KEY=first\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("ITERION_ENV_FILE", envPath)
	os.Unsetenv("MY_KEY")
	dotenvMu.Lock()
	dotenvAppliedKeys = nil
	dotenvMu.Unlock()

	loadIterionEnvFile()
	if got := os.Getenv("MY_KEY"); got != "first" {
		t.Fatalf("initial: env = %q, want first", got)
	}

	if err := os.WriteFile(envPath, []byte("MY_KEY=second\n"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	ReloadIterionEnvFile()
	if got := os.Getenv("MY_KEY"); got != "second" {
		t.Errorf("after reload: env = %q, want second", got)
	}
}
