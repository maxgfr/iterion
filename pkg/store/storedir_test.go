package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStoreDir_OverrideWins(t *testing.T) {
	got := ResolveStoreDir("/some/start", "/explicit/store")
	if got != "/explicit/store" {
		t.Fatalf("override should be returned verbatim, got %q", got)
	}
}

func TestResolveStoreDir_EmptyStartFallsBack(t *testing.T) {
	got := ResolveStoreDir("", "")
	if got != StoreDirName {
		t.Fatalf("empty start should fall back to %q, got %q", StoreDirName, got)
	}
}

func TestResolveStoreDir_FindsAncestor(t *testing.T) {
	root := t.TempDir()
	// Mark root as a git repo so the bounded walk-up can climb to it.
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	storeAt := filepath.Join(root, StoreDirName)
	if err := os.Mkdir(storeAt, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := ResolveStoreDir(deep, "")
	if got != storeAt {
		t.Fatalf("should find ancestor store at %q, got %q", storeAt, got)
	}
}

func TestResolveStoreDir_FindsAtSelf(t *testing.T) {
	root := t.TempDir()
	storeAt := filepath.Join(root, StoreDirName)
	if err := os.Mkdir(storeAt, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := ResolveStoreDir(root, "")
	if got != storeAt {
		t.Fatalf("should find store at start dir, got %q", got)
	}
}

func TestResolveStoreDir_NoAncestorCreatesNextToStart(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := ResolveStoreDir(deep, "")

	absDeep, _ := filepath.Abs(deep)
	want := filepath.Join(absDeep, StoreDirName)
	if got != want {
		t.Fatalf("no ancestor → expected %q, got %q", want, got)
	}
}

func TestResolveStoreDir_IgnoresFileNamedDotIterion(t *testing.T) {
	root := t.TempDir()
	// A regular file named .iterion must NOT be confused with the store dir.
	if err := os.WriteFile(filepath.Join(root, StoreDirName), []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	deep := filepath.Join(root, "sub")
	if err := os.Mkdir(deep, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := ResolveStoreDir(deep, "")

	absDeep, _ := filepath.Abs(deep)
	want := filepath.Join(absDeep, StoreDirName)
	if got != want {
		t.Fatalf("file (not dir) named .iterion must be skipped — expected %q, got %q", want, got)
	}
}

func TestResolveStoreDir_NearestAncestorWins(t *testing.T) {
	root := t.TempDir()
	// Mark `project/` as a git repo so the bounded walk-up climbs to it
	// without needing to escape into `root` (where the legacy outer
	// .iterion sits — under the new bounded behaviour it must NOT be
	// reached).
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Two .iterion dirs in the chain — the nearest (deepest) must win,
	// AND the outer one (above the repo root) must not be picked up
	// even if the inner is missing.
	outer := filepath.Join(root, StoreDirName)
	inner := filepath.Join(project, StoreDirName)
	if err := os.MkdirAll(outer, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	deep := filepath.Join(project, "sub", "leaf")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := ResolveStoreDir(deep, "")
	if got != inner {
		t.Fatalf("nearest ancestor (within repo) should win, expected %q got %q", inner, got)
	}
}

// TestResolveStoreDir_DoesNotEscapeRepo guards the regression that
// motivated the git-bounded walk: a stray .iterion above the repo root
// must NEVER be picked up by a workdir inside the repo, even when the
// repo itself has no .iterion of its own.
func TestResolveStoreDir_DoesNotEscapeRepo(t *testing.T) {
	root := t.TempDir()
	// Stray .iterion above the repo (typical: a long-forgotten ~/.iterion
	// the user created once, years ago).
	stray := filepath.Join(root, StoreDirName)
	if err := os.Mkdir(stray, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A fresh repo with no .iterion of its own.
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	deep := filepath.Join(project, "sub", "leaf")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := ResolveStoreDir(deep, "")
	want := filepath.Join(project, StoreDirName)
	if got != want {
		t.Fatalf("walk-up must stop at repo root, expected %q got %q (would have leaked to %q)", want, got, stray)
	}
}

// TestResolveStoreDir_NoRepoDoesNotWalkUp asserts the second half of
// the bounded-walk policy: when the workdir is not inside any git
// repository, we never inherit a parent .iterion — we just create
// one alongside the workdir. Without this guard, a temp dir nested
// under a user home that has a stray .iterion would silently capture
// the user's runs into the wrong store.
func TestResolveStoreDir_NoRepoDoesNotWalkUp(t *testing.T) {
	root := t.TempDir()
	// Stray .iterion above start, but no .git anywhere.
	if err := os.Mkdir(filepath.Join(root, StoreDirName), 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := ResolveStoreDir(deep, "")
	absDeep, _ := filepath.Abs(deep)
	want := filepath.Join(absDeep, StoreDirName)
	if got != want {
		t.Fatalf("non-repo workdir must not inherit parent .iterion, expected %q got %q", want, got)
	}
}
