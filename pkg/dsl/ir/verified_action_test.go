package ir

import "testing"

// Wrap a tool node in a minimal valid workflow so Compile + validate run.
func vaWorkflow(toolBody string) string {
	return `tool act:
` + toolBody + `

workflow w:
  entry: act
  act -> done
`
}

// A well-formed Verified Action node compiles clean and carries the quad.
func TestVerifiedAction_CompilesAndDefaults(t *testing.T) {
	src := vaWorkflow(`  command: "git commit -am wip"
  goal: "advance HEAD"
  postcondition: "git rev-parse HEAD"
  policy: recover`)
	wf := mustCompile(t, src)
	tn, ok := wf.Nodes["act"].(*ToolNode)
	if !ok {
		t.Fatal("act is not a ToolNode")
	}
	if tn.Postcondition == "" || tn.Goal == "" {
		t.Fatalf("quad not compiled: %+v", tn)
	}
	if tn.Policy != PolicyRecover {
		t.Fatalf("Policy = %q, want recover", tn.Policy)
	}
	// recover with no recovery block → default one self-repair attempt.
	if tn.Recovery == nil || tn.Recovery.MaxRepairAttempts != 1 {
		t.Fatalf("Recovery default = %+v, want MaxRepairAttempts=1", tn.Recovery)
	}
}

// A postcondition without an explicit policy defaults to required.
func TestVerifiedAction_DefaultPolicyRequired(t *testing.T) {
	src := vaWorkflow(`  command: "echo hi"
  postcondition: "true"`)
	wf := mustCompile(t, src)
	tn := wf.Nodes["act"].(*ToolNode)
	if tn.Policy != PolicyRequired {
		t.Fatalf("Policy = %q, want required (default)", tn.Policy)
	}
	if tn.Recovery != nil {
		t.Fatalf("required policy should have no recovery, got %+v", tn.Recovery)
	}
}

// C103 — invalid policy value.
func TestVerifiedAction_C103InvalidPolicy(t *testing.T) {
	src := vaWorkflow(`  command: "echo hi"
  postcondition: "true"
  policy: yolo`)
	r := compileFile(t, src)
	expectDiag(t, r, DiagInvalidPolicy)
}

// C104 — recovery configured without a postcondition.
func TestVerifiedAction_C104RecoveryNoPostcondition(t *testing.T) {
	src := vaWorkflow(`  command: "echo hi"
  policy: recover`)
	r := compileFile(t, src)
	expectDiag(t, r, DiagRecoveryNoPostcond)
}

// C105 — recovery attached to a gate (recipe == postcondition).
func TestVerifiedAction_C105RecoveryOnGate(t *testing.T) {
	src := vaWorkflow(`  command: "check_invariant"
  postcondition: "check_invariant"
  policy: recover`)
	r := compileFile(t, src)
	expectDiag(t, r, DiagRecoveryOnGate)
}

// C106 — recovery bounds present but policy is not recover (dead config).
func TestVerifiedAction_C106RecoveryWithoutRecover(t *testing.T) {
	src := vaWorkflow(`  command: "echo hi"
  postcondition: "true"
  policy: required
  recovery:
    max_repair_attempts: 2`)
	r := compileFile(t, src)
	expectDiag(t, r, DiagRecoveryWithoutRecov)
}

// A plain tool node (no quad) emits none of the new diagnostics.
func TestVerifiedAction_NoQuadNoDiagnostics(t *testing.T) {
	src := vaWorkflow(`  command: "echo hi"`)
	r := compileFile(t, src)
	expectNoDiag(t, r, DiagInvalidPolicy)
	expectNoDiag(t, r, DiagRecoveryNoPostcond)
	expectNoDiag(t, r, DiagRecoveryOnGate)
	expectNoDiag(t, r, DiagRecoveryWithoutRecov)
}
