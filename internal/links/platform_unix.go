//go:build unix

// On macOS, Linux and the BSDs links are plain POSIX: symlink(2) makes a
// symbolic link whatever the target is, link(2) makes a hard link, and stat
// gives a device and inode number that identify a file uniquely, which is
// what loop detection and the hard link count need. Junctions are an NTFS
// feature and do not exist here, so asking for one returns ErrUnsupported
// instead of silently making a symlink the user did not ask for.

package links

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

func platformSymlink(target, link string, isDir bool) error {
	return os.Symlink(target, link)
}

func platformJunction(target, link string) error {
	return &os.LinkError{Op: "junction", Old: target, New: link, Err: ErrUnsupported}
}

func fileIdentity(path string, fi fs.FileInfo) (FileID, bool) {
	if fi == nil {
		return FileID{}, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return FileID{}, false
	}
	return FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, true
}

func linkCount(path string, fi fs.FileInfo) uint64 {
	if fi == nil {
		return 0
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 0
}

func isJunction(path string, fi fs.FileInfo) bool { return false }

func isCrossDeviceErr(err error) bool { return errors.Is(err, syscall.EXDEV) }

func isPrivilegeErr(err error) bool { return false }

func isLoopErr(err error) bool { return errors.Is(err, syscall.ELOOP) }
