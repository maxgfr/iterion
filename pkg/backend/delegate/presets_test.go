package delegate

import (
	"strings"
	"testing"
)

// TestBuildSystemPrompt_PresetFocus verifies the launch-time preset bias is
// rendered under a "## Focus" section, and omitted entirely when no preset is
// active — parallel to the cursor "## Calibration" section.
func TestBuildSystemPrompt_PresetFocus(t *testing.T) {
	task := Task{
		SystemPrompt:   "Base task.",
		PresetFragment: "Operate as an SRE.\n\nRelevant skills (consult before acting): lang-js-fallow",
	}
	out := task.BuildSystemPrompt()
	if !strings.Contains(out, "## Focus") {
		t.Fatalf("missing ## Focus section:\n%s", out)
	}
	if !strings.Contains(out, "Operate as an SRE.") || !strings.Contains(out, "lang-js-fallow") {
		t.Fatalf("focus body/skills missing:\n%s", out)
	}
	// The base prompt is preserved.
	if !strings.Contains(out, "Base task.") {
		t.Fatalf("base prompt dropped:\n%s", out)
	}

	// No preset → no section.
	bare := Task{SystemPrompt: "Base."}
	if strings.Contains(bare.BuildSystemPrompt(), "## Focus") {
		t.Fatal("## Focus must be absent when PresetFragment is empty")
	}
}

// TestBuildSystemPrompt_FocusAfterCalibration documents the ordering contract:
// calibration (author-time cursors) precedes focus (operator-time preset).
func TestBuildSystemPrompt_FocusAfterCalibration(t *testing.T) {
	task := Task{
		SystemPrompt:    "Base.",
		CursorFragments: []string{"**rigor:** be thorough"},
		PresetFragment:  "Be an SRE.",
	}
	out := task.BuildSystemPrompt()
	ci := strings.Index(out, "## Calibration")
	fi := strings.Index(out, "## Focus")
	if ci < 0 || fi < 0 {
		t.Fatalf("expected both sections:\n%s", out)
	}
	if ci > fi {
		t.Fatalf("calibration should precede focus:\n%s", out)
	}
}
