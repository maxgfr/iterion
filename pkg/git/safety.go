package git

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidateRelPath accepts a path coming from an HTTP query and verifies
// it stays inside the run's working directory. It rejects absolute paths,
// `..` traversal, and NUL bytes.
//
// The accepted form is a forward-slash relative path. Callers should pass
// the value straight through to git/os.ReadFile after validation; we do
// not normalise to OS separators here because git itself uses forward
// slashes on every platform.
func ValidateRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("git: path must not be empty")
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("git: path contains null byte")
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return fmt.Errorf("git: path must be relative")
	}
	// Reject a leading dash. showAt (range.go) passes `ref:<relPath>`
	// as a single positional arg to `git show`; a path starting with
	// "-" would be parsed as a git flag (e.g. `git show HEAD:-v` ⇒
	// verbose mode), leaking unrelated output to the caller.
	if strings.HasPrefix(p, "-") {
		return fmt.Errorf("git: path %q must not start with '-' (would be parsed as a flag)", p)
	}
	// filepath.IsLocal (Go 1.20+) rejects "..", "" segments, drive
	// letters, and other escape attempts using the OS rules. We
	// normalise to OS separators just for this check so the same input
	// is judged identically on Windows and Linux.
	osPath := filepath.FromSlash(p)
	if !filepath.IsLocal(osPath) {
		return fmt.Errorf("git: path %q escapes working directory", p)
	}
	return nil
}

// branchNameAllowed accepts an alphanumeric, hyphen, underscore, dot
// or slash sequence. The leading byte must be alphanumeric so a value
// starting with `-` can never reach `git branch` as a positional that
// might be re-parsed as a flag (defense in depth — callers should also
// pass `--` to git, but this catches the bug earlier with a friendly
// error).
var branchNameAllowed = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// ValidateBranchName accepts a branch name coming from a user-controlled
// surface (`--branch-name` CLI flag, Launch API, studio modal) and
// rejects forms that could either confuse `git branch` flag parsing or
// be rejected by `git check-ref-format` downstream — failing early with
// a clear error rather than surfacing a noisy git stderr to the caller.
//
// The rules are intentionally tighter than git's own check-ref-format:
// we want a small allowlist (letters, digits, `.`, `_`, `/`, `-`) so
// every accepted value also passes git's checks. Combined with the
// `--` sentinel used by callers, this prevents flag injection through
// the storage-branch name.
func ValidateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("git: branch name must not be empty")
	}
	if len(name) > 255 {
		return fmt.Errorf("git: branch name must be at most 255 bytes")
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("git: branch name contains null byte")
	}
	if !branchNameAllowed.MatchString(name) {
		return fmt.Errorf("git: branch name %q must match [A-Za-z0-9][A-Za-z0-9._/-]* (no leading -/./_, no spaces or special chars)", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("git: branch name %q must not contain '..'", name)
	}
	if strings.Contains(name, "//") {
		return fmt.Errorf("git: branch name %q must not contain '//'", name)
	}
	if strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("git: branch name %q must not end with '/' or '.'", name)
	}
	if strings.HasSuffix(name, ".lock") {
		return fmt.Errorf("git: branch name %q must not end with '.lock'", name)
	}
	return nil
}
