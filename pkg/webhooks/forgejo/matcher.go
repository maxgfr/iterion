package forgejo

import "github.com/SocialGouv/iterion/pkg/webhooks"

// MatchEvent reports whether kind is permitted by the allowlist. An
// empty allowlist defaults to {"pull_request"} — the only event V1
// handles for Forgejo/Gitea.
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

// MatchProject mirrors the GitLab/GitHub matchers (trailing /* wildcard,
// bare "*", exact match). Delegates to webhooks.MatchProject (the
// canonical implementation shared by every provider).
func MatchProject(allowlist []string, projectPath string) bool {
	return webhooks.MatchProject(allowlist, projectPath)
}
