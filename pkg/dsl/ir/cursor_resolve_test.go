package ir

import "testing"

// TestResolveCursorValueRejectsNonFinite guards the NaN/Inf guard: a NaN value
// passes the v<0||v>1 range check (all NaN comparisons are false) and would
// otherwise select Values[0] (and on some architectures int(NaN) can be a
// negative index → panic). It must be rejected as a non-finite value.
func TestResolveCursorValueRejectsNonFinite(t *testing.T) {
	def := &CursorDef{
		Name:   "depth",
		Values: []CursorValue{{Name: "low", Prompt: "p-low"}, {Name: "high", Prompt: "p-high"}},
	}
	for _, raw := range []string{"NaN", "Inf", "+Inf", "-Inf"} {
		if _, ok, reason := ResolveCursorValue(def, raw); ok {
			t.Errorf("ResolveCursorValue(%q) ok=true, want rejected (reason=%q)", raw, reason)
		}
	}
	// A valid numeric value still resolves.
	if _, ok, reason := ResolveCursorValue(def, "0.9"); !ok {
		t.Errorf("ResolveCursorValue(0.9) ok=false, want resolved (reason=%q)", reason)
	}
	// A valid enum name still resolves.
	if p, ok, _ := ResolveCursorValue(def, "low"); !ok || p != "p-low" {
		t.Errorf("ResolveCursorValue(low) = (%q,%v), want (p-low,true)", p, ok)
	}
}
