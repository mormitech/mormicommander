package main

// Synchronize folders with an archive or a server as one side, or both (FEATURES.md F-555). Total Commander lets the
// Synchronize dialog compare a local folder with a zip file or an FTP folder, so a backup on a server or in an archive
// can be checked and brought up to date like any folder. The engine (internal/syncdirs) never touches the disk itself:
// each side is a syncdirs.FS, and a side it may change is a syncdirs.WritableFS. This file gives it such sides on top of
// the panels' vfs router, so every place a panel can show (an archive, FTP, SFTP, WebDAV) is a side here as well, with
// the same sessions, passwords and archive engines the panels use. Local folders stay syncdirs.DirFS, exactly as before.
//
// Three things differ from a local folder, and the side below handles them:
//
//   - Listing a server folder is one round trip, but asking for one file is one more. The compare asks for every entry
//     after listing its folder, so while comparing the side answers those questions from the listing it just read.
//     While synchronizing it always asks the server, because the engine's "unchanged since the compare" check must
//     see the real state.
//   - The engine writes a copy to a temporary file next to the target and renames it over the target at the end, so a
//     stopped copy never leaves half a file. On a server the temporary file is a real file there; a rename does not
//     replace an existing file on every server, so the old file is moved aside first and removed after. In an archive
//     every change rewrites the whole archive, so the temporary file stays on the local disk and is packed into the
//     archive once, under its final name, in a single rewrite.
//   - Servers and archives have no trash, may not keep file dates (an FTP server without MFMT, WebDAV) and can't hold
//     symbolic links. Deletes there are permanent (the Synchronize dialog says so before it starts), lost dates are
//     reported after the sync, and links are shown as what they point to, except broken ones, which stay links.
//
// A side that can't be changed at all (a RAR or 7z archive, an archive inside another one or on a server) is given to
// the engine as a read-only FS and marked with Result.SetReadOnly, so no row may copy to it or delete on it.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/syncdirs"
	"mormicommander/internal/vfs"
)

// syncMaxDepth is how deep the compare goes on a server or in an archive. Links on a server are followed (the panels
// show them as what they point to), so a link to a folder above itself would otherwise make the compare go on forever.
const syncMaxDepth = 64

// errSyncNoLinks is the error of a link copy onto a server or into an archive.
var errSyncNoLinks = errors.New("symbolic links can't be made on a server or in an archive")

// SyncSideInfo tells the page what kind of place a side is and what can be done there.
type SyncSideInfo struct {
	Kind     string `json:"kind"`          // local, archive, ftp, sftp, other (WebDAV and other plugins)
	ReadOnly bool   `json:"readOnly"`      // nothing can be copied to it or deleted on it
	Why      string `json:"why,omitempty"` // the sentence that says why it is read-only
	Trash    bool   `json:"trash"`         // deletes can go to the trash (local folders only)
	Times    bool   `json:"times"`         // copied files keep their dates
}

// syncSide is one side of a session: the folder as the page shows it, and the file system the engine works on.
type syncSide struct {
	path string       // the canonical path or URL (never with a password)
	raw  string       // the text of the field it came from, to recognise the same pair again
	fs   syncdirs.FS  // what the engine gets
	vfs  *syncVFS     // nil for a local folder
	info SyncSideInfo // for the page
}

// syncConnect opens a server URL the way the path bar does (asking for a password or a host key when needed) and
// returns its canonical panel path. Tests replace it.
var syncConnect = func(raw string) (string, error) {
	if theNet == nil {
		return "", errors.New("the network feature is not loaded")
	}
	c, err := theNet.Connect(raw)
	return c.Path, err
}

// syncOpenSide checks the text of a folder field and makes the side for it: a local folder as before, or an archive
// or server folder through the panels' router.
func (s *SyncDirs) syncOpenSide(side, raw string) (*syncSide, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("Choose the %s folder.", side)
	}
	r := vfsRouter(s.app)
	if !strings.Contains(raw, "://") {
		p := raw
		if strings.HasPrefix(p, "~") {
			if h, err := os.UserHomeDir(); err == nil {
				p = h + p[1:]
			}
		}
		p = filepath.Clean(p)
		fi, err := os.Stat(p)
		if err == nil && fi.IsDir() {
			return &syncSide{path: p, raw: raw, fs: syncdirs.DirFS(p), info: SyncSideInfo{Kind: "local", Trash: true, Times: true}}, nil
		}
		// Not a folder on the disk: it may be an archive file, or a folder inside one (/backup/2024.zip/docs).
		inArchive := r != nil && ((err == nil && fi.Mode().IsRegular() && (r.Archives.IsArchive(filepath.Base(p)) || vfsEngineOf(p) != "")) ||
			(err != nil && r.Kind(p) == vfs.KindArchive))
		if !inArchive {
			if err == nil {
				return nil, fmt.Errorf("%s is not a folder.", p)
			}
			return nil, fmt.Errorf("The %s folder %s is not there.", side, p)
		}
		return s.syncVFSSide(r, side, r.Clean(p), raw)
	}
	if r == nil {
		return nil, fmt.Errorf("The %s folder %s is not on this computer, and servers can't be opened here.", side, raw)
	}
	if netSchemes[netSchemeOf(raw)] == "" {
		// Search results (find://) and other addresses of the panels name files, not one folder on a server.
		return nil, fmt.Errorf("The %s folder %s can't be synchronized: use a folder, an archive or a server address (ftp://, sftp://, webdav://).", side, raw)
	}
	p, err := syncConnect(raw)
	if err != nil {
		return nil, err
	}
	return s.syncVFSSide(r, side, p, raw)
}

// syncVFSSide makes the side of an archive or server folder p, after checking that it can be listed.
func (s *SyncDirs) syncVFSSide(r *vfs.Router, side, p, raw string) (*syncSide, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := r.List(ctx, p); err != nil {
		return nil, fmt.Errorf("The %s folder can't be read: %s", side, listError(p, err))
	}
	var pi PathInfo
	if f := fsOf(s.app); f != nil {
		pi = f.PathInfo(p)
	} else {
		pi = PathInfo{Kind: vfsPathKind(r, p)}
	}
	if pi.Kind == "unknown" || pi.Kind == "local" || pi.Kind == "" {
		return nil, fmt.Errorf("mormiCommander can't synchronize %s: this kind of address is not supported.", p)
	}
	info := SyncSideInfo{Kind: pi.Kind, ReadOnly: pi.ReadOnly, Why: pi.ReadOnlyWhy}
	if info.ReadOnly && info.Why == "" {
		info.Why = fmt.Sprintf("%s can't be changed: the server does not allow writing there.", p)
	}
	v := newSyncVFS(r, p, pi.Kind == "archive")
	info.Times = v.archive || r.Caps(p).Chtimes
	ss := &syncSide{path: p, raw: raw, vfs: v, info: info, fs: v}
	if info.ReadOnly {
		ss.fs = syncVFSReader{v}
	}
	return ss, nil
}

// sameSide says whether the text of a field still names the folder compared last (compared, the side made for it),
// without connecting anywhere: for a local folder the same folder as before, for an archive or server folder the same
// text or the same canonical path.
func (s *SyncDirs) sameSide(sd *syncSide, compared, raw string) bool {
	raw = strings.TrimSpace(raw)
	if sd == nil || sd.vfs == nil {
		p, err := s.syncDir("", raw)
		return err == nil && p == compared
	}
	if raw == sd.raw || raw == sd.path {
		return true
	}
	return sd.vfs.r.Clean(raw) == sd.path
}

// syncBegin gets the archive and server sides ready for a compare (caching listings) or a sync (asking every time).
func syncBegin(ctx context.Context, caching bool, sides ...*syncSide) {
	for _, sd := range sides {
		if sd != nil && sd.vfs != nil {
			sd.vfs.begin(ctx, caching)
		}
	}
}

// syncEnd ends the work of syncBegin and returns the warnings for the page: the sides where copied files could not
// keep their dates, which a compare by date would then see as newer.
func syncEnd(sides ...*syncSide) []string {
	warnings := []string{}
	for i, sd := range sides {
		if sd == nil || sd.vfs == nil {
			continue
		}
		if sd.vfs.end() {
			name := [...]string{"left", "right"}[i%2]
			warnings = append(warnings, fmt.Sprintf("The %s side (%s) can't set file dates, so the files copied there carry the time of the copy. "+
				"Compare again by content, or with Ignore date, to check them.", name, sd.path))
		}
	}
	return warnings
}

// syncTrash is ExecOptions.Trash for any side: the system trash for a local folder, and a plain (permanent) delete on
// a server or in an archive, which have no trash; the Synchronize dialog says so before the user starts.
func syncTrash(f syncdirs.WritableFS, name string) error {
	if v, ok := f.(*syncVFS); ok {
		return v.Remove(name)
	}
	return syncdirs.SystemTrash(f, name)
}

// ---- the side ---------------------------------------------------------------------------------------------------

// syncVFS is an archive or server folder as a side of the sync engine. The engine calls it from one goroutine at a
// time (the session is busy while it works), but the page may cancel from another, hence the lock.
type syncVFS struct {
	r       *vfs.Router
	root    string
	archive bool // inside an archive: copies are packed in one rewrite each (see CreateTemp)

	mu       sync.Mutex
	ctx      context.Context
	caching  bool
	listed   map[string]map[string]vfs.Entry // folder (engine path) → its entries by name, while comparing
	staged   map[string]string               // archive: temp name → the local file that holds the copy
	pending  map[string]time.Time            // server: temp name → the date to give the file after the rename
	stageDir string
	lostTime bool // a copied file could not get its date (the server can't set dates)
}

func newSyncVFS(r *vfs.Router, root string, archive bool) *syncVFS {
	return &syncVFS{r: r, root: root, archive: archive, ctx: context.Background(), staged: map[string]string{}, pending: map[string]time.Time{}}
}

// begin sets the context of the work that starts (a compare with caching, or a sync without).
func (v *syncVFS) begin(ctx context.Context, caching bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ctx, v.caching, v.listed = ctx, caching, nil
	if caching {
		v.listed = map[string]map[string]vfs.Entry{}
	}
}

// end drops the cache and any copy staged for an archive that the engine did not finish, and returns whether a copied
// file lost its date.
func (v *syncVFS) end() (lostTime bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ctx, v.caching, v.listed = context.Background(), false, nil
	for _, l := range v.staged {
		os.Remove(l)
	}
	v.staged = map[string]string{}
	v.pending = map[string]time.Time{}
	if v.stageDir != "" {
		os.RemoveAll(v.stageDir)
		v.stageDir = ""
	}
	lostTime, v.lostTime = v.lostTime, false
	return lostTime
}

func (v *syncVFS) context() context.Context {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.ctx
}

// full turns an engine path ("docs/a.txt", "." for the root) into the router path, one name at a time so each file
// system puts its own separator in.
func (v *syncVFS) full(name string) string {
	if name == "." || name == "" {
		return v.root
	}
	p := v.root
	for _, part := range strings.Split(name, "/") {
		p = v.r.Join(p, part)
	}
	return p
}

// syncInfo is a vfs entry as the fs.FileInfo and fs.DirEntry the engine reads.
type syncInfo struct{ e vfs.Entry }

func (i syncInfo) Name() string { return i.e.Name }
func (i syncInfo) Size() int64 {
	if i.e.Flags&vfs.FlagDir != 0 {
		return 0
	}
	return i.e.Size
}
func (i syncInfo) Mode() fs.FileMode {
	perm := fs.FileMode(i.e.Mode).Perm()
	switch {
	case i.e.Flags&vfs.FlagBroken != 0:
		// A broken link can't be read or copied; as a link the engine lists it and can delete it, nothing more.
		return fs.ModeSymlink | 0o777
	case i.e.Flags&vfs.FlagDir != 0:
		if perm == 0 {
			perm = 0o755
		}
		return fs.ModeDir | perm
	}
	if perm == 0 {
		perm = 0o644
	}
	return perm
}
func (i syncInfo) ModTime() time.Time         { return time.UnixMilli(i.e.MTime) }
func (i syncInfo) IsDir() bool                { return i.Mode().IsDir() }
func (i syncInfo) Sys() any                   { return nil }
func (i syncInfo) Type() fs.FileMode          { return i.Mode().Type() }
func (i syncInfo) Info() (fs.FileInfo, error) { return i, nil }

// Lstat implements syncdirs.FS. While comparing, an entry of a folder that was just listed comes from that listing.
func (v *syncVFS) Lstat(name string) (fs.FileInfo, error) {
	if name == "." || name == "" {
		return v.statRoot()
	}
	v.mu.Lock()
	if v.caching {
		if es, ok := v.listed[path.Dir(name)]; ok {
			e, found := es[path.Base(name)]
			v.mu.Unlock()
			if !found {
				return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrNotExist}
			}
			return syncInfo{e}, nil
		}
	}
	v.mu.Unlock()
	e, err := v.r.Stat(v.full(name))
	if err != nil {
		return nil, err
	}
	return syncInfo{e}, nil
}

// statRoot describes the side's top folder. An archive file itself is a file for the router's Stat, but it is the
// root folder here, as in a panel.
func (v *syncVFS) statRoot() (fs.FileInfo, error) {
	e, err := v.r.Stat(v.root)
	if err == nil && e.Flags&vfs.FlagDir != 0 {
		return syncInfo{e}, nil
	}
	if _, lerr := v.r.List(v.context(), v.root); lerr != nil {
		if err == nil {
			err = lerr
		}
		return nil, err
	}
	return syncInfo{vfs.Entry{Name: path.Base(filepath.ToSlash(v.root)), Flags: vfs.FlagDir, MTime: e.MTime, Mode: uint32(fs.ModeDir | 0o755)}}, nil
}

// Stat implements syncdirs.FS; links are already followed by the file systems, so it is Lstat.
func (v *syncVFS) Stat(name string) (fs.FileInfo, error) {
	fi, err := v.Lstat(name)
	if err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return fi, err
}

// ReadDir implements syncdirs.FS.
func (v *syncVFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." && strings.Count(name, "/") >= syncMaxDepth {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errors.New("folders nested too deep (a link to a folder above itself?)")}
	}
	l, err := v.r.List(v.context(), v.full(name))
	if err != nil {
		return nil, err
	}
	out := make([]fs.DirEntry, 0, len(l.Entries))
	byName := make(map[string]vfs.Entry, len(l.Entries))
	for _, e := range l.Entries {
		if e.Name == "" || e.Name == "." || e.Name == ".." || strings.Contains(e.Name, "/") {
			continue
		}
		out = append(out, syncInfo{e})
		byName[e.Name] = e
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	v.mu.Lock()
	if v.caching {
		v.listed[name] = byName
	}
	v.mu.Unlock()
	return out, nil
}

// Open implements syncdirs.FS. For a server file this downloads it as it is read (by-content compares, copies).
func (v *syncVFS) Open(name string) (io.ReadCloser, error) {
	return v.r.Open(v.context(), v.full(name))
}

// Readlink implements syncdirs.FS. The file systems behind the router don't tell link targets.
func (v *syncVFS) Readlink(name string) (string, error) {
	return "", &fs.PathError{Op: "readlink", Path: name, Err: vfs.ErrUnsupported}
}

// Mkdir implements syncdirs.WritableFS.
func (v *syncVFS) Mkdir(name string, _ fs.FileMode) error {
	return v.r.Mkdir(v.context(), v.full(name))
}

// tempName is a name for a copy in progress, hidden and unlikely to meet a real file.
func tempName(dir string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return path.Join(dir, ".mcsync-"+hex.EncodeToString(b)+".tmp")
}

// CreateTemp implements syncdirs.WritableFS. In an archive the copy is written to a local file and packed by Rename;
// on a server it is written to a temporary file in the target folder.
func (v *syncVFS) CreateTemp(dir string) (io.WriteCloser, string, error) {
	name := tempName(dir)
	if v.archive {
		v.mu.Lock()
		if v.stageDir == "" {
			d, err := os.MkdirTemp("", "mormicommander-sync-")
			if err != nil {
				v.mu.Unlock()
				return nil, "", err
			}
			v.stageDir = d
		}
		local := filepath.Join(v.stageDir, path.Base(name))
		v.mu.Unlock()
		f, err := os.Create(local)
		if err != nil {
			return nil, "", err
		}
		v.mu.Lock()
		v.staged[name] = local
		v.mu.Unlock()
		return f, name, nil
	}
	w, err := v.r.Create(v.context(), v.full(name))
	if err != nil {
		return nil, "", err
	}
	return w, name, nil
}

// stagedFile returns the local file of an archive copy in progress.
func (v *syncVFS) stagedFile(name string) (string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	l, ok := v.staged[name]
	return l, ok
}

// Rename implements syncdirs.WritableFS. The engine only renames its temporary copies over their targets.
func (v *syncVFS) Rename(oldname, newname string) error {
	ctx := v.context()
	if local, ok := v.stagedFile(oldname); ok {
		// One archive rewrite: the local copy goes in under its final name, replacing the old entry, with the date
		// the engine gave the local file.
		if _, err := v.r.Upload(ctx, local, v.full(newname), vfs.TransferOptions{PreserveTime: true}); err != nil {
			return err
		}
		v.mu.Lock()
		delete(v.staged, oldname)
		v.mu.Unlock()
		os.Remove(local)
		return nil
	}
	from, to := v.full(oldname), v.full(newname)
	err := v.r.Rename(ctx, from, to)
	if errors.Is(err, fs.ErrExist) {
		// Most servers refuse to rename over a file. Move the old file aside, put the copy in its place, and only
		// then remove the old one, so a failure in between never leaves the target missing.
		aside := v.full(tempName(path.Dir(newname)))
		if err = v.r.Rename(ctx, to, aside); err != nil {
			return err
		}
		if err = v.r.Rename(ctx, from, to); err != nil {
			_ = v.r.Rename(ctx, aside, to)
			return err
		}
		_ = v.r.Remove(ctx, aside)
	}
	if err != nil {
		return err
	}
	v.mu.Lock()
	mt, ok := v.pending[oldname]
	delete(v.pending, oldname)
	v.mu.Unlock()
	if ok {
		// The date is set after the rename: some servers give a renamed file the time of the rename.
		if !v.r.Caps(to).Chtimes || v.r.Chtimes(ctx, to, mt) != nil {
			v.mu.Lock()
			v.lostTime = true
			v.mu.Unlock()
		}
	}
	return nil
}

// Remove implements syncdirs.WritableFS.
func (v *syncVFS) Remove(name string) error {
	if local, ok := v.stagedFile(name); ok {
		v.mu.Lock()
		delete(v.staged, name)
		v.mu.Unlock()
		return os.Remove(local)
	}
	v.mu.Lock()
	delete(v.pending, name)
	v.mu.Unlock()
	return v.r.Remove(v.context(), v.full(name))
}

// Chtimes implements syncdirs.WritableFS. A copy in progress gets its date later (on the local file packed into the
// archive, or on the server file after the rename); a folder whose date can't be set keeps the server's date, which
// the compare does not look at.
func (v *syncVFS) Chtimes(name string, mtime time.Time) error {
	if local, ok := v.stagedFile(name); ok {
		return os.Chtimes(local, mtime, mtime)
	}
	if strings.HasPrefix(path.Base(name), ".mcsync-") {
		v.mu.Lock()
		v.pending[name] = mtime
		v.mu.Unlock()
		return nil
	}
	p := v.full(name)
	if v.archive || !v.r.Caps(p).Chtimes {
		return nil
	}
	return v.r.Chtimes(v.context(), p, mtime)
}

// Chmod implements syncdirs.WritableFS; the engine ignores its errors (archives and many servers have no modes).
func (v *syncVFS) Chmod(name string, mode fs.FileMode) error {
	if local, ok := v.stagedFile(name); ok {
		return os.Chmod(local, mode)
	}
	p := v.full(name)
	if v.archive || !v.r.Caps(p).Chmod {
		return vfs.ErrUnsupported
	}
	return v.r.Chmod(v.context(), p, mode)
}

// Symlink implements syncdirs.WritableFS: not possible on these file systems.
func (v *syncVFS) Symlink(_, name string) error {
	return &fs.PathError{Op: "symlink", Path: name, Err: errSyncNoLinks}
}

// syncVFSReader is a read-only side: it has the reading half of syncVFS only, so the engine sees no WritableFS.
type syncVFSReader struct{ v *syncVFS }

func (r syncVFSReader) Lstat(name string) (fs.FileInfo, error)     { return r.v.Lstat(name) }
func (r syncVFSReader) Stat(name string) (fs.FileInfo, error)      { return r.v.Stat(name) }
func (r syncVFSReader) ReadDir(name string) ([]fs.DirEntry, error) { return r.v.ReadDir(name) }
func (r syncVFSReader) Open(name string) (io.ReadCloser, error)    { return r.v.Open(name) }
func (r syncVFSReader) Readlink(name string) (string, error)       { return r.v.Readlink(name) }
