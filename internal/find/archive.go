// This file is the hook for "Search archives" (F-528). Total Commander can look
// inside ZIP, 7z, RAR, TAR and other packed files while searching and lists hits as
// "c:\folder\pack.zip\inner\file.txt". The archive readers live in their own package
// (internal/archive, built by another agent), so this engine only defines the small
// interface it needs: is this file an archive, and walk its entries, giving a way to
// open each one for content search. Entries then go through the same name, size,
// date, attribute and text filters as ordinary files.

package find

import (
	"context"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

// ArchiveEntry describes one member of an archive.
type ArchiveEntry struct {
	Name    string      // path inside the archive, with forward slashes
	Size    int64       // unpacked size
	ModTime time.Time   // modification time stored in the archive
	IsDir   bool        // a folder entry
	Mode    fs.FileMode // permission bits if the format stores them, else 0
}

// ArchiveHandler gives the search access to archives.
type ArchiveHandler interface {
	// IsArchive reports whether the file at path should be looked into. It is
	// usually an extension check; info is the file's (followed) information.
	IsArchive(path string, info fs.FileInfo) bool
	// Walk calls fn for every entry of the archive at path. open returns the unpacked
	// contents of that entry and is only valid during the call. If fn returns an
	// error Walk must stop and return it; Walk should also stop when ctx is done.
	Walk(ctx context.Context, f FS, path string, fn func(e ArchiveEntry, open func() (io.ReadCloser, error)) error) error
}

// ArchiveFuncs adapts two functions to ArchiveHandler, for wiring without a type.
type ArchiveFuncs struct {
	Is       func(path string, info fs.FileInfo) bool
	WalkFunc func(ctx context.Context, f FS, path string, fn func(e ArchiveEntry, open func() (io.ReadCloser, error)) error) error
}

func (a ArchiveFuncs) IsArchive(p string, info fs.FileInfo) bool { return a.Is != nil && a.Is(p, info) }
func (a ArchiveFuncs) Walk(ctx context.Context, f FS, p string, fn func(ArchiveEntry, func() (io.ReadCloser, error)) error) error {
	return a.WalkFunc(ctx, f, p, fn)
}

// ArchiveExtensions returns an IsArchive function for a list of extensions such as
// ".zip", ".7z", ".tar.gz" (compared without regard to case).
func ArchiveExtensions(exts ...string) func(path string, info fs.FileInfo) bool {
	low := make([]string, len(exts))
	for i, e := range exts {
		low[i] = strings.ToLower(e)
	}
	return func(p string, info fs.FileInfo) bool {
		if info != nil && info.IsDir() {
			return false
		}
		n := strings.ToLower(p)
		for _, e := range low {
			if strings.HasSuffix(n, e) {
				return true
			}
		}
		return false
	}
}

// cleanInner normalises an archive member name: forward slashes, no leading "./" or
// "/", no trailing slash. Names that try to climb out ("../") are kept as written but
// cleaned, because the search only displays them.
func cleanInner(name string) string {
	n := strings.ReplaceAll(name, `\`, "/")
	n = path.Clean("/" + n)
	return strings.TrimPrefix(n, "/")
}
