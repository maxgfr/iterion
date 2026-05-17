package delegate

import "strings"

// boardMCPServerName is the name under which iterion registers its internal
// board MCP server with the claude_code CLI subprocess. The MCP convention
// gives every tool the FQN mcp__<serverName>__<toolName>.
const boardMCPServerName = "iterion_board"

// boardMCPSubcommand is the cobra subcommand exposed by the iterion binary
// when invoked as `iterion __mcp-board`. See cmd/iterion/mcp_board.go.
const boardMCPSubcommand = "__mcp-board"

// boardCapabilityPrefix is the namespace prefix that flags a capability as
// addressing the board. Capabilities outside this prefix are ignored by the
// board wiring (they may be honoured by other future subsystems).
const boardCapabilityPrefix = "board."

// boardToolByName maps each board tool to the capability that gates it. The
// list is the canonical source of truth for the FQN exposed to bots and the
// AllowedTools extension applied to CLI backends.
//
// Keep in sync with pkg/conductor/native/boardops.Tools().
var boardToolByName = []struct {
	Tool string // bare tool name (no FQN prefix)
	Cap  string // gating capability
}{
	{"create_issue", "board.create"},
	{"transition_issue", "board.move"},
	{"assign_issue", "board.assign"},
	{"set_labels", "board.label"},
	{"close_issue", "board.close"},
	{"list_issues", "board.read"},
	{"get_issue", "board.read"},
}

// boardToolFQN returns the MCP fully-qualified name a bot sees for the given
// bare tool name.
func boardToolFQN(tool string) string {
	return "mcp__" + boardMCPServerName + "__" + tool
}

// HasBoardCapability reports whether the granted-cap list contains any
// `board.*` entry — used to decide whether to register the board MCP server
// at all.
func HasBoardCapability(caps []string) bool {
	for _, c := range caps {
		if strings.HasPrefix(c, boardCapabilityPrefix) {
			return true
		}
	}
	return false
}

// BoardToolsFor returns the MCP FQN list the granted capabilities unlock.
// Order matches boardToolByName so AllowedTools extensions are deterministic.
func BoardToolsFor(caps []string) []string {
	grants := map[string]bool{}
	for _, c := range caps {
		grants[c] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range boardToolByName {
		if !grants[m.Cap] {
			continue
		}
		fqn := boardToolFQN(m.Tool)
		if seen[fqn] {
			continue
		}
		out = append(out, fqn)
		seen[fqn] = true
	}
	return out
}
