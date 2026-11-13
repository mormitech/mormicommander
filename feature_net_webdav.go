package main

// WebDAV shares in a panel (TC's WebDAV file system plugin, F-645): webdav://user@host/path is served over HTTP and
// webdavs://user@host/path over HTTPS (dav:// and davs:// too). netDAV implements vfs.FS and the optional operations
// on top of internal/webdav and is registered with vfs.Register for those schemes, so the panels, the router and the
// transfer jobs treat a WebDAV share like an FTP or SFTP server. Paths are server paths (the share's base URL is the
// path the user connects to); one client per scheme, user, host and port is shared by both panels.

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/remotefs"
	"mormicommander/internal/vfs"
	"mormicommander/internal/webdav"
)

// netDAVConfig is what the connection layer may fill in before a share is first used.
type netDAVConfig struct {
	User     string
	Password string
}

// netDAV is the file system of WebDAV shares.
type netDAV struct {
	configure func(key string, c *netDAVConfig) error

	mu       sync.Mutex
	sessions map[string]*netDAVSession
}

type netDAVSession struct {
	base netURL // scheme, user, host, port (no path, no password)
	c    *webdav.Client
}

func newNetDAV(configure func(key string, c *netDAVConfig) error) *netDAV {
	return &netDAV{configure: configure, sessions: map[string]*netDAVSession{}}
}

// session returns the client for the URL p (made when new) and the server path p names.
func (d *netDAV) session(ctx context.Context, p string) (*netDAVSession, string, error) {
	u, err := netParseURL(p)
	if err != nil {
		return nil, "", err
	}
	if netSchemes[u.Scheme] != "webdav" {
		return nil, "", errors.New("not a WebDAV address: " + p)
	}
	key := u.key()
	rp := u.Path
	if rp == "" {
		rp = "/"
	}
	d.mu.Lock()
	s := d.sessions[key]
	d.mu.Unlock()
	if s != nil {
		return s, rp, nil
	}
	cfg := netDAVConfig{User: u.User, Password: u.Password}
	if d.configure != nil {
		if err := d.configure(key, &cfg); err != nil {
			return nil, "", err
		}
	}
	scheme := "http"
	if u.Scheme == "webdavs" {
		scheme = "https"
	}
	host := u.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if u.Port != 0 {
		host += ":" + strconv.Itoa(u.Port)
	}
	c, err := webdav.New(webdav.Config{URL: scheme + "://" + host + "/", User: cfg.User, Password: cfg.Password})
	if err != nil {
		return nil, "", err
	}
	// one request checks that the server answers and takes the login
	if _, err := c.Stat(rp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	base := u
	base.Path, base.Password, base.HasPass = "", "", false
	s = &netDAVSession{base: base, c: c}
	d.mu.Lock()
	if old := d.sessions[key]; old != nil {
		s = old
	} else {
		d.sessions[key] = s
	}
	d.mu.Unlock()
	return s, rp, nil
}

func (s *netDAVSession) url(rp string) string {
	u := s.base
	u.Path = path.Clean("/" + rp)
	return u.String()
}

// Connect opens the share of u and returns the panel path to show.
func (d *netDAV) Connect(ctx context.Context, u netURL) (string, error) {
	s, rp, err := d.session(ctx, u.withPassword())
	if err != nil {
		return "", err
	}
	e, err := s.c.Stat(rp)
	if err != nil {
		return "", err
	}
	if !e.IsDir {
		rp = path.Dir(rp)
	}
	return s.url(rp), nil
}

// Disconnect forgets the client of a session key.
func (d *netDAV) Disconnect(key string) {
	d.mu.Lock()
	s := d.sessions[key]
	delete(d.sessions, key)
	d.mu.Unlock()
	if s != nil {
		s.c.Close()
	}
}

// Sessions lists the open shares as panel URLs of their roots.
func (d *netDAV) Sessions() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.sessions))
	for _, s := range d.sessions {
		out = append(out, s.url("/"))
	}
	sort.Strings(out)
	return out
}

// Close forgets every session.
func (d *netDAV) Close() error {
	d.mu.Lock()
	ss := d.sessions
	d.sessions = map[string]*netDAVSession{}
	d.mu.Unlock()
	for _, s := range ss {
		s.c.Close()
	}
	return nil
}

func netDAVEntry(e remotefs.Entry) vfs.Entry {
	out := vfs.Entry{Name: e.Name, Mode: uint32(e.Mode)}
	if !e.ModTime.IsZero() {
		out.MTime = e.ModTime.UnixMilli()
	}
	if e.IsDir {
		out.Flags |= vfs.FlagDir
		out.Mode |= uint32(fs.ModeDir)
	} else {
		out.Size = e.Size
	}
	if strings.HasPrefix(e.Name, ".") {
		out.Flags |= vfs.FlagHidden
	}
	return out
}

// List reads a folder of the share.
func (d *netDAV) List(ctx context.Context, p string) (vfs.Listing, error) {
	s, rp, err := d.session(ctx, p)
	if err != nil {
		return vfs.Listing{}, err
	}
	es, err := s.c.ReadDir(rp)
	if err != nil {
		if e, serr := s.c.Stat(rp); serr == nil && !e.IsDir {
			return vfs.Listing{}, &fs.PathError{Op: "list", Path: p, Err: vfs.ErrNotFolder}
		}
		return vfs.Listing{}, err
	}
	if err := ctx.Err(); err != nil {
		return vfs.Listing{}, err
	}
	out := make([]vfs.Entry, 0, len(es))
	for _, e := range es {
		if e.Name == "" || e.Name == "." || e.Name == ".." {
			continue
		}
		out = append(out, netDAVEntry(e))
	}
	lp := s.url(rp)
	return vfs.Listing{Path: lp, Parent: d.Parent(lp), Entries: out}, nil
}

// Stat describes one path of the share.
func (d *netDAV) Stat(p string) (vfs.Entry, error) {
	s, rp, err := d.session(context.Background(), p)
	if err != nil {
		return vfs.Entry{}, err
	}
	if rp == "/" {
		return vfs.Entry{Name: "/", Flags: vfs.FlagDir, Mode: uint32(fs.ModeDir | 0o755)}, nil
	}
	e, err := s.c.Stat(rp)
	if err != nil {
		return vfs.Entry{}, err
	}
	if e.Name == "" {
		e.Name = path.Base(rp)
	}
	return netDAVEntry(e), nil
}

// Clean makes a share URL canonical.
func (d *netDAV) Clean(p string) string {
	u, err := netParseURL(p)
	if err != nil {
		return p
	}
	return u.String()
}

// Parent is the folder above p, "" at the server root.
func (d *netDAV) Parent(p string) string {
	u, err := netParseURL(p)
	if err != nil || u.Path == "" || u.Path == "/" {
		return ""
	}
	u.Path = path.Dir(u.Path)
	return u.String()
}

// Root is the server root of p.
func (d *netDAV) Root(p string) string {
	u, err := netParseURL(p)
	if err != nil {
		return p
	}
	u.Path = "/"
	return u.String()
}

// Join puts a name under a folder of the share.
func (d *netDAV) Join(dir, name string) string {
	u, err := netParseURL(dir)
	if err != nil {
		return strings.TrimSuffix(dir, "/") + "/" + name
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	u.Path = path.Join(p, name)
	return u.String()
}

// Caps tells what works on a share: everything but permissions and times.
func (d *netDAV) Caps(string) vfs.Caps {
	return vfs.Caps{Read: true, Write: true, Mkdir: true, Delete: true, Rename: true, Remote: true}
}

// Open reads a file of the share.
func (d *netDAV) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	s, rp, err := d.session(ctx, p)
	if err != nil {
		return nil, err
	}
	return s.c.Open(rp)
}

// Create writes a file of the share (sent when closed).
func (d *netDAV) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	s, rp, err := d.session(ctx, p)
	if err != nil {
		return nil, err
	}
	return s.c.Create(rp)
}

// Mkdir makes a folder (and the folders above it).
func (d *netDAV) Mkdir(ctx context.Context, p string) error {
	s, rp, err := d.session(ctx, p)
	if err != nil {
		return err
	}
	if e, err := s.c.Stat(rp); err == nil && e.IsDir {
		return &fs.PathError{Op: "mkdir", Path: p, Err: fs.ErrExist}
	}
	return s.c.MkdirAll(rp)
}

// Remove deletes a file or an empty folder.
func (d *netDAV) Remove(ctx context.Context, p string) error {
	s, rp, err := d.session(ctx, p)
	if err != nil {
		return err
	}
	return s.c.Remove(rp)
}

// RemoveAll deletes a file or a folder with everything in it.
func (d *netDAV) RemoveAll(ctx context.Context, p string) error {
	s, rp, err := d.session(ctx, p)
	if err != nil {
		return err
	}
	return s.c.RemoveAll(rp)
}

// Rename moves within one share (server-side MOVE); other places give vfs.ErrCrossFS.
func (d *netDAV) Rename(ctx context.Context, from, to string) error {
	s1, a, err := d.session(ctx, from)
	if err != nil {
		return err
	}
	s2, b, err := d.session(ctx, to)
	if err != nil || s1 != s2 {
		return &fs.PathError{Op: "rename", Path: from, Err: vfs.ErrCrossFS}
	}
	return s1.c.Rename(a, b)
}

func netDAVOpts(opt vfs.TransferOptions) remotefs.TransferOptions {
	o := remotefs.TransferOptions{Resume: opt.Resume, PreserveTimes: opt.PreserveTime}
	if opt.Progress != nil {
		o.Progress = func(p remotefs.Progress) { opt.Progress(p.Done, p.Total) }
	}
	return o
}

// Download copies a file of the share to the local disk (with resume by HTTP range).
func (d *netDAV) Download(ctx context.Context, p, local string, opt vfs.TransferOptions) (int64, error) {
	s, rp, err := d.session(ctx, p)
	if err != nil {
		return 0, err
	}
	return s.c.Download(ctx, rp, local, netDAVOpts(opt))
}

// Upload copies a local file to the share.
func (d *netDAV) Upload(ctx context.Context, local, p string, opt vfs.TransferOptions) (int64, error) {
	s, rp, err := d.session(ctx, p)
	if err != nil {
		return 0, err
	}
	return s.c.Upload(ctx, local, rp, netDAVOpts(opt))
}

// Chtimes is not possible over WebDAV; uploads keep the server's time.
func (d *netDAV) Chtimes(ctx context.Context, p string, mtime time.Time) error {
	return &fs.PathError{Op: "chtimes", Path: p, Err: vfs.ErrUnsupported}
}

// Chmod is not possible over WebDAV.
func (d *netDAV) Chmod(ctx context.Context, p string, mode fs.FileMode) error {
	return &fs.PathError{Op: "chmod", Path: p, Err: vfs.ErrUnsupported}
}
