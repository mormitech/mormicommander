package main

// A mounted volume (a share mapped to the drive bar, a USB disk) is a drive of its own: its top folder has no ".." up
// to /Volumes, and the root command goes to it rather than to "/". Folders inside it keep their ".." as before.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/vfs"
)

func TestVolumeTopHasNoParent(t *testing.T) {
	vol := t.TempDir()
	os.Mkdir(filepath.Join(vol, "flac"), 0o755)
	f := &FS{app: &App{fs: vfs.Local{}}, gen: map[string]listing{}}
	f.rootOf = func(p string) string {
		if p == vol || strings.HasPrefix(p, vol+"/") {
			return vol
		}
		return ""
	}
	l, err := f.List("left", vol)
	if err != nil || l.Parent != "" {
		t.Fatalf("volume top: parent %q, err %v; want no parent", l.Parent, err)
	}
	sub := filepath.Join(vol, "flac")
	if l, err := f.List("left", sub); err != nil || l.Parent != vol {
		t.Fatalf("inside the volume: parent %q, err %v; want %q", l.Parent, err, vol)
	}
	if r := f.Root(sub); r != vol {
		t.Fatalf("Root = %q, want the volume %q", r, vol)
	}
	// any other folder is as before
	other := t.TempDir()
	if l, _ := f.List("left", other); l.Parent != filepath.Dir(other) {
		t.Fatalf("plain folder: parent %q", l.Parent)
	}
	if r := f.Root(other); r != "/" {
		t.Fatalf("plain folder Root = %q", r)
	}
}
