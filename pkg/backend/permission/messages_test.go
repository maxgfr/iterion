package permission

import "testing"

func TestParseAnswer(t *testing.T) {
	cases := []struct {
		in            string
		allow, always bool
	}{
		{"allow", true, false},
		{"Allow", true, false},
		{"yes", true, false},
		{"once", true, false},
		{"allow always", true, true},
		{"always", true, true},
		{"ALLOW ALWAYS", true, true},
		{"deny", false, false},
		{"no", false, false},
		{"", false, false},
		{"gibberish", false, false}, // fail-safe: unrecognized = deny
	}
	for _, c := range cases {
		allow, always := ParseAnswer(c.in)
		if allow != c.allow || always != c.always {
			t.Errorf("ParseAnswer(%q) = (%v,%v), want (%v,%v)", c.in, allow, always, c.allow, c.always)
		}
	}
}

func TestGrantRuleFor(t *testing.T) {
	// always → bare tool name (whole-tool grant)
	if got := GrantRuleFor("Bash", map[string]any{"command": "go build"}, true); got != "Bash" {
		t.Errorf("always grant = %q, want Bash", got)
	}
	// once → scoped to the argument so only the identical retry passes
	got := GrantRuleFor("Bash", map[string]any{"command": "go build ./..."}, false)
	if got != "Bash(go build ./...)" {
		t.Errorf("once grant = %q, want Bash(go build ./...)", got)
	}
	// A once-grant must actually authorize the same call when re-evaluated.
	p := mustPolicy(t, ModeAsk, []string{got}, nil, nil)
	if dec, _ := p.Evaluate("Bash", map[string]any{"command": "go build ./..."}); dec != Allow {
		t.Errorf("granted call = %v, want Allow", dec)
	}
}

func TestDenyMessageMentionsTool(t *testing.T) {
	msg := DenyMessage("Bash", map[string]any{"command": "rm -rf /"}, "Bash(rm -rf:*)")
	for _, want := range []string{"Bash", "denied", "rm -rf"} {
		if !contains(msg, want) {
			t.Errorf("DenyMessage missing %q: %s", want, msg)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
