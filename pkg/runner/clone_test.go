package runner

import "testing"

func TestInjectGitToken(t *testing.T) {
	cases := []struct{ in, tok, want string }{
		// https → oauth2 userinfo injected
		{"https://gitlab.example/grp/repo.git", "tok123", "https://oauth2:tok123@gitlab.example/grp/repo.git"},
		// no token → unchanged
		{"https://gitlab.example/grp/repo.git", "", "https://gitlab.example/grp/repo.git"},
		// non-https (scp-like / http) → unchanged, never carry a token in cleartext schemes
		{"git@github.com:grp/repo.git", "tok", "git@github.com:grp/repo.git"},
		{"http://insecure/repo.git", "tok", "http://insecure/repo.git"},
	}
	for _, c := range cases {
		if got := injectGitToken(c.in, c.tok); got != c.want {
			t.Errorf("injectGitToken(%q, %q) = %q; want %q", c.in, c.tok, got, c.want)
		}
	}
}

func TestFirstNonBlank(t *testing.T) {
	if got := firstNonBlank("", "  ", "x", "y"); got != "x" {
		t.Fatalf("firstNonBlank = %q; want x", got)
	}
	if got := firstNonBlank("", " ", "\t"); got != "" {
		t.Fatalf("firstNonBlank(all blank) = %q; want empty", got)
	}
}
