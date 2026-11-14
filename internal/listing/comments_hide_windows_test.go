// This file checks that saving comments on Windows leaves descript.ion with the hidden attribute, also when an
// existing visible file is rewritten, as Total Commander keeps it.

//go:build windows

package listing

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSaveCommentsMarksFileHidden(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, CommentFileName)
	if err := os.WriteFile(p, []byte("a.txt old\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewComments()
	c.Set("a.txt", "new")
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	u, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		t.Fatal(err)
	}
	a, err := syscall.GetFileAttributes(u)
	if err != nil {
		t.Fatal(err)
	}
	if a&syscall.FILE_ATTRIBUTE_HIDDEN == 0 {
		t.Fatalf("attributes = %#x, want hidden", a)
	}
}
