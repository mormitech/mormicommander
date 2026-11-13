package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mormicommander/internal/vfs"
)

func TestFSList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644)
	f := newFS(NewApp(startOpts{}))
	l, err := f.List("left", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Rows) != 1 || l.Rows[0][0] != "a.txt" || l.Rows[0][1] != int64(3) {
		t.Errorf("rows %v", l.Rows)
	}
	_, err = f.List("left", filepath.Join(dir, "missing"))
	if err == nil || !strings.Contains(err.Error(), "is not there") {
		t.Errorf("missing folder: %v", err)
	}
	_, err = f.List("left", filepath.Join(dir, "a.txt"))
	if err == nil || !strings.Contains(err.Error(), "is a file") {
		t.Errorf("file: %v", err)
	}
	if len(f.gen) != 0 {
		t.Error("listing left a cancel behind")
	}
}

func TestFSDemo(t *testing.T) {
	f := newFS(NewApp(startOpts{Demo: true}))
	if f.Home() != vfs.MockHome || f.Separator() != "/" {
		t.Error("demo home")
	}
	l, err := f.List("x", "~")
	if err != nil || l.Path != vfs.MockHome {
		t.Fatalf("%v %v", l.Path, err)
	}
	if v := f.VolumeOf("/Volumes/USB stick/Video.mp4"); v.Name != "USB stick" {
		t.Errorf("volume %+v", v)
	}
	if v := f.VolumeOf(vfs.MockHome + "/Documents"); v.Name != "Macintosh HD" {
		t.Errorf("volume %+v", v)
	}
	if f.LinkTarget(vfs.MockHome+"/Latest photos") == "" {
		t.Error("demo link has no target")
	}
	if err := f.OpenFile("/x"); err == nil {
		t.Error("demo opened a file")
	}
}

func TestPathWithin(t *testing.T) {
	if runtime.GOOS == "windows" {
		if !pathWithin(`c:\users`, `C:\`) {
			t.Error("drive")
		}
		return
	}
	for _, c := range []struct {
		p, root string
		want    bool
	}{{"/a/b", "/", true}, {"/Volumes/X/y", "/Volumes/X", true}, {"/Volumes/XY", "/Volumes/X", false}, {"/Volumes/X", "/Volumes/X", true}} {
		if got := pathWithin(c.p, c.root); got != c.want {
			t.Errorf("%s in %s = %v", c.p, c.root, got)
		}
	}
}

func TestVolumeOfKeepsOrder(t *testing.T) {
	f := newFS(NewApp(startOpts{Demo: true}))
	before := f.Volumes()[0].Name
	f.VolumeOf("/Volumes/Backup")
	if f.Volumes()[0].Name != before {
		t.Error("VolumeOf changed the order of the volumes")
	}
}
