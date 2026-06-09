// Package workflowfile is the single source of truth for which file
// extensions iterion recognises as workflow source files. Plain workflow
// sources use `.bot`; packaged workflows use `.botz` through the bundle
// loader.
package workflowfile

import "strings"

// Extensions lists the accepted workflow file suffixes, including the
// leading dot. Order is informational only.
var Extensions = []string{".bot"}

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
