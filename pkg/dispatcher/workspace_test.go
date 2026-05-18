package dispatcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceCreateAndPath(t *testing.T) {
	w, err := NewWorkspaces(t.TempDir())
	if err != nil {
		t.Fatalf("NewWorkspaces: %v", err)
	}
	path, created, err := w.Create("native:abc-123")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created {
		t.Fatal("first create should report created=true")
	}
	if !strings.HasPrefix(path, w.Root()) {
		t.Fatalf("path %q should be under root %q", path, w.Root())
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("workspace not a directory: %v %v", info, err)
	}

	// Idempotent: re-create reports created=false.
	_, created2, err := w.Create("native:abc-123")
	if err != nil {
		t.Fatalf("re-Create: %v", err)
	}
	if created2 {
		t.Fatal("second create should report created=false")
	}
}

func TestWorkspaceSanitize(t *testing.T) {
	w, _ := NewWorkspaces(t.TempDir())
	path, _, err := w.Create("github:owner/repo#42")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := filepath.Base(path)
	if strings.ContainsAny(base, ":/#") {
		t.Fatalf("sanitized base still contains hostile chars: %q", base)
	}
}

func TestWorkspaceRejectsHiddenName(t *testing.T) {
	w, _ := NewWorkspaces(t.TempDir())
	path, _, err := w.Create(".hidden")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if strings.HasPrefix(filepath.Base(path), ".") {
		t.Fatalf("workspace name should not be hidden: %q", filepath.Base(path))
	}
}

func TestWorkspaceRefusesSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir() // a separate directory
	w, _ := NewWorkspaces(root)

	// Pre-plant a symlink with a sanitized name that points outside the root.
	id := "evil_link"
	planted := filepath.Join(root, id)
	if err := os.Symlink(outside, planted); err != nil {
		t.Skipf("symlink unsupported on this fs: %v", err)
	}

	_, _, err := w.Create(id)
	if err == nil || !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("expected escape rejection, got %v", err)
	}
}

func TestWorkspaceRemove(t *testing.T) {
	w, _ := NewWorkspaces(t.TempDir())
	path, _, err := w.Create("x")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := w.Remove("x"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("workspace not removed: %v", err)
	}
	// idempotent
	if err := w.Remove("x"); err != nil {
		t.Fatalf("Remove (absent): %v", err)
	}
}
