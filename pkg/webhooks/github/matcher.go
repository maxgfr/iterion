package github

import "strings"

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
// We DON'T import gitlab to share the function so each provider can
// evolve independently — the duplication is trivial.
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
		prefix := strings.TrimSuffix(pat, "*")
		return strings.HasPrefix(path, prefix)
	}
	return pat == path
}
