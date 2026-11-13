// This file is the path router: the one FS a panel talks to. In Total
// Commander the path bar accepts anything: a local folder, a path that runs
// into an archive (C:\backup\2024.zip\docs), an FTP URL or an SFTP URL, and
// the panel just shows it. The router looks at a path and hands the call to
// the file system that owns it:
//
//   - URLs go by scheme: ftp://, ftps:// and ftpes:// to FTPFS; sftp:// and
//     scp:// to SFTPFS; any other scheme to a file system registered with
//     Register (plugins).
//   - A local path that exists on disk is local. A local path that does not
//     exist, but whose first existing ancestor is an archive file, is inside
//     that archive and goes to ArchiveFS.
//   - The archive file itself is a file for every operation (Stat, Open,
//     Remove, Rename) except List, which shows its contents, so Enter on an
//     archive and Backspace out of it both just work.
//   - An archive on an FTP or SFTP server is downloaded to a temp file when
//     the user enters it, then browsed read-only through ArchiveFS, which is
//     how TC handles archives on servers. Entering it again re-checks the
//     server copy's size and date and reloads when it changed.
//
// Router implements FS and every optional interface of adapter_ops.go by
// delegation, plus Connect/Disconnect for the network file systems and
// CanEnter for the Enter key, so the UI needs exactly one object.
package vfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"mormicommander/internal/archive"
	"mormicommander/internal/archive/tarfmt"
)

// Kind classifies a panel path.
type Kind int

const (
	KindLocal   Kind = iota // a path on the local disk
	KindArchive             // inside an archive (local, nested or on a server)
	KindFTP                 // an FTP server
	KindSFTP                // an SFTP server
	KindOther               // a URL served by a registered plugin FS
	KindUnknown             // a URL with a scheme nobody serves
)

// String names the kind for logs and tests.
func (k Kind) String() string {
	return [...]string{"local", "archive", "ftp", "sftp", "other", "unknown"}[k]
}

// Router dispatches panel paths to the file system that owns them. Create
// it with NewRouter; all fields may be replaced before first use.
type Router struct {
	Local    FS // the local disk; NewRouter sets LocalRW{}
	Archives *ArchiveFS
	FTP      *FTPFS
	SFTP     *SFTPFS
	// NoRemoteArchives turns off entering archives that live on servers.
	NoRemoteArchives bool

	mu      sync.Mutex
	remotes map[string]remoteStamp // remote archive mounts by panel path
}

// remoteStamp records the server copy a remote archive mount came from.
type remoteStamp struct {
	size  int64
	mtime int64
}

// NewRouter returns a router with fresh local, archive, FTP and SFTP file
// systems.
func NewRouter() *Router {
	return &Router{
		Local:    LocalRW{},
		Archives: NewArchiveFS(),
		FTP:      NewFTPFS(),
		SFTP:     NewSFTPFS(),
		remotes:  map[string]remoteStamp{},
	}
}

// RegisterSchemes registers the router with the package registry for the
// FTP and SFTP schemes, so vfs.For finds it for such URLs.
func (r *Router) RegisterSchemes() {
	for _, s := range []string{"ftp", "ftps", "ftpes", "sftp", "scp"} {
		Register(s, r)
	}
}

// unknownFS answers every call for a URL scheme nobody serves.
type unknownFS struct{ scheme string }

func (u unknownFS) err(p string) error {
	return &fs.PathError{Op: "open", Path: p, Err: ErrUnsupported}
}
func (u unknownFS) List(_ context.Context, p string) (Listing, error) { return Listing{}, u.err(p) }
func (u unknownFS) Stat(p string) (Entry, error)                      { return Entry{}, u.err(p) }
func (u unknownFS) Clean(p string) string                             { return p }
func (u unknownFS) Parent(p string) string                            { return urlParent(p) }
func (u unknownFS) Root(p string) string                              { return urlRoot(p) }
func (u unknownFS) Join(dir, name string) string                      { return urlJoin(dir, name) }

// network returns the FTP or SFTP file system for a URL, or nil.
func (r *Router) network(p string) FS {
	switch {
	case IsFTPURL(p) && r.FTP != nil:
		return r.FTP
	case IsSFTPURL(p) && r.SFTP != nil:
		return r.SFTP
	}
	return nil
}

// Resolve returns the file system that owns p and p in its canonical form.
// An archive file itself resolves to the file system it is stored on.
func (r *Router) Resolve(p string) (FS, string) {
	if hasScheme(p) {
		n := r.network(p)
		cp := cleanURL(p)
		if n != nil {
			cp = n.Clean(p)
		}
		if r.Archives != nil {
			if m, prefix, rest := r.Archives.findMount(cp, true); m != nil && len(rest) > 0 {
				return r.Archives, displayJoin(prefix, strings.Join(rest, "/"))
			}
		}
		if n != nil {
			return n, cp
		}
		scheme := strings.ToLower(p[:strings.Index(p, "://")])
		if f := For(p, nil); f != nil && f != FS(r) {
			return f, f.Clean(p)
		}
		return unknownFS{scheme}, p
	}
	c := r.Local.Clean(p)
	if _, err := os.Lstat(c); err == nil || r.Archives == nil {
		return r.Local, c
	}
	if _, inner, ok := r.Archives.Split(c); ok && inner != "" {
		return r.Archives, c
	}
	return r.Local, c
}

// Kind classifies p.
func (r *Router) Kind(p string) Kind {
	f, _ := r.Resolve(p)
	switch f.(type) {
	case *ArchiveFS:
		return KindArchive
	case *FTPFS:
		return KindFTP
	case *SFTPFS:
		return KindSFTP
	case unknownFS:
		return KindUnknown
	}
	if f == r.Local {
		return KindLocal
	}
	return KindOther
}

// CanEnter tells whether Enter on the entry e in the folder dir opens it in
// the panel: folders, links to folders, and files with an archive extension.
func (r *Router) CanEnter(dir string, e Entry) bool {
	if e.Flags&FlagDir != 0 {
		return true
	}
	if r.Archives == nil || !r.Archives.IsArchive(e.Name) {
		return false
	}
	if hasScheme(dir) && r.NoRemoteArchives {
		return false
	}
	return true
}

// Connect opens a typed URL (Ctrl+F / path bar) and returns the panel path
// to show; local paths are just cleaned.
func (r *Router) Connect(ctx context.Context, raw string) (string, error) {
	switch {
	case IsFTPURL(raw) && r.FTP != nil:
		return r.FTP.Connect(ctx, raw)
	case IsSFTPURL(raw) && r.SFTP != nil:
		return r.SFTP.Connect(ctx, raw)
	}
	f, c := r.Resolve(raw)
	if _, ok := f.(unknownFS); ok {
		return "", f.(unknownFS).err(raw)
	}
	return c, nil
}

// Disconnect closes the server connection p belongs to and forgets the
// archives downloaded from it.
func (r *Router) Disconnect(p string) {
	if !hasScheme(p) {
		return
	}
	root := urlRoot(p)
	if r.Archives != nil {
		r.Archives.Unmount(root)
	}
	r.mu.Lock()
	for k := range r.remotes {
		if _, ok := underPrefix(k, root); ok {
			delete(r.remotes, k)
		}
	}
	r.mu.Unlock()
	switch {
	case IsFTPURL(p) && r.FTP != nil:
		r.FTP.Disconnect(p)
	case IsSFTPURL(p) && r.SFTP != nil:
		r.SFTP.Disconnect(p)
	}
}

// Close closes every connection and archive and removes temp files.
func (r *Router) Close() error {
	var errs []error
	if r.Archives != nil {
		errs = append(errs, r.Archives.Close())
	}
	if r.FTP != nil {
		errs = append(errs, r.FTP.Close())
	}
	if r.SFTP != nil {
		errs = append(errs, r.SFTP.Close())
	}
	return errors.Join(errs...)
}

// ---- FS -------------------------------------------------------------------

// List reads the folder p. An archive file (local or on a server) lists as
// the archive's root.
func (r *Router) List(ctx context.Context, p string) (Listing, error) {
	f, c := r.Resolve(p)
	if r.Archives == nil {
		return f.List(ctx, c)
	}
	if f == r.Local {
		if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() {
			return r.Archives.List(ctx, c)
		}
		l, err := f.List(ctx, c)
		if errors.Is(err, syscall.ENOTDIR) {
			err = &fs.PathError{Op: "list", Path: c, Err: ErrNotFolder}
		}
		return l, err
	}
	if hasScheme(c) && r.network(c) != nil {
		if r.Archives.Mounted(c) {
			if err := r.refreshRemote(ctx, f, c); err != nil {
				return Listing{}, err
			}
			return r.Archives.List(ctx, c)
		}
		l, err := f.List(ctx, c)
		if err == nil || r.NoRemoteArchives || ctx.Err() != nil {
			return l, err
		}
		// Servers word "path runs through a file" differently (550, "not a
		// directory", SSH_FX_FAILURE), so any failure triggers the search
		// for an archive file along the path.
		if ok, merr := r.mountRemote(ctx, f, c); merr != nil {
			return Listing{}, merr
		} else if !ok {
			return l, err
		}
		return r.Archives.List(ctx, c)
	}
	return f.List(ctx, c)
}

// mountRemote looks for an archive file on the server along p (the file
// itself or an ancestor), downloads it and mounts it. It reports whether
// one was found. Paths that fail to stat are skipped on the way up, because
// a path below a file gives "not a directory" or a generic failure rather
// than "not found" on many servers.
func (r *Router) mountRemote(ctx context.Context, f FS, p string) (bool, error) {
	t, ok := f.(Transferer)
	if !ok {
		return false, nil
	}
	cur := p
	for i := 0; i < 64 && cur != ""; i++ {
		e, err := f.Stat(cur)
		if err == nil {
			if e.Flags&FlagDir != 0 || !likelyArchiveName(r.Archives, e.Name) {
				return false, nil
			}
			return true, r.download(ctx, t, cur, e)
		}
		cur = f.Parent(cur)
	}
	return false, nil
}

// likelyArchiveName reports whether a server file is worth downloading to
// look inside: an Enter extension, or any name an engine recognises (.jar,
// .docx, ...). A plain file is never downloaded just to fail.
func likelyArchiveName(a *ArchiveFS, name string) bool {
	return a.IsArchive(name) || archive.IsArchiveName(name) || tarfmt.IsArchiveName(name)
}

// download fetches the server file at p into the archive staging folder
// and mounts it read-only at p.
func (r *Router) download(ctx context.Context, t Transferer, p string, e Entry) error {
	tmp, err := r.Archives.newTempPath(e.Name)
	if err != nil {
		return err
	}
	dir := tmp[:len(tmp)-len(e.Name)]
	if _, err := t.Download(ctx, p, tmp, TransferOptions{}); err != nil {
		os.RemoveAll(dir)
		return err
	}
	if arcKind(tmp) == "" {
		os.RemoveAll(dir)
		return &fs.PathError{Op: "open", Path: p, Err: ErrNotFolder}
	}
	r.Archives.Mount(p, tmp, true, func() { os.RemoveAll(dir) })
	r.mu.Lock()
	r.remotes[r.Archives.Clean(p)] = remoteStamp{size: e.Size, mtime: e.MTime}
	r.mu.Unlock()
	return nil
}

// refreshRemote reloads a mounted server archive whose server copy changed.
func (r *Router) refreshRemote(ctx context.Context, f FS, p string) error {
	key := r.Archives.Clean(p)
	r.mu.Lock()
	st, known := r.remotes[key]
	r.mu.Unlock()
	if !known {
		return nil
	}
	e, err := f.Stat(p)
	if err != nil {
		return err
	}
	if e.Size == st.size && e.MTime == st.mtime {
		return nil
	}
	r.Archives.Unmount(p)
	t, ok := f.(Transferer)
	if !ok {
		return ErrUnsupported
	}
	return r.download(ctx, t, p, e)
}

// Stat describes p.
func (r *Router) Stat(p string) (Entry, error) {
	f, c := r.Resolve(p)
	return f.Stat(c)
}

// Clean makes p canonical for the file system that owns it.
func (r *Router) Clean(p string) string {
	_, c := r.Resolve(p)
	return c
}

// Parent is the folder above p ("" at a root). Above an archive's root is
// the folder that holds the archive file.
func (r *Router) Parent(p string) string {
	f, c := r.Resolve(p)
	return f.Parent(c)
}

// Root is the top of the volume p is on: the drive or mount point, the
// archive's root inside an archive, the server root on a server.
func (r *Router) Root(p string) string {
	f, c := r.Resolve(p)
	return f.Root(c)
}

// Join puts name under dir with the separator dir uses.
func (r *Router) Join(dir, name string) string {
	if hasScheme(dir) {
		return urlJoin(dir, name)
	}
	return r.Local.Join(dir, name)
}

// ---- optional operations ----------------------------------------------------

// Caps reports what works at p.
func (r *Router) Caps(p string) Caps {
	f, c := r.Resolve(p)
	return CapsOf(f, c)
}

// Open opens a file for reading.
func (r *Router) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	f, c := r.Resolve(p)
	return OpenFile(ctx, f, c)
}

// Create creates or replaces a file.
func (r *Router) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	f, c := r.Resolve(p)
	return CreateFile(ctx, f, c)
}

// Mkdir creates a folder.
func (r *Router) Mkdir(ctx context.Context, p string) error {
	f, c := r.Resolve(p)
	return Mkdir(ctx, f, c)
}

// Remove deletes a file or an empty folder.
func (r *Router) Remove(ctx context.Context, p string) error {
	f, c := r.Resolve(p)
	return Remove(ctx, f, c)
}

// RemoveAll deletes a file or a folder tree.
func (r *Router) RemoveAll(ctx context.Context, p string) error {
	f, c := r.Resolve(p)
	return RemoveAll(ctx, f, c)
}

// Rename moves within one file system; across file systems it returns
// ErrCrossFS and the job engine copies and deletes instead.
func (r *Router) Rename(ctx context.Context, from, to string) error {
	ff, cf := r.Resolve(from)
	ft, ct := r.resolveTarget(to)
	if ff != ft {
		return opErr("rename", from, ErrCrossFS)
	}
	return Rename(ctx, ff, cf, ct)
}

// resolveTarget resolves a path that may not exist yet: a missing path whose
// parent is inside an archive is inside that archive too, which Resolve
// already handles because it walks up to the first existing ancestor.
func (r *Router) resolveTarget(p string) (FS, string) { return r.Resolve(p) }

// Chmod changes permission bits.
func (r *Router) Chmod(ctx context.Context, p string, mode fs.FileMode) error {
	f, c := r.Resolve(p)
	return Chmod(ctx, f, c, mode)
}

// Chtimes changes the modification time.
func (r *Router) Chtimes(ctx context.Context, p string, mtime time.Time) error {
	f, c := r.Resolve(p)
	return Chtimes(ctx, f, c, mtime)
}

// Download copies the file p to the local path local.
func (r *Router) Download(ctx context.Context, p, local string, opt TransferOptions) (int64, error) {
	f, c := r.Resolve(p)
	if t, ok := f.(Transferer); ok {
		return t.Download(ctx, c, r.Local.Clean(local), opt)
	}
	return CopyFile(ctx, f, c, r.Local, r.Local.Clean(local), r.Local, opt)
}

// Upload copies the local file local to p.
func (r *Router) Upload(ctx context.Context, local, p string, opt TransferOptions) (int64, error) {
	f, c := r.Resolve(p)
	if t, ok := f.(Transferer); ok {
		return t.Upload(ctx, r.Local.Clean(local), c, opt)
	}
	return CopyFile(ctx, r.Local, r.Local.Clean(local), f, c, r.Local, opt)
}

// AddLocal packs local files into an archive folder in one rewrite; for
// other file systems it uploads them one by one (folders recursively).
func (r *Router) AddLocal(ctx context.Context, dir string, sources []string) error {
	f, c := r.Resolve(dir)
	if pk, ok := f.(Packer); ok {
		return pk.AddLocal(ctx, c, sources)
	}
	if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() && f == r.Local && r.Archives != nil {
		// dir is an archive file itself: pack into its root.
		return r.Archives.AddLocal(ctx, c, sources)
	}
	for _, s := range sources {
		if err := r.uploadTree(ctx, f, s, f.Join(c, localBase(s))); err != nil {
			return err
		}
	}
	return nil
}

// uploadTree copies a local file or folder tree to dst on f.
func (r *Router) uploadTree(ctx context.Context, f FS, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		_, err := CopyFile(ctx, r.Local, src, f, dst, r.Local, TransferOptions{PreserveTime: true})
		return err
	}
	if err := Mkdir(ctx, f, dst); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	des, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, de := range des {
		if err := r.uploadTree(ctx, f, r.Local.Join(src, de.Name()), f.Join(dst, de.Name())); err != nil {
			return err
		}
	}
	return nil
}

// localBase is the last element of a local path.
func localBase(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return path.Base(p)
}
