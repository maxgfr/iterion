package ir

import "testing"

const reviewGateSrc = `
schema review_verdict:
  decision: string
  confidence: string
  blockers: string[]

prompt companion_sys:
  You are a review companion. Walk the human through testing the change.

agent provision:
  model: "test-model"
  output: review_verdict

agent build:
  model: "test-model"
  output: review_verdict

human ship_review:
  interaction: review
  model: "test-model"
  system: companion_sys
  output: review_verdict
  review_url: "{{outputs.provision.url}}"
  posture: agent_verdict_ok
  merge_strategy: squash
  merge_into: current
  max_turns: 6

workflow shipit:
  entry: provision
  worktree: auto
  provision -> build
  build -> ship_review
  ship_review -> done when "decision == 'approved'"
  ship_review -> build when "decision == 'changes_requested'" as fix_loop(5)
  ship_review -> fail
`

func TestCompileReviewGate(t *testing.T) {
	w := mustCompile(t, reviewGateSrc)

	n, ok := w.Nodes["ship_review"].(*HumanNode)
	if !ok {
		t.Fatalf("ship_review is not a *HumanNode: %T", w.Nodes["ship_review"])
	}
	if n.Interaction != InteractionReview {
		t.Errorf("Interaction = %v, want review", n.Interaction)
	}
	if n.Model != "test-model" {
		t.Errorf("Model = %q, want test-model", n.Model)
	}
	if n.SystemPrompt != "companion_sys" {
		t.Errorf("SystemPrompt = %q, want companion_sys", n.SystemPrompt)
	}
	if n.Posture != PostureAgentVerdictOK {
		t.Errorf("Posture = %q, want %q", n.Posture, PostureAgentVerdictOK)
	}
	if n.MergeStrategy != "squash" {
		t.Errorf("MergeStrategy = %q, want squash", n.MergeStrategy)
	}
	if n.MergeInto != "current" {
		t.Errorf("MergeInto = %q, want current", n.MergeInto)
	}
	if n.MaxTurns != 6 {
		t.Errorf("MaxTurns = %d, want 6", n.MaxTurns)
	}
	if n.ReviewURL != "{{outputs.provision.url}}" {
		t.Errorf("ReviewURL = %q", n.ReviewURL)
	}
	if len(n.ReviewURLRefs) != 1 || n.ReviewURLRefs[0].Kind != RefOutputs {
		t.Errorf("ReviewURLRefs not parsed: %+v", n.ReviewURLRefs)
	}
}

func TestCompileReviewGateDefaults(t *testing.T) {
	// A review gate that omits posture/strategy/into/max_turns gets defaults.
	src := `
schema v:
  decision: string

human gate:
  interaction: review
  model: "test-model"
  output: v

workflow wf:
  entry: gate
  worktree: auto
  gate -> done when "decision == 'approved'"
  gate -> fail
`
	w := mustCompile(t, src)
	n := w.Nodes["gate"].(*HumanNode)
	if n.Posture != PostureHumanRequired {
		t.Errorf("default Posture = %q, want %q", n.Posture, PostureHumanRequired)
	}
	if n.MergeStrategy != "squash" {
		t.Errorf("default MergeStrategy = %q, want squash", n.MergeStrategy)
	}
	if n.MergeInto != "current" {
		t.Errorf("default MergeInto = %q, want current", n.MergeInto)
	}
	if n.MaxTurns != DefaultReviewMaxTurns {
		t.Errorf("default MaxTurns = %d, want %d", n.MaxTurns, DefaultReviewMaxTurns)
	}
}

// C100: a review gate without worktree: auto is an error — nothing to merge.
// `worktree: none` is the explicit opt-out; without it the IR now defaults
// to "auto" (workspace isolation is the default), so the explicit "none"
// is required to exercise C100.
func TestReviewGateRequiresWorktree(t *testing.T) {
	src := `
schema v:
  decision: string

human gate:
  interaction: review
  model: "test-model"
  output: v

workflow wf:
  entry: gate
  worktree: none
  gate -> done when "decision == 'approved'"
`
	r := compileFile(t, src)
	expectDiag(t, r, DiagReviewNeedsWorktree)
}

func TestReviewGateWithWorktreeNoC100(t *testing.T) {
	r := compileFile(t, reviewGateSrc)
	expectNoDiag(t, r, DiagReviewNeedsWorktree)
}

// C101: review_url referencing an unknown node is a warning.
func TestReviewGateUnknownURLRef(t *testing.T) {
	src := `
schema v:
  decision: string

human gate:
  interaction: review
  model: "test-model"
  output: v
  review_url: "{{outputs.nonexistent.url}}"

workflow wf:
  entry: gate
  worktree: auto
  gate -> done when "decision == 'approved'"
`
	r := compileFile(t, src)
	expectDiag(t, r, DiagReviewURLUnknownRef)
}

// A review gate must declare a companion model + output schema (shared with
// the llm / llm_or_human requirement).
func TestReviewGateRequiresModelAndOutput(t *testing.T) {
	src := `
human gate:
  interaction: review

workflow wf:
  entry: gate
  worktree: auto
  gate -> done
`
	r := compileFile(t, src)
	expectDiag(t, r, DiagMissingModelOrBackend)
}
