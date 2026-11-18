// This file fills the Windows-specific parts of an Item. Windows keeps the full attribute word (read-only, hidden,
// system, archive, compressed, encrypted and so on) next to three timestamps in the directory entry, which is exactly
// what TC's attribute column and three date fields show, so we copy them over. A dotfile is not hidden on Windows
// unless it carries the hidden attribute, and "programs" are recognised by extension, not by a permission bit.

//go:build windows

package listing

import (
	"io/fs"
	"syscall"
	"time"

	"mormicommander/internal/cloudfile"
)

func fillPlatform(it *Item, fi fs.FileInfo) {
	// FromFileInfo marks dotfiles hidden for Unix; on Windows only the attribute counts.
	it.Attr &^= AttrHidden | AttrExec
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return
	}
	it.CreateTime = time.Unix(0, d.CreationTime.Nanoseconds())
	it.AccessTime = time.Unix(0, d.LastAccessTime.Nanoseconds())
	a := d.FileAttributes
	set := func(bit uint32, attr Attr) {
		if a&bit != 0 {
			it.Attr |= attr
		}
	}
	set(syscall.FILE_ATTRIBUTE_READONLY, AttrReadOnly)
	set(syscall.FILE_ATTRIBUTE_HIDDEN, AttrHidden)
	set(syscall.FILE_ATTRIBUTE_SYSTEM, AttrSystem)
	set(syscall.FILE_ATTRIBUTE_ARCHIVE, AttrArchive)
	set(0x800, AttrCompressed) // FILE_ATTRIBUTE_COMPRESSED
	set(0x4000, AttrEncrypted) // FILE_ATTRIBUTE_ENCRYPTED
	set(0x100, AttrTemporary)  // FILE_ATTRIBUTE_TEMPORARY
	set(0x1000, AttrOffline)   // FILE_ATTRIBUTE_OFFLINE
	set(0x200, AttrSparse)     // FILE_ATTRIBUTE_SPARSE_FILE
	set(syscall.FILE_ATTRIBUTE_REPARSE_POINT, AttrLink)
	if cloudfile.Placeholder(fi) {
		it.Attr |= AttrCloud
	}
	if a&syscall.FILE_ATTRIBUTE_READONLY == 0 {
		it.Attr &^= AttrReadOnly
	}
	if !it.IsDir() && IsWindowsExecName(it.Name) {
		it.Attr |= AttrExec
	}
}
