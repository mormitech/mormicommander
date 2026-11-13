//go:build unix

package fileops

// On Unix-like systems the owner and group of a copy can be set with lchown, but only root (or the owner, for the
// group, when they belong to it) may do so; a refusal is expected and ignored by the caller. Rights travel with the
// mode bits (attrs.go), and there are no Windows-style attributes and no settable creation date to copy.

import (
	"io/fs"
	"os"
	"syscall"
)

func copyOwner(dst string, info fs.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Lchown(lp(dst), int(st.Uid), int(st.Gid))
}

func setCreationTime(dst string, info fs.FileInfo) error { return nil }
func copyPlatformAttrs(src, dst string) error            { return nil }

// linkCount is the number of names (hard links) the file has; wipe leaves the data alone when another name shares it.
func linkCount(p string, info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}
