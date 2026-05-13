//go:build desktop

package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// desktopURLFileName is the discovery file written into the iterion data
// dir on every server (re)start. External processes that want to drive
// the desktop programmatically — e.g. the iterion-control MCP server —
// read it to find the loopback HTTP API endpoint, which would otherwise
// be unreachable (random ephemeral port + Wails-only binding).
const desktopURLFileName = "desktop.json"

// desktopURLState is the on-disk shape of the discovery file.
type desktopURLState struct {
	URL       string `json:"url"`
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
}

// writeDesktopURLFile atomically updates ~/.iterion/desktop.json with
// the current server URL, the desktop PID, and the timestamp. Atomic
// rename means external readers either see the previous URL or the new
// one — never a half-written file.
//
// Failures are non-fatal: the desktop launches normally even if the
// discovery file can't be written (e.g. read-only home), the only
// affected feature is autonomous control via MCP.
func writeDesktopURLFile(url string) {
	dir := desktopDataDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	state := desktopURLState{
		URL:       url,
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	target := filepath.Join(dir, desktopURLFileName)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, target)
}

// removeDesktopURLFile clears the discovery file on shutdown so MCP
// callers don't try to hit a dead port. Failure is silent — a stale
// file is harmless (the MCP server will get a connection error and
// surface it).
func removeDesktopURLFile() {
	dir := desktopDataDir()
	if dir == "" {
		return
	}
	_ = os.Remove(filepath.Join(dir, desktopURLFileName))
}

// detectDaemonURL reads ~/.iterion/desktop.json and returns its URL if
// the recorded pid is still alive AND the TCP port still accepts a
// connection. This is the GUI's "is a headless daemon already running?"
// probe: when true, the GUI attaches to the daemon's server instead of
// spawning its own, so runs survive GUI close/rebuild/relaunch cycles.
//
// We deliberately combine the pid check (kill(pid, 0)) with a tiny
// TCP dial: pid-only checks let a half-crashed daemon (kernel hasn't
// reaped yet) appear alive, while dial-only checks would attach to a
// totally unrelated process that grabbed the same port after a crash.
// Both must agree → false positives are vanishingly rare.
func detectDaemonURL() (string, bool) {
	dir := desktopDataDir()
	if dir == "" {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(dir, desktopURLFileName))
	if err != nil {
		return "", false
	}
	var st desktopURLState
	if err := json.Unmarshal(raw, &st); err != nil {
		return "", false
	}
	if st.URL == "" || st.PID == 0 {
		return "", false
	}
	// Signal 0 doesn't deliver — it just checks whether the kernel still
	// has a process slot for that pid. Useful as a liveness probe that
	// works for processes we don't own.
	proc, err := os.FindProcess(st.PID)
	if err != nil {
		return "", false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return "", false
	}
	// TCP probe — strip the scheme/path off st.URL so we can dial the
	// raw host:port.
	hostPort := strings.TrimPrefix(strings.TrimPrefix(st.URL, "http://"), "https://")
	hostPort = strings.TrimRight(hostPort, "/")
	conn, err := net.DialTimeout("tcp", hostPort, 500*time.Millisecond)
	if err != nil {
		return "", false
	}
	_ = conn.Close()
	return st.URL, true
}

// desktopDataDir mirrors store.globalIterionDataDir without importing
// the store package (which would pull the runtime in via init chains).
// Resolution order: $ITERION_HOME → $HOME/.iterion → "" (skip the
// discovery file).
func desktopDataDir() string {
	if dir := strings.TrimRight(os.Getenv("ITERION_HOME"), string(filepath.Separator)); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".iterion")
	}
	return ""
}
