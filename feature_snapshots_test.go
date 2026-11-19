package main

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/snapshots"
	"mormicommander/internal/vfs"
)

func TestSnapshotVersionsRefusesNonLocal(t *testing.T) {
	for _, p := range []string{"", "sftp://host/a", "relative/a"} {
		if _, err := findSnapshotVersions(p); err == nil || !strings.Contains(err.Error(), "this computer") {
			t.Errorf("%q: err = %v", p, err)
		}
	}
}

func TestSnapshotVersionsTimeout(t *testing.T) {
	oldEnv, oldT := snapshotEnv, snapshotTimeout
	defer func() { snapshotEnv, snapshotTimeout = oldEnv, oldT }()
	block := make(chan struct{})
	defer close(block)
	snapshotTimeout = 50 * time.Millisecond
	snapshotEnv = func() snapshots.Env {
		return snapshots.Env{GOOS: "linux", Stat: func(string) (fs.FileInfo, error) { <-block; return nil, fs.ErrNotExist },
			ReadDir: func(string) ([]fs.DirEntry, error) { return nil, fs.ErrNotExist }, ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
			Run: func(context.Context, string, ...string) (string, error) { return "", fs.ErrNotExist }}
	}
	if _, err := findSnapshotVersions(filepath.Join(t.TempDir(), "a")); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("err = %v", err)
	}
}

func TestSnapshotVersionsDemo(t *testing.T) {
	f := &FS{app: &App{mock: vfs.NewMock()}}
	if _, err := f.SnapshotVersions("/a"); err == nil || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("err = %v", err)
	}
}
