// This file fills the Linux-specific parts of an Item. The classic stat call has no birth time (only statx does, and
// not on every file system), so CreateTime stays zero and the creationdate column renders empty instead of showing a
// misleading inode change time. Access time and owner come straight from the stat record.

//go:build linux

package listing

import (
	"io/fs"
	"syscall"
	"time"
)

func fillPlatform(it *Item, fi fs.FileInfo) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	it.AccessTime = time.Unix(int64(st.Atim.Sec), int64(st.Atim.Nsec))
	it.Owner = userName(st.Uid)
	it.Group = groupName(st.Gid)
}
