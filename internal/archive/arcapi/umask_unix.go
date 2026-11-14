//go:build unix

// On Unix-like systems every process has a umask: permission bits that newly
// created files must not get (typically 022, "nobody else may write"). Tools
// that unpack archives (tar without -p, unzip, Finder's Archive Utility)
// apply it to the modes stored in the archive, so a world-writable file in
// someone else's archive does not stay world-writable on this disk. The
// unpackers set modes with chmod, which ignores the umask, so this file reads
// the umask once, when the program starts, for them to apply themselves.
package arcapi

import (
	"io/fs"
	"syscall"
)

// processUmask is read at start-up. Reading the umask means setting it, so
// it is done once here, before the program starts any work that creates
// files, and put straight back.
var processUmask = func() fs.FileMode {
	old := syscall.Umask(0o022)
	syscall.Umask(old)
	return fs.FileMode(old) & fs.ModePerm
}()
