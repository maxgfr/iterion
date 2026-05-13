//go:build desktop && windows

package main

import (
	"os/exec"
	"syscall"
)

// detachChild is the Windows variant of the Unix setsid trick: setting
// CREATE_NEW_PROCESS_GROUP + DETACHED_PROCESS keeps the daemon alive
// after the GUI exits and prevents ctrl-c from the GUI's console (when
// run from a terminal) from cascading to the daemon.
func detachChild(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= 0x00000200 // CREATE_NEW_PROCESS_GROUP
	cmd.SysProcAttr.CreationFlags |= 0x00000008 // DETACHED_PROCESS
}
