package secrets

import "testing"

func TestDefaultFileMountPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"clean name untouched", "github_token", "/run/iterion/secrets/github_token"},
		{"dots and dashes kept inside", "my.api-key_v2", "/run/iterion/secrets/my.api-key_v2"},
		{"non-alnum run collapses to single underscore", "a@b!c", "/run/iterion/secrets/a_b_c"},
		{"slashes and traversal stripped", "my secret/../key", "/run/iterion/secrets/my_secret_.._key"},
		{"leading/trailing ._- trimmed", "_tok_.", "/run/iterion/secrets/tok"},
		{"all-trim chars fall back to secret", "...---", "/run/iterion/secrets/secret"},
		{"empty falls back to secret", "", "/run/iterion/secrets/secret"},
		{"only separators fall back to secret", "///", "/run/iterion/secrets/secret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DefaultFileMountPath(c.in)
			if got != c.want {
				t.Fatalf("DefaultFileMountPath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestResolveFileMountPath(t *testing.T) {
	cases := []struct {
		name     string
		secret   string
		override string
		want     string
	}{
		{"override returned verbatim", "tok", "/custom/path/tok", "/custom/path/tok"},
		{"blank override falls back to default", "tok", "", "/run/iterion/secrets/tok"},
		{"whitespace-only override falls back to default", "tok", "   ", "/run/iterion/secrets/tok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveFileMountPath(c.secret, c.override)
			if got != c.want {
				t.Fatalf("ResolveFileMountPath(%q, %q) = %q, want %q", c.secret, c.override, got, c.want)
			}
		})
	}
}

func TestRelativeToSecretFilesMountDir(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantRel string
		wantOK  bool
	}{
		{"direct child", "/run/iterion/secrets/tok", "tok", true},
		{"nested child", "/run/iterion/secrets/a/b", "a/b", true},
		{"non-clean input rejected", "/run/iterion/secrets/../x", "", false},
		{"interior traversal rejected", "/run/iterion/secrets/a/../b", "", false},
		{"outside the mount dir rejected", "/etc/passwd", "", false},
		{"prefix without trailing slash rejected", "/run/iterion/secrets", "", false},
		{"sibling dir sharing the prefix rejected", "/run/iterion/secrets-evil/tok", "", false},
		{"trailing slash (non-clean) rejected", "/run/iterion/secrets/tok/", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rel, ok := RelativeToSecretFilesMountDir(c.in)
			if ok != c.wantOK || rel != c.wantRel {
				t.Fatalf("RelativeToSecretFilesMountDir(%q) = (%q, %v), want (%q, %v)", c.in, rel, ok, c.wantRel, c.wantOK)
			}
		})
	}
}
