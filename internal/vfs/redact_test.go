package vfs

import (
	"strings"
	"testing"
)

// TestParseRemoteErrorHidesPassword: a URL typed with its password ("ftp://bob:s3cret@host:99999/") that can't be
// parsed must not repeat the password in the error, which the panel shows and the logs keep.
func TestParseRemoteErrorHidesPassword(t *testing.T) {
	for _, p := range []string{
		"ftp://bob:s3cret@host:99999/pub",
		"ftp://bob:s3cret@[::1/pub",
		"sftp://bob:s3cret@/pub",
		"ftp://bob:s3cret@host:x",
	} {
		_, err := parseRemote(p)
		if err == nil {
			t.Fatalf("%q parsed", p)
		}
		if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("error shows the password: %v", err)
		}
		if !strings.Contains(err.Error(), "bob") {
			t.Errorf("error lost the rest of the address: %v", err)
		}
	}
}
