package workflowfile

import "testing"

func TestIsWorkflowFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"foo.yaml", false},
		{"foo.bot", true},
		{"path/to/foo.yaml", false},
		{"path/to/foo.bot", true},
		{"foo.txt", false},
		{"foo", false},
		{"foo.bot.bak", false},
		{".yaml", false},
		{".bot", true},
		{"foo.botz", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsWorkflowFile(tc.path); got != tc.want {
			t.Errorf("IsWorkflowFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
