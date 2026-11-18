package cloudfile

import (
	"io/fs"
	"syscall"
)

// SFDataless is SF_DATALESS from <sys/stat.h>: the file's (or folder's) data lives with a File Provider and is
// fetched when it is read.
const SFDataless = 0x40000000

func placeholder(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Flags&SFDataless != 0
}
