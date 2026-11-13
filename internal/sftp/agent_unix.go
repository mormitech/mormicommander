//go:build unix

// On macOS and Linux the SSH agent listens on a Unix domain socket whose path
// is published in the SSH_AUTH_SOCK environment variable (macOS starts one per
// login session through launchd). Connecting is a plain socket dial.
package sftp

import (
	"errors"
	"net"
	"os"
)

func dialAgent(socket string) (agentConn, error) {
	if socket == "" {
		socket = os.Getenv("SSH_AUTH_SOCK")
	}
	if socket == "" {
		return nil, errors.New("sftp: no SSH agent (SSH_AUTH_SOCK is not set)")
	}
	return net.Dial("unix", socket)
}
