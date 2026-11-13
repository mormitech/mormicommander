package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/fileops"
)

// vfsTestApp is an app without a window (events go nowhere) with the FS and VfsJobs features bound.
func vfsTestApp(t *testing.T) (*FS, *VfsJobs) {
	t.Helper()
	a := NewApp(startOpts{})
	f := newFS(a)
	t.Cleanup(func() { f.router.Close() })
	return f, &VfsJobs{app: a, jobs: map[int]*vfsJob{}}
}

func vfsWrite(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// vfsRun starts a job and waits for it; it returns the job's errors.
func vfsRun(t *testing.T, js *VfsJobs, req VfsRequest) *vfsJob {
	t.Helper()
	var job *vfsJob
	js.mu.Lock()
	js.started = func(j *vfsJob) { job = j }
	js.mu.Unlock()
	id, err := js.Start(req)
	if err != nil {
		t.Fatalf("%s: %v", req.Kind, err)
	}
	if id == 0 {
		t.Fatalf("%s: the job was not taken", req.Kind)
	}
	deadline := time.Now().Add(20 * time.Second)
	for js.Running() {
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return job
}

func vfsNames(t *testing.T, f *FS, p string) []string {
	t.Helper()
	l, err := f.List("left", p)
	if err != nil {
		t.Fatalf("list %s: %v", p, err)
	}
	var out []string
	for _, r := range l.Rows {
		out = append(out, r[0].(string))
	}
	sort.Strings(out)
	return out
}

func TestVfsPackBrowseUnpack(t *testing.T) {
	f, js := vfsTestApp(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	vfsWrite(t, filepath.Join(src, "a.txt"), "alpha")
	vfsWrite(t, filepath.Join(src, "docs", "b.txt"), "beta")

	// F-598: pack to zip and tar.gz
	for _, format := range []string{"zip", "tar.gz"} {
		arc := filepath.Join(dir, "out."+format)
		job := vfsRun(t, js, VfsRequest{Kind: "pack", Format: format, Target: arc,
			Sources: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "docs")}})
		if len(job.p.Errors) > 0 {
			t.Fatalf("pack %s: %v", format, job.p.Errors)
		}
		// F-596: the archive lists as a folder, ".." leaves it
		if got := strings.Join(vfsNames(t, f, arc), ","); got != "a.txt,docs" {
			t.Errorf("%s root: %s", format, got)
		}
		l, _ := f.List("left", filepath.Join(arc, "docs"))
		if l.Parent != arc || len(l.Rows) != 1 {
			t.Errorf("%s docs: parent %q rows %v", format, l.Parent, l.Rows)
		}
		if info := f.PathInfo(filepath.Join(arc, "docs")); info.Kind != "archive" || info.Inner != "docs" || info.ReadOnly {
			t.Errorf("%s info %+v", format, info)
		}
		// F-614: test
		job = vfsRun(t, js, VfsRequest{Kind: "test", Sources: []string{arc}})
		if len(job.p.Report) != 1 || !job.p.Report[0].OK || job.p.Summary != "No errors found in the archive(s)." {
			t.Errorf("%s test: %+v %q %v", format, job.p.Report, job.p.Summary, job.p.Errors)
		}
	}
	if f.PathInfo(dir).Kind != "local" {
		t.Error("local folder kind")
	}
	if _, err := f.List("left", filepath.Join(src, "a.txt")); err == nil || !strings.Contains(err.Error(), "is a file") {
		t.Errorf("text file listed as folder: %v", err)
	}

	// F-612/F-613: unpack two archives, each to its own subfolder, with a mask
	out := filepath.Join(dir, "unpacked")
	job := vfsRun(t, js, VfsRequest{Kind: "unpack", Target: out, Separate: true, Mask: "*.*", Overwrite: fileops.Overwrite,
		Sources: []string{filepath.Join(dir, "out.zip"), filepath.Join(dir, "out.tar.gz"), filepath.Join(src, "a.txt")}})
	if len(job.p.Errors) > 0 {
		t.Fatalf("unpack: %v", job.p.Errors)
	}
	names, _ := vfsTopNames(out)
	if strings.Join(names, ",") != "out" { // "out.zip" and "out.tar.gz" both have the stem "out"
		t.Errorf("unpack folders %v", names)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "out", "docs", "b.txt")); string(b) != "beta" {
		t.Errorf("unpacked b.txt %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(out, "out", ".mc-unpack-*")); len(left) != 0 {
		t.Errorf("staging left behind: %v", left)
	}

	// unpack with the mask and without paths
	flat := filepath.Join(dir, "flat")
	vfsRun(t, js, VfsRequest{Kind: "unpack", Target: flat, Mask: "b*", NoPaths: true, Sources: []string{filepath.Join(dir, "out.zip")}})
	if names, _ := vfsTopNames(flat); strings.Join(names, ",") != "b.txt" {
		t.Errorf("flat unpack %v", names)
	}

	// no archive at all
	if _, err := js.Start(VfsRequest{Kind: "test", Sources: []string{filepath.Join(src, "a.txt")}}); err != nil {
		t.Fatal(err)
	}
	for js.Running() {
		time.Sleep(10 * time.Millisecond)
	}
}

func TestVfsCopyIntoAndOutOfArchive(t *testing.T) {
	f, js := vfsTestApp(t)
	dir := t.TempDir()
	vfsWrite(t, filepath.Join(dir, "a.txt"), "alpha")
	vfsWrite(t, filepath.Join(dir, "tree", "c.txt"), "gamma")
	arc := filepath.Join(dir, "box.zip")
	vfsRun(t, js, VfsRequest{Kind: "pack", Format: "zip", Target: arc, Sources: []string{filepath.Join(dir, "a.txt")}})

	// plain local copies are not taken
	if id, _ := js.Start(VfsRequest{Kind: "copy", Sources: []string{filepath.Join(dir, "a.txt")}, Target: filepath.Join(dir, "x")}); id != 0 {
		t.Error("a local copy was taken")
	}

	// F-615: copy into the archive (a folder in it), one rewrite
	vfsRun(t, js, VfsRequest{Kind: "copy", Sources: []string{filepath.Join(dir, "tree")}, Target: arc})
	if got := strings.Join(vfsNames(t, f, arc), ","); got != "a.txt,tree" {
		t.Errorf("after copy in: %s", got)
	}
	// a name already there with overwrite policy "skip" keeps the old file
	vfsWrite(t, filepath.Join(dir, "other", "a.txt"), "NEW")
	job := vfsRun(t, js, VfsRequest{Kind: "copy", Overwrite: fileops.Skip, Sources: []string{filepath.Join(dir, "other", "a.txt")}, Target: arc})
	if job.p.Skipped != 1 {
		t.Errorf("skipped %d", job.p.Skipped)
	}

	// F-483: copy out of the archive, with overwrite
	out := filepath.Join(dir, "out")
	vfsWrite(t, filepath.Join(out, "a.txt"), "old")
	vfsRun(t, js, VfsRequest{Kind: "copy", Overwrite: fileops.Overwrite, Sources: []string{filepath.Join(arc, "a.txt"), filepath.Join(arc, "tree")}, Target: out})
	if b, _ := os.ReadFile(filepath.Join(out, "a.txt")); string(b) != "alpha" {
		t.Errorf("copied out a.txt %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "tree", "c.txt")); string(b) != "gamma" {
		t.Errorf("copied out c.txt %q", b)
	}
	if names, _ := vfsTopNames(out); strings.Join(names, ",") != "a.txt,tree" {
		t.Errorf("out folder %v (staging left?)", names)
	}
	// copy out under a new name (auto-rename policy)
	vfsRun(t, js, VfsRequest{Kind: "copy", Overwrite: fileops.AutoRenameSource, Sources: []string{filepath.Join(arc, "a.txt")}, Target: out})
	if _, err := os.Stat(filepath.Join(out, "a(2).txt")); err != nil {
		t.Errorf("auto-rename: %v", err)
	}

	// F6 inside the archive: a rename there
	vfsRun(t, js, VfsRequest{Kind: "move", Sources: []string{filepath.Join(arc, "a.txt")}, Target: filepath.Join(arc, "tree")})
	if got := strings.Join(vfsNames(t, f, filepath.Join(arc, "tree")), ","); got != "a.txt,c.txt" {
		t.Errorf("after move in archive: %s", got)
	}
	// F-617: delete in the archive
	vfsRun(t, js, VfsRequest{Kind: "delete", Sources: []string{filepath.Join(arc, "tree", "c.txt")}})
	if got := strings.Join(vfsNames(t, f, filepath.Join(arc, "tree")), ","); got != "a.txt" {
		t.Errorf("after delete in archive: %s", got)
	}
	// move out of the archive removes the entry
	vfsRun(t, js, VfsRequest{Kind: "move", Sources: []string{filepath.Join(arc, "tree")}, Target: filepath.Join(dir, "moved")})
	if _, err := os.Stat(filepath.Join(dir, "moved", "tree", "a.txt")); err != nil {
		t.Errorf("moved out: %v", err)
	}
	if got := vfsNames(t, f, arc); len(got) != 0 {
		t.Errorf("after move out: %v", got)
	}
	// F-619 make folder, F-618 rename, through FS
	if err := f.Mkdir(filepath.Join(arc, "new")); err != nil {
		t.Fatal(err)
	}
	if err := f.Rename(filepath.Join(arc, "new"), filepath.Join(arc, "renamed")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(vfsNames(t, f, arc), ","); got != "renamed" {
		t.Errorf("mkdir/rename: %s", got)
	}
	// Enter/F3 inside archives: a local copy
	vfsRun(t, js, VfsRequest{Kind: "copy", Sources: []string{filepath.Join(dir, "a.txt")}, Target: arc})
	local, err := f.LocalCopy(filepath.Join(arc, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(local); string(b) != "alpha" {
		t.Errorf("local copy %q", b)
	}
	if !f.IsArchiveFile(arc) || f.IsArchiveFile(filepath.Join(dir, "a.txt")) || !f.IsArchive("x.tgz") {
		t.Error("archive detection")
	}
}

func TestVfsPackSeparateAndMove(t *testing.T) {
	_, js := vfsTestApp(t)
	dir := t.TempDir()
	vfsWrite(t, filepath.Join(dir, "one.txt"), "1")
	vfsWrite(t, filepath.Join(dir, "two", "x.txt"), "2")
	job := vfsRun(t, js, VfsRequest{Kind: "pack", Format: "zip", Separate: true, Move: true, Target: filepath.Join(dir, "out", "*.zip"),
		Sources: []string{filepath.Join(dir, "one.txt"), filepath.Join(dir, "two")}})
	if len(job.p.Errors) > 0 {
		t.Fatal(job.p.Errors)
	}
	names, _ := vfsTopNames(filepath.Join(dir, "out"))
	if strings.Join(names, ",") != "one.zip,two.zip" {
		t.Errorf("separate archives %v", names)
	}
	if _, err := os.Stat(filepath.Join(dir, "one.txt")); !os.IsNotExist(err) {
		t.Error("move to archive kept the source")
	}
	// single-file packers take one file only
	job = vfsRun(t, js, VfsRequest{Kind: "pack", Format: "gz", Target: filepath.Join(dir, "x.gz"), Sources: []string{filepath.Join(dir, "out")}})
	if len(job.p.Errors) == 0 {
		t.Error("gz packed a folder")
	}
}
