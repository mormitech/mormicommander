package main

// F3 and quick view on files and folders on servers (issue #101): the Lister opens a file on a WebDAV share (the same
// path the FTP and SFTP panels take through the router) from a temporary local copy instead of saying "not ready yet",
// and F3 on a server folder counts it from the server's listings. The server is a WebDAV server on 127.0.0.1 over a
// temp folder; nothing leaves the machine.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListerOpensServerFilesAndCountsServerFolders(t *testing.T) {
	host, dir := davServer(t, "", "")
	testNet(t, nil) // registers the webdav:// scheme with the router
	f, _ := vfsTestApp(t)
	os.MkdirAll(filepath.Join(dir, "pub", "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "pub", "notes.txt"), []byte("line one\nline two\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "pub", "sub", "a.txt"), []byte("abc"), 0o644)
	os.WriteFile(filepath.Join(dir, "pub", "sub", "b.txt"), []byte("hello"), 0o644)
	base := "webdav://" + host + "/pub"

	l := &Lister{app: f.app, sessions: map[int]*listerSession{}, sizes: map[string]context.CancelFunc{}}
	t.Cleanup(l.closeAll)

	info, err := l.Open(base+"/notes.txt", ListerOptions{})
	if err != nil {
		t.Fatalf("open a server file: %v", err)
	}
	if info.Name != "notes.txt" || info.Size != 18 || strings.Contains(info.Path, "://") {
		t.Fatalf("server file opened as %+v", info)
	}
	if b, _ := os.ReadFile(info.Path); string(b) != "line one\nline two\n" {
		t.Fatalf("local copy holds %q", b)
	}
	if _, err := l.Open(base+"/missing.txt", ListerOptions{}); err == nil {
		t.Fatal("a missing server file opened")
	}

	r, err := l.FolderSize("list", base+"/sub")
	if err != nil {
		t.Fatalf("server folder size: %v", err)
	}
	if r.Files != 2 || r.Bytes != 8 || r.Dirs != 0 {
		t.Fatalf("server folder counted %+v", r)
	}
	if r, err := l.FolderSize("list", base); err != nil || r.Files != 3 || r.Dirs != 1 {
		t.Fatalf("server folder with a subfolder: %+v %v", r, err)
	}

	// a file inside an archive opens the same way (before, Lister.Open only took local paths)
	arc := listerArchiveZip(t)
	if info, err := l.Open(filepath.Join(arc, "nums.txt"), ListerOptions{}); err != nil || info.Size != 10 {
		t.Fatalf("archive entry: %+v %v", info, err)
	}
	// local files open as before
	local := filepath.Join(t.TempDir(), "x.txt")
	os.WriteFile(local, []byte("x"), 0o644)
	if info, err := l.Open(local, ListerOptions{}); err != nil || info.Path != local {
		t.Fatalf("local file: %+v %v", info, err)
	}
}
