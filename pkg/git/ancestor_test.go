package git

import "testing"

// TestIsAncestor covers the out-of-band-merge detection the run-view uses
// to avoid offering a redundant "Squash and merge" for a branch already
// landed on the target (ticket: stale merge button after a git-CLI merge).
func TestIsAncestor(t *testing.T) {
	dir := gitRepo(t)                        // root commit on main
	a := resolveSHA(t, dir, "HEAD")          // root
	b := commit(t, dir, "b.txt", "b\n", "b") // main: a -> b

	mustRun(t, dir, "checkout", "-q", "-b", "feat")
	c := commit(t, dir, "c.txt", "c\n", "c") // feat: b -> c

	cases := []struct {
		name              string
		ancestor, descend string
		want              bool
	}{
		{"root is ancestor of b", a, b, true},
		{"b is ancestor of c", b, c, true},
		{"a is ancestor of c (transitive)", a, c, true},
		{"identical commit is its own ancestor", b, b, true},
		{"descendant is not ancestor of its parent", c, b, false},
		{"later commit not ancestor of earlier", c, a, false},
		{"empty ancestor", "", b, false},
		{"empty descendant", a, "", false},
		{"empty repo root", a, b, false},
		{"unknown ancestor sha", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", b, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := dir
			if tc.name == "empty repo root" {
				root = ""
			}
			if got := IsAncestor(root, tc.ancestor, tc.descend); got != tc.want {
				t.Errorf("IsAncestor(root=%q, %q, %q) = %v, want %v", root, tc.ancestor, tc.descend, got, tc.want)
			}
		})
	}

	// Out-of-band merge: FF main up to c (simulating a git-CLI merge of the
	// run's storage branch). c — the run's FinalCommit — is now an ancestor
	// of main / HEAD, which is exactly the signal reconcileOutOfBandMerge
	// keys on to skip the redundant squash.
	mustRun(t, dir, "checkout", "-q", "main")
	mustRun(t, dir, "merge", "-q", "--ff-only", "feat")
	if !IsAncestor(dir, c, "HEAD") {
		t.Errorf("after FF merge, FinalCommit %s should be an ancestor of HEAD", c)
	}
	if !IsAncestor(dir, c, "main") {
		t.Errorf("after FF merge, FinalCommit %s should be an ancestor of main", c)
	}
}
