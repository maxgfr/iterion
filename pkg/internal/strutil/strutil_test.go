package strutil

import "testing"

func TestFirstNonBlank(t *testing.T) {
	if got := FirstNonBlank("", "  ", "x", "y"); got != "x" {
		t.Fatalf("FirstNonBlank = %q; want x", got)
	}
	if got := FirstNonBlank("", " ", "\t"); got != "" {
		t.Fatalf("FirstNonBlank(all blank) = %q; want empty", got)
	}
	if got := FirstNonBlank(); got != "" {
		t.Fatalf("FirstNonBlank() = %q; want empty", got)
	}
}
