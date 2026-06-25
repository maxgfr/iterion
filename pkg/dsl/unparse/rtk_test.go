package unparse_test

import (
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	"github.com/SocialGouv/iterion/pkg/dsl/parser"
	"github.com/SocialGouv/iterion/pkg/dsl/unparse"
)

// TestRTKRoundTrip exercises parse → unparse → re-parse → re-compile
// on a workflow that uses rtk at every supported site. Unparse must
// emit each value as a bareword (no quotes, like worktree), and the
// re-compiled IR must preserve every RTK verbatim.
func TestRTKRoundTrip(t *testing.T) {
	src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty
  rtk: ultra

judge gate:
  model: "test-model"
  output: empty
  rtk: off

tool ship:
  command: "true"
  output: empty
  rtk: on

workflow minimal:
  entry: start
  rtk: on
  start -> gate
  gate -> ship
  ship -> done
`
	pr1 := parser.Parse("rtk.bot", src)
	for _, d := range pr1.Diagnostics {
		if d.Severity == parser.SeverityError {
			t.Fatalf("original parse error: %s", d.Error())
		}
	}
	unparsed := unparse.Unparse(pr1.File)
	// Every site must emit `rtk: <value>` as a bareword.
	for _, want := range []string{"rtk: on", "rtk: ultra", "rtk: off"} {
		if !strings.Contains(unparsed, want) {
			t.Fatalf("unparse missing %q:\n%s", want, unparsed)
		}
	}

	pr2 := parser.Parse("rtk.bot.roundtrip", unparsed)
	for _, d := range pr2.Diagnostics {
		if d.Severity == parser.SeverityError {
			t.Fatalf("re-parse error: %s\nUnparsed:\n%s", d.Error(), unparsed)
		}
	}
	cr2 := ir.Compile(pr2.File)
	for _, d := range cr2.Diagnostics {
		if d.Severity == ir.SeverityError {
			t.Fatalf("re-compile error: %s\nUnparsed:\n%s", d.Error(), unparsed)
		}
	}
	w := cr2.Workflow
	if w == nil {
		t.Fatal("re-compile returned nil workflow")
	}
	if w.RTK != "on" {
		t.Errorf("roundtrip workflow.RTK = %q, want on", w.RTK)
	}
	if a, ok := w.Nodes["start"].(*ir.AgentNode); !ok || a.RTK != "ultra" {
		t.Errorf("roundtrip start agent.RTK = %q, want ultra", agentRTK(w.Nodes["start"]))
	}
	if j, ok := w.Nodes["gate"].(*ir.JudgeNode); !ok || j.RTK != "off" {
		t.Errorf("roundtrip gate judge.RTK = %q, want off", judgeRTK(w.Nodes["gate"]))
	}
	if tn, ok := w.Nodes["ship"].(*ir.ToolNode); !ok || tn.RTK != "on" {
		t.Errorf("roundtrip ship tool.RTK = %q, want on", toolRTK(w.Nodes["ship"]))
	}
}

func agentRTK(n ir.Node) string {
	if a, ok := n.(*ir.AgentNode); ok {
		return a.RTK
	}
	return "<not-agent>"
}

func judgeRTK(n ir.Node) string {
	if j, ok := n.(*ir.JudgeNode); ok {
		return j.RTK
	}
	return "<not-judge>"
}

func toolRTK(n ir.Node) string {
	if t, ok := n.(*ir.ToolNode); ok {
		return t.RTK
	}
	return "<not-tool>"
}
