// This file holds the Windows forms of two small platform hooks used by the text formatters. The attribute column of
// cm_CopyDetailsToClip shows TC's "rahs" letters (read-only, archive, hidden, system) with '-' for a missing one, read
// from the Win32 attribute word the os package already fetched. cm_CopyNetNamesToClip turns a path on a mapped drive,
// such as Z:\docs\a.txt, into its network name \\server\share\docs\a.txt with WNetGetConnectionW, so the text can be
// pasted on another machine; paths on local drives are returned unchanged.

package clipfiles

import (
	"io/fs"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// attrString returns TC-style attribute letters for the details column.
func attrString(fi fs.FileInfo) string {
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return fi.Mode().String()
	}
	a := d.FileAttributes
	flag := func(bit uint32, c byte) byte {
		if a&bit != 0 {
			return c
		}
		return '-'
	}
	return string([]byte{
		flag(windows.FILE_ATTRIBUTE_READONLY, 'r'),
		flag(windows.FILE_ATTRIBUTE_ARCHIVE, 'a'),
		flag(windows.FILE_ATTRIBUTE_HIDDEN, 'h'),
		flag(windows.FILE_ATTRIBUTE_SYSTEM, 's'),
	})
}

var procWNetGetConnectionW = windows.NewLazySystemDLL("mpr.dll").NewProc("WNetGetConnectionW")

// uncName returns the UNC name of a path on a mapped network drive, or path itself.
func uncName(path string) string {
	vol := filepath.VolumeName(path)
	if len(vol) != 2 || vol[1] != ':' {
		return path // already UNC, or no drive letter
	}
	local, err := windows.UTF16PtrFromString(vol)
	if err != nil {
		return path
	}
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := procWNetGetConnectionW.Call(uintptr(unsafe.Pointer(local)), uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&n)))
	if r != 0 { // NO_ERROR is 0; anything else means a local drive or an unreachable mapping
		return path
	}
	return windows.UTF16ToString(buf) + path[len(vol):]
}
