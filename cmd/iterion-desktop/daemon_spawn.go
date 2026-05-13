//go:build desktop

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// spawnDaemonForProject starts a new headless daemon for the given
// project directory and returns its URL once the discovery file shows
// it's accepting connections. The child is fully detached via setsid
// so it survives the GUI's exit — that's the whole point of the
// daemon split.
//
// Logs go to ~/.iterion/daemons/<key>.log so the operator can
// post-mortem a crashed daemon without scraping syslog.
//
// Timeout is generous (20s) because cli.RunEditor's startup includes
// store migrations and file-watcher setup that can take a few seconds
// on cold caches. We poll the discovery file every 200ms so the GUI
// doesn't block on the spawn longer than necessary.
func spawnDaemonForProject(projectDir string) (string, error) {
	if projectDir == "" {
		return "", fmt.Errorf("spawnDaemonForProject: empty project dir")
	}
	self, err := selfExecPath()
	if err != nil {
		return "", fmt.Errorf("resolve self exec: %w", err)
	}
	logFile, err := openDaemonLogFile(projectDir)
	if err != nil {
		// Non-fatal: spawn anyway, daemon logs to /dev/null.
		log.Printf("desktop: daemon log open failed: %v (non-fatal)", err)
		logFile = nil
	}

	cmd := exec.Command(self, "--server-only", "--project="+projectDir)
	cmd.Env = os.Environ()
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	detachChild(cmd) // platform-specific: setsid on unix, equivalent on windows
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return "", fmt.Errorf("daemon spawn: %w", err)
	}
	// Don't wait on the child — it's intentionally orphaned. Release
	// the process handle so the GUI doesn't hold onto a zombie reaper
	// duty for the daemon's lifetime.
	go func() { _ = cmd.Process.Release() }()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if url, ok := findDaemonForProject(projectDir); ok {
			log.Printf("desktop: spawned daemon for %s at %s (pid=%d)", projectDir, url, cmd.Process.Pid)
			return url, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("daemon for %s did not come up within 20s", projectDir)
}

// selfExecPath returns an absolute path to the running iterion-desktop
// binary. os.Executable handles the "argv[0] was relative or a bare
// name" case correctly across platforms (Linux: /proc/self/exe).
func selfExecPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Clean(p), nil
}

// openDaemonLogFile opens (creating + appending) the per-project daemon
// log file at ~/.iterion/daemons/<key>.log. Returned file MUST be kept
// open by the caller (it owns the cmd's stdio); closing it would route
// the daemon's writes to a closed fd.
func openDaemonLogFile(projectDir string) (*os.File, error) {
	dir := daemonRegistryDir()
	if dir == "" {
		return nil, fmt.Errorf("no data dir resolved")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, encodeProjectDirKey(projectDir)+".log")
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}
