// Tests for the router itself: classifying paths, Enter decisions, the
// local read/write operations, scheme registration, unknown schemes and the
// capability fallbacks. Archive, FTP and SFTP behaviour is covered by the
// adapter tests.
package vfs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRouterKinds(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	makeZip(t, zp, map[string]string{"in/x.txt": "x"})
	r := newTestRouter(t)
	tests := []struct {
		p    string
		want Kind
	}{
		{dir, KindLocal},
		{filepath.Join(dir, "missing", "deeper"), KindLocal},
		{zp, KindLocal},
		{filepath.Join(zp, "in"), KindArchive},
		{filepath.Join(zp, "in", "not-yet"), KindArchive},
		{"ftp://host/x", KindFTP},
		{"FTPES://host/x", KindFTP},
		{"ftps://host/x", KindFTP},
		{"sftp://u@host/x", KindSFTP},
		{"scp://u@host/x", KindSFTP},
		{"gopher://host/x", KindUnknown},
	}
	for _, tt := range tests {
		if got := r.Kind(tt.p); got != tt.want {
			t.Errorf("Kind(%q) = %v, want %v", tt.p, got, tt.want)
		}
	}
	if got := r.Clean("scp://u:pw@Host:22/a/../b"); got != "sftp://u@host/b" {
		t.Errorf("clean scp URL %q", got)
	}
	if _, err := r.List(context.Background(), "gopher://host/x"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("unknown scheme: %v", err)
	}
	if _, err := r.Connect(context.Background(), "gopher://host/x"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("connect unknown scheme: %v", err)
	}
	if got, err := r.Connect(context.Background(), dir); err != nil || got != dir {
		t.Errorf("connect local %q %v", got, err)
	}
}

func TestRouterCanEnter(t *testing.T) {
	r := newTestRouter(t)
	tests := []struct {
		e    Entry
		want bool
	}{
		{Entry{Name: "folder", Flags: FlagDir}, true},
		{Entry{Name: "link", Flags: FlagDir | FlagLink}, true},
		{Entry{Name: "a.zip"}, true},
		{Entry{Name: "b.tar.gz"}, true},
		{Entry{Name: "c.txt"}, false},
		{Entry{Name: "d.docx"}, false},
	}
	for _, tt := range tests {
		if got := r.CanEnter("/x", tt.e); got != tt.want {
			t.Errorf("CanEnter(%q) = %v", tt.e.Name, got)
		}
	}
}

func TestRouterLocalOps(t *testing.T) {
	dir := t.TempDir()
	r := newTestRouter(t)
	ctx := context.Background()
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }

	if err := r.Mkdir(ctx, at("a/b")); err != nil {
		t.Fatal(err)
	}
	if err := r.Mkdir(ctx, at("a")); !errors.Is(err, fs.ErrExist) {
		t.Errorf("mkdir existing: %v", err)
	}
	writeVia(t, r, at("a/f.txt"), "local")
	if got := readAll(t, r, at("a/f.txt")); got != "local" {
		t.Errorf("content %q", got)
	}
	if _, err := r.Open(ctx, at("a")); !errors.Is(err, ErrIsFolder) {
		t.Errorf("open folder: %v", err)
	}
	writeVia(t, r, at("a/g.txt"), "g")
	if err := r.Rename(ctx, at("a/f.txt"), at("a/g.txt")); !errors.Is(err, fs.ErrExist) {
		t.Errorf("rename onto existing: %v", err)
	}
	if err := r.Rename(ctx, at("a/f.txt"), at("a/b/f2.txt")); err != nil {
		t.Fatal(err)
	}
	mt := time.Date(2019, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := r.Chtimes(ctx, at("a/b/f2.txt"), mt); err != nil {
		t.Fatal(err)
	}
	if e, _ := r.Stat(at("a/b/f2.txt")); e.MTime != mt.UnixMilli() {
		t.Errorf("mtime %d", e.MTime)
	}
	if err := r.Chmod(ctx, at("a/b/f2.txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := r.Download(ctx, at("a/b/f2.txt"), at("copy.txt"), TransferOptions{PreserveTime: true})
	if err != nil || n != 5 {
		t.Fatalf("local copy %d %v", n, err)
	}
	if fi, _ := os.Stat(at("copy.txt")); !fi.ModTime().Equal(mt) {
		t.Errorf("copy lost mtime: %v", fi.ModTime())
	}
	if err := r.Remove(ctx, at("a")); err == nil {
		t.Error("removed non-empty folder")
	}
	if err := r.RemoveAll(ctx, at("a")); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveAll(ctx, at("a")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("remove missing: %v", err)
	}
	if c := r.Caps(dir); !c.Write || !c.Chmod || c.Remote || c.Archive {
		t.Errorf("caps %+v", c)
	}
	// Moving between the disk and an archive is a cross-FS rename.
	zp := at("z.zip")
	makeZip(t, zp, map[string]string{"x": "x"})
	if err := r.Rename(ctx, at("copy.txt"), filepath.Join(zp, "copy.txt")); !errors.Is(err, ErrCrossFS) {
		t.Errorf("rename into archive: %v", err)
	}
	// AddLocal into a local folder copies trees.
	src := t.TempDir()
	makeTree(t, src, map[string]string{"tree/1.txt": "1", "tree/sub/2.txt": "2"})
	if err := r.AddLocal(ctx, at("dest"), topLevel(t, src)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(at("dest/tree/sub/2.txt")); string(b) != "2" {
		t.Errorf("tree copy %q", b)
	}
}

func TestRouterRegisterSchemes(t *testing.T) {
	r := newTestRouter(t)
	r.RegisterSchemes()
	for _, u := range []string{"ftp://h/", "sftp://u@h/", "ftpes://h/"} {
		if For(u, nil) != FS(r) {
			t.Errorf("For(%q) is not the router", u)
		}
	}
	// A plugin FS registered for another scheme is used by the router.
	m := &fakeFS{*NewMock()}
	Register("plug", m)
	if r.Kind("plug://x/y") != KindOther {
		t.Errorf("plugin kind %v", r.Kind("plug://x/y"))
	}
}

// bareFS implements only FS, to check the fallbacks.
type bareFS struct{ Mock }

func TestOptionalOpsFallback(t *testing.T) {
	b := &bareFS{*NewMock()}
	ctx := context.Background()
	if c := CapsOf(b, "/"); c != (Caps{}) {
		t.Errorf("caps of bare FS %+v", c)
	}
	checks := []error{
		Mkdir(ctx, b, "/x"),
		Remove(ctx, b, "/x"),
		RemoveAll(ctx, b, "/x"),
		Rename(ctx, b, "/x", "/y"),
		Chmod(ctx, b, "/x", 0o644),
		Chtimes(ctx, b, "/x", time.Now()),
	}
	if _, err := OpenFile(ctx, b, "/x"); true {
		checks = append(checks, err)
	}
	if _, err := CreateFile(ctx, b, "/x"); true {
		checks = append(checks, err)
	}
	for i, err := range checks {
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("check %d: %v", i, err)
		}
	}
	if c := CapsOf(LocalRW{}, "/"); !c.Read || !c.Chtimes {
		t.Errorf("LocalRW caps %+v", c)
	}
}
