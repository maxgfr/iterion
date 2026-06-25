package ir

import "testing"

// TestCompilePermission exercises the permission gate field end-to-end:
// parser → AST → IR on a workflow that sets the scalar mode + allow/ask/deny
// rule lists at workflow level and a per-node permission mode override on an
// agent, a judge, and a tool node. Each value uses one of the accepted
// barewords (off / ask / deny) so the C110 validator stays silent, and the
// workflow mode is "ask" so the rule lists do not trigger C111.
func TestCompilePermission(t *testing.T) {
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
  permission: deny

judge gate:
  model: "test-model"
  input: empty
  output: empty
  system: sys
  user: usr
  permission: ask

tool ship:
  command: "true"
  output: empty
  permission: off

workflow minimal:
  entry: start
  permission: ask
  allow: ["Read(**)"]
  ask: ["Bash(go build:*)"]
  deny: ["Bash(rm:*)"]
  start -> gate
  gate -> ship
  ship -> done
`
	w := mustCompile(t, src)

	if w.Permission != "ask" {
		t.Errorf("workflow.Permission = %q, want ask", w.Permission)
	}
	if got := w.PermissionAllow; len(got) != 1 || got[0] != "Read(**)" {
		t.Errorf("workflow.PermissionAllow = %v, want [Read(**)]", got)
	}
	if got := w.PermissionAsk; len(got) != 1 || got[0] != "Bash(go build:*)" {
		t.Errorf("workflow.PermissionAsk = %v, want [Bash(go build:*)]", got)
	}
	if got := w.PermissionDeny; len(got) != 1 || got[0] != "Bash(rm:*)" {
		t.Errorf("workflow.PermissionDeny = %v, want [Bash(rm:*)]", got)
	}

	a, ok := w.Nodes["start"].(*AgentNode)
	if !ok {
		t.Fatalf("start node = %T, want *AgentNode", w.Nodes["start"])
	}
	if a.Permission != "deny" {
		t.Errorf("agent.Permission = %q, want deny", a.Permission)
	}
	j, ok := w.Nodes["gate"].(*JudgeNode)
	if !ok {
		t.Fatalf("gate node = %T, want *JudgeNode", w.Nodes["gate"])
	}
	if j.Permission != "ask" {
		t.Errorf("judge.Permission = %q, want ask", j.Permission)
	}
	tn, ok := w.Nodes["ship"].(*ToolNode)
	if !ok {
		t.Fatalf("ship node = %T, want *ToolNode", w.Nodes["ship"])
	}
	if tn.Permission != "off" {
		t.Errorf("tool.Permission = %q, want off", tn.Permission)
	}
}

// TestValidatePermissionInvalid asserts that a typo like `permission: bogus`
// raises the C110 diagnostic on every site (workflow + agent + judge + tool).
func TestValidatePermissionInvalid(t *testing.T) {
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
  permission: bogus
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
  permission: bogus

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
  permission: bogus

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
  permission: bogus

workflow w:
  entry: ship
  ship -> done
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := compileFile(t, tc.src)
			expectDiag(t, r, DiagInvalidPermission)
		})
	}
}

// TestValidatePermissionValidNoDiag confirms that the three accepted barewords
// ("off", "ask", "deny") never trigger C110.
func TestValidatePermissionValidNoDiag(t *testing.T) {
	for _, v := range []string{"off", "ask", "deny"} {
		t.Run(v, func(t *testing.T) {
			src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty
  permission: ` + v + `

workflow w:
  entry: start
  permission: ` + v + `
  start -> done
`
			r := compileFile(t, src)
			expectNoDiag(t, r, DiagInvalidPermission)
		})
	}
}

// TestPermissionRulesWithoutGate verifies C111 fires when allow/ask/deny rules
// are declared but the resolved workflow permission mode is unset or off, and
// stays silent when the gate is enabled (ask/deny).
func TestPermissionRulesWithoutGate(t *testing.T) {
	t.Run("unset_mode_warns", func(t *testing.T) {
		src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty

workflow w:
  entry: start
  deny: ["Bash(rm:*)"]
  start -> done
`
		r := compileFile(t, src)
		expectDiag(t, r, DiagPermissionRulesNoGate)
	})

	t.Run("off_mode_warns", func(t *testing.T) {
		src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty

workflow w:
  entry: start
  permission: off
  allow: ["Read(**)"]
  start -> done
`
		r := compileFile(t, src)
		expectDiag(t, r, DiagPermissionRulesNoGate)
	})

	t.Run("ask_mode_silent", func(t *testing.T) {
		src := `
schema empty:
  ok: bool

agent start:
  model: "test-model"
  output: empty

workflow w:
  entry: start
  permission: ask
  deny: ["Bash(rm:*)"]
  start -> done
`
		r := compileFile(t, src)
		expectNoDiag(t, r, DiagPermissionRulesNoGate)
	})
}
