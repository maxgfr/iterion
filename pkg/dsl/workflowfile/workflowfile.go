// Package workflowfile is the single source of truth for which file
// extensions iterion recognises as workflow source files. Both `.iter`
// and `.bot` are accepted everywhere a workflow file is loaded: the
// CLI, the HTTP server, the file watcher, the embedded examples, and
// the studio. The two extensions are semantically identical at the
// parser, IR, and runtime levels — the distinction is narrative, not
// technical. See the README section "`.iter` vs `.bot`" for guidance.
package workflowfile

import "strings"

// Extensions lists the accepted workflow file suffixes, including the
// leading dot. Order is informational only.
var Extensions = []string{".iter", ".bot"}

// IsWorkflowFile reports whether path ends with one of the accepted
// workflow file extensions.
func IsWorkflowFile(path string) bool {
	for _, ext := range Extensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
