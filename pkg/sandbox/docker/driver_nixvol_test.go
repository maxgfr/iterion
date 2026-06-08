package docker

import "testing"

func TestNixVolumeNameFromID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"full sha256 ref", "sha256:9cbe935ad1f92fee064e3e573d4772f1d66aeef35cb9eaae00d28e06662bc439", "iterion-nix-9cbe935ad1f9"},
		{"bare id", "9cbe935ad1f92fee", "iterion-nix-9cbe935ad1f9"},
		{"whitespace trimmed", "  sha256:abcdef012345deadbeef  ", "iterion-nix-abcdef012345"},
		{"too short -> empty (skip volume)", "short", ""},
		{"empty -> empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nixVolumeNameFromID(c.in); got != c.want {
				t.Fatalf("nixVolumeNameFromID(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
