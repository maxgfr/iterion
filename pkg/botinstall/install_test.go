package botinstall

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// writeBundle creates a minimal bundle directory (main.bot + manifest.yaml).
// schemaVersion 0 → decodeManifest treats it as v1; a non-1, non-0 value is
// used by the malformed-rejection test.
func writeBundle(t *testing.T, dir, name string, schemaVersion int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.bot"), []byte("# "+name+"\nworkflow w:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	man := "name: " + name + "\nversion: 0.1.0\ndescription: test bot\n"
	if schemaVersion != 0 {
		man += "schema_version: " + strconv.Itoa(schemaVersion) + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(man), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInstall_SingleBundleRoot(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, repo, "mybot", 0)
	dest := t.TempDir()
	res, err := Install(context.Background(), Options{Source: repo, Dest: dest, Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "mybot" {
		t.Errorf("name: %q", res.Name)
	}
	if _, err := os.Stat(filepath.Join(dest, "mybot", "main.bot")); err != nil {
		t.Errorf("main.bot not installed: %v", err)
	}
}

func TestInstall_NameOverride(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, repo, "mybot", 0)
	dest := t.TempDir()
	res, err := Install(context.Background(), Options{Source: repo, Dest: dest, Name: "renamed", Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "renamed" {
		t.Errorf("name: %q", res.Name)
	}
	if _, err := os.Stat(filepath.Join(dest, "renamed", "main.bot")); err != nil {
		t.Errorf("not installed under override name: %v", err)
	}
}

func TestInstall_MalformedRejected(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, repo, "bad", 99) // unsupported schema_version
	dest := t.TempDir()
	if _, err := Install(context.Background(), Options{Source: repo, Dest: dest, Workdir: t.TempDir()}); err == nil {
		t.Fatal("expected install of a malformed bundle to fail")
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("nothing should be installed on validation failure, got %d entries", len(entries))
	}
}

func TestInstall_ExistingNeedsForce(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, repo, "mybot", 0)
	dest := t.TempDir()
	wd := t.TempDir()
	if _, err := Install(context.Background(), Options{Source: repo, Dest: dest, Workdir: wd}); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(context.Background(), Options{Source: repo, Dest: dest, Workdir: wd}); err == nil {
		t.Fatal("re-install without --force should fail")
	}
	if _, err := Install(context.Background(), Options{Source: repo, Dest: dest, Workdir: wd, Force: true}); err != nil {
		t.Fatalf("re-install with --force should succeed: %v", err)
	}
}

func TestInstall_MultiBundleNeedsPath(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, filepath.Join(repo, "a"), "bot-a", 0)
	writeBundle(t, filepath.Join(repo, "b"), "bot-b", 0)
	dest := t.TempDir()
	wd := t.TempDir()
	if _, err := Install(context.Background(), Options{Source: repo, Dest: dest, Workdir: wd}); err == nil {
		t.Fatal("multi-bundle repo without --path should fail")
	}
	res, err := Install(context.Background(), Options{Source: repo, Dest: dest, Path: "b", Workdir: wd})
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "bot-b" {
		t.Errorf("name: %q (want bot-b)", res.Name)
	}
}

func TestInstall_PathTraversalRejected(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, filepath.Join(repo, "a"), "bot-a", 0)
	if _, err := Install(context.Background(), Options{
		Source: repo, Dest: t.TempDir(), Path: "../../etc", Workdir: t.TempDir(),
	}); err == nil {
		t.Fatal("--path escaping the repo should be rejected")
	}
}

func TestInstall_RepoIndexConvention(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, filepath.Join(repo, "tools", "willy"), "willy", 0)
	idx := "bots:\n  - name: willy\n    path: tools/willy\n    description: improver\n"
	if err := os.WriteFile(filepath.Join(repo, repoBotIndexName), []byte(idx), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Install(context.Background(), Options{Source: repo, Dest: t.TempDir(), Path: "willy", Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "willy" {
		t.Errorf("name: %q", res.Name)
	}
}
