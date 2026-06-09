package delegate

import (
	"strings"
	"testing"
)

// TestBuildSystemPrompt_SecretsHygiene verifies the anti-exfil backstop
// clause is appended for both backend prompt modes when SecretsHygiene
// is set, and omitted otherwise.
func TestBuildSystemPrompt_SecretsHygiene(t *testing.T) {
	for _, mode := range []SystemPromptMode{SystemPromptAuthoredBase, SystemPromptAppendToNative, SystemPromptStandalone} {
		on := Task{SystemPrompt: "do the task", SystemPromptMode: mode, SecretsHygiene: true}.BuildSystemPrompt()
		if !strings.Contains(on, "## Secret handling") {
			t.Errorf("mode %d: expected secret-handling section when SecretsHygiene=true", mode)
		}
		if !strings.Contains(on, "__ITERION_SECRET_") {
			t.Errorf("mode %d: expected placeholder guidance in clause", mode)
		}
		off := Task{SystemPrompt: "do the task", SystemPromptMode: mode}.BuildSystemPrompt()
		if strings.Contains(off, "## Secret handling") {
			t.Errorf("mode %d: clause must be omitted when SecretsHygiene=false", mode)
		}
	}
}

func TestBuildSystemPrompt_SecretFileHints(t *testing.T) {
	got := Task{
		SystemPrompt: "do the task",
		SecretFiles: []SecretFileHint{{
			Name: "kubeconfig",
			Path: "/run/iterion/secrets/kubeconfig",
			Env:  "KUBECONFIG",
		}},
	}.BuildSystemPrompt()
	for _, want := range []string{
		"## Secret handling",
		"Mounted secret files",
		"/run/iterion/secrets/kubeconfig",
		"$KUBECONFIG",
		"do not open, read, cat, print",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}
