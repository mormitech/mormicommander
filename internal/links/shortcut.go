// Shortcut files are links stored as ordinary files: the target is written
// inside them instead of being known to the file system. This file decides
// which shortcut format a file is (by extension, plus a content sniff for
// macOS Finder aliases, which have none) and writes the formats we can write:
// .url, .webloc and .desktop, each holding either a web address or a file://
// URL. These are what TC's Ctrl+Shift+F5 makes when a real symbolic link is
// not wanted or not possible, for example on a FAT USB stick or when the user
// wants a shortcut that a Windows, macOS or Linux desktop opens with a
// double-click. Writing goes through a temporary file and a rename so that an
// interrupted write never leaves a half-written shortcut behind. Windows .lnk
// files are deliberately read-only: writing a correct one needs the shell's
// IShellLink to fill in volume IDs and item lists, and a hand-made one that
// Explorer misreads would be worse than none.

package links

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ShortcutExt returns the file extension for a writable or readable shortcut
// kind (".url", ".webloc", ".desktop", ".lnk"), or "" for other kinds.
func ShortcutExt(k Kind) string {
	switch k {
	case KindURL:
		return ".url"
	case KindWebloc:
		return ".webloc"
	case KindDesktop:
		return ".desktop"
	case KindLnk:
		return ".lnk"
	}
	return ""
}

// ShortcutKindByName returns the shortcut kind implied by a file name's
// extension, or KindNone. It never touches the disk, so the panel can use it
// to pick icons for a whole listing cheaply.
func ShortcutKindByName(name string) Kind {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".lnk":
		return KindLnk
	case ".url":
		return KindURL
	case ".webloc":
		return KindWebloc
	case ".desktop":
		return KindDesktop
	}
	return KindNone
}

// sniffAlias reports whether the file at p starts with bookmark data.
func sniffAlias(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [16]byte
	n, _ := f.Read(b[:])
	return IsAliasData(b[:n])
}

// CreateShortcut writes a shortcut file of the given kind (KindURL,
// KindWebloc or KindDesktop) at linkPath. target is a URL (kept as is) or a
// local path, which is made absolute and stored as a file:// URL.
func CreateShortcut(kind Kind, target, linkPath string, overwrite bool) error {
	fail := func(err error) error { return &os.LinkError{Op: "shortcut", Old: target, New: linkPath, Err: err} }
	if target == "" || linkPath == "" {
		return fail(fs.ErrInvalid)
	}
	u := target
	if !IsURL(target) {
		abs, err := filepath.Abs(target)
		if err != nil {
			return fail(err)
		}
		u = FileURL(abs)
	}
	var data []byte
	switch kind {
	case KindURL:
		data = URLShortcutData(u)
	case KindWebloc:
		data = WeblocData(u)
	case KindDesktop:
		name := filepath.Base(target)
		if IsURL(target) {
			name = strings.TrimSuffix(filepath.Base(linkPath), ".desktop")
		}
		data = DesktopLinkData(name, u)
	default:
		return fail(ErrUnsupported)
	}
	if fi, err := os.Lstat(linkPath); err == nil {
		if !overwrite {
			return fail(fs.ErrExist)
		}
		if fi.IsDir() {
			return fail(fs.ErrExist)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fail(err)
	}
	tmp := tempName(linkPath)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fail(err)
	}
	if err := os.Rename(tmp, linkPath); err != nil {
		os.Remove(tmp)
		return fail(err)
	}
	return nil
}
