//go:build !windows

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type flockLock struct {
	f *os.File
}

func (l *flockLock) Unlock() error {
	if err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN); err != nil {
		l.f.Close()
		return fmt.Errorf("store: flock unlock: %w", err)
	}
	return l.f.Close()
}

// lockRun acquires an exclusive flock on <runDir>/.lock.
// The OS automatically releases the lock if the process crashes.
func lockRun(runDir, label string) (RunLock, error) {
	if err := os.MkdirAll(runDir, dirPerm); err != nil {
		return nil, fmt.Errorf("store: mkdir for lock: %w", err)
	}
	return lockFile(filepath.Join(runDir, ".lock"), label)
}

// lockFile is the shared primitive used by lockRun and the exported
// AcquireFileLock. It locks an exact path (caller is responsible for
// ensuring the parent directory exists).
func lockFile(path, label string) (RunLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return nil, fmt.Errorf("store: mkdir for lock: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, fmt.Errorf("store: open lock file for %s: %w", label, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("store: %s is locked by another process", label)
	}
	return &flockLock{f: f}, nil
}
