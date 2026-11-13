//go:build freebsd || netbsd || openbsd || dragonfly

// Open flags for kqueue watches on the BSDs. They have no O_EVTONLY like macOS, so a plain read-only descriptor is
// used; unmounting a watched volume may then need the panel to leave the folder first, as with other tools there.
package fswatch

import "golang.org/x/sys/unix"

const evtOpenFlags = unix.O_RDONLY
