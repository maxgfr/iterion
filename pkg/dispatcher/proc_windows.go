//go:build windows

package dispatcher

// localPidGone is the Windows stub. The dispatcher's local-PID
// stale-claim sweep is a Unix-only optimisation (kill -0 liveness);
// release.yml still cross-compiles a windows/{amd64,arm64} binary, so
// this keeps the package building. Returning false means "never treat a
// local marker as stale" — the safe default that never reclaims a claim
// we cannot verify. A future Windows port can probe liveness via
// OpenProcess + GetExitCodeProcess.
func localPidGone(_ int) bool {
	return false
}
