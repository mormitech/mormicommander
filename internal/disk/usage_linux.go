// UsageOf answers "how big is this disk and how much can I still write?" for
// the drive info line above each panel. On Linux statfs reports block counts
// (f_blocks, f_bavail) in units of f_frsize, the fragment size; f_bsize is
// only the preferred I/O size. On ext4 the two are equal, which hides the
// difference, but on some NFS, CIFS and FUSE mounts they are not, and
// multiplying by Bsize gives sizes that are off by a large factor. So Frsize
// is used, with Bsize only as a fallback for old kernels that leave it zero.

package disk

import "golang.org/x/sys/unix"

// UsageOf reports the volume that holds path.
func UsageOf(path string) (Usage, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Usage{}, err
	}
	return usageFromStatfs(&st), nil
}

// usageFromStatfs turns statfs block counts into bytes (see the file comment).
func usageFromStatfs(st *unix.Statfs_t) Usage {
	bs := uint64(st.Frsize)
	if bs == 0 {
		bs = uint64(st.Bsize)
	}
	return Usage{Total: st.Blocks * bs, Free: st.Bavail * bs}
}
