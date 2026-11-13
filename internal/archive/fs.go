// This file exposes any Archive as a standard io/fs file system. That lets
// code written for real folders work inside archives unchanged: fs.WalkDir
// powers "Find files" inside archives and the branch view (F-626), and the
// virtual file system layer can mount an archive as a folder without knowing
// the format. Paths follow io/fs rules ("." is the root, no leading slash).
package archive

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

// FS returns a read-only fs.FS view of a. It also implements fs.ReadDirFS
// and fs.StatFS.
func FS(a Archive) fs.FS { return archFS{a} }

type archFS struct{ a Archive }

func toArchivePath(op, name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	if strings.Contains(name, "\\") {
		// Archive paths are cleaned to forward slashes; a backslash in an
		// io/fs name is a literal character that no entry can contain.
		return "", &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
	}
	if name == "." {
		return "", nil
	}
	return name, nil
}

func (f archFS) Open(name string) (fs.File, error) {
	p, err := toArchivePath("open", name)
	if err != nil {
		return nil, err
	}
	e, err := f.a.Stat(p)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if e.IsDir {
		kids, err := f.a.ReadDir(p)
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		return &dirFile{info: EntryInfo(e, name), kids: kids}, nil
	}
	rc, err := f.a.Open(e)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return &file{info: EntryInfo(e, name), rc: rc}, nil
}

func (f archFS) Stat(name string) (fs.FileInfo, error) {
	p, err := toArchivePath("stat", name)
	if err != nil {
		return nil, err
	}
	e, err := f.a.Stat(p)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return EntryInfo(e, name), nil
}

func (f archFS) ReadDir(name string) ([]fs.DirEntry, error) {
	p, err := toArchivePath("readdir", name)
	if err != nil {
		return nil, err
	}
	kids, err := f.a.ReadDir(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = fs.ErrNotExist
		}
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}
	out := make([]fs.DirEntry, len(kids))
	for i, k := range kids {
		out[i] = fs.FileInfoToDirEntry(EntryInfo(k, k.Name))
	}
	return out, nil
}

// EntryInfo adapts an Entry to fs.FileInfo. name is what Name() returns;
// pass "" to use the entry's own name.
func EntryInfo(e Entry, name string) fs.FileInfo {
	if name == "" || name == "." {
		name = e.Name
		if name == "" {
			name = "."
		}
	}
	return entryInfo{e: e, name: path.Base(name)}
}

type entryInfo struct {
	e    Entry
	name string
}

func (i entryInfo) Name() string       { return i.name }
func (i entryInfo) Size() int64        { return i.e.Size }
func (i entryInfo) Mode() fs.FileMode  { return i.e.Mode }
func (i entryInfo) ModTime() time.Time { return i.e.ModTime }
func (i entryInfo) IsDir() bool        { return i.e.IsDir }
func (i entryInfo) Sys() any           { return i.e }

type file struct {
	info fs.FileInfo
	rc   io.ReadCloser
}

func (f *file) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *file) Read(b []byte) (int, error) { return f.rc.Read(b) }
func (f *file) Close() error               { return f.rc.Close() }

type dirFile struct {
	info fs.FileInfo
	kids []Entry
	pos  int
}

func (d *dirFile) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *dirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.Name(), Err: errors.New("is a directory")}
}
func (d *dirFile) Close() error { return nil }

func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.kids[d.pos:]
	if n > 0 && len(rest) == 0 {
		return nil, io.EOF
	}
	if n > 0 && n < len(rest) {
		rest = rest[:n]
	}
	d.pos += len(rest)
	out := make([]fs.DirEntry, len(rest))
	for i, k := range rest {
		out[i] = fs.FileInfoToDirEntry(EntryInfo(k, k.Name))
	}
	return out, nil
}
