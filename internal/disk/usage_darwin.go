// UsageOf answers "how big is this disk and how much can I still write?" for
// the drive info line above each panel. On macOS statfs reports block counts
// in units of f_bsize, the file system's fundamental block size, so the byte
// totals are the counts times Bsize. Bavail (not Bfree) is used for free
// space because it is what an ordinary user can actually write.

package disk

import "golang.org/x/sys/unix"

// UsageOf reports the volume that holds path.
func UsageOf(path string) (Usage, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Usage{}, err
	}
	bs := uint64(st.Bsize)
	return Usage{Total: st.Blocks * bs, Free: st.Bavail * bs}, nil
}
