//go:build darwin || linux

package main

// Tests of the extended-attribute tools (feature_tools_xattr.go) in temporary folders: Properties lists the names,
// RemoveQuarantine clears the flag inside folders, XattrLossCheck says nothing for a target that keeps them and notes
// cloud folders, RemoveXattr removes one attribute of one file, and the [=mc.xattrs] column fields read the names.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	lst "mormicommander/internal/listing"
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

func TestToolsRemoveXattr(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	inner := filepath.Join(dir, "sub", "b.txt")
	toolsWrite(t, p, []byte("x"))
	toolsWrite(t, inner, []byte("y"))
	name := toolsTestXattrName()
	toolsSetXattr(t, p, name)
	toolsSetXattr(t, filepath.Join(dir, "sub"), name)
	toolsSetXattr(t, inner, name)
	tl := newTestTools()
	if _, err := tl.RemoveXattr("ftp://host/a.txt", name); err == nil {
		t.Fatal("a server path should be refused")
	}
	if _, err := tl.RemoveXattr(p, " "); err == nil {
		t.Fatal("an empty name should be refused")
	}
	ok, err := tl.RemoveXattr(p, name)
	if err != nil || !ok || xattr.Has(p, name) {
		t.Fatalf("RemoveXattr = %v, %v; still there: %v", ok, err, xattr.Has(p, name))
	}
	if ok, err := tl.RemoveXattr(p, name); err != nil || ok {
		t.Fatalf("second RemoveXattr = %v, %v", ok, err)
	}
	// a folder loses only its own attribute, not those of what is inside it
	if ok, err := tl.RemoveXattr(filepath.Join(dir, "sub"), name); err != nil || !ok {
		t.Fatalf("folder RemoveXattr = %v, %v", ok, err)
	}
	if !xattr.Has(inner, name) {
		t.Fatal("the file inside the folder lost its attribute too")
	}
}

func TestXattrColumnFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	toolsWrite(t, p, []byte("x"))
	toolsWrite(t, filepath.Join(dir, "plain.txt"), []byte("x"))
	toolsSetXattr(t, p, toolsTestXattrName())
	field := func(name string) lst.Field {
		f, ok := lst.LookupField("mc." + name)
		if !ok {
			t.Fatalf("no field mc.%s", name)
		}
		return f
	}
	it := lst.Item{Name: "a.txt", Dir: dir}
	if v := field("xattrs").Get(&it, "", nil); !strings.Contains(v.Text, toolsTestXattrName()) {
		t.Fatalf("mc.xattrs = %q", v.Text)
	}
	if v := field("xattrcount").Get(&it, "", nil); v.Kind != lst.KindNumber || v.Num < 1 {
		t.Fatalf("mc.xattrcount = %+v", v)
	}
	plain := lst.Item{Name: "plain.txt", Dir: dir}
	// macOS may add its own attributes (com.apple.provenance) to any new file, so only ours is checked here
	if v := field("xattrs").Get(&plain, "", nil); strings.Contains(v.Text, toolsTestXattrName()) {
		t.Fatalf("mc.xattrs of a plain file = %q", v.Text)
	}
	if v := field("quarantine").Get(&plain, "", nil); v.Text != "" {
		t.Fatalf("mc.quarantine of a plain file = %q", v.Text)
	}
	if runtime.GOOS == "darwin" {
		toolsSetXattr(t, p, xattr.Quarantine)
		if v := field("quarantine").Get(&it, "", nil); v.Text != "quarantined" {
			t.Fatalf("mc.quarantine = %q", v.Text)
		}
	}
	remote := lst.Item{Name: "a.txt", Dir: "sftp://host/x"}
	if v := field("xattrs").Get(&remote, "", nil); v.Text != "" {
		t.Fatalf("a server item should have no attributes read: %q", v.Text)
	}
}

func TestCloudFolder(t *testing.T) {
	home := "/Users/me"
	for in, want := range map[string]string{
		"/Users/me/Library/Mobile Documents/com~apple~CloudDocs/x":   "iCloud Drive",
		"/Users/me/Library/CloudStorage/Dropbox/a":                   "Dropbox",
		"/Users/me/Library/CloudStorage/OneDrive-Personal/a/b":       "OneDrive",
		"/Users/me/Library/CloudStorage/GoogleDrive-me@example.com/": "Google Drive",
		"/Users/me/Library/CloudStorage":                             "",
		"/Users/me/Library/Preferences":                              "",
		"/Users/me/Documents":                                        "",
		"/Users/other/Library/Mobile Documents/x":                    "",
		"/Volumes/USB": "",
	} {
		if got := cloudFolder(in, home); got != want {
			t.Errorf("cloudFolder(%q) = %q, want %q", in, got, want)
		}
	}
	if cloudFolder("/Users/me/Library/Mobile Documents/x", "") != "" {
		t.Error("no home folder: no cloud folder")
	}
}

func TestToolsXattrLossCheckCloud(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	toolsWrite(t, src, []byte("x"))
	toolsSetXattr(t, src, toolsTestXattrName())
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	l, err := newTestTools().XattrLossCheck([]string{src}, filepath.Join(home, "Library", "CloudStorage", "Dropbox", "in"))
	if err != nil {
		t.Fatal(err)
	}
	if l.Cloud != "Dropbox" || !l.Loses || l.Example != src {
		t.Fatalf("XattrLossCheck = %+v", l)
	}
	plain := filepath.Join(dir, "plain.txt")
	toolsWrite(t, plain, []byte("x"))
	if l, _ := newTestTools().XattrLossCheck([]string{plain}, filepath.Join(home, "Library", "Mobile Documents", "x")); l.Loses || l.Cloud != "iCloud Drive" {
		t.Fatalf("a source without attributes loses nothing: %+v", l)
	}
}
