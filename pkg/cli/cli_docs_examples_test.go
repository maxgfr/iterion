package cli_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestDocsNoStaleIterCLIExamples guards against the `.iter`→`.bot`
// CLI-example drift class (board finding native:0905941b, surfaced by a
// docs-refresh dogfood run). The `.iter` extension is no longer accepted at
// the CLI/server/studio boundaries — runnable workflows are `.bot` (`.botz`
// for bundles); `.iter` survives only as the DSL raw/testdata form. So every
// `iterion <verb> … <name>.iter` invocation in the docs is a copy-paste-broken
// command. The class is mechanically detectable, so fail CI on a regression
// rather than relying on an LLM docs-refresh pass to catch it (docs-refresh's
// scanners don't auto-detect example-argument extension drift — it's neither a
// dead link nor a bad flag).
//
// Prose that *describes* the `.iter` extension as legacy/rejected/raw-testdata
// (explaining the boundary rather than invoking a runnable workflow) is
// intentionally excluded.
var iterCLIInvocationRe = regexp.MustCompile(
	`iterion\s+(?:run|validate|diagram|inspect|resume|report)\b.*\.iter\b`,
)

// intentionalIterMention: lowercase substrings that mark a line as prose about
// the `.iter` boundary (legacy/rejected/raw) rather than a runnable example.
var intentionalIterMention = []string{
	"reject", "unsupported", "no longer", "legacy", "raw", "testdata",
	"not accepted", "expected .bot", "deprecat",
}

func TestDocsNoStaleIterCLIExamples(t *testing.T) {
	root := repoRootForDocsTest(t)

	var violations []string
	scan := func(path string) {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		rel, _ := filepath.Rel(root, path)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		ln := 0
		for sc.Scan() {
			ln++
			line := sc.Text()
			if !iterCLIInvocationRe.MatchString(line) {
				continue
			}
			low := strings.ToLower(line)
			skip := false
			for _, kw := range intentionalIterMention {
				if strings.Contains(low, kw) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			violations = append(violations, rel+":"+strconv.Itoa(ln)+"  "+strings.TrimSpace(line))
		}
	}

	// docs/**/*.{md,sh} + the two root docs.
	_ = filepath.WalkDir(filepath.Join(root, "docs"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// docs/bot-runs/ holds dogfood bilans that legitimately *discuss*
			// the .iter→.bot drift in prose; they are not user-facing CLI docs.
			if d.Name() == "bot-runs" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".sh") {
			scan(p)
		}
		return nil
	})
	scan(filepath.Join(root, "README.md"))
	scan(filepath.Join(root, "CLAUDE.md"))

	if len(violations) > 0 {
		t.Fatalf("stale `.iter` CLI-invocation example(s) in docs — use `.bot` "+
			"(`.iter` is no longer accepted at the CLI):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// repoRootForDocsTest walks up from the test's working directory (the package
// dir) until it finds the go.mod, returning the repo root.
func repoRootForDocsTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root (go.mod) not found from %s", dir)
		}
		dir = parent
	}
}
