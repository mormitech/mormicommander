//go:build unix

package main

// Ending an AI run on macOS, Linux and the BSDs: the CLI runs in a process group of its own, so Stop can end it
// together with everything it started (an MCP helper, a node process behind an npm install) with one signal to the
// group. It gets SIGTERM first, to end cleanly, and SIGKILL when it is still there after three seconds.

import (
	"os/exec"
	"syscall"
	"time"
)

func aiProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// aiHide does nothing here: there are no console windows to hide.
func aiHide(cmd *exec.Cmd) {}

func aiKillTree(cmd *exec.Cmd, exited <-chan struct{}) {
	pid := cmd.Process.Pid
	if syscall.Kill(-pid, syscall.SIGTERM) != nil {
		cmd.Process.Kill()
		return
	}
	go func() {
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			syscall.Kill(-pid, syscall.SIGKILL)
		}
	}()
}
