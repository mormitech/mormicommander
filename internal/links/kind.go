// This file defines the vocabulary shared by the whole package: the kinds of
// link a panel can meet, the errors the UI needs to tell apart (a broken link
// is not a loop, and "your Windows account may not create symlinks" needs its
// own hint), and the Info record that the panel, the Properties dialog and
// the "follow link" command all read. Keeping these in one place means every
// function speaks the same language and the UI can switch on a Kind instead
// of guessing from file extensions itself.

package links

import (
	"errors"
	"fmt"
)

// Kind tells what sort of link (if any) a path is.
type Kind int

const (
	// KindNone is an ordinary file or folder that is not a link.
	KindNone Kind = iota
	// KindSymlink is a symbolic link (all platforms).
	KindSymlink
	// KindJunction is an NTFS junction (directory mount point, Windows only).
	KindJunction
	// KindHardLink is a regular file whose data has more than one name.
	// Every name of a hard-linked file is equal; this kind only says that
	// other names exist, so the UI can warn before editing or deleting.
	KindHardLink
	// KindLnk is a Windows shell shortcut (.lnk). It is read, never written.
	KindLnk
	// KindURL is a Windows Internet shortcut (.url).
	KindURL
	// KindWebloc is a macOS web location file (.webloc).
	KindWebloc
	// KindDesktop is a freedesktop.org desktop entry (.desktop).
	KindDesktop
	// KindAlias is a macOS Finder alias (bookmark data in the data fork).
	KindAlias
)

var kindNames = [...]string{
	KindNone:     "none",
	KindSymlink:  "symlink",
	KindJunction: "junction",
	KindHardLink: "hardlink",
	KindLnk:      "lnk",
	KindURL:      "url",
	KindWebloc:   "webloc",
	KindDesktop:  "desktop",
	KindAlias:    "alias",
}

// String returns a short stable name for the kind (used in logs and tests).
func (k Kind) String() string {
	if k >= 0 && int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// IsFileSystemLink reports whether the kind is resolved by the file system
// itself (symlink, junction) rather than by reading a shortcut file.
func (k Kind) IsFileSystemLink() bool { return k == KindSymlink || k == KindJunction }

// IsShortcut reports whether the kind is a shortcut file whose target is
// stored inside the file (.lnk, .url, .webloc, .desktop, Finder alias).
func (k Kind) IsShortcut() bool {
	switch k {
	case KindLnk, KindURL, KindWebloc, KindDesktop, KindAlias:
		return true
	}
	return false
}

var (
	// ErrLoop means following a link leads back to a link already visited
	// (or more than MaxHops links in a row), so it can never be resolved.
	ErrLoop = errors.New("link loop")
	// ErrBroken means the link exists but its target does not.
	ErrBroken = errors.New("link target does not exist")
	// ErrNotLink means the path is neither a link nor a known shortcut file.
	ErrNotLink = errors.New("not a link")
	// ErrIsDir is returned when a hard link to a folder is requested; no
	// supported file system allows that.
	ErrIsDir = errors.New("hard links to folders are not possible")
	// ErrCrossDevice is returned when a hard link would span two volumes.
	ErrCrossDevice = errors.New("hard links must be on the same volume as their target")
	// ErrPrivilege is returned on Windows when the account may not create
	// symbolic links (enable Developer Mode or run as administrator).
	ErrPrivilege = errors.New("not allowed to create symbolic links (on Windows enable Developer Mode or run as administrator)")
	// ErrUnsupported is returned for operations the platform cannot do, such
	// as junctions outside Windows or writing .lnk files.
	ErrUnsupported = errors.ErrUnsupported
	// ErrBadShortcut means a shortcut file is truncated or malformed.
	ErrBadShortcut = errors.New("malformed shortcut file")
)

// MaxHops is how many links in a row Resolve follows before it gives up and
// reports ErrLoop. Linux uses 40 for the same purpose.
const MaxHops = 40

// Info describes one path as the panel and the Properties dialog show it.
type Info struct {
	// Path is the inspected path as given (made absolute).
	Path string
	// Kind is what sort of link the path is.
	Kind Kind
	// Target is the raw target as stored in the link or shortcut: a symlink's
	// text (possibly relative), a shortcut's path, or a URL.
	Target string
	// URL is set when the shortcut points to a web address (or any non-file
	// URL); Target holds the same value then.
	URL string
	// Resolved is the absolute, fully resolved local target when it could be
	// determined (empty for URLs and foreign paths such as a Windows path in
	// a .lnk read on macOS).
	Resolved string
	// Chain lists every hop from Path to Resolved for file-system links.
	Chain []string
	// TargetExists reports whether Resolved exists.
	TargetExists bool
	// TargetIsDir reports whether Resolved is a folder.
	TargetIsDir bool
	// Broken is true when the link exists but its target does not.
	Broken bool
	// Loop is true when the link can never be resolved because of a cycle.
	Loop bool
	// HardLinks is the number of names the file's data has (1 for a normal
	// file, 0 when unknown or not applicable).
	HardLinks uint64

	// Shortcut details (only filled for shortcut kinds that store them).

	// Name is the display name (.desktop Name=, .lnk description).
	Name string
	// Arguments is the command-line arguments stored in a .lnk.
	Arguments string
	// WorkingDir is the start-in folder of a .lnk / .url / .desktop.
	WorkingDir string
	// Icon is the icon location stored in the shortcut.
	Icon string
	// Exec is the command of a .desktop Application entry.
	Exec string
}
