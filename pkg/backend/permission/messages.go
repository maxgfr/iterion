package permission

import (
	"fmt"
	"strings"
)

// DenyMessage is the tool_result text shown to the MODEL when a call is
// refused by the gate. It names the offending tool/argument and the
// matched rule (when any) so the agent can adapt its approach instead of
// blindly retrying. Identical wording on both backends.
func DenyMessage(toolName string, input map[string]any, rule string) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "Permission denied: the `%s` tool is not authorized by this run's permission policy", toolName)
	if arg := briefArg(toolName, input); arg != "" {
		fmt.Fprintf(b, " for %q", arg)
	}
	if rule != "" {
		fmt.Fprintf(b, " (deny rule: %s)", rule)
	}
	b.WriteString(". Do not retry this action; choose an approach within the allowed tools, or explain why it is needed so the operator can authorize it.")
	return b.String()
}

// AskPrompt is the question surfaced to the HUMAN when a call needs
// approval (ModeAsk, off-policy). The operator answers with one of the
// AnswerAllowOnce / AnswerAllowAlways / AnswerDeny tokens (the studio
// renders these as buttons). Identical wording on both backends.
func AskPrompt(toolName string, input map[string]any, rule string) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "🔐 Permission requested: the agent wants to use the `%s` tool", toolName)
	if arg := briefArg(toolName, input); arg != "" {
		fmt.Fprintf(b, ":\n\n    %s\n", arg)
	} else {
		b.WriteString(".\n")
	}
	b.WriteString("\nApprove this action? Reply `allow` (once), `allow always` (add to the allowlist for the rest of this run), or `deny`.")
	return b.String()
}

// Answer tokens the operator returns for an AskPrompt. Matched
// case-insensitively and trimmed; "allow always" / "always" map to
// AnswerAllowAlways. Anything unrecognized is treated as a denial
// (fail-safe).
const (
	AnswerAllowOnce   = "allow"
	AnswerAllowAlways = "allow always"
	AnswerDeny        = "deny"
)

// ParseAnswer classifies an operator's free-text reply to an AskPrompt.
// Returns (allow, always). A reply that doesn't clearly approve is a
// denial — the gate fails safe.
func ParseAnswer(s string) (allow bool, always bool) {
	t := strings.ToLower(strings.TrimSpace(s))
	switch {
	case t == "deny" || t == "no" || t == "reject" || t == "n":
		return false, false
	case strings.Contains(t, "always"):
		return true, true
	case t == "allow" || t == "yes" || t == "y" || t == "approve" || t == "ok" || t == "once":
		return true, false
	default:
		return false, false
	}
}

// GrantRuleFor builds an allow-rule string that authorizes a granted
// call. "always" scopes the rule to the whole tool (bare name); "once"
// scopes it to the specific argument so only the identical retry passes.
func GrantRuleFor(toolName string, input map[string]any, always bool) string {
	if always {
		return canonicalRuleToolName(toolName)
	}
	if arg := briefArg(toolName, input); arg != "" {
		return fmt.Sprintf("%s(%s)", canonicalRuleToolName(toolName), arg)
	}
	return canonicalRuleToolName(toolName)
}

// briefArg renders the most identifying argument of a tool call for
// human/model messages (the command, path, url, …).
func briefArg(toolName string, input map[string]any) string {
	s := summarize(toolName, input)
	// summarize may join several candidates with '\n'; show the first.
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const max = 200
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// canonicalRuleToolName returns the tool name to use when synthesizing a
// grant rule. We keep the agent-issued spelling (it round-trips through
// canonicalToolName on the next Evaluate), defaulting to the raw name.
func canonicalRuleToolName(toolName string) string {
	if strings.TrimSpace(toolName) == "" {
		return "*"
	}
	return toolName
}
