// This file reads the hard-link count of a file on Unix systems (macOS, Linux,
// the BSDs). Saving an edited compare document normally replaces the file by
// renaming a temporary copy over it, but that would cut a hard-linked file off
// from its other names, so the save needs to know when a file has more than one.

//go:build unix

package compare

import (
	"os"
	"syscall"
)

// linkCount returns the number of hard links of the file described by st.
func linkCount(_ string, st os.FileInfo) uint64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Nlink)
	}
	return 1
}
