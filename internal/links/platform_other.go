//go:build !unix && !windows

// Platforms that are neither Unix nor Windows (js/wasm, wasip1, plan9) are
// not targets of the app, but the package should still build there so the
// pure-Go shortcut parsers stay usable. Symbolic links go through the
// standard library, which reports its own "not supported" errors; there is no
// portable file identity, so loop detection falls back to comparing cleaned
// paths, which still catches the common "link points to its own parent".

package links

import (
	"io/fs"
	"os"
)

func platformSymlink(target, link string, isDir bool) error {
	return os.Symlink(target, link)
}

func platformJunction(target, link string) error {
	return &os.LinkError{Op: "junction", Old: target, New: link, Err: ErrUnsupported}
}

func fileIdentity(path string, fi fs.FileInfo) (FileID, bool) { return FileID{}, false }

func linkCount(path string, fi fs.FileInfo) uint64 { return 0 }

func isJunction(path string, fi fs.FileInfo) bool { return false }

func isCrossDeviceErr(err error) bool { return false }

func isPrivilegeErr(err error) bool { return false }

func isLoopErr(err error) bool { return false }
