package server

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/SocialGouv/iterion/pkg/store"
)

// crossStoreLogChunkBudget caps each wsTypeLogChunk shipped during a
// drain. Mirrors logChunkBudget in pkg/runview/file_log_source.go.
const crossStoreLogChunkBudget = 64 * 1024

// streamLogsCrossStore is the read-only cross-store path for the log
// subscription. Mirrors streamEventsCrossStore on run.log instead of
// events.jsonl: initial drain from fromOffset to EOF, then an
// fsnotify-driven tail. Exits on c.closed (WS disconnect) or terminal
// run status.
//
// The cross-store path does not honour handleUnsubscribeLogs
// mid-connection — there is no in-process subscription handle to
// cancel, only the file tail goroutine. The studio never sends
// unsubscribe_logs while a view is mounted, so this asymmetry is
// acceptable.
func (c *runConn) streamLogsCrossStore(fromOffset int64) {
	if c.xStorePath == "" {
		c.sendError("cross_store_unconfigured", "xStorePath empty", "")
		return
	}
	logPath := filepath.Join(c.xStorePath, "runs", c.runID, "run.log")

	// Live tests using runtime.New(...) directly never create run.log
	// (only `iterion run` CLI and runview.Service.prepareRunLog tee
	// to disk). Without this short-circuit the tail loop polls a
	// non-existent file forever and the studio's log pane is stuck on
	// "Waiting for log output…". Producers that DO write run.log
	// always create it before the first event, so a missing file at
	// subscription time means it'll never exist for this run — emit
	// log_terminated so the UI transitions to "No log captured."
	// instead of waiting indefinitely.
	if _, err := os.Stat(logPath); errors.Is(err, os.ErrNotExist) {
		c.sendEnvelope(wsTypeLogTerminated, map[string]string{"run_id": c.runID}, "")
		return
	}

	offset := c.drainCrossStoreLog(logPath, fromOffset)
	c.tailCrossStoreLog(logPath, offset)
}

// tailCrossStoreLog tails the foreign run.log via fsnotify with a
// defensive poll fallback; the terminal ticker re-reads run.json so a
// terminated run flushes a final drain + wsTypeLogTerminated.
func (c *runConn) tailCrossStoreLog(logPath string, offset int64) {
	watcher, watcherErr := fsnotify.NewWatcher()
	if watcherErr != nil {
		c.server.logger.Warn("runs_ws: cross-store log tail (%s): fsnotify unavailable, polling: %v", c.runID, watcherErr)
		c.tailCrossStoreLogPolling(logPath, offset)
		return
	}
	defer watcher.Close()

	dir := filepath.Dir(logPath)
	if err := watcher.Add(dir); err != nil {
		c.server.logger.Warn("runs_ws: cross-store log tail (%s): watcher.Add(%q): %v — polling", c.runID, dir, err)
		c.tailCrossStoreLogPolling(logPath, offset)
		return
	}

	pollTicker := time.NewTicker(crossStoreTailPollInterval)
	defer pollTicker.Stop()
	terminalTicker := time.NewTicker(crossStoreTerminalCheckInterval)
	defer terminalTicker.Stop()

	for {
		select {
		case <-c.closed:
			return
		case <-terminalTicker.C:
			if c.checkCrossStoreLogTerminal(logPath, &offset) {
				return
			}
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(ev.Name) != filepath.Clean(logPath) {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				offset = c.drainCrossStoreLog(logPath, offset)
			}
		case <-pollTicker.C:
			offset = c.drainCrossStoreLog(logPath, offset)
		case err := <-watcher.Errors:
			c.server.logger.Warn("runs_ws: cross-store log tail (%s): watcher error: %v", c.runID, err)
		}
	}
}

// tailCrossStoreLogPolling is the fsnotify-less fallback (rare —
// inotify is usually available on Linux hosts).
func (c *runConn) tailCrossStoreLogPolling(logPath string, offset int64) {
	pollTicker := time.NewTicker(500 * time.Millisecond)
	defer pollTicker.Stop()
	terminalTicker := time.NewTicker(crossStoreTerminalCheckInterval)
	defer terminalTicker.Stop()

	for {
		select {
		case <-c.closed:
			return
		case <-terminalTicker.C:
			if c.checkCrossStoreLogTerminal(logPath, &offset) {
				return
			}
		case <-pollTicker.C:
			offset = c.drainCrossStoreLog(logPath, offset)
		}
	}
}

// drainCrossStoreLog reads bytes appended past offset, splits into
// chunks of crossStoreLogChunkBudget, ships each as wsTypeLogChunk.
// On send failure, returns the offset advanced past whatever we
// shipped — the next loop iteration sees c.closed and exits.
//
// Truncation/rotation: if Stat shows file shorter than offset, reset
// to 0 and replay. The log stream is byte-anchored (no seq dedup), so
// the client must handle a backwards-jumping offset by re-anchoring.
func (c *runConn) drainCrossStoreLog(logPath string, offset int64) int64 {
	f, err := os.Open(logPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			c.server.logger.Warn("runs_ws: cross-store log tail (%s): open: %v", c.runID, err)
		}
		return offset
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return offset
	}
	if st.Size() < offset {
		offset = 0
	}
	if st.Size() == offset {
		return offset
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		c.server.logger.Warn("runs_ws: cross-store log tail (%s): seek %d: %v", c.runID, offset, err)
		return offset
	}

	chunk := make([]byte, crossStoreLogChunkBudget)
	for {
		n, readErr := f.Read(chunk)
		if n > 0 {
			if !c.sendEnvelope(wsTypeLogChunk, wsLogChunkPayload{
				Offset: offset,
				Text:   string(chunk[:n]),
				Total:  offset + int64(n),
			}, "") {
				return offset + int64(n)
			}
			offset += int64(n)
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				c.server.logger.Warn("runs_ws: cross-store log tail (%s): read: %v", c.runID, readErr)
			}
			return offset
		}
		if n < len(chunk) {
			return offset
		}
	}
}

// checkCrossStoreLogTerminal re-reads run.json; on a terminal status,
// drains any final bytes, sends wsTypeLogTerminated, returns true.
func (c *runConn) checkCrossStoreLogTerminal(logPath string, offset *int64) bool {
	run, err := c.xStore.LoadRun(context.Background(), c.runID)
	if err != nil {
		return false
	}
	switch run.Status {
	case store.RunStatusFinished, store.RunStatusFailed, store.RunStatusFailedResumable, store.RunStatusCancelled:
		*offset = c.drainCrossStoreLog(logPath, *offset)
		c.sendEnvelope(wsTypeLogTerminated, map[string]string{"run_id": c.runID}, "")
		return true
	}
	return false
}
