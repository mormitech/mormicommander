//go:build unix

// On Unix-like systems several directory entries can share one file (hard
// links). When packing, tar stores the data once and records the other names
// as links to the first, just as GNU tar does, so the archive stays small
// and unpacking restores the links. This file reads the device and inode
// numbers that identify a file for that purpose.
package tarfmt

import (
	"io/fs"
	"syscall"
)

// fileKey identifies a file on disk independent of its name.
type fileKey struct{ dev, ino uint64 }

// hardLinkKey returns the identity of a regular file that has more than one
// name, and ok=false for everything else.
func hardLinkKey(fi fs.FileInfo) (fileKey, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || !fi.Mode().IsRegular() || st.Nlink < 2 {
		return fileKey{}, false
	}
	return fileKey{dev: uint64(st.Dev), ino: uint64(st.Ino)}, true
}
