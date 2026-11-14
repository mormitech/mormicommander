package main

// Quick view and F3 on things inside an archive (issue #141): the folder line of quick view counts a folder inside an
// archive from the archive's listing (there is nothing on the disk to stat), and a file inside an archive has a
// temporary local copy the viewer can open.

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// listerArchiveZip writes t.zip with nums.txt (10 bytes) and sub/{a.txt (3 bytes), deep/b.txt (5 bytes)}.
func listerArchiveZip(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.zip")
	zf, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	for name, body := range map[string]string{"nums.txt": "1234567890", "sub/a.txt": "abc", "sub/deep/b.txt": "hello"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zf.Close()
	return p
}

func TestListerFolderSizeInsideArchive(t *testing.T) {
	f, _ := vfsTestApp(t)
	arc := listerArchiveZip(t)
	l := &Lister{app: f.app, sessions: map[int]*listerSession{}, sizes: map[string]context.CancelFunc{}}

	r, err := l.FolderSize("quickview", filepath.Join(arc, "sub"))
	if err != nil {
		t.Fatalf("folder in archive: %v", err)
	}
	if r.Files != 2 || r.Dirs != 1 || r.Bytes != 8 {
		t.Fatalf("folder in archive counted %+v", r)
	}
	// the archive's root, as a folder inside it
	if r, err := l.FolderSize("quickview", filepath.Join(arc, "sub", "deep")); err != nil || r.Files != 1 || r.Bytes != 5 {
		t.Fatalf("deep folder: %+v %v", r, err)
	}
	// a local folder still goes through dirsize
	if r, err := l.FolderSize("quickview", filepath.Dir(arc)); err != nil || r.Files != 1 {
		t.Fatalf("local folder: %+v %v", r, err)
	}

	// the file a quick view shows: a temporary local copy with the entry's content
	local, err := f.LocalCopy(filepath.Join(arc, "nums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(local); string(b) != "1234567890" {
		t.Fatalf("local copy %q", b)
	}
}
