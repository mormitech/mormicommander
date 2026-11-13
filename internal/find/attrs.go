// This file holds the file attribute filter of the Advanced tab (F-533). Total
// Commander lets the user tick each attribute as "must be set", "must be cleared" or
// "don't care"; we model that as two bit sets, AttrSet and AttrClear, over a common
// Attr type that means the same thing on every platform. Windows reports read-only,
// hidden, system, archive, compressed and encrypted directly; macOS has a hidden flag
// and a compressed flag; Linux only knows dotfiles and permission bits. The platform
// files (attrs_windows.go, attrs_darwin.go, attrs_other.go) fill in what their system
// can tell, and this file adds the parts every platform shares. Unix permissions get
// their own mask/value filter because "group-writable" cannot be expressed as one of
// the Windows-style attributes.

package find

import (
	"io/fs"
	"runtime"
	"strings"
)

// Attr is a set of file attributes, normalised across platforms.
type Attr uint32

const (
	AttrReadOnly   Attr = 1 << iota // no write permission (Unix) or the read-only attribute (Windows)
	AttrHidden                      // dotfile, macOS hidden flag, or the Windows hidden attribute
	AttrSystem                      // Windows system attribute
	AttrArchive                     // Windows archive attribute ("changed since last backup")
	AttrDirectory                   // a folder
	AttrSymlink                     // a symbolic link (or a Windows reparse point that is one)
	AttrExecutable                  // executable bit (Unix) or an executable extension (Windows)
	AttrCompressed                  // NTFS compression or the macOS compressed flag
	AttrEncrypted                   // NTFS encryption (EFS)
)

// attrLetters maps each attribute to the letter used in AttrString and ParseAttr.
// The first four are the classic Total Commander letters.
var attrLetters = []struct {
	a Attr
	c byte
}{
	{AttrReadOnly, 'r'}, {AttrArchive, 'a'}, {AttrHidden, 'h'}, {AttrSystem, 's'},
	{AttrDirectory, 'd'}, {AttrSymlink, 'l'}, {AttrExecutable, 'x'},
	{AttrCompressed, 'c'}, {AttrEncrypted, 'e'},
}

// String lists the set attributes as letters, for logs and saved searches.
func (a Attr) String() string {
	var b strings.Builder
	for _, l := range attrLetters {
		if a&l.a != 0 {
			b.WriteByte(l.c)
		}
	}
	return b.String()
}

// ParseAttr turns letters like "rh" back into an Attr. Unknown letters are ignored so
// a saved search from a newer version still loads.
func ParseAttr(s string) Attr {
	var a Attr
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20
		for _, l := range attrLetters {
			if l.c == c {
				a |= l.a
			}
		}
	}
	return a
}

// attrsOf computes the attributes of one entry. For OSFS the platform hook adds what
// the operating system knows; for other file systems only the portable parts (mode
// bits and the dotfile rule) are available.
func attrsOf(f FS, name string, info fs.FileInfo) Attr {
	var a Attr
	m := info.Mode()
	if m.IsDir() {
		a |= AttrDirectory
	}
	if m&fs.ModeSymlink != 0 {
		a |= AttrSymlink
	}
	// Dotfiles count as hidden everywhere except local Windows folders, where only
	// the hidden attribute does (Explorer and Total Commander show ".git" there).
	local := isOSFS(f)
	if (!local || runtime.GOOS != "windows") && strings.HasPrefix(info.Name(), ".") && info.Name() != "." && info.Name() != ".." {
		a |= AttrHidden
	}
	if local {
		a |= sysAttrs(name, info)
	} else {
		a |= modeAttrs(info)
	}
	return a
}

// modeAttrs derives read-only and executable from Unix permission bits.
func modeAttrs(info fs.FileInfo) Attr {
	var a Attr
	m := info.Mode()
	if m.Perm()&0o222 == 0 {
		a |= AttrReadOnly
	}
	if m.IsRegular() && m.Perm()&0o111 != 0 {
		a |= AttrExecutable
	}
	return a
}
