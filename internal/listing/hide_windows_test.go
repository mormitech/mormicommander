//go:build windows

package listing

import (
	"syscall"
	"testing"
)

// markHidden gives path the hidden attribute: on Windows a leading dot alone
// does not hide a file, only FILE_ATTRIBUTE_HIDDEN does.
func markHidden(t *testing.T, path string) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(p, syscall.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
}
