//go:build windows

package conductor

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileLock is the cross-platform process lock returned by Lock.
type FileLock interface {
	Unlock() error
}

type windowsHandle struct {
	f *os.File
}

func (h *windowsHandle) Unlock() error { return h.f.Close() }

// Lock acquires an exclusive lock by opening the file in exclusive mode
// (Windows refuses concurrent opens of the same file with O_EXCL-like
// semantics via os.OpenFile when both writers request RDWR + CREATE).
// Best-effort — Windows operators should prefer the unix build for now.
func Lock(path string) (FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("conductor lock: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o644)
	if err != nil {
		return nil, fmt.Errorf("conductor lock: %s already held or unwritable: %w", path, err)
	}
	return &windowsHandle{f: f}, nil
}
