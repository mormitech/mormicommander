// On macOS one statfs call tells the mount point, the device, the file system type and the sizes of the volume that
// holds a path. Two macOS details matter for a file manager. First, the startup disk is a read-only system volume at
// "/" plus a data volume at /System/Volumes/Data whose folders appear inside "/"; the user knows both as one disk
// (normally "Macintosh HD"), so a path on either is reported with root "/" and the data volume's sizes, which are the
// ones that change. Second, statfs does not give the volume name of the startup disk; Finder publishes it as the
// symbolic link /Volumes/<name> pointing to "/", so the label is taken from that link. Other volumes are named after
// their mount point under /Volumes, which macOS sets from the volume label.
package dirsize

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const dataVolume = "/System/Volumes/Data"

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// VolumeOf describes the volume that holds path.
func VolumeOf(path string) (VolumeInfo, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return VolumeInfo{Path: path}, &os.PathError{Op: "statfs", Path: path, Err: err}
	}
	root := cstr(st.Mntonname[:])
	if root == dataVolume || root == "/" {
		var data unix.Statfs_t
		if unix.Statfs(dataVolume, &data) == nil && data.Blocks > 0 {
			st = data
		}
		v := fromStatfs(path, &st)
		v.Root = "/"
		v.Label = startupDiskName()
		return v, nil
	}
	v := fromStatfs(path, &st)
	v.Root = root
	v.Label = filepath.Base(root)
	return v, nil
}

func fromStatfs(path string, st *unix.Statfs_t) VolumeInfo {
	bs := uint64(st.Bsize)
	return VolumeInfo{
		Path:       path,
		FileSystem: cstr(st.Fstypename[:]),
		Device:     cstr(st.Mntfromname[:]),
		Total:      st.Blocks * bs,
		Free:       st.Bfree * bs,
		Available:  st.Bavail * bs,
		BlockSize:  bs,
		ReadOnly:   st.Flags&unix.MNT_RDONLY != 0,
		Remote:     st.Flags&unix.MNT_LOCAL == 0,
		Removable:  st.Flags&unix.MNT_REMOVABLE != 0,
	}
}

// startupDiskName finds the /Volumes entry that links to "/" ("Macintosh HD" unless the user renamed the disk).
func startupDiskName() string {
	entries, _ := os.ReadDir("/Volumes")
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		if t, err := os.Readlink(filepath.Join("/Volumes", e.Name())); err == nil && t == "/" {
			return e.Name()
		}
	}
	return "Macintosh HD"
}
