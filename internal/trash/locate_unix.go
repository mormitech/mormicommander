//go:build darwin || linux

package trash

// The parts of finding the trash that macOS and Linux share: which volume a path is on (its device number) and the
// folder that volume is mounted at, because both systems keep a separate trash at the top of every volume other than
// the one with the home folder.

import (
	"os"
	"path/filepath"
	"syscall"
)

// device is the device number of the nearest existing folder at or above path (the item itself may be gone, and so
// may the folder that held it, when that went to the trash later). ok is false when none can be read.
func device(path string) (uint64, string, bool) {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		if fi, err := os.Lstat(p); err == nil {
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
				return uint64(st.Dev), p, true
			}
			return 0, "", false
		}
		if filepath.Dir(p) == p {
			return 0, "", false
		}
	}
}

// mountPoint is the top folder of the volume that holds path: the highest folder above it on the same device.
func mountPoint(path string) (string, bool) {
	dev, p, ok := device(path)
	if !ok {
		return "", false
	}
	for {
		up := filepath.Dir(p)
		if up == p {
			return p, true
		}
		d, _, ok := device(up)
		if !ok || d != dev {
			return p, true
		}
		p = up
	}
}

// sameDevice says whether a and b are on the same volume.
func sameDevice(a, b string) bool {
	da, _, ok1 := device(a)
	db, _, ok2 := device(b)
	return ok1 && ok2 && da == db
}
