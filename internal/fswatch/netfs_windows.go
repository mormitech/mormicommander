// Network drive detection on Windows. UNC paths (\\server\share) are always network folders; for drive letters,
// GetDriveType says DRIVE_REMOTE for mapped network drives. ReadDirectoryChangesW does work on many SMB servers,
// but not on all (some NAS devices and non-Windows servers never send notifications), which is why Total Commander
// users often exclude network drives; polling is the dependable default.
package fswatch

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func isNetworkPath(path string) bool {
	vol := filepath.VolumeName(path)
	if strings.HasPrefix(vol, `\\`) && !strings.HasPrefix(vol, `\\?\`) && !strings.HasPrefix(vol, `\\.\`) {
		return true
	}
	if strings.HasPrefix(strings.ToUpper(vol), `\\?\UNC\`) {
		return true
	}
	if vol == "" {
		return false
	}
	root, err := windows.UTF16PtrFromString(vol + `\`)
	if err != nil {
		return false
	}
	return windows.GetDriveType(root) == windows.DRIVE_REMOTE
}
