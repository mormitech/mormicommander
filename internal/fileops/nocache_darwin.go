package fileops

// TC's "big file" copy mode keeps huge copies from pushing everything else out of the system's file cache. On macOS
// that is the F_NOCACHE flag on both open files: the bytes go straight through without being kept in memory.

import (
	"os"

	"golang.org/x/sys/unix"
)

func noCacheStart(f *os.File) { unix.FcntlInt(f.Fd(), unix.F_NOCACHE, 1) }
func noCacheEnd(f *os.File)   {}
