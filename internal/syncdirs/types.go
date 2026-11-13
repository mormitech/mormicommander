// Package syncdirs is the engine behind the Synchronize Directories tool
// (Total Commander's cm_SyncDirs, FEATURES.md section 25). It compares two
// folder trees, sorts every file pair into one of six categories (equal, left
// newer, right newer, left only, right only, unequal), proposes a default
// action for each pair the way Total Commander does, lets the user override
// those actions, and finally carries out the plan by copying in both
// directions and deleting. The package has no UI: the Wails front end shows
// the result list, the filter buttons and the Synchronize dialog, and calls
// in here for everything that touches the disk.
//
// This file holds the plain data types the UI sees: categories, actions,
// entries and items. They all marshal to readable JSON strings so they can be
// sent to the front end as they are.
package syncdirs

import (
	"fmt"
	"io/fs"
	"time"
)

// Category is the result of comparing one pair of names.
type Category int

const (
	// Equal means both sides exist and count as the same (same size and
	// date within tolerance, or the same content when comparing by content).
	Equal Category = iota
	// LeftNewer means both sides exist and the left file is newer.
	LeftNewer
	// RightNewer means both sides exist and the right file is newer.
	RightNewer
	// LeftOnly means the name exists only on the left side.
	LeftOnly
	// RightOnly means the name exists only on the right side.
	RightOnly
	// Unequal means both sides exist and differ, but neither can be called
	// newer (same date but different size or content, or a file on one side
	// and a folder or link on the other).
	Unequal
)

var categoryNames = [...]string{"equal", "leftNewer", "rightNewer", "leftOnly", "rightOnly", "unequal"}
var categorySymbols = [...]string{"=", "->", "<-", "->", "<-", "!="}

func (c Category) String() string {
	if c >= 0 && int(c) < len(categoryNames) {
		return categoryNames[c]
	}
	return fmt.Sprintf("Category(%d)", int(c))
}

// Symbol returns the short sign Total Commander draws between the two
// columns for this category.
func (c Category) Symbol() string {
	if c >= 0 && int(c) < len(categorySymbols) {
		return categorySymbols[c]
	}
	return "?"
}

// MarshalText writes the category as its name, for JSON.
func (c Category) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// UnmarshalText reads a category name written by MarshalText.
func (c *Category) UnmarshalText(b []byte) error {
	for i, n := range categoryNames {
		if n == string(b) {
			*c = Category(i)
			return nil
		}
	}
	return fmt.Errorf("syncdirs: unknown category %q", b)
}

// Action is what Synchronize will do with one item.
type Action int

const (
	// NoAction leaves the pair alone (Total Commander shows no arrow).
	NoAction Action = iota
	// CopyLeftToRight copies the left entry over (or next to) the right one.
	CopyLeftToRight
	// CopyRightToLeft copies the right entry over (or next to) the left one.
	CopyRightToLeft
	// DeleteLeft deletes the left entry.
	DeleteLeft
	// DeleteRight deletes the right entry.
	DeleteRight
	// DeleteBoth deletes the entry on both sides.
	DeleteBoth
)

var actionNames = [...]string{"none", "copyLeftToRight", "copyRightToLeft", "deleteLeft", "deleteRight", "deleteBoth"}

func (a Action) String() string {
	if a >= 0 && int(a) < len(actionNames) {
		return actionNames[a]
	}
	return fmt.Sprintf("Action(%d)", int(a))
}

// MarshalText writes the action as its name, for JSON.
func (a Action) MarshalText() ([]byte, error) { return []byte(a.String()), nil }

// UnmarshalText reads an action name, so the UI can send overrides as text.
func (a *Action) UnmarshalText(b []byte) error {
	for i, n := range actionNames {
		if n == string(b) {
			*a = Action(i)
			return nil
		}
	}
	return fmt.Errorf("syncdirs: unknown action %q", b)
}

// Kind says what an entry is on disk.
type Kind int

const (
	// KindFile is a regular file (or anything that is not a folder or link).
	KindFile Kind = iota
	// KindDir is a folder.
	KindDir
	// KindLink is a symbolic link that was not followed.
	KindLink
)

var kindNames = [...]string{"file", "dir", "link"}

func (k Kind) String() string {
	if k >= 0 && int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// MarshalText writes the kind as its name, for JSON.
func (k Kind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// Entry describes one side of a pair as it looked during the compare.
type Entry struct {
	Name    string      `json:"name"`    // base name on this side
	Kind    Kind        `json:"kind"`    // file, dir or (unfollowed) link
	Size    int64       `json:"size"`    // bytes; 0 for folders and links
	ModTime time.Time   `json:"modTime"` // modification time
	Mode    fs.FileMode `json:"mode"`    // permission and type bits
	// IsLink is true when the name is a symbolic link. With FollowSymlinks
	// on, Kind then describes the link target and Followed is true.
	IsLink     bool   `json:"isLink,omitempty"`
	Followed   bool   `json:"followed,omitempty"`
	LinkTarget string `json:"linkTarget,omitempty"`
	// ViaLink is true when the path to this entry passes through a folder
	// symlink that was followed. The engine never writes or deletes there,
	// because the change would land outside the tree the user picked.
	ViaLink bool `json:"viaLink,omitempty"`
}

// Item is one row of the compare result: a name and what is on each side.
type Item struct {
	// Path is the slash-separated path relative to the compared folders,
	// spelled as on the left side when it exists there.
	Path string `json:"path"`
	// LeftPath and RightPath are the paths on each side. When an entry is
	// missing they hold the path a copy would create, which can differ in
	// letter case from Path when folders were matched case-insensitively.
	LeftPath  string `json:"leftPath"`
	RightPath string `json:"rightPath"`
	Left      *Entry `json:"left,omitempty"`
	Right     *Entry `json:"right,omitempty"`
	// IsDir is true for folder rows (a folder on every side that exists).
	IsDir bool `json:"isDir"`
	// Depth is the nesting level, 0 for names directly in the root folders.
	Depth    int      `json:"depth"`
	Category Category `json:"category"`
	// TypeMismatch is set for Unequal rows where one side is a file and the
	// other a folder or link; such rows can only be deleted, not copied.
	TypeMismatch bool `json:"typeMismatch,omitempty"`
	// ContentCompared is true when the bytes of both files were compared.
	ContentCompared bool `json:"contentCompared,omitempty"`
	// Default is the action the engine proposed; Action is the current one
	// (the same until the user overrides it).
	Default Action `json:"default"`
	Action  Action `json:"action"`
	// Note explains problems such as an unreadable file or a symlink loop.
	Note string `json:"note,omitempty"`

	// lvia and rvia are true when the folder holding this row was reached
	// through a followed folder link on that side; nothing is written or
	// deleted there.
	lvia, rvia bool
}

// Duplicate reports whether the name exists on both sides.
func (it *Item) Duplicate() bool { return it.Left != nil && it.Right != nil }

// Single reports whether the name exists on only one side.
func (it *Item) Single() bool { return !it.Duplicate() }

// Side names one of the two compared trees.
type Side int

const (
	// LeftSide is the tree shown in the left column.
	LeftSide Side = iota
	// RightSide is the tree shown in the right column.
	RightSide
)

func (s Side) String() string {
	if s == LeftSide {
		return "left"
	}
	return "right"
}

// MarshalText writes the side as "left" or "right", for JSON.
func (s Side) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// CaseMode chooses how names on the two sides are matched.
type CaseMode int

const (
	// CaseAuto matches case-insensitively on Windows and macOS and
	// case-sensitively elsewhere, like the usual file systems there.
	CaseAuto CaseMode = iota
	// CaseSensitive treats "A.txt" and "a.txt" as different names.
	CaseSensitive
	// CaseInsensitive treats "A.txt" and "a.txt" as the same name.
	CaseInsensitive
)

var caseNames = [...]string{"auto", "sensitive", "insensitive"}

// MarshalText writes the mode as its name, for JSON.
func (m CaseMode) MarshalText() ([]byte, error) {
	if m >= 0 && int(m) < len(caseNames) {
		return []byte(caseNames[m]), nil
	}
	return nil, fmt.Errorf("syncdirs: bad case mode %d", int(m))
}

// UnmarshalText reads a case mode name.
func (m *CaseMode) UnmarshalText(b []byte) error {
	for i, n := range caseNames {
		if n == string(b) {
			*m = CaseMode(i)
			return nil
		}
	}
	return fmt.Errorf("syncdirs: unknown case mode %q", b)
}

func (m CaseMode) insensitive() bool {
	switch m {
	case CaseSensitive:
		return false
	case CaseInsensitive:
		return true
	}
	return defaultCaseInsensitive
}
