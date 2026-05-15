package conductor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Workspaces manages per-issue workspace directories under a single
// root. It enforces filename sanitization and refuses to traverse
// outside the root via symlinks or pathological IDs.
type Workspaces struct {
	root string
}

// NewWorkspaces returns a manager rooted at the given path. The root
// itself is created on first Create.
func NewWorkspaces(root string) (*Workspaces, error) {
	if root == "" {
		return nil, errors.New("workspace: root path required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: resolve root: %w", err)
	}
	return &Workspaces{root: abs}, nil
}

// Root returns the absolute root path.
func (w *Workspaces) Root() string { return w.root }

// Path returns the absolute workspace path for the given issue ID,
// without creating anything on disk.
func (w *Workspaces) Path(issueID string) string {
	return filepath.Join(w.root, sanitizeKey(issueID))
}

// Create ensures a workspace directory exists for issueID. The boolean
// return reports whether the directory was created by this call (so a
// caller can run after_create hooks only on first creation). The
// returned path is guaranteed to live under the configured root.
func (w *Workspaces) Create(issueID string) (path string, created bool, err error) {
	if issueID == "" {
		return "", false, errors.New("workspace: issue id required")
	}
	if err := os.MkdirAll(w.root, 0o755); err != nil {
		return "", false, fmt.Errorf("workspace: mkdir root: %w", err)
	}
	rootCanon, err := filepath.EvalSymlinks(w.root)
	if err != nil {
		return "", false, fmt.Errorf("workspace: canonicalize root: %w", err)
	}

	target := filepath.Join(w.root, sanitizeKey(issueID))
	target = filepath.Clean(target)

	created = true
	if info, statErr := os.Stat(target); statErr == nil {
		if !info.IsDir() {
			return "", false, fmt.Errorf("workspace: %s exists and is not a directory", target)
		}
		created = false
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", false, fmt.Errorf("workspace: stat target: %w", statErr)
	} else {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return "", false, fmt.Errorf("workspace: mkdir target: %w", err)
		}
	}

	canon, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", false, fmt.Errorf("workspace: canonicalize target: %w", err)
	}
	if !isWithin(canon, rootCanon) {
		return "", false, fmt.Errorf("workspace: target %q escapes root %q (symlink or traversal)", canon, rootCanon)
	}
	return canon, created, nil
}

// Remove deletes the workspace directory tree if present. Returns nil
// when the directory is already absent.
func (w *Workspaces) Remove(issueID string) error {
	target := filepath.Join(w.root, sanitizeKey(issueID))
	rootCanon, err := filepath.EvalSymlinks(w.root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("workspace: canonicalize root: %w", err)
	}
	canon, err := filepath.EvalSymlinks(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("workspace: canonicalize target: %w", err)
	}
	if rootCanon != "" && !isWithin(canon, rootCanon) {
		return fmt.Errorf("workspace: refusing to remove %q outside root", canon)
	}
	return os.RemoveAll(canon)
}

var sanitizeKeyRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// sanitizeKey replaces filesystem-hostile characters with underscore.
// A leading dot is also escaped so the directory is not hidden.
func sanitizeKey(s string) string {
	out := sanitizeKeyRe.ReplaceAllString(s, "_")
	out = strings.TrimSpace(out)
	if out == "" {
		out = "_"
	}
	if strings.HasPrefix(out, ".") {
		out = "_" + out
	}
	return out
}

// isWithin reports whether child sits at or below parent. Both must be
// absolute, canonical paths.
func isWithin(child, parent string) bool {
	if child == parent {
		return true
	}
	prefix := parent
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(child, prefix)
}
