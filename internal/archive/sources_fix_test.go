// Tests for "move to archive" in the shared source collector (zip and the
// other formats that use CollectSources), from the engine review of issue
// #91: following symbolic links (the default) must never lead to deleting
// the files a link points to, and a file that changed after it was scanned
// is kept rather than deleted. Everything runs in t.TempDir().
package archive

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRemoveSourcesKeepsFollowedLinkTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	os.MkdirAll(outside, 0o755)
	precious := filepath.Join(outside, "precious.txt")
	os.WriteFile(precious, []byte("keep"), 0o644)
	lone := filepath.Join(root, "lone.txt")
	os.WriteFile(lone, []byte("lone"), 0o644)
	src := filepath.Join(root, "src")
	os.MkdirAll(src, 0o755)
	own := filepath.Join(src, "own.txt")
	os.WriteFile(own, []byte("own"), 0o644)
	if err := os.Symlink(outside, filepath.Join(src, "dirlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(lone, filepath.Join(src, "filelink")); err != nil {
		t.Fatal(err)
	}
	files, err := CollectSources([]AddItem{{Source: src}}, PackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var sawPrecious bool
	for _, f := range files {
		if f.ArchivePath == "src/dirlink/precious.txt" {
			sawPrecious = true
			if !f.UnderLink {
				t.Error("file below a followed folder link not marked UnderLink")
			}
		}
	}
	if !sawPrecious {
		t.Fatal("followed folder link was not packed")
	}
	if _, err := RemoveSources(files); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(precious); err != nil || string(b) != "keep" {
		t.Fatalf("file behind a followed folder link lost: %v", err)
	}
	if _, err := os.Stat(lone); err != nil {
		t.Fatal("file behind a followed file link deleted")
	}
	for _, p := range []string{own, filepath.Join(src, "dirlink"), filepath.Join(src, "filelink"), src} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s was not removed", p)
		}
	}
}

func TestRemoveSourcesKeepsChangedFile(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(b, []byte("b"), 0o644)
	files, err := CollectSources([]AddItem{{Source: a}, {Source: b}}, PackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(b, []byte("b grew after the scan"), 0o644)
	later := time.Now().Add(time.Hour)
	os.Chtimes(b, later, later)
	removed, err := RemoveSources(files)
	if err == nil || !strings.Contains(err.Error(), "b.txt") {
		t.Errorf("err = %v, want b.txt reported", err)
	}
	if len(removed) != 1 || removed[0] != a {
		t.Errorf("removed = %v", removed)
	}
	if _, err := os.Stat(b); err != nil {
		t.Error("changed file deleted")
	}
}
