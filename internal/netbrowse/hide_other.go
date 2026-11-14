//go:build !windows

package netbrowse

import "os/exec"

func hide(cmd *exec.Cmd) {}
