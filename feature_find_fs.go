package main

// Where Find files looks. On the local disk the engine reads the disk itself (find.OSFS, with every attribute the
// system knows). Remote folders (ftp://, sftp://, any registered scheme; F-545) and the demo's fictional files go
// through the panels' own file systems instead: findVFS makes a vfs.FS look like the engine's find.FS, so the same
// search works on whatever a panel can show. Looking inside archives (F-528) uses vfs.ArchiveFS, which reads every
// archive format the panels can open (zip family, tar family, 7z, RAR).

import (
	"context"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mormicommander/internal/find"
	"mormicommander/internal/vfs"
)

// findVFS is find.FS over the panels' file systems: URLs go to the FS registered for their scheme, plain paths to the
// disk (or to the demo's files in a demo run).
type findVFS struct {
	def  vfs.FS // the app's file system for plain paths
	demo bool   // plain paths are the demo's, not the disk's
}

func (v *findVFS) local(p string) bool { return !v.demo && !strings.Contains(p, "://") }

func (v *findVFS) fsFor(p string) vfs.FS { return vfs.For(p, v.def) }

func (v *findVFS) Lstat(p string) (fs.FileInfo, error) {
	if v.local(p) {
		return find.OSFS{}.Lstat(p)
	}
	e, err := v.fsFor(p).Stat(p)
	if err != nil {
		return nil, err
	}
	return findInfo{e: e, name: v.Base(p), lstat: true}, nil
}

func (v *findVFS) Stat(p string) (fs.FileInfo, error) {
	if v.local(p) {
		return find.OSFS{}.Stat(p)
	}
	e, err := v.fsFor(p).Stat(p)
	if err != nil {
		return nil, err
	}
	return findInfo{e: e, name: v.Base(p)}, nil
}

func (v *findVFS) ReadDir(p string) ([]fs.DirEntry, error) {
	if v.local(p) {
		return find.OSFS{}.ReadDir(p)
	}
	l, err := v.fsFor(p).List(context.Background(), p)
	if err != nil {
		return nil, err
	}
	out := make([]fs.DirEntry, 0, len(l.Entries))
	for _, e := range l.Entries {
		out = append(out, fs.FileInfoToDirEntry(findInfo{e: e, name: e.Name, lstat: true}))
	}
	return out, nil
}

func (v *findVFS) Open(p string) (io.ReadCloser, error) {
	if v.local(p) {
		return find.OSFS{}.Open(p)
	}
	return vfs.OpenFile(context.Background(), v.fsFor(p), p)
}

func (v *findVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	if v.local(elem[0]) {
		return filepath.Join(elem...)
	}
	out := elem[0]
	f := v.fsFor(out)
	for _, e := range elem[1:] {
		out = f.Join(out, e)
	}
	return out
}

func (v *findVFS) Base(p string) string {
	if v.local(p) {
		return filepath.Base(p)
	}
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// findInfo is a vfs.Entry as an fs.FileInfo. lstat: a link reads as a link (not followed), as Lstat answers.
type findInfo struct {
	e     vfs.Entry
	name  string
	lstat bool
}

func (i findInfo) Name() string       { return i.name }
func (i findInfo) Size() int64        { return i.e.Size }
func (i findInfo) ModTime() time.Time { return time.UnixMilli(i.e.MTime) }
func (i findInfo) Sys() any           { return nil }
func (i findInfo) IsDir() bool        { return i.Mode().IsDir() }
func (i findInfo) Mode() fs.FileMode {
	m := fs.FileMode(i.e.Mode).Perm()
	switch {
	case i.lstat && i.e.Flags&vfs.FlagLink != 0:
		m |= fs.ModeSymlink
	case i.e.Flags&vfs.FlagDir != 0:
		m |= fs.ModeDir
	}
	return m
}

// findArchives lets the engine look inside archives through vfs.ArchiveFS.
type findArchives struct{ arc *vfs.ArchiveFS }

// IsArchive: local files with an archive extension (archives on servers would need a download each; TC skips them too).
func (a findArchives) IsArchive(p string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() || strings.Contains(p, "://") {
		return false
	}
	return a.arc.IsArchive(filepath.Base(p))
}

// Walk hands every member of the archive to fn, going down into its folders.
func (a findArchives) Walk(ctx context.Context, _ find.FS, p string, fn func(e find.ArchiveEntry, open func() (io.ReadCloser, error)) error) error {
	var walk func(dir, inner string, depth int) error
	walk = func(dir, inner string, depth int) error {
		if depth > 64 {
			return nil
		}
		l, err := a.arc.List(ctx, dir)
		if err != nil {
			return err
		}
		for _, e := range l.Entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			full := a.arc.Join(dir, e.Name)
			name := path.Join(inner, e.Name)
			isDir := e.Flags&vfs.FlagDir != 0
			ae := find.ArchiveEntry{Name: name, Size: e.Size, ModTime: time.UnixMilli(e.MTime), IsDir: isDir, Mode: fs.FileMode(e.Mode).Perm()}
			open := func() (io.ReadCloser, error) { return a.arc.Open(ctx, full) }
			if err := fn(ae, open); err != nil {
				return err
			}
			if isDir {
				if err := walk(full, name, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(p, "", 0)
}
