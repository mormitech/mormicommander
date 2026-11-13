// This file gives the local disk the same optional operations the archive
// and network file systems have, so code written against the router (the
// viewer opening a file, the job engine copying between two panels) does not
// need a special case for plain folders. These are deliberately the simple,
// direct versions of each operation: deleting to the recycle bin, keeping
// attributes and ownership, and the overwrite questions of a big copy job
// belong to internal/fileops and internal/trash, which the job engine uses
// for local-to-local work. LocalRW is what the router answers with for paths
// that are neither URLs nor inside an archive.
package vfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
	"syscall"
	"time"
)

// LocalRW is the local file system with the optional operations.
type LocalRW struct{ Local }

// Open opens a local file for reading.
func (LocalRW) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p = Local{}.Clean(p)
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, opErr("open", p, ErrIsFolder)
	}
	return os.Open(p)
}

// Create creates or truncates a local file.
func (LocalRW) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.Create(Local{}.Clean(p))
}

// Mkdir creates a folder and missing parents; an existing path is
// fs.ErrExist.
func (LocalRW) Mkdir(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p = Local{}.Clean(p)
	if _, err := os.Lstat(p); err == nil {
		return &fs.PathError{Op: "mkdir", Path: p, Err: fs.ErrExist}
	}
	return os.MkdirAll(p, 0o777)
}

// Remove deletes a file, a link or an empty folder.
func (LocalRW) Remove(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Remove(Local{}.Clean(p))
}

// RemoveAll deletes a folder tree permanently (links are not followed). A
// missing path is fs.ErrNotExist, as on the other file systems.
func (LocalRW) RemoveAll(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p = Local{}.Clean(p)
	if _, err := os.Lstat(p); err != nil {
		return err
	}
	return os.RemoveAll(p)
}

// Rename moves within the local disk. An existing target is fs.ErrExist,
// except when it is the same file (changing only the case of a name on a
// case-insensitive disk).
func (LocalRW) Rename(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	from, to = Local{}.Clean(from), Local{}.Clean(to)
	if ti, err := os.Lstat(to); err == nil {
		fi, ferr := os.Lstat(from)
		if ferr != nil || !os.SameFile(fi, ti) {
			return &fs.PathError{Op: "rename", Path: to, Err: fs.ErrExist}
		}
	}
	err := os.Rename(from, to)
	var le *os.LinkError
	if errors.As(err, &le) && isCrossDevice(le.Err) {
		return opErr("rename", from, ErrCrossFS)
	}
	return err
}

// Chmod changes permission bits (on Windows only the read-only bit).
func (LocalRW) Chmod(_ context.Context, p string, mode fs.FileMode) error {
	return os.Chmod(Local{}.Clean(p), mode)
}

// Chtimes changes the access and modification time.
func (LocalRW) Chtimes(_ context.Context, p string, mtime time.Time) error {
	return os.Chtimes(Local{}.Clean(p), mtime, mtime)
}

// Download and Upload copy between two local paths.
func (LocalRW) Download(ctx context.Context, p, local string, opt TransferOptions) (int64, error) {
	return localCopy(ctx, Local{}.Clean(p), Local{}.Clean(local), opt)
}

// Upload copies a local file to another local path.
func (LocalRW) Upload(ctx context.Context, local, p string, opt TransferOptions) (int64, error) {
	return localCopy(ctx, Local{}.Clean(local), Local{}.Clean(p), opt)
}

// Caps: everything works on the local disk.
func (LocalRW) Caps(string) Caps {
	return Caps{Read: true, Write: true, Mkdir: true, Delete: true, Rename: true, Chmod: true, Chtimes: true}
}

func localCopy(ctx context.Context, src, dst string, opt TransferOptions) (int64, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	if fi.IsDir() {
		return 0, opErr("copy", src, ErrIsFolder)
	}
	f, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var mt time.Time
	if opt.PreserveTime {
		mt = fi.ModTime()
	}
	return writeLocalAtomic(ctx, dst, f, fi.Size(), mt, opt.Progress)
}

// isCrossDevice reports whether err is EXDEV (Unix) or
// ERROR_NOT_SAME_DEVICE (Windows, errno 17).
func isCrossDevice(err error) bool {
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	var en syscall.Errno
	return runtime.GOOS == "windows" && errors.As(err, &en) && en == 17
}
