package main

// Ending an AI run on Windows: there are no process groups to signal, so taskkill /T ends the CLI with the programs
// it started (an npm install runs claude.cmd, which starts node). The CLI and its helpers get no console window.

import (
	"fmt"
	"os/exec"

	"mormicommander/internal/launch"
)

func aiProcessGroup(cmd *exec.Cmd) { launch.Hide(cmd) }

func aiHide(cmd *exec.Cmd) { launch.Hide(cmd) }

func aiKillTree(cmd *exec.Cmd, exited <-chan struct{}) {
	k := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(cmd.Process.Pid))
	launch.Hide(k)
	if k.Run() != nil {
		cmd.Process.Kill()
	}
}
