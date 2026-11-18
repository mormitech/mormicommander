package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// F-628: convert archives to another format; F-847/F-622: info on a packed file and the zip comment.
func TestVfsConvertArchive(t *testing.T) {
	f, js := vfsTestApp(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	vfsWrite(t, filepath.Join(src, "a.txt"), "alpha")
	vfsWrite(t, filepath.Join(src, "docs", "b.txt"), "beta")
	zip := filepath.Join(dir, "photos.zip")
	if job := vfsRun(t, js, VfsRequest{Kind: "pack", Format: "zip", Target: zip,
		Sources: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "docs")}}); len(job.p.Errors) > 0 {
		t.Fatal(job.p.Errors)
	}
	out := filepath.Join(dir, "out")
	job := vfsRun(t, js, VfsRequest{Kind: "convert", Format: "tar.gz", Target: out, Sources: []string{zip}})
	if len(job.p.Errors) > 0 {
		t.Fatalf("convert: %v", job.p.Errors)
	}
	tgz := filepath.Join(out, "photos.tar.gz")
	if got := strings.Join(vfsNames(t, f, tgz), ","); got != "a.txt,docs" {
		t.Fatalf("converted root: %s", got)
	}
	if got := strings.Join(vfsNames(t, f, filepath.Join(tgz, "docs")), ","); got != "b.txt" {
		t.Fatalf("converted docs: %s", got)
	}
	if _, err := os.Stat(zip); err != nil {
		t.Fatal("the source archive is gone")
	}
	// a second run does not overwrite what is there
	job = vfsRun(t, js, VfsRequest{Kind: "convert", Format: "tar.gz", Target: out, Sources: []string{zip}})
	if len(job.p.Errors) != 1 || !strings.Contains(job.p.Errors[0], "already there") {
		t.Fatalf("second convert: %v", job.p.Errors)
	}
	// a single-file packer refuses an archive of several files
	job = vfsRun(t, js, VfsRequest{Kind: "convert", Format: "gz", Target: out, Sources: []string{zip}})
	if len(job.p.Errors) != 1 || !strings.Contains(job.p.Errors[0], "one file only") {
		t.Fatalf("gz convert: %v", job.p.Errors)
	}

	// info on packed file and the archive comment
	in, err := f.PackedInfo(filepath.Join(zip, "docs", "b.txt"))
	if err != nil || in.Size != 4 || in.CRC == "" || !in.CanComment {
		t.Fatalf("info %+v %v", in, err)
	}
	if err := f.SetArchiveComment(zip, "hello"); err != nil {
		t.Fatal(err)
	}
	if in, _ = f.PackedInfo(zip); in.ArcComment != "hello" {
		t.Fatalf("comment %q", in.ArcComment)
	}
	if err := f.SetArchiveComment(tgz, "x"); err == nil {
		t.Fatal("a tar.gz took a comment")
	}
	if _, err := f.PackedInfo(filepath.Join(dir, "src", "a.txt")); err == nil {
		t.Fatal("info on a plain file")
	}
}
