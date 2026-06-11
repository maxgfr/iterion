package github

import "github.com/SocialGouv/iterion/pkg/webhooks"

// MatchEvent reports whether kind is permitted by the allowlist. An
// empty allowlist defaults to {"pull_request"} — the only event V1
// handles. We keep the contract identical to gitlab.MatchEvent so the
// CRUD UI doesn't have to special-case GitHub.
func MatchEvent(allowlist []string, kind string) bool {
	if len(allowlist) == 0 {
		return kind == "pull_request"
	}
	for _, a := range allowlist {
		if a == kind || a == "*" {
			return true
		}
	}
	return false
}

// MatchProject reports whether a repo full name ("owner/repo") is
// permitted by the allowlist. Same semantics as the GitLab matcher: a
// trailing "/*" prefix wildcard, a bare "*" wildcard, and exact match.
// Delegates to webhooks.MatchProject (the canonical implementation
// shared by every provider).
func MatchProject(allowlist []string, projectPath string) bool {
	return webhooks.MatchProject(allowlist, projectPath)
}
