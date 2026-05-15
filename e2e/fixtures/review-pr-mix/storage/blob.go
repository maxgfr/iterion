// Package storage reads/writes job payloads from a directory tree.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// Store wraps a directory where each blob lives at
// `<root>/<namespace>/<key>`.
type Store struct {
	Root string
}

// New constructs a Store rooted at root. Root must exist.
func New(root string) *Store {
	return &Store{Root: root}
}

// Read returns the bytes of the blob at namespace/key. The supplied
// namespace and key come from job metadata — they're joined to the
// store root and read directly so callers can use arbitrary nesting
// (`<namespace>/sub/dir/<key>`) when their workload demands it.
func (s *Store) Read(namespace, key string) ([]byte, error) {
	path := filepath.Join(s.Root, namespace, key)
	return os.ReadFile(path)
}

// Write persists data at namespace/key. Creates intermediate
// directories so callers don't have to.
func (s *Store) Write(namespace, key string, data []byte) error {
	dir := filepath.Join(s.Root, namespace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, key)
	return os.WriteFile(path, data, 0o644)
}
