// This file defines the small file system interface the sync engine works
// through. Total Commander can synchronize a local folder with an archive or
// an FTP folder (FEATURES.md F-555), so the engine never calls the os
// package directly: each side is an FS, and a side that can be changed is a
// WritableFS. OSFS is the local disk implementation and FromFS wraps any
// io/fs file system (a zip.Reader, an embed.FS, os.DirFS) as a read-only
// side. The virtual file system package can later provide adapters for
// archives and remote servers that implement the same interface.
//
// OSFS is also where most of the symlink safety lives. Every write or
// delete first checks that no folder on the way to the target is a symbolic
// link, so a link planted inside the tree (say "photos" pointing at the
// home folder) can never make a sync write or delete outside the folders the
// user picked.
package syncdirs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mormicommander/internal/trash"
)

// ErrSymlinkInPath is returned when a write or delete would pass through a
// folder that is a symbolic link.
var ErrSymlinkInPath = errors.New("syncdirs: path goes through a symbolic link")

// ErrNotRegular is returned when a copy or a content compare would have to
// open a named pipe, socket or device.
var ErrNotRegular = errors.New("syncdirs: not a regular file")

// ErrNoTrash is returned by SystemTrash for sides that are not on the local
// disk or on systems without a trash.
var ErrNoTrash = errors.New("syncdirs: moving to the trash is not possible here")

// ErrReadOnly is returned when the plan needs to change a side whose FS does
// not implement WritableFS (an archive opened read-only, for example).
var ErrReadOnly = errors.New("syncdirs: side is read-only")

// FS is a read-only view of one side. Names are slash-separated paths
// relative to the side's root, with "." for the root itself, as in io/fs.
type FS interface {
	// Lstat describes the named entry without following a final symlink.
	Lstat(name string) (fs.FileInfo, error)
	// Stat describes the named entry, following symlinks.
	Stat(name string) (fs.FileInfo, error)
	// ReadDir lists a folder.
	ReadDir(name string) ([]fs.DirEntry, error)
	// Open opens a file for reading.
	Open(name string) (io.ReadCloser, error)
	// Readlink returns the target of a symbolic link.
	Readlink(name string) (string, error)
}

// RealPather is implemented by file systems that can resolve a path to its
// canonical form with all symlinks expanded. The compare uses it to detect
// symlink loops when FollowSymlinks is on; without it a depth limit applies.
type RealPather interface {
	RealPath(name string) (string, error)
}

// WritableFS is a side the engine can change.
type WritableFS interface {
	FS
	// Mkdir creates one folder; the parent must exist.
	Mkdir(name string, perm fs.FileMode) error
	// CreateTemp creates a new temporary file in folder dir and returns it
	// with its name (relative to the root). The engine writes the copy
	// there and then renames it over the target, so a failed or cancelled
	// copy never leaves a half-written target behind.
	CreateTemp(dir string) (io.WriteCloser, string, error)
	// Rename moves oldname to newname, replacing newname if it exists.
	Rename(oldname, newname string) error
	// Remove deletes a file, a symlink (not its target) or an empty folder.
	Remove(name string) error
	// Chtimes sets the modification time.
	Chtimes(name string, mtime time.Time) error
	// Chmod sets the permission bits.
	Chmod(name string, mode fs.FileMode) error
	// Symlink creates name as a symbolic link to target.
	Symlink(target, name string) error
}

// OSFS is a side on the local disk, rooted at Root.
type OSFS struct {
	Root string
}

// DirFS returns a writable side rooted at the local folder root.
func DirFS(root string) *OSFS { return &OSFS{Root: root} }

func (o *OSFS) abs(op, name string) (string, error) {
	bad := !fs.ValidPath(name)
	if runtime.GOOS == "windows" && strings.ContainsAny(name, `\:`) {
		bad = true
	}
	if bad {
		return "", &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	return filepath.Join(o.Root, filepath.FromSlash(name)), nil
}

// writable resolves name for a change and refuses it when a parent folder
// is a symbolic link.
func (o *OSFS) writable(op, name string) (string, error) {
	p, err := o.abs(op, name)
	if err != nil {
		return "", err
	}
	if name == "." {
		return p, nil
	}
	parts := strings.Split(name, "/")
	cur := o.Root
	for _, part := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return p, nil
			}
			return "", err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return "", &fs.PathError{Op: op, Path: name, Err: ErrSymlinkInPath}
		}
	}
	return p, nil
}

// Lstat implements FS.
func (o *OSFS) Lstat(name string) (fs.FileInfo, error) {
	p, err := o.abs("lstat", name)
	if err != nil {
		return nil, err
	}
	return os.Lstat(p)
}

// Stat implements FS.
func (o *OSFS) Stat(name string) (fs.FileInfo, error) {
	p, err := o.abs("stat", name)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

// ReadDir implements FS.
func (o *OSFS) ReadDir(name string) ([]fs.DirEntry, error) {
	p, err := o.abs("readdir", name)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(p)
}

// Open implements FS. Pipes, sockets and devices are refused before the
// open call, which could otherwise block forever.
func (o *OSFS) Open(name string) (io.ReadCloser, error) {
	p, err := o.abs("open", name)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if st.Mode()&specialMode != 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: ErrNotRegular}
	}
	return os.Open(p)
}

// Readlink implements FS.
func (o *OSFS) Readlink(name string) (string, error) {
	p, err := o.abs("readlink", name)
	if err != nil {
		return "", err
	}
	return os.Readlink(p)
}

// RealPath implements RealPather.
func (o *OSFS) RealPath(name string) (string, error) {
	p, err := o.abs("realpath", name)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

// Mkdir implements WritableFS.
func (o *OSFS) Mkdir(name string, perm fs.FileMode) error {
	p, err := o.writable("mkdir", name)
	if err != nil {
		return err
	}
	return os.Mkdir(p, perm)
}

// CreateTemp implements WritableFS.
func (o *OSFS) CreateTemp(dir string) (io.WriteCloser, string, error) {
	p, err := o.writable("createtemp", path.Join(dir, "x"))
	if err != nil {
		return nil, "", err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".mcsync-*.tmp")
	if err != nil {
		return nil, "", err
	}
	return f, path.Join(dir, filepath.Base(f.Name())), nil
}

// Rename implements WritableFS.
func (o *OSFS) Rename(oldname, newname string) error {
	op, err := o.writable("rename", oldname)
	if err != nil {
		return err
	}
	np, err := o.writable("rename", newname)
	if err != nil {
		return err
	}
	return replaceFile(op, np)
}

// Remove implements WritableFS.
func (o *OSFS) Remove(name string) error {
	if name == "." {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrInvalid}
	}
	p, err := o.writable("remove", name)
	if err != nil {
		return err
	}
	return removeEntry(p)
}

// Chtimes implements WritableFS.
func (o *OSFS) Chtimes(name string, mtime time.Time) error {
	p, err := o.writable("chtimes", name)
	if err != nil {
		return err
	}
	return os.Chtimes(p, mtime, mtime)
}

// Chmod implements WritableFS.
func (o *OSFS) Chmod(name string, mode fs.FileMode) error {
	p, err := o.writable("chmod", name)
	if err != nil {
		return err
	}
	return os.Chmod(p, mode)
}

// Symlink implements WritableFS.
func (o *OSFS) Symlink(target, name string) error {
	p, err := o.writable("symlink", name)
	if err != nil {
		return err
	}
	return os.Symlink(target, p)
}

// SystemTrash moves an entry of a local side (an *OSFS) to the system trash
// or Recycle Bin. Pass it as ExecOptions.Trash when the "Delete to trash"
// setting is on. Other sides give ErrNoTrash, and like every write it
// refuses paths that pass through a symlinked folder.
func SystemTrash(f WritableFS, name string) error {
	o, ok := f.(*OSFS)
	if !ok || !trash.Supported() {
		return ErrNoTrash
	}
	if name == "." {
		return &fs.PathError{Op: "trash", Path: name, Err: fs.ErrInvalid}
	}
	p, err := o.writable("trash", name)
	if err != nil {
		return err
	}
	return trash.Move(p)
}

// FromFS wraps an io/fs file system as a read-only side. Symlinks are
// reported when the file system implements fs.ReadLinkFS (os.DirFS does).
func FromFS(fsys fs.FS) FS { return roFS{fsys} }

type roFS struct{ fsys fs.FS }

func (r roFS) Lstat(name string) (fs.FileInfo, error)     { return fs.Lstat(r.fsys, name) }
func (r roFS) Stat(name string) (fs.FileInfo, error)      { return fs.Stat(r.fsys, name) }
func (r roFS) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(r.fsys, name) }
func (r roFS) Open(name string) (io.ReadCloser, error)    { return r.fsys.Open(name) }
func (r roFS) Readlink(name string) (string, error)       { return fs.ReadLink(r.fsys, name) }
