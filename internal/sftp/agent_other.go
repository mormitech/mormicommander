//go:build !unix && !windows

// Platforms that are neither Unix nor Windows (for example js/wasm or plan9)
// have no standard SSH agent transport, so agent authentication reports a
// clear error there and the other methods are used instead.
package sftp

import "errors"

func dialAgent(socket string) (agentConn, error) {
	return nil, errors.New("sftp: SSH agent is not supported on this platform")
}
