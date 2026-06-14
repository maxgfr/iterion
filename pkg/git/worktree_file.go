package git

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// lstatWorktreePath resolves relPath under dir without following symlink
// components. The final component may itself be a symlink (callers that want
// git-like contents should read the link text), but any symlink in a parent
// component is treated as missing so a path cannot escape the worktree via a
// linked directory.
func lstatWorktreePath(dir, relPath string) (string, os.FileInfo, error) {
	osPath := filepath.FromSlash(relPath)
	if filepath.IsAbs(osPath) || !filepath.IsLocal(osPath) {
		return "", nil, &os.PathError{Op: "lstat", Path: relPath, Err: fs.ErrInvalid}
	}
	parts := strings.Split(osPath, string(filepath.Separator))
	current := dir
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return current, nil, err
		}
		if i < len(parts)-1 {
			if info.Mode()&os.ModeSymlink != 0 {
				return current, nil, &os.PathError{Op: "lstat", Path: current, Err: fs.ErrNotExist}
			}
			if !info.IsDir() {
				return current, nil, &os.PathError{Op: "lstat", Path: current, Err: fs.ErrInvalid}
			}
			continue
		}
		return current, info, nil
	}
	info, err := os.Lstat(current)
	if err != nil {
		return current, nil, err
	}
	return current, info, nil
}

func readWorktreeFile(dir, relPath string) ([]byte, error) {
	abs, info, err := lstatWorktreePath(dir, relPath)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(abs)
		if err != nil {
			return nil, err
		}
		return []byte(target), nil
	}
	if !info.Mode().IsRegular() {
		return nil, &os.PathError{Op: "read", Path: abs, Err: fs.ErrInvalid}
	}
	// Size guard: refuse to read an oversized regular file into memory.
	// The Lstat size is authoritative for a one-shot read here (unlike the
	// streaming CountUntrackedLines path, no re-stat-after-open is needed).
	// The symlink branch above is exempt — a link target is always tiny.
	if info.Size() > diffPayloadCap {
		return nil, errOversized
	}
	return os.ReadFile(abs)
}
