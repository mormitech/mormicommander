// Package arcapi holds the small, format-neutral contract that every archive
// engine in mormiCommander (tar, zip, 7z, rar, ...) implements, so the panels
// can show the inside of an archive like a folder and the job engine can copy
// files out of it without knowing which format it is dealing with. The
// contract is deliberately minimal: a flat list of entries, an extract call
// and a close call. Format packages add their own extras (creating, adding,
// deleting, testing) on top, and the UI reaches those through the concrete
// type. Keeping this file tiny makes it easy for the parallel engine
// branches to agree on it without merge pain. (It was merged in as
// internal/archive/iface.go and moved here because the zip branch defined a
// richer, conflicting contract in internal/archive itself; tarfmt imports it
// under the name "archive".)
package arcapi

import (
	"context"
	"errors"
	"io/fs"
	"time"
)

// NoFileTimes leaves unpacked files and folders with the time they were written instead of the times stored in the
// archive (the program's Options > Zip > Keep original file times when unpacking switched off). Every format's
// unpacker reads it.
var NoFileTimes bool

// Entry describes one item stored in an archive. Path always uses forward
// slashes, has no leading "./" or "/" and no trailing slash, so it can be
// compared and split the same way on every operating system. Size is the
// unpacked size and Packed the stored size; either may be -1 when the format
// cannot tell without unpacking everything.
type Entry struct {
	Path    string
	Size    int64
	Packed  int64
	ModTime time.Time
	Mode    fs.FileMode
	IsDir   bool
}

// Reader is an opened archive. Entries lists everything inside it; Extract
// unpacks the named entries (all of them when names is empty; naming a
// folder selects everything below it) into the folder dest.
type Reader interface {
	Entries(ctx context.Context) ([]Entry, error)
	Extract(ctx context.Context, names []string, dest string, opt ExtractOptions) error
	Close() error
}

// ConflictPolicy says what to do when a file being unpacked already exists
// in the target folder, mirroring the choices of Total Commander's
// "overwrite?" dialog.
type ConflictPolicy int

const (
	// ConflictAsk calls ExtractOptions.OnConflict for every clash. With no
	// callback set it behaves like ConflictSkip, which is the safe choice.
	ConflictAsk ConflictPolicy = iota
	// ConflictOverwrite replaces the existing file.
	ConflictOverwrite
	// ConflictSkip leaves the existing file alone.
	ConflictSkip
	// ConflictOverwriteOlder replaces the existing file only when the
	// archived copy is newer.
	ConflictOverwriteOlder
	// ConflictAbort stops the whole extraction with ErrAborted.
	ConflictAbort
)

// Conflict is what OnConflict is told about a clash: the archive entry, the
// local path it would be written to, and what is already there.
type Conflict struct {
	Entry    Entry
	Path     string
	Existing fs.FileInfo
}

// Progress is reported while extracting or packing. Bytes counts unpacked
// data written so far and Total the unpacked total when it is known (0 when
// it is not). ArchiveRead and ArchiveSize count the compressed archive
// itself, which is always known and gives a reliable percentage even before
// the archive has been listed.
type Progress struct {
	Name        string
	Files       int
	Bytes       int64
	Total       int64
	ArchiveRead int64
	ArchiveSize int64
}

// ExtractOptions are the knobs of Total Commander's unpack dialog.
type ExtractOptions struct {
	// Flatten drops the folder part of every name ("unpack path names"
	// unchecked); folders themselves are then not created.
	Flatten bool
	// StripPrefix removes a leading archive folder from every name, which is
	// what copying out of a panel that shows "archive/sub/dir" needs. Entries
	// outside the prefix are ignored.
	StripPrefix string
	// Filter, when set, limits extraction to entries for which it returns
	// true (the unpack dialog's file mask).
	Filter func(e Entry) bool
	// Conflict picks the overwrite behaviour; OnConflict answers ConflictAsk.
	Conflict   ConflictPolicy
	OnConflict func(c Conflict) ConflictPolicy
	// Progress, when set, is called regularly from the extracting goroutine.
	Progress func(p Progress)
}

// Errors shared by all formats, so the UI can react to them uniformly.
var (
	ErrAborted      = errors.New("archive: aborted by user")
	ErrUnsafePath   = errors.New("archive: entry path escapes the target folder")
	ErrUnsafeLink   = errors.New("archive: link points outside the target folder")
	ErrNotFound     = errors.New("archive: entry not found")
	ErrNotSupported = errors.New("archive: operation not supported for this format")
)

// EntryError wraps a problem with one entry. Extraction carries on with the
// remaining entries and returns all EntryErrors joined together at the end.
type EntryError struct {
	Name string
	Err  error
}

func (e *EntryError) Error() string { return e.Name + ": " + e.Err.Error() }
func (e *EntryError) Unwrap() error { return e.Err }
