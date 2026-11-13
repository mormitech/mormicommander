//go:build windows

// On Windows the built-in OpenSSH agent service listens on the named pipe
// \\.\pipe\openssh-ssh-agent. A named pipe can be opened like a file, which
// gives a synchronous read/write handle that the agent protocol is happy
// with, so no extra pipe library is needed. SSH_AUTH_SOCK, when set (some
// agents such as 1Password or gpg-agent bridges set it), names another pipe.
package sftp

import (
	"os"
)

const windowsAgentPipe = `\\.\pipe\openssh-ssh-agent`

func dialAgent(socket string) (agentConn, error) {
	if socket == "" {
		socket = os.Getenv("SSH_AUTH_SOCK")
	}
	if socket == "" {
		socket = windowsAgentPipe
	}
	return os.OpenFile(socket, os.O_RDWR, 0)
}
