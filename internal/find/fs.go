// This file defines the small file system interface the Find Files engine walks.
// Total Commander can search local drives, but also FTP/SFTP connections and other
// virtual folders (F-545), so the walker never calls the os package directly: it asks
// an FS for directory listings, file information and file contents. OSFS is the
// local implementation and is used when Options.FS is left nil; a remote connection
// only has to provide the same handful of methods to become searchable by name, and
// by content too if Open works.

package find

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// FS is what the search needs from a file system. Paths are in the file system's own
// syntax (native paths for OSFS, slash paths for a remote one); Join and Base are part
// of the interface so the walker never has to guess the separator.
type FS interface {
	// Lstat describes name without following a final symbolic link.
	Lstat(name string) (fs.FileInfo, error)
	// Stat describes name, following symbolic links.
	Stat(name string) (fs.FileInfo, error)
	// ReadDir lists a folder. The order does not matter; the walker sorts by name.
	ReadDir(name string) ([]fs.DirEntry, error)
	// Open opens a file for reading its contents (content search, duplicates).
	Open(name string) (io.ReadCloser, error)
	// Join builds a child path.
	Join(elem ...string) string
	// Base returns the last element of a path.
	Base(name string) string
}

// OSFS is the local file system. It is the default when Options.FS is nil, and it is
// the only FS for which the platform attribute bits (hidden flag, system, archive and
// so on) are read from the operating system.
type OSFS struct{}

func (OSFS) Lstat(name string) (fs.FileInfo, error)     { return os.Lstat(name) }
func (OSFS) Stat(name string) (fs.FileInfo, error)      { return os.Stat(name) }
func (OSFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }
func (OSFS) Join(elem ...string) string                 { return filepath.Join(elem...) }
func (OSFS) Base(name string) string                    { return filepath.Base(name) }

// ErrNotRegular is returned by OSFS.Open for named pipes, sockets and devices.
// Opening a pipe that has no writer would block forever, out of reach of the
// search's context, so such entries are refused before the open call.
var ErrNotRegular = errors.New("find: not a regular file")

// Open opens a local file for reading, refusing pipes, sockets and devices.
func (OSFS) Open(name string) (io.ReadCloser, error) {
	st, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if st.Mode()&specialMode != 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: ErrNotRegular}
	}
	return os.Open(name)
}

// SlashFS is a helper base for remote file systems that use forward slashes (FTP,
// SFTP, WebDAV). Embed it to get Join and Base with slash semantics.
type SlashFS struct{}

func (SlashFS) Join(elem ...string) string { return path.Join(elem...) }
func (SlashFS) Base(name string) string    { return path.Base(name) }

// isOSFS tells whether attribute bits may be read from the operating system.
func isOSFS(f FS) bool {
	switch f.(type) {
	case OSFS, *OSFS:
		return true
	}
	return false
}
