package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/dsl/ir"
)

// depHeuristicOut mirrors the heuristic_output schema: {packages, errors}.
type depHeuristicOut struct {
	Packages []depPackage `json:"packages"`
	Errors   []string     `json:"errors"`
}

type depPackage struct {
	Ecosystem      string      `json:"ecosystem"`
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	HeuristicScore int         `json:"heuristic_score"`
	Signals        []depSignal `json:"signals"`
}

// depSignal mirrors the malware-signals.md catalogue shape: {type, evidence,
// weight} with vuln-db-known carrying advisory metadata.
type depSignal struct {
	Type         string   `json:"type"`
	Evidence     string   `json:"evidence"`
	Weight       int      `json:"weight"`
	Advisory     string   `json:"advisory"`
	Aliases      []string `json:"aliases"`
	Severity     string   `json:"severity"`
	Called       bool     `json:"called"`
	FixedVersion string   `json:"fixed_version"`
}

// runDepHeuristic runs the ACTUAL heuristic node command from sec-audit-deps
// against a pre-seeded scan_dir. The scanner step is `command -v`-guarded and
// writes via .tmp+mv, so an absent/failing scanner on the test host never
// clobbers the seeded fixture — the parser reads exactly what we wrote.
func runDepHeuristic(t *testing.T, nodeID, scannerFile, fixture string) depHeuristicOut {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not in PATH — skipping sec-audit-deps heuristic test")
	}
	wf := compileFixture(t, "sec-audit-deps/main.bot")
	node, ok := wf.Nodes[nodeID]
	if !ok {
		t.Fatalf("workflow missing %s node", nodeID)
	}
	tool, ok := node.(*ir.ToolNode)
	if !ok {
		t.Fatalf("%s is not a ToolNode (got %T)", nodeID, node)
	}

	scanDir := t.TempDir()
	wsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(scanDir, scannerFile), []byte(fixture), 0o644); err != nil {
		t.Fatalf("seed %s: %v", scannerFile, err)
	}

	cmd := tool.Command
	cmd = strings.ReplaceAll(cmd, "{{vars.scan_dir}}", scanDir)
	cmd = strings.ReplaceAll(cmd, "{{vars.workspace_dir}}", wsDir)
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		t.Fatalf("%s command failed: %v", nodeID, err)
	}
	var res depHeuristicOut
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("%s output not valid heuristic JSON: %v\nout: %s", nodeID, err, out)
	}
	return res
}

func findPkg(pkgs []depPackage, name string) *depPackage {
	for i := range pkgs {
		if pkgs[i].Name == name {
			return &pkgs[i]
		}
	}
	return nil
}

// TestSecAuditDeps_GoHeuristic_ParsesGovulncheck is the regression test for the
// govulncheck-parsing half of native:3a81df64 — the node must turn the real
// govulncheck -json stream into known-vuln signals (called=high, import-only=
// medium) instead of the old `stub: implementation pending` discard.
func TestSecAuditDeps_GoHeuristic_ParsesGovulncheck(t *testing.T) {
	// A realistic govulncheck -json stream: config + progress noise, then a
	// CALLED vuln (trace has a function frame) and an IMPORT-ONLY vuln (no
	// function frame). Concatenated objects, as govulncheck emits them.
	fixture := `{"config": {"scanner_name": "govulncheck"}}
{"progress": {"message": "Scanning your code and 50 packages..."}}
{"osv": {"id": "GO-2024-1111", "summary": "DoS in example/foo Parse", "aliases": ["CVE-2024-1111"], "affected": [{"package": {"name": "github.com/example/foo", "ecosystem": "Go"}}]}}
{"finding": {"osv": "GO-2024-1111", "fixed_version": "1.2.3", "trace": [{"module": "github.com/example/foo", "version": "1.0.0", "package": "github.com/example/foo", "function": "Parse"}]}}
{"osv": {"id": "GO-2024-2222", "summary": "Import-only flaw in example/bar", "aliases": ["CVE-2024-2222"], "affected": [{"package": {"name": "github.com/example/bar", "ecosystem": "Go"}}]}}
{"finding": {"osv": "GO-2024-2222", "fixed_version": "2.0.0", "trace": [{"module": "github.com/example/bar", "version": "1.5.0", "package": "github.com/example/bar"}]}}`

	res := runDepHeuristic(t, "run_go_heuristics", "govulncheck.json", fixture)
	if len(res.Packages) != 2 {
		t.Fatalf("expected 2 vulnerable packages, got %d: %+v", len(res.Packages), res.Packages)
	}

	foo := findPkg(res.Packages, "github.com/example/foo")
	if foo == nil {
		t.Fatal("missing github.com/example/foo")
	}
	if foo.Version != "1.0.0" || foo.Ecosystem != "gomod" {
		t.Errorf("foo = %+v, want version 1.0.0 ecosystem gomod", foo)
	}
	if len(foo.Signals) != 1 {
		t.Fatalf("foo signals = %d, want 1", len(foo.Signals))
	}
	s := foo.Signals[0]
	if s.Type != "vuln-db-known" || s.Advisory != "GO-2024-1111" || !s.Called || s.Severity != "high" {
		t.Errorf("foo signal = %+v, want vuln-db-known GO-2024-1111 called=true severity=high", s)
	}
	if s.FixedVersion != "1.2.3" {
		t.Errorf("foo fixed_version = %q, want 1.2.3", s.FixedVersion)
	}
	if len(s.Aliases) == 0 || s.Aliases[0] != "CVE-2024-1111" {
		t.Errorf("foo aliases = %v, want [CVE-2024-1111]", s.Aliases)
	}
	if foo.HeuristicScore < 20 || s.Weight < 20 {
		t.Errorf("foo (called) heuristic_score=%d weight=%d, want >= 20", foo.HeuristicScore, s.Weight)
	}
	if !strings.Contains(s.Evidence, "GO-2024-1111") {
		t.Errorf("foo evidence should cite the advisory: %q", s.Evidence)
	}

	bar := findPkg(res.Packages, "github.com/example/bar")
	if bar == nil {
		t.Fatal("missing github.com/example/bar")
	}
	bs := bar.Signals[0]
	if bs.Called || bs.Severity != "medium" {
		t.Errorf("bar signal = %+v, want called=false severity=medium (import-only)", bs)
	}
	if bar.HeuristicScore >= foo.HeuristicScore {
		t.Errorf("import-only bar score (%d) should be below called foo score (%d)", bar.HeuristicScore, foo.HeuristicScore)
	}
}

// TestSecAuditDeps_PyHeuristic_ParsesPipAudit covers the pip-audit half: the
// {dependencies:[{name,version,vulns:[...]}]} shape, skipping deps with empty
// vulns, extracting id / fix_versions / aliases.
func TestSecAuditDeps_PyHeuristic_ParsesPipAudit(t *testing.T) {
	fixture := `{"dependencies": [
  {"name": "flask", "version": "0.5", "vulns": [
    {"id": "PYSEC-2019-179", "fix_versions": ["1.0"], "aliases": ["CVE-2019-1010083"], "description": "Flask before 1.0 DoS via crafted encoded JSON."}
  ]},
  {"name": "jinja2", "version": "3.0.2", "vulns": []},
  {"name": "pip", "version": "21.3.1", "vulns": []}
], "fixes": []}`

	res := runDepHeuristic(t, "run_py_heuristics", "pip-audit.json", fixture)
	if len(res.Packages) != 1 {
		t.Fatalf("expected 1 vulnerable package (flask; clean deps skipped), got %d: %+v", len(res.Packages), res.Packages)
	}
	flask := res.Packages[0]
	if flask.Name != "flask" || flask.Version != "0.5" || flask.Ecosystem != "pypi" {
		t.Errorf("pkg = %+v, want flask@0.5 pypi", flask)
	}
	s := flask.Signals[0]
	if s.Type != "vuln-db-known" || s.Advisory != "PYSEC-2019-179" || s.FixedVersion != "1.0" {
		t.Errorf("signal = %+v, want vuln-db-known PYSEC-2019-179 fixed 1.0", s)
	}
	if len(s.Aliases) == 0 || s.Aliases[0] != "CVE-2019-1010083" {
		t.Errorf("aliases = %v, want [CVE-2019-1010083]", s.Aliases)
	}
}

// TestSecAuditDeps_JsHeuristic_ParsesNpmAudit covers the npm audit half: the
// v7+ vulnerabilities{} map with severity + via advisory details, and the
// legacy v6 advisories{} fallback.
func TestSecAuditDeps_JsHeuristic_ParsesNpmAudit(t *testing.T) {
	t.Run("v7_vulnerabilities_map", func(t *testing.T) {
		fixture := `{"auditReportVersion": 2, "vulnerabilities": {
  "lodash": {"name": "lodash", "severity": "high", "isDirect": true,
    "via": [{"source": 1065, "name": "lodash", "title": "Prototype Pollution in lodash", "url": "https://github.com/advisories/GHSA-jf85", "severity": "high"}],
    "range": "<4.17.19", "fixAvailable": true}
}, "metadata": {"vulnerabilities": {"total": 1}}}`
		res := runDepHeuristic(t, "run_js_heuristics", "npm-audit.json", fixture)
		if len(res.Packages) != 1 {
			t.Fatalf("expected 1 vulnerable package, got %d: %+v", len(res.Packages), res.Packages)
		}
		s := res.Packages[0].Signals[0]
		if res.Packages[0].Name != "lodash" || s.Severity != "high" || res.Packages[0].HeuristicScore != 25 {
			t.Errorf("pkg/signal = %+v / %+v, want lodash severity high heuristic_score 25", res.Packages[0], s)
		}
		if s.Type != "vuln-db-known" || s.Advisory != "npm-advisory-1065" || !strings.Contains(s.Evidence, "Prototype Pollution") {
			t.Errorf("signal = %+v, want vuln-db-known npm-advisory-1065 + title in evidence", s)
		}
	})

	t.Run("v6_advisories_fallback", func(t *testing.T) {
		fixture := `{"advisories": {"1065": {"module_name": "lodash", "severity": "high", "title": "Prototype Pollution", "url": "https://npmjs.com/advisories/1065", "patched_versions": ">=4.17.19"}}}`
		res := runDepHeuristic(t, "run_js_heuristics", "npm-audit.json", fixture)
		if len(res.Packages) != 1 || res.Packages[0].Name != "lodash" {
			t.Fatalf("v6 fallback: expected lodash, got %+v", res.Packages)
		}
		if res.Packages[0].Signals[0].Severity != "high" {
			t.Errorf("v6 severity = %q, want high", res.Packages[0].Signals[0].Severity)
		}
	})
}
