// Network drive detection on macOS. statfs reports the filesystem type name of the volume holding a folder; shares
// mounted from Finder use smbfs, nfs, afpfs or webdavfs, and FUSE based mounts (sshfs, rclone) use a macfuse, osxfuse
// or fuse-t type. kqueue does not see changes made on the server side of any of these, so they are polled.
package fswatch

import (
	"strings"

	"golang.org/x/sys/unix"
)

func isNetworkPath(path string) bool {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return false
	}
	return isNetworkFSName(unix.ByteSliceToString(st.Fstypename[:]))
}

// isNetworkFSName classifies a macOS/BSD filesystem type name.
func isNetworkFSName(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case "smbfs", "nfs", "afpfs", "webdav", "webdavfs", "cifs", "ftp", "ftpfs":
		return true
	}
	return strings.Contains(name, "fuse")
}
