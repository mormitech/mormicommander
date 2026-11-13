//go:build unix && !darwin && !ios && !freebsd && !netbsd

package fileops

// The access date of a file, from the stat record's Atim field used by Linux and the other non-BSD systems (the BSD
// spelling is in times_bsd.go). Copies keep the source's access date next to its modification date.

import (
	"io/fs"
	"syscall"
	"time"
)

func atimeOf(info fs.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Atim.Sec), int64(st.Atim.Nsec))
	}
	return info.ModTime()
}
