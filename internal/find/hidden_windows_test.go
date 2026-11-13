package find

import (
	"syscall"
	"testing"
)

// markHidden sets the hidden attribute on path. In local Windows folders a leading dot does not make a file hidden
// (Explorer and Total Commander show ".git"); only the attribute does.
func markHidden(t *testing.T, path string) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(p, attrs|syscall.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
}
