// This file holds the overwrite question that Total Commander asks whenever a
// file being unpacked (or packed) already exists at the destination: Overwrite,
// Overwrite all, Skip, Skip all, Overwrite all older, Rename, Cancel. The UI
// supplies a callback that shows the dialog; this code remembers the "all"
// answers for the rest of the job so the user is asked only once, and also
// supports fixed policies for scripted use (overwrite, skip, older-only,
// automatic renaming to "name (2).ext").
package archive

import (
	"path"
	"strconv"
	"strings"
	"time"
)

// ConflictPolicy is the standing answer used before (or instead of) asking.
type ConflictPolicy int

const (
	// PolicyAsk calls the ConflictFunc for every conflict. Without a
	// ConflictFunc conflicting files are skipped, never silently replaced.
	PolicyAsk ConflictPolicy = iota
	// PolicyOverwrite always replaces the existing file.
	PolicyOverwrite
	// PolicySkip always keeps the existing file.
	PolicySkip
	// PolicyOverwriteOlder replaces the existing file only when it is older
	// than the incoming one.
	PolicyOverwriteOlder
	// PolicyAutoRename keeps both by giving the incoming file a free name.
	PolicyAutoRename
)

// FileStat is the short description shown on each side of the dialog.
type FileStat struct {
	Size     int64
	ModTime  time.Time
	IsDir    bool
	ReadOnly bool
}

// ConflictInfo describes one conflict for the dialog.
type ConflictInfo struct {
	// Path is the destination path (a local path when extracting, an
	// archive path when packing).
	Path     string
	Existing FileStat
	Incoming FileStat
}

// ConflictAction is the button the user pressed.
type ConflictAction int

const (
	ActOverwrite ConflictAction = iota
	ActSkip
	ActOverwriteAll
	ActSkipAll
	ActOverwriteOlderAll
	ActAutoRenameAll
	// ActRename uses ConflictDecision.NewName (a base name) for this file.
	ActRename
	// ActCancel stops the whole job with ErrCanceled.
	ActCancel
)

// ConflictDecision is the answer returned by a ConflictFunc.
type ConflictDecision struct {
	Action  ConflictAction
	NewName string
}

// ConflictFunc shows the overwrite dialog and returns the user's choice.
type ConflictFunc func(ConflictInfo) ConflictDecision

// Resolution is what the job should actually do with one file.
type Resolution int

const (
	ResolveOverwrite Resolution = iota
	ResolveSkip
	ResolveRename
	ResolveCancel
)

// ConflictResolver remembers "all" answers across one job. Formats use it
// when packing into an existing archive; Extract uses it internally.
type ConflictResolver struct {
	policy ConflictPolicy
	ask    ConflictFunc
}

// NewConflictResolver starts a job with a standing policy and an optional
// dialog callback.
func NewConflictResolver(p ConflictPolicy, ask ConflictFunc) *ConflictResolver {
	return &ConflictResolver{policy: p, ask: ask}
}

// Resolve decides one conflict. exists reports whether a candidate name is
// taken, which automatic renaming needs; the returned name is the base name to
// use when the resolution is ResolveRename.
func (c *ConflictResolver) Resolve(info ConflictInfo, exists func(name string) bool) (Resolution, string) {
	switch c.policy {
	case PolicyOverwrite:
		return ResolveOverwrite, ""
	case PolicySkip:
		return ResolveSkip, ""
	case PolicyOverwriteOlder:
		if info.Existing.ModTime.Before(info.Incoming.ModTime) {
			return ResolveOverwrite, ""
		}
		return ResolveSkip, ""
	case PolicyAutoRename:
		return ResolveRename, FreeName(path.Base(strings.ReplaceAll(info.Path, "\\", "/")), exists)
	}
	if c.ask == nil {
		return ResolveSkip, ""
	}
	d := c.ask(info)
	switch d.Action {
	case ActOverwrite:
		return ResolveOverwrite, ""
	case ActSkip:
		return ResolveSkip, ""
	case ActOverwriteAll:
		c.policy = PolicyOverwrite
	case ActSkipAll:
		c.policy = PolicySkip
	case ActOverwriteOlderAll:
		c.policy = PolicyOverwriteOlder
	case ActAutoRenameAll:
		c.policy = PolicyAutoRename
	case ActRename:
		name := strings.TrimSpace(d.NewName)
		if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
			return ResolveSkip, ""
		}
		return ResolveRename, name
	case ActCancel:
		return ResolveCancel, ""
	default:
		return ResolveSkip, ""
	}
	return c.Resolve(info, exists)
}

// FreeName returns name itself when it is free, otherwise the first free
// "base (N).ext" variant, the same pattern Total Commander's auto-rename
// uses. exists must report whether a candidate base name is already taken.
func FreeName(name string, exists func(string) bool) string {
	if exists == nil || !exists(name) {
		return name
	}
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" { // dot files such as ".profile"
		base, ext = name, ""
	}
	for i := 2; ; i++ {
		cand := base + " (" + strconv.Itoa(i) + ")" + ext
		if !exists(cand) {
			return cand
		}
	}
}
