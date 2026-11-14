package main

import (
	"archive/tar"
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Unpacking without paths, two members with one name used to collapse into one file in the staging folder before the
// merge question (#149). A zip keeps both (the later under a free name); a tar skips the later one and says so.
func TestVfsExtractNoPathsKeepsSameNames(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "two.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	for _, n := range []string{"d1/x.txt", "d2/x.txt"} {
		w, _ := zw.Create(n)
		w.Write([]byte(n))
	}
	zw.Close()
	f.Close()
	out := filepath.Join(dir, "zout")
	if _, problems, err := vfsExtract(context.Background(), zp, out, vfsExtractOpts{NoPaths: true}); err != nil || len(problems) > 0 {
		t.Fatalf("zip: %v %v", err, problems)
	}
	var got []string
	des, _ := os.ReadDir(out)
	for _, d := range des {
		b, _ := os.ReadFile(filepath.Join(out, d.Name()))
		got = append(got, string(b))
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "d1/x.txt,d2/x.txt" {
		t.Fatalf("zip without paths kept %v (%d files), want both contents", got, len(des))
	}

	tp := filepath.Join(dir, "two.tar")
	f, _ = os.Create(tp)
	tw := tar.NewWriter(f)
	for _, n := range []string{"d1/x.txt", "d2/x.txt"} {
		tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(n)), Typeflag: tar.TypeReg})
		tw.Write([]byte(n))
	}
	tw.Close()
	f.Close()
	out = filepath.Join(dir, "tout")
	_, problems, err := vfsExtract(context.Background(), tp, out, vfsExtractOpts{NoPaths: true})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "x.txt")); string(b) != "d1/x.txt" || len(problems) != 1 {
		t.Fatalf("tar without paths: x.txt = %q, problems %v; want the first kept and the second reported", b, problems)
	}
}
