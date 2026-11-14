package main

// Tests of F-460's merge rule outside the local job engine (internal/fileops has its own): a folder copied onto a
// same-named folder is merged, never replaced, when the copy goes to or from a server (feature_net_xfer.go) or out of
// an archive (feature_vfs_jobs.go). Servers are WebDAV servers started on 127.0.0.1 over temp folders.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mergeRead reads a file for the checks below ("" when it is missing).
func mergeRead(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func TestNetCopyMergesSameNamedFolder(t *testing.T) {
	host, dir := davServer(t, "", "")
	n, _ := testNet(t, nil)
	res, err := n.Connect("webdav://" + host + "/")
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSuffix(res.Path, "/")
	vfsWrite(t, filepath.Join(dir, "d", "shared.txt"), "server-shared")
	vfsWrite(t, filepath.Join(dir, "d", "only-on-server.txt"), "server-only")
	vfsWrite(t, filepath.Join(dir, "d", "sub", "old.txt"), "server-old")

	// upload: the local folder d goes onto the server's d
	local := t.TempDir()
	vfsWrite(t, filepath.Join(local, "d", "shared.txt"), "local-shared")
	vfsWrite(t, filepath.Join(local, "d", "sub", "new.txt"), "local-new")
	id, err := n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{filepath.Join(local, "d")}, Target: root, Overwrite: "overwrite"})
	if err != nil {
		t.Fatal(err)
	}
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("upload: %+v", p)
	}
	if mergeRead(filepath.Join(dir, "d", "shared.txt")) != "local-shared" || mergeRead(filepath.Join(dir, "d", "sub", "new.txt")) != "local-new" {
		t.Error("uploaded files missing or not overwritten")
	}
	if mergeRead(filepath.Join(dir, "d", "only-on-server.txt")) != "server-only" || mergeRead(filepath.Join(dir, "d", "sub", "old.txt")) != "server-old" {
		t.Error("the server folder was replaced instead of merged")
	}

	// download: the server's d goes onto a local d that has a file of its own
	down := t.TempDir()
	vfsWrite(t, filepath.Join(down, "d", "mine.txt"), "mine")
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{root + "/d"}, Target: down, Overwrite: "overwrite"})
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("download: %+v", p)
	}
	if mergeRead(filepath.Join(down, "d", "mine.txt")) != "mine" || mergeRead(filepath.Join(down, "d", "sub", "old.txt")) != "server-old" {
		t.Error("the local folder was replaced instead of merged")
	}
}

// A file where a folder should go is one clear error; the folder's contents are not tried (and failed) one by one.
func TestNetCopyFolderBlockedByFile(t *testing.T) {
	host, dir := davServer(t, "", "")
	n, _ := testNet(t, nil)
	res, err := n.Connect("webdav://" + host + "/")
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSuffix(res.Path, "/")
	vfsWrite(t, filepath.Join(dir, "e"), "a file, not a folder")
	local := t.TempDir()
	vfsWrite(t, filepath.Join(local, "e", "x.txt"), "x")
	vfsWrite(t, filepath.Join(local, "e", "deep", "y.txt"), "y")
	id, err := n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{filepath.Join(local, "e")}, Target: root, Overwrite: "overwrite"})
	if err != nil {
		t.Fatal(err)
	}
	p := waitJob(t, n, id, nil)
	if p.State != "failed" || len(p.Errors) != 1 {
		t.Fatalf("upload: want one error, got %s %q", p.State, p.Errors)
	}
	if mergeRead(filepath.Join(dir, "e")) != "a file, not a folder" {
		t.Error("the file in the way on the server was changed")
	}

	// the other way: the server's folder f onto a local file f (the local disk answers "exists" for a file too)
	vfsWrite(t, filepath.Join(dir, "f", "x.txt"), "x")
	vfsWrite(t, filepath.Join(dir, "f", "deep", "y.txt"), "y")
	down := t.TempDir()
	vfsWrite(t, filepath.Join(down, "f"), "local file")
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{root + "/f"}, Target: down, Overwrite: "overwrite"})
	p = waitJob(t, n, id, nil)
	if p.State != "failed" || len(p.Errors) != 1 || !strings.Contains(p.Errors[0], "in the way") {
		t.Fatalf("download: want one 'in the way' error, got %s %q", p.State, p.Errors)
	}
	if mergeRead(filepath.Join(down, "f")) != "local file" {
		t.Error("the local file in the way was changed")
	}
}

func TestNetUnder(t *testing.T) {
	dirs := []string{"webdav://h/a/b", `C:\x\y\`}
	for p, want := range map[string]bool{
		"webdav://h/a/b/c":  true,
		"webdav://h/a/b":    false,
		"webdav://h/a/bc/d": false,
		`C:\x\y\z.txt`:      true,
		`C:\x\yz`:           false,
	} {
		if got := netUnder(p, dirs); got != want {
			t.Errorf("netUnder(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestVfsCopyOutOfArchiveMergesSameNamedFolder(t *testing.T) {
	_, js := vfsTestApp(t)
	dir := t.TempDir()
	vfsWrite(t, filepath.Join(dir, "d", "a.txt"), "from-archive")
	vfsWrite(t, filepath.Join(dir, "d", "sub", "b.txt"), "from-archive-b")
	arc := filepath.Join(dir, "t.zip")
	if job := vfsRun(t, js, VfsRequest{Kind: "pack", Format: "zip", Target: arc, Sources: []string{filepath.Join(dir, "d")}}); len(job.p.Errors) > 0 {
		t.Fatal(job.p.Errors)
	}
	out := filepath.Join(dir, "out")
	vfsWrite(t, filepath.Join(out, "d", "mine.txt"), "mine")
	vfsWrite(t, filepath.Join(out, "d", "sub", "keep.txt"), "keep")
	if job := vfsRun(t, js, VfsRequest{Kind: "copy", Target: out, Sources: []string{filepath.Join(arc, "d")}, Overwrite: "overwrite"}); len(job.p.Errors) > 0 {
		t.Fatal(job.p.Errors)
	}
	if mergeRead(filepath.Join(out, "d", "a.txt")) != "from-archive" || mergeRead(filepath.Join(out, "d", "sub", "b.txt")) != "from-archive-b" {
		t.Error("files from the archive missing")
	}
	if mergeRead(filepath.Join(out, "d", "mine.txt")) != "mine" || mergeRead(filepath.Join(out, "d", "sub", "keep.txt")) != "keep" {
		t.Error("the target folder was replaced instead of merged")
	}
}
