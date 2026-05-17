package parser_test

import (
	"testing"

	"github.com/SocialGouv/iterion/pkg/dsl/parser"
)

// TestNodeNamedWithKeywordToken guards the parser fix that extended
// isKeywordToken to recognise the dozen-plus tokens that were defined
// as keywords but missing from the name-position whitelist. Each of
// these used to produce "expected identifier, got 'X'" for a node
// whose name collided with a DSL keyword.
func TestNodeNamedWithKeywordToken(t *testing.T) {
	// One agent declaration per previously-rejected name. The body
	// can be minimal; we only care that parsing reaches the name and
	// accepts it without a diagnostic.
	names := []string{
		"fork",
		"script",
		"language",
		"interaction",
		"interaction_prompt",
		"interaction_model",
		"default_backend",
		"multi",
		"llm",
		"round_robin",
		"auth",
		"readonly",
		"presets",
		"join",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			src := "agent " + name + ":\n  model: \"gpt-4\"\n"
			res := parser.Parse("kw.iter", src)
			for _, d := range res.Diagnostics {
				t.Errorf("unexpected diagnostic on `agent %s:` -> %v", name, d)
			}
			if len(res.File.Agents) != 1 {
				t.Fatalf("expected 1 agent, got %d", len(res.File.Agents))
			}
			if got := res.File.Agents[0].Name; got != name {
				t.Errorf("agent name = %q, want %q", got, name)
			}
		})
	}
}

// TestSkipToNextTopLevel_PreservesPresetsAfterError guards that an
// error inside `vars:` no longer silently swallows a following
// `presets:` block. The skip set was missing TokenPresets +
// TokenAttachments before.
func TestSkipToNextTopLevel_PreservesPresetsAfterError(t *testing.T) {
	src := `vars:
  ## intentional syntax error to trigger skipToNextTopLevel
  ~~~

presets:
  light:
    foo: "bar"

workflow trivial:
  entry: done
`
	res := parser.Parse("skip.iter", src)
	if res.File.Presets == nil {
		t.Fatalf("presets block lost after vars error; expected at least one preset to survive")
	}
}
