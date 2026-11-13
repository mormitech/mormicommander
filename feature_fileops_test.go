package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestJobs(t *testing.T) *Jobs {
	t.Helper()
	return &Jobs{app: NewApp(startOpts{})}
}

func TestFileopsParseTarget(t *testing.T) {
	j := newTestJobs(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	os.Mkdir(src, 0o755)
	os.Mkdir(filepath.Join(dir, "dst"), 0o755)

	tg, err := j.ParseTarget(filepath.Join(dir, "dst"), 2, src)
	if err != nil || tg.Dir != filepath.Join(dir, "dst") || tg.Rename != "" || !tg.DirExists || !tg.SameVolume {
		t.Errorf("existing folder: %+v %v", tg, err)
	}
	tg, _ = j.ParseTarget(filepath.Join(dir, "dst", "b.txt"), 1, src)
	if tg.Dir != filepath.Join(dir, "dst") || tg.Rename != "b.txt" {
		t.Errorf("new name: %+v", tg)
	}
	tg, _ = j.ParseTarget(filepath.Join(dir, "dst", "*.bak"), 3, src)
	if tg.Rename != "*.bak" {
		t.Errorf("mask: %+v", tg)
	}
	tg, _ = j.ParseTarget("sub"+string(filepath.Separator), 1, src)
	if tg.Dir != filepath.Join(src, "sub") || tg.Rename != "" || tg.DirExists {
		t.Errorf("relative folder: %+v", tg)
	}
	tg, _ = j.ParseTarget("copy.txt", 1, src)
	if tg.Dir != src || tg.Rename != "copy.txt" {
		t.Errorf("relative name: %+v", tg)
	}
	if _, err := j.ParseTarget("  ", 1, src); err == nil {
		t.Error("empty target accepted")
	}
	os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o644)
	if _, err := j.ParseTarget(filepath.Join(dir, "file", "x")+string(filepath.Separator), 2, src); err == nil {
		t.Error("a file as a folder was accepted")
	}
}

func TestFileopsMkDir(t *testing.T) {
	j := newTestJobs(t)
	dir := t.TempDir()
	first, err := j.MkDir(dir, "a/b/c")
	if err != nil || first != "a" {
		t.Fatalf("nested: %q %v", first, err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "a", "b", "c")); err != nil || !fi.IsDir() {
		t.Error("nested folder not made")
	}
	if _, err := j.MkDir(dir, "a"); err == nil || !strings.Contains(err.Error(), "already there") {
		t.Errorf("existing folder: %v", err)
	}
	if _, err := j.MkDir(dir, "  "); err == nil {
		t.Error("empty name accepted")
	}
	name, err := j.MkDir(dir, "log-[Y]")
	if err != nil || name != "log-"+time.Now().Format("2006") {
		t.Errorf("placeholder: %q %v", name, err)
	}
	if got := fileopsPlaceholders("[Y]-[M]-[D] [h].[m].[s]", time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local)); got != "2026-03-04 05.06.07" {
		t.Errorf("placeholders: %q", got)
	}
	demo := &Jobs{app: NewApp(startOpts{Demo: true})}
	if _, err := demo.MkDir(dir, "x"); err == nil {
		t.Error("the demo made a folder")
	}
}

func TestFileopsRename(t *testing.T) {
	j := newTestJobs(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	if _, err := j.Rename(a, "b.txt"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("rename onto another file: %v", err)
	}
	if _, err := j.Rename(a, "x/y"); err == nil {
		t.Error("a name with a slash was accepted")
	}
	p, err := j.Rename(a, "A.txt")
	if err != nil || filepath.Base(p) != "A.txt" {
		t.Fatalf("case-only rename: %q %v", p, err)
	}
	names, _ := os.ReadDir(dir)
	found := false
	for _, n := range names {
		if n.Name() == "A.txt" {
			found = true
		}
	}
	if !found {
		t.Error("A.txt not there after the rename")
	}
}

func TestFileopsSentence(t *testing.T) {
	if got := fileopsSentence("nothing is selected"); got != "Nothing is selected." {
		t.Errorf("%q", got)
	}
}
