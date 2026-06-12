package botinstall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspect_PopulatesMetadata(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, repo, "mybot", 0)
	// Enrich the manifest with the fields Inspect surfaces.
	man := strings.Join([]string{
		"name: mybot",
		"version: 1.2.3",
		"description: a sample bot",
		"author: jo",
		"display_name: My Bot",
		"triggers: [hello, world]",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repo, "manifest.yaml"), []byte(man), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Hello\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	// One preset.
	if err := os.MkdirAll(filepath.Join(repo, "presets"), 0o755); err != nil {
		t.Fatal(err)
	}
	preset := "---\nname: sre\ndisplay_name: SRE focus\ndescription: bias toward reliability\nskills: [obs, reliability]\n---\nfocus on uptime\n"
	if err := os.WriteFile(filepath.Join(repo, "presets", "sre.md"), []byte(preset), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	before, _ := os.ReadDir(dest)

	md, err := Inspect(context.Background(), Options{Source: repo, Dest: dest, Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if md.Name != "mybot" {
		t.Errorf("Name = %q", md.Name)
	}
	if md.DisplayName != "My Bot" {
		t.Errorf("DisplayName = %q", md.DisplayName)
	}
	if md.Description != "a sample bot" {
		t.Errorf("Description = %q", md.Description)
	}
	if md.Author != "jo" {
		t.Errorf("Author = %q", md.Author)
	}
	if md.Version != "1.2.3" {
		t.Errorf("Version = %q", md.Version)
	}
	if len(md.Triggers) != 2 || md.Triggers[0] != "hello" || md.Triggers[1] != "world" {
		t.Errorf("Triggers = %v", md.Triggers)
	}
	if len(md.Presets) != 1 {
		t.Fatalf("Presets = %d, want 1", len(md.Presets))
	}
	if md.Presets[0].Name != "sre" || md.Presets[0].DisplayName != "SRE focus" {
		t.Errorf("Presets[0] = %+v", md.Presets[0])
	}
	if len(md.Presets[0].Skills) != 2 {
		t.Errorf("Presets[0].Skills = %v", md.Presets[0].Skills)
	}
	if !strings.Contains(md.README, "Hello") {
		t.Errorf("README missing content: %q", md.README)
	}

	// Inspect must not write into the dest directory.
	after, _ := os.ReadDir(dest)
	if len(after) != len(before) {
		t.Errorf("Inspect wrote into dest: %d entries before, %d after", len(before), len(after))
	}
}

func TestInspect_MalformedRejected(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, repo, "bad", 99) // unsupported schema_version
	if _, err := Inspect(context.Background(), Options{Source: repo}); err == nil {
		t.Fatal("expected Inspect of a malformed bundle to fail")
	}
}

func TestInspect_NoREADMENoError(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, repo, "mybot", 0)
	md, err := Inspect(context.Background(), Options{Source: repo})
	if err != nil {
		t.Fatal(err)
	}
	if md.README != "" {
		t.Errorf("README = %q, want empty", md.README)
	}
}

func TestInspect_READMECapped(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, repo, "mybot", 0)
	// Write something larger than the cap.
	big := strings.Repeat("a", readmeMaxBytes+5000)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	md, err := Inspect(context.Background(), Options{Source: repo})
	if err != nil {
		t.Fatal(err)
	}
	if len(md.README) > readmeMaxBytes {
		t.Errorf("README length %d > cap %d", len(md.README), readmeMaxBytes)
	}
	if len(md.README) == 0 {
		t.Errorf("expected README content, got empty")
	}
}

func TestInspect_MultiBundleNeedsPath(t *testing.T) {
	repo := t.TempDir()
	writeBundle(t, filepath.Join(repo, "a"), "bot-a", 0)
	writeBundle(t, filepath.Join(repo, "b"), "bot-b", 0)
	if _, err := Inspect(context.Background(), Options{Source: repo}); err == nil {
		t.Fatal("multi-bundle repo without --path should fail")
	}
	md, err := Inspect(context.Background(), Options{Source: repo, Path: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if md.Name != "bot-b" {
		t.Errorf("Name = %q, want bot-b", md.Name)
	}
}
