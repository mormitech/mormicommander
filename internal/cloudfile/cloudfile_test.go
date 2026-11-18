package cloudfile

import (
	"os"
	"path/filepath"
	"testing"
)

// An ordinary file on disk is not a placeholder, and asking never fails on a missing path.
func TestOrdinaryFileIsNoPlaceholder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	if Placeholder(info) || PathPlaceholder(p) || PathPlaceholder(p+".missing") || Placeholder(nil) {
		t.Fatal("an ordinary or missing file counted as a cloud placeholder")
	}
}
