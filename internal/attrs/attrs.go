// Package attrs is the backend of the "Change attributes" dialog (cm_SetAttrib)
// and of the file Properties window (cm_Properties, Alt+Enter). Total Commander
// shows and changes the classic Windows attributes (read-only, archive, hidden,
// system) together with the file dates, and on macOS and Linux the same dialog
// has to speak Unix: rwx permissions for owner, group and others, the octal
// form, the setuid/setgid/sticky bits and the owner and group of a file. This
// package gathers everything the two dialogs need to show (Stat, Totals,
// DescribeType, Summarize) and everything they need to change (MakePlan and
// Plan.Apply), so the UI stays a thin layer and the logic can be tested here.
//
// This file holds the Info record that describes one file system entry. It is
// read with Lstat semantics, so a symbolic link is described as the link itself
// with its target filled in, which is what both dialogs want to display.
package attrs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Kind tells what sort of file system entry an Info describes. It is coarser
// than fs.FileMode on purpose: the dialogs only need a word for the type line.
type Kind int

const (
	KindFile Kind = iota
	KindDir
	KindSymlink
	KindNamedPipe
	KindSocket
	KindDevice
	KindCharDevice
	KindIrregular
)

// String returns a short plain-English name for the kind, used as a fallback
// type description.
func (k Kind) String() string {
	switch k {
	case KindFile:
		return "File"
	case KindDir:
		return "Folder"
	case KindSymlink:
		return "Symbolic link"
	case KindNamedPipe:
		return "Named pipe"
	case KindSocket:
		return "Socket"
	case KindDevice:
		return "Block device"
	case KindCharDevice:
		return "Character device"
	default:
		return "Special file"
	}
}

// kindOf maps a Go file mode onto a Kind.
func kindOf(m fs.FileMode) Kind {
	switch {
	case m&fs.ModeSymlink != 0:
		return KindSymlink
	case m.IsDir():
		return KindDir
	case m&fs.ModeNamedPipe != 0:
		return KindNamedPipe
	case m&fs.ModeSocket != 0:
		return KindSocket
	case m&fs.ModeDevice != 0 && m&fs.ModeCharDevice != 0:
		return KindCharDevice
	case m&fs.ModeDevice != 0:
		return KindDevice
	case m&fs.ModeIrregular != 0:
		return KindIrregular
	default:
		return KindFile
	}
}

// Info describes one file system entry with everything the Properties window
// and the attributes dialog display. Fields a platform cannot provide keep
// their zero value and the matching Has* flag stays false, so the UI can grey
// out a line instead of showing a wrong value.
type Info struct {
	Path string // the path as given to Stat
	Name string // the last path element
	Kind Kind
	Mode fs.FileMode // full Go mode including type bits

	Size       int64 // logical size in bytes
	SizeOnDisk int64 // allocated bytes; -1 when unknown

	Modified time.Time
	Accessed time.Time
	Changed  time.Time // inode change time (Unix only)
	Created  time.Time // birth time where the file system records it

	HasAccessed bool
	HasChanged  bool
	HasCreated  bool

	UID   int    // numeric owner; -1 where there is none (Windows)
	GID   int    // numeric group; -1 where there is none (Windows)
	Owner string // owner name, or the number as text when it has no name
	Group string // group name, or the number as text

	Links  uint64 // hard link count; 0 when unknown
	Inode  uint64 // file serial number; 0 when unknown
	Device uint64 // device the entry lives on; 0 when unknown

	// Win holds the Windows attribute bits. On Windows they are the real
	// FILE_ATTRIBUTE_* values (WinNative is true); elsewhere they are derived
	// from the permissions and the hidden state so the attribute column and
	// filters can still say "read-only" or "hidden" in a familiar way.
	Win       WinAttr
	WinNative bool

	// Flags holds the BSD file flags (chflags) on macOS; HiddenFlag tells
	// whether UF_HIDDEN, the flag Finder uses to hide files, is set.
	Flags      uint32
	HiddenFlag bool

	// Hidden is true when a file manager would hide the entry: a name that
	// starts with a dot, the macOS hidden flag or the Windows hidden bit.
	Hidden bool

	// For symbolic links: the target text as stored in the link, the fully
	// resolved absolute target and whether the target is missing.
	LinkTarget   string
	LinkResolved string
	LinkBroken   bool
}

// Perm returns only the permission part of the mode: the nine rwx bits plus
// setuid, setgid and sticky.
func (i Info) Perm() fs.FileMode { return i.Mode & PermMask }

// IsDir reports whether the entry is a folder (a link to a folder is not).
func (i Info) IsDir() bool { return i.Kind == KindDir }

// Stat describes path without following a final symbolic link.
func Stat(path string) (Info, error) { return stat(path, false) }

// StatFollow describes the entry a symbolic link points to (or path itself
// when it is not a link).
func StatFollow(path string) (Info, error) { return stat(path, true) }

func stat(path string, follow bool) (Info, error) {
	var fi fs.FileInfo
	var err error
	if follow {
		fi, err = os.Stat(path)
	} else {
		fi, err = os.Lstat(path)
	}
	if err != nil {
		return Info{}, err
	}
	return infoFrom(path, fi, follow), nil
}

// infoFrom builds an Info from an already obtained FileInfo and asks the
// platform layer for the rest (owners, extra dates, attributes, size on disk).
func infoFrom(path string, fi fs.FileInfo, follow bool) Info {
	info := Info{
		Path:       path,
		Name:       fi.Name(),
		Mode:       fi.Mode(),
		Kind:       kindOf(fi.Mode()),
		Size:       fi.Size(),
		Modified:   fi.ModTime(),
		SizeOnDisk: -1,
		UID:        -1,
		GID:        -1,
	}
	if info.Kind == KindSymlink {
		fillLink(&info)
	}
	if isDotName(info.Name) {
		info.Hidden = true
	}
	fillPlatform(&info, fi, follow)
	if info.HiddenFlag {
		info.Hidden = true
	}
	return info
}

// isDotName reports whether a name is a Unix-style hidden name.
func isDotName(name string) bool {
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}

// fillLink reads a symbolic link's target text and checks where it leads.
// A relative target is resolved against the link's own folder, the way the
// operating system does it.
func fillLink(info *Info) {
	target, err := os.Readlink(info.Path)
	if err != nil {
		return
	}
	info.LinkTarget = target
	resolved, err := filepath.EvalSymlinks(info.Path)
	if err != nil {
		info.LinkBroken = true
		abs := target
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(filepath.Dir(info.Path), target)
		}
		info.LinkResolved = filepath.Clean(abs)
		return
	}
	if a, err := filepath.Abs(resolved); err == nil {
		resolved = a
	}
	info.LinkResolved = resolved
}
