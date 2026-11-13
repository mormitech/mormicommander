package fileops

// On Windows the volume of a path is found with GetVolumePathName, which also resolves folders mounted into another
// drive (C:\mnt\usb is its own volume even though it starts with C:). The comparison ignores case, as Windows does.

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows"
)

func volumeOf(p string) (string, bool) {
	name, err := windows.UTF16PtrFromString(lp(p))
	if err != nil {
		return "", false
	}
	buf := make([]uint16, 1024)
	if err := windows.GetVolumePathName(name, &buf[0], uint32(len(buf))); err != nil {
		return "", false
	}
	v := strings.TrimPrefix(windows.UTF16ToString(buf), `\\?\`)
	return strings.ToLower(v), true
}

// isCrossDeviceOS recognises Windows' own "not the same device" error from MoveFileEx.
func isCrossDeviceOS(err error) bool { return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) }
