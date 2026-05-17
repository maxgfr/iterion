//go:build unix

package store

import (
	"os"
	"syscall"
)

// openNoFollow opens a file refusing to traverse a symlink at the
// final path component. Used by OpenRunFile to harden the TOCTOU
// window between EvalSymlinks and Open.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}
