// This file checks that an FTP tree download stops with an error on a server name that cannot be stored safely in
// the local folder, the same way an SFTP tree download does, instead of silently leaving the entry out. Only Windows
// has such names that survive the walk ("..\x", "CON"); elsewhere every plain child name is storable.

package ftp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"mormicommander/internal/remotefs"
)

func TestDownloadTreeRefusesUnsafeName(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("every name the walk passes on is storable on this OS")
	}
	srv := newTestServer(t, func(s *testServer) {
		s.canned = map[string]string{
			"/tree": "-rw-r--r--   1 u g    4 Jan  1  2024 ..\\evil.txt\r\n",
		}
	})
	srv.write("tree/keep.txt", "KEEP")
	cfg := srv.cfg()
	cfg.DisableMLSD = true
	c := dialT(t, cfg)
	parent := t.TempDir()
	back := filepath.Join(parent, "back")
	_, err := c.DownloadTree(context.Background(), "/tree", back, TreeOptions{})
	if !errors.Is(err, remotefs.ErrUnsafeName) {
		t.Fatalf("DownloadTree error = %v, want ErrUnsafeName", err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "evil.txt")); err == nil {
		t.Fatal("evil.txt was written outside the destination")
	}
}
