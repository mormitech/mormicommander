package vfs

import (
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"

	"mormicommander/internal/cloudfile"
)

var execExts = map[string]bool{".exe": true, ".bat": true, ".cmd": true, ".com": true, ".ps1": true, ".msi": true}

// sysFlags reads the Windows attributes: hidden, system, read-only, archive, and whether the extension runs.
func sysFlags(info fs.FileInfo, name string) int {
	f := 0
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		a := d.FileAttributes
		f |= FlagWinAttrs
		if a&syscall.FILE_ATTRIBUTE_ARCHIVE != 0 {
			f |= FlagArchive
		}
		if a&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
			f |= FlagHidden
		}
		if a&syscall.FILE_ATTRIBUTE_SYSTEM != 0 {
			f |= FlagSystem
		}
		if a&syscall.FILE_ATTRIBUTE_READONLY != 0 {
			f |= FlagReadOnly
		}
	}
	if cloudfile.Placeholder(info) {
		f |= FlagCloud
	}
	if !info.IsDir() && execExts[strings.ToLower(filepath.Ext(name))] {
		f |= FlagExec
	}
	return f
}
