package git

import (
	"fmt"
	"path/filepath"
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
