package conductor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// TestEngineRunnerAcceptsBundleDir verifies that an unpacked bundle
// directory (manifest.yaml + main.bot alongside) is a valid workflow
// source — the conductor can run it just like a plain `.iter`.
func TestEngineRunnerAcceptsBundleDir(t *testing.T) {
	// examples/secured-renovacy/ has the canonical bundle shape:
	// manifest.yaml + main.bot + prompts/. Walk up from the test
	// binary's working dir (pkg/conductor) to the repo root.
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	bundleDir := filepath.Join(repoRoot, "examples", "secured-renovacy")
	if _, err := os.Stat(filepath.Join(bundleDir, "manifest.yaml")); err != nil {
		t.Skipf("bundle fixture absent at %s: %v", bundleDir, err)
	}

	logger := iterlog.New(iterlog.LevelError, &bytes.Buffer{})
	r, err := NewEngineRunner(bundleDir, logger)
	if err != nil {
		t.Fatalf("NewEngineRunner on bundle dir: %v", err)
	}
	defer r.Close()

	if r.Workflow() == nil {
		t.Fatal("compiled workflow is nil")
	}
	if r.workflowHash == "" {
		t.Fatal("workflow hash is empty")
	}
	// The compiled bundle path should resolve to the bundle's
	// inner .iter, not the directory.
	if !filepath.IsAbs(r.workflowPath) {
		t.Fatalf("workflowPath should be absolute: %s", r.workflowPath)
	}
}

// TestEngineRunnerAcceptsPlainIter confirms the default (non-bundle)
// path still works after the bundle branch was added.
func TestEngineRunnerAcceptsPlainIter(t *testing.T) {
	dir := t.TempDir()
	iter := filepath.Join(dir, "hello.iter")
	src := "workflow hello:\n  entry: done\n"
	if err := os.WriteFile(iter, []byte(src), 0o644); err != nil {
		t.Fatalf("write iter: %v", err)
	}

	logger := iterlog.New(iterlog.LevelError, &bytes.Buffer{})
	r, err := NewEngineRunner(iter, logger)
	if err != nil {
		t.Fatalf("NewEngineRunner: %v", err)
	}
	defer r.Close()
	if r.bundle != nil {
		t.Fatal("plain .iter should not carry a bundle handle")
	}
}
