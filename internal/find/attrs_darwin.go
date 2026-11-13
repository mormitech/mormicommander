// This file reads the macOS-specific attribute flags for the attribute filter. Besides
// the Unix permission bits, macOS keeps BSD file flags in st_flags: UF_HIDDEN is what
// Finder uses to hide a file that is not a dotfile (chflags hidden), and UF_COMPRESSED
// marks files that HFS+/APFS store transparently compressed.

package find

import (
	"io/fs"
	"syscall"
)

const (
	ufCompressed = 0x20   // UF_COMPRESSED from <sys/stat.h>
	ufHidden     = 0x8000 // UF_HIDDEN from <sys/stat.h>
)

// sysAttrs adds the permission-derived bits and the BSD flags.
func sysAttrs(name string, info fs.FileInfo) Attr {
	a := modeAttrs(info)
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		if st.Flags&ufHidden != 0 {
			a |= AttrHidden
		}
		if st.Flags&ufCompressed != 0 {
			a |= AttrCompressed
		}
	}
	return a
}
