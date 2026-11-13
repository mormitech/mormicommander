// This file reads the Windows file attributes for the attribute filter. Windows keeps
// read-only, hidden, system, archive, compressed and encrypted as attribute bits that
// os.ReadDir and os.Lstat hand us in Win32FileAttributeData, so the filter can match
// exactly what Total Commander shows in its attribute column. Executable is not an
// attribute on Windows; like Explorer we decide it by extension.

package find

import (
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	fileAttributeCompressed = 0x800
	fileAttributeEncrypted  = 0x4000
)

var execExts = map[string]bool{".exe": true, ".bat": true, ".cmd": true, ".com": true, ".ps1": true, ".msi": true}

// sysAttrs translates the Windows attribute bits.
func sysAttrs(name string, info fs.FileInfo) Attr {
	var a Attr
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		fa := d.FileAttributes
		if fa&syscall.FILE_ATTRIBUTE_READONLY != 0 {
			a |= AttrReadOnly
		}
		if fa&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
			a |= AttrHidden
		}
		if fa&syscall.FILE_ATTRIBUTE_SYSTEM != 0 {
			a |= AttrSystem
		}
		if fa&syscall.FILE_ATTRIBUTE_ARCHIVE != 0 {
			a |= AttrArchive
		}
		if fa&fileAttributeCompressed != 0 {
			a |= AttrCompressed
		}
		if fa&fileAttributeEncrypted != 0 {
			a |= AttrEncrypted
		}
	} else if info.Mode().Perm()&0o222 == 0 {
		a |= AttrReadOnly
	}
	if !info.IsDir() && execExts[strings.ToLower(filepath.Ext(name))] {
		a |= AttrExecutable
	}
	return a
}
