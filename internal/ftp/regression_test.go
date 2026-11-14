// This file holds regression tests for bugs found after the first version
// of the engine: UNIX LIST names that start with a space must keep it (or
// two different files collapse into one name and later operations hit the
// wrong file), a resume against a target that is larger than the source
// must fail instead of reporting success, and a folder download must fetch
// files behind symbolic links and report what it could not copy instead of
// silently leaving it out. An upload over a TLS data connection must arrive
// whole although the server sent something (a TLS 1.3 session ticket) the
// client never read (found by the live tests of board issue #106).
package ftp

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

func TestListKeepsLeadingSpaces(t *testing.T) {
	srv := newTestServer(t, func(s *testServer) { s.noMLSD = true })
	srv.write(" a", "space-a")
	srv.write("a", "plain-a")
	cfg := srv.cfg()
	cfg.DisableMLSD = true
	c := dialT(t, cfg)
	ctx := context.Background()

	es, err := c.List(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	got := names(es)
	sort.Strings(got)
	if len(got) != 2 || got[0] != " a" || got[1] != "a" {
		t.Fatalf("names = %q, want [\" a\" \"a\"]", got)
	}
	// Downloading the space-prefixed entry must fetch that file, not "a".
	dst := filepath.Join(t.TempDir(), "out")
	if _, err := c.Download(ctx, "/ a", dst, TransferOptions{Mode: TransferBinary}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "space-a" {
		t.Fatalf("downloaded %q, want the space-prefixed file", b)
	}
}

func TestResumeRefusesLargerTarget(t *testing.T) {
	srv := newTestServer(t)
	c := dialT(t, srv.cfg())
	ctx := context.Background()
	dir := t.TempDir()
	const oldLocal = "an older, much longer local file"

	tests := []struct {
		name   string
		remote string
		local  string
		upload bool
	}{
		{"download local larger", "new short", oldLocal, false},
		{"upload remote larger", oldLocal, "new short", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := filepath.Base(t.Name()) + ".txt"
			srv.write(rel, tt.remote)
			lp := filepath.Join(dir, rel)
			if err := os.WriteFile(lp, []byte(tt.local), 0o644); err != nil {
				t.Fatal(err)
			}
			opt := TransferOptions{Resume: true, Mode: TransferBinary}
			var n int64
			var err error
			if tt.upload {
				n, err = c.Upload(ctx, lp, "/"+rel, opt)
			} else {
				n, err = c.Download(ctx, "/"+rel, lp, opt)
			}
			if !errors.Is(err, ErrResumeTooLarge) || n != 0 {
				t.Fatalf("n=%d err=%v, want ErrResumeTooLarge", n, err)
			}
			// Both sides are left exactly as they were.
			if b, _ := os.ReadFile(lp); string(b) != tt.local {
				t.Errorf("local changed to %q", b)
			}
			if got := srv.read(rel); got != tt.remote {
				t.Errorf("remote changed to %q", got)
			}
		})
	}
}

func TestDownloadTreeLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test server needs symlinks")
	}
	srv := newTestServer(t)
	srv.write("tree/real.txt", "REAL")
	srv.write("target/inner.txt", "INNER")
	root := srv.root
	if err := os.Symlink("real.txt", filepath.Join(root, "tree", "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../target", filepath.Join(root, "tree", "dirlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "tree", "broken")); err != nil {
		t.Fatal(err)
	}

	for _, mlsd := range []bool{true, false} {
		t.Run(map[bool]string{true: "MLSD", false: "LIST"}[mlsd], func(t *testing.T) {
			cfg := srv.cfg()
			cfg.DisableMLSD = !mlsd
			c := dialT(t, cfg)
			back := t.TempDir()
			res, err := c.DownloadTree(context.Background(), "/tree", back, TreeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(filepath.Join(back, "link.txt")); string(b) != "REAL" {
				t.Errorf("link.txt = %q, want the linked file's content", b)
			}
			if res.Files != 2 {
				t.Errorf("Files = %d, want 2", res.Files)
			}
			sort.Strings(res.Skipped)
			if len(res.Skipped) != 2 || res.Skipped[0] != "/tree/broken" || res.Skipped[1] != "/tree/dirlink" {
				t.Errorf("Skipped = %q, want broken and dirlink", res.Skipped)
			}
			for _, n := range []string{"broken", "dirlink"} {
				if _, err := os.Lstat(filepath.Join(back, n)); err == nil {
					t.Errorf("%s was created locally", n)
				}
			}
		})
	}
}

// A TLS 1.3 server sends session tickets on the data connection right after
// the handshake, which an upload never reads. Closing a socket with unread
// data makes the system send a reset instead of a normal close, and the
// server sees the upload fail or loses the part it had not read yet (the live
// FTPS test of board issue #106 got 1425408 of 1572871 bytes, reported as a
// success). The upload must end its side cleanly (close_notify, a half-close)
// and wait for the server to close. The test server sends one byte instead of
// a ticket, over plain FTP and over TLS, and answers 426 when its read fails.
func TestUploadWithUnreadServerDataArrivesWhole(t *testing.T) {
	serverTLS, clientTLS := testCert(t)
	for _, tc := range []struct {
		name   string
		tls    bool
		active bool
	}{{"plain", false, false}, {"tls", true, false}, {"tls active", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t, func(s *testServer) {
				s.storSends = true
				s.storDelay = 200 * time.Millisecond // the data waits in the socket buffers when the client is done
				if tc.tls {
					s.tlsConf = serverTLS
					s.requireTLS = true
				}
			})
			cfg := srv.cfg()
			if tc.tls {
				cfg.Security = SecurityExplicitTLS
				cfg.TLSConfig = clientTLS
			}
			if tc.active {
				cfg.DataMode = Active
			}
			c := dialT(t, cfg)
			data := make([]byte, 3<<20+11)
			rand.Read(data)
			src := filepath.Join(t.TempDir(), "big.bin")
			os.WriteFile(src, data, 0o644)
			n, err := c.Upload(context.Background(), src, "big.bin", TransferOptions{Mode: TransferBinary})
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if got := []byte(srv.read("big.bin")); n != int64(len(data)) || !bytes.Equal(got, data) {
				t.Fatalf("%d bytes sent, %d arrived of %d", n, len(got), len(data))
			}
			// the control connection is still in step: the next command works
			if _, err := c.Size(context.Background(), "big.bin"); err != nil {
				t.Fatalf("SIZE after the upload: %v", err)
			}
		})
	}
}
