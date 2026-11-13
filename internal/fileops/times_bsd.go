//go:build darwin || ios || freebsd || netbsd

package fileops

// The access date of a file is in the stat record, under a name that differs between the BSD family (Atimespec, here)
// and Linux and the others (Atim, in times_atim.go). Copies keep the source's access date next to its modification
// date, as TC does.

import (
	"io/fs"
	"syscall"
	"time"
)

func atimeOf(info fs.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Atimespec.Sec), int64(st.Atimespec.Nsec))
	}
	return info.ModTime()
}
