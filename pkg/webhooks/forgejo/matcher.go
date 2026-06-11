package forgejo

import "strings"

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
// bare "*", exact match). Duplicated rather than imported to keep each
// provider package self-contained.
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
