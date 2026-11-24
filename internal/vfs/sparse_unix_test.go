//go:build !windows

package vfs

import (
	"os"
	"path/filepath"
	"testing"
)

// A sparse file far smaller on disk than its length carries its size on disk in the listing (a sixth row value), so
// the size column shows what Docker.raw and the like really take; an ordinary file keeps five values.
func TestListingCarriesSizeOnDiskOfSparseFiles(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "disk.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 30); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), make([]byte, 100000), 0o644); err != nil {
		t.Fatal(err)
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, de := range des {
		e := localEntry(dir, de)
		row := Rows([]Entry{e})[0]
		switch e.Name {
		case "plain.txt":
			if e.Sparse || len(row) != 5 {
				t.Errorf("plain file: sparse=%v row %v, want no size on disk", e.Sparse, row)
			}
		case "disk.raw":
			if !e.Sparse {
				t.Skip("this file system did not keep the file sparse")
			}
			if e.Size != 1<<30 || e.Disk >= 1<<29 || len(row) != 6 || row[5] != e.Disk {
				t.Errorf("sparse file: size %d disk %d row %v", e.Size, e.Disk, row)
			}
		}
	}
}
