// macOS details of the comparison. APFS and HFS+ volumes ignore case by default, so CaseAuto matches names without
// regard to case, as Finder does. A file is hidden when its name starts with a dot or when it carries the UF_HIDDEN
// flag (set by "chflags hidden", used for folders like ~/Library), which matches what Finder and the panel hide.
package dircompare

import (
	"io/fs"
	"syscall"
)

const defaultCaseInsensitive = true

// ufHidden is UF_HIDDEN from <sys/stat.h>.
const ufHidden = 0x8000

func isHidden(name string, info fs.FileInfo) bool {
	if len(name) > 1 && name[0] == '.' {
		return true
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Flags&ufHidden != 0
	}
	return false
}
