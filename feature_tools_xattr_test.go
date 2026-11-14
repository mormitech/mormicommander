//go:build darwin || linux

package main

// Tests of the extended-attribute tools (feature_tools_xattr.go) in temporary folders: Properties lists the names,
// RemoveQuarantine clears the flag inside folders, and XattrLossCheck says nothing for a target that keeps them.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"

	"mormicommander/internal/xattr"
)

// toolsSetXattr sets an attribute or skips the test where the temporary folder's file system has none.
func toolsSetXattr(t *testing.T, p, name string) {
	t.Helper()
	if err := unix.Lsetxattr(p, name, []byte("v"), 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			t.Skip("no extended attributes here")
		}
		t.Fatal(err)
	}
}

func toolsTestXattrName() string {
	if runtime.GOOS == "linux" {
		return "user.mc.tag"
	}
	return "com.mormitech.test.tag"
}

func TestToolsPropertiesListsXattrs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.txt")
	toolsWrite(t, p, []byte("x"))
	toolsSetXattr(t, p, toolsTestXattrName())
	pr, err := newTestTools().Properties(p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range pr.Xattrs {
		found = found || a.Name == toolsTestXattrName()
	}
	if !found {
		t.Fatalf("Xattrs = %+v", pr.Xattrs)
	}
}

func TestToolsRemoveQuarantine(t *testing.T) {
	tl := newTestTools()
	if _, err := tl.RemoveQuarantine("q", []string{"ftp://host/x"}); err == nil {
		t.Fatal("a server path should be refused")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "App", "Contents", "bin")
	toolsWrite(t, f, []byte("x"))
	if runtime.GOOS != "darwin" {
		// Linux keeps no com.apple.* attributes: nothing is there, nothing fails
		r, err := tl.RemoveQuarantine("q", []string{filepath.Join(dir, "App")})
		if err != nil || r.Removed != 0 || len(r.Failed) != 0 || r.Checked != 3 {
			t.Fatalf("RemoveQuarantine = %+v, %v", r, err)
		}
		return
	}
	toolsSetXattr(t, f, xattr.Quarantine)
	toolsSetXattr(t, filepath.Join(dir, "App"), xattr.Quarantine)
	r, err := tl.RemoveQuarantine("q", []string{filepath.Join(dir, "App")})
	if err != nil || r.Removed != 2 || r.Checked != 3 || len(r.Failed) != 0 || r.Canceled {
		t.Fatalf("RemoveQuarantine = %+v, %v", r, err)
	}
	if xattr.Has(f, xattr.Quarantine) {
		t.Fatal("the flag is still there")
	}
}

func TestToolsXattrLossCheck(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	toolsWrite(t, src, []byte("x"))
	toolsSetXattr(t, src, toolsTestXattrName())
	tl := newTestTools()
	// the temporary folder is on the system's own disk (APFS, ext4, tmpfs …), which keeps extended attributes
	l, err := tl.XattrLossCheck([]string{src}, filepath.Join(dir, "not", "made", "yet"))
	if err != nil {
		t.Fatal(err)
	}
	if l.FileSystem == "" {
		t.Fatal("the target's file system was not found")
	}
	if xattr.DropsXattrs(l.FileSystem) != l.Loses {
		t.Fatalf("XattrLossCheck = %+v", l)
	}
	if l, _ := tl.XattrLossCheck(nil, dir); l.Loses || l.FileSystem != "" {
		t.Fatalf("no sources: %+v", l)
	}
	if _, err := os.Stat(filepath.Join(dir, "not")); err == nil {
		t.Fatal("the check made the target folder")
	}
}
