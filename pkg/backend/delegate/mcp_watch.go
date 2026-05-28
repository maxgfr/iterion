package delegate

import "strings"

// watchCapabilityPrefix flags a capability as addressing the watch
// subsystem (watch.subscribe / watch.unsubscribe).
const watchCapabilityPrefix = "watch."

// HasWatchCapability reports whether the granted-cap list contains any
// `watch.*` entry. Watch tools are currently wired for the claw backend
// only (see pkg/backend/tool/claw_watch_tools.go); the claude_code path
// uses this to warn rather than silently drop the capability.
func HasWatchCapability(caps []string) bool {
	for _, c := range caps {
		if strings.HasPrefix(c, watchCapabilityPrefix) {
			return true
		}
	}
	return false
}
