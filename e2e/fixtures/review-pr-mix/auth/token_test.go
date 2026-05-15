package auth

import "testing"

func TestValidateToken(t *testing.T) {
	cases := []struct {
		name     string
		supplied string
		secret   string
		want     bool
	}{
		{"match", "abc", "abc", true},
		{"mismatch", "abc", "def", false},
		{"empty supplied", "", "abc", false},
		{"empty secret", "abc", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateToken(tc.supplied, tc.secret); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
