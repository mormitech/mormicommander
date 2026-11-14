package netbrowse

import (
	"os/exec"
	"syscall"
)

// hide keeps the console window of a tool from flashing up.
func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
