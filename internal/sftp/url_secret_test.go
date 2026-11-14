package sftp

import (
	"strings"
	"testing"
)

// TestParseURLErrorHidesPassword: url.Parse repeats the whole address in its error, password included.
func TestParseURLErrorHidesPassword(t *testing.T) {
	_, err := ParseURL("sftp://bob:s3cret@host:99x/home")
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error %v", err)
	}
}
