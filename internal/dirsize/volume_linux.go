// On Linux, statfs gives the sizes of the volume that holds a path, but not where it is mounted, what it is called or
// which device it is. Those come from /proc/self/mountinfo (matched by the path and its device numbers, so bind mounts
// and stacked mounts resolve correctly) and from the label links udev keeps in /dev/disk/by-label. Whether a disk is
// removable is read from /sys/class/block, best effort, so the drive bar can show a USB icon and offer to eject.
package dirsize

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const stRdonly = 1 // ST_RDONLY in statfs flags

// Locations of the system files, variables so tests could point them elsewhere.
var (
	mountinfoPath = "/proc/self/mountinfo"
	byLabelDir    = "/dev/disk/by-label"
	sysBlockDir   = "/sys/class/block"
)

// VolumeOf describes the volume that holds path.
func VolumeOf(path string) (VolumeInfo, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return VolumeInfo{Path: path}, &os.PathError{Op: "statfs", Path: path, Err: err}
	}
	bs := uint64(st.Bsize)
	v := VolumeInfo{
		Path:      path,
		Root:      "/",
		Total:     st.Blocks * bs,
		Free:      st.Bfree * bs,
		Available: st.Bavail * bs,
		BlockSize: bs,
		ReadOnly:  uint64(st.Flags)&stRdonly != 0,
	}
	if st.Frsize > 0 {
		v.BlockSize = uint64(st.Frsize)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		abs = r
	}
	var major, minor uint64
	var ss unix.Stat_t
	haveDev := unix.Stat(abs, &ss) == nil
	if haveDev {
		major, minor = uint64(unix.Major(uint64(ss.Dev))), uint64(unix.Minor(uint64(ss.Dev)))
	}
	if f, err := os.Open(mountinfoPath); err == nil {
		mounts := parseMountinfo(f)
		f.Close()
		if m, ok := findMount(mounts, abs, major, minor, haveDev); ok {
			v.Root = m.MountPoint
			v.FileSystem = m.FSType
			v.Device = m.Source
			v.Remote = networkFS[m.FSType]
			if strings.HasPrefix(m.Source, "/dev/") {
				v.Label = labelOf(m.Source)
				v.Removable = removable(m.Source)
			}
			if m.FSType == "iso9660" || m.FSType == "udf" {
				v.Removable = true
			}
		}
	}
	return v, nil
}

// labelOf finds the /dev/disk/by-label link that resolves to the same device node.
func labelOf(dev string) string {
	target, err := filepath.EvalSymlinks(dev)
	if err != nil {
		target = dev
	}
	entries, err := os.ReadDir(byLabelDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		t, err := filepath.EvalSymlinks(filepath.Join(byLabelDir, e.Name()))
		if err == nil && t == target {
			return unescapeHex(e.Name())
		}
	}
	return ""
}

// removable reads /sys/class/block/<disk>/removable for the disk that holds a partition.
func removable(dev string) bool {
	target, err := filepath.EvalSymlinks(dev)
	if err != nil {
		target = dev
	}
	sys, err := filepath.EvalSymlinks(filepath.Join(sysBlockDir, filepath.Base(target)))
	if err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(sys, "partition")); err == nil {
		sys = filepath.Dir(sys)
	}
	b, err := os.ReadFile(filepath.Join(sys, "removable"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}
