package secrets

import "testing"

func TestCredentials_GenericSecret(t *testing.T) {
	// nil map → empty string, never a panic.
	var zero Credentials
	if got := zero.GenericSecret("kubeconfig"); got != "" {
		t.Fatalf("GenericSecret on nil map = %q, want \"\"", got)
	}

	c := Credentials{Generic: map[string]string{"kubeconfig": "payload"}}
	if got := c.GenericSecret("kubeconfig"); got != "payload" {
		t.Fatalf("GenericSecret(present) = %q, want %q", got, "payload")
	}
	if got := c.GenericSecret("absent"); got != "" {
		t.Fatalf("GenericSecret(absent) = %q, want \"\"", got)
	}
}

func TestCredentials_OAuthDir(t *testing.T) {
	// nil map → empty string (the uncovered nil branch).
	var zero Credentials
	if got := zero.OAuthDir("claude_code"); got != "" {
		t.Fatalf("OAuthDir on nil map = %q, want \"\"", got)
	}

	c := Credentials{OAuthCredentialFiles: map[string]string{"claude_code": "/tmp/oauth"}}
	if got := c.OAuthDir("claude_code"); got != "/tmp/oauth" {
		t.Fatalf("OAuthDir(present) = %q, want %q", got, "/tmp/oauth")
	}
	if got := c.OAuthDir("codex"); got != "" {
		t.Fatalf("OAuthDir(absent) = %q, want \"\"", got)
	}
}
