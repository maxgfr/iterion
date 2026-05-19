package git

import "testing"

func TestValidateBranchName(t *testing.T) {
	t.Parallel()

	valid := []string{
		"main",
		"feature/x",
		"iterion/run/2026-05-19",
		"a",
		"v1.2.3",
		"hot-fix",
		"a.b.c/d_e",
	}
	for _, name := range valid {
		if err := ValidateBranchName(name); err != nil {
			t.Errorf("ValidateBranchName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []struct {
		name   string
		reason string
	}{
		{"", "empty"},
		{"-force", "leading dash (flag injection)"},
		{"--force", "looks like a long flag"},
		{"/abs", "leading slash"},
		{".hidden", "leading dot"},
		{"_under", "leading underscore"},
		{"feat with space", "space"},
		{"feat\ttab", "tab"},
		{"feat:colon", "colon (git ref-format)"},
		{"feat?glob", "question mark"},
		{"feat*star", "wildcard"},
		{"feat~tilde", "tilde"},
		{"feat^caret", "caret"},
		{"feat\\bs", "backslash"},
		{"feat@{ref}", "@{ sequence"},
		{"feat\x00null", "null byte"},
		{"branch/..", "contains .."},
		{"feature/../etc", "traversal"},
		{"branch//double", "double slash"},
		{"trailing/", "trailing slash"},
		{"trailing.", "trailing dot"},
		{"trailing.lock", ".lock suffix"},
	}
	for _, tc := range invalid {
		if err := ValidateBranchName(tc.name); err == nil {
			t.Errorf("ValidateBranchName(%q) = nil, want error (%s)", tc.name, tc.reason)
		}
	}

	long := make([]byte, 256)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateBranchName(string(long)); err == nil {
		t.Errorf("ValidateBranchName(256 bytes) = nil, want error")
	}
}
