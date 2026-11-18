package trash

// Tests of Original (F-501): the old place of an item picked in a trash folder, read from the trash's record. Only
// temporary folders are used; the real trash is never read or changed.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOriginalFromTrashInfo(t *testing.T) {
	root := t.TempDir()
	files := filepath.Join(root, "Trash", "files")
	fakeTrash(t, files)
	src := filepath.Join(root, "work", "a b.txt")
	writeFile(t, filepath.Join(files, "a b.txt"), "x")
	writeFile(t, filepath.Join(root, "Trash", "info", "a b.txt.trashinfo"),
		"[Trash Info]\nPath="+filepath.ToSlash(filepath.Join(root, "work", "a%20b.txt"))+"\nDeletionDate=2026-01-02T03:04:05\n")
	got, err := Original(filepath.Join(files, "a b.txt"))
	if err != nil || got != src {
		t.Fatalf("Original = %q %v, want %q", got, err, src)
	}
	// and it can be put back there, record and all
	if err := Restore(filepath.Join(files, "a b.txt"), got); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(src); err != nil {
		t.Fatal("not put back")
	}
	if _, err := os.Lstat(filepath.Join(root, "Trash", "info", "a b.txt.trashinfo")); err == nil {
		t.Error("the record was left behind")
	}
}

func TestOriginalRelativeToVolumeTop(t *testing.T) {
	top := t.TempDir()
	files := filepath.Join(top, ".Trash-1000", "files")
	writeFile(t, filepath.Join(files, "r.txt"), "x")
	writeFile(t, filepath.Join(top, ".Trash-1000", "info", "r.txt.trashinfo"), "[Trash Info]\nPath=sub/r.txt\nDeletionDate=2026-01-02T03:04:05\n")
	got, err := Original(filepath.Join(files, "r.txt"))
	if err != nil || got != filepath.Join(top, "sub", "r.txt") {
		t.Fatalf("Original = %q %v", got, err)
	}
}

func TestOriginalFromRecycleRecord(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "$Recycle.Bin", "S-1-5-21-1")
	src := filepath.Join(root, "docs", "letter.docx")
	writeFile(t, filepath.Join(bin, "$RAB12CD.docx"), "dear")
	if err := os.WriteFile(filepath.Join(bin, "$IAB12CD.docx"), recycleRecord(2, src, time.Now()), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Original(filepath.Join(bin, "$RAB12CD.docx"))
	if err != nil || got != src {
		t.Fatalf("Original = %q %v", got, err)
	}
}

func TestOriginalWithoutRecord(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plain.txt"), "x")
	for _, p := range []string{filepath.Join(dir, "plain.txt"), filepath.Join(dir, "files", "gone.txt"), filepath.Join(dir, "$Rnone.txt")} {
		if _, err := Original(p); !errors.Is(err, ErrNoRecord) {
			t.Errorf("Original(%q) err = %v, want ErrNoRecord", p, err)
		}
	}
	if _, err := Original("rel/x"); err == nil {
		t.Error("a relative path was accepted")
	}
}
