// This file lists the errors the 7z/RAR engine hands back to its callers. The
// two underlying libraries (bodgit/sevenzip and nwaples/rardecode) each have
// their own error values and wrapping styles, and the UI should not have to
// know either of them to decide whether to show a password prompt, a "volume
// missing" message or a "CRC error" line in the operation report. So every
// library error that has a user-facing meaning is translated into one of the
// sentinel values below, which callers test with errors.Is / errors.As.

package sevenrar

import (
	"errors"
	"fmt"
)

var (
	// ErrUnsupportedFormat means the file is not a 7z or RAR archive. The
	// accompanying Detection (see Detect) still tells the caller what the
	// file is, so zip/tar/ISO files can be routed to their own engines.
	ErrUnsupportedFormat = errors.New("sevenrar: not a 7z or RAR archive")

	// ErrPasswordRequired means the archive (or some of its files) is
	// encrypted and no password was given and no password callback was set.
	ErrPasswordRequired = errors.New("sevenrar: password required")

	// ErrWrongPassword means a password was tried and rejected.
	ErrWrongPassword = errors.New("sevenrar: wrong password")

	// ErrPasswordCancelled means the password callback reported that the
	// user cancelled the password dialog.
	ErrPasswordCancelled = errors.New("sevenrar: password entry cancelled")

	// ErrUnsafePath means an entry name would have been written outside the
	// target folder (a ".." component), or would have been written through
	// a symbolic link. Such entries are never extracted.
	ErrUnsafePath = errors.New("sevenrar: unsafe path in archive")

	// ErrCRC means the extracted data did not match the checksum stored in
	// the archive. For encrypted entries this usually means a wrong
	// password, because neither format can always tell the two apart.
	ErrCRC = errors.New("sevenrar: checksum error")

	// ErrNotFound means a path asked for with Stat/Open/ReadDir is not in
	// the archive.
	ErrNotFound = errors.New("sevenrar: no such entry in archive")

	// ErrNotDir means ReadDir was called on a file.
	ErrNotDir = errors.New("sevenrar: not a directory")

	// ErrIsDir means Open was called on a directory.
	ErrIsDir = errors.New("sevenrar: is a directory")

	// ErrMissingVolume means one volume of a multi-volume archive could not
	// be found. Use errors.As with *MissingVolumeError to get its name.
	ErrMissingVolume = errors.New("sevenrar: archive volume missing")

	// ErrClosed means the archive was used after Close.
	ErrClosed = errors.New("sevenrar: archive is closed")
)

// MissingVolumeError names the volume file that could not be opened. Total
// Commander asks the user to "insert the next disk"; the UI can show Path and
// offer to retry after the user copied the file next to the others.
type MissingVolumeError struct {
	Path string // full path of the expected volume file
	Err  error  // the underlying open error
}

func (e *MissingVolumeError) Error() string {
	return fmt.Sprintf("sevenrar: archive volume missing: %s", e.Path)
}

// Is makes errors.Is(err, ErrMissingVolume) true.
func (e *MissingVolumeError) Is(target error) bool { return target == ErrMissingVolume }

// Unwrap gives access to the underlying open error.
func (e *MissingVolumeError) Unwrap() error { return e.Err }

// EntryError ties an error to the archive entry it happened on, so the
// operation report can list "file X: CRC error" lines.
type EntryError struct {
	Path string // entry path inside the archive (slash separated)
	Err  error
}

func (e *EntryError) Error() string { return fmt.Sprintf("%s: %v", e.Path, e.Err) }

// Unwrap returns the underlying error.
func (e *EntryError) Unwrap() error { return e.Err }
