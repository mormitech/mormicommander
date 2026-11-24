//go:build darwin || linux

package dirsize

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A sparse disk image (Docker's Docker.raw) claims far more than it takes: the size a folder's column shows is the
// space on disk, the logical size stays in Bytes.
func TestSizeIsSpaceOnDiskForSparseFiles(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "disk.raw"))
	if err != nil {
		t.Fatal(err)
	}
	const length = 1 << 30
	if err := f.Truncate(length); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(make([]byte, 4096), 0); err != nil {
		t.Fatal(err)
	}
	f.Close()
	r, err := Calculate(context.Background(), dir, Options{SameVolume: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Bytes != length {
		t.Errorf("bytes %d, want %d", r.Bytes, length)
	}
	if r.Allocated >= length/2 {
		t.Skipf("this file system did not keep the file sparse (%d bytes on disk)", r.Allocated)
	}
	if r.Size != r.Allocated {
		t.Errorf("size %d, want the size on disk %d", r.Size, r.Allocated)
	}
}
