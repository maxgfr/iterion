package webhooks

import "strings"

// ParseSlashCommand extracts a leading slash-command from a comment / note
// body, e.g. "/featurly add an export endpoint" → ("featurly", "add an export
// endpoint"). Returns ("", "") when the body does not start with a command.
//
// The match is case-insensitive (the command id is lowercased) and tolerant of
// the noise forge UIs prepend: blank lines and quote-reply lines (">", a
// GitLab/GitHub quote of an earlier comment) are skipped, so a quote-reply that
// leads with the quoted text still finds the operator's command on the first
// real line. The first non-blank, non-quote line decides: if it doesn't start
// with "/" there is no command.
//
// This is the single command grammar shared by every comment surface —
// gitlab.ParsedNote.Command, prforge.ParsedNote.Command, and the native board
// comment handler all delegate here so a "/command" parses identically
// everywhere.
func ParseSlashCommand(body string) (cmd, args string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ">") {
			continue // skip blank lines and quoted context
		}
		if !strings.HasPrefix(line, "/") {
			return "", ""
		}
		rest := strings.TrimPrefix(line, "/")
		if i := strings.IndexAny(rest, " \t"); i >= 0 {
			return strings.ToLower(rest[:i]), strings.TrimSpace(rest[i:])
		}
		return strings.ToLower(rest), ""
	}
	return "", ""
}
