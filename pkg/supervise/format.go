package supervise

import "strings"

// FormatOperatorMessages renders queued operator/supervisor messages the
// way the claude_code inbox-drain hooks do, so a steered raw Claude Code
// session sees the same framing an iterion-managed run does. Returns ""
// for an empty slice (the hook then emits a no-op).
func FormatOperatorMessages(texts []string) string {
	if len(texts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Operator queued message")
	if len(texts) > 1 {
		sb.WriteString("s")
	}
	sb.WriteString(":\n\n")
	for i, t := range texts {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		sb.WriteString(t)
	}
	return sb.String()
}
