package dirsize

import "golang.org/x/sys/unix"

// Space reports the size and free space of the volume that holds path.
func Space(path string) (Usage, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Usage{}, err
	}
	bs := statfsBlock(&st)
	return Usage{Total: st.Blocks * bs, Available: st.Bavail * bs}, nil
}

// statfsBlock is the unit of statfs's block counts: f_frsize; f_bsize is only the preferred I/O size and differs from
// it on some NFS, CIFS and FUSE mounts.
func statfsBlock(st *unix.Statfs_t) uint64 {
	if st.Frsize != 0 {
		return uint64(st.Frsize)
	}
	return uint64(st.Bsize)
}
