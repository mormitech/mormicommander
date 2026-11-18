package fileops

import (
	"io/fs"
	"syscall"

	"golang.org/x/sys/windows"
)

// hiddenTarget says whether a file that would be overwritten has the hidden or the system attribute.
func hiddenTarget(path string, fi fs.FileInfo) bool {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes&(windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_SYSTEM) != 0
	}
	p, err := windows.UTF16PtrFromString(lp(path))
	if err != nil {
		return false
	}
	a, err := windows.GetFileAttributes(p)
	return err == nil && a&(windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_SYSTEM) != 0
}
