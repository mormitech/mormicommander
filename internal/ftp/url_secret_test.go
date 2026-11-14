package ftp

import (
	"strings"
	"testing"
)

// TestParseURLErrorHidesPassword: url.Parse repeats the whole address in its error, password included.
func TestParseURLErrorHidesPassword(t *testing.T) {
	_, _, err := ParseURL("ftp://bob:s3cret@host:99x/pub")
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error %v", err)
	}
}
