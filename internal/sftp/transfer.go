// This file moves file contents between the local disk and the server:
// single-file download and upload with progress reporting, cancellation and
// resume of an interrupted transfer, plus recursive folder transfers and a
// remote tree walker built on top of them. Resume works the way Total
// Commander's "Resume" button in the overwrite dialog does: the existing
// partial destination is kept, the source is read from that offset and only
// the missing tail is appended. A cancelled transfer leaves its partial file
// in place precisely so that it can be resumed later; the job engine deletes
// it when the user chose not to keep incomplete files.
package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	pkgsftp "github.com/pkg/sftp"

	"mormicommander/internal/remotefs"
)

// progressFor wraps the user's progress callback so that every progress
// report also counts as activity for the idle timer.
func (c *Client) progressFor(fn remotefs.ProgressFunc) remotefs.ProgressFunc {
	return func(p remotefs.Progress) {
		c.touch()
		if fn != nil {
			fn(p)
		}
	}
}

// Download copies a remote file to a local path and returns the number of
// bytes transferred by this call (excluding a resumed prefix).
func (c *Client) Download(ctx context.Context, remote, local string, opts remotefs.TransferOptions) (int64, error) {
	end, err := c.begin()
	if err != nil {
		return 0, err
	}
	defer end()
	remote = c.abs(remote)

	rf, err := c.sftp.Open(remote)
	if err != nil {
		return 0, pathErr("open", remote, err)
	}
	defer rf.Close()
	rst, err := rf.Stat()
	if err != nil {
		return 0, pathErr("stat", remote, err)
	}
	if rst.IsDir() {
		return 0, pathErr("download", remote, errors.New("is a folder"))
	}
	total := rst.Size()

	var offset int64
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if lst, err := os.Stat(local); err == nil {
		switch {
		case lst.IsDir():
			return 0, pathErr("download", local, errors.New("destination is a folder"))
		case opts.Resume:
			offset = lst.Size()
			flags = os.O_WRONLY | os.O_APPEND
		case opts.NoOverwrite:
			return 0, remotefs.ErrExists
		}
	}
	if opts.Resume && offset >= total {
		// Nothing missing. A larger local file is left untouched: it is
		// not ours to truncate.
		if offset > total {
			return 0, fmt.Errorf("sftp: cannot resume %s: local file is larger than the remote file", local)
		}
		return 0, c.finishLocal(local, rst, opts)
	}
	if offset > 0 {
		if _, err := rf.Seek(offset, io.SeekStart); err != nil {
			return 0, pathErr("seek", remote, err)
		}
	}
	lf, err := os.OpenFile(local, flags, 0o644)
	if err != nil {
		return 0, err
	}
	pw := &remotefs.ProgressWriter{Ctx: ctx, W: lf, Path: remote, Done: offset, Total: total,
		Progress: c.progressFor(opts.Progress)}
	if opts.Progress != nil {
		opts.Progress(remotefs.Progress{Path: remote, Done: offset, Total: total})
	}
	n, err := rf.WriteTo(pw)
	if cerr := lf.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		return n, fmt.Errorf("sftp: downloading %s: %w", remote, err)
	}
	if got := offset + n; got != total {
		return n, fmt.Errorf("sftp: downloading %s: got %d of %d bytes", remote, got, total)
	}
	return n, c.finishLocal(local, rst, opts)
}

func (c *Client) finishLocal(local string, rst fs.FileInfo, opts remotefs.TransferOptions) error {
	if opts.PreserveMode {
		if err := os.Chmod(local, rst.Mode().Perm()); err != nil {
			return err
		}
	}
	if opts.PreserveTimes {
		atime := rst.ModTime()
		if st, ok := rst.Sys().(*pkgsftp.FileStat); ok && st.Atime != 0 {
			atime = time.Unix(int64(st.Atime), 0)
		}
		if err := os.Chtimes(local, atime, rst.ModTime()); err != nil {
			return err
		}
	}
	return nil
}

// Upload copies a local file to a remote path and returns the number of bytes
// transferred by this call (excluding a resumed prefix).
func (c *Client) Upload(ctx context.Context, local, remote string, opts remotefs.TransferOptions) (int64, error) {
	end, err := c.begin()
	if err != nil {
		return 0, err
	}
	defer end()
	remote = c.abs(remote)

	lf, err := os.Open(local)
	if err != nil {
		return 0, err
	}
	defer lf.Close()
	lst, err := lf.Stat()
	if err != nil {
		return 0, err
	}
	if lst.IsDir() {
		return 0, pathErr("upload", local, errors.New("is a folder"))
	}
	total := lst.Size()

	var offset int64
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if rst, err := c.sftp.Stat(remote); err == nil {
		switch {
		case rst.IsDir():
			return 0, pathErr("upload", remote, errors.New("destination is a folder"))
		case opts.Resume:
			offset = rst.Size()
			flags = os.O_WRONLY
		case opts.NoOverwrite:
			return 0, remotefs.ErrExists
		}
	}
	if opts.Resume && offset >= total {
		if offset > total {
			return 0, fmt.Errorf("sftp: cannot resume %s: remote file is larger than the local file", remote)
		}
		return 0, c.finishRemote(remote, lst, opts)
	}
	rf, err := c.sftp.OpenFile(remote, flags)
	if err != nil {
		return 0, pathErr("create", remote, err)
	}
	if offset > 0 {
		if _, err := rf.Seek(offset, io.SeekStart); err != nil {
			rf.Close()
			return 0, pathErr("seek", remote, err)
		}
		if _, err := lf.Seek(offset, io.SeekStart); err != nil {
			rf.Close()
			return 0, err
		}
	}
	pr := &remotefs.ProgressReader{Ctx: ctx, R: lf, Path: remote, Done: offset, Total: total,
		Progress: c.progressFor(opts.Progress)}
	if opts.Progress != nil {
		opts.Progress(remotefs.Progress{Path: remote, Done: offset, Total: total})
	}
	// Hide the size from ReadFrom: with a plain reader pkg/sftp writes
	// sequentially, which keeps cancellation and progress exact.
	n, err := rf.ReadFrom(struct{ io.Reader }{pr})
	if cerr := rf.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		return n, fmt.Errorf("sftp: uploading %s: %w", remote, err)
	}
	if got := offset + n; got != total {
		return n, fmt.Errorf("sftp: uploading %s: sent %d of %d bytes", remote, got, total)
	}
	return n, c.finishRemote(remote, lst, opts)
}

func (c *Client) finishRemote(remote string, lst fs.FileInfo, opts remotefs.TransferOptions) error {
	if opts.PreserveMode {
		if err := c.sftp.Chmod(remote, lst.Mode().Perm()); err != nil {
			return pathErr("chmod", remote, err)
		}
	}
	if opts.PreserveTimes {
		if err := c.sftp.Chtimes(remote, lst.ModTime(), lst.ModTime()); err != nil {
			return pathErr("chtimes", remote, err)
		}
	}
	return nil
}

// CopyRemote copies a file to another path on the same server. SFTP version
// 3 has no server-side copy, so the data is streamed through this machine;
// for the panel this still behaves like Total Commander's "copy within
// server" (F5 with a remote target in the same connection).
func (c *Client) CopyRemote(ctx context.Context, src, dst string, opts remotefs.TransferOptions) (int64, error) {
	end, err := c.begin()
	if err != nil {
		return 0, err
	}
	defer end()
	src, dst = c.abs(src), c.abs(dst)
	if src == dst {
		return 0, pathErr("copy", dst, errors.New("source and destination are the same file"))
	}
	sf, err := c.sftp.Open(src)
	if err != nil {
		return 0, pathErr("open", src, err)
	}
	defer sf.Close()
	sst, err := sf.Stat()
	if err != nil {
		return 0, pathErr("stat", src, err)
	}
	if sst.IsDir() {
		return 0, pathErr("copy", src, errors.New("is a folder"))
	}
	if opts.NoOverwrite {
		if _, err := c.sftp.Lstat(dst); err == nil {
			return 0, remotefs.ErrExists
		}
	}
	df, err := c.sftp.Create(dst)
	if err != nil {
		return 0, pathErr("create", dst, err)
	}
	pw := &remotefs.ProgressWriter{Ctx: ctx, W: df, Path: dst, Total: sst.Size(),
		Progress: c.progressFor(opts.Progress)}
	n, err := io.Copy(pw, struct{ io.Reader }{sf})
	if cerr := df.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		return n, fmt.Errorf("sftp: copying %s: %w", src, err)
	}
	if opts.PreserveMode {
		if err := c.sftp.Chmod(dst, sst.Mode().Perm()); err != nil {
			return n, pathErr("chmod", dst, err)
		}
	}
	if opts.PreserveTimes {
		if err := c.sftp.Chtimes(dst, sst.ModTime(), sst.ModTime()); err != nil {
			return n, pathErr("chtimes", dst, err)
		}
	}
	return n, nil
}

// WalkFunc is called for every entry Walk visits. Returning fs.SkipDir for
// a folder skips its contents; any other error stops the walk. When err is
// not nil the folder e could not be listed.
type WalkFunc func(e remotefs.Entry, err error) error

// Walk visits root and everything below it, folders before their contents,
// in the server's listing order. Symbolic links are reported but never
// followed, so link loops cannot trap it. It backs F8 on folders, the
// Ctrl+L size calculation and Find Files on a remote panel.
func (c *Client) Walk(ctx context.Context, root string, fn WalkFunc) error {
	root = c.abs(root)
	e, err := c.Lstat(root)
	if err != nil {
		return fn(remotefs.Entry{Name: path.Base(root), Path: root}, err)
	}
	if e.IsLink {
		e.IsDir = false // never descend through a link
	}
	return c.walk(ctx, e, fn)
}

func (c *Client) walk(ctx context.Context, e remotefs.Entry, fn WalkFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := fn(e, nil)
	if !e.IsDir || e.IsLink {
		return err
	}
	if errors.Is(err, fs.SkipDir) {
		return nil
	}
	if err != nil {
		return err
	}
	children, err := c.ReadDir(e.Path)
	if err != nil {
		if err := fn(e, err); err != nil && !errors.Is(err, fs.SkipDir) {
			return err
		}
		return nil
	}
	for _, ch := range children {
		if err := c.walk(ctx, ch, fn); err != nil {
			if errors.Is(err, fs.SkipDir) {
				continue
			}
			return err
		}
	}
	return nil
}

// TreeOptions controls DownloadTree and UploadTree.
type TreeOptions struct {
	// File applies to every single file transfer; its Progress receives
	// per-file progress.
	File remotefs.TransferOptions
	// OnFile, when not nil, is called before each file is transferred with
	// its source and destination, so the dialog can show the current name.
	// Returning an error stops the transfer; returning ErrSkip skips just
	// this file (the "Skip" button of the overwrite question).
	OnFile func(src, dst string) error
	// SkipExisting skips files that already exist at the destination
	// instead of failing (Total Commander's "Skip all").
	SkipExisting bool
}

// ErrSkip, returned from TreeOptions.OnFile, skips the current file.
var ErrSkip = errors.New("sftp: skip this file")

// DownloadTree copies a remote folder (or single file) into localDir,
// recreating the folder structure. Links to files are downloaded as regular
// files with the target's content; links to folders are skipped to avoid
// loops. It returns the bytes transferred.
func (c *Client) DownloadTree(ctx context.Context, remoteRoot, localDir string, opts TreeOptions) (int64, error) {
	remoteRoot = c.abs(remoteRoot)
	base := path.Dir(remoteRoot)
	var total int64
	err := c.Walk(ctx, remoteRoot, func(e remotefs.Entry, err error) error {
		if err != nil {
			return err
		}
		rel, err := localRel(base, e.Path)
		if err != nil {
			return err
		}
		dst := filepath.Join(localDir, rel)
		if e.IsLink {
			fi, err := c.Stat(e.Path)
			if err != nil || fi.IsDir {
				return nil // broken link or link to folder
			}
		}
		if e.IsDir {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			if opts.File.PreserveMode {
				os.Chmod(dst, e.Mode.Perm()|0o700)
			}
			return nil
		}
		if opts.SkipExisting {
			if _, err := os.Lstat(dst); err == nil {
				return nil
			}
		}
		if opts.OnFile != nil {
			if err := opts.OnFile(e.Path, dst); err != nil {
				if errors.Is(err, ErrSkip) {
					return nil
				}
				return err
			}
		}
		n, err := c.Download(ctx, e.Path, dst, opts.File)
		total += n
		return err
	})
	return total, err
}

// UploadTree copies a local folder (or single file) into remoteDir,
// recreating the folder structure ("upload with same folder structure").
// Local links to files are uploaded as regular files; links to folders are
// skipped.
func (c *Client) UploadTree(ctx context.Context, localRoot, remoteDir string, opts TreeOptions) (int64, error) {
	remoteDir = c.abs(remoteDir)
	base := filepath.Dir(localRoot)
	var total int64
	err := filepath.WalkDir(localRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		dst := path.Join(remoteDir, filepath.ToSlash(rel))
		isDir := d.IsDir()
		if d.Type()&fs.ModeSymlink != 0 {
			fi, err := os.Stat(p)
			if err != nil || fi.IsDir() {
				return nil
			}
		}
		if isDir {
			if err := c.MkdirAll(dst); err != nil {
				return err
			}
			if opts.File.PreserveMode {
				if fi, err := d.Info(); err == nil {
					c.Chmod(dst, fi.Mode().Perm()|0o700)
				}
			}
			return nil
		}
		if opts.SkipExisting {
			if ok, _ := c.Exists(dst); ok {
				return nil
			}
		}
		if opts.OnFile != nil {
			if err := opts.OnFile(p, dst); err != nil {
				if errors.Is(err, ErrSkip) {
					return nil
				}
				return err
			}
		}
		n, err := c.Upload(ctx, p, dst, opts.File)
		total += n
		return err
	})
	return total, err
}

// TreeSize totals the files below a remote path (Total Commander's Ctrl+L /
// space-bar folder size on a remote panel). Links are not followed.
func (c *Client) TreeSize(ctx context.Context, root string) (files, dirs int, bytes int64, err error) {
	err = c.Walk(ctx, root, func(e remotefs.Entry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case e.IsDir && !e.IsLink:
			dirs++
		default:
			files++
			if !e.IsLink {
				bytes += e.Size
			}
		}
		return nil
	})
	return
}

// relSlash returns target relative to base for slash paths.
// localRel turns a remote path below base into a relative local path that
// is guaranteed to stay inside the destination folder. Names come from the
// server: one with backslashes ("..\\x") is a plain name to the server but a
// path that climbs out of the folder on Windows, and "CON" or "a:b" cannot
// be stored there at all, so the relative path must be local on this OS
// before it is joined to the destination.
func localRel(base, target string) (string, error) {
	rel, err := relSlash(base, target)
	if err != nil {
		return "", err
	}
	lrel := filepath.FromSlash(rel)
	if !fs.ValidPath(rel) || !filepath.IsLocal(lrel) {
		return "", fmt.Errorf("sftp: refusing to download %q: not a safe local name", target)
	}
	return lrel, nil
}

func relSlash(base, target string) (string, error) {
	if base == "/" {
		return target[1:], nil
	}
	if len(target) > len(base) && target[:len(base)] == base && target[len(base)] == '/' {
		return target[len(base)+1:], nil
	}
	return "", fmt.Errorf("%s is not below %s", target, base)
}
