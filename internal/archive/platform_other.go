//go:build !windows

// Unix (macOS, Linux, BSD) helpers for extraction and packing. Unix file
// names may contain anything except "/" and NUL, so SanitizeName only replaces
// NUL bytes; the slash cannot appear because names are already split on it.
// Unix has no hidden or system attribute bits (hidden means "starts with a
// dot"), so setDOSAttrs does nothing and localDOSAttrs synthesizes the
// directory and read-only bits from the mode.

package archive

import (
	"io/fs"
	"strings"
)

// SanitizeName makes one path element valid as a file name on this system.
func SanitizeName(name string) string {
	if strings.IndexByte(name, 0) >= 0 {
		name = strings.ReplaceAll(name, "\x00", "_")
	}
	return name
}

func samePath(a, b string) bool { return a == b }

func setDOSAttrs(string, uint8) {}

func localDOSAttrs(fi fs.FileInfo) uint8 {
	var a uint8
	if fi.IsDir() {
		a |= AttrDir
	} else {
		a |= AttrArchive
	}
	if fi.Mode().Perm()&0o200 == 0 {
		a |= AttrReadOnly
	}
	if strings.HasPrefix(fi.Name(), ".") {
		a |= AttrHidden
	}
	return a
}
