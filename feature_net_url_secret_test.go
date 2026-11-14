package main

import (
	"strings"
	"testing"
)

// A server address typed with its password and a broken host name must not repeat the password in the error.
func TestNetParseURLErrorHidesPassword(t *testing.T) {
	_, err := netParseURL("ftp://bob:s3cret@[::1/pub")
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error %v", err)
	}
}
