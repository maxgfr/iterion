package gitlab

import "github.com/SocialGouv/iterion/pkg/webhooks"

// MatchEvent reports whether kind is permitted by the allowlist. An
// empty allowlist defaults to the union {merge_request, note} — both
// the auto-review path (MR open/reopen) and the on-demand /revi note
// trigger reach a zero-config webhook. Operators who want to gate one
// off list the other explicitly (e.g. ["merge_request"] disables /revi
// while keeping auto-review).
func MatchEvent(allowlist []string, kind string) bool {
	if len(allowlist) == 0 {
		return kind == "merge_request" || kind == "note"
	}
	for _, a := range allowlist {
		if a == kind || a == "*" {
			return true
		}
	}
	return false
}

// MatchProject reports whether a project path ("group/sub/repo") is
// permitted by the allowlist. An empty allowlist allows every project
// in the tenant. Entries support a trailing "/*" prefix wildcard
// ("group/*" matches "group/anything" and "group/sub/repo") and a bare
// "*" (match all); otherwise an exact match is required.
//
// Delegates to webhooks.MatchProject (the canonical implementation
// shared by every provider).
func MatchProject(allowlist []string, projectPath string) bool {
	return webhooks.MatchProject(allowlist, projectPath)
}
