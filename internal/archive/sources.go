// This file turns the user's selection in the Pack dialog into the flat list
// of files and folders a packer writes, applying the dialog's switches the way
// Total Commander does: "Pack path names" off stores bare file names,
// "Recursively pack subdirectories" off packs only the files directly inside
// selected folders, an exclude mask drops matching names, and symbolic links
// are either followed (the default, like Total Commander on Windows) or
// stored as links. Following links can create loops (a link pointing at its
// own parent), so folders already on the current walk path are skipped. The
// archive being written is skipped too, otherwise packing a folder into an
// archive inside that same folder would try to pack the archive into itself.
// RemoveSources implements "Move to archive" once the packer has verified the
// result. All formats share this code so the options behave identically.
package archive

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"
)

// SourceFile is one file, folder or link to be written into an archive.
type SourceFile struct {
	// ArchivePath is the clean slash path inside the archive.
	ArchivePath string
	// LocalPath is the file on disk, or "" for in-memory content.
	LocalPath string
	IsDir     bool
	IsLink    bool
	// LinkTarget is the stored target when IsLink is true.
	LinkTarget string
	Size       int64
	ModTime    time.Time
	Mode       fs.FileMode
	DOSAttrs   uint8
	// Open returns the content; nil for folders and links.
	Open func() (io.ReadCloser, error)
	// Item is the index into Changes.Add this file came from.
	Item int
}

// CollectSources expands the add items into individual files and folders.
// skipPaths lists local files that must never be packed (normally the
// archive being written and its temporary file).
func CollectSources(items []AddItem, opts PackOptions, skipPaths ...string) ([]SourceFile, error) {
	c := &collector{opts: opts, exclude: ParseMask(opts.ExcludeMask)}
	for _, p := range skipPaths {
		if fi, err := os.Stat(p); err == nil {
			c.skip = append(c.skip, fi)
		}
	}
	for i, it := range items {
		if err := c.addItem(i, it); err != nil {
			return c.out, err
		}
	}
	return c.out, nil
}

type collector struct {
	opts    PackOptions
	exclude Mask
	skip    []fs.FileInfo
	out     []SourceFile
}

func (c *collector) skipped(fi fs.FileInfo) bool {
	for _, s := range c.skip {
		if os.SameFile(s, fi) {
			return true
		}
	}
	return false
}

func (c *collector) addItem(i int, it AddItem) error {
	if it.Open != nil || it.Source == "" {
		dest, _ := CleanPath(it.Dest)
		if dest == "" {
			return fmt.Errorf("archive: in-memory item %d has no destination path", i)
		}
		if c.opts.FlattenPaths {
			dest = path.Base(dest)
		}
		mt := it.ModTime
		if mt.IsZero() {
			mt = time.Now()
		}
		sf := SourceFile{ArchivePath: dest, Size: it.Size, ModTime: mt, Mode: it.Mode, Item: i, Open: it.Open}
		if it.Mode.IsDir() {
			sf.IsDir, sf.Open = true, nil
			if sf.Mode.Perm() == 0 {
				sf.Mode |= 0o755
			}
		} else if sf.Mode.Perm() == 0 {
			sf.Mode |= 0o644
		}
		if it.Open == nil && !sf.IsDir {
			return fmt.Errorf("archive: item %d has neither Source nor Open", i)
		}
		c.out = append(c.out, sf)
		return nil
	}
	dest := it.Dest
	if dest == "" {
		dest = filepath.Base(it.Source)
	}
	dest, _ = CleanPath(dest)
	if dest == "" {
		return fmt.Errorf("archive: cannot pack %s: no name", it.Source)
	}
	fi, err := c.stat(it.Source)
	if err != nil {
		return err
	}
	if c.skipped(fi) {
		return nil
	}
	if fi.IsDir() {
		var stack []string
		if r, err := filepath.EvalSymlinks(it.Source); err == nil {
			stack = append(stack, r)
		}
		return c.walkDir(i, it.Source, dest, fi, stack, true)
	}
	c.addFile(i, it.Source, dest, fi)
	return nil
}

// stat follows links unless links are stored as links.
func (c *collector) stat(p string) (fs.FileInfo, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&fs.ModeSymlink != 0 && !c.opts.StoreSymlinks {
		if t, err := os.Stat(p); err == nil {
			return t, nil
		}
		// A broken link cannot be followed; store it as a link instead of
		// failing the whole pack.
	}
	return fi, nil
}

func (c *collector) addFile(i int, local, dest string, fi fs.FileInfo) {
	if c.opts.FlattenPaths {
		dest = path.Base(dest)
	}
	sf := SourceFile{
		ArchivePath: dest, LocalPath: local, Size: fi.Size(), ModTime: fi.ModTime(),
		Mode: fi.Mode(), DOSAttrs: localDOSAttrs(fi), Item: i,
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(local)
		if err == nil {
			sf.IsLink, sf.LinkTarget, sf.Size = true, filepath.ToSlash(target), int64(len(target))
			c.out = append(c.out, sf)
		}
		return
	}
	if !fi.Mode().IsRegular() {
		return // devices, sockets and pipes are never packed
	}
	sf.Open = func() (io.ReadCloser, error) { return os.Open(local) }
	c.out = append(c.out, sf)
}

func (c *collector) walkDir(i int, local, dest string, fi fs.FileInfo, stack []string, top bool) error {
	if !c.opts.FlattenPaths {
		c.out = append(c.out, SourceFile{
			ArchivePath: dest, LocalPath: local, IsDir: true, ModTime: fi.ModTime(),
			Mode: fi.Mode(), DOSAttrs: localDOSAttrs(fi), Item: i,
		})
	}
	if !top && c.opts.NoRecurse {
		return nil
	}
	ents, err := os.ReadDir(local)
	if err != nil {
		return err
	}
	sort.Slice(ents, func(a, b int) bool { return ents[a].Name() < ents[b].Name() })
	for _, de := range ents {
		name := de.Name()
		if c.excluded(name) {
			continue
		}
		childLocal := filepath.Join(local, name)
		childDest := dest + "/" + name
		cfi, err := c.stat(childLocal)
		if err != nil {
			return err
		}
		if c.skipped(cfi) {
			continue
		}
		if cfi.IsDir() {
			if c.opts.NoRecurse {
				continue
			}
			real, err := filepath.EvalSymlinks(childLocal)
			if err != nil {
				return err
			}
			loop := false
			for _, s := range stack {
				if samePath(s, real) {
					loop = true
					break
				}
			}
			if loop {
				continue
			}
			if err := c.walkDir(i, childLocal, childDest, cfi, append(stack, real), false); err != nil {
				return err
			}
			continue
		}
		c.addFile(i, childLocal, childDest, cfi)
	}
	return nil
}

// excluded applies the exclude mask: a name is excluded when it matches the
// mask's include patterns and is not rescued by its exclude patterns.
func (c *collector) excluded(name string) bool {
	if len(c.exclude.include) == 0 {
		return false
	}
	return c.exclude.Match(name)
}

// RemoveSources deletes the local files and folders that were packed, for
// "Move to archive". Files go first; folders are removed afterwards, deepest
// first, and only when they ended up empty, so anything the exclude mask kept
// out of the archive also stays on disk. It returns the removed paths.
func RemoveSources(files []SourceFile) ([]string, error) {
	var removed []string
	var dirs []string
	for _, f := range files {
		if f.LocalPath == "" {
			continue
		}
		if f.IsDir {
			dirs = append(dirs, f.LocalPath)
			continue
		}
		if err := os.Remove(f.LocalPath); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed = append(removed, f.LocalPath)
	}
	sort.Slice(dirs, func(a, b int) bool { return len(dirs[a]) > len(dirs[b]) })
	for _, d := range dirs {
		if os.Remove(d) == nil {
			removed = append(removed, d)
		}
	}
	return removed, nil
}
