package permission

import (
	"encoding/json"
	"testing"
)

func TestMarkerRoundTrip(t *testing.T) {
	m := Marker("Bash", map[string]any{"command": "go build"}, "Bash(go test:*)")
	// Survive a JSON round-trip (interaction record / checkpoint persistence).
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	tool, input, rule, ok := ParseMarker(back)
	if !ok || tool != "Bash" || rule != "Bash(go test:*)" || input["command"] != "go build" {
		t.Fatalf("ParseMarker = (%q, %v, %q, %v)", tool, input, rule, ok)
	}
}

func TestParseMarker_NonMarker(t *testing.T) {
	if _, _, _, ok := ParseMarker("just a string"); ok {
		t.Error("string is not a marker")
	}
	if _, _, _, ok := ParseMarker(map[string]any{"no": "tool"}); ok {
		t.Error("map without tool is not a marker")
	}
}

func TestGrantFromAnswer(t *testing.T) {
	in := map[string]any{"command": "go build ./..."}
	// approve once → argument-scoped grant that authorizes the same call
	rule, approved := GrantFromAnswer("allow", "Bash", in)
	if !approved {
		t.Fatal("allow should approve")
	}
	p := mustPolicy(t, ModeAsk, []string{rule}, nil, nil)
	if dec, _ := p.Evaluate("Bash", in); dec != Allow {
		t.Errorf("granted call = %v, want Allow", dec)
	}
	// approve always → whole-tool grant
	rule2, _ := GrantFromAnswer("allow always", "Bash", in)
	if rule2 != "Bash" {
		t.Errorf("always grant = %q, want Bash", rule2)
	}
	// deny → no grant
	if _, approved := GrantFromAnswer("deny", "Bash", in); approved {
		t.Error("deny should not approve")
	}
}
