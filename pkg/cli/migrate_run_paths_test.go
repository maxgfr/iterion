package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRunJSON(t *testing.T, storeDir, runID, content string) string {
	t.Helper()
	dir := filepath.Join(storeDir, "runs", runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "run.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMigrateRunPaths_RewritesMovedBots(t *testing.T) {
	storeDir := t.TempDir()
	moved := writeRunJSON(t, storeDir, "run-moved", `{
  "run_id": "run-moved",
  "status": "finished",
  "file_path": "/home/u/proj/examples/whats-next/main.bot",
  "bundle_path": "/home/u/proj/examples/whats-next",
  "note": "keep /examples/whats-next inside a non-path field untouched"
}`)
	stayed := writeRunJSON(t, storeDir, "run-demo",
		`{"run_id":"run-demo","file_path":"/home/u/proj/examples/cursors/sample.iter"}`)

	res, err := MigrateRunPaths(MigrateRunPathsOptions{StoreDir: storeDir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 2 || res.Updated != 1 {
		t.Fatalf("scanned=%d updated=%d, want 2/1", res.Scanned, res.Updated)
	}

	gs := readFile(t, moved)
	if !strings.Contains(gs, `"file_path": "/home/u/proj/bots/whats-next/main.bot"`) {
		t.Errorf("file_path not rewritten:\n%s", gs)
	}
	if !strings.Contains(gs, `"bundle_path": "/home/u/proj/bots/whats-next"`) {
		t.Errorf("bundle_path (bare bundle dir) not rewritten:\n%s", gs)
	}
	// Field-scoped: the same substring inside a non-path field is preserved.
	if !strings.Contains(gs, `keep /examples/whats-next inside`) {
		t.Errorf("non-path field must be preserved:\n%s", gs)
	}
	if !strings.Contains(gs, `"status": "finished"`) {
		t.Errorf("unrelated field lost:\n%s", gs)
	}
	// Demo bot that stayed under examples/ is untouched.
	if gd := readFile(t, stayed); !strings.Contains(gd, "examples/cursors/sample.iter") {
		t.Errorf("demo path should be untouched: %s", gd)
	}
}

func TestMigrateRunPaths_Idempotent(t *testing.T) {
	storeDir := t.TempDir()
	writeRunJSON(t, storeDir, "r1", `{"file_path":"/x/examples/feature_dev/main.bot"}`)
	if _, err := MigrateRunPaths(MigrateRunPathsOptions{StoreDir: storeDir}); err != nil {
		t.Fatal(err)
	}
	res, err := MigrateRunPaths(MigrateRunPathsOptions{StoreDir: storeDir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 0 {
		t.Fatalf("second pass updated=%d, want 0 (idempotent)", res.Updated)
	}
}

func TestMigrateRunPaths_DryRunDoesNotWrite(t *testing.T) {
	storeDir := t.TempDir()
	p := writeRunJSON(t, storeDir, "r1", `{"file_path":"/x/examples/doc-align/main.bot"}`)
	res, err := MigrateRunPaths(MigrateRunPathsOptions{StoreDir: storeDir, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("dry-run updated=%d, want 1 (counted, not written)", res.Updated)
	}
	if got := readFile(t, p); !strings.Contains(got, "examples/doc-align/main.bot") {
		t.Errorf("dry-run must not modify the file: %s", got)
	}
}

func TestMigrateRunPaths_MissingStoreIsNoop(t *testing.T) {
	res, err := MigrateRunPaths(MigrateRunPathsOptions{StoreDir: filepath.Join(t.TempDir(), "nope")})
	if err != nil {
		t.Fatalf("missing store should be a no-op, got %v", err)
	}
	if res.Scanned != 0 || res.Updated != 0 {
		t.Fatalf("missing store should scan 0: %+v", res)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
