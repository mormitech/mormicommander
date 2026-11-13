package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mormicommander/internal/vfs"
)

// F5 into an archive, F7 and Shift+F6 inside one go through the panels' router, not the local disk (F-615, F-618, F-619).
func TestVfsFileopsInsideArchive(t *testing.T) {
	f, js := vfsTestApp(t)
	j := &Jobs{app: f.app}
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	vfsWrite(t, filepath.Join(src, "a.txt"), "alpha")
	arc := filepath.Join(dir, "test.zip")
	if job := vfsRun(t, js, VfsRequest{Kind: "pack", Format: "zip", Target: arc, Sources: []string{src}}); len(job.p.Errors) > 0 {
		t.Fatal(job.p.Errors)
	}
	inner := filepath.Join(arc, "src")

	// the copy dialog's target: a folder in the archive, the archive's root, a new name in the archive
	tg, err := j.ParseTarget(inner+string(filepath.Separator), 1, dir)
	if err != nil || tg.Dir != inner || tg.Rename != "" || !tg.DirExists {
		t.Fatalf("target in archive: %+v %v", tg, err)
	}
	tg, err = j.ParseTarget(arc+string(filepath.Separator), 2, dir)
	if err != nil || tg.Dir != arc || !tg.DirExists {
		t.Fatalf("archive root: %+v %v", tg, err)
	}
	tg, err = j.ParseTarget(filepath.Join(inner, "b.txt"), 1, dir)
	if err != nil || tg.Dir != inner || tg.Rename != "b.txt" {
		t.Fatalf("new name in archive: %+v %v", tg, err)
	}
	tg, err = j.ParseTarget("sub"+string(filepath.Separator), 1, inner)
	if err != nil || tg.Dir != filepath.Join(inner, "sub") || tg.DirExists {
		t.Fatalf("relative target in archive: %+v %v", tg, err)
	}
	// a local file that is in the way is still not a folder
	vfsWrite(t, filepath.Join(dir, "plain.txt"), "x")
	if _, err := j.ParseTarget(filepath.Join(dir, "plain.txt", "x")+string(filepath.Separator), 2, dir); err == nil {
		t.Error("a file as a folder should be refused")
	}

	// F7
	made, err := j.MkDir(inner, "newdir")
	if err != nil || made != "newdir" {
		t.Fatalf("mkdir in archive: %q %v", made, err)
	}
	if _, err := j.MkDir(inner, "newdir"); err == nil || !strings.Contains(err.Error(), "already there") {
		t.Errorf("mkdir twice: %v", err)
	}
	if made, err := j.MkDir(arc, "x/y"); err != nil || made != "x" {
		t.Errorf("nested mkdir at the archive root: %q %v", made, err)
	}
	// Shift+F6
	dst, err := j.Rename(filepath.Join(inner, "a.txt"), "renamed.txt")
	if err != nil || dst != filepath.Join(inner, "renamed.txt") {
		t.Fatalf("rename in archive: %q %v", dst, err)
	}
	if got := strings.Join(vfsNames(t, f, inner), ","); got != "newdir,renamed.txt" {
		t.Errorf("archive now holds %s", got)
	}
	// the archive file itself is renamed on the disk
	if dst, err := j.Rename(arc, "t2.zip"); err != nil || dst != filepath.Join(dir, "t2.zip") {
		t.Errorf("rename the archive: %q %v", dst, err)
	}
}

// Pack: a new tar-based archive gets the permissions of any new file, not those of a private temp file.
func TestVfsPackTarPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix permissions")
	}
	_, js := vfsTestApp(t)
	dir := t.TempDir()
	vfsWrite(t, filepath.Join(dir, "a.txt"), "alpha")
	probe := filepath.Join(dir, "probe")
	pf, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	pf.Close()
	want, _ := os.Stat(probe)
	arc := filepath.Join(dir, "a.tgz")
	if job := vfsRun(t, js, VfsRequest{Kind: "pack", Format: "tar.gz", Target: arc, Sources: []string{filepath.Join(dir, "a.txt")}}); len(job.p.Errors) > 0 {
		t.Fatal(job.p.Errors)
	}
	fi, err := os.Stat(arc)
	if err != nil || fi.Mode().Perm() != want.Mode().Perm() {
		t.Errorf("a.tgz mode %v, want %v (%v)", fi.Mode().Perm(), want.Mode().Perm(), err)
	}
}

// The connection layer's hooks sit on the router the panels list through (an FTP login prompt answers the session the
// panel then uses).
func TestNetHooksOnPanelRouter(t *testing.T) {
	f, _ := vfsTestApp(t)
	n, _ := testNet(t, nil)
	if got := n.router(); got != f.router {
		t.Fatal("the net feature configures a router the panels don't use")
	}
	if f.router.FTP.Configure == nil || f.router.SFTP.Configure == nil {
		t.Error("no Configure hooks on the panels' router")
	}
	if r, _ := vfs.For("sftp://h/", nil).(*vfs.Router); r != f.router {
		t.Error("sftp:// is not served by the panels' router")
	}
}
