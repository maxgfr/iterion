package store

import (
	"os"
	"path/filepath"

	gitpkg "github.com/SocialGouv/iterion/pkg/git"
)

// StoreDirName is the conventional directory name for an iterion run store.
const StoreDirName = ".iterion"

// ResolveStoreDir picks the run-store directory shared by the CLI and the
// editor. An explicit override wins; otherwise it discovers an existing
// .iterion via a git-bounded walk-up:
//
//   - Inside a git repository: walk up from start looking for an existing
//     .iterion, but DO NOT escape the repository. If none is found by the
//     repo root, return <repoRoot>/.iterion.
//   - Outside any git repository: do not walk up at all (a parent .iterion
//     in such a setting almost always belongs to an unrelated context).
//     Return <start>/.iterion.
//
// The git-bounded walk fixes a long-standing footgun: a stray ~/.iterion
// (e.g. created when the user once ran iterion from $HOME) would otherwise
// be picked up by every project nested under home, silently sharing state
// across unrelated projects. Bounding the walk to the repo mirrors how
// .gitignore / .git themselves are scoped and keeps each repository's
// runs cleanly isolated.
func ResolveStoreDir(start, override string) string {
	if override != "" {
		return override
	}
	if start == "" {
		return StoreDirName
	}

	abs, err := filepath.Abs(start)
	if err != nil {
		return filepath.Join(start, StoreDirName)
	}

	repoRoot := gitpkg.FindRepoRoot(abs)

	dir := abs
	for {
		candidate := filepath.Join(dir, StoreDirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		// Stop at the repo boundary — never inherit an ancestor's .iterion
		// from outside the repository.
		if repoRoot != "" && dir == repoRoot {
			return filepath.Join(repoRoot, StoreDirName)
		}
		// Outside any repo: do not walk up at all. A parent .iterion in
		// this case is almost always an unrelated leftover.
		if repoRoot == "" {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return filepath.Join(abs, StoreDirName)
}
