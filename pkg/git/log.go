package git

import (
	"fmt"
	"strings"
	"time"
)

// CommitInfo is one entry in the log between two refs. Fields mirror what
// the Commits tab needs to render a GitHub-PR-style row: short SHA, the
// subject line, the author display name, and an absolute timestamp the
// frontend formats relatively.
type CommitInfo struct {
	SHA     string    `json:"sha"`
	Short   string    `json:"short"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	Email   string    `json:"email,omitempty"`
	Date    time.Time `json:"date"`
}

// Log returns commits in (base, head] inside repo. base may be empty —
// in which case all commits reachable from head are returned. The caller
// is responsible for passing valid refs (full SHAs preferred for stability).
//
// Errors that mean "this directory is not a git repository" are flattened
// to ErrNotGitRepo so the HTTP layer can render a 200 + available:false
// response, matching the contract of Status / StatusBetween.
func Log(repo, base, head string) ([]CommitInfo, error) {
	if !isGitDir(repo) {
		return nil, ErrNotGitRepo
	}
	rangeArg := head
	if base != "" {
		rangeArg = base + ".." + head
	}
	// Use NUL separators between both fields and records. Commit subjects,
	// author names, and emails are user-controlled and may contain tabs; a
	// field delimiter that can also appear in the data makes one malformed
	// commit break the entire Commits tab. Git commit messages cannot contain
	// NUL, so this gives parseLog an unambiguous six-field record shape.
	// `-z` suppresses git log's default newline between records and replaces
	// it with NUL; because %aI is the last field, that record separator is
	// also the sixth field separator for the next record.
	format := "%H%x00%h%x00%s%x00%an%x00%ae%x00%aI"
	out, err := run(repo, "log", "-z", "--reverse", "--pretty=format:"+format, rangeArg)
	if err != nil {
		return nil, err
	}
	return parseLog(out)
}

// RevParseHead resolves HEAD in repo to a full SHA. Returns ErrNotGitRepo
// when repo is not a git checkout. Used by the commits endpoint to surface
// the live worktree's tip alongside the commit list.
func RevParseHead(repo string) (string, error) {
	if !isGitDir(repo) {
		return "", ErrNotGitRepo
	}
	out, err := run(repo, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func parseLog(raw []byte) ([]CommitInfo, error) {
	if len(raw) == 0 {
		return []CommitInfo{}, nil
	}
	parts := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
	if len(parts) == 1 && parts[0] == "" {
		return []CommitInfo{}, nil
	}
	if len(parts)%6 != 0 {
		return nil, fmt.Errorf("git log: malformed NUL-delimited output (%d fields)", len(parts))
	}
	out := make([]CommitInfo, 0, len(parts)/6)
	for i := 0; i < len(parts); i += 6 {
		ts, tErr := time.Parse(time.RFC3339, parts[i+5])
		if tErr != nil {
			return nil, fmt.Errorf("git log: parse date %q: %w", parts[i+5], tErr)
		}
		out = append(out, CommitInfo{
			SHA:     parts[i],
			Short:   parts[i+1],
			Subject: parts[i+2],
			Author:  parts[i+3],
			Email:   parts[i+4],
			Date:    ts,
		})
	}
	return out, nil
}
