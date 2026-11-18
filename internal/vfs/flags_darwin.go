package vfs

import (
	"io/fs"
	"syscall"

	"mormicommander/internal/cloudfile"
)

// ufHidden is UF_HIDDEN from <sys/stat.h>: Finder hides files with this flag (chflags hidden), dotfile or not.
const ufHidden = 0x8000

// sysFlags reads what the system says about a file beyond its mode: hidden by flag, executable, read-only, online-only.
func sysFlags(info fs.FileInfo, name string) int {
	f := unixModeFlags(info)
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Flags&ufHidden != 0 {
		f |= FlagHidden
	}
	if cloudfile.Placeholder(info) {
		f |= FlagCloud
	}
	return f
}
