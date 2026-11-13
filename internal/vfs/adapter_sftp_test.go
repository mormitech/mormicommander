// Tests for SFTPFS through the router, against the in-process SSH/SFTP
// server of adapter_sftp_server_test.go on 127.0.0.1 serving a temp folder:
// connecting with a pre-trusted host key, home-relative URLs, listing with
// links, reading and writing, folder operations, attributes, transfers,
// reconnecting after a dropped connection, and archives on the server.
package vfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/sftp"
)

func sftpRouter(t *testing.T, s *sshServer) (*Router, string) {
	t.Helper()
	kh := filepath.Join(t.TempDir(), "known_hosts")
	if err := sftp.AddKnownHost(kh, s.Addr, s.HostKey.PublicKey(), false); err != nil {
		t.Fatal(err)
	}
	r := newTestRouter(t)
	r.SFTP.Configure = func(cfg *sftp.Config) error {
		cfg.Password = sshTestPassword
		cfg.KnownHostsFiles = []string{kh}
		cfg.Timeout = 10 * time.Second
		return nil
	}
	return r, fmt.Sprintf("sftp://%s@127.0.0.1:%d", sshTestUser, s.Port)
}

func TestSFTPBrowseReadWrite(t *testing.T) {
	s := newSSHServer(t)
	sshWriteFile(t, filepath.Join(s.Root, "docs", "a.txt"), "alpha over ssh")
	sshWriteFile(t, filepath.Join(s.Root, ".profile"), "p")
	if runtime.GOOS != "windows" {
		os.Symlink("docs", filepath.Join(s.Root, "link-to-docs"))
		os.Symlink("missing", filepath.Join(s.Root, "dangling"))
	}
	r, base := sftpRouter(t, s)
	ctx := context.Background()

	home, err := r.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(home, base+"/") || strings.Contains(home, sshTestPassword) {
		t.Fatalf("home %q", home)
	}
	if r.Kind(home) != KindSFTP {
		t.Errorf("kind %v", r.Kind(home))
	}
	// "/~/" URLs are relative to the login folder.
	viaTilde, err := r.Connect(ctx, base+"/~/docs")
	if err != nil || viaTilde != home+"/docs" {
		t.Errorf("tilde URL -> %q %v", viaTilde, err)
	}
	l, err := r.List(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".profile", "docs/"}
	if runtime.GOOS != "windows" {
		want = []string{".profile", "dangling", "docs/", "link-to-docs/"}
	}
	eq(t, "home", names(l), want)
	for _, e := range l.Entries {
		switch e.Name {
		case ".profile":
			if e.Flags&FlagHidden == 0 {
				t.Error("dotfile not hidden")
			}
		case "link-to-docs":
			if e.Flags&FlagLink == 0 || e.Flags&FlagDir == 0 {
				t.Errorf("link flags %b", e.Flags)
			}
		case "dangling":
			if e.Flags&FlagBroken == 0 {
				t.Errorf("dangling flags %b", e.Flags)
			}
		}
	}
	if got := readAll(t, r, home+"/docs/a.txt"); got != "alpha over ssh" {
		t.Errorf("content %q", got)
	}
	if e, err := r.Stat(home + "/docs/a.txt"); err != nil || e.Size != 14 {
		t.Errorf("stat %+v %v", e, err)
	}
	if _, err := r.List(ctx, home+"/docs/a.txt"); !errors.Is(err, ErrNotFolder) {
		t.Errorf("list file: %v", err)
	}
	if _, err := r.Stat(home + "/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat missing: %v", err)
	}
	if _, err := r.Open(ctx, home+"/docs"); !errors.Is(err, ErrIsFolder) {
		t.Errorf("open folder: %v", err)
	}

	if err := r.Mkdir(ctx, home+"/new/deep"); err != nil {
		t.Fatal(err)
	}
	if err := r.Mkdir(ctx, home+"/new"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("mkdir existing: %v", err)
	}
	writeVia(t, r, home+"/new/deep/f.txt", "written")
	if got := sshReadFile(t, filepath.Join(s.Root, "new", "deep", "f.txt")); got != "written" {
		t.Errorf("server has %q", got)
	}
	if err := r.Rename(ctx, home+"/new/deep/f.txt", home+"/new/g.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename(ctx, home+"/new/g.txt", home+"/docs/a.txt"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("rename onto existing: %v", err)
	}
	if err := r.Rename(ctx, home+"/new/g.txt", "ftp://elsewhere/x"); !errors.Is(err, ErrCrossFS) {
		t.Errorf("cross rename: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := r.Chmod(ctx, home+"/new/g.txt", 0o600); err != nil {
			t.Fatal(err)
		}
		if fi, _ := os.Stat(filepath.Join(s.Root, "new", "g.txt")); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v", fi.Mode())
		}
	}
	mt := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := r.Chtimes(ctx, home+"/new/g.txt", mt); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(s.Root, "new", "g.txt")); !fi.ModTime().Equal(mt) {
		t.Errorf("mtime %v", fi.ModTime())
	}
	if err := r.Remove(ctx, home+"/new"); err == nil {
		t.Error("removed non-empty folder")
	}
	if err := r.RemoveAll(ctx, home+"/new"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "new")); !os.IsNotExist(err) {
		t.Error("tree still on server")
	}

	dir := t.TempDir()
	local := filepath.Join(dir, "up.bin")
	data := strings.Repeat("sftp-data", 5000)
	os.WriteFile(local, []byte(data), 0o644)
	var last int64
	if _, err := r.Upload(ctx, local, home+"/up.bin", TransferOptions{Progress: func(d, _ int64) { last = d }}); err != nil {
		t.Fatal(err)
	}
	if last != int64(len(data)) {
		t.Errorf("progress %d", last)
	}
	down := filepath.Join(dir, "down.bin")
	os.WriteFile(down, []byte(data[:100]), 0o644)
	if _, err := r.Download(ctx, home+"/up.bin", down, TransferOptions{Resume: true}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(down); string(b) != data {
		t.Errorf("download has %d bytes", len(b))
	}
	if c := r.Caps(home); !c.Remote || !c.Write || !c.Chtimes {
		t.Errorf("caps %+v", c)
	}
}

func TestSFTPReconnectAndArchive(t *testing.T) {
	s := newSSHServer(t)
	zp := filepath.Join(s.Root, "pkg.zip")
	makeZip(t, zp, map[string]string{"inside/readme.md": "# zipped"})
	r, base := sftpRouter(t, s)
	ctx := context.Background()
	home, err := r.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	s.DropConnections()
	// The dead connection is noticed and replaced.
	deadline := time.Now().Add(5 * time.Second)
	var l Listing
	for {
		l, err = r.List(ctx, home)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("list after drop: %v", err)
	}
	eq(t, "home", names(l), []string{"pkg.zip"})

	arc := home + "/pkg.zip"
	if !r.CanEnter(home, l.Entries[0]) {
		t.Error("cannot enter zip on sftp")
	}
	l, err = r.List(ctx, arc+"/inside")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "zip on sftp", names(l), []string{"readme.md"})
	if got := readAll(t, r, arc+"/inside/readme.md"); got != "# zipped" {
		t.Errorf("content %q", got)
	}
	r.NoRemoteArchives = true
	if r.CanEnter(home, Entry{Name: "pkg.zip"}) {
		t.Error("CanEnter ignores NoRemoteArchives")
	}
}

func TestSFTPHostKeyRejected(t *testing.T) {
	s := newSSHServer(t)
	r := newTestRouter(t)
	empty := filepath.Join(t.TempDir(), "known_hosts")
	r.SFTP.Configure = func(cfg *sftp.Config) error {
		cfg.Password = sshTestPassword
		cfg.KnownHostsFiles = []string{empty}
		cfg.Timeout = 5 * time.Second
		return nil
	}
	_, err := r.List(context.Background(), fmt.Sprintf("sftp://%s@127.0.0.1:%d/", sshTestUser, s.Port))
	if err == nil {
		t.Fatal("unknown host key accepted without a prompt")
	}
	if len(r.SFTP.Sessions()) != 0 {
		t.Error("failed connection left a session")
	}
}
