package main

// Tests of native Quick Look (board issue #19, F-863): which paths may be shown, and that nothing reaches the system
// panel without a running app (the tests never open a window).

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuickLookPaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := quickLookPaths([]string{"", file, dir, "relative.txt", filepath.Join(dir, "missing")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != file || got[1] != dir {
		t.Fatalf("got %v", got)
	}
	if _, err := quickLookPaths([]string{"sftp://host/a.txt"}); err != errQuickLookLocal {
		t.Fatalf("server path: %v", err)
	}
	if _, err := quickLookPaths([]string{filepath.Join(dir, "missing")}); err != errQuickLookNothing {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := quickLookPaths(nil); err != errQuickLookNothing {
		t.Fatalf("no paths: %v", err)
	}
	// a local file next to a server path is still shown
	if got, err := quickLookPaths([]string{"ftp://h/x", file}); err != nil || len(got) != 1 {
		t.Fatalf("mixed: %v %v", got, err)
	}
}

func TestQuickLookWithoutApp(t *testing.T) {
	q := &QuickLook{app: &App{}}
	file := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if q.Supported() != quickLookSupported {
		t.Fatal("Supported differs from the platform")
	}
	err := q.Show([]string{file})
	if quickLookSupported && err != errNoApp {
		t.Fatalf("Show without the app: %v", err)
	}
	if !quickLookSupported && err != errQuickLookMacOnly {
		t.Fatalf("Show off macOS: %v", err)
	}
	if q.Update([]string{file}) || q.IsOpen() {
		t.Fatal("no panel can be open without the app")
	}
	q.Close() // must not block or panic
}
