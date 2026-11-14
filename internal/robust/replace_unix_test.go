//go:build darwin || linux

// Unix tests for Replace: extended attributes and the group of the old file survive the swap. File systems without
// user attributes and users with a single group skip the matching test.
package robust

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReplaceKeepsXattrs(t *testing.T) {
	dir := t.TempDir()
	tmp, target := filepath.Join(dir, "tmp"), filepath.Join(dir, "a.zip")
	writeFile(t, target, "old")
	name := "user.mormi.test"
	if err := unix.Setxattr(target, name, []byte("tag"), 0); err != nil {
		t.Skip("no extended attributes here:", err)
	}
	writeFile(t, tmp, "new")
	if err := Replace(tmp, target); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, err := unix.Getxattr(target, name, buf)
	if err != nil || string(buf[:n]) != "tag" {
		t.Fatalf("attribute lost: %v", err)
	}
	if readFile(t, target) != "new" {
		t.Fatal("content not replaced")
	}
}

func TestReplaceKeepsGroup(t *testing.T) {
	dir := t.TempDir()
	tmp, target := filepath.Join(dir, "tmp"), filepath.Join(dir, "a.zip")
	writeFile(t, target, "old")
	writeFile(t, tmp, "new")
	fi, _ := os.Stat(tmp)
	def := int(fi.Sys().(*syscall.Stat_t).Gid)
	groups, _ := os.Getgroups()
	other := -1
	for _, g := range groups {
		if g != def && os.Chown(target, -1, g) == nil {
			other = g
			break
		}
	}
	if other < 0 {
		t.Skip("no second group to test with")
	}
	if err := Replace(tmp, target); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(target)
	if got := int(fi.Sys().(*syscall.Stat_t).Gid); got != other {
		t.Fatalf("group %d, want %d", got, other)
	}
}
