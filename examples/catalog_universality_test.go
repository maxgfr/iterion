package examples

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Catalog bots (the *.bot bundles `iterion bots list` discovers under
// examples/) are general-purpose tools that must run on ANY target
// repository, in any language, with no knowledge of iterion's own
// layout baked in. See "Catalog bots are repo-agnostic" in CLAUDE.md.
//
// This test greps every catalog bot's typed var-default string
// literals for iterion *target-repo* facts. It guards the most
// common overfit — a bot author staring at the iterion tree
// hardcoding `cmd/iterion/*.go` / `.iterion/...` into a default that
// should be language/layout-agnostic. It deliberately inspects ONLY
// `name: type = "literal"` var defaults: comments may legitimately
// mention `cmd/iterion` as an example, and runtime references
// (mcp__iterion_board__*, `iterion report`, `.iter` DSL) are fine —
// the bot is written FOR iterion, it must not be scoped TO iterion.

// varDefaultRe matches a typed var default with a string literal:
//
//	doc_globs: string = "README.md,docs/**/*.md"
//
// Capture group 1 is the var name, group 2 the default value.
var varDefaultRe = regexp.MustCompile(
	`^\s+([A-Za-z_][A-Za-z0-9_]*):\s+(?:string|json)\s*=\s*"([^"]*)"`,
)

// violationPatterns are substrings that, when they appear in a var
// DEFAULT, scope the bot to iterion's own tree. Each one breaks a
// run against someone else's repo (the glob matches nothing, or the
// path scatters `.iterion/` into their working tree).
var violationPatterns = []string{
	"cmd/iterion",       // iterion's CLI package layout
	"pkg/dsl",           // iterion's DSL internals
	"pkg/**",            // iterion's Go pkg layout
	"cmd/**",            // iterion's Go cmd layout
	"examples/*/skills", // iterion's bundle-doc layout
	".iterion/",         // writing iterion's store dir into the target repo
}

// allowlist records (bot dir, var name, pattern) triples that are
// known iterion-specific defaults still pending a universal rewrite.
// Keep this list SHRINKING — every entry is debt. A new violation on
// a bot NOT in this list fails the test.
type allowEntry struct {
	bot, varName, pattern, reason string
}

var allowlist = []allowEntry{
	// sec-audit-* write scan scratch + caches under .iterion/ in the
	// target repo. Softer than a code-layout glob (it's gitignored
	// scratch, not a scanner that does iterion-specific work), but
	// still scatters .iterion/ into someone else's tree. Tracked for
	// relocation to a neutral scratch dir.
	{"sec-audit-deps", "scan_dir", ".iterion/", "scratch dir; pending relocation to neutral path"},
	{"sec-audit-deps", "cache_dir", ".iterion/", "host cache; pending relocation"},
	{"sec-audit-deps", "cache_path", ".iterion/", "host cache; pending relocation"},
	{"sec-audit-source", "scan_dir", ".iterion/", "scratch dir; pending relocation"},
	{"sec-audit-source", "fp_path", ".iterion/", "scratch dir; pending relocation"},
	{"sec-audit-source", "records_dir", ".iterion/", "scratch dir; pending relocation"},
	{"sec-audit-source", "matchers_dir", ".iterion/", "scratch dir; pending relocation"},
}

func isAllowed(bot, varName, pattern string) bool {
	for _, a := range allowlist {
		if a.bot == bot && a.varName == varName && a.pattern == pattern {
			return true
		}
	}
	return false
}

func TestCatalogBotsAreRepoAgnostic(t *testing.T) {
	bots, err := filepath.Glob("*/main.bot")
	if err != nil {
		t.Fatalf("glob catalog bots: %v", err)
	}
	if len(bots) == 0 {
		t.Fatal("no catalog bots found under examples/*/main.bot")
	}

	for _, botPath := range bots {
		bot := filepath.Base(filepath.Dir(botPath))
		data, err := os.ReadFile(botPath)
		if err != nil {
			t.Errorf("%s: read: %v", botPath, err)
			continue
		}
		inVars := false
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			// Track the top-level `vars:` block: it opens on a line
			// that is exactly `vars:` and closes at the next
			// top-level (column-0) declaration.
			if trimmed == "vars:" && !strings.HasPrefix(line, " ") {
				inVars = true
				continue
			}
			if inVars && line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inVars = false
			}
			if !inVars {
				continue
			}
			m := varDefaultRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			varName, value := m[1], m[2]
			for _, pat := range violationPatterns {
				if !strings.Contains(value, pat) {
					continue
				}
				if isAllowed(bot, varName, pat) {
					continue
				}
				t.Errorf(
					"%s: var %q default %q hardcodes iterion target-repo pattern %q — catalog bots must be repo-agnostic (see CLAUDE.md \"Catalog bots are repo-agnostic\"). Default to a language/layout-agnostic value and make the iterion-specific scope a per-run --var override.",
					botPath, varName, value, pat,
				)
			}
		}
	}
}
