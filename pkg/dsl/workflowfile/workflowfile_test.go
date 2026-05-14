package workflowfile

import "testing"

func TestIsWorkflowFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"foo.iter", true},
		{"foo.bot", true},
		{"path/to/foo.iter", true},
		{"path/to/foo.bot", true},
		{"foo.txt", false},
		{"foo", false},
		{"foo.iter.bak", false},
		{".iter", true},
		{".bot", true},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsWorkflowFile(tc.path); got != tc.want {
			t.Errorf("IsWorkflowFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
