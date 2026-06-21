package ir

import "testing"

// TestCompileRTK exercises the rtk field end-to-end: parser → AST → IR
// on a workflow that sets rtk at workflow level, on an agent node, on a
// judge node, and on a tool node. Each value uses one of the accepted
// barewords (on / off / ultra) so the C102 validator stays silent.
func TestCompileRTK(t *testing.T) {
	src := `
schema empty:
  ok: bool

prompt sys:
  hi

prompt usr:
  hi

agent start:
  model: "test-model"
  input: empty
  output: empty
  system: sys
  user: usr
  rtk: ultra

judge gate:
  model: "test-model"
  input: empty
  output: empty
  system: sys
  user: usr
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
	w := mustCompile(t, src)

	if w.RTK != "on" {
		t.Errorf("workflow.RTK = %q, want on", w.RTK)
	}
	a, ok := w.Nodes["start"].(*AgentNode)
	if !ok {
		t.Fatalf("start node = %T, want *AgentNode", w.Nodes["start"])
	}
	if a.RTK != "ultra" {
		t.Errorf("agent.RTK = %q, want ultra", a.RTK)
	}
	j, ok := w.Nodes["gate"].(*JudgeNode)
	if !ok {
		t.Fatalf("gate node = %T, want *JudgeNode", w.Nodes["gate"])
	}
	if j.RTK != "off" {
		t.Errorf("judge.RTK = %q, want off", j.RTK)
	}
	tn, ok := w.Nodes["ship"].(*ToolNode)
	if !ok {
		t.Fatalf("ship node = %T, want *ToolNode", w.Nodes["ship"])
	}
	if tn.RTK != "on" {
		t.Errorf("tool.RTK = %q, want on", tn.RTK)
	}
}

// TestValidateRTKInvalid asserts that a typo like `rtk: bogus` raises
// the C102 diagnostic on every site (workflow + agent + judge + tool),
// not just one — a silent fallback to "inherit" would defeat the
// purpose of the field.
func TestValidateRTKInvalid(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "workflow",
			src: `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty

workflow w:
  entry: start
  rtk: bogus
  start -> done
`,
		},
		{
			name: "agent",
			src: `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty
  rtk: bogus

workflow w:
  entry: start
  start -> done
`,
		},
		{
			name: "judge",
			src: `
schema empty:
  ok: bool

judge gate:
  model: "test-model"
  output: empty
  rtk: bogus

workflow w:
  entry: gate
  gate -> done
`,
		},
		{
			name: "tool",
			src: `
schema empty:
  ok: bool

tool ship:
  command: "true"
  output: empty
  rtk: bogus

workflow w:
  entry: ship
  ship -> done
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := compileFile(t, tc.src)
			expectDiag(t, r, DiagInvalidRTK)
		})
	}
}

// TestValidateRTKValidNoDiag confirms that the three accepted barewords
// ("on", "off", "ultra") never trigger C102.
func TestValidateRTKValidNoDiag(t *testing.T) {
	for _, v := range []string{"on", "off", "ultra"} {
		t.Run(v, func(t *testing.T) {
			src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty
  rtk: ` + v + `

workflow w:
  entry: start
  rtk: ` + v + `
  start -> done
`
			r := compileFile(t, src)
			expectNoDiag(t, r, DiagInvalidRTK)
		})
	}
}
