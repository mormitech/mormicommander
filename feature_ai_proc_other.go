//go:build !unix && !windows

package main

// Ending an AI run on systems that are neither Unix nor Windows: only the CLI itself can be ended.

import "os/exec"

func aiProcessGroup(cmd *exec.Cmd) {}

func aiHide(cmd *exec.Cmd) {}

func aiKillTree(cmd *exec.Cmd, exited <-chan struct{}) { cmd.Process.Kill() }
