//go:build !unix && !windows

// Process start-up on platforms that are neither Unix nor Windows (Plan 9, WebAssembly). Nothing special is done:
// programs are started plainly, if the platform can start programs at all.
package launch

import "os/exec"

// Hide does nothing on this platform.
func Hide(cmd *exec.Cmd) {}

func applyPlatform(cmd *exec.Cmd, c Command, detached bool) {}

func shellExecute(c Command) (bool, error) { return false, nil }

func shortName(path string) string { return path }
