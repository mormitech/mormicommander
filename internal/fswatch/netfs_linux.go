// Network drive detection on Linux. statfs returns a magic number for the filesystem type; NFS, SMB/CIFS, AFS, Coda,
// Ceph, 9P and FUSE (sshfs, rclone, gvfs) mounts are treated as network drives, because inotify only sees changes
// made through the local kernel and misses everything done by the server or other clients. FUSE is included even
// though some FUSE filesystems are local (ntfs-3g): polling those is merely slower, while missing changes on sshfs
// would be wrong.
package fswatch

import "golang.org/x/sys/unix"

var networkMagic = map[uint32]bool{
	0x6969:     true, // NFS
	0x517B:     true, // SMB
	0xFF534D42: true, // CIFS
	0xFE534D42: true, // SMB2
	0x5346414F: true, // AFS
	0x73757245: true, // Coda
	0x00C36400: true, // Ceph
	0x01021997: true, // 9P (v9fs, also WSL drive mounts)
	0x65735546: true, // FUSE
	0x564C:     true, // NCP
}

func isNetworkPath(path string) bool {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return false
	}
	return networkMagic[uint32(st.Type)]
}
