// This file reads the hard-link count of a file on Windows. NTFS supports hard
// links too (mklink /H), and saving an edited compare document by renaming a
// temporary copy over it would cut such a file off from its other names, so
// the save asks the file system how many names the file has.

package compare

import (
	"os"
	"syscall"
)

// linkCount returns the number of hard links of the file at path, or 1 when
// it cannot be determined.
func linkCount(path string, _ os.FileInfo) uint64 {
	f, err := os.Open(path)
	if err != nil {
		return 1
	}
	defer f.Close()
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info); err != nil {
		return 1
	}
	return uint64(info.NumberOfLinks)
}
