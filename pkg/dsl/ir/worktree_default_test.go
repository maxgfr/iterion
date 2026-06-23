package ir

import "testing"

// TestWorktreeDefaultUnsetResolvesToAuto locks the IR-level default:
// a workflow that does not declare `worktree:` resolves to "auto" so
// every run is isolated in a per-run git worktree by default. The
// runtime owns the "is this a git repo?" precheck and degrades to
// in-place when isolation is impossible — see workspaceIsGitRepo
// in pkg/runtime/worktree.go.
func TestWorktreeDefaultUnsetResolvesToAuto(t *testing.T) {
	src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty

workflow wf:
  entry: start
  start -> done
`
	w := mustCompile(t, src)
	if got, want := w.Worktree, "auto"; got != want {
		t.Errorf("Worktree default = %q, want %q", got, want)
	}
}

// TestWorktreeExplicitNonePreserved locks the explicit opt-out path:
// `worktree: none` must reach the runtime verbatim so the engine skips
// the worktree setup branch entirely (and so C100 still fires for
// review gates that opted out of isolation).
func TestWorktreeExplicitNonePreserved(t *testing.T) {
	src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty

workflow wf:
  entry: start
  worktree: none
  start -> done
`
	w := mustCompile(t, src)
	if got, want := w.Worktree, "none"; got != want {
		t.Errorf("Worktree explicit = %q, want %q", got, want)
	}
}

// TestWorktreeExplicitAutoPreserved confirms the explicit "auto"
// declaration round-trips through compile unchanged — the helper is
// a default resolver, not a normalizer that loses information.
func TestWorktreeExplicitAutoPreserved(t *testing.T) {
	src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty

workflow wf:
  entry: start
  worktree: auto
  start -> done
`
	w := mustCompile(t, src)
	if got, want := w.Worktree, "auto"; got != want {
		t.Errorf("Worktree explicit auto = %q, want %q", got, want)
	}
}

// TestDefaultWorktreeMode covers the helper directly: each input shape
// (unset, none, auto, casing variations, whitespace, unknowns) is
// pinned so future edits cannot quietly change the default.
func TestDefaultWorktreeMode(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "auto"},
		{"auto", "auto"},
		{"AUTO", "auto"},
		{"  auto  ", "auto"},
		{"none", "none"},
		{"None", "none"},
		{" none ", "none"},
		{"weird", "weird"}, // strict diagnostic flows can flag unknowns later
	}
	for _, tc := range cases {
		if got := defaultWorktreeMode(tc.in); got != tc.want {
			t.Errorf("defaultWorktreeMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
