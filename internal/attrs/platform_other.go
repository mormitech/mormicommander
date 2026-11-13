//go:build !unix && !windows

// Fallback for platforms that are neither Unix nor Windows (Plan 9, js/wasm).
// mormiCommander does not ship there; this file only keeps the package
// compiling. Reading works through the portable FileInfo fields and every
// change except the modification and access time reports itself unsupported.
package attrs

import (
	"io/fs"
	"os"
	"strconv"
	"time"
)

const (
	canSetCreated  = false
	canReadCreated = false
	canHideFlag    = false
)

func fillPlatform(info *Info, _ fs.FileInfo, _ bool) {
	if info.Hidden {
		info.Win |= AttrHidden
	}
	if info.Kind == KindDir {
		info.Win |= AttrDirectory
	}
}

func unixWinAttr(_ Kind, _ fs.FileMode, _ bool) WinAttr { return 0 }

func groupName(gid int) string { return strconv.Itoa(gid) }

func lookupUID(string) (int, error) { return -1, ErrUnsupported }
func lookupGID(string) (int, error) { return -1, ErrUnsupported }

func chmodPath(path string, perm fs.FileMode) error { return os.Chmod(path, perm) }

func chownPath(string, int, int, bool) error { return ErrUnsupported }

func setWinAttrs(string, WinAttr, bool) error { return ErrUnsupported }

func setHiddenFlag(string, bool, bool) error { return ErrUnsupported }

func setTimes(path string, mtime, atime, ctime *time.Time, _ bool) error {
	if ctime != nil {
		return ErrUnsupported
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	m, a := fi.ModTime(), fi.ModTime()
	if mtime != nil {
		m = *mtime
	}
	if atime != nil {
		a = *atime
	}
	return os.Chtimes(path, a, m)
}

func extraAccounts(bool) []Account { return nil }

func platformCaps() Capabilities { return Capabilities{} }

func ownerLabel(uid int) string { return strconv.Itoa(uid) }

// currentUID: there are no Unix owners here, so no planned change can open a
// folder by its owner bits.
func currentUID() int { return -2 }
