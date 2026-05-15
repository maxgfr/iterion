package projects

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPruneDeadProjectsDropsMissingDirs(t *testing.T) {
	alive := t.TempDir() // exists for the test lifetime
	dead := filepath.Join(t.TempDir(), "definitely-not-there")

	in := []Project{
		{ID: "a", Name: "alive", Dir: alive, LastOpened: time.Now()},
		{ID: "b", Name: "dead", Dir: dead, LastOpened: time.Now()},
		{ID: "c", Name: "empty", Dir: "", LastOpened: time.Now()},
	}
	out := pruneDeadProjects(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 surviving project, got %d: %+v", len(out), out)
	}
	if out[0].ID != "a" {
		t.Errorf("expected alive entry, got %q", out[0].ID)
	}
}

func TestLoadPromotesNextBestCurrentWhenPruned(t *testing.T) {
	// Build a config in memory, point Load at a temp path.
	alive := t.TempDir()
	dead := filepath.Join(t.TempDir(), "phantom")

	cfg := &Config{
		Version: schemaVersion,
		RecentProjects: []Project{
			{ID: "dead-id", Name: "phantom", Dir: dead, LastOpened: time.Now()},
			{ID: "alive-id", Name: filepath.Base(alive), Dir: alive, LastOpened: time.Now().Add(-time.Hour)},
		},
		CurrentProjectID: "dead-id",
	}
	tmp := filepath.Join(t.TempDir(), "config.json")
	cfg.path = tmp
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := loadFrom(tmp)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.RecentProjects) != 1 || loaded.RecentProjects[0].ID != "alive-id" {
		t.Fatalf("expected only alive project survived, got %+v", loaded.RecentProjects)
	}
	if loaded.CurrentProjectID != "alive-id" {
		t.Errorf("expected promoted CurrentProjectID=alive-id, got %q", loaded.CurrentProjectID)
	}
}
