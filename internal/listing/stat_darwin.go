// This file fills the macOS-specific parts of an Item. macOS records a real birth time, so the "creationdate" column
// works like it does on Windows, and it has BSD file flags: UF_HIDDEN hides a file in Finder without a leading dot
// (TC users expect such files to count as hidden), and UF_COMPRESSED marks transparently compressed files, which we
// show as the "c" attribute like NTFS compression.

//go:build darwin

package listing

import (
	"io/fs"
	"syscall"
	"time"

	"mormicommander/internal/cloudfile"
)

const (
	ufCompressed = 0x00000020
	ufHidden     = 0x00008000
)

func fillPlatform(it *Item, fi fs.FileInfo) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	it.AccessTime = time.Unix(st.Atimespec.Unix())
	it.CreateTime = time.Unix(st.Birthtimespec.Unix())
	if st.Flags&ufHidden != 0 {
		it.Attr |= AttrHidden
	}
	if st.Flags&ufCompressed != 0 {
		it.Attr |= AttrCompressed
	}
	if cloudfile.Placeholder(fi) {
		it.Attr |= AttrCloud
	}
	it.Owner = userName(st.Uid)
	it.Group = groupName(st.Gid)
}
