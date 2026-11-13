// This file packs files into new tar-family archives (TC's Alt+F5 pack
// dialog with the tar, tgz, tbz, txz, tzst packers, and the gz, bz2, xz, zst
// single-file packers). It follows the dialog's options: store path names or
// bare names, recurse into subfolders or not, put everything under a folder
// inside the archive, follow symlinks or store them as links, and "move to
// archive", which deletes the originals only after the new archive has been
// read back and tested. The archive is written under a temporary name next
// to the target and renamed at the end, so a cancelled or failed pack never
// leaves a broken archive behind and never destroys an older one.
package tarfmt

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	archive "mormicommander/internal/archive/arcapi"
)

// CreateOptions are the settings of the pack dialog.
type CreateOptions struct {
	// Format of the new archive; FormatUnknown means "from the file name".
	Format Format
	// Level is the compression level, 1 (fastest) .. 9 (smallest), 0 for
	// the codec default.
	Level int
	// BaseDir is the folder names are stored relative to (normally the
	// panel's current folder). Empty means each source is stored under its
	// own base name.
	BaseDir string
	// Prefix is a folder inside the archive to store everything under.
	Prefix string
	// NoPaths stores bare file names without folders ("pack path names"
	// unchecked).
	NoPaths bool
	// NoRecurse stores only the files directly inside selected folders,
	// not their subfolders ("recursively pack subdirectories" unchecked).
	NoRecurse bool
	// FollowSymlinks stores what a symlink points to instead of the link.
	FollowSymlinks bool
	// RemoveSources deletes the packed files and folders after the archive
	// has been written and tested ("move to archive").
	RemoveSources bool
	// Progress, when set, receives regular updates.
	Progress func(archive.Progress)
}

// item is one file system object queued for packing.
type item struct {
	src  string // local path
	name string // archive name (slash form, no trailing slash)
	fi   fs.FileInfo
	link string // symlink target when stored as a link
	top  bool   // named directly by the caller
}

// collect walks the sources and returns what will be stored, in order.
func collect(ctx context.Context, sources []string, opt CreateOptions, skip []fs.FileInfo) ([]item, error) {
	prefix := cleanName(opt.Prefix)
	if prefix != "" && !isLocalName(prefix) {
		return nil, fmt.Errorf("tarfmt: invalid folder %q inside archive", opt.Prefix)
	}
	var items []item
	seen := map[string]bool{}
	isSkipped := func(fi fs.FileInfo) bool {
		for _, s := range skip {
			if s != nil && os.SameFile(fi, s) {
				return true
			}
		}
		return false
	}
	var walk func(src, rel string, fi fs.FileInfo, depth int, top bool) error
	walk = func(src, rel string, fi fs.FileInfo, depth int, top bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 64 {
			return fmt.Errorf("tarfmt: %s: folders nested too deeply (symlink loop?)", src)
		}
		link := ""
		if fi.Mode()&fs.ModeSymlink != 0 {
			if opt.FollowSymlinks {
				tfi, err := os.Stat(src)
				if err != nil {
					return err
				}
				fi = tfi
			} else {
				l, err := os.Readlink(src)
				if err != nil {
					return err
				}
				link = filepath.ToSlash(l)
			}
		}
		if isSkipped(fi) {
			return nil
		}
		name := path.Join(prefix, rel)
		if opt.NoPaths {
			name = path.Join(prefix, path.Base(rel))
		}
		mode := fi.Mode()
		switch {
		case link != "" || mode.IsRegular():
			if !seen[src] {
				seen[src] = true
				items = append(items, item{src: src, name: name, fi: fi, link: link, top: top})
			}
			return nil
		case mode.IsDir():
			if !opt.NoPaths && !seen[src] {
				seen[src] = true
				items = append(items, item{src: src, name: name, fi: fi, top: top})
			}
			if opt.NoRecurse && !top {
				return nil
			}
			des, err := os.ReadDir(src)
			if err != nil {
				return err
			}
			for _, de := range des {
				cfi, err := de.Info()
				if err != nil {
					return err
				}
				if opt.NoRecurse && cfi.IsDir() {
					continue
				}
				if err := walk(filepath.Join(src, de.Name()), path.Join(rel, de.Name()), cfi, depth+1, false); err != nil {
					return err
				}
			}
		}
		// Sockets, devices and pipes are skipped.
		return nil
	}
	for _, s := range sources {
		abs, err := filepath.Abs(s)
		if err != nil {
			return nil, err
		}
		if opt.BaseDir != "" && !filepath.IsAbs(s) {
			abs = filepath.Join(opt.BaseDir, s)
		}
		fi, err := os.Lstat(abs)
		if err != nil {
			return nil, err
		}
		rel := filepath.Base(abs)
		if opt.BaseDir != "" {
			base, err := filepath.Abs(opt.BaseDir)
			if err != nil {
				return nil, err
			}
			if r, err := filepath.Rel(base, abs); err == nil && filepath.IsLocal(r) {
				rel = r
			}
		}
		if err := walk(abs, filepath.ToSlash(rel), fi, 0, true); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// packer writes items into a tar stream.
type packer struct {
	ctx   context.Context
	tw    *tar.Writer
	prog  *progress
	links map[fileKey]string
}

func newPacker(ctx context.Context, tw *tar.Writer, prog *progress) *packer {
	return &packer{ctx: ctx, tw: tw, prog: prog, links: map[fileKey]string{}}
}

func (pk *packer) add(it item) error {
	if err := pk.ctx.Err(); err != nil {
		return err
	}
	h, err := tar.FileInfoHeader(it.fi, it.link)
	if err != nil {
		return fmt.Errorf("%s: %w", it.src, err)
	}
	h.Name = it.name
	h.ModTime = it.fi.ModTime()
	h.AccessTime, h.ChangeTime = time.Time{}, time.Time{}
	h.Format = tar.FormatUnknown
	if it.fi.IsDir() {
		h.Name += "/"
	}
	if it.link != "" {
		h.Linkname = it.link
	}
	if key, ok := hardLinkKey(it.fi); ok && it.link == "" {
		if first, ok := pk.links[key]; ok {
			h.Typeflag = tar.TypeLink
			h.Linkname = first
			h.Size = 0
		} else {
			pk.links[key] = it.name
		}
	}
	pk.prog.file(it.name)
	if err := pk.tw.WriteHeader(h); err != nil {
		return fmt.Errorf("%s: %w", it.src, err)
	}
	if h.Typeflag != tar.TypeReg || h.Size == 0 {
		return nil
	}
	f, err := os.Open(it.src)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(ctxWriter{pk.ctx, pk.prog.writer(pk.tw)}, io.LimitReader(f, h.Size))
	if err != nil {
		return fmt.Errorf("%s: %w", it.src, err)
	}
	if n != h.Size {
		return fmt.Errorf("%s: file shrank while being packed", it.src)
	}
	return nil
}

// tempFor creates the temporary output file next to final.
func tempFor(final string) (*os.File, error) {
	dir, base := filepath.Split(final)
	if dir == "" {
		dir = "."
	}
	return os.CreateTemp(dir, "."+base+".mctmp-*")
}

// Create packs sources into a new archive at archivePath, replacing any
// existing file of that name only after the new archive is complete (the UI
// asks the user before calling this for an existing name).
func Create(ctx context.Context, archivePath string, sources []string, opt CreateOptions) error {
	// Replace the file a symbolic link points to, not the link.
	archivePath = archive.ReplaceTarget(archivePath)
	f := opt.Format
	if f == FormatUnknown {
		f = DetectFormat(archivePath)
	}
	if f == FormatUnknown {
		return fmt.Errorf("tarfmt: cannot tell the archive format from %q", filepath.Base(archivePath))
	}
	if len(sources) == 0 {
		return errors.New("tarfmt: nothing to pack")
	}
	tmp, err := tempFor(archivePath)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	skip := []fs.FileInfo{}
	if fi, err := tmp.Stat(); err == nil {
		skip = append(skip, fi)
	}
	if fi, err := os.Stat(archivePath); err == nil {
		skip = append(skip, fi)
	}
	items, err := collect(ctx, sources, opt, skip)
	if err != nil {
		return err
	}
	prog := progressFn(opt.Progress, 0, nil)
	for _, it := range items {
		if it.link == "" && it.fi.Mode().IsRegular() {
			prog.p.Total += it.fi.Size()
		}
	}
	if f.IsTar() {
		err = writeTar(ctx, tmp, f, opt.Level, items, prog)
	} else {
		err = writeSingle(ctx, tmp, f, opt.Level, items, prog)
	}
	if err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if opt.RemoveSources {
		// Verify before anything is deleted.
		if err := Test(ctx, tmpName, nil); err != nil {
			return fmt.Errorf("tarfmt: new archive failed its test, sources kept: %w", err)
		}
	}
	if err := os.Rename(tmpName, archivePath); err != nil {
		return err
	}
	ok = true
	prog.flush()
	if opt.RemoveSources {
		return removeItems(items)
	}
	return nil
}

func writeTar(ctx context.Context, w io.Writer, f Format, level int, items []item, prog *progress) error {
	cw, err := compress(f.Compression(), w, level)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(cw)
	pk := newPacker(ctx, tw, prog)
	for _, it := range items {
		if err := pk.add(it); err != nil {
			cw.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		cw.Close()
		return err
	}
	return cw.Close()
}

// writeSingle compresses exactly one regular file.
func writeSingle(ctx context.Context, w io.Writer, f Format, level int, items []item, prog *progress) error {
	var files []item
	for _, it := range items {
		if it.fi.Mode().IsRegular() && it.link == "" {
			files = append(files, it)
		}
	}
	if len(files) != 1 || len(items) != 1 {
		return fmt.Errorf("tarfmt: the %s format holds exactly one file; use a tar format for several files or folders", f)
	}
	it := files[0]
	cw, err := compress(f.Compression(), w, level)
	if err != nil {
		return err
	}
	if gz, ok := cw.(*gzip.Writer); ok {
		gz.Name = path.Base(it.name)
		gz.ModTime = it.fi.ModTime()
	}
	src, err := os.Open(it.src)
	if err != nil {
		cw.Close()
		return err
	}
	defer src.Close()
	prog.file(it.name)
	if _, err := io.Copy(ctxWriter{ctx, prog.writer(cw)}, src); err != nil {
		cw.Close()
		return err
	}
	return cw.Close()
}

// removeItems deletes packed sources for "move to archive": files and links
// first, then folders deepest first, and a folder only when it is empty (it
// may still hold things that were not packed, for example with NoRecurse).
func removeItems(items []item) error {
	var dirs []string
	var errs []error
	for _, it := range items {
		if it.fi.IsDir() && it.link == "" {
			dirs = append(dirs, it.src)
			continue
		}
		if err := os.Remove(it.src); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) > strings.Count(dirs[j], string(filepath.Separator))
	})
	for _, d := range dirs {
		if des, err := os.ReadDir(d); err == nil && len(des) == 0 {
			if err := os.Remove(d); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
