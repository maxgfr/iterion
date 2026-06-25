package ir

import (
	"time"
)

// Supervisor diagnostic codes (slot C190–C193, a fresh band above the
// current high-water mark).
const (
	DiagUnknownWatchedNode      DiagCode = "C190" // supervisor watches a node id that isn't an agent node
	DiagMalformedSupervisor     DiagCode = "C191" // supervisor decl is malformed (bad cooldown duration)
	DiagDuplicateSupervisor     DiagCode = "C192" // duplicate supervisor name in workflow
	DiagUnknownSupervisorPrompt DiagCode = "C193" // supervisor system: references an undeclared prompt
)

// Supervisor is the normalized IR form of a `supervisor NAME:`
// declaration. The system prompt is carried as a reference name
// (resolved against Workflow.Prompts at spawn time, like agent system
// prompts); Cooldown is the parsed duration (0 = engine default).
type Supervisor struct {
	Name     string
	Watches  []string
	Model    string
	System   string // prompt reference name
	Cooldown time.Duration
	MaxEvals int
}

// compileSupervisors converts every top-level `supervisor NAME:`
// declaration into a normalized Supervisor. Cross-references (watched
// node ids exist, system prompt declared) are validated separately in
// validateSupervisors, which runs after nodes + prompts are compiled.
func (c *compiler) compileSupervisors() []*Supervisor {
	if len(c.file.Supervisors) == 0 {
		return nil
	}
	out := make([]*Supervisor, 0, len(c.file.Supervisors))
	seen := make(map[string]bool, len(c.file.Supervisors))
	for _, decl := range c.file.Supervisors {
		if seen[decl.Name] {
			c.errorf(DiagDuplicateSupervisor,
				"duplicate supervisor name %q: supervisors must be unique within a file", decl.Name)
			continue
		}
		seen[decl.Name] = true

		sup := &Supervisor{
			Name:     decl.Name,
			Watches:  decl.Watches,
			Model:    decl.Model,
			System:   decl.System,
			MaxEvals: decl.MaxEvals,
		}
		if decl.Cooldown != "" {
			d, err := time.ParseDuration(decl.Cooldown)
			if err != nil {
				c.warnf(DiagMalformedSupervisor,
					"supervisor %q: invalid cooldown %q (want a Go duration like \"30s\") — using the default", decl.Name, decl.Cooldown)
			} else {
				sup.Cooldown = d
			}
		}
		out = append(out, sup)
	}
	return out
}

// validateSupervisors checks the cross-references a supervisor depends
// on, after nodes + prompts are compiled. Both are warnings (not hard
// errors): a supervisor is an enhancement, and a misconfigured one
// should degrade rather than block the whole workflow from compiling.
func (c *compiler) validateSupervisors(w *Workflow) {
	for _, sup := range w.Supervisors {
		for _, nodeID := range sup.Watches {
			n, ok := w.Nodes[nodeID]
			if !ok {
				c.warnf(DiagUnknownWatchedNode,
					"supervisor %q watches %q, which is not a declared node", sup.Name, nodeID)
				continue
			}
			if n.NodeKind() != NodeAgent {
				c.warnf(DiagUnknownWatchedNode,
					"supervisor %q watches %q, which is a %s node — supervisors steer agent nodes", sup.Name, nodeID, n.NodeKind())
			}
		}
		if sup.System != "" {
			if _, ok := w.Prompts[sup.System]; !ok {
				c.warnf(DiagUnknownSupervisorPrompt,
					"supervisor %q references system prompt %q, which is not declared", sup.Name, sup.System)
			}
		}
	}
}
