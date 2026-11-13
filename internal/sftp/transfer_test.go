// Tests for moving file contents: download and upload with progress, resume
// in both directions, the no-overwrite guard, preserving times and modes,
// cancellation mid-transfer (leaving a resumable partial file), copying
// within the server, and recursive folder transfers.
package sftp

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mormicommander/internal/remotefs"
)

func randomData(n int) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(int64(n))).Read(b)
	return b
}

func TestDownloadUpload(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	local := t.TempDir()
	sizes := []int{0, 1, 1000, 32 * 1024, 700*1024 + 13} // around pkg/sftp's packet size
	for _, n := range sizes {
		data := randomData(n)
		remote := filepath.Join(s.Root, "src.bin")
		os.WriteFile(remote, data, 0o644)

		var last remotefs.Progress
		dst := filepath.Join(local, "dl.bin")
		got, err := c.Download(t.Context(), remote, dst, remotefs.TransferOptions{
			Progress: func(p remotefs.Progress) { last = p }})
		if err != nil {
			t.Fatalf("download %d: %v", n, err)
		}
		if got != int64(n) {
			t.Errorf("download %d: returned %d", n, got)
		}
		if b, _ := os.ReadFile(dst); !bytes.Equal(b, data) {
			t.Fatalf("download %d: content differs", n)
		}
		if last.Done != int64(n) || last.Total != int64(n) || last.Path != remote {
			t.Errorf("download %d: last progress %+v", n, last)
		}

		up := filepath.Join(s.Root, "up.bin")
		got, err = c.Upload(t.Context(), dst, up, remotefs.TransferOptions{
			Progress: func(p remotefs.Progress) { last = p }})
		if err != nil {
			t.Fatalf("upload %d: %v", n, err)
		}
		if got != int64(n) || last.Done != int64(n) {
			t.Errorf("upload %d: returned %d, progress %+v", n, got, last)
		}
		if b, _ := os.ReadFile(up); !bytes.Equal(b, data) {
			t.Fatalf("upload %d: content differs", n)
		}
	}
}

func TestTransferErrors(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	local := t.TempDir()
	os.Mkdir(filepath.Join(s.Root, "adir"), 0o755)
	writeFile(t, filepath.Join(s.Root, "f"), "remote")
	writeFile(t, filepath.Join(local, "f"), "local")
	o := remotefs.TransferOptions{}
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{"download missing", func() error { _, err := c.Download(t.Context(), s.Root+"/none", local+"/x", o); return err }, os.ErrNotExist},
		{"download folder", func() error { _, err := c.Download(t.Context(), s.Root+"/adir", local+"/x", o); return err }, nil},
		{"download onto local folder", func() error { _, err := c.Download(t.Context(), s.Root+"/f", local, o); return err }, nil},
		{"download no overwrite", func() error {
			_, err := c.Download(t.Context(), s.Root+"/f", local+"/f", remotefs.TransferOptions{NoOverwrite: true})
			return err
		}, remotefs.ErrExists},
		{"upload missing", func() error { _, err := c.Upload(t.Context(), local+"/none", s.Root+"/x", o); return err }, os.ErrNotExist},
		{"upload folder", func() error { _, err := c.Upload(t.Context(), local, s.Root+"/x", o); return err }, nil},
		{"upload onto remote folder", func() error { _, err := c.Upload(t.Context(), local+"/f", s.Root+"/adir", o); return err }, nil},
		{"upload no overwrite", func() error {
			_, err := c.Upload(t.Context(), local+"/f", s.Root+"/f", remotefs.TransferOptions{NoOverwrite: true})
			return err
		}, remotefs.ErrExists},
		{"copy same file", func() error { _, err := c.CopyRemote(t.Context(), s.Root+"/f", s.Root+"/f", o); return err }, nil},
		{"copy folder", func() error { _, err := c.CopyRemote(t.Context(), s.Root+"/adir", s.Root+"/b", o); return err }, nil},
		{"copy no overwrite", func() error {
			_, err := c.CopyRemote(t.Context(), s.Root+"/f", s.Root+"/f2", o)
			if err != nil {
				return err
			}
			_, err = c.CopyRemote(t.Context(), s.Root+"/f", s.Root+"/f2", remotefs.TransferOptions{NoOverwrite: true})
			return err
		}, remotefs.ErrExists},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
	// The guarded files were not touched.
	if readFile(t, filepath.Join(local, "f")) != "local" || readFile(t, filepath.Join(s.Root, "f")) != "remote" {
		t.Error("no-overwrite changed a file")
	}
}

func TestResume(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	local := t.TempDir()
	data := randomData(300*1024 + 7)

	t.Run("download", func(t *testing.T) {
		remote := filepath.Join(s.Root, "big.bin")
		os.WriteFile(remote, data, 0o644)
		dst := filepath.Join(local, "big.bin")
		os.WriteFile(dst, data[:100000], 0o644)
		var first remotefs.Progress
		gotFirst := false
		n, err := c.Download(t.Context(), remote, dst, remotefs.TransferOptions{Resume: true,
			Progress: func(p remotefs.Progress) {
				if !gotFirst {
					first, gotFirst = p, true
				}
			}})
		if err != nil {
			t.Fatal(err)
		}
		if n != int64(len(data)-100000) {
			t.Errorf("transferred %d", n)
		}
		if first.Done != 100000 {
			t.Errorf("first progress %+v, want Done=100000", first)
		}
		if b, _ := os.ReadFile(dst); !bytes.Equal(b, data) {
			t.Fatal("resumed download differs")
		}
		// Already complete: nothing transferred.
		if n, err := c.Download(t.Context(), remote, dst, remotefs.TransferOptions{Resume: true}); n != 0 || err != nil {
			t.Errorf("complete resume: %d %v", n, err)
		}
		// Local larger than remote: refused, local untouched.
		os.WriteFile(dst, append(data, 'x'), 0o644)
		if _, err := c.Download(t.Context(), remote, dst, remotefs.TransferOptions{Resume: true}); err == nil {
			t.Error("resume onto larger file succeeded")
		}
		// Resume without existing file is a plain download.
		os.Remove(dst)
		if n, err := c.Download(t.Context(), remote, dst, remotefs.TransferOptions{Resume: true}); err != nil || n != int64(len(data)) {
			t.Errorf("resume without partial: %d %v", n, err)
		}
	})

	t.Run("upload", func(t *testing.T) {
		src := filepath.Join(local, "up.bin")
		os.WriteFile(src, data, 0o644)
		remote := filepath.Join(s.Root, "up.bin")
		os.WriteFile(remote, data[:54321], 0o644)
		n, err := c.Upload(t.Context(), src, remote, remotefs.TransferOptions{Resume: true})
		if err != nil {
			t.Fatal(err)
		}
		if n != int64(len(data)-54321) {
			t.Errorf("transferred %d", n)
		}
		if b, _ := os.ReadFile(remote); !bytes.Equal(b, data) {
			t.Fatal("resumed upload differs")
		}
		if n, err := c.Upload(t.Context(), src, remote, remotefs.TransferOptions{Resume: true}); n != 0 || err != nil {
			t.Errorf("complete resume: %d %v", n, err)
		}
		os.WriteFile(remote, append(data, 'x'), 0o644)
		if _, err := c.Upload(t.Context(), src, remote, remotefs.TransferOptions{Resume: true}); err == nil {
			t.Error("resume onto larger remote succeeded")
		}
	})
}

func TestCancelThenResume(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	data := randomData(2 * 1024 * 1024)
	remote := filepath.Join(s.Root, "c.bin")
	os.WriteFile(remote, data, 0o644)
	dst := filepath.Join(t.TempDir(), "c.bin")

	ctx, cancel := context.WithCancel(t.Context())
	_, err := c.Download(ctx, remote, dst, remotefs.TransferOptions{Progress: func(p remotefs.Progress) {
		if p.Done > 256*1024 {
			cancel()
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
	fi, err := os.Stat(dst)
	if err != nil || fi.Size() == 0 || fi.Size() >= int64(len(data)) {
		t.Fatalf("partial file: %v %v", fi, err)
	}
	if _, err := c.Download(t.Context(), remote, dst, remotefs.TransferOptions{Resume: true}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); !bytes.Equal(b, data) {
		t.Fatal("cancel+resume result differs")
	}

	// Upload cancellation too.
	up := filepath.Join(s.Root, "c-up.bin")
	ctx, cancel = context.WithCancel(t.Context())
	_, err = c.Upload(ctx, dst, up, remotefs.TransferOptions{Progress: func(p remotefs.Progress) {
		if p.Done > 256*1024 {
			cancel()
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("upload err = %v", err)
	}
	if _, err := c.Upload(t.Context(), dst, up, remotefs.TransferOptions{Resume: true}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(up); !bytes.Equal(b, data) {
		t.Fatal("upload cancel+resume differs")
	}
	// The connection is still usable after a cancelled transfer.
	if _, err := c.ReadDir(s.Root); err != nil {
		t.Fatal(err)
	}
}

func TestPreserve(t *testing.T) {
	if isWindows() {
		t.Skip("unix permissions")
	}
	s := newTestServer(t)
	c := s.dial(t)
	mt := time.Date(2010, 5, 6, 7, 8, 9, 0, time.UTC)
	remote := filepath.Join(s.Root, "p.txt")
	writeFile(t, remote, "x")
	os.Chmod(remote, 0o640)
	os.Chtimes(remote, mt, mt)
	dst := filepath.Join(t.TempDir(), "p.txt")
	opts := remotefs.TransferOptions{PreserveTimes: true, PreserveMode: true}
	if _, err := c.Download(t.Context(), remote, dst, opts); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(dst)
	if !fi.ModTime().Equal(mt) || fi.Mode().Perm() != 0o640 {
		t.Errorf("download preserve: %v %v", fi.ModTime(), fi.Mode().Perm())
	}

	mt2 := time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chmod(dst, 0o604)
	os.Chtimes(dst, mt2, mt2)
	up := filepath.Join(s.Root, "p2.txt")
	if _, err := c.Upload(t.Context(), dst, up, opts); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(up)
	if !fi.ModTime().Equal(mt2) || fi.Mode().Perm() != 0o604 {
		t.Errorf("upload preserve: %v %v", fi.ModTime(), fi.Mode().Perm())
	}

	cp := filepath.Join(s.Root, "p3.txt")
	if _, err := c.CopyRemote(t.Context(), up, cp, opts); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(cp)
	if !fi.ModTime().Equal(mt2) || fi.Mode().Perm() != 0o604 {
		t.Errorf("copy preserve: %v %v", fi.ModTime(), fi.Mode().Perm())
	}
}

func TestCopyRemote(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	data := randomData(100000)
	os.WriteFile(filepath.Join(s.Root, "a"), data, 0o644)
	var last remotefs.Progress
	n, err := c.CopyRemote(t.Context(), s.Root+"/a", s.Root+"/b", remotefs.TransferOptions{
		Progress: func(p remotefs.Progress) { last = p }})
	if err != nil || n != int64(len(data)) {
		t.Fatalf("CopyRemote = %d, %v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(s.Root, "b")); !bytes.Equal(b, data) {
		t.Fatal("copy differs")
	}
	if last.Done != int64(len(data)) || last.Total != int64(len(data)) {
		t.Errorf("progress %+v", last)
	}
}

func TestTrees(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	src := filepath.Join(t.TempDir(), "tree")
	writeFile(t, filepath.Join(src, "a.txt"), "A")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "BB")
	writeFile(t, filepath.Join(src, "sub", "deeper", "c.txt"), "CCC")
	os.MkdirAll(filepath.Join(src, "empty"), 0o755)
	if !isWindows() {
		os.Symlink("a.txt", filepath.Join(src, "link-to-file"))
		os.Symlink("sub", filepath.Join(src, "link-to-dir"))
	}

	var files []string
	n, err := c.UploadTree(t.Context(), src, s.Root+"/up", TreeOptions{
		OnFile: func(srcP, dst string) error { files = append(files, dst); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 {
		t.Errorf("OnFile saw %v", files)
	}
	wantBytes := int64(1 + 2 + 3)
	if !isWindows() {
		wantBytes++ // link-to-file uploaded as a copy of a.txt
	}
	if n != wantBytes {
		t.Errorf("uploaded %d bytes, want %d", n, wantBytes)
	}
	up := filepath.Join(s.Root, "up", "tree")
	for p, want := range map[string]string{"a.txt": "A", "sub/b.txt": "BB", "sub/deeper/c.txt": "CCC"} {
		if got := readFile(t, filepath.Join(up, filepath.FromSlash(p))); got != want {
			t.Errorf("%s = %q", p, got)
		}
	}
	if fi, err := os.Stat(filepath.Join(up, "empty")); err != nil || !fi.IsDir() {
		t.Error("empty folder not created")
	}
	if !isWindows() {
		if fi, err := os.Lstat(filepath.Join(up, "link-to-file")); err != nil || !fi.Mode().IsRegular() {
			t.Error("link to file not uploaded as regular file")
		}
		if _, err := os.Lstat(filepath.Join(up, "link-to-dir")); !os.IsNotExist(err) {
			t.Error("link to folder should be skipped")
		}
	}

	// Download back, with a skip and SkipExisting.
	dl := t.TempDir()
	writeFile(t, filepath.Join(dl, "tree", "a.txt"), "existing")
	n, err = c.DownloadTree(t.Context(), up, dl, TreeOptions{
		SkipExisting: true,
		OnFile: func(srcP, dst string) error {
			if filepath.Base(dst) == "c.txt" {
				return ErrSkip
			}
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dl, "tree", "a.txt")); got != "existing" {
		t.Errorf("SkipExisting overwrote: %q", got)
	}
	if got := readFile(t, filepath.Join(dl, "tree", "sub", "b.txt")); got != "BB" {
		t.Errorf("b.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dl, "tree", "sub", "deeper", "c.txt")); !os.IsNotExist(err) {
		t.Error("ErrSkip file was downloaded")
	}
	if fi, err := os.Stat(filepath.Join(dl, "tree", "sub", "deeper")); err != nil || !fi.IsDir() {
		t.Error("folder of skipped file missing")
	}

	// Remote links: file link downloaded as content, folder link skipped.
	if !isWindows() {
		os.Symlink("a.txt", filepath.Join(up, "rlink"))
		os.Symlink(filepath.Join(up, "sub"), filepath.Join(up, "rdirlink"))
		dl2 := t.TempDir()
		if _, err := c.DownloadTree(t.Context(), up, dl2, TreeOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, filepath.Join(dl2, "tree", "rlink")); got != "A" {
			t.Errorf("rlink = %q", got)
		}
		if _, err := os.Lstat(filepath.Join(dl2, "tree", "rdirlink")); !os.IsNotExist(err) {
			t.Error("remote folder link should be skipped")
		}
	}

	// A single file works as a tree root, and errors from OnFile stop.
	single := t.TempDir()
	if _, err := c.DownloadTree(t.Context(), filepath.Join(up, "a.txt"), single, TreeOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(single, "a.txt")); got != "A" {
		t.Errorf("single = %q", got)
	}
	stop := errors.New("stop")
	if _, err := c.DownloadTree(t.Context(), up, t.TempDir(), TreeOptions{OnFile: func(_, _ string) error { return stop }}); !errors.Is(err, stop) {
		t.Errorf("OnFile error not propagated: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.UploadTree(ctx, src, s.Root+"/up", TreeOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled UploadTree = %v", err)
	}
}

func TestRelSlash(t *testing.T) {
	tests := []struct{ base, target, want string }{
		{"/", "/a/b", "a/b"},
		{"/a", "/a/b", "b"},
		{"/a", "/ab/c", ""},
		{"/a/b", "/a/b/c/d", "c/d"},
	}
	for _, tc := range tests {
		got, err := relSlash(tc.base, tc.target)
		if tc.want == "" {
			if err == nil {
				t.Errorf("%s %s: want error", tc.base, tc.target)
			}
			continue
		}
		if got != tc.want || err != nil {
			t.Errorf("%s %s = %q %v", tc.base, tc.target, got, err)
		}
	}
}
