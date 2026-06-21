// Package strutil holds tiny string helpers that have no direct stdlib
// equivalent and were otherwise copy-pasted across packages. The
// no-trim "first non-empty" need is covered by the stdlib cmp.Or, and
// slice membership by slices.Contains; only the whitespace-trimming
// variant below has no stdlib counterpart, so it lives here once.
package strutil

import "strings"

// FirstNonBlank returns the first value that is non-empty after trimming
// surrounding whitespace, or "" when every value is blank. Use this
// (rather than cmp.Or) when a whitespace-only value must be treated as
// absent.
func FirstNonBlank(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
