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
