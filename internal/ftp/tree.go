// This file adds whole-folder operations on top of the single-item FTP
// commands, because the protocol itself only knows files and empty folders:
// walking a remote tree, creating a path with all its parents, deleting a
// folder with everything in it, and copying complete folders in either
// direction while keeping the same structure (Total Commander's "upload with
// same folder structure"). Symbolic links on the server are reported but not
// followed, so a link loop cannot make a walk run forever and deleting a
// link never deletes what it points to.
package ftp

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WalkFunc is called for every entry below the walk root. Returning
// fs.SkipDir for a folder skips its contents; fs.SkipAll ends the walk.
type WalkFunc func(p string, e Entry, err error) error

// Walk visits root's contents depth-first in listing order (the root itself
// is not reported). Folders are reported before their contents.
func (c *Client) Walk(ctx context.Context, root string, fn WalkFunc) error {
	err := c.walk(ctx, root, fn)
	if errors.Is(err, fs.SkipAll) {
		return nil
	}
	return err
}

func (c *Client) walk(ctx context.Context, dir string, fn WalkFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	es, err := c.List(ctx, dir)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fn(dir, Entry{Name: path.Base(dir), Type: TypeDir}, err)
	}
	for _, e := range es {
		if e.Name == "" || e.Name == "." || e.Name == ".." || strings.ContainsRune(e.Name, '/') {
			continue // not a plain child name; following it could leave the tree
		}
		p := Join(dir, e.Name)
		if err := fn(p, e, nil); err != nil {
			if errors.Is(err, fs.SkipDir) {
				if e.IsDir() {
					continue
				}
				return nil // SkipDir on a file skips the rest of this folder
			}
			return err
		}
		if e.IsDir() {
			if err := c.walk(ctx, p, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

// MkdirAll creates p and any missing parents; existing folders are fine.
func (c *Client) MkdirAll(ctx context.Context, p string) error {
	clean := path.Clean(p)
	if clean == "/" || clean == "." || clean == "" {
		return nil
	}
	if e, err := c.Stat(ctx, clean); err == nil {
		if e.IsDir() || e.Type == TypeLink {
			return nil
		}
		return &fs.PathError{Op: "mkdir", Path: p, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if parent := path.Dir(clean); parent != clean {
		if err := c.MkdirAll(ctx, parent); err != nil {
			return err
		}
	}
	err := c.Mkdir(ctx, clean)
	if err != nil && ReplyCode(err) == 550 {
		// Someone (or a parallel transfer) may have created it meanwhile.
		if e, serr := c.Stat(ctx, clean); serr == nil && e.IsDir() {
			return nil
		}
	}
	return err
}

// RemoveAll deletes p and, for a folder, everything below it. A missing
// path is not an error.
func (c *Client) RemoveAll(ctx context.Context, p string) error {
	e, err := c.Stat(ctx, p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !e.IsDir() {
		return c.Delete(ctx, p)
	}
	return c.removeDir(ctx, p)
}

func (c *Client) removeDir(ctx context.Context, dir string) error {
	es, err := c.List(ctx, dir)
	if err != nil {
		return err
	}
	for _, e := range es {
		if e.Name == "" || e.Name == "." || e.Name == ".." || strings.ContainsRune(e.Name, '/') {
			continue
		}
		p := Join(dir, e.Name)
		if e.IsDir() {
			err = c.removeDir(ctx, p)
		} else {
			err = c.Delete(ctx, p)
		}
		if err != nil {
			return err
		}
	}
	return c.Rmdir(ctx, dir)
}

// TreeOptions tune UploadTree and DownloadTree.
type TreeOptions struct {
	TransferOptions
	// OnFile is called before each file is copied; returning false skips
	// it (the UI uses this for overwrite questions and masks).
	OnFile func(local, remote string, size int64) bool
}

// TreeResult counts what a tree copy did.
type TreeResult struct {
	Files int
	Dirs  int
	Bytes int64
	// Skipped lists the remote paths DownloadTree left out: links the
	// server would not deliver as files (links to folders, broken links)
	// and special entries such as devices or pipes. The caller shows them
	// so the user knows the copy is not complete.
	Skipped []string
}

// UploadTree copies the local folder localDir into remoteDir (created if
// needed), recreating the same folder structure on the server.
func (c *Client) UploadTree(ctx context.Context, localDir, remoteDir string, opt TreeOptions) (TreeResult, error) {
	var res TreeResult
	if err := c.MkdirAll(ctx, remoteDir); err != nil {
		return res, err
	}
	err := filepath.WalkDir(localDir, func(lp string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(localDir, lp)
		if err != nil || rel == "." {
			return err
		}
		rp := Join(remoteDir, filepath.ToSlash(rel))
		switch {
		case d.IsDir():
			if err := c.MkdirAll(ctx, rp); err != nil {
				return err
			}
			res.Dirs++
		case d.Type().IsRegular():
			fi, err := d.Info()
			if err != nil {
				return err
			}
			if opt.OnFile != nil && !opt.OnFile(lp, rp, fi.Size()) {
				return nil
			}
			n, err := c.Upload(ctx, lp, rp, opt.TransferOptions)
			res.Bytes += n
			if err != nil {
				return err
			}
			res.Files++
		}
		return nil
	})
	return res, err
}

// DownloadTree copies the remote folder remoteDir into the local folder
// localDir (created if needed), keeping the folder structure. Remote
// symbolic links are downloaded as the file they point to, the way Total
// Commander does it (RETR follows the link on the server). Links the server
// refuses to deliver as a file (links to folders, broken links) and other
// special entries are not followed; they are listed in TreeResult.Skipped
// instead of being dropped silently.
func (c *Client) DownloadTree(ctx context.Context, remoteDir, localDir string, opt TreeOptions) (TreeResult, error) {
	var res TreeResult
	if err := os.MkdirAll(localDir, 0o777); err != nil {
		return res, err
	}
	err := c.Walk(ctx, remoteDir, func(rp string, e Entry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(rp, remoteDir), "/")
		if rel == "" || !fs.ValidPath(rel) || !filepath.IsLocal(filepath.FromSlash(rel)) {
			// A hostile listing ("../x", "..\x") must never make us
			// write outside localDir.
			if e.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		lp := filepath.Join(localDir, filepath.FromSlash(rel))
		switch e.Type {
		case TypeDir:
			if err := os.MkdirAll(lp, 0o777); err != nil {
				return err
			}
			res.Dirs++
		case TypeFile:
			if opt.OnFile != nil && !opt.OnFile(lp, rp, e.Size) {
				return nil
			}
			n, err := c.Download(ctx, rp, lp, opt.TransferOptions)
			res.Bytes += n
			if err != nil {
				return err
			}
			res.Files++
		case TypeLink:
			if opt.OnFile != nil && !opt.OnFile(lp, rp, -1) {
				return nil
			}
			n, err := c.Download(ctx, rp, lp, opt.TransferOptions)
			res.Bytes += n
			if err != nil {
				// A permanent refusal (550 "not a plain file") means the
				// link points at a folder or nowhere. Folders behind
				// links are not followed, so a link loop cannot make
				// the copy run forever.
				if code := ReplyCode(err); code >= 500 && code < 600 && n == 0 {
					res.Skipped = append(res.Skipped, rp)
					return nil
				}
				return err
			}
			res.Files++
		default:
			res.Skipped = append(res.Skipped, rp)
		}
		return nil
	})
	return res, err
}
