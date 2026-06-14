package store

import (
	"io"
	"os"
	"path/filepath"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// TeeRunLog opens <storeRoot>/runs/<runID>/run.log and returns a new
// logger whose output is multiplexed to both stderr and the file. The
// runID is validated before any path is constructed so a hostile CLI or
// dispatcher-supplied ID cannot create/open run.log outside the store and
// only fail later when CreateRun validates the ID.
//
// On error the original logger is returned with a nil closer so callers
// can keep running — a run with no writable store dir still works (logs
// go to stderr only). The returned closer is nil when no tee was set up;
// callers should defer Close on the non-nil result.
//
// Shared between the CLI runner (pkg/cli/run.go) and the in-process
// dispatcher runner (pkg/dispatcher/engine_runner.go) so dispatched runs
// produce the same per-run log file the studio's log viewer expects —
// without this, the studio renders "No log captured" on every
// dispatcher-spawned run because the file simply doesn't exist.
func TeeRunLog(logger *iterlog.Logger, level iterlog.Level, storeRoot, runID string) (*iterlog.Logger, io.Closer) {
	warn := func(format string, args ...any) {
		if logger != nil {
			logger.Warn(format, args...)
		}
	}
	if err := SanitizePathComponent("run ID", runID); err != nil {
		warn("store: refusing run.log tee for unsafe run ID: %v", err)
		return logger, nil
	}

	runsDir := filepath.Join(storeRoot, "runs")
	runDir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(runDir, dirPerm); err != nil {
		warn("store: mkdir run dir for log tee: %v", err)
		return logger, nil
	}
	// MkdirAll does not tighten existing directories, and this helper can be
	// the first store code path to touch <root>/runs/<runID>. Run logs can
	// contain prompts, model outputs, and secrets, so force the private store
	// modes even when an older build left the directory world-readable.
	if err := os.Chmod(runsDir, dirPerm); err != nil {
		warn("store: chmod runs dir for log tee: %v", err)
		return logger, nil
	}
	if err := os.Chmod(runDir, dirPerm); err != nil {
		warn("store: chmod run dir for log tee: %v", err)
		return logger, nil
	}

	logPath := filepath.Join(runDir, "run.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePerm)
	if err != nil {
		warn("store: open run.log for tee: %v", err)
		return logger, nil
	}
	if err := logFile.Chmod(filePerm); err != nil {
		_ = logFile.Close()
		warn("store: chmod run.log for tee: %v", err)
		return logger, nil
	}
	return iterlog.New(level, io.MultiWriter(os.Stderr, logFile)), logFile
}
