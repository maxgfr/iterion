package ir

import "regexp"

// Capability diagnostics.
const (
	DiagUnknownCapability   DiagCode = "C080" // unknown capability name (warning, registry is open)
	DiagMalformedCapability DiagCode = "C081" // capability name does not match the required shape
	DiagBoardCapInSandbox   DiagCode = "C082" // board.* capability requested while sandboxed (HTTP transport needed)
)

// KnownCapabilities is the set of capabilities the iterion runtime understands
// at compile time. The validator emits a C080 warning for any cap declared on
// an agent or judge that is not in this set — but does not reject it, so
// future capabilities can be registered out-of-tree without DSL changes.
//
// Capability naming convention: lowercase `domain` or `domain.action`,
// e.g. `board.create`, `board.read`. Enforced by C081.
var KnownCapabilities = map[string]bool{
	"board.read":   true,
	"board.create": true,
	"board.move":   true,
	"board.assign": true,
	"board.label":  true,
	"board.close":  true,
}

// capShapeRe enforces the lowercase `domain` or `domain.action` shape.
var capShapeRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)?$`)

// validateCapabilities walks every agent/judge node in the workflow and
// validates the shape and known-ness of each capability. Workflow-level
// capabilities are validated through inheritance: when a node has no
// explicit list, it inherits the workflow's, so workflow-level shape errors
// are surfaced via every inheriting node — that's intentional: an author
// who explicitly grants a malformed cap workflow-wide should see one error
// per affected node.
func (c *compiler) validateCapabilities(w *Workflow) {
	// Workflow-level: emit one diagnostic per malformed entry, attributed
	// to no specific node.
	for _, cap := range w.Capabilities {
		c.validateOneCapability(cap, "", "workflow")
	}
	for _, n := range w.Nodes {
		var caps []string
		var kind string
		switch v := n.(type) {
		case *AgentNode:
			caps = v.Capabilities
			kind = "agent"
		case *JudgeNode:
			caps = v.Capabilities
			kind = "judge"
		default:
			continue
		}
		for _, cap := range caps {
			c.validateOneCapability(cap, n.NodeID(), kind)
		}
	}
}

func (c *compiler) validateOneCapability(cap, nodeID, scope string) {
	if cap == "" {
		return
	}
	if !capShapeRe.MatchString(cap) {
		c.errorfAt(DiagMalformedCapability, nodeID, "",
			"%s capability %q must match shape 'domain' or 'domain.action' (lowercase letters, digits, underscores)",
			scope, cap)
		return
	}
	if !KnownCapabilities[cap] {
		c.warnfAt(DiagUnknownCapability, nodeID, "",
			"%s capability %q is not in the built-in registry (this is a warning, not an error — the cap will be passed through to the executor)",
			scope, cap)
	}
}
