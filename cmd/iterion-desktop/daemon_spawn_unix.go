//go:build desktop && !windows

package main

import (
	"os/exec"
	"syscall"
)

// detachChild puts the spawned daemon in its own session + process
// group so it survives the GUI's exit. Without Setsid the daemon would
// receive SIGHUP when the GUI terminates (default tty teardown). With
// it, the daemon becomes a session leader and the controlling terminal
// (if any) goes away cleanly, the same way `nohup … & disown` would
// behave from a shell.
func detachChild(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}
