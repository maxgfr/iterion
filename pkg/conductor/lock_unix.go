//go:build !windows

package conductor

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FileLock is the cross-platform process lock returned by Lock.
type FileLock interface {
	Unlock() error
}

type flockHandle struct {
	f *os.File
}

func (l *flockHandle) Unlock() error {
	defer l.f.Close()
	return syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
}

// Lock acquires a non-blocking exclusive lock on a file at path. If the
// file is already locked by another process, returns an error so the
// caller can refuse to start. The OS releases the lock automatically
// when the process exits, including on crash.
func Lock(path string) (FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("conductor lock: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("conductor lock: open %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("conductor lock: %s already held by another process", path)
	}
	return &flockHandle{f: f}, nil
}
