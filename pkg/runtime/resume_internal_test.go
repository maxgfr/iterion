package runtime

import (
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	"github.com/SocialGouv/iterion/pkg/store"
)

// resume.go (1000+ LOC) has end-to-end coverage via the engine_test.go
// resume scenarios, but several pure helpers carry significant
// reasoning (hash gating, loop-iteration unwinding, nested-loop path
// encoding) and had zero direct exercises. Tests below pin those
// helpers so a refactor can't silently shift the resume contract.

// ---- checkWorkflowHash ----

func TestCheckWorkflowHash_BothEmptyAllowsResume(t *testing.T) {
	e := &Engine{} // workflowHash empty
	r := &store.Run{ID: "r1"}
	if err := e.checkWorkflowHash(r); err != nil {
		t.Fatalf("expected nil when both hashes empty, got %v", err)
	}
}

func TestCheckWorkflowHash_MatchingHashesAllowResume(t *testing.T) {
	e := &Engine{workflowHash: "abc123def456"}
	r := &store.Run{ID: "r1", WorkflowHash: "abc123def456"}
	if err := e.checkWorkflowHash(r); err != nil {
		t.Fatalf("expected nil for matching hashes, got %v", err)
	}
}

func TestCheckWorkflowHash_MismatchReturnsError(t *testing.T) {
	e := &Engine{workflowHash: "abc123def456"}
	r := &store.Run{ID: "r1", WorkflowHash: "deadbeefcafe"}
	err := e.checkWorkflowHash(r)
	if err == nil {
		t.Fatal("expected error for hash mismatch")
	}
	if !strings.Contains(err.Error(), "workflow source has changed") {
		t.Errorf("error wording changed: %v", err)
	}
	// Hashes must be truncated to 12 chars for readability.
	if !strings.Contains(err.Error(), "abc123def456") || !strings.Contains(err.Error(), "deadbeefcafe") {
		t.Errorf("error should include both short hashes: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should hint at --force: %v", err)
	}
}

func TestCheckWorkflowHash_ForceAllowsMismatch(t *testing.T) {
	e := &Engine{workflowHash: "abc123def456", forceResume: true}
	r := &store.Run{ID: "r1", WorkflowHash: "deadbeefcafe"}
	if err := e.checkWorkflowHash(r); err != nil {
		t.Fatalf("expected --force to bypass hash check, got %v", err)
	}
}

// ---- rebuildArtifacts ----

func TestRebuildArtifacts_OnlyPublishedNodesAppear(t *testing.T) {
	e := &Engine{
		workflow: &ir.Workflow{
			Nodes: map[string]ir.Node{
				"a": &ir.AgentNode{BaseNode: ir.BaseNode{ID: "a"}, Publish: "first"},
				"b": &ir.AgentNode{BaseNode: ir.BaseNode{ID: "b"}}, // no publish
				"c": &ir.JudgeNode{BaseNode: ir.BaseNode{ID: "c"}, Publish: "third"},
			},
		},
	}
	outputs := map[string]map[string]interface{}{
		"a": {"k": "va"},
		"b": {"k": "vb"},
		"c": {"k": "vc"},
	}
	got := e.rebuildArtifacts(outputs)
	if len(got) != 2 {
		t.Fatalf("expected 2 artifacts, got %d: %v", len(got), got)
	}
	if got["first"]["k"] != "va" {
		t.Errorf("first should map to a's output, got %v", got["first"])
	}
	if got["third"]["k"] != "vc" {
		t.Errorf("third should map to c's output, got %v", got["third"])
	}
	if _, ok := got["b"]; ok {
		t.Errorf("non-publishing node should not appear: %v", got)
	}
}

func TestRebuildArtifacts_EmptyInput(t *testing.T) {
	e := &Engine{workflow: &ir.Workflow{Nodes: map[string]ir.Node{}}}
	got := e.rebuildArtifacts(nil)
	if got == nil {
		t.Fatal("rebuildArtifacts should always return a non-nil map")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// ---- interactionFields ----

func TestInteractionFields_AgentNode(t *testing.T) {
	a := &ir.AgentNode{
		InteractionFields: ir.InteractionFields{Interaction: ir.InteractionLLM, InteractionPrompt: "p", InteractionModel: "m"},
	}
	got := interactionFields(a)
	if got.Interaction != ir.InteractionLLM || got.InteractionPrompt != "p" || got.InteractionModel != "m" {
		t.Errorf("got %+v", got)
	}
}

func TestInteractionFields_JudgeNode(t *testing.T) {
	j := &ir.JudgeNode{InteractionFields: ir.InteractionFields{Interaction: ir.InteractionHuman}}
	if got := interactionFields(j); got.Interaction != ir.InteractionHuman {
		t.Errorf("got %+v", got)
	}
}

func TestInteractionFields_HumanNode(t *testing.T) {
	h := &ir.HumanNode{InteractionFields: ir.InteractionFields{Interaction: ir.InteractionLLMOrHuman}}
	if got := interactionFields(h); got.Interaction != ir.InteractionLLMOrHuman {
		t.Errorf("got %+v", got)
	}
}

func TestInteractionFields_OtherNodeYieldsZeroValue(t *testing.T) {
	// Tool / compute / done / fail / router don't carry InteractionFields.
	for _, n := range []ir.Node{
		&ir.ToolNode{},
		&ir.ComputeNode{},
		&ir.DoneNode{},
		&ir.FailNode{},
		&ir.RouterNode{},
	} {
		got := interactionFields(n)
		if (got != ir.InteractionFields{}) {
			t.Errorf("%T: expected zero-value, got %+v", n, got)
		}
	}
}

// ---- currentLoopIteration + currentLoopIterationPath ----

// buildLoopFixtureEngine returns an Engine whose workflow declares two
// loops (outer + inner). `inner` is nested inside `outer`.
func buildLoopFixtureEngine() *Engine {
	return &Engine{
		workflow: &ir.Workflow{
			Loops: map[string]*ir.Loop{
				"outer": {
					Name: "outer",
					Body: map[string]bool{"a": true, "b": true, "c": true},
				},
				"inner": {
					Name: "inner",
					Body: map[string]bool{"b": true, "c": true},
				},
			},
		},
	}
}

func TestCurrentLoopIteration_NodeOutsideAllLoops(t *testing.T) {
	e := buildLoopFixtureEngine()
	got := e.currentLoopIteration("z", map[string]int{"outer": 3, "inner": 5})
	if got != 0 {
		t.Errorf("node outside any loop should be 0, got %d", got)
	}
}

func TestCurrentLoopIteration_NodeInSingleLoop(t *testing.T) {
	e := buildLoopFixtureEngine()
	got := e.currentLoopIteration("a", map[string]int{"outer": 2, "inner": 5})
	if got != 2 {
		t.Errorf("'a' belongs only to outer, expected 2, got %d", got)
	}
}

func TestCurrentLoopIteration_NodeInNestedTakesMax(t *testing.T) {
	e := buildLoopFixtureEngine()
	// 'b' lives in both outer and inner — currentLoopIteration returns max.
	got := e.currentLoopIteration("b", map[string]int{"outer": 7, "inner": 2})
	if got != 7 {
		t.Errorf("'b' max(outer=7, inner=2)=7, got %d", got)
	}
	got = e.currentLoopIteration("b", map[string]int{"outer": 1, "inner": 4})
	if got != 4 {
		t.Errorf("'b' max(outer=1, inner=4)=4, got %d", got)
	}
}

func TestCurrentLoopIterationPath_NodeOutsideAllLoops(t *testing.T) {
	e := buildLoopFixtureEngine()
	got := e.currentLoopIterationPath("z", map[string]int{"outer": 3, "inner": 5})
	if got != "" {
		t.Errorf("node outside loops should yield empty path, got %q", got)
	}
}

func TestCurrentLoopIterationPath_NodeInSingleLoop(t *testing.T) {
	e := buildLoopFixtureEngine()
	got := e.currentLoopIterationPath("a", map[string]int{"outer": 3})
	if got != "outer=3" {
		t.Errorf("got %q", got)
	}
}

func TestCurrentLoopIterationPath_StableLexicographicOrder(t *testing.T) {
	e := buildLoopFixtureEngine()
	got := e.currentLoopIterationPath("b", map[string]int{"outer": 5, "inner": 2})
	// Names are sorted lexicographically: inner < outer.
	if got != "inner=2;outer=5" {
		t.Errorf("got %q, want \"inner=2;outer=5\"", got)
	}
}

func TestCurrentLoopIterationPath_FallbackToEdgeMembership(t *testing.T) {
	// Loop.Body empty (older IRs) — fall back to edge-endpoint membership.
	e := &Engine{
		workflow: &ir.Workflow{
			Loops: map[string]*ir.Loop{
				"L": {Name: "L"}, // Body nil/empty
			},
			Edges: []*ir.Edge{
				{From: "x", To: "y", LoopName: "L"},
			},
		},
	}
	got := e.currentLoopIterationPath("y", map[string]int{"L": 4})
	if got != "L=4" {
		t.Errorf("got %q", got)
	}
	// And currentLoopIteration mirrors that.
	if it := e.currentLoopIteration("y", map[string]int{"L": 4}); it != 4 {
		t.Errorf("currentLoopIteration fallback: got %d, want 4", it)
	}
}
