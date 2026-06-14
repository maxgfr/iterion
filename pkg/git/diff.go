package git

import (
	"errors"
	"os"
	"strings"
)

// Diff returns the HEAD content (Before) and current working-tree content
// (After) of relPath inside dir, suitable for feeding into Monaco's
// DiffEditor. Either side is nil when the file does not exist there:
//
//   - Before == nil: untracked or freshly added (no HEAD blob)
//   - After  == nil: deleted from the worktree
//
// When the content is binary (NUL byte present in either side), Before
// and After are both nil and Binary is true so the studio can surface a
// "binary file not shown" placeholder instead of feeding raw bytes into
// a text editor.
//
// When either side exceeds diffPayloadCap, Before and After are both nil
// and Oversized is true — the oversized side is never read into memory,
// so a multi-GB tracked file cannot stall the studio on a diff click.
//
// relPath must already have passed ValidateRelPath. dir must be inside a
// git working tree (else ErrNotGitRepo).
func Diff(dir, relPath string) (DiffPayload, error) {
	if !isGitDir(dir) {
		return DiffPayload{}, ErrNotGitRepo
	}

	payload := DiffPayload{Path: relPath}

	// HEAD-side content via `git show HEAD:path`. A non-zero exit usually
	// means the path is untracked or freshly added — distinguishable from
	// real failures by the stderr message ("does not exist" / "exists on
	// disk, but not in 'HEAD'"), but for our purposes any "no such object"
	// outcome is rendered as Before == nil.
	headOut, headErr := showHead(dir, relPath)
	switch {
	case headErr == nil:
		s := string(headOut)
		payload.Before = &s
	case errors.Is(headErr, errNotInHead):
		// nil Before
	case errors.Is(headErr, errOversized):
		payload.Oversized = true
	default:
		return DiffPayload{}, headErr
	}

	// Worktree-side content via direct file read. We don't pipe through
	// git here because `git show :path` would mirror the index, not the
	// dirty working copy that the user actually sees on disk.
	wtOut, wtErr := readWorktreeFile(dir, relPath)
	switch {
	case wtErr == nil:
		s := string(wtOut)
		payload.After = &s
	case os.IsNotExist(wtErr):
		// nil After (deleted)
	case errors.Is(wtErr, errOversized):
		payload.Oversized = true
	default:
		return DiffPayload{}, wtErr
	}

	payload.finalize()
	return payload, nil
}

// finalize applies the order-sensitive post-read rules to a freshly populated
// payload, shared by Diff and DiffBetween so the precedence lives in one place.
// Oversize wins over binary detection: an oversized side is never read, so it
// can't be NUL-scanned — blank both sides and stop. Otherwise a NUL byte on
// either side (git's own binary signal, see diff.c:buffer_is_binary) marks the
// payload binary and blanks both sides so the studio never feeds non-UTF-8
// bytes into Monaco. A clean text diff is left untouched.
func (p *DiffPayload) finalize() {
	if p.Oversized {
		p.Before = nil
		p.After = nil
		return
	}
	if (p.Before != nil && strings.IndexByte(*p.Before, 0) >= 0) ||
		(p.After != nil && strings.IndexByte(*p.After, 0) >= 0) {
		p.Binary = true
		p.Before = nil
		p.After = nil
	}
}

// errNotInHead is the sentinel for "this path doesn't exist at the requested
// ref" (untracked, newly added, or deleted on the After side). Callers map it
// to a nil Before/After rather than propagating it as a hard error.
var errNotInHead = errors.New("git: path not at ref")

// errOversized is the sentinel for "this side of the diff exceeds
// diffPayloadCap". The reading primitives (readWorktreeFile, showAt) return
// it instead of loading the content; Diff/DiffBetween map it to a payload
// with Oversized = true and both sides nil.
var errOversized = errors.New("git: file exceeds diff payload size cap")

// diffPayloadCap bounds the bytes read into memory for either side of a
// Monaco diff payload, so a multi-GB tracked file cannot be slurped whole on
// a diff click. It reuses untrackedReadCap's 5 MiB value (numstat.go); it is a
// separate var (not the const itself) only so tests can shrink it without
// giving up immutability on the line-counter path.
var diffPayloadCap int64 = untrackedReadCap

// showHead is the HEAD-pinned form of showAt. Kept as a named wrapper so
// Diff()'s caller stays readable.
func showHead(dir, relPath string) ([]byte, error) {
	return showAt(dir, "HEAD", relPath)
}
