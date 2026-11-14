// This file fills the Linux-specific parts of an Item. The classic stat call has no birth time, so the creation date
// is asked from statx, which returns it when both the kernel and the file system record one (ext4, btrfs, xfs, tmpfs
// on newer kernels). When statx is missing (old kernels, some sandboxes) or the file system has no birth time,
// CreateTime stays zero and the creationdate column renders empty instead of showing a misleading inode change time.
// Access time and owner come straight from the stat record.

//go:build linux

package listing

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// noStatx is set once statx turns out not to exist on this system, so a big folder does not pay for one failing
// system call per file.
var noStatx atomic.Bool

func fillPlatform(it *Item, fi fs.FileInfo) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	it.AccessTime = time.Unix(int64(st.Atim.Sec), int64(st.Atim.Nsec))
	it.Owner = userName(st.Uid)
	it.Group = groupName(st.Gid)
	if it.Dir != "" {
		if bt, ok := birthTime(filepath.Join(it.Dir, it.Name)); ok {
			it.CreateTime = bt
		}
	}
}

// birthTime returns the birth time of path itself (a link is not followed, matching how the item is listed), and
// false when the system or the file system cannot tell.
func birthTime(path string) (time.Time, bool) {
	if noStatx.Load() {
		return time.Time{}, false
	}
	var stx unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BTIME, &stx)
	if err != nil {
		if errors.Is(err, unix.ENOSYS) {
			noStatx.Store(true)
		}
		return time.Time{}, false
	}
	if stx.Mask&unix.STATX_BTIME == 0 {
		return time.Time{}, false
	}
	return time.Unix(stx.Btime.Sec, int64(stx.Btime.Nsec)), true
}
