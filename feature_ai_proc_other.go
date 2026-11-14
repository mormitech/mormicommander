//go:build !unix && !windows

package main

// Ending an AI run on systems that are neither Unix nor Windows: only the CLI itself can be ended.

import (
	"os"
	"os/exec"
)

func aiProcessGroup(cmd *exec.Cmd) {}

func aiHide(cmd *exec.Cmd) {}

func aiKillTree(cmd *exec.Cmd, exited <-chan struct{}) { cmd.Process.Kill() }

// aiPrivateDir says whether d is a real folder (not a link). The temporary folder is the user's own here, so that is
// all aiHome needs to check.
func aiPrivateDir(d string) bool {
	fi, err := os.Lstat(d)
	return err == nil && fi.IsDir()
}
