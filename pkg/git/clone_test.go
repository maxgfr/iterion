package git

import (
	"strings"
	"testing"
)

func TestValidateCloneSource(t *testing.T) {
	accepted := []struct {
		name string
		src  string
	}{
		{"https", "https://github.com/org/repo.git"},
		{"https no .git", "https://example.com/org/repo"},
		{"ssh scheme", "ssh://git@host.example.com/org/repo.git"},
		{"ssh scheme no user", "ssh://host.example.com/org/repo.git"},
		{"scp-like", "git@github.com:org/repo.git"},
		{"scp-like no user", "github.com:org/repo.git"},
	}
	for _, tc := range accepted {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			if err := ValidateCloneSource(tc.src); err != nil {
				t.Errorf("ValidateCloneSource(%q) = %v, want nil", tc.src, err)
			}
		})
	}

	rejected := []struct {
		name string
		src  string
	}{
		{"ext remote helper", "ext::sh -c 'touch /tmp/pwned'"},
		{"file scheme", "file:///etc/passwd"},
		{"git scheme", "git://github.com/org/repo.git"},
		{"http cleartext", "http://github.com/org/repo.git"},
		{"ftp", "ftp://host/repo"},
		{"arbitrary remote helper", "transport::address"},
		{"empty", ""},
		{"whitespace", "   "},
		{"null byte", "https://h/r\x00"},
		{"absolute path", "/tmp/some/repo"},
		{"relative path", "./repo"},
		{"plain word", "repo"},
	}
	for _, tc := range rejected {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			err := ValidateCloneSource(tc.src)
			if err == nil {
				t.Fatalf("ValidateCloneSource(%q) = nil, want error", tc.src)
			}
			// Acceptance #3: user-facing errors explain the source transport
			// is unsupported. Empty/null-byte are the structural exceptions.
			if tc.src != "" && strings.TrimSpace(tc.src) != "" && !strings.Contains(tc.src, "\x00") {
				msg := err.Error()
				if !strings.Contains(msg, "transport") && !strings.Contains(msg, "supported") {
					t.Errorf("error for %q = %q, want it to mention transport/supported", tc.src, msg)
				}
			}
		})
	}
}
