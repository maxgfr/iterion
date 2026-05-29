package botreplay

import "path/filepath"

// Scenario links a golden fixture to the bot/node it captures and the
// invariants the replay test enforces on it. It is the single source of
// truth shared by record mode (which inputs to feed the node) and replay
// mode (which checks to run on the recorded output).
type Scenario struct {
	Bot  string
	Name string
	Node string

	// RequiredNonEmpty names output fields that must be present AND
	// non-empty — the semantic-presence check that schema validation
	// (which accepts an empty json array/object) cannot express.
	RequiredNonEmpty []string

	// CheckAssignees enables the no-hallucinated-assignee scan over the
	// recorded output.
	CheckAssignees bool

	// Record-only fields (ignored by replay). Input is the per-node
	// input map handed to NodeExecutor.Execute.
	Vars  map[string]string
	Input map[string]interface{}
}

// FixturePath returns the testdata path for a scenario's fixture,
// relative to the package directory (where `go test` runs).
func (s Scenario) FixturePath() string {
	return filepath.Join("testdata", "bot-goldens", s.Bot, s.Name+".json")
}

// Scenarios returns the wired golden scenarios for this iteration:
// feature_dev, whats-next (the assignee-bearing bot, two scenarios), and
// doc-align. The claw-backed reviewer/proposer nodes are the cheapest to
// record live; emit_action (claude_code + board MCP + filesystem) is the
// headline created_issues scenario and is the heaviest to re-record.
func Scenarios() []Scenario {
	return []Scenario{
		{
			Bot:            "feature_dev",
			Name:           "reviewer_gpt_approve",
			Node:           "reviewer_gpt",
			CheckAssignees: true, // verdict_output carries no assignees; scan must stay clean
			Vars: map[string]string{
				"feature_prompt": "add Answer() int returning 42 in answer.go",
			},
			Input: map[string]interface{}{
				"prior_pushback":               []interface{}{},
				"prior_pushback_justification": "",
				"previous_scanned_areas":       []interface{}{},
			},
		},
		{
			Bot:            "whats-next",
			Name:           "propose_roadmap_basic",
			Node:           "propose_roadmap",
			CheckAssignees: true, // roadmap_item.assignee must resolve to a real bot or be ""
			Vars: map[string]string{
				"scope_notes": "",
			},
			Input: map[string]interface{}{
				"exploration":     map[string]interface{}{"observations": []interface{}{}},
				"user_priorities": "improve test coverage and developer tooling",
				"workspace_dir":   "",
			},
		},
		{
			Bot:              "whats-next",
			Name:             "emit_action_basic",
			Node:             "emit_action",
			RequiredNonEmpty: []string{"created_issues"},
			CheckAssignees:   true,
			Input: map[string]interface{}{
				"roadmap":         map[string]interface{}{},
				"user_priorities": "improve test coverage and developer tooling",
				"workspace_dir":   "",
				"selected_titles": []interface{}{},
			},
		},
		{
			Bot:            "doc-align",
			Name:           "reviewer_gpt_approve",
			Node:           "reviewer_gpt",
			CheckAssignees: true, // doc-align routes no work to bots; scan must stay clean
			Input: map[string]interface{}{
				"prior_pushback":               []interface{}{},
				"prior_pushback_justification": "",
				"previous_scanned_areas":       []interface{}{},
			},
		},
	}
}
