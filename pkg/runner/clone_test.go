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

func TestValidateRepoTarget(t *testing.T) {
	cases := []struct {
		name             string
		repoURL, repoSHA string
		wantErr          bool
	}{
		// Valid: https + ssh URLs, with/without a ref.
		{"https no ref", "https://github.com/org/repo.git", "", false},
		{"https with sha", "https://github.com/org/repo.git", "a1b2c3d4e5f6", false},
		{"https with branch ref", "https://gitlab.example/grp/repo.git", "feature/x", false},
		{"https with pull ref", "https://github.com/org/repo.git", "refs/pull/12/head", false},
		{"scp-like ssh", "git@github.com:org/repo.git", "main", false},
		{"ssh url", "ssh://git@host/org/repo.git", "main", false},
		// Injection: remote-helper transport in the URL → RCE vector.
		{"ext remote helper", "ext::sh -c 'id'", "main", true},
		{"transport marker", "fd::17", "main", true},
		// Injection: local-repo / cleartext transports git would honour.
		{"file url", "file:///etc/passwd", "main", true},
		{"git proto", "git://host/repo.git", "main", true},
		{"http cleartext", "http://host/repo.git", "main", true},
		// Empty / null URL.
		{"empty url", "", "main", true},
		{"null byte url", "https://h/r\x00.git", "main", true},
		// Injection: flag-shaped ref → `git fetch`/`checkout` option injection.
		{"flag ref upload-pack", "https://github.com/org/repo.git", "--upload-pack=/evil", true},
		{"flag ref dash", "https://github.com/org/repo.git", "-O/tmp/x", true},
		{"traversal ref", "https://github.com/org/repo.git", "a/../../b", true},
		{"null byte ref", "https://github.com/org/repo.git", "ma\x00in", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateRepoTarget(c.repoURL, c.repoSHA)
			if c.wantErr && err == nil {
				t.Fatalf("validateRepoTarget(%q, %q) = nil; want error", c.repoURL, c.repoSHA)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("validateRepoTarget(%q, %q) = %v; want nil", c.repoURL, c.repoSHA, err)
			}
		})
	}
}
