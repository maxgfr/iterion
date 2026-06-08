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
