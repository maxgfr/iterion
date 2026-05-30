package delegate

import "testing"

// TestEditMissCount covers the consecutive Edit-miss tally that drives the
// PostToolUse re-Read hint (see editMissHintAfter + the Edit-resilience hook).
func TestEditMissCount(t *testing.T) {
	const notFound = "<tool_use_error>String to replace not found in file.\nString: foo</tool_use_error>"

	tests := []struct {
		name     string
		toolName string
		response string
		prev     int
		want     int
	}{
		{"edit miss from zero", "Edit", notFound, 0, 1},
		{"edit miss increments", "Edit", notFound, 1, 2},
		{"multiedit miss increments", "MultiEdit", notFound, 2, 3},
		{"edit success resets", "Edit", "The file has been updated.", 3, 0},
		{"edit different error resets", "Edit", "Permission denied", 2, 0},
		{"non-edit tool leaves tally unchanged", "Read", "file contents", 2, 2},
		{"non-edit tool does not reset a wedge", "Bash", "ok", 1, 1},
		{"non-edit tool from zero stays zero", "Grep", "match", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := editMissCount(tt.toolName, tt.response, tt.prev); got != tt.want {
				t.Fatalf("editMissCount(%q, …, %d) = %d, want %d", tt.toolName, tt.prev, got, tt.want)
			}
		})
	}
}

// TestEditMissCount_ReadBetweenMissesDoesNotReset is the wedge scenario the
// hook targets: Edit-miss → Read (recovery attempt) → Edit-miss should reach
// the hint threshold, because the intervening Read must not reset the tally.
func TestEditMissCount_ReadBetweenMissesDoesNotReset(t *testing.T) {
	n := 0
	n = editMissCount("Edit", "String to replace not found in file", n) // miss 1
	n = editMissCount("Read", "current file contents", n)               // recovery read
	n = editMissCount("Edit", "String to replace not found in file", n) // miss 2
	if n < editMissHintAfter {
		t.Fatalf("after miss→read→miss, count=%d; want >= editMissHintAfter (%d) so the hint fires", n, editMissHintAfter)
	}
}
