//go:build !windows

package imgconv

import "os/exec"

func hideWindow(*exec.Cmd) {}
