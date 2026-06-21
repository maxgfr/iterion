package webhooks

import (
	"slices"
	"strings"
)

// MatchEvent is the canonical event-kind allowlist matcher used by
// every provider call site (gitlab/github/forgejo). When allowlist is
// non-empty it accepts kind iff the list contains kind or "*". When
// empty the provider's defaults take over — variadic so each call site
// stays explicit about the zero-config contract:
//
//   - gitlab: MatchEvent(list, kind, "merge_request", "note") — both
//     the auto-review (MR open/reopen) and the on-demand /revi note
//     trigger reach a zero-config webhook.
//   - github / forgejo: MatchEvent(list, kind, "pull_request") — the
//     only event V1 handles.
//
// Operators who want to gate one off list the other explicitly
// (e.g. ["merge_request"] disables /revi while keeping auto-review).
func MatchEvent(allowlist []string, kind string, defaults ...string) bool {
	if len(allowlist) == 0 {
		return slices.Contains(defaults, kind)
	}
	return slices.Contains(allowlist, kind) || slices.Contains(allowlist, "*")
}

// MatchProject is the canonical project-path allowlist matcher shared
// by every provider call site (gitlab/github/forgejo) and the generic
// JSON webhook in pkg/server. An empty allowlist allows every project
// in the tenant. Each entry supports:
//
//   - a bare "*" (match all),
//   - a trailing "/*" prefix wildcard ("group/*" matches "group/anything"
//     and "group/sub/repo"),
//   - otherwise an exact match.
func MatchProject(allowlist []string, projectPath string) bool {
	if len(allowlist) == 0 {
		return true
	}
	for _, pat := range allowlist {
		if matchProjectPattern(pat, projectPath) {
			return true
		}
	}
	return false
}

func matchProjectPattern(pat, path string) bool {
	pat = strings.TrimSpace(pat)
	if pat == "" {
		return false
	}
	if pat == "*" {
		return true
	}
	if strings.HasSuffix(pat, "/*") {
		prefix := strings.TrimSuffix(pat, "*") // keeps the trailing slash
		return strings.HasPrefix(path, prefix)
	}
	return pat == path
}
