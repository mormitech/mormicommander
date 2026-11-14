package main

// Board #143: archives that can't be changed (a 7z, which is only read here, and an archive entered from inside
// another archive) say so plainly. PathInfo reports an entered inner archive as read-only and nested, and F5, F6, F7
// and F8 there are refused up front with the reason, instead of asking questions first and failing vaguely later.

import (
	"archive/zip"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vfsZip writes a zip file at p holding the given name/content pairs.
func vfsZip(t *testing.T, p string, files map[string][]byte) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	out, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for n, b := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()
}

func vfsSum(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", md5.Sum(b))
}

func TestVfsReadOnlyArchivesSayWhy(t *testing.T) {
	f, js := vfsTestApp(t)
	j := &Jobs{app: f.app}
	dir := t.TempDir()
	vfsWrite(t, filepath.Join(dir, "x.txt"), "x")

	// a zip inside a zip: entering the inner one shows it as a read-only, nested archive
	var inner strings.Builder
	{
		tmp := filepath.Join(dir, "build", "src.zip")
		vfsZip(t, tmp, map[string][]byte{"a.txt": []byte("alpha")})
		b, _ := os.ReadFile(tmp)
		inner.Write(b)
	}
	nest := filepath.Join(dir, "nest.zip")
	vfsZip(t, nest, map[string][]byte{"src.zip": []byte(inner.String())})
	srcZip := filepath.Join(nest, "src.zip")
	if got := strings.Join(vfsNames(t, f, srcZip), ","); got != "a.txt" {
		t.Fatalf("inner archive lists %q", got)
	}
	info := f.PathInfo(srcZip)
	if info.Kind != "archive" || !info.ReadOnly || !info.Nested || info.Inner != "" || info.Archive != srcZip || !strings.Contains(info.ReadOnlyWhy, "inside another archive") {
		t.Errorf("PathInfo of the entered inner archive: %+v", info)
	}
	// the outer archive itself is still writable, and the inner archive file is an entry of it
	if info := f.PathInfo(nest); info.ReadOnly || info.Nested || info.ReadOnlyWhy != "" {
		t.Errorf("PathInfo of the outer archive: %+v", info)
	}
	// the copy dialog's target for the panel that shows it is that archive's root, not "a file, not a folder"
	if tg, err := j.ParseTarget(srcZip+string(filepath.Separator), 1, dir); err != nil || tg.Dir != srcZip || !tg.DirExists {
		t.Errorf("copy target in the inner archive: %+v %v", tg, err)
	}
	sum := vfsSum(t, nest)
	check := func(what string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "inside another archive") {
			t.Errorf("%s: %v", what, err)
		}
	}
	_, err := js.Start(VfsRequest{Kind: "delete", Sources: []string{filepath.Join(srcZip, "a.txt")}})
	check("F8 in the inner archive", err)
	_, err = js.Start(VfsRequest{Kind: "copy", Target: srcZip, Sources: []string{filepath.Join(dir, "x.txt")}})
	check("F5 into the inner archive", err)
	_, err = js.Start(VfsRequest{Kind: "move", Target: srcZip, Sources: []string{filepath.Join(dir, "x.txt")}})
	check("F6 into the inner archive", err)
	_, err = j.MkDir(srcZip, "new")
	check("F7 in the inner archive", err)
	if vfsSum(t, nest) != sum {
		t.Error("the outer archive changed")
	}
	// copying out of it still works
	if job := vfsRun(t, js, VfsRequest{Kind: "copy", Target: filepath.Join(dir, "out"), Sources: []string{filepath.Join(srcZip, "a.txt")}}); len(job.p.Errors) > 0 {
		t.Errorf("copy out of the inner archive: %v", job.p.Errors)
	}

	// a 7z can be read, not changed
	b, err := os.ReadFile(filepath.Join("internal", "archive", "sevenrar", "testdata", "7z", "lzma2.7z"))
	if err != nil {
		t.Skip("7z sample not available:", err)
	}
	sz := filepath.Join(dir, "s.7z")
	if err := os.WriteFile(sz, b, 0o644); err != nil {
		t.Fatal(err)
	}
	names := vfsNames(t, f, sz)
	if len(names) == 0 {
		t.Fatal("the 7z lists nothing")
	}
	if info := f.PathInfo(sz); !info.ReadOnly || info.Nested || info.Format != "7z" || !strings.Contains(info.ReadOnlyWhy, "(7Z)") {
		t.Errorf("PathInfo of the 7z: %+v", info)
	}
	sum = vfsSum(t, sz)
	check = func(what string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "archive type (7Z) is read-only") {
			t.Errorf("%s: %v", what, err)
		}
	}
	_, err = js.Start(VfsRequest{Kind: "delete", Sources: []string{filepath.Join(sz, names[0])}})
	check("F8 in the 7z", err)
	_, err = js.Start(VfsRequest{Kind: "copy", Target: sz, Sources: []string{filepath.Join(dir, "x.txt")}})
	check("F5 into the 7z", err)
	_, err = j.MkDir(sz, "new")
	check("F7 in the 7z", err)
	if vfsSum(t, sz) != sum {
		t.Error("the 7z changed")
	}
}
