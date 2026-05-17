//go:build !unix

package store

import "os"

// openNoFollow falls back to a plain open on platforms that don't
// expose O_NOFOLLOW (notably Windows). The TOCTOU window remains on
// those platforms — acceptable since cloud and CI run on Linux.
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
