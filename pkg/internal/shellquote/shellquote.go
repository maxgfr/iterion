// Package shellquote produces POSIX-shell-safe single-quoted tokens
// for use in `sh -c` command strings. Use this whenever an argv must
// be flattened to a shell snippet that may later go through another
// shell layer (e.g. devcontainer postCreate, kubectl exec wrappers).
//
// The function does NOT defend against pipelines that re-expand the
// quoted result (notably os.ExpandEnv on the post-quoted string, which
// would interpret $VAR even inside single quotes from sh's
// perspective). See pkg/backend/model.shellEscape's security note for
// the always-quoted variant used in template substitution.
package shellquote

import "strings"

// Quote returns s safe to drop into a /bin/sh command line. Empty
// strings become `”`; strings of safe chars are returned bare;
// everything else is wrapped in single quotes with embedded single
// quotes escaped via the canonical `'\”` sequence.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	if isSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isSafe(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9':
		case c == '_' || c == '-' || c == '.' || c == '/' || c == ':' || c == '@' || c == ',' || c == '+' || c == '=':
		default:
			return false
		}
	}
	return true
}
