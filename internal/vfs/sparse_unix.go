//go:build !windows

package vfs

import (
	"io/fs"
	"syscall"
)

// sparseMin is how much smaller than its length a file must be on disk before the size column shows its size on disk
// instead: a disk image such as Docker's Docker.raw claims hundreds of gigabytes and holds a few. Small differences
// (compression, the last block) are left alone, so ordinary files keep showing their exact length.
const sparseMin = 64 << 20

// sparseDisk is a regular file's size on disk when it is a sparse file far smaller on disk than its length, else -1.
// Online-only cloud files take no space either, but they are files in the cloud and keep their length.
func sparseDisk(info fs.FileInfo, flags int) int64 {
	if flags&FlagCloud != 0 || !info.Mode().IsRegular() {
		return -1
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return -1
	}
	disk := int64(st.Blocks) * 512
	if size := info.Size(); size-disk >= sparseMin && disk < size/2 {
		return disk
	}
	return -1
}
