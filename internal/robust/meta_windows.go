// Windows part of Replace. NTFS files can have hard links too; their number comes from the file's handle
// information. Ownership is left alone on Windows (files there carry an owner in their security descriptor, which
// a rename within the same folder does not need to change for the user's own archives).
package robust

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func linkCount(p string, _ fs.FileInfo) uint64 {
	f, err := os.Open(p)
	if err != nil {
		return 1
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info) != nil {
		return 1
	}
	return uint64(info.NumberOfLinks)
}

func keepOwner(fs.FileInfo, string) {}
