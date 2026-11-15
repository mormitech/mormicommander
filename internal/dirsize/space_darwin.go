package dirsize

import "golang.org/x/sys/unix"

// Space reports the size and free space of the volume that holds path.
func Space(path string) (Usage, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Usage{}, err
	}
	v := fromStatfs(path, &st)
	return Usage{Total: v.Total, Available: v.Available}, nil
}
