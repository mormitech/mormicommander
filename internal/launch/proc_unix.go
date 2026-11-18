//go:build unix

// Process start-up on macOS, Linux and the BSDs. Programs opened from the file manager are put into their own
// session (setsid), so they keep running when mormiCommander quits and do not receive the Ctrl+C or hang-up signals
// meant for the terminal mormiCommander itself may have been started from. Helper programs whose output is captured
// stay in our session so a cancelled context can stop them. There are no console windows to hide here.
package launch

import (
	"os/exec"
	"syscall"
)

// Hide does nothing outside Windows: helper programs have no window to hide. It exists so callers need no
// platform checks.
func Hide(cmd *exec.Cmd) {}

func applyPlatform(cmd *exec.Cmd, c Command, detached bool) {
	if detached {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
}

func shellExecute(c Command) (bool, error) { return false, nil }

func shortName(path string) string { return path }

// ownGroup puts a captured command into a session of its own (no terminal to read a password from, and a process
// group to kill as a whole), and makes cancelling its context kill that whole group, children included.
func ownGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
