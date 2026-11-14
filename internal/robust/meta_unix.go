//go:build unix

// Unix part of Replace: how many names a file has (its hard-link count) and handing a new file the old file's owner
// and group. Only root may give a file to another user, so when that is refused the group alone is tried, which an
// ordinary user may set to any group they belong to; if that fails too the new file simply keeps its own.
package robust

import (
	"io/fs"
	"os"
	"syscall"
)

func linkCount(_ string, fi fs.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}

func keepOwner(old fs.FileInfo, p string) {
	st, ok := old.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	cur, err := os.Lstat(p)
	if err != nil {
		return
	}
	if cs, ok := cur.Sys().(*syscall.Stat_t); ok && cs.Uid == st.Uid && cs.Gid == st.Gid {
		return
	}
	if os.Lchown(p, int(st.Uid), int(st.Gid)) != nil {
		_ = os.Lchown(p, -1, int(st.Gid))
	}
}
