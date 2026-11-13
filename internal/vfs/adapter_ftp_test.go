// Tests for FTPFS through the router, against the in-process FTP server of
// adapter_ftp_server_test.go listening on 127.0.0.1 and serving a temp
// folder: connecting by URL, listing, reading and writing, folder
// operations, permissions, transfers with resume, parallel use of the pool,
// archives stored on the server, and the URL helpers.
package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/ftp"
)

func ftpRouter(t *testing.T, s *ftpServer) (*Router, string) {
	t.Helper()
	r := newTestRouter(t)
	r.FTP.Configure = func(cfg *ftp.Config) error {
		cfg.ServerLocation = time.UTC
		cfg.Timeout = 5 * time.Second
		return nil
	}
	return r, fmt.Sprintf("ftp://u:pw@127.0.0.1:%d", s.port)
}

func newAuthFTPServer(t *testing.T) *ftpServer {
	return newFTPServer(t, func(s *ftpServer) { s.user, s.pass = "u", "pw" })
}

func TestFTPBrowseAndRead(t *testing.T) {
	s := newAuthFTPServer(t)
	s.write("pub/readme.txt", "hello ftp")
	s.write("pub/sub/deep.bin", "0123456789")
	s.write(".dot", "x")
	r, base := ftpRouter(t, s)
	ctx := context.Background()

	home, err := r.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("ftp://u@127.0.0.1:%d/", s.port)
	if home != want {
		t.Fatalf("home %q, want %q (password must not appear)", home, want)
	}
	if r.Kind(home) != KindFTP {
		t.Errorf("kind %v", r.Kind(home))
	}
	l, err := r.List(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "root", names(l), []string{".dot", "pub/"})
	if l.Parent != "" {
		t.Errorf("parent of server root %q", l.Parent)
	}
	pub := r.Join(home, "pub")
	l, err = r.List(ctx, pub)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "pub", names(l), []string{"readme.txt", "sub/"})
	if l.Path != want+"pub" || l.Parent != want {
		t.Errorf("path %q parent %q", l.Path, l.Parent)
	}
	for _, e := range l.Entries {
		if e.Name == "readme.txt" && (e.Size != 9 || e.MTime == 0) {
			t.Errorf("readme entry %+v", e)
		}
	}
	if got := readAll(t, r, pub+"/readme.txt"); got != "hello ftp" {
		t.Errorf("content %q", got)
	}
	e, err := r.Stat(pub + "/sub/deep.bin")
	if err != nil || e.Size != 10 || e.Name != "deep.bin" {
		t.Errorf("stat %+v %v", e, err)
	}
	if e, err := r.Stat(pub + "/sub"); err != nil || e.Flags&FlagDir == 0 {
		t.Errorf("stat dir %+v %v", e, err)
	}
	if _, err := r.Stat(pub + "/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat missing: %v", err)
	}
	if _, err := r.List(ctx, pub+"/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("list missing: %v", err)
	}
	if _, err := r.List(ctx, pub+"/readme.txt"); !errors.Is(err, ErrNotFolder) {
		t.Errorf("list file: %v", err)
	}
	if got := r.Root(pub + "/sub"); got != want {
		t.Errorf("root %q", got)
	}
	if c := r.Caps(pub); !c.Remote || !c.Write || !c.Chmod || !c.Chtimes || c.Archive {
		t.Errorf("caps %+v", c)
	}

	// While a file is being read, the panel can still list (second pooled
	// connection).
	rc, err := r.Open(ctx, pub+"/readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.List(ctx, pub); err != nil {
		t.Errorf("list during read: %v", err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello ftp" {
		t.Errorf("parallel read %q", b)
	}
	if got := r.FTP.Sessions(); len(got) != 1 || got[0] != want {
		t.Errorf("sessions %v", got)
	}
	r.Disconnect(pub)
	if len(r.FTP.Sessions()) != 0 {
		t.Error("still connected")
	}
}

func TestFTPWrite(t *testing.T) {
	s := newAuthFTPServer(t)
	s.write("keep.txt", "k")
	r, base := ftpRouter(t, s)
	ctx := context.Background()
	home, err := r.Connect(ctx, base+"/")
	if err != nil {
		t.Fatal(err)
	}
	at := func(p string) string { return strings.TrimSuffix(home, "/") + "/" + p }

	if err := r.Mkdir(ctx, at("a/b/c")); err != nil {
		t.Fatal(err)
	}
	if !s.exists("a/b/c") {
		t.Fatal("mkdir did not reach the server")
	}
	if err := r.Mkdir(ctx, at("a")); !errors.Is(err, fs.ErrExist) {
		t.Errorf("mkdir existing: %v", err)
	}
	writeVia(t, r, at("a/new.txt"), "written over ftp")
	if got := s.read("a/new.txt"); got != "written over ftp" {
		t.Errorf("server has %q", got)
	}
	if err := r.Rename(ctx, at("a/new.txt"), at("a/b/moved.txt")); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename(ctx, at("keep.txt"), at("a/b/moved.txt")); !errors.Is(err, fs.ErrExist) {
		t.Errorf("rename onto existing: %v", err)
	}
	if err := r.Rename(ctx, at("keep.txt"), filepath.Join(t.TempDir(), "x")); !errors.Is(err, ErrCrossFS) {
		t.Errorf("rename to local: %v", err)
	}
	if err := r.Chmod(ctx, at("keep.txt"), 0o600); err != nil {
		t.Errorf("chmod: %v", err)
	}
	mt := time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := r.Chtimes(ctx, at("keep.txt"), mt); err != nil {
		t.Errorf("chtimes: %v", err)
	}
	if err := r.Remove(ctx, at("a/b")); err == nil {
		t.Error("removed a non-empty folder")
	}
	if err := r.Remove(ctx, at("a/b/c")); err != nil {
		t.Errorf("remove empty folder: %v", err)
	}
	if err := r.Remove(ctx, at("a/b/moved.txt")); err != nil {
		t.Errorf("remove file: %v", err)
	}
	writeVia(t, r, at("a/b/again.txt"), "x")
	if err := r.RemoveAll(ctx, at("a")); err != nil {
		t.Fatal(err)
	}
	if s.exists("a") {
		t.Error("tree still on server")
	}
	if err := r.RemoveAll(ctx, at("a")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("remove missing: %v", err)
	}

	// Transfers with progress and resume.
	dir := t.TempDir()
	local := filepath.Join(dir, "up.bin")
	data := strings.Repeat("abcdefgh", 4096)
	os.WriteFile(local, []byte(data), 0o644)
	var last int64
	if _, err := r.Upload(ctx, local, at("up.bin"), TransferOptions{Progress: func(d, _ int64) { last = d }}); err != nil {
		t.Fatal(err)
	}
	if s.read("up.bin") != data || last != int64(len(data)) {
		t.Errorf("upload: %d bytes on server, progress %d", len(s.read("up.bin")), last)
	}
	down := filepath.Join(dir, "down.bin")
	os.WriteFile(down, []byte(data[:1000]), 0o644) // a partial earlier download
	if _, err := r.Download(ctx, at("up.bin"), down, TransferOptions{Resume: true}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(down); string(b) != data {
		t.Errorf("resumed download has %d bytes", len(b))
	}
	if s.sent("REST 1000") == 0 {
		t.Error("download did not resume with REST")
	}
	// Generic copy between FTP and an archive.
	zp := filepath.Join(dir, "z.zip")
	makeZip(t, zp, map[string]string{"in.txt": "zip to ftp"})
	sfs, sp := r.Resolve(filepath.Join(zp, "in.txt"))
	dfs, dp := r.Resolve(at("from-zip.txt"))
	if _, err := CopyFile(ctx, sfs, sp, dfs, dp, r.Local, TransferOptions{}); err != nil {
		t.Fatal(err)
	}
	if s.read("from-zip.txt") != "zip to ftp" {
		t.Error("zip to ftp copy failed")
	}
	// AddLocal on a server uploads trees.
	tree := t.TempDir()
	makeTree(t, tree, map[string]string{"t/x.txt": "x", "t/y/z.txt": "z"})
	if err := r.AddLocal(ctx, home, topLevel(t, tree)); err != nil {
		t.Fatal(err)
	}
	if s.read("t/y/z.txt") != "z" {
		t.Error("tree upload failed")
	}
}

func TestFTPArchiveOnServer(t *testing.T) {
	s := newAuthFTPServer(t)
	zp := filepath.Join(t.TempDir(), "remote.zip")
	makeZip(t, zp, map[string]string{"docs/a.txt": "remote alpha"})
	b, _ := os.ReadFile(zp)
	s.write("files/remote.zip", string(b))
	r, base := ftpRouter(t, s)
	ctx := context.Background()
	home, err := r.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	arc := home + "files/remote.zip"
	l, err := r.List(ctx, arc+"/docs")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "remote zip docs", names(l), []string{"a.txt"})
	if l.Parent != arc {
		t.Errorf("parent %q", l.Parent)
	}
	if r.Kind(arc+"/docs") != KindArchive || r.Kind(arc) != KindFTP {
		t.Errorf("kinds %v %v", r.Kind(arc+"/docs"), r.Kind(arc))
	}
	if got := readAll(t, r, arc+"/docs/a.txt"); got != "remote alpha" {
		t.Errorf("content %q", got)
	}
	if e, err := r.Stat(arc); err != nil || e.Flags&FlagDir != 0 {
		t.Errorf("archive on server should stat as a file: %+v %v", e, err)
	}
	if err := r.Mkdir(ctx, arc+"/x"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("write into server archive: %v", err)
	}
	if got := r.Parent(arc); got != home+"files" {
		t.Errorf("parent of archive %q", got)
	}
	if !r.CanEnter(home+"files", Entry{Name: "remote.zip"}) {
		t.Error("cannot enter archive on server")
	}

	// A new version on the server is picked up when the archive is entered.
	zp2 := filepath.Join(t.TempDir(), "v2.zip")
	makeZip(t, zp2, map[string]string{"new.txt": "v2", "more.txt": "more data"})
	b2, _ := os.ReadFile(zp2)
	s.write("files/remote.zip", string(b2))
	l, err = r.List(ctx, arc)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "after server change", names(l), []string{"more.txt", "new.txt"})

	r.Disconnect(home)
	if r.Archives.Mounted(arc) {
		t.Error("server archive still mounted after disconnect")
	}
}

func TestFTPReconnect(t *testing.T) {
	s := newAuthFTPServer(t)
	s.write("f.txt", "x")
	r, base := ftpRouter(t, s)
	ctx := context.Background()
	home, err := r.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	// Kill the idle pooled connection behind the client's back.
	r.FTP.mu.Lock()
	for _, sess := range r.FTP.sessions {
		c, err := sess.pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		c.Quote(ctx, "QUIT")
		sess.pool.Put(c)
	}
	r.FTP.mu.Unlock()
	if _, err := r.List(ctx, home); err != nil {
		t.Fatalf("list after dropped connection: %v", err)
	}
}

func TestFTPConfigureError(t *testing.T) {
	r := newTestRouter(t)
	r.FTP.Configure = func(*ftp.Config) error { return errors.New("user cancelled") }
	if _, err := r.List(context.Background(), "ftp://127.0.0.1:1/"); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("configure error not returned: %v", err)
	}
}

func TestRemoteURLHelpers(t *testing.T) {
	tests := []struct {
		in, clean, parent string
	}{
		{"ftp://host", "ftp://host/", ""},
		{"FTP://User:Secret@Host:21/pub/", "ftp://User@host/pub", "ftp://User@host/"},
		{"ftp://host:2121/a/b/../c", "ftp://host:2121/a/c", "ftp://host:2121/a"},
		{"ftps://h:990/x", "ftps://h/x", "ftps://h/"},
		{"sftp://bob@[::1]:2222/home/bob", "sftp://bob@[::1]:2222/home/bob", "sftp://bob@[::1]:2222/home"},
		{"ftp://a%40b:p%3Aw@h/with space/ü", "ftp://a%40b@h/with space/ü", "ftp://a%40b@h/with space"},
		{`ftp://h/dir\sub`, "ftp://h/dir/sub", "ftp://h/dir"},
	}
	for _, tt := range tests {
		if got := cleanURL(tt.in); got != tt.clean {
			t.Errorf("cleanURL(%q) = %q, want %q", tt.in, got, tt.clean)
		}
		if got := urlParent(tt.in); got != tt.parent {
			t.Errorf("urlParent(%q) = %q, want %q", tt.in, got, tt.parent)
		}
	}
	u, err := parseRemote("ftp://a%40b:p%3Aw@h:2121/x")
	if err != nil || u.User != "a@b" || u.Password != "p:w" || u.Port != 2121 || u.Path != "/x" {
		t.Errorf("parse = %+v %v", u, err)
	}
	if got := urlJoin("ftp://h/", "a b"); got != "ftp://h/a b" {
		t.Errorf("join %q", got)
	}
	if got := urlJoin("ftp://h/x", "y"); got != "ftp://h/x/y" {
		t.Errorf("join %q", got)
	}
	if got := urlRoot("sftp://u@h/x/y"); got != "sftp://u@h/" {
		t.Errorf("root %q", got)
	}
	for _, bad := range []string{"ftp://", "ftp://h:0/", "ftp://h:99999/", "ftp://[::1/", "nourl"} {
		if _, err := parseRemote(bad); err == nil {
			t.Errorf("parseRemote(%q) accepted", bad)
		}
	}
	if hasScheme(`C:\x`) || hasScheme("/a://b") || !hasScheme("ftp://x") {
		t.Error("hasScheme")
	}
}
