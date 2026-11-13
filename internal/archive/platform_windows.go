//go:build windows

// Windows-specific helpers for extraction and packing. Windows rejects the
// characters <>:"|?* and control characters in file names, silently drops
// trailing dots and spaces, and reserves device names such as CON, NUL, COM1
// and LPT1 in every folder. Archives made on other systems can contain such
// names, so SanitizeName rewrites them the way Total Commander does (illegal
// characters become "_", device names get a "_" prefix) instead of failing.
// The attribute helpers read and write the hidden/system/archive bits that
// zip files carry, so they survive a pack and unpack round trip.

package archive

import (
	"io/fs"
	"strings"
	"syscall"
)

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SanitizeName makes one path element valid as a Windows file name.
func SanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 32 || strings.ContainsRune(`<>:"|?*`, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	s := strings.TrimRight(b.String(), ". ")
	if s == "" {
		s = "_"
	}
	stem := strings.ToUpper(s)
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if reservedNames[stem] {
		s = "_" + s
	}
	return s
}

func samePath(a, b string) bool { return strings.EqualFold(a, b) }

// setDOSAttrs applies hidden and system bits after a file was written. The
// read-only bit is applied through the file mode by the caller.
func setDOSAttrs(path string, attrs uint8) {
	extra := uint32(attrs) & uint32(AttrHidden|AttrSystem)
	if extra == 0 {
		return
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	cur, err := syscall.GetFileAttributes(p)
	if err != nil {
		return
	}
	_ = syscall.SetFileAttributes(p, cur|extra)
}

// localDOSAttrs returns the DOS attribute bits of a local file.
func localDOSAttrs(fi fs.FileInfo) uint8 {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok && d != nil {
		return uint8(d.FileAttributes & 0x37)
	}
	var a uint8
	if fi.IsDir() {
		a |= AttrDir
	}
	if fi.Mode().Perm()&0o200 == 0 {
		a |= AttrReadOnly
	}
	return a
}
