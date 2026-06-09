package gitlab

import "strings"

// MatchEvent reports whether kind is permitted by the allowlist. An
// empty allowlist defaults to {"merge_request"} — the only event V1
// handles.
func MatchEvent(allowlist []string, kind string) bool {
	if len(allowlist) == 0 {
		return kind == "merge_request"
	}
	for _, a := range allowlist {
		if a == kind || a == "*" {
			return true
		}
	}
	return false
}

// MatchProject reports whether a project path ("group/sub/repo") is
// permitted by the allowlist. An empty allowlist allows every project in
// the tenant. Entries support a trailing "/*" prefix wildcard
// ("group/*" matches "group/anything" and "group/sub/repo") and a bare
// "*" (match all); otherwise an exact match is required.
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
