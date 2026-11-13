// Small helpers shared by several test files.
package sftp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"runtime"
	"testing"

	"golang.org/x/crypto/ssh"
)

func isWindows() bool { return runtime.GOOS == "windows" }

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
