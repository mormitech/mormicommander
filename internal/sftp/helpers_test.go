// Small helpers shared by several test files.
package sftp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/crypto/ssh"
)

func isWindows() bool { return runtime.GOOS == "windows" }

// remotePath turns a local path into the form an SFTP server running on
// this machine uses for it. SFTP paths are always slash-separated and
// absolute paths start with "/", so on Windows C:\x is "/C:/x" (what
// pkg/sftp and OpenSSH for Windows report); elsewhere the two are the same.
func remotePath(local string) string {
	if isWindows() {
		return "/" + filepath.ToSlash(local)
	}
	return local
}

// localPath is the inverse of remotePath, used by the test server to find
// the file a remote path names.
func localPath(remote string) string {
	if isWindows() && len(remote) >= 3 && remote[0] == '/' && remote[2] == ':' {
		remote = remote[1:]
	}
	return filepath.FromSlash(remote)
}

func newECDSAPublic(t *testing.T) (ssh.PublicKey, ssh.Signer) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return s.PublicKey(), s
}
