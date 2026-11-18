package vfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// F-743: List reads a folder in blocks and keeps a count of the names read; the result is still sorted by name, as
// os.ReadDir gave it, and the count is gone when the read ends.
func TestReadDirCountedSortedAndCounted(t *testing.T) {
	dir := t.TempDir()
	const n = 2500 // more than two blocks
	for i := n - 1; i >= 0; i-- {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d", i)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	des, err := readDirCounted(dir)
	if err != nil || len(des) != n {
		t.Fatalf("read %d entries, err %v", len(des), err)
	}
	for i, de := range des {
		if want := fmt.Sprintf("f%05d", i); de.Name() != want {
			t.Fatalf("entry %d is %s, want %s", i, de.Name(), want)
		}
	}
	if c := ReadCount(dir); c != -1 {
		t.Fatalf("count after the read: %d, want -1", c)
	}
	l, err := Local{}.List(context.Background(), dir)
	if err != nil || len(l.Entries) != n {
		t.Fatalf("List: %d entries, err %v", len(l.Entries), err)
	}
}

func TestReadCountWhileReading(t *testing.T) {
	dir := t.TempDir()
	c := new(atomic.Int64)
	c.Store(42)
	readCounts.Store(Local{}.Clean(dir), c)
	defer readCounts.Delete(Local{}.Clean(dir))
	if got := ReadCount(dir); got != 42 {
		t.Fatalf("ReadCount = %d, want 42", got)
	}
}

// F-755: on a case-sensitive disk (Linux, a case-sensitive APFS volume) two names that differ only in case are two
// entries. The test is skipped where the disk folds case (the macOS default).
func TestListKeepsNamesDifferingInCase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "A.txt"), []byte("22"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "1" {
		t.Skip("this disk does not tell names apart by case")
	}
	l, err := Local{}.List(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[string]int64{}
	for _, e := range l.Entries {
		sizes[e.Name] = e.Size
	}
	if len(sizes) != 2 || sizes["a.txt"] != 1 || sizes["A.txt"] != 2 {
		t.Fatalf("entries %v, want a.txt (1 byte) and A.txt (2 bytes)", sizes)
	}
}
