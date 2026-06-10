package knowledge

import (
	"errors"
	"testing"
)

func TestValidateDocPath(t *testing.T) {
	ok := []string{"a.md", "findings/2026.md", "a/b/c.md", "dotted.name.md"}
	for _, p := range ok {
		if err := ValidateDocPath(p); err != nil {
			t.Errorf("ValidateDocPath(%q) = %v, want nil", p, err)
		}
	}
	bad := []string{
		"",          // empty
		"/abs.md",   // absolute
		"../escape", // traversal
		"a/../../x", // mid traversal
		"..",        // bare dotdot
		`\\windows`, // backslash absolute
		"C:/win.md", // drive letter
		"a\x00b.md", // NUL
	}
	for _, p := range bad {
		if err := ValidateDocPath(p); err == nil {
			t.Errorf("ValidateDocPath(%q) = nil, want error", p)
		} else if !errors.Is(err, ErrInvalidDocPath) {
			t.Errorf("ValidateDocPath(%q) error = %v, want ErrInvalidDocPath", p, err)
		}
	}
}
