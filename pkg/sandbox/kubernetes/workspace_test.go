package kubernetes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestResolveCloneRoot verifies the worktree→clone-root resolution that
// populateWorkspace relies on: a git worktree's `.git` is a pointer file,
// so the k8s driver must copy the real clone (with objects + origin), not
// the worktree, into the sandbox.
func TestResolveCloneRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	repo := filepath.Join(dir, "clone")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", repo)
	git("-C", repo, "commit", "-q", "--allow-empty", "-m", "init")

	// plain clone → itself
	if got := resolveCloneRoot(ctx, repo); !sameDir(got, repo) {
		t.Errorf("resolveCloneRoot(clone) = %q, want %q", got, repo)
	}

	// worktree (its .git is a pointer file) → the clone root
	wt := filepath.Join(dir, "wt")
	git("-C", repo, "worktree", "add", "-q", "--detach", wt)
	if got := resolveCloneRoot(ctx, wt); !sameDir(got, repo) {
		t.Errorf("resolveCloneRoot(worktree) = %q, want clone root %q", got, repo)
	}

	// non-git dir → itself (best-effort fallback)
	plain := t.TempDir()
	if got := resolveCloneRoot(ctx, plain); got != plain {
		t.Errorf("resolveCloneRoot(non-git) = %q, want %q", got, plain)
	}
}

func sameDir(a, b string) bool {
	ra, erra := filepath.EvalSymlinks(a)
	rb, errb := filepath.EvalSymlinks(b)
	return erra == nil && errb == nil && ra == rb
}
