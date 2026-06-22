//go:build unix

package dispatcher

import (
	"errors"
	"syscall"
)

// localPidGone reports whether the given local PID is definitively gone,
// i.e. a claim marker referencing it is safe to reclaim. It uses
// kill(pid, 0): nil = the process is alive, EPERM = alive under another
// user, ESRCH = gone. Anything we can't confidently read as "gone"
// returns false so the stale-claim sweeper never reclaims a claim from a
// process that might still be live.
func localPidGone(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPERM) {
		return false
	}
	return errors.Is(err, syscall.ESRCH)
}
