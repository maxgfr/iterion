package webhooks

import "strings"

// MatchProject is the canonical project-path allowlist matcher shared
// by every provider package (gitlab/github/forgejo) and the generic
// JSON webhook in pkg/server. An empty allowlist allows every project
// in the tenant. Each entry supports:
//
//   - a bare "*" (match all),
//   - a trailing "/*" prefix wildcard ("group/*" matches "group/anything"
//     and "group/sub/repo"),
//   - otherwise an exact match.
//
// The per-provider MatchProject helpers delegate here so each forge can
// import this single source of truth without coupling the provider
// packages to one another.
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
