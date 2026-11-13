// This file defines what a panel can do with a file system beyond listing it.
// The core FS interface only lists folders, because that is all the panels
// needed so far. Total Commander, however, lets the user view, copy, rename,
// delete and create files inside zip archives, on FTP servers and over SFTP
// exactly as on a local disk, as long as the backend can do it. Rather than
// widening FS (and forcing every read-only backend to stub out a dozen
// methods), each extra ability is a small optional interface here: a backend
// implements the ones it supports, and callers use the helper functions in
// this file, which return ErrUnsupported when the backend cannot do it. Caps
// lets the UI grey out menu items up front (a 7z archive is read-only, an
// FTP server cannot change file times without MFMT, and so on). CopyFile ties
// the pieces together so the job engine can move one file between any two
// backends with progress and cancellation.
package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Errors returned by the adapters. They wrap or sit beside the io/fs errors,
// so errors.Is(err, fs.ErrNotExist) keeps working for missing paths.
var (
	// ErrUnsupported means the backend cannot perform this operation at all
	// (for example writing into a RAR archive).
	ErrUnsupported = errors.New("vfs: operation not supported by this file system")
	// ErrReadOnly means the operation is supported in general but this
	// particular location is read-only (a spanned zip set, an archive nested
	// inside another archive, an archive opened from an FTP server).
	ErrReadOnly = errors.New("vfs: location is read-only")
	// ErrCrossFS is returned by Rename when source and target live on
	// different file systems (two archives, an archive and the disk, two
	// servers). The job engine then copies and deletes instead.
	ErrCrossFS = errors.New("vfs: source and target are on different file systems")
	// ErrNotEmpty is returned by Remove for a folder that still has entries.
	ErrNotEmpty = errors.New("vfs: folder is not empty")
	// ErrIsFolder is returned by Open and Create for a folder.
	ErrIsFolder = errors.New("vfs: is a folder")
)

// Caps tells the UI which operations work at a path, so it can disable menu
// items and choose a copy strategy before trying.
type Caps struct {
	Read    bool // Open, Download
	Write   bool // Create, Upload
	Mkdir   bool
	Delete  bool // Remove, RemoveAll
	Rename  bool // Rename within the same file system
	Chmod   bool // change permission bits
	Chtimes bool // change the modification time
	// Remote is true for network file systems (FTP, SFTP): the UI shows a
	// disconnect button and avoids expensive per-file calls.
	Remote bool
	// Archive is true inside an archive; every write rewrites the archive,
	// so the job engine should batch changes (see Packer).
	Archive bool
}

// TransferOptions control Download and Upload.
type TransferOptions struct {
	// Resume continues a partial target file instead of starting over, where
	// the backend supports it (FTP REST, SFTP offset writes).
	Resume bool
	// PreserveTime copies the modification time to the target.
	PreserveTime bool
	// Progress, when set, receives the bytes done and the total (-1 when
	// unknown). It is called from the transferring goroutine.
	Progress func(done, total int64)
}

// Opener reads files.
type Opener interface {
	// Open returns the content of a file. Folders give ErrIsFolder.
	Open(ctx context.Context, p string) (io.ReadCloser, error)
}

// Creator writes files.
type Creator interface {
	// Create creates or replaces a file. The data is committed when the
	// returned writer is closed; its Close error must be checked.
	Create(ctx context.Context, p string) (io.WriteCloser, error)
}

// DirMaker creates folders.
type DirMaker interface {
	// Mkdir creates one folder; parents are created as needed where the
	// backend allows it.
	Mkdir(ctx context.Context, p string) error
}

// Remover deletes files and folders.
type Remover interface {
	// Remove deletes a file or an empty folder.
	Remove(ctx context.Context, p string) error
	// RemoveAll deletes a file or a folder with everything in it.
	RemoveAll(ctx context.Context, p string) error
}

// Renamer moves entries within one file system.
type Renamer interface {
	// Rename moves from to to; both must be on the same file system (else
	// ErrCrossFS).
	Rename(ctx context.Context, from, to string) error
}

// AttrSetter changes attributes.
type AttrSetter interface {
	Chmod(ctx context.Context, p string, mode fs.FileMode) error
	Chtimes(ctx context.Context, p string, mtime time.Time) error
}

// Transferer moves whole files between the backend and the local disk using
// the backend's native, resumable transfer.
type Transferer interface {
	// Download copies the file p to the local path local.
	Download(ctx context.Context, p, local string, opt TransferOptions) (int64, error)
	// Upload copies the local file local to p.
	Upload(ctx context.Context, local, p string, opt TransferOptions) (int64, error)
}

// Packer adds many local files and folders in one go. Archives implement it
// because each change rewrites the whole archive: packing a selection of
// a thousand files must be one rewrite, not a thousand.
type Packer interface {
	// AddLocal packs the local files and folders (recursively) into the
	// folder dir, keeping their base names.
	AddLocal(ctx context.Context, dir string, sources []string) error
}

// Capper reports capabilities for a path.
type Capper interface {
	Caps(p string) Caps
}

// CapsOf returns the capabilities of fsys at p. Backends that do not
// implement Capper are judged by the interfaces they implement.
func CapsOf(fsys FS, p string) Caps {
	if c, ok := fsys.(Capper); ok {
		return c.Caps(p)
	}
	var c Caps
	_, c.Read = fsys.(Opener)
	_, c.Write = fsys.(Creator)
	_, c.Mkdir = fsys.(DirMaker)
	_, c.Delete = fsys.(Remover)
	_, c.Rename = fsys.(Renamer)
	if _, ok := fsys.(AttrSetter); ok {
		c.Chmod, c.Chtimes = true, true
	}
	return c
}

// OpenFile opens p on fsys for reading.
func OpenFile(ctx context.Context, fsys FS, p string) (io.ReadCloser, error) {
	if o, ok := fsys.(Opener); ok {
		return o.Open(ctx, p)
	}
	return nil, opErr("open", p, ErrUnsupported)
}

// CreateFile creates p on fsys for writing.
func CreateFile(ctx context.Context, fsys FS, p string) (io.WriteCloser, error) {
	if c, ok := fsys.(Creator); ok {
		return c.Create(ctx, p)
	}
	return nil, opErr("create", p, ErrUnsupported)
}

// Mkdir creates the folder p on fsys.
func Mkdir(ctx context.Context, fsys FS, p string) error {
	if m, ok := fsys.(DirMaker); ok {
		return m.Mkdir(ctx, p)
	}
	return opErr("mkdir", p, ErrUnsupported)
}

// Remove deletes the file or empty folder p on fsys.
func Remove(ctx context.Context, fsys FS, p string) error {
	if r, ok := fsys.(Remover); ok {
		return r.Remove(ctx, p)
	}
	return opErr("remove", p, ErrUnsupported)
}

// RemoveAll deletes p and everything below it on fsys.
func RemoveAll(ctx context.Context, fsys FS, p string) error {
	if r, ok := fsys.(Remover); ok {
		return r.RemoveAll(ctx, p)
	}
	return opErr("remove", p, ErrUnsupported)
}

// Rename moves from to to on fsys.
func Rename(ctx context.Context, fsys FS, from, to string) error {
	if r, ok := fsys.(Renamer); ok {
		return r.Rename(ctx, from, to)
	}
	return opErr("rename", from, ErrUnsupported)
}

// Chmod changes the permission bits of p on fsys.
func Chmod(ctx context.Context, fsys FS, p string, mode fs.FileMode) error {
	if a, ok := fsys.(AttrSetter); ok {
		return a.Chmod(ctx, p, mode)
	}
	return opErr("chmod", p, ErrUnsupported)
}

// Chtimes changes the modification time of p on fsys.
func Chtimes(ctx context.Context, fsys FS, p string, mtime time.Time) error {
	if a, ok := fsys.(AttrSetter); ok {
		return a.Chtimes(ctx, p, mtime)
	}
	return opErr("chtimes", p, ErrUnsupported)
}

// opErr wraps err in an *fs.PathError so the UI can show which path failed.
func opErr(op, p string, err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return err
	}
	return &fs.PathError{Op: op, Path: p, Err: err}
}

// CopyFile copies one file from src on sfs to dst on dfs. It picks the most
// efficient route: the backend's own Download when the target is the local
// disk, its Upload when the source is, and a plain Open/Create stream
// otherwise (FTP to SFTP, zip to FTP, ...). local tells which FS is the local
// disk (nil when neither side may be treated as local). The returned count
// is the number of bytes written.
func CopyFile(ctx context.Context, sfs FS, src string, dfs FS, dst string, local FS, opt TransferOptions) (int64, error) {
	if local != nil && dfs == local {
		if t, ok := sfs.(Transferer); ok {
			return t.Download(ctx, src, dst, opt)
		}
	}
	if local != nil && sfs == local {
		if t, ok := dfs.(Transferer); ok {
			return t.Upload(ctx, src, dst, opt)
		}
	}
	total := int64(-1)
	var mtime time.Time
	if e, err := sfs.Stat(src); err == nil {
		if e.Flags&FlagDir != 0 {
			return 0, opErr("copy", src, ErrIsFolder)
		}
		total = e.Size
		mtime = time.UnixMilli(e.MTime)
	}
	r, err := OpenFile(ctx, sfs, src)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	w, err := CreateFile(ctx, dfs, dst)
	if err != nil {
		return 0, err
	}
	n, err := copyCtx(ctx, w, r, total, opt.Progress)
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, err
	}
	if opt.PreserveTime && !mtime.IsZero() && CapsOf(dfs, dst).Chtimes {
		_ = Chtimes(ctx, dfs, dst, mtime) // best effort, like TC
	}
	return n, nil
}

// copyCtx copies with cancellation checks between buffers and progress
// reports after each one.
func copyCtx(ctx context.Context, w io.Writer, r io.Reader, total int64, progress func(done, total int64)) (int64, error) {
	buf := make([]byte, 256<<10)
	var done int64
	for {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		n, rerr := r.Read(buf)
		if n > 0 {
			m, werr := w.Write(buf[:n])
			done += int64(m)
			if progress != nil {
				progress(done, total)
			}
			if werr != nil {
				return done, werr
			}
			if m < n {
				return done, io.ErrShortWrite
			}
		}
		if rerr == io.EOF {
			return done, nil
		}
		if rerr != nil {
			return done, rerr
		}
	}
}

// writeLocalAtomic streams r into the local file dst through a temporary
// file in the same folder, so a cancelled or failed copy never leaves a
// half-written file or destroys the file it was going to replace.
func writeLocalAtomic(ctx context.Context, dst string, r io.Reader, total int64, mtime time.Time, progress func(done, total int64)) (int64, error) {
	dir := filepath.Dir(dst)
	f, err := os.CreateTemp(dir, ".mormi-*.part")
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	n, err := copyCtx(ctx, f, r, total, progress)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && !mtime.IsZero() {
		_ = os.Chtimes(tmp, mtime, mtime)
	}
	if err == nil {
		err = os.Rename(tmp, dst)
		if err != nil {
			// Windows refuses to rename over an existing file in some
			// cases (read-only target); remove it and retry once.
			if rmErr := os.Remove(dst); rmErr == nil {
				err = os.Rename(tmp, dst)
			}
		}
	}
	if err != nil {
		os.Remove(tmp)
		return n, fmt.Errorf("write %s: %w", dst, err)
	}
	return n, nil
}
