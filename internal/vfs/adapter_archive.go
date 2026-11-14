// This file shows archives as folders. In Total Commander, pressing Enter on
// a zip, tar, 7z or RAR file opens it in the panel like a directory: the path
// bar reads C:\data\backup.zip\docs\2024, Backspace climbs back out, F3 views
// a packed file, F5 copies out of it, and for formats that can be written F5
// into it, F7, F8 and Shift+F6 add, create, delete and rename entries.
// ArchiveFS gives the panels exactly that through the ordinary FS interface
// plus the optional interfaces of adapter_ops.go.
//
// A path is an archive path when, walking up from it, the first component
// that exists on disk is a regular file that an archive engine can read; the
// rest of the path is the location inside the archive. Archives inside
// archives work the same way (backup.zip\old\src.tar.gz\main.go): the inner
// archive is unpacked to a temporary file once and then browsed like any
// other, read-only, as in TC. The router also "mounts" archives that live on
// FTP or SFTP servers after downloading them, so the same code serves them.
//
// Opened archives are cached, because listing a big tar.gz means
// decompressing all of it; the cache notices when the file changes on disk
// and reopens it. Every write goes through the engine's atomic rewrite (temp
// file plus rename), so a failed or cancelled change never damages the
// archive.
package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/archive/arcapi"
)

// DefaultEnterExtensions are the file name endings Enter treats as archives
// (lower case, without the dot), matching TC's built-in packers. Office and
// Java containers (.docx, .jar) open with their application on Enter but can
// still be browsed by giving their path explicitly (TC's Ctrl+PgDn).
var DefaultEnterExtensions = []string{
	"zip", "zipx", "cbz", "z01", "zip.001",
	"7z", "7z.001", "cb7", "rar", "cbr", "r00",
	"tar", "tgz", "taz", "tbz", "tbz2", "tb2", "txz", "tzst", "tlz",
	"tar.gz", "tar.bz2", "tar.xz", "tar.zst", "tar.lzma",
	"gz", "bz2", "xz", "zst", "lzma",
}

// ArchiveInfo describes the archive a path is in.
type ArchiveInfo struct {
	Format   string // engine format name: "zip", "tar.gz", "7z", "rar", ...
	Root     string // the panel path of the archive itself
	File     string // the local file that is read (a temp copy for nested or remote archives)
	Inner    string // slash path inside the archive, "" at its root
	ReadOnly bool   // writes are refused at this location
	Nested   bool   // the archive lives inside another archive or on a server
}

// ArchiveFS is the file system of archive contents. The zero value is not
// usable; create one with NewArchiveFS. It is safe for concurrent use.
type ArchiveFS struct {
	// Password is asked when an encrypted archive or entry needs a password;
	// ok=false means the user cancelled. Nil means encrypted data fails with
	// a "password required" error.
	Password func(req PasswordRequest) (password string, ok bool)
	// Encoding is the character set for names in old zips that are not
	// marked as UTF-8: "" or "auto", "cp437", "cp852", "shift_jis", ...
	Encoding string
	// EnterExtensions overrides DefaultEnterExtensions for IsArchive.
	EnterExtensions []string
	// MaxOpen bounds how many archives stay open in the cache (default 8).
	MaxOpen int
	// TempDir is where nested archives and files being packed are staged;
	// default a private folder below os.TempDir, removed by Close.
	TempDir string

	mu      sync.Mutex
	handles map[string]*arcHandle // by local file
	lru     []string
	mounts  map[string]*arcMount // by panel path prefix
	tmp     string
	seq     int
	// writeLocks holds one lock per archive file being changed.
	writeLocks map[string]*writeLock
}

// arcHandle is one opened archive.
type arcHandle struct {
	file     string
	display  string
	size     int64
	mtime    time.Time
	be       arcBackend
	readOnly bool
	nested   bool
	refs     int
	stale    bool
}

// arcMount maps a panel path prefix to a local file: nested archives and
// archives downloaded from a server.
type arcMount struct {
	file     string
	readOnly bool
	cleanup  func()
	// For nested archives: the outer archive file and its state when the
	// inner one was unpacked; a change of the outer file voids the mount.
	parent      string
	parentSize  int64
	parentMTime time.Time
}

// NewArchiveFS returns an empty archive file system.
func NewArchiveFS() *ArchiveFS {
	return &ArchiveFS{handles: map[string]*arcHandle{}, mounts: map[string]*arcMount{}}
}

// IsArchive reports whether a file name has an archive extension, which is
// what Enter on a file in the panel checks.
func (a *ArchiveFS) IsArchive(name string) bool {
	exts := a.EnterExtensions
	if exts == nil {
		exts = DefaultEnterExtensions
	}
	l := strings.ToLower(name)
	for _, e := range exts {
		if strings.HasSuffix(l, "."+e) && len(l) > len(e)+1 {
			return true
		}
	}
	// Numbered parts of split sets: x.part2.rar, x.r07, x.z05, x.7z.003.
	if m := strings.LastIndexByte(l, '.'); m > 0 {
		ext := l[m+1:]
		if (len(ext) == 3 && (ext[0] == 'r' || ext[0] == 'z') && isDigits(ext[1:])) ||
			(len(ext) == 3 && isDigits(ext) && (strings.HasSuffix(l[:m], ".7z") || strings.HasSuffix(l[:m], ".zip"))) {
			return true
		}
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ---- panel path helpers ---------------------------------------------------

// hasScheme reports whether p is a URL ("ftp://...") rather than a local
// path. A Windows drive letter is never a scheme.
func hasScheme(p string) bool {
	i := strings.Index(p, "://")
	return i > 1 && !strings.ContainsAny(p[:i], `/\:`)
}

// sepOf is the separator used in a panel path: "/" in URLs, the OS separator
// in local paths.
func sepOf(p string) string {
	if hasScheme(p) {
		return "/"
	}
	return string(os.PathSeparator)
}

// splitRest splits the part of a panel path below a prefix into slash
// components.
func splitRest(rest, sep string) []string {
	var out []string
	for _, c := range strings.Split(rest, sep) {
		if sep != "/" {
			for _, cc := range strings.Split(c, "/") {
				if cc != "" {
					out = append(out, cc)
				}
			}
			continue
		}
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

// displayJoin appends the slash path inner to the panel path root.
func displayJoin(root, inner string) string {
	if inner == "" {
		return root
	}
	sep := sepOf(root)
	if strings.HasSuffix(root, sep) {
		return root + strings.ReplaceAll(inner, "/", sep)
	}
	return root + sep + strings.ReplaceAll(inner, "/", sep)
}

// underPrefix reports whether p is prefix or below it, returning the rest.
func underPrefix(p, prefix string) (string, bool) {
	if p == prefix {
		return "", true
	}
	sep := sepOf(prefix)
	pre := prefix
	if !strings.HasSuffix(pre, sep) {
		pre += sep
	}
	if strings.HasPrefix(p, pre) {
		return p[len(pre):], true
	}
	return "", false
}

// Clean makes a panel path canonical: local paths as Local.Clean does, URLs
// with a cleaned slash path.
func (a *ArchiveFS) Clean(p string) string {
	if hasScheme(p) {
		return cleanURL(p)
	}
	return Local{}.Clean(p)
}

// Parent is the folder above p. The parent of an archive's root is the
// folder holding the archive file.
func (a *ArchiveFS) Parent(p string) string {
	p = a.Clean(p)
	if hasScheme(p) {
		return urlParent(p)
	}
	return localParent(p)
}

// Join puts name under dir.
func (a *ArchiveFS) Join(dir, name string) string {
	if hasScheme(dir) {
		return urlJoin(dir, name)
	}
	return filepath.Join(dir, name)
}

// Root is the archive itself: Ctrl+\ inside an archive goes to its top.
func (a *ArchiveFS) Root(p string) string {
	if root, _, ok := a.Split(p); ok {
		return root
	}
	if hasScheme(p) {
		return urlRoot(p)
	}
	return Local{}.Root(p)
}

// ---- locating the archive -------------------------------------------------

// Split tells whether p is inside an archive without opening it: root is
// the panel path of the innermost archive already known to contain p (an
// archive file on disk, or a nested or server archive that was entered) and
// inner the slash path below it ("" when p is that archive file itself).
func (a *ArchiveFS) Split(p string) (root, inner string, ok bool) {
	p = a.Clean(p)
	m, prefix, rest := a.findMount(p, true)
	if m != nil {
		return prefix, strings.Join(rest, "/"), true
	}
	file, rest2, err := findLocalArchive(p)
	if err != nil {
		return "", "", false
	}
	return file, strings.Join(rest2, "/"), true
}

// findMount returns the mount with the longest prefix that contains p, and
// the components of p below it. Stale nested mounts are dropped. With
// skipExactNested, a path naming a nested archive itself is treated as the
// file inside the outer archive rather than as the nested archive's root.
func (a *ArchiveFS) findMount(p string, skipExactNested bool) (*arcMount, string, []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for {
		best := ""
		for prefix, m := range a.mounts {
			if skipExactNested && m.parent != "" && p == prefix {
				continue // the nested archive file itself, not its contents
			}
			if _, ok := underPrefix(p, prefix); ok && len(prefix) > len(best) {
				best = prefix
			}
		}
		if best == "" {
			return nil, "", nil
		}
		m := a.mounts[best]
		if m.parent != "" {
			st, err := os.Stat(m.parent)
			if err != nil || st.Size() != m.parentSize || !st.ModTime().Equal(m.parentMTime) {
				a.dropMountLocked(best)
				continue
			}
		}
		rest, _ := underPrefix(p, best)
		return m, best, splitRest(rest, sepOf(best))
	}
}

// findLocalArchive walks up from the local path p to the first component
// that exists. When that is a regular file an engine can read, it is the
// archive and the remaining components are the path inside it.
func findLocalArchive(p string) (string, []string, error) {
	if hasScheme(p) {
		return "", nil, errNotArchive
	}
	cur := p
	var rest []string
	for {
		fi, err := os.Stat(cur)
		if err == nil {
			if fi.IsDir() {
				return "", nil, errNotArchive
			}
			if !fi.Mode().IsRegular() || arcKind(cur) == "" {
				if len(rest) > 0 {
					return "", nil, &fs.PathError{Op: "open", Path: p, Err: ErrNotFolder}
				}
				return "", nil, errNotArchive
			}
			return cur, rest, nil
		}
		parent := localParent(cur)
		if parent == "" || parent == cur {
			return "", nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
		cur = parent
	}
}

// findHost resolves p to the local file to open, its panel root, the
// components inside it, and whether it is read-only.
func (a *ArchiveFS) findHost(p string, mode locateMode) (file, display string, rest []string, ro, nested bool, err error) {
	if m, prefix, r := a.findMount(p, mode != locDir); m != nil {
		return m.file, prefix, r, m.readOnly, true, nil
	}
	file, rest, err = findLocalArchive(p)
	if err != nil {
		return "", "", nil, false, false, err
	}
	return file, file, rest, false, false, nil
}

// acquire returns the cached handle for file, opening or reopening it.
// The caller must release it.
func (a *ArchiveFS) acquire(file, display string, ro, nested bool) (*arcHandle, error) {
	st, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	if h := a.handles[file]; h != nil {
		if !h.stale && h.size == st.Size() && h.mtime.Equal(st.ModTime()) {
			h.refs++
			a.touchLocked(file)
			a.mu.Unlock()
			return h, nil
		}
		a.dropHandleLocked(h)
	}
	a.mu.Unlock()

	// Opening may ask for a password, so it runs without the lock.
	be, err := openBackend(file, openOptions{password: a.Password, encoding: a.Encoding})
	if err != nil {
		return nil, err
	}
	h := &arcHandle{file: file, display: display, size: st.Size(), mtime: st.ModTime(), be: be, readOnly: ro, nested: nested, refs: 1}
	a.mu.Lock()
	defer a.mu.Unlock()
	if old := a.handles[file]; old != nil && !old.stale {
		// Another goroutine opened it meanwhile; use theirs.
		be.close()
		old.refs++
		return old, nil
	}
	a.handles[file] = h
	a.touchLocked(file)
	a.evictLocked()
	return h, nil
}

// closeIdleArchives makes the cache close an archive as soon as nothing uses
// it. Windows cannot rename, delete or replace a file while any process has
// it open, so a cached handle would keep the user (and other programs) from
// renaming, deleting or updating an archive that was merely browsed. There
// the archive is reopened on the next access instead; elsewhere an open file
// does not block anything and the cache keeps it.
var closeIdleArchives = runtime.GOOS == "windows"

// release gives a handle back; stale handles close when unused.
func (a *ArchiveFS) release(h *arcHandle) {
	a.mu.Lock()
	defer a.mu.Unlock()
	h.refs--
	if h.refs > 0 {
		return
	}
	if h.stale {
		h.be.close()
	} else if closeIdleArchives {
		a.dropHandleLocked(h) // closes it, since it is unused
	}
}

func (a *ArchiveFS) touchLocked(file string) {
	for i, f := range a.lru {
		if f == file {
			a.lru = append(a.lru[:i], a.lru[i+1:]...)
			break
		}
	}
	a.lru = append(a.lru, file)
}

// dropHandleLocked removes h from the cache; it closes once unused.
func (a *ArchiveFS) dropHandleLocked(h *arcHandle) {
	if a.handles[h.file] == h {
		delete(a.handles, h.file)
	}
	for i, f := range a.lru {
		if f == h.file {
			a.lru = append(a.lru[:i], a.lru[i+1:]...)
			break
		}
	}
	if !h.stale {
		h.stale = true
		if h.refs <= 0 {
			h.be.close()
		}
	}
}

func (a *ArchiveFS) evictLocked() {
	limit := a.MaxOpen
	if limit <= 0 {
		limit = 8
	}
	for len(a.lru) > limit {
		h := a.handles[a.lru[0]]
		if h == nil {
			a.lru = a.lru[1:]
			continue
		}
		a.dropHandleLocked(h)
	}
}

func (a *ArchiveFS) dropMountLocked(prefix string) {
	m := a.mounts[prefix]
	if m == nil {
		return
	}
	delete(a.mounts, prefix)
	if h := a.handles[m.file]; h != nil {
		a.dropHandleLocked(h)
	}
	if m.cleanup != nil {
		m.cleanup()
	}
}

// Mount makes the local archive file appear at the panel path prefix. The
// router uses it for archives downloaded from FTP or SFTP servers; cleanup
// (may be nil) runs when the mount is dropped. An existing mount at prefix
// is replaced.
func (a *ArchiveFS) Mount(prefix, file string, readOnly bool, cleanup func()) {
	prefix = a.Clean(prefix)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dropMountLocked(prefix)
	a.mounts[prefix] = &arcMount{file: file, readOnly: readOnly, cleanup: cleanup}
}

// Unmount drops the mount at prefix and every mount below it.
func (a *ArchiveFS) Unmount(prefix string) {
	prefix = a.Clean(prefix)
	a.mu.Lock()
	defer a.mu.Unlock()
	for p := range a.mounts {
		if _, ok := underPrefix(p, prefix); ok {
			a.dropMountLocked(p)
		}
	}
}

// Mounted reports whether prefix is a mount point.
func (a *ArchiveFS) Mounted(prefix string) bool {
	prefix = a.Clean(prefix)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mounts[prefix] != nil
}

// tempDir returns the private staging folder, creating it on first use.
func (a *ArchiveFS) tempDir() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tmp != "" {
		return a.tmp, nil
	}
	d, err := os.MkdirTemp(a.TempDir, "mormi-vfs-*")
	if err != nil {
		return "", err
	}
	a.tmp = d
	return d, nil
}

// newTempPath returns a fresh path named name in its own temp subfolder.
func (a *ArchiveFS) newTempPath(name string) (string, error) {
	d, err := a.tempDir()
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.seq++
	n := a.seq
	a.mu.Unlock()
	sub := filepath.Join(d, fmt.Sprint(n))
	if err := os.Mkdir(sub, 0o700); err != nil {
		return "", err
	}
	// only a plain name goes in: a server name with separators or ".." must not lead out of the private folder,
	// whose path callers take as filepath.Dir of the result
	name = filepath.Base(filepath.FromSlash(name))
	if name == "." || name == ".." || name == string(filepath.Separator) {
		name = "file"
	}
	return filepath.Join(sub, name), nil
}

// locateMode says what the caller of locate needs.
type locateMode int

const (
	locFile    locateMode = iota // the path must exist
	locDir                       // the path must be a folder (nested archives are entered)
	locMissing                   // the path may be missing, its parents must be folders
)

// maxNesting bounds archives inside archives.
const maxNesting = 8

// locate opens the archive holding p and returns the acquired handle and
// the slash path inside it. Archive files inside the archive are unpacked
// and entered when the path goes through them (or, in locDir mode, ends at
// one).
func (a *ArchiveFS) locate(ctx context.Context, p string, mode locateMode) (*arcHandle, string, error) {
	p = a.Clean(p)
	file, display, rest, ro, nested, err := a.findHost(p, mode)
	if err != nil {
		return nil, "", err
	}
	h, err := a.acquire(file, display, ro, nested)
	if err != nil {
		return nil, "", err
	}
	for depth := 0; ; depth++ {
		inner := strings.Join(rest, "/")
		e, err := h.be.stat(ctx, inner)
		if err == nil {
			if e.IsDir || mode != locDir {
				return h, inner, nil
			}
			if depth >= maxNesting {
				a.release(h)
				return nil, "", &fs.PathError{Op: "open", Path: p, Err: ErrNotFolder}
			}
			nh, err := a.nest(ctx, h, inner)
			a.release(h)
			if err != nil {
				return nil, "", err
			}
			h, rest = nh, nil
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			a.release(h)
			return nil, "", err
		}
		// The path is not an entry: look for a file on the way that is
		// itself an archive.
		found := false
		for i := 1; i < len(rest); i++ {
			sub := strings.Join(rest[:i], "/")
			se, serr := h.be.stat(ctx, sub)
			if serr != nil {
				break
			}
			if se.IsDir {
				continue
			}
			if depth >= maxNesting {
				break
			}
			nh, nerr := a.nest(ctx, h, sub)
			if nerr != nil {
				a.release(h)
				if errors.Is(nerr, errNotArchive) {
					return nil, "", &fs.PathError{Op: "open", Path: p, Err: ErrNotFolder}
				}
				return nil, "", nerr
			}
			a.release(h)
			h, rest, found = nh, rest[i:], true
			break
		}
		if found {
			continue
		}
		if mode == locMissing {
			if parentOK(ctx, h.be, rest) {
				return h, inner, nil
			}
		}
		a.release(h)
		return nil, "", &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
}

// parentOK reports whether every existing ancestor of rest is a folder (and
// the missing part is only at the end of the chain).
func parentOK(ctx context.Context, be arcBackend, rest []string) bool {
	for i := len(rest) - 1; i >= 0; i-- {
		e, err := be.stat(ctx, strings.Join(rest[:i], "/"))
		if err == nil {
			return e.IsDir
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}

// nest unpacks the archive entry inner of h to a temp file (once), mounts it
// below h's panel path and returns its acquired handle.
func (a *ArchiveFS) nest(ctx context.Context, h *arcHandle, inner string) (*arcHandle, error) {
	prefix := displayJoin(h.display, inner)
	a.mu.Lock()
	m := a.mounts[prefix]
	a.mu.Unlock()
	if m == nil {
		tmp, err := a.newTempPath(path.Base(inner))
		if err != nil {
			return nil, err
		}
		rc, err := h.be.open(ctx, inner)
		if err != nil {
			return nil, err
		}
		_, err = writeLocalAtomic(ctx, tmp, rc, -1, time.Time{}, nil)
		rc.Close()
		if err == nil && arcKind(tmp) == "" {
			err = fmt.Errorf("%s: %w", prefix, errNotArchive)
		}
		if err != nil {
			os.RemoveAll(filepath.Dir(tmp))
			return nil, err
		}
		m = &arcMount{file: tmp, readOnly: true, cleanup: func() { os.RemoveAll(filepath.Dir(tmp)) },
			parent: h.file, parentSize: h.size, parentMTime: h.mtime}
		a.mu.Lock()
		if old := a.mounts[prefix]; old != nil {
			a.mu.Unlock()
			m.cleanup()
			m = old
		} else {
			a.mounts[prefix] = m
			a.mu.Unlock()
		}
	}
	return a.acquire(m.file, prefix, true, true)
}

// ---- reading ---------------------------------------------------------------

// entryOf converts an archive member to a panel entry.
func entryOf(e arcEntry) Entry {
	return modeEntry(e.Name, e.Size, e.ModTime, e.Mode, e.IsDir, e.IsLink, false, true,
		e.Hidden, e.System, e.ReadOnly)
}

// modeEntry builds a panel entry from portable facts. hasMode says whether
// mode carries real permission bits (FTP servers may not report any).
func modeEntry(name string, size int64, mt time.Time, mode fs.FileMode, isDir, isLink, broken, hasMode, hidden, system, readOnly bool) Entry {
	e := Entry{Name: name, Mode: uint32(mode)}
	if !mt.IsZero() {
		e.MTime = mt.UnixMilli()
	}
	if isDir {
		e.Flags |= FlagDir
	} else if size > 0 {
		e.Size = size
	}
	if isLink {
		e.Flags |= FlagLink
	}
	if broken {
		e.Flags |= FlagBroken
	}
	if hidden || strings.HasPrefix(name, ".") {
		e.Flags |= FlagHidden
	}
	if system {
		e.Flags |= FlagSystem
	}
	if readOnly || (hasMode && mode.Perm() != 0 && mode.Perm()&0o222 == 0) {
		e.Flags |= FlagReadOnly
	}
	if !isDir {
		if hasMode && mode.Perm()&0o111 != 0 {
			e.Flags |= FlagExec
		} else if _, ext := SplitExt(strings.ToLower(name)); ext == "exe" || ext == "bat" || ext == "cmd" || ext == "com" {
			e.Flags |= FlagExec
		}
	}
	return e
}

// List reads a folder inside an archive. Listing the archive file itself
// shows its root.
func (a *ArchiveFS) List(ctx context.Context, p string) (Listing, error) {
	p = a.Clean(p)
	h, inner, err := a.locate(ctx, p, locDir)
	if errors.Is(err, errNotArchive) {
		return Listing{}, &fs.PathError{Op: "list", Path: p, Err: ErrNotFolder}
	}
	if err != nil {
		return Listing{}, err
	}
	defer a.release(h)
	es, err := h.be.readDir(ctx, inner)
	if err != nil {
		return Listing{}, err
	}
	out := make([]Entry, len(es))
	for i, e := range es {
		out[i] = entryOf(e)
	}
	if err := ctx.Err(); err != nil {
		return Listing{}, err
	}
	return Listing{Path: p, Parent: a.Parent(p), Entries: out}, nil
}

// Stat describes a path inside an archive. The archive root is a folder
// named like the archive file.
func (a *ArchiveFS) Stat(p string) (Entry, error) {
	p = a.Clean(p)
	h, inner, err := a.locate(context.Background(), p, locFile)
	if err != nil {
		return Entry{}, err
	}
	defer a.release(h)
	if inner == "" {
		name := path.Base(strings.ReplaceAll(h.display, `\`, "/"))
		return Entry{Name: name, Flags: FlagDir, MTime: h.mtime.UnixMilli(), Mode: uint32(fs.ModeDir | 0o755)}, nil
	}
	e, err := h.be.stat(context.Background(), inner)
	if err != nil {
		return Entry{}, err
	}
	return entryOf(e), nil
}

// Info describes the archive p is in.
func (a *ArchiveFS) Info(p string) (ArchiveInfo, error) {
	h, inner, err := a.locate(context.Background(), p, locMissing)
	if err != nil {
		return ArchiveInfo{}, err
	}
	defer a.release(h)
	return ArchiveInfo{Format: h.be.format(), Root: h.display, File: h.file, Inner: inner,
		ReadOnly: h.readOnly || !h.be.writable(), Nested: h.nested}, nil
}

// FolderInfo describes the archive folder a panel shows at p. It differs
// from Info only for a path that names an archive which was entered from
// inside another archive (or from a server): Info sees that path as the file
// in the outer archive, FolderInfo as the root of the inner archive, which is
// what the panel shows there. It never unpacks anything: an inner archive
// that has not been entered yet is reported as the outer archive's file.
func (a *ArchiveFS) FolderInfo(p string) (ArchiveInfo, error) {
	p = a.Clean(p)
	a.mu.Lock()
	m := a.mounts[p]
	a.mu.Unlock()
	if m == nil || m.parent == "" {
		return a.Info(p)
	}
	h, inner, err := a.locate(context.Background(), p, locDir)
	if err != nil {
		return ArchiveInfo{}, err
	}
	defer a.release(h)
	return ArchiveInfo{Format: h.be.format(), Root: h.display, File: h.file, Inner: inner,
		ReadOnly: h.readOnly || !h.be.writable(), Nested: h.nested}, nil
}

// arcReader keeps the archive open while a member is being read.
type arcReader struct {
	io.ReadCloser
	a    *ArchiveFS
	h    *arcHandle
	once sync.Once
}

func (r *arcReader) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(func() { r.a.release(r.h) })
	return err
}

// Open streams a file out of an archive (F3, copy out, nested archives).
func (a *ArchiveFS) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	h, inner, err := a.locate(ctx, p, locFile)
	if err != nil {
		return nil, err
	}
	if inner == "" {
		a.release(h)
		return nil, opErr("open", p, ErrIsFolder)
	}
	rc, err := h.be.open(ctx, inner)
	if err != nil {
		a.release(h)
		return nil, opErr("open", p, err)
	}
	return &arcReader{ReadCloser: rc, a: a, h: h}, nil
}

// Download unpacks one file to a local path (temp file plus rename).
func (a *ArchiveFS) Download(ctx context.Context, p, local string, opt TransferOptions) (int64, error) {
	e, err := a.Stat(p)
	if err != nil {
		return 0, err
	}
	if e.Flags&FlagDir != 0 {
		return 0, opErr("download", p, ErrIsFolder)
	}
	rc, err := a.Open(ctx, p)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	var mt time.Time
	if opt.PreserveTime && e.MTime != 0 {
		mt = time.UnixMilli(e.MTime)
	}
	return writeLocalAtomic(ctx, local, rc, e.Size, mt, opt.Progress)
}

// Caps reports what works at p.
func (a *ArchiveFS) Caps(p string) Caps {
	h, _, err := a.locate(context.Background(), p, locMissing)
	if err != nil {
		return Caps{}
	}
	defer a.release(h)
	w := !h.readOnly && h.be.writable()
	return Caps{Read: true, Write: w, Mkdir: w, Delete: w, Rename: w, Archive: true}
}

// ---- writing ---------------------------------------------------------------

// modify applies changes built for the archive holding p. Every change
// rewrites the whole archive (read the old one, write a temp file, rename
// it over the original), so two writers to the same archive at once would
// each start from the old content and the last rename would silently drop
// the other's change. Writers therefore hold a per-archive lock from
// reading the current state (build) until the new file is in place.
func (a *ArchiveFS) modify(ctx context.Context, p string, mode locateMode, build func(h *arcHandle, inner string) (arcChanges, error)) error {
	file, _, _, _, _, err := a.findHost(a.Clean(p), mode)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		unlock := a.lockArchive(file)
		other, err := a.modifyLocked(ctx, p, mode, file, build)
		unlock()
		if other == "" || attempt >= 3 {
			return err
		}
		// p resolved into another archive file than the one locked
		// (a nested archive): lock that one and start over.
		file = other
	}
}

// modifyLocked does the work of modify while the lock for file is held. It
// returns the actual archive file instead when p resolves to another one.
func (a *ArchiveFS) modifyLocked(ctx context.Context, p string, mode locateMode, file string, build func(h *arcHandle, inner string) (arcChanges, error)) (string, error) {
	// Start from the archive as it is now on disk, not from a handle that
	// may have been opened before another writer replaced the file.
	a.dropCachedFile(file)
	h, inner, err := a.locate(ctx, p, mode)
	if err != nil {
		return "", err
	}
	if h.file != file {
		other := h.file
		a.release(h)
		return other, nil
	}
	if !h.be.writable() {
		a.release(h)
		if _, ok := h.be.(*sevenBackend); ok {
			return "", opErr("write", p, ErrUnsupported)
		}
		return "", opErr("write", p, ErrReadOnly)
	}
	if h.readOnly {
		a.release(h)
		if h.nested {
			return "", opErr("write", p, ErrNestedReadOnly)
		}
		return "", opErr("write", p, ErrReadOnly)
	}
	ch, err := build(h, inner)
	if err != nil {
		a.release(h)
		return "", opErr("write", p, err)
	}
	be := h.be
	a.mu.Lock()
	a.dropHandleLocked(h)
	a.mu.Unlock()
	a.release(h)
	err = be.update(ctx, file, ch)
	// A reader may have cached the archive while it was being replaced.
	a.dropCachedFile(file)
	if err != nil {
		return "", opErr("write", p, err)
	}
	return "", nil
}

// dropCachedFile removes the cached handle of file, if any.
func (a *ArchiveFS) dropCachedFile(file string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if h := a.handles[file]; h != nil {
		a.dropHandleLocked(h)
	}
}

// writeLock serialises the writers of one archive file.
type writeLock struct {
	mu   sync.Mutex
	refs int
}

// lockArchive takes the write lock of the archive file and returns the
// function that releases it. Paths that lead to the same file through a
// symbolic link share one lock.
func (a *ArchiveFS) lockArchive(file string) func() {
	key := arcapi.ReplaceTarget(file)
	if abs, err := filepath.Abs(key); err == nil {
		key = abs
	}
	a.mu.Lock()
	if a.writeLocks == nil {
		a.writeLocks = map[string]*writeLock{}
	}
	l := a.writeLocks[key]
	if l == nil {
		l = &writeLock{}
		a.writeLocks[key] = l
	}
	l.refs++
	a.mu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		a.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(a.writeLocks, key)
		}
		a.mu.Unlock()
	}
}

// Mkdir creates a folder inside an archive (F7).
func (a *ArchiveFS) Mkdir(ctx context.Context, p string) error {
	return a.modify(ctx, p, locMissing, func(h *arcHandle, inner string) (arcChanges, error) {
		be := h.be
		if inner == "" {
			return arcChanges{}, fs.ErrExist
		}
		if _, err := be.stat(ctx, inner); err == nil {
			return arcChanges{}, fs.ErrExist
		}
		return arcChanges{Mkdir: []string{inner}}, nil
	})
}

// Remove deletes a file or an empty folder inside an archive.
func (a *ArchiveFS) Remove(ctx context.Context, p string) error {
	return a.modify(ctx, p, locFile, func(h *arcHandle, inner string) (arcChanges, error) {
		be := h.be
		if inner == "" {
			return arcChanges{}, ErrUnsupported
		}
		e, err := be.stat(ctx, inner)
		if err != nil {
			return arcChanges{}, err
		}
		if e.IsDir {
			kids, err := be.readDir(ctx, inner)
			if err != nil {
				return arcChanges{}, err
			}
			if len(kids) > 0 {
				return arcChanges{}, ErrNotEmpty
			}
		}
		return arcChanges{Delete: []string{inner}}, nil
	})
}

// RemoveAll deletes a file or a folder with its contents inside an archive
// (F8). Deleting the archive itself is a job for the local file system.
func (a *ArchiveFS) RemoveAll(ctx context.Context, p string) error {
	return a.modify(ctx, p, locFile, func(h *arcHandle, inner string) (arcChanges, error) {
		if inner == "" {
			return arcChanges{}, ErrUnsupported
		}
		return arcChanges{Delete: []string{inner}}, nil
	})
}

// Rename moves an entry within one archive (Shift+F6). Moving between two
// archives returns ErrCrossFS, and an existing target fs.ErrExist (the UI
// asks and deletes the target first).
func (a *ArchiveFS) Rename(ctx context.Context, from, to string) error {
	ht, innerTo, err := a.locate(ctx, to, locMissing)
	if err != nil {
		return err
	}
	toFile := ht.file
	a.release(ht)
	return a.modify(ctx, from, locFile, func(h *arcHandle, inner string) (arcChanges, error) {
		if h.file != toFile {
			return arcChanges{}, ErrCrossFS
		}
		if inner == "" || innerTo == "" {
			return arcChanges{}, ErrUnsupported
		}
		// Checked here, under the archive's write lock, so a concurrent
		// write cannot create the target in between.
		if _, err := h.be.stat(ctx, innerTo); err == nil {
			return arcChanges{}, fs.ErrExist
		}
		if innerTo == inner || strings.HasPrefix(innerTo, inner+"/") {
			return arcChanges{}, fmt.Errorf("cannot move %s into itself", inner)
		}
		return arcChanges{Rename: [][2]string{{inner, innerTo}}}, nil
	})
}

// Upload packs one local file as p (copy into archive).
func (a *ArchiveFS) Upload(ctx context.Context, local, p string, opt TransferOptions) (int64, error) {
	fi, err := os.Stat(local)
	if err != nil {
		return 0, err
	}
	err = a.modify(ctx, p, locMissing, func(h *arcHandle, inner string) (arcChanges, error) {
		be := h.be
		if inner == "" {
			return arcChanges{}, ErrIsFolder
		}
		if e, err := be.stat(ctx, inner); err == nil && e.IsDir {
			return arcChanges{}, ErrIsFolder
		}
		return arcChanges{Add: []arcAdd{{Source: local, Dest: inner}}}, nil
	})
	if err != nil {
		return 0, err
	}
	if opt.Progress != nil {
		opt.Progress(fi.Size(), fi.Size())
	}
	return fi.Size(), nil
}

// AddLocal packs local files and folders into the archive folder dir in a
// single rewrite (F5 of a selection into an archive panel).
func (a *ArchiveFS) AddLocal(ctx context.Context, dir string, sources []string) error {
	if len(sources) == 0 {
		return nil
	}
	return a.modify(ctx, dir, locMissing, func(h *arcHandle, inner string) (arcChanges, error) {
		be := h.be
		if inner != "" {
			if e, err := be.stat(ctx, inner); err == nil && !e.IsDir {
				return arcChanges{}, ErrNotFolder
			}
		}
		var ch arcChanges
		seen := map[string]bool{}
		for _, s := range sources {
			if _, err := os.Lstat(s); err != nil {
				return arcChanges{}, err
			}
			dest := path.Join(inner, filepath.Base(s))
			if seen[dest] {
				return arcChanges{}, fmt.Errorf("%s: %w", dest, fs.ErrExist)
			}
			seen[dest] = true
			ch.Add = append(ch.Add, arcAdd{Source: s, Dest: dest})
		}
		sort.Slice(ch.Add, func(i, j int) bool { return ch.Add[i].Dest < ch.Add[j].Dest })
		return ch, nil
	})
}

// arcWriter collects data for Create in a temp file and packs it on Close.
type arcWriter struct {
	f      *os.File
	a      *ArchiveFS
	ctx    context.Context
	p      string
	closed bool
}

func (w *arcWriter) Write(b []byte) (int, error) { return w.f.Write(b) }

func (w *arcWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	tmp := w.f.Name()
	defer os.RemoveAll(filepath.Dir(tmp))
	if err := w.f.Close(); err != nil {
		return err
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	_, err := w.a.Upload(w.ctx, tmp, w.p, TransferOptions{})
	return err
}

// Create returns a writer whose content is packed as p when it is closed.
func (a *ArchiveFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	if c := a.Caps(p); !c.Write {
		if c.Read {
			return nil, opErr("create", p, ErrReadOnly)
		}
		return nil, opErr("create", p, fs.ErrNotExist)
	}
	_, inner, ok := a.Split(p)
	if !ok || inner == "" {
		return nil, opErr("create", p, ErrIsFolder)
	}
	tmp, err := a.newTempPath(path.Base(inner))
	if err != nil {
		return nil, err
	}
	f, err := os.Create(tmp)
	if err != nil {
		return nil, err
	}
	return &arcWriter{f: f, a: a, ctx: ctx, p: p}, nil
}

// Close closes every cached archive, drops all mounts and removes the
// staging folder.
func (a *ArchiveFS) Close() error {
	a.mu.Lock()
	for p := range a.mounts {
		a.dropMountLocked(p)
	}
	for _, h := range a.handles {
		a.dropHandleLocked(h)
	}
	tmp := a.tmp
	a.tmp = ""
	a.mu.Unlock()
	if tmp != "" {
		return os.RemoveAll(tmp)
	}
	return nil
}
