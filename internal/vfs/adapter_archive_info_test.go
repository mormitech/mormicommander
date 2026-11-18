package vfs

import (
	stdzip "archive/zip"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPackedInfoAndComment(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "data.zip")
	makeZip(t, zp, map[string]string{"docs/a.txt": "hello hello hello hello hello hello"})
	r := newTestRouter(t)
	ctx := context.Background()

	in, err := r.Archives.PackedInfo(filepath.Join(zp, "docs", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if in.Name != "a.txt" || in.Path != "docs/a.txt" || in.Size != 35 || in.Packed <= 0 || in.Ratio < 0 || in.CRC == "" || in.Method == "" || in.MTime == 0 || !in.CanComment {
		t.Fatalf("info = %+v", in)
	}
	if err := r.Archives.SetComment(ctx, zp, "Backup of 2024 – ő"); err != nil {
		t.Fatal(err)
	}
	in, err = r.Archives.PackedInfo(zp)
	if err != nil {
		t.Fatal(err)
	}
	if in.ArcComment != "Backup of 2024 – ő" || !in.IsDir {
		t.Fatalf("after comment: %+v", in)
	}
	if got := readAll(t, r, filepath.Join(zp, "docs", "a.txt")); got != "hello hello hello hello hello hello" {
		t.Fatalf("content changed: %q", got)
	}
	// a comment set from inside a folder of the archive is the archive's comment too
	if err := r.Archives.SetComment(ctx, filepath.Join(zp, "docs"), ""); err != nil {
		t.Fatal(err)
	}
	if in, _ = r.Archives.PackedInfo(zp); in.ArcComment != "" {
		t.Fatalf("comment not removed: %q", in.ArcComment)
	}

	tp := filepath.Join(dir, "data.tar.gz")
	makeTar(t, tp, map[string]string{"x.txt": "x"})
	in, err = r.Archives.PackedInfo(filepath.Join(tp, "x.txt"))
	if err != nil || in.Size != 1 || in.CanComment || in.Method == "" {
		t.Fatalf("tar info = %+v %v", in, err)
	}
	if err := r.Archives.SetComment(ctx, tp, "no"); !errors.Is(err, ErrNoComment) {
		t.Fatalf("tar comment err = %v", err)
	}
}

func TestSetEncodingRereadsOldZipNames(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "old.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	zw := stdzip.NewWriter(f)
	w, err := zw.CreateHeader(&stdzip.FileHeader{Name: "\x8b.txt", NonUTF8: true}) // "ő.txt" in code page 852
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hu"))
	zw.Close()
	f.Close()

	r := newTestRouter(t)
	ctx := context.Background()
	if err := r.Archives.SetEncoding("cp852"); err != nil {
		t.Fatal(err)
	}
	l, err := r.List(ctx, zp)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "cp852 names", names(l), []string{"ő.txt"})
	if err := r.Archives.SetEncoding("cp437"); err != nil {
		t.Fatal(err)
	}
	l, _ = r.List(ctx, zp)
	eq(t, "cp437 names", names(l), []string{"ï.txt"})
	if err := r.Archives.SetEncoding("klingon"); err == nil {
		t.Fatal("unknown encoding accepted")
	}
}

func TestISOBrowseReadOnly(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the image is made with macOS's hdiutil")
	}
	hd, err := exec.LookPath("hdiutil")
	if err != nil {
		t.Skip("no hdiutil")
	}
	src := t.TempDir()
	makeTree(t, src, map[string]string{"Mappa/árvíz.txt": "iso data"})
	iso := filepath.Join(t.TempDir(), "disc.iso")
	if b, err := exec.Command(hd, "makehybrid", "-iso", "-joliet", "-o", iso, src).CombinedOutput(); err != nil {
		t.Skipf("hdiutil: %v %s", err, b)
	}
	r := newTestRouter(t)
	l, err := r.List(context.Background(), filepath.Join(iso, "Mappa"))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "iso folder", names(l), []string{"árvíz.txt"})
	if got := readAll(t, r, filepath.Join(iso, "Mappa", "árvíz.txt")); got != "iso data" {
		t.Fatalf("got %q", got)
	}
	ai, err := r.Archives.Info(filepath.Join(iso, "Mappa"))
	if err != nil || ai.Format != "iso" || !ai.ReadOnly {
		t.Fatalf("info %+v %v", ai, err)
	}
	if err := r.Archives.Mkdir(context.Background(), filepath.Join(iso, "new")); err == nil {
		t.Fatal("mkdir in an ISO worked")
	}
	in, err := r.Archives.PackedInfo(filepath.Join(iso, "Mappa", "árvíz.txt"))
	if err != nil || in.Size != 8 || in.Packed != 8 || in.Ratio != 100 {
		t.Fatalf("iso info %+v %v", in, err)
	}
}
