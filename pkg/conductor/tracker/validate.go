package tracker

import (
	"fmt"
	"regexp"
	"strings"
)

// repoComponentRe matches a single owner / group / repo segment in a
// "owner/repo" or "owner/group/.../repo" path. Mirrors GitHub's
// constraint (alphanumeric, dash, dot, underscore; no leading dash or
// dot; ≤ 100 chars). Forgejo additionally allows nested sub-groups
// joined by `/`, but each segment follows the same rules. The check
// stops short of full RFC 3986 — we want to reject obviously bad
// inputs (path-traversal segments, embedded `?`/`#`, leading hyphens)
// that would otherwise interpolate into a request URL.
var repoComponentRe = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]{0,99}$`)

// ValidateRepoPath verifies that an `owner/repo` (or `owner/group/repo`)
// string is safe to interpolate into a tracker REST URL. Returns nil
// on success; an error citing the offending component otherwise.
//
// Without this, an opts.Repo of `../admin` or `owner/repo?token=x`
// would compose into `/api/v1/repos/../admin/issues/...` and re-write
// the request semantics. Validation is performed once in the adapter's
// constructor; the per-call URL composition stays as-is.
func ValidateRepoPath(repo string) error {
	if repo == "" {
		return fmt.Errorf("repo is empty")
	}
	if strings.HasPrefix(repo, "/") || strings.HasSuffix(repo, "/") {
		return fmt.Errorf("repo %q must not begin or end with '/'", repo)
	}
	parts := strings.Split(repo, "/")
	if len(parts) < 2 {
		return fmt.Errorf("repo %q must be owner/repo (or owner/group/repo for nested forges)", repo)
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return fmt.Errorf("repo %q has invalid path component %q", repo, p)
		}
		if !repoComponentRe.MatchString(p) {
			return fmt.Errorf("repo %q has invalid path component %q (allowed: A-Z, a-z, 0-9, '.', '_', '-')", repo, p)
		}
	}
	return nil
}
