package fileops

// TC's "big file" copy mode keeps huge copies from pushing everything else out of the system's file cache. Linux has
// no flag for that on an ordinary open file, so the copy declares sequential reading at the start and, at the end,
// tells the kernel that the cached pages of both files are no longer needed (POSIX_FADV_DONTNEED).

import (
	"os"

	"golang.org/x/sys/unix"
)

func noCacheStart(f *os.File) { unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_SEQUENTIAL) }

func noCacheEnd(f *os.File) {
	f.Sync() // dirty pages can't be dropped
	unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
}
