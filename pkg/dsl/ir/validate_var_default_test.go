package ir

import (
	"fmt"
	"testing"
)

// varDefaultSrc builds a minimal workflow whose vars: block is the given line.
func varDefaultSrc(varsLine string) string {
	return fmt.Sprintf(`
vars:
%s

prompt sys:
  hi

agent a:
  backend: "claw"
  model: "anthropic/claude-sonnet-4-6"
  system: sys

workflow w:
  entry: a
  a -> done
`, varsLine)
}

// TestC109_VarDefaultTypeMismatch verifies a var default whose literal type
// doesn't match the declared type is flagged (scalar types only; json and
// string[] accept loose literals and are never flagged).
func TestC109_VarDefaultTypeMismatch(t *testing.T) {
	cases := []struct {
		varsLine string
		want     int
	}{
		{`  count: int = "x"`, 1},     // string default on int var
		{`  count: int = 5`, 0},       // int matches
		{`  ratio: float = 5`, 0},     // int→float widening is allowed
		{`  ratio: float = 1.5`, 0},   // float matches
		{`  name: string = "x"`, 0},   // string matches
		{`  flag: bool = true`, 0},    // bool matches
		{`  flag: bool = "true"`, 1},  // string default on bool var
		{`  blob: json = 5`, 0},       // json accepts anything → not flagged
		{`  tags: string[] = "x"`, 0}, // string[] accepts a string literal
	}
	for _, tc := range cases {
		t.Run(tc.varsLine, func(t *testing.T) {
			r := compileFile(t, varDefaultSrc(tc.varsLine))
			if got := countCode(r, DiagVarDefaultTypeMismatch); got != tc.want {
				t.Errorf("C109 count = %d, want %d\ndiagnostics: %v", got, tc.want, r.Diagnostics)
			}
		})
	}
}

// TestC109_IsError pins the severity (consistent with C071 preset mismatch).
func TestC109_IsError(t *testing.T) {
	r := compileFile(t, varDefaultSrc(`  count: int = "x"`))
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == DiagVarDefaultTypeMismatch {
			found = true
			if d.Severity != SeverityError {
				t.Errorf("C109 severity = %s, want error", d.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("expected a C109 diagnostic, got %v", r.Diagnostics)
	}
}
